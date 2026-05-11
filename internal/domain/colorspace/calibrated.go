// Package colorspace provides PDF color space models.
//
//revive:disable:exported
package colorspace

import (
	"image/color"
	"math"
)

// CalGray represents a calibrated gray color space
type CalGray struct {
	whitePoint [3]float64 // XW, YW, ZW (YW must be 1.0)
	blackPoint [3]float64 // XB, YB, ZB (default [0, 0, 0])
	gamma      float64    // Gamma value (default 1.0)
}

// NewCalGray creates a new CalGray color space
func NewCalGray(whitePoint, blackPoint [3]float64, gamma float64) (*CalGray, error) {
	// Validate whitePoint (YW must be 1.0)
	if whitePoint[0] < 0 || whitePoint[2] < 0 || whitePoint[1] != 1.0 {
		// Use default D65 whitePoint
		whitePoint = [3]float64{0.9505, 1.0, 1.0890}
	}

	// Validate blackPoint
	if blackPoint[0] < 0 || blackPoint[1] < 0 || blackPoint[2] < 0 {
		blackPoint = [3]float64{0, 0, 0}
	}

	// Validate gamma
	if gamma < 1.0 {
		gamma = 1.0
	}

	return &CalGray{
		whitePoint: whitePoint,
		blackPoint: blackPoint,
		gamma:      gamma,
	}, nil
}

// Type returns ColorSpaceCalGray
func (cs *CalGray) Type() ColorSpaceType {
	return ColorSpaceCalGray
}

// Name returns "CalGray"
func (cs *CalGray) Name() string {
	return "CalGray"
}

// GetNumComponents returns 1
func (cs *CalGray) GetNumComponents() int {
	return 1
}

// ConvertToRGBA converts a calibrated gray value to RGBA
func (cs *CalGray) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) == 0 {
		return color.RGBA{0, 0, 0, 255}
	}

	// A represents a gray component
	A := values[0]

	// Apply gamma correction: AG = A^gamma
	AG := math.Pow(A, cs.gamma)

	// Compute L (luminance): L = YW * AG
	L := cs.whitePoint[1] * AG

	// Convert to RGB using simplified formula
	// Based on http://www.poynton.com/notes/colour_and_gamma/ColorFAQ.html
	val := math.Max(295.8*math.Pow(L, 1.0/3.0)-40.8, 0)

	// Clamp to [0, 255]
	if val > 255 {
		val = 255
	}

	v := uint8(val)
	return color.RGBA{R: v, G: v, B: v, A: 255}
}

// CalRGB represents a calibrated RGB color space
type CalRGB struct {
	whitePoint [3]float64 // XW, YW, ZW
	blackPoint [3]float64 // XB, YB, ZB
	gamma      [3]float64 // GR, GG, GB
	matrix     [9]float64 // 3x3 matrix for RGB to XYZ conversion
}

// Bradford chromatic adaptation matrices
var (
	bradfordScaleMatrix = [9]float64{
		0.8951, 0.2664, -0.1614,
		-0.7502, 1.7135, 0.0367,
		0.0389, -0.0685, 1.0296,
	}

	bradfordScaleInverseMatrix = [9]float64{
		0.9869929, -0.1470543, 0.1599627,
		0.4323053, 0.5183603, 0.0492912,
		-0.0085287, 0.0400428, 0.9684867,
	}

	// sRGB D65 XYZ to RGB conversion matrix
	srgbD65XYZToRGBMatrix = [9]float64{
		3.2404542, -1.5371385, -0.4985314,
		-0.9692660, 1.8760108, 0.0415560,
		0.0556434, -0.2040259, 1.0572252,
	}

	flatWhitePoint = [3]float64{1, 1, 1}
)

const decodeLConstant = ((8 + 16) / 116) * ((8 + 16) / 116) * ((8 + 16) / 116) / 8.0

// NewCalRGB creates a new CalRGB color space
func NewCalRGB(whitePoint, blackPoint, gamma [3]float64, matrix [9]float64) (*CalRGB, error) {
	// Validate whitePoint
	if whitePoint[0] < 0 || whitePoint[2] < 0 || whitePoint[1] != 1.0 {
		// Use default D65 whitePoint
		whitePoint = [3]float64{0.9505, 1.0, 1.0890}
	}

	// Validate blackPoint
	if blackPoint[0] < 0 || blackPoint[1] < 0 || blackPoint[2] < 0 {
		blackPoint = [3]float64{0, 0, 0}
	}

	// Validate gamma
	if gamma[0] < 0 || gamma[1] < 0 || gamma[2] < 0 {
		gamma = [3]float64{1, 1, 1}
	}

	// Default matrix is identity
	if matrix == [9]float64{} {
		matrix = [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}
	}

	return &CalRGB{
		whitePoint: whitePoint,
		blackPoint: blackPoint,
		gamma:      gamma,
		matrix:     matrix,
	}, nil
}

// Type returns ColorSpaceCalRGB
func (cs *CalRGB) Type() ColorSpaceType {
	return ColorSpaceCalRGB
}

// Name returns "CalRGB"
func (cs *CalRGB) Name() string {
	return "CalRGB"
}

// GetNumComponents returns 3
func (cs *CalRGB) GetNumComponents() int {
	return 3
}

// ConvertToRGBA converts calibrated RGB values to RGBA
func (cs *CalRGB) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) < 3 {
		return color.RGBA{0, 0, 0, 255}
	}

	// Clamp input values to [0, 1]
	A := clamp01(values[0])
	B := clamp01(values[1])
	C := clamp01(values[2])

	// Apply gamma correction
	var AGR, BGG, CGB float64
	if A == 1 {
		AGR = 1
	} else {
		AGR = math.Pow(A, cs.gamma[0])
	}
	if B == 1 {
		BGG = 1
	} else {
		BGG = math.Pow(B, cs.gamma[1])
	}
	if C == 1 {
		CGB = 1
	} else {
		CGB = math.Pow(C, cs.gamma[2])
	}

	// Convert to XYZ using matrix
	X := cs.matrix[0]*AGR + cs.matrix[1]*BGG + cs.matrix[2]*CGB
	Y := cs.matrix[3]*AGR + cs.matrix[4]*BGG + cs.matrix[5]*CGB
	Z := cs.matrix[6]*AGR + cs.matrix[7]*BGG + cs.matrix[8]*CGB

	// Normalize whitePoint to flat
	XYZ := [3]float64{X, Y, Z}
	XYZFlat := cs.normalizeWhitePointToFlat(cs.whitePoint, XYZ)

	// Compensate blackPoint
	XYZBlack := cs.compensateBlackPoint(cs.blackPoint, XYZFlat)

	// Normalize to D65
	XYZD65 := cs.normalizeWhitePointToD65(flatWhitePoint, XYZBlack)

	// Convert XYZ to sRGB
	SRGB := matrixProduct(srgbD65XYZToRGBMatrix, XYZD65)

	// Apply sRGB transfer function and convert to [0, 255]
	r := srgbTransferFunction(SRGB[0]) * 255
	g := srgbTransferFunction(SRGB[1]) * 255
	b := srgbTransferFunction(SRGB[2]) * 255

	return color.RGBA{
		R: uint8(math.Round(clamp(r, 0, 255))),
		G: uint8(math.Round(clamp(g, 0, 255))),
		B: uint8(math.Round(clamp(b, 0, 255))),
		A: 255,
	}
}

// matrixProduct multiplies a 3x3 matrix with a 3-element vector
func matrixProduct(matrix [9]float64, vec [3]float64) [3]float64 {
	return [3]float64{
		matrix[0]*vec[0] + matrix[1]*vec[1] + matrix[2]*vec[2],
		matrix[3]*vec[0] + matrix[4]*vec[1] + matrix[5]*vec[2],
		matrix[6]*vec[0] + matrix[7]*vec[1] + matrix[8]*vec[2],
	}
}

// toFlat converts lms to flat whitePoint
func toFlat(sourceWhitePoint, lms [3]float64) [3]float64 {
	return [3]float64{
		lms[0] * 1.0 / sourceWhitePoint[0],
		lms[1] * 1.0 / sourceWhitePoint[1],
		lms[2] * 1.0 / sourceWhitePoint[2],
	}
}

// toD65 converts lms to D65 whitePoint
func toD65(sourceWhitePoint, lms [3]float64) [3]float64 {
	const (
		D65X = 0.95047
		D65Y = 1.0
		D65Z = 1.08883
	)
	return [3]float64{
		lms[0] * D65X / sourceWhitePoint[0],
		lms[1] * D65Y / sourceWhitePoint[1],
		lms[2] * D65Z / sourceWhitePoint[2],
	}
}

// srgbTransferFunction applies sRGB gamma correction
func srgbTransferFunction(color float64) float64 {
	if color <= 0.0031308 {
		return clamp01(12.92 * color)
	}
	if color >= 0.99554525 {
		return 1.0
	}
	return clamp01((1.0+0.055)*math.Pow(color, 1.0/2.4) - 0.055)
}

// decodeL decodes l* value
func decodeL(l float64) float64 {
	if l < 0 {
		return -decodeL(-l)
	}
	if l > 8.0 {
		val := (l + 16) / 116
		return val * val * val
	}
	return l * decodeLConstant
}

// normalizeWhitePointToFlat normalizes XYZ to flat whitePoint
func (cs *CalRGB) normalizeWhitePointToFlat(sourceWhitePoint, xyzIn [3]float64) [3]float64 {
	// If already flat, no normalization needed
	if sourceWhitePoint[0] == 1 && sourceWhitePoint[2] == 1 {
		return xyzIn
	}

	lms := matrixProduct(bradfordScaleMatrix, xyzIn)
	lmsFlat := toFlat(sourceWhitePoint, lms)
	return matrixProduct(bradfordScaleInverseMatrix, lmsFlat)
}

// normalizeWhitePointToD65 normalizes XYZ to D65 whitePoint
func (cs *CalRGB) normalizeWhitePointToD65(sourceWhitePoint, xyzIn [3]float64) [3]float64 {
	lms := matrixProduct(bradfordScaleMatrix, xyzIn)
	lmsD65 := toD65(sourceWhitePoint, lms)
	return matrixProduct(bradfordScaleInverseMatrix, lmsD65)
}

// compensateBlackPoint applies blackPoint compensation
func (cs *CalRGB) compensateBlackPoint(sourceBlackPoint, xyzFlat [3]float64) [3]float64 {
	// If blackPoint is default, no compensation needed
	if sourceBlackPoint[0] == 0 && sourceBlackPoint[1] == 0 && sourceBlackPoint[2] == 0 {
		return xyzFlat
	}

	zeroDecodeL := decodeL(0)

	xDst := zeroDecodeL
	xSrc := decodeL(sourceBlackPoint[0])

	yDst := zeroDecodeL
	ySrc := decodeL(sourceBlackPoint[1])

	zDst := zeroDecodeL
	zSrc := decodeL(sourceBlackPoint[2])

	xScale := (1.0 - xDst) / (1.0 - xSrc)
	xOffset := 1 - xScale

	yScale := (1.0 - yDst) / (1.0 - ySrc)
	yOffset := 1 - yScale

	zScale := (1.0 - zDst) / (1.0 - zSrc)
	zOffset := 1 - zScale

	return [3]float64{
		xyzFlat[0]*xScale + xOffset,
		xyzFlat[1]*yScale + yOffset,
		xyzFlat[2]*zScale + zOffset,
	}
}

// Helper functions
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
