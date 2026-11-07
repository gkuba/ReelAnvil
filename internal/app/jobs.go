package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gkuba/ReelAnvil/internal/pipeline"
	"github.com/gkuba/ReelAnvil/internal/store"
	"github.com/gkuba/ReelAnvil/internal/worker"
)

// Job runner timing.
var (
	jobPoll       = 3 * time.Second
	opPoll        = 500 * time.Millisecond
	workerWait    = 30 * time.Second  // queued this long: say the worker is not answering
	staleOpCutoff = 120 * time.Second // claimed op with no heartbeat this long fails
	modelTimeout  = 10 * time.Minute
)

// Hunter windows stay well inside the model context set below. They are kept
// to a few minutes of speech: small models given a whole long transcript pick
// only from its first minutes.
const (
	hunterWindowChars = 4000
	modelContext      = 16384
)

// jobRunner tracks the one job being run.
type jobRunner struct {
	id     int64
	cancel context.CancelFunc
	wake   chan struct{}
}

// blockedError stops a job safely (storage cap) instead of failing it.
type blockedError struct{ msg string }

func (e blockedError) Error() string { return e.msg }

// startJobs fails jobs a previous process left running and starts the runner.
func (s *Server) startJobs() {
	ctx := s.background()
	if n, err := s.store.RecoverInterrupted(ctx); err != nil {
		s.log.Printf("jobs: recover: %v", err)
	} else if n > 0 {
		s.log.Printf("jobs: %d job(s) interrupted by the last shutdown; Retry continues them", n)
	}
	s.pullMu.Lock()
	s.jobs.wake = make(chan struct{}, 1)
	s.pullMu.Unlock()
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		for {
			_ = s.store.FailStaleOps(ctx, time.Now().Add(-staleOpCutoff))
			j, ok, err := s.store.NextQueuedJob(ctx)
			if err != nil && ctx.Err() == nil {
				s.log.Printf("jobs: %v", err)
			}
			if ok {
				s.runJob(ctx, j)
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-s.jobs.wake:
			case <-time.After(jobPoll):
			}
		}
	}()
}

func (s *Server) wakeJobs() {
	s.pullMu.Lock()
	wake := s.jobs.wake
	s.pullMu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// cancelRunning stops job id if it is the one running.
func (s *Server) cancelRunning(id int64) bool {
	s.pullMu.Lock()
	defer s.pullMu.Unlock()
	if s.jobs.id == id && s.jobs.cancel != nil {
		s.jobs.cancel()
		return true
	}
	return false
}

// run is one job's context for its steps.
type run struct {
	s    *Server
	ctx  context.Context
	job  store.Job
	cfg  Settings
	root string
	// step is the running step; done is its result line; summary is the job's.
	step, done, summary string
}

func (r *run) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_ = r.s.store.AppendLog(context.Background(), r.job.ID, "info", msg)
}

func (r *run) temp(name string) string {
	return filepath.Join(folderPath(r.root, FolderTemp), fmt.Sprint(r.job.ID), name)
}

func (r *run) rel(p string) string {
	rel, err := filepath.Rel(r.root, p)
	if err != nil {
		return p
	}
	return rel
}

func (r *run) progress(done, total int64, detail string) {
	_ = r.s.store.SetStepProgress(context.Background(), r.job.ID, r.step, done, total, detail)
}

func (s *Server) runJob(parent context.Context, j store.Job) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	s.pullMu.Lock()
	s.jobs.id, s.jobs.cancel = j.ID, cancel
	s.pullMu.Unlock()
	defer func() {
		s.pullMu.Lock()
		s.jobs.id, s.jobs.cancel = 0, nil
		s.pullMu.Unlock()
	}()
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	r := &run{s: s, ctx: ctx, job: j, cfg: cfg, root: effectiveMediaRoot(cfg, s.dataDir)}
	bg := context.Background()
	if err := s.store.SetJobState(bg, j.ID, store.JobRunning, "", ""); err != nil {
		s.log.Printf("job %d: %v", j.ID, err)
		return
	}
	r.logf("Started")
	steps := map[string]func() error{"import": r.importStep, "transcribe": r.transcribeStep, "hunt": r.huntStep,
		"review": r.reviewStep, "render": r.renderStep, "qa": r.qaStep}
	for _, name := range store.Steps {
		if j.Step(name).State == store.StepDone {
			continue
		}
		r.step = name
		_ = s.store.SetStep(bg, j.ID, name, store.StepRunning, "")
		err := steps[name]()
		if err == nil {
			_ = s.store.SetStep(bg, j.ID, name, store.StepDone, r.done)
			r.done = ""
			if fresh, ferr := s.store.JobByID(bg, j.ID); ferr == nil {
				r.job = fresh
			}
			continue
		}
		if parent.Err() != nil {
			// Shutting down: the next start marks the job interrupted.
			return
		}
		state, detail, msg := store.JobFailed, "Failed", err.Error()
		var blocked blockedError
		switch {
		case ctx.Err() != nil:
			state, detail, msg = store.JobCancelled, "Cancelled", "Cancelled by the user."
		case errors.As(err, &blocked):
			state, detail = store.JobBlocked, "Blocked"
		}
		_ = s.store.SetStep(bg, j.ID, name, store.StepFailed, detail)
		_ = s.store.SetJobState(bg, j.ID, state, "", msg)
		_ = s.store.AppendLog(bg, j.ID, "error", name+": "+msg)
		s.log.Printf("job %d %s: %s", j.ID, name, msg)
		return
	}
	_ = s.store.SetJobState(bg, j.ID, store.JobCompleted, r.summary, "")
	r.logf("Finished: %s", r.summary)
}

// op hands one operation to the worker and waits for its result.
func (r *run) op(kind string, args, out any) error {
	bg := context.Background()
	id, err := r.s.store.CreateOp(bg, r.job.ID, r.step, kind, args)
	if err != nil {
		return err
	}
	queued := time.Now()
	warned := false
	for {
		select {
		case <-r.ctx.Done():
			_ = r.s.store.CancelOp(bg, id, "cancelled")
			return r.ctx.Err()
		case <-time.After(opPoll):
		}
		o, err := r.s.store.OpByID(bg, id)
		if err != nil {
			return err
		}
		switch o.State {
		case store.OpDone:
			if out == nil {
				return nil
			}
			return json.Unmarshal(o.Result, out)
		case store.OpFailed, store.OpCancelled:
			return errors.New(o.Error)
		case store.OpQueued:
			if !warned && time.Since(queued) > workerWait {
				warned = true
				_ = r.s.store.SetJobDetail(bg, r.job.ID, "Waiting for the worker. Check that the reelanvil-worker service is running.")
			}
		case store.OpClaimed:
			if warned {
				warned = false
				_ = r.s.store.SetJobDetail(bg, r.job.ID, "")
			}
		}
	}
}

// ---- import ----

func (r *run) importStep() error {
	var src string
	title := r.job.Title
	switch r.job.Origin {
	case "url":
		var d worker.DownloadResult
		if err := r.op("download", worker.DownloadArgs{URL: r.job.Ref, Dir: r.temp("download")}, &d); err != nil {
			return fmt.Errorf("download: %w", err)
		}
		src = d.Path
		if d.Title != "" {
			title = d.Title
		}
		r.logf("Downloaded %s", filepath.Base(src))
	case "file":
		p, err := incomingFile(r.root, r.job.Ref)
		if err != nil {
			return err
		}
		src = p
	default:
		return fmt.Errorf("unknown origin %q", r.job.Origin)
	}
	var p worker.ProbeResult
	if err := r.op("probe", worker.ProbeArgs{Path: src}, &p); err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	switch {
	case !p.HasVideo:
		return errors.New("the file has no video stream")
	case !p.HasAudio:
		return errors.New("the file has no audio track, so there is nothing to transcribe")
	case p.DurationMS < int64(r.job.MinS)*1000:
		return fmt.Errorf("the video is %s long, shorter than the shortest clip", pipeline.Clock(p.DurationMS))
	}
	// Keep the original in archive/, named after the job.
	archive := folderPath(r.root, FolderArchive)
	if err := os.MkdirAll(archive, folderMode); err != nil {
		return err
	}
	dst := filepath.Join(archive, fmt.Sprintf("%d-%s", r.job.ID, pipeline.SafeName(filepath.Base(src))))
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("move the source to archive/: %w", err)
	}
	r.logf("Source kept as %s", r.rel(dst))
	if title == "" {
		title = filepath.Base(src)
	}
	id, err := r.s.store.AddSource(context.Background(), store.Source{Title: title, Origin: r.job.Origin, Ref: r.job.Ref, Path: r.rel(dst),
		Size: p.Size, DurationMS: p.DurationMS, Width: p.Width, Height: p.Height, HasAudio: p.HasAudio, RightsAt: r.job.RightsAt})
	if err != nil {
		return err
	}
	r.done = fmt.Sprintf("%s, %dx%d", pipeline.Clock(p.DurationMS), p.Width, p.Height)
	return r.s.store.SetJobSource(context.Background(), r.job.ID, id)
}

// incomingFile resolves a file name in incoming/ and refuses anything else.
func incomingFile(root, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("%q is not a file name in incoming/", name)
	}
	p := filepath.Join(folderPath(root, FolderIncoming), name)
	fi, err := os.Lstat(p)
	if err != nil {
		return "", fmt.Errorf("incoming/%s: %w", name, err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("incoming/%s is not a regular file", name)
	}
	return p, nil
}

func (r *run) source() (store.Source, error) {
	return r.s.store.SourceByID(context.Background(), r.job.SourceID)
}

// ---- transcribe ----

func (r *run) transcribeStep() error {
	src, err := r.source()
	if err != nil {
		return err
	}
	pcm := r.temp("audio.s16le")
	var a worker.AudioResult
	if err := r.op("audio", worker.AudioArgs{In: filepath.Join(r.root, src.Path), Out: pcm, DurationMS: src.DurationMS}, &a); err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	defer os.Remove(pcm)
	silences := make([]pipeline.Span, len(a.Silences))
	for i, sl := range a.Silences {
		silences[i] = pipeline.Span{StartMS: sl.StartMS, EndMS: sl.EndMS}
	}
	spans := pipeline.Cut(src.DurationMS, silences)
	f, err := os.Open(pcm)
	if err != nil {
		return err
	}
	defer f.Close()
	r.logf("Transcribing %d segments with %s", len(spans), r.cfg.WhisperModel)
	var segs []store.Segment
	for i, sp := range spans {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		r.progress(int64(i), int64(len(spans)), fmt.Sprintf("Segment %d of %d", i+1, len(spans)))
		buf := make([]byte, (sp.EndMS-sp.StartMS)*32)
		n, err := f.ReadAt(buf, sp.StartMS*32)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		text, err := wyomingTranscribe(r.ctx, r.cfg.WhisperAddress, r.cfg.WhisperLanguage, buf[:n&^1])
		if err != nil {
			return fmt.Errorf("transcription at %s: %w", pipeline.Clock(sp.StartMS), err)
		}
		if text = strings.Join(strings.Fields(text), " "); text != "" {
			segs = append(segs, store.Segment{StartMS: sp.StartMS, EndMS: sp.EndMS, Text: text})
		}
	}
	r.progress(int64(len(spans)), int64(len(spans)), "")
	if len(segs) == 0 {
		return errors.New("no speech was found")
	}
	r.done = fmt.Sprintf("%d segments", len(segs))
	return r.s.store.SaveTranscript(context.Background(), r.job.ID, r.cfg.WhisperModel, r.cfg.WhisperLanguage, segs)
}

// wyomingTranscribe sends one stretch of 16 kHz mono PCM to a Wyoming ASR
// service and returns the transcript.
func wyomingTranscribe(ctx context.Context, addr, language string, pcm []byte) (string, error) {
	d := net.Dialer{Timeout: serviceTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	w := bufio.NewWriter(conn)
	event := func(typ string, data any, payload []byte) {
		head := map[string]any{"type": typ}
		if data != nil {
			head["data"] = data
		}
		if payload != nil {
			head["payload_length"] = len(payload)
		}
		b, _ := json.Marshal(head)
		w.Write(b)
		w.WriteByte('\n')
		w.Write(payload)
	}
	format := map[string]int{"rate": 16000, "width": 2, "channels": 1}
	tr := map[string]any{}
	if language != "" && language != "auto" {
		tr["language"] = language
	}
	event("transcribe", tr, nil)
	event("audio-start", format, nil)
	for off := 0; off < len(pcm); off += 32000 {
		event("audio-chunk", format, pcm[off:min(off+32000, len(pcm))])
	}
	event("audio-stop", nil, nil)
	if err := w.Flush(); err != nil {
		return "", err
	}
	rd := bufio.NewReader(io.LimitReader(conn, 8<<20))
	for {
		line, err := rd.ReadBytes('\n')
		if err != nil {
			return "", fmt.Errorf("no transcript came back: %w", err)
		}
		var head struct {
			Type          string          `json:"type"`
			Data          json.RawMessage `json:"data"`
			DataLength    int             `json:"data_length"`
			PayloadLength int             `json:"payload_length"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			return "", errors.New("the reply was not a Wyoming event")
		}
		data := []byte(head.Data)
		if head.DataLength > 0 {
			data = make([]byte, min(head.DataLength, 1<<20))
			if _, err := io.ReadFull(rd, data); err != nil {
				return "", err
			}
		}
		if head.PayloadLength > 0 {
			if _, err := io.CopyN(io.Discard, rd, int64(head.PayloadLength)); err != nil {
				return "", err
			}
		}
		if head.Type == "transcript" {
			var t struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(data, &t); err != nil {
				return "", errors.New("the transcript event had no text")
			}
			return t.Text, nil
		}
	}
}

// ---- hunt and review ----

func toPipeline(segs []store.Segment) []pipeline.Segment {
	out := make([]pipeline.Segment, len(segs))
	for i, s := range segs {
		out[i] = pipeline.Segment{StartMS: s.StartMS, EndMS: s.EndMS, Text: s.Text}
	}
	return out
}

func (r *run) brief() pipeline.Brief {
	if r.job.CampaignID == "" {
		return pipeline.Brief{}
	}
	c, err := r.s.store.CampaignByID(context.Background(), r.job.CampaignID)
	if err != nil {
		return pipeline.Brief{}
	}
	return pipeline.Brief{Title: c.Title, Requirements: c.Requirements}
}

func (r *run) segments() ([]pipeline.Segment, error) {
	segs, err := r.s.store.Transcript(context.Background(), r.job.ID)
	if err != nil {
		return nil, err
	}
	return toPipeline(segs), nil
}

func (r *run) huntStep() error {
	model := r.cfg.HunterModel
	if model == "" {
		return blockedError{"Choose a hunter model in Settings → AI, then retry."}
	}
	segs, err := r.segments()
	if err != nil {
		return err
	}
	brief := r.brief()
	windows := pipeline.Windows(segs, hunterWindowChars)
	r.logf("Looking for clips with %s in %d part(s) of the transcript", model, len(windows))
	var all []pipeline.Pick
	for i, win := range windows {
		r.progress(int64(i), int64(len(windows)), fmt.Sprintf("Part %d of %d", i+1, len(windows)))
		prompt := pipeline.HunterPrompt(brief, segs, win[0], win[1], r.job.ClipCount, r.job.MinS, r.job.MaxS)
		raw, err := ollamaGenerate(r.ctx, r.cfg.OllamaAddress, model, prompt)
		if err != nil {
			return fmt.Errorf("hunter model: %w", err)
		}
		picks, problems := pipeline.ParseHunt(raw, segs, win[0], win[1], r.job.MinS, r.job.MaxS)
		if len(picks) == 0 && len(problems) > 0 {
			r.logf("Part %d: asking again (%s)", i+1, clip(strings.Join(problems, "; "), 300))
			if raw, err = ollamaGenerate(r.ctx, r.cfg.OllamaAddress, model, pipeline.RetryPrompt(prompt, problems)); err != nil {
				return fmt.Errorf("hunter model: %w", err)
			}
			picks, problems = pipeline.ParseHunt(raw, segs, win[0], win[1], r.job.MinS, r.job.MaxS)
		}
		if len(problems) > 0 {
			r.logf("Part %d: %d suggestion(s) skipped: %s", i+1, len(problems), clip(strings.Join(problems, "; "), 300))
		}
		all = append(all, picks...)
	}
	r.progress(int64(len(windows)), int64(len(windows)), "")
	// Twice the clip count, so the reviewer can turn some down.
	best := pipeline.Best(all, r.job.ClipCount*2)
	if len(best) == 0 {
		return errors.New("no usable clip suggestions came back; try other clip lengths or another model")
	}
	cands := make([]store.Candidate, len(best))
	for i, p := range best {
		sp := p.Span(segs)
		cands[i] = store.Candidate{StartMS: sp.StartMS, EndMS: sp.EndMS, Title: p.Title, Hook: p.Hook, Payoff: p.Payoff, Why: p.Why,
			Strength: p.Strength, Text: pipeline.Text(segs, p.Start, p.End), Model: model}
	}
	r.done = fmt.Sprintf("%d candidates", len(cands))
	return r.s.store.ReplaceCandidates(context.Background(), r.job.ID, cands)
}

// segRange finds the segment indices a candidate covers.
func segRange(segs []pipeline.Segment, startMS, endMS int64) (int, int, bool) {
	a, b := -1, -1
	for i, s := range segs {
		if s.StartMS == startMS {
			a = i
		}
		if s.EndMS == endMS {
			b = i
		}
	}
	return a, b, a >= 0 && b >= a
}

func (r *run) reviewStep() error {
	model := r.cfg.ReviewerModel
	if model == "" {
		model = r.cfg.HunterModel
		r.logf("No reviewer model chosen; the hunter model reviews in a separate request")
	}
	if model == "" {
		return blockedError{"Choose a reviewer model in Settings → AI, then retry."}
	}
	segs, err := r.segments()
	if err != nil {
		return err
	}
	cands, err := r.s.store.ListCandidates(context.Background(), r.job.ID)
	if err != nil {
		return err
	}
	brief := r.brief()
	accepted := 0
	for i, c := range cands {
		r.progress(int64(i), int64(len(cands)), fmt.Sprintf("Candidate %d of %d", i+1, len(cands)))
		a, b, ok := segRange(segs, c.StartMS, c.EndMS)
		if !ok {
			return fmt.Errorf("candidate %d does not match the transcript", c.ID)
		}
		prompt := pipeline.ReviewPrompt(brief, segs, a, b)
		verdict, reason := "", ""
		for try := 0; try < 2 && verdict == ""; try++ {
			raw, err := ollamaGenerate(r.ctx, r.cfg.OllamaAddress, model, prompt)
			if err != nil {
				return fmt.Errorf("reviewer model: %w", err)
			}
			if verdict, reason, err = pipeline.ParseReview(raw); err != nil {
				r.logf("Candidate %d: reply not usable (%v)", i+1, err)
			}
		}
		if verdict == "" {
			verdict, reason = "reject", "The reviewer's reply could not be read."
		}
		if verdict == "accept" {
			accepted++
		}
		if err := r.s.store.SaveReview(context.Background(), c.ID, verdict, reason, model); err != nil {
			return err
		}
	}
	r.progress(int64(len(cands)), int64(len(cands)), "")
	r.logf("Reviewer accepted %d of %d", accepted, len(cands))
	if accepted == 0 {
		return errors.New("the reviewer turned down every candidate; see the Candidates list")
	}
	r.done = fmt.Sprintf("%d of %d accepted", accepted, len(cands))
	return nil
}

// ollamaGenerate asks a model for a JSON reply.
func ollamaGenerate(ctx context.Context, addr, model, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, modelTimeout)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"model": model, "prompt": prompt, "stream": false, "format": "json",
		"options": map[string]any{"num_ctx": modelContext, "temperature": 0.2}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ollamaClient.Do(req)
	if err != nil {
		return "", errors.New(serviceError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ollamaHTTPError(resp)
	}
	var out struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return "", errors.New("the reply was not the expected data")
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	return out.Response, nil
}

// ---- render and QA ----

// renderBytesPerMS estimates output size (about 6 Mbit/s) for the storage check.
const renderBytesPerMS = 6_000_000 / 8 / 1000

func (r *run) renderStep() error {
	src, err := r.source()
	if err != nil {
		return err
	}
	segs, err := r.segments()
	if err != nil {
		return err
	}
	cands, err := r.s.store.ListCandidates(context.Background(), r.job.ID)
	if err != nil {
		return err
	}
	var pick []store.Candidate
	for _, c := range cands {
		if c.Verdict == "accept" {
			pick = append(pick, c)
		}
	}
	sort.SliceStable(pick, func(a, b int) bool { return pick[a].Strength > pick[b].Strength })
	pick = pick[:min(len(pick), r.job.ClipCount)]
	sort.Slice(pick, func(a, b int) bool { return pick[a].StartMS < pick[b].StartMS })

	var need int64 = 1 << 30
	for _, c := range pick {
		need += (c.EndMS - c.StartMS) * renderBytesPerMS
	}
	rep := checkStorage(r.root, r.cfg.StorageLimitGB, true)
	switch {
	case !rep.OK():
		msg, _ := rep.summary()
		return blockedError{"Storage check failed: " + msg + " Fix it in Settings → Paths & Storage, then retry."}
	case rep.FreeKnown && rep.Free < uint64(need):
		return blockedError{fmt.Sprintf("Rendering needs about %s, but only %s is free on the disk. Free space, then retry.", humanBytes(need), humanBytes(int64(rep.Free)))}
	case rep.LimitGB > 0 && rep.UsedKnown && rep.Used+need > rep.limitBytes():
		return blockedError{fmt.Sprintf("Rendering needs about %s, which would pass the %d GB storage limit (%s used). Free space or raise the limit, then retry.",
			humanBytes(need), rep.LimitGB, humanBytes(rep.Used))}
	}
	if err := r.s.store.DeletePendingClips(context.Background(), r.job.ID); err != nil {
		return err
	}
	in := filepath.Join(r.root, src.Path)
	burned := 0
	for i, c := range pick {
		_ = r.s.store.SetJobDetail(context.Background(), r.job.ID, fmt.Sprintf("Clip %d of %d", i+1, len(pick)))
		name := fmt.Sprintf("clip-%d-%s", i+1, pipeline.SafeName(c.Title))
		srt, out := r.temp(name+".srt"), r.temp(name+".mp4")
		if err := os.MkdirAll(filepath.Dir(srt), folderMode); err != nil {
			return err
		}
		if err := os.WriteFile(srt, []byte(pipeline.SRT(segs, pipeline.Span{StartMS: c.StartMS, EndMS: c.EndMS})), 0o640); err != nil {
			return err
		}
		var res worker.RenderResult
		if err := r.op("render", worker.RenderArgs{In: in, Out: out, SRT: srt, StartMS: c.StartMS, EndMS: c.EndMS}, &res); err != nil {
			return fmt.Errorf("render clip %d: %w", i+1, err)
		}
		if res.Captions == "burned" {
			burned++
		}
		if _, err := r.s.store.AddClip(context.Background(), store.Clip{JobID: r.job.ID, CandidateID: c.ID, Path: r.rel(out), SRTPath: r.rel(srt), Captions: res.Captions}); err != nil {
			return err
		}
		r.logf("Rendered %s (%s, captions %s)", name, pipeline.Clock(c.EndMS-c.StartMS), res.Captions)
	}
	_ = r.s.store.SetJobDetail(context.Background(), r.job.ID, "")
	if burned < len(pick) {
		r.logf("No fonts were available to burn captions; the .srt file comes with each clip")
	}
	r.done = fmt.Sprintf("%d clips", len(pick))
	return nil
}

// qaFrameTolerance is how far a clip's length may differ from its span.
const qaToleranceMS = 500

func (r *run) qaStep() error {
	clips, err := r.s.store.ListClips(context.Background(), "", r.job.ID)
	if err != nil {
		return err
	}
	passed := 0
	for i, c := range clips {
		r.progress(int64(i), int64(len(clips)), fmt.Sprintf("Clip %d of %d", i+1, len(clips)))
		path := filepath.Join(r.root, c.Path)
		var p worker.ProbeResult
		var problems []string
		if err := r.op("probe", worker.ProbeArgs{Path: path}, &p); err != nil {
			problems = append(problems, "could not be read: "+err.Error())
		} else {
			want := c.EndMS - c.StartMS
			if p.Width != 1080 || p.Height != 1920 {
				problems = append(problems, fmt.Sprintf("size is %dx%d, not 1080x1920", p.Width, p.Height))
			}
			if d := p.DurationMS - want; d < -qaToleranceMS || d > qaToleranceMS {
				problems = append(problems, fmt.Sprintf("length is %s, expected %s", pipeline.Clock(p.DurationMS), pipeline.Clock(want)))
			}
			if !p.HasAudio {
				problems = append(problems, "no audio")
			}
		}
		c.Width, c.Height, c.DurationMS, c.Size = p.Width, p.Height, p.DurationMS, p.Size
		c.QAPass = len(problems) == 0
		c.QA = "Passed: 1080x1920, length matches, audio present."
		folder := FolderCompleted
		if !c.QAPass {
			c.QA = "Failed: " + strings.Join(problems, "; ") + "."
			folder = FolderFailed
		} else {
			passed++
		}
		// Move the clip and its captions out of temp/.
		dir := filepath.Join(folderPath(r.root, folder), fmt.Sprint(r.job.ID))
		if err := os.MkdirAll(dir, folderMode); err != nil {
			return err
		}
		for _, f := range []*string{&c.Path, &c.SRTPath} {
			if *f == "" || !strings.HasPrefix(*f, "temp"+string(filepath.Separator)) {
				continue
			}
			dst := filepath.Join(dir, filepath.Base(*f))
			if err := os.Rename(filepath.Join(r.root, *f), dst); err != nil {
				return err
			}
			*f = r.rel(dst)
		}
		if err := r.s.store.SetClipQA(context.Background(), c); err != nil {
			return err
		}
		r.logf("QA clip %d: %s", i+1, c.QA)
	}
	r.progress(int64(len(clips)), int64(len(clips)), "")
	_ = os.RemoveAll(filepath.Join(folderPath(r.root, FolderTemp), fmt.Sprint(r.job.ID)))
	if passed == 0 {
		return errors.New("no clip passed QA; the files are in failed/")
	}
	r.done = fmt.Sprintf("%d of %d passed", passed, len(clips))
	r.summary = fmt.Sprintf("%d clip(s) ready for review", passed)
	return nil
}
