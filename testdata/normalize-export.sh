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
      -e 's/"mode": [0-9]+/"mode": <MODE>/g' \
      -e 's/mode: [0-9]+/mode: <MODE>/g' \
      "$file"
    ;;
  csv)
    tmp=$(mktemp)
    sed -E \
      -e 's|/tmp/cleanup-export-sample-fmt|<SAMPLE_DIR>|g' \
      -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}/<TIMESTAMP>/g' \
      "$file" > "$tmp"
    awk 'BEGIN{FS=OFS=","} NR==1{print; next} {if ($7 ~ /^d/){$7="<DIR_MODE>"} else if ($7 ~ /^-/){$7="<FILE_MODE>"}; print}' "$tmp"
    rm -f "$tmp"
    ;;
  tsv)
    tmp=$(mktemp)
    sed -E \
      -e 's|/tmp/cleanup-export-sample-fmt|<SAMPLE_DIR>|g' \
      -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}/<TIMESTAMP>/g' \
      "$file" > "$tmp"
    awk 'BEGIN{FS=OFS="\t"} NR==1{print; next} {if ($7 ~ /^d/){$7="<DIR_MODE>"} else if ($7 ~ /^-/){$7="<FILE_MODE>"}; print}' "$tmp"
    rm -f "$tmp"
    ;;
  *)
    echo "unknown format: $format" >&2
    exit 1
    ;;
esac
