package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// Upgrade replaces the binary, Compose file and units with the bundled ones
// from the running binary, pulls the latest images and restarts the services.
func (i *Installer) Upgrade() error {
	if err := i.validate(); err != nil {
		return err
	}
	st, err := i.loadState()
	if err != nil || !st.Complete {
		return fmt.Errorf("ReelAnvil is not installed (no complete record at %s); run `reelanvil install`", i.StatePath)
	}
	gid, err := i.lookupUser(&st)
	if err != nil {
		return fmt.Errorf("user %s is missing: run `reelanvil doctor --repair`", User)
	}

	binary := filepath.Join(st.Root, "server", "reelanvil")
	newBin, oldBin := fileHash(i.Exe), fileHash(binary)
	wantCompose, err := i.renderCompose(st)
	if err != nil {
		return err
	}
	composeDiffers := fileHash(filepath.Join(st.Root, "containers", "compose.yaml")) != hashBytes(wantCompose)
	media, err := i.mediaRoot(st)
	if err != nil {
		return err
	}
	unitsDiffer := false
	if st.Service {
		for _, name := range []string{serverUnit, workerUnit} {
			want, err := i.renderUnit(st, name)
			if err != nil {
				return err
			}
			if fileHash(filepath.Join(i.UnitDir, name)) != hashBytes(want) {
				unitsDiffer = true
			}
		}
	}
	i.say("ReelAnvil upgrade plan")
	i.say("  - binary: %s (installed version %s, this binary %s)", yesNo(newBin != oldBin, "replace, keeping reelanvil.previous", "unchanged"), st.Version, i.Version)
	i.say("  - compose.yaml: %s", yesNo(composeDiffers, "replace, keeping compose.yaml.bak", "unchanged"))
	if st.Service {
		i.say("  - systemd units: %s", yesNo(unitsDiffer, "rewrite", "unchanged"))
	}
	i.say("  - compose.yaml uses the saved Whisper model and language and the media root %s", media)
	i.say("  - media folder: create %s and its sub-folders if missing (existing folders are left alone)", media)
	i.say("  - settings: add any missing service fields to server/settings/config.json (existing values are kept)")
	i.say("  - pull the latest container images (they track :latest, so this also brings the media image's newer yt-dlp)")
	if st.Service {
		i.say("  - restart %s and %s", serverUnit, workerUnit)
	}
	if i.DryRun {
		i.say("Dry run: nothing changed.")
		return nil
	}
	if !i.confirm("Proceed?", true) {
		i.say("Nothing changed.")
		return nil
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
	if err := i.ensureMediaFolders(st, gid); err != nil {
		return err
	}
	if st.Service {
		if _, err := i.ensureUnits(st); err != nil {
			return err
		}
		if _, err := i.systemctl("daemon-reload"); err != nil {
			return err
		}
	}
	i.say("Pulling images")
	if _, err := i.Run.Run(withStream(i.asUser(st, filepath.Join(st.Root, "containers"), i.ComposeBin, "pull"))); err != nil {
		return err
	}
	st.Version = i.Version
	if st.Service {
		if _, err := i.systemctl("restart", serverUnit, workerUnit); err != nil {
			return err
		}
		if err := i.waitStarted(st); err != nil {
			return err
		}
	}
	if err := i.saveState(st); err != nil {
		return err
	}
	i.say("Upgrade finished.")
	if _, err := os.Stat(binary); err != nil {
		return err
	}
	return nil
}

func withStream(c Cmd) Cmd { c.Stream = true; return c }

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
