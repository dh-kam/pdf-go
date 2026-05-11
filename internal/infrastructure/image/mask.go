// Package image provides image mask implementation.
//
//revive:disable:exported,var-naming
package image

import (
	stdimage "image"
	"image/color"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/image"
)

// BitmapMask represents a bitmap image mask.
type BitmapMask struct {
	img      *stdimage.Gray
	inverted bool
}

// NewBitmapMask creates a new bitmap mask.
func NewBitmapMask(width, height int, inverted bool) *BitmapMask {
	return &BitmapMask{
		img:      stdimage.NewGray(stdimage.Rect(0, 0, width, height)),
		inverted: inverted,
	}
}

// NewBitmapMaskFromImage creates a bitmap mask from an existing image.
func NewBitmapMaskFromImage(img *stdimage.Gray, inverted bool) *BitmapMask {
	return &BitmapMask{
		img:      img,
		inverted: inverted,
	}
}

// Image returns the mask as an image.Image.
func (m *BitmapMask) Image() stdimage.Image {
	return m.img
}

// IsInverted returns true if the mask is inverted.
func (m *BitmapMask) IsInverted() bool {
	return m.inverted
}

// SetPixel sets a pixel in the mask.
func (m *BitmapMask) SetPixel(x, y int, value uint8) {
	m.img.SetGray(x, y, color.Gray{Y: value})
}

// GetPixel gets a pixel value from the mask.
func (m *BitmapMask) GetPixel(x, y int) uint8 {
	return m.img.GrayAt(x, y).Y
}

// ApplyMask applies the mask to an image.
func ApplyMask(img stdimage.Image, mask image.ImageMask) stdimage.Image {
	if mask == nil {
		return img
	}

	maskImg, ok := mask.Image().(*stdimage.Gray)
	if !ok {
		return img
	}

	bounds := img.Bounds()
	if bounds.Empty() {
		return img
	}
	maskBounds := maskImg.Bounds()
	if maskBounds.Empty() {
		return img
	}

	result := stdimage.NewRGBA(bounds)

	inverted := mask.IsInverted()
	maskW := maskBounds.Dx()
	maskH := maskBounds.Dy()
	imgW := bounds.Dx()
	imgH := bounds.Dy()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			original := img.At(x, y)
			mx := maskBounds.Min.X + (x-bounds.Min.X)*maskW/imgW
			my := maskBounds.Min.Y + (y-bounds.Min.Y)*maskH/imgH
			maskValue := maskImg.GrayAt(mx, my).Y

			var alpha uint8
			if inverted {
				// Inverted: 255 = opaque, 0 = transparent (invert the mask value)
				alpha = 255 - maskValue
			} else {
				// Normal: mask value directly (0 = transparent, 255 = opaque)
				alpha = maskValue
			}

			// Apply alpha to the original color
			r, g, b, oa := original.RGBA()
			var a uint32
			alpha16 := uint32(alpha) * 257 // Convert to 16-bit

			// Blend with transparency while preserving source alpha.
			r = (r * alpha16) / 65535
			g = (g * alpha16) / 65535
			b = (b * alpha16) / 65535
			a = (oa * alpha16) / 65535

			result.SetRGBA(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}

	return result
}

// ColorKeyMask represents a color key mask.
type ColorKeyMask struct {
	colorRange [][2]uint8 // [min, max] for each color component
	colorCount int        // Number of color components (1 for gray, 3 for RGB, 4 for CMYK)
}

// NewColorKeyMask creates a new color key mask.
func NewColorKeyMask(colorRange [][2]uint8, colorCount int) *ColorKeyMask {
	return &ColorKeyMask{
		colorRange: colorRange,
		colorCount: colorCount,
	}
}

// IsTransparent returns true if the given color is transparent (within the key range).
func (m *ColorKeyMask) IsTransparent(color []uint8) bool {
	if len(color) != m.colorCount {
		return false
	}

	for i := 0; i < m.colorCount; i++ {
		min := m.colorRange[i][0]
		max := m.colorRange[i][1]
		if color[i] < min || color[i] > max {
			return false
		}
	}

	return true
}

// ApplyColorKeyMask applies a color key mask to an image.
func ApplyColorKeyMask(img stdimage.Image, mask *ColorKeyMask) (stdimage.Image, error) {
	if mask == nil {
		return img, nil
	}

	bounds := img.Bounds()
	result := stdimage.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := img.At(x, y)

			var colorComponents []uint8
			switch p := pixel.(type) {
			case color.Gray:
				colorComponents = []uint8{p.Y}
			case color.RGBA:
				colorComponents = []uint8{p.R, p.G, p.B}
			case color.YCbCr:
				// Convert to RGB
				r, g, b := color.YCbCrToRGB(p.Y, p.Cb, p.Cr)
				colorComponents = []uint8{r, g, b}
			default:
				// Default to opaque
				result.Set(x, y, pixel)
				continue
			}

			if mask.IsTransparent(colorComponents) {
				// Make transparent
				result.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
			} else {
				result.Set(x, y, pixel)
			}
		}
	}

	return result, nil
}

// DecodeMaskData decodes mask data from a byte array.
func DecodeMaskData(data []byte, width, height, bitsPerComponent int, inverted bool) (image.ImageMask, error) {
	if bitsPerComponent != 1 {
		return nil, errors.Invalid("mask_bpc", nil)
	}

	mask := stdimage.NewGray(stdimage.Rect(0, 0, width, height))

	// Decode 1-bit mask data
	bytesPerRow := (width + 7) / 8

	for y := 0; y < height; y++ {
		rowStart := y * bytesPerRow
		for x := 0; x < width; x++ {
			byteIdx := rowStart + x/8
			bitIdx := 7 - (x % 8)

			if byteIdx < len(data) {
				bitSet := (data[byteIdx] >> bitIdx) & 1
				var value uint8
				if inverted {
					value = bitSet * 255
				} else {
					value = (1 - bitSet) * 255
				}
				mask.SetGray(x, y, color.Gray{Y: value})
			} else {
				// Default to opaque (no masking)
				var value uint8
				if inverted {
					value = 0
				} else {
					value = 255
				}
				mask.SetGray(x, y, color.Gray{Y: value})
			}
		}
	}

	return NewBitmapMaskFromImage(mask, inverted), nil
}
