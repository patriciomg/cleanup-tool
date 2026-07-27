package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/categories"
)

type Entry struct {
	Path       string
	Name       string
	Size       int64
	Usage      int64
	ModTime    time.Time
	AccessTime time.Time
	Mode       os.FileMode
	IsDir      bool
	Category   categories.Category
	Children   []*Entry
	Parent     *Entry
	NumFiles   int64
	NumDirs    int64
	Scanned    bool
	Error      error
}

func (e *Entry) TotalSize() int64 { return e.Size }
func (e *Entry) IsRoot() bool     { return e.Parent == nil }

func (e *Entry) PrettySize() string {
	return PrettySize(e.Size)
}

func PrettySize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (e *Entry) FullPath() string {
	return e.Path
}

func (e *Entry) AddChild(child *Entry) {
	child.Parent = e
	e.Children = append(e.Children, child)
}

func (e *Entry) Depth() int {
	depth := 0
	for p := e.Parent; p != nil; p = p.Parent {
		depth++
	}
	return depth
}

func (e *Entry) IsLeaf() bool { return len(e.Children) == 0 }

func NewRoot(path string) *Entry {
	abs, _ := filepath.Abs(path)
	return &Entry{
		Path:  abs,
		Name:  filepath.Base(abs),
		IsDir: true,
	}
}
