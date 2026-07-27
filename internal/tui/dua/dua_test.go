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

func newModel(roots []*analyzer.Entry) *Model {
	m := New(false, "", nil, analyzer.DupHashSmart, 100)
	if roots != nil {
		m.Update(scanMsg{roots: roots})
	}
	return m
}

func execCmd(m *Model, cmd tea.Cmd) *Model {
	if cmd == nil {
		return m
	}
	mod, _ := m.Update(cmd())
	return mod.(*Model)
}

func TestHandleScanResult(t *testing.T) {
	m := newModel(nil)
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
	m := newModel(makeTree())

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
	m := newModel(makeTree())

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
	m := newModel(nil)
	_ = m.View()
	m = newModel(makeTree())
	v := m.View()
	if !strings.Contains(v, "Dua-style Browser") {
		t.Fatal("expected view to contain header")
	}
}

func TestHelpView(t *testing.T) {
	m := newModel(makeTree())
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	v := m.View()
	if !strings.Contains(v, "Dua-style browser key bindings") {
		t.Fatal("expected help view to show key bindings")
	}
}

func TestMoveWithoutExternalDirShowsError(t *testing.T) {
	m := newModel(makeTree())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = execCmd(m, cmd)
	v := m.View()
	if !strings.Contains(v, "no external dir set") {
		t.Fatalf("expected error message for missing external dir, got:\n%s", v)
	}
}

func TestAnalyzerViewShowsHints(t *testing.T) {
	m := newModel(makeTree())
	m.hints = []*analyzer.DeletabilityHint{
		{Entry: &analyzer.Entry{Path: "/tmp/old.log", Name: "old.log"}, Reason: analyzer.ReasonOld, Detail: "last accessed 2024-01-01"},
		{Entry: &analyzer.Entry{Path: "/tmp/dup", Name: "dup"}, Reason: analyzer.ReasonDuplicate, Detail: "2 duplicates"},
	}
	m.view = viewAnalyzer
	v := m.View()
	if !strings.Contains(v, "Found 2 hints") {
		t.Fatalf("expected analyzer view to show hint count, got:\n%s", v)
	}
}

func TestDockerViewRequiresClient(t *testing.T) {
	m := newModel(nil)
	m.view = viewDocker
	m.Update(m.fetchDockerUsage())
	v := m.View()
	if !strings.Contains(v, "docker client not available") {
		t.Fatalf("expected docker client unavailable message, got:\n%s", v)
	}
}
