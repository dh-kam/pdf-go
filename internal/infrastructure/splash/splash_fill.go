package splash

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// bitCount4 mirrors the static nibble popcount LUT at Splash.cc:1381.
// Used by the AA loop to count set sub-cells in each output column.
var bitCount4 = [16]int{0, 1, 1, 2, 1, 2, 2, 3, 1, 2, 2, 3, 2, 3, 3, 4}

// fillImpl is the body of Splash.Fill — Splash.cc:2282-2289 dispatcher.
//
// Forwards to fillWithPattern using the current fill pattern + alpha.
func (s *Splash) fillImpl(p *xpath.Path, eo bool) error {
	if p == nil || p.IsEmpty() {
		return ErrEmptyPath
	}
	return s.fillWithPattern(p, eo, s.state.fillPattern, s.state.fillAlpha)
}

// fillWithPattern mirrors Splash::fillWithPattern (Splash.cc:2324-2453).
//
// Phase 2 scope: vectorAA path only. inShading branch (Splash.cc:2373) is a
// Phase 3 TODO. thinLineMode adjustments (Splash.cc:2356-2370, 2419-2426) are
// not ported — thinLineMode is always splashThinLineDefault for our backend
// per the SP3 scope, so adjustVertLine/lineShape collapse to constants.
func (s *Splash) fillWithPattern(p *xpath.Path, eo bool, pat Pattern, alpha float64) error {
	if p == nil || p.IsEmpty() {
		return ErrEmptyPath
	}
	if s.inShading {
		// Splash.cc:2373 inShading branch runs at device resolution with a
		// different output path. Phase 3 — flagged TODO until shading lands.
		return errNotImplemented
	}

	// 1. Stroke-adjust hint injection for filled rectangles (Splash.cc:2343-2354).
	// Applies only to single-subpath paths with no existing hints.
	s.maybeInjectFillRectHints(p)

	// 2. Build XPath. closeSubpaths=true matches Splash.cc:2372.
	xPath := xpath.AcquireXPath()
	defer xpath.ReleaseXPath(xPath)
	xPath.Reset(p, s.state.matrix, s.state.flatness, true)
	if s.vectorAA {
		xPath.AAScale() // Splash.cc:2374
	}
	s.applyTinyGroupedFillYMinBoundaryTie(xPath)
	s.applyTextGlyphPathIntegerSlopeBias(xPath)
	xPath.Sort() // Splash.cc:2376
	s.debugTraceSplashXPath(xPath)

	// 3. Resolve clip-space yMin/yMax for the scanner (Splash.cc:2377-2382).
	clip, _ := s.state.clip.(*xpath.Clip)
	var clipXMinI, clipYMinI, clipXMaxI, clipYMaxI int
	if clip != nil {
		clipXMinI, clipYMinI, clipXMaxI, clipYMaxI = clip.IntBounds()
		if clipXMinI > clipXMaxI || clipYMinI > clipYMaxI {
			return nil
		}
	} else {
		clipXMinI, clipYMinI = 0, 0
		clipXMaxI, clipYMaxI = s.bitmap.width-1, s.bitmap.height-1
	}
	yMinScan := clipYMinI
	yMaxScan := clipYMaxI
	if s.vectorAA {
		yMinScan = clipYMinI * splashAASize
		yMaxScan = (clipYMaxI+1)*splashAASize - 1
	}

	// 4. Build Scanner (Splash.cc:2383).
	scanner := xpath.AcquireScanner(xPath, eo, clipXMinI, yMinScan, clipXMaxI, yMaxScan)
	defer xpath.ReleaseScanner(scanner)

	// 5. Pull device-pixel bbox of the path under the active scale (Splash.cc:2386-2390).
	var xMinI, yMinI, xMaxI, yMaxI int
	if s.vectorAA {
		xMinI, yMinI, xMaxI, yMaxI = scanner.BBoxAA()
	} else {
		xMinI, yMinI, xMaxI, yMaxI = scanner.BBox()
	}
	if os.Getenv("PDF_DEBUG_TYPE3_TEMPBITMAP_DUMP") != "" && s.bitmap != nil && s.bitmap.width < 64 {
		nSeg := 0
		if xPath != nil {
			nSeg = len(xPath.Segs)
		}
		fmt.Fprintf(os.Stderr, "T3FILLIMPL bbox=(%d,%d,%d,%d) clip=(%d,%d,%d,%d) bitmap=%dx%d nSeg=%d aaBuf=%d\n",
			xMinI, yMinI, xMaxI, yMaxI, clipXMinI, clipYMinI, clipXMaxI, clipYMaxI, s.bitmap.width, s.bitmap.height, nSeg, len(s.aaBuf))
	}
	// Empty bbox (scanner saw no segs) → nothing to paint, but C++ still returns Ok.
	if yMinI > yMaxI || xMinI > xMaxI {
		return nil
	}

	// 6. xMinI == xMaxI zero-width gate (Splash.cc:2392-2399). Default thinLine mode
	// disables this gate in the C++; we skip to match (gate only fires under
	// thinLineSolid/thinLineShape, neither of which we expose).
	_ = xMinI
	_ = xMaxI

	// 7. Clamp the bbox to the bitmap (Splash.cc clipRes implicit via testRect).
	if xMinI < 0 {
		xMinI = 0
	}
	if yMinI < 0 {
		yMinI = 0
	}
	if xMaxI >= s.bitmap.width {
		xMaxI = s.bitmap.width - 1
	}
	if yMaxI >= s.bitmap.height {
		yMaxI = s.bitmap.height - 1
	}
	if clip != nil {
		if xMinI < clipXMinI {
			xMinI = clipXMinI
		}
		if yMinI < clipYMinI {
			yMinI = clipYMinI
		}
		if xMaxI > clipXMaxI {
			xMaxI = clipXMaxI
		}
		if yMaxI > clipYMaxI {
			yMaxI = clipYMaxI
		}
	}
	if xMinI > xMaxI || yMinI > yMaxI {
		return nil
	}

	// 8. Initialise the pipe (Splash.cc:2408).
	aInput := byte(Round(alpha * 255))
	pipeState := imagePipePool.Get().(*pipe)
	s.pipeInit(pipeState, 0, yMinI, pat, nil, aInput, s.vectorAA, false)
	defer func() {
		*pipeState = pipe{}
		imagePipePool.Put(pipeState)
	}()

	// 9. AA loop (Splash.cc:2411-2428) — render+clip+popcount+gamma+pipe per row.
	if s.vectorAA {
		s.fillAARows(pipeState, scanner, clip, xMinI, yMinI, xMaxI, yMaxI)
	} else {
		s.fillNoAARows(pipeState, scanner, clip, xMinI, yMinI, xMaxI, yMaxI)
	}

	return nil
}

const textGlyphIntegerSlopeBiasEpsilon = 1e-12

func (s *Splash) applyTextGlyphPathIntegerSlopeBias(xPath *xpath.XPath) {
	if s == nil || !s.textGlyphPathIntegerSlopeBias || xPath == nil {
		return
	}
	if debugSplashDisableTextGlyphIntegerSlopeBias {
		return
	}
	for i := range xPath.Segs {
		seg := &xPath.Segs[i]
		if seg.Flags&(xpath.XPathHoriz|xpath.XPathVert) != 0 || seg.DXDY == 0 {
			continue
		}
		rounded := math.Round(seg.DXDY)
		if math.Abs(seg.DXDY-rounded) > 1e-15 {
			continue
		}
		if seg.DXDY > 0 {
			seg.DXDY -= textGlyphIntegerSlopeBiasEpsilon
		} else {
			seg.DXDY += textGlyphIntegerSlopeBiasEpsilon
		}
		seg.DYDX = 1 / seg.DXDY
	}
}

func (s *Splash) debugTraceSplashXPath(x *xpath.XPath) {
	if debugSplashXpathTrace == "" || x == nil {
		return
	}
	if raw := strings.TrimSpace(debugSplashXpathTraceStroke); raw != "" {
		want, err := strconv.Atoi(raw)
		if err != nil || s == nil || s.debugStrokeIndex != want {
			return
		}
	}
	if raw := strings.TrimSpace(debugSplashXpathTraceFill); raw != "" {
		want, err := strconv.Atoi(raw)
		if err != nil || s == nil || s.debugFillIndex != want {
			return
		}
	}
	strokeIndex, fillIndex := -1, -1
	if s != nil {
		strokeIndex = s.debugStrokeIndex
		fillIndex = s.debugFillIndex
	}
	fmt.Fprintf(os.Stderr, "SPLASH_XPATH_TRACE segs=%d strokeIndex=%d fillIndex=%d\n", x.Length(), strokeIndex, fillIndex)
	for i, seg := range x.Segs {
		fmt.Fprintf(os.Stderr, "  seg[%03d]=(%.8f,%.8f)->(%.8f,%.8f) flags=0x%02x\n",
			i, seg.X0, seg.Y0, seg.X1, seg.Y1, seg.Flags)
	}
}

func (s *Splash) applyTinyGroupedFillYMinBoundaryTie(xPath *xpath.XPath) {
	if xPath == nil || debugSplashDisableTinyGroupYMinTie {
		return
	}
	if s == nil || len(s.groupStack) == 0 {
		return
	}
	const eps = 1e-9
	if !tinyGroupedFillYMinBoundaryTieCandidate(xPath, eps) {
		return
	}
	yMin := xPath.Segs[0].Y0
	for i := range xPath.Segs {
		seg := &xPath.Segs[i]
		if seg.Y0 < yMin {
			yMin = seg.Y0
		}
		if seg.Y1 < yMin {
			yMin = seg.Y1
		}
	}
	nearest := math.Round(yMin)
	if !(yMin > nearest && yMin-nearest <= eps) {
		return
	}
	// Poppler lands these tiny grouped fills just below the AA row boundary.
	// Adjust only the lower boundary points instead of biasing the whole path.
	nudged := math.Nextafter(nearest, math.Inf(-1))
	for i := range xPath.Segs {
		seg := &xPath.Segs[i]
		if math.Abs(seg.Y0-yMin) <= eps {
			seg.Y0 = nudged
		}
		if math.Abs(seg.Y1-yMin) <= eps {
			seg.Y1 = nudged
		}
		recomputeXPathSegDerived(seg)
	}
}

func recomputeXPathSegDerived(seg *xpath.XPathSeg) {
	seg.Flags = 0
	seg.DXDY = 0
	seg.DYDX = 0
	switch {
	case seg.Y1 == seg.Y0:
		seg.Flags |= xpath.XPathHoriz
		if seg.X1 == seg.X0 {
			seg.Flags |= xpath.XPathVert
		}
	case seg.X1 == seg.X0:
		seg.Flags |= xpath.XPathVert
	default:
		seg.DXDY = (seg.X1 - seg.X0) / (seg.Y1 - seg.Y0)
		seg.DYDX = 1 / seg.DXDY
	}
	if seg.Y0 > seg.Y1 {
		seg.Flags |= xpath.XPathFlip
	}
}

// fillAARows runs the antialiased per-row AA pipeline (Splash.cc:2411-2428).
func (s *Splash) fillAARows(pipe *pipe, scanner *xpath.Scanner, clip *xpath.Clip, xMinI, yMinI, xMaxI, yMaxI int) {
	if s.aaBuf == nil {
		return
	}
	if !debugSplashDisableFullWidthAabuf {
		s.fillAARowsFullWidth(pipe, scanner, clip, yMinI, yMaxI)
		return
	}
	width := (xMaxI - xMinI + 1) * splashAASize
	if width <= 0 {
		return
	}
	rowSize := (width + 7) >> 3

	for y := yMinI; y <= yMaxI; y++ {
		// 9a. RenderAALine zeros aaBuf and rasterises the path's coverage row.
		scanner.RenderAALine(y, s.aaBuf, xMinI, xMaxI)
		s.traceAABufPixels("preclip", xMinI, xMaxI, y, rowSize)
		// 9b. ClipAALine for each clip scanner (Splash.cc:2414-2416).
		if clip != nil {
			s.clipAALineThroughClip(clip, y, xMinI, xMaxI)
		}
		s.traceAABufPixels("postclip", xMinI, xMaxI, y, rowSize)
		// 9c. Popcount + gamma + pipe per output column (Splash.cc:1397-1426).
		s.runAALine(pipe, xMinI, xMaxI, y, rowSize)
	}
}

// fillAARowsFullWidth uses Poppler's full-destination AA bitmap contract
// (Splash.cc:2411-2428 with SplashXPathScanner.cc:353-519). The local buffer
// path remains available only as a debug fallback.
func (s *Splash) fillAARowsFullWidth(pipe *pipe, scanner *xpath.Scanner, clip *xpath.Clip, yMinI, yMaxI int) {
	if s.bitmap == nil || s.bitmap.width <= 0 {
		return
	}
	width := s.bitmap.width * splashAASize
	rowSize := (width + 7) >> 3
	if rowSize*splashAASize > len(s.aaBuf) {
		return
	}
	for y := yMinI; y <= yMaxI; y++ {
		x0, x1 := scanner.RenderAALineFullWidth(y, s.aaBuf, s.bitmap.width)
		if os.Getenv("PDF_DEBUG_TYPE3_TEMPBITMAP_DUMP") != "" && s.bitmap.width < 64 && y == yMinI {
			asum := 0
			for _, b := range s.aaBuf {
				asum += int(b)
			}
			fmt.Fprintf(os.Stderr, "T3AA y=%d renderSpan=[%d,%d] aaBufSum=%d\n", y, x0, x1, asum)
		}
		if clip != nil {
			x0, x1 = clip.ClipAALineFullWidth(y, s.aaBuf, x0, x1, s.bitmap.width)
		}
		s.runAALineFullWidth(pipe, x0, x1, y, rowSize)
	}
}

// clipAALineThroughClip drives every clip scanner's ClipAALine in turn over the
// shared aaBuf (matches state->clip->clipAALine at Splash.cc:2415; SplashClip's
// clipAALine internally walks all path scanners — we surface that here by
// iterating the parallel scanners[] array exposed by xpath.Clip).
func (s *Splash) clipAALineThroughClip(clip *xpath.Clip, y, xMinI, xMaxI int) {
	clip.ClipAALine(y, s.aaBuf, xMinI, xMaxI)
}

// runAALine implements the C++ drawAALine inner loop (Splash.cc:1378-1427).
//
// For each output device-pixel column x in [xMinI, xMaxI], collect the four
// sub-cells across the four sub-rows of aaBuf, popcount via bitCount4 to a
// 0..16 count, look up aaGamma[count] for the 0..255 coverage, then either
// run the pipe (count > 0) or advance the pipe cursor (count == 0).
func (s *Splash) runAALine(p *pipe, xMinI, xMaxI, y, rowSize int) {
	s.pipeSetXY(p, xMinI, y)
	for x := xMinI; x <= xMaxI; x++ {
		// Sub-cell index within aaBuf row, MSB-on-left: local cell = (x-xMinI)*4.
		cell := (x - xMinI) * splashAASize
		t := 0
		for yy := 0; yy < splashAASize; yy++ {
			rowOff := yy * rowSize
			byteIdx := rowOff + (cell >> 3)
			if byteIdx >= len(s.aaBuf) {
				continue
			}
			b := s.aaBuf[byteIdx]
			if cell&7 == 0 {
				// Top nibble (bits 7..4): cells [cell..cell+3].
				t += bitCount4[(b>>4)&0x0f]
			} else {
				// Bottom nibble (bits 3..0): cells [cell..cell+3] when cell&7==4.
				t += bitCount4[b&0x0f]
			}
		}
		if t != 0 {
			p.shape = s.aaGamma[t]
			if shouldTraceAALinePixel(x, y) {
				traceAALinePixelBefore(p, x, y, t)
			}
			p.run(p)
			if shouldTraceAALinePixel(x, y) {
				traceAALinePixelAfter(p, x, y)
			}
		} else {
			s.pipeIncX(p)
		}
	}
}

func (s *Splash) runAALineFullWidth(p *pipe, xMinI, xMaxI, y, rowSize int) {
	s.pipeSetXY(p, xMinI, y)
	dbg := os.Getenv("PDF_DEBUG_TYPE3_TEMPBITMAP_DUMP") != "" && s.bitmap != nil && s.bitmap.width < 64
	for x := xMinI; x <= xMaxI; x++ {
		p.sampleXOff = 0
		t := s.aaLineCoverage(x, rowSize)
		if dbg {
			fmt.Fprintf(os.Stderr, "T3RUN x=%d t=%d pattern=%T shapeInit=%v\n", x, t, p.pattern, p.shape)
		}
		if t != 0 {
			p.shape = s.aaGamma[t]
			if debugSplashAxialRightEdgeSamplePrev() && isDebugAxialRightEdgeSamplePrevCoverage(t) && pipeCanUseAxialRightEdgeSamplePrev(p) {
				if _, ok := p.pattern.(*AxialShader); ok && s.aaLineCoverage(x+1, rowSize) == 0 {
					p.sampleXOff = -1
				}
			}
			if shouldTraceAALinePixel(x, y) {
				traceAALinePixelBefore(p, x, y, t)
			}
			p.run(p)
			if shouldTraceAALinePixel(x, y) {
				traceAALinePixelAfter(p, x, y)
			}
		} else {
			s.pipeIncX(p)
		}
	}
	p.sampleXOff = 0
}

func (s *Splash) aaLineCoverage(x, rowSize int) int {
	if x < 0 || rowSize <= 0 {
		return 0
	}
	return s.aaBufCoverageAt(x, rowSize)
}

func (s *Splash) aaBufCoverageAt(x, rowSize int) int {
	cell := x * splashAASize
	byteIdx := cell >> 3
	lastIdx := byteIdx + (splashAASize-1)*rowSize
	if lastIdx >= 0 && lastIdx < len(s.aaBuf) {
		if cell&7 == 0 {
			return bitCount4[(s.aaBuf[byteIdx]>>4)&0x0f] +
				bitCount4[(s.aaBuf[byteIdx+rowSize]>>4)&0x0f] +
				bitCount4[(s.aaBuf[byteIdx+rowSize*2]>>4)&0x0f] +
				bitCount4[(s.aaBuf[byteIdx+rowSize*3]>>4)&0x0f]
		}
		return bitCount4[s.aaBuf[byteIdx]&0x0f] +
			bitCount4[s.aaBuf[byteIdx+rowSize]&0x0f] +
			bitCount4[s.aaBuf[byteIdx+rowSize*2]&0x0f] +
			bitCount4[s.aaBuf[byteIdx+rowSize*3]&0x0f]
	}
	return s.aaBufCoverageAtSlow(cell, rowSize)
}

func (s *Splash) aaBufCoverageAtSlow(cell, rowSize int) int {
	t := 0
	for yy := 0; yy < splashAASize; yy++ {
		byteIdx := yy*rowSize + (cell >> 3)
		if byteIdx >= 0 && byteIdx < len(s.aaBuf) {
			b := s.aaBuf[byteIdx]
			if cell&7 == 0 {
				t += bitCount4[(b>>4)&0x0f]
			} else {
				t += bitCount4[b&0x0f]
			}
		}
	}
	return t
}

type aaLineTracePixel struct {
	x int
	y int
}

var aaLineTracePixels = parseAALineTracePixels()

func parseAALineTracePixels() []aaLineTracePixel {
	return parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_AALINE_TRACE"))
}

func parseSplashTracePixels(raw string) []aaLineTracePixel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	pixels := make([]aaLineTracePixel, 0, len(parts))
	for _, part := range parts {
		xy := strings.Split(part, ",")
		if len(xy) != 2 {
			continue
		}
		x, errX := strconv.Atoi(strings.TrimSpace(xy[0]))
		y, errY := strconv.Atoi(strings.TrimSpace(xy[1]))
		if errX != nil || errY != nil {
			continue
		}
		pixels = append(pixels, aaLineTracePixel{x: x, y: y})
	}
	return pixels
}

func shouldTraceAALinePixel(x, y int) bool {
	for _, pixel := range aaLineTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func (s *Splash) traceAABufPixels(phase string, xMinI, xMaxI, y, rowSize int) {
	if len(aaLineTracePixels) == 0 {
		return
	}
	for _, pixel := range aaLineTracePixels {
		if pixel.y != y || pixel.x < xMinI || pixel.x > xMaxI {
			continue
		}
		cell := (pixel.x - xMinI) * splashAASize
		t := 0
		for yy := 0; yy < splashAASize; yy++ {
			rowOff := yy * rowSize
			byteIdx := rowOff + (cell >> 3)
			if byteIdx >= len(s.aaBuf) {
				continue
			}
			b := s.aaBuf[byteIdx]
			if cell&7 == 0 {
				t += bitCount4[(b>>4)&0x0f]
			} else {
				t += bitCount4[b&0x0f]
			}
		}
		if t != 0 {
			fmt.Fprintf(os.Stderr, "SPLASH_AABUF_TRACE phase=%s x=%d y=%d t=%d shape=%d xMin=%d xMax=%d\n",
				phase, pixel.x, y, t, s.aaGamma[t], xMinI, xMaxI)
		}
	}
}

func traceAALinePixelBefore(p *pipe, x, y, subcellCount int) {
	if p.colorBytesPerPixel < 3 || p.destOff+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil && p.aDestOff < len(p.aDestRow) {
		aDest = p.aDestRow[p.aDestOff]
	}
	src := p.cSrc
	if p.pattern != nil {
		var patternSrc Color
		if p.pattern.GetColor(pipePatternX(p), y, &patternSrc) {
			src = patternSrc
		}
	}
	strokeIndex := -1
	fillIndex := -1
	if p.s != nil {
		strokeIndex = p.s.debugStrokeIndex
		fillIndex = p.s.debugFillIndex
	}
	fmt.Fprintf(os.Stderr, "SPLASH_AALINE_TRACE before x=%d y=%d t=%d shape=%d aInput=%d usesShape=%t src=(%d,%d,%d) dst=(%d,%d,%d) aDest=%d strokeIndex=%d fillIndex=%d\n",
		x, y, subcellCount, p.shape, p.aInput, p.usesShape,
		src[0], src[1], src[2],
		p.destRow[p.destOff], p.destRow[p.destOff+1], p.destRow[p.destOff+2], aDest,
		strokeIndex, fillIndex)
}

func debugSplashAxialRightEdgeSamplePrev() bool {
	return debugSplashAxialRightEdgeSamplePrevVal
}

func isDebugAxialRightEdgeSamplePrevCoverage(t int) bool {
	switch t {
	case 2, 3, 4, 5, 8:
		return true
	default:
		return false
	}
}

func pipeCanUseAxialRightEdgeSamplePrev(p *pipe) bool {
	if p == nil || p.s == nil {
		return false
	}
	if p.softMask != nil || p.blendFunc != nil || p.alpha0 != nil {
		return false
	}
	if len(p.s.groupStack) != 0 {
		return false
	}
	if p.aDestRow == nil {
		return true
	}
	return p.aDestOff >= 0 && p.aDestOff < len(p.aDestRow) && p.aDestRow[p.aDestOff] == 255
}

func tinyGroupedFillYMinBoundaryTieCandidate(xPath *xpath.XPath, eps float64) bool {
	if xPath == nil || xPath.Length() != 6 || eps <= 0 {
		return false
	}
	var xMin, xMax, yMin, yMax float64
	haveBounds := false
	horiz := 0
	for i := range xPath.Segs {
		seg := &xPath.Segs[i]
		if seg.Flags&xpath.XPathHoriz != 0 {
			horiz++
		}
		for _, pt := range [][2]float64{{seg.X0, seg.Y0}, {seg.X1, seg.Y1}} {
			if !haveBounds {
				xMin, xMax = pt[0], pt[0]
				yMin, yMax = pt[1], pt[1]
				haveBounds = true
				continue
			}
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
	if !haveBounds || horiz < 2 {
		return false
	}
	if xMax-xMin > 24 || yMax-yMin > 12 {
		return false
	}
	nearest := math.Round(yMin)
	return yMin > nearest && yMin-nearest <= eps
}

func traceAALinePixelAfter(p *pipe, x, y int) {
	off := p.destOff - p.colorBytesPerPixel
	if p.colorBytesPerPixel < 3 || off < 0 || off+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil {
		aOff := p.aDestOff - 1
		if aOff >= 0 && aOff < len(p.aDestRow) {
			aDest = p.aDestRow[aOff]
		}
	}
	fmt.Fprintf(os.Stderr, "SPLASH_AALINE_TRACE after x=%d y=%d dst=(%d,%d,%d) aDest=%d\n",
		x, y, p.destRow[off], p.destRow[off+1], p.destRow[off+2], aDest)
}

// fillNoAARows is the non-AA branch (Splash.cc:2429-2447). Iterates the
// scanner's iterator and emits drawSpan for each coalesced inside-span.
func (s *Splash) fillNoAARows(pipe *pipe, scanner *xpath.Scanner, clip *xpath.Clip, xMinI, yMinI, xMaxI, yMaxI int) {
	for y := yMinI; y <= yMaxI; y++ {
		it := scanner.Iterator(y)
		for {
			x0, x1, ok := it.NextSpan()
			if !ok {
				break
			}
			if x0 < xMinI {
				x0 = xMinI
			}
			if x1 > xMaxI {
				x1 = xMaxI
			}
			if x0 > x1 {
				continue
			}
			// Optional clip span test (Splash.cc:2443).
			if clip != nil {
				switch clip.TestSpan(x0, x1, y) {
				case xpath.ClipAllOutside:
					continue
				case xpath.ClipPartial:
					s.drawSpanPipeClipped(pipe, clip, x0, x1, y)
					continue
				}
			}
			s.drawSpanPipe(pipe, x0, x1, y)
		}
	}
}

// drawSpanPipe runs the pipe across [x0..x1] at row y for the non-AA branch
// (Splash.cc:1351-1376 noClip variant). Used by fillNoAARows.
func (s *Splash) drawSpanPipe(p *pipe, x0, x1, y int) {
	s.pipeSetXY(p, x0, y)
	for x := x0; x <= x1; x++ {
		p.run(p)
	}
}

// drawSpanPipeClipped mirrors Splash::drawSpan(noClip=false), testing every
// pixel in a partial clip span and advancing the pipe over clipped-out pixels.
func (s *Splash) drawSpanPipeClipped(p *pipe, clip *xpath.Clip, x0, x1, y int) {
	s.pipeSetXY(p, x0, y)
	for x := x0; x <= x1; x++ {
		if clip.Test(x, y) {
			p.run(p)
		} else {
			s.pipeIncX(p)
		}
	}
}

// maybeInjectFillRectHints implements the stroke-adjust filled-rectangle hint
// injection from Splash.cc:2343-2354. Mutates p in place.
//
// Conditions (all required):
//   - state.strokeAdjust is true
//   - p has no existing hints
//   - either:
//     (a) length == 4 AND first point is NOT closed AND points 1,2 are NOT last
//     → close(true) the path and add hints (0,2,0,4) and (1,3,0,4).
//     (b) length == 5 AND first point IS closed AND points 1,2,3 are NOT last
//     → just add hints (0,2,0,4) and (1,3,0,4).
//
// This snaps axis-aligned rectangle edges to integer columns/rows.
func (s *Splash) maybeInjectFillRectHints(p *xpath.Path) {
	if !s.state.strokeAdjust {
		return
	}
	if len(p.Hints()) > 0 {
		return
	}
	n := p.Length()
	if n == 4 {
		_, f0 := p.Point(0)
		_, f1 := p.Point(1)
		_, f2 := p.Point(2)
		if f0&pathFlagClosed == 0 && f1&pathFlagLast == 0 && f2&pathFlagLast == 0 {
			_ = p.Close(true)
			p.AddStrokeAdjustHint(0, 2, 0, 4)
			p.AddStrokeAdjustHint(1, 3, 0, 4)
		}
		return
	}
	if n == 5 {
		_, f0 := p.Point(0)
		_, f1 := p.Point(1)
		_, f2 := p.Point(2)
		_, f3 := p.Point(3)
		if f0&pathFlagClosed != 0 && f1&pathFlagLast == 0 && f2&pathFlagLast == 0 && f3&pathFlagLast == 0 {
			p.AddStrokeAdjustHint(0, 2, 0, 4)
			p.AddStrokeAdjustHint(1, 3, 0, 4)
		}
	}
}
