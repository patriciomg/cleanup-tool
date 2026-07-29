package dockeritems

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/tui/tuitest"
)

// assertItemIDs asserts that the given items have exactly the expected IDs in
// the given order.
func assertItemIDs(t *testing.T, items []docker.DockerItem, want ...string) {
	t.Helper()
	if len(items) != len(want) {
		t.Fatalf("expected %d items, got %d: %v", len(want), len(items), items)
	}
	for i, it := range items {
		if it.ID != want[i] {
			t.Fatalf("expected item %d to have ID %q, got %q (items: %v)", i, want[i], it.ID, items)
		}
	}
}

// assertCmdReturnsMsg asserts that cmd is non-nil and returns a message of the
// expected type. It returns the typed message so callers can perform additional
// assertions.
func assertCmdReturnsMsg[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	var zero T
	if cmd == nil {
		t.Fatalf("expected command returning %T, got nil", zero)
	}
	msg := cmd()
	typed, ok := msg.(T)
	if !ok {
		t.Fatalf("expected %T, got %T", zero, msg)
	}
	return typed
}

// sendRefresh presses 'r' and executes the resulting refresh command.
func sendRefresh(t *testing.T, m *Model) *Model {
	t.Helper()
	m, refreshCmd := tuitest.SendKey(m, 'r')
	if refreshCmd == nil {
		t.Fatal("expected refresh command after pressing 'r'")
	}
	m, _ = tuitest.Send(m, refreshCmd())
	return m
}

// triggerDeleteError selects the current item, confirms deletion, and executes
// the delete command while the mock is configured to fail. It returns the
// updated model and the RefreshUsageMsg command emitted by the model.
func triggerDeleteError(t *testing.T, m *Model, wantErr error) (*Model, tea.Cmd) {
	t.Helper()
	m, _ = tuitest.SendKey(m, 'd')
	if !m.confirm {
		t.Fatal("expected confirm state after pressing 'd'")
	}
	m, delCmd := tuitest.SendKey(m, 'y')
	if delCmd == nil {
		t.Fatal("expected delete command after confirming")
	}
	if m.confirm {
		t.Fatal("expected confirm state to be cleared after confirming delete")
	}
	if m.itemToDelete != nil {
		t.Fatalf("expected itemToDelete to be cleared after confirming delete, got %v", m.itemToDelete)
	}
	m, refreshCmd := tuitest.Send(m, delCmd())
	if m.err != wantErr {
		t.Fatalf("expected err %v after delete, got %v", wantErr, m.err)
	}
	return m, refreshCmd
}

func TestFilteredItems(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m.items = []docker.DockerItem{
		{Type: "image", ID: "a", Name: "used-img", InUse: true},
		{Type: "image", ID: "b", Name: "dangling-img", Dangling: true},
		{Type: "image", ID: "c", Name: "unused-img"},
	}

	m.filter = "all"
	if got := len(m.filteredItems()); got != 3 {
		t.Fatalf("expected 3 items with 'all' filter, got %d", got)
	}

	m.filter = "dangling"
	assertItemIDs(t, m.filteredItems(), "b")

	m.filter = "unused"
	if got := len(m.filteredItems()); got != 2 {
		t.Fatalf("expected 2 unused items (dangling + truly unused), got %d", got)
	}
}

func TestCycleFilter(t *testing.T) {
	m := New(nil, "images", 80, 24)
	if m.filter != "all" {
		t.Fatalf("expected initial filter 'all', got %s", m.filter)
	}

	expected := []string{"dangling", "unused", "all"}
	for _, want := range expected {
		m.cycleFilter()
		if m.filter != want {
			t.Fatalf("expected filter %s, got %s", want, m.filter)
		}
	}
}

// TestCycleFilterViaKeyChangesFilteredItems verifies that cycling the filter
// (legacy name filteredDockerItems, now filteredItems) via the 'f' key updates
// the filter state and the filtered item list correctly.
func TestCycleFilterViaKeyChangesFilteredItems(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m.items = []docker.DockerItem{
		{Type: "image", ID: "u", Name: "used", InUse: true},
		{Type: "image", ID: "d", Name: "dangling", Dangling: true},
		{Type: "image", ID: "n", Name: "unused"},
	}

	if m.filter != "all" || len(m.filteredItems()) != 3 {
		t.Fatalf("expected initial 'all' filter with 3 items, got filter=%s items=%d", m.filter, len(m.filteredItems()))
	}

	m, _ = tuitest.SendKey(m, 'f')
	if m.filter != "dangling" {
		t.Fatalf("expected filter 'dangling' after first cycle, got %s", m.filter)
	}
	if m.selected != 0 {
		t.Fatalf("expected selected reset to 0 after filter cycle, got %d", m.selected)
	}
	assertItemIDs(t, m.filteredItems(), "d")

	m, _ = tuitest.SendKey(m, 'f')
	if m.filter != "unused" {
		t.Fatalf("expected filter 'unused' after second cycle, got %s", m.filter)
	}
	if m.selected != 0 {
		t.Fatalf("expected selected reset to 0 after filter cycle, got %d", m.selected)
	}
	// "unused" includes anything not in use, which also covers dangling items.
	assertItemIDs(t, m.filteredItems(), "d", "n")

	m, _ = tuitest.SendKey(m, 'f')
	if m.filter != "all" {
		t.Fatalf("expected filter 'all' after third cycle, got %s", m.filter)
	}
	if m.selected != 0 {
		t.Fatalf("expected selected reset to 0 after filter cycle, got %d", m.selected)
	}
	if got := len(m.filteredItems()); got != 3 {
		t.Fatalf("expected 3 items with 'all' filter, got %d", got)
	}
}

func TestGroupByProject(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m.items = []docker.DockerItem{
		{Type: "image", ID: "a", Name: "z", Project: "beta"},
		{Type: "image", ID: "b", Name: "a", Project: "alpha"},
	}
	m.groupByProject = true
	filtered := m.filteredItems()
	if filtered[0].Project != "alpha" {
		t.Fatalf("expected grouping to sort alpha first, got %v", filtered)
	}
}

func TestGroupByProjectKey(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m.items = []docker.DockerItem{
		{Type: "image", ID: "z", Name: "z", Project: "beta", Size: 10},
		{Type: "image", ID: "b", Name: "b", Project: "alpha", Size: 20},
		{Type: "image", ID: "a", Name: "a", Project: "alpha", Size: 5},
	}

	// Initially flat order (insertion order).
	assertItemIDs(t, m.filteredItems(), "z", "b", "a")

	// Toggle grouping on with 'g'.
	m, _ = tuitest.SendKey(m, 'g')
	if !m.groupByProject {
		t.Fatal("expected groupByProject to be true after pressing 'g'")
	}
	if !strings.Contains(m.View(), "group: by project") {
		t.Fatal("expected view to show 'group: by project'")
	}
	// Alpha project first; within alpha, larger size first, then beta item.
	assertItemIDs(t, m.filteredItems(), "b", "a", "z")

	// Toggle grouping off with 'g' again.
	m, _ = tuitest.SendKey(m, 'g')
	if m.groupByProject {
		t.Fatal("expected groupByProject to be false after pressing 'g' again")
	}
	if !strings.Contains(m.View(), "group: flat") {
		t.Fatal("expected view to show 'group: flat' after toggling off")
	}
	assertItemIDs(t, m.filteredItems(), "z", "b", "a")
}

func TestGroupByProjectWithFilter(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m.items = []docker.DockerItem{
		{Type: "image", ID: "alpha-used", Name: "alpha-used", Project: "alpha", Size: 100, InUse: true},
		{Type: "image", ID: "beta-dangle", Name: "beta-dangle", Project: "beta", Size: 200, Dangling: true},
		{Type: "image", ID: "alpha-dangle", Name: "alpha-dangle", Project: "alpha", Size: 50, Dangling: true},
		{Type: "image", ID: "beta-unused", Name: "beta-unused", Project: "beta", Size: 30},
		{Type: "image", ID: "gamma-used", Name: "gamma-used", Project: "gamma", Size: 10, InUse: true},
	}

	// Enable grouping while the filter is "all".
	m, _ = tuitest.SendKey(m, 'g')
	if !m.groupByProject {
		t.Fatal("expected groupByProject to be true")
	}

	// Cycle to the "dangling" filter.
	m, _ = tuitest.SendKey(m, 'f')
	if m.filter != "dangling" {
		t.Fatalf("expected filter 'dangling', got %s", m.filter)
	}
	dangling := m.filteredItems()
	if len(dangling) != 2 {
		t.Fatalf("expected 2 dangling items, got %v", dangling)
	}
	// Grouped by project: alpha first, then beta; within each project sorted by size descending.
	assertItemIDs(t, dangling, "alpha-dangle", "beta-dangle")
	if !strings.Contains(m.View(), "filter: dangling | group: by project") {
		t.Fatal("expected view to show 'filter: dangling | group: by project'")
	}

	// Cycle to the "unused" filter (includes dangling + truly unused items).
	m, _ = tuitest.SendKey(m, 'f')
	if m.filter != "unused" {
		t.Fatalf("expected filter 'unused', got %s", m.filter)
	}
	unused := m.filteredItems()
	if len(unused) != 3 {
		t.Fatalf("expected 3 unused items, got %v", unused)
	}
	// alpha group first, then beta; within beta larger size first.
	assertItemIDs(t, unused, "alpha-dangle", "beta-dangle", "beta-unused")
	if !strings.Contains(m.View(), "filter: unused | group: by project") {
		t.Fatal("expected view to show 'filter: unused | group: by project'")
	}

	// Disable grouping while still under the unused filter.
	m, _ = tuitest.SendKey(m, 'g')
	if m.groupByProject {
		t.Fatal("expected groupByProject to be false after pressing 'g' again")
	}
	flat := m.filteredItems()
	if len(flat) != 3 {
		t.Fatalf("expected 3 unused items when flat, got %v", flat)
	}
	// Flat order is the original insertion order filtered to unused.
	assertItemIDs(t, flat, "beta-dangle", "alpha-dangle", "beta-unused")
	if !strings.Contains(m.View(), "filter: unused | group: flat") {
		t.Fatal("expected view to show 'filter: unused | group: flat'")
	}
}

func TestDeleteKeyError(t *testing.T) {
	wantErr := errors.New("delete failed")
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
		DeleteFunc: func(_ docker.DockerItem) error {
			return wantErr
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}

	m, refreshCmd := triggerDeleteError(t, m, wantErr)
	_ = assertCmdReturnsMsg[RefreshUsageMsg](t, refreshCmd)

	wantMsg := "Delete failed: " + wantErr.Error()
	if m.msg != wantMsg {
		t.Fatalf("expected delete error message %q, got %q", wantMsg, m.msg)
	}
	// The item should remain in the list because deletion failed.
	assertItemIDs(t, m.items, "abc")
}

func TestDeleteKeyFlow(t *testing.T) {
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
	}
	m := New(mock, "images", 80, 24)
	// Load items into the model.
	m, _ = tuitest.Send(m, m.Init()())

	// Press 'd' to select the item for deletion.
	m, _ = m.UpdateModel(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.confirm {
		t.Fatal("expected confirm state after pressing 'd'")
	}

	// Press 'y' to confirm; a delete command should be returned.
	_, delCmd := tuitest.SendKey(m, 'y')
	if delCmd == nil {
		t.Fatal("expected delete command after confirming")
	}
	// The command performs the deletion; feeding its message back to the model
	// should produce a RefreshUsageMsg for the parent.
	_, refreshCmd := m.UpdateModel(delCmd())
	_ = assertCmdReturnsMsg[RefreshUsageMsg](t, refreshCmd)
	if len(mock.Deleted) != 1 || mock.Deleted[0].ID != "abc" {
		t.Fatalf("expected mock to record deletion, got %v", mock.Deleted)
	}
}

func TestPruneDangling(t *testing.T) {
	item := docker.DockerItem{Type: "image", ID: "d", Name: "dangling", Dangling: true}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
	}
	m := New(mock, "images", 80, 24)
	m, _ = tuitest.Send(m, m.Init()())

	_, cmd := tuitest.SendKey(m, 'D')
	if cmd == nil {
		t.Fatal("expected prune command after pressing D")
	}
	_, refreshCmd := m.UpdateModel(cmd())
	_ = assertCmdReturnsMsg[RefreshUsageMsg](t, refreshCmd)
	if len(mock.Deleted) != 1 || mock.Deleted[0].ID != "d" {
		t.Fatalf("expected dangling image to be deleted, got %v", mock.Deleted)
	}
}

func TestPruneDanglingKeepsNonDangling(t *testing.T) {
	used := docker.DockerItem{Type: "image", ID: "u", Name: "used", InUse: true, Size: 100}
	danglingA := docker.DockerItem{Type: "image", ID: "d1", Name: "dangling-a", Dangling: true, Size: 50}
	danglingB := docker.DockerItem{Type: "image", ID: "d2", Name: "dangling-b", Dangling: true, Size: 30}
	unused := docker.DockerItem{Type: "image", ID: "n", Name: "unused", Size: 20}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {used, danglingA, danglingB, unused},
		},
	}

	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}
	if len(m.items) != 4 {
		t.Fatalf("expected 4 items before prune, got %d", len(m.items))
	}

	// Press 'D' to prune all dangling items.
	m, pruneCmd := tuitest.SendKey(m, 'D')
	if pruneCmd == nil {
		t.Fatal("expected prune command after pressing D")
	}

	// Feed the prune result back into the model.
	m, refreshCmd := tuitest.Send(m, pruneCmd())

	// Only the dangling items should have been passed to the client for deletion.
	if len(mock.Deleted) != 2 {
		t.Fatalf("expected 2 dangling items to be deleted, got %v", mock.Deleted)
	}
	deleted := map[string]bool{}
	for _, it := range mock.Deleted {
		deleted[it.ID] = true
	}
	if !deleted["d1"] || !deleted["d2"] {
		t.Fatalf("expected d1 and d2 to be deleted, got %v", mock.Deleted)
	}

	// The local list should now contain only the used and truly-unused items.
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items after prune, got %d", len(m.items))
	}
	assertItemIDs(t, m.items, "u", "n")
	if m.selected != 0 {
		t.Fatalf("expected selected reset to 0 after prune, got %d", m.selected)
	}
	if m.err != nil {
		t.Fatalf("expected no error after prune, got %v", m.err)
	}
	wantReclaimed := analyzer.PrettySize(danglingA.Size + danglingB.Size)
	if !strings.Contains(m.msg, "Reclaimed") || !strings.Contains(m.msg, wantReclaimed) {
		t.Fatalf("expected success message to mention reclaimed size %s, got %q", wantReclaimed, m.msg)
	}

	// A refresh command should be emitted so parents update usage.
	_ = assertCmdReturnsMsg[RefreshUsageMsg](t, refreshCmd)
}

func TestCloseMsg(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m, cmd := tuitest.Send(m, tuitest.Esc())
	_ = assertCmdReturnsMsg[CloseMsg](t, cmd)
}

func TestWindowSizePropagation(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m, _ = tuitest.Send(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.width != 100 || m.height != 40 {
		t.Fatalf("expected width=100 height=40, got %dx%d", m.width, m.height)
	}
}

func TestInitialFetchError(t *testing.T) {
	wantErr := errors.New("initial fetch failed")
	mock := &docker.MockClient{
		ItemsErr: wantErr,
	}
	m := New(mock, "images", 80, 24)
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("expected initial fetch command")
	}

	m, _ = tuitest.Send(m, initCmd())
	if m.err != wantErr {
		t.Fatalf("expected err %v after initial fetch, got %v", wantErr, m.err)
	}
	if len(m.items) != 0 {
		t.Fatalf("expected items to be empty after initial fetch error, got %v", m.items)
	}
	if !strings.Contains(m.View(), "initial fetch failed") {
		t.Fatal("expected view to show the initial fetch error")
	}
}

func TestRefreshClearsPreviousError(t *testing.T) {
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {{Type: "image", ID: "old", Name: "old"}},
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}
	assertItemIDs(t, m.items, "old")

	// First refresh fails.
	wantErr := errors.New("docker boom")
	mock.ItemsErr = wantErr
	m = sendRefresh(t, m)
	if m.err != wantErr {
		t.Fatalf("expected err %v after refresh, got %v", wantErr, m.err)
	}

	// Second refresh succeeds and clears the error.
	mock.ItemsErr = nil
	mock.Items["images"] = []docker.DockerItem{{Type: "image", ID: "new", Name: "new"}}
	m = sendRefresh(t, m)
	if m.err != nil {
		t.Fatalf("expected err to be cleared after successful refresh, got %v", m.err)
	}
	if m.selected != 0 {
		t.Fatalf("expected selected reset to 0 after successful refresh, got %d", m.selected)
	}
	assertItemIDs(t, m.items, "new")
}

func TestRefreshClearsStaleMessageAfterError(t *testing.T) {
	wantErr := errors.New("delete failed")
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
		DeleteFunc: func(_ docker.DockerItem) error {
			return wantErr
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}
	assertItemIDs(t, m.items, "abc")

	// Trigger a delete error, leaving a stale message and error on the model.
	m, _ = triggerDeleteError(t, m, wantErr)
	if !strings.Contains(m.msg, "Delete failed") {
		t.Fatalf("expected delete error message, got %q", m.msg)
	}

	// A successful refresh should clear both the message and the error.
	mock.DeleteFunc = nil
	m = sendRefresh(t, m)

	if m.err != nil {
		t.Fatalf("expected err to be cleared after successful refresh, got %v", m.err)
	}
	if m.msg != "" {
		t.Fatalf("expected stale message to be cleared after successful refresh, got %q", m.msg)
	}
	assertItemIDs(t, m.items, "abc")
}

func TestRefreshClearsStaleMessageAfterPruneError(t *testing.T) {
	wantErr := errors.New("prune failed")
	item := docker.DockerItem{Type: "image", ID: "d", Name: "dangling", Dangling: true}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
		DeleteFunc: func(_ docker.DockerItem) error {
			return wantErr
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}
	assertItemIDs(t, m.items, "d")

	// Trigger a prune error, leaving a stale message and error on the model.
	m, pruneCmd := tuitest.SendKey(m, 'D')
	if pruneCmd == nil {
		t.Fatal("expected prune command after pressing D")
	}
	m, refreshCmd := tuitest.Send(m, pruneCmd())
	_ = assertCmdReturnsMsg[RefreshUsageMsg](t, refreshCmd)
	if m.err != wantErr {
		t.Fatalf("expected err %v after prune, got %v", wantErr, m.err)
	}
	wantMsg := "Prune failed: " + wantErr.Error()
	if m.msg != wantMsg {
		t.Fatalf("expected prune error message %q, got %q", wantMsg, m.msg)
	}
	// The dangling item should remain because prune failed.
	assertItemIDs(t, m.items, "d")

	// A successful refresh should clear both the message and the error.
	mock.DeleteFunc = nil
	m = sendRefresh(t, m)
	if m.err != nil {
		t.Fatalf("expected err to be cleared after successful refresh, got %v", m.err)
	}
	if m.msg != "" {
		t.Fatalf("expected stale message to be cleared after successful refresh, got %q", m.msg)
	}
	assertItemIDs(t, m.items, "d")
}

func TestViewNoLongerShowsDeleteErrorAfterRefresh(t *testing.T) {
	wantErr := errors.New("delete failed")
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
		DeleteFunc: func(_ docker.DockerItem) error {
			return wantErr
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}

	m, _ = triggerDeleteError(t, m, wantErr)

	// The view should show the error.
	if !strings.Contains(m.View(), "Error: delete failed") {
		t.Fatal("expected view to show the delete error")
	}

	// A successful refresh should remove the error from the view.
	mock.DeleteFunc = nil
	m = sendRefresh(t, m)
	if m.err != nil {
		t.Fatalf("expected err to be cleared after successful refresh, got %v", m.err)
	}
	if m.msg != "" {
		t.Fatalf("expected stale message to be cleared after successful refresh, got %q", m.msg)
	}
	if strings.Contains(m.View(), "Error: delete failed") {
		t.Fatal("expected view to no longer show the delete error")
	}
}

func TestRefreshKeyError(t *testing.T) {
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {{Type: "image", ID: "old", Name: "old"}},
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}
	assertItemIDs(t, m.items, "old")

	wantErr := errors.New("docker boom")
	mock.ItemsErr = wantErr
	m = sendRefresh(t, m)
	if m.err != wantErr {
		t.Fatalf("expected err %v after refresh, got %v", wantErr, m.err)
	}
	// The previous items are preserved while the error is displayed.
	assertItemIDs(t, m.items, "old")
	if !strings.Contains(m.View(), "docker boom") {
		t.Fatal("expected view to show the error message")
	}
}

func TestQuitKeyEmitsCloseMsg(t *testing.T) {
	m := New(nil, "images", 80, 24)
	m, cmd := tuitest.SendKey(m, 'q')
	closeMsg := assertCmdReturnsMsg[CloseMsg](t, cmd)
	if !closeMsg.Quit {
		t.Fatalf("expected CloseMsg.Quit to be true, got %v", closeMsg)
	}
}

func TestDeleteCancelFlow(t *testing.T) {
	item := docker.DockerItem{Type: "image", ID: "abc", Name: "img"}
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {item},
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}

	m, _ = tuitest.SendKey(m, 'd')
	if !m.confirm {
		t.Fatal("expected confirm state after pressing 'd'")
	}
	if !strings.Contains(m.View(), "Confirm Docker item deletion") {
		t.Fatal("expected confirmation view to be shown after pressing 'd'")
	}
	if m.itemToDelete == nil || m.itemToDelete.ID != "abc" {
		t.Fatalf("expected itemToDelete to be set to abc, got %v", m.itemToDelete)
	}

	m, cmd := tuitest.SendKey(m, 'n')
	if cmd != nil {
		t.Fatal("expected no command after pressing 'n'")
	}
	if m.confirm {
		t.Fatal("expected confirm state to be dismissed after pressing 'n'")
	}
	if m.itemToDelete != nil {
		t.Fatalf("expected itemToDelete to be nil after cancel, got %v", m.itemToDelete)
	}
	if strings.Contains(m.View(), "Confirm Docker item deletion") {
		t.Fatal("expected confirmation view to be dismissed after pressing 'n'")
	}
	if len(mock.Deleted) != 0 {
		t.Fatalf("expected no deletion after cancel, got %v", mock.Deleted)
	}
}

func TestRefreshKeyReloadsItems(t *testing.T) {
	mock := &docker.MockClient{
		Items: map[string][]docker.DockerItem{
			"images": {{Type: "image", ID: "old", Name: "old"}},
		},
	}
	m := New(mock, "images", 80, 24)
	if initCmd := m.Init(); initCmd != nil {
		m, _ = tuitest.Send(m, initCmd())
	}
	assertItemIDs(t, m.items, "old")

	// Replace the mock data, set a stale message, and press 'r' to refresh.
	mock.Items["images"] = []docker.DockerItem{{Type: "image", ID: "new", Name: "new"}}
	m.msg = "stale"
	m.selected = 42
	m = sendRefresh(t, m)
	if m.err != nil {
		t.Fatalf("expected no error after refresh, got %v", m.err)
	}
	if m.msg != "" {
		t.Fatalf("expected stale message to be cleared, got %q", m.msg)
	}
	if m.selected != 0 {
		t.Fatalf("expected selected reset to 0 after refresh, got %d", m.selected)
	}
	assertItemIDs(t, m.items, "new")
}
