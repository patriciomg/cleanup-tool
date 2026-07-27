package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/adrg/xdg"
	"github.com/patriciomg/cleanup-tool/internal/utils"
)

const currentVersion = 1

// Rule describes a reusable cleanup preset.
type Rule struct {
	Name             string   `json:"name"`
	Paths            []string `json:"paths"`
	IgnorePaths      []string `json:"ignore_paths"`
	IgnoreHidden     bool     `json:"ignore_hidden"`
	Categories       []string `json:"categories"`
	AgeThresholdDays int      `json:"age_threshold_days"`
	DupMode          string   `json:"dup_mode"`
	Action           string   `json:"action"`
	Destination      string   `json:"destination"`
	MaxDeletedBytes  int64    `json:"max_deleted_bytes"`
	DryRun           bool     `json:"dry_run"`
}

// File is the top-level JSON document stored on disk.
type File struct {
	Version int            `json:"version"`
	Rules   map[string]Rule `json:"rules"`
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Validate checks that the rule is usable.
func (r *Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("rule name is required")
	}
	if !nameRe.MatchString(r.Name) {
		return fmt.Errorf("rule name must match %s", nameRe.String())
	}
	if len(r.Paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	for _, p := range r.Paths {
		p = utils.ExpandHome(p)
		if p == "" {
			return fmt.Errorf("path cannot be empty")
		}
		if isProtectedPath(p) {
			return fmt.Errorf("path %q is protected", p)
		}
	}
	if r.Action != "trash" && r.Action != "move_external" {
		return fmt.Errorf("action must be trash or move_external")
	}
	if r.Action == "move_external" && strings.TrimSpace(r.Destination) == "" {
		return fmt.Errorf("destination is required for move_external")
	}
	for _, c := range r.Categories {
		switch c {
		case "old", "log/cache", "duplicate":
		default:
			return fmt.Errorf("unknown category %q", c)
		}
	}
	if r.DupMode != "" {
		switch strings.ToLower(r.DupMode) {
		case "none", "first10mb", "sample", "full", "smart":
		default:
			return fmt.Errorf("invalid dup_mode %q", r.DupMode)
		}
	}
	return nil
}

// IsProtectedPath reports whether a path is one we refuse to clean automatically.
func isProtectedPath(p string) bool {
	// Resolve symlinks so a rule pointing at a symlink to / or /System can't
	// bypass protection. If the target doesn't exist yet, fall back to Abs.
	abs, err := filepath.EvalSymlinks(p)
	if err != nil {
		abs, err = filepath.Abs(p)
		if err != nil {
			return true
		}
	}
	// Never allow the filesystem root, macOS system directories, or app bundles.
	if abs == "/" {
		return true
	}
	switch abs {
	case "/System", "/Library", "/Applications":
		return true
	}
	if strings.HasPrefix(abs, "/System/") || strings.HasPrefix(abs, "/Library/") {
		return true
	}
	return strings.HasSuffix(abs, ".app")
}

// Path returns the filesystem path for the rules JSON file.
var Path = func() string {
	return filepath.Join(xdg.ConfigHome, "cleanup-tool", "rules.json")
}

// Load reads the rules file from disk. If it does not exist, an empty file is returned.
func Load() (*File, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Version: currentVersion, Rules: make(map[string]Rule)}, nil
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	if f.Rules == nil {
		f.Rules = make(map[string]Rule)
	}
	return &f, nil
}

// Save writes the rules file to disk.
func (f *File) Save() error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Get returns a rule by name.
func (f *File) Get(name string) (Rule, bool) {
	r, ok := f.Rules[name]
	return r, ok
}

// Set stores a rule. The rule name is normalized before storage.
func (f *File) Set(r Rule) {
	if f.Rules == nil {
		f.Rules = make(map[string]Rule)
	}
	f.Rules[r.Name] = r
}

// Delete removes a rule by name.
func (f *File) Delete(name string) {
	delete(f.Rules, name)
}
