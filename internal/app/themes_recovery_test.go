package app

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recoveryServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	return &Server{dataDir: dir, themes: map[string]Theme{}, log: log.New(io.Discard, "", 0)}
}

func goodTheme(id string) Theme {
	return Theme{ID: id, Name: "Theme " + id, Base: "aurora", Colors: baseColors("aurora")}
}

func writeThemeFile(t *testing.T, path string, themes ...Theme) []byte {
	t.Helper()
	m := map[string]Theme{}
	for _, th := range themes {
		m[th.ID] = th
	}
	b, err := serializeThemes(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInvalidPrimaryUsesBackupAndKeepsPrimaryBytes(t *testing.T) {
	s := recoveryServer(t)
	writeThemeFile(t, s.file(themeBackup), goodTheme("kept"))
	bad := []byte("{\n  \"schema_version\": 1,\n  oops")
	if err := os.WriteFile(s.file(themePath), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	s.reloadThemes()
	if _, ok := s.themes["kept"]; !ok {
		t.Fatal("backup theme not loaded")
	}
	if !strings.Contains(s.themeError, "themes.json") || !strings.Contains(s.themeError, "line 3") {
		t.Fatalf("error should name the path and line: %q", s.themeError)
	}
	if got, _ := os.ReadFile(s.file(themePath)); string(got) != string(bad) {
		t.Fatal("invalid primary was modified")
	}
	if err := s.persistThemes(map[string]Theme{"x": goodTheme("x")}); err == nil {
		t.Fatal("writes must stay locked until repair")
	}
}

func TestBothFilesUnusableLeavesBuiltinsAndLocksWrites(t *testing.T) {
	s := recoveryServer(t)
	_ = os.WriteFile(s.file(themePath), []byte("nope"), 0o600)
	_ = os.WriteFile(s.file(themeBackup), []byte("also nope"), 0o600)
	s.reloadThemes()
	if s.themeError == "" || len(s.themes) != 0 {
		t.Fatalf("error %q, themes %d", s.themeError, len(s.themes))
	}
	if !validTheme("aurora") || !validTheme("foundry") {
		t.Fatal("built-in themes must stay available")
	}
	if got, _ := os.ReadFile(s.file(themeBackup)); string(got) != "also nope" {
		t.Fatal("unusable backup was overwritten")
	}
	// Repair and refresh: error clears, other valid themes survive, writes work.
	writeThemeFile(t, s.file(themePath), goodTheme("a"), goodTheme("b"))
	s.reloadThemes()
	if s.themeError != "" || len(s.themes) != 2 {
		t.Fatalf("after repair: error %q, themes %d", s.themeError, len(s.themes))
	}
	if err := s.persistThemes(cloneThemes(s.themes)); err != nil {
		t.Fatalf("writes should work after repair: %v", err)
	}
}

func TestMissingPrimaryWithInvalidBackupIsNotInvented(t *testing.T) {
	s := recoveryServer(t)
	_ = os.WriteFile(s.file(themeBackup), []byte("{broken"), 0o600)
	s.reloadThemes()
	if s.themeError == "" {
		t.Fatal("expected a visible error")
	}
	if _, err := os.Stat(s.file(themePath)); !os.IsNotExist(err) {
		t.Fatal("primary must not be created from an invalid backup")
	}
	if got, _ := os.ReadFile(s.file(themeBackup)); string(got) != "{broken" {
		t.Fatal("invalid backup was replaced")
	}
}

func TestValidRefreshUpdatesBackupToSnapshot(t *testing.T) {
	s := recoveryServer(t)
	writeThemeFile(t, s.file(themeBackup), goodTheme("old"))
	want := writeThemeFile(t, s.file(themePath), goodTheme("new"), goodTheme("other"))
	s.reloadThemes()
	if s.themeError != "" {
		t.Fatal(s.themeError)
	}
	if got, _ := os.ReadFile(s.file(themeBackup)); string(got) != string(want) {
		t.Fatal("backup is not a complete snapshot of the valid primary")
	}
	if len(s.themes) != 2 {
		t.Fatalf("themes = %d", len(s.themes))
	}
}

func TestInvalidThemesRejectedWithoutPartialActivation(t *testing.T) {
	good := goodTheme("good")
	c := baseColors("aurora")
	c.Primary = "#12"
	cases := map[string][]Theme{
		"reserved id":   {{ID: "aurora", Name: "X", Base: "aurora", Colors: baseColors("aurora")}},
		"duplicate id":  {good, good},
		"bad base":      {{ID: "b", Name: "B", Base: "nope", Colors: baseColors("aurora")}},
		"bad color":     {{ID: "c", Name: "C", Base: "aurora", Colors: c}},
		"empty name":    {{ID: "d", Name: "", Base: "aurora", Colors: baseColors("aurora")}},
		"newline name":  {{ID: "e", Name: "a\nb", Base: "aurora", Colors: baseColors("aurora")}},
		"long name":     {{ID: "f", Name: strings.Repeat("x", 65), Base: "aurora", Colors: baseColors("aurora")}},
		"mixed one bad": {good, {ID: "g", Name: "G", Base: "aurora", Colors: c}},
	}
	for name, themes := range cases {
		if m, err := validateThemes(themeFile{SchemaVersion: 1, Themes: themes}); err == nil || m != nil {
			t.Errorf("%s: accepted (m=%v err=%v)", name, m, err)
		}
	}
	if _, err := validateThemes(themeFile{SchemaVersion: 99, Themes: []Theme{good}}); err == nil {
		t.Error("unsupported schema accepted")
	}
}

func TestUnwritableDirectoryKeepsPrimaryAndReportsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s := recoveryServer(t)
	want := writeThemeFile(t, s.file(themePath), goodTheme("a"))
	writeThemeFile(t, s.file(themeBackup), goodTheme("a"))
	s.reloadThemes()
	dir := filepath.Dir(s.file(themePath))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if err := s.persistThemes(map[string]Theme{"a": goodTheme("a"), "b": goodTheme("b")}); err == nil {
		t.Fatal("write into a read-only directory should fail")
	}
	if got, _ := os.ReadFile(s.file(themePath)); string(got) != string(want) {
		t.Fatal("primary changed after a failed write")
	}
	if _, ok := s.themes["b"]; ok {
		t.Fatal("failed save activated the new theme")
	}
}

func TestSaveLeavesNoTemporaryFilesAndPrivatePermissions(t *testing.T) {
	s := recoveryServer(t)
	s.reloadThemes()
	if err := s.persistThemes(map[string]Theme{"a": goodTheme("a")}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(s.file(themePath)))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".themes.json-") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
	for _, p := range []string{themePath, themeBackup} {
		if fi, err := os.Stat(s.file(p)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %v err %v", p, fi.Mode(), err)
		}
	}
}

func TestCustomThemesSurviveRestart(t *testing.T) {
	s := recoveryServer(t)
	s.reloadThemes()
	if err := s.persistThemes(map[string]Theme{"mine": goodTheme("mine")}); err != nil {
		t.Fatal(err)
	}
	again := &Server{dataDir: s.dataDir, themes: map[string]Theme{}, log: s.log}
	again.reloadThemes()
	if again.themeError != "" || again.themes["mine"].Name != "Theme mine" {
		t.Fatalf("after restart: %q %v", again.themeError, again.themes)
	}
}
