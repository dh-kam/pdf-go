# Go PDF Rendering Library

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Pure Go](https://img.shields.io/badge/Pure%20Go-CGO__ENABLED%3D0-00ADD8?style=flat)](https://golang.org)
[![WebAssembly](https://img.shields.io/badge/WebAssembly-Live%20Demo-654FF0?style=flat&logo=webassembly&logoColor=white)](https://dh-kam.github.io/pdf-go/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Go PDF is a PDF parsing, rendering, and text extraction library written in **pure Go**. The project ports core PDF.js behavior into a server-side Go implementation and uses Poppler-compatible rendering as the primary raster accuracy target: the tracked regression corpus renders **pixel-identical** to Poppler 24.02 (`pdftoppm`) at 150 dpi.

Documentation: [English](README.md) | [한국어](README.ko.md)

## Why pdf-go

- **Pure Go, zero system dependencies.** The default build is `CGO_ENABLED=0` end to end — parser, rasterizer (a Splash port), and font engines (Type1/CFF/TrueType interpreters and a FreeType-compatible glyph rasterizer) are all Go. No Poppler, MuPDF, Cairo, or FreeType installation required, and cross-compiling is a plain `GOOS`/`GOARCH` switch.
- **Runs in the browser via WebAssembly.** The same rendering engine compiles with `GOOS=js GOARCH=wasm` and renders PDFs fully client-side — no server round-trip, no native plugin. Try the [live demo](https://dh-kam.github.io/pdf-go/).
- **Poppler-grade output.** Rendering is validated pixel-by-pixel against Poppler's `pdftoppm` on a multi-document regression corpus, page-for-page byte-exact.

## Features

- Pure-Go default build path with optional CGo integrations behind build tags.
- WebAssembly build target with a browser demo (document open, per-page render, text APIs).
- Clean Architecture layout with domain, use case, interface, and infrastructure layers.
- PDF parsing for classic XRef tables, XRef streams, and incremental update chains.
- Rendering for pages, paths, text, images, clipping, patterns, shadings, transparency groups, and XObjects.
- Font support for Standard 14, Type1, TrueType/OpenType, CFF/Type1C, and CID-keyed fonts.
- Image support for JPEG, PNG, masks, color conversion, and optional advanced decoders.
- Text extraction APIs with layout-aware helpers.
- Annotation support for links, text annotations, widgets, and appearance streams.
- CLI tools for rendering, metadata, text extraction, pixel comparison, and corpus analysis.

## Installation

```bash
go get github.com/dh-kam/pdf-go/pkg/pdf
```

## Quick Start

### Open a Document

```go
package main

import (
    "fmt"
    "log"

    "github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
    doc, err := pdf.Open("document.pdf")
    if err != nil {
        log.Fatal(err)
    }
    defer doc.Close()

    pageCount, err := doc.PageCount()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Pages: %d\n", pageCount)
}
```

### Render a Page

```go
package main

import (
    "context"
    "image/png"
    "log"
    "os"

    "github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
    doc, err := pdf.Open("document.pdf")
    if err != nil {
        log.Fatal(err)
    }
    defer doc.Close()

    page, err := doc.Page(0)
    if err != nil {
        log.Fatal(err)
    }

    renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
    options := pdf.DefaultRenderOptions()
    options.DPI = 150

    img, err := renderer.RenderPage(context.Background(), page, options)
    if err != nil {
        log.Fatal(err)
    }

    out, err := os.Create("page.png")
    if err != nil {
        log.Fatal(err)
    }
    defer out.Close()

    if err := png.Encode(out, img); err != nil {
        log.Fatal(err)
    }
}
```

### Extract Text

```go
text, err := doc.GetPageText(0)
if err != nil {
    log.Fatal(err)
}
fmt.Println(text)
```

## WebAssembly (Browser)

Because the whole engine is pure Go, it compiles unchanged to WebAssembly and renders PDFs entirely in the browser — documents never leave the client.

**Live demo:** https://dh-kam.github.io/pdf-go/ (deployed by the `[05] Build pages and deploy` workflow)

Build the WASM bundle locally:

```bash
make build-wasm
# emits build/js-wasm/default/{pdfwasm.wasm, wasm_exec.js, index.html, main.js, pdf_worker.js}
```

The module registers a `pdfgo` global with a small JavaScript API:

```js
const doc = pdfgo.openDocument(new Uint8Array(buffer), {});   // -> { id, pageCount, backend, ok }
const page = pdfgo.renderPage(doc.id, 0, { dpi: 150 });       // -> { width, height, data (RGBA), ok }
pdfgo.closeDocument(doc.id);
```

The bundled demo (`examples/browser/`) runs rendering inside a Web Worker (`pdf_worker.js`) so the UI stays responsive on large documents.

## CLI Build

```bash
make build-no-cgo
```

The default no-CGo build writes binaries under `build/linux-amd64/default/` and legacy command aliases under `bin/`.

Release packages can be built locally with:

```bash
make release-package RELEASE_VERSION=v0.0.0-dev
```

## CI and Release

GitHub Actions runs CI on pushes and pull requests. The release flow is tag-driven:

1. Run the `Bump Release Tag` workflow with `dry_run=false`, or push an annotated `v0.9.0-<upstream-slug>-YYYYMM.seq` tag.
2. The `Release` workflow validates the tag, builds release binaries, packages artifacts, and creates the GitHub Release.

The release tag format is `v<project-semver>-<upstream-slug>-YYYYMM.seq`. For the current Poppler-backed render baseline, the default example is `v0.9.0-poppler24-02-0-202606.1`.

Useful local gates:

```bash
make release-ci
make release-build
make release-package RELEASE_VERSION=v0.9.0-poppler24-02-0-202606.1
```

## Coverage

The current no-CGo coverage snapshot from `make coverage-core-no-cgo` is:

- Overall no-CGo coverage: `61.4%`.
- Core no-CGo coverage: `68.3%`.
- Core coverage target: `80.0%`.

The core gate is below target and currently fails at the threshold check after generating coverage profiles.

## Documentation

- [Korean README](README.ko.md)
- [Architecture](docs/architecture.md)
- [API](docs/api.md)
- [Features](docs/features.md)
- [Implementation Status](docs/implementation.md)
- [Release Notes](docs/release_0.9.0-poppler24-02-0-202605.1.md)
- [Rendering Progress](docs/design/progress.md)

Default documentation files are written in English. Korean-localized documents use the `.ko.md` suffix.
