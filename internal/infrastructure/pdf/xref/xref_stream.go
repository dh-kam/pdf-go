// Package xref provides XRef stream parsing functionality.
package xref

import (
	"bytes"
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/repository"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

// parseXRefStreamWithDetails parses an XRef stream at the given offset.
func (x *Table) parseXRefStreamWithDetails(offset uint64) error {
	if int(offset) >= len(x.stream) {
		return fmt.Errorf("xref stream offset out of bounds")
	}

	// XRef stream is an indirect object, need to parse it
	// Format: "N N obj << /Type /XRef /Index [start count] /W [n1 n2 n3] /Size N >> stream"
	// But we're already at offset, so we need to parse the object

	reader := bytes.NewReader(x.stream[offset:])
	lexer := parser.NewLexer(reader)

	// Parse object header: "N N obj"
	token1, err := lexer.NextToken()
	if err != nil {
		return fmt.Errorf("failed to read xref stream object number: %w", err)
	}
	if token1.Type != parser.TokenNumber {
		return fmt.Errorf("expected xref stream object number, got %s", token1.Type)
	}

	token2, err := lexer.NextToken()
	if err != nil {
		return fmt.Errorf("failed to read xref stream generation: %w", err)
	}
	if token2.Type != parser.TokenNumber {
		return fmt.Errorf("expected xref stream generation number, got %s", token2.Type)
	}

	token3, err := lexer.NextToken()
	if err != nil {
		return fmt.Errorf("failed to read 'obj': %w", err)
	}
	if token3.Type != parser.TokenKeyword || token3.Value != "obj" {
		return fmt.Errorf("expected 'obj', got %s %q", token3.Type, token3.Value)
	}

	// Parse the XRef stream dictionary
	p := parser.NewParser(lexer, x)
	streamObj, err := p.ParseObject()
	if err != nil {
		return fmt.Errorf("failed to parse xref stream dictionary: %w", err)
	}

	streamDict, ok := streamObj.(*entity.Dict)
	if !ok {
		return fmt.Errorf("xref stream is not a dictionary")
	}

	// Check Type
	typeVal := streamDict.Get(entity.Name("/Type"))
	if typeVal == nil || typeVal != entity.Name("XRef") {
		return fmt.Errorf("not an XRef stream (Type=%v)", typeVal)
	}

	// Get stream data using position-based extraction
	streamData, err := x.extractStreamDataFromOffset(offset, streamDict)
	if err != nil {
		return fmt.Errorf("failed to extract XRef stream data: %w", err)
	}

	// Decode the stream (usually FlateDecode)
	decodedData, err := x.decodeStream(streamData, streamDict)
	if err != nil {
		return fmt.Errorf("failed to decode XRef stream: %w", err)
	}

	// For XRef streams, the stream dictionary contains trailer fields.
	// Keep the first trailer discovered in Parse(), because the parser starts
	// from the newest xref and then traverses /Prev to older ones.
	if x.trailer == nil {
		x.trailer = streamDict
	}

	// Parse XRef stream fields
	if err := x.parseXRefStreamData(decodedData, streamDict); err != nil {
		return err
	}

	// Handle incremental updates: check for /Prev field
	// The /Prev field contains the byte offset of the previous XRef stream
	prevVal := streamDict.Get(entity.Name("/Prev"))
	if prevVal != nil {
		var prevOffset uint64
		switch v := prevVal.(type) {
		case *entity.Integer:
			prevOffset = uint64(v.Value())
		case *entity.Real:
			prevOffset = uint64(v.Value())
		default:
			return fmt.Errorf("invalid /Prev type: %T", prevVal)
		}

		if prevOffset > 0 && prevOffset < uint64(len(x.stream)) {
			// Recursively parse the previous XRef stream
			// Older entries only fill missing object slots so the latest section
			// remains authoritative for incremental updates.
			return x.parseXRefStreamWithDetails(prevOffset)
		}
	}

	return nil
}

// extractStreamDataFromOffset extracts stream data starting from a known offset.
func (x *Table) extractStreamDataFromOffset(startOffset uint64, streamDict *entity.Dict) ([]byte, error) {
	// Get /Length
	lengthVal := streamDict.Get(entity.Name("/Length"))
	if lengthVal == nil {
		return nil, fmt.Errorf("stream missing /Length")
	}

	var length int64
	switch v := lengthVal.(type) {
	case *entity.Integer:
		length = v.Value()
	default:
		return nil, fmt.Errorf("invalid /Length type: %T", lengthVal)
	}

	if length < 0 || int(length) > len(x.stream) {
		return nil, fmt.Errorf("invalid stream length: %d", length)
	}

	// Search for "stream" keyword starting from the known offset.
	searchStart := int(startOffset)
	searchEnd := searchStart + 5000 // Search up to 5000 bytes ahead
	if searchEnd > len(x.stream) {
		searchEnd = len(x.stream)
	}

	streamPos := findStreamKeywordOffset(x.stream, searchStart, searchEnd)
	if streamPos < 0 {
		return nil, fmt.Errorf("could not find 'stream' keyword after offset %d", startOffset)
	}

	streamPos = skipStreamKeywordEOL(x.stream, streamPos+len("stream"))

	// Check if we have enough data
	if streamPos+int(length) > len(x.stream) {
		return nil, fmt.Errorf("stream data exceeds file bounds")
	}

	// Extract the stream data
	return x.stream[streamPos : streamPos+int(length)], nil
}

// decodeStream decodes stream data using the specified filters
func (x *Table) decodeStream(data []byte, streamDict *entity.Dict) ([]byte, error) {
	if streamDict == nil {
		return data, nil
	}

	streamObj := entity.NewStream(streamDict, data)
	return pdfstream.NewFromEntity(streamObj).Decode()
}

// parseXRefStreamData parses the decoded XRef stream data
func (x *Table) parseXRefStreamData(data []byte, streamDict *entity.Dict) error {
	// Get /W field (array of 3 integers)
	wVal := streamDict.Get(entity.Name("/W"))
	if wVal == nil {
		return fmt.Errorf("XRef stream missing /W field")
	}

	wArr, ok := wVal.(*entity.Array)
	if !ok || wArr.Len() < 3 {
		return fmt.Errorf("invalid /W field")
	}

	// Extract field widths
	w := make([]int, 3)
	for i := 0; i < 3; i++ {
		if val := wArr.Get(i); val != nil {
			if intVal, ok := val.(*entity.Integer); ok {
				w[i] = int(intVal.Value())
			} else {
				w[i] = 1 // default width
			}
		} else {
			w[i] = 1
		}
	}

	// Get /Index field
	var indexFields [][]int
	indexVal := streamDict.Get(entity.Name("/Index"))
	if indexVal != nil {
		if arr, ok := indexVal.(*entity.Array); ok {
			for i := 0; i < arr.Len(); i += 2 {
				if i+1 < arr.Len() {
					startValue, okStart := arr.Get(i).(*entity.Integer)
					countValue, okCount := arr.Get(i + 1).(*entity.Integer)
					if !okStart || !okCount {
						continue
					}
					start := int(startValue.Value())
					count := int(countValue.Value())
					indexFields = append(indexFields, []int{start, count})
				}
			}
		}
	}

	// If no /Index, assume [0 Size]
	if len(indexFields) == 0 {
		sizeVal := streamDict.Get(entity.Name("/Size"))
		if sizeVal != nil {
			if sizeInt, ok := sizeVal.(*entity.Integer); ok {
				indexFields = append(indexFields, []int{0, int(sizeInt.Value())})
			}
		}
	}

	// Parse entries using the field widths
	pos := 0
	entryCount := 0
	for _, index := range indexFields {
		start := index[0]
		count := index[1]

		for i := 0; i < count; i++ {
			entry, _, err := x.readXRefStreamEntry(data, &pos, w)
			if err != nil {
				return fmt.Errorf("failed to read xref stream entry at position %d: %w", pos, err)
			}
			// Note: readXRefStreamEntry already advances pos internally via readXRefField
			// So we don't need to add n to pos here

			objNum := start + i

			for len(x.entries) <= objNum {
				x.entries = append(x.entries, nil)
			}
			// Preserve newer xref entries already parsed from the latest section.
			if x.entries[objNum] == nil {
				x.entries[objNum] = entry
			}
			entryCount++
		}
	}

	return nil
}

// readXRefStreamEntry reads a single entry from XRef stream data
func (x *Table) readXRefStreamEntry(data []byte, pos *int, w []int) (*repository.XRefEntry, int, error) {
	remaining := len(data) - *pos
	if remaining < w[0]+w[1]+w[2] {
		return nil, 0, fmt.Errorf("not enough data for xref stream entry")
	}

	// IMPORTANT: The /W array specifies field widths in a specific order
	// According to PDF spec, the default values (when not present) are:
	// w[0] = 1 (for type field)
	// w[1] = 0 (depends on type, 2 for offset)
	// w[2] = 0 (depends on type, 1 for generation)
	//
	// But when /W is specified, it's [w1, w2, w3] where:
	// - Field 1 is the TYPE field (contains 0, 1, or 2)
	// - Field 2 content depends on type
	// - Field 3 content depends on type
	//
	// Wait, let me check the spec again... The spec says:
	// "Default value: [1 2 1]" for the default widths, which means:
	// - w[1] = 1 (type field, 1 byte)
	// - w[2] = 2 (field 2, 2 bytes)
	// - w[3] = 1 (field 3, 1 byte)
	//
	// So /W [1 2 1] means:
	// - w[0] in our array = w[1] in spec = 1 byte for type field
	// - w[1] in our array = w[2] in spec = 2 bytes for field 2
	// - w[2] in our array = w[3] in spec = 1 byte for field 3
	//
	// Therefore:
	// - field1 (1 byte) = TYPE field
	// - field2 (2 bytes) = depends on type (offset for type 1, obj stream num for type 2)
	// - field3 (1 byte) = depends on type (generation for type 1, index for type 2)

	// If the type field is omitted, PDF xref streams default it to 1.
	// Poppler does the same in XRef::readXRefStreamSection.
	typeField := 1
	if w[0] > 0 {
		typeField = readXRefField(data, pos, w[0])
	}

	// Read field 2
	field2 := readXRefField(data, pos, w[1])

	// Read field 3
	field3 := readXRefField(data, pos, w[2])

	entry := &repository.XRefEntry{
		Type: repository.EntryTypeUncompressed,
	}

	// typeField determines the entry type
	switch typeField {
	case 0:
		// Free entry
		entry.Type = repository.EntryTypeFree
		entry.Free = true
		entry.Generation = uint16(field2) // Field 2 is generation number for free entries
	case 1:
		// Uncompressed entry
		// Field 2 is byte offset, Field 3 is generation number
		entry.Type = repository.EntryTypeUncompressed
		entry.Offset = uint64(field2)
		entry.Generation = uint16(field3)
		entry.Free = false
	case 2:
		// Compressed entry - in object stream
		// Field 2 is object stream number, Field 3 is index in stream
		entry.Type = repository.EntryTypeCompressed
		entry.ObjectStreamNumber = uint32(field2) // object stream number
		entry.ObjectStreamIndex = uint16(field3)  // index in stream
		entry.Free = false
	default:
		// Unknown type, assume uncompressed
		entry.Type = repository.EntryTypeUncompressed
		entry.Offset = uint64(field2)
		entry.Generation = uint16(field3)
		entry.Free = false
	}

	return entry, w[0] + w[1] + w[2], nil
}

// readXRefField reads a variable-width field from XRef stream data
func readXRefField(data []byte, pos *int, width int) int {
	result := 0
	for i := 0; i < width; i++ {
		if *pos >= len(data) {
			return 0
		}
		b := data[*pos]
		*pos++
		result = result<<8 | int(b)
	}
	return result
}
