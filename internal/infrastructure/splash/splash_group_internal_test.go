package splash

import (
	"testing"
)

// newGroupSplashRGB returns a Splash bound to a fresh RGB8 bitmap whose data
// plane is preset to paper-white (so the contrast against group draws is
// visible) and whose alpha plane is fully opaque.
func newGroupSplashRGB(t *testing.T, w, h int) *Splash {
	t.Helper()
	bm := NewBitmap(w, h, ModeRGB8, true)
	for i := range bm.data {
		bm.data[i] = 0xFF
	}
	for i := range bm.alpha {
		bm.alpha[i] = 0xFF
	}
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.fillAlpha = 1
	s.state.strokeAlpha = 1
	return s
}

// TestBeginPaintRoundTrip verifies that drawing into a group then painting it
// transfers the group rect onto the parent (PDF spec 11.4.7 Begin/Paint).
func TestBeginPaintRoundTrip(t *testing.T) {
	s := newGroupSplashRGB(t, 20, 20)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 20, 20}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if s.bitmap == parent {
		t.Fatalf("Begin should swap bitmap")
	}
	// Paint a 10x10 black opaque block into the group bitmap (manually,
	// to keep the test free of fillImpl coupling).
	g := s.bitmap
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 0
			g.data[off+1] = 0
			g.data[off+2] = 0
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("Paint should restore parent bitmap")
	}
	// Inside the rect: black on white = black.
	off := 7*parent.rowSize + 7*3
	if parent.data[off] != 0 || parent.data[off+1] != 0 || parent.data[off+2] != 0 {
		t.Fatalf("(7,7) parent: got %02x%02x%02x, want 000000",
			parent.data[off], parent.data[off+1], parent.data[off+2])
	}
	// Outside the rect: untouched white.
	off = 1*parent.rowSize + 1*3
	if parent.data[off] != 0xFF || parent.data[off+1] != 0xFF || parent.data[off+2] != 0xFF {
		t.Fatalf("(1,1) parent: got %02x%02x%02x, want FFFFFF",
			parent.data[off], parent.data[off+1], parent.data[off+2])
	}
}

// TestGroupStackDepth nests Begin twice, draws in the inner group, then Paints
// twice — the inner content must reach the outermost (parent) bitmap.
func TestGroupStackDepth(t *testing.T) {
	s := newGroupSplashRGB(t, 16, 16)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin outer: %v", err)
	}
	outer := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin inner: %v", err)
	}
	if s.bitmap == outer || s.bitmap == parent {
		t.Fatalf("inner group did not swap bitmap")
	}
	// Black block at (4..8) in inner.
	g := s.bitmap
	for y := 4; y < 8; y++ {
		for x := 4; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 0
			g.data[off+1] = 0
			g.data[off+2] = 0
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint inner: %v", err)
	}
	if s.bitmap != outer {
		t.Fatalf("after inner Paint expected outer bitmap")
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint outer: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("after outer Paint expected parent bitmap")
	}
	off := 5*parent.rowSize + 5*3
	if parent.data[off] != 0 || parent.data[off+1] != 0 || parent.data[off+2] != 0 {
		t.Fatalf("(5,5) parent: got %02x%02x%02x, want 000000",
			parent.data[off], parent.data[off+1], parent.data[off+2])
	}
}

// TestSoftMaskGating sets a uniform 50% mask and runs the AA pipe with a
// black, opaque source on a white backdrop. Result must be ~50% gray
// (black at 50% alpha over white = 127 or 128 due to integer rounding).
func TestSoftMaskGating(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	parent := s.bitmap
	mask := NewBitmap(4, 4, ModeMono8, false)
	for i := range mask.data {
		mask.data[i] = 128
	}
	s.SetSoftMask(mask)

	// Drive the AA pipe directly with full-coverage shape and opaque alpha.
	src := Color{0, 0, 0}
	var p pipe
	for y := 0; y < 4; y++ {
		s.pipeInit(&p, 0, y, nil, &src, 255, true, false)
		p.shape = 255
		pipeRun(&p, 4)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			off := y*parent.rowSize + x*3
			r := parent.data[off]
			if r < 124 || r > 132 {
				t.Fatalf("(%d,%d): got R=%d, want ~128 (50%% black over white)", x, y, r)
			}
		}
	}
}

func TestSoftMaskWithoutShapeUsesMask(t *testing.T) {
	s := newGroupSplashRGB(t, 1, 1)
	parent := s.bitmap
	mask := NewBitmap(1, 1, ModeMono8, false)
	mask.data[0] = 128
	s.SetSoftMask(mask)

	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, 255, false, false)
	p.run(&p)

	r := parent.data[0]
	if r < 124 || r > 132 {
		t.Fatalf("got R=%d, want ~128 from soft mask without source shape", r)
	}
}

func TestSoftMaskWithoutAlphaPlaneTreatsBackdropAsOpaque(t *testing.T) {
	bm := NewBitmap(1, 1, ModeRGB8, false)
	for i := range bm.data {
		bm.data[i] = 0xFF
	}
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mask := NewBitmap(1, 1, ModeMono8, false)
	mask.data[0] = 128
	s.SetSoftMask(mask)

	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, 255, false, false)
	p.run(&p)

	r := bm.data[0]
	if r < 124 || r > 132 {
		t.Fatalf("got R=%d, want ~128 from soft mask over opaque RGB bitmap", r)
	}
}

// TestSoftMaskCleared verifies that SetSoftMask(nil) restores the no-mask
// path so a fully-opaque source paints solid black.
func TestSoftMaskCleared(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	parent := s.bitmap
	// First install then clear.
	mask := NewBitmap(4, 4, ModeMono8, false)
	for i := range mask.data {
		mask.data[i] = 128
	}
	s.SetSoftMask(mask)
	s.SetSoftMask(nil)
	if s.state.softMask != nil {
		t.Fatalf("softMask should be nil after clear")
	}
	src := Color{0, 0, 0}
	var p pipe
	for y := 0; y < 4; y++ {
		s.pipeInit(&p, 0, y, nil, &src, 255, true, false)
		p.shape = 255
		pipeRun(&p, 4)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			off := y*parent.rowSize + x*3
			if parent.data[off] != 0 {
				t.Fatalf("(%d,%d): got R=%d, want 0 (full black)", x, y, parent.data[off])
			}
		}
	}
}

// TestGroupBlendMode draws a 50% gray block into a group, then paints over a
// fully-white parent under BlendMultiply. Multiply: 128 * 255 / 255 == 128;
// alpha-mix on opaque src yields 128 (the blended source replaces white).
func TestGroupBlendMode(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, BlendMultiply); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	g := s.bitmap
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 128
			g.data[off+1] = 128
			g.data[off+2] = 128
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	off := 4*parent.rowSize + 4*3
	r, gg, b := parent.data[off], parent.data[off+1], parent.data[off+2]
	if r < 124 || r > 132 || gg < 124 || gg > 132 || b < 124 || b > 132 {
		t.Fatalf("(4,4) parent: got %d,%d,%d, want ~128,128,128", r, gg, b)
	}
}

// TestEndTransparencyGroupDiscards verifies that End discards an unpainted
// group and restores the parent target.
func TestEndTransparencyGroupDiscards(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	g := s.bitmap
	// Draw black; should NOT reach parent because we End instead of Paint.
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 0
			g.data[off+1] = 0
			g.data[off+2] = 0
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("End should restore parent")
	}
	off := 4*parent.rowSize + 4*3
	if parent.data[off] != 0xFF {
		t.Fatalf("(4,4) parent should remain white after discard, got R=%d", parent.data[off])
	}
}

// TestSoftMaskOutOfBounds drives the AA pipe past the mask's right/bottom
// edges; softMaskByte must clamp to 0 (fully masked) so destination stays
// untouched white instead of panicking.
func TestSoftMaskOutOfBounds(t *testing.T) {
	s := newGroupSplashRGB(t, 6, 6)
	parent := s.bitmap
	mask := NewBitmap(2, 2, ModeMono8, false)
	for i := range mask.data {
		mask.data[i] = 255
	}
	s.SetSoftMask(mask)
	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, 255, true, false)
	p.shape = 255
	pipeRun(&p, 6)
	// (0,0) and (1,0): mask=255 → black.
	if parent.data[0*parent.rowSize+0*3] != 0 {
		t.Fatalf("(0,0): expected black under mask=255")
	}
	if parent.data[0*parent.rowSize+1*3] != 0 {
		t.Fatalf("(1,0): expected black under mask=255")
	}
	// (2,0)..(5,0): out of mask bounds → mask byte = 0 → src alpha = 0 →
	// destination preserved white.
	for x := 2; x < 6; x++ {
		off := 0*parent.rowSize + x*3
		if parent.data[off] != 0xFF {
			t.Fatalf("(%d,0): expected untouched white, got R=%d", x, parent.data[off])
		}
	}
}
