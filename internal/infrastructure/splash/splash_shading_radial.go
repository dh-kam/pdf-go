package splash

import (
	"fmt"
	"math"
	"os"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// radialDegenerateEpsilon mirrors Poppler's RADIAL_EPSILON
// (SplashOutputDev.cc:329).
const radialDegenerateEpsilon = 1.0 / 1024.0 / 1024.0

var radialTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_RADIAL_PIXEL_TRACE"))

// RadialShader implements Pattern for PDF Shading Type 3 (radial), evaluated by
// the generic shaded-fill driver at Splash.cc:6240 (PDF 1.7 §8.7.4.5.4).
//
// A radial gradient is parameterized by two circles (x0,y0,r0) at t=0 and
// (x1,y1,r1) at t=1. For a device pixel we solve a quadratic in t such that
// the pixel lies on the interpolated circle of center (xc(t), yc(t)) and
// radius r(t). The larger non-negative root in [0,1] is preferred to match
// Splash's outer-circle convention.
type RadialShader struct {
	// X0, Y0, R0 are the center+radius at t=0 (PDF 1.7 §8.7.4.5.4).
	X0, Y0, R0 float64
	// X1, Y1, R1 are the center+radius at t=1 (PDF 1.7 §8.7.4.5.4).
	X1, Y1, R1 float64
	// T0, T1 are the function-domain endpoints (PDF 1.7 §8.7.4.5.4).
	T0, T1 float64
	// Extend0 extends shading before T0 (PDF 1.7 §8.7.4.5.4 /Extend).
	Extend0 bool
	// Extend1 extends shading after T1 (PDF 1.7 §8.7.4.5.4 /Extend).
	Extend1 bool
	// Func evaluates t→Color over [T0, T1] (PDF 1.7 §8.7.4.5.4 /Function).
	Func func(t float64) Color
	// Mode is the bitmap color mode (SplashTypes.h:56).
	Mode ColorMode
	// Transform maps Splash device coordinates back into shading coordinates.
	// Poppler's SplashUnivariatePattern stores the inverse CTM and applies it
	// per sample before solving the radial parameter.
	Transform func(x, y float64) (float64, float64)

	// Pre-computed quadratic coefficients (constant in pixel coords).
	dxAxis, dyAxis, dr float64
	a                  float64
	degenerate         bool
}

// NewRadialShader builds a RadialShader between two circles (Splash.cc:6240).
func NewRadialShader(x0, y0, r0, x1, y1, r1, t0, t1 float64, extend0, extend1 bool, fn func(t float64) Color, mode ColorMode) *RadialShader {
	s := &RadialShader{
		X0: x0, Y0: y0, R0: r0,
		X1: x1, Y1: y1, R1: r1,
		T0: t0, T1: t1,
		Extend0: extend0,
		Extend1: extend1,
		Func:    fn,
		Mode:    mode,
	}
	s.dxAxis = x1 - x0
	s.dyAxis = y1 - y0
	s.dr = r1 - r0
	s.a = s.dxAxis*s.dxAxis + s.dyAxis*s.dyAxis - s.dr*s.dr
	if math.Abs(s.a) < radialDegenerateEpsilon {
		s.degenerate = true
	}
	return s
}

// NewRadialShaderWithTransform builds a RadialShader with an explicit
// device-to-shading coordinate transform.
func NewRadialShaderWithTransform(x0, y0, r0, x1, y1, r1, t0, t1 float64, extend0, extend1 bool, fn func(t float64) Color, mode ColorMode, transform func(x, y float64) (float64, float64)) *RadialShader {
	s := NewRadialShader(x0, y0, r0, x1, y1, r1, t0, t1, extend0, extend1, fn, mode)
	s.Transform = transform
	return s
}

// computeT solves the radial quadratic for t at device pixel (fx, fy), mirroring
// Poppler's SplashRadialPattern::getParameter().
//
// (px - xc(t))² + (py - yc(t))² = r(t)²
// expands to a*t² + b*t + c = 0 with
//
//	a = dxAxis² + dyAxis² - dr²
//	b = ex·dxAxis + ey·dyAxis + r0·dr
//	c = ex² + ey² - r0²
//
// where ex = px-x0, ey = py-y0. The precomputed `a` is constant per shader.
func (s *RadialShader) computeT(fx, fy float64) (t float64, inside bool) {
	ex := fx - s.X0
	ey := fy - s.Y0
	b := ex*s.dxAxis + ey*s.dyAxis + s.R0*s.dr
	c := ex*ex + ey*ey - s.R0*s.R0

	if s.degenerate {
		if math.Abs(b) < radialDegenerateEpsilon {
			return 0, false
		}
		s0 := 0.5 * c / b
		return s.selectPopplerRoot(s0, s0)
	}

	disc := b*b - s.a*c
	if disc < 0 {
		return 0, false
	}
	sq := math.Sqrt(disc)
	return s.selectPopplerRoot((b+sq)/s.a, (b-sq)/s.a)
}

func (s *RadialShader) selectPopplerRoot(s0, s1 float64) (float64, bool) {
	if t, ok := s.validatePopplerRoot(s0); ok {
		return t, true
	}
	return s.validatePopplerRoot(s1)
}

func (s *RadialShader) validatePopplerRoot(root float64) (float64, bool) {
	if s.R0+root*s.dr < 0 {
		return 0, false
	}
	switch {
	case root >= 0 && root <= 1:
		return root, true
	case root < 0 && s.Extend0:
		return 0, true
	case root > 1 && s.Extend1:
		return 1, true
	default:
		return 0, false
	}
}

// GetColor evaluates the shader at device pixel (x, y) (SplashPattern.h:47).
//
// Poppler's shaded-fill scanline evaluates radial colors at integer device
// coordinates after the shading coordinates have been mapped to Splash y-down.
func (s *RadialShader) GetColor(x, y int, c *Color) bool {
	if c == nil || s.Func == nil {
		return false
	}
	fx := float64(x)
	fy := float64(y)
	if s.Transform != nil {
		fx, fy = s.Transform(fx, fy)
	}
	t, ok := s.computeT(fx, fy)
	if !ok {
		return false
	}
	funcT := s.T0 + t*(s.T1-s.T0)
	*c = s.Func(funcT)
	if shouldTraceRadialPixel(x, y) {
		fmt.Fprintf(os.Stderr, "SPLASH_RADIAL_PIXEL_TRACE x=%d y=%d fx=%.12f fy=%.12f s=%.12f t=%.12f color=(%d,%d,%d)\n",
			x, y, fx, fy, t, funcT, c[0], c[1], c[2])
	}
	return true
}

// TestPosition reports whether (x, y) is covered by the shading (SplashPattern.h:50).
func (s *RadialShader) TestPosition(x, y int) bool {
	fx := float64(x)
	fy := float64(y)
	if s.Transform != nil {
		fx, fy = s.Transform(fx, fy)
	}
	t, ok := s.computeT(fx, fy)
	if !ok {
		return false
	}
	funcT := s.T0 + t*(s.T1-s.T0)
	if s.T0 < s.T1 {
		return funcT > s.T0 && funcT < s.T1
	}
	return funcT > s.T1 && funcT < s.T0
}

// IsStatic reports whether the pattern is constant — radial varies (SplashPattern.h:54).
func (s *RadialShader) IsStatic() bool { return false }

// IsCMYK reports whether the pattern emits CMYK colors (SplashPattern.h:57).
func (s *RadialShader) IsCMYK() bool {
	return s.Mode == ModeCMYK8 || s.Mode == ModeDeviceN8
}

func shouldTraceRadialPixel(x, y int) bool {
	for _, pixel := range radialTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

// FillRadialShading installs s as the fill pattern and runs the shaded-fill driver (Splash.cc:6240).
func (sp *Splash) FillRadialShading(shader *RadialShader, p *xpath.Path, eo bool) error {
	_ = eo
	return sp.FillRadialShadingWithBBox(shader, p, false)
}

// FillRadialShadingWithBBox runs the Poppler shaded-fill path with the shading BBox flag.
func (sp *Splash) FillRadialShadingWithBBox(shader *RadialShader, p *xpath.Path, hasBBox bool) error {
	if shader == nil {
		return ErrBadArg
	}
	sp.SetFillPattern(shader)
	return sp.shadedFill(p, hasBBox, shader, false)
}
