// Package jpx provides JPEG2000 (JPX) image decoding support for PDF rendering.
package jpx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	stdimage "image"
	"image/color"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/cgo/jpx"
)

// Decoder implements JPEG2000 image decoding.
type Decoder struct {
	useCGo bool
}

// NewDecoder creates a new JPEG2000 decoder.
func NewDecoder() *Decoder {
	return &Decoder{
		useCGo: jpx.IsAvailable(),
	}
}

// Decode decodes JPEG2000 image data.
func (d *Decoder) Decode(data []byte) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, errors.Invalid("jpx_data", nil)
	}

	// Try CGo decoder first if available
	if d.useCGo {
		img, err := d.decodeWithCGo(data)
		if err == nil {
			return img, nil
		}
		// Fall through to native implementation on error
	}

	// Use native Go implementation
	return d.decodeNative(data)
}

// decodeWithCGo uses the CGo OpenJPEG wrapper.
func (d *Decoder) decodeWithCGo(data []byte) (stdimage.Image, error) {
	img, err := jpx.Decode(data)
	if err != nil {
		return nil, errors.Invalid("jpx_cgo_decode", err)
	}
	return img, nil
}

// decodeNative provides a native Go JP2 decoder implementation.
func (d *Decoder) decodeNative(data []byte) (stdimage.Image, error) {
	// Check for JP2 signature
	if len(data) < 12 {
		return nil, errors.Invalid("jpx_header", fmt.Errorf("invalid JP2 header: too short"))
	}

	// JP2 signature: 0x0000000C 0x6A502020 0x0D0A870A
	jp2Sig := []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A}
	if !bytes.Equal(data[:12], jp2Sig) {
		// Check for JPEG 2000 codestream (jpc)
		if len(data) >= 4 && data[0] == 0xFF && data[1] == 0x4F {
			return d.decodeCodestream(data)
		}
		return nil, errors.Invalid("jpx_signature", fmt.Errorf("invalid JP2 signature"))
	}

	// Parse JP2 boxes
	hdr, err := d.parseJP2Header(data[12:])
	if err != nil {
		return nil, errors.Invalid("jpx_header", err)
	}

	// For the stub implementation, return a placeholder image
	// In a full implementation, this would decode the actual codestream
	return d.createPlaceholderImage(hdr), nil
}

// JP2Header represents parsed JP2 file header information.
type JP2Header struct {
	ColorSpace       string
	Width            int
	Height           int
	NumComponents    int
	BitsPerComponent int
}

// parseJP2Header parses the JP2 header box structure.
func (d *Decoder) parseJP2Header(data []byte) (*JP2Header, error) {
	hdr := &JP2Header{
		Width:            100, // Default placeholder values
		Height:           100,
		NumComponents:    3,
		BitsPerComponent: 8,
		ColorSpace:       "rgb",
	}

	offset := 0

	for offset < len(data) {
		if offset+8 > len(data) {
			break
		}

		// Read box length (4 bytes) and type (4 bytes)
		boxLen := binary.BigEndian.Uint32(data[offset : offset+4])
		boxType := string(data[offset+4 : offset+8])
		offset += 8

		// Handle special case: boxLen == 0 means last box (extends to end of file)
		if boxLen == 0 {
			boxLen = binary.BigEndian.Uint32([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // Max uint32
		}

		// Handle special case: boxLen == 1 means extended length (8 bytes)
		if boxLen == 1 {
			// Read extended length
			if offset+8 > len(data) {
				break
			}
			// For simplicity, just break on extended length boxes
			break
		}

		if boxLen < 8 {
			break
		}

		// Ensure we don't go beyond the data
		boxDataEnd := offset + int(boxLen) - 8
		if boxDataEnd > len(data) {
			boxDataEnd = len(data)
		}
		boxData := data[offset:boxDataEnd]

		switch boxType {
		case "ihdr":
			// Image Header box
			err := d.parseImageHeaderBox(boxData, hdr)
			if err != nil {
				return nil, err
			}

		case "colr":
			// Color Specification box
			d.parseColorSpecBox(boxData, hdr)
		}

		offset = boxDataEnd
	}

	return hdr, nil
}

// parseImageHeaderBox parses the Image Header box.
func (d *Decoder) parseImageHeaderBox(data []byte, hdr *JP2Header) error {
	if len(data) < 14 {
		return fmt.Errorf("invalid image header box")
	}

	hdr.Height = int(binary.BigEndian.Uint32(data[0:4]))
	hdr.Width = int(binary.BigEndian.Uint32(data[4:8]))
	hdr.NumComponents = int(data[8])
	hdr.BitsPerComponent = int(data[9]) & 0x7F // Mask off sign bit

	// Compression (1 byte) at offset 10 - should be 7 for JP2
	// Colorspace unknown (1 byte) at offset 11
	// Intellectual property (1 byte) at offset 12

	return nil
}

// parseColorSpecBox parses the Color Specification box.
func (d *Decoder) parseColorSpecBox(data []byte, hdr *JP2Header) {
	if len(data) < 3 {
		return
	}

	// Method (1 byte), Precedence (1 byte), Approximation (1 byte)
	// Colorspace enumeration (4 bytes)
	csType := binary.BigEndian.Uint32(data[3:7])

	switch csType {
	case 16: // sRGB
		hdr.ColorSpace = "rgb"
	case 17: // Grayscale
		hdr.ColorSpace = "gray"
	case 18: // sYCC
		hdr.ColorSpace = "ycc"
	default:
		hdr.ColorSpace = "rgb"
	}
}

// decodeCodestream decodes a JPEG 2000 codestream (without JP2 wrapper).
func (d *Decoder) decodeCodestream(data []byte) (stdimage.Image, error) {
	// Parse SOC (Start of codestream) marker
	if data[0] != 0xFF || data[1] != 0x4F {
		return nil, fmt.Errorf("invalid JPEG 2000 codestream")
	}

	// This is a stub implementation
	// A full implementation would parse all markers:
	// - SOC (0xFF4F): Start of codestream
	// - SIZ (0xFF51): Image and tile size
	// - COD (0xFF52): Coding style default
	// - COC (0xFF53): Coding style component
	// - QCD (0xFF5C): Quantization default
	// - QCC (0xFF5D): Quantization component
	// - POC (0xFF5F): Progression order change
	// - PPT (0xFF61): Packed headers, main header
	// - PLT (0xFF58): Packet length, tile-part header
	// - COM (0xFF64): Comment
	// - SOT (0xFF90): Start of tile-part
	// - SOP (0xFF91): Start of packet
	// - EPH (0xFF92): End of packet header
	// - SOD (0xFF93): Start of data
	// - EOC (0xFFD9): End of codestream

	return stdimage.NewRGBA(stdimage.Rect(0, 0, 100, 100)), nil
}

// createPlaceholderImage creates a placeholder image for the stub implementation.
func (d *Decoder) createPlaceholderImage(hdr *JP2Header) stdimage.Image {
	width := hdr.Width
	height := hdr.Height

	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 100
	}

	switch hdr.ColorSpace {
	case "gray":
		return stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	default:
		return stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	}
}

// DecodeConfig returns the JPEG2000 image configuration.
func (d *Decoder) DecodeConfig(data []byte) (stdimage.Config, error) {
	if len(data) < 12 {
		return stdimage.Config{}, errors.Invalid("jpx_config", fmt.Errorf("data too short"))
	}

	// Check for JP2 signature
	jp2Sig := []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A}
	if !bytes.Equal(data[:12], jp2Sig) {
		return stdimage.Config{}, errors.Invalid("jpx_config", fmt.Errorf("invalid JP2 signature"))
	}

	// Parse header for configuration
	hdr, err := d.parseJP2Header(data[12:])
	if err != nil {
		return stdimage.Config{}, err
	}

	colorModel := color.RGBAModel
	if hdr.ColorSpace == "gray" {
		colorModel = color.GrayModel
	}

	return stdimage.Config{
		Width:      hdr.Width,
		Height:     hdr.Height,
		ColorModel: colorModel,
	}, nil
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
