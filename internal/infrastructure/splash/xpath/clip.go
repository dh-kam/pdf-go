package xpath

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// errEmptyClip is the sentinel returned when ClipToRect collapses the clip to empty.
//
// Poppler returns splashOk in these cases (SplashClip.cc:178) — the empty
// state is encoded in xMax<xMin and downstream TestRect/TestSpan return
// AllOutside. We surface a non-fatal sentinel so callers can detect the
// transition without inspecting clip internals.
var errEmptyClip = errors.New("xpath: clip collapsed to empty")

// splashClipEO mirrors the bit defined at SplashClip.cc:40.
const splashClipEO byte = 0x01

// splashCeilLocal mirrors splashCeil (SplashMath.h:89) — rounds toward +inf.
//
// xpath.go already defines splashFloor; we add splashCeil locally rather than
// importing the parent splash package, per Phase 2 constraints.
func splashCeilLocal(x float64) int {
	if x < 0 {
		return -splashFloor(-x)
	}
	if x == math.Floor(x) {
		return int(x)
	}
	return int(x) + 1
}

// ClipResult mirrors SplashClipResult (SplashClip.h:38-43).
type ClipResult uint8

// ClipResult values match the SplashClip.h:38-43 enum order.
const (
	ClipAllInside  ClipResult = iota // splashClipAllInside (SplashClip.h:40)
	ClipAllOutside                   // splashClipAllOutside (SplashClip.h:41)
	ClipPartial                      // splashClipPartial (SplashClip.h:42)
)

// Clip mirrors SplashClip (SplashClip.h:49-129).
//
// Per locked decision D5 we ship the Poppler-faithful flags+scanners form.
// scanners holds shared *Scanner values (immutable after construction), eo
// holds the per-scanner even-odd bit (= splashClipEO in Poppler's flags byte),
// and flags is the per-row classification byte that Poppler's
// SplashClip.h:126 documents. We allocate flags lazily on first ClipToPath.
type Clip struct {
	hardXMin, hardYMin int
	hardXMax, hardYMax int

	// xMin/xMax/yMin/yMax are the floating-point clip bounds tracked alongside
	// integer xMinI/xMaxI/yMinI/yMaxI counterparts (SplashClip.h:124-125).
	xMinFP, yMinFP float64
	xMaxFP, yMaxFP float64
	xMin, yMin     int // integer xMinI/yMinI (SplashClip.h:125)
	xMax, yMax     int // integer xMaxI/yMaxI (SplashClip.h:125)

	pathXMinFP, pathYMinFP float64
	pathXMaxFP, pathYMaxFP float64
	hasPathBounds          bool

	// scanners + eo are the parallel shared-scanner / even-odd-flag arrays
	// (SplashClip.h:127, splashClipEO bit at SplashClip.cc:40,210).
	// Per ownership rules in 02_api_design.md §6, scanners are shared (not
	// deep-copied) on Clone; the slice header IS deep-copied so independent
	// clips can grow independently.
	scanners []*Scanner
	eo       []bool

	// flags mirrors SplashClip's per-path flag byte (SplashClip.h:126). Allocated
	// lazily — nil until first ClipToPath records a path-clip.
	flags []byte

	antialias bool
}

// NewClip creates a clip bounded by the hard rectangle (SplashClip ctor,
// SplashClip.cc:46-69).
//
// hardXMax/hardYMax are interpreted as inclusive integer pixel bounds — i.e.,
// hard rect = pixels in [hardXMin..hardXMax] × [hardYMin..hardYMax]. The
// live clip's float upper bounds are stored as (hardXMax+1, hardYMax+1) so
// that a freshly-built clip's TestRect(hardXMin, hardYMin, hardXMax, hardYMax)
// reports ClipAllInside. Poppler's SplashState calls the float-coord ctor
// with `width-0.001` so the integer xMaxI lands at width-1 (SplashState.cc:69);
// our integer-bounds API folds that convention in.
func NewClip(hardXMin, hardYMin, hardXMax, hardYMax int, antialias bool) *Clip {
	c := &Clip{
		hardXMin:  hardXMin,
		hardYMin:  hardYMin,
		hardXMax:  hardXMax,
		hardYMax:  hardYMax,
		antialias: antialias,
	}
	c.xMinFP = float64(hardXMin)
	c.yMinFP = float64(hardYMin)
	c.xMaxFP = float64(hardXMax + 1)
	c.yMaxFP = float64(hardYMax + 1)
	c.xMin = splashFloor(c.xMinFP)
	c.yMin = splashFloor(c.yMinFP)
	c.xMax = splashCeilLocal(c.xMaxFP) - 1
	c.yMax = splashCeilLocal(c.yMaxFP) - 1
	return c
}

// Clone returns a copy of c with shared-scanner semantics
// (SplashClip::SplashClip(const SplashClip*), SplashClip.cc:71-91).
//
// scanners are shared (immutable post-construction — see 02_api_design.md §6),
// so the slice's underlying *Scanner pointers are reused. eo and flags are
// deep-copied so the two clips can diverge under further ClipToPath calls.
func (c *Clip) Clone() *Clip {
	cp := &Clip{
		hardXMin:      c.hardXMin,
		hardYMin:      c.hardYMin,
		hardXMax:      c.hardXMax,
		hardYMax:      c.hardYMax,
		xMinFP:        c.xMinFP,
		yMinFP:        c.yMinFP,
		xMaxFP:        c.xMaxFP,
		yMaxFP:        c.yMaxFP,
		xMin:          c.xMin,
		yMin:          c.yMin,
		xMax:          c.xMax,
		yMax:          c.yMax,
		pathXMinFP:    c.pathXMinFP,
		pathYMinFP:    c.pathYMinFP,
		pathXMaxFP:    c.pathXMaxFP,
		pathYMaxFP:    c.pathYMaxFP,
		hasPathBounds: c.hasPathBounds,
		antialias:     c.antialias,
	}
	if len(c.scanners) > 0 {
		// Slice is copied (own header) but elements are shared *Scanner pointers.
		cp.scanners = make([]*Scanner, len(c.scanners))
		copy(cp.scanners, c.scanners)
	}
	if len(c.eo) > 0 {
		cp.eo = make([]bool, len(c.eo))
		copy(cp.eo, c.eo)
	}
	if len(c.flags) > 0 {
		cp.flags = make([]byte, len(c.flags))
		copy(cp.flags, c.flags)
	}
	return cp
}

// ResetToRect resets the clip to a rectangle and drops all path-clips
// (SplashClip::resetToRect, SplashClip.cc:111-136). After return, the clip
// has no scanners and bounds equal to the supplied rectangle (clamped to
// the hard rectangle is NOT done by Poppler here — only the constructor's
// argument-order normalisation, which we replicate).
func (c *Clip) ResetToRect(x0, y0, x1, y1 float64) {
	// Drop any existing path-clip state.
	c.scanners = nil
	c.eo = nil
	c.flags = nil
	c.hasPathBounds = false

	if x0 < x1 {
		c.xMinFP = x0
		c.xMaxFP = x1
	} else {
		c.xMinFP = x1
		c.xMaxFP = x0
	}
	if y0 < y1 {
		c.yMinFP = y0
		c.yMaxFP = y1
	} else {
		c.yMinFP = y1
		c.yMaxFP = y0
	}
	c.xMin = splashFloor(c.xMinFP)
	c.yMin = splashFloor(c.yMinFP)
	c.xMax = splashCeilLocal(c.xMaxFP) - 1
	c.yMax = splashCeilLocal(c.yMaxFP) - 1
}

// ClipToRect intersects the clip with a rectangle (SplashClip::clipToRect,
// SplashClip.cc:138-179).
//
// Returns errEmptyClip if the intersection collapses the clip to empty. In
// Poppler this is splashOk + xMax<xMin sentinel; we surface the sentinel so
// downstream callers can short-circuit. The sentinel is NOT propagated up the
// stack — TestRect/TestSpan must still work correctly on an empty clip.
func (c *Clip) ClipToRect(x0, y0, x1, y1 float64) error {
	if x0 < x1 {
		if x0 > c.xMinFP {
			c.xMinFP = x0
			c.xMin = splashFloor(c.xMinFP)
		}
		if x1 < c.xMaxFP {
			c.xMaxFP = x1
			c.xMax = splashCeilLocal(c.xMaxFP) - 1
		}
	} else {
		if x1 > c.xMinFP {
			c.xMinFP = x1
			c.xMin = splashFloor(c.xMinFP)
		}
		if x0 < c.xMaxFP {
			c.xMaxFP = x0
			c.xMax = splashCeilLocal(c.xMaxFP) - 1
		}
	}
	if y0 < y1 {
		if y0 > c.yMinFP {
			c.yMinFP = y0
			c.yMin = splashFloor(c.yMinFP)
		}
		if y1 < c.yMaxFP {
			c.yMaxFP = y1
			c.yMax = splashCeilLocal(c.yMaxFP) - 1
		}
	} else {
		if y1 > c.yMinFP {
			c.yMinFP = y1
			c.yMin = splashFloor(c.yMinFP)
		}
		if y0 < c.yMaxFP {
			c.yMaxFP = y0
			c.yMax = splashCeilLocal(c.yMaxFP) - 1
		}
	}
	if c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP {
		return errEmptyClip
	}
	return nil
}

// ClipToPathRectOnly applies ClipToPath only for Poppler's axis-aligned
// rectangle fast path. It returns false for general paths so callers can defer
// path-scanner clipping until their coordinate convention is known to match.
func (c *Clip) ClipToPathRectOnly(path *Path, matrix [6]float64, flatness float64) (bool, error) {
	xPath := NewXPath(path, matrix, flatness, true)
	if xPath.Length() == 4 && isAxisAlignedRect(xPath) {
		return true, c.ClipToRect(xPath.Segs[0].X0, xPath.Segs[0].Y0, xPath.Segs[2].X0, xPath.Segs[2].Y0)
	}
	return false, nil
}

// IntBounds returns the current integer clip bounds
// (SplashClip::getXMinI/getYMinI/getXMaxI/getYMaxI).
func (c *Clip) IntBounds() (xMin, yMin, xMax, yMax int) {
	return c.xMin, c.yMin, c.xMax, c.yMax
}

// Bounds returns the floating-point clip bounds
// (SplashClip::getXMin/getYMin/getXMax/getYMax).
func (c *Clip) Bounds() (xMin, yMin, xMax, yMax float64) {
	return c.xMinFP, c.yMinFP, c.xMaxFP, c.yMaxFP
}

// EffectiveBounds returns the visible clip bounds after intersecting the
// rectangular clip with any path-scanner bounds. This mirrors the bbox that
// GfxState exposes to Poppler's univariate shading path.
func (c *Clip) EffectiveBounds() (xMin, yMin, xMax, yMax float64, ok bool) {
	if c == nil || c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP {
		return 0, 0, 0, 0, false
	}
	xMin, yMin, xMax, yMax = c.xMinFP, c.yMinFP, c.xMaxFP, c.yMaxFP
	for _, scanner := range c.scanners {
		if scanner == nil || scanner.xMax < scanner.xMin || scanner.yMax < scanner.yMin {
			return 0, 0, 0, 0, false
		}

		sxMin := float64(scanner.xMin)
		syMin := float64(scanner.yMin)
		sxMax := float64(scanner.xMax + 1)
		syMax := float64(scanner.yMax + 1)
		if c.antialias {
			sxMin /= aaSize
			syMin /= aaSize
			sxMax /= aaSize
			syMax /= aaSize
		}
		if sxMin > xMin {
			xMin = sxMin
		}
		if syMin > yMin {
			yMin = syMin
		}
		if sxMax < xMax {
			xMax = sxMax
		}
		if syMax < yMax {
			yMax = syMax
		}
		if xMax <= xMin || yMax <= yMin {
			return 0, 0, 0, 0, false
		}
	}
	return xMin, yMin, xMax, yMax, true
}

// VectorEffectiveBounds returns the current clip bounds as tracked from vector
// path geometry, without scanner AA expansion. This mirrors GfxState's
// clipXMin/YMin bbox used by Poppler's univariate shading cache setup.
func (c *Clip) VectorEffectiveBounds() (xMin, yMin, xMax, yMax float64, ok bool) {
	if c == nil || c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP {
		return 0, 0, 0, 0, false
	}
	xMin, yMin, xMax, yMax = c.xMinFP, c.yMinFP, c.xMaxFP, c.yMaxFP
	if c.hasPathBounds {
		if c.pathXMinFP > xMin {
			xMin = c.pathXMinFP
		}
		if c.pathYMinFP > yMin {
			yMin = c.pathYMinFP
		}
		if c.pathXMaxFP < xMax {
			xMax = c.pathXMaxFP
		}
		if c.pathYMaxFP < yMax {
			yMax = c.pathYMaxFP
		}
	}
	if xMax <= xMin || yMax <= yMin {
		return 0, 0, 0, 0, false
	}
	return xMin, yMin, xMax, yMax, true
}

// HasPathClip reports whether the clip contains any path scanners
// (SplashClip::getNumPaths() > 0).
func (c *Clip) HasPathClip() bool {
	return len(c.scanners) > 0
}

// ClipToPath intersects the clip with a path (SplashClip::clipToPath,
// SplashClip.cc:181-223 + the matrix/flatness wiring from Splash.cc:1704-1707).
//
// Fast path: if the flattened XPath has exactly 4 segments forming a closed
// axis-aligned rectangle (SplashClip.cc:195-202, exact float == comparisons),
// dispatch directly to ClipToRect — no scanner is built.
//
// Otherwise the XPath is sorted (and AA-scaled if antialias is set), a Scanner
// is constructed, and the (scanner, eo) pair is appended to the parallel
// scanners/eo arrays. flags grows in lockstep, holding splashClipEO|0 per
// SplashClip.cc:210.
func (c *Clip) ClipToPath(path *Path, matrix [6]float64, flatness float64, eo bool) error {
	xPath := NewXPath(path, matrix, flatness, true)

	// Empty path collapses the clip (SplashClip.cc:188-192).
	if xPath.Length() == 0 {
		c.xMaxFP = c.xMinFP - 1
		c.yMaxFP = c.yMinFP - 1
		c.xMax = splashCeilLocal(c.xMaxFP) - 1
		c.yMax = splashCeilLocal(c.yMaxFP) - 1
		return nil
	}

	// Axis-aligned rect detection (SplashClip.cc:195-202). EXACT == per the
	// faithfulness rule in the task brief and SP3 §6a.
	if xPath.Length() == 4 && isAxisAlignedRect(xPath) {
		return c.ClipToRect(xPath.Segs[0].X0, xPath.Segs[0].Y0, xPath.Segs[2].X0, xPath.Segs[2].Y0)
	}
	if !c.hasPathBounds {
		c.intersectPathBounds(xPath)
	}

	// General path: sort + (optional) aaScale + emplace scanner.
	if c.antialias {
		xPath.AAScale()
	}
	xPath.Sort()

	var yMinAA, yMaxAA int
	if c.antialias {
		yMinAA = c.yMin * aaSize
		yMaxAA = (c.yMax+1)*aaSize - 1
	} else {
		yMinAA = c.yMin
		yMaxAA = c.yMax
	}

	// Scanner.NewScanner is provided by Dev1 (Phase 2). When that lands the
	// signature is NewScanner(x *XPath, eo bool, xMin, yMin, xMax, yMax int).
	scanner := NewScanner(xPath, eo, c.xMin, yMinAA, c.xMax, yMaxAA)

	// Append parallel entries; flags grows in lockstep per SplashClip.cc:210.
	c.scanners = append(c.scanners, scanner)
	c.eo = append(c.eo, eo)
	var flag byte
	if eo {
		flag = splashClipEO
	}
	c.flags = append(c.flags, flag)
	return nil
}

func (c *Clip) intersectPathBounds(xPath *XPath) {
	if c == nil || xPath == nil || len(xPath.Segs) == 0 {
		return
	}
	xMin, yMin := xPath.Segs[0].X0, xPath.Segs[0].Y0
	xMax, yMax := xMin, yMin
	for _, seg := range xPath.Segs {
		for _, pt := range [][2]float64{{seg.X0, seg.Y0}, {seg.X1, seg.Y1}} {
			if pt[0] < xMin {
				xMin = pt[0]
			}
			if pt[0] > xMax {
				xMax = pt[0]
			}
			if pt[1] < yMin {
				yMin = pt[1]
			}
			if pt[1] > yMax {
				yMax = pt[1]
			}
		}
	}
	if !c.hasPathBounds {
		c.pathXMinFP, c.pathYMinFP = xMin, yMin
		c.pathXMaxFP, c.pathYMaxFP = xMax, yMax
		c.hasPathBounds = true
		return
	}
	if xMin > c.pathXMinFP {
		c.pathXMinFP = xMin
	}
	if yMin > c.pathYMinFP {
		c.pathYMinFP = yMin
	}
	if xMax < c.pathXMaxFP {
		c.pathXMaxFP = xMax
	}
	if yMax < c.pathYMaxFP {
		c.pathYMaxFP = yMax
	}
}

// IntersectVectorBounds narrows the clip bbox with original vector path
// bounds before XPath flattening. Poppler's GfxState::clip() uses the source
// path points for this bbox, while SplashClip's scanner uses the flattened
// XPath for pixel coverage.
func (c *Clip) IntersectVectorBounds(xMin, yMin, xMax, yMax float64) {
	if c == nil || xMax <= xMin || yMax <= yMin {
		return
	}
	if !c.hasPathBounds {
		c.pathXMinFP, c.pathYMinFP = xMin, yMin
		c.pathXMaxFP, c.pathYMaxFP = xMax, yMax
		c.hasPathBounds = true
		return
	}
	if xMin > c.pathXMinFP {
		c.pathXMinFP = xMin
	}
	if yMin > c.pathYMinFP {
		c.pathYMinFP = yMin
	}
	if xMax < c.pathXMaxFP {
		c.pathXMaxFP = xMax
	}
	if yMax < c.pathYMaxFP {
		c.pathYMaxFP = yMax
	}
}

// isAxisAlignedRect implements the predicate at SplashClip.cc:195-202.
//
// The path is recognised as a closed axis-aligned rectangle if the four
// segments alternate Vert,Horiz,Vert,Horiz OR Horiz,Vert,Horiz,Vert AND
// share endpoints to form a closed rectangle. EXACT == comparison — no
// epsilon — per the task's faithfulness rule.
func isAxisAlignedRect(x *XPath) bool {
	if len(x.Segs) != 4 {
		return false
	}
	s := x.Segs
	// Pattern A: segs 0,2 are vertical (x0==x1) and segs 1,3 are horizontal
	// (y0==y1) — segments alternate V,H,V,H. SplashClip.cc:196-198.
	if s[0].X0 == s[0].X1 && s[0].X0 == s[1].X0 && s[0].X0 == s[3].X1 &&
		s[2].X0 == s[2].X1 && s[2].X0 == s[1].X1 &&
		s[2].X0 == s[3].X0 && s[1].Y0 == s[1].Y1 && s[1].Y0 == s[0].Y1 &&
		s[1].Y0 == s[2].Y0 && s[3].Y0 == s[3].Y1 &&
		s[3].Y0 == s[0].Y0 && s[3].Y0 == s[2].Y1 {
		return true
	}
	// Pattern B: segs 0,2 are horizontal and segs 1,3 are vertical —
	// segments alternate H,V,H,V. SplashClip.cc:199-201.
	if s[0].Y0 == s[0].Y1 && s[0].Y0 == s[1].Y0 && s[0].Y0 == s[3].Y1 &&
		s[2].Y0 == s[2].Y1 && s[2].Y0 == s[1].Y1 &&
		s[2].Y0 == s[3].Y0 && s[1].X0 == s[1].X1 && s[1].X0 == s[0].X1 &&
		s[1].X0 == s[2].X0 && s[3].X0 == s[3].X1 &&
		s[3].X0 == s[0].X0 && s[3].X0 == s[2].X1 {
		return true
	}
	return false
}

// TestRect classifies a rectangle against the clip
// (SplashClip::testRect, SplashClip.cc:225-240).
//
// Empty clip (xMaxFP < xMinFP, set by ClipToRect/ClipToPath when bounds
// collapse) is caught up front and reported AllOutside without consulting
// scanners — per the task brief's empty-sentinel rule. Then the cheap rect-
// vs-bounds test from Poppler runs unchanged.
func (c *Clip) TestRect(rectXMin, rectYMin, rectXMax, rectYMax int) ClipResult {
	if c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP {
		return ClipAllOutside
	}
	if float64(rectXMax+1) <= c.xMinFP || float64(rectXMin) >= c.xMaxFP ||
		float64(rectYMax+1) <= c.yMinFP || float64(rectYMin) >= c.yMaxFP {
		return ClipAllOutside
	}
	if float64(rectXMin) >= c.xMinFP && float64(rectXMax+1) <= c.xMaxFP &&
		float64(rectYMin) >= c.yMinFP && float64(rectYMax+1) <= c.yMaxFP &&
		len(c.scanners) == 0 {
		return ClipAllInside
	}
	return ClipPartial
}

// Test reports whether pixel (x,y) is inside the clip (SplashClip.h:73-82).
func (c *Clip) Test(x, y int) bool {
	if c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP {
		return false
	}
	if x < c.xMin || x > c.xMax || y < c.yMin || y > c.yMax {
		return false
	}
	if c.antialias {
		x *= aaSize
		y *= aaSize
	}
	for _, scanner := range c.scanners {
		if scanner == nil || !scanner.Test(x, y) {
			return false
		}
	}
	return true
}

// TestSpan classifies a horizontal span against the clip
// (SplashClip::testSpan, SplashClip.cc:242-272).
//
// Used by inner fill loops. Empty clip is short-circuited to AllOutside;
// then Partial is returned when the span straddles the rect bounds;
// otherwise scanners are queried via their testSpan method (with antialias
// multiplier) and a single Partial vote downgrades the result.
func (c *Clip) TestSpan(spanXMin, spanXMax, spanY int) ClipResult {
	if c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP {
		return ClipAllOutside
	}
	if float64(spanXMax+1) <= c.xMinFP || float64(spanXMin) >= c.xMaxFP ||
		float64(spanY+1) <= c.yMinFP || float64(spanY) >= c.yMaxFP {
		return ClipAllOutside
	}
	if !(float64(spanXMin) >= c.xMinFP && float64(spanXMax+1) <= c.xMaxFP &&
		float64(spanY) >= c.yMinFP && float64(spanY+1) <= c.yMaxFP) {
		return ClipPartial
	}
	for _, scanner := range c.scanners {
		if scanner == nil {
			return ClipPartial
		}
		x0, x1, y := spanXMin, spanXMax, spanY
		if c.antialias {
			x0 = spanXMin * aaSize
			x1 = spanXMax*aaSize + (aaSize - 1)
			y = spanY * aaSize
		}
		if !scanner.TestSpan(x0, x1, y) {
			return ClipPartial
		}
	}
	return ClipAllInside
}

// ClipAALine intersects aaBuf with the active clip at AA sub-pixel resolution.
//
// This mirrors SplashClip::clipAALine: first mask the rectangular clip bounds
// in X, then AND each path scanner's coverage into the caller's AA buffer.
func (c *Clip) ClipAALine(y int, aaBuf []byte, xMin, xMax int) {
	width := (xMax - xMin + 1) * aaSize
	if width <= 0 {
		return
	}
	rowSize := (width + 7) >> 3
	traceTargets := parseClipAABufTraceTargets()
	if c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP ||
		float64(y+1) <= c.yMinFP || float64(y) >= c.yMaxFP {
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, 0, width)
		}
		traceClipAABufTargets("empty", -1, c, nil, traceTargets, y, aaBuf, rowSize, xMin, xMax)
		return
	}

	traceClipAABufTargets("before-rect", -1, c, nil, traceTargets, y, aaBuf, rowSize, xMin, xMax)
	left := splashFloor(c.xMinFP*aaSize) - xMin*aaSize
	if left > 0 {
		if left > width {
			left = width
		}
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, 0, left)
		}
	}

	right := splashFloor(c.xMaxFP*aaSize) + 1 - xMin*aaSize
	if right < width {
		if right < 0 {
			right = 0
		}
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, right, width)
		}
	}
	traceClipAABufTargets("after-rect", -1, c, nil, traceTargets, y, aaBuf, rowSize, xMin, xMax)

	for i, scanner := range c.scanners {
		if scanner != nil {
			traceClipAABufTargets("before-scanner", i, c, scanner, traceTargets, y, aaBuf, rowSize, xMin, xMax)
			scanner.ClipAALine(y, aaBuf, xMin, xMax)
			traceClipAABufTargets("after-scanner", i, c, scanner, traceTargets, y, aaBuf, rowSize, xMin, xMax)
		}
	}
}

type clipAABufTraceTarget struct {
	x int
	y int
}

func parseClipAABufTraceTargets() []clipAABufTraceTarget {
	spec := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_CLIP_AABUF_TRACE"))
	if spec == "" {
		return nil
	}
	parts := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ';' || r == ' '
	})
	targets := make([]clipAABufTraceTarget, 0, len(parts))
	for _, part := range parts {
		xy := strings.Split(part, ",")
		if len(xy) != 2 {
			continue
		}
		x, errX := strconv.Atoi(strings.TrimSpace(xy[0]))
		y, errY := strconv.Atoi(strings.TrimSpace(xy[1]))
		if errX == nil && errY == nil {
			targets = append(targets, clipAABufTraceTarget{x: x, y: y})
		}
	}
	return targets
}

func traceClipAABufTargets(phase string, scannerIndex int, c *Clip, scanner *Scanner, targets []clipAABufTraceTarget, y int, aaBuf []byte, rowSize, xMin, xMax int) {
	if len(targets) == 0 {
		return
	}
	for _, target := range targets {
		if target.y != y || target.x < xMin || target.x > xMax {
			continue
		}
		shape := countClipAABufPixel(aaBuf, rowSize, xMin, target.x)
		if scanner != nil {
			sx0, sy0, sx1, sy1 := scanner.BBox()
			fmt.Fprintf(os.Stderr, "SPLASH_CLIP_AABUF_TRACE phase=%s scanner=%d x=%d y=%d shape=%d xMin=%d xMax=%d clip=(%.6f,%.6f)-(%.6f,%.6f) scannerBBox=(%d,%d)-(%d,%d)\n",
				phase, scannerIndex, target.x, target.y, shape, xMin, xMax, c.xMinFP, c.yMinFP, c.xMaxFP, c.yMaxFP, sx0, sy0, sx1, sy1)
			continue
		}
		fmt.Fprintf(os.Stderr, "SPLASH_CLIP_AABUF_TRACE phase=%s scanner=%d x=%d y=%d shape=%d xMin=%d xMax=%d clip=(%.6f,%.6f)-(%.6f,%.6f)\n",
			phase, scannerIndex, target.x, target.y, shape, xMin, xMax, c.xMinFP, c.yMinFP, c.xMaxFP, c.yMaxFP)
	}
}

func countClipAABufPixel(aaBuf []byte, rowSize, xMin, x int) int {
	cell := (x - xMin) * aaSize
	if cell < 0 {
		return 0
	}
	count := 0
	for yy := 0; yy < aaSize; yy++ {
		rowOff := yy * rowSize
		for xx := 0; xx < aaSize; xx++ {
			bit := cell + xx
			byteIdx := rowOff + (bit >> 3)
			if byteIdx < 0 || byteIdx >= len(aaBuf) {
				continue
			}
			if aaBuf[byteIdx]&(1<<(7-(bit&7))) != 0 {
				count++
			}
		}
	}
	return count
}

// ClipAALineFullWidth intersects a Poppler-style full-width AA buffer with the
// active clip over the row-local x0/x1 range returned by RenderAALineFullWidth.
func (c *Clip) ClipAALineFullWidth(y int, aaBuf []byte, x0, x1, bitmapWidth int) {
	width := bitmapWidth * aaSize
	if width <= 0 {
		return
	}
	rowSize := (width + 7) >> 3
	leftLimit := x0 * aaSize
	rightLimit := (x1 + 1) * aaSize
	traceTargets := parseClipAABufTraceTargets()
	trace := os.Getenv("PDF_DEBUG_SPLASH_CLIP_AABUF_TRACE") != "" && len(traceTargets) > 0
	if trace {
		traceClipAABufTargets("full-before-rect", -1, c, nil, traceTargets, y, aaBuf, rowSize, 0, bitmapWidth-1)
	}
	if rightLimit > width {
		rightLimit = width
	}
	if c.xMaxFP < c.xMinFP || c.yMaxFP < c.yMinFP ||
		float64(y+1) <= c.yMinFP || float64(y) >= c.yMaxFP {
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, leftLimit, rightLimit)
		}
		if trace {
			traceClipAABufTargets("full-empty", -1, c, nil, traceTargets, y, aaBuf, rowSize, 0, bitmapWidth-1)
		}
		return
	}

	left := splashFloor(c.xMinFP * aaSize)
	if left > leftLimit {
		if left > rightLimit {
			left = rightLimit
		}
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, leftLimit, left)
		}
	}

	right := splashFloor(c.xMaxFP*aaSize) + 1
	if right < rightLimit {
		if right < leftLimit {
			right = leftLimit
		}
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, right, rightLimit)
		}
	}
	if trace {
		traceClipAABufTargets("full-after-rect", -1, c, nil, traceTargets, y, aaBuf, rowSize, 0, bitmapWidth-1)
	}

	for i, scanner := range c.scanners {
		if scanner != nil {
			if trace {
				traceClipAABufTargets("full-before-scanner", i, c, scanner, traceTargets, y, aaBuf, rowSize, 0, bitmapWidth-1)
			}
			scanner.ClipAALineFullWidth(y, aaBuf, x0, x1, bitmapWidth)
			if trace {
				traceClipAABufTargets("full-after-scanner", i, c, scanner, traceTargets, y, aaBuf, rowSize, 0, bitmapWidth-1)
			}
		}
	}
}
