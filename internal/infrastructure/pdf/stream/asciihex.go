package stream

import (
	"bytes"
)

func init() {
	RegisterDecoder(FilterASCIIHex, &ASCIIHexFactory{})
}

// ASCIIHexFactory creates ASCIIHex decoders.
type ASCIIHexFactory struct{}

// CreateDecoder creates a new ASCIIHex decoder.
func (f *ASCIIHexFactory) CreateDecoder() (Decoder, error) {
	return &ASCIIHexDecoder{}, nil
}

// ASCIIHexDecoder implements ASCIIHex decoding.
type ASCIIHexDecoder struct{}

// Decode decodes ASCIIHex-encoded data.
func (d *ASCIIHexDecoder) Decode(data []byte) ([]byte, error) {
	// Remove whitespace and EOD marker
	data = bytes.TrimSpace(data)

	// Remove all whitespace from within the data
	cleaned := make([]byte, 0, len(data))
	for _, c := range data {
		switch c {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			// Skip whitespace
		case '>':
			// EOD marker - skip marker itself.
			continue
		default:
			cleaned = append(cleaned, c)
		}
	}
	data = cleaned

	// Pad to even length
	if len(data)%2 != 0 {
		data = append(data, '0')
	}

	result := make([]byte, 0, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		high := decodeHexByte(data[i])
		low := decodeHexByte(data[i+1])
		if high == 255 || low == 255 {
			// Invalid hex digit, skip
			continue
		}
		result = append(result, high<<4|low)
	}

	return result, nil
}

// decodeHexByte decodes a single hex byte.
// Returns 255 for invalid digits.
func decodeHexByte(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 255 // Invalid
	}
}
