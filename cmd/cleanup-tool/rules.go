package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/patriciomg/cleanup-tool/internal/executor"
	"github.com/patriciomg/cleanup-tool/internal/rules"
)

// handleRulesCmd dispatches the "rules" subcommand and its sub-commands.
func handleRulesCmd(args []string) {
	if len(args) == 0 {
		printRulesUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "create":
		rulesCreate(args[1:])
	case "list":
		rulesList(args[1:])
	case "show":
		rulesShow(args[1:])
	case "edit":
		rulesEdit(args[1:])
	case "delete":
		rulesDelete(args[1:])
	case "run":
		rulesRun(args[1:])
	default:
		printRulesUsage()
		os.Exit(1)
	}
}

func printRulesUsage() {
	fmt.Println("Usage: cleanup-tool rules <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  create   Create a new rule")
	fmt.Println("  list     List saved rules")
	fmt.Println("  show     Show a single rule")
	fmt.Println("  edit     Edit a rule in $EDITOR")
	fmt.Println("  delete   Delete a rule")
	fmt.Println("  run      Run a rule now")
}

func rulesCreate(args []string) {
	var r rules.Rule
	flagSet := flag.NewFlagSet("rules create", flag.ExitOnError)
	flagSet.StringVar(&r.Name, "name", "", "Unique rule name (required)")
	flagSet.StringVar(&r.Action, "action", "trash", "Action: trash or move_external")
	flagSet.StringVar(&r.Destination, "destination", "", "External destination for move_external")
	flagSet.StringVar(&r.DupMode, "dup-mode", "smart", "Duplicate detection: none, first10mb, sample, full, smart")
	flagSet.IntVar(&r.AgeThresholdDays, "age-threshold-days", 0, "Minimum age in days for old files (default: 365)")
	flagSet.Int64Var(&r.MaxDeletedBytes, "max-deleted-bytes", 0, "Maximum total bytes to delete (0 = unlimited)")
	flagSet.BoolVar(&r.DryRun, "dry-run", false, "Only report what would be deleted")

	var paths, ignorePaths, categories string
	flagSet.StringVar(&paths, "paths", "", "Comma-separated paths to scan (required)")
	flagSet.StringVar(&ignorePaths, "ignore-paths", "", "Comma-separated paths to ignore")
	flagSet.StringVar(&categories, "categories", "", "Comma-separated categories: old, log/cache, duplicate")

	if err := flagSet.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		os.Exit(1)
	}

	r.Paths = splitTrim(paths)
	r.IgnorePaths = splitTrim(ignorePaths)
	r.Categories = splitTrim(categories)

	if r.Name == "" {
		fmt.Fprintln(os.Stderr, "create: --name is required")
		os.Exit(1)
	}
	if len(r.Paths) == 0 {
		fmt.Fprintln(os.Stderr, "create: --paths is required")
		os.Exit(1)
	}

	saveRule(r)
	fmt.Printf("Created rule %q\n", r.Name)
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func rulesList(args []string) {
	if err := flag.NewFlagSet("rules list", flag.ContinueOnError).Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		os.Exit(1)
	}

	f, err := rules.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		os.Exit(1)
	}

	names := make([]string, 0, len(f.Rules))
	for name := range f.Rules {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Println("No saved rules.")
		return
	}

	fmt.Printf("%-20s %-10s %s\n", "NAME", "ACTION", "PATHS")
	for _, name := range names {
		r := f.Rules[name]
		fmt.Printf("%-20s %-10s %s\n", r.Name, r.Action, strings.Join(r.Paths, ", "))
	}
}

func rulesShow(args []string) {
	flagSet := flag.NewFlagSet("rules show", flag.ExitOnError)
	positionals, err := parseInterspersed(flagSet, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "show: %v\n", err)
		os.Exit(1)
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "show: rule name required")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintf(os.Stderr, "show: unexpected arguments: %v\n", positionals[1:])
		os.Exit(1)
	}

	f, err := rules.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "show: %v\n", err)
		os.Exit(1)
	}
	r, ok := f.Get(positionals[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "show: rule not found\n")
		os.Exit(1)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "show: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func rulesEdit(args []string) {
	flagSet := flag.NewFlagSet("rules edit", flag.ExitOnError)
	positionals, err := parseInterspersed(flagSet, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "edit: rule name required")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintf(os.Stderr, "edit: unexpected arguments: %v\n", positionals[1:])
		os.Exit(1)
	}
	name := positionals[0]

	f, err := rules.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}
	r, ok := f.Get(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "edit: rule not found\n")
		os.Exit(1)
	}

	tmp, err := os.CreateTemp("", "cleanup-tool-rule-*.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}
	if err := writeRuleJSON(tmp.Name(), r); err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "edit: editor failed: %v\n", err)
		os.Exit(1)
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}
	var updated rules.Rule
	if err := json.Unmarshal(data, &updated); err != nil {
		fmt.Fprintf(os.Stderr, "edit: invalid JSON after edit: %v\n", err)
		os.Exit(1)
	}
	updated.Name = name
	if err := updated.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "edit: invalid rule: %v\n", err)
		os.Exit(1)
	}
	f.Set(updated)
	if err := f.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "edit: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Updated rule %q\n", name)
}

func rulesDelete(args []string) {
	flagSet := flag.NewFlagSet("rules delete", flag.ExitOnError)
	positionals, err := parseInterspersed(flagSet, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete: %v\n", err)
		os.Exit(1)
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "delete: rule name required")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintf(os.Stderr, "delete: unexpected arguments: %v\n", positionals[1:])
		os.Exit(1)
	}
	name := positionals[0]

	f, err := rules.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete: %v\n", err)
		os.Exit(1)
	}
	if _, ok := f.Get(name); !ok {
		fmt.Fprintf(os.Stderr, "delete: rule not found\n")
		os.Exit(1)
	}
	f.Delete(name)
	if err := f.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "delete: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Deleted rule %q\n", name)
}

func rulesRun(args []string) {
	flagSet := flag.NewFlagSet("rules run", flag.ExitOnError)
	yes := flagSet.Bool("yes", false, "Skip confirmation prompt")
	dryRun := flagSet.Bool("dry-run", false, "Force dry-run mode")
	positionals, err := parseInterspersed(flagSet, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "run: rule name required")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintf(os.Stderr, "run: unexpected arguments: %v\n", positionals[1:])
		os.Exit(1)
	}
	name := positionals[0]

	f, err := rules.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
	r, ok := f.Get(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "run: rule not found\n")
		os.Exit(1)
	}

	if err := r.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "run: invalid rule: %v\n", err)
		os.Exit(1)
	}

	opts := executor.Options{Yes: *yes, DryRun: *dryRun}
	res := executor.ExecuteRule(context.Background(), r, opts)
	res.WriteSummary(os.Stdout)
	if res.Error != nil {
		os.Exit(1)
	}
}

func saveRule(r rules.Rule) {
	if err := r.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "create: invalid rule: %v\n", err)
		os.Exit(1)
	}

	f, err := rules.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		os.Exit(1)
	}
	f.Set(r)
	if err := f.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		os.Exit(1)
	}
}

func writeRuleJSON(path string, r rules.Rule) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
