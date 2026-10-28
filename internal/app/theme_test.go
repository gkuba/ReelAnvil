package app

import (
	"strings"
	"testing"
)

func TestThemePaletteTokens(t *testing.T) {
	source := pageTemplate.Tree.Root.String()
	for _, theme := range []string{":root", "body.dracula-dark", "body.tomorrow-night", "body.nocturne", "body.foundry"} {
		start := strings.Index(source, theme+"{")
		if start < 0 {
			t.Fatalf("missing palette %s", theme)
		}
		end := strings.Index(source[start:], "}")
		if end < 0 {
			t.Fatalf("unterminated palette %s", theme)
		}
		palette := source[start : start+end]
		for _, token := range []string{"--bg:", "--sidebar:", "--surface:", "--secondary:", "--tertiary:", "--warm:", "--tile-blue:", "--tile-teal:", "--tile-purple:"} {
			if !strings.Contains(palette, token) {
				t.Errorf("%s missing %s", theme, token)
			}
		}
	}
	if !strings.Contains(source, "body.dracula-dark{--bg:#17181d") {
		t.Error("Dracula background must be dark charcoal")
	}
}

func TestSettingsNavigationUsesInlineSVGIcons(t *testing.T) {
	source := pageTemplate.Tree.Root.String() + sidebarTemplateSource
	if got := strings.Count(source, `class="rail-icon"`); got < 15 {
		t.Fatalf("navigation has %d inline icons, want at least 15", got)
	}
	if strings.Contains(source, "⚙ Settings") || strings.Contains(source, "☷ General") || strings.Contains(source, "◈ Themes") || strings.Contains(source, "♧ Connectors") || strings.Contains(source, "▣ Paths") {
		t.Fatal("temporary Unicode settings icons remain")
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
	src := pageTemplate.Tree.Root.String() + sidebarCSS
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
