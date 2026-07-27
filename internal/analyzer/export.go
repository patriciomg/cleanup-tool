package analyzer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Exporter writes scan results in a specific format.
type Exporter interface {
	Export(roots []*Entry, w io.Writer) error
}

var exporters = map[string]Exporter{
	"json": JSONExporter{},
	"csv":  CSVExporter{},
	"yaml": YAMLExporter{},
}

// RegisterExporter registers an exporter for the given format name. It can be
// used to add custom formats at startup.
func RegisterExporter(name string, ex Exporter) {
	exporters[name] = ex
}

// Export writes roots to w using the named format.
func Export(roots []*Entry, w io.Writer, format string) error {
	ex, ok := exporters[format]
	if !ok {
		return fmt.Errorf("unknown format: %q", format)
	}
	return ex.Export(roots, w)
}

// ExportJSON writes the given scan roots as indented JSON to path.
func ExportJSON(roots []*Entry, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return Export(roots, file, "json")
}

// ExportJSONWriter writes the given scan roots as indented JSON to w.
func ExportJSONWriter(roots []*Entry, w io.Writer) error {
	return Export(roots, w, "json")
}

// JSONExporter writes the Entry tree as indented JSON.
type JSONExporter struct{}

// Export implements Exporter for JSON.
func (JSONExporter) Export(roots []*Entry, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(roots)
}

// YAMLExporter writes the Entry tree as YAML.
type YAMLExporter struct{}

// Export implements Exporter for YAML.
func (YAMLExporter) Export(roots []*Entry, w io.Writer) error {
	encoder := yaml.NewEncoder(w)
	defer encoder.Close()
	return encoder.Encode(roots)
}

// CSVExporter flattens the Entry tree into CSV rows.
type CSVExporter struct{}

// Export implements Exporter for CSV.
func (CSVExporter) Export(roots []*Entry, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	header := []string{"Path", "Name", "Size", "Usage", "ModTime", "AccessTime", "Mode", "IsDir", "Category", "NumFiles", "NumDirs", "Scanned"}
	if err := writer.Write(header); err != nil {
		return err
	}

	var walk func(entries []*Entry) error
	walk = func(entries []*Entry) error {
		for _, e := range entries {
			row := []string{
				e.Path,
				e.Name,
				strconv.FormatInt(e.Size, 10),
				strconv.FormatInt(e.Usage, 10),
				e.ModTime.Format("2006-01-02T15:04:05"),
				e.AccessTime.Format("2006-01-02T15:04:05"),
				e.Mode.String(),
				strconv.FormatBool(e.IsDir),
				string(e.Category),
				strconv.FormatInt(e.NumFiles, 10),
				strconv.FormatInt(e.NumDirs, 10),
				strconv.FormatBool(e.Scanned),
			}
			if err := writer.Write(row); err != nil {
				return err
			}
			if err := walk(e.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(roots)
}
