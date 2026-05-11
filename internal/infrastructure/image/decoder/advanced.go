// Package decoder provides advanced image decoding interfaces for PDF rendering.
package decoder

import (
	stdimage "image"
)

// AdvancedDecoder extends the basic decoder interface with support for
// advanced image formats like JPEG2000 and JBIG2.
type AdvancedDecoder interface {
	// Decode decodes image data and returns an image.Image.
	Decode(data []byte) (stdimage.Image, error)

	// SupportedFormats returns a list of supported format signatures.
	SupportedFormats() []string

	// CanDecode returns true if the decoder can handle the given data.
	CanDecode(data []byte) bool
}

// FormatSignature represents an image format signature for detection.
type FormatSignature struct {
	Name   string
	Magic  []byte
	Offset int
}

// DetectFormat attempts to detect the image format from the data.
func DetectFormat(data []byte, signatures []FormatSignature) string {
	for _, sig := range signatures {
		if len(data) < sig.Offset+len(sig.Magic) {
			continue
		}
		match := true
		for i, b := range sig.Magic {
			if data[sig.Offset+i] != b {
				match = false
				break
			}
		}
		if match {
			return sig.Name
		}
	}
	return ""
}

// JPEG2000Signatures returns the JP2 file format signatures.
func JPEG2000Signatures() []FormatSignature {
	return []FormatSignature{
		{
			Magic:  []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A},
			Offset: 0,
			Name:   "jp2",
		},
		{
			// JPEG 2000 codestream (without JP2 wrapper)
			Magic:  []byte{0xFF, 0x4F, 0xFF, 0x51},
			Offset: 0,
			Name:   "jpc",
		},
	}
}

// JBIG2Signatures returns the JBIG2 file format signatures.
func JBIG2Signatures() []FormatSignature {
	return []FormatSignature{
		{
			// JBIG2 standalone file
			Magic:  []byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A},
			Offset: 0,
			Name:   "jb2",
		},
	}
}
