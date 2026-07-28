package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/patriciomg/cleanup-tool/internal/launchd"
)

// handleScheduleCmd dispatches the "schedule" subcommand.
func handleScheduleCmd(args []string) {
	if len(args) == 0 {
		printScheduleUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "install":
		scheduleInstall(args[1:])
	case "remove":
		scheduleRemove(args[1:])
	case "list":
		scheduleList(args[1:])
	default:
		printScheduleUsage()
		os.Exit(1)
	}
}

func printScheduleUsage() {
	fmt.Println("Usage: cleanup-tool schedule <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  install  Install a launchd agent for a rule")
	fmt.Println("  remove   Remove the launchd agent for a rule")
	fmt.Println("  list     List installed launchd agents")
	fmt.Println()
	fmt.Println("Schedule options for install:")
	fmt.Println("  --daily --at HH:MM")
	fmt.Println("  --weekly --day Mon --at HH:MM")
	fmt.Println("  --interval SECONDS")
	fmt.Println("  --on-login")
}

func scheduleInstall(args []string) {
	var opts launchd.ScheduleOptions
	flagSet := newFlagSet("schedule install")
	flagSet.BoolVar(&opts.Daily, "daily", false, "Run every day")
	flagSet.BoolVar(&opts.Weekly, "weekly", false, "Run every week")
	flagSet.StringVar(&opts.Day, "day", "", "Day of week for weekly schedule (Mon, Tue, ...)")
	flagSet.StringVar(&opts.At, "at", "", "Time to run in HH:MM")
	flagSet.IntVar(&opts.Interval, "interval", 0, "Run every N seconds")
	flagSet.BoolVar(&opts.OnLogin, "on-login", false, "Run once after login")

	positionals, err := parseInterspersed(flagSet, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "schedule install: %v\n", err)
		os.Exit(1)
	}

	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "schedule install: rule name required")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintf(os.Stderr, "schedule install: unexpected arguments: %v\n", positionals[1:])
		os.Exit(1)
	}
	name := positionals[0]

	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "schedule install: %v\n", err)
		os.Exit(1)
	}
	// Use the symlink-free path if possible.
	binPath, err = filepath.Abs(binPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "schedule install: %v\n", err)
		os.Exit(1)
	}

	if err := launchd.Install(name, binPath, opts); err != nil {
		fmt.Fprintf(os.Stderr, "schedule install: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Installed schedule for rule %q\n", name)
}

func scheduleRemove(args []string) {
	flagSet := newFlagSet("schedule remove")
	positionals, err := parseInterspersed(flagSet, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "schedule remove: %v\n", err)
		os.Exit(1)
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "schedule remove: rule name required")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintf(os.Stderr, "schedule remove: unexpected arguments: %v\n", positionals[1:])
		os.Exit(1)
	}
	name := positionals[0]

	if err := launchd.Remove(name); err != nil {
		fmt.Fprintf(os.Stderr, "schedule remove: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed schedule for rule %q\n", name)
}

func scheduleList(args []string) {
	flagSet := newFlagSet("schedule list")
	if _, err := parseInterspersed(flagSet, args); err != nil {
		fmt.Fprintf(os.Stderr, "schedule list: %v\n", err)
		os.Exit(1)
	}

	jobs, err := launchd.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "schedule list: %v\n", err)
		os.Exit(1)
	}

	if len(jobs) == 0 {
		fmt.Println("No schedules installed.")
		return
	}

	fmt.Printf("%-20s %s\n", "RULE", "PLIST")
	for _, j := range jobs {
		fmt.Printf("%-20s %s\n", j.RuleName, j.Plist)
	}
}

// newFlagSet creates a flag set that prints usage to stderr on error.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}


