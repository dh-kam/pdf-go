# Go PDF Rendering Library - API 문서

## 개요

```go
import "github.com/dh-kam/pdf-go/pkg/pdf"
```

## 주요 타입

### Document

PDF 문서를 나타내는 최상위 타입입니다.

```go
type Document struct {
    // 내부 필드 (비공개)
}
```

#### 메서드

```go
// Open은 파일에서 PDF 문서를 엽니다
func Open(path string) (*Document, error)

// Close는 문서를 닫고 리소스를 해제합니다
func (d *Document) Close() error

// PageCount는 페이지 수를 반환합니다
func (d *Document) PageCount() (int, error)

// Page는 지정된 페이지를 반환합니다 (0-based)
func (d *Document) Page(index int) (*Page, error)

// Catalog는 문서 카탈로그를 반환합니다
func (d *Document) Catalog() *Dict

// Info는 문서 정보를 반환합니다
func (d *Document) Info() *Dict
```

#### 사용 예시

```go
// 문서 열기
doc, err := pdf.Open("document.pdf")
if err != nil {
    log.Fatal(err)
}
defer doc.Close()

// 페이지 수 확인
pageCount, err := doc.PageCount()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Pages: %d\n", pageCount)

// 첫 번째 페이지 가져오기
page, err := doc.Page(0)
if err != nil {
    log.Fatal(err)
}
```

### Page

PDF 페이지를 나타냅니다.

```go
type Page struct {
    // 내부 필드 (비공개)
}
```

#### 메서드

```go
// Index는 페이지 인덱스를 반환합니다 (0-based)
func (p *Page) Index() int

// MediaBox는 미디어 박스를 반환합니다 [x1, y1, x2, y2]
func (p *Page) MediaBox() [4]float64

// CropBox는 크롭 박스를 반환합니다
func (p *Page) CropBox() [4]float64

// Rotate는 페이지 회전 각도를 반환합니다 (0, 90, 180, 270)
func (p *Page) Rotate() int

// Width는 페이지 너비를 반환합니다 (포인트 단위)
func (p *Page) Width() float64

// Height는 페이지 높이를 반환합니다 (포인트 단위)
func (p *Page) Height() float64

// Contents는 페이지 콘텐츠 스트림을 반환합니다
func (p *Page) Contents() ([]Object, error)

// Annotations은 페이지 어노테이션을 반환합니다
func (p *Page) Annotations() ([]*Annotation, error)

// Resources는 페이지 리소스를 반환합니다
func (p *Page) Resources() (*Dict, error)
```

#### 사용 예시

```go
// 페이지 크기 확인
page, _ := doc.Page(0)
width := page.Width()
height := page.Height()
fmt.Printf("Size: %.2f x %.2f points\n", width, height)

// 페이지 회전 확인
rotation := page.Rotate()
fmt.Printf("Rotation: %d degrees\n", rotation)
```

### Renderer

페이지를 이미지로 렌더링합니다.

```go
type Renderer struct {
    // 내부 필드 (비공개)
}
```

#### 생성자

```go
// NewRenderer는 새 렌더러를 생성합니다
func NewRenderer(options RendererOptions) *Renderer

type RendererOptions struct {
    MaxWorkers int           // 최대 워커 수 (기본값: 4)
    CacheSize  int           // 캐시 크기 (기본값: 20)
    CacheTTL   time.Duration // 캐시 TTL (기본값: 10분)
}
```

#### 메서드

```go
// RenderPage는 단일 페이지를 렌더링합니다
func (r *Renderer) RenderPage(ctx context.Context, page *Page, options RenderOptions) (image.Image, error)

// RenderPages는 여러 페이지를 병렬로 렌더링합니다
func (r *Renderer) RenderPages(ctx context.Context, doc *Document, pageNumbers []int, options RenderOptions) <-chan RenderResult

// RenderAllPages는 모든 페이지를 병렬로 렌더링합니다
func (r *Renderer) RenderAllPages(ctx context.Context, doc *Document, options RenderOptions) <-chan RenderResult
```

#### RenderOptions

```go
type RenderOptions struct {
    DPI              float64     // 해상도 (기본값: 72)
    Scale            float64     // 배율 (기본값: 1.0)
    EnableCache      bool        // 캐시 사용 (기본값: true)
    BackgroundColor  color.Color // 배경색 (기본값: 흰색)
}
```

#### 사용 예시

```go
import (
    "context"
    "image"
    "image/png"
    "os"
)

// 렌더러 생성
renderer := pdf.NewRenderer(pdf.RendererOptions{
    MaxWorkers: 8,
    CacheSize:  50,
})

// 단일 페이지 렌더링
ctx := context.Background()
page, _ := doc.Page(0)

options := pdf.DefaultRenderOptions()
options.DPI = 150.0

img, err := renderer.RenderPage(ctx, page, options)
if err != nil {
    log.Fatal(err)
}

// 이미지 저장
f, _ := os.Create("page.png")
png.Encode(f, img)
f.Close()
```

#### 병렬 렌더링 예시

```go
// 여러 페이지 병렬 렌더링
pageNumbers := []int{0, 1, 2, 3, 4}
resultChan := renderer.RenderPages(ctx, doc, pageNumbers, options)

// 결과 수신
for result := range resultChan {
    if result.Error != nil {
        log.Printf("Error rendering page %d: %v", result.PageNum, result.Error)
        continue
    }

    // 이미지 저장
    filename := fmt.Sprintf("page_%d.png", result.PageNum)
    f, _ := os.Create(filename)
    png.Encode(f, result.Image)
    f.Close()
}
```

### TextExtractor

페이지에서 텍스트를 추출합니다.

```go
type TextExtractor struct {
    // 내부 필드 (비공개)
}
```

#### 메서드

```go
// ExtractText는 페이지에서 텍스트를 추출합니다
func (e *TextExtractor) ExtractText(page *Page) (string, error)

// ExtractTextWithLayout은 텍스트와 레이아웃 정보를 추출합니다
func (e *TextExtractor) ExtractTextWithLayout(page *Page) (*TextLayer, error)
```

#### TextLayer

```go
type TextLayer struct {
    TextItems []TextItem
}

type TextItem struct {
    Text      string
    X, Y      float64
    Width     float64
    Height    float64
    FontSize  float64
    FontName  string
}
```

#### 사용 예시

```go
extractor := pdf.NewTextExtractor()

// 단순 텍스트 추출
text, err := extractor.ExtractText(page)
if err != nil {
    log.Fatal(err)
}
fmt.Println(text)

// 레이아웃 정보와 함께 추출
layer, err := extractor.ExtractTextWithLayout(page)
for _, item := range layer.TextItems {
    fmt.Printf("(%s) at (%.2f, %.2f)\n", item.Text, item.X, item.Y)
}
```

### Annotation

PDF 어노테이션을 나타냅니다.

```go
type Annotation struct {
    // 내부 필드 (비공개)
}
```

#### 메서드

```go
// Type은 어노테이션 타입을 반환합니다
func (a *Annotation) Type() AnnotationType

// Rect는 어노테이션 위치를 반환합니다
func (a *Annotation) Rect() image.Rectangle

// Contents는 어노테이션 내용을 반환합니다
func (a *Annotation) Contents() string

// Subtype은 서브타입을 반환합니다
func (a *Annotation) Subtype() string
```

#### AnnotationType

```go
type AnnotationType int

const (
    AnnotationText       AnnotationType = iota // 텍스트
    AnnotationLink                             // 링크
    AnnotationFreeText                         // 자유 텍스트
    AnnotationLine                             // 선
    AnnotationSquare                           // 사각형
    AnnotationCircle                           // 원
    AnnotationHighlight                        // 하이라이트
    AnnotationUnderline                        // 밑줄
    AnnotationSquiggly                         // 물결밑줄
    AnnotationStrikeOut                        // 취소선
    AnnotationStamp                            // 스탬프
    AnnotationCaret                            // 캐럿
    AnnotationInk                              // 잉크
    AnnotationPopup                            // 팝업
    AnnotationFileAttachment                   // 파일 첨부
    AnnotationSound                            // 사운드
    AnnotationMovie                            // 동영상
    AnnotationWidget                           // 위젯 (폼 필드)
    AnnotationScreen                           // 스크린
    AnnotationPrinterMark                      // 프린터 마크
    AnnotationTrapNet                          // 트랩 네트워크
    AnnotationWatermark                        // 워터마크
    Annotation3D                               // 3D
    AnnotationRedact                           // 삭제 (블랙아웃)
)
```

#### LinkAnnotation

링크 어노테이션에 대한 추가 정보입니다.

```go
type LinkAnnotation struct {
    *Annotation
}

// URI는 링크 대상 URI를 반환합니다
func (l *LinkAnnotation) URI() string

// Dest는 링크 대상 페이지 위치를 반환합니다
func (l *LinkAnnotation) Dest() []interface{}
```

#### 사용 예시

```go
// 페이지 어노테이션 가져오기
annots, _ := page.Annotations()

for _, annot := range annots {
    switch annot.Type() {
    case pdf.AnnotationLink:
        if link, ok := annot.(*pdf.LinkAnnotation); ok {
            fmt.Printf("Link: %s\n", link.URI())
        }
    case pdf.AnnotationText:
        fmt.Printf("Text: %s\n", annot.Contents())
    }
}
```

## 캐시 관리

### CacheConfig

```go
type CacheConfig struct {
    MaxSize         int           // 최대 엔트리 수
    MaxBytes        int64         // 최대 바이트 수
    TTL             time.Duration // 시간 기반 만료
    CleanupInterval time.Duration // 클린업 간격
}
```

### 캐시 통계

```go
// RendererStats는 렌더러 통계를 반환합니다
func (r *Renderer) Stats() RendererStats

type RendererStats struct {
    RenderCount int64
    CacheHits   int64
    CacheMisses int64
}

// HitRate는 캐시 적중률을 반환합니다 (백분율)
func (s RendererStats) HitRate() float64
```

## 옵션

### DefaultRenderOptions

기본 렌더링 옵션을 반환합니다.

```go
func DefaultRenderOptions() RenderOptions
```

### DefaultRendererOptions

기본 렌더러 옵션을 반환합니다.

```go
func DefaultRendererOptions() RendererOptions
```

## 에러 처리

### 에러 타입

```go
type ErrorType int

const (
    ErrTypeInvalid    ErrorType = iota // 잘못된 데이터
    ErrTypeNotFound                     // 리소스 없음
    ErrTypeEncryption                  // 암호화
    ErrTypeFont                        // 폰트
    ErrTypeRendering                   // 렌더링
)
```

### 에러 래핑

```go
// 에러 래핑
if err != nil {
    return nil, fmt.Errorf("failed to render page: %w", err)
}

// 에러 타입 확인
var pdfErr *pdf.PDFError
if errors.As(err, &pdfErr) {
    switch pdfErr.Type {
    case pdf.ErrTypeFont:
        // 폰트 에러 처리
    case pdf.ErrTypeRendering:
        // 렌더링 에러 처리
    }
}
```

## 완전한 예시

### 문서 열기 및 페이지 렌더링

```go
package main

import (
    "context"
    "fmt"
    "image/png"
    "log"
    "os"

    "github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
    // 문서 열기
    doc, err := pdf.Open("document.pdf")
    if err != nil {
        log.Fatal(err)
    }
    defer doc.Close()

    // 페이지 수 확인
    pageCount, _ := doc.PageCount()
    fmt.Printf("Total pages: %d\n", pageCount)

    // 렌더러 생성
    renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())

    // 첫 번째 페이지 렌더링
    ctx := context.Background()
    page, _ := doc.Page(0)

    options := pdf.DefaultRenderOptions()
    options.DPI = 150.0

    img, err := renderer.RenderPage(ctx, page, options)
    if err != nil {
        log.Fatal(err)
    }

    // 이미지 저장
    f, _ := os.Create("page.png")
    defer f.Close()
    png.Encode(f, img)

    // 캐시 통계 출력
    stats := renderer.Stats()
    fmt.Printf("Cache hit rate: %.1f%%\n", stats.HitRate())
}
```

### 모든 페이지 렌더링

```go
func renderAllPages(doc *pdf.Document, outputDir string) error {
    renderer := pdf.NewRenderer(pdf.RendererOptions{
        MaxWorkers: 8,
        CacheSize:  50,
    })

    ctx := context.Background()
    options := pdf.DefaultRenderOptions()
    options.DPI = 150.0

    // 모든 페이지 병렬 렌더링
    resultChan := renderer.RenderAllPages(ctx, doc, options)

    // 결과 처리
    for result := range resultChan {
        if result.Error != nil {
            log.Printf("Error rendering page %d: %v", result.PageNum, result.Error)
            continue
        }

        // 이미지 저장
        filename := fmt.Sprintf("%s/page_%d.png", outputDir, result.PageNum)
        f, err := os.Create(filename)
        if err != nil {
            log.Printf("Error creating file: %v", err)
            continue
        }

        if err := png.Encode(f, result.Image); err != nil {
            log.Printf("Error encoding image: %v", err)
            f.Close()
            continue
        }
        f.Close()
    }

    return nil
}
```

### 텍스트 추출

```go
func extractText(doc *pdf.Document) error {
    extractor := pdf.NewTextExtractor()

    pageCount, _ := doc.PageCount()
    for i := 0; i < pageCount; i++ {
        page, _ := doc.Page(i)
        text, err := extractor.ExtractText(page)
        if err != nil {
            log.Printf("Error extracting text from page %d: %v", i, err)
            continue
        }

        fmt.Printf("=== Page %d ===\n", i)
        fmt.Println(text)
        fmt.Println()
    }

    return nil
}
```

### 어노테이션 처리

```go
func processAnnotations(page *pdf.Page) error {
    annots, err := page.Annotations()
    if err != nil {
        return err
    }

    for _, annot := range annots {
        rect := annot.Rect()
        fmt.Printf("Annotation at (%d, %d, %d, %d)\n",
            rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

        switch annot.Type() {
        case pdf.AnnotationLink:
            if link, ok := annot.(*pdf.LinkAnnotation); ok {
                fmt.Printf("  Link: %s\n", link.URI())
            }
        case pdf.AnnotationText:
            fmt.Printf("  Note: %s\n", annot.Contents())
        }
    }

    return nil
}
```
