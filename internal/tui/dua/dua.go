// Package dua implements a dua-cli-style interactive disk-usage browser.
// It shows a flat list of the current directory's entries sorted by size, with
// keyboard navigation (enter to descend, backspace/h/esc to go up, d to mark,
// x to trash, m to move, r to restore, a to analyze, D for Docker, ? for help,
// q to quit).
package dua

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

// viewState distinguishes the main browser, analyzer, and Docker views.
type viewState int

const (
	viewFiles viewState = iota
	viewDocker
	viewDockerConfirm
	viewAnalyzer

	// stackedBarWidth is the total width of the stacked summary bar.
	stackedBarWidth = 24
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

	externalDir      string
	dockerClient     docker.Client
	dockerUsage      *docker.Usage
	dockerSelected   int
	dockerErr        error
	dockerMsg        string
	hints            []*analyzer.DeletabilityHint
	analyzerFilter   analyzer.HintReason
	analyzerRunning  bool
	analyzerCancel   context.CancelFunc
	analyzerProg     analyzer.AnalyzerProgress
	analyzerProgCh   chan analyzer.AnalyzerProgress
	analyzerDoneCh   chan analyzerMsg
	dupMode          analyzer.DupHashMode
	progressInterval int
	view             viewState
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
func New(scanning bool, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int) *Model {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	m := &Model{
		scanning:         scanning,
		spinner:          sp,
		trashed:          make(map[string]bool),
		marked:           make(map[string]bool),
		externalDir:      externalDir,
		dockerClient:     dockerClient,
		dupMode:          dupMode,
		progressInterval: progressInterval,
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
	case moveMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Move failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				m.trashed[p] = true
				delete(m.marked, p)
			}
			m.rebuild()
			m.msg = fmt.Sprintf("Moved to external: %d items", len(msg.paths))
		}
		return m, nil
	case restoreMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Restore failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				delete(m.trashed, p)
			}
			m.rebuild()
			m.msg = fmt.Sprintf("Restored: %d items", len(msg.paths))
		}
		return m, nil
	case dockerUsageMsg:
		m.scanning = false
		if msg.err != nil {
			m.dockerErr = msg.err
		} else {
			m.dockerUsage = msg.usage
			m.dockerErr = nil
		}
		return m, nil
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
	switch m.view {
	case viewDockerConfirm:
		return m.handleDockerConfirmKey(msg)
	case viewDocker:
		return m.handleDockerKey(msg)
	case viewAnalyzer:
		return m.handleAnalyzerKey(msg)
	}

	if m.showHelp {
		switch msg.String() {
		case "?", "q", "esc", "ctrl+c":
			m.showHelp = false
		}
		return m, nil
	}

	// Any key dismisses a transient error/status banner so the user doesn't
	// have to quit the app.
	if m.err != nil || m.msg != "" {
		m.err = nil
		m.msg = ""
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
		return m.trashMarkedOrSelected()
	case "m":
		return m.moveMarkedOrSelected()
	case "r":
		return m.restoreSelected()
	case "c":
		m.clearMarks()
	case "a":
		m.view = viewAnalyzer
		m.analyzerRunning = true
		m.selected = 0
		m.err = nil
		return m, m.runAnalyzer()
	case "A":
		m.view = viewAnalyzer
		m.analyzerRunning = true
		m.selected = 0
		m.err = nil
		return m, m.runAnalyzerOnSelection()
	case "D":
		m.view = viewDocker
		m.dockerSelected = 0
		m.dockerErr = nil
		m.dockerMsg = ""
		m.err = nil
		return m, m.fetchDockerUsage
	case "?":
		m.showHelp = true
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
		m.dockerErr = nil
		m.dockerMsg = ""
		return m, m.fetchDockerUsage
	}
	return m, nil
}

func (m *Model) handleDockerConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if len(paths) == 0 {
			filtered := m.filteredHints()
			if m.selected < len(filtered) {
				paths = []string{filtered[m.selected].Entry.Path}
			}
		}
		if len(paths) > 0 {
			return m, m.trashPaths(paths)
		}
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

func (m *Model) trashMarkedOrSelected() (tea.Model, tea.Cmd) {
	paths := m.selectedActionPaths()
	if len(paths) == 0 {
		return m, nil
	}
	m.err = nil
	return m, m.trashPaths(paths)
}

func (m *Model) moveMarkedOrSelected() (tea.Model, tea.Cmd) {
	if m.externalDir == "" {
		return m, func() tea.Msg { return moveMsg{err: fmt.Errorf("no external dir set")} }
	}
	paths := m.selectedActionPaths()
	if len(paths) == 0 {
		return m, nil
	}
	m.err = nil
	return m, m.movePaths(paths)
}

func (m *Model) restoreSelected() (tea.Model, tea.Cmd) {
	item := m.selectedItem()
	if item == nil {
		return m, nil
	}
	m.err = nil
	return m, func() tea.Msg {
		if err := actions.Restore(filepath.Join(os.Getenv("HOME"), ".Trash"), item.Path); err != nil {
			return restoreMsg{paths: []string{item.Path}, err: err}
		}
		return restoreMsg{paths: []string{item.Path}}
	}
}

func (m *Model) trashPaths(paths []string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.Trash(paths...); err != nil {
			return trashMsg{paths: paths, err: err}
		}
		return trashMsg{paths: paths}
	}
}

func (m *Model) movePaths(paths []string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.MoveToExternal(m.externalDir, paths...); err != nil {
			return moveMsg{paths: paths, err: err}
		}
		return moveMsg{paths: paths}
	}
}

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

func (m *Model) selectedActionPaths() []string {
	paths := m.markedPaths()
	if len(paths) == 0 {
		item := m.selectedItem()
		if item != nil {
			paths = []string{item.Path}
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
	m.items = sortedChildren(m.current.Children)
	if m.selected >= len(m.items) {
		if len(m.items) == 0 {
			m.selected = 0
		} else {
			m.selected = len(m.items) - 1
		}
	}
}

// sortedChildren returns children sorted by size. Trashed items are not
// removed so they can be restored; they are styled differently in the view.
func sortedChildren(children []*analyzer.Entry) []*analyzer.Entry {
	out := make([]*analyzer.Entry, len(children))
	copy(out, children)
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
	switch m.view {
	case viewDocker:
		return m.dockerView()
	case viewDockerConfirm:
		return m.dockerConfirmView()
	case viewAnalyzer:
		return m.analyzerView()
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
		"[d] mark", "[x] trash", "[m] move", "[r] restore",
		"[a] analyze dir", "[A] analyze selection", "[D] Docker", "[c] clear", "[?] help", "[q] quit",
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
		"  m            move marked items (or selected) to external drive",
		"  r            restore selected item from Trash",
		"  c            clear all marks",
		"  a            analyze current directory",
		"  A            analyze selected/marked items",
		"  D            show Docker disk usage",
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
		if m.msg != "" {
			b.WriteString("\n" + msgStyle.Render(m.msg) + "\n")
		}
		b.WriteString("\n" + formatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d hints\n\n", len(m.hints)))
	if m.msg != "" {
		b.WriteString(msgStyle.Render(m.msg) + "\n")
	}

	summary := analyzer.SummarizeHints(m.hints)
	cats := summaryCategories(summary)
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

	start, end := m.visibleRange(len(filtered))
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

	types := []struct {
		name string
		u    docker.ResourceUsage
		key  string
	}{
		{"Images", m.dockerUsage.Images, "images"},
		{"Containers", m.dockerUsage.Containers, "containers"},
		{"Volumes", m.dockerUsage.Volumes, "volumes"},
		{"Build Cache", m.dockerUsage.BuildCache, "buildcache"},
	}

	for i, r := range types {
		line := fmt.Sprintf("%-12s size: %-10s reclaimable: %-10s count: %d",
			r.name, analyzer.PrettySize(r.u.Size), analyzer.PrettySize(r.u.Reclaimable), r.u.TotalCount)
		if i == m.dockerSelected {
			line = selectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	total := m.dockerUsage.TotalSize()
	b.WriteString(fmt.Sprintf("\nTotal used: %s\n", analyzer.PrettySize(total)))
	if m.msg != "" {
		b.WriteString(msgStyle.Render(m.msg) + "\n")
	}
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

	items := []string{"Images", "Containers", "Volumes", "Build Cache"}
	selected := items[m.dockerSelected]
	b.WriteString(fmt.Sprintf("Prune %s? This action cannot be undone.\n\n", selected))
	b.WriteString(formatHelpBar(m.width, []string{"[y] yes", "[n] no"}) + "\n")
	return b.String()
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

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

type summaryCategory struct {
	Value  int
	Reason analyzer.HintReason
	Label  string
}

func (sc summaryCategory) String() string {
	return fmt.Sprintf("%d %s", sc.Value, sc.Label)
}

func summaryCategories(summary analyzer.HintSummary) []summaryCategory {
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

func (m *Model) runAnalyzer() tea.Cmd {
	if m.current == nil {
		return func() tea.Msg { return analyzerMsg{err: fmt.Errorf("no directory selected")} }
	}
	return m.analyzeRoot(m.current)
}

func (m *Model) runAnalyzerOnSelection() tea.Cmd {
	paths := m.selectedActionPaths()
	if len(paths) == 0 {
		return func() tea.Msg { return analyzerMsg{err: fmt.Errorf("no items selected")} }
	}
	root := &analyzer.Entry{Path: "selection", Name: "selection", IsDir: true}
	for _, p := range paths {
		e := findEntryByPath(m.roots, p)
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

type analyzerMsg struct {
	hints []*analyzer.DeletabilityHint
	err   error
}

type analyzerProgressMsg analyzer.AnalyzerProgress

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

type dockerUsageMsg struct {
	usage *docker.Usage
	err   error
}

type dockerPruneMsg struct {
	reclaimed int64
	err       error
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

func findEntryByPath(roots []*analyzer.Entry, target string) *analyzer.Entry {
	for _, root := range roots {
		if e := analyzer.FindEntryByPath(root, target); e != nil {
			return e
		}
	}
	return nil
}

var (
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	selectStyle      = lipgloss.NewStyle().Background(lipgloss.Color("#3b4261"))
	dangerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	markedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	trashedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Strikethrough(true)
	currentPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	msgStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	barStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dcfff"))
	helpBoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	hintOldStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	hintDupStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7dcfff"))
	hintLogStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9e64"))
	summaryStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Bold(true).Underline(true)
	filterStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68")).Bold(true).Underline(true)
)

// RunWithScan starts the dua-style TUI and scans the given paths in the
// background. progressInterval controls how often scan progress is reported
// (a value <= 0 disables progress reports).
func RunWithScan(paths []string, ignore []string, ignoreHidden bool, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int) error {
	ctx, cancel := context.WithCancel(context.Background())
	p := tea.NewProgram(New(true, externalDir, dockerClient, dupMode, progressInterval))
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
