# cleanup-tool

A fast, terminal-based disk cleanup tool tailored for macOS developers who work with Docker, LLMs, and build artifacts.

## Features

- **Parallel directory scanner** with bounded concurrency, live progress, and throughput stats
- **Categorisation** of common space hogs: Docker, LLM models, build artifacts, dependencies, media, archives, logs, and caches
- **Interactive TUI** (powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)) with a tree-style file browser sorted by size
- **Safe actions**: move to Trash, move to an external drive via `rsync`, and restore
- **Batch operations**: mark and act on items across multiple directories
- **Configurable ignore paths** via `~/.config/cleanup-tool/config.json`
- **Duplicate file detection** with configurable hashing modes (sample, first 10 MB, full, smart)
- **Deletability analyzer** that flags old files, log/cache files, and duplicates, with per-category filtering, stacked bar summary, and live progress
- **Docker disk usage** analysis and prune wrapper for images, containers, volumes, and build cache
- **Mouse and keyboard support** in the analyzer summary for filtering by category
- **Benchmark mode** to measure scan performance (`-benchmark`)
- **Saved rules** for reusable cleanup presets with non-interactive execution (`rules` subcommand)
- **CI/CD and release tooling**: smoke-tested universal macOS binaries, tarball packaging, SHA-256 checksums, and GPG-signed releases via GitHub Actions

## Install

```bash
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
./cleanup-tool -paths ~/kk,~/personal

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
| `-paths` | Comma-separated paths to scan (e.g. `~/kk,~/personal`) | `~` |
| `-ignore-hidden` | Skip hidden files and directories | `false` |
| `-external` | Directory on an external drive to use for the move action | `""` |
| `-dup-mode` | Duplicate-detection hash mode: `first10mb`, `sample`, `full`, `smart` | `smart` |
| `-progress-interval` | Report scan/analyzer progress every N items | `100` |
| `-benchmark` | Run a non-interactive scan and print throughput stats | `false` |
| `-version` | Print version and exit | `false` |
| `-out` | Export scan results to the specified file (works with any `-format`) | `""` |
| `-json-out` | Deprecated: use `-out` instead | `""` |
| `-json` | Alias for `-stdout` (non-interactive) | `false` |
| `-stdout` | Export scan results to stdout (works with any `-format`; non-interactive) | `false` |
| `-format` | Export format: `json`, `csv`, `tsv`, `yaml`. Defaults to `json`; auto-detected from `-out` extension when omitted. | `json` |
| `-csv-columns` | Comma-separated CSV/TSV column names | `""` |

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

### File browser

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
| D | Open Docker disk usage |
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

## Screenshots

The screenshots below are representative ASCII mockups of the current TUI.

### Scanning progress

```
Cleanup Tool — scanning...
  Scanning... /Users/pato/personal/projects/big-repo
  12,405 files, 1,034 dirs
```

### File browser

```Cleanup Tool
  total: 312.4 GB  marked: 2
  scanned 1,245,032 files, 98,422 dirs in 12.34s (peak 102,391 files/sec, 8,112 dirs/sec)

       Size      Access       Category        Name
[x]   89.1 GB  2025-03-12   llm-model       Llama-3-70B
[ ]   24.7 GB  2024-11-08   docker          raw-images.tar
[ ]   12.3 GB  2025-01-19   build-artifact    release
▼[ ]    4.1 GB  2024-09-30   Directory       projects
    [ ]  2.8 GB  2024-09-30   Directory     kk
    [ ]  1.2 GB  2024-09-30   Directory     personal

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
# Record
asciinema rec cleanup-tool-demo.cast --command "./cleanup-tool -paths ~/kk,~/personal"

# Play locally
asciinema play cleanup-tool-demo.cast

# Share (after uploading to asciinema.org)
asciinema upload cleanup-tool-demo.cast
```

For a GIF, convert the cast with [agg](https://github.com/asciinema/agg) or record with a terminal GIF tool such as [vhs](https://github.com/charmbracelet/vhs).

## Tips & Tricks

### Docker quick clean

1. Run `cleanup-tool` and press `D` to open **Docker Disk Usage**.
2. Select **Images** or **Build Cache** and press `p` to prune.
3. Confirm with `y`.

### Find duplicates fast

Use the sample hash mode on large directories like **Downloads** or **Photos**, then run the analyzer with `a` or `A` to see duplicates faster:

```bash
./cleanup-tool -paths ~/Downloads,~/Pictures -dup-mode sample
```

### Analyze before deleting

In the file browser, navigate to a directory and press `a` to run the deletability analyzer.
Filter by category with `Tab` / arrow keys, mark items with `Space`, and press `d` to trash the marked ones.

### Move before you trash

Set an external drive with `-external` and press `m` to move selected items there first.
If you change your mind, press `u` to restore the last moved or trashed item.

```bash
./cleanup-tool -external /Volumes/External/cleanup-backups -paths ~/kk
```

### Benchmark scan speed

Run a non-interactive scan before and after a cleanup session to see the speed and total size:

```bash
./cleanup-tool -benchmark -paths ~/personal
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

Releases are built and signed automatically from Git tags via [`.github/workflows/release.yml`](../.github/workflows/release.yml). The pipeline builds a macOS universal binary, packages it as a tarball, generates a checksum file, signs everything with GPG, and attaches the assets to the GitHub Release.

> **Note:** `make release` must be run on macOS because the universal binary is created with `lipo`.

### Local release artifacts

```bash
# Build the universal binary, tarball, and checksums locally
cd cleanup-tool
make release

# Sign the artifacts with GPG (requires a configured GPG key).
# Optional: use GPG_KEY_ID to select a specific key.
make release-sign
# or: GPG_KEY_ID=YOUR_KEY_ID make release-sign

# Verify a signature
gpg --verify dist/checksums.txt.asc dist/checksums.txt
gpg --verify dist/cleanup-tool-<version>-darwin-universal.tar.gz.asc \
            dist/cleanup-tool-<version>-darwin-universal.tar.gz
```

### Publishing a release from GitHub Actions

1. **Generate a GPG key** (if you do not already have one):

   ```bash
   gpg --full-generate-key
   # Choose a secure key type, e.g., ED25519 or RSA/RSA 4096 bits.
   ```

2. **Export the private key** for GitHub Actions:

   ```bash
   gpg --list-secret-keys --keyid-format LONG
   # Use the long key ID from the output above
   gpg --armor --export-secret-keys <KEY_ID> > cleanup-tool-release.asc
   ```

3. **Add the secrets to your repository** on GitHub under **Settings → Secrets and variables → Actions**:

   - `GPG_PRIVATE_KEY`: the contents of `cleanup-tool-release.asc`
   - `GPG_PASSPHRASE`: the passphrase for that key. **Only create this secret if your key is protected by a passphrase**; if the key has no passphrase, omit this secret entirely. If `GPG_PRIVATE_KEY` is not set, the workflow will still publish a release, but the assets will not be signed.

4. **Push a version tag** (`v*`):

   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

5. The `Release` workflow will:

   - Run tests and `go vet` on `macos-latest`
   - Build `cleanup-tool-darwin-universal` and the release tarball
   - Generate `checksums.txt`
   - Import the GPG key and run `make release-sign`
   - Verify the signatures
   - Create the GitHub Release and upload the tarball, checksum file, and `.asc` signatures

### Verifying a published release

```bash
# Download the public key from the release author and import it
gpg --import <author-public-key.asc>

# Verify the checksum file signature
gpg --verify checksums.txt.asc checksums.txt

# Verify the tarball signature
gpg --verify cleanup-tool-<version>-darwin-universal.tar.gz.asc \
            cleanup-tool-<version>-darwin-universal.tar.gz
```

## Roadmap

### Done

- [x] Parallel directory scanner with real-time progress and throughput stats
- [x] Tree view with expand/collapse in the file browser
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
- [x] CI/CD smoke tests and GPG-signed macOS universal releases

### Coming soon

- [ ] Remove deprecated `-json-out` flag (use `-out` instead) — tracked in `.github/ISSUE_TEMPLATE/remove-deprecated-json-out-flag.md`
- [ ] LLM registry cleanup (Ollama, Hugging Face cache, LM Studio)
- [ ] Export scan results to JSON
- [ ] Native Homebrew formula
