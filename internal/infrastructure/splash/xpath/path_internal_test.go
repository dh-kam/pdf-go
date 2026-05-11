package xpath

import (
	"errors"
	"math"
	"testing"
)

// TestPathEmpty verifies the freshly-constructed path matches SplashPath.cc:46-54.
func TestPathEmpty(t *testing.T) {
	p := NewPath()
	if !p.IsEmpty() {
		t.Fatal("new path must be empty")
	}
	if p.Length() != 0 {
		t.Fatalf("Length()=%d, want 0", p.Length())
	}
	if _, _, ok := p.GetCurPt(); ok {
		t.Fatal("GetCurPt on empty path must report !valid")
	}
	if p.IsCurSubpathClosed() {
		t.Fatal("empty path must not report a closed subpath")
	}
}

// TestPathMoveLineCloseRoundTrip mirrors the basic SplashPath.cc:121-194 sequence.
func TestPathMoveLineCloseRoundTrip(t *testing.T) {
	p := NewPath()
	if err := p.MoveTo(1, 2); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(3, 4); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close on a non-coincident endpoint, an extra synthetic point
	// is emitted (SplashPath.cc:184-189). Expected: 3 points total.
	if p.Length() != 3 {
		t.Fatalf("Length=%d, want 3 (move + line + close-line)", p.Length())
	}

	pt0, f0 := p.Point(0)
	pt1, f1 := p.Point(1)
	pt2, f2 := p.Point(2)
	if pt0 != (PathPoint{1, 2}) || pt2 != (PathPoint{1, 2}) {
		t.Fatalf("close did not return to first point: %v ... %v", pt0, pt2)
	}
	if pt1 != (PathPoint{3, 4}) {
		t.Fatalf("middle point = %v, want {3,4}", pt1)
	}
	if f0&pathFirst == 0 {
		t.Fatalf("flags[0]=%#x missing pathFirst", f0)
	}
	if f0&pathClosed == 0 {
		t.Fatalf("flags[0]=%#x missing pathClosed (SplashPath.cc:190)", f0)
	}
	if f1 != 0 {
		t.Fatalf("middle flag = %#x, want 0 (intermediate line vertex)", f1)
	}
	if f2&pathLast == 0 || f2&pathClosed == 0 {
		t.Fatalf("flags[2]=%#x missing pathLast|pathClosed", f2)
	}
}

// TestPathMoveToBogusOnOnePtSubpath asserts SplashPath.cc:123-125 rejection.
func TestPathMoveToBogusOnOnePtSubpath(t *testing.T) {
	p := NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("first MoveTo: %v", err)
	}
	// Second MoveTo while only one point exists in the prev subpath ⇒ bogus.
	if err := p.MoveTo(1, 1); !errors.Is(err, errBogusPath) {
		t.Fatalf("second MoveTo error = %v, want errBogusPath", err)
	}
}

// TestPathMoveToDroppingEmptySubpathKeepsLaterSegments verifies the PDF replay
// helper drops a trailing empty subpath without discarding earlier geometry.
func TestPathMoveToDroppingEmptySubpathKeepsLaterSegments(t *testing.T) {
	p := NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("first MoveTo: %v", err)
	}
	if err := p.LineTo(10, 0); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.MoveTo(20, 20); err != nil {
		t.Fatalf("empty MoveTo: %v", err)
	}
	if err := p.MoveToDroppingEmptySubpath(30, 30); err != nil {
		t.Fatalf("MoveToDroppingEmptySubpath: %v", err)
	}
	if got := p.Length(); got != 3 {
		t.Fatalf("Length() = %d, want 3", got)
	}
	pt, flag := p.Point(2)
	if pt.X != 30 || pt.Y != 30 {
		t.Fatalf("last point = (%.1f, %.1f), want (30, 30)", pt.X, pt.Y)
	}
	if flag&(pathFirst|pathLast) != pathFirst|pathLast {
		t.Fatalf("last flag = %#x, want pathFirst|pathLast", flag)
	}
}

// TestPathLineToNoCurPt asserts SplashPath.cc:139-141.
func TestPathLineToNoCurPt(t *testing.T) {
	p := NewPath()
	if err := p.LineTo(1, 1); !errors.Is(err, errNoCurPt) {
		t.Fatalf("LineTo without MoveTo = %v, want errNoCurPt", err)
	}
}

// TestPathCurveToShape verifies SplashPath.cc:154-177 (3 points, last carries pathLast).
func TestPathCurveToShape(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 0)
	if err := p.CurveTo(1, 1, 2, 2, 3, 3); err != nil {
		t.Fatalf("CurveTo: %v", err)
	}
	if p.Length() != 4 {
		t.Fatalf("Length=%d, want 4", p.Length())
	}
	_, f1 := p.Point(1)
	_, f2 := p.Point(2)
	_, f3 := p.Point(3)
	if f1&pathCurve == 0 || f2&pathCurve == 0 {
		t.Fatalf("ctrl flags=%#x,%#x missing pathCurve", f1, f2)
	}
	if f3&pathLast == 0 {
		t.Fatalf("end flag=%#x missing pathLast", f3)
	}
	if f3&pathCurve != 0 {
		t.Fatalf("end flag=%#x must NOT carry pathCurve (SplashPath.cc:174)", f3)
	}
}

// TestPathFlattenWithMatrixUsesDeviceSpaceFlatness verifies Splash.cc:2110-2122:
// the flatness test is evaluated after applying the matrix, so device-space
// scaling can increase the subdivision count even though output coordinates
// remain in user space.
func TestPathFlattenWithMatrixUsesDeviceSpaceFlatness(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 0)
	if err := p.CurveTo(0, 10, 10, 10, 10, 0); err != nil {
		t.Fatalf("CurveTo: %v", err)
	}

	identity := p.FlattenWithMatrix(20, [6]float64{1, 0, 0, 1, 0, 0})
	scaled := p.FlattenWithMatrix(20, [6]float64{4, 0, 0, 4, 0, 0})
	if identity.Length() >= scaled.Length() {
		t.Fatalf("scaled matrix should subdivide more: identity=%d scaled=%d", identity.Length(), scaled.Length())
	}
}

// TestPathFlattenWithMatrixKeepsUserSpaceOutput verifies that FlattenWithMatrix
// only uses the matrix for flatness decisions; emitted points match
// Splash::flattenCurve lineTo(xr3, yr3) in user-space coordinates.
func TestPathFlattenWithMatrixKeepsUserSpaceOutput(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 0)
	if err := p.CurveTo(0, 10, 10, 10, 10, 0); err != nil {
		t.Fatalf("CurveTo: %v", err)
	}

	flattened := p.FlattenWithMatrix(20, [6]float64{4, 0, 0, 4, 100, 200})
	for i := 0; i < flattened.Length(); i++ {
		pt, _ := flattened.Point(i)
		if pt.X < 0 || pt.X > 10 || pt.Y < 0 || pt.Y > 10 {
			t.Fatalf("point %d was transformed into device space: %+v", i, pt)
		}
	}
}

// TestPathCloseIdempotentOnSharedEndpoint ensures SplashPath.cc:184 does not add a
// duplicate line when first==last already coincide.
func TestPathCloseIdempotentOnSharedEndpoint(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 0)
	_ = p.LineTo(1, 0)
	_ = p.LineTo(0, 0) // already at first
	before := p.Length()
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if p.Length() != before {
		t.Fatalf("Close added a synthetic line on coincident endpoint (len %d→%d)", before, p.Length())
	}
	if !p.IsCurSubpathClosed() {
		t.Fatal("subpath flag pathClosed not set")
	}
}

// TestPathCloseForceAddsLine verifies the force=true branch (SplashPath.cc:184).
func TestPathCloseForceAddsLine(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 0)
	_ = p.LineTo(1, 0)
	_ = p.LineTo(0, 0)
	before := p.Length()
	if err := p.Close(true); err != nil {
		t.Fatalf("Close(force): %v", err)
	}
	if p.Length() != before+1 {
		t.Fatalf("Close(force) did not add a synthetic line (len %d→%d)", before, p.Length())
	}
}

// TestPathCloseNoCurPt asserts SplashPath.cc:181.
func TestPathCloseNoCurPt(t *testing.T) {
	p := NewPath()
	if err := p.Close(false); !errors.Is(err, errNoCurPt) {
		t.Fatalf("Close on empty path = %v, want errNoCurPt", err)
	}
}

// TestPathCloneIndependent verifies deep-copy semantics — invariant from §9 supporting
// safe path-pool reuse: mutation of the clone must not leak.
func TestPathCloneIndependent(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(1, 1)
	_ = p.LineTo(2, 2)
	p.AddStrokeAdjustHint(0, 1, 0, 1)

	c := p.Clone()
	if c.Length() != p.Length() {
		t.Fatalf("clone length=%d, original=%d", c.Length(), p.Length())
	}
	// Mutate clone.
	_ = c.LineTo(9, 9)
	c.hints[0].Ctrl0 = 99

	if p.Length() == c.Length() {
		t.Fatal("mutation of clone leaked back into original points")
	}
	if p.hints[0].Ctrl0 == 99 {
		t.Fatal("mutation of clone leaked back into original hints")
	}
}

// TestPathOffset verifies SplashPath.cc:212-220 across all stored points.
func TestPathOffset(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(1, 2)
	_ = p.LineTo(3, 4)
	_ = p.CurveTo(5, 6, 7, 8, 9, 10)
	p.Offset(10, 20)
	wantX := []float64{11, 13, 15, 17, 19}
	wantY := []float64{22, 24, 26, 28, 30}
	for i := 0; i < p.Length(); i++ {
		pt, _ := p.Point(i)
		if pt.X != wantX[i] || pt.Y != wantY[i] {
			t.Fatalf("Point(%d) = %v, want {%v,%v}", i, pt, wantX[i], wantY[i])
		}
	}
}

// TestPathOffsetZeroIsIdentity is a trivial property check.
func TestPathOffsetZeroIsIdentity(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(3.14, 2.71)
	_ = p.LineTo(-1, -2)
	pre := append([]PathPoint(nil), p.pts...)
	p.Offset(0, 0)
	for i := range pre {
		if p.pts[i] != pre[i] {
			t.Fatalf("Offset(0,0) at %d: %v -> %v", i, pre[i], p.pts[i])
		}
	}
}

// TestPathAddPath concatenates and re-bases curSubpath (SplashPath.cc:113).
func TestPathAddPath(t *testing.T) {
	a := NewPath()
	_ = a.MoveTo(0, 0)
	_ = a.LineTo(1, 0)

	b := NewPath()
	_ = b.MoveTo(10, 10)
	_ = b.LineTo(11, 10)
	_ = b.Close(false)

	aLen := a.Length()
	bSubBefore := b.curSubpath
	a.AddPath(b)

	if a.Length() != aLen+b.Length() {
		t.Fatalf("after AddPath len=%d, want %d", a.Length(), aLen+b.Length())
	}
	if a.curSubpath != aLen+bSubBefore {
		t.Fatalf("curSubpath=%d, want %d (re-based per SplashPath.cc:113)", a.curSubpath, aLen+bSubBefore)
	}
	// Points 0,1 are still A.
	pt, _ := a.Point(0)
	if pt != (PathPoint{0, 0}) {
		t.Fatalf("a.Point(0) leaked: %v", pt)
	}
	// Point 2 must be the start of B.
	pt, _ = a.Point(aLen)
	if pt != (PathPoint{10, 10}) {
		t.Fatalf("a.Point(%d) = %v, want {10,10}", aLen, pt)
	}
}

// TestPathAddPathNilOrEmpty must not corrupt state.
func TestPathAddPathNilOrEmpty(t *testing.T) {
	a := NewPath()
	_ = a.MoveTo(1, 1)
	pre := a.Length()
	a.AddPath(nil)
	a.AddPath(NewPath())
	if a.Length() != pre {
		t.Fatalf("AddPath(nil/empty) changed length %d→%d", pre, a.Length())
	}
}

// TestPathAddStrokeAdjustHintAppendOnly asserts hints are append-only and survive in order.
func TestPathAddStrokeAdjustHintAppendOnly(t *testing.T) {
	p := NewPath()
	p.AddStrokeAdjustHint(1, 2, 3, 4)
	p.AddStrokeAdjustHint(5, 6, 7, 8)
	if len(p.hints) != 2 {
		t.Fatalf("hints len=%d, want 2", len(p.hints))
	}
	if p.hints[0] != (PathHint{Ctrl0: 1, Ctrl1: 2, FirstPt: 3, LastPt: 4}) {
		t.Fatalf("hint[0]=%+v", p.hints[0])
	}
	if p.hints[1] != (PathHint{Ctrl0: 5, Ctrl1: 6, FirstPt: 7, LastPt: 8}) {
		t.Fatalf("hint[1]=%+v", p.hints[1])
	}
}

// TestPathGetCurPt verifies SplashPath.cc:222-230.
func TestPathGetCurPt(t *testing.T) {
	p := NewPath()
	if _, _, ok := p.GetCurPt(); ok {
		t.Fatal("expected !valid on empty")
	}
	_ = p.MoveTo(7, 8)
	x, y, ok := p.GetCurPt()
	if !ok || x != 7 || y != 8 {
		t.Fatalf("GetCurPt after MoveTo = (%v,%v,%v), want (7,8,true)", x, y, ok)
	}
	_ = p.LineTo(11, 12)
	x, y, ok = p.GetCurPt()
	if !ok || x != 11 || y != 12 {
		t.Fatalf("GetCurPt after LineTo = (%v,%v,%v), want (11,12,true)", x, y, ok)
	}
}

// ---- Invariants from 03_aa_scanner.md §9 (Path-shape subset) ----

// TestPathInvariantHorizontalEdgeAtIntegerY (§9 #3): a horizontal subpath at exact
// integer Y is well-formed at the path level — both endpoints retain Y=5.0 verbatim
// (no rounding inside Path). The scanner's count=0 rule applies later in XPath.
func TestPathInvariantHorizontalEdgeAtIntegerY(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 5)
	_ = p.LineTo(10, 5)
	a, _ := p.Point(0)
	b, _ := p.Point(1)
	if a.Y != 5 || b.Y != 5 {
		t.Fatalf("horizontal edge endpoints lost Y=5 exactness: %v,%v", a, b)
	}
}

// TestPathInvariantAbuttingSegments (§9 #4): two abutting segments at a shared vertex
// are stored as 3 distinct points, no implicit deduplication. Path.go must not collapse
// the shared vertex; that's the scanner's job.
func TestPathInvariantAbuttingSegments(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(0, 0)
	_ = p.LineTo(0, 5)  // shared vertex at (0,5)
	_ = p.LineTo(0, 10) // segment 2 starts at the shared vertex
	if p.Length() != 3 {
		t.Fatalf("abutting segments must store 3 points, got %d", p.Length())
	}
	v0, _ := p.Point(0)
	v1, _ := p.Point(1)
	v2, _ := p.Point(2)
	if v1.Y != 5 || v0.Y != 0 || v2.Y != 10 {
		t.Fatalf("vertices lost: %v %v %v", v0, v1, v2)
	}
}

// TestPathInvariant1pxAxisAlignedStroke (§9 #5): the path representation of a closed
// 1-px-wide axis-aligned subpath has the right point/flag layout. The closing line is
// synthesised by Close(false) only when first!=last (SplashPath.cc:184).
func TestPathInvariant1pxAxisAlignedStroke(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(10, 0)
	_ = p.LineTo(10, 100)
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Expect 3 points: (10,0), (10,100), (10,0) — the last is the synthetic close-line.
	if p.Length() != 3 {
		t.Fatalf("closed 1-px stroke layout has %d pts, want 3", p.Length())
	}
	first, ff := p.Point(0)
	last, lf := p.Point(p.Length() - 1)
	if first != last {
		t.Fatalf("first %v != last %v after close", first, last)
	}
	if ff&pathClosed == 0 || lf&pathClosed == 0 {
		t.Fatalf("first/last flags missing pathClosed: %#x,%#x", ff, lf)
	}
}

// TestPathInvariantDegenerateSegment (§9 #5/#7-style): zero-length segment is preserved.
// Splash relies on storing degenerate segments (SplashXPath.cc:384-389); Path must not drop them.
func TestPathInvariantDegenerateSegment(t *testing.T) {
	p := NewPath()
	_ = p.MoveTo(3, 4)
	_ = p.LineTo(3, 4) // zero-length segment
	if p.Length() != 2 {
		t.Fatalf("degenerate segment dropped: len=%d, want 2", p.Length())
	}
	a, _ := p.Point(0)
	b, _ := p.Point(1)
	if a != b {
		t.Fatalf("degenerate segment lost identity: %v vs %v", a, b)
	}
}

// TestPathInvariantNaNPreserved (§9 #9): NaN coordinates are stored verbatim — the
// downstream scanner is responsible for the early-return guard (SplashXPathScanner.cc:56-58).
// Path.go must not silently reject NaN; doing so would mask the bug at the scanner boundary.
func TestPathInvariantNaNPreserved(t *testing.T) {
	p := NewPath()
	if err := p.MoveTo(math.NaN(), 0); err != nil {
		t.Fatalf("MoveTo(NaN): unexpected error %v", err)
	}
	if err := p.LineTo(1, math.NaN()); err != nil {
		t.Fatalf("LineTo(NaN): unexpected error %v", err)
	}
	a, _ := p.Point(0)
	b, _ := p.Point(1)
	if !math.IsNaN(a.X) {
		t.Fatalf("MoveTo dropped NaN.x: %v", a.X)
	}
	if !math.IsNaN(b.Y) {
		t.Fatalf("LineTo dropped NaN.y: %v", b.Y)
	}
}
