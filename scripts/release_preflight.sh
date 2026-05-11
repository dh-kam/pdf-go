#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${1:-$(pwd)}"
VERSION="${2:-}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: release_preflight.sh <repo-root> <version>" >&2
  exit 2
fi

cd "${ROOT_DIR}"

echo "[preflight] root=${ROOT_DIR}"
echo "[preflight] version=${VERSION}"

if [[ ! -d ".git" ]]; then
  echo "[preflight][fail] .git directory not found in ${ROOT_DIR}" >&2
  exit 10
fi

if ! command -v git >/dev/null 2>&1; then
  echo "[preflight][fail] git command not found" >&2
  exit 11
fi

if ! command -v go >/dev/null 2>&1; then
  echo "[preflight][fail] go command not found" >&2
  exit 12
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "[preflight][fail] current directory is not a git work tree" >&2
  exit 13
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "[preflight][fail] git worktree is dirty; commit or stash changes first" >&2
  git status --short || true
  exit 14
fi

if ! git remote get-url origin >/dev/null 2>&1; then
  echo "[preflight][fail] git remote 'origin' is missing" >&2
  exit 15
fi

if git rev-parse --verify --quiet "refs/tags/${VERSION}" >/dev/null; then
  echo "[preflight][fail] tag already exists: ${VERSION}" >&2
  exit 16
fi

echo "[preflight] checking release gate..."
make porting-complete-plus-goal98

if command -v gh >/dev/null 2>&1; then
  echo "[preflight] gh cli detected"
  if ! gh auth status >/dev/null 2>&1; then
    echo "[preflight][fail] gh auth status failed; run 'gh auth login'" >&2
    exit 17
  fi
else
  echo "[preflight][warn] gh cli not found; GitHub release step will require manual execution"
fi

echo "[preflight] ok"
