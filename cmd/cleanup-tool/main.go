package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/config"
	"github.com/patriciomg/cleanup-tool/internal/docker"
	"github.com/patriciomg/cleanup-tool/internal/tui/dua"
	"github.com/patriciomg/cleanup-tool/internal/tui/terminal"
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
		case "models":
			handleModelsCmd(os.Args[2:])
			return
		case "deps":
			handleDepsCmd(os.Args[2:])
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
		out              string
		json             bool
		stdout           bool
		format           string
		csvColumns       string
		dupModeFlag      string
		progressInterval int
		tuiStyle         string
	)

	ignoreHiddenFlag := &boolFlag{value: cfg.IgnoreHidden}

	flag.StringVar(&pathsFlag, "paths", "", "Comma-separated paths to scan (default: ~)")
	flag.StringVar(&externalFlag, "external", "", "External drive directory for move action")
	flag.Var(ignoreHiddenFlag, "ignore-hidden", "Ignore hidden files and directories")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&benchmark, "benchmark", false, "Benchmark scan and print throughput to stdout")
	flag.StringVar(&out, "out", "", "Export scan results to the specified file (implies non-interactive)")
	flag.BoolVar(&json, "json", false, "Export scan results to stdout (non-interactive)")
	flag.BoolVar(&stdout, "stdout", false, "Export scan results to stdout (works with any -format; alias for -json)")
	flag.StringVar(&format, "format", "json", "Export format: json, csv, tsv, yaml (default: json; auto-detected from -out extension when not specified)")
	flag.StringVar(&csvColumns, "csv-columns", "", "Comma-separated CSV/TSV column names (default: all columns)")
	flag.StringVar(&dupModeFlag, "dup-mode", cfg.DupMode, "Duplicate detection mode: first10mb, sample, full, smart")
	flag.IntVar(&progressInterval, "progress-interval", cfg.ProgressInterval, "Report analyzer progress every N files")
	flag.StringVar(&tuiStyle, "tui-style", "dua", "Interactive TUI style: terminal or dua")
	flag.Parse()

	ignoreHidden := cfg.IgnoreHidden
	if ignoreHiddenFlag.set {
		ignoreHidden = ignoreHiddenFlag.value
	}

	if showVersion {
		fmt.Println("cleanup-tool v0.3.0")
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

	format = strings.ToLower(format)

	// Auto-detect format from -out file extension unless the user explicitly
	// provided -format. An unknown extension is an error to avoid silently
	// writing JSON (or another format) to a file with a misleading extension.
	if !formatFlagExplicitlySet() && out != "" {
		format, err = resolveFormat(out, format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "format error: %v\n", err)
			os.Exit(1)
		}
	}

	switch format {
	case "json", "csv", "tsv", "yaml":
	default:
		fmt.Fprintf(os.Stderr, "invalid format %q; valid: json, csv, tsv, yaml\n", format)
		os.Exit(1)
	}

	if benchmark || out != "" || json || stdout {
		columns, err := parseCSVColumns(csvColumns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "csv-columns error: %v\n", err)
			os.Exit(1)
		}
		runNonInteractive(paths, cfg.IgnorePaths, ignoreHidden, benchmark, out, json || stdout, format, columns)
		return
	}

	tuiStyle = strings.ToLower(tuiStyle)
	style, err := resolveTUIStyle(tuiStyle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -tui-style %q; valid: terminal, dua\n", tuiStyle)
		os.Exit(1)
	}
	switch style {
	case "terminal":
		dockerClient := docker.NewClient()
		if err := terminal.RunWithScan(paths, cfg.IgnorePaths, ignoreHidden, utils.ExpandHome(externalFlag), dockerClient, dupMode, progressInterval); err != nil {
			fmt.Fprintf(os.Stderr, "terminal error: %v\n", err)
			os.Exit(1)
		}
	case "dua":
		dockerClient := docker.NewClient()
		if err := dua.RunWithScan(paths, cfg.IgnorePaths, ignoreHidden, utils.ExpandHome(externalFlag), dockerClient, dupMode, progressInterval); err != nil {
			fmt.Fprintf(os.Stderr, "dua error: %v\n", err)
			os.Exit(1)
		}
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

func runNonInteractive(paths []string, ignorePaths []string, ignoreHidden bool, benchmark bool, outFile string, stdout bool, format string, csvColumns []string) {
	start := time.Now()
	scanner := analyzer.NewScanner(ignorePaths, ignoreHidden, 0)
	roots, err := scanner.Scan(context.Background(), paths)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
		os.Exit(1)
	}

	if benchmark {
		printBenchmarkStats(paths, roots, elapsed)
	}

	if outFile == "" && !stdout {
		return
	}

	var w io.Writer = os.Stdout
	if outFile != "" {
		file, err := os.Create(outFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "export error: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		w = file
	}

	if (format == "csv" || format == "tsv") && len(csvColumns) > 0 {
		var exporter analyzer.Exporter
		if format == "tsv" {
			exporter = analyzer.NewTSVExporter(csvColumns)
		} else {
			exporter = analyzer.NewCSVExporter(csvColumns)
		}
		if err := exporter.Export(roots, w); err != nil {
			fmt.Fprintf(os.Stderr, "export error: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := analyzer.Export(roots, w, format); err != nil {
			fmt.Fprintf(os.Stderr, "export error: %v\n", err)
			os.Exit(1)
		}
	}
	if outFile != "" {
		fmt.Printf("Exported scan results to %s\n", outFile)
	}
}

// formatFlagExplicitlySet returns true if the -format flag was provided on
// the command line.
func formatFlagExplicitlySet() bool {
	var set bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "format" {
			set = true
		}
	})
	return set
}

// formatFromExtension infers an export format from a file extension.
// Supported: .json, .csv, .tsv, .yaml, .yml. Returns empty for unknown
// extensions.
func formatFromExtension(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}

// resolveFormat returns the format to use for the given output path. If the
// path has a supported extension, that format is returned. If it has no
// extension, the current default format is returned. If it has an unknown
// extension, an error is returned.
func resolveFormat(out, defaultFormat string) (string, error) {
	if extFmt := formatFromExtension(out); extFmt != "" {
		return extFmt, nil
	}
	if filepath.Ext(out) == "" {
		return defaultFormat, nil
	}
	return "", fmt.Errorf("unsupported output extension %q; use -format to choose json, csv, tsv, or yaml", filepath.Ext(out))
}

func parseCSVColumns(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	// Build a case-insensitive map of supported CSV column names.
	allowed := map[string]string{}
	for _, c := range analyzer.CSVColumns() {
		allowed[strings.ToLower(c)] = c
	}

	parts := strings.Split(s, ",")
	var cols []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if canon, ok := allowed[strings.ToLower(p)]; ok {
			cols = append(cols, canon)
		} else {
			return nil, fmt.Errorf("unknown CSV column %q", p)
		}
	}
	return cols, nil
}

func printBenchmarkStats(paths []string, roots []*analyzer.Entry, elapsed time.Duration) {
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

// resolveTUIStyle normalises a -tui-style value. It accepts "dua" for the
// dua-style browser and "terminal" or "tree" (legacy alias) for the
// terminal/tree-style browser. An empty string maps to the default style,
// which is currently "dua".
func resolveTUIStyle(style string) (string, error) {
	switch strings.ToLower(style) {
	case "":
		return "dua", nil
	case "terminal", "tree":
		return "terminal", nil
	case "dua":
		return "dua", nil
	default:
		return "", fmt.Errorf("invalid TUI style %q", style)
	}
}

