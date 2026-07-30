package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"

	"github.com/patriciomg/cleanup-tool/internal/defaults"
)

const appName = "cleanup-tool"

type Config struct {
	Version          int      `json:"version"`
	IgnorePaths      []string `json:"ignore_paths"`
	IgnoreHidden     bool     `json:"ignore_hidden"`
	IncludeVCS       bool     `json:"include_vcs"`
	TrashOnly        bool     `json:"trash_only"`
	DupMode          string   `json:"dup_mode"`
	ProgressInterval int      `json:"progress_interval"`

	// TUI preferences
	LastView       string `json:"last_view,omitempty"`
	AnalyzerFilter string `json:"analyzer_filter,omitempty"`
	SortOrder      string `json:"sort_order,omitempty"`
	// NotificationsEnabled controls whether macOS notifications are emitted.
	NotificationsEnabled bool `json:"notifications_enabled"`

	// DepsTargets is the list of dependency directory names used by the deps
	// subcommand when no -targets flag is provided.
	DepsTargets []string `json:"deps_targets,omitempty"`
}

func Default() *Config {
	return &Config{
		Version:              2,
		IgnorePaths:          DefaultIgnorePaths(),
		IgnoreHidden:         false,
		IncludeVCS:           false,
		TrashOnly:            true,
		DupMode:              "smart",
		ProgressInterval:     100,
		SortOrder:            "size",
		NotificationsEnabled: true,
		DepsTargets:          defaults.DepsTargets(),
	}
}

func DefaultIgnorePaths() []string {
	return []string{
		"/System",
		"/Volumes",
		"/dev",
		"/proc",
		"/net",
		"/private/var/db/timezone",
	}
}

func Path() string {
	return filepath.Join(xdg.ConfigHome, appName, "config.json")
}

func UndoPath() string {
	return filepath.Join(xdg.ConfigHome, appName, "undo.json")
}

func Load() (*Config, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.migrate()
	return &cfg, nil
}

// migrate applies forward-compatible updates for older config versions.
func (c *Config) migrate() {
	if c.SortOrder == "" {
		c.SortOrder = "size"
	}
	if len(c.DepsTargets) == 0 {
		c.DepsTargets = defaults.DepsTargets()
	}
	if c.Version < 2 {
		// Notifications were previously always enabled; keep that behavior.
		c.NotificationsEnabled = true
		c.Version = 2
	}
}

func (c *Config) Save() error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
