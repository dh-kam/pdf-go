package splash

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// shadedFill mirrors Poppler's Splash::shadedFill driver for shading patterns.
func (s *Splash) shadedFill(p *xpath.Path, hasBBox bool, pat Pattern, clipToStrokePath bool) error {
	if p == nil || p.IsEmpty() {
		return ErrEmptyPath
	}

	xPath := xpath.NewXPath(p, s.state.matrix, s.state.flatness, true)
	if s.vectorAA {
		xPath.AAScale()
	}
	xPath.Sort()

	clip := s.ensureClip()
	clipXMinI, clipYMinI, clipXMaxI, clipYMaxI := clip.IntBounds()
	yMinScan, yMaxScan := clipYMinI, clipYMaxI
	if s.vectorAA && !s.inShading {
		yMinScan = clipYMinI * splashAASize
		yMaxScan = (clipYMaxI+1)*splashAASize - 1
	}
	scanner := xpath.NewScanner(xPath, false, clipXMinI, yMinScan, clipXMaxI, yMaxScan)

	var xMinI, yMinI, xMaxI, yMaxI int
	if s.vectorAA {
		xMinI, yMinI, xMaxI, yMaxI = scanner.BBoxAA()
	} else {
		xMinI, yMinI, xMaxI, yMaxI = scanner.BBox()
	}
	if yMinI > yMaxI || xMinI > xMaxI {
		return nil
	}

	clipRes := clip.TestRect(xMinI, yMinI, xMaxI, yMaxI)
	if clipRes == xpath.ClipAllOutside {
		return nil
	}
	if yMinI < clipYMinI {
		yMinI = clipYMinI
	}
	if yMaxI > clipYMaxI {
		yMaxI = clipYMaxI
	}
	if yMinI > yMaxI {
		return nil
	}

	alpha := s.state.fillAlpha
	if clipToStrokePath {
		alpha = s.state.strokeAlpha
	}
	var pipe pipe
	s.pipeInit(&pipe, 0, yMinI, pat, nil, byte(Round(alpha*255)), s.vectorAA && !hasBBox, false)
	if s.vectorAA {
		s.shadedFillAARows(&pipe, scanner, clip, clipRes, pat, hasBBox, yMinI, yMaxI)
		return nil
	}
	s.fillNoAARows(&pipe, scanner, clip, xMinI, yMinI, xMaxI, yMaxI)
	return nil
}

func (s *Splash) shadedFillAARows(pipe *pipe, scanner *xpath.Scanner, clip *xpath.Clip, clipRes xpath.ClipResult, pat Pattern, hasBBox bool, yMinI, yMaxI int) {
	if s.bitmap == nil || s.bitmap.width <= 0 || s.aaBuf == nil {
		return
	}
	width := s.bitmap.width * splashAASize
	rowSize := (width + 7) >> 3
	if rowSize*splashAASize > len(s.aaBuf) {
		return
	}
	for y := yMinI; y <= yMaxI; y++ {
		x0, x1 := scanner.RenderAALineFullWidth(y, s.aaBuf, s.bitmap.width)
		clipHasPath := false
		if clipRes != xpath.ClipAllInside {
			x0, x1 = clip.ClipAALineFullWidth(y, s.aaBuf, x0, x1, s.bitmap.width)
			clipHasPath = clip != nil && clip.HasPathClip()
		}
		traceShadedFillAARow("before-edge-correction", x0, x1, y, clipHasPath, hasBBox)
		s.correctShadedFillAALineEdges(pat, hasBBox, clipHasPath, x0, x1, y, yMinI, yMaxI, rowSize)
		s.runAALineFullWidth(pipe, x0, x1, y, rowSize)
	}
}

func (s *Splash) correctShadedFillAALineEdges(pat Pattern, hasBBox, clipHasPath bool, x0, x1, y, yMinI, yMaxI, rowSize int) {
	if disableShadedFillAALineEdgeCorrection() || hasBBox || pat == nil || y <= yMinI || y >= yMaxI {
		return
	}
	if clipHasPath && skipShadedFillPathClipEdgeCorrection() {
		return
	}
	if skipper, ok := pat.(interface{ SkipShadedFillEdgeCorrection() bool }); ok && skipper.SkipShadedFillEdgeCorrection() {
		return
	}
	s.correctShadedFillAALineEdge(pat, x0, y, rowSize, true)
	s.correctShadedFillAALineEdge(pat, x1, y, rowSize, false)
}

func disableShadedFillAALineEdgeCorrection() bool {
	return os.Getenv("PDF_DEBUG_SPLASH_DISABLE_SHADED_FILL_EDGE_CORRECTION") == "1"
}

func skipShadedFillPathClipEdgeCorrection() bool {
	return os.Getenv("PDF_SPLASH_SHADED_FILL_SKIP_PATH_CLIP_EDGE_CORRECTION") == "1"
}

func (s *Splash) correctShadedFillAALineEdge(pat Pattern, x, y, rowSize int, left bool) {
	if s.bitmap == nil || x < 0 || x >= s.bitmap.width || rowSize <= 0 {
		return
	}
	trace := shouldTraceShadedFillEdge(x, y)
	testX := x + 1
	if left {
		testX = x - 1
	}
	inside := pat.TestPosition(testX, y)
	if trace {
		fmt.Fprintf(os.Stderr, "SPLASH_SHADED_EDGE_TRACE phase=test x=%d y=%d left=%t testX=%d inside=%t rowSize=%d\n",
			x, y, left, testX, inside, rowSize)
	}
	if left {
		if !inside {
			return
		}
	} else if !inside {
		return
	}

	byteIdx := x >> 1
	lastOff := byteIdx + 3*rowSize
	if byteIdx < 0 || lastOff >= len(s.aaBuf) {
		return
	}

	n0 := shadingFillNibble(s.aaBuf[byteIdx], x)
	n1 := shadingFillNibble(s.aaBuf[byteIdx+rowSize], x)
	n2 := shadingFillNibble(s.aaBuf[byteIdx+2*rowSize], x)
	n3 := shadingFillNibble(s.aaBuf[byteIdx+3*rowSize], x)
	if trace {
		fmt.Fprintf(os.Stderr, "SPLASH_SHADED_EDGE_TRACE phase=nibbles x=%d y=%d left=%t byteIdx=%d n=(%02x,%02x,%02x,%02x)\n",
			x, y, left, byteIdx, n0, n1, n2, n3)
	}
	if n0 != n1 || n1 != n2 || n2 != n3 {
		return
	}
	if left {
		if n0&0x03 != 0x03 {
			if trace {
				fmt.Fprintf(os.Stderr, "SPLASH_SHADED_EDGE_TRACE phase=skip-left-mask x=%d y=%d n=%02x\n", x, y, n0)
			}
			return
		}
	} else if n0&0x0c != 0x0c {
		if trace {
			fmt.Fprintf(os.Stderr, "SPLASH_SHADED_EDGE_TRACE phase=skip-right-mask x=%d y=%d n=%02x\n", x, y, n0)
		}
		return
	}

	mask := byte(0xf0)
	if x&1 != 0 {
		mask = 0x0f
	}
	s.aaBuf[byteIdx] |= mask
	s.aaBuf[byteIdx+rowSize] |= mask
	s.aaBuf[byteIdx+2*rowSize] |= mask
	s.aaBuf[byteIdx+3*rowSize] |= mask
	if trace {
		fmt.Fprintf(os.Stderr, "SPLASH_SHADED_EDGE_TRACE phase=apply x=%d y=%d left=%t mask=%02x\n", x, y, left, mask)
	}
}

func shadingFillNibble(b byte, x int) byte {
	if x&1 != 0 {
		return b & 0x0f
	}
	return b >> 4
}

var shadedEdgeTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_SHADED_EDGE_TRACE"))

func shouldTraceShadedFillEdge(x, y int) bool {
	for _, pixel := range shadedEdgeTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func traceShadedFillAARow(phase string, x0, x1, y int, clipHasPath, hasBBox bool) {
	for _, pixel := range shadedEdgeTracePixels {
		if pixel.y != y || (pixel.x != x0 && pixel.x != x1) {
			continue
		}
		fmt.Fprintf(os.Stderr, "SPLASH_SHADED_EDGE_TRACE phase=%s x=%d y=%d x0=%d x1=%d clipHasPath=%t hasBBox=%t\n",
			phase, pixel.x, y, x0, x1, clipHasPath, hasBBox)
	}
}
