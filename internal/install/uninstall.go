package install

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Uninstall removes everything Install set up, in the order proven by hand.
func (i *Installer) Uninstall() error {
	if err := i.validate(); err != nil {
		return err
	}
	st, err := i.loadState()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no install record at %s: nothing to uninstall", i.StatePath)
		}
		return err
	}
	var purge []string
	for _, p := range st.Packages {
		if !packageName.MatchString(p) {
			return fmt.Errorf("%s lists an invalid package name %q", i.StatePath, p)
		}
		purge = append(purge, p)
	}
	i.say("ReelAnvil uninstall plan")
	i.say("  - stop and disable %s and %s, then wait until both are inactive", serverUnit, workerUnit)
	if st.UserCreated {
		i.say("  - reset %s's Podman storage (containers and images), remove linger, the units and the user", User)
	} else {
		i.say("  - reset %s's Podman storage (containers and images), remove linger and the units; keep the user (it existed before ReelAnvil)", User)
	}
	if len(purge) > 0 {
		i.say("  - apt purge %d packages that ReelAnvil installed (Podman and its dependencies), after checking apt removes nothing else", len(purge))
	}
	i.say("  - %s", yesNo(i.DeleteData, "delete "+st.Root+" (settings, database, media, downloaded models)", "keep "+st.Root+" unless you answer yes below or pass --delete-data"))
	if !i.confirm("Proceed?", false) {
		i.say("Nothing changed.")
		return nil
	}
	// --yes never deletes data: only --delete-data or an interactive yes does.
	removeData := i.DeleteData
	if !removeData && !i.Yes {
		removeData = i.confirm(fmt.Sprintf("Also delete %s (settings, database, media, downloaded models)?", st.Root), false)
	}
	if !removeData {
		i.say("Keeping %s (add --delete-data to delete it).", st.Root)
	}
	if len(purge) > 0 && !i.Yes {
		if !i.confirm("Remove Podman and the packages ReelAnvil installed?", false) {
			purge = nil
		}
	}

	var warns []string
	warn := func(step string, err error) {
		if err != nil {
			warns = append(warns, step+": "+err.Error())
			i.say("  warning: %s: %v", step, err)
		}
	}

	if st.Service {
		// A rerun after a partial uninstall has no unit files left to disable.
		var present []string
		for _, u := range []string{serverUnit, workerUnit} {
			if _, serr := os.Stat(filepath.Join(i.UnitDir, u)); serr == nil {
				present = append(present, u)
			}
		}
		if len(present) > 0 {
			i.say("Stopping services")
			_, err = i.systemctl(append([]string{"disable", "--now"}, present...)...)
			warn("disable units", err)
		}
		// `stop` returns before the worker has finished `compose down`.
		err = i.poll("both units to be inactive", 5*time.Minute, func() (bool, error) {
			for _, u := range []string{serverUnit, workerUnit} {
				switch i.unitState(u) {
				case "active", "activating", "deactivating", "reloading":
					return false, nil
				}
			}
			return true, nil
		})
		warn("wait for units", err)
	}

	if _, _, lerr := i.Lookup(User); lerr == nil {
		if _, serr := os.Stat(i.RuntimeDir(st.UID)); serr == nil {
			i.say("Resetting Podman storage")
			_, err = i.Run.Run(i.asUser(st, "/", "podman", "system", "reset", "-f"))
			warn("podman system reset", err)
		}
		_, err = i.Run.Run(Cmd{Name: "loginctl", Args: []string{"disable-linger", User}})
		warn("disable-linger", err)
		// Ends user@UID before userdel; fails harmlessly when it already ended.
		i.Run.Run(Cmd{Name: "loginctl", Args: []string{"terminate-user", User}})
	}
	for _, u := range []string{serverUnit, workerUnit} {
		if err := os.Remove(filepath.Join(i.UnitDir, u)); err != nil && !os.IsNotExist(err) {
			warn("remove "+u, err)
		}
	}
	_, err = i.systemctl("daemon-reload")
	warn("daemon-reload", err)

	if _, _, lerr := i.Lookup(User); lerr == nil && st.UserCreated {
		i.say("Removing user %s", User)
		// user@UID.service may still be shutting down.
		err = i.poll("userdel to succeed", 30*time.Second, func() (bool, error) {
			_, err := i.Run.Run(Cmd{Name: "userdel", Args: []string{User}})
			return err == nil, nil
		})
		warn("userdel", err)
	}
	switch {
	case !removeData:
	case hasMarker(st.Root):
		i.say("Deleting %s", st.Root)
		warn("delete install directory", os.RemoveAll(st.Root))
	default:
		i.say("Not deleting %s: it has no %s file, so ReelAnvil did not create it. Delete it by hand if you are sure.", st.Root, markerFile)
	}
	if len(purge) > 0 {
		if i.purge(purge, warn) && (contains(purge, "podman") || contains(purge, "containers-common")) {
			// purge leaves these behind
			for _, d := range i.ContainerDirs {
				warn("remove "+d, os.RemoveAll(d))
			}
		}
	}
	if len(warns) > 0 {
		i.say("Finished with %d warning(s); the install record %s was kept so you can run uninstall again.", len(warns), i.StatePath)
		return fmt.Errorf("uninstall incomplete: %s", strings.Join(warns, "; "))
	}
	os.Remove(i.StatePath)
	os.Remove(filepath.Dir(i.StatePath))
	i.say("Uninstalled. Reboot to unload the network modules Podman used.")
	return nil
}

func contains(list []string, want string) bool {
	for _, p := range list {
		if p == want {
			return true
		}
	}
	return false
}

// purge removes the recorded packages, but only after `apt-get -s` shows it
// would remove nothing else (purge also takes packages that depend on them).
// It reports whether the packages were purged.
func (i *Installer) purge(pkgs []string, warn func(string, error)) bool {
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	out, err := i.Run.Run(Cmd{Name: "apt-get", Args: append([]string{"-s", "purge", "-y"}, pkgs...), Env: env})
	if err != nil {
		warn("apt purge simulation", err)
		return false
	}
	var extra []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && (f[0] == "Purg" || f[0] == "Remv") && !contains(pkgs, f[1]) {
			extra = append(extra, f[1])
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		i.say("  warning: not purging: apt would also remove %s, which ReelAnvil did not install. Review them, then run `apt-get purge %s` yourself if you want Podman gone.",
			strings.Join(extra, ", "), strings.Join(pkgs, " "))
		return false
	}
	i.say("Purging %d packages", len(pkgs))
	_, err = i.Run.Run(Cmd{Name: "apt-get", Args: append([]string{"purge", "-y"}, pkgs...), Env: env, Stream: true})
	warn("apt purge", err)
	return err == nil
}
