# Contributing to cleanup-tool

Thanks for taking the time to contribute! This document covers the workflow and conventions we follow.

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Development setup](#development-setup)
- [Building and testing](#building-and-testing)
- [Project conventions](#project-conventions)
- [Pull request workflow](#pull-request-workflow)
- [Snapshot-based CI checks](#snapshot-based-ci-checks)
- [Release process](#release-process)
- [Getting help](#getting-help)

## Code of conduct

Be respectful, inclusive, and constructive. We welcome contributors of all experience levels. If you are unsure about a change, open an issue first so we can agree on direction.

A dedicated Code of Conduct is available in [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). All contributors and participants are expected to follow it.

## Development setup

Requirements:

- **Go 1.22+** (see [`go.mod`](go.mod) for the exact version)
- **macOS** for full functionality (the tool is developed and tested on macOS; Linux builds may work but are not officially supported)
- **Make** (optional, for the convenience targets in [`Makefile`](Makefile))
- **rsync** (used by the move-to-external action; macOS ships with it)

Clone the repository and build the binary:

```bash
git clone https://github.com/patriciomg/cleanup-tool.git
cd cleanup-tool
go build ./cmd/cleanup-tool
```

For automatic rebuilds on file changes, install [`reflex`](https://github.com/cespare/reflex):

```bash
go install github.com/cespare/reflex@latest
make watch      # rebuild on every Go file change
```

## Building and testing

### Standard commands

```bash
# Build the binary
go build ./cmd/cleanup-tool

# Run all tests
go test ./...

# Run go vet
go vet ./...

# Build release artifacts (macOS universal binary, tarball, checksums)
make release
```

### Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Build the binary |
| `make test` | Run all tests |
| `make vet` | Run `go vet ./...` |
| `make release` | Build the macOS universal binary and tarball |
| `make watch` | Auto-rebuild on Go file changes |
| `make watch-test` | Auto-run tests on Go file changes |

### Running the tool locally

```bash
# Scan home directory with the default dua-style TUI
./cleanup-tool

# Benchmark scan performance
./cleanup-tool -benchmark -paths /tmp

# Export scan results to stdout
./cleanup-tool -format json -stdout -paths /tmp
```

## Project conventions

### Go style

- Follow standard Go formatting (`gofmt`).
- Keep exported symbols documented with a short Go doc comment.
- Prefer small, focused functions and packages.
- Reuse helpers and components from existing packages rather than reimplementing them.
- Avoid adding dependencies unless they are already used in the project or clearly justified.

### Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

<body>
```

Common types: `feat`, `fix`, `docs`, `ci`, `test`, `refactor`, `chore`.

Examples:

```
feat(analyzer): add smart duplicate-detection mode
docs(readme): add benchmark before/after example
ci: add export-format snapshot job
```

### Package layout

| Package | Purpose |
|---------|---------|
| `cmd/cleanup-tool` | CLI entry point and subcommands |
| `internal/analyzer` | Scanner, deletability analyzer, and exporters |
| `internal/categories` | File category classification |
| `internal/config` | Config file loading and defaults |
| `internal/deps` | Dependency directory finder |
| `internal/docker` | Docker disk usage client |
| `internal/llm` | LLM registry helpers |
| `internal/rules` | Saved rules storage and execution |
| `internal/tui` | Bubble Tea TUI components |
| `internal/actions` | File actions: trash, move, restore |
| `internal/undo` | Persistent undo operations |

## Pull request workflow

1. **Open an issue first** for major changes so we can agree on direction.
2. **Fork the repository** and create a feature branch from `main`.
3. **Make your changes** following the conventions above.
4. **Run tests and vet** before pushing:

   ```bash
   go test ./...
   go vet ./...
   ```

5. **Update documentation** if your change affects CLI flags, config, output formats, or CI checks.
6. **Open a pull request** with a clear description and link to the issue.
7. **Wait for CI** to pass. PRs cannot be merged with failing checks.

## Snapshot-based CI checks

We use snapshot tests to detect unintended changes to stable CLI outputs. If you change any of these outputs, you must update the corresponding snapshots.

### `test-benchmark-format`

Builds the binary, creates a deterministic sample, runs `-benchmark`, normalizes variable parts, and compares against [`testdata/benchmark-snapshot.txt`](testdata/benchmark-snapshot.txt).

Regenerate the snapshot:

```bash
go build -o cleanup-tool ./cmd/cleanup-tool
./testdata/create-benchmark-sample.sh
./cleanup-tool -benchmark -paths /tmp/cleanup-benchmark-sample | \
  sed -E 's/Total time: .*/Total time: <TIME>/; s/Avg throughput: .*/Avg throughput: <THROUGHPUT>/' \
  > testdata/benchmark-snapshot.txt
```

### `test-help-format`

Builds the binary, runs `-h`, normalizes the binary path, and compares against [`testdata/help-snapshot.txt`](testdata/help-snapshot.txt).

Regenerate the snapshot:

```bash
go build -o cleanup-tool ./cmd/cleanup-tool
./cleanup-tool -h 2>&1 | sed 's/^Usage of .*:$/Usage of <binary>:/' > testdata/help-snapshot.txt
```

The snapshot is the canonical reference for the help text; update the CLI flags table in `README.md` to match it.

### `test-export-format`

Builds the binary, creates a deterministic sample, exports it as `json`, `csv`, `tsv`, and `yaml`, normalizes the outputs, and compares against the snapshots in [`testdata/export-snapshots/`](testdata/export-snapshots/).

Regenerate the snapshots:

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

## Release process

Releases are handled automatically by the [`release.yml`](.github/workflows/release.yml) workflow when a `v*` tag is pushed. For the full checklist, including GPG setup, see [`docs/releasing.md`](docs/releasing.md).

Do not commit release binaries or tags manually unless you have configured GPG signing and understand the release checklist.

## Getting help

- Open an issue for bugs, feature requests, or questions. Use the appropriate template when possible:
  - **Bug report**: for crashes, incorrect behavior, or other problems.
  - **Feature request**: for enhancements or new capabilities.
- Check the [README](README.md) for usage and configuration docs.
- Review the [release notes](https://github.com/patriciomg/cleanup-tool/releases) for recent changes.

Thank you for contributing!
