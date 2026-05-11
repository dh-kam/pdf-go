package splash

import (
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// makeMonoCell builds an in-memory Mono8 Bitmap pre-populated row-major from src.
func makeMonoCell(t *testing.T, w, h int, src []byte) *Bitmap {
	t.Helper()
	if len(src) != w*h {
		t.Fatalf("makeMonoCell: src len %d != %d", len(src), w*h)
	}
	bm := NewBitmap(w, h, ModeMono8, false)
	copy(bm.Data(), src)
	return bm
}

// makeRGBCell builds an in-memory RGB8 Bitmap from a flat (3*w*h) byte stream.
func makeRGBCell(t *testing.T, w, h int, src []byte) *Bitmap {
	t.Helper()
	if len(src) != 3*w*h {
		t.Fatalf("makeRGBCell: src len %d != %d", len(src), 3*w*h)
	}
	bm := NewBitmap(w, h, ModeRGB8, false)
	copy(bm.Data(), src)
	return bm
}

// identMatrix is the identity affine [1 0 0 1 0 0].
var identMatrix = [6]float64{1, 0, 0, 1, 0, 0}

// TestTilingPatternInvertAffineIdentity asserts the inverse of identity is identity.
func TestTilingPatternInvertAffineIdentity(t *testing.T) {
	inv, ok := invertAffine(identMatrix)
	if !ok {
		t.Fatalf("identity matrix should be invertible")
	}
	want := identMatrix
	for i := 0; i < 6; i++ {
		if math.Abs(inv[i]-want[i]) > 1e-12 {
			t.Fatalf("inv[%d]=%v want %v", i, inv[i], want[i])
		}
	}
}

// TestTilingPatternInvertAffineSingular asserts singular matrices return ok=false.
func TestTilingPatternInvertAffineSingular(t *testing.T) {
	m := [6]float64{0, 0, 0, 0, 0, 0}
	if _, ok := invertAffine(m); ok {
		t.Fatalf("expected singular matrix")
	}
}

// TestTilingPatternInvertAffineRoundTrip asserts inv(M) * M = I for arbitrary M.
func TestTilingPatternInvertAffineRoundTrip(t *testing.T) {
	m := [6]float64{2, 1, -1, 3, 5, 7}
	inv, ok := invertAffine(m)
	if !ok {
		t.Fatalf("should invert")
	}
	// Pick a sample point, forward then inverse — should round-trip.
	for _, pt := range [][2]float64{{0, 0}, {1, 0}, {3, 4}, {-2, 5}} {
		fx, fy := applyAffine(m, pt[0], pt[1])
		bx, by := applyAffine(inv, fx, fy)
		if math.Abs(bx-pt[0]) > 1e-9 || math.Abs(by-pt[1]) > 1e-9 {
			t.Fatalf("round-trip mismatch: in (%v,%v) -> (%v,%v) -> (%v,%v)", pt[0], pt[1], fx, fy, bx, by)
		}
	}
}

// TestTilingPatternPosMod asserts posMod always returns [0, m) including for negatives.
func TestTilingPatternPosMod(t *testing.T) {
	cases := []struct {
		x, m, want float64
	}{
		{0, 4, 0},
		{1.5, 4, 1.5},
		{4, 4, 0},
		{4.5, 4, 0.5},
		{-0.5, 4, 3.5},
		{-4, 4, 0},
		{-7, 4, 1},
	}
	for _, c := range cases {
		got := posMod(c.x, c.m)
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("posMod(%v, %v) = %v, want %v", c.x, c.m, got, c.want)
		}
	}
}

// TestTilingPatternSingleCellGetColor verifies a 4×4 checker tiles at identity matrix.
//
// Cell:
//
//	0 255   0 255
//
// 255   0 255   0
//
//	0 255   0 255
//
// 255   0 255   0
func TestTilingPatternSingleCellGetColor(t *testing.T) {
	cell := []byte{
		0, 255, 0, 255,
		255, 0, 255, 0,
		0, 255, 0, 255,
		255, 0, 255, 0,
	}
	bm := makeMonoCell(t, 4, 4, cell)
	pat := NewTilingPattern(bm, [4]float64{0, 0, 4, 4}, 4, 4, identMatrix, 1, Color{})

	// Sample the original 4x4: should match the cell.
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			var c Color
			if !pat.GetColor(x, y, &c) {
				t.Fatalf("GetColor(%d,%d) returned false", x, y)
			}
			want := cell[y*4+x]
			if c[0] != want {
				t.Fatalf("(%d,%d): got %d want %d", x, y, c[0], want)
			}
		}
	}
	// Sample the next tile (offset by one cell in both directions): same pattern.
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			var c Color
			pat.GetColor(x+4, y+4, &c)
			want := cell[y*4+x]
			if c[0] != want {
				t.Fatalf("tile(%d,%d): got %d want %d", x+4, y+4, c[0], want)
			}
		}
	}
}

// TestTilingPatternMatrixTranslation verifies a translation matrix shifts the tiling origin.
//
// Matrix translates pattern origin to (10, 10) in device space; sampling at
// device (10, 10) should produce cell pixel (0, 0).
func TestTilingPatternMatrixTranslation(t *testing.T) {
	cell := []byte{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	}
	bm := makeMonoCell(t, 4, 4, cell)
	mat := [6]float64{1, 0, 0, 1, 10, 10} // translate (10, 10).
	pat := NewTilingPattern(bm, [4]float64{0, 0, 4, 4}, 4, 4, mat, 1, Color{})

	// Device pixel (10, 10) corresponds to pattern (0, 0) — cell pixel (0, 0) = 1.
	var c Color
	pat.GetColor(10, 10, &c)
	if c[0] != 1 {
		t.Fatalf("translated origin: got %d want 1", c[0])
	}
	// Device (13, 12) → pattern (3, 2) → cell (3, 2) = row2*4+col3 = 12.
	pat.GetColor(13, 12, &c)
	if c[0] != 12 {
		t.Fatalf("translated cell(3,2): got %d want 12", c[0])
	}
	// Device (14, 14) → pattern (4, 4) → wraps to cell (0, 0) = 1.
	pat.GetColor(14, 14, &c)
	if c[0] != 1 {
		t.Fatalf("translated wrap: got %d want 1", c[0])
	}
}

// TestTilingPatternModuloWrap asserts negative device coords wrap into [0, step).
func TestTilingPatternModuloWrap(t *testing.T) {
	cell := []byte{
		10, 20,
		30, 40,
	}
	bm := makeMonoCell(t, 2, 2, cell)
	pat := NewTilingPattern(bm, [4]float64{0, 0, 2, 2}, 2, 2, identMatrix, 1, Color{})

	// Device pixel (-1, -1) → pattern (-1, -1) → wraps to (1, 1) → cell (1, 1) = 40.
	var c Color
	pat.GetColor(-1, -1, &c)
	if c[0] != 40 {
		t.Fatalf("(-1,-1) wrap: got %d want 40", c[0])
	}
	// Device pixel (-2, 0) → pattern (-2, 0) → wraps to (0, 0) → cell (0, 0) = 10.
	pat.GetColor(-2, 0, &c)
	if c[0] != 10 {
		t.Fatalf("(-2,0) wrap: got %d want 10", c[0])
	}
}

// TestTilingPatternPaintType2Tint asserts uncolored cells are tinted by TintColor.
//
// Poppler colorizes PaintType=2 cells as a white-to-tint ramp. A grayscale
// cell with intensities {64, 192} tinted by red should keep R at 255 and
// ramp G/B from 64 to 192.
func TestTilingPatternPaintType2Tint(t *testing.T) {
	cell := []byte{64, 192}
	bm := makeMonoCell(t, 2, 1, cell)
	tint := Color{255, 0, 0}
	pat := NewTilingPatternWithMode(bm, [4]float64{0, 0, 2, 1}, 2, 1, identMatrix, 2, tint, ModeRGB8)

	var c Color
	pat.GetColor(0, 0, &c)
	if c[0] != 255 || c[1] < 63 || c[1] > 65 || c[2] < 63 || c[2] > 65 {
		t.Fatalf("paint2 (0,0): got %v, want [255 ~64 ~64]", c[:3])
	}
	pat.GetColor(1, 0, &c)
	if c[0] != 255 || c[1] < 191 || c[1] > 193 || c[2] < 191 || c[2] > 193 {
		t.Fatalf("paint2 (1,0): got %v, want [255 ~192 ~192]", c[:3])
	}
}

// TestTilingPatternPaintType1NoTint asserts colored (PaintType=1) returns raw cell pixels.
func TestTilingPatternPaintType1NoTint(t *testing.T) {
	rgb := []byte{
		255, 128, 0,
		0, 255, 128,
	}
	bm := makeRGBCell(t, 2, 1, rgb)
	tint := Color{0, 0, 255} // should be IGNORED for PaintType=1.
	pat := NewTilingPattern(bm, [4]float64{0, 0, 2, 1}, 2, 1, identMatrix, 1, tint)

	var c Color
	pat.GetColor(0, 0, &c)
	if c[0] != 255 || c[1] != 128 || c[2] != 0 {
		t.Fatalf("paint1 (0,0): got %v want [255 128 0]", c[:3])
	}
	pat.GetColor(1, 0, &c)
	if c[0] != 0 || c[1] != 255 || c[2] != 128 {
		t.Fatalf("paint1 (1,0): got %v want [0 255 128]", c[:3])
	}
}

// TestTilingPatternRotationMatrix asserts a 90° rotation matrix rotates the tiling.
//
// Matrix [0 1 -1 0 0 0] rotates pattern→device by +90° about origin. Inverse is
// [0 -1 1 0 0 0]. So device (x, y) maps to pattern (y, -x). With BBox at origin,
// device (0, 0) → pattern (0, 0) → cell (0, 0); device (0, -3) → pattern (-3, 0)
// → wraps modulo 4 to (1, 0) → cell (1, 0).
func TestTilingPatternRotationMatrix(t *testing.T) {
	cell := []byte{
		0, 1, 2, 3,
		10, 11, 12, 13,
		20, 21, 22, 23,
		30, 31, 32, 33,
	}
	bm := makeMonoCell(t, 4, 4, cell)
	// pattern→device 90° CCW: a=0, b=1, c=-1, d=0.
	mat := [6]float64{0, 1, -1, 0, 0, 0}
	pat := NewTilingPattern(bm, [4]float64{0, 0, 4, 4}, 4, 4, mat, 1, Color{})

	var c Color
	// device (0, 0) → pattern (0, 0) → cell (0, 0) = 0.
	pat.GetColor(0, 0, &c)
	if c[0] != 0 {
		t.Fatalf("rot (0,0): got %d want 0", c[0])
	}
	// Inverse [0 -1 1 0 0 0]: device (3, 0) → (a*x+c*y+e, b*x+d*y+f) =
	// (0*3 + 1*0, -1*3 + 0*0) = (0, -3) → posMod (0, 1) → cell (0, 1) = 10.
	pat.GetColor(3, 0, &c)
	if c[0] != 10 {
		t.Fatalf("rot (3,0): got %d want 10", c[0])
	}
}

// TestTilingPatternFillIntegrationRGB drives FillWithTilingPattern through Splash.Fill.
//
// Confirms the fill driver accepts a TilingPattern and runs without error on a
// closed rect path. (Per-pixel pattern sampling inside the AA pipe is a Phase 4
// follow-up — the current pipe captures cSrc once at init for non-static
// patterns, mirroring the same gap in P3-Dev1's shading integration.)
func TestTilingPatternFillIntegrationRGB(t *testing.T) {
	cell := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 0,
	}
	cellBM := makeRGBCell(t, 2, 2, cell)
	pat := NewTilingPattern(cellBM, [4]float64{0, 0, 2, 2}, 2, 2, identMatrix, 1, Color{})

	dst := NewBitmap(4, 4, ModeRGB8, false)
	dst.Clear(Color{}) // black ground.
	sp, err := New(dst, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := xpath.NewPath()
	if err := path.MoveTo(0, 0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := path.LineTo(4, 0); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := path.LineTo(4, 4); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := path.LineTo(0, 4); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if err := path.Close(true); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := sp.FillWithTilingPattern(pat, path, false); err != nil {
		t.Fatalf("FillWithTilingPattern: %v", err)
	}
}

// TestTilingPatternNilCell asserts a nil cell bitmap produces GetColor=false.
func TestTilingPatternNilCell(t *testing.T) {
	pat := NewTilingPattern(nil, [4]float64{0, 0, 2, 2}, 2, 2, identMatrix, 1, Color{})
	var c Color
	if pat.GetColor(0, 0, &c) {
		t.Fatalf("expected GetColor=false for nil cell")
	}
}

// TestTilingPatternSingularMatrix asserts a singular Matrix yields GetColor=false.
func TestTilingPatternSingularMatrix(t *testing.T) {
	bm := makeMonoCell(t, 2, 2, []byte{1, 2, 3, 4})
	mat := [6]float64{0, 0, 0, 0, 0, 0}
	pat := NewTilingPattern(bm, [4]float64{0, 0, 2, 2}, 2, 2, mat, 1, Color{})
	var c Color
	if pat.GetColor(0, 0, &c) {
		t.Fatalf("expected GetColor=false for singular matrix")
	}
}

// TestTilingPatternZeroStep asserts XStep<=0 or YStep<=0 produces GetColor=false.
func TestTilingPatternZeroStep(t *testing.T) {
	bm := makeMonoCell(t, 2, 2, []byte{1, 2, 3, 4})
	pat := NewTilingPattern(bm, [4]float64{0, 0, 2, 2}, 0, 2, identMatrix, 1, Color{})
	var c Color
	if pat.GetColor(0, 0, &c) {
		t.Fatalf("expected GetColor=false for XStep=0")
	}
}

// TestTilingPatternIsCMYKMode asserts IsCMYK reflects the cell bitmap mode.
func TestTilingPatternIsCMYKMode(t *testing.T) {
	rgb := makeMonoCell(t, 1, 1, []byte{0})
	patRGB := NewTilingPattern(rgb, [4]float64{0, 0, 1, 1}, 1, 1, identMatrix, 1, Color{})
	if patRGB.IsCMYK() {
		t.Fatalf("Mono8 cell should not report CMYK")
	}
	cmyk := NewBitmap(1, 1, ModeCMYK8, false)
	patCMYK := NewTilingPattern(cmyk, [4]float64{0, 0, 1, 1}, 1, 1, identMatrix, 1, Color{})
	if !patCMYK.IsCMYK() {
		t.Fatalf("CMYK8 cell should report CMYK")
	}
}

// TestTilingPatternStaticAndTestPosition asserts Pattern interface contract values.
func TestTilingPatternStaticAndTestPosition(t *testing.T) {
	bm := makeMonoCell(t, 1, 1, []byte{0})
	pat := NewTilingPattern(bm, [4]float64{0, 0, 1, 1}, 1, 1, identMatrix, 1, Color{})
	if pat.IsStatic() {
		t.Fatalf("tiling should not be static")
	}
	if !pat.TestPosition(99, 99) {
		t.Fatalf("tiling should cover all positions")
	}
}

// TestTilingPatternFillWithNilPattern asserts FillWithTilingPattern(nil) errors.
func TestTilingPatternFillWithNilPattern(t *testing.T) {
	dst := NewBitmap(4, 4, ModeRGB8, false)
	sp, _ := New(dst, true)
	path := xpath.NewPath()
	_ = path.MoveTo(0, 0)
	_ = path.LineTo(1, 0)
	_ = path.LineTo(1, 1)
	_ = path.LineTo(0, 1)
	_ = path.Close(true)
	if err := sp.FillWithTilingPattern(nil, path, false); err == nil {
		t.Fatalf("expected error for nil pattern")
	}
}
