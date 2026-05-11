package xpath

import "errors"

// Path flag constants mirror SplashPath.h:39-52.
const (
	pathFirst  byte = 0x01 // first point on each subpath (SplashPath.h:42)
	pathLast   byte = 0x02 // last point on each subpath (SplashPath.h:45)
	pathClosed byte = 0x04 // closed subpath: first==last (SplashPath.h:49)
	pathCurve  byte = 0x08 // curve control point (SplashPath.h:52)
)

// errNoCurPt mirrors splashErrNoCurPt (SplashErrorCodes.h, returned at SplashPath.cc:140,158,182).
var errNoCurPt = errors.New("xpath: no current point")

// errBogusPath mirrors splashErrBogusPath (SplashErrorCodes.h, returned at SplashPath.cc:124,128,145,162).
var errBogusPath = errors.New("xpath: bogus path (1-pt subpath)")

// PathPoint mirrors SplashPathPoint (SplashPath.h:32-35).
type PathPoint struct {
	X, Y float64
}

// PathHint mirrors SplashPathHint (SplashPath.h:58-62) plus computed/projecting flags used by stroke adjust.
type PathHint struct {
	Ctrl0, Ctrl1    int
	FirstPt, LastPt int
	ProjectingCap   bool
	Computed        bool
}

// Path is the user-space path type, equivalent to SplashPath (SplashPath.h:68-136).
//
// The Go port replaces C++'s manual grow()/realloc bookkeeping (SplashPath.cc:81-102)
// with native append; semantics are equivalent. curSubpath is tracked to mirror
// the three-state machine documented at SplashPath.cc:35-44.
type Path struct {
	pts        []PathPoint
	flags      []byte
	lengths    []int // currently unused; reserved for parity with future SplashPath fields
	hints      []PathHint
	curSubpath int // index of first point in last subpath (SplashPath.h:129)
}

// NewPath returns an empty Path (SplashPath::SplashPath, SplashPath.cc:46-54).
func NewPath() *Path {
	return &Path{}
}

// Clone returns a deep copy of p (SplashPath move-ctor analogue, SplashPath.cc:56-72).
func (p *Path) Clone() *Path {
	c := &Path{curSubpath: p.curSubpath}
	if p.pts != nil {
		c.pts = make([]PathPoint, len(p.pts))
		copy(c.pts, p.pts)
	}
	if p.flags != nil {
		c.flags = make([]byte, len(p.flags))
		copy(c.flags, p.flags)
	}
	if p.lengths != nil {
		c.lengths = make([]int, len(p.lengths))
		copy(c.lengths, p.lengths)
	}
	if p.hints != nil {
		c.hints = make([]PathHint, len(p.hints))
		copy(c.hints, p.hints)
	}
	return c
}

// noCurrentPoint mirrors SplashPath::noCurrentPoint (SplashPath.h:122).
func (p *Path) noCurrentPoint() bool {
	return p.curSubpath == len(p.pts)
}

// onePointSubpath mirrors SplashPath::onePointSubpath (SplashPath.h:123).
func (p *Path) onePointSubpath() bool {
	return p.curSubpath == len(p.pts)-1
}

// AddPath appends every point/flag of other to p (SplashPath::append, SplashPath.cc:104-119).
func (p *Path) AddPath(other *Path) {
	if other == nil || len(other.pts) == 0 {
		return
	}
	p.curSubpath = len(p.pts) + other.curSubpath
	p.pts = append(p.pts, other.pts...)
	p.flags = append(p.flags, other.flags...)
}

// Transformed returns a deep copy of p with every point transformed by matrix.
// Flags and stroke-adjust hint indices are preserved, matching SplashPath's
// point-index based contract.
func (p *Path) Transformed(matrix [6]float64) *Path {
	out := &Path{}
	if p == nil || len(p.pts) == 0 {
		return out
	}
	out.pts = make([]PathPoint, len(p.pts))
	for i, pt := range p.pts {
		out.pts[i] = PathPoint{
			X: pt.X*matrix[0] + pt.Y*matrix[2] + matrix[4],
			Y: pt.X*matrix[1] + pt.Y*matrix[3] + matrix[5],
		}
	}
	out.flags = append(out.flags, p.flags...)
	out.hints = append(out.hints, p.hints...)
	out.curSubpath = p.curSubpath
	return out
}

// MoveTo starts a new subpath at (x,y) (SplashPath::moveTo, SplashPath.cc:121-135).
func (p *Path) MoveTo(x, y float64) error {
	if p.onePointSubpath() {
		return errBogusPath
	}
	p.pts = append(p.pts, PathPoint{X: x, Y: y})
	p.flags = append(p.flags, pathFirst|pathLast)
	p.curSubpath = len(p.pts) - 1
	return nil
}

// MoveToDroppingEmptySubpath starts a new subpath after discarding a trailing
// one-point subpath. PDF content streams commonly emit consecutive `m`
// operators; Poppler's GfxPath conversion drops the empty subpath before it
// reaches SplashPath, so the canvas adapter needs this helper while keeping
// MoveTo's low-level SplashPath error contract intact.
func (p *Path) MoveToDroppingEmptySubpath(x, y float64) error {
	if p.onePointSubpath() {
		p.pts = p.pts[:p.curSubpath]
		p.flags = p.flags[:p.curSubpath]
		p.curSubpath = len(p.pts)
	}
	return p.MoveTo(x, y)
}

// LineTo appends a line segment to the last subpath (SplashPath::lineTo, SplashPath.cc:137-152).
func (p *Path) LineTo(x, y float64) error {
	if p.noCurrentPoint() {
		return errNoCurPt
	}
	p.flags[len(p.flags)-1] &^= pathLast
	p.pts = append(p.pts, PathPoint{X: x, Y: y})
	p.flags = append(p.flags, pathLast)
	return nil
}

// CurveTo appends a cubic Bezier (SplashPath::curveTo, SplashPath.cc:154-177).
//
// Three points are appended: the two control points carry pathCurve, the
// final endpoint carries pathLast. The C++ implementation grows by 3 in one
// shot (SplashPath.cc:160) — Go's append handles growth transparently.
func (p *Path) CurveTo(x1, y1, x2, y2, x3, y3 float64) error {
	if p.noCurrentPoint() {
		return errNoCurPt
	}
	p.flags[len(p.flags)-1] &^= pathLast
	p.pts = append(p.pts,
		PathPoint{X: x1, Y: y1},
		PathPoint{X: x2, Y: y2},
		PathPoint{X: x3, Y: y3},
	)
	p.flags = append(p.flags, pathCurve, pathCurve, pathLast)
	return nil
}

// Close closes the last subpath (SplashPath::close, SplashPath.cc:179-194).
//
// If force is true, or the subpath has only one point, or the last point
// differs from the subpath's first point, a synthesised lineTo to the first
// point is emitted before the closed flag is stamped on both endpoints.
func (p *Path) Close(force bool) error {
	if p.noCurrentPoint() {
		return errNoCurPt
	}
	first := p.pts[p.curSubpath]
	last := p.pts[len(p.pts)-1]
	if force || p.curSubpath == len(p.pts)-1 || last.X != first.X || last.Y != first.Y {
		if err := p.LineTo(first.X, first.Y); err != nil {
			return err
		}
	}
	p.flags[p.curSubpath] |= pathClosed
	p.flags[len(p.flags)-1] |= pathClosed
	p.curSubpath = len(p.pts)
	return nil
}

// AddStrokeAdjustHint appends a hint (SplashPath::addStrokeAdjustHint, SplashPath.cc:196-210).
func (p *Path) AddStrokeAdjustHint(ctrl0, ctrl1, firstPt, lastPt int) {
	p.hints = append(p.hints, PathHint{
		Ctrl0:   ctrl0,
		Ctrl1:   ctrl1,
		FirstPt: firstPt,
		LastPt:  lastPt,
	})
}

// Offset translates every point by (dx,dy) (SplashPath::offset, SplashPath.cc:212-220).
func (p *Path) Offset(dx, dy float64) {
	for i := range p.pts {
		p.pts[i].X += dx
		p.pts[i].Y += dy
	}
}

// GetCurPt reports the current point (SplashPath::getCurPt, SplashPath.cc:222-230).
func (p *Path) GetCurPt() (x, y float64, valid bool) {
	if p.noCurrentPoint() {
		return 0, 0, false
	}
	last := p.pts[len(p.pts)-1]
	return last.X, last.Y, true
}

// Length returns the number of points on the path (SplashPath::getLength, SplashPath.h:106).
func (p *Path) Length() int {
	return len(p.pts)
}

// IsCurSubpathClosed reports whether the last point in the current subpath carries pathClosed (SplashPath.h:49).
func (p *Path) IsCurSubpathClosed() bool {
	if len(p.flags) == 0 {
		return false
	}
	return p.flags[len(p.flags)-1]&pathClosed != 0
}

// IsEmpty reports whether p has no points (SplashPath::noCurrentPoint, SplashPath.h:122).
func (p *Path) IsEmpty() bool {
	return len(p.pts) == 0
}

// Hints returns the hint slice (read-only access for XPath consumers).
func (p *Path) Hints() []PathHint {
	return p.hints
}

// Point returns point i and its flag byte (SplashPath::getPoint, SplashPath.h:107-112).
func (p *Path) Point(i int) (PathPoint, byte) {
	return p.pts[i], p.flags[i]
}

// Flatten returns a new Path with every cubic Bezier replaced by a sequence of
// line segments, using identity device-space for the flatness test.
func (p *Path) Flatten(flatness float64) *Path {
	return p.FlattenWithMatrix(flatness, [6]float64{1, 0, 0, 1, 0, 0})
}

// FlattenWithMatrix mirrors Splash::flattenPath (Splash.cc:2043-2071). The
// emitted line endpoints stay in user space, but Poppler evaluates curve
// flatness in device space via the current matrix (Splash.cc:2110-2122).
// Subpath structure (pathFirst / pathLast / pathClosed) is preserved.
func (p *Path) FlattenWithMatrix(flatness float64, matrix [6]float64) *Path {
	out := &Path{}
	if p == nil || len(p.pts) == 0 {
		return out
	}
	flatness2 := flatness * flatness
	if flatness2 <= 0 {
		flatness2 = 1
	}
	n := len(p.pts)
	var x0, y0 float64
	subStartFlags := byte(0)
	i := 0
	for i < n {
		f := p.flags[i]
		if f&pathFirst != 0 {
			x0, y0 = p.pts[i].X, p.pts[i].Y
			out.pts = append(out.pts, PathPoint{X: x0, Y: y0})
			subStartFlags = pathFirst
			if f&pathLast != 0 {
				subStartFlags |= pathLast
			}
			if f&pathClosed != 0 {
				subStartFlags |= pathClosed
			}
			out.flags = append(out.flags, subStartFlags)
			out.curSubpath = len(out.pts) - 1
			i++
			continue
		}
		if f&pathCurve != 0 {
			x1 := p.pts[i].X
			y1 := p.pts[i].Y
			x2 := p.pts[i+1].X
			y2 := p.pts[i+1].Y
			x3 := p.pts[i+2].X
			y3 := p.pts[i+2].Y
			endFlag := p.flags[i+2]
			out.flattenCubicWithMatrix(x0, y0, x1, y1, x2, y2, x3, y3, matrix, flatness2, endFlag)
			x0, y0 = x3, y3
			i += 3
			continue
		}
		// Plain line segment: copy as-is.
		// The previous tail flag had pathLast set; clear it before appending.
		if len(out.flags) > 0 {
			out.flags[len(out.flags)-1] &^= pathLast
		}
		out.pts = append(out.pts, p.pts[i])
		out.flags = append(out.flags, f)
		x0, y0 = p.pts[i].X, p.pts[i].Y
		i++
	}
	return out
}

// flattenCubicWithMatrix subdivides one cubic via midpoint splitting (mirrors
// Splash::flattenCurve, Splash.cc:2074-2162) but emits line endpoints into a
// Path instead of XPath segments. endFlag is the flag byte that originally
// carried pathLast/pathClosed for the curve's terminal point and must be
// transferred to the final emitted endpoint.
func (p *Path) flattenCubicWithMatrix(x0, y0, x1, y1, x2, y2, x3, y3 float64, matrix [6]float64, flatness2 float64, endFlag byte) {
	const sz = 1<<10 + 1
	cx := make([]float64, sz*3)
	cy := make([]float64, sz*3)
	cNext := make([]int, sz)

	p1 := 0
	p2 := sz - 1

	cx[p1*3+0] = x0
	cx[p1*3+1] = x1
	cx[p1*3+2] = x2
	cx[p2*3+0] = x3
	cy[p1*3+0] = y0
	cy[p1*3+1] = y1
	cy[p1*3+2] = y2
	cy[p2*3+0] = y3
	cNext[p1] = p2

	// Clear pathLast on the previous tail since we're going to extend it.
	if len(p.flags) > 0 {
		p.flags[len(p.flags)-1] &^= pathLast
	}

	for p1 < sz-1 {
		xl0 := cx[p1*3+0]
		xx1 := cx[p1*3+1]
		xx2 := cx[p1*3+2]
		yl0 := cy[p1*3+0]
		yy1 := cy[p1*3+1]
		yy2 := cy[p1*3+2]
		p2 = cNext[p1]
		xr3 := cx[p2*3+0]
		yr3 := cy[p2*3+0]

		mx, my := transformPathPoint(matrix, (xl0+xr3)*0.5, (yl0+yr3)*0.5)
		tx, ty := transformPathPoint(matrix, xx1, yy1)
		dx := tx - mx
		dy := ty - my
		d1 := dx*dx + dy*dy
		tx, ty = transformPathPoint(matrix, xx2, yy2)
		dx = tx - mx
		dy = ty - my
		d2 := dx*dx + dy*dy

		if p2-p1 == 1 || (d1 <= flatness2 && d2 <= flatness2) {
			// Emit endpoint (xr3, yr3). The terminal segment carries endFlag.
			isLast := p2 == sz-1
			flag := byte(0)
			if isLast {
				flag = endFlag
			}
			p.pts = append(p.pts, PathPoint{X: xr3, Y: yr3})
			p.flags = append(p.flags, flag)
			p1 = p2
			continue
		}

		// de Casteljau split at t=0.5.
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

func transformPathPoint(matrix [6]float64, x, y float64) (float64, float64) {
	return x*matrix[0] + y*matrix[2] + matrix[4], x*matrix[1] + y*matrix[3] + matrix[5]
}
