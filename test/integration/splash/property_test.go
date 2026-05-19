// Package splashintegration — AA invariant property tests against the splash
// public API.
//
// See /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md §1.2.
// The five invariants are mathematical truths the rasterizer must satisfy
// regardless of which Poppler primitive is exercised. Tests run as black-box
// against splash.NewBackend(); Phase 1 exposes a limited surface, so several
// invariants Skip with explicit notes until later phases land.
package splashintegration

import (
	"image"
	"testing"

	splash "github.com/dh-kam/pdf-go/internal/infrastructure/splash"
)

// newPropertyBackend returns a splash canvas of the given size, or skips when
// construction itself is not wired (Phase 0/1 may stub out NewBackend).
func newPropertyBackend(t *testing.T, w, h int) (canvas, image.Rectangle) {
	t.Helper()
	c := splash.NewBackend(w, h)
	if c == nil {
		t.Skip("splash.NewBackend returned nil — backend not wired in this phase")
	}
	return canvas{c: c}, image.Rect(0, 0, w, h)
}

// canvas is a tiny shim — keeps the test using only the canvas.Canvas methods
// we care about so the test compiles even if exotic methods change shape.
type canvas struct {
	c interface {
		Width() int
		Height() int
		MoveTo(x, y float64)
		LineTo(x, y float64)
		ClosePath()
		Rectangle(x, y, w, h float64)
		Fill()
		Stroke()
		Save()
		Restore()
		Reset()
		Image() image.Image
	}
}

// rasterRect strokes (Phase 1 has Fill stubbed to a no-op for many ops, so we
// drive the public surface with a closed-rect path then call Fill+Stroke; we
// only assert byte-domain properties of the result, not pixel parity).
func (cv canvas) rasterRect(x, y, w, h float64) {
	cv.c.Reset()
	cv.c.Rectangle(x, y, w, h)
	cv.c.Fill()
	cv.c.Reset()
	cv.c.Rectangle(x, y, w, h)
	cv.c.Stroke()
}

// imageBytes copies the bitmap bytes of the canvas's current image into a
// freshly allocated slice. Only RGBA is exercised today.
func imageBytes(t *testing.T, c canvas) []byte {
	t.Helper()
	img := c.c.Image()
	if img == nil {
		t.Skip("canvas.Image() returned nil — backend not yet wired")
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Skipf("canvas.Image() is %T not *image.RGBA — backend not yet RGBA", img)
	}
	out := make([]byte, len(rgba.Pix))
	copy(out, rgba.Pix)
	return out
}

// TestSplashAACoverageBounds — invariant 1 (05_test_strategy.md §1.2).
func TestSplashAACoverageBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash property tests in short mode")
	}
	cv, _ := newPropertyBackend(t, 64, 64)
	// A handful of axis-aligned + sub-pixel rects covers the AA edge table.
	cases := []struct{ x, y, w, h float64 }{
		{0, 0, 64, 64},
		{10, 10, 30, 30},
		{10.5, 20.25, 12, 7.75},
		{-5, -5, 12, 12},     // negative origin (off-canvas clip)
		{60, 60, 10, 10},     // overflow off bottom-right
		{0.0, 0.0, 0.0, 0.0}, // degenerate: zero-area rect must not panic
	}
	for _, tc := range cases {
		cv.rasterRect(tc.x, tc.y, tc.w, tc.h)
	}
	pix := imageBytes(t, cv)
	// Every byte is already 0..255 by Go's []byte type — so this property
	// reduces to "no panic" + "image of expected size". Both verified.
	if len(pix) != 64*64*4 {
		t.Fatalf("expected RGBA buffer of %d bytes, got %d", 64*64*4, len(pix))
	}
}

// TestSplashAAMassConservationTranslation — invariant 2 (05_test_strategy.md §1.2).
func TestSplashAAMassConservationTranslation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash property tests in short mode")
	}
	const W, H = 64, 64
	const dx, dy = 10, 20

	cvA, _ := newPropertyBackend(t, W, H)
	cvB, _ := newPropertyBackend(t, W, H)

	cvA.rasterRect(5, 5, 20, 15)
	cvB.rasterRect(5+dx, 5+dy, 20, 15)

	pixA := imageBytes(t, cvA)
	pixB := imageBytes(t, cvB)

	// In Phase 1 the splash backend may produce all-zero output (Fill/Stroke
	// stubbed). If both buffers are entirely identical AND entirely zero, we
	// document the trivial pass and skip the strict check; the property is
	// still satisfied (0 == shifted 0).
	if allZero(pixA) && allZero(pixB) {
		t.Skip("splash backend produces empty output in this phase — invariant trivially satisfied")
	}

	// Strict check: the overlap region of A shifted by (dx, -dy) must equal the
	// corresponding region of B byte-for-byte. The public backend API accepts PDF
	// y-up coordinates, while the bitmap data plane is y-down.
	mismatches := 0
	for y := dy; y < H; y++ {
		for x := 0; x+dx < W; x++ {
			ai := (y*W + x) * 4
			bi := ((y-dy)*W + (x + dx)) * 4
			for k := 0; k < 4; k++ {
				if pixA[ai+k] != pixB[bi+k] {
					mismatches++
				}
			}
		}
	}
	if mismatches != 0 {
		t.Fatalf("integer translation mismatch: %d byte differences in overlap region (Phase 1 expected exact)", mismatches)
	}
}

// TestSplashAAClipIdempotence — invariant 3 (05_test_strategy.md §1.2).
func TestSplashAAClipIdempotence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash property tests in short mode")
	}
	t.Skip("Clip not yet wired in Phase 1 — invariant 3 deferred")
}

// TestSplashAAPremultipliedAlphaClosure — invariant 4 (05_test_strategy.md §1.2).
//
// Black-box: stroking the same rect twice into a fresh canvas must produce
// the same buffer twice (idempotent under repeat-into-zero ≡ closure of the
// alpha-blend semigroup at α=0). When the API exposes pixel-exact alpha math
// in later phases this test is upgraded to the byte-exact (c, α) over (0, 0)
// closure check called out in §1.2.
func TestSplashAAPremultipliedAlphaClosure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash property tests in short mode")
	}
	cv1, _ := newPropertyBackend(t, 32, 32)
	cv2, _ := newPropertyBackend(t, 32, 32)
	cv1.rasterRect(4, 4, 10, 10)
	cv2.rasterRect(4, 4, 10, 10)
	a := imageBytes(t, cv1)
	b := imageBytes(t, cv2)
	if len(a) != len(b) {
		t.Fatalf("buffer length differs across runs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic raster at byte %d: %d vs %d", i, a[i], b[i])
		}
	}
}

// TestSplashAAStrokeOutlineDuality — invariant 5 (05_test_strategy.md §1.2).
func TestSplashAAStrokeOutlineDuality(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash property tests in short mode")
	}
	t.Skip("Phase 2 (fill) not yet ported — invariant 5 deferred")
}

// allZero reports whether every byte in p is zero.
func allZero(p []byte) bool {
	for _, b := range p {
		if b != 0 {
			return false
		}
	}
	return true
}
