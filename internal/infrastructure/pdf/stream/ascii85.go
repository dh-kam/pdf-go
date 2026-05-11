package stream

import (
	"bytes"
)

func init() {
	RegisterDecoder(FilterASCII85, &ASCII85Factory{})
}

// ASCII85Factory creates ASCII85 decoders.
type ASCII85Factory struct{}

// CreateDecoder creates a new ASCII85 decoder.
func (f *ASCII85Factory) CreateDecoder() (Decoder, error) {
	return &ASCII85Decoder{}, nil
}

// ASCII85Decoder implements ASCII85 (Base85) decoding.
type ASCII85Decoder struct{}

// Decode decodes ASCII85-encoded data.
func (d *ASCII85Decoder) Decode(data []byte) ([]byte, error) {
	// Strip whitespace and optional Adobe-style delimiters.
	data = removeWhitespace(data)
	if bytes.HasPrefix(data, []byte("<~")) {
		data = data[2:]
	}
	if bytes.HasSuffix(data, []byte("~>")) {
		data = data[:len(data)-2]
	}

	var result bytes.Buffer
	tuple := uint32(0)
	count := 0

	for _, c := range data {
		switch {
		case c == 'z':
			// 'z' is shorthand for a full zero tuple and is only valid between tuples.
			if count != 0 {
				continue
			}
			result.Write([]byte{0, 0, 0, 0})
		case c < '!' || c > 'u':
			// Keep decoder permissive for malformed streams.
			continue
		default:
			value := uint32(c - '!')
			tuple = tuple*85 + value
			count++
			if count == 5 {
				result.WriteByte(byte(tuple >> 24))
				result.WriteByte(byte(tuple >> 16))
				result.WriteByte(byte(tuple >> 8))
				result.WriteByte(byte(tuple))
				tuple = 0
				count = 0
			}
		}
	}

	// Partial tuple: at least 2 chars are required for output.
	if count > 1 {
		for i := count; i < 5; i++ {
			tuple = tuple*85 + 84 // 'u' padding
		}
		for i := 0; i < count-1; i++ {
			shift := uint(24 - 8*i)
			result.WriteByte(byte(tuple >> shift))
		}
	}

	return result.Bytes(), nil
}

// removeWhitespace removes ASCII whitespace from data.
func removeWhitespace(data []byte) []byte {
	result := make([]byte, 0, len(data))
	for _, c := range data {
		switch c {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			// Skip whitespace
		default:
			result = append(result, c)
		}
	}
	return result
}
