package colorspace

import (
	"image/color"
	"math"
)

// Lab represents a CIE L*a*b* color space
type Lab struct {
	whitePoint [3]float64 // XW, YW, ZW (YW must be 1.0)
	blackPoint [3]float64 // XB, YB, ZB (not used in Lab formulas, kept for completeness)
	rangeA     [2]float64 // amin, amax (default [-100, 100])
	rangeB     [2]float64 // bmin, bmax (default [-100, 100])
}

// NewLab creates a new Lab color space
func NewLab(whitePoint, blackPoint [3]float64, rangeA, rangeB [2]float64) (*Lab, error) {
	// Validate whitePoint (YW must be 1.0)
	if whitePoint[0] < 0 || whitePoint[2] < 0 || whitePoint[1] != 1.0 {
		// Use default D65 whitePoint
		whitePoint = [3]float64{0.9505, 1.0, 1.0890}
	}

	// Validate blackPoint (not used in Lab, but validate anyway)
	if blackPoint[0] < 0 || blackPoint[1] < 0 || blackPoint[2] < 0 {
		blackPoint = [3]float64{0, 0, 0}
	}

	// Validate ranges
	if rangeA[0] > rangeA[1] {
		rangeA = [2]float64{-100, 100}
	}
	if rangeB[0] > rangeB[1] {
		rangeB = [2]float64{-100, 100}
	}

	// Use default if not specified
	if rangeA == [2]float64{0, 0} {
		rangeA = [2]float64{-100, 100}
	}
	if rangeB == [2]float64{0, 0} {
		rangeB = [2]float64{-100, 100}
	}

	return &Lab{
		whitePoint: whitePoint,
		blackPoint: blackPoint,
		rangeA:     rangeA,
		rangeB:     rangeB,
	}, nil
}

// Type returns ColorSpaceLab
func (cs *Lab) Type() ColorSpaceType {
	return ColorSpaceLab
}

// Name returns "Lab"
func (cs *Lab) Name() string {
	return "Lab"
}

// GetNumComponents returns 3
func (cs *Lab) GetNumComponents() int {
	return 3
}

// ConvertToRGBA converts Lab values to RGBA
// Input: [L*, a*, b*] where L* in [0, 100], a* and b* in range
func (cs *Lab) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) < 3 {
		return color.RGBA{0, 0, 0, 255}
	}

	// Get L*, a*, b* values (already in correct range [0,100], [amin,amax], [bmin,bmax])
	Ls := values[0]
	as := values[1]
	bs := values[2]

	// Clamp a* and b* to their ranges
	if as > cs.rangeA[1] {
		as = cs.rangeA[1]
	} else if as < cs.rangeA[0] {
		as = cs.rangeA[0]
	}
	if bs > cs.rangeB[1] {
		bs = cs.rangeB[1]
	} else if bs < cs.rangeB[0] {
		bs = cs.rangeB[0]
	}

	// Compute intermediate variables X, Y, Z
	M := (Ls + 16) / 116
	L := M + as/500
	N := M - bs/200

	X := cs.whitePoint[0] * fnG(L)
	Y := cs.whitePoint[1] * fnG(M)
	Z := cs.whitePoint[2] * fnG(N)

	var r, g, b float64

	// Use different conversion matrices for D50 and D65 white points
	// per http://www.color.org/srgb.pdf
	if cs.whitePoint[2] < 1 {
		// D50 (X=0.9642, Y=1.00, Z=0.8249)
		r = X*3.1339 + Y*-1.617 + Z*-0.4906
		g = X*-0.9785 + Y*1.916 + Z*0.0333
		b = X*0.072 + Y*-0.229 + Z*1.4057
	} else {
		// D65 (X=0.9505, Y=1.00, Z=1.0888)
		r = X*3.2406 + Y*-1.5372 + Z*-0.4986
		g = X*-0.9689 + Y*1.8758 + Z*0.0415
		b = X*0.0557 + Y*-0.204 + Z*1.057
	}

	// Apply sqrt and convert to [0, 255]
	// Note: sqrt is used instead of the standard sRGB transfer function
	rVal := math.Sqrt(math.Max(r, 0)) * 255
	gVal := math.Sqrt(math.Max(g, 0)) * 255
	bVal := math.Sqrt(math.Max(b, 0)) * 255

	return color.RGBA{
		R: uint8(math.Round(clampLab(rVal, 0, 255))),
		G: uint8(math.Round(clampLab(gVal, 0, 255))),
		B: uint8(math.Round(clampLab(bVal, 0, 255))),
		A: 255,
	}
}

// fnG is the g(x) function from the Lab spec
// g(x) = x^3 if x >= 6/29, else (108/841) * (x - 4/29)
func fnG(x float64) float64 {
	const threshold = 6.0 / 29.0
	if x >= threshold {
		return x * x * x
	}
	return (108.0 / 841.0) * (x - 4.0/29.0)
}

// clampLab clamps a value to [min, max]
func clampLab(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
