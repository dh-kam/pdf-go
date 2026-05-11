#!/usr/bin/env python3
"""
Summarize render parity priorities from report.csv.

Outputs:
- Worst page rows by similarity
- Worst documents by their lowest page similarity
- Optional prefix filtering for focused triage
"""

from __future__ import annotations

import argparse
import csv
from collections import defaultdict
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--report",
        default="/workspace/pdf-reader/go-pdf/test/testdata/output/render_parity/report.csv",
        help="Path to render parity report.csv",
    )
    parser.add_argument(
        "--top-pages",
        type=int,
        default=15,
        help="Number of worst page rows to print",
    )
    parser.add_argument(
        "--top-docs",
        type=int,
        default=10,
        help="Number of worst documents to print",
    )
    parser.add_argument(
        "--prefix",
        default="",
        help="Only include rows whose pdf path starts with this prefix",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    report_path = Path(args.report)
    if not report_path.exists():
        raise SystemExit(f"missing report: {report_path}")

    page_rows: list[tuple[float, float, str, str, str]] = []
    worst_by_doc: dict[str, float] = defaultdict(lambda: 101.0)

    with report_path.open(newline="") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            pdf_path = row["pdf"]
            if args.prefix and not pdf_path.startswith(args.prefix):
                continue
            if row["error"]:
                continue

            similarity = float(row["similarity_percent"])
            exact = float(row["exact_percent"])
            page_rows.append((similarity, exact, pdf_path, row["page"], row["pass"]))
            if similarity < worst_by_doc[pdf_path]:
                worst_by_doc[pdf_path] = similarity

    page_rows.sort()
    worst_docs = sorted(worst_by_doc.items(), key=lambda item: item[1])

    prefix_note = f" prefix={args.prefix!r}" if args.prefix else ""
    print(f"report={report_path}{prefix_note}")
    print()
    print(f"worst pages (top {min(args.top_pages, len(page_rows))})")
    for similarity, exact, pdf_path, page, passed in page_rows[: args.top_pages]:
        print(
            f"{similarity:8.4f}  exact={exact:8.4f}  pass={passed:5}  "
            f"{pdf_path}#p{page}"
        )

    print()
    print(f"worst documents (top {min(args.top_docs, len(worst_docs))})")
    for pdf_path, similarity in worst_docs[: args.top_docs]:
        print(f"{similarity:8.4f}  {pdf_path}")


if __name__ == "__main__":
    main()
