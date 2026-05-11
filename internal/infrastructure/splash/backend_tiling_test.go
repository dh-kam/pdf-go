package splash

import (
	"image/color"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// TestDrawTilingPattern_FallbackWhenNoContent verifies the diagonal-stripe
// synth fallback fires when a tiling pattern has no content stream. Mirrors
// the Phase 3 behaviour and confirms the fill region is not left blank.
func TestDrawTilingPattern_FallbackWhenNoContent(t *testing.T) {
	c := newTestBackend(t, 16, 16)
	pattern := entity.NewTilingPattern("T1", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 4, 4})
	pattern.SetXStep(4)
	pattern.SetYStep(4)
	// No SetContent — exercise the synth fallback path.
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})

	if err := c.DrawTilingPattern(pattern, [4]float64{0, 0, 16, 16}); err != nil {
		t.Fatalf("DrawTilingPattern err: %v", err)
	}
	// The diagonal-stripe synth cell paints black at every (x+y) odd cell
	// position. We sample (1,0) (odd) — it must NOT be paper-white.
	r, g, b := readBackendPixel(t, c, 1, 0)
	if r == 0xFF && g == 0xFF && b == 0xFF {
		t.Fatalf("expected fallback stripe pixel at (1,0); got paper-white (%d,%d,%d)", r, g, b)
	}
}

// TestDrawTilingPattern_RendersContentStream verifies the cell content stream
// is actually executed by the sub-Splash recursion. The pattern paints a 4×4
// black square at the bbox origin; tiling that cell over a 16×16 region must
// stamp the square at every (i*step, j*step) tile origin.
func TestDrawTilingPattern_RendersContentStream(t *testing.T) {
	c := newTestBackend(t, 16, 16)
	pattern := entity.NewTilingPattern("T2", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 8, 8})
	pattern.SetXStep(8)
	pattern.SetYStep(8)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	// Cell content: solid-black rect from (0,0) to (4,4) in pattern space
	// — a quarter of the cell, so tile boundaries are observable.
	pattern.SetContent([]byte("0 0 0 rg\n0 0 4 4 re\nf\n"))
	pattern.SetResources(entity.NewDict())

	if err := c.DrawTilingPattern(pattern, [4]float64{0, 0, 16, 16}); err != nil {
		t.Fatalf("DrawTilingPattern err: %v", err)
	}

	// Inside the painted square: pixel (1, 14) (cell-bottom-left, near origin
	// in PDF Y-up which is splash row 14) should be black or near-black,
	// proving the content stream actually executed.
	rIn, gIn, bIn := readBackendPixel(t, c, 1, 14)
	if rIn > 64 || gIn > 64 || bIn > 64 {
		t.Fatalf("expected painted region near-black at (1,14); got (%d,%d,%d)", rIn, gIn, bIn)
	}
	// Outside the painted square but still inside the fill bbox: pixel (6, 1)
	// (above the painted quarter of the first tile) should be paper-white,
	// confirming the cell honours its own bbox subregion.
	rOut, gOut, bOut := readBackendPixel(t, c, 6, 1)
	if rOut < 200 || gOut < 200 || bOut < 200 {
		t.Fatalf("expected unpainted region paper-white at (6,1); got (%d,%d,%d)", rOut, gOut, bOut)
	}
}

func TestSetFillPattern_TilingInstallsSplashPattern(t *testing.T) {
	c := newTestBackend(t, 16, 16)
	pattern := entity.NewTilingPattern("TSet", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 8, 8})
	pattern.SetXStep(8)
	pattern.SetYStep(8)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	pattern.SetContent([]byte("0 0 0 rg\n0 0 4 4 re\nf\n"))
	pattern.SetResources(entity.NewDict())

	c.SetFillColor(color.RGBA{R: 0xff, A: 0xff})
	c.SetFillPattern(pattern)

	if _, ok := c.s.state.fillPattern.(*TilingPattern); !ok {
		t.Fatalf("expected fillPattern *TilingPattern, got %T", c.s.state.fillPattern)
	}
}

func TestSetFillPattern_TilingPaintType2UsesCurrentFillColor(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	pattern := entity.NewTilingPattern("TSetUncolored", 2, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 4, 4})
	pattern.SetXStep(4)
	pattern.SetYStep(4)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	pattern.SetContent([]byte("0 0 4 4 re\nf\n"))
	pattern.SetResources(entity.NewDict())

	c.SetFillColor(color.RGBA{G: 0xff, A: 0xff})
	c.SetFillPattern(pattern)
	c.Rectangle(0, 0, 8, 8)
	c.Fill()

	r, g, b := readBackendPixel(t, c, 1, 6)
	if r > 80 || g < 180 || b > 80 {
		t.Fatalf("PaintType=2 SetFillPattern should use current green fill tint, got (%d,%d,%d)", r, g, b)
	}
}

func TestDrawTilingPattern_PaintType2UsesCurrentFillColor(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	pattern := entity.NewTilingPattern("TUncolored", 2, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 4, 4})
	pattern.SetXStep(4)
	pattern.SetYStep(4)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	pattern.SetContent([]byte("0 0 4 4 re\nf\n"))
	pattern.SetResources(entity.NewDict())

	c.SetFillColor(color.RGBA{R: 0xff, A: 0xff})
	if err := c.DrawTilingPattern(pattern, [4]float64{0, 0, 8, 8}); err != nil {
		t.Fatalf("DrawTilingPattern err: %v", err)
	}

	r, g, b := readBackendPixel(t, c, 1, 6)
	if r < 180 || g > 80 || b > 80 {
		t.Fatalf("PaintType=2 should use current red fill tint, got (%d,%d,%d)", r, g, b)
	}
}

// TestRenderTilingCell_SubBitmap exercises the renderTilingCell helper directly
// with a known content stream and verifies the sub-rendered cell bitmap has
// painted (non-paper) pixels.
func TestRenderTilingCell_SubBitmap(t *testing.T) {
	pattern := entity.NewTilingPattern("T3", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 8, 8})
	pattern.SetXStep(8)
	pattern.SetYStep(8)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	pattern.SetContent([]byte("0 0 0 rg\n0 0 8 8 re\nf\n"))
	pattern.SetResources(entity.NewDict())

	bm, err := renderTilingCell(pattern, 8, 8, 8.0, 1.0, 1.0, 0.0, 0.0)
	if err != nil {
		t.Fatalf("renderTilingCell err: %v", err)
	}
	if bm == nil {
		t.Fatalf("renderTilingCell returned nil bitmap")
	}
	if bm.Width() != 8 || bm.Height() != 8 {
		t.Fatalf("expected 8x8 cell bitmap, got %dx%d", bm.Width(), bm.Height())
	}
	// At least one pixel inside the cell must be painted (not paper-white).
	data := bm.Data()
	bpp := bytesPerPixel(bm.Mode())
	painted := false
	for off := 0; off+bpp <= len(data); off += bpp {
		if data[off] != 0xFF || data[off+1] != 0xFF || data[off+2] != 0xFF {
			painted = true
			break
		}
	}
	if !painted {
		t.Fatalf("expected sub-rendered cell to have painted pixels; all paper-white")
	}
}

// TestSynthFallbackTilingCell_StripePattern verifies the synth fallback's
// diagonal-stripe pattern: cells at (x+y) odd should be black, even paper.
func TestSynthFallbackTilingCell_StripePattern(t *testing.T) {
	bm := synthFallbackTilingCell(4, 4, ModeRGB8)
	if bm == nil || bm.Width() != 4 || bm.Height() != 4 {
		t.Fatalf("expected 4x4 fallback cell, got %v", bm)
	}
	data := bm.Data()
	rs := bm.RowSize()
	// (1,0): odd → black.
	if data[0*rs+1*3] != 0 || data[0*rs+1*3+1] != 0 || data[0*rs+1*3+2] != 0 {
		t.Fatalf("expected (1,0) black; got (%d,%d,%d)",
			data[0*rs+1*3], data[0*rs+1*3+1], data[0*rs+1*3+2])
	}
	// (0,0): even → paper-white.
	if data[0*rs+0*3] != 0xFF || data[0*rs+0*3+1] != 0xFF || data[0*rs+0*3+2] != 0xFF {
		t.Fatalf("expected (0,0) paper-white; got (%d,%d,%d)",
			data[0], data[1], data[2])
	}
}
