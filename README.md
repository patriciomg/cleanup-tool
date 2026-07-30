# cleanup-tool

[![CI](https://github.com/patriciomg/cleanup-tool/actions/workflows/ci.yml/badge.svg)](https://github.com/patriciomg/cleanup-tool/actions/workflows/ci.yml)
[![Benchmark format](https://img.shields.io/github/actions/workflow/status/patriciomg/cleanup-tool/ci.yml?job=test-benchmark-format&label=benchmark%20format&logo=github)](https://github.com/patriciomg/cleanup-tool/actions/workflows/ci.yml)
[![Export format](https://img.shields.io/github/actions/workflow/status/patriciomg/cleanup-tool/ci.yml?job=test-export-format&label=export%20format&logo=github)](https://github.com/patriciomg/cleanup-tool/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/patriciomg/cleanup-tool)](https://github.com/patriciomg/cleanup-tool/blob/main/go.mod)
[![GitHub release](https://img.shields.io/github/v/release/patriciomg/cleanup-tool?logo=github)](https://github.com/patriciomg/cleanup-tool/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Homebrew Tap](https://img.shields.io/badge/homebrew-patriciomg%2Fcleanup--tool-2e4053?logo=homebrew)](https://github.com/patriciomg/homebrew-cleanup-tool)

A fast, terminal-based disk cleanup tool tailored for macOS developers who work with Docker, LLMs, and build artifacts. It opens into a flat, size-sorted **dua-style** interactive file browser by default; switch to the classic terminal-style tree view with `-tui-style terminal`.

> **Default view:** `cleanup-tool` opens in the **dua-style** interactive browser. Use `-tui-style terminal` to switch to the classic tree-style browser.
>
> `./cleanup-tool -help` shows `-tui-style` default: `"dua"`.

## Platform support

`cleanup-tool` is developed and tested on **macOS**. The CI pipeline builds a macOS universal binary. Linux builds may work but are not officially supported; Windows is not supported.

## Table of Contents

- [Features](#features)
- [Install](#install)
- [Development](#development)
- [Usage](#usage)
- [CLI flags](#cli-flags)
- [Dependency directories](#dependency-directories)
- [Exporting results](#exporting-results)
- [Saved rules](#saved-rules)
- [Scheduling rules with launchd](#scheduling-rules-with-launchd)
- [Docker disk usage](#docker-disk-usage)
- [Benchmark mode](#benchmark-mode)
- [Performance](#performance)
- [Configuration file](#configuration-file)
- [Key bindings](#key-bindings)
- [Demo](#demo)
- [Recording a demo](#recording-a-demo)
- [Tips & Tricks](#tips--tricks)
- [Troubleshooting](#troubleshooting)
- [Trash and undo](#trash-and-undo)
- [Releasing](#releasing)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Parallel directory scanner** with bounded concurrency, live progress, and throughput stats
- **Categorisation** of common space hogs: Docker, LLM models, build artifacts, dependencies, media, archives, logs, and caches
- **Interactive TUI** (powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)) with a dua-style file browser by default and a terminal-style tree browser as an alternative
- **Safe actions**: move to Trash, move to an external drive via `rsync`, and restore
- **Batch operations**: mark and act on items across multiple directories
- **Configurable ignore paths** via `~/.config/cleanup-tool/config.json`
- **Duplicate file detection** with configurable hashing modes (sample, first 10 MB, full, smart)
- **Deletability analyzer** that flags old files, log/cache files, and duplicates, with per-category filtering, stacked bar summary, and live progress
- **Docker disk usage** analysis and prune wrapper for images, containers, volumes, and build cache
- **Mouse and keyboard support** in the analyzer summary for filtering by category
- **Benchmark mode** to measure scan performance (`-benchmark`)
- **Saved rules** for reusable cleanup presets with non-interactive execution (`rules` subcommand)
- **LLM registry cleanup**: inspect and delete models from Ollama, Hugging Face cache, and LM Studio via the TUI (`M` key) or CLI (`models list` / `models delete`)
- **CI/CD and release tooling**: smoke-tested universal macOS binaries, tarball packaging, SHA-256 checksums, and GPG-signed release artifacts via GitHub Actions

## Install

### Install with Homebrew (recommended)

`cleanup-tool` is available through the [`patriciomg/cleanup-tool`](https://github.com/patriciomg/homebrew-cleanup-tool) tap. On Homebrew 4.1+ you may be asked to trust the tap or formula before installing from a third-party repository.

```bash
brew install patriciomg/cleanup-tool/cleanup-tool
```

If Homebrew asks you to trust the tap first, run:

```bash
brew trust patriciomg/cleanup-tool
brew install patriciomg/cleanup-tool/cleanup-tool
```

Upgrade later with:

```bash
brew update && brew upgrade cleanup-tool
```

### Download a release

Grab the latest macOS universal tarball from the [Releases page](https://github.com/patriciomg/cleanup-tool/releases/latest), then extract and install the binary:

```bash
# Example: download and extract the latest release tarball
curl -L -o cleanup-tool.tar.gz "https://github.com/patriciomg/cleanup-tool/releases/latest/download/cleanup-tool-darwin-universal.tar.gz"
tar -xzf cleanup-tool.tar.gz
mv cleanup-tool /usr/local/bin/
```

### Install with Go

```bash
go install github.com/patriciomg/cleanup-tool/cmd/cleanup-tool@latest
```

### Build from source

```bash
git clone https://github.com/patriciomg/cleanup-tool.git
cd cleanup-tool
go build ./cmd/cleanup-tool
```

## Development

Auto-rebuild the binary whenever a Go file changes with `reflex`:

```bash
# Install reflex once
go install github.com/cespare/reflex@latest

# Make sure ~/go/bin is on PATH (add to your shell profile)
export PATH="$PATH:$(go env GOPATH)/bin"

# Auto-rebuild on every Go file change
make watch

# Or auto-run tests on every Go file change (slower; best for focused work)
make watch-test
```

`reflex` only rebuilds; run the resulting `./cleanup-tool` binary manually in another terminal so the interactive TUI is not interrupted by file watchers.

## Usage

```bash
# Scan home directory (TUI starts immediately and shows scan progress)
./cleanup-tool

# Scan specific paths
./cleanup-tool -paths ~/Downloads,~/Documents

# Ignore hidden files
./cleanup-tool -ignore-hidden

# Set external drive for move action
./cleanup-tool -external /Volumes/External/cleanup-tool-backups

# Use a faster (sample) or more accurate (full) duplicate-detection mode
./cleanup-tool -dup-mode sample

# Change the progress-report interval for scans and analyzer (default: 100 items)
./cleanup-tool -progress-interval 50

# Benchmark scan performance (non-interactive; prints throughput stats)
./cleanup-tool -benchmark -paths /tmp

# Create and run a saved rule
./cleanup-tool rules create --name logs --paths ~/Library/Logs --action trash --categories log/cache
./cleanup-tool rules run logs --dry-run
./cleanup-tool rules run logs --yes
```

## CLI flags

| Flag | Description | Default |
|------|-------------|---------|
| `-paths` | Comma-separated paths to scan (e.g. `~/Downloads,~/Documents`) | `~` |
| `-ignore-hidden` | Skip hidden files and directories | `false` |
| `-external` | Directory on an external drive to use for the move action | `""` |
| `-dup-mode` | Duplicate-detection hash mode: `first10mb`, `sample`, `full`, `smart` | `smart` |
| `-progress-interval` | Report scan/analyzer progress every N items | `100` |
| `-benchmark` | Run a non-interactive scan and print throughput stats | `false` |
| `-version` | Print version and exit | `false` |
| `-out` | Export scan results to the specified file (works with any `-format`) | `""` |
| `-json` | Alias for `-stdout` (non-interactive) | `false` |
| `-stdout` | Export scan results to stdout (works with any `-format`; non-interactive) | `false` |
| `-format` | Export format: `json`, `csv`, `tsv`, `yaml`. Defaults to `json`; auto-detected from `-out` extension when omitted. | `json` |
| `-csv-columns` | Comma-separated CSV/TSV column names | `""` |
| `-tui-style` | Interactive TUI style: `terminal` or `dua` | `dua` |### `deps` subcommand flags

```bash
./cleanup-tool deps [flags]
```

| Flag | Description | Default |
|------|-------------|---------|
| `-paths` | Comma-separated paths to scan | `~` |
| `-targets` | Comma-separated dependency directory names to find (uses `deps_targets` from config, then built-in defaults) | `config.DepsTargets`, then built-in defaults |
| `-sort` | Sort results by `size`, `access`, `mod`, or `path` | `size` |
| `-ignore-hidden` | Skip hidden files and directories | `false` |
| `-json` | Output results as JSON instead of a table | `false` |

## Dependency directories

The `deps` subcommand finds dependency directories — such as `node_modules`, `vendor`, `.venv`, `Pods`, and others — under one or more paths. It reports each directory's type, size, last access time, and last modified time, sorted by size by default. `deps` is a read-only discovery command: it only lists directories and never deletes anything.

### Sample output

```
PATH                                              TYPE         SIZE    LAST ACCESS      LAST MODIFIED
/Users/dev/projects/app/node_modules              node_modules 1.2 GB  2024-06-10 14:32  2024-06-01 09:15
/Users/dev/projects/api/vendor                    vendor       450 MB  2024-08-22 11:05  2024-08-20 16:40
/Users/dev/projects/ios/Pods                      Pods         120 MB  2024-11-12 08:20  2024-11-11 08:20

Found 3 dependency directories, total size 1.66 GB
```

### Examples

```bash
# Find dependency directories under your projects folder
./cleanup-tool deps -paths ~/projects

# Output as JSON for scripting or piping into another tool
./cleanup-tool deps -paths ~/projects -json

# Limit the search to specific directory names
./cleanup-tool deps -paths ~/projects -targets node_modules,vendor

# Sort by most recently accessed
./cleanup-tool deps -paths ~/projects -sort access
```

By default, `deps` searches the built-in list of dependency directory names. You can customize this list in `~/.config/cleanup-tool/config.json` with the `deps_targets` field, or override it per run with `-targets`. See [Configuration file](#configuration-file) for details.

### Common use cases

- **Find the largest dependency directories** across multiple projects before deciding where to free space.
- **Identify old vendored dependencies** by sorting on `-sort access` to see which directories have not been used recently.
- **Scope a cleanup to a specific ecosystem** by passing `-targets node_modules` or `-targets vendor`.
- **Export JSON results** for automated cleanup scripts or CI reporting.

## Exporting results

You can export a non-interactive scan to several formats. The format is controlled by `-format` and can be auto-detected from the `-out` file extension.

The examples below were produced by scanning a small sample directory with a few files. Your output will vary depending on the scanned paths.

### JSON

```bash
./cleanup-tool -format json -stdout -paths /tmp/cleanup-export-sample
```

> The example below is truncated for brevity; the real output includes every field and every nested child.

```json
[
  {
    "path": "/tmp/cleanup-export-sample",
    "name": "cleanup-export-sample",
    "size": 5132,
    "usage": 0,
    "modTime": "2026-07-30T12:27:02.599453107+02:00",
    "accessTime": "2026-07-30T12:27:02.594869761+02:00",
    "mode": 2147484141,
    "isDir": true,
    "category": "unknown",
    "children": [
      {
        "path": "/tmp/cleanup-export-sample/archive.tar",
        "name": "archive.tar",
        "size": 2048,
        "usage": 0,
        "modTime": "2026-07-30T12:27:02.599513607+02:00",
        "accessTime": "2026-07-30T12:27:02.599446399+02:00",
        "mode": 420,
        "isDir": false,
        "category": "archive",
        "numFiles": 0,
        "numDirs": 0,
        "scanned": true
      }
    ]
  }
]
```

> The JSON output contains every field of every entry, including nested `children`. Use a tool like `jq` to filter it.

### CSV

```bash
./cleanup-tool -format csv -stdout -paths /tmp/cleanup-export-sample
```

```
Path,Name,Size,Usage,ModTime,AccessTime,Mode,IsDir,Category,NumFiles,NumDirs,Scanned
/tmp/cleanup-export-sample,cleanup-export-sample,5132,0,2026-07-30T12:27:02,2026-07-30T12:27:02,drwxr-xr-x,true,unknown,3,1,true
/tmp/cleanup-export-sample/archive.tar,archive.tar,2048,0,2026-07-30T12:27:02,2026-07-30T12:27:02,-rw-r--r--,false,archive,0,0,true
/tmp/cleanup-export-sample/readme.txt,readme.txt,12,0,2026-07-30T12:27:02,2026-07-30T12:27:02,-rw-r--r--,false,unknown,0,0,true
/tmp/cleanup-export-sample/subdir,subdir,3072,0,2026-07-30T12:27:02,2026-07-30T12:27:02,drwxr-xr-x,true,unknown,1,0,true
/tmp/cleanup-export-sample/subdir/data.bin,data.bin,3072,0,2026-07-30T12:27:02,2026-07-30T12:27:02,-rw-r--r--,false,llm-model,0,0,true
```

### TSV

```bash
./cleanup-tool -format tsv -stdout -paths /tmp/cleanup-export-sample
```

```
Path	Name	Size	Usage	ModTime	AccessTime	Mode	IsDir	Category	NumFiles	NumDirs	Scanned
/tmp/cleanup-export-sample	cleanup-export-sample	5132	0	2026-07-30T12:27:02	2026-07-30T12:27:02	drwxr-xr-x	true	unknown	3	1	true
/tmp/cleanup-export-sample/archive.tar	archive.tar	2048	0	2026-07-30T12:27:02	2026-07-30T12:27:02	-rw-r--r--	false	archive	0	0	true
/tmp/cleanup-export-sample/readme.txt	readme.txt	12	0	2026-07-30T12:27:02	2026-07-30T12:27:02	-rw-r--r--	false	unknown	0	0	true
/tmp/cleanup-export-sample/subdir	subdir	3072	0	2026-07-30T12:27:02	2026-07-30T12:27:02	drwxr-xr-x	true	unknown	1	0	true
/tmp/cleanup-export-sample/subdir/data.bin	data.bin	3072	0	2026-07-30T12:27:02	2026-07-30T12:27:02	-rw-r--r--	false	llm-model	0	0	true
```

> Columns are separated by tab characters in the real output. The snippet above renders tabs as whitespace; pipe the output into `cat -A` to see `^I` between columns.

### YAML

```bash
./cleanup-tool -format yaml -stdout -paths /tmp/cleanup-export-sample
```

> The example below is truncated for brevity; the real output includes every field and every nested child.

```yaml
- path: /tmp/cleanup-export-sample
  name: cleanup-export-sample
  size: 5132
  usage: 0
  modtime: 2026-07-30T12:27:02.599453107+02:00
  accesstime: 2026-07-30T12:27:02.945488246+02:00
  mode: 2147484141
  isdir: true
  category: unknown
  children:
    - path: /tmp/cleanup-export-sample/archive.tar
      name: archive.tar
      size: 2048
      usage: 0
      modtime: 2026-07-30T12:27:02.599513607+02:00
      accesstime: 2026-07-30T12:27:02.599446399+02:00
      mode: 420
      isdir: false
      category: archive
      children: []
      numfiles: 0
      numdirs: 0
      scanned: true
      error: null
```

> Note: YAML keys appear in lower case (for example, `ModTime` is rendered as `modtime` and `NumFiles` as `numfiles`).

### Selecting columns

Both CSV and TSV support a subset of columns via `-csv-columns`:

```bash
./cleanup-tool -format csv -csv-columns "Name,Size,Category" -stdout -paths /tmp/cleanup-export-sample
```

```
Name,Size,Category
cleanup-export-sample,5132,unknown
archive.tar,2048,archive
readme.txt,12,unknown
subdir,3072,unknown
data.bin,3072,llm-model
```

Available columns: `Path`, `Name`, `Size`, `Usage`, `ModTime`, `AccessTime`, `Mode`, `IsDir`, `Category`, `NumFiles`, `NumDirs`, `Scanned`.

### Writing to a file

All of the above also work with `-out`. The format is auto-detected from the extension:

```bash
./cleanup-tool -out /tmp/scan.json -paths /tmp/cleanup-export-sample
./cleanup-tool -out /tmp/scan.csv -paths /tmp/cleanup-export-sample
./cleanup-tool -out /tmp/scan.tsv -paths /tmp/cleanup-export-sample
./cleanup-tool -out /tmp/scan.yaml -paths /tmp/cleanup-export-sample
```

### Unsupported extensions

If `-out` has an extension that is not `.json`, `.csv`, `.tsv`, `.yaml`, or `.yml`, you must specify `-format` explicitly:

```bash
./cleanup-tool -format json -out /tmp/scan.dat -paths /tmp
```

## Saved rules

Saved rules are reusable cleanup presets stored in `~/.config/cleanup-tool/rules.json`. They let you define a cleanup once and run it repeatedly — from the CLI, in scripts, or on a schedule via `launchd`. Each rule specifies where to scan, which categories to target, and whether to trash or move the matched items.

### Examples

```bash
# Create a rule that removes files older than 30 days from ~/Library/Logs
./cleanup-tool rules create --name weekly-logs \
  --paths ~/Library/Logs \
  --categories old \
  --action trash \
  --age-threshold-days 30

# Move old downloads to an external archive before deleting
./cleanup-tool rules create --name archive-downloads \
  --paths ~/Downloads \
  --categories old \
  --action move_external \
  --destination /Volumes/External/archive \
  --age-threshold-days 365

# List all saved rules
./cleanup-tool rules list

# Show a rule as JSON
./cleanup-tool rules show weekly-logs

# Edit a rule in $EDITOR
./cleanup-tool rules edit weekly-logs

# Dry-run before running for real
./cleanup-tool rules run weekly-logs --dry-run

# Run the rule non-interactively
./cleanup-tool rules run weekly-logs --yes

# Delete a rule
./cleanup-tool rules delete weekly-logs
```

### Rule fields

| Field | Description |
|-------|-------------|
| `name` | Unique identifier (required) |
| `paths` | Comma-separated paths to scan (required) |
| `ignore_paths` | Additional paths to ignore |
| `ignore_hidden` | Skip hidden files/directories |
| `categories` | Comma-separated: `old`, `log/cache`, `duplicate` |
| `age_threshold_days` | Minimum age for `old` files (default 365) |
| `dup_mode` | `none`, `first10mb`, `sample`, `full`, `smart` |
| `action` | `trash` or `move_external` |
| `destination` | External directory for `move_external` |
| `max_deleted_bytes` | Abort if matched size exceeds this |
| `dry_run` | Only report what would be deleted |

### Example: clean macOS caches

```bash
./cleanup-tool rules create --name cache-cleanup \
  --paths "~/Library/Caches" \
  --categories "log/cache" \
  --action trash \
  --max-deleted-bytes 2147483648
```

### Common use cases

- **Automated weekly cleanup** — create a rule for `~/Library/Logs` and `~/Library/Caches` and schedule it with `launchd`.
- **Safe dry-run in CI** — run a rule with `--dry-run` in CI to see what would be deleted without touching files.
- **Move before deleting** — set `--action move_external` and `--destination` to an external drive to archive files before eventual deletion.
- **Target duplicates** — use `--categories duplicate` and `--dup-mode smart` to reclaim space from duplicate files in `~/Downloads` or `~/Pictures`.
- **Age-based cleanup** — combine `--categories old` and `--age-threshold-days` to remove files that have not been accessed in a year.

See [Scheduling rules with launchd](#scheduling-rules-with-launchd) to automate any rule on a timer.

## Scheduling rules with launchd

The `schedule` subcommand turns any saved rule into a macOS `launchd` user agent in `~/Library/LaunchAgents`. This lets you run cleanups automatically — daily, weekly, on a fixed interval, or every time you log in — without keeping the app open. The rule must already exist before you can schedule it; see [Saved rules](#saved-rules).

### Examples

```bash
# Run a rule every day at 10:00
./cleanup-tool schedule install cache-cleanup --daily --at 10:00

# Run a rule every Monday at 09:00
./cleanup-tool schedule install cache-cleanup --weekly --day Mon --at 09:00

# Run a rule every hour
./cleanup-tool schedule install cache-cleanup --interval 3600

# Run a rule once after each login
./cleanup-tool schedule install cache-cleanup --on-login

# List installed schedules
./cleanup-tool schedule list

# Remove a schedule
./cleanup-tool schedule remove cache-cleanup
```

Scheduled runs execute the rule with `--yes`, so they skip the confirmation prompt. Output is written to `~/.local/state/cleanup-tool/<rule>.log` and errors to `<rule>.err`.

### Schedule options

| Option | Description | Example |
|--------|-------------|---------|
| `--daily --at HH:MM` | Run every day at the given time | `--daily --at 10:00` |
| `--weekly --day D --at HH:MM` | Run on a given weekday | `--weekly --day Mon --at 09:00` |
| `--interval SECONDS` | Run every N seconds | `--interval 3600` |
| `--on-login` | Run once after user login | `--on-login` |

### Sample generated plist

When you run `schedule install`, `cleanup-tool` writes a plist like this:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.cleanup-tool.cache-cleanup</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/cleanup-tool</string>
        <string>rules</string>
        <string>run</string>
        <string>cache-cleanup</string>
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
    <string>/Users/username/.local/state/cleanup-tool/cache-cleanup.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/username/.local/state/cleanup-tool/cache-cleanup.err</string>
</dict>
</plist>
```

> **Note:** schedule options (`--daily`, `--weekly`, `--interval`, `--on-login`) are mutually exclusive. The plist stores the absolute path to the `cleanup-tool` binary, so moving or deleting the binary after scheduling will break the schedule.

### Common use cases

- **Nightly log cleanup** — run a `log/cache` rule every night after work.
- **Weekly deep clean** — remove old files and duplicates every Sunday morning.
- **Login cleanup** — free up caches automatically every time you log in.
- **CI-style periodic cleanup** — use `--interval` on a shared Mac to keep build directories tidy during long-running tests.
- **Safe automation** — always dry-run a rule with `rules run <name> --dry-run` before scheduling it.

## Docker disk usage

Press `D` in either the dua-style or terminal-style file browser to open the **Docker disk usage** view. It shows a four-category summary of images, containers, volumes, and build cache, and lets you prune entire categories or inspect and delete individual items.

### Summary view

When you first press `D`, the summary looks like this:

```
Docker Disk Usage

Images       size: 24.7 GB  reclaimable: 18.2 GB  count: 12
Containers   size: 0 B      reclaimable: 0 B       count: 0
Volumes      size: 8.1 GB   reclaimable: 3.4 GB   count: 4
Build Cache  size: 1.2 GB   reclaimable: 1.1 GB   count: 23

Total used: 34.0 GB
```

From the summary you can:

- **Prune an entire category** (for example, all dangling images or all stopped containers).
- **Drill into a category** to browse individual images, containers, or volumes.

### Per-item view

Press `Enter` on a category to open the **per-item list**. Each row shows the item name, size, status, project label, and ID.

Status colors help you decide what is safe to remove:

- **Green / in-use** — the resource is referenced by a running container. Keep it unless you know it is stale.
- **Yellow / unused** — the resource exists but is not referenced by anything. Usually safe to delete, but double-check the name before confirming.
- **Red / dangling** — the resource is untagged, stopped, or unmounted. These are the safest to remove.

The per-item list also supports:

- **Filtering** with `f` to cycle through `all`, `dangling`, and `unused`.
- **Grouping** with `g` to sort items by Docker Compose project label.
- **Labels** with `i` to show the full label dump for the selected item.
- **Refresh** with `r` to reload the latest state from Docker.

### Examples

Open the Docker summary while browsing:

```bash
./cleanup-tool
# Then press D
```

Prune the selected category from the summary view:

```text
Select Images → press p → confirm with y
```

Delete a single dangling image from the per-item view:

```text
Select Images → Enter → press d on the dangling image → confirm with y
```

Delete all dangling items in the current category:

```text
Select Images → Enter → press D
```

### Keyboard shortcuts

#### Docker summary view

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| Enter | Drill into selected category |
| p | Prune selected category |
| r | Refresh summary |
| Esc / q | Back / quit |

#### Docker items view

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| d | Delete selected item |
| D | Delete all dangling items in the current category |
| f | Cycle filter: all → dangling → unused |
| g | Group by project label |
| i | Show/hide labels panel |
| r | Refresh list |
| Esc | Back to summary |
| q | Quit |

### Safety notes

- **Prunes are destructive.** `cleanup-tool` asks for confirmation before deleting individual items and before pruning an entire category.
- **Running containers are protected.** You must stop or remove a running container before its image can be freed.
- **Volumes used by containers are shown as in-use.** Delete the container first if you really want to reclaim the volume.
- **Volumes report `0 B` size** in the per-item list because `docker volume ls` does not return size; only the summary row shows volume size from `docker system df`.

## Benchmark mode

Use `-benchmark` to run a non-interactive scan and print throughput statistics. This is useful for measuring raw scan performance, comparing before/after tuning `ignore_paths`, or verifying that a network/external drive is not slowing things down.

### Examples

```bash
# Benchmark the default home directory scan
./cleanup-tool -benchmark

# Benchmark a specific path
./cleanup-tool -benchmark -paths ~/Downloads

# Benchmark with hidden files skipped
./cleanup-tool -benchmark -paths ~/Documents -ignore-hidden
```

### Sample output

```bash
./cleanup-tool -benchmark -paths /tmp/cleanup-benchmark-sample
```

```
Scan benchmark
Paths: /tmp/cleanup-benchmark-sample
Total time: 4ms
Files: 200
Dirs:  11
Avg throughput: 56016 files/sec, 3081 dirs/sec
Total size: 200.0 KB
```

### Interpreting the stats

| Stat | Meaning |
|------|---------|
| `Total time` | How long the scan took from start to finish. |
| `Files` / `Dirs` | Number of files and directories scanned. The directory count includes each scanned root. |
| `Avg throughput` | Average files and directories processed per second. Higher is better. These numbers are most meaningful on large, local directories. |
| `Total size` | Total logical size of everything under the scanned paths. |

### When benchmark numbers are misleading

- **Small directories** (a few files) finish in milliseconds, so the "per second" rate can look artificially high or low due to startup overhead.
- **Network drives** and **external SSDs** usually show lower throughput than the internal drive.
- **Hot vs. cold filesystem caches** can cause big differences between back-to-back runs. For a fair comparison, run the benchmark twice and use the second run, or drop caches between runs (on macOS: `sync && sudo purge`).
- **Counting directories** includes the root path itself, so a directory with only files still shows at least one directory.

### Example: before and after adding `ignore_paths`

Suppose you have a project whose `node_modules` directory you do not need to scan. Add the directory to `~/.config/cleanup-tool/config.json`:

> `ignore_paths` matches prefixes, so the entry must be the absolute path of the directory you want to skip.

```json
{
  "version": 2,
  "ignore_paths": ["/Users/dev/projects/my-app/node_modules"]
}
```

Run the benchmark before the change:

```bash
./cleanup-tool -benchmark -paths /Users/dev/projects/my-app
```

```
Scan benchmark
Paths: /Users/dev/projects/my-app
Total time: 1ms
Files: 250
Dirs:  3
Avg throughput: 178635 files/sec, 2144 dirs/sec
Total size: 250.0 KB
```

Run the benchmark after the change:

```
Scan benchmark
Paths: /Users/dev/projects/my-app
Total time: 1ms
Files: 50
Dirs:  2
Avg throughput: 65891 files/sec, 2636 dirs/sec
Total size: 50.0 KB
```

In this example, ignoring `node_modules` cut the scanned file count from 250 to 50 and the total size from 250 KB to 50 KB. The throughput numbers vary from run to run, but the reduction in work is what matters.

### Common use cases

- **Compare scan settings** — run `-benchmark` before and after adding paths to `ignore_paths` to see the speedup.
- **Validate hardware changes** — compare throughput after switching from a spinning disk to an SSD or after moving a project to an external drive.
- **Troubleshoot slow scans** — if the TUI feels sluggish, `-benchmark` tells you how fast the scanner is. If the benchmark throughput is low, the scanner (not the analyzer) is likely the bottleneck.

## Performance

This section explains what makes scanning and analysis fast or slow, and how to pick settings that match your hardware and data.

### Scanner

The scanner walks all provided roots concurrently. It uses two bounded semaphore pools to avoid exhausting file descriptors or spawning an unbounded number of goroutines:

- **Directory reads** (`readDirSem`): up to `CPU count × 4`, clamped between 64 and 256.
- **Metadata/stat lookups** (`statSem`): up to `CPU count × 8`, clamped between 128 and 512.

These defaults are tuned for fast local SSDs. You do not need to tune them; the scanner automatically throttles itself.

#### What slows the scanner down

- **Very large directories** with tens of thousands of entries still serialize on the metadata semaphore, so a single huge directory can become a bottleneck.
- **Network drives** and **external HDDs** usually cannot sustain the same concurrency as an internal SSD; throughput drops because the drives become the bottleneck, not the CPU.
- **Hot/cold filesystem caches** can make the same scan twice as fast or slow. Run `-benchmark` twice and use the second result for a fair comparison.
- **Hidden files** add overhead only if you need them; use `-ignore-hidden` when you don't.

### Analyzer

The analyzer runs on demand (press `a` in the TUI). It does three things:

1. **Age check**: compares access time against a threshold. This is essentially free.
2. **Category check**: checks if a file is classified as a log/cache file. Also essentially free.
3. **Duplicate detection**: reads file contents and hashes them. This is the expensive part.

### Duplicate-detection modes

| Mode | Speed | Accuracy | When to use |
|------|-------|----------|-------------|
| `none` | Fastest | No duplicate detection | You only care about old/log/cache files |
| `sample` | Fast | High | Large media/Downloads folders |
| `first10mb` | Medium | Medium | Legacy mode; mostly for compatibility |
| `full` | Slowest | Exact | Small, critical data sets |
| `smart` | Fast to medium | Near-exact | Default; recommended for most cases |

- **`smart`** groups files by size first. Files with unique sizes cannot be duplicates, so they are skipped. Colliding sizes are sample-hashed; only samples that collide are full-hashed. This gives near-exact results without reading every byte of every file.
- **`sample`** reads 1 MB from the start, middle, and end of each file. Good for large media libraries where reading the whole file would be slow.
- **`first10mb`** hashes only the first 10 MB. Fast but may miss duplicates that differ after 10 MB.
- **`full`** reads every byte. Most accurate, but can be very slow on large files.
- **`none`** skips duplicate detection entirely.

### Memory usage

- The scanner builds the entire directory tree in memory before the TUI or analyzer runs. Large scans can use hundreds of megabytes or more.
- Duplicate detection keeps hash keys in memory but does not keep file contents in memory.
- If you are scanning very large paths and memory is tight, split the scan into smaller paths or ignore the largest subtrees.

### Recommended settings

| Situation | Suggested flags |
|-----------|-----------------|
| Quick overview | `-ignore-hidden -dup-mode none` |
| Large media/Downloads | `-dup-mode sample` or `-dup-mode smart` |
| Critical small dataset | `-dup-mode full` |
| Slow external drive | `-ignore-hidden -dup-mode none`, then re-run with `smart` only on suspect paths |
| CI/automation | `-benchmark` first, then use the appropriate `-dup-mode` |

## Configuration file

`cleanup-tool` reads its configuration from `~/.config/cleanup-tool/config.json`.
You can create and edit this file directly; CLI flags override the corresponding config values where applicable.

Example `~/.config/cleanup-tool/config.json`:

```json
{
  "version": 2,
  "ignore_paths": [
    "/System",
    "/Volumes",
    "/dev",
    "/proc",
    "/net",
    "/private/var/db/timezone"
  ],
  "ignore_hidden": false,
  "trash_only": true,
  "dup_mode": "smart",
  "progress_interval": 100,
  "deps_targets": [
    "node_modules",
    ".pnpm",
    "vendor",
    ".venv",
    "venv",
    "bower_components",
    "Pods",
    "Carthage",
    ".gradle",
    ".m2",
    "target",
    ".tox",
    "packages",
    ".nuget",
    ".stack-work",
    "elm-stuff",
    "_build"
  ]
}
```

### Configuration fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | int | Config schema version (currently `2`) |
| `ignore_paths` | list of strings | Absolute paths to skip during scans |
| `ignore_hidden` | bool | Skip hidden files and directories by default |
| `trash_only` | bool | **Reserved**: currently not enforced by the CLI |
| `dup_mode` | string | Default duplicate-detection mode: `first10mb`, `sample`, `full`, `smart` |
| `progress_interval` | int | Report scan/analyzer progress every N items |
| `deps_targets` | list of strings | Dependency directory names used by the `deps` subcommand when `-targets` is not provided |

The `deps` subcommand uses `deps_targets` from the config when you do not pass `-targets` explicitly. If `deps_targets` is missing from the config, the built-in defaults are used.

If the config file contains invalid JSON, `deps` prints a warning to stderr and falls back to the built-in default targets. If you pass `-targets` explicitly, that value is still honored even when the config is invalid. Other subcommands may still exit on a broken config, but `deps` is designed to keep working so that `deps -help` and basic discovery still work. See [Dependency directories](#dependency-directories) for the full subcommand documentation.

For example, the command below scans for the directories listed in the config:

```bash
./cleanup-tool deps -paths ~/projects
```

You can always override the configured list for a single run:

```bash
./cleanup-tool deps -paths ~/projects -targets node_modules,vendor
```

## Key bindings

### File browser (terminal style)

The classic terminal-style file browser. Select it with `-tui-style terminal` (`tree` is still accepted as an alias).

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| l / Enter / → | Expand / collapse selected directory |
| h / Esc / ← | Collapse directory or move selection to parent |
| Space | Mark / unmark item |
| c | Clear all marks |
| d | Move selected item to Trash |
| m | Move selected item to external drive (`-external`) |
| u | Restore last moved / trashed item |
| Z | Undo last trash / move operation |
| a | Analyze selected directory |
| A | Analyze selected items |
| P | Open dependency directories view (node_modules, vendor, .venv, Pods, etc.) |
| D | Open Docker disk usage |
| q | Quit |

### Dua-style browser

This is the default interactive view. It shows a flat list of the entries in the
current directory, sorted by size, with a percentage and a small bar for each
item. Select the terminal view with `-tui-style terminal`.

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| Enter / l | Descend into selected directory |
| Backspace / h / u / Esc | Go to parent directory |
| d | Mark / unmark selected item |
| x | Trash marked items (or selected item if none marked) |
| m | Move marked items (or selected item) to external drive (`-external`) |
| r | Restore selected item from Trash |
| Z | Undo last trash / move operation |
| c | Clear all marks |
| a | Analyze current directory |
| A | Analyze selected / marked items |
| P | Open dependency directories view (node_modules, vendor, .venv, Pods, etc.) |
| D | Open Docker disk usage |
| ? | Toggle help |
| q | Quit |

### Analyzer summary

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| Tab / ← / → | Filter by category |
| 0 | Clear category filter |
| Space | Mark / unmark hint |
| c | Clear all marks |
| d | Trash marked hints |
| Esc | Back |
| q | Quit |

### Docker summary view

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| Enter | Drill into selected category |
| p | Prune selected category |
| r | Refresh summary |
| Esc | Back |
| q | Quit |

### Docker items view

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| d | Delete selected item |
| D | Delete all dangling items in the current category |
| f | Cycle filter: all → dangling → unused |
| g | Group by project label |
| i | Show/hide labels panel |
| r | Refresh list |
| Esc | Back to summary |
| q | Quit |

### Confirm prune

| Key | Action |
|-----|--------|
| y | Yes |
| n | No |

## Demo

The screenshots below are representative ASCII mockups of the current TUI. The
default interactive view is the **dua-style browser**; the terminal-style tree
browser is available with `-tui-style terminal`.

> Want to see the real thing? Record a session with [asciinema](#recording-a-demo) or open an issue if you'd like an official GIF.

### Scanning progress

```
Cleanup Tool — scanning...
  Scanning... /Users/dev/projects/big-repo
  12,405 files, 1,034 dirs
```

### Dua-style browser (default)

The default interactive view shows a flat, size-sorted list with a percentage
and a small bar for each item.

```
Cleanup Tool — /Users/dev
  total: 312.4 GB  marked: 2

   Size      %   Name
 89.1 GB   28% ████████████████  Llama-3-70B
 24.7 GB    8% █████             raw-images.tar
 12.3 GB    4% ██                  release
  4.1 GB    1% █                   projects
  2.8 GB   <1% ▏                   docs
  1.2 GB   <1% ▏                   notes

[j/k/down/up] navigate  [enter/l] descend  [h/u/esc] parent
[d] mark  [x] trash marked  [m] move  [r] restore
[a] analyze  [A] analyze selection  [D] Docker  [q] quit
```

### Terminal-style browser

The classic terminal-style tree browser. Select it with `-tui-style terminal`
(`tree` is still accepted as an alias).

```
Cleanup Tool
  total: 312.4 GB  marked: 2
  scanned 1,245,032 files, 98,422 dirs in 12.34s (peak 102,391 files/sec, 8,112 dirs/sec)

       Size      Access       Category        Name
[x]   89.1 GB  2025-03-12   llm-model       Llama-3-70B
[ ]   24.7 GB  2024-11-08   docker          raw-images.tar
[ ]   12.3 GB  2025-01-19   build-artifact    release
▼[ ]    4.1 GB  2024-09-30   Directory       projects
    [ ]  2.8 GB  2024-09-30   Directory     docs
    [ ]  1.2 GB  2024-09-30   Directory     notes

[j/k/down/up] navigate  [l/enter/right] expand  [h/esc/left] collapse
[c] clear  [d] trash  [m] move  [u] restore  [a] analyze dir
[A] analyze selection  [D] Docker  [q] quit
```

### Analyzer summary

```
Deletability Analysis

Found 6 hints

2 old files, 3 duplicates, 1 log/cache
  ████████████████████████
  ^ old       ^ duplicate    ^ log/cache (each segment is colored in the TUI)

Showing 6 of 6
       Reason          Detail          Path
[x]   old                 last accessed   old-logs/app.log
[ ]   duplicate           3 duplicates    photos/img_001.jpg
[ ]   log/cache           log-cache       .cache/npm/abc123

[j/k/down/up] nav  [tab/←/→] filter  [0] clear filter  [c] clear marks
[space] mark  [d] trash marked  [esc] back  [q] quit
```

### Docker disk usage

```
Docker Disk Usage

Images       size: 24.7 GB  reclaimable: 18.2 GB  count: 12
Containers   size: 0 B      reclaimable: 0 B       count: 0
Volumes      size: 8.1 GB   reclaimable: 3.4 GB   count: 4
Build Cache  size: 1.2 GB   reclaimable: 1.1 GB   count: 23

Total used: 34.0 GB

[↑/↓/j/k] navigate  [p] prune selected  [r] refresh  [esc] back  [q] quit
```

## Recording a demo

You can capture a real terminal session with [asciinema](https://asciinema.org/):

```bash
# Record the default dua-style view
asciinema rec cleanup-tool-demo.cast --command "./cleanup-tool -paths ~/Downloads,~/Documents"

# Record the terminal-style tree view instead
asciinema rec cleanup-tool-terminal-demo.cast --command "./cleanup-tool -tui-style terminal -paths ~/Downloads,~/Documents"

# Play locally
asciinema play cleanup-tool-demo.cast
asciinema play cleanup-tool-terminal-demo.cast

# Share (after uploading to asciinema.org)
asciinema upload cleanup-tool-demo.cast
asciinema upload cleanup-tool-terminal-demo.cast
```

For a GIF, convert the cast with [agg](https://github.com/asciinema/agg) or record with a terminal GIF tool such as [vhs](https://github.com/charmbracelet/vhs).

## Tips & Tricks

### Docker quick clean

1. Run `cleanup-tool` and press `D` to open **Docker Disk Usage**.
2. Select **Images** or **Build Cache** and press `p` to prune.
3. Confirm with `y`.

### Use the default dua-style shortcuts

`cleanup-tool` now starts in the dua-style view by default. A few shortcuts make it fast once the scan finishes:

- `j` / `k` (or ↑ / ↓) move up and down the size-sorted list.
- `Enter` or `l` descend into a directory; `h`, `u`, or `Esc` go back up.
- `d` marks an item; `x` trashes all marked items; `m` moves them to the `-external` drive.
- `a` analyzes the current directory; `A` analyzes the selected/marked items.
- `D` opens Docker disk usage.

Run `./cleanup-tool -tui-style terminal` if you prefer the classic tree browser.

### Find duplicates fast

Use the sample hash mode on large directories like **Downloads** or **Photos**, then run the analyzer with `a` or `A` to see duplicates faster:

```bash
./cleanup-tool -paths ~/Downloads,~/Pictures -dup-mode sample
```

### Analyze before deleting

In either browser, navigate to a directory and press `a` to run the deletability analyzer.
Filter by category with `Tab` / arrow keys, mark hints with `Space`, and press `d` to trash the marked ones.

### Move before you trash

Set an external drive with `-external` and press `m` to move selected items there first.
If you change your mind, press `u` to restore the last moved or trashed item.

```bash
./cleanup-tool -external /Volumes/External/cleanup-backups -paths ~/Downloads
```

### Benchmark scan speed

Run a non-interactive scan before and after a cleanup session to see the speed and total size:

```bash
./cleanup-tool -benchmark -paths ~/Documents
```

### Keep scans fast

Add folders you never want to scan (e.g. `~/Library/Caches`, `node_modules`) to the `ignore_paths` array in `~/.config/cleanup-tool/config.json`.

## Troubleshooting

### The TUI won't start

- Make sure you are running in a terminal that supports a TTY. Some CI/IDE consoles don't work with Bubble Tea.
- If colors cause rendering issues, try `TERM=xterm-256color ./cleanup-tool`.

### Scanning is very slow

- Skip hidden files with `-ignore-hidden`.
- Add slow or uninteresting directories (e.g. `~/Library/Caches`, `node_modules`) to `ignore_paths` in `~/.config/cleanup-tool/config.json`.
- Run `-benchmark -paths <dir>` to see the raw scan throughput.

### Docker disk usage shows an error

- Make sure Docker Desktop is running and `docker` is in your `PATH`.
- The app runs `docker system df`, which needs a working Docker context but not root on macOS.

### Move to external drive fails

- Check that `-external` points to an existing directory and that you have write permissions.
- Example: `-external /Volumes/MyDrive/cleanup-backups`.
- Make sure `rsync` is installed (`which rsync`); the move action uses `rsync` under the hood.

### I accidentally trashed something

- In the file browser, press `Z` to undo the last trash or move operation, or `u` to restore the selected item from Trash.
- If you already quit the app, open Trash in Finder and move the item back manually.

### Duplicate detection is slow or uses too much CPU

- Use `-dup-mode sample` for a faster, less exhaustive check.
- `-dup-mode smart` (default) balances speed and accuracy.
- `-dup-mode full` reads every byte of every file and is the slowest.

### The config file isn't being read

- Verify the file exists at `~/.config/cleanup-tool/config.json`.
- If the JSON is invalid, the app prints `config load error:` and exits. Fix the JSON and try again.

## Trash and undo

`cleanup-tool` uses macOS Finder to move items to Trash, which preserves Finder's native behavior (sounds, animations, and "Put Back" support). Undo in the TUI restores items from Trash back to their original locations.

### Where trashed items go

- **Boot volume**: Items are moved to `~/.Trash` (the standard macOS user Trash).
- **External volumes**: macOS stores trashed items in `.Trashes/<uid>/` at the root of the external volume, for example:

  ```
  /Volumes/MyDrive/.Trashes/501/
  ```

  `cleanup-tool` detects the source volume and tracks the correct Trash path for each item, so undo works for files trashed from external drives too.

### Conflict handling

If an item with the same name is already in the Trash, Finder renames the new item (for example, `foo.txt` becomes `foo 1.txt`). `cleanup-tool` captures source metadata before trashing and matches size/modification time to identify the correct Trash destination, even when names collide. Two files with the same basename trashed in the same batch are matched to distinct Trash paths so undo restores each item correctly.

### Restore safety

When you undo a trash or move operation:

- The item is moved from its Trash (or external) location back to its original path.
- If the original path has been reused by a new file, the new file is moved aside with a `-restored-<nanos>` suffix instead of being overwritten.
- If the source path no longer exists, undo stops and reports the error so later items are not restored incorrectly.

### Limitations

- Undo relies on the Trash path recorded at the time of the operation. If you manually move or rename an item inside Trash after trashing it, undo may not find it.
- Cross-device moves (for example, restoring from an external drive to the boot volume) use `rsync` as a fallback. Make sure `rsync` is installed.

## Releasing

The [`release.yml`](.github/workflows/release.yml) workflow automatically builds, signs, and publishes a GitHub Release when a `v*` tag is pushed. The [`ci.yml`](.github/workflows/ci.yml) workflow runs tests and a release smoke test on every push to `main` and on pull requests.

For the full release checklist, including GPG setup, see [`docs/releasing.md`](docs/releasing.md).

## Roadmap

### Done

- [x] Parallel directory scanner with real-time progress and throughput stats
- [x] Terminal-style tree view with expand/collapse in the file browser
- [x] Categorisation of common space hogs (Docker, LLMs, build artifacts, deps, media, archives, logs, caches)
- [x] Configurable ignore paths via `~/.config/cleanup-tool/config.json`
- [x] Duplicate file detection with configurable hashing modes
- [x] Deletability analyzer with per-category filtering, stacked bar summary, and live progress
- [x] Docker image/volume/cache analysis and prune wrapper
- [x] Batch mark / move / trash / restore across directories
- [x] Mouse and keyboard support in the analyzer summary
- [x] Benchmark mode (`-benchmark`)
- [x] Saved rules with non-interactive execution
- [x] launchd automation for saved rules
- [x] Dua-style interactive browser (default) with flat size-sorted list, percentages, and bars
- [x] LLM registry inspection and model deletion (Ollama, Hugging Face cache, LM Studio) via TUI (`M` key) and CLI (`models list`, `models delete`)
- [x] Export scan results to JSON, CSV, TSV, and YAML with auto-detected formats (`-out`, `-stdout`, `-format`, `-csv-columns`)
- [x] CI/CD smoke tests and GPG-signed macOS universal releases

### Coming soon

- [x] Remove deprecated `-json-out` flag (use `-out` instead) — tracked in `.github/ISSUE_TEMPLATE/remove-deprecated-json-out-flag.md`
- [x] Native Homebrew formula

## Contributing

Bug reports, feature requests, and pull requests are welcome. Please open an issue before major changes so we can agree on direction.

### CI checks

The following checks run on every pull request:

- `go test ./...` and `go vet ./...` on Ubuntu and macOS.
- Unit tests for the `actions` package both with and without `rsync` in PATH.
- macOS integration tests for the `actions` package with a real `osascript` environment.
- Docker client tests against a mock Docker daemon.
- A release smoke test that builds the macOS universal binary, tarball, and checksums.
- A Homebrew formula audit against the [`patriciomg/homebrew-cleanup-tool`](https://github.com/patriciomg/homebrew-cleanup-tool) tap.
- **`test-benchmark-format`** — verifies that the `-benchmark` output format has not changed.
- **`test-help-format`** — verifies that the CLI `-h`/`-help` output format has not changed.
- **`test-export-format`** — verifies that the JSON, CSV, TSV, and YAML export output formats have not changed.

#### `test-benchmark-format`

This job ensures the benchmark output example in the [Benchmark mode](#benchmark-mode) section stays in sync with the code.

What it does:

1. Builds the `cleanup-tool` binary.
2. Creates a deterministic sample directory using [`testdata/create-benchmark-sample.sh`](testdata/create-benchmark-sample.sh).
3. Runs `./cleanup-tool -benchmark -paths /tmp/cleanup-benchmark-sample`.
4. Normalizes variable parts of the output (`Total time` and `Avg throughput`).
5. Differs the normalized output against [`testdata/benchmark-snapshot.txt`](testdata/benchmark-snapshot.txt).

If the format changes, the job fails with:

```
Benchmark output format changed. Update testdata/benchmark-snapshot.txt and the 'Benchmark mode' section in README.md.
```

To update the snapshot after changing the benchmark output, regenerate it locally:

```bash
go build -o cleanup-tool ./cmd/cleanup-tool
./testdata/create-benchmark-sample.sh
./cleanup-tool -benchmark -paths /tmp/cleanup-benchmark-sample | \
  sed -E 's/Total time: .*/Total time: <TIME>/; s/Avg throughput: .*/Avg throughput: <THROUGHPUT>/' \
  > testdata/benchmark-snapshot.txt
```

#### `test-help-format`

This job ensures the [CLI flags](#cli-flags) table stays in sync with the actual `-h`/`-help` output.

What it does:

1. Builds the `cleanup-tool` binary.
2. Runs `./cleanup-tool -h` and captures the help output.
3. Normalizes the binary path in the `Usage of ...:` line.
4. Differs the normalized output against [`testdata/help-snapshot.txt`](testdata/help-snapshot.txt).

If the help text changes, the job fails with:

```
CLI help output format changed. Update testdata/help-snapshot.txt and the CLI flags table in README.md.
```

To update the snapshot after changing flags or their descriptions, regenerate it locally:

```bash
go build -o cleanup-tool ./cmd/cleanup-tool
./cleanup-tool -h 2>&1 | sed 's/^Usage of .*:$/Usage of <binary>:/' > testdata/help-snapshot.txt
```

The snapshot is the canonical reference for the help text; update the CLI flags table in README.md to match it, then commit both files together.

#### `test-export-format`

This job ensures the export examples in the [Exporting results](#exporting-results) section stay in sync with the code.

What it does:

1. Builds the `cleanup-tool` binary.
2. Creates a deterministic sample directory using [`testdata/create-export-sample.sh`](testdata/create-export-sample.sh).
3. Runs `./cleanup-tool -format <fmt> -stdout -paths /tmp/cleanup-export-sample-fmt` for `json`, `csv`, `tsv`, and `yaml`.
4. Normalizes variable parts of the outputs (paths, timestamps, and file modes) using [`testdata/normalize-export.sh`](testdata/normalize-export.sh).
5. Differs the normalized outputs against the snapshots in [`testdata/export-snapshots/`](testdata/export-snapshots/).

If any format changes, the job fails with:

```
<fmt> export output format changed. Update testdata/export-snapshots/<fmt>.txt and the 'Exporting results' section in README.md.
```

To update the snapshots after changing an export format, regenerate them locally:

```bash
go build -o cleanup-tool ./cmd/cleanup-tool
./testdata/create-export-sample.sh
./cleanup-tool -format json -stdout -paths /tmp/cleanup-export-sample-fmt > export.json
./cleanup-tool -format csv -stdout -paths /tmp/cleanup-export-sample-fmt > export.csv
./cleanup-tool -format tsv -stdout -paths /tmp/cleanup-export-sample-fmt > export.tsv
./cleanup-tool -format yaml -stdout -paths /tmp/cleanup-export-sample-fmt > export.yaml
./testdata/normalize-export.sh json export.json > testdata/export-snapshots/json.txt
./testdata/normalize-export.sh csv export.csv > testdata/export-snapshots/csv.txt
./testdata/normalize-export.sh tsv export.tsv > testdata/export-snapshots/tsv.txt
./testdata/normalize-export.sh yaml export.yaml > testdata/export-snapshots/yaml.txt
```

## License

[MIT](LICENSE) © Pato
