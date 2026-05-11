#!/usr/bin/env python3
"""
PDF Rendering Comparison Script
================================
Poppler(pdftoppm)와 go-pdf(pdfrender)로 각 PDF를 렌더링 후 PNG로 저장하고,
PIL로 픽셀 단위 비교를 수행하여 HTML 리포트를 생성합니다.

Usage:
    python3 scripts/compare_render.py [--dpi 150] [--skip-render] [--pdf-filter PREFIX]

Structure:
    testdata/compare/
    ├── pdfs/           # 수집된 테스트 PDF
    ├── poppler_png/    # Poppler 렌더링 결과
    ├── gopdf_png/      # go-pdf 렌더링 결과
    ├── diff_png/       # 차이 시각화 이미지
    └── report/
        ├── index.html              # 전체 요약 리포트
        └── detail_{name}.html      # 개별 PDF 상세 리포트
"""

from __future__ import annotations

import argparse
import csv
import html
import io
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

try:
    from PIL import Image, ImageChops, ImageDraw, ImageFont
except ImportError:
    print("PIL(Pillow)이 필요합니다: pip install Pillow")
    sys.exit(1)

# ── Constants ──────────────────────────────────────────────────────────────

BASE_DIR = Path(__file__).resolve().parent.parent / "testdata" / "compare"
PDFS_DIR = BASE_DIR / "pdfs"
POPPLER_DIR = BASE_DIR / "poppler_png"
GOPDF_DIR = BASE_DIR / "gopdf_png"
DIFF_DIR = BASE_DIR / "diff_png"
REPORT_DIR = BASE_DIR / "report"

PDFRENDER_BIN = Path(__file__).resolve().parent.parent / "bin" / "pdfrender"
PDFTOPPM_BIN = "pdftoppm"

DEFAULT_DPI = 150
THRESHOLD_PERCENT = 99.0
MAX_PAGES = 50  # 페이지 수 제한 (메모리 보호)


# ── Data Classes ───────────────────────────────────────────────────────────

@dataclass
class PageInfo:
    page_num: int
    poppler_png: Optional[Path] = None
    gopdf_png: Optional[Path] = None
    diff_png: Optional[Path] = None
    exact_percent: float = 0.0
    similarity_percent: float = 0.0
    total_pixels: int = 0
    diff_pixels: int = 0
    avg_diff: float = 0.0
    error: Optional[str] = None
    width: int = 0
    height: int = 0


@dataclass
class PDFResult:
    pdf_name: str
    pdf_path: Path
    pages: list[PageInfo] = field(default_factory=list)
    error: Optional[str] = None

    @property
    def avg_similarity(self) -> float:
        if not self.pages:
            return 0.0
        valid = [p.similarity_percent for p in self.pages if p.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_exact(self) -> float:
        if not self.pages:
            return 0.0
        valid = [p.exact_percent for p in self.pages if p.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def worst_page(self) -> Optional[PageInfo]:
        if not self.pages:
            return None
        valid = [p for p in self.pages if p.error is None]
        if not valid:
            return None
        return min(valid, key=lambda p: p.similarity_percent)

    @property
    def page_count(self) -> int:
        return len(self.pages)

    @property
    def failed_pages(self) -> int:
        return sum(1 for p in self.pages if p.error is not None)

    @property
    def passed(self) -> bool:
        if self.error:
            return False
        if not self.pages:
            return False
        return all(p.similarity_percent >= THRESHOLD_PERCENT for p in self.pages if p.error is None)


# ── Rendering ──────────────────────────────────────────────────────────────

def render_poppler(pdf_path: Path, output_dir: Path, dpi: int) -> list[Path]:
    """Poppler pdftoppm으로 PDF를 PNG로 렌더링."""
    output_dir.mkdir(parents=True, exist_ok=True)
    prefix = pdf_path.stem

    cmd = [
        PDFTOPPM_BIN,
        "-png",
        "-r", str(dpi),
        str(pdf_path),
        str(output_dir / prefix),
    ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    if result.returncode != 0:
        raise RuntimeError(f"pdftoppm failed: {result.stderr.strip()}")

    # pdftoppm outputs: prefix-1.png, prefix-2.png, ...
    pngs = sorted(output_dir.glob(f"{prefix}-*.png"))
    return pngs


def render_gopdf(pdf_path: Path, output_dir: Path, dpi: int) -> list[Path]:
    """go-pdf pdfrender로 PDF를 PNG로 렌더링."""
    output_dir.mkdir(parents=True, exist_ok=True)

    if not PDFRENDER_BIN.exists():
        raise RuntimeError(f"pdfrender binary not found: {PDFRENDER_BIN}")

    cmd = [
        str(PDFRENDER_BIN),
        "-o", str(output_dir),
        "-d", str(dpi),
        "-w", "1",
        str(pdf_path),
    ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    if result.returncode != 0:
        raise RuntimeError(f"pdfrender failed: {result.stderr.strip()}")

    # pdfrender outputs: {stem}_page_0001.png, ...
    pngs = sorted(output_dir.glob(f"*_page_*.png"))
    return pngs


# ── Image Comparison ───────────────────────────────────────────────────────

def compare_images(poppler_img: Image.Image, gopdf_img: Image.Image, diff_path: Path) -> tuple[float, float, int, int, float]:
    """
    두 이미지를 픽셀 단위로 비교.
    Returns: (exact_percent, similarity_percent, total_pixels, diff_pixels, avg_diff)
    """
    # 크기 맞추기 (큰 쪽에 맞춤, 부족한 부분은 흰색으로 패딩)
    pw, ph = poppler_img.size
    gw, gh = gopdf_img.size
    max_w, max_h = max(pw, gw), max(ph, gh)

    if poppler_img.size != (max_w, max_h):
        new_img = Image.new("RGBA", (max_w, max_h), (255, 255, 255, 255))
        new_img.paste(poppler_img, (0, 0))
        poppler_img = new_img

    if gopdf_img.size != (max_w, max_h):
        new_img = Image.new("RGBA", (max_w, max_h), (255, 255, 255, 255))
        new_img.paste(gopdf_img, (0, 0))
        gopdf_img = new_img

    # RGBA로 변환
    poppler_rgba = poppler_img.convert("RGBA")
    gopdf_rgba = gopdf_img.convert("RGBA")

    # Use numpy-free pixel access via raw bytes for speed
    poppler_bytes = poppler_rgba.tobytes()
    gopdf_bytes = gopdf_rgba.tobytes()

    total = max_w * max_h
    diff_count = 0
    total_diff = 0.0

    # 차이 시각화 이미지 - build via bytearray for speed
    diff_buf = bytearray(max_w * max_h * 4)
    diff_pixels = []

    for i in range(total):
        off = i * 4
        pr, pg2, pb, pa = poppler_bytes[off], poppler_bytes[off+1], poppler_bytes[off+2], poppler_bytes[off+3]
        gr, gg, gb, ga = gopdf_bytes[off], gopdf_bytes[off+1], gopdf_bytes[off+2], gopdf_bytes[off+3]

        dr = abs(pr - gr)
        dg = abs(pg2 - gg)
        db = abs(pb - gb)
        da = abs(pa - ga)
        pixel_diff = dr + dg + db + da
        total_diff += pixel_diff

        if pixel_diff > 0:
            diff_count += 1
            intensity = min(255, pixel_diff * 2)
            diff_pixels.append((i % max_w, i // max_w, intensity, pixel_diff))
            doff = off
            diff_buf[doff] = intensity
            diff_buf[doff+1] = 0
            diff_buf[doff+2] = 0
            diff_buf[doff+3] = 255
        # else: stays 0,0,0,0 (transparent)

    exact_percent = ((total - diff_count) / total * 100) if total > 0 else 0.0
    similarity_percent = (1.0 - total_diff / (total * 1020.0)) * 100 if total > 0 else 0.0
    avg_diff = total_diff / total if total > 0 else 0.0

    # 차이 이미지를 반투명 오버레이로 저장
    diff_img = Image.frombytes("RGBA", (max_w, max_h), bytes(diff_buf))
    overlay = poppler_rgba.copy()

    for px, py, intensity, _ in diff_pixels:
        orig = overlay.getpixel((px, py))
        r = min(255, orig[0] // 2 + intensity)
        g = max(0, orig[1] // 3)
        b = max(0, orig[2] // 3)
        overlay.putpixel((px, py), (r, g, b, 255))

    diff_path.parent.mkdir(parents=True, exist_ok=True)
    overlay.save(str(diff_path))

    return exact_percent, similarity_percent, total, diff_count, avg_diff


# ── Pipeline ───────────────────────────────────────────────────────────────

def process_pdf(pdf_path: Path, dpi: int, skip_render: bool) -> PDFResult:
    """단일 PDF를 처리: 렌더링 + 비교."""
    result = PDFResult(pdf_name=pdf_path.name, pdf_path=pdf_path)
    name = pdf_path.stem

    poppler_out = POPPLER_DIR / name
    gopdf_out = GOPDF_DIR / name
    diff_out = DIFF_DIR / name

    # Step 1: Render
    if not skip_render:
        try:
            render_poppler(pdf_path, poppler_out, dpi)
        except Exception as e:
            result.error = f"Poppler render failed: {e}"
            return result

        try:
            render_gopdf(pdf_path, gopdf_out, dpi)
        except Exception as e:
            result.error = f"go-pdf render failed: {e}"
            return result

    # Step 2: Collect rendered PNGs
    # Poppler: {name}/{name}-1.png, {name}-2.png ...
    poppler_pngs = sorted(poppler_out.glob(f"{name}-*.png"))
    # Also try without prefix (sometimes pdftoppm uses different naming)
    if not poppler_pngs:
        poppler_pngs = sorted(poppler_out.glob("*.png"))

    # go-pdf: {name}/*_page_XXXX.png
    gopdf_pngs = sorted(gopdf_out.glob("*_page_*.png"))
    if not gopdf_pngs:
        gopdf_pngs = sorted(gopdf_out.glob("*.png"))

    if not poppler_pngs:
        result.error = "No Poppler PNGs found"
        return result
    if not gopdf_pngs:
        result.error = "No go-pdf PNGs found"
        return result

    # Step 3: Compare page by page
    page_count = min(len(poppler_pngs), len(gopdf_pngs), MAX_PAGES)

    for i in range(page_count):
        pi = PageInfo(page_num=i + 1)
        pi.poppler_png = poppler_pngs[i]
        pi.gopdf_png = gopdf_pngs[i]

        try:
            poppler_img = Image.open(pi.poppler_png)
            gopdf_img = Image.open(pi.gopdf_png)

            pi.width, pi.height = poppler_img.size

            diff_path = diff_out / f"diff_page_{i+1:04d}.png"
            pi.diff_png = diff_path

            pi.exact_percent, pi.similarity_percent, pi.total_pixels, pi.diff_pixels, pi.avg_diff = \
                compare_images(poppler_img, gopdf_img, diff_path)

        except Exception as e:
            pi.error = str(e)

        result.pages.append(pi)

    return result


# ── HTML Report Generation ─────────────────────────────────────────────────

def generate_html_report(results: list[PDFResult]) -> None:
    """전체 HTML 리포트 생성 (index + detail pages)."""
    REPORT_DIR.mkdir(parents=True, exist_ok=True)

    # CSS styles
    css = """
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
               background: #0d1117; color: #c9d1d9; padding: 20px; }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 { color: #58a6ff; margin-bottom: 20px; font-size: 24px; }
        h2 { color: #58a6ff; margin: 20px 0 10px; font-size: 18px; }
        h3 { color: #8b949e; margin: 10px 0; font-size: 14px; }

        .summary-grid {
            display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px; margin-bottom: 24px;
        }
        .summary-card {
            background: #161b22; border: 1px solid #30363d; border-radius: 8px;
            padding: 16px; text-align: center;
        }
        .summary-card .value { font-size: 32px; font-weight: bold; color: #58a6ff; }
        .summary-card .label { font-size: 12px; color: #8b949e; margin-top: 4px; }
        .summary-card.pass .value { color: #3fb950; }
        .summary-card.fail .value { color: #f85149; }
        .summary-card.warn .value { color: #d29922; }

        table { width: 100%; border-collapse: collapse; margin: 16px 0; }
        th { background: #161b22; color: #8b949e; padding: 10px 12px; text-align: left;
             font-size: 12px; text-transform: uppercase; border-bottom: 2px solid #30363d; }
        td { padding: 10px 12px; border-bottom: 1px solid #21262d; font-size: 13px; }
        tr:hover { background: #161b22; }

        .badge { display: inline-block; padding: 2px 8px; border-radius: 12px;
                 font-size: 11px; font-weight: 600; }
        .badge-pass { background: #1a3a2a; color: #3fb950; }
        .badge-fail { background: #3a1a1a; color: #f85149; }
        .badge-error { background: #3a2a1a; color: #d29922; }

        .bar { height: 8px; background: #21262d; border-radius: 4px; overflow: hidden; }
        .bar-fill { height: 100%; border-radius: 4px; transition: width 0.3s; }
        .bar-fill.green { background: linear-gradient(90deg, #238636, #3fb950); }
        .bar-fill.yellow { background: linear-gradient(90deg, #9e6a03, #d29922); }
        .bar-fill.red { background: linear-gradient(90deg, #da3633, #f85149); }

        a { color: #58a6ff; text-decoration: none; }
        a:hover { text-decoration: underline; }

        .page-grid { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 16px; margin: 16px 0; }
        .page-card { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 12px; }
        .page-card img { width: 100%; border-radius: 4px; border: 1px solid #30363d; }
        .page-card .title { text-align: center; color: #8b949e; font-size: 12px; margin-bottom: 8px; }

        .page-stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; margin: 8px 0; }
        .page-stat { text-align: center; padding: 8px; background: #0d1117; border-radius: 4px; }
        .page-stat .val { font-size: 18px; font-weight: bold; }
        .page-stat .lbl { font-size: 10px; color: #8b949e; }

        .nav { display: flex; gap: 8px; margin: 16px 0; flex-wrap: wrap; }
        .nav a { padding: 6px 12px; background: #21262d; border-radius: 4px; color: #c9d1d9;
                 font-size: 13px; border: 1px solid #30363d; }
        .nav a:hover { background: #30363d; text-decoration: none; }
        .nav a.active { background: #1f6feb; border-color: #1f6feb; }

        .filter-bar { display: flex; gap: 8px; margin-bottom: 16px; }
        .filter-btn { padding: 6px 14px; background: #21262d; border: 1px solid #30363d;
                      border-radius: 20px; color: #c9d1d9; cursor: pointer; font-size: 12px; }
        .filter-btn:hover { background: #30363d; }
        .filter-btn.active { background: #1f6feb; border-color: #1f6feb; }

        .footer { text-align: center; color: #484f58; font-size: 11px; margin-top: 40px; padding: 20px; }
    </style>
    """

    # ── Generate index.html ──
    total_pdfs = len(results)
    total_pages = sum(r.page_count for r in results)
    passed_pdfs = sum(1 for r in results if r.passed and not r.error)
    failed_pdfs = sum(1 for r in results if not r.passed and not r.error)
    error_pdfs = sum(1 for r in results if r.error)

    all_similarities = []
    for r in results:
        for p in r.pages:
            if p.error is None:
                all_similarities.append(p.similarity_percent)

    avg_similarity = sum(all_similarities) / len(all_similarities) if all_similarities else 0
    min_similarity = min(all_similarities) if all_similarities else 0

    rows_html = ""
    for i, r in enumerate(results):
        if r.error:
            status_badge = f'<span class="badge badge-error">ERROR</span>'
            sim_bar = '<span style="color:#d29922">N/A</span>'
            pages_info = f'<span style="color:#d29922">{html.escape(r.error[:60])}</span>'
        else:
            if r.passed:
                status_badge = '<span class="badge badge-pass">PASS</span>'
            else:
                status_badge = '<span class="badge badge-fail">FAIL</span>'

            sim = r.avg_similarity
            bar_color = "green" if sim >= 99.5 else ("yellow" if sim >= 99.0 else "red")
            sim_bar = f'''
                <div class="bar"><div class="bar-fill {bar_color}" style="width:{sim:.1f}%"></div></div>
                <span style="font-size:11px; color:#8b949e;">{sim:.4f}%</span>
            '''
            pages_info = f'{r.page_count} pages, {r.failed_pages} failed'

        detail_link = f'detail_{r.pdf_name}.html'
        row_class = 'FAIL' if not r.passed and not r.error else ''
        rows_html += f'''
        <tr data-status="{'pass' if r.passed and not r.error else ('error' if r.error else 'fail')}">
            <td><a href="{detail_link}">{html.escape(r.pdf_name)}</a></td>
            <td>{status_badge}</td>
            <td>{sim_bar}</td>
            <td>{r.avg_exact:.4f}%</td>
            <td>{pages_info}</td>
        </tr>
        '''

    index_html = f"""<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PDF Rendering Comparison Report</title>
    {css}
</head>
<body>
<div class="container">
    <h1>PDF Rendering Comparison Report</h1>

    <div class="summary-grid">
        <div class="summary-card">
            <div class="value">{total_pdfs}</div>
            <div class="label">Total PDFs</div>
        </div>
        <div class="summary-card pass">
            <div class="value">{passed_pdfs}</div>
            <div class="label">Passed (&ge;{THRESHOLD_PERCENT}%)</div>
        </div>
        <div class="summary-card fail">
            <div class="value">{failed_pdfs}</div>
            <div class="label">Failed</div>
        </div>
        <div class="summary-card warn">
            <div class="value">{error_pdfs}</div>
            <div class="label">Errors</div>
        </div>
        <div class="summary-card">
            <div class="value">{total_pages}</div>
            <div class="label">Total Pages</div>
        </div>
        <div class="summary-card">
            <div class="value">{avg_similarity:.2f}%</div>
            <div class="label">Avg Similarity</div>
        </div>
        <div class="summary-card">
            <div class="value">{min_similarity:.2f}%</div>
            <div class="label">Min Similarity</div>
        </div>
    </div>

    <div class="filter-bar">
        <button class="filter-btn active" onclick="filterTable('all')">All</button>
        <button class="filter-btn" onclick="filterTable('pass')">Passed</button>
        <button class="filter-btn" onclick="filterTable('fail')">Failed</button>
        <button class="filter-btn" onclick="filterTable('error')">Errors</button>
    </div>

    <table id="results-table">
        <thead>
            <tr>
                <th>PDF File</th>
                <th>Status</th>
                <th>Similarity</th>
                <th>Exact Match</th>
                <th>Pages</th>
            </tr>
        </thead>
        <tbody>
            {rows_html}
        </tbody>
    </table>

    <div class="footer">
        Generated by compare_render.py | {len(results)} PDFs | {total_pages} pages compared
    </div>
</div>

<script>
function filterTable(status) {{
    document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
    event.target.classList.add('active');
    document.querySelectorAll('#results-table tbody tr').forEach(row => {{
        if (status === 'all') {{
            row.style.display = '';
        }} else {{
            row.style.display = row.dataset.status === status ? '' : 'none';
        }}
    }});
}}
</script>
</body>
</html>"""

    (REPORT_DIR / "index.html").write_text(index_html, encoding="utf-8")

    # ── Generate detail pages ──
    for r in results:
        generate_detail_page(r, results, css)

    print(f"\nReport generated: {REPORT_DIR / 'index.html'}")


def generate_detail_page(result: PDFResult, all_results: list[PDFResult], css: str) -> None:
    """개별 PDF의 상세 비교 페이지 생성."""

    # Navigation: prev / index / next
    current_idx = next((i for i, r in enumerate(all_results) if r.pdf_name == result.pdf_name), -1)
    prev_link = f'detail_{all_results[current_idx-1].pdf_name}.html' if current_idx > 0 else None
    next_link = f'detail_{all_results[current_idx+1].pdf_name}.html' if current_idx < len(all_results) - 1 else None

    nav_html = '<div class="nav">'
    if prev_link:
        nav_html += f'<a href="{prev_link}">&larr; {html.escape(all_results[current_idx-1].pdf_name[:30])}</a>'
    nav_html += '<a href="index.html">Index</a>'
    if next_link:
        nav_html += f'<a href="{next_link}">{html.escape(all_results[current_idx+1].pdf_name[:30])} &rarr;</a>'
    nav_html += '</div>'

    if result.error:
        page_content = f'''
        <div class="page-card" style="grid-column: 1/-1;">
            <h2>Error</h2>
            <p style="color:#f85149;">{html.escape(result.error)}</p>
        </div>
        '''
    else:
        page_content = ""
        for pi in result.pages:
            poppler_rel = os.path.relpath(pi.poppler_png, REPORT_DIR) if pi.poppler_png else ""
            gopdf_rel = os.path.relpath(pi.gopdf_png, REPORT_DIR) if pi.gopdf_png else ""
            diff_rel = os.path.relpath(pi.diff_png, REPORT_DIR) if pi.diff_png else ""

            sim = pi.similarity_percent
            bar_color = "green" if sim >= 99.5 else ("yellow" if sim >= 99.0 else "red")
            status_badge = '<span class="badge badge-pass">PASS</span>' if sim >= THRESHOLD_PERCENT else '<span class="badge badge-fail">FAIL</span>'

            page_content += f'''
            <h2>Page {pi.page_num} {status_badge}</h2>

            <div class="page-stats">
                <div class="page-stat">
                    <div class="val" style="color:{"#3fb950" if sim >= 99.0 else "#f85149"}">{sim:.4f}%</div>
                    <div class="lbl">Similarity</div>
                </div>
                <div class="page-stat">
                    <div class="val">{pi.exact_percent:.4f}%</div>
                    <div class="lbl">Exact Match</div>
                </div>
                <div class="page-stat">
                    <div class="val">{pi.diff_pixels:,} / {pi.total_pixels:,}</div>
                    <div class="lbl">Diff Pixels</div>
                </div>
            </div>

            <div class="bar" style="margin-bottom:12px;">
                <div class="bar-fill {bar_color}" style="width:{sim:.1f}%"></div>
            </div>

            <div class="page-grid">
                <div class="page-card">
                    <div class="title">Poppler (Reference)</div>
                    <img src="{poppler_rel}" alt="Poppler Page {pi.page_num}" loading="lazy">
                </div>
                <div class="page-card">
                    <div class="title">go-pdf (Ours)</div>
                    <img src="{gopdf_rel}" alt="go-pdf Page {pi.page_num}" loading="lazy">
                </div>
                <div class="page-card">
                    <div class="title">Difference</div>
                    <img src="{diff_rel}" alt="Diff Page {pi.page_num}" loading="lazy">
                </div>
            </div>
            '''

            if pi.error:
                page_content += f'<p style="color:#f85149; margin:8px 0;">Error: {html.escape(pi.error)}</p>'

            page_content += '<hr style="border:none;border-top:1px solid #30363d;margin:24px 0;">'

    detail_html = f"""<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{html.escape(result.pdf_name)} - Render Comparison</title>
    {css}
</head>
<body>
<div class="container">
    {nav_html}

    <h1>{html.escape(result.pdf_name)}</h1>
    <h3>Avg Similarity: {result.avg_similarity:.4f}% | Avg Exact: {result.avg_exact:.4f}% | Pages: {result.page_count}</h3>

    {page_content}

    <div class="footer">
        <a href="index.html">&larr; Back to Index</a>
    </div>
</div>
</body>
</html>"""

    (REPORT_DIR / f"detail_{result.pdf_name}").write_text(detail_html, encoding="utf-8")


# ── CSV Export ─────────────────────────────────────────────────────────────

def export_csv(results: list[PDFResult]) -> None:
    """결과를 CSV로 내보내기."""
    csv_path = BASE_DIR / "report.csv"
    with csv_path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(["pdf", "page", "exact_percent", "similarity_percent",
                         "total_pixels", "diff_pixels", "avg_diff", "pass", "error"])
        for r in results:
            if r.error:
                writer.writerow([r.pdf_name, 0, 0, 0, 0, 0, 0, False, r.error])
                continue
            for p in r.pages:
                writer.writerow([
                    r.pdf_name, p.page_num,
                    f"{p.exact_percent:.4f}", f"{p.similarity_percent:.4f}",
                    p.total_pixels, p.diff_pixels, f"{p.avg_diff:.4f}",
                    p.similarity_percent >= THRESHOLD_PERCENT,
                    p.error or ""
                ])
    print(f"CSV exported: {csv_path}")


# ── Main ───────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="PDF Rendering Comparison Tool")
    parser.add_argument("--dpi", type=int, default=DEFAULT_DPI, help="Render DPI")
    parser.add_argument("--skip-render", action="store_true", help="Skip rendering, use existing PNGs")
    parser.add_argument("--pdf-filter", default="", help="Only process PDFs starting with this prefix")
    parser.add_argument("--max-files", type=int, default=0, help="Max number of PDFs to process (0=all)")
    args = parser.parse_args()

    # Collect PDFs
    pdfs = sorted(PDFS_DIR.glob("*.pdf"))
    if args.pdf_filter:
        pdfs = [p for p in pdfs if p.name.startswith(args.pdf_filter)]
    if args.max_files > 0:
        pdfs = pdfs[:args.max_files]

    print(f"Found {len(pdfs)} PDFs to process")
    print(f"DPI: {args.dpi}")
    print(f"Skip render: {args.skip_render}")
    print(f"Threshold: {THRESHOLD_PERCENT}%")
    print()

    results: list[PDFResult] = []
    for i, pdf_path in enumerate(pdfs):
        print(f"[{i+1}/{len(pdfs)}] Processing: {pdf_path.name}")
        result = process_pdf(pdf_path, args.dpi, args.skip_render)

        if result.error:
            print(f"  ERROR: {result.error}")
        elif result.pages:
            print(f"  Pages: {result.page_count}, Avg Similarity: {result.avg_similarity:.4f}%")
            worst = result.worst_page
            if worst:
                print(f"  Worst page: {worst.page_num} ({worst.similarity_percent:.4f}%)")
        results.append(result)

    # Export CSV
    export_csv(results)

    # Generate HTML report
    generate_html_report(results)

    # Print summary
    print("\n" + "=" * 60)
    passed = sum(1 for r in results if r.passed and not r.error)
    failed = sum(1 for r in results if not r.passed and not r.error)
    errors = sum(1 for r in results if r.error)
    print(f"Summary: {passed} passed, {failed} failed, {errors} errors out of {len(results)} PDFs")
    print(f"Report: file://{REPORT_DIR / 'index.html'}")


if __name__ == "__main__":
    main()
