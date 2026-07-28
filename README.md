# cleanup-tool

[![CI](https://github.com/patriciomg/cleanup-tool/actions/workflows/ci.yml/badge.svg)](https://github.com/patriciomg/cleanup-tool/actions/workflows/ci.yml)
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
- [Exporting results](#exporting-results)
- [Saved rules](#saved-rules)
- [Scheduling rules with launchd](#scheduling-rules-with-launchd)
- [Configuration file](#configuration-file)
- [Key bindings](#key-bindings)
- [Demo](#demo)
- [Recording a demo](#recording-a-demo)
- [Tips & Tricks](#tips--tricks)
- [Troubleshooting](#troubleshooting)
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
| `-tui-style` | Interactive TUI style: `terminal` or `dua` | `dua` |

## Exporting results

You can export a non-interactive scan to several formats. The format is controlled by `-format` and can be auto-detected from the `-out` file extension.

### JSON

```bash
# Auto-detected from .json extension
./cleanup-tool -out /tmp/scan.json -paths /tmp

# Or explicitly
./cleanup-tool -format json -out /tmp/scan.json -paths /tmp
```

### CSV

```bash
# Auto-detected from .csv extension
./cleanup-tool -out /tmp/scan.csv -paths /tmp

# Select only specific columns
./cleanup-tool -format csv -csv-columns "Name,Size" -out /tmp/scan.csv -paths /tmp
```

### TSV

```bash
./cleanup-tool -format tsv -out /tmp/scan.tsv -paths /tmp
```

### YAML

```bash
./cleanup-tool -format yaml -out /tmp/scan.yaml -paths /tmp
```

### Stream to stdout

Use `-stdout` (or `-json`) to write the exported format to stdout instead of a file:

```bash
./cleanup-tool -stdout -format csv -paths /tmp > scan.csv
./cleanup-tool -stdout -format yaml -paths /tmp
```

### Unsupported extensions

If `-out` has an extension that is not `.json`, `.csv`, `.tsv`, `.yaml`, or `.yml`, you must specify `-format` explicitly:

```bash
./cleanup-tool -format json -out /tmp/scan.dat -paths /tmp
```

## Saved rules

Rules are reusable cleanup presets stored in `~/.config/cleanup-tool/rules.json`.

```bash
# Create a rule
./cleanup-tool rules create --name weekly-logs \
  --paths ~/Library/Logs,~/Library/Caches \
  --categories log/cache \
  --action trash \
  --age-threshold-days 30

# List rules
./cleanup-tool rules list

# Show a rule as JSON
./cleanup-tool rules show weekly-logs

# Edit a rule in $EDITOR
./cleanup-tool rules edit weekly-logs

# Delete a rule
./cleanup-tool rules delete weekly-logs

# Dry-run and run a rule
./cleanup-tool rules run weekly-logs --dry-run
./cleanup-tool rules run weekly-logs --yes
```

Rule fields:

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

## Scheduling rules with launchd

Rules can be scheduled as macOS user agents in `~/Library/LaunchAgents`. The `schedule` subcommand installs, removes, and lists these agents.

```bash
# Run a rule every day at 10:00
./cleanup-tool schedule install cache-cleanup --daily --at 10:00

# Run a rule every Monday at 09:00
./cleanup-tool schedule install cache-cleanup --weekly --day Mon --at 09:00

# Run a rule every hour
./cleanup-tool schedule install cache-cleanup --interval 3600

# Run a rule once after login
./cleanup-tool schedule install cache-cleanup --on-login

# List installed schedules
./cleanup-tool schedule list

# Remove a schedule
./cleanup-tool schedule remove cache-cleanup
```

Scheduled runs execute the rule with `--yes`, so they skip the confirmation prompt. Output is written to `~/.local/state/cleanup-tool/<rule>.log`.

> **Note:** schedule options (`--daily`, `--weekly`, `--interval`, `--on-login`) are mutually exclusive. The plist stores the absolute path to the `cleanup-tool` binary, so moving or deleting the binary after scheduling will break the schedule.

### Schedule options

| Option | Description | Example |
|--------|-------------|---------|
| `--daily --at HH:MM` | Run every day at the given time | `--daily --at 10:00` |
| `--weekly --day D --at HH:MM` | Run on a given weekday | `--weekly --day Mon --at 09:00` |
| `--interval SECONDS` | Run every N seconds | `--interval 3600` |
| `--on-login` | Run once after user login | `--on-login` |

## Configuration file

`cleanup-tool` reads its configuration from `~/.config/cleanup-tool/config.json`.
You can create and edit this file directly; CLI flags override the corresponding config values where applicable.

Example `~/.config/cleanup-tool/config.json`:

```json
{
  "version": 1,
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
  "progress_interval": 100
}
```

### Configuration fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | int | Config schema version (currently `1`) |
| `ignore_paths` | list of strings | Absolute paths to skip during scans |
| `ignore_hidden` | bool | Skip hidden files and directories by default |
| `trash_only` | bool | **Reserved**: currently not enforced by the CLI |
| `dup_mode` | string | Default duplicate-detection mode: `first10mb`, `sample`, `full`, `smart` |
| `progress_interval` | int | Report scan/analyzer progress every N items |

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

### Docker view

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| p | Prune selected |
| r | Refresh |
| Esc | Back |
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

- In the file browser, press `u` to restore the last moved or trashed item.
- If you already quit the app, open Trash in Finder and move the item back manually.

### Duplicate detection is slow or uses too much CPU

- Use `-dup-mode sample` for a faster, less exhaustive check.
- `-dup-mode smart` (default) balances speed and accuracy.
- `-dup-mode full` reads every byte of every file and is the slowest.

### The config file isn't being read

- Verify the file exists at `~/.config/cleanup-tool/config.json`.
- If the JSON is invalid, the app prints `config load error:` and exits. Fix the JSON and try again.

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

## License

[MIT](LICENSE) © Pato
