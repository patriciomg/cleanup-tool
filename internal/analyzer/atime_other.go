//go:build !darwin

package analyzer

import (
	"io/fs"
	"time"
)

// accessTime returns the access time for a file. On platforms where reliable
// access time is unavailable, modification time is used as a proxy.
func accessTime(info fs.FileInfo) time.Time {
	return info.ModTime()
}
