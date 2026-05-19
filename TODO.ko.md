# Go PDF Rendering Library - TODO Checklist

## Poppler Splash Exact100 후속 점검
- [ ] `GeoTopo p39` 22px stroke AA overlap 잔차를 Poppler `Splash::makeStrokePath()` 및 `Splash::pipeRunAARGB8()` 경로 기준으로 재검토한다.
- [ ] `GeoTopo p55` pattern/fill/stroke 잔차를 image/soft-mask가 아닌 segment stroke 및 tiling pattern 축으로 재분해한다.
- [ ] `GeoTopo p55`, `p23`, `p44`, `p97`을 함께 보면서 Poppler `strokeNarrow`와 `strokeWide` 분기 조건을 더 좁힌다.

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

## 렌더링 정확도 개선
- [ ] `pdf.js` fixture와 기존 sample fixture를 합쳐 render mismatch 상위 문서부터 98%+까지 반복 개선한다.
- [ ] 실패 문서 재비교 결과에서 `decode_or_transform`, `resample_or_antialias` 병목 근본 원인을 차단한다.
- [ ] `tmp/goal98_batch.go` 결과 기준 99% pass 보장을 목표로 `tmp/sample_compare` 및 `tmp/sample_compare_faildocs_recheck_only` 결과를 재분석한다.
- [ ] 병목 우선순위를 재정의하고 `scripts/render_bottleneck_backlog.sh`를 통해 다음 작업 단위를 반영한다.
- [ ] `007-imagemagick-images`, `019-grayscale-image`, `023-cmyk-image` 샘플 비교 포인트의 리샘플링 정책과 phase를 재검증한다.

## P0 정확도 작업
- [ ] `goal98` 실패 페이지 중 `decode_or_transform` 항목의 공통 원인을 정리하고 회귀 테스트를 추가한다.
- [ ] `resample_or_antialias` 항목에 대해 리샘플러, 안티에일리어싱, 좌표 phase 규칙을 재점검한다.
- [ ] `layout_or_transform`, `color_or_colorspace`, `minor_pixel_mismatch` 항목의 원인을 페이지 단위 픽셀 맵으로 분해한다.
- [ ] `ImageCanvas` 이미지 배치가 회전, 기울기, 전단 CTM에서 축 정렬 스케일 위주 변환으로 축약되지 않도록 개선한다.
- [ ] `internal/infrastructure/canvas/image_canvas.go`의 `DrawImageWithPhase`를 수정하고 회전/기울기 이미지 정합 테스트를 보강한다.
- [ ] `internal/domain/renderer/evaluator.go`의 `renderImageToCanvas` CTM 전달 방식과 샘플러 정책 reason 값을 보강한다.
- [ ] `tmp/goal98_rerun_affine_fix` 기준으로 단일 배치 재실행 후 `report.csv`와 HTML 비교 결과를 재확인한다.
- [ ] 실패 유형이 남으면 `tmp/goal98_rerun_affine_fix/bottleneck_backlog.md`를 갱신한다.

## P1 성능 작업
- [ ] `vector.rasterizeDstRGBASrcUniformOpOver` 및 `draw.ablInterpolator` hotspot에 캐시, 버퍼 재사용, 분기 최소화 후보를 적용한다.
- [ ] 성능 변경 후 동일 비교군으로 프로파일을 재생성하고 hotspot 비중 변화를 기록한다.
- [ ] 포트폴리오용 병목 백로그를 역할별 수행 계획과 월보 템플릿으로 문서화한다.

## 실행 체계
- [ ] 이미지 매핑 계약을 CTM 적용, 샘플러 phase, 좌표계 변환 기준으로 문서화하고 렌더 경로 전체에서 동일하게 사용한다.
- [ ] 실패 페이지 고정 fixture를 `goal98` 재실행 템플릿에 포함한다.
- [ ] 비교 HTML의 poppler/ours/xor 구조와 `failure_type` 2차 정렬을 유지한다.

## Release Checklist
- [x] GitHub Actions CI no-CGo validation/build workflow를 정리한다.
- [x] GitHub Actions 수동 실행 기반 semver tag bump workflow를 추가한다.
- [x] Tag push 기반 release artifact 빌드 및 GitHub Release 생성 workflow를 추가한다.
- [ ] Tag release.
- [ ] Create GitHub release.
- [ ] Publish Go module.
