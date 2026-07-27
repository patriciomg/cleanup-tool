package categories

import "testing"

func TestClassifyExtensions(t *testing.T) {
	cases := []struct {
		path string
		name string
		want Category
	}{
		{"/tmp/model.gguf", "model.gguf", LLM},
		{"/tmp/model.safetensors", "model.safetensors", LLM},
		{"/tmp/photo.jpg", "photo.jpg", Media},
		{"/tmp/archive.zip", "archive.zip", Archive},
		{"/tmp/file.log", "file.log", LogCache},
		{"/tmp/unknown.txt", "unknown.txt", Unknown},
	}

	for _, c := range cases {
		got := Classify(c.path, c.name)
		if got != c.want {
			t.Errorf("Classify(%q, %q) = %q, want %q", c.path, c.name, got, c.want)
		}
	}
}

func TestClassifyNames(t *testing.T) {
	cases := []struct {
		path string
		name string
		want Category
	}{
		{"/project/node_modules", "node_modules", Dependency},
		{"/project/.git", ".git", GitRepo},
		{"/project/target", "target", BuildArtifact},
		{"/project/.venv", ".venv", Dependency},
		{"/project/.docker", ".docker", Docker},
	}

	for _, c := range cases {
		got := Classify(c.path, c.name)
		if got != c.want {
			t.Errorf("Classify(%q, %q) = %q, want %q", c.path, c.name, got, c.want)
		}
	}
}
