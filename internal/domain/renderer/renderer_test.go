package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRenderOptions(t *testing.T) {
	got := DefaultRenderOptions()
	assert.Equal(t, 72.0, got.DPI)
	assert.Equal(t, 1.0, got.Scale)
	assert.True(t, got.EnableCache)
	assert.False(t, got.DebugImageSampling)
	assert.Equal(t, ImageSamplingModeLegacy, got.ImageSamplingMode)
	assert.Nil(t, got.BackgroundColor)
}
