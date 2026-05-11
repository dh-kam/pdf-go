package image

import (
	stdimage "image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyColorKeyMask_NilMaskAndColorModels(t *testing.T) {
	src := stdimage.NewRGBA(stdimage.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	src.SetRGBA(1, 0, color.RGBA{R: 200, G: 210, B: 220, A: 255})

	unchanged, err := ApplyColorKeyMask(src, nil)
	require.NoError(t, err)
	assert.Equal(t, src, unchanged)

	mask := NewColorKeyMask([][2]uint8{{0, 20}, {0, 30}, {0, 40}}, 3)
	masked, err := ApplyColorKeyMask(src, mask)
	require.NoError(t, err)
	out := masked.(*stdimage.RGBA)

	assert.Equal(t, uint8(0), out.RGBAAt(0, 0).A)
	assert.Equal(t, uint8(255), out.RGBAAt(1, 0).A)

	gray := stdimage.NewGray(stdimage.Rect(0, 0, 1, 1))
	gray.SetGray(0, 0, color.Gray{Y: 7})
	grayMask := NewColorKeyMask([][2]uint8{{0, 10}}, 1)
	grayOut, err := ApplyColorKeyMask(gray, grayMask)
	require.NoError(t, err)
	assert.Equal(t, uint8(0), grayOut.(*stdimage.RGBA).RGBAAt(0, 0).A)

	ycbcr := stdimage.NewYCbCr(stdimage.Rect(0, 0, 1, 1), stdimage.YCbCrSubsampleRatio444)
	ycbcr.Y[0] = 16
	ycbcr.Cb[0] = 128
	ycbcr.Cr[0] = 128
	ycbcrMask := NewColorKeyMask([][2]uint8{{0, 20}, {0, 20}, {0, 20}}, 3)
	ycbcrOut, err := ApplyColorKeyMask(ycbcr, ycbcrMask)
	require.NoError(t, err)
	assert.Equal(t, uint8(0), ycbcrOut.(*stdimage.RGBA).RGBAAt(0, 0).A)
}

func TestApplyColorKeyMask_UnsupportedPixelFallsBack(t *testing.T) {
	alpha := stdimage.NewAlpha(stdimage.Rect(0, 0, 1, 1))
	alpha.SetAlpha(0, 0, color.Alpha{A: 120})

	mask := NewColorKeyMask([][2]uint8{{0, 255}, {0, 255}, {0, 255}}, 3)
	out, err := ApplyColorKeyMask(alpha, mask)
	require.NoError(t, err)

	_, _, _, a := out.At(0, 0).RGBA()
	assert.Equal(t, uint32(120)*257, a)
}

func TestDecodeMaskData_OutOfRangeByteFallback(t *testing.T) {
	normalMask, err := DecodeMaskData([]byte{0x80}, 9, 1, 1, false)
	require.NoError(t, err)
	normal := normalMask.Image().(*stdimage.Gray)
	assert.Equal(t, uint8(255), normal.GrayAt(8, 0).Y)

	invertedMask, err := DecodeMaskData([]byte{0x80}, 9, 1, 1, true)
	require.NoError(t, err)
	inverted := invertedMask.Image().(*stdimage.Gray)
	assert.Equal(t, uint8(0), inverted.GrayAt(8, 0).Y)

	_, err = DecodeMaskData([]byte{0x80}, 8, 1, 8, false)
	require.Error(t, err)
}
