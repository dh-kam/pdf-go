// Package jpx provides CGo wrapper around OpenJPEG library for JPEG2000 decoding.
//go:build !nojpx

package jpx

/*
#cgo pkg-config: libopenjp2

#include <openjpeg.h>
#include <stdlib.h>

// Helper to decode JP2 data from a temporary in-memory path.
static opj_stream_t* create_file_stream(const char* path) {
    return opj_stream_create_default_file_stream(path, OPJ_TRUE);
}
*/
import "C"
import (
	"fmt"
	stdimage "image"
	"image/color"
	"os"
	"unsafe"
)

// IsAvailable returns true if OpenJPEG library is available.
func IsAvailable() bool {
	return true
}

// Decode decodes JPEG2000 data using OpenJPEG.
func Decode(data []byte) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	path, cleanup, err := writeJPXTempFile(data)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Create decoder
	cDecoder := C.opj_create_decompress(C.OPJ_CODEC_JP2)
	if cDecoder == nil {
		return nil, fmt.Errorf("failed to create decoder")
	}
	defer C.opj_destroy_codec(cDecoder)

	// Setup decoder parameters
	var params C.opj_dparameters_t
	C.opj_set_default_decoder_parameters(&params)
	if C.opj_setup_decoder(cDecoder, &params) != 1 {
		return nil, fmt.Errorf("failed to setup decoder")
	}

	// Create memory stream
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	cStream := C.create_file_stream(cPath)
	if cStream == nil {
		return nil, fmt.Errorf("failed to create stream")
	}
	defer C.opj_stream_destroy(cStream)

	// Read header
	var cImage *C.opj_image_t
	if C.opj_read_header(cStream, cDecoder, &cImage) != 1 {
		return nil, fmt.Errorf("failed to read header")
	}
	defer C.opj_image_destroy(cImage)

	// Decode image
	if C.opj_decode(cDecoder, cStream, cImage) != 1 {
		return nil, fmt.Errorf("failed to decode image")
	}

	// Convert to Go image
	return convertImage(cImage)
}

// convertImage converts OpenJPEG image to Go image.Image.
func convertImage(cImage *C.opj_image_t) (stdimage.Image, error) {
	if cImage == nil {
		return nil, fmt.Errorf("nil image")
	}

	width := int(cImage.x1 - cImage.x0)
	height := int(cImage.y1 - cImage.y0)
	numComps := int(cImage.numcomps)

	if width <= 0 || height <= 0 || numComps <= 0 {
		return nil, fmt.Errorf("invalid image dimensions")
	}

	// Get component data
	comps := (*[1 << 28]C.opj_image_comp_t)(unsafe.Pointer(cImage.comps))[:numComps:numComps]

	// Determine color space
	switch {
	case numComps == 1:
		return convertGrayImage(comps[0], width, height)
	case numComps == 3:
		return convertRGBImage(comps, width, height)
	case numComps == 4:
		return convertRGBAImage(comps, width, height)
	default:
		return nil, fmt.Errorf("unsupported number of components: %d", numComps)
	}
}

// convertGrayImage converts a grayscale component to Go image.Gray.
func convertGrayImage(comp C.opj_image_comp_t, width, height int) (stdimage.Image, error) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, width, height))

	data := (*[1 << 28]C.int)(unsafe.Pointer(comp.data))[: width*height : (width * height)]

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			val := int(data[idx])

			// Scale to 8-bit if needed
			prec := int(comp.prec)
			if prec > 8 {
				val = (val >> (prec - 8)) & 0xFF
			}

			img.SetGray(x, y, color.Gray{Y: uint8(val)})
		}
	}

	return img, nil
}

// convertRGBImage converts RGB components to Go image.RGBA.
func convertRGBImage(comps []C.opj_image_comp_t, width, height int) (stdimage.Image, error) {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))

	for i := 0; i < 3; i++ {
		data := (*[1 << 28]C.int)(unsafe.Pointer(comps[i].data))[: width*height : (width * height)]
		prec := int(comps[i].prec)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := y*width + x
				val := int(data[idx])

				// Scale to 8-bit if needed
				if prec > 8 {
					val = (val >> (prec - 8)) & 0xFF
				}

				c := img.RGBAAt(x, y)
				switch i {
				case 0:
					c.R = uint8(val)
				case 1:
					c.G = uint8(val)
				case 2:
					c.B = uint8(val)
				}
				img.SetRGBA(x, y, c)
			}
		}
	}

	return img, nil
}

// convertRGBAImage converts RGBA components to Go image.RGBA.
func convertRGBAImage(comps []C.opj_image_comp_t, width, height int) (stdimage.Image, error) {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))

	for i := 0; i < 4; i++ {
		data := (*[1 << 28]C.int)(unsafe.Pointer(comps[i].data))[: width*height : (width * height)]
		prec := int(comps[i].prec)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := y*width + x
				val := int(data[idx])

				// Scale to 8-bit if needed
				if prec > 8 {
					val = (val >> (prec - 8)) & 0xFF
				}

				c := img.RGBAAt(x, y)
				switch i {
				case 0:
					c.R = uint8(val)
				case 1:
					c.G = uint8(val)
				case 2:
					c.B = uint8(val)
				case 3:
					c.A = uint8(val)
				}
				img.SetRGBA(x, y, c)
			}
		}
	}

	return img, nil
}

// DecodeConfig returns the image configuration without decoding the full image.
func DecodeConfig(data []byte) (stdimage.Config, error) {
	if len(data) == 0 {
		return stdimage.Config{}, fmt.Errorf("empty data")
	}

	path, cleanup, err := writeJPXTempFile(data)
	if err != nil {
		return stdimage.Config{}, err
	}
	defer cleanup()

	// Create decoder
	cDecoder := C.opj_create_decompress(C.OPJ_CODEC_JP2)
	if cDecoder == nil {
		return stdimage.Config{}, fmt.Errorf("failed to create decoder")
	}
	defer C.opj_destroy_codec(cDecoder)

	// Setup decoder parameters
	var params C.opj_dparameters_t
	C.opj_set_default_decoder_parameters(&params)
	if C.opj_setup_decoder(cDecoder, &params) != 1 {
		return stdimage.Config{}, fmt.Errorf("failed to setup decoder")
	}

	// Create stream from temp file
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	cStream := C.create_file_stream(cPath)
	if cStream == nil {
		return stdimage.Config{}, fmt.Errorf("failed to create stream")
	}
	defer C.opj_stream_destroy(cStream)

	// Read header only
	var cImage *C.opj_image_t
	if C.opj_read_header(cStream, cDecoder, &cImage) != 1 {
		return stdimage.Config{}, fmt.Errorf("failed to read header")
	}
	defer C.opj_image_destroy(cImage)

	width := int(cImage.x1 - cImage.x0)
	height := int(cImage.y1 - cImage.y0)
	numComps := int(cImage.numcomps)

	colorModel := color.RGBAModel
	if numComps == 1 {
		colorModel = color.GrayModel
	}

	return stdimage.Config{
		Width:      width,
		Height:     height,
		ColorModel: colorModel,
	}, nil
}

func writeJPXTempFile(data []byte) (string, func(), error) {
	f, err := os.CreateTemp("", "go-pdf-jpx-*.jpx")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}

	path := f.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		cleanup()
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	return path, cleanup, nil
}
