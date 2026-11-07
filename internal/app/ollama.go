package app

// Ollama model management for Settings > AI: model pulls that run in the
// background, and a short generation test. Everything goes over HTTP to the
// saved Ollama address; the web server never touches a container socket.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	// pullStall ends a pull when Ollama sends nothing for this long.
	pullStall = 5 * time.Minute
	// modelTestTimeout bounds the generation test; a CPU-only host may need
	// most of it to load the model the first time.
	modelTestTimeout = 60 * time.Second
)

const modelTestPrompt = "Reply with OK."

// pullState is the pull in progress or the last one that ran.
type pullState struct {
	Model, Status, Err string
	Running, Done      bool
	Completed, Total   int64
	Started, Ended     time.Time
}

// pullView is what the AI page shows. Percent is -1 when Ollama has not
// reported byte counts yet; it is derived only from Ollama's own numbers.
type pullView struct {
	Active, Running, Done bool
	Model, Status, Err    string
	Elapsed, Bytes        string
	Percent               int
}

func (s *Server) pullView() pullView {
	s.pullMu.Lock()
	p := s.pull
	s.pullMu.Unlock()
	v := pullView{Active: p.Model != "", Running: p.Running, Done: p.Done, Model: p.Model, Status: p.Status, Err: p.Err, Percent: -1}
	if !v.Active {
		return v
	}
	end := p.Ended
	if p.Running {
		end = time.Now()
	}
	v.Elapsed = end.Sub(p.Started).Truncate(time.Second).String()
	if p.Total > 0 {
		v.Bytes = humanBytes(p.Completed) + " of " + humanBytes(p.Total)
		v.Percent = int(p.Completed * 100 / p.Total)
	}
	return v
}

// background returns the context pulls run under; it ends in stopBackground.
func (s *Server) background() context.Context {
	s.bgOnce.Do(func() { s.bgCtx, s.bgStop = context.WithCancel(context.Background()) })
	return s.bgCtx
}

// stopBackground cancels running pulls and waits briefly for them to end.
func (s *Server) stopBackground() {
	s.background()
	s.bgStop()
	done := make(chan struct{})
	go func() { s.bgWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// startPull begins pulling model in the background. Only one pull runs at a time.
func (s *Server) startPull(addr, model string) error {
	ctx, cancel := context.WithCancel(s.background())
	s.pullMu.Lock()
	if s.pull.Running {
		running := s.pull.Model
		s.pullMu.Unlock()
		cancel()
		return fmt.Errorf("a pull of %s is already running", running)
	}
	s.pull = pullState{Model: model, Status: "starting", Running: true, Started: time.Now()}
	s.pullCancel = cancel
	s.pullMu.Unlock()
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		defer cancel()
		err := streamPull(ctx, addr, model, func(status string, completed, total int64) {
			s.pullMu.Lock()
			s.pull.Status = status
			if total > 0 {
				s.pull.Completed, s.pull.Total = completed, total
			}
			s.pullMu.Unlock()
		})
		s.pullMu.Lock()
		s.pull.Running, s.pull.Ended = false, time.Now()
		switch {
		case err == nil:
			s.pull.Done, s.pull.Status = true, "success"
		case errors.Is(ctx.Err(), context.Canceled):
			s.pull.Err = "stopped"
		default:
			s.pull.Err = clip(serviceError(err), 200)
		}
		s.pullMu.Unlock()
		if err != nil {
			s.log.Printf("model pull %s: %v", model, err)
		}
	}()
	return nil
}

// cancelPull stops the running pull, if any.
func (s *Server) cancelPull() bool {
	s.pullMu.Lock()
	defer s.pullMu.Unlock()
	if !s.pull.Running || s.pullCancel == nil {
		return false
	}
	s.pullCancel()
	return true
}

// streamPull asks Ollama to pull model and follows its progress stream.
// progress gets the latest status text and the byte counts summed over the
// layers Ollama has reported so far. A nil error means Ollama said "success".
func streamPull(ctx context.Context, addr, model string, progress func(status string, completed, total int64)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stall := time.AfterFunc(pullStall, cancel)
	defer stall.Stop()

	body, _ := json.Marshal(map[string]any{"model": model, "name": model, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ollamaClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ollamaHTTPError(resp)
	}
	type layer struct{ completed, total int64 }
	layers := map[string]layer{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), maxServiceBody)
	for sc.Scan() {
		stall.Reset(pullStall)
		var ev struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Error != "" {
			return errors.New(ev.Error)
		}
		if ev.Digest != "" && ev.Total > 0 {
			layers[ev.Digest] = layer{ev.Completed, ev.Total}
		}
		var done, total int64
		for _, l := range layers {
			done += l.completed
			total += l.total
		}
		progress(ev.Status, done, total)
		if ev.Status == "success" {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) && stall.Stop() {
			return err // cancelled by the caller
		}
		return fmt.Errorf("no data from Ollama for %s", pullStall)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

// ollamaHTTPError turns a non-200 reply into an error carrying Ollama's own message.
func ollamaHTTPError(resp *http.Response) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxServiceBody)).Decode(&e)
	if e.Error != "" {
		return errors.New(e.Error)
	}
	return fmt.Errorf("HTTP %d from Ollama", resp.StatusCode)
}

// testModel runs a short generation with model and reports the reply and how
// long it took.
func testModel(addr, model string, timeout time.Duration) (string, bool) {
	if model == "" {
		return "Choose a hunter model first, then run the model test.", true
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"model": model, "prompt": modelTestPrompt, "stream": false, "options": map[string]any{"num_predict": 16}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "Model test failed: " + err.Error() + ".", true
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := ollamaClient.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = ollamaHTTPError(resp)
		}
	}
	var out struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err == nil {
		if derr := json.NewDecoder(io.LimitReader(resp.Body, maxServiceBody)).Decode(&out); derr != nil {
			err = errors.New("the reply was not the expected data")
		} else if out.Error != "" {
			err = errors.New(out.Error)
		}
	}
	took := time.Since(start).Round(10 * time.Millisecond)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Sprintf("Model %s gave no answer within %s. It may still be loading; try again.", model, timeout), true
		}
		return fmt.Sprintf("Model %s failed after %s: %s.", model, took, clip(serviceError(err), 200)), true
	}
	reply := strings.Join(strings.Fields(out.Response), " ")
	if reply == "" {
		reply = "(empty reply)"
	}
	return fmt.Sprintf("Model %s answered in %s: %q", model, took, clip(reply, 120)), false
}

// ---- handlers ----

func (s *Server) aiPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	// The free-text "Other model" field wins only when it is not empty.
	name := r.FormValue("pull_other")
	if strings.TrimSpace(name) == "" {
		name = r.FormValue("pull_model")
	}
	model, err := normalizeModelName(name)
	if err == nil && model == "" {
		err = errors.New("enter a model name such as qwen2.5:3b")
	}
	if err == nil {
		s.mu.Lock()
		addr := s.cfg.OllamaAddress
		s.mu.Unlock()
		err = s.startPull(addr, model)
	}
	if err != nil {
		setFlash(w, "Pull not started: "+err.Error()+".", true)
	} else {
		setFlash(w, "Pulling "+model+" from Ollama. The status below updates while it runs.", false)
	}
	http.Redirect(w, r, "/settings/ai#pull-card", http.StatusSeeOther)
}

func (s *Server) aiPullCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cancelPull() {
		setFlash(w, "Stopping the model pull.", false)
	} else {
		setFlash(w, "No model pull is running.", true)
	}
	http.Redirect(w, r, "/settings/ai#pull-card", http.StatusSeeOther)
}

func (s *Server) aiModelTest(w http.ResponseWriter, r *http.Request) {
	// The test can outlast the server's default write timeout.
	if r.Method == http.MethodPost {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(modelTestTimeout + 10*time.Second))
	}
	s.serviceTest("/settings/ai", func(c Settings) (string, bool) {
		return testModel(c.OllamaAddress, c.HunterModel, modelTestTimeout)
	})(w, r)
}

// ---- AI page ----

type roleOption struct {
	Name              string
	Selected, Missing bool
}

// roleOptions lists the installed models for a role's select, plus the saved
// choice when it is not installed, so an unreachable Ollama never hides it.
func roleOptions(installed []string, saved string) []roleOption {
	var out []roleOption
	found := saved == ""
	for _, n := range installed {
		out = append(out, roleOption{Name: n, Selected: n == saved})
		if n == saved {
			found = true
		}
	}
	if !found {
		out = append(out, roleOption{Name: saved, Selected: true, Missing: true})
	}
	return out
}

type aiPageData struct {
	pageBase
	Config                         Settings
	Ollama                         ollamaStatus
	HunterOptions, ReviewerOptions []roleOption
	Pull                           pullView
	Host                           hostInfo
	Compute                        string
	ModelChoices                   []modelChoice
	ModelCount                     int
	RetryChoices                   []int
	ScoreChoices                   []int
}

func (s *Server) aiPage(w http.ResponseWriter, r *http.Request, sec pageSpec, section string) {
	data := aiPageData{pageBase: s.base(w, r, sec.title, section)}
	s.mu.Lock()
	data.Config = s.cfg
	s.mu.Unlock()
	data.Ollama = probeOllama(data.Config.OllamaAddress)
	data.ModelCount = len(data.Ollama.Models)
	data.HunterOptions = roleOptions(data.Ollama.Models, data.Config.HunterModel)
	data.ReviewerOptions = roleOptions(data.Ollama.Models, data.Config.ReviewerModel)
	data.Pull = s.pullView()
	data.Host = detectHost(procMeminfo, drmVendorGlb)
	data.Compute = ollamaCompute(data.Config.OllamaAddress)
	data.ModelChoices = modelChoices(data.Host.RAMKB, data.Ollama.Models)
	for n := 0; n <= MaxAutoRetries; n++ {
		data.RetryChoices = append(data.RetryChoices, n)
	}
	for n := MinMinReviewScore; n <= MaxMinReviewScore; n++ {
		data.ScoreChoices = append(data.ScoreChoices, n)
	}
	s.render(w, sec.page, data)
}
