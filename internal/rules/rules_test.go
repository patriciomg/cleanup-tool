package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuleValidate(t *testing.T) {
	cases := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name:    "missing name",
			rule:    Rule{Paths: []string{"/tmp"}, Action: "trash"},
			wantErr: true,
		},
		{
			name:    "missing paths",
			rule:    Rule{Name: "test", Action: "trash"},
			wantErr: true,
		},
		{
			name:    "invalid action",
			rule:    Rule{Name: "test", Paths: []string{"/tmp"}, Action: "delete"},
			wantErr: true,
		},
		{
			name:    "move without destination",
			rule:    Rule{Name: "test", Paths: []string{"/tmp"}, Action: "move_external"},
			wantErr: true,
		},
		{
			name:    "protected path root",
			rule:    Rule{Name: "test", Paths: []string{"/"}, Action: "trash"},
			wantErr: true,
		},
		{
			name:    "protected path system",
			rule:    Rule{Name: "test", Paths: []string{"/System/Library"}, Action: "trash"},
			wantErr: true,
		},
		{
			name:    "valid trash",
			rule:    Rule{Name: "test", Paths: []string{"/tmp"}, Action: "trash"},
			wantErr: false,
		},
		{
			name:    "valid move",
			rule:    Rule{Name: "test", Paths: []string{"/tmp"}, Action: "move_external", Destination: "/Volumes/External"},
			wantErr: false,
		},
		{
			name:    "invalid category",
			rule:    Rule{Name: "test", Paths: []string{"/tmp"}, Action: "trash", Categories: []string{"old", "unknown"}},
			wantErr: true,
		},
		{
			name:    "invalid dup_mode",
			rule:    Rule{Name: "test", Paths: []string{"/tmp"}, Action: "trash", DupMode: "fast"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.rule.Validate()
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	origPath := Path
	Path = func() string { return filepath.Join(dir, "rules.json") }
	defer func() { Path = origPath }()

	f, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	f.Set(Rule{Name: "test", Paths: []string{"/tmp"}, Action: "trash"})
	if err := f.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	f2, err := Load()
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	r, ok := f2.Get("test")
	if !ok {
		t.Fatalf("rule not found after load")
	}
	if r.Action != "trash" {
		t.Fatalf("expected action trash, got %s", r.Action)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	origPath := Path
	Path = func() string { return filepath.Join(dir, "does-not-exist", "rules.json") }
	defer func() { Path = origPath }()

	f, err := Load()
	if err != nil {
		t.Fatalf("Load should return empty file on missing: %v", err)
	}
	if len(f.Rules) != 0 {
		t.Fatalf("expected empty rules, got %d", len(f.Rules))
	}
}

func TestProtectedPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/System", true},
		{"/System/Library", true},
		{"/Library", true},
		{"/Applications", true},
		{"/tmp", false},
		{"/Users/foo/Downloads/app.app", true},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := isProtectedPath(c.path)
			if got != c.want {
				t.Fatalf("isProtectedPath(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestProtectedPathSymlink(t *testing.T) {
	tmp := t.TempDir()
	link := filepath.Join(tmp, "root-link")
	if err := os.Symlink("/", link); err != nil {
		t.Fatal(err)
	}
	if !isProtectedPath(link) {
		t.Fatalf("expected symlink to / to be protected")
	}
}
