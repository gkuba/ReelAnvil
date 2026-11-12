package worker

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTestWorker(t *testing.T) *Worker {
	t.Helper()
	w, err := New(Config{Engine: "host", FFmpeg: "ffmpeg", YtDlp: "yt-dlp", MediaRoot: t.TempDir(), Log: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestPathChecks(t *testing.T) {
	w := newTestWorker(t)
	root := w.cfg.MediaRoot
	in := filepath.Join(root, "incoming", "a.mp4")
	if err := os.MkdirAll(filepath.Dir(in), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, []byte("x"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := w.checkInput(in); err != nil {
		t.Fatalf("plain input rejected: %v", err)
	}
	if err := w.checkOutput(in); err == nil {
		t.Fatal("output outside temp/ accepted")
	}
	for _, p := range []string{"/etc/passwd", root + "/../x", "temp/x", root + "/temp/../incoming/a.mp4"} {
		if err := w.checkInput(p); err == nil {
			t.Fatalf("%q accepted", p)
		}
	}
	// A symbolic link inside the root is refused, even to a file inside it.
	link := filepath.Join(root, "temp", "link")
	if err := os.Symlink(filepath.Join(root, "incoming"), link); err != nil {
		t.Fatal(err)
	}
	if err := w.checkInput(filepath.Join(link, "a.mp4")); err == nil {
		t.Fatal("input through a symlink accepted")
	}
	if err := w.checkOutput(filepath.Join(link, "new", "b.mp4")); err == nil {
		t.Fatal("output through a symlink accepted")
	}
	if err := checkURL("http://example.com/v"); err == nil {
		t.Fatal("http URL accepted")
	}
	if err := checkURL("https://user:pw@example.com/v"); err == nil {
		t.Fatal("URL with credentials accepted")
	}
}

// TestProbeAudioRender runs the real tools on a short generated clip.
func TestProbeAudioRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	w := newTestWorker(t)
	root := w.cfg.MediaRoot
	src := filepath.Join(root, "incoming", "test.mp4")
	_ = os.MkdirAll(filepath.Dir(src), 0750)
	// 6 s of 640x360 video; a tone for 2 s, silence for 2 s, a tone for 2 s.
	gen := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=6",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=6", "-af", "volume=enable='between(t,2,4)':volume=0",
		"-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v %s", err, out)
	}
	ctx := context.Background()
	op := func(kind string, args any) any {
		t.Helper()
		b, _ := json.Marshal(args)
		res, err := w.do(ctx, Op{Kind: kind, Args: b}, func(int64, int64, string) {})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		return res
	}
	p := op("probe", ProbeArgs{Path: src}).(ProbeResult)
	if p.Width != 640 || p.Height != 360 || !p.HasAudio || !p.HasVideo || p.DurationMS < 5900 || p.DurationMS > 6200 {
		t.Fatalf("probe = %+v", p)
	}
	pcm := filepath.Join(root, "temp", "1", "audio.s16le")
	a := op("audio", AudioArgs{In: src, Out: pcm, DurationMS: p.DurationMS}).(AudioResult)
	if fi, err := os.Stat(pcm); err != nil || fi.Size() < 6*16000*2*9/10 {
		t.Fatalf("audio file: %v %v", fi, err)
	}
	if len(a.Silences) != 1 || a.Silences[0].StartMS < 1800 || a.Silences[0].EndMS > 4200 {
		t.Fatalf("silences = %+v", a.Silences)
	}
	srt := filepath.Join(root, "temp", "1", "clip.srt")
	_ = os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,000\nHello there\n"), 0640)
	out := filepath.Join(root, "temp", "1", "clip.mp4")
	r := op("render", RenderArgs{In: src, Out: out, SRT: srt, StartMS: 1000, EndMS: 4000}).(RenderResult)
	if r.Captions != "burned" && r.Captions != "sidecar" {
		t.Fatalf("captions = %q", r.Captions)
	}
	v := op("probe", ProbeArgs{Path: out}).(ProbeResult)
	if v.Width != 1080 || v.Height != 1920 || v.DurationMS < 2900 || v.DurationMS > 3200 || !v.HasAudio {
		t.Fatalf("render probe = %+v", v)
	}
	// Silence scan of the whole source, then a render that joins the two tones.
	sil := op("silences", SilenceArgs{In: src, StartMS: 0, EndMS: 6000}).(AudioResult)
	if len(sil.Silences) != 1 || sil.Silences[0].StartMS < 1800 || sil.Silences[0].EndMS > 4200 {
		t.Fatalf("silences op = %+v", sil.Silences)
	}
	ass := filepath.Join(root, "temp", "1", "clip.ass")
	_ = os.WriteFile(ass, []byte("[Script Info]\nPlayResX: 1080\nPlayResY: 1920\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, OutlineColour, BackColour, Bold, Alignment, MarginV\nStyle: Default,Arial,80,&H00FFFFFF,&H00000000,&H00000000,1,2,400\n\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:00.00,0:00:01.50,Default,Hello\n"), 0640)
	cut := filepath.Join(root, "temp", "1", "cut.mp4")
	op("render", RenderArgs{In: src, Out: cut, SRT: srt, ASS: ass, StartMS: 0, EndMS: 6000, Keep: []Range{{0, 2200}, {3800, 6000}}})
	c := op("probe", ProbeArgs{Path: cut}).(ProbeResult)
	if c.Width != 1080 || c.Height != 1920 || c.DurationMS < 4200 || c.DurationMS > 4700 || !c.HasAudio {
		t.Fatalf("cut probe = %+v", c)
	}
	if _, err := w.do(ctx, Op{Kind: "render", Args: []byte(`{"in":"` + src + `","out":"` + cut + `","srt":"` + srt + `","start_ms":0,"end_ms":6000,"keep":[{"start_ms":0,"end_ms":9000}]}`)}, nil); err == nil {
		t.Fatal("piece outside the clip accepted")
	}
	if _, err := w.do(ctx, Op{Kind: "shell", Args: []byte("{}")}, nil); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown kind: %v", err)
	}
}
