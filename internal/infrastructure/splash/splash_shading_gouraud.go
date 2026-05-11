package splash

import (
	"fmt"
	"os"
	"sync/atomic"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

var (
	gouraudStatsDirectCalls      uint64
	gouraudStatsPatternCalls     uint64
	gouraudStatsPatternHits      uint64
	gouraudStatsPatternParamHits uint64
	gouraudStatsPatternAnnounced uint32
	gouraudTracePixels           = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_GOURAUD_TRACE"))
)

func gouraudStatsEnabled() bool {
	return os.Getenv("PDF_DEBUG_SPLASH_GOURAUD_STATS") != ""
}

func recordGouraudPatternStats(hit, parameterized bool) {
	if !gouraudStatsEnabled() {
		return
	}
	total := atomic.AddUint64(&gouraudStatsPatternCalls, 1)
	hits := atomic.LoadUint64(&gouraudStatsPatternHits)
	paramHits := atomic.LoadUint64(&gouraudStatsPatternParamHits)
	if hit {
		hits = atomic.AddUint64(&gouraudStatsPatternHits, 1)
		if parameterized {
			paramHits = atomic.AddUint64(&gouraudStatsPatternParamHits, 1)
		}
	}
	if atomic.CompareAndSwapUint32(&gouraudStatsPatternAnnounced, 0, 1) || total%100000 == 0 {
		fmt.Fprintf(os.Stderr, "SPLASH_GOURAUD_STATS pattern_getcolor total=%d hits=%d misses=%d parameterized_hits=%d\n",
			total, hits, total-hits, paramHits)
	}
}

func shouldTraceGouraudPixel(x, y int) bool {
	for _, pixel := range gouraudTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

type gouraudDirectStats struct {
	callID               uint64
	triangles            int
	parameterized        int
	written              uint64
	clipPathWrites       uint64
	clipRejectedByBinary uint64
	clipMissing          uint64
	hasPath              bool
	evenOdd              bool
	alphaBitmap          bool
	fillAlpha            float64
	blendFunc            bool
	softMask             bool
}

func newGouraudDirectStats(sp *Splash, p *xpath.Path, eo bool) *gouraudDirectStats {
	if !gouraudStatsEnabled() {
		return nil
	}
	stats := &gouraudDirectStats{
		callID:    atomic.AddUint64(&gouraudStatsDirectCalls, 1),
		hasPath:   p != nil,
		evenOdd:   eo,
		fillAlpha: 1,
	}
	if sp != nil {
		if sp.bitmap != nil {
			stats.alphaBitmap = sp.bitmap.alpha != nil
		}
		if sp.state != nil {
			stats.fillAlpha = sp.state.fillAlpha
			stats.blendFunc = sp.state.blendFunc != nil
			stats.softMask = sp.state.softMask != nil
		}
	}
	return stats
}

func (s *gouraudDirectStats) print() {
	if s == nil {
		return
	}
	patternTotal := atomic.LoadUint64(&gouraudStatsPatternCalls)
	patternHits := atomic.LoadUint64(&gouraudStatsPatternHits)
	patternParamHits := atomic.LoadUint64(&gouraudStatsPatternParamHits)
	fmt.Fprintf(os.Stderr, "SPLASH_GOURAUD_STATS direct_fill call=%d triangles=%d parameterized_triangles=%d written=%d clip_path_writes=%d clip_rejected_by_binary_test=%d clip_missing=%d path_arg=%t even_odd=%t alpha_bitmap=%t fill_alpha=%.6f blend_func=%t soft_mask=%t pattern_getcolor_total=%d pattern_getcolor_hits=%d pattern_getcolor_parameterized_hits=%d\n",
		s.callID, s.triangles, s.parameterized, s.written, s.clipPathWrites,
		s.clipRejectedByBinary, s.clipMissing, s.hasPath, s.evenOdd,
		s.alphaBitmap, s.fillAlpha, s.blendFunc, s.softMask,
		patternTotal, patternHits, patternParamHits)
}

// GouraudVertex is one mesh vertex with device-space coords + per-channel color
// (Splash.cc::gouraudTriangleShadedFill, Splash.cc:5255).
type GouraudVertex struct {
	// X, Y are device-space coordinates (Splash.cc:5326-5333).
	X, Y float64
	// Color is the per-vertex color (Splash.cc:5324 / SplashPattern.h:91).
	Color Color
	// Params stores the raw vertex function inputs for parameterized Gouraud
	// meshes. Poppler interpolates these values and evaluates the Function per
	// pixel instead of interpolating pre-quantized RGB bytes.
	Params []float64
}

// GouraudShader implements PDF Shading Type 4/5 (Gouraud-shaded triangle mesh).
// Splash equivalent: Splash::gouraudTriangleShadedFill at Splash.cc:5255.
type GouraudShader struct {
	// Triangles is a flat list — every 3 vertices form one triangle
	// (Splash.cc:5323 getNTriangles loop).
	Triangles []GouraudVertex
	// Mode is the bitmap color mode (SplashTypes.h:56).
	Mode ColorMode
	// Functions and ColorSpace are set for parameterized shadings.
	Functions  []entity.Function
	ColorSpace string
}

// NewGouraudShader builds a GouraudShader from a flat triangle list (Splash.cc:5255).
//
// `triangles` must be a multiple of 3; partial trailing groups are dropped.
func NewGouraudShader(triangles []GouraudVertex, mode ColorMode) *GouraudShader {
	n := len(triangles) - len(triangles)%3
	out := make([]GouraudVertex, n)
	copy(out, triangles[:n])
	return &GouraudShader{Triangles: out, Mode: mode}
}

func NewParameterizedGouraudShader(triangles []GouraudVertex, mode ColorMode, functions []entity.Function, colorSpace string) *GouraudShader {
	shader := NewGouraudShader(triangles, mode)
	shader.Functions = append([]entity.Function(nil), functions...)
	shader.ColorSpace = colorSpace
	return shader
}

// nTriangles returns the number of triangles in the mesh (Splash.cc:5323).
func (s *GouraudShader) nTriangles() int { return len(s.Triangles) / 3 }

// triangleVerts returns the three vertices of triangle i (Splash.cc:5324).
func (s *GouraudShader) triangleVerts(i int) (GouraudVertex, GouraudVertex, GouraudVertex) {
	base := i * 3
	return s.Triangles[base], s.Triangles[base+1], s.Triangles[base+2]
}

// barycentric solves for (w0, w1, w2) at point (px, py) given triangle
// vertices in screen space. Returns ok=false on degenerate triangles.
// Mirrors the implicit barycentric inversion used by Splash.cc:5481-5494
// (where Poppler instead linearizes per scanline; we recompute directly so
// TestPosition is correct).
func barycentric(px, py float64, x0, y0, x1, y1, x2, y2 float64) (w0, w1, w2 float64, ok bool) {
	det := (x0-x2)*(y1-y2) - (x1-x2)*(y0-y2)
	if det > -1e-20 && det < 1e-20 {
		return 0, 0, 0, false
	}
	w0 = ((px-x2)*(y1-y2) - (x1-x2)*(py-y2)) / det
	w1 = ((x0-x2)*(py-y2) - (px-x2)*(y0-y2)) / det
	w2 = 1.0 - w0 - w1
	return w0, w1, w2, true
}

// pointInTriangle reports whether the barycentric weights are all in [0,1]
// (with a small epsilon for edge inclusion).
func pointInTriangle(w0, w1, w2 float64) bool {
	const eps = 1e-9
	return w0 >= -eps && w1 >= -eps && w2 >= -eps
}

// blendColor blends the three vertex colors c0/c1/c2 at barycentric weights
// (w0, w1, w2) and writes into out. Mirrors the per-component interpolation
// at Splash.cc:5481-5496 except we use direct barycentric weights instead of
// scanline-linearized form.
func blendColor(c0, c1, c2 Color, w0, w1, w2 float64, out *Color) {
	for k := 0; k < splashMaxColorComps; k++ {
		v := w0*float64(c0[k]) + w1*float64(c1[k]) + w2*float64(c2[k])
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		out[k] = byte(v + 0.5)
	}
}

func blendParams(p0, p1, p2 []float64, w0, w1, w2 float64) ([]float64, bool) {
	if len(p0) == 0 || len(p0) != len(p1) || len(p0) != len(p2) {
		return nil, false
	}
	out := make([]float64, len(p0))
	for i := range p0 {
		out[i] = w0*p0[i] + w1*p1[i] + w2*p2[i]
	}
	return out, true
}

// findContainingTriangle returns the index of the first triangle that
// contains (px, py), or -1 if none. Used by GetColor for the Pattern path.
func (s *GouraudShader) findContainingTriangle(px, py float64) (int, float64, float64, float64) {
	for i := 0; i < s.nTriangles(); i++ {
		v0, v1, v2 := s.triangleVerts(i)
		x0, y0 := float64(Round(v0.X)), float64(Round(v0.Y))
		x1, y1 := float64(Round(v1.X)), float64(Round(v1.Y))
		x2, y2 := float64(Round(v2.X)), float64(Round(v2.Y))
		w0, w1, w2, ok := barycentric(px, py, x0, y0, x1, y1, x2, y2)
		if !ok {
			continue
		}
		if pointInTriangle(w0, w1, w2) {
			return i, w0, w1, w2
		}
	}
	return -1, 0, 0, 0
}

// GetColor evaluates the Gouraud mesh at device pixel (x, y)
// (Splash.cc:5481-5496 inner loop).
//
// Poppler's Gouraud scanline path evaluates the interpolated color at integer
// device pixel coordinates while sweeping Y.
func (s *GouraudShader) GetColor(x, y int, c *Color) bool {
	if c == nil || len(s.Triangles) < 3 {
		recordGouraudPatternStats(false, len(s.Functions) > 0)
		return false
	}
	if len(s.Functions) > 0 {
		param, idx, ok := s.parameterizedScanlineValueWithTriangle(x, y)
		if !ok {
			recordGouraudPatternStats(false, true)
			return false
		}
		out, err := evalShadingFunctions(s.Functions, []float64{param})
		if err != nil || len(out) == 0 {
			recordGouraudPatternStats(false, true)
			return false
		}
		*c = packShadingOutput(out, s.ColorSpace, s.Mode)
		if shouldTraceGouraudPixel(x, y) {
			v0, v1, v2 := s.triangleVerts(idx)
			fmt.Fprintf(os.Stderr, "SPLASH_GOURAUD_TRACE x=%d y=%d tri=%d param=%.12f color=(%d,%d,%d) v0=(%.6f,%.6f,%.12f) v1=(%.6f,%.6f,%.12f) v2=(%.6f,%.6f,%.12f)\n",
				x, y, idx, param, (*c)[0], (*c)[1], (*c)[2],
				v0.X, v0.Y, firstParam(v0.Params),
				v1.X, v1.Y, firstParam(v1.Params),
				v2.X, v2.Y, firstParam(v2.Params))
		}
		recordGouraudPatternStats(true, true)
		return true
	}
	fx := float64(x)
	fy := float64(y)
	idx, w0, w1, w2 := s.findContainingTriangle(fx, fy)
	if idx < 0 {
		recordGouraudPatternStats(false, false)
		return false
	}
	v0, v1, v2 := s.triangleVerts(idx)
	blendColor(v0.Color, v1.Color, v2.Color, w0, w1, w2, c)
	if shouldTraceGouraudPixel(x, y) {
		fmt.Fprintf(os.Stderr, "SPLASH_GOURAUD_TRACE x=%d y=%d tri=%d weights=(%.12f,%.12f,%.12f) color=(%d,%d,%d) v0=(%.6f,%.6f) v1=(%.6f,%.6f) v2=(%.6f,%.6f)\n",
			x, y, idx, w0, w1, w2, (*c)[0], (*c)[1], (*c)[2],
			v0.X, v0.Y, v1.X, v1.Y, v2.X, v2.Y)
	}
	recordGouraudPatternStats(true, false)
	return true
}

func firstParam(params []float64) float64 {
	if len(params) == 0 {
		return 0
	}
	return params[0]
}

func (s *GouraudShader) parameterizedScanlineValue(px, py int) (float64, bool) {
	param, _, ok := s.parameterizedScanlineValueWithTriangle(px, py)
	return param, ok
}

func (s *GouraudShader) parameterizedScanlineValueWithTriangle(px, py int) (float64, int, bool) {
	for i := 0; i < s.nTriangles(); i++ {
		v0, v1, v2 := s.triangleVerts(i)
		if len(v0.Params) == 0 || len(v1.Params) == 0 || len(v2.Params) == 0 {
			continue
		}
		t := &gouraudVert3{}
		t.x[0], t.y[0], t.params[0] = Round(v0.X), Round(v0.Y), []float64{v0.Params[0]}
		t.x[1], t.y[1], t.params[1] = Round(v1.X), Round(v1.Y), []float64{v1.Params[0]}
		t.x[2], t.y[2], t.params[2] = Round(v2.X), Round(v2.Y), []float64{v2.Params[0]}
		if param, ok := gouraudParameterizedValueAt(t, px, py); ok {
			return param, i, true
		}
	}
	return 0, -1, false
}

// TestPosition reports whether (x, y) lies inside any triangle (SplashPattern.h:50).
func (s *GouraudShader) TestPosition(x, y int) bool {
	fx := float64(x)
	fy := float64(y)
	idx, _, _, _ := s.findContainingTriangle(fx, fy)
	return idx >= 0
}

// IsStatic always false — the mesh varies per pixel (SplashPattern.h:54).
func (s *GouraudShader) IsStatic() bool { return false }

// IsCMYK reports whether the pattern emits CMYK colors (SplashPattern.h:57).
func (s *GouraudShader) IsCMYK() bool {
	return s.Mode == ModeCMYK8 || s.Mode == ModeDeviceN8
}

// gouraudVert3 is the local sortable triple used for scanline rasterization
// (Splash.cc:5337-5364 insertion sort).
type gouraudVert3 struct {
	x          [3]int
	y          [3]int
	c          [3]Color
	params     [3][]float64
	functions  []entity.Function
	colorSpace string
	mode       ColorMode
}

// sortByY mirrors the insertion sort at Splash.cc:5337-5364 over the rounded
// integer vertex coords.
func (v *gouraudVert3) sortByY() {
	if v.y[0] > v.y[1] {
		v.x[0], v.x[1] = v.x[1], v.x[0]
		v.y[0], v.y[1] = v.y[1], v.y[0]
		v.c[0], v.c[1] = v.c[1], v.c[0]
		v.params[0], v.params[1] = v.params[1], v.params[0]
	}
	if v.y[1] > v.y[2] {
		tmpX, tmpY, tmpC, tmpParams := v.x[2], v.y[2], v.c[2], v.params[2]
		v.x[2], v.y[2], v.c[2] = v.x[1], v.y[1], v.c[1]
		v.params[2] = v.params[1]
		if v.y[0] > tmpY {
			v.x[1], v.y[1], v.c[1] = v.x[0], v.y[0], v.c[0]
			v.params[1] = v.params[0]
			v.x[0], v.y[0], v.c[0] = tmpX, tmpY, tmpC
			v.params[0] = tmpParams
		} else {
			v.x[1], v.y[1], v.c[1] = tmpX, tmpY, tmpC
			v.params[1] = tmpParams
		}
	}
}

// rasterizeTriangle scanline-rasterizes one triangle into the bitmap using
// the Poppler-style rounded-vertex sweep at Splash.cc:5337-5506. Per project
// memory rasterization with rounded vertex (NOT float barycentric per-pixel)
// gains parity vs Poppler.
func (sp *Splash) rasterizeTriangle(t *gouraudVert3, stats *gouraudDirectStats) {
	t.sortByY()
	x, y, color := t.x, t.y, t.c
	parameterized := len(t.functions) > 0 &&
		len(t.params[0]) > 0 && len(t.params[1]) > 0 && len(t.params[2]) > 0
	if stats != nil {
		stats.triangles++
		if parameterized {
			stats.parameterized++
		}
	}

	// Degenerate: det(T) == 0 (Splash.cc:5372).
	if (x[0]-x[2])*(y[1]-y[2])-(x[1]-x[2])*(y[0]-y[2]) == 0 {
		return
	}

	// scanEdgeL/R hold the active edge endpoints (Splash.cc:5380-5398).
	var scanEdgeL, scanEdgeR [2]int
	scanEdgeL[0] = 0
	scanEdgeR[0] = 0
	if y[0] == y[1] {
		scanEdgeL[0] = 1
		scanEdgeL[1] = 2
		scanEdgeR[1] = 2
	} else {
		scanEdgeL[1] = 1
		scanEdgeR[1] = 2
	}

	// Per-Y linear maps for x-extent (Splash.cc:5403-5406).
	var scanLimitMapL, scanLimitMapR [2]float64
	scanLimitMapL[0] = float64(x[scanEdgeL[1]]-x[scanEdgeL[0]]) / float64(y[scanEdgeL[1]]-y[scanEdgeL[0]])
	scanLimitMapL[1] = float64(x[scanEdgeL[0]]) - float64(y[scanEdgeL[0]])*scanLimitMapL[0]
	scanLimitMapR[0] = float64(x[scanEdgeR[1]]-x[scanEdgeR[0]]) / float64(y[scanEdgeR[1]]-y[scanEdgeR[0]])
	scanLimitMapR[1] = float64(x[scanEdgeR[0]]) - float64(y[scanEdgeR[0]])*scanLimitMapR[0]
	var scanParamMapL, scanParamMapR [2]float64
	if parameterized {
		scanParamMapL = gouraudEdgeParamMap(t, scanEdgeL)
		scanParamMapR = gouraudEdgeParamMap(t, scanEdgeR)
	}

	// Swap if "left" ended up to the right of "right" (Splash.cc:5408-5418).
	xa := float64(y[1])*scanLimitMapL[0] + scanLimitMapL[1]
	xt := float64(y[1])*scanLimitMapR[0] + scanLimitMapR[1]
	if xa > xt {
		scanEdgeL[0], scanEdgeR[0] = scanEdgeR[0], scanEdgeL[0]
		scanEdgeL[1], scanEdgeR[1] = scanEdgeR[1], scanEdgeL[1]
		scanLimitMapL[0], scanLimitMapR[0] = scanLimitMapR[0], scanLimitMapL[0]
		scanLimitMapL[1], scanLimitMapR[1] = scanLimitMapR[1], scanLimitMapL[1]
		scanParamMapL[0], scanParamMapR[0] = scanParamMapR[0], scanParamMapL[0]
		scanParamMapL[1], scanParamMapR[1] = scanParamMapR[1], scanParamMapL[1]
	}

	hasFurtherSegment := y[1] < y[2]

	for Y := y[0]; Y <= y[2]; Y++ {
		// SWEEP EVENT: cross y[1] — switch the segment that ended (Splash.cc:5433-5458).
		if hasFurtherSegment && Y == y[1] {
			if scanEdgeL[1] == 1 {
				scanEdgeL[0] = 1
				scanEdgeL[1] = 2
				scanLimitMapL[0] = float64(x[scanEdgeL[1]]-x[scanEdgeL[0]]) / float64(y[scanEdgeL[1]]-y[scanEdgeL[0]])
				scanLimitMapL[1] = float64(x[scanEdgeL[0]]) - float64(y[scanEdgeL[0]])*scanLimitMapL[0]
				if parameterized {
					scanParamMapL = gouraudEdgeParamMap(t, scanEdgeL)
				}
			} else if scanEdgeR[1] == 1 {
				scanEdgeR[0] = 1
				scanEdgeR[1] = 2
				scanLimitMapR[0] = float64(x[scanEdgeR[1]]-x[scanEdgeR[0]]) / float64(y[scanEdgeR[1]]-y[scanEdgeR[0]])
				scanLimitMapR[1] = float64(x[scanEdgeR[0]]) - float64(y[scanEdgeR[0]])*scanLimitMapR[0]
				if parameterized {
					scanParamMapR = gouraudEdgeParamMap(t, scanEdgeR)
				}
			}
			hasFurtherSegment = false
		}

		yt := float64(Y)
		xa = yt*scanLimitMapL[0] + scanLimitMapL[1]
		xt = yt*scanLimitMapR[0] + scanLimitMapR[1]
		scanLimitL := Round(xa)
		scanLimitR := Round(xt)
		paramL, paramR := 0.0, 0.0
		if parameterized {
			paramL = yt*scanParamMapL[0] + scanParamMapL[1]
			paramR = yt*scanParamMapR[0] + scanParamMapR[1]
		}

		if Y < 0 || Y >= sp.bitmap.height {
			continue
		}
		if scanLimitL > scanLimitR {
			continue
		}

		// Non-parameterized color still uses rounded-vertex barycentric weights.
		// Parameterized shadings follow Poppler's scalar scanline interpolation
		// from Splash.cc:5460-5496.
		fx0, fy0 := float64(x[0]), float64(y[0])
		fx1, fy1 := float64(x[1]), float64(y[1])
		fx2, fy2 := float64(x[2]), float64(y[2])

		for X := scanLimitL; X <= scanLimitR; X++ {
			if X < 0 || X >= sp.bitmap.width {
				continue
			}
			var px Color
			if parameterized {
				paramMap0 := 0.0
				if scanLimitR != scanLimitL {
					paramMap0 = (paramR - paramL) / float64(scanLimitR-scanLimitL)
				}
				param := paramMap0*float64(X) + (paramL - float64(scanLimitL)*paramMap0)
				out, err := evalShadingFunctions(t.functions, []float64{param})
				if err != nil || len(out) == 0 {
					continue
				}
				px = packShadingOutput(out, t.colorSpace, t.mode)
				sp.writeBitmapPixel(X, Y, px, stats)
				continue
			}
			w0, w1, w2, ok := barycentric(float64(X), yt, fx0, fy0, fx1, fy1, fx2, fy2)
			if !ok {
				continue
			}
			// Clamp weights to [0,1] (numerical safety on edges).
			if w0 < 0 {
				w0 = 0
			} else if w0 > 1 {
				w0 = 1
			}
			if w1 < 0 {
				w1 = 0
			} else if w1 > 1 {
				w1 = 1
			}
			if w2 < 0 {
				w2 = 0
			} else if w2 > 1 {
				w2 = 1
			}
			blendColor(color[0], color[1], color[2], w0, w1, w2, &px)
			sp.writeBitmapPixel(X, Y, px, stats)
		}
	}
}

func gouraudEdgeParamMap(t *gouraudVert3, edge [2]int) [2]float64 {
	y0 := t.y[edge[0]]
	y1 := t.y[edge[1]]
	p0 := t.params[edge[0]][0]
	p1 := t.params[edge[1]][0]
	slope := (p1 - p0) / float64(y1-y0)
	return [2]float64{slope, p0 - float64(y0)*slope}
}

func gouraudParameterizedValueAt(t *gouraudVert3, px, py int) (float64, bool) {
	t.sortByY()
	x, y := t.x, t.y
	if py < y[0] || py > y[2] {
		return 0, false
	}
	if (x[0]-x[2])*(y[1]-y[2])-(x[1]-x[2])*(y[0]-y[2]) == 0 {
		return 0, false
	}

	scanEdgeL := [2]int{0, 0}
	scanEdgeR := [2]int{0, 0}
	if y[0] == y[1] {
		scanEdgeL[0] = 1
		scanEdgeL[1] = 2
		scanEdgeR[1] = 2
	} else {
		scanEdgeL[1] = 1
		scanEdgeR[1] = 2
	}
	scanLimitMapL := gouraudEdgeLimitMap(t, scanEdgeL)
	scanLimitMapR := gouraudEdgeLimitMap(t, scanEdgeR)
	scanParamMapL := gouraudEdgeParamMap(t, scanEdgeL)
	scanParamMapR := gouraudEdgeParamMap(t, scanEdgeR)

	xa := float64(y[1])*scanLimitMapL[0] + scanLimitMapL[1]
	xt := float64(y[1])*scanLimitMapR[0] + scanLimitMapR[1]
	if xa > xt {
		scanEdgeL, scanEdgeR = scanEdgeR, scanEdgeL
		scanLimitMapL, scanLimitMapR = scanLimitMapR, scanLimitMapL
		scanParamMapL, scanParamMapR = scanParamMapR, scanParamMapL
	}

	if y[1] < y[2] && py >= y[1] {
		if scanEdgeL[1] == 1 {
			scanEdgeL = [2]int{1, 2}
			scanLimitMapL = gouraudEdgeLimitMap(t, scanEdgeL)
			scanParamMapL = gouraudEdgeParamMap(t, scanEdgeL)
		} else if scanEdgeR[1] == 1 {
			scanEdgeR = [2]int{1, 2}
			scanLimitMapR = gouraudEdgeLimitMap(t, scanEdgeR)
			scanParamMapR = gouraudEdgeParamMap(t, scanEdgeR)
		}
	}

	yt := float64(py)
	scanLimitL := Round(yt*scanLimitMapL[0] + scanLimitMapL[1])
	scanLimitR := Round(yt*scanLimitMapR[0] + scanLimitMapR[1])
	if px < scanLimitL || px > scanLimitR || scanLimitL > scanLimitR {
		return 0, false
	}

	paramL := yt*scanParamMapL[0] + scanParamMapL[1]
	paramR := yt*scanParamMapR[0] + scanParamMapR[1]
	paramMap0 := 0.0
	if scanLimitR != scanLimitL {
		paramMap0 = (paramR - paramL) / float64(scanLimitR-scanLimitL)
	}
	return paramMap0*float64(px) + (paramL - float64(scanLimitL)*paramMap0), true
}

func gouraudEdgeLimitMap(t *gouraudVert3, edge [2]int) [2]float64 {
	y0 := t.y[edge[0]]
	y1 := t.y[edge[1]]
	x0 := t.x[edge[0]]
	x1 := t.x[edge[1]]
	slope := float64(x1-x0) / float64(y1-y0)
	return [2]float64{slope, float64(x0) - float64(y0)*slope}
}

// writeBitmapPixel writes pixel `c` directly into the bitmap, mirroring the
// direct-blit path at Splash.cc:5483-5503 used when noTransparency holds.
// Channel count is selected from the bitmap mode.
func (sp *Splash) writeBitmapPixel(x, y int, c Color, stats *gouraudDirectStats) {
	bm := sp.bitmap
	if bm == nil || bm.data == nil {
		return
	}
	if x < 0 || x >= bm.width || y < 0 || y >= bm.height {
		return
	}
	bpp := bytesPerPixel(bm.mode)
	if bpp <= 0 {
		return
	}
	off := y*bm.rowSize + x*bpp
	if off < 0 || off+bpp > len(bm.data) {
		return
	}
	if clip, ok := sp.state.clip.(*xpath.Clip); ok && clip != nil {
		if clip.HasPathClip() && stats != nil {
			stats.clipPathWrites++
		}
		if !clip.Test(x, y) {
			if stats != nil {
				stats.clipRejectedByBinary++
			}
			return
		}
	} else if stats != nil {
		stats.clipMissing++
	}
	if stats != nil {
		stats.written++
	}
	for k := 0; k < bpp; k++ {
		bm.data[off+k] = c[k]
	}
	if bm.alpha != nil {
		ai := y*bm.width + x
		if ai >= 0 && ai < len(bm.alpha) {
			bm.alpha[ai] = 0xFF
		}
	}
}

// FillGouraudTriangleShadedFill drives a Gouraud mesh fill — called by the
// PDF "sh" op for ShadingType=4/5. Mirrors Splash::gouraudTriangleShadedFill
// at Splash.cc:5255.
//
// Path argument `p` mirrors the optional bbox path used by `sh`; for the
// direct-blit branch we ignore it (the mesh defines its own coverage).
func (sp *Splash) FillGouraudTriangleShadedFill(shader *GouraudShader, p *xpath.Path, eo bool) error {
	if shader == nil {
		return ErrBadArg
	}
	if sp == nil || sp.bitmap == nil {
		return ErrBadArg
	}
	stats := newGouraudDirectStats(sp, p, eo)
	defer stats.print()
	for i := 0; i < shader.nTriangles(); i++ {
		v0, v1, v2 := shader.triangleVerts(i)
		// Map device coords to integer raster vertices (Splash.cc:5326-5333).
		// state.matrix is identity by default; we honor it by transforming.
		m := sp.state.matrix
		t := &gouraudVert3{}
		coords := [3][2]float64{{v0.X, v0.Y}, {v1.X, v1.Y}, {v2.X, v2.Y}}
		colors := [3]Color{v0.Color, v1.Color, v2.Color}
		params := [3][]float64{v0.Params, v1.Params, v2.Params}
		for k := 0; k < 3; k++ {
			xt := coords[k][0]*m[0] + coords[k][1]*m[2] + m[4]
			yt := coords[k][0]*m[1] + coords[k][1]*m[3] + m[5]
			t.x[k] = Round(xt)
			t.y[k] = Round(yt)
			t.c[k] = colors[k]
			t.params[k] = params[k]
		}
		t.functions = shader.Functions
		t.colorSpace = shader.ColorSpace
		t.mode = shader.Mode
		sp.rasterizeTriangle(t, stats)
	}
	return nil
}
