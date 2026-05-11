package splash

import (
	"testing"
)

// makeMaskSrc returns an ImageMaskSource that produces a w*h mono mask whose
// pixel at (x,y) is the supplied generator's byte value (the source closure
// pulls rows in order — Splash polls top-to-bottom).
func makeMaskSrc(w, h int, gen func(x, y int) byte) ImageMaskSource {
	row := 0
	return func(rIdx int, line []byte) error {
		_ = rIdx
		for x := 0; x < w; x++ {
			line[x] = gen(x, row)
		}
		row++
		return nil
	}
}

func newMaskRGBSplash(w, h int) *Splash {
	bm := NewBitmap(w, h, ModeRGB8, false)
	bm.Clear(Color{0xFF, 0xFF, 0xFF})
	s, _ := New(bm, false)
	s.SetFillPattern(NewSolidColor(Color{0, 0, 0}))
	return s
}

func TestMaskScaleIdentity(t *testing.T) {
	src := makeMaskSrc(4, 4, func(x, y int) byte {
		if (x+y)%2 == 0 {
			return 1
		}
		return 0
	})
	s := newMaskRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 1, 1}
	if err := s.FillImageMask(src, 4, 4, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskScale2xUp(t *testing.T) {
	src := makeMaskSrc(2, 2, func(x, y int) byte {
		if x == 0 {
			return 1
		}
		return 0
	})
	s := newMaskRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 0, 0}
	if err := s.FillImageMask(src, 2, 2, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskScaleHalfDown(t *testing.T) {
	src := makeMaskSrc(8, 8, func(x, y int) byte {
		if x < 4 {
			return 1
		}
		return 0
	})
	s := newMaskRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 0, 0}
	if err := s.FillImageMask(src, 8, 8, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskScalePrimeRatio(t *testing.T) {
	src := makeMaskSrc(7, 5, func(x, y int) byte { return byte(x + y) % 2 })
	s := newMaskRGBSplash(16, 16)
	mat := [6]float64{11, 0, 0, 13, 0, 0}
	if err := s.FillImageMask(src, 7, 5, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskScaleSubPixelOffset(t *testing.T) {
	src := makeMaskSrc(4, 4, func(x, y int) byte { return 1 })
	s := newMaskRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, 4, 1.5, 2.25}
	if err := s.FillImageMask(src, 4, 4, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

// TestMaskLastRowEdge ensures the last source row of an upscaled mask
// reaches the bottom of the destination band — symmetric to the bilinear
// last-row clamp for color images, but for the YupXup mask kernel.
func TestMaskLastRowEdge(t *testing.T) {
	// 1-row mask of 1s at the bottom, 0s elsewhere.
	src := makeMaskSrc(2, 3, func(x, y int) byte {
		if y == 2 {
			return 1
		}
		return 0
	})
	s := newMaskRGBSplash(8, 12)
	mat := [6]float64{8, 0, 0, 12, 0, 0}
	if err := s.FillImageMask(src, 2, 3, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
	// Bottom band of the bitmap should be painted by the fill pattern (black).
	bm := s.bitmap
	last := bm.height - 1
	for x := 0; x < bm.width; x++ {
		off := last*bm.rowSize + x*3
		r := bm.data[off]
		if r > 0x40 {
			t.Fatalf("mask last-row clamp: bottom pixel at x=%d still white (R=%d)", x, r)
		}
	}
}

func TestMaskArbitraryTransformRotation(t *testing.T) {
	src := makeMaskSrc(4, 4, func(x, y int) byte { return 1 })
	s := newMaskRGBSplash(16, 16)
	const c = 0.70710678
	mat := [6]float64{4 * c, 4 * c, -4 * c, 4 * c, 8, 4}
	if err := s.FillImageMask(src, 4, 4, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskArbitraryTransformShear(t *testing.T) {
	src := makeMaskSrc(3, 3, func(x, y int) byte { return 1 })
	s := newMaskRGBSplash(16, 16)
	mat := [6]float64{6, 0, 3, 6, 1, 1}
	if err := s.FillImageMask(src, 3, 3, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskVerticalFlip(t *testing.T) {
	src := makeMaskSrc(2, 2, func(x, y int) byte {
		if y == 0 {
			return 1
		}
		return 0
	})
	s := newMaskRGBSplash(8, 8)
	mat := [6]float64{4, 0, 0, -4, 0, 6}
	if err := s.FillImageMask(src, 2, 2, mat, false); err != nil {
		t.Fatalf("FillImageMask: %v", err)
	}
}

func TestMaskSingularMatrix(t *testing.T) {
	src := makeMaskSrc(2, 2, func(x, y int) byte { return 1 })
	s := newMaskRGBSplash(8, 8)
	if err := s.FillImageMask(src, 2, 2, [6]float64{0, 0, 0, 0, 0, 0}, false); err != ErrSingularMatrix {
		t.Fatalf("expected ErrSingularMatrix, got %v", err)
	}
}

func TestMaskZeroSize(t *testing.T) {
	src := makeMaskSrc(0, 0, func(x, y int) byte { return 0 })
	s := newMaskRGBSplash(8, 8)
	if err := s.FillImageMask(src, 0, 0, [6]float64{4, 0, 0, 4, 0, 0}, false); err != ErrZeroImage {
		t.Fatalf("expected ErrZeroImage, got %v", err)
	}
}

func TestMaskScaleDispatcherCovers(t *testing.T) {
	cases := []struct {
		name       string
		srcW, srcH int
		dstW, dstH int
	}{
		{"YdownXdown", 8, 8, 4, 4},
		{"YdownXup", 4, 8, 8, 4},
		{"YupXdown", 8, 4, 4, 8},
		{"YupXup", 2, 2, 8, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := makeMaskSrc(tc.srcW, tc.srcH, func(x, y int) byte { return 1 })
			s := newMaskRGBSplash(16, 16)
			scaled, err := s.scaleMask(src, tc.srcW, tc.srcH, tc.dstW, tc.dstH)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if scaled.width != tc.dstW || scaled.height != tc.dstH {
				t.Fatalf("%s: scaled size %dx%d, want %dx%d", tc.name, scaled.width, scaled.height, tc.dstW, tc.dstH)
			}
			for i, b := range scaled.data {
				if b != 0xFF {
					t.Fatalf("%s[%d]=%d, expected 255 for all-ones source", tc.name, i, b)
				}
			}
		})
	}
}
