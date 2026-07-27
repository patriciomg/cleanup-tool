package dua

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
)

func makeTree() []*analyzer.Entry {
	root := &analyzer.Entry{Path: "/tmp", Name: "tmp", IsDir: true, Size: 300}
	a := &analyzer.Entry{Path: "/tmp/a", Name: "a", IsDir: true, Size: 200, Parent: root}
	b := &analyzer.Entry{Path: "/tmp/b", Name: "b", IsDir: false, Size: 100, Parent: root}
	c := &analyzer.Entry{Path: "/tmp/a/c", Name: "c", IsDir: false, Size: 50, Parent: a}
	a.Children = []*analyzer.Entry{c}
	root.Children = []*analyzer.Entry{a, b}
	return []*analyzer.Entry{root}
}

func TestHandleScanResult(t *testing.T) {
	m := New(false)
	roots := makeTree()
	m.Update(scanMsg{roots: roots})

	if m.current == nil || m.current.Path != "/tmp" {
		t.Fatalf("expected current dir /tmp, got %v", m.current)
	}
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}
	// Should be sorted by size descending.
	if m.items[0].Path != "/tmp/a" {
		t.Fatalf("expected /tmp/a first, got %s", m.items[0].Path)
	}
}

func TestDescendAndAscend(t *testing.T) {
	m := New(false)
	m.Update(scanMsg{roots: makeTree()})

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.current.Path != "/tmp/a" {
		t.Fatalf("expected to descend into /tmp/a, got %s", m.current.Path)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.current.Path != "/tmp" {
		t.Fatalf("expected to ascend to /tmp, got %s", m.current.Path)
	}
}

func TestToggleMark(t *testing.T) {
	m := New(false)
	m.Update(scanMsg{roots: makeTree()})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.marked["/tmp/a"] {
		t.Fatal("expected /tmp/a to be marked")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.marked["/tmp/a"] {
		t.Fatal("expected /tmp/a to be unmarked")
	}
}

func TestViewDoesNotPanic(t *testing.T) {
	m := New(false)
	_ = m.View()
	m.Update(scanMsg{roots: makeTree()})
	v := m.View()
	if !strings.Contains(v, "Dua-style Browser") {
		t.Fatal("expected view to contain header")
	}
}

func TestHelpView(t *testing.T) {
	m := New(false)
	m.Update(scanMsg{roots: makeTree()})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	v := m.View()
	if !strings.Contains(v, "Dua-style browser key bindings") {
		t.Fatal("expected help view to show key bindings")
	}
}
