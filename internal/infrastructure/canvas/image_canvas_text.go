package canvas

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// DrawText draws text at the specified position.
func (c *ImageCanvas) DrawText(text string, x, y float64, font entity.Font, fontSize float64) error {
	if font == nil || len(text) == 0 {
		return nil
	}

	fillColor := c.fillColor
	if fillColor == nil {
		fillColor = color.Black
	}

	unitsPerEm := float64(font.UnitsPerEm())
	if unitsPerEm <= 0 {
		unitsPerEm = 1000
	}
	bitmapRenderer, bitmapRendererOK := unwrapBitmapRenderer(font)
	phasedRenderer, phasedRendererOK := unwrapPhasedRenderer(font)
	transformedRenderer, transformedRendererOK := unwrapTransformedPhasedRenderer(font)
	bitmapScaleX, bitmapScaleY, bitmapTransformOK := c.textBitmapRenderScales()
	bitmapDPI, bitmapPathOK := c.textBitmapRenderDPI()
	bitmapFontSize := fontSize

	currentX := x
	currentY := y

	codes := splitTextCodes([]byte(text), font)

	for _, charCode := range codes {
		glyph, err := font.CharCodeToGlyph(charCode)
		if err != nil {
			continue
		}

		rendered := false
		if transformedRendererOK && bitmapTransformOK {
			phaseX := popplerGlyphXPhaseForFont(currentX, font, bitmapFontSize, bitmapScaleY)
			buf, bw, bh, bleft, btop, bitmapErr := transformedRenderer.RenderGlyphBitmapTransformedPhased(glyph, bitmapFontSize, bitmapScaleX, bitmapScaleY, phaseX, 0)
			if bitmapErr == nil {
				if len(buf) > 0 && bw > 0 && bh > 0 {
					c.renderGlyphBitmapPhased(buf, bw, bh, bleft, btop, currentX, currentY, fillColor)
				}
				rendered = true
			}
		} else if phasedRendererOK && bitmapPathOK {
			// X sub-pixel phase: fractional cursor position within a pixel.
			// Y phase is deliberately 0: applying Y translation changes bitmap_top by -1
			// (boundary crossing), causing a systematic 1-pixel downward shift.
			scale := float64(bitmapDPI) / 72.0
			phaseX := popplerGlyphXPhaseForFont(currentX, font, bitmapFontSize, scale)
			buf, bw, bh, bleft, btop, bitmapErr := phasedRenderer.RenderGlyphBitmapPhased(glyph, bitmapFontSize, bitmapDPI, phaseX, 0)
			if bitmapErr == nil {
				if len(buf) > 0 && bw > 0 && bh > 0 {
					c.renderGlyphBitmapPhased(buf, bw, bh, bleft, btop, currentX, currentY, fillColor)
				}
				rendered = true
			}
		} else if bitmapRendererOK && bitmapPathOK {
			buf, bw, bh, bleft, btop, bitmapErr := bitmapRenderer.RenderGlyphBitmap(glyph, bitmapFontSize, bitmapDPI)
			if bitmapErr == nil {
				if len(buf) > 0 && bw > 0 && bh > 0 {
					c.renderGlyphBitmap(buf, bw, bh, bleft, btop, currentX, currentY, fillColor)
				}
				rendered = true
			}
		}

		if !rendered {
			glyphPath, pathErr := font.RenderGlyph(glyph, fontSize)
			if pathErr != nil {
				if !isWhitespaceGlyphCode(charCode) {
					c.renderGlyphFallback(currentX, currentY, fontSize, fillColor)
				}
			} else {
				c.renderGlyphPath(glyphPath, currentX, currentY, fillColor)
			}
		}

		width, err := font.GetGlyphWidth(glyph)
		if err != nil {
			width = 500
		}

		advance := (width / unitsPerEm) * fontSize
		currentX += advance
	}

	return nil
}

func (c *ImageCanvas) textBitmapRenderDPI() (int, bool) {
	scale, ok := c.textBitmapRenderScale()
	if !ok {
		return 0, false
	}

	dpi := int(math.Round(72 * scale))
	if dpi <= 0 {
		return 0, false
	}
	return dpi, true
}

func (c *ImageCanvas) textBitmapRenderScales() (float64, float64, bool) {
	gt := c.glyphTransform
	if math.Abs(gt[1]) > 1e-6 || math.Abs(gt[2]) > 1e-6 {
		return 0, 0, false
	}

	scaleX := math.Abs(gt[0])
	scaleY := math.Abs(gt[3])
	if scaleX <= 0 || scaleY <= 0 {
		return 0, 0, false
	}
	return scaleX, scaleY, true
}

func (c *ImageCanvas) textBitmapRenderScale() (float64, bool) {
	scaleX, scaleY, ok := c.textBitmapRenderScales()
	if !ok {
		return 0, false
	}
	if math.Abs(scaleX-scaleY) > 0.01 {
		return 0, false
	}

	return (scaleX + scaleY) / 2, true
}

func popplerGlyphXPhase(x float64) float64 {
	frac := x - math.Floor(x)
	phase := math.Floor(frac * 4)
	if phase < 0 {
		return 0
	}
	if phase > 3 {
		return 0.75
	}
	return phase / 4
}

func popplerGlyphXPhaseForFont(x float64, font entity.Font, fontSize, scaleY float64) float64 {
	if popplerGlyphCacheHeight(font, fontSize, scaleY) > 50 {
		return 0
	}
	return popplerGlyphXPhase(x)
}

func popplerGlyphCacheHeight(font entity.Font, fontSize, scaleY float64) float64 {
	if font == nil || fontSize <= 0 || scaleY <= 0 {
		return 0
	}

	_, yMin, _, yMax := font.GetBoundingBox()
	unitsPerEm := float64(font.UnitsPerEm())
	if unitsPerEm <= 0 {
		unitsPerEm = 1000
	}

	return math.Abs(yMax-yMin)/unitsPerEm*fontSize*scaleY + 3
}

func isWhitespaceGlyphCode(charCode uint32) bool {
	if charCode <= 0x20 {
		return true
	}

	switch charCode {
	case 0xA0, 0x1680, 0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200A, 0x202F, 0x3000:
		return true
	}

	return false
}

func splitTextCodes(raw []byte, font entity.Font) []uint32 {
	if len(raw) == 0 {
		return nil
	}

	if font != nil && font.IsCIDFont() {
		out := make([]uint32, 0, (len(raw)+1)/2)
		for i := 0; i < len(raw); {
			if i+1 < len(raw) {
				out = append(out, uint32(raw[i])<<8|uint32(raw[i+1]))
				i += 2
				continue
			}
			out = append(out, uint32(raw[i]))
			i++
		}
		return out
	}

	out := make([]uint32, 0, len(raw))
	for _, b := range raw {
		out = append(out, uint32(b))
	}
	return out
}

func (c *ImageCanvas) renderGlyphPath(glyphPath *entity.GlyphPath, x, y float64, fillColor color.Color) {
	if glyphPath == nil {
		return
	}

	commands := glyphPath.Commands
	if len(commands) == 0 {
		return
	}

	drawCmds := make([]glyphDrawCommand, 0, len(commands))
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	hasPoint := false

	baseCanvasX := x - float64(c.img.Bounds().Min.X)
	baseCanvasY := math.Floor(c.glyphYOrigin()-y+0.5) + float64(c.img.Bounds().Min.Y)
	// glyphTransform scales/rotates glyph path coords from user space to device space.
	gt := c.glyphTransform
	transformGlyphPoint := func(gx, gy float64) (float64, float64) {
		tx := gt[0]*gx + gt[2]*gy
		ty := gt[1]*gx + gt[3]*gy
		return baseCanvasX + tx, baseCanvasY + ty
	}
	originCanvasX, originCanvasY := transformGlyphPoint(0, 0)

	for _, cmd := range commands {
		switch cmd.Type() {
		case entity.CmdMoveTo:
			if moveCmd, ok := cmd.(*entity.PathMoveTo); ok {
				scaledX, scaledY := transformGlyphPoint(moveCmd.X, moveCmd.Y)
				drawCmds = append(drawCmds, glyphDrawCommand{kind: entity.CmdMoveTo, x: scaledX, y: scaledY})
				minX, maxX = math.Min(minX, scaledX), math.Max(maxX, scaledX)
				minY, maxY = math.Min(minY, scaledY), math.Max(maxY, scaledY)
				hasPoint = true
			}
		case entity.CmdLineTo:
			if lineCmd, ok := cmd.(*entity.PathLineTo); ok {
				scaledX, scaledY := transformGlyphPoint(lineCmd.X, lineCmd.Y)
				drawCmds = append(drawCmds, glyphDrawCommand{kind: entity.CmdLineTo, x: scaledX, y: scaledY})
				minX, maxX = math.Min(minX, scaledX), math.Max(maxX, scaledX)
				minY, maxY = math.Min(minY, scaledY), math.Max(maxY, scaledY)
				hasPoint = true
			}
		case entity.CmdCurveTo:
			if curveCmd, ok := cmd.(*entity.PathCurveTo); ok {
				scaledX1, scaledY1 := transformGlyphPoint(curveCmd.X1, curveCmd.Y1)
				scaledX2, scaledY2 := transformGlyphPoint(curveCmd.X2, curveCmd.Y2)
				scaledX, scaledY := transformGlyphPoint(curveCmd.X3, curveCmd.Y3)
				drawCmds = append(drawCmds, glyphDrawCommand{
					kind: entity.CmdCurveTo,
					x:    scaledX,
					y:    scaledY,
					c1x:  scaledX1,
					c1y:  scaledY1,
					c2x:  scaledX2,
					c2y:  scaledY2,
				})
				minX, maxX = math.Min(minX, scaledX), math.Max(maxX, scaledX)
				minY, maxY = math.Min(minY, scaledY), math.Max(maxY, scaledY)
				minX, maxX = math.Min(minX, scaledX1), math.Max(maxX, scaledX1)
				minY, maxY = math.Min(minY, scaledY1), math.Max(maxY, scaledY1)
				minX, maxX = math.Min(minX, scaledX2), math.Max(maxX, scaledX2)
				minY, maxY = math.Min(minY, scaledY2), math.Max(maxY, scaledY2)
				hasPoint = true
			}
		case entity.CmdClose:
			drawCmds = append(drawCmds, glyphDrawCommand{kind: entity.CmdClose})
		}
	}
	if !hasPoint {
		return
	}

	clampedMinX := math.Max(0, math.Floor(minX))
	clampedMinY := math.Max(0, math.Floor(minY))
	clampedMaxX := math.Min(float64(c.width), math.Ceil(maxX))
	clampedMaxY := math.Min(float64(c.height), math.Ceil(maxY))
	if clampedMaxX <= clampedMinX || clampedMaxY <= clampedMinY {
		return
	}

	dstRect := image.Rect(int(clampedMinX), int(clampedMinY), int(clampedMaxX), int(clampedMaxY))
	mask := c.activeGlyphRasterStrategy().RasterizeGlyphMask(drawCmds, dstRect, originCanvasX, originCanvasY, textGlyphSupersampleFactorForDebug())
	draw.DrawMask(c.img, dstRect, &image.Uniform{C: fillColor}, image.Point{}, mask, image.Point{}, draw.Over)
}

// unwrapBitmapRenderer checks if the font (or any wrapped base font) implements BitmapGlyphRenderer.
func unwrapBitmapRenderer(font entity.Font) (entity.BitmapGlyphRenderer, bool) {
	if br, ok := font.(entity.BitmapGlyphRenderer); ok {
		return br, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		return unwrapBitmapRenderer(u.BaseFont())
	}
	return nil, false
}

// unwrapPhasedRenderer checks if the font (or any wrapped base font) implements BitmapGlyphRendererPhased.
func unwrapPhasedRenderer(font entity.Font) (entity.BitmapGlyphRendererPhased, bool) {
	if pr, ok := font.(entity.BitmapGlyphRendererPhased); ok {
		return pr, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		return unwrapPhasedRenderer(u.BaseFont())
	}
	return nil, false
}

type transformedPhasedGlyphRenderer interface {
	RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error)
}

func unwrapTransformedPhasedRenderer(font entity.Font) (transformedPhasedGlyphRenderer, bool) {
	if pr, ok := font.(transformedPhasedGlyphRenderer); ok {
		return pr, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		return unwrapTransformedPhasedRenderer(u.BaseFont())
	}
	return nil, false
}

// renderGlyphBitmapPhased places a phase-corrected glyph bitmap using Floor for exact placement.
func (c *ImageCanvas) renderGlyphBitmapPhased(buf []byte, bw, bh, bleft, btop int, textX, textY float64, fillColor color.Color) {
	canvasX := textX - float64(c.img.Bounds().Min.X) + float64(bleft)
	canvasY := c.glyphYOrigin() - textY + float64(c.img.Bounds().Min.Y) - float64(btop)
	startX := int(math.Floor(canvasX)) // Floor: bitmap was rendered at the correct sub-pixel phase
	startY := int(math.Floor(canvasY))

	r32, g32, b32, a32 := fillColor.RGBA()
	fr := uint8(r32 >> 8)
	fg := uint8(g32 >> 8)
	fb := uint8(b32 >> 8)
	fa := uint8(a32 >> 8)

	for dy := 0; dy < bh; dy++ {
		for dx := 0; dx < bw; dx++ {
			alpha := buf[dy*bw+dx]
			if alpha == 0 {
				continue
			}
			px := startX + dx
			py := startY + dy
			if px < 0 || py < 0 || px >= c.width || py >= c.height {
				continue
			}
			c.img.SetRGBA(px, py, c.blendGlyphBitmapPixel(c.img.RGBAAt(px, py), fr, fg, fb, fa, alpha))
		}
	}
}

// renderGlyphBitmap draws a pre-rendered glyph bitmap onto the canvas.
func (c *ImageCanvas) renderGlyphBitmap(buf []byte, bw, bh, bleft, btop int, textX, textY float64, fillColor color.Color) {
	// Convert text position to canvas coordinates
	canvasX := textX - float64(c.img.Bounds().Min.X) + float64(bleft)
	canvasY := c.glyphYOrigin() - textY + float64(c.img.Bounds().Min.Y) - float64(btop)
	startX := int(math.Round(canvasX))
	startY := int(math.Floor(canvasY))

	r32, g32, b32, a32 := fillColor.RGBA()
	fr := uint8(r32 >> 8)
	fg := uint8(g32 >> 8)
	fb := uint8(b32 >> 8)
	fa := uint8(a32 >> 8)

	for dy := 0; dy < bh; dy++ {
		for dx := 0; dx < bw; dx++ {
			alpha := buf[dy*bw+dx]
			if alpha == 0 {
				continue
			}
			px := startX + dx
			py := startY + dy
			if px < 0 || py < 0 || px >= c.width || py >= c.height {
				continue
			}
			c.img.SetRGBA(px, py, c.blendGlyphBitmapPixel(c.img.RGBAAt(px, py), fr, fg, fb, fa, alpha))
		}
	}
}

func (c *ImageCanvas) blendGlyphBitmapPixel(dst color.RGBA, fr, fg, fb, fillAlpha, maskAlpha uint8) color.RGBA {
	aSrc := splashDiv255(uint32(fillAlpha) * uint32(maskAlpha))
	if aSrc == 0 {
		return dst
	}
	src := color.RGBA{R: fr, G: fg, B: fb, A: 255}
	if aSrc == 255 {
		return c.applyColorTransfer(src)
	}
	if c.isOpaquePaperPixel(dst) {
		transferred := c.applyColorTransfer(src)
		alpha := uint32(aSrc)
		alpha1 := uint32(255 - aSrc)
		return color.RGBA{
			R: splashDiv255(alpha1*uint32(c.paperColor.R) + alpha*uint32(transferred.R)),
			G: splashDiv255(alpha1*uint32(c.paperColor.G) + alpha*uint32(transferred.G)),
			B: splashDiv255(alpha1*uint32(c.paperColor.B) + alpha*uint32(transferred.B)),
			A: 255,
		}
	}

	a := uint32(aSrc)
	inv := uint32(255 - aSrc)
	nr := splashDiv255(uint32(fr)*a + uint32(dst.R)*inv)
	ng := splashDiv255(uint32(fg)*a + uint32(dst.G)*inv)
	nb := splashDiv255(uint32(fb)*a + uint32(dst.B)*inv)
	na := a + uint32(splashDiv255(uint32(dst.A)*inv))
	if na > 255 {
		na = 255
	}
	return c.applyColorTransfer(color.RGBA{R: nr, G: ng, B: nb, A: uint8(na)})
}

func (c *ImageCanvas) isOpaquePaperPixel(col color.RGBA) bool {
	return c.paperColorActive &&
		col.A == 255 &&
		col.R == c.paperColor.R &&
		col.G == c.paperColor.G &&
		col.B == c.paperColor.B
}

func (c *ImageCanvas) renderGlyphFallback(x, y, fontSize float64, fillColor color.Color) {
	width := fontSize * 0.5
	height := fontSize * 0.7

	x0 := int(x)
	y0 := int(y - height)
	x1 := int(x + width)
	y1 := int(y)

	bounds := c.img.Bounds()
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if y0 < bounds.Min.Y {
		y0 = bounds.Min.Y
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}

	draw.Draw(c.img, image.Rect(x0, y0, x1, y1), &image.Uniform{fillColor}, image.Point{}, draw.Over)
}

// BeginText begins a text block at the specified position.
func (c *ImageCanvas) BeginText(x, y float64) {
	c.textPosition = [2]float64{x, y}
	c.inTextBlock = true
}

// EndText ends the current text block.
func (c *ImageCanvas) EndText() {
	c.inTextBlock = false
}

// ShowText shows text at the current text position.
func (c *ImageCanvas) ShowText(text string) error {
	return nil
}

// MoveTextPoint moves the text position by (tx, ty).
func (c *ImageCanvas) MoveTextPoint(tx, ty float64) {
	c.textPosition[0] += tx
	c.textPosition[1] += ty
}
