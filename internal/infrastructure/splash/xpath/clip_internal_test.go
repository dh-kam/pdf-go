package xpath

import (
	"errors"
	"testing"
)

// identityMatrix is the 2x3 affine identity used in clip-to-path tests
// (SplashCoord matrix layout, SplashXPath.cc:54-61).
var identityMatrix = [6]float64{1, 0, 0, 1, 0, 0}

// TestNewClipUnbounded verifies that a fresh clip equals the hard rectangle
// (SplashClip ctor at SplashClip.cc:46-69).
func TestClipNewUnbounded(t *testing.T) {
	c := NewClip(0, 0, 100, 100, false)
	if got := c.TestRect(0, 0, 100, 100); got != ClipAllInside {
		t.Errorf("TestRect(full-bounds) = %v, want ClipAllInside", got)
	}
	if got := c.TestRect(10, 10, 20, 20); got != ClipAllInside {
		t.Errorf("TestRect(inner) = %v, want ClipAllInside", got)
	}
	// Outside the hard bounds → AllOutside.
	if got := c.TestRect(200, 200, 300, 300); got != ClipAllOutside {
		t.Errorf("TestRect(far-outside) = %v, want ClipAllOutside", got)
	}
}

// TestResetToRect verifies destructive reset semantics
// (SplashClip::resetToRect, SplashClip.cc:111-136).
func TestClipResetToRect(t *testing.T) {
	c := NewClip(0, 0, 100, 100, false)
	c.ResetToRect(20, 20, 80, 80)

	// Fully inside — AllInside.
	if got := c.TestRect(50, 50, 60, 60); got != ClipAllInside {
		t.Errorf("TestRect(50,50,60,60) = %v, want ClipAllInside", got)
	}
	// Fully outside — AllOutside.
	if got := c.TestRect(0, 0, 10, 10); got != ClipAllOutside {
		t.Errorf("TestRect(0,0,10,10) = %v, want ClipAllOutside", got)
	}
	// Straddling the boundary — Partial.
	if got := c.TestRect(10, 10, 30, 30); got != ClipPartial {
		t.Errorf("TestRect(10,10,30,30) = %v, want ClipPartial", got)
	}
}

// TestClipToRectIntersect verifies that ClipToRect shrinks but never expands
// the bounds (SplashClip::clipToRect, SplashClip.cc:138-179).
func TestClipToRectIntersect(t *testing.T) {
	c := NewClip(0, 0, 100, 100, false)
	c.ResetToRect(20, 20, 80, 80)

	if err := c.ClipToRect(40, 40, 60, 60); err != nil {
		t.Fatalf("ClipToRect: unexpected error %v", err)
	}
	// Fully inside the new bounds.
	if got := c.TestRect(45, 45, 55, 55); got != ClipAllInside {
		t.Errorf("post-intersect AllInside = %v", got)
	}
	// Inside the old bounds but outside the new bounds.
	if got := c.TestRect(25, 25, 30, 30); got != ClipAllOutside {
		t.Errorf("post-intersect AllOutside = %v", got)
	}
}

// TestClipToRectEmpty verifies the empty-collapse sentinel
// (SplashClip.cc:138-179 + xMax<xMin sentinel from SplashClip.cc:189).
func TestClipToRectEmpty(t *testing.T) {
	c := NewClip(0, 0, 100, 100, false)
	c.ResetToRect(20, 20, 30, 30)

	// Disjoint rectangles → empty clip.
	err := c.ClipToRect(50, 50, 60, 60)
	if err == nil {
		t.Fatalf("ClipToRect: expected empty-clip error, got nil")
	}
	if !errors.Is(err, errEmptyClip) {
		t.Fatalf("ClipToRect: error = %v, want errEmptyClip", err)
	}

	// Empty clip → every TestRect returns AllOutside.
	if got := c.TestRect(50, 50, 60, 60); got != ClipAllOutside {
		t.Errorf("empty-clip TestRect(50,50,60,60) = %v, want ClipAllOutside", got)
	}
	if got := c.TestRect(0, 0, 100, 100); got != ClipAllOutside {
		t.Errorf("empty-clip TestRect(full) = %v, want ClipAllOutside", got)
	}
}

// buildAxisAlignedRectPath constructs a 4-segment closed axis-aligned
// rectangle path (the shape that triggers the SplashClip.cc:195-202 fast path).
func buildAxisAlignedRectPath(t *testing.T, x0, y0, x1, y1 float64) *Path {
	t.Helper()
	p := NewPath()
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
	if err := p.Close(true); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return p
}

// TestClipToPathAxisAlignedRectFastPath verifies that a path matching the
// 4-segment axis-aligned rectangle pattern bypasses scanner construction and
// behaves identically to a direct ClipToRect call (SplashClip.cc:195-202).
func TestClipToPathAxisAlignedRectFastPath(t *testing.T) {
	c := NewClip(0, 0, 100, 100, false)
	p := buildAxisAlignedRectPath(t, 20, 30, 70, 80)

	if err := c.ClipToPath(p, identityMatrix, 1.0, false); err != nil {
		t.Fatalf("ClipToPath: %v", err)
	}
	if len(c.scanners) != 0 {
		t.Errorf("axis-aligned fast path pushed %d scanner(s); want 0", len(c.scanners))
	}

	// Compare against a peer clip built with the same bounds via ClipToRect.
	ref := NewClip(0, 0, 100, 100, false)
	if err := ref.ClipToRect(20, 30, 70, 80); err != nil {
		t.Fatalf("ref ClipToRect: %v", err)
	}
	if c.xMinFP != ref.xMinFP || c.xMaxFP != ref.xMaxFP ||
		c.yMinFP != ref.yMinFP || c.yMaxFP != ref.yMaxFP {
		t.Errorf("fast-path bounds (%v,%v,%v,%v) differ from ClipToRect (%v,%v,%v,%v)",
			c.xMinFP, c.yMinFP, c.xMaxFP, c.yMaxFP,
			ref.xMinFP, ref.yMinFP, ref.xMaxFP, ref.yMaxFP)
	}
	if c.xMin != ref.xMin || c.xMax != ref.xMax || c.yMin != ref.yMin || c.yMax != ref.yMax {
		t.Errorf("fast-path int bounds differ from ClipToRect")
	}
}

// TestClipToPathComplexPath drives ClipToPath with a non-rectangular triangle
// path, expecting a scanner to be appended. The full inside/outside/edge
// classification is deferred (Scanner.TestSpan / NewScanner are Dev1's work);
// we assert the structural invariants that don't require a scanner impl.
func TestClipToPathComplexPath(t *testing.T) {
	if !scannerCtorAvailable() {
		t.Skip("Scanner not yet ported (Dev1 Phase 2 dependency)")
	}
	c := NewClip(0, 0, 100, 100, false)
	p := NewPath()
	_ = p.MoveTo(10, 10)
	_ = p.LineTo(90, 10)
	_ = p.LineTo(50, 90)
	_ = p.Close(true)

	if err := c.ClipToPath(p, identityMatrix, 1.0, false); err != nil {
		t.Fatalf("ClipToPath: %v", err)
	}
	if len(c.scanners) != 1 {
		t.Fatalf("triangle path: scanners = %d, want 1", len(c.scanners))
	}
	if len(c.eo) != 1 || c.eo[0] != false {
		t.Errorf("triangle path: eo = %v, want [false]", c.eo)
	}
	if len(c.flags) != 1 || c.flags[0] != 0 {
		t.Errorf("triangle path: flags = %v, want [0]", c.flags)
	}

	// With a path scanner present, TestRect can never report AllInside even
	// for a region that's geometrically inside the rect bounds — the scanner
	// list forces Partial (SplashClip.cc:236).
	if got := c.TestRect(40, 30, 60, 50); got != ClipPartial {
		t.Errorf("TestRect with scanner = %v, want ClipPartial", got)
	}
	if got := c.TestSpan(45, 55, 20); got != ClipAllInside {
		t.Errorf("TestSpan fully inside scanner = %v, want ClipAllInside", got)
	}
	if got := c.TestSpan(5, 15, 20); got != ClipPartial {
		t.Errorf("TestSpan crossing scanner edge = %v, want ClipPartial", got)
	}
	// Far outside the rect bounds → AllOutside (cheap rect-only test).
	if got := c.TestRect(200, 200, 300, 300); got != ClipAllOutside {
		t.Errorf("TestRect far-outside = %v, want ClipAllOutside", got)
	}
}

func TestClipEffectiveBoundsIncludesPathScannerBounds(t *testing.T) {
	if !scannerCtorAvailable() {
		t.Skip("Scanner not yet ported")
	}
	c := NewClip(0, 0, 100, 100, true)
	p := NewPath()
	_ = p.MoveTo(20.25, 30.25)
	_ = p.LineTo(60.75, 30.25)
	_ = p.LineTo(40.0, 70.75)
	_ = p.Close(true)
	c.IntersectVectorBounds(20.25, 30.25, 60.75, 70.75)

	if err := c.ClipToPath(p, identityMatrix, 1.0, false); err != nil {
		t.Fatalf("ClipToPath: %v", err)
	}
	xMin, yMin, xMax, yMax, ok := c.EffectiveBounds()
	if !ok {
		t.Fatalf("EffectiveBounds returned empty")
	}
	if xMin != 20.25 || yMin != 30.25 || xMax != 61 || yMax != 71 {
		t.Fatalf("EffectiveBounds = (%.2f,%.2f)-(%.2f,%.2f), want (20.25,30.25)-(61,71)", xMin, yMin, xMax, yMax)
	}
	vxMin, vyMin, vxMax, vyMax, ok := c.VectorEffectiveBounds()
	if !ok {
		t.Fatalf("VectorEffectiveBounds returned empty")
	}
	if vxMin != 20.25 || vyMin != 30.25 || vxMax != 60.75 || vyMax != 70.75 {
		t.Fatalf("VectorEffectiveBounds = (%.2f,%.2f)-(%.2f,%.2f), want (20.25,30.25)-(60.75,70.75)", vxMin, vyMin, vxMax, vyMax)
	}
}

// TestCloneIndependence verifies the SplashClip::SplashClip(const SplashClip*)
// contract (SplashClip.cc:71-91): bounds and flags are deep-copied so each
// clip can mutate independently, but scanners are shared (immutable post-
// construction — see 02_api_design.md §6).
func TestClipCloneIndependence(t *testing.T) {
	a := NewClip(0, 0, 100, 100, false)
	a.ResetToRect(10, 10, 90, 90)
	// Inject a path-clip so we have a scanner to check sharing.
	if scannerCtorAvailable() {
		p := NewPath()
		_ = p.MoveTo(10, 10)
		_ = p.LineTo(90, 10)
		_ = p.LineTo(50, 90)
		_ = p.Close(true)
		if err := a.ClipToPath(p, identityMatrix, 1.0, false); err != nil {
			t.Fatalf("seed ClipToPath: %v", err)
		}
	}

	b := a.Clone()

	// Mutating A's bounds must not change B's bounds.
	if err := a.ClipToRect(20, 20, 80, 80); err != nil {
		t.Fatalf("ClipToRect on A: %v", err)
	}
	if b.xMinFP != 10 || b.xMaxFP != 90 || b.yMinFP != 10 || b.yMaxFP != 90 {
		t.Errorf("Clone bounds were aliased: B = (%v,%v,%v,%v); want (10,10,90,90)",
			b.xMinFP, b.yMinFP, b.xMaxFP, b.yMaxFP)
	}

	// Scanners are shared (when present): the underlying *Scanner pointers
	// point to the same object even though the slice headers are independent.
	if len(a.scanners) > 0 && len(b.scanners) > 0 {
		if a.scanners[0] != b.scanners[0] {
			t.Errorf("Clone broke scanner sharing: a[0]=%p, b[0]=%p",
				a.scanners[0], b.scanners[0])
		}
	}

	// Mutating A's eo slice must not affect B (slice header is deep-copied).
	if len(a.eo) > 0 {
		a.eo[0] = !a.eo[0]
		if len(b.eo) > 0 && b.eo[0] == a.eo[0] {
			t.Errorf("Clone aliased eo slice")
		}
	}
}

// TestClipToPathEoFlag verifies that the eo argument is recorded in the eo
// slice and as splashClipEO in the flags byte (SplashClip.cc:210).
func TestClipToPathEoFlag(t *testing.T) {
	if !scannerCtorAvailable() {
		t.Skip("Scanner not yet ported (Dev1 Phase 2 dependency)")
	}
	c := NewClip(0, 0, 100, 100, false)
	p := NewPath()
	_ = p.MoveTo(10, 10)
	_ = p.LineTo(90, 10)
	_ = p.LineTo(50, 90)
	_ = p.Close(true)

	if err := c.ClipToPath(p, identityMatrix, 1.0, true); err != nil {
		t.Fatalf("ClipToPath eo=true: %v", err)
	}
	if len(c.eo) != 1 || c.eo[0] != true {
		t.Errorf("eo slice = %v, want [true]", c.eo)
	}
	if len(c.flags) != 1 || c.flags[0]&splashClipEO == 0 {
		t.Errorf("flags[0] = %#x, want splashClipEO bit set", c.flags[0])
	}
}

func TestClipToPathETicketRoundedFrameRemovesSideStripAA(t *testing.T) {
	const (
		scale        = 1240.0 / 595.0
		bitmapWidth  = 1240
		bitmapHeight = 1755
	)
	pageMatrix := [6]float64{scale, 0, 0, -scale, 0, bitmapHeight}
	c := NewClip(0, 0, bitmapWidth-1, bitmapHeight-1, true)
	for _, path := range buildETicketRoundedFrameClipPaths(t, 526.35, 806) {
		if err := c.ClipToPath(path, pageMatrix, 1.0, false); err != nil {
			t.Fatalf("ClipToPath: %v", err)
		}
	}

	tests := []struct {
		name string
		path *Path
		x    int
		y    int
	}{
		{name: "top strip", path: mustPath(t, []pathOp{
			{op: "m", x: 51, y: 803}, {op: "l", x: 544, y: 803}, {op: "l", x: 547, y: 806}, {op: "l", x: 48, y: 806}, {op: "l", x: 51, y: 803},
		}), x: 250, y: 80},
		{name: "right strip", path: mustPath(t, []pathOp{
			{op: "m", x: 544, y: 803}, {op: "l", x: 544, y: 529.35}, {op: "l", x: 547, y: 526.35}, {op: "l", x: 547, y: 806}, {op: "l", x: 544, y: 803},
		}), x: 1136, y: 295},
		{name: "bottom strip", path: mustPath(t, []pathOp{
			{op: "m", x: 544, y: 529.35}, {op: "l", x: 51, y: 529.35}, {op: "l", x: 48, y: 526.35}, {op: "l", x: 547, y: 526.35}, {op: "l", x: 544, y: 529.35},
		}), x: 250, y: 653},
		{name: "left strip", path: mustPath(t, []pathOp{
			{op: "m", x: 51, y: 529.35}, {op: "l", x: 51, y: 803}, {op: "l", x: 48, y: 806}, {op: "l", x: 48, y: 526.35}, {op: "l", x: 51, y: 529.35},
		}), x: 102, y: 295},
	}
	for _, tt := range tests {
		before, got := clippedPixelShape(tt.path, pageMatrix, c, tt.x, tt.y, bitmapWidth, bitmapHeight)
		if before == 0 {
			t.Errorf("%s unclipped shape at (%d,%d) = 0, test fixture is invalid", tt.name, tt.x, tt.y)
		}
		if got != 0 {
			t.Errorf("%s clipped shape at (%d,%d) = %d, want clipped out (unclipped=%d)", tt.name, tt.x, tt.y, got, before)
		}
	}
}

func TestClipAALineFullWidthReturnsRectAdjustedBounds(t *testing.T) {
	const bitmapWidth = 20
	rowSize := (bitmapWidth*aaSize + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	for i := range aaBuf {
		aaBuf[i] = 0xff
	}

	clip := NewClip(0, 0, bitmapWidth-1, 10, true)
	if err := clip.ClipToRect(5.25, 0, 12.75, 10); err != nil {
		t.Fatalf("ClipToRect: %v", err)
	}

	gotX0, gotX1 := clip.ClipAALineFullWidth(1, aaBuf, 0, bitmapWidth-1, bitmapWidth)
	if gotX0 != 5 || gotX1 != 12 {
		t.Fatalf("adjusted bounds = (%d,%d), want (5,12)", gotX0, gotX1)
	}
	if got := countFullWidthAABufPixel(aaBuf, rowSize, 4); got != 0 {
		t.Fatalf("left-clipped pixel shape = %d, want 0", got)
	}
	if got := countFullWidthAABufPixel(aaBuf, rowSize, 13); got != 0 {
		t.Fatalf("right-clipped pixel shape = %d, want 0", got)
	}
}

func TestClipAALineFullWidthUsesPopplerLeftByteBoundaryClear(t *testing.T) {
	const bitmapWidth = 6
	rowSize := (bitmapWidth*aaSize + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	for i := range aaBuf {
		aaBuf[i] = 0xff
	}

	clip := NewClip(0, 0, bitmapWidth-1, 10, true)
	if err := clip.ClipToRect(1.25, 0, bitmapWidth, 10); err != nil {
		t.Fatalf("ClipToRect: %v", err)
	}

	gotX0, _ := clip.ClipAALineFullWidth(1, aaBuf, 1, bitmapWidth-1, bitmapWidth)
	if gotX0 != 1 {
		t.Fatalf("adjusted x0 = %d, want 1", gotX0)
	}
	for yy := 0; yy < aaSize; yy++ {
		if got := aaBuf[yy*rowSize]; got != 0x07 {
			t.Fatalf("row %d byte 0 = 0x%02x, want Poppler left mask 0x07", yy, got)
		}
	}
}

func TestClipAALineFullWidthUsesPopplerRightWholeByteClear(t *testing.T) {
	const bitmapWidth = 6
	rowSize := (bitmapWidth*aaSize + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	for i := range aaBuf {
		aaBuf[i] = 0xff
	}

	clip := NewClip(0, 0, bitmapWidth-1, 10, true)
	if err := clip.ClipToRect(0, 0, 1.25, 10); err != nil {
		t.Fatalf("ClipToRect: %v", err)
	}

	_, gotX1 := clip.ClipAALineFullWidth(1, aaBuf, 0, 2, bitmapWidth)
	if gotX1 != 1 {
		t.Fatalf("adjusted x1 = %d, want 1", gotX1)
	}
	for yy := 0; yy < aaSize; yy++ {
		rowOff := yy * rowSize
		if got := aaBuf[rowOff]; got != 0xfc {
			t.Fatalf("row %d byte 0 = 0x%02x, want Poppler right leading mask 0xfc", yy, got)
		}
		if got := aaBuf[rowOff+1]; got != 0x00 {
			t.Fatalf("row %d byte 1 = 0x%02x, want Poppler right whole-byte clear", yy, got)
		}
	}
}

func TestClipToPathSnapsNearIntegerYBoundaryForClipScanners(t *testing.T) {
	const nearIntegerY = 2.0 - 1e-13
	leakyRow := splashFloor(nearIntegerY * aaSize)
	snappedRow := leakyRow + 1

	t.Setenv("PDF_DISABLE_SPLASH_CLIP_SCANNER_Y_FLOOR_SNAP", "0")
	t.Setenv("PDF_DEBUG_SPLASH_CLIP_SCANNER_Y_FLOOR_SNAP_EPS", "")
	scanner := clipScannerForNearIntegerYBoundary(t, nearIntegerY)
	if got := scannerIntersectionsAtRow(scanner, leakyRow); got != 0 {
		t.Fatalf("default clip scanner row %d intersections = %d, want 0", leakyRow, got)
	}
	if got := scannerIntersectionsAtRow(scanner, snappedRow); got == 0 {
		t.Fatalf("default clip scanner row %d intersections = 0, want snapped boundary intersections", snappedRow)
	}

	t.Setenv("PDF_DISABLE_SPLASH_CLIP_SCANNER_Y_FLOOR_SNAP", "1")
	unsnapped := clipScannerForNearIntegerYBoundary(t, nearIntegerY)
	if got := scannerIntersectionsAtRow(unsnapped, leakyRow); got == 0 {
		t.Fatalf("disabled clip scanner row %d intersections = 0, want old floor-row intersections", leakyRow)
	}
}

func clipScannerForNearIntegerYBoundary(t *testing.T, y float64) *Scanner {
	t.Helper()
	clip := NewClip(0, 0, 64, 64, true)
	path := mustPath(t, []pathOp{
		{op: "m", x: 10, y: y},
		{op: "l", x: 30, y: y},
		{op: "l", x: 20, y: 10},
	})
	_ = path.Close(true)
	if err := clip.ClipToPath(path, identityMatrix, 1.0, false); err != nil {
		t.Fatalf("ClipToPath: %v", err)
	}
	if len(clip.scanners) != 1 {
		t.Fatalf("clip scanners = %d, want 1", len(clip.scanners))
	}
	return clip.scanners[0]
}

func scannerIntersectionsAtRow(scanner *Scanner, row int) int {
	if scanner == nil || row < scanner.yMin || row > scanner.yMax {
		return 0
	}
	return len(scanner.allIntersections[row-scanner.yMin])
}

func clippedPixelShape(path *Path, matrix [6]float64, clip *Clip, x, y, bitmapWidth, bitmapHeight int) (int, int) {
	xPath := NewXPath(path, matrix, 1.0, true)
	xPath.AAScale()
	xPath.Sort()
	scanner := NewScanner(xPath, false, 0, 0, bitmapWidth*aaSize, bitmapHeight*aaSize)
	rowSize := ((bitmapWidth * aaSize) + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)
	x0, x1 := scanner.RenderAALineFullWidth(y, aaBuf, bitmapWidth)
	before := countFullWidthAABufPixel(aaBuf, rowSize, x)
	_, _ = clip.ClipAALineFullWidth(y, aaBuf, x0, x1, bitmapWidth)
	return before, countFullWidthAABufPixel(aaBuf, rowSize, x)
}

func buildETicketRoundedFrameClipPaths(t *testing.T, yMin, yMax float64) []*Path {
	t.Helper()
	return []*Path{
		mustPath(t, []pathOp{
			{op: "m", x: 48, y: yMin},
			{op: "l", x: 48, y: yMax - 3},
			{op: "c", x1: 48, y1: yMax - 1.66, x2: 49.66, y2: yMax, x: 51, y: yMax},
			{op: "l", x: 547, y: yMax},
			{op: "l", x: 547, y: yMin},
			{op: "l", x: 48, y: yMin},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 48, y: yMax},
			{op: "l", x: 544, y: yMax},
			{op: "c", x1: 545.34, y1: yMax, x2: 547, y2: yMax - 1.66, x: 547, y: yMax - 3},
			{op: "l", x: 547, y: yMin},
			{op: "l", x: 48, y: yMin},
			{op: "l", x: 48, y: yMax},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 547, y: yMax},
			{op: "l", x: 547, y: yMin + 3},
			{op: "c", x1: 547, y1: yMin + 1.65, x2: 545.34, y2: yMin, x: 544, y: yMin},
			{op: "l", x: 48, y: yMin},
			{op: "l", x: 48, y: yMax},
			{op: "l", x: 547, y: yMax},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 547, y: yMin},
			{op: "l", x: 51, y: yMin},
			{op: "c", x1: 49.66, y1: yMin, x2: 48, y2: yMin + 1.65, x: 48, y: yMin + 3},
			{op: "l", x: 48, y: yMax},
			{op: "l", x: 547, y: yMax},
			{op: "l", x: 547, y: yMin},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 48.75, y: yMax - 3},
			{op: "c", x1: 48.75, y1: yMax - 1.99, x2: 49.99, y2: yMax - 0.75, x: 51, y: yMax - 0.75},
			{op: "l", x: 544, y: yMax - 0.75},
			{op: "l", x: 546.25, y: yMax - 3},
			{op: "l", x: 546.25, y: yMin + 3},
			{op: "l", x: 544, y: yMin + 0.75},
			{op: "l", x: 51, y: yMin + 0.75},
			{op: "l", x: 48.75, y: yMin + 3},
			{op: "l", x: 48.75, y: yMax - 3},
			{op: "l", x: 48, y: yMax - 3},
			{op: "l", x: 48, y: yMin},
			{op: "l", x: 547, y: yMin},
			{op: "l", x: 547, y: yMax},
			{op: "l", x: 48, y: yMax},
			{op: "l", x: 48, y: yMax - 3},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 544, y: yMax - 0.75},
			{op: "c", x1: 545.01, y1: yMax - 0.75, x2: 546.25, y2: yMax - 1.99, x: 546.25, y: yMax - 3},
			{op: "l", x: 546.25, y: yMin + 3},
			{op: "l", x: 544, y: yMin + 0.75},
			{op: "l", x: 51, y: yMin + 0.75},
			{op: "l", x: 48.75, y: yMin + 3},
			{op: "l", x: 48.75, y: yMax - 3},
			{op: "l", x: 51, y: yMax - 0.75},
			{op: "l", x: 544, y: yMax - 0.75},
			{op: "l", x: 544, y: yMax},
			{op: "l", x: 48, y: yMax},
			{op: "l", x: 48, y: yMin},
			{op: "l", x: 547, y: yMin},
			{op: "l", x: 547, y: yMax},
			{op: "l", x: 544, y: yMax},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 546.25, y: yMin + 3},
			{op: "c", x1: 546.25, y1: yMin + 1.99, x2: 545.01, y2: yMin + 0.75, x: 544, y: yMin + 0.75},
			{op: "l", x: 51, y: yMin + 0.75},
			{op: "l", x: 48.75, y: yMin + 3},
			{op: "l", x: 48.75, y: yMax - 3},
			{op: "l", x: 51, y: yMax - 0.75},
			{op: "l", x: 544, y: yMax - 0.75},
			{op: "l", x: 546.25, y: yMax - 3},
			{op: "l", x: 546.25, y: yMin + 3},
			{op: "l", x: 547, y: yMin + 3},
			{op: "l", x: 547, y: yMax},
			{op: "l", x: 48, y: yMax},
			{op: "l", x: 48, y: yMin},
			{op: "l", x: 547, y: yMin},
			{op: "l", x: 547, y: yMin + 3},
		}),
		mustPath(t, []pathOp{
			{op: "m", x: 51, y: yMin + 0.75},
			{op: "c", x1: 49.99, y1: yMin + 0.75, x2: 48.75, y2: yMin + 1.99, x: 48.75, y: yMin + 3},
			{op: "l", x: 48.75, y: yMax - 3},
			{op: "l", x: 51, y: yMax - 0.75},
			{op: "l", x: 544, y: yMax - 0.75},
			{op: "l", x: 546.25, y: yMax - 3},
			{op: "l", x: 546.25, y: yMin + 3},
			{op: "l", x: 544, y: yMin + 0.75},
			{op: "l", x: 51, y: yMin + 0.75},
			{op: "l", x: 51, y: yMin},
			{op: "l", x: 547, y: yMin},
			{op: "l", x: 547, y: yMax},
			{op: "l", x: 48, y: yMax},
			{op: "l", x: 48, y: yMin},
			{op: "l", x: 51, y: yMin},
		}),
	}
}

type pathOp struct {
	op     string
	x, y   float64
	x1, y1 float64
	x2, y2 float64
}

func mustPath(t *testing.T, ops []pathOp) *Path {
	t.Helper()
	p := NewPath()
	for _, op := range ops {
		var err error
		switch op.op {
		case "m":
			err = p.MoveTo(op.x, op.y)
		case "l":
			err = p.LineTo(op.x, op.y)
		case "c":
			err = p.CurveTo(op.x1, op.y1, op.x2, op.y2, op.x, op.y)
		default:
			t.Fatalf("unknown path op %q", op.op)
		}
		if err != nil {
			t.Fatalf("%s: %v", op.op, err)
		}
	}
	return p
}

// scannerCtorAvailable reports whether Dev1's Phase 2 NewScanner has been
// landed in a usable form. The Phase 0 skeleton returns &Scanner{} with no
// fields populated, so we cannot exercise scanner-aware paths until then.
//
// We detect a "real" implementation by constructing a minimal XPath and
// checking that NewScanner returns a non-nil scanner with at least one of
// the bbox fields wired up. This is heuristic but adequate for skip-gating.
func scannerCtorAvailable() bool {
	x := &XPath{}
	x.addSegment(0, 0, 10, 10)
	x.Sort()
	s := NewScanner(x, false, 0, 0, 10, 10)
	if s == nil {
		return false
	}
	xMin, yMin, xMax, yMax := s.BBox()
	// Phase 0 skeleton returns all zeros; a real impl will reflect at least
	// the (0,0,10,10) box we just passed in.
	return !(xMin == 0 && yMin == 0 && xMax == 0 && yMax == 0)
}
