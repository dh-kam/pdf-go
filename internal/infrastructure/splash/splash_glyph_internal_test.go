package splash

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// 1-bit glyph: one set pixel at (3,3) within an 8×8 bitmap.
// MSB-on-left convention: byte (3,3) → row 3, byte (3/8)=0, bit-shift 0x80>>3 = 0x10.
func TestSplashGlyph1BitSinglePixel(t *testing.T) {
	s, _ := New(makeBitmapForTest(16, 16, ModeRGB8), false)
	s.SetFillPattern(NewSolidColor(Color{255, 0, 0, 0, 0, 0, 0, 0}))
	s.SetFillAlpha(1)

	g := &GlyphBitmap{
		X: 0, Y: 0, W: 8, H: 8, AA: false,
		Data: make([]byte, 8), // 8 rows × 1 byte each
	}
	g.Data[3] = 0x80 >> 3 // pixel at column 3 set
	if err := s.FillGlyph(2, 2, g); err != nil {
		t.Fatalf("FillGlyph: %v", err)
	}
	// Glyph origin at (x0,y0)=(2,2); xStart = x0-glyph.X = 2; yStart = 2.
	// Set pixel at glyph (3,3) → bitmap (2+3, 2+3) = (5,5).
	off := (5*16 + 5) * 3
	if s.bitmap.data[off] != 255 {
		t.Fatalf("1-bit glyph: bitmap (5,5) red = %d, want 255", s.bitmap.data[off])
	}
	// And (4,4) (below) should be untouched.
	off2 := (4*16 + 4) * 3
	if s.bitmap.data[off2] != 0 {
		t.Fatalf("1-bit glyph: bitmap (4,4) bled, got %d", s.bitmap.data[off2])
	}
}

// 8-bit AA glyph: alpha=128 at (3,3) → 50%-blend of src on a white dst.
func TestSplashGlyphAAAlphaMid(t *testing.T) {
	b := makeBitmapForTest(16, 16, ModeRGB8)
	for i := range b.data {
		b.data[i] = 255
	}
	for i := range b.alpha {
		b.alpha[i] = 255
	}
	s, _ := New(b, false)
	s.SetFillPattern(NewSolidColor(Color{0, 0, 0, 0, 0, 0, 0, 0}))
	s.SetFillAlpha(1)

	g := &GlyphBitmap{
		X: 0, Y: 0, W: 8, H: 8, AA: true,
		Data: make([]byte, 64),
	}
	g.Data[3*8+3] = 128
	if err := s.FillGlyph(2, 2, g); err != nil {
		t.Fatalf("FillGlyph: %v", err)
	}
	// dst pixel (5,5) painted with shape=128, src=0, dst=255.
	// aSrc = div255(255*128) = 128. aResult = 128+255-div255(128*255) = 255.
	// c = ((255-128)*255 + 128*0)/255 = 127.
	off := (5*16 + 5) * 3
	got := int(s.bitmap.data[off])
	if got < 120 || got > 135 {
		t.Fatalf("AA glyph mid: got %d, want ~127", got)
	}
}

func TestSplashGlyphAAUsesPopplerTruncatingRGBBlend(t *testing.T) {
	b := makeBitmapForTest(16, 16, ModeRGB8)
	for i := range b.data {
		b.data[i] = 201
	}
	for i := range b.alpha {
		b.alpha[i] = 255
	}
	s, _ := New(b, false)
	s.SetFillPattern(NewSolidColor(Color{0, 0, 0, 0, 0, 0, 0, 0}))
	s.SetFillAlpha(1)

	g := &GlyphBitmap{
		X: 0, Y: 0, W: 8, H: 8, AA: true,
		Data: make([]byte, 64),
	}
	g.Data[3*8+3] = 128
	if err := s.FillGlyph(2, 2, g); err != nil {
		t.Fatalf("FillGlyph: %v", err)
	}

	off := (5*16 + 5) * 3
	if got, want := int(s.bitmap.data[off]), 100; got != want {
		t.Fatalf("AA glyph RGB blend = %d, want %d", got, want)
	}
}

func TestSplashGlyphBlackOnColoredDestinationUsesPopplerTruncatingRGBBlend(t *testing.T) {
	b := makeBitmapForTest(16, 16, ModeRGB8)
	for i := 0; i < len(b.data); i += 3 {
		b.data[i+0] = 255
		b.data[i+1] = 234
		b.data[i+2] = 234
	}
	for i := range b.alpha {
		b.alpha[i] = 255
	}
	s, _ := New(b, false)
	s.SetFillPattern(NewSolidColor(Color{0, 0, 0, 0, 0, 0, 0, 0}))
	s.SetFillAlpha(1)

	g := &GlyphBitmap{
		X: 0, Y: 0, W: 8, H: 8, AA: true,
		Data: make([]byte, 64),
	}
	g.Data[3*8+3] = 241
	if err := s.FillGlyph(2, 2, g); err != nil {
		t.Fatalf("FillGlyph: %v", err)
	}

	off := (5*16 + 5) * 3
	if got, want := b.data[off+0], byte(14); got != want {
		t.Fatalf("AA glyph red channel = %d, want %d", got, want)
	}
	if got, want := b.data[off+1], byte(12); got != want {
		t.Fatalf("AA glyph green channel = %d, want %d", got, want)
	}
	if got, want := b.data[off+2], byte(12); got != want {
		t.Fatalf("AA glyph blue channel = %d, want %d", got, want)
	}
}

func TestSplashGlyphBlackOnGrayDestinationUsesPopplerTruncatingRGBBlend(t *testing.T) {
	b := makeBitmapForTest(16, 16, ModeRGB8)
	for i := 0; i < len(b.data); i += 3 {
		b.data[i+0] = 217
		b.data[i+1] = 217
		b.data[i+2] = 217
	}
	for i := range b.alpha {
		b.alpha[i] = 255
	}
	s, _ := New(b, false)
	s.SetFillPattern(NewSolidColor(Color{0, 0, 0, 0, 0, 0, 0, 0}))
	s.SetFillAlpha(1)

	g := &GlyphBitmap{
		X: 0, Y: 0, W: 8, H: 8, AA: true,
		Data: make([]byte, 64),
	}
	g.Data[3*8+3] = 227
	if err := s.FillGlyph(2, 2, g); err != nil {
		t.Fatalf("FillGlyph: %v", err)
	}

	off := (5*16 + 5) * 3
	if got, want := b.data[off+0], byte(23); got != want {
		t.Fatalf("AA glyph gray red channel = %d, want %d", got, want)
	}
	if got, want := b.data[off+1], byte(23); got != want {
		t.Fatalf("AA glyph gray green channel = %d, want %d", got, want)
	}
	if got, want := b.data[off+2], byte(23); got != want {
		t.Fatalf("AA glyph gray blue channel = %d, want %d", got, want)
	}
}

func TestSplashGlyphRespectsClipRect(t *testing.T) {
	b := makeBitmapForTest(16, 16, ModeRGB8)
	for i := range b.data {
		b.data[i] = 255
	}
	for i := range b.alpha {
		b.alpha[i] = 255
	}
	s, _ := New(b, false)
	s.SetFillPattern(NewSolidColor(Color{0, 0, 0, 0, 0, 0, 0, 0}))
	s.SetFillAlpha(1)
	s.ClipResetToRect(0, 0, 5, 5)

	g := &GlyphBitmap{
		X: 0, Y: 0, W: 8, H: 8, AA: true,
		Data: make([]byte, 64),
	}
	g.Data[2*8+2] = 255 // Destination (4,4), inside [0,5).
	g.Data[3*8+3] = 255 // Destination (5,5), outside [0,5).
	if err := s.FillGlyph(2, 2, g); err != nil {
		t.Fatalf("FillGlyph: %v", err)
	}
	inside := (4*16 + 4) * 3
	if b.data[inside] != 0 {
		t.Fatalf("inside clipped glyph pixel = %d, want painted black", b.data[inside])
	}
	outside := (5*16 + 5) * 3
	if b.data[outside] != 255 {
		t.Fatalf("outside clipped glyph pixel = %d, want untouched white", b.data[outside])
	}
}

// Splash-native rasterizer: a 1×1 axis-aligned filled square at (0,0)-(1,1),
// rasterized at scale=1, should produce a 1×1 bitmap with alpha == 255.
func TestRasterizeGlyphUnitSquare(t *testing.T) {
	p := xpath.NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.LineTo(1, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.LineTo(1, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.LineTo(0, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(true); err != nil {
		t.Fatal(err)
	}
	g := RasterizeGlyph(p, 1, false)
	if g.W != 1 || g.H != 1 {
		t.Fatalf("rasterize unit: W=%d H=%d, want 1×1", g.W, g.H)
	}
	if g.Data[0] != 255 {
		t.Fatalf("rasterize unit: alpha = %d, want 255", g.Data[0])
	}
}

// Splash-native rasterizer: a thin axis-aligned filled stripe diagonal-ish
// (triangle) — bounding 4×4 — should produce intermediate AA values >0 and
// <255 on the diagonal fringe.
func TestRasterizeGlyphTriangleHasFringe(t *testing.T) {
	p := xpath.NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.LineTo(4, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.LineTo(4, 4); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(true); err != nil {
		t.Fatal(err)
	}
	g := RasterizeGlyph(p, 1, false)
	if g.W != 4 || g.H != 4 {
		t.Fatalf("triangle: W=%d H=%d, want 4×4", g.W, g.H)
	}
	hasFringe := false
	for _, v := range g.Data {
		if v > 0 && v < 255 {
			hasFringe = true
			break
		}
	}
	if !hasFringe {
		t.Fatalf("triangle: no AA fringe pixels; data=%v", g.Data)
	}
}
