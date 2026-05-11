#!/usr/bin/env bash
set -euo pipefail

BUMP="${1:-patch}"
EXPLICIT_VERSION="${2:-}"
SEMVER_TAG_REGEX='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

if [[ -n "${EXPLICIT_VERSION}" ]]; then
  if ! [[ "${EXPLICIT_VERSION}" =~ ${SEMVER_TAG_REGEX} ]]; then
    echo "invalid version tag: ${EXPLICIT_VERSION}" >&2
    exit 2
  fi
  printf '%s\n' "${EXPLICIT_VERSION}"
  exit 0
fi

case "${BUMP}" in
  major | minor | patch)
    ;;
  *)
    echo "invalid bump component: ${BUMP}" >&2
    exit 3
    ;;
esac

latest_tag="$(
  git tag --list 'v[0-9]*' --sort=-v:refname |
    grep -E "${SEMVER_TAG_REGEX}" |
    head -n 1 || true
)"

if [[ -z "${latest_tag}" ]]; then
  latest_tag="v0.0.0"
fi

base="${latest_tag%%-*}"
base="${base#v}"
IFS='.' read -r major minor patch <<< "${base}"

case "${BUMP}" in
  major)
    major=$((major + 1))
    minor=0
    patch=0
    ;;
  minor)
    minor=$((minor + 1))
    patch=0
    ;;
  patch)
    patch=$((patch + 1))
    ;;
esac

printf 'v%d.%d.%d\n' "${major}" "${minor}" "${patch}"
