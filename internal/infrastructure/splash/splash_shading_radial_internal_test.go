package splash

import (
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// radialLinearGray maps t∈[0,1] to a grayscale RGB triplet.
func radialLinearGray(t float64) Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	v := byte(t*255 + 0.5)
	return Color{v, v, v}
}

// TestRadialShaderConcentricCenter verifies a concentric radial gradient: a
// pixel at the center maps to t=0 (inner color), a pixel on the outer circle
// maps to t≈1.
func TestRadialShaderConcentricCenter(t *testing.T) {
	// Concentric circles at (0,0) with r0=0, r1=10.
	// Sampling rule: pixel (0, 0) is evaluated at integer device y=0.
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, false, false, radialLinearGray, ModeRGB8)
	var c Color
	if !s.GetColor(0, 0, &c) {
		t.Fatalf("GetColor at center expected ok")
	}
	if c[0] != 0 {
		t.Fatalf("center: got %d, want 0", c[0])
	}
	// Pixel at distance 10 from center on outer circle.
	if !s.GetColor(10, 0, &c) {
		t.Fatalf("GetColor at outer expected ok")
	}
	if c[0] < 250 {
		t.Fatalf("outer: got %d, want ~255", c[0])
	}
}

// TestRadialShaderOutsideExtendOff verifies a pixel outside the outer circle
// returns transparent when /Extend1 is off.
func TestRadialShaderOutsideExtendOff(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, false, false, radialLinearGray, ModeRGB8)
	var c Color
	// Pixel at distance 20 from center.
	if s.GetColor(20, 0, &c) {
		t.Fatalf("GetColor outside outer expected transparent without Extend1")
	}
}

// TestRadialShaderExtendOn verifies /Extend clamps t to [0,1].
func TestRadialShaderExtendOn(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, true, true, radialLinearGray, ModeRGB8)
	var c Color
	if !s.GetColor(20, 0, &c) {
		t.Fatalf("GetColor with Extend1 expected ok at distance 20")
	}
	if c[0] != 255 {
		t.Fatalf("extend1: got %d, want 255", c[0])
	}
}

// TestRadialShaderHalfRadius verifies a pixel at half radius hits ~t=0.5.
func TestRadialShaderHalfRadius(t *testing.T) {
	// Concentric circles r0=0, r1=10. Pixel at distance 5 from center → t=0.5.
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, false, false, radialLinearGray, ModeRGB8)
	var c Color
	if !s.GetColor(5, 0, &c) {
		t.Fatalf("GetColor at half-radius expected ok")
	}
	if c[0] < 125 || c[0] > 130 {
		t.Fatalf("half-radius: got %d, want ~127", c[0])
	}
}

// TestRadialShaderIntegerYSampleRule verifies integer device-Y sampling.
func TestRadialShaderIntegerYSampleRule(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, false, false, radialLinearGray, ModeRGB8)
	var c0, c1 Color
	// Sample center.
	if !s.GetColor(0, 0, &c0) {
		t.Fatalf("center sample failed")
	}
	if c0[0] != 0 {
		t.Fatalf("center value: got %d, want 0", c0[0])
	}
	// One pixel down → t = 0.1.
	if !s.GetColor(0, 1, &c1) {
		t.Fatalf("near-center sample failed")
	}
	want := byte(0.1*255 + 0.5)
	if c1[0] != want {
		t.Fatalf("near-center: got %d, want %d", c1[0], want)
	}
}

// TestRadialShaderTestPosition verifies Poppler's strict interior
// testPosition semantics. GetColor may paint endpoints or extended roots, but
// Splash's edge correction only treats t0 < t < t1 as inside.
func TestRadialShaderTestPosition(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, true, true, radialLinearGray, ModeRGB8)
	if s.TestPosition(0, 0) {
		t.Fatalf("TestPosition center endpoint: want false")
	}
	if s.TestPosition(10, 0) {
		t.Fatalf("TestPosition outer endpoint: want false")
	}
	if !s.TestPosition(5, 0) {
		t.Fatalf("TestPosition interior: want true")
	}
	var c Color
	if !s.GetColor(20, 0, &c) {
		t.Fatalf("GetColor extended outside: want true")
	}
	if s.TestPosition(20, 0) {
		t.Fatalf("TestPosition extended outside: want false")
	}
}

// TestRadialShaderIsCMYK verifies the IsCMYK predicate matches the bitmap mode.
func TestRadialShaderIsCMYK(t *testing.T) {
	rgb := NewRadialShader(0, 0, 0, 1, 0, 1, 0, 1, false, false, radialLinearGray, ModeRGB8)
	if rgb.IsCMYK() {
		t.Fatalf("RGB8 IsCMYK: got true, want false")
	}
	cmyk := NewRadialShader(0, 0, 0, 1, 0, 1, 0, 1, false, false, radialLinearGray, ModeCMYK8)
	if !cmyk.IsCMYK() {
		t.Fatalf("CMYK8 IsCMYK: got false, want true")
	}
}

// TestRadialShaderIsStatic verifies IsStatic is always false.
func TestRadialShaderIsStatic(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 1, 0, 1, 0, 1, false, false, radialLinearGray, ModeRGB8)
	if s.IsStatic() {
		t.Fatalf("IsStatic: got true, want false")
	}
}

// TestRadialShaderNilFunc guards against a nil Function.
func TestRadialShaderNilFunc(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, true, true, nil, ModeRGB8)
	var c Color
	if s.GetColor(0, 0, &c) {
		t.Fatalf("GetColor with nil Func: got true, want false")
	}
}

// TestRadialShaderOffsetCircles verifies the quadratic solver picks the larger
// root for offset (non-concentric) circles.
func TestRadialShaderOffsetCircles(t *testing.T) {
	// Inner circle (0, 0, r=1), outer circle (10, 0, r=5).
	s := NewRadialShader(0, 0, 1, 10, 0, 5, 0, 1, false, false, radialLinearGray, ModeRGB8)
	var c Color
	// Pixel on the outer circle directly to the right of its center.
	if !s.GetColor(15, 0, &c) {
		t.Fatalf("GetColor on outer circle expected ok")
	}
	if c[0] < 250 {
		t.Fatalf("outer: got %d, want ~255", c[0])
	}
}

// TestRadialShaderOneRowGradient renders a 1-row gradient and confirms
// per-pixel monotonic progression.
func TestRadialShaderOneRowGradient(t *testing.T) {
	s := NewRadialShader(0, 0, 0, 0, 0, 10, 0, 1, true, true, radialLinearGray, ModeRGB8)
	last := byte(0)
	for x := 0; x <= 10; x++ {
		var c Color
		if !s.GetColor(x, 0, &c) {
			t.Fatalf("GetColor (%d, 0) failed", x)
		}
		if x > 0 && c[0] < last {
			t.Fatalf("non-monotonic at x=%d: %d < %d", x, c[0], last)
		}
		last = c[0]
	}
}

// TestRadialShaderDegenerate verifies the linear (a≈0) branch produces a
// usable t value rather than dividing by zero.
func TestRadialShaderDegenerate(t *testing.T) {
	// Both circles at same point with radii (0, 5). a = dx² + dy² - dr² =
	// 0 + 0 - 25 = -25, so NOT degenerate. Use a = 0 case: dx² + dy² == dr².
	// Set dx=3, dy=4, dr=5: 9 + 16 - 25 = 0. Origin (0,0,r0=0) → (3,4,r1=5).
	s := NewRadialShader(0, 0, 0, 3, 4, 5, 0, 1, true, true, radialLinearGray, ModeRGB8)
	if !s.degenerate {
		t.Fatalf("expected degenerate (a≈0): a=%v", s.a)
	}
	var c Color
	// Poppler treats b≈0 in the linearized equation as invalid. A point on the
	// cone away from the apex exercises the usable degenerate branch.
	if s.GetColor(0, 0, &c) {
		t.Fatalf("degenerate apex with b≈0 should be outside")
	}
	if !s.GetColor(3, 4, &c) {
		t.Fatalf("degenerate GetColor away from apex failed")
	}
	// Symbolic check: math.Abs is used in solver — sanity that the package compiles
	// the math import path.
	_ = math.Abs(0)
}

// TestShadingFillRadialWiring exercises FillRadialShading end-to-end through
// the existing fill pipeline. The shading→pipe pattern-refresh is owned by a
// later phase, so this test only verifies the entry point installs the shader
// as the fill pattern and runs without error.
func TestShadingFillRadialWiring(t *testing.T) {
	bm := NewBitmap(32, 32, ModeRGB8, false)
	bm.rowSize = 32 * 3
	bm.data = make([]byte, bm.rowSize*32)
	sp, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sp.state.fillAlpha = 1
	p := xpath.NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(32, 0); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(32, 32); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(0, 32); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	shader := NewRadialShader(16, 16, 0, 16, 16, 16, 0, 1, true, true, radialLinearGray, ModeRGB8)
	if err := sp.FillRadialShading(shader, p, false); err != nil {
		t.Fatalf("FillRadialShading: %v", err)
	}
	if got := sp.state.fillPattern; got != Pattern(shader) {
		t.Fatalf("fillPattern: got %T %p, want shader %p", got, got, shader)
	}
}

// TestShadingFillRadialNilShader verifies a nil shader argument is rejected.
func TestShadingFillRadialNilShader(t *testing.T) {
	bm := NewBitmap(8, 8, ModeRGB8, false)
	bm.rowSize = 8 * 3
	bm.data = make([]byte, bm.rowSize*8)
	sp, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := xpath.NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(8, 0); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(8, 8); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(0, 8); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := sp.FillRadialShading(nil, p, false); err != ErrBadArg {
		t.Fatalf("nil shader: got %v, want ErrBadArg", err)
	}
}
