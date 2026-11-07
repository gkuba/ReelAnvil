// Package install sets ReelAnvil up on a Debian 13 host: rootless Podman under
// one system user, the folder layout, the Compose file and (with --service)
// the systemd units. It is the Go form of the manual install proven on pixelreel.
// Every step is idempotent so install, upgrade and doctor --repair share code.
package install

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

const (
	DefaultRoot      = "/opt/ReelAnvil"
	DefaultPort      = 4747
	DefaultStatePath = "/etc/reelanvil/install.json"
	DefaultUnitDir   = "/etc/systemd/system"
	User             = "reelanvil"
	serverUnit       = "reelanvil.service"
	workerUnit       = "reelanvil-worker.service"
	// markerFile is written into Root by the installer. Uninstall deletes
	// data, and install accepts a non-empty directory, only where it exists.
	markerFile = ".reelanvil-install"
)

//go:embed assets/*.tmpl
var assets embed.FS

var podmanPackages = []string{"podman", "podman-compose"}

// packageName guards the purge list read back from the state file.
var packageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)

var rootChars = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// checkRoot refuses install directories that are unclean, use unusual
// characters or sit in or under system locations: install chowns the
// directory, useradd makes it the home and uninstall can delete it.
func checkRoot(root string) error { return checkDir("install directory", root) }

// checkDir applies checkRoot's rules to any folder the installer creates or
// mounts; what names it in the messages.
func checkDir(what, root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.Count(root, "/") < 2 {
		return fmt.Errorf("%s %q must be a clean absolute path such as %s", what, root, DefaultRoot)
	}
	if !rootChars.MatchString(root) {
		return fmt.Errorf("%s %q may only contain letters, digits and . _ - /", what, root)
	}
	parts := strings.Split(root[1:], "/")
	top := parts[0]
	switch {
	case strings.HasPrefix(top, "lib"):
		return fmt.Errorf("%s %q is a system location", what, root)
	case top == "usr" || top == "etc" || top == "bin" || top == "sbin" || top == "boot" ||
		top == "proc" || top == "sys" || top == "dev" || top == "run" || top == "root":
		return fmt.Errorf("%s %q is a system location", what, root)
	case (top == "var" || top == "home" || top == "tmp") && len(parts) <= 2:
		return fmt.Errorf("%s %q is a system location", what, root)
	case top == "var" && len(parts) >= 3 && parts[1] == "lib" && parts[2] == "containers":
		return fmt.Errorf("%s %q is a system location", what, root)
	}
	return nil
}

// hasMarker reports whether root holds the installer's marker file.
func hasMarker(root string) bool {
	fi, err := os.Lstat(filepath.Join(root, markerFile))
	return err == nil && fi.Mode().IsRegular()
}

// checkFreshRoot refuses an existing, non-empty directory the installer did
// not create.
func checkFreshRoot(root string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 || hasMarker(root) {
		return nil
	}
	return fmt.Errorf("%s exists, is not empty and was not created by ReelAnvil (no %s file): choose another --dir or empty it", root, markerFile)
}

// refuseSymlink fails when path is a symbolic link. A missing path is fine.
func refuseSymlink(path string) error {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link: refusing to follow it", path)
	}
	return nil
}

// Cmd is one external command.
type Cmd struct {
	Name   string
	Args   []string
	Env    []string // appended to the process environment
	Dir    string
	Stream bool // also show the output while it runs (apt, image pulls)
}

// Runner runs external commands and returns their combined output.
type Runner interface {
	Run(Cmd) (string, error)
}

// ExecRunner is the real Runner.
type ExecRunner struct{ Out io.Writer }

func (r ExecRunner) Run(c Cmd) (string, error) {
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.Dir = c.Dir
	var buf strings.Builder
	if c.Stream && r.Out != nil {
		w := io.MultiWriter(&buf, r.Out)
		cmd.Stdout, cmd.Stderr = w, w
	} else {
		cmd.Stdout, cmd.Stderr = &buf, &buf
	}
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", c.Name, strings.Join(c.Args, " "), err, tail(out))
	}
	return out, nil
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = "..." + s[len(s)-400:]
	}
	return s
}

// State is what the installer did, kept root-owned in /etc/reelanvil so the
// service user can never edit what root later removes.
type State struct {
	Root        string   `json:"root"`
	Port        int      `json:"port"`
	Service     bool     `json:"service"`
	UID         int      `json:"uid"`
	Version     string   `json:"version"`
	Complete    bool     `json:"complete"`
	UserCreated bool     `json:"user_created"`
	Packages    []string `json:"packages,omitempty"` // installed by the installer, purged by uninstall
}

// Installer holds the settings and the seams that tests replace.
type Installer struct {
	Root    string
	Port    int
	Service bool
	Yes     bool
	DryRun  bool
	// DeleteData lets uninstall delete Root; --yes alone never does.
	DeleteData bool

	Out     io.Writer
	In      *bufio.Reader
	Run     Runner
	Version string
	Exe     string // the running binary, copied into Root/server

	StatePath string
	UnitDir   string
	// RuntimeDir returns /run/user/UID.
	RuntimeDir func(uid int) string
	ComposeBin string
	// LatestURL serves {"tag_name": "..."} for the latest yt-dlp release.
	LatestURL string
	IsRoot    bool
	// ContainerDirs are removed by uninstall after Podman is purged.
	ContainerDirs []string

	Lookup func(name string) (uid, gid int, err error)
	Chown  func(path string, uid, gid int) error
	Sleep  func(time.Duration)
	Get    func(url string) (status int, body []byte, err error)
	// Dial opens the TCP connection the doctor uses to test a service
	// address; nil means net.DialTimeout.
	Dial func(network, addr string, timeout time.Duration) (net.Conn, error)
}

// New returns an Installer wired to the real host.
func New(out io.Writer, in io.Reader, version string) *Installer {
	exe, _ := os.Executable()
	compose, err := exec.LookPath("podman-compose")
	if err != nil {
		compose = "/usr/bin/podman-compose"
	}
	return &Installer{
		Root: DefaultRoot, Port: DefaultPort,
		Out: out, In: bufio.NewReader(in), Run: ExecRunner{Out: out},
		Version: version, Exe: exe,
		StatePath: DefaultStatePath, UnitDir: DefaultUnitDir,
		RuntimeDir:    func(uid int) string { return "/run/user/" + strconv.Itoa(uid) },
		ComposeBin:    compose,
		LatestURL:     "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest",
		IsRoot:        os.Geteuid() == 0,
		ContainerDirs: []string{"/etc/containers", "/var/lib/containers"},
		Lookup: func(name string) (int, int, error) {
			u, err := user.Lookup(name)
			if err != nil {
				return 0, 0, err
			}
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			return uid, gid, nil
		},
		Chown: os.Lchown,
		Sleep: time.Sleep,
		Get: func(url string) (int, []byte, error) {
			client := http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				return 0, nil, err
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			return resp.StatusCode, b, err
		},
	}
}

func (i *Installer) say(format string, a ...any) { fmt.Fprintf(i.Out, format+"\n", a...) }

// confirm asks a yes/no question; --yes answers yes.
func (i *Installer) confirm(question string, def bool) bool {
	if i.Yes {
		return true
	}
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Fprintf(i.Out, "%s %s ", question, hint)
	line, _ := i.In.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	}
	return def
}

func (i *Installer) validate() error {
	if !i.IsRoot {
		return errors.New("run this command with sudo (it creates a system user, units and packages)")
	}
	if err := checkRoot(i.Root); err != nil {
		return err
	}
	if i.Port < 1 || i.Port > 65535 {
		return fmt.Errorf("port %d is out of range", i.Port)
	}
	return nil
}

func (i *Installer) loadState() (State, error) {
	var st State
	b, err := os.ReadFile(i.StatePath)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, fmt.Errorf("%s: %w", i.StatePath, err)
	}
	if err := checkRoot(st.Root); err != nil {
		return st, fmt.Errorf("%s: unsafe install directory: %w", i.StatePath, err)
	}
	return st, nil
}

func (i *Installer) saveState(st State) error {
	if err := os.MkdirAll(filepath.Dir(i.StatePath), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return writeAtomic(i.StatePath, append(b, '\n'), 0o644, nil)
}

// writeAtomic writes through an exclusively created temp file in the target
// directory, so a planted file or link cannot redirect it, and refuses a
// directory that is a symbolic link.
func writeAtomic(path string, data []byte, mode os.FileMode, chown func(string) error) error {
	dir := filepath.Dir(path)
	if err := refuseSymlink(dir); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Chmod(mode)
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp)
		return werr
	}
	if chown != nil {
		if err := chown(tmp); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// asUser wraps a command to run as the service user with a rootless Podman
// environment.
func (i *Installer) asUser(st State, dir string, name string, args ...string) Cmd {
	full := []string{"-u", User, "--", "env", "HOME=" + st.Root, "XDG_RUNTIME_DIR=" + i.RuntimeDir(st.UID), name}
	return Cmd{Name: "runuser", Args: append(full, args...), Dir: dir}
}

func (i *Installer) systemctl(args ...string) (string, error) {
	return i.Run.Run(Cmd{Name: "systemctl", Args: args})
}

// poll calls fn every few seconds until it returns true, an error, or timeout.
func (i *Installer) poll(what string, timeout time.Duration, fn func() (bool, error)) error {
	const step = 2 * time.Second
	start := time.Duration(0)
	for {
		ok, err := fn()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if start >= timeout {
			return fmt.Errorf("timed out waiting for %s", what)
		}
		if start > 0 && start%(20*time.Second) == 0 {
			i.say("  still waiting for %s (%s elapsed)", what, start)
		}
		i.Sleep(step)
		start += step
	}
}

// unitState returns systemctl's active state word.
func (i *Installer) unitState(unit string) string {
	out, _ := i.systemctl("is-active", unit)
	return strings.TrimSpace(out)
}

func (i *Installer) installedPackages() (map[string]bool, error) {
	out, err := i.Run.Run(Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${db:Status-Abbrev} ${Package}\n"}})
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "ii" {
			set[f[1]] = true
		}
	}
	return set, nil
}

// ---- idempotent steps shared by install, upgrade and doctor --repair ----

type unitData struct {
	User, Root, ComposeBin string
	Port, UID              int
}

func (i *Installer) data(st State) unitData {
	return unitData{User: User, Root: st.Root, ComposeBin: i.ComposeBin, Port: st.Port, UID: st.UID}
}

func (i *Installer) renderUnit(st State, name string) ([]byte, error) {
	t, err := template.ParseFS(assets, "assets/"+name+".tmpl")
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	if err := t.Execute(&b, i.data(st)); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// renderCompose fills the Compose template from the install record and the
// settings file (Whisper model and language, media root).
func (i *Installer) renderCompose(st State) ([]byte, error) {
	d, err := i.composeValues(st)
	if err != nil {
		return nil, err
	}
	t, err := template.ParseFS(assets, "assets/compose.yaml.tmpl")
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	if err := t.Execute(&b, d); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func (i *Installer) chown(st State, gid int, path string) error {
	if i.Chown == nil {
		return nil
	}
	return i.Chown(path, st.UID, gid)
}

func (i *Installer) lookupUser(st *State) (gid int, err error) {
	uid, gid, err := i.Lookup(User)
	if err != nil {
		return 0, err
	}
	st.UID = uid
	return gid, nil
}

func (i *Installer) ensureUser(st *State) (int, error) {
	if gid, err := i.lookupUser(st); err == nil {
		return gid, nil
	}
	i.say("Creating system user %s", User)
	// -F gives the user subuid/subgid ranges for rootless Podman. No
	// --create-home: it would copy skel dotfiles into the install directory.
	if _, err := i.Run.Run(Cmd{Name: "useradd", Args: []string{"--system", "-F", "--user-group", "--home-dir", st.Root, "--no-create-home", "--shell", "/usr/sbin/nologin", User}}); err != nil {
		return 0, err
	}
	st.UserCreated = true
	return i.lookupUser(st)
}

func (i *Installer) mkdir(st State, gid int, path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	// Lstat: the service user owns these folders and could swap one for a link.
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link: refusing to follow it", path)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	return i.chown(st, gid, path)
}

func (i *Installer) ensureLayout(st State, gid int) error {
	if err := i.mkdir(st, gid, st.Root, 0o750); err != nil {
		return err
	}
	for _, d := range []string{"server", "containers"} {
		if err := i.mkdir(st, gid, filepath.Join(st.Root, d), 0o755); err != nil {
			return err
		}
	}
	if err := i.ensureMediaFolders(st, gid); err != nil {
		return err
	}
	return i.ensureMarker(st.Root)
}

// ensureMediaFolders creates the media root and its logical sub-folders for
// the service user. A folder that already exists is left exactly as it is, so
// a media root on another disk, or one with data in it, is never re-owned.
func (i *Installer) ensureMediaFolders(st State, gid int) error {
	root, err := i.mediaRoot(st)
	if err != nil {
		return err
	}
	for _, p := range append([]string{root}, mediaFolderPaths(root)...) {
		if _, err := os.Lstat(p); err == nil {
			if err := refuseSymlink(p); err != nil {
				return err
			}
			continue
		}
		mode := os.FileMode(0o750)
		if p == root {
			mode = 0o755
		}
		if err := i.mkdir(st, gid, p, mode); err != nil {
			return err
		}
	}
	return nil
}

// ensureMarker writes the root-owned marker file into Root.
func (i *Installer) ensureMarker(root string) error {
	if hasMarker(root) {
		return nil
	}
	return writeAtomic(filepath.Join(root, markerFile), []byte("Created by the ReelAnvil installer. Uninstall deletes this folder only where this file exists.\n"), 0o644, nil)
}

func (i *Installer) ensureLinger(st State) error {
	if _, err := i.Run.Run(Cmd{Name: "loginctl", Args: []string{"enable-linger", User}}); err != nil {
		return err
	}
	rt := i.RuntimeDir(st.UID)
	return i.poll("systemd to create "+rt, 30*time.Second, func() (bool, error) {
		_, err := os.Stat(rt)
		return err == nil, nil
	})
}

// ensureBinary copies the running binary into place. Without force it only
// fills a missing file (doctor --repair never changes the installed version).
func (i *Installer) ensureBinary(st State, gid int, force bool) (bool, error) {
	dst := filepath.Join(st.Root, "server", "reelanvil")
	if err := refuseSymlink(dst); err != nil {
		return false, err
	}
	if _, err := os.Stat(dst); err == nil && !force {
		return false, nil
	}
	if same(i.Exe, dst) {
		return false, nil
	}
	data, err := os.ReadFile(i.Exe)
	if err != nil {
		return false, err
	}
	if old, err := os.ReadFile(dst); err == nil {
		if string(old) == string(data) {
			return false, nil
		}
		if err := writeAtomic(dst+".previous", old, 0o755, func(p string) error { return i.chown(st, gid, p) }); err != nil {
			return false, err
		}
	}
	return true, writeAtomic(dst, data, 0o755, func(p string) error { return i.chown(st, gid, p) })
}

func same(a, b string) bool {
	x, err1 := os.Stat(a)
	y, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}

// ensureCompose writes the bundled Compose file. With overwrite false an
// existing file is kept (it may be edited).
func (i *Installer) ensureCompose(st State, gid int, overwrite bool) (bool, error) {
	dst := filepath.Join(st.Root, "containers", "compose.yaml")
	if err := refuseSymlink(dst); err != nil {
		return false, err
	}
	want, err := i.renderCompose(st)
	if err != nil {
		return false, err
	}
	if old, err := os.ReadFile(dst); err == nil {
		if string(old) == string(want) || !overwrite {
			return false, nil
		}
		if err := writeAtomic(dst+".bak", old, 0o644, func(p string) error { return i.chown(st, gid, p) }); err != nil {
			return false, err
		}
	}
	return true, writeAtomic(dst, want, 0o644, func(p string) error { return i.chown(st, gid, p) })
}

// ensureUnits writes both units and reports whether anything changed.
func (i *Installer) ensureUnits(st State) (bool, error) {
	changed := false
	for _, name := range []string{serverUnit, workerUnit} {
		want, err := i.renderUnit(st, name)
		if err != nil {
			return false, err
		}
		dst := filepath.Join(i.UnitDir, name)
		if old, err := os.ReadFile(dst); err == nil && string(old) == string(want) {
			continue
		}
		if err := writeAtomic(dst, want, 0o644, nil); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

// waitStarted waits for the worker (image pulls can take minutes) and the web app.
func (i *Installer) waitStarted(st State) error {
	i.say("Waiting for the worker (first start pulls the container images; this can take several minutes)")
	err := i.poll("the worker", 25*time.Minute, func() (bool, error) {
		switch s := i.unitState(workerUnit); s {
		case "active":
			return true, nil
		case "failed", "inactive":
			return false, fmt.Errorf("%s is %s: see journalctl -u %s", workerUnit, s, workerUnit)
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return i.waitHTTP(st)
}

func (i *Installer) waitHTTP(st State) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/", st.Port)
	return i.poll("the web app on "+url, 60*time.Second, func() (bool, error) {
		code, _, err := i.Get(url)
		return err == nil && code == 200, nil
	})
}

// ---- install ----

// Install runs the installer.
func (i *Installer) Install() error {
	if err := i.validate(); err != nil {
		return err
	}
	st, err := i.loadState()
	switch {
	case err == nil && st.Complete:
		i.say("ReelAnvil is already installed in %s.", st.Root)
		if st.Root != i.Root {
			i.say("  note: --dir %s is ignored; the install record says %s.", i.Root, st.Root)
		}
		if st.Port != i.Port {
			i.say("  note: --port %d is ignored; the install record says %d.", i.Port, st.Port)
		}
		if i.Service && !st.Service {
			return i.addService(st)
		}
		if !i.confirm("Run upgrade now?", true) {
			i.say("Nothing changed. Use `reelanvil doctor --repair` to fix problems.")
			return nil
		}
		return i.Upgrade()
	case err == nil: // an earlier run stopped half way: continue it
		i.say("Continuing an unfinished install (%s).", st.Root)
	case errors.Is(err, os.ErrNotExist):
		if err := checkFreshRoot(i.Root); err != nil {
			return err
		}
		st = State{Root: i.Root, Port: i.Port, Service: i.Service}
	default:
		return err
	}
	if i.Service {
		st.Service = true
	}
	st.Version = i.Version

	missing, err := i.missingPackages()
	if err != nil {
		return err
	}
	i.say("ReelAnvil install plan")
	if len(missing) > 0 {
		i.say("  - apt install %s (Debian packages)", strings.Join(missing, " "))
	}
	i.say("  - system user %s (rootless Podman, linger)", User)
	i.say("  - %s with server/ and containers/; the binary and the compose.yaml rendered from the settings", st.Root)
	i.say("  - the media folder (%s by default) with %s, owned by %s", filepath.Join(st.Root, defaultMediaDir), strings.Join(mediaFolders, ", "), User)
	i.say("  - server/settings/config.json: service addresses, Whisper model and language, media root (existing values are kept)")
	if st.Service {
		i.say("  - systemd units %s and %s, enabled and started on port %d", serverUnit, workerUnit, st.Port)
	} else {
		i.say("  - no systemd units (add them later with `install --service`)")
	}
	if !i.confirm("Proceed?", true) {
		i.say("Nothing changed.")
		return nil
	}

	if len(missing) > 0 {
		before, err := i.installedPackages()
		if err != nil {
			return err
		}
		i.say("Installing %s", strings.Join(missing, ", "))
		if _, err := i.Run.Run(Cmd{Name: "apt-get", Args: append([]string{"install", "-y"}, missing...), Env: []string{"DEBIAN_FRONTEND=noninteractive"}, Stream: true}); err != nil {
			return err
		}
		after, err := i.installedPackages()
		if err != nil {
			return err
		}
		for p := range after {
			if !before[p] {
				st.Packages = append(st.Packages, p)
			}
		}
		sort.Strings(st.Packages)
		if err := i.saveState(st); err != nil {
			return err
		}
	}
	gid, err := i.ensureUser(&st)
	if err != nil {
		return err
	}
	if err := i.saveState(st); err != nil {
		return err
	}
	if err := i.ensureLayout(st, gid); err != nil {
		return err
	}
	if err := i.ensureLinger(st); err != nil {
		return err
	}
	if _, err := i.ensureBinary(st, gid, true); err != nil {
		return err
	}
	if _, err := i.ensureCompose(st, gid, true); err != nil {
		return err
	}
	if _, err := i.ensureConfig(st, gid); err != nil {
		return err
	}
	if st.Service {
		if _, err := i.ensureUnits(st); err != nil {
			return err
		}
		if _, err := i.systemctl("daemon-reload"); err != nil {
			return err
		}
		if _, err := i.systemctl("enable", "--now", serverUnit); err != nil {
			return err
		}
		if err := i.waitStarted(st); err != nil {
			return err
		}
	}
	st.Complete = true
	if err := i.saveState(st); err != nil {
		return err
	}
	if st.Service {
		i.say("ReelAnvil is running: http://<this host>:%d/ (no login: trusted networks only)", st.Port)
	} else {
		i.say("Installed. Start by hand: runuser -u %s -- env HOME=%s XDG_RUNTIME_DIR=%s podman-compose -f %s/containers/compose.yaml up -d",
			User, st.Root, i.RuntimeDir(st.UID), st.Root)
	}
	return nil
}

// addService adds the systemd units to an install that was made without them.
func (i *Installer) addService(st State) error {
	i.say("  - systemd units %s and %s, enabled and started on port %d", serverUnit, workerUnit, st.Port)
	if !i.confirm("Add them now?", true) {
		i.say("Nothing changed.")
		return nil
	}
	st.Service = true
	if err := i.saveState(st); err != nil {
		return err
	}
	if _, err := i.ensureUnits(st); err != nil {
		return err
	}
	if _, err := i.systemctl("daemon-reload"); err != nil {
		return err
	}
	if _, err := i.systemctl("enable", "--now", serverUnit); err != nil {
		return err
	}
	if err := i.waitStarted(st); err != nil {
		return err
	}
	i.say("ReelAnvil is running: http://<this host>:%d/ (no login: trusted networks only)", st.Port)
	return nil
}

func (i *Installer) missingPackages() ([]string, error) {
	have, err := i.installedPackages()
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, p := range podmanPackages {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	return missing, nil
}
