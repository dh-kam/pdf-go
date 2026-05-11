package splash

import (
	"errors"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// newTestFillSplashRGB returns a Splash bound to a freshly-allocated RGB8
// bitmap (no alpha plane) with the fill pattern set to opaque red and full
// fill alpha. vectorAA selects the AA pipeline.
func newTestFillSplashRGB(t *testing.T, w, h int, vectorAA bool) *Splash {
	t.Helper()
	bm := NewBitmap(w, h, ModeRGB8, false)
	bm.rowSize = w * 3
	bm.data = make([]byte, bm.rowSize*h)
	s, err := New(bm, vectorAA)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.fillPattern = NewSolidColor(Color{0xFF, 0x00, 0x00})
	s.state.fillAlpha = 1
	return s
}

// rectPath builds a closed axis-aligned rectangle path in user-space.
// Order: (x0,y0)→(x1,y0)→(x1,y1)→(x0,y1)→close.
func rectPath(t *testing.T, x0, y0, x1, y1 float64) *xpath.Path {
	t.Helper()
	p := xpath.NewPath()
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
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return p
}

// readPx returns the (R,G,B) triplet at (x,y) in an RGB8 bitmap.
func readPx(bm *Bitmap, x, y int) (byte, byte, byte) {
	off := y*bm.rowSize + x*3
	return bm.data[off], bm.data[off+1], bm.data[off+2]
}

// TestSplashFillAxisAlignedRectInteger fills an integer-aligned rect under
// the AA pipeline and verifies interior pixels are fully red and exterior
// pixels are paper white (zero).
func TestSplashFillAxisAlignedRectInteger(t *testing.T) {
	s := newTestFillSplashRGB(t, 64, 48, true)
	p := rectPath(t, 10, 10, 50, 30)
	if err := s.Fill(p, false); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	// Interior pixels: well inside [10,50)×[10,30) should be fully red.
	for y := 12; y < 28; y++ {
		for x := 12; x < 48; x++ {
			r, g, b := readPx(s.bitmap, x, y)
			if r != 0xFF || g != 0 || b != 0 {
				t.Fatalf("(%d,%d) interior: got %02x %02x %02x, want FF 00 00", x, y, r, g, b)
			}
		}
	}
	// Exterior pixels above row 10 should be untouched.
	for x := 0; x < 64; x++ {
		r, _, _ := readPx(s.bitmap, x, 5)
		if r != 0 {
			t.Fatalf("(%d,5) exterior: got R=%02x, want 0", x, r)
		}
	}
}

// TestSplashFillEmptyPath verifies an empty path returns ErrEmptyPath.
func TestSplashFillEmptyPath(t *testing.T) {
	s := newTestFillSplashRGB(t, 8, 8, true)
	p := xpath.NewPath()
	if err := s.Fill(p, false); !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("Fill empty: got %v, want ErrEmptyPath", err)
	}
}

// TestSplashFillNoAA fills a rect with vectorAA=false (drawSpan path) and
// confirms the interior is solid red.
func TestSplashFillNoAA(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, false)
	p := rectPath(t, 4, 4, 20, 20)
	if err := s.Fill(p, false); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	// Interior should be opaque red.
	r, g, b := readPx(s.bitmap, 10, 10)
	if r != 0xFF || g != 0 || b != 0 {
		t.Fatalf("noAA interior (10,10): got %02x %02x %02x", r, g, b)
	}
	// Far exterior untouched.
	r, _, _ = readPx(s.bitmap, 25, 25)
	if r != 0 {
		t.Fatalf("noAA exterior (25,25): got R=%02x", r)
	}
}

func TestSplashFillNoAAPartialPathClipTestsPixels(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, false)
	clip := xpath.NewPath()
	if err := clip.MoveTo(4, 4); err != nil {
		t.Fatalf("clip MoveTo: %v", err)
	}
	if err := clip.LineTo(20, 4); err != nil {
		t.Fatalf("clip LineTo: %v", err)
	}
	if err := clip.LineTo(4, 20); err != nil {
		t.Fatalf("clip LineTo: %v", err)
	}
	if err := clip.Close(false); err != nil {
		t.Fatalf("clip Close: %v", err)
	}
	if err := s.ClipToPath(clip, false); err != nil {
		t.Fatalf("ClipToPath: %v", err)
	}

	p := rectPath(t, 4, 4, 20, 20)
	if err := s.Fill(p, false); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if r, g, b := readPx(s.bitmap, 6, 6); r != 0xFF || g != 0 || b != 0 {
		t.Fatalf("inside clipped triangle: got %02x %02x %02x, want FF 00 00", r, g, b)
	}
	if r, g, b := readPx(s.bitmap, 18, 18); r != 0 || g != 0 || b != 0 {
		t.Fatalf("outside clipped triangle: got %02x %02x %02x, want untouched", r, g, b)
	}
}

// TestSplashFillSubPixelRectAA verifies that a rect with sub-pixel offsets
// produces partial-coverage edge pixels and fully covered interior pixels.
// Coverage is observed via the alpha plane (the colour channel is the source
// colour regardless of shape when dst=0).
func TestSplashFillSubPixelRectAA(t *testing.T) {
	bm := NewBitmap(64, 48, ModeRGB8, false)
	bm.rowSize = 64 * 3
	bm.data = make([]byte, bm.rowSize*48)
	bm.alpha = make([]byte, 64*48) // alpha plane lets us read coverage
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.fillPattern = NewSolidColor(Color{0xFF, 0x00, 0x00})
	s.state.fillAlpha = 1

	p := rectPath(t, 10.5, 10.5, 50.5, 30.5)
	if err := s.Fill(p, false); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	// Deep interior at (20,20) should be fully red and fully opaque.
	r, g, b := readPx(s.bitmap, 20, 20)
	if r != 0xFF || g != 0 || b != 0 {
		t.Fatalf("interior (20,20): got %02x %02x %02x, want full red", r, g, b)
	}
	if a := bm.alpha[20*64+20]; a != 0xFF {
		t.Fatalf("interior alpha (20,20): got %02x, want FF", a)
	}
	// The edge pixel at (10,10) sits at the rect's NW corner (10.5,10.5).
	// Coverage there is one quadrant of sub-cells (~4/16) so the alpha
	// channel must report partial coverage.
	aEdge := bm.alpha[10*64+10]
	if aEdge == 0 || aEdge == 0xFF {
		t.Fatalf("edge alpha (10,10): got %02x, want partial coverage", aEdge)
	}
}

// TestSplashFillEvenOddHole confirms the even-odd rule produces a donut: an
// outer rect with a smaller inner rect (in any winding) leaves the interior
// of the inner rect as a hole.
func TestSplashFillEvenOddHole(t *testing.T) {
	s := newTestFillSplashRGB(t, 64, 64, true)

	p := xpath.NewPath()
	// Outer: (0,0)→(60,0)→(60,60)→(0,60)→close.
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("%v", err)
	}
	_ = p.LineTo(60, 0)
	_ = p.LineTo(60, 60)
	_ = p.LineTo(0, 60)
	_ = p.Close(false)
	// Inner: (15,15)→(45,15)→(45,45)→(15,45)→close — same winding, EO rule
	// flips inside/outside to produce the hole.
	if err := p.MoveTo(15, 15); err != nil {
		t.Fatalf("%v", err)
	}
	_ = p.LineTo(45, 15)
	_ = p.LineTo(45, 45)
	_ = p.LineTo(15, 45)
	_ = p.Close(false)

	if err := s.Fill(p, true); err != nil {
		t.Fatalf("Fill EO: %v", err)
	}
	// Donut wall at (5,30) should be red.
	r, _, _ := readPx(s.bitmap, 5, 30)
	if r != 0xFF {
		t.Fatalf("wall (5,30): got R=%02x, want FF", r)
	}
	// Hole centre (30,30) should be paper.
	r, _, _ = readPx(s.bitmap, 30, 30)
	if r != 0 {
		t.Fatalf("hole (30,30): got R=%02x, want 0", r)
	}
}

// TestSplashFillStrokeAdjustHints verifies that a 4-pt rect with strokeAdjust
// enabled has the stroke-adjust hints injected by maybeInjectFillRectHints.
func TestSplashFillStrokeAdjustHints(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, true)
	s.state.strokeAdjust = true

	p := xpath.NewPath()
	_ = p.MoveTo(5, 5)
	_ = p.LineTo(20, 5)
	_ = p.LineTo(20, 25)
	_ = p.LineTo(5, 25)
	// NOT closed → expect maybeInjectFillRectHints to close + add 2 hints.

	s.maybeInjectFillRectHints(p)
	if got := len(p.Hints()); got != 2 {
		t.Fatalf("hints after inject: got %d, want 2", got)
	}
	if !p.IsCurSubpathClosed() {
		t.Fatalf("expected path to be closed after maybeInjectFillRectHints")
	}
}

// TestSplashFillStrokeAdjustHintsClosed5 verifies that a 5-pt closed rect
// path with strokeAdjust enabled gets the hints without a second close.
func TestSplashFillStrokeAdjustHintsClosed5(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, true)
	s.state.strokeAdjust = true

	p := xpath.NewPath()
	_ = p.MoveTo(5, 5)
	_ = p.LineTo(20, 5)
	_ = p.LineTo(20, 25)
	_ = p.LineTo(5, 25)
	_ = p.Close(false) // 5th pt + closed flag
	if !p.IsCurSubpathClosed() {
		t.Fatalf("setup: expected closed subpath")
	}

	before := p.Length()
	s.maybeInjectFillRectHints(p)
	if got := len(p.Hints()); got != 2 {
		t.Fatalf("hints after inject (closed-5): got %d, want 2", got)
	}
	if p.Length() != before {
		t.Fatalf("length changed from %d to %d (closed-5 path should not grow)", before, p.Length())
	}
}

// TestSplashFillStrokeAdjustNoOpWithExistingHints verifies that the rect-hint
// injection bails when hints already exist.
func TestSplashFillStrokeAdjustNoOpWithExistingHints(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, true)
	s.state.strokeAdjust = true

	p := xpath.NewPath()
	_ = p.MoveTo(5, 5)
	_ = p.LineTo(20, 5)
	_ = p.LineTo(20, 25)
	_ = p.LineTo(5, 25)
	p.AddStrokeAdjustHint(0, 1, 0, 3) // pre-existing hint blocks injection.
	s.maybeInjectFillRectHints(p)
	if got := len(p.Hints()); got != 1 {
		t.Fatalf("hints with existing: got %d, want 1 (no injection)", got)
	}
}

// TestSplashFillStrokeAdjustOff verifies the rect-hint injection is gated by
// state.strokeAdjust.
func TestSplashFillStrokeAdjustOff(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, true)
	s.state.strokeAdjust = false // explicit

	p := xpath.NewPath()
	_ = p.MoveTo(5, 5)
	_ = p.LineTo(20, 5)
	_ = p.LineTo(20, 25)
	_ = p.LineTo(5, 25)
	s.maybeInjectFillRectHints(p)
	if got := len(p.Hints()); got != 0 {
		t.Fatalf("hints with strokeAdjust=false: got %d, want 0", got)
	}
}

// TestStrokeWideHorizontalLine verifies the strokeWide → fillImpl wiring.
// A horizontal line stroked at width 4 should produce a 4-pixel-tall band.
func TestStrokeWideHorizontalLine(t *testing.T) {
	bm := NewBitmap(64, 32, ModeRGB8, false)
	bm.rowSize = 64 * 3
	bm.data = make([]byte, bm.rowSize*32)
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.strokePattern = NewSolidColor(Color{0xFF, 0x00, 0x00})
	s.state.strokeAlpha = 1
	s.state.lineWidth = 4
	s.state.lineCap = int(LineCapButt)

	p := xpath.NewPath()
	_ = p.MoveTo(10, 10)
	_ = p.LineTo(50, 10)

	if err := s.Stroke(p); err != nil {
		t.Fatalf("Stroke: %v", err)
	}
	// Sample a column near mid-stroke (x=30). Splash centres the band on y=10
	// with butt caps, so rows 8..11 (height 4) should be opaque red.
	hits := 0
	for y := 0; y < 32; y++ {
		r, _, _ := readPx(s.bitmap, 30, y)
		if r == 0xFF {
			hits++
		}
	}
	if hits < 3 || hits > 5 {
		t.Fatalf("strokeWide horizontal width=4: red rows at x=30 = %d, want 3..5", hits)
	}
	// Ends of the line band should still be roughly in the band region —
	// at x=30 we already verified band height; double-check x=20 too.
	hits20 := 0
	for y := 0; y < 32; y++ {
		r, _, _ := readPx(s.bitmap, 20, y)
		if r == 0xFF {
			hits20++
		}
	}
	if hits20 < 3 || hits20 > 5 {
		t.Fatalf("strokeWide horizontal width=4: red rows at x=20 = %d, want 3..5", hits20)
	}
}

// TestSplashFillNarrowSubPixelGate exercises the very-thin-rect fill path.
// A near-zero-width rect should not crash and should not produce wild output.
func TestSplashFillNarrowSubPixelGate(t *testing.T) {
	s := newTestFillSplashRGB(t, 32, 32, true)
	// Width 0.05 px, well under the 0.2px gate.
	p := rectPath(t, 10, 10, 10.05, 25)
	if err := s.Fill(p, false); err != nil {
		t.Fatalf("Fill thin: %v", err)
	}
	// Deep exterior must remain paper.
	r, _, _ := readPx(s.bitmap, 5, 15)
	if r != 0 {
		t.Fatalf("thin-fill exterior at (5,15): got R=%02x, want 0", r)
	}
}

// TestSplashFillRunAALineCount verifies runAALine's popcount path on a
// hand-painted aaBuf where one column has all 16 sub-cells set (full
// coverage → aaGamma[16] = 255).
func TestSplashFillRunAALineCount(t *testing.T) {
	s := newTestFillSplashRGB(t, 4, 4, true)
	// Manually paint aaBuf: column x=1 has all 16 sub-cells set.
	// Layout: width = 4*4 = 16 sub-cells, rowSize = (16+7)>>3 = 2 bytes.
	rowSize := 2
	for yy := 0; yy < splashAASize; yy++ {
		// Column x=1 occupies sub-cells [4..8). Bits 4..7 of byte 0 = 0x0F.
		s.aaBuf[yy*rowSize+0] = 0x0F
	}
	var p pipe
	col := Color{0xFF, 0x00, 0x00}
	s.state.fillPattern = NewSolidColor(col)
	s.pipeInit(&p, 0, 0, s.state.fillPattern, nil, 255, true, false)
	s.runAALine(&p, 0, 3, 0, rowSize)
	r, g, b := readPx(s.bitmap, 1, 0)
	if r != 0xFF || g != 0 || b != 0 {
		t.Fatalf("runAALine full coverage at (1,0): got %02x %02x %02x, want FF 00 00", r, g, b)
	}
	// Other columns untouched (count=0 → pipeIncX, no write).
	r, _, _ = readPx(s.bitmap, 0, 0)
	if r != 0 {
		t.Fatalf("runAALine column 0 unwritten: got R=%02x", r)
	}
}
