package jbig2

import (
	"encoding/binary"
	"fmt"
	stdimage "image"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

type patternDictionary struct {
	patterns []*stdimage.Gray
}

type halftoneRegion struct {
	info       regionInfo
	payload    []byte
	mmr        bool
	template   byte
	enableSkip bool
	combOp     byte
	defaultPix bool
	gridW      int
	gridH      int
	gridX      int32
	gridY      int32
	stepX      uint16
	stepY      uint16
}

func decodePatternDictionarySegment(segment segmentHeader) (patternDictionary, error) {
	if len(segment.data) < 7 {
		return patternDictionary{}, errors.Invalid("jbig2_pattern_dictionary", fmt.Errorf("truncated pattern dictionary"))
	}
	flags := segment.data[0]
	patternW := int(segment.data[1])
	patternH := int(segment.data[2])
	grayMax := int(binary.BigEndian.Uint32(segment.data[3:7]))
	if patternW <= 0 || patternH <= 0 {
		return patternDictionary{}, errors.Invalid("jbig2_pattern_dictionary", fmt.Errorf("invalid pattern size: %dx%d", patternW, patternH))
	}
	if grayMax < 0 {
		return patternDictionary{}, errors.Invalid("jbig2_pattern_dictionary", fmt.Errorf("invalid gray max"))
	}

	patternCount := grayMax + 1
	bitmapW := patternCount * patternW
	region := genericRegion{
		info: regionInfo{
			width:  bitmapW,
			height: patternH,
		},
		payload:  segment.data[7:],
		mmr:      flags&0x01 != 0,
		template: (flags >> 1) & 0x03,
		atPixels: []adaptiveTemplatePixel{
			{x: -patternW, y: 0},
			{x: -3, y: -1},
			{x: 2, y: -2},
			{x: -2, y: -2},
		},
	}
	bitmap, err := decodeGenericRegion(region)
	if err != nil {
		return patternDictionary{}, errors.Invalid("jbig2_pattern_dictionary", err)
	}

	dict := patternDictionary{patterns: make([]*stdimage.Gray, patternCount)}
	for i := 0; i < patternCount; i++ {
		dict.patterns[i] = sliceGray(bitmap, i*patternW, 0, patternW, patternH)
	}
	return dict, nil
}

func decodeHalftoneRegionSegment(segment segmentHeader, patternDicts map[uint32]patternDictionary) (regionInfo, *stdimage.Gray, error) {
	region, err := parseHalftoneRegionSegment(segment.data)
	if err != nil {
		return regionInfo{}, nil, errors.Invalid("jbig2_halftone_region", err)
	}
	if len(segment.referredToSegmentNumbers) != 1 {
		return regionInfo{}, nil, errors.Invalid("jbig2_halftone_region", fmt.Errorf("halftone region must reference exactly one pattern dictionary"))
	}
	dict, ok := patternDicts[segment.referredToSegmentNumbers[0]]
	if !ok || len(dict.patterns) == 0 || dict.patterns[0] == nil {
		return regionInfo{}, nil, errors.Invalid("jbig2_halftone_region", fmt.Errorf("missing referenced pattern dictionary %d", segment.referredToSegmentNumbers[0]))
	}
	img, err := decodeHalftoneRegion(region, dict)
	if err != nil {
		return regionInfo{}, nil, err
	}
	return region.info, img, nil
}

func parseHalftoneRegionSegment(data []byte) (halftoneRegion, error) {
	info, offset, err := parseRegionInfo(data)
	if err != nil {
		return halftoneRegion{}, err
	}
	if len(data)-offset < 21 {
		return halftoneRegion{}, fmt.Errorf("truncated halftone region header")
	}
	flags := data[offset]
	offset++

	region := halftoneRegion{
		info:       info,
		mmr:        flags&0x01 != 0,
		template:   (flags >> 1) & 0x03,
		enableSkip: flags&0x08 != 0,
		combOp:     (flags >> 4) & 0x07,
		defaultPix: flags&0x80 != 0,
		gridW:      int(binary.BigEndian.Uint32(data[offset : offset+4])),
		gridH:      int(binary.BigEndian.Uint32(data[offset+4 : offset+8])),
		gridX:      int32(binary.BigEndian.Uint32(data[offset+8 : offset+12])),
		gridY:      int32(binary.BigEndian.Uint32(data[offset+12 : offset+16])),
		stepX:      binary.BigEndian.Uint16(data[offset+16 : offset+18]),
		stepY:      binary.BigEndian.Uint16(data[offset+18 : offset+20]),
	}
	offset += 20
	if region.gridW <= 0 || region.gridH <= 0 {
		return halftoneRegion{}, fmt.Errorf("invalid halftone grid size: %dx%d", region.gridW, region.gridH)
	}
	region.payload = data[offset:]
	return region, nil
}

func decodeHalftoneRegion(region halftoneRegion, dict patternDictionary) (*stdimage.Gray, error) {
	bpp := halftoneBitsPerPixel(len(dict.patterns))
	pattern := dict.patterns[0]
	if pattern == nil {
		return nil, errors.Invalid("jbig2_halftone_region", fmt.Errorf("missing base pattern bitmap"))
	}
	img := stdimage.NewGray(stdimage.Rect(0, 0, region.info.width, region.info.height))
	defaultPixel := uint8(0xff)
	if region.defaultPix {
		defaultPixel = 0x00
	}
	for i := range img.Pix {
		img.Pix[i] = defaultPixel
	}

	var skip *stdimage.Gray
	if region.enableSkip {
		skip = computeHalftoneSkipBitmap(region, pattern.Bounds().Dx(), pattern.Bounds().Dy())
	}

	gray := make([]uint32, region.gridW*region.gridH)
	if bpp > 0 {
		if err := decodeHalftoneGrayImage(region, skip, gray, bpp); err != nil {
			return nil, err
		}
	}

	i := 0
	for m := 0; m < region.gridH; m++ {
		xx := int(region.gridX) + m*int(region.stepY)
		yy := int(region.gridY) + m*int(region.stepX)
		for n := 0; n < region.gridW; n++ {
			if skip == nil || genericRegionPixel(skip, n, m) == 0 {
				patternIndex := int(gray[i])
				if patternIndex >= len(dict.patterns) || dict.patterns[patternIndex] == nil {
					return nil, errors.Invalid("jbig2_halftone_region", fmt.Errorf("invalid pattern index %d", patternIndex))
				}
				composeBilevelBitmap(img, dict.patterns[patternIndex], xx>>8, yy>>8, region.combOp)
			}
			xx += int(region.stepX)
			yy -= int(region.stepY)
			i++
		}
	}
	return img, nil
}

func decodeHalftoneGrayImage(region halftoneRegion, skip *stdimage.Gray, gray []uint32, bpp int) error {
	planeRegion := genericRegion{
		info: regionInfo{
			width:  region.gridW,
			height: region.gridH,
		},
		payload:  region.payload,
		mmr:      region.mmr,
		template: region.template,
		skip:     skip,
		atPixels: []adaptiveTemplatePixel{
			{x: adaptiveTemplateX(region.template), y: -1},
			{x: -3, y: -1},
			{x: 2, y: -2},
			{x: -2, y: -2},
		},
	}
	if region.mmr {
		decoder := NewMMRDecoder(region.payload, region.gridW, region.gridH*bpp)
		for bit := bpp - 1; bit >= 0; bit-- {
			bitmap := stdimage.NewGray(stdimage.Rect(0, 0, region.gridW, region.gridH))
			for i := range bitmap.Pix {
				bitmap.Pix[i] = 0xff
			}
			if err := decodeMMRGenericRegionWithDecoder(planeRegion, bitmap, decoder); err != nil {
				return errors.Invalid("jbig2_halftone_region", err)
			}
			accumulateHalftoneGrayPlane(gray, bitmap)
		}
		return nil
	}

	stats := make([]uint8, genericRegionStatsSize(region.template))
	decoder := NewArithmeticDecoder(region.payload)
	for bit := bpp - 1; bit >= 0; bit-- {
		bitmap := stdimage.NewGray(stdimage.Rect(0, 0, region.gridW, region.gridH))
		for i := range bitmap.Pix {
			bitmap.Pix[i] = 0xff
		}
		if _, err := decodeArithmeticGenericRegionWithState(planeRegion, bitmap, decoder, stats); err != nil {
			return errors.Invalid("jbig2_halftone_region", err)
		}
		accumulateHalftoneGrayPlane(gray, bitmap)
	}
	return nil
}

func decodeMMRGenericRegionWithDecoder(region genericRegion, img *stdimage.Gray, decoder *MMRDecoder) error {
	for y := 0; y < region.info.height; y++ {
		line, err := decoder.DecodeLine()
		if err != nil {
			return err
		}
		paintBilevelRow(img, y, line)
	}
	return nil
}

func accumulateHalftoneGrayPlane(gray []uint32, bitmap *stdimage.Gray) {
	i := 0
	for y := 0; y < bitmap.Bounds().Dy(); y++ {
		for x := 0; x < bitmap.Bounds().Dx(); x++ {
			bit := uint32(0)
			if bitmap.Pix[y*bitmap.Stride+x] == 0x00 {
				bit = 1
			}
			gray[i] = (gray[i] << 1) | (bit ^ (gray[i] & 1))
			i++
		}
	}
}

func computeHalftoneSkipBitmap(region halftoneRegion, patternW, patternH int) *stdimage.Gray {
	skip := stdimage.NewGray(stdimage.Rect(0, 0, region.gridW, region.gridH))
	for i := range skip.Pix {
		skip.Pix[i] = 0xff
	}
	for m := 0; m < region.gridH; m++ {
		for n := 0; n < region.gridW; n++ {
			xx := int64(region.gridX) + int64(m)*int64(region.stepY) + int64(n)*int64(region.stepX)
			yy := int64(region.gridY) + int64(m)*int64(region.stepX) - int64(n)*int64(region.stepY)
			outsideX := ((xx+int64(patternW))>>8) <= 0 || (xx>>8) >= int64(region.info.width)
			outsideY := ((yy+int64(patternH))>>8) <= 0 || (yy>>8) >= int64(region.info.height)
			if outsideX || outsideY {
				skip.Pix[m*skip.Stride+n] = 0x00
			}
		}
	}
	return skip
}

func halftoneBitsPerPixel(patternCount int) int {
	if patternCount <= 1 {
		return 0
	}
	value := patternCount - 1
	bpp := 0
	for value > 0 {
		bpp++
		value >>= 1
	}
	return bpp
}

func adaptiveTemplateX(template byte) int {
	if template <= 1 {
		return 3
	}
	return 2
}

func sliceGray(src *stdimage.Gray, x, y, w, h int) *stdimage.Gray {
	dst := stdimage.NewGray(stdimage.Rect(0, 0, w, h))
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			dst.Pix[yy*dst.Stride+xx] = src.Pix[(y+yy)*src.Stride+x+xx]
		}
	}
	return dst
}

func composeBilevelBitmap(dst, src *stdimage.Gray, x0, y0 int, op byte) {
	for y := 0; y < src.Bounds().Dy(); y++ {
		dstY := y0 + y
		if dstY < 0 || dstY >= dst.Bounds().Dy() {
			continue
		}
		for x := 0; x < src.Bounds().Dx(); x++ {
			dstX := x0 + x
			if dstX < 0 || dstX >= dst.Bounds().Dx() {
				continue
			}
			dstOffset := dstY*dst.Stride + dstX
			srcOffset := y*src.Stride + x
			dst.Pix[dstOffset] = combineBilevelPixel(dst.Pix[dstOffset], src.Pix[srcOffset], op)
		}
	}
}
