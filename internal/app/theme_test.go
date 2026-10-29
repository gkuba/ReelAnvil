package app

import (
	"io/fs"
	"strings"
	"testing"
)

func readAsset(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(webAssets, name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestThemePaletteTokens(t *testing.T) {
	for _, theme := range builtinThemes {
		for _, token := range themeTokens {
			if !hexColor.MatchString(theme.Tokens[token]) {
				t.Errorf("%s: token %s = %q, want #RRGGBB", theme.ID, token, theme.Tokens[token])
			}
		}
		if len(theme.Tokens) != len(themeTokens) {
			t.Errorf("%s defines %d tokens, want %d", theme.ID, len(theme.Tokens), len(themeTokens))
		}
	}
	// Build 0.7 consolidated Dracula on the blue-black Lavender surfaces.
	if c := baseColors("dracula-dark"); c.Background != "#10131f" || c.Surface != "#131722" {
		t.Errorf("Dracula surfaces = %s / %s", c.Background, c.Surface)
	}
	css := themeCSS(nil)
	if !strings.HasPrefix(css, ":root,body.aurora{") {
		t.Errorf("Aurora must be the :root fallback: %.40q", css)
	}
}

// Every CSS custom property the stylesheet uses must be defined by the themes.
func TestStylesheetUsesOnlyThemeTokens(t *testing.T) {
	css := readAsset(t, "web/static/app.css")
	defined := map[string]bool{}
	for _, k := range themeTokens {
		defined[k] = true
	}
	for _, part := range strings.Split(css, "var(--")[1:] {
		name := part[:strings.IndexAny(part, ")")]
		if !defined[name] {
			t.Errorf("app.css uses undefined token --%s", name)
		}
	}
}

func TestSettingsNavigationUsesInlineSVGIcons(t *testing.T) {
	source := readAsset(t, "web/templates/sidebar.html") + readAsset(t, "web/templates/themes.html") + readAsset(t, "web/templates/general.html")
	if got := strings.Count(source, `class="rail-icon"`); got < 15 {
		t.Fatalf("navigation has %d inline icons, want at least 15", got)
	}
	if strings.Contains(source, "⚙ Settings") || strings.Contains(source, "☷ General") || strings.Contains(source, "◈ Themes") || strings.Contains(source, "♧ Connectors") || strings.Contains(source, "▣ Paths") {
		t.Fatal("temporary Unicode settings icons remain")
	}
}

// The CSP forbids inline scripts and styles, so templates must not use them.
func TestTemplatesAvoidInlineScriptsAndStyles(t *testing.T) {
	entries, err := fs.ReadDir(webAssets, "web/templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		src := readAsset(t, "web/templates/"+e.Name())
		for _, banned := range []string{"<style", "style=", "<script>", " onclick=", " onchange="} {
			if strings.Contains(src, banned) {
				t.Errorf("%s contains %q", e.Name(), banned)
			}
		}
	}
}

func TestDraculaConsolidatedAndNavigationContrast(t *testing.T) {
	options := builtinOptions()
	count := 0
	for _, theme := range options {
		if strings.HasPrefix(theme.ID, "dracula") {
			count++
			if theme.Name != "Dracula" {
				t.Fatalf("unexpected Dracula label %q", theme.Name)
			}
		}
	}
	if count != 1 {
		t.Fatalf("want one Dracula, got %d", count)
	}
	src := readAsset(t, "web/static/app.css") + themeCSS(nil)
	for _, needle := range []string{"#app-sidebar .rail-flyout a:hover", "#app-sidebar nav a.active", "color-mix(in srgb,var(--accent) 23%", "--tertiary:#ff79c6"} {
		if !strings.Contains(src, needle) {
			t.Errorf("missing nav or palette rule: %s", needle)
		}
	}
	c := baseColors("dracula-dark")
	if c.Primary != "#a897f5" {
		t.Fatalf("Dracula accent = %s", c.Primary)
	}
	custom := Theme{ID: "my-dracula", Name: "My Dracula", Base: "dracula-lavender", Colors: c}
	m, err := validateThemes(themeFile{SchemaVersion: 1, Themes: []Theme{custom}})
	if err != nil || m[custom.ID].Base != "dracula-dark" || m[custom.ID].Colors != c {
		t.Fatalf("custom base migration failed: %v %#v", err, m)
	}
}
