package analyzer

import (
	"encoding/json"
	"io"
	"os"
)

// ExportJSON writes the given scan roots as indented JSON to path.
func ExportJSON(roots []*Entry, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return ExportJSONWriter(roots, file)
}

// ExportJSONWriter writes the given scan roots as indented JSON to w.
func ExportJSONWriter(roots []*Entry, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(roots)
}
