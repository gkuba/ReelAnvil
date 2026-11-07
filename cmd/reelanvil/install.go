package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/gkuba/ReelAnvil/internal/app"
	"github.com/gkuba/ReelAnvil/internal/install"
)

// installFlags binds the flags shared by install, upgrade and uninstall.
func installFlags(command string, in *install.Installer) *flag.FlagSet {
	flags := flag.NewFlagSet("reelanvil "+command, flag.ContinueOnError)
	flags.BoolVar(&in.Yes, "yes", false, "answer yes to every question (never deletes data: see uninstall --delete-data)")
	flags.StringVar(&in.Root, "dir", install.DefaultRoot, "install directory")
	flags.IntVar(&in.Port, "port", install.DefaultPort, "web app port")
	switch command {
	case "install":
		flags.BoolVar(&in.Service, "service", false, "also install and start the systemd units")
	case "uninstall":
		flags.BoolVar(&in.DeleteData, "delete-data", false, "also delete the install directory (settings, database, media, models); --yes does not")
	case "upgrade":
		flags.BoolVar(&in.DryRun, "dry-run", false, "show the plan and change nothing")
	}
	return flags
}

// runInstall runs install, upgrade or uninstall and returns the exit code.
func runInstall(command string, args []string) int {
	in := install.New(os.Stdout, os.Stdin, app.Version)
	flags := installFlags(command, in)
	if code, done := parse(flags, args); done {
		return code
	}
	var err error
	switch command {
	case "install":
		err = in.Install()
	case "upgrade":
		err = in.Upgrade()
	case "uninstall":
		err = in.Uninstall()
	}
	return report(err)
}

// runDoctor prints the local diagnostics, then the system-install report.
func runDoctor(args []string) int {
	var opts app.Options
	flags := newFlags("doctor", &opts)
	in := install.New(os.Stdout, os.Stdin, app.Version)
	var repair, clean bool
	flags.BoolVar(&repair, "repair", false, "fix the system install: folders, units, linger, services (never settings, data or media)")
	flags.BoolVar(&clean, "clean", false, "with --repair: also reset the service user's Podman storage so images are pulled fresh")
	if code, done := parse(flags, args); done {
		return code
	}
	if clean && !repair {
		fmt.Fprintln(os.Stderr, "--clean needs --repair")
		return 2
	}
	if !repair {
		if code := report(app.Doctor(opts)); code != 0 {
			return code
		}
	}
	return report(in.Doctor(repair, clean))
}

func parse(flags *flag.FlagSet, args []string) (code int, done bool) {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, true
		}
		return 2, true
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments:", flags.Args())
		return 2, true
	}
	return 0, false
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "reelanvil:", err)
		return 1
	}
	return 0
}
