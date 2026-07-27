package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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

// Client is the interface for Docker operations.
type Client interface {
	IsRunning(ctx context.Context) bool
	GetUsage(ctx context.Context) (*Usage, error)
	Prune(ctx context.Context, resourceType string) (int64, error)
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
