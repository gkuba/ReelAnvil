package install

import (
	"os"
	"regexp"
	"testing"
)

// The Compose file pins Ollama and Whisper by digest; only the media image
// may follow a tag, so its yt-dlp stays current.
func TestComposePinsOllamaAndWhisperByDigest(t *testing.T) {
	b, err := os.ReadFile("assets/compose.yaml.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"ollama/ollama", "linuxserver/faster-whisper"} {
		if !regexp.MustCompile(`image: docker\.io/` + image + `@sha256:[0-9a-f]{64}\b`).Match(b) {
			t.Errorf("%s is not pinned by digest", image)
		}
	}
}
