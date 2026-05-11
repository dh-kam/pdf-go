package xpath

import (
	"math"
	"testing"
)

// TestAddSegmentHorizontal verifies SP3 §9 invariant 3:
// horizontal edge produces XPathHoriz flag and dxdy=0
// (SplashXPath.cc:384-389).
func TestAddSegmentHorizontal(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 5, 10, 5)
	if len(x.Segs) != 1 {
		t.Fatalf("want 1 seg, got %d", len(x.Segs))
	}
	s := x.Segs[0]
	if s.Flags&XPathHoriz == 0 {
		t.Errorf("XPathHoriz not set: flags=%x", s.Flags)
	}
	if s.Flags&XPathVert != 0 {
		t.Errorf("XPathVert should be unset: flags=%x", s.Flags)
	}
	if s.DXDY != 0 || s.DYDX != 0 {
		t.Errorf("dxdy/dydx must be 0 for horiz: %v %v", s.DXDY, s.DYDX)
	}
	if s.Flags&XPathFlip != 0 {
		t.Errorf("XPathFlip should be unset (y0==y1): flags=%x", s.Flags)
	}
}

// TestAddSegmentVertical: x0==x1 sets XPathVert with dxdy/dydx=0.
func TestAddSegmentVertical(t *testing.T) {
	x := &XPath{}
	x.addSegment(7, 0, 7, 10)
	s := x.Segs[0]
	if s.Flags&XPathVert == 0 {
		t.Errorf("XPathVert not set: flags=%x", s.Flags)
	}
	if s.Flags&XPathHoriz != 0 {
		t.Errorf("XPathHoriz must not be set: flags=%x", s.Flags)
	}
	if s.DXDY != 0 || s.DYDX != 0 {
		t.Errorf("dxdy/dydx zero for vertical: %v %v", s.DXDY, s.DYDX)
	}
}

// TestAddSegmentZeroLength: degenerate point sets BOTH Horiz and Vert
// (SplashXPath.cc:384-389).
func TestAddSegmentZeroLength(t *testing.T) {
	x := &XPath{}
	x.addSegment(3, 3, 3, 3)
	s := x.Segs[0]
	if s.Flags&XPathHoriz == 0 || s.Flags&XPathVert == 0 {
		t.Errorf("degenerate seg must set both Horiz|Vert: flags=%x", s.Flags)
	}
	if s.Flags&XPathFlip != 0 {
		t.Errorf("y0==y1 cannot be flipped: flags=%x", s.Flags)
	}
}

// TestAddSegmentFlip: y0>y1 sets XPathFlip; endpoints stored verbatim
// (SplashXPath.cc:397-399).
func TestAddSegmentFlip(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 10, 5, 0)
	s := x.Segs[0]
	if s.Flags&XPathFlip == 0 {
		t.Errorf("XPathFlip not set when y0>y1: flags=%x", s.Flags)
	}
	if s.Y0 != 10 || s.Y1 != 0 {
		t.Errorf("endpoints must NOT be swapped: %v..%v", s.Y0, s.Y1)
	}
}

// TestAddSegmentAbutting verifies SP3 §9 invariant 4: two abutting segments
// produce flip flags letting the scanner avoid double-counting at the join.
func TestAddSegmentAbutting(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, 0, 5)  // downward in screen-Y → no flip
	x.addSegment(0, 5, 0, 10) // downward → no flip
	if x.Segs[0].Flags&XPathFlip != 0 {
		t.Errorf("first seg unexpectedly flipped: %x", x.Segs[0].Flags)
	}
	if x.Segs[1].Flags&XPathFlip != 0 {
		t.Errorf("second seg unexpectedly flipped: %x", x.Segs[1].Flags)
	}
}

// TestAddSegmentSlope: dxdy = (x1-x0)/(y1-y0); dydx = 1/dxdy
// (SplashXPath.cc:394-395).
func TestAddSegmentSlope(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, 4, 8)
	s := x.Segs[0]
	if s.DXDY != 0.5 {
		t.Errorf("dxdy: want 0.5 got %v", s.DXDY)
	}
	if s.DYDX != 2.0 {
		t.Errorf("dydx: want 2.0 got %v", s.DYDX)
	}
}

// TestAddCurveConverges: a cubic close to a line flattens within
// MaxCurveSplits without panic (SplashXPath.cc:268-371).
func TestAddCurveConverges(t *testing.T) {
	x := &XPath{}
	// nearly-straight cubic from (0,0) to (10,0)
	x.addCurve(0, 0, 3, 0.001, 7, -0.001, 10, 0, 1.0)
	if len(x.Segs) == 0 {
		t.Fatalf("expected at least 1 segment")
	}
	// endpoints must match the curve endpoints.
	if x.Segs[0].X0 != 0 || x.Segs[0].Y0 != 0 {
		t.Errorf("start endpoint: %v,%v", x.Segs[0].X0, x.Segs[0].Y0)
	}
	last := x.Segs[len(x.Segs)-1]
	if last.X1 != 10 || last.Y1 != 0 {
		t.Errorf("end endpoint: %v,%v", last.X1, last.Y1)
	}
}

// TestAddCurveDeepSubdivision: a heavily curved cubic still terminates;
// emits MaxCurveSplits-bounded segments (SplashXPath.cc:296, 327).
func TestAddCurveDeepSubdivision(t *testing.T) {
	x := &XPath{}
	x.addCurve(0, 0, 100, 100, -100, 100, 1, 0, 0.5)
	if len(x.Segs) == 0 {
		t.Fatalf("no segments produced")
	}
	if len(x.Segs) > MaxCurveSplits {
		t.Errorf("emitted %d segs, exceeds cap %d", len(x.Segs), MaxCurveSplits)
	}
}

// TestAAScaleScalesEndpoints verifies SP3 §9 + faithfulness rule 9: AAScale
// multiplies endpoints by 4 and DOES NOT touch slope (SplashXPath.cc:427-438).
func TestAAScaleScalesEndpoints(t *testing.T) {
	x := &XPath{}
	x.addSegment(1, 2, 5, 10)
	origDxdy := x.Segs[0].DXDY
	origDydx := x.Segs[0].DYDX
	x.AAScale()
	s := x.Segs[0]
	if s.X0 != 4 || s.Y0 != 8 || s.X1 != 20 || s.Y1 != 40 {
		t.Errorf("aaScale: %v,%v..%v,%v", s.X0, s.Y0, s.X1, s.Y1)
	}
	if s.DXDY != origDxdy || s.DYDX != origDydx {
		t.Errorf("aaScale must not recompute slope: dxdy %v->%v, dydx %v->%v",
			origDxdy, s.DXDY, origDydx, s.DYDX)
	}
}

// TestSortYMajorThenX verifies SP3 §9 + comparator at SplashXPath.cc:403-425.
// Upper-Y first; X is tie-breaker.
func TestSortYMajorThenX(t *testing.T) {
	x := &XPath{}
	// (3,5)→(7,10): upper-Y = 5
	x.addSegment(3, 5, 7, 10)
	// (1,1)→(2,8): upper-Y = 1
	x.addSegment(1, 1, 2, 8)
	// (10,5)→(11,9): upper-Y = 5, x=10 (tiebreak after first seg's x=3)
	x.addSegment(10, 5, 11, 9)
	x.Sort()

	if x.Segs[0].Y0 != 1 {
		t.Errorf("smallest upperY first: got y0=%v", x.Segs[0].Y0)
	}
	if x.Segs[1].X0 != 3 || x.Segs[2].X0 != 10 {
		t.Errorf("tie-broken by x: got x0=%v then %v", x.Segs[1].X0, x.Segs[2].X0)
	}
}

// TestSortRespectsFlip verifies the comparator reads flipped y from y1
// (SplashXPath.cc:409-415).
func TestSortRespectsFlip(t *testing.T) {
	x := &XPath{}
	// goes UP: y0=10, y1=2 → flipped, upper-Y=2
	x.addSegment(0, 10, 5, 2)
	// non-flipped, upper-Y=5
	x.addSegment(0, 5, 0, 9)
	x.Sort()
	// First should be the flipped segment because its upper-Y (=y1=2) is smallest.
	if x.Segs[0].Flags&XPathFlip == 0 {
		t.Errorf("flipped seg should sort first: flags=%x", x.Segs[0].Flags)
	}
}

// TestStrokeAdjustMin1pxSlab verifies SP3 §9 invariant 7: a hint with
// adj0==adj1==10.4 yields effective (10, 10.99) — minimum 1px slab
// (SplashXPath.cc:130-145).
func TestStrokeAdjustMin1pxSlab(t *testing.T) {
	// Build XPath with two vertical segs at x=10.4, then apply a hint that
	// targets ctrl0=0, ctrl1=1 (both at x=10.4, zero-width slab).
	x := &XPath{}
	x.addSegment(10.4, 0, 10.4, 5) // ctrl0
	x.addSegment(10.4, 5, 10.4, 0) // ctrl1
	hints := []PathHint{{Ctrl0: 0, Ctrl1: 1, FirstPt: 0, LastPt: 1}}
	x.StrokeAdjust(hints)

	// After adjust: x0 should be 10, x1 should be 10.99 (=11-0.01).
	// Both segs touch the x0/x1 zones depending on their x position.
	// At minimum, splashFloor(adj.x1) must equal adj.x0 (1px slab).
	// We test the public effect on the segments' X coords.
	for i, s := range x.Segs {
		// Slab edges: every endpoint should be adjusted to either 10.0 or 10.99.
		if !(approxEq(s.X0, 10.0) || approxEq(s.X0, 10.99)) {
			t.Errorf("seg[%d].X0 not snapped: %v", i, s.X0)
		}
		if !(approxEq(s.X1, 10.0) || approxEq(s.X1, 10.99)) {
			t.Errorf("seg[%d].X1 not snapped: %v", i, s.X1)
		}
	}
	// And splashFloor of upper edge equals lower edge integer (1px slab rule).
	if splashFloor(10.99) != 10 {
		t.Errorf("splashFloor(10.99)=%d, want 10", splashFloor(10.99))
	}
}

// TestStrokeAdjustNoOpWithoutHints verifies safety: no hints means no mutation.
func TestStrokeAdjustNoOpWithoutHints(t *testing.T) {
	x := &XPath{}
	x.addSegment(1.234, 5.678, 9.0, 10.5)
	before := x.Segs[0]
	x.StrokeAdjust(nil)
	if x.Segs[0] != before {
		t.Errorf("StrokeAdjust(nil) mutated seg: %v -> %v", before, x.Segs[0])
	}
}

// TestNaNEndpointPreservedInSegment verifies that addSegment does not crash
// on NaN endpoints — the scanner-side NaN guard (SplashXPathScanner.cc:56)
// is the layer responsible for filtering. addSegment itself just records.
func TestNaNEndpointPreservedInSegment(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, math.NaN(), 5)
	if len(x.Segs) != 1 {
		t.Fatalf("addSegment should still record: %d", len(x.Segs))
	}
	// flags must be coherent — y1 != y0, x0 != NaN, slope produces NaN.
	if !math.IsNaN(x.Segs[0].DXDY) {
		// NaN propagates through (NaN-0)/(5-0) = NaN
		t.Errorf("expected NaN dxdy from NaN endpoint, got %v", x.Segs[0].DXDY)
	}
}

// TestLengthEqualsSegCount: trivial, mirrors C++ length field.
func TestLengthEqualsSegCount(t *testing.T) {
	x := &XPath{}
	if x.Length() != 0 {
		t.Errorf("empty Length: %d", x.Length())
	}
	x.addSegment(0, 0, 1, 1)
	x.addSegment(2, 2, 3, 3)
	if x.Length() != 2 {
		t.Errorf("Length: want 2, got %d", x.Length())
	}
}

// TestXPathCtorTransformsAndFlattens exercises NewXPath end-to-end on a
// hand-built Path (bypasses Dev1's MoveTo/LineTo skeletons).
func TestXPathCtorTransformsAndFlattens(t *testing.T) {
	// 2-point line subpath: (0,0) -> (1,1), no curve, not closed.
	p := &Path{
		pts:   []PathPoint{{0, 0}, {1, 1}},
		flags: []byte{pathFirst, pathLast},
	}
	// Identity matrix with translation (5, 7).
	m := [6]float64{1, 0, 0, 1, 5, 7}
	x := NewXPath(p, m, 1.0, false)
	if len(x.Segs) != 1 {
		t.Fatalf("want 1 seg, got %d", len(x.Segs))
	}
	s := x.Segs[0]
	// Endpoint translated: (0,0)→(5,7), (1,1)→(6,8).
	if s.X0 != 5 || s.Y0 != 7 || s.X1 != 6 || s.Y1 != 8 {
		t.Errorf("transform: got %v,%v..%v,%v", s.X0, s.Y0, s.X1, s.Y1)
	}
}

// TestXPathCtorClosesSubpath verifies closeSubpaths=true emits the synthetic
// closing segment when the path was not explicitly closed
// (SplashXPath.cc:210-212).
func TestXPathCtorClosesSubpath(t *testing.T) {
	// (0,0) -> (10,0) -> (10,10), open. closeSubpaths=true should add
	// (10,10) -> (0,0) as a synthetic segment.
	p := &Path{
		pts:   []PathPoint{{0, 0}, {10, 0}, {10, 10}},
		flags: []byte{pathFirst, 0, pathLast},
	}
	m := [6]float64{1, 0, 0, 1, 0, 0}
	x := NewXPath(p, m, 1.0, true)
	if len(x.Segs) != 3 {
		t.Fatalf("want 3 segs (2 line + 1 close), got %d", len(x.Segs))
	}
	closer := x.Segs[2]
	if closer.X0 != 10 || closer.Y0 != 10 || closer.X1 != 0 || closer.Y1 != 0 {
		t.Errorf("closing seg wrong: %v,%v..%v,%v",
			closer.X0, closer.Y0, closer.X1, closer.Y1)
	}
}

// approxEq for float comparison in tests (1e-9 tolerance).
func approxEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
