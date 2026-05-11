package splash

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// linearGray maps t in [0,1] to a grayscale RGB triplet (black→white).
func linearGray(t float64) Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	v := byte(t*255 + 0.5)
	return Color{v, v, v}
}

// TestAxialShaderEndpointsAndMidpoint verifies the parametric mapping at the
// axis endpoints and midpoint — sample-point convention is (x, y+1).
func TestAxialShaderEndpointsAndMidpoint(t *testing.T) {
	// Use Y values shifted -1 because GetColor samples at (x, y+1).
	// Axis from (0, -1) → (10, -1) so that pixels on row y=-1 sample at fy=0.
	s := NewAxialShader(0, 0, 10, 0, 0, 1, false, false, linearGray, ModeRGB8)
	// Sample at (x=0, y=-1) → fy = 0, project on axis returns t=0 → black.
	var c Color
	if !s.GetColor(0, -1, &c) {
		t.Fatalf("GetColor at start expected ok")
	}
	if c[0] != 0 {
		t.Fatalf("start: got %d, want 0", c[0])
	}
	// Midpoint: sample at (5, -1) → t=0.5 → ~127 or 128.
	if !s.GetColor(5, -1, &c) {
		t.Fatalf("GetColor at mid expected ok")
	}
	if c[0] < 126 || c[0] > 129 {
		t.Fatalf("mid: got %d, want ~127", c[0])
	}
	// End: sample at (10, -1) → t=1 → 255.
	if !s.GetColor(10, -1, &c) {
		t.Fatalf("GetColor at end expected ok")
	}
	if c[0] != 255 {
		t.Fatalf("end: got %d, want 255", c[0])
	}
}

// TestAxialShaderYPlusOneSampleRule verifies the (x, y+1) convention from
// memory shading_corner_sample_2026_04_25: axis along Y means sampling at
// device y produces the t for y+1.
func TestAxialShaderYPlusOneSampleRule(t *testing.T) {
	// Vertical axis from (0,0)→(0,10), domain 0..1.
	s := NewAxialShader(0, 0, 0, 10, 0, 1, false, false, linearGray, ModeRGB8)
	// Sample at device y=0 → fy=1 → t=0.1 → ~26.
	var c Color
	if !s.GetColor(0, 0, &c) {
		t.Fatalf("GetColor expected ok at y=0")
	}
	wantF1 := 0.1*255 + 0.5
	want := byte(int(wantF1))
	if c[0] != want {
		t.Fatalf("y=0: got %d, want %d (verifies (x,y+1) sampling)", c[0], want)
	}
	// Sample at device y=4 → fy=5 → t=0.5.
	if !s.GetColor(0, 4, &c) {
		t.Fatalf("GetColor expected ok at y=4")
	}
	wantF2 := 0.5*255 + 0.5
	want = byte(int(wantF2))
	if c[0] != want {
		t.Fatalf("y=4: got %d, want %d", c[0], want)
	}
}

// TestAxialShaderExtendOff verifies sampling outside the axis with no /Extend
// returns false (transparent).
func TestAxialShaderExtendOff(t *testing.T) {
	s := NewAxialShader(0, 0, 10, 0, 0, 1, false, false, linearGray, ModeRGB8)
	var c Color
	// Sample at x=-5, y=-1 → t=-0.5 → outside, no extend → false.
	if s.GetColor(-5, -1, &c) {
		t.Fatalf("GetColor before t=0 should be transparent without Extend0")
	}
	// Sample at x=20, y=-1 → t=2 → outside → false.
	if s.GetColor(20, -1, &c) {
		t.Fatalf("GetColor after t=1 should be transparent without Extend1")
	}
}

// TestAxialShaderExtendOn verifies sampling outside with /Extend clamps to
// T0 / T1 endpoints.
func TestAxialShaderExtendOn(t *testing.T) {
	s := NewAxialShader(0, 0, 10, 0, 0, 1, true, true, linearGray, ModeRGB8)
	var c Color
	// Before t=0 → clamped to t=0 → black.
	if !s.GetColor(-5, -1, &c) {
		t.Fatalf("GetColor before t=0 should succeed with Extend0")
	}
	if c[0] != 0 {
		t.Fatalf("before-extend: got %d, want 0", c[0])
	}
	// After t=1 → clamped to t=1 → white.
	if !s.GetColor(20, -1, &c) {
		t.Fatalf("GetColor after t=1 should succeed with Extend1")
	}
	if c[0] != 255 {
		t.Fatalf("after-extend: got %d, want 255", c[0])
	}
}

// TestAxialShaderIsCMYK verifies the IsCMYK predicate matches the bitmap mode.
func TestAxialShaderIsCMYK(t *testing.T) {
	rgb := NewAxialShader(0, 0, 1, 0, 0, 1, false, false, linearGray, ModeRGB8)
	if rgb.IsCMYK() {
		t.Fatalf("RGB8 IsCMYK: got true, want false")
	}
	cmyk := NewAxialShader(0, 0, 1, 0, 0, 1, false, false, linearGray, ModeCMYK8)
	if !cmyk.IsCMYK() {
		t.Fatalf("CMYK8 IsCMYK: got false, want true")
	}
	devN := NewAxialShader(0, 0, 1, 0, 0, 1, false, false, linearGray, ModeDeviceN8)
	if !devN.IsCMYK() {
		t.Fatalf("DeviceN8 IsCMYK: got false, want true")
	}
}

// TestAxialShaderVerticalAxis verifies that an X0==X1 axis produces a gradient
// that varies in Y only (horizontal stripes).
func TestAxialShaderVerticalAxis(t *testing.T) {
	s := NewAxialShader(0, 0, 0, 8, 0, 1, false, false, linearGray, ModeRGB8)
	var ca, cb Color
	// Two pixels on the same row should produce identical color.
	if !s.GetColor(0, 3, &ca) {
		t.Fatalf("GetColor (0,3) failed")
	}
	if !s.GetColor(7, 3, &cb) {
		t.Fatalf("GetColor (7,3) failed")
	}
	if ca != cb {
		t.Fatalf("vertical axis: row should be uniform, got %v vs %v", ca, cb)
	}
	// Different rows must differ.
	var cc Color
	if !s.GetColor(0, 6, &cc) {
		t.Fatalf("GetColor (0,6) failed")
	}
	if ca == cc {
		t.Fatalf("vertical axis: rows 3 and 6 should differ")
	}
}

// TestAxialShaderDiagonalAxis verifies a 45° axis: pixels along the direction
// perpendicular to the axis share the same color.
func TestAxialShaderDiagonalAxis(t *testing.T) {
	// Axis from (0,0)→(10,10). Perpendicular is direction (1,-1).
	s := NewAxialShader(0, 0, 10, 10, 0, 1, false, false, linearGray, ModeRGB8)
	// Pick a base sample, then move along the perpendicular by (+1,-1):
	// the (x, y+1) sample rule means base is (5, 4)=fy=5 / perp (6, 3)=fy=4.
	// Both should map to the same projection on the axis.
	var ca, cb Color
	if !s.GetColor(5, 4, &ca) {
		t.Fatalf("GetColor (5,4) failed")
	}
	if !s.GetColor(6, 3, &cb) {
		t.Fatalf("GetColor (6,3) failed")
	}
	if ca != cb {
		t.Fatalf("diagonal axis: perpendicular pixels should match, got %v vs %v", ca, cb)
	}
}

// TestAxialShaderDegenerate verifies a zero-length axis returns t=0 always.
func TestAxialShaderDegenerate(t *testing.T) {
	s := NewAxialShader(5, 5, 5, 5, 0.25, 0.75, false, false, linearGray, ModeRGB8)
	var c Color
	if !s.GetColor(0, 0, &c) {
		t.Fatalf("degenerate axis: expected ok")
	}
	got := c[0]
	wantF := 0.25*255 + 0.5
	want := byte(int(wantF))
	if got != want {
		t.Fatalf("degenerate: got %d, want %d", got, want)
	}
}

// TestShadingFillAxialWiring exercises FillAxialShading end-to-end through the
// existing fill pipeline. Phase 3 pipe pattern-refresh is owned by P3-Dev?, so
// this test only verifies that the entry point installs the shader as the
// fill pattern, runs without error, and returns the wired pattern instance.
func TestShadingFillAxialWiring(t *testing.T) {
	bm := NewBitmap(32, 8, ModeRGB8, false)
	bm.rowSize = 32 * 3
	bm.data = make([]byte, bm.rowSize*8)
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
	if err := p.LineTo(32, 8); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(0, 8); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}
	shader := NewAxialShader(0, 0, 32, 0, 0, 1, true, true, linearGray, ModeRGB8)
	if err := sp.FillAxialShading(shader, p, false); err != nil {
		t.Fatalf("FillAxialShading: %v", err)
	}
	if got := sp.state.fillPattern; got != Pattern(shader) {
		t.Fatalf("fillPattern: got %T %p, want shader %p", got, got, shader)
	}
}

// TestShadingFillAxialNilShader verifies a nil shader argument is rejected.
func TestShadingFillAxialNilShader(t *testing.T) {
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
	if err := sp.FillAxialShading(nil, p, false); err == nil {
		t.Fatalf("FillAxialShading(nil): expected error, got nil")
	}
}

// TestAxialShaderTestPosition verifies Poppler's strict interior
// testPosition semantics.
func TestAxialShaderTestPosition(t *testing.T) {
	s := NewAxialShader(0, 0, 10, 0, 0, 1, true, true, linearGray, ModeRGB8)
	if !s.TestPosition(5, -1) {
		t.Fatalf("TestPosition(5,-1) inside: want true")
	}
	if s.TestPosition(0, -1) {
		t.Fatalf("TestPosition(0,-1) start endpoint: want false")
	}
	if s.TestPosition(10, -1) {
		t.Fatalf("TestPosition(10,-1) end endpoint: want false")
	}
	if s.TestPosition(-5, -1) {
		t.Fatalf("TestPosition(-5,-1) extended outside: want false")
	}
	if s.TestPosition(20, -1) {
		t.Fatalf("TestPosition(20,-1) extended outside: want false")
	}
}

// TestAxialShaderIsStatic verifies the IsStatic predicate is always false.
func TestAxialShaderIsStatic(t *testing.T) {
	s := NewAxialShader(0, 0, 10, 0, 0, 1, false, false, linearGray, ModeRGB8)
	if s.IsStatic() {
		t.Fatalf("IsStatic: got true, want false")
	}
}

// TestAxialShaderNilFunc guards against a nil Function — GetColor must return false.
func TestAxialShaderNilFunc(t *testing.T) {
	s := NewAxialShader(0, 0, 10, 0, 0, 1, true, true, nil, ModeRGB8)
	var c Color
	if s.GetColor(5, -1, &c) {
		t.Fatalf("GetColor with nil Func: got true, want false")
	}
}
