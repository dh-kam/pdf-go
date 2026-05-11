package canvas

import "image"

// GlyphRasterStrategy defines the interface for glyph rasterization.
// Implementations control how glyph outlines are converted to alpha masks.
// Use SetGlyphRasterStrategy on ImageCanvas to switch between strategies.
type GlyphRasterStrategy interface {
	// RasterizeGlyphMask converts draw commands to an alpha mask.
	RasterizeGlyphMask(
		drawCmds []glyphDrawCommand,
		dstRect image.Rectangle,
		originCanvasX, originCanvasY float64,
		supersample int,
	) *image.Alpha

	// Name returns a human-readable name for this strategy.
	Name() string
}

// SetGlyphRasterStrategy sets the glyph rasterization strategy.
// Pass nil to use the default Go vector rasterizer.
func (c *ImageCanvas) SetGlyphRasterStrategy(s GlyphRasterStrategy) {
	c.glyphRasterStrategy = s
}

// activeGlyphRasterStrategy returns the current strategy.
// Priority: explicit setting > Cairo (if available) > Go vector.
func (c *ImageCanvas) activeGlyphRasterStrategy() GlyphRasterStrategy {
	if c.glyphRasterStrategy != nil {
		return c.glyphRasterStrategy
	}
	if s := defaultCairoStrategyIfAvailable(); s != nil {
		return s
	}
	return &goVectorStrategy{}
}
