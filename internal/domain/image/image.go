// Package image provides image decoding interfaces and types for PDF rendering.
//
//revive:disable:exported
package image

import (
	"image"
)

// ColorSpace represents a PDF color space.
type ColorSpace string

const (
	ColorSpaceDeviceGray ColorSpace = "DeviceGray"
	ColorSpaceDeviceRGB  ColorSpace = "DeviceRGB"
	ColorSpaceDeviceCMYK ColorSpace = "DeviceCMYK"
	ColorSpaceICCBased   ColorSpace = "ICCBased"
	ColorSpaceCalGray    ColorSpace = "CalGray"
	ColorSpaceCalRGB     ColorSpace = "CalRGB"
	ColorSpaceLab        ColorSpace = "Lab"
	ColorSpaceIndexed    ColorSpace = "Indexed"
	ColorSpacePattern    ColorSpace = "Pattern"
	ColorSpaceSeparation ColorSpace = "Separation"
	ColorSpaceDeviceN    ColorSpace = "DeviceN"
)

// ImageFilter represents an image compression filter.
type ImageFilter string

const (
	FilterNone      ImageFilter = ""
	FilterASCIIHex  ImageFilter = "ASCIIHexDecode"
	FilterASCII85   ImageFilter = "ASCII85Decode"
	FilterLZW       ImageFilter = "LZWDecode"
	FilterFlate     ImageFilter = "FlateDecode"
	FilterRunLength ImageFilter = "RunLengthDecode"
	FilterCCITTFax  ImageFilter = "CCITTFaxDecode"
	FilterDCT       ImageFilter = "DCTDecode"   // JPEG
	FilterJPX       ImageFilter = "JPXDecode"   // JPEG2000
	FilterJBIG2     ImageFilter = "JBIG2Decode" // JBIG2
)

// CMYK conversion modes for experimental rendering paths.
const (
	CMYKConversionModeDefault           = ""
	CMYKConversionModeSimpleSubtractive = "simple-subtractive"
	CMYKConversionModeStdlib            = "stdlib"
	CMYKConversionModeHybrid75          = "hybrid-75"
	CMYKConversionModePoly8             = "poly8"
	CMYKConversionModeLUT               = "lut"
)

// Image edge modes for experimental reconstruction contracts.
const (
	ImageEdgeModeDefault                  = ""
	ImageEdgeModeTransparentEdgeOverWhite = "transparent-edge-over-white"
)

// Decoder represents an image decoder that can decode PDF image data.
type Decoder interface {
	// Decode decodes the image data and returns an image.Image.
	Decode(data []byte) (image.Image, error)

	// DecodeConfig returns the image configuration without decoding the full image.
	DecodeConfig(data []byte) (image.Config, error)

	// ColorSpace returns the color space of the decoded image.
	ColorSpace() ColorSpace
}

// DecoderFactory creates decoders for different image formats.
type DecoderFactory interface {
	// CreateDecoder creates a decoder for the given filter.
	CreateDecoder(filter ImageFilter) (Decoder, error)

	// CanDecode returns true if the factory can create a decoder for the filter.
	CanDecode(filter ImageFilter) bool
}

// ImageMask represents an image mask.
type ImageMask interface {
	// Image returns the mask as an image.Image.
	Image() image.Image

	// IsInverted returns true if the mask is inverted.
	IsInverted() bool
}

// ImageData represents PDF image data with metadata.
type ImageData struct {
	Mask               ImageMask
	DecodeParms        map[string]interface{}
	IndexedLookup      []byte
	ColorSpace         ColorSpace
	CMYKConversionMode string
	ImageEdgeMode      string
	ICCProfile         []byte
	ICCComponents      int
	IndexedBase        ColorSpace
	Filter             ImageFilter
	Data               []byte
	Decode             []float64
	Width              int
	Height             int
	BitsPerComponent   int
}

// Image represents a decoded PDF image.
type Image interface {
	// Image returns the Go standard library image.
	Image() image.Image

	// Width returns the image width.
	Width() int

	// Height returns the image height.
	Height() int

	// ColorSpace returns the image color space.
	ColorSpace() ColorSpace

	// BitsPerComponent returns the number of bits per component.
	BitsPerComponent() int

	// HasMask returns true if the image has a mask.
	HasMask() bool

	// Mask returns the image mask, if any.
	Mask() ImageMask
}

// ColorSpaceConverter converts images between color spaces.
type ColorSpaceConverter interface {
	// Convert converts an image from one color space to another.
	Convert(img image.Image, from ColorSpace, to ColorSpace) (image.Image, error)
}

// ImageDecoder is the main image decoder interface.
type ImageDecoder interface {
	// Decode decodes PDF image data into an Image.
	Decode(data *ImageData) (Image, error)

	// RegisterDecoder registers a custom decoder for a filter.
	RegisterDecoder(filter ImageFilter, decoder Decoder)

	// UnregisterDecoder removes a custom decoder.
	UnregisterDecoder(filter ImageFilter)
}
