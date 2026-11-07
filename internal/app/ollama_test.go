package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// pullStub serves /api/pull. It writes each line and flushes; when hold is
// not nil it waits there after the first line until hold closes or the client
// goes away.
func pullStub(t *testing.T, lines []string, hold chan struct{}) (*httptest.Server, chan map[string]any) {
	t.Helper()
	got := make(chan map[string]any, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		got <- body
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i, l := range lines {
			w.Write([]byte(l + "\n"))
			w.(http.Flusher).Flush()
			if i == 0 && hold != nil {
				select {
				case <-hold:
				case <-r.Context().Done():
					return
				}
			}
		}
		if hold != nil && len(lines) == 1 {
			<-r.Context().Done()
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, got
}

func waitPull(t *testing.T, s *Server, running bool) pullView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if v := s.pullView(); v.Running == running && v.Active {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pull never reached running=%v: %+v", running, s.pullView())
	return pullView{}
}

func TestPullStreamsRealProgressAndFinishes(t *testing.T) {
	s := securityTestServer(t)
	hold := make(chan struct{})
	srv, got := pullStub(t, []string{
		`{"status":"pulling manifest"}`,
		`{"status":"pulling aaa","digest":"sha256:aaa","total":1000,"completed":250}`,
		`{"status":"pulling bbb","digest":"sha256:bbb","total":500,"completed":500}`,
		`{"status":"pulling aaa","digest":"sha256:aaa","total":1000,"completed":1000}`,
		`{"status":"verifying sha256 digest"}`,
		`{"status":"success"}`,
	}, hold)
	if err := s.startPull(addrOf(srv), "qwen2.5:3b"); err != nil {
		t.Fatal(err)
	}
	if body := <-got; body["model"] != "qwen2.5:3b" || body["stream"] != true {
		t.Errorf("request %v", body)
	}
	if v := waitPull(t, s, true); v.Model != "qwen2.5:3b" {
		t.Errorf("running view %+v", v)
	}
	// While running, a second pull is refused and the page refreshes itself.
	if err := s.startPull(addrOf(srv), "other"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("second pull: %v", err)
	}
	page := get(t, s.handler(), "/settings/ai").Body.String()
	if !strings.Contains(page, `id="hunter_model" name="hunter_model" data-refresh`) || !strings.Contains(page, `id="reviewer_model" name="reviewer_model" data-refresh`) || !strings.Contains(page, `data-poll=`) || strings.Contains(page, `http-equiv="refresh"`) || !strings.Contains(page, "Pulling qwen2.5:3b") || strings.Contains(page, "Stop pull") == false {
		t.Errorf("running page lacks refresh, title or stop button")
	}
	close(hold)
	v := waitPull(t, s, false)
	if !v.Done || v.Err != "" || v.Status != "success" || v.Bytes != "1.5 KB of 1.5 KB" || v.Percent != 100 {
		t.Errorf("finished view %+v", v)
	}
	s.pullMu.Lock()
	completed, total := s.pull.Completed, s.pull.Total
	s.pullMu.Unlock()
	if completed != 1500 || total != 1500 {
		t.Errorf("summed bytes %d of %d, want 1500 of 1500", completed, total)
	}
	page = get(t, s.handler(), "/settings/ai").Body.String()
	if strings.Contains(page, `data-poll=`) || strings.Contains(page, `http-equiv="refresh"`) || !strings.Contains(page, "Pulled qwen2.5:3b") || !strings.Contains(page, "finished") || !strings.Contains(page, "Elapsed") {
		t.Errorf("finished page wrong")
	}
	for _, banned := range []string{"ETA", "remaining", "left"} {
		if strings.Contains(page, banned) {
			t.Errorf("page mentions %q", banned)
		}
	}
	// A finished pull does not block the next one.
	if err := s.startPull(addrOf(srv), "again"); err != nil {
		t.Errorf("pull after finish: %v", err)
	}
	s.stopBackground()
}

func TestPullReportsOllamaErrors(t *testing.T) {
	s := securityTestServer(t)
	srv, _ := pullStub(t, []string{`{"status":"pulling manifest"}`, `{"error":"pull model manifest: file does not exist"}`}, nil)
	if err := s.startPull(addrOf(srv), "nope:1b"); err != nil {
		t.Fatal(err)
	}
	if v := waitPull(t, s, false); v.Done || !strings.Contains(v.Err, "file does not exist") {
		t.Errorf("view %+v", v)
	}

	http404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer http404.Close()
	if err := s.startPull(addrOf(http404), "x"); err != nil {
		t.Fatal(err)
	}
	if v := waitPull(t, s, false); !strings.Contains(v.Err, "model not found") {
		t.Errorf("404 view %+v", v)
	}

	cut, _ := pullStub(t, []string{`{"status":"pulling manifest"}`}, nil)
	if err := s.startPull(addrOf(cut), "x"); err != nil {
		t.Fatal(err)
	}
	if v := waitPull(t, s, false); v.Done || v.Err == "" {
		t.Errorf("stream cut short should fail: %+v", v)
	}
}

func TestPullStopsOnShutdownAndOnCancel(t *testing.T) {
	s := securityTestServer(t)
	srv, got := pullStub(t, []string{`{"status":"pulling manifest"}`}, make(chan struct{}))
	if err := s.startPull(addrOf(srv), "big:70b"); err != nil {
		t.Fatal(err)
	}
	<-got
	waitPull(t, s, true)
	start := time.Now()
	s.stopBackground()
	if v := waitPull(t, s, false); v.Err != "stopped" || v.Done {
		t.Errorf("after shutdown %+v", v)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("shutdown waited too long for the pull")
	}

	s2 := securityTestServer(t)
	srv2, got2 := pullStub(t, []string{`{"status":"pulling manifest"}`}, make(chan struct{}))
	defer s2.stopBackground()
	if err := s2.startPull(addrOf(srv2), "x"); err != nil {
		t.Fatal(err)
	}
	<-got2
	waitPull(t, s2, true)
	f := flashFrom(t, post(t, s2, "/settings/ai/pull/cancel", url.Values{}).Result().Cookies())
	if f.Error {
		t.Errorf("cancel flash %+v", f)
	}
	if v := waitPull(t, s2, false); v.Err != "stopped" {
		t.Errorf("after cancel %+v", v)
	}
	if f := flashFrom(t, post(t, s2, "/settings/ai/pull/cancel", url.Values{}).Result().Cookies()); !f.Error {
		t.Errorf("cancel with nothing running: %+v", f)
	}
}

func TestPullGivesUpWhenOllamaGoesQuiet(t *testing.T) {
	old := pullStall
	pullStall = 150 * time.Millisecond
	t.Cleanup(func() { pullStall = old })
	s := securityTestServer(t)
	srv, _ := pullStub(t, []string{`{"status":"pulling manifest"}`}, make(chan struct{}))
	if err := s.startPull(addrOf(srv), "x"); err != nil {
		t.Fatal(err)
	}
	if v := waitPull(t, s, false); !strings.Contains(v.Err, "no data from Ollama") {
		t.Errorf("view %+v", v)
	}
}

func TestPullHandlerValidatesAndStarts(t *testing.T) {
	s := securityTestServer(t)
	defer s.stopBackground()
	for _, name := range []string{"", "bad name", "-x", "a;b", strings.Repeat("a", 200)} {
		f := flashFrom(t, post(t, s, "/settings/ai/pull", url.Values{"pull_model": {name}}).Result().Cookies())
		if !f.Error || !strings.Contains(f.Text, "Pull not started") {
			t.Errorf("%q: flash %+v", name, f)
		}
	}
	if s.pullView().Active {
		t.Error("a bad name started a pull")
	}
	srv, got := pullStub(t, []string{`{"status":"success"}`}, nil)
	s.cfg.OllamaAddress = addrOf(srv)
	w := post(t, s, "/settings/ai/pull", url.Values{"pull_model": {" llama3.2:1b "}})
	if f := flashFrom(t, w.Result().Cookies()); f.Error || w.Header().Get("Location") != "/settings/ai#pull-card" {
		t.Errorf("flash %+v location %q", f, w.Header().Get("Location"))
	}
	if body := <-got; body["model"] != "llama3.2:1b" {
		t.Errorf("request %v", body)
	}
	if v := waitPull(t, s, false); !v.Done {
		t.Errorf("view %+v", v)
	}
}

func generateStub(t *testing.T, status int, reply string, delay time.Duration) (*httptest.Server, chan map[string]any) {
	t.Helper()
	got := make(chan map[string]any, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		got <- body
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestTestModel(t *testing.T) {
	srv, got := generateStub(t, 200, `{"model":"m","response":"  OK\n","done":true}`, 20*time.Millisecond)
	msg, bad := testModel(addrOf(srv), "qwen2.5:3b", 5*time.Second)
	if bad || !strings.Contains(msg, "qwen2.5:3b answered in") || !strings.Contains(msg, `"OK"`) {
		t.Errorf("got %q bad=%v", msg, bad)
	}
	body := <-got
	opts, _ := body["options"].(map[string]any)
	if body["model"] != "qwen2.5:3b" || body["prompt"] != "Reply with OK." || body["stream"] != false || opts["num_predict"] == nil {
		t.Errorf("request %v", body)
	}

	if msg, bad := testModel(addrOf(srv), "", time.Second); !bad || !strings.Contains(msg, "Choose a hunter model") {
		t.Errorf("no model: %q bad=%v", msg, bad)
	}
	missing, _ := generateStub(t, 404, `{"error":"model 'zzz' not found"}`, 0)
	if msg, bad := testModel(addrOf(missing), "zzz", time.Second); !bad || !strings.Contains(msg, "model 'zzz' not found") {
		t.Errorf("missing model: %q bad=%v", msg, bad)
	}
	garbage, _ := generateStub(t, 200, `<html>`, 0)
	if msg, bad := testModel(addrOf(garbage), "m", time.Second); !bad || !strings.Contains(msg, "not the expected data") {
		t.Errorf("garbage: %q bad=%v", msg, bad)
	}
	slow, _ := generateStub(t, 200, `{"response":"OK"}`, 5*time.Second)
	start := time.Now()
	if msg, bad := testModel(addrOf(slow), "m", 150*time.Millisecond); !bad || !strings.Contains(msg, "no answer within 150ms") || time.Since(start) > 2*time.Second {
		t.Errorf("timeout: %q bad=%v after %s", msg, bad, time.Since(start))
	}
}

func TestModelTestHandlerUsesTypedHunterModel(t *testing.T) {
	s := securityTestServer(t)
	srv, got := generateStub(t, 200, `{"response":"OK"}`, 0)
	s.cfg.OllamaAddress = addrOf(srv)
	s.cfg.HunterModel = "saved:1b"
	f := flashFrom(t, post(t, s, "/settings/ai/model-test", url.Values{"ollama_address": {addrOf(srv)}, "hunter_model": {"typed:3b"}}).Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "typed:3b answered") {
		t.Errorf("flash %+v", f)
	}
	if body := <-got; body["model"] != "typed:3b" {
		t.Errorf("request %v", body)
	}
	if s.cfg.HunterModel != "saved:1b" {
		t.Error("test changed the saved model")
	}
	s.cfg.HunterModel = ""
	if f := flashFrom(t, post(t, s, "/settings/ai/model-test", url.Values{}).Result().Cookies()); !f.Error || !strings.Contains(f.Text, "Choose a hunter model") {
		t.Errorf("no model flash %+v", f)
	}
}

func TestAIPageListsInstalledModelsForEachRole(t *testing.T) {
	s := securityTestServer(t)
	srv := ollamaStub(t, "0.9.1", []string{"llama3.2:3b", "qwen2.5:3b"})
	s.cfg.OllamaAddress = addrOf(srv)
	s.cfg.HunterModel = "qwen2.5:3b"
	s.cfg.ReviewerModel = "gone:7b"
	body := get(t, s.handler(), "/settings/ai").Body.String()
	for _, want := range []string{
		`id="hunter_model"`, `id="reviewer_model"`, `<option value="">None</option>`,
		`<option value="qwen2.5:3b" selected>qwen2.5:3b</option>`, `<option value="llama3.2:3b">llama3.2:3b</option>`,
		`<option value="gone:7b" selected>gone:7b (not installed)</option>`,
		"Connected", "Ollama 0.9.1", "2 model(s) installed", `formaction="/settings/ai/model-test"`, `action="/settings/ai/pull"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("AI page lacks %q", want)
		}
	}
	if strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("idle page refreshes itself")
	}
}

func TestAIPageWhenOllamaIsDown(t *testing.T) {
	s := securityTestServer(t)
	s.cfg.OllamaAddress = "127.0.0.1:1"
	s.cfg.HunterModel = "qwen2.5:3b"
	w := get(t, s.handler(), "/settings/ai")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Not reachable") || !strings.Contains(body, `<option value="qwen2.5:3b" selected>qwen2.5:3b (not installed)</option>`) {
		t.Errorf("down page: %d\n%s", w.Code, body)
	}
}

func TestSaveModelRoles(t *testing.T) {
	s := securityTestServer(t)
	f := flashFrom(t, post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "hunter_model": {" qwen2.5:3b "}, "reviewer_model": {"llama3.2:3b"}}).Result().Cookies())
	if f.Error {
		t.Fatalf("flash %+v", f)
	}
	saved, err := load(s.cfgPath)
	if err != nil || saved.HunterModel != "qwen2.5:3b" || saved.ReviewerModel != "llama3.2:3b" {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	// Choosing None clears a role; a form without role fields leaves them alone.
	post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "hunter_model": {""}})
	if s.cfg.HunterModel != "" || s.cfg.ReviewerModel != "llama3.2:3b" {
		t.Errorf("roles %q / %q", s.cfg.HunterModel, s.cfg.ReviewerModel)
	}
	post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}})
	if s.cfg.ReviewerModel != "llama3.2:3b" {
		t.Error("address-only save cleared a role")
	}
	f = flashFrom(t, post(t, s, "/settings/ai", url.Values{"ollama_address": {"127.0.0.1:11434"}, "hunter_model": {"bad name"}}).Result().Cookies())
	if !f.Error || !strings.Contains(f.Text, "not saved") || s.cfg.HunterModel != "" {
		t.Errorf("bad name flash %+v, hunter %q", f, s.cfg.HunterModel)
	}
}
