// Package image_test provides tests for image decoding.
package image_test

import (
	"bytes"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image"
)

func createTestJPEGImage(width, height int) []byte {
	// Create a simple JPEG image
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))

	// Fill with a gradient
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(128)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Encode as JPEG
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

func TestNewJPEGDecoder(t *testing.T) {
	decoder := image.NewJPEGDecoder()
	assert.NotNil(t, decoder)
}

func TestJPEGDecoder_Decode(t *testing.T) {
	decoder := image.NewJPEGDecoder()

	jpegData := createTestJPEGImage(100, 100)
	require.NotEmpty(t, jpegData)

	img, err := decoder.Decode(jpegData)
	require.NoError(t, err)
	assert.NotNil(t, img)

	bounds := img.Bounds()
	assert.Equal(t, 100, bounds.Dx())
	assert.Equal(t, 100, bounds.Dy())
}

func TestJPEGDecoder_DecodeConfig(t *testing.T) {
	decoder := image.NewJPEGDecoder()

	jpegData := createTestJPEGImage(50, 75)
	require.NotEmpty(t, jpegData)

	cfg, err := decoder.DecodeConfig(jpegData)
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.Width)
	assert.Equal(t, 75, cfg.Height)
}

func TestJPEGDecoder_DecodeInvalid(t *testing.T) {
	decoder := image.NewJPEGDecoder()

	_, err := decoder.Decode([]byte("not a jpeg"))
	assert.Error(t, err)
}

func TestJPEGImage_ColorSpace(t *testing.T) {
	decoder := image.NewJPEGDecoder()
	jpegData := createTestJPEGImage(10, 10)
	require.NotEmpty(t, jpegData)

	cs := decoder.ColorSpace()
	assert.Equal(t, domainimage.ColorSpaceDeviceRGB, cs)
}

func TestNewJPEGImage(t *testing.T) {
	testImg := stdimage.NewRGBA(stdimage.Rect(0, 0, 50, 50))
	jpegImg := image.NewJPEGImage(testImg, domainimage.ColorSpaceDeviceRGB, 8)

	assert.NotNil(t, jpegImg)
	assert.Equal(t, 50, jpegImg.Width())
	assert.Equal(t, 50, jpegImg.Height())
	assert.Equal(t, domainimage.ColorSpaceDeviceRGB, jpegImg.ColorSpace())
	assert.Equal(t, 8, jpegImg.BitsPerComponent())
	assert.False(t, jpegImg.HasMask())
	assert.Nil(t, jpegImg.Mask())
}

func TestJPEGImage_SetMask(t *testing.T) {
	testImg := stdimage.NewRGBA(stdimage.Rect(0, 0, 50, 50))
	jpegImg := image.NewJPEGImage(testImg, domainimage.ColorSpaceDeviceRGB, 8)

	maskImg := stdimage.NewGray(stdimage.Rect(0, 0, 50, 50))
	mask := image.NewBitmapMaskFromImage(maskImg, false)

	jpegImg.SetMask(mask)
	assert.True(t, jpegImg.HasMask())
	assert.Equal(t, mask, jpegImg.Mask())
}

func TestNewBitmapMask(t *testing.T) {
	mask := image.NewBitmapMask(100, 100, false)
	assert.NotNil(t, mask)
	assert.False(t, mask.IsInverted())

	invertedMask := image.NewBitmapMask(100, 100, true)
	assert.True(t, invertedMask.IsInverted())
}

func TestBitmapMask_SetGetPixel(t *testing.T) {
	mask := image.NewBitmapMask(50, 50, false)

	mask.SetPixel(10, 20, 128)
	value := mask.GetPixel(10, 20)
	assert.Equal(t, uint8(128), value)
}

func TestNewBitmapMaskFromImage(t *testing.T) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, 30, 30))
	mask := image.NewBitmapMaskFromImage(img, true)

	assert.NotNil(t, mask)
	assert.True(t, mask.IsInverted())
	assert.Equal(t, img, mask.Image())
}

func TestDecodeMaskData(t *testing.T) {
	// Create 1-bit mask data (4x4 image)
	// Each byte represents 8 pixels
	data := []byte{
		0xAA, 0x55, // Row 0-1: 10101010, 01010101
		0xAA, 0x55, // Row 2-3: 10101010, 01010101
	}

	mask, err := image.DecodeMaskData(data, 16, 4, 1, false)
	require.NoError(t, err)
	assert.NotNil(t, mask)

	img := mask.Image().(*stdimage.Gray)
	assert.NotNil(t, img)

	// Check some pixel values
	assert.Equal(t, uint8(0), img.GrayAt(0, 0).Y)   // Bit 7 of 0xAA = 1, inverted to 0
	assert.Equal(t, uint8(255), img.GrayAt(1, 0).Y) // Bit 6 of 0xAA = 0, inverted to 255
}

func TestDecodeMaskData_Inverted(t *testing.T) {
	data := []byte{0xFF, 0x00}

	mask, err := image.DecodeMaskData(data, 16, 2, 1, true)
	require.NoError(t, err)

	img := mask.Image().(*stdimage.Gray)
	assert.NotNil(t, img)
	assert.True(t, mask.IsInverted())

	// With inverted, 1 bits = 255 (opaque)
	assert.Equal(t, uint8(255), img.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(255), img.GrayAt(7, 0).Y)
}

func TestNewColorKeyMask(t *testing.T) {
	colorRange := [][2]uint8{
		{0, 10}, // R: 0-10
		{0, 10}, // G: 0-10
		{0, 10}, // B: 0-10
	}

	mask := image.NewColorKeyMask(colorRange, 3)
	assert.NotNil(t, mask)
}

func TestColorKeyMask_IsTransparent(t *testing.T) {
	colorRange := [][2]uint8{
		{0, 10},
		{0, 10},
		{0, 10},
	}
	mask := image.NewColorKeyMask(colorRange, 3)

	// Color within range - should be transparent
	assert.True(t, mask.IsTransparent([]uint8{5, 5, 5}))

	// Color outside range - not transparent
	assert.False(t, mask.IsTransparent([]uint8{20, 5, 5}))
	assert.False(t, mask.IsTransparent([]uint8{5, 20, 5}))
	assert.False(t, mask.IsTransparent([]uint8{5, 5, 20}))
}

func TestNewDecoder(t *testing.T) {
	decoder := image.NewDecoder()
	assert.NotNil(t, decoder)
}

func TestDecoder_DecodeJPEG(t *testing.T) {
	decoder := image.NewDecoder()

	jpegData := createTestJPEGImage(100, 100)
	require.NotEmpty(t, jpegData)

	imgData := &domainimage.ImageData{
		Width:            100,
		Height:           100,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterDCT,
		Data:             jpegData,
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 100, result.Width())
	assert.Equal(t, 100, result.Height())
}

func TestDecoder_DecodeRawGray(t *testing.T) {
	decoder := image.NewDecoder()

	// Create simple 8-bit grayscale data (2x2)
	data := []byte{
		0x00, 0x80, // Row 0: black, gray
		0xFF, 0x40, // Row 1: white, dark gray
	}

	imgData := &domainimage.ImageData{
		Width:            2,
		Height:           2,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterNone,
		Data:             data,
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)
	assert.NotNil(t, result)

	img := result.Image().(*stdimage.Gray)
	require.NotNil(t, img)

	// Check pixel values
	assert.Equal(t, uint8(0), img.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(128), img.GrayAt(1, 0).Y)
	assert.Equal(t, uint8(255), img.GrayAt(0, 1).Y)
	assert.Equal(t, uint8(64), img.GrayAt(1, 1).Y)
}

func TestDecoder_DecodeRawRGB(t *testing.T) {
	decoder := image.NewDecoder()

	// Create simple 8-bit RGB data (2x1 pixel)
	// Pixel 0: red, Pixel 1: green
	data := []byte{
		0xFF, 0x00, 0x00, // Red pixel
		0x00, 0xFF, 0x00, // Green pixel
	}

	imgData := &domainimage.ImageData{
		Width:            2,
		Height:           1,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterNone,
		Data:             data,
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)
	assert.NotNil(t, result)

	img := result.Image().(*stdimage.RGBA)
	require.NotNil(t, img)

	// Check pixel values
	r0, g0, b0, _ := img.RGBAAt(0, 0).RGBA()
	assert.Equal(t, uint32(0xFFFF), r0)
	assert.Equal(t, uint32(0), g0)
	assert.Equal(t, uint32(0), b0)

	r1, g1, b1, _ := img.RGBAAt(1, 0).RGBA()
	assert.Equal(t, uint32(0), r1)
	assert.Equal(t, uint32(0xFFFF), g1)
	assert.Equal(t, uint32(0), b1)
}

func TestDecoder_DecodeRawCMYK(t *testing.T) {
	decoder := image.NewDecoder()

	// Create simple 8-bit CMYK data (1x1 pixel)
	// Cyan only
	data := []byte{
		0xFF, 0x00, 0x00, 0x00, // Cyan pixel
	}

	imgData := &domainimage.ImageData{
		Width:            1,
		Height:           1,
		ColorSpace:       domainimage.ColorSpaceDeviceCMYK,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterNone,
		Data:             data,
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)
	assert.NotNil(t, result)

	img := result.Image().(*stdimage.RGBA)
	require.NotNil(t, img)

	// Cyan should give us blueish color
	r, g, b, _ := img.RGBAAt(0, 0).RGBA()
	// Cyan (C=255, M=0, Y=0, K=0) with the current CMYK conversion curve.
	assert.Equal(t, uint32(0), r)
	assert.Equal(t, uint32(0xB8B8), g)
	assert.Equal(t, uint32(0xF1F1), b)
}

func TestDecoder_RegisterCustomDecoder(t *testing.T) {
	decoder := image.NewDecoder()

	customDecoder := image.NewJPEGDecoder()
	decoder.RegisterDecoder(domainimage.FilterJPX, customDecoder)

	// After registration, we should be able to retrieve it
	// (The decode will fail since JPX is not implemented, but registration works)
	imgData := &domainimage.ImageData{
		Width:            10,
		Height:           10,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterJPX,
		Data:             []byte("dummy data"),
	}

	_, err := decoder.Decode(imgData)
	assert.Error(t, err) // JPX not implemented in JPEG decoder
}

func TestDecoder_UnregisterDecoder(t *testing.T) {
	decoder := image.NewDecoder()

	// Unregister existing decoder
	decoder.UnregisterDecoder(domainimage.FilterDCT)

	imgData := &domainimage.ImageData{
		Width:            10,
		Height:           10,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterDCT,
		Data:             []byte("dummy data"),
	}

	_, err := decoder.Decode(imgData)
	assert.Error(t, err) // Decoder was unregistered
}

func TestDecoder_DecodeWithMask(t *testing.T) {
	decoder := image.NewDecoder()

	// Create simple RGB data
	data := []byte{
		0xFF, 0x00, 0x00, // Red
		0x00, 0xFF, 0x00, // Green
		0x00, 0x00, 0xFF, // Blue
		0xFF, 0xFF, 0x00, // Yellow
	}

	// Create mask data (2x2)
	maskData := []byte{0b11000000} // First two pixels opaque, last two transparent
	mask, _ := image.DecodeMaskData(maskData, 4, 2, 1, false)

	imgData := &domainimage.ImageData{
		Width:            2,
		Height:           2,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterNone,
		Data:             data,
		Mask:             mask,
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Result should have a mask
	assert.True(t, result.HasMask())
}

func TestDecodeArray(t *testing.T) {
	// This is an indirect test via the applyDecode function
	// A direct test would require exposing the function

	decoder := image.NewDecoder()

	// Create grayscale data
	data := []byte{0x00, 0x80, 0xFF}

	imgData := &domainimage.ImageData{
		Width:            3,
		Height:           1,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
		BitsPerComponent: 8,
		Filter:           domainimage.FilterNone,
		Data:             data,
		Decode:           []float64{255, 0}, // Invert: 0->255, 255->0
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)

	img := result.Image().(*stdimage.Gray)
	require.NotNil(t, img)

	// Values should be inverted
	assert.Equal(t, uint8(255), img.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(127), img.GrayAt(1, 0).Y) // Approx 128 inverted
	assert.Equal(t, uint8(0), img.GrayAt(2, 0).Y)
}

func TestApplyMask(t *testing.T) {
	// Create a red image
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	// Create a half-transparent mask
	maskImg := stdimage.NewGray(stdimage.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if x < 2 {
				maskImg.SetGray(x, y, color.Gray{Y: 255}) // Opaque
			} else {
				maskImg.SetGray(x, y, color.Gray{Y: 0}) // Transparent
			}
		}
	}

	mask := image.NewBitmapMaskFromImage(maskImg, false)
	result := image.ApplyMask(img, mask)

	resultRGBA := result.(*stdimage.RGBA)
	require.NotNil(t, resultRGBA)

	// Left half should be red and opaque
	r, g, b, a := resultRGBA.RGBAAt(0, 0).RGBA()
	assert.Equal(t, uint32(0xFFFF), r)
	assert.Equal(t, uint32(0), g)
	assert.Equal(t, uint32(0), b)
	assert.Equal(t, uint32(0xFFFF), a)

	// Right half should be transparent
	_, _, _, a2 := resultRGBA.RGBAAt(3, 0).RGBA()
	assert.Equal(t, uint32(0), a2)
}
