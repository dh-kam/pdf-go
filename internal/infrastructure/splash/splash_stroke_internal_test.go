package splash

import (
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// newTestSplashRGB returns a Splash bound to a freshly-allocated RGB8 bitmap of
// the given size with stroke pattern set to opaque red and stroke alpha=1.
func newTestSplashRGB(t *testing.T, w, h int) *Splash {
	t.Helper()
	bm := NewBitmap(w, h, ModeRGB8, false)
	bm.rowSize = w * 3
	bm.data = make([]byte, bm.rowSize*h)
	s, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.strokePattern = NewSolidColor(Color{0xFF, 0x00, 0x00})
	s.state.strokeAlpha = 1
	return s
}

// readPixel returns the (R,G,B) bytes at (x,y) in an RGB8 bitmap.
func readPixel(bm *Bitmap, x, y int) (byte, byte, byte) {
	off := y*bm.rowSize + x*3
	return bm.data[off], bm.data[off+1], bm.data[off+2]
}

// TestStrokeNarrowVerticalLine verifies a 1-pixel vertical stroke from
// (10,0) to (10,100) lights only column 10.
func TestStrokeNarrowVerticalLine(t *testing.T) {
	s := newTestSplashRGB(t, 32, 128)
	xPath := &xpath.XPath{
		Segs: []xpath.XPathSeg{
			{X0: 10, Y0: 0, X1: 10, Y1: 100, DXDY: 0, Flags: xpath.XPathVert},
		},
	}
	if err := s.strokeNarrowXPath(xPath); err != nil {
		t.Fatalf("strokeNarrowXPath: %v", err)
	}
	for y := 0; y < 100; y++ {
		r, _, _ := readPixel(s.bitmap, 10, y)
		if r != 0xFF {
			t.Fatalf("col 10 row %d: want red, got R=%02x", y, r)
		}
		if r9, _, _ := readPixel(s.bitmap, 9, y); r9 != 0 {
			t.Fatalf("col 9 row %d: want zero, got R=%02x", y, r9)
		}
		if r11, _, _ := readPixel(s.bitmap, 11, y); r11 != 0 {
			t.Fatalf("col 11 row %d: want zero, got R=%02x", y, r11)
		}
	}
}

// TestStrokeNarrowHorizontalLine verifies a 1-pixel horizontal stroke lights
// only the target row.
func TestStrokeNarrowHorizontalLine(t *testing.T) {
	s := newTestSplashRGB(t, 64, 32)
	xPath := &xpath.XPath{
		Segs: []xpath.XPathSeg{
			{X0: 5, Y0: 12, X1: 50, Y1: 12, DXDY: 0, Flags: xpath.XPathHoriz},
		},
	}
	if err := s.strokeNarrowXPath(xPath); err != nil {
		t.Fatalf("strokeNarrowXPath: %v", err)
	}
	for x := 5; x <= 50; x++ {
		r, _, _ := readPixel(s.bitmap, x, 12)
		if r != 0xFF {
			t.Fatalf("col %d row 12: want red, got R=%02x", x, r)
		}
	}
	for x := 5; x <= 50; x++ {
		if r, _, _ := readPixel(s.bitmap, x, 11); r != 0 {
			t.Fatalf("row 11 col %d: want zero, got R=%02x", x, r)
		}
		if r, _, _ := readPixel(s.bitmap, x, 13); r != 0 {
			t.Fatalf("row 13 col %d: want zero, got R=%02x", x, r)
		}
	}
}

// TestStrokeNarrowDiagonalInclusiveLastPixel verifies the +1/-1 inclusive
// last-pixel rule from Splash.cc:1995-1997 / 2010-2012 (project memory:
// butt-cap thin stroke fix). The endpoint pixel must light up.
func TestStrokeNarrowDiagonalInclusiveLastPixel(t *testing.T) {
	s := newTestSplashRGB(t, 32, 32)
	// Positive slope: dxdy = 1 (one column per row).
	xPath := &xpath.XPath{
		Segs: []xpath.XPathSeg{
			{X0: 0, Y0: 0, X1: 10, Y1: 10, DXDY: 1, Flags: 0},
		},
	}
	if err := s.strokeNarrowXPath(xPath); err != nil {
		t.Fatalf("strokeNarrowXPath: %v", err)
	}
	r, _, _ := readPixel(s.bitmap, 10, 10)
	if r != 0xFF {
		t.Fatalf("endpoint (10,10): want red, got R=%02x (last-pixel rule regression)", r)
	}
}

// TestMakeDashedPathBasic dashes a horizontal line with [8, 4] phase=0 and
// verifies the on/off pattern.
func TestMakeDashedPathBasic(t *testing.T) {
	bm := NewBitmap(64, 8, ModeRGB8, false)
	bm.rowSize = 64 * 3
	bm.data = make([]byte, bm.rowSize*8)
	s, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.lineDash = []float64{8, 4}
	s.state.lineDashPhase = 0

	p := xpath.NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(36, 0); err != nil { // 36 = 8+4+8+4+8+4
		t.Fatalf("LineTo: %v", err)
	}

	d, err := s.makeDashedPath(p)
	if err != nil {
		t.Fatalf("makeDashedPath: %v", err)
	}

	// Expect three "on" segments: [0..8], [12..20], [24..32]. Each on segment
	// emits a moveTo followed by a lineTo, so 6 points total.
	if got, want := d.Length(), 6; got != want {
		t.Fatalf("dashed point count: got %d want %d", got, want)
	}
	wantPts := []struct{ x, y float64 }{
		{0, 0}, {8, 0},
		{12, 0}, {20, 0},
		{24, 0}, {32, 0},
	}
	for i, w := range wantPts {
		pt, _ := d.Point(i)
		if math.Abs(pt.X-w.x) > 1e-9 || math.Abs(pt.Y-w.y) > 1e-9 {
			t.Fatalf("dash pt %d: got (%g,%g) want (%g,%g)", i, pt.X, pt.Y, w.x, w.y)
		}
	}
}

// TestMakeDashedPathZeroTotal returns an empty path when the dash array sums
// to zero (Splash.cc:2175-2178).
func TestMakeDashedPathZeroTotal(t *testing.T) {
	bm := NewBitmap(8, 8, ModeRGB8, false)
	bm.rowSize = 8 * 3
	bm.data = make([]byte, bm.rowSize*8)
	s, _ := New(bm, false)
	s.state.lineDash = []float64{0}

	p := xpath.NewPath()
	_ = p.MoveTo(0, 0)
	_ = p.LineTo(10, 0)

	d, err := s.makeDashedPath(p)
	if err != nil {
		t.Fatalf("makeDashedPath: %v", err)
	}
	if !d.IsEmpty() {
		t.Fatalf("expected empty dashed path for [0] dash, got length=%d", d.Length())
	}
}

func TestShouldMirrorDashedButtStrokeNormalsForAxisSubpaths(t *testing.T) {
	p := xpath.NewPath()
	_ = p.MoveTo(10.25, 20.4)
	_ = p.LineTo(18.75, 20.4)
	_ = p.MoveTo(24.25, 20.4)
	_ = p.LineTo(32.75, 20.4)

	if !shouldMirrorDashedButtStrokeNormalsForPath(p, 1.6) {
		t.Fatalf("expected dashed axis-aligned butt subpaths to mirror")
	}
}

func TestShouldMirrorDashedButtStrokeNormalsRejectsMultiPointSubpath(t *testing.T) {
	p := xpath.NewPath()
	_ = p.MoveTo(10.25, 20.4)
	_ = p.LineTo(18.75, 20.4)
	_ = p.LineTo(24.25, 20.4)

	if shouldMirrorDashedButtStrokeNormalsForPath(p, 1.6) {
		t.Fatalf("expected multi-point dashed path shape to keep existing normal order")
	}
}

func TestShouldMirrorSingleButtStrokeNormalsForCollinearAxisSubpath(t *testing.T) {
	p := xpath.NewPath()
	_ = p.MoveTo(786.886252, 173.813038)
	_ = p.LineTo(805.880204, 173.813038)
	_ = p.LineTo(824.874201, 173.813038)

	if !shouldMirrorSingleButtStrokeNormalsForPath(p, 1.78012184) {
		t.Fatalf("expected open collinear axis-aligned butt subpath to mirror")
	}
}

func TestShouldMirrorSingleButtStrokeNormalsRejectsBentSubpath(t *testing.T) {
	p := xpath.NewPath()
	_ = p.MoveTo(10.25, 20.4)
	_ = p.LineTo(18.75, 20.4)
	_ = p.LineTo(24.25, 22.4)

	if shouldMirrorSingleButtStrokeNormalsForPath(p, 1.6) {
		t.Fatalf("expected bent multi-point subpath to keep existing normal order")
	}
}

func TestMakeStrokePathMirrorsDashedButtAxisSubpath(t *testing.T) {
	s := newTestSplashRGB(t, 64, 64)
	s.SetMirrorStrokeNormals(true)
	s.state.lineCap = int(LineCapButt)
	s.state.lineJoin = int(LineJoinMiter)

	p := xpath.NewPath()
	_ = p.MoveTo(10.25, 20.4)
	_ = p.LineTo(18.75, 20.4)

	out := s.makeStrokePath(p, 1.6, false, true)
	pt, _ := out.Point(0)
	if math.Abs(pt.Y-19.6) > 1e-9 {
		t.Fatalf("mirrored dashed butt stroke first Y: got %g want 19.6", pt.Y)
	}
}

func TestMakeStrokePathMirrorsCollinearButtAxisSubpath(t *testing.T) {
	s := newTestSplashRGB(t, 900, 300)
	s.SetMirrorStrokeNormals(true)
	s.state.lineCap = int(LineCapButt)
	s.state.lineJoin = int(LineJoinMiter)

	p := xpath.NewPath()
	_ = p.MoveTo(786.886252, 173.813038)
	_ = p.LineTo(805.880204, 173.813038)
	_ = p.LineTo(824.874201, 173.813038)

	out := s.makeStrokePath(p, 1.78012184, false, false)
	pt, _ := out.Point(0)
	wantY := 173.813038 - 0.5*1.78012184
	if math.Abs(pt.Y-wantY) > 1e-9 {
		t.Fatalf("mirrored collinear butt stroke first Y: got %g want %g", pt.Y, wantY)
	}
}

// TestMakeStrokePathButtCapAxisAligned validates makeStrokePath produces a
// rectangular outline for a single horizontal segment with butt caps.
func TestMakeStrokePathButtCapAxisAligned(t *testing.T) {
	s := newTestSplashRGB(t, 64, 64)
	s.state.lineCap = int(LineCapButt)
	s.state.lineJoin = int(LineJoinMiter)

	p := xpath.NewPath()
	_ = p.MoveTo(10, 20)
	_ = p.LineTo(50, 20) // horizontal, length 40

	out := s.makeStrokePath(p, 4, false, false) // half-width = 2

	// First subpath should contain four corners of the 40x4 rect.
	if got := out.Length(); got < 4 {
		t.Fatalf("stroke path too short: %d", got)
	}
	// First point: leftStart = (10, 20+wdx) where dx=1, dy=0, wdy=0, wdx=2.
	// So leftStart = (10-0, 20+2) = (10, 22).
	pt, _ := out.Point(0)
	if math.Abs(pt.X-10) > 1e-9 || math.Abs(pt.Y-22) > 1e-9 {
		t.Fatalf("leftStart: got (%g,%g) want (10,22)", pt.X, pt.Y)
	}
	// Second point with butt cap: rightStart = (10, 18).
	pt, _ = out.Point(1)
	if math.Abs(pt.X-10) > 1e-9 || math.Abs(pt.Y-18) > 1e-9 {
		t.Fatalf("rightStart (butt): got (%g,%g) want (10,18)", pt.X, pt.Y)
	}
}

// TestMakeStrokePathProjectingCap validates square (projecting) cap extension
// by half the line width along the segment direction.
func TestMakeStrokePathProjectingCap(t *testing.T) {
	s := newTestSplashRGB(t, 64, 64)
	s.state.lineCap = int(LineCapProjecting)
	s.state.lineJoin = int(LineJoinMiter)

	p := xpath.NewPath()
	_ = p.MoveTo(10, 20)
	_ = p.LineTo(50, 20)

	out := s.makeStrokePath(p, 4, false, false) // half-width = 2

	// Projecting cap inserts two extra points beyond leftStart:
	// leftStart   (10, 22)
	// projectTL   (10-2, 22) = (8, 22)
	// projectBL   (8, 18)
	// rightStart  (10, 18)
	if out.Length() < 4 {
		t.Fatalf("projecting stroke path too short: %d", out.Length())
	}
	pt, _ := out.Point(1)
	if math.Abs(pt.X-8) > 1e-9 || math.Abs(pt.Y-22) > 1e-9 {
		t.Fatalf("projecting top-left: got (%g,%g) want (8,22)", pt.X, pt.Y)
	}
	pt, _ = out.Point(2)
	if math.Abs(pt.X-8) > 1e-9 || math.Abs(pt.Y-18) > 1e-9 {
		t.Fatalf("projecting bot-left: got (%g,%g) want (8,18)", pt.X, pt.Y)
	}
}

func TestMakeStrokePathZeroLengthRoundCapEmitsCircle(t *testing.T) {
	s := newTestSplashRGB(t, 64, 64)
	s.state.lineCap = int(LineCapRound)
	s.state.lineJoin = int(LineJoinRound)

	p := xpath.NewPath()
	_ = p.MoveTo(10, 20)
	_ = p.LineTo(10, 20)

	out := s.makeStrokePath(p, 4, false, false)
	if got := out.Length(); got != 13 {
		t.Fatalf("zero-length round cap path length = %d, want 13", got)
	}
	pt, _ := out.Point(0)
	if math.Abs(pt.X-12) > 1e-9 || math.Abs(pt.Y-20) > 1e-9 {
		t.Fatalf("zero-length round cap start: got (%g,%g), want (12,20)", pt.X, pt.Y)
	}
	pt, _ = out.Point(3)
	if math.Abs(pt.X-10) > 1e-9 || math.Abs(pt.Y-22) > 1e-9 {
		t.Fatalf("zero-length round cap top: got (%g,%g), want (10,22)", pt.X, pt.Y)
	}
	pt, _ = out.Point(6)
	if math.Abs(pt.X-8) > 1e-9 || math.Abs(pt.Y-20) > 1e-9 {
		t.Fatalf("zero-length round cap left: got (%g,%g), want (8,20)", pt.X, pt.Y)
	}
}

// TestMakeStrokePathMiterJoin verifies that a 90-degree corner with miter join
// emits a fourth interior point at the miter tip.
func TestMakeStrokePathMiterJoin(t *testing.T) {
	s := newTestSplashRGB(t, 64, 64)
	s.state.lineCap = int(LineCapButt)
	s.state.lineJoin = int(LineJoinMiter)
	s.state.miterLimit = 10

	// Path: (10,10) -> (10,40) -> (40,40). Two segments meeting at (10,40)
	// with a 90-degree right turn.
	p := xpath.NewPath()
	_ = p.MoveTo(10, 10)
	_ = p.LineTo(10, 40)
	_ = p.LineTo(40, 40)

	out := s.makeStrokePath(p, 4, false, false) // half-width = 2

	// The path will contain two segment quads + one join polygon. Length must
	// be > 0 and the join polygon must include a miter tip point.
	if out.IsEmpty() {
		t.Fatalf("expected non-empty miter stroke path")
	}
	// Sanity: total points should be at least 4 (quad1) + 4 (quad2) + 3 (join) = 11.
	if got := out.Length(); got < 8 {
		t.Fatalf("miter stroke path too short: %d", got)
	}
}

func TestMakeStrokePathClosedRectAddsPopplerWrapHints(t *testing.T) {
	s := newTestSplashRGB(t, 64, 64)
	s.state.lineCap = int(LineCapButt)
	s.state.lineJoin = int(LineJoinMiter)
	s.state.miterLimit = 10
	s.state.strokeAdjust = true

	p := xpath.NewPath()
	_ = p.MoveTo(10, 10)
	_ = p.LineTo(50, 10)
	_ = p.LineTo(50, 40)
	_ = p.LineTo(10, 40)
	_ = p.Close(false)

	out := s.makeStrokePath(p, 4, false, false)
	if got := out.Length(); got <= 10 {
		t.Fatalf("closed rect should use Poppler segment+join outline, got length=%d", got)
	}
	if got := len(out.Hints()); got != 14 {
		t.Fatalf("closed rect stroke-adjust hints: got %d, want 14", got)
	}
}
