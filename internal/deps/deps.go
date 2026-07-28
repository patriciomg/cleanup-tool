// Package deps finds dependency directories such as node_modules and vendor,
// reporting their recursive size and last access / modification times.
package deps

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/utils"
)

// DependencyDir describes a discovered dependency directory.
type DependencyDir struct {
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	AccessTime time.Time `json:"accessTime"`
	ModTime    time.Time `json:"modTime"`
}

// PrettySize returns a human-readable size string.
func (d *DependencyDir) PrettySize() string {
	return analyzer.PrettySize(d.Size)
}

// Finder scans filesystem roots for dependency directories.
type Finder struct {
	Targets      []string
	IgnorePaths  []string
	IgnoreHidden bool
}

// DefaultTargets returns the built-in dependency directory names.
func DefaultTargets() []string {
	return []string{"node_modules", "vendor", ".venv", "venv", "bower_components", "Pods", "Carthage"}
}

// NewFinder creates a new Finder.
func NewFinder(targets, ignorePaths []string, ignoreHidden bool) *Finder {
	var expanded []string
	for _, p := range ignorePaths {
		expanded = append(expanded, normalizeIgnorePath(utils.ExpandHome(p)))
	}
	return &Finder{
		Targets:      targets,
		IgnorePaths:  expanded,
		IgnoreHidden: ignoreHidden,
	}
}

func normalizeIgnorePath(p string) string {
	p = filepath.Clean(p)
	if !strings.HasSuffix(p, string(os.PathSeparator)) {
		p += string(os.PathSeparator)
	}
	return p
}

// Find walks the given roots and returns all discovered dependency directories.
func (f *Finder) Find(ctx context.Context, roots []string) ([]*DependencyDir, error) {
	targets := f.Targets
	if len(targets) == 0 {
		targets = DefaultTargets()
	}
	targetSet := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		targetSet[t] = struct{}{}
	}

	results := make([]*DependencyDir, 0)
	var mu sync.Mutex

	for _, root := range roots {
		absRoot, err := filepath.Abs(utils.ExpandHome(root))
		if err != nil {
			return nil, fmt.Errorf("resolve root %q: %w", root, err)
		}
		if f.shouldIgnore(absRoot) {
			continue
		}

		err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}

			name := d.Name()
			if name == "." || name == ".." {
				return nil
			}

			if f.IgnoreHidden && strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			if f.shouldIgnore(path) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			if _, ok := targetSet[name]; !ok || !d.IsDir() {
				return nil
			}

			size, access, mod, err := measureDir(ctx, path)
			if err != nil {
				return fmt.Errorf("measure %q: %w", path, err)
			}

			mu.Lock()
			results = append(results, &DependencyDir{
				Path:       path,
				Type:       name,
				Size:       size,
				AccessTime: access,
				ModTime:    mod,
			})
			mu.Unlock()

			// Do not descend into the dependency directory; it is counted above.
			return fs.SkipDir
		})
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

func (f *Finder) shouldIgnore(path string) bool {
	absPath, err := filepath.Abs(utils.ExpandHome(path))
	if err != nil {
		return false
	}
	for _, p := range f.IgnorePaths {
		if absPath == filepath.Clean(strings.TrimSuffix(p, string(os.PathSeparator))) {
			return true
		}
		if strings.HasPrefix(absPath+string(os.PathSeparator), p) {
			return true
		}
	}
	return false
}

// measureDir returns the total recursive size, latest access time, and latest
// modification time for all regular files under path.
func measureDir(ctx context.Context, path string) (int64, time.Time, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, time.Time{}, time.Time{}, err
	}

	var size int64
	lastAccess, lastMod := fileTimes(info)

	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Do not follow symlinks; their targets may be outside the directory or
		// counted elsewhere.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		acc, mod := fileTimes(info)
		if acc.After(lastAccess) {
			lastAccess = acc
		}
		if mod.After(lastMod) {
			lastMod = mod
		}

		if !d.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, lastAccess, lastMod, err
}

// SortResults sorts dependency directories in place.
// Supported values for by: "size" (default), "access", "mod", "path".
func SortResults(results []*DependencyDir, by string) {
	switch by {
	case "access":
		sort.Slice(results, func(i, j int) bool {
			if !results[i].AccessTime.Equal(results[j].AccessTime) {
				return results[i].AccessTime.After(results[j].AccessTime)
			}
			return results[i].Size > results[j].Size
		})
	case "mod":
		sort.Slice(results, func(i, j int) bool {
			if !results[i].ModTime.Equal(results[j].ModTime) {
				return results[i].ModTime.After(results[j].ModTime)
			}
			return results[i].Size > results[j].Size
		})
	case "path":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Path < results[j].Path
		})
	default:
		// Default: largest first, then newest access time.
		sort.Slice(results, func(i, j int) bool {
			if results[i].Size != results[j].Size {
				return results[i].Size > results[j].Size
			}
			return results[i].AccessTime.After(results[j].AccessTime)
		})
	}
}
