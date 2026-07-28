package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFormatFromExtension(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/tmp/scan.json", "json"},
		{"/tmp/scan.csv", "csv"},
		{"/tmp/scan.tsv", "tsv"},
		{"/tmp/scan.yaml", "yaml"},
		{"/tmp/scan.yml", "yaml"},
		{"/tmp/scan.YAML", "yaml"},
		{"/tmp/scan", ""},
		{"/tmp/scan.txt", ""},
		{"scan.JSON", "json"},
	}

	for _, tc := range cases {
		got := formatFromExtension(tc.path)
		if got != tc.want {
			t.Fatalf("formatFromExtension(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestResolveFormat(t *testing.T) {
	cases := []struct {
		out, defaultFormat, want string
		wantErr                  bool
	}{
		{"/tmp/scan.json", "json", "json", false},
		{"/tmp/scan.csv", "json", "csv", false},
		{"/tmp/scan.tsv", "json", "tsv", false},
		{"/tmp/scan.yaml", "json", "yaml", false},
		{"/tmp/scan.yml", "json", "yaml", false},
		{"/tmp/scan", "csv", "csv", false},
		{"/tmp/scan.txt", "json", "", true},
		{"/tmp/scan.JSON", "json", "json", false},
	}

	for _, tc := range cases {
		got, err := resolveFormat(tc.out, tc.defaultFormat)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("resolveFormat(%q, %q) expected error, got nil", tc.out, tc.defaultFormat)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveFormat(%q, %q) unexpected error: %v", tc.out, tc.defaultFormat, err)
		}
		if got != tc.want {
			t.Fatalf("resolveFormat(%q, %q) = %q, want %q", tc.out, tc.defaultFormat, got, tc.want)
		}
	}
}

func TestCLIFormatAutoDetect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	tmp := t.TempDir()
	csvOut := filepath.Join(tmp, "scan.csv")
	yamlOut := filepath.Join(tmp, "scan.yaml")
	jsonOut := filepath.Join(tmp, "scan.csv")

	// Auto-detect CSV from .csv extension.
	cmd := exec.Command("go", "run", ".", "-out", csvOut, "-paths", "/tmp")
	cmd.Dir = toolDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CSV auto-detect failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(csvOut)
	if err != nil {
		t.Fatalf("reading CSV output: %v", err)
	}
	if !strings.HasPrefix(string(data), "Path,") {
		t.Fatalf("expected CSV header, got: %s", string(data)[:min(len(data), 50)])
	}

	// Auto-detect YAML from .yaml extension.
	cmd = exec.Command("go", "run", ".", "-out", yamlOut, "-paths", "/tmp")
	cmd.Dir = toolDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("YAML auto-detect failed: %v\n%s", err, out)
	}
	data, err = os.ReadFile(yamlOut)
	if err != nil {
		t.Fatalf("reading YAML output: %v", err)
	}
	if !strings.HasPrefix(string(data), "- path:") {
		t.Fatalf("expected YAML list, got: %s", string(data)[:min(len(data), 50)])
	}

	// Explicit -format overrides extension.
	cmd = exec.Command("go", "run", ".", "-out", jsonOut, "-format", "json", "-paths", "/tmp")
	cmd.Dir = toolDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("explicit format override failed: %v\n%s", err, out)
	}
	data, err = os.ReadFile(jsonOut)
	if err != nil {
		t.Fatalf("reading JSON override output: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		t.Fatalf("expected JSON array, got: %s", string(data)[:min(len(data), 50)])
	}

	// Unknown extension is rejected unless an explicit format is provided.
	txtOut := filepath.Join(tmp, "scan.txt")
	cmd = exec.Command("go", "run", ".", "-out", txtOut, "-paths", "/tmp")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for unsupported extension, got none")
	}
	if !strings.Contains(string(out), "unsupported output extension") {
		t.Fatalf("expected unsupported extension error, got: %s", out)
	}
}

func TestCLIRemovedJSONOutRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	cmd := exec.Command("go", "run", ".", "-json-out", "/tmp/scan.json", "-paths", "/tmp")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected -json-out to be rejected, got output: %s", out)
	}
	if !strings.Contains(string(out), "flag provided but not defined: -json-out") {
		t.Fatalf("expected 'flag provided but not defined: -json-out', got: %s", out)
	}
}

func TestCLIStdoutFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	// -stdout with default format writes JSON to stdout.
	cmd := exec.Command("go", "run", ".", "-stdout", "-paths", "/tmp")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-stdout default format failed: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "[") {
		t.Fatalf("expected JSON array on stdout, got: %s", string(out)[:min(len(out), 50)])
	}

	// -stdout -format csv writes CSV to stdout.
	cmd = exec.Command("go", "run", ".", "-stdout", "-format", "csv", "-paths", "/tmp")
	cmd.Dir = toolDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-stdout csv failed: %v\n%s", err, out)
	}
	if !strings.HasPrefix(string(out), "Path,") {
		t.Fatalf("expected CSV header on stdout, got: %s", string(out)[:min(len(out), 50)])
	}

	// -stdout -format yaml writes YAML to stdout.
	cmd = exec.Command("go", "run", ".", "-stdout", "-format", "yaml", "-paths", "/tmp")
	cmd.Dir = toolDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-stdout yaml failed: %v\n%s", err, out)
	}
	if !strings.HasPrefix(string(out), "- path:") {
		t.Fatalf("expected YAML list on stdout, got: %s", string(out)[:min(len(out), 50)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// findModuleRoot returns the absolute path of the Go module root by walking
// up from the current working directory looking for go.mod.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod")
		}
		dir = parent
	}
}

func TestParseCSVColumns(t *testing.T) {
	cases := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		{"", nil, false},
		{"Name,Size", []string{"Name", "Size"}, false},
		{"name, size, MODTIME", []string{"Name", "Size", "ModTime"}, false},
		{"Invalid", nil, true},
		{"Name,", []string{"Name"}, false},
	}

	for _, tc := range cases {
		got, err := parseCSVColumns(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseCSVColumns(%q) expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseCSVColumns(%q) unexpected error: %v", tc.input, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("parseCSVColumns(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestResolveTUIStyle(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", "dua", false},
		{"dua", "dua", false},
		{"terminal", "terminal", false},
		{"tree", "terminal", false},
		{"DUA", "dua", false},
		{"Terminal", "terminal", false},
		{"invalid", "", true},
	}

	for _, tc := range cases {
		got, err := resolveTUIStyle(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("resolveTUIStyle(%q) expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveTUIStyle(%q) unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("resolveTUIStyle(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	cmd := exec.Command("go", "run", ".", "-version")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-version command failed: %v\n%s", err, out)
	}
	want := "v0.4.3"
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected -version to report %q, got:\n%s", want, out)
	}
}

func TestTUIStyleSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("finding module root: %v", err)
	}
	toolDir := filepath.Join(root, "cmd", "cleanup-tool")

	// The help output should show dua as the default value.
	cmd := exec.Command("go", "run", ".", "-help")
	cmd.Dir = toolDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-help failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `-tui-style`) || !strings.Contains(string(out), `default "dua"`) {
		t.Fatalf("expected -tui-style help to show 'dua' as default, got:\n%s", out)
	}

	// An invalid style is rejected with a helpful error.
	cmd = exec.Command("go", "run", ".", "-tui-style", "invalid_style")
	cmd.Dir = toolDir
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for invalid tui-style, got none")
	}
	if !strings.Contains(string(out), "invalid -tui-style") || !strings.Contains(string(out), "valid: terminal, dua") {
		t.Fatalf("expected invalid tui-style error, got:\n%s", out)
	}
}
