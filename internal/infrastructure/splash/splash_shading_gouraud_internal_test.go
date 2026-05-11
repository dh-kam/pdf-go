package splash

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// makeFlatTriangle builds a 3-vertex constant-color triangle.
func makeFlatTriangle(x0, y0, x1, y1, x2, y2 float64, c Color) []GouraudVertex {
	return []GouraudVertex{
		{X: x0, Y: y0, Color: c},
		{X: x1, Y: y1, Color: c},
		{X: x2, Y: y2, Color: c},
	}
}

// TestGouraudShaderInteriorFlatColor verifies a triangle painted with a single
// color returns that color at any interior pixel.
func TestGouraudShaderInteriorFlatColor(t *testing.T) {
	tri := makeFlatTriangle(0, 0, 10, 0, 0, 10, Color{200, 100, 50})
	s := NewGouraudShader(tri, ModeRGB8)
	var c Color
	// Poppler's scanline path samples at integer device coordinates.
	if !s.GetColor(2, 2, &c) {
		t.Fatalf("interior GetColor expected ok")
	}
	if c[0] != 200 || c[1] != 100 || c[2] != 50 {
		t.Fatalf("flat: got %v, want {200,100,50}", c)
	}
}

// TestGouraudShaderOutsideTransparent verifies pixels outside any triangle
// return false (transparent).
func TestGouraudShaderOutsideTransparent(t *testing.T) {
	tri := makeFlatTriangle(0, 0, 10, 0, 0, 10, Color{200, 100, 50})
	s := NewGouraudShader(tri, ModeRGB8)
	var c Color
	// Pixel (50, 50) → far outside.
	if s.GetColor(50, 50, &c) {
		t.Fatalf("outside GetColor expected transparent")
	}
}

// TestGouraudShaderBarycentricBlend verifies the per-vertex color interpolation
// at the centroid of an equilateral-ish triangle.
func TestGouraudShaderBarycentricBlend(t *testing.T) {
	// Triangle (0,0,red) (12,0,green) (6,12,blue). Centroid at (6, 4).
	tri := []GouraudVertex{
		{X: 0, Y: 0, Color: Color{255, 0, 0}},
		{X: 12, Y: 0, Color: Color{0, 255, 0}},
		{X: 6, Y: 12, Color: Color{0, 0, 255}},
	}
	s := NewGouraudShader(tri, ModeRGB8)
	var c Color
	if !s.GetColor(6, 4, &c) {
		t.Fatalf("centroid GetColor expected ok")
	}
	// Each channel should be ≈ 85 (255/3).
	for k := 0; k < 3; k++ {
		if c[k] < 80 || c[k] > 90 {
			t.Fatalf("centroid channel %d: got %d, want ~85", k, c[k])
		}
	}
}

// TestGouraudShaderTestPosition verifies TestPosition is consistent with
// GetColor's containment decision.
func TestGouraudShaderTestPosition(t *testing.T) {
	tri := makeFlatTriangle(0, 0, 10, 0, 0, 10, Color{1, 2, 3})
	s := NewGouraudShader(tri, ModeRGB8)
	if !s.TestPosition(2, 1) {
		t.Fatalf("inside TestPosition: want true")
	}
	if s.TestPosition(50, 50) {
		t.Fatalf("outside TestPosition: want false")
	}
}

// TestGouraudShaderIsCMYK verifies the IsCMYK predicate matches the bitmap mode.
func TestGouraudShaderIsCMYK(t *testing.T) {
	tri := makeFlatTriangle(0, 0, 1, 0, 0, 1, Color{})
	rgb := NewGouraudShader(tri, ModeRGB8)
	if rgb.IsCMYK() {
		t.Fatalf("RGB8 IsCMYK: got true, want false")
	}
	cmyk := NewGouraudShader(tri, ModeCMYK8)
	if !cmyk.IsCMYK() {
		t.Fatalf("CMYK8 IsCMYK: got false, want true")
	}
	devN := NewGouraudShader(tri, ModeDeviceN8)
	if !devN.IsCMYK() {
		t.Fatalf("DeviceN8 IsCMYK: got false, want true")
	}
}

// TestGouraudShaderIsStatic verifies IsStatic is always false.
func TestGouraudShaderIsStatic(t *testing.T) {
	tri := makeFlatTriangle(0, 0, 1, 0, 0, 1, Color{})
	s := NewGouraudShader(tri, ModeRGB8)
	if s.IsStatic() {
		t.Fatalf("IsStatic: got true, want false")
	}
}

// TestGouraudShaderTrimsPartialTrailing verifies a triangle list whose length
// is not a multiple of 3 has the trailing partial group dropped.
func TestGouraudShaderTrimsPartialTrailing(t *testing.T) {
	verts := []GouraudVertex{
		{X: 0, Y: 0, Color: Color{1}},
		{X: 1, Y: 0, Color: Color{1}},
		{X: 0, Y: 1, Color: Color{1}},
		{X: 5, Y: 5, Color: Color{2}}, // partial trailing → dropped
	}
	s := NewGouraudShader(verts, ModeRGB8)
	if len(s.Triangles) != 3 {
		t.Fatalf("triangle list trim: got %d, want 3", len(s.Triangles))
	}
	if s.nTriangles() != 1 {
		t.Fatalf("nTriangles: got %d, want 1", s.nTriangles())
	}
}

// TestGouraudShaderMultiTriangle verifies pixel containment across two adjacent
// triangles forming a square.
func TestGouraudShaderMultiTriangle(t *testing.T) {
	// Square (0,0)-(10,10) split into two triangles.
	verts := []GouraudVertex{
		{X: 0, Y: 0, Color: Color{255, 0, 0}},
		{X: 10, Y: 0, Color: Color{255, 0, 0}},
		{X: 10, Y: 10, Color: Color{255, 0, 0}},
		{X: 0, Y: 0, Color: Color{0, 255, 0}},
		{X: 10, Y: 10, Color: Color{0, 255, 0}},
		{X: 0, Y: 10, Color: Color{0, 255, 0}},
	}
	s := NewGouraudShader(verts, ModeRGB8)
	if s.nTriangles() != 2 {
		t.Fatalf("nTriangles: got %d, want 2", s.nTriangles())
	}
	var c Color
	// Pixel solidly in the upper-right triangle.
	if !s.GetColor(8, 2, &c) {
		t.Fatalf("upper-right GetColor failed")
	}
	if c[0] != 255 || c[1] != 0 {
		t.Fatalf("upper-right: got %v, want red", c)
	}
	// Pixel solidly in the lower-left triangle.
	if !s.GetColor(1, 8, &c) {
		t.Fatalf("lower-left GetColor failed")
	}
	if c[1] != 255 || c[0] != 0 {
		t.Fatalf("lower-left: got %v, want green", c)
	}
}

// TestGouraudShaderNilColorBuffer guards against a nil out-pointer.
func TestGouraudShaderNilColorBuffer(t *testing.T) {
	tri := makeFlatTriangle(0, 0, 10, 0, 0, 10, Color{1})
	s := NewGouraudShader(tri, ModeRGB8)
	if s.GetColor(2, 1, nil) {
		t.Fatalf("nil out: got true, want false")
	}
}

// TestGouraudShaderEmptyMesh verifies an empty mesh returns transparent.
func TestGouraudShaderEmptyMesh(t *testing.T) {
	s := NewGouraudShader(nil, ModeRGB8)
	var c Color
	if s.GetColor(5, 5, &c) {
		t.Fatalf("empty mesh GetColor: got true, want false")
	}
	if s.TestPosition(5, 5) {
		t.Fatalf("empty mesh TestPosition: got true, want false")
	}
}

// TestGouraudFillIntegrationRGB drives a single red triangle through the
// scanline rasterizer and confirms the bitmap shows red inside.
func TestGouraudFillIntegrationRGB(t *testing.T) {
	bm := NewBitmap(16, 16, ModeRGB8, true)
	bm.rowSize = 16 * 3
	bm.data = make([]byte, bm.rowSize*16)
	bm.alpha = make([]byte, 16*16)
	sp, err := New(bm, false) // direct-blit path is simpler without AA
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	verts := []GouraudVertex{
		{X: 2, Y: 2, Color: Color{255, 0, 0}},
		{X: 14, Y: 2, Color: Color{255, 0, 0}},
		{X: 8, Y: 14, Color: Color{255, 0, 0}},
	}
	shader := NewGouraudShader(verts, ModeRGB8)
	p := xpath.NewPath()
	if err := sp.FillGouraudTriangleShadedFill(shader, p, false); err != nil {
		t.Fatalf("FillGouraudTriangleShadedFill: %v", err)
	}
	// Centroid (8, 6) should be red.
	off := 6*bm.rowSize + 8*3
	if bm.data[off] != 255 || bm.data[off+1] != 0 || bm.data[off+2] != 0 {
		t.Fatalf("centroid pixel: got %v, want red", bm.data[off:off+3])
	}
	// Alpha should be set inside.
	if bm.alpha[6*16+8] != 0xFF {
		t.Fatalf("alpha at centroid: got %d, want 255", bm.alpha[6*16+8])
	}
	// Outside the triangle (corner 0,0) should remain unset.
	if bm.alpha[0] != 0 {
		t.Fatalf("alpha at corner: got %d, want 0", bm.alpha[0])
	}
}

// TestGouraudFillIntegrationGradient drives a 3-color triangle and confirms
// each near-vertex pixel is dominated by the corresponding vertex color.
func TestGouraudFillIntegrationGradient(t *testing.T) {
	bm := NewBitmap(32, 32, ModeRGB8, true)
	bm.rowSize = 32 * 3
	bm.data = make([]byte, bm.rowSize*32)
	bm.alpha = make([]byte, 32*32)
	sp, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	verts := []GouraudVertex{
		{X: 4, Y: 4, Color: Color{255, 0, 0}},
		{X: 28, Y: 4, Color: Color{0, 255, 0}},
		{X: 16, Y: 28, Color: Color{0, 0, 255}},
	}
	shader := NewGouraudShader(verts, ModeRGB8)
	p := xpath.NewPath()
	if err := sp.FillGouraudTriangleShadedFill(shader, p, false); err != nil {
		t.Fatalf("FillGouraudTriangleShadedFill: %v", err)
	}
	// Pixel near the red vertex should be mostly red.
	off := 5*bm.rowSize + 5*3
	if bm.data[off] < 200 {
		t.Fatalf("near-red R: got %d, want ≥200", bm.data[off])
	}
	// Pixel near the green vertex should be mostly green.
	off = 5*bm.rowSize + 26*3
	if bm.data[off+1] < 200 {
		t.Fatalf("near-green G: got %d, want ≥200", bm.data[off+1])
	}
	// Pixel near the blue vertex should be mostly blue.
	off = 26*bm.rowSize + 16*3
	if bm.data[off+2] < 200 {
		t.Fatalf("near-blue B: got %d, want ≥200", bm.data[off+2])
	}
}

// TestGouraudShaderNilSplashGuard verifies the driver gracefully rejects a
// nil shader.
func TestGouraudShaderNilSplashGuard(t *testing.T) {
	bm := NewBitmap(4, 4, ModeRGB8, false)
	bm.rowSize = 4 * 3
	bm.data = make([]byte, 4*4*3)
	sp, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := sp.FillGouraudTriangleShadedFill(nil, nil, false); err != ErrBadArg {
		t.Fatalf("nil shader: got %v, want ErrBadArg", err)
	}
}

// TestGouraudShaderDegenerateTriangle verifies a colinear triangle is silently
// skipped (matches Splash.cc:5372 det=0 guard).
func TestGouraudShaderDegenerateTriangle(t *testing.T) {
	bm := NewBitmap(8, 8, ModeRGB8, true)
	bm.rowSize = 8 * 3
	bm.data = make([]byte, 8*8*3)
	bm.alpha = make([]byte, 8*8)
	sp, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	verts := []GouraudVertex{
		{X: 0, Y: 0, Color: Color{255}},
		{X: 4, Y: 0, Color: Color{255}},
		{X: 8, Y: 0, Color: Color{255}}, // colinear
	}
	shader := NewGouraudShader(verts, ModeRGB8)
	p := xpath.NewPath()
	if err := sp.FillGouraudTriangleShadedFill(shader, p, false); err != nil {
		t.Fatalf("FillGouraudTriangleShadedFill: %v", err)
	}
	// No alpha should have been set anywhere.
	for i, a := range bm.alpha {
		if a != 0 {
			t.Fatalf("degenerate triangle wrote alpha at %d", i)
		}
	}
}
