package app

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMigratesVersion4ToVersion5(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	v4 := `{"version":4,"instance_name":"Kept","theme":"aurora","listen_address":"0.0.0.0:4747","ollama_address":"10.0.0.5:11434","whisper_address":"10.0.0.6:10300","media_engine":"host","ffmpeg_path":"/opt/ff","ytdlp_path":"/opt/yt"}`
	if err := os.WriteFile(p, []byte(v4), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 5 || c.InstanceName != "Kept" || c.OllamaAddress != "10.0.0.5:11434" || c.MediaEngine != "host" || c.FFmpegPath != "/opt/ff" {
		t.Errorf("v4 values changed: %+v", c)
	}
	if c.WhisperModel != "tiny-int8" || c.WhisperLanguage != "en" || c.HunterModel != "" || c.ReviewerModel != "" || c.MediaRoot != "" || c.StorageLimitGB != 0 {
		t.Errorf("v5 defaults not filled: %+v", c)
	}
	// Saving writes the new fields and version.
	if err := save(p, c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{`"version": 5`, `"whisper_model": "tiny-int8"`, `"storage_limit_gb": 0`, `"media_root": ""`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("saved file lacks %s:\n%s", want, b)
		}
	}
	// Future versions are still refused.
	if err := os.WriteFile(p, []byte(`{"version":6}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(p); err == nil {
		t.Error("load accepted version 6")
	}
}

func TestLoadRejectsInvalidVersion5Values(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	for _, bad := range []string{
		`"whisper_model":"tiny int8"`, `"whisper_model":"a/b"`, `"whisper_language":"en;rm"`,
		`"hunter_model":"bad name"`, `"reviewer_model":"-x"`,
		`"media_root":"relative/path"`, `"media_root":"/srv/../etc"`, `"media_root":"/"`, `"media_root":"/usr/media"`,
		`"storage_limit_gb":-1`, `"storage_limit_gb":99999999999`,
	} {
		if err := os.WriteFile(p, []byte(`{"version":5,`+bad+`}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := load(p); err == nil {
			t.Errorf("load accepted %s", bad)
		}
	}
	good := `{"version":5,"whisper_model":"large-v3","whisper_language":"auto","hunter_model":"qwen2.5:3b","reviewer_model":"hf.co/team/model:Q4_K_M","media_root":"/srv/reel/media","storage_limit_gb":500}`
	if err := os.WriteFile(p, []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := load(p)
	if err != nil || c.WhisperModel != "large-v3" || c.ReviewerModel != "hf.co/team/model:Q4_K_M" || c.MediaRoot != "/srv/reel/media" || c.StorageLimitGB != 500 {
		t.Errorf("good values: %+v, %v", c, err)
	}
}

func TestNormalizeMediaRoot(t *testing.T) {
	good := map[string]string{
		"/srv/reel/media":       "/srv/reel/media",
		" /mnt/media/ ":         "/mnt/media",
		"/srv//reel/./media":    "/srv/reel/media",
		"/home/alice/reel":      "/home/alice/reel",
		"/opt/ReelAnvil/media":  "/opt/ReelAnvil/media",
		"/var/lib/reelanvil/m1": "/var/lib/reelanvil/m1",
	}
	for in, want := range good {
		if got, err := normalizeMediaRoot(in); err != nil || got != want {
			t.Errorf("normalizeMediaRoot(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "media", "./media", "../media", "/srv/../etc", "/srv/media/..", "/", "/srv", "/usr/local/media", "/etc/reel",
		"/var/x", "/home/alice", "/tmp/x", "/var/lib/containers/m", "/srv/my media", "/srv/a:b", "/srv/a\nb", "/proc/x/y", "/" + strings.Repeat("a", 600)} {
		if got, err := normalizeMediaRoot(in); err == nil {
			t.Errorf("normalizeMediaRoot(%q) = %q, want an error", in, got)
		}
	}
}

func TestParseStorageLimit(t *testing.T) {
	for in, want := range map[string]int{"": 0, " 0 ": 0, "250": 250} {
		if got, err := parseStorageLimit(in); err != nil || got != want {
			t.Errorf("parseStorageLimit(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"-1", "1.5", "abc", "99999999999", "1e3"} {
		if _, err := parseStorageLimit(in); err == nil {
			t.Errorf("parseStorageLimit(%q) accepted", in)
		}
	}
}

func TestEnsureMediaFoldersCreatesOnlyMissingOnes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.mp4"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "archive"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive", "original.mp4"), []byte("orig"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := ensureMediaFolders(root)
	if err != nil || strings.Join(created, ",") != "incoming,temp,completed,failed" {
		t.Fatalf("created %v, %v", created, err)
	}
	for _, f := range storageFolders {
		fi, err := os.Stat(filepath.Join(root, f.Name))
		if err != nil || !fi.IsDir() {
			t.Errorf("%s missing: %v", f.Name, err)
		}
	}
	if fi, _ := os.Stat(filepath.Join(root, "incoming")); fi.Mode().Perm() != 0o750 {
		t.Errorf("incoming mode %v, want 0750", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Join(root, "archive")); fi.Mode().Perm() != 0o700 {
		t.Errorf("existing archive folder was changed: %v", fi.Mode().Perm())
	}
	for _, f := range []string{"keep.mp4", "archive/original.mp4"} {
		if b, err := os.ReadFile(filepath.Join(root, f)); err != nil || len(b) == 0 {
			t.Errorf("%s was touched: %v", f, err)
		}
	}
	if again, err := ensureMediaFolders(root); err != nil || len(again) != 0 {
		t.Errorf("second run created %v, %v", again, err)
	}
	if _, err := ensureMediaFolders(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Errorf("missing root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); err == nil {
		t.Error("created the missing root")
	}
	if folderPath(root, FolderIncoming) != filepath.Join(root, incomingFolderName) {
		t.Error("incoming key does not map to the arrival folder constant")
	}
}

func TestCheckStorage(t *testing.T) {
	missing := checkStorage(filepath.Join(t.TempDir(), "nope"), 0, true)
	if missing.Exists || missing.OK() {
		t.Errorf("missing root: %+v", missing)
	}
	if msg, bad := missing.summary(); !bad || !strings.Contains(msg, "does not exist") {
		t.Errorf("missing summary %q", msg)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := checkStorage(file, 0, true); !r.Exists || r.IsDir || r.OK() {
		t.Errorf("file as root: %+v", r)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.bin"), make([]byte, 3000), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "incoming"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "incoming", "b.bin"), make([]byte, 1000), 0600); err != nil {
		t.Fatal(err)
	}
	r := checkStorage(root, 10, true)
	if !r.IsDir || !r.Writable || !r.FreeKnown || r.Free == 0 || r.Total < r.Free || !r.UsedKnown || r.Used != 4000 || r.UsedPartial || r.Over() || !r.OK() {
		t.Errorf("healthy root: %+v", r)
	}
	if strings.Join(r.Missing, ",") != "temp,completed,failed,archive" {
		t.Errorf("missing folders %v", r.Missing)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".reelanvil-write-test") {
			t.Errorf("write test left %s behind", e.Name())
		}
	}
	msg, bad := r.summary()
	if bad || !strings.Contains(msg, "writable") || !strings.Contains(msg, "free of") || !strings.Contains(msg, "3.9 KB") || !strings.Contains(msg, "10 GB limit") {
		t.Errorf("summary %q bad=%v", msg, bad)
	}

	if r := checkStorage(root, 0, false); r.UsedKnown {
		t.Error("used space measured when not asked")
	}
}

func TestCheckStorageOverLimitUsesSparseFile(t *testing.T) {
	root := t.TempDir()
	f, err := os.Create(filepath.Join(root, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(3 << 29); err != nil { // 1.5 GB, sparse
		t.Skip("cannot make a sparse file here:", err)
	}
	f.Close()
	r := checkStorage(root, 1, true)
	if !r.Over() || r.OK() {
		t.Fatalf("1.5 GB used against a 1 GB limit: %+v", r)
	}
	if msg, bad := r.summary(); !bad || !strings.Contains(msg, "Over the storage limit") {
		t.Errorf("summary %q bad=%v", msg, bad)
	}
	if r := checkStorage(root, 2, true); r.Over() {
		t.Error("1.5 GB reported over a 2 GB limit")
	}
	if r := checkStorage(root, 0, true); r.Over() {
		t.Error("no limit reported over")
	}
}

func TestUsedSpaceStopsAtTheEntryCap(t *testing.T) {
	old := usedWalkEntries
	usedWalkEntries = 5
	t.Cleanup(func() { usedWalkEntries = old })
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(root, "f"+string(rune('a'+i))), []byte("12345"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	total, partial := usedSpace(root)
	if !partial || total >= 100 {
		t.Errorf("total %d partial %v, want a partial count below 100", total, partial)
	}
	r := checkStorage(root, 0, true)
	if msg, _ := r.summary(); !strings.Contains(msg, "At least") {
		t.Errorf("partial summary %q", msg)
	}
}

func TestCheckStorageUnwritableRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o700) })
	r := checkStorage(root, 0, true)
	if r.Writable || r.OK() {
		t.Errorf("read-only root: %+v", r)
	}
	if msg, bad := r.summary(); !bad || !strings.Contains(msg, "NOT writable") {
		t.Errorf("summary %q", msg)
	}
}

func TestStorageSettingsSaveCreatesFoldersAndNeverDeletes(t *testing.T) {
	s := securityTestServer(t)
	s.dataDir = t.TempDir()
	root := filepath.Join(t.TempDir(), "media")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "existing.mp4")
	if err := os.WriteFile(keep, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	w := post(t, s, "/settings/storage", url.Values{"media_root": {root + "/"}, "storage_limit_gb": {"120"}})
	f := flashFrom(t, w.Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "Created incoming, temp, completed, failed, archive") || w.Header().Get("Location") != "/settings/storage" {
		t.Fatalf("flash %+v, location %q", f, w.Header().Get("Location"))
	}
	saved, err := load(s.cfgPath)
	if err != nil || saved.MediaRoot != root || saved.StorageLimitGB != 120 {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("existing file disappeared")
	}
	if fi, err := os.Stat(filepath.Join(root, "temp")); err != nil || fi.Mode().Perm() != 0o750 {
		t.Errorf("temp folder: %v %v", fi, err)
	}

	// A root that does not exist is saved, and nothing is created.
	absent := filepath.Join(t.TempDir(), "later")
	f = flashFrom(t, post(t, s, "/settings/storage", url.Values{"media_root": {absent}, "storage_limit_gb": {""}}).Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "does not exist yet") {
		t.Errorf("absent root flash %+v", f)
	}
	if _, err := os.Stat(absent); err == nil {
		t.Error("created the missing media root")
	}
	if s.cfg.MediaRoot != absent || s.cfg.StorageLimitGB != 0 {
		t.Errorf("settings %+v", s.cfg)
	}
}

func TestStorageSettingsRejectBadInputWithoutChange(t *testing.T) {
	s := securityTestServer(t)
	before := s.cfg
	for _, form := range []url.Values{
		{"media_root": {"relative"}, "storage_limit_gb": {"1"}},
		{"media_root": {"/srv/../etc/x"}, "storage_limit_gb": {"1"}},
		{"media_root": {""}, "storage_limit_gb": {"1"}},
		{"media_root": {"/srv/reel/media"}, "storage_limit_gb": {"-5"}},
		{"media_root": {"/srv/reel/media"}, "storage_limit_gb": {"lots"}},
	} {
		f := flashFrom(t, post(t, s, "/settings/storage", form).Result().Cookies())
		if !f.Error || !strings.Contains(f.Text, "not saved") {
			t.Errorf("%v: flash %+v", form, f)
		}
	}
	if !reflect.DeepEqual(s.cfg, before) {
		t.Errorf("rejected forms changed settings: %+v", s.cfg)
	}
}

func TestStorageCheckHandlerUsesTypedValuesWithoutSaving(t *testing.T) {
	s := securityTestServer(t)
	root := t.TempDir()
	saved := s.cfg
	f := flashFrom(t, post(t, s, "/settings/storage/check", url.Values{"media_root": {root}, "storage_limit_gb": {"5"}}).Result().Cookies())
	if f.Error || !strings.Contains(f.Text, "writable") || !strings.Contains(f.Text, "5 GB limit") || !strings.Contains(f.Text, "Missing folders") {
		t.Errorf("flash %+v", f)
	}
	if !reflect.DeepEqual(s.cfg, saved) {
		t.Error("check changed the settings")
	}
	f = flashFrom(t, post(t, s, "/settings/storage/check", url.Values{"media_root": {filepath.Join(root, "nope")}}).Result().Cookies())
	if !f.Error || !strings.Contains(f.Text, "does not exist") {
		t.Errorf("missing root flash %+v", f)
	}
	f = flashFrom(t, post(t, s, "/settings/storage/check", url.Values{"media_root": {"nope"}}).Result().Cookies())
	if !f.Error || !strings.Contains(f.Text, "Test not run") {
		t.Errorf("invalid root flash %+v", f)
	}
}

func TestStoragePageShowsRootLimitAndFolders(t *testing.T) {
	s := securityTestServer(t)
	s.dataDir = t.TempDir()
	body := get(t, s.handler(), "/settings/storage").Body.String()
	for _, want := range []string{filepath.Join(s.dataDir, "media"), "Media root", "Storage limit (GB)", "<code>incoming/</code>", "<code>archive/</code>", "missing", "does not exist yet", "Data directory (current)"} {
		if !strings.Contains(body, want) {
			t.Errorf("default page lacks %q", want)
		}
	}
	if strings.Contains(body, "Not yet configurable") {
		t.Error("placeholder inputs remain")
	}
	root := filepath.Join(t.TempDir(), "m")
	os.Mkdir(root, 0o750)
	ensureMediaFolders(root)
	s.cfg.MediaRoot, s.cfg.StorageLimitGB = root, 42
	body = get(t, s.handler(), "/settings/storage").Body.String()
	if !strings.Contains(body, `value="`+root+`"`) || !strings.Contains(body, `value="42"`) || strings.Contains(body, "does not exist yet") || strings.Contains(body, ">missing<") {
		t.Errorf("configured page wrong:\n%s", body)
	}
}

func TestEffectiveMediaRootDefaultsBesideDataDirectory(t *testing.T) {
	if got := effectiveMediaRoot(Settings{}, "/srv/reel/data"); got != "/srv/reel/data/media" {
		t.Errorf("default root %q", got)
	}
	if got := effectiveMediaRoot(Settings{MediaRoot: "/mnt/media"}, "/srv/reel/data"); got != "/mnt/media" {
		t.Errorf("saved root %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	for in, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB", 5 << 20: "5.0 MB", 3 << 30: "3.0 GB", 2 << 40: "2.0 TB", -4: "0 B"} {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
