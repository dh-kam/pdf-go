// Package renderer provides PDF content stream evaluation and rendering.
//
//revive:disable:exported
//nolint:errcheck,govet,ineffassign
package renderer

import (
	"fmt"
	stdimage "image"
	"math"
	"os"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
)

func (e *Evaluator) emitImageSamplingTrace(
	filter domainimage.ImageFilter,
	colorSpace string,
	indexedBase string,
	indexedLookupLen int,
	cmykConversionMode string,
	imageEdgeMode string,
	indexedGrayExperimentalCandidate string,
	cmykExperimentalCandidate string,
	edgeExperimentalCandidate string,
	grayICCExperimentalCandidate string,
	grayICCProfileMode string,
	sampler string,
	reason string,
	experimentalCandidate string,
	ctm [6]float64,
	phaseX, phaseY float64,
	x, y, width, height float64,
	img stdimage.Image,
) {
	if !e.debugImageSampling {
		return
	}
	docID := e.debugDocumentID
	if docID == "" {
		docID = "unknown"
	}
	filterName := string(filter)
	if filterName == "" {
		filterName = "none"
	}
	page := e.debugPageNumber
	if page <= 0 {
		page = -1
	}
	srcW, srcH := 0, 0
	if img != nil {
		srcW = img.Bounds().Dx()
		srcH = img.Bounds().Dy()
	}
	indexedInfo := ""
	if strings.EqualFold(colorSpace, "Indexed") {
		indexedInfo = fmt.Sprintf(
			" indexed_base=%s indexed_palette_entries=%d",
			indexedBase,
			indexedPaletteEntries(indexedBase, indexedLookupLen),
		)
	}
	cmykInfo := ""
	if strings.EqualFold(colorSpace, "Indexed") {
		cmykInfo = fmt.Sprintf(
			" indexed_gray_candidate=%s cmyk_candidate=%s",
			indexedGrayExperimentalCandidate,
			cmykExperimentalCandidate,
		)
	}
	if usesCMYKImageConversion(colorSpace, indexedBase) {
		cmykInfo = fmt.Sprintf(
			"%s cmyk_conversion_mode=%s",
			cmykInfo,
			formatCMYKConversionModeForTrace(cmykConversionMode),
		)
	}
	edgeInfo := fmt.Sprintf(
		" edge_candidate=%s edge_mode=%s",
		edgeExperimentalCandidate,
		formatImageEdgeModeForTrace(imageEdgeMode),
	)
	grayICCInfo := fmt.Sprintf(
		" gray_icc_candidate=%s gray_icc_profile_mode=%s",
		grayICCExperimentalCandidate,
		formatGrayICCProfileModeForTrace(grayICCProfileMode),
	)
	fmt.Fprintf(
		os.Stderr,
		"[image-sampling] doc=%s page=%d filter=%s colorspace=%s%s%s%s%s sampler=%s reason=%s experimental_candidate=%s ctm=[%.6f %.6f %.6f %.6f %.6f %.6f] phase=(x=%.4f y=%.4f) dst=(x=%.4f y=%.4f w=%.4f h=%.4f) src=%dx%d\n",
		docID,
		page,
		filterName,
		colorSpace,
		indexedInfo,
		cmykInfo,
		edgeInfo,
		grayICCInfo,
		sampler,
		reason,
		experimentalCandidate,
		ctm[0], ctm[1], ctm[2], ctm[3], ctm[4], ctm[5],
		phaseX, phaseY,
		x, y, width, height,
		srcW, srcH,
	)
}

func indexedPaletteEntries(base string, lookupLen int) int {
	switch {
	case lookupLen <= 0:
		return 0
	case strings.EqualFold(base, "DeviceGray"):
		return lookupLen
	case strings.EqualFold(base, "DeviceRGB"):
		return lookupLen / 3
	case strings.EqualFold(base, "DeviceCMYK"):
		return lookupLen / 4
	default:
		return 0
	}
}

func formatCMYKConversionModeForTrace(mode string) string {
	if mode == "" {
		return "default"
	}
	return mode
}

func formatImageEdgeModeForTrace(mode string) string {
	if mode == "" {
		return "default"
	}
	return mode
}

func formatGrayICCProfileModeForTrace(mode string) string {
	if mode == "" {
		return "default"
	}
	return mode
}

func usesCMYKImageConversion(colorSpace, indexedBase string) bool {
	return strings.EqualFold(colorSpace, "DeviceCMYK") ||
		(strings.EqualFold(colorSpace, "Indexed") && strings.EqualFold(indexedBase, "DeviceCMYK"))
}

func classifyExperimentalRGBEdgeCandidate(
	colorSpace string,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
	ctm [6]float64,
) string {
	if !strings.EqualFold(colorSpace, "DeviceRGB") {
		return "rejected_non_rgb_colorspace"
	}
	if srcWidth <= 0 || srcHeight <= 0 {
		return "rejected_invalid_source"
	}
	if math.Abs(ctm[1]) > 1e-12 || math.Abs(ctm[2]) > 1e-12 {
		return "rejected_non_axis_aligned"
	}
	if !isImageUpscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return "rejected_non_upscale"
	}
	if ctm[5] <= math.Floor(ctm[5]) {
		return "rejected_zero_or_negative_vertical_offset"
	}
	return "candidate_positive_subpixel_vertical_offset"
}

func classifyExperimentalIndexedCMYKCandidate(
	colorSpace string,
	indexedBase string,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
) string {
	if !strings.EqualFold(colorSpace, "Indexed") {
		return "rejected_non_indexed_colorspace"
	}
	if !strings.EqualFold(indexedBase, "DeviceCMYK") {
		return "rejected_non_cmyk_indexed_base"
	}
	if srcWidth <= 0 || srcHeight <= 0 {
		return "rejected_invalid_source"
	}
	if srcWidth < 256 || srcHeight < 256 {
		return "rejected_small_source"
	}
	if isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return "candidate_large_indexed_cmyk_downscale"
	}
	if isImageUpscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return "candidate_indexed_cmyk_upscale"
	}
	return "rejected_near_identity"
}

func classifyExperimentalIndexedGrayOriginCandidate(
	colorSpace string,
	indexedBase string,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
	ctm [6]float64,
) string {
	if !strings.EqualFold(colorSpace, "Indexed") {
		return "rejected_non_indexed_colorspace"
	}
	if !strings.EqualFold(indexedBase, "DeviceGray") {
		return "rejected_non_gray_indexed_base"
	}
	if srcWidth <= 0 || srcHeight <= 0 {
		return "rejected_invalid_source"
	}
	if srcWidth < 256 || srcHeight < 256 {
		return "rejected_small_source"
	}
	if !isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return "rejected_non_downscale"
	}
	if math.Abs(ctm[1]) > 1e-6 || math.Abs(ctm[2]) > 1e-6 {
		return "rejected_non_axis_aligned"
	}
	if math.Abs(ctm[4]) > 1e-6 || math.Abs(ctm[5]) > 1e-6 {
		return "rejected_non_origin_placement"
	}
	return "candidate_large_indexed_gray_origin_downscale"
}

func classifyExperimentalDCTGrayIgnoreICCCandidate(
	filter domainimage.ImageFilter,
	colorSpace string,
	sourceICCBased bool,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
	ctm [6]float64,
) string {
	if filter != domainimage.FilterDCT {
		return "rejected_non_dct_filter"
	}
	if !strings.EqualFold(colorSpace, "DeviceGray") {
		return "rejected_non_gray_colorspace"
	}
	if !sourceICCBased {
		return "rejected_non_iccbased_source"
	}
	if srcWidth <= 0 || srcHeight <= 0 {
		return "rejected_invalid_source"
	}
	if srcWidth > 32 || srcHeight > 32 {
		return "rejected_large_source"
	}
	if !isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return "rejected_non_downscale"
	}
	if math.Abs(ctm[1]) > 1e-6 || math.Abs(ctm[2]) > 1e-6 {
		return "rejected_non_axis_aligned"
	}
	return "candidate_tiny_dct_iccbased_gray_downscale"
}

func resolveExperimentalIndexedCMYKConversionMode(mode string, candidate string) string {
	switch candidate {
	case "candidate_large_indexed_cmyk_downscale":
		switch normalizeImageSamplingMode(mode) {
		case ImageSamplingModeLegacy:
			return domainimage.CMYKConversionModeHybrid75
		case ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1:
			return domainimage.CMYKConversionModeHybrid75
		case ImageSamplingModeExperimentalDCTGrayIgnoreICCV1:
			return domainimage.CMYKConversionModeHybrid75
		case ImageSamplingModeExperimentalIndexedCMYKSimpleV1:
			return domainimage.CMYKConversionModeSimpleSubtractive
		case ImageSamplingModeExperimentalIndexedCMYKHybrid75V1:
			return domainimage.CMYKConversionModeHybrid75
		case ImageSamplingModeExperimentalIndexedCMYKStdlibV1:
			return domainimage.CMYKConversionModeStdlib
		case ImageSamplingModeExperimentalIndexedCMYKLUTV1:
			return domainimage.CMYKConversionModeLUT
		default:
			return domainimage.CMYKConversionModeDefault
		}
	case "candidate_indexed_cmyk_upscale":
		switch normalizeImageSamplingMode(mode) {
		case ImageSamplingModeLegacy:
			return domainimage.CMYKConversionModeStdlib
		case ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1:
			return domainimage.CMYKConversionModeStdlib
		case ImageSamplingModeExperimentalDCTGrayIgnoreICCV1:
			return domainimage.CMYKConversionModeStdlib
		case ImageSamplingModeExperimentalIndexedCMYKSimpleV1:
			return domainimage.CMYKConversionModeSimpleSubtractive
		case ImageSamplingModeExperimentalIndexedCMYKHybrid75V1:
			return domainimage.CMYKConversionModeHybrid75
		case ImageSamplingModeExperimentalIndexedCMYKStdlibV1:
			return domainimage.CMYKConversionModeStdlib
		case ImageSamplingModeExperimentalIndexedCMYKLUTV1:
			return domainimage.CMYKConversionModeLUT
		default:
			return domainimage.CMYKConversionModePoly8
		}
	default:
		return domainimage.CMYKConversionModeDefault
	}
}

func resolveSelectiveIndexedGrayOriginDownscaleSampler(
	mode string,
	candidate string,
	sampler string,
	reason string,
) (string, string) {
	if candidate != "candidate_large_indexed_gray_origin_downscale" {
		return sampler, reason
	}
	if sampler != "auto_downscale_bilinear" {
		return sampler, reason
	}
	return "experimental_indexed_origin_downscale_bilinear", "legacy_selective_indexed_origin_downscale_phase"
}

func resolveExperimentalImageEdgeMode(
	mode string,
	candidate string,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
) string {
	if candidate != "candidate_positive_subpixel_vertical_offset" {
		return domainimage.ImageEdgeModeDefault
	}
	if !supportsExperimentalRGBTransparentEdgeSurface(srcWidth, srcHeight, dstWidth, dstHeight) {
		return domainimage.ImageEdgeModeDefault
	}

	switch normalizeImageSamplingMode(mode) {
	case ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1:
		return domainimage.ImageEdgeModeTransparentEdgeOverWhite
	default:
		return domainimage.ImageEdgeModeDefault
	}
}

func resolveExperimentalDCTGrayICCProfile(
	mode string,
	candidate string,
	iccProfile []byte,
	iccComponents int,
) ([]byte, int, string) {
	if candidate != "candidate_tiny_dct_iccbased_gray_downscale" {
		return iccProfile, iccComponents, "default"
	}

	switch normalizeImageSamplingMode(mode) {
	case ImageSamplingModeLegacy:
		return nil, 0, "legacy_selective_ignore"
	case ImageSamplingModeExperimentalDCTGrayIgnoreICCV1:
		return nil, 0, "ignore"
	default:
		return iccProfile, iccComponents, "default"
	}
}

func supportsExperimentalRGBTransparentEdgeSurface(
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if srcWidth > 32 || srcHeight > 32 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	return dstWidth <= 32 && dstHeight <= 32
}

const imageNearIdentityScaleTolerance = 1e-3

type imageSamplingDecision struct {
	Interpolate           bool
	Sampler               string
	Reason                string
	ExperimentalCandidate string
}

func chooseImageSamplingPolicy(
	mode string,
	interpolate bool,
	interpolateExplicit bool,
	filter domainimage.ImageFilter,
	colorSpace string,
	sourceICCBased bool,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
) imageSamplingDecision {
	mode = normalizeImageSamplingMode(mode)
	experimentalCandidate := classifyExperimentalSplashScaleOnlyCandidate(
		interpolateExplicit,
		colorSpace,
		srcWidth,
		srcHeight,
		dstWidth,
		dstHeight,
	)

	if interpolateExplicit {
		if interpolate {
			return imageSamplingDecision{
				Interpolate:           true,
				Sampler:               "explicit_approx_bilinear",
				Reason:                "explicit_interpolate=true",
				ExperimentalCandidate: experimentalCandidate,
			}
		}
		return imageSamplingDecision{
			Interpolate:           false,
			Sampler:               "explicit_nearest",
			Reason:                "explicit_interpolate=false",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if mode == ImageSamplingModeAdaptiveDCTICCBasedV1 && interpolate {
		if isTinyImageSource(srcWidth, srcHeight) &&
			isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
			!isEncodedImageFilter(filter) &&
			strings.EqualFold(colorSpace, "DeviceGray") {
			return imageSamplingDecision{
				Interpolate:           false,
				Sampler:               "adaptive_nearest_tiny_gray_downscale_non_encoded",
				Reason:                "adaptive_tiny_gray_downscale_non_encoded",
				ExperimentalCandidate: experimentalCandidate,
			}
		}
		if isEncodedImageFilter(filter) &&
			(sourceICCBased || strings.EqualFold(colorSpace, "DeviceCMYK")) {
			return imageSamplingDecision{
				Interpolate:           true,
				Sampler:               "adaptive_approx_bilinear_dct_or_iccbased",
				Reason:                "adaptive_encoded_or_iccbased_downscale",
				ExperimentalCandidate: experimentalCandidate,
			}
		}
	}

	if mode == ImageSamplingModeAdaptiveDCTICCBasedV1 &&
		!interpolate &&
		!interpolateExplicit &&
		isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		isStrongImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		srcWidth <= 16 &&
		srcHeight <= 16 &&
		sourceICCBased &&
		!isEncodedImageFilter(filter) {
		return imageSamplingDecision{
			Interpolate:           false,
			Sampler:               "adaptive_nearest_tiny_gray_downscale_non_encoded",
			Reason:                "adaptive_tiny_gray_downscale_iccbased",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if mode == ImageSamplingModeAdaptiveDCTICCBasedV1 &&
		!interpolate &&
		!interpolateExplicit &&
		isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		isStrongImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		isEncodedImageFilter(filter) &&
		strings.EqualFold(colorSpace, "DeviceGray") &&
		srcWidth <= 16 &&
		srcHeight <= 16 {
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "adaptive_downscale_bilinear_tiny_encoded_gray",
			Reason:                "adaptive_tiny_gray_downscale_encoded",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if mode == ImageSamplingModeAdaptiveDCTICCBasedV1 &&
		!interpolate &&
		!interpolateExplicit &&
		isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "auto_downscale_bilinear",
			Reason:                "adaptive_downscale",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if mode == ImageSamplingModeAdaptiveDCTICCBasedV1 &&
		!interpolate &&
		!interpolateExplicit &&
		isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		isStrongImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		!isEncodedImageFilter(filter) &&
		(strings.EqualFold(colorSpace, "DeviceGray") || strings.EqualFold(colorSpace, "Indexed")) &&
		srcWidth <= 16 &&
		srcHeight <= 16 {
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "adaptive_approx_bilinear_tiny_gray_downscale_non_encoded",
			Reason:                "adaptive_tiny_gray_downscale_non_encoded",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if mode == ImageSamplingModeExperimentalSplashScaleOnlyV1 &&
		isExperimentalSplashScaleOnlyCandidate(
			interpolateExplicit,
			colorSpace,
			srcWidth,
			srcHeight,
			dstWidth,
			dstHeight,
		) {
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "experimental_splash_scale_only",
			Reason:                "experimental_splash_scale_only_small_gray_or_indexed",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if mode == ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1 &&
		!interpolate &&
		!interpolateExplicit &&
		isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
		strings.EqualFold(colorSpace, "Indexed") {
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "experimental_indexed_origin_downscale_bilinear",
			Reason:                "experimental_indexed_origin_downscale_phase",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if !interpolate &&
		!interpolateExplicit &&
		isNearIdentityImageScale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return imageSamplingDecision{
			Interpolate:           false,
			Sampler:               "auto_nearest",
			Reason:                "auto_interpolate=false_near_identity_scale",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if !interpolate &&
		!interpolateExplicit &&
		isImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		if strings.EqualFold(colorSpace, "DeviceGray") &&
			srcWidth <= 16 &&
			srcHeight <= 16 &&
			isStrongImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) &&
			sourceICCBased {
			return imageSamplingDecision{
				Interpolate:           true,
				Sampler:               "auto_box_tiny_iccbased_gray_downscale",
				Reason:                "auto_interpolate=false_downscale_tiny_iccbased_gray",
				ExperimentalCandidate: experimentalCandidate,
			}
		}

		if (strings.EqualFold(colorSpace, "DeviceGray") || strings.EqualFold(colorSpace, "Indexed")) &&
			srcWidth <= 32 &&
			srcHeight <= 32 {
			if filter == domainimage.FilterCCITTFax &&
				strings.EqualFold(colorSpace, "DeviceGray") {
				return imageSamplingDecision{
					Interpolate:           true,
					Sampler:               "auto_approx_bilinear_tiny_gray_ccittfax_downscale",
					Reason:                "auto_interpolate=false_downscale_tiny_gray_ccittfax",
					ExperimentalCandidate: experimentalCandidate,
				}
			}
			return imageSamplingDecision{
				Interpolate:           true,
				Sampler:               "auto_approx_bilinear",
				Reason:                "auto_interpolate=false_downscale_small_grayscale",
				ExperimentalCandidate: experimentalCandidate,
			}
		}

		if isStrongImageDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
			if srcWidth <= 16 && srcHeight <= 16 {
				return imageSamplingDecision{
					Interpolate:           true,
					Sampler:               "auto_downscale_bilinear",
					Reason:                "auto_interpolate=false_downscale_small_source",
					ExperimentalCandidate: experimentalCandidate,
				}
			}
			return imageSamplingDecision{
				Interpolate:           true,
				Sampler:               "auto_downscale_bilinear",
				Reason:                "auto_interpolate=false_downscale",
				ExperimentalCandidate: experimentalCandidate,
			}
		}
	}

	if !interpolate && !interpolateExplicit && isImageUpscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		if isPopplerHighMagnificationUpscale(srcWidth, srcHeight, dstWidth, dstHeight) {
			return imageSamplingDecision{
				Interpolate:           false,
				Sampler:               "auto_nearest",
				Reason:                "auto_interpolate=false_upscale_400pct_poppler",
				ExperimentalCandidate: experimentalCandidate,
			}
		}
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "auto_upscale_bilinear",
			Reason:                "auto_interpolate=false_upscale",
			ExperimentalCandidate: experimentalCandidate,
		}
	}

	if interpolate {
		return imageSamplingDecision{
			Interpolate:           true,
			Sampler:               "auto_approx_bilinear",
			Reason:                "auto_interpolate=true",
			ExperimentalCandidate: experimentalCandidate,
		}
	}
	return imageSamplingDecision{
		Interpolate:           false,
		Sampler:               "auto_nearest",
		Reason:                "auto_interpolate=false",
		ExperimentalCandidate: experimentalCandidate,
	}
}

func isNearIdentityImageScale(srcWidth, srcHeight int, dstWidth, dstHeight float64) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	scaleX := dstWidth / float64(srcWidth)
	scaleY := dstHeight / float64(srcHeight)
	if scaleX <= 0 || scaleY <= 0 {
		return false
	}
	return math.Abs(scaleX-1.0) <= imageNearIdentityScaleTolerance &&
		math.Abs(scaleY-1.0) <= imageNearIdentityScaleTolerance &&
		(scaleX >= 1.0 || scaleY >= 1.0)
}

func isTinyImageSource(srcWidth, srcHeight int) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	return srcWidth <= 32 && srcHeight <= 32
}

func isImageDownscale(srcWidth, srcHeight int, dstWidth, dstHeight float64) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	return dstWidth <= float64(srcWidth) && dstHeight <= float64(srcHeight)
}

func isImageUpscale(srcWidth, srcHeight int, dstWidth, dstHeight float64) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	return dstWidth > float64(srcWidth) || dstHeight > float64(srcHeight)
}

func isPopplerHighMagnificationUpscale(srcWidth, srcHeight int, dstWidth, dstHeight float64) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	// Poppler Splash disables interpolation for /Interpolate false when either
	// integer scaled dimension is at least 400% of the source dimension
	// (Splash.cc isImageInterpolationRequired).
	scaledWidth := int(math.Ceil(dstWidth))
	scaledHeight := int(math.Ceil(dstHeight))
	return scaledWidth/srcWidth >= 4 || scaledHeight/srcHeight >= 4
}

func isImageStrictDownscale(srcWidth, srcHeight int, dstWidth, dstHeight float64) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	return dstWidth < float64(srcWidth) || dstHeight < float64(srcHeight)
}

func isExperimentalSplashScaleOnlyCandidate(
	interpolateExplicit bool,
	colorSpace string,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
) bool {
	classification := classifyExperimentalSplashScaleOnlyCandidate(
		interpolateExplicit,
		colorSpace,
		srcWidth,
		srcHeight,
		dstWidth,
		dstHeight,
	)
	return classification == "candidate_small_gray_or_indexed_non_downscale"
}

func classifyExperimentalSplashScaleOnlyCandidate(
	interpolateExplicit bool,
	colorSpace string,
	srcWidth int,
	srcHeight int,
	dstWidth float64,
	dstHeight float64,
) string {
	if interpolateExplicit {
		return "rejected_interpolate_explicit"
	}
	if srcWidth <= 0 || srcHeight <= 0 {
		return "rejected_invalid_source"
	}
	if !(strings.EqualFold(colorSpace, "DeviceGray") || strings.EqualFold(colorSpace, "Indexed")) {
		return "rejected_colorspace"
	}
	if srcWidth > 32 || srcHeight > 32 {
		return "rejected_large_source"
	}
	if isImageStrictDownscale(srcWidth, srcHeight, dstWidth, dstHeight) {
		return "rejected_strict_downscale"
	}
	return "candidate_small_gray_or_indexed_non_downscale"
}

func isStrongImageDownscale(srcWidth, srcHeight int, dstWidth, dstHeight float64) bool {
	if srcWidth <= 0 || srcHeight <= 0 {
		return false
	}
	if dstWidth <= 0 || dstHeight <= 0 {
		return false
	}
	dsx := dstWidth / float64(srcWidth)
	dsy := dstHeight / float64(srcHeight)
	if dsx >= 1.0 || dsy >= 1.0 {
		return false
	}
	return math.Min(dsx, dsy) <= 0.75
}

func normalizeImageSamplingMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "default", ImageSamplingModeLegacy:
		return ImageSamplingModeLegacy
	case ImageSamplingModeAdaptiveDCTICCBasedV1:
		return ImageSamplingModeAdaptiveDCTICCBasedV1
	case ImageSamplingModeExperimentalSplashScaleOnlyV1:
		return ImageSamplingModeExperimentalSplashScaleOnlyV1
	case ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1:
		return ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1
	case ImageSamplingModeExperimentalIndexedCMYKSimpleV1:
		return ImageSamplingModeExperimentalIndexedCMYKSimpleV1
	case ImageSamplingModeExperimentalIndexedCMYKHybrid75V1:
		return ImageSamplingModeExperimentalIndexedCMYKHybrid75V1
	case ImageSamplingModeExperimentalIndexedCMYKStdlibV1:
		return ImageSamplingModeExperimentalIndexedCMYKStdlibV1
	case ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1:
		return ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1
	case ImageSamplingModeExperimentalDCTGrayIgnoreICCV1:
		return ImageSamplingModeExperimentalDCTGrayIgnoreICCV1
	case ImageSamplingModeExperimentalIndexedCMYKLUTV1:
		return ImageSamplingModeExperimentalIndexedCMYKLUTV1
	default:
		return ImageSamplingModeLegacy
	}
}

func (e *Evaluator) resolveImageCMYKConversionMode(colorSpace, indexedBase string, srcWidth, srcHeight int, dstWidth, dstHeight float64) string {
	if !usesCMYKImageConversion(colorSpace, indexedBase) {
		return domainimage.CMYKConversionModeDefault
	}

	return resolveExperimentalIndexedCMYKConversionMode(
		e.imageSamplingMode,
		classifyExperimentalIndexedCMYKCandidate(
			colorSpace,
			indexedBase,
			srcWidth,
			srcHeight,
			dstWidth,
			dstHeight,
		),
	)
}

func (e *Evaluator) resolveImageEdgeMode(colorSpace string, srcWidth, srcHeight int, ctm [6]float64, dstWidth, dstHeight float64) string {
	candidate := classifyExperimentalRGBEdgeCandidate(
		colorSpace,
		srcWidth,
		srcHeight,
		dstWidth,
		dstHeight,
		ctm,
	)
	return resolveExperimentalImageEdgeMode(
		e.imageSamplingMode,
		candidate,
		srcWidth,
		srcHeight,
		dstWidth,
		dstHeight,
	)
}

func resolveImageInterpolate(obj entity.Object, defaultValue bool) bool {
	switch v := obj.(type) {
	case *entity.Boolean:
		return v.Value()
	case *entity.Integer:
		return v.Value() != 0
	default:
		return defaultValue
	}
}

func resolveImageInterpolateOption(obj entity.Object, defaultValue bool) (bool, bool) {
	if obj == nil {
		return defaultValue, false
	}
	return resolveImageInterpolate(obj, defaultValue), true
}
