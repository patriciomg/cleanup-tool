package undo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OpType classifies an undoable operation.
type OpType string

const (
	// OpTrash marks an item moved to the Trash.
	OpTrash OpType = "trash"
	// OpMove marks an item moved to an external directory.
	OpMove OpType = "move"
)

// Item records a single original path and the path it ended up at after an
// operation (Trash or MoveToExternal).
type Item struct {
	Original string // Path before the operation.
	Dest     string // Path after the operation.
}

// Operation is a single undoable action.
type Operation struct {
	Type      OpType
	Items     []Item
	Timestamp time.Time
}

// Stack is a thread-safe bounded stack of operations.
type Stack struct {
	mu          sync.Mutex
	saveMu      sync.Mutex
	ops         []Operation
	maxLen      int
	persistPath string
	lastErr     error
}

// NewStack returns a bounded stack that keeps at most max operations. A max
// value less than or equal to zero defaults to 10.
func NewStack(max int) *Stack {
	if max <= 0 {
		max = 10
	}
	return &Stack{maxLen: max}
}

// Push adds an operation to the top of the stack, dropping the oldest item if
// the stack has exceeded its maximum size. If a persist path has been set, the
// stack is written to disk after the update.
func (s *Stack) Push(op Operation) {
	s.mu.Lock()
	s.ops = append(s.ops, op)
	if len(s.ops) > s.maxLen {
		s.ops = s.ops[len(s.ops)-s.maxLen:]
	}
	path := s.persistPath
	ops := make([]Operation, len(s.ops))
	copy(ops, s.ops)
	s.mu.Unlock()

	if path != "" {
		s.save(path, ops)
	}
}

// Pop removes and returns the most recent operation. The second return value
// is false when the stack is empty. If a persist path has been set, the stack
// is written to disk after the update.
func (s *Stack) Pop() (Operation, bool) {
	s.mu.Lock()
	if len(s.ops) == 0 {
		s.mu.Unlock()
		return Operation{}, false
	}
	idx := len(s.ops) - 1
	op := s.ops[idx]
	s.ops = s.ops[:idx]
	path := s.persistPath
	ops := make([]Operation, len(s.ops))
	copy(ops, s.ops)
	s.mu.Unlock()

	if path != "" {
		s.save(path, ops)
	}
	return op, true
}

// Len returns the current number of operations in the stack.
func (s *Stack) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ops)
}

// Peek returns the most recent operation without removing it. The second return
// value is false when the stack is empty.
func (s *Stack) Peek() (Operation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ops) == 0 {
		return Operation{}, false
	}
	return s.ops[len(s.ops)-1], true
}

// Load reads the persisted operations from path and replaces the current stack.
// The path is remembered so that future Push/Pop calls persist automatically.
// Missing files are treated as an empty stack; corrupt files return an error.
func (s *Stack) Load(path string) error {
	s.mu.Lock()
	s.persistPath = path
	s.mu.Unlock()

	if path == "" {
		return nil
	}

	// Serialize with any in-progress save so a concurrent writer doesn't
	// overwrite the loaded state after we release the lock.
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var ops []Operation
	if err := json.Unmarshal(data, &ops); err != nil {
		return fmt.Errorf("parse undo file: %w", err)
	}

	if len(ops) > s.maxLen {
		ops = ops[len(ops)-s.maxLen:]
	}

	s.mu.Lock()
	s.ops = ops
	s.mu.Unlock()
	return nil
}

// Save writes the current stack to path using an atomic write. It does not
// change the stack's configured persistPath; use Load to set the auto-save
// path.
func (s *Stack) Save(path string) error {
	s.mu.Lock()
	ops := make([]Operation, len(s.ops))
	copy(ops, s.ops)
	s.mu.Unlock()

	return saveOps(path, ops)
}

// save persists ops to path and records the last error. Writes are serialized
// by saveMu so concurrent Push/Pop calls don't leave the file in a stale
// state.
func (s *Stack) save(path string, ops []Operation) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.lastErr = saveOps(path, ops)
}

// LastError returns the most recent persistence error, if any.
func (s *Stack) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// saveOps writes ops to path atomically (temp file + rename).
func saveOps(path string, ops []Operation) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(ops, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
