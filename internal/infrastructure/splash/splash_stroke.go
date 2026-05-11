package splash

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// Path flag bits, mirroring SplashPath.h:42,45,49 (also xpath/path.go pathFirst/Last/Closed).
// Duplicated here as exported-by-position constants because xpath does not export the bit
// names; the values are part of the on-the-wire path format and won't change.
const (
	pathFlagFirst  byte = 0x01 // SplashPath.h:42
	pathFlagLast   byte = 0x02 // SplashPath.h:45
	pathFlagClosed byte = 0x04 // SplashPath.h:49
)

// bezierCircle / bezierCircle2 mirror the two constants used at Splash.cc near round-cap
// expansions (Splash.cc:5904-5910). Used by makeStrokePath round-cap (Phase-2 stub uses
// linear approximation; constants kept for future parity).
const (
	bezierCircle  = 0.55228475 // 4*(sqrt(2)-1)/3, Splash.cc round-cap quarter-arc kappa
	bezierCircle2 = 0.27614237 // bezierCircle / 2
)

// Stroke is replaced in splash.go to forward to strokeImpl. This file owns the body.
// strokeImpl mirrors Splash::stroke (Splash.cc:1886-1943).
func (s *Splash) strokeImpl(p *xpath.Path) error {
	if p == nil || p.IsEmpty() {
		return ErrEmptyPath
	}

	// Flatten cubic Beziers before dashing / stroke-outline generation
	// (Splash.cc:1899). Poppler keeps emitted endpoints in user space but tests
	// flatness in device space through the current matrix (Splash.cc:2110-2122).
	path2 := p.FlattenWithMatrix(debugStrokeFlatness(s.state.flatness), s.state.matrix)
	if path2 == nil || path2.IsEmpty() {
		return ErrEmptyPath
	}

	// Apply dash conversion if a dash array is set (Splash.cc:1900-1908).
	dashedStroke := len(s.state.lineDash) > 0
	if dashedStroke {
		dashed, err := s.makeDashedPath(path2)
		if err != nil {
			return err
		}
		if dashed.IsEmpty() {
			return ErrEmptyPath
		}
		path2 = dashed
	}

	// Compute approximate transformed line width via half the larger unit-square diagonal
	// (Splash.cc:1910-1922).
	t1 := s.state.matrix[0] + s.state.matrix[2]
	t2 := s.state.matrix[1] + s.state.matrix[3]
	d1 := t1*t1 + t2*t2
	t1 = s.state.matrix[0] - s.state.matrix[2]
	t2 = s.state.matrix[1] - s.state.matrix[3]
	d2 := t1*t1 + t2*t2
	if d2 > d1 {
		d1 = d2
	}
	d1 *= 0.5

	// minLineWidth bump (Splash.cc:1923-1925).
	if d1 > 0 && d1*s.state.lineWidth*s.state.lineWidth < s.minLineWidth*s.minLineWidth {
		w := s.minLineWidth / math.Sqrt(d1)
		return s.strokeWide(path2, w, dashedStroke)
	}

	// Mode mono1 fast path is intentionally not ported: ModeRGB8 path only (per scope).
	// See Splash.cc:1926-1932 for the mono1 branch we skip.

	if s.state.lineWidth == 0 {
		return s.strokeNarrow(path2)
	}
	return s.strokeWide(path2, s.state.lineWidth, dashedStroke)
}

func debugStrokeFlatness(defaultFlatness float64) float64 {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_STROKE_FLATNESS"))
	if raw == "" {
		return defaultFlatness
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return defaultFlatness
	}
	return v
}

// strokeNarrow rasterises a 1-pixel-wide stroke directly to the bitmap
// (Splash.cc:1945-2032). Uses the same +1/-1 last-pixel inclusive rule as the
// reference implementation (Splash.cc:1995-1997 for positive slope,
// Splash.cc:2010-2012 for negative slope), preserving the project memory
// "butt-cap thin stroke" fix.
func (s *Splash) strokeNarrow(p *xpath.Path) error {
	xPath := xpath.NewXPath(p, s.state.matrix, s.state.flatness, false)
	return s.strokeNarrowXPath(xPath)
}

// strokeNarrowXPath is the test seam for strokeNarrow that accepts an
// already-built XPath. This isolates rasterisation from xpath flattening so
// unit tests can synthesise segments by hand.
func (s *Splash) strokeNarrowXPath(xPath *xpath.XPath) error {
	if xPath == nil {
		return nil
	}

	var c Color
	if s.state.strokePattern != nil {
		s.state.strokePattern.GetColor(0, 0, &c)
	}
	alpha := uint8(Round(s.state.strokeAlpha * 255))

	for i := range xPath.Segs {
		seg := xPath.Segs[i]
		var x0, x1, y0, y1 int
		if seg.Y0 <= seg.Y1 {
			y0 = Floor(seg.Y0)
			y1 = Floor(seg.Y1)
			x0 = Floor(seg.X0)
			x1 = Floor(seg.X1)
		} else {
			y0 = Floor(seg.Y1)
			y1 = Floor(seg.Y0)
			x0 = Floor(seg.X1)
			x1 = Floor(seg.X0)
		}

		if y0 == y1 {
			if x0 <= x1 {
				s.drawSpan(x0, x1, y0, c, alpha)
			} else {
				s.drawSpan(x1, x0, y0, c, alpha)
			}
			continue
		}

		// Slope march with inclusive last-pixel rule (Splash.cc:1990-2020).
		dxdy := seg.DXDY
		segX0 := seg.X0
		segY0 := seg.Y0
		// xpath stores segments with the topological flag XPathFlip set when y0>y1;
		// strokeNarrow re-orients via the y0<=y1 branch above, so segX0/segY0 must be
		// the sorted endpoint matching x0/y0. Recover it:
		if seg.Y0 > seg.Y1 {
			segX0 = seg.X1
			segY0 = seg.Y1
		}

		if x0 <= x1 {
			xa := x0
			for y := y0; y <= y1; y++ {
				var xb int
				if y < y1 {
					xb = Floor(segX0 + (float64(y)+1-segY0)*dxdy)
				} else {
					xb = x1 + 1 // inclusive last pixel (Splash.cc:1995-1997)
				}
				if xa == xb {
					s.drawPixel(xa, y, c, alpha)
				} else {
					s.drawSpan(xa, xb-1, y, c, alpha)
				}
				xa = xb
			}
		} else {
			xa := x0
			for y := y0; y <= y1; y++ {
				var xb int
				if y < y1 {
					xb = Floor(segX0 + (float64(y)+1-segY0)*dxdy)
				} else {
					xb = x1 - 1 // inclusive last pixel (Splash.cc:2010-2012)
				}
				if xa == xb {
					s.drawPixel(xa, y, c, alpha)
				} else {
					s.drawSpan(xb+1, xa, y, c, alpha)
				}
				xa = xb
			}
		}
	}
	return nil
}

// strokeWide builds a fillable stroke outline and fills it (Splash.cc:2034-2042).
// P2-Dev4 surgical edit (2026-04-27): wires the outline into Phase 2's fillImpl.
//
// Splash.cc swaps the fill/stroke pattern+alpha pair before fillWithPattern (the
// stroke variant uses state->strokePattern/strokeAlpha). We reproduce that by
// stashing+restoring the fill side around the call, then drive non-zero (eo=false)
// fill semantics per Splash.cc:2042.
func (s *Splash) strokeWide(p *xpath.Path, w float64, dashedStroke bool) error {
	outline := s.makeStrokePath(p, w, false, dashedStroke)
	if outline == nil || outline.IsEmpty() {
		return nil
	}
	debugTraceStrokeOutline(s.debugStrokeIndex, outline)
	savedPat := s.state.fillPattern
	savedAlpha := s.state.fillAlpha
	s.state.fillPattern = s.state.strokePattern
	s.state.fillAlpha = s.state.strokeAlpha
	err := s.fillImpl(outline, false)
	s.state.fillPattern = savedPat
	s.state.fillAlpha = savedAlpha
	return err
}

func debugTraceStrokeOutline(index int, p *xpath.Path) {
	mode := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_STROKE_OUTLINE_TRACE"))
	if mode == "" || p == nil {
		return
	}
	if mode != "all" {
		want, err := strconv.Atoi(mode)
		if err != nil || want != index {
			return
		}
	}
	fmt.Fprintf(os.Stderr, "SPLASH_STROKE_OUTLINE index=%d pathLen=%d hints=%d\n", index, p.Length(), len(p.Hints()))
	for i := 0; i < p.Length(); i++ {
		pt, flag := p.Point(i)
		fmt.Fprintf(os.Stderr, "  out[%03d]=(%.8f,%.8f) flag=0x%02x\n", i, pt.X, pt.Y, flag)
	}
	for i, h := range p.Hints() {
		fmt.Fprintf(os.Stderr, "  hint[%03d]=ctrl(%d,%d) pts(%d,%d)\n", i, h.Ctrl0, h.Ctrl1, h.FirstPt, h.LastPt)
	}
}

// makeStrokePath builds the polygonal outline of a stroked path
// (Splash.cc:5842-6218). Phase 1 implements butt/projecting caps + miter/bevel joins
// + closed subpaths. Round caps and round joins are linearly approximated (single
// chord across the half-disc) and a TODO is left for Phase 2/3 polish.
//
// The flatten parameter is honoured only as a no-op forwarding flag here: the
// caller (strokeImpl) is responsible for flattening + dashing before invocation.
// This matches the strokeWide call site at Splash.cc:2038 which passes flatten=false.
func (s *Splash) makeStrokePath(p *xpath.Path, w float64, flatten bool, dashedStroke bool) *xpath.Path {
	_ = flatten
	out := xpath.NewPath()
	if p == nil || p.IsEmpty() {
		return out
	}

	n := p.Length()
	halfW := 0.5 * w
	mirrorNormals := s.shouldMirrorStrokeNormalsForPath(p, w, dashedStroke)

	// Walk subpaths. For Phase 1 we collapse repeated identical points (Splash.cc:5880).
	i0 := 0
	for i0 < n {
		// Find run end (last flag) for this subpath, skipping degenerate repeats.
		i1 := i0
		for {
			_, fi := p.Point(i1)
			if fi&pathFlagLast != 0 || i1+1 >= n {
				break
			}
			pi, _ := p.Point(i1)
			pn, _ := p.Point(i1 + 1)
			if pn.X != pi.X || pn.Y != pi.Y {
				break
			}
			i1++
		}

		_, f0 := p.Point(i0)
		if _, fi := p.Point(i1); fi&pathFlagLast != 0 {
			if f0&pathFlagFirst != 0 && s.state.lineCap == int(LineCapRound) {
				p0, _ := p.Point(i0)
				addZeroLengthRoundCapCircle(out, p0.X, p0.Y, w)
			}
			i0 = i1 + 1
			continue
		}
		closed := f0&pathFlagClosed != 0

		// Find end of subpath (last with pathFlagLast).
		subEnd := i1
		for k := i1; k < n; k++ {
			_, fk := p.Point(k)
			if fk&pathFlagLast != 0 {
				subEnd = k
				break
			}
		}

		// Single-point subpath: only round caps emit anything — Phase 1 stubs to no-op.
		if i0 == subEnd {
			i0 = subEnd + 1
			continue
		}

		// Build per-segment quads + joins.
		// Iterate segments [a, b] where b = a+1 within the subpath.
		first := true
		seg := 0
		left0, left1, right0, right1, join0, join1 := 0, 0, 0, 0, 0, 0
		leftFirst, rightFirst, firstPt := 0, 0, 0
		for a := i0; a < subEnd; a++ {
			b := a + 1
			pa, _ := p.Point(a)
			pb, _ := p.Point(b)
			dx, dy, ok := unitVec(pa.X, pa.Y, pb.X, pb.Y)
			if !ok {
				continue
			}
			wdx := halfW * dx
			wdy := halfW * dy
			if mirrorNormals {
				wdx = -wdx
				wdy = -wdy
			}

			isLast := b == subEnd

			// Start cap (only on the first segment of an open subpath).
			startCap := first && !closed
			endCap := isLast && !closed

			// Emit one quad: leftStart, leftEnd, rightEnd, rightStart, close.
			lsX, lsY := pa.X-wdy, pa.Y+wdx
			leX, leY := pb.X-wdy, pb.Y+wdx
			reX, reY := pb.X+wdy, pb.Y-wdx
			rsX, rsY := pa.X+wdy, pa.Y-wdx

			_ = out.MoveTo(lsX, lsY)
			if first {
				firstPt = out.Length() - 1
			}
			if startCap {
				switch s.state.lineCap {
				case int(LineCapButt):
					_ = out.LineTo(rsX, rsY)
				case int(LineCapProjecting):
					_ = out.LineTo(pa.X-wdx-wdy, pa.Y+wdx-wdy)
					_ = out.LineTo(pa.X-wdx+wdy, pa.Y-wdx-wdy)
					_ = out.LineTo(rsX, rsY)
				case int(LineCapRound):
					// Splash.cc:5996-6000 emits a two-cubic semicircle for round caps.
					_ = out.CurveTo(
						pa.X-wdy-bezierCircle*wdx, pa.Y+wdx-bezierCircle*wdy,
						pa.X-wdx-bezierCircle*wdy, pa.Y-wdy+bezierCircle*wdx,
						pa.X-wdx, pa.Y-wdy,
					)
					_ = out.CurveTo(
						pa.X-wdx+bezierCircle*wdy, pa.Y-wdy-bezierCircle*wdx,
						pa.X+wdy-bezierCircle*wdx, pa.Y-wdx-bezierCircle*wdy,
						rsX, rsY,
					)
				}
			} else {
				_ = out.LineTo(rsX, rsY)
			}

			left2 := out.Length() - 1
			_ = out.LineTo(reX, reY)

			if endCap {
				switch s.state.lineCap {
				case int(LineCapButt):
					_ = out.LineTo(leX, leY)
				case int(LineCapProjecting):
					_ = out.LineTo(pb.X+wdy+wdx, pb.Y-wdx+wdy)
					_ = out.LineTo(pb.X-wdy+wdx, pb.Y+wdx+wdy)
					_ = out.LineTo(leX, leY)
				case int(LineCapRound):
					// Splash.cc:6022-6026 mirrors the start-cap semicircle at the end point.
					_ = out.CurveTo(
						pb.X+wdy+bezierCircle*wdx, pb.Y-wdx+bezierCircle*wdy,
						pb.X+wdx+bezierCircle*wdy, pb.Y+wdy-bezierCircle*wdx,
						pb.X+wdx, pb.Y+wdy,
					)
					_ = out.CurveTo(
						pb.X+wdx-bezierCircle*wdy, pb.Y+wdy+bezierCircle*wdx,
						pb.X-wdy+bezierCircle*wdx, pb.Y+wdx+bezierCircle*wdy,
						leX, leY,
					)
				}
			} else {
				_ = out.LineTo(leX, leY)
			}

			right2 := out.Length() - 1
			_ = out.Close(s.state.strokeAdjust)

			// Join at pb if not the final endpoint of an open subpath.
			join2 := out.Length()
			if !isLast || closed {
				// Find next segment endpoint.
				ja := b
				jb := b + 1
				if isLast && closed {
					jb = i0 + 1
				}
				if jb > subEnd && !closed {
					break
				}
				pja, _ := p.Point(ja)
				var pjb xpath.PathPoint
				if jb <= subEnd {
					pjb, _ = p.Point(jb)
				} else {
					// closed: wrap to subpath start+1 to get next direction
					pjb, _ = p.Point(i0 + 1)
				}
				dxn, dyn, okn := unitVec(pja.X, pja.Y, pjb.X, pjb.Y)
				if okn {
					s.emitJoin(out, pb.X, pb.Y, dx, dy, dxn, dyn, halfW, mirrorNormals)
				}
			}

			if s.state.strokeAdjust {
				// Mirrors Splash.cc:6146-6203. These cross-segment hint ranges
				// are what keep closed rectangular joins from producing stray AA
				// corner coverage without changing the global ±0.01 snap window.
				if seg == 0 && !closed {
					if s.state.lineCap == int(LineCapButt) {
						out.AddStrokeAdjustHint(firstPt, left2+1, firstPt, firstPt+1)
						if isLast {
							out.AddStrokeAdjustHint(firstPt, left2+1, left2+1, left2+2)
						}
					} else if s.state.lineCap == int(LineCapProjecting) {
						if isLast {
							out.AddStrokeAdjustHint(firstPt+1, left2+2, firstPt+1, firstPt+2)
							out.AddStrokeAdjustHint(firstPt+1, left2+2, left2+2, left2+3)
						} else {
							out.AddStrokeAdjustHint(firstPt+1, left2+1, firstPt+1, firstPt+2)
						}
					}
				}
				if seg >= 1 {
					if seg >= 2 {
						out.AddStrokeAdjustHint(left1, right1, left0+1, right0)
						out.AddStrokeAdjustHint(left1, right1, join0, left2)
					} else {
						out.AddStrokeAdjustHint(left1, right1, firstPt, left2)
					}
					out.AddStrokeAdjustHint(left1, right1, right2+1, right2+1)
				}
				left0 = left1
				left1 = left2
				right0 = right1
				right1 = right2
				join0 = join1
				join1 = join2
				if seg == 0 {
					leftFirst = left2
					rightFirst = right2
				}
				if isLast {
					if seg >= 2 {
						out.AddStrokeAdjustHint(left1, right1, left0+1, right0)
						out.AddStrokeAdjustHint(left1, right1, join0, out.Length()-1)
					} else {
						out.AddStrokeAdjustHint(left1, right1, firstPt, out.Length()-1)
					}
					if closed {
						out.AddStrokeAdjustHint(left1, right1, firstPt, leftFirst)
						out.AddStrokeAdjustHint(left1, right1, rightFirst+1, rightFirst+1)
						out.AddStrokeAdjustHint(leftFirst, rightFirst, left1+1, right1)
						out.AddStrokeAdjustHint(leftFirst, rightFirst, join1, out.Length()-1)
					}
					if !closed && seg > 0 {
						if s.state.lineCap == int(LineCapButt) {
							out.AddStrokeAdjustHint(left1-1, left1+1, left1+1, left1+2)
						} else if s.state.lineCap == int(LineCapProjecting) {
							out.AddStrokeAdjustHint(left1-1, left1+2, left1+2, left1+3)
						}
					}
				}
			}

			first = false
			seg++
		}

		i0 = subEnd + 1
	}

	return out
}

func addZeroLengthRoundCapCircle(out *xpath.Path, x, y, w float64) {
	if out == nil {
		return
	}
	// Poppler draws a zero-length subpath with round line caps as a four-cubic
	// circle (Splash.cc:6021-6033).
	r := 0.5 * w
	k := bezierCircle2 * w
	_ = out.MoveTo(x+r, y)
	_ = out.CurveTo(x+r, y+k, x+k, y+r, x, y+r)
	_ = out.CurveTo(x-k, y+r, x-r, y+k, x-r, y)
	_ = out.CurveTo(x-r, y-k, x-k, y-r, x, y-r)
	_ = out.CurveTo(x+k, y-r, x+r, y-k, x+r, y)
	_ = out.Close(false)
}

func (s *Splash) shouldMirrorStrokeNormalsForPath(p *xpath.Path, w float64, dashedStroke bool) bool {
	if !s.mirrorStrokeNormals || s.state.lineCap != int(LineCapButt) {
		return false
	}
	if dashedStroke {
		if os.Getenv("PDF_DEBUG_SPLASH_DISABLE_DASHED_BUTT_STROKE_MIRROR") == "1" {
			return false
		}
		return shouldMirrorDashedButtStrokeNormalsForPath(p, w)
	}
	if shouldMirrorSingleButtStrokeNormalsForPath(p, w) {
		return true
	}
	return false
}

func shouldMirrorSingleButtStrokeNormalsForPath(p *xpath.Path, w float64) bool {
	if p == nil || p.Length() < 2 {
		return false
	}
	if p.Length() == 2 {
		p0, f0 := p.Point(0)
		p1, f1 := p.Point(1)
		return shouldMirrorAxisButtStrokeNormals(p0, f0, p1, f1, w)
	}
	if !isOpenCollinearAxisSubpath(p) {
		return false
	}
	p0, f0 := p.Point(0)
	p1, f1 := p.Point(p.Length() - 1)
	return shouldMirrorAxisButtStrokeNormals(p0, f0, p1, f1, w)
}

func isOpenCollinearAxisSubpath(p *xpath.Path) bool {
	n := p.Length()
	if n < 3 {
		return false
	}
	p0, f0 := p.Point(0)
	pn, fn := p.Point(n - 1)
	if f0&pathFlagFirst == 0 ||
		fn&pathFlagLast == 0 ||
		f0&pathFlagClosed != 0 ||
		fn&pathFlagClosed != 0 {
		return false
	}

	const axisEpsilon = 1e-9
	sameX := math.Abs(pn.X-p0.X) <= axisEpsilon
	sameY := math.Abs(pn.Y-p0.Y) <= axisEpsilon
	if !sameX && !sameY {
		return false
	}
	for i := 1; i < n-1; i++ {
		pt, flag := p.Point(i)
		if flag&(pathFlagFirst|pathFlagLast|pathFlagClosed) != 0 {
			return false
		}
		if sameX && math.Abs(pt.X-p0.X) > axisEpsilon {
			return false
		}
		if sameY && math.Abs(pt.Y-p0.Y) > axisEpsilon {
			return false
		}
	}
	return true
}

func shouldMirrorDashedButtStrokeNormalsForPath(p *xpath.Path, w float64) bool {
	if p == nil || p.Length() == 0 {
		return false
	}
	sawSegment := false
	for i := 0; i < p.Length(); i += 2 {
		if i+1 >= p.Length() {
			return false
		}
		p0, f0 := p.Point(i)
		p1, f1 := p.Point(i + 1)
		if !shouldMirrorAxisButtStrokeNormals(p0, f0, p1, f1, w) {
			return false
		}
		sawSegment = true
	}
	return sawSegment
}

func shouldMirrorAxisButtStrokeNormals(p0 xpath.PathPoint, f0 byte, p1 xpath.PathPoint, f1 byte, w float64) bool {
	if !(f0&pathFlagFirst != 0 &&
		f1&pathFlagLast != 0 &&
		f0&pathFlagClosed == 0 &&
		f1&pathFlagClosed == 0) {
		return false
	}

	halfW := 0.5 * w
	const (
		axisEpsilon           = 1e-9
		maxMirroredTickLength = 32.0
	)
	forceLongAxisMirror := os.Getenv("PDF_DEBUG_SPLASH_FORCE_MIRROR_STROKE_NORMALS") == "1"
	dx := math.Abs(p1.X - p0.X)
	dy := math.Abs(p1.Y - p0.Y)
	if dx <= axisEpsilon {
		// Long grid lines already match Poppler with the existing path ordering;
		// mirror only when the cap planes are not sitting on half-pixel rows.
		if dy > maxMirroredTickLength &&
			!forceLongAxisMirror &&
			(strokeCapPlaneAlreadyHalfPixel(p0.Y) || strokeCapPlaneAlreadyHalfPixel(p1.Y)) {
			return false
		}
		return !strokeEdgeAlreadyPixelAligned(p0.X-halfW) &&
			!strokeEdgeAlreadyPixelAligned(p0.X+halfW)
	}
	if dy <= axisEpsilon {
		if dx > maxMirroredTickLength &&
			!forceLongAxisMirror &&
			(strokeCapPlaneAlreadyHalfPixel(p0.X) || strokeCapPlaneAlreadyHalfPixel(p1.X)) {
			return false
		}
		return !strokeEdgeAlreadyPixelAligned(p0.Y-halfW) &&
			!strokeEdgeAlreadyPixelAligned(p0.Y+halfW)
	}
	return true
}

func strokeEdgeAlreadyPixelAligned(v float64) bool {
	return math.Abs(v-math.Round(v)) < 0.01
}

func strokeCapPlaneAlreadyHalfPixel(v float64) bool {
	return math.Abs(v-(math.Floor(v)+0.5)) < 0.01
}

// emitJoin emits a miter/bevel/round join polygon at vertex (vx, vy) where the
// incoming segment unit vector is (dx,dy) and outgoing is (dxn,dyn) (Splash.cc:5995-6147).
func (s *Splash) emitJoin(out *xpath.Path, vx, vy, dx, dy, dxn, dyn, halfW float64, mirrorNormals bool) {
	wdx := halfW * dx
	wdy := halfW * dy
	wdxN := halfW * dxn
	wdyN := halfW * dyn
	if mirrorNormals {
		wdx = -wdx
		wdy = -wdy
		wdxN = -wdxN
		wdyN = -wdyN
	}

	cross := dx*dyn - dy*dxn
	dot := -(dx*dxn + dy*dyn)
	hasAngle := cross != 0 || dx*dxn < 0 || dy*dyn < 0
	if !hasAngle {
		return
	}

	var miter, m float64
	if dot > 0.9999 {
		miter = (s.state.miterLimit + 1) * (s.state.miterLimit + 1)
		m = 0
	} else {
		miter = 2 / (1 - dot)
		if miter < 1 {
			miter = 1
		}
		m = math.Sqrt(miter - 1)
	}

	join := s.state.lineJoin
	miterOK := join == int(LineJoinMiter) && math.Sqrt(miter) <= s.state.miterLimit

	_ = out.MoveTo(vx, vy)
	if join == int(LineJoinRound) {
		w := 2 * halfW
		if cross < 0 {
			angle := math.Atan2(dx, -dy)
			angleNext := math.Atan2(dxn, -dyn)
			if angle < angleNext {
				angle += 2 * math.Pi
			}
			dAngle := (angle - angleNext) / math.Pi
			_ = out.LineTo(vx-wdyN, vy+wdxN)
			if dAngle < 0.501 {
				kappa := dAngle * bezierCircle * w
				_ = out.CurveTo(
					vx-wdyN-kappa*dxn, vy+wdxN-kappa*dyn,
					vx-wdy+kappa*dx, vy+wdx+kappa*dy,
					vx-wdy, vy+wdx,
				)
			} else {
				dJoin := math.Hypot(-wdy-(-wdyN), wdx-wdxN)
				if dJoin > 0 {
					dxJoin := (-wdyN + wdy) / dJoin
					dyJoin := (wdxN - wdx) / dJoin
					xc := vx + halfW*math.Cos(0.5*(angle+angleNext))
					yc := vy + halfW*math.Sin(0.5*(angle+angleNext))
					kappa := dAngle * bezierCircle2 * w
					_ = out.CurveTo(
						vx-wdyN-kappa*dxn, vy+wdxN-kappa*dyn,
						xc+kappa*dxJoin, yc+kappa*dyJoin,
						xc, yc,
					)
					_ = out.CurveTo(
						xc-kappa*dxJoin, yc-kappa*dyJoin,
						vx-wdy+kappa*dx, vy+wdx+kappa*dy,
						vx-wdy, vy+wdx,
					)
				}
			}
		} else {
			angle := math.Atan2(-dx, dy)
			angleNext := math.Atan2(-dxn, dyn)
			if angleNext < angle {
				angleNext += 2 * math.Pi
			}
			dAngle := (angleNext - angle) / math.Pi
			_ = out.LineTo(vx+wdy, vy-wdx)
			if dAngle < 0.501 {
				kappa := dAngle * bezierCircle * w
				_ = out.CurveTo(
					vx+wdy+kappa*dx, vy-wdx+kappa*dy,
					vx+wdyN-kappa*dxn, vy-wdxN-kappa*dyn,
					vx+wdyN, vy-wdxN,
				)
			} else {
				dJoin := math.Hypot(wdy-wdyN, -wdx-(-wdxN))
				if dJoin > 0 {
					dxJoin := (wdyN - wdy) / dJoin
					dyJoin := (-wdxN + wdx) / dJoin
					xc := vx + halfW*math.Cos(0.5*(angle+angleNext))
					yc := vy + halfW*math.Sin(0.5*(angle+angleNext))
					kappa := dAngle * bezierCircle2 * w
					_ = out.CurveTo(
						vx+wdy+kappa*dx, vy-wdx+kappa*dy,
						xc-kappa*dxJoin, yc-kappa*dyJoin,
						xc, yc,
					)
					_ = out.CurveTo(
						xc+kappa*dxJoin, yc+kappa*dyJoin,
						vx+wdyN-kappa*dxn, vy-wdxN-kappa*dyn,
						vx+wdyN, vy-wdxN,
					)
				}
			}
		}
		_ = out.Close(false)
		return
	}
	if cross < 0 {
		// Inner side on the left: vx-wdy, vy+wdx for both segments.
		_ = out.LineTo(vx-wdyN, vy+wdxN)
		if miterOK {
			_ = out.LineTo(vx-wdy+wdx*m, vy+wdx+wdy*m)
		}
		// LineJoinRound is approximated as bevel for Phase 1.
		// TODO Phase 2/3: bezier round join (Splash.cc:6031-6115).
		_ = out.LineTo(vx-wdy, vy+wdx)
	} else {
		_ = out.LineTo(vx+wdy, vy-wdx)
		if miterOK {
			_ = out.LineTo(vx+wdy+wdx*m, vy-wdx+wdy*m)
		}
		// LineJoinRound is approximated as bevel for Phase 1.
		_ = out.LineTo(vx+wdyN, vy-wdxN)
	}
	_ = out.Close(false)
}

// unitVec returns the normalised (dx, dy) from (x0,y0) to (x1,y1) and false if the
// distance is zero.
func unitVec(x0, y0, x1, y1 float64) (float64, float64, bool) {
	dx := x1 - x0
	dy := y1 - y0
	d := math.Hypot(dx, dy)
	if d == 0 {
		return 0, 0, false
	}
	inv := 1 / d
	return dx * inv, dy * inv, true
}

// drawSpan writes a horizontal run of solid colour pixels from x0..x1 (inclusive)
// on row y (Splash.cc:1976; SplashBitmap.cc:213-220 for ModeRGB8). Out-of-range
// pixels are silently dropped.
//
// Partial clips use per-pixel Clip.Test just like Splash::drawSpan
// (Splash.cc:1370-1395); otherwise tile replay strokes can overpaint a few
// pixels along path-clipped pattern cell edges.
func (s *Splash) drawSpan(x0, x1, y int, c Color, alpha byte) {
	if s.bitmap == nil || s.bitmap.data == nil {
		return
	}
	if y < 0 || y >= s.bitmap.height {
		return
	}
	if x0 < 0 {
		x0 = 0
	}
	if x1 >= s.bitmap.width {
		x1 = s.bitmap.width - 1
	}
	if x0 > x1 {
		return
	}
	var clip *xpath.Clip
	clipRes := xpath.ClipAllInside
	if clip, ok := s.state.clip.(*xpath.Clip); ok && clip != nil {
		clipRes = clip.TestSpan(x0, x1, y)
		if clipRes == xpath.ClipAllOutside {
			return
		}
	}
	inside := func(x int) bool {
		return clipRes != xpath.ClipPartial || clip == nil || clip.Test(x, y)
	}
	switch s.bitmap.mode {
	case ModeRGB8:
		stride := s.bitmap.rowSize
		off := y*stride + x0*3
		alphaOff := y*s.bitmap.width + x0
		for x := x0; x <= x1; x++ {
			if inside(x) {
				s.bitmap.data[off+0] = c[0]
				s.bitmap.data[off+1] = c[1]
				s.bitmap.data[off+2] = c[2]
				if s.bitmap.alpha != nil {
					s.bitmap.alpha[alphaOff] = alpha
				}
			}
			off += 3
			alphaOff++
		}
	case ModeMono8:
		stride := s.bitmap.rowSize
		off := y*stride + x0
		alphaOff := y*s.bitmap.width + x0
		for x := x0; x <= x1; x++ {
			if inside(x) {
				s.bitmap.data[off] = c[0]
				if s.bitmap.alpha != nil {
					s.bitmap.alpha[alphaOff] = alpha
				}
			}
			off++
			alphaOff++
		}
	default:
		// Other modes wired up in Phase 2.
	}
}

// drawPixel writes a single solid-colour pixel (Splash.cc:1999, 2014).
func (s *Splash) drawPixel(x, y int, c Color, alpha byte) {
	s.drawSpan(x, x, y, c, alpha)
}
