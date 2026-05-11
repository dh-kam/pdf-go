package canvas

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
)

type imagePlacementProbeResult struct {
	dstRect    image.Rectangle
	dstMinX    float64
	dstMinY    float64
	dstMaxX    float64
	dstMaxY    float64
	transform  [6]float64
}

func (r imagePlacementProbeResult) translationX() float64 {
	return r.transform[2]
}

func (r imagePlacementProbeResult) translationY() float64 {
	return r.transform[5]
}

func (r imagePlacementProbeResult) width() float64 {
	return r.dstMaxX - r.dstMinX
}

func (r imagePlacementProbeResult) height() float64 {
	return r.dstMaxY - r.dstMinY
}

func measureImagePlacementProbeForCanvas(
	c *ImageCanvas,
	src image.Image,
	x, y, width, height float64,
	phaseX, phaseY float64,
) imagePlacementProbeResult {
	srcBounds := src.Bounds()
	srcWidth := float64(srcBounds.Dx())
	srcHeight := float64(srcBounds.Dy())

	p00X, p00Y := c.transformPoint(x, y)
	p10X, p10Y := c.transformPoint(x+width, y)
	p01X, p01Y := c.transformPoint(x, y+height)
	p11X, p11Y := c.transformPoint(x+width, y+height)

	minX := minFloatForPlacementProbe(p00X, p10X, p01X, p11X)
	maxX := maxFloatForPlacementProbe(p00X, p10X, p01X, p11X)
	minY := minFloatForPlacementProbe(
		float64(c.height)-p00Y,
		float64(c.height)-p10Y,
		float64(c.height)-p01Y,
		float64(c.height)-p11Y,
	)
	maxY := maxFloatForPlacementProbe(
		float64(c.height)-p00Y,
		float64(c.height)-p10Y,
		float64(c.height)-p01Y,
		float64(c.height)-p11Y,
	)

	uScaleX := (p10X - p00X) / srcWidth
	uScaleY := (p10Y - p00Y) / srcWidth
	vScaleX := (p01X - p00X) / srcHeight
	vScaleY := (p01Y - p00Y) / srcHeight

	return imagePlacementProbeResult{
		dstRect: image.Rect(
			int(minX),
			int(minY),
			int(maxX+0.999999999),
			int(maxY+0.999999999),
		),
		dstMinX: minX,
		dstMinY: minY,
		dstMaxX: maxX,
		dstMaxY: maxY,
		transform: [6]float64{
			uScaleX,
			vScaleX,
			p00X + uScaleX*phaseX + vScaleX*phaseY,
			-uScaleY,
			-vScaleY,
			float64(c.height) - p00Y - (uScaleY*phaseX + vScaleY*phaseY),
		},
	}
}

func minFloatForPlacementProbe(a, b, c, d float64) float64 {
	if a > b {
		a = b
	}
	if a > c {
		a = c
	}
	if a > d {
		a = d
	}
	return a
}

func maxFloatForPlacementProbe(a, b, c, d float64) float64 {
	if a < b {
		a = b
	}
	if a < c {
		a = c
	}
	if a < d {
		a = d
	}
	return a
}

func TestMeasureImagePlacementProbeForCanvas_TinyGray16To8UsesQuarterPixelTranslation(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	src := image.NewGray(image.Rect(0, 0, 16, 16))

	result := measureImagePlacementProbeForCanvas(c, src, 0, 0, 8, 8, 0.5, 0.5)

	assert.Equal(t, image.Rect(0, 0, 8, 8), result.dstRect)
	assert.InDelta(t, 8.0, result.width(), 1e-9)
	assert.InDelta(t, 8.0, result.height(), 1e-9)
	assert.InDelta(t, 0.5, result.transform[0], 1e-9)
	assert.InDelta(t, -0.5, result.transform[4], 1e-9)
	assert.InDelta(t, 0.25, result.translationX(), 1e-9)
	assert.InDelta(t, 7.75, result.translationY(), 1e-9)
}

func TestMeasureImagePlacementProbeForCanvas_TinyGray16To4UsesHalfPixelTranslation(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 4, 4)).(*ImageCanvas)
	src := image.NewGray(image.Rect(0, 0, 16, 16))

	result := measureImagePlacementProbeForCanvas(c, src, 0, 0, 4, 4, 0.5, 0.5)

	assert.Equal(t, image.Rect(0, 0, 4, 4), result.dstRect)
	assert.InDelta(t, 4.0, result.width(), 1e-9)
	assert.InDelta(t, 4.0, result.height(), 1e-9)
	assert.InDelta(t, 0.25, result.transform[0], 1e-9)
	assert.InDelta(t, -0.25, result.transform[4], 1e-9)
	assert.InDelta(t, 0.125, result.translationX(), 1e-9)
	assert.InDelta(t, 3.875, result.translationY(), 1e-9)
}
