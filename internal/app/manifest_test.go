package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gkuba/ReelAnvil/internal/pipeline"
	"github.com/gkuba/ReelAnvil/internal/store"
)

func TestWriteManifestBesideClip(t *testing.T) {
	out := filepath.Join(t.TempDir(), "clip-1-intro.mp4")
	c := store.Candidate{StartMS: 1000, EndMS: 31000, Title: "Intro", Model: "m", Attempt: 1}
	if err := writeManifest(out, store.Source{Path: "incoming/a.mp4"}, c, "burned", edit{keep: []pipeline.Span{{StartMS: 1000, EndMS: 11000}, {StartMS: 12000, EndMS: 31000}}, length: 29000, title: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(out), "clip-1-intro.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m clipManifest
	if err := json.Unmarshal(b, &m); err != nil || m.Source != "incoming/a.mp4" || m.EndMS != 31000 || m.Captions != "burned" || m.Output != "clip-1-intro.mp4" {
		t.Fatalf("manifest %+v, %v", m, err)
	}
}
