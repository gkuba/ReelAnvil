package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The server's settings file lives in Root/server/settings/config.json. The
// installer pre-fills the container service fields there so the Settings
// pages start with working values. These defaults must match internal/app
// (a test compares them).
const (
	configVersion  = 4
	defaultOllama  = "127.0.0.1:11434"
	defaultWhisper = "127.0.0.1:10300"
	defaultEngine  = "podman"
	defaultFFmpeg  = "ffmpeg"
	defaultYtDlp   = "yt-dlp"
)

// serviceFields lists the settings keys the installer fills, in file order.
var serviceFields = []struct{ key, value string }{
	{"ollama_address", defaultOllama},
	{"whisper_address", defaultWhisper},
	{"media_engine", defaultEngine},
	{"ffmpeg_path", defaultFFmpeg},
	{"ytdlp_path", defaultYtDlp},
}

func (i *Installer) configPath(st State) string {
	return filepath.Join(st.Root, "server", "settings", "config.json")
}

// ensureConfig creates the settings file with the service defaults, or adds
// the service fields an existing file lacks. Values already present, other
// keys and the file's version are left alone; a file that cannot be read as
// version 4 or older settings is not touched.
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
	for _, f := range serviceFields {
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
