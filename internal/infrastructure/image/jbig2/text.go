package jbig2

import (
	"encoding/binary"
	"fmt"
	stdimage "image"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

type textRegion struct {
	info         regionInfo
	payload      []byte
	atPixels     []adaptiveTemplatePixel
	huff         bool
	refine       bool
	transposed   bool
	defaultPix   bool
	logStrips    uint
	refCorner    byte
	combOp       byte
	template     byte
	sOffset      int
	numInstances int
	huffFS       byte
	huffDS       byte
	huffDT       byte
	huffRDW      byte
	huffRDH      byte
	huffRDX      byte
	huffRDY      byte
	huffRSize    byte
}

func parseTextRegionSegment(data []byte) (textRegion, error) {
	info, offset, err := parseRegionInfo(data)
	if err != nil {
		return textRegion{}, err
	}
	if len(data)-offset < 2 {
		return textRegion{}, fmt.Errorf("truncated text region flags")
	}

	flags := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	region := textRegion{
		info:       info,
		huff:       flags&0x0001 != 0,
		refine:     flags&0x0002 != 0,
		logStrips:  uint((flags >> 2) & 0x03),
		refCorner:  byte((flags >> 4) & 0x03),
		transposed: flags&0x0040 != 0,
		combOp:     byte((flags >> 7) & 0x03),
		defaultPix: flags&0x0200 != 0,
		sOffset:    int((flags >> 10) & 0x1f),
		template:   byte((flags >> 15) & 0x01),
	}
	if region.sOffset&0x10 != 0 {
		region.sOffset |= ^0x0f
	}
	if region.huff {
		if len(data)-offset < 2 {
			return textRegion{}, fmt.Errorf("truncated text region Huffman flags")
		}
		huffFlags := binary.BigEndian.Uint16(data[offset : offset+2])
		region.huffFS = byte(huffFlags & 0x03)
		region.huffDS = byte((huffFlags >> 2) & 0x03)
		region.huffDT = byte((huffFlags >> 4) & 0x03)
		region.huffRDW = byte((huffFlags >> 6) & 0x03)
		region.huffRDH = byte((huffFlags >> 8) & 0x03)
		region.huffRDX = byte((huffFlags >> 10) & 0x03)
		region.huffRDY = byte((huffFlags >> 12) & 0x03)
		region.huffRSize = byte((huffFlags >> 14) & 0x01)
		offset += 2
	}
	if region.refine && region.template == 0 {
		atPixels, nextOffset, err := parseAdaptiveTemplatePixels(data, offset, 2)
		if err != nil {
			return textRegion{}, err
		}
		region.atPixels = atPixels
		offset = nextOffset
	}
	if len(data)-offset < 4 {
		return textRegion{}, fmt.Errorf("truncated text region instance count")
	}
	numInstances := binary.BigEndian.Uint32(data[offset : offset+4])
	if numInstances > uint32(maxInt()) {
		return textRegion{}, fmt.Errorf("text region instance count too large")
	}
	region.numInstances = int(numInstances)
	region.payload = data[offset+4:]
	return region, nil
}

func decodeTextRegionSegment(segment segmentHeader, dictionaries map[uint32]symbolDictionary) (regionInfo, *stdimage.Gray, error) {
	return decodeTextRegionSegmentWithCodeTables(segment, dictionaries, nil)
}

func decodeTextRegionSegmentWithCodeTables(segment segmentHeader, dictionaries map[uint32]symbolDictionary, codeTables map[uint32][]huffmanTableEntry) (regionInfo, *stdimage.Gray, error) {
	region, err := parseTextRegionSegment(segment.data)
	if err != nil {
		return regionInfo{}, nil, errors.Invalid("jbig2_text_region", err)
	}
	symbols, _ := collectReferencedSymbols(segment, dictionaries)
	referencedCodeTables := collectReferencedCodeTables(segment, codeTables)
	if !region.huff {
		for _, ref := range segment.referredToSegmentNumbers {
			if _, ok := dictionaries[ref]; !ok {
				return regionInfo{}, nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing referenced symbol dictionary %d", ref))
			}
		}
	}
	if region.numInstances > 0 && len(symbols) == 0 {
		return regionInfo{}, nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing referenced symbol dictionary"))
	}
	if region.huff {
		img, err := decodeHuffmanTextRegionWithCodeTables(region, symbols, referencedCodeTables)
		if err != nil {
			return regionInfo{}, nil, err
		}
		return region.info, img, nil
	}

	img, err := decodeArithmeticTextRegion(region, symbols)
	if err != nil {
		return regionInfo{}, nil, err
	}
	return region.info, img, nil
}

type huffmanTextTables struct {
	firstS    []huffmanTableEntry
	deltaS    []huffmanTableEntry
	deltaT    []huffmanTableEntry
	refineDW  []huffmanTableEntry
	refineDH  []huffmanTableEntry
	refineDX  []huffmanTableEntry
	refineDY  []huffmanTableEntry
	refineLen []huffmanTableEntry
}

func decodeHuffmanTextRegion(region textRegion, symbols []*stdimage.Gray) (*stdimage.Gray, error) {
	return decodeHuffmanTextRegionWithCodeTables(region, symbols, nil)
}

func decodeHuffmanTextRegionWithCodeTables(region textRegion, symbols []*stdimage.Gray, codeTables [][]huffmanTableEntry) (*stdimage.Gray, error) {
	tables, err := selectHuffmanTextTables(region, codeTables)
	if err != nil {
		return nil, err
	}
	decoder := newHuffmanDecoder(region.payload)
	symCodeTab, err := buildTextRegionSymbolCodeTable(decoder, len(symbols))
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	decoder.reset()
	return decodeHuffmanTextRegionWithState(region, symbols, decoder, symCodeTab, symbolCodeLength(len(symbols), true), tables)
}

func selectHuffmanTextTables(region textRegion, codeTables [][]huffmanTableEntry) (huffmanTextTables, error) {
	var tables huffmanTextTables
	tableIndex := 0
	nextCodeTable := func(name string) ([]huffmanTableEntry, error) {
		if tableIndex >= len(codeTables) {
			return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing custom Huffman %s table", name))
		}
		table := codeTables[tableIndex]
		tableIndex++
		return table, nil
	}

	switch region.huffFS {
	case 0:
		tables.firstS = huffmanTableF
	case 1:
		tables.firstS = huffmanTableG
	default:
		table, err := nextCodeTable("first-S")
		if err != nil {
			return tables, err
		}
		tables.firstS = table
	}
	switch region.huffDS {
	case 0:
		tables.deltaS = huffmanTableH
	case 1:
		tables.deltaS = huffmanTableI
	case 2:
		tables.deltaS = huffmanTableJ
	default:
		table, err := nextCodeTable("delta-S")
		if err != nil {
			return tables, err
		}
		tables.deltaS = table
	}
	switch region.huffDT {
	case 0:
		tables.deltaT = huffmanTableK
	case 1:
		tables.deltaT = huffmanTableL
	case 2:
		tables.deltaT = huffmanTableM
	default:
		table, err := nextCodeTable("delta-T")
		if err != nil {
			return tables, err
		}
		tables.deltaT = table
	}
	if !region.refine {
		return tables, nil
	}
	switch region.huffRDW {
	case 0:
		tables.refineDW = huffmanTableN
	case 1:
		tables.refineDW = huffmanTableO
	default:
		table, err := nextCodeTable("refinement-width")
		if err != nil {
			return tables, err
		}
		tables.refineDW = table
	}
	switch region.huffRDH {
	case 0:
		tables.refineDH = huffmanTableN
	case 1:
		tables.refineDH = huffmanTableO
	default:
		table, err := nextCodeTable("refinement-height")
		if err != nil {
			return tables, err
		}
		tables.refineDH = table
	}
	switch region.huffRDX {
	case 0:
		tables.refineDX = huffmanTableN
	case 1:
		tables.refineDX = huffmanTableO
	default:
		table, err := nextCodeTable("refinement-x")
		if err != nil {
			return tables, err
		}
		tables.refineDX = table
	}
	switch region.huffRDY {
	case 0:
		tables.refineDY = huffmanTableN
	case 1:
		tables.refineDY = huffmanTableO
	default:
		table, err := nextCodeTable("refinement-y")
		if err != nil {
			return tables, err
		}
		tables.refineDY = table
	}
	if region.huffRSize != 0 {
		table, err := nextCodeTable("refinement-size")
		if err != nil {
			return tables, err
		}
		tables.refineLen = table
	} else {
		tables.refineLen = huffmanTableA
	}
	return tables, nil
}

func buildTextRegionSymbolCodeTable(decoder *huffmanDecoder, symbolCount int) ([]huffmanTableEntry, error) {
	runLengthTab := make([]huffmanTableEntry, 36)
	for i := 0; i < 32; i++ {
		prefixLen, err := decoder.readBits(4)
		if err != nil {
			return nil, err
		}
		runLengthTab[i] = huffmanTableEntry{val: i, prefixLen: uint(prefixLen)}
	}
	for i, entry := range []huffmanTableEntry{
		{val: 0x103, rangeLen: 2},
		{val: 0x203, rangeLen: 3},
		{val: 0x20b, rangeLen: 7},
	} {
		prefixLen, err := decoder.readBits(4)
		if err != nil {
			return nil, err
		}
		entry.prefixLen = uint(prefixLen)
		runLengthTab[32+i] = entry
	}
	runLengthTab[35] = huffmanTableEntry{rangeLen: jbig2HuffmanEOT}
	runLengthBuilt, err := buildHuffmanTable(runLengthTab, 35)
	if err != nil {
		return nil, err
	}

	symCodeTab := make([]huffmanTableEntry, symbolCount+1)
	for i := 0; i < symbolCount; i++ {
		symCodeTab[i] = huffmanTableEntry{val: i}
	}
	i := 0
	for i < symbolCount {
		run, ok, err := decoder.decodeInt(runLengthBuilt)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("symbol-code prefix run returned OOB")
		}
		switch {
		case run > 0x200:
			for count := run - 0x200; count > 0 && i < symbolCount; count-- {
				symCodeTab[i].prefixLen = 0
				i++
			}
		case run > 0x100:
			if i == 0 {
				symCodeTab[i].prefixLen = 0
				i++
			}
			for count := run - 0x100; count > 0 && i < symbolCount; count-- {
				symCodeTab[i].prefixLen = symCodeTab[i-1].prefixLen
				i++
			}
		default:
			if run < 0 {
				return nil, fmt.Errorf("negative symbol-code prefix length %d", run)
			}
			symCodeTab[i].prefixLen = uint(run)
			i++
		}
	}
	symCodeTab[symbolCount] = huffmanTableEntry{rangeLen: jbig2HuffmanEOT}
	return buildHuffmanTable(symCodeTab, symbolCount)
}

func decodeHuffmanTextRegionWithState(region textRegion, symbols []*stdimage.Gray, decoder *huffmanDecoder, symCodeTab []huffmanTableEntry, symCodeLen uint, tables huffmanTextTables) (*stdimage.Gray, error) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, region.info.width, region.info.height))
	defaultPixel := uint8(0xff)
	if region.defaultPix {
		defaultPixel = 0x00
	}
	for i := range img.Pix {
		img.Pix[i] = defaultPixel
	}
	if region.numInstances == 0 {
		return img, nil
	}

	strips := 1 << region.logStrips
	t, ok, err := decoder.decodeInt(tables.deltaT)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing initial text T value"))
	}
	t *= -strips

	instance := 0
	sFirst := 0
	for instance < region.numInstances {
		dt, ok, err := decoder.decodeInt(tables.deltaT)
		if err != nil {
			return nil, errors.Invalid("jbig2_text_region", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing text delta T"))
		}
		t += dt * strips

		ds, ok, err := decoder.decodeInt(tables.firstS)
		if err != nil {
			return nil, errors.Invalid("jbig2_text_region", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing first text S value"))
		}
		sFirst += ds
		s := sFirst

		for instance < region.numInstances {
			tt := t
			if strips != 1 {
				stripOffset, err := decoder.readBits(region.logStrips)
				if err != nil {
					return nil, errors.Invalid("jbig2_text_region", err)
				}
				tt += int(stripOffset)
			}

			var symID int
			if symCodeTab != nil {
				var ok bool
				symID, ok, err = decoder.decodeInt(symCodeTab)
				if err != nil {
					return nil, errors.Invalid("jbig2_text_region", err)
				}
				if !ok {
					return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing text symbol id"))
				}
			} else {
				rawSymID, err := decoder.readBits(symCodeLen)
				if err != nil {
					return nil, errors.Invalid("jbig2_text_region", err)
				}
				symID = int(rawSymID)
			}
			if symID < 0 || symID >= len(symbols) || symbols[symID] == nil {
				return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("invalid text symbol id %d", symID))
			}

			symbolBitmap := symbols[symID]
			if region.refine {
				refineFlag, err := decoder.readBit()
				if err != nil {
					return nil, errors.Invalid("jbig2_text_region", err)
				}
				if refineFlag != 0 {
					symbolBitmap, err = decodeHuffmanTextRefinementSymbol(region, symbolBitmap, decoder, tables)
					if err != nil {
						return nil, err
					}
				}
			}

			s += composeTextSymbol(img, symbolBitmap, region, s, tt)
			instance++

			ds, ok, err = decoder.decodeInt(tables.deltaS)
			if err != nil {
				return nil, errors.Invalid("jbig2_text_region", err)
			}
			if !ok {
				break
			}
			s += region.sOffset + ds
		}
	}
	return img, nil
}

func decodeHuffmanTextRefinementSymbol(region textRegion, symbol *stdimage.Gray, decoder *huffmanDecoder, tables huffmanTextTables) (*stdimage.Gray, error) {
	rdw, ok, err := decoder.decodeInt(tables.refineDW)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing Huffman refinement width delta"))
	}
	rdh, ok, err := decoder.decodeInt(tables.refineDH)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing Huffman refinement height delta"))
	}
	rdx, ok, err := decoder.decodeInt(tables.refineDX)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing Huffman refinement x delta"))
	}
	rdy, ok, err := decoder.decodeInt(tables.refineDY)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing Huffman refinement y delta"))
	}
	bitmapSize, ok, err := decoder.decodeInt(tables.refineLen)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok || bitmapSize < 0 {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("invalid Huffman refinement bitmap size %d", bitmapSize))
	}

	width := symbol.Bounds().Dx() + rdw
	height := symbol.Bounds().Dy() + rdh
	if width <= 0 || height <= 0 {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("invalid refinement symbol size: %dx%d", width, height))
	}

	decoder.reset()
	arithmeticDecoder := NewArithmeticDecoder(decoder.remainingBytes())
	refinementStats := make([]uint8, refinementRegionStatsSize(region.template))
	refinement := genericRefinementRegion{
		info: regionInfo{
			width:  width,
			height: height,
		},
		ref:      symbol,
		refDX:    jbig2HalfDelta(rdw) + rdx,
		refDY:    jbig2HalfDelta(rdh) + rdy,
		atPixels: region.atPixels,
		template: region.template,
	}
	refined, err := decodeGenericRefinementRegionWithState(refinement, arithmeticDecoder, refinementStats)
	if err != nil {
		return nil, err
	}
	if err := decoder.advanceBytes(arithmeticDecoder.offset); err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	_ = bitmapSize
	return refined, nil
}

func decodeArithmeticTextRegion(region textRegion, symbols []*stdimage.Gray) (*stdimage.Gray, error) {
	symCodeLen := symbolCodeLength(len(symbols), false)
	intStats, err := newArithmeticIntStats(symCodeLen)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	refinementStats := make([]uint8, refinementRegionStatsSize(region.template))
	decoder := NewArithmeticDecoder(region.payload)
	return decodeArithmeticTextRegionWithState(region, symbols, symCodeLen, decoder, intStats, refinementStats)
}

func decodeArithmeticTextRegionWithState(region textRegion, symbols []*stdimage.Gray, symCodeLen uint, decoder *ArithmeticDecoder, intStats *arithmeticIntStats, refinementStats []uint8) (*stdimage.Gray, error) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, region.info.width, region.info.height))
	defaultPixel := uint8(0xff)
	if region.defaultPix {
		defaultPixel = 0x00
	}
	for i := range img.Pix {
		img.Pix[i] = defaultPixel
	}
	if region.numInstances == 0 {
		return img, nil
	}
	strips := 1 << region.logStrips

	t, ok, err := decoder.DecodeInt(intStats.iadt)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing initial text T value"))
	}
	t *= -strips

	instance := 0
	sFirst := 0
	for instance < region.numInstances {
		dt, ok, err := decoder.DecodeInt(intStats.iadt)
		if err != nil {
			return nil, errors.Invalid("jbig2_text_region", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing text delta T"))
		}
		t += dt * strips

		ds, ok, err := decoder.DecodeInt(intStats.iafs)
		if err != nil {
			return nil, errors.Invalid("jbig2_text_region", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing first text S value"))
		}
		sFirst += ds
		s := sFirst

		for instance < region.numInstances {
			tt := t
			if strips != 1 {
				dt, ok, err := decoder.DecodeInt(intStats.iait)
				if err != nil {
					return nil, errors.Invalid("jbig2_text_region", err)
				}
				if !ok {
					return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing text strip offset"))
				}
				tt += dt
			}

			symID, err := decoder.DecodeIAID(symCodeLen, intStats.iaid)
			if err != nil {
				return nil, errors.Invalid("jbig2_text_region", err)
			}
			if int(symID) >= len(symbols) || symbols[symID] == nil {
				return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("invalid text symbol id %d", symID))
			}

			symbolBitmap := symbols[symID]
			if region.refine {
				ri, ok, err := decoder.DecodeInt(intStats.iari)
				if err != nil {
					return nil, errors.Invalid("jbig2_text_region", err)
				}
				if !ok {
					return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing text refinement flag"))
				}
				if ri != 0 {
					var err error
					symbolBitmap, err = decodeTextRefinementSymbol(region, symbolBitmap, decoder, intStats, refinementStats)
					if err != nil {
						return nil, err
					}
				}
			}

			s += composeTextSymbol(img, symbolBitmap, region, s, tt)
			instance++

			ds, ok, err = decoder.DecodeInt(intStats.iads)
			if err != nil {
				return nil, errors.Invalid("jbig2_text_region", err)
			}
			if !ok {
				break
			}
			s += region.sOffset + ds
		}
	}
	return img, nil
}

func decodeTextRefinementSymbol(region textRegion, symbol *stdimage.Gray, decoder *ArithmeticDecoder, intStats *arithmeticIntStats, refinementStats []uint8) (*stdimage.Gray, error) {
	rdw, ok, err := decoder.DecodeInt(intStats.iardw)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing refinement width delta"))
	}
	rdh, ok, err := decoder.DecodeInt(intStats.iardh)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing refinement height delta"))
	}
	rdx, ok, err := decoder.DecodeInt(intStats.iardx)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing refinement x delta"))
	}
	rdy, ok, err := decoder.DecodeInt(intStats.iardy)
	if err != nil {
		return nil, errors.Invalid("jbig2_text_region", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("missing refinement y delta"))
	}

	width := symbol.Bounds().Dx() + rdw
	height := symbol.Bounds().Dy() + rdh
	if width <= 0 || height <= 0 {
		return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("invalid refinement symbol size: %dx%d", width, height))
	}
	refDX := jbig2HalfDelta(rdw) + rdx
	refDY := jbig2HalfDelta(rdh) + rdy
	refinement := genericRefinementRegion{
		info: regionInfo{
			width:  width,
			height: height,
		},
		ref:      symbol,
		refDX:    refDX,
		refDY:    refDY,
		atPixels: region.atPixels,
		template: region.template,
	}
	return decodeGenericRefinementRegionWithState(refinement, decoder, refinementStats)
}

func composeTextSymbol(dst, symbol *stdimage.Gray, region textRegion, s, t int) int {
	bw := symbol.Bounds().Dx() - 1
	bh := symbol.Bounds().Dy() - 1
	x := s
	y := t
	if region.transposed {
		if region.refCorner == 2 || region.refCorner == 3 {
			x = t - bw
		} else {
			x = t
		}
		y = s
	} else if region.refCorner == 0 || region.refCorner == 2 {
		y = t - bh
	}
	composeBilevelBitmap(dst, symbol, x, y, region.combOp)
	if region.transposed {
		return bh
	}
	return bw
}

func jbig2HalfDelta(value int) int {
	if value >= 0 {
		return value / 2
	}
	return (value - 1) / 2
}
