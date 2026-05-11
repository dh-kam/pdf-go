// Package jbig2 provides CGo wrapper around jbig2dec library for JBIG2 decoding.
//go:build !nojbig2

package jbig2

/*
#cgo CFLAGS: -I/usr/include
#cgo LDFLAGS: -ljbig2dec

#include <stdint.h>
#include <stdlib.h>
#include <jbig2.h>

static Jbig2Ctx* go_jbig2_ctx_new(Jbig2GlobalCtx* global_ctx, int embedded) {
	Jbig2Options options = embedded ? JBIG2_OPTIONS_EMBEDDED : 0;
	return jbig2_ctx_new(NULL, options, global_ctx, NULL, NULL);
}
*/
import "C"

import (
	"fmt"
	stdimage "image"
	"image/color"
	"unsafe"
)

// IsAvailable returns true if jbig2dec library is available.
func IsAvailable() bool {
	return true
}

// Decode decodes JBIG2 data using jbig2dec.
func Decode(data []byte) (stdimage.Image, error) {
	return DecodeWithOptions(data, DecodeOptions{})
}

// DecodeConfig returns the image configuration without decoding the full image.
func DecodeConfig(data []byte) (stdimage.Config, error) {
	img, err := Decode(data)
	if err != nil {
		return stdimage.Config{}, err
	}

	bounds := img.Bounds()
	return stdimage.Config{
		Width:      bounds.Dx(),
		Height:     bounds.Dy(),
		ColorModel: color.GrayModel,
	}, nil
}

// DecodeWithOptions decodes JBIG2 data with options.
type DecodeOptions struct {
	Globals []byte // Global segment data (for embedded JBIG2)
}

// DecodeWithOptions is an exported API.
func DecodeWithOptions(data []byte, opts DecodeOptions) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	globalCtx, globalCtxOwner, err := buildGlobalContext(opts.Globals)
	if err != nil {
		return nil, err
	}
	if globalCtxOwner != nil {
		defer globalCtxOwner()
	}

	embedded := 0
	if len(opts.Globals) > 0 {
		embedded = 1
	}

	ctx := C.go_jbig2_ctx_new(globalCtx, C.int(embedded))
	if ctx == nil {
		return nil, fmt.Errorf("failed to create jbig2 context")
	}
	defer C.jbig2_ctx_free(ctx)

	if ret := C.jbig2_data_in(ctx, (*C.uchar)(&data[0]), C.size_t(len(data))); ret < 0 {
		return nil, fmt.Errorf("jbig2_data_in failed with code %d", int(ret))
	}

	if ret := C.jbig2_complete_page(ctx); ret < 0 {
		return nil, fmt.Errorf("jbig2_complete_page failed with code %d", int(ret))
	}

	img := C.jbig2_page_out(ctx)
	if img == nil {
		return nil, fmt.Errorf("jbig2_page_out returned nil image")
	}
	defer C.jbig2_release_page(ctx, img)

	return convertImage(img)
}

func buildGlobalContext(globals []byte) (*C.Jbig2GlobalCtx, func(), error) {
	if len(globals) == 0 {
		return nil, nil, nil
	}

	ctx := C.go_jbig2_ctx_new(nil, 1)
	if ctx == nil {
		return nil, nil, fmt.Errorf("failed to create jbig2 global context")
	}

	if ret := C.jbig2_data_in(ctx, (*C.uchar)(&globals[0]), C.size_t(len(globals))); ret < 0 {
		C.jbig2_ctx_free(ctx)
		return nil, nil, fmt.Errorf("jbig2 global data_in failed with code %d", int(ret))
	}

	globalCtx := C.jbig2_make_global_ctx(ctx)
	if globalCtx == nil {
		C.jbig2_ctx_free(ctx)
		return nil, nil, fmt.Errorf("failed to create jbig2 global state")
	}

	C.jbig2_ctx_free(ctx)
	cleanup := func() {
		C.jbig2_global_ctx_free(globalCtx)
	}
	return globalCtx, cleanup, nil
}

// convertImage converts JBIG2 image to Go image.Gray.
func convertImage(cImage *C.Jbig2Image) (stdimage.Image, error) {
	if cImage == nil {
		return nil, fmt.Errorf("nil image")
	}

	width := int(cImage.width)
	height := int(cImage.height)
	stride := int(cImage.stride)
	if width <= 0 || height <= 0 || stride <= 0 || cImage.data == nil {
		return nil, fmt.Errorf("invalid jbig2 image metadata")
	}

	img := stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	rowBytes := (width + 7) / 8
	src := unsafeSlice((*byte)(cImage.data), stride*height)

	for y := 0; y < height; y++ {
		row := src[y*stride : y*stride+rowBytes]
		for x := 0; x < width; x++ {
			byteIdx := x / 8
			bitIdx := 7 - (x % 8)
			bit := (row[byteIdx] >> bitIdx) & 1
			if bit == 1 {
				img.SetGray(x, y, color.Gray{Y: 0})
				continue
			}
			img.SetGray(x, y, color.Gray{Y: 255})
		}
	}

	return img, nil
}

func unsafeSlice(ptr *byte, size int) []byte {
	return unsafe.Slice(ptr, size)
}
