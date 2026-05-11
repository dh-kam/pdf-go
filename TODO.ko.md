# Go PDF Rendering Library - TODO Checklist

## Poppler Splash Exact100 후속 점검
- [ ] `GeoTopo p39` 22px stroke AA overlap 잔차를 Poppler `Splash::makeStrokePath()` 및 `Splash::pipeRunAARGB8()` 경로 기준으로 재검토한다.
- [ ] `GeoTopo p55` pattern/fill/stroke 잔차를 image/soft-mask가 아닌 segment stroke 및 tiling pattern 축으로 재분해한다.
- [ ] `GeoTopo p55`, `p23`, `p44`, `p97`을 함께 보면서 Poppler `strokeNarrow`와 `strokeWide` 분기 조건을 더 좁힌다.

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
