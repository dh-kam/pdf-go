// Package jbig2 provides JBIG2 image decoding support for PDF rendering.
package jbig2

import (
	stdimage "image"

	"github.com/dh-kam/pdf-go/internal/domain/image"
)

// Wrapper adapts the JBIG2 decoder to the domain Decoder interface.
type Wrapper struct {
	decoder *Decoder
}

// NewWrapper creates a new JBIG2 decoder wrapper.
func NewWrapper() *Wrapper {
	return &Wrapper{
		decoder: NewDecoder(),
	}
}

// Decode decodes JBIG2 image data.
func (w *Wrapper) Decode(data []byte) (stdimage.Image, error) {
	return w.decoder.Decode(data)
}

// DecodeConfig returns the JBIG2 image configuration.
func (w *Wrapper) DecodeConfig(data []byte) (stdimage.Config, error) {
	return w.decoder.DecodeConfig(data)
}

// ColorSpace returns the color space for JBIG2 images.
func (w *Wrapper) ColorSpace() image.ColorSpace {
	return w.decoder.ColorSpace()
}

// Ensure Wrapper implements image.Decoder
var _ image.Decoder = (*Wrapper)(nil)
