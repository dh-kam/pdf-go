#!/usr/bin/env bash
#
# Regenerate the Splash golden corpus.
#
# Hard-fails on Poppler version mismatch — the goldens are version-locked
# to pdftoppm 24.02.0 (per test/testdata/splash_golden/POPPLER_VERSION).
#
# Run from go-pdf/ root:
#   bash scripts/regen_splash_golden.sh
#
set -euo pipefail

# Locate go-pdf root (the dir containing scripts/ and test/testdata/).
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
GO_PDF_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
cd "$GO_PDF_ROOT"

GOLDEN_DIR="test/testdata/splash_golden"
PDFS_DIR="$GOLDEN_DIR/pdfs"
EXPECTED_DIR="$GOLDEN_DIR/expected"
VERSION_FILE="$GOLDEN_DIR/POPPLER_VERSION"

if [[ ! -f "$VERSION_FILE" ]]; then
    echo "ERROR: $VERSION_FILE not found" >&2
    exit 1
fi
expected_version=$(tr -d '[:space:]' < "$VERSION_FILE")

if ! command -v pdftoppm >/dev/null 2>&1; then
    echo "ERROR: pdftoppm not found in PATH" >&2
    exit 1
fi

actual_line=$(pdftoppm -v 2>&1 | head -1)
if [[ "$actual_line" != *"$expected_version"* ]]; then
    echo "ERROR: pdftoppm version mismatch." >&2
    echo "       expected: $expected_version" >&2
    echo "       got:      $actual_line" >&2
    exit 1
fi

mkdir -p "$PDFS_DIR" "$EXPECTED_DIR"

# 1. Generate PDFs (deterministic).
echo "==> Generating PDF fixtures"
rm -f "$PDFS_DIR"/*.pdf
python3 scripts/gen_splash_fixtures.py --out "$PDFS_DIR/"

# 2. Render each via pdftoppm and rename, then strip tIME chunks.
echo "==> Rendering reference PNGs at 150 DPI"
rm -f "$EXPECTED_DIR"/*.png
for pdf in "$PDFS_DIR"/*.pdf; do
    stem=$(basename "$pdf" .pdf)
    pdftoppm -r 150 -png -aa yes -aaVector yes "$pdf" "$EXPECTED_DIR/$stem"
    # pdftoppm appends -1 to the output stem for single-page docs.
    if [[ -f "$EXPECTED_DIR/$stem-1.png" ]]; then
        mv "$EXPECTED_DIR/$stem-1.png" "$EXPECTED_DIR/$stem.png"
    fi
done

# 3. Strip tIME chunks for byte-stable hashes (pngcrush optional fallback).
echo "==> Stripping tIME chunks"
if command -v pngcrush >/dev/null 2>&1; then
    for png in "$EXPECTED_DIR"/*.png; do
        pngcrush -q -rem tIME "$png" "$png.tmp"
        mv "$png.tmp" "$png"
    done
else
    python3 - "$EXPECTED_DIR" <<'PY'
import struct, sys, zlib
from pathlib import Path

png_dir = Path(sys.argv[1])
SIG = b"\x89PNG\r\n\x1a\n"

def strip_tIME(data: bytes) -> bytes:
    if data[:8] != SIG:
        raise ValueError("not a PNG")
    out = bytearray(SIG)
    i = 8
    while i < len(data):
        length = struct.unpack(">I", data[i:i+4])[0]
        ctype = data[i+4:i+8]
        chunk = data[i:i+8+length+4]
        i += 8 + length + 4
        if ctype == b"tIME":
            continue
        out += chunk
    return bytes(out)

for png in sorted(png_dir.glob("*.png")):
    raw = png.read_bytes()
    out = strip_tIME(raw)
    if out != raw:
        png.write_bytes(out)
PY
fi

# 4. Update MANIFEST.tsv.
echo "==> Writing MANIFEST.tsv"
python3 scripts/gen_splash_fixtures.py --manifest --root "$GOLDEN_DIR" > "$GOLDEN_DIR/MANIFEST.tsv"

# 5. Summarize.
pdf_count=$(ls "$PDFS_DIR" | wc -l)
png_count=$(ls "$EXPECTED_DIR" | wc -l)
total_bytes=$(du -bc "$PDFS_DIR" "$EXPECTED_DIR" | tail -1 | awk '{print $1}')

echo "==> Done."
echo "    pdfs:     $pdf_count"
echo "    pngs:     $png_count"
echo "    on-disk:  $total_bytes bytes"
