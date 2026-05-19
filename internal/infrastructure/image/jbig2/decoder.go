// Package jbig2 provides JBIG2 image decoding support for PDF rendering.
//
//revive:disable:exported
//nolint:staticcheck,unused
package jbig2

import (
	"fmt"
	stdimage "image"
	"image/color"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/image"
)

// Decoder implements JBIG2 image decoding.
type Decoder struct{}

// DecodeOptions carries PDF-level JBIG2 decode parameters.
type DecodeOptions struct {
	Globals []byte
	Width   int
	Height  int
}

// NewDecoder creates a new JBIG2 decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// NewNativeDecoder creates a decoder that uses only the clean-room Go path.
func NewNativeDecoder() *Decoder {
	return &Decoder{}
}

// Decode decodes JBIG2 image data.
func (d *Decoder) Decode(data []byte) (stdimage.Image, error) {
	return d.DecodeWithOptions(data, DecodeOptions{})
}

// DecodeWithOptions decodes JBIG2 image data with PDF DecodeParms.
func (d *Decoder) DecodeWithOptions(data []byte, opts DecodeOptions) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, errors.Invalid("jbig2_data", nil)
	}

	return d.decodeNative(data, opts)
}

// decodeNative provides a native Go JBIG2 decoder implementation.
func (d *Decoder) decodeNative(data []byte, opts DecodeOptions) (stdimage.Image, error) {
	if len(data) == 0 {
		return nil, errors.Invalid("jbig2_header", fmt.Errorf("invalid JBIG2 header: too short"))
	}

	doc, err := parseNativeDocument(data)
	if err != nil {
		return nil, errors.Invalid("jbig2_header", err)
	}
	if len(opts.Globals) > 0 {
		if err := doc.prependGlobals(opts.Globals); err != nil {
			return nil, errors.Invalid("jbig2_globals", err)
		}
	}
	doc.applyFallbackDimensions(opts.Width, opts.Height)
	if doc.hasPageInfo {
		img, decoded, err := d.decodePageAssociatedBitmap(doc)
		if err != nil {
			return nil, err
		}
		if decoded {
			return img, nil
		}
	}
	if img, decoded, err := d.decodeFirstBitmapFallback(doc); err != nil {
		return nil, err
	} else if decoded {
		return img, nil
	}

	return d.createPlaceholderImage(doc.pageInfo), nil
}

func putSegmentValue[V any](values map[uint32]V, number uint32, value V) {
	if _, exists := values[number]; exists {
		return
	}
	values[number] = value
}

func (d *Decoder) decodeFirstBitmapFallback(doc *nativeDocument) (*stdimage.Gray, bool, error) {
	page := d.createPlaceholderImage(doc.pageInfo).(*stdimage.Gray)
	patternDicts := map[uint32]patternDictionary{}
	symbolDicts := map[uint32]symbolDictionary{}
	codeTables := map[uint32][]huffmanTableEntry{}
	bitmaps := map[uint32]*stdimage.Gray{}

	for _, segment := range doc.segments {
		if segment.typ == SegmentTables {
			table, err := decodeCodeTableSegment(segment)
			if err != nil {
				return nil, false, err
			}
			putSegmentValue(codeTables, segment.number, table)
			continue
		}
		if segment.isSymbolDictionary() {
			dict, err := decodeSymbolDictionarySegmentWithCodeTables(segment, symbolDicts, codeTables)
			if err != nil {
				return nil, false, err
			}
			putSegmentValue(symbolDicts, segment.number, dict)
			continue
		}
		if segment.isPatternDictionary() {
			dict, err := decodePatternDictionarySegment(segment)
			if err != nil {
				return nil, false, err
			}
			putSegmentValue(patternDicts, segment.number, dict)
			continue
		}
		if segment.pageAssociation == 0 || !segment.requiresBitmapDecoding() {
			continue
		}
		if segment.isTextRegion() {
			_, img, err := decodeTextRegionSegmentWithCodeTables(segment, symbolDicts, codeTables)
			if err != nil {
				return nil, false, err
			}
			return img, true, nil
		}
		if segment.isHalftoneRegion() {
			_, img, err := decodeHalftoneRegionSegment(segment, patternDicts)
			if err != nil {
				return nil, false, err
			}
			return img, true, nil
		}
		if segment.isGenericRefinementRegion() {
			_, img, err := decodeGenericRefinementRegionSegment(segment, page, bitmaps)
			if err != nil {
				return nil, false, err
			}
			return img, true, nil
		}
		if segment.isGenericRegion() {
			_, img, err := d.decodeGenericBitmapSegment(segment)
			if err != nil {
				return nil, false, err
			}
			return img, true, nil
		}
		return nil, false, errors.NotImplemented(
			"jbig2_segment_decode",
			fmt.Errorf("native JBIG2 segment type %d not implemented", segment.typ),
		)
	}

	return nil, false, nil
}

func (d *Decoder) decodePageAssociatedBitmap(doc *nativeDocument) (*stdimage.Gray, bool, error) {
	page := d.createPlaceholderImage(doc.pageInfo).(*stdimage.Gray)
	patternDicts := map[uint32]patternDictionary{}
	symbolDicts := map[uint32]symbolDictionary{}
	codeTables := map[uint32][]huffmanTableEntry{}
	bitmaps := map[uint32]*stdimage.Gray{}
	decoded := false

	for _, segment := range doc.segments {
		if segment.typ == SegmentTables {
			table, err := decodeCodeTableSegment(segment)
			if err != nil {
				return nil, false, err
			}
			putSegmentValue(codeTables, segment.number, table)
			continue
		}
		if segment.isSymbolDictionary() {
			dict, err := decodeSymbolDictionarySegmentWithCodeTables(segment, symbolDicts, codeTables)
			if err != nil {
				return nil, false, err
			}
			putSegmentValue(symbolDicts, segment.number, dict)
			continue
		}
		if segment.isPatternDictionary() {
			dict, err := decodePatternDictionarySegment(segment)
			if err != nil {
				return nil, false, err
			}
			putSegmentValue(patternDicts, segment.number, dict)
			continue
		}
		if segment.pageAssociation == 0 || !segment.requiresBitmapDecoding() {
			continue
		}
		if segment.isHalftoneRegion() {
			regionInfo, img, err := decodeHalftoneRegionSegment(segment, patternDicts)
			if err != nil {
				return nil, false, err
			}
			if segment.isImmediateHalftoneRegion() {
				composeRegionIntoPage(page, regionInfo, img)
				decoded = true
			} else {
				putSegmentValue(bitmaps, segment.number, img)
			}
			continue
		}
		if segment.isGenericRefinementRegion() {
			regionInfo, img, err := decodeGenericRefinementRegionSegment(segment, page, bitmaps)
			if err != nil {
				return nil, false, err
			}
			discardReferencedBitmap(segment, bitmaps)
			if segment.isImmediateGenericRefinementRegion() {
				composeRegionIntoPage(page, regionInfo, img)
				decoded = true
			} else {
				putSegmentValue(bitmaps, segment.number, img)
			}
			continue
		}
		if segment.isTextRegion() {
			regionInfo, img, err := decodeTextRegionSegmentWithCodeTables(segment, symbolDicts, codeTables)
			if err != nil {
				return nil, false, err
			}
			if segment.isImmediateTextRegion() {
				composeRegionIntoPage(page, regionInfo, img)
				decoded = true
			} else {
				putSegmentValue(bitmaps, segment.number, img)
			}
			continue
		}
		if !segment.isGenericRegion() {
			return nil, false, errors.NotImplemented(
				"jbig2_segment_decode",
				fmt.Errorf("native JBIG2 segment type %d not implemented", segment.typ),
			)
		}

		region, img, err := d.decodeGenericBitmapSegment(segment)
		if err != nil {
			return nil, false, err
		}
		if segment.isImmediateGenericRegion() {
			composeRegionIntoPage(page, region.info, img)
			decoded = true
		} else {
			putSegmentValue(bitmaps, segment.number, img)
		}
	}

	return page, decoded, nil
}

func discardReferencedBitmap(segment segmentHeader, bitmaps map[uint32]*stdimage.Gray) {
	if len(segment.referredToSegmentNumbers) != 1 {
		return
	}
	delete(bitmaps, segment.referredToSegmentNumbers[0])
}

func (d *Decoder) decodeGenericBitmapSegment(segment segmentHeader) (genericRegion, *stdimage.Gray, error) {
	if err := validateBitmapSegmentBody(segment); err != nil {
		return genericRegion{}, nil, errors.Invalid("jbig2_segment_body", err)
	}
	if !segment.isGenericRegion() {
		return genericRegion{}, nil, errors.NotImplemented(
			"jbig2_segment_decode",
			fmt.Errorf("native JBIG2 segment type %d not implemented", segment.typ),
		)
	}

	region, err := parseGenericRegionSegment(segment.data)
	if err != nil {
		return genericRegion{}, nil, errors.Invalid("jbig2_segment_body", err)
	}
	img, err := decodeGenericRegion(region)
	if err != nil {
		return genericRegion{}, nil, errors.Invalid("jbig2_segment_decode", err)
	}
	return region, img, nil
}

// JBIG2Header represents parsed JBIG2 file header information.
type JBIG2Header struct {
	Width         int
	Height        int
	DefaultPixel  bool
	IsStriped     bool
	MaxStripeSize int
}

// parseFileHeader parses the JBIG2 file header.
func (d *Decoder) parseFileHeader(data []byte) (*JBIG2Header, error) {
	file, err := parseStandaloneFileHeader(data)
	if err != nil {
		return nil, err
	}

	hdr := defaultJBIG2Header()
	if file.headerLength < len(data) {
		d.parseSegmentHeader(data[file.headerLength:], hdr)
	}
	return hdr, nil
}

// parseSegmentHeader parses JBIG2 segment headers.
func (d *Decoder) parseSegmentHeader(data []byte, hdr *JBIG2Header) {
	segments, err := parseNativeSegments(data)
	if err != nil {
		return
	}
	doc := &nativeDocument{segments: segments, pageInfo: hdr}
	doc.applyPageInfo()
}

// decodeEmbedded handles embedded JBIG2 data (without file header).
func (d *Decoder) decodeEmbedded(data []byte) (stdimage.Image, error) {
	doc, err := parseEmbeddedDocument(data)
	if err != nil {
		return nil, errors.Invalid("jbig2_embedded", err)
	}
	return d.createPlaceholderImage(doc.pageInfo), nil
}

// createPlaceholderImage creates a placeholder image for the stub implementation.
func (d *Decoder) createPlaceholderImage(hdr *JBIG2Header) stdimage.Image {
	width := hdr.Width
	height := hdr.Height

	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 100
	}

	// JBIG2 is typically bi-level (1 bit per pixel)
	img := stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	pixel := uint8(0xff)
	if hdr.DefaultPixel {
		pixel = 0x00
	}
	for i := range img.Pix {
		img.Pix[i] = pixel
	}
	return img
}

// DecodeConfig returns the JBIG2 image configuration.
func (d *Decoder) DecodeConfig(data []byte) (stdimage.Config, error) {
	if len(data) == 0 {
		return stdimage.Config{}, errors.Invalid("jbig2_config", fmt.Errorf("data too short"))
	}

	doc, err := parseNativeDocument(data)
	if err != nil {
		return stdimage.Config{}, err
	}

	return stdimage.Config{
		Width:      doc.pageInfo.Width,
		Height:     doc.pageInfo.Height,
		ColorModel: color.GrayModel,
	}, nil
}

// ColorSpace returns the color space for JBIG2 images.
func (d *Decoder) ColorSpace() image.ColorSpace {
	return image.ColorSpaceDeviceGray
}

// CanDecode checks if the data appears to be a JBIG2 image.
func (d *Decoder) CanDecode(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	if hasStandaloneSignature(data) {
		return true
	}

	_, err := parseNativeSegmentHeader(data)
	return err == nil
}

// SupportedFormats returns the supported JBIG2 format identifiers.
func (d *Decoder) SupportedFormats() []string {
	return []string{"jb2", "jbig2"}
}

// DecodeSegment decodes a single JBIG2 segment (for embedded data).
func (d *Decoder) DecodeSegment(data []byte) (stdimage.Image, error) {
	return d.decodeNative(data, DecodeOptions{})
}

// SegmentType represents JBIG2 segment types.
type SegmentType int

const (
	SegmentSymbolDictionary                         SegmentType = 0
	SegmentIntermediateText                         SegmentType = 4
	SegmentImmediateText                            SegmentType = 6
	SegmentImmediateLosslessText                    SegmentType = 7
	SegmentPatternDictionary                        SegmentType = 16
	SegmentIntermediateHalftone                     SegmentType = 20
	SegmentImmediateHalftone                        SegmentType = 22
	SegmentImmediateLosslessHalftone                SegmentType = 23
	SegmentIntermediateGenericRegion                SegmentType = 36
	SegmentImmediateGenericRegion                   SegmentType = 38
	SegmentImmediateLosslessGenericRegion           SegmentType = 39
	SegmentIntermediateGenericRefinementRegion      SegmentType = 40
	SegmentImmediateGenericRefinementRegion         SegmentType = 42
	SegmentImmediateLosslessGenericRefinementRegion SegmentType = 43
	SegmentPageInformation                          SegmentType = 48
	SegmentEndOfPage                                SegmentType = 49
	SegmentEndOfStripe                              SegmentType = 50
	SegmentEndOfFile                                SegmentType = 51
	SegmentProfiles                                 SegmentType = 52
	SegmentTables                                   SegmentType = 53
	SegmentExtension                                SegmentType = 62
)

// RegionType represents JBIG2 region types.
type RegionType int

const (
	RegionTypeText              RegionType = 0
	RegionTypeHalftone          RegionType = 1
	RegionTypeGeneric           RegionType = 2
	RegionTypeGenericRefinement RegionType = 3
)

// MMRDecoder implements Modified Modified READ (MMR) compression.
type MMRDecoder struct {
	bitOffset    int
	decodedLines int
	data         []byte
	reference    []byte
	width        int
	height       int
}

// NewMMRDecoder creates a new MMR decoder.
func NewMMRDecoder(data []byte, width, height int) *MMRDecoder {
	return &MMRDecoder{
		data:   data,
		width:  width,
		height: height,
	}
}

// DecodeLine decodes one line using MMR compression.
func (md *MMRDecoder) DecodeLine() ([]byte, error) {
	lineBytes := (md.width + 7) / 8
	if lineBytes <= 0 {
		return nil, fmt.Errorf("invalid MMR width: %d", md.width)
	}
	if md.decodedLines >= md.height {
		return nil, io.EOF
	}

	line := make([]byte, lineBytes)
	x := 0
	black := false
	for x < md.width {
		modeBit, err := md.readBit()
		if err != nil {
			return nil, err
		}
		if modeBit == 1 {
			next := md.nextReferenceChangingElement(x, black)
			if err := setVerticalRun(line, &x, next, black, md.width); err != nil {
				return nil, err
			}
			black = !black
			continue
		}

		modeOffset := md.bitOffset - 1
		second, err := md.readBit()
		if err != nil {
			return nil, err
		}
		third, err := md.readBit()
		if err != nil {
			return nil, err
		}
		if second == 1 && third == 1 {
			next := md.nextReferenceChangingElement(x, black) + 1
			if err := setVerticalRun(line, &x, next, black, md.width); err != nil {
				return nil, err
			}
			black = !black
			continue
		}
		if second == 1 && third == 0 {
			next := md.nextReferenceChangingElement(x, black) - 1
			if err := setVerticalRun(line, &x, next, black, md.width); err != nil {
				return nil, err
			}
			black = !black
			continue
		}
		if second == 0 && third == 0 {
			fourth, err := md.readBit()
			if err != nil {
				return nil, err
			}
			if fourth == 1 {
				next := md.secondReferenceChangingElement(x, black)
				if err := setVerticalRun(line, &x, next, black, md.width); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("unsupported MMR code at bit offset %d", modeOffset)
		}
		if second != 0 || third != 1 {
			return nil, fmt.Errorf("unsupported MMR code at bit offset %d", modeOffset)
		}

		whiteRun, err := md.readWhiteTerminatingRun()
		if err != nil {
			return nil, err
		}
		blackRun, err := md.readBlackTerminatingRun()
		if err != nil {
			return nil, err
		}
		x += whiteRun
		if x+blackRun > md.width {
			return nil, fmt.Errorf("MMR run exceeds line width: %d > %d", x+blackRun, md.width)
		}
		setBilevelRun(line, x, blackRun, 1)
		x += blackRun
	}

	md.reference = append(md.reference[:0], line...)
	md.decodedLines++
	return line, nil
}

func setVerticalRun(line []byte, x *int, next int, black bool, width int) error {
	if next < *x || next > width {
		return fmt.Errorf("invalid vertical MMR transition: %d -> %d", *x, next)
	}
	if black {
		setBilevelRun(line, *x, next-*x, 1)
	}
	*x = next
	return nil
}

func (md *MMRDecoder) nextReferenceChangingElement(start int, black bool) int {
	for x := start + 1; x < md.width; x++ {
		if md.referencePixelBlack(x) == md.referencePixelBlack(x-1) {
			continue
		}
		if md.referencePixelBlack(x) != black {
			return x
		}
	}
	return md.width
}

func (md *MMRDecoder) secondReferenceChangingElement(start int, black bool) int {
	first := md.nextReferenceChangingElement(start, black)
	if first >= md.width {
		return md.width
	}
	return md.nextReferenceChangingElement(first, !black)
}

func (md *MMRDecoder) referencePixelBlack(x int) bool {
	if len(md.reference) == 0 {
		return false
	}
	byteOffset := x / 8
	if byteOffset >= len(md.reference) {
		return false
	}
	bitOffset := 7 - (x % 8)
	return ((md.reference[byteOffset] >> bitOffset) & 0x01) == 1
}

func (md *MMRDecoder) readBit() (uint8, error) {
	if md.bitOffset >= len(md.data)*8 {
		return 0, io.EOF
	}
	byteOffset := md.bitOffset / 8
	shift := 7 - (md.bitOffset % 8)
	md.bitOffset++
	return (md.data[byteOffset] >> shift) & 0x01, nil
}

func (md *MMRDecoder) readBits(count int) (uint32, error) {
	var value uint32
	for i := 0; i < count; i++ {
		bit, err := md.readBit()
		if err != nil {
			return 0, err
		}
		value = (value << 1) | uint32(bit)
	}
	return value, nil
}

func (md *MMRDecoder) readWhiteTerminatingRun() (int, error) {
	return md.readTerminatingRun(whiteTerminatingRunCodes, "white")
}

func (md *MMRDecoder) readBlackTerminatingRun() (int, error) {
	return md.readTerminatingRun(blackTerminatingRunCodes, "black")
}

func (md *MMRDecoder) readTerminatingRun(codes []mmrRunCode, color string) (int, error) {
	var value uint16
	start := md.bitOffset
	for width := uint8(1); width <= maxMMRTerminatingCodeLength(codes); width++ {
		bit, err := md.readBit()
		if err != nil {
			return 0, err
		}
		value = (value << 1) | uint16(bit)
		for _, code := range codes {
			if code.width == width && code.bits == value {
				return code.run, nil
			}
		}
	}
	return 0, fmt.Errorf("unsupported %s MMR terminating code at bit offset %d", color, start)
}

func maxMMRTerminatingCodeLength(codes []mmrRunCode) uint8 {
	var max uint8
	for _, code := range codes {
		if code.width > max {
			max = code.width
		}
	}
	return max
}

func setBilevelRun(row []byte, start, length int, bit uint8) {
	if bit == 0 {
		return
	}
	for x := start; x < start+length; x++ {
		byteOffset := x / 8
		bitOffset := 7 - (x % 8)
		if byteOffset >= len(row) {
			return
		}
		row[byteOffset] |= 1 << bitOffset
	}
}

// DecodeGenericRegion decodes a generic region segment.
func (d *Decoder) DecodeGenericRegion(data []byte, width, height int) (stdimage.Image, error) {
	return decodeGenericRegion(genericRegion{
		info: regionInfo{
			width:  width,
			height: height,
		},
		payload: data,
	})
}

// DecodeTextRegion decodes a text region segment.
func (d *Decoder) DecodeTextRegion(data []byte, width, height int, symbols []stdimage.Image) (stdimage.Image, error) {
	converted := make([]*stdimage.Gray, 0, len(symbols))
	for _, symbol := range symbols {
		gray, ok := symbol.(*stdimage.Gray)
		if !ok {
			return nil, errors.Invalid("jbig2_text_region", fmt.Errorf("symbol is not a bilevel gray bitmap"))
		}
		converted = append(converted, gray)
	}
	return decodeArithmeticTextRegion(textRegion{
		info: regionInfo{
			width:  width,
			height: height,
		},
		payload:      data,
		numInstances: 0,
	}, converted)
}
