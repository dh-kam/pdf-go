package splash

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// axialDegenerateEpsilon guards the axis length-squared against zero
// (Splash.cc::univariateShadedFill / shadedFill driver path, Splash.cc:6240).
const axialDegenerateEpsilon = 1e-20

var axialTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_AXIAL_PIXEL_TRACE"))

// AxialShader implements Pattern for PDF Shading Type 2 (axial), evaluated by
// the generic shaded-fill driver at Splash.cc:6240 (PDF 1.7 §8.7.4.5.3).
type AxialShader struct {
	// X0, Y0 are the axis start point in device space (PDF 1.7 §8.7.4.5.3).
	X0, Y0 float64
	// X1, Y1 are the axis end point in device space (PDF 1.7 §8.7.4.5.3).
	X1, Y1 float64
	// T0, T1 are the function-domain endpoints (PDF 1.7 §8.7.4.5.3).
	T0, T1 float64
	// Extend0 extends shading before T0 (PDF 1.7 §8.7.4.5.3 /Extend).
	Extend0 bool
	// Extend1 extends shading after T1 (PDF 1.7 §8.7.4.5.3 /Extend).
	Extend1 bool
	// Func evaluates t→Color over [T0, T1] (PDF 1.7 §8.7.4.5.3 /Function).
	Func func(t float64) Color
	// Mode is the bitmap color mode (SplashTypes.h:56).
	Mode ColorMode
	// Transform maps Splash device coordinates back into shading coordinates.
	// Poppler's SplashUnivariatePattern stores the inverse CTM and applies it
	// per sample before solving the axial parameter.
	Transform func(x, y float64) (float64, float64)

	// Pre-computed axis vector and inverse length-squared.
	dx, dy     float64
	invLen2    float64
	degenerate bool
}

// NewAxialShader builds an AxialShader covering coord0→coord1 in device space (Splash.cc:6240).
func NewAxialShader(x0, y0, x1, y1, t0, t1 float64, extend0, extend1 bool, fn func(t float64) Color, mode ColorMode) *AxialShader {
	s := &AxialShader{
		X0: x0, Y0: y0,
		X1: x1, Y1: y1,
		T0: t0, T1: t1,
		Extend0: extend0,
		Extend1: extend1,
		Func:    fn,
		Mode:    mode,
	}
	s.dx = x1 - x0
	s.dy = y1 - y0
	len2 := s.dx*s.dx + s.dy*s.dy
	if len2 < axialDegenerateEpsilon {
		s.degenerate = true
		s.invLen2 = 0
	} else {
		s.invLen2 = 1.0 / len2
	}
	return s
}

// NewAxialShaderWithTransform builds an AxialShader with an explicit
// device-to-shading coordinate transform.
func NewAxialShaderWithTransform(x0, y0, x1, y1, t0, t1 float64, extend0, extend1 bool, fn func(t float64) Color, mode ColorMode, transform func(x, y float64) (float64, float64)) *AxialShader {
	s := NewAxialShader(x0, y0, x1, y1, t0, t1, extend0, extend1, fn, mode)
	s.Transform = transform
	return s
}

// computeT projects device pixel (fx, fy) onto the shading axis (PDF 1.7 §8.7.4.5.3).
func (s *AxialShader) computeT(fx, fy float64) (t float64, inside bool) {
	if s.degenerate {
		return 0, true
	}
	t = ((fx-s.X0)*s.dx + (fy-s.Y0)*s.dy) * s.invLen2
	if t < 0 {
		if s.Extend0 {
			return 0, true
		}
		return 0, false
	}
	if t > 1 {
		if s.Extend1 {
			return 1, true
		}
		return 1, false
	}
	return t, true
}

// GetColor evaluates the shader at device pixel (x, y) (SplashPattern.h:47).
//
// When Transform is set, this mirrors Poppler's SplashUnivariatePattern:
// integer Splash device coordinates are transformed back through the inverse
// CTM before the axial parameter is solved. The nil-transform branch preserves
// the older pre-port convention for direct unit tests.
func (s *AxialShader) GetColor(x, y int, c *Color) bool {
	if c == nil || s.Func == nil {
		return false
	}
	fx := float64(x)
	fy := float64(y)
	if s.Transform != nil {
		fx, fy = s.Transform(fx, fy)
	} else {
		fy += 1.0
	}
	t, ok := s.computeT(fx, fy)
	if !ok {
		return false
	}
	funcT := s.T0 + t*(s.T1-s.T0)
	*c = s.Func(funcT)
	if shouldTraceAxialPixel(x, y) {
		fmt.Fprintf(os.Stderr, "SPLASH_AXIAL_PIXEL_TRACE x=%d y=%d fx=%.12f fy=%.12f s=%.12f t=%.12f color=(%d,%d,%d)\n",
			x, y, fx, fy, t, funcT, c[0], c[1], c[2])
	}
	return true
}

// TestPosition reports whether (x, y) is covered by the shading (SplashPattern.h:50).
func (s *AxialShader) TestPosition(x, y int) bool {
	fx := float64(x)
	fy := float64(y)
	if s.Transform != nil {
		fx, fy = s.Transform(fx, fy)
	} else {
		fy += 1.0
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

// IsStatic reports whether the pattern is constant — axial varies (SplashPattern.h:54).
func (s *AxialShader) IsStatic() bool { return false }

// IsCMYK reports whether the pattern emits CMYK colors (SplashPattern.h:57).
func (s *AxialShader) IsCMYK() bool {
	return s.Mode == ModeCMYK8 || s.Mode == ModeDeviceN8
}

func shouldTraceAxialPixel(x, y int) bool {
	for _, pixel := range axialTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

// FillAxialShading installs s as the fill pattern and runs the shaded-fill driver (Splash.cc:6240).
func (sp *Splash) FillAxialShading(shader *AxialShader, p *xpath.Path, eo bool) error {
	_ = eo
	return sp.FillAxialShadingWithBBox(shader, p, false)
}

// FillAxialShadingWithBBox runs the Poppler shaded-fill path with the shading BBox flag.
func (sp *Splash) FillAxialShadingWithBBox(shader *AxialShader, p *xpath.Path, hasBBox bool) error {
	if shader == nil {
		return ErrBadArg
	}
	sp.SetFillPattern(shader)
	return sp.shadedFill(p, hasBBox, shader, false)
}
