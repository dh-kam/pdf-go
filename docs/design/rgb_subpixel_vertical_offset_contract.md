# RGB Subpixel Vertical Offset Contract

Korean localization: [rgb_subpixel_vertical_offset_contract.ko.md](rgb_subpixel_vertical_offset_contract.ko.md).

## Overview

This document records the `zero vs positive subpixel vertical offset` contract observed in the pure RGB synthetic lane. It explains where the current renderer and Poppler/PDF.js-style rasterization rules diverge.

The conclusion is not a universal statement about the whole sample corpus. It is a design note based on the current synthetic probe and the sample-free pure RGB lane.

## Problem Statement

The probes repeatedly showed these facts:

- In the pure RGB small-page lane, the `transparent edge + white backdrop` reference is closer to Poppler than the current renderer.
- In sample-like large-page geometry, the current renderer is usually closer.
- In a narrow positive subpixel band near `y-centered` placement on large pages, the transparent-edge behavior becomes closer again.
- Binary search narrowed the synthetic transition point to approximately:

```text
410.000000000000 < threshold <= 410.0000009536743
```

In practical terms, `y offset == 0` and `y offset > 0` behave like different rasterization contracts.

## Observed Contract

Core contract:

- `exact integer-aligned y`: the current renderer is closer to Poppler.
- `positive subpixel y`: the `transparent edge + white backdrop` reference is closer to Poppler.

This is locked by `TestSyntheticRGBTransparentEdgeZeroVsPositiveSubpixelYOffsetContract` in [synthetic_render_parity_test.go](/workspace/pdf-reader/go-pdf/test/integration/pdf/synthetic_render_parity_test.go).

## Explicit Mode Surface

The implemented `experimental-rgb-transparent-edge-upscale-v1` mode affects a narrower surface than the broad reference hypothesis.

| Signature | Geometry | Legacy vs Experimental |
| --- | --- | --- |
| `small_page_positive_subpixel_y` | `24x24 page`, `16x16 src`, `20x20 dst`, `(1.5, 1.5)` | experimental better |
| `large_page_zero_y_offset` | `595x842 page`, `16x16 src`, `20x20 dst`, `(100, 410.0)` | exact match |
| `large_page_positive_y_non_centered` | `595x842 page`, `16x16 src`, `20x20 dst`, `(100, 100)` | exact match |
| `large_page_positive_y_centered_band` | `595x842 page`, `16x16 src`, `20x20 dst`, `(100, 411)` | exact match |

This is locked by `TestSyntheticRGBTransparentEdgeExperimentalModeSignatureMatrixProbeAgainstPoppler`.

Tiny-lane behavior also depends on content family:

| Pattern family | Geometry | Legacy vs Experimental |
| --- | --- | --- |
| `flat` | `24x24 page`, `16x16 src`, `20x20 dst`, `(1.5, 1.5)` | legacy better |
| `gradient` | `24x24 page`, `16x16 src`, `20x20 dst`, `(1.5, 1.5)` | experimental better |
| `checker` | `24x24 page`, `16x16 src`, `20x20 dst`, `(1.5, 1.5)` | experimental better |
| `tiled_identity` | `24x24 page`, `16x16 src`, `20x20 dst`, `(1.5, 1.5)` | experimental better |

This is locked by `TestSyntheticRGBTransparentEdgeExperimentalModePatternFamilyProbeAgainstPoppler`.

## Interpretation

The mismatch is better explained as a vertical source-edge treatment difference between exact-grid placement and positive subpixel placement. It is less likely to be a broad geometry, footprint, DPI, or coarse-phase issue.

The current explicit mode does not imply a broad draw-path effect. In the current implementation, most large-page tiny-footprint signatures are pixel-identical between legacy and experimental behavior. The effect is effectively limited to the small-page positive-subpixel-y tiny synthetic lane.

## Renderer Difference

The current synthetic path behaves closer to a clamped opaque source edge for samples outside the source bounds. This is closer to Poppler for integer-aligned placement, but leaves a stronger edge contribution than Poppler for positive subpixel vertical offsets.

The transparent-edge reference treats the outside contribution as transparent before compositing on a white backdrop. That better matches Poppler for the observed positive-subpixel RGB lane.

## Engineering Rule

Do not generalize this mode to all images or all transformed draws without a Poppler-backed fixture. Any broadening must include:

- A synthetic probe.
- A Poppler oracle.
- A corpus comparison.
- A regression check for pages that previously matched exactly.
