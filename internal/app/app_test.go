package app

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
)

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	original := Settings{Version: 3, InstanceName: "Test", Theme: "aurora", IconVariant: "molten-orange", ThemeIcons: map[string]string{"aurora": "amethyst", "dracula-dark": "molten-orange", "custom": "amethyst"}}
	if err := save(p, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, original) {
		t.Fatalf("got %+v, want %+v", loaded, original)
	}
}

func TestSettingsVersionOneMigratesToGlobalIconDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"version":1,"instance_name":"Kept","theme":"aurora"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 3 || c.InstanceName != "Kept" || c.Theme != "aurora" || c.IconVariant != "blue-violet" {
		t.Fatalf("unexpected migrated settings: %+v", c)
	}
}

func TestSettingsVersionTwoPreservesActiveThemeIconAndAllLegacyPreferences(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"version":2,"instance_name":"Kept","theme":"dracula-dark","theme_icons":{"dracula-dark":"amethyst","aurora":"molten-orange","custom-copy":"amethyst"}}`
	if err := os.WriteFile(p, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 3 || c.Theme != "dracula-dark" || c.IconVariant != "amethyst" {
		t.Fatalf("active theme appearance changed on upgrade: %+v", c)
	}
	if c.ThemeIcons["aurora"] != "molten-orange" || c.ThemeIcons["custom-copy"] != "amethyst" {
		t.Fatalf("legacy theme preferences were not preserved: %#v", c.ThemeIcons)
	}
}

func TestSettingsMigrationKeepsIconWhenThemeIDIsMigrated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"version":2,"instance_name":"Kept","theme":"dracula-lavender","theme_icons":{"dracula-lavender":"molten-orange"}}`
	if err := os.WriteFile(p, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "dracula-dark" || c.IconVariant != "molten-orange" || c.ThemeIcons["dracula-lavender"] != "molten-orange" {
		t.Fatalf("legacy theme ID migration lost saved appearance: %+v", c)
	}
}

func TestFreshSettingsDefaultToDraculaAndBlueViolet(t *testing.T) {
	c, err := load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 3 || c.Theme != "dracula-dark" || c.IconVariant != "blue-violet" {
		t.Fatalf("unexpected fresh defaults: %+v", c)
	}
}

func TestCampaignPagesManualTrackingEscapingAndFilters(t *testing.T) {
	s := securityTestServer(t)
	ctx := context.Background()
	_, created, err := s.store.AddManualCampaign(ctx, store.CampaignInput{Platform: "vyro", URL: "https://example.com/campaign?utm_source=test", Title: "<script>alert(1)</script>", Requirements: "Keep this safe"})
	if err != nil || !created {
		t.Fatalf("add campaign: created=%v err=%v", created, err)
	}
	h := s.handler()
	for _, path := range []string{"/", "/campaigns?view=tracked", "/campaigns?view=available", "/campaigns?view=tracked&platform=vyro&sort=deadline"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, body=%s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
			t.Fatalf("unescaped campaign metadata on %s", path)
		}
		if strings.Contains(path, "view=tracked") && !strings.Contains(w.Body.String(), "&lt;script&gt;alert(1)&lt;/script&gt;") {
			t.Fatalf("campaign title not rendered safely on %s", path)
		}
		if strings.Contains(w.Body.String(), "href=\"javascript:") {
			t.Fatalf("unsafe external link rendered on %s", path)
		}
	}
	all, err := s.store.ListCampaigns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].Tracked || all[0].UserStatus != "Interested" {
		t.Fatalf("manual campaign not tracked: %+v", all)
	}
	if err = s.store.TrackCampaign(ctx, all[0].ID, "Active", "Local notes survive refresh"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.UpsertDiscoveredCampaign(ctx, store.Campaign{Platform: "vyro", ExternalID: "stable-1", URL: "https://example.com/campaign", Title: "Updated source title", Provenance: "verified public source"}); err != nil {
		t.Fatal(err)
	}
	updated, err := s.store.CampaignByID(ctx, all[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.UserStatus != "Active" || updated.Notes != "Local notes survive refresh" || updated.Title != "Updated source title" {
		t.Fatalf("campaign refresh lost local or source data: %+v", updated)
	}
}

func TestGlobalIconSettingSharesThemeSaveAndServesFavicon(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	w := postForm(t, h, "127.0.0.1:8080", "/settings", "theme_page=1&instance_name=ReelAnvil&theme=foundry&icon_variant=amethyst", "same-origin", "http://127.0.0.1:8080", "")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("theme and icon save status=%d body=%s", w.Code, w.Body.String())
	}
	loaded, err := load(s.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "foundry" || loaded.IconVariant != "amethyst" {
		t.Fatalf("theme and global icon were not saved together: %+v", loaded)
	}
	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/settings/themes", nil)
	r.Host = "127.0.0.1:8080"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Application icon") || !strings.Contains(w.Body.String(), "name=\"icon_variant\" value=\"amethyst\" checked") || !strings.Contains(w.Body.String(), "Molten Orange") || strings.Contains(w.Body.String(), "icon-theme") || strings.Contains(w.Body.String(), "Save theme icon") {
		t.Fatalf("global icon settings did not render in theme panel: %d", w.Code)
	}
	// Changing the interface theme without submitting an icon leaves the global choice intact.
	w = postForm(t, h, "127.0.0.1:8080", "/settings", "theme_page=1&instance_name=ReelAnvil&theme=aurora", "same-origin", "http://127.0.0.1:8080", "")
	if w.Code != http.StatusSeeOther || s.cfg.IconVariant != "amethyst" || s.cfg.Theme != "aurora" {
		t.Fatalf("theme switch changed global icon: theme=%s icon=%s status=%d", s.cfg.Theme, s.cfg.IconVariant, w.Code)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	r.Host = "127.0.0.1:8080"
	h.ServeHTTP(w, r)
	wantFavicon, err := brandingAssets.ReadFile("branding/favicon-amethyst.ico")
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/vnd.microsoft.icon" || !bytes.HasPrefix(w.Body.Bytes(), []byte{0, 0, 1, 0}) || !bytes.Equal(w.Body.Bytes(), wantFavicon) {
		t.Fatalf("favicon response invalid: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/assets/branding/icon-amethyst-16.png", nil)
	r.Host = "127.0.0.1:8080"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("local icon response invalid: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestSharedSidebarHierarchyAcrossApplicationPages(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	paths := []string{"/", "/campaigns", "/jobs", "/media", "/review", "/diagnostics", "/settings", "/settings/themes", "/settings/connectors", "/settings/storage", "/services"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Host = "127.0.0.1:8080"
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			body := w.Body.String()
			if w.Code != http.StatusOK || !strings.Contains(body, `id="app-sidebar"`) {
				t.Fatalf("shared sidebar missing on %s: %d", path, w.Code)
			}
			for _, href := range []string{`href="/"`, `href="/campaigns"`, `href="/jobs"`, `href="/media"`, `href="/review"`, `href="/diagnostics"`, `href="/settings"`, `href="/settings/themes"`, `href="/settings/connectors"`, `href="/settings/storage"`, `href="/services"`} {
				if !strings.Contains(body, href) {
					t.Errorf("%s missing from sidebar navigation: %s", path, href)
				}
			}
			if strings.Contains(body, "Automation") || strings.Contains(body, "data-group=\"workflow\"") {
				t.Errorf("legacy Automation group remains on %s", path)
			}
			for _, iconName := range []string{"Overview", "Campaigns", "Jobs", "Media", "Media Library", "Review", "Diagnostics", "Settings", "General", "Themes", "Connectors", "Paths &amp; Storage", "Services"} {
				if !strings.Contains(body, iconName) {
					t.Errorf("%s missing navigation item %q", path, iconName)
				}
			}
		})
	}
}

func TestThemeAndGlobalIconShareSingleSaveForm(t *testing.T) {
	s := securityTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "/settings/themes", nil)
	r.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	body := w.Body.String()
	icon := strings.Index(body, `name="icon_variant"`)
	refresh := strings.Index(body, `form="refresh-themes"`)
	save := strings.Index(body, `>Save Settings</button>`)
	if w.Code != http.StatusOK || icon < 0 || refresh < 0 || save < 0 || !(icon < refresh && refresh < save) {
		t.Fatalf("theme panel does not place global icon before the shared save actions")
	}
	if strings.Contains(body, `name="theme_key"`) || strings.Contains(body, `id="icon-theme"`) || strings.Contains(body, "Save theme icon") || strings.Contains(body, "Choose an icon for each interface theme") {
		t.Fatal("legacy per-theme icon controls remain")
	}
}

func TestCampaignPostRejectsCrossOriginAndMethods(t *testing.T) {
	s := securityTestServer(t)
	h := s.handler()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/campaigns/add", strings.NewReader("platform=vyro&url=https%3A%2F%2Fexample.com&title=Test"))
	r.Host = "127.0.0.1:8080"
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin campaign POST status=%d", w.Code)
	}
	all, err := s.store.ListCampaigns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("cross-origin request created campaign: %+v", all)
	}
	r = httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:8080/campaigns/add", nil)
	r.Host = "127.0.0.1:8080"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE campaign add status=%d", w.Code)
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
			if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self'") {
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

func TestCampaignMutationRoutesRejectCrossOriginPosts(t *testing.T) {
	routes := []struct{ path, body string }{
		{"/campaigns/add", "platform=vyro&url=https%3A%2F%2Fexample.com&title=Probe"},
		{"/campaigns/track", "id=missing"},
		{"/campaigns/untrack", "id=missing"},
		{"/campaigns/status", "id=missing&status=Active"},
		{"/campaigns/update", "id=missing&platform=vyro&url=https%3A%2F%2Fexample.com&title=Probe&status=Active"},
		{"/settings", "theme_page=1&instance_name=Probe&theme=dracula-dark&icon_variant=amethyst"},
	}
	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			s := securityTestServer(t)
			before, err := s.store.ListCampaigns(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			w := postForm(t, s.handler(), "127.0.0.1:18080", route.path, route.body, "cross-site", "http://evil.example", "")
			if w.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			after, err := s.store.ListCampaigns(context.Background())
			if err != nil || len(after) != len(before) {
				t.Fatalf("rejected request mutated campaign database: before=%+v after=%+v err=%v", before, after, err)
			}
		})
	}
}

func TestCampaignMutationRoutesRejectNonPostMethods(t *testing.T) {
	for _, route := range []string{"/campaigns/add", "/campaigns/track", "/campaigns/untrack", "/campaigns/status", "/campaigns/update"} {
		t.Run(route, func(t *testing.T) {
			s := securityTestServer(t)
			r := httptest.NewRequest(http.MethodGet, route, nil)
			r.Host = "127.0.0.1:8080"
			w := httptest.NewRecorder()
			s.handler().ServeHTTP(w, r)
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET %s = %d", route, w.Code)
			}
		})
	}
}

func TestEndingSoonUsesCalendarDatesAndKnownDeadlinesOnly(t *testing.T) {
	now := time.Date(2026, 10, 28, 19, 0, 0, 0, time.FixedZone("local", -6*60*60))
	if !deadlineWithinSevenDays(store.Campaign{Deadline: "2026-10-28"}, now) {
		t.Fatal("today's calendar deadline should count as ending soon")
	}
	if !deadlineWithinSevenDays(store.Campaign{Deadline: "2026-11-04"}, now) {
		t.Fatal("deadline on the seventh day should count")
	}
	if deadlineWithinSevenDays(store.Campaign{Deadline: "2026-11-05"}, now) {
		t.Fatal("deadline beyond seven days counted")
	}
	if deadlineWithinSevenDays(store.Campaign{Deadline: "unknown"}, now) {
		t.Fatal("invalid deadline counted")
	}
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
	if c.Theme != "dracula-dark" || c.IconVariant != "blue-violet" {
		t.Fatal(c)
	}
}
func TestUnsupportedVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"version":4}`), 0600); err != nil {
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
		if !strings.Contains(themeCSS(nil), "body."+theme+"{") {
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
	db, err := store.Open(filepath.Join(dir, "data", "reelanvil.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test campaign database: %v", err)
		}
	})
	return &Server{
		cfg: defaults(), cfgPath: cfgPath, started: time.Now(), log: log.New(io.Discard, "", 0), themes: themes,
		store: db,
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
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/settings/themes" || !strings.Contains(w.Header().Get("Set-Cookie"), flashCookie+"=") {
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
