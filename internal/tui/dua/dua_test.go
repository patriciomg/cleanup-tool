package dua

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/deps"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/tui/dockeritems"
	"github.com/patriciomg/cleanup-tool/internal/tui/tuitest"
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

func TestDepsView(t *testing.T) {
	m := newModel(makeTree())
	m.view = viewDeps
	m.depsList = []*deps.DependencyDir{
		{Path: "/tmp/a/node_modules", Type: "node_modules", Size: 100, AccessTime: time.Now(), ModTime: time.Now()},
	}
	v := m.View()
	if !strings.Contains(v, "node_modules") {
		t.Fatalf("expected deps view to show dependency directory, got:\n%s", v)
	}
}

func TestHandleDepsKey(t *testing.T) {
	m := newModel(makeTree())
	m.view = viewDeps
	m.depsList = []*deps.DependencyDir{
		{Path: "/tmp/a/node_modules", Type: "node_modules", Size: 100, AccessTime: time.Now(), ModTime: time.Now()},
		{Path: "/tmp/vendor", Type: "vendor", Size: 200, AccessTime: time.Now(), ModTime: time.Now()},
	}

	// Navigate down.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.depsSelected != 1 {
		t.Fatalf("expected depsSelected 1 after j, got %d", m.depsSelected)
	}

	// Mark selected (d).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.depsMarked["/tmp/vendor"] {
		t.Fatal("expected /tmp/vendor to be marked after d")
	}
}

func TestFilterDepsListClampsSelection(t *testing.T) {
	m := newModel(makeTree())
	m.depsList = []*deps.DependencyDir{
		{Path: "/tmp/a/node_modules", Type: "node_modules", Size: 100, AccessTime: time.Now(), ModTime: time.Now()},
		{Path: "/tmp/vendor", Type: "vendor", Size: 200, AccessTime: time.Now(), ModTime: time.Now()},
	}
	m.depsSelected = 1
	m.filterDepsList(map[string]bool{"/tmp/vendor": true})
	if m.depsSelected != 0 {
		t.Fatalf("expected depsSelected clamped to 0, got %d", m.depsSelected)
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

func TestDockerItemsView(t *testing.T) {
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Running:   true,
		UsageResp: &docker.Usage{},
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
	}
	m := newModel(nil)
	m.dockerClient = mock
	m.view = viewDockerItems
	m.dockerItems = dockeritems.New(m.dockerClient, "images", 80, 24)
	cmd := m.dockerItems.Init()
	if cmd != nil {
		updated, _ := m.dockerItems.Update(cmd())
		m.dockerItems = updated.(*dockeritems.Model)
	}
	v := m.dockerItems.View()
	if !strings.Contains(v, "img") {
		t.Fatalf("expected docker items view to show image name, got:\n%s", v)
	}
}

func TestDockerItemDeleteResetsSelection(t *testing.T) {
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
	}
	m := newModel(nil)
	m.dockerClient = mock
	m.view = viewDockerItems
	m.dockerItems = dockeritems.New(m.dockerClient, "images", 80, 24)
	// Load items into the sub-model.
	m.dockerItems, _ = tuitest.Send(m.dockerItems, m.dockerItems.Init()())

	// Press 'd' to select the current item for deletion.
	updated, _ := tuitest.SendKey(m.dockerItems, 'd')
	m.dockerItems = updated
	if !strings.Contains(m.dockerItems.View(), "Confirm Docker item deletion") {
		t.Fatalf("expected confirm view, got:\n%s", m.dockerItems.View())
	}

	// Press 'y' to confirm; a delete command should be returned.
	updated, delCmd := tuitest.SendKey(m.dockerItems, 'y')
	m.dockerItems = updated
	if delCmd == nil {
		t.Fatal("expected delete command to be returned")
	}

	// Execute the delete command and verify the mock recorded it.
	delCmd()
	if len(mock.Deleted) != 1 || mock.Deleted[0].ID != "abc" {
		t.Fatalf("expected mock to record deletion, got %v", mock.Deleted)
	}
}

func TestFilterItems(t *testing.T) {
	m := newModel(makeTree())
	m.filter = "a"
	m.rebuild()
	if len(m.items) != 1 || m.items[0].Path != "/tmp/a" {
		t.Fatalf("expected 1 filtered item (/tmp/a), got %v", m.items)
	}
}

func TestFilterModeTyping(t *testing.T) {
	m := newModel(makeTree())
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.filtering {
		t.Fatal("expected to enter filter mode after /")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if m.filter != "b" {
		t.Fatalf("expected filter 'b', got %q", m.filter)
	}
	if len(m.items) != 1 || m.items[0].Path != "/tmp/b" {
		t.Fatalf("expected 1 filtered item (/tmp/b), got %v", m.items)
	}
}

func TestFilterModeEscapeClearsFilter(t *testing.T) {
	m := newModel(makeTree())
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filtering {
		t.Fatal("expected to exit filter mode after esc")
	}
	if m.filter != "" {
		t.Fatalf("expected empty filter after esc, got %q", m.filter)
	}
	if len(m.items) != 2 {
		t.Fatalf("expected all 2 items after clearing filter, got %d", len(m.items))
	}
}
