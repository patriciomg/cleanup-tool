package undo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStackDefaultSize(t *testing.T) {
	s := NewStack(0)
	if s.maxLen != 10 {
		t.Fatalf("expected default maxLen 10, got %d", s.maxLen)
	}
}

func TestPushAndPop(t *testing.T) {
	s := NewStack(3)
	s.Push(Operation{Type: OpTrash})
	s.Push(Operation{Type: OpMove})

	if s.Len() != 2 {
		t.Fatalf("expected length 2, got %d", s.Len())
	}

	op, ok := s.Pop()
	if !ok {
		t.Fatal("expected Pop to succeed")
	}
	if op.Type != OpMove {
		t.Fatalf("expected last op type move, got %s", op.Type)
	}

	if s.Len() != 1 {
		t.Fatalf("expected length 1 after pop, got %d", s.Len())
	}
}

func TestStackDropsOldestWhenFull(t *testing.T) {
	s := NewStack(2)
	s.Push(Operation{Type: OpTrash, Items: []Item{{Original: "a"}}})
	s.Push(Operation{Type: OpTrash, Items: []Item{{Original: "b"}}})
	s.Push(Operation{Type: OpTrash, Items: []Item{{Original: "c"}}})

	if s.Len() != 2 {
		t.Fatalf("expected length 2, got %d", s.Len())
	}

	op, _ := s.Pop()
	if op.Items[0].Original != "c" {
		t.Fatalf("expected newest item c, got %s", op.Items[0].Original)
	}
	op, _ = s.Pop()
	if op.Items[0].Original != "b" {
		t.Fatalf("expected second item b, got %s", op.Items[0].Original)
	}
}

func TestPopEmptyStack(t *testing.T) {
	s := NewStack(10)
	_, ok := s.Pop()
	if ok {
		t.Fatal("expected Pop on empty stack to fail")
	}
}

func TestPeek(t *testing.T) {
	s := NewStack(10)
	s.Push(Operation{Type: OpMove})
	op, ok := s.Peek()
	if !ok {
		t.Fatal("expected Peek to succeed")
	}
	if op.Type != OpMove {
		t.Fatalf("expected move op, got %s", op.Type)
	}
	if s.Len() != 1 {
		t.Fatalf("expected length 1 after Peek, got %d", s.Len())
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "undo.json")

	s := NewStack(10)
	s.Push(Operation{Type: OpTrash, Items: []Item{{Original: "/a", Dest: "/trash/a"}}, Timestamp: time.Now()})
	s.Push(Operation{Type: OpMove, Items: []Item{{Original: "/b", Dest: "/ext/b"}}, Timestamp: time.Now()})

	if err := s.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded := NewStack(10)
	if err := loaded.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Len() != 2 {
		t.Fatalf("expected length 2 after load, got %d", loaded.Len())
	}
	op, _ := loaded.Pop()
	if op.Type != OpMove {
		t.Fatalf("expected last op move, got %s", op.Type)
	}
	if op.Items[0].Original != "/b" {
		t.Fatalf("expected original /b, got %s", op.Items[0].Original)
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "undo.json")

	s := NewStack(10)
	if err := s.Load(path); err != nil {
		t.Fatalf("Load on missing file should not error, got: %v", err)
	}
	if s.Len() != 0 {
		t.Fatalf("expected empty stack after missing file load, got %d", s.Len())
	}
}

func TestAutoSaveOnPushAndPop(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "undo.json")

	s := NewStack(10)
	if err := s.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	s.Push(Operation{Type: OpTrash, Items: []Item{{Original: "/a", Dest: "/trash/a"}}, Timestamp: time.Now()})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("undo file not written after Push: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("undo file is empty after Push")
	}

	s.Pop()
	if s.Len() != 0 {
		t.Fatalf("expected empty stack after Pop, got %d", s.Len())
	}

	loaded := NewStack(10)
	if err := loaded.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Len() != 0 {
		t.Fatalf("expected persisted empty stack, got %d", loaded.Len())
	}
}

func TestLoadRespectsMaxLen(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "undo.json")

	s := NewStack(5)
	for i := 0; i < 10; i++ {
		s.Push(Operation{Type: OpTrash, Items: []Item{{Original: filepath.Join("/", string(rune('a'+i)))}}, Timestamp: time.Now()})
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded := NewStack(3)
	if err := loaded.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Len() != 3 {
		t.Fatalf("expected length 3 after load with maxLen 3, got %d", loaded.Len())
	}
}
