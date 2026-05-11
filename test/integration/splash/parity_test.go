// Phase 3+4 splash parity gate (Phase 2 fill gate, expanded to track Phase 3
// fixture promotions, then re-evaluated post Phase 4 wiring of text, blend
// modes, transparency groups, soft masks, and tiling cell sub-render).
// (see /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md
// §1.3 + 04_phase_plan.md Phase 3/4 exit criteria.)
//
// This file is owned by P3-QA1, re-evaluated by P4-QA1 (2026-04-27). It
// complements (does NOT replace) Q2's existing TestSplashGoldenCorpus harness
// in golden_corpus_test.go: that test validates determinism + manifest drift;
// this file adds the actual pixel-byte parity gate.
//
// Re-evaluation log — Phase 4 close (P4-QA1 2026-04-27):
//
//	30_dash_zero_phase  — XFAIL → REQUIRED. Now pixel-equal post Phase 4
//	                      stroke pipeline tightening (was already
//	                      "ACCIDENTALLY pixel-equal" under Phase 3).
//	09_round_cap_round_join — kept XFAIL. ~144 byte diff (round-geometry
//	                      polish still deferred per Phase 2 backlog).
//	11_axial_grad_h     — kept XFAIL (~1131 byte diff). Phase 4 didn't
//	                      touch shading; promote when shading parity work
//	                      lands.
//	15/16/17 glyph      — kept XFAIL (1k–5k byte diffs). P4-Dev1 wired
//	                      DrawText through Splash.fillGlyph BUT the glyph
//	                      bitmap still drifts vs poppler at sub-px (TTF
//	                      hinter / phase rounding mismatch). Property test
//	                      TestTextDrawsGlyphs (p4_property_test.go) pins
//	                      that glyphs *render*; pixel parity is post-Phase-4.
//	20–23 image         — kept XFAIL. Phase 3 Dev4 wired image routing,
//	                      Phase 4 didn't revisit it; large diffs (~13k–50k
//	                      bytes) suggest sampler/colorspace gaps unrelated
//	                      to the Phase 4 deliverables.
//	24_clip_intersect   — kept XFAIL (~67k byte diff). Splash's Clip path
//	                      is still gated on errNotImplemented at the
//	                      splashCanvas adapter (backend.go ClipToPath
//	                      stub). Phase 4 did not pick this up.
//	25_link_border      — kept XFAIL (~15k byte diff). Annotation border
//	                      not threaded into splashCanvas (legacy backend
//	                      still owns annotations); not in P4 scope.
//	18/19 softmask      — kept XFAIL. PDFs are MISSING from
//	                      test/testdata/splash_golden/pdfs/ (Q1 still has them
//	                      as # TODO in MANIFEST.tsv); P4-QA1 added unit-
//	                      level coverage (TestSoftMaskAttenuates +
//	                      TestSoftMaskCleared) for the splash-side
//	                      composite path. Promotion to REQUIRED needs
//	                      gen_splash_fixtures.py to grow Form XObject
//	                      /Group /Luminosity + /Alpha builders — punted
//	                      to a Q1 follow-up so this gate stays stable.
package splashintegration

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// phase2RequiredTargets is the union of fixtures expected to land at
// byte-equal parity with pdftoppm 24.02.0 through Phase 3. The Phase 2
// 9-fixture set (rect fill / stroke / dash) is the carry-over base; the
// Q1-TODO Phase 3 entries (#12, #13, #14) are listed REQUIRED so that
// once Q1 ships their PDF + reference PNG the gate picks them up
// automatically — until then the loop SKIPs them with a clear "manifest
// entry missing" note (see TestSplashStrokePrimitiveParity).
//
// Phase 3 PASS-promoted fixtures (those whose splash-backend output
// already matches pdftoppm post-decode) are added inline below. Phase 3
// fixtures whose pdfrender-side wiring is still INCOMPLETE (the
// splashCanvas Adapter currently stubs DrawShadingPattern /
// DrawTilingPattern as no-ops; see backend.go:320,327) live in
// phase3IntegrationGap and remain in phase2XFAIL until Dev wires the
// adapter through.
//
// Variable name retained from Phase 2 so CI selectors / grep logs stay
// stable; the "phase2" prefix is now historical, not semantic.
var phase2RequiredTargets = map[string]bool{
	// Phase 2 set (carry-over).
	"01_rect_solid_aligned": true, // rect fill (axis-aligned)
	"02_rect_solid_subpx":   true, // rect fill (sub-pixel origin)
	"03_rect_eo_hole":       true, // even-odd fill
	"04_aa_diag_edge":       true, // AA diagonal edge fill
	"05_thin_hline_1px":     true, // strokeNarrow hline
	"06_thin_vline_1px":     true, // strokeNarrow vline
	"07_miter_join_45":      true, // miter join
	"08_bevel_join_45":      true, // bevel join
	"10_dash_pattern":       true, // dash pattern

	// Phase 3 promotions — Q1-TODO (manifest entry missing; SKIP cleanly).
	"12_axial_grad_diag": true, // axial shading 45°  — Q1-TODO PDF
	"13_radial_grad":     true, // radial shading     — Q1-TODO PDF
	"14_tiling_pattern":  true, // tiling fanout      — Q1-TODO PDF

	// Phase 4 promotion (P4-QA1 2026-04-27): under Phase 4 wiring this
	// fixture renders pixel-equal to pdftoppm — the prior "ACCIDENTALLY
	// pixel-equal" XFAIL log line in p3 confirmed the parity. Promoting so
	// the gate locks the regression in.
	"30_dash_zero_phase": true, // dash phase=0 edge case
}

// phase3IntegrationGap captures Phase 3 fixtures whose splash-side unit
// tests pass (per memory checkpoint_type1_render etc.) but whose
// pdfrender integration is currently incomplete: splashCanvas's
// DrawShadingPattern / DrawTilingPattern stubs return nil without
// invoking the new Splash.FillAxialShading / FillWithTilingPattern
// methods, so PDF rendering through `--backend=splash` falls back to
// blank/partial output for these primitives.
//
// Per spec rule "no test fails for a 'not yet supported' reason — those
// should be SKIP with clear note", these stay in XFAIL until Dev wires
// the adapter calls. Once adapter wiring lands, move each entry to
// phase2RequiredTargets and re-run the gate.
//
// Captured 2026-04-27 by P3-QA1 against backend.go:320,327 stubs.
var phase3IntegrationGap = map[string]bool{
	"11_axial_grad_h":        true, // FillAxialShading not invoked from splashCanvas.DrawShadingPattern
	"15_glyph_mono":          true, // glyph blit through canvas DrawText path lossy at sub-px
	"16_glyph_aa_subpx":      true, // glyph blit through canvas DrawText path lossy at sub-px
	"17_glyph_blend_lsb":     true, // glyph blit through canvas DrawText path lossy at sub-px
	"20_image_bilinear_up":   true, // canvas.DrawImage path doesn't route through Splash.DrawImage
	"21_image_bilinear_down": true, // canvas.DrawImage path doesn't route through Splash.DrawImage
	"22_image_indexed":       true, // canvas.DrawImage path doesn't route through Splash.DrawImage
	"23_image_cmyk":          true, // canvas.DrawImage path doesn't route through Splash.DrawImage
	"24_clip_intersect":      true, // splashCanvas.Clip stub returns errNotImplemented
	"25_link_border":         true, // annotation border not threaded into splashCanvas
}

// phase2XFAIL is the residual set of fixtures we explicitly do NOT expect
// to be byte-equal. After the Phase 4 re-evaluation the list is:
//   - 09 round_cap_round_join : round-geometry polish deferred (Phase 2 backlog)
//   - phase3IntegrationGap     : Phase 3 splash-side wired; pixel parity still
//     drifts post Phase 4 (text/image/clip/annotation gaps are not closed yet,
//     re-check during Phase 5).
//   - 18 softmask_lum          : PDF + reference PNG MISSING (Q1 # TODO).
//     Splash-side composite path is unit-tested via TestSoftMaskAttenuates in
//     p4_property_test.go; promote to REQUIRED when Q1 ships the fixture.
//   - 19 softmask_alpha        : same situation as 18.
//
// Note: 30_dash_zero_phase moved from XFAIL to REQUIRED in Phase 4 — see
// phase2RequiredTargets above.
//
// Variable name retained from Phase 2 so CI selectors / grep logs stay stable.
var phase2XFAIL = map[string]bool{
	"09_round_cap_round_join": true, // round cap/join geometry deferred
	"18_softmask_lum":         true, // soft-mask /Group /Luminosity — Q1 fixture pending
	"19_softmask_alpha":       true, // soft-mask /Group /Alpha — Q1 fixture pending
}

func init() {
	for k := range phase3IntegrationGap {
		phase2XFAIL[k] = true
	}
}

// TestSplashStrokePrimitiveParity asserts that splash backend output is
// pixel-equal post-decode against pdftoppm 24.02.0 reference for 9 Phase 2
// primitives (rect fill + stroke variants). Per R-B 2026-04-27, the gate is
// pixel-level (image.RGBA.Pix byte-equal) NOT raw PNG SHA, because Go's
// compress/flate produces different DEFLATE bitstream than libpng's zlib at
// the same level (encoder-only, not rendering correctness). Pixel-equal is
// the correct invariant: it captures everything the Splash rasterizer is
// responsible for and ignores what the PNG encoder happens to compress to.
//
// See /workspace/pdf-reader/tmp/splash_port_design/00_DESIGN.md §11 (R-B).
//
// Naming carried over from Phase 1 to keep CI selectors stable.
func TestSplashStrokePrimitiveParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping splash stroke parity in -short mode")
	}

	root := parityRepoRoot(t)
	goldenRoot := filepath.Join(root, "test", "testdata", "splash_golden")
	manifestPath := filepath.Join(goldenRoot, "MANIFEST.tsv")
	expectedDir := filepath.Join(goldenRoot, "expected")
	pdfsDir := filepath.Join(goldenRoot, "pdfs")

	if _, err := os.Stat(manifestPath); err != nil {
		t.Skipf("Q1 fixtures not present (%s missing): %v", manifestPath, err)
	}

	// Phase 2 seam: pdfrender must accept --backend=splash. If the binary
	// predates W1's wiring (or hasn't been rebuilt locally) the gate skips
	// cleanly — this is a CI/dev-machine state, NOT a Phase 2 failure.
	if !pdfrenderHasBackendFlag(t, root) {
		t.Skipf("pdfrender does not support --backend flag — " +
			"build with `go build -o bin/pdfrender ./cmd/pdfrender` after " +
			"W1's --backend=splash wiring landed. Once `pdfrender --help` " +
			"advertises --backend with the splash choice this gate becomes active.")
	}

	manifest := loadFixtureManifest(t, manifestPath)

	// Track which REQUIRED fixtures we actually visited; any REQUIRED name that
	// is not iterated below (e.g. Q1 still has it as TODO with no PDF/PNG yet,
	// like #12 axial_grad_diag, #13 radial_grad, #14 tiling_pattern) gets a
	// per-fixture SKIP subtest at the end so the missing-reference state is
	// visible in test output rather than silently absent.
	visited := make(map[string]bool, len(phase2RequiredTargets))

	for _, entry := range manifest {
		entry := entry
		// Only iterate over the PDF rows; expected PNG rows are looked up
		// by name and TODOs are obviously skipped.
		if entry.IsTODO {
			continue
		}
		if filepath.Ext(entry.RelPath) != ".pdf" {
			continue
		}
		visited[entry.Name] = true
		t.Run(entry.Name, func(t *testing.T) {
			pdfPath := filepath.Join(pdfsDir, filepath.Base(entry.RelPath))
			expectedPath := filepath.Join(expectedDir, entry.Name+".png")

			if _, err := os.Stat(pdfPath); err != nil {
				t.Skipf("fixture pdf missing: %v", err)
			}
			if _, err := os.Stat(expectedPath); err != nil {
				// Per Phase 3 promotion rule: REQUIRED but missing reference
				// PNG is a clean SKIP, not a fail. Q1 owns reference PNG
				// regeneration via scripts/regen_splash_golden.sh.
				t.Skipf("expected png missing — run scripts/regen_splash_golden.sh: %v", err)
			}

			producedBytes, err := renderPDFViaSplash(t, pdfPath)
			if err != nil {
				if phase2RequiredTargets[entry.Name] {
					t.Fatalf("render via splash backend: %v", err)
				}
				t.Logf("XFAIL %s (primitive=%s) — render error: %v",
					entry.Name, entry.Primitive, err)
				return
			}

			expectedBytes, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read reference: %v", err)
			}

			equal, mismatch, err := comparePNGsPixelEqual(producedBytes, expectedBytes)

			switch {
			case phase2RequiredTargets[entry.Name]:
				if err != nil {
					t.Errorf("FAIL %s (primitive=%s): pixel-decode error: %v",
						entry.Name, entry.Primitive, err)
					saveDiffArtifacts(t, entry.Name, producedBytes, expectedBytes)
					return
				}
				if !equal {
					if mismatch < 0 {
						t.Errorf("FAIL %s (primitive=%s): PNG dimension mismatch",
							entry.Name, entry.Primitive)
					} else {
						ref, decErr := png.Decode(bytes.NewReader(expectedBytes))
						if decErr != nil {
							t.Errorf("FAIL %s (primitive=%s): %d pixel bytes differ "+
								"(reference re-decode failed: %v)",
								entry.Name, entry.Primitive, mismatch, decErr)
						} else {
							b := ref.Bounds()
							total := b.Dx() * b.Dy() * 4 // 4 bytes per pixel (R,G,B,A)
							pctIdentical := 100.0 * float64(total-mismatch) / float64(total)
							t.Errorf("FAIL %s (primitive=%s): %d/%d pixel bytes differ (%.4f%% identical)",
								entry.Name, entry.Primitive, mismatch, total, pctIdentical)
						}
					}
					saveDiffArtifacts(t, entry.Name, producedBytes, expectedBytes)
				} else {
					t.Logf("PASS %s (primitive=%s): pixel-equal (post-decode)",
						entry.Name, entry.Primitive)
				}
			case phase2XFAIL[entry.Name]:
				if err != nil {
					t.Logf("XFAIL %s (primitive=%s) — pixel-decode error: %v",
						entry.Name, entry.Primitive, err)
				} else if equal {
					t.Logf("OK (unexpected pass — consider promoting) %s "+
						"(primitive=%s) ACCIDENTALLY pixel-equal to reference",
						entry.Name, entry.Primitive)
				} else {
					t.Logf("XFAIL %s (primitive=%s) — %d pixel bytes differ",
						entry.Name, entry.Primitive, mismatch)
				}
			default:
				t.Logf("fixture %s not classified (primitive=%s); equal=%v err=%v",
					entry.Name, entry.Primitive, equal, err)
			}
		})
	}

	// Surface REQUIRED fixtures that the manifest hasn't shipped yet (still
	// in Q1's TODO list — e.g. axial-diag #12, radial #13, tiling #14). One
	// SKIP subtest per missing fixture so the state is visible in CI output.
	for name := range phase2RequiredTargets {
		if visited[name] {
			continue
		}
		name := name
		t.Run(name, func(t *testing.T) {
			t.Skipf("REQUIRED fixture %q has no MANIFEST entry yet — "+
				"Q1-TODO PDF + reference PNG pending. "+
				"Generate via scripts/gen_splash_fixtures.py + scripts/regen_splash_golden.sh.",
				name)
		})
	}
}

// TestSplashFixtureManifestIntegrity verifies that every non-TODO entry in
// MANIFEST.tsv still matches its recorded sha256 (see
// /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md §1.3).
func TestSplashFixtureManifestIntegrity(t *testing.T) {
	root := parityRepoRoot(t)
	goldenRoot := filepath.Join(root, "test", "testdata", "splash_golden")
	manifestPath := filepath.Join(goldenRoot, "MANIFEST.tsv")

	if _, err := os.Stat(manifestPath); err != nil {
		t.Skipf("Q1 fixtures not present (%s missing): %v", manifestPath, err)
	}

	manifest := loadFixtureManifest(t, manifestPath)
	if len(manifest) == 0 {
		t.Fatalf("manifest %s parsed empty", manifestPath)
	}

	checked := 0
	for _, entry := range manifest {
		if entry.IsTODO {
			continue
		}
		if entry.Sha256 == "" {
			t.Errorf("manifest entry %q has no sha256", entry.RelPath)
			continue
		}
		fullPath := filepath.Join(goldenRoot, entry.RelPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("manifest entry %q: read %s: %v", entry.RelPath, fullPath, err)
			continue
		}
		if entry.Bytes > 0 && len(data) != entry.Bytes {
			t.Errorf("manifest entry %q: byte count %d != recorded %d",
				entry.RelPath, len(data), entry.Bytes)
		}
		actual := sha256Hex(data)
		if actual != entry.Sha256 {
			t.Errorf("manifest drift for %q: actual=%s recorded=%s",
				entry.RelPath, actual, entry.Sha256)
		}
		checked++
	}
	if checked == 0 {
		t.Fatalf("manifest %s has zero non-TODO entries", manifestPath)
	}
	t.Logf("verified sha256 for %d manifest entries", checked)
}
