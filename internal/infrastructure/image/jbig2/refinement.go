package jbig2

import (
	"fmt"
	stdimage "image"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

type genericRefinementRegion struct {
	info     regionInfo
	payload  []byte
	ref      *stdimage.Gray
	refDX    int
	refDY    int
	atPixels []adaptiveTemplatePixel
	template byte
	tpgrOn   bool
}

func parseGenericRefinementRegionSegment(data []byte) (genericRefinementRegion, error) {
	info, offset, err := parseRegionInfo(data)
	if err != nil {
		return genericRefinementRegion{}, err
	}
	if len(data)-offset < 1 {
		return genericRefinementRegion{}, fmt.Errorf("truncated generic refinement region flags")
	}

	flags := data[offset]
	offset++
	region := genericRefinementRegion{
		info:     info,
		template: flags & 0x01,
		tpgrOn:   flags&0x02 != 0,
	}
	if region.template > 1 {
		return genericRefinementRegion{}, fmt.Errorf("invalid generic refinement template: %d", region.template)
	}
	if region.template == 0 {
		atPixels, nextOffset, err := parseAdaptiveTemplatePixels(data, offset, 2)
		if err != nil {
			return genericRefinementRegion{}, err
		}
		region.atPixels = atPixels
		offset = nextOffset
	}
	region.payload = data[offset:]
	return region, nil
}

func decodeGenericRefinementRegionSegment(segment segmentHeader, page *stdimage.Gray, bitmaps map[uint32]*stdimage.Gray) (regionInfo, *stdimage.Gray, error) {
	region, err := parseGenericRefinementRegionSegment(segment.data)
	if err != nil {
		return regionInfo{}, nil, errors.Invalid("jbig2_refinement_region", err)
	}
	if len(segment.referredToSegmentNumbers) > 1 {
		return regionInfo{}, nil, errors.Invalid("jbig2_refinement_region", fmt.Errorf("generic refinement region may reference at most one bitmap"))
	}
	if len(segment.referredToSegmentNumbers) == 1 {
		refNum := segment.referredToSegmentNumbers[0]
		ref, ok := bitmaps[refNum]
		if !ok || ref == nil {
			return regionInfo{}, nil, errors.Invalid("jbig2_refinement_region", fmt.Errorf("missing referenced bitmap %d", refNum))
		}
		region.ref = ref
	} else {
		region.ref = sliceGrayWithWhiteDefault(page, region.info.x, region.info.y, region.info.width, region.info.height)
	}

	img, err := decodeGenericRefinementRegion(region)
	if err != nil {
		return regionInfo{}, nil, err
	}
	return region.info, img, nil
}

func decodeGenericRefinementRegion(region genericRefinementRegion) (*stdimage.Gray, error) {
	stats := make([]uint8, refinementRegionStatsSize(region.template))
	decoder := NewArithmeticDecoder(region.payload)
	return decodeGenericRefinementRegionWithState(region, decoder, stats)
}

func decodeGenericRefinementRegionWithState(region genericRefinementRegion, decoder *ArithmeticDecoder, stats []uint8) (*stdimage.Gray, error) {
	if region.ref == nil {
		return nil, errors.Invalid("jbig2_refinement_region", fmt.Errorf("missing reference bitmap"))
	}
	img := stdimage.NewGray(stdimage.Rect(0, 0, region.info.width, region.info.height))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	ltpContext := genericRefinementTypicalPredictionContext(region.template)
	for y := 0; y < region.info.height; y++ {
		for x := 0; x < region.info.width; x++ {
			if region.tpgrOn {
				if _, err := decoder.DecodeBitContext(stats, ltpContext); err != nil {
					return nil, errors.Invalid("jbig2_refinement_region", err)
				}
				if typicalRefinementPixel, ok := genericRefinementTypicalPixel(region.ref, x, y, region.refDX, region.refDY); ok {
					if typicalRefinementPixel != 0 {
						img.Pix[y*img.Stride+x] = 0x00
					}
					continue
				}
			}

			context := genericRefinementContext(region, img, x, y)
			bit, err := decoder.DecodeBitContext(stats, context)
			if err != nil {
				return nil, errors.Invalid("jbig2_refinement_region", err)
			}
			if bit != 0 {
				img.Pix[y*img.Stride+x] = 0x00
			}
		}
	}
	return img, nil
}

func refinementRegionStatsSize(template byte) int {
	if template == 0 {
		return 1 << 13
	}
	return 1 << 10
}

func genericRefinementTypicalPredictionContext(template byte) uint32 {
	if template == 0 {
		return 0x0010
	}
	return 0x0008
}

func genericRefinementContext(region genericRefinementRegion, img *stdimage.Gray, x, y int) uint32 {
	if region.template == 0 {
		return genericRefinementTemplate0Context(region, img, x, y)
	}
	return genericRefinementTemplate1Context(region, img, x, y)
}

func genericRefinementTemplate0Context(region genericRefinementRegion, img *stdimage.Gray, x, y int) uint32 {
	ref := region.ref
	refX := x - region.refDX
	refY := y - region.refDY

	cx0 := genericRegionPixel(img, x, y-1)<<1 |
		genericRegionPixel(img, x+1, y-1)
	cx2 := genericRegionPixel(ref, refX, refY-1)<<1 |
		genericRegionPixel(ref, refX+1, refY-1)
	cx3 := genericRegionPixel(ref, refX-1, refY)<<2 |
		genericRegionPixel(ref, refX, refY)<<1 |
		genericRegionPixel(ref, refX+1, refY)
	cx4 := genericRegionPixel(ref, refX-1, refY+1)<<2 |
		genericRegionPixel(ref, refX, refY+1)<<1 |
		genericRegionPixel(ref, refX+1, refY+1)

	return (cx0 << 11) |
		(genericRegionPixel(img, x-1, y) << 10) |
		(cx2 << 8) |
		(cx3 << 5) |
		(cx4 << 2) |
		(genericRefinementATPixel(img, x, y, region.atPixels, 0, 0, 0) << 1) |
		genericRefinementATPixel(ref, x, y, region.atPixels, 1, region.refDX, region.refDY)
}

func genericRefinementTemplate1Context(region genericRefinementRegion, img *stdimage.Gray, x, y int) uint32 {
	ref := region.ref
	refX := x - region.refDX
	refY := y - region.refDY

	cx0 := genericRegionPixel(img, x-1, y-1)<<2 |
		genericRegionPixel(img, x, y-1)<<1 |
		genericRegionPixel(img, x+1, y-1)
	cx3 := genericRegionPixel(ref, refX-1, refY)<<2 |
		genericRegionPixel(ref, refX, refY)<<1 |
		genericRegionPixel(ref, refX+1, refY)
	cx4 := genericRegionPixel(ref, refX, refY+1)<<1 |
		genericRegionPixel(ref, refX+1, refY+1)

	return (cx0 << 7) |
		(genericRegionPixel(img, x-1, y) << 6) |
		(genericRegionPixel(ref, refX, refY-1) << 5) |
		(cx3 << 2) |
		cx4
}

func genericRefinementATPixel(img *stdimage.Gray, x, y int, atPixels []adaptiveTemplatePixel, index, dx, dy int) uint32 {
	if index >= len(atPixels) {
		return 0
	}
	pixel := atPixels[index]
	return genericRegionPixel(img, x+pixel.x-dx, y+pixel.y-dy)
}

func genericRefinementTypicalPixel(ref *stdimage.Gray, x, y, refDX, refDY int) (uint32, bool) {
	refX := x - refDX
	refY := y - refDY
	row0 := refinementTypicalRow(ref, refX, refY-1)
	row1 := refinementTypicalRow(ref, refX, refY)
	row2 := refinementTypicalRow(ref, refX, refY+1)
	if row0 == 0 && row1 == 0 && row2 == 0 {
		return 0, true
	}
	if row0 == 7 && row1 == 7 && row2 == 7 {
		return 1, true
	}
	return 0, false
}

func refinementTypicalRow(ref *stdimage.Gray, x, y int) uint32 {
	return genericRegionPixel(ref, x, y)<<2 |
		genericRegionPixel(ref, x+1, y)<<1 |
		genericRegionPixel(ref, x+2, y)
}

func sliceGrayWithWhiteDefault(src *stdimage.Gray, x, y, w, h int) *stdimage.Gray {
	dst := stdimage.NewGray(stdimage.Rect(0, 0, w, h))
	for i := range dst.Pix {
		dst.Pix[i] = 0xff
	}
	for yy := 0; yy < h; yy++ {
		srcY := y + yy
		if srcY < 0 || srcY >= src.Bounds().Dy() {
			continue
		}
		for xx := 0; xx < w; xx++ {
			srcX := x + xx
			if srcX < 0 || srcX >= src.Bounds().Dx() {
				continue
			}
			dst.Pix[yy*dst.Stride+xx] = src.Pix[srcY*src.Stride+srcX]
		}
	}
	return dst
}
