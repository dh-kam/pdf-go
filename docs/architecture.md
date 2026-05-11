# Architecture

Korean localization: [architecture.ko.md](architecture.ko.md).

## Overview

The project follows Clean Architecture. The domain layer owns the PDF concepts and rendering contracts, while infrastructure packages provide concrete parsers, decoders, renderers, and external integrations.

Dependency direction:

```text
Infrastructure -> Interface -> Use Case -> Domain
```

The domain layer must not depend on infrastructure packages.

## Layer Diagram

```mermaid
graph TB
    subgraph "Presentation"
        CMD["cmd/*"]
        CLI["CLI tools"]
    end

    subgraph "Interface"
        Controller["internal/interface/controller"]
        Presenter["internal/interface/presenter"]
    end

    subgraph "Use Case"
        ParserUC["internal/usecase/parser"]
        RendererUC["internal/usecase/renderer"]
        ExtractorUC["internal/usecase/extractor"]
    end

    subgraph "Domain"
        Entity["internal/domain/entity"]
        Repository["internal/domain/repository"]
        Canvas["internal/domain/canvas"]
        Font["internal/domain/font"]
        Content["internal/domain/content"]
        Annotation["internal/domain/annotation"]
        Renderer["internal/domain/renderer"]
        Cache["internal/domain/cache"]
    end

    subgraph "Infrastructure"
        PDF["internal/infrastructure/pdf"]
        FontImpl["internal/infrastructure/font"]
        ImageImpl["internal/infrastructure/image"]
        CanvasImpl["internal/infrastructure/canvas"]
        ContentImpl["internal/infrastructure/content"]
        AnnotationImpl["internal/infrastructure/annotation"]
        Splash["internal/infrastructure/splash"]
        CacheImpl["internal/infrastructure/cache"]
    end

    CMD --> Controller
    CLI --> Controller
    Controller --> ParserUC
    Controller --> RendererUC
    Controller --> ExtractorUC
    ParserUC --> Entity
    RendererUC --> Renderer
    ExtractorUC --> Content
    Repository --> PDF
    Font --> FontImpl
    Canvas --> CanvasImpl
    Content --> ContentImpl
    Annotation --> AnnotationImpl
    Renderer --> Splash
    Cache --> CacheImpl
```

## Package Responsibilities

- `cmd/`: CLI entry points and diagnostic tools.
- `pkg/pdf`: public API facade for document opening, page access, rendering, text extraction, and annotations.
- `internal/domain`: core model, contracts, rendering decisions, cache abstractions, and PDF concepts.
- `internal/usecase`: application workflows that coordinate parsing, rendering, and extraction.
- `internal/interface`: adapters between CLI/API inputs and use case boundaries.
- `internal/infrastructure`: concrete PDF parsing, font, image, canvas, splash, and cache implementations.
- `test/`: integration and end-to-end tests plus fixture data.

## Rendering Path

```mermaid
sequenceDiagram
    participant API as pkg/pdf
    participant Renderer as Domain Renderer
    participant Eval as Content Evaluator
    participant Splash as Splash Backend
    participant Canvas as Canvas/Image

    API->>Renderer: RenderPage(ctx, page, options)
    Renderer->>Eval: Evaluate page content stream
    Eval->>Splash: Draw paths, glyphs, images, masks
    Splash->>Canvas: Write raster pixels
    Canvas-->>API: image.Image
```

## Design Rules

- Keep struct fields private by default.
- Use constructor functions for invariants and required dependencies.
- Place small interfaces near the package that consumes them.
- Keep public APIs in `pkg/pdf` stable and documented.
- Gate optional native dependencies behind build tags.
- Preserve Poppler parity tests and comparison artifacts for rendering changes.

## Release Architecture

The release path is tag-driven:

```mermaid
flowchart LR
    Manual["Bump Release Tag workflow"] --> Tag["v0.9.0-upstream-YYYYMM.seq tag"]
    Direct["Manual git tag push"] --> Tag
    Tag --> Release["Release workflow"]
    Release --> Validate["make release-ci"]
    Validate --> Build["make release-build"]
    Build --> Package["package artifacts"]
    Package --> Publish["GitHub Release"]
```
