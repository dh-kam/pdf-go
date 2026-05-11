#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=${1:-"$(cd "$(dirname "$0")/.." && pwd)"}
OUT_DIR=${2:-"$ROOT_DIR/tmp/sample_compare_icc_focus"}
MODES_CSV=${3:-"legacy,adaptive-dct-iccbased-v1"}
TIMEOUT_SEC=${4:-3600}
PER_PAGE_TIMEOUT_SEC=${5:-600}
FOCUS_FIXTURE=${6:-"$ROOT_DIR/test/testdata/image_sampling_focus.csv"}
SAMPLE_ROOT=${7:-"test/testdata/sample-files/"}

if [[ ! -f "$FOCUS_FIXTURE" ]]; then
  echo "focus fixture not found: $FOCUS_FIXTURE" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"
INCLUDE_DOC_LIST="$OUT_DIR/include_docs.txt"

awk -F, 'NR>1 {print $1}' "$FOCUS_FIXTURE" | sort -u > "$INCLUDE_DOC_LIST"

MODES=()
IFS=',' read -r -a MODES <<< "$MODES_CSV"
if [[ ${#MODES[@]} -eq 0 ]]; then
  echo "no modes provided" >&2
  exit 1
fi

for i in "${!MODES[@]}"; do
  MODES[$i]="$(echo "${MODES[$i]}" | xargs)"
  if [[ -z "${MODES[$i]}" ]]; then
    echo "empty mode in MODES_CSV=$MODES_CSV" >&2
    exit 1
  fi
 done

extract_focus_rows() {
  local fixture_csv="$1"
  local report_csv="$2"
  local out_csv="$3"
  awk -F, 'NR==FNR { if (FNR>1) keys[$1"#"$2]=1; next } FNR==1 || keys[$1"#"$2] { print }' \
    "$fixture_csv" "$report_csv" > "$out_csv"
}

summarize_focus_csv() {
  local mode="$1"
  local focus_csv="$2"
  awk -F, -v mode="$mode" '
    NR==1 {next}
    {
      rows++
      exact+=$3
      mae+=$4
      if ($6=="true") passExact++
      if ($7=="true") passMAE++
      if ($10!="") errs++
    }
    END {
      if (rows==0) {
        printf("| %s | 0 | 0 | 0 | 0.0000 | 0.0000 | 0 |\n", mode)
      } else {
        printf("| %s | %d | %d | %d | %.4f | %.4f | %d |\n", mode, rows, passExact, passMAE, exact/rows, mae/rows, errs)
      }
    }
  ' "$focus_csv"
}

join_delta_rows() {
  local baseline_csv="$1"
  local target_csv="$2"
  local mode="$3"
  awk -F, -v mode="$mode" '
    NR==FNR {
      if (FNR==1) next
      key=$1"#"$2
      exact[key]=$3
      mae[key]=$4
      next
    }
    FNR==1 {next}
    {
      key=$1"#"$2
      if (!(key in exact)) next
      dExact=$3-exact[key]
      dMAE=$4-mae[key]
      printf("| %s | %s | %s | %.4f | %.4f |\n", mode, $1, $2, dExact, dMAE)
    }
  ' "$baseline_csv" "$target_csv"
}

for mode in "${MODES[@]}"; do
  mode_out="$OUT_DIR/$mode"
  mkdir -p "$mode_out"

  echo "[focus-matrix] running mode=$mode"
  (
    cd "$ROOT_DIR"
    go run -tags='nojpx,nojbig2' ./tmp/goal98_batch.go \
      -root "$ROOT_DIR" \
      -out "$mode_out" \
      -dpi 150 \
      -image-sampling-mode "$mode" \
      -threshold 99 \
      -threshold-mae 99 \
      -timeout-sec "$TIMEOUT_SEC" \
      -per-page-timeout-sec "$PER_PAGE_TIMEOUT_SEC" \
      -sample-only=true \
      -sample-root "$SAMPLE_ROOT" \
      -skip-compressed-duplicates=false \
      -include-doc-list "$INCLUDE_DOC_LIST"

    go run -tags='nojpx,nojbig2' ./tmp/goal98_compare_html.go \
      -report "$mode_out/report.csv" \
      -out "$mode_out/html" \
      -threshold 99 \
      -sample-only=true \
      -sample-root "$SAMPLE_ROOT"
  )

  extract_focus_rows "$FOCUS_FIXTURE" "$mode_out/report.csv" "$mode_out/focus.csv"
 done

MATRIX_MD="$OUT_DIR/matrix.md"
{
  echo "# ICCBased Focus Matrix"
  echo
  echo "- generated_at: $(date +%Y-%m-%dT%H:%M:%S)"
  echo "- modes: $MODES_CSV"
  echo "- include_docs: $INCLUDE_DOC_LIST"
  echo "- focus_fixture: $FOCUS_FIXTURE"
  echo
  echo "## Mode Summary (focus pages only)"
  echo
  echo "| Mode | Rows | Pass Exact | Pass MAE | Avg Exact | Avg MAE | Errors |"
  echo "|---|---:|---:|---:|---:|---:|---:|"
  for mode in "${MODES[@]}"; do
    summarize_focus_csv "$mode" "$OUT_DIR/$mode/focus.csv"
  done

  if [[ -f "$OUT_DIR/legacy/focus.csv" ]]; then
    echo
    echo "## Delta Vs legacy (focus pages)"
    echo
    echo "| Mode | PDF | Page | Delta Exact | Delta MAE |"
    echo "|---|---|---:|---:|---:|"
    for mode in "${MODES[@]}"; do
      [[ "$mode" == "legacy" ]] && continue
      join_delta_rows "$OUT_DIR/legacy/focus.csv" "$OUT_DIR/$mode/focus.csv" "$mode"
    done
  fi
} > "$MATRIX_MD"

echo "focus_matrix=$MATRIX_MD"
for mode in "${MODES[@]}"; do
  echo "mode_report[$mode]=$OUT_DIR/$mode/report.csv"
  echo "mode_focus[$mode]=$OUT_DIR/$mode/focus.csv"
  echo "mode_html[$mode]=$OUT_DIR/$mode/html/index.html"
 done
