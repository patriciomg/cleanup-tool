package actions

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/undo"
)

func TestIsTrashConflict(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		match bool
	}{
		{"foo", "foo", true},
		{"foo 1", "foo", true},
		{"foo 1.txt", "foo.txt", true},
		{"foo 123.txt", "foo.txt", true},
		{"foo.txt", "foo.txt", true},
		{"bar", "foo", false},
		{"foo1", "foo", false},
		{"foo 1 2", "foo", false},
	}
	for _, c := range cases {
		if got := isTrashConflict(c.name, c.base); got != c.match {
			t.Errorf("isTrashConflict(%q, %q) = %v, want %v", c.name, c.base, got, c.match)
		}
	}
}

func TestFreeTrashPath(t *testing.T) {
	dir := t.TempDir()

	first := freeTrashPath(dir, "foo.txt")
	if first != filepath.Join(dir, "foo.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(dir, "foo.txt"), first)
	}

	// Create the first file so the next call picks a conflict name.
	if err := createEmptyFile(first); err != nil {
		t.Fatalf("create file: %v", err)
	}
	second := freeTrashPath(dir, "foo.txt")
	if second != filepath.Join(dir, "foo 1.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(dir, "foo 1.txt"), second)
	}

	if err := createEmptyFile(second); err != nil {
		t.Fatalf("create file: %v", err)
	}
	third := freeTrashPath(dir, "foo.txt")
	if third != filepath.Join(dir, "foo 2.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(dir, "foo 2.txt"), third)
	}
}

func TestFindTrashDest(t *testing.T) {
	dir := t.TempDir()
	before := map[string]os.FileInfo{}

	// Simulate a conflict where Finder renamed "foo.txt" to "foo 1.txt".
	createEmptyFile(filepath.Join(dir, "foo.txt"))
	createEmptyFile(filepath.Join(dir, "foo 1.txt"))

	after := map[string]os.FileInfo{}
	for _, name := range []string{"foo.txt", "foo 1.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		after[name] = info
	}

	got := findTrashDest(dir, "foo.txt", nil, before, after)
	if got != filepath.Join(dir, "foo.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(dir, "foo.txt"), got)
	}

	// Now pretend "foo.txt" already existed in the Trash.
	before["foo.txt"] = after["foo.txt"]
	got = findTrashDest(dir, "foo.txt", nil, before, after)
	if got != filepath.Join(dir, "foo 1.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(dir, "foo 1.txt"), got)
	}
}

func TestFindTrashDestPrefersNewest(t *testing.T) {
	dir := t.TempDir()
	before := map[string]os.FileInfo{}

	createEmptyFile(filepath.Join(dir, "foo 1.txt"))
	createEmptyFile(filepath.Join(dir, "foo 2.txt"))

	// Ensure different modification times.
	newer := filepath.Join(dir, "foo 2.txt")
	older := filepath.Join(dir, "foo 1.txt")
	if err := os.Chtimes(older, time.Now(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	after := map[string]os.FileInfo{}
	for _, name := range []string{"foo 1.txt", "foo 2.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		after[name] = info
	}

	got := findTrashDest(dir, "foo.txt", nil, before, after)
	if got != newer {
		t.Fatalf("expected newest %s, got %s", newer, got)
	}
}

func createEmptyFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}

func TestFindTrashDestConsumesCandidates(t *testing.T) {
	dir := t.TempDir()
	before := map[string]os.FileInfo{}

	// Simulate two distinct source files with the same basename "foo.txt" that
	// ended up as "foo.txt" and "foo 1.txt" in the Trash.
	writeFile(t, filepath.Join(dir, "foo.txt"), "first")
	writeFile(t, filepath.Join(dir, "foo 1.txt"), "second")

	after := map[string]os.FileInfo{}
	for _, name := range []string{"foo.txt", "foo 1.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		after[name] = info
	}

	first := findTrashDest(dir, "foo.txt", nil, before, after)
	if first != filepath.Join(dir, "foo.txt") {
		t.Fatalf("expected first match %s, got %s", filepath.Join(dir, "foo.txt"), first)
	}

	// Simulate the caller consuming the match so the next same-named source
	// gets the remaining candidate.
	delete(after, "foo.txt")

	second := findTrashDest(dir, "foo.txt", nil, before, after)
	if second != filepath.Join(dir, "foo 1.txt") {
		t.Fatalf("expected second match %s, got %s", filepath.Join(dir, "foo 1.txt"), second)
	}
}

func TestFindTrashDestMatchesMetadata(t *testing.T) {
	dir := t.TempDir()
	before := map[string]os.FileInfo{}

	// Create two conflict candidates; the newer one should NOT be the match
	// because the original metadata matches the older candidate.
	older := filepath.Join(dir, "foo 1.txt")
	newer := filepath.Join(dir, "foo 2.txt")
	writeFile(t, older, "match")
	writeFile(t, newer, "mismatch")
	if err := os.Chtimes(older, time.Now(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	after := map[string]os.FileInfo{}
	for _, name := range []string{"foo 1.txt", "foo 2.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		after[name] = info
	}

	// Build a fake FileInfo that matches the older candidate.
	stat, err := os.Stat(older)
	if err != nil {
		t.Fatalf("stat older: %v", err)
	}

	got := findTrashDest(dir, "foo.txt", stat, before, after)
	if got != older {
		t.Fatalf("expected metadata match %s, got %s", older, got)
	}
}

func TestRestoreAvoidsOverwrite(t *testing.T) {
	trash := filepath.Join(t.TempDir(), "trash.txt")
	original := filepath.Join(t.TempDir(), "original.txt")
	writeFile(t, trash, "trashed")
	writeFile(t, original, "replaced")

	if err := Restore(trash, original); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if readFile(t, original) != "trashed" {
		t.Fatalf("expected original to contain trashed content")
	}
	matches, err := filepath.Glob(original + "-restored-*")
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %d", len(matches))
	}
	if readFile(t, matches[0]) != "replaced" {
		t.Fatalf("expected backup to contain original content")
	}
}

func TestMoveBackSourceMissing(t *testing.T) {
	src := filepath.Join(t.TempDir(), "missing.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	writeFile(t, dest, "untouched")

	err := MoveBack(src, dest)
	if err == nil {
		t.Fatalf("expected MoveBack to fail when source does not exist")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
	if readFile(t, dest) != "untouched" {
		t.Fatalf("expected destination to remain untouched when source is missing")
	}
}

func TestMoveBackParentNotWritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}

	src := filepath.Join(t.TempDir(), "src.txt")
	writeFile(t, src, "data")

	grandparent := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(grandparent, 0o555); err != nil {
		t.Fatalf("create readonly dir: %v", err)
	}
	defer os.Chmod(grandparent, 0o755)

	dest := filepath.Join(grandparent, "parent", "dest.txt")
	err := MoveBack(src, dest)
	if err == nil {
		t.Fatalf("expected MoveBack to fail when parent directory cannot be created")
	}
}

func TestMoveBackFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	writeFile(t, src, "hello")

	if err := MoveBack(src, dest); err != nil {
		t.Fatalf("MoveBack failed: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected source to be removed")
	}
	if readFile(t, dest) != "hello" {
		t.Fatalf("expected destination content to be preserved")
	}
}

func TestMoveBackDirectory(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatalf("create src: %v", err)
	}
	writeFile(t, filepath.Join(src, "nested", "file.txt"), "data")

	if err := MoveBack(src, dest); err != nil {
		t.Fatalf("MoveBack failed: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected source to be removed")
	}
	if readFile(t, filepath.Join(dest, "nested", "file.txt")) != "data" {
		t.Fatalf("expected directory contents to be preserved")
	}
}

func TestMoveBackConflictRenames(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	original := filepath.Join(t.TempDir(), "dest.txt")
	writeFile(t, src, "new")
	writeFile(t, original, "old")

	if err := MoveBack(src, original); err != nil {
		t.Fatalf("MoveBack failed: %v", err)
	}

	if readFile(t, original) != "new" {
		t.Fatalf("expected original to be replaced with source")
	}

	restored := original + "-restored"
	matches, err := filepath.Glob(restored + "-*")
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one restored file, got %d", len(matches))
	}
	if readFile(t, matches[0]) != "old" {
		t.Fatalf("expected old content to be in restored file")
	}
}

func TestMoveBackCrossDeviceRsyncFails(t *testing.T) {
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	// Create a fake rsync that always exits non-zero.
	tmpDir := t.TempDir()
	fakeRsync := filepath.Join(tmpDir, "rsync")
	script := "#!/bin/sh\necho 'rsync failed' >&2\nexit 1\n"
	if err := os.WriteFile(fakeRsync, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}

	src := filepath.Join(t.TempDir(), "src.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	writeFile(t, src, "data")

	err := moveBackWithRsync(src, dest, fakeRsync)
	if err == nil {
		t.Fatalf("expected MoveBack to fail when rsync exits non-zero")
	}
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source to remain in place when rsync fails")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected destination to not be created when rsync fails")
	}
}

func TestMoveBackCrossDeviceRsyncMissing(t *testing.T) {
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	src := filepath.Join(t.TempDir(), "src.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	writeFile(t, src, "data")

	err := moveBackWithRsync(src, dest, filepath.Join(t.TempDir(), "nonexistent-rsync"))
	if err == nil {
		t.Fatalf("expected MoveBack to fail when rsync is missing")
	}
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source to remain in place when rsync fallback fails")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected destination to not be created when rsync fallback fails")
	}
}

func TestMoveBackFileCrossDevice(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	src := filepath.Join(t.TempDir(), "src.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	writeFile(t, src, "cross-device")

	if err := MoveBack(src, dest); err != nil {
		t.Fatalf("MoveBack failed: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected source to be removed after rsync fallback")
	}
	if readFile(t, dest) != "cross-device" {
		t.Fatalf("expected destination content after rsync fallback")
	}
}

func TestMoveBackDirectoryCrossDevice(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	src := filepath.Join(t.TempDir(), "src")
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatalf("create src: %v", err)
	}
	writeFile(t, filepath.Join(src, "nested", "file.txt"), "data")

	if err := MoveBack(src, dest); err != nil {
		t.Fatalf("MoveBack failed: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected source to be removed after rsync fallback")
	}
	if readFile(t, filepath.Join(dest, "nested", "file.txt")) != "data" {
		t.Fatalf("expected directory contents after rsync fallback")
	}
}

func TestUndoTrash(t *testing.T) {
	trashPath := filepath.Join(t.TempDir(), "file.txt")
	original := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, trashPath, "data")

	op := undo.Operation{
		Type: undo.OpTrash,
		Items: []undo.Item{
			{Original: original, Dest: trashPath},
		},
	}
	if err := Undo(op); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Fatalf("expected trash path to be empty after undo")
	}
	if readFile(t, original) != "data" {
		t.Fatalf("expected original to be restored")
	}
}

func TestUndoMove(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "moved.txt")
	original := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, dest, "data")

	op := undo.Operation{
		Type: undo.OpMove,
		Items: []undo.Item{
			{Original: original, Dest: dest},
		},
	}
	if err := Undo(op); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected dest path to be empty after undo")
	}
	if readFile(t, original) != "data" {
		t.Fatalf("expected original to be restored")
	}
}

func TestUndoStopsOnFirstFailure(t *testing.T) {
	dest0 := filepath.Join(t.TempDir(), "moved-0.txt")
	original0 := filepath.Join(t.TempDir(), "original-0.txt")
	writeFile(t, dest0, "data-0")

	// dest1 exists initially but is removed before the call, so MoveBack fails.
	dest1 := filepath.Join(t.TempDir(), "moved-1.txt")
	original1 := filepath.Join(t.TempDir(), "original-1.txt")
	writeFile(t, dest1, "data-1")
	if err := os.Remove(dest1); err != nil {
		t.Fatalf("remove dest1: %v", err)
	}

	dest2 := filepath.Join(t.TempDir(), "moved-2.txt")
	original2 := filepath.Join(t.TempDir(), "original-2.txt")
	writeFile(t, dest2, "data-2")

	op := undo.Operation{
		Type: undo.OpMove,
		Items: []undo.Item{
			{Original: original0, Dest: dest0},
			{Original: original1, Dest: dest1},
			{Original: original2, Dest: dest2},
		},
	}
	if err := Undo(op); err == nil {
		t.Fatalf("expected Undo to fail when an item is missing")
	}

	// First item should have been processed before the failure.
	if readFile(t, original0) != "data-0" {
		t.Fatalf("expected first item to be restored before the failure")
	}

	// Second item was missing, so it should remain gone and not be restored.
	if _, err := os.Stat(dest1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing item to remain untouched")
	}
	if _, err := os.Stat(original1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing item's original to remain unrestored")
	}

	// Third item should not be processed because Undo stopped early.
	if _, err := os.Stat(dest2); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected third item to remain in place when Undo stops early")
	}
	if _, err := os.Stat(original2); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected third original to remain unrestored when Undo stops early")
	}
}

func TestUndoMultiple(t *testing.T) {
	cases := []struct {
		name      string
		opType    undo.OpType
		srcTmpl   string
		origTmpl  string
	}{
		{"Move", undo.OpMove, "moved-%d.txt", "original-%d.txt"},
		{"Trash", undo.OpTrash, "trashed-%d.txt", "original-%d.txt"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var items []undo.Item
			var srcPaths []string
			var originals []string

			for i := range 3 {
				src := filepath.Join(t.TempDir(), fmt.Sprintf(c.srcTmpl, i))
				original := filepath.Join(t.TempDir(), fmt.Sprintf(c.origTmpl, i))
				writeFile(t, src, fmt.Sprintf("data-%d", i))
				srcPaths = append(srcPaths, src)
				originals = append(originals, original)
				items = append(items, undo.Item{Original: original, Dest: src})
			}

			op := undo.Operation{Type: c.opType, Items: items}
			if err := Undo(op); err != nil {
				t.Fatalf("Undo failed: %v", err)
			}

			for i, src := range srcPaths {
				if _, err := os.Stat(src); !os.IsNotExist(err) {
					t.Fatalf("expected source %s to be removed after undo", src)
				}
				if readFile(t, originals[i]) != fmt.Sprintf("data-%d", i) {
					t.Fatalf("expected original %s to be restored", originals[i])
				}
			}
		})
	}
}

func TestUndoWithRsyncFake(t *testing.T) {
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	tmpDir := t.TempDir()
	fakeRsync := filepath.Join(tmpDir, "rsync")
	script := "#!/bin/sh\necho 'rsync failed' >&2\nexit 1\n"
	if err := os.WriteFile(fakeRsync, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "moved.txt")
	original := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, dest, "data")

	op := undo.Operation{
		Type: undo.OpMove,
		Items: []undo.Item{
			{Original: original, Dest: dest},
		},
	}
	if err := UndoWithRsync(op, fakeRsync); err == nil {
		t.Fatalf("expected UndoWithRsync to fail when rsync exits non-zero")
	}
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source to remain in place when rsync fails")
	}
	if _, err := os.Stat(original); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected original to remain unrestored when rsync fails")
	}
}

func TestVolumeRoot(t *testing.T) {
	root, ok := volumeRoot("/")
	if !ok {
		t.Fatalf("volumeRoot(/) failed")
	}
	if root != "/" {
		t.Fatalf("expected volume root of / to be /, got %s", root)
	}

	tmp := t.TempDir()
	root, ok = volumeRoot(tmp)
	if !ok {
		t.Fatalf("volumeRoot(%s) failed", tmp)
	}
	if root != "/" {
		t.Fatalf("expected volume root of temp dir to be boot root, got %s", root)
	}
}

func TestTrashDirForBootVolume(t *testing.T) {
	tmp := t.TempDir()
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat temp dir: %v", err)
	}
	t.Setenv("HOME", "/tmp/fakehome")
	got := trashDirFor(tmp, info, "/tmp/fakehome", 501)
	if got != "/tmp/fakehome/.Trash" {
		t.Fatalf("expected boot-volume trash dir, got %s", got)
	}
}

func TestTrashWithDestFallbackSameNamedFiles(t *testing.T) {
	// Use a fresh HOME so the test does not touch the user's real Trash.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Use a fake osascript that always fails so the fallback path is exercised.
	fakeOsascript := filepath.Join(t.TempDir(), "osascript")
	script := "#!/bin/sh\necho 'osascript failed' >&2\nexit 1\n"
	if err := os.WriteFile(fakeOsascript, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake osascript: %v", err)
	}

	tmp := t.TempDir()
	dir1 := filepath.Join(tmp, "a")
	dir2 := filepath.Join(tmp, "b")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatalf("create dir1: %v", err)
	}
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatalf("create dir2: %v", err)
	}

	p1 := filepath.Join(dir1, "foo.txt")
	p2 := filepath.Join(dir2, "foo.txt")
	writeFile(t, p1, "first")
	writeFile(t, p2, "second")

	dests, err := TrashWithDestWithOsascript(fakeOsascript, p1, p2)
	if err != nil {
		t.Fatalf("TrashWithDestWithOsascript failed: %v", err)
	}

	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations, got %d", len(dests))
	}
	if dests[0] == dests[1] {
		t.Fatalf("expected distinct destinations, both got %s", dests[0])
	}

	// The first file should land at the base name; the second should be renamed
	// to avoid a collision.
	if filepath.Base(dests[0]) != "foo.txt" {
		t.Fatalf("expected first destination base to be foo.txt, got %s", filepath.Base(dests[0]))
	}
	if filepath.Base(dests[1]) != "foo 1.txt" {
		t.Fatalf("expected second destination base to be 'foo 1.txt', got %s", filepath.Base(dests[1]))
	}

	for _, p := range []string{p1, p2} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected source %s to be removed", p)
		}
	}
	for _, d := range dests {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("expected destination %s to exist: %v", d, err)
		}
	}
}

func TestIntegrationTrashRealOsascript(t *testing.T) {
	if os.Getenv("TEST_REAL_OSASCRIPT") == "" {
		t.Skip("Skipping real osascript integration tests; set TEST_REAL_OSASCRIPT=1 to run")
	}

	// Finder may not be responsive in headless CI environments. Ping it first.
	if err := exec.Command("osascript", "-e", "tell application \"Finder\" to get version").Run(); err != nil {
		t.Skipf("Finder is not responding to osascript in this environment: %v", err)
	}

	// Use a unique basename so we can locate the item in Trash without
	// interfering with anything else and clean it up afterwards.
	base := fmt.Sprintf("cleanup-tool-test-%d", time.Now().UnixNano())
	src := filepath.Join(t.TempDir(), base)
	writeFile(t, src, "real-trash-test")

	dests, err := TrashWithDestWithOsascript("osascript", src)
	if err != nil {
		t.Fatalf("TrashWithDestWithOsascript failed: %v", err)
	}
	if len(dests) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(dests))
	}

	// Verify the item is in the boot-volume Trash and matches the original.
	trashPath := dests[0]
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("expected trashed item to exist at %s: %v", trashPath, err)
	}
	if readFile(t, trashPath) != "real-trash-test" {
		t.Fatalf("expected trashed item to preserve its content")
	}
	// Defensive cleanup: if the test fails before undo, remove the item from
	// the real Trash so we do not pollute the CI runner.
	t.Cleanup(func() {
		_ = os.RemoveAll(trashPath)
	})

	// Undo restores the item to its original path and removes it from Trash.
	op := undo.Operation{
		Type: undo.OpTrash,
		Items: []undo.Item{
			{Original: src, Dest: trashPath},
		},
	}
	if err := Undo(op); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Fatalf("expected trashed item to be removed from Trash after undo")
	}
	if readFile(t, src) != "real-trash-test" {
		t.Fatalf("expected original item to be restored")
	}
}

func TestIntegrationTrashExternalVolume(t *testing.T) {
	if os.Getenv("TEST_REAL_OSASCRIPT") == "" {
		t.Skip("Skipping real osascript integration tests; set TEST_REAL_OSASCRIPT=1 to run")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not available, skipping external volume test")
	}
	if err := exec.Command("osascript", "-e", "tell application \"Finder\" to get version").Run(); err != nil {
		t.Skipf("Finder is not responding to osascript in this environment: %v", err)
	}

	volName := fmt.Sprintf("CleanupTestVol%d", time.Now().UnixNano())
	dmgPath := filepath.Join(t.TempDir(), "testvol.dmg")
	mountPoint := filepath.Join("/Volumes", volName)

	// Create a small external volume image.
	cmd := exec.Command("hdiutil", "create", "-size", "50m", "-fs", "HFS+", "-volname", volName, dmgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hdiutil create failed: %v\n%s", err, string(out))
	}

	// Mount it.
	cmd = exec.Command("hdiutil", "attach", dmgPath, "-quiet")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hdiutil attach failed: %v\n%s", err, string(out))
	}

	var trashPath string
	t.Cleanup(func() {
		_ = os.RemoveAll(trashPath)
		_ = exec.Command("hdiutil", "detach", mountPoint, "-force").Run()
	})

	// Give Finder a moment to recognize the new volume.
	time.Sleep(1 * time.Second)

	base := fmt.Sprintf("ext-trash-test-%d.txt", time.Now().UnixNano())
	src := filepath.Join(mountPoint, base)
	writeFile(t, src, "external-volume-data")

	dests, err := TrashWithDestWithOsascript("osascript", src)
	if err != nil {
		t.Fatalf("TrashWithDestWithOsascript failed: %v", err)
	}
	if len(dests) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(dests))
	}

	trashPath = dests[0]
	expectedTrashDir := filepath.Join(mountPoint, ".Trashes", strconv.Itoa(osGetuid()))
	if filepath.Dir(trashPath) != expectedTrashDir {
		t.Fatalf("expected trash path in %s, got %s", expectedTrashDir, trashPath)
	}
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("expected trashed item to exist at %s: %v", trashPath, err)
	}
	if readFile(t, trashPath) != "external-volume-data" {
		t.Fatalf("expected trashed item to preserve its content")
	}

	// Un-trash the file.
	op := undo.Operation{
		Type: undo.OpTrash,
		Items: []undo.Item{
			{Original: src, Dest: trashPath},
		},
	}
	if err := Undo(op); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Fatalf("expected trashed item to be removed from Trash after undo")
	}
	if readFile(t, src) != "external-volume-data" {
		t.Fatalf("expected original item to be restored")
	}
}

func TestUndoWithRsyncMissing(t *testing.T) {
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	dest := filepath.Join(t.TempDir(), "moved.txt")
	original := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, dest, "data")

	op := undo.Operation{
		Type: undo.OpMove,
		Items: []undo.Item{
			{Original: original, Dest: dest},
		},
	}
	if err := UndoWithRsync(op, filepath.Join(t.TempDir(), "nonexistent-rsync")); err == nil {
		t.Fatalf("expected UndoWithRsync to fail when rsync is missing")
	}
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source to remain in place when rsync is missing")
	}
	if _, err := os.Stat(original); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected original to remain unrestored when rsync is missing")
	}
}

func TestUndoMoveCrossDevice(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
	origRename := moveBackRename
	moveBackRename = func(_, _ string) error { return fmt.Errorf("cross-device") }
	defer func() { moveBackRename = origRename }()

	dest := filepath.Join(t.TempDir(), "moved.txt")
	original := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, dest, "cross-device-undo")

	op := undo.Operation{
		Type: undo.OpMove,
		Items: []undo.Item{
			{Original: original, Dest: dest},
		},
	}
	if err := Undo(op); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected dest path to be empty after undo")
	}
	if readFile(t, original) != "cross-device-undo" {
		t.Fatalf("expected original to be restored via rsync fallback")
	}
}
