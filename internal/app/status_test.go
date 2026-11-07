package app

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func whisperUp(t *testing.T) string {
	t.Helper()
	return wyomingStub(t, func(c net.Conn, _ string) {
		c.Write([]byte(`{"type":"info","data":{"asr":[{"name":"faster-whisper","models":[{"name":"tiny-int8"},{"name":"base"}]}]}}` + "\n"))
	})
}

func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// readyServer returns a server whose every service answers and whose media
// root is a ready folder.
func readyServer(t *testing.T) *Server {
	t.Helper()
	s := securityTestServer(t)
	s.dataDir = t.TempDir()
	root := filepath.Join(t.TempDir(), "media")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureMediaFolders(root); err != nil {
		t.Fatal(err)
	}
	s.cfg.MediaRoot = root
	s.cfg.OllamaAddress = addrOf(ollamaStub(t, "0.9.1", []string{"qwen2.5:3b", "llama3.2:3b"}))
	s.cfg.WhisperAddress = whisperUp(t)
	s.cfg.HunterModel, s.cfg.ReviewerModel = "qwen2.5:3b", "llama3.2:3b"
	return s
}

func TestServicesPageReady(t *testing.T) {
	s := readyServer(t)
	w := get(t, s.handler(), "/services")
	body := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	for _, want := range []string{
		"Ready. Every check that runs from the web app passed.", "status-ok",
		"<h2>Web app</h2>", "<h2>Ollama</h2>", "<h2>Whisper</h2>", "<h2>Storage</h2>",
		Version, "Uptime", "Installed models", "qwen2.5:3b, llama3.2:3b", "Models reported", "tiny-int8, base",
		"Check again", `href="/services"`, "tiny-int8, en",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("services page lacks %q", want)
		}
	}
	if strings.Contains(body, "Needs action") || strings.Contains(body, "status-warn") {
		t.Error("ready page shows a problem")
	}
	// A container engine can't be checked from the web app, so it gets no card.
	if strings.Contains(body, "<h2>Media engine</h2>") {
		t.Error("services page shows a media engine card for a container engine")
	}
	if strings.Contains(body, "ETA") {
		t.Error("services page mentions an ETA")
	}
}

func TestServicesPageNeedsActionWithCommands(t *testing.T) {
	s := readyServer(t)
	s.cfg.OllamaAddress = closedAddr(t)
	s.cfg.WhisperAddress = closedAddr(t)
	s.cfg.MediaEngine = EngineHost
	s.cfg.FFmpegPath, s.cfg.YtDlpPath = "/nonexistent/ffmpeg", "/nonexistent/yt-dlp"
	s.cfg.MediaRoot = filepath.Join(t.TempDir(), "absent")
	body := get(t, s.handler(), "/services").Body.String()
	for _, want := range []string{
		"Needs action: 4 item(s) below.", "Not reachable", "Programs missing", "Root missing",
		"<code>sudo reelanvil doctor --repair</code>", `href="/settings/ai"`, `href="/settings/transcription"`, `href="/settings/media"`, `href="/settings/storage"`,
		"FFmpeg and yt-dlp not found", "status-warn",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(body, "Ready. Every check") {
		t.Error("shows Ready despite problems")
	}
}

func TestServicesPageOllamaStates(t *testing.T) {
	cfg := Settings{OllamaAddress: "x:1"}
	empty := ollamaStatus{Reachable: true, Version: "1"}
	card, actions := ollamaCard(cfg, empty)
	if card.Status != "No models installed" || len(actions) != 1 {
		t.Errorf("no models: %+v %+v", card, actions)
	}
	withModels := ollamaStatus{Reachable: true, Version: "1", Models: []string{"a", "b"}}
	if card, _ := ollamaCard(cfg, withModels); card.Status != "Model roles not set" {
		t.Errorf("roles unset: %+v", card)
	}
	cfg.HunterModel, cfg.ReviewerModel = "a", "zzz"
	card, actions = ollamaCard(cfg, withModels)
	if card.Status != "Model not installed" || len(actions) != 1 || !strings.Contains(actions[0].Text, "zzz") {
		t.Errorf("missing reviewer: %+v %+v", card, actions)
	}
	cfg.ReviewerModel = "b"
	if card, actions := ollamaCard(cfg, withModels); card.Status != "Ready" || card.Level != "ok" || len(actions) != 0 {
		t.Errorf("ready: %+v %+v", card, actions)
	}
	down := ollamaStatus{Err: "connection refused"}
	if card, actions := ollamaCard(cfg, down); card.Status != "Not reachable" || len(actions) != 1 || actions[0].Command != "sudo reelanvil doctor --repair" {
		t.Errorf("down: %+v %+v", card, actions)
	}
	if card, _ := ollamaCard(cfg, ollamaStatus{Reachable: true, TagsErr: "boom"}); card.Status != "Model list unreadable" {
		t.Errorf("tags error: %+v", card)
	}
}

func TestServicesPageStorageStates(t *testing.T) {
	root := t.TempDir()
	card, actions := storageCard(root, checkStorage(root, 0, true))
	if card.Status != "Folders missing" || len(actions) != 1 {
		t.Errorf("folders missing: %+v %+v", card, actions)
	}
	ensureMediaFolders(root)
	if card, actions := storageCard(root, checkStorage(root, 0, true)); card.Status != "Ready" || len(actions) != 0 {
		t.Errorf("ready: %+v %+v", card, actions)
	}
	over := storageReport{Root: root, Exists: true, IsDir: true, Writable: true, LimitGB: 1, Used: 2 << 30, UsedKnown: true}
	if card, actions := storageCard(root, over); card.Status != "Over limit" || len(actions) != 1 {
		t.Errorf("over limit: %+v %+v", card, actions)
	}
	file := filepath.Join(root, "f")
	os.WriteFile(file, nil, 0600)
	if card, _ := storageCard(file, checkStorage(file, 0, true)); card.Status != "Root is not a folder" {
		t.Errorf("file root: %+v", card)
	}
}

func TestServicesMediaCardHostProgramsAreLocatedNotRun(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"ffmpeg", "yt-dlp"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\ntouch "+filepath.Join(dir, "was-run")+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	card, actions := mediaCard(Settings{MediaEngine: EngineHost, FFmpegPath: "ffmpeg", YtDlpPath: "yt-dlp"})
	if card.Status != "Ready" || len(actions) != 0 {
		t.Errorf("both found: %+v %+v", card, actions)
	}
	if _, err := os.Stat(filepath.Join(dir, "was-run")); err == nil {
		t.Error("a program was run")
	}
}

func TestServicesPageStaysQuickWhenServicesHang(t *testing.T) {
	old := serviceTimeout
	serviceTimeout = 300 * time.Millisecond
	t.Cleanup(func() { serviceTimeout = old })
	s := readyServer(t)
	release := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer hang.Close()
	defer close(release)
	s.cfg.OllamaAddress = addrOf(hang)
	s.cfg.WhisperAddress = wyomingStub(t, func(net.Conn, string) { <-release })
	start := time.Now()
	body := get(t, s.handler(), "/services").Body.String()
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("page took %s with hanging services", took)
	}
	if !strings.Contains(body, "Not reachable") || !strings.Contains(body, "Needs action") {
		t.Error("hanging Ollama not reported")
	}
}

func TestTranscriptionPageShowsReportedModelsAndFields(t *testing.T) {
	s := securityTestServer(t)
	s.cfg.WhisperAddress = whisperUp(t)
	s.cfg.WhisperModel, s.cfg.WhisperLanguage = "base", "de"
	body := get(t, s.handler(), "/settings/transcription").Body.String()
	for _, want := range []string{
		`name="whisper_model"`, `value="base"`, `name="whisper_language"`, `value="de"`,
		`<option value="tiny-int8">`, `<option value="base">`, "reports 2 speech recognition model(s)",
		"sudo reelanvil upgrade", "never controls containers",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("transcription page lacks %q", want)
		}
	}
	s.cfg.WhisperAddress = closedAddr(t)
	body = get(t, s.handler(), "/settings/transcription").Body.String()
	if !strings.Contains(body, "Not reachable") || !strings.Contains(body, `value="base"`) {
		t.Error("page with Whisper down is wrong")
	}
}

func TestSaveWhisperModelAndLanguage(t *testing.T) {
	s := securityTestServer(t)
	f := flashFrom(t, post(t, s, "/settings/transcription", url.Values{"whisper_address": {"127.0.0.1:10300"}, "whisper_model": {" large-v3 "}, "whisper_language": {"auto"}}).Result().Cookies())
	if f.Error {
		t.Fatalf("flash %+v", f)
	}
	saved, err := load(s.cfgPath)
	if err != nil || saved.WhisperModel != "large-v3" || saved.WhisperLanguage != "auto" {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	// Blank means the default; a form without the fields leaves them alone.
	post(t, s, "/settings/transcription", url.Values{"whisper_address": {"127.0.0.1:10300"}, "whisper_model": {""}, "whisper_language": {"fr"}})
	if s.cfg.WhisperModel != "tiny-int8" || s.cfg.WhisperLanguage != "fr" {
		t.Errorf("blank/default: %q %q", s.cfg.WhisperModel, s.cfg.WhisperLanguage)
	}
	post(t, s, "/settings/transcription", url.Values{"whisper_address": {"127.0.0.1:10300"}})
	if s.cfg.WhisperLanguage != "fr" {
		t.Error("address-only save reset the language")
	}
	for _, bad := range []url.Values{
		{"whisper_address": {"127.0.0.1:10300"}, "whisper_model": {"tiny int8"}},
		{"whisper_address": {"127.0.0.1:10300"}, "whisper_language": {"en\nPUID: 0"}},
		{"whisper_address": {"127.0.0.1:10300"}, "whisper_model": {"a/b"}},
		{"whisper_address": {"127.0.0.1:10300"}, "whisper_model": {strings.Repeat("a", 65)}},
	} {
		f := flashFrom(t, post(t, s, "/settings/transcription", bad).Result().Cookies())
		if !f.Error || !strings.Contains(f.Text, "not saved") {
			t.Errorf("%v: flash %+v", bad, f)
		}
	}
	if s.cfg.WhisperModel != "tiny-int8" || s.cfg.WhisperLanguage != "fr" {
		t.Errorf("rejected forms changed settings: %q %q", s.cfg.WhisperModel, s.cfg.WhisperLanguage)
	}
}

func TestConnectorsPageIsHonest(t *testing.T) {
	s := securityTestServer(t)
	body := get(t, s.handler(), "/settings/connectors").Body.String()
	for _, want := range []string{"Manual campaigns", "Available", `href="/campaigns"`, "Platform discovery", "Disabled", "public API"} {
		if !strings.Contains(body, want) {
			t.Errorf("connectors page lacks %q", want)
		}
	}
	for _, banned := range []string{"<input", "Not yet configurable", "API credential", "Service endpoint"} {
		if strings.Contains(body, banned) {
			t.Errorf("connectors page still has %q", banned)
		}
	}
}

func TestSettingsAndServicesPagesHighlightOwnSidebarEntry(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	for path, title := range map[string]string{
		"/settings/storage": "Paths &amp; Storage", "/settings/connectors": "Connectors", "/services": "Services",
		"/settings/ai": "AI", "/settings/transcription": "Transcription", "/settings/media": "Media", "/settings": "General", "/settings/themes": "Themes",
	} {
		body := get(t, h, path).Body.String()
		if !strings.Contains(body, `<a href="`+path+`" title="`+title+`" class="active" aria-current="page">`) {
			t.Errorf("%s: own sidebar entry not active", path)
		}
		if strings.Count(body, `aria-current="page"`) != 1 {
			t.Errorf("%s: %d active entries", path, strings.Count(body, `aria-current="page"`))
		}
		if !strings.Contains(body, `data-group="settings"><button type="button" class="nav-group-toggle" aria-expanded="true"`) {
			t.Errorf("%s: Settings group not expanded", path)
		}
		if !strings.Contains(body, `data-group="media"><button type="button" class="nav-group-toggle" aria-expanded="false"`) {
			t.Errorf("%s: Media group wrongly expanded", path)
		}
	}
	for path, title := range map[string]string{"/media": "Media Library", "/review": "Review", "/jobs": "Jobs", "/diagnostics": "Diagnostics"} {
		body := get(t, h, path).Body.String()
		if !strings.Contains(body, `<a href="`+path+`" title="`+title+`" class="active" aria-current="page">`) || strings.Count(body, `aria-current="page"`) != 1 {
			t.Errorf("%s: sidebar highlight wrong", path)
		}
		if strings.Contains(body, `data-group="settings"><button type="button" class="nav-group-toggle" aria-expanded="true"`) {
			t.Errorf("%s: Settings group wrongly expanded", path)
		}
	}
}

func TestNewRoutesRejectCrossOriginAndWrongMethods(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	posts := []string{"/settings/storage", "/settings/storage/check", "/settings/ai/pull", "/settings/ai/pull/cancel", "/settings/ai/model-test"}
	for _, path := range posts {
		before := s.cfg
		w := postForm(t, h, "127.0.0.1:8080", path, "media_root=/srv/x/y&pull_model=x", "same-origin", "http://attacker.example", "")
		if w.Code != http.StatusForbidden {
			t.Errorf("cross-origin POST %s = %d", path, w.Code)
		}
		if before.MediaRoot != s.cfg.MediaRoot || s.pullView().Active {
			t.Errorf("%s: rejected request had an effect", path)
		}
		r := httptest.NewRequest(http.MethodDelete, path, nil)
		r.Host = "127.0.0.1:8080"
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s = %d", path, w.Code)
		}
	}
	for _, path := range []string{"/settings/storage/check", "/settings/ai/pull", "/settings/ai/pull/cancel", "/settings/ai/model-test"} {
		if w := get(t, h, path); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d", path, w.Code)
		}
	}
	if w := post(t, s, "/services", url.Values{}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /services = %d", w.Code)
	}
}
