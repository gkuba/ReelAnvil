package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// A custom theme stores exactly seven editable semantic colors. Other CSS tokens
// are derived from the base theme, rather than hardcoded into UI components.
type Theme struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Base   string      `json:"base"`
	Colors ThemeColors `json:"colors"`
}
type ThemeColors struct {
	Background string `json:"background"`
	Sidebar    string `json:"sidebar"`
	Surface    string `json:"surface"`
	Border     string `json:"border"`
	Text       string `json:"text"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
}
type themeFile struct {
	SchemaVersion int     `json:"schema_version"`
	Themes        []Theme `json:"themes"`
}
type ColorField struct{ Key, Label, Value string }

// builtinTheme is a read-only palette. Tokens become CSS custom properties
// (--bg, --surface, ...) in the generated /assets/themes.css.
type builtinTheme struct {
	ID, Name string
	Tokens   map[string]string
}

// themeTokens lists every token a palette defines, in CSS output order.
var themeTokens = []string{"bg", "sidebar", "surface", "line", "text", "muted", "accent", "accent-text", "secondary", "tertiary", "warm", "glow", "input", "note-bg", "note-text", "success", "focus"}

const defaultTheme = "dracula-dark"

// builtinThemes is the single source of truth for built-in palettes. The first
// entry is also the :root fallback.
var builtinThemes = []builtinTheme{
	{ID: "aurora", Name: "Aurora", Tokens: map[string]string{"bg": "#0b1020", "sidebar": "#10182b", "surface": "#151c27", "line": "#344467", "text": "#cdd8f5", "muted": "#9caed0", "accent": "#7aa2f7", "accent-text": "#0b1733", "secondary": "#7dcfff", "tertiary": "#bb9af7", "warm": "#e0af68", "glow": "#263a70", "input": "#111a2e", "note-bg": "#1e2d4b", "note-text": "#c6d7ff", "success": "#9ece6a", "focus": "#bb9af7"}},
	{ID: "dracula-dark", Name: "Dracula", Tokens: map[string]string{"bg": "#10131f", "sidebar": "#161b2b", "surface": "#131722", "line": "#44475a", "text": "#f2f0f6", "muted": "#aaa7b6", "accent": "#a897f5", "accent-text": "#17181d", "secondary": "#8be9fd", "tertiary": "#ff79c6", "warm": "#f1fa8c", "glow": "#221b2d", "input": "#101522", "note-bg": "#252934", "note-text": "#e4d5f6", "success": "#50fa7b", "focus": "#8be9fd"}},
	{ID: "tomorrow-night", Name: "Tomorrow Night", Tokens: map[string]string{"bg": "#0b0d10", "sidebar": "#111419", "surface": "#12151a", "line": "#3a4551", "text": "#e2e5e9", "muted": "#a3adb9", "accent": "#81a2be", "accent-text": "#0a121b", "secondary": "#b5bd68", "tertiary": "#cc99cc", "warm": "#f0c674", "glow": "#202b36", "input": "#12161b", "note-bg": "#252e34", "note-text": "#d2e0e8", "success": "#b5bd68", "focus": "#f0c674"}},
	{ID: "nocturne", Name: "Nocturne", Tokens: map[string]string{"bg": "#141019", "sidebar": "#1b1726", "surface": "#1a1526", "line": "#554166", "text": "#eee5f6", "muted": "#b8a6c8", "accent": "#ad8bfa", "accent-text": "#1c112a", "secondary": "#8db6d9", "tertiary": "#d99cbd", "warm": "#e6bc9a", "glow": "#372444", "input": "#1c1525", "note-bg": "#35263f", "note-text": "#e3cbed", "success": "#a8d6b2", "focus": "#d1a6f6"}},
	{ID: "foundry", Name: "Foundry", Tokens: map[string]string{"bg": "#0d1014", "sidebar": "#17202a", "surface": "#111a21", "line": "#465d6c", "text": "#e8edf0", "muted": "#a4b2ba", "accent": "#f0a44c", "accent-text": "#1b1510", "secondary": "#81a9c4", "tertiary": "#70b8a4", "warm": "#eac06c", "glow": "#3d2d1c", "input": "#1c1b19", "note-bg": "#352b22", "note-text": "#f1d5b5", "success": "#b5c993", "focus": "#86a9ba"}},
}

// legacyThemeIDs maps retired built-in IDs to their replacements, for both
// saved settings and custom theme bases.
var legacyThemeIDs = map[string]string{
	"dark":             "tomorrow-night",
	"slate":            "aurora",
	"tokyo-night":      "aurora",
	"pastel":           "nocturne",
	"dracula-lavender": "dracula-dark",
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var themeID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

const maxThemeIDLen = 48
const maxThemeNameLen = 64

// Theme files live under the data directory.
const themePath = "settings/themes.json"
const themeBackup = "settings/themes.json.bak"

func findBuiltin(id string) (builtinTheme, bool) {
	for _, t := range builtinThemes {
		if t.ID == id {
			return t, true
		}
	}
	return builtinTheme{}, false
}
func validTheme(id string) bool {
	_, ok := findBuiltin(id)
	return ok
}
func builtinOptions() []Theme {
	out := make([]Theme, len(builtinThemes))
	for i, t := range builtinThemes {
		out[i] = Theme{ID: t.ID, Name: t.Name}
	}
	return out
}
func sortedThemes(m map[string]Theme) []Theme {
	out := make([]Theme, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func colorFields(t Theme) []ColorField {
	c := t.Colors
	return []ColorField{{"background", "Background", c.Background}, {"sidebar", "Sidebar", c.Sidebar}, {"surface", "Panels / Tiles", c.Surface}, {"border", "Borders", c.Border}, {"text", "Text", c.Text}, {"primary", "Primary Accent", c.Primary}, {"secondary", "Secondary Accent", c.Secondary}}
}
func validThemeName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= maxThemeNameLen && !strings.ContainsAny(name, "\r\n")
}
func validateThemes(f themeFile) (map[string]Theme, error) {
	if f.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported theme schema version %d", f.SchemaVersion)
	}
	m := make(map[string]Theme)
	for i, t := range f.Themes {
		if replacement, ok := legacyThemeIDs[t.Base]; ok {
			t.Base = replacement
		}
		if !themeID.MatchString(t.ID) || validTheme(t.ID) {
			return nil, fmt.Errorf("theme %d: invalid or reserved ID %q", i+1, t.ID)
		}
		if !validThemeName(t.Name) {
			return nil, fmt.Errorf("theme %q: name must be 1–%d characters", t.ID, maxThemeNameLen)
		}
		if !validTheme(t.Base) {
			return nil, fmt.Errorf("theme %q: unknown built-in base %q", t.ID, t.Base)
		}
		if _, exists := m[t.ID]; exists {
			return nil, fmt.Errorf("duplicate theme ID %q", t.ID)
		}
		for _, field := range colorFields(t) {
			if !hexColor.MatchString(field.Value) {
				return nil, fmt.Errorf("theme %q: %s must be a #RRGGBB hex color", t.ID, field.Label)
			}
		}
		m[t.ID] = t
	}
	return m, nil
}
func parseThemeFile(path string) (map[string]Theme, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f themeFile
	if err = json.Unmarshal(b, &f); err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			line, col := 1, 1
			for _, ch := range b[:min(int(syn.Offset), len(b))] {
				if ch == '\n' {
					line++
					col = 1
				} else {
					col++
				}
			}
			return nil, fmt.Errorf("%s: JSON syntax error at line %d, column %d: %w", path, line, col, err)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m, err := validateThemes(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// writeAtomic replaces path with b via a synced temporary file in the same
// directory, so readers see either the old or the new content.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
func serializeThemes(m map[string]Theme) ([]byte, error) {
	f := themeFile{SchemaVersion: 1, Themes: sortedThemes(m)}
	b, e := json.MarshalIndent(f, "", "  ")
	return append(b, '\n'), e
}
func (s *Server) reloadThemes() {
	s.mu.Lock()
	defer s.mu.Unlock()
	primaryPath, backupPath := s.file(themePath), s.file(themeBackup)
	m, err := parseThemeFile(primaryPath)
	if errors.Is(err, os.ErrNotExist) {
		// Restore a valid backup if the primary was removed. Do not replace an
		// invalid backup with empty data, since it may be the only recovery file.
		if recovered, backupErr := parseThemeFile(backupPath); backupErr == nil {
			b, e := serializeThemes(recovered)
			if e == nil {
				e = writeAtomic(primaryPath, b)
			}
			if e != nil {
				s.themeError = fmt.Sprintf("restore from backup failed: %v", e)
				s.log.Println(s.themeError)
				return
			}
			s.themes = recovered
			s.themeError = ""
			s.log.Println("restored themes from last known-good backup")
			return
		} else if !errors.Is(backupErr, os.ErrNotExist) {
			s.themes = map[string]Theme{}
			s.themeError = fmt.Sprintf("primary theme file is missing; backup is invalid: %v", backupErr)
			s.log.Println(s.themeError)
			return
		}

		// First launch: both primary and backup are absent, so establish empty files.
		empty := map[string]Theme{}
		b, _ := serializeThemes(empty)
		if e := writeAtomic(primaryPath, b); e != nil {
			s.themeError = e.Error()
			s.log.Println("theme initialization:", e)
			return
		}
		if e := writeAtomic(backupPath, b); e != nil {
			s.themeError = e.Error()
			s.log.Println("theme backup:", e)
			return
		}
		s.themes = empty
		s.themeError = ""
		return
	}
	if err == nil {
		// Preserve the last valid backup before activating the refreshed file.
		b, e := os.ReadFile(primaryPath)
		if e == nil {
			e = writeAtomic(backupPath, b)
		}
		if e != nil {
			s.themeError = fmt.Sprintf("backup update failed: %v", e)
			s.log.Println(s.themeError)
			return
		}
		s.themes = m
		s.themeError = ""
		return
	}
	s.themeError = err.Error()
	s.log.Println("theme validation failed:", err)
	if recovered, e := parseThemeFile(backupPath); e == nil {
		s.themes = recovered
		s.log.Println("using last known-good theme backup")
	} else {
		s.themes = map[string]Theme{}
		s.log.Println("theme backup unavailable or invalid:", e)
	}
}

// persistThemes validates and saves m. The caller must hold s.mu.
func (s *Server) persistThemes(m map[string]Theme) error {
	if s.themeError != "" {
		return errors.New("repair themes.json and refresh before saving")
	}
	if _, err := validateThemes(themeFile{SchemaVersion: 1, Themes: sortedThemes(m)}); err != nil {
		return err
	}
	b, err := serializeThemes(m)
	if err != nil {
		return err
	}
	// Existing backup remains valid if a primary write fails. Never back up invalid data.
	if err = writeAtomic(s.file(themePath), b); err != nil {
		return err
	}
	if err = writeAtomic(s.file(themeBackup), b); err != nil {
		s.themeError = fmt.Sprintf("saved primary, but backup failed: %v; theme saving locked", err)
		return err
	}
	s.themes = m
	return nil
}
func postOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}
func (s *Server) refreshThemes(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	s.reloadThemes()
	http.Redirect(w, r, "/settings/themes", http.StatusSeeOther)
}

// copyThemeID returns an unused ID of the form <source>-copy or
// <source>-copy-N, shortening the source part so the result stays valid.
func copyThemeID(source string, taken map[string]Theme) string {
	for n := 1; ; n++ {
		suffix := "-copy"
		if n > 1 {
			suffix = fmt.Sprintf("-copy-%d", n)
		}
		base := source
		if len(base)+len(suffix) > maxThemeIDLen {
			base = strings.TrimRight(base[:maxThemeIDLen-len(suffix)], "-")
		}
		if id := base + suffix; taken[id].ID == "" {
			return id
		}
	}
}

// copyThemeName appends " (Copy)", shortening the original name if needed.
func copyThemeName(name string) string {
	const suffix = " (Copy)"
	runes := []rune(strings.TrimSpace(name))
	if limit := maxThemeNameLen - utf8.RuneCountInString(suffix); len(runes) > limit {
		runes = runes[:limit]
	}
	return strings.TrimSpace(string(runes)) + suffix
}

func (s *Server) duplicateTheme(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	source := r.FormValue("source")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.themeError != "" {
		http.Error(w, s.themeError, http.StatusConflict)
		return
	}
	var t Theme
	if custom, ok := s.themes[source]; ok {
		t = Theme{Name: custom.Name, Base: custom.Base, Colors: custom.Colors}
	} else if builtin, ok := findBuiltin(source); ok {
		t = Theme{Name: builtin.Name, Base: builtin.ID, Colors: baseColors(builtin.ID)}
	} else {
		http.Error(w, "unknown theme", http.StatusBadRequest)
		return
	}
	t.ID = copyThemeID(source, s.themes)
	t.Name = copyThemeName(t.Name)
	next := cloneThemes(s.themes)
	next[t.ID] = t
	if err := s.persistThemes(next); err != nil {
		s.log.Println("duplicate theme:", err)
		http.Error(w, "could not save the duplicated theme", http.StatusInternalServerError)
		return
	}
	setFlash(w, "Theme duplicated.", false)
	http.Redirect(w, r, "/settings/themes?edit="+t.ID, http.StatusSeeOther)
}
func cloneThemes(m map[string]Theme) map[string]Theme {
	n := make(map[string]Theme, len(m)+1)
	for k, v := range m {
		n[k] = v
	}
	return n
}
func (s *Server) saveTheme(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	id := r.FormValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.themeError != "" {
		http.Error(w, s.themeError, http.StatusConflict)
		return
	}
	t, ok := s.themes[id]
	if !ok {
		http.Error(w, "unknown custom theme", http.StatusBadRequest)
		return
	}
	t.Name = strings.TrimSpace(r.FormValue("name"))
	c := &t.Colors
	c.Background = r.FormValue("background")
	c.Sidebar = r.FormValue("sidebar")
	c.Surface = r.FormValue("surface")
	c.Border = r.FormValue("border")
	c.Text = r.FormValue("text")
	c.Primary = r.FormValue("primary")
	c.Secondary = r.FormValue("secondary")
	next := cloneThemes(s.themes)
	next[id] = t
	target := "/settings/themes?edit=" + id
	if _, err := validateThemes(themeFile{SchemaVersion: 1, Themes: []Theme{t}}); err != nil {
		setFlash(w, "Theme not saved: "+strings.TrimPrefix(err.Error(), fmt.Sprintf("theme %q: ", id)), true)
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	if err := s.persistThemes(next); err != nil {
		s.log.Println("save theme:", err)
		http.Error(w, "could not save the theme", http.StatusInternalServerError)
		return
	}
	setFlash(w, "Theme saved.", false)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// themeCSS renders every built-in palette and the given custom themes as CSS
// custom properties. Custom colors are validated #RRGGBB strings and IDs are
// constrained to ASCII slug characters, so the output needs no escaping.
func themeCSS(custom map[string]Theme) string {
	var b strings.Builder
	for i, t := range builtinThemes {
		selector := "body." + t.ID
		if i == 0 {
			selector = ":root," + selector
		}
		writeTokens(&b, selector, t.Tokens)
	}
	for _, t := range sortedThemes(custom) {
		writeTokens(&b, "body.theme-"+t.ID, customTokens(t))
	}
	return b.String()
}
func writeTokens(b *strings.Builder, selector string, tokens map[string]string) {
	b.WriteString(selector)
	b.WriteString("{")
	for _, k := range themeTokens {
		fmt.Fprintf(b, "--%s:%s;", k, tokens[k])
	}
	b.WriteString("}\n")
}

// customTokens overlays a custom theme's seven colors on its base palette.
func customTokens(t Theme) map[string]string {
	base, _ := findBuiltin(t.Base)
	tokens := make(map[string]string, len(base.Tokens))
	for k, v := range base.Tokens {
		tokens[k] = v
	}
	c := t.Colors
	for k, v := range map[string]string{"bg": c.Background, "sidebar": c.Sidebar, "surface": c.Surface, "line": c.Border, "text": c.Text, "accent": c.Primary, "secondary": c.Secondary, "input": c.Sidebar, "note-bg": c.Surface, "focus": c.Primary} {
		tokens[k] = v
	}
	return tokens
}

func (s *Server) themesStylesheet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	css := themeCSS(s.themes)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(css))
	}
}

func baseColors(id string) ThemeColors {
	t, _ := findBuiltin(id)
	m := t.Tokens
	return ThemeColors{m["bg"], m["sidebar"], m["surface"], m["line"], m["text"], m["accent"], m["secondary"]}
}

func bodyTheme(id string) string {
	if validTheme(id) {
		return id
	}
	return "theme-" + id
}
