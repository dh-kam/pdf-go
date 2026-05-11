package jpx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapper_DelegatesToDecoder(t *testing.T) {
	w := NewWrapper()

	cfg, err := w.DecodeConfig([]byte{
		0x00, 0x00, 0x00, 0x0C,
		0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
	})
	require.NoError(t, err)
	assert.True(t, cfg.Width > 0)
	assert.True(t, cfg.Height > 0)
	assert.Equal(t, "DeviceRGB", string(w.ColorSpace()))
}

func TestWrapper_DecodeFallbackPath(t *testing.T) {
	w := NewWrapper()
	img, err := w.Decode([]byte{
		0x00, 0x00, 0x00, 0x0C,
		0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
	})
	require.NoError(t, err)
	assert.NotNil(t, img)
}
