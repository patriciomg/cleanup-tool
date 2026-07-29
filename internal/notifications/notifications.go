package notifications

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// Notify shows a native macOS notification with the given title and body.
// On non-Darwin platforms it is a no-op. The call is asynchronous and returns
// immediately so it cannot block the Bubble Tea update loop.
func Notify(title, body string) {
	if runtime.GOOS != "darwin" {
		return
	}
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	go exec.Command("osascript", "-e", script).Run()
}

// ScanComplete notifies the user that a scan finished. If the scan took 5s or
// less, or if notifications are disabled, no notification is shown.
func ScanComplete(elapsed time.Duration, enabled bool) {
	if !enabled || elapsed <= 5*time.Second {
		return
	}
	Notify("cleanup-tool", fmt.Sprintf("Scan finished in %s", elapsed.Round(time.Second)))
}
