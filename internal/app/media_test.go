package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkuba/ReelAnvil/internal/pipeline"
)

func TestWorkerAPIAuth(t *testing.T) {
	s := securityTestServer(t)
	s.workerToken = "0123456789abcdef0123456789abcdef"
	h := s.handler()
	call := func(remote, token, method string) int {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080/api/worker/claim", strings.NewReader("{}"))
		r.Host = "127.0.0.1:8080"
		r.RemoteAddr = remote
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, c := range []struct {
		remote, token, method string
		want                  int
	}{
		{"127.0.0.1:5000", "", http.MethodPost, http.StatusForbidden},
		{"127.0.0.1:5000", "wrong", http.MethodPost, http.StatusForbidden},
		{"192.0.2.10:5000", s.workerToken, http.MethodPost, http.StatusForbidden},
		{"127.0.0.1:5000", s.workerToken, http.MethodGet, http.StatusMethodNotAllowed},
		{"127.0.0.1:5000", s.workerToken, http.MethodPost, http.StatusNoContent},
	} {
		if got := call(c.remote, c.token, c.method); got != c.want {
			t.Errorf("%s %s token=%q: %d, want %d", c.method, c.remote, c.token, got, c.want)
		}
	}
}

func TestEnsureWorkerTokenIsStableAndPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	a, err := ensureWorkerToken(dir)
	if err != nil || len(a) != 64 {
		t.Fatalf("token %q, %v", a, err)
	}
	b, _ := ensureWorkerToken(dir)
	if a != b {
		t.Fatal("token changed between starts")
	}
	fi, _ := os.Stat(filepath.Join(dir, workerTokenPath))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestNewJobForm(t *testing.T) {
	s := securityTestServer(t)
	s.dataDir, _ = os.Getwd()
	h := s.handler()
	root := effectiveMediaRoot(s.cfg, s.dataDir)
	if err := os.MkdirAll(folderPath(root, FolderIncoming), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folderPath(root, FolderIncoming), "talk.mp4"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	post := func(body string) *httptest.ResponseRecorder {
		return postForm(t, h, "127.0.0.1:8080", "/media/jobs", body, "same-origin", "http://127.0.0.1:8080", "")
	}
	for _, body := range []string{
		"origin=file&file=talk.mp4&clips=2&min_s=20&max_s=60",                          // no rights
		"rights=yes&origin=file&file=../config.json&clips=2&min_s=20&max_s=60",         // outside incoming
		"rights=yes&origin=url&url=http://example.com/v.mp4&clips=2&min_s=20&max_s=60", // not https
		"rights=yes&origin=file&file=talk.mp4&clips=9&min_s=20&max_s=60",               // too many clips
	} {
		if w := post(body); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/media" {
			t.Errorf("%s: %d %s", body, w.Code, w.Header().Get("Location"))
		}
	}
	w := post("rights=yes&origin=file&file=talk.mp4&clips=2&min_s=20&max_s=60")
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/jobs?id=") {
		t.Fatalf("valid job: %d %s", w.Code, w.Header().Get("Location"))
	}
	if page := get(t, h, w.Header().Get("Location")); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "talk") {
		t.Fatalf("job page: %d", page.Code)
	}
}

// Regression: an 18-minute transcript went to the hunter in one part and
// every pick came from its first three minutes.
func TestHunterWindowsSplitLongTranscripts(t *testing.T) {
	segs := make([]pipeline.Segment, 140)
	for i := range segs {
		segs[i] = pipeline.Segment{StartMS: int64(i) * 8000, EndMS: int64(i+1) * 8000, Text: strings.Repeat("word ", 28)}
	}
	if n := len(pipeline.Windows(segs, hunterWindowChars)); n < 4 {
		t.Fatalf("18-minute transcript in %d part(s), want at least 4", n)
	}
}

func TestUnderRoot(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "completed", "1"), 0o750)
	_ = os.WriteFile(filepath.Join(root, "completed", "1", "a.mp4"), []byte("x"), 0o640)
	_ = os.Symlink("/etc/passwd", filepath.Join(root, "completed", "1", "b.mp4"))
	if _, ok := underRoot(root, "completed/1/a.mp4"); !ok {
		t.Fatal("plain file refused")
	}
	for _, rel := range []string{"", "/etc/passwd", "../x", "completed/1/b.mp4", "completed/../../x"} {
		if _, ok := underRoot(root, rel); ok {
			t.Errorf("%q accepted", rel)
		}
	}
}
