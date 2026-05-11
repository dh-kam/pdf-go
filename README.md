# Go PDF Rendering Library

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

순수 Go로 작성된 PDF 렌더링 라이브러리입니다. PDF.js를 포팅하여 브라우저 의존성 없이 PDF 문서를 파싱, 렌더링, 텍스트 추출 기능을 제공합니다.

## 특징

- **순수 Go 구현**: CGo 의존성 없음
- **Clean Architecture**: 유지보수 가능한 모듈형 구조
- **병렬 처리**: 다중 페이지 동시 렌더링
- **캐싱**: LRU 캐시로 성능 최적화
- **폰트 지원**: Standard 14, Type1, TrueType, CID-keyed (CJK)
- **이미지 지원**: JPEG, PNG, 이미지 마스크
- **텍스트 추출**: 텍스트와 레이아웃 정보 추출
- **어노테이션**: 링크, 텍스트, 위젯 어노테이션 지원

## 설치

```bash
go get github.com/dh-kam/pdf-go/pkg/pdf
```

## 빠른 시작

### PDF 문서 열기

```go
package main

import (
    "fmt"
    "log"
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
    pageCount, err := doc.PageCount()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("페이지 수: %d\n", pageCount)
}
```

### 페이지 렌더링

```go
import (
    "context"
    "image/png"
    "os"
    "github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
    doc, _ := pdf.Open("document.pdf")
    defer doc.Close()

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
}
```

### 병렬 페이지 렌더링

```go
func renderAll(doc *pdf.Document) {
    renderer := pdf.NewRenderer(pdf.RendererOptions{
        MaxWorkers: 8,
    })

    ctx := context.Background()
    options := pdf.DefaultRenderOptions()

    // 모든 페이지 병렬 렌더링
    resultChan := renderer.RenderAllPages(ctx, doc, options)

    // 결과 수신
    for result := range resultChan {
        if result.Error != nil {
            log.Printf("Page %d error: %v", result.PageNum, result.Error)
            continue
        }

        // 결과 처리
        saveImage(result.PageNum, result.Image)
    }
}
```

### 텍스트 추출

```go
func extractText(doc *pdf.Document) {
    extractor := pdf.NewTextExtractor()

    pageCount, _ := doc.PageCount()
    for i := 0; i < pageCount; i++ {
        page, _ := doc.Page(i)
        text, err := extractor.ExtractText(page)
        if err != nil {
            continue
        }

        fmt.Printf("=== Page %d ===\n%s\n\n", i, text)
    }
}
```

## 프로젝트 구조

```
go-pdf/
├── cmd/                    # CLI 도구
│   ├── pdfinfo/           # PDF 정보 도구
│   ├── pdftext/           # 텍스트 추출 도구
│   └── pdfrender/         # 렌더링 도구
├── internal/              # 내부 패키지
│   ├── domain/            # 도메인 계층 (인터페이스)
│   │   ├── entity/        # 엔티티
│   │   ├── canvas/        # 캔버스 인터페이스
│   │   ├── font/          # 폰트 인터페이스
│   │   ├── content/       # 콘텐츠 인터페이스
│   │   ├── annotation/    # 어노테이션 인터페이스
│   │   ├── renderer/      # 렌더러 인터페이스
│   │   └── cache/         # 캐시 인터페이스
│   └── infrastructure/    # 인프라 계층 (구현)
│       ├── pdf/           # PDF 파싱
│       ├── font/          # 폰트 처리
│       ├── image/         # 이미지 디코딩
│       ├── canvas/        # 캔버스 구현
│       ├── content/       # 콘텐츠 평가
│       ├── annotation/    # 어노테이션 처리
│       ├── renderer/      # 렌더러 구현
│       └── cache/         # 캐시 구현
├── pkg/                   # 공용 API
│   └── pdf/              # 공개 패키지
├── test/                  # 테스트
│   ├── unit/             # 단위 테스트
│   ├── integration/      # 통합 테스트
│   └── testdata/         # 테스트 데이터
└── docs/                  # 문서
    ├── architecture.md   # 아키텍처
    └── api.md           # API 문서
```

## 지원하는 PDF 기능

### 구현 완료 ✅

| 카테고리 | 기능 |
|---------|------|
| **파싱** | PDF 1.4-1.7, XRef 테이블, XRef 스트림, 증분 업데이트 |
| **폰트** | Standard 14, Type1, TrueType/OpenType, CFF/Type1C, CID-keyed (CJK) |
| **이미지** | JPEG, JPEG2000 (선택적 CGo), JBIG2 (선택적 CGo), 이미지 마스크, 컬러 공간 변환 |
| **콘텐츠** | 그래픽 상태, 경로, 텍스트, 색상 공간, 패턴, 셰이딩, XObject |
| **어노테이션** | 링크, 텍스트, 위젯, 외관 스트림 |
| **렌더링** | 경로, 텍스트, 이미지, 클리핑, 투명도 |
| **성능** | LRU 캐싱, 객체 풀링, 병렬 렌더링 |
| **스트림** | FlateDecode, ASCIIHexDecode, ASCII85Decode, LZWDecode, RunLengthDecode, CCITTFaxDecode, Predictor |
| **압축** | Predictor (모든 타입 지원), PNG Predictor |

### 부분 구현 🚧

| 카테고리 | 상태 |
|---------|------|
| **폰트** | GPOS/GSUB (선택적), 폰트 서브셋팅 |
| **보안** | 암호화 (RC4/AES), 권한 플래그 |

## 빌드 태그

### JPEG2000 지원

```bash
# OpenJPEG가 설치된 경우
go build

# OpenJPEG 없이 빌드
go build -tags=nojpx
```

### JBIG2 지원

```bash
# jbig2dec가 설치된 경우
go build

# jbig2dec 없이 빌드
go build -tags=nojbig2
```

### 모두 비활성화

```bash
go build -tags='nojpx,nojbig2'
```

## 성능

### 벤치마크

| 작업 | 성능 |
|------|------|
| 단일 페이지 렌더링 (72 DPI) | ~100ms |
| 단일 페이지 렌더링 (150 DPI) | ~250ms |
| 캐시 적중 시간 | <1ms |
| 텍스트 추출 | ~50ms |

### 최적화

- **LRU 캐시**: 자주 접근하는 페이지 캐싱
- **객체 풀**: 바이트 버퍼 재사용
- **병렬 처리**: 다중 페이지 동시 렌더링
- **읽기 락**: 동시 읽기 지원 (RWMutex)
- **스트림 필터 벤치마크**: 모든 압축 필터에 대한 성능 측정

### 테스트 커버리지

- **단위 테스트**: 80% 이상 커버리지
- **통합 테스트**: 다양한 PDF 버전 및 기능 테스트
- **벤치마크**: 스트림 필터 및 렌더링 성능 측정

```bash
# 단위 테스트 실행
go test ./...

# 커버리지 확인
go test ./... -cover

# 벤치마크 실행
go test -bench=. ./internal/infrastructure/pdf/stream/
```

## CLI 도구

### pdfinfo

PDF 문서 정보를 표시합니다.

```bash
go run cmd/pdfinfo/main.go document.pdf
```

### pdftext

텍스트를 추출합니다.

```bash
# 일반 텍스트
go run cmd/pdftext/main.go document.txt document.pdf

# JSON 출력
go run cmd/pdftext/main.go -json document.txt document.pdf
```

### pdfrender

페이지를 이미지로 렌더링합니다.

```bash
# 단일 페이지 렌더링 (1-based page index)
go run cmd/pdfrender/main.go -d 150 -p 1 document.pdf

# 페이지 범위 렌더링
go run cmd/pdfrender/main.go -d 150 -p 1-3 document.pdf

# 출력 디렉토리 지정
go run cmd/pdfrender/main.go -o ./output document.pdf

# Viper 환경변수로 출력 디렉토리 지정
PDFRENDER_OUTPUT=./output go run cmd/pdfrender/main.go -p 1 document.pdf
```

## 기여

기여를 환영합니다! 다음 단계를 따라주세요:

1. 포크하세요
2. 기능 브랜치 생성 (`git checkout -b feature/AmazingFeature`)
3. 커밋 (`git commit -m 'Add some AmazingFeature'`)
4. 푸시 (`git push origin feature/AmazingFeature`)
5. 풀 리퀘스트 오픈

### 코드 스타일

```bash
# 린트 실행
make lint

# Java parity 확인
make parity-java

# Java parity strict-regression 확인
make parity-regression-no-cgo

# 렌더 baseline exact-compare 회귀 (캐시 무시)
make render-regression-no-cgo

# 포팅 완료 게이트
make porting-complete

# 포맷팅
make fmt

# 테스트
make test
```

## 라이선스

MIT 라이선스 - [LICENSE](LICENSE) 파일을 참조하세요.

## 참조

- [PDF Reference 1.7](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf)
- [PDF.js](https://github.com/mozilla/pdf.js)

## 최신 업데이트

### v0.9.0-202602.1 최근 개선사항

- **XRef 스트림 지원**: 압축된 XRef 스트림 파싱, /Prev 체인을 통한 증분 업데이트 지원
- **CFF/Type1C 폰트**: Compact Font Format 파싱 및 글리프 렌더링
- **바이너리 CMap**: 바이너리 형식 CMap 파서 추가
- **스트림 필터 벤치마크**: 모든 압축 필터에 대한 포괄적인 벤치마크
- **통합 테스트**: 다양한 PDF 버전 및 기능에 대한 통합 테스트 스위트
- **오류 처리**: 100% 테스트 커버리지를 갖는 포괄적인 오류 처리 시스템
- **복합 객체 렌더링**: TrueType 복합 글리프 지원
- **패턴/쉐이딩**: 축 방향, 방사형, 함수 기반 패턴 지원

## 감사

모든 기여자분들께 감사드립니다.
