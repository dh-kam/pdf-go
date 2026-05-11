#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 <baseline_report.csv> <current_report.csv> [--fail-on-regression]" >&2
	exit 1
fi

BASE_REPORT="$1"
CURRENT_REPORT="$2"
FAIL_ON_REGRESSION="${3:-}"

if [[ ! -f "${BASE_REPORT}" ]]; then
	echo "baseline report not found: ${BASE_REPORT}" >&2
	exit 1
fi
if [[ ! -f "${CURRENT_REPORT}" ]]; then
	echo "current report not found: ${CURRENT_REPORT}" >&2
	exit 1
fi

python3 - "$BASE_REPORT" "$CURRENT_REPORT" "$FAIL_ON_REGRESSION" <<'PY'
import csv
import sys


def load_report(path: str) -> dict:
    total_rows = 0
    compared_pages = 0
    errors = 0
    pass_exact = 0
    pass_mae = 0
    exact_sum = 0.0
    mae_sum = 0.0

    with open(path, "r", encoding="utf-8", newline="") as f:
        reader = csv.DictReader(f)
        for row in reader:
            total_rows += 1

            error = (row.get("error") or "").strip()
            if error:
                errors += 1

            page = int((row.get("page") or "0").strip() or "0")
            if page <= 0:
                continue

            compared_pages += 1

            if (row.get("pass_exact") or "").strip().lower() == "true":
                pass_exact += 1
            if (row.get("pass_mae") or "").strip().lower() == "true":
                pass_mae += 1

            exact_sum += float((row.get("exact_percent") or "0").strip() or "0")
            mae_sum += float((row.get("mae_similarity_percent") or "0").strip() or "0")

    avg_exact = exact_sum / compared_pages if compared_pages else 0.0
    avg_mae = mae_sum / compared_pages if compared_pages else 0.0

    return {
        "total_rows": total_rows,
        "compared_pages": compared_pages,
        "errors": errors,
        "pass_exact": pass_exact,
        "pass_mae": pass_mae,
        "avg_exact": avg_exact,
        "avg_mae": avg_mae,
    }


def pct(numerator: int, denominator: int) -> float:
    if denominator == 0:
        return 0.0
    return 100.0 * numerator / denominator


def fmt_delta(value: float, digits: int = 4) -> str:
    sign = "+" if value >= 0 else ""
    return f"{sign}{value:.{digits}f}"


base_path = sys.argv[1]
curr_path = sys.argv[2]
fail_on_regression = sys.argv[3] == "--fail-on-regression"

base = load_report(base_path)
curr = load_report(curr_path)

delta_rows = curr["total_rows"] - base["total_rows"]
delta_compared = curr["compared_pages"] - base["compared_pages"]
delta_errors = curr["errors"] - base["errors"]
delta_pass_exact = curr["pass_exact"] - base["pass_exact"]
delta_pass_mae = curr["pass_mae"] - base["pass_mae"]
delta_avg_exact = curr["avg_exact"] - base["avg_exact"]
delta_avg_mae = curr["avg_mae"] - base["avg_mae"]

print("goal98 report diff")
print(f"  baseline: {base_path}")
print(f"  current:  {curr_path}")
print("")
print(f"  rows:       {curr['total_rows']} ({fmt_delta(delta_rows, 0)})")
print(f"  pages:      {curr['compared_pages']} ({fmt_delta(delta_compared, 0)})")
print(f"  errors:     {curr['errors']} ({fmt_delta(delta_errors, 0)})")
print(f"  pass exact: {curr['pass_exact']}/{curr['compared_pages']} ({pct(curr['pass_exact'], curr['compared_pages']):.2f}%)")
print(f"              delta={fmt_delta(delta_pass_exact, 0)} pages")
print(f"  pass mae:   {curr['pass_mae']}/{curr['compared_pages']} ({pct(curr['pass_mae'], curr['compared_pages']):.2f}%)")
print(f"              delta={fmt_delta(delta_pass_mae, 0)} pages")
print(f"  avg exact:  {curr['avg_exact']:.4f}% ({fmt_delta(delta_avg_exact)} pts)")
print(f"  avg mae:    {curr['avg_mae']:.4f}% ({fmt_delta(delta_avg_mae)} pts)")

if fail_on_regression:
    regressed = (
        delta_pass_exact < 0
        or delta_pass_mae < 0
        or delta_avg_exact < 0
        or delta_avg_mae < 0
    )
    if regressed:
        print("")
        print("regression detected (--fail-on-regression enabled)", file=sys.stderr)
        sys.exit(2)
PY
