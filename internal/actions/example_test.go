package actions

import (
	"fmt"

	"github.com/patriciomg/cleanup-tool/internal/undo"
)

// ExampleMoveToExternalWithRsync shows how to copy directories to an external
// drive using a specific rsync executable.
func ExampleMoveToExternalWithRsync() {
	if err := MoveToExternalWithRsync(
		"/Volumes/ExternalDrive/backups",
		"/usr/local/bin/rsync",
		"/Users/me/Documents",
		"/Users/me/Photos",
	); err != nil {
		panic(err)
	}
	fmt.Println("copied to external drive")
}

// ExampleUndoWithRsync shows how to undo a move operation while specifying the
// rsync executable that is used for cross-device fallbacks.
func ExampleUndoWithRsync() {
	op := undo.Operation{
		Type: undo.OpMove,
		Items: []undo.Item{
			{
				Original: "/Users/me/original.txt",
				Dest:     "/Volumes/External/moved.txt",
			},
		},
	}
	if err := UndoWithRsync(op, "/usr/local/bin/rsync"); err != nil {
		panic(err)
	}
	fmt.Println("undid move")
}
