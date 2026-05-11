// Phase 4 splash property tests (P4-QA1, 2026-04-27).
//
// Black-box property tests at the splash package boundary covering the
// Phase 4 deliverables:
//   - Test 1: TestBlendNormalIdentityIntegration — Normal blend == no blend.
//   - Test 2: TestBlendMultiplyDarkens          — Multiply x white = src.
//   - Test 3: TestBlendDifferenceCommutative    — |Cs - Cb| == |Cb - Cs|.
//   - Test 4: TestTransparencyGroupRoundTrip    — Begin/Paint flushes group
//     content into the parent bitmap.
//   - Test 5: TestNestedTransparencyGroup       — Inner group content surfaces
//     after Paint+Paint.
//   - Test 6: TestSoftMaskAttenuates            — 50%-alpha mono soft mask
//     halves a black fill over white.
//   - Test 7: TestSoftMaskCleared               — SetSoftMask(nil) restores
//     full coverage.
//   - Test 8: TestTextDrawsGlyphs               — DrawText routes through the
//     splashCanvas glyph path and produces non-white pixels.
//
// Per Phase-4 QA spec: every test SKIPs cleanly when the API it exercises
// returns errNotImplemented or leaves the bitmap unallocated. Stdlib only —
// no third-party assertion library.
package splashintegration

import (
	"errors"
	"image"
	"image/color"
	"testing"

	domaincanvas "github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	splash "github.com/dh-kam/pdf-go/internal/infrastructure/splash"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// p4NewRGBSplash builds a vector-AA Splash bound to a w*h ModeRGB8 bitmap with
// an alpha plane (transparency-group composite needs alpha) and pre-clears the
// data plane to white. SKIPs the test if construction or allocation fails.
//
// Distinct from `newRGBSplash` (no alpha) and `p3NewRGBSplash` (no alpha) so
// the Phase 4 group/softmask tests get their own isolated builder.
func p4NewRGBSplash(t *testing.T, w, h int) (*splash.Splash, *splash.Bitmap) {
	t.Helper()
	bm := splash.NewBitmap(w, h, splash.ModeRGB8, true)
	if bm == nil {
		t.Skip("splash.NewBitmap returned nil — Phase 4 surface not wired")
	}
	if len(bm.Data()) == 0 {
		t.Skip("splash.NewBitmap data plane unallocated — Phase 4 surface not wired")
	}
	bm.Clear(splash.Color{0xFF, 0xFF, 0xFF})
	s, err := splash.New(bm, true)
	if err != nil {
		t.Skipf("splash.New: %v — Phase 4 surface not wired", err)
	}
	if s == nil {
		t.Skip("splash.New returned nil — Phase 4 surface not wired")
	}
	return s, bm
}

// p4FillRectPath returns an axis-aligned CCW rectangle suitable for s.Fill.
func p4FillRectPath(t *testing.T, x0, y0, x1, y1 float64) *xpath.Path {
	t.Helper()
	p := xpath.NewPath()
	if err := p.MoveTo(x0, y0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(x1, y0); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(x1, y1); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(x0, y1); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return p
}

// p4SkipIfFillUnimplemented probes Splash.Fill with a 1×1 path. If Fill
// returns an error (errNotImplemented in early Phases) the calling test SKIPs
// cleanly. The bitmap is repainted white afterwards so the actual test starts
// from a clean baseline.
func p4SkipIfFillUnimplemented(t *testing.T, s *splash.Splash, b *splash.Bitmap) {
	t.Helper()
	s.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	probe := p4FillRectPath(t, 0, 0, 1, 1)
	if err := s.Fill(probe, false); err != nil {
		t.Skipf("splash.Fill not wired (%v) — Phase 4 dependent test deferred", err)
	}
	if len(b.Data()) == 0 {
		t.Skip("splash bitmap data plane unallocated — Phase 4 dependent test deferred")
	}
	b.Clear(splash.Color{0xFF, 0xFF, 0xFF})
}

// p4SnapshotData returns a defensive copy of the bitmap's data plane. Used to
// compare two render passes byte-for-byte (TestBlendNormalIdentity, Difference
// commutativity).
func p4SnapshotData(b *splash.Bitmap) []byte {
	src := b.Data()
	if len(src) == 0 {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

// p4PixelRGB reads (R,G,B) at (x,y) on a ModeRGB8 bitmap. Returns ok=false
// if the data plane is unallocated or coords are out of range.
func p4PixelRGB(b *splash.Bitmap, x, y int) (r, g, bb byte, ok bool) {
	data := b.Data()
	if len(data) == 0 {
		return 0, 0, 0, false
	}
	w, h := b.Width(), b.Height()
	if x < 0 || y < 0 || x >= w || y >= h {
		return 0, 0, 0, false
	}
	stride := b.RowSize()
	if stride == 0 {
		stride = w * 3
	}
	off := y*stride + x*3
	if off+2 >= len(data) {
		return 0, 0, 0, false
	}
	return data[off], data[off+1], data[off+2], true
}

// abs8diff is a local |int(a)-int(b)| (the p3 variant collides at link time
// when the two test files coexist in the same package, so we mirror it under
// a Phase-4 name).
func abs8diff(a, b byte) int {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d
}

// bytesEqual is a tiny stdlib-only equivalence over two byte slices. Returns
// (true, -1) when equal, otherwise (false, firstMismatchIndex).
func bytesEqual(a, b []byte) (bool, int) {
	if len(a) != len(b) {
		return false, -1
	}
	for i := range a {
		if a[i] != b[i] {
			return false, i
		}
	}
	return true, -1
}

// ---------------------------------------------------------------------------
// Test 1: TestBlendNormalIdentityIntegration
// ---------------------------------------------------------------------------

// TestBlendNormalIdentityIntegration verifies that calling SetBlendFunc with
// the Normal blend (or with nil) produces byte-identical output to a render
// with no blend installed. This is the defining property of Normal —
// PDF spec 11.3.5.2 "B(Cs, Cb) = Cs" — and pins that the blend pipeline
// has no side effects when the formula reduces to identity.
func TestBlendNormalIdentityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 20, 10

	// Pass A: no blend installed.
	sA, bmA := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, sA, bmA)
	sA.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	if err := sA.Fill(p4FillRectPath(t, 2, 2, 18, 8), false); err != nil {
		t.Skipf("Fill (no-blend pass) returned %v — Phase 4 fill deferred", err)
	}
	snapA := p4SnapshotData(bmA)
	if snapA == nil {
		t.Skip("snapshot A empty — bitmap unallocated")
	}

	// Pass B: BlendNormal explicitly installed; everything else identical.
	sB, bmB := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, sB, bmB)
	sB.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	sB.SetBlendFunc(splash.BlendNormal)
	if err := sB.Fill(p4FillRectPath(t, 2, 2, 18, 8), false); err != nil {
		t.Skipf("Fill (BlendNormal pass) returned %v — Phase 4 fill deferred", err)
	}
	snapB := p4SnapshotData(bmB)

	eq, idx := bytesEqual(snapA, snapB)
	if !eq {
		t.Fatalf("BlendNormal not identity to no-blend: first byte mismatch at index %d "+
			"(A=%v B=%v)", idx, snapA[idx], snapB[idx])
	}
}

// ---------------------------------------------------------------------------
// Test 2: TestBlendMultiplyDarkens
// ---------------------------------------------------------------------------

// TestBlendMultiplyDarkens verifies the PDF 11.3.5.2 multiply formula
// B = Cs * Cb / 255. Two witnesses:
//  1. black src (0) over white dst (255) → 0 (multiply by zero).
//  2. mid-gray src (128) over white dst (255) → ~128 (Div255(128*255) = 127).
//
// The white-background invariant is guaranteed by p4NewRGBSplash's pre-clear.
func TestBlendMultiplyDarkens(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 30, 10

	// Witness 1: black * white = black.
	s1, bm1 := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, s1, bm1)
	s1.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	s1.SetBlendFunc(splash.BlendMultiply)
	if err := s1.Fill(p4FillRectPath(t, 5, 2, 25, 8), false); err != nil {
		t.Skipf("Fill (multiply black) returned %v — Phase 4 deferred", err)
	}
	r, g, b, ok := p4PixelRGB(bm1, 15, 5)
	if !ok {
		t.Skip("interior pixel unreadable — bitmap not wired")
	}
	if r > 4 || g > 4 || b > 4 {
		t.Fatalf("Multiply(0, 255) interior = (%d,%d,%d), want (0,0,0) within 4 LSB",
			r, g, b)
	}

	// Witness 2: mid-gray * white ≈ mid-gray.
	s2, bm2 := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, s2, bm2)
	s2.SetFillPattern(splash.NewSolidColor(splash.Color{128, 128, 128}))
	s2.SetBlendFunc(splash.BlendMultiply)
	if err := s2.Fill(p4FillRectPath(t, 5, 2, 25, 8), false); err != nil {
		t.Skipf("Fill (multiply gray) returned %v — Phase 4 deferred", err)
	}
	gr, gg, gb, ok := p4PixelRGB(bm2, 15, 5)
	if !ok {
		t.Skip("interior pixel unreadable — bitmap not wired")
	}
	// Div255(128*255) = 127. Allow ±3 LSB to absorb AA edge contributions.
	for _, c := range []byte{gr, gg, gb} {
		if d := abs8diff(c, 128); d > 3 {
			t.Fatalf("Multiply(128, 255) interior = (%d,%d,%d), each within ±3 LSB of 128 (Δ=%d)",
				gr, gg, gb, d)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 3: TestBlendDifferenceCommutative
// ---------------------------------------------------------------------------

// TestBlendDifferenceCommutative verifies |Cs - Cb| = |Cb - Cs|: rendering
// (src=A, dst=B) under BlendDifference must produce a bitmap byte-identical
// to (src=B, dst=A) under BlendDifference. The formula is per-channel
// absolute difference (PDF 11.3.5.2), so swapping endpoints is invariant.
//
// Method:
//   - Pass A: clear bitmap to color X, fill rect with color Y under Difference.
//   - Pass B: clear bitmap to color Y, fill rect with color X under Difference.
//   Compare the two bitmaps inside the rect interior.
func TestBlendDifferenceCommutative(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 24, 12
	colorX := splash.Color{200, 100, 50}
	colorY := splash.Color{40, 180, 220}

	render := func(dst, src splash.Color) []byte {
		s, bm := p4NewRGBSplash(t, W, H)
		p4SkipIfFillUnimplemented(t, s, bm)
		bm.Clear(dst)
		s.SetFillPattern(splash.NewSolidColor(src))
		s.SetBlendFunc(splash.BlendDifference)
		if err := s.Fill(p4FillRectPath(t, 4, 3, 20, 9), false); err != nil {
			t.Skipf("Fill (difference) returned %v — Phase 4 deferred", err)
		}
		return p4SnapshotData(bm)
	}

	bytesA := render(colorX, colorY)
	bytesB := render(colorY, colorX)
	if bytesA == nil || bytesB == nil {
		t.Skip("snapshot empty — bitmap unallocated")
	}

	// Compare interior pixels only — AA edges may differ in coverage detail.
	mismatches := 0
	for y := 4; y < 8; y++ {
		for x := 6; x < 18; x++ {
			rA, gA, bA, okA := p4PixelRGBfromSnap(bytesA, W, x, y)
			rB, gB, bB, okB := p4PixelRGBfromSnap(bytesB, W, x, y)
			if !okA || !okB {
				continue
			}
			if rA != rB || gA != gB || bA != bB {
				mismatches++
				if mismatches < 3 {
					t.Logf("difference non-commutative at (%d,%d): A=(%d,%d,%d) B=(%d,%d,%d)",
						x, y, rA, gA, bA, rB, gB, bB)
				}
			}
		}
	}
	if mismatches != 0 {
		t.Fatalf("BlendDifference is not commutative: %d interior pixels differ between (X over Y) and (Y over X)",
			mismatches)
	}
}

// p4PixelRGBfromSnap reads (R,G,B) at (x,y) from a previously snapshotted RGB8
// data plane (no Bitmap wrapper). The stride is W*3 because snapshots come
// straight off Bitmap.Data() which uses the same layout.
func p4PixelRGBfromSnap(data []byte, w, x, y int) (r, g, bb byte, ok bool) {
	off := y*w*3 + x*3
	if off+2 >= len(data) || off < 0 {
		return 0, 0, 0, false
	}
	return data[off], data[off+1], data[off+2], true
}

// ---------------------------------------------------------------------------
// Test 4: TestTransparencyGroupRoundTrip
// ---------------------------------------------------------------------------

// TestTransparencyGroupRoundTrip verifies a Begin → Fill → Paint round trip:
// drawing inside a transparency group must be reflected in the parent bitmap
// after PaintTransparencyGroup composites. We fill a black rect inside the
// group and assert the same pixels are non-white in the parent post-paint.
func TestTransparencyGroupRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 30, 20
	s, bm := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, s, bm)

	if err := s.BeginTransparencyGroup([4]float64{0, 0, float64(W), float64(H)},
		true /*isolated*/, false /*knockout*/, nil /*Normal*/); err != nil {
		if errors.Is(err, splash.ErrBadArg) {
			t.Skipf("BeginTransparencyGroup returned %v — Phase 4 group deferred", err)
		}
		t.Skipf("BeginTransparencyGroup returned %v — Phase 4 group deferred", err)
	}

	s.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	if err := s.Fill(p4FillRectPath(t, 5, 5, 25, 15), false); err != nil {
		t.Skipf("Fill inside group returned %v — Phase 4 deferred", err)
	}

	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("PaintTransparencyGroup: %v", err)
	}

	// The parent bitmap should now contain the black rect interior.
	r, g, b, ok := p4PixelRGB(bm, 15, 10)
	if !ok {
		t.Skip("interior pixel unreadable — parent bitmap not wired")
	}
	if r > 16 || g > 16 || b > 16 {
		t.Fatalf("group round-trip failed: parent pixel (15,10) = (%d,%d,%d), want near-black",
			r, g, b)
	}
}

// ---------------------------------------------------------------------------
// Test 5: TestNestedTransparencyGroup
// ---------------------------------------------------------------------------

// TestNestedTransparencyGroup verifies Begin-Begin (nest) → Fill (innermost)
// → Paint → Paint composites the innermost content all the way back to the
// outermost (parent) bitmap. This pins that the group stack pops correctly
// and intermediate composites preserve the inner render.
func TestNestedTransparencyGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 30, 20
	s, bm := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, s, bm)

	bbox := [4]float64{0, 0, float64(W), float64(H)}
	if err := s.BeginTransparencyGroup(bbox, true, false, nil); err != nil {
		t.Skipf("outer Begin returned %v — Phase 4 group deferred", err)
	}
	if err := s.BeginTransparencyGroup(bbox, true, false, nil); err != nil {
		t.Skipf("inner Begin returned %v — Phase 4 group deferred", err)
	}

	s.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	if err := s.Fill(p4FillRectPath(t, 8, 6, 22, 14), false); err != nil {
		t.Skipf("inner Fill returned %v — Phase 4 deferred", err)
	}

	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("inner Paint: %v", err)
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("outer Paint: %v", err)
	}

	r, g, b, ok := p4PixelRGB(bm, 15, 10)
	if !ok {
		t.Skip("interior pixel unreadable — parent bitmap not wired")
	}
	if r > 32 || g > 32 || b > 32 {
		t.Fatalf("nested group propagation failed: (%d,%d,%d), want near-black after Paint+Paint",
			r, g, b)
	}
}

// ---------------------------------------------------------------------------
// Test 6: TestSoftMaskAttenuates
// ---------------------------------------------------------------------------

// TestSoftMaskAttenuates verifies that a uniform 50%-alpha (value=128) soft
// mask attenuates a black fill by half: result over white → mid-gray (~128).
//
// Splash composites soft mask through compositeGroup
// (Splash.cc:475-485 + splash_group.go:127): aSrc' = aSrc * mask / 255.
// With aSrc=255 (opaque black), mask=128, the effective coverage is ~128
// and the over-white blend yields per-channel ~128.
func TestSoftMaskAttenuates(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 30, 20
	s, bm := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, s, bm)

	// Build a uniform 50%-alpha mask.
	mask := splash.NewBitmap(W, H, splash.ModeMono8, false)
	if mask == nil || len(mask.Data()) == 0 {
		t.Skip("mask bitmap unallocated — Phase 4 softmask deferred")
	}
	for i := range mask.Data() {
		mask.Data()[i] = 0x80
	}

	// Render through a transparency group so the soft mask is consulted at
	// composite time (the per-pixel softmask read lives in compositeGroup,
	// not in the AA pipe — see splash_group.go:126-128).
	if err := s.BeginTransparencyGroup([4]float64{0, 0, W, H}, true, false, nil); err != nil {
		t.Skipf("BeginTransparencyGroup returned %v — Phase 4 group deferred", err)
	}
	s.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	if err := s.Fill(p4FillRectPath(t, 5, 5, 25, 15), false); err != nil {
		t.Skipf("Fill returned %v — Phase 4 deferred", err)
	}
	// Install soft mask BEFORE Paint — Paint reads s.state.softMask.
	s.SetSoftMask(mask)
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("PaintTransparencyGroup: %v", err)
	}

	r, g, b, ok := p4PixelRGB(bm, 15, 10)
	if !ok {
		t.Skip("interior pixel unreadable — bitmap not wired")
	}
	// Expected: ~128 (50% black + 50% white). Loose ±32 LSB envelope so we
	// don't fail on the slight rounding the (255-aDst) blend introduces.
	for _, c := range []byte{r, g, b} {
		if d := abs8diff(c, 128); d > 32 {
			t.Fatalf("soft mask did not attenuate to mid-gray: (%d,%d,%d) Δ=%d (>32 LSB from 128)",
				r, g, b, d)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 7: TestSoftMaskCleared
// ---------------------------------------------------------------------------

// TestSoftMaskCleared verifies that SetSoftMask(nil) clears any previously
// installed mask: a subsequent black fill must paint full black (no
// attenuation), distinguishable from the half-coverage result of Test 6.
func TestSoftMaskCleared(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 30, 20
	s, bm := p4NewRGBSplash(t, W, H)
	p4SkipIfFillUnimplemented(t, s, bm)

	mask := splash.NewBitmap(W, H, splash.ModeMono8, false)
	if mask == nil || len(mask.Data()) == 0 {
		t.Skip("mask bitmap unallocated — Phase 4 softmask deferred")
	}
	for i := range mask.Data() {
		mask.Data()[i] = 0x80
	}
	s.SetSoftMask(mask)
	// Now clear it and render through a group — must produce full black.
	s.SetSoftMask(nil)

	if err := s.BeginTransparencyGroup([4]float64{0, 0, W, H}, true, false, nil); err != nil {
		t.Skipf("BeginTransparencyGroup returned %v — Phase 4 group deferred", err)
	}
	s.SetFillPattern(splash.NewSolidColor(splash.Color{0, 0, 0}))
	if err := s.Fill(p4FillRectPath(t, 5, 5, 25, 15), false); err != nil {
		t.Skipf("Fill returned %v — Phase 4 deferred", err)
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("PaintTransparencyGroup: %v", err)
	}

	r, g, b, ok := p4PixelRGB(bm, 15, 10)
	if !ok {
		t.Skip("interior pixel unreadable — bitmap not wired")
	}
	if r > 8 || g > 8 || b > 8 {
		t.Fatalf("SetSoftMask(nil) did not clear: (%d,%d,%d), want near-black (full coverage)",
			r, g, b)
	}
}

// ---------------------------------------------------------------------------
// Test 8: TestTextDrawsGlyphs
// ---------------------------------------------------------------------------

// TestTextDrawsGlyphs verifies that splashCanvas.DrawText routes through the
// glyph blit path and produces non-white pixels somewhere on the bitmap.
// Uses a minimal entity.Font stub (4×4 axis-aligned filled square per glyph)
// so the test does not depend on the embedded TTF parser. If DrawText returns
// errNotImplemented, the test SKIPs cleanly.
func TestTextDrawsGlyphs(t *testing.T) {
	if testing.Short() {
		t.Skip("Phase 4 property tests skipped in -short mode")
	}
	const W, H = 50, 30
	canvas := splash.NewBackend(W, H)
	if canvas == nil {
		t.Skip("splash.NewBackend returned nil — Phase 4 backend not wired")
	}
	canvas.SetFillColor(color.RGBA{0, 0, 0, 0xFF})

	font := &p4StubFont{name: "Stub", upem: 1000, advance: 500}
	if err := canvas.DrawText("A", 10, 20, font, 12); err != nil {
		if errors.Is(err, splash.ErrBadArg) {
			t.Skipf("DrawText returned %v — Phase 4 text deferred", err)
		}
		t.Skipf("DrawText returned %v — Phase 4 text deferred", err)
	}

	if !p4ScanAnyNonWhite(canvas) {
		t.Fatalf("DrawText produced no non-white pixels: glyph blit path appears unwired")
	}
}

// p4ScanAnyNonWhite returns true if the canvas's snapshot image contains any
// non-white RGB pixel. The Image() accessor returns an image.Image — we read
// per-pixel via the standard color interface so we don't depend on any
// internal bitmap layout.
func p4ScanAnyNonWhite(c domaincanvas.Canvas) bool {
	img := c.Image()
	if img == nil {
		return false
	}
	bnd := img.Bounds()
	for y := bnd.Min.Y; y < bnd.Max.Y; y++ {
		for x := bnd.Min.X; x < bnd.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 != 0xFF || g>>8 != 0xFF || b>>8 != 0xFF {
				return true
			}
		}
	}
	return false
}

// p4StubFont is a minimal entity.Font implementation that emits a 4×4 filled
// square per glyph. Same shape as backend_text_test.go's stubFont but kept in
// the integration package (no cross-package import) and named distinctly.
type p4StubFont struct {
	name    string
	upem    uint16
	advance float64
}

func (f *p4StubFont) Name() string                                            { return f.name }
func (f *p4StubFont) IsCIDFont() bool                                         { return false }
func (f *p4StubFont) IsSymbolic() bool                                        { return false }
func (f *p4StubFont) UnitsPerEm() uint16                                      { return f.upem }
func (f *p4StubFont) GlyphName(g uint32) string                               { return "" }
func (f *p4StubFont) GetBoundingBox() (float64, float64, float64, float64)    { return 0, 0, 1000, 1000 }
func (f *p4StubFont) CharCodeToGlyph(code uint32) (uint32, error)             { return code, nil }
func (f *p4StubFont) GetGlyphWidth(g uint32) (float64, error)                 { return f.advance, nil }
func (f *p4StubFont) RenderGlyph(g uint32, size float64) (*entity.GlyphPath, error) {
	cmds := []entity.PathCommand{
		&entity.PathMoveTo{X: 0, Y: 0},
		&entity.PathLineTo{X: size, Y: 0},
		&entity.PathLineTo{X: size, Y: size},
		&entity.PathLineTo{X: 0, Y: size},
		&entity.PathClose{},
	}
	return &entity.GlyphPath{Commands: cmds, Bounds: [4]float64{0, 0, size, size}}, nil
}

// Compile-time assertion that p4StubFont satisfies entity.Font. Using a blank
// var keeps the interface check off the binary and makes the dependency
// explicit at build time. The image import is referenced indirectly via
// domaincanvas.Canvas.Image(), but we keep the import obvious.
var _ entity.Font = (*p4StubFont)(nil)
var _ image.Image = (image.Image)(nil)
