package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderOptionsDefaultOnForOldSettingsAndStayOffWhenSaved(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"version":5,"instance_name":"X","theme":"aurora","icon_variant":"amethyst"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil || !c.TrimSilences || !c.WordCaptions || !c.TitleCard {
		t.Fatalf("old settings: %+v, %v", c, err)
	}
	c.TrimSilences, c.TitleCard = false, false
	if err := save(p, c); err != nil {
		t.Fatal(err)
	}
	again, err := load(p)
	if err != nil || again.TrimSilences || again.TitleCard || !again.WordCaptions {
		t.Fatalf("saved off values lost: %+v, %v", again, err)
	}
}
