package splash

import "github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"

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
		if clipRes != xpath.ClipAllInside {
			clip.ClipAALineFullWidth(y, s.aaBuf, x0, x1, s.bitmap.width)
		}
		s.correctShadedFillAALineEdges(pat, hasBBox, x0, x1, y, yMinI, yMaxI, rowSize)
		s.runAALineFullWidth(pipe, x0, x1, y, rowSize)
	}
}

func (s *Splash) correctShadedFillAALineEdges(pat Pattern, hasBBox bool, x0, x1, y, yMinI, yMaxI, rowSize int) {
	if hasBBox || pat == nil || y <= yMinI || y >= yMaxI {
		return
	}
	s.correctShadedFillAALineEdge(pat, x0, y, rowSize, true)
	s.correctShadedFillAALineEdge(pat, x1, y, rowSize, false)
}

func (s *Splash) correctShadedFillAALineEdge(pat Pattern, x, y, rowSize int, left bool) {
	if s.bitmap == nil || x < 0 || x >= s.bitmap.width || rowSize <= 0 {
		return
	}
	if left {
		if !pat.TestPosition(x-1, y) {
			return
		}
	} else if !pat.TestPosition(x+1, y) {
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
	if n0 != n1 || n1 != n2 || n2 != n3 {
		return
	}
	if left {
		if n0&0x03 != 0x03 {
			return
		}
	} else if n0&0x0c != 0x0c {
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
}

func shadingFillNibble(b byte, x int) byte {
	if x&1 != 0 {
		return b & 0x0f
	}
	return b >> 4
}
