#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${1:-$(pwd)}"
VERSION="${2:-}"
RELEASE_GATE="${RELEASE_GATE:-release-ci}"
RELEASE_TAG_REGEX='^v[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z][0-9A-Za-z-]*-[0-9]{6}\.[1-9][0-9]*$'

if [[ -z "${VERSION}" ]]; then
  echo "usage: release_preflight.sh <repo-root> <version>" >&2
  exit 2
fi

cd "${ROOT_DIR}"

echo "[preflight] root=${ROOT_DIR}"
echo "[preflight] version=${VERSION}"

if ! [[ "${VERSION}" =~ ${RELEASE_TAG_REGEX} ]]; then
  echo "[preflight][fail] invalid release tag: ${VERSION}" >&2
  echo "[preflight][fail] expected: v<project-semver>-<upstream-slug>-YYYYMM.seq" >&2
  exit 9
fi

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

echo "[preflight] checking release gate: ${RELEASE_GATE}"
make "${RELEASE_GATE}"

echo "[preflight] ok"
