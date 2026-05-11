// Command advanced_decoding demonstrates advanced image decoding usage.
package main

import (
	"fmt"
	"log"

	"github.com/dh-kam/pdf-go/internal/domain/image"
	imageinfra "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image/jbig2"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image/jpx"
)

func main() {
	ExampleDetectFormat()
	ExampleBuildTags()
	ExampleDecoderIntegration()
}

// ExampleDecodeJPEG2000 demonstrates JPEG2000 image decoding.
func ExampleDecodeJPEG2000() {
	// Create a JPEG2000 decoder
	decoder := jpx.NewDecoder()

	// Check if format is supported
	jp2Data := []byte{
		0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20,
		0x0D, 0x0A, 0x87, 0x0A, // JP2 signature
		// ... rest of JP2 data
	}

	if decoder.CanDecode(jp2Data) {
		img, err := decoder.Decode(jp2Data)
		if err != nil {
			log.Printf("Decoding error: %v", err)
			return
		}
		fmt.Printf("Decoded JPEG2000 image: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

// ExampleDecodeJBIG2 demonstrates JBIG2 image decoding.
func ExampleDecodeJBIG2() {
	// Create a JBIG2 decoder
	decoder := jbig2.NewDecoder()

	// Check if format is supported
	jbig2Data := []byte{
		0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A, // JBIG2 signature
		// ... rest of JBIG2 data
	}

	if decoder.CanDecode(jbig2Data) {
		img, err := decoder.Decode(jbig2Data)
		if err != nil {
			log.Printf("Decoding error: %v", err)
			return
		}
		fmt.Printf("Decoded JBIG2 image: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

// ExampleDecodePDFImage demonstrates using the main decoder with advanced formats.
func ExampleDecodePDFImage() {
	// Create the main decoder
	decoder := imageinfra.NewDecoder()

	// JPEG2000 image data from PDF
	jpxImageData := &image.ImageData{
		Width:            100,
		Height:           100,
		ColorSpace:       image.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           image.FilterJPX,
		Data: []byte{
			0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20,
			0x0D, 0x0A, 0x87, 0x0A, // JP2 signature
		},
	}

	// Decode the image
	img, err := decoder.Decode(jpxImageData)
	if err != nil {
		log.Printf("Decoding error: %v", err)
		return
	}

	fmt.Printf("Decoded PDF image: %dx%d, color space: %s\n",
		img.Width(), img.Height(), img.ColorSpace())
}

// ExampleDetectFormat demonstrates automatic format detection.
func ExampleDetectFormat() {
	data := []byte{
		0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20,
		0x0D, 0x0A, 0x87, 0x0A, // JP2 signature
	}

	// Detect format
	jpxDecoder := jpx.NewDecoder()
	jbig2Decoder := jbig2.NewDecoder()

	switch {
	case jpxDecoder.CanDecode(data):
		fmt.Println("Format: JPEG2000")
	case jbig2Decoder.CanDecode(data):
		fmt.Println("Format: JBIG2")
	default:
		fmt.Println("Format: Unknown")
	}
}

// ExampleBuildTags demonstrates building with different CGo configurations.
func ExampleBuildTags() {
	// To build without CGo (stub implementations):
	// go build -tags='nojpx,nojbig2'
	//
	// To build with CGo (requires OpenJPEG and jbig2dec):
	// go build
	//
	// The decoder automatically uses the best available implementation:
	// - CGo wrapper when library is available
	// - Native Go stub when library is not available

	_ = "see comments above"
}

// ExampleDecoderIntegration shows how the decoder integrates with the PDF reader.
func ExampleDecoderIntegration() {
	// The main decoder is created with all supported formats registered
	decoder := imageinfra.NewDecoder()

	// It can handle:
	// - DCTDecode (JPEG) via standard Go library
	// - JPXDecode (JPEG2000) via OpenJPEG or stub
	// - JBIG2Decode (JBIG2) via jbig2dec or stub
	// - FlateDecode (zlib compressed)
	// - LZWDecode
	// - ASCIIHexDecode
	// - ASCII85Decode

	_ = decoder
	fmt.Println("Decoder supports: JPEG, JPEG2000, JBIG2, and more")
}
