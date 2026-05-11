package stream

import (
	"bytes"
	"compress/lzw"
	"fmt"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func init() {
	RegisterDecoder(FilterLZW, &LZWFactory{})
}

// LZWFactory creates LZW decoders.
type LZWFactory struct{}

// CreateDecoder creates a new LZW decoder.
func (f *LZWFactory) CreateDecoder() (Decoder, error) {
	return &LZWDecoder{earlyChange: 1}, nil
}

// LZWDecoder implements LZW decompression for PDF streams.
type LZWDecoder struct {
	earlyChange int // 1 is PDF default; stdlib behavior aligns with this.
}

// SetDecodeParams sets LZW-specific decode parameters.
func (d *LZWDecoder) SetDecodeParams(params *entity.Dict) {
	if params == nil {
		return
	}
	if value, ok := objectToInt(params.Get(entity.Name("EarlyChange"))); ok {
		d.earlyChange = value
	}
}

// Decode decodes LZW-compressed data.
func (d *LZWDecoder) Decode(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	// Go's LZW reader uses MSB bit order and 8-bit literal width, which matches
	// the common PDF LZW stream configuration.
	reader := lzw.NewReader(bytes.NewReader(data), lzw.MSB, 8)
	defer func() { _ = reader.Close() }() //nolint:errcheck // Close errors are non-actionable after read completion.

	decoded, err := io.ReadAll(reader)
	if err != nil {
		// Some malformed PDFs still decode with LSB ordering.
		fallback := lzw.NewReader(bytes.NewReader(data), lzw.LSB, 8)
		defer func() { _ = fallback.Close() }() //nolint:errcheck // Close errors are non-actionable after read completion.
		if fallbackDecoded, fallbackErr := io.ReadAll(fallback); fallbackErr == nil {
			return fallbackDecoded, nil
		}
		return nil, fmt.Errorf("lzw decode failed: %w", err)
	}

	return decoded, nil
}

// lzwReader is kept as a thin adapter for existing tests.
type lzwReader struct {
	reader io.ReadCloser
}

const (
	maxCode    = 4095
	tableSize  = 4096
	resetCode  = 256
	eoiCodeVal = 257
	firstCode  = 258
)

const invalidEntry = 0xFFFF

func newLZWReader(r io.Reader, _ int) io.ReadCloser {
	return &lzwReader{
		reader: lzw.NewReader(r, lzw.MSB, 8),
	}
}

// Read is an exported API.
func (l *lzwReader) Read(p []byte) (int, error) {
	if l == nil || l.reader == nil {
		return 0, io.EOF
	}
	return l.reader.Read(p)
}

// Close is an exported API.
func (l *lzwReader) Close() error {
	if l == nil || l.reader == nil {
		return nil
	}
	return l.reader.Close()
}
