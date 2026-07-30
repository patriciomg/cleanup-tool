package common

import (
	"testing"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
)

func TestCapitalize(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"images", "Images"},
		{"containers", "Containers"},
		{"hello world", "Hello world"},
		{"already", "Already"},
		{"ünicode", "Ünicode"},
		{"123abc", "123abc"},
	}
	for _, c := range cases {
		got := Capitalize(c.input)
		if got != c.want {
			t.Errorf("Capitalize(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSortTreeBySize(t *testing.T) {
	a := &analyzer.Entry{Name: "a", Path: "/a", IsDir: true, Size: 10}
	b := &analyzer.Entry{Name: "b", Path: "/b", IsDir: false, Size: 20}
	c := &analyzer.Entry{Name: "c", Path: "/c", IsDir: false, Size: 5}
	root := &analyzer.Entry{Name: "root", Path: "/root", IsDir: true, Size: 35, Children: []*analyzer.Entry{a, b, c}}

	SortTree([]*analyzer.Entry{root}, "size")

	if root.Children[0].Name != "b" || root.Children[1].Name != "a" || root.Children[2].Name != "c" {
		t.Fatalf("expected size-sorted children b, a, c, got %s, %s, %s", root.Children[0].Name, root.Children[1].Name, root.Children[2].Name)
	}
}

func TestSortTreeByName(t *testing.T) {
	z := &analyzer.Entry{Name: "Z", Path: "/Z", IsDir: false, Size: 1}
	a := &analyzer.Entry{Name: "a", Path: "/a", IsDir: false, Size: 2}
	b := &analyzer.Entry{Name: "B", Path: "/B", IsDir: false, Size: 3}
	root := &analyzer.Entry{Name: "root", Path: "/root", IsDir: true, Children: []*analyzer.Entry{z, a, b}}

	SortTree([]*analyzer.Entry{root}, "name")

	want := []string{"a", "B", "Z"}
	for i, child := range root.Children {
		if child.Name != want[i] {
			t.Fatalf("expected name-sorted child %q at index %d, got %q", want[i], i, child.Name)
		}
	}
}

func TestSortTreeByAccessTime(t *testing.T) {
	now := time.Now()
	old := &analyzer.Entry{Name: "old", Path: "/old", AccessTime: now.Add(-2 * time.Hour)}
	mid := &analyzer.Entry{Name: "mid", Path: "/mid", AccessTime: now.Add(-1 * time.Hour)}
	new := &analyzer.Entry{Name: "new", Path: "/new", AccessTime: now}
	root := &analyzer.Entry{Name: "root", Path: "/root", IsDir: true, Children: []*analyzer.Entry{old, new, mid}}

	SortTree([]*analyzer.Entry{root}, "access")

	want := []string{"new", "mid", "old"}
	for i, child := range root.Children {
		if child.Name != want[i] {
			t.Fatalf("expected access-sorted child %q at index %d, got %q", want[i], i, child.Name)
		}
	}
}

func TestSortTreeByModTime(t *testing.T) {
	now := time.Now()
	old := &analyzer.Entry{Name: "old", Path: "/old", ModTime: now.Add(-2 * time.Hour)}
	mid := &analyzer.Entry{Name: "mid", Path: "/mid", ModTime: now.Add(-1 * time.Hour)}
	new := &analyzer.Entry{Name: "new", Path: "/new", ModTime: now}
	root := &analyzer.Entry{Name: "root", Path: "/root", IsDir: true, Children: []*analyzer.Entry{old, new, mid}}

	SortTree([]*analyzer.Entry{root}, "modified")

	want := []string{"new", "mid", "old"}
	for i, child := range root.Children {
		if child.Name != want[i] {
			t.Fatalf("expected modified-sorted child %q at index %d, got %q", want[i], i, child.Name)
		}
	}
}

func TestSortTreeDefaultsToSize(t *testing.T) {
	a := &analyzer.Entry{Name: "a", Path: "/a", IsDir: true, Size: 10}
	b := &analyzer.Entry{Name: "b", Path: "/b", IsDir: false, Size: 20}
	root := &analyzer.Entry{Name: "root", Path: "/root", IsDir: true, Children: []*analyzer.Entry{a, b}}

	SortTree([]*analyzer.Entry{root}, "unknown-order")

	if root.Children[0].Name != "b" {
		t.Fatalf("expected default size sort with b first, got %s", root.Children[0].Name)
	}
}
