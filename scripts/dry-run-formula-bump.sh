#!/usr/bin/env bash
set -euo pipefail

# Dry-run the release formula auto-bump step.
#
# Builds a local release tarball, updates Formula/cleanup-tool.rb with a local
# file:// URL and the computed SHA-256, runs `brew audit --new`, and restores
# the original formula afterwards.

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FORMULA="${REPO_ROOT}/Formula/cleanup-tool.rb"
VERSION="${1:-v0.0.0-dry-run}"
BACKUP_DIR="$(mktemp -d /tmp/cleanup-tool-formula-backup.XXXXXX)"
BACKUP="${BACKUP_DIR}/cleanup-tool.rb"
TAP_DIR="$(mktemp -d /tmp/homebrew-cleanup-tool.XXXXXX)"

cleanup() {
  echo "Cleaning up..."
  if [[ -s "$BACKUP" ]]; then
    cp "$BACKUP" "$FORMULA"
    echo "Restored original formula."
  fi
  rm -rf "$TAP_DIR" "$BACKUP_DIR"
}
trap cleanup EXIT

if [[ ! -f "$FORMULA" ]]; then
  echo "Formula not found: $FORMULA" >&2
  exit 1
fi

echo "==> Backing up current formula"
cp "$FORMULA" "$BACKUP"

echo "==> Building release tarball (VERSION=$VERSION)"
cd "$REPO_ROOT"
make release-clean
VERSION="$VERSION" make release

tarball="${REPO_ROOT}/dist/cleanup-tool-${VERSION}-darwin-universal.tar.gz"
if [[ ! -f "$tarball" ]]; then
  echo "Tarball not found: $tarball" >&2
  exit 1
fi

echo "==> Computing SHA-256"
sha256="$(shasum -a 256 "$tarball" | awk '{print $1}')"
echo "SHA-256: $sha256"

echo "==> Updating formula with local file:// URL"
file_url="file://${tarball}"
sed -i.bak "s|url \"[^\"]*\"|url \"$file_url\"|" "$FORMULA"
sed -i.bak "s/sha256 \"[^\"]*\"/sha256 \"$sha256\"/" "$FORMULA"
rm -f "${FORMULA}.bak"

if ! grep -qF "$file_url" "$FORMULA"; then
  echo "ERROR: failed to update formula URL" >&2
  exit 1
fi
if ! grep -qF "$sha256" "$FORMULA"; then
  echo "ERROR: failed to update formula SHA-256" >&2
  exit 1
fi

echo "==> Setting up temporary Homebrew tap"
mkdir -p "${TAP_DIR}/Formula"
cp "$FORMULA" "${TAP_DIR}/Formula/cleanup-tool.rb"
cd "$TAP_DIR"
git init -q
# GitHub Actions runners do not have a default git user, so set one.
git config user.name "github-actions[bot]" 2>/dev/null || true
git config user.email "github-actions[bot]@users.noreply.github.com" 2>/dev/null || true
git add Formula/cleanup-tool.rb
git commit -q -m "cleanup-tool formula"

echo "==> Running brew audit --new patriciomg/cleanup-tool/cleanup-tool"
brew untap patriciomg/cleanup-tool 2>/dev/null || true
brew tap patriciomg/cleanup-tool "$TAP_DIR"
brew audit --new patriciomg/cleanup-tool/cleanup-tool

echo ""
echo "Dry-run passed. The formula auto-bump logic and brew audit are working."

echo ""
echo "Tarball path: $tarball"
echo "Cleaning up temporary release artifacts..."
cd "$REPO_ROOT"
make release-clean
