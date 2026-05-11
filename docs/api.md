# API

Korean localization: [api.ko.md](api.ko.md).

## Import

```go
import "github.com/dh-kam/pdf-go/pkg/pdf"
```

## Document

`Document` is the top-level public type for an opened PDF document.

```go
doc, err := pdf.Open("document.pdf")
if err != nil {
    log.Fatal(err)
}
defer doc.Close()
```

Common methods:

```go
func Open(path string) (*Document, error)
func (d *Document) Close() error
func (d *Document) PageCount() (int, error)
func (d *Document) Page(index int) (*Page, error)
func (d *Document) Catalog() *Dict
func (d *Document) Info() *Dict
```

## Page

`Page` represents a single PDF page. Page indexes are zero-based.

Common methods:

```go
func (p *Page) Index() int
func (p *Page) Width() float64
func (p *Page) Height() float64
func (p *Page) MediaBox() Rect
func (p *Page) CropBox() Rect
func (p *Page) Rotate() int
```

## Rendering

Create a renderer and render a page to an image:

```go
renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())

options := pdf.DefaultRenderOptions()
options.DPI = 150

page, err := doc.Page(0)
if err != nil {
    log.Fatal(err)
}

img, err := renderer.RenderPage(context.Background(), page, options)
if err != nil {
    log.Fatal(err)
}
```

Render all pages concurrently:

```go
renderer := pdf.NewRenderer(pdf.RendererOptions{
    MaxWorkers: 8,
})

results := renderer.RenderAllPages(context.Background(), doc, pdf.DefaultRenderOptions())
for result := range results {
    if result.Error != nil {
        log.Printf("page %d: %v", result.PageNum, result.Error)
        continue
    }
    saveImage(result.PageNum, result.Image)
}
```

## Text Extraction

```go
text, err := doc.GetPageText(0)
if err != nil {
    log.Fatal(err)
}
```

Layout-aware extraction is available through the public text APIs in `pkg/pdf`.

## Annotations

Annotations expose type, rectangle, contents, and subtype information. Link annotations also expose URI or destination data when available.

Typical flow:

```go
annotations, err := doc.GetPageAnnotations(0)
if err != nil {
    log.Fatal(err)
}

for _, annotation := range annotations {
    fmt.Println(annotation.Type(), annotation.Rect(), annotation.Contents())
}
```

## Options

Use defaults unless a caller needs explicit control:

```go
renderOptions := pdf.DefaultRenderOptions()
rendererOptions := pdf.DefaultRendererOptions()
```

Common rendering controls include DPI, background handling, worker count, cache configuration, and optional rendering features.

## Errors

APIs return wrapped errors with contextual information. Callers should propagate errors unless they can make a local recovery decision.

```go
if err != nil {
    return fmt.Errorf("render page %d: %w", pageIndex, err)
}
```

## Compatibility Notes

Public API stability is concentrated in `pkg/pdf`. Internal packages are private implementation details and can change without compatibility guarantees.
