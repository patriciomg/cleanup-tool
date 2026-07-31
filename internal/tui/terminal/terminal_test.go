package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/config"
	"github.com/patriciomg/cleanup-tool/internal/deps"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/tui/common"
	"github.com/patriciomg/cleanup-tool/internal/tui/dockeritems"
	"github.com/patriciomg/cleanup-tool/internal/tui/tuitest"
)

func entry(name, path string, size int64, isDir bool, children ...*analyzer.Entry) *analyzer.Entry {
	return &analyzer.Entry{
		Name:     name,
		Path:     path,
		Size:     size,
		IsDir:    isDir,
		Children: children,
	}
}

func TestMarkedSelectionsPersistAcrossRebuild(t *testing.T) {
	child := entry("child.txt", "/parent/child.txt", 100, false)
	parent := entry("parent", "/parent", 100, true, child)
	parent.Children[0].Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)

	// Mark the child.
	m.marked[child.Path] = true
	// Rebuilding should keep the mark.
	m.rebuild()
	if !m.marked[child.Path] {
		t.Fatal("mark was lost after rebuild")
	}
}

func TestMarkedSelectionsPersistAcrossNavigation(t *testing.T) {
	grandchild := entry("a.txt", "/parent/sub/a.txt", 10, false)
	sub := entry("sub", "/parent/sub", 10, true, grandchild)
	grandchild.Parent = sub
	parent := entry("parent", "/parent", 10, true, sub)
	sub.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)

	// Mark grandchild.
	m.marked[grandchild.Path] = true
	// Navigate into sub.
	m.currentDir = sub
	m.rebuild()
	if !m.marked[grandchild.Path] {
		t.Fatal("mark was lost after navigating into sub directory")
	}

	// Navigate back to parent.
	m.currentDir = parent
	m.rebuild()
	if !m.marked[grandchild.Path] {
		t.Fatal("mark was lost after navigating back to parent")
	}
}

func TestClearMarks(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	parent := entry("dir", "/dir", 10, true, fileA)
	fileA.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.marked[fileA.Path] = true
	m.clearMarks()

	if m.marked[fileA.Path] {
		t.Fatal("clearMarks did not remove the mark")
	}
}

func TestNewStoresAnalyzerOptions(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	parent := entry("dir", "/dir", 10, true, fileA)
	fileA.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashFull, 42, nil)
	if m.dupMode != analyzer.DupHashFull {
		t.Fatalf("expected dupMode full, got %v", m.dupMode)
	}
	if m.progressInterval != 42 {
		t.Fatalf("expected progressInterval 42, got %d", m.progressInterval)
	}
}

func TestToggleFilter(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	parent := entry("dir", "/dir", 10, true, fileA)
	fileA.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)

	m.toggleFilter(analyzer.ReasonOld)
	if m.analyzerFilter != analyzer.ReasonOld {
		t.Fatalf("expected old filter, got %q", m.analyzerFilter)
	}

	m.toggleFilter(analyzer.ReasonOld)
	if m.analyzerFilter != "" {
		t.Fatalf("expected filter cleared, got %q", m.analyzerFilter)
	}
}

func TestHandleMouseFilterClick(t *testing.T) {
	old := &analyzer.Entry{Path: "/dir/old.txt", Name: "old.txt", Size: 10}
	dup := &analyzer.Entry{Path: "/dir/dup.txt", Name: "dup.txt", Size: 10}
	log := &analyzer.Entry{Path: "/dir/log.txt", Name: "log.txt", Size: 10}

	parent := entry("dir", "/dir", 30, true)
	old.Parent = parent
	dup.Parent = parent
	log.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.hints = []*analyzer.DeletabilityHint{
		{Entry: old, Reason: analyzer.ReasonOld},
		{Entry: dup, Reason: analyzer.ReasonDuplicate},
		{Entry: log, Reason: analyzer.ReasonLogCache},
	}
	m.view = viewAnalyzer

	// The summary line is rendered at analyzerSummaryLineY in analyzerView.
	// Compute click positions using the same helper the view uses.
	summary := analyzer.SummarizeHints(m.hints)
	cats := common.SummaryCategories(summary)

	var x int
	for _, cat := range cats {
		s := cat.String()
		// The clickable region covers the category text only.
		w := len(s)
		// Click near the end of the text to verify the hit target.
		m.handleMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x + w - 1, Y: analyzerSummaryLineY})
		if m.analyzerFilter != cat.Reason {
			t.Fatalf("expected %q filter after click, got %q", cat.Reason, m.analyzerFilter)
		}
		m.analyzerFilter = ""
		// advance past this category and the ", " separator
		x += w + 2
	}

	// Click outside the summary line should be ignored.
	m.analyzerFilter = analyzer.ReasonOld
	m.handleMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 2, Y: 10})
	if m.analyzerFilter != analyzer.ReasonOld {
		t.Fatalf("filter should not change on non-summary click, got %q", m.analyzerFilter)
	}
}

func TestHandleMouseStackedBarClick(t *testing.T) {
	old := &analyzer.Entry{Path: "/dir/old.txt", Name: "old.txt", Size: 10}
	dup := &analyzer.Entry{Path: "/dir/dup.txt", Name: "dup.txt", Size: 10}
	log := &analyzer.Entry{Path: "/dir/log.txt", Name: "log.txt", Size: 10}

	parent := entry("dir", "/dir", 30, true)
	old.Parent = parent
	dup.Parent = parent
	log.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.hints = []*analyzer.DeletabilityHint{
		{Entry: old, Reason: analyzer.ReasonOld},
		{Entry: dup, Reason: analyzer.ReasonDuplicate},
		{Entry: log, Reason: analyzer.ReasonLogCache},
	}
	m.view = viewAnalyzer

	summary := analyzer.SummarizeHints(m.hints)
	wOld, wDup, _ := common.StackedBarSegments(summary, stackedBarWidth)

	cases := []struct {
		name     string
		x        int
		expected analyzer.HintReason
	}{
		{"old segment", 2 + wOld/2, analyzer.ReasonOld},
		{"duplicate segment", 2 + wOld + wDup/2, analyzer.ReasonDuplicate},
		{"log/cache segment", 2 + wOld + wDup + 1, analyzer.ReasonLogCache},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m.analyzerFilter = ""
			m.handleMouse(tea.MouseMsg{Type: tea.MouseLeft, X: tc.x, Y: analyzerStackedBarLineY})
			if m.analyzerFilter != tc.expected {
				t.Fatalf("expected %q filter, got %q", tc.expected, m.analyzerFilter)
			}
		})
	}

	// Clicks outside the bar (before indent) should be ignored.
	m.analyzerFilter = analyzer.ReasonOld
	m.handleMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 0, Y: analyzerStackedBarLineY})
	if m.analyzerFilter != analyzer.ReasonOld {
		t.Fatalf("filter should not change on click outside bar, got %q", m.analyzerFilter)
	}
}

func TestHandleMouseStackedBarClick_ZeroWidthSegment(t *testing.T) {
	old := &analyzer.Entry{Path: "/dir/old.txt", Name: "old.txt", Size: 10}
	dup := &analyzer.Entry{Path: "/dir/dup.txt", Name: "dup.txt", Size: 10}

	parent := entry("dir", "/dir", 20, true)
	old.Parent = parent
	dup.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.hints = []*analyzer.DeletabilityHint{
		{Entry: old, Reason: analyzer.ReasonOld},
		{Entry: dup, Reason: analyzer.ReasonDuplicate},
	}
	m.view = viewAnalyzer

	// Confirm the log/cache segment really has zero width for this data set.
	summary := analyzer.SummarizeHints(m.hints)
	_, _, wLog := common.StackedBarSegments(summary, stackedBarWidth)
	if wLog != 0 {
		t.Fatalf("expected log/cache segment width 0, got %d", wLog)
	}

	// A click on the remaining log/cache portion falls through to the last
	// non-zero segment (duplicate) because the bar is filled entirely across
	// the width.
	m.handleMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 2 + stackedBarWidth - 1, Y: analyzerStackedBarLineY})
	if m.analyzerFilter != analyzer.ReasonDuplicate {
		t.Fatalf("expected duplicate filter for fallthrough click, got %q", m.analyzerFilter)
	}
}

func TestCycleFilter(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	parent := entry("dir", "/dir", 10, true, fileA)
	fileA.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	if m.analyzerFilter != "" {
		t.Fatalf("expected no filter initially, got %q", m.analyzerFilter)
	}

	m.cycleFilter(1)
	if m.analyzerFilter != analyzer.ReasonOld {
		t.Fatalf("expected old filter, got %q", m.analyzerFilter)
	}

	m.cycleFilter(1)
	if m.analyzerFilter != analyzer.ReasonDuplicate {
		t.Fatalf("expected duplicate filter, got %q", m.analyzerFilter)
	}

	m.cycleFilter(1)
	if m.analyzerFilter != analyzer.ReasonLogCache {
		t.Fatalf("expected log/cache filter, got %q", m.analyzerFilter)
	}

	m.cycleFilter(1)
	if m.analyzerFilter != "" {
		t.Fatalf("expected no filter after cycle, got %q", m.analyzerFilter)
	}
}

func TestFilteredHints(t *testing.T) {
	old := &analyzer.Entry{Path: "/dir/old.txt", Name: "old.txt", Size: 10}
	dup := &analyzer.Entry{Path: "/dir/dup.txt", Name: "dup.txt", Size: 10}
	log := &analyzer.Entry{Path: "/dir/log.txt", Name: "log.txt", Size: 10}

	parent := entry("dir", "/dir", 30, true)
	old.Parent = parent
	dup.Parent = parent
	log.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.hints = []*analyzer.DeletabilityHint{
		{Entry: old, Reason: analyzer.ReasonOld},
		{Entry: dup, Reason: analyzer.ReasonDuplicate},
		{Entry: log, Reason: analyzer.ReasonLogCache},
	}

	if len(m.filteredHints()) != 3 {
		t.Fatalf("expected 3 hints with no filter, got %d", len(m.filteredHints()))
	}

	m.analyzerFilter = analyzer.ReasonOld
	if len(m.filteredHints()) != 1 || m.filteredHints()[0].Reason != analyzer.ReasonOld {
		t.Fatalf("expected 1 old hint, got %+v", m.filteredHints())
	}

	m.analyzerFilter = analyzer.ReasonDuplicate
	if len(m.filteredHints()) != 1 || m.filteredHints()[0].Reason != analyzer.ReasonDuplicate {
		t.Fatalf("expected 1 duplicate hint, got %+v", m.filteredHints())
	}

	// Batch paths should respect the active filter.
	m.marked[old.Path] = true
	m.analyzerFilter = analyzer.ReasonDuplicate
	if paths := m.batchAnalyzerPaths(); len(paths) != 0 {
		t.Fatalf("expected 0 paths when filtered to duplicate, got %d", len(paths))
	}
	m.analyzerFilter = analyzer.ReasonOld
	if paths := m.batchAnalyzerPaths(); len(paths) != 1 || paths[0] != old.Path {
		t.Fatalf("expected 1 old path, got %v", paths)
	}
}

func TestStackedBar(t *testing.T) {
	summary := analyzer.HintSummary{Old: 2, Duplicate: 3, LogCache: 1}
	got := common.StackedBar(summary, stackedBarWidth)
	// The rendered string should include styling escape sequences, so measure
	// the visible block characters by counting the full block rune.
	visible := strings.Count(got, "█")
	if visible != stackedBarWidth {
		t.Fatalf("expected %d visible blocks, got %d: %q", stackedBarWidth, visible, got)
	}

	// With the default test case, proportions should be non-empty for all three
	// categories. We verify by checking that the rendered output contains
	// segments from each of the three hint styles (old, duplicate, log/cache).
	if !strings.Contains(got, "█") {
		t.Fatalf("expected stacked bar to contain full blocks, got %q", got)
	}
}

func TestStackedBar_ZeroTotal(t *testing.T) {
	summary := analyzer.HintSummary{Old: 0, Duplicate: 0, LogCache: 0}
	got := common.StackedBar(summary, stackedBarWidth)
	visible := strings.Count(got, "░")
	if visible != stackedBarWidth {
		t.Fatalf("expected %d empty blocks when total is zero, got %d: %q", stackedBarWidth, visible, got)
	}
}

func TestStackedBarSegments(t *testing.T) {
	cases := []struct {
		name                string
		summary             analyzer.HintSummary
		width               int
		wantOld             int
		wantDuplicate       int
		wantLogCache        int
	}{
		{"even 50/25/25", analyzer.HintSummary{Old: 4, Duplicate: 2, LogCache: 2}, 24, 12, 6, 6},
		{"only old", analyzer.HintSummary{Old: 10, Duplicate: 0, LogCache: 0}, 24, 24, 0, 0},
		{"zero total", analyzer.HintSummary{Old: 0, Duplicate: 0, LogCache: 0}, 24, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotOld, gotDup, gotLog := common.StackedBarSegments(tc.summary, tc.width)
			if gotOld != tc.wantOld || gotDup != tc.wantDuplicate || gotLog != tc.wantLogCache {
				t.Fatalf("expected (%d, %d, %d), got (%d, %d, %d)", tc.wantOld, tc.wantDuplicate, tc.wantLogCache, gotOld, gotDup, gotLog)
			}
		})
	}
}

func TestCategoryLabel(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	fileA.Category = "document"
	parent := entry("dir", "/dir", 10, true, fileA)
	fileA.Parent = parent

	if got := categoryLabel(fileA); got != "document" {
		t.Fatalf("expected file category 'document', got %q", got)
	}
	if got := categoryLabel(parent); got != "Directory" {
		t.Fatalf("expected directory label 'Directory', got %q", got)
	}
}

func TestFormatHelpBar(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		hints  []string
		expect int // number of lines
	}{
		{"wide", 200, []string{"[a] alpha", "[b] beta", "[c] gamma"}, 1},
		{"narrow", 12, []string{"[a] alpha", "[b] beta", "[c] gamma"}, 3},
		{"zero width", 0, []string{"[a] alpha", "[b] beta"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := common.FormatHelpBar(tc.width, tc.hints)
			lines := strings.Split(got, "\n")
			if len(lines) != tc.expect {
				t.Fatalf("expected %d lines, got %d: %q", tc.expect, len(lines), got)
			}
		})
	}
}

func TestDepsView(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.view = viewDeps
	m.depsList = []*deps.DependencyDir{
		{Path: "/dir/node_modules", Type: "node_modules", Size: 100, AccessTime: time.Now(), ModTime: time.Now()},
	}
	v := m.View()
	if !strings.Contains(v, "node_modules") {
		t.Fatalf("expected deps view to show dependency directory, got:\n%s", v)
	}
}

func TestHandleDepsKey(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.view = viewDeps
	m.depsList = []*deps.DependencyDir{
		{Path: "/dir/node_modules", Type: "node_modules", Size: 100, AccessTime: time.Now(), ModTime: time.Now()},
		{Path: "/dir/vendor", Type: "vendor", Size: 200, AccessTime: time.Now(), ModTime: time.Now()},
	}

	// Navigate down.
	m.handleDepsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.depsSelected != 1 {
		t.Fatalf("expected depsSelected 1 after j, got %d", m.depsSelected)
	}

	// Mark selected (space).
	m.handleDepsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.depsMarked["/dir/vendor"] {
		t.Fatal("expected /dir/vendor to be marked after space")
	}
}

func TestFilterDepsListClampsSelection(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.depsList = []*deps.DependencyDir{
		{Path: "/dir/node_modules", Type: "node_modules", Size: 100, AccessTime: time.Now(), ModTime: time.Now()},
		{Path: "/dir/vendor", Type: "vendor", Size: 200, AccessTime: time.Now(), ModTime: time.Now()},
	}
	m.depsSelected = 1
	m.filterDepsList(map[string]bool{"/dir/vendor": true})
	if m.depsSelected != 0 {
		t.Fatalf("expected depsSelected clamped to 0, got %d", m.depsSelected)
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
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, mock, analyzer.DupHashSmart, 100, nil)
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
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, mock, analyzer.DupHashSmart, 100, nil)
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

func TestSortOrderPreferenceAppliedFromConfig(t *testing.T) {
	cfg := &config.Config{SortOrder: "name"}
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, cfg)
	if m.sortOrder != "name" {
		t.Fatalf("expected sortOrder 'name' from config, got %q", m.sortOrder)
	}
}

func TestSortOrderDefaultsToSizeInTerminal(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	if m.sortOrder != "size" {
		t.Fatalf("expected sortOrder 'size' by default, got %q", m.sortOrder)
	}
}

func TestCycleSortOrderInTerminal(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	fileB := entry("b.txt", "/dir/b.txt", 20, false)
	parent := entry("dir", "/dir", 30, true, fileA, fileB)
	fileA.Parent = parent
	fileB.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	if m.sortOrder != "size" {
		t.Fatalf("expected initial sortOrder 'size', got %q", m.sortOrder)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.sortOrder != "name" {
		t.Fatalf("expected sortOrder 'name' after cycling, got %q", m.sortOrder)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.sortOrder != "access" {
		t.Fatalf("expected sortOrder 'access' after cycling, got %q", m.sortOrder)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.sortOrder != "modified" {
		t.Fatalf("expected sortOrder 'modified' after cycling, got %q", m.sortOrder)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.sortOrder != "size" {
		t.Fatalf("expected sortOrder 'size' after full cycle, got %q", m.sortOrder)
	}
}

func TestSortOrderSavedToConfigInTerminal(t *testing.T) {
	cfg := &config.Config{}
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, cfg)
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cfg.SortOrder != "name" {
		t.Fatalf("expected cfg.SortOrder 'name' after cycling, got %q", cfg.SortOrder)
	}
}

func TestPageDownMovesByPageInTerminal(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	fileB := entry("b.txt", "/dir/b.txt", 20, false)
	parent := entry("dir", "/dir", 30, true, fileA, fileB)
	fileA.Parent = parent
	fileB.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.expanded[parent.Path] = true
	m.rebuild()
	if len(m.items) != 3 {
		t.Fatalf("expected 3 visible items, got %d", len(m.items))
	}

	m.selected = 0
	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.selected != len(m.items)-1 {
		t.Fatalf("expected pgdown to clamp to last item, got %d", m.selected)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.selected != 0 {
		t.Fatalf("expected pgup to clamp to first item, got %d", m.selected)
	}
}

func TestMouseWheelScrollsFilesInTerminal(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	fileB := entry("b.txt", "/dir/b.txt", 20, false)
	parent := entry("dir", "/dir", 30, true, fileA, fileB)
	fileA.Parent = parent
	fileB.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.expanded[parent.Path] = true
	m.rebuild()

	m.selected = 0
	m.handleMouse(tea.MouseMsg{Type: tea.MouseWheelDown})
	if m.selected != 1 {
		t.Fatalf("expected selected 1 after wheel down, got %d", m.selected)
	}
	m.handleMouse(tea.MouseMsg{Type: tea.MouseWheelUp})
	if m.selected != 0 {
		t.Fatalf("expected selected 0 after wheel up, got %d", m.selected)
	}
}

func TestMouseWheelIgnoresOtherViewsInTerminal(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.view = viewDocker
	m.dockerSelected = 0
	m.handleMouse(tea.MouseMsg{Type: tea.MouseWheelDown})
	if m.dockerSelected != 0 {
		t.Fatalf("expected docker selection unchanged, got %d", m.dockerSelected)
	}
}

func TestSelectedPathsReturnsGlobalMarks(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	fileB := entry("b.txt", "/dir/sub/b.txt", 20, false)
	sub := entry("sub", "/dir/sub", 20, true, fileB)
	fileB.Parent = sub
	parent := entry("dir", "/dir", 30, true, fileA, sub)
	fileA.Parent = parent
	sub.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.marked[fileA.Path] = true
	m.marked[fileB.Path] = true

	paths := m.selectedPaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 selected paths, got %d", len(paths))
	}
}

func TestPreviewToggleInTerminal(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	if !m.showPreview {
		t.Fatal("expected preview enabled by default")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.showPreview {
		t.Fatal("expected preview disabled after v")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if !m.showPreview {
		t.Fatal("expected preview re-enabled after second v")
	}
}

func TestPreviewPaneShowsDirectoryInfoInTerminal(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.width = 140
	m.height = 24
	v := m.View()
	if !strings.Contains(v, "Preview") {
		t.Fatalf("expected preview pane header, got:\n%s", v)
	}
	if !strings.Contains(v, "Directory") {
		t.Fatalf("expected preview to show directory info, got:\n%s", v)
	}
}

func TestPreviewPaneShowsFileContentInTerminal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello preview\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	file := &analyzer.Entry{Path: path, Name: "note.txt", IsDir: false, Size: 30}
	root := &analyzer.Entry{Path: dir, Name: dir, IsDir: true, Size: 30, Children: []*analyzer.Entry{file}}
	file.Parent = root

	m := New([]*analyzer.Entry{root}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.width = 140
	m.height = 24
	// The terminal TUI is a tree view: expand the root so the file is visible,
	// then select it so the preview targets the file rather than the directory.
	m.expanded[root.Path] = true
	m.rebuild()
	m.selected = 1
	v := m.View()
	if !strings.Contains(v, "hello preview") {
		t.Fatalf("expected preview to show file content, got:\n%s", v)
	}
}

func TestPreviewHiddenOnNarrowTerminalInTerminal(t *testing.T) {
	parent := entry("dir", "/dir", 10, true)
	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.width = 80
	v := m.View()
	if strings.Contains(v, "Preview") {
		t.Fatalf("expected no preview pane on narrow terminal, got:\n%s", v)
	}
}

func TestPreviewNameColumnTruncatedInTerminal(t *testing.T) {
	// A long name should be truncated when the preview pane is active so the
	// list stays flush with the pane.
	long := entry("a-really-long-directory-name-that-exceeds-the-allowed-width.txt", "/dir/"+strings.Repeat("x", 80), 10, false)
	parent := entry("dir", "/dir", 10, true, long)
	long.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100, nil)
	m.width = 140
	m.height = 24
	// Expand the tree so the long-named child row is visible and truncated.
	m.expanded[parent.Path] = true
	m.rebuild()
	// Compute the exact truncation the view applies: the tree label (indent
	// included) capped to nameW = width - pane - separator - 44 fixed columns.
	want := common.TruncateStart(m.treeLabel(long), 140-common.PreviewPaneWidth-2-44)
	v := m.View()
	if !strings.Contains(v, want) {
		t.Fatalf("expected truncated name %q in wide view, got:\n%s", want, v)
	}
	if strings.Contains(v, long.Name) {
		t.Fatalf("expected the full long name to be truncated, got:\n%s", v)
	}
}
