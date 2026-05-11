package splash

import (
	"math"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// TilingPattern implements Pattern for PDF Tiling Pattern Type 1 (PDF 1.7 §8.7.3.3).
//
// The cell bitmap is rendered ONCE upfront (typically by a sub-Splash render of
// the pattern's content stream) and is then sampled modulo (XStep, YStep) over
// the fill region. Device pixel (x, y) is mapped to pattern space via the
// inverse of Matrix, wrapped into [0, XStep) × [0, YStep), and read from the
// cell bitmap at the corresponding cell-space pixel. When XStep/YStep differ
// from the BBox extent (gaps or overlapping cells), positions inside the step
// rectangle but outside the cell bbox return false (pattern hole) — matching
// the legacy stamp+clip behaviour in image_canvas.DrawTilingPattern where the
// per-tile step clip rectangle is `[ceil(tileX), ceil(tileX+xStepPx))`.
type TilingPattern struct {
	// CellBitmap is the pre-rendered pattern cell (PDF 1.7 §8.7.3.3 /Resources stream).
	CellBitmap *Bitmap
	// BBox is the cell bbox in pattern space (xMin, yMin, xMax, yMax) (PDF 1.7 §8.7.3.3 /BBox).
	BBox [4]float64
	// XStep is the horizontal step in pattern space (PDF 1.7 §8.7.3.3 /XStep).
	XStep float64
	// YStep is the vertical step in pattern space (PDF 1.7 §8.7.3.3 /YStep).
	YStep float64
	// Matrix is the pattern→device transform (PDF 1.7 §8.7.3.3 /Matrix).
	Matrix [6]float64
	// PaintType is 1 for colored, 2 for uncolored (PDF 1.7 §8.7.3.3 /PaintType).
	PaintType int
	// TintColor is the color used to tint a PaintType=2 (uncolored) cell.
	TintColor Color
	// ColorMode is the destination mode used after PaintType=2 colorization.
	ColorMode ColorMode

	// invMatrix is the precomputed device→pattern inverse of Matrix.
	invMatrix [6]float64
	// invOK is false when Matrix is singular; GetColor short-circuits to false.
	invOK bool
	// cellHasAlpha is set when the cell bitmap was rendered with an alpha plane —
	// for PaintType=2 we use the alpha plane as the coverage signal so a cell
	// rendered with a transparent backdrop tints only the painted strokes.
	cellHasAlpha bool
}

// NewTilingPattern constructs a TilingPattern and precomputes the inverse matrix (PDF 1.7 §8.7.3.3).
func NewTilingPattern(cell *Bitmap, bbox [4]float64, xstep, ystep float64, matrix [6]float64, paintType int, tintColor Color) *TilingPattern {
	mode := ModeRGB8
	if cell != nil {
		mode = cell.Mode()
	}
	return NewTilingPatternWithMode(cell, bbox, xstep, ystep, matrix, paintType, tintColor, mode)
}

// NewTilingPatternWithMode constructs a TilingPattern with an explicit output mode.
func NewTilingPatternWithMode(cell *Bitmap, bbox [4]float64, xstep, ystep float64, matrix [6]float64, paintType int, tintColor Color, mode ColorMode) *TilingPattern {
	tp := &TilingPattern{
		CellBitmap: cell,
		BBox:       bbox,
		XStep:      xstep,
		YStep:      ystep,
		Matrix:     matrix,
		PaintType:  paintType,
		TintColor:  tintColor,
		ColorMode:  mode,
	}
	tp.invMatrix, tp.invOK = invertAffine(matrix)
	if cell != nil && cell.Alpha() != nil {
		tp.cellHasAlpha = true
	}
	return tp
}

// invertAffine inverts a 6-tuple affine [a b c d e f] (PDF 8.3.3); ok=false when singular.
func invertAffine(m [6]float64) (inv [6]float64, ok bool) {
	a, b, c, d, e, f := m[0], m[1], m[2], m[3], m[4], m[5]
	det := a*d - b*c
	if math.Abs(det) < 1e-20 {
		return inv, false
	}
	invDet := 1.0 / det
	// Affine inverse: [ d  -b  ; -c  a ] / det, with translation back-out.
	inv[0] = d * invDet
	inv[1] = -b * invDet
	inv[2] = -c * invDet
	inv[3] = a * invDet
	inv[4] = (c*f - d*e) * invDet
	inv[5] = (b*e - a*f) * invDet
	return inv, true
}

// applyAffine maps point (x, y) through 6-tuple affine [a b c d e f] (PDF 8.3.3).
func applyAffine(m [6]float64, x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// posMod returns x mod m clamped to [0, m); handles negative x (Go math.Mod sign).
func posMod(x, m float64) float64 {
	if m <= 0 {
		return 0
	}
	r := math.Mod(x, m)
	if r < 0 {
		r += m
	}
	return r
}

func (p *TilingPattern) sampleCellPixel(x, y int) (int, int, bool) {
	if p.CellBitmap == nil || !p.invOK {
		return 0, 0, false
	}
	cellW := p.CellBitmap.Width()
	cellH := p.CellBitmap.Height()
	if cellW <= 0 || cellH <= 0 || p.XStep <= 0 || p.YStep <= 0 {
		return 0, 0, false
	}
	bboxW := p.BBox[2] - p.BBox[0]
	bboxH := p.BBox[3] - p.BBox[1]
	if bboxW <= 0 || bboxH <= 0 {
		return 0, 0, false
	}
	fx := float64(x)
	fy := float64(y)
	px, py := applyAffine(p.invMatrix, fx, fy)
	// Position within step [0, XStep) × [0, YStep) relative to bbox origin.
	cellX := posMod(px-p.BBox[0], p.XStep)
	cellY := posMod(py-p.BBox[1], p.YStep)
	// Step gap: when step > bbox extent, the tail of the step is outside the
	// cell — that region is a pattern hole (PDF 1.7 §8.7.3.3 spacing tiles).
	if cellX >= bboxW || cellY >= bboxH {
		return 0, 0, false
	}
	// cellW / bboxW == render scaleX (modulo ceil rounding); using this ratio
	// (instead of cellW / XStep) preserves the cell pixel layout when step
	// differs from bbox extent (overlapping or gapped cells).
	bx := int(math.Floor(cellX * float64(cellW) / bboxW))
	by := int(math.Floor(cellY * float64(cellH) / bboxH))
	if bx < 0 {
		bx = 0
	}
	if bx >= cellW {
		bx = cellW - 1
	}
	if by < 0 {
		by = 0
	}
	if by >= cellH {
		by = cellH - 1
	}
	return bx, by, true
}

// GetColor evaluates the tiling at device pixel (x, y) (SplashPattern.h:47).
//
// Device→pattern→cell-bitmap pipeline:
//  1. Sample at (float64(x), float64(y)).
//  2. Inverse-Matrix into pattern space.
//  3. Modulo into [0, XStep) × [0, YStep) relative to BBox origin.
//  4. Scale by cellW/(bbox[2]-bbox[0]) (== render scaleX, after ceiling rounding)
//     to get cell-bitmap pixel coordinates. This preserves the legacy stamp
//     semantics where step != cell extent — positions inside the step but
//     outside the cell bbox are pattern holes (return false).
//  5. PaintType=2: Poppler colorizes the grayscale cell with the parent tint
//     and carries the cell alpha separately through drawImage.
func (p *TilingPattern) GetColor(x, y int, c *Color) bool {
	if c == nil {
		return false
	}
	bx, by, ok := p.sampleCellPixel(x, y)
	if !ok {
		return false
	}
	cellW := p.CellBitmap.Width()
	if p.cellHasAlpha {
		alpha := p.CellBitmap.Alpha()
		idx := by*cellW + bx
		if idx < 0 || idx >= len(alpha) || alpha[idx] == 0 {
			return false
		}
	}
	readBitmapPixel(p.CellBitmap, bx, by, c)
	if p.PaintType == 2 {
		tintUncoloredCell(c, &p.TintColor, p.ColorMode)
	}
	return true
}

// PatternAlpha returns the pre-rendered cell alpha at a tiled device pixel.
// Poppler keeps this alpha separate from PaintType=2 colorization in
// tilingBitmapSrc(), then lets drawImage composite with it.
func (p *TilingPattern) PatternAlpha(x, y int) byte {
	if !p.cellHasAlpha {
		return 255
	}
	bx, by, ok := p.sampleCellPixel(x, y)
	if !ok {
		return 0
	}
	alpha := p.CellBitmap.Alpha()
	cellW := p.CellBitmap.Width()
	idx := by*cellW + bx
	if idx < 0 || idx >= len(alpha) {
		return 0
	}
	return alpha[idx]
}

// readBitmapPixel reads bm pixel (x, y) into c per the bitmap's color mode (SplashBitmap.h:102).
func readBitmapPixel(bm *Bitmap, x, y int, c *Color) {
	bpp := bytesPerPixel(bm.Mode())
	rs := rowStride(bm, bpp)
	off := y*rs + x*bpp
	data := bm.Data()
	if off < 0 || off+bpp > len(data) {
		*c = Color{}
		return
	}
	var out Color
	for i := 0; i < bpp; i++ {
		out[i] = data[off+i]
	}
	*c = out
}

// tintUncoloredCell mirrors Poppler's tilingBitmapSrc PaintType=2 colorization:
// RGB output is white at cell gray 255 and the parent tint at cell gray 0.
func tintUncoloredCell(c *Color, tint *Color, mode ColorMode) {
	bpp := bytesPerPixel(mode)
	if bpp <= 0 {
		return
	}
	gray := int(c[0])
	for i := 0; i < bpp; i++ {
		if mode == ModeCMYK8 || mode == ModeDeviceN8 {
			c[i] = byte(Div255(int(tint[i]) * (255 - gray)))
		} else {
			c[i] = byte(255 - Div255((255-int(tint[i]))*(255-gray)))
		}
	}
}

// TestPosition reports the pixel as covered — tilings span the entire fill region (SplashPattern.h:50).
func (p *TilingPattern) TestPosition(x, y int) bool { return true }

// IsStatic reports whether the pattern is constant — tilings vary per pixel (SplashPattern.h:54).
func (p *TilingPattern) IsStatic() bool { return false }

// IsCMYK reports whether the cell bitmap uses a CMYK or DeviceN color mode (SplashPattern.h:57).
func (p *TilingPattern) IsCMYK() bool {
	if p.CellBitmap == nil {
		return false
	}
	m := p.CellBitmap.Mode()
	if p.PaintType == 2 {
		m = p.ColorMode
	}
	return m == ModeCMYK8 || m == ModeDeviceN8
}

// FillWithTilingPattern installs pat as the fill pattern and runs Fill (Splash.cc:2324 fill driver).
func (sp *Splash) FillWithTilingPattern(pat *TilingPattern, p *xpath.Path, eo bool) error {
	if pat == nil {
		return ErrBadArg
	}
	sp.SetFillPattern(pat)
	return sp.fillImpl(p, eo)
}
