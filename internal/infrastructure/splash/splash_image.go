package splash

import (
	"fmt"
	"math"
	"os"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// imgCoordMungeLower mirrors imgCoordMungeLower (Splash.cc:108).
func imgCoordMungeLower(x float64) int { return Floor(x) }

// imgCoordMungeUpper mirrors imgCoordMungeUpper (Splash.cc:112).
func imgCoordMungeUpper(x float64) int { return Floor(x) + 1 }

// imgCoordMungeLowerC mirrors imgCoordMungeLowerC (Splash.cc:116).
func imgCoordMungeLowerC(x float64, glyphMode bool) int {
	if glyphMode {
		return Ceil(x+0.5) - 1
	}
	return Floor(x)
}

// imgCoordMungeUpperC mirrors imgCoordMungeUpperC (Splash.cc:120).
func imgCoordMungeUpperC(x float64, glyphMode bool) int {
	if glyphMode {
		return Ceil(x+0.5) - 1
	}
	return Floor(x) + 1
}

// isImageInterpolationRequired mirrors isImageInterpolationRequired (Splash.cc:3940).
func isImageInterpolationRequired(srcW, srcH, dstW, dstH int, interpolate bool) bool {
	if interpolate || srcW == 0 || srcH == 0 {
		return true
	}
	if dstW/srcW >= 4 || dstH/srcH >= 4 {
		return false
	}
	return true
}

// nCompsForMode returns the bytes-per-pixel a row buffer must hold (Splash.cc:3506-3531).
func nCompsForMode(m ColorMode) int {
	switch m {
	case ModeMono8:
		return 1
	case ModeRGB8, ModeBGR8:
		return 3
	case ModeXBGR8, ModeCMYK8:
		return 4
	case ModeDeviceN8:
		return splashMaxColorComps
	}
	return 0
}

// DrawImageImpl rasterizes a sampled image (Splash.cc:3489).
func (s *Splash) DrawImageImpl(src ImageSource, w, h int, mat [6]float64, interpolate bool) error {
	return s.drawImageImpl(src, w, h, mat, interpolate, s.bitmap != nil && s.bitmap.alpha != nil)
}

func (s *Splash) drawImageImpl(src ImageSource, w, h int, mat [6]float64, interpolate bool, sourceAlpha bool) error {
	if s.bitmap == nil || s.bitmap.data == nil {
		return ErrBadArg
	}
	nComps := nCompsForMode(s.bitmap.mode)
	if nComps == 0 {
		return ErrModeMismatch
	}
	if w <= 0 || h <= 0 {
		return ErrZeroImage
	}
	// singular-matrix check (Splash.cc:3541).
	det := mat[0]*mat[3] - mat[1]*mat[2]
	if math.Abs(det) < 1e-6 {
		return ErrSingularMatrix
	}

	minorAxisZero := mat[1] == 0 && mat[2] == 0

	// scaling only (Splash.cc:3548).
	if mat[0] > 0 && minorAxisZero && mat[3] > 0 {
		x0 := imgCoordMungeLower(mat[4])
		y0 := imgCoordMungeLower(mat[5])
		x1 := imgCoordMungeUpper(mat[0] + mat[4])
		y1 := imgCoordMungeUpper(mat[3] + mat[5])
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
		scaled, err := s.scaleImageWithSourceAlpha(src, w, h, dstW, dstH, interpolate, sourceAlpha)
		if err != nil {
			return err
		}
		return s.blitImage(scaled, x0, y0, clipRes)
	}

	// scaling plus vertical flip (Splash.cc:3581).
	if mat[0] > 0 && minorAxisZero && mat[3] < 0 {
		// Integer-aligned 2× downscale fastpath (2026-04-27, Path A).
		// When both axes are exact 2× integer downscales at integer device
		// origins (e.g. 150 DPI on a 3.84pt page → 16-pixel image scaled to
		// 8 device pixels) the standard Bresenham 16→9 + vertFlipBitmap +
		// blit pipeline produces a 1-row vertical shift vs pdftoppm. The
		// reference (pdftoppm/legacy canvas) instead uses Poppler's
		// asymmetric grouping (canvas/image_canvas_image_fastpath.go:113
		// popplerSourceRange1D): j=0 alone, j=half alone, last 2 src rows
		// unused. Mirror that here for the integer-aligned 2× case to fix
		// 007-imagemagick at 150 DPI from 75% → ~100% similarity.
		if isIntegerAligned2xDownscale(mat, w, h) {
			dstW := w / 2
			dstH := h / 2
			x0 := int(math.Round(mat[4]))
			y0 := int(math.Round(mat[3] + mat[5]))
			clipRes := s.testRect(x0, y0, x0+dstW-1, y0+dstH-1)
			if clipRes == xpath.ClipAllOutside {
				return nil
			}
			return s.drawIntegerAligned2xDownscaleVFlip(src, w, h, dstW, dstH, x0, y0, clipRes, sourceAlpha)
		}
		x0 := imgCoordMungeLower(mat[4])
		y0 := imgCoordMungeLower(mat[3] + mat[5])
		x1 := imgCoordMungeUpper(mat[0] + mat[4])
		y1 := imgCoordMungeUpper(mat[5])
		if x0 == x1 {
			if mat[4]+mat[0]*0.5 < float64(x0) {
				x0--
			} else {
				x1++
			}
		}
		if y0 == y1 {
			if mat[5]+mat[1]*0.5 < float64(y0) {
				y0--
			} else {
				y1++
			}
		}
		clipRes := s.testRect(x0, y0, x1-1, y1-1)
		if clipRes == xpath.ClipAllOutside {
			return nil
		}
		dstW := x1 - x0
		dstH := y1 - y0
		// Yup×Yup bilinear path: with the standard "closure-flip + scale + vertFlip"
		// pair the bilinear's last-row clamp lands on the wrong end of the dst
		// bitmap (the kernel iterates ySrc up to srcH-yStep with clamp, so the
		// END of the iteration is "stuck" on the last source row). After the
		// vertFlip that "stuck" row becomes the SECOND canvas row near the
		// origin, producing a 1-row vertical shift relative to pdftoppm and
		// image-canvas's drawAxisAlignedSplashBilinear (which iterates top-down
		// directly without the flip pair).
		//
		// Fix: when bilinear is selected, read source rows directly top-down
		// (ignore the closure's Y-flip by indexing srcH-1-row) and skip the
		// post-vertFlip — produces the same end-to-end orientation but with
		// the bilinear blend distributed top-to-bottom matching pdftoppm.
		// Memory bilinear_yflip_2026_04_27.
		if dstW >= w && dstH >= h && isImageInterpolationRequired(w, h, dstW, dstH, interpolate) {
			scaled := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
			if scaled.data == nil {
				return ErrZeroImage
			}
			topDownSrc := func(row int, color, alpha []byte) error {
				// Closure delivers row k as stdlib row (srcH-1-k); rewrap so
				// row k → stdlib row k (top-down).
				return src(h-1-row, color, alpha)
			}
			if err := s.scaleImageYupXupBilinear(topDownSrc, w, h, dstW, dstH, scaled); err != nil {
				return err
			}
			return s.blitImage(scaled, x0, y0, clipRes)
		}
		if dstW >= w && dstH >= h {
			scaled := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
			if scaled.data == nil {
				return ErrZeroImage
			}
			topDownSrc := func(row int, color, alpha []byte) error {
				return src(h-1-row, color, alpha)
			}
			if err := s.scaleImageYupXup(topDownSrc, w, h, dstW, dstH, scaled); err != nil {
				return err
			}
			return s.blitImage(scaled, x0, y0, clipRes)
		}
		// The backend ImageSource already presents regular PDF images in
		// top-down display order. For Y-down downscales, scaling that stream
		// directly matches Poppler's row grouping; scaling then vert-flipping
		// shifts high-resolution Flate images such as GeoTopo p31.
		if (s.downscaleVFlipTopDown || !sourceAlpha) && dstW < w && dstH < h {
			scaled := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
			if scaled.data == nil {
				return ErrZeroImage
			}
			topDownSrc := func(row int, color, alpha []byte) error {
				return src(h-1-row, color, alpha)
			}
			if err := s.scaleImageYdownXdown(topDownSrc, w, h, dstW, dstH, scaled); err != nil {
				return err
			}
			return s.blitImage(scaled, x0, y0, clipRes)
		}
		scaled, err := s.scaleImageWithSourceAlpha(src, w, h, dstW, dstH, interpolate, sourceAlpha)
		if err != nil {
			return err
		}
		vertFlipBitmap(scaled, nComps)
		return s.blitImage(scaled, x0, y0, clipRes)
	}

	// general affine (Splash.cc:3623).
	return s.arbitraryTransformImage(src, w, h, mat, interpolate, sourceAlpha)
}

// isIntegerAligned2xDownscale reports whether the supplied image-placement
// matrix is an axis-aligned vertical-flip 2× integer downscale anchored at
// integer device coordinates — the canonical "page-pixel" image case where
// pdftoppm uses Poppler's asymmetric box grouping rather than Splash's
// Bresenham. mat must already satisfy mat[0]>0 && mat[1]==0 && mat[2]==0 &&
// mat[3]<0 (the caller's branch guard).
func isIntegerAligned2xDownscale(mat [6]float64, w, h int) bool {
	if w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 {
		return false
	}
	dstW := float64(w / 2)
	dstH := float64(h / 2)
	if mat[0] != dstW || mat[3] != -dstH {
		return false
	}
	if !isNearlyIntegerCoord(mat[4]) || !isNearlyIntegerCoord(mat[5]) {
		return false
	}
	return true
}

func isNearlyIntegerCoord(v float64) bool {
	return math.Abs(v-math.Round(v)) < 1e-9
}

// drawIntegerAligned2xDownscaleVFlip mirrors canvas's
// drawAxisAlignedPopplerStyle2xBox (image_canvas_image_fastpath.go:129) for
// the splash mat[3]<0 branch. It reads the ImageSource row-by-row (the
// closure already supplies stdlib-Y rows in PDF Y-up order — i.e. closure
// row 0 = top of the image as it should appear top-down on the splash
// bitmap) and applies the asymmetric `popplerSourceRange1D` 2× grouping.
//
// Source row mapping (16→8, dstH=8, srcH=16):
//
//	dst row 0 ← src row 0 (alone)
//	dst row j (1≤j<4) ← mean(src rows 2j-1, 2j)
//	dst row 4 ← src row 7 (alone)
//	dst row j (5≤j<8) ← mean(src rows 8+2(j-5), 8+2(j-5)+1)
//	src rows 14, 15 are unused.
//
// Same mapping is applied along X.
//
// NOTE: the splash ImageSource closure (backend.go:651) flips stdlib Y so
// closure row k = stdlib row (srcH-1-k). The mat[3]<0 branch is itself a
// vertical flip path — Splash normally pairs scaleImage + vertFlipBitmap to
// re-flip. To emulate the same end-to-end orientation here without that
// pair, we read closure rows in REVERSE (closure[srcH-1] first, closure[0]
// last) so the iteration sees stdlib top-to-bottom.
func (s *Splash) drawIntegerAligned2xDownscaleVFlip(
	src ImageSource, srcW, srcH, dstW, dstH, dstX, dstY int, clipRes xpath.ClipResult, sourceAlpha bool,
) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := sourceAlpha

	// Buffer all source rows (top-to-bottom in stdlib order). Splash's
	// closure delivers row k = stdlib row (srcH-1-k), so closureRow[srcH-1]
	// is the stdlib top row.
	rows := make([][]byte, srcH)
	var alphaRows [][]byte
	if hasAlpha {
		alphaRows = make([][]byte, srcH)
	}
	for k := 0; k < srcH; k++ {
		row := make([]byte, srcW*nComps)
		var alpha []byte
		if hasAlpha {
			alpha = make([]byte, srcW)
		}
		if err := src(k, row, alpha); err != nil {
			return err
		}
		// closure row k = stdlib row (srcH-1-k).
		stdlibRow := srcH - 1 - k
		rows[stdlibRow] = row
		if hasAlpha {
			alphaRows[stdlibRow] = alpha
		}
	}

	// Build a temporary scaled bitmap, then blit it through the same
	// pipeline as the standard path (so clipping / fill-alpha composition
	// stay consistent).
	scaled := NewBitmap(dstW, dstH, s.bitmap.mode, hasAlpha)
	if scaled.data == nil {
		return ErrZeroImage
	}
	bpp := bytesPerPixel(scaled.mode)

	for dy := 0; dy < dstH; dy++ {
		ry0, ry1 := popplerRange1D(dy, dstH, srcH)
		for dx := 0; dx < dstW; dx++ {
			rx0, rx1 := popplerRange1D(dx, dstW, srcW)
			var pix [splashMaxColorComps]uint32
			var aSum uint32
			count := uint32((ry1 - ry0) * (rx1 - rx0))
			if count == 0 {
				continue
			}
			for sy := ry0; sy < ry1; sy++ {
				row := rows[sy]
				for sx := rx0; sx < rx1; sx++ {
					base := sx * nComps
					for c := 0; c < nComps; c++ {
						pix[c] += uint32(row[base+c])
					}
					if hasAlpha {
						aSum += uint32(alphaRows[sy][sx])
					}
				}
			}
			for c := 0; c < nComps; c++ {
				pix[c] /= count
			}
			off := (dy*dstW + dx) * bpp
			writePixel(scaled.data, off, scaled.mode, pix[:])
			if hasAlpha {
				scaled.alpha[dy*dstW+dx] = byte(aSum / count)
			}
		}
	}

	return s.blitImage(scaled, dstX, dstY, clipRes)
}

// popplerRange1D mirrors canvas.popplerSourceRange1D
// (image_canvas_image_fastpath.go:113) — Poppler's asymmetric 2× downscale
// grouping. Returns [start, end) source indices for destination index j.
func popplerRange1D(j, dstDim, srcDim int) (int, int) {
	half := dstDim / 2
	switch {
	case j == 0:
		return 0, 1
	case j == half:
		return srcDim/2 - 1, srcDim / 2
	case j < half:
		s := 2*j - 1
		return s, s + 2
	default:
		s := srcDim/2 + 2*(j-half-1)
		return s, s + 2
	}
}

// testRect mirrors SplashClip::testRect (SplashClip.cc) — phase-3 callers
// use bitmap bounds when the clip is unset.
func (s *Splash) testRect(x0, y0, x1, y1 int) xpath.ClipResult {
	if clip, ok := s.state.clip.(*xpath.Clip); ok && clip != nil {
		return clip.TestRect(x0, y0, x1, y1)
	}
	if x1 < 0 || y1 < 0 || x0 >= s.bitmap.width || y0 >= s.bitmap.height {
		return xpath.ClipAllOutside
	}
	if x0 >= 0 && y0 >= 0 && x1 < s.bitmap.width && y1 < s.bitmap.height {
		return xpath.ClipAllInside
	}
	return xpath.ClipPartial
}

// scaleImage dispatches one of the 5 axis-aligned kernels (Splash.cc:3955).
func (s *Splash) scaleImage(src ImageSource, srcW, srcH, dstW, dstH int, interpolate bool) (*Bitmap, error) {
	return s.scaleImageWithSourceAlpha(src, srcW, srcH, dstW, dstH, interpolate, s.bitmap != nil && s.bitmap.alpha != nil)
}

func (s *Splash) scaleImageWithSourceAlpha(src ImageSource, srcW, srcH, dstW, dstH int, interpolate bool, sourceAlpha bool) (*Bitmap, error) {
	dest := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
	if dest.data == nil || srcW <= 0 || srcH <= 0 {
		return nil, ErrZeroImage
	}
	var err error
	if dstH < srcH {
		if dstW < srcW {
			err = s.scaleImageYdownXdown(src, srcW, srcH, dstW, dstH, dest)
		} else {
			err = s.scaleImageYdownXup(src, srcW, srcH, dstW, dstH, dest)
		}
	} else {
		if dstW < srcW {
			err = s.scaleImageYupXdown(src, srcW, srcH, dstW, dstH, dest)
		} else {
			if isImageInterpolationRequired(srcW, srcH, dstW, dstH, interpolate) {
				err = s.scaleImageYupXupBilinear(src, srcW, srcH, dstW, dstH, dest)
			} else {
				err = s.scaleImageYupXup(src, srcW, srcH, dstW, dstH, dest)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return dest, nil
}

// readImageRow pulls one source row (color + optional alpha) (Splash.h:50-55).
func (s *Splash) readImageRow(src ImageSource, srcW int, color, alpha []byte, row int) error {
	_ = srcW
	if alpha != nil {
		return src(row, color, alpha)
	}
	return src(row, color, nil)
}

// scaleImageYdownXdown — both axes shrink (Splash.cc:3990).
func (s *Splash) scaleImageYdownXdown(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil

	yp := srcH / dstH
	yq := srcH % dstH
	xp := srcW / dstW
	xq := srcW % dstW

	lineBuf := make([]byte, srcW*nComps)
	pixBuf := make([]uint32, srcW*nComps)
	var alphaLine []byte
	var alphaPix []uint32
	if hasAlpha {
		alphaLine = make([]byte, srcW)
		alphaPix = make([]uint32, srcW)
	}

	yt := 0
	rowIdx := 0
	destOff := 0
	destAlphaOff := 0
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
		if hasAlpha {
			for j := range alphaPix {
				alphaPix[j] = 0
			}
		}
		for i := 0; i < yStep; i++ {
			if err := s.readImageRow(src, srcW, lineBuf, alphaLine, rowIdx); err != nil {
				return err
			}
			rowIdx++
			for j := 0; j < srcW*nComps; j++ {
				pixBuf[j] += uint32(lineBuf[j])
			}
			if hasAlpha {
				for j := 0; j < srcW; j++ {
					alphaPix[j] += uint32(alphaLine[j])
				}
			}
		}

		xt := 0
		d0 := uint32((1 << 23) / (yStep * xp))
		d1 := uint32((1 << 23) / (yStep * (xp + 1)))
		xx := 0
		xxa := 0
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

			var pix [splashMaxColorComps]uint32
			for i := 0; i < xStep; i++ {
				for c := 0; c < nComps; c++ {
					pix[c] += pixBuf[xx+c]
				}
				xx += nComps
			}
			for c := 0; c < nComps; c++ {
				pix[c] = (pix[c] * d) >> 23
			}
			storeScaledPixel(dest.data, &destOff, dest.mode, pix[:])

			if hasAlpha {
				var a uint32
				for i := 0; i < xStep; i++ {
					a += alphaPix[xxa]
					xxa++
				}
				a = (a * d) >> 23
				dest.alpha[destAlphaOff] = byte(a)
				destAlphaOff++
			}
		}
	}
	return nil
}

// scaleImageYdownXup — Y shrinks, X grows (Splash.cc:4230).
func (s *Splash) scaleImageYdownXup(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil

	yp := srcH / dstH
	yq := srcH % dstH
	xp := dstW / srcW
	xq := dstW % srcW

	lineBuf := make([]byte, srcW*nComps)
	pixBuf := make([]uint32, srcW*nComps)
	var alphaLine []byte
	var alphaPix []uint32
	if hasAlpha {
		alphaLine = make([]byte, srcW)
		alphaPix = make([]uint32, srcW)
	}

	yt := 0
	rowIdx := 0
	destOff := 0
	destAlphaOff := 0
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
		if hasAlpha {
			for j := range alphaPix {
				alphaPix[j] = 0
			}
		}
		for i := 0; i < yStep; i++ {
			if err := s.readImageRow(src, srcW, lineBuf, alphaLine, rowIdx); err != nil {
				return err
			}
			rowIdx++
			for j := 0; j < srcW*nComps; j++ {
				pixBuf[j] += uint32(lineBuf[j])
			}
			if hasAlpha {
				for j := 0; j < srcW; j++ {
					alphaPix[j] += uint32(alphaLine[j])
				}
			}
		}

		xt := 0
		d := uint32((1 << 23) / yStep)
		for x := 0; x < srcW; x++ {
			var xStep int
			xt += xq
			if xt >= srcW {
				xt -= srcW
				xStep = xp + 1
			} else {
				xStep = xp
			}
			var pix [splashMaxColorComps]uint32
			for c := 0; c < nComps; c++ {
				pix[c] = (pixBuf[x*nComps+c] * d) >> 23
			}
			for i := 0; i < xStep; i++ {
				storeScaledPixel(dest.data, &destOff, dest.mode, pix[:])
			}
			if hasAlpha {
				a := (alphaPix[x] * d) >> 23
				for i := 0; i < xStep; i++ {
					dest.alpha[destAlphaOff] = byte(a)
					destAlphaOff++
				}
			}
		}
	}
	return nil
}

// scaleImageYupXdown — Y grows, X shrinks (Splash.cc:4382).
func (s *Splash) scaleImageYupXdown(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil
	bpp := bytesPerPixel(dest.mode)

	yp := dstH / srcH
	yq := dstH % srcH
	xp := srcW / dstW
	xq := srcW % dstW

	lineBuf := make([]byte, srcW*nComps)
	var alphaLine []byte
	if hasAlpha {
		alphaLine = make([]byte, srcW)
	}

	yt := 0
	destRowBase := 0
	destAlphaRowBase := 0
	for y := 0; y < srcH; y++ {
		var yStep int
		yt += yq
		if yt >= srcH {
			yt -= srcH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		if err := s.readImageRow(src, srcW, lineBuf, alphaLine, y); err != nil {
			return err
		}

		xt := 0
		d0 := uint32((1 << 23) / xp)
		d1 := uint32((1 << 23) / (xp + 1))
		xx := 0
		xxa := 0
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
			var pix [splashMaxColorComps]uint32
			for i := 0; i < xStep; i++ {
				for c := 0; c < nComps; c++ {
					pix[c] += uint32(lineBuf[xx])
					xx++
				}
			}
			for c := 0; c < nComps; c++ {
				pix[c] = (pix[c] * d) >> 23
			}
			for i := 0; i < yStep; i++ {
				off := destRowBase + (i*dstW+x)*bpp
				writePixel(dest.data, off, dest.mode, pix[:])
			}
			if hasAlpha {
				var a uint32
				for i := 0; i < xStep; i++ {
					a += uint32(alphaLine[xxa])
					xxa++
				}
				a = (a * d) >> 23
				for i := 0; i < yStep; i++ {
					dest.alpha[destAlphaRowBase+i*dstW+x] = byte(a)
				}
			}
		}
		destRowBase += yStep * dstW * bpp
		if hasAlpha {
			destAlphaRowBase += yStep * dstW
		}
	}
	return nil
}

// scaleImageYupXup — both grow nearest/box (Splash.cc:4542).
func (s *Splash) scaleImageYupXup(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil
	bpp := bytesPerPixel(dest.mode)

	yp := dstH / srcH
	yq := dstH % srcH
	xp := dstW / srcW
	xq := dstW % srcW

	lineBuf := make([]byte, srcW*nComps)
	var alphaLine []byte
	if hasAlpha {
		alphaLine = make([]byte, srcW)
	}

	yt := 0
	destRowBase := 0
	destAlphaRowBase := 0
	for y := 0; y < srcH; y++ {
		var yStep int
		yt += yq
		if yt >= srcH {
			yt -= srcH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		if err := s.readImageRow(src, srcW, lineBuf, alphaLine, y); err != nil {
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
			var pix [splashMaxColorComps]uint32
			for c := 0; c < nComps; c++ {
				pix[c] = uint32(lineBuf[x*nComps+c])
			}
			for i := 0; i < yStep; i++ {
				for j := 0; j < xStep; j++ {
					off := destRowBase + (i*dstW+xx+j)*bpp
					writePixel(dest.data, off, dest.mode, pix[:])
				}
			}
			if hasAlpha {
				a := alphaLine[x]
				for i := 0; i < yStep; i++ {
					for j := 0; j < xStep; j++ {
						dest.alpha[destAlphaRowBase+i*dstW+xx+j] = a
					}
				}
			}
			xx += xStep
		}
		destRowBase += yStep * dstW * bpp
		if hasAlpha {
			destAlphaRowBase += yStep * dstW
		}
	}
	return nil
}

// expandRow expands one srcWidth row to scaledWidth via linear interpolation
// (Splash.cc:4697). srcBuf must have one extra pixel of slack at index srcWidth.
func expandRow(srcBuf, dstBuf []byte, srcWidth, scaledWidth, nComps int) {
	if srcWidth == 0 || scaledWidth == 0 {
		return
	}
	xStep := float64(srcWidth) / float64(scaledWidth)
	xSrc := 0.0
	// pad slot equal to last pixel (Splash.cc:4707).
	for i := 0; i < nComps; i++ {
		srcBuf[srcWidth*nComps+i] = srcBuf[(srcWidth-1)*nComps+i]
	}
	for x := 0; x < scaledWidth; x++ {
		xInt, xFrac := math.Modf(xSrc)
		p := int(xInt)
		for c := 0; c < nComps; c++ {
			a := float64(srcBuf[nComps*p+c])
			b := float64(srcBuf[nComps*(p+1)+c])
			dstBuf[nComps*x+c] = byte(a*(1.0-xFrac) + b*xFrac)
		}
		xSrc += xStep
	}
}

// scaleImageYupXupBilinear — both grow with bilinear interpolation (Splash.cc:4722).
//
// CRITICAL last-row clamp: when currentSrcRow reaches srcH-1 the kernel must
// NOT read another row past the source. lineBuf2 retains the previous (last
// valid) data, providing the row of "padding" the interpolation needs. This
// matches Splash.cc:4771 and is required by memory bilinear_lastrow_clamp_2026_04_26.
func (s *Splash) scaleImageYupXupBilinear(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	if srcW < 1 || srcH < 1 {
		return ErrZeroImage
	}
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil
	bpp := bytesPerPixel(dest.mode)

	srcBuf := make([]byte, (srcW+1)*nComps)
	lineBuf1 := make([]byte, dstW*nComps)
	lineBuf2 := make([]byte, dstW*nComps)
	var alphaSrcBuf, alphaLineBuf1, alphaLineBuf2 []byte
	if hasAlpha {
		alphaSrcBuf = make([]byte, srcW+1)
		alphaLineBuf1 = make([]byte, dstW)
		alphaLineBuf2 = make([]byte, dstW)
	}

	yStep := float64(srcH) / float64(dstH)
	ySrc := 0.0
	currentSrcRow := -1
	rowIdx := 0

	if err := s.readImageRow(src, srcW, srcBuf[:srcW*nComps], alphaSrcBuf, rowIdx); err != nil {
		return err
	}
	rowIdx++
	expandRow(srcBuf, lineBuf2, srcW, dstW, nComps)
	if hasAlpha {
		expandRow(alphaSrcBuf, alphaLineBuf2, srcW, dstW, 1)
	}

	for y := 0; y < dstH; y++ {
		yInt, yFrac := math.Modf(ySrc)
		if int(yInt) > currentSrcRow {
			currentSrcRow++
			// promote line2 → line1
			copy(lineBuf1, lineBuf2)
			if hasAlpha {
				copy(alphaLineBuf1, alphaLineBuf2)
			}
			// last-row clamp: only fetch the next row if we are not yet at the
			// final source row. Otherwise lineBuf2 keeps its current values
			// (= lineBuf1 after the copy above), giving a clamp-to-last-row
			// pad. Splash.cc:4771; memory bilinear_lastrow_clamp_2026_04_26.
			if currentSrcRow < srcH-1 {
				if err := s.readImageRow(src, srcW, srcBuf[:srcW*nComps], alphaSrcBuf, rowIdx); err != nil {
					return err
				}
				rowIdx++
				expandRow(srcBuf, lineBuf2, srcW, dstW, nComps)
				if hasAlpha {
					expandRow(alphaSrcBuf, alphaLineBuf2, srcW, dstW, 1)
				}
			}
		}

		for x := 0; x < dstW; x++ {
			var pix [splashMaxColorComps]uint32
			for i := 0; i < nComps; i++ {
				a := float64(lineBuf1[x*nComps+i])
				b := float64(lineBuf2[x*nComps+i])
				pix[i] = uint32(byte(a*(1.0-yFrac) + b*yFrac))
			}
			off := (y*dstW + x) * bpp
			writePixel(dest.data, off, dest.mode, pix[:])
			if hasAlpha {
				a := float64(alphaLineBuf1[x])
				b := float64(alphaLineBuf2[x])
				dest.alpha[y*dstW+x] = byte(a*(1.0-yFrac) + b*yFrac)
			}
		}
		ySrc += yStep
	}
	return nil
}

// storeScaledPixel writes one nComps-mode pixel and advances destOff
// (used by Y*X* kernels that emit pixels sequentially) (Splash.cc:4080-4202).
func storeScaledPixel(dst []byte, destOff *int, mode ColorMode, pix []uint32) {
	switch mode {
	case ModeMono8:
		dst[*destOff] = byte(pix[0])
		*destOff = *destOff + 1
	case ModeRGB8:
		dst[*destOff] = byte(pix[0])
		dst[*destOff+1] = byte(pix[1])
		dst[*destOff+2] = byte(pix[2])
		*destOff += 3
	case ModeBGR8:
		dst[*destOff] = byte(pix[2])
		dst[*destOff+1] = byte(pix[1])
		dst[*destOff+2] = byte(pix[0])
		*destOff += 3
	case ModeXBGR8:
		dst[*destOff] = byte(pix[2])
		dst[*destOff+1] = byte(pix[1])
		dst[*destOff+2] = byte(pix[0])
		dst[*destOff+3] = 255
		*destOff += 4
	case ModeCMYK8:
		dst[*destOff] = byte(pix[0])
		dst[*destOff+1] = byte(pix[1])
		dst[*destOff+2] = byte(pix[2])
		dst[*destOff+3] = byte(pix[3])
		*destOff += 4
	case ModeDeviceN8:
		for i := 0; i < splashMaxColorComps; i++ {
			dst[*destOff+i] = byte(pix[i])
		}
		*destOff += splashMaxColorComps
	}
}

// writePixel stores one mode-specific pixel at offset off (random-access
// counterpart to storeScaledPixel) (Splash.cc:4467-4513).
func writePixel(dst []byte, off int, mode ColorMode, pix []uint32) {
	switch mode {
	case ModeMono8:
		dst[off] = byte(pix[0])
	case ModeRGB8:
		dst[off] = byte(pix[0])
		dst[off+1] = byte(pix[1])
		dst[off+2] = byte(pix[2])
	case ModeBGR8:
		dst[off] = byte(pix[2])
		dst[off+1] = byte(pix[1])
		dst[off+2] = byte(pix[0])
	case ModeXBGR8:
		dst[off] = byte(pix[2])
		dst[off+1] = byte(pix[1])
		dst[off+2] = byte(pix[0])
		dst[off+3] = 255
	case ModeCMYK8:
		dst[off] = byte(pix[0])
		dst[off+1] = byte(pix[1])
		dst[off+2] = byte(pix[2])
		dst[off+3] = byte(pix[3])
	case ModeDeviceN8:
		for i := 0; i < splashMaxColorComps; i++ {
			dst[off+i] = byte(pix[i])
		}
	}
}

// vertFlipBitmap mirrors Splash::vertFlipImage (Splash.cc:4844).
func vertFlipBitmap(b *Bitmap, nComps int) {
	if b == nil || b.data == nil {
		return
	}
	w := b.width * nComps
	tmp := make([]byte, w)
	for top, bot := 0, b.height-1; top < bot; top, bot = top+1, bot-1 {
		copy(tmp, b.data[top*w:top*w+w])
		copy(b.data[top*w:top*w+w], b.data[bot*w:bot*w+w])
		copy(b.data[bot*w:bot*w+w], tmp)
	}
	if b.alpha != nil {
		aw := b.width
		atmp := make([]byte, aw)
		for top, bot := 0, b.height-1; top < bot; top, bot = top+1, bot-1 {
			copy(atmp, b.alpha[top*aw:top*aw+aw])
			copy(b.alpha[top*aw:top*aw+aw], b.alpha[bot*aw:bot*aw+aw])
			copy(b.alpha[bot*aw:bot*aw+aw], atmp)
		}
	}
}

// blitImage writes scaled onto the main bitmap with optional clip (Splash.cc:4880).
func (s *Splash) blitImage(scaled *Bitmap, xDest, yDest int, clipRes xpath.ClipResult) error {
	if scaled == nil || scaled.data == nil {
		return nil
	}
	w := scaled.width
	h := scaled.height
	hasAlpha := scaled.alpha != nil
	bpp := bytesPerPixel(s.bitmap.mode)
	srcBpp := bytesPerPixel(scaled.mode)
	if bpp != srcBpp {
		return ErrModeMismatch
	}

	// Resolve unclipped sub-rect in scaled coords (Splash.cc:4890-4919).
	x0, y0 := 0, 0
	x1, y1 := w, h
	clip, _ := s.state.clip.(*xpath.Clip)
	if clipRes != xpath.ClipAllInside {
		if clip != nil && clip.HasPathClip() {
			x0, x1 = w, w
			y0, y1 = h, h
		} else if clip != nil {
			clipXMin, clipYMin, clipXMax, clipYMax := clip.Bounds()
			if t := Ceil(clipXMin) - xDest; t > x0 {
				x0 = t
			}
			if t := Ceil(clipYMin) - yDest; t > y0 {
				y0 = t
			}
			if t := Floor(clipXMax) - xDest; t < x1 {
				x1 = t
			}
			if t := Floor(clipYMax) - yDest; t < y1 {
				y1 = t
			}
		} else {
			if t := -xDest; t > x0 {
				x0 = t
			}
			if t := -yDest; t > y0 {
				y0 = t
			}
			if t := s.bitmap.width - xDest; t < x1 {
				x1 = t
			}
			if t := s.bitmap.height - yDest; t < y1 {
				y1 = t
			}
		}
	}
	if x0 > x1 {
		x1 = x0
	}
	if y0 > y1 {
		y1 = y0
	}

	if x0 < x1 && y0 < y1 {
		alphaIn := byte(Round(s.state.fillAlpha * 255))
		var p pipe
		s.pipeInit(&p, xDest+x0, yDest+y0, nil, &Color{}, alphaIn, hasAlpha, false)

		for y := y0; y < y1; y++ {
			s.pipeSetXY(&p, xDest+x0, yDest+y)
			srcOff := (y*w + x0) * srcBpp
			var aOff int
			if hasAlpha {
				aOff = y*w + x0
			}
			for x := x0; x < x1; x++ {
				var c Color
				readScaledPixel(scaled.data, srcOff, scaled.mode, &c)
				shape := byte(255)
				if hasAlpha {
					shape = scaled.alpha[aOff]
					p.shape = shape
					unpremultiplyImageColor(&c, scaled.mode, shape)
					aOff++
				}
				dx := xDest + x
				dy := yDest + y
				p.cSrc = c
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelBefore(&p, "blitImage", dx, dy, x, y, c, shape)
				}
				p.run(&p)
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelAfter(&p, "blitImage", dx, dy)
				}
				srcOff += srcBpp
			}
		}
	}

	if clipRes == xpath.ClipAllInside {
		return nil
	}
	if y0 > 0 {
		if err := s.blitImageClipped(scaled, 0, 0, xDest, yDest, w, y0); err != nil {
			return err
		}
	}
	if y1 < h {
		if err := s.blitImageClipped(scaled, 0, y1, xDest, yDest+y1, w, h-y1); err != nil {
			return err
		}
	}
	if x0 > 0 && y0 < y1 {
		if err := s.blitImageClipped(scaled, 0, y0, xDest, yDest+y0, x0, y1-y0); err != nil {
			return err
		}
	}
	if x1 < w && y0 < y1 {
		if err := s.blitImageClipped(scaled, x1, y0, xDest+x1, yDest+y0, w-x1, y1-y0); err != nil {
			return err
		}
	}
	return nil
}

func (s *Splash) blitImageClipped(scaled *Bitmap, xSrc, ySrc, xDest, yDest, w, h int) error {
	if w <= 0 || h <= 0 {
		return nil
	}
	if s.vectorAA {
		return s.blitImageClippedAA(scaled, xSrc, ySrc, xDest, yDest, w, h)
	}
	return s.blitImageClippedNoAA(scaled, xSrc, ySrc, xDest, yDest, w, h)
}

func (s *Splash) blitImageClippedAA(scaled *Bitmap, xSrc, ySrc, xDest, yDest, w, h int) error {
	clip, _ := s.state.clip.(*xpath.Clip)
	if clip == nil {
		return nil
	}
	_, clipYMin, _, clipYMax := clip.IntBounds()
	bpp := bytesPerPixel(scaled.mode)
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	hasAlpha := scaled.alpha != nil
	var p pipe
	s.pipeInit(&p, xDest, yDest, nil, &Color{}, alphaIn, true, false)

	rowSize := (s.bitmap.width*splashAASize + 7) >> 3
	aaLen := rowSize * splashAASize
	if len(s.aaBuf) < aaLen {
		s.aaBuf = make([]byte, aaLen)
	}

	for y := 0; y < h; y++ {
		dy := yDest + y
		if dy < clipYMin || dy > clipYMax || dy < 0 || dy >= s.bitmap.height {
			continue
		}
		for i := 0; i < aaLen; i++ {
			s.aaBuf[i] = 0xff
		}
		clip.ClipAALine(dy, s.aaBuf, 0, s.bitmap.width-1)

		srcOff := ((ySrc+y)*scaled.width + xSrc) * bpp
		alphaOff := (ySrc+y)*scaled.width + xSrc
		for x := 0; x < w; x++ {
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width {
				srcOff += bpp
				alphaOff++
				continue
			}
			t := s.aaCoverageAt(dx, rowSize)
			if t == 0 {
				srcOff += bpp
				alphaOff++
				continue
			}
			var c Color
			readScaledPixel(scaled.data, srcOff, scaled.mode, &c)
			shape := byte(255)
			if hasAlpha {
				shape = scaled.alpha[alphaOff]
				unpremultiplyImageColor(&c, scaled.mode, shape)
			}
			shape = byte(Div255(int(s.aaGamma[t]) * int(shape)))
			if shape != 0 {
				p.cSrc = c
				p.shape = shape
				s.pipeSetXY(&p, dx, dy)
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelBefore(&p, "blitImageClippedAA", dx, dy, xSrc+x, ySrc+y, c, shape)
				}
				p.run(&p)
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelAfter(&p, "blitImageClippedAA", dx, dy)
				}
			}
			srcOff += bpp
			alphaOff++
		}
	}
	return nil
}

func (s *Splash) blitImageClippedNoAA(scaled *Bitmap, xSrc, ySrc, xDest, yDest, w, h int) error {
	clip, _ := s.state.clip.(*xpath.Clip)
	if clip == nil {
		return nil
	}
	bpp := bytesPerPixel(scaled.mode)
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	hasAlpha := scaled.alpha != nil
	var p pipe
	s.pipeInit(&p, xDest, yDest, nil, &Color{}, alphaIn, hasAlpha, false)

	for y := 0; y < h; y++ {
		dy := yDest + y
		if dy < 0 || dy >= s.bitmap.height {
			continue
		}
		srcOff := ((ySrc+y)*scaled.width + xSrc) * bpp
		alphaOff := (ySrc+y)*scaled.width + xSrc
		for x := 0; x < w; x++ {
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width || clip.TestSpan(dx, dx, dy) == xpath.ClipAllOutside {
				srcOff += bpp
				alphaOff++
				continue
			}
			var c Color
			readScaledPixel(scaled.data, srcOff, scaled.mode, &c)
			shape := byte(255)
			if hasAlpha {
				shape = scaled.alpha[alphaOff]
				p.shape = shape
				unpremultiplyImageColor(&c, scaled.mode, shape)
			}
			p.cSrc = c
			s.pipeSetXY(&p, dx, dy)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelBefore(&p, "blitImageClippedNoAA", dx, dy, xSrc+x, ySrc+y, c, shape)
			}
			p.run(&p)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelAfter(&p, "blitImageClippedNoAA", dx, dy)
			}
			srcOff += bpp
			alphaOff++
		}
	}
	return nil
}

func (s *Splash) aaCoverageAt(x, rowSize int) int {
	cell := x * splashAASize
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
	return t
}

// unpremultiplyImageColor converts Go image.Image RGBA() premultiplied samples
// back to the straight source-color samples Splash expects alongside srcAlpha
// shape (Splash.cc blitImage -> pipe.shape). Without this, soft-mask/color-key
// image paths darken partially transparent pixels by applying alpha to color and
// then again in the pipe.
func unpremultiplyImageColor(c *Color, mode ColorMode, alpha byte) {
	if alpha == 0 || alpha == 255 {
		return
	}
	switch mode {
	case ModeMono8:
		c[0] = unpremultiplyByte(c[0], alpha)
	case ModeRGB8, ModeBGR8:
		c[0] = unpremultiplyByte(c[0], alpha)
		c[1] = unpremultiplyByte(c[1], alpha)
		c[2] = unpremultiplyByte(c[2], alpha)
	}
}

func unpremultiplyByte(v byte, alpha byte) byte {
	out := (int(v)*255 + int(alpha)/2) / int(alpha)
	if out > 255 {
		return 255
	}
	return byte(out)
}

// readScaledPixel reads a single pixel from the scaled bitmap into c
// (Splash.cc:4929 src->getPixel).
func readScaledPixel(data []byte, off int, mode ColorMode, c *Color) {
	switch mode {
	case ModeMono8:
		c[0] = data[off]
	case ModeRGB8:
		c[0] = data[off]
		c[1] = data[off+1]
		c[2] = data[off+2]
	case ModeBGR8:
		c[0] = data[off+2]
		c[1] = data[off+1]
		c[2] = data[off]
	case ModeXBGR8:
		c[0] = data[off+2]
		c[1] = data[off+1]
		c[2] = data[off]
		c[3] = 255
	case ModeCMYK8:
		c[0] = data[off]
		c[1] = data[off+1]
		c[2] = data[off+2]
		c[3] = data[off+3]
	case ModeDeviceN8:
		for i := 0; i < splashMaxColorComps; i++ {
			c[i] = data[off+i]
		}
	}
}

// arbitraryTransformImage rasterises a non-axis-aligned image via Poppler's
// three-section quadrilateral scan (Splash.cc:3750-4074).
func (s *Splash) arbitraryTransformImage(src ImageSource, srcW, srcH int, mat [6]float64, interpolate bool, sourceAlpha bool) error {
	// four target-quad vertices (Splash.cc:3645-3652).
	vx := [4]float64{mat[4], mat[2] + mat[4], mat[0] + mat[2] + mat[4], mat[0] + mat[4]}
	vy := [4]float64{mat[5], mat[3] + mat[5], mat[1] + mat[3] + mat[5], mat[1] + mat[5]}

	// device bbox.
	xMin := imgCoordMungeLower(vx[0])
	xMax := imgCoordMungeUpper(vx[0])
	yMin := imgCoordMungeLower(vy[0])
	yMax := imgCoordMungeUpper(vy[0])
	for i := 1; i < 4; i++ {
		if t := imgCoordMungeLower(vx[i]); t < xMin {
			xMin = t
		}
		if t := imgCoordMungeUpper(vx[i]); t > xMax {
			xMax = t
		}
		if t := imgCoordMungeLower(vy[i]); t < yMin {
			yMin = t
		}
		if t := imgCoordMungeUpper(vy[i]); t > yMax {
			yMax = t
		}
	}
	clipRes := s.testRect(xMin, yMin, xMax-1, yMax-1)
	if clipRes == xpath.ClipAllOutside {
		return nil
	}

	// Compute scale factors as in Splash.cc:3798-3847.
	var scaledW, scaledH int
	if math.Abs(mat[0]) >= math.Abs(mat[1]) {
		scaledW = xMax - xMin
		scaledH = yMax - yMin
	} else {
		scaledW = yMax - yMin
		scaledH = xMax - xMin
	}
	if scaledH <= 1 || scaledW <= 1 {
		var t0, t1, th int
		if mat[0] >= 0 {
			t0 = imgCoordMungeUpper(mat[0]+mat[4]) - imgCoordMungeLower(mat[4])
		} else {
			t0 = imgCoordMungeUpper(mat[4]) - imgCoordMungeLower(mat[0]+mat[4])
		}
		if mat[1] >= 0 {
			t1 = imgCoordMungeUpper(mat[1]+mat[5]) - imgCoordMungeLower(mat[5])
		} else {
			t1 = imgCoordMungeUpper(mat[5]) - imgCoordMungeLower(mat[1]+mat[5])
		}
		scaledW = t0
		if t1 > scaledW {
			scaledW = t1
		}
		if mat[2] >= 0 {
			t0 = imgCoordMungeUpper(mat[2]+mat[4]) - imgCoordMungeLower(mat[4])
			if math.Abs(mat[1]) >= 1 {
				th = imgCoordMungeUpper(mat[2]) - imgCoordMungeLower(mat[0]*mat[3]/mat[1])
				if th > t0 {
					t0 = th
				}
			}
		} else {
			t0 = imgCoordMungeUpper(mat[4]) - imgCoordMungeLower(mat[2]+mat[4])
			if math.Abs(mat[1]) >= 1 {
				th = imgCoordMungeUpper(mat[0]*mat[3]/mat[1]) - imgCoordMungeLower(mat[2])
				if th > t0 {
					t0 = th
				}
			}
		}
		if mat[3] >= 0 {
			t1 = imgCoordMungeUpper(mat[3]+mat[5]) - imgCoordMungeLower(mat[5])
			if math.Abs(mat[0]) >= 1 {
				th = imgCoordMungeUpper(mat[3]) - imgCoordMungeLower(mat[1]*mat[2]/mat[0])
				if th > t1 {
					t1 = th
				}
			}
		} else {
			t1 = imgCoordMungeUpper(mat[5]) - imgCoordMungeLower(mat[3]+mat[5])
			if math.Abs(mat[0]) >= 1 {
				th = imgCoordMungeUpper(mat[1]*mat[2]/mat[0]) - imgCoordMungeLower(mat[3])
				if th > t1 {
					t1 = th
				}
			}
		}
		scaledH = t0
		if t1 > scaledH {
			scaledH = t1
		}
	}
	if scaledW == 0 {
		scaledW = 1
	}
	if scaledH == 0 {
		scaledH = 1
	}

	scaled, err := s.scaleImageWithSourceAlpha(src, srcW, srcH, scaledW, scaledH, interpolate, sourceAlpha)
	if err != nil {
		return err
	}

	// compute inverse of the post-scale 2x2.
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
	i := 0
	if vy[1] < vy[i] {
		i = 1
	}
	if vy[2] < vy[i] {
		i = 2
	}
	if vy[3] < vy[i] {
		i = 3
	}
	if math.Abs(vy[i]-vy[(i+3)&3]) <= 0.000001 && vy[(i+3)&3] < vy[(i+1)&3] {
		i = (i + 3) & 3
	}
	nSections := 1
	if math.Abs(vy[i]-vy[(i+1)&3]) <= 0.000001 {
		sections[0].y0 = imgCoordMungeLower(vy[i])
		sections[0].y1 = imgCoordMungeUpper(vy[(i+2)&3]) - 1
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
		sections[0].y0 = imgCoordMungeLower(vy[i])
		sections[2].y1 = imgCoordMungeUpper(vy[(i+2)&3]) - 1
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
			sections[1].y0 = imgCoordMungeLower(vy[(i+1)&3])
			sections[2].y0 = imgCoordMungeUpper(vy[(i+3)&3])
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
			sections[1].y0 = imgCoordMungeLower(vy[(i+3)&3])
			sections[2].y0 = imgCoordMungeUpper(vy[(i+1)&3])
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

	bpp := bytesPerPixel(s.bitmap.mode)
	hasAlpha := scaled.alpha != nil
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	var p pipe
	usesShape := hasAlpha || (s.vectorAA && clipRes != xpath.ClipAllInside)
	s.pipeInit(&p, 0, 0, nil, &Color{}, alphaIn, usesShape, false)
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

	for j := 0; j < nSections; j++ {
		sec := sections[j]
		for y := sec.y0; y <= sec.y1; y++ {
			xa := imgCoordMungeLower(sec.xa0 + (float64(y)+0.5-sec.ya0)*sec.dxdya)
			if xa < 0 {
				xa = 0
			}
			xb := imgCoordMungeUpper(sec.xb0 + (float64(y)+0.5-sec.yb0)*sec.dxdyb)
			if xa == xb {
				xb++
			}
			if clipRes == xpath.ClipAllInside && xb > s.bitmap.width {
				xb = s.bitmap.width
			}
			clipRes2 := clipRes
			if clipRes != xpath.ClipAllInside && clip != nil {
				clipRes2 = clip.TestSpan(xa, xb-1, y)
			}
			aaReady := false
			if s.vectorAA && clipRes2 != xpath.ClipAllInside && clip != nil {
				if y >= 0 && y < s.bitmap.height {
					for k := 0; k < aaLen; k++ {
						s.aaBuf[k] = 0xff
					}
					clip.ClipAALine(y, s.aaBuf, 0, s.bitmap.width-1)
					aaReady = true
				}
			}
			for x := xa; x < xb; x++ {
				if x < 0 || x >= s.bitmap.width || y < 0 || y >= s.bitmap.height {
					continue
				}
				fx := float64(x) + 0.5 - mat[4]
				fy := float64(y) + 0.5 - mat[5]
				xx := Floor(fx*ir00 + fy*ir10)
				yy := Floor(fx*ir01 + fy*ir11)
				if xx < 0 {
					xx = 0
				} else if xx >= scaledW {
					xx = scaledW - 1
				}
				if yy < 0 {
					yy = 0
				} else if yy >= scaledH {
					yy = scaledH - 1
				}
				off := (yy*scaledW + xx) * bpp
				var c Color
				readScaledPixel(scaled.data, off, scaled.mode, &c)
				shape := byte(255)
				if hasAlpha {
					shape = scaled.alpha[yy*scaledW+xx]
					unpremultiplyImageColor(&c, scaled.mode, shape)
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
				p.cSrc = c
				p.shape = shape
				s.pipeSetXY(&p, x, y)
				if shouldTraceImagePixel(x, y) {
					traceImagePixelBefore(&p, "arbitraryTransformImage", x, y, xx, yy, c, shape)
				}
				p.run(&p)
				if shouldTraceImagePixel(x, y) {
					traceImagePixelAfter(&p, "arbitraryTransformImage", x, y)
				}
			}
		}
	}
	return nil
}

var imageTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_IMAGE_TRACE"))

func shouldTraceImagePixel(x, y int) bool {
	for _, pixel := range imageTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func traceImagePixelBefore(p *pipe, op string, x, y, srcX, srcY int, c Color, shape byte) {
	if p.colorBytesPerPixel < 3 || p.destOff+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil && p.aDestOff < len(p.aDestRow) {
		aDest = p.aDestRow[p.aDestOff]
	}
	fmt.Fprintf(os.Stderr, "SPLASH_IMAGE_TRACE before op=%s x=%d y=%d srcXY=(%d,%d) src=(%d,%d,%d) shape=%d dst=(%d,%d,%d) aDest=%d\n",
		op, x, y, srcX, srcY, c[0], c[1], c[2], shape,
		p.destRow[p.destOff], p.destRow[p.destOff+1], p.destRow[p.destOff+2], aDest)
}

func traceImagePixelAfter(p *pipe, op string, x, y int) {
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
	fmt.Fprintf(os.Stderr, "SPLASH_IMAGE_TRACE after op=%s x=%d y=%d dst=(%d,%d,%d) aDest=%d\n",
		op, x, y, p.destRow[off], p.destRow[off+1], p.destRow[off+2], aDest)
}
