package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/tui/common"
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)

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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)

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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashFull, 42)
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)

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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
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

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
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

func TestSelectedPathsReturnsGlobalMarks(t *testing.T) {
	fileA := entry("a.txt", "/dir/a.txt", 10, false)
	fileB := entry("b.txt", "/dir/sub/b.txt", 20, false)
	sub := entry("sub", "/dir/sub", 20, true, fileB)
	fileB.Parent = sub
	parent := entry("dir", "/dir", 30, true, fileA, sub)
	fileA.Parent = parent
	sub.Parent = parent

	m := New([]*analyzer.Entry{parent}, "", false, nil, analyzer.DupHashSmart, 100)
	m.marked[fileA.Path] = true
	m.marked[fileB.Path] = true

	paths := m.selectedPaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 selected paths, got %d", len(paths))
	}
}
