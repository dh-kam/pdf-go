#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${1:-$(pwd)}"
VERSION="${2:-}"
DRY_RUN="${DRY_RUN:-false}"
REPO_MODULE="${REPO_MODULE:-github.com/dh-kam/pdf-go/pkg/pdf}"

if [[ -z "${VERSION}" ]]; then
  echo "usage: release_publish.sh <repo-root> <version>" >&2
  exit 2
fi

cd "${ROOT_DIR}"

run_cmd() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    echo "[dry-run] $*"
    return 0
  fi
  echo "[run] $*"
  "$@"
}

echo "[release] root=${ROOT_DIR}"
echo "[release] version=${VERSION}"
echo "[release] dry_run=${DRY_RUN}"
echo "[release] module=${REPO_MODULE}"

"${ROOT_DIR}/scripts/release_preflight.sh" "${ROOT_DIR}" "${VERSION}"

run_cmd git tag -a "${VERSION}" -m "release: ${VERSION}"
run_cmd git push origin "${VERSION}"

if command -v gh >/dev/null 2>&1; then
  run_cmd gh release create "${VERSION}" \
    --verify-tag \
    --title "${VERSION}" \
    --notes "Automated release for ${VERSION}"
else
  echo "[release][warn] gh cli not found; skip GitHub release creation"
fi

run_cmd env GOPROXY=https://proxy.golang.org go list -m "${REPO_MODULE}@${VERSION}"

echo "[release] done"
