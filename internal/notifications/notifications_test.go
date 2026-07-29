package notifications

import (
	"runtime"
	"testing"
)

func TestNotifyDoesNotPanic(t *testing.T) {
	// On non-Darwin platforms this is a no-op; on macOS it spawns an async
	// osascript. Either way it must not panic.
	Notify("test-title", "test-body")
}

func TestNotifyIsNoOpOnNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("only relevant on non-Darwin platforms")
	}
	// Should complete instantly without spawning anything.
	Notify("test-title", "test-body")
}
