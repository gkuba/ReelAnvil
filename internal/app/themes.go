package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var themeID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

const themePath = "settings/themes.json"
const themeBackup = "settings/themes.json.bak"

func builtinOptions() []Theme {
	return []Theme{{ID: "aurora", Name: "Aurora"}, {ID: "dracula-dark", Name: "Dracula"}, {ID: "tomorrow-night", Name: "Tomorrow Night"}, {ID: "nocturne", Name: "Nocturne"}, {ID: "foundry", Name: "Foundry"}}
}
func sortedThemes(m map[string]Theme) []Theme {
	out := make([]Theme, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func colorFields(t Theme) []ColorField {
	c := t.Colors
	return []ColorField{{"background", "Background", c.Background}, {"sidebar", "Sidebar", c.Sidebar}, {"surface", "Panels / Tiles", c.Surface}, {"border", "Borders", c.Border}, {"text", "Text", c.Text}, {"primary", "Primary Accent", c.Primary}, {"secondary", "Secondary Accent", c.Secondary}}
}
func validateThemes(f themeFile) (map[string]Theme, error) {
	if f.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported theme schema version %d", f.SchemaVersion)
	}
	m := make(map[string]Theme)
	for i, t := range f.Themes {
		// Preserve custom themes based on the retired Tokyo Night palette.
		if t.Base == "tokyo-night" {
			t.Base = "aurora"
		}
		if t.Base == "dracula-lavender" {
			t.Base = "dracula-dark"
		}
		if !themeID.MatchString(t.ID) || validTheme(t.ID) {
			return nil, fmt.Errorf("theme %d: invalid or reserved ID %q", i+1, t.ID)
		}
		if strings.TrimSpace(t.Name) == "" || len(t.Name) > 64 || strings.ContainsAny(t.Name, "\r\n") {
			return nil, fmt.Errorf("theme %q: name must be 1–64 characters", t.ID)
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
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".themes-*")
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
	m, err := parseThemeFile(themePath)
	if errors.Is(err, os.ErrNotExist) {
		// Restore a valid backup if the primary was removed. Do not replace an
		// invalid backup with empty data, since it may be the only recovery file.
		if recovered, backupErr := parseThemeFile(themeBackup); backupErr == nil {
			b, e := serializeThemes(recovered)
			if e == nil {
				e = writeAtomic(themePath, b)
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
		if e := writeAtomic(themePath, b); e != nil {
			s.themeError = e.Error()
			s.log.Println("theme initialization:", e)
			return
		}
		if e := writeAtomic(themeBackup, b); e != nil {
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
		b, e := os.ReadFile(themePath)
		if e == nil {
			e = writeAtomic(themeBackup, b)
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
	if recovered, e := parseThemeFile(themeBackup); e == nil {
		s.themes = recovered
		s.log.Println("using last known-good theme backup")
	} else {
		s.themes = map[string]Theme{}
		s.log.Println("theme backup unavailable or invalid:", e)
	}
}
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
	if err = writeAtomic(themePath, b); err != nil {
		return err
	}
	if err = writeAtomic(themeBackup, b); err != nil {
		s.themeError = fmt.Sprintf("saved primary, but backup failed: %v; theme saving locked", err)
		return err
	}
	s.themes = m
	return nil
}
func postOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return false
	}
	return true
}
func (s *Server) refreshThemes(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	s.reloadThemes()
	http.Redirect(w, r, "/settings/themes", 303)
}
func (s *Server) duplicateTheme(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	source := r.FormValue("source")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.themeError != "" {
		http.Error(w, s.themeError, 409)
		return
	}
	var base string
	var c ThemeColors
	var name string
	if t, ok := s.themes[source]; ok {
		base = t.Base
		c = t.Colors
		name = t.Name
	} else if validTheme(source) {
		base = source
		name = strings.ReplaceAll(source, "-", " ")
		c = baseColors(source)
	} else {
		http.Error(w, "unknown theme", 400)
		return
	}
	id := source + "-copy"
	if len(id) > 40 {
		id = id[:40]
	}
	for n := 2; s.themes[id].ID != ""; n++ {
		id = fmt.Sprintf("%s-copy-%d", source, n)
		if len(id) > 48 {
			http.Error(w, "theme ID too long", 400)
			return
		}
	}
	t := Theme{ID: id, Name: name + " (Copy)", Base: base, Colors: c}
	next := cloneThemes(s.themes)
	next[id] = t
	nextSettings := s.cfg
	nextSettings.ThemeIcons = cloneThemeIcons(s.cfg.ThemeIcons)
	variant := nextSettings.ThemeIcons[source]
	if !validIconVariant(variant) {
		variant = "blue-violet"
	}
	nextSettings.ThemeIcons[id] = variant
	nextSettings.Version = 2
	if err := save(s.cfgPath, nextSettings); err != nil {
		s.log.Println("custom theme icon default save failed:", err)
		http.Error(w, "could not save the new theme icon default", http.StatusInternalServerError)
		return
	}
	if err := s.persistThemes(next); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.cfg = nextSettings
	http.Redirect(w, r, "/settings/themes?edit="+id+"&message=Theme+duplicated", 303)
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
		http.Error(w, "invalid form", 400)
		return
	}
	id := r.FormValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.themeError != "" {
		http.Error(w, s.themeError, 409)
		return
	}
	t, ok := s.themes[id]
	if !ok {
		http.Error(w, "unknown custom theme", 400)
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
	if err := s.persistThemes(next); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/settings/themes?edit="+id+"&message=Theme+saved", 303)
}

// Values are validated #RRGGBB strings, IDs are constrained to ASCII slug characters.
func themeCSS(themes map[string]Theme) template.CSS {
	var b strings.Builder
	for _, t := range sortedThemes(themes) {
		base := baseTokens(t.Base)
		c := t.Colors
		for k, v := range map[string]string{"bg": c.Background, "sidebar": c.Sidebar, "surface": c.Surface, "line": c.Border, "text": c.Text, "accent": c.Primary, "secondary": c.Secondary, "tile-blue": c.Surface, "tile-teal": c.Sidebar, "tile-purple": c.Surface, "tile-blue-line": c.Border, "tile-teal-line": c.Border, "tile-purple-line": c.Border, "input": c.Sidebar, "nav-active": c.Primary, "note-bg": c.Surface, "focus": c.Primary} {
			base[k] = v
		}
		b.WriteString("body.theme-")
		b.WriteString(t.ID)
		b.WriteString("{")
		keys := make([]string, 0, len(base))
		for k := range base {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("--")
			b.WriteString(k)
			b.WriteString(":")
			b.WriteString(base[k])
			b.WriteString(";")
		}
		b.WriteString("}")
	}
	return template.CSS(b.String())
}

// Keep built-in source CSS as the authority for all derived token defaults.
func baseTokens(id string) map[string]string {
	source := pageTemplate.Tree.Root.String()
	prefix := ":root{"
	if id != "aurora" {
		prefix = "body." + id + "{"
	}
	start := strings.Index(source, prefix)
	if start < 0 {
		return map[string]string{}
	}
	chunk := source[start+len(prefix):]
	chunk = chunk[:strings.Index(chunk, "}")]
	m := map[string]string{}
	for _, part := range strings.Split(chunk, ";") {
		parts := strings.SplitN(part, ":", 2)
		if len(parts) == 2 {
			m[strings.TrimPrefix(parts[0], "--")] = parts[1]
		}
	}
	return m
}
func baseColors(id string) ThemeColors {
	m := baseTokens(id)
	return ThemeColors{m["bg"], m["sidebar"], m["surface"], m["line"], m["text"], m["accent"], m["secondary"]}
}

func bodyTheme(id string) string {
	if validTheme(id) {
		return id
	}
	return "theme-" + id
}
