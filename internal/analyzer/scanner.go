package analyzer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/categories"
)

// Progress reports scan progress.
type Progress struct {
	Files int64
	Dirs  int64
	Path  string
}

// ProgressFunc is called periodically while scanning.
type ProgressFunc func(Progress)

// Scanner walks file trees and builds Entry trees.
type Scanner struct {
	IgnorePaths  []string
	IgnoreHidden bool
	OnProgress   ProgressFunc
	progressStep int

	// progressCount is incremented atomically from many goroutines.
	progressCount uint64
	// filesScanned and dirsScanned track absolute totals for progress reporting.
	filesScanned int64
	dirsScanned  int64

	// readDirSem limits the number of concurrent directory reads. This is the
	// operation that consumes file descriptors, so bounding it avoids ulimit
	// exhaustion while still allowing the walk to be highly parallel.
	readDirSem chan struct{}
	// statSem limits the number of concurrent file/directory metadata lookups.
	// Without it, scanning a directory with many children could spawn an
	// unbounded number of goroutines.
	statSem chan struct{}
}

// NewScanner creates a new Scanner.
func NewScanner(ignore []string, ignoreHidden bool, progressStep int) *Scanner {
	if progressStep < 0 {
		progressStep = 0
	}
	s := &Scanner{
		IgnorePaths:  ignore,
		IgnoreHidden: ignoreHidden,
		progressStep: progressStep,
		readDirSem:   make(chan struct{}, defaultReadDirConcurrency()),
		statSem:      make(chan struct{}, defaultStatConcurrency()),
	}
	return s
}

func defaultReadDirConcurrency() int {
	// Bound concurrent directory reads. A small multiple of the CPU count is
	// usually enough to saturate a fast SSD without keeping thousands of
	// directory file descriptors open at once.
	n := runtime.GOMAXPROCS(0) * 4
	if n < 64 {
		n = 64
	}
	if n > 256 {
		n = 256
	}
	return n
}

func defaultStatConcurrency() int {
	// File/directory metadata lookups are cheaper than directory reads, so
	// allow more concurrency, but still cap it to avoid resource exhaustion.
	n := runtime.GOMAXPROCS(0) * 8
	if n < 128 {
		n = 128
	}
	if n > 512 {
		n = 512
	}
	return n
}

func (s *Scanner) shouldSkip(path string, d fs.DirEntry) bool {
	if s.IgnoreHidden {
		name := d.Name()
		if strings.HasPrefix(name, ".") && name != "." && name != ".." {
			return true
		}
	}
	for _, p := range s.IgnorePaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// Scan walks all roots concurrently and returns the root entries.
func (s *Scanner) Scan(ctx context.Context, roots []string) ([]*Entry, error) {
	var results []*Entry
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, root := range roots {
		root := root
		wg.Add(1)
		go func() {
			defer wg.Done()

			info, err := os.Stat(root)
			if err != nil {
				mu.Lock()
				results = append(results, &Entry{Path: root, Name: filepath.Base(root), Error: err, Scanned: false})
				mu.Unlock()
				return
			}

			entry, _ := s.walk(ctx, root, info)
			mu.Lock()
			results = append(results, entry)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results, nil
}

func (s *Scanner) reportProgress(p Progress) {
	if s.OnProgress == nil {
		return
	}

	if p.Files > 0 {
		atomic.AddInt64(&s.filesScanned, p.Files)
	}
	if p.Dirs > 0 {
		atomic.AddInt64(&s.dirsScanned, p.Dirs)
	}

	count := atomic.AddUint64(&s.progressCount, 1)
	if s.progressStep > 0 && count%uint64(s.progressStep) == 0 {
		s.OnProgress(Progress{
			Files: atomic.LoadInt64(&s.filesScanned),
			Dirs:  atomic.LoadInt64(&s.dirsScanned),
			Path:  p.Path,
		})
	}
}

// entryFromInfo builds a basic Entry from os.FileInfo.
func entryFromInfo(path string, info fs.FileInfo) *Entry {
	entry := &Entry{
		Path:    path,
		Name:    info.Name(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Mode:    info.Mode(),
		IsDir:   info.IsDir(),
		Scanned: true,
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.AccessTime = time.Unix(stat.Atimespec.Sec, int64(stat.Atimespec.Nsec))
	} else {
		entry.AccessTime = info.ModTime()
	}

	return entry
}

func (s *Scanner) readDirWithSem(ctx context.Context, path string) ([]fs.DirEntry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.readDirSem <- struct{}{}:
		entries, err := os.ReadDir(path)
		<-s.readDirSem
		return entries, err
	}
}

func (s *Scanner) walk(ctx context.Context, path string, info fs.FileInfo) (*Entry, error) {
	if err := ctx.Err(); err != nil {
		return &Entry{Path: path, Name: filepath.Base(path), Error: err, Scanned: false}, nil
	}

	entry := entryFromInfo(path, info)

	if !info.IsDir() {
		entry.Category = categories.Classify(path, info.Name())
		s.reportProgress(Progress{Files: 1, Path: path})
		return entry, nil
	}

	// Directory: reset own size to avoid double counting metadata, then sum children.
	entry.Size = 0

	entries, err := s.readDirWithSem(ctx, path)
	if err != nil {
		if err == ctx.Err() {
			entry.Error = err
			return &Entry{Path: path, Name: filepath.Base(path), Error: err, Scanned: false}, nil
		}
		entry.Error = err
		return entry, nil
	}

	// Process all children concurrently. Metadata lookups are bounded by
	// statSem to avoid goroutine explosion in directories with many entries.
	children := make([]*Entry, len(entries))
	var childWg sync.WaitGroup

loop:
	for i, d := range entries {
		if err := ctx.Err(); err != nil {
			break
		}

		childPath := filepath.Join(path, d.Name())
		if s.shouldSkip(childPath, d) {
			continue
		}

		// Use the stat semaphore as back-pressure so directories with tens of
		// thousands of children do not spawn tens of thousands of goroutines.
		select {
		case <-ctx.Done():
			break loop
		case s.statSem <- struct{}{}:
		}

		childWg.Add(1)
		go func(i int, d fs.DirEntry, childPath string) {
			defer childWg.Done()
			children[i] = s.processChild(ctx, childPath, d)
		}(i, d, childPath)
	}

	childWg.Wait()

	// Aggregate child results. This is sequential and safe because all children
	// have finished writing their slots.
	for _, child := range children {
		if child == nil {
			continue
		}
		entry.AddChild(child)
		entry.Size += child.Size
		if child.IsDir {
			entry.NumDirs += 1 + child.NumDirs
			entry.NumFiles += child.NumFiles
		} else {
			entry.NumFiles++
		}
	}

	entry.Category = categories.Classify(path, info.Name())
	s.reportProgress(Progress{Dirs: 1, Path: path})
	return entry, nil
}

// processChild resolves metadata for a single directory entry and, if it is a
// directory, recursively walks it. The caller must hold one slot in statSem;
// this function releases it before returning.
func (s *Scanner) processChild(ctx context.Context, childPath string, d fs.DirEntry) *Entry {
	childInfo, err := d.Info()
	if err != nil {
		<-s.statSem
		return &Entry{Path: childPath, Name: d.Name(), Error: err, Scanned: false}
	}

	// d.Info() may return symlink info without following the target. The
	// previous implementation used os.Stat, so follow symlinks to preserve
	// the same semantics.
	if childInfo.Mode()&os.ModeSymlink != 0 {
		resolved, err := os.Stat(childPath)
		if err != nil {
			<-s.statSem
			return &Entry{Path: childPath, Name: d.Name(), Error: err, Scanned: false}
		}
		childInfo = resolved
	}
	<-s.statSem

	if childInfo.IsDir() {
		child, _ := s.walk(ctx, childPath, childInfo)
		return child
	}

	child := entryFromInfo(childPath, childInfo)
	child.Category = categories.Classify(childPath, childInfo.Name())
	s.reportProgress(Progress{Files: 1, Path: childPath})
	return child
}
