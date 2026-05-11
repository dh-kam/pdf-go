// Package jbig2 provides stub implementation when jbig2dec is not available.
//go:build nojbig2

package jbig2

import (
	"fmt"
	stdimage "image"
)

// IsAvailable returns false when jbig2dec is not available.
func IsAvailable() bool {
	return false
}

// Decode returns an error indicating jbig2dec is not available.
func Decode(data []byte) (stdimage.Image, error) {
	return nil, fmt.Errorf("JBIG2 support is not available (built with 'nojbig2' tag)")
}

// DecodeConfig returns an error indicating jbig2dec is not available.
func DecodeConfig(data []byte) (stdimage.Config, error) {
	return stdimage.Config{}, fmt.Errorf("JBIG2 support is not available (built with 'nojbig2' tag)")
}

// DecodeOptions represents JBIG2 decoding options.
type DecodeOptions struct {
	Globals []byte // Global segment data (for embedded JBIG2)
}

// DecodeWithOptions returns an error indicating jbig2dec is not available.
func DecodeWithOptions(data []byte, opts DecodeOptions) (stdimage.Image, error) {
	return nil, fmt.Errorf("JBIG2 support is not available (built with 'nojbig2' tag)")
}
