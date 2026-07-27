---
name: Remove deprecated `-json-out` flag
about: Track the removal of the deprecated `-json-out` CLI flag in a future release
title: "[v0.3.0] Remove deprecated `-json-out` flag"
labels: breaking-change, deprecation, v0.3.0
---

## Summary

The `-json-out` flag was deprecated in favor of the format-agnostic `-out` flag. It currently still works for backward compatibility but prints a warning to stderr.

This issue tracks its complete removal in the next breaking/major release.

## Background

- `-json-out` was the original way to write JSON scan results to a file.
- `-out` was introduced as a replacement that works with any `-format` (json, csv, tsv, yaml).
- The `-json-out` flag is currently preserved as a legacy alias that maps to `-out` and emits:
  ```
  warning: -json-out is deprecated; use -out instead
  ```

## Acceptance Criteria

- [ ] Remove the `-json-out` flag definition from `cmd/cleanup-tool/main.go`.
- [ ] Remove the `maybeWarnDeprecatedJSONOut` helper and any related warning logic.
- [ ] Remove the legacy handling in `effectiveOutputFile` that maps `-json-out` to `-out`.
- [ ] Remove or update tests that exercise `-json-out` (e.g., `TestCLIDeprecatedJSONOut`).
- [ ] Update the CLI flags table in `README.md` to remove the `-json-out` row.
- [ ] Add a prominent breaking-change note to `CHANGELOG.md` under the new version.
- [ ] Update any other documentation that mentions `-json-out`.

## Migration Path

Before removing, users should replace:

```bash
cleanup-tool -json-out /path/to/scan.json -paths /tmp
```

with:

```bash
cleanup-tool -out /path/to/scan.json -paths /tmp
```

For other formats:

```bash
cleanup-tool -out /path/to/scan.csv -format csv -paths /tmp
```

## Target Release

v0.3.0 (or next appropriate major/breaking release).

## Related

- PR/commit that introduced the deprecation warning and `-out` flag.
