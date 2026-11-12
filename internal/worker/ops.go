package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Operation arguments and results, shared with the server.
type (
	DownloadArgs struct {
		URL string `json:"url"`
		Dir string `json:"dir"`
	}
	DownloadResult struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	ProbeArgs struct {
		Path string `json:"path"`
	}
	ProbeResult struct {
		DurationMS int64 `json:"duration_ms"`
		Width      int   `json:"width"`
		Height     int   `json:"height"`
		HasVideo   bool  `json:"has_video"`
		HasAudio   bool  `json:"has_audio"`
		Size       int64 `json:"size"`
	}
	AudioArgs struct {
		In         string `json:"in"`
		Out        string `json:"out"`
		DurationMS int64  `json:"duration_ms"`
	}
	Silence struct {
		StartMS int64 `json:"start_ms"`
		EndMS   int64 `json:"end_ms"`
	}
	AudioResult struct {
		Silences []Silence `json:"silences"`
	}
	// SilenceArgs scans In from StartMS to EndMS; the result is in source time.
	SilenceArgs struct {
		In      string `json:"in"`
		StartMS int64  `json:"start_ms"`
		EndMS   int64  `json:"end_ms"`
	}
	// Range is a part of the clip to keep, in milliseconds from the clip's start.
	Range struct {
		StartMS int64 `json:"start_ms"`
		EndMS   int64 `json:"end_ms"`
	}
	RenderArgs struct {
		In  string `json:"in"`
		Out string `json:"out"`
		SRT string `json:"srt"`
		// ASS, when set, is burned instead of the SRT (the SRT stays a sidecar).
		ASS     string `json:"ass,omitempty"`
		StartMS int64  `json:"start_ms"`
		EndMS   int64  `json:"end_ms"`
		// Keep lists the parts to join, relative to StartMS. Empty keeps it all.
		Keep []Range `json:"keep,omitempty"`
	}
	RenderResult struct {
		Captions string `json:"captions"` // burned or sidecar
	}
)

// executor builds the command line for one tool.
type executor interface {
	command(ctx context.Context, tool string, args ...string) (*exec.Cmd, error)
	// stop ends a tool still running after its command was killed.
	stop(marker string)
}

type hostExec struct{ ffmpeg, ytdlp string }

func (h hostExec) command(ctx context.Context, tool string, args ...string) (*exec.Cmd, error) {
	bin := map[string]string{"ffmpeg": h.ffmpeg, "yt-dlp": h.ytdlp}[tool]
	if tool == "ffprobe" {
		// ffprobe sits next to the configured ffmpeg.
		bin = "ffprobe"
		if strings.Contains(h.ffmpeg, "/") {
			bin = filepath.Join(filepath.Dir(h.ffmpeg), "ffprobe")
		}
	}
	if bin == "" {
		return nil, fmt.Errorf("unknown tool %s", tool)
	}
	return exec.CommandContext(ctx, bin, args...), nil
}

func (hostExec) stop(string) {}

// containerExec runs tools with "podman exec" (or docker) in the compose
// service named media.
type containerExec struct {
	engine string
	mu     sync.Mutex
	name   string
}

func (c *containerExec) container() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name != "" {
		return c.name, nil
	}
	out, err := exec.Command(c.engine, "ps", "--filter", "label=com.docker.compose.service=media", "--format", "{{.Names}}").Output()
	if err != nil {
		return "", fmt.Errorf("%s ps: %v", c.engine, err)
	}
	name := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if name == "" {
		return "", fmt.Errorf("the media container is not running")
	}
	c.name = name
	return name, nil
}

func (c *containerExec) command(ctx context.Context, tool string, args ...string) (*exec.Cmd, error) {
	if tool != "ffmpeg" && tool != "ffprobe" && tool != "yt-dlp" {
		return nil, fmt.Errorf("unknown tool %s", tool)
	}
	name, err := c.container()
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, c.engine, append([]string{"exec", name, tool}, args...)...), nil
}

// stop kills the tool inside the container: killing the exec client does not.
func (c *containerExec) stop(marker string) {
	c.mu.Lock()
	name := c.name
	c.mu.Unlock()
	if name != "" && marker != "" {
		_ = exec.Command(c.engine, "exec", name, "pkill", "-KILL", "-f", "--", marker).Run()
	}
}

// forget drops the cached container name after a failure, so a recreated
// container is found again.
func (w *Worker) forget() {
	if c, ok := w.exec.(*containerExec); ok {
		c.mu.Lock()
		c.name = ""
		c.mu.Unlock()
	}
}

// run starts tool and feeds its stdout and stderr lines to the callbacks. The
// error carries the last stderr lines.
func (w *Worker) run(ctx context.Context, tool string, args []string, marker string, stdout, stderr func(string)) error {
	cmd, err := w.exec.command(ctx, tool, args...)
	if err != nil {
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		w.forget()
		return err
	}
	var tail []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	scan := func(r io.Reader, fn func(string), keep bool) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		sc.Split(scanLines)
		for sc.Scan() {
			line := sc.Text()
			if fn != nil {
				fn(line)
			}
			if keep && strings.TrimSpace(line) != "" {
				mu.Lock()
				tail = append(tail, line)
				if len(tail) > 8 {
					tail = tail[1:]
				}
				mu.Unlock()
			}
		}
	}
	wg.Add(2)
	go scan(outPipe, stdout, false)
	go scan(errPipe, stderr, true)
	wg.Wait()
	err = cmd.Wait()
	if ctx.Err() != nil {
		w.exec.stop(marker)
		return ctx.Err()
	}
	if err != nil {
		w.forget()
		return fmt.Errorf("%s: %v: %s", tool, err, strings.Join(tail, " | "))
	}
	return nil
}

// scanLines splits on \n and \r, since tools redraw progress with \r.
func scanLines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(raw) > 2000 {
		return fmt.Errorf("only https:// URLs can be imported")
	}
	return nil
}

func (w *Worker) download(ctx context.Context, a DownloadArgs, progress progressFunc) (DownloadResult, error) {
	var res DownloadResult
	if err := checkURL(a.URL); err != nil {
		return res, err
	}
	if err := w.checkOutput(filepath.Join(a.Dir, "source")); err != nil {
		return res, err
	}
	args := []string{"--ignore-config", "--no-playlist", "--restrict-filenames", "--no-mtime", "--newline", "--progress",
		"--progress-template", "download:RAPROG %(progress.downloaded_bytes)s %(progress.total_bytes)s %(progress.total_bytes_estimate)s",
		"-f", "bv*[height<=1080]+ba/b[height<=1080]/bv*+ba/b", "--merge-output-format", "mp4",
		"--print", "after_move:RAPATH %(filepath)s", "--print", "after_move:RATITLE %(title)s",
		"-o", filepath.Join(a.Dir, "source.%(ext)s"), "--", a.URL}
	err := w.run(ctx, "yt-dlp", args, a.Dir, func(line string) {
		switch {
		case strings.HasPrefix(line, "RAPROG "):
			f := strings.Fields(line)
			done, _ := strconv.ParseInt(f[1], 10, 64)
			total, _ := strconv.ParseInt(f[2], 10, 64)
			if total == 0 && len(f) > 3 {
				est, _ := strconv.ParseFloat(f[3], 64)
				total = int64(est)
			}
			progress(done, total, "Downloading")
		case strings.HasPrefix(line, "RAPATH "):
			res.Path = strings.TrimPrefix(line, "RAPATH ")
		case strings.HasPrefix(line, "RATITLE "):
			res.Title = strings.TrimPrefix(line, "RATITLE ")
		}
	}, nil)
	if err != nil {
		return res, err
	}
	if res.Path == "" || filepath.Dir(res.Path) != a.Dir {
		return res, fmt.Errorf("yt-dlp did not report the downloaded file")
	}
	return res, w.checkInput(res.Path)
}

func (w *Worker) probe(ctx context.Context, path string) (ProbeResult, error) {
	var res ProbeResult
	var out strings.Builder
	err := w.run(ctx, "ffprobe", []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "--", path}, "",
		func(l string) { out.WriteString(l); out.WriteByte('\n') }, nil)
	if err != nil {
		return res, err
	}
	var p struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
		} `json:"format"`
	}
	if err = json.Unmarshal([]byte(out.String()), &p); err != nil {
		return res, fmt.Errorf("ffprobe output: %v", err)
	}
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if !res.HasVideo {
				res.HasVideo, res.Width, res.Height = true, s.Width, s.Height
			}
		case "audio":
			res.HasAudio = true
		}
	}
	d, _ := strconv.ParseFloat(p.Format.Duration, 64)
	res.DurationMS = int64(d * 1000)
	res.Size, _ = strconv.ParseInt(p.Format.Size, 10, 64)
	return res, nil
}

// progressTime reads "out_time_us=" lines from ffmpeg -progress.
func progressTime(line string) (int64, bool) {
	v, ok := strings.CutPrefix(line, "out_time_us=")
	if !ok {
		return 0, false
	}
	us, err := strconv.ParseInt(v, 10, 64)
	return us / 1000, err == nil && us >= 0
}

func (w *Worker) audio(ctx context.Context, a AudioArgs, progress progressFunc) (AudioResult, error) {
	var res AudioResult
	if err := w.checkInput(a.In); err != nil {
		return res, err
	}
	if err := w.checkOutput(a.Out); err != nil {
		return res, err
	}
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-y", "-i", a.In, "-vn", "-map", "0:a:0", "-ac", "1", "-ar", "16000",
		"-af", "silencedetect=noise=-30dB:d=0.35", "-f", "s16le", "-progress", "pipe:1", a.Out}
	open := int64(-1)
	err := w.run(ctx, "ffmpeg", args, a.Out, func(line string) {
		if ms, ok := progressTime(line); ok {
			progress(min(ms, a.DurationMS), a.DurationMS, "Extracting audio")
		}
	}, func(line string) {
		if i := strings.Index(line, "silence_start: "); i >= 0 {
			if v, err := strconv.ParseFloat(strings.Fields(line[i+15:])[0], 64); err == nil {
				open = int64(v * 1000)
			}
		} else if i := strings.Index(line, "silence_end: "); i >= 0 && open >= 0 {
			if v, err := strconv.ParseFloat(strings.Fields(line[i+13:])[0], 64); err == nil {
				res.Silences = append(res.Silences, Silence{open, int64(v * 1000)})
				open = -1
			}
		}
	})
	if err != nil {
		return res, err
	}
	if open >= 0 {
		res.Silences = append(res.Silences, Silence{open, a.DurationMS})
	}
	return res, nil
}

// MaxKeepRanges bounds how many pieces a clip is joined from.
const MaxKeepRanges = 60

// silences finds pauses inside one part of a source, for cutting.
func (w *Worker) silences(ctx context.Context, a SilenceArgs) (AudioResult, error) {
	var res AudioResult
	if err := w.checkInput(a.In); err != nil {
		return res, err
	}
	if a.EndMS <= a.StartMS {
		return res, fmt.Errorf("empty range")
	}
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-ss", ms(a.StartMS), "-t", ms(a.EndMS - a.StartMS), "-i", a.In,
		"-vn", "-af", "silencedetect=noise=-32dB:d=0.6", "-f", "null", "-"}
	open := int64(-1)
	err := w.run(ctx, "ffmpeg", args, a.In, nil, func(line string) {
		if i := strings.Index(line, "silence_start: "); i >= 0 {
			if v, err := strconv.ParseFloat(strings.Fields(line[i+15:])[0], 64); err == nil {
				open = a.StartMS + int64(v*1000)
			}
		} else if i := strings.Index(line, "silence_end: "); i >= 0 && open >= 0 {
			if v, err := strconv.ParseFloat(strings.Fields(line[i+13:])[0], 64); err == nil {
				res.Silences = append(res.Silences, Silence{open, a.StartMS + int64(v*1000)})
				open = -1
			}
		}
	})
	if err != nil {
		return res, err
	}
	if open >= 0 {
		res.Silences = append(res.Silences, Silence{open, a.EndMS})
	}
	return res, nil
}

// filterSafe rejects characters that would need FFmpeg filter escaping.
func filterSafe(p string) bool { return !strings.ContainsAny(p, "'\\:,;[]=") }

// VerticalFilter is the 1080x1920 layout: the source scaled to the width,
// centred over a blurred, cropped copy of itself.
const VerticalFilter = "[0:v]split[a][b];[a]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,gblur=sigma=40[bg];" +
	"[b]scale=1080:-2[fg];[bg][fg]overlay=(W-w)/2:(H-h)/2,format=yuv420p"

// CaptionStyle is the libass style for burned captions (SRT play height 288).
const CaptionStyle = "Fontsize=13,Bold=1,Outline=2,Shadow=0,Alignment=2,MarginV=70,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000"

func (w *Worker) render(ctx context.Context, a RenderArgs, progress progressFunc) (RenderResult, error) {
	res := RenderResult{Captions: "sidecar"}
	if err := w.checkInput(a.In); err != nil {
		return res, err
	}
	if err := w.checkInput(a.SRT); err != nil {
		return res, err
	}
	if a.ASS != "" {
		if err := w.checkInput(a.ASS); err != nil {
			return res, err
		}
	}
	if err := w.checkOutput(a.Out); err != nil {
		return res, err
	}
	if a.EndMS <= a.StartMS {
		return res, fmt.Errorf("empty clip")
	}
	dur := a.EndMS - a.StartMS
	var graph, audio string
	vertical := VerticalFilter
	if n := len(a.Keep); n > 0 {
		if n > MaxKeepRanges {
			return res, fmt.Errorf("too many pieces to join")
		}
		var sb strings.Builder
		var pads strings.Builder
		dur = 0
		for i, k := range a.Keep {
			if k.StartMS < 0 || k.EndMS <= k.StartMS || k.EndMS > a.EndMS-a.StartMS {
				return res, fmt.Errorf("bad piece %d", i+1)
			}
			dur += k.EndMS - k.StartMS
			fmt.Fprintf(&sb, "[0:v]trim=start=%s:end=%s,setpts=PTS-STARTPTS[v%d];[0:a]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS[a%d];",
				ms(k.StartMS), ms(k.EndMS), i, ms(k.StartMS), ms(k.EndMS), i)
			fmt.Fprintf(&pads, "[v%d][a%d]", i, i)
		}
		graph = sb.String() + pads.String() + fmt.Sprintf("concat=n=%d:v=1:a=1[cv][ca];", n)
		vertical = strings.Replace(vertical, "[0:v]", "[cv]", 1)
		audio = "[ca]"
	}
	graph += vertical
	if fonts := w.prepareFonts(); fonts != "" && filterSafe(a.SRT) && filterSafe(fonts) {
		switch {
		case a.ASS != "" && filterSafe(a.ASS):
			graph += fmt.Sprintf(",subtitles=filename='%s':fontsdir='%s'", a.ASS, fonts)
			res.Captions = "burned"
		case a.ASS == "":
			graph += fmt.Sprintf(",subtitles=filename='%s':fontsdir='%s':force_style='%s'", a.SRT, fonts, CaptionStyle)
			res.Captions = "burned"
		}
	}
	graph += "[v]"
	if audio == "" {
		audio = "0:a:0?"
	}
	args := []string{"-nostdin", "-hide_banner", "-nostats", "-y",
		"-ss", ms(a.StartMS), "-t", ms(a.EndMS - a.StartMS), "-i", a.In,
		"-filter_complex", graph, "-map", "[v]", "-map", audio,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", "-progress", "pipe:1", a.Out}
	err := w.run(ctx, "ffmpeg", args, a.Out, func(line string) {
		if t, ok := progressTime(line); ok {
			progress(min(t, dur), dur, "Rendering")
		}
	}, nil)
	if err != nil {
		_ = os.Remove(a.Out)
	}
	return res, err
}

func ms(v int64) string { return fmt.Sprintf("%d.%03d", v/1000, v%1000) }
