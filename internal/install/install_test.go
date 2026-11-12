package install

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRun struct {
	t        *testing.T
	log      []string
	userMade bool
	pkgs     map[string]bool
	active   map[string]string
	fail     map[string]error
	env      *Installer
}

func (f *fakeRun) Run(c Cmd) (string, error) {
	line := c.Name + " " + strings.Join(c.Args, " ")
	f.log = append(f.log, line)
	for k, err := range f.fail {
		if strings.Contains(line, k) {
			return "", err
		}
	}
	switch c.Name {
	case "runuser":
		if strings.Contains(line, "podman ps --format {{.Names}}") {
			return "containers_server_1\ncontainers_media_1\ncontainers_ai_1\n", nil
		}
	case "dpkg-query":
		var b strings.Builder
		for p := range f.pkgs {
			b.WriteString("ii  " + p + "\n")
		}
		return b.String(), nil
	case "apt-get":
		if c.Args[0] == "install" {
			for _, p := range []string{"podman", "podman-compose", "conmon", "crun", "fonts-dejavu-core"} {
				f.pkgs[p] = true
			}
		}
	case "useradd":
		f.userMade = true
	case "userdel":
		f.userMade = false
	case "loginctl":
		if c.Args[0] == "enable-linger" {
			os.MkdirAll(f.env.RuntimeDir(988), 0o755)
		}
	case "systemctl":
		if c.Args[0] == "enable" || c.Args[0] == "start" || c.Args[0] == "restart" {
			f.active[serverUnit], f.active[workerUnit] = "active", "active"
		}
		if c.Args[0] == "disable" || c.Args[0] == "stop" {
			f.active[serverUnit], f.active[workerUnit] = "inactive", "inactive"
		}
		if c.Args[0] == "is-active" {
			return f.active[c.Args[1]] + "\n", nil
		}
	}
	return "", nil
}

func (f *fakeRun) ran(sub string) int {
	n := 0
	for _, l := range f.log {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

func newTest(t *testing.T) (*Installer, *fakeRun, string) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin-reelanvil")
	os.WriteFile(exe, []byte("binary-v1"), 0o755)
	f := &fakeRun{t: t, pkgs: map[string]bool{"bash": true}, active: map[string]string{}}
	in := &Installer{
		Root: filepath.Join(dir, "opt", "ReelAnvil"), Port: 4747, Service: true, Yes: true,
		Out: &strings.Builder{}, In: bufio.NewReader(strings.NewReader("")),
		Run: f, Version: "t1", Exe: exe,
		StatePath:  filepath.Join(dir, "etc", "reelanvil", "install.json"),
		UnitDir:    filepath.Join(dir, "units"),
		RuntimeDir: func(uid int) string { return filepath.Join(dir, "run", "user", "988") },
		ComposeBin: "/usr/bin/podman-compose",
		IsRoot:     true,
		Lookup: func(string) (int, int, error) {
			if f.userMade {
				return 988, 988, nil
			}
			return 0, 0, os.ErrNotExist
		},
		Sleep: func(time.Duration) {},
		Get:   func(string) (int, []byte, error) { return 200, []byte(`{"tag_name":"2026.11.01"}`), nil },
		Dial: func(string, string, time.Duration) (net.Conn, error) {
			c, _ := net.Pipe()
			return c, nil
		},
	}
	os.MkdirAll(in.UnitDir, 0o755)
	f.env = in
	return in, f, dir
}

func TestInstallServiceRecordsPackagesAndWritesUID(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	st, err := in.loadState()
	if err != nil || !st.Complete || !st.UserCreated || st.UID != 988 {
		t.Fatalf("state = %+v, %v", st, err)
	}
	want := "conmon crun fonts-dejavu-core podman podman-compose"
	if got := strings.Join(st.Packages, " "); got != want {
		t.Errorf("packages = %q, want %q", got, want)
	}
	worker, _ := os.ReadFile(filepath.Join(in.UnitDir, workerUnit))
	for _, s := range []string{"user@988.service", "XDG_RUNTIME_DIR=/run/user/988", "User=reelanvil"} {
		if !strings.Contains(string(worker), s) {
			t.Errorf("worker unit lacks %q", s)
		}
	}
	server, _ := os.ReadFile(filepath.Join(in.UnitDir, serverUnit))
	if !strings.Contains(string(server), "--addr 0.0.0.0:4747") || !strings.Contains(string(server), in.Root+"/server/reelanvil serve") {
		t.Errorf("server unit wrong:\n%s", server)
	}
	if f.ran("--no-create-home") != 1 || f.ran("enable --now reelanvil.service") != 1 {
		t.Errorf("commands: %v", f.log)
	}
	compose, _ := os.ReadFile(filepath.Join(in.Root, "containers", "compose.yaml"))
	if !strings.Contains(string(compose), "nandyalu/python-ffmpeg:latest") || !strings.Contains(string(compose), in.Root+"/media:") {
		t.Errorf("compose not rendered for root:\n%s", compose)
	}
}

func TestInstallDoesNotRecordPreinstalledPodman(t *testing.T) {
	in, f, _ := newTest(t)
	f.pkgs["podman"], f.pkgs["podman-compose"], f.pkgs["fonts-dejavu-core"] = true, true, true
	in.Service = false
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	st, _ := in.loadState()
	if len(st.Packages) != 0 || f.ran("apt-get") != 0 {
		t.Errorf("packages=%v log=%v", st.Packages, f.log)
	}
	if _, err := os.Stat(filepath.Join(in.UnitDir, serverUnit)); err == nil {
		t.Error("plain install wrote units")
	}
}

func TestUninstallOrderAndExactPurge(t *testing.T) {
	in, f, _ := newTest(t)
	in.DeleteData = true
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	f.log = nil
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	order := []string{"disable --now", "podman system reset -f", "disable-linger", "daemon-reload", "userdel", "apt-get purge -y conmon crun fonts-dejavu-core podman podman-compose"}
	last := -1
	for _, want := range order {
		idx := -1
		for k, l := range f.log {
			if strings.Contains(l, want) {
				idx = k
				break
			}
		}
		if idx < 0 || idx < last {
			t.Fatalf("%q out of order or missing (idx %d, last %d):\n%s", want, idx, last, strings.Join(f.log, "\n"))
		}
		last = idx
	}
	if _, err := os.Stat(in.Root); !os.IsNotExist(err) {
		t.Error("install dir kept with --delete-data")
	}
	if _, err := os.Stat(in.StatePath); !os.IsNotExist(err) {
		t.Error("state kept")
	}
	for _, u := range []string{serverUnit, workerUnit} {
		if _, err := os.Stat(filepath.Join(in.UnitDir, u)); !os.IsNotExist(err) {
			t.Errorf("%s kept", u)
		}
	}
}

func TestUninstallWaitsForUnitsInactive(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	polls := 0
	in.Run = runnerFunc(func(c Cmd) (string, error) {
		if c.Name == "systemctl" && c.Args[0] == "is-active" && c.Args[1] == workerUnit {
			polls++
			if polls < 4 {
				return "deactivating\n", nil
			}
			return "inactive\n", nil
		}
		if c.Name == "systemctl" && c.Args[0] == "is-active" {
			return "inactive\n", nil
		}
		return f.Run(c)
	})
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if polls < 4 {
		t.Errorf("did not wait: %d polls", polls)
	}
}

type runnerFunc func(Cmd) (string, error)

func (r runnerFunc) Run(c Cmd) (string, error) { return r(c) }

func TestUninstallRejectsTamperedPackageList(t *testing.T) {
	in, _, _ := newTest(t)
	in.saveState(State{Root: in.Root, Port: 4747, Complete: true, Packages: []string{"podman", "--allow-remove-essential"}})
	if err := in.Uninstall(); err == nil || !strings.Contains(err.Error(), "invalid package name") {
		t.Errorf("err = %v", err)
	}
}

func TestUninstallRejectsUnsafeRoot(t *testing.T) {
	in, _, _ := newTest(t)
	in.saveState(State{Root: "/", Complete: true})
	if err := in.Uninstall(); err == nil {
		t.Error("accepted root /")
	}
}

func TestUninstallKeepsRecordOnFailure(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	f.fail = map[string]error{"apt-get purge": os.ErrPermission}
	if err := in.Uninstall(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(in.StatePath); err != nil {
		t.Error("record removed despite failure")
	}
}

func TestUpgradeDryRunChangesNothing(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(in.Exe, []byte("binary-v2"), 0o755)
	f.log = nil
	in.DryRun = true
	if err := in.Upgrade(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(in.Root, "server", "reelanvil"))
	if string(got) != "binary-v1" || f.ran("apt-get") != 0 || f.ran("systemctl") != 0 || f.ran("pull") != 0 {
		t.Errorf("dry run changed things: %q %v", got, f.log)
	}
	if !strings.Contains(in.Out.(*strings.Builder).String(), "replace, keeping reelanvil.previous") {
		t.Error("plan does not mention the binary")
	}
}

func TestUpgradeReplacesAndPulls(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(in.Exe, []byte("binary-v2"), 0o755)
	f.log = nil
	if err := in.Upgrade(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(in.Root, "server", "reelanvil"))
	prev, _ := os.ReadFile(filepath.Join(in.Root, "server", "reelanvil.previous"))
	if string(got) != "binary-v2" || string(prev) != "binary-v1" {
		t.Errorf("binary %q previous %q", got, prev)
	}
	if f.ran("podman-compose pull") != 1 || f.ran("restart reelanvil.service reelanvil-worker.service") != 1 {
		t.Errorf("log: %v", f.log)
	}
}

func TestInstallOnExistingInstallOffersUpgrade(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	in.Yes = false
	in.In = bufio.NewReader(strings.NewReader("n\n"))
	f.log = nil
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	if len(f.log) != 0 {
		t.Errorf("declined upgrade still ran: %v", f.log)
	}
}

func TestDoctorRepairRestoresUnitsButNotBinary(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(in.UnitDir, workerUnit))
	os.Remove(filepath.Join(in.Root, "containers", "compose.yaml"))
	os.WriteFile(in.Exe, []byte("binary-v2"), 0o755)
	f.log = nil
	if err := in.Doctor(true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(in.UnitDir, workerUnit)); err != nil {
		t.Error("worker unit not restored")
	}
	if _, err := os.Stat(filepath.Join(in.Root, "containers", "compose.yaml")); err != nil {
		t.Error("compose not restored")
	}
	got, _ := os.ReadFile(filepath.Join(in.Root, "server", "reelanvil"))
	if string(got) != "binary-v1" {
		t.Error("repair changed the installed version")
	}
	if f.ran("podman system reset") != 0 {
		t.Error("reset without --clean")
	}
}

func TestDoctorCleanResetsPodman(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	f.log = nil
	if err := in.Doctor(true, true); err != nil {
		t.Fatal(err)
	}
	if f.ran("podman system reset -f") != 1 {
		t.Errorf("log: %v", f.log)
	}
}

func TestDoctorMediaReportSuggestsUpgradeWhenBehind(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	st, _ := in.loadState()
	os.MkdirAll(filepath.Join(st.Root, "server", "logs"), 0o755)
	os.WriteFile(filepath.Join(st.Root, "server", "logs", "download-error.txt"), []byte("ERROR: Unable to extract\nmore"), 0o644)
	in.Run = runnerFunc(func(c Cmd) (string, error) {
		joined := strings.Join(c.Args, " ")
		switch {
		case strings.Contains(joined, "label=com.docker.compose.service=media"):
			return "containers_media_1\n", nil
		case strings.Contains(joined, "yt-dlp --version"):
			return "2026.07.04\n", nil
		}
		return f.Run(c)
	})
	out := in.Out.(*strings.Builder)
	out.Reset()
	if err := in.Doctor(false, false); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"2026.07.04, latest release: 2026.11.01 (behind)", "Unable to extract", "reelanvil upgrade"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("missing %q in:\n%s", s, out.String())
		}
	}
	if f.ran("pip") != 0 || f.ran("yt-dlp -U") != 0 {
		t.Error("doctor tried to update yt-dlp")
	}
}

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		less bool
	}{
		{"2026.07.04", "2026.10.26", true},
		{"2026.10.26", "2026.10.26", false},
		{"2026.10.26", "2026.10.26.1", true},
		{"2026.11.01", "2026.10.26", false},
	}
	for _, c := range cases {
		if versionLess(c.a, c.b) != c.less {
			t.Errorf("versionLess(%s,%s) != %v", c.a, c.b, c.less)
		}
	}
}

func TestValidateRequiresRootAndSafePath(t *testing.T) {
	in, _, _ := newTest(t)
	in.IsRoot = false
	if in.validate() == nil {
		t.Error("non-root accepted")
	}
	in.IsRoot, in.Root = true, "/opt/../etc"
	if in.validate() == nil {
		t.Error("unclean path accepted")
	}
}

// ---- review findings ----

func installed(t *testing.T) (*Installer, *fakeRun, string) {
	t.Helper()
	in, f, dir := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	f.log = nil
	return in, f, dir
}

func TestCheckRootRefusesSystemAndOddPaths(t *testing.T) {
	bad := []string{
		"/", "/usr", "/usr/local/x", "/etc/reelanvil", "/var", "/var/lib", "/var/lib/containers", "/bin/x",
		"/lib64/x", "/boot/x", "/proc/x", "/sys/x", "/dev/x", "/run/x", "/root/x", "/home", "/home/alice", "/tmp", "/tmp/x",
		"/opt/Reel Anvil", "/opt/a;b", "/opt/$HOME", "/opt/a\nb", "/opt/../etc", "opt/x",
	}
	for _, root := range bad {
		if checkRoot(root) == nil {
			t.Errorf("accepted %q", root)
		}
	}
	for _, root := range []string{"/opt/ReelAnvil", "/srv/reel-anvil_1.0", "/home/alice/reelanvil", "/var/lib/reelanvil", "/tmp/a/b"} {
		if err := checkRoot(root); err != nil {
			t.Errorf("rejected %q: %v", root, err)
		}
	}
}

func TestStateWithSystemRootIsRefused(t *testing.T) {
	in, f, _ := newTest(t)
	for _, root := range []string{"/usr/lib/x", "/home/alice", "/opt/a b"} {
		in.saveState(State{Root: root, Complete: true})
		if err := in.Uninstall(); err == nil || !strings.Contains(err.Error(), "unsafe install directory") {
			t.Errorf("%s: err = %v", root, err)
		}
	}
	if len(f.log) != 0 {
		t.Errorf("ran commands: %v", f.log)
	}
}

func TestInstallRefusesNonEmptyUnmarkedDirectory(t *testing.T) {
	in, f, _ := newTest(t)
	os.MkdirAll(in.Root, 0o700)
	os.WriteFile(filepath.Join(in.Root, "mine.txt"), []byte("x"), 0o600)
	if err := in.Install(); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("err = %v", err)
	}
	if len(f.log) != 0 {
		t.Errorf("ran commands: %v", f.log)
	}
	if fi, _ := os.Stat(in.Root); fi.Mode().Perm() != 0o700 {
		t.Errorf("mode changed to %v", fi.Mode().Perm())
	}
	os.WriteFile(filepath.Join(in.Root, markerFile), nil, 0o644)
	if err := in.Install(); err != nil {
		t.Errorf("marked directory refused: %v", err)
	}
}

func TestInstallAcceptsEmptyExistingDirectory(t *testing.T) {
	in, _, _ := newTest(t)
	os.MkdirAll(in.Root, 0o755)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	if !hasMarker(in.Root) {
		t.Error("no marker written")
	}
}

func TestUninstallKeepsDataWithoutMarker(t *testing.T) {
	in, _, _ := installed(t)
	in.DeleteData = true
	os.Remove(filepath.Join(in.Root, markerFile))
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(in.Root); err != nil {
		t.Error("deleted a directory without the marker")
	}
	if !strings.Contains(in.Out.(*strings.Builder).String(), "Not deleting") {
		t.Error("no explanation printed")
	}
}

func TestUninstallRemovesContainerDirsOnlyWithPodmanRecorded(t *testing.T) {
	for _, tc := range []struct {
		pkgs    []string
		removed bool
	}{{[]string{"conmon", "crun"}, false}, {[]string{"conmon", "podman"}, true}, {[]string{"containers-common"}, true}} {
		in, _, dir := newTest(t)
		a, b := filepath.Join(dir, "etc-containers"), filepath.Join(dir, "lib-containers")
		os.MkdirAll(a, 0o755)
		os.MkdirAll(b, 0o755)
		in.ContainerDirs = []string{a, b}
		in.saveState(State{Root: in.Root, Complete: true, Packages: tc.pkgs})
		if err := in.Uninstall(); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat(a)
		if os.IsNotExist(err) != tc.removed {
			t.Errorf("packages %v: removed = %v, want %v", tc.pkgs, os.IsNotExist(err), tc.removed)
		}
	}
}

func TestUninstallSkipsPurgeThatWouldRemoveOtherPackages(t *testing.T) {
	in, f, dir := installed(t)
	keep := filepath.Join(dir, "etc-containers")
	os.MkdirAll(keep, 0o755)
	in.ContainerDirs = []string{keep}
	in.Run = runnerFunc(func(c Cmd) (string, error) {
		if line := c.Name + " " + strings.Join(c.Args, " "); strings.HasPrefix(line, "apt-get -s purge") {
			f.log = append(f.log, line)
			return "Purg podman [5.4]\nPurg crun [1.0]\nPurg cockpit-podman [9]\nRemv other-app [1]\n", nil
		}
		return f.Run(c)
	})
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if f.ran("apt-get purge") != 0 {
		t.Errorf("purged anyway: %v", f.log)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("container dir removed although nothing was purged")
	}
	if out := in.Out.(*strings.Builder).String(); !strings.Contains(out, "cockpit-podman, other-app") {
		t.Errorf("extra packages not named:\n%s", out)
	}
}

func TestWriteAtomicIgnoresPredictableTempLink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	os.WriteFile(victim, []byte("keep"), 0o600)
	target := filepath.Join(dir, "file")
	for _, name := range []string{target + ".tmp", target + ".tmp-1"} {
		os.Symlink(victim, name)
	}
	if err := writeAtomic(target, []byte("new"), 0o644, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("wrote through a link: %q", b)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("target = %q", b)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	if err := writeAtomic(filepath.Join(dir, "link", "x"), nil, 0o644, nil); err == nil {
		t.Error("wrote into a missing dir")
	}
}

func TestLayoutRefusesSymlinkedFolders(t *testing.T) {
	in, _, dir := installed(t)
	out := filepath.Join(dir, "outside")
	os.MkdirAll(out, 0o700)
	var chowned []string
	in.Chown = func(p string, _, _ int) error { chowned = append(chowned, p); return nil }
	st, _ := in.loadState()

	server := filepath.Join(in.Root, "server")
	os.RemoveAll(server)
	os.Symlink(out, server)
	if err := in.ensureLayout(st, 988); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("symlinked server folder: err = %v", err)
	}
	if fi, _ := os.Stat(out); fi.Mode().Perm() != 0o700 {
		t.Errorf("chmod followed the link: %v", fi.Mode().Perm())
	}
	for _, p := range chowned {
		if p == server || strings.HasPrefix(p, out) {
			t.Errorf("chown touched %s", p)
		}
	}

	os.Remove(server)
	os.MkdirAll(server, 0o755)
	secret := filepath.Join(dir, "secret")
	os.WriteFile(secret, []byte("secret"), 0o600)
	os.Symlink(secret, filepath.Join(server, "reelanvil"))
	if _, err := in.ensureBinary(st, 988, true); err == nil {
		t.Error("followed a symlinked binary")
	}
	if _, err := os.Stat(filepath.Join(server, "reelanvil.previous")); err == nil {
		t.Error("copied the link target into reelanvil.previous")
	}
}

func TestUninstallYesDoesNotDeleteData(t *testing.T) {
	in, _, _ := installed(t)
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if !hasMarker(in.Root) {
		t.Error("--yes deleted the data")
	}
	if _, err := os.Stat(in.StatePath); !os.IsNotExist(err) {
		t.Error("record kept")
	}
}

func TestUninstallInteractiveYesDeletesData(t *testing.T) {
	in, _, _ := installed(t)
	in.Yes = false
	in.In = bufio.NewReader(strings.NewReader("y\ny\ny\n"))
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(in.Root); !os.IsNotExist(err) {
		t.Error("data kept after an interactive yes")
	}
}

func TestInstallServiceFlagAddsUnitsToExistingInstall(t *testing.T) {
	in, f, _ := newTest(t)
	in.Service = false
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(in.UnitDir, serverUnit)); err == nil {
		t.Fatal("plain install wrote units")
	}
	f.log = nil
	in.Service, in.Port = true, 9999
	in.Root = filepath.Join(filepath.Dir(in.Root), "Other")
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	st, _ := in.loadState()
	if !st.Service || st.Port != 4747 {
		t.Errorf("record = %+v", st)
	}
	for _, u := range []string{serverUnit, workerUnit} {
		if _, err := os.Stat(filepath.Join(in.UnitDir, u)); err != nil {
			t.Errorf("%s not written", u)
		}
	}
	if f.ran("daemon-reload") != 1 || f.ran("enable --now reelanvil.service") != 1 || f.ran("pull") != 0 {
		t.Errorf("log: %v", f.log)
	}
	out := in.Out.(*strings.Builder).String()
	if !strings.Contains(out, "--dir") || !strings.Contains(out, "--port 9999 is ignored") {
		t.Errorf("no note about ignored flags:\n%s", out)
	}
}

func TestUninstallRerunAfterPartialUninstall(t *testing.T) {
	in, f, _ := installed(t)
	in.DeleteData = true
	f.fail = map[string]error{"apt-get purge": os.ErrPermission}
	if err := in.Uninstall(); err == nil {
		t.Fatal("expected the purge failure")
	}
	// Units, user and data are gone now; systemctl disable fails on missing units.
	f.fail = map[string]error{"disable --now": errors.New("Unit file reelanvil.service does not exist")}
	f.log = nil
	if err := in.Uninstall(); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if f.ran("disable --now") != 0 {
		t.Errorf("tried to disable missing units: %v", f.log)
	}
	if _, err := os.Stat(in.StatePath); !os.IsNotExist(err) {
		t.Error("record kept after a clean rerun")
	}
}

func TestUninstallKeepsUserItDidNotCreate(t *testing.T) {
	in, f, _ := newTest(t)
	f.userMade = true // exists before install
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	if st, _ := in.loadState(); st.UserCreated {
		t.Fatal("recorded a pre-existing user as created")
	}
	f.log = nil
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if f.ran("userdel") != 0 || !f.userMade {
		t.Errorf("removed a user it did not create: %v", f.log)
	}
}

func TestUninstallRemovesUserItCreated(t *testing.T) {
	in, f, _ := installed(t)
	if err := in.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if f.ran("userdel") != 1 {
		t.Errorf("log: %v", f.log)
	}
}

func TestRepairChownsTreeWhenUserGetsNewUID(t *testing.T) {
	in, f, _ := installed(t)
	chowned := map[string]int{}
	in.Chown = func(p string, uid, _ int) error { chowned[p] = uid; return nil }
	in.Lookup = func(string) (int, int, error) {
		if f.userMade {
			return 1001, 1001, nil
		}
		return 0, 0, os.ErrNotExist
	}
	f.userMade = false // the user was deleted
	if err := in.Doctor(true, false); err != nil {
		t.Fatal(err)
	}
	st, _ := in.loadState()
	if st.UID != 1001 {
		t.Errorf("record uid = %d", st.UID)
	}
	for _, p := range []string{in.Root, filepath.Join(in.Root, "server", "reelanvil"), filepath.Join(in.Root, "containers", "compose.yaml")} {
		if chowned[p] != 1001 {
			t.Errorf("%s not given to uid 1001 (got %d)", p, chowned[p])
		}
	}
	if !strings.Contains(in.Out.(*strings.Builder).String(), "subuid") {
		t.Error("no subuid warning")
	}
}

func TestDoctorFailsWhenChecksFail(t *testing.T) {
	in, _, _ := installed(t)
	if err := in.Doctor(false, false); err != nil {
		t.Fatalf("healthy install: %v", err)
	}
	os.Remove(filepath.Join(in.Root, "containers", "compose.yaml"))
	if err := in.Doctor(false, false); err == nil {
		t.Error("doctor returned nil with FAIL lines")
	}
}

func TestDoctorRepairFailsWhenProblemsRemain(t *testing.T) {
	in, f, _ := installed(t)
	os.Remove(filepath.Join(in.Root, "containers", "compose.yaml"))
	in.Run = runnerFunc(func(c Cmd) (string, error) {
		if strings.Contains(strings.Join(c.Args, " "), "podman ps --format") {
			return "", nil // containers never come up
		}
		return f.Run(c)
	})
	if err := in.Doctor(true, false); err == nil || !strings.Contains(err.Error(), "still failing") {
		t.Errorf("err = %v", err)
	}
}

func TestDoctorRepairReportsStopFailures(t *testing.T) {
	in, f, _ := installed(t)
	os.Remove(filepath.Join(in.Root, "containers", "compose.yaml"))
	f.fail = map[string]error{"systemctl stop": fmt.Errorf("stop refused")}
	err := in.Doctor(true, false)
	if err == nil || !strings.Contains(err.Error(), "stop refused") {
		t.Fatalf("err = %v", err)
	}
	if _, serr := os.Stat(filepath.Join(in.Root, "containers", "compose.yaml")); serr != nil {
		t.Error("repair did not carry on after the stop failure")
	}
}

func TestDoctorRepairReportsUnitsThatDoNotStop(t *testing.T) {
	in, f, _ := installed(t)
	os.Remove(filepath.Join(in.Root, "containers", "compose.yaml"))
	in.Run = runnerFunc(func(c Cmd) (string, error) {
		if c.Name == "systemctl" && c.Args[0] == "is-active" {
			return "active\n", nil
		}
		return f.Run(c)
	})
	if err := in.Doctor(true, false); err == nil || !strings.Contains(err.Error(), "timed out waiting for units to stop") {
		t.Errorf("err = %v", err)
	}
}

// An install made before a package became required gets it on upgrade, and
// uninstall will remove it.
func TestUpgradeInstallsNewlyRequiredPackages(t *testing.T) {
	in, f, _ := newTest(t)
	if err := in.Install(); err != nil {
		t.Fatal(err)
	}
	delete(f.pkgs, "fonts-dejavu-core")
	os.WriteFile(in.Exe, []byte("binary-v2"), 0o755)
	f.log = nil
	if err := in.Upgrade(); err != nil {
		t.Fatal(err)
	}
	if f.ran("apt-get install -y fonts-dejavu-core") != 1 {
		t.Errorf("log: %v", f.log)
	}
	if st, _ := in.loadState(); !strings.Contains(strings.Join(st.Packages, " "), "fonts-dejavu-core") {
		t.Errorf("package not recorded: %v", st.Packages)
	}
}
