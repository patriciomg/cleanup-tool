package terminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/patriciomg/cleanup-tool/internal/actions"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/config"
	"github.com/patriciomg/cleanup-tool/internal/defaults"
	"github.com/patriciomg/cleanup-tool/internal/notifications"
	"github.com/patriciomg/cleanup-tool/internal/deps"
	"github.com/patriciomg/cleanup-tool/internal/recent"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/llm"
	"github.com/patriciomg/cleanup-tool/internal/tui/common"
	"github.com/patriciomg/cleanup-tool/internal/tui/dockeritems"
	"github.com/patriciomg/cleanup-tool/internal/undo"
)

type viewState int

const (
	viewFiles viewState = iota
	viewDocker
	viewDockerConfirm
	viewDockerItems
	viewAnalyzer
	viewModels
	viewModelsConfirm
	viewDeps
	viewConfirmAction

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
	dockerItems    *dockeritems.Model
	llmClient      llm.Client
	llmRegistries  []llm.Registry
	llmSelectedReg int
	llmSelectedModel int
	llmErr         error
	llmMsg         string
	marked           map[string]bool
	trashed          map[string]bool
	expanded         map[string]bool
	scanStart        time.Time

	ignoreHidden bool
	ignorePaths  []string
	scanPaths    []string
	filter       string
	filtering    bool
	sortOrder    string
	depsList     []*deps.DependencyDir
	depsSelected int
	depsErr      error
	depsMsg      string
	depsRunning  bool
	depsMarked   map[string]bool
	scanDuration     time.Duration
	peakFilesPerSec  float64
	peakDirsPerSec   float64
	undoStack        *undo.Stack
	hints            []*analyzer.DeletabilityHint
	analyzerFilter   analyzer.HintReason
	analyzerRunning  bool
	analyzerCancel   context.CancelFunc
	analyzerProg     analyzer.AnalyzerProgress
	analyzerProgCh   chan analyzer.AnalyzerProgress
	analyzerDoneCh   chan analyzerMsg
	dupMode          analyzer.DupHashMode
	progressInterval int
	cfg              *config.Config

	pendingAction *actionIntent
}

// actionIntent holds a trash/move action awaiting user confirmation.
type actionIntent struct {
	actionType string // "trash" or "move"
	paths      []string
	totalSize  int64
	returnView viewState
}

type ScanMsg struct {
	Roots []*analyzer.Entry
	Err   error
}

// depsMsg is sent when the dependency directory scan finishes.
type depsMsg struct {
	deps []*deps.DependencyDir
	err  error
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


type llmMsg struct {
	registries []llm.Registry
	err        error
}

type llmDeleteMsg struct {
	registry string
	model    string
	err      error
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

type undoMsg struct {
	op    undo.Operation
	paths []string
	err   error
}

func New(roots []*analyzer.Entry, externalDir string, scanning bool, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int, cfg *config.Config) *Model {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	m := &Model{
		roots:            roots,
		externalDir:      externalDir,
		spinner:          sp,
		scanning:         scanning,
		dockerClient:     dockerClient,
		llmClient:        llm.NewClient(),
		marked:           make(map[string]bool),
		trashed:          make(map[string]bool),
		expanded:         make(map[string]bool),
		depsMarked:       make(map[string]bool),
		dupMode:          dupMode,
		progressInterval: progressInterval,
		undoStack:        undo.NewStack(10),
		cfg:              cfg,
	}
	_ = m.undoStack.Load(config.UndoPath())
	if len(roots) > 0 {
		m.currentDir = roots[0]
	}
	m.sortOrder = "size"
	m.applyPreferences()
	common.SortTree(m.roots, m.sortOrder)
	m.rebuild()
	if scanning {
		m.scanStart = time.Now()
		m.peakFilesPerSec = 0
		m.peakDirsPerSec = 0
	}
	return m
}

// applyPreferences restores TUI state from the loaded config.
func (m *Model) applyPreferences() {
	if m.cfg == nil {
		return
	}
	switch m.cfg.LastView {
	case "docker":
		m.view = viewDocker
	case "analyzer":
		m.view = viewAnalyzer
	case "models":
		m.view = viewModels
	case "deps":
		m.view = viewDeps
	default:
		m.view = viewFiles
	}
	if m.cfg.AnalyzerFilter != "" {
		m.analyzerFilter = analyzer.HintReason(m.cfg.AnalyzerFilter)
	}
	if m.cfg.SortOrder != "" {
		m.sortOrder = m.cfg.SortOrder
	} else {
		m.sortOrder = "size"
	}
}

// saveViewPreference persists the current view in the config when the user
// navigates to a named top-level view.
func (m *Model) saveViewPreference() {
	if m.cfg == nil {
		return
	}
	switch m.view {
	case viewFiles:
		m.cfg.LastView = "files"
	case viewDocker, viewDockerItems, viewDockerConfirm:
		m.cfg.LastView = "docker"
	case viewAnalyzer:
		m.cfg.LastView = "analyzer"
	case viewModels, viewModelsConfirm:
		m.cfg.LastView = "models"
	case viewDeps:
		m.cfg.LastView = "deps"
	}
}

// saveAnalyzerFilterPreference persists the current analyzer filter in the
// config so it can be restored on the next run.
func (m *Model) saveAnalyzerFilterPreference() {
	if m.cfg == nil {
		return
	}
	m.cfg.AnalyzerFilter = string(m.analyzerFilter)
}

// saveSortOrderPreference persists the current file browser sort order.
func (m *Model) saveSortOrderPreference() {
	if m.cfg == nil {
		return
	}
	m.cfg.SortOrder = m.sortOrder
}

// cycleSortOrder rotates through the available sort orders, re-sorts the
// scanned tree, and rebuilds the visible list so the change is immediately
// visible.
func (m *Model) cycleSortOrder() {
	orders := []string{"size", "name", "access", "modified"}
	idx := 0
	for i, o := range orders {
		if o == m.sortOrder {
			idx = i
			break
		}
	}
	idx = (idx + 1) % len(orders)
	m.sortOrder = orders[idx]
	m.saveSortOrderPreference()
	common.SortTree(m.roots, m.sortOrder)
	m.rebuild()
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
		if m.dockerItems != nil {
			m.dockerItems, _ = m.dockerItems.UpdateModel(msg)
		}
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
			common.SortTree(m.roots, m.sortOrder)
			if len(m.roots) > 0 {
				m.currentDir = m.roots[0]
			}
			m.rebuild()
		}
		// Notify the user if the scan took a while.
		if !m.scanStart.IsZero() && m.cfg != nil {
			notifications.ScanComplete(time.Since(m.scanStart), m.cfg.NotificationsEnabled)
		}
	case depsMsg:
		m.depsRunning = false
		if msg.err != nil {
			m.depsErr = msg.err
			m.depsMsg = "Deps error: " + msg.err.Error()
		} else {
			m.depsList = msg.deps
			m.depsSelected = 0
			m.depsMsg = fmt.Sprintf("Found %d dependency directories", len(msg.deps))
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
	case dockeritems.CloseMsg:
		if msg.Quit {
			return m, tea.Quit
		}
		m.view = viewDocker
		return m, nil
	case dockeritems.RefreshUsageMsg:
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
			removed := make(map[string]bool)
			for _, p := range msg.paths {
				m.trashed[p] = true
				delete(m.marked, p)
				delete(m.depsMarked, p)
				removed[p] = true
			}
			m.filterDepsList(removed)
			m.msg = fmt.Sprintf("Moved to Trash: %d items", len(msg.paths))
		}
	case moveMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Move failed: " + msg.err.Error()
		} else {
			removed := make(map[string]bool)
			for _, p := range msg.paths {
				m.trashed[p] = true
				delete(m.marked, p)
				delete(m.depsMarked, p)
				removed[p] = true
			}
			m.filterDepsList(removed)
			m.msg = fmt.Sprintf("Moved to external: %d items", len(msg.paths))
		}
	case undoMsg:
		if msg.err != nil {
			if msg.op.Type != "" && m.undoStack != nil {
				m.undoStack.Push(msg.op)
			}
			m.err = msg.err
			m.msg = "Undo failed: " + msg.err.Error()
		} else {
			for _, p := range msg.paths {
				delete(m.trashed, p)
			}
			m.rebuild()
			m.msg = fmt.Sprintf("Undone: %d items", len(msg.paths))
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
	case llmMsg:
		if msg.err != nil {
			m.llmErr = msg.err
		} else {
			m.llmRegistries = msg.registries
			m.llmErr = nil
		}
	case llmDeleteMsg:
		if msg.err != nil {
			m.llmErr = msg.err
			m.llmMsg = "Delete failed: " + msg.err.Error()
		} else {
			m.llmMsg = fmt.Sprintf("Deleted %s from %s", msg.model, msg.registry)
			return m, m.fetchLLMRegistries
		}
	}
	return m, nil
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseWheelUp:
		m.mouseScroll(-1)
		return m, nil
	case tea.MouseWheelDown:
		m.mouseScroll(1)
		return m, nil
	}
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
	cats := common.SummaryCategories(summary)
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
	wOld, wDup, _ := common.StackedBarSegments(summary, stackedBarWidth)
	switch {
	case x < wOld:
		return analyzer.ReasonOld, true
	case x < wOld+wDup:
		return analyzer.ReasonDuplicate, true
	default:
		return analyzer.ReasonLogCache, true
	}
}

// mouseScroll moves the selection of the active list by one row per wheel
// notch (dir is -1 for up, +1 for down).
func (m *Model) mouseScroll(dir int) {
	switch m.view {
	case viewFiles:
		if dir < 0 && m.selected > 0 {
			m.selected--
		} else if dir > 0 && m.selected < len(m.items)-1 {
			m.selected++
		}
	case viewAnalyzer:
		if m.analyzerRunning {
			return
		}
		n := len(m.filteredHints())
		if dir < 0 && m.selected > 0 {
			m.selected--
		} else if dir > 0 && m.selected < n-1 {
			m.selected++
		}
	case viewDeps:
		if dir < 0 && m.depsSelected > 0 {
			m.depsSelected--
		} else if dir > 0 && m.depsSelected < len(m.depsList)-1 {
			m.depsSelected++
		}
	}
}

// pageFiles moves the file-browser selection by a full visible page.
func (m *Model) pageFiles(dir int) {
	n := len(m.items)
	if n == 0 {
		return
	}
	page := 20
	if dir < 0 {
		m.selected -= page
		if m.selected < 0 {
			m.selected = 0
		}
	} else {
		m.selected += page
		if m.selected >= n {
			m.selected = n - 1
		}
	}
}

// pageAnalyzer moves the analyzer selection by a full visible page.
func (m *Model) pageAnalyzer(dir int) {
	filtered := m.filteredHints()
	n := len(filtered)
	if n == 0 {
		return
	}
	page := 20
	if dir < 0 {
		m.selected -= page
		if m.selected < 0 {
			m.selected = 0
		}
	} else {
		m.selected += page
		if m.selected >= n {
			m.selected = n - 1
		}
	}
}

// pageDeps moves the deps selection by a full visible page.
func (m *Model) pageDeps(dir int) {
	n := len(m.depsList)
	if n == 0 {
		return
	}
	page := 20
	if dir < 0 {
		m.depsSelected -= page
		if m.depsSelected < 0 {
			m.depsSelected = 0
		}
	} else {
		m.depsSelected += page
		if m.depsSelected >= n {
			m.depsSelected = n - 1
		}
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		switch msg.Type {
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
			m.rebuild()
		case tea.KeyEnter:
			m.filtering = false
			m.rebuild()
		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				runes := []rune(m.filter)
				m.filter = string(runes[:len(runes)-1])
			}
			m.rebuild()
		case tea.KeyRunes:
			m.filter += msg.String()
			m.rebuild()
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		return m, nil
	}

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
	case viewDockerItems:
		if m.dockerItems == nil {
			m.view = viewDocker
			return m, nil
		}
		var cmd tea.Cmd
		m.dockerItems, cmd = m.dockerItems.UpdateModel(msg)
		return m, cmd
	case viewAnalyzer:
		return m.handleAnalyzerKey(msg)
	case viewModels:
		return m.handleModelsKey(msg)
	case viewModelsConfirm:
		switch msg.String() {
		case "y":
			m.view = viewModels
			return m, m.deleteSelectedModel()
		case "n", "esc":
			m.view = viewModels
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	case viewDeps:
		return m.handleDepsKey(msg)
	case viewConfirmAction:
		return m.handleConfirmActionKey(msg)
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
	case "pgup", "ctrl+b":
		m.pageFiles(-1)
	case "pgdown", "ctrl+f":
		m.pageFiles(1)
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
	case "Z":
		return m.undoLast()
	case "D":
		m.view = viewDocker
		return m, m.fetchDockerUsage
	case "M":
		m.view = viewModels
		return m, m.fetchLLMRegistries
	case "P":
		m.view = viewDeps
		m.depsSelected = 0
		m.depsErr = nil
		m.depsMsg = ""
		return m, m.fetchDeps()
	case "a":
		m.view = viewAnalyzer
		m.analyzerRunning = true
		return m, m.runAnalyzer()
	case "A":
		m.view = viewAnalyzer
		m.analyzerRunning = true
		return m, m.runAnalyzerOnSelection()
	case "o":
		if rps, err := recent.Paths(); err == nil && len(rps) > 0 {
			m.msg = "Recent: " + strings.Join(rps, ", ")
		} else {
			m.msg = "No recent paths"
		}
		return m, nil
	case "/":
		m.filtering = true
		m.filter = ""
		m.rebuild()
		return m, nil
	case "s":
		m.cycleSortOrder()
		return m, nil
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
	case "l", "enter", "tab":
		cats := []string{"images", "containers", "volumes", "buildcache"}
		cat := cats[m.dockerSelected]
		if cat == "buildcache" {
			m.dockerMsg = "Per-item view is not available for build cache. Use [p] to prune."
			return m, nil
		}
		m.dockerItems = dockeritems.New(m.dockerClient, cat, m.width, m.height)
		m.view = viewDockerItems
		return m, m.dockerItems.Init()
	case "r":
		return m, m.fetchDockerUsage
	}
	return m, nil
}

func (m *Model) handleConfirmActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		intent := m.pendingAction
		m.pendingAction = nil
		if intent == nil {
			m.view = viewFiles
			return m, nil
		}
		m.view = intent.returnView
		switch intent.actionType {
		case "trash":
			return m, m.trashPaths(intent.paths)
		case "move":
			return m, m.movePaths(intent.paths)
		}
		return m, nil
	case "n", "esc":
		returnView := viewFiles
		if m.pendingAction != nil {
			returnView = m.pendingAction.returnView
		}
		m.pendingAction = nil
		m.view = returnView
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) confirmActionView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Confirm action"))
	b.WriteString("\n\n")

	if m.pendingAction == nil {
		b.WriteString("No action pending.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back"}) + "\n")
		return b.String()
	}

	intent := m.pendingAction
	var action string
	switch intent.actionType {
	case "trash":
		action = "Trash"
	case "move":
		action = "Move to external"
	}

	b.WriteString(fmt.Sprintf("%s %d item(s), total size %s\n\n", action, len(intent.paths), analyzer.PrettySize(intent.totalSize)))

	maxPreview := 10
	for i, p := range intent.paths {
		if i >= maxPreview {
			b.WriteString(fmt.Sprintf("... and %d more\n", len(intent.paths)-maxPreview))
			break
		}
		b.WriteString(fmt.Sprintf("  %s\n", common.Truncate(p, m.width-4)))
	}

	b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[y] confirm", "[n] cancel"}) + "\n")
	return b.String()
}

func (m *Model) handleModelsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = viewFiles
		return m, nil
	case "up", "k":
		m.navigateModelsUp()
	case "down", "j":
		m.navigateModelsDown()
	case "d":
		m.view = viewModelsConfirm
		return m, nil
	case "r":
		return m, m.fetchLLMRegistries
	}
	return m, nil
}

func (m *Model) navigateModelsUp() {
	if len(m.llmRegistries) == 0 {
		return
	}
	if m.llmSelectedModel > 0 {
		m.llmSelectedModel--
		return
	}
	// Move to the last model of the previous non-empty registry.
	for m.llmSelectedReg > 0 {
		m.llmSelectedReg--
		prev := m.llmRegistries[m.llmSelectedReg]
		if len(prev.Models) > 0 {
			m.llmSelectedModel = len(prev.Models) - 1
			return
		}
	}
}

func (m *Model) navigateModelsDown() {
	if len(m.llmRegistries) == 0 {
		return
	}
	reg := m.llmRegistries[m.llmSelectedReg]
	if m.llmSelectedModel < len(reg.Models)-1 {
		m.llmSelectedModel++
		return
	}
	if m.llmSelectedReg < len(m.llmRegistries)-1 {
		m.llmSelectedReg++
		m.llmSelectedModel = 0
	}
}

func (m *Model) selectedModel() (llm.Registry, llm.Model, bool) {
	if m.llmSelectedReg >= len(m.llmRegistries) {
		return llm.Registry{}, llm.Model{}, false
	}
	reg := m.llmRegistries[m.llmSelectedReg]
	if m.llmSelectedModel >= len(reg.Models) {
		return reg, llm.Model{}, false
	}
	return reg, reg.Models[m.llmSelectedModel], true
}

func (m *Model) deleteSelectedModel() tea.Cmd {
	reg, model, ok := m.selectedModel()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		if err := m.llmClient.DeleteModel(context.Background(), reg.Name, model.Name); err != nil {
			return llmDeleteMsg{registry: reg.Name, model: model.Name, err: err}
		}
		return llmDeleteMsg{registry: reg.Name, model: model.Name}
	}
}

func (m *Model) fetchLLMRegistries() tea.Msg {
	if m.llmClient == nil {
		return llmMsg{err: fmt.Errorf("llm client not available")}
	}
	regs, err := m.llmClient.GetRegistries(context.Background())
	return llmMsg{registries: regs, err: err}
}

func (m *Model) modelsView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("LLM Model Registries"))
	b.WriteString("\n\n")
	if m.llmErr != nil {
		b.WriteString(common.DangerStyle.Render("Error: "+m.llmErr.Error()) + "\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if m.llmClient == nil {
		b.WriteString(m.spinner.View() + " Loading LLM registries...\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if len(m.llmRegistries) == 0 {
		b.WriteString("No LLM registries found.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("%-20s %-12s %s\n", "Registry", "Models", "Size"))
	for i, reg := range m.llmRegistries {
		line := fmt.Sprintf("%-20s %-12d %s", reg.Name, len(reg.Models), analyzer.PrettySize(reg.TotalSize()))
		if i == m.llmSelectedReg {
			line = common.SelectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	if m.llmSelectedReg < len(m.llmRegistries) {
		reg := m.llmRegistries[m.llmSelectedReg]
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("Models in %s\n", reg.Name))
		b.WriteString(fmt.Sprintf("%-40s %s\n", "Name", "Size"))
		for i, model := range reg.Models {
			line := fmt.Sprintf("%-40s %s", common.Truncate(model.Name, 38), analyzer.PrettySize(model.Size))
			if i == m.llmSelectedModel {
				line = common.SelectStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	if m.llmMsg != "" {
		b.WriteString("\n" + m.llmMsg + "\n")
	}
	hints := []string{"[↑/↓/j/k] navigate", "[d] delete model", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
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
	case "pgup", "ctrl+b":
		m.pageAnalyzer(-1)
	case "pgdown", "ctrl+f":
		m.pageAnalyzer(1)
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
			m.pendingAction = &actionIntent{actionType: "trash", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewAnalyzer}
			m.view = viewConfirmAction
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
	m.pendingAction = &actionIntent{actionType: "trash", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewFiles}
	m.view = viewConfirmAction
	return nil
}

// sizeForPaths sums the recursive size of the given paths by looking them up
// in the scanned tree.
func (m *Model) sizeForPaths(paths []string) int64 {
	var total int64
	for _, p := range paths {
		if e := m.findEntryByPath(p); e != nil {
			total += e.Size
		}
	}
	return total
}

func (m *Model) trashPaths(paths []string) tea.Cmd {
	return func() tea.Msg {
		dests, err := actions.TrashWithDest(paths...)
		if err != nil {
			return trashMsg{paths: paths, err: err}
		}
		m.pushUndo(undo.OpTrash, paths, dests)
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
	m.pendingAction = &actionIntent{actionType: "move", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewFiles}
	m.view = viewConfirmAction
	return nil
}

func (m *Model) movePaths(paths []string) tea.Cmd {
	return func() tea.Msg {
		dests, err := actions.MoveToExternalWithDest(m.externalDir, paths...)
		if err != nil {
			return moveMsg{paths: paths, err: err}
		}
		m.pushUndo(undo.OpMove, paths, dests)
		return moveMsg{paths: paths}
	}
}

func (m *Model) pushUndo(typ undo.OpType, paths, dests []string) {
	if m.undoStack == nil {
		return
	}
	op := undo.Operation{
		Type:      typ,
		Timestamp: time.Now(),
		Items:     make([]undo.Item, len(paths)),
	}
	for i, p := range paths {
		op.Items[i] = undo.Item{Original: p, Dest: dests[i]}
	}
	m.undoStack.Push(op)
}

func (m *Model) undoLast() (tea.Model, tea.Cmd) {
	if m.undoStack == nil || m.undoStack.Len() == 0 {
		m.msg = "Nothing to undo"
		return m, nil
	}
	op, ok := m.undoStack.Pop()
	if !ok {
		m.msg = "Nothing to undo"
		return m, nil
	}

	var originalPaths []string
	for _, it := range op.Items {
		originalPaths = append(originalPaths, it.Original)
	}

	return m, func() tea.Msg {
		if err := actions.Undo(op); err != nil {
			return undoMsg{op: op, paths: originalPaths, err: err}
		}
		return undoMsg{op: op, paths: originalPaths}
	}
}

func (m *Model) restoreSelected() tea.Cmd {
	if m.selected >= len(m.items) {
		return nil
	}
	item := m.items[m.selected]
	trashDir := filepath.Join(os.Getenv("HOME"), ".Trash")
	trashPath := filepath.Join(trashDir, filepath.Base(item.Path))
	return func() tea.Msg {
		if err := actions.Restore(trashPath, item.Path); err != nil {
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
	if m.filter != "" {
		f := strings.ToLower(m.filter)
		var filtered []*analyzer.Entry
		for _, item := range m.items {
			if strings.Contains(strings.ToLower(item.Name), f) {
				filtered = append(filtered, item)
			}
		}
		m.items = filtered
	}
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



func (m *Model) View() string {
	if m.err != nil {
		return common.DangerStyle.Render("Error: "+m.err.Error()) + "\nq to quit\n"
	}

	switch m.view {
	case viewDocker:
		return m.dockerView()
	case viewDockerConfirm:
		return m.dockerConfirmView()
	case viewDockerItems:
		if m.dockerItems == nil {
			return ""
		}
		return m.dockerItems.View()
	case viewAnalyzer:
		return m.analyzerView()
	case viewModels:
		return m.modelsView()
	case viewDeps:
		return m.depsView()
	case viewConfirmAction:
		return m.confirmActionView()
	}

	if m.scanning {
		return m.scanView()
	}

	var b strings.Builder
	if m.filtering || m.filter != "" {
		cursor := ""
		if m.filtering {
			cursor = "_"
		}
		b.WriteString(fmt.Sprintf("Filter: /%s%s\n", m.filter, cursor))
	}

	if len(m.items) == 0 {
		b.WriteString("No items found.\n")
		return b.String()
	}

	b.WriteString(common.HeaderStyle.Render("Cleanup Tool"))
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
			line = common.TrashedStyle.Render(line)
		} else if m.marked[item.Path] {
			line = common.MarkedStyle.Render(line)
		} else if i == m.selected {
			line = common.SelectStyle.Render(line)
		}
		_ = style
		b.WriteString(line + "\n")
	}

	hints := []string{
		"[j/k/down/up] navigate", "[l/enter/right] expand", "[h/esc/left] collapse",
		"[space] mark", "[c] clear", "[d] trash", "[m] move", "[u] restore", "[Z] undo",
		"[a] analyze dir", "[A] analyze selection", "[P] deps", "[D] Docker", "[M] Models", "[o] recent", "[/] filter", "[s] sort", "[q] quit",
	}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
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
	b.WriteString(common.HeaderStyle.Render("Cleanup Tool — scanning..."))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " ")
	b.WriteString(fmt.Sprintf("Files: %d  Dirs: %d\n", m.files, m.dirs))

	elapsed := time.Since(m.scanStart)
	if !m.scanStart.IsZero() && elapsed > 0 {
		secs := elapsed.Seconds()
		b.WriteString(fmt.Sprintf("Speed: %.0f files/sec  %.0f dirs/sec  (%.1fs)\n",
			float64(m.files)/secs, float64(m.dirs)/secs, secs))
	}

	b.WriteString(fmt.Sprintf("Last: %s\n", common.Truncate(m.lastPath, 60)))
	b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[q] quit"}) + "\n")
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

func (m *Model) analyzerView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Deletability Analysis"))
	b.WriteString("\n\n")

	if m.analyzerRunning {
		b.WriteString(m.spinner.View() + " Analyzing...\n\n")
		b.WriteString(fmt.Sprintf("Stage:   %s\n", m.analyzerProg.Stage))
		b.WriteString(fmt.Sprintf("Files:   %d\n", m.analyzerProg.FilesProcessed))
		b.WriteString(fmt.Sprintf("Current: %s\n\n", common.Truncate(m.analyzerProg.CurrentPath, 60)))

		summary := m.analyzerProg.HintsFound
		b.WriteString(fmt.Sprintf("Found so far: %s old %s, %s %s, %s %s\n",
			common.SummaryStyle.Render(fmt.Sprintf("%d", summary.Old)),
			common.Pluralize(summary.Old, "file", "files"),
			common.SummaryStyle.Render(fmt.Sprintf("%d", summary.Duplicate)),
			common.Pluralize(summary.Duplicate, "duplicate", "duplicates"),
			common.SummaryStyle.Render(fmt.Sprintf("%d", summary.LogCache)),
			common.Pluralize(summary.LogCache, "log/cache", "log/cache"),
		))
		b.WriteString("  " + common.StackedBar(summary, stackedBarWidth) + "\n\n")

		b.WriteString(common.FormatHelpBar(m.width, []string{"[esc] cancel", "[q] quit"}) + "\n")
		return b.String()
	}

	if len(m.hints) == 0 {
		b.WriteString("No hints found.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d hints\n\n", len(m.hints)))

	summary := analyzer.SummarizeHints(m.hints)
	cats := common.SummaryCategories(summary)
	renderCat := func(cat common.SummaryCategory) string {
		s := cat.String()
		if m.analyzerFilter == cat.Reason {
			return common.FilterStyle.Render(s)
		}
		if cat.Value > 0 {
			return common.SummaryStyle.Render(s)
		}
		return s
	}
	parts := make([]string, len(cats))
	for i, cat := range cats {
		parts[i] = renderCat(cat)
	}
	b.WriteString(strings.Join(parts, ", ") + "\n")
	b.WriteString("  " + common.StackedBar(summary, stackedBarWidth) + "\n\n")

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
			style = common.HintOldStyle
		case analyzer.ReasonDuplicate:
			style = common.HintDupStyle
		case analyzer.ReasonLogCache:
			style = common.HintLogStyle
		}
		line := fmt.Sprintf("%-3s %-12s %-15s %s",
			prefix,
			string(h.Reason),
			common.Truncate(h.Detail, 14),
			common.Truncate(h.Entry.Path, 60),
		)
		if i == m.selected {
			line = common.SelectStyle.Render(line)
		} else {
			line = style.Render(line)
		}
		b.WriteString(line + "\n")
	}

	hints := []string{"[j/k/down/up] nav", "[tab/←/→] filter", "[0] clear filter", "[c] clear marks", "[space] mark", "[d] trash marked", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) visibleRangeFor(n int) (int, int) {
	return m.visibleRangeForWithSelected(n, m.selected)
}

func (m *Model) visibleRangeForWithSelected(n, sel int) (int, int) {
	height := 20
	start := sel - height/2
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
	b.WriteString(common.HeaderStyle.Render("Docker Disk Usage"))
	b.WriteString("\n\n")

	if m.dockerErr != nil {
		b.WriteString(common.DangerStyle.Render("Error: "+m.dockerErr.Error()) + "\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if m.dockerUsage == nil {
		b.WriteString(m.spinner.View() + " Loading Docker usage...\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
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
			line = common.SelectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	total := m.dockerUsage.TotalSize()
	b.WriteString(fmt.Sprintf("\nTotal used: %s\n", analyzer.PrettySize(total)))
	if m.dockerMsg != "" {
		b.WriteString(m.dockerMsg + "\n")
	}
	hints := []string{"[↑/↓/j/k] navigate", "[p] prune selected", "[l/enter/tab] item list", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) dockerConfirmView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Confirm Docker prune"))
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
	b.WriteString(common.FormatHelpBar(m.width, []string{"[y] yes", "[n] no"}) + "\n")
	return b.String()
}

func (m *Model) depsView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Dependency Directories"))
	b.WriteString("\n\n")

	if m.depsRunning {
		b.WriteString(m.spinner.View() + " Scanning for dependency directories...\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if m.depsErr != nil {
		b.WriteString(common.DangerStyle.Render("Error: "+m.depsErr.Error()) + "\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if len(m.depsList) == 0 {
		b.WriteString("No dependency directories found.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	if m.depsMsg != "" {
		b.WriteString(m.depsMsg + "\n")
	}

	b.WriteString(fmt.Sprintf("%-3s %-12s %-10s %-16s %-16s %s\n", "", "Type", "Size", "Last Access", "Last Modified", "Path"))
	start, end := m.visibleRangeForWithSelected(len(m.depsList), m.depsSelected)
	for i := start; i < end && i < len(m.depsList); i++ {
		d := m.depsList[i]
		prefix := "[ ]"
		if m.depsMarked[d.Path] {
			prefix = "[x]"
		}
		line := fmt.Sprintf("%-3s %-12s %-10s %-16s %-16s %s",
			prefix,
			d.Type,
			d.PrettySize(),
			d.AccessTime.Format("2006-01-02"),
			d.ModTime.Format("2006-01-02"),
			common.Truncate(d.Path, 60),
		)
		if i == m.depsSelected {
			line = common.SelectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	var total int64
	for _, d := range m.depsList {
		total += d.Size
	}
	b.WriteString(fmt.Sprintf("\nTotal: %s across %d directories\n", analyzer.PrettySize(total), len(m.depsList)))

	hints := []string{"[↑/↓/j/k] navigate", "[space] mark", "[d] trash", "[m] move", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
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

func (m *Model) fetchDeps() tea.Cmd {
	if m.depsRunning {
		return nil
	}
	m.depsRunning = true
	m.depsErr = nil
	m.depsMsg = ""
	return func() tea.Msg {
		scanPaths := m.scanPaths
		if len(scanPaths) == 0 {
			for _, r := range m.roots {
				scanPaths = append(scanPaths, r.Path)
			}
		}
		finder := deps.NewFinder(defaults.DepsTargets(), m.ignorePaths, m.ignoreHidden)
		found, err := finder.Find(context.Background(), scanPaths)
		if err == nil {
			deps.SortResults(found, "size")
		}
		return depsMsg{deps: found, err: err}
	}
}

func (m *Model) handleDepsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = viewFiles
		m.depsMsg = ""
		return m, nil
	case "up", "k":
		if m.depsSelected > 0 {
			m.depsSelected--
		}
	case "down", "j":
		if m.depsSelected < len(m.depsList)-1 {
			m.depsSelected++
		}
	case "pgup", "ctrl+b":
		m.pageDeps(-1)
	case "pgdown", "ctrl+f":
		m.pageDeps(1)
	case " ":
		dep := m.selectedDep()
		if dep != nil {
			m.depsMarked[dep.Path] = !m.depsMarked[dep.Path]
		}
	case "d":
		return m, m.trashMarkedOrSelectedDep()
	case "m":
		return m, m.moveMarkedOrSelectedDep()
	case "r":
		m.depsErr = nil
		m.depsMsg = ""
		return m, m.fetchDeps()
	}
	return m, nil
}

func (m *Model) trashMarkedOrSelectedDep() tea.Cmd {
	paths := m.depsMarkedPaths()
	if len(paths) == 0 {
		dep := m.selectedDep()
		if dep == nil {
			return nil
		}
		paths = []string{dep.Path}
	}
	m.pendingAction = &actionIntent{actionType: "trash", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewDeps}
	m.view = viewConfirmAction
	return nil
}

func (m *Model) moveMarkedOrSelectedDep() tea.Cmd {
	if m.externalDir == "" {
		return func() tea.Msg { return moveMsg{err: fmt.Errorf("no external dir set")} }
	}
	paths := m.depsMarkedPaths()
	if len(paths) == 0 {
		dep := m.selectedDep()
		if dep == nil {
			return nil
		}
		paths = []string{dep.Path}
	}
	m.pendingAction = &actionIntent{actionType: "move", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewDeps}
	m.view = viewConfirmAction
	return nil
}

func (m *Model) depsMarkedPaths() []string {
	var paths []string
	for p, ok := range m.depsMarked {
		if ok {
			paths = append(paths, p)
		}
	}
	return paths
}

func (m *Model) selectedDep() *deps.DependencyDir {
	if m.depsSelected < 0 || m.depsSelected >= len(m.depsList) {
		return nil
	}
	return m.depsList[m.depsSelected]
}

func (m *Model) filterDepsList(paths map[string]bool) {
	var filtered []*deps.DependencyDir
	for _, d := range m.depsList {
		if !paths[d.Path] {
			filtered = append(filtered, d)
		}
	}
	m.depsList = filtered
	if m.depsSelected >= len(m.depsList) {
		if len(m.depsList) == 0 {
			m.depsSelected = 0
		} else {
			m.depsSelected = len(m.depsList) - 1
		}
	}
}

func categoryLabel(item *analyzer.Entry) string {
	if item.IsDir {
		return "Directory"
	}
	return string(item.Category)
}

// Run starts the TUI with the given root entries.
func Run(roots []*analyzer.Entry, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int, cfg *config.Config) error {
	m := New(roots, externalDir, false, dockerClient, dupMode, progressInterval, cfg)
	p := tea.NewProgram(m, tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}
	if cfg != nil {
		return cfg.Save()
	}
	return nil
}

// RunWithScan starts the TUI and scans the given paths in the background.
func RunWithScan(paths []string, ignore []string, ignoreHidden bool, includeVCS bool, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int, cfg *config.Config) error {
	_ = recent.Save(paths)
	ctx, cancel := context.WithCancel(context.Background())
	m := New(nil, externalDir, true, dockerClient, dupMode, progressInterval, cfg)
	m.ignoreHidden = ignoreHidden
	m.ignorePaths = ignore
	m.scanPaths = paths
	p := tea.NewProgram(m, tea.WithMouseCellMotion())
	go func() {
		defer cancel()
		scanner := analyzer.NewScanner(ignore, ignoreHidden, progressInterval, includeVCS)
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
