package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/patriciomg/cleanup-tool/internal/deps"
)

func TestDefaultIncludesDepsTargets(t *testing.T) {
	cfg := Default()
	if len(cfg.DepsTargets) == 0 {
		t.Fatal("expected default DepsTargets to be non-empty")
	}
	want := deps.DefaultTargets()
	if len(cfg.DepsTargets) != len(want) {
		t.Fatalf("expected %d default targets, got %d", len(want), len(cfg.DepsTargets))
	}
	for i, got := range cfg.DepsTargets {
		if got != want[i] {
			t.Fatalf("expected target %q at index %d, got %q", want[i], i, got)
		}
	}
}

func TestLoadMigrationsEmptyDepsTargets(t *testing.T) {
	// Use a temporary config dir so we don't touch the user's config.
	tmp := t.TempDir()
	oldConfigHome := xdg.ConfigHome
	t.Cleanup(func() { xdg.ConfigHome = oldConfigHome })
	xdg.ConfigHome = tmp

	cfgPath := filepath.Join(tmp, "cleanup-tool", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte(`{"version": 1, "ignore_hidden": false}`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if len(cfg.DepsTargets) == 0 {
		t.Fatal("expected migrated DepsTargets to be non-empty")
	}
	want := deps.DefaultTargets()
	if len(cfg.DepsTargets) != len(want) {
		t.Fatalf("expected %d default targets after migration, got %d", len(want), len(cfg.DepsTargets))
	}
}

func TestLoadPreservesCustomDepsTargets(t *testing.T) {
	tmp := t.TempDir()
	oldConfigHome := xdg.ConfigHome
	t.Cleanup(func() { xdg.ConfigHome = oldConfigHome })
	xdg.ConfigHome = tmp

	cfgPath := filepath.Join(tmp, "cleanup-tool", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte(`{"deps_targets": ["custom_dir"]}`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if len(cfg.DepsTargets) != 1 || cfg.DepsTargets[0] != "custom_dir" {
		t.Fatalf("expected custom target [custom_dir], got %v", cfg.DepsTargets)
	}
}
