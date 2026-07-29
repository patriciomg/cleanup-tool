package common

import (
	"testing"
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
