package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScannerWalkSizes(t *testing.T) {
	tmp := t.TempDir()

	// Create a small tree:
	// tmp/
	//   a/
	//     file1.txt (content "hello")
	//   b/
	//     file2.txt (content "world")
	//   top.txt (content "top")
	aDir := filepath.Join(tmp, "a")
	bDir := filepath.Join(tmp, "b")
	must(t, os.MkdirAll(aDir, 0o755))
	must(t, os.MkdirAll(bDir, 0o755))
	must(t, os.WriteFile(filepath.Join(aDir, "file1.txt"), []byte("hello"), 0o644))
	must(t, os.WriteFile(filepath.Join(bDir, "file2.txt"), []byte("world"), 0o644))
	must(t, os.WriteFile(filepath.Join(tmp, "top.txt"), []byte("top"), 0o644))

	scanner := NewScanner(nil, false, 0)
	roots, err := scanner.Scan(context.Background(), []string{tmp})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	root := roots[0]
	if !root.IsDir {
		t.Fatalf("expected root to be a directory")
	}

	expectedSize := int64(len("hello") + len("world") + len("top"))
	if root.Size != expectedSize {
		t.Errorf("expected root size %d, got %d", expectedSize, root.Size)
	}
	if root.NumFiles != 3 {
		t.Errorf("expected 3 files, got %d", root.NumFiles)
	}
	if root.NumDirs != 2 {
		t.Errorf("expected 2 dirs, got %d", root.NumDirs)
	}
}

func TestScannerFollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	tmp := t.TempDir()

	target := filepath.Join(tmp, "target.txt")
	link := filepath.Join(tmp, "link.txt")

	must(t, os.WriteFile(target, []byte("hello"), 0o644))
	must(t, os.Symlink(target, link))

	scanner := NewScanner(nil, false, 0)
	roots, err := scanner.Scan(context.Background(), []string{tmp})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	root := roots[0]
	// The symlink is followed, so its target's content is counted again.
	expectedSize := int64(len("hello") * 2)
	if root.Size != expectedSize {
		t.Errorf("expected root size %d, got %d", expectedSize, root.Size)
	}
	if root.NumFiles != 2 {
		t.Errorf("expected 2 files, got %d", root.NumFiles)
	}
}

func TestScannerFollowsSymlinkToDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	tmp := t.TempDir()

	aDir := filepath.Join(tmp, "a")
	bLink := filepath.Join(tmp, "b")
	must(t, os.MkdirAll(aDir, 0o755))
	must(t, os.WriteFile(filepath.Join(aDir, "file.txt"), []byte("world"), 0o644))
	must(t, os.Symlink(aDir, bLink))

	scanner := NewScanner(nil, false, 0)
	roots, err := scanner.Scan(context.Background(), []string{tmp})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	root := roots[0]
	// The symlinked directory is followed, so the single file is counted twice.
	expectedSize := int64(len("world") * 2)
	if root.Size != expectedSize {
		t.Errorf("expected root size %d, got %d", expectedSize, root.Size)
	}
	if root.NumFiles != 2 {
		t.Errorf("expected 2 files, got %d", root.NumFiles)
	}
}

func TestScannerBrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	tmp := t.TempDir()

	link := filepath.Join(tmp, "broken")
	must(t, os.Symlink(filepath.Join(tmp, "missing"), link))

	scanner := NewScanner(nil, false, 0)
	roots, err := scanner.Scan(context.Background(), []string{tmp})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	root := roots[0]
	if root.NumFiles != 1 {
		t.Errorf("expected broken symlink to be counted as 1 file, got %d", root.NumFiles)
	}

	// The broken symlink entry should be reported but unscanned.
	found := FindEntryByPath(root, link)
	if found == nil {
		t.Fatalf("broken symlink entry not found")
	}
	if found.Scanned {
		t.Errorf("expected broken symlink entry to be unscanned")
	}
	if found.Error == nil {
		t.Errorf("expected broken symlink entry to have an error")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
