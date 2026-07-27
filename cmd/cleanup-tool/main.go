package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/config"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/tui"
	"github.com/patriciomg/cleanup-tool/internal/utils"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "rules":
			handleRulesCmd(os.Args[2:])
			return
		case "schedule":
			handleScheduleCmd(os.Args[2:])
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}

	var (
		pathsFlag        string
		externalFlag     string
		showVersion      bool
		benchmark        bool
		dupModeFlag      string
		progressInterval int
	)

	ignoreHiddenFlag := &boolFlag{value: cfg.IgnoreHidden}

	flag.StringVar(&pathsFlag, "paths", "", "Comma-separated paths to scan (default: ~)")
	flag.StringVar(&externalFlag, "external", "", "External drive directory for move action")
	flag.Var(ignoreHiddenFlag, "ignore-hidden", "Ignore hidden files and directories")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&benchmark, "benchmark", false, "Benchmark scan and print throughput to stdout")
	flag.StringVar(&dupModeFlag, "dup-mode", cfg.DupMode, "Duplicate detection mode: first10mb, sample, full, smart")
	flag.IntVar(&progressInterval, "progress-interval", cfg.ProgressInterval, "Report analyzer progress every N files")
	flag.Parse()

	ignoreHidden := cfg.IgnoreHidden
	if ignoreHiddenFlag.set {
		ignoreHidden = ignoreHiddenFlag.value
	}

	if showVersion {
		fmt.Println("cleanup-tool v0.2.0")
		return
	}

	var dupMode analyzer.DupHashMode
	switch strings.ToLower(dupModeFlag) {
	case "first10mb", "first":
		dupMode = analyzer.DupHashFirst10MB
	case "sample":
		dupMode = analyzer.DupHashSample
	case "full":
		dupMode = analyzer.DupHashFull
	case "smart":
		dupMode = analyzer.DupHashSmart
	default:
		fmt.Fprintf(os.Stderr, "invalid dup-mode %q; valid: first10mb, sample, full, smart\n", dupModeFlag)
		os.Exit(1)
	}

	if progressInterval <= 0 {
		fmt.Fprintf(os.Stderr, "progress-interval must be > 0\n")
		os.Exit(1)
	}

	var paths []string
	if pathsFlag != "" {
		parts := strings.Split(pathsFlag, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			abs, err := filepath.Abs(utils.ExpandHome(p))
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid path %q: %v\n", p, err)
				os.Exit(1)
			}
			paths = append(paths, abs)
		}
	} else {			paths = []string{utils.ExpandHome("~")}
	}

	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no paths to scan")
		os.Exit(1)
	}

	if benchmark {
		runBenchmark(paths, cfg.IgnorePaths, ignoreHidden)
		return
	}

	dockerClient := docker.NewClient()
	if err := tui.RunWithScan(paths, cfg.IgnorePaths, ignoreHidden, utils.ExpandHome(externalFlag), dockerClient, dupMode, progressInterval); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
}

// boolFlag is a boolean flag.Value that tracks whether the flag was explicitly set.
type boolFlag struct {
	value bool
	set   bool
}

func (b *boolFlag) String() string  { return strconv.FormatBool(b.value) }
func (b *boolFlag) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	b.value = v
	b.set = true
	return nil
}

// IsBoolFlag lets the flag package accept -ignore-hidden without a value.
func (b *boolFlag) IsBoolFlag() bool { return true }

// parseInterspersed parses a FlagSet where flags may appear after positional
// arguments (e.g., `cmd name --flag`). It returns the collected positional
// arguments. Flags with values, boolean flags, and the `--` terminator are
// handled correctly.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		remainder := fs.Args()
		if len(remainder) == 0 {
			break
		}
		// If the parser stopped at `--`, everything after is positional.
		consumed := len(args) - len(remainder)
		if consumed > 0 && args[consumed-1] == "--" {
			positionals = append(positionals, remainder...)
			break
		}
		// Collect the first unparsed argument and continue parsing the rest.
		positionals = append(positionals, remainder[0])
		args = remainder[1:]
	}
	return positionals, nil
}

func runBenchmark(paths []string, ignorePaths []string, ignoreHidden bool) {
	start := time.Now()
	scanner := analyzer.NewScanner(ignorePaths, ignoreHidden, 0)
	roots, err := scanner.Scan(context.Background(), paths)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
		os.Exit(1)
	}

	var totalFiles, totalDirs int64
	var totalSize int64
	for _, r := range roots {
		totalFiles += r.NumFiles
		totalDirs += r.NumDirs
		totalSize += r.Size
		// Count the root directory itself.
		if r.IsDir {
			totalDirs++
		}
	}
	secs := elapsed.Seconds()

	fmt.Println("Scan benchmark")
	fmt.Printf("Paths: %s\n", strings.Join(paths, ", "))
	fmt.Printf("Total time: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Files: %d\n", totalFiles)
	fmt.Printf("Dirs:  %d\n", totalDirs)
	if secs > 0 {
		fmt.Printf("Avg throughput: %.0f files/sec, %.0f dirs/sec\n", float64(totalFiles)/secs, float64(totalDirs)/secs)
	}
	fmt.Printf("Total size: %s\n", analyzer.PrettySize(totalSize))
}


