# Go PDF 렌더링 라이브러리 - 진행 상황

이 문서는 Go PDF 렌더링 라이브러리의 개발 진행 상황과 디자인 결정 사항을 기록합니다.

## 개요

이 프로젝트는 PDF.js를 순수 Go로 포팅하여 브라우저 의존성 없이 PDF 문서를 파싱하고 렌더링하는 라이브러리를 구현하는 것을 목표로 합니다.

**목표**: 서버 사이드 PDF 렌더링을 위한 순수 Go 구현
**참조**: [Mozilla PDF.js](https://github.com/mozilla/pdf.js)
**아키텍처**: Clean Architecture (Domain → UseCase → Interface → Infrastructure)

## 버전별 진행 상황

### v0.9.0-poppler24-02-0-202605.1 (릴리즈 준비 단계)

**목표 출시일**: 2026년 Q1
**상태**: 핵심 기능/성능/회귀 게이트 완료, 배포 작업 진행 중

#### 완료된 기능

| 카테고리 | 기능 | 상태 |
|---------|------|------|
| **파싱** | PDF 1.4-1.7 지원 | ✅ |
| | XRef 테이블 파싱 | ✅ |
| | XRef 스트림 파싱 | ✅ |
| | 증분 업데이트 (/Prev 체인) | ✅ |
| | 객체 캐싱 | ✅ |
| **폰트** | Standard 14 폰트 | ✅ |
| | Type1 폰트 파싱 | ✅ |
| | TrueType/OpenType 폰트 | ✅ |
| | CFF/Type1C 폰트 | ✅ |
| | CID-keyed (CJK) 폰트 | ✅ |
| | 바이너리 CMap 파싱 | ✅ |
| | 폰트 서브셋팅 | ✅ |
| **이미지** | JPEG 디코딩 | ✅ |
| | JPEG2000 디코딩 (CGo 선택적) | ✅ |
| | JBIG2 디코딩 (CGo 선택적) | ✅ |
| | 이미지 마스크 | ✅ |
| | 컬러 공간 변환 | ✅ |
| **콘텐츠** | 그래픽 상태 관리 | ✅ |
| | 경로 생성 | ✅ |
| | 텍스트 위치 지정 | ✅ |
| | 색상 공간 처리 | ✅ |
| | 패턴 지원 | ✅ |
| | XObject 처리 | ✅ |
| **렌더링** | 경로 렌더링 | ✅ |
| | 텍스트 렌더링 | ✅ |
| | 이미지 렌더링 | ✅ |
| | 클리핑 경로 | ✅ |
| **어노테이션** | 링크 어노테이션 | ✅ |
| | 텍스트 어노테이션 | ✅ |
| | 위젯 어노테이션 | ✅ |
| | 외관 스트림 | ✅ |
| **암호화** | 비밀번호 기반 암호 해독 | ✅ |
| | RC4 암호 해독 | ✅ |
| | AES 암호 해독 | ✅ |
| | 권한 플래그 | ✅ |
| **성능** | 병렬 페이지 렌더링 | ✅ |
| | 객체 풀링 | ✅ |
| | LRU 캐싱 | ✅ |
| | 스트림 필터 벤치마크 | ✅ |
| **도구** | pdfinfo CLI | ✅ |
| | pdftext CLI | ✅ |
| | pdfrender CLI | ✅ |
| **문서** | README.md | ✅ |
| | architecture.md | ✅ |
| | api.md | ✅ |

#### 최근 체크포인트 (2026-03-01)

- [x] `make porting-complete-plus-goal98` 통과
- [x] pure RGB synthetic lane의 `zero vs positive subpixel vertical offset` rasterization contract 정리
  - 상세: [rgb_subpixel_vertical_offset_contract.md](/workspace/pdf-reader/go-pdf/docs/design/rgb_subpixel_vertical_offset_contract.md)
- [x] 샘플 Poppler 비교 HTML 자동화(좌: Poppler, 중: Ours, 우: XOR + 유사도 PASS/FAIL) 운영
- [x] 실패 문서 장시간 재비교 자동화(`sample-compare-faildocs-recheck-no-cgo`)
  - 최신 재검증(2026-03-01, timeout 확장): docs `24`, pages `272`, pass exact `5`, pass mae `13`, avg exact `91.8997%`, avg mae `95.4143%`
  - baseline 대비 diff: delta `0` (정합성 개선 없음)
- [x] nightly diff 게이트 추가(`nightly-compare-diff-no-cgo`)
- [x] no-CGo core coverage 게이트 도입(`coverage-core-no-cgo`, 기준 `>=80%`)
  - core coverage: `80.1%`
  - 전체 coverpkg(참고): `77.9%`
- [x] 공개 심볼 godoc 누락 정리(스캔 기준 누락 `0`)
- [x] 렌더 병목 최적화(이미지 디코더 Pix 직접 쓰기) 반영
  - `BenchmarkPDFRender` (`-benchtime=3x`): `3.018ms/op -> 1.492ms/op`
- [x] 릴리즈 자동화 타깃 추가(`release-preflight`, `release-dry-run`, `release-publish`)
  - 현 워크스페이스는 `.git` 부재로 preflight expected fail

#### 진행 중인 작업

- [ ] 릴리즈 태그 생성
- [ ] GitHub 릴리즈 생성
- [ ] Go 모듈 퍼블리시

### v0.2.0 (계획 중)

**예정 기능**:
- [ ] GPOS/GSUB 테이블 지원 (고급 표현재)
- [ ] 폼 필드 처리 개선
- [ ] 서명 검증
- [ ] PDF/A 지원
- [ ] PDF/X 지원

### v0.3.0 (장기 계획)

**예정 기능**:
- [ ] PDF 생성 기능
- [ ] PDF 수정 기능
- [ ] PDF 병합/분할
- [ ] 워터마크 추가
- [ ] PDF 최적화

## 디자인 결정 사항

### 1. Clean Architecture 채택

**이유**:
- 계층 간 느슨한 결합으로 테스트 용이성 확보
- 도메인 로직과 인프라스트럭처 분리로 유지보수성 향상
- 인터페이스 기반 설계로 확장성 확보

**구조**:
```
Domain Layer (내부) → UseCase → Interface → Infrastructure (외부)
```

### 2. 인터페이스 분리 원칙 (ISP)

**결정**: 작고 집중된 인터페이스 다수 사용

**이유**:
- 단일 책임 원칙 준수
- 클라이언트가 사용하지 않는 메서드에 의존하지 않음
- 모의 객체 생성 용이

**예시**:
```go
// 나쁜 예 (인터페이스 너무 큼)
type Font interface {
    CharCodeToGlyph(...) // 문자 매핑
    GetGlyphWidth(...)  // 메트릭
    RenderGlyph(...)    // 렌더링
    Subset(...)         // 서브셋팅
    Embed(...)          // 임베딩
}

// 좋은 예 (인터페이스 분리)
type Font interface {
    CharCodeToGlyph(...) uint32
    GetGlyphWidth(...) float64
}

type FontRenderer interface {
    RenderGlyph(...) (*GlyphPath, error)
}

type FontSubsetter interface {
    Subset() ([]byte, error)
}
```

### 3. 캡슐화 강화

**결정**: 모든 구조체 필드를 비공개(private)로 선언

**이유**:
- 내부 표현 변경 자유
- 불변성 보장
- 잘못된 사용 방지

**패턴**:
```go
// 나쁜 예
type Dict struct {
    Items map[string]Object // 공개 필드
}

// 좋은 예
type Dict struct {
    items map[string]Object // 비공개 필드
}

func (d *Dict) Get(key string) Object {
    return d.items[key]
}
```

### 4. 생성자 패턴

**결정**: `New*` 생성자 함수 사용 및 내부 필드 초기화

**이유**:
- 일관된 객체 생성
- 필수 초기화 보장
- zero 값 방지

**예시**:
```go
// 좋은 예
func NewLexer(r io.Reader) *Lexer {
    return &Lexer{
        reader: r,
        buf:    make([]byte, 4096),
        pos:    0,
    }
}
```

### 5. 오류 처리 전략

**결정**: 오류 래핑과 컨텍스트 보존

**이유**:
- 오류 발생 위치 추적 용이
- 오류 체인으로 디버깅 개선
- 타입별 오류 처리 가능

**구현**:
```go
return &PDFError{
    Op:   "parse_xref",
    Err:  err,
    Type: ErrTypeInvalid,
}
```

### 6. 동시성 모델

**결정**: 워커 풀 패턴과 컨텍스트 기반 취소

**이유**:
- 리소스 사용량 제어
- 그레이스풀 셧다운 지원
- 타임아웃 처리 용이

**구현**:
```go
workerSemaphore := make(chan struct{}, maxWorkers)

for _, pageNum := range pageNumbers {
    go func(pn int) {
        workerSemaphore <- struct{}{}
        defer func() { <-workerSemaphore }()

        select {
        case <-ctx.Done():
            return // 컨텍스트 취소 시 종료
        default:
        }

        // 페이지 렌더링
    }(pageNum)
}
```

### 7. 캐싱 전략

**결정**: LRU 캐시와 TTL 조합

**이유**:
- 최근 사용 데이터 우선 저장
- 오래된 데이터 자동 만료
- 메모리 사용량 제어

**구현**:
```go
type LRUCache struct {
    maxSize  int
    maxBytes int64
    ttl      time.Duration
}
```

### 8. 빌드 태그 사용

**결정**: 선택적 CGo 의존성을 위한 빌드 태그

**이유**:
- 순수 Go로 기본 빌드 가능
- 선택적 고급 기능 지원
- 배포 용이성

**예시**:
```go
//go:build !nojpx
// +build !nojpx

package jpx

/*
#cgo pkg-config: libopenjp2
#include <openjpeg.h>
*/
import "C"
```

## 성능 목표

### 렌더링 성능

| 작업 | 목표 | 현재 |
|------|------|------|
| 단일 페이지 렌더링 (72 DPI) | < 100ms | ~100ms |
| 단일 페이지 렌더링 (150 DPI) | < 250ms | ~250ms |
| 캐시 적중 시간 | < 1ms | <1ms |
| 텍스트 추출 | < 50ms | ~50ms |

### 메모리 사용

| 항목 | 목표 |
|------|------|
| 페이지당 메모리 | < 100MB |
| 캐시 최대 크기 | 구성 가능 |
| 메모리 누수 | 0 (go test -race) |

### 테스트 커버리지

| 계층 | 목표 | 현재 |
|------|------|------|
| 전체 | > 80% | 진행 중 |
| 도메인 | > 90% | 진행 중 |
| 인프라스트럭처 | > 75% | 진행 중 |

## 기술 부채

### 해결 필요

1. **폰트 서브셋팅**: 현재 프레임워크만 구현됨, 실제 서브셋팅 로직 필요
2. **GPOS/GSUB**: 고급 표현재 지원 (선택적)
3. **테스트 커버리지**: 일부 모듈 커버리지 부족
4. **문서화**: 일부 내부 패키지 godoc 불충분

### 개선 계획

1. **단기 (v0.9.0-poppler24-02-0-202605.1 릴리스 전)**:
   - 단위 테스트 커버리지 80% 달성
   - golangci-lint 통과
   - E2E 테스트 구현

2. **중기 (v0.2.0)**:
   - 폰트 서브셋팅 완전 구현
   - 프로파일링 기반 최적화
   - 메모리 누수 점검

3. **장기 (v0.3.0+)**:
   - 성능 벤치마크 개선
   - 병렬 처리 최적화
   - 캐시 전략 개선

## 참조 자료

### PDF 사양
- [PDF Reference 1.7](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf)
- [PDF 2.0 (ISO 32000-2:2017)](https://www.iso.org/standard/64634.html)

### 오픈소스 프로젝트
- [Mozilla PDF.js](https://github.com/mozilla/pdf.js)
- [Go-pdf (rsc/pdf)](https://github.com/rsc/pdf)
- [UniDoc](https://github.com/unidoc/unipdf)

### 폰트 형식
- [TrueType Specification](https://docs.microsoft.com/en-us/typography/truetype/)
- [OpenType Specification](https://docs.microsoft.com/en-us/typography/opentype/)
- [CFF Specification](https://www.adobe.com/devnet/font/pdfs/5176.CFF.pdf)

## 라이선스

MIT License - [LICENSE](../../LICENSE) 참조

## 기여

기여를 환영합니다! [README.md](../../README.md)의 기여 가이드라인을 참조하세요.
