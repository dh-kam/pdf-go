// Package jpx provides JPEG2000 (JPX) image decoding support for PDF rendering.
package jpx

import (
	"bytes"
	stdimage "image"

	jpeg2000 "github.com/ajroetker/go-jpeg2000"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/image"
)

// Decoder implements JPEG2000 image decoding.
type Decoder struct{}

// NewDecoder creates a new JPEG2000 decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// NewNativeDecoder creates a JPEG2000 decoder that always uses the pure Go path.
func NewNativeDecoder() *Decoder {
	return &Decoder{}
}

// Decode decodes JPEG2000 image data.
func (d *Decoder) Decode(data []byte) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, errors.Invalid("jpx_data", nil)
	}

	return d.decodeNative(data)
}

// decodeNative provides a native Go JP2 decoder implementation.
func (d *Decoder) decodeNative(data []byte) (stdimage.Image, error) {
	img, err := jpeg2000.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Invalid("jpx_native_decode", err)
	}
	return img, nil
}

// DecodeConfig returns the JPEG2000 image configuration.
func (d *Decoder) DecodeConfig(data []byte) (stdimage.Config, error) {
	cfg, err := jpeg2000.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return stdimage.Config{}, errors.Invalid("jpx_config", err)
	}
	return cfg, nil
}

// ColorSpace returns the color space for JPEG2000 images.
func (d *Decoder) ColorSpace() image.ColorSpace {
	return image.ColorSpaceDeviceRGB
}

// CanDecode checks if the data appears to be a JPEG2000 image.
func (d *Decoder) CanDecode(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// Check JP2 signature
	if len(data) >= 12 {
		jp2Sig := []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A}
		if bytes.Equal(data[:12], jp2Sig) {
			return true
		}
	}

	// Check for JPEG 2000 codestream
	if data[0] == 0xFF && data[1] == 0x4F {
		return true
	}

	return false
}

// SupportedFormats returns the supported JPEG2000 format identifiers.
func (d *Decoder) SupportedFormats() []string {
	return []string{"jp2", "jpc", "jpx"}
}
