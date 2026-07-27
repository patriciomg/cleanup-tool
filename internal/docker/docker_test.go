package docker

import (
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
