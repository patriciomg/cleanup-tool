#!/bin/bash
# Creates a deterministic sample directory for the benchmark-format CI check.
# The sample is intentionally tiny so the benchmark runs quickly and is stable
# across CI runners.
set -e

SAMPLE_DIR="${1:-/tmp/cleanup-benchmark-sample}"

rm -rf "$SAMPLE_DIR"
mkdir -p "$SAMPLE_DIR/subdir"
printf 'hello\n' > "$SAMPLE_DIR/file1.txt"
printf 'world\n' > "$SAMPLE_DIR/subdir/file2.txt"
