package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScannerOnProgress(t *testing.T) {
	tmp := t.TempDir()
	must(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644))
	must(t, os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"), 0o644))

	var events int
	scanner := NewScanner(nil, false, 0, false)
	scanner.progressStep = 1
	scanner.OnProgress = func(p Progress) {
		events++
	}
	_, err := scanner.Scan(context.Background(), []string{tmp})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if events == 0 {
		t.Errorf("expected at least one progress event, got %d", events)
	}
}

func TestScannerCancellation(t *testing.T) {
	tmp := t.TempDir()
	// Create a deep enough tree to allow cancellation mid-scan.
	for i := 0; i < 10; i++ {
		dir := filepath.Join(tmp, "dir", filepath.Join(repeat("d", i)...))
		must(t, os.MkdirAll(dir, 0o755))
		must(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scanner := NewScanner(nil, false, 0, false)
	roots, err := scanner.Scan(ctx, []string{tmp})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Scanned {
		t.Errorf("expected cancelled scan to mark root as unscanned")
	}
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}
