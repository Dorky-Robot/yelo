#!/usr/bin/env bash
#
# Release script for yelo
#
# Usage: scripts/release.sh [patch|minor|major|X.Y.Z]
#
# Steps:
#   1. Pre-flight checks (branch, clean tree, pull)
#   2. Bump version in Cargo.toml
#   3. Commit, tag, push
#   4. Wait for CI release workflow
#   5. Update local Formula/yelo.rb with SHA256s
#   6. Commit and push formula update
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

BUMP="${1:-}"

# --- colors ---
red()   { printf '\033[1;31m%s\033[0m\n' "$*"; }
green() { printf '\033[1;32m%s\033[0m\n' "$*"; }
blue()  { printf '\033[1;34m%s\033[0m\n' "$*"; }
dim()   { printf '\033[2m%s\033[0m\n' "$*"; }

die() { red "error: $*" >&2; exit 1; }

# --- pre-flight ---
blue "==> Pre-flight checks"

BRANCH="$(git branch --show-current)"
[ "$BRANCH" = "main" ] || die "not on main (on $BRANCH)"

[ -z "$(git status --porcelain)" ] || die "working tree is dirty"

dim "    pulling latest..."
git pull --quiet origin main

CURRENT="$(grep '^version' Cargo.toml | head -1 | sed 's/.*"\(.*\)"/\1/')"
echo "    current version: $CURRENT"

# --- determine new version ---
bump_version() {
  local cur="$1" type="$2"
  local major minor patch
  IFS='.' read -r major minor patch <<< "$cur"
  case "$type" in
    patch) echo "$major.$minor.$((patch + 1))" ;;
    minor) echo "$major.$((minor + 1)).0" ;;
    major) echo "$((major + 1)).0.0" ;;
    *)     die "unknown bump type: $type" ;;
  esac
}

if [ -z "$BUMP" ]; then
  echo ""
  echo "  bump type:"
  echo "    patch — bug fixes, docs, small tweaks"
  echo "    minor — new features, backward-compatible"
  echo "    major — breaking changes"
  echo ""
  read -rp "  bump [patch/minor/major/X.Y.Z]: " BUMP
  [ -n "$BUMP" ] || BUMP="patch"
fi

if [[ "$BUMP" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  NEW_VERSION="$BUMP"
else
  NEW_VERSION="$(bump_version "$CURRENT" "$BUMP")"
fi

echo "    new version: $NEW_VERSION"

# confirm
read -rp "  release v${NEW_VERSION}? [y/N] " CONFIRM
[[ "$CONFIRM" =~ ^[Yy]$ ]] || { echo "aborted."; exit 0; }

# --- check tag ---
if git tag -l "v$NEW_VERSION" | grep -q .; then
  die "tag v$NEW_VERSION already exists"
fi

# --- bump Cargo.toml ---
blue "==> Bumping version to $NEW_VERSION"
sed -i.bak "s/^version = \"$CURRENT\"/version = \"$NEW_VERSION\"/" Cargo.toml
rm -f Cargo.toml.bak

# verify it compiled
dim "    verifying build..."
cargo check --quiet 2>/dev/null || die "cargo check failed after version bump"

# --- commit, tag, push ---
blue "==> Committing and tagging"
git add Cargo.toml
git commit --quiet -m "release: v${NEW_VERSION}"
git tag "v${NEW_VERSION}"

blue "==> Pushing to origin"
git push --quiet origin main --tags

# --- wait for CI ---
blue "==> Waiting for release workflow"
dim "    this builds binaries for 4 platforms — usually takes 5-10 minutes"

sleep 5  # give GitHub a moment to pick up the tag

while true; do
  RUN_JSON="$(gh run list --workflow=release.yml --limit=1 --json status,conclusion,databaseId 2>/dev/null || echo '[]')"
  STATUS="$(echo "$RUN_JSON" | jq -r '.[0].status // "unknown"')"
  CONCLUSION="$(echo "$RUN_JSON" | jq -r '.[0].conclusion // "null"')"
  RUN_ID="$(echo "$RUN_JSON" | jq -r '.[0].databaseId // "0"')"

  if [ "$STATUS" = "completed" ]; then
    if [ "$CONCLUSION" = "success" ]; then
      green "    CI passed (run $RUN_ID)"
      break
    else
      red "    CI failed: $CONCLUSION"
      echo "    https://github.com/Dorky-Robot/yelo/actions/runs/$RUN_ID"
      die "release workflow failed"
    fi
  fi

  printf "    status: %s (waiting 30s...)\r" "$STATUS"
  sleep 30
done

# --- update local formula ---
blue "==> Updating local Formula/yelo.rb"

TARGETS=(
  "aarch64-apple-darwin"
  "x86_64-apple-darwin"
  "x86_64-unknown-linux-gnu"
)

declare -A SHAS
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

for TARGET in "${TARGETS[@]}"; do
  FILE="$TMPDIR/yelo-${TARGET}.tar.gz"
  dim "    downloading yelo-${TARGET}.tar.gz..."
  curl -sL --retry 5 --retry-delay 5 --retry-all-errors -f \
    "https://github.com/Dorky-Robot/yelo/releases/download/v${NEW_VERSION}/yelo-${TARGET}.tar.gz" \
    -o "$FILE"
  SHA="$(sha256sum "$FILE" | cut -d' ' -f1)"
  SHAS["$TARGET"]="$SHA"
  dim "    $TARGET: $SHA"
done

# rewrite formula
TAG="v${NEW_VERSION}"
BASE="https://github.com/Dorky-Robot/yelo/releases/download/${TAG}"

cat > Formula/yelo.rb << FORMULA
class Yelo < Formula
  desc "FTP-style CLI for Amazon S3 and Glacier"
  homepage "https://github.com/Dorky-Robot/yelo"
  version "${NEW_VERSION}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "${BASE}/yelo-aarch64-apple-darwin.tar.gz"
      sha256 "${SHAS[aarch64-apple-darwin]}"
    else
      url "${BASE}/yelo-x86_64-apple-darwin.tar.gz"
      sha256 "${SHAS[x86_64-apple-darwin]}"
    end
  end

  on_linux do
    url "${BASE}/yelo-x86_64-unknown-linux-gnu.tar.gz"
    sha256 "${SHAS[x86_64-unknown-linux-gnu]}"
  end

  def install
    bin.install "yelo"
  end

  test do
    assert_match "yelo #{version}", shell_output("#{bin}/yelo --version")
  end
end
FORMULA

git add Formula/yelo.rb
git commit --quiet -m "formula: update to v${NEW_VERSION}"
git push --quiet origin main

# --- done ---
echo ""
green "==> Released yelo v${NEW_VERSION}"
echo "    tag:     v${NEW_VERSION}"
echo "    release: https://github.com/Dorky-Robot/yelo/releases/tag/v${NEW_VERSION}"
echo "    formula: updated"
echo ""
echo "  To install/upgrade via Homebrew:"
dim "    brew update && brew upgrade dorky-robot/tap/yelo"
