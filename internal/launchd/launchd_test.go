package launchd

import (
	"strings"
	"testing"
)

func TestScheduleOptionsValidate(t *testing.T) {
	cases := []struct {
		name    string
		options ScheduleOptions
		wantErr bool
	}{
		{
			name:    "empty",
			options: ScheduleOptions{},
			wantErr: true,
		},
		{
			name:    "daily no time",
			options: ScheduleOptions{Daily: true},
			wantErr: true,
		},
		{
			name:    "daily with time",
			options: ScheduleOptions{Daily: true, At: "10:00"},
			wantErr: false,
		},
		{
			name:    "weekly no day",
			options: ScheduleOptions{Weekly: true, At: "09:00"},
			wantErr: true,
		},
		{
			name:    "weekly invalid day",
			options: ScheduleOptions{Weekly: true, Day: "Monday", At: "09:00"},
			wantErr: true,
		},
		{
			name:    "weekly valid day",
			options: ScheduleOptions{Weekly: true, Day: "Mon", At: "09:00"},
			wantErr: false,
		},
		{
			name:    "interval",
			options: ScheduleOptions{Interval: 3600},
			wantErr: false,
		},
		{
			name:    "on-login",
			options: ScheduleOptions{OnLogin: true},
			wantErr: false,
		},
		{
			name:    "multiple triggers",
			options: ScheduleOptions{Daily: true, At: "10:00", Interval: 3600},
			wantErr: true,
		},
		{
			name:    "invalid time format",
			options: ScheduleOptions{Daily: true, At: "25:00"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.options.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		input       string
		wantHour    int
		wantMinute  int
		wantErr     bool
	}{
		{"00:00", 0, 0, false},
		{"23:59", 23, 59, false},
		{"10:30", 10, 30, false},
		{"24:00", 0, 0, true},
		{"12:60", 0, 0, true},
		{"not-a-time", 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			hour, minute, err := parseTime(c.input)
			if (err != nil) != c.wantErr {
				t.Fatalf("parseTime(%q) error = %v, wantErr %v", c.input, err, c.wantErr)
			}
			if err != nil {
				return
			}
			if hour != c.wantHour || minute != c.wantMinute {
				t.Fatalf("parseTime(%q) = %d:%d, want %d:%d", c.input, hour, minute, c.wantHour, c.wantMinute)
			}
		})
	}
}

func TestBuildPlistDaily(t *testing.T) {
	opts := ScheduleOptions{Daily: true, At: "10:00"}
	plist, err := buildPlist("daily-rule", "/usr/local/bin/cleanup-tool", opts)
	if err != nil {
		t.Fatalf("buildPlist failed: %v", err)
	}
	if !strings.Contains(plist, "<string>com.cleanup-tool.daily-rule</string>") {
		t.Fatalf("plist missing expected label")
	}
	if !strings.Contains(plist, "<key>StartCalendarInterval</key>") {
		t.Fatalf("plist missing StartCalendarInterval")
	}
	if !strings.Contains(plist, "<integer>10</integer>") {
		t.Fatalf("plist missing expected hour")
	}
}

func TestBuildPlistWeekly(t *testing.T) {
	opts := ScheduleOptions{Weekly: true, Day: "Mon", At: "09:30"}
	plist, err := buildPlist("weekly-rule", "/usr/local/bin/cleanup-tool", opts)
	if err != nil {
		t.Fatalf("buildPlist failed: %v", err)
	}
	if !strings.Contains(plist, "<key>Weekday</key>") {
		t.Fatalf("plist missing Weekday")
	}
	if !strings.Contains(plist, "<integer>1</integer>") {
		t.Fatalf("plist missing expected weekday")
	}
}

func TestBuildPlistInterval(t *testing.T) {
	opts := ScheduleOptions{Interval: 3600}
	plist, err := buildPlist("interval-rule", "/usr/local/bin/cleanup-tool", opts)
	if err != nil {
		t.Fatalf("buildPlist failed: %v", err)
	}
	if !strings.Contains(plist, "<key>StartInterval</key>") {
		t.Fatalf("plist missing StartInterval")
	}
	if !strings.Contains(plist, "<integer>3600</integer>") {
		t.Fatalf("plist missing expected interval")
	}
}

func TestBuildPlistOnLogin(t *testing.T) {
	opts := ScheduleOptions{OnLogin: true}
	plist, err := buildPlist("login-rule", "/usr/local/bin/cleanup-tool", opts)
	if err != nil {
		t.Fatalf("buildPlist failed: %v", err)
	}
	if !strings.Contains(plist, "<key>RunAtLoad</key>") {
		t.Fatalf("plist missing RunAtLoad")
	}
}

func TestBuildPlistXMLEscapesDynamicValues(t *testing.T) {
	opts := ScheduleOptions{OnLogin: true}
	// Paths with characters that would break XML if not escaped.
	binPath := "/usr/local/bin/cleanup-tool & friends</other>"
	plist, err := buildPlist("rule&name", binPath, opts)
	if err != nil {
		t.Fatalf("buildPlist failed: %v", err)
	}
	// The static XML declaration must remain intact.
	if !strings.Contains(plist, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Fatalf("static XML declaration was mangled")
	}
	// Dynamic values should be escaped.
	if !strings.Contains(plist, "&amp;") {
		t.Fatalf("expected &amp; in plist")
	}
	if !strings.Contains(plist, "&lt;/other&gt;") {
		t.Fatalf("expected escaped tags in plist")
	}
}
