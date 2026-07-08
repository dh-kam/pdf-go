# Go PDF Rendering Library - TODO Checklist

## Poppler Splash Exact100 후속 점검
- [x] Splash backend 기준 tracked corpus + 외부 2nd corpus의 Poppler(pdftoppm) 대비 픽셀 parity를 exact100으로 달성한다 (2026-07-08: tracked 251/252 — 유일 예외는 pdftoppm이 레퍼런스를 못 만드는 password 문서로 수동 레퍼런스 대비 0 diff; 2nd 459/459). 최종 fix: arbitraryTransformImage 행중심 +0.5·scaled-space 역행렬, univariate shading의 클립된 t-범위 캐시 보간, 패턴 CTM iCTM 왕복 이식.
- [ ] LEGACY(image-canvas) backend 잔여 2건을 수정한다: ① 16×16 ICCBased-gray ramp가 tiny-downscale 샘플러에서 검정으로 렌더 (007 bucket 테스트 skip 처리, `TestSampleDecodeOrTransform007BucketsRenderParity` 참고) ② render-parity 대상 form 문서 p1 exact% −0.0019pp drift (watchset.tsv floor 조정 커밋 참고). 두 경우 모두 Splash backend는 시스템 pdftoppm 대비 byte-exact 확인됨.
- [ ] `GO_PDF_YDOWN_BASE=1` y-down 베이스 마이그레이션을 기본화한다 (셰이딩/타일링/annotation/Type3 전 클래스 flag-on 검증 완료; corpus flag-on 재검증 후 default 전환 + flag 제거).
- [ ] `B`/`B*`/`b`/`b*` fill-and-stroke operator의 stroke 절반을 기본화하기 전에 `PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_STROKE_PATH=1`을 별도 평가한다. broad gate는 focus/286을 통과했지만 path-heavy 문서의 성능 위험이 있다.
- [ ] Poppler 호환 cropped group geometry, backdrop, transfer, performance parity가 검증될 때까지 ExtGState `/SMask` Form-mask 렌더링은 `GO_PDF_ENABLE_EXTGSTATE_SMASK=1` 뒤에 유지한다.
- [ ] Poppler version/path 중 `Page::display`에서 page `/Group`을 render group으로 감싸는 근거가 확인되기 전까지 page-level transparency group 렌더링은 비활성화 상태로 유지한다.
- [ ] 현재 2nd-corpus 경로에서는 djpeg-go DCTDecode를 기본 비활성화로 유지한다. JPEG A/B subset은 `-pure-go-jpeg`에서 `27/380`에서 `24/380` exact100으로 회귀했다.
- [ ] `GeoTopo p39` 22px stroke AA overlap 잔차를 Poppler `Splash::makeStrokePath()` 및 `Splash::pipeRunAARGB8()` 경로 기준으로 재검토한다.
- [ ] `GeoTopo p55` pattern/fill/stroke 잔차를 image/soft-mask가 아닌 segment stroke 및 tiling pattern 축으로 재분해한다.
- [ ] `GeoTopo p55`, `p23`, `p44`, `p97`을 함께 보면서 Poppler `strokeNarrow`와 `strokeWide` 분기 조건을 더 좁힌다.
- [ ] native Splash는 `minimal-document.pdf`를 즉시 렌더링하지만 Node WASM Splash 렌더링이 20초 안에 반환되지 않는 원인을 진단한다.

## Pure Go 포팅 계획
- [x] 과거 `nojpx,nojbig2` exact100은 현재 corpus 범위의 검증으로만 보고 JPX/JBIG2 포팅 완료로 간주하지 않는다.
- [x] CGo inventory gate를 `import "C"`가 0개인지 확인하는 검사로 전환한다.
- [x] PDF `DecodeParms` / `JBIG2Globals`를 JBIG2 decode option으로 연결하고 full-corpus exact100을 유지한다.
- [x] JBIG2 MQ arithmetic decoder와 arithmetic generic bitmap region template 0-3을 포팅한다.
- [x] Poppler JBIG2 arithmetic generic region typical prediction(`TPGDON`) row 처리를 맞춘다.
- [x] Poppler와 동일한 JBIG2 external region combination operator를 적용하고 immediate generic region 여러 개를 segment 순서대로 합성한다.
- [x] JBIG2 Page Information flags의 default pixel 초기화를 Poppler와 맞춘다.
- [x] JBIG2 pattern dictionary와 halftone skip/multi-plane arithmetic gray-image 합성을 포팅한다.
- [x] Poppler 기준 JBIG2 generic refinement region 파싱, context, TPGR 처리, shared arithmetic refinement decoding을 포팅한다.
- [x] Poppler 기준 JBIG2 arithmetic integer/IAID decoding과 symbol dictionary/text region 헤더 파싱 및 저장 scaffolding을 추가한다.
- [x] Poppler 기준 JBIG2 arithmetic symbol dictionary refinement/aggregate bitmap decoding 및 export 흐름을 포팅한다.
- [x] JBIG2 multi-plane MMR halftone gray-image decoding과 intermediate halftone region storage를 포팅한다.
- [x] Poppler 기준 JBIG2 default Huffman text region placement 및 refinement switching을 포팅한다.
- [x] Poppler 기준 JBIG2 default Huffman symbol dictionary collective bitmap, refinement/aggregate, export 흐름을 포팅한다.
- [x] JBIG2 custom Huffman code-table segment parsing 및 symbol/text region referenced-table dispatch를 포팅한다.
- [x] PageInfo가 없는 fallback 경로에서 dictionary/code-table을 수집한 뒤 첫 text, halftone, refinement, generic bitmap segment를 decode한다.
- [x] global stream과 page stream이 같은 segment 번호를 재사용할 때 Poppler `JBIG2Globals` 우선순위를 맞춘다.
- [x] generic refinement decode 뒤 참조된 intermediate bitmap을 제거하는 Poppler bitmap 생명주기를 맞춘다.
- [ ] malformed/interleaved `JBIG2Globals` reference 관련 남은 JBIG2 native decoder edge case를 완료한다.
- [x] JP2 box와 raw codestream을 처리하는 실제 pure Go JPEG2000 decoder 경로를 추가한다.
- [x] JP2 container와 raw J2K codestream decode 경로의 JPX 단위 테스트 coverage를 추가한다.
- [ ] JPX exact parity를 Poppler/OpenJPEG coverage 기준으로 검증하고 미지원 JPEG2000 feature gap을 닫는다.
- [x] FreeType glyph lookup, glyph name, outline, bounding box, transform, approximate bitmap rasterization을 처리하는 pure Go SFNT fallback을 추가한다.
- [x] Type1이 아닌 pure-Go SFNT bitmap glyph에 Poppler 방식 ppem/matrix 정규화를 적용한다.
- [x] Type1 no-CGo glyph rasterization을 별도 gate로 유지해 격리 측정 시 끌 수 있게 하되, Type1-heavy parity가 더 좋아지므로 `PDF_FREETYPE_GO=1`에서는 기본 활성화한다.
- [x] `freetype-go`가 Type1/CFF cubic outline과 raster cubic tag를 보존하도록 갱신하고, local replace 없이 git dependency를 올린다.
- [x] Type1/CFF `freetype-go` raster 경로에 FreeType fill-rule coverage를 gate하고 full-corpus parity 검증 후 git dependency를 갱신한다.
- [x] FreeType 의존 glyph API를 pure Go glyph lookup, bbox, outline, transform, bitmap, phase, matrix raster 경로로 대체한다.
- [x] Cairo glyph mask rasterization을 pure Go raster strategy로 대체하거나 Splash exact parity가 유지되면 Cairo 경로를 제거한다.
- [x] FreeType, JPX, JBIG2, Cairo CGo wrapper를 source tree에서 제거한다.
- [x] e2e 하위 `go run` / `go build` 명령에서 feature-disabling build tag 전달을 제거한다.
- [x] `CGO_ENABLED=0`으로 build/test하는 no-CGo release gate를 추가한다.
- [x] Makefile no-CGo 검증 target이 실제로 `CGO_ENABLED=0`으로 실행되게 하고 race 검증은 CGo가 필요한 별도 gate로 분리한다.
- [x] 장시간 검증이 필요한 full-corpus Poppler exact100 HTML 생성은 `-timeout-sec 0`으로 실행 가능하게 유지한다.
- [x] pure Go PDF 렌더링을 브라우저에서 사용할 수 있도록 WebAssembly facade와 Worker 기반 demo를 추가한다.
- [x] Browser WebAssembly demo 기본값을 Splash로 바꾸고 CDP에서 확인 가능한 runtime log를 노출한다.

## FreeType-Go 업스트림 Workflow
- [ ] 확인된 FreeType parity gap마다 `freetype-go`에서 최소 fixture로 재현하고, 정확한 API 입력, pure Go 함수, 관측 delta를 기록한다.
- [ ] 확인된 `freetype-go` 결함은 먼저 `/workspace/freetype-go`에서 수정하고 검증한 뒤, `pdf-go`가 upstream 가능한 동작을 소비하도록 갱신한다.
- [ ] 확인된 각 gap에 대해 pure Go mismatch, fixture/page evidence, proposed fix를 포함한 GitHub issue를 `dh-kam/freetype-go`에 열고, 필요하면 implementation note를 issue comment로 남긴다.
- [ ] `freetype-go` 작업 전에는 `git fetch origin`을 실행하고, local worktree가 clean이거나 변경분이 안전하게 commit/stash된 경우에만 `git pull --rebase`를 실행한다.
- [x] `GeoTopo-komprimiert.pdf` no-CGo Splash exact100을 맞추는 과정에서 확인한 raw CFF Type1C Encoding charmap 및 FontBBox parity gap을 upstream한다.
- [ ] 변환된 TrueType glyph coverage parity를 위해 FreeType `ftgrays.c`의 `gray_render_line` / `gray_render_scanline`을 port 또는 upstream한 뒤 `Invoice-INV-6214322-202602-0001.pdf`와 receipt focus set을 재확인한다.

## 렌더링 정확도 개선
- [x] 공개 인터넷 PDF 100개 첫 페이지를 varied-DPI canvas size로 Poppler-vs-Splash 비교한다. arXiv batch는 `tmp/random_pdf_compare_20260606`에서 DPI 72/96/150/200/300으로 실행했고, `83/100` exact100, render timeout `2`건, 성공 비교 `98`페이지 기준 weighted pixel exact `99.92457612%`를 기록했다.
- [x] 동일한 공개 arXiv PDF 100개를 CIDToGIDMap 수정 후 varied-DPI canvas size로 전체 문서 Poppler-vs-Splash 비교하고 timeout 또는 mismatch 페이지를 선별한다. `tmp/random_pdf_compare_20260606/full_compare_after_cidmap_noimg`에서 DPI 72/96/150/200/300으로 전체 `2,629` 예정 페이지를 비교했고, `pdfinfo` 기준 page-count mismatch는 `0`건이었다. `2,579`페이지가 성공 렌더링됐으며, 성공 페이지 중 `2,147`페이지가 exact100, weighted pixel exact는 `99.90401462%`였다. 렌더러 메모리 폭증으로 종료된 2개 문서는 96 DPI의 `doc_077.pdf`와 200 DPI의 `doc_049.pdf`다.
- [x] 현재 소스 기준으로 first-page-only 샘플이 아닌 전체 `full_corpus` 입력을 사용해 공개 PDF 100개 전체 문서 Poppler-vs-Splash 비교를 재실행한다. 결과는 `tmp/random_pdf_compare_20260606/full_compare_current`에 있으며, `pdfinfo` 예정 페이지와 `report.csv` 행 수가 모두 `2,629`로 일치하고 page-count mismatch는 `0`건이다. `2,579`페이지가 성공 렌더링됐고 `50`페이지는 렌더러 에러 행으로 기록됐으며, `2,147/2,629`행이 exact100이었다(`81.66603271%` overall, 성공 페이지 기준 `83.25%`). 성공 페이지 기준 weighted pixel exact는 `99.91881806%`였다. 에러 행은 렌더러 메모리 보호 종료가 발생한 96 DPI의 `doc_077.pdf` `35`페이지와 200 DPI의 `doc_049.pdf` `15`페이지에 한정된다.
- [x] cached Type3 CharProc pre-parse에서 inline-image operator를 보존해 뒤따르는 graphics-state restore가 replay되도록 수정한다. 전체 문서 focused 재비교에서 arXiv `doc_007.pdf` 17페이지는 96 DPI 기준 `90.85663622%`에서 `99.98096777%`로, `doc_026.pdf` 1페이지는 72 DPI 기준 `92.20245758%`에서 `99.89127385%`로 개선됐다. 다음 큰 잔차는 300 DPI `doc_070.pdf` 6/9페이지의 image decode/sampling 축이다.
- [x] `doc_070.pdf` 300 DPI 전체 페이지 미일치 분해를 이어서 수행했다. `imageDrawOptions.disableTopDownDownscale`이 실제 Splash 분기에서 적용되도록 고쳐 explicit DCT downscale이 의도한 scale-then-flip 경로를 타게 했고, page 6은 `92.41326203%`에서 `99.44988711%`로 개선됐다. DCT RGB auto-scale은 Splash axis-aligned scale path를 쓰도록 `interpolate=false`를 보존해 page 11이 `95.17839572%`에서 `97.11688651%`로 개선됐다. 큰 explicit-nearest RGB surface에서는 Splash required interpolation fallback을 건너뛰도록 제한해 page 9가 `92.69415330%`에서 `97.34211527%`로 개선됐고 page 1/page 4 회귀는 제거했다. 최종 focused 산출물은 `tmp/codex_accuracy/doc070_final3_accuracy_300`이며, 남은 최상위 잔차는 page 9 FlateDecode DeviceRGB point-cloud overlay의 red/green sparse point 차이로 nearest sampling phase 또는 row grouping mismatch에 가깝다.
- [x] `doc_070.pdf` page 9 FlateDecode DeviceRGB overlay 분석을 이어서 수행했다. Poppler `pdfimages` source 추출과 로컬 decoder 추출은 page-9 이미지 XObject 전체에서 `0` bad pixel로 일치해 source decode/color conversion은 남은 원인이 아니었다. source-run nearest phase shift, 1px image placement shift, image clip AA toggle, Form transparency-group toggle은 page 9를 악화시키거나 변화가 없어 제외했다. 큰 explicit-nearest 업스케일은 center-mapped nearest source index를 사용하도록 바꿔 page 9를 `97.34211527%` / `223,661` bad pixels에서 `97.34452763%` / `223,458`로 개선했고, `tmp/codex_accuracy/doc070_centerprod_accuracy_300` 기준 나머지 10페이지는 변하지 않았다. `doc_100.pdf` 150 DPI 성능 corpus는 `tmp/codex_perf/doc100_opcap_150`와 byte-identical을 유지했다.
- [x] 성능 작업 이후 현재 `doc_070.pdf` 300 DPI 정확도 근거를 다시 확정했다. 최신 current 출력은 `tmp/codex_accuracy/doc070_goal_fresh_300`이며 `0/11` exact100이다. page 9가 여전히 최상위 잔차로 `97.34452763%` / `223,458` bad pixels이고, 그다음은 text-heavy page 7 `99.03382056%` / `81,304`, page 8 `99.17856209%` / `69,124`다. page 9 probe 중 전체 large explicit-nearest surface에 Splash required interpolation을 켜는 방식은 `251,908` bad pixels로 악화됐고, 768x512 Flate RGB plot 이미지만 켜도 `248,843`으로 악화됐으며, center-mapped nearest X/Y delta `{-1,0,+1}` 조합은 현재 `x0_y0`가 최선이었다. text probe로 `PDF_DEBUG_SPLASH_GLYPH_ALPHA_BIAS=-4,-1,+1,+4`를 page 7에 적용했지만 `81,304`에서 최소 `523,438` bad pixels로 악화되어 남은 text cluster는 전역 glyph-alpha bias 문제가 아니다.
- [x] `tmp/codex_exact/current_mismatch_20260614` 기준으로 현재
  `doc_070.pdf` page 9 exactness 분석을 이어서 진행했다. 가장 큰 잔차 영역은
  누락 content가 아니라 sparse FlateDecode DeviceRGB plot/overlay image stack
  내부였다. 전역 top-down vertical-flip scale-then-flip probe는 page 9를
  `223,458`에서 `222,999` bad pixels로 개선했고 worsened pixel은 `0`이었다.
  다만 large positive-subpixel 768x512 plot image scoped probe는 required
  interpolation 보정 후 유지 효과가 없어서 rejected 처리했다(`223,458 ->
  223,458`, changed `2,376` pixels). 유지한 exactness 변경은 small Flate RGB
  explicit-nearest downscale overlay에만 제한한
  `explicit_nearest_rgb_small_downscale_scale_then_flip` sampler다(`src<=32x32`,
  `dst<src`). focused page 9는 `223,458 -> 222,999` bad pixels,
  exact percent `97.34452763 -> 97.34998217`, improved `459`, worsened `0`으로
  이동했다. full 검증
  `tmp/codex_exact/rgb_edge_scale_then_flip_20260614/verify`에서는
  `doc_070.pdf` page 9만 변경됐고 regression은 `0`, `doc_100.pdf`는 변경
  page `0`이었다. exact100은 아직 `0/11`, `0/31`이다.
- [x] 현재 tracked exact100 미일치에서 큰 Flate RGB downscale rounding 케이스를 분석해 수정했다. `doc_027.pdf` page 4는 2365x665 `FlateDecode` `DeviceRGB` 이미지가 72 DPI 출력으로 축소될 때 generic average 경로가 광범위한 1-LSB 차이를 남기는 것이 핵심이었다. 전체 `auto_downscale_bilinear`에 fixed-average를 켜는 probe는 일부 페이지를 개선했지만 작은 GeoTopo 이미지에서 회귀가 있어 제외했고, 유지한 정책은 표시 크기가 큰 Flate RGB downscale(`dst >= 300x100`)에만 `auto_downscale_bilinear_fixed_avg`를 선택한다. isolated no-fixed overlay baseline 대비 `tmp/codex_accuracy_work/conditional_fixedavg_verify_20260614`의 tracked 72-DPI corpus는 `42/438` exact100을 유지하면서 악화 페이지 `0`개였고, 전체 bad pixels는 `4,185,624 -> 4,047,060`으로 줄었다. `base64image.pdf` page 1은 `87,695 -> 11,343`, `doc_027.pdf` page 4는 `76,925 -> 14,713` bad pixels로 개선됐다.
- [ ] `pdf.js` fixture와 기존 sample fixture를 합쳐 render mismatch 상위 문서부터 98%+까지 반복 개선한다.
- [ ] 실패 문서 재비교 결과에서 `decode_or_transform`, `resample_or_antialias` 병목 근본 원인을 차단한다.
- [ ] `tmp/goal98_batch.go` 결과 기준 99% pass 보장을 목표로 `tmp/sample_compare` 및 `tmp/sample_compare_faildocs_recheck_only` 결과를 재분석한다.
- [ ] 병목 우선순위를 재정의하고 `scripts/render_bottleneck_backlog.sh`를 통해 다음 작업 단위를 반영한다.
- [ ] `007-imagemagick-images`, `019-grayscale-image`, `023-cmyk-image` 샘플 비교 포인트의 리샘플링 정책과 phase를 재검증한다.

## P0 정확도 작업
- [ ] `goal98` 실패 페이지 중 `decode_or_transform` 항목의 공통 원인을 정리하고 회귀 테스트를 추가한다.
- [ ] `resample_or_antialias` 항목에 대해 리샘플러, 안티에일리어싱, 좌표 phase 규칙을 재점검한다.
- [ ] `layout_or_transform`, `color_or_colorspace`, `minor_pixel_mismatch` 항목의 원인을 페이지 단위 픽셀 맵으로 분해한다.
- [x] `ImageCanvas` 이미지 배치가 회전, 기울기, 전단 CTM에서 축 정렬 스케일 위주 변환으로 축약되지 않도록 개선한다.
- [x] `internal/infrastructure/canvas/image_canvas.go`의 `DrawImageWithPhase`를 수정하고 회전/기울기 이미지 정합 테스트를 보강한다.
- [ ] `internal/domain/renderer/evaluator.go`의 `renderImageToCanvas` CTM 전달 방식과 샘플러 정책 reason 값을 보강한다.
- [ ] `tmp/goal98_rerun_affine_fix` 기준으로 단일 배치 재실행 후 `report.csv`와 HTML 비교 결과를 재확인한다.
- [ ] 실패 유형이 남으면 `tmp/goal98_rerun_affine_fix/bottleneck_backlog.md`를 갱신한다.

## P1 성능 작업
- [x] 이미지 샘플링 및 마스크 루프(sampleImageRGBA8, getGrayVal, getRGBAPixel, resolveSoftMaskDetails, decodeMappedRGBImage, decodeSeparationImage)에서 구체 타입 어설션을 사용해 인터페이스 메서드 호출 및 힙 할당을 완전히 제거함. 이를 통해 전체 메모리 사용량을 ~26% 감축하고 doc_049.pdf 및 doc_077.pdf의 OOM/timeout 문제를 해결함.
- [x] 전체 렌더링 검사 로직에 프로파일링 기능을 추가하고 메모리 사용량 및 렌더링 속도 병목 대상을 분석하여 docs/perf-targets.ko.md에 문서화함.
- [x] `freetypego_adapter.go` 내부에서 폰트 슬라이스의 포인터/길이 메타데이터(`fontKey`)를 기반으로 한 스레드 안전 `*LockableFace` 캐싱을 적용해 매 글리프 로드 시의 중복 폰트 파싱 및 슬라이스 확장 연산을 완전히 배제함. 이를 통해 페이지당 메모리 할당 객체 수를 62.4% 감축함.
- [x] raw image byte 변환에서 sample 단위 `os.Getenv` 호출을 제거하고, Splash 이미지 debug 로그를 gate 처리하며, 이미지 stream 치수 기반 FlateDecode buffer pre-size와 `pdfcompare`용 fast PNG 압축을 추가함.
- [ ] `vector.rasterizeDstRGBASrcUniformOpOver` 및 `draw.ablInterpolator` hotspot에 캐시, 버퍼 재사용, 분기 최소화 후보를 적용한다.
- [x] 성능 변경 후 `doc_077.pdf` p31 및 `doc_049.pdf` p8/p9 기준으로 프로파일을 재생성함. `pdfcompare` 기준 `doc_077` p31은 1512ms/297.5MB alloc에서 614ms/212.1MB로, `doc_049` p8은 1822ms/139.1MB에서 979ms/120.9MB로 개선됐고 fast PNG 출력은 pixel-identical로 확인됨.
- [x] 현재 느린 random-corpus 페이지들을 하나씩 프로파일링하고 최적화함. 96 DPI `doc_077.pdf` p31은 single-pass URW AFM metric parsing, Flate predictor in-place decode, RGB8 decode copy/LUT fast path, RGB8 bilinear direct write loop 적용 후 `0.82s` / `223,666,728` TotalAlloc / `138,880KB` MaxRSS에서 `0.51s` / `164,237,592` TotalAlloc / `134,668KB` MaxRSS로 개선됨. `doc_049.pdf` p8/p9는 PNG byte-identical을 유지하면서 TotalAlloc이 각각 `122,011,552 -> 94,165,976`, `127,052,792 -> 98,226,720`으로 감소함. 다음 CPU 후보는 `sampleImageRGBA8`, `expandRow`, bitmap/output image allocation 축이다.
- [x] 150 DPI `doc_100.pdf` page 4를 프로파일링하고 최적화함. arbitrary-transform 이미지 샘플링이 원본 크기가 아니라 scaled bitmap bounds를 기준으로 좌표/offset을 계산하도록 수정해 panic을 완료 렌더로 전환했고, page-sized Mono8 soft-mask bitmap을 pool로 재사용해 반복 full-page `Bitmap.ClearWithAlpha`/`NewBitmap` 비용을 제거함. page 4는 `38.88s` / `56,144,933,024` TotalAlloc / `268,948KB` MaxRSS에서 `2.41s` / `1,355,079,072` TotalAlloc / `254,720KB` MaxRSS로 개선됐고, post-bounds-fix baseline 대비 PNG 출력은 byte-identical임. 전체 `doc_100.pdf` page loop는 이제 error `0`이며 page 4만 1초를 초과한다.
- [x] 다음 `doc_100.pdf` 후보를 이어서 프로파일링함. non-linearized PDF에서 전체 파일 linearized-page scan을 건너뛰고, scanner intersection 구조를 줄이며, Splash state/XPath buffer와 XPath transform scratch를 재사용하고, cached-operator 임시 zero slice를 제거했다. PNG 출력은 모두 byte-identical을 유지했고, TotalAlloc은 page 7 `291,228,912 -> 172,685,952`, page 22 `324,616,336 -> 191,466,944`, page 8 `304,380,632 -> 160,194,168`로 감소했다. 최종 측정은 page 7 `0.61s` / `78,248KB` MaxRSS, page 22 `0.61s` / `93,120KB` MaxRSS, page 8 `0.51s` / `93,760KB` MaxRSS이며, RSS는 대체로 노이즈 범위였고 page 22는 기존 `91,520KB`보다 약간 높았다.
- [x] 2026-06-13 느린 페이지 후속 프로파일링을 이어서 수행함. fast PNG 경로는 filtered scanline 전체 버퍼를 만들지 않고 zlib로 row streaming하며, 필터 후보 생성과 abs-score 계산을 단일 pass로 합쳤다. `doc_049.pdf` 200 DPI page 9는 `1114ms` / `132,213,600` alloc에서 `323ms` / `79,895,784` alloc으로, page 8은 `979ms` / `126,822,680` alloc에서 `281ms` / `76,996,312` alloc으로 감소했고 PNG는 byte-identical이었다. `doc_077.pdf` 96 DPI page 31은 RGBA/Gray/Alpha fast pixel access와 bilinear expand-row plan 재사용 후 `614ms` / `222,395,664` alloc에서 `390ms` / `154,342,576` alloc으로 감소했다.
- [x] 2026-06-13 `doc_100.pdf` 150 DPI page 4를 추가로 프로파일링하고 최적화함. cached Form operator replay가 `e.operators` history를 증식하지 않게 하고, soft-mask pool bitmap은 page-wide clear 대신 image device bounds만 clear하며, cached Form parser operator slice capacity hint를 추가했다. 출력은 기존 PNG와 byte-identical이며 direct render는 `2.53s` / `248,640KB` MaxRSS / `919.51MB` alloc-space에서 `1.11s` / `190,080KB` MaxRSS / `848.28MB` alloc-space로 감소했다. CPU profile의 `runtime.memclrNoHeapPointers`는 `1.19s`에서 약 `0.04s` 수준으로 내려갔다.
- [x] `doc_100.pdf` 150 DPI page 4를 다시 프로파일링해 남은 할당 압력을 줄였다. soft-mask 임시 Splash는 vector-AA row buffer를 만들지 않고 mask rasterization 뒤 pooled state를 반환하며, 큰 drawing stream에 맞춰 cached Form operator capacity를 확장했다. 현재 direct render는 PNG byte-identical을 유지하면서 `1.12s` / `908,055,488` TotalAlloc / `186,560KB` MaxRSS에서 `1.01s` / `635,202,968` TotalAlloc / `186,720KB` MaxRSS로 개선됐다. decoded stream cache는 TotalAlloc은 줄였지만 retained heap/RSS를 늘려 제외했다. `doc_077.pdf` p31 및 `doc_049.pdf` p8/p9 회귀 렌더도 기존 PNG와 byte-identical이었다.
- [x] 현재 느린 `doc_100.pdf` 150 DPI 페이지들을 이어서 페이지 단위로 프로파일링했다. byte-mode lexer token peek는 heap escape가 발생하는 포인터 대신 값으로 저장하고, scanner debug flag를 캐시하며, scanner row sort는 작은/중간 row에서 allocation-free stable insertion sort를 쓰고 큰 row는 `sort.SliceStable`로 fallback하도록 했다. Indexed 8-bpc Gray/RGB 이미지는 palette를 직접 확장한다. page 4 direct render는 PNG byte-identical을 유지하면서 `1.02s` / `485,395,320` TotalAlloc / `182,240KB` MaxRSS에서 `0.92s` / `458,744,344` TotalAlloc / `176,320KB` MaxRSS로 개선됐다. 전체 31페이지 `pdfcompare` profile은 fresh baseline `5,842ms` / `2,773,343,112` alloc / `184,640KB` MaxRSS에서 `tmp/perf_after_peek_hybrid128_doc100_150_codex` 기준 `5,697ms` / `2,530,015,648` alloc / `177,440KB` MaxRSS로 개선됐다. all-insertion probe가 약간 더 빨랐지만 큰 row의 O(n²) 위험을 줄이기 위해 낮은 위험의 `128` row threshold를 유지했고, Flate decode size hint 확대는 TotalAlloc과 wall time이 늘어 제외했다.
- [x] fresh reprofile 이후 `doc_100.pdf` 150 DPI 느린 페이지 프로파일링을 이어서 수행했다. XRef 및 object-stream 파싱은 byte slice를 reader로 감싸지 않고 byte-slice lexer를 직접 사용하며, decoded Gray soft-mask 이미지는 다시 Gray로 복사하지 않고 재사용하고, bilinear expand-row plan은 공통 component count에 대해 재사용 및 특화한다. 전체 31페이지 `stage=ours` profile은 `tmp/perf_reprofile_doc100_150_20260613_codex`의 `5,716ms` / `2,532,901,008` alloc / `176,800KB` MaxRSS에서 `tmp/perf_doc100_expandfast_20260613_codex` 기준 `5,544ms` / `2,408,653,952` alloc / `172,800KB` MaxRSS로 개선됐다. page 4는 `867ms` / `463,582,480` alloc / `176,800KB` MaxRSS에서 `814ms` / `397,278,208` alloc / `172,800KB` MaxRSS로 개선됐고, focused page 7 렌더는 PNG byte-identical을 유지했다. operator capacity, scanner row-cap, evaluator color-cache, Splash `SolidColor` cache probe는 full-profile 또는 focused 측정에서 TotalAlloc/RSS가 증가해 제외했다.
- [x] 현재 `doc_100.pdf` 150 DPI 느린 페이지 세트를 계속 페이지 단위로 프로파일링했다. TrueType `glyf` 파서는 이미 알고 있는 glyph count로 prealloc하고, Splash transfer reset은 미리 만든 identity table 대입으로 바꾸며, 완전한 256-entry palette를 가진 8-bpc Indexed Gray/RGB 이미지는 항상 RGBA로 확장하지 않고 `image.Paletted`와 Splash row-source fast path를 사용한다. page 4 direct는 PNG byte-identical을 유지하면서 `413,702,168` TotalAlloc / `140,952,216` MemAlloc에서 `373,729,720` / `59,896,128`로 개선됐고, page 7 direct도 PNG byte-identical을 유지하면서 `149,992,872` / `32,792,208`에서 `133,899,624` / `22,124,016`으로 개선됐다. 전체 31페이지 profile은 `tmp/perf_doc100_expandfast_20260613_codex`의 `5,544ms` / `2,408,653,952` alloc / `172,800KB` MaxRSS에서 `tmp/perf_doc100_final2_20260613_codex` 기준 `5,476ms` / `2,363,050,456` alloc / `173,760KB` MaxRSS로 이동했다. operator capacity, clip pooling, scanner row preallocation, Indexed branch-only fast path, current-path reuse probe는 focused 또는 full-profile에서 wall time, retained heap, MaxRSS 중 하나를 악화시켜 제외했다.
- [x] 다음 `doc_100.pdf` 150 DPI Indexed-image 후보를 이어서 페이지 단위로 프로파일링했다. 8-bpc Indexed Gray/RGB 이미지가 작은 palette를 갖더라도 visible pixel index가 모두 palette 범위 안이면 `image.Paletted`로 유지해 RGBA 확장을 피하고, 잘못된 index가 있는 경우에는 기존 clamp fallback을 유지한다. page 22 focused render는 PNG byte-identical을 유지하면서 heap profile의 `decodeIndexedRGB8 -> image.NewRGBA` 할당이 사라졌고 `MemTotalAlloc`은 `108,936,920`으로 줄었다. page 7도 PNG byte-identical을 유지하면서 `123,608,152` TotalAlloc을 기록했다. 반복 full 31페이지 profile은 `tmp/perf_doc100_final2_20260613_codex`의 `5,476ms` / `2,363,050,456` alloc / `173,760KB` MaxRSS에서 `tmp/perf_doc100_indexedpal2_20260613_codex` 기준 `5,463ms` / `2,340,656,376` alloc / `171,840KB` MaxRSS로 개선됐다. 정확한 Flate decode hint를 encoded-size 추정보다 우선하는 probe는 첫 full run에서 wall time만 소폭 줄고 allocation/RSS가 늘어 제외했다.
- [x] `doc_100.pdf` 150 DPI page 4와 page 7의 느린 페이지 프로파일링을 이어서 수행했다. renderer debug-context 환경 gate를 캐시해 operator hot path의 반복 `os.Getenv` 호출을 제거했다. page 4는 PNG byte-identical을 유지했고 CPU profile의 `syscall.Getenv`는 약 `70ms`에서 sampling noise 이하로 내려갔으며 direct render user CPU는 `0.87s`에서 `0.73s`로 개선됐다. 전체 31페이지 profile은 `tmp/perf_doc100_debugenv_20260613_codex` 기준 wall time이 `5,463ms`에서 `5,294ms`로 줄었고 allocation은 사실상 동일했다(`2,340,656,376` -> `2,341,737,288`). MaxRSS는 `171,840KB`에서 `180,320KB`로 움직였지만 focused page-4 run에서도 보인 노이즈 범위로 봤다. no-record operator-history probe는 page 4 allocation을 줄이지 못하고 live heap을 악화시켜 제외했다. 다음 후보인 page 7은 PNG byte-identical을 유지하면서 `0.51s` / `125,105,696` TotalAlloc / `72,040KB` MaxRSS를 기록했고, 남은 hotspot은 scanner intersection storage/sorting 및 PNG row filtering이다. 다만 이전 scanner row-preallocation probe가 rejected 상태라 더 좁은 memory-safe 계획이 필요하다.
- [x] `doc_100.pdf` page 7에서 CLI PNG 출력 할당과 renderer 내부 할당을 분리해 추가 최적화했다. seek 가능한 출력 파일에서는 fast/no-compression PNG IDAT를 압축된 byte slice로 전부 materialize하지 않고 파일에 직접 streaming한 뒤 chunk length/CRC를 patch하며, 기본 best-compression parity 경로는 그대로 유지한다. focused page 7 fast render는 PNG byte-identical을 유지하면서 `0.51s` / `125,105,696` TotalAlloc / `72,040KB` MaxRSS에서 `0.51s` / `124,137,424` / `70,516KB`로 줄었다. 전체 31페이지 `stage=ours` profile은 allocation이 `2,341,737,288`에서 `2,310,284,576`으로, MaxRSS가 `180,320KB`에서 `180,000KB`로 줄었다. 단일 full run wall time은 `5,294ms -> 5,442ms`로 움직였지만 user CPU는 거의 같거나 낮아 이 변경은 memory 개선으로 유지하고, 다음 병목은 scanner storage/sorting으로 이어간다.
- [x] 현재 `doc_100.pdf` 150 DPI 느린 페이지를 계속 페이지 단위로 프로파일링했다. page-sized soft-mask bitmap은 `splashCanvas` scratch buffer로 결정적으로 재사용하고, `DecodeParms`가 없는 Flate 이미지 stream은 dictionary 기반 decode-size hint를 사용하며, soft-mask matte unblend는 임시 `*image.RGBA`를 in-place로 보정하고, `xpath.Scanner`는 row별 intersection 수를 먼저 센 뒤 row storage를 할당한다. focused page 4는 PNG byte-identical을 유지하면서 `396,831,720` TotalAlloc / `174,240KB` MaxRSS에서 `344,245,568` / `173,760KB`로 개선됐고, focused page 7도 PNG byte-identical을 유지하면서 `124,395,664` / `73,608KB`에서 `108,256,840` / `70,492KB`로 개선됐다. 전체 page-chunk profile은 `tmp/codex_perf/doc100_current_150`의 `2,306,849,376` alloc에서 `tmp/codex_perf/doc100_after_p7_150`의 `2,256,877,816` alloc으로 이동했다. page 4 allocation은 `366,977,784 -> 339,763,344`, page 7은 `122,940,680 -> 106,401,056`으로 줄었다. 다음 후보는 page 22/page 8이며, 남은 비용은 단일 transient allocation보다 page/output `NewBitmap`, 최종 `image.NewRGBA`, bilinear scaling, PNG filtering 쪽에 더 가깝다.
- [x] `doc_100.pdf` 150 DPI page 22, page 8, page 4를 이어서 페이지 단위로 프로파일링했다. `splashCanvas` current path storage를 재사용하고, single-point subpath가 없는 경우 `popplerDropOpenSinglePointSubpaths` 복사를 피하며, renderer path를 XPath로 바꿀 때 point storage를 미리 잡고, Type1 font `Length1/Length2/Length3` stream에만 정확한 Flate initial-buffer hint를 적용했다. 전체 31개 ours PNG는 기존 출력과 모두 byte-identical이었다. focused page 22는 `108,204,800` TotalAlloc에서 `97,576,712`로, page 8은 `108,808,880`에서 `94,601,640`으로, page 4는 `330,477,472`에서 `325,084,864`로 줄었고 page 4 MaxRSS는 `177,120KB -> 173,280KB`로 이동했다. 전체 31페이지 profile은 `tmp/codex_perf/doc100_after_p7_150`의 `5,466ms` / `2,256,877,816` alloc / `177,280KB` MaxRSS에서 `tmp/codex_perf/doc100_flateexact_150` 기준 `5,384ms` / `2,200,268,360` alloc / `166,560KB` MaxRSS로 개선됐다. page 8 allocation은 `108,036,496 -> 92,333,120`, page 22는 `106,712,280 -> 96,542,312`, page 4는 `339,763,344 -> 325,341,096`으로 줄었다. 남은 page 4 hotspot은 Flate/image/font decode buffer, `parseOperatorsOnly`, 고정 page/output `NewBitmap`/`image.NewRGBA`, scanner/clip storage다.
- [x] `doc_100.pdf` 150 DPI page 4 프로파일링을 이어서 수행했다. 신뢰 가능한 decoded-size hint가 있는 tiny Flate stream은 기본 `1024` byte initial buffer를 강제하지 않도록 바꿔, `50K`개가 넘는 작은 inline-image stream에서 반복 over-allocation을 제거했다. 큰 stream 추정 정책은 그대로 유지했다. focused page 4는 PNG byte-identical을 유지하면서 `326,742,752` TotalAlloc / `145,229,688` MemAlloc / `173,280KB` MaxRSS에서 `304,371,624` / `127,629,104` / `169,120KB`로 개선됐고, focused page 7은 `106,572,544` TotalAlloc로 사실상 변화가 없었다. 전체 31페이지 profile은 `tmp/codex_perf/doc100_flateexact_150`의 `5,384ms` / `2,200,268,360` alloc / `166,560KB` MaxRSS에서 `tmp/codex_perf/doc100_tinyhint_final_150` 기준 `5,375ms` / `2,178,933,920` alloc / `171,200KB` MaxRSS로 이동했다. page 4 allocation은 `325,341,096 -> 302,820,616`으로 줄었다. inline-image decoded-data cache probe는 focused page 4를 `318,043,672` TotalAlloc / `182,000,128` MemAlloc / `188,960KB` MaxRSS로 악화시켜 rejected 처리했다.
- [x] `doc_100.pdf` 150 DPI page 4를 이어서 프로파일링한 뒤 page 7/page 22/page 8을 재확인했다. 반복 soft-mask image detail은 evaluator 단위로 캐시하고, decoded entity stream은 작은 stream(`<=64KiB`)만 캐시해 반복 tiny image/SMask stream은 재사용하되 큰 page/Form decoded buffer는 붙잡지 않게 했다. focused page 4는 PNG byte-identical을 유지하면서 `0.72s` / `302,881,592` TotalAlloc / `166,880KB` MaxRSS에서 `0.63s` / `263,775,008` / `165,440KB`로 개선됐다. 전체 31페이지 `stage=ours` profile은 `tmp/codex_perf/doc100_reprofile_150`의 `5,440ms` / `2,177,041,040` alloc / `171,840KB` MaxRSS에서 `tmp/codex_perf/doc100_after_smaskcache_150` 기준 `5,302ms` / `2,138,600,888` alloc / `157,920KB` MaxRSS로 이동했고, 31개 ours PNG는 모두 byte-identical이었다. form-operator no-copy cache probe는 `TestFormOperatorCache_SetGetAndIsolation`이 cache isolation 계약 위반을 잡아 rejected 처리했다. page 7/page 22/page 8 profile의 남은 비용은 반복 decode보다 고정 bitmap/output, scanner/path, PNG filtering 쪽이다.
- [x] 2026-06-13 `doc_100.pdf` 150 DPI page 4/page 7/page 22를 계속 페이지 단위로 프로파일링했다. 함수 내부에서만 쓰는 fill/shading `xpath.Scanner`는 bounded pool로 row-count 및 intersection storage를 재사용하고, clip에 저장되는 장수명 scanner는 pool 대상에서 제외했다. 출력은 PNG byte-identical을 유지했고 full profile 기준 page 4 TotalAlloc은 `263,582,408 -> 258,153,480`, page 7은 `105,331,696 -> 99,345,440`, page 22는 `96,397,320 -> 85,846,416`, page 8은 `92,453,920 -> 86,216,680`으로 줄었다. 전체 31페이지 `stage=ours` profile은 `tmp/codex_perf/doc100_fresh_150`의 `5,336ms` / `2,138,510,200` alloc / `163,520KB` MaxRSS에서 `tmp/codex_perf/doc100_after_scannerpool_150` 기준 `5,322ms` / `2,100,105,952` alloc / `156,800KB` MaxRSS로 개선됐다. temporary image bitmap pool probe는 TotalAlloc은 줄였지만 전체 run이 `6,011ms` 및 `161,120KB` MaxRSS로 악화되어 rejected 처리했다.
- [x] scanner pool 이후 `doc_100.pdf` 150 DPI page 4/page 7/page 22 느린 페이지를 계속 프로파일링했다. cached Form/Type3 operator slice capacity는 decoded stream byte length만 보지 않고 문자열, name, comment, hex string, inline-image data를 건너뛰는 lightweight content-operator scan 기반으로 잡게 했다. Splash saved-state clip은 copy-on-write로 바꿔 반복 `q`/`Q`에서 현재 clip이 실제로 변경될 때만 path clip을 deep-copy한다. focused page 4는 PNG byte-identical을 유지하면서 `257,670,144` TotalAlloc / `145,615,512` MemAlloc / `156,320KB` MaxRSS에서 `250,548,288` / `134,234,760` / `153,920KB`로 개선됐고, focused page 7/page 22는 byte-identical 상태에서 거의 flat이었다. 전체 31페이지 `stage=ours` profile은 `tmp/codex_perf/doc100_current_run_150`의 `5,391ms` / `2,100,029,096` alloc / `163,360KB` MaxRSS에서 `tmp/codex_perf/doc100_cowclip_150` 기준 `4,981ms` / `2,084,966,808` alloc / `153,600KB` MaxRSS로 개선됐으며, 31개 ours PNG와 `report.csv`는 baseline과 byte-identical이었다.
- [x] `doc_100.pdf` 150 DPI 느린 페이지를 계속 page-by-page로 프로파일링했다. content-operator scan이 가장 큰 page-4 stream에서 `205,022`개 추정 대비 실제 `205,123`개로 충분히 정확함을 확인해, 큰 cached Form/Type3 operator stream에는 더 좁은 capacity slack을 적용한다. 이로써 해당 stream에서 불필요한 `Operator` slot 약 `51K`개를 피하면서 작은 stream slack은 유지했다. focused page 4는 PNG byte-identical을 유지하면서 `252,988,024` TotalAlloc / `136,945,424` MemAlloc / `153,440KB` MaxRSS에서 `248,428,968` / `130,320,520` / `158,560KB`로 이동했다. RSS 증가는 full run에서도 노이즈가 보여 개선 판단 기준에서 제외했다. 전체 31페이지 `stage=ours` allocation은 `tmp/codex_perf/doc100_cowclip_150`의 `2,084,966,808`에서 `tmp/codex_perf/doc100_opcap_150`의 `2,082,244,280`으로 줄었고, page 4 allocation은 `250,834,184 -> 247,585,576`으로 줄었으며, 31개 ours PNG는 모두 byte-identical이었다. page 7/page 22/page 8은 각각 `0.51s` / `99,955,992` TotalAlloc / `73,680KB` MaxRSS, `0.41s` / `83,143,760` / `68,480KB`, `0.41s` / `84,588,752` / `63,520KB`로 재프로파일링했고, 남은 hotspot은 반복 decode/capacity보다 고정 bitmap/output allocation, PDF file read, scanner/path curve storage, PNG filtering에 가깝다.
- [x] operator-cap baseline 이후 `doc_100.pdf` 150 DPI page 4 프로파일링을 이어서 수행했다. renderer `graphics.State` save/restore clone은 전용 pool을 재사용하고 restore 뒤 교체된 current state를 반환하도록 했다. focused page 4는 PNG byte-identical을 유지하면서 `248,738,440` TotalAlloc / `132,818,128` MemAlloc / `156,800KB` MaxRSS에서 `241,120,616` / `123,664,560` / `160,320KB`로 이동했고, pprof 기준 `cloneCurrentState` alloc은 `9MB`에서 `512KB`로 줄었다. 전체 31페이지 profile도 `report.csv`와 31개 ours PNG가 모두 byte-identical이며 `tmp/codex_perf/doc100_fresh_now_150`의 `4,854ms` / `2,082,271,344` alloc / `163,360KB` MaxRSS에서 `tmp/codex_perf/doc100_statepool_150` 기준 `4,753ms` / `2,070,054,280` alloc / `156,480KB` MaxRSS로 개선됐다. page 4는 `635ms -> 596ms`, `247,180,776 -> 238,870,120` TotalAlloc으로 줄었고, page 8과 page 22도 각각 `2,206,104`, `1,183,448` alloc bytes 감소했다.
- [x] state-pool 작업 이후 `doc_100.pdf` 150 DPI page 7을 재프로파일링하고 안전하지 않거나 mixed signal인 probe를 제외했다. focused page는 `0.41s`로 PNG byte-identical을 유지했다. `newPalettedImageSource` table pre-expansion은 CPU sample을 제거하고 focused `MemTotalAlloc`을 `99,913,088`에서 `99,580,472`로 줄였지만, 전체 31페이지 run은 MaxRSS가 `152,000KB`로 낮아진 대신 wall/alloc이 `4,753ms` / `2,070,054,280`에서 `4,787ms` / `2,071,454,624`로 악화되어 유지하지 않았다. 큰 scanner row용 typed stable merge sort도 PNG byte-identical이고 focused user CPU를 `0.29s -> 0.23s`로 줄였지만, focused `MemTotalAlloc`이 `100,631,736`, `MemAlloc`이 `41,318,784`로 악화되어 되돌렸다. 남은 page 7 비용은 고정 page/output bitmap, PDF file read, scanner/path work, image bilinear expansion, PNG filtering 쪽이다.
- [x] state-pool 이후 `doc_100.pdf` 150 DPI page 4/page 7/page 22 프로파일링을 계속하고 mixed probe를 제외했다. Type1/glyph-path preallocation은 focused page 7의 sampled path allocation을 줄였지만 반복 full profile에서 allocation이 `2,070,054,280`에서 `2,065,625,112`로만 움직이고 wall time과 page 4/page 7 allocation이 악화되어 되돌렸다. resource-stack slice-copy 제거 probe는 focused page 22 sampled object count를 `583k -> 519k`로 줄였지만 전체 wall/alloc이 `4,898ms` / `2,070,577,464`로 악화되어 되돌렸다. Indexed small-palette clamp probe는 31개 PNG와 `report.csv`를 byte-identical로 유지했지만 256-entry palette 방식과 custom clamp-image 방식 모두 full wall time을 `4,954ms`, `4,893ms`로 악화시키고 page 4/page 12/page 7 allocation을 늘려 되돌렸다. 남은 안전 후보는 path element representation, 고정 bitmap/output allocation, PNG filtering 같은 구조적 변경 쪽이다.
- [x] `doc_100.pdf` 150 DPI page 4/page 7을 계속 페이지 단위로 프로파일링했다. Splash scale scratch-buffer 재사용 probe는 focused p4/p7 PNG가 byte-identical이었지만 p4 focused allocation을 `241,000,640`에서 `242,534,920` TotalAlloc으로, p7을 약 `99MB`에서 `100,433,152`로 악화시켜 되돌렸다. cached `Operator` 저장 구조 compact는 유지했다. 사용되지 않는 `Resources` 포인터를 제거하고 드문 inline-image payload를 out-of-line으로 옮겼다. focused page 4는 PNG byte-identical을 유지하면서 `241,000,640` TotalAlloc / `124,627,272` MemAlloc / `159,680KB` MaxRSS에서 `233,759,976` / `114,873,400` / `149,280KB`로 개선됐다. 전체 31페이지 profile은 `report.csv`와 PNG가 모두 byte-identical이며 `4,888ms` / `2,071,477,568` alloc / `147,040KB` MaxRSS에서 `4,841ms` / `2,058,831,232` alloc / `144,960KB` MaxRSS로 이동했고, page 4는 `674ms -> 589ms`, `239,914,456 -> 231,755,432` alloc으로 줄었다. debug-context no-op defer skip probe는 focused p4가 `0.91s` / `121,431,944` MemAlloc으로 악화되고 TotalAlloc은 사실상 flat이라 되돌렸다.
- [x] `doc_100.pdf` 150 DPI page 7 및 scanner 메모리 압력 프로파일링을 이어서 수행했다. transient `xpath.Scanner` bounded pool의 보존 상한을 `64K`에서 `128K` entry로 올려 큰 반복 scanner row storage가 매번 폐기되지 않도록 했다. focused page 7은 PNG byte-identical을 유지하면서 `99,515,696` TotalAlloc / `72,572KB` MaxRSS에서 `95,441,704` / `70,584KB`로 줄었고, focused page 22는 `83,143,760`에서 `81,187,072` TotalAlloc으로 줄었으며 page 4는 사실상 flat이었다. 반복 full 31페이지 profile은 `report.csv`와 모든 PNG가 byte-identical이며 `4,841ms` / `2,058,831,232` alloc / `144,960KB` MaxRSS에서 `4,858ms` / `2,055,558,560` alloc / `144,960KB` MaxRSS로 이동했다. 첫 full probe에는 page 7 wall-time outlier가 있어 반복 run을 acceptance signal로 사용했다.
- [x] `doc_100.pdf` 150 DPI page 22의 path allocation 압력을 이어서 프로파일링했다. fill/stroke CTM 렌더링에서 일시적으로 만드는 renderer-to-XPath 변환 path는 pool로 재사용하고 즉시 반환하되, 장수명 canvas path는 pool 대상에서 제외했다. focused page 22는 PNG byte-identical을 유지하면서 `81,187,072` TotalAlloc / `49,810,232` MemAlloc / `66,080KB` MaxRSS에서 `78,225,960` / `43,145,728` / `58,720KB`로 줄었고, focused page 8은 `82,262,768` TotalAlloc에서 `78,274,560`으로 줄었다. page 4는 wall/RSS 노이즈가 있었지만 live heap은 크게 낮아졌다. 반복 full 31페이지 profile은 `report.csv`와 31개 ours PNG가 모두 byte-identical이며 `4,858ms` / `2,055,558,560` alloc / `144,960KB` MaxRSS에서 `4,875ms` / `2,047,566,696` alloc / `142,400KB` MaxRSS로 이동했다. `CGO_ENABLED=0 PDF_FREETYPE_GO=1 go test ./internal/infrastructure/splash/xpath ./internal/infrastructure/splash -count=1`도 통과했다. renderer `Rect` path element compact probe는 focused page 4의 `AddRect` sampled object를 `393,221`에서 `57,347`로 줄였지만, full run wall/RSS가 `4,875ms -> 4,945ms`, `142,400KB -> 145,440KB`로 악화되고 allocation 감소가 `0.2MB`뿐이라 rejected 처리했다. `Path.Clear` pointer-slot clearing도 focused page 4가 `1.01s` 및 높은 live heap으로 악화되어 되돌렸다.
- [x] 현재 `doc_100.pdf` 150 DPI 느린 page 4/page 7/page 8/page 22 프로파일링을 이어서 수행했다. 축 정렬 interpolated 이미지는 transform이 순수 scale/vertical flip일 때 원본 크기 affine buffer 대신 Splash scale-only path를 사용하도록 바꿨고, 31페이지 run에서 회귀 없이 exact percent가 page 2 `+0.13822936`, page 4 `+0.04962567`, page 7 `+0.05628045`, page 8 `+0.00014261`, page 22 `+0.11721925` 개선됐다. `blitImage`의 `pipe`는 pool로 재사용해 axis-interpolation run 대비 `report.csv`와 31개 ours PNG가 모두 byte-identical을 유지했고, 전체 `stage=ours` profile은 fresh baseline `4,964ms` / `2,048,618,712` alloc / `144,960KB` MaxRSS에서 `tmp/codex_perf/doc100_pipe_pool_150` 기준 `4,749ms` / `1,994,059,808` alloc / `143,680KB` MaxRSS로 개선됐다. page 4는 `670ms` / `232,055,368` alloc에서 `628ms` / `220,294,840`으로, page 7은 `321ms` / `93,772,680`에서 `266ms` / `76,453,128`로, page 22는 `267ms` / `77,165,552`에서 `234ms` / `61,486,232`로 줄었다. soft-mask matte lookup-table probe와 production form-cache no-copy probe는 focused/full 측정이 flat 또는 악화되어 rejected 처리했고, page 8의 남은 hotspot은 curve/path/glyph 작업과 고정 bitmap/output allocation이다.
- [x] `doc_100.pdf` 150 DPI page 4/page 7 느린 페이지를 계속 프로파일링하고 page 8을 재확인했다. content-operator hint가 `4096`을 넘는 큰 Form stream은 첫 실행 때 operator slice를 크게 캐시하지 않고 streaming 실행하며, 같은 evaluator에서 재사용될 때만 기존 cache 경로를 탄다. 큰 scanner row는 `sort.SliceStable`의 reflection swap 대신 `slices.SortStableFunc` typed stable sort를 사용해 equal-X 안정 순서는 유지하면서 CPU 비용을 낮췄다. 전체 31페이지 run은 `changed_ours_png 0`으로 byte-identical을 유지했고, `tmp/codex_perf/doc100_pipe_pool_150`의 `4,749ms` / `1,994,059,808` alloc / `143,680KB` MaxRSS에서 `tmp/codex_perf/doc100_typed_stable_150` 기준 `4,678ms` / `1,977,956,176` alloc / `115,360KB` MaxRSS로 이동했다. page 4는 `628ms` / `220,294,840` / `143,680KB`에서 `570ms` / `208,531,832` / `115,360KB`로, page 7은 `266ms` / `76,453,128`에서 `227ms` / `74,931,664`로 줄었다. unstable scanner sort probe는 PNG가 동일했지만 full run에서 wall/alloc signal이 섞여 기본화하지 않았고, page 8은 `Evaluator.curveTo`, rawPath/path element representation 같은 구조적 path/curve allocation 후보로 남겼다.
- [x] `doc_100.pdf` 150 DPI page 8을 이어서 프로파일링하고 page 4/page 22/page 7을 재확인했다. renderer path는 heap-allocated path-element interface object 대신 packed element metadata와 coordinate storage로 저장하고, Splash raw-CTM fill/stroke 변환은 legacy `[]PathElement` wrapper를 만들지 않고 renderer path를 직접 소비하도록 바꿨다. focused page 8은 PNG byte-identical을 유지하면서 TotalAlloc이 `75,566,584`에서 `70,072,664`로 줄었고, alloc profile에서 `Evaluator.curveTo`가 사라졌으며 `XPath.Reset`은 `4.50MB`에서 약 `1MB`로 내려갔다. 전체 31페이지 profile은 report 통계와 PNG가 모두 byte-identical이며 `tmp/codex_perf/doc100_reprofile_next_150`의 `4,720ms` / `1,978,885,896` alloc / `116,000KB` MaxRSS에서 `tmp/codex_perf/doc100_pathpacked_150` 기준 `4,662ms` / `1,966,509,016` alloc / `116,800KB` MaxRSS로 이동했다. page 8은 `267ms -> 249ms`, `74,595,800 -> 69,136,376` alloc으로, page 22는 `241ms -> 235ms`, `60,521,816 -> 56,794,472`로, page 4는 `586ms -> 584ms`, `208,476,832 -> 206,916,672`로, page 7은 `235ms -> 223ms`로 줄었다. current-path clone 제거 probe는 focused page 4가 byte-identical이었지만 TotalAlloc이 `208,584,208 -> 208,557,088`로만 움직이고 wall/RSS 개선이 없어 rejected 처리했으며, 낮은 위험의 packed-path 변경만 유지한다.
- [x] 비교 출력 오버헤드와 고메모리 페이지를 이어서 프로파일링했다. best-compression PNG parity 경로는 그대로 두고, `fast`/`none` PNG 출력은 filter-0 scanline을 직접 쓰며 Splash canvas에서는 최종 page bitmap을 `image.RGBA`로 확장하지 않고 RGB8 row를 바로 streaming한다. clipped image AA 루프에 남아 있던 debug env gate도 캐시했다. `doc_070.pdf` 300 DPI page 9는 decoded pixel이 동일한 상태(`bad 0`)에서 full page-profile 기준 사전 출력 baseline `719ms` / `231,220,792` alloc에서 `523ms` / `197,904,160` alloc으로 줄었다. 11페이지 `stage=ours` 합계는 `4,551ms` / `1,265,519,408` alloc / `148,960KB` MaxRSS에서 `2,720ms` / `895,311,296` alloc / `140,764KB`로 이동했다. `doc_100.pdf` 150 DPI는 `tmp/codex_perf/doc100_pathpacked_150` 대비 31페이지 report 통계가 모두 동일하고 `4,662ms` / `1,966,509,016` alloc / `116,800KB` MaxRSS에서 `3,382ms` / `1,706,759,592` alloc / `109,120KB`로 개선됐다. fast PNG byte는 기존 filtered-fast encoder와 더 이상 동일할 필요가 없으므로 이 경로는 decoded pixel/report 통계로 검증한다. 남은 page 9 메모리 hotspot은 최종 출력 확장이 아니라 고정 Splash `NewBitmap`, decoded stream buffer, 중간 image decode/conversion 쪽이다.
- [x] 2026-06-13 현재 baseline으로 느린 페이지를 다시 정렬했다. `page-chunk-size=1` 기준 `doc_070.pdf` 300 DPI page 9가 `465ms` / `173.1MiB`로 가장 큰 메모리 타깃이고, `doc_100.pdf` 150 DPI page 4가 `474ms` / `187.9MiB`로 뒤를 이었다. Splash의 큰 center-mapped explicit-nearest RGB upscale은 임시 scaled bitmap을 만들지 않고 기존 pipe 및 clip/AA 로직으로 직접 그리도록 바꿨다. focused `doc_070` page 9는 decoded pixel 동일성(`old_vs_new_bad 0`)을 유지하면서 `MemTotalAlloc`이 `182,520,360 -> 154,869,160`, live `MemAlloc`이 `73,752,144 -> 43,020,016`으로 줄었다. 보수적인 RGB8/no-alpha/clip-inside direct downscale path도 추가해 `YdownXdown` 평균을 임시 scaled bitmap 없이 바로 그린다. focused `doc_100` page 4는 pixel-identical을 유지하면서 `MemTotalAlloc`이 `198,341,096 -> 194,522,968`로 줄었다. full `doc_100` report 통계는 동일했고(`diffs 0`), page 4 allocation은 `197,001,312 -> 194,299,016`, 전체 `stage=ours` allocation은 `1,703,578,648 -> 1,701,724,856`으로 이동했다. full `doc_070` report 통계도 동일했고(`diffs 0`), page 9 allocation은 `181,492,480 -> 154,710,912`, 전체 `stage=ours` allocation은 `860,992,032 -> 834,183,928`로 개선됐다. `doc_100` page 4의 RGBA row-source probe와 parser dictionary-capacity probe는 pixel-identical이어도 TotalAlloc 또는 live heap을 악화시켜 rejected 처리했다.
- [x] 2026-06-13 `doc_070.pdf` 300 DPI page 9/page 11과 `doc_100.pdf` 150 DPI page 4를 이어서 페이지 단위로 프로파일링했다. ImageMagick/libjpeg stdout 수집은 이제 JPEG 치수로 알 수 있는 Netpbm/raw 출력 크기를 미리 잡아 `cmd.Output()`의 zero-capacity `bytes.Buffer` 성장을 피한다. focused `doc_070` page 9는 decoded-pixel 동일성(`bad 0`)을 유지하면서 TotalAlloc이 `172,223,192 -> 148,426,744`로 줄었다. full `doc_070` page-profile은 report 통계와 PNG가 동일했고(`diffs 0`, `changed_png 0`), `stage=ours` 합계가 `2,712ms` / `868,260,728` alloc에서 `2,527ms` / `816,818,512` alloc으로 이동했다. page 9는 `491ms -> 440ms`, `171,158,416 -> 148,447,264`로, page 11은 `453ms -> 349ms`, `116,027,640 -> 91,720,032`로 줄었다. full `doc_100` profile도 report 통계와 PNG가 동일했고(`diffs 0`, `changed_png 0`), `3,369ms` / `1,702,436,912` alloc에서 `3,325ms` / `1,701,040,408` alloc으로 소폭 개선됐다. 재프로파일링한 `doc_100` page 4는 여전히 다음 타깃이며 `195,407,360` TotalAlloc, 남은 hotspot은 Flate buffer growth, Form parsing, clip/path clone-on-mutation, 고정 Splash bitmap, scanner/path storage 쪽이다.
- [x] ImageMagick stdout pre-size 작업 이후 `doc_100.pdf` page 4와 `doc_070.pdf` page 9/page 11 프로파일링을 계속했다. release되는 soft-mask child `Splash`는 작은 object pool을 재사용하고, `q` graphics-state save는 current path가 PDF saved graphics state 대상이 아니므로 current/raw path를 deep-copy하지 않으며, `cloneCurrentState`는 사용하지 않는 embedded `graphics.State` current/clip path를 보관하지 않는다. Form/Type3/soft-mask 경계에서는 path-clearing 의미 보존에 필요한 비어 있지 않은 caller path만 선택적으로 보존한다. focused `doc_100` page 4는 PNG byte-identical을 유지했다. pprof 기준 soft-mask `Splash.New` struct allocation은 pool 적용 후 제거됐고(`210.88MB -> 188.33MB` total alloc-space), current/raw path clone 제거 뒤 focused profile은 `184.29MB`가 됐으며, 남은 `Path.Clone` sample은 필요한 clip snapshot 비용이다. 최종 full `doc_100` 31페이지 run(`tmp/codex_perf/doc100_selectivepath_full2_150`)은 `report.csv`와 PNG가 byte-identical이고 `3,387ms` / `1,699,590,280` alloc에서 `3,377ms` / `1,693,862,552` alloc으로 이동했다. page 4는 `505ms -> 507ms`, `193,466,432 -> 188,712,776` alloc으로 이동했다. 최종 반복 `doc_070` 300 DPI run(`tmp/codex_perf/doc070_selectivepath_full_300`)도 `report.csv`/PNG byte-identical을 유지했고 `2,527ms` / `816,818,512` alloc에서 `2,563ms` / `816,657,928` alloc으로 이동했다. page 9는 `440ms -> 456ms`, `148,447,264 -> 148,806,360` alloc으로 이동했고 page 11은 `349ms -> 354ms`, `91,720,032 -> 91,717,208` alloc으로 사실상 flat이었다. 기본 Flate `encodedLen*4` buffer-hint probe는 focused page 4 pprof가 `188.09MB`에서 `195.71MB` alloc-space로 악화되어(`MemTotalAlloc 203,982,264`) rejected 처리했다.
- [x] selective path 작업 이후 느린 페이지 프로파일링을 계속했다. 이미 decoded된 8bpc `/DeviceRGB` 이미지 중 default `/Decode`, ICC profile 없음, mask 없음, color-key mask 없음, matte 없음 조건에만 RGBA 확장을 건너뛰는 packed raw RGB fast path를 추가했다. 그 외 raw RGB 이미지는 이전 전역 `RGBImage` 전환에서 보였던 `RGBImage.At` allocation 회귀를 피하기 위해 기존 decoder 경로를 유지한다. focused `doc_070.pdf` 300 DPI page 9는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `133,729,744 -> 119,404,744`로 줄었고, focused `doc_070` page 11은 `71,834,400` TotalAlloc까지 내려갔으며 full-run wall-time outlier가 있던 page 7은 단일 재측정에서 `54,012,584`로 확인했다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지했고 사실상 flat이었다(`186,671,672 -> 186,934,456`, 반복 `187,190,080`), 대신 `image.NewRGBA` sample은 `12.40MB -> 9.46MB`로 줄었다. full `doc_070`과 `doc_100` run은 report 통계가 동일했고(`report_diff=0`) 모든 ours PNG가 byte-identical이었다(`changed=0`). `doc_070` allocation은 `816,657,928 -> 761,569,872`, page 9는 `148,806,360 -> 119,923,696`, page 11은 `91,717,208 -> 71,844,416`로 줄었고, `doc_100` allocation은 `1,693,862,552 -> 1,682,534,368`, page 4는 `188,712,776 -> 187,239,680`로 이동했다. Type1/font stream decoded-length hint probe는 focused page 4가 `187,519,752` TotalAlloc로 악화되고 `bytes.growSlice`도 개선되지 않아 rejected 처리했다.
- [x] packed raw RGB 경로 이후 느린 페이지를 계속 페이지 단위로 프로파일링했다. 장수명 clip scanner는 eager intersection 생성 뒤 transient XPath source 참조를 끊고, `ClipToPath`/rect-only clip은 pooled XPath buffer를 재사용한다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `187,830,848 -> 176,350,200`, live `MemAlloc`이 `77,295,088 -> 64,651,888`로 줄었다. 전체 31페이지 run도 report 통계와 PNG가 동일했고 total `stage=ours` allocation은 `1,682,534,368 -> 1,667,513,472`, page 4는 `187,239,680 -> 175,736,912`로 줄었다. full `doc_070.pdf` 300 DPI도 report/PNG 동일성을 유지하며 total allocation이 `761,569,872 -> 761,006,736`으로 줄었다. page 9는 대부분 고정 `NewBitmap` 및 Flate decode buffer 비용이고, `doc_100` page 12는 Type1 font parsing 지배라 더 넓은 font-cache 설계는 별도 후속으로 둔다.
- [x] clip/XPath 메모리 패스 이후 Type1-heavy 느린 페이지를 계속 프로파일링했다. embedded non-CFF Type1 font는 built-in encoding 적용 시 이미 파싱된 font의 `EncodingName` 데이터를 재사용해 `resolveEmbeddedType1Encoding`에서 두 번째 `type1.NewFontFromBytes` parse를 피한다. Type1C/CFF는 기존 raw-encoding merge 경로를 유지한다. focused `doc_100.pdf` 150 DPI page 20은 PNG byte-identical을 유지하며 `MemTotalAlloc`이 `76,198,800 -> 71,122,696`, page 12는 `77,749,848 -> 69,697,056`으로 줄었다. full `doc_100`도 report/PNG 동일성을 유지했고 total `stage=ours` allocation은 `1,667,513,472 -> 1,554,193,608`, page 12는 `76,890,128 -> 68,401,360`, page 20은 `75,412,232 -> 69,939,280`, page 4는 `175,736,912 -> 169,942,992`로 줄었다. full `doc_070` 300 DPI도 report/PNG 동일성을 유지하며 total allocation이 `761,006,736 -> 731,244,072`로 줄었다.
- [x] Type1 패스 이후 `doc_100.pdf` 150 DPI page 4 느린 페이지 프로파일링을 계속했다. embedded TrueType font는 decoded font bytes 기준으로 evaluator 내부에서 캐시하고, embedded font stream은 evaluator decoded-stream cache를 경유하며, PDF dictionary parser는 작은 capacity hint로 시작하고, matte/color-key/ICC/custom Decode 보정이 없는 안전한 soft-masked packed RGB8 이미지는 RGBA 확장 없이 그릴 수 있게 했다. focused page 4는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `171,157,480 -> 167,437,136`, live `MemAlloc`이 `96,751,832 -> 91,347,872`로 줄었다. full `doc_100`은 `tmp/codex_perf/doc100_perf_probe_20260613_150`에서 report/PNG byte-identical을 유지했고 total `stage=ours`가 `3,227ms` / `1,554,193,608` alloc / `111,680KB` MaxRSS에서 `3,201ms` / `1,547,531,664` alloc / `111,040KB`로 이동했다. page 4는 `506ms -> 498ms`, `169,942,992 -> 166,907,784` alloc으로 줄었다. full `doc_070`도 `tmp/codex_perf/doc070_perf_probe_20260613_300`에서 report/PNG byte-identical을 유지했고 total allocation은 `731,244,072 -> 730,162,040`으로 줄었지만 page 9는 사실상 flat/noisy라 다음 메모리 타깃으로 남긴다. 관련 패키지 테스트와 `git diff --check`는 `CGO_ENABLED=0 PDF_FREETYPE_GO=1` 조건에서 통과했다.
- [x] Type1/page-4 패스 이후 `doc_070.pdf` 300 DPI page 9 프로파일링을 계속했다. FlateDecode는 이제 정확한 decoded-size hint 이후에도 `io.Copy`가 `bytes.Buffer.ReadFrom`으로 과성장하지 않도록 write-only wrapper와 pooled 32KiB scratch buffer로 decompressed bytes를 복사한다. focused page 9는 PNG byte-identical을 유지했고 `MemTotalAlloc`이 `119,061,704`에서 안정적으로 `98.5-98.9M`까지 내려갔으며(pprof run `98,605,168`), `bytes.growSlice` sampled alloc-space는 `40.09MB -> 21.27MB`로 줄었다. full `doc_070`은 `tmp/codex_perf/doc070_flatenorf_full_300`에서 report/PNG byte-identical을 유지했고 total `stage=ours` allocation은 `730,162,040 -> 707,728,944`, page 9는 `117,701,848 -> 98,610,704`로 이동했으며 wall은 노이즈 범위였다(`2,408ms -> 2,450ms`). full `doc_100`도 `tmp/codex_perf/doc100_flatenorf_full_150`에서 report/PNG byte-identical을 유지했고 total allocation은 `1,547,531,664 -> 1,545,696,944`, page 4는 `166,907,784 -> 163,180,000`, MaxRSS는 `111,040KB -> 107,840KB`로 이동했다. vflip/YupXdown direct scaled-image path와 반복 대형 decoded-stream cache probe는 flat이거나 live heap을 키워 rejected 처리했다. 관련 패키지 테스트와 `git diff --check`는 `CGO_ENABLED=0 PDF_FREETYPE_GO=1` 조건에서 통과했다.
- [x] pure-Go JPEG decoding을 켠 상태로 느린 페이지 프로파일링을 계속했다. go-pdf JPEG decoder에서 djpeg-go RGB24 raster를 `Raster.RGBA`로 확장하지 않고 packed RGB8 이미지로 유지하도록 바꿨으며, djpeg-go 소스 변경은 필요하지 않았다. focused `doc_070.pdf` 300 DPI page 9는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `120,489,368 -> 105,703,176`으로 줄었다. 반복 full `doc_070` run은 report/PNG byte-identical을 유지했고 total allocation은 `730.1MiB -> 693.0MiB`, page 9는 `114.4MiB -> 100.0MiB`, page 11은 `96.1MiB -> 77.0MiB`로 줄었다. full `doc_100.pdf` 150 DPI도 report/PNG byte-identical을 유지했고 allocation은 사실상 flat이었지만 page 4 MaxRSS는 `108,960KB -> 106,560KB`로 낮아졌다. RGB8 soft-mask matte in-place probe는 focused page 4가 byte-identical이어도 `MemTotalAlloc`을 `164,094,200 -> 164,230,016`으로 악화시키고 남은 `image.NewRGBA` hotspot도 제거하지 못해 rejected 처리했다. Type1 embedded-font byte cache probe도 focused page 4에서 `MemTotalAlloc`이 `163,910,592 -> 164,070,232`, live `MemAlloc`이 `84,270,040 -> 85,799,752`로 악화되어 rejected 처리했다.
- [x] pure-Go JPEG RGB24 pass 이후 페이지 단위 느린 페이지 프로파일링을 계속했다. 현재 `page-chunk-size=1` 재정렬 기준으로 `doc_100.pdf` 150 DPI page 4가 1위, `doc_070.pdf` 300 DPI page 9가 2위였다. raw Form stream이 `128KiB` 이상인 대형 Form XObject Flate stream에만 보수적인 decode-size hint를 전달해, 고압축 drawing Form에서 반복되는 `bytes.Buffer` growth를 줄이고 이미지 및 작은 Form은 기존 경로를 유지한다. focused `doc_100` page 4는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `162,977,896 -> 156,575,640`, live `MemAlloc`이 `85,499,592 -> 82,989,672`로 줄었다. full `doc_100` run도 report/PNG byte-identical을 유지했고 total `stage=ours` allocation은 `1,545,046,776 -> 1,539,333,184`, page 4는 `162,551,560 -> 157,654,624`, MaxRSS는 `106,080KB -> 101,120KB`로 이동했다. full `doc_070` run도 report/PNG byte-identical을 유지했다. total allocation은 flat/noisy(`708,928,968 -> 709,367,376`)였고 page 9는 `576ms -> 408ms`, `99,290,008 -> 98,952,584`로 이동했다. TrueType `glyf` zero-copy slice probe는 focused page 4가 PNG-identical이어도 `MemTotalAlloc`을 `162,977,896 -> 164,044,792`, live `MemAlloc`을 `85,499,592 -> 87,582,496`로 악화시켜 rejected 처리했다. `doc_070` page 9 pprof의 남은 최상위 비용은 고정 300-DPI Splash bitmap 및 decoded Flate/image buffer였고 decoded pixel은 동일(`bad 0`)했으므로 낮은 위험의 page-9 추가 변경은 유지하지 않았다.
- [x] 대형 Form decode-size hint 이후 느린 페이지 프로파일링을 이어서 수행했다. soft-mask matte가 있는 raw RGB8 이미지는 canvas가 soft-mask drawing을 지원할 때만 3-byte packed RGB copy에 matte unblend를 직접 적용하도록 해, 다른 canvas 및 보정 이미지 fallback은 유지하면서 기존 RGBA 확장을 피한다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `156,575,640 -> 155,843,904`, live `MemAlloc`이 `82,989,672 -> 76,662,448`로 줄었고 pprof에서 해당 page의 `image.NewRGBA` sample이 사라졌다. full `doc_100` run은 report-identical을 유지하며 total `stage=ours` allocation이 `1,539,333,184 -> 1,536,227,808`, MaxRSS가 `101,120KB -> 96,800KB`, page 4 allocation이 `157,654,624 -> 154,197,384`로 이동했다. 반복 full `doc_070` 검증도 report-identical을 유지했고 total allocation은 `709,367,376 -> 708,249,872`, page 9는 `98,952,584 -> 98,638,032`으로 줄었으며 wall time은 반복 run 기준 노이즈 범위로 판단했다. generic `decodeRGBImage` packed-return probe는 soft-mask matte fallback에서 `image.NewRGBA`와 `RGBImage.At` allocation을 다시 만들었기 때문에 rejected 처리했고, resource-frame no-copy lookup probe도 focused page 4 allocation 개선이 없어 rejected 처리했다.
- [x] pure-Go JPEG 기준으로 `doc_100.pdf` 150 DPI page 4를 다시 프로파일링했다. pending clip 적용 시 큰 `renderer.Path` deep clone을 보관하지 않고, 실제 canvas clip은 원본 path로 즉시 적용한 뒤 renderer state에는 이미지/셰이딩 bbox 판단용 clip bounds만 저장한다. focused page 4는 `tmp/codex_perf/focused_doc100_p4_profile_20260613_codex` 대비 decoded pixel 동일성(`pixel_bad 0`)을 유지하면서 `MemTotalAlloc`이 `156,044,480 -> 150,338,560`, live `MemAlloc`이 `75,978,568 -> 71,588,168`로 줄었다. pure-Go full `doc_100` run은 `tmp/codex_perf/profile_doc100_rgb24fast_150_20260613` 대비 report 및 ours PNG가 byte-identical이고, total `stage=ours` allocation은 `1,547,569,336 -> 1,532,534,592`, MaxRSS는 `106,560KB -> 94,080KB`, page 4 allocation은 `164,887,312 -> 149,921,872`로 이동했다. full `doc_070` 300 DPI pure-Go 검증은 `tmp/codex_perf/profile_doc070_rgb24fast_repeat_300_20260613` 대비 report 및 ours PNG가 byte-identical이었고, page 9 allocation은 `104,817,032 -> 105,081,032`로 사실상 flat/noisy였다. 관련 renderer/splash/xpath 테스트와 `git diff --check`는 `CGO_ENABLED=0 PDF_FREETYPE_GO=1` 조건에서 통과했다.
- [x] pending-clip bounds 패스 이후 페이지 단위 프로파일링을 이어서 수행했다. 반복 soft-mask matte RGB8 XObject는 evaluator 내부에서 image stream과 mask stream 조합으로 캐시해 packed matte copy를 반복하지 않도록 했고, inline image 및 안전한 key가 없는 경로는 기존 fallback을 유지한다. full `doc_100.pdf` 150 DPI 검증은 `tmp/codex_perf/profile_doc100_clipbounds_150_20260613_codex` 대비 report와 ours PNG가 byte-identical이었고, total `stage=ours` allocation은 `1,532,534,592 -> 1,527,582,472`, page 4는 `449ms -> 433ms` 및 `149,921,872 -> 145,020,816` alloc으로 줄었다. full `doc_070.pdf` 300 DPI 검증도 report/PNG byte-identical을 유지했고 total allocation은 `726,892,440 -> 725,731,344`로 소폭 감소했다. page 9/page 11은 flat/noisy였으며 pure-Go page-9 pprof의 남은 최상위 비용은 고정 300-DPI Splash bitmap, djpeg decoded raster/color pipeline, Flate buffer, output/profile overhead였으므로 추가 저위험 page-9 변경은 유지하지 않았다.
- [x] raw RGB matte cache 이후 현재 느린 페이지 프로파일링을 이어서 수행했다. pure-Go JPEG 및 `page-chunk-size=1` 기준 재랭킹에서 current baseline `tmp/codex_perf/slowpage_current_20260613`은 `doc_100.pdf` 150 DPI page 4가 `430ms` / `144,199,920` alloc으로 1위였고, 그다음은 `doc_070.pdf` 300 DPI page 9 `381ms` / `105,070,480` alloc 및 page 11 `280ms` / `80,792,904` alloc이었다. streamed first-use Form 실행은 캐시하지 않고 즉시 실행하는 operator operands 복사를 피하도록 바꿨고, focused page 4는 PNG byte-identical을 유지하면서 `MemTotalAlloc`이 `145,571,648 -> 140,930,120`으로 줄었다. 비활성 image/scale trace pixel gate는 hot-loop trace matcher 진입 전에 short-circuit하도록 바꿨고, focused page 9는 PNG byte-identical을 유지하면서 trace matcher sample이 CPU top에서 사라졌지만 wall/alloc은 실질적으로 flat이었다. 최종 full 검증 `tmp/codex_perf/slowpage_tracefast_20260613`은 baseline 대비 report 및 ours PNG가 모두 byte-identical이었다. `doc_100` total `stage=ours` allocation은 `1,527,712,008 -> 1,519,834,032`, page 4는 `144,199,920 -> 139,028,480`으로 줄었다. `doc_070` total allocation은 noisy/flat(`725,424,688 -> 725,872,312`)이었고 wall은 `2,381ms -> 2,369ms`, page 9 allocation은 flat(`105,070,480 -> 105,086,760`)이었다. page 9의 남은 비용은 고정 300-DPI Splash bitmap, djpeg-go raster/color pipeline, decoded Flate image buffer, clip scanner storage다. 관련 renderer/splash/pdfrender 테스트와 `git diff --check`는 `CGO_ENABLED=0 PDF_FREETYPE_GO=1`로 통과했다.
- [x] `slowpage_tracefast_20260613` 이후 다음 느린 페이지들을 하나씩 프로파일링했다. focused `doc_100.pdf` 150 DPI page 4는 `MemTotalAlloc 141,294,320`이었고, 주요 할당은 고정 `NewBitmap`, PDF file read, `Dict.Set`, `Clip.Clone`, scanner storage, Type1/TrueType parsing이었다. `Clip.Clone` scanner slice sharing probe는 page 4 PNG를 동일하게 유지하며 focused `MemTotalAlloc`을 `140,380,176`으로 줄였지만, full 검증 `tmp/codex_perf/slowpage_clipshare_20260613`에서 report와 ours PNG는 동일한 상태로 `doc_070`/`doc_100` 합산 allocation이 악화되어 되돌렸다. parser dictionary capacity-16 probe도 focused page 4가 `152,189,072` TotalAlloc로 악화되어 되돌렸다. focused `doc_070.pdf` 300 DPI page 9는 PNG-identical 상태에서 `106,017,576` TotalAlloc이었고, image-sampling trace상 남은 임시 scaled bitmap은 DCT downscale과 X-down/Y-up Flate plot 이미지에서 나온다. 다만 temporary bitmap pool 및 scale scratch probe는 이미 full-run wall/RSS 회귀로 rejected 상태라 유지하지 않았다. focused page 11은 PNG-identical 상태에서 `80.05MB` alloc-space였고, CPU는 PNG fast zlib output, 고정 `ClearWithAlpha`, djpeg-go가 지배했다. 이번 profiling pass에서는 새 코드 변경을 유지하지 않았다.
- [x] `doc_100.pdf` page 4와 `doc_070.pdf` page 9/page 11 focused slow-page profiling을 이어서 수행했다. `Bitmap.ClearWithAlpha`는 uniform data/alpha plane을 doubling-copy fill로 채우고, Splash AA gamma table은 한 번만 생성한 LUT를 복사하며, AA 4x4 coverage는 기존 loop fallback을 유지한 unrolled fast path를 사용한다. fast PNG streaming의 `RGB8Scanline`은 alpha row가 전부 투명/불투명인 경우 흰색 fill 또는 RGB row copy로 바로 빠진다. focused 출력은 `doc_100` page 4, `doc_070` page 9, `doc_070` page 11 모두 PNG byte-identical을 유지했다. 최종 no-profile focused run은 page 4 `0.42s` / `139,105,168` TotalAlloc / `96,480KB` MaxRSS, page 9 `0.43s` / `99,261,360` TotalAlloc / `96,160KB` MaxRSS, page 11 `0.41s` / `72,161,784` TotalAlloc / `77,440KB` MaxRSS를 기록했다. Type1 embedded-font byte cache는 focused page 4 allocation/live heap을 악화시켜 계속 rejected이며, generic temporary scale-bitmap reuse도 이전 full-run wall/RSS 회귀 근거로 유지하지 않았다. 관련 `splash`, `renderer`, `pdfrender` 테스트와 `CGO_ENABLED=0 go build ./cmd/pdfrender`가 통과했다.
- [x] AA/PNG row fast path 이후 느린 페이지를 하나씩 다시 프로파일링했다. 현재 `page-chunk-size=1` 순위는 `doc_100.pdf` 150 DPI page 4가 1위(`402ms` / `140,466,240` alloc), 그다음이 `doc_070.pdf` 300 DPI page 9(`313ms` / `104,810,336`)와 page 11(`251ms` / `80,991,904`)이었다. RGB8/no-alpha/clip-inside direct `YdownXdown` 이미지 경로는 source-width `[]uint32` buffer 대신 destination-width RGB sum으로 누적하도록 바꿔 동일한 `xStep/yStep` 평균 계약을 유지하면서 임시 메모리를 줄였다. 새 internal test는 비정수 축소 비율에서 direct output과 기존 scaled-bitmap average가 동일한지 비교한다. focused `doc_100` page 4는 PNG byte-identical을 유지하며 `137,720,760` TotalAlloc / `95,840KB` MaxRSS를 기록했고, focused `doc_070` page 9/page 11은 각각 `98,935,000` / `96,160KB`, `72,016,464` / `77,760KB`였다. full 검증 `tmp/codex_perf/directdst_full_20260613`은 report-identical 및 ours PNG byte-identical을 유지했다. `doc_100` total `stage=ours` allocation은 `1,520,700,704 -> 1,518,317,072`, page 4는 `140,466,240 -> 137,904,808`로 줄었고, `doc_070` total은 `725,877,832 -> 725,457,128`로 줄었다. 재프로파일한 `doc_070` page 9의 남은 저위험 한계는 고정 300-DPI `NewBitmap`, Flate/JPEG decoded buffer, clip scanner storage다. 관련 `splash`, `renderer`, `pdfrender` 테스트와 `CGO_ENABLED=0 go build ./cmd/pdfrender`, `git diff --check`가 통과했다.
- [x] 현재 느린 페이지를 다시 페이지 단위로 프로파일링했다. parser의 PDF dictionary 초기 capacity를 `8`에서 `4`로 낮춰 `doc_100.pdf` page 4에서 반복되는 작은 dictionary bucket 과할당을 줄였고, 중간 크기 Form XObject Flate stream(`32KiB..128KiB`)에는 보수적인 decoded-size hint를 적용하되 기존 대형 Form `x16` hint는 유지했다. focused page 4는 현재 출력과 pixel-identical을 유지했고 sampled alloc-space는 `134.68MB -> 122.27MB`로 줄었다. full 검증 `tmp/codex_perf/slowpass_formdict_full_20260613`은 `tmp/codex_accuracy_work/avgprobe_full_20260613` 대비 report-identical 및 ours PNG byte-identical을 유지했다. `doc_100` total `stage=ours` allocation은 `1,518,475,144 -> 1,515,795,296`, page 4는 `137,945,680 -> 137,175,240`으로 줄었다. pure-Go JPEG 조건으로 `doc_070` page 9/page 11을 재프로파일한 결과 남은 주요 비용은 고정 300-DPI `NewBitmap`, djpeg-go raster/color pipeline, Flate decoded buffer, clip scanner storage였고 추가 저위험 변경은 유지하지 않았다. 반복 `doc_070` 검증은 report/PNG 동일성을 유지했으며 allocation은 noise/소폭 회귀 범위(`+0.18MB`, repeat `+0.42MB`), wall은 noise 범위(`+104ms`, repeat `+36ms`)였다. 관련 renderer/parser/stream/pdfrender/pdfcompare 테스트, `CGO_ENABLED=0` build, `git diff --check`가 통과했다.
- [x] 현재 `slowpass_formdict_full_20260613` baseline 기준으로 느린 페이지 프로파일링을 계속했다. content stream parsing은 renderer content stream에서만 numeric token string materialization을 건너뛰고 `Token`에 parsed numeric value를 보관하는 byte-slice lexer mode를 사용하며, 일반 lexer API의 `Token.Value` 호환은 유지한다. focused `doc_100.pdf` 150 DPI page 4는 pixel-identical을 유지했고 pprof alloc-space는 `137.38MB -> 136.25MB`로 줄었으며, no-profile 반복 측정 평균 `MemTotalAlloc`은 약 `138.3MB -> 135.7MB`로 이동했다. full 검증 `tmp/codex_perf/slowpage_numfast_full_20260613`은 report-identical 및 모든 ours PNG byte-identical을 유지했다. `doc_100` total `stage=ours` allocation은 `1,515,795,296 -> 1,514,196,280`, page 4는 `137,175,240 -> 136,318,984`로 줄었고, `doc_070`은 `726,487,048 -> 726,317,080`, wall `2,088ms -> 2,012ms`로 이동했다. 재프로파일한 `doc_070` page 9는 고정 300-DPI `NewBitmap`(`42.76MB`), decoded stream buffer(`11.98MB`), djpeg-go raster/color pipeline(`16.10MB`), scanner storage가 지배적이었고, page 11도 `NewBitmap`, djpeg-go, PNG zlib output이 지배적이라 추가 저위험 p9/p11 변경은 유지하지 않았다. parser/renderer/stream/pdfrender/pdfcompare 테스트, `CGO_ENABLED=0` build, `git diff --check`가 통과했다.
- [x] `slowpage_numfast_full_20260613` 이후 느린 페이지 프로파일링을 계속했다. renderer page rendering은 즉시 실행 경로에서 retained operator history를 끄고, decoded stream cache 보관 상한은 `2MiB`로 올려 페이지 안에서 반복되는 중간 크기 Flate image stream을 매번 다시 풀지 않게 했다. focused `doc_070.pdf` 300 DPI page 9는 5회 반복 모두 PNG byte-identical을 유지하며 `MemTotalAlloc`이 약 `104.5MB`에서 `99.6-100.1MB`로 줄었고, focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical 상태로 약 `135MB`에서 `132.3-132.9MB`로 줄었다. full 검증 `tmp/codex_perf/slowpage_iter_full2_20260613`은 `slowpage_numfast_full_20260613` 대비 report-identical 및 모든 ours PNG byte-identical을 유지했다. `doc_070` total `stage=ours` allocation은 `726,317,080 -> 717,950,584`, page 9는 `105,074,648 -> 99,585,520`으로 줄었고, `doc_100` page 4는 `136,318,984 -> 132,733,816`으로 줄었다. clip pooling은 focused 출력이 달라져 rejected 처리했고, operator-keyword switch는 측정 가능한 개선으로 주장하지 않고 parser cleanup으로만 유지했다.
- [x] `slowpage_iter_full2_20260613` 이후 현재 느린 페이지를 계속 프로파일링했다. 현재 page-profile 순위는 여전히 `doc_100.pdf` 150 DPI page 4가 1위(`406ms` / `132,733,816` alloc), 그다음이 `doc_070.pdf` 300 DPI page 9(`323ms` / `99,585,520`)와 page 11(`271ms` / `80,927,344`)이다. focused pprof에서 page 4의 남은 allocation은 고정 Splash bitmap, PDF file read, dictionary parsing, Type1/TrueType parsing, clip clone/scanner storage, decoded stream buffer였고, page 9는 고정 300-DPI `NewBitmap`(`44.09MB` alloc-space), Flate growth(`16.29MB`), file read, scanner storage가 지배적이었다. Type1 CharString `Command` value-return probe와 operand-pop probe는 모두 rejected 처리했다. overlay control build로 p4/p6/p11 출력은 byte-identical임을 확인했지만, no-profile focused memory 개선이 일관되지 않았고(`doc_100` p4 control `131,616,368` vs value-return `132,192,264` TotalAlloc), operand-pop은 p4/p9 focused run을 줄였지만 `doc_070` full total allocation을 `665,728` bytes 악화시켰다. 이번 pass에서는 production code 변경을 유지하지 않았고, 남은 저위험 후보는 decoded Flate buffer growth, 고정 bitmap lifetime/RSS, clip scanner storage다.
- [x] `tmp/codex_perf/slowpage_current_20260614` 기준으로 2026-06-14 느린 페이지 프로파일링을 이어서 수행했다. scanner pointer 공유 수명 때문에 path clip은 풀링하지 않고, scanner가 없는 `xpath.Clip`만 Splash state release 시점에 pool로 반환한다. focused `doc_100.pdf` 150 DPI page 4는 pixel-identical을 유지하며 `MemTotalAlloc`이 `132,998,776 -> 126,450,056`으로 줄었고, full `doc100_150` page-profile은 `2,931ms` / `1,508,942,768` alloc / `96,640KB` MaxRSS에서 `2,835ms` / `1,504,011,632` / `96,480KB`로 이동했다. focused `doc_070.pdf` page 9/page 11도 pixel-identical을 유지했으며, full `doc070_300`은 wall이 `1,964ms -> 1,936ms`로 소폭 좋아졌지만 allocation/RSS는 noisy/소폭 증가(`718,195,016 -> 718,909,248`, MaxRSS `140,360KB -> 141,928KB`)했다. page 9를 다시 pprof한 결과 남은 비용은 고정 300-DPI bitmap, djpeg-go raster/color pipeline, clip scanner storage, decoded Flate buffer, PNG output이었고 추가 변경은 유지하지 않았다. DCT scaled decode는 CTM/destination scale을 JPEG decode로 전달하는 구조 변경과 pixel parity 리스크가 커서 보류했다.
- [x] scannerless clip pool pass 이후 2026-06-14 느린 페이지 프로파일링을 계속했다. 이미지 디코딩은 이미지/마스크마다 새 decoder를 만들지 않고 evaluator 범위 lazy decoder를 재사용하며, 기본 8-bit DeviceGray soft mask는 SMask filter가 이미 완전히 해제된 경우 decoded raw bytes를 직접 `image.Gray`로 감싼다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지했고 full page-profile 기준 allocation이 `127,334,800 -> 125,759,056`, wall이 `402ms -> 384ms`, MaxRSS가 `98,240KB -> 96,800KB`로 이동했다. full `doc100_150` profile은 `2,913ms` / `1,503,761,648` alloc에서 `2,854ms` / `1,501,823,008` alloc으로 줄었다. `doc_070.pdf` 300 DPI focused page 9도 PNG byte-identical을 유지했고, 반복 full profile에서 MaxRSS는 flat인 상태로 total allocation이 `718,727,984 -> 718,441,736`, page 9는 `99,877,152 -> 99,602,704` alloc으로 이동했다. 남은 p4 비용은 고정 Splash bitmap, `Clip.Clone`/scanner storage, PDF file read, decoded Flate buffer, font parsing이다.
- [x] 남은 `Clip.Clone`/scanner slice 비용을 대상으로 2026-06-14 느린 페이지 프로파일링을 이어서 수행했다. `Clip.Clone`은 immutable scanner pointer slice를 `cap=len`으로 공유해 이후 `ClipToPath` append가 원본을 변경하지 않고 새 slice를 만들게 하며, `eo`와 `flags`는 기존처럼 deep-copy한다. clone append isolation을 검증하는 xpath unit test도 추가했다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지했고 `MemTotalAlloc`이 `125,782,920 -> 125,036,552`로 줄었다. 반복 full `doc100_150` 검증은 report-identical을 유지했고 total `stage=ours` wall/allocation/max-RSS가 `2,854ms` / `1,501,823,008` / `96,800KB`에서 `2,831ms` / `1,498,492,632` / `94,400KB`로 이동했으며, page 4 allocation은 `125,759,056 -> 125,273,488`로 줄었다. full `doc070_300`은 report-identical, wall `1,913ms` flat, max-RSS `142,400KB -> 142,240KB`였고 allocation은 `718,441,736 -> 718,858,952`의 작은 noise/소폭 회귀가 남았다. 다음 타깃은 page 9의 고정 bitmap과 decoded image/Flate buffer 비용이다.
- [x] `Clip.Clone` scanner-slice pass 이후 느린 페이지 프로파일링을 계속했다. Splash state restore 후 아래 saved state가 같은 clip을 더 이상 참조하지 않으면 해당 clip을 다시 owned 상태로 표시해, `Q`/restore 뒤 불필요한 copy-on-write `Clip.Clone`을 피한다. scanner를 가진 path clip은 참조 카운트가 없어 여전히 풀링하지 않는다. last-owner 및 nested shared-clip restore 동작을 검증하는 Splash state 테스트를 추가했다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지했고 `MemTotalAlloc` `126,170,120 -> 119,261,344`, `MemAlloc` `81,199,088 -> 78,366,320`, MaxRSS `95,360KB -> 93,280KB`로 이동했다. full `doc100_150` 검증은 report-identical을 유지했고 total `stage=ours` allocation `1,498,252,928 -> 1,492,457,816`, page 4 `125,125,848 -> 119,394,024`, max-RSS `96,320KB -> 92,800KB`로 줄었으며 wall은 flat/noise 범위(`2,910ms -> 2,900ms`)였다. full `doc070_300`도 report-identical을 유지했고 allocation은 flat/소폭 개선(`718,904,032 -> 718,778,424`)인 반면 wall/RSS는 noise 범위였다. page 9 재프로파일 결과 남은 비용은 고정 300-DPI Splash bitmap, djpeg-go raster/color pipeline/component buffer, decoded Flate growth, scanner storage, PNG output이며, direct scale path 확대나 djpeg-go iMCU-row streaming은 pixel parity 또는 구조 리스크가 커서 이번 pass에서는 추가 코드를 유지하지 않았다.
- [x] 현재 150 DPI random/current PDF corpus의 느린 페이지 프로파일링을 이어서 수행했다. Type1 `/Subrs` 파서는 비정상적으로 큰 선언 count에 맞춰 선할당하지 않고 실제 in-range entry까지만 증가하도록 바꿨고, renderer evaluator의 operator history 및 cache map은 lazy allocation으로 바꿔 tiling/pattern helper evaluator가 사용하지 않는 버퍼를 매번 만들지 않게 했다. focused `doc_027.pdf` page 45는 PNG byte-identical을 유지하며 `MemTotalAlloc`이 `154,142,536 -> 56,601,184`로 줄었고, `GeoTopo.pdf` page 95는 `167,477,440 -> 84,437,400`, `GeoTopo.pdf` page 55는 lazy evaluator 변경 후 `129,606,608 -> 81,794,952`로 이동했다. `GeoTopo.pdf`, `doc_027.pdf`, `doc_078.pdf` 269페이지 subset은 baseline 대비 report-identical(`report_changed_pages=0`)을 유지했고 total `stage=ours` allocation은 `22,717,943,368 -> 11,883,148,080`, wall 합계는 `22,885ms -> 21,119ms`로 줄었다. 남은 상위 allocation은 고정 Splash bitmap, 반복 tiling helper의 `NewGraphicsState`, file read, path storage, Type1 charstring decode 쪽이다.
- [x] lazy evaluator allocation 절감 이후 현재 150 DPI random/current corpus의 느린 페이지를 다시 하나씩 프로파일링했다. stroke outline path는 point/hint buffer를 미리 잡고 transient path를 pool에 반환하며, XPath stroke-adjust scratch를 재사용하고, object stream은 stream별 1회 파싱하되 decoded-data cache를 stream당 `4MiB` 및 전체 `16MiB`로 제한한다. Type1 FreeType name-index lookup도 렌더된 glyph 단위 lazy cache로 바꿨다. focused `GeoTopo.pdf` page 24/95/96은 PNG byte-identical을 유지했고, page 95는 object-stream cache 효과를 유지해 `MemTotalAlloc 67,809,136`, MaxRSS `60,000KB`를 기록했다. 최종 269페이지 검증 `tmp/codex_perf/slowpage_final_subset_150_20260614`는 baseline 대비 report-identical(`metric_changed_rows_vs_base=0`, `Exact100 0/269`)이며 total `stage=ours` allocation은 `11,883,148,080 -> 11,407,753,496`, wall 합계는 `21,119ms -> 20,782ms`, page-profile 최대 RSS는 `69,440KB -> 67,360KB`로 이동했다. 남은 저위험 후보는 고정 Splash bitmap allocation/lifetime, decoded image 및 soft-mask buffer, file read, Type1 charstring decode, scanner storage다.
- [x] 같은 150 DPI 269페이지 subset에서 느린 페이지를 계속 하나씩 프로파일링했다. Pattern/shading fill은 pooled Splash `pipe` 객체를 재사용하고, embedded Type1 font data는 evaluator 단위 content hash cache를 적용해 `newEmbeddedType1Font`와 embedded encoding fallback이 같은 font bytes를 두 번 파싱하지 않게 했다. focused `doc_027.pdf` page 48은 PNG byte-identical을 유지하며 `MemTotalAlloc`이 `50,818,960 -> 42,499,064`로 줄었다. focused `GeoTopo.pdf` page 24는 PNG byte-identical을 유지했지만 여전히 full-size Flate RGB image decode 후 downscale 구조가 지배적이라 Flate hint와 glyph-name memo probe는 rejected 처리했다. 최종 269페이지 검증 `tmp/codex_perf/type1cache_subset_150_20260614`는 `Exact100 0/269`를 유지했고, `tmp/codex_perf/pipepool_subset_150_20260614` 대비 total `stage=ours` allocation은 `11,394,902,200 -> 10,799,990,256`, wall 합계는 `20,294ms -> 19,421ms`, page-profile 최대 RSS는 `68,800KB -> 67,520KB`로 이동했다. 남은 wall 상위 페이지는 `GeoTopo.pdf` 96, 31, 97, 35, 55, 95, 24이며, 다음 안전 후보는 여전히 고정 Splash bitmap, decoded image/soft-mask buffer, file read, Type1 charstring decode, scanner storage 축이다.
- [x] Type1 font-data cache 이후 현재 `GeoTopo.pdf` wall/allocation 상위 페이지를 계속 프로파일링했다. generic soft-mask image stream은 이제 DeviceGray raw decode-size hint를 Flate decode로 전달해, generic stream heuristic에만 의존하던 `/SMask` stream의 buffer growth를 줄인다. encoded DCT/JPX/JBIG2 보존 경로는 변경하지 않았다. focused `GeoTopo.pdf` page 24/page 96은 PNG byte-identical을 유지했고 `MemTotalAlloc`은 각각 `75,409,608 -> 75,259,744`, `60,639,184 -> 60,429,544`로 이동했다. 최종 269페이지 검증 `tmp/codex_perf/smaskhint_subset_150_20260614`는 report-identical(`report_changed_rows=0`, `Exact100 0/269`)을 유지했고 total `stage=ours` allocation은 `10,799,990,256 -> 10,799,159,056`, wall 합계는 `19,421ms -> 19,282ms`로 줄었으며 page-profile 최대 RSS는 `67,520KB`로 동일했다. lazy Type1 path-generation probe는 focused run만 유리했고 이번 pass에서 full-subset 안전성을 확립하지 못해 유지하지 않았다.
- [x] 추가 성능 작업 전에 현재 `Exact100` blocker를 분석했다. `tmp/codex_perf/smaskhint_subset_150_20260614/report.csv`를 재정렬한 결과 최저 페이지는 텍스트 중심 `doc_078.pdf` page 6/7/4(`91.44940493%`, `91.73249219%`, `91.95608610%`)였고, near-exact 페이지에는 mismatch 10개뿐인 `GeoTopo.pdf` page 5가 포함됐다. `tmp/codex_exact/doc078_150_20260614` focused 이미지 보존 run에서 `doc_078.pdf` page 6 차이는 embedded Type1 Latin Modern 텍스트의 광범위한 glyph-edge mismatch이며, 누락 이미지나 ligature code mapping 문제가 아니었다. code 12/13 ligature는 없었고 `PDF_DEBUG_POPPLER_TEXT_SHIFT=1`도 exactness를 바꾸지 않았다. FreeType glyph probe들은 유지하지 않았다. hinting 활성화는 page 6을 악화했고(`91.44940493% -> 90.60368059%`), transformed glyph path 비활성화도 악화했으며(`83.51000637%`), pure-Go FreeType adapter 비활성화도 악화했다(`84.63523458%`). phase epsilon sweep도 page 6 또는 `GeoTopo.pdf` page 5를 개선하지 않았다. `GeoTopo.pdf` page 5의 잔여 mismatch 10개는 빨간 텍스트 antialias edge의 glyph-alpha ±1-3 차이이므로, 다음 exactness 작업은 PDF text positioning, image sampling, Splash compositing보다 freetype-go raster alpha parity를 우선해야 한다.
- [x] Exact100 blocker 분석 이후 현재 150 DPI 269페이지 subset의 느린 페이지를 다시 하나씩 프로파일링했다. `GeoTopo.pdf` page 31을 먼저 재프로파일링했고, 남은 안전 후보는 Type1 parsing/path setup과 AA scanner 작업이었다. Type1 glyph setup은 raw CharString slice 복사를 피하고, `CharCodeToGlyph` miss에서 synthetic empty glyph 객체를 만들지 않으며, fallback Type1 path generation은 실제 `RenderGlyph`가 필요할 때까지 지연한다. probe test는 더 이상 해당 allocation side effect에 의존하지 않도록 missing glyph를 허용하게 갱신했다. focused `GeoTopo.pdf` page 31은 PNG byte-identical을 유지했고 `MemTotalAlloc`은 `64,410,432`에서 약 `60.5-61.8MB`로 이동했으며, focused page 31/96/55/24는 기존 Poppler exact score를 유지했다. 전체 검증 `tmp/codex_perf/lazytype1_subset_150_20260614`는 `tmp/codex_perf/smaskhint_subset_150_20260614`와 report-identical(`changed_rows=0`, `Exact100 0/269`)을 유지했고 total `stage=ours` allocation은 `10,799,159,056 -> 10,373,016,072`, wall 합계는 `19,282ms -> 19,153ms`, page-profile 최대 RSS는 `67,520KB -> 67,040KB`로 이동했다. 남은 리더는 여전히 고정 Splash bitmap allocation/lifetime, Type1 charstring decode, file read, decoded image/soft-mask buffer, scanner storage다.
- [x] lazy Type1 pass 이후 `GeoTopo.pdf` page 96 focused profiling을 이어서 수행했다. renderer evaluation은 debug render context가 켜진 경우에만 `page.contents[n]` debug-path label을 만들도록 바꿔, 일반 렌더링의 page-96 CPU profile에서 `fmt.Sprintf` sample을 제거했다. 렌더 출력은 변경되지 않았다. focused page 96은 기존 Poppler score(`99.75444638`, mismatch `5,345`)를 유지했고, focused page 24/31/55/96은 lazy-Type1 출력과 PNG byte-identical이었다. 최종 focused `MemTotalAlloc`은 `tmp/codex_perf/debugctx_focus_final` 기준 page 24 `71,713,128`, page 31 `60,847,120`, page 55 `68,020,384`, page 96 `56,367,368`이었다. 광범위 재실행 `tmp/codex_perf/debugctx_subset_150_20260614`는 `GeoTopo.pdf` 단계에서 2분 이상 추가 진행이 없어 중단했으므로 broad baseline은 `tmp/codex_perf/lazytype1_subset_150_20260614`로 유지한다. 다음 broad check는 corpus를 문서/page range 단위로 쪼개거나 멈춘 `pdfcompare` run을 먼저 진단해야 한다.
- [x] `GeoTopo.pdf` page 97 focused profiling을 이어서 수행했다. Type1 CharString best-effort decoding은 lenIV 후보가 모두 실패한 경우에만 encrypted raw fallback bytes를 복사하도록 바꿔, 정상적으로 decoded candidate가 있는 경우 매번 버려지던 raw copy를 제거했다. focused page 97은 PNG byte-identical을 유지했고 `MemTotalAlloc`은 `45,714,904 -> 44,282,504`로 줄었다. `tmp/codex_perf/rawcopy_focus_final` 기준 focused page 24/31/55/96/97은 모두 이전 출력과 PNG byte-identical이었다. 최종 focused allocation은 page 24 `71,507,320`, page 31 `60,725,416`, page 55 `68,127,776`, page 96 `56,502,128`, page 97 `44,282,104`이며, page 55/page 96은 측정 noise/flat으로 본다. 남은 Type1-heavy allocation은 이제 버려지는 fallback raw copy가 아니라 실제 command/argument decode와 freetype-go Type1 token/path 작업이다.
- [x] 현재 `GeoTopo.pdf` 150 DPI 상위 느린 페이지들을 계속 page-by-page로 프로파일링했다. page 24는 먼저 재프로파일했지만 alloc-space가 full-size Flate RGB image 및 DeviceGray SMask decode buffer(`bytes.growSlice` `30.76MB`), PDF file read, 고정 Splash bitmap allocation, Type1 parsing에 지배되어 구조적 backlog로 남겼다. 안전한 개선은 추가 hint가 아니라 streaming/downscale decode 쪽이 필요하다. Type1/vector-heavy 페이지에 대해서는 transient stroke path와 fill-clip phase 비교 path가 pooled `xpath.Path` clone/transform storage를 쓰고 사용 후 release되게 했으며, Type1 font는 glyph render마다 FreeType source slice를 새로 만들지 않도록 생성 시 한 번만 구성한다. focused page 24/31/55/96/97은 `tmp/codex_perf/rawcopy_focus_final` 대비 모두 PNG byte-identical이었고, page 31 반복 no-profile `MemTotalAlloc`은 `60,725,416`에서 `58,990,520..59,477,792`로, page 96은 `56,502,128`에서 `54,691,280..55,158,200`으로 이동했다. 관련 `renderer`, `type1`, `splash`, `xpath`, `pdfrender` 테스트, `CGO_ENABLED=0` build, `git diff --check`가 통과했다.
- [x] path/Type1 pass 이후 `GeoTopo.pdf` page 55를 계속 page-by-page로 프로파일링했다. 이 페이지는 tiling pattern replay가 tile마다 새 helper `Evaluator`를 만들면서 focused pprof에서 `NewEvaluator`/`NewGraphicsState`가 약 `18.02MB` alloc-space를 차지하는 구조가 병목이었다. Tiling replay는 이제 pattern fill마다 evaluator 하나를 재사용하고 tile 사이에는 graphics/text/path volatile state만 reset하며, 안전한 per-pattern cache는 유지한다. renderer 소유 `graphics.State`는 사용하지 않는 legacy current-path allocation도 생략할 수 있게 했다. focused page 55는 `tmp/codex_perf/rawcopy_focus_final` 대비 PNG byte-identical을 유지했고 반복 no-profile `MemTotalAlloc`은 `68,127,776`에서 `49,600,360..49,804,664`, wall은 `0.10..0.11s`로 이동했다. focused page 24/31/55/96/97은 모두 PNG byte-identical을 유지했고 total allocation은 `259,082,736 -> 241,445,536`으로 줄었다. GeoTopo 단독 117-page `pdfcompare` 결과 `tmp/codex_perf/tileeval_geotopo_150_20260614`는 `tmp/codex_perf/lazytype1_subset_150_20260614` 대비 모든 page metric이 동일했다. GeoTopo `stage=ours` allocation은 `4,940,736,208 -> 4,863,669,560`, wall은 `7,947ms -> 7,813ms`로 줄었고 max page-profile RSS는 noise/소폭 증가(`67,040KB -> 68,800KB`)로 남았다. 관련 `graphics`, `renderer`, `splash`, `xpath`, `pdfrender`, `pdfcompare` 테스트, `CGO_ENABLED=0` build, `git diff --check`가 통과했다.
- [x] `tmp/codex_perf/tileeval_geotopo_150_20260614` 이후 page-by-page profiling을 계속했다. Type1 PFA/PFB parser는 retained raw data를 parser input slice로 alias하고, PFA font stream 전체를 string으로 변환하지 않고 byte slice에서 `eexec`를 찾도록 바꿨다. scanner intersection은 `X0`, `X1`, `Count`를 `int32`로 저장한다. focused page 24/31/55/96/97은 PNG byte-identical을 유지했고, Type1 raw-data 변경 후 page 31 allocation은 약 `59.1-59.7MB`에서 `57.9-58.6MB`로, scanner storage 변경 후 page 96은 `53.6MB`에서 `52.0-52.4MB`로 이동했으며 focused 5-page total은 `227,795,648` `MemTotalAlloc`까지 내려갔다. GeoTopo 단독 117-page 검증 `tmp/codex_perf/int32_geotopo_150_20260614`는 `tmp/codex_perf/tileeval_geotopo_150_20260614` 대비 report-identical(`changed_rows=0`)을 유지했고 `stage=ours` allocation은 `4,863,669,560 -> 4,763,304,464`, max page-profile RSS는 `68,800KB -> 68,000KB`로 줄었다. wall은 단일 실행 noise/소폭 악화(`7,813ms -> 7,964ms`)로 본다. `CharStringDecoder` command/stack preallocation probe는 page 31이 `64.5-65.1MB`로 악화돼 rejected 처리했다. 별도 `freetype-go` Type1 private-token numeric precheck는 이후 `github.com/dh-kam/freetype-go v0.1.2`로 release 및 소비가 완료됐으므로 이 항목의 dependency bump 작업은 남아 있지 않다. 현재 slow-page profile에는 그 bump 이후에도 실제 Type1 token/path decode 비용이 잔여 비용으로 남아 있다. 남은 리더는 고정 Splash bitmap, PDF file read, page 24의 full-size Flate RGB/SMask downscale, Type1 CharString/freetype-go path decode, scanner AA CPU다.
- [x] restored-clip ownership pass 이후 느린 페이지 프로파일링을 계속했다. byte slice에서 로드하는 TrueType font는 이제 전체 `glyf` table을 즉시 파싱하지 않고 실제 요청된 glyph record만 lazy parse/cache하며, 직접 parser caller용 `ParseFontFile`은 기존처럼 eager parse를 유지한다. focused `doc_100.pdf` 150 DPI page 4는 PNG byte-identical을 유지했고 `MemTotalAlloc` `110,508,488 -> 104,112,080`, live `MemAlloc` `69,352,168 -> 61,124,304`, MaxRSS `85,280KB -> 77,280KB`로 이동했다. full `doc100_150` page-profile은 `tmp/codex_perf/lazyglyph_slowdocs_20260614`에서 report-identical을 유지했고 page 4 allocation은 `119.93MB -> 99.00MB`, RSS는 `94.5MB -> 76.1MB`로 줄었다. full `doc070_300`도 report-identical을 유지했으며 total allocation은 `684.94MB -> 648.38MB`, page 9는 `94.98MB -> 89.77MB`, RSS plateau는 `169.1MB -> 138.8MB`로 이동했다. lazy glyph cache unit coverage도 추가했다. 의심했던 `sourceAlpha` 축은 재확인했지만 Splash image 변경은 유지하지 않았다. 일반 이미지와 soft-mask source는 이미 non-alpha path를 사용하고 있고, 남은 page 9 비용은 exactness-sensitive scale-then-flip/decoded image buffering, 고정 300-DPI bitmap, djpeg-go raster/color pipeline, scanner storage, Flate buffer, PNG output이다.
- [x] `tmp/codex_perf/slowpage_now_20260614` 기준으로 현재 느린 페이지 프로파일링을 이어서 수행했다. 최신 상위 페이지는 `doc_100.pdf` 150 DPI page 4(`393ms` / `98.41MiB` alloc), `doc_070.pdf` 300 DPI page 9(`348ms` / `89.77MiB`), page 11(`310ms` / `74.75MiB`)이다. 일반 resource lookup은 object만 필요한 경우 resource-frame slice를 할당/복사하지 않도록 바꾸고, inherited frame이 필요한 Pattern/Shading 경로는 기존 복사 semantics를 유지했다. focused `doc_100` page 4는 PNG byte-identical을 유지했고 control `MemTotalAlloc`은 `103,477,512 -> 103,022,584`로 이동했다. resource-name slash cache probe는 focused allocation을 `103,524,664`로 악화시켜 rejected 처리했다. full 검증은 `tmp/codex_perf/slowpage_noframes_20260614/doc100_geotopo_150` 및 `doc070_300` 모두 report-identical이었다. 150 DPI run은 wall이 flat/소폭 개선됐지만 allocation은 noise/소폭 회귀(`10,629ms -> 10,580ms`, `5,989,357,920 -> 5,991,144,000`)였고, `doc_070` 300 DPI는 소폭 개선됐다(`2,103ms -> 2,026ms`, `679,974,672 -> 679,540,136`). `doc_070` page 9 재프로파일 결과 남은 비용은 구조적이다: 고정 300-DPI `NewBitmap`(`43.03MB` alloc-space), Flate buffer growth(`16.29MB`), PDF file read, 실제 scanner intersection storage, image decode/scale 작업이며, 더 낮은 위험의 page-9 bitmap/scanner 변경은 유지하지 않았다.
- [x] 현재 느린 페이지 리더인 `doc_100.pdf` 150 DPI page 4와 `doc_070.pdf` 300 DPI page 9를 focused profiling으로 계속 확인했다. direct-nearest image scaling probe는 PNG-identical이었지만 wall이 noisy/악화되고 큰 temporary bitmap 비용을 제거하지 못해 되돌렸으며, parser raw-name 및 dict-capacity probe도 flat 또는 회귀라 rejected 처리했다. Standard14 URW OTF 데이터는 package init에서 모든 substitute raster font를 읽지 않고 `sync.Once`로 실제 사용 시 lazy-load하도록 바꿨고, `SourceExactGlyphBlend`와 URW 미설치 fallback 동작은 유지했다. old/new no-pprof focused 비교는 PNG byte-identical을 유지했다. page 4는 `MemTotalAlloc` `102,961,648 -> 101,537,688`, live `MemAlloc` `62,601,624 -> 43,723,864`, MaxRSS는 `77,280KB -> 81,440KB`로 noisy했고, page 9는 `88,211,296 -> 86,691,008`, live `72,584,456 -> 71,146,232`, MaxRSS `85,280KB -> 83,360KB`로 이동했다. `CGO_ENABLED=0 PDF_FREETYPE_GO=1 go test ./internal/infrastructure/font/standard ./cmd/pdfrender`는 통과했다. 남은 page 9/page 4 비용은 여전히 고정 Splash bitmap, decoded image/Flate buffer, scanner storage, PDF file read, 실제 font/path decode 작업이다.
- [x] Standard14 lazy-load pass 이후 현재 느린 페이지를 다시 하나씩 프로파일링했다. `--png-compression fast` 기준 focused no-pprof 현재값은 `doc_100.pdf` 150 DPI page 4가 약 `101.5-102.0MB`, `doc_070.pdf` 300 DPI page 9가 `86.5-86.8MB`, page 11이 `68.1-68.5MB` `MemTotalAlloc`이었다. CPU profile에서 기본 best-compression PNG 출력은 `pdfcompare` 실제 경로가 아니므로 제외했고, 실제 `fast` 경로의 남은 병목은 고정 Splash page bitmap, decoded Flate/image buffer, scanner storage, PDF file read, 실제 font/path 작업으로 확인했다. opaque page-alpha 및 scratch bitmap pool probe는 PNG 변경 또는 live heap/RSS 증가가 있어 rejected 처리했다. `pdfcompare`에는 `-ours-png-compression` 옵션을 추가했다(`fast` 기본, `none` 선택 가능). profiling run은 `none`으로 큰 임시 PNG를 감수하는 대신 ours render 시간을 줄일 수 있고 decoded pixel은 바뀌지 않는다. focused probe에서 `none`은 pixel-identical을 유지하며 `doc_070` page 9 wall을 `0.31-0.32s -> 0.27-0.28s`, page 11을 `0.27-0.28s -> 0.20-0.22s`로 줄였다. full `doc_070` 300 DPI `pdfcompare -keep-images=false`는 report metric 동일성을 유지했고 `stage=ours` wall/allocation은 `2600ms / 650,522,864 -> 1819ms / 644,890,520`으로 이동했다. `CGO_ENABLED=0 PDF_FREETYPE_GO=1 go test ./cmd/pdfcompare ./cmd/pdfrender` 및 `go build ./cmd/pdfcompare ./cmd/pdfrender`가 통과했다.
- [x] 현재 느린 리더들을 계속 page-by-page로 프로파일링했다. `tmp/codex_perf/slowpage_current_recheck_20260614` 기준 최상위는 `doc_100.pdf` page 4(`385ms`, `119.33MiB` alloc), `doc_070.pdf` 300 DPI page 9(`304ms`, `95.24MiB`), page 11(`243ms`, `76.99MiB`)이었다. Form XObject는 이제 첫 사용은 streaming 실행하고 두 번째 사용부터 parsed operator를 cache해 단발성 form operator allocation을 줄인다. broad 검증 `tmp/codex_perf/slowpage_formstream_verify_20260614`에서 `doc_100`은 report metric 동일성을 유지했고 total `stage=ours` allocation은 `294.68MiB` 감소했으며, page 4 allocation은 `119.33MiB -> 96.80MiB`, RSS는 `94.1MiB -> 79.2MiB`로 이동했다. `doc_070`은 page 6과 page 11만 변경됐고 둘 다 Poppler 방향으로 개선됐다(`99.44664290 -> 99.44988711`, `99.93407011 -> 99.93628045`); total allocation은 `66.32MiB` 줄었고 wall은 noise/소폭 악화(`+50ms`)였다. parser dictionary capacity 8 probe는 mixed/regressive라 rejected 처리했고, `djpeg-go`는 page 9에서 PNG-identical이었지만 allocation/RSS가 증가해 rejected 처리했다(`86.6MiB -> 92.6MiB`, RSS `83.4MiB -> 91.2MiB`). stdlib JPEG fallback은 픽셀이 달라 제외했다. 남은 page 9 비용은 고정 300-DPI page bitmap, retained decoded DCT/Flate RGB image buffer, scanner storage, exactness-sensitive image scaling이다.
- [x] 현재 가장 큰 exactness mismatch였던 `doc_070.pdf` page 4를 수정했다. `tmp/codex_exact/doc070_p4_regions_20260614` 시각 확인 결과 ours는 상단 Form XObject figure 이후 렌더링이 멈춘 반면 Poppler는 본문 텍스트를 계속 그렸다. 해당 figure는 `/Im3` transparency-group Form XObject이고 resource에 alpha ExtGState(`ca .49804`)가 있다. form group을 끄는 probe는 `91.31856209 -> 99.33504456`으로 유효했지만 너무 넓은 변경이라 제외했다. 최종 수정은 Form transparency group을 Form BBox clip이 아직 활성인 상태에서 먼저 composite하고, 그 다음 caller graphics state를 restore해 Form clip이 후속 page content로 leak되지 않게 한다. focused/broad 검증 `tmp/codex_exact/formgroup_paintfirst_verify_20260614`에서 확인한 변경은 각 set당 한 페이지뿐이었다: `doc_070` page 4는 `91.31856209 -> 99.66288770`(`+702,175` matched pixels), `doc_100` page 4는 `94.85076649 -> 95.35282234`(`+10,562` matched pixels)로 개선됐다. Exact100은 아직 `0/11`, `0/31`이며 다음 `doc_070` 최악 타깃은 page 9의 `97.34452763`이다.
- [x] Form transparency-group exactness 수정 이후 현재 느린 페이지 리더들을 다시 프로파일링했다. `tmp/codex_perf/slowpage_focus_20260614`의 focused `pdfrender --png-compression fast` 반복 측정에서 `doc_100.pdf` page 4가 여전히 wall/allocation 1순위(`0.38..0.41s`, 약 `101.4..101.9MB` `MemTotalAlloc`)였고, 그 다음은 `doc_070.pdf` page 9(`0.31..0.33s`, 약 `86.4..86.7MB`)와 page 11(`0.27..0.28s`, 약 `68.1..68.5MB`)이었다. `doc_100` page 4 pprof는 수천 개의 작은 Flate RGB+SMask image resource를 가진 큰 Form XObject가 지배하며, xref fetch, 작은 decoded stream, soft-mask details, raw RGB matte image는 이미 안전한 cache 경로를 탄다. hot resource slash-key probe는 PNG-identical이었지만 allocation/wall이 noise 수준이라 유지하지 않았다. `doc_070` page 9 pprof의 남은 비용은 구조적이다: 300-DPI Splash bitmap(`347.35MB/8회` alloc-space), ImageMagick/libjpeg decoded DCT buffer(`84.93MB/8회`), Flate buffer(`47.89MB/8회`), scanner storage(`~41.60MB/8회`), PNG flate output이다. Form transparency group은 이미 cropped temporary bitmap을 사용한다(`doc_100` page 4 group bitmap `703x404`, `doc_070` page 4 group bitmap `2143x658`)는 것도 trace로 확인했으므로 추가 group-bitmap 축소는 유지하지 않았다. 다음 안전 후보는 streaming/downscale image decode, PNG roundtrip 없는 raw output 비교, 또는 더 깊은 Splash bitmap lifetime reuse처럼 설계 변경이 필요하다.
- [x] `doc_070.pdf` page 9와 `doc_100.pdf` page 4의 slow-page pass를 계속 진행했다. page 9 focused 분석에서 가장 큰 mismatch는 누락된 page content가 아니라 `/Im5` Form의 screenshot image stack 내부에 집중돼 있었다. 기존 image-sampling mode(`adaptive-dct-iccbased-v1`, `experimental-splash-scale-only-v1`, `experimental-indexed-origin-downscale-phase-v1`)는 legacy와 출력이 동일했고, top-down downscale 비활성화는 page 9를 소폭 개선했지만(`97.34452763 -> 97.34998217`) 렌더가 느려져(`0.292s -> 0.363s`) exactness probe로만 남기고 유지하지 않았다. 유지한 성능 변경은 vertical-flip RGB downscale이 clip 전체 내부, no-alpha, no-post-transform 조건일 때 임시 scaled bitmap 없이 page bitmap에 직접 그리게 하는 것이다. 샘플링 수학은 그대로 유지한다. old/new binary 비교에서 page 9와 page 4는 PNG-identical이었고, page 9 `fast` 반복 평균은 `0.328s / 86,641,753` `MemTotalAlloc`에서 `0.322s / 86,537,739`로, page 4 focused allocation은 `102,084,096 -> 101,267,480`으로 이동했다. `tmp/codex_perf/vflip_direct_doc070_20260614`의 full `doc_070` 300 DPI 검증은 report-identical(`changed=0`, `Exact100 0/11`)이었다. vflip direct path가 기존 scaled+blit path와 동일한 출력을 내는 Splash unit coverage를 추가했다.
- [x] vflip pass 이후 `tmp/codex_perf/slowpage_current2_20260614` 기준 현재 느린 리더들을 다시 프로파일링했다. 현재 page-profile 순위는 여전히 `doc_100.pdf` 150 DPI page 4(`381ms`, `96.44MiB` alloc), `doc_070.pdf` 300 DPI page 9(`306ms`, `82.17MiB`), page 11(`220ms`, `65.03MiB`)이다. focused pprof에서 page 4는 `NewBitmap`, dictionary insertion, file read, Type1 작업, scanner storage, packed RGB image source가 지배하고, page 9는 고정 300-DPI page/group bitmap, decoded DCT/Flate image buffer, scanner storage, exactness-sensitive image scaling이 지배하는 것을 확인했다. 유지한 변경은 image/soft-mask/stream dictionary lookup의 hot PDF name을 slash-prefixed constant로 바꿔, 일반적으로 parser가 저장한 `/Name` key에서 slash-insensitive fallback 작업을 피하게 하는 낮은 위험의 cleanup이다. exactness는 변경되지 않았다: `tmp/codex_perf/slashnames_verify_20260614`는 `doc100_150`과 `doc070_300` 모두 report-identical(`changed_pages=0`, `Exact100 0/31`, `0/11`)이었다. broad 성능 효과는 작고 noisy하다: `doc100_150` `stage=ours`는 `2460ms / 1,175,598,888` alloc / `80,800KB` MaxRSS에서 `2446ms / 1,175,176,672` / `80,800KB`로, `doc070_300`은 `1672ms / 644,386,848` / `142,080KB`에서 `1720ms / 644,672,168` / `141,920KB`로 이동했다. parser exact-capacity dictionary construction과 `DecodeParms` nil-slice probe는 focused run에서 회귀하거나 mixed라 rejected 처리했다. 남은 개선은 streaming/downscale image decode, 고정 Splash bitmap lifetime reuse, PNG roundtrip 없는 raw output comparison, scanner storage 축소 같은 더 큰 설계 변경이 필요하다.
- [x] 현재 worst exactness page인 `doc_100.pdf` 150 DPI page 2(`94.51132501%`, bad pixel `115,468`)를 분석했다. artifact는 `tmp/codex_exact/doc100_p2_regions_20260614`에 있다. region 분석에서는 큰 content 누락이 없었고, 가장 큰 연결 영역도 glyph 크기였으며 sampled body-text 영역의 gray mismatch가 `96,890`개로 본문 텍스트 edge가 지배적이었다. 이 페이지는 embedded Type1 Nimbus Roman 계열과 CID TrueType DejaVuSans를 사용한다. visual crop은 glyph shape가 거의 같고 edge alpha delta만 남는 형태였다. image trace에서는 `FlateDecode` `DeviceRGB` 이미지 4개가 `889x756 -> 190.34x161.87`로 downscale되지만, 해당 image box 내부 bad pixel은 page 2 전체의 약 `14.37%`뿐이었다. image-sampling mode, image contract knob, integer text shift, `SPLASH_DISABLE_FT_GLYPH=1` path fallback(`94.51% -> 86.51%`), forced text fast-path, glyph phase force/bias/eps/snap, global glyph alpha bias, signed glyph matrix(`82.42%`), `/workspace/freetype-go` local replace(`94.51132501%`, unchanged)는 모두 동일하거나 악화되어 rejected 처리했다. `(477,527)` text sample은 ours glyph alpha가 `2`이고 Poppler 결과는 alpha `3`을 시사했지만, 전체 histogram은 positive/negative delta가 섞여 있어 단순 global alpha 보정은 유효하지 않다. 이번 pass에서는 production code 변경을 유지하지 않았다. 다음 exactness 작업은 freetype-go/Splash Type1 bitmap parity를 glyph-alpha/phase 단위로 더 좁히거나, text-raster 지배가 아닌 near-exact page를 선택해야 한다.
- [x] near-exact `doc_070.pdf` 300 DPI page 5를
  `tmp/codex_exact/doc070_p5_current_20260615`와 후속 probe로 분석했다. 이
  페이지는 아직 `99.95292929%`, bad pixel `3,961`이며 모두 grayscale glyph-edge
  delta다. Signed alpha/gray delta는 전역적으로 밝거나 어두운 한쪽으로 치우치지
  않았고, page-global gray/alpha remap simulation은 같은 gray 값이 다른 픽셀에서
  이미 대량으로 맞기 때문에 net candidate가 `0`이었다. Traced bad pixel은
  `NimbusRomNo9L-Regu`, `CMMI7`, `CMMI10` 등 여러 font의 직접 black glyph alpha
  write에서 나와 단일 font 또는 image path 문제가 아니었다. Global glyph alpha
  bias, Type1/font source toggle, coarse glyph phase forcing,
  `PDF_DEBUG_SPLASH_GLYPH_PHASE_EPS` sweep은 모두 유지하지 않았다(`1e-6`/`0.001`
  은 동일, 더 큰 eps는 bad pixel `4,041..8,926`으로 악화). Production code 변경은
  유지하지 않았고, 잔차는 freetype-go/Splash glyph bitmap coverage parity 문제로
  남긴다.
- [x] remaining exactness miss였던
  `007-imagemagick-images/imagemagick-images.pdf` page 4를 분석하고
  수정했다. artifact는 `tmp/codex_exact/imagemagick_current_turn_20260615`와
  `tmp/codex_exact/imagemagick_noflip2x_patch_20260615`에 있다. `pdfimages
  -png` 기준 page 4 embedded image는 16x16 one-component JPEG였고,
  Poppler/ImageMagick/djpeg/Pillow의 16x16 decoded sample은 모두 동일했다.
  mismatch 원인은 JPEG decode가 아니라 scaling-only path였다. trace에서
  `mat=[8 0 0 8 0 0]`, `interpolate=true`였지만 기존 구현은 16x16 이미지를
  9x9로 만든 뒤 8x8 canvas로 clip했고, Poppler의 8x8 결과는 기존
  `popplerRange1D` asymmetric 16->8 mapping과 `64/64` pixel 일치했다.
  scaling-only integer-aligned 2x downscale no-flip fast path를 추가해
  Poppler와 같은 source-range averaging을 사용하게 했고, unit coverage를
  추가했다. focused compare에서 `imagemagick-images.pdf`는 `5/6 -> 6/6`
  exact 100으로 개선됐고, ImageMagick fixture group은 `9/9` exact 100을
  유지했다. broad compare는 `doc_027.pdf` 처리 중 오래 걸려 중단했으므로
  전체 exactness 개선 폭은 아직 재산출하지 않았다. selected regression set은
  기존 known non-exact 상태를 유지했고, related Splash/CLI tests 및
  `git diff --check`가 통과했다.
- [x] slash-name 및 exactness pass 이후 현재 느린 페이지를 다시 하나씩 프로파일링했다. `tmp/codex_perf/smallint_focus_20260614`에서 focused `doc_100.pdf` 150 DPI page 4와 `doc_070.pdf` 300 DPI page 9는 PNG byte-identical을 유지했고, 현재 no-pprof 반복값은 page 4 약 `100.0-100.8MB`, page 9 약 `86.5-86.6MB` `MemTotalAlloc` 수준이었다. 유지한 변경은 immutable PDF integer 중 자주 나오는 `-16..256` 범위의 small integer cache다. focused p4에서 sampled `NewInteger` hotspot이 사라졌고, 같은 세션 직전 control 대비 broad 검증 `tmp/codex_perf/smallint_verify_20260614`는 report-identical(`metric_changed=0`)을 유지하면서 `stage=ours` allocation이 `doc_100`에서 `1,189,496,528 -> 1,186,594,152`, `doc_070`에서 `649,552,032 -> 649,360,648`로 이동했다. rejected probe는 다음과 같다. slice-backed small dictionary는 focused correctness 위험은 없었지만 `tmp/codex_perf/smalldict_verify_20260614` broad 검증에서 wall/allocation이 회귀했다. `YupXdown` vertical-flip direct drawing은 PNG-identical이었지만 page 9 allocation을 줄이지 못했다. lazy Standard14 AFM metrics는 focused page 9 allocation을 `86.4MB -> 84.7-85.3MB`로 낮췄지만 broad wall-time/outlier 위험이 있어 제거했다. eager combined AFM parsing은 exactness-neutral이었지만 broad allocation baseline을 이기지 못해 metrics 변경은 유지하지 않았다. 현재 pprof 기준 남은 page 4/page 9 비용은 구조적이다: 고정 Splash page/group bitmap, PDF file read, dictionary/object parsing, decoded DCT/Flate image buffer, scanner intersection storage, exactness-sensitive image scaling. 관련 `entity`, `parser`, `stream`, `standard`, `splash` targeted test는 `CGO_ENABLED=0 PDF_FREETYPE_GO=1`로 통과했다.
- [x] small-integer pass 이후 `doc_070.pdf` 300 DPI page 9를 corrected `YupXdown` vertical-flip direct path로 다시 확인했다. 유지한 구현은 RGB no-alpha/no-post-transform `YupXdown + vertFlip` 이미지에서 기존 X-down averaging과 Y-up flip map을 그대로 재현해 Splash pipe로 직접 쓰며 temporary scaled bitmap을 제거한다. 출력은 바뀌면 안 되는 성능 변경이다. `tmp/codex_perf/doc070_p9_yupxdown_direct_20260614` focused old/new binary 검증에서 page 9는 PNG byte-identical이었다(`cmp run_1..3 OK`). 같은 세션 5회 비교에서 page 9 `MemTotalAlloc`은 약 `86.35-86.83MB -> 78.85-79.24MB`, live `MemAlloc`은 `70.8-71.3MB -> 63.9-65.1MB`, MaxRSS는 `82.6-83.5MB -> 76.2-77.8MB`로 이동했고, scheduler outlier 1회를 제외하면 median wall은 여전히 약 `0.31-0.32s`였다. `tmp/codex_perf/doc070_yupxdown_direct_full_compare_20260614`의 full `doc_070` 300 DPI render는 이전 ours pixel과 `11/11` page 모두 동일했다(`changed 0`) 그리고 full-doc `MemTotalAlloc`은 기존 `649,360,648` baseline에서 `578,687,152`로 감소했다. 새 direct path와 기존 scaled+blit path를 비교하는 Splash unit coverage를 추가했고, `CGO_ENABLED=0 PDF_FREETYPE_GO=1 go test ./internal/infrastructure/splash ./cmd/pdfrender`, `go build ./cmd/pdfrender ./cmd/pdfcompare`, `git diff --check`가 통과했다.
- [x] corrected `YupXdown` direct path 이후 다음 current slow leader인 `doc_100.pdf` 150 DPI page 4를 다시 프로파일링했다. 현재 focused artifact는 `tmp/codex_perf/doc100_p4_profile_after_yupxdown_20260614`에 있다. CPU profiling 포함 실행에서 page 4는 `MemTotalAlloc 100,635,976`, live `MemAlloc 58,640,960`, MaxRSS `75,200KB`, wall `0.51s`였다. profile은 단일 low-risk hotspot이 아니라 분산된 상태다: `NewBitmap`(`13.28MB` alloc-space), PDF file read(`7.05MB`), `bytes.growSlice`(`6.89MB`), dictionary insertion(`6.50MB`), scanner storage(`~4.69MB` intersection/count rows), Flate decode(`8.30MB` cumulative), parser dict/object 작업(`parseDict` `10.50MB`, `ParseObjectWithSpan` `12.50MB` cumulative), Type1/freetype 작업, packed RGB matte image가 함께 남아 있다. 이번 pass에서는 이 페이지에 대한 추가 production 변경은 유지하지 않았다. 다음 안전 후보는 parser/dict allocation reuse, streaming/downscale image buffer, Splash bitmap lifetime reuse를 더 좁은 설계로 잡아야 한다.
- [x] corrected `YupXdown` direct path 이후 slow page를 계속 page-by-page로
  프로파일링했다. 출력 중립 성능 cleanup 두 가지를 유지했다: simple/CID font
  width override map은 알려진 초기 capacity를 사용하고, `ConcurrentRenderer`는
  `RenderOptions.EnableCache`가 true일 때만 rendered-page cache를 lazy 생성한다.
  `tmp/codex_perf/lazy_pagecache_focus_20260614`의 focused old/new 검증에서
  `doc_100.pdf` page 4, `doc_070.pdf` page 9, page 11은 PNG byte-identical을
  유지했다. `tmp/codex_perf/lazy_pagecache_p11_profile_20260614`의 page 11 in-use
  profile에서는 no-cache render에서 사용하지 않는 page-cache cleanup
  `time.NewTicker`/`lruCache.cleanup` allocation이 사라졌다. full 검증
  `tmp/codex_perf/lazy_pagecache_full_verify_20260614`는
  `tmp/codex_perf/smallint_verify_20260614` 대비 `doc100_150`과 `doc070_300` 모두
  report-identical이었다(`report_diffs=0`, `ours_png changed=0`, `Exact100 0/31`
  및 `0/11`). integer-valued `Real` object cache probe는 PNG-identical이었지만
  focused allocation이 flat/noisy이고 일부 run에서 악화되어 rejected 처리했다.
- [x] lazy page-cache pass 이후 현재 slow leader를 다시 확인했다. focused run
  산출물은 `tmp/codex_perf/slowpage_recheck3_20260614`이며 순위는 `doc_100.pdf`
  150 DPI page 4(`0.39..0.41s`, `100.1..101.0MB` `MemTotalAlloc`),
  `doc_070.pdf` 300 DPI page 9(`0.30..0.31s`, `78.9..79.0MB`), page 11
  (`0.27..0.28s`, `68.2..68.3MB`) 순서로 유지됐다. p4 alloc-space는 고정
  `NewBitmap`, `Dict.Set`, Form/Flate `bytes.growSlice`, whole-file read,
  `NewReal`, scanner storage, font parsing으로 분산되어 있고, p9는 고정 300-DPI
  bitmap과 decoded JPEG/Flate buffer가 지배적이다. Medium Form decode hint
  `rawLen*10` 및 `Real(-1/0/1)` singleton probe는 PNG-identical이었지만 focused
  allocation/RSS 개선이 없어 둘 다 되돌렸다.
- [x] 동일한 slow leader를 `tmp/codex_perf/current_slowpage_20260614`에서
  페이지별로 다시 프로파일링했다. 현재 page-profile 순위는 여전히 `doc_100.pdf`
  150 DPI page 4(`370ms`, `100,035,832` alloc), `doc_070.pdf` 300 DPI page 9
  (`329ms`, `79,197,408` alloc), page 11(`296ms`, `68,489,088` alloc)이다.
  유지한 변경은 출력 중립 renderer cleanup이다: `ConcurrentRenderer`가
  `RenderOptions.EnableCache`에서 실제로 필요할 때만 shared Form XObject
  operator cache를 만들고, 사용되지 않는 `bytePool` field/allocation을 제거했다.
  focused p4는 PNG byte-identical을 유지했고 in-use profile에서 사용하지 않던
  `time.NewTicker`/LRU cleanup allocation이 사라졌다. 해당 profiled run의 live
  `MemAlloc`은 약 `60.1MB`에서 `35.0MB`로 이동했으며 MaxRSS는 noise 범위였다.
  최종 broad 검증 `tmp/codex_perf/current_slowpage_20260614/renderer_lazy_verify`는
  `doc100_150`과 `doc070_300` 모두 report-identical이었다(`report_diffs=0`).
  doc100 page-profile은 flat/noisy(`1,185,581,352 -> 1,185,978,984` alloc)였고,
  doc070은 소폭 개선됐다(`641,109,432 -> 640,186,344` alloc, max-RSS
  `142,880KB -> 142,560KB`). page 9는 `329ms / 79,197,408` alloc에서
  `304ms / 78,891,264` alloc로 이동했다. p9/p11의 남은 비용은 고정 300-DPI
  Splash bitmap과 retained decoded ImageMagick/JPEG 및 Flate image buffer가
  지배하므로, 다음 큰 개선은 streaming/downscale decode, bitmap lifetime reuse,
  raw compare pipeline 작업이 필요하다.
- [x] lazy renderer-cache pass 이후 현재 느린 리더들을 계속 하나씩
  프로파일링했다. `doc_100.pdf` 150 DPI page 4가 여전히 최상위 타깃으로
  `MemTotalAlloc` 약 `100.4..101.1MB`였고, `doc_070.pdf` 300 DPI page 9는
  `78.6..79.0MB`, page 11은 `68.3MB` 수준이었다. 유지한 parser cleanup은
  자주 나오는 unescaped PDF name과 dictionary key를 allocation-free switch
  경로로 intern해 반복 `scanName` 및 `"/"+name` 할당을 줄이며, page 단위
  프로세스 실행에서 누적되는 전역 map 초기화 비용은 만들지 않는다. focused
  old/new 검증 `tmp/codex_perf/slowpage_next_20260614/namekey_switch_focus`에서
  p4/p9/p11은 PNG byte-identical을 유지했다. p4 반복 `MemTotalAlloc`은 대략
  `101.0MB`에서 `98.4..99.0MB`로 이동했고, 최종 p4 profile에서는
  `Lexer.scanName`이 allocation top에서 사라졌다. broad 검증
  `tmp/codex_perf/slowpage_next_20260614/namekey_switch_verify_chunk1`은
  `doc100_150`과 `doc070_300` 모두 report-identical이었다(`report_diffs=0`).
  `doc100_150` total `stage=ours`는 `2,798ms / 1,185,978,984` alloc /
  `80,800KB` MaxRSS에서 `2,756ms / 1,183,295,984` / `77,440KB`로 이동했고,
  `doc070_300`은 `2,012ms / 640,186,344` alloc / `142,560KB`에서
  `2,023ms / 640,782,928` / `142,560KB`로 flat/noise 범위였다. 관련 parser,
  `pdfrender`, `pdfcompare` 테스트, no-CGo build, `git diff --check`가
  통과했다.
- [x] hot-name parser pass 이후 동일한 current slow leader를 계속 하나씩
  프로파일링했다. `doc_100.pdf` 150 DPI page 4 profile에는 Type1 best-effort
  CharString 후보 count 경로가 남아 있었는데, 실제 glyph decode 전에 count만
  확인하려고도 전체 `Command` slice를 decode하고 있었다. 유지한 변경은 후보
  count 경로를 allocation-free byte scanner로 바꿔 numeric operand payload를
  건너뛰고 emitted Type1 operator만 세게 한 것이다. 실제 glyph 렌더링은 기존
  decoder를 그대로 쓴다. focused p4/p9/p11 렌더는 모두 PNG byte-identical을
  유지했다. p4 반복 `MemTotalAlloc`은 약 `98.4..98.9MB`에서
  `95.5..96.6MB`로, `doc_070` page 9는 약 `78.8..79.0MB`에서
  `78.1..78.4MB`로, page 11은 약 `68.2..68.4MB`에서 `67.4..67.6MB`로
  내려갔다. broad 검증 `tmp/codex_perf/type1_count_probe_20260614`는
  `doc100_150`과 `doc070_300` 모두 report-identical이었다(`report_changed=0`).
  `doc100_150` total `stage=ours` allocation은
  `1,183,295,984 -> 1,148,790,896`, MaxRSS는 `77,440KB -> 77,280KB`로
  이동했고, `doc070_300` allocation은 `640,782,928 -> 630,364,304`,
  MaxRSS는 `142,560KB -> 142,080KB`로 이동했다. 단일 실행 wall 합계는
  noise/소폭 악화(`2,756ms -> 2,797ms`, `2,023ms -> 2,058ms`)였으므로,
  accepted signal은 pixel 동일성과 allocation/RSS 감소다. 이번 pass에서
  inline small PDF dictionary probe는 focused p4 일부를 줄였지만 broad
  wall/RSS tradeoff가 나빠 rejected 처리했고, single-filter `DecodeParms`
  slice elision도 allocation 이득이 작고 broad wall/RSS가 나빠 rejected
  처리했다. 관련 Type1, `pdfrender`, `pdfcompare` 테스트, no-CGo build,
  `git diff --check`가 통과했다.
- [x] Type1 count scanner pass 이후 page-by-page profiling을 계속했다.
  `doc_100.pdf` 150 DPI page 4를 다시 프로파일링했고, 남은 작업은 고정
  Splash bitmap, whole-file read, stream/image buffer, dictionary 작업,
  scanner storage, 실제 font 작업으로 분산되어 있음을 확인했다. 유지한 변경은
  출력 중립 normalization cleanup이다: common image filter name과
  image-sampling mode가 호출마다 lowercase string을 할당하지 않게 하고,
  Splash image hot path의 debug env toggle은 init-time cached value를 사용한다.
  focused old/new 렌더에서 `doc_100` page 4 및 `doc_070` pages 9/11은 PNG
  byte-identical을 유지했다. focused page 4 반복 `MemTotalAlloc` median은 대략
  `97.1MB -> 95.7MB`로 이동했고, `doc_070` pages 9/11은 flat/noise 범위였다.
  broad 검증 `tmp/codex_perf/filterfast_verify_20260614`는 `doc100_150`과
  `doc070_300` 모두 report-identical이었다(`report_diffs=0`). broad
  `doc100_150` allocation은 `1,148,790,896 -> 1,148,566,080`, MaxRSS는
  `77,280KB -> 75,200KB`로 이동했고, `doc070_300` allocation은 flat
  (`630,364,304 -> 630,361,952`), RSS는 noise 범위였다. fresh `doc_070`
  page 9 profile에서 다음 큰 비용은 여전히 구조적임을 확인했다: 고정 300-DPI
  Splash/page-group bitmap, ImageMagick/JPEG stdout 및 decoded raster buffer,
  Flate buffer, scanner storage, PNG output이다.
- [x] 현재 slow page를 다시 하나씩 프로파일링했다. 대상은 `doc_100.pdf`
  page 4와 `doc_070.pdf` pages 9/11이다. `pdfcompare`의 CLI 기본
  `-ours-png-compression`을 `auto`로 바꿨다. 이미지를 보존할 때는 기존처럼
  `fast`를 쓰고, `-keep-images=false`에서는 임시 PNG를 비교 후 삭제하므로
  `none`을 사용해 PNG flate 출력 시간을 줄인다. broad old-fast vs new-auto
  검증 `tmp/codex_perf/pdfcompare_auto_none_20260614`에서 `doc100_150`과
  `doc070_300` report metric은 동일했다(`metric_diffs=0`). `doc100_150`
  `stage=ours`는 `2705ms / 1,146,468,800` alloc에서
  `2356ms / 1,133,153,040`으로, `doc070_300`은
  `2311ms / 630,504,840` alloc에서 `1568ms / 625,455,632`로 이동했다.
  new-auto 기준 현재 slow ranking은 여전히 `doc_100.pdf` page 4(`375ms`,
  `94,778,160` alloc), `doc_070.pdf` page 9(`270ms`, `77,684,360` alloc),
  page 11(`220ms`, `66,984,424` alloc) 순이다. 이미 init-time cache 변수가
  있던 Splash debug env 조회도 hot path에서 제거했다. 단,
  `PDF_DEBUG_SPLASH_TEXT_PATH_DEFER_MATRIX`는 테스트와 진단에서 package init
  이후 `t.Setenv`로 제어하므로 runtime read로 유지했다. 관련 `pdfcompare`,
  `pdfrender`, Splash 테스트, no-CGo build, `git diff --check`가 통과했다.
- [x] `pdfcompare` auto-compression pass 이후 남은 slow page를 계속 하나씩
  프로파일링했다. 유지한 변경은 no-alpha/post-transform인 top-down RGB8
  vflip upscale에서 중간 scaled bitmap을 만든 뒤 blit하지 않고 destination
  bitmap에 직접 쓰는 Splash image direct path다. 안전하게 적용 가능한 nearest와
  bilinear 케이스만 포함했다. focused PNG 출력은 `doc_070.pdf` page 9와
  `doc_100.pdf` page 4 모두 byte-identical을 유지했다. 반복 focused run에서
  `doc_070` page 9 평균 `MemTotalAlloc`은 `78,308,624 -> 78,094,464`,
  live heap은 `64,143,428 -> 63,032,776`으로 이동했고, `doc_100` page 4 평균
  `MemTotalAlloc`은 `95,431,899 -> 95,109,027`로 이동했다. 최종 broad 검증
  `tmp/codex_perf/slowpage_iter_20260614`에서 `doc100_150`과 `doc070_300`은
  report-identical이었다(`changed=0`). `doc100_150` total `stage=ours`는
  `2408ms / 1,133,656,848` alloc에서 `2376ms / 1,132,214,336`으로,
  `doc070_300`은 `1494ms / 625,403,120` alloc에서
  `1527ms / 625,250,720`으로 이동했으며, wall/RSS는 단일 실행 noise로
  취급했다. 이번 pass에서 parser dictionary capacity 변경은 broad doc070
  allocation/RSS가 나빠 rejected 처리했고, `GO_PDF_ENABLE_DJPEG_GO=1` 기본화는
  `doc_070` page 11 출력이 달라지고 alloc/RSS가 증가해 rejected 처리했다.
  page 11의 남은 비용은 exact parity 기준 구조적이다: 고정 300-DPI Splash
  bitmap과 Splash scaling 전에 필요한 ImageMagick/JPEG stdout 및 decoded raster
  buffer가 크다. 관련 Splash/parser/CLI 테스트, no-CGo build, `git diff --check`가
  통과했다.
- [x] vflip direct path 이후 현재 slow page pass를 계속 진행했다. text code-unit
  split에서 불필요한 `[]byte(text)` 복사를 제거했고, fast text path가
  char-by-char 렌더링으로 fallback할 때 이미 만든 code-unit slice를 재사용하게
  했다. focused old/new 렌더에서 `doc_100.pdf` page 4와 `doc_070.pdf` pages
  9/11은 PNG byte-identical을 유지했다. 5회 focused 평균 `MemTotalAlloc`은
  `doc_100` page 4 `94,799,571 -> 94,297,403`, `doc_070` page 9
  `77,811,670 -> 77,478,899`, page 11 `67,103,526 -> 66,819,421`로 이동했다.
  broad 검증 `tmp/codex_perf/textunit_verify_20260614`는 `doc100_150`과
  `doc070_300` 모두 report-identical이었다(`metric_diffs=0`).
  total `stage=ours` allocation은 `doc100_150`
  `1,132,214,336 -> 1,128,908,040`, `doc070_300`
  `625,250,720 -> 622,903,608`으로 감소했다. wall-time 변화는 단일 실행
  noise로 취급했다. 관련 renderer/pdfrender 테스트가 통과했다.
- [x] 남은 current slow leader 대상으로 slow-page profiling pass를 계속했다.
  유지한 변경은 `pdfrender` direct Splash canvas output을 기본 `best` PNG
  compression 경로까지 확장하고, 파일 출력 시 filtered RGB8 scanline을 IDAT에
  직접 streaming하는 것이다. 이로써 중간 RGBA image와 전체 filtered payload
  materialization을 피하면서 libpng-style canonical filter 출력은 byte-identical로
  유지한다. `doc_100.pdf` 150 DPI page 4와 `doc_070.pdf` 300 DPI pages 9/11
  focused old/new 검증은 byte-identical이었다. 전체 `doc_070.pdf` old/new CLI
  render 비교도 `byte_changed=0`, `pixel_changed=0`이었다. focused no-profile
  측정에서 page 4 `MemTotalAlloc`은 `112.0MB -> 95.5MB`, page 9는
  `143.3MB -> 78.0MB`, page 11은 `139.0MB -> 66.8MB`로 이동했고, pages 9/11은
  live heap도 크게 줄었다. fresh page 9 CPU profile의 지배 비용은 이제
  `compress/flate` best compression이므로, 추가 wall-time 감소는 투명한
  exact-output 최적화가 아니라 `--png-compression=fast`/`none` 같은 명시적 출력
  정책 변경으로 다루는 것이 맞다. 관련 `pdfrender`, `pdfcompare`, Splash 테스트,
  no-CGo build, `git diff --check`가 통과했다.
- [x] direct PNG streaming 이후 slow-page profiling을 이어서 진행했고, 남은
  저위험 allocation source 두 가지를 줄였다. `pdfrender`는 Unix 계열에서 PDF
  입력을 read-only `mmap`으로 매핑하고, 지원되지 않거나 매핑이 실패하면
  `os.ReadFile`로 fallback한다. 따라서 per-page render가 입력 PDF 전체를 Go
  heap에 복사하지 않는다. bytes-mode lexer는 common PDF content operator와
  parser keyword를 intern해 반복적인 작은 string allocation을 줄이며 token
  값은 그대로 유지한다. focused old/new 렌더에서 `doc_100.pdf` page 4와
  `doc_070.pdf` pages 9/11은 PNG byte-identical이었다. Broad 검증
  `tmp/codex_perf/slowpage_continue_20260614/mmap_verify`에서 `doc100_150`과
  `doc070_300`은 report-identical이었다(`metric_diffs=0`). `doc100_150`
  total `stage=ours`는 `2393ms / 1,128,907,736` alloc / `74,720KB` RSS에서
  `2348ms / 937,802,632` alloc / `70,560KB` RSS로, `doc070_300`은
  `1835ms / 623,617,064` alloc / `142,240KB` RSS에서
  `1535ms / 586,222,176` alloc / `142,080KB` RSS로 이동했다. fresh page 4 heap
  profile은 `MemTotalAlloc` 약 `88.2MB`, live heap 약 `42.2MB`를 보고하며,
  입력 PDF 전체에 해당하던 `os.readFileContents` allocation은 hot list에서
  사라졌다. 남은 leader는 여전히 `doc_100.pdf` page 4, `doc_070.pdf` pages
  9/11이고, 다음 비용은 고정 Splash bitmap, stream buffer, dictionary/parser
  allocation, scanner storage, numeric operand 쪽이다.
- [x] `mmap` pass 이후 현재 slow leader를 계속 프로파일링했다.
  `tmp/codex_perf/slowpage_profile_now_20260614` 기준 순위는
  `doc_100.pdf` page 4(`343ms`, `88,321,816` alloc), `doc_070.pdf`
  page 9(`269ms`, `74,028,080` alloc), page 11(`211ms`,
  `63,274,720` alloc)이다. 유지한 변경은 Splash scanner의 row별
  intersection count scratch를 `[]int`에서 `[]int32`로 줄이는 focused memory
  cleanup이다. 실제 저장되는 intersection 값도 이미 int32라 출력에는 영향을 주면
  안 된다. page 4/page 9/page 11 focused old/new 렌더는 PNG byte-identical이었다.
  5회 focused 평균 `MemTotalAlloc`은 page 4
  `88,523,451 -> 88,128,643`, page 9 `74,140,606 -> 73,562,442`로
  내려갔고, page 11은 flat/noise 범위였다(`63,291,085 -> 63,386,150`).
  memory-only p9 heap profile에서 `Scanner.ensureCountScratch`는
  `1.04MB -> 0.51MB`, `MemTotalAlloc`은 `74,113,848 -> 73,738,136`으로
  이동했다. broad 검증 `countscratch_verify`는 `doc100_150`과 `doc070_300`
  모두 report-identical이었고, 단일 실행 aggregate allocation은 `doc100_150`은
  소폭 개선, `doc070_300`은 noisy/mixed였다. 따라서 retained signal은
  exactness-neutral한 p9 scanner memory 감소다. 남은 p9 큰 비용은 고정 Splash
  page/group bitmap과 retained decoded ImageMagick/JPEG 및 Flate image buffer다.
- [x] count-scratch pass 이후 현재 slow leader를 다시 하나씩 프로파일링했다.
  fresh artifact는 `tmp/codex_perf/current_profile_p4_20260614`,
  `tmp/codex_perf/current_profile_p9_20260614`,
  `tmp/codex_perf/current_profile_p11_20260614`에 있다.
  `--png-compression none` 기준 `doc_100.pdf` page 4는 `0.51s`,
  `88,786,760` `MemTotalAlloc`, `67,200KB` MaxRSS였고, alloc-space는 고정
  Splash bitmap(`13.63MB`), dictionary/parser map 작업(`Dict.Set`
  `12.52MB`), stream buffer, scanner storage, 실제 font 작업으로 분산되어
  있었다. `doc_070.pdf` page 9는 `0.41s`, `74,649,744` `MemTotalAlloc`,
  `75,840KB` MaxRSS였고, page 11은 `0.31s`, `64,609,768` `MemTotalAlloc`,
  `72,960KB` MaxRSS였다. 두 doc070 page는 이제 고정 300-DPI Splash bitmap,
  필요한 decoded ImageMagick/JPEG stdout/raster buffer, Flate buffer,
  scanner row가 지배한다. 이번 pass에서는 추가 production 변경을 유지하지
  않았다. 남은 개선은 streaming/downscale image decode, Splash bitmap lifetime
  reuse, PNG roundtrip 없는 raw comparison pipeline 같은 더 큰 설계 작업이
  필요하다.
- [x] `tmp/codex_perf/current_slowpage_fresh_20260614` 기준으로 slow page를
  다시 하나씩 이어서 프로파일링했다. fresh 순위는 여전히 `doc_100.pdf`
  page 4(`356ms`, `83.68MiB` alloc), `doc_070.pdf` page 9(`305ms`,
  `70.49MiB`), page 11(`267ms`, `60.48MiB`) 순이었다. 유지한 저위험 변경은
  Standard14 AFM glyph-name map 및 reverse glyph-name map을 초기 capacity로
  생성해 모든 `pdfrender` 프로세스의 init-time map growth를 줄이는 것이다.
  glyph 데이터와 출력은 바뀌지 않는다. page 4/page 9/page 11 focused old/new
  렌더는 PNG byte-identical이었다. 5회 focused 평균에서 page 4
  `MemTotalAlloc`은 `84.24MiB -> 83.69MiB`, live heap은
  `40.69MiB -> 39.76MiB`로 이동했고, page 9 live heap/RSS는
  `58.86MiB -> 58.31MiB`, `74.4MiB -> 73.8MiB`로 이동했으며, page 11은
  flat/소폭 개선(`60.43MiB -> 60.37MiB`, wall `274ms -> 264ms`)이었다. broad
  검증 `tmp/codex_perf/standard_mapcap_verify_20260614`에서 `doc100_150`과
  `doc070_300` 모두 report-identical이었다(`metric_diffs=0`). aggregate
  allocation은 `doc100_150` `938,376,424 -> 934,686,296`, `doc070_300`
  `585,961,016 -> 584,314,496`로 이동했고, 단일 실행 wall은 noise 범위였다.
  다음 큰 타깃은 고정 Splash bitmap, stream/image buffer, dictionary/parser
  allocation, scanner storage, exactness-sensitive streaming/downscale decode
  설계다.
- [x] 포트폴리오용 병목 백로그를 역할별 수행 계획과 월보 템플릿으로
  `docs/perf-targets.md` 및 `docs/perf-targets.ko.md`에 문서화했다.
- [x] 현재 Standard14 map-cap baseline에서 slow page profiling을 이어서
  진행했다. fresh artifact는
  `tmp/codex_perf/slowpage_profile_continue_20260614` 및
  `tmp/codex_perf/ydown_upscale_direct_20260614`에 있다.
  `--png-compression none` focused 기준 현재 run은 대략 `doc_100.pdf`
  page 4가 `87.0..88.1MB` `MemTotalAlloc`, `doc_070.pdf` page 9가
  `73.4..73.5MB`, page 11이 `63.2..63.3MB`였다. 남은 p9/p11 alloc-space는
  여전히 고정 300-DPI Splash page/group bitmap과 retained
  ImageMagick/JPEG stdout RGB buffer, Flate/image buffer, scanner storage가
  지배하고, p4는 고정 bitmap, parser/dictionary 작업, stream buffer,
  scanner storage, 실제 Type1/CFF font 작업으로 분산되어 있다.
  scaling-only `Ydown`/`YupXup` direct-image probe는 p4/p9/p11 PNG
  byte-identical을 유지했지만 반복 측정이 flat 또는 소폭 악화였다(`p11`
  평균 `MemTotalAlloc` `63.29MB -> 63.40MB`, p4 `87.40MB -> 87.94MB`,
  p9 allocation은 flat이고 RSS/live heap은 noise). 따라서 해당 probe는 되돌려
  rejected 처리했고 이번 pass에서는 추가 production 변경을 유지하지 않았다.
- [x] `tmp/codex_perf/profile_pages_20260614` 기준으로 slow-page profiling을
  이어서 진행했다. fresh recheck에서도 `doc_100.pdf` page 4가 1순위이고,
  `doc_070.pdf` pages 9/11이 뒤따랐다. `MCID`, `P`, common dictionary key 같은
  structural hot-name interning probe는 focused 출력 byte-identical을 유지했지만
  반복 측정이 flat/noise 또는 소폭 악화라 되돌려 rejected 처리했다. 유지한 변경은
  `github.com/dh-kam/freetype-go v0.1.2` bump와 ImageMagick PGM parser cleanup이다.
  `freetype-go v0.1.2`는 commit `21bc891`에서 release/tag/push했고 Type1 private
  token 분류에서 `strconv.ParseFloat`를 피한다. `go-pdf`는 해당 tag를 사용하도록
  올렸다. PGM parser는 grayscale DCT 이미지에서 ImageMagick stdout raster 전체를
  다시 복사하지 않고 `image.Gray`가 같은 raster slice를 보유하게 했다. 이후
  ImageMagick `rgb:-`/`gray:-` raw-stdout probe도 focused PNG byte-identical을
  유지했지만 반복 측정이 mixed였다(`p11` 소폭 개선, `p9` flat/noise, `p4` 악화).
  따라서 해당 raw probe는 되돌려 rejected 처리했다. focused
  old/new 렌더에서 `doc_100.pdf` page 4와 `doc_070.pdf` pages 9/11은 PNG
  byte-identical이었다. 5회 focused 평균 `MemTotalAlloc`은 page 4
  `88,185,040 -> 87,651,466`, page 9는 flat(`73,465,846 -> 73,477,563`), page 11은
  소폭 개선(`63,275,574 -> 63,118,882`)이었다. full `ft012_verify` compare report는
  `doc100_150` 및 `doc070_300` 모두 기존 `recheck` report와 byte-identical이었다.
  현재 page-profile leader는 여전히 page 4(`87,105,456` alloc)와 `doc_070`
  pages 9/11(`72,974,072` / `63,122,744` alloc)이다. 관련 no-CGo targeted test,
  CLI build, `git diff --check`가 통과했다. 남은 큰 비용은 고정 Splash bitmap,
  ImageMagick/JPEG stdout buffer, Flate/image buffer, scanner storage,
  parser/dictionary 작업이다.
- [x] 같은 현재 slow leader를
  `tmp/codex_perf/slowpage_iter_next_20260614` 기준으로 하나씩 계속
  프로파일링했고, `tmp/codex_perf/ydown_direct_clip_20260614`에서
  partial-clip `YdownXdown` direct-image path를 유지했다. 이 direct path는
  `ClipAllInside`뿐 아니라 partial clip에서도 임시 scaled bitmap을 만들지 않고,
  기존 Splash pipe 및 clip/AA coverage 규칙을 그대로 사용한다. focused old/new
  렌더에서 `doc_100.pdf` page 4와 `doc_070.pdf` pages 9/11은 PNG
  byte-identical을 유지했다. 5회 focused 평균은 page 4 `MemTotalAlloc`
  `87,588,674 -> 87,573,166`, live heap `41,740,619 -> 41,527,395`
  (MaxRSS는 noise로 `70,560KB -> 71,296KB`), page 9
  `73,482,117 -> 73,268,194`, live heap `61,048,245 -> 60,850,771`,
  page 11 `63,233,984 -> 62,579,464`, live heap
  `56,092,763 -> 55,441,579`로 이동했다. 새 p11 pprof에서는 `NewBitmap`이
  고정 300-DPI page bitmap 중심으로만 남아 이 경로의 임시 image-scale bitmap이
  제거됐음을 확인했다. p9에는 generic scale-then-flip path의 별도 `4.58MB`
  fallback scaled bitmap이 아직 남아 있고, source-alpha/post-transform 계열
  설계를 더 넓게 잡기 전에는 건드리지 않았다. 후속 RGB8/no-alpha
  `postTransform` direct probe는
  `tmp/codex_perf/posttransform_direct_20260614`에서 focused p4/p9/p11 PNG
  byte-identical을 유지했지만, 5회 평균이 flat 또는 소폭 악화였다(`p4`
  `MemTotalAlloc` `87,208,726 -> 87,294,350`, p9 `73,306,854 -> 73,334,926`).
  따라서 되돌려 rejected 처리했다. full 검증
  `tmp/codex_perf/ydown_direct_clip_20260614/verify`는 `ft012_verify` 기준으로
  `doc070_300`(`11` rows, `diffs 0`)과 `doc100_150`(`31` rows, `diffs 0`) 모두
  report-identical이었다. 관련 no-CGo targeted test, CLI build, `git diff --check`가
  통과했다.
- [x] `tmp/codex_perf/slowpage_reprofile_20260614` 기준으로 slow-page
  profiling을 한 페이지씩 계속 진행했다. 현재 page-profile leader는
  `doc_100.pdf` 150 DPI page 4(`366ms`, `83.08MiB` alloc),
  `doc_070.pdf` 300 DPI page 9(`268ms`, `69.98MiB` alloc), page 11(`205ms`,
  `59.49MiB` alloc)였다. focused pprof에서 page 4는 고정 Splash bitmap,
  dictionary/parser 작업, stream buffer, scanner storage, 실제 font 작업으로
  분산되어 있고, page 9는 고정 300-DPI Splash bitmap(`35.63MB` alloc-space),
  image decode buffer(`15.03MB` `bytes.growSlice`), scanner row가 지배했다.
  유지한 저위험 변경은 Standard14 URW AFM metric을 package initialization 때
  모든 AFM 파일을 읽는 방식에서 실제 font metric 요청 시 lazy load하는 방식으로
  바꾼 것이다. focused old/new 렌더는 PNG byte-identical을 유지했고, page 4
  `MemTotalAlloc`은 `87,971,368 -> 87,280,264`, MaxRSS는 `72,160KB ->
  69,920KB`로 이동했다. page 9는 `73,339,632 -> 71,878,496`으로 줄었고
  MaxRSS는 `75,200KB`로 flat이었다. full 검증
  `tmp/codex_perf/lazyafm_verify_20260614`는 `doc100_150`, `doc070_300` 모두
  report-identical이었다. `stage=ours` 총 allocation은 `doc100_150`
  `922,977,648 -> 883,590,736`, `doc070_300`
  `579,861,600 -> 565,814,728`로 이동했고 RSS는 각각
  `71,680KB -> 68,320KB`, `142,240KB -> 141,920KB`였다. full page-profile
  wall time은 일부 page outlier가 커서 acceptance signal은 PNG/report identity와
  allocation/RSS 감소로 판단했다.

- [x] `tmp/codex_perf/slowpage_profile_20260614`에서 `doc_100.pdf` 150 DPI
  page 4와 `doc_070.pdf` 300 DPI pages 9/11 slow-page profiling을 이어서
  수행했다. focused current baseline은 page 4 `~87.10MB` `MemTotalAlloc`,
  page 9 `~72.57MB`, page 11 `~61.51MB`였다. parser dictionary capacity,
  Form/operator cache capacity, raw RGB matte in-place decode, clipped bilinear
  direct draw, ImageMagick CMYK stdout streaming, decoded image XObject cache,
  djpeg-go default, Type1 glyph-name lookup cache, initial page alpha-clear
  skip probe는 PNG 변경, allocation/RSS 악화, 또는 noise 수준 효과라 모두
  rejected 처리했다. 마지막 `aaBuf` fill-helper probe는 같은 worktree baseline
  대비 `doc_070` 300 DPI 전체 page와 `doc_100` 150 DPI 전체 page에서 PNG
  byte-identical을 유지했다(`changed 0`, `11/11` 및 `31/31` pages). 하지만
  memory 개선이 없었다(`doc070` `MemTotalAlloc` `552,232,664 -> 552,411,096`,
  live heap `58,841,208 -> 64,919,680`; `doc100`
  `808,726,824 -> 813,741,080`, live heap `30,889,856 -> 45,864,544`) 그래서
  되돌렸다. targeted no-CGo test와 `go build ./cmd/pdfrender`는 통과했다. 이번
  pass에서는 추가 production 변경을 유지하지 않았고, 남은 큰 비용은 고정 Splash
  page/group bitmap, decoded JPEG/Flate buffer, scanner storage,
  parser/dictionary 작업이다.

- [x] `tmp/codex_perf/slowpage_turn_20260614`,
  `tmp/codex_perf/slowpage_p9_turn_20260614`,
  `tmp/codex_perf/slowpage_p11_turn_20260614` 기준으로 현재 slow-page profiling을
  계속했다. focused baseline은 `doc_100.pdf` page 4가 약 `0.37s` /
  `87.25MB` `MemTotalAlloc`, `doc_070.pdf` page 9가 약 `0.32s` / `72.53MB`,
  page 11이 약 `0.26s` / `61.65MB`였다. page 11 pprof에서는 고정 300-DPI
  Splash bitmap allocation(`33.44MB`), JPEG/ImageMagick stdout buffer growth
  (`13.75MB`), PNG zlib output CPU, 실제 Type1 glyph 작업이 지배적이었고,
  page 11 전용으로 안전하게 유지할 production 변경은 찾지 못했다. rejected
  probe는 두 가지다. Form XObject cache-first 실행은 page 4 allocation을 약
  `104MB`로 악화시켰고, Type1 `CharStringDecoder.pop`/arithmetic allocation
  probe는 출력은 동일했지만 noise 수준 또는 wall-negative였다. 유지한 저위험
  변경은 ImageMagick JPEG 경로에서 `exec.LookPath("convert")`를 process당 한 번만
  캐시하는 cleanup이다. focused 렌더에서 `doc_100.pdf` page 4와 `doc_070.pdf`
  pages 9/11은 PNG byte-identical을 유지했고, 5회 median은 page 4 `0.37s` /
  `87.07MB`, page 9 `0.31s` / `72.47MB`, page 11 `0.27s` / `61.62MB`로
  이동했다. `tmp/codex_perf/convert_path_cache_verify_20260614`의 `doc100_150`
  broad 검증은 `tmp/codex_perf/current_slowpage_fresh_20260614` 대비
  report-identical이었다. `doc070_300`은 해당 오래된 baseline이 현재 worktree
  exactness 상태와 더 이상 맞지 않아 broad identity signal로 사용하지 않았다.
  `CGO_ENABLED=0 PDF_FREETYPE_GO=1 go test ./internal/infrastructure/image`는
  통과했다.
- [x] focused `pdfcompare -pages` 지원을 추가하고 현재 slow page를 페이지 단위로
  다시 프로파일링했다. 새 옵션은 `4,8,22`, `2-4` 같은 page list/range를
  normalize해서 Poppler와 `pdfrender` 양쪽에 선택 범위로 전달하며, parser 및
  split helper 테스트를 추가했다. Fresh `doc_100.pdf` 150 DPI baseline
  `tmp/codex_perf/slowpage_reprofile_20260614/doc100_150`에서 page 4가 1위였다
  (`372ms`, `86,620,896` alloc, `69,600KB` MaxRSS). 다음 후보는 pages 8, 22,
  7, 2였다. 유지한 저위험 memory cleanup은 `drawYdownXdownDirect`의 임시 RGB
  downscale buffer를 pool로 재사용하는 것이다. Focused page 4는 PNG
  byte-identical을 유지했고 `pdfcompare` allocation은
  `86,410,296 -> 85,683,880`, MaxRSS는 `69,440KB -> 69,120KB`로 이동했다
  (wall은 `358ms -> 368ms`로 noise 범위). Full `doc100_150` 검증
  `tmp/codex_perf/slowpage_after_pool_20260614/doc100_150`은
  report-identical이었다(`accuracy_rows_equal true`). 전체 `stage=ours`
  allocation은 `882,407,696 -> 882,237,640`, wall 합계는 `2246ms`로 동일했다.
  pages 8 및 22 pprof는 고정 Splash/output bitmap, 실제 glyph/path 작업,
  Flate/image buffer, scanner storage가 주된 구조적 비용임을 보여 이번 pass에서는
  추가 page-specific production 변경을 유지하지 않았다. `CGO_ENABLED=0
  PDF_FREETYPE_GO=1 go test ./internal/infrastructure/splash ./cmd/pdfcompare`,
  `go build ./cmd/pdfcompare ./cmd/pdfrender`, `git diff --check`가 통과했다.
- [x] `tmp/codex_perf/slowpage_active_20260614`에서 현재 slow page를 다시
  하나씩 프로파일링했다. Fresh ranking은 여전히 `doc_100.pdf` 150 DPI page 4
  (`385ms`, `85,880,144` alloc)가 1위였고, 그다음은 `doc_070.pdf` 300 DPI
  page 9(`279ms`, `72,143,592` alloc)와 page 11(`245ms`, `61,071,712`
  alloc)이었다. p4는 고정 Splash bitmap allocation과 parser/dictionary 작업이
  지배하고, p9/p11은 고정 300-DPI bitmap 및 ImageMagick/JPEG decoded raster
  buffer가 지배하므로 broad bitmap 또는 JPEG streaming 변경은 유지하지 않았다.
  유지한 저위험 cleanup은 default non-Type3 text renderer에서 evaluator 소유
  text code-unit scratch slice를 재사용하는 것이다. custom renderer와 Type3
  nested-evaluation 가능 경로는 기존 allocation 경로를 유지한다. focused
  old/new 렌더에서 p4/p8/p22/p9/p11은 PNG byte-identical이었고, broad 검증은
  `doc100_150`, `doc070_300` 모두 report-identical이었다. 총 `stage=ours`
  allocation은 `doc100_150`에서 `883,057,152 -> 878,307,608`,
  `doc070_300`에서 `564,659,792 -> 562,704,384`로 이동했다.
  `CGO_ENABLED=0 PDF_FREETYPE_GO=1 go test ./internal/domain/renderer
  ./cmd/pdfrender ./cmd/pdfcompare`, `go build ./cmd/pdfcompare
  ./cmd/pdfrender`, `git diff --check`가 통과했다.
- [x] 같은 slow-page profiling pass를 `doc_100.pdf` page 4와 `doc_070.pdf`
  pages 9/11에서 계속했다. raw axis-aligned clip-rectangle pre-fast-path probe는
  `tmp/codex_perf/current_profile_20260614/rectfast_focus2`와
  `tmp/codex_perf/current_profile_20260614/rectfast_verify`에서 PNG/report
  identical을 유지했지만, focused allocation은 flat/noisy였고 full `stage=ours`
  allocation은 소폭 증가했다(`doc100_150` `878,307,608 -> 880,327,216`,
  `doc070_300` `562,704,384 -> 562,994,800`). 따라서 해당 probe는 되돌렸다.
  persistent clip-scanner `countScratch` release probe도
  `tmp/codex_perf/current_profile_20260614/scanscratch_focus`에서 PNG-identical을
  유지했지만, page render 중 GC를 강제하지 않는 조건에서는 즉시 `MemAlloc`/RSS
  감소가 나타나지 않아 되돌렸다. 최신 page-9 pprof
  `tmp/codex_perf/current_profile_20260614/profile_p9_current`는 남은 비용이
  구조적임을 재확인했다: 고정 300-DPI Splash bitmap(`36.69MB` alloc-space),
  ImageMagick/libjpeg raw raster stdout(`10.62MB` retained output buffer),
  Flate decoded image buffer(`6.31MB`), scanner intersection storage가 주된
  비용이다. 이번 pass에서는 production code 변경을 유지하지 않았다.
- [x] `tmp/codex_perf/slowpage_profile_current_run_20260614` 기준으로
  page-by-page profiling을 이어서 수행했다. Fresh page-profile leader는
  `doc_100.pdf` 150 DPI page 4(`397ms`, `86,077,872` alloc, `69,920KB`
  MaxRSS), `doc_070.pdf` 300 DPI page 9(`272ms`, `71,984,872` alloc,
  `142,080KB` MaxRSS), page 11(`224ms`, `60,996,736` alloc)이었다. Focused
  pprof에서 p4는 고정 Splash bitmap, dictionary/parser 작업, stream buffer,
  scanner storage, Type1/CFF font 작업으로 분산되어 있고, p9/p11은 고정
  300-DPI Splash bitmap과 retained ImageMagick/JPEG raster buffer,
  Flate/image buffer가 지배함을 재확인했다. 유지한 저위험 cleanup은 stream
  DecodeParms hot key를 `/Predictor`, `/Columns`, `/Colors`,
  `/BitsPerComponent`, `/K`, `/Rows`, `/BlackIs1`, `/EncodedByteAlign`,
  `/EarlyChange` slash-prefixed 상수로 조회하게 한 것이다. parser가 저장한
  `/Name` key에서 slash-insensitive fallback 작업을 피하기 위한 변경이며 출력은
  바뀌면 안 된다. focused p4/p9/p11 렌더는 PNG byte-identical을 유지했고,
  broad 검증 `tmp/codex_perf/slowpage_stream_slash_20260614/verify`에서
  `doc100_150`, `doc070_300` 모두 report-identical이었다. 총 allocation은
  `doc100_150` `879,560,256 -> 879,473,952`, `doc070_300`
  `562,938,320 -> 562,406,520`으로 이동했고 wall/RSS는 noise로 판단했다.
  parser dictionary capacity-8 probe는 출력은 byte-identical이었지만 focused
  p4/p9/p11 allocation이 모두 악화되어 되돌렸다. 남은 큰 개선은
  streaming/downscale image decode, Splash bitmap lifetime reuse, PNG roundtrip
  없는 raw comparison pipeline 같은 설계 작업이 필요하다.

- [x] stream slash-key cleanup 이후 현재 slow page를 계속 하나씩
  프로파일링했다. path clip 생성은 pooled `XPath`와 `Scanner`를 재사용하고,
  owned clip release는 cloned clip의 Poppler식 shared-scanner semantics를
  보존하기 위해 unshared tail scanner만 pool로 반환한다. Focused p4/p9/p11
  렌더는 PNG byte-identical을 유지했다. focused median 기준 `doc_100.pdf` p4는
  TotalAlloc `86,141,536 -> 83,216,712`, live heap `42,833,928 ->
  40,954,576`으로 이동했고, `doc_070.pdf` p9는 TotalAlloc `71,857,152 ->
  67,225,616`, live heap `60,555,304 -> 56,104,056`으로 이동했다. p11은
  TotalAlloc `60,981,648 -> 60,976,224`로 사실상 flat이었다. broad 검증
  `tmp/codex_perf/pooled_clip_scanner_20260614/verify`는
  `tmp/codex_perf/slowpage_stream_slash_20260614/verify` 대비 `doc100_150`
  `31/31`행, `doc070_300` `11/11`행 모두 누락/추가/metric 변경 없이
  report-identical이었다.
- [x] pooled clip-scanner pass 이후 `doc_100.pdf` 150 DPI page 4를
  `tmp/codex_perf/pooled_clip_scanner_20260614/profile_p4_current`에서 다시
  프로파일링했다. pprof run은 `0.50s`, `84,300,480` TotalAlloc,
  `49,112,560` live heap, `66,560KB` MaxRSS로 완료됐다. allocation은 고정
  Splash bitmap(`13.63MB` alloc-space), dictionary/parser map construction
  (`Dict.Set` `9.52MB`, `ParseObjectWithSpan` `18.02MB` cumulative), stream
  buffer growth(`9.50MB`), Type1/TrueType/CFF font parsing, packed RGB matte
  image, scanner intersection rows/storage로 분산되어 있었다. 남은 후보는 단일
  저위험 hotspot이 아니라 구조적 비용이어서 이 profile에서는 추가 page-4
  production 변경을 유지하지 않았다.
- [x] `tmp/codex_perf/slowpage_profile_now2_20260614`에서 현재 slow page를
  다시 하나씩 프로파일링했다. Fresh page-profile leader는 여전히
  `doc_100.pdf` 150 DPI page 4(`367ms`, `83,244,312` alloc),
  `doc_070.pdf` 300 DPI page 9(`265ms`, `66,864,656` alloc), page 11
  (`233ms`, `61,122,112` alloc)이었다. Focused pprof에서 p4는 고정 Splash
  bitmap, ImageMagick/JPEG stdout buffer, parser/dict, stream buffer,
  soft-mask image, font parsing으로 분산되어 있고, p9/p11은 고정 300-DPI
  page bitmap과 ImageMagick decoded RGB buffer가 지배함을 재확인했다. Rejected
  probe: ImageMagick JPEG 비활성화는 p9/p11 exactness와 allocation을 모두
  악화했고, opaque page alpha는 p4/p9/p11 exactness를 악화했으며, shared
  Standard14 fallback width table은 output-identical이었지만 allocation 개선이
  없거나 noise-negative였다. 유지한 변경은 `--png-compression none`에서
  seekable streaming IDAT에 `compress/flate` writer를 만들지 않고 zlib stored
  block을 직접 쓰는 최적화다. Focused p4/p9/p11 report는 metric-identical을
  유지했고, p11 pprof `alloc_space`는 `60.15MB -> 57.30MB`로 이동하며
  `compress/flate.NewWriter`가 hot list에서 사라졌다. Broad 검증
  `stored_verify_doc100_150`, `stored_verify_doc070_300`은 report diff `0`을
  유지했다. 총 `stage=ours` allocation은 `doc100_150`
  `876,451,328 -> 856,905,464`, `doc070_300`
  `555,083,400 -> 548,206,360`으로 감소했고, 총 wall은 각각
  `3418ms -> 2217ms`, `1505ms -> 1466ms`로 이동했다. wall time은 여전히
  noise 가능성이 있어 allocation/report identity를 주 acceptance signal로 둔다.
- [x] `tmp/codex_perf/slowpage_profile_active_20260614`에서 slow page profiling을
  이어서 `doc_100.pdf` 150 DPI page 4와 `doc_070.pdf` 300 DPI pages 9/11을
  다시 확인했다. focused baseline은 p4 `379ms` / `81,854,392` alloc,
  p9 `279ms` / `66,446,704` alloc, p11 `219ms` / `60,135,968` alloc이었다.
  유지한 변경은 glyph hot loop에서 glyph trace와 alpha-bias debug env가 꺼진
  기본 실행 시 trace/bias 작업을 건너뛰는 저위험 cleanup이다. p4 `pdfrender`
  5회 median은 wall `0.39s -> 0.38s`, MaxRSS `70080KB -> 69600KB`,
  `MemTotalAlloc` `83,053,152 -> 82,780,808`로 이동했다. image trace call-site
  guard는 p9 median이 noise-negative(`0.26s -> 0.27s`,
  `66,168,240 -> 66,191,808` alloc)라 되돌렸다. focused p4/p9/p11 report는
  metric-identical이었고, broad 검증 `broad_final/doc100_150`,
  `broad_final/doc070_300`은 `slowpage_profile_now2_20260614/stored_verify_*`
  기준 report diff `0`을 유지했다. 남은 p9/p11 memory는 여전히 고정 300-DPI
  Splash bitmap과 JPEG/ImageMagick decoded RGB buffer가 지배적이어서 이번
  pass에서는 추가 저위험 page-specific production 변경을 유지하지 않았다.
- [x] 현재 slow leader를
  `tmp/codex_perf/packed_source_local_20260614`와
  `tmp/codex_perf/vflip_direct_20260614`에서 계속 page-by-page로
  프로파일링했다. 출력 중립 Splash image 변경 두 가지를 유지했다. packed
  RGB8 image source는 hot draw path에서 heap escape되는 row closure를 만들지
  않으며, 강제 DCT/RGB downscale `scale + vertFlip + blit` 케이스는 같은 평균
  row를 계산한 뒤 임시 scaled bitmap 없이 뒤집힌 목적 row로 직접 그린다.
  focused p4/p9/p11 렌더는 pre-change binary와 PNG byte-identical이었고, full
  `doc_070.pdf` 300 DPI 검증도 report-identical이었다(`Exact100 0/11`). p4
  no-pprof `MemTotalAlloc`은 packed-source 변경 후 대략 `84.0MB -> 81.8MB`로
  이동했다. p9는 alloc-space `Splash.NewBitmap`이 `36.69MB -> 32.11MB`로
  줄었고, 5회 focused 평균은 TotalAlloc `67,153,301 -> 63,388,408`, live heap
  `55,919,723 -> 53,903,101`, MaxRSS `70,912KB -> 68,448KB`로 이동했다.
  scale-scratch 재사용 probe는 focused 출력은 동일했지만 RSS/live heap 개선이
  안정적이지 않아 되돌렸다.
- [x] `tmp/codex_perf/slowpage_profile_next2_20260614`에서 slow page
  profiling을 이어갔다. Fresh leader는 `doc_100.pdf` 150 DPI page 4
  (`355ms`, `80,036,000` alloc), `doc_070.pdf` 300 DPI page 9
  (`273ms`, `62,943,168` alloc), page 11(`216ms`, `60,318,216` alloc) 순서였다.
  page 4 pprof는 고정 Splash bitmap(`12.63MB` alloc-space), dictionary map
  insertion(`Dict.Set` `7.00MB`), Flate buffer growth(`6.77MB`), parser object
  materialization, font decode, packed matte RGB image, scanner storage로
  분산되어 있었다. page 9는 여전히 고정 300-DPI Splash bitmap allocation
  (`32.11MB`)과 decoded Flate/image buffer(`bytes.growSlice` `15.67MB`)가
  지배적인 구조적 profile이었다. 유지한 변경은 font/Form/image/ExtGState hot
  dictionary lookup을 slash-prefixed constant로 바꿔 parsed `/Name` key에서
  첫 lookup에 맞게 하는 출력 중립 renderer cleanup이다. focused p4/p9/p11은
  PNG byte-identical이었고 broad `doc100_150`, `doc070_300` report도
  identical이었다. leader allocation은 p4 `80,036,000 -> 79,739,048`, p9
  `62,943,168 -> 62,618,728`, p11 `60,318,216 -> 60,311,464`로 이동했지만,
  문서 전체 allocation은 mixed/noisy였다(`doc100_150`
  `851,700,536 -> 853,395,808`, `doc070_300`
  `542,484,992 -> 541,412,120`). 다음 큰 개선은 Splash bitmap lifetime reuse,
  decoded image/Flate streaming 또는 downscale decode, dictionary 표현 변경처럼
  더 큰 설계 작업이 필요하다.
- [x] `tmp/codex_perf/page_by_page_profile_20260614`에서 현재 slow leader를
  page-by-page로 다시 profiling했고,
  `tmp/codex_perf/type1_callsubr_inline_probe_20260614`의 Type1 charstring
  `callsubr` cleanup을 유지했다. Fresh pprof의 page 순서는 동일하게
  `doc_100.pdf` 150 DPI page 4, `doc_070.pdf` 300 DPI page 9, page 11이었다.
  page 4는 여전히 `Dict.Set`, 고정 Splash bitmap, Flate buffer, font decode,
  scanner storage로 분산되어 있고, page 9/11은 고정 300-DPI Splash bitmap과
  decoded image buffer가 지배적이다. 유지한 Type1 변경은 subroutine 실행 시
  nested `CharStringDecoder`와 `subCommands` slice를 만들지 않고 현재 decoder
  안에서 inline으로 실행하며, `return` filtering과 depth restore 동작은 유지한다.
  focused p4/p9/p11 렌더는 PNG byte-identical이었고, broad 검증도
  `doc100_150`, `doc070_300` 모두 report-identical이었다(`metric_diffs=0`).
  총 `stage=ours`는 `doc100_150`이 `2295ms / 853,395,808` alloc /
  `69,600KB` MaxRSS에서 `2184ms / 848,864,016` / `69,760KB`로 이동했고,
  `doc070_300`은 `1863ms / 541,412,120` / `142,400KB`에서
  `1501ms / 540,047,984` / `141,600KB`로 이동했다. 거부한 probe는 parser
  dictionary initial capacity `4 -> 8` 조정(mixed/noisy)과 p11 allocation/RSS를
  악화한 Type1 operand stack preallocation이다.
- [x] Type1 `callsubr` pass 이후 같은 current slow leader를 계속 하나씩
  profiling했다. `--png-compression none` 기준 fresh focused baseline은
  `doc_100.pdf` 150 DPI page 4가 대략 `0.38..0.41s` /
  `79.9..80.7MB` TotalAlloc, `doc_070.pdf` 300 DPI page 9가 `0.26s` /
  `61.8..62.5MB`, page 11이 `0.19..0.20s` / `60.0..60.2MB` 수준이었다.
  page 4는 여전히 고정 Splash bitmap, `Dict.Set`/dictionary construction,
  Flate buffer, parser object materialization, Type1/TrueType 작업, scanner
  storage로 분산되어 있고, page 9/11은 고정 300-DPI Splash bitmap과 decoded
  JPEG/Flate image buffer가 지배적이다. 유지한 변경은 좁은 font-encoding
  cleanup이다. hot glyph-name alias resolution이 candidate slice를 매번
  만들지 않고 원래 이름과 단일 alias를 직접 확인하며, 기존
  `encodingGlyphNameCandidates` 동작은 test/debug caller용으로 유지했다.
  focused p4/p9/p11 렌더는 PNG byte-identical이었고, 관련 renderer 및
  pdfrender 테스트가 통과했으며,
  `tmp/codex_perf/glyph_alias_lazy_full_verify_20260615`의 full `doc100_150` /
  `doc070_300` report는
  `tmp/codex_perf/type1_callsubr_inline_probe_20260614`와 byte-identical이었다.
  이 변경은 큰 TotalAlloc 개선이라고 주장하지 않고 small object churn을 줄이는
  cleanup으로 본다. focused allocation은 flat/noisy였다. no-chunk full 검증의
  `stage=ours` 집계는 `doc100_150` `1839ms / 777,470,640` alloc /
  `92,332KB` MaxRSS, `doc070_300` `1322ms / 529,173,624` alloc /
  `118,512KB` MaxRSS였다. 이번 pass에서 거부한
  probe는 integer-valued `Real` object cache(PNG-identical이나 flat/noisy),
  parser dictionary capacity `4 -> 2`(mixed/noisy), `djpeg-go`(page 9는
  PNG-identical이지만 allocation/RSS 증가, page 11은 픽셀 변경), opaque
  page-alpha(픽셀 변경)다. 다음 큰 개선은 decoded image/Flate streaming 또는
  downscale decode, Splash bitmap lifetime reuse, dictionary 표현 변경 같은
  더 큰 설계가 필요하다.
- [x] `tmp/codex_perf/slowpage_profile_20260615`에서 현재 slow page를 다시
  하나씩 profiling했다. 대상은 `doc_100.pdf` 150 DPI page 4,
  `doc_070.pdf` 300 DPI page 9, page 11이다. Fresh pprof에서 병목은 여전히
  고정 Splash page bitmap, decoded JPEG/Flate image buffer, font/glyph 작업,
  parser/dictionary churn으로 확인됐다. 유지한 변경은 좁은 Splash page bitmap
  lifetime reuse다. direct Splash `pdfrender` 출력은 PNG streaming 후 page
  canvas를 release하고, `NewBackend`는 같은 크기의 RGB page bitmap을 pool에서
  재사용한다. Focused old/new 검증에서 `doc_070.pdf` pages 9-11과
  `doc_100.pdf` pages 4-6은 PNG byte-identical이었다. Focused allocation은
  `doc_070` pages 9-11 `164,191,320 -> 96,629,288` bytes, MaxRSS
  `111,320KB -> 99,040KB`로 이동했고, `doc_100` pages 4-6은
  `129,086,224 -> 112,100,136` bytes, MaxRSS `91,680KB -> 88,960KB`로 이동했다.
  Full 검증 `tmp/codex_perf/release_pool_full_verify_20260615`에서
  `doc100_150`과 `doc070_300` report는
  `tmp/codex_perf/glyph_alias_lazy_full_verify_20260615`와 byte-identical이었다.
  Full `stage=ours` allocation은 `doc100_150`
  `777,470,640 -> 528,739,904`, `doc070_300`
  `529,173,624 -> 191,182,584`로 줄었다. pool이 마지막 page bitmap 하나를
  보관하므로 full MaxRSS는 noisy/소폭 증가로 관측됐다.
- [x] page-bitmap reuse pass 이후
  `tmp/codex_perf/name_intern_20260615`에서 slow page를 계속 하나씩
  profiling했다. 출력 중립 small-object cleanup만 유지했다. Type1 token parsing은
  candidate slice 및 단일 문자 token allocation을 피하고, PDF name interning에는
  현재 느린 `doc_100.pdf` page 4 qdf/profile 데이터에서 자주 나온 name들을
  추가했다. Focused p4/p9/p11 렌더는 PNG byte-identical이었다. Full A/B 검증은
  같은 `pdfcompare` binary와 old/new `pdfrender` binary, `-page-chunk-size 1`로
  수행했고 report metric은 바뀌지 않았다(`0` metric diffs). `doc100_150`은
  `2222ms / 848,658,680` alloc에서 `2187ms / 846,545,304`로, `doc070_300`은
  `1500ms / 539,754,864` alloc에서 `1470ms / 540,091,440`으로 이동했다. 이
  변경은 큰 memory win이 아니라 작은 object-churn cleanup이며, allocation delta는
  작고 noisy하다. content-stream `fastContentOperand` probe는 focused allocation
  개선이 없고 parser semantic risk가 커서 유지하지 않았다. 남은 리더는 여전히
  구조적이다: 고정 Splash bitmap lifetime/reuse, decoded JPEG/Flate image buffer,
  parser/dictionary object materialization, scanner storage.
- [x] `tmp/codex_perf/slowpage_reprofile_20260615`에서 현재 느린 페이지를 다시
  하나씩 프로파일링하고, 좁은 FreeType env gate cleanup만 유지했다. 새
  page-profile의 안정적인 리더는 focused render 기준 `doc_100.pdf` 150 DPI page
  4(`0.35s`, 약 `79.7MB` TotalAlloc), `doc_070.pdf` 300 DPI page 9(`265ms`,
  `61,487,672` alloc), page 11(`228ms`, `60,093,144` alloc)이었다. 최초
  `doc_100` page 9 wall spike는 focused 반복 측정이 `0.05s` / 약 `25.7MB`로
  유지되어 scheduler/process outlier로 제외했다. Page 4 pprof는 여전히 고정
  Splash bitmap(`13.26MB` alloc-space), Flate decode growth(`8.57MB`),
  dictionary/object parsing(`Dict.Set`, `NewReal`, `NewInteger`), FreeType
  Type1/SFNT glyph 작업, scanner storage로 분산되어 있었다. Bitmap allocation
  trace 결과 큰 allocation은 대부분 고정 page/group/Mono8 mask bitmap이었고,
  많은 tiny `scaleMask` bitmap은 byte보다 object-count noise에 가까웠다. Glyph
  key trace도 cacheability가 낮았다(`1584` glyph render, `1517` unique key).
  유지한 변경은 FreeType adapter 및 TrueType/CFF wrapper에서
  `PDF_FREETYPE_GO` / `PDF_FREETYPE_GO_TYPE1` gate를 process 시작 시 캐시해 hot
  glyph path의 반복 env read를 제거하는 것이다. Focused page 4는 `5/5` run에서
  PNG byte-identical을 유지했고 median TotalAlloc은 `80,168,208 -> 79,761,112`
  로 이동했다. Full `doc100_150` 및 `doc070_300` report는 baseline과
  byte-identical이었다. Full allocation은 mixed/noisy였다(`doc100_150`
  `846,024,936 -> 846,288,760`, `doc070_300`
  `538,644,360 -> 539,814,904`)라서 이 변경은 큰 memory win이 아니라 작은
  branch/env cleanup이다. 다음 material gain은 parser value object/dictionary
  representation, streaming/downscale image decode, 더 깊은 Splash bitmap
  lifetime 관리 같은 큰 설계 변경이 필요하다.
- [x] `tmp/codex_perf/slowpage_profile_work_20260615`에서 현재 slow-page
  profiling을 이어 진행했고, exactness는 안전하지만 성능 신호가 나쁜 probe들을
  유지하지 않았다. 현재 focused leader를 다시 확인했다: `doc_100.pdf` 150 DPI
  page 4는 `0.35-0.38s`, `79.3-80.3MB` TotalAlloc, 약 `69.7-70.4MB` MaxRSS;
  `doc_070.pdf` 300 DPI page 9는 `0.24-0.27s`, `61.5-62.2MB` TotalAlloc, 약
  `67.7-68.0MB` MaxRSS; page 11은 `0.24-0.26s`, `60.1-60.4MB` TotalAlloc,
  약 `71.5-72.5MB` MaxRSS였다. Page 4 alloc-space는 고정 Splash bitmap
  (`NewBitmap` `14.13MB` flat), dictionary/parser map growth(`Dict.Set`
  `11.02MB` flat), Flate growth(`bytes.growSlice` `6.34MB` flat),
  Type1/SFNT glyph 작업으로 분산되어 있었다. Page 9/11은 고정 300-DPI page RGB
  bitmap(`32.11MB`)과 ImageMagick JPEG stdout buffer(`11.12MB`, `13.75MB`)가
  지배적이었다. Group-bitmap/soft-mask scratch pool probe는 p4/p9 PNG가
  byte-identical이어도 allocation/RSS가 flat 또는 악화되어 유지하지 않았다.
  djpeg-go 기본 활성화는 p9가 byte-identical이고 약간 빨랐지만 TotalAlloc/RSS가
  약 `68MB` / `70-75MB`로 악화되어 유지하지 않았다. Parser dict 초기 capacity
  `4 -> 8` probe도 p4가 byte-identical이지만 안정적인 allocation win이 없어
  되돌렸다. 이번 pass에서 production code 변경은 유지하지 않았다. 관련 Splash,
  parser, `pdfrender` 테스트와 `pdfrender` 빌드는 통과했다.
- [x] `tmp/codex_perf/slowpage_20260615_current`에서 slow page를 계속 하나씩
  profiling하고 output-neutral AA scanner cleanup을 유지했다. 현재 full
  page-profile leader는 `doc_100.pdf` 150 DPI page 4(`796ms`, `79,719,016`
  alloc, wall은 outlier지만 focused는 안정적으로 `0.36-0.39s`), `doc_070.pdf`
  300 DPI page 11(`396ms`, `60,093,712` alloc), page 9(`357ms`, `61,881,400`
  alloc) 순서였다. Page 4 pprof는 고정 Splash bitmap, dictionary/object parsing,
  Flate growth, Type1/SFNT glyph 작업, scanner storage로 계속 분산되어 있었고,
  page 9는 고정 300-DPI bitmap과 ImageMagick JPEG stdout/Flate decoded buffer가
  지배적이었다. `parseArray` capacity-4 probe는 byte-identical이지만 p4/p9/p11
  기준 mixed/noisy라 되돌렸다. 유지한 변경은 `clearBitsRange`,
  `clearBitsRangePopplerClip`, `clearBitsRangePopplerClipGap`의 full-byte AA
  gap-zeroing loop를 `clear(slice)`로 바꾸는 것이며, partial-byte mask와 반환
  cursor는 그대로 보존했다. Focused p4/p9/p11 렌더는 PNG byte-identical이었다.
  Full 검증도 `doc100_150`, `doc070_300` 모두 report-identical이었다
  (`report_metric_diffs=0`). `stage=ours`는 `doc100_150`
  `2642ms / 847,261,816` alloc / `70,560KB` MaxRSS에서
  `2250ms / 847,097,200` / `70,080KB`로, `doc070_300`
    `1766ms / 539,035,928` / `142,560KB`에서
    `1617ms / 539,237,136` / `142,240KB`로 이동했다. 가장 큰 page-level wall
    개선은 `doc070` p9 `357ms -> 248ms`, p11 `396ms -> 257ms`였고, allocation은
    예상대로 flat/noisy였다. 관련 XPath/Splash/CLI 테스트, no-CGo build,
    `git diff --check`가 통과했다.
- [x] 현재 slow page를 `tmp/codex_perf/current_profile_20260615`에서 이어서
  focused profiling했다. 유지한 pure-Go 변경은 두 가지다. TrueType `loca`/`hmtx`
  table parsing은 per-entry `binary.Read` 대신 table payload를 한 번 읽어 decode하고,
  FreeType-Go face wrapper는 같은 locked face/size의 반복 `SetPixelSizes` 호출을
  건너뛴다. Focused p4/p9/p11 렌더는 PNG byte-identical을 유지했다. `doc_100.pdf`
  p4 TotalAlloc은 대략 `78.9MB -> 77.4MB`, `doc_070.pdf` p9는 대략
  `62.2MB -> 61.1MB`로 줄었고 p11은 거의 flat이었다. p9 20회 반복 CPU profile에서
  `sfnt.Face.SetPixelSizes` 누적은 약 `0.20s -> 0.03s`로 감소했다. Splash direct
  RGB8 image-write fast path는 byte-identical이었지만 stable speed/allocation win이
  없어 되돌렸다. 남은 p9/p11 비용은 image blit/output 및 transient DCT RGB buffer이고,
  p4는 parser/dict lookup, glyph work, group composite, Splash scanner/image 경로로
  분산되어 있다.
- [x] `tmp/codex_perf/slowpage_profile_next_20260615`에서 다음 slow-page
  profiling pass를 진행했다. 같은 binary focused baseline 평균은 `doc_100.pdf`
  page 4가 `0.327s` / `77.43MB` TotalAlloc, `doc_070.pdf` page 9가 `0.227s` /
  `60.91MB`, page 11이 `0.193s` / `60.01MB`였다. Page 4 CPU/heap profile은
  반복 Form 실행, soft-mask/image draw, glyph rendering, parser/dict lookup,
  group composite, 고정 Splash bitmap으로 분산되어 있었다. Page 9는 여전히
  decoded ImageMagick JPEG stdout/raster buffer, Flate decoded stream buffer,
  고정 300-DPI page bitmap이 구조적으로 지배했다. Parser dict capacity `4 -> 8`,
  inline image dict capacity, PNG stored-stream raw-row fast path, Flate/image
  exact decode-size hint, Form first-use operator cache, gray-mask row helper,
  generic JPEG djpeg-go 활성화 probe는 byte-identical이지만 flat/worse이거나
  byte가 바뀌어서 모두 유지하지 않았다. djpeg-go는 page 9만 byte-identical이고
  allocation/RSS가 증가했으며, pages 4/11은 byte가 변경됐다. 이번 pass에서
  production code 변경은 유지하지 않았다. 관련 stream/renderer/Splash/`pdfrender`
  테스트는 `CGO_ENABLED=0 PDF_FREETYPE_GO=1`로 통과했다. 남은 material gain은
  streaming/downscale image decode, Splash bitmap lifetime reuse,
  parser/dictionary representation 변경, 또는 더 좁은 composite/glyph hot-path
  작업 같은 큰 설계가 필요하다.

- [x] temporary glyph trace metadata를 `GlyphBitmap`에서 제거한 뒤 현재 slow
  leader를 `tmp/codex_perf/slowpage_fresh_20260615`에서 재확인하고, p4/p9를
  `tmp/codex_perf/slowpage_profile_20260615`에서 pprof로 분석했다. 비교 artifact를
  보존하지 않는 fresh page-profile 결과는 `doc_100.pdf` 150 DPI page 4가
  `348ms` / `77,155,144` TotalAlloc / `68,000KB` MaxRSS, `doc_070.pdf` 300 DPI
  page 9가 `224ms` / `61,187,544` / `68,000KB`, page 11이 `199ms` /
  `60,205,192` / `72,320KB`였다. Page 4 pprof는 여전히 고정 Splash bitmap
  (`NewBitmap` `15.13MB` alloc-space), Flate/Form decode growth, dictionary map
  materialization, `NewReal`, scanner storage, Type1/SFNT glyph 작업으로 분산되어
  있었다. Page 9는 고정 300-DPI page bitmap(`32.11MB`), ImageMagick JPEG stdout
  / raster buffer(`10.62MB`), Flate decoded stream buffer(`6.82MB`)가 구조적으로
  지배했다. `-keep-images=false` 검증에서 bad pixel/region 출력이 없을 때 쓰는
  `pdfcompare` RGBA/NRGBA no-diagnostics compare fast path를 유지했다.
  `tmp/codex_perf/slowpage_comparefast_20260615` 검증은 report metric 동일성을
  유지했다(`0` diffs). Compare stage wall은 `doc_100` p4 `39ms -> 25ms`,
  `doc_070` p9 `155ms -> 111ms`, p11 `154ms -> 111ms`로 줄었다. 남은 renderer
  memory win은 streaming/downscale image decode, PNG/NetPBM 없는 renderer 출력,
  parser/dictionary representation 변경, 더 깊은 Splash bitmap lifetime reuse 같은
  큰 설계가 필요하다.

- [x] 현재 slow page를
  `tmp/codex_perf/slowpage_current_20260615_codex`,
  `tmp/codex_perf/slowpage_p4_profile_20260615_codex`,
  `tmp/codex_perf/p9_profile_20260615_codex`,
  `tmp/codex_perf/p11_profile_20260615_codex`에서 계속 하나씩 profiling했다. 현재
  leader는 `doc_100.pdf` 150 DPI page 4(`326ms` / `77,797,984` alloc),
  `doc_070.pdf` 300 DPI page 9(`234ms` / `60,657,712` alloc), page 11(`197ms` /
  `60,132,808` alloc)이다. 출력이 바뀌지 않는 debug policy cleanup을 유지했다:
  `PDF_DEBUG_SKIP_IMAGES`, `PDF_DEBUG_SKIP_XOBJECTS`,
  `PDF_DEBUG_POPPLER_TEXT_SHIFT`를 text/XObject hot path에서 매번
  `os.Getenv`로 읽지 않고 process init 시점에 cache한다. Focused p4 렌더는 PNG
  byte-identical이었고, p4 반복 `MemTotalAlloc`은 약 `77.2..77.5MB`에서
  `76.7..77.3MB`로 이동했으며 wall은 `0.33s`로 flat이었다. Broad 검증
  `tmp/codex_perf/envcache_verify_20260615_codex`는 `doc100_150`과 `doc070_300`
  report metric 동일성을 유지했다(`report_diffs=0`). p9/p11 재프로파일에서는
  유지할 작은 안전 patch가 없었다. 둘 다 여전히 고정 300-DPI Splash bitmap,
  ImageMagick JPEG stdout/raster buffer, Flate decoded stream buffer가 지배한다.
  관련 renderer/`pdfrender`/`pdfcompare` 테스트, no-CGo build, `git diff --check`가
  통과했다.

- [x] 다음 one-by-one slow-page pass를
  `tmp/codex_perf/slowpage_next_20260615_codex2`의 focused p4/p9/p11 profile과
  `tmp/codex_perf/renderpage_release_verify_20260615_codex` 검증 출력으로 이어
  진행했다. Splash Y-down/X-down post-transform direct image-write probe는
  p4/p9/p11 PNG byte-identical을 유지했지만 stable win이 없었다. p4 반복
  no-profile median은 `0.34s`로 flat이고 allocation은 `77.31MB -> 77.69MB`로
  움직였으므로 image-path 코드는 유지하지 않았다. 대신 `RenderPage` lifecycle
  fix를 유지했다. `Image()`가 반환 image를 분리한 뒤 `Release()`를 구현한 canvas를
  bitmap pool로 돌려보내고, render error 경로에서도 partially built canvas를
  release한다. 이 변경은 주로 `RenderPage` API와 direct가 아닌 CLI fallback에
  영향을 준다. direct Splash PNG 출력은 이미 `RenderPageCanvas`와 explicit release를
  사용하고 있었다. 임시 p9 300-DPI 5회 반복 probe에서 release 없는 old-style
  `RenderPageCanvas+Image`는 `462,396,160` TotalAlloc / `1.528s`였고, 새
  `RenderPage` 경로는 `328,071,760` TotalAlloc / `1.093s`였다. 이는 첫 렌더 이후
  300-DPI page bitmap을 재사용한 효과와 일치한다. p4/p9/p11 검증 PNG는 계속
  byte-identical이었고, renderer/Splash/`pdfrender` 테스트는
  `CGO_ENABLED=0 PDF_FREETYPE_GO=1`로 통과했다.
- [x] 현재 slow leader를 대상으로
  `tmp/codex_perf/slowpage_20260615_profile`에서 one-by-one profiling을
  계속했다. Fresh focused baseline은 `doc_100.pdf` 150 DPI page 4가 `0.50s` /
  `78,819,704` TotalAlloc / `66,400KB` MaxRSS, `doc_070.pdf` 300 DPI page 9가
  `0.40s` / `61,888,312` / `67,840KB`, page 11이 `0.30s` /
  `61,348,240` / `71,840KB`였다. Heap profile상 p9/p11은 여전히 고정 300-DPI
  Splash bitmap(`NewBitmap` 약 `32MB`)과 image/stream buffer(`bytes.growSlice`
  p9 약 `18MB`, p11 약 `14MB`)가 지배했다. 유지한 변경은 Splash image clipped-AA
  cleanup이다. Image blit에서 기존 `ClipAALineFullWidth` 경로를 기본으로 쓰고,
  `PDF_DEBUG_SPLASH_DISABLE_FULL_WIDTH_AABUF=1`로 끌 수 있게 유지했으며, 관련
  image clip debug env toggle은 process init 시점에 cache한다. Focused old/new
  PNG는 byte-identical이었다. No-profile focused 측정은 p9
  `0.40s / 61,888,312 -> 0.33s / 60,815,720`, p11
  `0.30s / 61,348,240 -> 0.21s / 60,140,064`, p4
  `0.50s / 78,819,704 -> 0.37s / 77,380,712`로 이동했다. Per-pixel clip
  span-gate probe는 p9 출력이 달라져 rejected 처리했다. Full old/new document
  render는 byte-identical이었다(`doc_070` `0/11` changed, `doc_100` `0/31`
  changed). 다만 aggregate wall/RSS는 noise가 있었다(`doc_070`
  `1.20s/112000KB -> 1.27s/111520KB`, repeat는 `1.62s old` vs `1.57s patched`;
  `doc_100` `1.87s/94060KB -> 1.93s/96144KB`). 관련
    Splash/`pdfrender`/`pdfcompare` 테스트와 `git diff --check`가 통과했다. 남은
    material memory win은 여전히 fixed bitmap lifetime/alpha-plane 설계와 streaming
    image decode/output 쪽이다.

- [x] 현재 `doc_100.pdf`와 `doc_070.pdf` set에서 slow-page profiling을 이어서
  진행했다. `doc_100.pdf` 150 DPI page 4가 여전히 가장 느리고, 그 다음은
  `doc_070.pdf` 300 DPI pages 9/11이다. Page 4 profiling에서는 text/glyph cache와
  debug text-code check가 hot path에 있었다. Splash canvas가 `ReleaseTransientCaches`
  를 제공하고, concurrent renderer가 page bitmap을 그린 직후 glyph bitmap cache를
  버리도록 했다. `NewEvaluator`도 `PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE`를 snapshot
  해서 일반 렌더링 중 per-character `os.Getenv`/debug-code parsing을 피한다.
  Focused page 4는 PNG byte-identical을 유지했고, glyph-cache release는
  `MemTotalAlloc`을 `78,545,312 -> 77,690,616`, live `MemAlloc`을
  `44,997,352 -> 43,244,888`로 이동시켰다. Text-policy CPU profile에서는
  `debugTextCodeSetForBase`/`ShouldSkipTextCode` sample이 사라졌고, p4 출력은
  byte-identical이었다. `tmp/codex_perf/textpolicy_glyph_verify_20260615` full 검증은
  `tmp/codex_perf/envcache_verify_20260615_codex`와 report-identical이었다. Aggregate
  total은 mixed/noisy였다(`doc_100` wall `2060 -> 2033ms`, allocation
  `840,577,432 -> 841,968,392`; `doc_070` allocation `535,321,096 -> 534,726,128`,
  wall outlier `1360 -> 1698ms`). Live cache retention 축소와 hot-path debug-env
  제거 효과 때문에 변경을 유지한다. Resource `GetRaw`와 evaluator resource-resolve
  cache probe는 focused p4 allocation이 개선되지 않아 rejected 처리했다.
- [x] one-by-one slow-page profiling을
  `tmp/codex_perf/slowpage_current_cont_20260615`,
  `tmp/codex_perf/slowpage_p4_cont_20260615`,
  `tmp/codex_perf/slowpage_p9_cont_20260615`,
  `tmp/codex_perf/slowpage_p11_cont_20260615`에서 계속했다. 현재 순위는 여전히
  `doc_100.pdf` 150 DPI page 4(`324ms` / `73.49MiB` alloc),
  `doc_070.pdf` 300 DPI page 9(`227ms` / `58.08MiB`), page 11(`191ms` /
  `57.36MiB`) 순이다. Inline image stream-dictionary normalization probe는 focused
  p4 PNG byte-identical을 유지했고 no-profile p4 allocation을 `76.7..77.3MB`로
  움직였지만, broad 검증에서 flat/regressive였다(`doc100_150` allocation `+2.0MB`,
  `doc070_300` allocation `+0.9MB`). 따라서 probe는 되돌렸다. Parser/entity
  `Dict` inline-storage probe도 rejected 처리했다. Capacity `4`는 `report.csv`
  byte-identical을 유지하고 `doc100` page 4 allocation을
  `77,057,448 -> 76,657,384`로 줄였지만, full aggregate allocation은
  flat/regressive였다(`doc100_150` `840,989,896 -> 841,533,232`, `doc070_300`
  `535,028,648 -> 535,042,496`). Capacity `2`는 focused p4를 `78,212,984`로
  악화시켰다. Page 9/page 11
  profile은 남은 memory가 구조적임을 재확인했다: 고정 300-DPI Splash page bitmap
  (`~32MB`), ImageMagick JPEG stdout/raster buffer(`10.6..14.9MB`), 그리고 page 9의
  Flate decoded stream buffer(`7.7MB`)가 핵심이다. 이번 pass에서는 production code
  변경을 유지하지 않았고, 다음 유의미한 개선은 streaming image decode/output 또는
  Splash bitmap lifetime/alpha-plane 재설계가 필요하다.

- [x] 현재 slow-page profiling을
  `tmp/codex_perf/slowpage_live_20260615`에서 이어서 진행했다. Fresh focused
  no-profile 기준 리더는 여전히 `doc_100.pdf` 150 DPI page 4(`0.34..0.36s`,
  `77.1..77.6MB` TotalAlloc), `doc_070.pdf` 300 DPI page 9(`0.23..0.24s`,
  `60.8..61.0MB`), page 11(`0.19..0.20s`, `60.1..60.3MB`) 순이었다. p4 pprof는
  고정 Splash bitmap allocation, dictionary/object materialization,
  Flate/Form decode buffer, packed image path, Type1/SFNT glyph 작업으로
  분산되어 있었다. p9/p11은 여전히 고정 300-DPI Splash bitmap과 ImageMagick
  decoded JPEG stdout buffer, 그리고 p9의 Flate decoded stream buffer가 구조적
  리더다. 유지한 변경은 출력 중립 encoding cleanup이다. simple font base
  encoding map은 shared read-only map을 쓰고, `/Differences` dictionary만
  mutation 전에 clone한다. Focused old/new p4/p9/p11 렌더는 PNG byte-identical을
  유지했다. Broad 검증도 `doc100_150`, `doc070_300` 모두 report-identical이었다
  (`report_diffs=0`). `doc100_150` ours allocation은
  `524,247,192 -> 519,664,528`으로 줄었고, `doc070_300` allocation은
  `186,115,960 -> 186,453,368`으로 noise 범위였으며 MaxRSS는
  `111,360KB -> 110,080KB`로 이동했다. 관련 renderer/CLI 테스트, no-CGo build,
  `git diff --check`가 통과했다. 남은 material win은 streaming/downscale image
  decode, PNG/raw-output pipeline 변경, 더 깊은 Splash bitmap lifetime/alpha-plane
  재설계가 필요하다.

- [x] 현재 compare 비용이 큰 focused slow page들을 이어서 profiling했다. JPEG
  raw-raster metadata probe는 p4/p9/p11 출력이 byte-identical이었지만 allocation이
  flat/noisy라 rejected 처리했다. 유지한 변경은 `pdfcompare` RGBX fast path다.
  diagnostics 또는 diff PNG가 필요한 same-size RGBA/NRGBA 페이지에서 row offset으로
  직접 비교하고 XOR diff image도 직접 기록해 per-pixel `SetRGBA`/`PixOffset` 호출을
  피한다. Focused p4/p9/p11/p10 report와 diff PNG는 byte-identical을 유지했다.
  Compare-stage wall은 p4 `101ms -> 89ms`, p9 `376ms -> 340ms`, p11
  `371ms -> 322ms`, p10 `369ms -> 317ms`로 이동했다. Full `doc100_150`
  검증은 report 및 diff PNG가 동일했다(`31/31` diff PNG unchanged), compare wall은
  `2765ms -> 2421ms`로 이동했다. Split `doc070_300` pages 1-8 및 page 10도
  report 및 diff PNG가 동일했고(`9/9` diff PNG unchanged), compare wall은
  `2991ms -> 2544ms`로 이동했다. 관련 `pdfcompare`, `pdfrender`, image 테스트,
  no-CGo build, `git diff --check`가 통과했다.
- [x] 현재 slow page들을 `tmp/codex_perf/slowpage_reprofile_20260615`에서
  하나씩 다시 profiling했다. Fresh `page_profile.csv` 순위는 `doc_100.pdf`
  150 DPI page 4가 1순위(`334ms`, `74.64MiB` alloc), 그 다음이
  `doc_070.pdf` 300 DPI page 9(`239ms`, `58.04MiB`)와 page 11(`216ms`,
  `57.40MiB`)이다. Focused p4 pprof는 여전히 고정 Splash page bitmap
  allocation(`NewBitmap` `13.78MiB`), Flate/Form buffer(`bytes.growSlice`
  `7.90MiB`), dictionary insertion(`Dict.Set` `6.52MiB`), scanner storage,
  Type1/SFNT 작업으로 분산되어 있다. Focused p9는 고정 300-DPI page bitmap
  allocation(`32.11MiB`), ImageMagick JPEG stdout raster buffer(`10.62MiB`),
  Flate decoded buffer(`6.17MiB`)가 구조적으로 지배한다. 출력 중립 Splash AA
  probe 두 가지는 rejected 처리했다. Bounds-check-free `aaBufCoverageAt` image
  helper와 `ClipAALineFullWidth` loop-bounds clamp 모두 p4/p9/p11 PNG는
  byte-identical을 유지했지만, 5회 focused 측정에서 flat 또는 p9/p11 회귀였다.
  이번 pass에서는 production code 변경을 유지하지 않았고, 다음 유의미한 개선은
  streaming image decode/output 또는 더 깊은 Splash bitmap alpha-plane/lifetime
  재설계가 필요하다.
- [x] `tmp/codex_perf/slowpage_reprofile_20260615` 이후 one-by-one slow-page
  profiling을 이어서 진행했다. 유지한 변경은 Type1 embedded font 초기화 cleanup이다.
  pure-Go FreeType Type1 adapter가 raw Type1 program을 직접 렌더링할 수 있는 경우
  synthetic Type1-to-OTF 생성과 `sfnt.Parse`를 건너뛰고,
  `PDF_DEBUG_TYPE1_FT_SOURCE=otf`로 OTF 경로를 명시한 경우에만 기존 fallback을
  만든다. Focused old/new 렌더는
  `tmp/codex_perf/type1_nootf_probe_20260615_082954`에서 `doc_100.pdf` page 4와
  `doc_070.pdf` pages 9/11 모두 PNG byte-identical이었다. Full 검증
  `tmp/codex_perf/type1_nootf_full_20260615_083115`은
  `tmp/codex_perf/slowpage_reprofile_20260615` 대비 `doc100_150`과 `doc070_300`
  report diff가 `0`이었다. `doc100_150` `stage=ours` allocation은
  `841,495,504 -> 818,240,352`, `doc070_300`은 `534,430,544 -> 527,621,248`로
  줄었고 focused wall time은 flat/noisy였다. 관련 Type1/freetype/renderer/Splash/CLI
  테스트, no-CGo build, `git diff --check`가 통과했다. 남은 유의미한 개선은
  streaming image decode/output 또는 더 깊은 Splash bitmap alpha-plane/lifetime
  재설계가 필요하다.
- [x] 현재 slow page들을 `tmp/codex_perf/slowpage_onebyone_20260615_084217`,
  `tmp/codex_perf/resource_category_probe_20260615_084950`,
  `tmp/codex_perf/resource_category_full_20260615_085320`에서 한 페이지씩 이어서
  profiling했다. Focused 반복 측정 결과 `doc070` page 2의 full-profile wall spike는
  outlier였다(`0.11..0.12s`). 안정적인 leader는 `doc_100.pdf` 150 DPI page 4
  (`0.33..0.36s`, `~77MB` alloc), `doc_070.pdf` 300 DPI page 9(`0.27..0.29s`,
  `~61MB` alloc), page 11(`0.25..0.28s`, `~60MB` alloc, scheduler outlier 1회
  제외)이다. 유지한 변경은 출력 중립 resource lookup cleanup이다. renderer hot path의
  resource category lookup이 `/Font`, `/XObject`, `/ExtGState`, `/Shading`,
  `/Pattern`, `/ColorSpace`처럼 slash-prefixed constant를 넘겨 `Dict.Get`의 반복
  slash-fallback 작업을 피한다. Focused p4/p9/p11 old/new PNG는 byte-identical이었다.
  Full 검증은 `tmp/codex_perf/type1_nootf_full_20260615_083115` 대비
  `doc100_150`, `doc070_300` 모두 report-identical이었다(`report_diffs=0`).
  `doc100_150` ours totals는 `2077ms / 818,240,352` alloc / `66,400KB` MaxRSS에서
  `2063ms / 818,044,560` alloc / `68,000KB`로 이동했고, `doc070_300`은
  `1741ms / 527,621,248` alloc / `141,920KB`에서
  `1386ms / 527,689,576` alloc / `141,920KB`로 이동했다. Image decoded-stream
  no-cache probe는 p4/p9 total allocation 및 RSS가 회귀해 rejected 처리했고,
  p11 JPEG 경로의 `djpeg-go` 활성화도 PNG가 달라지고 allocation이 증가해
  rejected 처리했다(`~60.6MB -> 69.5MB`). p9/p11 profile은 여전히 fixed 300-DPI
  page bitmap(`~32.9MB`)이 live heap을 지배하고, p11의 남은 image allocation은
  ImageMagick JPEG stdout raster buffer에서 발생한다. 관련 renderer/image/Splash/CLI
  테스트, no-CGo build, `git diff --check`가 통과했다.
- [x] 현재 slow leader들을 `tmp/codex_perf/profile_current_20260615_090101`에서
  다시 하나씩 profiling했다. 현재 page-profile leader는 `doc_100.pdf` 150 DPI
  page 4(`335ms`, `76,440,208` alloc), `doc_070.pdf` 300 DPI page 9(`246ms`,
  `60,667,688` alloc), page 11(`218ms`, `59,601,840` alloc)이다. 유지한 변경은
  FreeType-Go face 단위 glyph-name index cache다. 반복 Type1
  `GetGlyphIndexByName` 호출이 face name lookup 경로를 다시 타지 않게 하는
  낮은 위험의 cache이며, focused page 4는 PNG byte-identical이었다. Type1/freetype
  focused alloc-space는 `19.12MB -> 13.62MB`로 줄었다. Full page-profile 효과는
  mixed/noisy지만 report-identical이다. `doc100_150`은
  `2063ms / 819,698,032` alloc / `67,040KB` MaxRSS에서
  `2088ms / 820,643,936` / `66,560KB`로, `doc070_300`은
  `1383ms / 528,622,488` alloc / `141,920KB` MaxRSS에서
  `1387ms / 527,620,184` / `141,920KB`로 이동했다. 재프로파일한 page 9는 live
  heap이 fixed 300-DPI RGB8+alpha page bitmap(`32.9MB`, in-use heap의 `88%`)에
  구조적으로 지배된다. 남은 `ImageMagick` JPEG stdout buffer는 decoded image backing
  store라 중복 copy가 아니므로 streaming으로 바꿔도 대부분 같은 allocation 이름만 바뀔
  가능성이 크다. 관련 freetype/type1/Splash/CLI 테스트와 `git diff --check`가 통과했다.
- [x] `tmp/codex_perf/slowpage_profile_after_form_debug_20260615_092109` 및 관련
  focused A/B 산출물에서 slow leader를 한 페이지씩 계속 profiling했다. 유지한 변경은
  두 가지 출력 중립 renderer cleanup이다. Form XObject 실행은 evaluator-local operator
  cache를 shared cache보다 먼저 확인해 한 evaluator 안에서 shared-cache slice copy가
  반복되지 않게 했고, `executeOperator`는 render context trace가 꺼져 있을 때 debug
  paint-context defer 설치를 건너뛴다. Focused old/new PNG는 `doc_100.pdf` page 4와
  `doc_070.pdf` pages 9/11 모두 byte-identical이었다. Page 4 pprof alloc-space는
  `87.91MB -> 79.87MB`로 이동했지만, 5회 non-pprof median은 flat이었다(`0.32s` 동일,
  `MemTotalAlloc` `76,470,904 -> 76,412,104`). Page 9 pure-Go JPEG 조건의 old/new
  median도 `0.24s`로 flat이고 allocation만 소폭 줄었다(`66,341,552 -> 66,341,272`);
  default ImageMagick wall은 noise가 컸다. p9/p11 재프로파일 결과 큰 memory 비용은
  fixed 300-DPI Splash page bitmap(`~32..33MB`)과 decoded JPEG raster backing store
  (`~14..16MB`)라는 구조적 비용으로 재확인했다. `djpeg-go`는 속도 tradeoff이지 안전한
  기본 memory 개선은 아니었다. Page 9는 exactness가 동일하고 더 빨랐지만(`0.24s ->
  0.21s`) memory가 증가했고, page 11은 더 빨랐지만(`0.26s -> 0.21s`) RSS/alloc이
  증가하고 exactness도 바뀌었다(`99.93628045 -> 99.93407011`). 따라서 JPEG default
  변경은 유지하지 않았다. 관련 renderer/image/freetype/type1/Splash/CLI 테스트와
  `git diff --check`가 통과했다.
- [x] 현재 slow leader를
  `tmp/codex_perf/slowpage_current_20260615_093754`에서 다시 한 페이지씩
  profiling했다. Fresh page-profile 순위는 `doc_100.pdf` 150 DPI page 4
  (`330ms`, `76,968,224` alloc), `doc_070.pdf` 300 DPI page 11(`260ms`,
  `59,426,288` alloc), page 9(`248ms`, `60,682,336` alloc) 순이었다. p4 profile은
  고정 Splash bitmap, `Dict.Set`/parser materialization, Form/Flate buffer,
  Type1/SFNT 작업으로 분산되어 있고, p9/p11 live heap은 fixed 300-DPI page
  bitmap이 `83-90%`를 차지했다. `bytes.growSlice`의 큰 항목은
  `decodeJPEGWithImageMagick` stdout backing store로 확인했으며, 이미 size hint를
  사용하므로 중복 grow가 아니라 실제 decoded raster 저장 비용이다. `YupXdown +
  vFlip` direct path의 x/y map 및 row buffer pool probe는 p4/p9/p11 PNG가
  byte-identical이었지만 focused 5회 측정에서 flat 또는 p9 allocation/wall 회귀라
  rejected 처리했다. Production code 변경은 유지하지 않았고, 다음 유의미한 개선은
  streaming/downscale image decode/output 또는 Splash page bitmap alpha-plane/lifetime
  재설계가 필요하다.
- [x] `tmp/codex_perf/slowpage_profile_20260615_095144`에서 slow page profiling을
  이어서 수행했다. 현재 leader는 `doc_100.pdf` 150 DPI page 4(`339ms`,
  `76,559,640` alloc), `doc_070.pdf` 300 DPI page 9(`248ms`, `60,196,904`
  alloc), page 11(`226ms`, `59,509,664` alloc)이다. CPU/heap profile은 p9/p11
  live heap이 fixed 300-DPI RGB8+alpha page bitmap(`~32.9MB`, in-use heap의
  `88-91%`)에 구조적으로 지배되고, ImageMagick JPEG의 큰 `bytes.growSlice`는 중복
  buffer가 아니라 decoded raster backing store임을 다시 확인했다. 유지한 변경은
  낮은 위험의 parser allocation cleanup 하나다. `parseArray`가 capacity 4로 시작해
  일반적인 PDF array의 slice grow를 줄이며, p4/p9/p11 PNG는 byte-identical이었다.
  Full page-profile A/B에서 `doc100_150` totals는 `2096ms / 820,345,600` alloc에서
  `2096ms / 818,766,688` alloc으로, `doc070_300`은
  `1490ms / 527,818,192` alloc에서 `1460ms / 527,662,376` alloc으로 이동했다.
  Focused page 효과는 p4 `76,559,640 -> 75,777,896` alloc, p9
  `60,196,904 -> 59,923,240`, p11 `59,509,664 -> 59,488,880`이었다.
  Boolean/Null singleton caching은 기존 테스트가 clone pointer 독립성을 요구해
  rejected 처리했고, 작은 `Real` constant caching은 유지한 array-cap build 대비
  allocation이 회귀해 rejected 처리했다(`doc100_150 +520,160`, `doc070_300 +22,936`).
  다음 유의미한 memory 개선은 cropped/offset soft-mask storage, streaming/downscale
  image decode/output, 또는 더 깊은 Splash page-bitmap alpha-plane/lifetime 재설계가
  필요하다. 관련 entity/parser/Splash/CLI 테스트와 `git diff --check`가 통과했다.
- [x] parser array-cap pass 이후 같은 slow leader들을 다시 한 페이지씩 profiling했다.
  CPU/heap pprof 기준 page 4는 단일 안전 hotspot이 아니라 fixed Splash bitmap,
  Form/Flate buffer, dictionary/object parsing, Type1/SFNT 작업, scanner storage,
  packed RGB matte image로 분산되어 있었다. Page 9/page 11은 여전히 fixed 300-DPI
  RGB8+alpha page bitmap과 실제 decoded JPEG/Flate raster backing store가 구조적으로
  지배한다. 유지한 변경은 좁은 Splash bitmap-clear cleanup이다. `ClearWithAlpha`는
  data/alpha plane을 bulk copy helper로 채우고, 내부 fresh transparent page/annotation
  canvas에서는 alpha plane이 방금 할당된 경우에만 중복 alpha-zero fill을 생략한다.
  Public `ClearWithAlpha` 동작은 내부 regression test로 보존했다. Focused p4/p9/p11
  old/new PNG는 byte-identical이었다. 5회 focused 측정은 material memory win이 아니라
  micro-optimization signal로만 해석한다. p9 wall은 median 기준 소폭 개선됐다
  (`0.24s -> 0.23s`), p11 allocation은 소폭 줄었다(`59,641,840 -> 59,559,832`
  median), p4는 noisy/flat이었다. 남은 유의미한 memory 개선은 streaming/downscale
  image decode/output, cropped/offset soft-mask storage, 또는 더 깊은 page-bitmap
  alpha-plane/lifetime 재설계가 필요하다. 관련 Splash/CLI 테스트, no-CGo build,
  `git diff --check`가 통과했다.
- [x] 현재 slow page들을 `tmp/codex_perf/slowpage_profile_20260615_104425`와 후속
  probe에서 다시 한 페이지씩 profiling했다. Fresh focused profile 기준 leader는
  여전히 `doc_100.pdf` 150 DPI page 4였고(`0.50s` CPU/heap profiling 포함,
  `79,338,592` `MemTotalAlloc`), 그 다음은 `doc_070.pdf` 300 DPI page 9
  (`63,034,848`)와 page 11(`61,974,840`)이었다. Page 4 alloc-space는
  `Dict.Set`/parser materialization(`14.03MB`), fixed Splash bitmap(`11.59MB`),
  Flate/Form buffer growth(`10.53MB`), Type1/SFNT glyph 작업, packed RGB matte
  image로 계속 분산되어 있었다. Page 9/page 11은 fixed 300-DPI RGB8+alpha page
  bitmap(`~32MB`)과 실제 decoded raster backing store가 구조적으로 지배했다. Large
  Form decode hint를 `rawLen*16 -> rawLen*8`로 낮추는 probe는 p9만 소폭 개선되고
  p4가 회귀해 rejected 처리했다(`77,580,912 -> 79,576,584` median
  `MemTotalAlloc`). 유지한 변경은 더 낮은 위험의 stream allocation cleanup이다.
  `/DecodeParms`가 없는 일반 stream에서는 `getDecodeParamsList`가 `[]*Dict`를
  할당하지 않는다. Focused p4/p9/p11 PNG는 byte-identical이었고 median allocation은
  p4 `77,580,912 -> 77,314,744`, p9 `61,768,896 -> 61,513,832`, p11은 flat
  (`60,628,672 -> 60,634,360`)이었다. Full current render
  `tmp/codex_perf/decodeparms_nil_full_20260615_105211`는
  `tmp/codex_perf/slowpage_profile_20260615_095144` 대비 `doc100_150` 및
  `doc070_300` 모두 report-identical이었다(`metric_diffs=0`). 관련 stream/CLI
  테스트, no-CGo build, `git diff --check`가 통과했다. 남은 material memory win은
  fixed bitmap alpha-plane/lifetime 재설계, cropped/offset soft-mask storage,
  streaming/downscale image decode/output이다.
- [x] `tmp/codex_perf/profile_onebyone_20260615_110844`에서 slow page를 계속 한
  페이지씩 profiling했다. 유지한 변경은 낮은 위험의 Splash scale-kernel cleanup이다.
  `scaleImageYdownXdown`, `scaleImageYdownXup`, `scaleImageYupXdown`,
  `scaleImageYupXup`, center-mapped `YupXup`이 row/pixel buffer를 매번 새로 만들지
  않고 기존 image-scale scratch pool을 재사용한다. Focused `doc_100.pdf` page 4
  출력은 pixel-identical이었고 가장 강한 focused signal에서 no-profile
  `MemTotalAlloc`은 `77,451,336 -> 76,306,832`로 이동했다. Per-page full 검증
  `tmp/codex_perf/scale_pool_chunk1_20260615_111440`은
  `tmp/codex_perf/decodeparms_nil_full_20260615_105211` 대비 `doc100_150`과
  `doc070_300` 모두 report-identical이었다. Page 4는 `321 -> 309ms`,
  `73.27 -> 72.62MiB` allocation으로 이동했고, `doc070_300` 전체 효과는
  flat/noisy였지만 RSS는 소폭 낮아졌다(`139.1 -> 138.4MiB`). Fresh page 9 heap
  profile에서 in-use memory는 여전히 fixed 300-DPI RGB8+alpha page bitmap이
  지배했다(`32.88MiB`, in-use `87.39%`). 남은 큰 allocation은 ImageMagick/Flate
  raster backing buffer라 즉시 제거하기 어렵다. `Real` constant-cache probe는
  focused page 4 allocation 개선이 없어 rejected 처리했다. 관련 Splash/CLI 테스트,
  no-CGo build, `git diff --check`가 통과했다.
- [x] scale-kernel cleanup 이후 현재 slow leader들을
  `tmp/codex_perf/slowpage_live2_20260615`에서 다시 profiling했다. 최신
  page-profile 순위는 `doc_100.pdf` 150 DPI page 4(`326ms`, `73.47MiB` alloc),
  `doc_070.pdf` 300 DPI page 9(`246ms`, `57.41MiB`), page 11(`191ms`,
  `56.53MiB`)이었다. Focused heap profile에서 page 9/page 11의 in-use memory는
  fixed 300-DPI RGB8+alpha page bitmap이 지배했다(`~32.88MiB`, in-use heap의
  약 `88-90%`). `bytes.growSlice`로 보이는 hotspot은 ImageMagick PPM/PGM stdout
  backing store이며 decoded RGB/Gray image가 그대로 참조하므로 제거 가능한
  중간 복사가 아니다. `interpolate=true` `YdownXdown` direct-draw probe는 5회
  A/B가 mixed이고 page 9 wall time이 회귀해 rejected 처리했다. Alpha-less page
  bitmap probe는 allocation/RSS는 줄었지만 p4/p9/p11 focused PNG가 바뀌어
  rejected 처리했다. 다음 material memory 개선은 exactness를 보존하는 page
  alpha-plane/lifetime 재설계, streaming/downscale image decode/output, 또는
  soft-mask matte unblend row-source drawing 같은 구조 변경이 필요하다. Type1 glyph
  cache key는 이미 font identity, transform, size, Poppler-style phase slot을
  포함하므로 phase를 더 합치는 방식은 낮은 위험의 cache fix가 아니라 exactness
  위험으로 분류한다.
- [x] 최신 focused slow-page profiling 이후 낮은 위험의 soft-mask matte unblend
  row-source 경로를 구현했다. `newPackedRawRGBMatteImageForCanvas`는 더 이상
  unblended RGB 전체 복사본을 만들지 않고, Splash가 packed row-source fast path로
  RGB row를 지연 소비한다. 또한 `alpha == 255` matte unblend는 정확히 no-op이므로
  division을 건너뛴다. Focused old/new render
  `tmp/codex_perf/matte_row_fastalpha_focus_20260615_121103`에서 `doc_100.pdf`
  150 DPI page 4, `doc_070.pdf` 300 DPI page 9, page 11은 모두
  byte-identical이었다. 5회 `MemTotalAlloc` median은 page 4에서
  `76,362,936 -> 74,459,464` bytes로 내려갔고, 해당 run의 wall-time median은
  동률이었다. Page 9/page 11은 예상대로 fixed 300-DPI page bitmap이 지배해
  flat/noisy였다. 전체 페이지 검증
  `tmp/codex_perf/matte_row_fastalpha_full_20260615_121159`은
  `tmp/codex_perf/profile_iter_20260615_115143` 대비 `doc100_150`(31 pages)과
  `doc070_300`(11 pages) 모두 report-identical이었다. Page-profile 기준 page 4
  allocation은 `70.80MiB`로 내려갔다. 관련 renderer/Splash/CLI 테스트, no-CGo
  build, `git diff --check`가 통과했다.
- [x] `tmp/codex_perf/matte_row_fastalpha_full_20260615_121159` 기준으로
  page-by-page profiling을 계속했다. 현재 leader는 `doc_100.pdf` 150 DPI page
  4(`324ms`, `70.80MiB` alloc), `doc_070.pdf` 300 DPI page 9(`239ms`,
  `57.55MiB`), page 11(`224ms`, `56.52MiB`)이었다. Fresh pprof
  `tmp/codex_perf/profile_onebyone_20260615_122943`에서 page 4는 fixed Splash
  bitmap, parser dictionary, Form/Flate buffer, font 작업, scanner/image 작업에
  계속 분산되어 있었고, page 9 in-use heap은 fixed 300-DPI RGB8+alpha page
  bitmap이 지배했다(`32.88MiB`, in-use `92.60%`). 유지한 변경은 좁은 Splash
  AA-buffer cleanup이다. Clipped image/mask 경로가 `aaBuf`를 byte loop로 채우지
  않고 기존 bulk `fillBitmapBytes` helper로 채운다. Focused old/new PNG는
  page 4/page 9/page 11 모두 byte-identical이었다. Same-session median은
  mixed이지만 image-heavy page에서는 유리했다. Page 9 wall/alloc은 `0.27s /
  61,545,952 -> 0.26s / 61,335,672`, page 11 wall은 `0.26s -> 0.25s`로
  이동했고, page 4는 wall 기준 noise-flat/slightly worse였다(`0.34s -> 0.35s`).
  Full 검증 `tmp/codex_perf/aabuf_fill_full_20260615_123513`은
  `tmp/codex_perf/matte_row_fastalpha_full_20260615_121159` 대비 `doc100_150`과
  `doc070_300` 모두 report-identical이었다. `doc070_300` wall은
  `1439ms -> 1296ms`로 이동했고, `doc100_150`은 noise-flat/slightly worse였다
  (`2050ms -> 2068ms`). 관련 Splash/CLI 테스트, no-CGo build,
  `git diff --check`가 통과했다. `uniformAlphaRow`를 `bytes.Count`로 바꾸는
  probe는 PNG-identical이었지만 page 9/page 11 wall/RSS가 회귀해 rejected 처리했다.
  남은 material memory work는 여전히 fixed page bitmap alpha-plane/lifetime
  재설계 또는 streaming/downscale image decode/output이다.
- [x] `tmp/codex_perf/slowpage_profiled_20260615_125233`에서 slow page를
  계속 하나씩 profiling했다. Fresh page-profile leader는 `doc_070.pdf` 300 DPI
  page 11/page 9와 `doc_100.pdf` 150 DPI page 4였다. Focused pprof는 page
  9/page 11 memory가 구조적으로 fixed 300-DPI Splash page bitmap에 지배되고,
  page 4는 fixed bitmap, parser dictionary, Form/Flate buffer, font 작업,
  scanner, image path에 계속 분산되어 있음을 재확인했다. 유지한 변경은 parser
  dictionary allocation cleanup이다. `parseDict`가 가까운 dictionary name density를
  추정해 큰 dictionary로 보일 때만 map 초기 capacity를 키우고, 작은 dictionary의
  broad capacity 증가는 피한다. Focused old/new PNG는 `doc_100.pdf` page 4 및
  `doc_070.pdf` pages 9/11에서 byte-identical이었다. Full 검증
  `tmp/codex_perf/dictcap_adaptive_probe_20260615_125733`은
  `tmp/codex_perf/slowpage_profiled_20260615_125233` 대비 `doc100_150`과
  `doc070_300` 모두 report-identical이었다(`report_metric_diffs=0`). Aggregate
  `stage=ours` allocation은 `doc100_150`에서 `818,271,104 -> 817,285,304`,
  `doc070_300`에서 `528,134,424 -> 527,974,912`로 이동했고, MaxRSS는
  `67,040KB -> 65,440KB`, `142,720KB -> 141,920KB`로 이동했다. 관련 parser,
  renderer, Splash, CLI 테스트, no-CGo build, `git diff --check`가 통과했다.
- [x] adaptive dictionary pass 이후 현재 slow-page leader들을
  `tmp/codex_perf/slowpage_user_20260615_130629`에서 다시 하나씩 profiling했다.
  Fresh all-page ranking은 `doc_100.pdf` 150 DPI page 4가 1순위(`316ms`,
  `74,648,144` alloc), 그다음이 `doc_070.pdf` 300 DPI page 9(`219ms`,
  `60,631,904` alloc), page 11(`185ms`, `59,357,936` alloc), `doc_100.pdf`
  pages 8/22(`146ms`/`140ms`)이었다. Focused profile은
  `profile_doc100_p4`, `profile_doc070_p9`, `profile_doc070_p11`,
  `profile_doc100_p8`, `profile_doc100_p22`에 남겼다. Page 4는 여전히 fixed
  Splash bitmap, parser dictionary growth, Form/Flate buffer, font 작업,
  scanner storage, packed matte image row로 분산되어 있었다.
  `formStreamDecodeSizeHint` medium multiplier를 `6 -> 8`로 올리는 scoped
  probe는 PNG-identical이었지만 p4 median wall이 `0.32s -> 0.33s`로 회귀하고
  TotalAlloc 개선은 noise 수준이라 rejected 처리했다. Pages 9/11은 fixed
  300-DPI RGB8+alpha page bitmap이 구조적으로 지배했다(`32.88MiB`,
  in-use `88-92%`). 큰 `bytes.growSlice` sample은 ImageMagick PPM/PGM stdout
  backing store와 Flate decoded-raster preallocation이며 제거 가능한 추가 복사가
  아니다. Pages 8/22는 이미 낮은 수준(`~0.14s`)이고 주로 pattern/vector 또는
  fixed bitmap 작업이었다. 이번 pass에서 production code 변경은 유지하지 않았다.
  남은 material memory work는 exactness-safe page alpha-plane/lifetime 재설계나
  streaming/downscale image decode/output이다.
- [x] `tmp/codex_perf/slowpage_user_20260615_131953`에서 slow-page pass를
  이어서 진행했다. Fresh all-page profiling 기준 `doc_100.pdf` 150 DPI page
  4가 1순위(`341ms`, `74,272,168` alloc)였고, 그다음은 `doc_070.pdf` 300 DPI
  page 9(`229ms`, `60,323,480`)와 page 11(`220ms`, `59,332,120`)이었다.
  Focused pprof는 page 4가 fixed Splash bitmap, parser dictionary map,
  Form/Flate buffer, Type1/SFNT font 작업, scanner storage, Standard14 AFM/OTF
  lazy read로 계속 분산되어 있고, page 9/page 11은 fixed 300-DPI RGB8+alpha
  page bitmap과 retained decoded JPEG/Flate raster buffer가 지배함을 재확인했다.
  유지한 변경은 낮은 위험의 parser cleanup이다. 단일 문자 PDF name을
  allocation-free로 intern하고, `Border`, `FunctionType`, `Rect`, `Title`
  dictionary key도 literal key로 처리한다. Focused p4/p9/p11 old/new PNG는
  byte-identical이었다. Broad 검증
  `tmp/codex_perf/slowpage_user_20260615_131953/name_intern_verify_full`은
  `doc100_150`과 `doc070_300` 모두 report-identical이었다. Aggregate
  `stage=ours` allocation은 `doc100_150`에서 `818,178,448 -> 816,965,568`로
  이동했고, `doc070_300`은 noise-flat이었다(`527,939,136 -> 528,393,656`,
  wall `1386ms -> 1305ms`). Decoded-stream second-use cache 정책은 PNG-identical
  이었지만 p4/p9 TotalAlloc과 live heap을 악화시켜 rejected 처리했다. 관련
  parser/CLI 테스트, no-CGo build, `git diff --check`가 통과했다.

- [x] 사용자가 지정한 현재 느린 리더인 `doc_100.pdf` 150 DPI page 4와
  `doc_070.pdf` 300 DPI page 9를
  `tmp/codex_perf/slowpage_ft_face_content_cache_20260615_141252`에서 focused
  profiling으로 이어서 확인했다. 유지한 변경은 기존 FreeType face pointer-key
  fast path를 보존하면서, 동일한 embedded font bytes가 다른 backing slice로
  다시 decode될 때 content-hash fallback으로 같은 face를 재사용하게 하는 것이다.
  반복 렌더 PNG는 `tmp/codex_perf/slowpage_profile_20260615_133838` 기준과
  byte-identical이었다. 20회 반복 렌더 기준 page 4는 `MemTotalAlloc
  1,017,755,496 -> 963,666,608`, live `MemAlloc 97,400,992 -> 44,728,544`,
  MaxRSS `140,396KB -> 94,312KB`, wall `5.83s -> 5.73s`로 이동했다. Page 9는
  `522,088,872 -> 494,415,928`, live `55,900,880 -> 50,102,624`, MaxRSS
  `119,072KB -> 108,600KB`, wall은 `4.33s`로 flat이었다. Pointer-key와
  content-key cache 계약을 검증하는 unit coverage를 추가했다. Full-page old/new
  binary 검증 `tmp/codex_perf/ft_face_content_fullcheck_20260615_141918`에서도
  `doc_100` `31/31` page와 `doc_070` `11/11` page가 모두 SHA-identical이었다.
  Full-doc allocation은 `doc_100`에서 `499,232,952 -> 441,534,320`,
  `doc_070`에서 `180,652,328 -> 162,699,296`으로 이동했다. 변경 후 pprof에서
  남은 page-9 `bytes.growSlice` 최대 sample은 ImageMagick NetPBM stdout backing
  store와 Flate decoded-raster buffer였으므로, 이전 streaming parser 및 decoded
  image cache probe는 retained heap/RSS를 키우는 rejected 상태로 유지한다. 관련
  FreeType/Type1/TrueType, renderer, Splash, `pdfrender` 테스트는
  `CGO_ENABLED=0 PDF_FREETYPE_GO=1` 조건에서 통과했다.
- [x] FreeType face content-cache pass 이후 현재 slow leader를
  `tmp/codex_perf/slowpage_current2_20260615_142912`에서 다시 page-by-page로
  profiling했다. 최신 page-profile 순위는 `doc_100.pdf` 150 DPI page 4가
  1순위(`328ms`, `71.35MiB` alloc), 그다음이 `doc_070.pdf` 300 DPI page 9
  (`227ms`, `57.83MiB`), page 11(`183ms`, `56.73MiB`), `doc_100.pdf` pages
  8/22(`159ms`/`157ms`)이다. Focused pprof 산출물은 `profile_doc100_p4`,
  `profile_doc070_p9`, `profile_doc070_p11`, `profile_doc100_p8`에 있다.
  Page 4는 fixed Splash bitmap, parser dictionary map, Flate/Form stream
  buffer, Type1/SFNT font 작업, scanner storage, matte row로 계속 분산되어
  단일 low-risk hotspot이 남아 있지 않다. Page 9/11은 fixed 300-DPI
  RGB8+alpha page bitmap(`~32MiB`, in-use heap의 `84-90%`)과 Flate
  decoded-raster buffer가 구조적으로 지배한다. Page 8은 vector/path 및 Type1
  glyph 작업이 중심이며, 관측된 `NewPath` 비용은 Form BBox clipping state에서
  발생하므로 clip-state lifetime 의미를 바꾸지 않고 안전하게 pool 처리하기
  어렵다. 이번 pass에서는 production code 변경을 유지하지 않았다. 다음 material
  memory 개선은 exactness-safe page alpha-plane/lifetime 재설계 또는
  streaming/downscale image decode/output이다.
- [x] `tmp/codex_perf/slowpage_live2_20260615` 기준으로 slow-page profiling을
  이어서 수행하고, packed RGB matte row-source의 좁은 CPU cleanup을 유지했다.
  Fresh baseline은 `doc_100.pdf` 150 DPI page 4가 계속 1순위(`324ms`,
  `74,369,848` alloc)였고, 그다음은 `doc_070.pdf` 300 DPI page 9(`236ms`,
  `60,900,064`)와 page 11(`191ms`, `59,336,064`)이었다. Focused pprof에서
  page 4는 matte row unblend 내부의 `getGrayVal` sample이 보였고, p9/p11은
  fixed 300-DPI RGB8+alpha page bitmap과 실제 ImageMagick/Flate raster backing
  buffer가 계속 지배했다. `packedRawRGBMatteImage.RGB8Row`는 이제 generic
  per-pixel gray converter 대신 `image.Gray`와 `image.Alpha` mask row를 직접
  읽는다. Focused p4/p9/p11 old/new PNG는 byte-identical이었고, 5회 반복
  median은 p4 `0.33s -> 0.32s`, p9 `0.27s -> 0.25s`, p11
  `0.25s -> 0.23s`로 이동했으며 TotalAlloc은 flat/slightly down이었다. Full
  검증 `tmp/codex_perf/slowpage_live2_20260615/grayrow_verify`는 `doc100_150`,
  `doc070_300` 모두 report-identical이었다(`report_diffs=0`). Aggregate
  `stage=ours`는 `doc100_150` `2073ms / 818,294,896 -> 2047ms / 818,109,264`,
  `doc070_300` `1320ms / 528,453,704 -> 1277ms / 528,159,816`로 이동했다.
  관련 renderer/Splash/CLI 테스트, no-CGo build, `git diff --check`가 통과했다.
  남은 material memory 개선은 여전히 exactness-safe page alpha-plane/lifetime
  재설계 또는 streaming/downscale image decode/output이 필요하다.
- [x] 현재 slow page들을
  `tmp/codex_perf/slowpage_user_profile_20260615_150933`에서 다시 한 페이지씩
  profiling했다. 현재 순위는 `doc_100.pdf` 150 DPI page 4가 계속 1순위
  (`320ms`, `74,449,736` alloc)이고, 그다음은 `doc_070.pdf` 300 DPI page 9
  (`221ms`, `60,908,856`), page 11(`182ms`, `59,547,360`), `doc_100.pdf`
  pages 22/8(`148ms`/`143ms`)이다. Page 4 focused pprof
  `tmp/codex_perf/profile_p4_user_20260615_151025`는 fixed Splash bitmap
  (`11.78MB` alloc-space), parser `Dict.Set` map bucket(`9.50MB`), Flate
  buffer(`6.29MB`), Type1/SFNT glyph/font 작업으로 분산되어 있었다. Inline
  image dictionary key intern probe
  `tmp/codex_perf/inline_key_probe_20260615_151248`는 PNG byte-identical을
  유지했지만 median p4 wall time을 악화시켰고(`322ms -> 366ms`) allocation도
  줄이지 못해 되돌렸다. P9/P11 focused pprof
  `tmp/codex_perf/profile_p9p11_user_20260615_151331`은 in-use heap이 여전히
  fixed 300-DPI page RGB8+alpha bitmap(`~32MiB`, `88-91%`)에 지배되고,
  남은 transient allocation은 실제 Flate/DCT decoded raster buffer임을
  재확인했다. Page 8 pprof `tmp/codex_perf/profile_p8_user_20260615_151556`은
  vector/path와 Type1 glyph 작업 중심이다. 이번 pass에서는 production code
  변경을 유지하지 않았다. 다음 material memory 개선은 page bitmap alpha-plane/
  lifetime 재설계 또는 streaming/downscale image decode/output이 필요하다.
- [x] fresh baseline `tmp/codex_perf/current_user_20260615_152354` 기준으로 현재
  slow leader들을 다시 프로파일링했다. Page-profile 순위는 `doc_100.pdf` 150 DPI
  page 4가 1순위(`321ms`, `70.37MiB` alloc), 그다음이 `doc_070.pdf` 300 DPI
  page 9(`231ms`, `57.81MiB`)와 page 11(`193ms`, `56.92MiB`)이다. Page 4 focused
  pprof `tmp/codex_perf/profile_p4_current_20260615_152550`는 여전히 분산형이다:
  fixed `NewBitmap`(`15.63MB` alloc-space), parser `Dict.Set` map bucket
  (`7.02MB`), Flate/Form `bytes.growSlice`(`5.74MB`), scanner storage,
  Type1/SFNT glyph 작업이 함께 남아 있다. P9/P11 focused pprof
  `tmp/codex_perf/profile_doc070_p9p11_current_20260615_152938`은 fixed 300-DPI
  RGB8+alpha page bitmap(`~32-33MB`)과 실제 ImageMagick/Flate raster buffer가
  지배했다. Rejected probe는 다음과 같다. `Real` singleton cache는
  PNG-identical이었지만 full `doc100_150` allocation을
  `816,485,264 -> 817,608,440`으로 악화시켰다. local `djpeg-go` 기본 decode는
  p9 allocation을 늘렸고 p11 pixel도 변경했다. `YupXdown` direct-buffer pooling은
  pixel-identical이었지만 같은 세션 A/B에서 p9 allocation을 악화시켜 되돌렸다.
  이번 pass에서는 production code 변경을 유지하지 않았다. 남은 material memory
  개선은 exactness-safe page alpha-plane/lifetime 재설계,
  streaming/downscale image decode/output, 또는 decoded-PNG roundtrip을 피하는
  raw compare/output path가 필요하다.
- [x] fresh baseline 이후 slow page를 다시 한 페이지씩 profiling했다. 산출물은
  `tmp/codex_perf/slowpage_p4_now_20260615_1600` 및
  `tmp/codex_perf/slowpage_doc070_now_20260615_1600`이다. Page 4 direct pprof는
  여전히 분산형이다. `MemTotalAlloc 75,245,448`, `MemAlloc 42,355,112`이고,
  `NewBitmap 14.66MB` alloc-space는 page RGB bitmap(`8.07MB`), soft-mask
  scratch(`2.04MB`), cropped group(`1.02MB`), image-scale temp bitmap(`3.53MB`)로
  나뉘며, 그다음은 `Dict.Set 7MB`, Flate/Form `bytes.growSlice 6.18MB`,
  scanner, Type1/SFNT 작업이다. Page 9는 fixed 300-DPI page RGB8+alpha bitmap이
  지배했다. `MemTotalAlloc 62,177,752`, `MemAlloc 53,997,624`, in-use
  `NewBitmap 33,392KB`(`85.01%`)이고 `acquirePageRGBBitmap 32.61MB` 외에 sampled
  group/scale-temp bitmap은 없었다. Page 11도 `MemTotalAlloc 60,965,552`,
  `MemAlloc 55,398,912`, in-use `NewBitmap 32,880KB`(`82.57%`)가 중심이며, 실제
  ImageMagick JPEG stdout raster buffer(`bytes.growSlice 14.92MB`)가 뒤따른다.
  `PDF_DEBUG_SPLASH_OPAQUE_PAGE_ALPHA=1` probe는 p4/p9/p11 모두 pixel을 바꿔,
  page alpha plane을 기본 비활성화하는 것은 exactness-safe하지 않다. 이번
  pass에서는 production code 변경을 유지하지 않았고, generated render PNG는
  디스크 절약을 위해 삭제했으며 pprof/top/log artifact만 남겼다. 남은 material
  memory 타깃은 restartable/lazy exactness-safe page alpha 설계, Splash로 직접
  streaming하는 DCT/downscale decode, 또는 peak RSS를 늘리지 않고 page chunk
  사이에서 fixed page bitmap을 재사용할 수 있는 profile harness 변경이다.
- [x] 현재 p4/p9/p11 leader를 계속 한 페이지씩 profiling하고, profiling/stat
  출력용 retained-heap cleanup을 유지했다. Baseline 산출물은
  `tmp/codex_perf/slowpage_current_pass_20260615_160208`이다. 작은 soft-mask
  matte inline-storage probe는 PNG-identical이었지만
  `tmp/codex_perf/softmask_matte_small_20260615_160623`에서 p4 median
  `MemTotalAlloc`/`MemAlloc`/RSS가 약 `+3.0MB`/`+3.2MB`/`+3.7MB` 악화되어
  rejected 처리했다. 유지한 변경은 `pdfrender --print-mem-stats` 및
  `--memprofile` heap capture 직전에 released Splash/XPath pool을 trim한다.
  `tmp/codex_perf/pool_trim_xpath_verify_20260615_161346`에서 `doc_100.pdf` p4와
  `doc_070.pdf` p9/p11 old/new focused render는 모두 PNG byte-identical이었다.
  5회 median retained `MemAlloc`은 p4 `48,109,152 -> 3,330,672`, p9
  `52,447,648 -> 1,244,944`, p11 `54,131,880 -> 952,176`으로 줄었고 wall time과
  MaxRSS는 noise 수준이었다. 최종 heap profile은
  `tmp/codex_perf/pool_trim_final_heap_20260615_161516`에 있으며 이전 retained
  `NewBitmap`/scanner/scale-buffer pool entry가 사라졌다. p4 retained heap은 이제
  실제 decoded stream/font 초기화 sample이 중심이고, p9/p11은 대부분
  runtime/init 잔여분이다. 관련 `pdfrender`, Splash, XPath, renderer 테스트는
  `CGO_ENABLED=0 PDF_FREETYPE_GO=1`로 통과했다.
- [x] near-exact `GeoTopo.pdf` page 5(`10` bad pixels)를 Exact100 후보로
  재확인했다. Focused trace
  `tmp/codex_exact/geotopo_p5_topcap_current_20260615_145911`에서 첫 cluster는
  FreeType bitmap glyph 83의 top-row coverage였고, ours row는
  `06 2d 46 3a 18`, Poppler는 앞 4개 alpha가 대략 `-1/-3/-3/-1` 낮아야 했다.
  좁은 top-cap bitmap correction probe는 해당 빨간 4픽셀을 고쳤지만
  `(992..995,56)`의 회색 glyph 4픽셀을 새로 만들어, focused 검증
  `tmp/codex_exact/geotopo_p5_topcap_verify_20260615_150454`는 그대로
  `2176704/2176714`(`99.99954059%`, `10` bad pixels)였다. 이 probe는
  bitmap-only context만으로는 안전하지 않아 rejected 처리했다. 잔차는 PDF
  positioning이나 image/color decode가 아니라 FreeType-Go/Splash glyph coverage
  및 compositing-rounding parity 문제로 남았다.
- [x] fresh baseline `tmp/codex_perf/slowpage_fresh_20260615_163424` 기준으로
  현재 느린 페이지를 다시 한 페이지씩 profiling했다. 현재 leader는
  `doc_100.pdf` 150 DPI page 4(`285ms`, `74,611,048` alloc),
  `doc_070.pdf` 300 DPI page 9(`208ms`, `66,343,224` alloc), page 11
  (`161ms`, `68,501,480` alloc)이고 그다음은 `doc_100.pdf` pages 8/22/7이다.
  Focused pprof 산출물은 `tmp/codex_perf/p4_profile_20260615_163635`,
  `tmp/codex_perf/p9_profile_20260615_163929`,
  `tmp/codex_perf/p11_profile_20260615_163722`이다. Page 4는 fixed Splash
  bitmap, parser dictionary map, Flate/Form buffer, Type1/SFNT 작업으로 계속
  분산되어 있고, page 9/11은 fixed 300-DPI page bitmap과 full DCT/Flate raster
  buffer가 지배한다. Embedded font SHA-key cache probe는 output report가
  identical이었지만 broad allocation이
  `tmp/codex_perf/fontkeycache_full_20260615_164614`에서 악화되어 되돌렸다
  (`doc100` `819,379,784 -> 819,494,480`, `doc070`
  `545,322,016 -> 545,464,136`). 유지한 변경은 `pdfcompare`의 row-equality
  fast path이다. Decoded RGBA/NRGBA row가 byte-identical이면 per-pixel RGB
  비교를 건너뛰며, `tmp/codex_perf/compare_rowfast_20260615_165342`에서 report는
  byte-identical을 유지했고 compare-stage wall time은 `doc100`
  `635ms -> 607ms`, `doc070` `926ms -> 875ms`로 줄었다. Allocation은 사실상
  flat이다. 남은 material memory 개선은 exactness-safe page bitmap/alpha
  lifetime 재설계, Splash로 직접 들어가는 streaming/downscale DCT decode, 또는
  decoded-PNG roundtrip을 피하는 raw comparison/output path가 필요하다.
- [x] fresh full-page profile `tmp/codex_perf/doc100_fresh_20260615_171017`
  기준으로 현재 `doc_100.pdf` 150 DPI slow page를 계속 하나씩 profiling했다.
  현재 상위 페이지는 page 4(`338ms`, `74,687,928` alloc), page 8(`158ms`,
  `32,241,808` alloc), page 22(`157ms`, `28,390,320` alloc)이다. Page 4 focused
  산출물은 `tmp/codex_perf/doc100_p4_profile_20260615_171155`이며 no-profile
  baseline은 `MemTotalAlloc 73,893,048`이다. p4는 parser dictionary map,
  Form/Flate decode buffer, Type1/SFNT glyph 작업, scanner/clip storage, fixed
  Splash bitmap으로 분산되어 있었다. cached clip scanner env lookup, raw 5-point
  rect clip fast path, integer-valued `Real` cache probe는 개선이 없거나 악화되어
  rejected 처리했다. Page 8
  `tmp/codex_perf/doc100_p8_profile_20260615_173415`는 glyph raster 중심이다.
  glyph-cache key canonicalization probe는 PNG byte-identical이었지만 glyph miss
  수가 그대로(`1703`)이고 5회 반복 allocation도 flat이라 되돌렸다. Page 22
  `tmp/codex_perf/doc100_p22_profile_20260615_174132`는 fixed `NewBitmap`/page
  bitmap allocation과 실제 Form/image/text 작업이 대부분이며, `canvas.NewCanvas`
  sample은 backend factory path일 뿐 추가 ImageCanvas allocation이 아님을 확인했다.
  이번 pass에서는 production code 변경을 유지하지 않았다. 남은 material 개선은
  page/group bitmap lifetime 및 alpha-plane을 exactness-safe하게 재설계하거나,
  image decode/downscale을 Splash로 streaming하거나, PNG roundtrip 없는 raw
  compare/output path를 만드는 쪽이다.
- [x] 느린 페이지 pass를 이어서 shared `LockableFace` 내부에 FreeType-Go
  pixel-size glyph outline cache를 유지했다. Cached outline은 phase/transform
  변이 전에 항상 clone하므로 glyph pixel은 바꾸지 않으면서 반복 Type1/SFNT glyph
  outline decode를 피한다. Focused old/new render
  `tmp/codex_perf/outlinecache_probe_20260615_175230`에서 `doc_100.pdf`
  pages 4/8/22 및 `doc_070.pdf` pages 9/11은 PNG byte-identical이었다. 5회
  median `MemTotalAlloc`은 doc100 p8 `32,618,312 -> 27,001,832`, p4
  `74,558,544 -> 69,610,936`, p22 `28,537,000 -> 25,864,424`로 줄었고,
  doc070 p11은 `59,488,336 -> 57,924,096`, p9는 사실상 flat/noise
  (`60,420,808 -> 60,304,896`)였다. 같은 조건의 `pdfcompare` report는 전체
  page에서 identical이었다. Full page-profile `ours` allocation은 doc100
  `816,221,712 -> 672,464,408`, wall `2023ms -> 1876ms`, doc070
  `528,232,704 -> 520,237,584`, wall `1373ms -> 1338ms`로 이동했다. 남은
  material memory 개선은 여전히 page/group bitmap alpha-plane lifetime 재설계와
  streaming image decode/downscale 경로다.
- [x] FreeType-Go outline cache pass 이후 slow-page profiling을
  `tmp/codex_perf/slowpage_pagechunk_20260615_180745` page-chunk baseline으로
  이어서 수행했다. 현재 상위 페이지는 `doc_100.pdf` 150 DPI page 4(`324ms`,
  `71,086,912` alloc), `doc_070.pdf` 300 DPI page 9(`261ms`,
  `61,183,296` alloc), page 11(`260ms`, `58,928,104` alloc)이다. 유지한 변경은
  content-stream `Real` object cache이며, no-number-value lexer에서 stream이
  `128KiB` 이상이고 real token이 `1024`개 이상일 때만 활성화하며 unique value는
  `8192`개로 제한한다. Focused p4 render는 PNG byte-identical을 유지했고
  `MemTotalAlloc`은 `72,055,192 -> 69,067,488`로 이동했다
  (`tmp/codex_perf/profile_p4_realcache_stream128k_20260615_183910`). Full
  page-profile 검증
  `tmp/codex_perf/slowpage_realcache_stream128k_full_20260615_183931`은
  `doc100_150`, `doc070_300` 모두 report-identical이었다
  (`report_metric_diffs=0`). p4 page-profile allocation은
  `71,086,912 -> 69,201,384`로 줄었지만, broad 효과는 시간 개선이라기보다
  small/noisy 범위다. doc100 `stage=ours` allocation은
  `709,626,568 -> 709,354,304`, wall은 `2216ms -> 2260ms`, doc070 allocation은
  `532,165,928 -> 532,545,592`, wall은 `1740ms -> 1746ms`였다. p4 CPU는 여전히
  lexer/parser, image draw/sampling, glyph 작업, runtime allocation으로 분산되어
  있고, p9/p11은 고정 300-DPI RGB8+alpha page bitmap과 decoded DCT/Flate raster
  buffer가 지배한다. 다음 material 성능 작업은 streaming/downscale image decode
  또는 Splash bitmap/alpha lifetime 재설계다.
- [x] 현재 slow-page pass를 이어서 `doc_100.pdf` 150 DPI page 4와
  `doc_070.pdf` 150 DPI page 9를 focused pprof로 다시 확인했다. Fresh baseline은
  `tmp/codex_perf/current_doc100_150/out/page_profile.csv` 및
  `tmp/codex_perf/current_doc070_150/out/page_profile.csv`에 있으며, `doc_100`
  page 4가 `295ms` / `69,721,432` alloc / `64,480KB` RSS로 1순위였고,
  `doc_070` pages 9/11은 약 `132ms` / `41.4MB` alloc 수준이었다. Page 4 focused
  profile `tmp/codex_perf/p4_profile_current`는 fixed Splash bitmap,
  dictionary/parser allocation, stream buffer, Type1/SFNT 작업, scanner storage,
  packed RGB matte image drawing으로 계속 분산되어 있었다. Same-size
  image/SMask source-alpha fast path는
  `tmp/codex_perf/p4_profile_after_softmask_fastpath`에서 검증했지만 pixel이
  바뀌고 allocation도 증가했다(`~69.9MB -> ~77.6MB`). 따라서 probe는 되돌렸고,
  `tmp/codex_perf/p4_profile_after_revert_softmask_compat`에서 p4 PNG가 baseline과
  byte-for-byte 동일함을 확인했다. `doc_070` page 9 pure-Go JPEG 경로
  `tmp/codex_perf/doc070_p9_profile_pure`는 `djpeg-go` output raster/color
  pipeline, fixed Splash bitmap, decoded Flate buffer가 구조적 allocation임을
  보여줬다. 유지한 변경은 `go-pdf`가 아니라 `djpeg-go` 쪽 decoder-owned scratch
  buffer release다. Decode finish 후 scratch를 해제해 focused retained in-use
  profile이 `tmp/codex_perf/doc070_p9_profile_after_release` 기준 약 `5.34MB ->
  4.26MB`로 줄었고, alloc-space 및 single-page MaxRSS는 대부분 noise였다. Full
  `doc_070` 150 DPI 검증
  `tmp/codex_perf/current_doc070_150_after_release`도 같은 성능 범위였다
  (`842ms -> 844ms`, allocation `266,545,648 -> 265,819,664`, RSS는 noise).
  관련 `djpeg-go go test ./...`, `go-pdf` Splash 테스트, CLI build, p4 PNG
  compatibility check가 통과했다.
- [x] 현재 worktree 기준으로 느린 페이지를 하나씩 다시 profiling했다. Artifact는
  `tmp/codex_perf/profile_doc100_p4_live_20260615_193327`,
  `tmp/codex_perf/profile_doc100_p4_nodefer_20260615_194215`,
  `tmp/codex_perf/profile_doc070_p9_300_live_20260615_194501`이다.
  `doc_100.pdf` 150 DPI page 4는 image draw가 `26,027`회이며 대부분
  `explicit_nearest_rgb_small_downscale_scale_then_flip`을 쓰는 반복 tiny
  DeviceRGB Flate image(`5x6`, `5x7`, `6x6`, `6x7`)였다. 유지한 변경은
  Splash hot path의 작은 cleanup이다. `drawYdownXdownDirectWithOptions`에서
  per-call `defer` 2개를 제거하고 동일한 pipe/buffer cleanup을 명시적으로 수행하게
  했다. Focused p4 PNG는 baseline과 byte-identical이었고, focused
  `MemTotalAlloc`은 `69,608,416 -> 68,964,216`, MaxRSS는 `67,040KB ->
  66,400KB`로 이동했으며 wall은 약 `0.50s`로 같았다. Full `doc_100` 150 DPI
  검증 `tmp/codex_perf/current_doc100_150_nodefer_20260615_194339`는
  pixel-report-identical이었다(`pixel_report_changes=0`). Page-profile p4
  allocation은 `69,721,432 -> 68,054,864`로 줄었지만 wall은 noise 범위였다
  (`295ms -> 320ms`). Parser dictionary-capacity probe는 `Dict.Set` allocation을
  더 큰 initial map allocation으로 옮기며 focused p4 allocation을 증가시켜
  rejected 처리했다(`69.6MB -> 70.9MB`). `doc_070.pdf` 300 DPI page 9는 여전히
  fixed page bitmap allocation(`NewBitmap 32.11MB`)과 decoded image buffer가
  지배하며, render 후 in-use heap은 거의 남지 않는다. Pure-Go `djpeg-go` JPEG
  실행 `tmp/codex_perf/profile_doc070_p9_300_djpeg_20260615_194618`은 p9 PNG가
  byte-identical이고 wall은 빨라졌지만(`0.40s -> 0.30s`), Go allocation은
  증가했다(`MemTotalAlloc 61,897,808 -> 66,925,872`, alloc-space
  `59.65MB -> 67.61MB`)고 RSS도 약간 높아져 memory 개선이 아닌 opt-in speed
  path로 남긴다. `doc_070.pdf` 300 DPI page 11
  `tmp/codex_perf/profile_doc070_p11_300_live_20260615_195022`도 같은 구조였다.
  `NewBitmap 34.10MB`, `bytes.growSlice 13.75MB`, glyph fill 약 `4MB`,
  render 후 `MemAlloc 2.33MB` 수준이어서 해당 page에는 추가 low-risk memory
  fix를 유지하지 않았다.
- [x] 현재 slow-page set을 page 단위로 다시 baseline했다
  (`tmp/codex_perf/slowpage_rebaseline_20260615_195624`). 현재 리더는 변하지
  않았다. `doc_100.pdf` 150 DPI page 4가 `314ms`, `67,938,400` alloc,
  `64,160KB` RSS이고, 그 다음은 `doc_070.pdf` 300 DPI page 9(`263ms`,
  `60,819,032` alloc, `141,920KB` RSS)와 page 11(`209ms`, `57,856,048` alloc,
  `141,920KB` RSS)이다. Fresh focused profile은
  `tmp/codex_perf/slowpage_p4_profile_20260615_195742`,
  `tmp/codex_perf/slowpage_p9_profile_20260615_200235`,
  `tmp/codex_perf/slowpage_p11_profile_20260615_200330`에 있다. Parser
  collection preallocation probe는
  `tmp/codex_perf/parser_capacity_p4_focus_20260615_200121`에서 검증했지만,
  p4 PNG는 byte-identical인 반면 allocation이 `67.6..68.1MB`에서
  `69.8..70.3MB`로 악화되어 되돌렸다. 남은 p4 비용은 fixed Splash bitmap,
  `Dict.Set`/parser allocation, Flate buffer, Type1/SFNT glyph 작업, image
  drawing으로 분산되어 있다. p9/p11 memory는 구조적이다. Page/group
  `NewBitmap`(`32.11..34.10MB`), ImageMagick JPEG stdout buffer
  (`14.85..16.93MB`), Flate decode buffer, exactness-sensitive image drawing이
  지배한다. 이번 pass에서는 새 production patch를 유지하지 않았다. 다음 material
  memory 작업은 streaming/downscale image decode 또는 Splash bitmap/group
  lifetime 재설계다.
- [x] `slowpage_rebaseline_20260615_195624` 이후 slow page profiling을
  계속했다. 작은 PDF dictionary는 이제 4개 이하 entry를 inline으로 보관하고,
  다섯 번째 distinct key에서 기존 map 표현으로 승격한다. 이로써 parser map
  bucket allocation을 줄이되 slash-insensitive lookup, overwrite, clone, XRef
  dereference 동작은 유지한다. Focused `doc_100.pdf` 150 DPI page 4는
  pixel-identical이었고, `none` PNG 출력은 이전 baseline과 byte-for-byte로
  일치했다. Full `/tmp` 검증
  `/tmp/pdf-reader-codex/inline_dict_profile_20260615_201932`에서 `doc_100`과
  `doc_070` 모두 report-identical이었다(`report_changed=0`). `doc_100`
  `stage=ours` total allocation은 `672,557,296 -> 670,903,072`, page 4는
  `67,938,400 -> 67,023,192`로 이동했고 wall은 noise/flat이었다
  (`1,875ms -> 1,882ms`). `doc_070` total allocation은
  `520,516,936 -> 520,319,808`, page 9는 `60,819,032 -> 60,528,000`, page 11은
  `57,856,048 -> 57,649,760`으로 이동했으며 wall noise는 유리했다. 남은 p9/p11
  `bytes.growSlice` 비용은 ImageMagick stdout raw raster를 담는 필수 buffer라
  local buffer tweak로 없애기 어렵고, streaming/downscale decode 재설계가
  필요하다. 관련 entity/parser/renderer/image/pdfrender/pdfcompare 테스트와
  `git diff --check`가 `CGO_ENABLED=0 PDF_FREETYPE_GO=1`로 통과했다.
- [x] 현재 재현 가능한 slow page를 `/tmp/pdf-reader-codex/perf_profile_20260615_202837`
  기준으로 한 페이지씩 다시 profile했다. 이전 random-corpus의 `doc_070.pdf`와
  `doc_100.pdf`는 현재 repository에 남아 있지 않아, 재현 가능한 GeoTopo 150 DPI
  전체 page-profile을 새 기준으로 삼았다. 현재 리더는 page 31(`123ms`,
  `38,718,728` alloc), page 96(`113ms`, `33,129,368` alloc), page 35(`102ms`,
  `26,976,696` alloc), page 97(`100ms`, `27,505,560` alloc), memory-heavy page
  24(`95ms`, `55,291,736` alloc)였다. Focused p31은 Type1 glyph, path scanner,
  fixed Splash bitmap 비용이 분산되어 있었고, p96은 parser number scan, glyph
  paint, Type1, scanner 비용이 작은 hotspot들로 나뉘어 있었다. p24는 큰
  DeviceRGB Flate image 4개(`1024x1024`, `721x768`, `901x1024`, `1430x1720`)를
  약 187px 폭으로 downscale하는 구조이며, 남은 memory는 full raw image decode
  buffer와 fixed page bitmap이 지배했다. 유지한 output-neutral 변경은 두 가지다.
  freetype-go gray glyph bitmap이 이미 contiguous이면 추가 compact copy 없이 같은
  buffer를 사용하고, final Flate filter에는 slash-prefixed PDF color-space name과
  PNG predictor row byte를 포함한 exact image decoded-size hint를 전달한다.
  Focused p24는 PNG byte-identical을 유지했고 allocation은 `54.9..55.3MB`에서
  `55.0..55.2MB` 수준으로 이동했다. Full GeoTopo 검증은 report-identical이었다
  (`changed=0`, `Exact100 1/117`). 전체 `stage=ours`는 `5,559ms /
  2,910,508,384` alloc / `55,520KB` max RSS에서 `5,406ms /
  2,898,690,624` alloc / `55,200KB` max RSS로 이동했다. 남은 p24의 큰 개선은
  streaming/downscale Flate image decode가 필요하고, p31/p96은 local buffer tweak
  보다 Type1/freetype-go 및 scanner/parser 쪽의 더 깊은 작업이 필요하다.
- [x] `/tmp/pdf-reader-codex/perf_slowpage_20260615_205724`에서 GeoTopo
  slow-page profiling을 이어서 수행했다. 117page 전체 baseline은 `5,543ms /
  2,900,181,488` alloc / `55,040KB` max RSS, `Exact100 1/117`이었고, 느린
  리더는 p31, p96, p35, p97, memory-heavy p24/p25였다. Focused p31/p96은
  scanner AA line rendering과 Type1/freetype-go 작업이 시간 병목임을 확인했다.
  Focused p24/p25는 큰 Flate RGB image와 SMask가 약 188px 폭 출력으로
  downscale되기 전에 raw RGB/gray buffer로 full decode되는 구조가 memory 병목임을
  확인했다. 유지한 변경은 output-neutral cleanup만으로 제한했다. Standard14
  glyph-name reverse map을 `GlyphIDByName` 호출 시점까지 lazy 생성하고, AA row
  buffer 초기화는 `clear`를 사용한다. Full GeoTopo rerun은 metric-identical
  (`Exact100 1/117`, `metric_changed=0`)이었고 `5,480ms / 2,903,949,560` alloc /
  `55,040KB` max RSS로 이동했으므로, material memory fix가 아니라 작은 wall-time
  cleanup으로 취급한다. Rejected probe는 p31 allocation을 늘린 pooled path
  flattening, p24/p25 allocation/RSS를 줄이지 못하거나 악화한 raw RGB/SMask용
  scratch Flate decode, p24 악화와 p25 약 `173KB` 개선에 그친 large SMask cache
  cap이다. 남은 material 작업은 여전히 streaming/downscale Flate image decode와
  scanner/Type1 쪽의 더 깊은 parity/performance 개선이다.

## 실행 체계
- [x] no-CGo coverage package discovery가 generated `tmp/` probe와 local untracked experiment에 흔들리지 않도록 tracked Go source directory 기준으로 package gate를 산출한다.
- [x] PNG predictor 및 raw CMYK unit fixture를 현재 Poppler-aligned 구현 기준으로 갱신해 no-CGo unit coverage가 실행되게 한다.
- [ ] no-CGo core coverage를 현재 `68.3%`에서 `80.0%` gate target까지 올리거나, 더 좁은 gate 정의를 문서화하고 정당화한다.
- [ ] 이미지 매핑 계약을 CTM 적용, 샘플러 phase, 좌표계 변환 기준으로 문서화하고 렌더 경로 전체에서 동일하게 사용한다.
- [ ] 실패 페이지 고정 fixture를 `goal98` 재실행 템플릿에 포함한다.
- [ ] 비교 HTML의 poppler/ours/xor 구조와 `failure_type` 2차 정렬을 유지한다.

## Release Checklist
- [x] GitHub Actions CI no-CGo validation/build workflow를 정리한다.
- [x] GitHub Actions 수동 실행 기반 semver tag bump workflow를 추가한다.
- [x] Tag push 기반 release artifact 빌드 및 GitHub Release 생성 workflow를 추가한다.
- [x] `v0.9.0-poppler24-02-0-202606.1` release tag를 생성한다.
- [x] `v0.9.0-poppler24-02-0-202606.1` GitHub Release를 생성한다.
- [x] Go module `github.com/dh-kam/pdf-go@v0.9.0-poppler24-02-0-202606.1`을 publish한다.
- [x] WebAssembly demo를 GitHub Pages에 publish한다.
