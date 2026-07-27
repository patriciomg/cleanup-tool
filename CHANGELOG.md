# Changelog

All notable changes to `cleanup-tool` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Duplicate file detection** with four configurable hash modes: `first10mb`, `sample`, `full`, and `smart` (size → sample → full).
- **Deletability analyzer** that flags files as:
  - older than one year,
  - log/cache files,
  - or duplicates.
- **Live per-category summary** while the analyzer runs, showing counts for old files, duplicates, and log/cache files.
- **Interactive analyzer filtering** by category using the keyboard (Tab/arrow keys, `0` to clear) or by clicking the summary line.
- **Visual bar charts / sparklines** next to the analyzer summary counts.
- **Docker disk usage** view with per-resource breakdown (images, containers, volumes, build cache) and a prune wrapper (`p`).
- **Persistent marks** across directory navigation so batch trash/move operations work on items selected in multiple folders.
- **Directory label** in the file list Category column so folders are clearly shown as `Directory`.
- Mouse support in the TUI for analyzer category filtering.

### Changed

- Analyzer progress reporting is now throttled and cancellable mid-run.
- Summary categories use a shared helper for both rendering and click-hit testing.

### Fixed

- Deadlock in the scanner when walking large directories.
- Analyzer hit-test now covers the rendered bar chart, not just the category text.

### Documentation

- Refreshed `README.md` screenshots and demo section with current TUI mockups and asciinema/vhs recording instructions.
- Updated the roadmap to split features into Done and Coming soon.
- Refreshed the Features list to match the roadmap.
- Reorganized the Key bindings table by view to match the current TUI help bars.

## [0.1.0] - 2026-07-25

### Added

- Initial `cleanup-tool` release.
- Parallel directory scanner with real-time progress in the TUI.
- Categorization of common space hogs (Docker, LLM models, build artifacts, dependencies, media, archives, logs, caches, downloads, documents, applications, git repos).
- Bubble Tea-based TUI with size-sorted file navigation, directory drill-down, marking, trash, move to external drive, and restore.
- Configurable ignore paths via `~/.config/cleanup-tool/config.json`.
