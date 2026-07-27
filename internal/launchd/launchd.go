package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/adrg/xdg"
	"github.com/patriciomg/cleanup-tool/internal/rules"
)

// ScheduleOptions describes when a rule should run.
type ScheduleOptions struct {
	Daily    bool
	Weekly   bool
	Day      string // "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"
	At       string // "HH:MM"
	Interval int    // seconds
	OnLogin  bool
}

// Job represents an installed launchd job for a cleanup-tool rule.
type Job struct {
	RuleName string
	Plist    string
	Loaded   bool
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.cleanup-tool.{{.RuleName}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinPath}}</string>
        <string>rules</string>
        <string>run</string>
        <string>{{.RuleName}}</string>
        <string>--yes</string>
    </array>{{if .OnLogin}}
    <key>RunAtLoad</key>
    <true/>{{end}}{{if gt .Interval 0}}
    <key>StartInterval</key>
    <integer>{{.Interval}}</integer>{{end}}{{if .Calendar}}
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>{{.Hour}}</integer>
        <key>Minute</key>
        <integer>{{.Minute}}</integer>{{if .Weekly}}
        <key>Weekday</key>
        <integer>{{.Weekday}}</integer>{{end}}
    </dict>{{end}}
    <key>StandardOutPath</key>
    <string>{{.LogDir}}/{{.RuleName}}.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/{{.RuleName}}.err</string>
</dict>
</plist>
`

type plistData struct {
	RuleName string
	BinPath  string
	LogDir   string
	OnLogin  bool
	Interval int
	Calendar bool
	Weekly   bool
	Hour     int
	Minute   int
	Weekday  int
}

// AgentDir is the directory where user launchd plists are stored.
func AgentDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Library", "LaunchAgents")
}

// PlistPath returns the path to the plist for a given rule name.
func PlistPath(ruleName string) string {
	return filepath.Join(AgentDir(), fmt.Sprintf("com.cleanup-tool.%s.plist", ruleName))
}

// LogDir returns the directory for scheduled rule logs.
func LogDir() string {
	return filepath.Join(xdg.StateHome, "cleanup-tool")
}

// Validate checks that the schedule options are usable.
func (o ScheduleOptions) Validate() error {
	sum := 0
	if o.Daily {
		sum++
	}
	if o.Weekly {
		sum++
	}
	if o.Interval > 0 {
		sum++
	}
	if o.OnLogin {
		sum++
	}
	if sum == 0 {
		return fmt.Errorf("at least one schedule option is required (daily, weekly, interval, on-login)")
	}
	if sum > 1 {
		return fmt.Errorf("only one schedule option is allowed at a time")
	}

	if o.Weekly && o.Day == "" {
		return fmt.Errorf("--day is required for weekly schedules")
	}
	if o.Weekly {
		validDays := map[string]bool{"sun": true, "mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true}
		if !validDays[strings.ToLower(o.Day)] {
			return fmt.Errorf("invalid --day %q; use Mon, Tue, Wed, Thu, Fri, Sat, or Sun", o.Day)
		}
	}

	if (o.Daily || o.Weekly) && o.At == "" {
		return fmt.Errorf("--at is required for daily/weekly schedules")
	}

	if o.At != "" {
		if _, _, err := parseTime(o.At); err != nil {
			return fmt.Errorf("invalid --at time: %w", err)
		}
	}

	if o.Interval < 0 {
		return fmt.Errorf("--interval must be >= 0")
	}
	return nil
}

// Install creates and loads a launchd agent for the given rule.
func Install(ruleName string, binPath string, opts ScheduleOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	// Verify the rule exists.
	f, err := rules.Load()
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	if _, ok := f.Get(ruleName); !ok {
		return fmt.Errorf("rule %q not found", ruleName)
	}

	if err := os.MkdirAll(AgentDir(), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := os.MkdirAll(LogDir(), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	data, err := buildPlist(ruleName, binPath, opts)
	if err != nil {
		return err
	}

	plistPath := PlistPath(ruleName)
	if err := os.WriteFile(plistPath, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	// Bootstrap the job in the user's gui domain. Boot out any stale
	// registration first; bootstrap fails if the label is already loaded,
	// and bootout fails harmlessly when the job is not loaded.
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain, plistPath).Run()

	cmd := exec.Command("launchctl", "bootstrap", domain, plistPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %w\n%s", err, string(out))
	}
	return nil
}

// Remove unloads and deletes a launchd agent for the given rule.
func Remove(ruleName string) error {
	plistPath := PlistPath(ruleName)

	// bootout the job even if it isn't loaded; ignore errors.
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	cmd := exec.Command("launchctl", "bootout", domain, plistPath)
	_ = cmd.Run()

	if _, err := os.Stat(plistPath); err == nil {
		if err := os.Remove(plistPath); err != nil {
			return fmt.Errorf("remove plist: %w", err)
		}
	}
	return nil
}

// List returns all installed cleanup-tool launchd jobs.
func List() ([]Job, error) {
	pattern := filepath.Join(AgentDir(), "com.cleanup-tool.*.plist")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	for _, p := range matches {
		base := filepath.Base(p)
		prefix := "com.cleanup-tool."
		suffix := ".plist"
		if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
			continue
		}
		name := base[len(prefix) : len(base)-len(suffix)]
		jobs = append(jobs, Job{RuleName: name, Plist: p, Loaded: false})
	}
	return jobs, nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func buildPlist(ruleName, binPath string, opts ScheduleOptions) (string, error) {
	data := plistData{
		RuleName: xmlEscape(ruleName),
		BinPath:  xmlEscape(binPath),
		LogDir:   xmlEscape(LogDir()),
		OnLogin:  opts.OnLogin,
		Interval: opts.Interval,
	}

	if opts.Daily || opts.Weekly {
		hour, minute, err := parseTime(opts.At)
		if err != nil {
			return "", err
		}
		data.Calendar = true
		data.Hour = hour
		data.Minute = minute
	}
	if opts.Weekly {
		data.Weekly = true
		data.Weekday = parseWeekday(opts.Day)
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return "", fmt.Errorf("parse plist template: %w", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("execute plist template: %w", err)
	}
	return b.String(), nil
}

func parseTime(s string) (hour, minute int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return 0, 0, fmt.Errorf("invalid hour: %w", err)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minute); err != nil {
		return 0, 0, fmt.Errorf("invalid minute: %w", err)
	}
	if hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour must be between 0 and 23")
	}
	if minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute must be between 0 and 59")
	}
	return hour, minute, nil
}

func parseWeekday(s string) int {
	switch strings.ToLower(s) {
	case "sun":
		return 0
	case "mon":
		return 1
	case "tue":
		return 2
	case "wed":
		return 3
	case "thu":
		return 4
	case "fri":
		return 5
	case "sat":
		return 6
	default:
		return 1
	}
}
