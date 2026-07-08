#!/usr/bin/env python3
import csv
import os
import shutil
import subprocess
import sys
from pathlib import Path

def main():
    root_dir = Path("/workspace/pdf-reader/go-pdf")
    selected_tsv = root_dir / "tmp/random_pdf_compare_20260606/meta/selected.tsv"
    run_dir = root_dir / "tmp/run_classify_raw"
    bin_pdfcompare = root_dir / "bin/pdfcompare"

    # Artifact output path
    artifact_path = Path("/home/dh.kam/.gemini/antigravity-cli/brain/ceb0ba35-5946-40bd-8cb4-c56f5842a4fa/exact100_failures.md")

    if not selected_tsv.exists():
        print(f"ERROR: {selected_tsv} not found.", file=sys.stderr)
        sys.exit(1)
    if not bin_pdfcompare.exists():
        print(f"ERROR: {bin_pdfcompare} not found. Build it first.", file=sys.stderr)
        sys.exit(1)

    # Clean previous runs
    if run_dir.exists():
        shutil.rmtree(run_dir)
    run_dir.mkdir(parents=True, exist_ok=True)

    # Read selected PDFs
    jobs = []
    with open(selected_tsv, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f, delimiter="\t")
        for row in reader:
            jobs.append({
                "index": int(row["index"]),
                "dpi": int(row["dpi"]),
                "url": row["url"],
                "raw_pdf": Path(row["raw_pdf"]),
            })

    total_pdfs = len(jobs)
    total_pages = 0
    exact100_pages = 0
    failed_pages_list = []
    errored_docs_list = []

    print(f"Starting comparison of {total_pdfs} raw PDFs across assigned DPIs...")
    sys.stdout.flush()

    for job in jobs:
        idx = job["index"]
        dpi = job["dpi"]
        pdf_path = job["raw_pdf"]
        pdf_name = pdf_path.name

        if not pdf_path.exists():
            print(f"[{idx}/{total_pdfs}] SKIP: {pdf_name} does not exist.")
            continue

        doc_tmp_dir = run_dir / f"doc_{idx}"
        out_tmp_dir = run_dir / f"out_{idx}"
        doc_tmp_dir.mkdir(parents=True, exist_ok=True)
        out_tmp_dir.mkdir(parents=True, exist_ok=True)

        # Copy raw PDF to target folder
        shutil.copy(pdf_path, doc_tmp_dir / pdf_name)

        # Run pdfcompare
        cmd = [
            str(bin_pdfcompare),
            "-scan-root", str(doc_tmp_dir),
            "-out", str(out_tmp_dir),
            "-dpi", str(dpi),
            "-backend", "splash",
            "-workers", "12",
            "-timeout-sec", "360",
            "-bad-pixel-limit-per-page", "0",
            "-keep-images=false"
        ]

        env = os.environ.copy()
        env["CGO_ENABLED"] = "0"
        env["PDF_FREETYPE_GO"] = "1"

        try:
            res = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=450)
            report_csv = out_tmp_dir / "report.csv"
            if not report_csv.exists():
                err_msg = res.stderr.strip() or "No report.csv generated"
                print(f"[{idx}/{total_pdfs}] ERROR: {pdf_name} failed to process: {err_msg}")
                errored_docs_list.append({"index": idx, "pdf": pdf_name, "dpi": dpi, "error": err_msg})
                continue

            # Parse report.csv
            doc_pages = 0
            doc_exact = 0
            doc_failed = 0
            with open(report_csv, "r", encoding="utf-8") as rf:
                csv_reader = csv.DictReader(rf)
                for row in csv_reader:
                    doc_pages += 1
                    total_pages += 1
                    exact100 = row.get("exact100") == "true"
                    err = row.get("error", "").strip()

                    if exact100 and not err:
                        doc_exact += 1
                        exact100_pages += 1
                    else:
                        doc_failed += 1
                        failed_pages_list.append({
                            "pdf": pdf_name,
                            "page": int(row.get("page", 0)),
                            "dpi": dpi,
                            "matched": row.get("matched_pixels", "0"),
                            "total": row.get("total_pixels", "0"),
                            "exact_pct": row.get("exact_percent", "0"),
                            "error": err
                        })

            print(f"[{idx}/{total_pdfs}] {pdf_name} (DPI {dpi}): {doc_pages} pages, {doc_exact} exact100, {doc_failed} failed")
            sys.stdout.flush()

        except subprocess.TimeoutExpired:
            print(f"[{idx}/{total_pdfs}] TIMEOUT: {pdf_name} exceeded timeout limits.")
            errored_docs_list.append({"index": idx, "pdf": pdf_name, "dpi": dpi, "error": "Timeout expired"})
        except Exception as e:
            print(f"[{idx}/{total_pdfs}] ERROR: {pdf_name} failed: {e}")
            errored_docs_list.append({"index": idx, "pdf": pdf_name, "dpi": dpi, "error": str(e)})
        finally:
            # Clean up immediately to save disk space
            shutil.rmtree(doc_tmp_dir, ignore_errors=True)
            shutil.rmtree(out_tmp_dir, ignore_errors=True)

    # Write Markdown report
    exact_pct_overall = (exact100_pages / total_pages * 100) if total_pages > 0 else 0.0
    failed_pages_count = len(failed_pages_list)

    # Grouping failures by category
    type_a = []  # Timeout / Execution Error
    type_b = []  # Significant Mismatch (< 99.0%)
    type_c = []  # Moderate Mismatch (99.0% ~ 99.9%)
    type_d = []  # Minor / AA Mismatch (99.9% ~ 99.9999%)

    for fp in failed_pages_list:
        try:
            pct = float(fp["exact_pct"])
        except ValueError:
            pct = 0.0
        err = fp["error"]
        
        if err or (pct == 0.0 and fp["total"] == "0"):
            type_a.append(fp)
        elif pct < 99.0:
            type_b.append(fp)
        elif pct < 99.9:
            type_c.append(fp)
        else:
            type_d.append(fp)

    for err_doc in errored_docs_list:
        type_a.append({
            "pdf": err_doc["pdf"],
            "page": "-",
            "dpi": err_doc["dpi"],
            "matched": "0",
            "total": "0",
            "exact_pct": "0.0",
            "error": err_doc["error"]
        })

    with open(artifact_path, "w", encoding="utf-8") as out_f:
        out_f.write(f"# Exact100 Parity Report (Full Raw arXiv PDFs)\n\n")
        out_f.write(f"This report classifies all pages that do not meet the Exact100 pixel-exact comparison standard when compared against Poppler.\n\n")
        
        out_f.write(f"## Summary Metrics\n\n")
        out_f.write(f"| Metric | Value |\n")
        out_f.write(f"| :--- | :--- |\n")
        out_f.write(f"| **Total PDFs Evaluated** | {total_pdfs} |\n")
        out_f.write(f"| **Total Pages Evaluated** | {total_pages} |\n")
        out_f.write(f"| **Exact100 Pages** | {exact100_pages} ({exact_pct_overall:.2f}%) |\n")
        out_f.write(f"| **Exact100 Discrepancy Pages** | {failed_pages_count} ({100.0 - exact_pct_overall:.2f}%) |\n")
        out_f.write(f"| **Errored/Timed-out Documents** | {len(errored_docs_list)} |\n\n")

        out_f.write(f"## Discrepancy Classification Summary\n\n")
        out_f.write(f"To assist in debugging, the discrepancies have been classified into four types based on the severity of the pixel mismatch:\n\n")
        out_f.write(f"| Classification Type | Description | Page Count | Percentage of Failures |\n")
        out_f.write(f"| :--- | :--- | :---: | :---: |\n")
        total_failures = len(type_a) + len(type_b) + len(type_c) + len(type_d)
        pct_a = (len(type_a) / total_failures * 100) if total_failures > 0 else 0.0
        pct_b = (len(type_b) / total_failures * 100) if total_failures > 0 else 0.0
        pct_c = (len(type_c) / total_failures * 100) if total_failures > 0 else 0.0
        pct_d = (len(type_d) / total_failures * 100) if total_failures > 0 else 0.0
        out_f.write(f"| **Type A**: Timeout / Execution Error | Document timeouts (>360s) or rendering system errors | {len(type_a)} | {pct_a:.2f}% |\n")
        out_f.write(f"| **Type B**: Functional Mismatch (< 99.0%) | Significant rendering mismatch (missing fonts, CMYK/Color, image predictors, shading issues) | {len(type_b)} | {pct_b:.2f}% |\n")
        out_f.write(f"| **Type C**: Moderate Mismatch (99.0% ~ 99.9%) | Intermediate differences (complex shading, stroke width discrepancies, antialiasing phase discrepancies) | {len(type_c)} | {pct_c:.2f}% |\n")
        out_f.write(f"| **Type D**: Minor/AA Mismatch (>= 99.9%) | Near-perfect pixel matching. Typically minor font anti-aliasing tie-breaking differences. | {len(type_d)} | {pct_d:.2f}% |\n\n")

        # Type A Section
        out_f.write(f"### Type A: Timeout / Execution Error ({len(type_a)} pages)\n\n")
        if type_a:
            out_f.write(f"| PDF Name | Page | DPI | Error Context |\n")
            out_f.write(f"| :--- | :---: | :---: | :--- |\n")
            for fp in type_a:
                out_f.write(f"| {fp['pdf']} | {fp['page']} | {fp['dpi']} | {fp['error'] or 'Missing page output / 0% match'} |\n")
        else:
            out_f.write(f"*No Type A failures detected.*\n")
        out_f.write(f"\n")

        # Type B Section
        out_f.write(f"### Type B: Functional Mismatch (< 99.0%) ({len(type_b)} pages)\n\n")
        if type_b:
            out_f.write(f"| PDF Name | Page | DPI | Matched/Total Pixels | Exact % | Error |\n")
            out_f.write(f"| :--- | :---: | :---: | :---: | :---: | :--- |\n")
            for fp in type_b:
                out_f.write(f"| {fp['pdf']} | {fp['page']} | {fp['dpi']} | {fp['matched']}/{fp['total']} | {float(fp['exact_pct']):.6f}% | {fp['error'] or '-'} |\n")
        else:
            out_f.write(f"*No Type B failures detected.*\n")
        out_f.write(f"\n")

        # Type C Section
        out_f.write(f"### Type C: Moderate Mismatch (99.0% ~ 99.9%) ({len(type_c)} pages)\n\n")
        if type_c:
            out_f.write(f"| PDF Name | Page | DPI | Matched/Total Pixels | Exact % | Error |\n")
            out_f.write(f"| :--- | :---: | :---: | :---: | :---: | :--- |\n")
            for fp in type_c:
                out_f.write(f"| {fp['pdf']} | {fp['page']} | {fp['dpi']} | {fp['matched']}/{fp['total']} | {float(fp['exact_pct']):.6f}% | {fp['error'] or '-'} |\n")
        else:
            out_f.write(f"*No Type C failures detected.*\n")
        out_f.write(f"\n")

        # Type D Section
        out_f.write(f"### Type D: Minor / AA Mismatch (>= 99.9%) ({len(type_d)} pages)\n\n")
        if type_d:
            out_f.write(f"| PDF Name | Page | DPI | Matched/Total Pixels | Exact % | Error |\n")
            out_f.write(f"| :--- | :---: | :---: | :---: | :---: | :--- |\n")
            for fp in type_d:
                out_f.write(f"| {fp['pdf']} | {fp['page']} | {fp['dpi']} | {fp['matched']}/{fp['total']} | {float(fp['exact_pct']):.6f}% | {fp['error'] or '-'} |\n")
        else:
            out_f.write(f"*No Type D failures detected.*\n")
        out_f.write(f"\n")

    print(f"\nClassification complete. Report written to: {artifact_path}")

if __name__ == "__main__":
    main()
