// Package renderer provides PDF content stream evaluation and rendering.
//
//revive:disable:exported
//nolint:errcheck,govet,ineffassign
package renderer

import (
	"fmt"
	stdimage "image"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image"
)

const (
	pdfNameAIS              entity.Name = "/AIS"
	pdfNameBC               entity.Name = "/BC"
	pdfNameBBox             entity.Name = "/BBox"
	pdfNameBaseEncoding     entity.Name = "/BaseEncoding"
	pdfNameBaseFont         entity.Name = "/BaseFont"
	pdfNameBitsPerComponent entity.Name = "/BitsPerComponent"
	pdfNameBM               entity.Name = "/BM"
	pdfNameBPC              entity.Name = "/BPC"
	pdfNameCA               entity.Name = "/CA"
	pdfNameCharProcs        entity.Name = "/CharProcs"
	pdfNameCIDToGIDMap      entity.Name = "/CIDToGIDMap"
	pdfNameCS               entity.Name = "/CS"
	pdfNameColorSpace       entity.Name = "/ColorSpace"
	pdfNameD                entity.Name = "/D"
	pdfNameDecode           entity.Name = "/Decode"
	pdfNameDP               entity.Name = "/DP"
	pdfNameDecodeParms      entity.Name = "/DecodeParms"
	pdfNameDescendantFonts  entity.Name = "/DescendantFonts"
	pdfNameDifferences      entity.Name = "/Differences"
	pdfNameDW               entity.Name = "/DW"
	pdfNameEncoding         entity.Name = "/Encoding"
	pdfNameExtGState        entity.Name = "/ExtGState"
	pdfNameF                entity.Name = "/F"
	pdfNameFilter           entity.Name = "/Filter"
	pdfNameFirstChar        entity.Name = "/FirstChar"
	pdfNameFont             entity.Name = "/Font"
	pdfNameFontBBox         entity.Name = "/FontBBox"
	pdfNameFontDescriptor   entity.Name = "/FontDescriptor"
	pdfNameFontFile         entity.Name = "/FontFile"
	pdfNameFontFile2        entity.Name = "/FontFile2"
	pdfNameFontFile3        entity.Name = "/FontFile3"
	pdfNameFontMatrix       entity.Name = "/FontMatrix"
	pdfNameG                entity.Name = "/G"
	pdfNameH                entity.Name = "/H"
	pdfNameHeight           entity.Name = "/Height"
	pdfNameGroup            entity.Name = "/Group"
	pdfNameI                entity.Name = "/I"
	pdfNameIM               entity.Name = "/IM"
	pdfNameImageMask        entity.Name = "/ImageMask"
	pdfNameInterpolate      entity.Name = "/Interpolate"
	pdfNameJBIG2Globals     entity.Name = "/JBIG2Globals"
	pdfNameK                entity.Name = "/K"
	pdfNameLC               entity.Name = "/LC"
	pdfNameLJ               entity.Name = "/LJ"
	pdfNameLW               entity.Name = "/LW"
	pdfNameLastChar         entity.Name = "/LastChar"
	pdfNameML               entity.Name = "/ML"
	pdfNameMask             entity.Name = "/Mask"
	pdfNameMatte            entity.Name = "/Matte"
	pdfNameMatrix           entity.Name = "/Matrix"
	pdfNameN                entity.Name = "/N"
	pdfNamePaintType        entity.Name = "/PaintType"
	pdfNamePattern          entity.Name = "/Pattern"
	pdfNamePatternType      entity.Name = "/PatternType"
	pdfNameResources        entity.Name = "/Resources"
	pdfNameS                entity.Name = "/S"
	pdfNameSA               entity.Name = "/SA"
	pdfNameSMask            entity.Name = "/SMask"
	pdfNameShading          entity.Name = "/Shading"
	pdfNameShadingType      entity.Name = "/ShadingType"
	pdfNameSubtype          entity.Name = "/Subtype"
	pdfNameTilingType       entity.Name = "/TilingType"
	pdfNameToUnicode        entity.Name = "/ToUnicode"
	pdfNameTR               entity.Name = "/TR"
	pdfNameTR2              entity.Name = "/TR2"
	pdfNameType             entity.Name = "/Type"
	pdfNameW                entity.Name = "/W"
	pdfNameWidth            entity.Name = "/Width"
	pdfNameWidths           entity.Name = "/Widths"
	pdfNameXObject          entity.Name = "/XObject"
	pdfNameXStep            entity.Name = "/XStep"
	pdfNameYStep            entity.Name = "/YStep"
	pdfNameca               entity.Name = "/ca"
)

func (e *Evaluator) evaluateImageXObject(xobj *entity.Stream, name entity.Name) error {
	dict := xobj.Dict()
	// Get image dimensions
	widthVal := dict.Get(pdfNameWidth)
	if widthVal == nil {
		return fmt.Errorf("image %s has no width", name)
	}
	width, err := getNumberOperand(widthVal)
	if err != nil {
		return fmt.Errorf("image %s: invalid width: %w", name, err)
	}

	heightVal := dict.Get(pdfNameHeight)
	if heightVal == nil {
		return fmt.Errorf("image %s has no height", name)
	}
	height, err := getNumberOperand(heightVal)
	if err != nil {
		return fmt.Errorf("image %s: invalid height: %w", name, err)
	}

	filterObj := dict.Get(pdfNameFilter)
	imageFilter, useEncodedData := resolveXObjectImageFilter(filterObj)
	encodedPrefixLen := 0
	if encodedFilter, prefixLen, ok := resolveXObjectEncodedFilterPipeline(filterObj); ok {
		imageFilter = encodedFilter
		useEncodedData = true
		encodedPrefixLen = prefixLen
	}

	colorSpaceObj := dict.Get(pdfNameColorSpace)
	imageMask := isImageMaskDict(dict)
	// Poppler treats a missing ImageMask BPC as 1, and accepts the inline-style
	// /BPC alias as a fallback (Gfx.cc image bit-depth parsing).
	bpcObj := dict.GetTry(pdfNameBitsPerComponent, pdfNameBPC)
	bpc := getImageBitsPerComponent(bpcObj)
	if imageMask && bpcObj == nil {
		bpc = 1
	}
	rawDecodeSizeHint := e.imageXObjectDecodeSizeHint(colorSpaceObj, width, height, bpc, imageMask)

	var data []byte
	if useEncodedData {
		// For image masks we must decode to raw pixel bits before applying the mask.
		if imageMask {
			decoded, decodeErr := e.decodeEntityStreamWithSizeHint(xobj, rawDecodeSizeHint)
			if decodeErr == nil {
				data = decoded
			} else {
				data = xobj.RawBytes()
			}
		} else {
			// JPEG/JPX/JBIG2 data must stay encoded for image decoder plugins.
			// If generic filters precede the encoded image filter, decode only
			// that prefix and leave the final encoded image bytes intact.
			decodedPrefix, decodeErr := decodeImageEncodedFilterPrefix(xobj, encodedPrefixLen)
			if decodeErr != nil {
				return errors.Invalid("decode_image_filter_prefix", decodeErr)
			}
			data = decodedPrefix
		}
	} else {
		// Decode generic stream filters (Flate/LZW/ASCII/CCITT...) through stream layer.
		data, err = e.decodeEntityStreamWithSizeHint(xobj, rawDecodeSizeHint)
		if err != nil {
			return errors.Invalid("decode_image_xobject", err)
		}
		imageFilter = domainimage.FilterNone
	}

	sourceICCBased := e.isICCBasedColorSpace(colorSpaceObj)
	var iccProfile []byte
	iccComponents := 0
	if sourceICCBased {
		iccProfile, _ = e.resolveICCBasedProfile(colorSpaceObj, 0)
		iccComponents = e.resolveICCBasedComponentCount(colorSpaceObj)
	}

	if shouldSkipAllImagesForDebug() {
		return nil
	}
	if imageMask {
		decode := e.resolveImageDecodeArray(dict.Get(pdfNameDecode))
		paintBitOne := resolveImageMaskPaintBit(decode)
		interpolate, interpolateExplicit := resolveImageInterpolateOption(dict.Get(pdfNameInterpolate), false)
		sourceFilter := resolveXObjectImageSourceFilter(dict.Get(pdfNameFilter))
		if err := e.renderImageMaskToCanvas(
			data,
			width,
			height,
			bpc,
			sourceFilter,
			paintBitOne,
			interpolate,
			interpolateExplicit,
		); err != nil {
			e.renderPlaceholderImage(width, height)
		}
		return nil
	}

	// Resolve color space for raw image decoding.
	colorSpace, ok := e.resolveImageColorSpace(colorSpaceObj)
	if !ok {
		if debugImageDecodeErrors() {
			fmt.Fprintf(os.Stderr, "PDF_IMAGE_SKIP name=%s reason=colorspace raw_cs=%T:%v filter=%v w=%.0f h=%.0f bpc=%d data=%d\n",
				name, colorSpaceObj, colorSpaceObj, filterObj, width, height, bpc, len(data))
		}
		// Unsupported color space: skip image rendering for now.
		return nil
	}
	colorMapper, ok := e.resolveImageColorMapper(colorSpace, colorSpaceObj)
	if !ok {
		if debugImageDecodeErrors() {
			fmt.Fprintf(os.Stderr, "PDF_IMAGE_SKIP name=%s reason=colorspace_mapper cs=%s raw_cs=%T:%v filter=%v w=%.0f h=%.0f bpc=%d data=%d\n",
				name, colorSpace, colorSpaceObj, colorSpaceObj, filterObj, width, height, bpc, len(data))
		}
		return nil
	}

	indexedBase := ""
	var indexedLookup []byte
	if colorSpace == "Indexed" {
		base, lookup, indexedOK := e.resolveIndexedColorSpace(colorSpaceObj, 0)
		if !indexedOK {
			if debugImageDecodeErrors() {
				fmt.Fprintf(os.Stderr, "PDF_IMAGE_SKIP name=%s reason=indexed raw_cs=%T:%v filter=%v w=%.0f h=%.0f bpc=%d data=%d\n",
					name, colorSpaceObj, colorSpaceObj, filterObj, width, height, bpc, len(data))
			}
			return nil
		}
		indexedBase = base
		indexedLookup = lookup
	}

	// If canvas is set, render the image
	if e.canvas != nil {
		decode := e.resolveImageDecodeArray(dict.Get(pdfNameDecode))
		interpolate, interpolateExplicit := resolveImageInterpolateOption(dict.Get(pdfNameInterpolate), false)
		smaskObj := dict.Get(pdfNameSMask)
		softMask := e.resolveSoftMaskDetails(smaskObj)
		mask := softMask.mask
		maskMatte := softMask.matte
		explicitMask := false
		maskInterpolate := false
		if mask != nil {
			maskInterpolate = e.resolveImageMaskInterpolate(smaskObj)
		}
		if mask == nil {
			maskObj := dict.Get(pdfNameMask)
			if explicit := e.resolveExplicitImageMask(maskObj); explicit != nil {
				mask = explicit
				explicitMask = true
				maskInterpolate = e.resolveImageMaskInterpolate(maskObj)
				maskMatte = nil
			} else {
				// A /Mask image stream without ImageMask follows Poppler's soft-mask image path.
				softMask = e.resolveSoftMaskDetails(maskObj)
				mask = softMask.mask
				maskMatte = softMask.matte
				if mask != nil {
					maskInterpolate = e.resolveImageMaskInterpolate(maskObj)
				}
			}
		}
		colorKeyMask := e.resolveColorKeyMask(dict.Get(pdfNameMask), colorSpace)
		if mask != nil {
			// When soft mask is present, favor SMask alpha and ignore color-key masking.
			colorKeyMask = nil
		}
		if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
			fmt.Fprintf(os.Stderr, "IMGDRAW w=%.0f h=%.0f cs=%s filter=%s smask=%t mask=%t matte=%t colorKey=%t fillAlpha=%.4f blend=%s\n",
				width, height, colorSpace, imageFilter, dict.Get(pdfNameSMask) != nil, dict.Get(pdfNameMask) != nil, maskMatte != nil, colorKeyMask != nil,
				e.graphics.fillAlpha, e.graphics.blendMode)
		}
		e.renderImageToCanvas(
			xobj,
			data,
			width,
			height,
			colorSpace,
			colorMapper,
			sourceICCBased,
			iccProfile,
			iccComponents,
			indexedBase,
			indexedLookup,
			bpc,
			imageFilter,
			resolveXObjectImageSourceFilter(dict.Get(pdfNameFilter)),
			e.resolveImageDecodeParms(dict.Get(pdfNameDecodeParms), encodedPrefixLen),
			decode,
			mask,
			maskMatte,
			softMask.stream,
			explicitMask,
			maskInterpolate,
			colorKeyMask,
			interpolate,
			interpolateExplicit,
		)
	}

	return nil
}

func (e *Evaluator) imageXObjectDecodeSizeHint(colorSpaceObj entity.Object, width, height float64, bpc int32, imageMask bool) int {
	w := int(width)
	h := int(height)
	if w <= 0 || h <= 0 || bpc <= 0 {
		return 0
	}

	components := 0
	if imageMask {
		components = 1
	} else if colorSpace, ok := e.resolveImageColorSpace(colorSpaceObj); ok {
		components = imageDecodeSizeHintComponents(colorSpace)
	}
	if components <= 0 {
		return 0
	}

	rowBits := w * components * int(bpc)
	if rowBits <= 0 {
		return 0
	}
	size := ((rowBits + 7) / 8) * h
	const maxDecodeSizeHint = 512 << 20
	if size <= 0 || size > maxDecodeSizeHint {
		return 0
	}
	return size
}

func imageDecodeSizeHintComponents(colorSpace string) int {
	switch colorSpace {
	case "DeviceGray", "Indexed", "Separation":
		return 1
	case "DeviceRGB", "CalRGB":
		return 3
	case "DeviceCMYK":
		return 4
	default:
		return 0
	}
}

func getImageBitsPerComponent(obj entity.Object) int32 {
	const defaultBitsPerComponent = int32(8)

	switch v := obj.(type) {
	case nil:
		return defaultBitsPerComponent
	case *entity.Integer:
		if v.Value() <= 0 || v.Value() > 16 {
			return defaultBitsPerComponent
		}
		return int32(v.Value())
	case *entity.Real:
		bpc := int32(math.Round(v.Value()))
		if bpc <= 0 || bpc > 16 {
			return defaultBitsPerComponent
		}
		return bpc
	case *entity.String:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v.Value()), 64)
		if err != nil {
			return defaultBitsPerComponent
		}
		bpc := int32(math.Round(parsed))
		if bpc <= 0 || bpc > 16 {
			return defaultBitsPerComponent
		}
		return bpc
	default:
		return defaultBitsPerComponent
	}
}

func isImageMaskDictValue(obj entity.Object) bool {
	switch v := obj.(type) {
	case *entity.Boolean:
		return v.Value()
	case *entity.Integer:
		return v.Value() != 0
	default:
		return false
	}
}

func isImageMaskDict(dict *entity.Dict) bool {
	if dict == nil {
		return false
	}
	return isImageMaskDictValue(dict.Get(pdfNameImageMask)) ||
		isImageMaskDictValue(dict.Get(pdfNameIM))
}

func resolveImageMaskPaintBit(decode []float64) bool {
	if len(decode) < 2 {
		// According to PDF image mask semantics, a missing decode array defaults to [1 0].
		// That means bit value 0 is painted (opaque) and bit value 1 is transparent.
		return false
	}
	return decode[0] < decode[1]
}

func (e *Evaluator) resolveImageDecodeParms(obj entity.Object, filterIndex int) map[string]interface{} {
	selected := selectDecodeParmsObject(obj, filterIndex)
	dict, ok := e.resolveDecodeParmsDict(selected, 0)
	if !ok {
		return nil
	}

	params := make(map[string]interface{})
	if globals, ok := e.resolveDecodeParmsBytes(dict.Get(pdfNameJBIG2Globals), 0); ok {
		params["JBIG2Globals"] = globals
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func selectDecodeParmsObject(obj entity.Object, filterIndex int) entity.Object {
	if arr, ok := obj.(*entity.Array); ok {
		if filterIndex < 0 || filterIndex >= arr.Len() {
			return nil
		}
		return arr.Get(filterIndex)
	}
	return obj
}

func (e *Evaluator) resolveDecodeParmsDict(obj entity.Object, depth int) (*entity.Dict, bool) {
	if obj == nil || depth > 8 {
		return nil, false
	}
	switch typed := obj.(type) {
	case *entity.Dict:
		return typed, true
	case entity.Ref:
		if e.xref == nil {
			return nil, false
		}
		resolved, err := e.xref.Fetch(typed)
		if err != nil {
			return nil, false
		}
		return e.resolveDecodeParmsDict(resolved, depth+1)
	default:
		return nil, false
	}
}

func (e *Evaluator) resolveDecodeParmsBytes(obj entity.Object, depth int) ([]byte, bool) {
	if obj == nil || depth > 8 {
		return nil, false
	}
	switch typed := obj.(type) {
	case *entity.Stream:
		decoded, err := e.decodeEntityStream(typed)
		if err == nil {
			return decoded, true
		}
		return typed.RawBytes(), true
	case *entity.String:
		return []byte(typed.Value()), true
	case entity.Ref:
		if e.xref == nil {
			return nil, false
		}
		resolved, err := e.xref.Fetch(typed)
		if err != nil {
			return nil, false
		}
		return e.resolveDecodeParmsBytes(resolved, depth+1)
	default:
		return nil, false
	}
}

// renderImageToCanvas renders an image to the canvas.
func (e *Evaluator) renderImageToCanvas(
	imageStream *entity.Stream,
	data []byte,
	width, height float64,
	colorSpace string,
	colorMapper colorspace.ColorSpace,
	sourceICCBased bool,
	iccProfile []byte,
	iccComponents int,
	indexedBase string,
	indexedLookup []byte,
	bpc int32,
	filter domainimage.ImageFilter,
	sourceFilter domainimage.ImageFilter,
	decodeParms map[string]interface{},
	decode []float64,
	mask domainimage.ImageMask,
	maskMatte []float64,
	maskStream *entity.Stream,
	explicitMask bool,
	maskInterpolate bool,
	colorKeyMask *image.ColorKeyMask,
	interpolate bool,
	interpolateExplicit bool,
) {
	if e.canvas == nil {
		return
	}

	// Create image data structure
	imageCTM := e.currentImageTransform()
	projectedWidth, projectedHeight := projectedImageDimensions(imageCTM, int(width), int(height))
	cmykExperimentalCandidate := classifyExperimentalIndexedCMYKCandidate(
		colorSpace,
		indexedBase,
		int(width),
		int(height),
		projectedWidth,
		projectedHeight,
	)
	grayICCExperimentalCandidate := classifyExperimentalDCTGrayIgnoreICCCandidate(
		filter,
		colorSpace,
		sourceICCBased,
		int(width),
		int(height),
		projectedWidth,
		projectedHeight,
		imageCTM,
	)
	resolvedICCProfile, resolvedICCComponents, grayICCProfileMode := resolveExperimentalDCTGrayICCProfile(
		e.imageSamplingMode,
		grayICCExperimentalCandidate,
		iccProfile,
		iccComponents,
	)

	imgData := &domainimage.ImageData{
		Data:             data,
		Width:            int(width),
		Height:           int(height),
		BitsPerComponent: int(bpc),
		ColorSpace:       domainimage.ColorSpace(colorSpace),
		ColorMapper:      colorMapper,
		CMYKConversionMode: e.resolveImageCMYKConversionMode(
			colorSpace,
			indexedBase,
			int(width),
			int(height),
			projectedWidth,
			projectedHeight,
		),
		ImageEdgeMode: e.resolveImageEdgeMode(
			colorSpace,
			int(width),
			int(height),
			imageCTM,
			projectedWidth,
			projectedHeight,
		),
		ICCProfile:    resolvedICCProfile,
		ICCComponents: resolvedICCComponents,
		IndexedBase:   domainimage.ColorSpace(indexedBase),
		IndexedLookup: indexedLookup,
		Filter:        filter,
		DecodeParms:   decodeParms,
		Decode:        decode,
		Mask:          mask,
	}

	softMask := domainimage.ImageMask(nil)
	explicitImageMask := false
	var err error
	img, fastRawRGB := newPackedRawRGBImageForCanvas(data, imgData, sourceICCBased, colorKeyMask, maskMatte)
	if !fastRawRGB && mask != nil && !explicitMask && len(maskMatte) > 0 && canvasSupportsSoftMaskedImages(e.canvas) {
		img, fastRawRGB = e.packedRawRGBMatteImageForCanvas(
			imageStream,
			maskStream,
			data,
			imgData,
			mask,
			maskMatte,
			colorSpace,
			colorMapper,
			sourceICCBased,
			colorKeyMask,
		)
		if fastRawRGB {
			softMask = mask
		}
	}
	if !fastRawRGB {
		// Decode the image
		decodedImg, decodeErr := e.decodeImageData(imgData)
		if decodeErr != nil {
			if debugImageDecodeErrors() {
				fmt.Fprintf(os.Stderr, "PDF_IMAGE_DECODE_ERROR cs=%s indexed_base=%s filter=%s source_filter=%s w=%d h=%d bpc=%d data=%d lookup=%d decode=%v decode_parms=%v err=%v\n",
					colorSpace, indexedBase, filter, sourceFilter, imgData.Width, imgData.Height, imgData.BitsPerComponent, len(data), len(indexedLookup), decode, decodeParms, decodeErr)
			}
			// If decoding fails, fall back to placeholder
			e.renderPlaceholderImage(width, height)
			return
		}

		// Convert decoded domain image to std image for canvas drawing.
		img = decodedImg.Image()
		if img == nil {
			if debugImageDecodeErrors() {
				fmt.Fprintf(os.Stderr, "PDF_IMAGE_DECODE_ERROR cs=%s indexed_base=%s filter=%s w=%d h=%d bpc=%d data=%d err=nil-image\n",
					colorSpace, indexedBase, filter, imgData.Width, imgData.Height, imgData.BitsPerComponent, len(data))
			}
			e.renderPlaceholderImage(width, height)
			return
		}
		if decodedImg.HasMask() {
			softMask = decodedImg.Mask()
			explicitImageMask = explicitMask
			if !explicitImageMask && len(maskMatte) > 0 {
				img = applySoftMaskMatteUnblend(img, softMask, maskMatte, colorSpace, colorMapper)
			}
			if explicitImageMask {
				if _, ok := e.canvas.(sourceAlphaImageDrawer); !ok {
					img = image.ApplyMask(img, softMask)
					softMask = nil
					explicitImageMask = false
				}
			} else if _, ok := e.canvas.(softMaskedImageDrawer); !ok {
				img = image.ApplyMask(img, softMask)
				softMask = nil
			}
		}
		if colorKeyMask != nil {
			if masked, err := image.ApplyColorKeyMask(img, colorKeyMask); err == nil {
				img = masked
			}
		}
	}

	srcBounds := img.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()
	if srcWidth <= 0 || srcHeight <= 0 {
		e.renderPlaceholderImage(width, height)
		return
	}

	// Compute effective destination size in device space (before sampling policy / trace).
	x, y := transformPointWithMatrix(imageCTM, 0, 0)
	imgWidth, imgHeight := projectedImageDimensions(imageCTM, srcWidth, srcHeight)
	decision := chooseImageSamplingPolicy(
		e.imageSamplingMode,
		interpolate,
		interpolateExplicit,
		sourceFilter,
		colorSpace,
		sourceICCBased,
		img.Bounds().Dx(),
		img.Bounds().Dy(),
		imgWidth,
		imgHeight,
	)
	indexedGrayExperimentalCandidate := classifyExperimentalIndexedGrayOriginCandidate(
		colorSpace,
		indexedBase,
		img.Bounds().Dx(),
		img.Bounds().Dy(),
		imgWidth,
		imgHeight,
		imageCTM,
	)
	effectiveInterpolate, sampler := decision.Interpolate, decision.Sampler
	reason := decision.Reason
	sampler, reason = resolveSelectiveIndexedGrayOriginDownscaleSampler(
		e.imageSamplingMode,
		indexedGrayExperimentalCandidate,
		sampler,
		reason,
	)
	edgeExperimentalCandidate := classifyExperimentalRGBEdgeCandidate(
		colorSpace,
		img.Bounds().Dx(),
		img.Bounds().Dy(),
		imgWidth,
		imgHeight,
		imageCTM,
	)
	sampler, reason = resolveSelectiveRGBEdgeScaleThenFlipSampler(
		e.imageSamplingMode,
		edgeExperimentalCandidate,
		sourceFilter,
		colorSpace,
		img.Bounds().Dx(),
		img.Bounds().Dy(),
		imgWidth,
		imgHeight,
		sampler,
		reason,
	)
	phaseX, phaseY := imageSamplingPhase(sampler, reason, effectiveInterpolate, imageCTM)
	e.emitImageSamplingTrace(
		sourceFilter,
		colorSpace,
		indexedBase,
		len(indexedLookup),
		imgData.CMYKConversionMode,
		imgData.ImageEdgeMode,
		indexedGrayExperimentalCandidate,
		cmykExperimentalCandidate,
		edgeExperimentalCandidate,
		grayICCExperimentalCandidate,
		grayICCProfileMode,
		sampler,
		reason,
		decision.ExperimentalCandidate,
		imageCTM,
		phaseX,
		phaseY,
		x,
		y,
		imgWidth,
		imgHeight,
		img,
	)

	// Draw the image to the canvas
	if softMask != nil {
		if explicitImageMask && canApplyExplicitImageMaskAtSourceResolution(img, softMask) {
			err = e.drawSourceAlphaImageUsingCurrentTransform(
				applyExplicitImageMaskToImage(img, softMask),
				imageCTM,
				effectiveInterpolate,
				sampler,
				phaseX,
				phaseY,
				imgData.ImageEdgeMode,
			)
		} else {
			err = e.drawSoftMaskedImageUsingCurrentTransform(
				img,
				softMask,
				imageCTM,
				effectiveInterpolate,
				maskInterpolate,
				sampler,
				phaseX,
				phaseY,
				imgData.ImageEdgeMode,
			)
		}
	} else {
		err = e.drawImageUsingCurrentTransform(
			img,
			imageCTM,
			effectiveInterpolate,
			sampler,
			phaseX,
			phaseY,
			imgData.ImageEdgeMode,
		)
	}
	if err != nil {
		if debugImageDecodeErrors() {
			fmt.Fprintf(os.Stderr, "PDF_IMAGE_DRAW_ERROR cs=%s indexed_base=%s filter=%s source_filter=%s w=%d h=%d bpc=%d img=%dx%d ctm=%v err=%v\n",
				colorSpace, indexedBase, filter, sourceFilter, imgData.Width, imgData.Height, imgData.BitsPerComponent, srcWidth, srcHeight, imageCTM, err)
		}
		// Fallback to placeholder if drawing fails
		e.renderPlaceholderImage(width, height)
	}
}

func canvasSupportsSoftMaskedImages(canvas interface{}) bool {
	if canvas == nil {
		return false
	}
	if _, ok := canvas.(softMaskedImageDrawerWithMaskInterpolation); ok {
		return true
	}
	_, ok := canvas.(softMaskedImageDrawer)
	return ok
}

func newPackedRawRGBImageForCanvas(
	data []byte,
	imgData *domainimage.ImageData,
	sourceICCBased bool,
	colorKeyMask *image.ColorKeyMask,
	maskMatte []float64,
) (stdimage.Image, bool) {
	if imgData == nil ||
		imgData.Filter != domainimage.FilterNone ||
		imgData.ColorSpace != domainimage.ColorSpaceDeviceRGB ||
		imgData.BitsPerComponent != 8 ||
		sourceICCBased ||
		len(imgData.ICCProfile) != 0 ||
		imgData.ICCComponents != 0 ||
		imgData.IndexedBase != "" ||
		len(imgData.IndexedLookup) != 0 ||
		colorKeyMask != nil ||
		len(maskMatte) != 0 ||
		imgData.Mask != nil ||
		!isDefaultRawRGBDecodeArray(imgData.Decode) {
		return nil, false
	}
	if imgData.Width <= 0 || imgData.Height <= 0 {
		return nil, false
	}
	stride := imgData.Width * 3
	needed := stride * imgData.Height
	if needed <= 0 || len(data) < needed {
		return nil, false
	}
	return image.NewRGBImage(stdimage.Rect(0, 0, imgData.Width, imgData.Height), data[:needed], stride), true
}

type packedRawRGBMatteImageCacheKey struct {
	image *entity.Stream
	mask  *entity.Stream
}

func (e *Evaluator) packedRawRGBMatteImageForCanvas(
	imageStream *entity.Stream,
	maskStream *entity.Stream,
	data []byte,
	imgData *domainimage.ImageData,
	mask domainimage.ImageMask,
	maskMatte []float64,
	colorSpace string,
	colorMapper colorspace.ColorSpace,
	sourceICCBased bool,
	colorKeyMask *image.ColorKeyMask,
) (stdimage.Image, bool) {
	if e != nil && imageStream != nil && maskStream != nil {
		key := packedRawRGBMatteImageCacheKey{image: imageStream, mask: maskStream}
		if e.rawRGBMatteCache != nil {
			if img, ok := e.rawRGBMatteCache[key]; ok {
				return img, true
			}
		}
		img, ok := newPackedRawRGBMatteImageForCanvas(
			data,
			imgData,
			mask,
			maskMatte,
			colorSpace,
			colorMapper,
			sourceICCBased,
			colorKeyMask,
		)
		if ok {
			e.ensureRawRGBMatteCache()[key] = img
			return img, true
		}
		return img, ok
	}
	return newPackedRawRGBMatteImageForCanvas(
		data,
		imgData,
		mask,
		maskMatte,
		colorSpace,
		colorMapper,
		sourceICCBased,
		colorKeyMask,
	)
}

func newPackedRawRGBMatteImageForCanvas(
	data []byte,
	imgData *domainimage.ImageData,
	mask domainimage.ImageMask,
	maskMatte []float64,
	colorSpace string,
	colorMapper colorspace.ColorSpace,
	sourceICCBased bool,
	colorKeyMask *image.ColorKeyMask,
) (stdimage.Image, bool) {
	if imgData == nil ||
		imgData.Filter != domainimage.FilterNone ||
		imgData.ColorSpace != domainimage.ColorSpaceDeviceRGB ||
		imgData.BitsPerComponent != 8 ||
		sourceICCBased ||
		len(imgData.ICCProfile) != 0 ||
		imgData.ICCComponents != 0 ||
		imgData.IndexedBase != "" ||
		len(imgData.IndexedLookup) != 0 ||
		colorKeyMask != nil ||
		mask == nil ||
		mask.Image() == nil ||
		len(maskMatte) == 0 ||
		!isDefaultRawRGBDecodeArray(imgData.Decode) {
		return nil, false
	}
	if imgData.Width <= 0 || imgData.Height <= 0 {
		return nil, false
	}
	maskImg := mask.Image()
	maskBounds := maskImg.Bounds()
	if maskBounds.Dx() != imgData.Width || maskBounds.Dy() != imgData.Height {
		return nil, false
	}
	matteRGB, ok := softMaskMatteRGB(maskMatte, colorSpace, colorMapper)
	if !ok {
		return nil, false
	}
	stride := imgData.Width * 3
	needed := stride * imgData.Height
	if needed <= 0 || len(data) < needed {
		return nil, false
	}

	return &packedRawRGBMatteImage{
		pix:      data[:needed],
		stride:   stride,
		rect:     stdimage.Rect(0, 0, imgData.Width, imgData.Height),
		mask:     maskImg,
		maskRect: maskBounds,
		matteRGB: matteRGB,
	}, true
}

type packedRawRGBMatteImage struct {
	pix      []byte
	stride   int
	rect     stdimage.Rectangle
	mask     stdimage.Image
	maskRect stdimage.Rectangle
	matteRGB [3]uint8
}

func (i *packedRawRGBMatteImage) ColorModel() color.Model {
	return color.RGBAModel
}

func (i *packedRawRGBMatteImage) Bounds() stdimage.Rectangle {
	if i == nil {
		return stdimage.Rectangle{}
	}
	return i.rect
}

func (i *packedRawRGBMatteImage) At(x, y int) color.Color {
	if i == nil || !stdimage.Pt(x, y).In(i.rect) {
		return color.RGBA{}
	}
	srcOff := (y-i.rect.Min.Y)*i.stride + (x-i.rect.Min.X)*3
	if srcOff < 0 || srcOff+2 >= len(i.pix) {
		return color.RGBA{}
	}
	alpha := getGrayVal(i.mask, i.maskRect.Min.X+x-i.rect.Min.X, i.maskRect.Min.Y+y-i.rect.Min.Y)
	return color.RGBA{
		R: unblendMatteComponent(i.pix[srcOff], i.matteRGB[0], alpha),
		G: unblendMatteComponent(i.pix[srcOff+1], i.matteRGB[1], alpha),
		B: unblendMatteComponent(i.pix[srcOff+2], i.matteRGB[2], alpha),
		A: 0xff,
	}
}

// RGB8Row writes a top-down unblended RGB row without materializing a full copy.
func (i *packedRawRGBMatteImage) RGB8Row(y int, dst []byte) bool {
	if i == nil || y < i.rect.Min.Y || y >= i.rect.Max.Y {
		return false
	}
	width := i.rect.Dx()
	if width <= 0 || len(dst) < width*3 {
		return false
	}
	srcOff := (y - i.rect.Min.Y) * i.stride
	if srcOff < 0 || srcOff+width*3 > len(i.pix) {
		return false
	}
	maskY := i.maskRect.Min.Y + (y - i.rect.Min.Y)
	if gray, ok := i.mask.(*stdimage.Gray); ok {
		maskOff := (maskY-gray.Rect.Min.Y)*gray.Stride + (i.maskRect.Min.X - gray.Rect.Min.X)
		if maskOff >= 0 && maskOff+width <= len(gray.Pix) {
			for x := 0; x < width; x++ {
				src := srcOff + x*3
				alpha := gray.Pix[maskOff+x]
				dst[x*3] = unblendMatteComponent(i.pix[src], i.matteRGB[0], alpha)
				dst[x*3+1] = unblendMatteComponent(i.pix[src+1], i.matteRGB[1], alpha)
				dst[x*3+2] = unblendMatteComponent(i.pix[src+2], i.matteRGB[2], alpha)
			}
			return true
		}
	}
	if alphaImg, ok := i.mask.(*stdimage.Alpha); ok {
		maskOff := (maskY-alphaImg.Rect.Min.Y)*alphaImg.Stride + (i.maskRect.Min.X - alphaImg.Rect.Min.X)
		if maskOff >= 0 && maskOff+width <= len(alphaImg.Pix) {
			for x := 0; x < width; x++ {
				src := srcOff + x*3
				alpha := alphaImg.Pix[maskOff+x]
				dst[x*3] = unblendMatteComponent(i.pix[src], i.matteRGB[0], alpha)
				dst[x*3+1] = unblendMatteComponent(i.pix[src+1], i.matteRGB[1], alpha)
				dst[x*3+2] = unblendMatteComponent(i.pix[src+2], i.matteRGB[2], alpha)
			}
			return true
		}
	}
	for x := 0; x < width; x++ {
		src := srcOff + x*3
		alpha := getGrayVal(i.mask, i.maskRect.Min.X+x, maskY)
		dst[x*3] = unblendMatteComponent(i.pix[src], i.matteRGB[0], alpha)
		dst[x*3+1] = unblendMatteComponent(i.pix[src+1], i.matteRGB[1], alpha)
		dst[x*3+2] = unblendMatteComponent(i.pix[src+2], i.matteRGB[2], alpha)
	}
	return true
}

func isDefaultRawRGBDecodeArray(decode []float64) bool {
	if len(decode) == 0 {
		return true
	}
	if len(decode) != 6 {
		return false
	}
	for i := 0; i < 3; i++ {
		if decode[i*2] != 0 || decode[i*2+1] != 1 {
			return false
		}
	}
	return true
}

func debugImageDecodeErrors() bool {
	return os.Getenv("PDF_DEBUG_IMAGE_DECODE_ERRORS") == "1"
}

func canApplyExplicitImageMaskAtSourceResolution(img stdimage.Image, mask domainimage.ImageMask) bool {
	if img == nil || mask == nil || mask.Image() == nil {
		return false
	}
	imgBounds := img.Bounds()
	maskBounds := mask.Image().Bounds()
	return maskBounds.Dx() > 0 &&
		maskBounds.Dy() > 0 &&
		maskBounds.Dx() <= imgBounds.Dx() &&
		maskBounds.Dy() <= imgBounds.Dy()
}

func getGrayVal(img stdimage.Image, x, y int) byte {
	switch im := img.(type) {
	case *stdimage.Gray:
		if stdimage.Pt(x, y).In(im.Rect) {
			return im.Pix[im.PixOffset(x, y)]
		}
	case *stdimage.Alpha:
		if stdimage.Pt(x, y).In(im.Rect) {
			return im.Pix[im.PixOffset(x, y)]
		}
	case *stdimage.RGBA:
		if stdimage.Pt(x, y).In(im.Rect) {
			off := im.PixOffset(x, y)
			r := uint32(im.Pix[off])
			r |= r << 8
			g := uint32(im.Pix[off+1])
			g |= g << 8
			b := uint32(im.Pix[off+2])
			b |= b << 8
			yVal := (299*r + 587*g + 114*b + 500) / 1000
			return byte(yVal >> 8)
		}
	case *stdimage.NRGBA:
		if stdimage.Pt(x, y).In(im.Rect) {
			off := im.PixOffset(x, y)
			r := uint32(im.Pix[off])
			r |= r << 8
			g := uint32(im.Pix[off+1])
			g |= g << 8
			b := uint32(im.Pix[off+2])
			b |= b << 8
			yVal := (299*r + 587*g + 114*b + 500) / 1000
			return byte(yVal >> 8)
		}
	}
	return color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y
}

func getRGBAPixel(img stdimage.Image, x, y int) (uint32, uint32, uint32, uint32) {
	switch im := img.(type) {
	case *stdimage.RGBA:
		if stdimage.Pt(x, y).In(im.Rect) {
			off := im.PixOffset(x, y)
			r, g, b, a := im.Pix[off], im.Pix[off+1], im.Pix[off+2], im.Pix[off+3]
			return uint32(r) | uint32(r)<<8, uint32(g) | uint32(g)<<8, uint32(b) | uint32(b)<<8, uint32(a) | uint32(a)<<8
		}
	case *stdimage.NRGBA:
		if stdimage.Pt(x, y).In(im.Rect) {
			off := im.PixOffset(x, y)
			r, g, b, a := im.Pix[off], im.Pix[off+1], im.Pix[off+2], im.Pix[off+3]
			r16 := (uint32(r) | uint32(r)<<8) * uint32(a) / 0xff
			g16 := (uint32(g) | uint32(g)<<8) * uint32(a) / 0xff
			b16 := (uint32(b) | uint32(b)<<8) * uint32(a) / 0xff
			a16 := uint32(a) | uint32(a)<<8
			return r16, g16, b16, a16
		}
	case *stdimage.Gray:
		if stdimage.Pt(x, y).In(im.Rect) {
			gray := uint32(im.Pix[im.PixOffset(x, y)])
			gray16 := gray | gray<<8
			return gray16, gray16, gray16, 0xFFFF
		}
	}
	return img.At(x, y).RGBA()
}

func applyExplicitImageMaskToImage(img stdimage.Image, mask domainimage.ImageMask) stdimage.Image {
	if img == nil || mask == nil || mask.Image() == nil {
		return img
	}
	srcBounds := img.Bounds()
	maskImg := mask.Image()
	maskBounds := maskImg.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	maskW, maskH := maskBounds.Dx(), maskBounds.Dy()
	if srcW <= 0 || srcH <= 0 || maskW <= 0 || maskH <= 0 {
		return img
	}

	out := stdimage.NewNRGBA(srcBounds)
	inverted := mask.IsInverted()
	for y := srcBounds.Min.Y; y < srcBounds.Max.Y; y++ {
		my := maskBounds.Min.Y + (y-srcBounds.Min.Y)*maskH/srcH
		for x := srcBounds.Min.X; x < srcBounds.Max.X; x++ {
			mx := maskBounds.Min.X + (x-srcBounds.Min.X)*maskW/srcW
			alpha := getGrayVal(maskImg, mx, my)
			if inverted {
				alpha = 0xFF - alpha
			}
			r16, g16, b16, a16 := getRGBAPixel(img, x, y)
			baseAlpha := uint8(a16 >> 8)
			finalAlpha := uint8((uint16(baseAlpha)*uint16(alpha) + 127) / 255)
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r16 >> 8),
				G: uint8(g16 >> 8),
				B: uint8(b16 >> 8),
				A: finalAlpha,
			})
		}
	}
	return out
}

func applySoftMaskMatteUnblend(img stdimage.Image, mask domainimage.ImageMask, matte []float64, colorSpace string, colorMapper colorspace.ColorSpace) stdimage.Image {
	if img == nil || mask == nil || mask.Image() == nil || len(matte) == 0 {
		return img
	}
	imgBounds := img.Bounds()
	maskImg := mask.Image()
	maskBounds := maskImg.Bounds()
	if imgBounds.Dx() != maskBounds.Dx() || imgBounds.Dy() != maskBounds.Dy() {
		return img
	}
	matteRGB, ok := softMaskMatteRGB(matte, colorSpace, colorMapper)
	if !ok {
		return img
	}

	if rgba, ok := img.(*stdimage.RGBA); ok {
		for y := imgBounds.Min.Y; y < imgBounds.Max.Y; y++ {
			my := maskBounds.Min.Y + (y - imgBounds.Min.Y)
			for x := imgBounds.Min.X; x < imgBounds.Max.X; x++ {
				mx := maskBounds.Min.X + (x - imgBounds.Min.X)
				alpha := getGrayVal(maskImg, mx, my)
				dst := rgba.PixOffset(x, y)
				rgba.Pix[dst] = unblendMatteComponent(rgba.Pix[dst], matteRGB[0], alpha)
				rgba.Pix[dst+1] = unblendMatteComponent(rgba.Pix[dst+1], matteRGB[1], alpha)
				rgba.Pix[dst+2] = unblendMatteComponent(rgba.Pix[dst+2], matteRGB[2], alpha)
			}
		}
		return rgba
	}

	out := stdimage.NewRGBA(imgBounds)
	for y := imgBounds.Min.Y; y < imgBounds.Max.Y; y++ {
		my := maskBounds.Min.Y + (y - imgBounds.Min.Y)
		for x := imgBounds.Min.X; x < imgBounds.Max.X; x++ {
			mx := maskBounds.Min.X + (x - imgBounds.Min.X)
			alpha := getGrayVal(maskImg, mx, my)
			r16, g16, b16, a16 := getRGBAPixel(img, x, y)
			out.SetRGBA(x, y, color.RGBA{
				R: unblendMatteComponent(uint8(r16>>8), matteRGB[0], alpha),
				G: unblendMatteComponent(uint8(g16>>8), matteRGB[1], alpha),
				B: unblendMatteComponent(uint8(b16>>8), matteRGB[2], alpha),
				A: uint8(a16 >> 8),
			})
		}
	}
	return out
}

func softMaskMatteRGB(matte []float64, colorSpace string, colorMapper colorspace.ColorSpace) ([3]uint8, bool) {
	if colorMapper != nil && len(matte) == colorMapper.GetNumComponents() {
		rgba := colorMapper.ConvertToRGBA(matte)
		return [3]uint8{rgba.R, rgba.G, rgba.B}, true
	}
	switch colorSpace {
	case "DeviceGray":
		if len(matte) != 1 {
			return [3]uint8{}, false
		}
		v := matteComponentByte(matte[0])
		return [3]uint8{v, v, v}, true
	case "DeviceRGB":
		if len(matte) != 3 {
			return [3]uint8{}, false
		}
		return [3]uint8{
			matteComponentByte(matte[0]),
			matteComponentByte(matte[1]),
			matteComponentByte(matte[2]),
		}, true
	default:
		return [3]uint8{}, false
	}
}

func matteComponentByte(v float64) uint8 {
	return uint8(math.Round(clamp(v, 0, 1) * 255))
}

func unblendMatteComponent(src uint8, matte uint8, alpha uint8) uint8 {
	if alpha == 0 {
		return matte
	}
	if alpha == 0xff {
		return src
	}
	value := int(matte) + (int(src)-int(matte))*255/int(alpha)
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

func (e *Evaluator) renderImageMaskToCanvas(
	data []byte,
	width, height float64,
	bpc int32,
	filter domainimage.ImageFilter,
	paintBitOne bool,
	interpolate bool,
	interpolateExplicit bool,
) error {
	if e.canvas == nil {
		return nil
	}

	intWidth := int(width)
	intHeight := int(height)
	if intWidth <= 0 || intHeight <= 0 {
		return errors.Invalid("image_mask_size", nil)
	}

	mask, err := image.DecodeMaskData(data, intWidth, intHeight, int(bpc), paintBitOne)
	if err != nil {
		return errors.Invalid("image_mask_decode", err)
	}

	maskAlphaMode := evaluateImageMaskUniformAlpha(mask)
	if maskAlphaMode == imageMaskAlphaTransparent {
		return nil
	}

	imageCTM := e.currentImageTransform()
	if maskAlphaMode == imageMaskAlphaOpaque &&
		e.canFillImageMaskViaClip(intWidth, intHeight, imageCTM) {
		return e.fillImageMaskWithCurrentClip(intWidth, intHeight, imageCTM)
	}

	if os.Getenv("GO_PDF_ENABLE_SPLASH_IMAGE_MASK_DIRECT") == "1" {
		if drawer, ok := e.canvas.(imageMaskCanvas); ok {
			return e.drawImageMaskUsingCurrentTransform(drawer, mask, imageCTM)
		}
	}

	fill := colorFromGraphicsState(e.graphics.fillColor, e.graphics.fillAlpha)
	fillR, fillG, fillB, fillA := fill.RGBA()
	fillColor := color.RGBA{
		R: uint8(fillR >> 8),
		G: uint8(fillG >> 8),
		B: uint8(fillB >> 8),
		A: uint8(fillA >> 8),
	}

	solid := stdimage.NewRGBA(stdimage.Rect(0, 0, intWidth, intHeight))
	for y := 0; y < intHeight; y++ {
		for x := 0; x < intWidth; x++ {
			solid.SetRGBA(x, y, fillColor)
		}
	}
	img := image.ApplyMask(solid, mask)
	if img == nil {
		return nil
	}

	srcBounds := img.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()
	if srcWidth <= 0 || srcHeight <= 0 {
		return errors.Invalid("image_mask_size", nil)
	}

	x, y := transformPointWithMatrix(imageCTM, 0, 0)
	imgWidth, imgHeight := projectedImageDimensions(imageCTM, srcWidth, srcHeight)
	var effectiveInterpolate bool
	var sampler string
	var reason string
	if interpolate {
		decision := chooseImageSamplingPolicy(
			e.imageSamplingMode,
			interpolate,
			interpolateExplicit,
			filter,
			"DeviceRGB",
			false,
			img.Bounds().Dx(),
			img.Bounds().Dy(),
			imgWidth,
			imgHeight,
		)
		effectiveInterpolate = decision.Interpolate
		sampler = decision.Sampler
		reason = decision.Reason
	} else {
		effectiveInterpolate = false
		sampler = "explicit_nearest"
		reason = "image_mask_nointerpolate"
	}
	phaseX, phaseY := imageSamplingPhase(sampler, reason, effectiveInterpolate, imageCTM)
	edgeExperimentalCandidate := classifyExperimentalRGBEdgeCandidate(
		"DeviceRGB",
		img.Bounds().Dx(),
		img.Bounds().Dy(),
		imgWidth,
		imgHeight,
		imageCTM,
	)
	e.emitImageSamplingTrace(
		filter,
		"DeviceRGB",
		"",
		0,
		"",
		domainimage.ImageEdgeModeDefault,
		"rejected_non_indexed_colorspace",
		"rejected_non_indexed_colorspace",
		edgeExperimentalCandidate,
		"rejected_non_dct_filter",
		"default",
		sampler,
		reason,
		"rejected_colorspace",
		imageCTM,
		phaseX,
		phaseY,
		x,
		y,
		imgWidth,
		imgHeight,
		img,
	)
	return e.drawImageUsingCurrentTransform(
		img,
		imageCTM,
		effectiveInterpolate,
		sampler,
		phaseX,
		phaseY,
		domainimage.ImageEdgeModeDefault,
	)
}

type imageMaskAlphaMode int

const (
	imageMaskAlphaMixed imageMaskAlphaMode = iota
	imageMaskAlphaOpaque
	imageMaskAlphaTransparent
)

func evaluateImageMaskUniformAlpha(mask domainimage.ImageMask) imageMaskAlphaMode {
	if mask == nil {
		return imageMaskAlphaMixed
	}

	maskImg, ok := mask.Image().(*stdimage.Gray)
	if !ok {
		return imageMaskAlphaUniformityFallback(mask)
	}

	b := maskImg.Bounds()
	if b.Empty() {
		return imageMaskAlphaMixed
	}

	inverted := mask.IsInverted()
	first := maskImg.GrayAt(b.Min.X, b.Min.Y).Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := maskImg.GrayAt(x, y).Y
			if v != first {
				return imageMaskAlphaMixed
			}
		}
	}

	alpha := first
	if inverted {
		alpha = 255 - alpha
	}

	switch alpha {
	case 0:
		return imageMaskAlphaTransparent
	case 255:
		return imageMaskAlphaOpaque
	default:
		return imageMaskAlphaMixed
	}
}

func imageMaskAlphaUniformityFallback(mask domainimage.ImageMask) imageMaskAlphaMode {
	b := mask.Image()
	if b == nil || b.Bounds().Empty() {
		return imageMaskAlphaMixed
	}
	ref := b.Bounds().Min
	firstR, firstG, firstB, firstA := b.At(ref.X, ref.Y).RGBA()

	switch im := b.(type) {
	case *stdimage.Gray:
		firstGray := im.Pix[im.PixOffset(ref.X, ref.Y)]
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			off := im.PixOffset(im.Bounds().Min.X, y)
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				if im.Pix[off] != firstGray {
					return imageMaskAlphaMixed
				}
				off++
			}
		}
	case *stdimage.Alpha:
		firstAlpha := im.Pix[im.PixOffset(ref.X, ref.Y)]
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			off := im.PixOffset(im.Bounds().Min.X, y)
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				if im.Pix[off] != firstAlpha {
					return imageMaskAlphaMixed
				}
				off++
			}
		}
	case *stdimage.RGBA:
		firstPixel := im.RGBAAt(ref.X, ref.Y)
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			off := im.PixOffset(im.Bounds().Min.X, y)
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				if im.Pix[off] != firstPixel.R || im.Pix[off+1] != firstPixel.G || im.Pix[off+2] != firstPixel.B || im.Pix[off+3] != firstPixel.A {
					return imageMaskAlphaMixed
				}
				off += 4
			}
		}
	case *stdimage.NRGBA:
		firstPixel := im.NRGBAAt(ref.X, ref.Y)
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			off := im.PixOffset(im.Bounds().Min.X, y)
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				if im.Pix[off] != firstPixel.R || im.Pix[off+1] != firstPixel.G || im.Pix[off+2] != firstPixel.B || im.Pix[off+3] != firstPixel.A {
					return imageMaskAlphaMixed
				}
				off += 4
			}
		}
	default:
		for y := b.Bounds().Min.Y; y < b.Bounds().Max.Y; y++ {
			for x := b.Bounds().Min.X; x < b.Bounds().Max.X; x++ {
				r, g, b1, a := b.At(x, y).RGBA()
				if r != firstR || g != firstG || b1 != firstB || a != firstA {
					return imageMaskAlphaMixed
				}
			}
		}
	}

	inverted := mask.IsInverted()
	alpha := uint8(firstA >> 8)
	if !inverted {
		alpha = uint8(firstA >> 8)
	}
	if inverted {
		alpha = 255 - alpha
	}

	switch alpha {
	case 0:
		return imageMaskAlphaTransparent
	case 255:
		return imageMaskAlphaOpaque
	default:
		return imageMaskAlphaMixed
	}
}

func (e *Evaluator) fillImageMaskWithCurrentClip(width, height int, imageCTM [6]float64) error {
	if e.canvas == nil {
		return nil
	}
	if _, _, _, _, ok := e.currentPathClipBounds(); !ok {
		return nil
	}

	if width <= 0 || height <= 0 {
		return nil
	}

	for _, v := range imageCTM {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
	}

	p00x, p00y := transformPointWithMatrix(imageCTM, 0, 0)
	p10x, p10y := transformPointWithMatrix(imageCTM, 1, 0)
	p11x, p11y := transformPointWithMatrix(imageCTM, 1, 1)
	p01x, p01y := transformPointWithMatrix(imageCTM, 0, 1)

	// Fill only the image bounds in the current clip region. The clip itself is already
	// represented by the active canvas clip path.
	prevPath := e.graphics.path
	rectPath := NewPath()
	rectPath.AddElement(&MoveTo{X: p00x, Y: p00y})
	rectPath.AddElement(&LineTo{X: p10x, Y: p10y})
	rectPath.AddElement(&LineTo{X: p11x, Y: p11y})
	rectPath.AddElement(&LineTo{X: p01x, Y: p01y})
	rectPath.AddElement(&Close{})
	e.graphics.path = rectPath

	e.renderPathToCanvas(true)
	e.graphics.path = prevPath

	return nil
}

func (e *Evaluator) canFillImageMaskViaClip(width, height int, imageCTM [6]float64) bool {
	if e.canvas == nil {
		return false
	}
	cx0, cy0, cx1, cy1, ok := e.currentPathClipBounds()
	if !ok {
		return false
	}

	if width <= 0 || height <= 0 {
		return false
	}

	for _, v := range imageCTM {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}

	x0, y0 := transformPointWithMatrix(imageCTM, 0, 0)
	x1, y1 := transformPointWithMatrix(imageCTM, 1, 0)
	x2, y2 := transformPointWithMatrix(imageCTM, 0, 1)
	x3, y3 := transformPointWithMatrix(imageCTM, 1, 1)

	minX, maxX := math.Min(math.Min(x0, x1), math.Min(x2, x3)), math.Max(math.Max(x0, x1), math.Max(x2, x3))
	minY, maxY := math.Min(math.Min(y0, y1), math.Min(y2, y3)), math.Max(math.Max(y0, y1), math.Max(y2, y3))

	const epsilon = 0.5
	return minX <= cx0+epsilon &&
		maxX >= cx1-epsilon &&
		minY <= cy0+epsilon &&
		maxY >= cy1-epsilon
}

func (e *Evaluator) drawImageUsingCurrentTransform(
	img stdimage.Image,
	imageCTM [6]float64,
	interpolate bool,
	sampler string,
	phaseX float64,
	phaseY float64,
	imageEdgeMode string,
) error {
	e.canvas.Save()
	e.canvas.Transform(imageCTM)
	defer e.canvas.Restore()
	if drawer, ok := e.canvas.(interface {
		DrawImageWithPhaseSamplerAndEdgeMode(
			img stdimage.Image,
			x, y, w, h float64,
			interpolate bool,
			sampler string,
			phaseX, phaseY float64,
			edgeMode string,
		) error
	}); ok {
		return drawer.DrawImageWithPhaseSamplerAndEdgeMode(
			img,
			0,
			0,
			1,
			1,
			interpolate,
			sampler,
			phaseX,
			phaseY,
			imageEdgeMode,
		)
	}
	if drawer, ok := e.canvas.(interface {
		DrawImageWithPhaseAndSampler(
			img stdimage.Image,
			x, y, w, h float64,
			interpolate bool,
			sampler string,
			phaseX, phaseY float64,
		) error
	}); ok {
		return drawer.DrawImageWithPhaseAndSampler(
			img,
			0,
			0,
			1,
			1,
			interpolate,
			sampler,
			phaseX,
			phaseY,
		)
	}
	if drawer, ok := e.canvas.(interface {
		DrawImageWithPhase(
			img stdimage.Image,
			x, y, w, h float64,
			interpolate bool,
			phaseX, phaseY float64,
		) error
	}); ok {
		return drawer.DrawImageWithPhase(
			img,
			0,
			0,
			1,
			1,
			interpolate,
			phaseX,
			phaseY,
		)
	}
	return e.canvas.DrawImage(
		img,
		0,
		0,
		1,
		1,
		interpolate,
	)
}

type softMaskedImageDrawer interface {
	DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(
		img stdimage.Image,
		mask domainimage.ImageMask,
		x, y, w, h float64,
		interpolate bool,
		sampler string,
		phaseX, phaseY float64,
		edgeMode string,
	) error
}

type imageMaskCanvas interface {
	DrawImageMask(
		mask domainimage.ImageMask,
		x, y, w, h float64,
	) error
}

type softMaskedImageDrawerWithMaskInterpolation interface {
	DrawImageWithSoftMaskAndMaskInterpolate(
		img stdimage.Image,
		mask domainimage.ImageMask,
		x, y, w, h float64,
		interpolate bool,
		maskInterpolate bool,
		sampler string,
		phaseX, phaseY float64,
		edgeMode string,
	) error
}

type sourceAlphaImageDrawer interface {
	DrawImageWithSourceAlpha(
		img stdimage.Image,
		x, y, w, h float64,
		interpolate bool,
	) error
}

func (e *Evaluator) drawSourceAlphaImageUsingCurrentTransform(
	img stdimage.Image,
	imageCTM [6]float64,
	interpolate bool,
	sampler string,
	phaseX float64,
	phaseY float64,
	imageEdgeMode string,
) error {
	drawer, ok := e.canvas.(sourceAlphaImageDrawer)
	if !ok {
		return e.drawImageUsingCurrentTransform(img, imageCTM, interpolate, sampler, phaseX, phaseY, imageEdgeMode)
	}

	e.canvas.Save()
	e.canvas.Transform(imageCTM)
	defer e.canvas.Restore()
	return drawer.DrawImageWithSourceAlpha(img, 0, 0, 1, 1, interpolate)
}

func (e *Evaluator) drawImageMaskUsingCurrentTransform(
	drawer imageMaskCanvas,
	mask domainimage.ImageMask,
	imageCTM [6]float64,
) error {
	e.canvas.Save()
	e.canvas.Transform(imageCTM)
	defer e.canvas.Restore()
	return drawer.DrawImageMask(mask, 0, 0, 1, 1)
}

func (e *Evaluator) drawSoftMaskedImageUsingCurrentTransform(
	img stdimage.Image,
	mask domainimage.ImageMask,
	imageCTM [6]float64,
	interpolate bool,
	maskInterpolate bool,
	sampler string,
	phaseX float64,
	phaseY float64,
	imageEdgeMode string,
) error {
	if drawer, ok := e.canvas.(softMaskedImageDrawerWithMaskInterpolation); ok {
		e.canvas.Save()
		e.canvas.Transform(imageCTM)
		defer e.canvas.Restore()
		return drawer.DrawImageWithSoftMaskAndMaskInterpolate(
			img,
			mask,
			0,
			0,
			1,
			1,
			interpolate,
			maskInterpolate,
			sampler,
			phaseX,
			phaseY,
			imageEdgeMode,
		)
	}

	drawer, ok := e.canvas.(softMaskedImageDrawer)
	if !ok {
		return fmt.Errorf("canvas does not support soft masked images")
	}

	e.canvas.Save()
	e.canvas.Transform(imageCTM)
	defer e.canvas.Restore()
	return drawer.DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(
		img,
		mask,
		0,
		0,
		1,
		1,
		interpolate,
		sampler,
		phaseX,
		phaseY,
		imageEdgeMode,
	)
}

func imageSamplingPhase(sampler, reason string, interpolate bool, ctm [6]float64) (float64, float64) {
	if sampler == "experimental_indexed_origin_downscale_bilinear" {
		if math.Abs(ctm[4]) <= 1e-6 && math.Abs(ctm[5]) <= 1e-6 {
			return 0.5, 0.5
		}
		return 0, 0
	}

	if interpolate {
		if sampler == "auto_approx_bilinear" {
			return 0, 0
		}
		if sampler == "auto_approx_bilinear_tiny_gray_ccittfax_downscale" {
			return 0, 0
		}
		if sampler == "adaptive_downscale_bilinear_tiny_encoded_gray" {
			return 0.5, 0.5
		}
		if strings.Contains(sampler, "bilinear") {
			return 0, 0
		}
		if strings.Contains(sampler, "box") {
			return 0, 0
		}
	}

	if interpolate {
		return 0.5, 0.5
	}
	if strings.Contains(sampler, "nearest") {
		return 0, 0
	}
	switch sampler {
	case "auto_dct_rgb_splash_scale_downscale":
		return 0, 0
	case "auto_dct_rgb_splash_scale_upscale":
		return 0, 0
	case "auto_downscale_nearest":
		return 0, 0
	case "auto_upscale_nearest":
		return 0, 0
	case "explicit_nearest":
		return 0, 0
	case "auto_nearest":
		if reason == "auto_interpolate=false_downscale_small_grayscale" {
			return 0, 0
		}
	}
	return 0.5, 0.5
}

func projectedImageDimensions(ctm [6]float64, srcWidth, srcHeight int) (float64, float64) {
	_ = srcWidth
	_ = srcHeight
	width := math.Sqrt(ctm[0]*ctm[0] + ctm[1]*ctm[1])
	height := math.Sqrt(ctm[2]*ctm[2] + ctm[3]*ctm[3])
	return width, height
}
