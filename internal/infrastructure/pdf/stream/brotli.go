// Package stream provides PDF stream handling and filtering.
package stream

import (
	"bytes"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
)

func init() {
	RegisterDecoder(FilterBrotli, &BrotliDecoderFactory{})
}

// DecodeBrotli decodes Brotli-compressed data.
func DecodeBrotli(input []byte) ([]byte, error) {
	if len(input) == 0 {
		return []byte{}, nil
	}

	reader := brotli.NewReader(bytes.NewReader(input))
	output, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("brotli decode failed: %w", err)
	}

	return output, nil
}

// BrotliDecoder implements the Decoder interface for Brotli decompression.
type BrotliDecoder struct{}

// Decode decodes Brotli-compressed data.
func (d *BrotliDecoder) Decode(data []byte) ([]byte, error) {
	return DecodeBrotli(data)
}

// BrotliDecoderFactory creates BrotliDecoder instances.
type BrotliDecoderFactory struct{}

// CreateDecoder creates a new BrotliDecoder.
func (f *BrotliDecoderFactory) CreateDecoder() (Decoder, error) {
	return &BrotliDecoder{}, nil
}
