# Features

Korean localization: [features.ko.md](features.ko.md).

## Core Library

- Open PDF files through `pkg/pdf`.
- Read page count, page metrics, document metadata, and catalog information.
- Render pages to Go `image.Image` values.
- Extract page text and layout-aware text information.
- Read annotations, links, widgets, and appearance streams.

## PDF Parsing

- Classic XRef table parsing.
- XRef stream parsing.
- Incremental update support through `/Prev` chains.
- Object caching and indirect-reference resolution.
- Stream filter handling for common PDF compression paths.

## Rendering

- Page rendering with configurable DPI.
- Path fill and stroke rendering.
- Text rendering with font resolution and glyph mapping.
- Image rendering with masks, color conversion, and sampler policy controls.
- Clipping paths, patterns, and XObject handling.
- Splash-backed rendering paths used to improve Poppler pixel parity.
- Pure Go browser WebAssembly rendering through `cmd/pdfwasm`.

## Browser WebAssembly

- `cmd/pdfwasm` exposes a JavaScript API on `globalThis.pdfgo`.
- `openDocument(Uint8Array, options)` loads a PDF from browser file bytes and returns a document id plus page count.
- `renderPage(documentID, pageIndex, options)` renders a zero-based page index and returns width, height, and RGBA `Uint8ClampedArray` pixels.
- `pageInfo(documentID, pageIndex)` returns page metrics, media box, crop box, and rotation.
- `closeDocument(documentID)` releases the document from the WASM-side store.
- `make build-wasm` builds `pdfwasm.wasm`, copies Go's `wasm_exec.js`, and stages the browser demo under `build/js-wasm/default/`.
- The browser example runs rendering inside a Web Worker so large pages do not block the UI thread.
- The browser example defaults to `splash`; `image-canvas` remains selectable as a diagnostic fallback.
- Browser worker and WASM operations write progress logs to the page log panel and browser console for CDP inspection.

## Fonts

- Standard 14 fonts.
- Type1 fonts.
- TrueType and OpenType fonts.
- CFF and Type1C fonts.
- CID-keyed fonts for CJK documents.
- CMap parsing, including binary CMap handling.

## Images

- JPEG decoding.
- PNG image handling.
- Image masks.
- Device color spaces and color conversion paths.
- Optional advanced image decoding through build-tagged integrations.

## CLI Tools

- `pdfrender`: render PDF pages to image files.
- `pdfinfo`: inspect document metadata and page information.
- `pdftext`: extract text.
- `pdfcompare`: generate Poppler-vs-ours comparison HTML.
- Analysis and pixel-diff helper tools under `cmd/`.

## Build Variants

- Default no-CGo build for portable CI and release artifacts.
- Release artifacts are produced for Linux, macOS, and Windows on amd64 and arm64.
- Browser WebAssembly artifacts are produced with `GOOS=js`, `GOARCH=wasm`, and `CGO_ENABLED=0`.

## Accuracy Workflow

The renderer is developed against Poppler parity. The project keeps tools for corpus rendering, exact pixel comparison, XOR visualization, and bottleneck classification so regressions can be diagnosed page by page.
