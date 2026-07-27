package main

import (
	"flag"
	"testing"
)

func TestParseInterspersedPositionalOnly(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var b bool
	fs.BoolVar(&b, "flag", false, "")

	positionals, err := parseInterspersed(fs, []string{"foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positionals) != 1 || positionals[0] != "foo" {
		t.Fatalf("expected [foo], got %v", positionals)
	}
}

func TestParseInterspersedFlagBeforePositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var b bool
	fs.BoolVar(&b, "flag", false, "")

	positionals, err := parseInterspersed(fs, []string{"--flag", "foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !b {
		t.Fatalf("expected flag to be set")
	}
	if len(positionals) != 1 || positionals[0] != "foo" {
		t.Fatalf("expected [foo], got %v", positionals)
	}
}

func TestParseInterspersedFlagAfterPositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var b bool
	fs.BoolVar(&b, "flag", false, "")

	positionals, err := parseInterspersed(fs, []string{"foo", "--flag"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !b {
		t.Fatalf("expected flag to be set")
	}
	if len(positionals) != 1 || positionals[0] != "foo" {
		t.Fatalf("expected [foo], got %v", positionals)
	}
}

func TestParseInterspersedFlagWithValueAfterPositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var s string
	fs.StringVar(&s, "name", "", "")

	positionals, err := parseInterspersed(fs, []string{"foo", "--name", "bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "bar" {
		t.Fatalf("expected name=bar, got %q", s)
	}
	if len(positionals) != 1 || positionals[0] != "foo" {
		t.Fatalf("expected [foo], got %v", positionals)
	}
}

func TestParseInterspersedTerminator(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var b bool
	fs.BoolVar(&b, "flag", false, "")

	positionals, err := parseInterspersed(fs, []string{"--", "--flag"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b {
		t.Fatalf("expected flag not to be set after --")
	}
	if len(positionals) != 1 || positionals[0] != "--flag" {
		t.Fatalf("expected [--flag], got %v", positionals)
	}
}

func TestParseInterspersedMultiplePositionals(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var b bool
	fs.BoolVar(&b, "flag", false, "")

	positionals, err := parseInterspersed(fs, []string{"foo", "--flag", "bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !b {
		t.Fatalf("expected flag to be set")
	}
	if len(positionals) != 2 || positionals[0] != "foo" || positionals[1] != "bar" {
		t.Fatalf("expected [foo bar], got %v", positionals)
	}
}
