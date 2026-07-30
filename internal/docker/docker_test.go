package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"0B", 0},
		{"1.5KB", 1536},
		{"2MB", 2 * 1024 * 1024},
		{"1.5GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"", 0},
	}
	for _, c := range cases {
		got := parseSize(c.input)
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestParseReclaimed(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"Total reclaimed space: 1.234GB", 1324997410},
		{"Total reclaimed space: 0B", 0},
		{"Total reclaimed space: 567MB", 594542592},
	}
	for _, c := range cases {
		got := parseReclaimed(c.input)
		if got != c.want {
			t.Errorf("parseReclaimed(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestUsageTotalSize(t *testing.T) {
	u := &Usage{
		Images:     ResourceUsage{Size: 100, Reclaimable: 50},
		Containers: ResourceUsage{Size: 20, Reclaimable: 20},
		Volumes:    ResourceUsage{Size: 30, Reclaimable: 30},
		BuildCache: ResourceUsage{Size: 40, Reclaimable: 40},
	}
	if got := u.TotalSize(); got != 190 {
		t.Errorf("TotalSize() = %d, want 190", got)
	}
	if got := u.TotalReclaimable(); got != 140 {
		t.Errorf("TotalReclaimable() = %d, want 140", got)
	}
}

func TestParseSizeFloat(t *testing.T) {
	got := parseSize("1.5GB")
	want := int64(1610612736)
	if got != want {
		t.Errorf("parseSize(\"1.5GB\") = %d, want %d", got, want)
	}
}

func TestImageName(t *testing.T) {
	cases := []struct {
		repo, tag, want string
	}{
		{"nginx", "latest", "nginx"},
		{"nginx", "1.0", "nginx:1.0"},
		{"<none>", "<none>", "<dangling>"},
		{"app", "", "app"},
	}
	for _, c := range cases {
		got := imageName(c.repo, c.tag)
		if got != c.want {
			t.Errorf("imageName(%q, %q) = %q, want %q", c.repo, c.tag, got, c.want)
		}
	}
}

func TestDockerItemProjectKey(t *testing.T) {
	item := DockerItem{Project: "myproject"}
	if got := item.ProjectKey(); got != "myproject" {
		t.Errorf("ProjectKey() = %q, want myproject", got)
	}
	item2 := DockerItem{}
	if got := item2.ProjectKey(); got != "(no project)" {
		t.Errorf("ProjectKey() = %q, want (no project)", got)
	}
}

func TestSortDockerItems(t *testing.T) {
	items := []DockerItem{
		{Name: "a", Size: 10},
		{Name: "b", Size: 30},
		{Name: "c", Size: 20},
	}
	SortDockerItems(items)
	if items[0].Name != "b" || items[1].Name != "c" || items[2].Name != "a" {
		t.Errorf("SortDockerItems did not sort by size descending: %v", items)
	}
}

func TestMockClientListAndDelete(t *testing.T) {
	mock := &MockClient{
		Items: map[string][]DockerItem{
			"images": {
				{Type: "image", ID: "abc", Name: "img", Size: 1024},
			},
		},
	}
	items, err := mock.ListItems(nil, "images")
	if err != nil {
		t.Fatalf("ListItems error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "abc" {
		t.Fatalf("unexpected items: %v", items)
	}

	item := DockerItem{Type: "image", ID: "abc", Name: "img"}
	if err := mock.DeleteItem(nil, item); err != nil {
		t.Fatalf("DeleteItem error: %v", err)
	}
	if len(mock.Deleted) != 1 || mock.Deleted[0].ID != "abc" {
		t.Fatalf("expected deleted item abc, got %v", mock.Deleted)
	}
}

// setupMockDocker installs a fake `docker` binary at the front of PATH so the
// realClient exercises the mock daemon in testdata/docker instead of a real
// Docker installation.
func setupMockDocker(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("mock docker tests not supported on Windows")
	}

	src, err := os.ReadFile("testdata/docker")
	if err != nil {
		t.Fatalf("read mock docker script: %v", err)
	}

	tmp := t.TempDir()
	dst := filepath.Join(tmp, "docker")
	if err := os.WriteFile(dst, src, 0755); err != nil {
		t.Fatalf("write mock docker binary: %v", err)
	}

	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRealClientIsRunning(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	if !client.IsRunning(context.Background()) {
		t.Fatal("expected mock docker daemon to be running")
	}
}

func TestRealClientGetUsage(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	usage, err := client.GetUsage(context.Background())
	if err != nil {
		t.Fatalf("GetUsage error: %v", err)
	}
	if usage.Images.Type != "Images" {
		t.Errorf("expected Images type, got %q", usage.Images.Type)
	}
	if usage.Images.TotalCount != 5 {
		t.Errorf("expected 5 images, got %d", usage.Images.TotalCount)
	}
	if usage.Containers.TotalCount != 3 {
		t.Errorf("expected 3 containers, got %d", usage.Containers.TotalCount)
	}
	if usage.Volumes.TotalCount != 2 {
		t.Errorf("expected 2 volumes, got %d", usage.Volumes.TotalCount)
	}
	if usage.BuildCache.TotalCount != 0 {
		t.Errorf("expected 0 build cache entries, got %d", usage.BuildCache.TotalCount)
	}
}

func TestRealClientPrune(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	cases := []struct {
		resourceType string
	}{
		{"images"},
		{"containers"},
		{"volumes"},
		{"buildcache"},
		{"all"},
	}
	for _, c := range cases {
		reclaimed, err := client.Prune(context.Background(), c.resourceType)
		if err != nil {
			t.Fatalf("Prune(%q) error: %v", c.resourceType, err)
		}
		if reclaimed == 0 {
			t.Errorf("Prune(%q) expected positive reclaimed space, got %d", c.resourceType, reclaimed)
		}
	}
}

func TestRealClientListImages(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	items, err := client.ListItems(context.Background(), "images")
	if err != nil {
		t.Fatalf("ListItems images error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 images, got %d", len(items))
	}
	if items[0].ID != "img1" {
		t.Errorf("expected first image img1, got %q", items[0].ID)
	}
	if items[1].ID != "img2" {
		t.Errorf("expected second image img2, got %q", items[1].ID)
	}
	if !items[1].Dangling {
		t.Error("expected img2 to be dangling")
	}
	if items[0].Project != "myproject" {
		t.Errorf("expected project label myproject, got %q", items[0].Project)
	}
	if len(items[0].UsedBy) != 2 {
		t.Errorf("expected img1 to be used by 2 containers, got %v", items[0].UsedBy)
	}
	if len(items[1].UsedBy) != 1 || items[1].UsedBy[0] != "db" {
		t.Errorf("expected img2 to be used by container db, got %v", items[1].UsedBy)
	}
}

func TestRealClientListContainers(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	items, err := client.ListItems(context.Background(), "containers")
	if err != nil {
		t.Fatalf("ListItems containers error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(items))
	}
	if items[0].Name != "web" {
		t.Errorf("expected first container web, got %q", items[0].Name)
	}
	if !items[0].InUse {
		t.Error("expected web to be in use (running)")
	}
	if items[1].Name != "db" {
		t.Errorf("expected second container db, got %q", items[1].Name)
	}
	if items[1].InUse {
		t.Error("expected db to not be in use (exited)")
	}
	if !items[1].Dangling {
		t.Error("expected exited container to be dangling")
	}
	if items[2].Name != "api" {
		t.Errorf("expected third container api, got %q", items[2].Name)
	}
	if !items[2].InUse {
		t.Error("expected api to be in use (running)")
	}
}

func TestRealClientListVolumes(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	items, err := client.ListItems(context.Background(), "volumes")
	if err != nil {
		t.Fatalf("ListItems volumes error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(items))
	}
	if items[0].Name != "vol1" {
		t.Errorf("expected first volume vol1, got %q", items[0].Name)
	}
	if !items[0].InUse {
		t.Error("expected vol1 to be in use (mounted by c1)")
	}
	if items[1].Name != "vol2" {
		t.Errorf("expected second volume vol2, got %q", items[1].Name)
	}
	if items[1].InUse {
		t.Error("expected vol2 to not be in use")
	}
	if !items[1].Dangling {
		t.Error("expected vol2 to be dangling")
	}
	if len(items[0].UsedBy) != 2 {
		t.Errorf("expected vol1 to be used by 2 containers, got %v", items[0].UsedBy)
	}
	if len(items[1].UsedBy) != 0 {
		t.Errorf("expected vol2 to have no users, got %v", items[1].UsedBy)
	}
}

func TestRealClientDeleteItem(t *testing.T) {
	setupMockDocker(t)
	client := NewClient()
	cases := []DockerItem{
		{Type: "image", ID: "img1", Name: "img1"},
		{Type: "container", ID: "c1", Name: "c1"},
		{Type: "volume", ID: "vol1", Name: "vol1"},
	}
	for _, item := range cases {
		if err := client.DeleteItem(context.Background(), item); err != nil {
			t.Fatalf("DeleteItem(%q %s) error: %v", item.Type, item.ID, err)
		}
	}
}
