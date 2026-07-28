//go:build darwin

package analyzer

import (
	"io/fs"
	"syscall"
	"time"
)

// accessTime returns the access time for a file on Darwin.
func accessTime(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Atimespec.Sec, int64(stat.Atimespec.Nsec))
	}
	return info.ModTime()
}
