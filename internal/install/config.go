package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The server's settings file lives in Root/server/settings/config.json. The
// installer pre-fills the container service fields there so the Settings
// pages start with working values. These defaults must match internal/app
// (a test compares them).
const (
	configVersion       = 5
	defaultOllama       = "127.0.0.1:11434"
	defaultWhisper      = "127.0.0.1:10300"
	defaultEngine       = "podman"
	defaultFFmpeg       = "ffmpeg"
	defaultYtDlp        = "yt-dlp"
	defaultWhisperModel = "tiny-int8"
	defaultWhisperLang  = "en"
	// defaultMediaDir is the media root's name inside the install directory.
	defaultMediaDir = "media"
)

// mediaFolders are the logical folders under the media root (the same names
// as in internal/app; a test compares them).
var mediaFolders = []string{"incoming", "temp", "completed", "failed", "archive"}

// whisperName limits Whisper model and language values, which go into the
// Compose file.
var whisperName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func mediaFolderPaths(root string) []string {
	out := make([]string, len(mediaFolders))
	for k, f := range mediaFolders {
		out[k] = filepath.Join(root, f)
	}
	return out
}

func defaultMediaRoot(st State) string { return filepath.Join(st.Root, defaultMediaDir) }

// serviceFields lists the settings keys the installer fills, in file order.
func serviceFields(st State) []struct{ key, value string } {
	return []struct{ key, value string }{
		{"ollama_address", defaultOllama},
		{"whisper_address", defaultWhisper},
		{"media_engine", defaultEngine},
		{"ffmpeg_path", defaultFFmpeg},
		{"ytdlp_path", defaultYtDlp},
		{"whisper_model", defaultWhisperModel},
		{"whisper_language", defaultWhisperLang},
		{"media_root", defaultMediaRoot(st)},
	}
}

// checkMediaRoot applies the install directory's rules to the media root: it
// is mounted into the media container and created by the installer.
func checkMediaRoot(p string) error { return checkDir("media_root", p) }

// composeData is what the Compose template needs.
type composeData struct{ MediaRoot, WhisperModel, WhisperLang string }

// composeValues reads the Whisper model, language and media root from the
// settings file, with the defaults for anything missing. A file that cannot be
// read as JSON gives the defaults (install leaves it for the user to fix);
// values that are present but not allowed are an error, because they would be
// written into the Compose file.
func (i *Installer) composeValues(st State) (composeData, error) {
	d := composeData{MediaRoot: defaultMediaRoot(st), WhisperModel: defaultWhisperModel, WhisperLang: defaultWhisperLang}
	b, err := os.ReadFile(i.configPath(st))
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	var c struct {
		MediaRoot string `json:"media_root"`
		Model     string `json:"whisper_model"`
		Lang      string `json:"whisper_language"`
	}
	if json.Unmarshal(b, &c) != nil {
		return d, nil
	}
	for _, f := range []struct {
		key, in string
		out     *string
	}{{"whisper_model", c.Model, &d.WhisperModel}, {"whisper_language", c.Lang, &d.WhisperLang}} {
		if v := strings.TrimSpace(f.in); v != "" {
			if !whisperName.MatchString(v) {
				return d, fmt.Errorf("%s: %s %q may only contain letters, digits and . _ - (up to 64 characters)", i.configPath(st), f.key, v)
			}
			*f.out = v
		}
	}
	if v := strings.TrimSpace(c.MediaRoot); v != "" {
		if err := checkMediaRoot(v); err != nil {
			return d, fmt.Errorf("%s: %w", i.configPath(st), err)
		}
		d.MediaRoot = v
	}
	return d, nil
}

// mediaRoot is the media folder from the settings file, or Root/media.
func (i *Installer) mediaRoot(st State) (string, error) {
	d, err := i.composeValues(st)
	return d.MediaRoot, err
}

func (i *Installer) configPath(st State) string {
	return filepath.Join(st.Root, "server", "settings", "config.json")
}

// ensureConfig creates the settings file with the service defaults, or adds
// the service fields an existing file lacks. Values already present, other
// keys and the file's version are left alone; a file that cannot be read as
// version 5 or older settings is not touched.
func (i *Installer) ensureConfig(st State, gid int) (changed bool, err error) {
	path := i.configPath(st)
	if err := i.mkdir(st, gid, filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := refuseSymlink(path); err != nil {
		return false, err
	}
	fields := map[string]json.RawMessage{}
	old, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		fields["version"] = json.RawMessage(fmt.Sprint(configVersion))
	case err != nil:
		return false, err
	default:
		if json.Unmarshal(old, &fields) != nil {
			i.say("  note: %s is not valid JSON; leaving it for you to fix", path)
			return false, nil
		}
		var v int
		if json.Unmarshal(fields["version"], &v) != nil || v < 1 || v > configVersion {
			i.say("  note: %s has a settings version this installer does not know; leaving it alone", path)
			return false, nil
		}
	}
	for _, f := range serviceFields(st) {
		var cur string
		if json.Unmarshal(fields[f.key], &cur) == nil && strings.TrimSpace(cur) != "" {
			continue
		}
		b, _ := json.Marshal(f.value)
		fields[f.key] = b
		changed = true
	}
	if !changed {
		return false, nil
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return false, err
	}
	return true, writeAtomic(path, append(out, '\n'), 0o600, func(p string) error { return i.chown(st, gid, p) })
}

// serviceAddresses reads the Ollama and Whisper addresses from the settings
// file, using the defaults where the file or a field is missing.
func (i *Installer) serviceAddresses(st State) (ollama, whisper string, err error) {
	ollama, whisper = defaultOllama, defaultWhisper
	b, err := os.ReadFile(i.configPath(st))
	if os.IsNotExist(err) {
		return ollama, whisper, nil
	}
	if err != nil {
		return "", "", err
	}
	var c struct {
		Ollama  string `json:"ollama_address"`
		Whisper string `json:"whisper_address"`
	}
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&c); err != nil {
		return "", "", fmt.Errorf("%s: %w", i.configPath(st), err)
	}
	if s := strings.TrimSpace(c.Ollama); s != "" {
		ollama = s
	}
	if s := strings.TrimSpace(c.Whisper); s != "" {
		whisper = s
	}
	return ollama, whisper, nil
}

// canConnect reports whether addr accepts a TCP connection.
func (i *Installer) canConnect(addr string) error {
	dial := i.Dial
	if dial == nil {
		dial = net.DialTimeout
	}
	conn, err := dial("tcp", addr, 3*time.Second)
	if err != nil {
		return err
	}
	return conn.Close()
}
