package image

import (
	"bytes"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeJPEGWithDjpegGo_FeatureFlag(t *testing.T) {
	t.Setenv(enableDjpegGoJPEGEnv, "1")

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 8, 6))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 31),
				G: uint8(y * 37),
				B: uint8(64 + x + y),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))

	decoded, err := decodeJPEGWithDjpegGo(buf.Bytes())
	require.NoError(t, err)
	require.NotNil(t, decoded)
	require.Equal(t, img.Bounds(), decoded.Bounds())
}

func TestDecodeJPEGWithDjpegGo_Disabled(t *testing.T) {
	t.Setenv(enableDjpegGoJPEGEnv, "")

	decoded, err := decodeJPEGWithDjpegGo([]byte("not a jpeg"))
	require.Error(t, err)
	require.Nil(t, decoded)
}
