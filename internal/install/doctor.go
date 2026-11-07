package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type check struct {
	name   string
	ok     bool
	detail string
	repair bool // doctor --repair can fix it
}

// Doctor reports on the system install and, with repair, fixes what it can
// (folders, ownership, Compose file if missing, units, linger, services). It
// never touches settings, data, media or the installed version. With clean it
// first resets the service user's Podman storage so the images are pulled fresh.
func (i *Installer) Doctor(repair, clean bool) error {
	st, err := i.loadState()
	if os.IsNotExist(err) {
		if repair {
			return fmt.Errorf("no install record at %s: run `reelanvil install` first", i.StatePath)
		}
		i.say("\nSystem install: not found (%s). Standalone use needs no installer.", i.StatePath)
		return nil
	}
	if err != nil {
		return err
	}
	if repair && !i.IsRoot {
		return fmt.Errorf("doctor --repair needs sudo")
	}
	i.say("\nSystem install (%s, port %d, %s)", st.Root, st.Port, yesNo(st.Service, "service mode", "no units"))
	recordedUID := st.UID
	bad := i.printChecks(&st, false)
	i.mediaReport(st)
	if !repair {
		if bad > 0 {
			i.say("\n%d problem(s). `sudo reelanvil doctor --repair` fixes what it can.", bad)
			return fmt.Errorf("%d check(s) failed", bad)
		}
		return nil
	}
	if bad == 0 && !clean {
		i.say("\nNothing to repair.")
		return nil
	}
	warns, err := i.repair(st, recordedUID, clean)
	if err != nil {
		return err
	}
	st, err = i.loadState()
	if err != nil {
		return err
	}
	i.say("\nAfter repair")
	left := i.printChecks(&st, true)
	var problems []string
	if len(warns) > 0 {
		problems = append(problems, strings.Join(warns, "; "))
	}
	if left > 0 {
		problems = append(problems, fmt.Sprintf("%d check(s) still failing", left))
	}
	if len(problems) > 0 {
		return fmt.Errorf("repair incomplete: %s", strings.Join(problems, "; "))
	}
	i.say("  all checks pass")
	return nil
}

// printChecks runs the checks, prints them (only the failures with failedOnly)
// and returns how many failed.
func (i *Installer) printChecks(st *State, failedOnly bool) int {
	bad := 0
	for _, c := range i.checks(st) {
		mark := "ok  "
		if !c.ok {
			mark = "FAIL"
			bad++
		} else if failedOnly {
			continue
		}
		i.say("  [%s] %s: %s", mark, c.name, c.detail)
	}
	return bad
}

func (i *Installer) checks(st *State) []check {
	var out []check
	add := func(name string, ok bool, detail string, repair bool) {
		out = append(out, check{name, ok, detail, repair})
	}
	gid, uerr := i.lookupUser(st)
	_ = gid
	add("user "+User, uerr == nil, yesNo(uerr == nil, "uid "+strconv.Itoa(st.UID), "missing"), true)
	for _, d := range []string{"", "server", "containers"} {
		p := filepath.Join(st.Root, d)
		_, err := os.Stat(p)
		add("folder "+p, err == nil, yesNo(err == nil, "present", "missing"), true)
	}
	for _, f := range []string{"server/reelanvil", "containers/compose.yaml"} {
		p := filepath.Join(st.Root, f)
		_, err := os.Stat(p)
		add(f, err == nil, yesNo(err == nil, "present", "missing"), true)
	}
	want, cerr := i.renderCompose(*st)
	switch c := fileHash(filepath.Join(st.Root, "containers", "compose.yaml")); {
	case cerr != nil:
		add("compose.yaml settings", false, cerr.Error(), false)
	case c != "" && c != hashBytes(want):
		add("compose.yaml", true, "differs from what the settings produce (edited, older, or Whisper/media settings changed); `sudo reelanvil upgrade` replaces it", false)
	}
	i.mediaChecks(st, uerr == nil, add)
	add("install marker", hasMarker(st.Root), yesNo(hasMarker(st.Root), "present", "missing ("+markerFile+")"), true)
	_, err := os.Stat(i.RuntimeDir(st.UID))
	add("linger / runtime dir", err == nil, yesNo(err == nil, i.RuntimeDir(st.UID), i.RuntimeDir(st.UID)+" missing"), true)
	if st.Service {
		for _, name := range []string{serverUnit, workerUnit} {
			want, _ := i.renderUnit(*st, name)
			got := fileHash(filepath.Join(i.UnitDir, name))
			add("unit "+name, got == hashBytes(want), yesNo(got == hashBytes(want), "as expected", "missing or different (wrong UID?)"), true)
		}
		if i.IsRoot {
			for _, name := range []string{serverUnit, workerUnit} {
				s := i.unitState(name)
				add(name+" active", s == "active", s, true)
			}
		}
	}
	if i.IsRoot && uerr == nil {
		out2, err := i.Run.Run(i.asUser(*st, "/", "podman", "ps", "--format", "{{.Names}}"))
		n := len(strings.Fields(out2))
		add("containers running", err == nil && n >= 3, fmt.Sprintf("%d of 3", n), true)
	}
	if ollama, whisper, cerr := i.serviceAddresses(*st); cerr != nil {
		add("service addresses", false, cerr.Error(), false)
	} else {
		for _, svc := range []struct{ name, addr string }{{"Ollama", ollama}, {"Whisper", whisper}} {
			detail := "accepts connections"
			err := i.canConnect(svc.addr)
			if err != nil {
				detail = "not reachable: " + err.Error()
			}
			add(svc.name+" "+svc.addr, err == nil, detail, false)
		}
	}
	if st.Service {
		code, _, herr := i.Get(fmt.Sprintf("http://127.0.0.1:%d/", st.Port))
		add("web app", herr == nil && code == 200, yesNo(herr == nil && code == 200, "HTTP 200", "not answering on port "+strconv.Itoa(st.Port)), true)
	}
	return out
}

// mediaChecks reports on the media root from the settings file: it exists, is
// a folder, and the service user can write to it. The write check runs as that
// user, so it needs root.
func (i *Installer) mediaChecks(st *State, userOK bool, add func(name string, ok bool, detail string, repair bool)) {
	root, err := i.mediaRoot(*st)
	if err != nil {
		add("media_root", false, err.Error(), false)
		return
	}
	name := "folder " + root + " (media root)"
	fi, err := os.Stat(root)
	switch {
	case err != nil:
		add(name, false, "missing", true)
		return
	case !fi.IsDir():
		add(name, false, "not a folder", false)
		return
	}
	if !i.IsRoot || !userOK {
		add(name, true, "present (run with sudo to test that "+User+" can write to it)", false)
		return
	}
	if _, err := i.Run.Run(i.asUser(*st, "/", "test", "-d", root, "-a", "-w", root, "-a", "-x", root)); err != nil {
		add(name, false, User+" cannot write to it; fix with: sudo chown "+User+":"+User+" "+root, false)
		return
	}
	add(name, true, "present and writable by "+User, false)
}

// repair fixes what it can. Problems that do not stop it (a unit that would
// not stop) come back as warnings; recordedUID is the UID the install record
// had before the user was looked up or recreated.
func (i *Installer) repair(st State, recordedUID int, clean bool) (warns []string, err error) {
	warn := func(step string, err error) {
		warns = append(warns, step+": "+err.Error())
		i.say("  warning: %s: %v", step, err)
	}
	gid, err := i.ensureUser(&st)
	if err != nil {
		return nil, err
	}
	if recordedUID != 0 && st.UID != recordedUID {
		i.say("User %s now has uid %d (the record had %d): giving it ownership of %s", User, st.UID, recordedUID, st.Root)
		if err := i.chownTree(st, gid, st.Root); err != nil {
			return nil, err
		}
		i.say("  warning: the user's subuid/subgid ranges may differ from before, so existing Podman storage can be unreadable. If containers fail to start, run `sudo reelanvil doctor --repair --clean`.")
	}
	if err := i.saveState(st); err != nil {
		return nil, err
	}
	if st.Service {
		// Only units that have a file: stopping a missing one fails.
		var present []string
		for _, u := range []string{serverUnit, workerUnit} {
			if _, serr := os.Lstat(filepath.Join(i.UnitDir, u)); serr == nil {
				present = append(present, u)
			}
		}
		if len(present) > 0 {
			if _, err := i.systemctl(append([]string{"stop"}, present...)...); err != nil {
				warn("stop units", err)
			}
		}
		err := i.poll("units to stop", 5*time.Minute, func() (bool, error) {
			for _, u := range []string{serverUnit, workerUnit} {
				if s := i.unitState(u); s == "active" || s == "activating" || s == "deactivating" {
					return false, nil
				}
			}
			return true, nil
		})
		if err != nil {
			warn("wait for units to stop", err)
		}
	}
	if err := i.ensureLayout(st, gid); err != nil {
		return warns, err
	}
	if err := i.ensureLinger(st); err != nil {
		return warns, err
	}
	if clean {
		i.say("Resetting %s's Podman storage (settings, data and media are kept)", User)
		if _, err := i.Run.Run(i.asUser(st, "/", "podman", "system", "reset", "-f")); err != nil {
			return warns, err
		}
	}
	if _, err := i.ensureBinary(st, gid, false); err != nil {
		return warns, err
	}
	if _, err := i.ensureCompose(st, gid, false); err != nil {
		return warns, err
	}
	if st.Service {
		if _, err := i.ensureUnits(st); err != nil {
			return warns, err
		}
		if _, err := i.systemctl("daemon-reload"); err != nil {
			return warns, err
		}
		if _, err := i.systemctl("enable", "--now", serverUnit); err != nil {
			return warns, err
		}
		if _, err := i.systemctl("start", workerUnit); err != nil {
			return warns, err
		}
		if err := i.waitStarted(st); err != nil {
			return warns, err
		}
	}
	i.say("Repair finished.")
	return warns, nil
}

// chownTree gives root and everything under it to the service user. It never
// follows symbolic links.
func (i *Installer) chownTree(st State, gid int, root string) error {
	return filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return i.chown(st, gid, path)
	})
}

// mediaReport prints the media container's yt-dlp against the latest release
// and the last download error. It never changes yt-dlp: the image owns it.
func (i *Installer) mediaReport(st State) {
	i.say("  Media (yt-dlp)")
	if !i.IsRoot {
		i.say("    run with sudo to inspect the media container")
		return
	}
	names, err := i.Run.Run(i.asUser(st, "/", "podman", "ps", "--filter", "label=com.docker.compose.service=media", "--format", "{{.Names}}"))
	name := strings.TrimSpace(strings.SplitN(names, "\n", 2)[0])
	if err != nil || name == "" {
		i.say("    media container is not running")
		return
	}
	out, err := i.Run.Run(i.asUser(st, "/", "podman", "exec", name, "yt-dlp", "--version"))
	have := strings.TrimSpace(out)
	if err != nil || have == "" {
		i.say("    could not read yt-dlp's version in %s", name)
		return
	}
	latest := i.latestYtDlp()
	lastErr := i.lastDownloadError(st)
	behind := latest != "" && versionLess(have, latest)
	switch {
	case latest == "":
		i.say("    in container: %s (latest release unknown: GitHub not reachable)", have)
	case behind:
		i.say("    in container: %s, latest release: %s (behind)", have, latest)
	default:
		i.say("    in container: %s, latest release: %s (current)", have, latest)
	}
	if lastErr == "" {
		i.say("    last download error: none recorded")
	} else {
		i.say("    last download error: %s", lastErr)
	}
	if behind {
		i.say("    The image's yt-dlp is older than the latest release. If downloads fail with an extractor error, run `sudo reelanvil upgrade` to pull the newest media image.")
	}
}

func (i *Installer) latestYtDlp() string {
	code, body, err := i.Get(i.LatestURL)
	if err != nil || code != 200 {
		return ""
	}
	var r struct {
		Tag string `json:"tag_name"`
	}
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	return strings.TrimPrefix(r.Tag, "v")
}

// lastDownloadError reads the first line of logs/download-error.txt, which the
// worker will write when a download fails.
func (i *Installer) lastDownloadError(st State) string {
	b, err := os.ReadFile(filepath.Join(st.Root, "server", "logs", "download-error.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

// versionLess compares dotted numeric versions such as 2026.10.26 and 2026.10.26.1.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for k := 0; k < len(pa) || k < len(pb); k++ {
		var x, y int
		if k < len(pa) {
			x, _ = strconv.Atoi(pa[k])
		}
		if k < len(pb) {
			y, _ = strconv.Atoi(pb[k])
		}
		if x != y {
			return x < y
		}
	}
	return false
}
