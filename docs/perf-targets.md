# PDF Rendering Performance Targets

This document tracks current rendering bottlenecks, rejected probes, and execution plans for reducing wall time and heap pressure without changing Poppler parity.

## Current Baseline

Baseline artifact: `tmp/codex_perf/slowpage_continue_20260614/mmap_verify`

The current rows come from full page-profile verification with `CGO_ENABLED=0 PDF_FREETYPE_GO=1`, Splash backend, one worker, and temporary comparison output that can use `--png-compression none` when images are not retained.

| Target | Canvas | Page-Profile Wall | MemTotalAlloc | MaxRSS | Current Status |
| --- | ---: | ---: | ---: | ---: | --- |
| `doc_100.pdf` page 4 | 150 DPI | `382ms` | `88.60MB` | `70.56MB` | Current slow leader. |
| `doc_070.pdf` page 9 | 300 DPI | `282ms` | `74.26MB` | `141.92MB` | Image/bitmap-heavy leader. |
| `doc_070.pdf` page 11 | 300 DPI | `217ms` | `63.40MB` | `142.08MB` | Secondary image/JPEG case. |

## Latest Profile Findings

Artifact: `tmp/codex_perf/slowpage_continue_20260614/profile_p4_mmap`

### `doc_100.pdf` Page 4

The mmap pass removed the whole input PDF read from the Go heap. Heap allocation is now distributed across rendering and parsing work rather than dominated by one safe hotspot:

| Source | Alloc-Space | Notes |
| --- | ---: | --- |
| `splash.NewBitmap` | `14.78MB` | Fixed page/group bitmap allocation. |
| `bytes.growSlice` | `8.50MB` | Mostly decoded stream and output buffers. |
| `entity.(*Dict).Set` | `8.00MB` | Many small parsed dictionaries and resource dictionaries. |
| `runtime.allocm` | `6.51MB` | Runtime/thread allocation visible in the focused profile. |
| Scanner/numeric operands | `~2..3MB` | Clip/intersection storage and content numeric objects remain visible. |
| Raw RGB matte path | `2.04MB` | Canvas image materialization still appears on this page. |
| `os.readFileContents` | `0.55MB` | No longer the whole PDF input; remaining reads are secondary resources. |

### `doc_070.pdf` Page 9

This page is structurally image-heavy. The latest pass reduces the per-process input copy with `mmap`, but the main cost is still fixed 300-DPI bitmap and image pipeline work:

| Source | Alloc-Space | Notes |
| --- | ---: | --- |
| `splash.NewBitmap` | high | Fixed 300-DPI page/group bitmaps remain structural. |
| Decoded image and stream buffers | high | JPEG/Flate decoded data and scaling buffers remain necessary before deeper streaming decode work. |
| PNG/flate output | medium | Default exact output can still be compression-bound; compare-only temporary output uses `none`. |
| Parser/scanner storage | medium | Remaining content parsing and clip/intersection storage. |
| Whole-file input read | removed | `pdfrender` maps the PDF input on Unix and falls back to `os.ReadFile` only when needed. |

## Rejected Probes

| Probe | Result |
| --- | --- |
| Medium Form XObject Flate hint `rawLen*6 -> rawLen*10` | PNG-identical, but p4 allocation/RSS did not improve and live heap became noisier. Reverted. |
| `Real(-1/0/1)` singleton cache | PNG-identical, but p4 allocation was flat/slightly worse and p9/p11 stayed within noise. Reverted. |
| Wider `Real` cache, slice-backed small dictionaries, parser dict capacity changes | Previously verified as flat, mixed, or regressive in focused or broad runs. |
| Keyword interning without input mmap | Correct and retained, but by itself only produced small allocation wins and noisy wall/RSS movement. |

## Bottleneck Backlog

| Priority | Area | Execution Plan | Owner Role | Validation Gate |
| --- | --- | --- | --- | --- |
| P0 | Streaming/downscale image decode | Pass destination scale and clip information into image decode so large DCT/Flate images can decode or emit only needed rows/pixels. Keep current exact renderer path as fallback. | Renderer + image pipeline engineer | PNG byte-identical on p9/p11 first, then full `doc070_300` report-identical. |
| P0 | Splash bitmap lifetime reuse | Reuse page/group bitmap storage where ownership is clear, especially temporary transparency groups and soft-mask/group surfaces. Avoid pooling page outputs that increase retained heap. | Splash engineer | Focused RSS must not regress; full page profile must keep report rows identical. |
| P1 | Raw output comparison path | Let `pdfcompare` compare raw RGB/RGBA rows directly to avoid PNG roundtrip when `-keep-images=false`, keeping PNG output only for artifacts. | CLI/tooling engineer | Report metrics identical to PNG compare on `doc100_150` and `doc070_300`. |
| P1 | Scanner storage reduction | Compress or reuse scanner intersection/count buffers beyond the current int32 storage. Target pages with wide clips and many AA rows. | Rasterization engineer | Focused pages must stay PNG-identical; broad GeoTopo/doc100 profile must improve allocation without wall regression. |
| P2 | Parser/dictionary allocation model | Replace hot parsed operator dictionaries with specialized compact forms only where key set is known, not a generic small-dict replacement. | Parser/core engineer | Must beat current map path on p4 and stay flat or better on broad random corpus. |
| P2 | Persistent input workers | `pdfrender` now avoids a Go-heap input copy with Unix `mmap`. The remaining per-page chunk cost is process startup, parsing, and resource setup, so the next step is persistent workers or shared parsed state. | CLI/runtime engineer | Compare output must be unchanged; page-profile wall/alloc should improve on chunk-size 1 runs. |

## Monthly Report Template

```markdown
# Rendering Performance Report: YYYY-MM

## Summary
- Baseline artifact:
- New artifact:
- Total pages compared:
- Exactness/report changes:
- Net wall-time change:
- Net allocation change:
- MaxRSS change:

## Top Slow Pages
| Rank | PDF | Page | DPI | Wall | MemTotalAlloc | MaxRSS | Primary Bottleneck |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |

## Accepted Changes
| Change | Focused Impact | Broad Impact | Parity Evidence |
| --- | --- | --- | --- |

## Rejected Probes
| Probe | Reason Rejected | Artifact |
| --- | --- | --- |

## Next Month Plan
| Priority | Target | Owner | Validation Gate |
| --- | --- | --- | --- |
```
