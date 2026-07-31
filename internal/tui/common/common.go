// Package common holds shared helpers, styles, and UI widgets used by the
// multiple TUI implementations in internal/tui.
package common

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
)

// Preview pane constants shared by the TUI implementations.
const (
	// PreviewMinWidth is the minimum terminal width at which the file
	// preview pane is shown automatically.
	PreviewMinWidth = 110
	// PreviewPaneWidth is the width of the file preview pane.
	PreviewPaneWidth = 44
	// PreviewMaxBytes caps how much of a file is read for the preview pane.
	PreviewMaxBytes = 32 * 1024
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

// TruncateStart shortens s to at most n runes, keeping the beginning of the
// string. Unlike Truncate (which keeps the end for path readability), this is
// used for file names and preview content where the start matters.
func TruncateStart(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return "..."
	}
	return string(r[:n-3]) + "..."
}

// FilePreview returns the file's text lines (up to PreviewMaxBytes) and
// whether the content was cut short. Binary, empty, and unreadable files
// produce a short notice line instead.
func FilePreview(path string) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return []string{"(cannot read)"}, false
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, PreviewMaxBytes))
	if err != nil {
		return []string{"(cannot read)"}, false
	}
	if len(data) == 0 {
		return []string{"(empty file)"}, false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return []string{"(binary file)"}, false
	}
	return strings.Split(string(data), "\n"), len(data) >= PreviewMaxBytes
}

// PreviewPane renders a side pane with details and, for text files, a content
// preview of the given item. It is only rendered when the terminal is wide
// enough for the PreviewMinWidth threshold. maxLines caps the pane height so
// it never overflows the list it sits beside.
func PreviewPane(item *analyzer.Entry, maxLines int) string {
	if item == nil {
		return ""
	}

	var lines []string
	lines = append(lines, "Preview")
	lines = append(lines, "")

	if item.IsDir {
		lines = append(lines, "Directory")
		lines = append(lines, "Size: "+analyzer.PrettySize(item.Size))
		lines = append(lines, fmt.Sprintf("Items: %d", len(item.Children)))
		lines = append(lines, "Path: "+item.Path)
	} else {
		lines = append(lines, "File")
		lines = append(lines, "Size: "+analyzer.PrettySize(item.Size))
		if !item.ModTime.IsZero() {
			lines = append(lines, "Modified: "+item.ModTime.Format("2006-01-02 15:04"))
		}
		lines = append(lines, "Path: "+item.Path)
		lines = append(lines, "")
		content, truncated := FilePreview(item.Path)
		lines = append(lines, content...)
		if truncated {
			lines = append(lines, "...")
		}
	}

	// Cap the whole pane to maxLines so it never overflows the view, keeping a
	// trailing ellipsis to indicate more content.
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines-1], "...")
	}

	// The border is drawn outside the width in lipgloss, so cap the content
	// width to leave room for it.
	pw := PreviewPaneWidth - 2
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		s := TruncateStart(l, pw)
		// Style the header after truncation so rune slicing never corrupts
		// ANSI escape codes.
		if i == 0 {
			s = HeaderStyle.Render(s)
		}
		out = append(out, s)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(pw).
		Render(strings.Join(out, "\n"))
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
