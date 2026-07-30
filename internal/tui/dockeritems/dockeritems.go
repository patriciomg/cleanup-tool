// Package dockeritems provides a reusable Bubble Tea sub-model for browsing,
// filtering, and acting on individual Docker resources. It is used by both the
// dua-style and terminal-style TUIs to avoid duplicating the Docker item list
// view, key handling, and confirmation dialog.
package dockeritems

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/tui/common"
)

// CloseMsg is emitted when the user wants to leave the docker items view.
// If Quit is true the parent should quit the application; otherwise the
// parent should return to the previous view.
type CloseMsg struct {
	Quit bool
}

// RefreshUsageMsg is emitted after a delete or prune so the parent can refresh
// the Docker usage summary.
type RefreshUsageMsg struct{}

type itemsMsg struct {
	items []docker.DockerItem
	err   error
}

type itemDeleteMsg struct {
	item docker.DockerItem
	err  error
}

type pruneMsg struct {
	reclaimed int64
	err       error
}

// Model is a Bubble Tea sub-model for browsing and acting on Docker items.
type Model struct {
	client         docker.Client
	category       string
	width          int
	height         int
	items          []docker.DockerItem
	selected       int
	filter         string
	groupByProject bool
	itemToDelete   *docker.DockerItem
	confirm        bool
	msg            string
	err            error
}

// New creates a new docker items model for the given client, category, and
// terminal dimensions.
func New(client docker.Client, category string, width, height int) *Model {
	return &Model{
		client:   client,
		category: category,
		width:    width,
		height:   height,
		filter:   "all",
	}
}

// Init fetches the initial list of items.
func (m *Model) Init() tea.Cmd {
	return m.fetch
}

// UpdateModel is a convenience wrapper around Update that returns the concrete
// *Model type, avoiding type assertions in callers.
func (m *Model) UpdateModel(msg tea.Msg) (*Model, tea.Cmd) {
	model, cmd := m.Update(msg)
	return model.(*Model), cmd
}

// Update handles messages for the docker items model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case itemsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.items = msg.items
			m.selected = 0
			m.err = nil
		}
		return m, nil
	case itemDeleteMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Delete failed: " + msg.err.Error()
		} else {
			m.msg = fmt.Sprintf("Deleted %s %s", msg.item.Type, common.Truncate(msg.item.Name, 30))
			m.removeItem(msg.item)
		}
		return m, func() tea.Msg { return RefreshUsageMsg{} }
	case pruneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.msg = "Prune failed: " + msg.err.Error()
		} else {
			m.msg = fmt.Sprintf("Reclaimed %s", analyzer.PrettySize(msg.reclaimed))
			// Keep only non-dangling items in the local list so the UI reflects
			// what was actually deleted without requiring a full refresh.
			var kept []docker.DockerItem
			for _, it := range m.items {
				if !it.Dangling {
					kept = append(kept, it)
				}
			}
			m.items = kept
			m.selected = 0
		}
		return m, func() tea.Msg { return RefreshUsageMsg{} }
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm {
		return m.handleConfirmKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, func() tea.Msg { return CloseMsg{Quit: true} }
	case "esc", "h", "left", "tab":
		return m, func() tea.Msg { return CloseMsg{Quit: false} }
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		items := m.filteredItems()
		if m.selected < len(items)-1 {
			m.selected++
		}
	case "r":
		m.msg = ""
		return m, m.fetch
	case "f":
		m.cycleFilter()
	case "g":
		m.groupByProject = !m.groupByProject
	case "d":
		items := m.filteredItems()
		if m.selected < len(items) {
			item := items[m.selected]
			m.itemToDelete = &item
			m.confirm = true
		}
	case "D":
		return m, m.pruneDangling
	}
	return m, nil
}

func (m *Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.confirm = false
		if m.itemToDelete != nil {
			item := *m.itemToDelete
			m.itemToDelete = nil
			return m, m.deleteItem(item)
		}
		return m, nil
	case "n", "esc":
		m.confirm = false
		m.itemToDelete = nil
		return m, nil
	case "q", "ctrl+c":
		return m, func() tea.Msg { return CloseMsg{Quit: true} }
	}
	return m, nil
}

func (m *Model) fetch() tea.Msg {
	if m.client == nil {
		return itemsMsg{err: fmt.Errorf("docker client not available")}
	}
	items, err := m.client.ListItems(context.Background(), m.category)
	return itemsMsg{items: items, err: err}
}

func (m *Model) deleteItem(item docker.DockerItem) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.DeleteItem(context.Background(), item); err != nil {
			return itemDeleteMsg{item: item, err: err}
		}
		return itemDeleteMsg{item: item}
	}
}

func (m *Model) pruneDangling() tea.Msg {
	var total int64
	for _, it := range m.filteredItems() {
		if !it.Dangling {
			continue
		}
		if err := m.client.DeleteItem(context.Background(), it); err != nil {
			return pruneMsg{err: err}
		}
		total += it.Size
	}
	return pruneMsg{reclaimed: total}
}

func (m *Model) cycleFilter() {
	filters := []string{"all", "dangling", "unused"}
	idx := 0
	for i, f := range filters {
		if f == m.filter {
			idx = i
			break
		}
	}
	m.filter = filters[(idx+1)%len(filters)]
	m.selected = 0
}

func (m *Model) filteredItems() []docker.DockerItem {
	items := m.items
	if m.groupByProject {
		items = make([]docker.DockerItem, len(m.items))
		copy(items, m.items)
		sort.Slice(items, func(i, j int) bool {
			pi, pj := items[i].ProjectKey(), items[j].ProjectKey()
			if pi != pj {
				return pi < pj
			}
			return items[i].Size > items[j].Size
		})
	}
	// "unused" means anything not currently in use. Because dangling items are
	// also not in use, they are included in the "unused" filter.
	switch m.filter {
	case "dangling":
		var out []docker.DockerItem
		for _, it := range items {
			if it.Dangling {
				out = append(out, it)
			}
		}
		return out
	case "unused":
		var out []docker.DockerItem
		for _, it := range items {
			if !it.InUse {
				out = append(out, it)
			}
		}
		return out
	}
	return items
}

func (m *Model) removeItem(item docker.DockerItem) {
	var updated []docker.DockerItem
	for _, it := range m.items {
		if it.ID != item.ID || it.Type != item.Type {
			updated = append(updated, it)
		}
	}
	m.items = updated
	if m.selected >= len(m.items) {
		if len(m.items) == 0 {
			m.selected = 0
		} else {
			m.selected = len(m.items) - 1
		}
	}
}

// statusInfo returns a human-readable status label and an appropriate style
// for the given Docker item. The style encodes deletion safety at a glance:
// green means in-use/keep, red means safe to delete (dangling/stopped), and
// yellow means unused but named (proceed with caution).
func statusInfo(it docker.DockerItem) (string, lipgloss.Style) {
	switch it.Type {
	case "container":
		if it.InUse {
			return "running", common.SizeStyle
		}
		if it.Dangling {
			return "exited", common.DangerStyle
		}
		return "unused", common.FilterStyle
	case "volume":
		if it.InUse {
			return "mounted", common.SizeStyle
		}
		if it.Dangling {
			return "unmounted", common.DangerStyle
		}
		return "unused", common.FilterStyle
	case "image":
		if len(it.UsedBy) > 0 {
			return "in-use", common.SizeStyle
		}
		if it.Dangling {
			return "dangling", common.DangerStyle
		}
		return "unused", common.FilterStyle
	}
	return "unknown", common.BarStyle
}

// safetyText returns a sentence explaining whether it is safe to delete the
// item and why.
func safetyText(it docker.DockerItem) string {
	switch it.Type {
	case "container":
		if it.InUse {
			return "This container is currently running. Stopping and deleting it will terminate the process."
		}
		return "This container is stopped/exited and is safe to delete."
	case "volume":
		if it.InUse {
			return fmt.Sprintf("This volume is mounted by %d container(s). Deleting it may break those containers.", len(it.UsedBy))
		}
		return "This volume is not mounted by any container and is safe to delete."
	case "image":
		if len(it.UsedBy) > 0 {
			return fmt.Sprintf("This image is used by %d container(s). Deleting it may prevent those containers from restarting.", len(it.UsedBy))
		}
		if it.Dangling {
			return "This image is dangling (untagged/orphaned) and is safe to delete."
		}
		return "This image is not currently used by any container, but it may be referenced by name."
	}
	return ""
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

// View renders the docker items list or confirmation dialog.
func (m *Model) View() string {
	if m.confirm {
		return m.confirmView()
	}
	return m.listView()
}

func (m *Model) listView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Docker " + common.Capitalize(m.category)))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(common.DangerStyle.Render("Error: "+m.err.Error()) + "\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[q] quit"}) + "\n")
		return b.String()
	}

	items := m.filteredItems()
	filterLabel := m.filter
	if filterLabel == "" {
		filterLabel = "all"
	}
	groupLabel := "flat"
	if m.groupByProject {
		groupLabel = "by project"
	}
	b.WriteString(fmt.Sprintf("filter: %s | group: %s | %d items\n", filterLabel, groupLabel, len(items)))

	if len(items) == 0 {
		b.WriteString("No items match the current filter.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back", "[f] filter", "[r] refresh", "[q] quit"}) + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("%-30s %-10s %-10s %-20s %s\n", "Name", "Size", "Status", "Project", "ID"))
	start, end := m.visibleRangeWithSelected(len(items), m.selected)
	prevProject := ""
	if start > 0 {
		prevProject = items[start-1].ProjectKey()
	}
	for i := start; i < end && i < len(items); i++ {
		it := items[i]
		status, statusStyle := statusInfo(it)
		project := it.ProjectKey()
		projectDisplay := project
		if m.groupByProject && project == prevProject {
			projectDisplay = "  ↳"
		}
		prevProject = project
		size := analyzer.PrettySize(it.Size)
		if it.Type == "volume" && it.Size == 0 {
			size = "-"
		}
		line := fmt.Sprintf("%-30s %-10s %-10s %-20s %s",
			common.Truncate(it.Name, 29),
			size,
			statusStyle.Render(status),
			common.Truncate(projectDisplay, 19),
			common.Truncate(it.ID, 12),
		)
		if i == m.selected {
			line = common.SelectStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	if m.msg != "" {
		b.WriteString("\n" + m.msg + "\n")
	}

	// Passive details panel for the currently selected item.
	if len(items) > 0 && m.selected < len(items) {
		b.WriteString(m.detailView(items[m.selected]))
	}

	hints := []string{"[↑/↓/j/k] nav", "[d] delete item", "[D] delete all dangling", "[f] filter", "[g] group", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
	return b.String()
}

func (m *Model) detailView(it docker.DockerItem) string {
	var b strings.Builder
	b.WriteString(common.BarStyle.Render(strings.Repeat("─", m.width)) + "\n")
	b.WriteString(fmt.Sprintf("Details:  %s\n", common.HeaderStyle.Render(common.Truncate(it.Name, m.width-10))))
	b.WriteString(fmt.Sprintf("ID:       %s\n", common.Truncate(it.ID, m.width-10)))
	status, _ := statusInfo(it)
	b.WriteString(fmt.Sprintf("Status:   %s\n", status))
	if !it.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf("Created:  %s\n", it.CreatedAt.Local().Format("2006-01-02 15:04:05")))
	}
	b.WriteString(fmt.Sprintf("Project:  %s\n", it.ProjectKey()))
	if it.Size > 0 {
		b.WriteString(fmt.Sprintf("Size:     %s\n", analyzer.PrettySize(it.Size)))
	}
	if len(it.UsedBy) > 0 {
		b.WriteString(fmt.Sprintf("Used by:  %s\n", common.Truncate(strings.Join(it.UsedBy, ", "), m.width-10)))
	}
	if len(it.Labels) > 0 {
		b.WriteString(fmt.Sprintf("Labels:   %d\n", len(it.Labels)))
	}
	return b.String()
}

func (m *Model) confirmView() string {
	var b strings.Builder
	b.WriteString(common.HeaderStyle.Render("Confirm Docker item deletion"))
	b.WriteString("\n\n")

	if m.itemToDelete == nil {
		b.WriteString("No item selected.\n")
		b.WriteString("\n" + common.FormatHelpBar(m.width, []string{"[esc] back"}) + "\n")
		return b.String()
	}

	it := *m.itemToDelete
	b.WriteString(fmt.Sprintf("Delete %s %s (%s)? This cannot be undone.\n\n", it.Type, it.Name, analyzer.PrettySize(it.Size)))
	b.WriteString(common.DangerStyle.Render("Safety: ") + safetyText(it) + "\n\n")
	b.WriteString(common.FormatHelpBar(m.width, []string{"[y] yes", "[n] no"}) + "\n")
	return b.String()
}
