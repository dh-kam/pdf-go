#!/usr/bin/env bash
set -euo pipefail

bad=0
count=0

printf 'CGo inventory\n'
printf 'Expected CGo files: 0\n\n'
printf '%-62s %-34s %s\n' 'file' 'go_build' 'cgo_directive'

while IFS= read -r file; do
	if ! grep -q 'import "C"' "$file"; then
		continue
	fi

	rel="${file#./}"
	count=$((count + 1))
	build_expr="$(awk '/^\/\/go:build / {sub(/^\/\/go:build /, ""); print; exit}' "$file")"
	cgo_directive="$(grep -m 1 -E '^[[:space:]]*#cgo[[:space:]]' "$file" | sed 's/^[[:space:]]*//' || true)"

	printf '%-62s %-34s %s\n' "$rel" "${build_expr:-<missing>}" "${cgo_directive:-<none>}"
	printf 'ERROR: %s still imports C after CGo support removal.\n' "$rel" >&2
	bad=1
done < <(find . \
	-path './.git' -prune -o \
	-path './bin' -prune -o \
	-path './build' -prune -o \
	-path './dist' -prune -o \
	-path './tmp' -prune -o \
	-name '*.go' -type f -print | sort)

printf '\nCGo files: %d\n' "$count"
exit "$bad"
