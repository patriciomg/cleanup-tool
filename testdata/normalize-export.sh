#!/bin/bash
# Normalize an export file so it can be compared against a stored snapshot.
# Usage: normalize-export.sh <format> <file>
# Supported formats: json, csv, tsv, yaml
set -e

format="$1"
file="$2"

case "$format" in
  json|yaml)
    sed -E \
      -e 's|/tmp/cleanup-export-sample-fmt|<SAMPLE_DIR>|g' \
      -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})?/<TIMESTAMP>/g' \
      -e 's/"mode": 2147484141/"mode": <DIR_MODE>/g' \
      -e 's/"mode": 420/"mode": <FILE_MODE>/g' \
      -e 's/mode: 2147484141/mode: <DIR_MODE>/g' \
      -e 's/mode: 420/mode: <FILE_MODE>/g' \
      "$file"
    ;;
  csv|tsv)
    sed -E \
      -e 's|/tmp/cleanup-export-sample-fmt|<SAMPLE_DIR>|g' \
      -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}/<TIMESTAMP>/g' \
      -e 's|drwxr-xr-x|<DIR_MODE>|g' \
      -e 's|-rw-r--r--|<FILE_MODE>|g' \
      "$file"
    ;;
  *)
    echo "unknown format: $format" >&2
    exit 1
    ;;
esac
