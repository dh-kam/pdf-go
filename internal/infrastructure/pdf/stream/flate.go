// Package stream provides PDF stream filter implementations.
package stream

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"io"
)

func init() {
	RegisterDecoder(FilterFlate, &FlateFactory{})
}

// FlateFactory creates Flate decoders.
type FlateFactory struct{}

// CreateDecoder creates a new Flate decoder.
func (f *FlateFactory) CreateDecoder() (Decoder, error) {
	return &FlateDecoder{}, nil
}

// FlateDecoder implements Flate decompression.
type FlateDecoder struct{}

// Decode decodes Flate-compressed data.
func (d *FlateDecoder) Decode(data []byte) ([]byte, error) {
	// Handle empty input
	if len(data) == 0 {
		return []byte{}, nil
	}

	// PDF FlateDecode commonly uses zlib-wrapped streams.
	if zr, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		var buf bytes.Buffer
		if _, copyErr := io.Copy(&buf, zr); copyErr == nil {
			if closeErr := zr.Close(); closeErr != nil {
				return nil, closeErr
			}
			return buf.Bytes(), nil
		}
		if closeErr := zr.Close(); closeErr != nil {
			return nil, closeErr
		}
	}

	// Fallback for raw DEFLATE streams.
	r := flate.NewReader(bytes.NewReader(data))

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		if closeErr := r.Close(); closeErr != nil {
			return nil, closeErr
		}
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
