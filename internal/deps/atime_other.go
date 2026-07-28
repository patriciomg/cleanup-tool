//go:build !darwin

package deps

import (
	"io/fs"
	"time"
)

// fileTimes returns the access and modification times for a file.
// On platforms where reliable access time is unavailable, modification time
// is used as a proxy.
func fileTimes(info fs.FileInfo) (access, mod time.Time) {
	mod = info.ModTime()
	return mod, mod
}
