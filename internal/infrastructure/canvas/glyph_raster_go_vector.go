package canvas

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// goVectorStrategy uses Go's x/image/vector rasterizer (default).
type goVectorStrategy struct{}

func (s *goVectorStrategy) Name() string { return "go-vector" }

func (s *goVectorStrategy) RasterizeGlyphMask(
	drawCmds []glyphDrawCommand,
	dstRect image.Rectangle,
	originCanvasX, originCanvasY float64,
	supersample int,
) *image.Alpha {
	if supersample < 1 {
		supersample = 1
	}

	hiRect := image.Rect(0, 0, dstRect.Dx()*supersample, dstRect.Dy()*supersample)
	ras := vector.NewRasterizer(hiRect.Dx(), hiRect.Dy())

	var startX, startY float64
	hasStart := false
	scale := float64(supersample)
	for _, cmd := range drawCmds {
		switch cmd.kind {
		case entity.CmdMoveTo:
			if hasStart {
				ras.ClosePath()
			}
			startX = (cmd.x - float64(dstRect.Min.X)) * scale
			startY = (cmd.y - float64(dstRect.Min.Y)) * scale
			ras.MoveTo(float32(startX), float32(startY))
			hasStart = true
		case entity.CmdLineTo, entity.CmdCurveTo:
			if !hasStart {
				startX = (originCanvasX - float64(dstRect.Min.X)) * scale
				startY = (originCanvasY - float64(dstRect.Min.Y)) * scale
				ras.MoveTo(float32(startX), float32(startY))
				hasStart = true
			}
			if cmd.kind == entity.CmdLineTo {
				ras.LineTo(float32((cmd.x-float64(dstRect.Min.X))*scale), float32((cmd.y-float64(dstRect.Min.Y))*scale))
				continue
			}
			ras.CubeTo(
				float32((cmd.c1x-float64(dstRect.Min.X))*scale),
				float32((cmd.c1y-float64(dstRect.Min.Y))*scale),
				float32((cmd.c2x-float64(dstRect.Min.X))*scale),
				float32((cmd.c2y-float64(dstRect.Min.Y))*scale),
				float32((cmd.x-float64(dstRect.Min.X))*scale),
				float32((cmd.y-float64(dstRect.Min.Y))*scale),
			)
		case entity.CmdClose:
			if !hasStart {
				continue
			}
			ras.LineTo(float32(startX), float32(startY))
			ras.ClosePath()
		}
	}

	mask := image.NewAlpha(hiRect)
	ras.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	if supersample == 1 {
		return mask
	}
	return downsampleAlphaMaskStrategy(mask, supersample)
}

func downsampleAlphaMaskStrategy(src *image.Alpha, factor int) *image.Alpha {
	if factor <= 1 {
		return src
	}

	dstWidth := (src.Bounds().Dx() + factor - 1) / factor
	dstHeight := (src.Bounds().Dy() + factor - 1) / factor
	dst := image.NewAlpha(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < dstHeight; y++ {
		for x := 0; x < dstWidth; x++ {
			var sum uint32
			var count uint32
			for sy := y * factor; sy < glyphMinInt((y+1)*factor, src.Bounds().Dy()); sy++ {
				for sx := x * factor; sx < glyphMinInt((x+1)*factor, src.Bounds().Dx()); sx++ {
					sum += uint32(src.AlphaAt(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy).A)
					count++
				}
			}
			if count == 0 {
				continue
			}
			avg := sum / count
			// Skip the float round-trip when gamma == 1.0 (default). uint32(x*255)
			// truncation loses a 1 LSB on edge alphas (e.g. avg=1 → 0.00392 → 0.9996 → 0)
			// even when the math is mathematically identity. That off-by-one shows up
			// as a uniform (-1,-1,-1) residual on glyph anti-aliased edges (K2 finding).
			gamma := textGlyphGammaForDebug()
			if gamma != 1.0 && avg > 0 && avg < 255 {
				normalized := float64(avg) / 255.0
				boosted := math.Pow(normalized, gamma)
				avg = uint32(boosted * 255.0)
				if avg > 255 {
					avg = 255
				}
			}
			dst.SetAlpha(x, y, color.Alpha{A: uint8(avg)})
		}
	}

	return dst
}

func glyphMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
