#!/usr/bin/env bash
set -euo pipefail

BUMP="${1:-sequence}"
EXPLICIT_VERSION="${2:-}"
UPSTREAM_VERSION="${3:-${RELEASE_UPSTREAM_VERSION:-}}"
RELEASE_MONTH="${4:-${RELEASE_MONTH:-$(date -u +%Y%m)}}"
PROJECT_VERSION="${PROJECT_VERSION:-0.9.0}"

PROJECT_VERSION_REGEX='^[0-9]+\.[0-9]+\.[0-9]+$'
RELEASE_MONTH_REGEX='^[0-9]{6}$'
RELEASE_TAG_REGEX='^v[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z][0-9A-Za-z-]*-[0-9]{6}\.[1-9][0-9]*$'

sanitize_upstream() {
  local raw="${1}"
  local slug

  slug="$(
    printf '%s' "${raw}" |
      tr '[:upper:]' '[:lower:]' |
      sed -E 's/[^0-9a-z]+/-/g; s/^-+//; s/-+$//'
  )"

  if [[ -z "${slug}" ]]; then
    echo "invalid upstream version: ${raw}" >&2
    exit 4
  fi

  printf '%s\n' "${slug}"
}

if [[ -n "${EXPLICIT_VERSION}" ]]; then
  if ! [[ "${EXPLICIT_VERSION}" =~ ${RELEASE_TAG_REGEX} ]]; then
    echo "invalid release tag: ${EXPLICIT_VERSION}" >&2
    echo "expected: v<project-semver>-<upstream-slug>-YYYYMM.seq, for example v0.9.0-poppler24-02-0-202605.1" >&2
    exit 2
  fi
  printf '%s\n' "${EXPLICIT_VERSION}"
  exit 0
fi

case "${BUMP}" in
  sequence | seq | patch)
    ;;
  *)
    echo "invalid bump component: ${BUMP}; only sequence is supported for release-train tags" >&2
    exit 3
    ;;
esac

if ! [[ "${PROJECT_VERSION}" =~ ${PROJECT_VERSION_REGEX} ]]; then
  echo "invalid PROJECT_VERSION: ${PROJECT_VERSION}" >&2
  exit 5
fi

if ! [[ "${RELEASE_MONTH}" =~ ${RELEASE_MONTH_REGEX} ]]; then
  echo "invalid RELEASE_MONTH: ${RELEASE_MONTH}; expected YYYYMM" >&2
  exit 6
fi

if [[ -z "${UPSTREAM_VERSION}" ]]; then
  if [[ -f test/testdata/splash_golden/POPPLER_VERSION ]]; then
    UPSTREAM_VERSION="poppler$(tr -d '[:space:]' < test/testdata/splash_golden/POPPLER_VERSION)"
  else
    echo "RELEASE_UPSTREAM_VERSION is required when Poppler golden version is unavailable" >&2
    exit 7
  fi
fi

UPSTREAM_SLUG="$(sanitize_upstream "${UPSTREAM_VERSION}")"
TAG_PREFIX="v${PROJECT_VERSION}-${UPSTREAM_SLUG}-${RELEASE_MONTH}."
tag_regex="^v${PROJECT_VERSION//./\\.}-${UPSTREAM_SLUG}-${RELEASE_MONTH}\\.([0-9]+)$"
max_seq=0

while IFS= read -r tag; do
  if [[ "${tag}" =~ ${tag_regex} ]]; then
    seq="${BASH_REMATCH[1]}"
    if (( seq > max_seq )); then
      max_seq="${seq}"
    fi
  fi
done < <(git tag --list "${TAG_PREFIX}*" || true)

printf '%s%d\n' "${TAG_PREFIX}" "$((max_seq + 1))"
