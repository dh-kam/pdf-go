#!/usr/bin/env python3
"""Count exact100 pages (0 differing pixels) per doc and corpus-wide.
Reads test/testdata/<base>/{poppler_png,gopdf_png} rendered at the same DPI.

Default <base> is the tracked `compare` corpus (437 pages). Use `--base 2nd`
to aggregate the external ~1441-page corpus rendered from test/2nd_pdfs.list.
"""
import argparse
from PIL import Image
import numpy as np, glob, os
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def base_dirs(base: str):
    b = ROOT / "test" / "testdata" / base
    return b / "poppler_png", b / "gopdf_png"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="compare",
                    help="corpus subtree under test/testdata/ (default: compare; use '2nd' for the external list corpus)")
    args = ap.parse_args()

    POP, GO = base_dirs(args.base)
    if not POP.exists():
        print(f"poppler_png not found: {POP}")
        return

    def doc_pages(name):
        pop = sorted(glob.glob(str(POP / name / "*.png")))
        go = sorted(glob.glob(str(GO / name / "*.png")))
        return pop, go

    total_pages = exact_pages = 0
    per_doc = []
    for d in sorted(os.listdir(str(POP))):
        pop, go = doc_pages(d)
        n = min(len(pop), len(go))
        if n == 0:
            continue
        ex = 0
        for i in range(n):
            try:
                a = np.asarray(Image.open(pop[i]).convert("L")).astype(int)
                b = np.asarray(Image.open(go[i]).convert("L")).astype(int)
            except Exception:
                continue
            if a.shape != b.shape:
                continue
            if int((np.abs(a - b) > 0).sum()) == 0:
                ex += 1
        total_pages += n
        exact_pages += ex
        per_doc.append((d, ex, n))

    print(f"corpus: {args.base}  ({POP})")
    print(f"{'doc':45s} {'exact':>7} {'/':>3} {'pages':>5}")
    print("-" * 65)
    for d, ex, n in per_doc:
        flag = "" if ex == n else "  <"
        print(f"{d:45s} {ex:7d}   {n:5d}{flag}")
    print("-" * 65)
    if total_pages:
        print(f"{'TOTAL':45s} {exact_pages:7d}   {total_pages:5d}  ({100*exact_pages/total_pages:.1f}%)")
    else:
        print("no pages compared")


if __name__ == "__main__":
    main()
