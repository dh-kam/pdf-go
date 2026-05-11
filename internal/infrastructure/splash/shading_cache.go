package splash

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type splashUnivariateShadingColorCache struct {
	bounds []float64
	coeff  float64
	values [][]float64
	last   int
}

// newSplashUnivariateShadingColorCache mirrors Poppler's
// GfxUnivariateShading::setupCache: restrict the cache to the parameter range
// touched by the current user-space clip bbox, sample the shading functions at
// a CTM-scaled number of stops, then linearly interpolate per-pixel colors.
func newSplashUnivariateShadingColorCache(shading *entity.Shading, ctm [6]float64, bbox [4]float64) *splashUnivariateShadingColorCache {
	if shading == nil {
		return nil
	}
	functions := shading.GetFunctions()
	if len(functions) == 0 {
		return nil
	}
	sMin, sMax, ok := splashUnivariateParameterRange(shading, bbox)
	if !ok || sMin == sMax {
		return nil
	}
	distance := splashUnivariateDistance(shading, sMin, sMax)
	if distance <= 0 {
		return nil
	}
	domain0, domain1 := splashUnivariateDomain(shading)
	var tMin, tMax float64
	if domain0 < domain1 {
		tMin = domain0 + sMin*(domain1-domain0)
		tMax = domain0 + sMax*(domain1-domain0)
	} else {
		tMin = domain0 + sMax*(domain1-domain0)
		tMax = domain0 + sMin*(domain1-domain0)
	}
	if tMin == tMax {
		return nil
	}

	xMin, yMin, xMax, yMax := transformedBBoxBounds(bbox, ctm)
	area := math.Abs((xMax - xMin) * (yMax - yMin))
	if area <= 0 {
		return nil
	}

	maxSize := int(math.Ceil(splashMatrixNorm(ctm) * distance))
	if maxSize < 2 {
		maxSize = 2
	}
	if float64(maxSize) > area {
		return nil
	}

	cache := &splashUnivariateShadingColorCache{
		bounds: make([]float64, maxSize),
		coeff:  float64(maxSize-1) / (tMax - tMin),
		values: make([][]float64, maxSize),
		last:   1,
	}
	step := (tMax - tMin) / float64(maxSize-1)
	for i := 0; i < maxSize; i++ {
		t := tMin + float64(i)*step
		values, err := evalShadingFunctions(functions, []float64{t})
		if err != nil || len(values) == 0 {
			return nil
		}
		cache.bounds[i] = t
		cache.values[i] = values
	}
	if os.Getenv("PDF_DEBUG_SPLASH_CACHE_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "SPLASH_CACHE_TRACE type=%d bbox=%v s=(%.12f,%.12f) t=(%.12f,%.12f) distance=%.12f norm=%.12f area=%.12f size=%d\n",
			shading.GetShadingType(), bbox, sMin, sMax, tMin, tMax, distance, splashMatrixNorm(ctm), area, maxSize)
	}
	return cache
}

func (c *splashUnivariateShadingColorCache) Evaluate(t float64) ([]float64, bool) {
	if c == nil || len(c.bounds) < 2 || len(c.values) != len(c.bounds) {
		return nil, false
	}
	last := c.last
	if last <= 0 || last >= len(c.bounds) {
		last = 1
	}
	if c.bounds[last-1] >= t {
		last = sort.SearchFloat64s(c.bounds[:last], t)
	} else if c.bounds[last] < t {
		last = last + 1 + sort.SearchFloat64s(c.bounds[last+1:], t)
	}
	if last < 1 {
		last = 1
	}
	if last >= len(c.bounds) {
		last = len(c.bounds) - 1
	}
	c.last = last

	upper := c.values[last]
	lower := c.values[last-1]
	if len(upper) != len(lower) {
		return nil, false
	}
	x := (t - c.bounds[last-1]) * c.coeff
	ix := 1.0 - x
	out := make([]float64, len(upper))
	for i := range out {
		out[i] = ix*lower[i] + x*upper[i]
	}
	if shouldTraceSplashCacheT(t) {
		fmt.Fprintf(os.Stderr, "SPLASH_CACHE_EVAL_TRACE t=%.12f last=%d lowerBound=%.12f upperBound=%.12f x=%.12f lower=%v upper=%v out=%v\n",
			t, last, c.bounds[last-1], c.bounds[last], x, lower, upper, out)
	}
	return out, true
}

func shouldTraceSplashCacheT(t float64) bool {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_CACHE_T_TRACE"))
	if raw == "" {
		return false
	}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		target, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err == nil && math.Abs(t-target) < 1e-9 {
			return true
		}
	}
	return false
}

func splashUnivariateDomain(shading *entity.Shading) (float64, float64) {
	domain := shading.GetDomain()
	if domain[0] != 0 || domain[1] != 0 {
		return domain[0], domain[1]
	}
	return 0, 1
}

func splashUnivariateDistance(shading *entity.Shading, sMin, sMax float64) float64 {
	coords := shading.GetCoords()
	switch shading.GetShadingType() {
	case entity.ShadingAxial:
		if len(coords) < 4 {
			return 0
		}
		xMin := coords[0] + sMin*(coords[2]-coords[0])
		yMin := coords[1] + sMin*(coords[3]-coords[1])
		xMax := coords[0] + sMax*(coords[2]-coords[0])
		yMax := coords[1] + sMax*(coords[3]-coords[1])
		return math.Hypot(xMax-xMin, yMax-yMin)
	case entity.ShadingRadial:
		if len(coords) < 6 {
			return 0
		}
		xMin := coords[0] + sMin*(coords[3]-coords[0])
		yMin := coords[1] + sMin*(coords[4]-coords[1])
		rMin := coords[2] + sMin*(coords[5]-coords[2])
		xMax := coords[0] + sMax*(coords[3]-coords[0])
		yMax := coords[1] + sMax*(coords[4]-coords[1])
		rMax := coords[2] + sMax*(coords[5]-coords[2])
		return math.Hypot(xMax-xMin, yMax-yMin) + math.Abs(rMax-rMin)
	default:
		return 0
	}
}

func splashUnivariateParameterRange(shading *entity.Shading, bbox [4]float64) (float64, float64, bool) {
	xMin, xMax := orderedPair(bbox[0], bbox[2])
	yMin, yMax := orderedPair(bbox[1], bbox[3])
	switch shading.GetShadingType() {
	case entity.ShadingAxial:
		return splashAxialParameterRange(shading.GetCoords(), xMin, yMin, xMax, yMax)
	case entity.ShadingRadial:
		return splashRadialParameterRange(shading.GetCoords(), xMin, yMin, xMax, yMax)
	default:
		return 0, 0, false
	}
}

func orderedPair(a, b float64) (float64, float64) {
	if a <= b {
		return a, b
	}
	return b, a
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func splashAxialParameterRange(coords []float64, xMin, yMin, xMax, yMax float64) (float64, float64, bool) {
	if len(coords) < 4 {
		return 0, 0, false
	}
	pdx := coords[2] - coords[0]
	pdy := coords[3] - coords[1]
	den := pdx*pdx + pdy*pdy
	if den == 0 {
		return 0, 0, true
	}
	pdx /= den
	pdy /= den

	t := (xMin-coords[0])*pdx + (yMin-coords[1])*pdy
	tdx := (xMax - xMin) * pdx
	tdy := (yMax - yMin) * pdy
	lower, upper := t, t
	if tdx < 0 {
		lower += tdx
	} else {
		upper += tdx
	}
	if tdy < 0 {
		lower += tdy
	} else {
		upper += tdy
	}
	return clampUnit(lower), clampUnit(upper), true
}

func splashRadialParameterRange(coords []float64, xMin, yMin, xMax, yMax float64) (float64, float64, bool) {
	if len(coords) < 6 {
		return 0, 0, false
	}
	cx, cy, cr := coords[0], coords[1], coords[2]
	dx := coords[3] - cx
	dy := coords[4] - cy
	dr := coords[5] - cr

	if xMin >= xMax || yMin >= yMax ||
		(math.Abs(coords[2]-coords[5]) < radialDegenerateEpsilon &&
			(math.Min(coords[2], coords[5]) < radialDegenerateEpsilon ||
				math.Max(math.Abs(coords[0]-coords[3]), math.Abs(coords[1]-coords[4])) < 2*radialDegenerateEpsilon)) {
		return 0, 0, true
	}

	xMin -= cx
	yMin -= cy
	xMax -= cx
	yMax -= cy

	xMin -= radialDegenerateEpsilon
	yMin -= radialDegenerateEpsilon
	xMax += radialDegenerateEpsilon
	yMax += radialDegenerateEpsilon

	minX := xMin - radialDegenerateEpsilon
	minY := yMin - radialDegenerateEpsilon
	maxX := xMax + radialDegenerateEpsilon
	maxY := yMax + radialDegenerateEpsilon
	minDR := -(cr + radialDegenerateEpsilon)

	lower, upper := 0.0, 0.0
	valid := false
	extendRange := func(value float64) {
		if !valid {
			lower, upper = value, value
			valid = true
			return
		}
		if value < lower {
			lower = value
		} else if value > upper {
			upper = value
		}
	}

	if math.Abs(dr) >= radialDegenerateEpsilon {
		tFocus := -cr / dr
		xFocus := tFocus * dx
		yFocus := tFocus * dy
		if minX <= xFocus && xFocus <= maxX && minY <= yFocus && yFocus <= maxY {
			extendRange(tFocus)
		}
	}

	radialEdge := func(num, den, delta, edgeLower, edgeUpper float64) {
		if math.Abs(den) < radialDegenerateEpsilon {
			return
		}
		tEdge := num / den
		v := tEdge * delta
		if tEdge*dr >= minDR && edgeLower <= v && v <= edgeUpper {
			extendRange(tEdge)
		}
	}

	radialCorner1 := func(x, y float64) {
		b := x*dx + y*dy + cr*dr
		if math.Abs(b) < radialDegenerateEpsilon {
			return
		}
		c := x*x + y*y - cr*cr
		tCorner := 0.5 * c / b
		if tCorner*dr >= minDR {
			extendRange(tCorner)
		}
	}

	radialCorner2 := func(x, y, a, invA float64) {
		b := x*dx + y*dy + cr*dr
		c := x*x + y*y - cr*cr
		d := b*b - a*c
		if d < 0 {
			return
		}
		sqrtD := math.Sqrt(d)
		tCorner := (b + sqrtD) * invA
		if tCorner*dr >= minDR {
			extendRange(tCorner)
		}
		tCorner = (b - sqrtD) * invA
		if tCorner*dr >= minDR {
			extendRange(tCorner)
		}
	}

	radialEdge(xMin-cr, dx+dr, dy, minY, maxY)
	radialEdge(xMax+cr, dx-dr, dy, minY, maxY)
	radialEdge(yMin-cr, dy+dr, dx, minX, maxX)
	radialEdge(yMax+cr, dy-dr, dx, minX, maxX)

	a := dx*dx + dy*dy - dr*dr
	if math.Abs(a) < radialDegenerateEpsilon*radialDegenerateEpsilon {
		if dr < 0 {
			extendRange(0)
		} else {
			extendRange(1)
		}
		radialCorner1(xMin, yMin)
		radialCorner1(xMin, yMax)
		radialCorner1(xMax, yMin)
		radialCorner1(xMax, yMax)
	} else {
		invA := 1 / a
		radialCorner2(xMin, yMin, a, invA)
		radialCorner2(xMin, yMax, a, invA)
		radialCorner2(xMax, yMin, a, invA)
		radialCorner2(xMax, yMax, a, invA)
	}

	return clampUnit(lower), clampUnit(upper), true
}

func splashMatrixNorm(m [6]float64) float64 {
	i := m[0]*m[0] + m[1]*m[1]
	j := m[2]*m[2] + m[3]*m[3]
	f := 0.5 * (i + j)
	g := 0.5 * (i - j)
	h := m[0]*m[2] + m[1]*m[3]
	return math.Sqrt(f + math.Hypot(g, h))
}
