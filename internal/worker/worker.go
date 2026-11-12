// Package worker runs media operations for the web app: downloads, probes,
// audio extraction and renders. It claims operations over the server's
// loopback worker API and runs only fixed FFmpeg, ffprobe and yt-dlp command
// lines, never a shell or a command sent by the server.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config is what the worker needs from the server's settings.
type Config struct {
	Server    string // host:port of the web app, loopback
	Token     string
	Engine    string // podman, docker or host
	FFmpeg    string // host engine only
	YtDlp     string // host engine only
	MediaRoot string
	Log       *log.Logger
}

// Worker claims and runs operations until its context ends.
type Worker struct {
	cfg      Config
	http     *http.Client
	root     string // MediaRoot with symlinks resolved
	exec     executor
	mu       sync.Mutex
	fontsDir string
}

func New(cfg Config) (*Worker, error) {
	if !filepath.IsAbs(cfg.MediaRoot) {
		return nil, fmt.Errorf("media root %q is not absolute", cfg.MediaRoot)
	}
	cfg.MediaRoot = filepath.Clean(cfg.MediaRoot)
	if err := os.MkdirAll(filepath.Join(cfg.MediaRoot, "temp"), 0750); err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(cfg.MediaRoot)
	if err != nil {
		return nil, err
	}
	w := &Worker{cfg: cfg, root: root, http: &http.Client{Timeout: 30 * time.Second}}
	switch cfg.Engine {
	case "host":
		w.exec = hostExec{ffmpeg: cfg.FFmpeg, ytdlp: cfg.YtDlp}
	case "podman", "docker":
		w.exec = &containerExec{engine: cfg.Engine}
	default:
		return nil, fmt.Errorf("unknown media engine %q", cfg.Engine)
	}
	return w, nil
}

// Run claims operations until ctx ends.
func (w *Worker) Run(ctx context.Context) error {
	w.cfg.Log.Printf("worker started: engine %s, media root %s, server %s", w.cfg.Engine, w.cfg.MediaRoot, w.cfg.Server)
	w.prepareFonts()
	idle := time.Second
	for {
		op, err := w.claim(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			w.cfg.Log.Printf("claim: %v", err)
			idle = min(idle*2, 30*time.Second)
		} else if op != nil {
			idle = time.Second
			w.runOp(ctx, *op)
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(idle):
		}
	}
}

// Op is a claimed operation.
type Op struct {
	ID   int64           `json:"id"`
	Kind string          `json:"kind"`
	Args json.RawMessage `json:"args"`
}

func (w *Worker) post(ctx context.Context, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+w.cfg.Server+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	return w.http.Do(req)
}

func (w *Worker) claim(ctx context.Context) (*Op, error) {
	resp, err := w.post(ctx, "/api/worker/claim", struct{}{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var op Op
		return &op, json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&op)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return nil, fmt.Errorf("server answered %s: %s", resp.Status, strings.TrimSpace(string(b)))
}

// reporter sends progress at most once a second and a heartbeat every 15 s.
// A 409 answer cancels the operation.
type reporter struct {
	w      *Worker
	id     int64
	cancel context.CancelFunc
	mu     sync.Mutex
	done   int64
	total  int64
	detail string
	dirty  bool
}

func (r *reporter) set(done, total int64, detail string) {
	r.mu.Lock()
	r.done, r.total, r.detail, r.dirty = done, total, detail, true
	r.mu.Unlock()
}

func (r *reporter) loop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		r.mu.Lock()
		send := r.dirty || time.Since(last) > 15*time.Second
		body := map[string]any{"done": r.done, "total": r.total, "detail": r.detail}
		r.dirty = false
		r.mu.Unlock()
		if !send {
			continue
		}
		last = time.Now()
		resp, err := r.w.post(ctx, fmt.Sprintf("/api/worker/ops/%d/progress", r.id), body)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			r.w.cfg.Log.Printf("op %d cancelled by the server", r.id)
			r.cancel()
			return
		}
	}
}

func (w *Worker) runOp(parent context.Context, op Op) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	rep := &reporter{w: w, id: op.ID, cancel: cancel}
	go rep.loop(ctx)
	start := time.Now()
	result, err := w.do(ctx, op, rep.set)
	body := map[string]any{"ok": err == nil, "result": result}
	if err != nil {
		if ctx.Err() != nil && parent.Err() == nil {
			err = errors.New("cancelled")
		}
		body["error"] = err.Error()
		w.cfg.Log.Printf("op %d %s failed after %s: %v", op.ID, op.Kind, time.Since(start).Round(time.Millisecond), err)
	} else {
		w.cfg.Log.Printf("op %d %s done in %s", op.ID, op.Kind, time.Since(start).Round(time.Millisecond))
	}
	// Report even when the operation was cancelled; use a fresh context.
	fctx, fcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer fcancel()
	if resp, err := w.post(fctx, fmt.Sprintf("/api/worker/ops/%d/finish", op.ID), body); err == nil {
		resp.Body.Close()
	} else {
		w.cfg.Log.Printf("op %d: could not report the result: %v", op.ID, err)
	}
}

type progressFunc func(done, total int64, detail string)

func (w *Worker) do(ctx context.Context, op Op, progress progressFunc) (any, error) {
	switch op.Kind {
	case "download":
		var a DownloadArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return nil, err
		}
		return w.download(ctx, a, progress)
	case "probe":
		var a ProbeArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return nil, err
		}
		if err := w.checkInput(a.Path); err != nil {
			return nil, err
		}
		return w.probe(ctx, a.Path)
	case "audio":
		var a AudioArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return nil, err
		}
		return w.audio(ctx, a, progress)
	case "silences":
		var a SilenceArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return nil, err
		}
		return w.silences(ctx, a)
	case "render":
		var a RenderArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return nil, err
		}
		return w.render(ctx, a, progress)
	}
	return nil, fmt.Errorf("unknown operation %q", op.Kind)
}

// checkInput accepts an existing regular file inside the media root with no
// symlinked path component.
func (w *Worker) checkInput(p string) error {
	if err := w.checkPath(p, false); err != nil {
		return err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", p)
	}
	return nil
}

// checkOutput accepts a path under temp/ and creates its folder.
func (w *Worker) checkOutput(p string) error {
	if err := w.checkPath(p, true); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		return err
	}
	return w.checkPath(filepath.Dir(p), true)
}

func (w *Worker) checkPath(p string, temp bool) error {
	base := w.cfg.MediaRoot
	if temp {
		base = filepath.Join(base, "temp")
	}
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || !strings.HasPrefix(p, base+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside %s", p, base)
	}
	rel, _ := filepath.Rel(w.cfg.MediaRoot, p)
	// Resolve the deepest existing part and require it to match the plain path.
	probe := p
	for {
		real, err := filepath.EvalSymlinks(probe)
		if err == nil {
			r, _ := filepath.Rel(w.cfg.MediaRoot, probe)
			if real != filepath.Join(w.root, r) {
				return fmt.Errorf("path %q goes through a symbolic link", rel)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) || probe == w.cfg.MediaRoot {
			return err
		}
		probe = filepath.Dir(probe)
	}
}

// prepareFonts copies a few host fonts into temp/fonts so FFmpeg can burn
// captions, also inside the media container, which sees only the media root.
func (w *Worker) prepareFonts() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	dir := filepath.Join(w.cfg.MediaRoot, "temp", "fonts")
	if w.fontsDir != "" {
		return w.fontsDir
	}
	if fontCount(dir) == 0 {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return ""
		}
		copied := copyFonts(dir, []string{"/usr/share/fonts", "/usr/local/share/fonts"})
		w.cfg.Log.Printf("copied %d font files to %s", copied, dir)
	}
	if fontCount(dir) > 0 {
		w.fontsDir = dir
	} else {
		w.cfg.Log.Printf("no fonts found: captions are written as .srt files only")
	}
	return w.fontsDir
}

func isFont(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".ttf" || ext == ".otf"
}

func fontCount(dir string) int {
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if e.Type().IsRegular() && isFont(e.Name()) {
			n++
		}
	}
	return n
}

// copyFonts copies preferred sans fonts, or else the first fonts found,
// up to 40 files.
func copyFonts(dst string, roots []string) int {
	var preferred, other []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() || !isFont(d.Name()) {
				return nil
			}
			lower := strings.ToLower(d.Name())
			if strings.Contains(lower, "dejavusans") || strings.Contains(lower, "liberationsans") || strings.Contains(lower, "notosans-") {
				preferred = append(preferred, p)
			} else {
				other = append(other, p)
			}
			return nil
		})
	}
	n := 0
	for _, p := range append(preferred, other...) {
		if n >= 40 {
			break
		}
		if copyFile(p, filepath.Join(dst, filepath.Base(p))) == nil {
			n++
		}
	}
	return n
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, io.LimitReader(in, 50<<20)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
