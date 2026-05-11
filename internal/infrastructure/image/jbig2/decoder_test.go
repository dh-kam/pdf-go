package jbig2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapper_DelegatesToDecoder(t *testing.T) {
	w := NewWrapper()

	cfg, err := w.DecodeConfig([]byte{
		0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A,
	})
	require.NoError(t, err)
	assert.True(t, cfg.Width > 0)
	assert.True(t, cfg.Height > 0)

	assert.Equal(t, "DeviceGray", string(w.ColorSpace()))
}

func TestWrapper_DecodeFallbackPath(t *testing.T) {
	w := NewWrapper()
	img, err := w.Decode([]byte{
		0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x1B, // intentionally invalid header checksum
	})
	require.NoError(t, err)
	assert.NotNil(t, img)
}
