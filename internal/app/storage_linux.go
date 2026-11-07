//go:build linux

package app

import "syscall"

// diskSpace returns the free (available to unprivileged users) and total bytes
// of the file system holding path.
func diskSpace(path string) (free, total uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), uint64(st.Blocks) * uint64(st.Bsize), true
}
