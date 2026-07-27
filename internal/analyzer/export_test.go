package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportJSON(t *testing.T) {
	root := &Entry{Name: "root", Path: "/tmp/root", IsDir: true, Size: 100}
	child := &Entry{Name: "child.txt", Path: "/tmp/root/child.txt", Size: 100}
	root.AddChild(child)

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "scan.json")

	if err := ExportJSON([]*Entry{root}, outPath); err != nil {
		t.Fatalf("ExportJSON error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}

	var roots []*Entry
	if err := json.Unmarshal(data, &roots); err != nil {
		t.Fatalf("unmarshaling exported JSON: %v", err)
	}

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Name != "root" {
		t.Fatalf("expected root name %q, got %q", "root", roots[0].Name)
	}
	if len(roots[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(roots[0].Children))
	}
	if roots[0].Children[0].Name != "child.txt" {
		t.Fatalf("expected child name %q, got %q", "child.txt", roots[0].Children[0].Name)
	}
	// Parent should not be present in JSON to avoid cycles.
	if roots[0].Children[0].Parent != nil {
		t.Fatalf("expected Parent to be nil after JSON round-trip")
	}
}

func TestExportJSONInvalidPath(t *testing.T) {
	root := &Entry{Name: "root", Path: "/tmp/root", IsDir: true}
	err := ExportJSON([]*Entry{root}, "/nonexistent_dir/test.json")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestExportJSONWriter(t *testing.T) {
	root := &Entry{Name: "root", Path: "/tmp/root", IsDir: true, Size: 100}
	child := &Entry{Name: "child.txt", Path: "/tmp/root/child.txt", Size: 100}
	root.AddChild(child)

	var buf strings.Builder
	if err := ExportJSONWriter([]*Entry{root}, &buf); err != nil {
		t.Fatalf("ExportJSONWriter error: %v", err)
	}

	var roots []*Entry
	if err := json.Unmarshal([]byte(buf.String()), &roots); err != nil {
		t.Fatalf("unmarshaling exported JSON: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Name != "root" {
		t.Fatalf("expected root name %q, got %q", "root", roots[0].Name)
	}
	if len(roots[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(roots[0].Children))
	}
}
