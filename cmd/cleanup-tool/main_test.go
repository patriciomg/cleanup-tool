package main

import (
	"flag"
	"testing"
)

func TestBoolFlag(t *testing.T) {
	cases := []struct {
		name      string
		initial   bool
		args      []string
		wantValue bool
		wantSet   bool
		wantErr   bool
	}{
		{"default false not set", false, nil, false, false, false},
		{"default true not set", true, nil, true, false, false},
		{"set true from default false", false, []string{"-flag=true"}, true, true, false},
		{"set false from default true", true, []string{"-flag=false"}, false, true, false},
		{"set true without explicit value", false, []string{"-flag"}, true, true, false},
		{"invalid value errors", false, []string{"-flag=maybe"}, false, false, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bf := &boolFlag{value: c.initial}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.Var(bf, "flag", "bool flag for testing")

			err := fs.Parse(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error parsing %v", c.args)
				}
				if bf.set {
					t.Errorf("set = true, want false after invalid value")
				}
				if bf.value != c.initial {
					t.Errorf("value = %v, want %v after invalid value", bf.value, c.initial)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error parsing %v: %v", c.args, err)
			}

			if bf.value != c.wantValue {
				t.Errorf("value = %v, want %v", bf.value, c.wantValue)
			}
			if bf.set != c.wantSet {
				t.Errorf("set = %v, want %v", bf.set, c.wantSet)
			}
		})
	}
}

func TestBoolFlagIsBoolFlag(t *testing.T) {
	bf := &boolFlag{}
	if !bf.IsBoolFlag() {
		t.Error("IsBoolFlag() = false, want true")
	}
}

func TestBoolFlagString(t *testing.T) {
	if got := (&boolFlag{value: true}).String(); got != "true" {
		t.Errorf("String() = %q, want \"true\"", got)
	}
	if got := (&boolFlag{value: false}).String(); got != "false" {
		t.Errorf("String() = %q, want \"false\"", got)
	}
}
