package xpath

import (
	"math"
	"sort"
)

// MaxCurveSplits is the curve-flattening recursion cap (SplashXPath.h:32).
const MaxCurveSplits = 1 << 10

// aaSize mirrors splashAASize (SplashTypes.h:46) — sub-pixel scale for aaScale.
const aaSize = 4

// XPathSeg flag bits mirror SplashXPath.h:47-53.
const (
	XPathHoriz uint32 = 0x01 // y0 == y1 (SplashXPath.h:47)
	XPathVert  uint32 = 0x02 // x0 == x1 (SplashXPath.h:50)
	XPathFlip  uint32 = 0x04 // y0 > y1  (SplashXPath.h:53)
)

// XPathSeg mirrors SplashXPathSeg (SplashXPath.h:38-45).
type XPathSeg struct {
	X0, Y0 float64
	X1, Y1 float64
	DXDY   float64
	DYDX   float64
	Flags  uint32
}

// XPath is the device-space, flattened segment list (SplashXPath.h:59-93).
type XPath struct {
	Segs []XPathSeg
}

// xpathAdjust mirrors SplashXPathAdjust (SplashXPath.cc:42-49).
type xpathAdjust struct {
	firstPt, lastPt   int
	vert              bool
	x0a, x0b          float64
	xma, xmb          float64
	x1a, x1b          float64
	x0, x1, xm        float64
}

// splashFloor copies SplashMath.h:80-86 (portable branch) — truncates toward -inf.
func splashFloor(x float64) int {
	if x > 0 {
		return int(x)
	}
	return int(math.Floor(x))
}

// splashRound copies SplashMath.h:175 — floor(x+0.5), NOT banker's rounding.
func splashRound(x float64) int {
	return splashFloor(x + 0.5)
}

// transformPt is SplashXPath::transform (SplashXPath.cc:54-61).
func transformPt(m [6]float64, xi, yi float64) (xo, yo float64) {
	xo = xi*m[0] + yi*m[2] + m[4]
	yo = xi*m[1] + yi*m[3] + m[5]
	return
}

// NewXPath flattens path under matrix at the given flatness (SplashXPath::SplashXPath, SplashXPath.cc:67).
func NewXPath(p *Path, matrix [6]float64, flatness float64, closeSubpaths bool) *XPath {
	x := &XPath{}
	x.Reset(p, matrix, flatness, closeSubpaths)
	return x
}

// Reset re-initialises x in place from path/matrix (used with the xpathPool).
func (x *XPath) Reset(p *Path, matrix [6]float64, flatness float64, closeSubpaths bool) {
	x.Segs = x.Segs[:0]
	if p == nil {
		return
	}
	n := len(p.pts)
	if n == 0 {
		return
	}

	// transform every point through matrix (SplashXPath.cc:77-80).
	pts := make([]PathPoint, n)
	for i := 0; i < n; i++ {
		px, py := transformPt(matrix, p.pts[i].X, p.pts[i].Y)
		pts[i] = PathPoint{X: px, Y: py}
	}

	// build adjust descriptors from hints (SplashXPath.cc:83-149).
	var adjusts []xpathAdjust
	if len(p.hints) > 0 {
		adjusts = make([]xpathAdjust, 0, len(p.hints))
		ok := true
		for i := 0; i < len(p.hints); i++ {
			h := &p.hints[i]
			if h.Ctrl0+1 >= n || h.Ctrl1+1 >= n {
				adjusts = nil
				ok = false
				break
			}
			x0 := pts[h.Ctrl0].X
			y0 := pts[h.Ctrl0].Y
			x1 := pts[h.Ctrl0+1].X
			y1 := pts[h.Ctrl0+1].Y
			x2 := pts[h.Ctrl1].X
			y2 := pts[h.Ctrl1].Y
			x3 := pts[h.Ctrl1+1].X
			y3 := pts[h.Ctrl1+1].Y
			var adj xpathAdjust
			var adj0, adj1 float64
			if x0 == x1 && x2 == x3 {
				adj.vert = true
				adj0, adj1 = x0, x2
			} else if y0 == y1 && y2 == y3 {
				adj.vert = false
				adj0, adj1 = y0, y2
			} else {
				adjusts = nil
				ok = false
				break
			}
			if adj0 > adj1 {
				adj0, adj1 = adj1, adj0
			}
			adj.x0a = adj0 - 0.01
			adj.x0b = adj0 + 0.01
			adj.xma = 0.5*(adj0+adj1) - 0.01
			adj.xmb = 0.5*(adj0+adj1) + 0.01
			adj.x1a = adj1 - 0.01
			adj.x1b = adj1 + 0.01
			// rounding both edges so adjacent strokes line up (SplashXPath.cc:125-129).
			rx0 := splashRound(adj0)
			rx1 := splashRound(adj1)
			if rx1 == rx0 {
				// adjustLines branch is gated by callers we do not yet expose; default to
				// "x1 = x1 + 1" (SplashXPath.cc:140) — minimum 1px slab width.
				rx1 = rx1 + 1
			}
			adj.x0 = float64(rx0)
			adj.x1 = float64(rx1) - 0.01
			adj.xm = 0.5 * (adj.x0 + adj.x1)
			adj.firstPt = h.FirstPt
			adj.lastPt = h.LastPt
			adjusts = append(adjusts, adj)
		}
		_ = ok
	}

	// apply stroke adjustment to point cloud (SplashXPath.cc:156-163).
	if len(adjusts) > 0 {
		for i := range adjusts {
			a := &adjusts[i]
			for j := a.firstPt; j <= a.lastPt && j < n; j++ {
				strokeAdjustPt(a, &pts[j])
			}
		}
	}

	// walk path emitting line/curve segments (SplashXPath.cc:172-214).
	var x0, y0, xsp, ysp float64
	i := 0
	for i < n {
		if p.flags[i]&pathFirst != 0 {
			x0 = pts[i].X
			y0 = pts[i].Y
			xsp = x0
			ysp = y0
			i++
			continue
		}
		if p.flags[i]&pathCurve != 0 {
			x1 := pts[i].X
			y1 := pts[i].Y
			x2 := pts[i+1].X
			y2 := pts[i+1].Y
			x3 := pts[i+2].X
			y3 := pts[i+2].Y
			x.addCurve(x0, y0, x1, y1, x2, y2, x3, y3, flatness)
			x0 = x3
			y0 = y3
			i += 3
		} else {
			x1 := pts[i].X
			y1 := pts[i].Y
			x.addSegment(x0, y0, x1, y1)
			x0 = x1
			y0 = y1
			i++
		}
		// close subpath if requested (SplashXPath.cc:210-212).
		if closeSubpaths && i > 0 && (p.flags[i-1]&pathLast != 0) &&
			(pts[i-1].X != pts[lastSubpathStart(p, i-1)].X ||
				pts[i-1].Y != pts[lastSubpathStart(p, i-1)].Y) {
			// xsp/ysp track current subpath's first point.
			x.addSegment(x0, y0, xsp, ysp)
		}
	}
}

// lastSubpathStart returns the index of the most recent pathFirst at or before idx.
func lastSubpathStart(p *Path, idx int) int {
	for k := idx; k >= 0; k-- {
		if p.flags[k]&pathFirst != 0 {
			return k
		}
	}
	return 0
}

// strokeAdjustPt mirrors SplashXPath::strokeAdjust (SplashXPath.cc:220-243).
func strokeAdjustPt(a *xpathAdjust, pt *PathPoint) {
	if a.vert {
		x := pt.X
		switch {
		case x > a.x0a && x < a.x0b:
			pt.X = a.x0
		case x > a.xma && x < a.xmb:
			pt.X = a.xm
		case x > a.x1a && x < a.x1b:
			pt.X = a.x1
		}
	} else {
		y := pt.Y
		switch {
		case y > a.x0a && y < a.x0b:
			pt.Y = a.x0
		case y > a.xma && y < a.xmb:
			pt.Y = a.xm
		case y > a.x1a && y < a.x1b:
			pt.Y = a.x1
		}
	}
}

// addSegment appends a flattened line segment (SplashXPath.cc:373-401).
func (x *XPath) addSegment(x0, y0, x1, y1 float64) {
	seg := XPathSeg{X0: x0, Y0: y0, X1: x1, Y1: y1}
	switch {
	case y1 == y0:
		// horizontal — dxdy/dydx undefined, store 0 (SplashXPath.cc:384-389).
		seg.Flags |= XPathHoriz
		if x1 == x0 {
			seg.Flags |= XPathVert
		}
	case x1 == x0:
		seg.Flags |= XPathVert
	default:
		seg.DXDY = (x1 - x0) / (y1 - y0)
		seg.DYDX = 1.0 / seg.DXDY
	}
	if y0 > y1 {
		seg.Flags |= XPathFlip
	}
	x.Segs = append(x.Segs, seg)
}

// addCurve flattens a cubic via midpoint subdivision (SplashXPath.cc:268-371).
//
// Subdivision criterion: squared distance of each interior control point to the
// CHORD MIDPOINT (SplashXPath.cc:312-315 — "a bit of a hack, but much faster").
// flatness2 = flatness*flatness, NOT (flatness*0.5)^2.
func (x *XPath) addCurve(x0, y0, x1, y1, x2, y2, x3, y3, flatness float64) {
	const sz = MaxCurveSplits + 1
	cx := make([]float64, sz*3)
	cy := make([]float64, sz*3)
	cNext := make([]int, sz)
	flatness2 := flatness * flatness

	p1 := 0
	p2 := MaxCurveSplits

	cx[p1*3+0] = x0
	cx[p1*3+1] = x1
	cx[p1*3+2] = x2
	cx[p2*3+0] = x3
	cy[p1*3+0] = y0
	cy[p1*3+1] = y1
	cy[p1*3+2] = y2
	cy[p2*3+0] = y3
	cNext[p1] = p2

	for p1 < MaxCurveSplits {
		xl0 := cx[p1*3+0]
		xx1 := cx[p1*3+1]
		xx2 := cx[p1*3+2]
		yl0 := cy[p1*3+0]
		yy1 := cy[p1*3+1]
		yy2 := cy[p1*3+2]
		p2 = cNext[p1]
		xr3 := cx[p2*3+0]
		yr3 := cy[p2*3+0]

		mx := (xl0 + xr3) * 0.5
		my := (yl0 + yr3) * 0.5
		dx := xx1 - mx
		dy := yy1 - my
		d1 := dx*dx + dy*dy
		dx = xx2 - mx
		dy = yy2 - my
		d2 := dx*dx + dy*dy

		// flat enough OR depth cap reached → emit chord (SplashXPath.cc:327-329).
		if p2-p1 == 1 || (d1 <= flatness2 && d2 <= flatness2) {
			x.addSegment(xl0, yl0, xr3, yr3)
			p1 = p2
			continue
		}

		// de Casteljau split at t=0.5 (SplashXPath.cc:333-364).
		xl1 := (xl0 + xx1) * 0.5
		yl1 := (yl0 + yy1) * 0.5
		xh := (xx1 + xx2) * 0.5
		yh := (yy1 + yy2) * 0.5
		xl2 := (xl1 + xh) * 0.5
		yl2 := (yl1 + yh) * 0.5
		xr2 := (xx2 + xr3) * 0.5
		yr2 := (yy2 + yr3) * 0.5
		xr1 := (xh + xr2) * 0.5
		yr1 := (yh + yr2) * 0.5
		xr0 := (xl2 + xr1) * 0.5
		yr0 := (yl2 + yr1) * 0.5
		p3 := (p1 + p2) / 2

		cx[p1*3+1] = xl1
		cx[p1*3+2] = xl2
		cy[p1*3+1] = yl1
		cy[p1*3+2] = yl2
		cNext[p1] = p3
		cx[p3*3+0] = xr0
		cx[p3*3+1] = xr1
		cx[p3*3+2] = xr2
		cy[p3*3+0] = yr0
		cy[p3*3+1] = yr1
		cy[p3*3+2] = yr2
		cNext[p3] = p2
	}
}

// StrokeAdjust applies stroke-adjustment hints to the segment list
// (SplashXPath::strokeAdjust hints, SplashXPath.cc:119-145).
//
// Note: the C++ ctor folds hint application into the initial point transform
// (it adjusts pts[] before walking). This method is provided for callers that
// build an XPath from already-flattened segments and want to retro-snap the
// endpoints. It computes per-hint adjusts from the existing seg endpoints and
// applies the same three-zone snap (x0/xm/x1) per SplashXPath.cc:119-145.
func (x *XPath) StrokeAdjust(hints []PathHint) {
	if len(hints) == 0 || len(x.Segs) == 0 {
		return
	}
	for i := range hints {
		h := &hints[i]
		if h.Ctrl0 >= len(x.Segs) || h.Ctrl1 >= len(x.Segs) {
			continue
		}
		s0 := &x.Segs[h.Ctrl0]
		s1 := &x.Segs[h.Ctrl1]
		var adj xpathAdjust
		var adj0, adj1 float64
		if s0.X0 == s0.X1 && s1.X0 == s1.X1 {
			adj.vert = true
			adj0, adj1 = s0.X0, s1.X0
		} else if s0.Y0 == s0.Y1 && s1.Y0 == s1.Y1 {
			adj.vert = false
			adj0, adj1 = s0.Y0, s1.Y0
		} else {
			continue
		}
		if adj0 > adj1 {
			adj0, adj1 = adj1, adj0
		}
		adj.x0a = adj0 - 0.01
		adj.x0b = adj0 + 0.01
		adj.xma = 0.5*(adj0+adj1) - 0.01
		adj.xmb = 0.5*(adj0+adj1) + 0.01
		adj.x1a = adj1 - 0.01
		adj.x1b = adj1 + 0.01
		rx0 := splashRound(adj0)
		rx1 := splashRound(adj1)
		if rx1 == rx0 {
			rx1 = rx1 + 1 // +1 ensures min 1px slab (SplashXPath.cc:140).
		}
		adj.x0 = float64(rx0)
		adj.x1 = float64(rx1) - 0.01 // -0.01 keeps splashFloor(x1)==x0 (SplashXPath.cc:144).
		adj.xm = 0.5 * (adj.x0 + adj.x1)

		// snap every endpoint in [firstPt..lastPt] across all touched segs.
		for k := h.FirstPt; k <= h.LastPt && k < len(x.Segs); k++ {
			seg := &x.Segs[k]
			pt0 := PathPoint{X: seg.X0, Y: seg.Y0}
			pt1 := PathPoint{X: seg.X1, Y: seg.Y1}
			strokeAdjustPt(&adj, &pt0)
			strokeAdjustPt(&adj, &pt1)
			seg.X0, seg.Y0 = pt0.X, pt0.Y
			seg.X1, seg.Y1 = pt1.X, pt1.Y
		}
	}
}

// AAScale multiplies all coordinates by splashAASize for AA rendering
// (SplashXPath::aaScale, SplashXPath.cc:427-438). dxdy/dydx are NOT recomputed —
// slopes are invariant under uniform scaling.
func (x *XPath) AAScale() {
	for i := range x.Segs {
		x.Segs[i].X0 *= aaSize
		x.Segs[i].Y0 *= aaSize
		x.Segs[i].X1 *= aaSize
		x.Segs[i].Y1 *= aaSize
	}
}

// Sort sorts segments by upper coordinate in y-major order
// (SplashXPath::sort, SplashXPath.cc:440-443; comparator at :403-425).
func (x *XPath) Sort() {
	sort.Slice(x.Segs, func(i, j int) bool {
		return cmpXPathSegs(&x.Segs[i], &x.Segs[j])
	})
}

// cmpXPathSegs is the y-major then x-tiebreaker comparator (SplashXPath.cc:403-425).
func cmpXPathSegs(a, b *XPathSeg) bool {
	var x0, y0, x1, y1 float64
	if a.Flags&XPathFlip != 0 {
		x0, y0 = a.X1, a.Y1
	} else {
		x0, y0 = a.X0, a.Y0
	}
	if b.Flags&XPathFlip != 0 {
		x1, y1 = b.X1, b.Y1
	} else {
		x1, y1 = b.X0, b.Y0
	}
	if y0 != y1 {
		return y0 < y1
	}
	return x0 < x1
}

// Length returns the number of segments (mirrors SplashXPath::length, SplashXPath.h:88).
func (x *XPath) Length() int {
	return len(x.Segs)
}
