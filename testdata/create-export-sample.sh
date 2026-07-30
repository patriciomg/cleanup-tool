#!/bin/bash
set -e

DIR="${1:-/tmp/cleanup-export-sample-fmt}"
rm -rf "$DIR"
mkdir -p "$DIR/subdir"

# Fixed content so sizes are deterministic.
: > "$DIR/archive.tar"
printf 'hello world\n' > "$DIR/readme.txt"
printf 'binary data\n' > "$DIR/subdir/data.bin"

# Fixed modes so output is the same across platforms/umasks.
chmod 755 "$DIR" "$DIR/subdir"
chmod 644 "$DIR/archive.tar" "$DIR/readme.txt" "$DIR/subdir/data.bin"
