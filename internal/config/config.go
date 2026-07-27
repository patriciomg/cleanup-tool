package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const appName = "cleanup-tool"

type Config struct {
	Version          int      `json:"version"`
	IgnorePaths      []string `json:"ignore_paths"`
	IgnoreHidden     bool     `json:"ignore_hidden"`
	TrashOnly        bool     `json:"trash_only"`
	DupMode          string   `json:"dup_mode"`
	ProgressInterval int      `json:"progress_interval"`
}

func Default() *Config {
	return &Config{
		Version:          1,
		IgnorePaths:      DefaultIgnorePaths(),
		IgnoreHidden:     false,
		TrashOnly:        true,
		DupMode:          "smart",
		ProgressInterval: 100,
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
	return &cfg, nil
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
