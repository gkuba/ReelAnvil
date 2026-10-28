package app

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	original := Settings{Version: 1, InstanceName: "Test", Theme: "aurora"}
	if err := save(p, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != original {
		t.Fatalf("got %+v, want %+v", loaded, original)
	}
}

func TestLoopbackAddressRequired(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "[::1]:8080", "localhost:8080"} {
		if err := loopbackAddress(address); err != nil {
			t.Errorf("loopback address %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8080", ":8080", "192.0.2.1:8080"} {
		if err := loopbackAddress(address); err == nil {
			t.Errorf("non-loopback address %q accepted", address)
		}
	}
}

func TestLocalRequestRejectsForeignHostAndOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := localRequest(next)
	for _, tc := range []struct {
		name, host, origin string
		method             string
		want               int
	}{
		{"same-origin post", "127.0.0.1:8080", "http://127.0.0.1:8080", http.MethodPost, http.StatusNoContent},
		{"foreign-origin post", "127.0.0.1:8080", "http://attacker.example", http.MethodPost, http.StatusForbidden},
		{"foreign host", "attacker.example", "", http.MethodGet, http.StatusMisdirectedRequest},
		{"originless local post", "localhost:8080", "", http.MethodPost, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://"+tc.host+"/", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
			if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("security headers missing")
			}
		})
	}
	t.Run("fetch metadata rejects cross-site POST", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/", nil)
		r.Host = "127.0.0.1:8080"
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
		}
	})
}

func TestLocalRequestCapsPostBody(t *testing.T) {
	handler := localRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	r.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}
func TestDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.json")
	c, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "aurora" {
		t.Fatal(c)
	}
}
func TestUnsupportedVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(p); err == nil {
		t.Fatal("expected version error")
	}
}

func TestThemePaletteAndLegacyMigration(t *testing.T) {
	for _, theme := range []string{"aurora", "dracula-dark", "tomorrow-night", "nocturne", "foundry"} {
		if !validTheme(theme) {
			t.Fatalf("missing theme %s", theme)
		}
		if theme != "aurora" && !strings.Contains(pageTemplate.Tree.Root.String(), "body."+theme+"{") {
			t.Fatalf("missing palette %s", theme)
		}
	}
	for old, want := range map[string]string{"dark": "tomorrow-night", "slate": "aurora", "pastel": "nocturne", "dracula-lavender": "dracula-dark"} {
		p := filepath.Join(t.TempDir(), "config.json")
		if err := save(p, Settings{Version: 1, InstanceName: "Legacy", Theme: old}); err != nil {
			t.Fatal(err)
		}
		got, err := load(p)
		if err != nil {
			t.Fatal(err)
		}
		if got.Theme != want {
			t.Fatalf("%s migrated to %s, want %s", old, got.Theme, want)
		}
	}
}

func securityTestServer(t *testing.T) *Server {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Mkdir("settings", 0700); err != nil {
		t.Fatal(err)
	}
	colors := ThemeColors{
		Background: "#101010", Sidebar: "#202020", Surface: "#303030",
		Border: "#404040", Text: "#eeeeee", Primary: "#5566aa", Secondary: "#66aaaa",
	}
	themes := map[string]Theme{"review-theme": {ID: "review-theme", Name: "Review Theme", Base: "aurora", Colors: colors}}
	serialized, err := serializeThemes(themes)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(themePath, serialized); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(themeBackup, serialized); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "settings", "config.json")
	if err := save(cfgPath, defaults()); err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg: defaults(), cfgPath: cfgPath, started: time.Now(), log: log.New(io.Discard, "", 0), themes: themes,
	}
}

func postForm(t *testing.T, handler http.Handler, host, path, body, fetchSite, origin, referer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "http://"+host+path, strings.NewReader(body))
	r.Host = host
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if referer != "" {
		r.Header.Set("Referer", referer)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestThemeDuplicateBrowserRequestSecurity(t *testing.T) {
	for _, host := range []string{"127.0.0.1:18080", "127.0.0.1:18085"} {
		for _, tc := range []struct {
			name, fetchSite, origin, referer string
			want                             int
		}{
			{"opaque Origin with same-origin metadata", "same-origin", "null", "", http.StatusSeeOther},
			{"matching Origin", "same-origin", "http://" + host, "", http.StatusSeeOther},
			{"Referer only", "", "", "http://" + host + "/settings/themes", http.StatusSeeOther},
			{"missing origin metadata", "same-origin", "", "", http.StatusSeeOther},
			{"opaque Origin without Fetch Metadata", "", "null", "", http.StatusForbidden},
			{"cross-site metadata", "cross-site", "", "", http.StatusForbidden},
			{"same-site but not same-origin", "same-site", "http://" + host, "", http.StatusForbidden},
			{"foreign origin", "same-origin", "http://evil.example", "", http.StatusForbidden},
			{"different port", "same-origin", "http://127.0.0.1:18086", "", http.StatusForbidden},
			{"foreign referer", "same-origin", "", "http://evil.example/page", http.StatusForbidden},
		} {
			t.Run(host+"/"+tc.name, func(t *testing.T) {
				s := securityTestServer(t)
				before, err := os.ReadFile(themePath)
				if err != nil {
					t.Fatal(err)
				}
				w := postForm(t, s.handler(), host, "/themes/duplicate", "source=aurora", tc.fetchSite, tc.origin, tc.referer)
				if w.Code != tc.want {
					t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.want, w.Body.String())
				}
				if tc.want == http.StatusSeeOther {
					if !strings.Contains(w.Header().Get("Location"), "edit=aurora-copy") {
						t.Fatalf("missing editor redirect: %q", w.Header().Get("Location"))
					}
					if _, err := os.Stat(themePath); err != nil {
						t.Fatalf("duplicate theme was not persisted: %v", err)
					}
				} else if after, err := os.ReadFile(themePath); err != nil || !bytes.Equal(before, after) {
					t.Fatalf("rejected request changed themes file: read error %v", err)
				}
			})
		}
	}
}

func TestSettingsPostRoutesRejectForeignOriginsWithoutMutation(t *testing.T) {
	routes := []struct{ path, body string }{
		{"/settings", "instance_name=Changed&theme=dracula-dark"},
		{"/themes/refresh", ""},
		{"/themes/duplicate", "source=aurora"},
		{"/themes/save", "id=review-theme&name=Changed"},
	}
	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			s := securityTestServer(t)
			paths := []string{s.cfgPath, themePath, themeBackup}
			before := make([][]byte, len(paths))
			for i, path := range paths {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[i] = b
			}
			w := postForm(t, s.handler(), "127.0.0.1:18080", route.path, route.body, "", "http://evil.example", "")
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
			}
			for i, path := range paths {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before[i], after) {
					t.Fatalf("rejected request changed %s: read error %v", path, err)
				}
			}
		})
	}
}

func TestSettingsAndThemeMutationRoutesAcceptSameOriginBrowserPosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1:18080", "127.0.0.1:18085"} {
		t.Run("apply on "+host, func(t *testing.T) {
			s := securityTestServer(t)
			w := postForm(t, s.handler(), host, "/settings", "instance_name=ReelAnvil+Check&theme=dracula-dark&theme_page=1", "same-origin", "null", "")
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/settings/themes?saved=1" {
				t.Fatalf("status/location = %d/%q; body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
			}
			cfg, err := load(s.cfgPath)
			if err != nil || cfg.Theme != "dracula-dark" {
				t.Fatalf("theme selection not persisted: config=%+v err=%v", cfg, err)
			}
		})
		t.Run("custom theme save on "+host, func(t *testing.T) {
			s := securityTestServer(t)
			values := url.Values{
				"id": {"review-theme"}, "name": {"Updated Review Theme"},
				"background": {"#111111"}, "sidebar": {"#222222"}, "surface": {"#333333"},
				"border": {"#444444"}, "text": {"#eeeeee"}, "primary": {"#5566aa"}, "secondary": {"#66aaaa"},
			}
			w := postForm(t, s.handler(), host, "/themes/save", values.Encode(), "same-origin", "http://"+host, "")
			if w.Code != http.StatusSeeOther {
				t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
			}
			themes, err := parseThemeFile(themePath)
			if err != nil || themes["review-theme"].Name != "Updated Review Theme" || themes["review-theme"].Colors.Background != "#111111" {
				t.Fatalf("custom theme update not persisted: themes=%+v err=%v", themes, err)
			}
		})
		t.Run("theme refresh on "+host, func(t *testing.T) {
			s := securityTestServer(t)
			w := postForm(t, s.handler(), host, "/themes/refresh", "", "same-origin", "http://"+host, "")
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/settings/themes" {
				t.Fatalf("status/location = %d/%q; body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
			}
			if _, ok := s.themes["review-theme"]; !ok {
				t.Fatal("refresh did not load the persisted custom theme")
			}
		})
	}
}
