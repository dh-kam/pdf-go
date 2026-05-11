package pdf

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

type legacyStreamState struct {
	data   []byte
	offset int
}

// LegacySignData is a Java-compatibility sign payload used by legacy alias methods.
type LegacySignData struct {
	fieldIndex           int
	fieldName            string
	filter               string
	subFilter            string
	cert                 string
	original             []byte
	signed               []byte
	byteRange            []int64
	signedBy             string
	signedAt             string
	reason               string
	location             string
	originalStreamHandle int
	pageIndex            int
	rect                 [4]float64
}

// FieldIndex returns one compatibility field index.
func (d *LegacySignData) FieldIndex() int {
	if d == nil {
		return -1
	}
	return d.fieldIndex
}

// FieldName returns one signature field name.
func (d *LegacySignData) FieldName() string {
	if d == nil {
		return ""
	}
	return d.fieldName
}

// SignedData returns one copy of signed bytes.
func (d *LegacySignData) SignedData() []byte {
	if d == nil {
		return nil
	}
	return append([]byte(nil), d.signed...)
}

// SetSignedData replaces signed bytes.
func (d *LegacySignData) SetSignedData(value []byte) {
	if d == nil {
		return
	}
	d.signed = append([]byte(nil), value...)
}

// ByteRangeValues returns one copy of ByteRange values.
func (d *LegacySignData) ByteRangeValues() []int64 {
	if d == nil {
		return nil
	}
	return append([]int64(nil), d.byteRange...)
}

// OriginalStreamHandle returns one compatibility stream handle.
func (d *LegacySignData) OriginalStreamHandle() int {
	if d == nil {
		return 0
	}
	return d.originalStreamHandle
}

func cloneLegacySignData(input *LegacySignData) *LegacySignData {
	if input == nil {
		return nil
	}
	return &LegacySignData{
		fieldIndex:           input.fieldIndex,
		fieldName:            input.fieldName,
		filter:               input.filter,
		subFilter:            input.subFilter,
		cert:                 input.cert,
		original:             append([]byte(nil), input.original...),
		signed:               append([]byte(nil), input.signed...),
		byteRange:            append([]int64(nil), input.byteRange...),
		signedBy:             input.signedBy,
		signedAt:             input.signedAt,
		reason:               input.reason,
		location:             input.location,
		originalStreamHandle: input.originalStreamHandle,
		pageIndex:            input.pageIndex,
		rect:                 input.rect,
	}
}

func (d *Document) allocLegacyStreamHandle(data []byte) int {
	if len(data) == 0 {
		return 0
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	handle := d.nextLegacyStreamHandle
	if handle <= 0 {
		handle = 1
	}
	d.nextLegacyStreamHandle = handle + 1
	d.legacyStreams[handle] = &legacyStreamState{
		data: append([]byte(nil), data...),
	}
	return handle
}

func (d *Document) readLegacyStreamChunk(handle int, maxLen int) ([]byte, bool) {
	if maxLen <= 0 {
		return nil, false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	stream, ok := d.legacyStreams[handle]
	if !ok || stream == nil || stream.offset >= len(stream.data) {
		return nil, false
	}

	end := stream.offset + maxLen
	if end > len(stream.data) {
		end = len(stream.data)
	}
	chunk := append([]byte(nil), stream.data[stream.offset:end]...)
	stream.offset = end
	return chunk, true
}

func (d *Document) legacyStreamLength(handle int) int {
	d.mu.RLock()
	stream, ok := d.legacyStreams[handle]
	d.mu.RUnlock()
	if ok && stream != nil {
		return len(stream.data)
	}

	raw, err := d.rawPDFData()
	if err != nil {
		return 0
	}
	return len(raw)
}

func (d *Document) deleteLegacyStreamHandle(handle int) int {
	if handle <= 0 {
		return 0
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.legacyStreams[handle]; !ok {
		return 0
	}
	delete(d.legacyStreams, handle)
	return 1
}

func (d *Document) nextLegacySignatureFieldName(seed string) string {
	base := strings.TrimSpace(seed)
	if base == "" {
		base = "ezPDFSignature"
	}

	base = strings.TrimRightFunc(base, unicode.IsDigit)
	if base == "" {
		base = "ezPDFSignature"
	}

	candidate := base
	suffix := 1
	for d.legacySignatureFieldExists(candidate) {
		candidate = fmt.Sprintf("%s%d", base, suffix)
		suffix++
	}
	return candidate
}

func (d *Document) legacySignatureFieldExists(fieldName string) bool {
	trimmed := strings.TrimSpace(fieldName)
	if trimmed == "" {
		return false
	}

	d.mu.RLock()
	_, exists := d.signatureFields[trimmed]
	d.mu.RUnlock()
	if exists {
		return true
	}

	signatures, err := d.Signatures()
	if err != nil {
		return false
	}
	for _, signature := range signatures {
		if signature != nil && signature.FieldName == trimmed {
			return true
		}
	}
	return false
}

func (d *Document) legacySignaturePageIndex(pageHint int) (int, bool) {
	pageCount, err := d.PageCount()
	if err != nil || pageCount <= 0 {
		return 0, false
	}

	if pageHint > 0 && pageHint <= pageCount {
		return pageHint - 1, true
	}
	if pageHint >= 0 && pageHint < pageCount {
		return pageHint, true
	}
	return 0, true
}

func (d *Document) setNowSigning(value bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nowSigning = value
}

func normalizeLegacySignTimestamp(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return time.Now().UTC().Format("D:20060102150405Z")
}
