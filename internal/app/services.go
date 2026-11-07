package app

// Settings for the container services (Ollama, Whisper and the media engine)
// and the connection tests on their pages. The web server only opens network
// connections to the addresses saved here; it never talks to the Docker or
// Podman socket, because the worker owns the containers.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultOllamaAddr  = "127.0.0.1:11434"
	DefaultWhisperAddr = "127.0.0.1:10300"
	DefaultFFmpegPath  = "ffmpeg"
	DefaultYtDlpPath   = "yt-dlp"

	EnginePodman = "podman"
	EngineDocker = "docker"
	EngineHost   = "host"
)

// serviceTimeout bounds every connection test step.
var serviceTimeout = 3 * time.Second

const (
	maxServiceBody = 1 << 20
	maxToolPath    = 512
	maxListedNames = 12
)

// validEngine reports whether v is a known media engine.
func validEngine(v string) bool {
	return v == EnginePodman || v == EngineDocker || v == EngineHost
}

// normalizeServiceAddress checks a host:port value and returns it in canonical
// form. The host is a name or an IP address; schemes and paths are refused.
func normalizeServiceAddress(v string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(v))
	if err != nil {
		return "", errors.New("address must be host:port, for example 127.0.0.1:11434")
	}
	host = normalizeHost(host)
	if net.ParseIP(host) == nil && (len(host) > 253 || !hostNamePattern.MatchString(host)) {
		return "", fmt.Errorf("%q is not a valid host name or IP address", host)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("port must be a number from 1 to 65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(n)), nil
}

// normalizeToolPath checks a program name or path; empty means the default.
func normalizeToolPath(v, def string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, nil
	}
	if len(v) > maxToolPath || strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return "", errors.New("program path must be one line of at most 512 characters")
	}
	return v, nil
}

// fillServices replaces empty service fields with defaults and rejects
// invalid saved values. This is also how settings from before version 4 gain
// their service fields.
func (c *Settings) fillServices() (err error) {
	fill := func(name string, field *string, def string, norm func(string) (string, error)) {
		if err != nil {
			return
		}
		if strings.TrimSpace(*field) == "" {
			*field = def
			return
		}
		var out string
		if out, err = norm(*field); err != nil {
			err = fmt.Errorf("invalid %s %q: %w", name, *field, err)
			return
		}
		*field = out
	}
	fill("ollama_address", &c.OllamaAddress, DefaultOllamaAddr, normalizeServiceAddress)
	fill("whisper_address", &c.WhisperAddress, DefaultWhisperAddr, normalizeServiceAddress)
	fill("ffmpeg_path", &c.FFmpegPath, DefaultFFmpegPath, func(v string) (string, error) { return normalizeToolPath(v, DefaultFFmpegPath) })
	fill("ytdlp_path", &c.YtDlpPath, DefaultYtDlpPath, func(v string) (string, error) { return normalizeToolPath(v, DefaultYtDlpPath) })
	if err != nil {
		return err
	}
	if c.MediaEngine == "" {
		c.MediaEngine = EnginePodman
	} else if !validEngine(c.MediaEngine) {
		return fmt.Errorf("invalid media_engine %q", c.MediaEngine)
	}
	return nil
}

// updateSettings applies change to a copy of the saved settings and saves it.
func (s *Server) updateSettings(change func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	change(&next)
	next.Version = settingsVersion
	if err := save(s.cfgPath, next); err != nil {
		s.log.Println("settings save failed:", err)
		return errors.New("could not write settings")
	}
	s.cfg = next
	return nil
}

// serviceSave handles a settings form POST: parse validates the form and
// returns the change to apply. GET shows the page.
func (s *Server) serviceSave(path, label string, parse func(*http.Request) (func(*Settings), error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			s.page(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		change, err := parse(r)
		if err == nil {
			err = s.updateSettings(change)
		}
		if err != nil {
			setFlash(w, label+" not saved: "+err.Error()+".", true)
		} else {
			setFlash(w, label+" saved.", false)
		}
		http.Redirect(w, r, path, http.StatusSeeOther)
	}
}

// serviceTest handles a Test connection POST. run gets the settings with any
// values typed into the form applied, so a change can be tried before it is
// saved. The result goes back through the flash message.
func (s *Server) serviceTest(path string, run func(Settings) (string, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		cfg := s.cfg
		s.mu.Unlock()
		if err := applyServiceForm(&cfg, r); err != nil {
			setFlash(w, "Test not run: "+err.Error()+".", true)
		} else {
			msg, bad := run(cfg)
			setFlash(w, clip(msg, 480), bad)
		}
		http.Redirect(w, r, path, http.StatusSeeOther)
	}
}

// applyServiceForm overlays the service fields present in the form on c.
func applyServiceForm(c *Settings, r *http.Request) error {
	var err error
	if r.Form.Has("ollama_address") {
		c.OllamaAddress, err = normalizeServiceAddress(r.FormValue("ollama_address"))
	}
	if err == nil && r.Form.Has("whisper_address") {
		c.WhisperAddress, err = normalizeServiceAddress(r.FormValue("whisper_address"))
	}
	if err == nil && r.Form.Has("media_engine") {
		if c.MediaEngine = r.FormValue("media_engine"); !validEngine(c.MediaEngine) {
			err = errors.New("choose podman, docker or host")
		}
	}
	if err == nil && r.Form.Has("ffmpeg_path") {
		c.FFmpegPath, err = normalizeToolPath(r.FormValue("ffmpeg_path"), DefaultFFmpegPath)
	}
	if err == nil && r.Form.Has("ytdlp_path") {
		c.YtDlpPath, err = normalizeToolPath(r.FormValue("ytdlp_path"), DefaultYtDlpPath)
	}
	return err
}

// clip shortens text so it fits in a flash message.
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func (s *Server) aiSettings(w http.ResponseWriter, r *http.Request) {
	s.serviceSave("/settings/ai", "AI settings", func(r *http.Request) (func(*Settings), error) {
		addr, err := normalizeServiceAddress(r.FormValue("ollama_address"))
		return func(c *Settings) { c.OllamaAddress = addr }, err
	})(w, r)
}

func (s *Server) aiTest(w http.ResponseWriter, r *http.Request) {
	s.serviceTest("/settings/ai", func(c Settings) (string, bool) { return testOllama(c.OllamaAddress) })(w, r)
}

func (s *Server) transcriptionSettings(w http.ResponseWriter, r *http.Request) {
	s.serviceSave("/settings/transcription", "Transcription settings", func(r *http.Request) (func(*Settings), error) {
		addr, err := normalizeServiceAddress(r.FormValue("whisper_address"))
		return func(c *Settings) { c.WhisperAddress = addr }, err
	})(w, r)
}

func (s *Server) transcriptionTest(w http.ResponseWriter, r *http.Request) {
	s.serviceTest("/settings/transcription", func(c Settings) (string, bool) { return testWhisper(c.WhisperAddress) })(w, r)
}

func (s *Server) mediaSettings(w http.ResponseWriter, r *http.Request) {
	s.serviceSave("/settings/media", "Media settings", func(r *http.Request) (func(*Settings), error) {
		var c Settings
		if !r.Form.Has("media_engine") {
			return nil, errors.New("choose a media engine")
		}
		if err := applyServiceForm(&c, r); err != nil {
			return nil, err
		}
		return func(next *Settings) {
			next.MediaEngine = c.MediaEngine
			if c.FFmpegPath != "" {
				next.FFmpegPath = c.FFmpegPath
			}
			if c.YtDlpPath != "" {
				next.YtDlpPath = c.YtDlpPath
			}
		}, nil
	})(w, r)
}

func (s *Server) mediaTest(w http.ResponseWriter, r *http.Request) {
	s.serviceTest("/settings/media", testMedia)(w, r)
}

// ---- connection tests ----

// serviceError turns a network error into short text.
func serviceError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Sprintf("no answer within %s", serviceTimeout)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "the connection was closed"
	}
	return err.Error()
}

var ollamaClient = &http.Client{
	Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func ollamaGet(ctx context.Context, addr, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return err
	}
	resp, err := ollamaClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d, so this does not look like Ollama", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxServiceBody)).Decode(out); err != nil {
		return fmt.Errorf("%s did not return the expected data", path)
	}
	return nil
}

// testOllama asks Ollama for its version and installed models.
func testOllama(addr string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceTimeout)
	defer cancel()
	var ver struct {
		Version string `json:"version"`
	}
	if err := ollamaGet(ctx, addr, "/api/version", &ver); err != nil {
		return fmt.Sprintf("Could not reach Ollama at %s: %s.", addr, serviceError(err)), true
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := ollamaGet(ctx, addr, "/api/tags", &tags); err != nil {
		return fmt.Sprintf("Connected to Ollama %s at %s, but the model list could not be read: %s.", ver.Version, addr, serviceError(err)), true
	}
	var names []string
	for _, m := range tags.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return fmt.Sprintf("Connected to Ollama %s at %s. %s", ver.Version, addr, modelSummary("Installed models", "No models installed yet.", names)), false
}

// modelSummary lists up to maxListedNames names.
func modelSummary(prefix, none string, names []string) string {
	if len(names) == 0 {
		return none
	}
	extra := ""
	if len(names) > maxListedNames {
		extra = fmt.Sprintf(" and %d more", len(names)-maxListedNames)
		names = names[:maxListedNames]
	}
	return prefix + ": " + strings.Join(names, ", ") + extra + "."
}

// wyomingInfo is the part of a Wyoming "info" event that lists speech
// recognition models.
type wyomingInfo struct {
	ASR []struct {
		Name   string `json:"name"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	} `json:"asr"`
}

// readWyomingInfo sends a describe event and reads the info reply. The reply
// is one JSON line, optionally followed by data_length bytes of extra JSON.
func readWyomingInfo(conn net.Conn) (wyomingInfo, error) {
	var info wyomingInfo
	if _, err := io.WriteString(conn, `{"type":"describe"}`+"\n"); err != nil {
		return info, err
	}
	rd := bufio.NewReader(io.LimitReader(conn, maxServiceBody))
	line, err := rd.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return info, err
	}
	var head struct {
		Type       string          `json:"type"`
		Data       json.RawMessage `json:"data"`
		DataLength int             `json:"data_length"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return info, errors.New("reply was not a Wyoming event")
	}
	if head.Type != "info" {
		return info, fmt.Errorf("reply was %q, not info", head.Type)
	}
	data := []byte(head.Data)
	if head.DataLength > 0 {
		if head.DataLength > maxServiceBody {
			return info, errors.New("info reply too large")
		}
		extra := make([]byte, head.DataLength)
		if _, err := io.ReadFull(rd, extra); err != nil {
			return info, err
		}
		data = extra
	}
	if len(data) == 0 {
		return info, errors.New("info reply had no data")
	}
	return info, json.Unmarshal(data, &info)
}

// testWhisper opens a TCP connection and asks the Wyoming service to describe itself.
func testWhisper(addr string) (string, bool) {
	conn, err := net.DialTimeout("tcp", addr, serviceTimeout)
	if err != nil {
		return fmt.Sprintf("Could not connect to %s: %s.", addr, serviceError(err)), true
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(serviceTimeout))
	info, err := readWyomingInfo(conn)
	if err != nil {
		return fmt.Sprintf("Connected to %s, but it sent no service info (%s).", addr, serviceError(err)), false
	}
	var names []string
	for _, p := range info.ASR {
		for _, m := range p.Models {
			if m.Name != "" {
				names = append(names, m.Name)
			}
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("Connected to %s. It reported no speech recognition models.", addr), false
	}
	return fmt.Sprintf("Connected to %s. %s", addr, modelSummary("Speech recognition models", "", names)), false
}

// testMedia checks the media engine. Containers belong to the worker, so only
// the host engine has anything to look for; programs are located, not run.
func testMedia(c Settings) (string, bool) {
	if c.MediaEngine != EngineHost {
		return "The media engine is " + c.MediaEngine + ". The worker starts and manages the media container, so there is nothing to test from here.", false
	}
	var parts []string
	bad := false
	for _, t := range []struct{ name, path string }{{"FFmpeg", c.FFmpegPath}, {"yt-dlp", c.YtDlpPath}} {
		found, err := exec.LookPath(t.path)
		if err != nil {
			parts = append(parts, t.name+" not found ("+t.path+")")
			bad = true
			continue
		}
		parts = append(parts, t.name+" found at "+found)
	}
	return strings.Join(parts, "; ") + ".", bad
}
