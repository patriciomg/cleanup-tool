package analyzer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Exporter writes scan results in a specific format.
type Exporter interface {
	Export(roots []*Entry, w io.Writer) error
}

var exporters = map[string]Exporter{
	"json": JSONExporter{},
	"csv":  &CSVExporter{},
	"tsv":  &TSVExporter{},
	"yaml": YAMLExporter{},
	"md":   MarkdownExporter{},
	"html": HTMLExporter{},
	"txt":  TextExporter{},
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

// MarkdownExporter writes the Entry tree as a Markdown table.
type MarkdownExporter struct{}

// Export implements Exporter for Markdown.
func (MarkdownExporter) Export(roots []*Entry, w io.Writer) error {
	return writeMarkdown(roots, w)
}

// HTMLExporter writes the Entry tree as an HTML report.
type HTMLExporter struct{}

// Export implements Exporter for HTML.
func (HTMLExporter) Export(roots []*Entry, w io.Writer) error {
	return writeHTML(roots, w)
}

// TextExporter writes the Entry tree as a plain text list.
type TextExporter struct{}

// Export implements Exporter for plain text.
func (TextExporter) Export(roots []*Entry, w io.Writer) error {
	return writeText(roots, w)
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
	return writeDelimited(roots, c.Columns, csv.NewWriter(w))
}

// TSVExporter flattens the Entry tree into tab-separated rows.
type TSVExporter struct {
	// Columns is the ordered list of column names to export. If empty,
	// DefaultCSVColumns is used.
	Columns []string
}

// NewTSVExporter creates a TSV exporter using the given columns. If columns is
// empty, the default columns are used.
func NewTSVExporter(columns []string) *TSVExporter {
	return &TSVExporter{Columns: columns}
}

// Export implements Exporter for TSV.
func (t *TSVExporter) Export(roots []*Entry, w io.Writer) error {
	tabWriter := csv.NewWriter(w)
	tabWriter.Comma = '\t'
	return writeDelimited(roots, t.Columns, tabWriter)
}

// writeDelimited flattens the Entry tree into delimited rows using writer.
func writeDelimited(roots []*Entry, columns []string, writer *csv.Writer) error {
	if len(columns) == 0 {
		columns = DefaultCSVColumns
	}
	if err := validateCSVColumns(columns); err != nil {
		return err
	}

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

// writeMarkdown flattens the Entry tree into a Markdown table.
func writeMarkdown(roots []*Entry, w io.Writer) error {
	columns := DefaultCSVColumns
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(columns, " | ")); err != nil {
		return err
	}
	separator := make([]string, len(columns))
	for i := range columns {
		separator[i] = "---"
	}
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(separator, " | ")); err != nil {
		return err
	}

	var walk func(entries []*Entry) error
	walk = func(entries []*Entry) error {
		for _, e := range entries {
			row := make([]string, len(columns))
			for i, col := range columns {
				row[i] = escapeMarkdown(csvColumnValue(e, col))
			}
			if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(row, " | ")); err != nil {
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

func escapeMarkdown(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// writeHTML renders the Entry tree as a simple HTML report.
func writeHTML(roots []*Entry, w io.Writer) error {
	const tmpl = `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>cleanup-tool scan</title>
	<style>
		body { font-family: sans-serif; margin: 2rem; }
		table { border-collapse: collapse; width: 100%; }
		th, td { border: 1px solid #ccc; padding: 0.5rem; text-align: left; }
		th { background: #f5f5f5; }
		tr:nth-child(even) { background: #fafafa; }
	</style>
</head>
<body>
	<h1>cleanup-tool scan</h1>
	<table>
		<thead>
			<tr>
				{{ range .Columns }}<th>{{ . }}</th>{{ end }}
			</tr>
		</thead>
		<tbody>
			{{ range .Rows }}
			<tr>
				{{ range . }}<td>{{ . }}</td>{{ end }}
			</tr>
			{{ end }}
		</tbody>
	</table>
</body>
</html>
`
	type rowData struct {
		Columns []string
		Rows    [][]string
	}

	columns := DefaultCSVColumns
	d := rowData{Columns: columns}
	var walk func(entries []*Entry) error
	walk = func(entries []*Entry) error {
		for _, e := range entries {
			r := make([]string, len(columns))
			for i, col := range columns {
				r[i] = csvColumnValue(e, col)
			}
			d.Rows = append(d.Rows, r)
			if err := walk(e.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(roots); err != nil {
		return err
	}

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return err
	}
	return t.Execute(w, d)
}

// writeText renders the Entry tree as a tab-separated plain text table.
func writeText(roots []*Entry, w io.Writer) error {
	columns := DefaultCSVColumns
	if _, err := fmt.Fprintf(w, "%s\n", strings.Join(columns, "\t")); err != nil {
		return err
	}
	var walk func(entries []*Entry) error
	walk = func(entries []*Entry) error {
		for _, e := range entries {
			row := make([]string, len(columns))
			for i, col := range columns {
				row[i] = csvColumnValue(e, col)
			}
			if _, err := fmt.Fprintf(w, "%s\n", strings.Join(row, "\t")); err != nil {
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
