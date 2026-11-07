package app

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNormalizeServiceAddress(t *testing.T) {
	good := map[string]string{
		"127.0.0.1:11434":    "127.0.0.1:11434",
		" Whisper.LAN:10300": "whisper.lan:10300",
		"[::1]:10300":        "[::1]:10300",
		"localhost:1":        "localhost:1",
	}
	for in, want := range good {
		if got, err := normalizeServiceAddress(in); err != nil || got != want {
			t.Errorf("normalizeServiceAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "localhost", "127.0.0.1:", ":11434", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:abc",
		"http://127.0.0.1:11434", "host/path:80", "bad host:80", "a..b:80", "127.0.0.1:80/api", "-x:80"} {
		if got, err := normalizeServiceAddress(in); err == nil {
			t.Errorf("normalizeServiceAddress(%q) = %q, want an error", in, got)
		}
	}
}

func TestLoadFillsServiceDefaultsForOlderVersions(t *testing.T) {
	for _, v := range []string{"1", "2", "3"} {
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, []byte(`{"version":`+v+`,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747"}`), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := load(p)
		if err != nil {
			t.Fatal(err)
		}
		if c.Version != 4 || c.InstanceName != "Kept" || c.OllamaAddress != "127.0.0.1:11434" || c.WhisperAddress != "127.0.0.1:10300" ||
			c.MediaEngine != "podman" || c.FFmpegPath != "ffmpeg" || c.YtDlpPath != "yt-dlp" {
			t.Errorf("v%s migrated to %+v", v, c)
		}
	}
}

func TestLoadKeepsSavedServiceValuesAndRejectsBadOnes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"version":4,"instance_name":"x","ollama_address":"10.0.0.5:11434","whisper_address":"10.0.0.6:10300","media_engine":"host","ffmpeg_path":"/opt/ff","ytdlp_path":"/opt/yt"}`)
	c, err := load(p)
	if err != nil || c.OllamaAddress != "10.0.0.5:11434" || c.WhisperAddress != "10.0.0.6:10300" || c.MediaEngine != "host" || c.FFmpegPath != "/opt/ff" || c.YtDlpPath != "/opt/yt" {
		t.Fatalf("saved values changed: %+v, %v", c, err)
	}
	for _, bad := range []string{`"ollama_address":"nope"`, `"whisper_address":"x:99999"`, `"media_engine":"lxc"`} {
		write(`{"version":4,"instance_name":"x",` + bad + `}`)
		if _, err := load(p); err == nil {
			t.Errorf("load accepted %s", bad)
		}
	}
	// An installer-written file has only the service fields; the rest defaults.
	write(`{"version":4,"ollama_address":"127.0.0.1:11434"}`)
	if c, err := load(p); err != nil || c.InstanceName != "ReelAnvil" {
		t.Errorf("sparse config: %+v, %v", c, err)
	}
}

func post(t *testing.T, s *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return postForm(t, s.handler(), "127.0.0.1:8080", path, form.Encode(), "same-origin", "http://127.0.0.1:8080", "")
}

func TestSaveServiceSettings(t *testing.T) {
	s := securityTestServer(t)
	w := post(t, s, "/settings/ai", url.Values{"ollama_address": {" Gpu-Box.lan:11434 "}})
	if f := flashFrom(t, w.Result().Cookies()); f.Error || w.Header().Get("Location") != "/settings/ai" {
		t.Fatalf("save flash %+v, location %q", f, w.Header().Get("Location"))
	}
	w = post(t, s, "/settings/transcription", url.Values{"whisper_address": {"10.1.1.1:10300"}})
	if f := flashFrom(t, w.Result().Cookies()); f.Error {
		t.Fatalf("save flash %+v", f)
	}
	w = post(t, s, "/settings/media", url.Values{"media_engine": {"host"}, "ffmpeg_path": {"/usr/local/bin/ffmpeg"}, "ytdlp_path": {""}})
	if f := flashFrom(t, w.Result().Cookies()); f.Error {
		t.Fatalf("save flash %+v", f)
	}
	saved, err := load(s.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.OllamaAddress != "gpu-box.lan:11434" || saved.WhisperAddress != "10.1.1.1:10300" || saved.MediaEngine != "host" ||
		saved.FFmpegPath != "/usr/local/bin/ffmpeg" || saved.YtDlpPath != "yt-dlp" || saved.Version != 4 {
		t.Fatalf("saved %+v", saved)
	}
	page := get(t, s.handler(), "/settings/ai").Body.String()
	if !strings.Contains(page, `value="gpu-box.lan:11434"`) {
		t.Error("AI page does not show the saved address")
	}
	page = get(t, s.handler(), "/settings/media").Body.String()
	if !strings.Contains(page, `<option value="host" selected>`) || !strings.Contains(page, `value="/usr/local/bin/ffmpeg"`) {
		t.Error("media page does not show the saved engine and path")
	}
}

func TestSaveServiceSettingsRejectsBadInput(t *testing.T) {
	s := securityTestServer(t)
	before := s.cfg
	cases := []struct {
		path string
		form url.Values
		want string
	}{
		{"/settings/ai", url.Values{"ollama_address": {"http://x:1"}}, "host:port"},
		{"/settings/ai", url.Values{}, "host:port"},
		{"/settings/transcription", url.Values{"whisper_address": {"host:70000"}}, "1 to 65535"},
		{"/settings/media", url.Values{"media_engine": {"lxc"}}, "podman, docker or host"},
		{"/settings/media", url.Values{}, "choose a media engine"},
		{"/settings/media", url.Values{"media_engine": {"host"}, "ffmpeg_path": {"a\nb"}}, "one line"},
	}
	for _, tc := range cases {
		f := flashFrom(t, post(t, s, tc.path, tc.form).Result().Cookies())
		if !f.Error || !strings.Contains(f.Text, "not saved") || !strings.Contains(f.Text, tc.want) {
			t.Errorf("%s %v: flash %+v, want error containing %q", tc.path, tc.form, f, tc.want)
		}
	}
	if s.cfg.OllamaAddress != before.OllamaAddress || s.cfg.WhisperAddress != before.WhisperAddress || s.cfg.MediaEngine != before.MediaEngine {
		t.Fatalf("invalid forms changed settings: %+v", s.cfg)
	}
}

func TestServiceRoutesRejectCrossOriginAndWrongMethods(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	for _, path := range []string{"/settings/ai", "/settings/ai/test", "/settings/transcription", "/settings/transcription/test", "/settings/media", "/settings/media/test"} {
		w := postForm(t, h, "127.0.0.1:8080", path, "ollama_address=x:1", "same-origin", "http://attacker.example", "")
		if w.Code != http.StatusForbidden {
			t.Errorf("cross-origin POST %s = %d", path, w.Code)
		}
		r := httptest.NewRequest(http.MethodDelete, path, nil)
		r.Host = "127.0.0.1:8080"
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s = %d", path, w.Code)
		}
	}
	for _, path := range []string{"/settings/ai/test", "/settings/transcription/test", "/settings/media/test"} {
		if w := get(t, h, path); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d", path, w.Code)
		}
	}
}

func ollamaStub(t *testing.T, version string, models []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": version})
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		type m struct {
			Name string `json:"name"`
		}
		list := []m{}
		for _, n := range models {
			list = append(list, m{n})
		}
		json.NewEncoder(w).Encode(map[string]any{"models": list})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func addrOf(srv *httptest.Server) string { return strings.TrimPrefix(srv.URL, "http://") }

func TestTestOllama(t *testing.T) {
	srv := ollamaStub(t, "0.9.1", []string{"llama3.2:3b", "qwen2.5:7b"})
	msg, bad := testOllama(addrOf(srv))
	if bad || !strings.Contains(msg, "Ollama 0.9.1") || !strings.Contains(msg, "llama3.2:3b, qwen2.5:7b") {
		t.Errorf("got %q, bad=%v", msg, bad)
	}

	empty := ollamaStub(t, "0.9.1", nil)
	if msg, bad := testOllama(addrOf(empty)); bad || !strings.Contains(msg, "No models installed yet.") {
		t.Errorf("empty list: %q, bad=%v", msg, bad)
	}

	many := make([]string, 20)
	for i := range many {
		many[i] = "model" + string(rune('a'+i))
	}
	if msg, _ := testOllama(addrOf(ollamaStub(t, "1", many))); !strings.Contains(msg, "and 8 more") || len(msg) > 480 {
		t.Errorf("long list: %q", msg)
	}

	notOllama := httptest.NewServer(http.NotFoundHandler())
	defer notOllama.Close()
	if msg, bad := testOllama(addrOf(notOllama)); !bad || !strings.Contains(msg, "HTTP 404") {
		t.Errorf("not ollama: %q, bad=%v", msg, bad)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := l.Addr().String()
	l.Close()
	if msg, bad := testOllama(closed); !bad || !strings.Contains(msg, "Could not reach Ollama") {
		t.Errorf("closed port: %q, bad=%v", msg, bad)
	}
}

func TestTestOllamaTimesOut(t *testing.T) {
	old := serviceTimeout
	serviceTimeout = 100 * time.Millisecond
	t.Cleanup(func() { serviceTimeout = old })
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	start := time.Now()
	msg, bad := testOllama(addrOf(srv))
	if !bad || !strings.Contains(msg, "no answer within") || time.Since(start) > 2*time.Second {
		t.Errorf("got %q, bad=%v after %s", msg, bad, time.Since(start))
	}
}

func TestOllamaTestUsesTypedAddressWithoutSaving(t *testing.T) {
	s := securityTestServer(t)
	srv := ollamaStub(t, "0.9.1", []string{"mistral"})
	saved := s.cfg.OllamaAddress
	w := post(t, s, "/settings/ai/test", url.Values{"ollama_address": {addrOf(srv)}})
	f := flashFrom(t, w.Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "mistral") || w.Header().Get("Location") != "/settings/ai" {
		t.Fatalf("flash %+v, location %q", f, w.Header().Get("Location"))
	}
	if s.cfg.OllamaAddress != saved {
		t.Error("test changed the saved address")
	}
	if f := flashFrom(t, post(t, s, "/settings/ai/test", url.Values{"ollama_address": {"garbage"}}).Result().Cookies()); !f.Error || !strings.Contains(f.Text, "Test not run") {
		t.Errorf("garbage address flash %+v", f)
	}
	// Without a typed value the saved address is tested.
	s.cfg.OllamaAddress = addrOf(srv)
	if f := flashFrom(t, post(t, s, "/settings/ai/test", url.Values{}).Result().Cookies()); f.Error || !strings.Contains(f.Text, "0.9.1") {
		t.Errorf("saved address flash %+v", f)
	}
}

// wyomingStub accepts connections and calls handle with each one.
func wyomingStub(t *testing.T, handle func(c net.Conn, line string)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				line, _ := bufio.NewReader(c).ReadString('\n')
				handle(c, line)
			}()
		}
	}()
	return l.Addr().String()
}

func TestTestWhisper(t *testing.T) {
	var got string
	inline := wyomingStub(t, func(c net.Conn, line string) {
		got = line
		c.Write([]byte(`{"type":"info","data":{"asr":[{"name":"faster-whisper","models":[{"name":"tiny-int8"},{"name":"base"}]}]}}` + "\n"))
	})
	msg, bad := testWhisper(inline)
	if bad || !strings.Contains(msg, "tiny-int8, base") || strings.TrimSpace(got) != `{"type":"describe"}` {
		t.Errorf("inline info: %q, bad=%v, sent %q", msg, bad, got)
	}

	// Newer Wyoming versions send the data as bytes after the header line.
	body := `{"asr":[{"name":"w","models":[{"name":"small"}]}]}`
	separate := wyomingStub(t, func(c net.Conn, _ string) {
		c.Write([]byte(`{"type":"info","data_length":` + strconv.Itoa(len(body)) + `}` + "\n" + body))
	})
	if msg, bad := testWhisper(separate); bad || !strings.Contains(msg, "small") {
		t.Errorf("separate data: %q, bad=%v", msg, bad)
	}

	noModels := wyomingStub(t, func(c net.Conn, _ string) { c.Write([]byte(`{"type":"info","data":{"asr":[]}}` + "\n")) })
	if msg, bad := testWhisper(noModels); bad || !strings.Contains(msg, "no speech recognition models") {
		t.Errorf("no models: %q, bad=%v", msg, bad)
	}

	silent := wyomingStub(t, func(net.Conn, string) {})
	if msg, bad := testWhisper(silent); bad || !strings.Contains(msg, "Connected to") || !strings.Contains(msg, "sent no service info") {
		t.Errorf("closed without reply: %q, bad=%v", msg, bad)
	}

	notWyoming := wyomingStub(t, func(c net.Conn, _ string) { c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n")) })
	if msg, bad := testWhisper(notWyoming); bad || !strings.Contains(msg, "sent no service info") {
		t.Errorf("not wyoming: %q, bad=%v", msg, bad)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := l.Addr().String()
	l.Close()
	if msg, bad := testWhisper(closed); !bad || !strings.Contains(msg, "Could not connect") {
		t.Errorf("closed port: %q, bad=%v", msg, bad)
	}
}

func TestTestWhisperTimesOutWhenNoReply(t *testing.T) {
	old := serviceTimeout
	serviceTimeout = 150 * time.Millisecond
	t.Cleanup(func() { serviceTimeout = old })
	release := make(chan struct{})
	hang := wyomingStub(t, func(net.Conn, string) { <-release })
	defer close(release)
	start := time.Now()
	msg, bad := testWhisper(hang)
	if bad || !strings.Contains(msg, "Connected to") || time.Since(start) > 2*time.Second {
		t.Errorf("got %q, bad=%v after %s", msg, bad, time.Since(start))
	}
}

func TestWhisperTestHandler(t *testing.T) {
	s := securityTestServer(t)
	addr := wyomingStub(t, func(c net.Conn, _ string) {
		c.Write([]byte(`{"type":"info","data":{"asr":[{"models":[{"name":"tiny-int8"}]}]}}` + "\n"))
	})
	f := flashFrom(t, post(t, s, "/settings/transcription/test", url.Values{"whisper_address": {addr}}).Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "tiny-int8") {
		t.Errorf("flash %+v", f)
	}
}

func TestMediaTest(t *testing.T) {
	for _, engine := range []string{"podman", "docker"} {
		msg, bad := testMedia(Settings{MediaEngine: engine, FFmpegPath: "/nonexistent/ffmpeg", YtDlpPath: "/nonexistent/yt-dlp"})
		if bad || !strings.Contains(msg, "worker") || strings.Contains(msg, "not found") {
			t.Errorf("%s: %q, bad=%v", engine, msg, bad)
		}
	}

	dir := t.TempDir()
	tool := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\ntouch "+filepath.Join(dir, "was-run")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	msg, bad := testMedia(Settings{MediaEngine: "host", FFmpegPath: "ffmpeg", YtDlpPath: "yt-dlp"})
	if !bad || !strings.Contains(msg, "FFmpeg found at "+tool) || !strings.Contains(msg, "yt-dlp not found (yt-dlp)") {
		t.Errorf("one missing: %q, bad=%v", msg, bad)
	}
	other := filepath.Join(dir, "ytdlp-custom")
	if err := os.WriteFile(other, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg, bad = testMedia(Settings{MediaEngine: "host", FFmpegPath: tool, YtDlpPath: other})
	if bad || !strings.Contains(msg, "yt-dlp found at "+other) {
		t.Errorf("both found: %q, bad=%v", msg, bad)
	}
	if _, err := os.Stat(filepath.Join(dir, "was-run")); err == nil {
		t.Error("the test ran a program")
	}
}

func TestMediaTestHandlerUsesTypedValues(t *testing.T) {
	s := securityTestServer(t)
	f := flashFrom(t, post(t, s, "/settings/media/test", url.Values{"media_engine": {"host"}, "ffmpeg_path": {"/nonexistent/ffmpeg"}, "ytdlp_path": {"/nonexistent/yt"}}).Result().Cookies())
	if !f.Error || !strings.Contains(f.Text, "FFmpeg not found") {
		t.Errorf("flash %+v", f)
	}
	if s.cfg.MediaEngine != "podman" {
		t.Error("test changed the saved engine")
	}
	f = flashFrom(t, post(t, s, "/settings/media/test", url.Values{"media_engine": {"docker"}}).Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "worker") {
		t.Errorf("docker flash %+v", f)
	}
}

func TestServicePagesHighlightTheirOwnSidebarEntry(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	for path, title := range map[string]string{"/settings/ai": "AI", "/settings/transcription": "Transcription", "/settings/media": "Media"} {
		body := get(t, h, path).Body.String()
		active := `<a href="` + path + `" title="` + title + `" class="active" aria-current="page">`
		if !strings.Contains(body, active) {
			t.Errorf("%s: own sidebar entry not active", path)
		}
		if strings.Count(body, `aria-current="page"`) != 1 {
			t.Errorf("%s: more than one active sidebar entry", path)
		}
		if !strings.Contains(body, `data-group="settings"><button type="button" class="nav-group-toggle" aria-expanded="true"`) {
			t.Errorf("%s: Settings group is not expanded", path)
		}
		if !strings.Contains(body, `data-group="media"><button type="button" class="nav-group-toggle" aria-expanded="false"`) {
			t.Errorf("%s: Media group wrongly expanded", path)
		}
		if strings.Contains(body, "ETA") {
			t.Errorf("%s mentions an ETA", path)
		}
	}
}
