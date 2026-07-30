package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ResourceUsage holds usage for a single Docker resource category.
type ResourceUsage struct {
	Type        string
	TotalCount  int
	Size        int64
	Reclaimable int64
}

// Usage holds Docker disk usage for all resource categories.
type Usage struct {
	Images     ResourceUsage
	Containers ResourceUsage
	Volumes    ResourceUsage
	BuildCache ResourceUsage
}

// TotalSize returns the total disk space used by Docker resources.
func (u *Usage) TotalSize() int64 {
	if u == nil {
		return 0
	}
	return u.Images.Size + u.Containers.Size + u.Volumes.Size + u.BuildCache.Size
}

// TotalReclaimable returns the total reclaimable disk space.
func (u *Usage) TotalReclaimable() int64 {
	if u == nil {
		return 0
	}
	return u.Images.Reclaimable + u.Containers.Reclaimable + u.Volumes.Reclaimable + u.BuildCache.Reclaimable
}

// DockerItem represents a single Docker resource that can be inspected or
// deleted from the TUI.
type DockerItem struct {
	Type      string            // "image", "container", "volume"
	ID        string            // Short or full ID; used for deletion.
	Name      string            // Human-readable name.
	Size      int64             // Size in bytes (best-effort; volumes may be 0).
	CreatedAt time.Time         // Creation time if available.
	Dangling  bool              // True for dangling images / unused volumes / stopped containers.
	InUse     bool              // True if the resource is currently referenced by a running container.
	Project   string            // Docker Compose project label, when present.
	Labels    map[string]string // All labels for diagnostics.
	UsedBy    []string          // Names of containers referencing this image/volume (best-effort).
}

// ProjectKey returns the project label key used for grouping.
func (i DockerItem) ProjectKey() string {
	if i.Project != "" {
		return i.Project
	}
	return "(no project)"
}

// Client is the interface for Docker operations.
type Client interface {
	IsRunning(ctx context.Context) bool
	GetUsage(ctx context.Context) (*Usage, error)
	Prune(ctx context.Context, resourceType string) (int64, error)
	ListItems(ctx context.Context, itemType string) ([]DockerItem, error)
	DeleteItem(ctx context.Context, item DockerItem) error
}

// NewClient returns a real Docker client that shells out to the docker CLI.
func NewClient() Client {
	return &realClient{}
}

type realClient struct{}

func (r *realClient) IsRunning(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info")
	err := cmd.Run()
	return err == nil
}

// dfRow matches the JSON output of `docker system df --format '{{json .}}'`.
type dfRow struct {
	Type        string `json:"Type"`
	TotalCount  string `json:"TotalCount"`
	Active      string `json:"Active"`
	Size        string `json:"Size"`
	Reclaimable string `json:"Reclaimable"`
}

func (r *realClient) GetUsage(ctx context.Context) (*Usage, error) {
	cmd := exec.CommandContext(ctx, "docker", "system", "df", "--format", "{{json .}}")
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker system df failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}

	usage := &Usage{}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row dfRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse docker system df row: %w", err)
		}
		ru := ResourceUsage{
			Type:        row.Type,
			TotalCount:  parseCount(row.TotalCount),
			Size:        parseSize(row.Size),
			Reclaimable: parseSize(row.Reclaimable),
		}
		switch row.Type {
		case "Images":
			usage.Images = ru
		case "Containers":
			usage.Containers = ru
		case "Local Volumes":
			usage.Volumes = ru
		case "Build Cache":
			usage.BuildCache = ru
		}
	}

	return usage, nil
}

var resourceCommands = map[string][]string{
	"images":     {"image", "prune", "-f"},
	"containers": {"container", "prune", "-f"},
	"volumes":    {"volume", "prune", "-f"},
	"buildcache": {"builder", "prune", "-f"},
	"all":        {"system", "prune", "-a", "-f", "--volumes"},
}

func (r *realClient) Prune(ctx context.Context, resourceType string) (int64, error) {
	args, ok := resourceCommands[resourceType]
	if !ok {
		return 0, fmt.Errorf("unknown resource type %q", resourceType)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("docker %s prune failed: %w: %s", resourceType, err, strings.TrimSpace(errOut.String()))
	}

	reclaimed := parseReclaimed(out.String())
	return reclaimed, nil
}

// ListItems returns a list of individual Docker resources for the given
// category. Supported itemType values are "images", "containers", and
// "volumes".
func (r *realClient) ListItems(ctx context.Context, itemType string) ([]DockerItem, error) {
	switch itemType {
	case "images":
		return r.listImages(ctx)
	case "containers":
		return r.listContainers(ctx)
	case "volumes":
		return r.listVolumes(ctx)
	default:
		return nil, fmt.Errorf("unknown docker item type %q", itemType)
	}
}

// DeleteItem removes a single Docker resource identified by its type and ID.
func (r *realClient) DeleteItem(ctx context.Context, item DockerItem) error {
	switch item.Type {
	case "image":
		cmd := exec.CommandContext(ctx, "docker", "image", "rm", "-f", item.ID)
		return runDockerCmd(cmd, "image rm")
	case "container":
		cmd := exec.CommandContext(ctx, "docker", "rm", "-f", item.ID)
		return runDockerCmd(cmd, "container rm")
	case "volume":
		cmd := exec.CommandContext(ctx, "docker", "volume", "rm", "-f", item.ID)
		return runDockerCmd(cmd, "volume rm")
	default:
		return fmt.Errorf("cannot delete unknown docker item type %q", item.Type)
	}
}

func runDockerCmd(cmd *exec.Cmd, op string) error {
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s failed: %w: %s", op, err, strings.TrimSpace(errOut.String()))
	}
	return nil
}

// imageRow matches the JSON output of `docker image ls --format '{{json .}}'`.
type imageRow struct {
	ID        string `json:"ID"`
	Repository string `json:"Repository"`
	Tag       string `json:"Tag"`
	CreatedAt string `json:"CreatedAt"`
	Size      string `json:"Size"`
}

// containerRef matches a subset of `docker ps -a --format '{{json .}}'` used
// to build reverse-lookup maps for Docker items.
type containerRef struct {
	ID    string `json:"ID"`
	Image string `json:"Image"`
	Names string `json:"Names"`
}

// containerReferences returns a list of running and stopped containers with
// just enough information to determine what images/volumes they reference.
func (r *realClient) containerReferences(ctx context.Context) ([]containerRef, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	out, err := runDockerOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}
	var refs []containerRef
	for _, line := range out {
		var ref containerRef
		if err := json.Unmarshal([]byte(line), &ref); err != nil {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (r *realClient) listImages(ctx context.Context) ([]DockerItem, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "ls", "--format", "{{json .}}")
	out, err := runDockerOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var items []DockerItem
	var ids []string
	// refToID maps common image references (ID, repo, repo:tag) to image ID so
	// that container image names can be resolved back to the actual image.
	refToID := make(map[string]string)
	for _, line := range out {
		var row imageRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse docker image row: %w", err)
		}
		name := imageName(row.Repository, row.Tag)
		created := parseDockerTime(row.CreatedAt)
		items = append(items, DockerItem{
			Type:      "image",
			ID:        row.ID,
			Name:      name,
			Size:      parseSize(row.Size),
			CreatedAt: created,
			Dangling:  row.Repository == "<none>",
		})
		ids = append(ids, row.ID)

		// Register every likely way a container may reference this image.
		refToID[row.ID] = row.ID
		if row.Repository != "" && row.Repository != "<none>" {
			refToID[row.Repository] = row.ID
			if row.Tag != "" && row.Tag != "<none>" {
				refToID[row.Repository+":"+row.Tag] = row.ID
				// Docker treats "repo" as "repo:latest" when starting containers.
				if row.Tag == "latest" {
					refToID[row.Repository+":latest"] = row.ID
				}
			}
		}
	}

	// Resolve container image references back to the images we just listed.
	refs, err := r.containerReferences(ctx)
	if err == nil {
		usedBy := make(map[string][]string)
		for _, ref := range refs {
			if id, ok := refToID[ref.Image]; ok {
				usedBy[id] = append(usedBy[id], strings.TrimPrefix(ref.Names, "/"))
			}
		}
		for i := range items {
			items[i].UsedBy = usedBy[items[i].ID]
		}
	}

	// Images that are referenced by at least one container are considered in
	// use, which helps callers color-code the status correctly.
	for i := range items {
		if len(items[i].UsedBy) > 0 {
			items[i].InUse = true
		}
	}

	labels, err := r.bulkLabels(ctx, ids, ".Config.Labels")
	if err == nil {
		applyLabels(items, labels)
	}
	return items, nil
}

// containerRow matches the JSON output of `docker ps -a --format '{{json .}}'`.
type containerRow struct {
	ID        string `json:"ID"`
	Image     string `json:"Image"`
	Names     string `json:"Names"`
	Status    string `json:"Status"`
	CreatedAt string `json:"CreatedAt"`
	Size      string `json:"Size"`
}

func (r *realClient) listContainers(ctx context.Context) ([]DockerItem, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	out, err := runDockerOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var items []DockerItem
	var ids []string
	for _, line := range out {
		var row containerRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse docker container row: %w", err)
		}
		created := parseDockerTime(row.CreatedAt)
		items = append(items, DockerItem{
			Type:      "container",
			ID:        row.ID,
			Name:      strings.TrimPrefix(row.Names, "/"),
			Size:      parseSize(row.Size),
			CreatedAt: created,
			Dangling:  !strings.HasPrefix(row.Status, "Up"),
			InUse:     strings.HasPrefix(row.Status, "Up"),
		})
		ids = append(ids, row.ID)
	}

	labels, err := r.bulkLabels(ctx, ids, ".Config.Labels")
	if err == nil {
		applyLabels(items, labels)
	}
	return items, nil
}

// volumeRow matches the JSON output of `docker volume ls --format '{{json .}}'`.
type volumeRow struct {
	Driver string            `json:"Driver"`
	Labels string            `json:"Labels"`
	Name   string            `json:"Name"`
	Scope  string            `json:"Scope"`
}

func (r *realClient) listVolumes(ctx context.Context) ([]DockerItem, error) {
	cmd := exec.CommandContext(ctx, "docker", "volume", "ls", "--format", "{{json .}}")
	out, err := runDockerOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}

	mounted, err := r.containerMountedVolumes(ctx)
	if err != nil {
		// Without mount information we cannot safely determine whether a
		// volume is in use, so surface the error rather than guessing.
		return nil, fmt.Errorf("cannot determine volume mounts: %w", err)
	}

	var items []DockerItem
	var ids []string
	for _, line := range out {
		var row volumeRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse docker volume row: %w", err)
		}
		inUse := len(mounted[row.Name]) > 0
		items = append(items, DockerItem{
			Type:     "volume",
			ID:       row.Name,
			Name:     row.Name,
			InUse:    inUse,
			Dangling: !inUse,
			UsedBy:   mounted[row.Name],
			// Volumes report no size from docker volume ls; size must come
			// from docker system df.
		})
		ids = append(ids, row.Name)
	}

	// Volume labels require a separate inspect path.
	labels, err := r.bulkLabels(ctx, ids, ".Labels")
	if err == nil {
		applyLabels(items, labels)
	}
	return items, nil
}

// dockerMount matches the JSON representation of a single Docker mount.
type dockerMount struct {
	Type   string `json:"Type"`
	Source string `json:"Source"`
}

// containerMountedVolumes returns a map from volume name to the list of
// container names that mount it. It tolerates a missing Docker daemon and
// returns an empty map when no containers exist.
func (r *realClient) containerMountedVolumes(ctx context.Context) (map[string][]string, error) {
	refs, err := r.containerReferences(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w", err)
	}
	if len(refs) == 0 {
		return map[string][]string{}, nil
	}

	ids := make([]string, len(refs))
	for i, ref := range refs {
		ids[i] = ref.ID
	}

	args := []string{"inspect", "--format", "{{json .Mounts}}"}
	args = append(args, ids...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := runDockerOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}

	mounted := make(map[string][]string)
	for i, line := range out {
		if i >= len(ids) {
			break
		}
		containerName := strings.TrimPrefix(refs[i].Names, "/")
		var mounts []dockerMount
		if err := json.Unmarshal([]byte(line), &mounts); err != nil {
			// Best-effort: ignore unparseable mount lines.
			continue
		}
		for _, m := range mounts {
			if m.Type == "volume" {
				mounted[m.Source] = append(mounted[m.Source], containerName)
			}
		}
	}
	return mounted, nil
}

// bulkLabels runs docker inspect on a batch of IDs and returns a map from
// each ID to its labels. It returns a non-nil error only if the inspect
// command itself fails; missing individual items are tolerated and omitted
// from the map.
func (r *realClient) bulkLabels(ctx context.Context, ids []string, path string) (map[string]map[string]string, error) {
	result := make(map[string]map[string]string)
	if len(ids) == 0 {
		return result, nil
	}
	args := []string{"inspect", "--format", "{{json " + path + "}}"}
	args = append(args, ids...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := runDockerOutput(ctx, cmd)
	if err != nil {
		return nil, err
	}
	for i, line := range out {
		if i >= len(ids) {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || line == "<no value>" {
			result[ids[i]] = nil
			continue
		}
		var labels map[string]string
		if err := json.Unmarshal([]byte(line), &labels); err != nil {
			result[ids[i]] = nil
			continue
		}
		result[ids[i]] = labels
	}
	return result, nil
}

func applyLabels(items []DockerItem, labels map[string]map[string]string) {
	for i := range items {
		l := labels[items[i].ID]
		items[i].Labels = l
		if l != nil {
			items[i].Project = l["com.docker.compose.project"]
		}
	}
}

func runDockerOutput(ctx context.Context, cmd *exec.Cmd) ([]string, error) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker command failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	s := strings.TrimSpace(out.String())
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

func imageName(repo, tag string) string {
	if repo == "<none>" {
		return "<dangling>"
	}
	if tag == "" || tag == "<none>" {
		return repo
	}
	if tag == "latest" {
		return repo
	}
	return repo + ":" + tag
}

var reclaimedRe = regexp.MustCompile(`Total reclaimed space:\s+([\d.]+)\s*(B|KB|MB|GB|TB)?`)

func parseReclaimed(s string) int64 {
	matches := reclaimedRe.FindStringSubmatch(s)
	if len(matches) < 3 {
		return 0
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	unit := 1.0
	switch matches[2] {
	case "KB":
		unit = 1024
	case "MB":
		unit = 1024 * 1024
	case "GB":
		unit = 1024 * 1024 * 1024
	case "TB":
		unit = 1024 * 1024 * 1024 * 1024
	}
	return int64(value * unit)
}

func parseCount(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// parseSize parses Docker's human-readable size strings like "1.234GB" or "567MB".
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
		return 0
	}
	// Docker may use units like "1.234GB", "567MB", etc.
	re := regexp.MustCompile(`^([\d.]+)\s*([KMGT]?)B?$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) < 3 {
		return 0
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	unit := 1.0
	switch matches[2] {
	case "K":
		unit = 1024
	case "M":
		unit = 1024 * 1024
	case "G":
		unit = 1024 * 1024 * 1024
	case "T":
		unit = 1024 * 1024 * 1024 * 1024
	}
	return int64(value * unit)
}

// parseDockerTime parses the timestamps produced by docker --format '{{json .}}'.
// Docker uses RFC3339Nano in recent versions; fall back to a few common layouts.
func parseDockerTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// SortDockerItems sorts docker items by size descending. It mutates the slice.
func SortDockerItems(items []DockerItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Size > items[j].Size
	})
}
