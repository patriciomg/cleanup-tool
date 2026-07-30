package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/config"
	"github.com/patriciomg/cleanup-tool/internal/defaults"
	"github.com/patriciomg/cleanup-tool/internal/deps"
	"github.com/patriciomg/cleanup-tool/internal/utils"
)

// handleDepsCmd dispatches the "deps" subcommand.
func handleDepsCmd(args []string) {
	flagSet := flag.NewFlagSet("deps", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)

	var pathsFlag, targetsFlag, sortFlag string
	var ignoreHidden, jsonOut bool
	flagSet.StringVar(&pathsFlag, "paths", "", "Comma-separated paths to scan (default: ~)")
	flagSet.StringVar(&targetsFlag, "targets", "", "Comma-separated dependency directory names to find (default: from config, then built-in defaults)")
	flagSet.StringVar(&sortFlag, "sort", "size", "Sort by: size, access, mod, path")
	flagSet.BoolVar(&ignoreHidden, "ignore-hidden", false, "Ignore hidden files and directories")
	flagSet.BoolVar(&jsonOut, "json", false, "Output results as JSON")

	if err := flagSet.Parse(args); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "deps: %v\n", err)
		os.Exit(1)
	}

	if flagSet.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "deps: unexpected positional arguments: %v\n", flagSet.Args())
		os.Exit(1)
	}

	paths := splitTrim(pathsFlag)
	if len(paths) == 0 {
		paths = []string{utils.ExpandHome("~")}
	}
	for i, p := range paths {
		abs, err := filepath.Abs(utils.ExpandHome(p))
		if err != nil {
			fmt.Fprintf(os.Stderr, "deps: invalid path %q: %v\n", p, err)
			os.Exit(1)
		}
		paths[i] = abs
	}

	var targets []string
	if len(targetsFlag) > 0 {
		targets = splitTrim(targetsFlag)
	}

	cfg, err := config.Load()
	if err != nil {
		if len(targets) == 0 {
			fmt.Fprintf(os.Stderr, "deps: warning: config load error, using built-in defaults: %v\n", err)
			targets = defaults.DepsTargets()
		} else {
			fmt.Fprintf(os.Stderr, "deps: warning: config load error: %v\n", err)
		}
		cfg = &config.Config{}
	} else if len(targets) == 0 {
		targets = cfg.DepsTargets
		if len(targets) == 0 {
			targets = defaults.DepsTargets()
		}
	}

	switch sortFlag {
	case "size", "access", "mod", "path":
	default:
		fmt.Fprintf(os.Stderr, "deps: invalid sort %q; valid: size, access, mod, path\n", sortFlag)
		os.Exit(1)
	}

	finder := deps.NewFinder(targets, cfg.IgnorePaths, ignoreHidden)
	results, err := finder.Find(context.Background(), paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deps: %v\n", err)
		os.Exit(1)
	}

	deps.SortResults(results, sortFlag)

	if jsonOut {
		if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
			fmt.Fprintf(os.Stderr, "deps: json encode: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(results) == 0 {
		fmt.Println("No dependency directories found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PATH\tTYPE\tSIZE\tLAST ACCESS\tLAST MODIFIED")

	var total int64
	for _, d := range results {
		total += d.Size
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			d.Path,
			d.Type,
			d.PrettySize(),
			d.AccessTime.Format("2006-01-02 15:04"),
			d.ModTime.Format("2006-01-02 15:04"),
		)
	}
	_ = w.Flush()

	fmt.Println()
	fmt.Printf("Found %d dependency directories, total size %s\n", len(results), analyzer.PrettySize(total))
}
