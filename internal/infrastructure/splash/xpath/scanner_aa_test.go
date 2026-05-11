package xpath

import (
	"testing"
)

// makeRectXPath builds a closed axis-aligned rectangle (sub-pixel coords).
// Caller is responsible for aaScale-ing the inputs (the scanner runs in
// sub-pixel space). The four edges are emitted in Splash's standard order.
//
// NOTE: integer-aligned right/bottom edges paint one extra sub-cell column
// because splashFloor(N)==N — Splash normally avoids this by stroke-adjusting
// the right edge to (N-0.01) in `SplashXPath.cc:144`. For tests that want a
// rect spanning EXACTLY device cols [a, b-1] use makeRectXPathSnapped, which
// applies the same -0.01 trick on the right edge.
func makeRectXPath(x0, y0, x1, y1 float64) *XPath {
	x := &XPath{}
	x.addSegment(x0, y0, x1, y0) // top horiz
	x.addSegment(x1, y0, x1, y1) // right vert (down)
	x.addSegment(x1, y1, x0, y1) // bottom horiz
	x.addSegment(x0, y1, x0, y0) // left vert (up → flip)
	return x
}

// makeRectXPathSnapped is like makeRectXPath but offsets x1 by -0.01 to mimic
// Splash's stroke-adjusted right-edge convention (SplashXPath.cc:144). This
// guarantees splashFloor(x1) == x1_int - 1, so the painted sub-cell range is
// exactly [x0, x1_int) (x1_int - x0 cells). Used by tests that need a clean
// inclusive-low / exclusive-high cell range.
func makeRectXPathSnapped(x0, y0, x1, y1 float64) *XPath {
	return makeRectXPath(x0, y0, x1-0.01, y1-0.01)
}

// bitSet reports whether bit at sub-cell index `cell` of sub-row `subRow` is
// set in aaBuf (MSB-on-left) given the row stride rowSize bytes.
func bitSet(aaBuf []byte, rowSize, subRow, cell int) bool {
	off := subRow*rowSize + (cell >> 3)
	if off < 0 || off >= len(aaBuf) {
		return false
	}
	return aaBuf[off]&(1<<(7-(cell&7))) != 0
}

// countSetBits counts set bits in sub-row `subRow` of aaBuf within local
// sub-cell range [a, b).
func countSetBits(aaBuf []byte, rowSize, subRow, a, b int) int {
	c := 0
	for cell := a; cell < b; cell++ {
		if bitSet(aaBuf, rowSize, subRow, cell) {
			c++
		}
	}
	return c
}

// fillAll sets every byte in aaBuf to 0xff (test helper for ClipAALine).
func fillAll(aaBuf []byte) {
	for i := range aaBuf {
		aaBuf[i] = 0xff
	}
}

func TestClipAALineMasksRectBounds(t *testing.T) {
	c := NewClip(0, 0, 15, 15, true)
	if err := c.ClipToRect(2.5, 0, 5.25, 10); err != nil {
		t.Fatalf("ClipToRect: %v", err)
	}

	xMin, xMax := 0, 7
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	fillAll(aaBuf)

	c.ClipAALine(1, aaBuf, xMin, xMax)

	for sr := 0; sr < aaSize; sr++ {
		for cell := 0; cell < width; cell++ {
			want := cell >= 10 && cell < 22
			if got := bitSet(aaBuf, rowSize, sr, cell); got != want {
				t.Fatalf("sub-row %d cell %d = %v, want %v", sr, cell, got, want)
			}
		}
	}
}

// TestRenderAALineSolidRect: a 10×10 device-pixel rect (stroke-adjust snapped)
// at device row Y=5 must produce aaBuf with all 4 sub-rows fully set across
// cells 0..39 (10 device cols × aaSize=4).
func TestRenderAALineSolidRect(t *testing.T) {
	// Snap right/bottom -0.01 so splashFloor of the right edge falls at cell 39
	// (not 40), giving exactly 40 painted cells instead of 41.
	x := makeRectXPathSnapped(0, 0, 10*aaSize, 10*aaSize)
	// Scanner bbox bounds in sub-pixel coords (Splash passes yMinA*aaSize..yMaxA*aaSize).
	s := NewScanner(x, false /*eo=nonzero*/, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize // 40 sub-cells
	rowSize := (width + 7) >> 3         // 5 bytes
	aaBuf := make([]byte, rowSize*aaSize)

	s.RenderAALine(5, aaBuf, xMin, xMax)
	for sr := 0; sr < aaSize; sr++ {
		got := countSetBits(aaBuf, rowSize, sr, 0, width)
		if got != width {
			t.Errorf("sub-row %d: expected %d set bits, got %d", sr, width, got)
		}
	}
}

// TestRenderAALineHalfDiagonal: a triangle (0,0)-(10,0)-(10,10) closed —
// device-row Y=5 should produce ~half coverage on each sub-row, and the
// covered cells should be on the right side (the triangle's interior is
// x in [y, 10] for any y ∈ [0,10]).
func TestRenderAALineHalfDiagonal(t *testing.T) {
	x := &XPath{}
	// Triangle (0,0)→(10,0)→(10,10)→(0,0). All segments in sub-pixel coords.
	x.addSegment(0, 0, 10*aaSize, 0)
	x.addSegment(10*aaSize, 0, 10*aaSize, 10*aaSize)
	x.addSegment(10*aaSize, 10*aaSize, 0, 0) // hypotenuse
	s := NewScanner(x, false, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)

	s.RenderAALine(5, aaBuf, xMin, xMax)

	totalSet := 0
	for sr := 0; sr < aaSize; sr++ {
		totalSet += countSetBits(aaBuf, rowSize, sr, 0, width)
	}
	// Triangle at row 5 covers roughly half — allow generous tolerance for
	// sub-pixel sampling exactness across 4 sub-rows × 40 cells = 160 bits.
	maxBits := aaSize * width
	if totalSet < maxBits/4 || totalSet > 3*maxBits/4 {
		t.Errorf("triangle coverage at Y=5: got %d / %d (expected ~half)", totalSet, maxBits)
	}
	// Spot-check: cell 0 (left edge) should NOT be set on every sub-row, but
	// cell 39 (right edge under the hypotenuse) should be set somewhere.
	rightSet := false
	for sr := 0; sr < aaSize; sr++ {
		if bitSet(aaBuf, rowSize, sr, width-1) {
			rightSet = true
			break
		}
	}
	if !rightSet {
		t.Errorf("triangle at Y=5: expected at least one sub-row set at right edge cell %d", width-1)
	}
}

// TestRenderAALineClearsBetweenRows: SP3 §9 invariant 8 — aaBuf MUST be zeroed
// at the start of each RenderAALine call (no leftover bits from the previous
// row). Mirrors SplashXPathScanner.cc:360.
func TestRenderAALineClearsBetweenRows(t *testing.T) {
	x := makeRectXPathSnapped(0, 5*aaSize, 10*aaSize, 6*aaSize) // 1-row-tall rect at Y=5
	s := NewScanner(x, false, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)

	// Row 5: should have bits set.
	s.RenderAALine(5, aaBuf, xMin, xMax)
	any5 := false
	for sr := 0; sr < aaSize; sr++ {
		if countSetBits(aaBuf, rowSize, sr, 0, width) > 0 {
			any5 = true
			break
		}
	}
	if !any5 {
		t.Fatalf("Y=5 expected bits set inside the rect")
	}
	// Row 10: NO intersections (above & below the rect). aaBuf must be all zeros.
	s.RenderAALine(10, aaBuf, xMin, xMax)
	for i, b := range aaBuf {
		if b != 0 {
			t.Errorf("Y=10 leftover bits at byte %d = 0x%02x (must be zeroed)", i, b)
			break
		}
	}
}

// TestRenderAALineEdgeAlignment: a vertical edge at exact integer device-x = 5
// should produce a clean column boundary in aaBuf — bits set up to (but not
// past) sub-cell 5*aaSize=20 along the left edge of the path. Verifies the
// 1-LSB rule (splashFloor) at sub-pixel granularity.
func TestRenderAALineEdgeAlignment(t *testing.T) {
	// Filled rect spanning device cols [5, 9] × rows [0, 10] (right edge snapped).
	x := makeRectXPathSnapped(5*aaSize, 0, 10*aaSize, 10*aaSize)
	s := NewScanner(x, false, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize // 40 cells
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)

	s.RenderAALine(5, aaBuf, xMin, xMax)
	// Sub-cells 0..19 (device cols 0..4) MUST be unset on every sub-row.
	for sr := 0; sr < aaSize; sr++ {
		if got := countSetBits(aaBuf, rowSize, sr, 0, 5*aaSize); got != 0 {
			t.Errorf("sub-row %d: expected 0 set bits in cells [0,20), got %d", sr, got)
		}
		// Sub-cells 20..39 must all be set (interior of rect).
		if got := countSetBits(aaBuf, rowSize, sr, 5*aaSize, width); got != width-5*aaSize {
			t.Errorf("sub-row %d: expected %d set bits in cells [20,40), got %d", sr, width-5*aaSize, got)
		}
	}
}

// TestClipAALineIntersect: pre-fill aaBuf with all 1s, then clip against a
// rect covering device cols [2, 6] × all rows. Bits OUTSIDE the rect (in the
// gap regions) must be cleared; bits inside must pass through unchanged.
func TestClipAALineIntersect(t *testing.T) {
	// Clip path covers sub-cells [2*aaSize=8, 7*aaSize=28) = device cols 2..6.
	x := makeRectXPathSnapped(2*aaSize, 0, 7*aaSize, 10*aaSize)
	s := NewScanner(x, false, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	fillAll(aaBuf)

	s.ClipAALine(5, aaBuf, xMin, xMax)
	for sr := 0; sr < aaSize; sr++ {
		// Cells [0, 8): cleared.
		if got := countSetBits(aaBuf, rowSize, sr, 0, 2*aaSize); got != 0 {
			t.Errorf("sub-row %d: leading gap [0,8): expected 0 set, got %d", sr, got)
		}
		// Cells [8, 28): preserved (all 1s).
		want := 5 * aaSize
		if got := countSetBits(aaBuf, rowSize, sr, 2*aaSize, 7*aaSize); got != want {
			t.Errorf("sub-row %d: clip interior [8,28): expected %d set, got %d", sr, want, got)
		}
		// Cells [28, 40): cleared.
		if got := countSetBits(aaBuf, rowSize, sr, 7*aaSize, width); got != 0 {
			t.Errorf("sub-row %d: trailing gap [28,40): expected 0 set, got %d", sr, got)
		}
	}
}

// TestClipAALineEmpty: row Y=20 has no intersections (above the rect at Y∈[0,10]).
// Pre-filled aaBuf must be zeroed entirely (clip excludes everything).
func TestClipAALineEmpty(t *testing.T) {
	x := makeRectXPathSnapped(2*aaSize, 0, 7*aaSize, 10*aaSize)
	s := NewScanner(x, false, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	fillAll(aaBuf)

	s.ClipAALine(20, aaBuf, xMin, xMax)
	for i, b := range aaBuf {
		if b != 0 {
			t.Errorf("clip empty: byte %d = 0x%02x (must be zeroed)", i, b)
			break
		}
	}
}

// TestClipAALineNilScanner: scanner with empty path → ClipAALine zeros aaBuf
// entirely (entire row outside the [empty] clip region).
func TestClipAALineNilScanner(t *testing.T) {
	s := NewScanner(&XPath{}, false, 0, 0, 100, 100)
	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	fillAll(aaBuf)
	s.ClipAALine(0, aaBuf, xMin, xMax)
	for i, b := range aaBuf {
		if b != 0 {
			t.Errorf("nil-clip: byte %d = 0x%02x (must be zeroed)", i, b)
			break
		}
	}
}

// TestAARenderClipRoundtrip: render a fill, then clip with a smaller rect →
// final aaBuf is the AND of the two coverages (intersection at sub-pixel level).
func TestAARenderClipRoundtrip(t *testing.T) {
	// Fill: full rect at device cols [0, 9] × rows [0, 10] (snapped right edge).
	fillPath := makeRectXPathSnapped(0, 0, 10*aaSize, 10*aaSize)
	fs := NewScanner(fillPath, false, 0, 0, 100*aaSize, 100*aaSize)

	// Clip: smaller rect at device cols [3, 6] (snapped right edge).
	clipPath := makeRectXPathSnapped(3*aaSize, 0, 7*aaSize, 10*aaSize)
	cs := NewScanner(clipPath, false, 0, 0, 100*aaSize, 100*aaSize)

	xMin, xMax := 0, 9
	width := (xMax - xMin + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)

	fs.RenderAALine(5, aaBuf, xMin, xMax) // full row set
	cs.ClipAALine(5, aaBuf, xMin, xMax)   // intersect with [12, 28)

	for sr := 0; sr < aaSize; sr++ {
		// [0, 12): cleared.
		if got := countSetBits(aaBuf, rowSize, sr, 0, 3*aaSize); got != 0 {
			t.Errorf("sub-row %d: pre-clip gap should be clear, got %d set", sr, got)
		}
		// [12, 28): set.
		want := 4 * aaSize
		if got := countSetBits(aaBuf, rowSize, sr, 3*aaSize, 7*aaSize); got != want {
			t.Errorf("sub-row %d: clip interior expected %d set, got %d", sr, want, got)
		}
		// [28, 40): cleared.
		if got := countSetBits(aaBuf, rowSize, sr, 7*aaSize, width); got != 0 {
			t.Errorf("sub-row %d: post-clip gap should be clear, got %d set", sr, got)
		}
	}
}

// TestAABitOpsHelpers: direct unit tests for setBitsRange / clearBitsRange
// to lock down the MSB-first bit-twiddling. Single-byte, byte-aligned, and
// cross-byte cases.
func TestAABitOpsHelpers(t *testing.T) {
	// setBitsRange — single-byte partial.
	buf := []byte{0x00}
	setBitsRange(buf, 0, 2, 5)
	// bits 2..4 set, MSB-first → 0b00111000 = 0x38.
	if buf[0] != 0x38 {
		t.Errorf("setBitsRange[2,5): got 0x%02x want 0x38", buf[0])
	}
	// setBitsRange — byte-aligned full byte.
	buf = []byte{0x00}
	setBitsRange(buf, 0, 0, 8)
	if buf[0] != 0xff {
		t.Errorf("setBitsRange[0,8): got 0x%02x want 0xff", buf[0])
	}
	// setBitsRange — cross-byte.
	buf = []byte{0x00, 0x00}
	setBitsRange(buf, 0, 3, 13)
	// byte0 bits 3..7 set = 0x1f; byte1 bits 0..4 set = 0xf8.
	if buf[0] != 0x1f || buf[1] != 0xf8 {
		t.Errorf("setBitsRange[3,13): got 0x%02x,0x%02x want 0x1f,0xf8", buf[0], buf[1])
	}
	// clearBitsRange — single-byte partial against pre-filled.
	buf = []byte{0xff}
	clearBitsRange(buf, 0, 2, 5)
	// bits 2..4 cleared from 0xff → 0xc7.
	if buf[0] != 0xc7 {
		t.Errorf("clearBitsRange[2,5): got 0x%02x want 0xc7", buf[0])
	}
	// clearBitsRange — cross-byte.
	buf = []byte{0xff, 0xff}
	clearBitsRange(buf, 0, 3, 13)
	// byte0 bits 3..7 cleared from 0xff → 0xe0; byte1 bits 0..4 cleared from 0xff → 0x07.
	if buf[0] != 0xe0 || buf[1] != 0x07 {
		t.Errorf("clearBitsRange[3,13): got 0x%02x,0x%02x want 0xe0,0x07", buf[0], buf[1])
	}
}
