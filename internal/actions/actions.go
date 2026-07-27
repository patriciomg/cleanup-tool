package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Trash moves the given paths to the macOS Trash bin.
func Trash(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	// Use osascript so Finder handles the Trash correctly, including sound
	// and "Put Back" support.
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = quoteAppleScript(p)
	}
	script := fmt.Sprintf("tell application \"Finder\" to delete {%s}", strings.Join(quoted, ", "))
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		// Fallback to ~/.Trash if osascript fails
		for _, p := range paths {
			dest := filepath.Join(os.Getenv("HOME"), ".Trash", filepath.Base(p))
			if err := os.Rename(p, dest); err != nil {
				return err
			}
		}
	}
	return nil
}

func quoteAppleScript(s string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(s, "\"", "\\\""))
}

// MoveToExternal rsyncs each source path to destDir on an external drive,
// verifies the copy, and removes the originals.
func MoveToExternal(destDir string, srcs ...string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	for _, src := range srcs {
		if err := moveOneToExternal(src, destDir); err != nil {
			return err
		}
	}
	return nil
}

func moveOneToExternal(src string, destDir string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("source does not exist: %w", err)
	}

	name := filepath.Base(src)
	destPath := filepath.Join(destDir, name)
	if _, err := os.Stat(destPath); err == nil {
		destPath = filepath.Join(destDir, fmt.Sprintf("%s-%d", name, time.Now().Unix()))
	}

	cmd := exec.Command("rsync", "-avhP", "--remove-source-files", src, destDir+"/")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync failed: %w\n%s", err, string(out))
	}

	// Verify copy exists.
	if info, err := os.Stat(destPath); err != nil || (info.IsDir() && !sameSize(src, destPath)) {
		return fmt.Errorf("verification failed after rsync")
	}

	// Remove any leftover source file or empty directory that rsync may have left.
	if _, err := os.Stat(src); err == nil {
		if err := os.RemoveAll(src); err != nil {
			return fmt.Errorf("remove source after rsync: %w", err)
		}
	}
	return nil
}

// Restore attempts to move a path from the macOS Trash back to its original
// parent directory. It is best-effort; the user can still use Finder's Put Back.
func Restore(trashDir, originalPath string) error {
	name := filepath.Base(originalPath)
	trashPath := filepath.Join(trashDir, name)
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
