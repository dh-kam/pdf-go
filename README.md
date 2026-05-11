# Go PDF Rendering Library

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Go PDF is a PDF parsing, rendering, and text extraction library written in Go. The project ports core PDF.js behavior into a server-side Go implementation and uses Poppler-compatible rendering work as the primary raster accuracy target.

Korean documentation is available in [README.ko.md](README.ko.md).

## Features

- Pure-Go default build path with optional CGo integrations behind build tags.
- Clean Architecture layout with domain, use case, interface, and infrastructure layers.
- PDF parsing for classic XRef tables, XRef streams, and incremental update chains.
- Rendering for pages, paths, text, images, clipping, patterns, and XObjects.
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

## CLI Build

```bash
make build-no-cgo
```

The default no-CGo build writes binaries under `build/linux-amd64/nocgo/` and legacy command aliases under `bin/`.

Release packages can be built locally with:

```bash
make release-package RELEASE_VERSION=v0.0.0-dev
```

## CI and Release

GitHub Actions runs CI on pushes and pull requests. The release flow is tag-driven:

1. Run the `Bump Release Tag` workflow with `dry_run=false`, or push an annotated `v0.9.0-<upstream-slug>-YYYYMM.seq` tag.
2. The `Release` workflow validates the tag, builds release binaries, packages artifacts, and creates the GitHub Release.

The release tag format is `v<project-semver>-<upstream-slug>-YYYYMM.seq`. For the current Poppler-backed render baseline, the default example is `v0.9.0-poppler24-02-0-202605.1`.

Useful local gates:

```bash
make release-ci
make release-build
make release-package RELEASE_VERSION=v0.9.0-poppler24-02-0-202605.1
```

## Documentation

- [Architecture](docs/architecture.md)
- [API](docs/api.md)
- [Features](docs/features.md)
- [Implementation Status](docs/implementation.md)
- [Release Notes](docs/release_0.9.0-poppler24-02-0-202605.1.md)
- [Rendering Progress](docs/design/progress.md)

Default documentation files are written in English. Korean-localized documents use the `.ko.md` suffix.
