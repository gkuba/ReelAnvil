package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gkuba/ReelAnvil/internal/pipeline"
	"github.com/gkuba/ReelAnvil/internal/store"
)

// fakeOllama answers /api/generate with whatever reply(prompt) returns and
// records every prompt.
type fakeOllama struct {
	mu      sync.Mutex
	prompts []string
}

func (f *fakeOllama) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

func newFakeOllama(t *testing.T, reply func(prompt string) string) (*fakeOllama, string) {
	t.Helper()
	f := &fakeOllama{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Prompt string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.prompts = append(f.prompts, body.Prompt)
		f.mu.Unlock()
		out, _ := json.Marshal(map[string]string{"response": reply(body.Prompt)})
		w.Write(out)
	}))
	t.Cleanup(srv.Close)
	return f, addrOf(srv)
}

func isFixPrompt(p string) bool { return strings.Contains(p, "You fix a short vertical video clip") }
func isReviewPrompt(p string) bool {
	return strings.Contains(p, "You review a short vertical video clip")
}

// lineSegments is n transcript lines of 8 seconds, "line 0." and so on.
func lineSegments(n int) []store.Segment {
	segs := make([]store.Segment, n)
	for i := range segs {
		segs[i] = store.Segment{StartMS: int64(i) * 8000, EndMS: int64(i+1) * 8000, Text: fmt.Sprintf("line %d.", i)}
	}
	return segs
}

// seedJob stores a job with a transcript and one candidate over lines a..b.
func seedJob(t *testing.T, s *Server, clipCount, a, b int) store.Job {
	t.Helper()
	ctx := context.Background()
	id, err := s.store.CreateJob(ctx, store.JobInput{Title: "talk", Origin: "file", Ref: "a.mp4", ClipCount: clipCount, MinS: 20, MaxS: 60})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.SaveTranscript(ctx, id, "whisper", "en", lineSegments(40)); err != nil {
		t.Fatal(err)
	}
	cand := store.Candidate{StartMS: int64(a) * 8000, EndMS: int64(b+1) * 8000, Title: "First pick", Strength: 7, Text: "x", Model: "hun"}
	if err = s.store.ReplaceCandidates(ctx, id, []store.Candidate{cand}); err != nil {
		t.Fatal(err)
	}
	j, err := s.store.JobByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func testRun(s *Server, j store.Job, retries int) *run {
	cfg := s.config()
	cfg.HunterModel, cfg.ReviewerModel, cfg.AutoRetries = "hun", "rev", retries
	return &run{s: s, ctx: context.Background(), job: j, cfg: cfg, root: filepath.Join(os.TempDir(), "unused")}
}

func logText(t *testing.T, s *Server, jobID int64) string {
	t.Helper()
	lines, err := s.store.JobLog(context.Background(), jobID, 200)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l.Message + "\n")
	}
	return sb.String()
}

// reviewReply is a reviewer reply with the complete score "complete" and every
// other score "other", over lines 10 to 13.
func reviewReply(verdict, reason string, other, complete int) string {
	return fmt.Sprintf(`{"scores":{"hook":%d,"complete":%d,"standalone":%d,"interest":%d,"fit":%d},"reasons":{"hook":"h","complete":"stops mid-thought","standalone":"s","interest":"i","fit":"f"},`+
		`"parts":[{"part":"hook","line":10},{"part":"setup","line":11},{"part":"payoff","line":13}],"verdict":%q,"reason":%q}`, other, complete, other, other, other, verdict, reason)
}

const fixReply = `{"clips":[{"start":12,"end":15,"title":"Better start","hook":"h","payoff":"p","why":"w","strength":8}]}`

func TestReviewStepRetriesRejectedCandidateUntilAccepted(t *testing.T) {
	s := securityTestServer(t)
	reviews := 0
	f, addr := newFakeOllama(t, func(p string) string {
		switch {
		case isFixPrompt(p):
			return fixReply
		case isReviewPrompt(p):
			reviews++
			if reviews == 1 {
				return reviewReply("reject", "starts mid-sentence", 8, 8)
			}
			return reviewReply("accept", "clean", 8, 8)
		}
		return "{}"
	})
	s.cfg.OllamaAddress = addr
	j := seedJob(t, s, 1, 10, 13)
	r := testRun(s, j, 2)
	if err := r.reviewStep(); err != nil {
		t.Fatal(err)
	}
	cands, err := s.store.ListCandidates(context.Background(), j.ID)
	if err != nil || len(cands) != 2 {
		t.Fatalf("candidates = %+v %v", cands, err)
	}
	if cands[0].Verdict != "reject" || cands[1].Verdict != "accept" || cands[1].Ord != 2 || cands[1].Attempt != 2 || cands[1].Title != "Better start" {
		t.Fatalf("candidates = %+v", cands)
	}
	if cands[1].StartMS != 12*8000 || cands[1].EndMS != 16*8000 {
		t.Fatalf("retry span = %d-%d", cands[1].StartMS, cands[1].EndMS)
	}
	prompts := f.seen()
	if len(prompts) != 3 || !isReviewPrompt(prompts[0]) || !isFixPrompt(prompts[1]) || !isReviewPrompt(prompts[2]) {
		t.Fatalf("prompt order: %d prompts", len(prompts))
	}
	if !strings.Contains(prompts[1], "starts mid-sentence") || !strings.Contains(prompts[1], "lines 10-13") {
		t.Errorf("fix prompt lacks the reviewer's reason or the original lines:\n%s", prompts[1])
	}
	if r.done != "1 of 2 accepted" {
		t.Errorf("done = %q", r.done)
	}
	if log := logText(t, s, j.ID); !strings.Contains(log, "Candidate 1 retry 1: starts mid-sentence → accepted") {
		t.Errorf("log:\n%s", log)
	}
}

func TestReviewStepGivesUpAfterTheConfiguredRetries(t *testing.T) {
	for _, tc := range []struct{ retries, wantFixes int }{{0, 0}, {1, 1}, {3, 3}} {
		s := securityTestServer(t)
		f, addr := newFakeOllama(t, func(p string) string {
			if isFixPrompt(p) {
				return fixReply
			}
			return reviewReply("reject", "dull", 8, 8)
		})
		s.cfg.OllamaAddress = addr
		j := seedJob(t, s, 1, 10, 13)
		err := testRun(s, j, tc.retries).reviewStep()
		if err == nil || !strings.Contains(err.Error(), "turned down every candidate") {
			t.Fatalf("retries %d: err = %v", tc.retries, err)
		}
		fixes := 0
		for _, p := range f.seen() {
			if isFixPrompt(p) {
				fixes++
			}
		}
		if fixes != tc.wantFixes {
			t.Errorf("retries %d: %d fix prompts, want %d", tc.retries, fixes, tc.wantFixes)
		}
		cands, _ := s.store.ListCandidates(context.Background(), j.ID)
		if len(cands) != 1+tc.wantFixes {
			t.Errorf("retries %d: %d candidates", tc.retries, len(cands))
		}
		if tc.retries > 0 && !strings.Contains(logText(t, s, j.ID), "retry 1: dull → rejected") {
			t.Errorf("retries %d: log\n%s", tc.retries, logText(t, s, j.ID))
		}
	}
}

func TestReviewStepDoesNotRetryWhenEnoughClipsWereAccepted(t *testing.T) {
	s := securityTestServer(t)
	f, addr := newFakeOllama(t, func(p string) string { return reviewReply("accept", "fine", 8, 8) })
	s.cfg.OllamaAddress = addr
	j := seedJob(t, s, 1, 10, 13)
	if err := testRun(s, j, 3).reviewStep(); err != nil {
		t.Fatal(err)
	}
	if n := len(f.seen()); n != 1 {
		t.Errorf("%d model calls, want 1", n)
	}
}

func TestFixWindowIsNinetySecondsEachSide(t *testing.T) {
	segs := make([]pipeline.Segment, 40)
	for i, sg := range lineSegments(40) {
		segs[i] = pipeline.Segment{StartMS: sg.StartMS, EndMS: sg.EndMS, Text: sg.Text}
	}
	from, to, a, b := fixWindow(segs, 20*8000, 24*8000)
	if a != 20 || b != 23 || from != 8 || to != 35 {
		t.Errorf("window = %d-%d around %d-%d", from, to, a, b)
	}
	from, to, a, b = fixWindow(segs, 0, 4*8000)
	if from != 0 || a != 0 || b != 3 || to != 15 {
		t.Errorf("window at the start = %d-%d around %d-%d", from, to, a, b)
	}
	if _, to, _, _ = fixWindow(segs, 36*8000, 40*8000); to != 39 {
		t.Errorf("window at the end stops at %d", to)
	}
}

// seedClip stores a job with a rendered clip that passed QA over lines 20..23.
func seedClip(t *testing.T, s *Server) (store.Job, int64) {
	t.Helper()
	ctx := context.Background()
	j := seedJob(t, s, 2, 20, 23)
	cands, _ := s.store.ListCandidates(ctx, j.ID)
	clip, err := s.store.AddClip(ctx, store.Clip{JobID: j.ID, CandidateID: cands[0].ID, Path: "p.mp4", SRTPath: "p.srt", Captions: "burned"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.SetClipQA(ctx, store.Clip{ID: clip, Path: "p.mp4", SRTPath: "p.srt", QAPass: true, Width: 1080, Height: 1920}); err != nil {
		t.Fatal(err)
	}
	return j, clip
}

func TestRetryHuntUsesFixPromptAroundTheRejectedClip(t *testing.T) {
	s := securityTestServer(t)
	f, addr := newFakeOllama(t, func(p string) string {
		if isFixPrompt(p) {
			return `{"clips":[{"start":21,"end":25,"title":"Fixed","strength":9}]}`
		}
		return "{}"
	})
	s.cfg.OllamaAddress = addr
	old, clip := seedClip(t, s)
	if err := s.store.DecideClip(context.Background(), clip, store.DecisionRejected, "ends too early", true); err != nil {
		t.Fatal(err)
	}
	id, err := s.store.CreateRedoJob(context.Background(), clip, "ends too early")
	if err != nil {
		t.Fatal(err)
	}
	j, _ := s.store.JobByID(context.Background(), id)
	r := testRun(s, j, 0)
	if err = r.huntStep(); err != nil {
		t.Fatal(err)
	}
	prompts := f.seen()
	if len(prompts) != 1 {
		t.Fatalf("%d prompts", len(prompts))
	}
	p := prompts[0]
	for _, want := range []string{"ends too early", "lines 20-23", "[8] ", "[35] ", "Problems the user asked to avoid:"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "[7] ") || strings.Contains(p, "[36] ") {
		t.Error("prompt goes beyond 90 seconds around the clip")
	}
	cands, _ := s.store.ListCandidates(context.Background(), id)
	if len(cands) != 1 || cands[0].Title != "Fixed" || cands[0].Attempt != 2 || cands[0].StartMS != 21*8000 {
		t.Fatalf("redo candidates = %+v", cands)
	}
	if old.Attempt != 1 {
		t.Errorf("original attempt = %d", old.Attempt)
	}
}

func decide(t *testing.T, s *Server, form url.Values) (flash, string) {
	t.Helper()
	w := post(t, s, "/review/decide", form)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("decide = %d", w.Code)
	}
	return flashFrom(t, w.Result().Cookies()), w.Header().Get("Location")
}

func TestRejectWithRetryNeedsANoteAndQueuesARedoJob(t *testing.T) {
	s := securityTestServer(t)
	old, clip := seedClip(t, s)
	id := fmt.Sprint(clip)
	f, loc := decide(t, s, url.Values{"id": {id}, "decision": {"rejected"}, "retry": {"1"}, "note": {"  "}, "view": {"pending"}})
	if !f.Error || f.Text != "Add a note saying what to fix." || loc != "/review?view=pending" {
		t.Fatalf("flash %+v, location %s", f, loc)
	}
	if c, _ := s.store.ClipByID(context.Background(), clip); c.Decision != store.DecisionPending {
		t.Fatalf("a refused retry changed the decision to %s", c.Decision)
	}
	if jobs, _ := s.store.ListJobs(context.Background(), 10); len(jobs) != 1 {
		t.Fatalf("%d jobs after a refused retry", len(jobs))
	}

	f, _ = decide(t, s, url.Values{"id": {id}, "decision": {"rejected"}, "retry": {"1"}, "note": {"start later"}})
	if f.Error || f.Text != "Retry queued as job 2." {
		t.Fatalf("flash %+v", f)
	}
	redo, err := s.store.JobByID(context.Background(), 2)
	if err != nil || redo.RedoClipID != clip || redo.RedoNote != "start later" || redo.Attempt != old.Attempt+1 || redo.State != store.JobQueued {
		t.Fatalf("redo job %+v %v", redo, err)
	}
	if c, _ := s.store.ClipByID(context.Background(), clip); c.Decision != store.DecisionRejected {
		t.Fatalf("decision %s", c.Decision)
	}
	if ls, _ := s.store.ListLessons(context.Background(), 5); len(ls) != 0 {
		t.Fatalf("lessons without feedback: %+v", ls)
	}

	// Retry is ignored unless the decision is Reject.
	_, other := seedClip(t, s)
	f, _ = decide(t, s, url.Values{"id": {fmt.Sprint(other)}, "decision": {"approved"}, "retry": {"1"}, "note": {"fine"}})
	if f.Error || strings.Contains(f.Text, "Retry") {
		t.Fatalf("approve with retry: %+v", f)
	}
	if jobs, _ := s.store.ListJobs(context.Background(), 10); len(jobs) != 3 {
		t.Fatalf("%d jobs, want the two seeded and one redo", len(jobs))
	}
}

func TestFeedbackSwitchNeedsANoteOnRejectOnly(t *testing.T) {
	s := securityTestServer(t)
	_, clip := seedClip(t, s)
	id := fmt.Sprint(clip)
	f, _ := decide(t, s, url.Values{"id": {id}, "decision": {"rejected"}, "feedback": {"1"}})
	if !f.Error || f.Text != "Add a note saying what to fix." {
		t.Fatalf("flash %+v", f)
	}
	f, _ = decide(t, s, url.Values{"id": {id}, "decision": {"approved"}, "feedback": {"1"}})
	if f.Error {
		t.Fatalf("flash %+v", f)
	}
	if ls, _ := s.store.ListLessons(context.Background(), 5); len(ls) != 1 || ls[0].Kind != "good" {
		t.Fatalf("lessons %+v", ls)
	}
}

func TestReviewPageShowsSwitchesAttemptAndLessonsTab(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	_, clip := seedClip(t, s)
	page := get(t, h, "/review").Body.String()
	for _, want := range []string{`type="checkbox" role="switch" name="feedback"`, `type="checkbox" role="switch" name="retry"`,
		"Teach the model: approved clips become good examples, rejected ones become things to fix.", "Reject and try again using your note.",
		`href="/review?view=lessons"`, "Lessons (0)"} {
		if !strings.Contains(page, want) {
			t.Errorf("review page lacks %q", want)
		}
	}
	if strings.Contains(page, "style=") || strings.Contains(page, "<script>") || strings.Contains(page, "Attempt") {
		t.Error("inline style or script, or an attempt on a first-attempt clip")
	}
	css := readAsset(t, "web/static/app.css")
	for _, want := range []string{".switch input:checked+.track", "var(--accent)"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}

	// A clip from a retry shows its attempt.
	decide(t, s, url.Values{"id": {fmt.Sprint(clip)}, "decision": {"rejected"}, "retry": {"1"}, "note": {"try again"}})
	ctx := context.Background()
	if err := s.store.ReplaceCandidates(ctx, 2, []store.Candidate{{StartMS: 0, EndMS: 30000, Title: "Second try", Strength: 5, Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	cands, _ := s.store.ListCandidates(ctx, 2)
	again, err := s.store.AddClip(ctx, store.Clip{JobID: 2, CandidateID: cands[0].ID, Path: "q.mp4", SRTPath: "q.srt", Captions: "burned"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.SetClipQA(ctx, store.Clip{ID: again, Path: "q.mp4", SRTPath: "q.srt", QAPass: true}); err != nil {
		t.Fatal(err)
	}
	if page = get(t, h, "/review").Body.String(); !strings.Contains(page, "Attempt 2") || !strings.Contains(page, "Second try") {
		t.Error("retry clip does not show Attempt 2")
	}
}

func TestLessonsTabListsAndDeletesLessons(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	_, clip := seedClip(t, s)
	decide(t, s, url.Values{"id": {fmt.Sprint(clip)}, "decision": {"rejected"}, "feedback": {"1"}, "note": {"cut the intro"}})
	page := get(t, h, "/review?view=lessons").Body.String()
	for _, want := range []string{"First pick", "cut the intro", "To fix", "Lessons (1)", `action="/review/lesson-delete"`, "Delete"} {
		if !strings.Contains(page, want) {
			t.Errorf("lessons tab lacks %q", want)
		}
	}
	if strings.Contains(page, "clip-video") || strings.Contains(page, "style=") {
		t.Error("lessons tab shows clips or inline style")
	}
	ls, _ := s.store.ListLessons(context.Background(), 5)
	w := post(t, s, "/review/lesson-delete", url.Values{"id": {fmt.Sprint(ls[0].ID)}, "view": {"lessons"}})
	if w.Header().Get("Location") != "/review?view=lessons" {
		t.Fatalf("location %s", w.Header().Get("Location"))
	}
	if f := flashFrom(t, w.Result().Cookies()); f.Error || f.Text != "Lesson deleted." {
		t.Fatalf("flash %+v", f)
	}
	if ls, _ = s.store.ListLessons(context.Background(), 5); len(ls) != 0 {
		t.Fatalf("lessons left: %+v", ls)
	}
	if page = get(t, h, "/review?view=lessons").Body.String(); !strings.Contains(page, "No lessons yet") {
		t.Error("empty lessons tab has no message")
	}
	// The clip itself is untouched.
	if c, _ := s.store.ClipByID(context.Background(), clip); c.Decision != store.DecisionRejected {
		t.Fatalf("decision %s", c.Decision)
	}
	if w = post(t, s, "/review/lesson-delete", url.Values{"id": {"999"}}); !flashFrom(t, w.Result().Cookies()).Error {
		t.Error("deleting a missing lesson did not report an error")
	}
}

func TestAutoRetriesSetting(t *testing.T) {
	// Settings saved before the field existed get the default; an explicit 0 stays.
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"version":5,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := load(p); err != nil || c.AutoRetries != 2 {
		t.Fatalf("old settings: %+v %v", c, err)
	}
	if err := os.WriteFile(p, []byte(`{"version":5,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747","auto_retries":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := load(p); err != nil || c.AutoRetries != 0 {
		t.Fatalf("explicit zero: %+v %v", c, err)
	}
	if err := os.WriteFile(p, []byte(`{"version":5,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747","auto_retries":4}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(p); err == nil {
		t.Fatal("auto_retries 4 accepted")
	}

	s := securityTestServer(t)
	if !strings.Contains(get(t, s.handler(), "/settings/ai").Body.String(), `<option value="2" selected>2</option>`) {
		t.Error("AI settings do not show the default of 2")
	}
	f := flashFrom(t, post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "auto_retries": {"3"}}).Result().Cookies())
	if f.Error || s.cfg.AutoRetries != 3 {
		t.Fatalf("save: %+v, %d", f, s.cfg.AutoRetries)
	}
	if saved, err := load(s.cfgPath); err != nil || saved.AutoRetries != 3 {
		t.Fatalf("saved %+v %v", saved, err)
	}
	// A form without the field leaves it alone; a bad value is refused.
	post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}})
	if s.cfg.AutoRetries != 3 {
		t.Errorf("auto retries changed to %d", s.cfg.AutoRetries)
	}
	for _, bad := range []string{"4", "-1", "x", ""} {
		f = flashFrom(t, post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "auto_retries": {bad}}).Result().Cookies())
		if !f.Error || s.cfg.AutoRetries != 3 {
			t.Errorf("auto_retries %q: %+v, now %d", bad, f, s.cfg.AutoRetries)
		}
	}
}

func TestReviewStepRejectsAcceptWithOneLowScoreAndRetriesWithThatReason(t *testing.T) {
	s := securityTestServer(t)
	reviews := 0
	f, addr := newFakeOllama(t, func(p string) string {
		switch {
		case isFixPrompt(p):
			return fixReply
		case isReviewPrompt(p):
			reviews++
			if reviews == 1 {
				// The model says accept, but one score is below the default of 6.
				return reviewReply("accept", "looks fine", 8, 4)
			}
			return reviewReply("accept", "clean", 8, 8)
		}
		return "{}"
	})
	s.cfg.OllamaAddress = addr
	j := seedJob(t, s, 1, 10, 13)
	r := testRun(s, j, 2)
	if r.cfg.MinReviewScore != 6 {
		t.Fatalf("default lowest score = %d", r.cfg.MinReviewScore)
	}
	if err := r.reviewStep(); err != nil {
		t.Fatal(err)
	}
	cands, _ := s.store.ListCandidates(context.Background(), j.ID)
	if len(cands) != 2 || cands[0].Verdict != "reject" || cands[0].Reason != "Complete 4/10: stops mid-thought" || cands[1].Verdict != "accept" {
		t.Fatalf("candidates = %+v", cands)
	}
	if !strings.Contains(cands[0].Scores, `"complete":4`) || cands[0].Parts != `[{"part":"hook","ms":0},{"part":"setup","ms":8000},{"part":"payoff","ms":24000}]` {
		t.Errorf("stored scores %q parts %q", cands[0].Scores, cands[0].Parts)
	}
	prompts := f.seen()
	if len(prompts) != 3 || !isFixPrompt(prompts[1]) || !strings.Contains(prompts[1], "Complete 4/10: stops mid-thought") {
		t.Fatalf("fix prompt lacks the lowest criterion as its note (%d prompts)", len(prompts))
	}
	if log := logText(t, s, j.ID); !strings.Contains(log, "Candidate 1: hook 8, complete 4, standalone 8, interest 8, fit 8 → reject") {
		t.Errorf("log:\n%s", log)
	}
}

func TestReviewStepUnreadableScoresStayARejectAfterTwoTries(t *testing.T) {
	s := securityTestServer(t)
	f, addr := newFakeOllama(t, func(p string) string { return `{"verdict":"accept","reason":"no scores"}` })
	s.cfg.OllamaAddress = addr
	j := seedJob(t, s, 1, 10, 13)
	if err := testRun(s, j, 0).reviewStep(); err == nil {
		t.Fatal("every candidate was rejected but the step succeeded")
	}
	if n := len(f.seen()); n != 2 {
		t.Errorf("%d model calls, want 2", n)
	}
	cands, _ := s.store.ListCandidates(context.Background(), j.ID)
	if cands[0].Verdict != "reject" || cands[0].Scores != "" {
		t.Errorf("candidate = %+v", cands[0])
	}
}

func TestMinReviewScoreSetting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"version":5,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := load(p); err != nil || c.MinReviewScore != 6 {
		t.Fatalf("old settings: %+v %v", c, err)
	}
	if err := os.WriteFile(p, []byte(`{"version":5,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747","min_review_score":11}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(p); err == nil {
		t.Fatal("min_review_score 11 accepted")
	}
	s := securityTestServer(t)
	if !strings.Contains(get(t, s.handler(), "/settings/ai").Body.String(), `<option value="6" selected>6</option>`) {
		t.Error("AI settings do not show the default of 6")
	}
	f := flashFrom(t, post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "min_review_score": {"8"}}).Result().Cookies())
	if f.Error || s.cfg.MinReviewScore != 8 {
		t.Fatalf("save: %+v, %d", f, s.cfg.MinReviewScore)
	}
	if saved, err := load(s.cfgPath); err != nil || saved.MinReviewScore != 8 {
		t.Fatalf("saved %+v %v", saved, err)
	}
	post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}})
	if s.cfg.MinReviewScore != 8 {
		t.Errorf("lowest score changed to %d", s.cfg.MinReviewScore)
	}
	for _, bad := range []string{"0", "11", "-1", "x", ""} {
		f = flashFrom(t, post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "min_review_score": {bad}}).Result().Cookies())
		if !f.Error || s.cfg.MinReviewScore != 8 {
			t.Errorf("min_review_score %q: %+v, now %d", bad, f, s.cfg.MinReviewScore)
		}
	}
}

func TestReviewPageShowsScoreChipsAndParts(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	j, _ := seedClip(t, s)
	if page := get(t, h, "/review").Body.String(); strings.Contains(page, `class="chips"`) || strings.Contains(page, `class="parts"`) {
		t.Fatal("a clip without stored scores shows chips or parts")
	}
	cands, _ := s.store.ListCandidates(context.Background(), j.ID)
	scores := `{"scores":{"hook":8,"complete":4,"standalone":7,"interest":6,"fit":9},"reasons":{"hook":"bold question","complete":"cuts <off>","standalone":"","interest":"niche","fit":"on brief"}}`
	if err := s.store.SaveReview(context.Background(), cands[0].ID, "reject", "Complete 4/10", "rev", scores, `[{"part":"hook","ms":0},{"part":"setup","ms":7000},{"part":"payoff","ms":24000}]`); err != nil {
		t.Fatal(err)
	}
	page := get(t, h, "/review").Body.String()
	for _, want := range []string{`<li class="chip">Hook 8</li>`, `<li class="chip">Complete 4</li>`, `<li class="chip">Fit 9</li>`, "bold question", "cuts &lt;off&gt;",
		"0:00 Hook · 0:07 Setup · 0:24 Payoff"} {
		if !strings.Contains(page, want) {
			t.Errorf("review page lacks %q", want)
		}
	}
	if strings.Contains(page, "style=") || strings.Contains(page, "<script>") {
		t.Error("inline style or script")
	}
	if css := readAsset(t, "web/static/app.css"); !strings.Contains(css, ".chip{") || !strings.Contains(css, "var(--line)") {
		t.Error("app.css lacks the chip style")
	}
}
