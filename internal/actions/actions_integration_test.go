package actions

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMoveBackCrossDevice exercises the rsync fallback in MoveBack using a real
// macOS disk image. os.Rename fails across volumes, so the code must fall back
// to rsync.
func TestMoveBackCrossDevice(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("cross-device integration test requires macOS (hdiutil)")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not found")
	}
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not found")
	}

	workDir := t.TempDir()
	dmgPath := filepath.Join(workDir, "test.dmg")
	mountPt := filepath.Join(workDir, "mnt")

	if err := os.Mkdir(mountPt, 0o755); err != nil {
		t.Fatalf("create mount point: %v", err)
	}

	createCmd := exec.Command("hdiutil", "create", "-size", "2m", "-fs", "HFS+", "-volname", "CrossDeviceVol", dmgPath)
	if out, err := createCmd.CombinedOutput(); err != nil {
		t.Fatalf("hdiutil create failed: %v\n%s", err, string(out))
	}

	attachCmd := exec.Command("hdiutil", "attach", "-mountpoint", mountPt, dmgPath)
	if out, err := attachCmd.CombinedOutput(); err != nil {
		t.Fatalf("hdiutil attach failed: %v\n%s", err, string(out))
	}
	defer func() {
		if out, err := exec.Command("hdiutil", "detach", mountPt, "-force").CombinedOutput(); err != nil {
			t.Logf("hdiutil detach failed: %v\n%s", err, string(out))
		}
	}()

	t.Run("file", func(t *testing.T) {
		src := filepath.Join(mountPt, "test-file.txt")
		dest := filepath.Join(t.TempDir(), "original-file.txt")
		writeFile(t, src, "integration-data")

		if err := MoveBack(src, dest); err != nil {
			t.Fatalf("MoveBack failed: %v", err)
		}

		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Errorf("expected source to be removed from external volume")
		}
		if readFile(t, dest) != "integration-data" {
			t.Errorf("expected file contents to survive cross-device migration")
		}
	})

	t.Run("directory", func(t *testing.T) {
		srcDir := filepath.Join(mountPt, "test-dir")
		destDir := filepath.Join(t.TempDir(), "original-dir")

		if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
			t.Fatalf("create src dir: %v", err)
		}
		writeFile(t, filepath.Join(srcDir, "nested", "data.txt"), "dir-data")

		if err := MoveBack(srcDir, destDir); err != nil {
			t.Fatalf("MoveBack directory failed: %v", err)
		}

		if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
			t.Errorf("expected source dir to be fully removed from external volume")
		}
		if readFile(t, filepath.Join(destDir, "nested", "data.txt")) != "dir-data" {
			t.Errorf("expected nested file contents to survive cross-device migration")
		}
	})
}
