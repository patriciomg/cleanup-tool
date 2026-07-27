package analyzer

import (
	"encoding/json"
	"os"
)

// ExportJSON writes the given scan roots as indented JSON to path.
func ExportJSON(roots []*Entry, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(roots)
}
