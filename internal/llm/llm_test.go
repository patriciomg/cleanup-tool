package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigWithDefaults(t *testing.T) {
	c := Config{}.withDefaults()
	if c.OllamaDir == "" {
		t.Fatal("OllamaDir should have a default")
	}
	if c.HuggingFaceDir == "" {
		t.Fatal("HuggingFaceDir should have a default")
	}
	if c.LMStudioDir == "" {
		t.Fatal("LMStudioDir should have a default")
	}
}

func TestHuggingFaceRegistry(t *testing.T) {
	tmp := t.TempDir()
	modelDir := filepath.Join(tmp, "models--org--model-a")
	if err := os.MkdirAll(filepath.Join(modelDir, "snapshots", "abc123"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "snapshots", "abc123", "model.bin"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}

	// Point to the parent directory so listHubModels walks the prefix correctly.
	client := NewClientWithConfig(Config{HuggingFaceDir: tmp})
	regs, err := client.GetRegistries(context.Background())
	if err != nil {
		t.Fatalf("GetRegistries: %v", err)
	}

	var found *Registry
	for i := range regs {
		if regs[i].Name == "Hugging Face" {
			found = &regs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("Hugging Face registry not found")
	}
	if len(found.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(found.Models))
	}
	if found.Models[0].Name != "models--org--model-a" {
		t.Fatalf("unexpected model name: %s", found.Models[0].Name)
	}
	if found.TotalSize() == 0 {
		t.Fatal("expected non-zero total size")
	}
}

func TestLMStudioRegistry(t *testing.T) {
	tmp := t.TempDir()
	modelDir := filepath.Join(tmp, "publisher", "model-b")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.safetensors"), make([]byte, 2000), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClientWithConfig(Config{LMStudioDir: tmp})
	regs, err := client.GetRegistries(context.Background())
	if err != nil {
		t.Fatalf("GetRegistries: %v", err)
	}

	var found *Registry
	for i := range regs {
		if regs[i].Name == "LM Studio" {
			found = &regs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("LM Studio registry not found")
	}
	if len(found.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(found.Models))
	}
	if found.Models[0].Name != "publisher" {
		t.Fatalf("unexpected model name: %s", found.Models[0].Name)
	}
}

func TestParseSize(t *testing.T) {
	// helper converts a float byte value to the same int64 that parseSize returns.
	f := func(gb float64) int64 {
		return int64(gb * float64(1024*1024*1024))
	}
	cases := []struct {
		input string
		want  int64
	}{
		{"4.7 GB", f(4.7)},
		{"1.2GB", f(1.2)},
		{"500 MB", int64(500 * 1024 * 1024)},
		{"1.5 KB", int64(1.5 * 1024)},
		{"5 B", 5},
	}
	for _, tc := range cases {
		got, err := parseSize(tc.input)
		if err != nil {
			t.Fatalf("parseSize(%q) error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("parseSize(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
