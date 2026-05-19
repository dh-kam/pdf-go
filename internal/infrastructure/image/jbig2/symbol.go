package jbig2

import (
	"encoding/binary"
	"fmt"
	stdimage "image"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

type symbolDictionary struct {
	symbols         []*stdimage.Gray
	genericStats    []uint8
	refinementStats []uint8
}

type symbolDictionarySegment struct {
	payload            []byte
	atPixels           []adaptiveTemplatePixel
	refinementATPixels []adaptiveTemplatePixel
	huff               bool
	refAgg             bool
	contextUsed        bool
	contextRetained    bool
	sdTemplate         byte
	sdrTemplate        byte
	huffDH             byte
	huffDW             byte
	huffBMSize         byte
	huffAggInst        byte
	numExSyms          int
	numNewSyms         int
}

type arithmeticIntStats struct {
	iadh  []uint8
	iadw  []uint8
	iaex  []uint8
	iaai  []uint8
	iadt  []uint8
	iait  []uint8
	iafs  []uint8
	iads  []uint8
	iardx []uint8
	iardy []uint8
	iardw []uint8
	iardh []uint8
	iari  []uint8
	iaid  []uint8
}

func parseSymbolDictionarySegment(data []byte) (symbolDictionarySegment, error) {
	if len(data) < 2 {
		return symbolDictionarySegment{}, fmt.Errorf("truncated symbol dictionary flags")
	}

	flags := binary.BigEndian.Uint16(data[0:2])
	offset := 2
	segment := symbolDictionarySegment{
		huff:            flags&0x0001 != 0,
		refAgg:          flags&0x0002 != 0,
		contextUsed:     flags&0x0100 != 0,
		contextRetained: flags&0x0200 != 0,
		sdTemplate:      byte((flags >> 10) & 0x03),
		sdrTemplate:     byte((flags >> 12) & 0x01),
		huffDH:          byte((flags >> 2) & 0x03),
		huffDW:          byte((flags >> 4) & 0x03),
		huffBMSize:      byte((flags >> 6) & 0x01),
		huffAggInst:     byte((flags >> 7) & 0x01),
	}

	if !segment.huff {
		atPixelCount := 1
		if segment.sdTemplate == 0 {
			atPixelCount = 4
		}
		atPixels, nextOffset, err := parseAdaptiveTemplatePixels(data, offset, atPixelCount)
		if err != nil {
			return symbolDictionarySegment{}, err
		}
		segment.atPixels = atPixels
		offset = nextOffset
	}
	if segment.refAgg && segment.sdrTemplate == 0 {
		atPixels, nextOffset, err := parseAdaptiveTemplatePixels(data, offset, 2)
		if err != nil {
			return symbolDictionarySegment{}, err
		}
		segment.refinementATPixels = atPixels
		offset = nextOffset
	}
	if len(data)-offset < 8 {
		return symbolDictionarySegment{}, fmt.Errorf("truncated symbol dictionary symbol counts")
	}

	numExSyms := binary.BigEndian.Uint32(data[offset : offset+4])
	numNewSyms := binary.BigEndian.Uint32(data[offset+4 : offset+8])
	if numExSyms > uint32(maxInt()) || numNewSyms > uint32(maxInt()) {
		return symbolDictionarySegment{}, fmt.Errorf("symbol dictionary count too large")
	}
	segment.numExSyms = int(numExSyms)
	segment.numNewSyms = int(numNewSyms)
	segment.payload = data[offset+8:]
	return segment, nil
}

func decodeSymbolDictionarySegment(segment segmentHeader, dictionaries map[uint32]symbolDictionary) (symbolDictionary, error) {
	return decodeSymbolDictionarySegmentWithCodeTables(segment, dictionaries, nil)
}

func decodeSymbolDictionarySegmentWithCodeTables(segment segmentHeader, dictionaries map[uint32]symbolDictionary, codeTables map[uint32][]huffmanTableEntry) (symbolDictionary, error) {
	dictSegment, err := parseSymbolDictionarySegment(segment.data)
	if err != nil {
		return symbolDictionary{}, errors.Invalid("jbig2_symbol_dictionary", err)
	}

	inputSymbols, inputDict := collectReferencedSymbols(segment, dictionaries)
	referencedCodeTables := collectReferencedCodeTables(segment, codeTables)
	if !dictSegment.huff {
		for _, ref := range segment.referredToSegmentNumbers {
			if _, ok := dictionaries[ref]; !ok {
				return symbolDictionary{}, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing referenced symbol dictionary %d", ref))
			}
		}
	}
	if dictSegment.contextUsed && inputDict == nil {
		return symbolDictionary{}, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing context source symbol dictionary"))
	}
	symCodeLen := symbolCodeLength(len(inputSymbols)+dictSegment.numNewSyms, dictSegment.huff)
	if dictSegment.huff {
		allSymbols := make([]*stdimage.Gray, 0, len(inputSymbols)+dictSegment.numNewSyms)
		allSymbols = append(allSymbols, inputSymbols...)

		decoder := newHuffmanDecoder(dictSegment.payload)
		if dictSegment.numNewSyms > 0 {
			newSymbols, err := decodeHuffmanSymbolBitmaps(decoder, dictSegment, inputSymbols, symCodeLen, referencedCodeTables)
			if err != nil {
				return symbolDictionary{}, err
			}
			allSymbols = append(allSymbols, newSymbols...)
		}

		exported, err := decodeHuffmanSymbolDictionaryExports(decoder, dictSegment, allSymbols)
		if err != nil {
			return symbolDictionary{}, err
		}
		return symbolDictionary{symbols: exported}, nil
	}

	intStats, err := newArithmeticIntStats(symCodeLen)
	if err != nil {
		return symbolDictionary{}, errors.Invalid("jbig2_symbol_dictionary", err)
	}

	allSymbols := make([]*stdimage.Gray, 0, len(inputSymbols)+dictSegment.numNewSyms)
	allSymbols = append(allSymbols, inputSymbols...)

	var genericStats []uint8
	var refinementStats []uint8
	if !dictSegment.huff {
		genericStats = make([]uint8, genericRegionStatsSize(dictSegment.sdTemplate))
		if dictSegment.contextUsed && inputDict != nil && len(inputDict.genericStats) == len(genericStats) {
			copy(genericStats, inputDict.genericStats)
		}
		refinementStats = make([]uint8, refinementRegionStatsSize(dictSegment.sdrTemplate))
		if dictSegment.contextUsed && inputDict != nil && len(inputDict.refinementStats) == len(refinementStats) {
			copy(refinementStats, inputDict.refinementStats)
		}
	}

	decoder := NewArithmeticDecoder(dictSegment.payload)
	if dictSegment.numNewSyms > 0 {
		newSymbols, err := decodeArithmeticSymbolBitmaps(decoder, dictSegment, inputSymbols, symCodeLen, genericStats, refinementStats, intStats)
		if err != nil {
			return symbolDictionary{}, err
		}
		allSymbols = append(allSymbols, newSymbols...)
	}

	exported, err := decodeSymbolDictionaryExports(decoder, dictSegment, allSymbols, intStats)
	if err != nil {
		return symbolDictionary{}, err
	}

	dict := symbolDictionary{symbols: exported}
	if dictSegment.contextRetained {
		dict.genericStats = cloneBytes(genericStats)
		dict.refinementStats = cloneBytes(refinementStats)
	}
	return dict, nil
}

func collectReferencedSymbols(segment segmentHeader, dictionaries map[uint32]symbolDictionary) ([]*stdimage.Gray, *symbolDictionary) {
	var symbols []*stdimage.Gray
	var lastDict *symbolDictionary
	for _, ref := range segment.referredToSegmentNumbers {
		dict, ok := dictionaries[ref]
		if !ok {
			continue
		}
		lastDict = &dict
		symbols = append(symbols, dict.symbols...)
	}
	return symbols, lastDict
}

type huffmanSymbolTables struct {
	deltaHeight []huffmanTableEntry
	deltaWidth  []huffmanTableEntry
	bitmapSize  []huffmanTableEntry
	aggInst     []huffmanTableEntry
}

func selectHuffmanSymbolTables(segment symbolDictionarySegment, codeTables [][]huffmanTableEntry) (huffmanSymbolTables, error) {
	var tables huffmanSymbolTables
	tableIndex := 0
	nextCodeTable := func(name string) ([]huffmanTableEntry, error) {
		if tableIndex >= len(codeTables) {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing custom Huffman %s table", name))
		}
		table := codeTables[tableIndex]
		tableIndex++
		return table, nil
	}

	switch segment.huffDH {
	case 0:
		tables.deltaHeight = huffmanTableD
	case 1:
		tables.deltaHeight = huffmanTableE
	default:
		table, err := nextCodeTable("delta-height")
		if err != nil {
			return tables, err
		}
		tables.deltaHeight = table
	}
	switch segment.huffDW {
	case 0:
		tables.deltaWidth = huffmanTableB
	case 1:
		tables.deltaWidth = huffmanTableC
	default:
		table, err := nextCodeTable("delta-width")
		if err != nil {
			return tables, err
		}
		tables.deltaWidth = table
	}
	if segment.huffBMSize != 0 {
		table, err := nextCodeTable("bitmap-size")
		if err != nil {
			return tables, err
		}
		tables.bitmapSize = table
	} else {
		tables.bitmapSize = huffmanTableA
	}
	if segment.huffAggInst != 0 {
		table, err := nextCodeTable("aggregate-instance")
		if err != nil {
			return tables, err
		}
		tables.aggInst = table
	} else {
		tables.aggInst = huffmanTableA
	}
	return tables, nil
}

func decodeHuffmanSymbolBitmaps(decoder *huffmanDecoder, segment symbolDictionarySegment, inputSymbols []*stdimage.Gray, symCodeLen uint, codeTables [][]huffmanTableEntry) ([]*stdimage.Gray, error) {
	tables, err := selectHuffmanSymbolTables(segment, codeTables)
	if err != nil {
		return nil, err
	}

	symbols := make([]*stdimage.Gray, 0, segment.numNewSyms)
	availableSymbols := make([]*stdimage.Gray, 0, len(inputSymbols)+segment.numNewSyms)
	availableSymbols = append(availableSymbols, inputSymbols...)
	symHeight := 0

	for len(symbols) < segment.numNewSyms {
		dh, ok, err := decoder.decodeInt(tables.deltaHeight)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing symbol height class"))
		}
		if dh < 0 && -dh >= symHeight {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol height delta %d", dh))
		}
		symHeight += dh
		if symHeight <= 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol height %d", symHeight))
		}

		symWidth := 0
		classSymbols := 0
		classWidths := make([]int, 0)
		totalWidth := 0
		for {
			dw, ok, err := decoder.decodeInt(tables.deltaWidth)
			if err != nil {
				return nil, errors.Invalid("jbig2_symbol_dictionary", err)
			}
			if !ok {
				break
			}
			if segment.refAgg {
				if len(symbols) >= segment.numNewSyms {
					return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("too many symbols in Huffman symbol dictionary"))
				}
			} else if len(symbols)+classSymbols >= segment.numNewSyms {
				return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("too many symbols in Huffman symbol dictionary"))
			}
			if dw < 0 && -dw >= symWidth {
				return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol width delta %d", dw))
			}
			symWidth += dw
			if symWidth <= 0 {
				return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol width %d", symWidth))
			}

			if segment.refAgg {
				img, err := decodeHuffmanRefinementAggregateSymbol(decoder, segment, symWidth, symHeight, availableSymbols, symCodeLen, tables)
				if err != nil {
					return nil, err
				}
				symbols = append(symbols, img)
				availableSymbols = append(availableSymbols, img)
			} else {
				classWidths = append(classWidths, symWidth)
				totalWidth += symWidth
			}
			classSymbols++
		}
		if classSymbols == 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("empty symbol height class"))
		}
		if !segment.refAgg {
			classBitmaps, err := decodeHuffmanCollectiveSymbolClass(decoder, tables, classWidths, totalWidth, symHeight)
			if err != nil {
				return nil, err
			}
			symbols = append(symbols, classBitmaps...)
			availableSymbols = append(availableSymbols, classBitmaps...)
		}
	}
	return symbols, nil
}

func decodeHuffmanRefinementAggregateSymbol(decoder *huffmanDecoder, segment symbolDictionarySegment, symWidth, symHeight int, symbols []*stdimage.Gray, symCodeLen uint, tables huffmanSymbolTables) (*stdimage.Gray, error) {
	refAggNum, ok, err := decoder.decodeInt(tables.aggInst)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	if !ok || refAggNum <= 0 {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid refinement aggregate instance count %d", refAggNum))
	}
	if refAggNum == 1 {
		if symCodeLen > 32 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("symbol code length too large: %d", symCodeLen))
		}
		symID, err := decoder.readBits(symCodeLen)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if int(symID) >= len(symbols) || symbols[symID] == nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid refinement symbol id %d", symID))
		}
		refDX, ok, err := decoder.decodeInt(huffmanTableO)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing refinement x delta"))
		}
		refDY, ok, err := decoder.decodeInt(huffmanTableO)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing refinement y delta"))
		}
		bitmapSize, ok, err := decoder.decodeInt(huffmanTableA)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok || bitmapSize < 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid refinement bitmap size %d", bitmapSize))
		}

		decoder.reset()
		arithmeticDecoder := NewArithmeticDecoder(decoder.remainingBytes())
		refinementStats := make([]uint8, refinementRegionStatsSize(segment.sdrTemplate))
		refinement := genericRefinementRegion{
			info: regionInfo{
				width:  symWidth,
				height: symHeight,
			},
			ref:      symbols[symID],
			refDX:    refDX,
			refDY:    refDY,
			atPixels: segment.refinementATPixels,
			template: segment.sdrTemplate,
		}
		img, err := decodeGenericRefinementRegionWithState(refinement, arithmeticDecoder, refinementStats)
		if err != nil {
			return nil, err
		}
		if err := decoder.advanceBytes(arithmeticDecoder.offset); err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		_ = bitmapSize
		return img, nil
	}

	region := textRegion{
		info: regionInfo{
			width:  symWidth,
			height: symHeight,
		},
		atPixels:     segment.refinementATPixels,
		huff:         true,
		refine:       true,
		refCorner:    1,
		combOp:       jbig2CombineOr,
		template:     segment.sdrTemplate,
		numInstances: refAggNum,
	}
	textTables := huffmanTextTables{
		firstS:    huffmanTableF,
		deltaS:    huffmanTableH,
		deltaT:    huffmanTableK,
		refineDW:  huffmanTableO,
		refineDH:  huffmanTableO,
		refineDX:  huffmanTableO,
		refineDY:  huffmanTableO,
		refineLen: huffmanTableA,
	}
	return decodeHuffmanTextRegionWithState(region, symbols, decoder, nil, symCodeLen, textTables)
}

func decodeHuffmanCollectiveSymbolClass(decoder *huffmanDecoder, tables huffmanSymbolTables, widths []int, totalWidth, height int) ([]*stdimage.Gray, error) {
	bitmapSize, ok, err := decoder.decodeInt(tables.bitmapSize)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	if !ok || bitmapSize < 0 {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid collective bitmap size %d", bitmapSize))
	}

	decoder.reset()
	collective, err := decodeHuffmanCollectiveBitmap(decoder, totalWidth, height, bitmapSize)
	if err != nil {
		return nil, err
	}

	symbols := make([]*stdimage.Gray, 0, len(widths))
	x := 0
	for _, width := range widths {
		symbols = append(symbols, sliceGray(collective, x, 0, width, height))
		x += width
	}
	return symbols, nil
}

func decodeHuffmanCollectiveBitmap(decoder *huffmanDecoder, width, height, bitmapSize int) (*stdimage.Gray, error) {
	img := stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	if bitmapSize == 0 {
		rowBytes := (width + 7) >> 3
		byteCount := rowBytes * height
		data := decoder.remainingBytes()
		if byteCount > len(data) {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("truncated collective bitmap: need %d bytes, have %d", byteCount, len(data)))
		}
		for y := 0; y < height; y++ {
			paintBilevelRow(img, y, data[y*rowBytes:y*rowBytes+rowBytes])
		}
		if err := decoder.advanceBytes(byteCount); err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		return img, nil
	}

	data := decoder.remainingBytes()
	if bitmapSize > len(data) {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("truncated MMR collective bitmap: need %d bytes, have %d", bitmapSize, len(data)))
	}
	mmr := NewMMRDecoder(data[:bitmapSize], width, height)
	for y := 0; y < height; y++ {
		line, err := mmr.DecodeLine()
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		paintBilevelRow(img, y, line)
	}
	if err := decoder.advanceBytes(bitmapSize); err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	return img, nil
}

func decodeHuffmanSymbolDictionaryExports(decoder *huffmanDecoder, segment symbolDictionarySegment, symbols []*stdimage.Gray) ([]*stdimage.Gray, error) {
	if len(symbols) == 0 {
		if segment.numExSyms != 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("too few exported symbols"))
		}
		return nil, nil
	}

	i := 0
	exported := make([]*stdimage.Gray, 0, segment.numExSyms)
	export := false
	for i < len(symbols) {
		run, ok, err := decoder.decodeInt(huffmanTableA)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok || run < 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid export run %d", run))
		}
		if i+run > len(symbols) || (export && len(exported)+run > segment.numExSyms) {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("export run exceeds symbol dictionary"))
		}
		if export {
			for count := 0; count < run; count++ {
				exported = append(exported, cloneGray(symbols[i]))
				i++
			}
		} else {
			i += run
		}
		export = !export
	}
	if len(exported) != segment.numExSyms {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("exported %d symbols, want %d", len(exported), segment.numExSyms))
	}
	return exported, nil
}

func decodeArithmeticSymbolBitmaps(decoder *ArithmeticDecoder, segment symbolDictionarySegment, inputSymbols []*stdimage.Gray, symCodeLen uint, genericStats, refinementStats []uint8, intStats *arithmeticIntStats) ([]*stdimage.Gray, error) {
	symbols := make([]*stdimage.Gray, 0, segment.numNewSyms)
	availableSymbols := make([]*stdimage.Gray, 0, len(inputSymbols)+segment.numNewSyms)
	availableSymbols = append(availableSymbols, inputSymbols...)
	symHeight := 0

	for len(symbols) < segment.numNewSyms {
		dh, ok, err := decoder.DecodeInt(intStats.iadh)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing symbol height class"))
		}
		if dh < 0 && -dh >= symHeight {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol height delta %d", dh))
		}
		symHeight += dh
		if symHeight <= 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol height %d", symHeight))
		}

		symWidth := 0
		classSymbols := 0
		for len(symbols) < segment.numNewSyms {
			dw, ok, err := decoder.DecodeInt(intStats.iadw)
			if err != nil {
				return nil, errors.Invalid("jbig2_symbol_dictionary", err)
			}
			if !ok {
				break
			}
			if dw < 0 && -dw >= symWidth {
				return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol width delta %d", dw))
			}
			symWidth += dw
			if symWidth <= 0 {
				return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid symbol width %d", symWidth))
			}

			var img *stdimage.Gray
			if segment.refAgg {
				img, err = decodeArithmeticRefinementAggregateSymbol(decoder, segment, symWidth, symHeight, availableSymbols, symCodeLen, intStats, refinementStats)
				if err != nil {
					return nil, err
				}
			} else {
				img = stdimage.NewGray(stdimage.Rect(0, 0, symWidth, symHeight))
				for i := range img.Pix {
					img.Pix[i] = 0xff
				}
				region := genericRegion{
					info: regionInfo{
						width:  symWidth,
						height: symHeight,
					},
					atPixels: segment.atPixels,
					template: segment.sdTemplate,
				}
				if _, err := decodeArithmeticGenericRegionWithState(region, img, decoder, genericStats); err != nil {
					return nil, errors.Invalid("jbig2_symbol_dictionary", err)
				}
			}
			symbols = append(symbols, img)
			availableSymbols = append(availableSymbols, img)
			classSymbols++
		}
		if classSymbols == 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("empty symbol height class"))
		}
	}
	return symbols, nil
}

func decodeArithmeticRefinementAggregateSymbol(decoder *ArithmeticDecoder, segment symbolDictionarySegment, symWidth, symHeight int, symbols []*stdimage.Gray, symCodeLen uint, intStats *arithmeticIntStats, refinementStats []uint8) (*stdimage.Gray, error) {
	refAggNum, ok, err := decoder.DecodeInt(intStats.iaai)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	if !ok || refAggNum <= 0 {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid refinement aggregate instance count %d", refAggNum))
	}
	if refAggNum == 1 {
		return decodeArithmeticSingleRefinementSymbol(decoder, segment, symWidth, symHeight, symbols, symCodeLen, intStats, refinementStats)
	}

	region := textRegion{
		info: regionInfo{
			width:  symWidth,
			height: symHeight,
		},
		atPixels:     segment.refinementATPixels,
		refine:       true,
		refCorner:    1,
		combOp:       jbig2CombineOr,
		template:     segment.sdrTemplate,
		numInstances: refAggNum,
	}
	return decodeArithmeticTextRegionWithState(region, symbols, symCodeLen, decoder, intStats, refinementStats)
}

func decodeArithmeticSingleRefinementSymbol(decoder *ArithmeticDecoder, segment symbolDictionarySegment, symWidth, symHeight int, symbols []*stdimage.Gray, symCodeLen uint, intStats *arithmeticIntStats, refinementStats []uint8) (*stdimage.Gray, error) {
	symID, err := decoder.DecodeIAID(symCodeLen, intStats.iaid)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	if int(symID) >= len(symbols) || symbols[symID] == nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid refinement symbol id %d", symID))
	}
	refDX, ok, err := decoder.DecodeInt(intStats.iardx)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing refinement x delta"))
	}
	refDY, ok, err := decoder.DecodeInt(intStats.iardy)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	if !ok {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("missing refinement y delta"))
	}

	region := genericRefinementRegion{
		info: regionInfo{
			width:  symWidth,
			height: symHeight,
		},
		ref:      symbols[symID],
		refDX:    refDX,
		refDY:    refDY,
		atPixels: segment.refinementATPixels,
		template: segment.sdrTemplate,
	}
	img, err := decodeGenericRefinementRegionWithState(region, decoder, refinementStats)
	if err != nil {
		return nil, errors.Invalid("jbig2_symbol_dictionary", err)
	}
	return img, nil
}

func decodeSymbolDictionaryExports(decoder *ArithmeticDecoder, segment symbolDictionarySegment, symbols []*stdimage.Gray, intStats *arithmeticIntStats) ([]*stdimage.Gray, error) {
	if len(symbols) == 0 {
		if segment.numExSyms != 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("too few exported symbols"))
		}
		return nil, nil
	}
	return decodeSymbolDictionaryExportsFromCurrentDecoder(decoder, segment, symbols, intStats)
}

func decodeSymbolDictionaryExportsFromCurrentDecoder(decoder *ArithmeticDecoder, segment symbolDictionarySegment, symbols []*stdimage.Gray, intStats *arithmeticIntStats) ([]*stdimage.Gray, error) {
	i := 0
	exported := make([]*stdimage.Gray, 0, segment.numExSyms)
	export := false
	for i < len(symbols) {
		run, ok, err := decoder.DecodeInt(intStats.iaex)
		if err != nil {
			return nil, errors.Invalid("jbig2_symbol_dictionary", err)
		}
		if !ok || run < 0 {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("invalid export run %d", run))
		}
		if i+run > len(symbols) || (export && len(exported)+run > segment.numExSyms) {
			return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("export run exceeds symbol dictionary"))
		}
		if export {
			for count := 0; count < run; count++ {
				exported = append(exported, cloneGray(symbols[i]))
				i++
			}
		} else {
			i += run
		}
		export = !export
	}
	if len(exported) != segment.numExSyms {
		return nil, errors.Invalid("jbig2_symbol_dictionary", fmt.Errorf("exported %d symbols, want %d", len(exported), segment.numExSyms))
	}
	return exported, nil
}

func symbolCodeLength(symbolCount int, huff bool) uint {
	if symbolCount <= 1 {
		if huff {
			return 1
		}
		return 0
	}
	value := symbolCount - 1
	var codeLen uint
	for value > 0 {
		codeLen++
		value >>= 1
	}
	return codeLen
}

func newArithmeticIntStats(symCodeLen uint) (*arithmeticIntStats, error) {
	if symCodeLen+1 >= 31 {
		return nil, fmt.Errorf("symbol code length too large: %d", symCodeLen)
	}
	return &arithmeticIntStats{
		iadh:  make([]uint8, 1<<9),
		iadw:  make([]uint8, 1<<9),
		iaex:  make([]uint8, 1<<9),
		iaai:  make([]uint8, 1<<9),
		iadt:  make([]uint8, 1<<9),
		iait:  make([]uint8, 1<<9),
		iafs:  make([]uint8, 1<<9),
		iads:  make([]uint8, 1<<9),
		iardx: make([]uint8, 1<<9),
		iardy: make([]uint8, 1<<9),
		iardw: make([]uint8, 1<<9),
		iardh: make([]uint8, 1<<9),
		iari:  make([]uint8, 1<<9),
		iaid:  make([]uint8, 1<<(symCodeLen+1)),
	}, nil
}

func cloneGray(src *stdimage.Gray) *stdimage.Gray {
	if src == nil {
		return nil
	}
	dst := stdimage.NewGray(stdimage.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+dst.Bounds().Dx()], src.Pix[y*src.Stride:y*src.Stride+src.Bounds().Dx()])
	}
	return dst
}

func cloneBytes(src []uint8) []uint8 {
	if len(src) == 0 {
		return nil
	}
	dst := make([]uint8, len(src))
	copy(dst, src)
	return dst
}
