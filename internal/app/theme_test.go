package app

import (
	"strings"
	"testing"
)

func TestThemePaletteTokens(t *testing.T) {
	source := pageTemplate.Tree.Root.String()
	for _, theme := range []string{":root", "body.dracula-dark", "body.tomorrow-night", "body.tokyo-night", "body.nocturne", "body.foundry"} {
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
	if !strings.Contains(source, "body.dracula-dark{--bg:#101116") {
		t.Error("Dracula background must be dark charcoal")
	}
}
