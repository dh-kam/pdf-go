#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "usage: $0 <report.csv> <out.md> [focus_fixture.csv]" >&2
  exit 1
fi

REPORT_CSV="$1"
OUT_MD="$2"
FOCUS_FIXTURE="${3:-test/testdata/image_sampling_focus.csv}"

if [[ ! -f "$REPORT_CSV" ]]; then
  echo "report csv not found: $REPORT_CSV" >&2
  exit 1
fi

if [[ ! -f "$FOCUS_FIXTURE" ]]; then
  echo "focus fixture not found: $FOCUS_FIXTURE" >&2
  exit 1
fi

python3 - "$REPORT_CSV" "$OUT_MD" "$FOCUS_FIXTURE" <<'PY'
import csv
import datetime
import sys
from collections import defaultdict
from pathlib import Path

report_path = Path(sys.argv[1])
out_path = Path(sys.argv[2])
fixture_path = Path(sys.argv[3])

focus_rows = []
with fixture_path.open(newline="", encoding="utf-8") as f:
    for row in csv.DictReader(f):
        pdf = (row.get("pdf") or "").strip()
        page_raw = (row.get("page") or "").strip()
        if not pdf or not page_raw:
            continue
        try:
            page = int(page_raw)
        except ValueError:
            continue
        if page <= 0:
            continue
        focus_rows.append(
            {
                "pdf": pdf,
                "page": page,
                "goal": (row.get("goal") or "").strip(),
                "notes": (row.get("notes") or "").strip(),
            }
        )

report_map = {}
with report_path.open(newline="", encoding="utf-8") as f:
    for row in csv.DictReader(f):
        pdf = (row.get("pdf") or "").strip()
        page_raw = (row.get("page") or "").strip()
        if not pdf or not page_raw:
            continue
        try:
            page = int(page_raw)
        except ValueError:
            continue
        if page <= 0:
            continue
        report_map[(pdf, page)] = {
            "exact": float((row.get("exact_percent") or "0").strip() or "0"),
            "mae": float((row.get("mae_similarity_percent") or "0").strip() or "0"),
            "pass_exact": (row.get("pass_exact") or "").strip().lower() == "true",
            "pass_mae": (row.get("pass_mae") or "").strip().lower() == "true",
            "failure_type": ((row.get("failure_type") or "").strip() or "unknown"),
            "error": (row.get("error") or "").strip(),
        }

status_counts = defaultdict(int)
failure_counts = defaultdict(int)
lines = []
lines.append("# 이미지 샘플링 포커스 리포트")
lines.append("")
lines.append(f"- generated_at: `{datetime.datetime.now().isoformat(timespec='seconds')}`")
lines.append(f"- report_csv: `{report_path}`")
lines.append(f"- focus_fixture: `{fixture_path}`")
lines.append(f"- focus_pages: `{len(focus_rows)}`")
lines.append("")
lines.append("| pdf | page | goal | exact(%) | mae(%) | failure_type | status | notes |")
lines.append("| --- | ---: | --- | ---: | ---: | --- | --- | --- |")

for item in focus_rows:
    key = (item["pdf"], item["page"])
    row = report_map.get(key)
    if row is None:
        status = "missing"
        status_counts[status] += 1
        lines.append(
            f"| `{item['pdf']}` | {item['page']} | `{item['goal']}` | - | - | - | `{status}` | {item['notes']} |"
        )
        continue

    status = "pass" if row["pass_exact"] and row["pass_mae"] and not row["error"] else "fail"
    status_counts[status] += 1
    failure_counts[row["failure_type"]] += 1
    lines.append(
        f"| `{item['pdf']}` | {item['page']} | `{item['goal']}` | {row['exact']:.4f} | {row['mae']:.4f} | `{row['failure_type']}` | `{status}` | {item['notes']} |"
    )

lines.append("")
lines.append("## 상태 집계")
lines.append("")
for k in sorted(status_counts.keys()):
    lines.append(f"- {k}: {status_counts[k]}")

if failure_counts:
    lines.append("")
    lines.append("## 실패 유형 집계")
    lines.append("")
    lines.append("| failure_type | count |")
    lines.append("| --- | ---: |")
    for failure_type, count in sorted(failure_counts.items(), key=lambda kv: kv[1], reverse=True):
        lines.append(f"| `{failure_type}` | {count} |")

out_path.parent.mkdir(parents=True, exist_ok=True)
out_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"image_sampling_focus_report={out_path}")
PY
