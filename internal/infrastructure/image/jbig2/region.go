package jbig2

import (
	"encoding/binary"
	"fmt"
	stdimage "image"
)

const regionInfoLength = 17

const (
	jbig2CombineOr byte = iota
	jbig2CombineAnd
	jbig2CombineXor
	jbig2CombineXnor
	jbig2CombineReplace
)

type regionInfo struct {
	width  int
	height int
	x      int
	y      int
	flags  byte
}

type adaptiveTemplatePixel struct {
	x int
	y int
}

type genericRegion struct {
	info     regionInfo
	payload  []byte
	atPixels []adaptiveTemplatePixel
	skip     *stdimage.Gray
	mmr      bool
	template byte
	tpgdOn   bool
}

func parseGenericRegionSegment(data []byte) (genericRegion, error) {
	info, offset, err := parseRegionInfo(data)
	if err != nil {
		return genericRegion{}, err
	}
	if len(data)-offset < 1 {
		return genericRegion{}, fmt.Errorf("truncated generic region flags")
	}

	flags := data[offset]
	offset++

	region := genericRegion{
		info:     info,
		mmr:      flags&0x01 != 0,
		template: (flags >> 1) & 0x03,
		tpgdOn:   flags&0x08 != 0,
	}

	if !region.mmr {
		atPixelCount := 1
		if region.template == 0 {
			atPixelCount = 4
		}

		atPixels, nextOffset, err := parseAdaptiveTemplatePixels(data, offset, atPixelCount)
		if err != nil {
			return genericRegion{}, err
		}
		region.atPixels = atPixels
		offset = nextOffset
	}

	region.payload = data[offset:]
	return region, nil
}

func parseRegionInfo(data []byte) (regionInfo, int, error) {
	if len(data) < regionInfoLength {
		return regionInfo{}, 0, fmt.Errorf("truncated region info")
	}

	info := regionInfo{
		width:  int(binary.BigEndian.Uint32(data[0:4])),
		height: int(binary.BigEndian.Uint32(data[4:8])),
		x:      int(binary.BigEndian.Uint32(data[8:12])),
		y:      int(binary.BigEndian.Uint32(data[12:16])),
		flags:  data[16],
	}
	if info.width <= 0 || info.height <= 0 {
		return regionInfo{}, 0, fmt.Errorf("invalid region dimensions: %dx%d", info.width, info.height)
	}

	return info, regionInfoLength, nil
}

func parseAdaptiveTemplatePixels(data []byte, offset, count int) ([]adaptiveTemplatePixel, int, error) {
	byteCount := count * 2
	if len(data)-offset < byteCount {
		return nil, 0, fmt.Errorf("truncated adaptive template")
	}

	pixels := make([]adaptiveTemplatePixel, count)
	for i := 0; i < count; i++ {
		pixels[i] = adaptiveTemplatePixel{
			x: int(int8(data[offset+i*2])),
			y: int(int8(data[offset+i*2+1])),
		}
	}
	return pixels, offset + byteCount, nil
}

func validateBitmapSegmentBody(segment segmentHeader) error {
	switch segment.typ {
	case SegmentIntermediateGenericRegion,
		SegmentImmediateGenericRegion,
		SegmentImmediateLosslessGenericRegion:
		_, err := parseGenericRegionSegment(segment.data)
		return err
	case SegmentIntermediateGenericRefinementRegion,
		SegmentImmediateGenericRefinementRegion,
		SegmentImmediateLosslessGenericRefinementRegion:
		_, err := parseGenericRefinementRegionSegment(segment.data)
		return err
	default:
		return nil
	}
}

func decodeGenericRegion(region genericRegion) (*stdimage.Gray, error) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, region.info.width, region.info.height))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}

	if !region.mmr {
		return decodeArithmeticGenericRegion(region, img)
	}

	decoder := NewMMRDecoder(region.payload, region.info.width, region.info.height)
	for y := 0; y < region.info.height; y++ {
		line, err := decoder.DecodeLine()
		if err != nil {
			return nil, err
		}
		paintBilevelRow(img, y, line)
	}
	return img, nil
}

func decodeArithmeticGenericRegion(region genericRegion, img *stdimage.Gray) (*stdimage.Gray, error) {
	stats := make([]uint8, genericRegionStatsSize(region.template))
	decoder := NewArithmeticDecoder(region.payload)
	return decodeArithmeticGenericRegionWithState(region, img, decoder, stats)
}

func decodeArithmeticGenericRegionWithState(region genericRegion, img *stdimage.Gray, decoder *ArithmeticDecoder, stats []uint8) (*stdimage.Gray, error) {
	ltp := false
	ltpContext := genericRegionTypicalPredictionContext(region.template)
	for y := 0; y < region.info.height; y++ {
		if region.tpgdOn {
			bit, err := decoder.DecodeBitContext(stats, ltpContext)
			if err != nil {
				return nil, err
			}
			if bit != 0 {
				ltp = !ltp
			}
			if ltp {
				if y > 0 {
					copy(img.Pix[y*img.Stride:y*img.Stride+region.info.width], img.Pix[(y-1)*img.Stride:(y-1)*img.Stride+region.info.width])
				}
				continue
			}
		}
		for x := 0; x < region.info.width; x++ {
			if region.skip != nil && genericRegionPixel(region.skip, x, y) != 0 {
				img.Pix[y*img.Stride+x] = 0xff
				continue
			}
			context := genericRegionContext(img, x, y, region.template, region.atPixels)
			bit, err := decoder.DecodeBitContext(stats, context)
			if err != nil {
				return nil, err
			}
			if bit == 1 {
				img.Pix[y*img.Stride+x] = 0x00
			} else {
				img.Pix[y*img.Stride+x] = 0xff
			}
		}
	}
	return img, nil
}

func genericRegionTypicalPredictionContext(template byte) uint32 {
	switch template {
	case 0:
		return 0x3953
	case 1:
		return 0x079a
	case 2:
		return 0x0e3
	case 3:
		return 0x18b
	default:
		return 0
	}
}

func genericRegionStatsSize(template byte) int {
	switch template {
	case 0:
		return 1 << 16
	case 1:
		return 1 << 13
	default:
		return 1 << 10
	}
}

func genericRegionContext(img *stdimage.Gray, x, y int, template byte, atPixels []adaptiveTemplatePixel) uint32 {
	switch template {
	case 0:
		return genericRegionTemplate0Context(img, x, y, atPixels)
	case 1:
		return genericRegionTemplate1Context(img, x, y, atPixels)
	case 2:
		return genericRegionTemplate2Context(img, x, y, atPixels)
	case 3:
		return genericRegionTemplate3Context(img, x, y, atPixels)
	default:
		return 0
	}
}

func genericRegionTemplate0Context(img *stdimage.Gray, x, y int, atPixels []adaptiveTemplatePixel) uint32 {
	var context uint32
	for i := 0; i < 4; i++ {
		context |= genericRegionPixel(img, x-1-i, y) << i
	}
	context |= genericRegionATPixel(img, x, y, atPixels, 0) << 4
	context |= genericRegionPixel(img, x+2, y-1) << 5
	context |= genericRegionPixel(img, x+1, y-1) << 6
	context |= genericRegionPixel(img, x, y-1) << 7
	context |= genericRegionPixel(img, x-1, y-1) << 8
	context |= genericRegionPixel(img, x-2, y-1) << 9
	context |= genericRegionATPixel(img, x, y, atPixels, 1) << 10
	context |= genericRegionATPixel(img, x, y, atPixels, 2) << 11
	context |= genericRegionPixel(img, x+1, y-2) << 12
	context |= genericRegionPixel(img, x, y-2) << 13
	context |= genericRegionPixel(img, x-1, y-2) << 14
	context |= genericRegionATPixel(img, x, y, atPixels, 3) << 15
	return context
}

func genericRegionTemplate1Context(img *stdimage.Gray, x, y int, atPixels []adaptiveTemplatePixel) uint32 {
	var context uint32
	for i := 0; i < 3; i++ {
		context |= genericRegionPixel(img, x-1-i, y) << i
	}
	context |= genericRegionATPixel(img, x, y, atPixels, 0) << 3
	context |= genericRegionPixel(img, x+2, y-1) << 4
	context |= genericRegionPixel(img, x+1, y-1) << 5
	context |= genericRegionPixel(img, x, y-1) << 6
	context |= genericRegionPixel(img, x-1, y-1) << 7
	context |= genericRegionPixel(img, x-2, y-1) << 8
	context |= genericRegionPixel(img, x+2, y-2) << 9
	context |= genericRegionPixel(img, x+1, y-2) << 10
	context |= genericRegionPixel(img, x, y-2) << 11
	context |= genericRegionPixel(img, x-1, y-2) << 12
	return context
}

func genericRegionTemplate2Context(img *stdimage.Gray, x, y int, atPixels []adaptiveTemplatePixel) uint32 {
	var context uint32
	context |= genericRegionPixel(img, x-1, y)
	context |= genericRegionPixel(img, x-2, y) << 1
	context |= genericRegionATPixel(img, x, y, atPixels, 0) << 2
	context |= genericRegionPixel(img, x+1, y-1) << 3
	context |= genericRegionPixel(img, x, y-1) << 4
	context |= genericRegionPixel(img, x-1, y-1) << 5
	context |= genericRegionPixel(img, x-2, y-1) << 6
	context |= genericRegionPixel(img, x+1, y-2) << 7
	context |= genericRegionPixel(img, x, y-2) << 8
	context |= genericRegionPixel(img, x-1, y-2) << 9
	return context
}

func genericRegionTemplate3Context(img *stdimage.Gray, x, y int, atPixels []adaptiveTemplatePixel) uint32 {
	var context uint32
	for i := 0; i < 4; i++ {
		context |= genericRegionPixel(img, x-1-i, y) << i
	}
	context |= genericRegionATPixel(img, x, y, atPixels, 0) << 4
	context |= genericRegionPixel(img, x+1, y-1) << 5
	context |= genericRegionPixel(img, x, y-1) << 6
	context |= genericRegionPixel(img, x-1, y-1) << 7
	context |= genericRegionPixel(img, x-2, y-1) << 8
	context |= genericRegionPixel(img, x-3, y-1) << 9
	return context
}

func genericRegionATPixel(img *stdimage.Gray, x, y int, atPixels []adaptiveTemplatePixel, index int) uint32 {
	if index >= len(atPixels) {
		return 0
	}
	pixel := atPixels[index]
	return genericRegionPixel(img, x+pixel.x, y+pixel.y)
}

func genericRegionPixel(img *stdimage.Gray, x, y int) uint32 {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return 0
	}
	if img.Pix[y*img.Stride+x] == 0x00 {
		return 1
	}
	return 0
}

func paintBilevelRow(img *stdimage.Gray, y int, row []byte) {
	bounds := img.Bounds()
	for x := 0; x < bounds.Dx(); x++ {
		byteOffset := x / 8
		bitOffset := 7 - (x % 8)
		pixel := uint8(0xff)
		if byteOffset < len(row) && ((row[byteOffset]>>bitOffset)&0x01) == 1 {
			pixel = 0x00
		}
		img.Pix[y*img.Stride+x] = pixel
	}
}

func composeRegionOnPage(pageInfo *JBIG2Header, regionInfo regionInfo, region *stdimage.Gray) *stdimage.Gray {
	page := stdimage.NewGray(stdimage.Rect(0, 0, pageInfo.Width, pageInfo.Height))
	pixel := uint8(0xff)
	if pageInfo.DefaultPixel {
		pixel = 0x00
	}
	for i := range page.Pix {
		page.Pix[i] = pixel
	}
	composeRegionIntoPage(page, regionInfo, region)
	return page
}

func composeRegionIntoPage(page *stdimage.Gray, regionInfo regionInfo, region *stdimage.Gray) {
	for y := 0; y < region.Bounds().Dy(); y++ {
		dstY := regionInfo.y + y
		if dstY < 0 || dstY >= page.Bounds().Dy() {
			continue
		}
		for x := 0; x < region.Bounds().Dx(); x++ {
			dstX := regionInfo.x + x
			if dstX < 0 || dstX >= page.Bounds().Dx() {
				continue
			}
			dstOffset := dstY*page.Stride + dstX
			srcOffset := y*region.Stride + x
			page.Pix[dstOffset] = combineBilevelPixel(page.Pix[dstOffset], region.Pix[srcOffset], regionInfo.flags&0x07)
		}
	}
}

func combineBilevelPixel(dst, src, op byte) byte {
	dstBlack := dst == 0x00
	srcBlack := src == 0x00

	var black bool
	switch op {
	case jbig2CombineOr:
		black = dstBlack || srcBlack
	case jbig2CombineAnd:
		black = dstBlack && srcBlack
	case jbig2CombineXor:
		black = dstBlack != srcBlack
	case jbig2CombineXnor:
		black = dstBlack == srcBlack
	case jbig2CombineReplace:
		black = srcBlack
	default:
		black = dstBlack
	}
	if black {
		return 0x00
	}
	return 0xff
}
