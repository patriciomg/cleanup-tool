package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/patriciomg/cleanup-tool/internal/actions"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/docker"
)

type viewState int

const (
	viewFiles viewState = iota
	viewDocker
	viewDockerConfirm
	viewAnalyzer

	// analyzerSummaryLineY is the 0-based Y position of the interactive
	// analyzer summary line (header + blank + "Found X hints" + blank).
	// WARNING: this must stay in sync with analyzerView() output.
	analyzerSummaryLineY = 4

	// analyzerStackedBarLineY is the 0-based Y position of the stacked bar
	// rendered directly below the summary text.
	// WARNING: this must stay in sync with analyzerView() output.
	analyzerStackedBarLineY = analyzerSummaryLineY + 1

	// stackedBarWidth is the total width of the stacked summary bar.
	stackedBarWidth = 24
)

type Model struct {
	width  int
	height int

	roots          []*analyzer.Entry
	currentDir     *analyzer.Entry
	items          []*analyzer.Entry
	selected       int
	err            error
	msg            string
	externalDir    string
	scanning       bool
	files          int64
	dirs           int64
	lastPath       string
	spinner        spinner.Model
	view           viewState
	dockerClient   docker.Client
	dockerUsage    *docker.Usage
	dockerSelected int
	dockerErr      error
	dockerMsg      string
	marked           map[string]bool
	trashed          map[string]bool
	expanded         map[string]bool
	scanStart        time.Time
	scanDuration     time.Duration
	peakFilesPerSec  float64
	peakDirsPerSec   float64
	hints            []*analyzer.DeletabilityHint
	analyzerFilter   analyzer.HintReason
	analyzerRunning  bool
	analyzerCancel   context.CancelFunc
	analyzerProg     analyzer.AnalyzerProgress
	analyzerProgCh   chan analyzer.AnalyzerProgress
	analyzerDoneCh   chan analyzerMsg
	dupMode          analyzer.DupHashMode
	progressInterval int
}

type ScanMsg struct {
	Roots []*analyzer.Entry
	Err   error
}

type progressMsg struct {
	files int64
	dirs  int64
	path  string
}

type dockerUsageMsg struct {
	usage *docker.Usage
	err   error
}

type dockerPruneMsg struct {
	reclaimed int64
	err       error
}

type analyzerMsg struct {
	hints []*analyzer.DeletabilityHint
	err   error
}

type analyzerProgressMsg analyzer.AnalyzerProgress

type trashMsg struct {
	paths []string
	err   error
}

type moveMsg struct {
	paths []string
	err   error
}

type restoreMsg struct {
	paths []string
	err   error
}

func New(roots []*analyzer.Entry, externalDir string, scanning bool, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int) *Model {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	m := &Model{
		roots:            roots,
		externalDir:      externalDir,
		spinner:          sp,
		scanning:         scanning,
		dockerClient:     dockerClient,
		marked:           make(map[string]bool),
		trashed:          make(map[string]bool),
		expanded:         make(map[string]bool),
		dupMode:          dupMode,
		progressInterval: progressInterval,
	}
	if len(roots) > 0 {
		m.currentDir = roots[0]
	}
	m.sortTree(m.roots)
	m.rebuild()
	if scanning {
		m.scanStart = time.Now()
		m.peakFilesPerSec = 0
		m.peakDirsPerSec = 0
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(keyMsg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case ScanMsg:
		m.scanning = false
		if !m.scanStart.IsZero() {
			m.scanDuration = time.Since(m.scanStart)
			m.updatePeakThroughput(m.scanDuration)
		}
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.roots = msg.Roots
			m.sortTree(m.roots)
			if len(m.roots) > 0 {
				m.currentDir = m.roots[0]
			}
			m.rebuild()
		}
	case progressMsg:
		m.files = msg.files
		m.dirs = msg.dirs
		m.lastPath = msg.path
		m.updatePeakThroughput(time.Since(m.scanStart))
	case dockerUsageMsg:
		m.scanning = false
		if msg.err != nil {
			m.dockerErr = msg.err
		} else {
			m.dockerUsage = msg.usage
			m.dockerErr = nil
		}
	case dockerPruneMsg:
		if msg.err != nil {
			m.dockerErr = msg.err
			m.dockerMsg = "Prune failed: " + msg.err.Error()
		} else {
			m.dockerMsg = fmt.Sprintf("Reclaimed %s", analyzer.PrettySize(msg.reclaimed))
		}
		return m, m.fetchDockerUsage
	case analyzerProgressMsg:
		m.analyzerProg = analyzer.AnalyzerProgress(msg)
		return m, m.readAnalyzerUpdate
	case analyzerMsg:
		m.analyzerRunning = false
		m.analyzerCancel = nil
		m.analyzerProg = analyzer.AnalyzerProgress{}
		if msg.err != nil {
			if msg.err == context.Canceled {
				m.msg = "Analysis cancelled"
			} else {
				m.err = msg.err
			}
		} else {
			m.hints = msg.hints
			m.analyzerFilter = ""
			m.selected = 0
			m.msg = fmt.Sprintf("Analyzer found %d hints", len(msg.hints))
		}
	case trashMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Trash failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				m.trashed[p] = true
				delete(m.marked, p)
			}
			m.msg = fmt.Sprintf("Moved to Trash: %d items", len(msg.paths))
		}
	case moveMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Move failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				m.trashed[p] = true
				delete(m.marked, p)
			}
			m.msg = fmt.Sprintf("Moved to external: %d items", len(msg.paths))
		}
	case restoreMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Restore failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				delete(m.trashed, p)
			}
			m.msg = fmt.Sprintf("Restored: %d items", len(msg.paths))
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.view != viewAnalyzer || m.analyzerRunning || len(m.hints) == 0 {
		return m, nil
	}
	if msg.Type != tea.MouseLeft {
		return m, nil
	}

	summary := analyzer.SummarizeHints(m.hints)
	switch msg.Y {
	case analyzerSummaryLineY:
		m.handleSummaryTextClick(summary, msg.X)
	case analyzerStackedBarLineY:
		m.handleStackedBarClick(summary, msg.X)
	}
	return m, nil
}

func (m *Model) handleSummaryTextClick(summary analyzer.HintSummary, x int) {
	cats := m.summaryCategories(summary)
	var pos int
	for _, cat := range cats {
		s := cat.String()
		w := len(s)
		if x >= pos && x < pos+w {
			m.toggleFilter(cat.Reason)
			return
		}
		pos += w + 2
	}
}

func (m *Model) handleStackedBarClick(summary analyzer.HintSummary, x int) {
	reason, ok := m.categoryAtX(summary, x)
	if ok {
		m.toggleFilter(reason)
	}
}

// categoryAtX maps an X coordinate within the stacked bar to the corresponding
// deletability category. The bar is indented by 2 spaces.
func (m *Model) categoryAtX(summary analyzer.HintSummary, x int) (analyzer.HintReason, bool) {
	x -= 2
	if x < 0 || x >= stackedBarWidth {
		return "", false
	}
	wOld, wDup, _ := stackedBarSegments(summary, stackedBarWidth)
	switch {
	case x < wOld:
		return analyzer.ReasonOld, true
	case x < wOld+wDup:
		return analyzer.ReasonDuplicate, true
	default:
		return analyzer.ReasonLogCache, true
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewDockerConfirm:
		switch msg.String() {
		case "y":
			m.view = viewDocker
			return m, m.pruneDockerSelected
		case "n", "esc":
			m.view = viewDocker
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	case viewDocker:
		return m.handleDockerKey(msg)
	case viewAnalyzer:
		return m.handleAnalyzerKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		if m.analyzerRunning && m.analyzerCancel != nil {
			m.analyzerCancel()
			m.analyzerCancel = nil
		}
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
		if m.selected < len(m.items) && m.items[m.selected].IsDir {
			m.toggleExpanded(m.items[m.selected].Path)
			m.rebuild()
		}
	case "left", "backspace", "h", "esc":
		if m.selected >= len(m.items) {
			break
		}
		item := m.items[m.selected]
		if item.IsDir && m.expanded[item.Path] {
			m.toggleExpanded(item.Path)
			m.rebuild()
			break
		}
		if item.Parent != nil {
			// Move selection to the parent entry.
			for i, it := range m.items {
				if it.Path == item.Parent.Path {
					m.selected = i
					break
				}
			}
		}
	case " ":
		if m.selected < len(m.items) {
			item := m.items[m.selected]
			m.marked[item.Path] = !m.marked[item.Path]
		}
	case "c":
		m.clearMarks()
	case "d":
		return m, m.trashSelected()
	case "m":
		return m, m.moveSelected()
	case "u":
		return m, m.restoreSelected()
	case "D":
		m.view = viewDocker
		return m, m.fetchDockerUsage
	case "a":
		m.view = viewAnalyzer
		m.analyzerRunning = true
		return m, m.runAnalyzer()
	case "A":
		m.view = viewAnalyzer
		m.analyzerRunning = true
		return m, m.runAnalyzerOnSelection()
	}
	return m, nil
}

func (m *Model) handleDockerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = viewFiles
		return m, nil
	case "up", "k":
		if m.dockerSelected > 0 {
			m.dockerSelected--
		}
	case "down", "j":
		if m.dockerSelected < 3 {
			m.dockerSelected++
		}
	case "p":
		m.view = viewDockerConfirm
	case "r":
		return m, m.fetchDockerUsage
	}
	return m, nil
}

func (m *Model) handleAnalyzerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.analyzerRunning {
		switch msg.String() {
		case "q", "ctrl+c":
			if m.analyzerCancel != nil {
				m.analyzerCancel()
				m.analyzerCancel = nil
			}
			return m, tea.Quit
		case "esc":
			if m.analyzerCancel != nil {
				m.analyzerCancel()
				m.analyzerCancel = nil
			}
			m.analyzerRunning = false
			m.view = viewFiles
			m.msg = "Analysis cancelled"
			return m, nil
		}
		return m, nil
	}

	filtered := m.filteredHints()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = viewFiles
		return m, nil
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(filtered)-1 {
			m.selected++
		}
	case "left", "shift+tab":
		m.cycleFilter(-1)
	case "right", "tab":
		m.cycleFilter(1)
	case "0":
		m.analyzerFilter = ""
		m.selected = 0
	case "c":
		m.clearMarks()
	case " ":
		if m.selected < len(filtered) {
			item := filtered[m.selected].Entry
			m.marked[item.Path] = !m.marked[item.Path]
		}
	case "d":
		paths := m.batchAnalyzerPaths()
		if len(paths) > 0 {
			return m, m.trashPaths(paths)
		}
	}
	return m, nil
}

func (m *Model) selectedPaths() []string {
	paths := m.markedPaths()
	if len(paths) == 0 && m.selected < len(m.items) {
		paths = []string{m.items[m.selected].Path}
	}
	return paths
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

func (m *Model) toggleFilter(reason analyzer.HintReason) {
	if m.analyzerFilter == reason {
		m.analyzerFilter = ""
	} else {
		m.analyzerFilter = reason
	}
	m.selected = 0
}

func (m *Model) cycleFilter(dir int) {
	filters := []analyzer.HintReason{"", analyzer.ReasonOld, analyzer.ReasonDuplicate, analyzer.ReasonLogCache}
	idx := 0
	for i, f := range filters {
		if f == m.analyzerFilter {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(filters)) % len(filters)
	m.analyzerFilter = filters[idx]
	m.selected = 0
}

func (m *Model) filteredHints() []*analyzer.DeletabilityHint {
	if m.analyzerFilter == "" {
		return m.hints
	}
	var res []*analyzer.DeletabilityHint
	for _, h := range m.hints {
		if h.Reason == m.analyzerFilter {
			res = append(res, h)
		}
	}
	return res
}

func (m *Model) batchAnalyzerPaths() []string {
	marked := make(map[string]bool)
	for _, p := range m.markedPaths() {
		marked[p] = true
	}
	var result []string
	for _, h := range m.filteredHints() {
		if marked[h.Entry.Path] {
			result = append(result, h.Entry.Path)
		}
	}
	return result
}

func (m *Model) trashSelected() tea.Cmd {
	paths := m.selectedPaths()
	if len(paths) == 0 {
		return nil
	}
	return m.trashPaths(paths)
}

func (m *Model) trashPaths(paths []string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.Trash(paths...); err != nil {
			return trashMsg{paths: paths, err: err}
		}
		return trashMsg{paths: paths}
	}
}

func (m *Model) moveSelected() tea.Cmd {
	if m.externalDir == "" {
		return func() tea.Msg { return moveMsg{err: fmt.Errorf("no external dir set")} }
	}
	paths := m.selectedPaths()
	if len(paths) == 0 {
		return nil
	}
	return m.movePaths(paths)
}

func (m *Model) movePaths(paths []string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.MoveToExternal(m.externalDir, paths...); err != nil {
			return moveMsg{paths: paths, err: err}
		}
		return moveMsg{paths: paths}
	}
}

func (m *Model) restoreSelected() tea.Cmd {
	if m.selected >= len(m.items) {
		return nil
	}
	item := m.items[m.selected]
	return func() tea.Msg {
		if err := actions.Restore(filepath.Join(os.Getenv("HOME"), ".Trash"), item.Path); err != nil {
			return restoreMsg{paths: []string{item.Path}, err: err}
		}
		return restoreMsg{paths: []string{item.Path}}
	}
}

func (m *Model) runAnalyzer() tea.Cmd {
	item := m.selectedEntry()
	if item == nil {
		return func() tea.Msg { return analyzerMsg{err: fmt.Errorf("no directory selected")} }
	}
	if !item.IsDir {
		return func() tea.Msg { return analyzerMsg{err: fmt.Errorf("selected item is not a directory")} }
	}
	return m.analyzeRoot(item)
}

func (m *Model) runAnalyzerOnSelection() tea.Cmd {
	paths := m.selectedPaths()
	if len(paths) == 0 {
		return func() tea.Msg { return analyzerMsg{err: fmt.Errorf("no items selected")} }
	}
	root := &analyzer.Entry{Path: "selection", Name: "selection", IsDir: true}
	for _, p := range paths {
		e := m.findEntryByPath(p)
		if e != nil {
			root.Children = append(root.Children, e)
		}
	}
	if len(root.Children) == 0 {
		return func() tea.Msg { return analyzerMsg{err: fmt.Errorf("no items selected")} }
	}
	return m.analyzeRoot(root)
}

func (m *Model) analyzeRoot(root *analyzer.Entry) tea.Cmd {
	m.analyzerProgCh = make(chan analyzer.AnalyzerProgress, 1)
	m.analyzerDoneCh = make(chan analyzerMsg, 1)
	m.analyzerProg = analyzer.AnalyzerProgress{Stage: "starting"}

	ctx, cancel := context.WithCancel(context.Background())
	m.analyzerCancel = cancel

	go func() {
		opts := analyzer.HintOptions{
			DupMode:          m.dupMode,
			ProgressInterval: m.progressInterval,
			OnProgress: func(p analyzer.AnalyzerProgress) {
				select {
				case m.analyzerProgCh <- p:
				default:
				}
			},
		}
		hints, err := analyzer.FindHintsWithOptions(ctx, root, opts)
		m.analyzerDoneCh <- analyzerMsg{hints: hints, err: err}
	}()

	return m.readAnalyzerUpdate
}

func (m *Model) readAnalyzerUpdate() tea.Msg {
	if m.analyzerDoneCh == nil || m.analyzerProgCh == nil {
		return nil
	}
	// Drain any pending progress before checking completion, so the UI shows
	// the most recent state before the final result.
	select {
	case p := <-m.analyzerProgCh:
		return analyzerProgressMsg(p)
	default:
	}
	select {
	case p := <-m.analyzerProgCh:
		return analyzerProgressMsg(p)
	case msg := <-m.analyzerDoneCh:
		return msg
	}
}

func (m *Model) rebuild() {
	var previousPath string
	if m.selected >= 0 && m.selected < len(m.items) {
		previousPath = m.items[m.selected].Path
	}
	m.items = nil
	if len(m.roots) == 0 {
		m.selected = 0
		return
	}
	m.items = m.visibleTreeItems(m.roots)
	// Restore the selection index from the previously selected path. If the
	// path is no longer visible (e.g., it was inside a collapsed directory),
	// fall back to its nearest visible ancestor.
	m.selected = m.indexOfVisibleOrAncestor(previousPath)
}

// indexOfVisibleOrAncestor returns the index of the entry whose path matches
// previousPath, or the nearest visible ancestor of that entry.
func (m *Model) indexOfVisibleOrAncestor(previousPath string) int {
	if previousPath == "" {
		return 0
	}
	for i, item := range m.items {
		if item.Path == previousPath {
			return i
		}
	}
	// Find the entry in the full tree and walk up to the first visible ancestor.
	entry := m.findEntryByPath(previousPath)
	if entry == nil {
		return 0
	}
	for p := entry.Parent; p != nil; p = p.Parent {
		for i, item := range m.items {
			if item.Path == p.Path {
				return i
			}
		}
	}
	return 0
}

// visibleTreeItems returns a flat list of all entries that are visible given
// the current expansion state. Roots are always shown; a directory's children
// are included only when the directory is expanded.
func (m *Model) visibleTreeItems(roots []*analyzer.Entry) []*analyzer.Entry {
	var items []*analyzer.Entry
	for _, root := range roots {
		m.appendVisible(root, &items)
	}
	return items
}

func (m *Model) appendVisible(e *analyzer.Entry, items *[]*analyzer.Entry) {
	*items = append(*items, e)
	if !e.IsDir || !m.expanded[e.Path] {
		return
	}
	for _, child := range e.Children {
		m.appendVisible(child, items)
	}
}

// sortTree recursively sorts every directory's children by descending size.
func (m *Model) sortTree(entries []*analyzer.Entry) {
	for _, e := range entries {
		if e.IsDir && len(e.Children) > 0 {
			sort.Slice(e.Children, func(i, j int) bool {
				return e.Children[i].Size > e.Children[j].Size
			})
			m.sortTree(e.Children)
		}
	}
}

func (m *Model) toggleExpanded(path string) {
	m.expanded[path] = !m.expanded[path]
}

// selectedEntry returns the currently selected entry, or nil if none.
func (m *Model) selectedEntry() *analyzer.Entry {
	if m.selected < 0 || m.selected >= len(m.items) {
		return nil
	}
	return m.items[m.selected]
}

// totalRootsSize returns the combined size of all root entries.
func (m *Model) totalRootsSize() int64 {
	var total int64
	for _, r := range m.roots {
		total += r.Size
	}
	return total
}

// treeLabel renders the indented name of an entry with an expand/collapse
// indicator for directories.
func (m *Model) treeLabel(item *analyzer.Entry) string {
	indent := strings.Repeat("  ", item.Depth())
	if !item.IsDir {
		return indent + "  " + item.Name
	}
	indicator := " "
	if len(item.Children) > 0 {
		if m.expanded[item.Path] {
			indicator = "▼"
		} else {
			indicator = "▶"
		}
	}
	return indent + indicator + " " + item.Name
}

// findEntryByPath searches all roots for an entry with the given path.
func (m *Model) findEntryByPath(path string) *analyzer.Entry {
	for _, root := range m.roots {
		if e := analyzer.FindEntryByPath(root, path); e != nil {
			return e
		}
	}
	return nil
}

func (m *Model) clearMarks() {
	m.marked = make(map[string]bool)
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	selectStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#3b4261"))
	dangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	sizeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	trashedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Strikethrough(true)
	markedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	hintOldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	hintDupStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dcfff"))
	hintLogStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9e64"))
	summaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Bold(true).Underline(true)
	filterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68")).Bold(true).Underline(true)
	barStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
)

func (m *Model) View() string {
	if m.err != nil {
		return dangerStyle.Render("Error: "+m.err.Error()) + "\nq to quit\n"
	}

	switch m.view {
	case viewDocker:
		return m.dockerView()
	case viewDockerConfirm:
		return m.dockerConfirmView()
	case viewAnalyzer:
		return m.analyzerView()
	}

	if m.scanning {
		return m.scanView()
	}

	if len(m.items) == 0 {
		return "No items found.\n"
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Cleanup Tool"))
	b.WriteString(fmt.Sprintf("  total: %s  marked: %d\n", analyzer.PrettySize(m.totalRootsSize()), len(m.markedPaths())))
	if m.scanDuration > 0 {
		b.WriteString(fmt.Sprintf("  scanned %d files, %d dirs in %s (peak %.0f files/sec, %.0f dirs/sec)\n",
			m.files, m.dirs, m.scanDuration.Round(time.Millisecond), m.peakFilesPerSec, m.peakDirsPerSec))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%-3s %-10s %-12s %-15s %s\n", "", "Size", "Access", "Category", "Name"))

	start, end := m.visibleRange()
	for i := start; i < end && i < len(m.items); i++ {
		item := m.items[i]
		prefix := ""
		if m.marked[item.Path] {
			prefix = "[x]"
		} else {
			prefix = "[ ]"
		}
		label := m.treeLabel(item)
		line := fmt.Sprintf("%-3s %-10s %-12s %-15s %s",
			prefix,
			analyzer.PrettySize(item.Size),
			item.AccessTime.Format("2006-01-02"),
			categoryLabel(item),
			label,
		)
		style := lipgloss.NewStyle()
		if m.trashed[item.Path] {
			line = trashedStyle.Render(line)
		} else if m.marked[item.Path] {
			line = markedStyle.Render(line)
		} else if i == m.selected {
			line = selectStyle.Render(line)
		}
		_ = style
		b.WriteString(line + "\n")
	}

	hints := []string{
		"[j/k/down/up] navigate", "[l/enter/right] expand", "[h/esc/left] collapse",
		"[space] mark", "[c] clear", "[d] trash", "[m] move", "[u] restore",
		"[a] analyze dir", "[A] analyze selection", "[D] Docker", "[q] quit",
	}
	b.WriteString("\n" + formatHelpBar(m.width, hints) + "\n")
	if m.msg != "" {
		b.WriteString("\n" + m.msg + "\n")
	}
	return b.String()
}

func (m *Model) visibleRange() (int, int) {
	height := 20
	start := m.selected - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(m.items) {
		end = len(m.items)
	}
	return start, end
}

func (m *Model) scanView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Cleanup Tool — scanning..."))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " ")
	b.WriteString(fmt.Sprintf("Files: %d  Dirs: %d\n", m.files, m.dirs))

	elapsed := time.Since(m.scanStart)
	if !m.scanStart.IsZero() && elapsed > 0 {
		secs := elapsed.Seconds()
		b.WriteString(fmt.Sprintf("Speed: %.0f files/sec  %.0f dirs/sec  (%.1fs)\n",
			float64(m.files)/secs, float64(m.dirs)/secs, secs))
	}

	b.WriteString(fmt.Sprintf("Last: %s\n", truncate(m.lastPath, 60)))
	b.WriteString("\n" + formatHelpBar(m.width, []string{"[q] quit"}) + "\n")
	return b.String()
}

// updatePeakThroughput updates the stored peak rates based on the elapsed
// scan time. It should be called from Update whenever progress arrives, not
// from View, so the model stays pure.
func (m *Model) updatePeakThroughput(elapsed time.Duration) {
	if m.scanStart.IsZero() || elapsed <= 0 {
		return
	}
	secs := elapsed.Seconds()
	filesRate := float64(m.files) / secs
	dirsRate := float64(m.dirs) / secs
	if filesRate > m.peakFilesPerSec {
		m.peakFilesPerSec = filesRate
	}
	if dirsRate > m.peakDirsPerSec {
		m.peakDirsPerSec = dirsRate
	}
}

type summaryCategory struct {
	Value  int
	Reason analyzer.HintReason
	Label  string
}

func (sc summaryCategory) String() string {
	return fmt.Sprintf("%d %s", sc.Value, sc.Label)
}

func (m *Model) summaryCategories(summary analyzer.HintSummary) []summaryCategory {
	return []summaryCategory{
		{Value: summary.Old, Reason: analyzer.ReasonOld, Label: pluralize(summary.Old, "old file", "old files")},
		{Value: summary.Duplicate, Reason: analyzer.ReasonDuplicate, Label: pluralize(summary.Duplicate, "duplicate", "duplicates")},
		{Value: summary.LogCache, Reason: analyzer.ReasonLogCache, Label: "log/cache"},
	}
}

// stackedBarSegments returns the widths of the old, duplicate, and log/cache
// segments for a stacked bar of the given total width.
func stackedBarSegments(summary analyzer.HintSummary, width int) (int, int, int) {
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

// stackedBar renders a single stacked bar where each segment is proportional
// to the count of hints in that category.
func stackedBar(summary analyzer.HintSummary, width int) string {
	if summary.Old+summary.Duplicate+summary.LogCache == 0 {
		return barStyle.Render(strings.Repeat("░", width))
	}
	wOld, wDup, wLog := stackedBarSegments(summary, width)
	return hintOldStyle.Render(strings.Repeat("█", wOld)) +
		hintDupStyle.Render(strings.Repeat("█", wDup)) +
		hintLogStyle.Render(strings.Repeat("█", wLog))
}

func (m *Model) analyzerView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Deletability Analysis"))
	b.WriteString("\n\n")

	if m.analyzerRunning {
		b.WriteString(m.spinner.View() + " Analyzing...\n\n")
		b.WriteString(fmt.Sprintf("Stage:   %s\n", m.analyzerProg.Stage))
		b.WriteString(fmt.Sprintf("Files:   %d\n", m.analyzerProg.FilesProcessed))
		b.WriteString(fmt.Sprintf("Current: %s\n\n", truncate(m.analyzerProg.CurrentPath, 60)))

		summary := m.analyzerProg.HintsFound
		b.WriteString(fmt.Sprintf("Found so far: %s old %s, %s %s, %s %s\n",
			summaryStyle.Render(fmt.Sprintf("%d", summary.Old)),
			pluralize(summary.Old, "file", "files"),
			summaryStyle.Render(fmt.Sprintf("%d", summary.Duplicate)),
			pluralize(summary.Duplicate, "duplicate", "duplicates"),
			summaryStyle.Render(fmt.Sprintf("%d", summary.LogCache)),
			pluralize(summary.LogCache, "log/cache", "log/cache"),
		))
		b.WriteString("  " + stackedBar(summary, stackedBarWidth) + "\n\n")

		b.WriteString(formatHelpBar(m.width, []string{"[esc] cancel", "[q] quit"}) + "\n")
		return b.String()
	}

	if len(m.hints) == 0 {
		b.WriteString("No hints found.\n")
		b.WriteString("\n" + formatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d hints\n\n", len(m.hints)))

	summary := analyzer.SummarizeHints(m.hints)
	cats := m.summaryCategories(summary)
	renderCat := func(cat summaryCategory) string {
		s := cat.String()
		if m.analyzerFilter == cat.Reason {
			return filterStyle.Render(s)
		}
		if cat.Value > 0 {
			return summaryStyle.Render(s)
		}
		return s
	}
	parts := make([]string, len(cats))
	for i, cat := range cats {
		parts[i] = renderCat(cat)
	}
	b.WriteString(strings.Join(parts, ", ") + "\n")
	b.WriteString("  " + stackedBar(summary, stackedBarWidth) + "\n\n")

	filtered := m.filteredHints()
	b.WriteString(fmt.Sprintf("Showing %d of %d\n", len(filtered), len(m.hints)))
	b.WriteString(fmt.Sprintf("%-3s %-12s %-15s %s\n", "", "Reason", "Detail", "Path"))

	start, end := m.visibleRangeFor(len(filtered))
	for i := start; i < end && i < len(filtered); i++ {
		h := filtered[i]
		prefix := ""
		if m.marked[h.Entry.Path] {
			prefix = "[x]"
		} else {
			prefix = "[ ]"
		}
		var style lipgloss.Style
		switch h.Reason {
		case analyzer.ReasonOld:
			style = hintOldStyle
		case analyzer.ReasonDuplicate:
			style = hintDupStyle
		case analyzer.ReasonLogCache:
			style = hintLogStyle
		}
		line := fmt.Sprintf("%-3s %-12s %-15s %s",
			prefix,
			string(h.Reason),
			truncate(h.Detail, 14),
			truncate(h.Entry.Path, 60),
		)
		if i == m.selected {
			line = selectStyle.Render(line)
		} else {
			line = style.Render(line)
		}
		b.WriteString(line + "\n")
	}

	hints := []string{"[j/k/down/up] nav", "[tab/←/→] filter", "[0] clear filter", "[c] clear marks", "[space] mark", "[d] trash marked", "[esc] back", "[q] quit"}
	b.WriteString("\n" + formatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) visibleRangeFor(n int) (int, int) {
	height := 20
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

func (m *Model) dockerView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Docker Disk Usage"))
	b.WriteString("\n\n")

	if m.dockerErr != nil {
		b.WriteString(dangerStyle.Render("Error: "+m.dockerErr.Error()) + "\n")
		b.WriteString("\n" + formatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if m.dockerUsage == nil {
		b.WriteString(m.spinner.View() + " Loading Docker usage...\n")
		b.WriteString("\n" + formatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	type row struct {
		name string
		u    docker.ResourceUsage
		key  string
	}
	rows := []row{
		{"Images", m.dockerUsage.Images, "images"},
		{"Containers", m.dockerUsage.Containers, "containers"},
		{"Volumes", m.dockerUsage.Volumes, "volumes"},
		{"Build Cache", m.dockerUsage.BuildCache, "buildcache"},
	}

	for i, r := range rows {
		line := fmt.Sprintf("%-12s size: %-10s reclaimable: %-10s count: %d",
			r.name, analyzer.PrettySize(r.u.Size), analyzer.PrettySize(r.u.Reclaimable), r.u.TotalCount)
		if i == m.dockerSelected {
			line = selectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	total := m.dockerUsage.TotalSize()
	b.WriteString(fmt.Sprintf("\nTotal used: %s\n", analyzer.PrettySize(total)))
	if m.dockerMsg != "" {
		b.WriteString(m.dockerMsg + "\n")
	}
	hints := []string{"[↑/↓/j/k] navigate", "[p] prune selected", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + formatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) dockerConfirmView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Confirm Docker prune"))
	b.WriteString("\n\n")

	type item struct{ name, key string }
	items := []item{
		{"Images", "images"},
		{"Containers", "containers"},
		{"Volumes", "volumes"},
		{"Build Cache", "buildcache"},
	}
	selected := items[m.dockerSelected]
	b.WriteString(fmt.Sprintf("Prune %s? This action cannot be undone.\n\n", selected.name))
	b.WriteString(formatHelpBar(m.width, []string{"[y] yes", "[n] no"}) + "\n")
	return b.String()
}

func (m *Model) fetchDockerUsage() tea.Msg {
	if m.dockerClient == nil {
		return dockerUsageMsg{err: fmt.Errorf("docker client not available")}
	}
	usage, err := m.dockerClient.GetUsage(context.Background())
	return dockerUsageMsg{usage: usage, err: err}
}

func (m *Model) pruneDockerSelected() tea.Msg {
	if m.dockerClient == nil {
		return dockerPruneMsg{err: fmt.Errorf("docker client not available")}
	}
	items := []string{"images", "containers", "volumes", "buildcache"}
	if m.dockerSelected < 0 || m.dockerSelected >= len(items) {
		return dockerPruneMsg{err: fmt.Errorf("invalid selection")}
	}
	reclaimed, err := m.dockerClient.Prune(context.Background(), items[m.dockerSelected])
	return dockerPruneMsg{reclaimed: reclaimed, err: err}
}

func categoryLabel(item *analyzer.Entry) string {
	if item.IsDir {
		return "Directory"
	}
	return string(item.Category)
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

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// formatHelpBar lays out keybinding hints across as many lines as needed so
// that each line fits within the given width. If width is unknown (<=0), the
// hints are joined on a single line.
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

// Run starts the TUI with the given root entries.
func Run(roots []*analyzer.Entry, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int) error {
	m := New(roots, externalDir, false, dockerClient, dupMode, progressInterval)
	p := tea.NewProgram(m, tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// RunWithScan starts the TUI and scans the given paths in the background.
func RunWithScan(paths []string, ignore []string, ignoreHidden bool, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int) error {
	ctx, cancel := context.WithCancel(context.Background())
	p := tea.NewProgram(New(nil, externalDir, true, dockerClient, dupMode, progressInterval), tea.WithMouseCellMotion())
	go func() {
		defer cancel()
		scanner := analyzer.NewScanner(ignore, ignoreHidden, progressInterval)
		scanner.OnProgress = func(pr analyzer.Progress) {
			// Run progress emission in its own goroutine so the scanner never
			// stalls waiting for the Bubble Tea event loop. Progress messages
			// are informational only, and the progressInterval throttle keeps
			// the volume low enough that goroutine overhead is negligible.
			go p.Send(progressMsg{files: pr.Files, dirs: pr.Dirs, path: pr.Path})
		}
		roots, err := scanner.Scan(ctx, paths)
		if ctx.Err() != nil {
			return
		}
		p.Send(ScanMsg{Roots: roots, Err: err})
	}()
	if _, err := p.Run(); err != nil {
		cancel()
		return err
	}
	return nil
}
