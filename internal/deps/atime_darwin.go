//go:build darwin

package deps

import (
	"io/fs"
	"syscall"
	"time"
)

// fileTimes returns the access and modification times for a file.
// On Darwin, the access time is read from the underlying syscall.Stat_t.
func fileTimes(info fs.FileInfo) (access, mod time.Time) {
	mod = info.ModTime()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		access = time.Unix(stat.Atimespec.Sec, int64(stat.Atimespec.Nsec))
	} else {
		access = mod
	}
	return access, mod
}
