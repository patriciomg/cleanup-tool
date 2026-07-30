package deps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFinderFindsTargets(t *testing.T) {
	root := t.TempDir()

	// project-a/node_modules with 2 + 4 = 6 bytes
	a := filepath.Join(root, "project-a", "node_modules")
	mustWriteFile(t, filepath.Join(a, "a", "package.json"), "pkg")
	mustWriteFile(t, filepath.Join(a, "b", "index.js"), "x") // 1 byte

	// project-a/vendor with 5 bytes
	b := filepath.Join(root, "project-a", "vendor")
	mustWriteFile(t, filepath.Join(b, "go.mod"), "hello") // 5 bytes

	// project-a/.venv with 7 bytes
	c := filepath.Join(root, "project-a", ".venv")
	mustWriteFile(t, filepath.Join(c, "lib", "x"), "abcdefg") // 7 bytes

	// project-b/Pods with 4 bytes
	d := filepath.Join(root, "project-b", "Pods")
	mustWriteFile(t, filepath.Join(d, "SomePod", "file"), "1234") // 4 bytes

	// Nested node_modules inside project-a/node_modules should roll into the
	// outer one, not be reported separately.
	mustWriteFile(t, filepath.Join(a, "a", "node_modules", "c", "x"), "yy") // 2 bytes

	finder := NewFinder(DefaultTargets(), nil, false)
	results, err := finder.Find(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 dependency dirs, got %d: %v", len(results), results)
	}

	byPath := make(map[string]*DependencyDir)
	for _, d := range results {
		byPath[d.Path] = d
	}

	nodePath := filepath.Join(root, "project-a", "node_modules")
	if _, ok := byPath[nodePath]; !ok {
		t.Fatalf("missing node_modules at %s", nodePath)
	}
	if byPath[nodePath].Size != 6 {
		t.Fatalf("expected node_modules size 6, got %d", byPath[nodePath].Size)
	}
	if byPath[nodePath].Type != "node_modules" {
		t.Fatalf("expected type node_modules, got %s", byPath[nodePath].Type)
	}

	vendorPath := filepath.Join(root, "project-a", "vendor")
	if _, ok := byPath[vendorPath]; !ok {
		t.Fatalf("missing vendor at %s", vendorPath)
	}
	if byPath[vendorPath].Size != 5 {
		t.Fatalf("expected vendor size 5, got %d", byPath[vendorPath].Size)
	}

	venvPath := filepath.Join(root, "project-a", ".venv")
	if _, ok := byPath[venvPath]; !ok {
		t.Fatalf("missing .venv at %s", venvPath)
	}
	if byPath[venvPath].Size != 7 {
		t.Fatalf("expected .venv size 7, got %d", byPath[venvPath].Size)
	}

	podsPath := filepath.Join(root, "project-b", "Pods")
	if _, ok := byPath[podsPath]; !ok {
		t.Fatalf("missing Pods at %s", podsPath)
	}
	if byPath[podsPath].Size != 4 {
		t.Fatalf("expected Pods size 4, got %d", byPath[podsPath].Size)
	}

	// Ensure the nested node_modules was not reported separately.
	nestedPath := filepath.Join(root, "project-a", "node_modules", "a", "node_modules")
	if _, ok := byPath[nestedPath]; ok {
		t.Fatalf("nested node_modules should not be reported separately: %s", nestedPath)
	}
}

func TestFinderIgnoresHidden(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".hidden", "node_modules", "pkg", "x"), "abc")
	mustWriteFile(t, filepath.Join(root, "visible", "node_modules", "pkg", "x"), "def")

	finder := NewFinder(DefaultTargets(), nil, true)
	results, err := finder.Find(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != "node_modules" {
		t.Fatalf("expected node_modules, got %s", results[0].Type)
	}
}

func TestFinderRespectsIgnorePaths(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, "skip")
	mustWriteFile(t, filepath.Join(ignorePath, "node_modules", "a", "x"), "123")
	mustWriteFile(t, filepath.Join(root, "keep", "node_modules", "b", "x"), "456")

	finder := NewFinder(DefaultTargets(), []string{ignorePath}, false)
	results, err := finder.Find(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Size != 3 {
		t.Fatalf("expected size 3, got %d", results[0].Size)
	}
}

func TestDefaultTargets(t *testing.T) {
	want := []string{
		"node_modules",
		".pnpm",
		"vendor",
		".venv",
		"venv",
		"bower_components",
		"Pods",
		"Carthage",
		".gradle",
		".m2",
		"target",
		".tox",
		"packages",
		".nuget",
		".stack-work",
		"elm-stuff",
		"_build",
	}
	got := DefaultTargets()
	if len(got) != len(want) {
		t.Fatalf("DefaultTargets length mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DefaultTargets()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFinderFindsExpandedTargets(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "rust", "target", "debug", "app"), "x")
	mustWriteFile(t, filepath.Join(root, "maven", ".m2", "repository", "a", "b"), "y")
	mustWriteFile(t, filepath.Join(root, "pnpm", ".pnpm", "store", "pkg"), "z")
	mustWriteFile(t, filepath.Join(root, "tox", ".tox", "py", "bin"), "w")
	mustWriteFile(t, filepath.Join(root, "nuget", "packages", "pkg", "x"), "v")
	mustWriteFile(t, filepath.Join(root, "build", "out.js"), "u")

	finder := NewFinder(DefaultTargets(), nil, false)
	results, err := finder.Find(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}

	byType := make(map[string]string)
	for _, d := range results {
		byType[d.Type] = d.Path
	}
	wantTypes := []string{"target", ".m2", ".pnpm", ".tox", "packages"}
	for _, typ := range wantTypes {
		if _, ok := byType[typ]; !ok {
			t.Fatalf("expected to find %s, got types: %v", typ, byType)
		}
	}
}

func TestSortResults(t *testing.T) {
	a := &DependencyDir{Path: "/a", Size: 100}
	b := &DependencyDir{Path: "/b", Size: 200}
	c := &DependencyDir{Path: "/c", Size: 50}

	results := []*DependencyDir{a, b, c}
	SortResults(results, "size")
	if results[0].Path != "/b" || results[1].Path != "/a" || results[2].Path != "/c" {
		t.Fatalf("unexpected sort order: %v", results)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
