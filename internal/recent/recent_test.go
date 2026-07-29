package recent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	origPath := path
	path = func() string { return filepath.Join(dir, "recent.json") }
	defer func() { path = origPath }()

	p := []string{"/foo", "/bar"}
	if err := Save(p); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Paths()
	if err != nil {
		t.Fatalf("Paths error: %v", err)
	}
	if !slices.Equal(loaded, p) {
		t.Fatalf("unexpected recent paths: got %v, want %v", loaded, p)
	}
}

func TestSaveDeduplicates(t *testing.T) {
	dir := t.TempDir()
	origPath := path
	path = func() string { return filepath.Join(dir, "recent.json") }
	defer func() { path = origPath }()

	if err := Save([]string{"/a"}); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if err := Save([]string{"/a"}); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Paths()
	if err != nil {
		t.Fatalf("Paths error: %v", err)
	}
	if !slices.Equal(loaded, []string{"/a"}) {
		t.Fatalf("unexpected recent paths: %v", loaded)
	}
}

func TestSaveLimitsEntries(t *testing.T) {
	dir := t.TempDir()
	origPath := path
	path = func() string { return filepath.Join(dir, "recent.json") }
	defer func() { path = origPath }()

	for i := 0; i < maxEntries+2; i++ {
		if err := Save([]string{string(rune('a' + i))}); err != nil {
			t.Fatalf("Save error: %v", err)
		}
	}
	entries, err := load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(entries) != maxEntries {
		t.Fatalf("expected %d entries, got %d", maxEntries, len(entries))
	}
}

func TestPathsMissingFile(t *testing.T) {
	dir := t.TempDir()
	origPath := path
	path = func() string { return filepath.Join(dir, "missing.json") }
	defer func() { path = origPath }()

	loaded, err := Paths()
	if err != nil {
		t.Fatalf("Paths error: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty paths, got %v", loaded)
	}
}

func TestPathsBadJSON(t *testing.T) {
	dir := t.TempDir()
	origPath := path
	path = func() string { return filepath.Join(dir, "recent.json") }
	defer func() { path = origPath }()

	if err := os.WriteFile(path(), []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	_, err := Paths()
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}
