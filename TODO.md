# Go PDF Rendering Library - TODO Checklist

Korean localization: [TODO.ko.md](TODO.ko.md).

## Poppler Splash Exact100 Follow-up

- [ ] Revisit the `GeoTopo p39` 22px stroke AA overlap residual against Poppler `Splash::makeStrokePath()` and `Splash::pipeRunAARGB8()`.
- [ ] Reclassify the `GeoTopo p55` pattern, fill, and stroke residuals as segment stroke and tiling-pattern issues rather than image or soft-mask issues.
- [ ] Review `GeoTopo p55`, `p23`, `p44`, and `p97` together to narrow the Poppler `strokeNarrow` and `strokeWide` branch conditions.

## Pure Go Porting Plan

- [x] Treat historical `nojpx,nojbig2` exact100 as corpus coverage only, not JPX/JBIG2 port completion.
- [x] Replace the CGo inventory gate with a zero-`import "C"` assertion.
- [x] Wire PDF `DecodeParms` / `JBIG2Globals` into JBIG2 decode options and keep full-corpus exact100.
- [x] Port the JBIG2 MQ arithmetic decoder and arithmetic generic bitmap region templates 0-3.
- [x] Match Poppler JBIG2 arithmetic generic-region typical prediction (`TPGDON`) row handling.
- [x] Match Poppler JBIG2 external region combination operators and compose multiple immediate generic regions in segment order.
- [x] Match Poppler JBIG2 page default-pixel initialization from Page Information flags.
- [x] Port JBIG2 pattern dictionary and halftone skip/multi-plane arithmetic gray-image composition.
- [x] Port Poppler-aligned JBIG2 generic refinement region parsing, contexts, TPGR handling, and shared arithmetic refinement decoding.
- [x] Add Poppler-aligned JBIG2 arithmetic integer/IAID decoding plus symbol dictionary and text region header parsing/storage scaffolding.
- [x] Port Poppler-aligned JBIG2 arithmetic symbol dictionary refinement/aggregate bitmap decoding and export flow.
- [x] Port JBIG2 multi-plane MMR halftone gray-image decoding and intermediate halftone region storage.
- [x] Port Poppler-aligned JBIG2 default Huffman text region placement and refinement switching.
- [x] Port Poppler-aligned JBIG2 default Huffman symbol dictionary collective bitmap, refinement/aggregate, and export flow.
- [x] Port JBIG2 custom Huffman code-table segment parsing and referenced-table dispatch for symbol/text regions.
- [x] Decode the first supported text, halftone, refinement, or generic bitmap segment in the no-PageInfo fallback path after collecting dictionaries and code tables.
- [x] Match Poppler `JBIG2Globals` priority when global and page streams reuse a segment number.
- [x] Match Poppler generic-refinement bitmap lifetime by discarding the referenced intermediate bitmap after refinement decoding.
- [ ] Finish remaining JBIG2 native decoder edge cases around malformed/interleaved `JBIG2Globals` references.
- [x] Add a real pure Go JPEG2000 decoder path for JP2 boxes and raw codestreams.
- [x] Add JPX unit coverage for both JP2 container and raw J2K codestream decode paths.
- [ ] Validate JPX exact parity against Poppler/OpenJPEG coverage and close unsupported JPEG2000 feature gaps.
- [x] Add a pure Go SFNT fallback for FreeType glyph lookup, glyph names, outlines, bounding boxes, transforms, and approximate bitmap rasterization.
- [x] Match Poppler-style ppem/matrix normalization for non-Type1 pure-Go SFNT bitmap glyphs.
- [x] Keep Type1 no-CGo glyph rasterization separately gated so it can be disabled for isolation, while defaulting it on under `PDF_FREETYPE_GO=1` because it improves Type1-heavy parity.
- [x] Update `freetype-go` to preserve Type1/CFF cubic outlines and raster cubic tags, then bump the git dependency without a local replace.
- [x] Gate FreeType fill-rule coverage for Type1/CFF `freetype-go` raster paths and bump the git dependency after full-corpus parity validation.
- [x] Match FreeType bitmap placement for top-zero pure-Go glyph bitmaps without flipping manually created raster test surfaces.
- [x] Match FreeType CFF design-unit outline scaling so pure-Go CFF glyph bitmaps align with FreeType rounding.
- [x] Match FreeType TrueType phantom-origin outline translation so pure-Go glyph bitmaps align with Poppler on `pdfkit.pdf`.
- [x] Match FreeType TrueType high-precision design-unit scaling so pure-Go glyph bitmaps align with Poppler on `libreoffice-form.pdf`.
- [x] Remove the pure-Go rotated TrueType Y-phase hotfix after matching Poppler/FreeType matrix phase behavior on `habibi-rotated.pdf`.
- [x] Match FreeType Type1 size-metric scaling so pure-Go Type1 glyph outlines follow `FT_Set_Pixel_Sizes` / `FT_Request_Metrics`.
- [x] Trust embedded subset Type1 fonts without ASCII renderability probes so one-glyph subsets do not fall back to standard fonts.
- [x] Replace FreeType-dependent glyph APIs with pure Go glyph lookup, bbox, outline, transform, bitmap, phase, and matrix raster paths.
- [x] Replace Cairo glyph mask rasterization with a pure Go raster strategy or remove the Cairo path when Splash exact parity is preserved.
- [x] Remove FreeType, JPX, JBIG2, and Cairo CGo wrappers from the source tree.
- [x] Remove feature-disabling build tags from e2e child `go run` / `go build` commands.
- [x] Add a no-CGo release gate that builds and tests with `CGO_ENABLED=0`.
- [x] Force Makefile no-CGo validation targets to run with `CGO_ENABLED=0` and keep race checks as a separate CGo-required gate.
- [x] Keep full-corpus Poppler exact100 HTML generation available with `-timeout-sec 0` for long-running verification.

## FreeType-Go Upstream Workflow

- [ ] For every confirmed FreeType parity gap, reproduce it in `freetype-go` with a minimal fixture and record the exact API inputs, pure Go function, and observed delta.
- [ ] Fix confirmed `freetype-go` defects in `/workspace/freetype-go` first, verify them there, then update `pdf-go` to consume the upstreamable behavior.
- [ ] Open a GitHub issue in `dh-kam/freetype-go` for each confirmed gap with pure Go mismatch, fixture/page evidence, and proposed fix; add implementation notes as issue comments when needed.
- [ ] Run `git fetch origin` before `freetype-go` work and only `git pull --rebase` when the local worktree is clean or local changes are safely committed/stashed.
- [ ] Upstream the raw CFF Type1C Encoding charmap and FontBBox parity gap found while matching `GeoTopo-komprimiert.pdf` no-CGo Splash exact100.

## Rendering Accuracy

- [ ] Merge the `pdf.js` fixture corpus with the existing sample fixtures and iterate on the highest render-mismatch documents until they reach 98%+.
- [ ] Block the root causes behind `decode_or_transform` and `resample_or_antialias` bottlenecks found in failed-document rechecks.
- [ ] Reanalyze `tmp/sample_compare` and `tmp/sample_compare_faildocs_recheck_only` results using `tmp/goal98_batch.go` as the pass-rate baseline.
- [ ] Reprioritize bottlenecks and update the next work units through `scripts/render_bottleneck_backlog.sh`.
- [ ] Revalidate resampling policy and phase behavior for `007-imagemagick-images`, `019-grayscale-image`, and `023-cmyk-image`.

## P0 Accuracy Work

- [ ] Summarize common causes for `decode_or_transform` pages in the `goal98` failure set and add regression tests.
- [ ] Recheck sampler, antialiasing, and coordinate-phase rules for `resample_or_antialias` failures.
- [ ] Break down `layout_or_transform`, `color_or_colorspace`, and `minor_pixel_mismatch` by per-page pixel maps.
- [ ] Prevent `ImageCanvas` placement from reducing rotated, skewed, and sheared CTMs to axis-aligned scale-only transforms.
- [ ] Fix `DrawImageWithPhase` in `internal/infrastructure/canvas/image_canvas.go` and strengthen rotated/skewed image parity tests.
- [ ] Improve CTM forwarding and sampler-policy reason values in `internal/domain/renderer/evaluator.go`.
- [ ] Rerun a single batch against `tmp/goal98_rerun_affine_fix` and review `report.csv` plus the HTML comparison output.
- [ ] Update `tmp/goal98_rerun_affine_fix/bottleneck_backlog.md` if failure classes remain.

## P1 Performance Work

- [ ] Apply cache, buffer reuse, and branch-reduction candidates to `vector.rasterizeDstRGBASrcUniformOpOver` and `draw.ablInterpolator` hotspots.
- [ ] Regenerate profiles after performance changes and record hotspot-ratio deltas for the same comparison set.
- [ ] Document a portfolio-ready bottleneck backlog with role-based execution plans and a monthly report template.

## Execution System

- [ ] Document the image-mapping contract for CTM application, sampler phase, and coordinate conversion, then reuse it across all render paths.
- [ ] Include fixed failing-page fixtures in the `goal98` rerun template.
- [ ] Preserve the comparison HTML layout with Poppler, ours, XOR, and secondary sorting by `failure_type`.

## Release Checklist

- [x] Consolidate GitHub Actions no-CGo validation and build workflow.
- [x] Add the manually triggered release-train tag bump workflow.
- [x] Add tag-push release artifact build and GitHub Release workflow.
- [ ] Tag release.
- [ ] Create GitHub release.
- [ ] Publish Go module.
