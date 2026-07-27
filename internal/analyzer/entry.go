package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/patriciomg/cleanup-tool/internal/categories"
)

type Entry struct {
	Path       string              `json:"path"`
	Name       string              `json:"name"`
	Size       int64               `json:"size"`
	Usage      int64               `json:"usage"`
	ModTime    time.Time           `json:"modTime"`
	AccessTime time.Time           `json:"accessTime"`
	Mode       os.FileMode         `json:"mode"`
	IsDir      bool                `json:"isDir"`
	Category   categories.Category `json:"category"`
	Children   []*Entry            `json:"children,omitempty"`
	Parent     *Entry              `json:"-" yaml:"-"`
	NumFiles   int64               `json:"numFiles"`
	NumDirs    int64               `json:"numDirs"`
	Scanned    bool                `json:"scanned"`
	Error      error             `json:"error,omitempty"`
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
