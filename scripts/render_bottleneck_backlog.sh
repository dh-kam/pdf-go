#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 5 || $# -gt 6 ]]; then
  echo "usage: $0 <profile_summary.log> <cpu.out> <mem.out> <compare_report.csv> <out.md> [top_n]" >&2
  exit 1
fi

PROFILE_SUMMARY="$1"
CPU_PROFILE="$2"
MEM_PROFILE="$3"
COMPARE_REPORT="$4"
OUT_MD="$5"
TOP_N="${6:-8}"

if [[ ! -f "$PROFILE_SUMMARY" ]]; then
  echo "profile summary not found: $PROFILE_SUMMARY" >&2
  exit 1
fi
if [[ ! -f "$CPU_PROFILE" ]]; then
  echo "cpu profile not found: $CPU_PROFILE" >&2
  exit 1
fi
if [[ ! -f "$MEM_PROFILE" ]]; then
  echo "mem profile not found: $MEM_PROFILE" >&2
  exit 1
fi
if [[ ! -f "$COMPARE_REPORT" ]]; then
  echo "compare report not found: $COMPARE_REPORT" >&2
  exit 1
fi

python3 - "$PROFILE_SUMMARY" "$CPU_PROFILE" "$MEM_PROFILE" "$COMPARE_REPORT" "$OUT_MD" "$TOP_N" <<'PY'
import csv
import datetime
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

profile_summary = Path(sys.argv[1])
cpu_profile = Path(sys.argv[2])
mem_profile = Path(sys.argv[3])
compare_report = Path(sys.argv[4])
out_md = Path(sys.argv[5])
top_n = int(sys.argv[6])

summary_text = profile_summary.read_text(encoding="utf-8")


def find_number(pattern: str):
    m = re.search(pattern, summary_text)
    return m.group(1).strip() if m else ""


def parse_pprof_top(args):
    raw = subprocess.check_output(args, text=True, stderr=subprocess.STDOUT)
    rows = []
    in_table = False
    for line in raw.splitlines():
        if line.startswith("      flat"):
            in_table = True
            continue
        if not in_table:
            continue
        if not line.strip():
            continue
        # Example:
        #   110ms 35.48% 35.48% 150ms 48.39% golang.org/x/image/vector.(*Rasterizer)...
        parts = re.split(r"\s+", line.strip(), maxsplit=5)
        if len(parts) < 6:
            continue
        flat, flat_pct, _sum, cum, cum_pct, fn = parts
        rows.append(
            {
                "flat": flat,
                "flat_pct": flat_pct,
                "cum": cum,
                "cum_pct": cum_pct,
                "func": fn,
            }
        )
    return rows


cpu_rows = parse_pprof_top(["go", "tool", "pprof", "-top", str(cpu_profile)])
mem_rows = parse_pprof_top(
    ["go", "tool", "pprof", "-top", "-sample_index=inuse_space", str(mem_profile)]
)

total_pages = 0
pass_exact = 0
pass_mae = 0
sum_exact = 0.0
sum_mae = 0.0
fail_docs = defaultdict(int)
failure_type_counts = defaultdict(int)
worst_rows = []

with compare_report.open(newline="", encoding="utf-8") as f:
    reader = csv.DictReader(f)
    for row in reader:
        try:
            page = int((row.get("page") or "").strip())
        except Exception:
            continue
        if page <= 0:
            continue
        total_pages += 1
        exact = float((row.get("exact_percent") or "0").strip() or "0")
        mae = float((row.get("mae_similarity_percent") or "0").strip() or "0")
        p_exact = (row.get("pass_exact") or "").strip().lower() == "true"
        p_mae = (row.get("pass_mae") or "").strip().lower() == "true"
        failure_type = (row.get("failure_type") or "").strip() or "unknown"
        pass_exact += int(p_exact)
        pass_mae += int(p_mae)
        sum_exact += exact
        sum_mae += mae
        if not p_exact:
            fail_docs[(row.get("pdf") or "").strip()] += 1
        if (not p_exact) or (not p_mae) or (row.get("error") or "").strip():
            failure_type_counts[failure_type] += 1
        worst_rows.append(
            {
                "pdf": (row.get("pdf") or "").strip(),
                "page": page,
                "exact": exact,
                "mae": mae,
                "failure_type": failure_type,
            }
        )

worst_rows.sort(key=lambda r: (r["exact"], r["mae"]))
worst_rows = worst_rows[:top_n]
top_fail_docs = sorted(fail_docs.items(), key=lambda kv: kv[1], reverse=True)[:top_n]

avg_exact = (sum_exact / total_pages) if total_pages else 0.0
avg_mae = (sum_mae / total_pages) if total_pages else 0.0
pass_exact_rate = (pass_exact * 100.0 / total_pages) if total_pages else 0.0
pass_mae_rate = (pass_mae * 100.0 / total_pages) if total_pages else 0.0

docs = find_number(r"docs:\s*([0-9]+)")
pages_tried = find_number(r"pages tried:\s*([0-9]+)")
pages_ok = find_number(r"pages ok:\s*([0-9]+)")
pages_failed = find_number(r"pages failed:\s*([0-9]+)")
workers = find_number(r"workers:\s*([0-9]+)")
cache_enabled = find_number(r"cache enabled:\s*([^\n]+)")
avg_render = find_number(r"avg render per page:\s*([^\n]+)")
total_render = find_number(r"total render time:\s*([^\n]+)")
slowest = ""
for line in summary_text.splitlines():
    if re.match(r"\s*1\)\s", line):
        slowest = line.strip()
        break

now = datetime.datetime.now().isoformat(timespec="seconds")
out_md.parent.mkdir(parents=True, exist_ok=True)

with out_md.open("w", encoding="utf-8") as w:
    w.write("# PDF 렌더 병목/요구사항/우선순위 백로그\n\n")
    w.write(f"- 생성 시각: `{now}`\n")
    w.write(f"- 프로파일 요약: `{profile_summary}`\n")
    w.write(f"- 비교 리포트: `{compare_report}`\n\n")

    w.write("## 체크포인트\n\n")
    w.write(f"- 프로파일 문서/페이지: `{docs}` docs, `{pages_tried}` pages\n")
    w.write(f"- 페이지 성공/실패: `{pages_ok}` / `{pages_failed}`\n")
    if cache_enabled:
        w.write(f"- cache enabled: `{cache_enabled}`\n")
    w.write(f"- workers: `{workers}`\n")
    w.write(f"- 평균 렌더/페이지: `{avg_render}`\n")
    w.write(f"- 총 렌더 시간: `{total_render}`\n")
    if slowest:
        w.write(f"- 최장 페이지: `{slowest}`\n")
    w.write(
        f"- 샘플 정합: exact `{pass_exact}/{total_pages}` (`{pass_exact_rate:.2f}%`), "
        f"MAE `{pass_mae}/{total_pages}` (`{pass_mae_rate:.2f}%`)\n"
    )
    w.write(f"- 평균 유사도: exact `{avg_exact:.4f}%`, MAE `{avg_mae:.4f}%`\n\n")

    w.write("## 병목 상위 Top\n\n")
    w.write("### CPU\n\n")
    w.write("| Rank | Flat | Flat% | Cum | Cum% | Function |\n")
    w.write("|---:|---:|---:|---:|---:|---|\n")
    for i, row in enumerate(cpu_rows[:top_n], start=1):
        w.write(
            f"| {i} | {row['flat']} | {row['flat_pct']} | {row['cum']} | "
            f"{row['cum_pct']} | `{row['func']}` |\n"
        )
    w.write("\n")

    w.write("### Memory (inuse_space)\n\n")
    w.write("| Rank | Flat | Flat% | Cum | Cum% | Function |\n")
    w.write("|---:|---:|---:|---:|---:|---|\n")
    for i, row in enumerate(mem_rows[:top_n], start=1):
        w.write(
            f"| {i} | {row['flat']} | {row['flat_pct']} | {row['cum']} | "
            f"{row['cum_pct']} | `{row['func']}` |\n"
        )
    w.write("\n")

    w.write("## 역할별 요구사항 명세\n\n")
    w.write("### 아키텍트 관점\n\n")
    w.write("- 렌더 핵심 경로(`renderPath`, vector rasterizer)는 성능/정확도 정책을 분리해 실험 가능해야 한다.\n")
    w.write("- 이미지 렌더 경로는 CTM/리샘플링/색공간(특히 ICCBased) 처리 단계를 독립 계층으로 분리해 원인 추적이 가능해야 한다.\n")
    w.write("- acceptance: 평균 렌더 시간 `<= 40ms`, 최장 페이지 `<= 80ms` 유지하면서 비교 리포트 MAE 평균 `>= 96%`.\n\n")

    w.write("### 개발자 관점\n\n")
    w.write("- 프로파일링/정합 리포트 생성은 단일 명령으로 반복 가능해야 하며 로그 마커를 남겨 재현성을 보장해야 한다.\n")
    w.write("- 이미지 샘플(007/imagemagick, 019/grayscale, 023/cmyk)은 페이지 단위 회귀 테스트로 고정해야 한다.\n")
    w.write("- acceptance: e2e 비교 테스트에서 지정 샘플 MAE가 기준 미달 시 즉시 실패.\n\n")

    w.write("### 테스터 관점\n\n")
    w.write("- 문서별 실패 페이지 Top-N과 XOR 이미지를 한 번에 확인할 수 있어야 한다.\n")
    w.write("- 정확도 기준은 exact/MAE를 동시에 관리하고, 실패 유형(색공간/리샘플링/텍스트/마스크)을 태깅해야 한다.\n")
    w.write("- acceptance: nightly 리포트에 문서별 실패 카운트와 worst-page 목록이 자동 포함.\n\n")

    w.write("### 기획자 관점\n\n")
    w.write("- 릴리즈 게이트 KPI를 수치로 확정하고 변경 시 승인 프로세스를 명문화해야 한다.\n")
    w.write("- 권장 KPI: 평균 렌더 `<= 40ms`, 최장 페이지 `<= 100ms`, 샘플 exact pass `>= 8%`, 샘플 MAE pass `>= 12%`.\n")
    w.write("- acceptance: KPI 변경은 `TODO.md` 체크포인트와 리포트 링크를 포함한 변경 이력으로만 반영.\n\n")

    w.write("### 사용자 관점\n\n")
    w.write("- 대용량/암호/부분 실패 문서에서 어떤 페이지가 왜 실패했는지 CLI 로그로 즉시 확인 가능해야 한다.\n")
    w.write("- 실행 재시도 가이드는 `--password`, `--fail-on-page-error`, `--max-page-pixels`, `--max-inflight-pixels` 중심으로 제공해야 한다.\n")
    w.write("- acceptance: FAQ 예시에 따라 재실행 시 실패 원인 분류가 동일하게 재현.\n\n")

    w.write("## 우선순위 작업 리스트\n\n")
    w.write("### P0 (정확도 리스크)\n\n")
    w.write("- [ ] ICCBased 이미지(007/019) 디코딩/색변환 경로 점검 및 비교 실험(원본/대체 색공간) 자동화.\n")
    w.write("- [ ] `imagemagick-images.pdf#p4` (DCTDecode) 렌더 오차 분석 후 회귀 테스트 추가.\n")
    w.write("- [x] 샘플 전체 compare에서 실패 유형 태깅(`failure_type` 컬럼 + HTML 표시) 반영.\n\n")

    w.write("### P1 (성능)\n\n")
    w.write("- [ ] `renderPath`/vector rasterizer hotspot의 입력 path 복잡도 기반 fast-path 도입.\n")
    w.write("- [ ] Form cache hit/miss 통계 수집 및 페이지별 적중률 리포트화.\n")
    w.write("- [ ] open/read 메모리 점유(`os.ReadFile`) 축소를 위한 스트리밍 경로 검토.\n\n")

    w.write("### P2 (운영/품질)\n\n")
    w.write("- [ ] KPI 변경 승인 절차를 `TODO.md` 운영 규칙으로 고정.\n")
    w.write("- [ ] 암호 PDF/부분 실패 복구 FAQ 초안 작성 및 smoke 스크립트 연동.\n")
    w.write("- [ ] nightly 결과를 기준 리포트와 자동 diff해 회귀 경고를 출력.\n\n")

    w.write("## 정확도 실패 상위\n\n")
    w.write("| Rank | PDF | Fail Pages |\n")
    w.write("|---:|---|---:|\n")
    for i, (pdf, cnt) in enumerate(top_fail_docs, start=1):
        w.write(f"| {i} | `{pdf}` | {cnt} |\n")
    w.write("\n")

    w.write("## 최저 유사도 페이지 Top\n\n")
    w.write("| Rank | PDF | Page | Exact % | MAE % | Failure Type |\n")
    w.write("|---:|---|---:|---:|---:|---|\n")
    for i, row in enumerate(worst_rows, start=1):
        w.write(
            f"| {i} | `{row['pdf']}` | {row['page']} | "
            f"{row['exact']:.3f} | {row['mae']:.3f} | `{row['failure_type']}` |\n"
        )
    w.write("\n")

    if failure_type_counts:
        w.write("## 실패 유형 집계\n\n")
        w.write("| Rank | Failure Type | Count |\n")
        w.write("|---:|---|---:|\n")
        for i, (ftype, cnt) in enumerate(
            sorted(failure_type_counts.items(), key=lambda kv: kv[1], reverse=True)[:top_n],
            start=1,
        ):
            w.write(f"| {i} | `{ftype}` | {cnt} |\n")
        w.write("\n")

print(f"bottleneck_backlog={out_md}")
PY
