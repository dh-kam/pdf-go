package xpath

import (
	"math"
	"testing"
)

// helper: count total intersection entries across all rows.
func totalIntersections(s *Scanner) int {
	n := 0
	for _, line := range s.allIntersections {
		n += len(line)
	}
	return n
}

// helper: count entries on a given row with non-zero winding count.
func windingCount(s *Scanner, y int) int {
	if y < s.yMin || y > s.yMax {
		return 0
	}
	c := 0
	for _, e := range s.allIntersections[y-s.yMin] {
		c += e.Count
	}
	return c
}

// TestScannerEmptyXPath: empty XPath → bbox sentinels (xMin=yMin=1, xMax=yMax=0)
// per SplashXPathScanner.cc:52-53; computeIntersections returns immediately
// (yMin > yMax) so allIntersections is nil.
func TestScannerEmptyXPath(t *testing.T) {
	x := &XPath{}
	s := NewScanner(x, false, 0, 0, 100, 100)
	if s.xMin != 1 || s.yMin != 1 || s.xMax != 0 || s.yMax != 0 {
		t.Errorf("empty bbox: want (1,1,0,0), got (%d,%d,%d,%d)", s.xMin, s.yMin, s.xMax, s.yMax)
	}
	if s.allIntersections != nil {
		t.Errorf("empty path should not allocate allIntersections, got len=%d", len(s.allIntersections))
	}
	if _, _, ok := s.NextSpan(0); ok {
		t.Errorf("empty path NextSpan must return ok=false")
	}
	if s.HasNextSpan(0) {
		t.Errorf("empty path HasNextSpan must be false")
	}
}

// TestScannerNilXPath: nil XPath survives gracefully (defensive).
func TestScannerNilXPath(t *testing.T) {
	s := NewScanner(nil, false, 0, 0, 100, 100)
	if s.xMin != 1 || s.yMin != 1 || s.xMax != 0 || s.yMax != 0 {
		t.Errorf("nil bbox: want (1,1,0,0), got (%d,%d,%d,%d)", s.xMin, s.yMin, s.xMax, s.yMax)
	}
}

// TestScannerNaNGuard verifies SP3 §9 invariant 9: NaN endpoint causes
// scanner to early-return with empty bbox (SplashXPathScanner.cc:56-58, 75-77).
func TestScannerNaNGuard(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, math.NaN(), 5) // x1 = NaN
	s := NewScanner(x, false, 0, 0, 100, 100)
	if s.xMin != 1 || s.yMin != 1 || s.xMax != 0 || s.yMax != 0 {
		t.Errorf("NaN must early-return: want (1,1,0,0), got (%d,%d,%d,%d)",
			s.xMin, s.yMin, s.xMax, s.yMax)
	}
	if totalIntersections(s) != 0 {
		t.Errorf("NaN must produce 0 intersections, got %d", totalIntersections(s))
	}
}

// TestScannerNaNOnSecondSeg: NaN found mid-loop also early-returns.
func TestScannerNaNOnSecondSeg(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, 10, 10)
	x.addSegment(10, 10, math.NaN(), 20)
	s := NewScanner(x, false, 0, 0, 100, 100)
	if s.xMin != 1 || s.yMin != 1 {
		t.Errorf("NaN in second seg must keep sentinel bbox, got (%d,%d,%d,%d)",
			s.xMin, s.yMin, s.xMax, s.yMax)
	}
}

// TestScannerHorizontalEdgeCount0 verifies SP3 §9 invariant 3: horizontal
// edge contributes count=0 to winding (SplashXPathScanner.cc:253). Under both
// eo and nonzero, a path consisting of only a horizontal seg paints no spans
// because winding count never transitions.
func TestScannerHorizontalEdgeCount0(t *testing.T) {
	for _, eo := range []bool{false, true} {
		x := &XPath{}
		x.addSegment(2, 5, 8, 5) // horizontal at y=5, x in [2,8]
		s := NewScanner(x, eo, 0, 0, 100, 100)
		// expect: row 5 has exactly 1 intersection entry, with count==0.
		if !s.HasNextSpan(5) {
			t.Fatalf("eo=%v: row 5 should have an entry", eo)
		}
		entries := s.allIntersections[5-s.yMin]
		if len(entries) != 1 {
			t.Fatalf("eo=%v: want 1 entry, got %d", eo, len(entries))
		}
		if entries[0].Count != 0 {
			t.Errorf("eo=%v: horizontal must have count=0, got %d", eo, entries[0].Count)
		}
		// the span IS emitted (shape contribution): floor(2)..floor(8) = 2..8.
		x0, x1, ok := s.NextSpan(5)
		if !ok {
			t.Fatalf("eo=%v: NextSpan must emit horizontal span", eo)
		}
		if x0 != 2 || x1 != 8 {
			t.Errorf("eo=%v: span want [2,8], got [%d,%d]", eo, x0, x1)
		}
	}
}

// TestAddIntersectionWindingGate verifies Poppler's half-open segment rule:
// count is recorded for segYMin <= y < segYMax. Adjacent segments sharing y=5
// therefore append two entries, but only the segment starting at y=5 carries
// count on that row. (SplashXPathScanner.cc:339.)
func TestAddIntersectionWindingGate(t *testing.T) {
	// Build two abutting vertical-ish segments: (0,0)→(0,5) and (0,5)→(0,10).
	// At y=5 (segYMax of seg1 and segYMin of seg2), Poppler appends both
	// intersections.
	x := &XPath{}
	x.addSegment(2, 0, 4, 5)  // first seg, segYMax = 5
	x.addSegment(4, 5, 6, 10) // second seg, segYMin = 5
	s := NewScanner(x, false /*eo=false → nonzero*/, 0, 0, 100, 100)
	row := s.allIntersections[5-s.yMin]
	if len(row) != 2 {
		t.Fatalf("expected two raw Poppler intersections on row 5, got %d", len(row))
	}
	zeroCount, nonZeroCount := 0, 0
	for _, ent := range row {
		if ent.Count == 0 {
			zeroCount++
		} else {
			nonZeroCount++
		}
	}
	if zeroCount != 1 || nonZeroCount != 1 {
		t.Errorf("row 5 counts = %+v, want one zero and one non-zero entry", row)
	}
}

// TestAddIntersectionLowerEndpointGate verifies the half-open segment gate
// used by Poppler's SplashXPathScanner::addIntersection.
func TestAddIntersectionLowerEndpointGate(t *testing.T) {
	x := &XPath{}
	// seed scanner with bbox 0..10
	x.addSegment(0, 0, 0, 10)
	s := NewScanner(x, false, 0, 0, 100, 100)
	// y == segYMin carries winding count.
	s.allIntersections[5-s.yMin] = nil // clear test row
	s.addIntersection(5.0, 8.0, 5, 3, 4, 1)
	row := s.allIntersections[5-s.yMin]
	if len(row) != 1 {
		t.Fatalf("want 1 entry, got %d", len(row))
	}
	if row[0].Count != 1 {
		t.Errorf("y == segYMin must yield count=1, got %d", row[0].Count)
	}
	// y == segYMax is outside the segment.
	s.allIntersections[8-s.yMin] = nil
	s.addIntersection(5.0, 8.0, 8, 3, 4, 1)
	row = s.allIntersections[8-s.yMin]
	if row[0].Count != 0 {
		t.Errorf("y == segYMax must yield count=0, got %d", row[0].Count)
	}
	// segYMin < y records count.
	s.allIntersections[6-s.yMin] = nil
	s.addIntersection(5.0, 8.0, 6, 3, 4, 1)
	row = s.allIntersections[6-s.yMin]
	if row[0].Count != 1 {
		t.Errorf("row below segYMin must yield count=1, got %d", row[0].Count)
	}
}

// TestAddIntersectionAppendsTouchingEntries verifies Poppler's addIntersection
// appends raw entries and defers overlap handling to span iteration.
func TestAddIntersectionAppendsTouchingEntries(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, 0, 10)
	s := NewScanner(x, false, 0, 0, 100, 100)

	rowIdx := 5 - s.yMin
	s.allIntersections[rowIdx] = nil
	s.addIntersection(0, 10, 5, 3, 4, 1)
	s.addIntersection(0, 10, 5, 5, 6, 2)
	row := s.allIntersections[rowIdx]
	if len(row) != 2 {
		t.Fatalf("touching entries should remain raw, got %d entries", len(row))
	}

	s.addIntersection(0, 10, 5, 8, 9, 4)
	row = s.allIntersections[rowIdx]
	if len(row) != 3 {
		t.Fatalf("third entry should append, got %d entries", len(row))
	}
}

// TestComputeIntersectionsSlopeClamp verifies SP3 §7c: per-row clamp of
// xx0/xx1 to [segXMin, segXMax] (SplashXPathScanner.cc:295-309).
func TestComputeIntersectionsSlopeClamp(t *testing.T) {
	// segment (0,0)→(10,10), slope dxdy=1. segXMin=0, segXMax=10.
	// At y=0, xx0 is computed from xbase, the clamp should bound to [0,10].
	x := &XPath{}
	x.addSegment(0, 0, 10, 10)
	s := NewScanner(x, false, 0, 0, 100, 100)
	for y := 0; y <= 10; y++ {
		row := s.allIntersections[y-s.yMin]
		for _, e := range row {
			if e.X0 < 0 || e.X1 > 10 {
				t.Errorf("y=%d: x range [%d,%d] outside clamp [0,10]", y, e.X0, e.X1)
			}
		}
	}
}

// TestComputeIntersectionsVertical: vertical seg writes 1px intersections
// at floor(x) on every row in [floor(yMin),floor(yMax)] (SplashXPathScanner.cc:257-272).
func TestComputeIntersectionsVertical(t *testing.T) {
	x := &XPath{}
	x.addSegment(7.5, 2, 7.5, 5)
	s := NewScanner(x, false, 0, 0, 100, 100)
	for y := 2; y <= 5; y++ {
		row := s.allIntersections[y-s.yMin]
		if len(row) != 1 {
			t.Errorf("y=%d: want 1 entry, got %d", y, len(row))
			continue
		}
		if row[0].X0 != 7 || row[0].X1 != 7 {
			t.Errorf("y=%d: want x=7..7, got %d..%d", y, row[0].X0, row[0].X1)
		}
	}
	// scanner allocates rows for [yMin..yMax] only — verify alloc size matches.
	if len(s.allIntersections) != 4 {
		t.Errorf("want 4 rows allocated (yMax-yMin+1=4), got %d", len(s.allIntersections))
	}
}

// TestComputeIntersectionsHorizontalSpan: single horizontal seg → spans for
// the one row, with x range from min(x0,x1) to max(x0,x1).
func TestComputeIntersectionsHorizontalSpan(t *testing.T) {
	x := &XPath{}
	x.addSegment(8, 4, 2, 4) // x0>x1
	s := NewScanner(x, false, 0, 0, 100, 100)
	row := s.allIntersections[4-s.yMin]
	if len(row) != 1 {
		t.Fatalf("want 1 entry, got %d", len(row))
	}
	if row[0].X0 != 2 || row[0].X1 != 8 {
		t.Errorf("want x=[2,8], got [%d,%d]", row[0].X0, row[0].X1)
	}
}

// TestBBoxBasic exercises BBox getters.
func TestBBoxBasic(t *testing.T) {
	x := &XPath{}
	x.addSegment(2.3, 4.7, 8.6, 9.1)
	s := NewScanner(x, false, 0, 0, 100, 100)
	xMin, yMin, xMax, yMax := s.BBox()
	// floor(2.3)=2, floor(8.6)=8, floor(4.7)=4, floor(9.1)=9
	if xMin != 2 || yMin != 4 || xMax != 8 || yMax != 9 {
		t.Errorf("BBox want (2,4,8,9), got (%d,%d,%d,%d)", xMin, yMin, xMax, yMax)
	}
}

// TestBBoxAA verifies BBoxAA divides BBox by aaSize=4 (SplashXPathScanner.cc:117-123).
func TestBBoxAA(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, 16, 20) // bbox 0..16, 0..20
	s := NewScanner(x, false, 0, 0, 100, 100)
	xMin, yMin, xMax, yMax := s.BBox()
	axMin, ayMin, axMax, ayMax := s.BBoxAA()
	if axMin != xMin/4 || ayMin != yMin/4 || axMax != xMax/4 || ayMax != yMax/4 {
		t.Errorf("BBoxAA want (%d,%d,%d,%d), got (%d,%d,%d,%d)",
			xMin/4, yMin/4, xMax/4, yMax/4, axMin, ayMin, axMax, ayMax)
	}
	// explicit values
	if axMin != 0 || ayMin != 0 || axMax != 4 || ayMax != 5 {
		t.Errorf("BBoxAA explicit: want (0,0,4,5), got (%d,%d,%d,%d)", axMin, ayMin, axMax, ayMax)
	}
}

// TestNextSpanEmpty: span query outside bbox returns ok=false.
func TestNextSpanEmpty(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 5, 10, 5)
	s := NewScanner(x, false, 0, 0, 100, 100)
	if _, _, ok := s.NextSpan(99); ok {
		t.Errorf("y=99 outside bbox must return ok=false")
	}
	if s.HasNextSpan(99) {
		t.Errorf("HasNextSpan(99) must be false")
	}
}

// TestNextSpanNonzeroCoalesce: build a closed CCW square and verify NextSpan
// emits a single coalesced span across the interior under nonzero winding.
func TestNextSpanNonzeroCoalesce(t *testing.T) {
	// closed CCW square (0,0)→(10,0)→(10,10)→(0,10)→(0,0).
	x := &XPath{}
	x.addSegment(0, 0, 10, 0)   // top horizontal
	x.addSegment(10, 0, 10, 10) // right vertical (count: -1 if not flip → -1)
	x.addSegment(10, 10, 0, 10) // bottom horizontal
	x.addSegment(0, 10, 0, 0)   // left vertical (flipped → count +1)
	s := NewScanner(x, false, 0, 0, 100, 100)
	// pick a row inside, e.g. y=5: expect a single span [0,10].
	it := s.Iterator(5)
	x0, x1, ok := it.NextSpan()
	if !ok {
		t.Fatalf("row 5 must emit a span")
	}
	if x0 != 0 || x1 != 10 {
		t.Errorf("row 5 span want [0,10], got [%d,%d]", x0, x1)
	}
	if _, _, ok := it.NextSpan(); ok {
		t.Errorf("row 5 must emit only 1 coalesced span, got more")
	}
}

// TestEOOverlapEmpty verifies SP3 §9 invariant 10: under even-odd, the
// doubly-covered overlap of two CCW rectangles produces non-overlapping spans
// (the inner doubly-covered region is excluded).
func TestEOOverlapEmpty(t *testing.T) {
	// rect A: (0,0)-(10,10) and rect B: (5,0)-(15,10) — they overlap in
	// x range [5,10]. Under eo, sample row 5: the inner region [5,10] must
	// NOT be inside (count parity goes 1→0 there).
	x := &XPath{}
	// rect A edges
	x.addSegment(0, 0, 10, 0)
	x.addSegment(10, 0, 10, 10)
	x.addSegment(10, 10, 0, 10)
	x.addSegment(0, 10, 0, 0)
	// rect B edges
	x.addSegment(5, 0, 15, 0)
	x.addSegment(15, 0, 15, 10)
	x.addSegment(15, 10, 5, 10)
	x.addSegment(5, 10, 5, 0)
	s := NewScanner(x, true /*eo*/, 0, 0, 100, 100)
	// On row 5, with eo, expect TWO spans: [0,5] and [10,15] (overlap excluded).
	it := s.Iterator(5)
	a0, a1, ok := it.NextSpan()
	if !ok {
		t.Fatalf("row 5 first span missing")
	}
	b0, b1, ok := it.NextSpan()
	if !ok {
		t.Fatalf("row 5 second span missing (eo overlap rule)")
	}
	// expect a==[0,5], b==[10,15] (or swapped if x ordering surprises us)
	if !(a0 == 0 && a1 == 5 && b0 == 10 && b1 == 15) {
		t.Errorf("eo overlap: spans want [0,5],[10,15]; got [%d,%d],[%d,%d]", a0, a1, b0, b1)
	}
	if _, _, ok := it.NextSpan(); ok {
		t.Errorf("row 5 should have exactly 2 spans under eo, got 3+")
	}
}

// TestComputeIntersectionsAllocSize: allocates exactly yMax-yMin+1 rows.
func TestComputeIntersectionsAllocSize(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 3, 10, 9)
	s := NewScanner(x, false, 0, 0, 100, 100)
	want := s.yMax - s.yMin + 1
	if len(s.allIntersections) != want {
		t.Errorf("alloc size want %d, got %d", want, len(s.allIntersections))
	}
}

// TestNextSpanFollowsNonzero: nonzero rule keeps inside coalescing; eo flips parity.
func TestNextSpanFollowsNonzero(t *testing.T) {
	// two stacked CCW rects sharing an edge in y: rect1 (0,0)-(10,5), rect2 (0,5)-(10,10).
	x := &XPath{}
	// rect1
	x.addSegment(0, 0, 10, 0)
	x.addSegment(10, 0, 10, 5)
	x.addSegment(10, 5, 0, 5)
	x.addSegment(0, 5, 0, 0)
	// rect2
	x.addSegment(0, 5, 10, 5)
	x.addSegment(10, 5, 10, 10)
	x.addSegment(10, 10, 0, 10)
	x.addSegment(0, 10, 0, 5)
	s := NewScanner(x, false, 0, 0, 100, 100)
	// row 7 is inside rect2 only — expect single span [0,10].
	it := s.Iterator(7)
	a0, a1, ok := it.NextSpan()
	if !ok || a0 != 0 || a1 != 10 {
		t.Errorf("nonzero row 7 want [0,10], got [%d,%d] ok=%v", a0, a1, ok)
	}
}

// TestPartialClipSetsFlag: clipYMax narrows yMax and sets partialClip.
func TestPartialClipSetsFlag(t *testing.T) {
	x := &XPath{}
	x.addSegment(0, 0, 10, 50)
	s := NewScanner(x, false, 0, 0, 100, 20) // clipYMax=20 < 50
	if !s.partialClip {
		t.Errorf("partialClip must be set when clipYMax < computed yMax")
	}
	if s.yMax != 20 {
		t.Errorf("yMax want 20, got %d", s.yMax)
	}
}
