package xpath

import (
	"fmt"
	"math"
	"os"
	"sort"
)

// intersect mirrors SplashIntersect (SplashXPathScanner.h:39-44).
type intersect struct {
	Y      int
	X0, X1 int
	Count  int
}

// Scanner mirrors SplashXPathScanner (SplashXPathScanner.h:50-112).
type Scanner struct {
	xPath            *XPath
	eo               bool
	xMin, yMin       int
	xMax, yMax       int
	xMinFP, yMinFP   float64
	xMaxFP, yMaxFP   float64
	partialClip      bool
	allIntersections [][]intersect
}

// ScanIterator mirrors SplashXPathScanIterator (SplashXPathScanner.h:114-134).
type ScanIterator struct {
	scanner  *Scanner
	line     []intersect
	interIdx int
	interCnt int
	eo       bool
}

// NewScanner builds a scanner from a sorted XPath (SplashXPathScanner ctor, SplashXPathScanner.cc:42-113).
func NewScanner(x *XPath, eo bool, xMinA, yMinA, xMaxA, yMaxA int) *Scanner {
	s := &Scanner{
		xPath: x,
		eo:    eo,
	}
	// bbox sentinels (SplashXPathScanner.cc:52-53) — empty path yields yMin>yMax
	// so computeIntersections returns immediately at :234.
	s.xMin, s.yMin = 1, 1
	s.xMax, s.yMax = 0, 0

	if x != nil && len(x.Segs) > 0 {
		seg := &x.Segs[0]
		// NaN early-return (SplashXPathScanner.cc:56-58).
		if math.IsNaN(seg.X0) || math.IsNaN(seg.X1) || math.IsNaN(seg.Y0) || math.IsNaN(seg.Y1) {
			return s
		}
		var xMinFP, xMaxFP, yMinFP, yMaxFP float64
		if seg.X0 <= seg.X1 {
			xMinFP, xMaxFP = seg.X0, seg.X1
		} else {
			xMinFP, xMaxFP = seg.X1, seg.X0
		}
		if seg.Flags&XPathFlip != 0 {
			yMinFP, yMaxFP = seg.Y1, seg.Y0
		} else {
			yMinFP, yMaxFP = seg.Y0, seg.Y1
		}
		for i := 1; i < len(x.Segs); i++ {
			seg = &x.Segs[i]
			if math.IsNaN(seg.X0) || math.IsNaN(seg.X1) || math.IsNaN(seg.Y0) || math.IsNaN(seg.Y1) {
				return s
			}
			if seg.X0 < xMinFP {
				xMinFP = seg.X0
			} else if seg.X0 > xMaxFP {
				xMaxFP = seg.X0
			}
			if seg.X1 < xMinFP {
				xMinFP = seg.X1
			} else if seg.X1 > xMaxFP {
				xMaxFP = seg.X1
			}
			if seg.Flags&XPathFlip != 0 {
				if seg.Y0 > yMaxFP {
					yMaxFP = seg.Y0
				}
			} else {
				if seg.Y1 > yMaxFP {
					yMaxFP = seg.Y1
				}
			}
		}
		s.xMinFP, s.xMaxFP = xMinFP, xMaxFP
		s.yMinFP, s.yMaxFP = yMinFP, yMaxFP
		s.xMin = splashFloor(xMinFP)
		s.xMax = splashFloor(xMaxFP)
		s.yMin = splashFloor(yMinFP)
		s.yMax = splashFloor(yMaxFP)
		// clipYMin/clipYMax adjust (SplashXPathScanner.cc:102-109).
		if yMinA > s.yMin {
			s.yMin = yMinA
			s.partialClip = true
		}
		if yMaxA < s.yMax {
			s.yMax = yMaxA
			s.partialClip = true
		}
		_ = xMinA
		_ = xMaxA
	}
	s.computeIntersections()
	return s
}

// computeIntersections mirrors SplashXPathScanner::computeIntersections (SplashXPathScanner.cc:228-323).
func (s *Scanner) computeIntersections() {
	if s.yMin > s.yMax {
		return
	}
	if s.xPath == nil {
		return
	}
	s.allIntersections = make([][]intersect, s.yMax-s.yMin+1)

	for i := 0; i < len(s.xPath.Segs); i++ {
		seg := &s.xPath.Segs[i]
		var segYMin, segYMax float64
		if seg.Flags&XPathFlip != 0 {
			segYMin, segYMax = seg.Y1, seg.Y0
		} else {
			segYMin, segYMax = seg.Y0, seg.Y1
		}
		switch {
		case seg.Flags&XPathHoriz != 0:
			// horizontal — count=0 hard-coded (SplashXPathScanner.cc:250-256).
			y := splashFloor(seg.Y0)
			if y >= s.yMin && y <= s.yMax {
				s.addIntersection(segYMin, segYMax, y, splashFloor(seg.X0), splashFloor(seg.X1), 0)
			}
		case seg.Flags&XPathVert != 0:
			// vertical (SplashXPathScanner.cc:257-272).
			y0 := splashFloor(segYMin)
			if y0 < s.yMin {
				y0 = s.yMin
			}
			y1 := splashFloor(segYMax)
			if y1 > s.yMax {
				y1 = s.yMax
			}
			x := splashFloor(seg.X0)
			count := -1
			if s.eo || (seg.Flags&XPathFlip != 0) {
				count = 1
			}
			for y := y0; y <= y1; y++ {
				s.addIntersection(segYMin, segYMax, y, x, x, count)
			}
		default:
			// slope edge (SplashXPathScanner.cc:273-318).
			var segXMin, segXMax float64
			if seg.X0 < seg.X1 {
				segXMin, segXMax = seg.X0, seg.X1
			} else {
				segXMin, segXMax = seg.X1, seg.X0
			}
			y0 := splashFloor(segYMin)
			if y0 < s.yMin {
				y0 = s.yMin
			}
			y1 := splashFloor(segYMax)
			if y1 > s.yMax {
				y1 = s.yMax
			}
			count := -1
			if s.eo || (seg.Flags&XPathFlip != 0) {
				count = 1
			}
			// xbase = x0 - y0_seg * dxdy → x at y=0 (SplashXPathScanner.cc:292).
			xbase := seg.X0 - seg.Y0*seg.DXDY
			xx0 := xbase + float64(y0)*seg.DXDY
			if xx0 < segXMin {
				xx0 = segXMin
			} else if xx0 > segXMax {
				xx0 = segXMax
			}
			x0 := splashFloor(xx0)
			for y := y0; y <= y1; y++ {
				xx1 := xbase + float64(y+1)*seg.DXDY
				if xx1 < segXMin {
					xx1 = segXMin
				} else if xx1 > segXMax {
					xx1 = segXMax
				}
				x1 := splashFloor(xx1)
				s.addIntersection(segYMin, segYMax, y, x0, x1, count)
				xx0 = xx1
				x0 = x1
			}
		}
	}
	// per-row sort by x0 (SplashXPathScanner.cc:320-322).
	for i := range s.allIntersections {
		line := s.allIntersections[i]
		sort.SliceStable(line, func(a, b int) bool {
			return line[a].X0 < line[b].X0
		})
	}
}

// addIntersection mirrors SplashXPathScanner::addIntersection.
func (s *Scanner) addIntersection(segYMin, segYMax float64, y, x0, x1, count int) {
	var ent intersect
	ent.Y = y
	if x0 < x1 {
		ent.X0, ent.X1 = x0, x1
	} else {
		ent.X0, ent.X1 = x1, x0
	}
	if segYMin <= float64(y) && float64(y) < segYMax {
		ent.Count = count
	} else {
		ent.Count = 0
	}
	s.allIntersections[y-s.yMin] = append(s.allIntersections[y-s.yMin], ent)
}

// BBox returns the path bounding box in integer coords (SplashXPathScanner.h:62-68).
func (s *Scanner) BBox() (xMin, yMin, xMax, yMax int) {
	return s.xMin, s.yMin, s.xMax, s.yMax
}

// BBoxAA returns the AA-divided bounding box (SplashXPathScanner.cc:117-123).
func (s *Scanner) BBoxAA() (xMin, yMin, xMax, yMax int) {
	return s.xMin / aaSize, s.yMin / aaSize, s.xMax / aaSize, s.yMax / aaSize
}

// HasNextSpan reports whether row y has any intersection entries (precondition for NextSpan, derived from SplashXPathScanner.cc:196-217).
func (s *Scanner) HasNextSpan(y int) bool {
	if y < s.yMin || y > s.yMax {
		return false
	}
	if s.allIntersections == nil {
		return false
	}
	return len(s.allIntersections[y-s.yMin]) > 0
}

// Test reports whether point (x,y) is inside the path
// (SplashXPathScanner.cc:148-162). It is used by SplashClip::test for
// partial-clipped non-AA stroke pixels.
func (s *Scanner) Test(x, y int) bool {
	if y < s.yMin || y > s.yMax || s.allIntersections == nil {
		return false
	}
	line := s.allIntersections[y-s.yMin]
	count := 0
	for i := 0; i < len(line) && line[i].X0 <= x; i++ {
		if x <= line[i].X1 {
			return true
		}
		count += line[i].Count
	}
	if s.eo {
		return (count & 1) != 0
	}
	return count != 0
}

// TestSpan reports whether [x0..x1] at row y is fully inside the path
// (SplashXPathScanner.cc:164-195). It is used by SplashClip::testSpan to prove
// a clipped span is all-inside even when path scanners are present.
func (s *Scanner) TestSpan(x0, x1, y int) bool {
	if y < s.yMin || y > s.yMax || s.allIntersections == nil {
		return false
	}
	line := s.allIntersections[y-s.yMin]
	count := 0
	i := 0
	for i < len(line) && line[i].X1 < x0 {
		count += line[i].Count
		i++
	}

	xx1 := x0 - 1
	for xx1 < x1 {
		if i >= len(line) {
			return false
		}
		inside := false
		if s.eo {
			inside = (count & 1) != 0
		} else {
			inside = count != 0
		}
		if line[i].X0 > xx1+1 && !inside {
			return false
		}
		if line[i].X1 > xx1 {
			xx1 = line[i].X1
		}
		count += line[i].Count
		i++
	}
	return true
}

// NextSpan returns the next inside span at row y under the active winding rule (SplashXPathScanIterator::getNextSpan, SplashXPathScanner.cc:196-217).
//
// NOTE: this method is stateless and ALWAYS returns the *first* coalesced span
// of row y. To walk all spans on a row use Iterator(y).NextSpan in a loop.
// Returns ok=false when the row has no spans.
func (s *Scanner) NextSpan(y int) (x0, x1 int, ok bool) {
	if y < s.yMin || y > s.yMax {
		return 0, 0, false
	}
	if s.allIntersections == nil {
		return 0, 0, false
	}
	line := s.allIntersections[y-s.yMin]
	it := &ScanIterator{scanner: s, line: line, eo: s.eo}
	return it.NextSpan()
}

// Iterator returns a ScanIterator for row y (SplashXPathScanIterator ctor, SplashXPathScanner.cc:219-226).
func (s *Scanner) Iterator(y int) *ScanIterator {
	if y < s.yMin || y > s.yMax || s.allIntersections == nil {
		return &ScanIterator{scanner: s, line: nil, eo: s.eo}
	}
	return &ScanIterator{scanner: s, line: s.allIntersections[y-s.yMin], eo: s.eo}
}

// NextSpan walks coalesced spans under the winding rule (SplashXPathScanner.cc:196-217).
func (it *ScanIterator) NextSpan() (x0, x1 int, ok bool) {
	if it.interIdx >= len(it.line) {
		return 0, 0, false
	}
	xx0 := it.line[it.interIdx].X0
	xx1 := it.line[it.interIdx].X1
	it.interCnt += it.line[it.interIdx].Count
	it.interIdx++
	for it.interIdx < len(it.line) {
		next := &it.line[it.interIdx]
		inside := false
		if it.eo {
			inside = (it.interCnt & 1) != 0
		} else {
			inside = it.interCnt != 0
		}
		if next.X0 <= xx1 || inside {
			if next.X1 > xx1 {
				xx1 = next.X1
			}
			it.interCnt += next.Count
			it.interIdx++
			continue
		}
		break
	}
	return xx0, xx1, true
}

// RenderAALine rasterises one anti-aliased device-pixel scanline into aaBuf
// (SplashXPathScanner::renderAALine, SplashXPathScanner.cc:353-430).
//
// aaBuf holds aaSize=4 sub-rows of Mono1 (MSB-on-left) covering device columns
// [xMin, xMax]. Sub-row stride is rowSize = ((xMax-xMin+1)*aaSize + 7)>>3 bytes,
// total length 4*rowSize. Bit at sub-cell (sub-row yy, abs sub-cell xx) lives at
// aaBuf[yy*rowSize + (xx-xMin*aaSize)>>3], MSB = bit 0.
//
// The caller owns allocation; this method only writes (zeroing aaBuf at start
// per :360 invariant §9 #8).
func (s *Scanner) RenderAALine(y int, aaBuf []byte, xMin, xMax int) {
	width := (xMax - xMin + 1) * aaSize
	if width <= 0 {
		return
	}
	rowSize := (width + 7) >> 3
	// :360 — zero all 4 sub-rows before painting.
	totalBytes := rowSize * aaSize
	if totalBytes > len(aaBuf) {
		totalBytes = len(aaBuf)
	}
	for i := 0; i < totalBytes; i++ {
		aaBuf[i] = 0
	}
	if s.yMin > s.yMax || s.allIntersections == nil {
		return
	}
	// :364-372 — sub-row clamp against scanner's sub-pixel y bounds.
	yy := 0
	yyMax := aaSize - 1
	if s.yMin > aaSize*y {
		yy = s.yMin - aaSize*y
	}
	if yyMax+aaSize*y > s.yMax {
		yyMax = s.yMax - aaSize*y
	}
	subOriginBit := xMin * aaSize // absolute sub-cell at bit 0 of aaBuf row.
	for ; yy <= yyMax; yy++ {
		idx := aaSize*y + yy - s.yMin
		if idx < 0 || idx >= len(s.allIntersections) {
			continue
		}
		line := s.allIntersections[idx]
		interIdx, interCount := 0, 0
		for interIdx < len(line) {
			xx0 := line[interIdx].X0
			xx1 := line[interIdx].X1
			interCount += line[interIdx].Count
			interIdx++
			// :383 — coalesce overlap or "still inside" runs.
			for interIdx < len(line) {
				inside := false
				if s.eo {
					inside = (interCount & 1) != 0
				} else {
					inside = interCount != 0
				}
				if line[interIdx].X0 <= xx1 || inside {
					if line[interIdx].X1 > xx1 {
						xx1 = line[interIdx].X1
					}
					interCount += line[interIdx].Count
					interIdx++
					continue
				}
				break
			}
			// :393 — half-open: span is [xx0, xx1+1) sub-cells.
			xx1++
			// :390-396 — clamp to aaBuf width [0, width) in local coords.
			a := xx0 - subOriginBit
			b := xx1 - subOriginBit
			if a < 0 {
				a = 0
			}
			if b > width {
				b = width
			}
			if a >= b {
				continue
			}
			rowOff := yy * rowSize
			setBitsRange(aaBuf, rowOff, a, b)
		}
	}
}

// RenderAALineFullWidth mirrors Poppler's SplashXPathScanner::renderAALine
// buffer contract: aaBuf spans the full destination bitmap width, and the
// returned x0/x1 are the row-local device-pixel range actually touched.
func (s *Scanner) RenderAALineFullWidth(y int, aaBuf []byte, bitmapWidth int) (x0, x1 int) {
	width := bitmapWidth * aaSize
	if width <= 0 {
		return 0, 0
	}
	rowSize := (width + 7) >> 3
	totalBytes := rowSize * aaSize
	if totalBytes > len(aaBuf) {
		totalBytes = len(aaBuf)
	}
	for i := 0; i < totalBytes; i++ {
		aaBuf[i] = 0
	}

	xxMin := width
	xxMax := -1
	traceX, trace := scannerRenderTraceTarget(y, bitmapWidth)
	traceCell := traceX * aaSize
	if trace {
		fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_RENDER_TRACE phase=start x=%d y=%d cell=%d width=%d scannerY=(%d,%d) eo=%t\n",
			traceX, y, traceCell, width, s.yMin, s.yMax, s.eo)
	}
	if s.yMin <= s.yMax && s.allIntersections != nil {
		yy := 0
		yyMax := aaSize - 1
		if s.yMin > aaSize*y {
			yy = s.yMin - aaSize*y
		}
		if yyMax+aaSize*y > s.yMax {
			yyMax = s.yMax - aaSize*y
		}
		for ; yy <= yyMax; yy++ {
			idx := aaSize*y + yy - s.yMin
			if idx < 0 || idx >= len(s.allIntersections) {
				continue
			}
			line := s.allIntersections[idx]
			if trace {
				fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_RENDER_TRACE yy=%d lineSize=%d intersectionIndex=%d\n", yy, len(line), idx)
				for traceIdx, ent := range line {
					if ent.X1 >= traceCell-aaSize && ent.X0 <= traceCell+aaSize*2 {
						fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_RENDER_TRACE yy=%d entry=%d x0=%d x1=%d count=%d\n",
							yy, traceIdx, ent.X0, ent.X1, ent.Count)
					}
				}
			}
			interIdx, interCount := 0, 0
			for interIdx < len(line) {
				xx0 := line[interIdx].X0
				xx1 := line[interIdx].X1
				interCount += line[interIdx].Count
				interIdx++
				for interIdx < len(line) {
					inside := false
					if s.eo {
						inside = (interCount & 1) != 0
					} else {
						inside = interCount != 0
					}
					if line[interIdx].X0 <= xx1 || inside {
						if line[interIdx].X1 > xx1 {
							xx1 = line[interIdx].X1
						}
						interCount += line[interIdx].Count
						interIdx++
						continue
					}
					break
				}
				if xx0 < 0 {
					xx0 = 0
				}
				xx1++
				if xx1 > width {
					xx1 = width
				}
				if trace && xx1 > traceCell && xx0 < traceCell+aaSize {
					fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_RENDER_TRACE yy=%d span=[%d,%d) coversTarget=true\n", yy, xx0, xx1)
				}
				if xx0 < xx1 {
					setBitsRange(aaBuf, yy*rowSize, xx0, xx1)
					if xx0 < xxMin {
						xxMin = xx0
					}
					if xx1 > xxMax {
						xxMax = xx1
					}
				}
			}
		}
	}
	if xxMin > xxMax {
		xxMin = xxMax
	}
	if trace {
		fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_RENDER_TRACE phase=end x=%d y=%d shape=%d x0=%d x1=%d xxMin=%d xxMax=%d\n",
			traceX, y, countFullWidthAABufPixel(aaBuf, rowSize, traceX), xxMin/aaSize, (xxMax-1)/aaSize, xxMin, xxMax)
	}
	return xxMin / aaSize, (xxMax - 1) / aaSize
}

func scannerRenderTraceTarget(y, bitmapWidth int) (int, bool) {
	if os.Getenv("PDF_DEBUG_SPLASH_SCANNER_RENDER_TRACE") == "" {
		return 0, false
	}
	for _, target := range parseClipAABufTraceTargets() {
		if target.y == y && target.x >= 0 && target.x < bitmapWidth {
			return target.x, true
		}
	}
	return 0, false
}

func countFullWidthAABufPixel(aaBuf []byte, rowSize, x int) int {
	cell := x * aaSize
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

// ClipAALine ANDs the scanner's per-row coverage into existing aaBuf bits
// (SplashXPathScanner::clipAALine, SplashXPathScanner.cc:432-519). Bits in the
// gaps between spans (and beyond the last span) are cleared; bits inside spans
// pass through unchanged. Implements clip-mask intersection at sub-pixel res.
//
// xMin/xMax bound the clip's device-pixel column range corresponding to aaBuf;
// the underlying spans are walked in absolute sub-cell coordinates and only
// the gap bits are zeroed (no SET — the existing bits remain the caller's
// responsibility, matching the C++ memset-the-gaps approach).
func (s *Scanner) ClipAALine(y int, aaBuf []byte, xMin, xMax int) {
	width := (xMax - xMin + 1) * aaSize
	if width <= 0 {
		return
	}
	rowSize := (width + 7) >> 3
	subOriginBit := xMin * aaSize
	traceX, trace := scannerClipTraceTarget(y, xMin, xMax)
	// :439-447 — sub-row clamp: rows OUTSIDE [yyMin..yyMax] get fully zeroed.
	yyMin, yyMax := 0, aaSize-1
	if s.yMin > s.yMax || s.allIntersections == nil {
		// Whole row outside path → all sub-rows fall in the "outside" branch
		// at :496-517 → zero everything.
		for yy := 0; yy < aaSize; yy++ {
			rowOff := yy * rowSize
			clearBitsRange(aaBuf, rowOff, 0, width)
		}
		return
	}
	if s.yMin > aaSize*y {
		yyMin = s.yMin - aaSize*y
	}
	if yyMax+aaSize*y > s.yMax {
		yyMax = s.yMax - aaSize*y
	}
	for yy := 0; yy < aaSize; yy++ {
		rowOff := yy * rowSize
		// :449 — start cursor at xMin*aaSize in absolute coords (= 0 local).
		xx := 0 // local sub-cell cursor.
		if trace {
			fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_CLIP_TRACE yy=%d phase=start x=%d y=%d shape=%d xx=%d x0=%d x1=%d yyMin=%d yyMax=%d scannerY=(%d,%d) eo=%t\n",
				yy, traceX, y, countClipAABufPixel(aaBuf, rowSize, xMin, traceX), xx, xMin, xMax, yyMin, yyMax, s.yMin, s.yMax, s.eo)
		}
		if yy >= yyMin && yy <= yyMax {
			idx := aaSize*y + yy - s.yMin
			if idx >= 0 && idx < len(s.allIntersections) {
				line := s.allIntersections[idx]
				if trace {
					fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_CLIP_TRACE yy=%d lineSize=%d intersectionIndex=%d\n", yy, len(line), idx)
					for traceIdx, ent := range line {
						fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_CLIP_TRACE yy=%d entry=%d x0=%d x1=%d count=%d\n",
							yy, traceIdx, ent.X0, ent.X1, ent.Count)
					}
				}
				interIdx, interCount := 0, 0
				for interIdx < len(line) && xx < width {
					xx0 := line[interIdx].X0
					xx1 := line[interIdx].X1
					interCount += line[interIdx].Count
					interIdx++
					for interIdx < len(line) {
						inside := false
						if s.eo {
							inside = (interCount & 1) != 0
						} else {
							inside = interCount != 0
						}
						if line[interIdx].X0 <= xx1 || inside {
							if line[interIdx].X1 > xx1 {
								xx1 = line[interIdx].X1
							}
							interCount += line[interIdx].Count
							interIdx++
							continue
						}
						break
					}
					// :470-490 — clear [xx, xx0) (the gap before this span).
					a := xx0 - subOriginBit
					if a > width {
						a = width
					}
					if xx < a {
						clearBitsRange(aaBuf, rowOff, xx, a)
					}
					// Advance xx past span [xx0, xx1] → next gap starts at xx1+1.
					b := xx1 + 1 - subOriginBit
					if b >= xx {
						xx = b
					}
				}
			}
		}
		// :496-517 — zero the trailing gap [xx, width) (or the entire row when
		// yy is outside [yyMin, yyMax]).
		if xx < 0 {
			xx = 0
		}
		if xx < width {
			clearBitsRange(aaBuf, rowOff, xx, width)
		}
		if trace {
			fmt.Fprintf(os.Stderr, "SPLASH_SCANNER_CLIP_TRACE yy=%d phase=end x=%d y=%d shape=%d finalXX=%d\n",
				yy, traceX, y, countClipAABufPixel(aaBuf, rowSize, xMin, traceX), xx)
		}
	}
}

func scannerClipTraceTarget(y, xMin, xMax int) (int, bool) {
	if os.Getenv("PDF_DEBUG_SPLASH_SCANNER_CLIP_TRACE") == "" {
		return 0, false
	}
	for _, target := range parseClipAABufTraceTargets() {
		if target.y == y && target.x >= xMin && target.x <= xMax {
			return target.x, true
		}
	}
	return 0, false
}

// ClipAALineFullWidth mirrors Poppler's full-width AA buffer clipping path.
func (s *Scanner) ClipAALineFullWidth(y int, aaBuf []byte, x0, x1, bitmapWidth int) {
	width := bitmapWidth * aaSize
	if width <= 0 {
		return
	}
	rowSize := (width + 7) >> 3
	yyMin, yyMax := 0, aaSize-1
	if s.yMin > s.yMax || s.allIntersections == nil {
		for yy := 0; yy < aaSize; yy++ {
			clearBitsRange(aaBuf, yy*rowSize, x0*aaSize, (x1+1)*aaSize)
		}
		return
	}
	if s.yMin > aaSize*y {
		yyMin = s.yMin - aaSize*y
	}
	if yyMax+aaSize*y > s.yMax {
		yyMax = s.yMax - aaSize*y
	}
	for yy := 0; yy < aaSize; yy++ {
		rowOff := yy * rowSize
		xx := x0 * aaSize
		limit := (x1 + 1) * aaSize
		if limit > width {
			limit = width
		}
		if yy >= yyMin && yy <= yyMax {
			idx := aaSize*y + yy - s.yMin
			if idx >= 0 && idx < len(s.allIntersections) {
				line := s.allIntersections[idx]
				interIdx, interCount := 0, 0
				for interIdx < len(line) && xx < limit {
					xx0 := line[interIdx].X0
					xx1 := line[interIdx].X1
					interCount += line[interIdx].Count
					interIdx++
					for interIdx < len(line) {
						inside := false
						if s.eo {
							inside = (interCount & 1) != 0
						} else {
							inside = interCount != 0
						}
						if line[interIdx].X0 <= xx1 || inside {
							if line[interIdx].X1 > xx1 {
								xx1 = line[interIdx].X1
							}
							interCount += line[interIdx].Count
							interIdx++
							continue
						}
						break
					}
					if xx0 > width {
						xx0 = width
					}
					if xx < xx0 {
						// Poppler uses a slightly different partial-byte mask for
						// the gap before a covered span than for the trailing gap
						// below (SplashXPathScanner.cc:470-489).
						clearBitsRangePopplerClipGap(aaBuf, rowOff, xx, xx0)
					}
					if xx1 >= xx {
						xx = xx1 + 1
					}
				}
			}
		}
		if xx < 0 {
			xx = 0
		}
		if xx < limit {
			clearBitsRangePopplerClip(aaBuf, rowOff, xx, limit)
		}
	}
}

// setBitsRange OR-sets aaBuf bits in [a, b) (sub-cell indices local to the
// row at byte offset rowOff). Mono1 MSB-on-left, mirroring SplashXPathScanner.cc:399-414.
func setBitsRange(aaBuf []byte, rowOff, a, b int) {
	if a >= b {
		return
	}
	byteStart := rowOff + (a >> 3)
	byteEnd := rowOff + (b >> 3)
	startBit := a & 7
	endBit := b & 7
	if byteStart >= len(aaBuf) {
		return
	}
	if byteStart == byteEnd {
		// All within one byte: bits [startBit, endBit).
		mask := byte((0xff >> startBit) & ^(0xff >> endBit))
		aaBuf[byteStart] |= mask
		return
	}
	// Leading partial byte.
	if startBit != 0 {
		aaBuf[byteStart] |= byte(0xff >> startBit)
		byteStart++
	}
	// Full bytes.
	for byteStart < byteEnd && byteStart < len(aaBuf) {
		aaBuf[byteStart] = 0xff
		byteStart++
	}
	// Trailing partial byte.
	if endBit != 0 && byteEnd < len(aaBuf) {
		aaBuf[byteEnd] |= byte(^(0xff >> endBit) & 0xff)
	}
}

// clearBitsRange AND-clears aaBuf bits in [a, b) (Mono1 MSB-on-left). Mirrors
// the gap-zeroing memset pattern in SplashXPathScanner.cc:475-489 / :502-516.
func clearBitsRange(aaBuf []byte, rowOff, a, b int) {
	if a >= b {
		return
	}
	byteStart := rowOff + (a >> 3)
	byteEnd := rowOff + (b >> 3)
	startBit := a & 7
	endBit := b & 7
	if byteStart >= len(aaBuf) {
		return
	}
	if byteStart == byteEnd {
		// All within one byte: clear bits [startBit, endBit).
		mask := byte(^((0xff >> startBit) & ^(0xff >> endBit)) & 0xff)
		aaBuf[byteStart] &= mask
		return
	}
	if startBit != 0 {
		aaBuf[byteStart] &= byte(^(0xff >> startBit) & 0xff)
		byteStart++
	}
	for byteStart < byteEnd && byteStart < len(aaBuf) {
		aaBuf[byteStart] = 0x00
		byteStart++
	}
	if endBit != 0 && byteEnd < len(aaBuf) {
		aaBuf[byteEnd] &= byte(0xff >> endBit)
	}
}

func clearBitsRangePopplerClip(aaBuf []byte, rowOff, a, b int) {
	if a >= b {
		return
	}
	if a < 0 {
		a = 0
	}
	byteIdx := rowOff + (a >> 3)
	if byteIdx >= len(aaBuf) {
		return
	}
	xx := a
	if xx&7 != 0 {
		mask := byte((int(0xff00) >> (xx & 7)) & 0xff)
		if (xx &^ 7) == (b &^ 7) {
			mask &= byte(0xff >> (b & 7))
		}
		aaBuf[byteIdx] &= mask
		xx = (xx &^ 7) + 8
		byteIdx++
	}
	for xx+7 < b && byteIdx < len(aaBuf) {
		aaBuf[byteIdx] = 0x00
		xx += 8
		byteIdx++
	}
	if xx < b && byteIdx < len(aaBuf) {
		aaBuf[byteIdx] &= byte(0xff >> (b & 7))
	}
}

func clearBitsRangePopplerClipGap(aaBuf []byte, rowOff, a, b int) {
	if a >= b {
		return
	}
	if a < 0 {
		a = 0
	}
	byteIdx := rowOff + (a >> 3)
	if byteIdx >= len(aaBuf) {
		return
	}
	xx := a
	if xx&7 != 0 {
		mask := byte((int(0xff00) >> (xx & 7)) & 0xff)
		if (xx &^ 7) == (b &^ 7) {
			mask |= byte(0xff >> (b & 7))
		}
		aaBuf[byteIdx] &= mask
		xx = (xx &^ 7) + 8
		byteIdx++
	}
	for xx+7 < b && byteIdx < len(aaBuf) {
		aaBuf[byteIdx] = 0x00
		xx += 8
		byteIdx++
	}
	if xx < b && byteIdx < len(aaBuf) {
		aaBuf[byteIdx] &= byte(0xff >> (b & 7))
	}
}
