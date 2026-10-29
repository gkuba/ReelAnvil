package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gkuba/ReelAnvil/internal/store"
)

func get(t *testing.T, h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Host = "127.0.0.1:8080"
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestEveryPageRendersWithSharedAssets(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	for _, path := range []string{"/", "/campaigns", "/jobs", "/media", "/review", "/services", "/diagnostics", "/settings", "/settings/themes", "/settings/connectors", "/settings/storage"} {
		w := get(t, h, path)
		body := w.Body.String()
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, w.Code)
		}
		for _, want := range []string{`href="/assets/themes.css"`, `href="/assets/app.css?v=`, `src="/assets/app.js?v=`, `id="app-sidebar"`, "Build " + Version} {
			if path == "/" || path == "/campaigns" {
				if want == "Build "+Version {
					continue
				}
			}
			if !strings.Contains(body, want) {
				t.Errorf("%s missing %q", path, want)
			}
		}
	}
	if w := get(t, h, "/nope"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown page = %d", w.Code)
	}
}

func TestStaticAssetsAndThemeStylesheet(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	if w := get(t, h, "/assets/app.css"); w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("app.css = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := get(t, h, "/assets/app.js"); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("app.js = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := get(t, h, "/assets/"); w.Code != http.StatusNotFound {
		t.Fatalf("asset directory listing = %d", w.Code)
	}
	w := get(t, h, "/assets/themes.css")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "body.theme-review-theme{--bg:#101010;") || !strings.Contains(w.Body.String(), "body.foundry{") {
		t.Fatalf("themes.css = %d %s", w.Code, w.Body.String())
	}
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("CSP still allows inline code: %s", csp)
	}
}

func TestFlashMessageShownOnceAndNotFromQuery(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	w := postForm(t, h, "127.0.0.1:8080", "/settings", "instance_name=Renamed", "same-origin", "http://127.0.0.1:8080", "")
	cookies := w.Result().Cookies()
	if w.Code != http.StatusSeeOther || len(cookies) != 1 || cookies[0].Name != flashCookie {
		t.Fatalf("settings save = %d cookies=%v", w.Code, cookies)
	}
	page := get(t, h, "/settings", cookies[0])
	if !strings.Contains(page.Body.String(), "Settings saved successfully.") {
		t.Fatal("flash message not rendered")
	}
	if cleared := page.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("flash cookie not cleared: %v", cleared)
	}
	if body := get(t, h, "/campaigns?message=Injected+text&error=Injected+error").Body.String(); strings.Contains(body, "Injected") {
		t.Fatal("query parameters still render as messages")
	}
}

func TestCampaignErrorsAreUserSafe(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	ctx := context.Background()
	a, _, err := s.store.AddManualCampaign(ctx, store.CampaignInput{Platform: "vyro", URL: "https://example.com/a", Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.store.AddManualCampaign(ctx, store.CampaignInput{Platform: "vyro", URL: "https://example.com/b", Title: "B"}); err != nil {
		t.Fatal(err)
	}
	flashText := func(w *httptest.ResponseRecorder) string {
		t.Helper()
		cookies := w.Result().Cookies()
		if w.Code != http.StatusSeeOther || len(cookies) != 1 {
			t.Fatalf("status=%d cookies=%v", w.Code, cookies)
		}
		body := get(t, h, "/campaigns?view=tracked", cookies[0]).Body.String()
		start := strings.Index(body, `role="alert">`)
		if start < 0 {
			start = strings.Index(body, `role="status">`)
		}
		if start < 0 {
			t.Fatal("no flash message on page")
		}
		return body[start : start+120]
	}
	form := url.Values{"id": {a.ID}, "platform": {"vyro"}, "url": {"https://example.com/b"}, "title": {"A"}, "status": {"Interested"}, "tracked": {"true"}}
	msg := flashText(postForm(t, h, "127.0.0.1:8080", "/campaigns/update", form.Encode(), "same-origin", "http://127.0.0.1:8080", ""))
	if strings.Contains(msg, "UNIQUE") || !strings.Contains(msg, "already uses that URL") {
		t.Fatalf("duplicate URL message = %q", msg)
	}
	msg = flashText(postForm(t, h, "127.0.0.1:8080", "/campaigns/untrack", "id=missing", "same-origin", "http://127.0.0.1:8080", ""))
	if !strings.Contains(msg, "no longer exists") {
		t.Fatalf("missing campaign message = %q", msg)
	}
	msg = flashText(postForm(t, h, "127.0.0.1:8080", "/campaigns/add", "platform=vyro&url=http%3A%2F%2Fexample.com&title=X", "same-origin", "http://127.0.0.1:8080", ""))
	if !strings.Contains(msg, "HTTPS") {
		t.Fatalf("validation message = %q", msg)
	}
}

func TestDuplicateThemeNamesIDsAndLeavesSettingsAlone(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	long := strings.Repeat("Long name ", 7)[:60]
	s.themes["a-very-long-custom-theme-identifier-for-tests-x"] = Theme{ID: "a-very-long-custom-theme-identifier-for-tests-x", Name: long, Base: "aurora", Colors: baseColors("aurora")}
	before, err := os.ReadFile(s.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		w := postForm(t, h, "127.0.0.1:8080", "/themes/duplicate", "source=a-very-long-custom-theme-identifier-for-tests-x", "same-origin", "http://127.0.0.1:8080", "")
		if w.Code != http.StatusSeeOther {
			t.Fatalf("duplicate %d = %d %s", i, w.Code, w.Body.String())
		}
	}
	w := postForm(t, h, "127.0.0.1:8080", "/themes/duplicate", "source=dracula-dark", "same-origin", "http://127.0.0.1:8080", "")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("duplicate built-in = %d", w.Code)
	}
	themes, err := parseThemeFile(themePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 6 {
		t.Fatalf("themes after duplicates = %d", len(themes))
	}
	for id, th := range themes {
		if len(id) > maxThemeIDLen || utf8.RuneCountInString(th.Name) > maxThemeNameLen {
			t.Errorf("invalid duplicate %q %q", id, th.Name)
		}
	}
	if d := themes["dracula-dark-copy"]; d.Name != "Dracula (Copy)" || d.Colors != baseColors("dracula-dark") {
		t.Fatalf("built-in duplicate = %+v", d)
	}
	after, err := os.ReadFile(s.cfgPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("duplicating a theme rewrote config.json: err=%v", err)
	}
}

func TestThemeEditorFallsBackForUnknownEditID(t *testing.T) {
	s := securityTestServer(t)
	body := get(t, s.handler(), "/settings/themes?edit=missing").Body.String()
	if !strings.Contains(body, `value="Review Theme"`) {
		t.Fatal("unknown edit ID did not fall back to the first custom theme")
	}
}

func TestSaveThemeValidationErrorRedirectsWithMessage(t *testing.T) {
	s := securityTestServer(t)
	values := url.Values{"id": {"review-theme"}, "name": {"Bad"}, "background": {"red"}, "sidebar": {"#222222"}, "surface": {"#333333"}, "border": {"#444444"}, "text": {"#eeeeee"}, "primary": {"#5566aa"}, "secondary": {"#66aaaa"}}
	w := postForm(t, s.handler(), "127.0.0.1:8080", "/themes/save", values.Encode(), "same-origin", "http://127.0.0.1:8080", "")
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "edit=review-theme") {
		t.Fatalf("invalid save = %d %q", w.Code, w.Header().Get("Location"))
	}
	if s.themes["review-theme"].Name != "Review Theme" {
		t.Fatal("invalid theme was saved")
	}
}
