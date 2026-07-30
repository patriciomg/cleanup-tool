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

// viewState distinguishes the main browser, analyzer, and Docker views.
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
	dockerClient           docker.Client
	dockerUsage            *docker.Usage
	dockerSelected         int
	dockerErr              error
	dockerMsg              string
	dockerItems            *dockeritems.Model
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
	llmClient        llm.Client
	llmRegistries    []llm.Registry
	llmSelectedReg   int
	llmSelectedModel int
	llmErr           error
	llmMsg           string

	ignoreHidden bool
	ignorePaths  []string
	scanPaths    []string
	filter       string
	filtering    bool
	sortOrder    string
	undoStack    *undo.Stack
	depsList     []*deps.DependencyDir
	depsSelected int
	depsErr      error
	depsMsg      string
	depsRunning  bool
	depsMarked   map[string]bool

	pendingAction *actionIntent
	cfg           *config.Config
}

// actionIntent holds a trash/move action awaiting user confirmation.
type actionIntent struct {
	actionType string // "trash" or "move"
	paths      []string
	totalSize  int64
	returnView viewState
}

// scanMsg is sent when the background scan finishes.
type scanMsg struct {
	roots []*analyzer.Entry
	err   error
}

// depsMsg is sent when the dependency directory scan finishes.
type depsMsg struct {
	deps []*deps.DependencyDir
	err  error
}

// progressMsg is sent when the scanner reports progress.
type progressMsg struct {
	files int64
	dirs  int64
	path  string
}

// New creates a new dua-style model. If scanning is true the UI starts in the
// scanning state.
func New(scanning bool, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int, cfg *config.Config) *Model {
	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	m := &Model{
		scanning:         scanning,
		spinner:          sp,
		trashed:          make(map[string]bool),
		marked:           make(map[string]bool),
		depsMarked:       make(map[string]bool),
		externalDir:      externalDir,
		dockerClient:     dockerClient,
		llmClient:        llm.NewClient(),
		dupMode:          dupMode,
		progressInterval: progressInterval,
		undoStack:        undo.NewStack(10),
		cfg:              cfg,
	}
	_ = m.undoStack.Load(config.UndoPath())
	m.sortOrder = "size"
	m.applyPreferences()
	if scanning {
		m.scanStart = time.Now()
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
		if m.dockerItems != nil {
			m.dockerItems, _ = m.dockerItems.UpdateModel(msg)
		}
		return m, nil
	case progressMsg:
		m.files = msg.files
		m.dirs = msg.dirs
		m.lastPath = msg.path
		return m, nil
	case scanMsg:
		return m.handleScanResult(msg)
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
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
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
			m.rebuild()
			m.filterDepsList(removed)
			m.msg = fmt.Sprintf("Moved to Trash: %d items", len(msg.paths))
		}
		return m, nil
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
			m.rebuild()
			m.filterDepsList(removed)
			m.msg = fmt.Sprintf("Moved to external: %d items", len(msg.paths))
		}
		return m, nil
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
	case dockeritems.CloseMsg:
		if msg.Quit {
			return m, tea.Quit
		}
		m.view = viewDocker
		return m, nil
	case dockeritems.RefreshUsageMsg:
		return m, m.fetchDockerUsage
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
	common.SortTree(m.roots, m.sortOrder)
	if len(m.roots) > 0 {
		m.current = m.roots[0]
	}
	m.rebuild()

	// Notify the user if the scan took a while.
	if !m.scanStart.IsZero() && m.cfg != nil {
		notifications.ScanComplete(time.Since(m.scanStart), m.cfg.NotificationsEnabled)
	}

	return m, nil
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
		return m.handleDockerConfirmKey(msg)
	case viewDockerItems:
		if m.dockerItems == nil {
			m.view = viewDocker
			return m, nil
		}
		var cmd tea.Cmd
		m.dockerItems, cmd = m.dockerItems.UpdateModel(msg)
		return m, cmd
	case viewDocker:
		return m.handleDockerKey(msg)
	case viewAnalyzer:
		return m.handleAnalyzerKey(msg)
	case viewModels:
		return m.handleModelsKey(msg)
	case viewModelsConfirm:
		return m.handleModelsConfirmKey(msg)
	case viewDeps:
		return m.handleDepsKey(msg)
	case viewConfirmAction:
		return m.handleConfirmActionKey(msg)
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
	case "Z":
		return m.undoLast()
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
		m.saveViewPreference()
		m.dockerSelected = 0
		m.dockerErr = nil
		m.dockerMsg = ""
		m.err = nil
		return m, m.fetchDockerUsage
	case "M":
		m.view = viewModels
		m.saveViewPreference()
		m.llmSelectedReg = 0
		m.llmSelectedModel = 0
		m.llmErr = nil
		m.llmMsg = ""
		m.err = nil
		m.msg = ""
		return m, m.fetchLLMRegistries
	case "P":
		m.view = viewDeps
		m.saveViewPreference()
		m.depsSelected = 0
		m.depsErr = nil
		m.depsMsg = ""
		m.err = nil
		m.msg = ""
		return m, m.fetchDeps()
	case "o":
		if rps, err := recent.Paths(); err == nil && len(rps) > 0 {
			m.msg = common.Truncate("Recent: "+strings.Join(rps, ", "), m.width-4)
		} else {
			m.msg = "No recent paths"
		}
	case "/":
		m.filtering = true
		m.filter = ""
		m.rebuild()
	case "s":
		m.cycleSortOrder()
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
		m.saveViewPreference()
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
			m.saveViewPreference()
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
		m.saveViewPreference()
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
		m.saveAnalyzerFilterPreference()
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
			m.pendingAction = &actionIntent{actionType: "trash", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewAnalyzer}
			m.view = viewConfirmAction
		}
	}
	return m, nil
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

// cycleSortOrder rotates through the available sort orders and rebuilds the
// file list so the change is immediately visible.
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
	m.rebuild()
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
	m.pendingAction = &actionIntent{actionType: "trash", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewFiles}
	m.view = viewConfirmAction
	return m, nil
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
	m.pendingAction = &actionIntent{actionType: "move", paths: paths, totalSize: m.sizeForPaths(paths), returnView: viewFiles}
	m.view = viewConfirmAction
	return m, nil
}

// sizeForPaths sums the recursive sizes of the given paths by looking them up
// in the scanned tree.
func (m *Model) sizeForPaths(paths []string) int64 {
	var total int64
	for _, p := range paths {
		if e := findEntryByPath(m.roots, p); e != nil {
			total += e.Size
		}
	}
	return total
}

func (m *Model) restoreSelected() (tea.Model, tea.Cmd) {
	item := m.selectedItem()
	if item == nil {
		return m, nil
	}
	m.err = nil
	trashDir := filepath.Join(os.Getenv("HOME"), ".Trash")
	trashPath := filepath.Join(trashDir, filepath.Base(item.Path))
	return m, func() tea.Msg {
		if err := actions.Restore(trashPath, item.Path); err != nil {
			return restoreMsg{paths: []string{item.Path}, err: err}
		}
		return restoreMsg{paths: []string{item.Path}}
	}
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
	children := m.current.Children
	if m.filter != "" {
		f := strings.ToLower(m.filter)
		filtered := make([]*analyzer.Entry, 0, len(children))
		for _, c := range children {
			if strings.Contains(strings.ToLower(c.Name), f) {
				filtered = append(filtered, c)
			}
		}
		children = filtered
	}
	m.items = m.sortedChildren(children)
	if m.selected >= len(m.items) {
		if len(m.items) == 0 {
			m.selected = 0
		} else {
			m.selected = len(m.items) - 1
		}
	}
}

// sortedChildren returns children sorted according to the model's sortOrder.
// Trashed items are not removed so they can be restored; they are styled
// differently in the view.
func (m *Model) sortedChildren(children []*analyzer.Entry) []*analyzer.Entry {
	out := make([]*analyzer.Entry, len(children))
	copy(out, children)
	switch m.sortOrder {
	case "name":
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	case "access":
		sort.Slice(out, func(i, j int) bool {
			return out[i].AccessTime.After(out[j].AccessTime)
		})
	case "modified":
		sort.Slice(out, func(i, j int) bool {
			return out[i].ModTime.After(out[j].ModTime)
		})
	default:
		sort.Slice(out, func(i, j int) bool {
			return out[i].Size > out[j].Size
		})
	}
	return out
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.err != nil {
		return common.DangerStyle.Render("Error: "+m.err.Error()) + "\nq to quit\n"
	}
	if m.scanning {
		return m.scanView()
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
	case viewModelsConfirm:
		return m.modelsConfirmView()
	case viewDeps:
		return m.depsView()
	case viewConfirmAction:
		return m.confirmActionView()
	}
	if m.showHelp {
		return m.helpView()
	}

	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Dua-style Browser"))
	if m.current != nil {
		b.WriteString(fmt.Sprintf("  %s  total %s\n", common.CurrentPathStyle.Render(common.Truncate(m.current.Path, 60)), analyzer.PrettySize(m.current.Size)))
	}
	if m.msg != "" {
		b.WriteString(common.MsgStyle.Render(m.msg) + "\n")
	}
	if m.filtering || m.filter != "" {
		cursor := ""
		if m.filtering {
			cursor = "_"
		}
		b.WriteString(fmt.Sprintf("Filter: /%s%s\n", m.filter, cursor))
	}
	b.WriteString("\n")

	if len(m.items) == 0 {
		if m.filter != "" {
			b.WriteString("No items match your filter.\n")
		} else {
			b.WriteString("No items.\n")
		}
		b.WriteString(common.FormatHelpBar(m.width, []string{"[?] help", "[q] quit"}))
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
			line = common.TrashedStyle.Render(line)
		} else if m.marked[item.Path] {
			line = common.MarkedStyle.Render(line)
		} else if i == m.selected {
			line = common.SelectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	hints := []string{
		"[j/k/↓/↑] nav", "[enter/l] descend", "[backspace/h/u] up",
		"[d] mark", "[x] trash", "[m] move", "[r] restore", "[Z] undo",
		"[a] analyze dir", "[A] analyze selection", "[P] deps", "[D] Docker", "[M] Models", "[o] recent", "[/] filter", "[s] sort", "[c] clear", "[?] help", "[q] quit",
	}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) scanView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Dua-style Browser — scanning..."))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " ")
	b.WriteString(fmt.Sprintf("Files: %d  Dirs: %d\n", m.files, m.dirs))
	if !m.scanStart.IsZero() {
		secs := time.Since(m.scanStart).Seconds()
		if secs > 0 {
			b.WriteString(fmt.Sprintf("Speed: %.0f files/sec  %.0f dirs/sec\n", float64(m.files)/secs, float64(m.dirs)/secs))
		}
	}
	b.WriteString(fmt.Sprintf("Last: %s\n", common.Truncate(m.lastPath, 60)))
	b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[q] quit"}) + "\n")
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
		"  s            cycle sort order (size, name, access, modified)",
		"  x            preview trash for marked items (or selected)",
		"  m            preview move for marked items (or selected) to external drive",
		"  r            restore selected item from Trash",
		"  Z            undo last trash/move operation",
		"  c            clear all marks",
		"  a            analyze current directory",
		"  A            analyze selected/marked items",
		"  P            show dependency directories (node_modules, vendor, etc.)",
		"  D            show Docker disk usage",
		"  M            show LLM model registries",
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
	return common.HelpBoxStyle.Width(boxWidth).Render(b.String())
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
		if m.msg != "" {
			b.WriteString("\n" + common.MsgStyle.Render(m.msg) + "\n")
		}
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d hints\n\n", len(m.hints)))
	if m.msg != "" {
		b.WriteString(common.MsgStyle.Render(m.msg) + "\n")
	}

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
			line = common.SelectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	total := m.dockerUsage.TotalSize()
	b.WriteString(fmt.Sprintf("\nTotal used: %s\n", analyzer.PrettySize(total)))
	if m.msg != "" {
		b.WriteString(common.MsgStyle.Render(m.msg) + "\n")
	}
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

	items := []string{"Images", "Containers", "Volumes", "Build Cache"}
	selected := items[m.dockerSelected]
	b.WriteString(fmt.Sprintf("Prune %s? This action cannot be undone.\n\n", selected))
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
		b.WriteString(common.MsgStyle.Render(m.depsMsg) + "\n")
	}

	b.WriteString(fmt.Sprintf("%-3s %-12s %-10s %-16s %-16s %s\n", "", "Type", "Size", "Last Access", "Last Modified", "Path"))
	start, end := m.visibleRangeWithSelected(len(m.depsList), m.depsSelected)
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

	hints := []string{"[↑/↓/j/k] navigate", "[d] mark", "[x] trash", "[m] move", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) visibleRange(n int) (int, int) {
	return m.visibleRangeWithSelected(n, m.selected)
}

func (m *Model) visibleRangeWithSelected(n, sel int) (int, int) {
	height := m.height - 6
	if height < 8 {
		height = 8
	}
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
	return common.BarStyle.Render(strings.Repeat("█", w)) + strings.Repeat("░", width-w)
}

func center(s string, width int) string {
	if len(s) >= width {
		return s
	}
	pad := (width - len(s)) / 2
	return strings.Repeat(" ", pad) + s
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
	case "d":
		dep := m.selectedDep()
		if dep != nil {
			m.depsMarked[dep.Path] = !m.depsMarked[dep.Path]
		}
	case "x":
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

// --- LLM model registry ---

type llmMsg struct {
	registries []llm.Registry
	err        error
}

type llmDeleteMsg struct {
	registry string
	model    string
	err      error
}

func (m *Model) fetchLLMRegistries() tea.Msg {
	if m.llmClient == nil {
		return llmMsg{err: fmt.Errorf("llm client not available")}
	}
	regs, err := m.llmClient.GetRegistries(context.Background())
	return llmMsg{registries: regs, err: err}
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
		m.llmErr = nil
		m.llmMsg = ""
		return m, m.fetchLLMRegistries
	}
	return m, nil
}

func (m *Model) handleModelsConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		b.WriteString("\n" + common.MsgStyle.Render(m.llmMsg) + "\n")
	}
	hints := []string{"[↑/↓/j/k] navigate", "[d] delete model", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
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

func (m *Model) modelsConfirmView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Confirm model deletion"))
	b.WriteString("\n\n")

	reg, model, ok := m.selectedModel()
	if !ok {
		b.WriteString("No model selected.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Delete %s from %s (%s)? This cannot be undone.\n\n",
		model.Name, reg.Name, analyzer.PrettySize(model.Size)))
	b.WriteString(common.FormatHelpBar(m.width, []string{"[y] yes", "[n] no"}) + "\n")
	return b.String()
}

func findEntryByPath(roots []*analyzer.Entry, target string) *analyzer.Entry {
	for _, root := range roots {
		if e := analyzer.FindEntryByPath(root, target); e != nil {
			return e
		}
	}
	return nil
}



// RunWithScan starts the dua-style TUI and scans the given paths in the
// background. progressInterval controls how often scan progress is reported
// (a value <= 0 disables progress reports).
func RunWithScan(paths []string, ignore []string, ignoreHidden bool, includeVCS bool, externalDir string, dockerClient docker.Client, dupMode analyzer.DupHashMode, progressInterval int, cfg *config.Config) error {
	_ = recent.Save(paths)
	ctx, cancel := context.WithCancel(context.Background())
	m := New(true, externalDir, dockerClient, dupMode, progressInterval, cfg)
	m.ignoreHidden = ignoreHidden
	m.ignorePaths = ignore
	m.scanPaths = paths
	p := tea.NewProgram(m)
	go func() {
		defer cancel()
		scanner := analyzer.NewScanner(ignore, ignoreHidden, progressInterval, includeVCS)
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
	if cfg != nil {
		if err := cfg.Save(); err != nil {
			return err
		}
	}
	return nil
}
