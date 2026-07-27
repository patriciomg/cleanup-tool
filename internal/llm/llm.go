package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/actions"
)

// Registry represents a single LLM model registry.
type Registry struct {
	Name   string
	Path   string
	Models []Model
}

// TotalSize returns the combined size of all models in the registry.
func (r *Registry) TotalSize() int64 {
	var size int64
	for _, m := range r.Models {
		size += m.Size
	}
	return size
}

// Model represents an installed model.
type Model struct {
	Name     string
	Path     string
	Size     int64
	Modified time.Time
}

// Client inspects and cleans up LLM model registries.
type Client interface {
	GetRegistries(ctx context.Context) ([]Registry, error)
	DeleteModel(ctx context.Context, registryName, modelName string) error
}

// Config holds the paths used by the client. Zero value uses the defaults.
type Config struct {
	OllamaDir      string
	HuggingFaceDir string
	LMStudioDir    string
}

func (c Config) withDefaults() Config {
	if c.OllamaDir == "" {
		c.OllamaDir = expandHome("~/.ollama/models")
	}
	if c.HuggingFaceDir == "" {
		c.HuggingFaceDir = expandHome("~/.cache/huggingface/hub")
	}
	if c.LMStudioDir == "" {
		c.LMStudioDir = expandHome("~/.cache/lm-studio/models")
	}
	return c
}

// NewClient returns a real LLM client using the default paths.
func NewClient() Client {
	return &realClient{cfg: Config{}.withDefaults()}
}

// NewClientWithConfig returns a real LLM client using the supplied paths.
func NewClientWithConfig(cfg Config) Client {
	return &realClient{cfg: cfg.withDefaults()}
}

type realClient struct {
	cfg Config
}

// GetRegistries returns the supported registries, skipping those that are not
// installed on the system.
func (c *realClient) GetRegistries(ctx context.Context) ([]Registry, error) {
	registries := []Registry{
		c.ollamaRegistry(),
		c.huggingfaceRegistry(),
		c.lmStudioRegistry(),
	}
	return registries, nil
}

func (c *realClient) ollamaRegistry() Registry {
	return Registry{
		Name:   "Ollama",
		Path:   c.cfg.OllamaDir,
		Models: c.listOllamaModels(c.cfg.OllamaDir),
	}
}

func (c *realClient) huggingfaceRegistry() Registry {
	return Registry{
		Name:   "Hugging Face",
		Path:   c.cfg.HuggingFaceDir,
		Models: c.listHubModels(c.cfg.HuggingFaceDir, "models--"),
	}
}

func (c *realClient) lmStudioRegistry() Registry {
	return Registry{
		Name:   "LM Studio",
		Path:   c.cfg.LMStudioDir,
		Models: c.listHubModels(c.cfg.LMStudioDir, ""),
	}
}

func (c *realClient) DeleteModel(ctx context.Context, registryName, modelName string) error {
	regs, err := c.GetRegistries(ctx)
	if err != nil {
		return err
	}
	for _, reg := range regs {
		if reg.Name != registryName {
			continue
		}
		for _, m := range reg.Models {
			if m.Name == modelName {
				return deleteModel(reg, m)
			}
		}
	}
	return fmt.Errorf("model %q not found in registry %q", modelName, registryName)
}

func deleteModel(reg Registry, m Model) error {
	if reg.Name == "Ollama" {
		return fmt.Errorf("ollama models must be deleted with `ollama rm %s`", m.Name)
	}
	return actions.Trash(m.Path)
}

func (c *realClient) listOllamaModels(base string) []Model {
	// Prefer the official CLI for accurate names and sizes.
	models, err := listOllamaViaCLI(base)
	if err == nil {
		return models
	}
	// Fallback to scanning manifests when the CLI is unavailable.
	manifestsDir := filepath.Join(base, "manifests")
	info, err := os.Stat(manifestsDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	models = nil
	walkManifests(manifestsDir, base, &models)
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models
}

func listOllamaViaCLI(base string) ([]Model, error) {
	cmd := exec.Command("ollama", "list")
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ollama list failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}

	var models []Model
	lines := strings.Split(out.String(), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		size, _ := parseSize(parts[2])
		var modified time.Time
		if len(parts) >= 4 {
			modified, _ = time.Parse("2006-01-02", parts[3])
		}
		models = append(models, Model{
			Name:     name,
			Path:     filepath.Join(base, name),
			Size:     size,
			Modified: modified,
		})
	}
	return models, nil
}

func walkManifests(manifestsDir, base string, models *[]Model) {
	_ = filepath.Walk(manifestsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || info.Name() == "latest" {
			return nil
		}
		rel, err := filepath.Rel(manifestsDir, path)
		if err != nil {
			return nil
		}
		name := strings.ReplaceAll(rel, string(filepath.Separator), ":")
		*models = append(*models, Model{
			Name:     name,
			Path:     path,
			Size:     0,
			Modified: info.ModTime(),
		})
		return nil
	})
}

func parseSize(s string) (int64, error) {
	// Parse Ollama/HF style sizes: 4.7 GB, 1.2GB, 1.5K, etc.
	s = strings.TrimSpace(s)
	var multiplier int64 = 1
	if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}
	s = strings.TrimSpace(s)
	var value float64
	if _, err := fmt.Sscanf(s, "%f", &value); err != nil {
		return 0, err
	}
	return int64(value * float64(multiplier)), nil
}

func (c *realClient) listHubModels(base, prefix string) []Model {
	info, err := os.Stat(base)
	if err != nil || !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	var models []Model
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if prefix != "" && !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		path := filepath.Join(base, entry.Name())
		size, modified := dirSizeAndMtime(path)
		models = append(models, Model{
			Name:     entry.Name(),
			Path:     path,
			Size:     size,
			Modified: modified,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models
}

func dirSizeAndMtime(path string) (int64, time.Time) {
	var size int64
	var mtime time.Time
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		if info.ModTime().After(mtime) {
			mtime = info.ModTime()
		}
		return nil
	})
	_ = err
	return size, mtime
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
