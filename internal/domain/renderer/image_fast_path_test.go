package renderer

import (
	stdimage "image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	infraimage "github.com/dh-kam/pdf-go/internal/infrastructure/image"
)

func TestNewPackedRawRGBImageForCanvasAcceptsSafeRawRGB(t *testing.T) {
	data := []byte{
		0x10, 0x20, 0x30,
		0x40, 0x50, 0x60,
	}
	imgData := &domainimage.ImageData{
		Data:             data,
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		Filter:           domainimage.FilterNone,
	}

	img, ok := newPackedRawRGBImageForCanvas(data, imgData, false, nil, nil)
	require.True(t, ok)
	rgb, ok := img.(*infraimage.RGBImage)
	require.True(t, ok)
	assert.Equal(t, 6, rgb.Stride)
	assert.Equal(t, data, rgb.Pix)
}

func TestNewPackedRawRGBImageForCanvasAcceptsSoftMaskedSafeRawRGB(t *testing.T) {
	data := []byte{
		0x10, 0x20, 0x30,
		0x40, 0x50, 0x60,
	}
	imgData := &domainimage.ImageData{
		Data:             data,
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		Filter:           domainimage.FilterNone,
		Mask:             infraimage.NewBitmapMask(2, 1, false),
	}

	img, ok := newPackedRawRGBImageForCanvas(data, imgData, false, nil, nil)
	require.True(t, ok)
	rgb, ok := img.(*infraimage.RGBImage)
	require.True(t, ok)
	assert.Equal(t, data, rgb.Pix)

	img, ok = newPackedRawRGBImageForCanvas(data, imgData, false, nil, []float64{1, 1, 1})
	assert.False(t, ok)
	assert.Nil(t, img)
}

func TestNewPackedRawRGBImageForCanvasRejectsAdjustedImages(t *testing.T) {
	data := []byte{0, 0, 0}
	imgData := &domainimage.ImageData{
		Data:             data,
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		Filter:           domainimage.FilterNone,
		Decode:           []float64{1, 0, 0, 1, 0, 1},
	}

	img, ok := newPackedRawRGBImageForCanvas(data, imgData, false, nil, nil)
	assert.False(t, ok)
	assert.Nil(t, img)
}

func TestNewPackedRawRGBMatteImageForCanvasUnblendsRowsLazily(t *testing.T) {
	data := []byte{
		64, 100, 200,
		10, 20, 30,
	}
	imgData := &domainimage.ImageData{
		Data:             data,
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		Filter:           domainimage.FilterNone,
	}
	maskImage := stdimage.NewGray(stdimage.Rect(0, 0, 2, 1))
	maskImage.Pix[0] = 128
	maskImage.Pix[1] = 255
	mask := infraimage.NewBitmapMaskFromImage(maskImage, false)

	img, ok := newPackedRawRGBMatteImageForCanvas(
		data,
		imgData,
		mask,
		[]float64{0, 0, 0},
		"DeviceRGB",
		colorspace.NewDeviceRGB(),
		false,
		nil,
	)

	require.True(t, ok)
	rower, ok := img.(interface {
		RGB8Row(y int, dst []byte) bool
	})
	require.True(t, ok)
	row := make([]byte, 6)
	require.True(t, rower.RGB8Row(0, row))
	assert.Equal(t, []byte{127, 199, 255, 10, 20, 30}, row)

	data[0] = 255
	require.True(t, rower.RGB8Row(0, row))
	assert.Equal(t, uint8(255), row[0])
}

func TestPackedRawRGBMatteImageRGB8RowUsesAlphaMaskBounds(t *testing.T) {
	data := []byte{
		64, 100, 200,
		10, 20, 30,
	}
	maskImage := stdimage.NewAlpha(stdimage.Rect(10, 20, 12, 21))
	maskImage.Pix[0] = 128
	maskImage.Pix[1] = 255
	img := &packedRawRGBMatteImage{
		pix:      data,
		stride:   6,
		rect:     stdimage.Rect(0, 0, 2, 1),
		mask:     maskImage,
		maskRect: maskImage.Rect,
		matteRGB: [3]uint8{0, 0, 0},
	}

	row := make([]byte, 6)
	require.True(t, img.RGB8Row(0, row))
	assert.Equal(t, []byte{127, 199, 255, 10, 20, 30}, row)
}

func TestPackedRawRGBMatteImageForCanvasCachesXObjectResult(t *testing.T) {
	e := NewEvaluator(nil)
	sourceStream := entity.NewStream(entity.NewDict(), []byte("source"))
	maskStream := entity.NewStream(entity.NewDict(), []byte("mask"))
	data := []byte{
		64, 100, 200,
		10, 20, 30,
	}
	imgData := &domainimage.ImageData{
		Data:             data,
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		Filter:           domainimage.FilterNone,
	}
	maskImage := stdimage.NewGray(stdimage.Rect(0, 0, 2, 1))
	maskImage.Pix[0] = 128
	maskImage.Pix[1] = 255
	mask := infraimage.NewBitmapMaskFromImage(maskImage, false)

	img, ok := e.packedRawRGBMatteImageForCanvas(
		sourceStream,
		maskStream,
		data,
		imgData,
		mask,
		[]float64{0, 0, 0},
		"DeviceRGB",
		colorspace.NewDeviceRGB(),
		false,
		nil,
	)
	require.True(t, ok)
	data[0] = 255

	cached, ok := e.packedRawRGBMatteImageForCanvas(
		sourceStream,
		maskStream,
		data,
		imgData,
		mask,
		[]float64{0, 0, 0},
		"DeviceRGB",
		colorspace.NewDeviceRGB(),
		false,
		nil,
	)
	require.True(t, ok)
	assert.Same(t, img, cached)
	rower, ok := cached.(interface {
		RGB8Row(y int, dst []byte) bool
	})
	require.True(t, ok)
	row := make([]byte, 6)
	require.True(t, rower.RGB8Row(0, row))
	assert.Equal(t, []byte{255, 199, 255, 10, 20, 30}, row)
}
