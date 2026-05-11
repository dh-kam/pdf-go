#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
DIST_DIR="${2:-dist}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: package_release_assets.sh <version> [dist-dir]" >&2
  exit 2
fi

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"
DIST_DIR="$(cd "${DIST_DIR}" && pwd)"

shopt -s nullglob
release_dirs=(build/*/release)

if [[ "${#release_dirs[@]}" -eq 0 ]]; then
  echo "no release build directories found under build/*/release" >&2
  exit 3
fi

for release_dir in "${release_dirs[@]}"; do
  platform="$(basename "$(dirname "${release_dir}")")"
  asset_base="pdf-go_${VERSION}_${platform}"

  if [[ "${platform}" == windows-* ]]; then
    if command -v zip >/dev/null 2>&1; then
      (
        cd "${release_dir}"
        zip -q -r "${DIST_DIR}/${asset_base}.zip" .
      )
    else
      tar -C "${release_dir}" -czf "${DIST_DIR}/${asset_base}.tar.gz" .
    fi
  else
    tar -C "${release_dir}" -czf "${DIST_DIR}/${asset_base}.tar.gz" .
  fi
done

(
  cd "${DIST_DIR}"
  sha256sum * > checksums.txt
)

echo "Packaged release assets:"
find "${DIST_DIR}" -maxdepth 1 -type f -print | sort
