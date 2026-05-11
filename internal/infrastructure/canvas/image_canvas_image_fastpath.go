package canvas

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
)

func (c *ImageCanvas) canUseAxisAlignedTransparentEdgeOverWhiteFastPath(
	edgeMode string,
	p00X, p00Y float64,
	p10X, p10Y float64,
	p01X, p01Y float64,
) bool {
	if edgeMode != domainimage.ImageEdgeModeTransparentEdgeOverWhite {
		return false
	}
	if !nearlyEqual(p10Y, p00Y) || !nearlyEqual(p01X, p00X) {
		return false
	}
	return (p10X-p00X) > 0 && (p01Y-p00Y) > 0
}

func (c *ImageCanvas) canUseAxisAlignedBoxDownscaleFastPath(
	sampler string,
	p00X, p00Y float64,
	p10X, p10Y float64,
	p01X, p01Y float64,
	srcBounds image.Rectangle,
	dstRect image.Rectangle,
	dstMinXF, dstMinYF float64,
) bool {
	if !strings.Contains(sampler, "box") {
		return false
	}
	if !nearlyEqual(p10Y, p00Y) || !nearlyEqual(p01X, p00X) {
		return false
	}
	if (p10X-p00X) <= 0 || (p01Y-p00Y) <= 0 {
		return false
	}
	if !isNearlyInteger(dstMinXF) || !isNearlyInteger(dstMinYF) {
		return false
	}
	if dstRect.Min.X < c.img.Bounds().Min.X || dstRect.Min.Y < c.img.Bounds().Min.Y ||
		dstRect.Max.X > c.img.Bounds().Max.X || dstRect.Max.Y > c.img.Bounds().Max.Y {
		return false
	}
	dstWidth := dstRect.Dx()
	dstHeight := dstRect.Dy()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()
	if dstWidth <= 0 || dstHeight <= 0 || srcWidth <= dstWidth || srcHeight <= dstHeight {
		return false
	}
	if srcWidth%dstWidth != 0 || srcHeight%dstHeight != 0 {
		return false
	}
	// 2x downscale with iccbased sampler uses a Poppler-matched path, not standard box averaging.
	if strings.Contains(sampler, "iccbased") && srcWidth/dstWidth == 2 && srcHeight/dstHeight == 2 {
		return false
	}
	return true
}

func (c *ImageCanvas) canUseAxisAlignedPopplerStyle2xBoxFastPath(
	sampler string,
	p00X, p00Y float64,
	p10X, p10Y float64,
	p01X, p01Y float64,
	srcBounds image.Rectangle,
	dstRect image.Rectangle,
	dstMinXF, dstMinYF float64,
) bool {
	if !strings.Contains(sampler, "iccbased") && !strings.Contains(sampler, "ccittfax") {
		return false
	}
	if !nearlyEqual(p10Y, p00Y) || !nearlyEqual(p01X, p00X) {
		return false
	}
	if (p10X-p00X) <= 0 || (p01Y-p00Y) <= 0 {
		return false
	}
	if !isNearlyInteger(dstMinXF) || !isNearlyInteger(dstMinYF) {
		return false
	}
	if dstRect.Min.X < c.img.Bounds().Min.X || dstRect.Min.Y < c.img.Bounds().Min.Y ||
		dstRect.Max.X > c.img.Bounds().Max.X || dstRect.Max.Y > c.img.Bounds().Max.Y {
		return false
	}
	dstWidth := dstRect.Dx()
	dstHeight := dstRect.Dy()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	return srcWidth/dstWidth == 2 && srcHeight/dstHeight == 2 &&
		srcWidth%dstWidth == 0 && srcHeight%dstHeight == 0
}

// popplerSourceRange1D returns the source pixel indices that contribute to destination
// pixel j when downscaling from srcDim to dstDim=srcDim/2 using Poppler's algorithm.
// The grouping is asymmetric: j=0 takes src[0] alone, j=half takes src[half-1] alone,
// and the last two source pixels are unused.
func popplerSourceRange1D(j, dstDim, srcDim int) (start, end int) {
	half := dstDim / 2
	switch {
	case j == 0:
		return 0, 1
	case j == half:
		return srcDim/2 - 1, srcDim / 2
	case j < half:
		s := 2*j - 1
		return s, s + 2
	default: // j > half
		s := srcDim/2 + 2*(j-half-1)
		return s, s + 2
	}
}

func (c *ImageCanvas) drawAxisAlignedPopplerStyle2xBox(
	src image.Image,
	srcBounds image.Rectangle,
	dstRect image.Rectangle,
) {
	dstImg := c.img
	offsetX := dstRect.Min.X
	offsetY := dstRect.Min.Y
	if c.clipMask != nil {
		dstImg = image.NewRGBA(image.Rect(0, 0, dstRect.Dx(), dstRect.Dy()))
		offsetX = 0
		offsetY = 0
	}

	dstW := dstRect.Dx()
	dstH := dstRect.Dy()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	for dy := 0; dy < dstH; dy++ {
		rowStart, rowEnd := popplerSourceRange1D(dy, dstH, srcH)
		for dx := 0; dx < dstW; dx++ {
			colStart, colEnd := popplerSourceRange1D(dx, dstW, srcW)

			var sumR, sumG, sumB, sumA uint64
			count := uint64((rowEnd - rowStart) * (colEnd - colStart))
			for sy := srcBounds.Min.Y + rowStart; sy < srcBounds.Min.Y+rowEnd; sy++ {
				for sx := srcBounds.Min.X + colStart; sx < srcBounds.Min.X+colEnd; sx++ {
					r, g, b, a := rgba8Components(src.At(sx, sy))
					sumR += uint64(r)
					sumG += uint64(g)
					sumB += uint64(b)
					sumA += uint64(a)
				}
			}
			if count == 0 {
				continue
			}
			dstImg.SetRGBA(dstRect.Min.X+dx-offsetX, dstRect.Min.Y+dy-offsetY, color.RGBA{
				R: uint8(sumR / count),
				G: uint8(sumG / count),
				B: uint8(sumB / count),
				A: uint8(sumA / count),
			})
		}
	}

	if c.clipMask == nil {
		return
	}

	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			dstX := dstRect.Min.X + dx
			dstY := dstRect.Min.Y + dy
			maskAlpha := c.clipMask.AlphaAt(dstX, dstY).A
			if maskAlpha == 0 {
				continue
			}
			srcColor := dstImg.RGBAAt(dx, dy)
			if srcColor.A == 0 {
				continue
			}
			srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
			if srcMasked.A == 0 {
				continue
			}
			dstColor := c.img.RGBAAt(dstX, dstY)
			c.img.SetRGBA(dstX, dstY, compositeOver(dstColor, srcMasked))
		}
	}
}

func (c *ImageCanvas) canUseAxisAlignedSplashDownscaleFastPath(
	sampler string,
	p00X, p00Y float64,
	p10X, p10Y float64,
	p01X, p01Y float64,
	srcBounds image.Rectangle,
) bool {
	if sampler != "auto_downscale_bilinear" {
		return false
	}
	if !nearlyEqual(p10Y, p00Y) || !nearlyEqual(p01X, p00X) {
		return false
	}
	if (p10X-p00X) <= 0 || (p01Y-p00Y) <= 0 {
		return false
	}
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return false
	}
	scaledW := imgCoordMungeUpper(p10X) - imgCoordMungeLower(p00X)
	scaledH := imgCoordMungeUpper(p01Y) - imgCoordMungeLower(p00Y)
	return scaledW > 0 && scaledH > 0 && scaledW < srcW && scaledH < srcH
}

func (c *ImageCanvas) drawAxisAlignedSplashDownscale(
	src image.Image,
	srcBounds image.Rectangle,
	p00X, p00Y, p10X, p01Y float64,
) {
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return
	}

	splashX0 := imgCoordMungeLower(p00X)
	splashX1 := imgCoordMungeUpper(p10X)
	splashY0 := imgCoordMungeLower(math.Min(p00Y, p01Y))
	splashY1 := imgCoordMungeUpper(math.Max(p00Y, p01Y))
	scaledW := splashX1 - splashX0
	scaledH := splashY1 - splashY0
	if scaledW <= 0 || scaledH <= 0 {
		return
	}

	yp := srcH / scaledH
	yq := srcH % scaledH
	xp := srcW / scaledW
	xq := srcW % scaledW
	if yp <= 0 || xp <= 0 {
		return
	}

	pixR := make([]uint64, srcW)
	pixG := make([]uint64, srcW)
	pixB := make([]uint64, srcW)
	opaqueSource := imageBoundsFullyOpaque(src, srcBounds)
	var pixA []uint64
	if !opaqueSource {
		pixA = make([]uint64, srcW)
	}
	canvasBounds := c.img.Bounds()
	canvasYTop := imgCoordMungeLower(float64(c.height) - math.Max(p00Y, p01Y))
	srcMinX := srcBounds.Min.X
	srcMinY := srcBounds.Min.Y

	yt := 0
	srcRow := 0
	for y := 0; y < scaledH; y++ {
		yStep := yp
		yt += yq
		if yt >= scaledH {
			yt -= scaledH
			yStep++
		}

		for i := range pixR {
			pixR[i], pixG[i], pixB[i] = 0, 0, 0
			if !opaqueSource {
				pixA[i] = 0
			}
		}
		for i := 0; i < yStep && srcRow+i < srcH; i++ {
			sy := srcMinY + srcRow + i
			for sx := 0; sx < srcW; sx++ {
				r, g, b, a := rgba8Components(src.At(srcMinX+sx, sy))
				pixR[sx] += uint64(r)
				pixG[sx] += uint64(g)
				pixB[sx] += uint64(b)
				if !opaqueSource {
					pixA[sx] += uint64(a)
				}
			}
		}
		srcRow += yStep

		dy := canvasYTop + y
		if dy < canvasBounds.Min.Y || dy >= canvasBounds.Max.Y {
			continue
		}

		xt := 0
		xx := 0
		d0 := uint64((1 << 23) / (yStep * xp))
		d1 := uint64((1 << 23) / (yStep * (xp + 1)))
		for x := 0; x < scaledW; x++ {
			xStep := xp
			d := d0
			xt += xq
			if xt >= scaledW {
				xt -= scaledW
				xStep++
				d = d1
			}

			var sumR, sumG, sumB, sumA uint64
			for i := 0; i < xStep && xx+i < srcW; i++ {
				sumR += pixR[xx+i]
				sumG += pixG[xx+i]
				sumB += pixB[xx+i]
				if !opaqueSource {
					sumA += pixA[xx+i]
				}
			}
			xx += xStep

			dx := splashX0 + x
			if dx < canvasBounds.Min.X || dx >= canvasBounds.Max.X {
				continue
			}

			srcC := color.RGBA{
				R: uint8((sumR * d) >> 23),
				G: uint8((sumG * d) >> 23),
				B: uint8((sumB * d) >> 23),
				A: 255,
			}
			if !opaqueSource {
				srcC.A = uint8((sumA * d) >> 23)
			}
			if srcC.A == 0 {
				continue
			}
			if c.clipMask != nil {
				maskAlpha := c.clipMask.AlphaAt(dx, dy).A
				if maskAlpha == 0 {
					continue
				}
				srcC = applyPremultipliedAlpha(srcC, maskAlpha)
				if srcC.A == 0 {
					continue
				}
			}
			dstC := c.img.RGBAAt(dx, dy)
			c.img.SetRGBA(dx, dy, compositeOver(dstC, srcC))
		}
	}
}

func imageBoundsFullyOpaque(src image.Image, bounds image.Rectangle) bool {
	switch img := src.(type) {
	case *image.Gray, *image.YCbCr, *image.CMYK:
		return true
	case *image.RGBA:
		scan := bounds.Intersect(img.Bounds())
		for y := scan.Min.Y; y < scan.Max.Y; y++ {
			row := img.Pix[(y-img.Rect.Min.Y)*img.Stride:]
			for x := scan.Min.X; x < scan.Max.X; x++ {
				if row[(x-img.Rect.Min.X)*4+3] != 255 {
					return false
				}
			}
		}
		return true
	case *image.NRGBA:
		scan := bounds.Intersect(img.Bounds())
		for y := scan.Min.Y; y < scan.Max.Y; y++ {
			row := img.Pix[(y-img.Rect.Min.Y)*img.Stride:]
			for x := scan.Min.X; x < scan.Max.X; x++ {
				if row[(x-img.Rect.Min.X)*4+3] != 255 {
					return false
				}
			}
		}
		return true
	default:
		scan := bounds.Intersect(src.Bounds())
		for y := scan.Min.Y; y < scan.Max.Y; y++ {
			for x := scan.Min.X; x < scan.Max.X; x++ {
				_, _, _, a := src.At(x, y).RGBA()
				if a != 0xffff {
					return false
				}
			}
		}
		return true
	}
}

func rgba8Components(c color.Color) (r, g, b, a uint8) {
	r16, g16, b16, a16 := c.RGBA()
	return uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8), uint8(a16 >> 8)
}

func (c *ImageCanvas) canUseAxisAlignedSplashScaleOnlyFastPath(
	sampler string,
	p00X, p00Y float64,
	p10X, p10Y float64,
	p01X, p01Y float64,
	dstMinXF, dstMaxXF float64,
	dstMinYF, dstMaxYF float64,
) bool {
	if !strings.Contains(sampler, "splash_scale_only") {
		return false
	}
	if !nearlyEqual(p10Y, p00Y) || !nearlyEqual(p01X, p00X) {
		return false
	}
	if (p10X-p00X) <= 0 || (p01Y-p00Y) <= 0 {
		return false
	}
	if dstMaxXF <= dstMinXF || dstMaxYF <= dstMinYF {
		return false
	}
	return true
}

func (c *ImageCanvas) drawAxisAlignedBoxDownscale(
	src image.Image,
	srcBounds image.Rectangle,
	dstRect image.Rectangle,
) {
	dstImg := c.img
	offsetX := dstRect.Min.X
	offsetY := dstRect.Min.Y
	if c.clipMask != nil {
		dstImg = image.NewRGBA(image.Rect(0, 0, dstRect.Dx(), dstRect.Dy()))
		offsetX = 0
		offsetY = 0
	}

	scaleX := srcBounds.Dx() / dstRect.Dx()
	scaleY := srcBounds.Dy() / dstRect.Dy()

	for dy := 0; dy < dstRect.Dy(); dy++ {
		srcMinY := srcBounds.Min.Y + dy*scaleY
		srcMaxY := srcMinY + scaleY
		dstY := dstRect.Min.Y + dy

		for dx := 0; dx < dstRect.Dx(); dx++ {
			srcMinX := srcBounds.Min.X + dx*scaleX
			srcMaxX := srcMinX + scaleX
			dstX := dstRect.Min.X + dx

			var sumR, sumG, sumB, sumA uint64
			count := uint64((srcMaxX - srcMinX) * (srcMaxY - srcMinY))
			for sy := srcMinY; sy < srcMaxY; sy++ {
				for sx := srcMinX; sx < srcMaxX; sx++ {
					r, g, b, a := src.At(sx, sy).RGBA()
					sumR += uint64(r)
					sumG += uint64(g)
					sumB += uint64(b)
					sumA += uint64(a)
				}
			}
			if count == 0 {
				continue
			}

			dstImg.SetRGBA(dstX-offsetX, dstY-offsetY, color.RGBA{
				R: uint8((sumR / count) >> 8),
				G: uint8((sumG / count) >> 8),
				B: uint8((sumB / count) >> 8),
				A: uint8((sumA / count) >> 8),
			})
		}
	}

	if c.clipMask == nil {
		return
	}

	for dy := 0; dy < dstRect.Dy(); dy++ {
		for dx := 0; dx < dstRect.Dx(); dx++ {
			dstX := dstRect.Min.X + dx
			dstY := dstRect.Min.Y + dy
			maskAlpha := c.clipMask.AlphaAt(dstX, dstY).A
			if maskAlpha == 0 {
				continue
			}

			srcColor := dstImg.RGBAAt(dx, dy)
			if srcColor.A == 0 {
				continue
			}
			srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
			if srcMasked.A == 0 {
				continue
			}
			dstColor := c.img.RGBAAt(dstX, dstY)
			c.img.SetRGBA(dstX, dstY, compositeOver(dstColor, srcMasked))
		}
	}
}

func (c *ImageCanvas) drawAxisAlignedSplashScaleOnly(
	src image.Image,
	srcBounds image.Rectangle,
	dstMinXF, dstMaxXF float64,
	dstMinYF, dstMaxYF float64,
	interpolate bool,
) {
	x0 := imgCoordMungeLower(dstMinXF)
	y0 := imgCoordMungeLower(dstMinYF)
	x1 := imgCoordMungeUpper(dstMaxXF)
	y1 := imgCoordMungeUpper(dstMaxYF)
	targetRect := image.Rect(x0, y0, x1, y1)
	if targetRect.Empty() {
		return
	}

	scaled := image.NewRGBA(image.Rect(0, 0, targetRect.Dx(), targetRect.Dy()))
	transform := f64.Aff3{
		float64(targetRect.Dx()) / float64(srcBounds.Dx()),
		0,
		0,
		0,
		float64(targetRect.Dy()) / float64(srcBounds.Dy()),
		0,
	}
	if interpolate {
		xdraw.ApproxBiLinear.Transform(scaled, transform, src, srcBounds, draw.Src, nil)
	} else {
		xdraw.NearestNeighbor.Transform(scaled, transform, src, srcBounds, draw.Src, nil)
	}

	clipped := targetRect.Intersect(c.img.Bounds())
	if clipped.Empty() {
		return
	}
	srcPoint := image.Point{X: clipped.Min.X - targetRect.Min.X, Y: clipped.Min.Y - targetRect.Min.Y}
	if c.clipMask == nil {
		draw.Draw(c.img, clipped, scaled, srcPoint, draw.Over)
		return
	}

	for dy := 0; dy < clipped.Dy(); dy++ {
		for dx := 0; dx < clipped.Dx(); dx++ {
			dstX := clipped.Min.X + dx
			dstY := clipped.Min.Y + dy
			maskAlpha := c.clipMask.AlphaAt(dstX, dstY).A
			if maskAlpha == 0 {
				continue
			}
			srcColor := scaled.RGBAAt(srcPoint.X+dx, srcPoint.Y+dy)
			if srcColor.A == 0 {
				continue
			}
			srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
			if srcMasked.A == 0 {
				continue
			}
			dstColor := c.img.RGBAAt(dstX, dstY)
			c.img.SetRGBA(dstX, dstY, compositeOver(dstColor, srcMasked))
		}
	}
}

func (c *ImageCanvas) drawAxisAlignedTransparentEdgeOverWhite(
	src image.Image,
	dstMinXF float64,
	dstMinYF float64,
	dstWidthF float64,
	dstHeightF float64,
) {
	dstW := max(1, int(math.Round(dstWidthF)))
	dstH := max(1, int(math.Round(dstHeightF)))
	temp := resampleTransparentEdgeBilinear(src, dstW, dstH)
	tempBounds := temp.Bounds()

	targetRect := image.Rect(
		int(math.Floor(dstMinXF-0.5)),
		int(math.Floor(dstMinYF-0.5)),
		int(math.Ceil(dstMinXF+float64(dstW)+0.5)),
		int(math.Ceil(dstMinYF+float64(dstH)+0.5)),
	).Intersect(c.img.Bounds())
	if targetRect.Empty() {
		return
	}

	for dy := targetRect.Min.Y; dy < targetRect.Max.Y; dy++ {
		for dx := targetRect.Min.X; dx < targetRect.Max.X; dx++ {
			u := float64(dx) + 0.5 - dstMinXF
			v := float64(dy) + 0.5 - dstMinYF
			opaque := compositeRGBAOverWhite(sampleTransparentEdgeBilinear(temp, tempBounds, u, v))
			if c.clipMask == nil {
				c.img.SetRGBA(dx, dy, opaque)
				continue
			}

			maskAlpha := c.clipMask.AlphaAt(dx, dy).A
			if maskAlpha == 0 {
				continue
			}
			srcMasked := applyPremultipliedAlpha(opaque, maskAlpha)
			if srcMasked.A == 0 {
				continue
			}
			dstColor := c.img.RGBAAt(dx, dy)
			c.img.SetRGBA(dx, dy, compositeOver(dstColor, srcMasked))
		}
	}
}

func resampleTransparentEdgeBilinear(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return dst
	}

	scaleX := float64(dstW) / srcW
	scaleY := float64(dstH) / srcH
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			u := (float64(dx) + 0.5) / scaleX
			v := (float64(dy) + 0.5) / scaleY
			dst.SetRGBA(dx, dy, sampleTransparentEdgeBilinear(src, srcBounds, u, v))
		}
	}
	return dst
}

func sampleTransparentEdgeBilinear(src image.Image, srcBounds image.Rectangle, u, v float64) color.RGBA {
	sx := u - 0.5
	sy := v - 0.5

	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	x1 := x0 + 1
	y1 := y0 + 1

	c00 := sampleTransparentPremultipliedRGBA(src, srcBounds, x0, y0)
	c10 := sampleTransparentPremultipliedRGBA(src, srcBounds, x1, y0)
	c01 := sampleTransparentPremultipliedRGBA(src, srcBounds, x0, y1)
	c11 := sampleTransparentPremultipliedRGBA(src, srcBounds, x1, y1)

	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	r := c00[0]*w00 + c10[0]*w10 + c01[0]*w01 + c11[0]*w11
	g := c00[1]*w00 + c10[1]*w10 + c01[1]*w01 + c11[1]*w11
	b := c00[2]*w00 + c10[2]*w10 + c01[2]*w01 + c11[2]*w11
	a := c00[3]*w00 + c10[3]*w10 + c01[3]*w01 + c11[3]*w11

	return color.RGBA{
		R: uint8(math.Round(math.Min(math.Max(r, 0), 1) * 255)),
		G: uint8(math.Round(math.Min(math.Max(g, 0), 1) * 255)),
		B: uint8(math.Round(math.Min(math.Max(b, 0), 1) * 255)),
		A: uint8(math.Round(math.Min(math.Max(a, 0), 1) * 255)),
	}
}

func sampleTransparentPremultipliedRGBA(src image.Image, srcBounds image.Rectangle, x, y int) [4]float64 {
	if x < srcBounds.Min.X || x >= srcBounds.Max.X || y < srcBounds.Min.Y || y >= srcBounds.Max.Y {
		return [4]float64{}
	}
	r, g, b, a := src.At(x, y).RGBA()
	alpha := float64(uint8(a/257)) / 255.0
	return [4]float64{
		float64(uint8(r/257)) / 255.0 * alpha,
		float64(uint8(g/257)) / 255.0 * alpha,
		float64(uint8(b/257)) / 255.0 * alpha,
		alpha,
	}
}

func compositeRGBAOverWhite(src color.RGBA) color.RGBA {
	alpha := float64(src.A) / 255.0
	inv := 1 - alpha
	return color.RGBA{
		R: uint8(math.Round(float64(src.R) + 255.0*inv)),
		G: uint8(math.Round(float64(src.G) + 255.0*inv)),
		B: uint8(math.Round(float64(src.B) + 255.0*inv)),
		A: 255,
	}
}

type flippedImage struct {
	src image.Image
}

func (f flippedImage) ColorModel() color.Model {
	return f.src.ColorModel()
}

func (f flippedImage) Bounds() image.Rectangle {
	return f.src.Bounds()
}

func (f flippedImage) At(x, y int) color.Color {
	bounds := f.src.Bounds()
	flippedY := bounds.Max.Y - 1 - (y - bounds.Min.Y)
	return f.src.At(x, flippedY)
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) <= imageDirectCopyEpsilon
}

func isNearlyInteger(v float64) bool {
	return nearlyEqual(v, math.Round(v))
}

func imgCoordMungeLower(v float64) int {
	return int(math.Floor(v))
}

func imgCoordMungeUpper(v float64) int {
	if nearlyEqual(v, math.Round(v)) {
		return int(math.Round(v)) + 1
	}
	return int(math.Ceil(v))
}

// canUseAxisAlignedSplashBilinear returns true when the image transform is axis-aligned
// (no rotation/skew) and upscaling in both dimensions — the condition under which
// Poppler's Splash uses scaleImageYupXupBilinear.
func (c *ImageCanvas) canUseAxisAlignedSplashBilinear(
	p00X, p00Y float64,
	p10X, p10Y float64,
	p01X, p01Y float64,
	srcW, srcH int,
) bool {
	if !nearlyEqual(p10Y, p00Y) || !nearlyEqual(p01X, p00X) {
		return false
	}
	if (p10X-p00X) <= 0 || (p01Y-p00Y) <= 0 {
		return false
	}
	// Compute Splash's scaledW/scaledH
	scaledW := imgCoordMungeUpper(p10X) - imgCoordMungeLower(p00X)
	scaledH := imgCoordMungeUpper(p01Y) - imgCoordMungeLower(p00Y)
	// Only use this path when actually upscaling (both dimensions)
	return scaledW >= srcW && scaledH >= srcH
}

// drawAxisAlignedSplashBilinear implements Poppler Splash's scaleImageYupXupBilinear exactly.
//
// Splash's internal bitmap is Y-DOWN (row 0 = top). It processes rows top-to-bottom with
// ySrc accumulated from 0, reading from the unflipped (Y-DOWN) source image.
// src must be the original unflipped image (row 0 = top of image).
func (c *ImageCanvas) drawAxisAlignedSplashBilinear(
	src image.Image,
	srcBounds image.Rectangle,
	p00X, p00Y, p10X, p01Y float64,
) {
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return
	}

	splashX0 := imgCoordMungeLower(p00X)
	splashX1 := imgCoordMungeUpper(p10X)
	splashY0 := imgCoordMungeLower(math.Min(p00Y, p01Y))
	splashY1 := imgCoordMungeUpper(math.Max(p00Y, p01Y))

	scaledW := splashX1 - splashX0
	scaledH := splashY1 - splashY0
	if scaledW <= 0 || scaledH <= 0 {
		return
	}

	xStep := float64(srcW) / float64(scaledW)
	yStep := float64(srcH) / float64(scaledH)

	srcMinX := srcBounds.Min.X
	srcMinY := srcBounds.Min.Y

	// expandRow expands Y-DOWN source row srcRow (0=top of image) into scaledW pixels.
	expandRow := func(srcRow int, outR, outG, outB, outA []uint8) {
		row := srcRow
		if row >= srcH {
			row = srcH - 1
		}
		xSrc := 0.0
		for x := 0; x < scaledW; x++ {
			xInt := int(xSrc)
			xFrac := xSrc - float64(xInt)
			x0 := xInt
			x1 := xInt + 1
			if x0 >= srcW {
				x0 = srcW - 1
			}
			if x1 >= srcW {
				x1 = srcW - 1
			}
			r0, g0, b0, a0 := src.At(srcMinX+x0, srcMinY+row).RGBA()
			r1, g1, b1, a1 := src.At(srcMinX+x1, srcMinY+row).RGBA()
			f0r, f0g, f0b, f0a := float64(r0>>8), float64(g0>>8), float64(b0>>8), float64(a0>>8)
			f1r, f1g, f1b, f1a := float64(r1>>8), float64(g1>>8), float64(b1>>8), float64(a1>>8)
			outR[x] = uint8(f0r*(1.0-xFrac) + f1r*xFrac)
			outG[x] = uint8(f0g*(1.0-xFrac) + f1g*xFrac)
			outB[x] = uint8(f0b*(1.0-xFrac) + f1b*xFrac)
			outA[x] = uint8(f0a*(1.0-xFrac) + f1a*xFrac)
			xSrc += xStep
		}
	}

	lineBuf1R := make([]uint8, scaledW)
	lineBuf1G := make([]uint8, scaledW)
	lineBuf1B := make([]uint8, scaledW)
	lineBuf1A := make([]uint8, scaledW)
	lineBuf2R := make([]uint8, scaledW)
	lineBuf2G := make([]uint8, scaledW)
	lineBuf2B := make([]uint8, scaledW)
	lineBuf2A := make([]uint8, scaledW)

	expandRow(0, lineBuf2R, lineBuf2G, lineBuf2B, lineBuf2A)
	currentSrcRow := -1

	canvasBounds := c.img.Bounds()
	// Compute canvas Y-top by converting Splash Y-UP top coord to Y-DOWN bitmap row,
	// matching Poppler's scaledYMin = imgCoordMungeLower(bitmap_yMin) where
	// bitmap_yMin = height - p01Y. Using c.height - splashY1 is wrong when p01Y is
	// exactly an integer because imgCoordMungeUpper adds 1 to exact integers.
	maxPY := math.Max(p00Y, p01Y)
	canvasYTop := imgCoordMungeLower(float64(c.height) - maxPY)

	ySrc := 0.0
	// Process rows top-to-bottom (Y-DOWN), matching Splash's bitmap write order.
	for i := 0; i < scaledH; i++ {
		yInt := int(ySrc)
		yFrac := ySrc - float64(yInt)

		for yInt > currentSrcRow {
			currentSrcRow++
			lineBuf1R, lineBuf2R = lineBuf2R, lineBuf1R
			lineBuf1G, lineBuf2G = lineBuf2G, lineBuf1G
			lineBuf1B, lineBuf2B = lineBuf2B, lineBuf1B
			lineBuf1A, lineBuf2A = lineBuf2A, lineBuf1A
			if currentSrcRow < srcH-1 {
				expandRow(currentSrcRow+1, lineBuf2R, lineBuf2G, lineBuf2B, lineBuf2A)
			} else {
				// Past last source row: clamp lineBuf2 to lineBuf1 (= last row)
				// so the y-blend stays on the last row instead of reading stale data.
				copy(lineBuf2R, lineBuf1R)
				copy(lineBuf2G, lineBuf1G)
				copy(lineBuf2B, lineBuf1B)
				copy(lineBuf2A, lineBuf1A)
			}
		}

		dy := canvasYTop + i
		if dy < canvasBounds.Min.Y || dy >= canvasBounds.Max.Y {
			ySrc += yStep
			continue
		}

		if c.clipMask == nil {
			for x := 0; x < scaledW; x++ {
				dx := splashX0 + x
				if dx < canvasBounds.Min.X || dx >= canvasBounds.Max.X {
					continue
				}
				r := uint8(float64(lineBuf1R[x])*(1.0-yFrac) + float64(lineBuf2R[x])*yFrac)
				g := uint8(float64(lineBuf1G[x])*(1.0-yFrac) + float64(lineBuf2G[x])*yFrac)
				b := uint8(float64(lineBuf1B[x])*(1.0-yFrac) + float64(lineBuf2B[x])*yFrac)
				a := uint8(float64(lineBuf1A[x])*(1.0-yFrac) + float64(lineBuf2A[x])*yFrac)
				srcC := color.RGBA{R: r, G: g, B: b, A: a}
				if srcC.A == 0 {
					continue
				}
				dstC := c.img.RGBAAt(dx, dy)
				c.img.SetRGBA(dx, dy, compositeOver(dstC, srcC))
			}
		} else {
			for x := 0; x < scaledW; x++ {
				dx := splashX0 + x
				if dx < canvasBounds.Min.X || dx >= canvasBounds.Max.X {
					continue
				}
				maskAlpha := c.clipMask.AlphaAt(dx, dy).A
				if maskAlpha == 0 {
					continue
				}
				r := uint8(float64(lineBuf1R[x])*(1.0-yFrac) + float64(lineBuf2R[x])*yFrac)
				g := uint8(float64(lineBuf1G[x])*(1.0-yFrac) + float64(lineBuf2G[x])*yFrac)
				b := uint8(float64(lineBuf1B[x])*(1.0-yFrac) + float64(lineBuf2B[x])*yFrac)
				a := uint8(float64(lineBuf1A[x])*(1.0-yFrac) + float64(lineBuf2A[x])*yFrac)
				srcC := color.RGBA{R: r, G: g, B: b, A: a}
				if srcC.A == 0 {
					continue
				}
				srcMasked := applyPremultipliedAlpha(srcC, maskAlpha)
				if srcMasked.A == 0 {
					continue
				}
				dstC := c.img.RGBAAt(dx, dy)
				c.img.SetRGBA(dx, dy, compositeOver(dstC, srcMasked))
			}
		}
		ySrc += yStep
	}
}
