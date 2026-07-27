package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
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
	cats := m.summaryCategories(summary)

	var x int
	for _, cat := range cats {
		s := cat.String()
		// The clickable region covers the category text, a space, and the bar.
		w := len(s) + 1 + barChartWidth
		// Click in the middle of the bar area to verify the expanded hit target.
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

func TestCategoryBar(t *testing.T) {
	cases := []struct {
		n, max, width int
		expectFilled  int
	}{
		{0, 10, 4, 0},
		{1, 10, 4, 1},
		{5, 10, 4, 2},
		{10, 10, 4, 4},
		{20, 10, 4, 4},
		{5, 0, 4, 0},
		{5, 10, 0, 0},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d_of_%d_width_%d", tc.n, tc.max, tc.width), func(t *testing.T) {
			got := categoryBar(tc.n, tc.max, tc.width)
			if utf8.RuneCountInString(got) != tc.width {
				t.Fatalf("expected width %d, got %d: %q", tc.width, utf8.RuneCountInString(got), got)
			}
			filled := strings.Count(got, "█")
			if filled != tc.expectFilled {
				t.Fatalf("expected %d filled blocks, got %d: %q", tc.expectFilled, filled, got)
			}
		})
	}
}

func TestCategoryBar_NonPositiveWidth(t *testing.T) {
	for _, width := range []int{0, -1, -42} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			got := categoryBar(5, 10, width)
			if got != "" {
				t.Fatalf("expected empty string for non-positive width, got %q", got)
			}
		})
	}
}

func TestSparkline(t *testing.T) {
	summary := analyzer.HintSummary{Old: 5, Duplicate: 10, LogCache: 0}
	got := sparkline(summary, 10, barChartWidth)
	parts := strings.Split(got, " ")
	if len(parts) != 3 {
		t.Fatalf("expected 3 bars, got %d: %q", len(parts), got)
	}
	if strings.Count(parts[0], "█") != 2 {
		t.Fatalf("expected old bar width 2, got %q", parts[0])
	}
	if strings.Count(parts[1], "█") != 4 {
		t.Fatalf("expected duplicate bar width 4, got %q", parts[1])
	}
	if strings.Count(parts[2], "█") != 0 {
		t.Fatalf("expected log/cache bar empty, got %q", parts[2])
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
			got := formatHelpBar(tc.width, tc.hints)
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
