#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 4 ]]; then
  echo "usage: $0 <report.csv> <out.md> [profile_summary.log] [top_n]" >&2
  exit 1
fi

REPORT_CSV="$1"
OUT_MD="$2"
PROFILE_SUMMARY="${3:-}"
TOP_N="${4:-20}"

if [[ ! -f "$REPORT_CSV" ]]; then
  echo "report csv not found: $REPORT_CSV" >&2
  exit 1
fi

python3 - "$REPORT_CSV" "$OUT_MD" "$PROFILE_SUMMARY" "$TOP_N" <<'PY'
import csv
import datetime
import os
import re
import sys
from collections import defaultdict
from pathlib import Path

report_path = Path(sys.argv[1])
out_path = Path(sys.argv[2])
profile_path_raw = sys.argv[3].strip()
top_n = int(sys.argv[4])


def parse_duration_ms(s: str) -> float:
    s = s.strip()
    if s.endswith("ms"):
        return float(s[:-2])
    if s.endswith("µs") or s.endswith("us"):
        return float(s[:-2]) / 1000.0
    if s.endswith("s"):
        return float(s[:-1]) * 1000.0
    raise ValueError(f"unsupported duration: {s}")


rows = []
failure_type_counts = defaultdict(int)
with report_path.open(newline="", encoding="utf-8") as f:
    reader = csv.DictReader(f)
    for row in reader:
        try:
            page = int((row.get("page") or "0").strip() or "0")
        except ValueError:
            page = 0
        if page <= 0:
            continue

        try:
            exact = float((row.get("exact_percent") or "0").strip() or "0")
        except ValueError:
            exact = 0.0
        try:
            mae = float((row.get("mae_similarity_percent") or "0").strip() or "0")
        except ValueError:
            mae = 0.0

        pass_exact = (row.get("pass_exact") or "").strip().lower() == "true"
        pass_mae = (row.get("pass_mae") or "").strip().lower() == "true"
        failure_type = (row.get("failure_type") or "").strip()
        if not failure_type:
            failure_type = "unknown"

        item = {
            "pdf": (row.get("pdf") or "").strip(),
            "page": page,
            "exact": exact,
            "mae": mae,
            "pass_exact": pass_exact,
            "pass_mae": pass_mae,
            "failure_type": failure_type,
            "error": (row.get("error") or "").strip(),
        }
        rows.append(item)
        if not pass_exact or not pass_mae or item["error"]:
            failure_type_counts[failure_type] += 1

total_pages = len(rows)
pass_exact_count = sum(1 for r in rows if r["pass_exact"])
pass_mae_count = sum(1 for r in rows if r["pass_mae"])
fail_exact_count = total_pages - pass_exact_count
avg_exact = (sum(r["exact"] for r in rows) / total_pages) if total_pages else 0.0
avg_mae = (sum(r["mae"] for r in rows) / total_pages) if total_pages else 0.0

doc_fail_counts = defaultdict(int)
for r in rows:
    if not r["pass_exact"]:
        doc_fail_counts[r["pdf"]] += 1

worst_pages = sorted(rows, key=lambda r: (r["exact"], r["mae"]))[:top_n]
top_fail_docs = sorted(doc_fail_counts.items(), key=lambda kv: kv[1], reverse=True)[:top_n]

latency = {
    "docs": None,
    "pages_tried": None,
    "pages_ok": None,
    "pages_failed": None,
    "workers": None,
    "cache_enabled": None,
    "avg_render_ms": None,
    "total_render_ms": None,
    "slowest_ms": None,
}

profile_summary_path = Path(profile_path_raw) if profile_path_raw else None
if profile_summary_path and profile_summary_path.exists():
    text = profile_summary_path.read_text(encoding="utf-8")

    def m_int(pattern: str):
        m = re.search(pattern, text)
        return int(m.group(1)) if m else None

    def m_str(pattern: str):
        m = re.search(pattern, text)
        return m.group(1).strip() if m else None

    latency["docs"] = m_int(r"docs:\s*(\d+)")
    latency["pages_tried"] = m_int(r"pages tried:\s*(\d+)")
    latency["pages_ok"] = m_int(r"pages ok:\s*(\d+)")
    latency["pages_failed"] = m_int(r"pages failed:\s*(\d+)")
    latency["workers"] = m_int(r"workers:\s*(\d+)")
    latency["cache_enabled"] = m_str(r"cache enabled:\s*([^\n]+)")

    avg_render = m_str(r"avg render per page:\s*([^\n]+)")
    if avg_render:
        latency["avg_render_ms"] = parse_duration_ms(avg_render)
    total_render = m_str(r"total render time:\s*([^\n]+)")
    if total_render:
        latency["total_render_ms"] = parse_duration_ms(total_render)

    for line in text.splitlines():
        if re.match(r"\s*1\)\s", line):
            tokens = line.strip().split()
            for tok in reversed(tokens):
                try:
                    latency["slowest_ms"] = parse_duration_ms(tok)
                    break
                except Exception:
                    continue
            if latency["slowest_ms"] is not None:
                break

now = datetime.datetime.now().isoformat(timespec="seconds")
out_path.parent.mkdir(parents=True, exist_ok=True)

with out_path.open("w", encoding="utf-8") as w:
    w.write("# Render Accuracy/Performance Tradeoff Report\n\n")
    w.write(f"- Generated: `{now}`\n")
    w.write(f"- Report CSV: `{report_path}`\n")
    if profile_summary_path:
        w.write(f"- Profile Summary: `{profile_summary_path}`\n")
    else:
        w.write("- Profile Summary: `(not provided)`\n")
    w.write("\n")

    w.write("## Accuracy Summary\n\n")
    w.write(f"- Compared pages: `{total_pages}`\n")
    w.write(f"- PASS exact: `{pass_exact_count}` (`{(pass_exact_count * 100.0 / total_pages) if total_pages else 0.0:.2f}%`)\n")
    w.write(f"- FAIL exact: `{fail_exact_count}` (`{(fail_exact_count * 100.0 / total_pages) if total_pages else 0.0:.2f}%`)\n")
    w.write(f"- PASS MAE: `{pass_mae_count}` (`{(pass_mae_count * 100.0 / total_pages) if total_pages else 0.0:.2f}%`)\n")
    w.write(f"- Avg exact similarity: `{avg_exact:.4f}%`\n")
    w.write(f"- Avg MAE similarity: `{avg_mae:.4f}%`\n\n")

    w.write("## Performance Summary\n\n")
    if latency["avg_render_ms"] is None:
        w.write("- Profile summary was not provided or not parseable.\n\n")
    else:
        w.write(f"- Profile docs/pages: `{latency['docs']}` docs, `{latency['pages_tried']}` pages tried\n")
        w.write(f"- Profile pages ok/failed: `{latency['pages_ok']}` / `{latency['pages_failed']}`\n")
        w.write(f"- Workers: `{latency['workers']}`\n")
        w.write(f"- Cache enabled: `{latency['cache_enabled']}`\n")
        w.write(f"- Avg render per page: `{latency['avg_render_ms']:.3f}ms`\n")
        w.write(f"- Total render time: `{latency['total_render_ms']:.3f}ms`\n")
        w.write(f"- Slowest page time: `{latency['slowest_ms']:.3f}ms`\n\n")

    w.write("## Worst Pages (Exact Similarity)\n\n")
    w.write("| Rank | PDF | Page | Exact % | MAE % |\n")
    w.write("|---:|---|---:|---:|---:|\n")
    for i, r in enumerate(worst_pages, start=1):
        w.write(f"| {i} | `{r['pdf']}` | {r['page']} | {r['exact']:.3f} | {r['mae']:.3f} |\n")
    w.write("\n")

    w.write("## Top Fail Documents (Exact)\n\n")
    w.write("| Rank | PDF | Fail Pages |\n")
    w.write("|---:|---|---:|\n")
    for i, (pdf, cnt) in enumerate(top_fail_docs, start=1):
        w.write(f"| {i} | `{pdf}` | {cnt} |\n")
    w.write("\n")

    if failure_type_counts:
        w.write("## Fail Types\n\n")
        w.write("| Rank | Failure Type | Count |\n")
        w.write("|---:|---|---:|\n")
        for i, (ftype, cnt) in enumerate(
            sorted(failure_type_counts.items(), key=lambda kv: kv[1], reverse=True)[:top_n],
            start=1,
        ):
            w.write(f"| {i} | `{ftype}` | {cnt} |\n")
        w.write("\n")

print(f"tradeoff_report={out_path}")
PY
