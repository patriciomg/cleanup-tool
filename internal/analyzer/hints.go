package analyzer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/categories"
)

// HintReason describes why a file might be deletable.
type HintReason string

const (
	ReasonOld       HintReason = "old"
	ReasonLogCache  HintReason = "log/cache"
	ReasonDuplicate HintReason = "duplicate"
)

// DeletabilityHint pairs an entry with a reason it may be safe to delete.
type DeletabilityHint struct {
	Entry  *Entry
	Reason HintReason
	Detail string
}

// HintSummary reports how many hints were found in each deletability category.
type HintSummary struct {
	Old       int
	LogCache  int
	Duplicate int
}

// SummarizeHints computes a per-category summary from a slice of hints.
func SummarizeHints(hints []*DeletabilityHint) HintSummary {
	var s HintSummary
	for _, h := range hints {
		if h == nil {
			continue
		}
		switch h.Reason {
		case ReasonOld:
			s.Old++
		case ReasonLogCache:
			s.LogCache++
		case ReasonDuplicate:
			s.Duplicate++
		}
	}
	return s
}

// DupHashMode selects how duplicate detection compares file contents.
type DupHashMode int

const (
	// DupHashNone skips duplicate detection entirely.
	DupHashNone DupHashMode = iota
	// DupHashFirst10MB replicates the original behaviour: hash the first 10 MB.
	DupHashFirst10MB
	// DupHashSample hashes 1 MB sampled from the start, middle and end of the file.
	DupHashSample
	// DupHashFull hashes the entire file.
	DupHashFull
	// DupHashSmart groups by size, then sample-hash, then full-hash only the
	// samples that collide. This is the most accurate mode and is still fast
	// because uniquely-sized files are skipped entirely.
	DupHashSmart
)

func (m DupHashMode) String() string {
	switch m {
	case DupHashNone:
		return "none"
	case DupHashFirst10MB:
		return "first10mb"
	case DupHashSample:
		return "sample"
	case DupHashFull:
		return "full"
	case DupHashSmart:
		return "smart"
	default:
		return "unknown"
	}
}

// AnalyzerProgress reports the current state of an in-flight analysis.
type AnalyzerProgress struct {
	Stage          string
	FilesProcessed int
	CurrentPath    string
	HintsFound     HintSummary
}

// HintOptions controls the deletability analysis.
type HintOptions struct {
	DupMode DupHashMode
	// OnProgress is called periodically while the analyzer works.
	OnProgress func(AnalyzerProgress)
	// ProgressInterval reports a progress update every N files. A value <= 0
	// defaults to 100.
	ProgressInterval int
	// AgeThreshold overrides the default 365-day threshold for "old" files.
	// A value <= 0 uses the default.
	AgeThreshold time.Duration

	state *analyzerState
}

type analyzerState struct {
	filesProcessed int
	stage          string
	currentPath    string
	summary        HintSummary
}

const (
	maxHashSize     = 10 * 1024 * 1024 // 10 MB
	hashChunk       = 64 * 1024          // 64 KB
	sampleBlockSize = 1 * 1024 * 1024    // 1 MB
)

// FindHints traverses the entry tree and returns deletability hints.
// It is CPU/I/O heavy (especially duplicate detection), so it should be run
// on demand and respect context cancellation.
// It now defaults to the smart duplicate-detection mode.
func FindHints(ctx context.Context, root *Entry) ([]*DeletabilityHint, error) {
	return FindHintsWithOptions(ctx, root, HintOptions{DupMode: DupHashSmart})
}

// FindHintsWithOptions is like FindHints but allows tuning the analysis.
func FindHintsWithOptions(ctx context.Context, root *Entry, opts HintOptions) ([]*DeletabilityHint, error) {
	if opts.OnProgress != nil && opts.state == nil {
		opts.state = &analyzerState{}
	}

	var hints []*DeletabilityHint

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	walk(root, func(e *Entry) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		if e.IsDir {
			return true
		}
		ageThreshold := opts.AgeThreshold
		if ageThreshold <= 0 {
			ageThreshold = 365 * 24 * time.Hour
		}
		if time.Since(ageTime(e)) > ageThreshold {
			hints = append(hints, &DeletabilityHint{
				Entry:  e,
				Reason: ReasonOld,
				Detail: fmt.Sprintf("last touched %s", ageTime(e).Format("2006-01-02")),
			})
			if opts.state != nil {
				opts.state.summary.Old++
			}
		}
		if e.Category == categories.LogCache {
			hints = append(hints, &DeletabilityHint{
				Entry:  e,
				Reason: ReasonLogCache,
				Detail: string(e.Category),
			})
			if opts.state != nil {
				opts.state.summary.LogCache++
			}
		}
		report(opts, "discovering hints", e.Path)
		return true
	})

	duplicates := collectDuplicates(ctx, root, opts)
	for _, group := range duplicates {
		for _, e := range group {
			hints = append(hints, &DeletabilityHint{
				Entry:  e,
				Reason: ReasonDuplicate,
				Detail: fmt.Sprintf("%d duplicates", len(group)),
			})
		}
	}

	if err := ctx.Err(); err != nil {
		return hints, err
	}
	return hints, nil
}

// collectDuplicates returns groups of entries that are likely duplicate files.
func collectDuplicates(ctx context.Context, root *Entry, opts HintOptions) [][]*Entry {
	if opts.DupMode == DupHashNone {
		return nil
	}
	if opts.DupMode == DupHashFirst10MB {
		return collectByFirst10MB(ctx, root, opts)
	}

	// Build size buckets. Uniquely sized files cannot have duplicates.
	sizeMap := make(map[int64][]*Entry)
	walk(root, func(e *Entry) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if e == nil || e.IsDir {
			return true
		}
		if e.Size > 0 {
			sizeMap[e.Size] = append(sizeMap[e.Size], e)
		} else {
			// Zero-byte files are all identical, so they can only duplicate
			// other zero-byte files. Add them to a dedicated bucket.
			sizeMap[0] = append(sizeMap[0], e)
		}
		return true
	})

	var result [][]*Entry
	for _, entries := range sizeMap {
		if len(entries) < 2 {
			continue
		}

		var finalGroups [][]*Entry
		if opts.DupMode == DupHashSample {
			finalGroups = groupBySampleHash(ctx, entries, opts)
		} else if opts.DupMode == DupHashFull {
			finalGroups = groupByFullHash(ctx, entries, opts)
		} else {
			// Smart mode: size -> sample -> full.
			for _, sampleGroup := range groupBySampleHash(ctx, entries, opts) {
				if len(sampleGroup) < 2 {
					continue
				}
				finalGroups = append(finalGroups, groupByFullHash(ctx, sampleGroup, opts)...)
			}
		}

		if len(finalGroups) > 0 {
			result = append(result, finalGroups...)
			if opts.state != nil {
				for _, g := range finalGroups {
					opts.state.summary.Duplicate += len(g)
				}
				emitProgress(opts)
			}
		}
	}

	return result
}

func collectByFirst10MB(ctx context.Context, root *Entry, opts HintOptions) [][]*Entry {
	seen := make(map[string][]*Entry)
	walk(root, func(e *Entry) bool {
		if e == nil || e.IsDir {
			return true
		}
		report(opts, "first 10 MB hash", e.Path)
		key := contentHashKey(ctx, e.Path, e.Size)
		if key != "" {
			seen[key] = append(seen[key], e)
		}
		return true
	})

	var result [][]*Entry
	for _, group := range seen {
		if len(group) >= 2 {
			result = append(result, group)
			if opts.state != nil {
				opts.state.summary.Duplicate += len(group)
			}
		}
	}
	if len(result) > 0 && opts.state != nil {
		emitProgress(opts)
	}
	return result
}

func groupBySampleHash(ctx context.Context, entries []*Entry, opts HintOptions) [][]*Entry {
	groups := make(map[string][]*Entry)
	for _, e := range entries {
		report(opts, "sample hash", e.Path)
		key, err := sampleHashKey(ctx, e.Path, e.Size)
		if err != nil {
			continue
		}
		groups[key] = append(groups[key], e)
	}
	var result [][]*Entry
	for _, g := range groups {
		if len(g) >= 2 {
			result = append(result, g)
		}
	}
	return result
}

func groupByFullHash(ctx context.Context, entries []*Entry, opts HintOptions) [][]*Entry {
	groups := make(map[string][]*Entry)
	for _, e := range entries {
		report(opts, "full hash", e.Path)
		key, err := fullHashKey(ctx, e.Path, e.Size)
		if err != nil {
			continue
		}
		groups[key] = append(groups[key], e)
	}
	var result [][]*Entry
	for _, g := range groups {
		if len(g) >= 2 {
			result = append(result, g)
		}
	}
	return result
}

func report(opts HintOptions, stage, path string) {
	if opts.OnProgress == nil || opts.state == nil {
		return
	}
	opts.state.stage = stage
	opts.state.filesProcessed++
	opts.state.currentPath = path

	interval := opts.ProgressInterval
	if interval <= 0 {
		interval = 100
	}
	// Report the first file immediately so the UI is responsive for small
	// directories, then throttle to every interval files afterwards.
	if opts.state.filesProcessed == 1 || opts.state.filesProcessed%interval == 0 {
		opts.OnProgress(AnalyzerProgress{
			Stage:          stage,
			FilesProcessed: opts.state.filesProcessed,
			CurrentPath:    path,
			HintsFound:     opts.state.summary,
		})
	}
}

// emitProgress sends the current state as a progress update without throttling.
// Use it when the summary changes (e.g. a duplicate group is confirmed) so the
// UI reflects it immediately.
func emitProgress(opts HintOptions) {
	if opts.OnProgress == nil || opts.state == nil {
		return
	}
	opts.OnProgress(AnalyzerProgress{
		Stage:          opts.state.stage,
		FilesProcessed: opts.state.filesProcessed,
		CurrentPath:    opts.state.currentPath,
		HintsFound:     opts.state.summary,
	})
}

// contentHashKey returns a hash of the first 10 MB of a file (legacy mode).
func contentHashKey(ctx context.Context, path string, size int64) string {
	if size == 0 {
		return "0:empty"
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:", size)))

	limit := int64(maxHashSize)
	if size < limit {
		limit = size
	}
	buf := make([]byte, hashChunk)
	remaining := limit
	for remaining > 0 {
		select {
		case <-ctx.Done():
			return ""
		default:
		}
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return ""
		}
		if n == 0 {
			break
		}
		h.Write(buf[:n])
		remaining -= int64(n)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// sampleHashKey returns a hash of 1 MB blocks taken from the start, middle
// and end of the file.
func sampleHashKey(ctx context.Context, path string, size int64) (string, error) {
	if size == 0 {
		return "0:empty", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:", size)))

	offsets := sampleOffsets(size)
	buf := make([]byte, sampleBlockSize)
	for _, off := range offsets {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return "", err
		}
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return "", err
		}
		h.Write(buf[:n])
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func sampleOffsets(size int64) []int64 {
	if size <= sampleBlockSize {
		return []int64{0}
	}
	mid := size / 2
	return []int64{0, mid, size - sampleBlockSize}
}

// fullHashKey returns a hash of the entire file.
func fullHashKey(ctx context.Context, path string, size int64) (string, error) {
	if size == 0 {
		return "0:empty", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:", size)))

	buf := make([]byte, hashChunk)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		if n == 0 {
			break
		}
		h.Write(buf[:n])
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ageTime returns the time to use when deciding if an entry is "old".
// Access time is preferred, but if it is more recent than the modification
// time we fall back to the modification time. This avoids marking files as
// recent just because a backup tool or indexer touched them, and makes the
// age check robust on filesystems where access time updates are flaky.
func ageTime(e *Entry) time.Time {
	if e.AccessTime.IsZero() {
		return e.ModTime
	}
	if e.ModTime.IsZero() {
		return e.AccessTime
	}
	if e.AccessTime.Before(e.ModTime) {
		return e.AccessTime
	}
	return e.ModTime
}

func walk(e *Entry, fn func(*Entry) bool) bool {
	if e == nil {
		return true
	}
	if !fn(e) {
		return false
	}
	for _, c := range e.Children {
		if !walk(c, fn) {
			return false
		}
	}
	return true
}

// FindEntryByPath returns the first entry whose path matches target.
func FindEntryByPath(root *Entry, target string) *Entry {
	var found *Entry
	walk(root, func(e *Entry) bool {
		if e.Path == target {
			found = e
			return false
		}
		return true
	})
	return found
}

// FileExists is a small helper used by the TUI to verify a path still exists
// after a soft delete.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
