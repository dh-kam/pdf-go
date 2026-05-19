package jbig2

import (
	"encoding/binary"
	"fmt"
)

var jbig2FileSignature = []byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A}

const unknownSegmentDataLength = uint32(0xffffffff)

type fileHeader struct {
	numberOfPages     uint32
	headerLength      int
	sequential        bool
	unknownPageCount  bool
	standaloneFile    bool
	hasNumberOfPages  bool
	hasCompleteHeader bool
}

type segmentHeader struct {
	number                   uint32
	typ                      SegmentType
	pageAssociation          uint32
	dataLength               uint32
	headerLength             int
	deferredNonRetain        bool
	largePageAssociation     bool
	referredToSegmentNumbers []uint32
	data                     []byte
}

type nativeDocument struct {
	fileHeader  fileHeader
	segments    []segmentHeader
	pageInfo    *JBIG2Header
	hasPageInfo bool
}

func hasStandaloneSignature(data []byte) bool {
	return len(data) >= len(jbig2FileSignature) && string(data[:len(jbig2FileSignature)]) == string(jbig2FileSignature)
}

func parseNativeDocument(data []byte) (*nativeDocument, error) {
	if hasStandaloneSignature(data) {
		return parseStandaloneDocument(data)
	}
	return parseEmbeddedDocument(data)
}

func parseStandaloneDocument(data []byte) (*nativeDocument, error) {
	if len(data) < len(jbig2FileSignature) {
		return nil, fmt.Errorf("JBIG2 file signature too short")
	}

	file, err := parseStandaloneFileHeader(data[len(jbig2FileSignature):])
	if err != nil {
		return nil, err
	}
	file.standaloneFile = true

	offset := len(jbig2FileSignature) + file.headerLength
	if offset > len(data) {
		return &nativeDocument{
			fileHeader: file,
			pageInfo:   defaultJBIG2Header(),
		}, nil
	}

	segments, err := parseNativeSegments(data[offset:])
	if err != nil {
		return nil, err
	}

	doc := &nativeDocument{
		fileHeader: file,
		segments:   segments,
		pageInfo:   defaultJBIG2Header(),
	}
	doc.applyPageInfo()
	return doc, nil
}

func parseEmbeddedDocument(data []byte) (*nativeDocument, error) {
	segments, err := parseNativeSegments(data)
	if err != nil {
		return nil, err
	}

	doc := &nativeDocument{
		fileHeader: fileHeader{sequential: true},
		segments:   segments,
		pageInfo:   defaultJBIG2Header(),
	}
	doc.applyPageInfo()
	return doc, nil
}

func parseStandaloneFileHeader(data []byte) (fileHeader, error) {
	file := fileHeader{
		headerLength: 0,
		sequential:   true,
	}

	if len(data) == 0 {
		return file, nil
	}

	flags := data[0]
	file.headerLength = 1
	file.sequential = flags&0x01 == 0
	file.unknownPageCount = flags&0x02 != 0
	file.hasCompleteHeader = true

	if file.unknownPageCount {
		return file, nil
	}
	if len(data) < 5 {
		return file, fmt.Errorf("truncated JBIG2 file header: missing page count")
	}

	file.numberOfPages = binary.BigEndian.Uint32(data[1:5])
	file.headerLength = 5
	file.hasNumberOfPages = true
	return file, nil
}

func parseNativeSegments(data []byte) ([]segmentHeader, error) {
	segments := make([]segmentHeader, 0)
	offset := 0

	for offset < len(data) {
		header, err := parseNativeSegmentHeader(data[offset:])
		if err != nil {
			return segments, err
		}
		offset += header.headerLength

		if header.dataLength == unknownSegmentDataLength {
			header.data = data[offset:]
			offset = len(data)
			segments = append(segments, header)
			break
		}

		dataLength := int(header.dataLength)
		if dataLength < 0 || offset+dataLength > len(data) {
			return segments, fmt.Errorf("segment %d data truncated: need %d bytes, have %d", header.number, dataLength, len(data)-offset)
		}

		header.data = data[offset : offset+dataLength]
		offset += dataLength
		segments = append(segments, header)

		if header.typ == SegmentEndOfFile {
			break
		}
	}

	return segments, nil
}

func parseNativeSegmentHeader(data []byte) (segmentHeader, error) {
	if len(data) < 6 {
		return segmentHeader{}, fmt.Errorf("truncated segment header")
	}

	header := segmentHeader{
		number: binary.BigEndian.Uint32(data[0:4]),
	}

	flags := data[4]
	header.deferredNonRetain = flags&0x80 != 0
	header.largePageAssociation = flags&0x40 != 0
	header.typ = SegmentType(flags & 0x3f)
	if !isKnownSegmentType(header.typ) {
		return segmentHeader{}, fmt.Errorf("unknown JBIG2 segment type: %d", header.typ)
	}

	offset := 5
	refCount, err := parseReferredToSegmentCount(data, &offset)
	if err != nil {
		return segmentHeader{}, err
	}

	refSize := referredToSegmentNumberSize(header.number)
	if refCount > 0 {
		if refCount > uint32((len(data)-offset)/refSize) {
			return segmentHeader{}, fmt.Errorf("truncated referred-to segment numbers")
		}
		header.referredToSegmentNumbers = make([]uint32, refCount)
		for i := uint32(0); i < refCount; i++ {
			header.referredToSegmentNumbers[i] = readUint(data[offset : offset+refSize])
			offset += refSize
		}
	}

	pageAssociationSize := 1
	if header.largePageAssociation {
		pageAssociationSize = 4
	}
	if len(data)-offset < pageAssociationSize+4 {
		return segmentHeader{}, fmt.Errorf("truncated page association or data length")
	}

	header.pageAssociation = readUint(data[offset : offset+pageAssociationSize])
	offset += pageAssociationSize

	header.dataLength = binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	header.headerLength = offset

	return header, nil
}

func parseReferredToSegmentCount(data []byte, offset *int) (uint32, error) {
	if *offset >= len(data) {
		return 0, fmt.Errorf("truncated referred-to segment count")
	}

	first := data[*offset]
	*offset += 1

	shortCount := first >> 5
	if shortCount != 7 {
		return uint32(shortCount), nil
	}

	if len(data)-(*offset) < 3 {
		return 0, fmt.Errorf("truncated long referred-to segment count")
	}

	count := uint32(first&0x1f)<<24 |
		uint32(data[*offset])<<16 |
		uint32(data[*offset+1])<<8 |
		uint32(data[*offset+2])
	*offset += 3

	retentionFlagBytes := int((count + 1 + 7) / 8)
	if len(data)-(*offset) < retentionFlagBytes {
		return 0, fmt.Errorf("truncated referred-to retention flags")
	}
	*offset += retentionFlagBytes
	return count, nil
}

func referredToSegmentNumberSize(segmentNumber uint32) int {
	switch {
	case segmentNumber <= 256:
		return 1
	case segmentNumber <= 65536:
		return 2
	default:
		return 4
	}
}

func readUint(data []byte) uint32 {
	switch len(data) {
	case 1:
		return uint32(data[0])
	case 2:
		return uint32(binary.BigEndian.Uint16(data))
	case 4:
		return binary.BigEndian.Uint32(data)
	default:
		return 0
	}
}

func isKnownSegmentType(typ SegmentType) bool {
	switch typ {
	case SegmentSymbolDictionary,
		SegmentIntermediateText,
		SegmentImmediateText,
		SegmentImmediateLosslessText,
		SegmentPatternDictionary,
		SegmentIntermediateHalftone,
		SegmentImmediateHalftone,
		SegmentImmediateLosslessHalftone,
		SegmentIntermediateGenericRegion,
		SegmentImmediateGenericRegion,
		SegmentImmediateLosslessGenericRegion,
		SegmentIntermediateGenericRefinementRegion,
		SegmentImmediateGenericRefinementRegion,
		SegmentImmediateLosslessGenericRefinementRegion,
		SegmentPageInformation,
		SegmentEndOfPage,
		SegmentEndOfStripe,
		SegmentEndOfFile,
		SegmentProfiles,
		SegmentTables,
		SegmentExtension:
		return true
	default:
		return false
	}
}

func (doc *nativeDocument) firstBitmapSegment() (segmentHeader, bool) {
	for _, segment := range doc.segments {
		if segment.pageAssociation == 0 {
			continue
		}
		if segment.requiresBitmapDecoding() {
			return segment, true
		}
	}
	return segmentHeader{}, false
}

func (doc *nativeDocument) prependGlobals(data []byte) error {
	globals, err := parseEmbeddedDocument(data)
	if err != nil {
		return err
	}
	segments := make([]segmentHeader, 0, len(globals.segments)+len(doc.segments))
	segments = append(segments, globals.segments...)
	segments = append(segments, doc.segments...)
	doc.segments = segments
	if !doc.hasPageInfo && globals.hasPageInfo {
		doc.pageInfo = globals.pageInfo
		doc.hasPageInfo = true
	}
	return nil
}

func (segment segmentHeader) requiresBitmapDecoding() bool {
	switch segment.typ {
	case SegmentSymbolDictionary,
		SegmentIntermediateText,
		SegmentImmediateText,
		SegmentImmediateLosslessText,
		SegmentPatternDictionary,
		SegmentIntermediateHalftone,
		SegmentImmediateHalftone,
		SegmentImmediateLosslessHalftone,
		SegmentIntermediateGenericRegion,
		SegmentImmediateGenericRegion,
		SegmentImmediateLosslessGenericRegion,
		SegmentIntermediateGenericRefinementRegion,
		SegmentImmediateGenericRefinementRegion,
		SegmentImmediateLosslessGenericRefinementRegion:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isGenericRegion() bool {
	switch segment.typ {
	case SegmentIntermediateGenericRegion,
		SegmentImmediateGenericRegion,
		SegmentImmediateLosslessGenericRegion:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isSymbolDictionary() bool {
	return segment.typ == SegmentSymbolDictionary
}

func (segment segmentHeader) isTextRegion() bool {
	switch segment.typ {
	case SegmentIntermediateText,
		SegmentImmediateText,
		SegmentImmediateLosslessText:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isImmediateTextRegion() bool {
	switch segment.typ {
	case SegmentImmediateText,
		SegmentImmediateLosslessText:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isImmediateGenericRegion() bool {
	switch segment.typ {
	case SegmentImmediateGenericRegion,
		SegmentImmediateLosslessGenericRegion:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isPatternDictionary() bool {
	return segment.typ == SegmentPatternDictionary
}

func (segment segmentHeader) isHalftoneRegion() bool {
	switch segment.typ {
	case SegmentIntermediateHalftone,
		SegmentImmediateHalftone,
		SegmentImmediateLosslessHalftone:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isImmediateHalftoneRegion() bool {
	switch segment.typ {
	case SegmentImmediateHalftone,
		SegmentImmediateLosslessHalftone:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isGenericRefinementRegion() bool {
	switch segment.typ {
	case SegmentIntermediateGenericRefinementRegion,
		SegmentImmediateGenericRefinementRegion,
		SegmentImmediateLosslessGenericRefinementRegion:
		return true
	default:
		return false
	}
}

func (segment segmentHeader) isImmediateGenericRefinementRegion() bool {
	switch segment.typ {
	case SegmentImmediateGenericRefinementRegion,
		SegmentImmediateLosslessGenericRefinementRegion:
		return true
	default:
		return false
	}
}

func (doc *nativeDocument) applyPageInfo() {
	if doc.pageInfo == nil {
		doc.pageInfo = defaultJBIG2Header()
	}

	for _, segment := range doc.segments {
		if segment.typ != SegmentPageInformation {
			continue
		}
		if len(segment.data) < 8 {
			continue
		}

		width := binary.BigEndian.Uint32(segment.data[0:4])
		height := binary.BigEndian.Uint32(segment.data[4:8])
		if width <= uint32(maxInt()) {
			doc.pageInfo.Width = int(width)
		}
		if height <= uint32(maxInt()) {
			doc.pageInfo.Height = int(height)
		}
		if len(segment.data) >= 17 {
			flags := segment.data[16]
			doc.pageInfo.DefaultPixel = flags&0x04 != 0
			doc.pageInfo.IsStriped = flags&0x02 != 0
		}
		if len(segment.data) >= 19 {
			doc.pageInfo.MaxStripeSize = int(binary.BigEndian.Uint16(segment.data[17:19]))
		}
		doc.hasPageInfo = true
		return
	}
}

func (doc *nativeDocument) applyFallbackDimensions(width, height int) {
	if doc.pageInfo == nil {
		doc.pageInfo = defaultJBIG2Header()
	}
	if doc.hasPageInfo {
		return
	}
	if width > 0 {
		doc.pageInfo.Width = width
	}
	if height > 0 {
		doc.pageInfo.Height = height
	}
}

func defaultJBIG2Header() *JBIG2Header {
	return &JBIG2Header{
		Width:  100,
		Height: 100,
	}
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
