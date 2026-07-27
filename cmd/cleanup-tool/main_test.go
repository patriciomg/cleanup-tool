package main

import (
	"slices"
	"testing"
)

func TestParseCSVColumns(t *testing.T) {
	cases := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		{"", nil, false},
		{"Name,Size", []string{"Name", "Size"}, false},
		{"name, size, MODTIME", []string{"Name", "Size", "ModTime"}, false},
		{"Invalid", nil, true},
		{"Name,", []string{"Name"}, false},
	}

	for _, tc := range cases {
		got, err := parseCSVColumns(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseCSVColumns(%q) expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseCSVColumns(%q) unexpected error: %v", tc.input, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("parseCSVColumns(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
