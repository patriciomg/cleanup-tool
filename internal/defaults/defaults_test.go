package defaults

import (
	"slices"
	"testing"
)

func TestDepsTargets(t *testing.T) {
	got := DepsTargets()
	if len(got) == 0 {
		t.Fatal("expected non-empty default dependency targets")
	}

	want := []string{
		"node_modules",
		".pnpm",
		"vendor",
		".venv",
		"venv",
		"bower_components",
		"Pods",
		"Carthage",
		".gradle",
		".m2",
		"target",
		".tox",
		"packages",
		".nuget",
		".stack-work",
		"elm-stuff",
		"_build",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("DepsTargets() = %v, want %v", got, want)
	}
}
