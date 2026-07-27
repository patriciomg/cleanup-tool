# Changelog

All notable changes to `cleanup-tool` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Saved rules** (`rules` subcommand) for reusable cleanup presets with create, list, show, edit, delete, and run commands.
- **Non-interactive rule execution** with dry-run mode, category filtering, age threshold override, max-deleted-bytes safety limit, and optional confirmation prompt.
- **launchd scheduling** (`schedule` subcommand) to install, remove, and list macOS user agents for saved rules. Supports daily, weekly, interval, and on-login triggers.
- Example rule in `README.md` for cleaning `~/Library/Caches`.
- `internal/launchd` package for generating plists and managing `launchctl` user agents.
- **Duplicate file detection** with five configurable hash modes: `none`, `first10mb`, `sample`, `full`, and `smart` (size → sample → full).
- **Deletability analyzer** that flags files as:
  - older than one year,
  - log/cache files,
  - or duplicates.
- **Live per-category summary** while the analyzer runs, showing counts for old files, duplicates, and log/cache files.
- **Interactive analyzer filtering** by category using the keyboard (Tab/arrow keys, `0` to clear) or by clicking the summary line.
- **Stacked bar summary** for analyzer hints, with proportional old / duplicate / log/cache segments colored by category (and clickable in the TUI).
- **Docker disk usage** view with per-resource breakdown (images, containers, volumes, build cache) and a prune wrapper (`p`).
- **Persistent marks** across directory navigation so batch trash/move operations work on items selected in multiple folders.
- **Directory label** in the file list Category column so folders are clearly shown as `Directory`.
- Mouse support in the TUI for analyzer category filtering.
- Unit tests for the custom `boolFlag` type used by the `-ignore-hidden` flag.

### Changed

- Analyzer progress reporting is now throttled and cancellable mid-run.
- Summary categories use a shared helper for both rendering and click-hit testing.

### Fixed

- Deadlock in the scanner when walking large directories.
- Saved rules now resolve symlinks before checking protected paths, preventing a rule pointing at a symlink to `/` or system directories from bypassing protection.
- Saved rules default to `dup-mode: smart` when created via the CLI, matching the analyzer and executor defaults.
- Analyzer hit-test now covers the rendered bar chart, not just the category text.
- The `-ignore-hidden` flag now honors the `ignore_hidden` value from the config file and can be overridden with `-ignore-hidden=false`.
- `launchctl bootstrap` no longer fails with "Input/output error" when re-installing an existing schedule (added `bootout` before `bootstrap`).
- Generated launchd plists now correctly escape dynamic values (`&`, `<`, `>`, `"`, `'`) to produce valid XML.
- CLI subcommands now accept flags after positional arguments (e.g., `schedule install e2e-test-cache --on-login` works, not just `--on-login e2e-test-cache`).

### Documentation

- Refreshed `README.md` screenshots and demo section with current TUI mockups and asciinema/vhs recording instructions.
- Updated the roadmap to split features into Done and Coming soon.
- Refreshed the Features list to match the roadmap.
- Reorganized the Key bindings table by view to match the current TUI help bars.
- Added a **Configuration file** reference section to `README.md` documenting `~/.config/cleanup-tool/config.json` and all supported fields.

## [0.1.0] - 2026-07-25

### Added

- Initial `cleanup-tool` release.
- Parallel directory scanner with real-time progress in the TUI.
- Categorization of common space hogs (Docker, LLM models, build artifacts, dependencies, media, archives, logs, caches, downloads, documents, applications, git repos).
- Bubble Tea-based TUI with size-sorted file navigation, directory drill-down, marking, trash, move to external drive, and restore.
- Configurable ignore paths via `~/.config/cleanup-tool/config.json`.
