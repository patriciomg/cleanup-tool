// Package common holds shared helpers, styles, and UI widgets used by the
// multiple TUI implementations in internal/tui.
package common

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
)

// Shared lipgloss styles used by the TUIs.
var (
	HeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	SelectStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#3b4261"))
	DangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	TrashedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Strikethrough(true)
	MarkedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	HintOldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	HintDupStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dcfff"))
	HintLogStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9e64"))
	SummaryStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Bold(true).Underline(true)
	FilterStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68")).Bold(true).Underline(true)
	BarStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	SizeStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	MsgStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	CurrentPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	HelpBoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

// FormatHelpBar lays out keybinding hints across as many lines as needed so
// that each line fits within the given width. If width is unknown (<=0), the
// hints are joined on a single line.
func FormatHelpBar(width int, hints []string) string {
	if width <= 0 {
		return strings.Join(hints, "  ")
	}
	var lines []string
	var current string
	for _, hint := range hints {
		if current == "" {
			current = hint
			continue
		}
		if len(current)+2+len(hint) <= width {
			current += "  " + hint
		} else {
			lines = append(lines, current)
			current = hint
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

// Truncate shortens s to at most n runes, inserting an ellipsis when needed.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return "..."
	}
	return "..." + s[len(s)-(n-3):]
}

// Pluralize returns the singular or plural form based on n.
func Pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// SortTree recursively sorts every directory's children according to order.
// Supported orders values are "size" (default), "name", "access", and
// "modified".
func SortTree(entries []*analyzer.Entry, order string) {
	for _, e := range entries {
		if e.IsDir && len(e.Children) > 0 {
			switch order {
			case "name":
				sort.Slice(e.Children, func(i, j int) bool {
					return strings.ToLower(e.Children[i].Name) < strings.ToLower(e.Children[j].Name)
				})
			case "access":
				sort.Slice(e.Children, func(i, j int) bool {
					return e.Children[i].AccessTime.After(e.Children[j].AccessTime)
				})
			case "modified":
				sort.Slice(e.Children, func(i, j int) bool {
					return e.Children[i].ModTime.After(e.Children[j].ModTime)
				})
			default:
				sort.Slice(e.Children, func(i, j int) bool {
					return e.Children[i].Size > e.Children[j].Size
				})
			}
			SortTree(e.Children, order)
		}
	}
}

// SummaryCategory describes one deletability summary bucket.
type SummaryCategory struct {
	Value  int
	Reason analyzer.HintReason
	Label  string
}

func (sc SummaryCategory) String() string {
	return fmt.Sprintf("%d %s", sc.Value, sc.Label)
}

// Capitalize returns s with its first Unicode character upper-cased. It is a
// tiny helper shared by the TUI packages so they do not duplicate formatting
// utilities.
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// SummaryCategories returns the three deletability summary buckets.
func SummaryCategories(summary analyzer.HintSummary) []SummaryCategory {
	return []SummaryCategory{
		{Value: summary.Old, Reason: analyzer.ReasonOld, Label: Pluralize(summary.Old, "old file", "old files")},
		{Value: summary.Duplicate, Reason: analyzer.ReasonDuplicate, Label: Pluralize(summary.Duplicate, "duplicate", "duplicates")},
		{Value: summary.LogCache, Reason: analyzer.ReasonLogCache, Label: "log/cache"},
	}
}

// StackedBarSegments returns the widths of the old, duplicate, and log/cache
// segments for a stacked bar of the given total width.
func StackedBarSegments(summary analyzer.HintSummary, width int) (int, int, int) {
	total := summary.Old + summary.Duplicate + summary.LogCache
	if total == 0 {
		return 0, 0, 0
	}

	wOld := int(math.Round(float64(summary.Old) / float64(total) * float64(width)))
	wDup := int(math.Round(float64(summary.Duplicate) / float64(total) * float64(width)))
	wLog := width - wOld - wDup

	// Guard against rounding errors pushing any segment negative.
	if wLog < 0 {
		wDup += wLog
		wLog = 0
	}

	return wOld, wDup, wLog
}

// StackedBar renders a single stacked bar where each segment is proportional
// to the count of hints in that category.
func StackedBar(summary analyzer.HintSummary, width int) string {
	if summary.Old+summary.Duplicate+summary.LogCache == 0 {
		return BarStyle.Render(strings.Repeat("░", width))
	}
	wOld, wDup, wLog := StackedBarSegments(summary, width)
	return HintOldStyle.Render(strings.Repeat("█", wOld)) +
		HintDupStyle.Render(strings.Repeat("█", wDup)) +
		HintLogStyle.Render(strings.Repeat("█", wLog))
}
