package install

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gkuba/ReelAnvil/internal/app"
)

func readConfig(t *testing.T, in *Installer) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(in.Root, "server", "settings", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("config is not JSON: %v\n%s", err, b)
	}
	return m
}

func writeConfig(t *testing.T, in *Installer, body string) {
	t.Helper()
	dir := filepath.Join(in.Root, "server", "settings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// An earlier install left its marker, so install accepts the folder.
	if err := os.WriteFile(filepath.Join(in.Root, markerFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerDefaultsMatchTheApp(t *testing.T) {
	if defaultOllama != app.DefaultOllamaAddr || defaultWhisper != app.DefaultWhisperAddr || defaultEngine != app.EnginePodman ||
		defaultFFmpeg != app.DefaultFFmpegPath || defaultYtDlp != app.DefaultYtDlpPath ||
		defaultWhisperModel != app.DefaultWhisperModel || defaultWhisperLang != app.DefaultWhisperLang {
		t.Error("installer service defaults differ from internal/app")
	}
	if strings.Join(mediaFolders, ",") != strings.Join(app.MediaFolderNames(), ",") {
		t.Errorf("installer media folders %v differ from the app's %v", mediaFolders, app.MediaFolderNames())
	}
}

func TestMediaRootRulesMatchTheApp(t *testing.T) {
	for _, p := range []string{"/opt/ReelAnvil/media", "/srv/reel/media", "/mnt/media/a", "/home/alice/reel", "/var/lib/reel/m", "/tmp/a/b",
		"", "media", "/", "/srv", "/usr/media/x", "/etc/reel/x", "/var/x", "/home/alice", "/tmp/x", "/var/lib/containers/m", "/srv/my media",
		"/srv/a:b", "/srv/../etc/x", "/srv/x/", "/srv//x", "/proc/a/b", "/lib64/a/b", "/run/a/b", "/root/a/b"} {
		if got, want := checkMediaRoot(p) == nil, app.CheckMediaRoot(p) == nil; got != want {
			t.Errorf("%q: installer accepts=%v, app accepts=%v", p, got, want)
		}
	}
}

func TestInstallWritesServiceConfig(t *testing.T) {
	in, _, _ := newTest(t)
	var chowned []string
	in.Chown = func(path string, uid, gid int) error { chowned = append(chowned, filepath.Base(path)); return nil }
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	m := readConfig(t, in)
	want := map[string]any{"version": float64(5), "ollama_address": "127.0.0.1:11434", "whisper_address": "127.0.0.1:10300",
		"media_engine": "podman", "ffmpeg_path": "ffmpeg", "ytdlp_path": "yt-dlp",
		"whisper_model": "tiny-int8", "whisper_language": "en", "media_root": filepath.Join(in.Root, "media")}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
	p := filepath.Join(in.Root, "server", "settings", "config.json")
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("config mode: %v, %v", fi, err)
	}
	if fi, err := os.Stat(filepath.Dir(p)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("settings dir mode: %v, %v", fi, err)
	}
	if !strings.Contains(strings.Join(chowned, " "), "settings") || !strings.Contains(strings.Join(chowned, " "), ".config.json.tmp-") {
		t.Errorf("settings dir and file not given to the service user: %v", chowned)
	}
	if !strings.Contains(in.Out.(*strings.Builder).String(), "config.json") {
		t.Error("install plan does not mention the settings file")
	}
}

func TestInstallKeepsExistingConfigValuesAndFillsMissing(t *testing.T) {
	in, _, _ := newTest(t)
	writeConfig(t, in, `{"version":3,"instance_name":"Mine","theme":"aurora","ollama_address":"10.0.0.9:11434","whisper_address":"  "}`)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	m := readConfig(t, in)
	if m["version"] != float64(3) || m["instance_name"] != "Mine" || m["theme"] != "aurora" || m["ollama_address"] != "10.0.0.9:11434" {
		t.Errorf("existing values changed: %v", m)
	}
	if m["whisper_address"] != "127.0.0.1:10300" || m["media_engine"] != "podman" || m["ffmpeg_path"] != "ffmpeg" || m["ytdlp_path"] != "yt-dlp" {
		t.Errorf("missing fields not filled: %v", m)
	}
}

func TestInstallLeavesCompleteAndUnreadableConfigsAlone(t *testing.T) {
	complete := `{"version":5,"ollama_address":"a:1","whisper_address":"b:2","media_engine":"host","ffmpeg_path":"/x/ff","ytdlp_path":"/x/yt","whisper_model":"base","whisper_language":"de","media_root":"@ROOT@/custom-media"}`
	for name, body := range map[string]string{
		"complete":       complete,
		"future version": `{"version":9,"theme":"x"}`,
		"version 6":      `{"version":6,"theme":"x"}`,
		"not json":       `{oops`,
		"no version":     `{"theme":"x"}`,
	} {
		in, _, _ := newTest(t)
		body = strings.ReplaceAll(body, "@ROOT@", in.Root)
		writeConfig(t, in, body)
		if err := in.Install(); err != nil {
			t.Fatalf("%s: install failed: %v", name, err)
		}
		got, _ := os.ReadFile(filepath.Join(in.Root, "server", "settings", "config.json"))
		if string(got) != body {
			t.Errorf("%s: file changed to %s", name, got)
		}
	}
}

func TestUpgradeFillsServiceFieldsOfAnOlderConfig(t *testing.T) {
	in, _, _ := installed(t)
	writeConfig(t, in, `{"version":3,"instance_name":"Old"}`)
	if err := in.Upgrade(); err != nil {
		t.Fatal(err)
	}
	m := readConfig(t, in)
	if m["instance_name"] != "Old" || m["version"] != float64(3) || m["ollama_address"] != "127.0.0.1:11434" || m["media_engine"] != "podman" {
		t.Errorf("config after upgrade: %v", m)
	}
}

func TestDoctorChecksServiceAddressesFromConfig(t *testing.T) {
	in, _, _ := installed(t)
	writeConfig(t, in, `{"version":4,"ollama_address":"gpu.lan:11434","whisper_address":"10.0.0.7:10300"}`)
	var dialed []string
	in.Dial = func(_, addr string, _ time.Duration) (net.Conn, error) {
		dialed = append(dialed, addr)
		if addr == "10.0.0.7:10300" {
			return nil, errors.New("connection refused")
		}
		c, _ := net.Pipe()
		return c, nil
	}
	err := in.Doctor(false, false)
	if err == nil {
		t.Fatal("doctor passed with an unreachable Whisper address")
	}
	out := in.Out.(*strings.Builder).String()
	if strings.Join(dialed, " ") != "gpu.lan:11434 10.0.0.7:10300" {
		t.Errorf("dialed %v", dialed)
	}
	if !strings.Contains(out, "[ok  ] Ollama gpu.lan:11434: accepts connections") || !strings.Contains(out, "[FAIL] Whisper 10.0.0.7:10300: not reachable: connection refused") {
		t.Errorf("doctor output:\n%s", out)
	}
}

func TestDoctorUsesDefaultServiceAddressesWithoutConfig(t *testing.T) {
	in, _, _ := installed(t)
	os.Remove(filepath.Join(in.Root, "server", "settings", "config.json"))
	var dialed []string
	in.Dial = func(_, addr string, _ time.Duration) (net.Conn, error) {
		dialed = append(dialed, addr)
		c, _ := net.Pipe()
		return c, nil
	}
	if err := in.Doctor(false, false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(dialed, " ") != "127.0.0.1:11434 127.0.0.1:10300" {
		t.Errorf("dialed %v", dialed)
	}
}

func TestDoctorFailsOnUnreadableConfig(t *testing.T) {
	in, _, _ := installed(t)
	writeConfig(t, in, `{oops`)
	if err := in.Doctor(false, false); err == nil || !strings.Contains(in.Out.(*strings.Builder).String(), "[FAIL] service addresses") {
		t.Errorf("err=%v\n%s", err, in.Out.(*strings.Builder).String())
	}
}

func TestDoctorRepairDoesNotTouchConfig(t *testing.T) {
	in, _, _ := installed(t)
	writeConfig(t, in, `{"version":3}`)
	os.Remove(filepath.Join(in.Root, "containers", "compose.yaml"))
	if err := in.Doctor(true, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(in.Root, "server", "settings", "config.json"))
	if string(got) != `{"version":3}` {
		t.Errorf("repair changed settings: %s", got)
	}
}
