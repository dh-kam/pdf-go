# 기능 및 동작 방식

이 문서는 현재 라이브러리가 제공하는 주요 기능과 각 기능이 내부에서 동작하는 방식을 설명한다. 공개 API는 주로 `pkg/pdf`에 있고, 렌더링과 파싱 구현은 `internal/` 하위 계층에서 처리한다.

## 문서 로딩과 PDF 객체 해석

`pdf.Open()`은 파일을 열어 PDF 바이트를 읽고, xref table 또는 xref stream을 해석해 `Document`를 구성한다. 증분 업데이트가 있는 문서는 `/Prev` 체인을 따라 이전 xref를 함께 읽어 최신 객체 상태를 만든다.

문서 내부 객체는 `Dict`, `Array`, `Name`, `Ref`, `Stream` 같은 값 객체로 보관된다. 간접 객체는 xref를 통해 지연 해석되며, 페이지나 리소스를 요청할 때 필요한 참조만 resolve한다.

지원하는 주요 스트림 필터는 `FlateDecode`, `ASCIIHexDecode`, `ASCII85Decode`, `LZWDecode`, `RunLengthDecode`, `CCITTFaxDecode`, `DCTDecode`, `JPXDecode`, `JBIG2Decode`이다. PNG predictor와 TIFF predictor는 stream decode 단계에서 적용한다.

## 페이지 정보와 문서 메타데이터

페이지 수, `MediaBox`, `CropBox`, 회전값, 100% 기준 너비/높이, zoom 적용 크기를 조회할 수 있다. 렌더링은 기본적으로 페이지의 visible box인 `CropBox`와 page rotation을 반영한다.

문서 정보 사전과 XMP 메타데이터는 별도 API로 읽는다. 제목, 작성자, 주제, 키워드, 생성/수정일 같은 문서 속성은 가능한 경우 PDF info dictionary와 metadata stream을 함께 사용한다.

## 렌더링 백엔드

렌더러는 `ConcurrentRenderer`가 담당한다. 단일 페이지는 `RenderPage()`로, 여러 페이지는 `RenderPages()` 또는 `RenderAllPages()`로 렌더링한다. 여러 페이지 렌더링은 worker pool을 사용하며 결과는 channel로 반환한다.

`RendererOptions.Backend`로 렌더링 백엔드를 선택한다. 기본값은 `image-canvas`이고, Poppler Splash 정합 작업에 사용하는 `splash` 백엔드도 제공한다. 빈 backend 값은 `image-canvas`와 같다.

`RenderOptions`는 DPI, scale, background color, page cache 사용 여부, 이미지 샘플링 debug, 이미지 샘플링 mode를 제어한다. DPI와 scale은 point 단위 PDF 좌표를 device pixel 크기로 변환할 때 사용한다.

페이지 캐시는 page identity, DPI, scale, 배경색, 샘플링 mode를 포함한 키로 동작한다. Form XObject operator cache는 반복 사용되는 Form stream의 operator 파싱 결과를 재사용한다.

## 콘텐츠 스트림 평가

콘텐츠 evaluator는 PDF content stream의 graphics operator를 순서대로 실행한다. 그래픽 상태 stack, CTM, line width, dash, clipping path, color space, text state, resource dictionary를 유지하면서 canvas 명령으로 변환한다.

Path 명령은 move/line/curve/close/fill/stroke/clip으로 평가한다. Splash 백엔드는 Poppler Splash와의 exact pixel parity를 목표로 stroke outline, glyph phase, pattern replay, shading cache, RGB8 blend rounding을 Poppler 방식에 맞춘다.

Text 명령은 font resource, text matrix, text line matrix, glyph advance, ToUnicode/CMap 정보를 사용한다. Type1, Type1C/CFF, TrueType/OpenType, CID-keyed font를 처리하며, embedded font가 없을 때는 제한된 system fallback을 사용한다.

## 이미지 디코딩과 샘플링

이미지 XObject와 inline image는 filter, color space, bits per component, decode array, image mask, soft mask를 해석해 bitmap으로 변환한다. `DCTDecode`는 JPEG decoder 경로를 사용하며, `GO_PDF_ENABLE_DJPEG_GO=1`이면 `github.com/dh-kam/djpeg-go`의 Poppler PDF compatibility decoder를 우선 시도한다.

마지막 filter가 `DCTDecode`, `JPXDecode`, `JBIG2Decode`인 filter array는 앞단 prefix filter만 먼저 해제하고 encoded image bytes를 이미지 decoder에 넘긴다. 이 방식은 Poppler처럼 JPEG/JPEG2000/JBIG2 자체 decoder가 원본 encoded stream을 처리하도록 하기 위한 동작이다.

색공간은 DeviceGray, DeviceRGB, DeviceCMYK, Indexed, ICCBased 계열을 처리한다. CMYK와 Indexed CMYK는 샘플 문서 parity를 위해 변환 mode와 LUT 기반 경로를 함께 제공한다.

이미지 샘플링 mode는 `legacy`를 기본값으로 사용한다. 실험 또는 비교용으로 adaptive DCT/ICCBased, indexed origin downscale, indexed CMYK 변환, tiny DCT gray ICC 무시 mode 등을 선택할 수 있다. 기본 경로에는 문서 surface가 검증된 narrow gate만 승격한다.

## 패턴, 셰이딩, 투명도

Tiling pattern은 colored/uncolored pattern을 구분하고, 필요한 경우 cell alpha와 tint color를 분리해 합성한다. overlap tile이나 Poppler fallback replay가 필요한 경우에는 tile bitmap sampling이 아니라 tile form replay 순서를 기준으로 맞춘다.

Axial/radial shading은 pattern matrix, CTM, clipping bbox를 반영해 shading cache를 구성한다. Gouraud shading은 vertex color 또는 parameterized scalar를 Poppler 방식으로 보간한 뒤 function을 평가한다.

Soft mask는 source image와 mask image를 별도 scratch bitmap에 렌더한 뒤 최종 합성한다. JPEG SMask는 일반 stream decode에서 미리 손실 decode하지 않고 encoded bytes를 보존해 이미지 decoder 경로로 처리한다.

## 어노테이션과 폼

페이지 contents 렌더링 뒤 annotation post-page pass를 실행한다. `/AP` appearance stream이 있으면 해당 stream을 annotation rect와 matrix에 맞춰 렌더링한다.

`/Text`, `/Highlight`, `/Ink`, `/Link`, `/Widget` 같은 annotation은 appearance가 없거나 generated appearance가 필요한 경우 내부 appearance를 생성한다. `/NeedAppearances true`인 AcroForm widget은 기존 appearance보다 생성 appearance를 우선한다.

폼 필드는 AcroForm tree를 순회해 field name, type, value, default value, options를 읽는다. 세션 범위에서 field value와 option을 변경할 수 있으며, 변경 내용은 session state나 save 경로를 통해 유지한다.

Annotation mutation API는 페이지별 annotation 추가, 제거, 교체, rect/type/contents/path/user data 수정을 지원한다. 이 변경도 원본 PDF를 즉시 파괴하지 않고 session overlay로 관리한다.

## 텍스트 추출과 검색

`Document.Text()`는 페이지 text layer를 추출해 reading order에 맞춰 문자열로 반환한다. `TextRange()`는 페이지 텍스트의 rune 범위를 잘라 반환한다.

`TextLines()`와 `TextParagraphs()`는 glyph 위치와 line height를 기반으로 줄과 문단을 구성한다. `GetPageTextAsXMLSL()`은 추출된 텍스트 항목을 XML 형태로 내보낸다.

`SearchText()`와 `SearchTextInPage()`는 case-sensitive, whole-word, page range, max results 옵션을 받아 텍스트를 검색한다. 검색 결과에는 page index, 문자열 범위, context, bbox, quad points가 포함된다.

## 북마크, 아웃라인, 페이지 편집

Outline API는 PDF outline tree를 읽고 title, destination, action, color, child outline을 반환한다. 세션 범위에서 outline 추가와 삭제가 가능하다.

페이지 편집은 session page order를 사용한다. 페이지 삭제, 이동, 중복 같은 작업은 원본 페이지 객체를 즉시 재작성하지 않고 현재 세션의 page order overlay에 반영한다.

Page label, layout, read direction, double-page view 관련 API는 viewer 호환 동작을 위해 제공된다. 값이 원본 PDF에 없으면 문서 상태 또는 기본값을 사용한다.

## 첨부 파일, 사용자 데이터, 세션 상태

Attachment API는 `/Names/EmbeddedFiles` tree를 읽어 첨부 파일 목록과 payload를 반환한다. 세션 범위에서 첨부 파일 추가와 삭제를 지원하며, 파일에서 바로 첨부하는 helper도 제공한다.

User data API는 namespace/key 기반으로 문서 또는 페이지 단위 임의 데이터를 저장한다. viewer 상태나 외부 애플리케이션 확장 데이터를 PDF 처리 흐름과 함께 유지할 때 사용한다.

Session state는 page order, form override, annotation override, attachment override, viewer state 같은 런타임 변경 사항을 JSON으로 export/import한다. `SaveWithEmbeddedSession()`은 세션 데이터를 PDF 안에 임베드하는 저장 경로를 제공한다.

## 서명과 보안 관련 기능

Signature API는 AcroForm signature field를 수집하고, ByteRange와 digest 정보를 검증한다. `SignatureDigest()`는 지정한 field의 서명 대상 바이트 범위에 대해 hash digest를 계산한다.

Visible signature field는 세션 범위에서 추가하거나 제거할 수 있다. 실제 CMS/PKCS#7 서명 생성과 외부 인증서 검증은 별도 서명 시스템이 담당해야 하며, 현재 API는 PDF 구조와 digest 계산 중심이다.

## Viewer 호환 상태

`ViewerSession`은 create/resume/pause/destroy lifecycle, 현재 페이지, 다음/이전 페이지 이동, zoom, toolbar/menu 표시 여부, page curl 옵션을 관리한다.

문서 레벨 viewer alias API는 기존 Java 계열 호출과 맞추기 위해 제공된다. width-fit, height-fit, night mode, watermark, page piece info, double-page view 같은 상태를 관리한다.

## CLI 도구

`pdfinfo`는 문서 메타데이터와 페이지 정보를 출력한다. `pdftext`는 페이지 텍스트를 추출한다. `pdfrender`는 페이지를 PNG로 렌더링하며 DPI, page range, backend, image sampling mode, debug trace 옵션을 제공한다.

`pixeldiff`, `splash_pixel_diff`, `compare_pixels` 계열 도구는 Poppler와의 렌더링 차이를 분석하기 위한 개발용 CLI이다. 일반 사용 경로보다는 회귀 분석과 exact parity 작업에 사용한다.

## 빌드

JPX/JBIG2/FreeType/Splash 렌더링 경로는 기본적으로 순수 Go 구현을 사용한다. 외부 OpenJPEG, jbig2dec, FreeType, Cairo 라이브러리 링크가 필요하지 않다.

```bash
CGO_ENABLED=0 go build ./cmd/pdfrender
CGO_ENABLED=0 go test ./...
```
