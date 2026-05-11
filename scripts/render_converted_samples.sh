#!/usr/bin/env bash

set -euo pipefail
shopt -s nullglob

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SAMPLE_ROOT="${SAMPLE_ROOT:-$ROOT_DIR/test/testdata/sample-files}"
OUT_ROOT="${OUT_ROOT:-$ROOT_DIR/converted}"
POPPLER_OUT="${POPPLER_OUT:-$OUT_ROOT/poppler}"
MINE_OUT="${MINE_OUT:-$OUT_ROOT/mine}"
LOG_FILE="${LOG_FILE:-$OUT_ROOT/render_all.log}"
DPI="${DPI:-150}"
DOC_FILTER="${DOC_FILTER:-}"
PDFRENDER_BIN="${PDFRENDER_BIN:-$ROOT_DIR/bin/pdfrender}"

sample_password_for() {
	local rel_path="$1"
	case "$rel_path" in
		005-libreoffice-writer-password/libreoffice-writer-password.pdf)
			printf '%s' 'openpassword'
			;;
		*)
			return 0
			;;
	esac
}

normalize_poppler_pages() {
	local dir="$1"
	local source=""
	local page_no=""
	local dest=""

	for source in "$dir"/rendered-*.png; do
		page_no="${source##*-}"
		page_no="${page_no%.png}"
		printf -v dest "%s/page_%04d.png" "$dir" "$((10#$page_no))"
		mv "$source" "$dest"
	done
}

normalize_mine_pages() {
	local dir="$1"
	local source=""
	local page_no=""
	local dest=""

	for source in "$dir"/rendered_page_*.png; do
		page_no="${source##*_page_}"
		page_no="${page_no%.png}"
		dest="$dir/page_${page_no}.png"
		mv "$source" "$dest"
	done
}

count_pages() {
	local dir="$1"
	find "$dir" -maxdepth 1 -type f -name 'page_*.png' | wc -l | tr -d ' '
}

read_error_text() {
	local path="$1"
	if [ ! -f "$path" ]; then
		return 0
	fi
	tr '\n' ' ' < "$path" | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//'
}

if [ ! -d "$SAMPLE_ROOT" ]; then
	echo "sample root not found: $SAMPLE_ROOT" >&2
	exit 1
fi

if ! command -v pdftoppm >/dev/null 2>&1; then
	echo "pdftoppm not found in PATH" >&2
	exit 1
fi

if [ ! -x "$PDFRENDER_BIN" ]; then
	echo "pdfrender binary not found or not executable: $PDFRENDER_BIN" >&2
	exit 1
fi

mkdir -p "$POPPLER_OUT" "$MINE_OUT" "$OUT_ROOT"
: > "$LOG_FILE"

total=0
ok=0
fail=0
mismatch=0

while IFS= read -r -d '' pdf_path; do
	rel_path="${pdf_path#$SAMPLE_ROOT/}"
	if [ -n "$DOC_FILTER" ] && [[ ! "$rel_path" =~ $DOC_FILTER ]]; then
		continue
	fi

	group_name="${rel_path%%/*}"
	base_name="$(basename "$pdf_path")"
	doc_name="${group_name}__${base_name}"
	password="$(sample_password_for "$rel_path")"
	poppler_dir="$POPPLER_OUT/$doc_name"
	mine_dir="$MINE_OUT/$doc_name"
	poppler_err_file="$poppler_dir/.stderr.log"
	mine_err_file="$mine_dir/.stderr.log"
	poppler_out_file="$poppler_dir/.stdout.log"
	mine_out_file="$mine_dir/.stdout.log"

	total=$((total + 1))
	rm -rf "$poppler_dir" "$mine_dir"
	mkdir -p "$poppler_dir" "$mine_dir"

	poppler_status="OK"
	mine_status="OK"

	poppler_cmd=(pdftoppm -png -r "$DPI")
	if [ -n "$password" ]; then
		poppler_cmd+=(-upw "$password")
	fi
	poppler_cmd+=("$pdf_path" "$poppler_dir/rendered")
	if ! "${poppler_cmd[@]}" >"$poppler_out_file" 2>"$poppler_err_file"; then
		poppler_status="FAIL"
	fi
	normalize_poppler_pages "$poppler_dir"

	mine_cmd=("$PDFRENDER_BIN" -q -d "$DPI" -o "$mine_dir" --prefix rendered)
	if [ -n "$password" ]; then
		mine_cmd+=(--password "$password")
	fi
	mine_cmd+=("$pdf_path")
	if ! "${mine_cmd[@]}" >"$mine_out_file" 2>"$mine_err_file"; then
		mine_status="FAIL"
	fi
	normalize_mine_pages "$mine_dir"

	poppler_pages="$(count_pages "$poppler_dir")"
	mine_pages="$(count_pages "$mine_dir")"

	if [ "$poppler_status" = "OK" ] && [ "$mine_status" = "OK" ] && [ "$poppler_pages" = "$mine_pages" ]; then
		ok=$((ok + 1))
		printf 'OK  %s poppler=%s ours=%s\n' "$doc_name" "$poppler_pages" "$mine_pages" >>"$LOG_FILE"
	else
		fail=$((fail + 1))
		if [ "$poppler_pages" != "$mine_pages" ]; then
			mismatch=$((mismatch + 1))
		fi
		printf 'FAIL  %s poppler_status=%s ours_status=%s poppler=%s ours=%s\n' \
			"$doc_name" "$poppler_status" "$mine_status" "$poppler_pages" "$mine_pages" >>"$LOG_FILE"

		poppler_error="$(read_error_text "$poppler_err_file")"
		mine_error="$(read_error_text "$mine_err_file")"
		if [ -n "$poppler_error" ]; then
			printf '  poppler_error: %s\n' "$poppler_error" >>"$LOG_FILE"
		fi
		if [ -n "$mine_error" ]; then
			printf '  ours_error: %s\n' "$mine_error" >>"$LOG_FILE"
		fi
	fi

	rm -f "$poppler_err_file" "$mine_err_file" "$poppler_out_file" "$mine_out_file"
done < <(find "$SAMPLE_ROOT" -mindepth 2 -maxdepth 2 -type f -name '*.pdf' -print0 | sort -z)

printf '\n__CONVERTED_RENDER_DONE__:count=%d ok=%d fail=%d mismatch=%d dpi=%s\n' \
	"$total" "$ok" "$fail" "$mismatch" "$DPI" >>"$LOG_FILE"

cat "$LOG_FILE"
