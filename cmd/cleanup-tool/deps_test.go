package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIDepsUsesConfigTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	// Set up a temporary XDG config dir with custom deps_targets.
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "cleanup-tool"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgData := []byte(`{"deps_targets": ["custom_dir"]}`)
	if err := os.WriteFile(filepath.Join(configDir, "cleanup-tool", "config.json"), cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Create a scan directory with both a default target and a custom target.
	scanDir := filepath.Join(tmp, "scan")
	if err := os.MkdirAll(filepath.Join(scanDir, "custom_dir"), 0o755); err != nil {
		t.Fatalf("mkdir custom_dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(scanDir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}

	cmd := exec.Command("go", "run", ".", "deps", "-json", "-paths", scanDir)
	cmd.Dir = toolDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deps command failed: %v\n%s", err, out)
	}

	var results []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("unmarshal deps output: %v\n%s", err, out)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %s", len(results), out)
	}
	if results[0].Type != "custom_dir" {
		t.Fatalf("expected custom_dir, got %s", results[0].Type)
	}
}

func TestCLIDepsTargetsOverrideConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	// Set up a temporary XDG config dir with custom deps_targets.
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "cleanup-tool"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgData := []byte(`{"deps_targets": ["custom_dir"]}`)
	if err := os.WriteFile(filepath.Join(configDir, "cleanup-tool", "config.json"), cfgData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	scanDir := filepath.Join(tmp, "scan")
	if err := os.MkdirAll(filepath.Join(scanDir, "custom_dir"), 0o755); err != nil {
		t.Fatalf("mkdir custom_dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(scanDir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}

	cmd := exec.Command("go", "run", ".", "deps", "-json", "-paths", scanDir, "-targets", "node_modules")
	cmd.Dir = toolDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deps command failed: %v\n%s", err, out)
	}

	var results []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("unmarshal deps output: %v\n%s", err, out)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %s", len(results), out)
	}
	if results[0].Type != "node_modules" {
		t.Fatalf("expected node_modules, got %s", results[0].Type)
	}
}

func TestCLIDepsHelpMentionsConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	cmd := exec.Command("go", "run", ".", "deps", "-help")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deps -help failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "-targets") {
		t.Fatalf("expected help to mention -targets flag, got:\n%s", out)
	}
}
