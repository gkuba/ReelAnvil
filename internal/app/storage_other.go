//go:build !linux

package app

// diskSpace is only implemented for Linux, the supported platform.
func diskSpace(string) (free, total uint64, ok bool) { return 0, 0, false }
