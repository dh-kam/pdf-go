// Package image provides image decoding implementations for PDF images.
package image

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"

	djpeg "github.com/dh-kam/djpeg-go"
	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/image"
)

const imageMagickJPEGTimeout = 10 * time.Second
const enableDjpegGoJPEGEnv = "GO_PDF_ENABLE_DJPEG_GO"
const disableDCTDeviceCMYKPopplerLineEnv = "PDF_DISABLE_DCT_DEVICE_CMYK_POPPLER_LINE"
const debugDCTDeviceCMYKTraceEnv = "PDF_DEBUG_DCT_DEVICE_CMYK_TRACE"
const debugDCTDeviceCMYKForceBranchEnv = "PDF_DEBUG_DCT_DEVICE_CMYK_FORCE_BRANCH"

const (
	dctDeviceCMYKBranchAuto              = ""
	dctDeviceCMYKBranchImageMagickLine   = "imagemagick-popplerline"
	dctDeviceCMYKBranchStdlibPopplerLine = "stdlib-popplerline"
	dctDeviceCMYKBranchDjpegPopplerLine  = "djpeg-popplerline"
)

// JPEGDecoder decodes JPEG images (DCTDecode filter).
type JPEGDecoder struct{}

// NewJPEGDecoder creates a new JPEG decoder.
func NewJPEGDecoder() *JPEGDecoder {
	return &JPEGDecoder{}
}

// Decode decodes JPEG image data.
func (d *JPEGDecoder) Decode(data []byte) (stdimage.Image, error) {
	if img, err := decodeJPEGWithDjpegGo(data); err == nil {
		return img, nil
	}

	// Poppler's DCTStream delegates scanline output to libjpeg. Prefer the local
	// ImageMagick/libjpeg path so IDCT and color conversion match.
	if img, err := decodeJPEGWithImageMagick(data); err == nil {
		return img, nil
	}

	r := bytes.NewReader(data)
	img, err := jpeg.Decode(r)
	if err != nil {
		return nil, errors.Invalid("jpeg_decode", err)
	}
	return img, nil
}

func decodeJPEGWithDjpegGo(data []byte) (stdimage.Image, error) {
	if os.Getenv(enableDjpegGoJPEGEnv) != "1" {
		return nil, errors.Invalid("djpeg_go_jpeg_decode", fmt.Errorf("disabled"))
	}

	raster, err := djpeg.DecodeRaster(bytes.NewReader(data), djpeg.WithCompatibility(djpeg.CompatibilityPopplerPDF))
	if err != nil {
		return nil, errors.Invalid("djpeg_go_jpeg_decode", err)
	}
	return stdImageFromDjpegRaster(raster)
}

func stdImageFromDjpegRaster(raster *djpeg.Raster) (stdimage.Image, error) {
	if raster == nil {
		return nil, errors.Invalid("djpeg_go_jpeg_decode", fmt.Errorf("nil raster"))
	}
	if raster.Format == djpeg.PixelFormatUnknown {
		return nil, errors.Invalid("djpeg_go_jpeg_decode", fmt.Errorf("unsupported raster format: %s", raster.Format))
	}
	if raster.Format != djpeg.PixelFormatGray8 {
		return raster.RGBA(), nil
	}

	bounds := raster.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	out := stdimage.NewGray(bounds)
	for y := 0; y < height; y++ {
		src := raster.Pix[y*raster.Stride : y*raster.Stride+width]
		dst := out.Pix[y*out.Stride : y*out.Stride+width]
		copy(dst, src)
	}
	return out, nil
}

func decodeDeviceCMYKDCTWithPopplerLine(data []byte, width, height int) (stdimage.Image, error) {
	switch forced := forcedDCTDeviceCMYKBranch(); forced {
	case dctDeviceCMYKBranchImageMagickLine:
		return decodeDeviceCMYKDCTWithImageMagickCMYKPopplerLine(data, width, height)
	case dctDeviceCMYKBranchStdlibPopplerLine:
		return decodeDeviceCMYKDCTWithStdlibCMYKPopplerLine(data, width, height)
	case dctDeviceCMYKBranchDjpegPopplerLine:
		return decodeDeviceCMYKDCTWithDjpegCMYKPopplerLine(data, width, height)
	}

	// Poppler's DCTStream is libjpeg-backed; ImageMagick's JPEG delegate gives
	// closer CMYK IDCT parity than Go's stdlib decoder, with stdlib as fallback.
	if img, err := decodeDeviceCMYKDCTWithImageMagickCMYKPopplerLine(data, width, height); err == nil {
		return img, nil
	}

	if os.Getenv("PDF_DEBUG_DCT_DEVICE_CMYK_POPPLER_LINE") != "1" {
		return decodeDeviceCMYKDCTWithStdlibCMYKPopplerLine(data, width, height)
	}

	if img, err := decodeDeviceCMYKDCTWithStdlibCMYKPopplerLine(data, width, height); err == nil {
		return img, nil
	}

	return decodeDeviceCMYKDCTWithDjpegCMYKPopplerLine(data, width, height)
}

func decodeDeviceCMYKDCTWithImageMagickCMYKPopplerLine(data []byte, width, height int) (stdimage.Image, error) {
	if os.Getenv("GO_PDF_DISABLE_IMAGEMAGICK_JPEG") == "1" {
		return nil, errors.Invalid("imagemagick_cmyk_jpeg_decode", fmt.Errorf("disabled"))
	}
	if width <= 0 || height <= 0 {
		return nil, errors.Invalidf("imagemagick_cmyk_jpeg_decode", "invalid image size: %dx%d", width, height)
	}

	convertPath, err := exec.LookPath("convert")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), imageMagickJPEGTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, convertPath, "jpeg:-", "-depth", "8", "cmyk:-")
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("convert jpeg to raw cmyk failed: %w: %s", err, stderr.String())
	}

	// CMYK JPEGs with Adobe APP14 use the same sample polarity correction as
	// Poppler/libjpeg's DCTStream setup before DeviceCMYK getRGBLine conversion.
	return rgbaFromImageMagickCMYKPopplerLine(raw, width, height, jpegHasAdobeMarker(data))
}

func rgbaFromImageMagickCMYKPopplerLine(raw []byte, width, height int, invertSamples bool) (*stdimage.RGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.Invalidf("imagemagick_cmyk_jpeg_decode", "invalid image size: %dx%d", width, height)
	}
	expected := width * height * 4
	if len(raw) != expected {
		return nil, errors.Invalidf(
			"imagemagick_cmyk_jpeg_decode",
			"raw cmyk length %d does not match image size %dx%d",
			len(raw),
			width,
			height,
		)
	}

	out := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	src := 0
	dst := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := raw[src]
			m := raw[src+1]
			yComp := raw[src+2]
			k := raw[src+3]
			if invertSamples {
				c = 0xff - c
				m = 0xff - m
				yComp = 0xff - yComp
				k = 0xff - k
			}
			rgba := colorspace.ConvertDeviceCMYKBytesToRGBLineRGBA(c, m, yComp, k)
			out.Pix[dst] = rgba.R
			out.Pix[dst+1] = rgba.G
			out.Pix[dst+2] = rgba.B
			out.Pix[dst+3] = rgba.A
			src += 4
			dst += 4
		}
	}
	return out, nil
}

func decodeDeviceCMYKDCTWithDjpegCMYKPopplerLine(data []byte, width, height int) (stdimage.Image, error) {
	opts := []djpeg.Option{
		djpeg.WithCompatibility(djpeg.CompatibilityPopplerPDF),
		djpeg.WithOutputColorSpace(djpeg.ColorSpaceCMYK),
	}
	if inputColorSpace, ok := popplerDeviceCMYKDCTInputColorSpace(data); ok {
		opts = append(opts, djpeg.WithInputColorSpace(inputColorSpace))
	}

	raster, err := djpeg.DecodeRaster(bytes.NewReader(data), opts...)
	if err != nil {
		return nil, errors.Invalid("dct_device_cmyk_poppler_line", err)
	}
	if raster == nil || raster.Format != djpeg.PixelFormatCMYK32 {
		return nil, errors.Invalidf("dct_device_cmyk_poppler_line", "unexpected raster format: %v", raster)
	}

	bounds := raster.Bounds()
	if width > 0 && height > 0 && (bounds.Dx() != width || bounds.Dy() != height) {
		return nil, errors.Invalidf(
			"dct_device_cmyk_poppler_line",
			"raster size %dx%d does not match PDF image size %dx%d",
			bounds.Dx(),
			bounds.Dy(),
			width,
			height,
		)
	}

	out := stdimage.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		src := (y-bounds.Min.Y)*raster.Stride + (bounds.Min.X-raster.Rect.Min.X)*4
		dst := (y-out.Rect.Min.Y)*out.Stride + (bounds.Min.X-out.Rect.Min.X)*4
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba := colorspace.ConvertDeviceCMYKBytesToRGBLineRGBA(
				raster.Pix[src],
				raster.Pix[src+1],
				raster.Pix[src+2],
				raster.Pix[src+3],
			)
			out.Pix[dst] = rgba.R
			out.Pix[dst+1] = rgba.G
			out.Pix[dst+2] = rgba.B
			out.Pix[dst+3] = rgba.A
			src += 4
			dst += 4
		}
	}
	return out, nil
}

func forcedDCTDeviceCMYKBranch() string {
	switch os.Getenv(debugDCTDeviceCMYKForceBranchEnv) {
	case dctDeviceCMYKBranchImageMagickLine:
		return dctDeviceCMYKBranchImageMagickLine
	case dctDeviceCMYKBranchStdlibPopplerLine:
		return dctDeviceCMYKBranchStdlibPopplerLine
	case dctDeviceCMYKBranchDjpegPopplerLine:
		return dctDeviceCMYKBranchDjpegPopplerLine
	default:
		return dctDeviceCMYKBranchAuto
	}
}

func decodeDeviceCMYKDCTWithStdlibCMYKPopplerLine(data []byte, width, height int) (stdimage.Image, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	cmyk, ok := img.(*stdimage.CMYK)
	if !ok {
		return nil, errors.Invalidf("dct_device_cmyk_poppler_line", "unexpected stdlib image type: %T", img)
	}

	bounds := cmyk.Bounds()
	if width > 0 && height > 0 && (bounds.Dx() != width || bounds.Dy() != height) {
		return nil, errors.Invalidf(
			"dct_device_cmyk_poppler_line",
			"stdlib image size %dx%d does not match PDF image size %dx%d",
			bounds.Dx(),
			bounds.Dy(),
			width,
			height,
		)
	}

	invertSamples := jpegHasAdobeMarker(data)
	out := stdimage.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		src := (y-cmyk.Rect.Min.Y)*cmyk.Stride + (bounds.Min.X-cmyk.Rect.Min.X)*4
		dst := (y-out.Rect.Min.Y)*out.Stride + (bounds.Min.X-out.Rect.Min.X)*4
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := cmyk.Pix[src]
			m := cmyk.Pix[src+1]
			yComp := cmyk.Pix[src+2]
			k := cmyk.Pix[src+3]
			if invertSamples {
				c = 0xff - c
				m = 0xff - m
				yComp = 0xff - yComp
				k = 0xff - k
			}
			rgba := colorspace.ConvertDeviceCMYKBytesToRGBLineRGBA(c, m, yComp, k)
			out.Pix[dst] = rgba.R
			out.Pix[dst+1] = rgba.G
			out.Pix[dst+2] = rgba.B
			out.Pix[dst+3] = rgba.A
			src += 4
			dst += 4
		}
	}
	return out, nil
}

func jpegHasAdobeMarker(data []byte) bool {
	cfg, err := djpeg.DecodeRasterConfig(bytes.NewReader(data), djpeg.WithCompatibility(djpeg.CompatibilityPopplerPDF))
	return err == nil && cfg.SawAdobeMarker
}

func traceDeviceCMYKDCTDecodeCandidate(data []byte, meta *image.ImageData) {
	if os.Getenv(debugDCTDeviceCMYKTraceEnv) != "1" || meta == nil || meta.Filter != image.FilterDCT {
		return
	}

	forced := forcedDCTDeviceCMYKBranch()
	cfg, cfgErr := djpeg.DecodeRasterConfig(
		bytes.NewReader(data),
		djpeg.WithCompatibility(djpeg.CompatibilityPopplerPDF),
		djpeg.WithOutputColorSpace(djpeg.ColorSpaceCMYK),
	)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "DCT_CMYK_TRACE width=%d height=%d colorspace=%s bpc=%d decode=%d icc=%d raw_crc=%08x force=%s config_error=%v\n",
			meta.Width,
			meta.Height,
			meta.ColorSpace,
			meta.BitsPerComponent,
			len(meta.Decode),
			len(meta.ICCProfile),
			crc32.ChecksumIEEE(data),
			forced,
			cfgErr,
		)
		return
	}

	inputColorSpace, hasInputColorSpace := popplerDeviceCMYKDCTInputColorSpace(data)
	fmt.Fprintf(os.Stderr, "DCT_CMYK_TRACE width=%d height=%d colorspace=%s bpc=%d decode=%d icc=%d raw_crc=%08x force=%s components=%d adobe=%t transform=%d input=%v input_forced=%t\n",
		meta.Width,
		meta.Height,
		meta.ColorSpace,
		meta.BitsPerComponent,
		len(meta.Decode),
		len(meta.ICCProfile),
		crc32.ChecksumIEEE(data),
		forced,
		cfg.InputComponents,
		cfg.SawAdobeMarker,
		cfg.AdobeTransform,
		inputColorSpace,
		hasInputColorSpace,
	)
}

func popplerDeviceCMYKDCTInputColorSpace(data []byte) (djpeg.InputColorSpace, bool) {
	cfg, err := djpeg.DecodeRasterConfig(
		bytes.NewReader(data),
		djpeg.WithCompatibility(djpeg.CompatibilityPopplerPDF),
		djpeg.WithOutputColorSpace(djpeg.ColorSpaceCMYK),
	)
	if err != nil || cfg.InputComponents != 4 {
		return djpeg.InputAuto, false
	}

	// Poppler DCTStream.cc sets 4-component jpeg_color_space from the Adobe
	// APP14 transform only; component IDs such as 1/2/3/4 do not imply YCCK.
	if cfg.SawAdobeMarker && cfg.AdobeTransform != 0 {
		return djpeg.InputYCCK, true
	}
	return djpeg.InputCMYK, true
}

func decodeJPEGWithImageMagick(data []byte) (stdimage.Image, error) {
	if os.Getenv("GO_PDF_DISABLE_IMAGEMAGICK_JPEG") == "1" {
		return nil, errors.Invalid("imagemagick_jpeg_decode", fmt.Errorf("disabled"))
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var outputFormat string
	var parse func([]byte) (stdimage.Image, error)
	switch cfg.ColorModel {
	case color.YCbCrModel:
		outputFormat = "ppm:-"
		parse = parseBinaryPPM
	case color.GrayModel:
		outputFormat = "pgm:-"
		parse = parseBinaryPGM
	default:
		return nil, errors.Invalid("imagemagick_jpeg_decode", fmt.Errorf("unsupported color model"))
	}

	convertPath, err := exec.LookPath("convert")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), imageMagickJPEGTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, convertPath, "jpeg:-", outputFormat)
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("convert jpeg to ppm failed: %w: %s", err, stderr.String())
	}

	return parse(out)
}

func parseBinaryPPM(data []byte) (stdimage.Image, error) {
	offset := 0
	magic, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}
	if magic != "P6" {
		return nil, fmt.Errorf("unsupported ppm magic: %s", magic)
	}

	widthToken, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}
	heightToken, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}
	maxValueToken, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}

	width, err := strconv.Atoi(widthToken)
	if err != nil || width <= 0 {
		return nil, fmt.Errorf("invalid ppm width: %s", widthToken)
	}
	height, err := strconv.Atoi(heightToken)
	if err != nil || height <= 0 {
		return nil, fmt.Errorf("invalid ppm height: %s", heightToken)
	}
	maxValue, err := strconv.Atoi(maxValueToken)
	if err != nil || maxValue != 255 {
		return nil, fmt.Errorf("unsupported ppm max value: %s", maxValueToken)
	}

	if offset < len(data) && isPPMWhitespace(data[offset]) {
		offset++
	}

	pixelBytes := width * height * 3
	if len(data)-offset < pixelBytes {
		return nil, fmt.Errorf("truncated ppm raster: expected %d bytes, got %d", pixelBytes, len(data)-offset)
	}

	out := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	src := data[offset : offset+pixelBytes]
	dst := out.Pix
	for srcOffset, dstOffset := 0, 0; srcOffset < len(src); srcOffset, dstOffset = srcOffset+3, dstOffset+4 {
		dst[dstOffset+0] = src[srcOffset+0]
		dst[dstOffset+1] = src[srcOffset+1]
		dst[dstOffset+2] = src[srcOffset+2]
		dst[dstOffset+3] = 255
	}
	return out, nil
}

func parseBinaryPGM(data []byte) (stdimage.Image, error) {
	offset := 0
	magic, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}
	if magic != "P5" {
		return nil, fmt.Errorf("unsupported pgm magic: %s", magic)
	}

	widthToken, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}
	heightToken, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}
	maxValueToken, err := nextPPMToken(data, &offset)
	if err != nil {
		return nil, err
	}

	width, err := strconv.Atoi(widthToken)
	if err != nil || width <= 0 {
		return nil, fmt.Errorf("invalid pgm width: %s", widthToken)
	}
	height, err := strconv.Atoi(heightToken)
	if err != nil || height <= 0 {
		return nil, fmt.Errorf("invalid pgm height: %s", heightToken)
	}
	maxValue, err := strconv.Atoi(maxValueToken)
	if err != nil || maxValue != 255 {
		return nil, fmt.Errorf("unsupported pgm max value: %s", maxValueToken)
	}

	if offset < len(data) && isPPMWhitespace(data[offset]) {
		offset++
	}

	pixelBytes := width * height
	if len(data)-offset < pixelBytes {
		return nil, fmt.Errorf("truncated pgm raster: expected %d bytes, got %d", pixelBytes, len(data)-offset)
	}

	out := stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	copy(out.Pix, data[offset:offset+pixelBytes])
	return out, nil
}

func nextPPMToken(data []byte, offset *int) (string, error) {
	skipPPMHeaderWhitespace(data, offset)
	if *offset >= len(data) {
		return "", fmt.Errorf("unexpected end of ppm header")
	}

	start := *offset
	for *offset < len(data) && !isPPMWhitespace(data[*offset]) {
		*offset++
	}
	return string(data[start:*offset]), nil
}

func skipPPMHeaderWhitespace(data []byte, offset *int) {
	for *offset < len(data) {
		if isPPMWhitespace(data[*offset]) {
			*offset++
			continue
		}
		if data[*offset] == '#' {
			for *offset < len(data) && data[*offset] != '\n' && data[*offset] != '\r' {
				*offset++
			}
			continue
		}
		return
	}
}

func isPPMWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

// DecodeConfig returns the JPEG image configuration.
func (d *JPEGDecoder) DecodeConfig(data []byte) (stdimage.Config, error) {
	r := bytes.NewReader(data)
	cfg, err := jpeg.DecodeConfig(r)
	if err != nil {
		return stdimage.Config{}, errors.Invalid("jpeg_config", err)
	}
	return cfg, nil
}

// ColorSpace returns the color space for JPEG images.
func (d *JPEGDecoder) ColorSpace() image.ColorSpace {
	// JPEG can be grayscale, RGB, or CMYK
	// We'll determine this from the actual image during decoding
	return image.ColorSpaceDeviceRGB
}

// JPEGImage represents a decoded JPEG image.
type JPEGImage struct {
	img              stdimage.Image
	mask             image.ImageMask
	colorSpace       image.ColorSpace
	width            int
	height           int
	bitsPerComponent int
}

// NewJPEGImage creates a new JPEG image.
func NewJPEGImage(img stdimage.Image, colorSpace image.ColorSpace, bitsPerComponent int) *JPEGImage {
	bounds := img.Bounds()
	return &JPEGImage{
		img:              img,
		width:            bounds.Dx(),
		height:           bounds.Dy(),
		colorSpace:       colorSpace,
		bitsPerComponent: bitsPerComponent,
	}
}

// Image returns the Go standard library image.
func (i *JPEGImage) Image() stdimage.Image {
	return i.img
}

// Width returns the image width.
func (i *JPEGImage) Width() int {
	return i.width
}

// Height returns the image height.
func (i *JPEGImage) Height() int {
	return i.height
}

// ColorSpace returns the image color space.
func (i *JPEGImage) ColorSpace() image.ColorSpace {
	return i.colorSpace
}

// BitsPerComponent returns the number of bits per component.
func (i *JPEGImage) BitsPerComponent() int {
	return i.bitsPerComponent
}

// HasMask returns true if the image has a mask.
func (i *JPEGImage) HasMask() bool {
	return i.mask != nil
}

// Mask returns the image mask.
func (i *JPEGImage) Mask() image.ImageMask {
	return i.mask
}

// SetMask sets the image mask.
func (i *JPEGImage) SetMask(mask image.ImageMask) {
	i.mask = mask
}

// JPEGDecoderFactory creates JPEG decoders.
type JPEGDecoderFactory struct{}

// NewJPEGDecoderFactory creates a new JPEG decoder factory.
func NewJPEGDecoderFactory() *JPEGDecoderFactory {
	return &JPEGDecoderFactory{}
}

// CreateDecoder creates a JPEG decoder.
func (f *JPEGDecoderFactory) CreateDecoder(filter image.ImageFilter) (image.Decoder, error) {
	if filter != image.FilterDCT {
		return nil, fmt.Errorf("JPEG decoder only supports DCTDecode filter, got: %s", filter)
	}
	return NewJPEGDecoder(), nil
}

// CanDecode returns true if the filter is DCTDecode (JPEG).
func (f *JPEGDecoderFactory) CanDecode(filter image.ImageFilter) bool {
	return filter == image.FilterDCT
}

// decodeArray applies a decode array to map color values.
func decodeArray(value float64, decode []float64, maxValue float64) float64 {
	if len(decode) < 2 {
		return value
	}

	if maxValue <= 0 {
		return value
	}

	// Linear interpolation: decode[0] + value * (decode[1] - decode[0])
	// where value is normalized to [0, 1]
	normalized := value / maxValue
	if normalized < 0 {
		normalized = 0
	} else if normalized > 1 {
		normalized = 1
	}
	return decode[0] + normalized*(decode[1]-decode[0])
}

func isDecodeNormalized(decode []float64) bool {
	if len(decode) == 0 {
		return false
	}

	const decodeNormEps = 1e-9
	for _, v := range decode {
		if v < -decodeNormEps || v > 1+decodeNormEps {
			return false
		}
	}
	return true
}

func clampToByte(value float64, decodeNormalized bool) uint8 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if decodeNormalized {
		value *= 255
	}
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(value)
}

// applyDecode applies the decode array to an image.
func applyDecode(img stdimage.Image, decode []float64, bitsPerComponent int) stdimage.Image {
	if len(decode) == 0 {
		return img
	}

	// Raw image decode paths normalize sample values to [0,255] before this step.
	// Keep maxValue fixed at 255 so decode array semantics match PDF image sampling inputs.
	_ = bitsPerComponent
	maxValue := 255.0
	decodeIsNormalized := isDecodeNormalized(decode)

	switch img := img.(type) {
	case *stdimage.Gray:
		bounds := img.Bounds()
		out := stdimage.NewGray(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				orig := img.GrayAt(x, y)
				newValue := decodeArray(float64(orig.Y), decode, maxValue)
				out.SetGray(x, y, color.Gray{Y: clampToByte(newValue, decodeIsNormalized)})
			}
		}
		return out

	case *stdimage.RGBA:
		bounds := img.Bounds()
		out := stdimage.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				orig := img.RGBAAt(x, y)
				var r, g, b, a uint8
				if len(decode) >= 2 {
					r = clampToByte(decodeArray(float64(orig.R), decode[0:2], maxValue), decodeIsNormalized)
				} else {
					r = orig.R
				}
				if len(decode) >= 4 {
					g = clampToByte(decodeArray(float64(orig.G), decode[2:4], maxValue), decodeIsNormalized)
				} else {
					g = orig.G
				}
				if len(decode) >= 6 {
					b = clampToByte(decodeArray(float64(orig.B), decode[4:6], maxValue), decodeIsNormalized)
				} else {
					b = orig.B
				}
				a = orig.A
				out.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
			}
		}
		return out

	default:
		return img
	}
}
