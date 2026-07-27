# cleanup-tool

A fast, terminal-based disk cleanup tool tailored for macOS developers who work with Docker, LLMs, and build artifacts.

## Features

- **Parallel directory scanner** with bounded concurrency, live progress, and throughput stats
- **Categorisation** of common space hogs: Docker, LLM models, build artifacts, dependencies, media, archives, logs, and caches
- **Interactive TUI** (powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)) sorted by size
- **Safe actions**: move to Trash, move to an external drive via `rsync`, and restore
- **Batch operations**: mark and act on items across multiple directories
- **Configurable ignore paths** via `~/.config/cleanup-tool/config.json`
- **Duplicate file detection** with configurable hashing modes (sample, first 10 MB, full, smart)
- **Deletability analyzer** that flags old files, log/cache files, and duplicates, with per-category filtering, sparkline summary, and live progress
- **Docker disk usage** analysis and prune wrapper for images, containers, volumes, and build cache
- **Mouse and keyboard support** in the analyzer summary for filtering by category
- **Benchmark mode** to measure scan performance (`-benchmark`)
- **CI/CD and release tooling**: smoke-tested universal macOS binaries, tarball packaging, SHA-256 checksums, and GPG-signed releases via GitHub Actions

## Install

```bash
cd cleanup-tool
go build ./cmd/cleanup-tool
```

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

## Key bindings

### File browser

| Key | Action |
|-----|--------|
| ↑ / ↓ / j / k | Navigate |
| l / Enter | Open selected item |
| h / Esc | Go back |
| Space | Mark / unmark item |
| c | Clear all marks |
| d | Move selected item to Trash |
| m | Move selected item to external drive (`-external`) |
| u | Restore last moved / trashed item |
| a | Analyze current directory |
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

```
Cleanup Tool — /Users/pato/personal
  total: 312.4 GB  marked: 2
  scanned 1,245,032 files, 98,422 dirs in 12.34s (peak 102,391 files/sec, 8,112 dirs/sec)

       Size      Access       Category        Path
[x]   89.1 GB  2025-03-12   llm-model       models/Llama-3-70B
[ ]   24.7 GB  2024-11-08   docker          Docker/raw-images.tar
[ ]   12.3 GB  2025-01-19   build-artifact  target/release
[ ]    4.1 GB  2024-09-30   Directory       projects/kk

[j/k/down/up] navigate  [l/enter] open  [h/esc] back  [space] mark
[c] clear  [d] trash  [m] move  [u] restore  [a] analyze dir
[A] analyze selection  [D] Docker  [q] quit
```

### Analyzer summary

```
Deletability Analysis

Found 6 hints

2 old files ████, 3 duplicates ██████, 1 log/cache █
  ▄█ ▄▄█ ▁

Showing 6 of 6
       Reason          Detail          Path
[x]   untouched > 1 year  last accessed   old-logs/app.log
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
- [x] Categorisation of common space hogs (Docker, LLMs, build artifacts, deps, media, archives, logs, caches)
- [x] Configurable ignore paths via `~/.config/cleanup-tool/config.json`
- [x] Duplicate file detection with configurable hashing modes
- [x] Deletability analyzer with per-category filtering, sparkline summary, and live progress
- [x] Docker image/volume/cache analysis and prune wrapper
- [x] Batch mark / move / trash / restore across directories
- [x] Mouse and keyboard support in the analyzer summary
- [x] Benchmark mode (`-benchmark`)
- [x] CI/CD smoke tests and GPG-signed macOS universal releases

### Coming soon

- [ ] Tree view with expand/collapse in the file browser
- [ ] Saved rules and weekly automation via launchd
- [ ] LLM registry cleanup (Ollama, Hugging Face cache, LM Studio)
- [ ] Export scan results to JSON
- [ ] Native Homebrew formula
