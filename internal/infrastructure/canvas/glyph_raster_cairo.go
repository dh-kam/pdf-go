package canvas

import "image"

// CairoRasterStrategy is kept as a source-compatible strategy name.
//
// The project no longer links against Cairo. Callers that explicitly select
// this strategy get the pure Go vector rasterizer instead.
type CairoRasterStrategy struct{}

func (s *CairoRasterStrategy) Name() string { return "cairo" }

// IsCairoAvailable returns false because Cairo is no longer linked.
func IsCairoAvailable() bool { return false }

// defaultCairoStrategyIfAvailable returns nil so ImageCanvas defaults to Go vector.
func defaultCairoStrategyIfAvailable() GlyphRasterStrategy { return nil }

func (s *CairoRasterStrategy) RasterizeGlyphMask(
	drawCmds []glyphDrawCommand,
	dstRect image.Rectangle,
	originCanvasX, originCanvasY float64,
	supersample int,
) *image.Alpha {
	return (&goVectorStrategy{}).RasterizeGlyphMask(drawCmds, dstRect, originCanvasX, originCanvasY, supersample)
}
