//go:build unix

package app

import (
	"os"
	"syscall"
)

// Reexec replaces the process with a fresh copy of the binary and the same
// arguments, so a restart also picks up a rebuilt executable.
func Reexec() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
