package app

import (
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCustomThemeValidationAndCSS(t *testing.T) {
	c := baseColors("aurora")
	theme := Theme{ID: "my-aurora", Name: "My Aurora", Base: "aurora", Colors: c}
	m, err := validateThemes(themeFile{SchemaVersion: 1, Themes: []Theme{theme}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(themeCSS(m)), "body.theme-my-aurora{") {
		t.Fatal("custom CSS selector missing")
	}
	theme.Colors.Primary = "red"
	if _, err = validateThemes(themeFile{SchemaVersion: 1, Themes: []Theme{theme}}); err == nil {
		t.Fatal("accepted invalid color")
	}
}
func TestThemeFileParsing(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "themes.json")
	m := map[string]Theme{"custom": {ID: "custom", Name: "Custom", Base: "foundry", Colors: baseColors("foundry")}}
	b, err := serializeThemes(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseThemeFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if parsed["custom"].Name != "Custom" {
		t.Fatal("theme not round-tripped")
	}
	if err = os.WriteFile(p, []byte("{\n invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = parseThemeFile(p)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected line-numbered syntax error, got %v", err)
	}
}

func TestMissingPrimaryRestoresValidBackup(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.MkdirAll(filepath.Join(dir, "settings"), 0700); err != nil {
		t.Fatal(err)
	}
	theme := Theme{ID: "saved-theme", Name: "Saved Theme", Base: "aurora", Colors: baseColors("aurora")}
	backup, err := serializeThemes(map[string]Theme{theme.ID: theme})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, themeBackup), backup, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	s := &Server{themes: map[string]Theme{}, log: log.New(io.Discard, "", 0)}
	s.reloadThemes()
	if s.themeError != "" {
		t.Fatalf("unexpected theme error: %s", s.themeError)
	}
	if got := s.themes[theme.ID]; got != theme {
		t.Fatalf("loaded theme = %#v, want %#v", got, theme)
	}
	primary, err := parseThemeFile(themePath)
	if err != nil {
		t.Fatalf("restored primary is invalid: %v", err)
	}
	if got := primary[theme.ID]; got != theme {
		t.Fatalf("restored primary theme = %#v, want %#v", got, theme)
	}
	if got, err := os.ReadFile(filepath.Join(dir, themeBackup)); err != nil || string(got) != string(backup) {
		t.Fatalf("backup changed during restore: err=%v", err)
	}
}

func TestSettingsPromptsToDuplicateWhenNoCustomThemes(t *testing.T) {
	s := &Server{
		cfg:     defaults(),
		started: time.Now(),
		log:     log.New(io.Discard, "", 0),
		themes:  map[string]Theme{},
	}
	r := httptest.NewRequest("GET", "/settings/themes", nil)
	w := httptest.NewRecorder()
	s.page(w, r)
	if w.Code != 200 {
		t.Fatalf("settings response = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Duplicate a theme to create your first editable copy.") {
		t.Fatal("empty Theme Manager prompt is missing")
	}
	if strings.Contains(body, `id="theme-save"`) {
		t.Fatal("empty Theme Manager exposes a Save Changes button")
	}
}
