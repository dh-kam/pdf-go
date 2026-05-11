// Package jpx provides stub implementation when OpenJPEG is not available.
//go:build nojpx

package jpx

import (
	"fmt"
	stdimage "image"
)

// IsAvailable returns false when OpenJPEG is not available.
func IsAvailable() bool {
	return false
}

// Decode returns an error indicating OpenJPEG is not available.
func Decode(data []byte) (stdimage.Image, error) {
	return nil, fmt.Errorf("JPEG2000 support is not available (built with 'nojpx' tag)")
}

// DecodeConfig returns an error indicating OpenJPEG is not available.
func DecodeConfig(data []byte) (stdimage.Config, error) {
	return stdimage.Config{}, fmt.Errorf("JPEG2000 support is not available (built with 'nojpx' tag)")
}
