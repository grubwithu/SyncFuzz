//go:build linux

package environment

import (
	"os"
	"syscall"
)

// readProcFDIdentity resolves the kernel object currently referenced by a
// procfs FD entry. Stat follows /proc/<pid>/fd/<n>, so it also works for an
// unlinked file that remains open as "(deleted)".
func readProcFDIdentity(fdPath string) (uint64, uint64) {
	info, err := os.Stat(fdPath)
	if err != nil {
		return 0, 0
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Dev), uint64(stat.Ino)
}
