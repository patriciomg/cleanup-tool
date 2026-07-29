package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/undo"
)

// Trash moves the given paths to the macOS Trash bin.
func Trash(paths ...string) error {
	return TrashWithOsascript("osascript", paths...)
}

// TrashWithOsascript is like Trash but uses the provided osascript command
// path. An empty osascript value defaults to "osascript".
func TrashWithOsascript(osascript string, paths ...string) error {
	_, err := TrashWithDestWithOsascript(osascript, paths...)
	return err
}

// TrashWithDest moves the given paths to the macOS Trash bin and returns the
// destination paths inside the Trash. It detects when Finder renames an item
// because of a naming conflict (e.g. "foo" becomes "foo 1") so that undo can
// restore the correct path.
func TrashWithDest(paths ...string) ([]string, error) {
	return TrashWithDestWithOsascript("osascript", paths...)
}

// TrashWithDestWithOsascript is like TrashWithDest but uses the provided
// osascript command path. An empty osascript value defaults to "osascript".
func TrashWithDestWithOsascript(osascript string, paths ...string) ([]string, error) {
	if osascript == "" {
		osascript = "osascript"
	}
	if len(paths) == 0 {
		return nil, nil
	}
	trashDir := filepath.Join(os.Getenv("HOME"), ".Trash")

	before, err := readTrashDir(trashDir)
	if err != nil {
		return nil, err
	}

	// Pre-compute the fallback destinations in case osascript fails or we are
	// on a non-macOS system where the Trash directory is used directly.
	fallback := make([]string, len(paths))
	for i, p := range paths {
		fallback[i] = filepath.Join(trashDir, filepath.Base(p))
	}

	// Use osascript so Finder handles the Trash correctly, including sound
	// and "Put Back" support.
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = quoteAppleScript(p)
	}
	script := fmt.Sprintf("tell application \"Finder\" to delete {%s}", strings.Join(quoted, ", "))
	cmd := exec.Command(osascript, "-e", script)
	if err := cmd.Run(); err != nil {
		// Fallback: move the items ourselves, resolving collisions so we don't
		// overwrite anything already in the Trash.
		for i, p := range paths {
			fallback[i] = freeTrashPath(trashDir, filepath.Base(p))
			if err := os.Rename(p, fallback[i]); err != nil {
				return nil, err
			}
		}
		return fallback, nil
	}

	after, err := readTrashDir(trashDir)
	if err != nil {
		return fallback, nil
	}

	for i, p := range paths {
		fallback[i] = findTrashDest(trashDir, filepath.Base(p), before, after)
	}
	return fallback, nil
}

func quoteAppleScript(s string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(s, "\"", "\\\""))
}

// readTrashDir returns a map of names in the Trash directory to their file
// info. Missing directories are treated as empty.
func readTrashDir(trashDir string) (map[string]os.FileInfo, error) {
	entries := make(map[string]os.FileInfo)
	dir, err := os.Open(trashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(trashDir, name))
		if err == nil {
			entries[name] = info
		}
	}
	return entries, nil
}

// findTrashDest returns the actual path of an item in the Trash after Finder
// moved it, accounting for conflict renames such as "foo" -> "foo 1".
//
// Note: this function compares a snapshot of the Trash taken before and after
// the osascript call. If another process modifies the Trash between those two
// snapshots, detection can be off. This is a best-effort approach.
func findTrashDest(trashDir, base string, before, after map[string]os.FileInfo) string {
	// If the exact base is present now and was not there before, that's the
	// item we are looking for.
	if _, ok := after[base]; ok {
		if _, existed := before[base]; !existed {
			return filepath.Join(trashDir, base)
		}
	}

	// Collect new items that match the expected basename or a Finder-style
	// conflict name ("name 1.ext").
	var candidates []string
	for name := range after {
		if _, existed := before[name]; existed {
			continue
		}
		if isTrashConflict(name, base) {
			candidates = append(candidates, name)
		}
	}

	if len(candidates) == 0 {
		return filepath.Join(trashDir, base)
	}

	// Pick the newest candidate; tie-break by name to keep the result
	// deterministic when two candidates share the same modification time.
	sort.Slice(candidates, func(i, j int) bool {
		var ti, tj time.Time
		if info := after[candidates[i]]; info != nil {
			ti = info.ModTime()
		}
		if info := after[candidates[j]]; info != nil {
			tj = info.ModTime()
		}
		if ti.Equal(tj) {
			return candidates[i] < candidates[j]
		}
		return ti.After(tj)
	})
	return filepath.Join(trashDir, candidates[0])
}

// isTrashConflict reports whether name matches base exactly or looks like a
// Finder-generated conflict name ("base 1", "base 1.ext", etc.).
func isTrashConflict(name, base string) bool {
	if name == base {
		return true
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if ext != "" {
		if !strings.HasSuffix(name, ext) {
			return false
		}
		name = strings.TrimSuffix(name, ext)
	}
	prefix := stem + " "
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	num := strings.TrimPrefix(name, prefix)
	_, err := strconv.Atoi(num)
	return err == nil
}

// freeTrashPath returns an unused path inside the Trash directory, appending
// a numeric suffix when base already exists.
func freeTrashPath(trashDir, base string) string {
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return filepath.Join(trashDir, base)
	}
	dest := filepath.Join(trashDir, base)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		var candidate string
		if ext == "" {
			candidate = filepath.Join(trashDir, fmt.Sprintf("%s %d", stem, n))
		} else {
			candidate = filepath.Join(trashDir, fmt.Sprintf("%s %d%s", stem, n, ext))
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// MoveToExternal rsyncs each source path to destDir on an external drive,
// verifies the copy, and removes the originals.
func MoveToExternal(destDir string, srcs ...string) error {
	return MoveToExternalWithRsync(destDir, "rsync", srcs...)
}

// MoveToExternalWithRsync is like MoveToExternal but uses the provided rsync
// command path. An empty rsync value defaults to "rsync".
func MoveToExternalWithRsync(destDir, rsync string, srcs ...string) error {
	_, err := MoveToExternalWithDestWithRsync(destDir, rsync, srcs...)
	return err
}

// MoveToExternalWithDest is like MoveToExternal but returns the actual
// destination paths used for each source.
func MoveToExternalWithDest(destDir string, srcs ...string) ([]string, error) {
	return MoveToExternalWithDestWithRsync(destDir, "rsync", srcs...)
}

// MoveToExternalWithDestWithRsync is like MoveToExternalWithDest but uses the
// provided rsync command path. An empty rsync value defaults to "rsync".
func MoveToExternalWithDestWithRsync(destDir, rsync string, srcs ...string) ([]string, error) {
	if rsync == "" {
		rsync = "rsync"
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create destination: %w", err)
	}
	dests := make([]string, len(srcs))
	for i, src := range srcs {
		destPath, err := moveOneToExternalWithRsync(src, destDir, rsync)
		if err != nil {
			return nil, err
		}
		dests[i] = destPath
	}
	return dests, nil
}

// moveOneToExternalWithRsync copies a single src to destDir using the provided
// rsync command. It is the internal implementation used by MoveToExternal* so
// that tests can inject a fake rsync without relying on PATH manipulation.
func moveOneToExternalWithRsync(src, destDir, rsync string) (string, error) {
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("source does not exist: %w", err)
	}

	name := filepath.Base(src)
	destPath := filepath.Join(destDir, name)
	if _, err := os.Stat(destPath); err == nil {
		destPath = filepath.Join(destDir, fmt.Sprintf("%s-%d", name, time.Now().Unix()))
	}

	cmd := exec.Command(rsync, "-avhP", "--remove-source-files", src, destDir+"/")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("rsync failed: %w\n%s", err, string(out))
	}

	// Verify copy exists.
	if info, err := os.Stat(destPath); err != nil || (info.IsDir() && !sameSize(src, destPath)) {
		return "", fmt.Errorf("verification failed after rsync")
	}

	// Remove any leftover source file or empty directory that rsync may have left.
	if _, err := os.Stat(src); err == nil {
		if err := os.RemoveAll(src); err != nil {
			return "", fmt.Errorf("remove source after rsync: %w", err)
		}
	}
	return destPath, nil
}

// Restore attempts to move a path from the macOS Trash back to its original
// parent directory. trashPath should be the exact path of the item inside the
// Trash (including any Finder-added numeric suffix). It is best-effort; the
// user can still use Finder's Put Back.
func Restore(trashPath, originalPath string) error {
	parent := filepath.Dir(originalPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent: %w", err)
	}
	if _, err := os.Stat(trashPath); err != nil {
		return fmt.Errorf("item not in trash: %w", err)
	}
	if err := os.Rename(trashPath, originalPath); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}
	return nil
}

// Undo reverses a single operation from the undo stack.
func Undo(op undo.Operation) error {
	return UndoWithRsync(op, "rsync")
}

// UndoWithRsync is like Undo but uses the provided rsync command path for
// cross-device fallbacks during OpMove reversions. An empty rsync value
// defaults to "rsync".
func UndoWithRsync(op undo.Operation, rsync string) error {
	if rsync == "" {
		rsync = "rsync"
	}
	switch op.Type {
	case undo.OpTrash:
		for _, it := range op.Items {
			if err := Restore(it.Dest, it.Original); err != nil {
				return err
			}
		}
	case undo.OpMove:
		for _, it := range op.Items {
			if err := moveBackWithRsync(it.Dest, it.Original, rsync); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
	return nil
}

// moveBackRename is an internal test seam that can be swapped to force the
// cross-device rsync fallback in unit tests.
var moveBackRename = os.Rename

// MoveBack moves src back to its original location. If the original path
// already exists, a suffix is appended to avoid overwriting. It uses rsync
// from PATH.
func MoveBack(src, original string) error {
	return moveBackWithRsync(src, original, "rsync")
}

// moveBackWithRsync is the internal implementation of MoveBack. It first tries
// to rename src onto original; when that fails (e.g., the two paths live on
// different filesystems), it falls back to the provided rsync command to copy
// the data and then removes the source.
//
// The rsync parameter lets callers (and tests) specify which rsync executable
// to use. An empty value defaults to "rsync".
func moveBackWithRsync(src, original, rsync string) error {
	if rsync == "" {
		rsync = "rsync"
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("source does not exist: %w", err)
	}
	parent := filepath.Dir(original)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent: %w", err)
	}
	dest := original
	if _, err := os.Stat(dest); err == nil {
		backup := fmt.Sprintf("%s-restored-%d", dest, time.Now().Unix())
		if err := os.Rename(dest, backup); err != nil {
			return fmt.Errorf("backup existing destination: %w", err)
		}
	}
	if err := moveBackRename(src, dest); err == nil {
		return nil
	}
	// Cross-device fallback using rsync.
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		cmd := exec.Command(rsync, "-avhP", "--remove-source-files", src+"/", dest+"/")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("rsync failed: %w\n%s", err, string(out))
		}
		// rsync with --remove-source-files leaves the now-empty source directory.
		if err := os.RemoveAll(src); err != nil {
			return fmt.Errorf("remove leftover source dir: %w", err)
		}
	} else {
		cmd := exec.Command(rsync, "-avhP", "--remove-source-files", src, dest)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("rsync failed: %w\n%s", err, string(out))
		}
	}
	return nil
}

func sameSize(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return ai.Size() == bi.Size()
}
