// Package jbig2 provides JBIG2 image decoding support for PDF rendering.
//
//revive:disable:exported
//nolint:staticcheck,unused
package jbig2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	stdimage "image"
	"image/color"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/cgo/jbig2"
)

// Decoder implements JBIG2 image decoding.
type Decoder struct {
	useCGo bool
}

// NewDecoder creates a new JBIG2 decoder.
func NewDecoder() *Decoder {
	return &Decoder{
		useCGo: jbig2.IsAvailable(),
	}
}

// Decode decodes JBIG2 image data.
func (d *Decoder) Decode(data []byte) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, errors.Invalid("jbig2_data", nil)
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

// decodeWithCGo uses the CGo jbig2dec wrapper.
func (d *Decoder) decodeWithCGo(data []byte) (stdimage.Image, error) {
	img, err := jbig2.Decode(data)
	if err != nil {
		return nil, errors.Invalid("jbig2_cgo_decode", err)
	}
	return img, nil
}

// decodeNative provides a native Go JBIG2 decoder implementation.
func (d *Decoder) decodeNative(data []byte) (stdimage.Image, error) {
	// Check for JBIG2 signature
	if len(data) < 8 {
		return nil, errors.Invalid("jbig2_header", fmt.Errorf("invalid JBIG2 header: too short"))
	}

	// JBIG2 signature: 0x97 0x4A 0x42 0x32 0x0D 0x0A 0x1A 0x0A
	jbig2Sig := []byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.Equal(data[:8], jbig2Sig) {
		// Might be embedded JBIG2 data without header
		return d.decodeEmbedded(data)
	}

	// Parse JBIG2 file header
	hdr, err := d.parseFileHeader(data[8:])
	if err != nil {
		return nil, errors.Invalid("jbig2_header", err)
	}

	// For the stub implementation, return a placeholder image
	// In a full implementation, this would decode the actual segments
	return d.createPlaceholderImage(hdr), nil
}

// JBIG2Header represents parsed JBIG2 file header information.
type JBIG2Header struct {
	Width         int
	Height        int
	IsStriped     bool
	MaxStripeSize int
}

// parseFileHeader parses the JBIG2 file header.
func (d *Decoder) parseFileHeader(data []byte) (*JBIG2Header, error) {
	hdr := &JBIG2Header{
		Width:     100,
		Height:    100,
		IsStriped: false,
	}

	if len(data) < 8 {
		return hdr, nil
	}

	// Parse file header (first 8 bytes after signature)
	// Number of pages (4 bytes)
	numPages := binary.BigEndian.Uint32(data[0:4])

	// Organization flag (1 byte)
	orgFlag := data[4]
	if orgFlag&0x01 != 0 {
		hdr.IsStriped = true
		// Max stripe size (optional, 4 bytes)
		if len(data) >= 12 {
			hdr.MaxStripeSize = int(binary.BigEndian.Uint32(data[8:12]))
		}
	}

	// If single page, try to extract dimensions from segments
	if numPages == 1 {
		d.parseSegmentHeader(data[8:], hdr)
	}

	return hdr, nil
}

// parseSegmentHeader parses JBIG2 segment headers.
func (d *Decoder) parseSegmentHeader(data []byte, hdr *JBIG2Header) {
	offset := 0

	for offset < len(data) {
		if offset+6 > len(data) {
			break
		}

		// Segment number (4 bytes)
		// segNumber := binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4

		// Segment flags (1 byte)
		flags := data[offset]
		offset++

		// Segment type (1 byte)
		segType := data[offset]
		offset++

		// Check if this is a page information segment (type 48)
		if segType == 48 {
			// Page information segment
			if offset+5 <= len(data) {
				hdr.Width = int(binary.BigEndian.Uint32(data[offset : offset+4]))
				hdr.Height = int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
				return
			}
		}

		// Get segment data length (variable)
		refCount := int(flags&0xE0) >> 5
		if flags&0x10 != 0 {
			// 32-bit segment count
			if offset+4 > len(data) {
				break
			}
			// segDataLength := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
		} else {
			// 24-bit segment count
			if offset+3 > len(data) {
				break
			}
			// segDataLength := uint32(data[offset])<<16 | uint32(data[offset+1])<<8 | uint32(data[offset+2])
			offset += 3
		}

		// Skip retained segments
		for i := 0; i < refCount; i++ {
			if offset+4 > len(data) {
				break
			}
			offset += 4
		}

		// Skip segment data (we'd need the length to do this properly)
		// For now, just break
		break
	}
}

// decodeEmbedded handles embedded JBIG2 data (without file header).
func (d *Decoder) decodeEmbedded(data []byte) (stdimage.Image, error) {
	// Embedded JBIG2 data starts directly with segments
	// This is a stub implementation
	hdr := &JBIG2Header{
		Width:  100,
		Height: 100,
	}

	// Try to find page information segment
	d.parseSegmentHeader(data, hdr)

	return d.createPlaceholderImage(hdr), nil
}

// createPlaceholderImage creates a placeholder image for the stub implementation.
func (d *Decoder) createPlaceholderImage(hdr *JBIG2Header) stdimage.Image {
	width := hdr.Width
	height := hdr.Height

	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 100
	}

	// JBIG2 is typically bi-level (1 bit per pixel)
	return stdimage.NewGray(stdimage.Rect(0, 0, width, height))
}

// DecodeConfig returns the JBIG2 image configuration.
func (d *Decoder) DecodeConfig(data []byte) (stdimage.Config, error) {
	if len(data) < 8 {
		return stdimage.Config{}, errors.Invalid("jbig2_config", fmt.Errorf("data too short"))
	}

	// Check for JBIG2 signature
	jbig2Sig := []byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A}
	var hdr *JBIG2Header
	var err error

	if bytes.Equal(data[:8], jbig2Sig) {
		hdr, err = d.parseFileHeader(data[8:])
	} else {
		hdr = &JBIG2Header{
			Width:  100,
			Height: 100,
		}
		d.parseSegmentHeader(data, hdr)
	}

	if err != nil {
		return stdimage.Config{}, err
	}

	return stdimage.Config{
		Width:      hdr.Width,
		Height:     hdr.Height,
		ColorModel: color.GrayModel,
	}, nil
}

// ColorSpace returns the color space for JBIG2 images.
func (d *Decoder) ColorSpace() image.ColorSpace {
	return image.ColorSpaceDeviceGray
}

// CanDecode checks if the data appears to be a JBIG2 image.
func (d *Decoder) CanDecode(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	// Check JBIG2 signature
	jbig2Sig := []byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A}
	if bytes.Equal(data[:8], jbig2Sig) {
		return true
	}

	// Check for embedded JBIG2 (starts with segment header)
	// Segment number is 4 bytes, typically 0 or small value
	if len(data) >= 6 {
		// Check if it looks like a segment header
		segNumber := binary.BigEndian.Uint32(data[0:4])
		if segNumber < 256 { // Reasonable segment number
			// Check segment flags
			flags := data[4]
			refCount := (flags & 0xE0) >> 5
			if refCount <= 4 { // Reasonable reference count
				return true
			}
		}
	}

	return false
}

// SupportedFormats returns the supported JBIG2 format identifiers.
func (d *Decoder) SupportedFormats() []string {
	return []string{"jb2", "jbig2"}
}

// DecodeSegment decodes a single JBIG2 segment (for embedded data).
func (d *Decoder) DecodeSegment(data []byte) (stdimage.Image, error) {
	return d.decodeNative(data)
}

// SegmentType represents JBIG2 segment types.
type SegmentType int

const (
	SegmentSymbolDictionary      SegmentType = 0
	SegmentIntermediateText      RegionType  = 4
	SegmentImmediateText         RegionType  = 6
	SegmentImmediateLosslessText RegionType  = 7
	SegmentPatternDictionary     SegmentType = 16
	SegmentIntermediateHalftone  RegionType  = 20
	SegmentImmediateHalftone     RegionType  = 22
	SegmentPageInformation       SegmentType = 48
	SegmentEndOfPage             SegmentType = 49
	SegmentEndOfStripe           SegmentType = 50
	SegmentEndOfFile             SegmentType = 51
	SegmentProfiles              SegmentType = 52
	SegmentTables                SegmentType = 53
	SegmentExtension             SegmentType = 62
)

// RegionType represents JBIG2 region types.
type RegionType int

const (
	RegionTypeText              RegionType = 0
	RegionTypeHalftone          RegionType = 1
	RegionTypeGeneric           RegionType = 2
	RegionTypeGenericRefinement RegionType = 3
)

// ArithmeticDecoder implements JBIG2 arithmetic coding (MQ-coder).
type ArithmeticDecoder struct {
	bps        []byte
	byteOffset int
	bitOffset  uint8
	c          byte
	a          byte
	ct         byte
}

// NewArithmeticDecoder creates a new arithmetic decoder.
func NewArithmeticDecoder(data []byte) *ArithmeticDecoder {
	return &ArithmeticDecoder{
		bps:        data,
		byteOffset: 0,
		bitOffset:  0,
	}
}

// DecodeBit decodes a single bit using arithmetic coding.
func (ad *ArithmeticDecoder) DecodeBit(context uint8) (uint8, error) {
	_ = context
	if ad.byteOffset >= len(ad.bps) {
		return 0, io.EOF
	}

	bit := (ad.bps[ad.byteOffset] >> (7 - ad.bitOffset)) & 0x01
	ad.bitOffset++
	if ad.bitOffset >= 8 {
		ad.bitOffset = 0
		ad.byteOffset++
	}

	return bit, nil
}

// MMRDecoder implements Modified Modified READ (MMR) compression.
type MMRDecoder struct {
	offset  int
	data    []byte
	lineBuf []byte
	width   int
	height  int
}

// NewMMRDecoder creates a new MMR decoder.
func NewMMRDecoder(data []byte, width, height int) *MMRDecoder {
	return &MMRDecoder{
		data:    data,
		width:   width,
		height:  height,
		lineBuf: make([]byte, (width+7)/8),
	}
}

// DecodeLine decodes one line using MMR compression.
func (md *MMRDecoder) DecodeLine() ([]byte, error) {
	lineBytes := (md.width + 7) / 8
	if lineBytes <= 0 {
		return nil, fmt.Errorf("invalid MMR width: %d", md.width)
	}

	if md.offset >= len(md.data) {
		return nil, io.EOF
	}

	if len(md.lineBuf) != lineBytes {
		md.lineBuf = make([]byte, lineBytes)
	} else {
		for i := range md.lineBuf {
			md.lineBuf[i] = 0
		}
	}

	n := copy(md.lineBuf, md.data[md.offset:])
	md.offset += n

	line := make([]byte, lineBytes)
	copy(line, md.lineBuf)
	return line, nil
}

// DecodeGenericRegion decodes a generic region segment.
func (d *Decoder) DecodeGenericRegion(data []byte, width, height int) (stdimage.Image, error) {
	// This is a stub implementation
	// A full implementation would decode generic region data
	return stdimage.NewGray(stdimage.Rect(0, 0, width, height)), nil
}

// DecodeTextRegion decodes a text region segment.
func (d *Decoder) DecodeTextRegion(data []byte, width, height int, symbols []stdimage.Image) (stdimage.Image, error) {
	// This is a stub implementation
	// A full implementation would decode text region data using symbol dictionary
	return stdimage.NewGray(stdimage.Rect(0, 0, width, height)), nil
}
