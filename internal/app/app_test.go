package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	original := Settings{Version: 1, InstanceName: "Test", Theme: "tokyo-night"}
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
	for _, theme := range []string{"aurora", "dracula-dark", "tomorrow-night", "tokyo-night", "nocturne", "foundry"} {
		if !validTheme(theme) {
			t.Fatalf("missing theme %s", theme)
		}
		if theme != "aurora" && !strings.Contains(pageTemplate.Tree.Root.String(), "body."+theme+"{") {
			t.Fatalf("missing palette %s", theme)
		}
	}
	for old, want := range map[string]string{"dark": "tomorrow-night", "slate": "tokyo-night", "pastel": "nocturne"} {
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
