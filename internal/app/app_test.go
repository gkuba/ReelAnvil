package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	original := Settings{Version: 1, InstanceName: "Test", Theme: "slate"}
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
