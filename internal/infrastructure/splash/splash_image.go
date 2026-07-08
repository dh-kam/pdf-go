package splash

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

const (
	maxPooledImageScaleLineBytes = 1 << 20
	maxPooledImageScaleUint32s   = 1 << 20
	maxPooledImageScaleRuns      = 1 << 18
)

var imageScaleBufferPool = sync.Pool{
	New: func() any {
		return &imageScaleBuffers{}
	},
}

type imageScaleBuffers struct {
	line    []byte
	pix     []uint32
	xStarts []int
	xSteps  []int
}

func acquireImageScaleBuffers(lineLen, pixLen, runLen int) *imageScaleBuffers {
	b := imageScaleBufferPool.Get().(*imageScaleBuffers)
	if cap(b.line) < lineLen {
		b.line = make([]byte, lineLen)
	} else {
		b.line = b.line[:lineLen]
	}
	if cap(b.pix) < pixLen {
		b.pix = make([]uint32, pixLen)
	} else {
		b.pix = b.pix[:pixLen]
	}
	if cap(b.xStarts) < runLen {
		b.xStarts = make([]int, runLen)
	} else {
		b.xStarts = b.xStarts[:runLen]
	}
	if cap(b.xSteps) < runLen {
		b.xSteps = make([]int, runLen)
	} else {
		b.xSteps = b.xSteps[:runLen]
	}
	return b
}

func releaseImageScaleBuffers(b *imageScaleBuffers) {
	if b == nil {
		return
	}
	if cap(b.line) > maxPooledImageScaleLineBytes {
		b.line = nil
	} else {
		b.line = b.line[:0]
	}
	if cap(b.pix) > maxPooledImageScaleUint32s {
		b.pix = nil
	} else {
		b.pix = b.pix[:0]
	}
	if cap(b.xStarts) > maxPooledImageScaleRuns {
		b.xStarts = nil
	} else {
		b.xStarts = b.xStarts[:0]
	}
	if cap(b.xSteps) > maxPooledImageScaleRuns {
		b.xSteps = nil
	} else {
		b.xSteps = b.xSteps[:0]
	}
	imageScaleBufferPool.Put(b)
}

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
	return isImageInterpolationRequiredWithOptions(srcW, srcH, dstW, dstH, interpolate, false)
}

func isImageInterpolationRequiredWithOptions(srcW, srcH, dstW, dstH int, interpolate bool, disableRequired bool) bool {
	if interpolate || srcW == 0 || srcH == 0 {
		return true
	}
	if disableRequired {
		return false
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
	return s.drawImageImplWithPostTransformOptions(src, w, h, mat, interpolate, sourceAlpha, nil, imageDrawOptions{})
}

type postScaleRGBTransform func(r, g, b byte) (byte, byte, byte)

func (s *Splash) drawImageImplWithPostTransformOptions(
	src ImageSource,
	w, h int,
	mat [6]float64,
	interpolate bool,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
	options imageDrawOptions,
) error {
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
	if debugSplashImageDrawTrace {
		fmt.Fprintf(os.Stderr, "DEBUG: drawImageImpl w=%d h=%d mat=%v interpolate=%t\n", w, h, mat, interpolate)
		fmt.Fprintf(os.Stderr, "IMGSTATE w=%d h=%d x0=%.2f y0=%.2f fillAlpha=%.4f blendSet=%t groupDepth=%d\n",
			w, h, mat[4], mat[5], s.state.fillAlpha, s.state.blendFunc != nil, len(s.groupStack))
		if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
			for gi := range s.groupStack {
				g := s.groupStack[gi]
				bm := 0
				if g.blendMode != nil {
					bm = 1
				}
				fmt.Fprintf(os.Stderr, "  GRPSTACK[%d] savedFA=%.4f savedBlendSet=%t isolated=%t\n", gi, g.savedFillAlpha, bm != 0, g.isolated)
			}
		}
	}
	// singular-matrix check (Splash.cc:3541).
	det := mat[0]*mat[3] - mat[1]*mat[2]
	if math.Abs(det) < 1e-6 {
		return ErrSingularMatrix
	}

	minorAxisZero := mat[1] == 0 && mat[2] == 0
	axisAlignedScale := mat[0] > 0 && minorAxisZero && mat[3] != 0
	// Axis-aligned interpolated images can use the scale-only paths below
	// instead of buffering the full source image in the affine rasterizer.
	if interpolate && !axisAlignedScale {
		return s.arbitraryTransformImage(src, w, h, mat, interpolate, sourceAlpha, postTransform)
	}

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
		dstW := x1 - x0
		dstH := y1 - y0
		if debugSplashImageDrawTrace {
			fmt.Fprintf(os.Stderr, "drawImage (scaling only): mat=%v x0=%d y0=%d x1=%d y1=%d dstW=%d dstH=%d interpolate=%v\n", mat, x0, y0, x1, y1, dstW, dstH, interpolate)
		}
		if isIntegerAligned2xDownscaleNoFlip(mat, w, h) {
			dstW := w / 2
			dstH := h / 2
			x0 := int(math.Round(mat[4]))
			y0 := int(math.Round(mat[5]))
			clipRes := s.testRect(x0, y0, x0+dstW-1, y0+dstH-1)
			if clipRes == xpath.ClipAllOutside {
				return nil
			}
			return s.drawIntegerAligned2xDownscale(src, w, h, dstW, dstH, x0, y0, clipRes, sourceAlpha, postTransform)
		}
		clipRes := s.testRect(x0, y0, x1-1, y1-1)
		if clipRes == xpath.ClipAllOutside {
			return nil
		}
		if s.canDrawYdownXdownDirect(w, h, dstW, dstH, interpolate, sourceAlpha, postTransform, clipRes) {
			return s.drawYdownXdownDirect(src, w, h, dstW, dstH, x0, y0, clipRes, options.popplerFixedScaleAverage)
		}
		scaled, err := s.scaleImageWithSourceAlphaOptions(src, w, h, dstW, dstH, interpolate, sourceAlpha, options)
		if err != nil {
			return err
		}
		s.applyPostScaleRGBTransform(scaled, postTransform)
		return s.blitImage(scaled, x0, y0, clipRes)
	}

	// scaling plus vertical flip (Splash.cc:3581).
	if mat[0] > 0 && minorAxisZero && mat[3] < 0 {
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
		dstW := x1 - x0
		dstH := y1 - y0
		if debugSplashImageDrawTrace {
			fmt.Fprintf(os.Stderr, "drawImage (scaling+vflip): mat=%v x0=%d y0=%d x1=%d y1=%d dstW=%d dstH=%d interpolate=%v\n", mat, x0, y0, x1, y1, dstW, dstH, interpolate)
		}
		forcePopplerScaleThenFlip := s.forceScaleThenFlip || options.forceScaleThenFlip ||
			debugSplashDisableTopdownVFlipScale
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
		if !forcePopplerScaleThenFlip && isIntegerAligned2xDownscale(mat, w, h) {
			dstW := w / 2
			dstH := h / 2
			x0 := int(math.Round(mat[4]))
			y0 := int(math.Round(mat[3] + mat[5]))
			clipRes := s.testRect(x0, y0, x0+dstW-1, y0+dstH-1)
			if clipRes == xpath.ClipAllOutside {
				return nil
			}
			return s.drawIntegerAligned2xDownscaleVFlip(src, w, h, dstW, dstH, x0, y0, clipRes, sourceAlpha, postTransform)
		}
		clipRes := s.testRect(x0, y0, x1-1, y1-1)
		if clipRes == xpath.ClipAllOutside {
			return nil
		}
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
		if !forcePopplerScaleThenFlip && dstW >= w && dstH >= h &&
			isImageInterpolationRequiredWithOptions(w, h, dstW, dstH, interpolate, shouldDisableRequiredInterpolationForImage(options, w, h)) {
			topDownSrc := func(row int, color, alpha []byte) error {
				// Closure delivers row k as stdlib row (srcH-1-k); rewrap so
				// row k → stdlib row k (top-down).
				return src(h-1-row, color, alpha)
			}
			if s.canDrawYupXupBilinearDirect(w, h, dstW, dstH, sourceAlpha, postTransform, clipRes) {
				return s.drawYupXupBilinearDirect(topDownSrc, w, h, dstW, dstH, x0, y0, options)
			}
			scaled := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
			if scaled.data == nil {
				return ErrZeroImage
			}
			if err := s.scaleImageYupXupBilinearWithOptions(topDownSrc, w, h, dstW, dstH, scaled, options); err != nil {
				return err
			}
			s.applyPostScaleRGBTransform(scaled, postTransform)
			return s.blitImage(scaled, x0, y0, clipRes)
		}
		if !forcePopplerScaleThenFlip && dstW >= w && dstH >= h {
			topDownSrc := func(row int, color, alpha []byte) error {
				return src(h-1-row, color, alpha)
			}
			if s.canDrawCenterMappedNearestDirect(w, h, dstW, dstH, sourceAlpha, options, clipRes) {
				return s.drawCenterMappedNearestDirect(topDownSrc, w, h, dstW, dstH, x0, y0, clipRes, postTransform)
			}
			if s.canDrawYupXupDirect(w, h, dstW, dstH, sourceAlpha, postTransform, clipRes, options) {
				return s.drawYupXupDirect(topDownSrc, w, h, dstW, dstH, x0, y0, clipRes)
			}
			scaled := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
			if scaled.data == nil {
				return ErrZeroImage
			}
			if err := s.scaleImageYupXupWithOptions(topDownSrc, w, h, dstW, dstH, scaled, options); err != nil {
				return err
			}
			s.applyPostScaleRGBTransform(scaled, postTransform)
			return s.blitImage(scaled, x0, y0, clipRes)
		}
		// The backend ImageSource already presents regular PDF images in
		// top-down display order. For Y-down downscales, scaling that stream
		// directly matches Poppler's row grouping; scaling then vert-flipping
		// shifts high-resolution Flate images such as GeoTopo p31.
		disableTopDownDownscale := options.disableTopDownDownscale ||
			debugSplashDisableTopdownDownscale
		if !forcePopplerScaleThenFlip && (s.downscaleVFlipTopDown || (!sourceAlpha && !disableTopDownDownscale)) && dstW < w && dstH < h {
			topDownSrc := func(row int, color, alpha []byte) error {
				return src(h-1-row, color, alpha)
			}
			if s.canDrawYdownXdownDirect(w, h, dstW, dstH, interpolate, sourceAlpha, postTransform, clipRes) {
				return s.drawYdownXdownDirect(topDownSrc, w, h, dstW, dstH, x0, y0, clipRes, options.popplerFixedScaleAverage)
			}
			scaled := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
			if scaled.data == nil {
				return ErrZeroImage
			}
			if err := s.scaleImageYdownXdown(topDownSrc, w, h, dstW, dstH, scaled, options.popplerFixedScaleAverage); err != nil {
				return err
			}
			s.applyPostScaleRGBTransform(scaled, postTransform)
			return s.blitImage(scaled, x0, y0, clipRes)
		}
		if s.canDrawYupXdownVFlipDirect(w, h, dstW, dstH, interpolate, sourceAlpha, postTransform, clipRes) {
			return s.drawYupXdownVFlipDirect(src, w, h, dstW, dstH, x0, y0, clipRes, options.popplerFixedScaleAverage)
		}
		if s.canDrawYdownXdownScaleThenVFlipDirect(w, h, dstW, dstH, interpolate, sourceAlpha, clipRes) {
			return s.drawYdownXdownScaleThenVFlipDirect(src, w, h, dstW, dstH, x0, y0, clipRes, options.popplerFixedScaleAverage, postTransform)
		}
		scaled, err := s.scaleImageWithSourceAlphaOptions(src, w, h, dstW, dstH, interpolate, sourceAlpha, options)
		if err != nil {
			return err
		}
		s.applyPostScaleRGBTransform(scaled, postTransform)
		vertFlipBitmap(scaled, nComps)
		return s.blitImage(scaled, x0, y0, clipRes)
	}

	// general affine (Splash.cc:3623).
	return s.arbitraryTransformImage(src, w, h, mat, interpolate, sourceAlpha, postTransform)
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

func isIntegerAligned2xDownscaleNoFlip(mat [6]float64, w, h int) bool {
	if w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 {
		return false
	}
	dstW := float64(w / 2)
	dstH := float64(h / 2)
	if mat[0] != dstW || mat[3] != dstH {
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
	src ImageSource,
	srcW, srcH, dstW, dstH, dstX, dstY int,
	clipRes xpath.ClipResult,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
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

	s.applyPostScaleRGBTransform(scaled, postTransform)
	return s.blitImage(scaled, dstX, dstY, clipRes)
}

func (s *Splash) drawIntegerAligned2xDownscale(
	src ImageSource,
	srcW, srcH, dstW, dstH, dstX, dstY int,
	clipRes xpath.ClipResult,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := sourceAlpha

	rows := make([][]byte, srcH)
	var alphaRows [][]byte
	if hasAlpha {
		alphaRows = make([][]byte, srcH)
	}
	for rowIdx := 0; rowIdx < srcH; rowIdx++ {
		row := make([]byte, srcW*nComps)
		var alpha []byte
		if hasAlpha {
			alpha = make([]byte, srcW)
		}
		if err := src(rowIdx, row, alpha); err != nil {
			return err
		}
		rows[rowIdx] = row
		if hasAlpha {
			alphaRows[rowIdx] = alpha
		}
	}

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

	s.applyPostScaleRGBTransform(scaled, postTransform)
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
	return s.scaleImageWithSourceAlphaOptions(src, srcW, srcH, dstW, dstH, interpolate, sourceAlpha, imageDrawOptions{})
}

func (s *Splash) scaleImageWithSourceAlphaOptions(src ImageSource, srcW, srcH, dstW, dstH int, interpolate bool, sourceAlpha bool, options imageDrawOptions) (*Bitmap, error) {
	dest := NewBitmap(dstW, dstH, s.bitmap.mode, sourceAlpha)
	if dest.data == nil || srcW <= 0 || srcH <= 0 {
		return nil, ErrZeroImage
	}
	var err error
	// Match Poppler's Splash::scaleImage dispatch (Splash.cc:3962-3977): a downscale
	// (scaledHeight < srcHeight) ALWAYS uses the box-average scaler regardless of the
	// interpolate flag; interpolate is consulted only in the pure-upscale case via
	// isImageInterpolationRequired. Routing interpolate=true downscales to bilinear
	// (the old leading `if interpolate` branch) diverges from Poppler.
	if dstH < srcH {
		if dstW < srcW {
			err = s.scaleImageYdownXdown(src, srcW, srcH, dstW, dstH, dest, options.popplerFixedScaleAverage)
		} else {
			err = s.scaleImageYdownXup(src, srcW, srcH, dstW, dstH, dest, options.popplerFixedScaleAverage)
		}
	} else {
		if dstW < srcW {
			err = s.scaleImageYupXdown(src, srcW, srcH, dstW, dstH, dest, options.popplerFixedScaleAverage)
		} else {
			if isImageInterpolationRequiredWithOptions(srcW, srcH, dstW, dstH, interpolate, shouldDisableRequiredInterpolationForImage(options, srcW, srcH)) {
				err = s.scaleImageYupXupBilinearWithOptions(src, srcW, srcH, dstW, dstH, dest, options)
			} else {
				err = s.scaleImageYupXupWithOptions(src, srcW, srcH, dstW, dstH, dest, options)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return dest, nil
}

func popplerScaleFixedDivisor(denom int) uint32 {
	if denom <= 0 {
		return 0
	}
	return uint32((1 << 23) / denom)
}

func popplerScaleFixedAverage(sum uint32, divisor uint32) uint32 {
	return (sum * divisor) >> 23
}

func splashScaleAverage(sum uint32, denom int, usePopplerFixed bool) uint32 {
	if denom <= 0 {
		return 0
	}
	// Always use Poppler's fixed-point averaging for consistency with C FreeType.
	// The old integer division (sum/denom) produces different results than
	// Poppler's (sum * ((1<<23)/denom)) >> 23 approach.
	return popplerScaleFixedAverage(sum, popplerScaleFixedDivisor(denom))
}

func shouldDisableRequiredInterpolationForImage(options imageDrawOptions, srcW, srcH int) bool {
	if !options.disableRequiredInterpolation {
		return false
	}
	// Thin image strips still match Poppler better through Splash's required
	// interpolation fallback; large explicit-nearest surfaces match sparse
	// overlay PDFs better when the explicit nearest request is honored.
	return srcW >= 300 && srcH >= 200
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
func (s *Splash) scaleImageYdownXdown(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap, usePopplerFixedAverage bool) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil

	yp := srcH / dstH
	yq := srcH % dstH
	xp := srcW / dstW
	xq := srcW % dstW

	buffers := acquireImageScaleBuffers(srcW*nComps, srcW*nComps, 0)
	defer releaseImageScaleBuffers(buffers)
	lineBuf := buffers.line
	pixBuf := buffers.pix
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
		srcY0 := rowIdx

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
		xx := 0
		xxa := 0
		for x := 0; x < dstW; x++ {
			var xStep int
			xt += xq
			if xt >= dstW {
				xt -= dstW
				xStep = xp + 1
			} else {
				xStep = xp
			}
			srcX0 := xx / nComps
			denom := xStep * yStep

			var pix [splashMaxColorComps]uint32
			for i := 0; i < xStep; i++ {
				for c := 0; c < nComps; c++ {
					pix[c] += pixBuf[xx+c]
				}
				xx += nComps
			}
			rawPix := pix
			for c := 0; c < nComps; c++ {
				pix[c] = splashScaleAverage(pix[c], denom, usePopplerFixedAverage)
			}
			if imageScaleTracePixelsEnabled && s.shouldTraceImageScalePixel(x, y) {
				fmt.Fprintf(os.Stderr,
					"SPLASH_SCALE_TRACE op=scaleImageYdownXdown dst=(%d,%d) srcX=[%d,%d] srcY=[%d,%d] src=%dx%d dstSize=%dx%d step=(%d,%d) bresenham=(xp=%d xq=%d yp=%d yq=%d xt=%d yt=%d denom=%d) raw=(%d,%d,%d) out=(%d,%d,%d)%s\n",
					x, y, srcX0, srcX0+xStep-1, srcY0, srcY0+yStep-1,
					srcW, srcH, dstW, dstH, xStep, yStep, xp, xq, yp, yq, xt, yt, denom,
					rawPix[0], rawPix[1], rawPix[2], pix[0], pix[1], pix[2],
					imageTraceContextForSplash(s))
			}
			storeScaledPixel(dest.data, &destOff, dest.mode, pix[:])

			if hasAlpha {
				var a uint32
				for i := 0; i < xStep; i++ {
					a += alphaPix[xxa]
					xxa++
				}
				a = splashScaleAverage(a, denom, usePopplerFixedAverage)
				dest.alpha[destAlphaOff] = byte(a)
				destAlphaOff++
			}
		}
	}
	return nil
}

func (s *Splash) canDrawYdownXdownDirect(
	srcW, srcH, dstW, dstH int,
	interpolate bool,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
	clipRes xpath.ClipResult,
) bool {
	return s != nil &&
		s.bitmap != nil &&
		s.bitmap.mode == ModeRGB8 &&
		!interpolate &&
		!sourceAlpha &&
		postTransform == nil &&
		clipRes != xpath.ClipAllOutside &&
		dstW > 0 &&
		dstH > 0 &&
		dstW < srcW &&
		dstH < srcH
}

func (s *Splash) drawYdownXdownDirect(
	src ImageSource,
	srcW, srcH, dstW, dstH int,
	xDest, yDest int,
	clipRes xpath.ClipResult,
	usePopplerFixedAverage bool,
) error {
	return s.drawYdownXdownDirectWithOptions(src, srcW, srcH, dstW, dstH, xDest, yDest, clipRes, usePopplerFixedAverage, nil, false, "drawYdownXdownDirect")
}

func (s *Splash) canDrawYdownXdownScaleThenVFlipDirect(
	srcW, srcH, dstW, dstH int,
	interpolate bool,
	sourceAlpha bool,
	clipRes xpath.ClipResult,
) bool {
	return s != nil &&
		s.bitmap != nil &&
		s.bitmap.mode == ModeRGB8 &&
		!interpolate &&
		!sourceAlpha &&
		clipRes != xpath.ClipAllOutside &&
		dstW > 0 &&
		dstH > 0 &&
		dstW < srcW &&
		dstH < srcH
}

func (s *Splash) drawYdownXdownScaleThenVFlipDirect(
	src ImageSource,
	srcW, srcH, dstW, dstH int,
	xDest, yDest int,
	clipRes xpath.ClipResult,
	usePopplerFixedAverage bool,
	postTransform postScaleRGBTransform,
) error {
	return s.drawYdownXdownDirectWithOptions(src, srcW, srcH, dstW, dstH, xDest, yDest, clipRes, usePopplerFixedAverage, postTransform, true, "drawYdownXdownScaleThenVFlipDirect")
}

func (s *Splash) drawYdownXdownDirectWithOptions(
	src ImageSource,
	srcW, srcH, dstW, dstH int,
	xDest, yDest int,
	clipRes xpath.ClipResult,
	usePopplerFixedAverage bool,
	postTransform postScaleRGBTransform,
	vFlip bool,
	traceOp string,
) error {
	const nComps = 3
	yp := srcH / dstH
	yq := srcH % dstH

	buffers := acquireImageScaleBuffers(srcW*nComps, dstW*nComps, dstW)
	lineBuf := buffers.line
	pixBuf := buffers.pix
	xStarts := buffers.xStarts
	xSteps := buffers.xSteps
	fillXDownRuns(xStarts, xSteps, srcW, dstW)

	alphaIn := byte(Round(s.state.fillAlpha * 255))
	p := imagePipePool.Get().(*pipe)
	clip, _ := s.state.clip.(*xpath.Clip)
	useClipAA := clipRes != xpath.ClipAllInside && s.vectorAA && clip != nil
	s.pipeInit(p, xDest, yDest, nil, &Color{}, alphaIn, useClipAA, false)
	rowSize := 0
	aaLen := 0
	if useClipAA {
		rowSize = (s.bitmap.width*splashAASize + 7) >> 3
		aaLen = rowSize * splashAASize
		if len(s.aaBuf) < aaLen {
			s.aaBuf = make([]byte, aaLen)
		}
	}

	yt := 0
	rowIdx := 0
	for y := 0; y < dstH; y++ {
		var yStep int
		yt += yq
		if yt >= dstH {
			yt -= dstH
			yStep = yp + 1
		} else {
			yStep = yp
		}
		srcY0 := rowIdx

		for j := range pixBuf {
			pixBuf[j] = 0
		}
		for i := 0; i < yStep; i++ {
			if err := s.readImageRow(src, srcW, lineBuf, nil, rowIdx); err != nil {
				*p = pipe{}
				imagePipePool.Put(p)
				releaseImageScaleBuffers(buffers)
				return err
			}
			rowIdx++
			for x, xStep := range xSteps {
				srcOff := xStarts[x] * nComps
				pixOff := x * nComps
				for col := 0; col < xStep; col++ {
					pixBuf[pixOff] += uint32(lineBuf[srcOff])
					pixBuf[pixOff+1] += uint32(lineBuf[srcOff+1])
					pixBuf[pixOff+2] += uint32(lineBuf[srcOff+2])
					srcOff += nComps
				}
			}
		}

		dy := yDest + y
		if vFlip {
			dy = yDest + dstH - 1 - y
		}
		if dy < 0 || dy >= s.bitmap.height {
			continue
		}
		if useClipAA {
			fillBitmapBytes(s.aaBuf[:aaLen], 0xff)
			clip.ClipAALine(dy, s.aaBuf, 0, s.bitmap.width-1)
		}
		if clipRes == xpath.ClipAllInside {
			s.pipeSetXY(p, xDest, dy)
		}
		for x, xStep := range xSteps {
			denom := xStep * yStep
			pixOff := x * nComps
			var c Color
			c[0] = byte(splashScaleAverage(pixBuf[pixOff], denom, usePopplerFixedAverage))
			c[1] = byte(splashScaleAverage(pixBuf[pixOff+1], denom, usePopplerFixedAverage))
			c[2] = byte(splashScaleAverage(pixBuf[pixOff+2], denom, usePopplerFixedAverage))
			if postTransform != nil {
				c[0], c[1], c[2] = postTransform(c[0], c[1], c[2])
			}

			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width {
				continue
			}
			shape := byte(255)
			if clipRes != xpath.ClipAllInside && clip != nil {
				if useClipAA {
					t := s.aaCoverageAt(dx, rowSize)
					t = adjustClippedImageLowAACoverageForDebug(t)
					if t == 0 {
						continue
					}
					shape = byte(Div255(int(s.aaGamma[t]) * 255))
					if shape == 0 {
						continue
					}
					p.shape = shape
				} else if !clip.Test(dx, dy) {
					// Poppler's non-AA clipped image blit uses the per-pixel
					// point test (state->clip->test, Splash.cc:5010/5027) —
					// TestSpan can never report AllOutside from a path
					// scanner, so span-culling silently ignored path clips.
					continue
				}
			}
			p.cSrc = c
			if clipRes != xpath.ClipAllInside {
				s.pipeSetXY(p, dx, dy)
			}
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelBefore(p, traceOp, dx, dy, xStarts[x], srcY0, c, shape)
			}
			p.run(p)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelAfter(p, traceOp, dx, dy)
			}
		}
	}
	*p = pipe{}
	imagePipePool.Put(p)
	releaseImageScaleBuffers(buffers)
	return nil
}

func (s *Splash) canDrawYupXdownVFlipDirect(
	srcW, srcH, dstW, dstH int,
	interpolate bool,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
	clipRes xpath.ClipResult,
) bool {
	return s != nil &&
		s.bitmap != nil &&
		s.bitmap.mode == ModeRGB8 &&
		!interpolate &&
		!sourceAlpha &&
		postTransform == nil &&
		clipRes != xpath.ClipAllOutside &&
		srcW > 0 &&
		srcH > 0 &&
		dstW > 0 &&
		dstH > 0 &&
		dstW < srcW &&
		dstH >= srcH
}

func (s *Splash) drawYupXdownVFlipDirect(
	src ImageSource,
	srcW, srcH, dstW, dstH, xDest, yDest int,
	clipRes xpath.ClipResult,
	usePopplerFixedAverage bool,
) error {
	xStart := make([]int, dstW)
	xStep := make([]int, dstW)
	fillXDownRuns(xStart, xStep, srcW, dstW)

	yMap := make([]int, dstH)
	fillYupVFlipMap(yMap, srcH, dstH)

	lineBuf := make([]byte, srcW*3)
	rowBuf := make([]byte, dstW*3)
	lastSrcY := -1
	readLine := func(scaledY int) (int, error) {
		srcY := yMap[scaledY]
		if srcY == lastSrcY {
			return srcY, nil
		}
		if err := s.readImageRow(src, srcW, lineBuf, nil, srcY); err != nil {
			return srcY, err
		}
		for x := 0; x < dstW; x++ {
			start := xStart[x] * 3
			step := xStep[x]
			var r, g, b uint32
			for i := 0; i < step; i++ {
				off := start + i*3
				r += uint32(lineBuf[off])
				g += uint32(lineBuf[off+1])
				b += uint32(lineBuf[off+2])
			}
			out := x * 3
			rowBuf[out] = byte(splashScaleAverage(r, step, usePopplerFixedAverage))
			rowBuf[out+1] = byte(splashScaleAverage(g, step, usePopplerFixedAverage))
			rowBuf[out+2] = byte(splashScaleAverage(b, step, usePopplerFixedAverage))
		}
		lastSrcY = srcY
		return srcY, nil
	}
	sample := func(x int) Color {
		off := x * 3
		return Color{rowBuf[off], rowBuf[off+1], rowBuf[off+2]}
	}
	return s.blitMappedRGBDirect(dstW, dstH, xDest, yDest, clipRes, readLine, sample, xStart)
}

func fillXDownRuns(xStart, xStep []int, srcW, dstW int) {
	xp := srcW / dstW
	xq := srcW % dstW
	xt := 0
	xx := 0
	for x := 0; x < dstW; x++ {
		step := xp
		xt += xq
		if xt >= dstW {
			xt -= dstW
			step++
		}
		xStart[x] = xx
		xStep[x] = step
		xx += step
	}
}

func fillYupVFlipMap(dstToSrc []int, srcH, dstH int) {
	yp := dstH / srcH
	yq := dstH % srcH
	yt := 0
	rawY := 0
	for srcY := 0; srcY < srcH && rawY < dstH; srcY++ {
		step := yp
		yt += yq
		if yt >= srcH {
			yt -= srcH
			step++
		}
		for i := 0; i < step && rawY < dstH; i++ {
			dstToSrc[dstH-1-rawY] = srcY
			rawY++
		}
	}
	for rawY < dstH {
		dstToSrc[dstH-1-rawY] = srcH - 1
		rawY++
	}
}

func (s *Splash) canDrawYupXupDirect(
	srcW, srcH, dstW, dstH int,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
	clipRes xpath.ClipResult,
	options imageDrawOptions,
) bool {
	return s != nil &&
		s.bitmap != nil &&
		s.bitmap.mode == ModeRGB8 &&
		!sourceAlpha &&
		postTransform == nil &&
		clipRes == xpath.ClipAllInside &&
		srcW > 0 &&
		srcH > 0 &&
		dstW > 0 &&
		dstH > 0 &&
		dstW >= srcW &&
		dstH >= srcH &&
		!shouldUseCenterMappedNearestUpscaleForImage(options, srcW, srcH)
}

func (s *Splash) drawYupXupDirect(
	src ImageSource,
	srcW, srcH, dstW, dstH, xDest, yDest int,
	clipRes xpath.ClipResult,
) error {
	xMap := make([]int, dstW)
	fillNearestUpscaleMap(xMap, srcW, dstW, false, 0)
	yMap := make([]int, dstH)
	fillNearestUpscaleMap(yMap, srcH, dstH, false, 0)

	lineBuf := make([]byte, srcW*3)
	lastSrcY := -1
	readLine := func(scaledY int) (int, error) {
		srcY := yMap[scaledY]
		if srcY == lastSrcY {
			return srcY, nil
		}
		if err := s.readImageRow(src, srcW, lineBuf, nil, srcY); err != nil {
			return srcY, err
		}
		lastSrcY = srcY
		return srcY, nil
	}
	sample := func(x int) Color {
		off := xMap[x] * 3
		return Color{lineBuf[off], lineBuf[off+1], lineBuf[off+2]}
	}
	return s.blitMappedRGBDirect(dstW, dstH, xDest, yDest, clipRes, readLine, sample, xMap)
}

func (s *Splash) canDrawYupXupBilinearDirect(
	srcW, srcH, dstW, dstH int,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
	clipRes xpath.ClipResult,
) bool {
	return s != nil &&
		s.bitmap != nil &&
		s.bitmap.mode == ModeRGB8 &&
		!sourceAlpha &&
		postTransform == nil &&
		// The direct blit performs no clip tests at all, so it is only valid
		// when the destination rect is entirely inside the clip. Partial
		// clips (e.g. a sub-pixel sliver path clip) must fall through to
		// scaleImage+blitImage, whose clipped blitters honor path scanners.
		clipRes == xpath.ClipAllInside &&
		srcW > 0 &&
		srcH > 0 &&
		dstW > 0 &&
		dstH > 0 &&
		dstW >= srcW &&
		dstH >= srcH
}

func (s *Splash) drawYupXupBilinearDirect(
	src ImageSource,
	srcW, srcH, dstW, dstH, xDest, yDest int,
	options imageDrawOptions,
) error {
	const nComps = 3
	srcBuf := make([]byte, (srcW+1)*nComps)
	lineBuf1 := make([]byte, dstW*nComps)
	lineBuf2 := make([]byte, dstW*nComps)
	// Poppler origin phase (see scaleImageYupXupBilinearWithOptions).
	expandPlan := newPopplerOriginExpandRowPlan(srcW, dstW)

	yStep := float64(srcH) / float64(dstH)
	ySrc := 0.0
	currentSrcRow := -1
	rowIdx := 0

	if err := s.readImageRow(src, srcW, srcBuf[:srcW*nComps], nil, rowIdx); err != nil {
		return err
	}
	rowIdx++
	expandRowWithPlan(srcBuf, lineBuf2, nComps, expandPlan)

	alphaIn := byte(Round(s.state.fillAlpha * 255))
	p := imagePipePool.Get().(*pipe)
	s.pipeInit(p, xDest, yDest, nil, &Color{}, alphaIn, false, false)
	defer func() {
		*p = pipe{}
		imagePipePool.Put(p)
	}()

	for y := 0; y < dstH; y++ {
		ySrcClamped := ySrc
		if ySrcClamped < 0 {
			ySrcClamped = 0
		}
		yInt, yFrac := math.Modf(ySrcClamped)
		for int(yInt) > currentSrcRow {
			currentSrcRow++
			copy(lineBuf1, lineBuf2)
			if currentSrcRow < srcH-1 {
				if err := s.readImageRow(src, srcW, srcBuf[:srcW*nComps], nil, rowIdx); err != nil {
					return err
				}
				rowIdx++
				expandRowWithPlan(srcBuf, lineBuf2, nComps, expandPlan)
			}
		}

		yWeight0 := 1.0 - yFrac
		s.pipeSetXY(p, xDest, yDest+y)
		srcOff := 0
		for x := 0; x < dstW; x++ {
			c := Color{
				byte(float64(lineBuf1[srcOff])*yWeight0 + float64(lineBuf2[srcOff])*yFrac),
				byte(float64(lineBuf1[srcOff+1])*yWeight0 + float64(lineBuf2[srcOff+1])*yFrac),
				byte(float64(lineBuf1[srcOff+2])*yWeight0 + float64(lineBuf2[srcOff+2])*yFrac),
			}
			dx := xDest + x
			dy := yDest + y
			p.cSrc = c
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelBefore(p, "drawYupXupBilinearDirect", dx, dy, x, y, c, 255)
			}
			p.run(p)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelAfter(p, "drawYupXupBilinearDirect", dx, dy)
			}
			srcOff += nComps
		}
		ySrc += yStep
	}
	return nil
}

func (s *Splash) blitMappedRGBDirect(
	dstW, dstH, xDest, yDest int,
	clipRes xpath.ClipResult,
	readLine func(int) (int, error),
	sample func(int) Color,
	xMap []int,
) error {
	x0, y0 := 0, 0
	x1, y1 := dstW, dstH
	clip, _ := s.state.clip.(*xpath.Clip)
	if clipRes != xpath.ClipAllInside {
		if clip != nil && clip.HasPathClip() {
			x0, x1 = dstW, dstW
			y0, y1 = dstH, dstH
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
		if err := s.blitCenterMappedNearestDirectRect(x0, y0, x1, y1, xDest, yDest, readLine, sample, xMap); err != nil {
			return err
		}
	}

	if clipRes == xpath.ClipAllInside {
		return nil
	}
	if y0 > 0 {
		if err := s.blitCenterMappedNearestDirectClipped(0, 0, xDest, yDest, dstW, y0, readLine, sample, xMap); err != nil {
			return err
		}
	}
	if y1 < dstH {
		if err := s.blitCenterMappedNearestDirectClipped(0, y1, xDest, yDest+y1, dstW, dstH-y1, readLine, sample, xMap); err != nil {
			return err
		}
	}
	if x0 > 0 && y0 < y1 {
		if err := s.blitCenterMappedNearestDirectClipped(0, y0, xDest, yDest+y0, x0, y1-y0, readLine, sample, xMap); err != nil {
			return err
		}
	}
	if x1 < dstW && y0 < y1 {
		if err := s.blitCenterMappedNearestDirectClipped(x1, y0, xDest+x1, yDest+y0, dstW-x1, y1-y0, readLine, sample, xMap); err != nil {
			return err
		}
	}
	return nil
}

// scaleImageYdownXup — Y shrinks, X grows (Splash.cc:4230).
func (s *Splash) scaleImageYdownXup(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap, usePopplerFixedAverage bool) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil

	yp := srcH / dstH
	yq := srcH % dstH
	xp := dstW / srcW
	xq := dstW % srcW

	buffers := acquireImageScaleBuffers(srcW*nComps, srcW*nComps, 0)
	defer releaseImageScaleBuffers(buffers)
	lineBuf := buffers.line
	pixBuf := buffers.pix
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
				pix[c] = splashScaleAverage(pixBuf[x*nComps+c], yStep, usePopplerFixedAverage)
			}
			for i := 0; i < xStep; i++ {
				storeScaledPixel(dest.data, &destOff, dest.mode, pix[:])
			}
			if hasAlpha {
				a := splashScaleAverage(alphaPix[x], yStep, usePopplerFixedAverage)
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
func (s *Splash) scaleImageYupXdown(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap, usePopplerFixedAverage bool) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil
	bpp := bytesPerPixel(dest.mode)

	yp := dstH / srcH
	yq := dstH % srcH
	xp := srcW / dstW
	xq := srcW % dstW

	buffers := acquireImageScaleBuffers(srcW*nComps, 0, 0)
	defer releaseImageScaleBuffers(buffers)
	lineBuf := buffers.line
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
		xxa := 0
		for x := 0; x < dstW; x++ {
			var xStep int
			xt += xq
			if xt >= dstW {
				xt -= dstW
				xStep = xp + 1
			} else {
				xStep = xp
			}

			var pix [splashMaxColorComps]uint32
			for i := 0; i < xStep; i++ {
				for c := 0; c < nComps; c++ {
					pix[c] += uint32(lineBuf[xx])
					xx++
				}
			}
			for c := 0; c < nComps; c++ {
				pix[c] = splashScaleAverage(pix[c], xStep, usePopplerFixedAverage)
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
				a = splashScaleAverage(a, xStep, usePopplerFixedAverage)
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

	buffers := acquireImageScaleBuffers(srcW*nComps, 0, 0)
	defer releaseImageScaleBuffers(buffers)
	lineBuf := buffers.line
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

func (s *Splash) scaleImageYupXupWithOptions(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap, options imageDrawOptions) error {
	if shouldUseCenterMappedNearestUpscaleForImage(options, srcW, srcH) {
		return s.scaleImageYupXupCenterMapped(src, srcW, srcH, dstW, dstH, dest)
	}
	return s.scaleImageYupXup(src, srcW, srcH, dstW, dstH, dest)
}

func shouldUseCenterMappedNearestUpscaleForImage(options imageDrawOptions, srcW, srcH int) bool {
	if !options.centerMappedNearestUpscale {
		return false
	}
	return srcW >= 300 && srcH >= 200
}

func (s *Splash) canDrawCenterMappedNearestDirect(srcW, srcH, dstW, dstH int, sourceAlpha bool, options imageDrawOptions, clipRes xpath.ClipResult) bool {
	return s != nil &&
		s.bitmap != nil &&
		s.bitmap.mode == ModeRGB8 &&
		!sourceAlpha &&
		clipRes != xpath.ClipAllOutside &&
		dstW >= srcW &&
		dstH >= srcH &&
		shouldUseCenterMappedNearestUpscaleForImage(options, srcW, srcH) &&
		!isImageInterpolationRequiredWithOptions(srcW, srcH, dstW, dstH, false, shouldDisableRequiredInterpolationForImage(options, srcW, srcH))
}

func (s *Splash) drawCenterMappedNearestDirect(
	src ImageSource,
	srcW, srcH, dstW, dstH, xDest, yDest int,
	clipRes xpath.ClipResult,
	postTransform postScaleRGBTransform,
) error {
	if dstW <= 0 || dstH <= 0 || srcW <= 0 || srcH <= 0 {
		return ErrZeroImage
	}
	lineBuf := make([]byte, srcW*3)
	xMap := make([]int, dstW)
	fillNearestUpscaleMap(xMap, srcW, dstW, true, 0)

	lastSrcY := -1
	readLine := func(y int) (int, error) {
		srcY := centerMappedNearestSourceIndex(y, srcH, dstH)
		if srcY != lastSrcY {
			if err := s.readImageRow(src, srcW, lineBuf, nil, srcY); err != nil {
				return srcY, err
			}
			lastSrcY = srcY
		}
		return srcY, nil
	}
	sample := func(x int) Color {
		srcOff := xMap[x] * 3
		c := Color{lineBuf[srcOff], lineBuf[srcOff+1], lineBuf[srcOff+2]}
		if postTransform != nil {
			c[0], c[1], c[2] = postTransform(c[0], c[1], c[2])
		}
		return c
	}

	x0, y0 := 0, 0
	x1, y1 := dstW, dstH
	clip, _ := s.state.clip.(*xpath.Clip)
	if clipRes != xpath.ClipAllInside {
		if clip != nil && clip.HasPathClip() {
			x0, x1 = dstW, dstW
			y0, y1 = dstH, dstH
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
		if err := s.blitCenterMappedNearestDirectRect(x0, y0, x1, y1, xDest, yDest, readLine, sample, xMap); err != nil {
			return err
		}
	}

	if clipRes == xpath.ClipAllInside {
		return nil
	}
	if y0 > 0 {
		if err := s.blitCenterMappedNearestDirectClipped(0, 0, xDest, yDest, dstW, y0, readLine, sample, xMap); err != nil {
			return err
		}
	}
	if y1 < dstH {
		if err := s.blitCenterMappedNearestDirectClipped(0, y1, xDest, yDest+y1, dstW, dstH-y1, readLine, sample, xMap); err != nil {
			return err
		}
	}
	if x0 > 0 && y0 < y1 {
		if err := s.blitCenterMappedNearestDirectClipped(0, y0, xDest, yDest+y0, x0, y1-y0, readLine, sample, xMap); err != nil {
			return err
		}
	}
	if x1 < dstW && y0 < y1 {
		if err := s.blitCenterMappedNearestDirectClipped(x1, y0, xDest+x1, yDest+y0, dstW-x1, y1-y0, readLine, sample, xMap); err != nil {
			return err
		}
	}
	return nil
}

func (s *Splash) blitCenterMappedNearestDirectRect(
	x0, y0, x1, y1, xDest, yDest int,
	readLine func(int) (int, error),
	sample func(int) Color,
	xMap []int,
) error {
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	p := imagePipePool.Get().(*pipe)
	s.pipeInit(p, xDest+x0, yDest+y0, nil, &Color{}, alphaIn, false, false)
	for y := y0; y < y1; y++ {
		srcY, err := readLine(y)
		if err != nil {
			*p = pipe{}
			imagePipePool.Put(p)
			return err
		}
		s.pipeSetXY(p, xDest+x0, yDest+y)
		for x := x0; x < x1; x++ {
			c := sample(x)
			p.cSrc = c
			dx := xDest + x
			dy := yDest + y
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelBefore(p, "drawCenterMappedNearestDirect", dx, dy, xMap[x], srcY, c, 255)
			}
			p.run(p)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelAfter(p, "drawCenterMappedNearestDirect", dx, dy)
			}
		}
	}
	*p = pipe{}
	imagePipePool.Put(p)
	return nil
}

func (s *Splash) blitCenterMappedNearestDirectClipped(
	xSrc, ySrc, xDest, yDest, w, h int,
	readLine func(int) (int, error),
	sample func(int) Color,
	xMap []int,
) error {
	if w <= 0 || h <= 0 {
		return nil
	}
	if s.vectorAA {
		return s.blitCenterMappedNearestDirectClippedAA(xSrc, ySrc, xDest, yDest, w, h, readLine, sample, xMap)
	}
	return s.blitCenterMappedNearestDirectClippedNoAA(xSrc, ySrc, xDest, yDest, w, h, readLine, sample, xMap)
}

func (s *Splash) blitCenterMappedNearestDirectClippedAA(
	xSrc, ySrc, xDest, yDest, w, h int,
	readLine func(int) (int, error),
	sample func(int) Color,
	xMap []int,
) error {
	clip, _ := s.state.clip.(*xpath.Clip)
	if clip == nil {
		return nil
	}
	_, clipYMin, _, clipYMax := clip.IntBounds()
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	var p pipe
	s.pipeInit(&p, xDest, yDest, nil, &Color{}, alphaIn, true, false)
	spanGate := debugSplashImageClipSpanGate
	fullWidthClip := !debugSplashDisableFullWidthAabuf || debugSplashImageClipFullWidth

	rowSize := (s.bitmap.width*splashAASize + 7) >> 3
	aaLen := rowSize * splashAASize
	if len(s.aaBuf) < aaLen {
		s.aaBuf = make([]byte, aaLen)
	}

	for y := 0; y < h; y++ {
		scaledY := ySrc + y
		dy := yDest + y
		if dy < clipYMin || dy > clipYMax || dy < 0 || dy >= s.bitmap.height {
			continue
		}
		srcY, err := readLine(scaledY)
		if err != nil {
			return err
		}
		fillBitmapBytes(s.aaBuf[:aaLen], 0xff)
		if fullWidthClip {
			_, _ = clip.ClipAALineFullWidth(dy, s.aaBuf, 0, s.bitmap.width-1, s.bitmap.width)
		} else {
			clip.ClipAALine(dy, s.aaBuf, 0, s.bitmap.width-1)
		}

		for x := 0; x < w; x++ {
			scaledX := xSrc + x
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width {
				continue
			}
			t := -1
			if spanGate {
				switch clip.TestSpan(dx, dx, dy) {
				case xpath.ClipAllOutside:
					continue
				case xpath.ClipAllInside:
					t = splashAASize * splashAASize
				}
			}
			if t < 0 {
				t = s.aaCoverageAt(dx, rowSize)
			}
			t = adjustClippedImageLowAACoverageForDebug(t)
			if t == 0 {
				continue
			}
			shape := byte(s.aaGamma[t])
			if shape == 0 {
				continue
			}
			c := sample(scaledX)
			p.cSrc = c
			p.shape = shape
			s.pipeSetXY(&p, dx, dy)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelBefore(&p, "drawCenterMappedNearestDirectClippedAA", dx, dy, xMap[scaledX], srcY, c, shape)
			}
			p.run(&p)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelAfter(&p, "drawCenterMappedNearestDirectClippedAA", dx, dy)
			}
		}
	}
	return nil
}

func (s *Splash) blitCenterMappedNearestDirectClippedNoAA(
	xSrc, ySrc, xDest, yDest, w, h int,
	readLine func(int) (int, error),
	sample func(int) Color,
	xMap []int,
) error {
	clip, _ := s.state.clip.(*xpath.Clip)
	if clip == nil {
		return nil
	}
	alphaIn := byte(Round(s.state.fillAlpha * 255))
	var p pipe
	s.pipeInit(&p, xDest, yDest, nil, &Color{}, alphaIn, false, false)
	for y := 0; y < h; y++ {
		scaledY := ySrc + y
		dy := yDest + y
		if dy < 0 || dy >= s.bitmap.height {
			continue
		}
		srcY, err := readLine(scaledY)
		if err != nil {
			return err
		}
		for x := 0; x < w; x++ {
			scaledX := xSrc + x
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width || !clip.Test(dx, dy) {
				continue
			}
			c := sample(scaledX)
			p.cSrc = c
			s.pipeSetXY(&p, dx, dy)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelBefore(&p, "drawCenterMappedNearestDirectClippedNoAA", dx, dy, xMap[scaledX], srcY, c, 255)
			}
			p.run(&p)
			if shouldTraceImagePixel(dx, dy) {
				traceImagePixelAfter(&p, "drawCenterMappedNearestDirectClippedNoAA", dx, dy)
			}
		}
	}
	return nil
}

func (s *Splash) scaleImageYupXupCenterMapped(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	return s.scaleImageYupXupMapped(src, srcW, srcH, dstW, dstH, dest, true, true, 0)
}

func (s *Splash) scaleImageYupXupMapped(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap, centerX, centerY bool, yDelta int) error {
	nComps := nCompsForMode(s.bitmap.mode)
	hasAlpha := dest.alpha != nil
	bpp := bytesPerPixel(dest.mode)
	buffers := acquireImageScaleBuffers(srcW*nComps, 0, 0)
	defer releaseImageScaleBuffers(buffers)
	lineBuf := buffers.line
	var alphaLine []byte
	if hasAlpha {
		alphaLine = make([]byte, srcW)
	}
	xMap := make([]int, dstW)
	fillNearestUpscaleMap(xMap, srcW, dstW, centerX, 0)
	yMap := make([]int, dstH)
	fillNearestUpscaleMap(yMap, srcH, dstH, centerY, yDelta)
	lastSrcY := -1
	for y := 0; y < dstH; y++ {
		srcY := yMap[y]
		if srcY != lastSrcY {
			if err := s.readImageRow(src, srcW, lineBuf, alphaLine, srcY); err != nil {
				return err
			}
			lastSrcY = srcY
		}
		destOff := y * dstW * bpp
		destAlphaOff := y * dstW
		for x := 0; x < dstW; x++ {
			srcX := xMap[x]
			var pix [splashMaxColorComps]uint32
			for c := 0; c < nComps; c++ {
				pix[c] = uint32(lineBuf[srcX*nComps+c])
			}
			writePixel(dest.data, destOff, dest.mode, pix[:])
			destOff += bpp
			if hasAlpha {
				dest.alpha[destAlphaOff] = alphaLine[srcX]
				destAlphaOff++
			}
		}
	}
	return nil
}

func fillNearestUpscaleMap(dstToSrc []int, srcSize, dstSize int, center bool, delta int) {
	if len(dstToSrc) == 0 {
		return
	}
	if center {
		for dst := range dstToSrc {
			src := centerMappedNearestSourceIndex(dst, srcSize, dstSize) + delta
			if src < 0 {
				src = 0
			} else if src >= srcSize {
				src = srcSize - 1
			}
			dstToSrc[dst] = src
		}
		return
	}
	if srcSize <= 0 {
		for dst := range dstToSrc {
			dstToSrc[dst] = 0
		}
		return
	}
	xp := dstSize / srcSize
	xq := dstSize % srcSize
	xt := 0
	dst := 0
	for src := 0; src < srcSize && dst < len(dstToSrc); src++ {
		step := xp
		xt += xq
		if xt >= srcSize {
			xt -= srcSize
			step++
		}
		for i := 0; i < step && dst < len(dstToSrc); i++ {
			v := src + delta
			if v < 0 {
				v = 0
			} else if v >= srcSize {
				v = srcSize - 1
			}
			dstToSrc[dst] = v
			dst++
		}
	}
	for dst < len(dstToSrc) {
		dstToSrc[dst] = srcSize - 1
		dst++
	}
}

func centerMappedNearestSourceIndex(dst, srcSize, dstSize int) int {
	if srcSize <= 1 || dstSize <= 1 {
		return 0
	}
	src := ((2*dst + 1) * srcSize) / (2 * dstSize)
	if src < 0 {
		return 0
	}
	if src >= srcSize {
		return srcSize - 1
	}
	return src
}

// expandRow expands one srcWidth row to scaledWidth via linear interpolation
// (Splash.cc:4697). srcBuf must have one extra pixel of slack at index srcWidth.
func expandRow(srcBuf, dstBuf []byte, srcWidth, scaledWidth, nComps int) {
	expandRowWithPlan(srcBuf, dstBuf, nComps, newExpandRowPlan(srcWidth, scaledWidth))
}

type expandRowPlan struct {
	srcWidth    int
	scaledWidth int
	srcIndex    []int
	frac        []float64
}

func newExpandRowPlan(srcWidth, scaledWidth int) expandRowPlan {
	if srcWidth == 0 || scaledWidth == 0 {
		return expandRowPlan{}
	}
	xStep := float64(srcWidth) / float64(scaledWidth)
	return newExpandRowPlanWithStart(srcWidth, scaledWidth, 0.5*xStep-0.5)
}

func newPopplerOriginExpandRowPlan(srcWidth, scaledWidth int) expandRowPlan {
	return newExpandRowPlanWithStart(srcWidth, scaledWidth, 0)
}

func newExpandRowPlanWithStart(srcWidth, scaledWidth int, xSrc float64) expandRowPlan {
	if srcWidth == 0 || scaledWidth == 0 {
		return expandRowPlan{}
	}
	xStep := float64(srcWidth) / float64(scaledWidth)
	plan := expandRowPlan{
		srcWidth:    srcWidth,
		scaledWidth: scaledWidth,
		srcIndex:    make([]int, scaledWidth),
		frac:        make([]float64, scaledWidth),
	}
	for x := 0; x < scaledWidth; x++ {
		xSrcClamped := xSrc
		if xSrcClamped < 0 {
			xSrcClamped = 0
		}
		xInt, xFrac := math.Modf(xSrcClamped)
		plan.srcIndex[x] = int(xInt)
		plan.frac[x] = xFrac
		xSrc += xStep
	}
	return plan
}

func expandRowWithPlan(srcBuf, dstBuf []byte, nComps int, plan expandRowPlan) {
	srcWidth := plan.srcWidth
	scaledWidth := plan.scaledWidth
	if srcWidth == 0 || scaledWidth == 0 {
		return
	}
	// pad slot equal to last pixel (Splash.cc:4707).
	for i := 0; i < nComps; i++ {
		srcBuf[srcWidth*nComps+i] = srcBuf[(srcWidth-1)*nComps+i]
	}
	for x := 0; x < scaledWidth; x++ {
		xFrac := plan.frac[x]
		xWeight0 := 1.0 - xFrac
		p := plan.srcIndex[x]
		srcOff := nComps * p
		dstOff := nComps * x
		switch nComps {
		case 1:
			a := float64(srcBuf[srcOff])
			b := float64(srcBuf[srcOff+1])
			dstBuf[dstOff] = byte(a*xWeight0 + b*xFrac)
		case 3:
			nextOff := srcOff + 3
			a := float64(srcBuf[srcOff])
			b := float64(srcBuf[nextOff])
			dstBuf[dstOff] = byte(a*xWeight0 + b*xFrac)
			a = float64(srcBuf[srcOff+1])
			b = float64(srcBuf[nextOff+1])
			dstBuf[dstOff+1] = byte(a*xWeight0 + b*xFrac)
			a = float64(srcBuf[srcOff+2])
			b = float64(srcBuf[nextOff+2])
			dstBuf[dstOff+2] = byte(a*xWeight0 + b*xFrac)
		default:
			nextOff := srcOff + nComps
			for c := 0; c < nComps; c++ {
				a := float64(srcBuf[srcOff+c])
				b := float64(srcBuf[nextOff+c])
				dstBuf[dstOff+c] = byte(a*xWeight0 + b*xFrac)
			}
		}
	}
}

// scaleImageYupXupBilinear — both grow with bilinear interpolation (Splash.cc:4722).
//
// CRITICAL last-row clamp: when currentSrcRow reaches srcH-1 the kernel must
// NOT read another row past the source. lineBuf2 retains the previous (last
// valid) data, providing the row of "padding" the interpolation needs. This
// matches Splash.cc:4771 and is required by memory bilinear_lastrow_clamp_2026_04_26.
func (s *Splash) scaleImageYupXupBilinear(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap) error {
	return s.scaleImageYupXupBilinearWithOptions(src, srcW, srcH, dstW, dstH, dest, imageDrawOptions{})
}

func (s *Splash) scaleImageYupXupBilinearWithOptions(src ImageSource, srcW, srcH, dstW, dstH int, dest *Bitmap, options imageDrawOptions) error {
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
	// Poppler's scaleImageYupXupBilinear uses origin phase (Splash.cc:4700/4747:
	// xSrc=0, ySrc=0). Center phase (0.5*step-0.5) diverges for non-trivial
	// scales, so origin phase is the unconditional default.
	expandPlan := newPopplerOriginExpandRowPlan(srcW, dstW)

	yStep := float64(srcH) / float64(dstH)
	ySrc := 0.0
	currentSrcRow := -1
	rowIdx := 0

	if err := s.readImageRow(src, srcW, srcBuf[:srcW*nComps], alphaSrcBuf, rowIdx); err != nil {
		return err
	}
	rowIdx++
	expandRowWithPlan(srcBuf, lineBuf2, nComps, expandPlan)
	if hasAlpha {
		expandRowWithPlan(alphaSrcBuf, alphaLineBuf2, 1, expandPlan)
	}

	for y := 0; y < dstH; y++ {
		ySrcClamped := ySrc
		if ySrcClamped < 0 {
			ySrcClamped = 0
		}
		yInt, yFrac := math.Modf(ySrcClamped)
		for int(yInt) > currentSrcRow {
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
				expandRowWithPlan(srcBuf, lineBuf2, nComps, expandPlan)
				if hasAlpha {
					expandRowWithPlan(alphaSrcBuf, alphaLineBuf2, 1, expandPlan)
				}
			}
		}

		yWeight0 := 1.0 - yFrac
		if dest.mode == ModeRGB8 {
			dstOff := y * dstW * 3
			for x := 0; x < dstW; x++ {
				srcOff := x * 3
				dest.data[dstOff] = byte(float64(lineBuf1[srcOff])*yWeight0 + float64(lineBuf2[srcOff])*yFrac)
				dest.data[dstOff+1] = byte(float64(lineBuf1[srcOff+1])*yWeight0 + float64(lineBuf2[srcOff+1])*yFrac)
				dest.data[dstOff+2] = byte(float64(lineBuf1[srcOff+2])*yWeight0 + float64(lineBuf2[srcOff+2])*yFrac)
				if hasAlpha {
					a := float64(alphaLineBuf1[x])
					b := float64(alphaLineBuf2[x])
					dest.alpha[y*dstW+x] = byte(a*yWeight0 + b*yFrac)
				}
				dstOff += 3
			}
		} else {
			for x := 0; x < dstW; x++ {
				var pix [splashMaxColorComps]uint32
				for i := 0; i < nComps; i++ {
					a := float64(lineBuf1[x*nComps+i])
					b := float64(lineBuf2[x*nComps+i])
					pix[i] = uint32(byte(a*yWeight0 + b*yFrac))
				}
				off := (y*dstW + x) * bpp
				writePixel(dest.data, off, dest.mode, pix[:])
				if hasAlpha {
					a := float64(alphaLineBuf1[x])
					b := float64(alphaLineBuf2[x])
					dest.alpha[y*dstW+x] = byte(a*yWeight0 + b*yFrac)
				}
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

func (s *Splash) applyPostScaleRGBTransform(bitmap *Bitmap, transform postScaleRGBTransform) {
	if transform == nil || bitmap == nil || bitmap.data == nil {
		return
	}
	switch bitmap.mode {
	case ModeRGB8:
		for i := 0; i+2 < len(bitmap.data); i += 3 {
			bitmap.data[i], bitmap.data[i+1], bitmap.data[i+2] = transform(bitmap.data[i], bitmap.data[i+1], bitmap.data[i+2])
		}
	case ModeBGR8:
		for i := 0; i+2 < len(bitmap.data); i += 3 {
			r, g, b := transform(bitmap.data[i+2], bitmap.data[i+1], bitmap.data[i])
			bitmap.data[i] = b
			bitmap.data[i+1] = g
			bitmap.data[i+2] = r
		}
	case ModeXBGR8:
		for i := 0; i+3 < len(bitmap.data); i += 4 {
			r, g, b := transform(bitmap.data[i+2], bitmap.data[i+1], bitmap.data[i])
			bitmap.data[i] = b
			bitmap.data[i+1] = g
			bitmap.data[i+2] = r
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
		p := imagePipePool.Get().(*pipe)
		s.pipeInit(p, xDest+x0, yDest+y0, nil, &Color{}, alphaIn, hasAlpha, false)

		for y := y0; y < y1; y++ {
			s.pipeSetXY(p, xDest+x0, yDest+y)
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
					if shouldUnpremultiplyImageColor() {
						unpremultiplyImageColor(&c, scaled.mode, shape)
					}
					aOff++
				}
				dx := xDest + x
				dy := yDest + y
				p.cSrc = c
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelBefore(p, "blitImage", dx, dy, x, y, c, shape)
				}
				p.run(p)
				if shouldTraceImagePixel(dx, dy) {
					traceImagePixelAfter(p, "blitImage", dx, dy)
				}
				srcOff += srcBpp
			}
		}
		*p = pipe{}
		imagePipePool.Put(p)
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
	spanGate := os.Getenv("PDF_DEBUG_SPLASH_IMAGE_CLIP_SPAN_GATE") == "1"
	fullWidthClip := os.Getenv("PDF_DEBUG_SPLASH_IMAGE_CLIP_FULLWIDTH") == "1"

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
		fillBitmapBytes(s.aaBuf[:aaLen], 0xff)
		if fullWidthClip {
			_, _ = clip.ClipAALineFullWidth(dy, s.aaBuf, 0, s.bitmap.width-1, s.bitmap.width)
		} else {
			clip.ClipAALine(dy, s.aaBuf, 0, s.bitmap.width-1)
		}

		srcOff := ((ySrc+y)*scaled.width + xSrc) * bpp
		alphaOff := (ySrc+y)*scaled.width + xSrc
		for x := 0; x < w; x++ {
			dx := xDest + x
			if dx < 0 || dx >= s.bitmap.width {
				srcOff += bpp
				alphaOff++
				continue
			}
			t := -1
			if spanGate {
				switch clip.TestSpan(dx, dx, dy) {
				case xpath.ClipAllOutside:
					srcOff += bpp
					alphaOff++
					continue
				case xpath.ClipAllInside:
					t = splashAASize * splashAASize
				}
			}
			if t < 0 {
				t = s.aaCoverageAt(dx, rowSize)
			}
			t = adjustClippedImageLowAACoverageForDebug(t)
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
				if shouldUnpremultiplyImageColor() {
					unpremultiplyImageColor(&c, scaled.mode, shape)
				}
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

func adjustClippedImageLowAACoverageForDebug(t int) int {
	if !debugSplashImageClipT2ToT1 {
		return t
	}
	if t == 2 {
		return 1
	}
	return t
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
			if dx < 0 || dx >= s.bitmap.width || !clip.Test(dx, dy) {
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
				if shouldUnpremultiplyImageColor() {
					unpremultiplyImageColor(&c, scaled.mode, shape)
				}
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
	if x < 0 || rowSize <= 0 {
		return 0
	}
	return s.aaBufCoverageAt(x, rowSize)
}

// unpremultiplyImageColor is kept as an opt-in diagnostic for legacy callers
// that feed premultiplied color samples into ImageSource. The PDF image path
// mirrors Poppler's maskedImageSrc contract: color samples are already straight
// source colors, and source alpha is carried separately as pipe.shape.
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

func shouldUnpremultiplyImageColor() bool {
	return splashImageEnableAlphaUnpremultiply
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
func (s *Splash) arbitraryTransformImage(
	src ImageSource,
	srcW, srcH int,
	mat [6]float64,
	interpolate bool,
	sourceAlpha bool,
	postTransform postScaleRGBTransform,
) error {
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
	clipRes := s.testRect(xMin, yMin, xMax, yMax)
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

	// Scale the source to the bbox-derived device resolution, passing the real
	// interpolate flag (Poppler Splash.cc:3851: scaledImg = scaleImage(src,
	// srcW, srcH, scaledWidth, scaledHeight, interpolate)). Poppler's
	// arbitraryTransformImage is a two-pass scale-then-nearest-warp — NOT a
	// one-pass bilinear warp — so we must NOT keep the source at 1:1 here.
	scaled, err := s.scaleImageWithSourceAlpha(src, srcW, srcH, scaledW, scaledH, interpolate, sourceAlpha)
	if err != nil {
		return err
	}
	s.applyPostScaleRGBTransform(scaled, postTransform)

	// Compute the inverse of the (image→device) matrix in Poppler's exact
	// formulation (Splash.cc:3763-3776): first scale the linear part into
	// scaled-image space (r = mat/scaled), then invert THAT. The result maps
	// device points straight to scaled-image pixel coords, so the warp below
	// needs no ·sampleW/·sampleH re-scale, and every float op matches
	// Poppler's — deriving the same values in a different op-order flips
	// splashFloor at pixel boundaries on rotated images.
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

	sampleW := scaled.width
	sampleH := scaled.height
	if sampleW <= 0 || sampleH <= 0 {
		return nil
	}
	bpp := bytesPerPixel(scaled.mode)
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
			// Poppler samples the section edges at the row CENTER
			// (Splash.cc:3903: xa0 + (y + 0.5 - ya0) * dxdya). Omitting the
			// +0.5 is invisible for axis-aligned quads (dxdy = 0) but shifts
			// every span boundary of a ROTATED image by half a row.
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
					clip.ClipAALineFullWidth(y, s.aaBuf, 0, s.bitmap.width-1, s.bitmap.width)
					aaReady = true
				}
			}
			for x := xa; x < xb; x++ {
				if x < 0 || x >= s.bitmap.width || y < 0 || y >= s.bitmap.height {
					continue
				}
				fx := float64(x) + 0.5
				fy := float64(y) + 0.5

				var c Color
				shape := byte(255)

				// Nearest-neighbor warp of the scaled bitmap (Poppler
				// Splash.cc:3920-3923): map (x+0.5, y+0.5) back to the scaled
				// image with Poppler's exact expression — translation
				// subtracted BEFORE the multiply, no interpolation in the warp.
				// The bilinear case is handled by scaleImage above, which is
				// given the real interpolate flag.
				srcXf := (fx-mat[4])*ir00 + (fy-mat[5])*ir10
				srcYf := (fx-mat[4])*ir01 + (fy-mat[5])*ir11
				ix := int(math.Floor(srcXf))
				iy := int(math.Floor(srcYf))
				if ix < 0 {
					ix = 0
				} else if ix >= sampleW {
					ix = sampleW - 1
				}
				if iy < 0 {
					iy = 0
				} else if iy >= sampleH {
					iy = sampleH - 1
				}
				readScaledPixel(scaled.data, (iy*sampleW+ix)*bpp, scaled.mode, &c)
				if hasAlpha {
					shape = scaled.alpha[iy*sampleW+ix]
					if shouldUnpremultiplyImageColor() {
						unpremultiplyImageColor(&c, scaled.mode, shape)
					}
				}

				if s.vectorAA && clipRes2 != xpath.ClipAllInside {
					if !aaReady {
						continue
					}
					t := s.aaCoverageAt(x, rowSize)
					if shouldTraceImagePixel(x, y) {
						cxMin, cyMin, cxMax, cyMax := clip.Bounds()
						fmt.Fprintf(os.Stderr, "SPLASH_IMAGE_CLIPAA x=%d y=%d t=%d clipRes2=%d hasPath=%t bounds=(%.17g,%.17g)-(%.17g,%.17g)\n",
							x, y, t, clipRes2, clip.HasPathClip(), cxMin, cyMin, cxMax, cyMax)
					}
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
					traceImagePixelBefore(&p, "arbitraryTransformImage", x, y, -1, -1, c, shape) // srcXY not easily available for bilinear
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
var imageScaleTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_SCALE_TRACE"))
var imageTracePixelsEnabled = len(imageTracePixels) > 0
var imageScaleTracePixelsEnabled = len(imageScaleTracePixels) > 0

func shouldTraceImagePixel(x, y int) bool {
	if !imageTracePixelsEnabled {
		return false
	}
	return imageTracePixelMatch(x, y)
}

func imageTracePixelMatch(x, y int) bool {
	for _, pixel := range imageTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func (s *Splash) shouldTraceImageScalePixel(x, y int) bool {
	if len(imageScaleTracePixels) == 0 {
		return false
	}
	filter := os.Getenv("PDF_DEBUG_SPLASH_SCALE_TRACE_CONTEXT")
	if filter != "" && (s == nil || !strings.Contains(s.debugPaintContext, filter)) {
		return false
	}
	for _, pixel := range imageScaleTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func imageTraceContextForSplash(s *Splash) string {
	if s == nil || s.debugPaintContext == "" {
		return ""
	}
	return fmt.Sprintf(" ctx=%q", s.debugPaintContext)
}

func traceImagePixelBefore(p *pipe, op string, x, y, srcX, srcY int, c Color, shape byte) {
	if p.colorBytesPerPixel < 3 || p.destOff+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil && p.aDestOff < len(p.aDestRow) {
		aDest = p.aDestRow[p.aDestOff]
	}
	fmt.Fprintf(os.Stderr, "SPLASH_IMAGE_TRACE before op=%s x=%d y=%d srcXY=(%d,%d) src=(%d,%d,%d) shape=%d dst=(%d,%d,%d) aDest=%d%s\n",
		op, x, y, srcX, srcY, c[0], c[1], c[2], shape,
		p.destRow[p.destOff], p.destRow[p.destOff+1], p.destRow[p.destOff+2], aDest,
		imageTraceContext(p))
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
	fmt.Fprintf(os.Stderr, "SPLASH_IMAGE_TRACE after op=%s x=%d y=%d dst=(%d,%d,%d) aDest=%d%s\n",
		op, x, y, p.destRow[off], p.destRow[off+1], p.destRow[off+2], aDest,
		imageTraceContext(p))
}

func imageTraceContext(p *pipe) string {
	if p == nil || p.s == nil || p.s.debugPaintContext == "" {
		return ""
	}
	return fmt.Sprintf(" ctx=%q", p.s.debugPaintContext)
}
