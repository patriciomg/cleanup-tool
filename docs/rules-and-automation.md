# cleanup-tool Saved Rules & launchd Automation Design

## Overview

This document describes how `cleanup-tool` will support **saved cleanup rules** and **scheduled execution via macOS launchd**. Saved rules let users define reusable cleanup presets that can be run on-demand or automatically at a schedule.

## Goals

- Allow users to define reusable cleanup rules via JSON and CLI commands.
- Run rules non-interactively (no TUI) for automation.
- Schedule rules using macOS `launchd` user agents.
- Keep the feature safe with dry-run mode, size caps, and confirmations.

---

## 1. Rule Schema

A rule defines *where* to scan, *what* to target, *which action* to take, and *safety limits*.

### Example Rule

```json
{
  "name": "weekly-log-cleanup",
  "paths": ["~/Library/Logs", "~/Library/Caches"],
  "ignore_paths": ["*/ImportantLogs/*"],
  "ignore_hidden": false,
  "categories": ["old", "log/cache"],
  "age_threshold_days": 30,
  "dup_mode": "none",
  "action": "trash",
  "destination": "",
  "max_deleted_bytes": 5368709120,
  "dry_run": false
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique rule identifier. Used for CLI commands and launchd job label. |
| `paths` | []string | Yes | Paths to scan. Supports `~` expansion. |
| `ignore_paths` | []string | No | Paths to ignore. Falls back to config file defaults if empty. |
| `ignore_hidden` | bool | No | Skip hidden files and directories. |
| `categories` | []string | No | Categories to act on: `old`, `log/cache`, `duplicate`. Empty means all. |
| `age_threshold_days` | int | No | Minimum age in days for `old` files. Overrides the default 365 days. |
| `dup_mode` | string | No | Duplicate detection mode: `first10mb`, `sample`, `full`, `smart`, `none`. Default: `none`. |
| `action` | string | Yes | Action to take: `trash` or `move_external`. |
| `destination` | string | No | External directory when `action` is `move_external`. |
| `max_deleted_bytes` | int | No | Maximum total size of matched files before aborting. |
| `dry_run` | bool | No | If `true`, only report what would be deleted. |

### Category Filter

- `old`: files older than `age_threshold_days`.
- `log/cache`: files in the `log/cache` category.
- `duplicate`: files flagged as duplicates.

---

## 2. Storage

Rules are stored separately from the main config:

- **Rules file:** `~/.config/cleanup-tool/rules.json`
- **Config file:** `~/.config/cleanup-tool/config.json`

`config.json` stores global defaults (e.g., `ignore_paths`, `dup_mode`).
`rules.json` stores a dictionary of named rules.

```json
{
  "version": 1,
  "rules": {
    "weekly-log-cleanup": { ... },
    "archive-old-videos": { ... }
  }
}
```

---

## 3. CLI Surface

A new `rules` subcommand is introduced:

```bash
# Create a new rule interactively or via flags
./cleanup-tool rules create --name weekly-log-cleanup \
  --paths "~/Library/Logs,~/Library/Caches" \
  --categories "log/cache" \
  --action trash

# List all rules
./cleanup-tool rules list

# Show a single rule
./cleanup-tool rules show weekly-log-cleanup

# Edit a rule in $EDITOR
./cleanup-tool rules edit weekly-log-cleanup

# Delete a rule
./cleanup-tool rules delete weekly-log-cleanup

# Run a rule now
./cleanup-tool rules run weekly-log-cleanup

# Run a rule non-interactively
./cleanup-tool rules run weekly-log-cleanup --yes

# Dry-run a rule
./cleanup-tool rules run weekly-log-cleanup --dry-run
```

---

## 4. Non-Interactive Execution

Running `./cleanup-tool rules run <name>` performs the following steps:

1. **Load** the rule from `rules.json`.
2. **Resolve paths** and expand `~`.
3. **Scan** the target paths using `analyzer.NewScanner`.
4. **Analyze** the scanned tree using `analyzer.FindHints` with the configured `dup_mode`.
5. **Filter** hints by `categories` and `age_threshold_days`.
6. **Safety check:** if total matched size exceeds `max_deleted_bytes`, abort.
7. **Prompt** for confirmation if running in a TTY, unless `--yes` is passed.
8. **Execute** the action (`trash` or `move_external`).
9. **Report** summary to stdout/stderr:
   - number of files deleted
   - total bytes freed
   - any errors

---

## 5. launchd Integration

Rules are scheduled as **macOS user agents** in `~/Library/LaunchAgents/`.

### Schedule Commands

```bash
# Install a schedule for a rule
./cleanup-tool schedule install weekly-log-cleanup --daily --at 10:00

# Remove a schedule
./cleanup-tool schedule remove weekly-log-cleanup

# List installed schedules
./cleanup-tool schedule list
```

### Generated Plist Example

`~/Library/LaunchAgents/com.cleanup-tool.weekly-log-cleanup.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.cleanup-tool.weekly-log-cleanup</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/cleanup-tool</string>
        <string>rules</string>
        <string>run</string>
        <string>weekly-log-cleanup</string>
        <string>--yes</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>10</integer>
        <key>Minute</key>
        <integer>0</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>/Users/username/.local/state/cleanup-tool/weekly-log-cleanup.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/username/.local/state/cleanup-tool/weekly-log-cleanup.err</string>
</dict>
</plist>
```

### Scheduling Options

| Option | Description |
|--------|-------------|
| `--daily --at 10:00` | Run every day at 10:00 |
| `--weekly --day Mon --at 09:00` | Run every Monday at 09:00 |
| `--interval 3600` | Run every N seconds |
| `--on-login` | Run once after user login |

### Notifications

After a scheduled run, the app will emit a macOS notification:

```osascript
display notification "Freed 1.2 GB of logs" with title "cleanup-tool"
```

---

## 6. Safety Features

- **Dry-run mode:** report what would be deleted without touching files.
- **Max deleted bytes:** abort if matched files exceed a configurable size.
- **Interactive confirmation:** prompt by default unless `--yes` is used.
- **Protected paths:** never process `/`, `/System`, `/Library` (root), or `.app` bundles.
- **User-level launchd:** run as the current user, preserving `~/.Trash` and permissions.

---

## 7. Suggested Implementation Order

1. **`internal/rules` package**
   - Define `Rule` struct and `Load`/`Save` for `rules.json`.

2. **`internal/executor` package**
   - Implement `RunRule(ctx, rule, opts)` for non-interactive rule execution.

3. **`cmd/cleanup-tool/rules.go`**
   - Add `rules` subcommand with `create`, `list`, `show`, `edit`, `delete`, `run`.

4. **`internal/launchd` package**
   - Generate plists and wrap `launchctl load` / `unload`.

5. **`cmd/cleanup-tool/schedule.go`**
   - Add `schedule` subcommand for installing/removing/listing launchd jobs.

6. **Notifications and logging**
   - Add `osascript` notification helper and log rotation for scheduled runs.

---

## 8. Open Questions

1. Should rules support per-rule `progress_interval` and `ignore_hidden`, or inherit from global config?
2. Should the CLI `rules create` be interactive (wizard) or flag-based?
3. Should scheduled runs be able to trigger macOS notifications only on success, only on failure, or always?
4. Should the tool auto-detect the binary path for launchd plists, or require an explicit `--bin` flag?
