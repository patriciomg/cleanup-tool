// Package dua implements a dua-cli-style interactive disk-usage browser.
// It shows a flat list of the current directory's entries sorted by size, with
// keyboard navigation (enter to descend, backspace/u/h to go up, d to mark,
// x to trash marked, ? for help, q to quit).
package dua

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/patriciomg/cleanup-tool/internal/actions"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
)

// Model holds the state of the dua-style TUI.
type Model struct {
	width     int
	height    int
	roots     []*analyzer.Entry
	current   *analyzer.Entry
	items     []*analyzer.Entry
	selected  int
	scanning  bool
	scanStart time.Time
	files     int64
	dirs      int64
	lastPath  string
	spinner   spinner.Model
	msg       string
	err       error
	trashed   map[string]bool
	marked    map[string]bool
	showHelp  bool
}

// scanMsg is sent when the background scan finishes.
type scanMsg struct {
	roots []*analyzer.Entry
	err   error
}

// progressMsg is sent when the scanner reports progress.
type progressMsg struct {
	files int64
	dirs  int64
	path  string
}

// New creates a new dua-style model. If scanning is true the UI starts in the
// scanning state.
func New(scanning bool) *Model {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	m := &Model{
		scanning: scanning,
		spinner:  sp,
		trashed:  make(map[string]bool),
		marked:   make(map[string]bool),
	}
	if scanning {
		m.scanStart = time.Now()
	}
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case progressMsg:
		m.files = msg.files
		m.dirs = msg.dirs
		m.lastPath = msg.path
		return m, nil
	case scanMsg:
		return m.handleScanResult(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case trashMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Trash failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				m.trashed[p] = true
				delete(m.marked, p)
			}
			m.rebuild()
			m.msg = fmt.Sprintf("Moved to Trash: %d items", len(msg.paths))
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(keyMsg)
	}

	return m, nil
}

func (m *Model) handleScanResult(msg scanMsg) (tea.Model, tea.Cmd) {
	m.scanning = false
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.roots = msg.roots
	sortTree(m.roots)
	if len(m.roots) > 0 {
		m.current = m.roots[0]
	}
	m.rebuild()
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		switch msg.String() {
		case "?", "q", "esc", "ctrl+c":
			m.showHelp = false
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.items)-1 {
			m.selected++
		}
	case "right", "enter", "l":
		return m.descend()
	case "left", "backspace", "h", "u", "esc":
		return m.ascend()
	case "d":
		m.toggleMark()
	case "x":
		return m.trashMarked()
	case "c":
		m.clearMarks()
	case "?":
		m.showHelp = true
	}
	return m, nil
}

func (m *Model) descend() (tea.Model, tea.Cmd) {
	item := m.selectedItem()
	if item == nil || !item.IsDir {
		return m, nil
	}
	m.current = item
	m.rebuild()
	m.selected = 0
	return m, nil
}

func (m *Model) ascend() (tea.Model, tea.Cmd) {
	if m.current == nil || m.current.Parent == nil {
		return m, nil
	}
	child := m.current
	m.current = m.current.Parent
	m.rebuild()
	// Try to keep the cursor on the directory we just came from.
	for i, it := range m.items {
		if it.Path == child.Path {
			m.selected = i
			return m, nil
		}
	}
	m.selected = 0
	return m, nil
}

func (m *Model) toggleMark() {
	item := m.selectedItem()
	if item == nil {
		return
	}
	m.marked[item.Path] = !m.marked[item.Path]
}

func (m *Model) trashMarked() (tea.Model, tea.Cmd) {
	paths := m.markedPaths()
	if len(paths) == 0 {
		item := m.selectedItem()
		if item == nil {
			return m, nil
		}
		paths = []string{item.Path}
	}
	return m, func() tea.Msg {
		if err := actions.Trash(paths...); err != nil {
			return trashMsg{paths: paths, err: err}
		}
		return trashMsg{paths: paths}
	}
}

type trashMsg struct {
	paths []string
	err   error
}

func (m *Model) clearMarks() {
	m.marked = make(map[string]bool)
}

func (m *Model) selectedItem() *analyzer.Entry {
	if m.selected < 0 || m.selected >= len(m.items) {
		return nil
	}
	return m.items[m.selected]
}

func (m *Model) markedPaths() []string {
	var paths []string
	for p, ok := range m.marked {
		if ok {
			paths = append(paths, p)
		}
	}
	return paths
}

func (m *Model) rebuild() {
	m.items = nil
	if m.current == nil {
		m.selected = 0
		return
	}
	m.items = sortedVisible(m.current.Children, m.trashed)
	if m.selected >= len(m.items) {
		if len(m.items) == 0 {
			m.selected = 0
		} else {
			m.selected = len(m.items) - 1
		}
	}
}

// sortedVisible returns children that have not been trashed, sorted by size.
func sortedVisible(children []*analyzer.Entry, trashed map[string]bool) []*analyzer.Entry {
	var out []*analyzer.Entry
	for _, c := range children {
		if !trashed[c.Path] {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Size > out[j].Size
	})
	return out
}

func sortTree(entries []*analyzer.Entry) {
	for _, e := range entries {
		if e.IsDir && len(e.Children) > 0 {
			sort.Slice(e.Children, func(i, j int) bool {
				return e.Children[i].Size > e.Children[j].Size
			})
			sortTree(e.Children)
		}
	}
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.err != nil {
		return dangerStyle.Render("Error: "+m.err.Error()) + "\nq to quit\n"
	}
	if m.scanning {
		return m.scanView()
	}
	if m.showHelp {
		return m.helpView()
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Dua-style Browser"))
	if m.current != nil {
		b.WriteString(fmt.Sprintf("  %s  total %s\n", currentPathStyle.Render(truncate(m.current.Path, 60)), analyzer.PrettySize(m.current.Size)))
	}
	if m.msg != "" {
		b.WriteString(msgStyle.Render(m.msg) + "\n")
	}
	b.WriteString("\n")

	if len(m.items) == 0 {
		b.WriteString("No items.\n")
		b.WriteString(formatHelpBar(m.width, []string{"[?] help", "[q] quit"}))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("%-12s %6s %-12s %s\n", "Size", "Pct", "Bar", "Name"))

	maxSize := m.items[0].Size
	start, end := m.visibleRange(len(m.items))
	for i := start; i < end; i++ {
		item := m.items[i]
		pct := percent(item.Size, m.current.Size)
		line := fmt.Sprintf("%-12s %5s%% %-12s %s",
			analyzer.PrettySize(item.Size),
			pct,
			bar(item.Size, maxSize, 12),
			label(item),
		)
		if m.trashed[item.Path] {
			line = trashedStyle.Render(line)
		} else if m.marked[item.Path] {
			line = markedStyle.Render(line)
		} else if i == m.selected {
			line = selectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	hints := []string{
		"[j/k/↓/↑] nav", "[enter/l] descend", "[backspace/h/u] up",
		"[d] mark", "[x] trash marked", "[c] clear", "[?] help", "[q] quit",
	}
	b.WriteString("\n" + formatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) scanView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Dua-style Browser — scanning..."))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " ")
	b.WriteString(fmt.Sprintf("Files: %d  Dirs: %d\n", m.files, m.dirs))
	if !m.scanStart.IsZero() {
		secs := time.Since(m.scanStart).Seconds()
		if secs > 0 {
			b.WriteString(fmt.Sprintf("Speed: %.0f files/sec  %.0f dirs/sec\n", float64(m.files)/secs, float64(m.dirs)/secs))
		}
	}
	b.WriteString(fmt.Sprintf("Last: %s\n", truncate(m.lastPath, 60)))
	b.WriteString("\n" + formatHelpBar(m.width, []string{"[q] quit"}) + "\n")
	return b.String()
}

func (m *Model) helpView() string {
	lines := []string{
		"",
		"Dua-style browser key bindings",
		"",
		"  j/k or ↓/↑   navigate items",
		"  enter/l      descend into directory",
		"  backspace/h/u/esc  go to parent directory",
		"  d            mark/unmark selected item",
		"  x            trash marked items (or selected if none marked)",
		"  c            clear all marks",
		"  ?            toggle this help",
		"  q            quit",
		"",
	}
	boxWidth := 60
	if m.width > 0 && m.width < boxWidth {
		boxWidth = m.width
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(center(l, boxWidth) + "\n")
	}
	return helpBoxStyle.Width(boxWidth).Render(b.String())
}

func (m *Model) visibleRange(n int) (int, int) {
	height := m.height - 6
	if height < 8 {
		height = 8
	}
	start := m.selected - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > n {
		end = n
	}
	return start, end
}

func label(item *analyzer.Entry) string {
	if item.IsDir {
		return item.Name + "/"
	}
	return item.Name
}

func percent(part, total int64) string {
	if total <= 0 {
		return "0.0"
	}
	p := float64(part) * 100 / float64(total)
	if p >= 99.95 {
		return "100"
	}
	return fmt.Sprintf("%.1f", p)
}

func bar(size, max int64, width int) string {
	if max <= 0 || width <= 0 {
		return ""
	}
	w := int(math.Round(float64(size) / float64(max) * float64(width)))
	if w < 0 {
		w = 0
	}
	if w > width {
		w = width
	}
	return barStyle.Render(strings.Repeat("█", w)) + strings.Repeat("░", width-w)
}

func center(s string, width int) string {
	if len(s) >= width {
		return s
	}
	pad := (width - len(s)) / 2
	return strings.Repeat(" ", pad) + s
}

func formatHelpBar(width int, hints []string) string {
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return "..."
	}
	return "..." + s[len(s)-(n-3):]
}

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	selectStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#3b4261"))
	dangerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	markedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	trashedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Strikethrough(true)
	currentPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	msgStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	barStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dcfff"))
	helpBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

// RunWithScan starts the dua-style TUI and scans the given paths in the
// background. progressInterval controls how often scan progress is reported
// (a value <= 0 disables progress reports).
func RunWithScan(paths []string, ignore []string, ignoreHidden bool, progressInterval int) error {
	ctx, cancel := context.WithCancel(context.Background())
	p := tea.NewProgram(New(true))
	go func() {
		defer cancel()
		scanner := analyzer.NewScanner(ignore, ignoreHidden, progressInterval)
		scanner.OnProgress = func(pr analyzer.Progress) {
			go p.Send(progressMsg{files: pr.Files, dirs: pr.Dirs, path: pr.Path})
		}
		roots, err := scanner.Scan(ctx, paths)
		if ctx.Err() != nil {
			return
		}
		p.Send(scanMsg{roots: roots, err: err})
	}()
	if _, err := p.Run(); err != nil {
		cancel()
		return err
	}
	return nil
}
