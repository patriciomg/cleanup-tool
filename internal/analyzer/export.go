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
	"csv":  &CSVExporter{},
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

// DefaultCSVColumns is the default set of columns written by CSVExporter.
var DefaultCSVColumns = []string{"Path", "Name", "Size", "Usage", "ModTime", "AccessTime", "Mode", "IsDir", "Category", "NumFiles", "NumDirs", "Scanned"}

// CSVColumns returns the list of supported CSV column names.
func CSVColumns() []string {
	return []string{"Path", "Name", "Size", "Usage", "ModTime", "AccessTime", "Mode", "IsDir", "Category", "NumFiles", "NumDirs", "Scanned"}
}

// CSVExporter flattens the Entry tree into CSV rows.
type CSVExporter struct {
	// Columns is the ordered list of column names to export. If empty,
	// DefaultCSVColumns is used.
	Columns []string
}

// NewCSVExporter creates a CSV exporter using the given columns. If columns is
// empty, the default columns are used.
func NewCSVExporter(columns []string) *CSVExporter {
	return &CSVExporter{Columns: columns}
}

// Export implements Exporter for CSV.
func (c *CSVExporter) Export(roots []*Entry, w io.Writer) error {
	columns := c.Columns
	if len(columns) == 0 {
		columns = DefaultCSVColumns
	}
	if err := validateCSVColumns(columns); err != nil {
		return err
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write(columns); err != nil {
		return err
	}

	var walk func(entries []*Entry) error
	walk = func(entries []*Entry) error {
		for _, e := range entries {
			row := make([]string, len(columns))
			for i, col := range columns {
				row[i] = csvColumnValue(e, col)
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

func validateCSVColumns(columns []string) error {
	allowed := map[string]struct{}{}
	for _, c := range CSVColumns() {
		allowed[c] = struct{}{}
	}
	for _, c := range columns {
		if _, ok := allowed[c]; !ok {
			return fmt.Errorf("unknown CSV column %q", c)
		}
	}
	return nil
}

func csvColumnValue(e *Entry, col string) string {
	switch col {
	case "Path":
		return e.Path
	case "Name":
		return e.Name
	case "Size":
		return strconv.FormatInt(e.Size, 10)
	case "Usage":
		return strconv.FormatInt(e.Usage, 10)
	case "ModTime":
		return e.ModTime.Format("2006-01-02T15:04:05")
	case "AccessTime":
		return e.AccessTime.Format("2006-01-02T15:04:05")
	case "Mode":
		return e.Mode.String()
	case "IsDir":
		return strconv.FormatBool(e.IsDir)
	case "Category":
		return string(e.Category)
	case "NumFiles":
		return strconv.FormatInt(e.NumFiles, 10)
	case "NumDirs":
		return strconv.FormatInt(e.NumDirs, 10)
	case "Scanned":
		return strconv.FormatBool(e.Scanned)
	default:
		return ""
	}
}
