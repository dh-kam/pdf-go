# Progress

Korean localization: [progress.ko.md](progress.ko.md).

## Overview

This document tracks development progress and design decisions for the Go PDF rendering library.

Project goal: provide a server-side Go implementation for parsing, rendering, and extracting PDF content without browser dependencies.

Reference projects:

- [Mozilla PDF.js](https://github.com/mozilla/pdf.js)
- Poppler for raster parity investigation

Architecture: Clean Architecture with domain, use case, interface, and infrastructure layers.

## v0.9.0-poppler24-02-0-202605.1 Status

Target window: May 2026.

Current status: core features, performance gates, release CI, and regression automation are in place. Release publication remains pending.

## Completed Capabilities

| Category | Capability | Status |
| --- | --- | --- |
| Parsing | PDF 1.4-1.7 support | Done |
| Parsing | XRef table parsing | Done |
| Parsing | XRef stream parsing | Done |
| Parsing | Incremental update chains | Done |
| Parsing | Object caching | Done |
| Fonts | Standard 14 fonts | Done |
| Fonts | Type1 fonts | Done |
| Fonts | TrueType/OpenType fonts | Done |
| Fonts | CFF/Type1C fonts | Done |
| Fonts | CID-keyed CJK fonts | Done |
| Fonts | Binary CMap parsing | Done |
| Images | JPEG decoding | Done |
| Images | Optional JPEG2000 decoding | Done |
| Images | Optional JBIG2 decoding | Done |
| Images | Image masks | Done |
| Images | Color-space conversion | Done |
| Content | Graphics state management | Done |
| Content | Path construction | Done |
| Content | Text positioning | Done |
| Content | Pattern support | Done |
| Content | XObject handling | Done |
| Rendering | Path rendering | Done |
| Rendering | Text rendering | Done |
| Rendering | Image rendering | Done |
| Rendering | Clipping paths | Done |
| Annotations | Link annotations | Done |
| Annotations | Text annotations | Done |
| Annotations | Widget annotations | Done |
| Annotations | Appearance streams | Done |
| Encryption | Password-based decryption | Done |
| Encryption | RC4 decryption | Done |
| Encryption | AES decryption | Done |
| Performance | Parallel page rendering | Done |
| Performance | Object pooling | Done |
| Performance | LRU caching | Done |
| Tooling | `pdfinfo`, `pdftext`, `pdfrender` | Done |
| Tooling | Poppler comparison HTML | Done |
| Release | GitHub Actions CI | Done |
| Release | Manual release-train bump workflow | Done |
| Release | Tag-driven release workflow | Done |

## Recent Checkpoints

- `make porting-complete-plus-goal98` passed in the historical gate.
- The pure RGB `zero vs positive subpixel vertical offset` rasterization contract was documented.
- Sample Poppler comparison HTML automation was added with Poppler, ours, and XOR views.
- Long-running failed-document recheck automation was added.
- A nightly diff gate was added.
- The no-CGo core coverage gate was added with an 80% target.
- Missing public-symbol godoc count reached zero in the project scan.
- Image-decoder performance improvements were applied.
- Release workflows were added for CI, release-train bumping, and tag-driven GitHub Releases.

## Pending Release Tasks

- [ ] Create the release tag.
- [ ] Confirm the GitHub Release.
- [ ] Confirm Go module proxy publication.

## Design Decisions

### Clean Architecture

Decision: use strict layer separation.

Reasoning:

- Keeps domain logic independent from parser, canvas, and external-library details.
- Improves testability through small interfaces.
- Makes renderer parity work easier to isolate by implementation package.

### Interface Segregation

Decision: prefer small consumer-owned interfaces.

Reasoning:

- Avoids forcing callers to depend on methods they do not use.
- Keeps mocks and regression tests focused.
- Allows alternate renderer and decoder implementations.

### Encapsulation

Decision: keep struct fields private by default and expose behavior through constructors and methods.

Reasoning:

- Protects invariants.
- Allows internal representation changes.
- Prevents invalid partially initialized values.

### Error Handling

Decision: propagate wrapped errors with context.

Reasoning:

- Preserves root causes.
- Makes parser and renderer failures traceable to document, page, or content operation context.

### Build Tags

Decision: use build tags for optional native integrations.

Reasoning:

- Keeps the default release path portable.
- Allows advanced decoders or system libraries to be enabled explicitly.
- Keeps CI predictable on GitHub-hosted runners.

## Technical Debt

- Continue Poppler Exact100 parity work.
- Add regression fixtures for each fixed mismatch class.
- Expand transformed-image tests for rotation, skew, shear, and phase handling.
- Keep release documentation synchronized with GitHub Actions and Makefile behavior.
