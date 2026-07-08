# PDF 렌더링 성능 최적화 대상

이 문서는 Poppler 정합성을 바꾸지 않으면서 렌더링 wall time과 힙 할당을 줄이기 위한 현재 병목, 폐기한 프로브, 실행 계획을 추적한다.

## 현재 기준선

기준 산출물: `tmp/codex_perf/slowpage_continue_20260614/mmap_verify`

현재 행은 `CGO_ENABLED=0 PDF_FREETYPE_GO=1`, Splash backend, worker 1개 조건의 전체 page-profile 검증에서 가져왔다. 이미지를 보존하지 않는 임시 비교 출력은 `--png-compression none`을 사용할 수 있다.

| 대상 | Canvas | Page-Profile Wall | MemTotalAlloc | MaxRSS | 현재 상태 |
| --- | ---: | ---: | ---: | ---: | --- |
| `doc_100.pdf` page 4 | 150 DPI | `382ms` | `88.60MB` | `70.56MB` | 현재 slow leader. |
| `doc_070.pdf` page 9 | 300 DPI | `282ms` | `74.26MB` | `141.92MB` | 이미지/비트맵 중심 leader. |
| `doc_070.pdf` page 11 | 300 DPI | `217ms` | `63.40MB` | `142.08MB` | 보조 이미지/JPEG 케이스. |

## 최신 프로파일 결과

산출물: `tmp/codex_perf/slowpage_continue_20260614/profile_p4_mmap`

### `doc_100.pdf` Page 4

mmap pass로 입력 PDF 전체 read가 Go heap에서 제거됐다. 현재 heap allocation은 하나의 안전한 hotspot이 아니라 렌더링/파싱 비용으로 분산되어 있다.

| Source | Alloc-Space | 비고 |
| --- | ---: | --- |
| `splash.NewBitmap` | `14.78MB` | 고정 page/group bitmap allocation. |
| `bytes.growSlice` | `8.50MB` | 주로 decoded stream 및 output buffer. |
| `entity.(*Dict).Set` | `8.00MB` | 작은 parsed dictionary와 resource dictionary가 많음. |
| `runtime.allocm` | `6.51MB` | focused profile에서 runtime/thread allocation이 보임. |
| Scanner/numeric operands | `~2..3MB` | clip/intersection storage와 content numeric object가 남아 있음. |
| Raw RGB matte path | `2.04MB` | 이 페이지에서는 canvas image materialization이 아직 보임. |
| `os.readFileContents` | `0.55MB` | 더 이상 PDF 입력 전체가 아니며, 남은 read는 보조 resource다. |

### `doc_070.pdf` Page 9

이 페이지는 구조적으로 image-heavy하다. 최신 pass는 `mmap`으로 per-process 입력 복사를 줄였지만, 주 비용은 여전히 고정 300-DPI bitmap과 image pipeline 작업이다.

| Source | Alloc-Space | 비고 |
| --- | ---: | --- |
| `splash.NewBitmap` | high | 고정 300-DPI page/group bitmap은 구조적으로 남아 있다. |
| Decoded image and stream buffers | high | 더 깊은 streaming decode 전까지 JPEG/Flate decoded data와 scaling buffer가 필요하다. |
| PNG/flate output | medium | 기본 exact output은 여전히 compression-bound가 될 수 있으며, compare-only 임시 출력은 `none`을 사용한다. |
| Parser/scanner storage | medium | content parsing 및 clip/intersection storage가 남아 있다. |
| Whole-file input read | removed | `pdfrender`는 Unix에서 PDF 입력을 `mmap`하고 필요할 때만 `os.ReadFile`로 fallback한다. |

## 폐기한 프로브

| 프로브 | 결과 |
| --- | --- |
| Medium Form XObject Flate hint `rawLen*6 -> rawLen*10` | PNG는 동일했지만 p4 allocation/RSS가 개선되지 않았고 live heap 노이즈가 커져 되돌림. |
| `Real(-1/0/1)` singleton cache | PNG는 동일했지만 p4 allocation이 flat/slightly worse였고 p9/p11은 noise 수준이라 되돌림. |
| 더 넓은 `Real` cache, slice-backed small dictionary, parser dict capacity 변경 | focused 또는 broad run에서 flat, mixed, regressive로 이미 폐기. |
| input mmap 없이 keyword interning만 적용 | 정확하고 유지한 변경이지만, 단독으로는 allocation 개선이 작고 wall/RSS 변화가 noisy했다. |

## 병목 백로그

| 우선순위 | 영역 | 실행 계획 | 담당 역할 | 검증 게이트 |
| --- | --- | --- | --- | --- |
| P0 | Streaming/downscale image decode | destination scale과 clip 정보를 image decode 단계로 전달해 큰 DCT/Flate 이미지를 필요한 row/pixel만 decode 또는 emit한다. 현재 exact path는 fallback으로 유지한다. | Renderer + image pipeline engineer | p9/p11 PNG byte-identical 이후 전체 `doc070_300` report-identical. |
| P0 | Splash bitmap lifetime reuse | ownership이 명확한 page/group bitmap storage를 재사용한다. 특히 transparency group, soft-mask/group surface가 대상이다. retained heap을 키우는 page output pooling은 제외한다. | Splash engineer | focused RSS가 회귀하지 않아야 하며 full page profile report row가 동일해야 한다. |
| P1 | Raw output comparison path | `-keep-images=false`에서 `pdfcompare`가 PNG roundtrip 없이 raw RGB/RGBA row를 직접 비교하고, PNG는 artifact용으로만 남긴다. | CLI/tooling engineer | `doc100_150`, `doc070_300`에서 PNG compare와 report metric이 동일해야 한다. |
| P1 | Scanner storage reduction | 현재 int32 storage 이후에도 남는 scanner intersection/count buffer를 압축 또는 재사용한다. wide clip과 AA row가 많은 페이지를 대상으로 한다. | Rasterization engineer | focused PNG 동일, broad GeoTopo/doc100 profile에서 wall 회귀 없이 allocation 개선. |
| P2 | Parser/dictionary allocation model | generic small-dict 대체가 아니라 key set이 알려진 hot parsed operator dictionary에만 compact form을 적용한다. | Parser/core engineer | p4에서 현재 map path보다 빨라야 하며 broad random corpus에서 flat 또는 개선이어야 한다. |
| P2 | Persistent input workers | `pdfrender`는 Unix `mmap`으로 Go-heap 입력 복사를 피한다. 남은 per-page chunk 비용은 process startup, parsing, resource setup이므로 다음 단계는 persistent worker 또는 shared parsed state다. | CLI/runtime engineer | compare output 동일, chunk-size 1 page-profile에서 wall/alloc 개선. |

## 월간 리포트 템플릿

```markdown
# 렌더링 성능 리포트: YYYY-MM

## 요약
- 기준 산출물:
- 신규 산출물:
- 비교한 전체 페이지:
- 정확도/report 변화:
- 전체 wall-time 변화:
- 전체 allocation 변화:
- MaxRSS 변화:

## 상위 Slow Page
| Rank | PDF | Page | DPI | Wall | MemTotalAlloc | MaxRSS | 주 병목 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |

## 유지한 변경
| 변경 | Focused 영향 | Broad 영향 | Parity 근거 |
| --- | --- | --- | --- |

## 폐기한 프로브
| 프로브 | 폐기 사유 | 산출물 |
| --- | --- | --- |

## 다음 달 계획
| 우선순위 | 대상 | 담당 | 검증 게이트 |
| --- | --- | --- | --- |
```
