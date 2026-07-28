# Changelog

All notable changes to `cleanup-tool` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Alternative `dua`-style interactive TUI (`-tui-style dua`) with a flat, size-sorted list, percentage/bar columns, and `dua i`-like keyboard navigation.
- Analyzer support in dua mode: press `a` to analyze the current directory or `A` to analyze selected/marked items, with the same live progress, per-category summary, stacked bar, and filtering as the tree view.
- Docker disk usage/prune support in dua mode: press `D` to view images, containers, volumes, and build cache, and `p` to prune the selected resource.
- External move and restore support in dua mode: press `m` to move marked/selected items to the `-external` drive, and `r` to restore a selected item from Trash.
- Full dependency wiring for dua mode: `-external`, `-dup-mode`, and Docker are now passed through to the dua TUI.
- `-out` flag as a format-agnostic alias for writing scan exports (JSON, CSV, TSV, YAML) to a file.
- `-stdout` flag as a clearer alias for `-json` that exports scan results to stdout with any `-format`.
- Format auto-detection from the `-out` file extension: `.json`, `.csv`, `.tsv`, `.yaml`/`.yml`.
- Test coverage for `maybeWarnDeprecatedJSONOut` and `formatFromExtension`/`resolveFormat`.
- Integration tests verifying `-out` auto-detects CSV/YAML, explicit `-format` overrides the extension, and `-stdout` works with JSON/CSV/YAML.

### Changed

- The default interactive TUI style is now `dua`. The previous default (`tree`) is now named `terminal`, with `tree` kept as an alias.
- README key bindings and CLI flag docs now refer to the `terminal` style and note that `dua` is the default.
- The terminal/tree-style TUI implementation was moved from `internal/tui` to `internal/tui/terminal` (package `terminal`) to mirror the `internal/tui/dua` layout.

### Deprecated

- `-json-out` is now deprecated in favor of `-out`. A warning is printed to stderr when `-json-out` is used.

## [0.2.0] - 2026-07-27

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

[Unreleased]: https://github.com/patriciomg/cleanup-tool/compare/v0.2.0...main
[0.2.0]: https://github.com/patriciomg/cleanup-tool/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/patriciomg/cleanup-tool/releases/tag/v0.1.0
