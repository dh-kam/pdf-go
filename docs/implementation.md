# Implementation Status

Korean localization: [implementation.ko.md](implementation.ko.md).

## Overview

This document tracks the current implementation status and the major technical areas that affect parser correctness, rendering parity, and release readiness.

## Current Status

- Public package: `github.com/dh-kam/pdf-go/pkg/pdf`.
- Default CI path: no-CGo build with `nojpx,nojbig2,nofreetype,nocairo` tags.
- Release gate: `make release-ci`.
- Release artifact build: `make release-package RELEASE_VERSION=v0.9.0-poppler24-02-0-202605.1`.
- Primary rendering reference: Poppler raster output.

## Parser Implementation

Implemented areas:

- Indirect object parsing.
- Classic XRef tables.
- XRef streams.
- Incremental update chains through `/Prev`.
- Object stream handling.
- Stream filter decoding for common PDF filters.
- Document catalog, page tree, and metadata traversal.

Important parser contracts:

- Indirect references must be parsed as `N N R` and not as standalone integers.
- Object parsing at explicit offsets must skip `N N obj` headers before parsing object bodies.
- XRef stream entries must handle free, uncompressed, and compressed object entries.

## Rendering Implementation

Implemented areas:

- Graphics state stack.
- Path fill and stroke.
- Text positioning and glyph rendering.
- Image drawing with sampler policy controls.
- Clipping paths.
- Patterns and XObjects.
- Splash-backed rasterization paths for Poppler parity.

Current accuracy work focuses on exact pixel matching against Poppler. Known active areas include stroke AA overlap, tiling-pattern residuals, image resampling phase, CTM handling for transformed images, and color-space conversion.

## Font Implementation

Implemented areas:

- Standard 14 fonts.
- Type1 parsing.
- TrueType/OpenType support.
- CFF/Type1C support.
- CID-keyed fonts and CMap parsing.
- Font subsetting scaffolding.

## Image Implementation

Implemented areas:

- JPEG decoding.
- Image mask handling.
- Color-space conversion.
- Sampler and phase selection.
- Optional advanced decoders behind build tags.

The project also evaluates external decoder parity when native-library behavior affects Poppler matching.

## CLI and Diagnostics

The `cmd/` tree includes production CLI tools and diagnostic helpers:

- `pdfrender`
- `pdfinfo`
- `pdftext`
- `pdfcompare`
- pixel-diff and render-analysis tools

Diagnostic tools are kept buildable in the no-CGo CI path unless a native dependency is explicitly required.

## CI and Release Implementation

GitHub Actions provides:

- Pull request and main-branch CI.
- No-CGo validation, vet, tests, builds, and vulnerability checks.
- Manual release-train tag bumping through workflow dispatch.
- Tag-push release artifact generation and GitHub Release publishing.

Local release commands:

```bash
make release-ci
make release-build
make release-package RELEASE_VERSION=v0.9.0-poppler24-02-0-202605.1
```

## Remaining Work

- Continue Exact100 Poppler parity improvements.
- Keep regression cases for every rendering fix.
- Expand integration coverage for transformed images and sampler phase contracts.
- Keep release documentation and workflow behavior synchronized with the Makefile.
