package recent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const (
	appName    = "cleanup-tool"
	recentFile = "recent.json"
	maxEntries = 5
)

// Paths returns the most recently saved scan paths. If the file does not
// exist yet, an empty slice is returned.
func Paths() ([]string, error) {
	entries, err := load()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return entries[0], nil
}

// Save stores the given paths as the most recent entry, keeping only the
// last maxEntries distinct entries.
func Save(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	entries, _ := load()
	var updated [][]string
	updated = append(updated, clean(paths))
	for _, e := range entries {
		if !same(e, paths) && len(updated) < maxEntries {
			updated = append(updated, e)
		}
	}
	return write(updated)
}

func load() ([][]string, error) {
	data, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries [][]string
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func write(entries [][]string) error {
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}

// path is a function variable so tests can override the storage location.
var path = func() string {
	return filepath.Join(xdg.ConfigHome, appName, recentFile)
}

func clean(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
