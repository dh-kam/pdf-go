# Implementation Status

## Overview

이 문서는 Go PDF 렌더링 라이브러리의 현재 구현 상태와 최근 수정 내용을 기록합니다.

## 최근 수정 (2025-02-08)

### XRef Stream Parsing 진행 상황

**현재 상태**: 23/32 PDFs 작동 (71.9%)

**구현된 기능:**
1. XRef stream dictionary 파싱 (`parseXRefStreamWithDetails`)
2. Stream 데이터 추출 (`extractStreamData`) - 스캔 방식으로 개선
3. Stream 디코딩 (`decodeStream`) - FlateDecode, ASCIIHexDecode, ASCII85Decode 지원
4. XRef stream 데이터 파싱 (`parseXRefStreamData`) - /W, /Index, /Size 필드 처리
5. Entry type 0 (free), 1 (uncompressed), 2 (compressed) 처리

**남은 문제:**
- **Object Stream 처리**: PDF 1.5+의 object streams (압축된 객체들이 포함된 스트림)
  - Entry type 2 (compressed)인 경우, 해당 객체는 object stream 내부에 압축되어 저장됨
  - Object stream을 가져와서 디코딩 후 개별 객체를 추출해야 함
  - 실패하는 6개 PDF: minimal-document.pdf, pdflatex-image.pdf, pdflatex-4-pages.pdf, pdflatex-outline.pdf, GeoTopo.pdf, multicolumn.pdf

**구현이 필요한 기능:**
```go
// Fetch에서 EntryTypeCompressed인 경우 처리
if entry.Type == repository.EntryTypeCompressed {
    // 1. object stream 가져오기 (entry.Offset은 object stream 번호)
    // 2. object stream 디코딩
    // 3. stream 내에서 entry.Generation 번째 객체 추출
    return x.parseObjectStream(entry.Offset, entry.Generation)
}
```

### 해결된 버그

#### 1. Parser Indirect Reference Parsing (parser.go)

**문제**: 파서가 간접 참조(Indirect Reference) 형식인 `N N R` (예: `17 0 R`)을 올바르게 파싱하지 못함

**원인**: 파서가 숫자를 읽을 때 바로 `Integer`를 반환하여, 이후 `R` 토큰을 확인할 수 없었음

**해결**: `ParseObject` 함수에서 간접 참조 패턴을 감지하도록 수정
```go
// Number 토큰 처리 시 다음 토큰들을 확인하여 간접 참조인지 체크
if token.Type == TokenNumber {
    num, err := parseInteger(token.Value)

    // 다음 토큰이 숫자이고, 그 다음이 "R"이면 간접 참조
    next1, err := p.lexer.Peek()
    if err == nil && next1.Type == TokenNumber {
        p.lexer.NextToken() // consume
        gen, _ := parseInteger(next1.Value)

        next2, _ := p.lexer.Peek()
        if err == nil && next2.Type == TokenKeyword && next2.Value == "R" {
            p.lexer.NextToken() // consume
            return entity.NewRef(uint32(num), uint16(gen)), nil
        }
    }

    return entity.NewInteger(num), nil
}
```

#### 2. parseObjectAt Function (xref.go)

**문제**: `parseObjectAt` 함수가 객체 헤더(`N N obj`)를 건너뛰지 않고 객체 번호를 반환

**원인**: 함수가 `ParseObject()`를 바로 호출하여 첫 번째 토큰(객체 번호)를 반환

**해결**: 객체 헤더를 명시적으로 파싱하고 건너뛰도록 수정
```go
func (x *Table) parseObjectAt(offset uint64) (entity.Object, error) {
    lexer := parser.NewLexer(bytes.NewReader(x.stream[offset:]))

    // Skip: N N obj
    lexer.NextToken() // object number
    lexer.NextToken() // generation number
    token3, _ := lexer.NextToken()
    if token3.Type != parser.TokenKeyword || token3.Value != "obj" {
        return nil, fmt.Errorf("expected 'obj'")
    }

    // Parse actual object content
    p := parser.NewParser(lexer, x)
    return p.ParseObject()
}
```

#### 3. resolveCatalog Function (xref.go)

**문제**: `Dict.Get()`의 auto-dereferencing 동작으로 인해 카탈로그 확인 실패

**원인**: `trailer.Get("Root")`를 호출하면 자동으로 참조를 해제하여 딕셔너리를 반환
하지만 `resolveCatalog`는 `entity.Ref` 타입을 기대하므로 타입 단언 실패

**해결**: 이미 dereferenced 된 딕셔너리인 경우와 `Ref`인 경우를 모두 처리
```go
func (x *Table) resolveCatalog() error {
    rootVal := x.trailer.Get(entity.Name("Root"))

    // Case 1: 이미 dereferenced된 catalog
    if catalog, ok := rootVal.(*entity.Dict); ok {
        if typeVal := catalog.Get(entity.Name("Type")); typeVal == entity.Name("Catalog") {
            x.catalog = catalog
            return nil
        }
    }

    // Case 2: indirect reference (또는 fetch 실패)
    if ref, ok := rootVal.(entity.Ref); ok {
        obj, err := x.Fetch(ref)
        // ...
    }
}
```

#### 4. parseTraditionalXRef Function (xref.go)

**문제**: `Fscanf`와 lexer를 혼합 사용하여 버퍼링 이슈 발생

**해결**: lexer만 사용하도록 수정 (`readXRefEntryFromLexer` 함수 추가)

#### 5. Array Parsing Buffer Issue (parser.go) - NEW

**문제**: 배열 파싱 시 간접 참조 감지를 위해 두 번째 숫자를 버퍼링했지만, `parseArray()`가 버퍼를 확인하지 않아 토큰이 누락됨

**원인**: `ParseObject()`가 "1 2 1"에서 "2"를 버퍼링하고 "1"을 반환했지만, `parseArray()`가 `lexer.Peek()`를 사용하여 버퍼를 건너뜀

**해결**: `parseArray()`가 버퍼를 먼저 확인하도록 수정
```go
func (p *Parser) parseArray() (entity.Object, error) {
    var items []entity.Object

    for {
        // Check for buffered value first
        if p.buf1 != nil {
            items = append(items, p.buf1)
            p.buf1 = nil
            continue
        }

        token, err := p.lexer.Peek()
        // ... rest of implementation
    }
}
```

#### 6. XRef Stream Support (xref.go)

**문제**: PDF 1.5+ XRef Stream 파싱 미구현

**해결**: 객체 스캔 방식으로 폴백하도록 구현
- `parseByObjectScanning()` 함수를 사용하여 XRef 스트림이 있는 PDF 처리
- `buildCatalogFromScan()`이 최소한의 트레일러와 카탈로그 생성
- 향후 완전한 XRef 스트림 파싱이 필요함

### 현재 지원 기능

#### ✅ 지원함 (MVP P0)
- PDF 1.3-1.4 문서 파싱 (Traditional XRef Table)
- 문서 열기 (`pdf.Open`)
- 페이지 수 조회 (`doc.PageCount()`)
- 페이지 접근 (`doc.Page(index)`)
- 페이지 크기 정보 (`page.Width()`, `page.Height()`)
- 페이지 회전 정보 (`page.Rotate()`)
- 트레일러 파싱 (`/Size`, `/Root`, `/Info` 등)
- 간접 참조 파싱 (Indirect References: `N N R`)
- 카탈로그 resolving
- 배열 내 버퍼링 처리
- 기본 페이지 렌더링 (canvas)
- 페이지 컨텐츠 추출

#### ✅ 작동 샘플 PDF (23/32)
- 002-trivial-libre-office-writer.pdf
- 005-libreoffice-writer-password.pdf
- 007-imagemagick-images/*.pdf (all 4 files)
- 008-reportlab-inline-image/inline-image.pdf
- 011-google-doc-document/google-doc-document.pdf
- 012-libreoffice-form/libreoffice-form.pdf
- 013-reportlab-overlay/reportlab-overlay.pdf
- 014-outlines/mistitled_outlines_example.pdf
- 015-arabic/*.pdf (all 2 files: habibi.pdf, habibi-rotated.pdf)
- 015-arabic/habibi-oneline-cmap.pdf
- 016-rotated/libre-office-link.pdf
- 017-metadata/*.pdf (all 4 files)
- 018-different-fonts/*.pdf (all 4 files)
- 019-unicode/*.pdf (all 4 files)
- 020-conference/* (all conference PDFs)
- with-attachment.pdf
- annotated_pdf.pdf

#### ❌ 미지원 PDF (6/32)
- 001-trivial/minimal-document.pdf (XRef stream + object streams)
- 003-pdflatex-image/pdflatex-image.pdf (XRef stream + object streams)
- 004-pdflatex-4-pages/pdflatex-4-pages.pdf (XRef stream + object streams)
- 006-pdflatex-outline/pdflatex-outline.pdf (XRef stream + object streams)
- 009-pdflatex-geotopo/GeoTopo.pdf (XRef stream + object streams)
- 026-latex-multicolumn/multicolumn.pdf (XRef stream + object streams)

**공통점**: 실패하는 PDF는 모두 PDF 1.5+ XRef stream과 **object streams**를 사용함

#### ⏭️ 건너뜀 (3/32)
- 009-pdflatex-geotopo/GeoTopo-komprimiert.pdf (catalog root not found)
- 017-metadata/unreadablemetadata.pdf (metadata issue)

#### ⚠️ 부분 지원
- XRef Stream (PDF 1.5+):
  - XRef stream dictionary 파싱 완료
  - Stream 데이터 추출 및 디코딩 완료
  - **Object stream 처리 미구현**: 압축된 객체들의 추출이 필요
  - 현재 object scanning fallback 사용 중

#### ❌ 완전 미지원
- JPEG2000 이미지
- JBIG2 이미지
- 암호화된 PDF
- 텍스트 추출
- 어노테이션 렌더링
- 폼 필드

### 테스트 결과 (2025-02-08)
```
Total PDFs: 32
✅ Working: 23 (71.9%)
❌ Failing: 9 (28.1%)

성공 사례:
- LibreOffice Writer PDFs: ✅
- ImageMagick PDFs: ✅
- Google Docs PDFs: ✅
- ReportLab PDFs: ✅

실패 사례:
- pdflatex PDFs: ❌ (XRef stream + object streams)
- 최소 PDF: ❌ (Compressed content)
```

### 진행 중인 작업
- XRef stream 파싱 (Task #9)
- Object stream 파싱 (Task #9)
- 페이지 렌더링 개선

## 프로젝트 구조

```
go-pdf/
├── docs/
│   └── implementation.md  (이 파일)
├── internal/
│   ├── domain/           # 도메인 계층
│   │   ├── entity/       # 핵심 엔티티
│   │   ├── errors/       # 에러 정의
│   │   └── repository/   # 저장소 인터페이스
│   ├── infrastructure/
│   │   ├── pdf/          # PDF 파싱 구현
│   │   │   ├── parser/   # Lexer, Parser
│   │   │   └── xref/     # XRef 테이블 (최근 수정됨)
│   ├── usecase/pdf/     # PDF 로딩 유스케이스
│   └── ...
├── pkg/pdf/             # 공개 API
│   └── document.go      # Document, Page 타입
└── test/
    ├── integration/pdf/ # 통합 테스트
    └── testdata/        # 테스트 PDF 파일
```

## 테스트 결과

### 통합 테스트 성공
```bash
$ go test -v ./test/integration/pdf -run TestOpenPDF
=== RUN   TestOpenPDF
Successfully opened PDF
Page count: 1
Page 0: 4 x 4 points
--- PASS: TestOpenPDF
```

### 지원되는 PDF 형식
- **PDF 1.3-1.4**: Traditional XRef Table 사용하는 PDF ✅
- **PDF 1.5+**: XRef Stream 사용하는 PDF (제한적 지원)

## 다음 단계

1. **XRef Stream 파싱 완전 구현** (높은 우선순위)
   - `/W`, `/Index`, `/Size` 필드 파싱
   - 압축된 XRef 데이터 해제
   - 하이브리드 XRef (traditional + stream)

2. **기본 렌더링 구현**
   - Canvas 그리기
   - 텍스트 렌더링�
   - 이미지 렌더링�

3. **추가 테스트**
   - 다양한 PDF 샘플 파일 테스트
   - 에지 케이스 처리

## 참고 사항

- AGENTS.md 가이드라인 준수 (Clean Architecture, Encapsulation)
- 코드는 영어, 문서는 한국어
- 80% 테스트 커버리지 목표
- 변경사항은 TODO.md 실시간 업데이트 필요
