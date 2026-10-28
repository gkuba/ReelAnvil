package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
