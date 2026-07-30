package analyzer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, data string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return p
}

func entryFor(path string, size int64) *Entry {
	return &Entry{
		Path:     path,
		Name:     filepath.Base(path),
		Size:     size,
		Category: "",
	}
}

func TestFindHintsWithOptions_DuplicateModes(t *testing.T) {
	tmp := t.TempDir()
	data := "hello world this is the same content"

	// Two files with identical content and size.
	a := writeFile(t, tmp, "a.txt", data)
	b := writeFile(t, tmp, "b.txt", data)
	// A file with different content but same size as a.txt (padded).
	c := writeFile(t, tmp, "c.txt", data+" different ending")

	root := &Entry{
		Path:   tmp,
		Name:   "tmp",
		IsDir:  true,
		AccessTime: time.Now(),
		Children: []*Entry{
			entryFor(a, int64(len(data))),
			entryFor(b, int64(len(data))),
			entryFor(c, int64(len(data+" different ending"))),
		},
	}

	modes := []DupHashMode{DupHashFirst10MB, DupHashSample, DupHashFull, DupHashSmart}
	for _, mode := range modes {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			hints, err := FindHintsWithOptions(context.Background(), root, HintOptions{DupMode: mode})
			if err != nil {
				t.Fatalf("FindHintsWithOptions: %v", err)
			}

			var duplicates int
			for _, h := range hints {
				if h.Reason == ReasonDuplicate {
					duplicates++
				}
			}
			// a.txt and b.txt should be detected as duplicates.
			if duplicates < 2 {
				t.Fatalf("expected at least 2 duplicate hints, got %d", duplicates)
			}
		})
	}
}

func TestFindHintsWithOptions_NoFalsePositives(t *testing.T) {
	tmp := t.TempDir()
	a := writeFile(t, tmp, "a.txt", "first file content")
	b := writeFile(t, tmp, "b.txt", "second file content")

	root := &Entry{
		Path:   tmp,
		Name:   "tmp",
		IsDir:  true,
		AccessTime: time.Now(),
		Children: []*Entry{
			entryFor(a, int64(len("first file content"))),
			entryFor(b, int64(len("second file content"))),
		},
	}

	for _, mode := range []DupHashMode{DupHashSample, DupHashFull, DupHashSmart} {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			hints, err := FindHintsWithOptions(context.Background(), root, HintOptions{DupMode: mode})
			if err != nil {
				t.Fatalf("FindHintsWithOptions: %v", err)
			}
			for _, h := range hints {
				if h.Reason == ReasonDuplicate {
					t.Fatalf("unexpected duplicate hint for non-duplicate files: %+v", h)
				}
			}
		})
	}
}

func TestSampleHashKey_SmallFile(t *testing.T) {
	tmp := t.TempDir()
	data := "tiny"
	p := writeFile(t, tmp, "small.txt", data)

	key, err := sampleHashKey(context.Background(), p, int64(len(data)))
	if err != nil {
		t.Fatalf("sampleHashKey: %v", err)
	}
	if key == "" {
		t.Fatal("expected non-empty hash")
	}
}

func TestFullHashKey_DetectsDifferenceAfterFirst10MB(t *testing.T) {
	// Two files that share the same first bytes but differ at the very end.
	tmp := t.TempDir()
	base := make([]byte, 10*1024*1024+1)
	for i := range base {
		base[i] = 'x'
	}

	aData := append([]byte{}, base...)
	aData[len(aData)-1] = 'a'
	bData := append([]byte{}, base...)
	bData[len(bData)-1] = 'b'

	a := writeFile(t, tmp, "a.bin", string(aData))
	b := writeFile(t, tmp, "b.bin", string(bData))

	akey, err := fullHashKey(context.Background(), a, int64(len(aData)))
	if err != nil {
		t.Fatalf("fullHashKey: %v", err)
	}
	bkey, err := fullHashKey(context.Background(), b, int64(len(bData)))
	if err != nil {
		t.Fatalf("fullHashKey: %v", err)
	}
	if akey == bkey {
		t.Fatal("full hash should distinguish files that differ at the end")
	}
}

func TestDupHashModeString(t *testing.T) {
	if DupHashFirst10MB.String() == "" {
		t.Fatal("expected non-empty mode name")
	}
}

func TestFindHintsWithOptions_OnProgress(t *testing.T) {
	tmp := t.TempDir()
	data := "duplicate me"

	var children []*Entry
	for i := 0; i < 110; i++ {
		p := writeFile(t, tmp, fmt.Sprintf("dup%d.txt", i), data)
		children = append(children, entryFor(p, int64(len(data))))
	}

	root := &Entry{
		Path:       tmp,
		Name:       "tmp",
		IsDir:      true,
		AccessTime: time.Now(),
		Children:   children,
	}

	var progress []AnalyzerProgress
	ctx := context.Background()
	_, err := FindHintsWithOptions(ctx, root, HintOptions{
		DupMode: DupHashSmart,
		OnProgress: func(p AnalyzerProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatalf("FindHintsWithOptions: %v", err)
	}
	if len(progress) == 0 {
		t.Fatal("expected at least one progress update")
	}
}

func TestSmartMode_FullHashFallbackAvoidsFalsePositive(t *testing.T) {
	// Two files that are identical for the first 1 MB but differ at the end.
	// DupHashSample should group them; DupHashSmart should NOT.
	tmp := t.TempDir()
	base := make([]byte, 2*1024*1024)
	for i := range base {
		base[i] = byte('a')
	}

	aData := append([]byte{}, base...)
	bData := append([]byte{}, base...)
	bData[len(bData)-1] = 'z'

	a := writeFile(t, tmp, "a.bin", string(aData))
	b := writeFile(t, tmp, "b.bin", string(bData))

	root := &Entry{
		Path:   tmp,
		Name:   "tmp",
		IsDir:  true,
		AccessTime: time.Now(),
		Children: []*Entry{
			entryFor(a, int64(len(aData))),
			entryFor(b, int64(len(bData))),
		},
	}

	// Sample mode should falsely flag them because the samples match.
	sampleHints, err := FindHintsWithOptions(context.Background(), root, HintOptions{DupMode: DupHashSample})
	if err != nil {
		t.Fatalf("FindHintsWithOptions: %v", err)
	}
	if len(sampleHints) == 0 {
		t.Fatal("sample mode should have produced a duplicate hint for these files")
	}

	// Smart mode should not.
	smartHints, err := FindHintsWithOptions(context.Background(), root, HintOptions{DupMode: DupHashSmart})
	if err != nil {
		t.Fatalf("FindHintsWithOptions: %v", err)
	}
	for _, h := range smartHints {
		if h.Reason == ReasonDuplicate {
			t.Fatal("smart mode should not flag files that only share their sample blocks")
		}
	}
}

func TestFindHintsWithOptions_Cancellation(t *testing.T) {
	tmp := t.TempDir()
	data := "duplicate me"

	// Create enough files that the analyzer will run long enough to cancel.
	var children []*Entry
	for i := 0; i < 500; i++ {
		p := writeFile(t, tmp, fmt.Sprintf("dup%d.txt", i), data)
		children = append(children, entryFor(p, int64(len(data))))
	}

	root := &Entry{
		Path:       tmp,
		Name:       "tmp",
		IsDir:      true,
		AccessTime: time.Now(),
		Children:   children,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel very early from a separate goroutine so the analyzer is still running.
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()

	_, err := FindHintsWithOptions(ctx, root, HintOptions{
		DupMode:    DupHashSmart,
		OnProgress: func(p AnalyzerProgress) {},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestFindHintsWithOptions_LiveSummary(t *testing.T) {
	tmp := t.TempDir()

	// Create an old file.
	oldPath := writeFile(t, tmp, "old.txt", "old content")
	oldEntry := entryFor(oldPath, int64(len("old content")))
	oldEntry.AccessTime = time.Now().Add(-2 * 365 * 24 * time.Hour)

	// Create duplicate files.
	var dups []*Entry
	for i := 0; i < 3; i++ {
		p := writeFile(t, tmp, fmt.Sprintf("dup%d.txt", i), "duplicate me")
		e := entryFor(p, int64(len("duplicate me")))
		e.AccessTime = time.Now()
		dups = append(dups, e)
	}

	root := &Entry{
		Path:       tmp,
		Name:       "tmp",
		IsDir:      true,
		AccessTime: time.Now(),
		Children:   append([]*Entry{oldEntry}, dups...),
	}

	var progress []AnalyzerProgress
	_, err := FindHintsWithOptions(context.Background(), root, HintOptions{
		DupMode: DupHashSmart,
		OnProgress: func(p AnalyzerProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatalf("FindHintsWithOptions: %v", err)
	}
	if len(progress) == 0 {
		t.Fatal("expected at least one progress update")
	}

	last := progress[len(progress)-1]
	if last.HintsFound.Old != 1 {
		t.Fatalf("expected 1 old hint, got %d", last.HintsFound.Old)
	}
	if last.HintsFound.Duplicate != 3 {
		t.Fatalf("expected 3 duplicate hints, got %d", last.HintsFound.Duplicate)
	}

	// Confirm the duplicate count was communicated during the analysis.
	var sawDuplicate bool
	for _, p := range progress {
		if p.HintsFound.Duplicate > 0 {
			sawDuplicate = true
			break
		}
	}
	if !sawDuplicate {
		t.Fatal("expected a live duplicate count update")
	}
}

func TestFindHintsWithOptions_ProgressInterval(t *testing.T) {
	tmp := t.TempDir()
	data := "duplicate me"

	var children []*Entry
	for i := 0; i < 10; i++ {
		p := writeFile(t, tmp, fmt.Sprintf("dup%d.txt", i), data)
		children = append(children, entryFor(p, int64(len(data))))
	}

	root := &Entry{
		Path:       tmp,
		Name:       "tmp",
		IsDir:      true,
		AccessTime: time.Now(),
		Children:   children,
	}

	var progress []AnalyzerProgress
	_, err := FindHintsWithOptions(context.Background(), root, HintOptions{
		DupMode:          DupHashSmart,
		ProgressInterval: 5,
		OnProgress: func(p AnalyzerProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatalf("FindHintsWithOptions: %v", err)
	}
	// In smart mode each file is reported twice (sample + full), so 10 files
	// produce 20 reports. With ProgressInterval=5 and the first report at 1,
	// we expect updates at 1, 5, 10, 15, 20.
	if len(progress) < 4 {
		t.Fatalf("expected at least 4 progress updates, got %d", len(progress))
	}
}

func TestAgeTime(t *testing.T) {
	old := time.Now().Add(-2 * 365 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)

	tests := []struct {
		name      string
		access    time.Time
		mod       time.Time
		wantOld   time.Time
	}{
		{"both zero values", time.Time{}, time.Time{}, time.Time{}},
		{"access older", old, recent, old},
		{"mod older", recent, old, old},
		{"mod zero uses access", old, time.Time{}, old},
		{"access zero uses mod", time.Time{}, old, old},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &Entry{AccessTime: tc.access, ModTime: tc.mod}
			got := ageTime(e)
			if !got.Equal(tc.wantOld) {
				t.Fatalf("ageTime() = %v, want %v", got, tc.wantOld)
			}
		})
	}
}

func TestSummarizeHints(t *testing.T) {
	hints := []*DeletabilityHint{
		{Entry: &Entry{Path: "/old/1"}, Reason: ReasonOld},
		{Entry: &Entry{Path: "/old/2"}, Reason: ReasonOld},
		{Entry: &Entry{Path: "/log/1"}, Reason: ReasonLogCache},
		{Entry: &Entry{Path: "/dup/1"}, Reason: ReasonDuplicate},
		{Entry: &Entry{Path: "/dup/2"}, Reason: ReasonDuplicate},
		{Entry: &Entry{Path: "/dup/3"}, Reason: ReasonDuplicate},
	}
	s := SummarizeHints(hints)
	if s.Old != 2 {
		t.Fatalf("expected 2 old hints, got %d", s.Old)
	}
	if s.LogCache != 1 {
		t.Fatalf("expected 1 log/cache hint, got %d", s.LogCache)
	}
	if s.Duplicate != 3 {
		t.Fatalf("expected 3 duplicate hints, got %d", s.Duplicate)
	}
}

func TestSummarizeHints_SkipsNil(t *testing.T) {
	hints := []*DeletabilityHint{
		{Entry: &Entry{Path: "/old/1"}, Reason: ReasonOld},
		nil,
	}
	s := SummarizeHints(hints)
	if s.Old != 1 || s.LogCache != 0 || s.Duplicate != 0 {
		t.Fatalf("expected nil-safe summary, got %+v", s)
	}
}

func TestFindHintsWithOptions_ZeroByteFiles(t *testing.T) {
	tmp := t.TempDir()
	a := writeFile(t, tmp, "empty1.txt", "")
	b := writeFile(t, tmp, "empty2.txt", "")

	root := &Entry{
		Path:   tmp,
		Name:   "tmp",
		IsDir:  true,
		AccessTime: time.Now(),
		Children: []*Entry{
			entryFor(a, 0),
			entryFor(b, 0),
		},
	}

	for _, mode := range []DupHashMode{DupHashSample, DupHashFull, DupHashSmart} {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			hints, err := FindHintsWithOptions(context.Background(), root, HintOptions{DupMode: mode})
			if err != nil {
				t.Fatalf("FindHintsWithOptions: %v", err)
			}
			var duplicates int
			for _, h := range hints {
				if h.Reason == ReasonDuplicate {
					duplicates++
				}
			}
			if duplicates < 2 {
				t.Fatalf("expected zero-byte files to be flagged as duplicates, got %d hints", duplicates)
			}
		})
	}
}
