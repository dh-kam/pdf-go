package splash

import (
	"math"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// FillImageMaskImpl rasterises a 1-bit-style image mask (Splash.cc:2740).
func (s *Splash) FillImageMaskImpl(src ImageMaskSource, w, h int, mat [6]float64, glyphMode bool) error {
	if s.bitmap == nil || s.bitmap.data == nil {
		return ErrBadArg
	}
	if w == 0 && h == 0 {
		return ErrZeroImage
	}
	det := mat[0]*mat[3] - mat[1]*mat[2]
	if math.Abs(det) < 1e-6 {
		return ErrSingularMatrix
	}

	minorAxisZero := mat[1] == 0 && mat[2] == 0

	// scaling only (Splash.cc:2764).
	if mat[0] > 0 && minorAxisZero && mat[3] > 0 {
		x0 := imgCoordMungeLowerC(mat[4], glyphMode)
		y0 := imgCoordMungeLowerC(mat[5], glyphMode)
		x1 := imgCoordMungeUpperC(mat[0]+mat[4], glyphMode)
		y1 := imgCoordMungeUpperC(mat[3]+mat[5], glyphMode)
		if x0 == x1 {
			x1++
		}
		if y0 == y1 {
			y1++
		}
		clipRes := s.testRect(x0, y0, x1-1, y1-1)
		if clipRes == xpath.ClipAllOutside {
			return nil
		}
		dstW := x1 - x0
		dstH := y1 - y0
		scaled, err := s.scaleMask(src, w, h, dstW, dstH)
		if err != nil {
			return err
		}
		return s.blitMask(scaled, x0, y0, clipRes)
	}

	// scaling plus vertical flip (Splash.cc:2791).
	if mat[0] > 0 && minorAxisZero && mat[3] < 0 {
		x0 := imgCoordMungeLowerC(mat[4], glyphMode)
		y0 := imgCoordMungeLowerC(mat[3]+mat[5], glyphMode)
		x1 := imgCoordMungeUpperC(mat[0]+mat[4], glyphMode)
		y1 := imgCoordMungeUpperC(mat[5], glyphMode)
		if x0 == x1 {
			x1++
		}
		if y0 == y1 {
			y1++
		}
		clipRes := s.testRect(x0, y0, x1-1, y1-1)
		if clipRes == xpath.ClipAllOutside {
			return nil
		}
		dstW := x1 - x0
		dstH := y1 - y0
		scaled, err := s.scaleMask(src, w, h, dstW, dstH)
		if err != nil {
			return err
		}
		vertFlipBitmap(scaled, 1)
		return s.blitMask(scaled, x0, y0, clipRes)
	}

	return s.arbitraryTransformMask(src, w, h, mat, glyphMode)
}

// scaleMask dispatches to one of the 4 mask kernels (Splash.cc:3090).
func (s *Splash) scaleMask(src ImageMaskSource, srcW, srcH, dstW, dstH int) (*Bitmap, error) {
	dest := NewBitmap(dstW, dstH, ModeMono8, false)
	if dest.data == nil || srcW <= 0 || srcH <= 0 {
		return nil, ErrZeroImage
	}
	var err error
	if dstH < srcH {
		if dstW < srcW {
			err = s.scaleMaskYdownXdown(src, srcW, srcH, dstW, dstH, dest)
		} else {
			err = s.scaleMaskYdownXup(src, srcW, srcH, dstW, dstH, dest)
		}
	} else {
		if dstW < srcW {
			err = s.scaleMaskYupXdown(src, srcW, srcH, dstW, dstH, dest)
		} else {
			err = s.scaleMaskYupXup(src, srcW, srcH, dstW, dstH, dest)
		}
	}
	if err != nil {
		return nil, err
	}
	return dest, nil
}

// scaleMaskYdownXdown — both axes shrink (Splash.cc:3111).
func (s *Splash) scaleMaskYdownXdown(src ImageMaskSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	yp := srcH / dstH
	yq := srcH % dstH
	xp := srcW / dstW
	xq := srcW % dstW

	lineBuf := make([]byte, srcW)
	pixBuf := make([]uint32, srcW)

	yt := 0
	rowIdx := 0
	destOff := 0
	for y := 0; y < dstH; y++ {
		var yStep int
		yt += yq
		if yt >= dstH {
			yt -= dstH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		for j := range pixBuf {
			pixBuf[j] = 0
		}
		for i := 0; i < yStep; i++ {
			if err := src(rowIdx, lineBuf); err != nil {
				return err
			}
			rowIdx++
			for j := 0; j < srcW; j++ {
				pixBuf[j] += uint32(lineBuf[j])
			}
		}

		xt := 0
		d0 := uint32((255 << 23) / (yStep * xp))
		d1 := uint32((255 << 23) / (yStep * (xp + 1)))
		xx := 0
		for x := 0; x < dstW; x++ {
			var xStep int
			var d uint32
			xt += xq
			if xt >= dstW {
				xt -= dstW
				xStep = xp + 1
				d = d1
			} else {
				xStep = xp
				d = d0
			}
			var pix uint32
			for i := 0; i < xStep; i++ {
				pix += pixBuf[xx]
				xx++
			}
			pix = (pix * d) >> 23
			dest.data[destOff] = byte(pix)
			destOff++
		}
	}
	return nil
}

// scaleMaskYdownXup — Y shrinks, X grows (Splash.cc:3195).
func (s *Splash) scaleMaskYdownXup(src ImageMaskSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	yp := srcH / dstH
	yq := srcH % dstH
	xp := dstW / srcW
	xq := dstW % srcW

	lineBuf := make([]byte, srcW)
	pixBuf := make([]uint32, srcW)

	yt := 0
	rowIdx := 0
	destOff := 0
	for y := 0; y < dstH; y++ {
		var yStep int
		yt += yq
		if yt >= dstH {
			yt -= dstH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		for j := range pixBuf {
			pixBuf[j] = 0
		}
		for i := 0; i < yStep; i++ {
			if err := src(rowIdx, lineBuf); err != nil {
				return err
			}
			rowIdx++
			for j := 0; j < srcW; j++ {
				pixBuf[j] += uint32(lineBuf[j])
			}
		}

		xt := 0
		d := uint32((255 << 23) / yStep)
		for x := 0; x < srcW; x++ {
			var xStep int
			xt += xq
			if xt >= srcW {
				xt -= srcW
				xStep = xp + 1
			} else {
				xStep = xp
			}
			pix := (pixBuf[x] * d) >> 23
			for i := 0; i < xStep; i++ {
				dest.data[destOff] = byte(pix)
				destOff++
			}
		}
	}
	return nil
}

// scaleMaskYupXdown — Y grows, X shrinks (Splash.cc:3274).
func (s *Splash) scaleMaskYupXdown(src ImageMaskSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	yp := dstH / srcH
	yq := dstH % srcH
	xp := srcW / dstW
	xq := srcW % dstW

	lineBuf := make([]byte, srcW)
	yt := 0
	destRowBase := 0
	for y := 0; y < srcH; y++ {
		var yStep int
		yt += yq
		if yt >= srcH {
			yt -= srcH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		if err := src(y, lineBuf); err != nil {
			return err
		}

		xt := 0
		d0 := uint32((255 << 23) / xp)
		d1 := uint32((255 << 23) / (xp + 1))
		xx := 0
		for x := 0; x < dstW; x++ {
			var xStep int
			var d uint32
			xt += xq
			if xt >= dstW {
				xt -= dstW
				xStep = xp + 1
				d = d1
			} else {
				xStep = xp
				d = d0
			}
			var pix uint32
			for i := 0; i < xStep; i++ {
				pix += uint32(lineBuf[xx])
				xx++
			}
			pix = (pix * d) >> 23
			for i := 0; i < yStep; i++ {
				dest.data[destRowBase+i*dstW+x] = byte(pix)
			}
		}
		destRowBase += yStep * dstW
	}
	return nil
}

// scaleMaskYupXup — both grow (binary expand) (Splash.cc:3354).
func (s *Splash) scaleMaskYupXup(src ImageMaskSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	yp := dstH / srcH
	yq := dstH % srcH
	xp := dstW / srcW
	xq := dstW % srcW

	lineBuf := make([]byte, srcW)
	yt := 0
	destRowBase := 0
	for y := 0; y < srcH; y++ {
		var yStep int
		yt += yq
		if yt >= srcH {
			yt -= srcH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		if err := src(y, lineBuf); err != nil {
			return err
		}

		xt := 0
		xx := 0
		for x := 0; x < srcW; x++ {
			var xStep int
			xt += xq
			if xt >= srcW {
				xt -= srcW
				xStep = xp + 1
			} else {
				xStep = xp
			}
			var pix byte
			if lineBuf[x] != 0 {
				pix = 255
			}
			for i := 0; i < yStep; i++ {
				for j := 0; j < xStep; j++ {
					dest.data[destRowBase+i*dstW+xx+j] = pix
				}
			}
			xx += xStep
		}
		destRowBase += yStep * dstW
	}
	return nil
}

// blitMask blends a mono8 alpha mask onto the bitmap (Splash.cc:3435).
func (s *Splash) blitMask(scaled *Bitmap, xDest, yDest int, clipRes xpath.ClipResult) error {
	if scaled == nil || scaled.data == nil {
		return nil
	}
	if s.vectorAA && clipRes != xpath.ClipAllInside {
		return s.blitMaskClippedAA(scaled, xDest, yDest)
	}
	w := scaled.width
	h := scaled.height
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	var p pipe
	s.pipeInit(&p, xDest, yDest, s.state.fillPattern, nil, alphaIn, true, false)

	clip, _ := s.state.clip.(*xpath.Clip)
	off := 0
	for y := 0; y < h; y++ {
		dy := yDest + y
		if dy < 0 || dy >= s.bitmap.height {
			off += w
			continue
		}
		s.pipeSetXY(&p, xDest, dy)
		for x := 0; x < w; x++ {
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width {
				s.pipeIncX(&p)
				off++
				continue
			}
			shape := scaled.data[off]
			if shape != 0 && (clipRes == xpath.ClipAllInside || clip == nil || clip.Test(dx, dy)) {
				p.shape = shape
				if shouldTraceImagePixel(dx, dy) {
					var c Color
					if p.pattern != nil {
						_ = p.pattern.GetColor(dx, dy, &c)
					} else {
						c = p.cSrc
					}
					traceImagePixelBefore(&p, "blitMask", dx, dy, x, y, c, shape)
				}
				p.run(&p)
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelAfter(&p, "blitMask", dx, dy)
				}
			} else {
				s.pipeIncX(&p)
			}
			off++
		}
	}
	return nil
}

func (s *Splash) blitMaskClippedAA(scaled *Bitmap, xDest, yDest int) error {
	clip, _ := s.state.clip.(*xpath.Clip)
	if clip == nil {
		return nil
	}
	w := scaled.width
	h := scaled.height
	_, clipYMin, _, clipYMax := clip.IntBounds()
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	var p pipe
	s.pipeInit(&p, xDest, yDest, s.state.fillPattern, nil, alphaIn, true, false)

	rowSize := (s.bitmap.width*splashAASize + 7) >> 3
	aaLen := rowSize * splashAASize
	if len(s.aaBuf) < aaLen {
		s.aaBuf = make([]byte, aaLen)
	}

	off := 0
	for y := 0; y < h; y++ {
		dy := yDest + y
		if dy < clipYMin || dy > clipYMax || dy < 0 || dy >= s.bitmap.height {
			off += w
			continue
		}
		for i := 0; i < aaLen; i++ {
			s.aaBuf[i] = 0xff
		}
		clip.ClipAALine(dy, s.aaBuf, 0, s.bitmap.width-1)

		for x := 0; x < w; x++ {
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width {
				off++
				continue
			}
			shape := scaled.data[off]
			if shape != 0 {
				t := s.aaCoverageAt(dx, rowSize)
				if t != 0 {
					shape = byte(Div255(int(s.aaGamma[t]) * int(shape)))
					if shape != 0 {
						p.shape = shape
						s.pipeSetXY(&p, dx, dy)
						if shouldTraceImagePixel(dx, dy) {
							var c Color
							if p.pattern != nil {
								_ = p.pattern.GetColor(dx, dy, &c)
							} else {
								c = p.cSrc
							}
							traceImagePixelBefore(&p, "blitMaskClippedAA", dx, dy, x, y, c, shape)
						}
						p.run(&p)
						if shouldTraceImagePixel(dx, dy) {
							traceImagePixelAfter(&p, "blitMaskClippedAA", dx, dy)
						}
					}
				}
			}
			off++
		}
	}
	return nil
}

type imageMaskSection struct {
	y0, y1   int
	ia0, ia1 int
	ib0, ib1 int
	xa0, ya0 float64
	xa1, ya1 float64
	xb0, yb0 float64
	xb1, yb1 float64
	dxdya    float64
	dxdyb    float64
}

// arbitraryTransformMask rasterises a non-axis-aligned mask via inverse
// mapping (Splash.cc:2905).
func (s *Splash) arbitraryTransformMask(src ImageMaskSource, srcW, srcH int, mat [6]float64, glyphMode bool) error {
	vx := [4]float64{mat[4], mat[2] + mat[4], mat[0] + mat[2] + mat[4], mat[0] + mat[4]}
	vy := [4]float64{mat[5], mat[3] + mat[5], mat[1] + mat[3] + mat[5], mat[1] + mat[5]}

	xMin := imgCoordMungeLowerC(vx[0], glyphMode)
	xMax := imgCoordMungeUpperC(vx[0], glyphMode)
	yMin := imgCoordMungeLowerC(vy[0], glyphMode)
	yMax := imgCoordMungeUpperC(vy[0], glyphMode)
	for i := 1; i < 4; i++ {
		if t := imgCoordMungeLowerC(vx[i], glyphMode); t < xMin {
			xMin = t
		}
		if t := imgCoordMungeUpperC(vx[i], glyphMode); t > xMax {
			xMax = t
		}
		if t := imgCoordMungeLowerC(vy[i], glyphMode); t < yMin {
			yMin = t
		}
		if t := imgCoordMungeUpperC(vy[i], glyphMode); t > yMax {
			yMax = t
		}
	}
	clipRes := s.testRect(xMin, yMin, xMax-1, yMax-1)
	if clipRes == xpath.ClipAllOutside {
		return nil
	}

	// Compute scale factors exactly like Splash.cc:2965-2993.
	var t0, t1 int
	if mat[0] >= 0 {
		t0 = imgCoordMungeUpperC(mat[0]+mat[4], glyphMode) - imgCoordMungeLowerC(mat[4], glyphMode)
	} else {
		t0 = imgCoordMungeUpperC(mat[4], glyphMode) - imgCoordMungeLowerC(mat[0]+mat[4], glyphMode)
	}
	if mat[1] >= 0 {
		t1 = imgCoordMungeUpperC(mat[1]+mat[5], glyphMode) - imgCoordMungeLowerC(mat[5], glyphMode)
	} else {
		t1 = imgCoordMungeUpperC(mat[5], glyphMode) - imgCoordMungeLowerC(mat[1]+mat[5], glyphMode)
	}
	scaledW := t0
	if t1 > scaledW {
		scaledW = t1
	}
	if mat[2] >= 0 {
		t0 = imgCoordMungeUpperC(mat[2]+mat[4], glyphMode) - imgCoordMungeLowerC(mat[4], glyphMode)
	} else {
		t0 = imgCoordMungeUpperC(mat[4], glyphMode) - imgCoordMungeLowerC(mat[2]+mat[4], glyphMode)
	}
	if mat[3] >= 0 {
		t1 = imgCoordMungeUpperC(mat[3]+mat[5], glyphMode) - imgCoordMungeLowerC(mat[5], glyphMode)
	} else {
		t1 = imgCoordMungeUpperC(mat[5], glyphMode) - imgCoordMungeLowerC(mat[3]+mat[5], glyphMode)
	}
	scaledH := t0
	if t1 > scaledH {
		scaledH = t1
	}
	if scaledW == 0 {
		scaledW = 1
	}
	if scaledH == 0 {
		scaledH = 1
	}

	scaled, err := s.scaleMask(src, srcW, srcH, scaledW, scaledH)
	if err != nil {
		return err
	}

	r00 := mat[0] / float64(scaledW)
	r01 := mat[1] / float64(scaledW)
	r10 := mat[2] / float64(scaledH)
	r11 := mat[3] / float64(scaledH)
	det := r00*r11 - r01*r10
	if math.Abs(det) < 1e-6 {
		return nil
	}
	ir00 := r11 / det
	ir01 := -r01 / det
	ir10 := -r10 / det
	ir11 := r00 / det

	var sections [3]imageMaskSection
	i := 3
	if vy[2] <= vy[3] {
		i = 2
	}
	if vy[1] <= vy[i] {
		i = 1
	}
	if vy[0] < vy[i] || (i != 3 && vy[0] == vy[i]) {
		i = 0
	}
	nSections := 1
	if vy[i] == vy[(i+1)&3] {
		sections[0].y0 = imgCoordMungeLowerC(vy[i], glyphMode)
		sections[0].y1 = imgCoordMungeUpperC(vy[(i+2)&3], glyphMode) - 1
		if vx[i] < vx[(i+1)&3] {
			sections[0].ia0 = i
			sections[0].ia1 = (i + 3) & 3
			sections[0].ib0 = (i + 1) & 3
			sections[0].ib1 = (i + 2) & 3
		} else {
			sections[0].ia0 = (i + 1) & 3
			sections[0].ia1 = (i + 2) & 3
			sections[0].ib0 = i
			sections[0].ib1 = (i + 3) & 3
		}
	} else {
		sections[0].y0 = imgCoordMungeLowerC(vy[i], glyphMode)
		sections[2].y1 = imgCoordMungeUpperC(vy[(i+2)&3], glyphMode) - 1
		sections[0].ia0 = i
		sections[0].ib0 = i
		sections[2].ia1 = (i + 2) & 3
		sections[2].ib1 = (i + 2) & 3
		if vx[(i+1)&3] < vx[(i+3)&3] {
			sections[0].ia1 = (i + 1) & 3
			sections[2].ia0 = (i + 1) & 3
			sections[0].ib1 = (i + 3) & 3
			sections[2].ib0 = (i + 3) & 3
		} else {
			sections[0].ia1 = (i + 3) & 3
			sections[2].ia0 = (i + 3) & 3
			sections[0].ib1 = (i + 1) & 3
			sections[2].ib0 = (i + 1) & 3
		}
		if vy[(i+1)&3] < vy[(i+3)&3] {
			sections[1].y0 = imgCoordMungeLowerC(vy[(i+1)&3], glyphMode)
			sections[2].y0 = imgCoordMungeUpperC(vy[(i+3)&3], glyphMode)
			if vx[(i+1)&3] < vx[(i+3)&3] {
				sections[1].ia0 = (i + 1) & 3
				sections[1].ia1 = (i + 2) & 3
				sections[1].ib0 = i
				sections[1].ib1 = (i + 3) & 3
			} else {
				sections[1].ia0 = i
				sections[1].ia1 = (i + 3) & 3
				sections[1].ib0 = (i + 1) & 3
				sections[1].ib1 = (i + 2) & 3
			}
		} else {
			sections[1].y0 = imgCoordMungeLowerC(vy[(i+3)&3], glyphMode)
			sections[2].y0 = imgCoordMungeUpperC(vy[(i+1)&3], glyphMode)
			if vx[(i+1)&3] < vx[(i+3)&3] {
				sections[1].ia0 = i
				sections[1].ia1 = (i + 1) & 3
				sections[1].ib0 = (i + 3) & 3
				sections[1].ib1 = (i + 2) & 3
			} else {
				sections[1].ia0 = (i + 3) & 3
				sections[1].ia1 = (i + 2) & 3
				sections[1].ib0 = i
				sections[1].ib1 = (i + 1) & 3
			}
		}
		sections[0].y1 = sections[1].y0 - 1
		sections[1].y1 = sections[2].y0 - 1
		nSections = 3
	}
	for j := 0; j < nSections; j++ {
		sec := &sections[j]
		sec.xa0 = vx[sec.ia0]
		sec.ya0 = vy[sec.ia0]
		sec.xa1 = vx[sec.ia1]
		sec.ya1 = vy[sec.ia1]
		sec.xb0 = vx[sec.ib0]
		sec.yb0 = vy[sec.ib0]
		sec.xb1 = vx[sec.ib1]
		sec.yb1 = vy[sec.ib1]
		sec.dxdya = (sec.xa1 - sec.xa0) / (sec.ya1 - sec.ya0)
		sec.dxdyb = (sec.xb1 - sec.xb0) / (sec.yb1 - sec.yb0)
	}

	alphaIn := byte(Round(s.state.fillAlpha * 255))
	var p pipe
	s.pipeInit(&p, 0, 0, s.state.fillPattern, nil, alphaIn, true, false)
	clip, _ := s.state.clip.(*xpath.Clip)
	rowSize := (s.bitmap.width*splashAASize + 7) >> 3
	aaLen := rowSize * splashAASize
	if s.vectorAA && clipRes != xpath.ClipAllInside && len(s.aaBuf) < aaLen {
		s.aaBuf = make([]byte, aaLen)
	}

	if nSections == 1 {
		if sections[0].y0 == sections[0].y1 {
			sections[0].y1++
			clipRes = xpath.ClipPartial
		}
	} else if sections[0].y0 == sections[2].y1 {
		sections[1].y1++
		clipRes = xpath.ClipPartial
	}

	for i := 0; i < nSections; i++ {
		sec := sections[i]
		for y := sec.y0; y <= sec.y1; y++ {
			xa := imgCoordMungeLowerC(sec.xa0+(float64(y)+0.5-sec.ya0)*sec.dxdya, glyphMode)
			xb := imgCoordMungeUpperC(sec.xb0+(float64(y)+0.5-sec.yb0)*sec.dxdyb, glyphMode)
			if xa < 0 {
				xa = 0
			}
			if xa == xb {
				xb++
			}
			clipRes2 := clipRes
			if clipRes != xpath.ClipAllInside && clip != nil {
				clipRes2 = clip.TestSpan(xa, xb-1, y)
			}
			aaReady := false
			if s.vectorAA && clipRes2 != xpath.ClipAllInside && clip != nil {
				if y >= 0 && y < s.bitmap.height {
					for j := 0; j < aaLen; j++ {
						s.aaBuf[j] = 0xff
					}
					clip.ClipAALine(y, s.aaBuf, 0, s.bitmap.width-1)
					aaReady = true
				}
			}
			for x := xa; x < xb; x++ {
				fx := float64(x) + 0.5 - mat[4]
				fy := float64(y) + 0.5 - mat[5]
				xx := Floor(fx*ir00 + fy*ir10)
				yy := Floor(fx*ir01 + fy*ir11)
				if xx < 0 {
					xx = 0
					clipRes2 = xpath.ClipPartial
				} else if xx >= scaledW {
					xx = scaledW - 1
					clipRes2 = xpath.ClipPartial
				}
				if yy < 0 {
					yy = 0
					clipRes2 = xpath.ClipPartial
				} else if yy >= scaledH {
					yy = scaledH - 1
					clipRes2 = xpath.ClipPartial
				}
				shape := scaled.data[yy*scaledW+xx]
				if shape == 0 || x < 0 || x >= s.bitmap.width || y < 0 || y >= s.bitmap.height {
					continue
				}
				if s.vectorAA && clipRes2 != xpath.ClipAllInside {
					if !aaReady {
						continue
					}
					t := s.aaCoverageAt(x, rowSize)
					if t == 0 {
						continue
					}
					shape = byte(Div255(int(s.aaGamma[t]) * int(shape)))
					if shape == 0 {
						continue
					}
				} else if clipRes2 != xpath.ClipAllInside && clip != nil && !clip.Test(x, y) {
					continue
				}
				p.shape = shape
				s.pipeSetXY(&p, x, y)
				if shouldTraceImagePixel(x, y) {
					var c Color
					if p.pattern != nil {
						_ = p.pattern.GetColor(x, y, &c)
					} else {
						c = p.cSrc
					}
					traceImagePixelBefore(&p, "arbitraryTransformMask", x, y, xx, yy, c, shape)
				}
				p.run(&p)
				if shouldTraceImagePixel(x, y) {
					traceImagePixelAfter(&p, "arbitraryTransformMask", x, y)
				}
			}
		}
	}
	return nil
}
