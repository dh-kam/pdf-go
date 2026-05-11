# Go PDF Rendering Library - TODO Checklist

Korean localization: [TODO.ko.md](TODO.ko.md).

## Poppler Splash Exact100 Follow-up

- [ ] Revisit the `GeoTopo p39` 22px stroke AA overlap residual against Poppler `Splash::makeStrokePath()` and `Splash::pipeRunAARGB8()`.
- [ ] Reclassify the `GeoTopo p55` pattern, fill, and stroke residuals as segment stroke and tiling-pattern issues rather than image or soft-mask issues.
- [ ] Review `GeoTopo p55`, `p23`, `p44`, and `p97` together to narrow the Poppler `strokeNarrow` and `strokeWide` branch conditions.

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
