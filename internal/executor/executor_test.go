package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/rules"
)

func TestExecuteRuleDryRun(t *testing.T) {
	dir := t.TempDir()
	// Use a plain name so the file is not also classified as log/cache.
	oldFile := filepath.Join(dir, "oldfile")
	recentFile := filepath.Join(dir, "recentfile")

	writeFile(t, oldFile, "old content")
	writeFile(t, recentFile, "recent content")
	// Make oldFile older than 30 days.
	oldTime := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	rule := rules.Rule{
		Name:             "test-dry-run",
		Paths:            []string{dir},
		Action:           "trash",
		Categories:       []string{"old"},
		AgeThresholdDays: 30,
		DryRun:           true,
	}

	res := ExecuteRule(context.Background(), rule, Options{Yes: true})
	if res.Error != nil {
		t.Fatalf("ExecuteRule failed: %v", res.Error)
	}
	if res.DryRun != true {
		t.Fatalf("expected DryRun true")
	}
	if res.MatchedFiles != 1 {
		t.Fatalf("expected 1 matched file, got %d", res.MatchedFiles)
	}
	// Ensure the file was not actually deleted.
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("dry-run deleted the file: %v", err)
	}
}

func TestExecuteRuleProtectedPath(t *testing.T) {
	rule := rules.Rule{
		Name:   "test-protected",
		Paths:  []string{"/"},
		Action: "trash",
	}
	res := ExecuteRule(context.Background(), rule, Options{Yes: true})
	if res.Error == nil {
		t.Fatalf("expected error for protected path")
	}
}

func TestExecuteRuleMaxDeletedBytes(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "oldfile")
	recentFile := filepath.Join(dir, "recentfile")

	writeFile(t, oldFile, "old content")
	writeFile(t, recentFile, "recent content")
	oldTime := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	rule := rules.Rule{
		Name:             "test-max-size",
		Paths:            []string{dir},
		Action:           "trash",
		Categories:       []string{"old"},
		AgeThresholdDays: 30,
		MaxDeletedBytes:  1, // far less than the matched file size
		DryRun:           false,
	}

	res := ExecuteRule(context.Background(), rule, Options{Yes: true})
	if res.Error != nil {
		t.Fatalf("ExecuteRule failed: %v", res.Error)
	}
	if !res.AbortedMaxSize {
		t.Fatalf("expected AbortedMaxSize true")
	}
	if len(res.DeletedPaths) != 0 {
		t.Fatalf("expected no deletions when aborted")
	}
}

func TestExecuteRuleSymlinkProtected(t *testing.T) {
	tmp := t.TempDir()
	symlink := filepath.Join(tmp, "link")
	if err := os.Symlink("/", symlink); err != nil {
		t.Fatal(err)
	}

	rule := rules.Rule{
		Name:   "test-symlink",
		Paths:  []string{symlink},
		Action: "trash",
	}
	res := ExecuteRule(context.Background(), rule, Options{Yes: true})
	if res.Error == nil {
		t.Fatalf("expected error for symlink pointing to protected path")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
