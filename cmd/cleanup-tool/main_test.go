package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEffectiveOutputFile(t *testing.T) {
	cases := []struct {
		out     string
		jsonOut string
		want    string
	}{
		{"", "", ""},
		{"/tmp/out.csv", "", "/tmp/out.csv"},
		{"", "/tmp/legacy.json", "/tmp/legacy.json"},
		{"/tmp/out.csv", "/tmp/legacy.json", "/tmp/out.csv"},
	}

	for _, tc := range cases {
		got := effectiveOutputFile(tc.out, tc.jsonOut)
		if got != tc.want {
			t.Fatalf("effectiveOutputFile(%q, %q) = %q, want %q", tc.out, tc.jsonOut, got, tc.want)
		}
	}
}

func TestMaybeWarnDeprecatedJSONOut(t *testing.T) {
	var b bytes.Buffer
	maybeWarnDeprecatedJSONOut(&b, "")
	if b.String() != "" {
		t.Fatalf("expected no warning when jsonOut is empty, got %q", b.String())
	}

	b.Reset()
	maybeWarnDeprecatedJSONOut(&b, "/tmp/legacy.json")
	got := b.String()
	want := "warning: -json-out is deprecated; use -out instead\n"
	if got != want {
		t.Fatalf("expected warning %q, got %q", want, got)
	}
}

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
