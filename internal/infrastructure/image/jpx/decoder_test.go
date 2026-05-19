package jpx

import (
	"bytes"
	stdimage "image"
	"image/color"
	"testing"

	jpeg2000 "github.com/ajroetker/go-jpeg2000"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapper_DelegatesToDecoder(t *testing.T) {
	w := NewWrapper()

	cfg, err := w.DecodeConfig(testJP2Data(t))
	require.NoError(t, err)
	assert.Equal(t, 4, cfg.Width)
	assert.Equal(t, 4, cfg.Height)
	assert.Equal(t, "DeviceRGB", string(w.ColorSpace()))
}

func TestNativeDecoder_DecodesValidJP2(t *testing.T) {
	decoder := NewNativeDecoder()

	img, err := decoder.Decode(testJP2Data(t))
	require.NoError(t, err)
	assert.Equal(t, stdimage.Rect(0, 0, 4, 4), img.Bounds())
}

func TestNativeDecoder_DecodesRawCodestream(t *testing.T) {
	decoder := NewNativeDecoder()
	data := testJ2KData(t)

	img, err := decoder.Decode(data)
	require.NoError(t, err)
	assert.True(t, decoder.CanDecode(data))
	assert.Equal(t, stdimage.Rect(0, 0, 4, 4), img.Bounds())
}

func TestNativeDecoder_RejectsHeaderOnlyJP2(t *testing.T) {
	decoder := NewNativeDecoder()

	_, err := decoder.Decode([]byte{
		0x00, 0x00, 0x00, 0x0C,
		0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
	})
	require.Error(t, err)
}

func testJP2Data(t *testing.T) []byte {
	return testJPEG2000Data(t, jpeg2000.FormatJP2)
}

func testJ2KData(t *testing.T) []byte {
	return testJPEG2000Data(t, jpeg2000.FormatJ2K)
}

func testJPEG2000Data(t *testing.T, format jpeg2000.FileFormat) []byte {
	t.Helper()

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(10 + x*20),
				G: uint8(30 + y*20),
				B: uint8(90 + x*10 + y*5),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	err := jpeg2000.Encode(&buf, img, &jpeg2000.EncodeOptions{
		Lossless:       true,
		NumResolutions: 1,
		FileFormat:     format,
	})
	require.NoError(t, err)
	return buf.Bytes()
}
