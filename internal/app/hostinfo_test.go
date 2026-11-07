package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMemTotal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "meminfo")
	os.WriteFile(p, []byte("MemFree:  100 kB\nMemTotal:       16384000 kB\nMemAvailable: 5 kB\n"), 0o644)
	if got := parseMemTotalKB(p); got != 16384000 {
		t.Errorf("got %d", got)
	}
	os.WriteFile(p, []byte("MemTotal: lots kB\n"), 0o644)
	if parseMemTotalKB(p) != 0 || parseMemTotalKB(filepath.Join(dir, "none")) != 0 {
		t.Error("bad or missing file should give 0")
	}
	if got := (hostInfo{}).RAMText(); got != "not detected" {
		t.Errorf("RAMText %q", got)
	}
}

func TestGPUVendorMapping(t *testing.T) {
	for id, want := range map[string]string{"0x1002\n": "AMD", "0x10DE": "NVIDIA", "0x8086": "Intel", "0x1234": "unknown", "": "unknown"} {
		if got := gpuVendorName(id); got != want {
			t.Errorf("%q: got %q want %q", id, got, want)
		}
	}
	dir := t.TempDir()
	for i, id := range []string{"0x1002", "0x10de"} {
		d := filepath.Join(dir, "card"+string(rune('0'+i)), "device")
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "vendor"), []byte(id+"\n"), 0o644)
	}
	h := detectHost(filepath.Join(dir, "nomem"), filepath.Join(dir, "card*", "device", "vendor"))
	if h.GPUText() != "AMD, NVIDIA" || h.RAMText() != "not detected" {
		t.Errorf("host %+v", h)
	}
	if (hostInfo{}).GPUText() != "not detected" {
		t.Error("no GPU text")
	}
}

func tagsOf(cs []modelChoice) map[string]string {
	m := map[string]string{}
	for _, c := range cs {
		for _, tag := range []string{"Recommended", "Fits", "Needs more RAM"} {
			if strings.Contains(c.Label, " — "+tag) {
				m[c.Name] = tag
			}
		}
	}
	return m
}

func TestModelRecommendation(t *testing.T) {
	gb := int64(1024 * 1024)
	cases := []struct {
		ramKB int64
		want  map[string]string
		sel   string
	}{
		{8 * gb, map[string]string{"qwen2.5:3b": "Recommended", "qwen2.5:7b": "Needs more RAM", "qwen2.5:14b": "Needs more RAM"}, "qwen2.5:3b"},
		{12 * gb, map[string]string{"qwen2.5:3b": "Fits", "qwen2.5:7b": "Recommended", "qwen2.5:14b": "Needs more RAM"}, "qwen2.5:7b"},
		{20 * gb, map[string]string{"qwen2.5:3b": "Fits", "qwen2.5:7b": "Fits", "qwen2.5:14b": "Recommended"}, "qwen2.5:14b"},
		{27 * gb, map[string]string{"qwen2.5:3b": "Fits", "qwen2.5:7b": "Fits", "qwen2.5:14b": "Recommended"}, "qwen2.5:14b"},
		{4 * gb, map[string]string{"qwen2.5:3b": "Needs more RAM", "qwen2.5:7b": "Needs more RAM", "qwen2.5:14b": "Needs more RAM"}, "qwen2.5:3b"},
		{0, map[string]string{}, "qwen2.5:3b"},
	}
	for _, c := range cases {
		cs := modelChoices(c.ramKB, nil)
		if len(cs) != 3 {
			t.Fatalf("ram %d: %d options", c.ramKB, len(cs))
		}
		got := tagsOf(cs)
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("ram %d %s: tag %q want %q", c.ramKB, k, got[k], v)
			}
		}
		if len(got) != len(c.want) {
			t.Errorf("ram %d: tags %v want %v", c.ramKB, got, c.want)
		}
		for _, o := range cs {
			if o.Selected != (o.Name == c.sel) {
				t.Errorf("ram %d: %s selected=%v", c.ramKB, o.Name, o.Selected)
			}
		}
	}
	cs := modelChoices(12*gb, []string{"qwen2.5:7b"})
	if !strings.Contains(cs[1].Label, "Installed") || strings.Contains(cs[0].Label, "Installed") {
		t.Errorf("installed tag: %q / %q", cs[1].Label, cs[0].Label)
	}
}

func psStub(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return addrOf(srv)
}

func TestOllamaCompute(t *testing.T) {
	if got := ollamaCompute(psStub(t, `{"models":[{"name":"a","size_vram":0},{"name":"b","size_vram":900}]}`)); got != "GPU" {
		t.Errorf("gpu: %q", got)
	}
	if got := ollamaCompute(psStub(t, `{"models":[{"name":"a","size_vram":0}]}`)); got != "CPU" {
		t.Errorf("cpu: %q", got)
	}
	if got := ollamaCompute(psStub(t, `{"models":[]}`)); got != "not known until a model is loaded" {
		t.Errorf("none: %q", got)
	}
	if got := ollamaCompute("127.0.0.1:1"); got != "Ollama not reachable" {
		t.Errorf("down: %q", got)
	}
}

func TestAIPageShowsModelDropdownAndOtherField(t *testing.T) {
	s := securityTestServer(t)
	s.cfg.OllamaAddress = addrOf(ollamaStub(t, "0.9.1", []string{"qwen2.5:3b"}))
	body := get(t, s.handler(), "/settings/ai").Body.String()
	for _, want := range []string{`<select id="pull_model" name="pull_model"`, `value="qwen2.5:14b"`, "needs 12 GB", "Installed", `id="pull_other" name="pull_other"`, ">Download</button>", "Ollama runs on", "RAM</dt>", "GPU</dt>"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestPullFromDropdownAndOtherField(t *testing.T) {
	s := securityTestServer(t)
	defer s.stopBackground()
	srv, got := pullStub(t, []string{`{"status":"success"}`}, nil)
	s.cfg.OllamaAddress = addrOf(srv)
	post(t, s, "/settings/ai/pull", url.Values{"pull_model": {"qwen2.5:7b"}, "pull_other": {"  "}})
	if body := <-got; body["model"] != "qwen2.5:7b" {
		t.Errorf("dropdown: %v", body)
	}
	waitPull(t, s, false)
	post(t, s, "/settings/ai/pull", url.Values{"pull_model": {"qwen2.5:7b"}, "pull_other": {"llama3.2:1b"}})
	if body := <-got; body["model"] != "llama3.2:1b" {
		t.Errorf("other field should win: %v", body)
	}
}
