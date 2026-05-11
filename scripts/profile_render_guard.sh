#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
  echo "usage: $0 <summary.log> [max_avg_ms] [max_slowest_ms]" >&2
  exit 1
fi

SUMMARY="$1"
MAX_AVG_MS="${2:-80}"
MAX_SLOWEST_MS="${3:-200}"

if [[ ! -f "$SUMMARY" ]]; then
  echo "summary log not found: $SUMMARY" >&2
  exit 1
fi

python3 - "$SUMMARY" "$MAX_AVG_MS" "$MAX_SLOWEST_MS" <<'PY'
import re
import sys
from pathlib import Path

summary_path = Path(sys.argv[1])
max_avg_ms = float(sys.argv[2])
max_slowest_ms = float(sys.argv[3])

text = summary_path.read_text(encoding="utf-8")


def parse_duration_ms(s: str) -> float:
    s = s.strip()
    if s.endswith("ms"):
        return float(s[:-2])
    if s.endswith("µs"):
        return float(s[:-2]) / 1000.0
    if s.endswith("us"):
        return float(s[:-2]) / 1000.0
    if s.endswith("s"):
        return float(s[:-1]) * 1000.0
    raise ValueError(f"unsupported duration: {s}")

m_failed = re.search(r"pages failed:\s*(\d+)", text)
if not m_failed:
    print("failed to parse pages failed", file=sys.stderr)
    sys.exit(2)
pages_failed = int(m_failed.group(1))

m_avg = re.search(r"avg render per page:\s*([^\n]+)", text)
if not m_avg:
    print("failed to parse avg render per page", file=sys.stderr)
    sys.exit(2)
avg_ms = parse_duration_ms(m_avg.group(1).strip())

slowest_ms = None
for line in text.splitlines():
    if re.match(r"\s*1\)\s", line):
        parts = line.strip().split()
        for token in parts[::-1]:
            try:
                slowest_ms = parse_duration_ms(token)
                break
            except Exception:
                continue
        if slowest_ms is not None:
            break

if slowest_ms is None:
    print("failed to parse slowest page duration", file=sys.stderr)
    sys.exit(2)

print("profile render guard")
print(f"  summary: {summary_path}")
print(f"  pages_failed: {pages_failed}")
print(f"  avg_ms: {avg_ms:.3f} (limit {max_avg_ms:.3f})")
print(f"  slowest_ms: {slowest_ms:.3f} (limit {max_slowest_ms:.3f})")

failed = False
if pages_failed > 0:
    print("guard failed: pages_failed > 0", file=sys.stderr)
    failed = True
if avg_ms > max_avg_ms:
    print("guard failed: avg_ms exceeded", file=sys.stderr)
    failed = True
if slowest_ms > max_slowest_ms:
    print("guard failed: slowest_ms exceeded", file=sys.stderr)
    failed = True

if failed:
    sys.exit(3)
PY
