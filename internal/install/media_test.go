package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readCompose(t *testing.T, in *Installer) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(in.Root, "containers", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestComposeUsesDefaultsWithoutSettings(t *testing.T) {
	in, _, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	compose := readCompose(t, in)
	media := filepath.Join(in.Root, "media")
	for _, want := range []string{`WHISPER_MODEL: "tiny-int8"`, `WHISPER_LANG: "en"`, `- "` + media + `:` + media + `"`, "nandyalu/python-ffmpeg:latest", "127.0.0.1:11434:11434"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q:\n%s", want, compose)
		}
	}
	if strings.Contains(compose, "{{") {
		t.Errorf("compose has unrendered values:\n%s", compose)
	}
}

func TestComposeFollowsSettingsOnUpgrade(t *testing.T) {
	in, _, dir := installed(t)
	custom := filepath.Join(dir, "disk2", "reel-media")
	writeConfig(t, in, `{"version":5,"whisper_model":"large-v3","whisper_language":"de","media_root":"`+custom+`"}`)
	if err := in.Upgrade(); err != nil {
		t.Fatal(err)
	}
	compose := readCompose(t, in)
	for _, want := range []string{`WHISPER_MODEL: "large-v3"`, `WHISPER_LANG: "de"`, `- "` + custom + `:` + custom + `"`} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q:\n%s", want, compose)
		}
	}
	if _, err := os.Stat(filepath.Join(in.Root, "containers", "compose.yaml.bak")); err != nil {
		t.Error("old compose.yaml not kept as .bak")
	}
	// The saved values were kept, not overwritten by defaults.
	m := readConfig(t, in)
	if m["whisper_model"] != "large-v3" || m["media_root"] != custom {
		t.Errorf("config after upgrade: %v", m)
	}
	out := in.Out.(*strings.Builder).String()
	if !strings.Contains(out, "Whisper model and language and the media root "+custom) {
		t.Errorf("upgrade plan does not mention the media root:\n%s", out)
	}
	// The custom media root and its folders now exist.
	for _, p := range append([]string{custom}, mediaFolderPaths(custom)...) {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Errorf("%s not created: %v", p, err)
		}
	}
}

func TestComposeRefusesUnsafeSettingsValues(t *testing.T) {
	for name, body := range map[string]string{
		"model with space":      `{"version":5,"whisper_model":"tiny int8"}`,
		"model with newline":    `{"version":5,"whisper_model":"tiny\nPUID: 0"}`,
		"model with quote":      `{"version":5,"whisper_model":"x\" injected: \""}`,
		"language too long":     `{"version":5,"whisper_language":"` + strings.Repeat("a", 65) + `"}`,
		"relative media root":   `{"version":5,"media_root":"media"}`,
		"media root with colon": `{"version":5,"media_root":"/srv/a:/etc"}`,
		"media root in /etc":    `{"version":5,"media_root":"/etc/reel/media"}`,
		"media root with ..":    `{"version":5,"media_root":"/srv/x/../../etc/y"}`,
		"media root is /":       `{"version":5,"media_root":"/"}`,
	} {
		in, _, _ := installed(t)
		before := readCompose(t, in)
		writeConfig(t, in, body)
		err := in.Upgrade()
		if err == nil || !strings.Contains(err.Error(), "config.json") {
			t.Errorf("%s: upgrade err = %v", name, err)
		}
		if readCompose(t, in) != before {
			t.Errorf("%s: compose.yaml changed", name)
		}
		if err := in.Doctor(false, false); err == nil || !strings.Contains(in.Out.(*strings.Builder).String(), "[FAIL]") {
			t.Errorf("%s: doctor did not fail", name)
		}
	}
}

func TestComposeIgnoresUnreadableSettings(t *testing.T) {
	in, _, _ := installed(t)
	writeConfig(t, in, `{oops`)
	if err := in.Upgrade(); err != nil {
		t.Fatalf("upgrade with unreadable settings: %v", err)
	}
	if !strings.Contains(readCompose(t, in), `WHISPER_MODEL: "tiny-int8"`) {
		t.Error("defaults not used")
	}
}

func TestInstallCreatesMediaFoldersForTheServiceUser(t *testing.T) {
	in, _, _ := newTest(t)
	chowned := map[string]bool{}
	in.Chown = func(path string, uid, gid int) error { chowned[path] = true; return nil }
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(in.Root, "media")
	if fi, err := os.Stat(media); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("media root: %v %v", fi, err)
	}
	for _, f := range []string{"incoming", "temp", "completed", "failed", "archive"} {
		p := filepath.Join(media, f)
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o750 {
			t.Errorf("%s: %v %v", f, fi, err)
		}
		if !chowned[p] {
			t.Errorf("%s not given to the service user", p)
		}
	}
	if !chowned[media] {
		t.Error("media root not given to the service user")
	}
}

func TestExistingMediaFoldersAreNeitherChownedNorChmodded(t *testing.T) {
	in, _, dir := installed(t)
	custom := filepath.Join(dir, "disk2", "media")
	if err := os.MkdirAll(filepath.Join(custom, "archive"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "archive", "orig.mp4"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var chowned []string
	in.Chown = func(path string, _, _ int) error { chowned = append(chowned, path); return nil }
	writeConfig(t, in, `{"version":5,"media_root":"`+custom+`"}`)
	st, _ := in.loadState()
	if err := in.ensureMediaFolders(st, 988); err != nil {
		t.Fatal(err)
	}
	for _, p := range chowned {
		if p == custom || p == filepath.Join(custom, "archive") {
			t.Errorf("chown touched existing %s", p)
		}
	}
	if fi, _ := os.Stat(custom); fi.Mode().Perm() != 0o700 {
		t.Errorf("existing media root re-moded to %v", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Join(custom, "archive")); fi.Mode().Perm() != 0o700 {
		t.Errorf("existing archive re-moded to %v", fi.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(custom, "temp")); err != nil {
		t.Error("missing sub-folder not created")
	}
	if b, _ := os.ReadFile(filepath.Join(custom, "archive", "orig.mp4")); string(b) != "x" {
		t.Error("existing file touched")
	}
	// A media root that is a symbolic link is refused.
	link := filepath.Join(dir, "linked")
	os.Symlink(custom, link)
	writeConfig(t, in, `{"version":5,"media_root":"`+link+`"}`)
	if err := in.ensureMediaFolders(st, 988); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("symlinked media root: %v", err)
	}
}

func TestDoctorChecksMediaRoot(t *testing.T) {
	in, f, dir := installed(t)
	if err := in.Doctor(false, false); err != nil {
		t.Fatalf("healthy: %v", err)
	}
	out := in.Out.(*strings.Builder).String()
	media := filepath.Join(in.Root, "media")
	if !strings.Contains(out, "[ok  ] folder "+media+" (media root): present and writable by reelanvil") {
		t.Errorf("healthy output:\n%s", out)
	}
	if f.ran("-- env HOME="+in.Root) == 0 || f.ran("test -d "+media+" -a -w "+media+" -a -x "+media) != 1 {
		t.Errorf("writable test was not run as the service user: %v", f.log)
	}

	// Not writable by the service user.
	in.Out = &strings.Builder{}
	f.fail = map[string]error{"test -d": errors.New("exit status 1")}
	if err := in.Doctor(false, false); err == nil {
		t.Error("doctor passed with an unwritable media root")
	}
	out = in.Out.(*strings.Builder).String()
	if !strings.Contains(out, "[FAIL] folder "+media+" (media root): reelanvil cannot write to it; fix with: sudo chown reelanvil:reelanvil "+media) {
		t.Errorf("unwritable output:\n%s", out)
	}
	f.fail = nil

	// Missing: reported, and repair recreates it with its folders.
	in.Out = &strings.Builder{}
	os.RemoveAll(media)
	if err := in.Doctor(false, false); err == nil || !strings.Contains(in.Out.(*strings.Builder).String(), "[FAIL] folder "+media+" (media root): missing") {
		t.Errorf("missing media root: %v\n%s", err, in.Out.(*strings.Builder).String())
	}
	in.Out = &strings.Builder{}
	if err := in.Doctor(true, false); err != nil {
		t.Fatalf("repair: %v\n%s", err, in.Out.(*strings.Builder).String())
	}
	if fi, err := os.Stat(filepath.Join(media, "incoming")); err != nil || !fi.IsDir() {
		t.Errorf("repair did not recreate the media folders: %v", err)
	}

	// A media root from the settings file is the one checked.
	custom := filepath.Join(dir, "disk2", "media")
	writeConfig(t, in, `{"version":5,"media_root":"`+custom+`"}`)
	in.Out = &strings.Builder{}
	if err := in.Doctor(false, false); err == nil || !strings.Contains(in.Out.(*strings.Builder).String(), "folder "+custom+" (media root): missing") {
		t.Errorf("custom root: %v\n%s", err, in.Out.(*strings.Builder).String())
	}
	// A file where the folder should be.
	os.MkdirAll(filepath.Dir(custom), 0o755)
	os.WriteFile(custom, []byte("x"), 0o600)
	in.Out = &strings.Builder{}
	if err := in.Doctor(false, false); err == nil || !strings.Contains(in.Out.(*strings.Builder).String(), "not a folder") {
		t.Errorf("file as media root: %v", err)
	}
}

func TestDoctorWithoutSudoOnlyChecksThatTheMediaRootExists(t *testing.T) {
	in, f, _ := installed(t)
	in.IsRoot = false
	f.log = nil
	if err := in.Doctor(false, false); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(in.Out.(*strings.Builder).String(), "(media root): present (run with sudo to test that reelanvil can write to it)") {
		t.Errorf("output:\n%s", in.Out.(*strings.Builder).String())
	}
	if f.ran("test -d") != 0 {
		t.Error("ran the writable test without root")
	}
}

func TestDoctorNotesComposeOutOfStepWithSettings(t *testing.T) {
	in, _, _ := installed(t)
	writeConfig(t, in, `{"version":5,"whisper_model":"small"}`)
	if err := in.Doctor(false, false); err != nil {
		t.Fatalf("doctor failed on a settings change: %v", err)
	}
	if !strings.Contains(in.Out.(*strings.Builder).String(), "differs from what the settings produce") {
		t.Errorf("output:\n%s", in.Out.(*strings.Builder).String())
	}
}
