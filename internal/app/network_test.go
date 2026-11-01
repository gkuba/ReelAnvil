package app

import (
	"encoding/base64"
	"encoding/json"
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

func TestParseAllowedHosts(t *testing.T) {
	got, err := parseAllowedHosts("ReelAnvil.lan, media-box\nreelanvil.lan.")
	if err != nil || strings.Join(got, ",") != "reelanvil.lan,media-box" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"192.168.1.5", "[::1]", "bad_name", "-x.lan", "a/b", "x:8080"} {
		if _, err := parseAllowedHosts(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := parseAllowedHosts(strings.Repeat("h ", 1) + strings.Repeat("x", 254)); err == nil {
		t.Error("over-long name accepted")
	}
}

func TestListenAddress(t *testing.T) {
	for _, tc := range []struct{ bind, port, want string }{
		{"0.0.0.0", "8080", "0.0.0.0:8080"},
		{"", "9000", "0.0.0.0:9000"},
		{"::", "8080", "[::]:8080"},
		{"[::1]", "8080", "[::1]:8080"},
		{"localhost", "18089", "localhost:18089"},
	} {
		if got, err := listenAddress(tc.bind, tc.port); err != nil || got != tc.want {
			t.Errorf("listenAddress(%q, %q) = %q, %v; want %q", tc.bind, tc.port, got, err, tc.want)
		}
	}
	for _, tc := range [][2]string{{"example.com", "8080"}, {"0.0.0.0", "0"}, {"0.0.0.0", "65536"}, {"0.0.0.0", "http"}} {
		if _, err := listenAddress(tc[0], tc[1]); err == nil {
			t.Errorf("listenAddress(%q, %q) accepted", tc[0], tc[1])
		}
	}
}

func TestLoadNetworkSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"version":3,"instance_name":"X","theme":"aurora"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil || c.ListenAddress != DefaultAddr || len(c.AllowedHosts) != 0 {
		t.Fatalf("older settings: %+v, %v", c, err)
	}
	for _, bad := range []string{`{"version":3,"listen_address":"example.com:80"}`, `{"version":3,"allowed_hosts":["bad host!"]}`} {
		if err := os.WriteFile(p, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := load(p); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestRestartURL(t *testing.T) {
	for _, tc := range []struct{ host, target, want string }{
		{"192.168.1.20:8080", "0.0.0.0:9000", "http://192.168.1.20:9000/settings"},
		{"media-box:8080", "[::]:8080", "http://media-box:8080/settings"},
		{"media-box:8080", "192.168.1.20:8080", "http://192.168.1.20:8080/settings"},
		{"127.0.0.1:8080", "127.0.0.1:8081", "http://127.0.0.1:8081/settings"},
		{"[::1]:8080", "127.0.0.1:8081", "http://[::1]:8081/settings"},
		{"localhost:8080", "localhost:8081", "http://localhost:8081/settings"},
	} {
		if got := restartURL(tc.host, tc.target); got != tc.want {
			t.Errorf("restartURL(%q, %q) = %q, want %q", tc.host, tc.target, got, tc.want)
		}
	}
}

func flashFrom(t *testing.T, cookies []*http.Cookie) flash {
	t.Helper()
	for _, c := range cookies {
		if c.Name == flashCookie {
			b, _ := base64.RawURLEncoding.DecodeString(c.Value)
			var f flash
			if err := json.Unmarshal(b, &f); err != nil {
				t.Fatal(err)
			}
			return f
		}
	}
	t.Fatal("no flash cookie")
	return flash{}
}

func TestSaveNetworkSettings(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	if strings.Contains(get(t, h, "/settings").Body.String(), "restart-note") {
		t.Fatal("restart banner shown before any change")
	}
	form := url.Values{"bind_address": {"0.0.0.0"}, "port": {"9000"}, "allowed_hosts": {"ReelAnvil.lan\nmedia-box"}}
	w := postForm(t, h, "127.0.0.1:8080", "/settings/network", form.Encode(), "same-origin", "http://127.0.0.1:8080", "")
	if f := flashFrom(t, w.Result().Cookies()); f.Error || !strings.Contains(f.Text, "Restart ReelAnvil") {
		t.Fatalf("flash = %+v", f)
	}
	saved, err := load(s.cfgPath)
	if err != nil || saved.ListenAddress != "0.0.0.0:9000" || strings.Join(saved.AllowedHosts, ",") != "reelanvil.lan,media-box" {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	page := get(t, h, "/settings").Body.String()
	for _, want := range []string{"restart-note", `value="9000"`, "reelanvil.lan\nmedia-box</textarea>", `action="/system/restart"`} {
		if !strings.Contains(page, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
	if !strings.Contains(get(t, h, "/campaigns").Body.String(), "restart-note") {
		t.Error("restart banner missing on other pages")
	}

	w = postForm(t, h, "127.0.0.1:8080", "/settings/network", "bind_address=example.com&port=9000", "same-origin", "http://127.0.0.1:8080", "")
	if f := flashFrom(t, w.Result().Cookies()); !f.Error || !strings.Contains(f.Text, "IP address") {
		t.Fatalf("invalid bind flash = %+v", f)
	}
	if again, _ := load(s.cfgPath); again.ListenAddress != "0.0.0.0:9000" {
		t.Fatalf("invalid form changed settings: %+v", again)
	}
	w = postForm(t, h, "127.0.0.1:8080", "/settings/network", "bind_address=0.0.0.0&port=9000", "same-origin", "http://attacker.example", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin save status = %d", w.Code)
	}
}

func TestSaveLoopbackFromRemoteWarns(t *testing.T) {
	s := securityTestServer(t)
	s.hosts = newHostPolicy(true, nil)
	r := httptestPost("192.168.1.20:8080", "/settings/network", "bind_address=127.0.0.1&port=8080")
	r.RemoteAddr = "192.168.1.50:51000"
	w := serve(s.handler(), r)
	if f := flashFrom(t, w.Result().Cookies()); !strings.Contains(f.Text, "only be reachable from its own machine") {
		t.Fatalf("flash = %+v", f)
	}
}

func TestRestartRefusesUnbindableAddress(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	s := securityTestServer(t)
	s.restart = make(chan struct{})
	s.cfg.ListenAddress = busy.Addr().String()
	w := postForm(t, s.handler(), "127.0.0.1:8080", "/system/restart", "", "same-origin", "http://127.0.0.1:8080", "")
	if f := flashFrom(t, w.Result().Cookies()); !f.Error || !strings.Contains(f.Text, "was not restarted") {
		t.Fatalf("flash = %+v", f)
	}
	select {
	case <-s.restart:
		t.Fatal("restart requested despite bind failure")
	default:
	}
}

func TestRestartRendersReconnectPage(t *testing.T) {
	s := securityTestServer(t)
	s.restart = make(chan struct{})
	h := s.handler()
	if w := get(t, h, "/system/restart"); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET restart status = %d", w.Code)
	}
	w := postForm(t, h, "127.0.0.1:8080", "/system/restart", "", "same-origin", "http://127.0.0.1:8080", "")
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `<meta http-equiv="refresh" content="3;url=http://127.0.0.1:8080/settings">`) || strings.Contains(body, "restart-note") {
		t.Fatalf("status=%d body=%s", w.Code, body)
	}
	select {
	case <-s.restart:
	case <-time.After(restartDelay + 2*time.Second):
		t.Fatal("restart not requested")
	}
	postForm(t, h, "127.0.0.1:8080", "/system/restart", "", "same-origin", "http://127.0.0.1:8080", "") // second request must not panic
}

func httptestPost(host, path, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "http://"+host+path, strings.NewReader(body))
	r.Host = host
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Origin", "http://"+host)
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
