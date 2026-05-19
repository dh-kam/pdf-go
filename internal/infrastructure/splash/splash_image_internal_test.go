package splash

import (
	"testing"
)

// makeSrcRGB returns an ImageSource that streams a w*h RGB image whose pixel
// at (x,y) takes the supplied generator. Each call returns the next sequential
// row — Splash polls top-to-bottom so the row counter is local to the closure.
func makeSrcRGB(w, h int, gen func(x, y int) [3]byte) (ImageSource, *int) {
	row := 0
	rowsRead := &row
	return func(rIdx int, color, alpha []byte) error {
		_ = rIdx
		_ = alpha
		for x := 0; x < w; x++ {
			c := gen(x, *rowsRead)
			color[3*x] = c[0]
			color[3*x+1] = c[1]
			color[3*x+2] = c[2]
		}
		*rowsRead++
		return nil
	}, rowsRead
}

func newRGBSplash(w, h int) *Splash {
	bm := NewBitmap(w, h, ModeRGB8, false)
	bm.Clear(Color{0xFF, 0xFF, 0xFF})
	s, _ := New(bm, false)
	s.SetFillPattern(NewSolidColor(Color{}))
	return s
}

func TestImageScaleIdentity(t *testing.T) {
	src, _ := makeSrcRGB(4, 4, func(x, y int) [3]byte {
		return [3]byte{byte(10 * (x + y)), byte(20 * y), byte(30 * x)}
	})
	s := newRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 1, 1}
	if err := s.DrawImage(src, 4, 4, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestImageScale2xUp(t *testing.T) {
	src, _ := makeSrcRGB(2, 2, func(x, y int) [3]byte {
		return [3]byte{byte(50 + 50*x), byte(50 + 50*y), 0}
	})
	s := newRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 0, 0}
	if err := s.DrawImage(src, 2, 2, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestImageScaleHalfDown(t *testing.T) {
	src, _ := makeSrcRGB(8, 8, func(x, y int) [3]byte {
		return [3]byte{byte(10 * x), byte(10 * y), 0}
	})
	s := newRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 0, 0}
	if err := s.DrawImage(src, 8, 8, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestImageScalePrimeRatio(t *testing.T) {
	src, _ := makeSrcRGB(7, 5, func(x, y int) [3]byte {
		return [3]byte{byte(10 * x), byte(20 * y), 0xFF}
	})
	s := newRGBSplash(16, 16)
	mat := [6]float64{11, 0, 0, 13, 0, 0}
	if err := s.DrawImage(src, 7, 5, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestImageScaleSubPixelOffset(t *testing.T) {
	src, _ := makeSrcRGB(4, 4, func(x, y int) [3]byte {
		return [3]byte{0x80, 0x80, 0x80}
	})
	s := newRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 1.5, 2.25}
	if err := s.DrawImage(src, 4, 4, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

// TestBilinearLastRowClamp — THE memory-fragile rule. We construct a 2×3
// source where the last row is dramatically distinct from the rest, then
// scale up on Y. Without the last-row clamp the bilinear kernel reads
// uninitialised lineBuf2 past srcH-1 and corrupts the bottom rows. We
// verify the bottom row of the scaled output is the correct distinct value.
func TestBilinearLastRowClamp(t *testing.T) {
	// 2x3 RGB source: rows 0,1 are black, row 2 is pure red.
	src, _ := makeSrcRGB(2, 3, func(x, y int) [3]byte {
		if y == 2 {
			return [3]byte{0xFF, 0, 0}
		}
		return [3]byte{0, 0, 0}
	})
	bm := NewBitmap(8, 12, ModeRGB8, false)
	bm.Clear(Color{0, 0xFF, 0})
	s, _ := New(bm, false)
	s.SetFillPattern(NewSolidColor(Color{}))
	mat := [6]float64{8, 0, 0, 12, 0, 0}
	// interpolate=true forces bilinear path on a 4x scale-up boundary.
	if err := s.DrawImage(src, 2, 3, mat, true); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	// The very last pixel row of the destination should be predominantly
	// red (clamped to source row 2 = pure red). If the clamp is missing it
	// will be polluted with whatever uninitialised bytes lineBuf2 held.
	last := bm.height - 1
	for x := 0; x < bm.width; x++ {
		off := last*bm.rowSize + x*3
		// Allow some bilinear softening — assert R dominant, G/B near 0.
		r, g, b := bm.data[off], bm.data[off+1], bm.data[off+2]
		if r < 0xC0 || g > 0x40 || b > 0x40 {
			t.Fatalf("bilinear last-row clamp violated at x=%d: got RGB=(%d,%d,%d), want R-dominant near (255,0,0)",
				x, r, g, b)
		}
	}
}

func TestBilinearIdentitySmooth(t *testing.T) {
	src, _ := makeSrcRGB(4, 4, func(x, y int) [3]byte {
		return [3]byte{byte(40 * x), byte(40 * y), 0}
	})
	s := newRGBSplash(16, 16)
	mat := [6]float64{16, 0, 0, 16, 0, 0}
	// scaledW/srcW = 4 → C++ skips bilinear (>=4× rule). We force interpolate=true.
	if err := s.DrawImage(src, 4, 4, mat, true); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestArbitraryTransformRotation45(t *testing.T) {
	src, _ := makeSrcRGB(4, 4, func(x, y int) [3]byte {
		return [3]byte{byte(60 * x), byte(60 * y), 0}
	})
	s := newRGBSplash(16, 16)
	// 45° rotation + translation
	const c = 0.70710678
	mat := [6]float64{4 * c, 4 * c, -4 * c, 4 * c, 8, 4}
	if err := s.DrawImage(src, 4, 4, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestArbitraryTransformShear(t *testing.T) {
	src, _ := makeSrcRGB(3, 3, func(x, y int) [3]byte {
		return [3]byte{0x80, 0x40, 0xC0}
	})
	s := newRGBSplash(16, 16)
	// shear in X: mat[0]=a mat[1]=0 mat[2]=shear mat[3]=d
	mat := [6]float64{6, 0, 3, 6, 1, 1}
	if err := s.DrawImage(src, 3, 3, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestArbitraryTransformVerticalFlip(t *testing.T) {
	src, _ := makeSrcRGB(2, 2, func(x, y int) [3]byte {
		if y == 0 {
			return [3]byte{0xFF, 0, 0}
		}
		return [3]byte{0, 0, 0xFF}
	})
	s := newRGBSplash(8, 8)
	// negative mat[3] triggers the vertical-flip dispatcher.
	mat := [6]float64{4, 0, 0, -4, 0, 6}
	if err := s.DrawImage(src, 2, 2, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
}

func TestDownscaleVFlipTopDownUsesTopRows(t *testing.T) {
	const srcW, srcH = 6, 6
	src := func(row int, color, alpha []byte) error {
		sourceY := srcH - 1 - row
		gray := byte(sourceY * 40)
		for x := 0; x < srcW; x++ {
			color[3*x] = gray
			color[3*x+1] = gray
			color[3*x+2] = gray
			if alpha != nil {
				alpha[x] = 0xFF
			}
		}
		return nil
	}
	mat := [6]float64{4, 0, 0, -4, 1, 5}

	renderTopByte := func(withAlpha, forceTopDown bool) byte {
		t.Helper()
		bm := NewBitmap(8, 8, ModeRGB8, withAlpha)
		bm.Clear(Color{0xFF, 0xFF, 0xFF})
		s, err := New(bm, false)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		s.SetFillPattern(NewSolidColor(Color{}))
		s.downscaleVFlipTopDown = forceTopDown
		if err := s.DrawImage(src, srcW, srcH, mat, false); err != nil {
			t.Fatalf("DrawImage alpha=%v topDown=%v: %v", withAlpha, forceTopDown, err)
		}
		return s.bitmap.data[1*s.bitmap.rowSize+1*3]
	}

	defaultTop := renderTopByte(false, false)
	topDownTop := renderTopByte(false, true)
	if topDownTop != defaultTop {
		t.Fatalf("regular image downscale should use top-down rows by default: topDown=%d default=%d", topDownTop, defaultTop)
	}
	if topDownTop > 80 {
		t.Fatalf("top-down vflip downscale top row = %d, want source top rows", topDownTop)
	}

	alphaDefaultTop := renderTopByte(true, false)
	alphaTopDownTop := renderTopByte(true, true)
	if alphaTopDownTop >= alphaDefaultTop {
		t.Fatalf("alpha top-down gate did not select earlier rows: topDown=%d default=%d", alphaTopDownTop, alphaDefaultTop)
	}
	if alphaTopDownTop != topDownTop {
		t.Fatalf("alpha top-down gate should match regular top-down rows: alpha=%d regular=%d", alphaTopDownTop, topDownTop)
	}
}

func TestImageSingularMatrix(t *testing.T) {
	src, _ := makeSrcRGB(2, 2, func(x, y int) [3]byte { return [3]byte{0, 0, 0} })
	s := newRGBSplash(8, 8)
	if err := s.DrawImage(src, 2, 2, [6]float64{0, 0, 0, 0, 0, 0}, false); err != ErrSingularMatrix {
		t.Fatalf("expected ErrSingularMatrix, got %v", err)
	}
}

func TestImageZeroSource(t *testing.T) {
	src, _ := makeSrcRGB(2, 2, func(x, y int) [3]byte { return [3]byte{0, 0, 0} })
	s := newRGBSplash(8, 8)
	if err := s.DrawImage(src, 0, 0, [6]float64{4, 0, 0, 4, 0, 0}, false); err != ErrZeroImage {
		t.Fatalf("expected ErrZeroImage, got %v", err)
	}
}

func TestExpandRowLinear(t *testing.T) {
	// 4-pixel mono row → 8-pixel: each output pair should walk linearly.
	src := []byte{0, 0xFF, 0, 0xFF, 0} // last byte is the +1 padding slot
	dst := make([]byte, 8)
	expandRow(src, dst, 4, 8, 1)
	// expansion ratio xStep=0.5 → samples at 0,0.5,1,1.5,2,2.5,3,3.5
	// p=0 fr=0    → 0
	// p=0 fr=0.5  → 0.5*0 + 0.5*255 ≈ 127
	// p=1 fr=0    → 255
	// p=1 fr=0.5  → 0.5*255 + 0.5*0 ≈ 127
	// p=2 fr=0    → 0
	// p=2 fr=0.5  → 127
	// p=3 fr=0    → 255
	// p=3 fr=0.5  → 0.5*255 + 0.5*255 (pad) = 255
	want := []byte{0, 127, 255, 127, 0, 127, 255, 255}
	for i := range want {
		if abs8(dst[i], want[i]) > 1 {
			t.Errorf("expandRow[%d]=%d want≈%d", i, dst[i], want[i])
		}
	}
}

func abs8(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestVertFlipBitmap(t *testing.T) {
	bm := NewBitmap(2, 3, ModeMono8, false)
	bm.data = []byte{0xAA, 0xAA, 0xBB, 0xBB, 0xCC, 0xCC}
	vertFlipBitmap(bm, 1)
	if bm.data[0] != 0xCC || bm.data[4] != 0xAA {
		t.Errorf("vertFlipBitmap failed: %v", bm.data)
	}
}

// TestIntegerAligned2xDownscaleVFlipBoxAverage pins the
// integer-aligned axis-aligned 2× downscale fast path
// (drawIntegerAligned2xDownscaleVFlip) introduced 2026-04-27 to fix
// 007-imagemagick at 150 DPI. The bug: Splash's standard mat[3]<0 path
// computes scaledHeight = imgCoordMungeUpper(mat[5]) = 9 for an integer
// mat[5]=8, then runs Bresenham 16→9 + vertFlipBitmap + blit. That
// produced a 1-row vertical shift versus pdftoppm/legacy's reference,
// which uses Poppler's asymmetric 2× box grouping (popplerSourceRange1D
// at canvas/image_canvas_image_fastpath.go:113).
//
// The fixture: a 16×16 source whose unique-row pattern lets us identify
// exactly which source rows the kernel grouped together. We assert the
// asymmetric mapping (j=0 → src[0] alone, j=4 → src[7] alone, src rows
// 14&15 unused).
func TestIntegerAligned2xDownscaleVFlipBoxAverage(t *testing.T) {
	// Source rows 0..15: row r has R-channel value (r*16) so each row is
	// uniquely identifiable in the down-sampled output. G/B = 0.
	const W, H = 16, 16
	src, _ := makeSrcRGB(W, H, func(x, y int) [3]byte {
		// Splash's ImageSource closure (backend.go:651) reverses Y, so the
		// row index passed in here is the closure index, NOT the stdlib
		// row. Our DrawImage helper bypasses the closure and constructs an
		// ImageSource directly via makeSrcRGB which streams rows in
		// order — so y here is exactly the row order seen by
		// scaleImage / our fastpath.
		return [3]byte{byte(y * 16), 0, 0}
	})
	bm := NewBitmap(8, 8, ModeRGB8, false)
	bm.Clear(Color{0xFF, 0xFF, 0xFF})
	s, _ := New(bm, false)
	s.SetFillPattern(NewSolidColor(Color{}))
	// mat[3]=-8, mat[5]=8 → integer-aligned 2× vertical-flip downscale.
	mat := [6]float64{8, 0, 0, -8, 0, 8}
	if err := s.DrawImage(src, W, H, mat, false); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}

	// makeSrcRGB streams rows 0..H-1 in call order; the splash backend's
	// closure reverses Y for the standard pipeline. Our test uses
	// DrawImage directly so the closure isn't invoked — the source row r
	// passed by scaleImage IS the row whose generator argument was r.
	// drawIntegerAligned2xDownscaleVFlip uses `srcH-1-k` to invert that,
	// so the value it stores at stdlib row k = generator output for
	// closure row (srcH-1-k). With our generator y → 16y, closure row k
	// holds value 16k; after the fastpath's reverse, stdlib row r = 16
	// * (srcH-1-r). The popplerRange1D mapping then averages those
	// stdlib values. Compute the expected R for each (dy):
	expectR := func(dy int) byte {
		ry0, ry1 := popplerRange1D(dy, 8, H)
		var sum int
		for r := ry0; r < ry1; r++ {
			// stdlib row r has value (srcH-1-r)*16 in this test.
			sum += (H - 1 - r) * 16
		}
		return byte(sum / (ry1 - ry0))
	}
	for dy := 0; dy < 8; dy++ {
		want := expectR(dy)
		// All output columns should share R because src is constant in X.
		for dx := 0; dx < 8; dx++ {
			off := dy*bm.rowSize + dx*3
			got := bm.data[off]
			if got != want {
				t.Fatalf("dy=%d dx=%d: R=%d want=%d", dy, dx, got, want)
			}
		}
	}
}

func TestIntegerAligned2xDownscaleDispatcher(t *testing.T) {
	// Sanity: isIntegerAligned2xDownscale must accept the canonical 16×16
	// → 8×8 case and reject near-misses (non-integer origin, non-2×).
	cases := []struct {
		name string
		mat  [6]float64
		w, h int
		want bool
	}{
		{"canonical_2x_int_origin", [6]float64{8, 0, 0, -8, 0, 8}, 16, 16, true},
		{"shifted_int_origin", [6]float64{8, 0, 0, -8, 12, 16}, 16, 16, true},
		{"non_integer_origin", [6]float64{8, 0, 0, -8, 0.5, 8}, 16, 16, false},
		{"non_2x_ratio", [6]float64{8, 0, 0, -8, 0, 8}, 24, 16, false},
		{"odd_src", [6]float64{8, 0, 0, -8, 0, 8}, 17, 16, false},
		{"y_not_negative", [6]float64{8, 0, 0, 8, 0, 8}, 16, 16, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isIntegerAligned2xDownscale(tc.mat, tc.w, tc.h)
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestScaleImageDispatcherCovers(t *testing.T) {
	// covers all four down/up branches via direct calls.
	cases := []struct {
		name        string
		srcW, srcH  int
		dstW, dstH  int
		interpolate bool
	}{
		{"YdownXdown", 8, 8, 4, 4, false},
		{"YdownXup", 4, 8, 8, 4, false},
		{"YupXdown", 8, 4, 4, 8, false},
		{"YupXup_nearest", 2, 2, 8, 8, false},
		{"YupXup_bilinear", 4, 4, 8, 8, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, _ := makeSrcRGB(tc.srcW, tc.srcH, func(x, y int) [3]byte {
				return [3]byte{byte(x * 8), byte(y * 8), 0}
			})
			s := newRGBSplash(16, 16)
			scaled, err := s.scaleImage(src, tc.srcW, tc.srcH, tc.dstW, tc.dstH, tc.interpolate)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if scaled.width != tc.dstW || scaled.height != tc.dstH {
				t.Fatalf("%s: scaled size %dx%d, want %dx%d", tc.name, scaled.width, scaled.height, tc.dstW, tc.dstH)
			}
		})
	}
}
