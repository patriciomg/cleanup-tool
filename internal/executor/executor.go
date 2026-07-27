package executor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/actions"
	"github.com/patriciomg/cleanup-tool/internal/analyzer"
	"github.com/patriciomg/cleanup-tool/internal/config"
	"github.com/patriciomg/cleanup-tool/internal/rules"
	"github.com/patriciomg/cleanup-tool/internal/utils"
)

// Options controls how a rule is executed.
type Options struct {
	// Yes skips the interactive confirmation prompt.
	Yes bool
	// DryRun forces a dry run even if the rule says otherwise.
	DryRun bool
	// Stdin/Stdout/Stderr can be overridden for testing.
	Stdin  *bufio.Reader
	Stdout *os.File
	Stderr *os.File
}

// Result reports what happened during a rule run.
type Result struct {
	RuleName       string
	ScannedPaths   []string
	HintsFound     int
	MatchedFiles   int
	MatchedBytes   int64
	Action         string
	DeletedPaths   []string
	DeletedBytes   int64
	DryRun         bool
	AbortedMaxSize bool
	Error          error
}

func (r Result) WriteSummary(out io.Writer) {
	if r.Error != nil {
		fmt.Fprintf(out, "Rule %q failed: %v\n", r.RuleName, r.Error)
		return
	}
	fmt.Fprintf(out, "Rule: %s\n", r.RuleName)
	fmt.Fprintf(out, "Hints found: %d\n", r.HintsFound)
	fmt.Fprintf(out, "Matched files: %d (%s)\n", r.MatchedFiles, analyzer.PrettySize(r.MatchedBytes))
	if r.AbortedMaxSize {
		fmt.Fprintf(out, "Aborted: matched size exceeded max_deleted_bytes\n")
		return
	}
	if r.DryRun {
		fmt.Fprintf(out, "Dry run: no files were modified\n")
		return
	}
	fmt.Fprintf(out, "Action: %s\n", r.Action)
	fmt.Fprintf(out, "Deleted files: %d (%s)\n", len(r.DeletedPaths), analyzer.PrettySize(r.DeletedBytes))
}

// ExecuteRule runs a saved rule non-interactively.
func ExecuteRule(ctx context.Context, rule rules.Rule, opts Options) Result {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = bufio.NewReader(os.Stdin)
	}

	res := Result{RuleName: rule.Name, Action: rule.Action, DryRun: rule.DryRun || opts.DryRun}

	if err := rule.Validate(); err != nil {
		res.Error = fmt.Errorf("validate rule: %w", err)
		return res
	}

	paths := utils.ExpandHomeSlice(rule.Paths)
	res.ScannedPaths = paths

	// Build ignore list by merging rule-specific ignores with global defaults.
	cfg, err := config.Load()
	if err != nil {
		res.Error = fmt.Errorf("load config: %w", err)
		return res
	}
	ignorePaths := utils.ExpandHomeSlice(cfg.IgnorePaths)
	ignorePaths = append(ignorePaths, utils.ExpandHomeSlice(rule.IgnorePaths)...)

	scanner := analyzer.NewScanner(ignorePaths, rule.IgnoreHidden, 0)
	roots, err := scanner.Scan(ctx, paths)
	if err != nil {
		res.Error = fmt.Errorf("scan: %w", err)
		return res
	}

	dupMode := parseDupMode(rule.DupMode)

	var ageThreshold time.Duration
	if rule.AgeThresholdDays > 0 {
		ageThreshold = time.Duration(rule.AgeThresholdDays) * 24 * time.Hour
	}

	var allHints []*analyzer.DeletabilityHint
	for _, root := range roots {
		hints, err := analyzer.FindHintsWithOptions(ctx, root, analyzer.HintOptions{
			DupMode:      dupMode,
			AgeThreshold: ageThreshold,
		})
		if err != nil {
			res.Error = fmt.Errorf("analyze: %w", err)
			return res
		}
		allHints = append(allHints, hints...)
	}
	res.HintsFound = len(allHints)

	filtered := filterHints(allHints, rule)
	res.MatchedFiles = len(filtered)

	var total int64
	for _, h := range filtered {
		total += h.Entry.Size
	}
	res.MatchedBytes = total

	if rule.MaxDeletedBytes > 0 && total > rule.MaxDeletedBytes {
		res.AbortedMaxSize = true
		return res
	}

	if len(filtered) == 0 {
		return res
	}

	if res.DryRun {
		return res
	}

	if !opts.Yes && isTerminal(opts.Stdout) {
		fmt.Fprintf(opts.Stdout, "Delete %d files (%s)? [y/N] ", len(filtered), analyzer.PrettySize(total))
		text, err := opts.Stdin.ReadString('\n')
		if err != nil {
			res.Error = fmt.Errorf("read confirmation: %w", err)
			return res
		}
		text = strings.TrimSpace(strings.ToLower(text))
		if text != "y" && text != "yes" {
			res.Error = fmt.Errorf("cancelled by user")
			return res
		}
	}

	pathsToDelete := make([]string, len(filtered))
	for i, h := range filtered {
		pathsToDelete[i] = h.Entry.Path
	}

	res.DeletedBytes = total
	res.DeletedPaths = pathsToDelete

	switch rule.Action {
	case "trash":
		if err := actions.Trash(pathsToDelete...); err != nil {
			res.Error = fmt.Errorf("trash: %w", err)
			return res
		}
	case "move_external":
		dest := utils.ExpandHome(rule.Destination)
		if err := actions.MoveToExternal(dest, pathsToDelete...); err != nil {
			res.Error = fmt.Errorf("move: %w", err)
			return res
		}
	}

	return res
}

func parseDupMode(s string) analyzer.DupHashMode {
	switch strings.ToLower(s) {
	case "none":
		return analyzer.DupHashNone
	case "first10mb", "first":
		return analyzer.DupHashFirst10MB
	case "sample":
		return analyzer.DupHashSample
	case "full":
		return analyzer.DupHashFull
	case "smart":
		return analyzer.DupHashSmart
	default:
		return analyzer.DupHashSmart
	}
}

func filterHints(hints []*analyzer.DeletabilityHint, rule rules.Rule) []*analyzer.DeletabilityHint {
	categories := rule.Categories
	if len(categories) == 0 {
		categories = []string{"old", "log/cache", "duplicate"}
	}
	include := make(map[string]bool)
	for _, c := range categories {
		include[c] = true
	}

	var result []*analyzer.DeletabilityHint
	for _, h := range hints {
		if !include[string(h.Reason)] {
			continue
		}
		result = append(result, h)
	}
	return result
}

func isTerminal(out *os.File) bool {
	if out == nil {
		return false
	}
	info, err := out.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
