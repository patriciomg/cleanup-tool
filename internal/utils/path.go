package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading "~" in a path to the user's home directory.
func ExpandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[1:])
	}
	return p
}

// ExpandHomeSlice expands a leading "~" in each path.
func ExpandHomeSlice(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = ExpandHome(p)
	}
	return out
}
