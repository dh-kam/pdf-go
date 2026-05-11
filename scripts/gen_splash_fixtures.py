#!/usr/bin/env python3
"""
Splash golden corpus PDF fixture generator.

Hand-rolled minimal PDFs (no reportlab) so the byte stream is fully
deterministic. Each fixture isolates ONE Splash primitive from the
SP5 test-strategy doc (§2). Page box [0 0 100 60], rendered at 150 DPI
by `pdftoppm 24.02.0` to produce 208×125 PNGs.

Usage:
    gen_splash_fixtures.py --out testdata/splash_golden/pdfs/
    gen_splash_fixtures.py --manifest > testdata/splash_golden/MANIFEST.tsv
"""

from __future__ import annotations

import argparse
import hashlib
import os
import sys
from pathlib import Path
from typing import Callable, List, Tuple

# ---------------------------------------------------------------------------
# Minimal PDF builder
# ---------------------------------------------------------------------------

PDF_HEADER = b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"
CREATION_DATE = b"D:20260427000000Z"


class PDFBuilder:
    """Tiny PDF builder with explicit object IDs and deterministic output."""

    def __init__(self, fixture_id: int, primitive: str):
        self.objects: List[bytes] = []  # objects[i-1] is object i body
        self.fixture_id = fixture_id
        self.primitive = primitive
        # Reserve slots; we'll fill them by index later.
        self._reserved = 0

    def reserve(self) -> int:
        self._reserved += 1
        self.objects.append(b"")  # placeholder
        return self._reserved

    def set(self, obj_id: int, body: bytes) -> None:
        self.objects[obj_id - 1] = body

    def add(self, body: bytes) -> int:
        self.objects.append(body)
        self._reserved = len(self.objects)
        return self._reserved

    def build(self) -> bytes:
        out = bytearray()
        # SP5 fixture-# comment per spec
        comment = f"% SP5 fixture #{self.fixture_id:02d} - {self.primitive}\n".encode("ascii")
        out += PDF_HEADER
        out += comment
        offsets: List[int] = []
        for i, body in enumerate(self.objects, start=1):
            offsets.append(len(out))
            out += f"{i} 0 obj\n".encode("ascii")
            out += body
            if not body.endswith(b"\n"):
                out += b"\n"
            out += b"endobj\n"
        xref_pos = len(out)
        n = len(self.objects)
        out += f"xref\n0 {n + 1}\n".encode("ascii")
        out += b"0000000000 65535 f \n"
        for off in offsets:
            out += f"{off:010d} 00000 n \n".encode("ascii")
        out += b"trailer\n"
        out += f"<< /Size {n + 1} /Root 1 0 R /Info {n} 0 R\n".encode("ascii")
        # Deterministic /ID built from fixture_id (no timestamp).
        seed = f"splash-golden-{self.fixture_id:02d}".encode("ascii")
        idhex = hashlib.md5(seed).hexdigest().upper().encode("ascii")
        out += b"   /ID [<" + idhex + b"> <" + idhex + b">]\n"
        out += b">>\nstartxref\n"
        out += f"{xref_pos}\n".encode("ascii")
        out += b"%%EOF\n"
        return bytes(out)


def page_skeleton(b: PDFBuilder, content: bytes, resources: bytes = b"<< /ProcSet [/PDF /Text /ImageB /ImageC /ImageI] >>", extras: bytes = b"") -> None:
    """Build standard 4-object skeleton: catalog, pages, page, contents.

    Caller MUST call finalize_info(b) after adding any aux objects (fonts,
    images, shading), so that /Info ends up as the last object and aux
    object IDs (typically 5+) match `5 0 R` references in resources.
    """
    cat_id = b.add(b"<< /Type /Catalog /Pages 2 0 R >>")
    assert cat_id == 1
    pages_id = b.add(b"<< /Type /Pages /Count 1 /Kids [3 0 R] >>")
    assert pages_id == 2
    page_id = b.add(
        b"<< /Type /Page /Parent 2 0 R "
        b"/MediaBox [0 0 100 60] "
        b"/Contents 4 0 R "
        b"/Resources " + resources + b" "
        + extras +
        b">>"
    )
    assert page_id == 3
    stream_body = b"<< /Length " + str(len(content)).encode("ascii") + b" >>\nstream\n" + content + b"\nendstream"
    cs_id = b.add(stream_body)
    assert cs_id == 4


def finalize_info(b: PDFBuilder) -> None:
    info = (
        b"<< /Producer (splash-golden-gen) "
        b"/CreationDate (" + CREATION_DATE + b") "
        b"/ModDate (" + CREATION_DATE + b") >>"
    )
    b.add(info)


# ---------------------------------------------------------------------------
# Fixture builders
# ---------------------------------------------------------------------------

# Each fixture function returns (filename, primitive, pdf_bytes).
# Page coords: PDF y is up; box [0 0 100 60].

def fx_01_rect_solid_aligned() -> Tuple[str, str, bytes]:
    b = PDFBuilder(1, "rect-fill")
    # 100x40 black rect aligned at integer coords (0,10) -> (100,50)
    content = b"q\n0 0 0 rg\n0 10 100 40 re\nf\nQ\n"
    page_skeleton(b, content)
    finalize_info(b)
    return "01_rect_solid_aligned.pdf", "rect-fill", b.build()


def fx_02_rect_solid_subpx() -> Tuple[str, str, bytes]:
    b = PDFBuilder(2, "rect-fill-subpx")
    # 60x30 rect at (10.5, 20.5)
    content = b"q\n0 0 0 rg\n10.5 20.5 60 30 re\nf\nQ\n"
    page_skeleton(b, content)
    finalize_info(b)
    return "02_rect_solid_subpx.pdf", "rect-fill-subpx", b.build()


def fx_03_rect_eo_hole() -> Tuple[str, str, bytes]:
    b = PDFBuilder(3, "even-odd")
    # outer 80x50 rect (10,5)->(90,55) CW, inner 30x20 rect (35,20)->(65,40) reverse
    content = (
        b"q\n0 0 0 rg\n"
        b"10 5 m 90 5 l 90 55 l 10 55 l h\n"
        b"35 20 m 35 40 l 65 40 l 65 20 l h\n"
        b"f*\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "03_rect_eo_hole.pdf", "even-odd", b.build()


def fx_04_aa_diag_edge() -> Tuple[str, str, bytes]:
    b = PDFBuilder(4, "aa-edge")
    # 1° tilted long thin rect. Use cm to rotate. cos(1°)≈0.9998, sin(1°)≈0.01745
    content = (
        b"q\n0 0 0 rg\n"
        b"1 0 0 1 10 30 cm\n"
        b"0.9998 0.01745 -0.01745 0.9998 0 0 cm\n"
        b"0 -1 80 2 re\nf\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "04_aa_diag_edge.pdf", "aa-edge", b.build()


def fx_05_thin_hline() -> Tuple[str, str, bytes]:
    b = PDFBuilder(5, "thin-hstroke")
    content = b"q\n0 0 0 RG\n0.7 w\n10 30 m 90 30 l S\nQ\n"
    page_skeleton(b, content)
    finalize_info(b)
    return "05_thin_hline_1px.pdf", "thin-hstroke", b.build()


def fx_06_thin_vline() -> Tuple[str, str, bytes]:
    b = PDFBuilder(6, "thin-vstroke")
    content = b"q\n0 0 0 RG\n0.7 w\n50 5 m 50 55 l S\nQ\n"
    page_skeleton(b, content)
    finalize_info(b)
    return "06_thin_vline_1px.pdf", "thin-vstroke", b.build()


def fx_07_miter_join() -> Tuple[str, str, bytes]:
    b = PDFBuilder(7, "miter-join")
    # two 4-px strokes meeting at 45°. /Miter join (default = 0 == miter)
    content = (
        b"q\n0 0 0 RG\n4 w\n0 j\n10 M\n"
        b"20 20 m 50 50 l 80 20 l S\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "07_miter_join_45.pdf", "miter-join", b.build()


def fx_08_bevel_join() -> Tuple[str, str, bytes]:
    b = PDFBuilder(8, "bevel-join")
    # /Bevel = line join 2
    content = (
        b"q\n0 0 0 RG\n4 w\n2 j\n"
        b"20 20 m 50 50 l 80 20 l S\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "08_bevel_join_45.pdf", "bevel-join", b.build()


def fx_09_round_cap_join() -> Tuple[str, str, bytes]:
    b = PDFBuilder(9, "round-cap-join")
    # round cap (1) + round join (1)
    content = (
        b"q\n0 0 0 RG\n3 w\n1 J\n1 j\n"
        b"20 20 m 50 40 l 80 20 l S\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "09_round_cap_round_join.pdf", "round-cap-join", b.build()


def fx_10_dash_pattern() -> Tuple[str, str, bytes]:
    b = PDFBuilder(10, "dash")
    content = (
        b"q\n0 0 0 RG\n2 w\n"
        b"[8 4] 0 d\n"
        b"5 30 m 95 30 l S\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "10_dash_pattern.pdf", "dash", b.build()


def fx_15_glyph_mono() -> Tuple[str, str, bytes]:
    b = PDFBuilder(15, "glyph-mono")
    # Use Helvetica (built-in) at integer pos. Spec calls for Type3 'A',
    # but Type3 hand-roll is heavy; Helvetica integer-pos still exercises
    # mono blit rounding.
    content = (
        b"q\nBT\n"
        b"/F1 24 Tf\n"
        b"10 20 Td\n"
        b"(A) Tj\n"
        b"ET\nQ\n"
    )
    resources = (
        b"<< /Font << /F1 5 0 R >> "
        b"/ProcSet [/PDF /Text] >>"
    )
    page_skeleton(b, content, resources)
    b.add(
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
    )
    finalize_info(b)
    return "15_glyph_mono.pdf", "glyph-mono", b.build()


def fx_16_glyph_aa_subpx() -> Tuple[str, str, bytes]:
    b = PDFBuilder(16, "glyph-aa-subpx")
    # Times-Roman 'g' at fractional x.
    content = (
        b"q\nBT\n"
        b"/F1 20 Tf\n"
        b"10.5 20 Td\n"
        b"(g) Tj\n"
        b"ET\nQ\n"
    )
    resources = b"<< /Font << /F1 5 0 R >> /ProcSet [/PDF /Text] >>"
    page_skeleton(b, content, resources)
    b.add(
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman /Encoding /WinAnsiEncoding >>"
    )
    finalize_info(b)
    return "16_glyph_aa_subpx.pdf", "glyph-aa-subpx", b.build()


def fx_17_glyph_blend_lsb() -> Tuple[str, str, bytes]:
    b = PDFBuilder(17, "glyph-blend")
    # Two overlapping glyphs at near-same position, second drawn 50% gray.
    content = (
        b"q\nBT\n"
        b"/F1 28 Tf\n"
        b"0 0 0 rg\n"
        b"10 20 Td\n"
        b"(O) Tj\n"
        b"0.5 0.5 0.5 rg\n"
        b"4 0 Td\n"
        b"(O) Tj\n"
        b"ET\nQ\n"
    )
    resources = b"<< /Font << /F1 5 0 R >> /ProcSet [/PDF /Text] >>"
    page_skeleton(b, content, resources)
    b.add(
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
    )
    finalize_info(b)
    return "17_glyph_blend_lsb.pdf", "glyph-blend", b.build()


def fx_20_image_bilinear_up() -> Tuple[str, str, bytes]:
    b = PDFBuilder(20, "image-up")
    # 4x4 RGB raw image, upsampled to fill 64x64 area on page
    pixels = bytearray()
    for y in range(4):
        for x in range(4):
            pixels += bytes([(x * 80) & 0xFF, (y * 80) & 0xFF, ((x ^ y) * 80) & 0xFF])
    pix = bytes(pixels)
    content = (
        b"q\n"
        b"64 0 0 64 18 -2 cm\n"
        b"/Im1 Do\nQ\n"
    )
    resources = b"<< /XObject << /Im1 5 0 R >> /ProcSet [/PDF /ImageC] >>"
    page_skeleton(b, content, resources)
    img = (
        b"<< /Type /XObject /Subtype /Image /Width 4 /Height 4 "
        b"/ColorSpace /DeviceRGB /BitsPerComponent 8 "
        b"/Length " + str(len(pix)).encode("ascii") + b" >>\nstream\n"
        + pix + b"\nendstream"
    )
    b.add(img)
    finalize_info(b)
    return "20_image_bilinear_up.pdf", "image-up", b.build()


def fx_21_image_bilinear_down() -> Tuple[str, str, bytes]:
    b = PDFBuilder(21, "image-down")
    # 32x32 grayscale raw -> rendered into 16x16 page region (downscale).
    # Keep small to stay <4 KB. Use a simple gradient.
    w, h = 32, 32
    pix = bytearray()
    for y in range(h):
        for x in range(w):
            pix.append(((x + y) * 4) & 0xFF)
    pix = bytes(pix)
    content = (
        b"q\n"
        b"32 0 0 32 34 14 cm\n"
        b"/Im1 Do\nQ\n"
    )
    resources = b"<< /XObject << /Im1 5 0 R >> /ProcSet [/PDF /ImageB] >>"
    page_skeleton(b, content, resources)
    img = (
        b"<< /Type /XObject /Subtype /Image /Width 32 /Height 32 "
        b"/ColorSpace /DeviceGray /BitsPerComponent 8 "
        b"/Length " + str(len(pix)).encode("ascii") + b" >>\nstream\n"
        + pix + b"\nendstream"
    )
    b.add(img)
    finalize_info(b)
    return "21_image_bilinear_down.pdf", "image-down", b.build()


def fx_22_image_indexed() -> Tuple[str, str, bytes]:
    b = PDFBuilder(22, "image-indexed")
    # 16-color palette image: 16x16 sized, each pixel = (x ^ y) & 0xF
    palette = bytearray()
    for i in range(16):
        # simple gradient palette
        palette += bytes([(i * 17) & 0xFF, ((i * 5) % 256), ((255 - i * 17) & 0xFF)])
    palette = bytes(palette)
    pix = bytearray()
    for y in range(16):
        for x in range(16):
            pix.append((x ^ y) & 0x0F)
    pix = bytes(pix)
    content = (
        b"q\n"
        b"32 0 0 32 34 14 cm\n"
        b"/Im1 5 0 R Do\nQ\n"
    )
    # Note: reference image XObject by name in resources, draw via /Im1 Do.
    content = b"q\n32 0 0 32 34 14 cm\n/Im1 Do\nQ\n"
    resources = b"<< /XObject << /Im1 5 0 R >> /ProcSet [/PDF /ImageI] >>"
    page_skeleton(b, content, resources)
    pal_hex = palette.hex().upper().encode("ascii")
    img = (
        b"<< /Type /XObject /Subtype /Image /Width 16 /Height 16 "
        b"/ColorSpace [/Indexed /DeviceRGB 15 <" + pal_hex + b">] "
        b"/BitsPerComponent 8 "
        b"/Length " + str(len(pix)).encode("ascii") + b" >>\nstream\n"
        + pix + b"\nendstream"
    )
    b.add(img)
    finalize_info(b)
    return "22_image_indexed.pdf", "image-indexed", b.build()


def fx_23_image_cmyk() -> Tuple[str, str, bytes]:
    b = PDFBuilder(23, "image-cmyk")
    # 16x16 CMYK gradient (kept small to stay <4 KB)
    w, h = 16, 16
    pix = bytearray()
    for y in range(h):
        for x in range(w):
            c = (x * 16) & 0xFF
            m = (y * 16) & 0xFF
            yc = ((x + y) * 8) & 0xFF
            k = 0
            pix += bytes([c, m, yc, k])
    pix = bytes(pix)
    content = b"q\n32 0 0 32 34 14 cm\n/Im1 Do\nQ\n"
    resources = b"<< /XObject << /Im1 5 0 R >> /ProcSet [/PDF /ImageC] >>"
    page_skeleton(b, content, resources)
    img = (
        b"<< /Type /XObject /Subtype /Image /Width 16 /Height 16 "
        b"/ColorSpace /DeviceCMYK /BitsPerComponent 8 "
        b"/Length " + str(len(pix)).encode("ascii") + b" >>\nstream\n"
        + pix + b"\nendstream"
    )
    b.add(img)
    finalize_info(b)
    return "23_image_cmyk.pdf", "image-cmyk", b.build()


def fx_24_clip_intersect() -> Tuple[str, str, bytes]:
    b = PDFBuilder(24, "clip-nested")
    # Outer clip: 80x40 rect. Nested clip: triangular path.
    # Then fill with a wide black rect; only the intersection should appear.
    content = (
        b"q\n"
        b"10 10 80 40 re W n\n"
        b"q\n"
        b"30 5 m 70 55 l 70 5 l h W n\n"
        b"0 0 0 rg\n"
        b"0 0 100 60 re f\n"
        b"Q\nQ\n"
    )
    page_skeleton(b, content)
    finalize_info(b)
    return "24_clip_intersect.pdf", "clip-nested", b.build()


def fx_25_link_border() -> Tuple[str, str, bytes]:
    b = PDFBuilder(25, "link-border")
    # /Link annotation with /Border [0 0 1] (1pt black border)
    # Page references annot via /Annots in extras.
    content = b"q\n0.8 0.8 0.8 rg\n20 20 60 20 re f\nQ\n"
    resources = b"<< /ProcSet [/PDF] >>"
    # Need /Annots reference on page
    cat_id = b.add(b"<< /Type /Catalog /Pages 2 0 R >>")
    assert cat_id == 1
    pages_id = b.add(b"<< /Type /Pages /Count 1 /Kids [3 0 R] >>")
    page_obj = (
        b"<< /Type /Page /Parent 2 0 R "
        b"/MediaBox [0 0 100 60] "
        b"/Contents 4 0 R "
        b"/Resources " + resources + b" "
        b"/Annots [5 0 R] "
        b">>"
    )
    page_id = b.add(page_obj)
    cs_id = b.add(b"<< /Length " + str(len(content)).encode("ascii") + b" >>\nstream\n" + content + b"\nendstream")
    annot = (
        b"<< /Type /Annot /Subtype /Link "
        b"/Rect [20 20 80 40] "
        b"/Border [0 0 1] "
        b"/C [0 0 0] "
        b"/A << /S /URI /URI (https://example.invalid/) >> "
        b">>"
    )
    b.add(annot)
    info = (
        b"<< /Producer (splash-golden-gen) "
        b"/CreationDate (" + CREATION_DATE + b") "
        b"/ModDate (" + CREATION_DATE + b") >>"
    )
    b.add(info)
    return "25_link_border.pdf", "link-border", b.build()


# ---------------------------------------------------------------------------
# Stretch fixtures (best-effort)
# ---------------------------------------------------------------------------

def fx_11_axial_grad_h() -> Tuple[str, str, bytes]:
    b = PDFBuilder(11, "axial-h")
    # axial shading, black->white horizontal across page
    content = b"q\n10 20 80 20 re W n\n/Sh1 sh\nQ\n"
    resources = b"<< /Shading << /Sh1 5 0 R >> /ProcSet [/PDF] >>"
    page_skeleton(b, content, resources)
    # Axial shading: ShadingType 2, function = exponential 1
    b.add(
        b"<< /ShadingType 2 /ColorSpace /DeviceRGB "
        b"/Coords [10 30 90 30] "
        b"/Domain [0 1] "
        b"/Function << /FunctionType 2 /Domain [0 1] "
        b"/C0 [0 0 0] /C1 [1 1 1] /N 1 >> "
        b"/Extend [true true] >>"
    )
    finalize_info(b)
    return "11_axial_grad_h.pdf", "axial-h", b.build()


def fx_25b_dash_zero_phase() -> Tuple[str, str, bytes]:
    b = PDFBuilder(30, "dash-zero-phase")
    # dashed path with phase=0 starting on a "skip" via [4 8] dash + phase 4
    content = b"q\n0 0 0 RG\n2 w\n[8 4] 0 d\n5 15 m 95 15 l S\n[4 8] 0 d\n5 45 m 95 45 l S\nQ\n"
    page_skeleton(b, content)
    finalize_info(b)
    return "30_dash_zero_phase.pdf", "dash-zero-phase", b.build()


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------

FIXTURES: List[Callable[[], Tuple[str, str, bytes]]] = [
    fx_01_rect_solid_aligned,
    fx_02_rect_solid_subpx,
    fx_03_rect_eo_hole,
    fx_04_aa_diag_edge,
    fx_05_thin_hline,
    fx_06_thin_vline,
    fx_07_miter_join,
    fx_08_bevel_join,
    fx_09_round_cap_join,
    fx_10_dash_pattern,
    fx_11_axial_grad_h,
    fx_15_glyph_mono,
    fx_16_glyph_aa_subpx,
    fx_17_glyph_blend_lsb,
    fx_20_image_bilinear_up,
    fx_21_image_bilinear_down,
    fx_22_image_indexed,
    fx_23_image_cmyk,
    fx_24_clip_intersect,
    fx_25_link_border,
    fx_25b_dash_zero_phase,
]


# Fixtures NOT shipped (documented as TODO):
TODO_FIXTURES = [
    ("12_axial_grad_diag.pdf", "axial-diag", "TODO: 45-deg axial shading"),
    ("13_radial_grad.pdf", "radial", "TODO: ShadingType 3 radial disc"),
    ("14_tiling_pattern.pdf", "tiling-pattern", "TODO: Type 1 tiling pattern (8x8 checker)"),
    ("18_softmask_lum.pdf", "softmask-lum", "TODO: Form XObject + /Group /Luminosity"),
    ("19_softmask_alpha.pdf", "softmask-alpha", "TODO: Form XObject + /Group /Alpha"),
    ("26_clip_path.pdf", "clip-path", "TODO: single complex W path"),
    ("27_glyph_type1.pdf", "glyph-type1", "TODO: Type1 font sub-pixel"),
    ("28_image_a85.pdf", "image-a85", "TODO: ASCII85-encoded image stream"),
    ("29_softmask_invert.pdf", "softmask-invert", "TODO: soft mask /TR /Identity"),
]


# ---------------------------------------------------------------------------
# Manifest
# ---------------------------------------------------------------------------

def write_pdfs(out_dir: Path) -> List[Tuple[str, str, bytes]]:
    out_dir.mkdir(parents=True, exist_ok=True)
    results = []
    for fn in FIXTURES:
        name, prim, data = fn()
        (out_dir / name).write_bytes(data)
        results.append((name, prim, data))
    return results


def emit_manifest(pdfs_dir: Path, expected_dir: Path) -> str:
    rows: List[str] = ["fixture\tprimitive\tbytes\tsha256"]
    # PDFs (sorted)
    for p in sorted(pdfs_dir.glob("*.pdf")):
        data = p.read_bytes()
        sha = hashlib.sha256(data).hexdigest()
        rows.append(f"pdfs/{p.name}\t{_lookup_primitive(p.name)}\t{len(data)}\t{sha}")
    # Expected PNGs (sorted)
    for p in sorted(expected_dir.glob("*.png")):
        data = p.read_bytes()
        sha = hashlib.sha256(data).hexdigest()
        rows.append(f"expected/{p.name}\t{_lookup_primitive(p.name)}-expected\t{len(data)}\t{sha}")
    # TODO entries
    for name, prim, note in TODO_FIXTURES:
        rows.append(f"# TODO\tpdfs/{name}\t{prim}\t{note}")
    return "\n".join(rows) + "\n"


def _lookup_primitive(filename: str) -> str:
    stem = filename.rsplit(".", 1)[0]
    for fn in FIXTURES:
        name, prim, _ = fn()
        if name.rsplit(".", 1)[0] == stem:
            return prim
    return "unknown"


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main(argv: List[str]) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, help="Output dir for PDFs")
    parser.add_argument("--manifest", action="store_true", help="Emit MANIFEST.tsv to stdout")
    parser.add_argument("--root", type=Path, default=Path("testdata/splash_golden"),
                        help="Splash golden root (for --manifest mode)")
    args = parser.parse_args(argv)

    if args.out:
        results = write_pdfs(args.out)
        for name, prim, data in results:
            print(f"  wrote {name} ({len(data)} bytes)", file=sys.stderr)
        return 0

    if args.manifest:
        sys.stdout.write(emit_manifest(args.root / "pdfs", args.root / "expected"))
        return 0

    parser.print_help()
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
