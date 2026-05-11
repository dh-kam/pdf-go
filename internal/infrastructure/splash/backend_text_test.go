package splash

import (
	"errors"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// stubFont is a minimal entity.Font used to exercise the splash text path
// without pulling in the full standard.StandardFont machinery. Glyph paths
// are tiny axis-aligned squares so the rasterizer produces a deterministic
// non-empty bitmap (Splash.cc:2603 fillGlyph blit smoke).
type stubFont struct {
	name           string
	unitsPerEm     uint16
	advance        float64
	renderCalls    int
	failGlyph      uint32
	missingPathFor uint32
}

func (f *stubFont) Name() string                      { return f.name }
func (f *stubFont) IsCIDFont() bool                   { return false }
func (f *stubFont) IsSymbolic() bool                  { return false }
func (f *stubFont) UnitsPerEm() uint16                { return f.unitsPerEm }
func (f *stubFont) GlyphName(g uint32) string         { return "" }
func (f *stubFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}
func (f *stubFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if code == f.failGlyph {
		return 0, errors.New("missing")
	}
	return code, nil
}
func (f *stubFont) GetGlyphWidth(g uint32) (float64, error) {
	return f.advance, nil
}
func (f *stubFont) RenderGlyph(g uint32, size float64) (*entity.GlyphPath, error) {
	f.renderCalls++
	if g == f.missingPathFor {
		return &entity.GlyphPath{}, nil
	}
	// Emit a 4×4 axis-aligned filled square at (0,0)-(size, size). Y-up font
	// space matches what entity.Font.RenderGlyph promises (PDF 1.7 §9.6).
	cmds := []entity.PathCommand{
		&entity.PathMoveTo{X: 0, Y: 0},
		&entity.PathLineTo{X: size, Y: 0},
		&entity.PathLineTo{X: size, Y: size},
		&entity.PathLineTo{X: 0, Y: size},
		&entity.PathClose{},
	}
	return &entity.GlyphPath{Commands: cmds, Bounds: [4]float64{0, 0, size, size}}, nil
}

// scanNonWhitePixels counts RGB8 pixels in the backend's bitmap that are not
// pure paper-white. Used to assert glyphs actually drew through fillGlyph.
func scanNonWhitePixels(t *testing.T, c *splashCanvas) int {
	t.Helper()
	bm := c.s.bitmap
	if bm.Mode() != ModeRGB8 {
		t.Fatalf("expected ModeRGB8, got %d", bm.Mode())
	}
	data := bm.Data()
	rs := bm.RowSize()
	count := 0
	for y := 0; y < bm.height; y++ {
		off := y * rs
		for x := 0; x < bm.width; x++ {
			r, g, b := data[off], data[off+1], data[off+2]
			if r != 0xFF || g != 0xFF || b != 0xFF {
				count++
			}
			off += 3
		}
	}
	return count
}

// TestSplashCanvasDrawTextDrawsPixels confirms DrawText routes through
// Splash.fillGlyph: after drawing a 12pt square glyph, at least one pixel
// must be non-white (Splash.cc:2603).
func TestSplashCanvasDrawTextDrawsPixels(t *testing.T) {
	c := newTestBackend(t, 50, 30)
	c.SetFillColor(blackColorForTest())
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}
	if err := c.DrawText("A", 10, 20, font, 12); err != nil {
		t.Fatalf("DrawText: %v", err)
	}
	if got := scanNonWhitePixels(t, c); got == 0 {
		t.Fatalf("DrawText drew no pixels through splash; expected glyph blit")
	}
}

// TestSplashCanvasGlyphCacheHit verifies fetchGlyph memoises so repeated
// DrawText with the same rune does not re-rasterize.
func TestSplashCanvasGlyphCacheHit(t *testing.T) {
	c := newTestBackend(t, 50, 30)
	c.SetFillColor(blackColorForTest())
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}
	if err := c.DrawText("A", 10, 20, font, 12); err != nil {
		t.Fatalf("DrawText#1: %v", err)
	}
	if font.renderCalls != 1 {
		t.Fatalf("first DrawText: renderCalls=%d, want 1", font.renderCalls)
	}
	if err := c.DrawText("A", 25, 20, font, 12); err != nil {
		t.Fatalf("DrawText#2: %v", err)
	}
	if font.renderCalls != 1 {
		t.Fatalf("second DrawText with same glyph: renderCalls=%d, want still 1 (cache hit)", font.renderCalls)
	}
}

// TestSplashCanvasMoveTextPointAdvances checks that BeginText + MoveTextPoint +
// ShowText emit glyphs at the translated origin, not the original origin.
func TestSplashCanvasMoveTextPointAdvances(t *testing.T) {
	c := newTestBackend(t, 80, 40)
	c.SetFillColor(blackColorForTest())
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}
	c.currentFont = font
	c.fontSize = 12
	c.BeginText(5, 20)
	c.MoveTextPoint(20, 0)
	if c.textX != 25 || c.textY != 20 {
		t.Fatalf("MoveTextPoint: got (%v,%v), want (25,20)", c.textX, c.textY)
	}
	if err := c.ShowText("A"); err != nil {
		t.Fatalf("ShowText: %v", err)
	}
	c.EndText()
	if c.inText {
		t.Fatalf("EndText: inText still true")
	}
	// After ShowText, textX advanced by (500/1000)*12 = 6 → 31.
	if c.textX != 31 {
		t.Fatalf("ShowText advance: textX=%v, want 31", c.textX)
	}
	if got := scanNonWhitePixels(t, c); got == 0 {
		t.Fatalf("MoveTextPoint+ShowText: no glyph pixels drew")
	}
}

// TestSplashCanvasShowTextEmpty verifies ShowText(\"\") is a quiet no-op.
func TestSplashCanvasShowTextEmpty(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}
	c.currentFont = font
	c.fontSize = 10
	if err := c.ShowText(""); err != nil {
		t.Fatalf("ShowText(\"\"): %v", err)
	}
	if got := scanNonWhitePixels(t, c); got != 0 {
		t.Fatalf("ShowText(\"\") modified pixels: %d", got)
	}
}

// TestSplashCanvasShowTextNoFont verifies ShowText with no font returns nil
// and does not modify the bitmap.
func TestSplashCanvasShowTextNoFont(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	if err := c.ShowText("A"); err != nil {
		t.Fatalf("ShowText with nil font: %v", err)
	}
	if got := scanNonWhitePixels(t, c); got != 0 {
		t.Fatalf("ShowText with nil font modified pixels: %d", got)
	}
}

// TestSplashCanvasDrawTextEmptyText verifies DrawText("") returns nil with no
// drawing, matching ImageCanvas semantics (image_canvas_text.go:14).
func TestSplashCanvasDrawTextEmptyText(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}
	if err := c.DrawText("", 5, 10, font, 12); err != nil {
		t.Fatalf("DrawText empty: %v", err)
	}
	if got := scanNonWhitePixels(t, c); got != 0 {
		t.Fatalf("DrawText empty modified pixels: %d", got)
	}
}

// TestSplashCanvasDrawTextNilFont verifies DrawText with nil font is a no-op.
func TestSplashCanvasDrawTextNilFont(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	if err := c.DrawText("A", 5, 10, nil, 12); err != nil {
		t.Fatalf("DrawText nil font: %v", err)
	}
	if got := scanNonWhitePixels(t, c); got != 0 {
		t.Fatalf("DrawText nil font modified pixels: %d", got)
	}
}

// blackColorForTest returns a stdlib black color for SetFillColor in tests.
func blackColorForTest() rgbaTestColor {
	return rgbaTestColor{R: 0, G: 0, B: 0, A: 0xFF}
}

// rgbaTestColor is a stdlib-compatible color.Color used by tests in this file
// to avoid importing image/color in every test (matches color.RGBA layout).
type rgbaTestColor struct{ R, G, B, A uint8 }

func (c rgbaTestColor) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r |= r << 8
	g = uint32(c.G)
	g |= g << 8
	b = uint32(c.B)
	b |= b << 8
	a = uint32(c.A)
	a |= a << 8
	return
}
