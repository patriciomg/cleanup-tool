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
	for i := start; i < end && i < len(items); i++ {
		it := items[i]
		status := "used"
		if it.InUse {
			status = "running"
		} else if it.Dangling {
			status = "dangling"
		}
		project := it.Project
		if project == "" {
			project = "-"
		}
		size := analyzer.PrettySize(it.Size)
		if it.Type == "volume" && it.Size == 0 {
			size = "-"
		}
		line := fmt.Sprintf("%-30s %-10s %-10s %-20s %s",
			common.Truncate(it.Name, 29),
			size,
			status,
			common.Truncate(project, 19),
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
	hints := []string{"[↑/↓/j/k] nav", "[d] delete item", "[D] delete all dangling", "[f] filter", "[g] group", "[r] refresh", "[esc] back", "[q] quit"}
	b.WriteString("\n" + common.FormatHelpBar(m.width, hints) + "\n")
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
	b.WriteString(common.FormatHelpBar(m.width, []string{"[y] yes", "[n] no"}) + "\n")
	return b.String()
}
