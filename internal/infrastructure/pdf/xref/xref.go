// Package xref implements PDF cross-reference table parsing and object resolution.
package xref

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/repository"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/crypto"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

// Table implements the XRef interface.
type Table struct {
	entries             []*repository.XRefEntry
	trailer             *entity.Dict
	catalog             *entity.Dict
	cache               map[entity.Ref]entity.Object
	stream              []byte // Raw PDF data
	encryption          *crypto.EncryptionHandler
	fileID              []byte
	syntheticCatalog    bool
	linearizedPageCount int

	objStreamLocations  map[uint32]objectStreamLocation
	objStreamIndexBuilt bool
}

type objectStreamLocation struct {
	streamNum uint32
	index     uint16
}

// NewTable creates a new XRef table.
func NewTable(data []byte) *Table {
	return &Table{
		entries:             make([]*repository.XRefEntry, 0),
		cache:               make(map[entity.Ref]entity.Object),
		stream:              data,
		objStreamLocations:  make(map[uint32]objectStreamLocation),
		objStreamIndexBuilt: false,
	}
}

// SetEncryptionHandler sets the encryption handler for decrypting objects.
func (x *Table) SetEncryptionHandler(handler *crypto.EncryptionHandler) {
	x.encryption = handler
}

// EncryptionHandler returns the encryption handler.
func (x *Table) EncryptionHandler() *crypto.EncryptionHandler {
	return x.encryption
}

// UsesSyntheticCatalog returns true when catalog/pages were rebuilt from raw page scans.
func (x *Table) UsesSyntheticCatalog() bool {
	return x.syntheticCatalog
}

// LinearizedPageCount returns the page count from a linearization dictionary, when present.
func (x *Table) LinearizedPageCount() (int, bool) {
	if x.linearizedPageCount <= 0 {
		return 0, false
	}
	return x.linearizedPageCount, true
}

// SetFileID sets the file ID used for encryption.
func (x *Table) SetFileID(id []byte) {
	x.fileID = id
}

// FileID returns the file ID.
func (x *Table) FileID() []byte {
	return x.fileID
}

// Parse parses the cross-reference table from PDF data.
func (x *Table) Parse() error {
	// Find the startxref offset
	startXRef, err := x.findStartXRef()
	if err != nil {
		return errors.Invalid("find_startxref", err)
	}

	// Parse XRef table(s) starting from startxref
	if err := x.parseXRefAt(startXRef); err != nil {
		return errors.Invalid("parse_xref", err)
	}

	// fmt.Printf("DEBUG: After parseXRefAt, entries=%d\n", len(x.entries))

	// Parse trailer dictionary (if not already parsed by XRef stream)
	if x.trailer == nil {
		if err := x.parseTrailer(); err != nil {
			return errors.Invalid("parse_trailer", err)
		}
	}

	x.linearizedPageCount = x.detectLinearizedPageCount()

	// Handle incremental updates via trailer /Prev field
	// Check if trailer has a /Prev field pointing to previous XRef
	if x.trailer != nil {
		if prevVal := x.trailer.Get(entity.Name("/Prev")); prevVal != nil {
			var prevOffset uint64
			switch v := prevVal.(type) {
			case *entity.Integer:
				prevOffset = uint64(v.Value())
			case *entity.Real:
				prevOffset = uint64(v.Value())
			}

			if prevOffset > 0 && prevOffset < uint64(len(x.stream)) {
				// Parse the previous XRef table/stream
				// We parse it but don't override the main trailer
				// This allows incremental updates to merge properly
				x.parsePreviousXRefBestEffort(prevOffset)
			}
		}
	}

	// Resolve catalog
	if err := x.resolveCatalog(); err != nil {
		return errors.Invalid("resolve_catalog", err)
	}

	return nil
}

func (x *Table) parsePreviousXRefBestEffort(offset uint64) {
	if err := x.parseXRefAt(offset); err != nil {
		// Ignore previous-section failures and keep latest section authoritative.
		return
	}
}

// findStartXRef finds the byte offset of the XRef table.
func (x *Table) findStartXRef() (uint64, error) {
	// Search for "startxref" from the end of file
	data := x.stream
	searchSize := int64(len(data))
	if searchSize > 1024 {
		searchSize = 1024
	}

	startFrom := len(data) - int(searchSize)
	idx := bytes.LastIndex(data[startFrom:], []byte("startxref"))
	if idx == -1 {
		return 0, fmt.Errorf("startxref not found")
	}

	// Read the offset after "startxref"
	reader := bufio.NewReader(bytes.NewReader(data[startFrom+idx:]))
	if _, err := reader.ReadBytes('\n'); err != nil {
		return 0, err
	}

	// Read offset value
	var offset uint64
	_, err := fmt.Fscanf(reader, "%d", &offset)
	if err != nil {
		return 0, fmt.Errorf("invalid startxref offset: %w", err)
	}

	return offset, nil
}

// parseXRefAt parses an XRef table at the given offset.
func (x *Table) parseXRefAt(offset uint64) error {
	if int(offset) >= len(x.stream) {
		return fmt.Errorf("xref offset out of bounds")
	}

	reader := bufio.NewReader(bytes.NewReader(x.stream[offset:]))
	lexer := parser.NewLexer(reader)

	// Check if this is a traditional XRef table or XRef stream
	token, err := lexer.NextToken()
	if err != nil {
		return err
	}

	if token.Type == parser.TokenKeyword && token.Value == "xref" {
		return x.parseTraditionalXRef(lexer)
	}

	// Otherwise, it should be an XRef stream (starts with object number)
	// Try to parse as XRef stream first
	err = x.parseXRefStream(offset)
	if err != nil {
		// XRef stream parsing failed, fall back to object scanning
		return x.parseByObjectScanning()
	}
	return nil
}

// parseTraditionalXRef parses a traditional XRef table.
func (x *Table) parseTraditionalXRef(lexer *parser.Lexer) error {
	for {
		// Read subsection header: "start count"
		// Read start number
		startToken, err := lexer.NextToken()
		if err != nil {
			// End of XRef table
			break
		}
		if startToken.Type != parser.TokenNumber {
			// Not a subsection header, must be end of xref
			break
		}

		startNum := parseInt64(startToken.Value)

		// Read count number
		countToken, err := lexer.NextToken()
		if err != nil {
			return fmt.Errorf("expected count number, got error: %w", err)
		}
		if countToken.Type != parser.TokenNumber {
			return fmt.Errorf("expected count number, got %s", countToken.Type)
		}
		count := parseInt64(countToken.Value)

		// Read entries
		for i := int64(0); i < count; i++ {
			entry, err := x.readXRefEntryFromLexer(lexer)
			if err != nil {
				return err
			}

			// Ensure entries slice is large enough
			objNum := int(startNum + i)
			for len(x.entries) <= objNum {
				x.entries = append(x.entries, nil)
			}
			// Keep the first-seen entry for one object number.
			// Parse() starts from the latest xref and then follows /Prev, so
			// this preserves newer offsets and avoids old incremental sections
			// overriding them.
			if x.entries[objNum] == nil {
				x.entries[objNum] = entry
			}
		}
	}

	return nil
}

// parseInt64 parses an int64 from a string.
func parseInt64(s string) int64 {
	var result int64
	var sign int64 = 1

	i := 0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		i++
	}

	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		result = result*10 + int64(s[i]-'0')
	}

	return sign * result
}

// readXRefEntryFromLexer reads a single XRef entry using the lexer.
// Format: "offset generation [f/n]" (18 bytes + EOL)
func (x *Table) readXRefEntryFromLexer(lexer *parser.Lexer) (*repository.XRefEntry, error) {
	// Read offset (first 10 digits)
	offsetToken, err := lexer.NextToken()
	if err != nil {
		return nil, err
	}
	if offsetToken.Type != parser.TokenNumber {
		return nil, fmt.Errorf("expected offset number, got %s", offsetToken.Type)
	}
	offset := uint64(parseInt64(offsetToken.Value))

	// Read generation (next 5 digits)
	genToken, err := lexer.NextToken()
	if err != nil {
		return nil, err
	}
	if genToken.Type != parser.TokenNumber {
		return nil, fmt.Errorf("expected generation number, got %s", genToken.Type)
	}
	gen := uint16(parseInt64(genToken.Value))

	// Read type ('f' or 'n')
	typeToken, err := lexer.NextToken()
	if err != nil {
		return nil, err
	}
	if typeToken.Type != parser.TokenKeyword {
		return nil, fmt.Errorf("expected type keyword, got %s", typeToken.Type)
	}
	typ := typeToken.Value

	entry := &repository.XRefEntry{
		Offset:     offset,
		Generation: gen,
		Type:       repository.EntryTypeUncompressed,
		Free:       typ == "f",
	}

	if entry.Free {
		entry.Type = repository.EntryTypeFree
	}

	return entry, nil
}

// parseXRefStream parses an XRef stream (PDF 1.5+).
func (x *Table) parseXRefStream(offset uint64) error {
	// Use the XRef stream parser in xref_stream.go
	return x.parseXRefStreamWithDetails(offset)
}

// parseByObjectScanning scans the entire PDF for indirect objects.
// This is a fallback for PDFs with XRef streams.
func (x *Table) parseByObjectScanning() error {
	// Use enhanced object scanning
	return x.scanAllObjects()
}

// RebuildCatalogByObjectScan rebuilds xref entries/catalog by scanning raw objects.
// This is used as a fallback for malformed documents with broken xref references.
func (x *Table) RebuildCatalogByObjectScan() error {
	return x.scanAllObjects()
}

// RecoverPageRefsByObjectScan scans objects and returns page references found in raw PDF data.
func (x *Table) RecoverPageRefsByObjectScan() ([]entity.Ref, error) {
	if err := x.scanAllObjects(); err != nil {
		return nil, err
	}

	pageRefs := x.collectPageRefsFromEntries()
	if rawPageRefs := x.collectPageRefsFromRawPatterns(x.stream); len(rawPageRefs) > len(pageRefs) {
		pageRefs = rawPageRefs
	}
	if len(pageRefs) == 0 {
		return nil, fmt.Errorf("no pages found by object scan")
	}

	out := make([]entity.Ref, len(pageRefs))
	copy(out, pageRefs)
	return out, nil
}

// scanAllObjects scans the entire PDF for all indirect objects.
// This works for PDFs with or without XRef streams, as long as objects are not compressed.
func (x *Table) scanAllObjects() error {
	data := x.stream

	// Clear existing entries and build from scratch
	x.entries = make([]*repository.XRefEntry, 0)

	// Scan for all indirect objects by looking for "N N obj" patterns
	// We scan byte-by-byte to find "obj" keyword, then look backwards for numbers
	pos := 0
	for pos < len(data)-3 {
		// Find "obj" keyword
		if data[pos] != 'o' || data[pos+1] != 'b' || data[pos+2] != 'j' {
			pos++
			continue
		}

		// Found "obj", now look backwards for "N N" pattern
		// Skip past "obj" for next iteration
		lookBehind := pos
		var objNum, gen int64

		// Look backwards: we expect "...digit digit whitespace obj" or "...digit digit obj"
		// So we need to find two numbers before "obj"
		lookBehind -= 1 // Move past 'o'

		// Skip whitespace between gen number and "obj"
		for lookBehind >= 0 && isWhitespace(data[lookBehind]) {
			lookBehind--
		}

		// Now look for generation number
		gen, newPos := parseNumberBackward(data, lookBehind)
		if newPos < 0 {
			pos++ // Not a valid object header
			continue
		}
		lookBehind = newPos

		// Skip whitespace between objNum and gen
		for lookBehind >= 0 && isWhitespace(data[lookBehind]) {
			lookBehind--
		}

		// Look for object number
		objNum, newPos = parseNumberBackward(data, lookBehind)
		if newPos < 0 {
			pos++ // Not a valid object header
			continue
		}

		// Successfully found "N N obj" pattern!
		objStart := newPos + 1 // First digit of object number

		// Find object end. -1 means the real endobj is beyond the 100 KB search window
		// (e.g. a large image stream). Treat the rest of the file as this object's extent
		// so it is not skipped; parseObjectAt will locate the stream independently.
		endPos := x.findObjectEnd(data, pos+3)
		if endPos == -1 {
			endPos = len(data)
		}
		if endPos <= objStart {
			pos++
			continue
		}

		// Ensure entries array is large enough
		for len(x.entries) <= int(objNum) {
			x.entries = append(x.entries, nil)
		}

		ref := entity.NewRef(uint32(objNum), uint16(gen))
		obj, err := x.parseObjectAt(uint64(objStart), ref)
		if err != nil {
			// False-positive "obj" match (often inside stream bytes). Keep scanning.
			pos++
			continue
		}

		// Create XRef entry
		entry := &repository.XRefEntry{
			Offset:     uint64(objStart),
			Generation: uint16(gen),
			Type:       repository.EntryTypeUncompressed,
			Free:       false,
		}

		// Prefer later offsets for duplicate object numbers (incremental updates).
		existing := x.entries[int(objNum)]
		if existing == nil || entry.Offset > existing.Offset {
			x.entries[int(objNum)] = entry
			x.cache[ref] = obj
		}

		// Move past current object. For large objects where endPos == len(data)
		// we only know the object header position, not the real end — skip past
		// the "obj" keyword so we keep scanning for subsequent objects.
		if endPos == len(data) {
			pos = pos + 3 // resume scan just past "obj"
		} else {
			pos = endPos
		}
	}

	// After scanning, check if we found any objects
	if len(x.entries) == 0 {
		return fmt.Errorf("no objects found in PDF")
	}

	// Try to find catalog
	return x.findCatalogInScannedObjects()
}

// parseNumberBackward parses a number by scanning backwards from the given position.
// Returns the number and the position BEFORE the first digit.
func parseNumberBackward(data []byte, pos int) (int64, int) {
	if pos < 0 || pos >= len(data) {
		return 0, -1
	}

	// Skip trailing whitespace first
	for pos >= 0 && isWhitespace(data[pos]) {
		pos--
	}

	if pos < 0 || data[pos] < '0' || data[pos] > '9' {
		return 0, -1
	}

	// Found a digit, parse the full number (might have multiple digits)
	end := pos
	start := pos

	// Check for negative sign
	isNegative := false
	if start > 0 && data[start-1] == '-' {
		isNegative = true
		start--
	}

	// Find the start of the number
	for start > 0 && data[start-1] >= '0' && data[start-1] <= '9' {
		start--
	}

	// Parse the number
	var result int64
	for i := start; i <= end; i++ {
		result = result*10 + int64(data[i]-'0')
	}

	if isNegative {
		result = -result
	}

	return result, start - 1
}

func parseNumberForward(data []byte, pos int) (int64, int, bool) {
	for pos < len(data) && isWhitespace(data[pos]) {
		pos++
	}
	if pos >= len(data) || !isDigitByte(data[pos]) {
		return 0, pos, false
	}

	var result int64
	for pos < len(data) && isDigitByte(data[pos]) {
		result = result*10 + int64(data[pos]-'0')
		pos++
	}

	return result, pos, true
}

func countParentRefMentions(data []byte, ref entity.Ref) int {
	if len(data) == 0 {
		return 0
	}

	count := 0
	searchPos := 0
	parentToken := []byte("/Parent")

	for searchPos < len(data) {
		idxRel := bytes.Index(data[searchPos:], parentToken)
		if idxRel == -1 {
			break
		}

		idx := searchPos + idxRel
		cursor := idx + len(parentToken)

		objNum, cursor, ok := parseNumberForward(data, cursor)
		if !ok {
			searchPos = idx + 1
			continue
		}

		genNum, cursor, ok := parseNumberForward(data, cursor)
		if !ok {
			searchPos = idx + 1
			continue
		}

		for cursor < len(data) && isWhitespace(data[cursor]) {
			cursor++
		}
		if cursor >= len(data) || data[cursor] != 'R' {
			searchPos = idx + 1
			continue
		}

		if objNum == int64(ref.Num()) && genNum == int64(ref.Gen()) {
			count++
		}
		searchPos = idx + 1
	}

	return count
}

// isWhitespace checks if a byte is a PDF whitespace character.
func isWhitespace(b byte) bool {
	return b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func isLikelyObjectHeaderStart(data []byte, idx int) bool {
	if idx <= 0 {
		return true
	}

	cursor := idx - 1
	for cursor >= 0 && (data[cursor] == ' ' || data[cursor] == '\t') {
		cursor--
	}
	if cursor < 0 {
		return true
	}

	return data[cursor] == '\n' || data[cursor] == '\r'
}

// findObjectEnd finds the end of an object definition by looking for "endobj".
// Returns the position after "endobj" or -1 if not found.
// No upper bound is applied: content streams can exceed 100 KB (e.g. compressed
// page streams), so we search the full tail.
func (x *Table) findObjectEnd(data []byte, objPos int) int {
	if objPos < 0 || objPos >= len(data) {
		return -1
	}
	idx := bytes.Index(data[objPos:], []byte("endobj"))
	if idx == -1 {
		return -1
	}
	return objPos + idx + 6 // Position after "endobj"
}

// findCatalogInScannedObjects finds the catalog by scanning parsed objects
func (x *Table) findCatalogInScannedObjects() error {
	var fallbackCatalog *entity.Dict
	fallbackCatalogObjNum := -1

	// Try to find catalog object by checking each object
	for objNum, entry := range x.entries {
		if entry == nil || entry.Free {
			continue
		}

		// Fetch the object
		obj, err := x.parseObjectAt(entry.Offset, entity.Ref{})
		if err != nil {
			continue
		}

		// Check if it's a catalog
		if dict, ok := obj.(*entity.Dict); ok {
			typeVal := dict.Get(entity.Name("/Type"))
			if typeVal == entity.Name("Catalog") || typeVal == entity.Name("/Catalog") {
				if x.catalogHasResolvablePages(dict) {
					x.catalog = dict
					x.syntheticCatalog = false

					// Create minimal trailer
					if x.trailer == nil {
						trailer := entity.NewDict()
						trailer.Set(entity.Name("/Root"), entity.NewRef(uint32(objNum), 0))
						trailer.Set(entity.Name("/Size"), entity.NewInteger(int64(len(x.entries))))
						x.trailer = trailer
					}

					return nil
				}
				if fallbackCatalog == nil {
					fallbackCatalog = dict
					fallbackCatalogObjNum = objNum
				}
			}
		}
	}

	if err := x.buildCatalogAndPagesFromScan(x.stream); err == nil && x.catalog != nil {
		return nil
	}

	if fallbackCatalog != nil {
		x.catalog = fallbackCatalog
		x.syntheticCatalog = false
		if x.trailer == nil && fallbackCatalogObjNum >= 0 {
			trailer := entity.NewDict()
			trailer.Set(entity.Name("/Root"), entity.NewRef(uint32(fallbackCatalogObjNum), 0))
			trailer.Set(entity.Name("/Size"), entity.NewInteger(int64(len(x.entries))))
			x.trailer = trailer
		}
		return nil
	}

	// No catalog found, build minimal structure.
	return x.buildCatalogFromScan(x.stream)
}

// buildCatalogFromScan builds a minimal catalog by scanning for key objects.
func (x *Table) buildCatalogFromScan(data []byte) error {
	// Look for catalog-like patterns
	// "/Type /Catalog" indicates a catalog dictionary
	if idx := bytes.Index(data, []byte("/Type /Catalog")); idx != -1 {
		// Found catalog - create a minimal structure
		catalog := entity.NewDict()
		catalog.Set(entity.Name("/Type"), entity.Name("Catalog"))

		// Try to find Pages reference
		// Look for "/Pages" before or after the catalog
		contextEnd := idx + 200
		if contextEnd > len(data) {
			contextEnd = len(data)
		}

		context := data[idx:contextEnd]
		if pagesIdx := bytes.Index(context, []byte("/Pages")); pagesIdx != -1 {
			// Found Pages reference - for now just create a placeholder
			pagesDict := entity.NewDict()
			pagesDict.Set(entity.Name("Type"), entity.Name("Pages"))
			pagesDict.Set(entity.Name("Count"), entity.NewInteger(1))
			pagesDict.Set(entity.Name("Kids"), entity.NewArray())
			catalog.Set(entity.Name("Pages"), pagesDict)
		}

		x.catalog = catalog
		x.syntheticCatalog = true

		// Preserve existing trailer from the parsed file when available.
		if x.trailer == nil {
			trailer := entity.NewDict()
			trailer.Set(entity.Name("/Root"), catalog)
			x.trailer = trailer
		}

		return nil
	}

	// Fallback - create minimal catalog
	catalog := entity.NewDict()
	catalog.Set(entity.Name("/Type"), entity.Name("Catalog"))
	pagesDict := entity.NewDict()
	pagesDict.Set(entity.Name("/Type"), entity.Name("Pages"))
	pagesDict.Set(entity.Name("/Count"), entity.NewInteger(1))
	pagesDict.Set(entity.Name("/Kids"), entity.NewArray())
	catalog.Set(entity.Name("/Pages"), pagesDict)

	x.catalog = catalog
	x.syntheticCatalog = true

	// Preserve existing trailer from the parsed file when available.
	if x.trailer == nil {
		trailer := entity.NewDict()
		trailer.Set(entity.Name("/Root"), catalog)
		x.trailer = trailer
	}

	return nil
}

// buildCatalogAndPagesFromScan builds catalog and page tree by scanning for page objects.
// This is used for PDFs with XRef streams where we can't parse the stream directly.
func (x *Table) buildCatalogAndPagesFromScan(data []byte) error {
	pageRefs := x.collectPageRefsFromEntries()
	if rawPageRefs := x.collectPageRefsFromRawPatterns(data); len(rawPageRefs) > len(pageRefs) {
		pageRefs = rawPageRefs
	}

	if len(pageRefs) == 0 {
		// No pages found, fall back to minimal catalog
		return x.buildCatalogFromScan(data)
	}

	// Create catalog and pages structure
	catalog := entity.NewDict()
	catalog.Set(entity.Name("/Type"), entity.Name("Catalog"))

	// Create Kids array with page references
	kidsItems := make([]entity.Object, len(pageRefs))
	for i, pageRef := range pageRefs {
		kidsItems[i] = pageRef
	}
	kidsArray := entity.NewArray(kidsItems...)

	pagesDict := entity.NewDict()
	pagesDict.Set(entity.Name("/Type"), entity.Name("Pages"))
	pagesDict.Set(entity.Name("/Count"), entity.NewInteger(int64(len(pageRefs))))
	pagesDict.Set(entity.Name("/Kids"), kidsArray)

	catalog.Set(entity.Name("/Pages"), pagesDict)

	x.catalog = catalog

	// Preserve existing trailer from the parsed file when available.
	if x.trailer == nil {
		trailer := entity.NewDict()
		trailer.Set(entity.Name("/Root"), catalog)
		trailer.Set(entity.Name("/Size"), entity.NewInteger(int64(len(pageRefs))))
		x.trailer = trailer
	}

	return nil
}

func (x *Table) detectLinearizedPageCount() int {
	for objNum, entry := range x.entries {
		if entry == nil || entry.Free || entry.Type != repository.EntryTypeUncompressed {
			continue
		}

		ref := entity.NewRef(uint32(objNum), entry.Generation)
		obj, err := x.parseObjectAt(entry.Offset, ref)
		if err != nil {
			continue
		}

		dict, ok := objectAsDict(obj)
		if !ok || dict.Get(entity.Name("/Linearized")) == nil {
			continue
		}

		nVal := dict.Get(entity.Name("/N"))
		nObj, ok := nVal.(*entity.Integer)
		if !ok || nObj.Value() <= 0 {
			continue
		}

		return int(nObj.Value())
	}

	return 0
}

type pageCandidate struct {
	ref    entity.Ref
	offset uint64
}

func (x *Table) collectPageRefsFromEntries() []entity.Ref {
	candidates := make([]pageCandidate, 0)
	seen := make(map[entity.Ref]struct{})

	for objNum, entry := range x.entries {
		if entry == nil || entry.Free || entry.Type != repository.EntryTypeUncompressed {
			continue
		}

		ref := entity.NewRef(uint32(objNum), entry.Generation)
		start := int(entry.Offset)
		end := x.findObjectEnd(x.stream, start)
		if end > start && end <= len(x.stream) {
			if hasPageTypeToken(x.stream[start:end]) {
				if _, exists := seen[ref]; !exists {
					seen[ref] = struct{}{}
					candidates = append(candidates, pageCandidate{ref: ref, offset: entry.Offset})
				}
				continue
			}
		}

		obj, err := x.parseObjectAt(entry.Offset, ref)
		if err != nil {
			continue
		}

		dict, ok := obj.(*entity.Dict)
		if !ok || !isPageDict(dict) {
			continue
		}

		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		candidates = append(candidates, pageCandidate{ref: ref, offset: entry.Offset})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].offset == candidates[j].offset {
			return candidates[i].ref.Num() < candidates[j].ref.Num()
		}
		return candidates[i].offset < candidates[j].offset
	})

	pageRefs := make([]entity.Ref, 0, len(candidates))
	for _, candidate := range candidates {
		pageRefs = append(pageRefs, candidate.ref)
	}

	return pageRefs
}

func hasPageTypeToken(objData []byte) bool {
	patterns := [][]byte{
		[]byte("/Type/Page"),
		[]byte("/Type /Page"),
	}

	for _, pattern := range patterns {
		searchPos := 0
		for searchPos < len(objData) {
			idxRel := bytes.Index(objData[searchPos:], pattern)
			if idxRel == -1 {
				break
			}
			idx := searchPos + idxRel
			next := idx + len(pattern)
			if next >= len(objData) || (objData[next] != 's' && objData[next] != 'S') {
				return true
			}
			searchPos = idx + 1
		}
	}

	return false
}

func (x *Table) collectPageRefsFromRawPatterns(data []byte) []entity.Ref {
	patterns := [][]byte{
		[]byte("/Type/Page"),
		[]byte("/Type /Page"),
	}

	candidates := make([]pageCandidate, 0)
	seen := make(map[entity.Ref]struct{})

	for _, pattern := range patterns {
		searchPos := 0
		for searchPos < len(data) {
			idxRel := bytes.Index(data[searchPos:], pattern)
			if idxRel == -1 {
				break
			}
			pageTypePos := searchPos + idxRel
			searchPos = pageTypePos + len(pattern)

			headerSearchStart := pageTypePos - 128
			if headerSearchStart < 0 {
				headerSearchStart = 0
			}
			objKeywordRel := bytes.LastIndex(data[headerSearchStart:pageTypePos], []byte("obj"))
			if objKeywordRel == -1 {
				continue
			}
			objKeywordPos := headerSearchStart + objKeywordRel
			if objKeywordPos <= 0 {
				continue
			}

			lookBehind := objKeywordPos - 1
			genNum, newPos := parseNumberBackward(data, lookBehind)
			if newPos < 0 || genNum < 0 || genNum > 65535 {
				continue
			}

			lookBehind = newPos
			for lookBehind >= 0 && isWhitespace(data[lookBehind]) {
				lookBehind--
			}

			objNum, newPos := parseNumberBackward(data, lookBehind)
			if newPos < 0 || objNum < 0 || objNum > int64(^uint32(0)) {
				continue
			}

			ref := entity.NewRef(uint32(objNum), uint16(genNum))
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			candidates = append(candidates, pageCandidate{
				ref:    ref,
				offset: uint64(newPos + 1),
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].offset == candidates[j].offset {
			return candidates[i].ref.Num() < candidates[j].ref.Num()
		}
		return candidates[i].offset < candidates[j].offset
	})

	pageRefs := make([]entity.Ref, 0, len(candidates))
	for _, candidate := range candidates {
		// Prefer currently active xref entry generation for each object number.
		// Raw pattern scan can discover stale generations from incremental updates.
		if int(candidate.ref.Num()) < len(x.entries) {
			entry := x.entries[candidate.ref.Num()]
			if entry != nil && !entry.Free && entry.Type == repository.EntryTypeUncompressed {
				if entry.Generation != candidate.ref.Gen() {
					continue
				}
				parsedObj, ok := x.cache[candidate.ref]
				if !ok {
					var err error
					parsedObj, err = x.parseObjectAt(entry.Offset, candidate.ref)
					if err != nil {
						continue
					}
				}
				pageDict, dictOK := objectAsDict(parsedObj)
				if !dictOK || !isPageDict(pageDict) {
					continue
				}
				pageRefs = append(pageRefs, candidate.ref)
				continue
			}
		}

		// Fallback validation when entry metadata is missing: require a parsable Page dict.
		if !isLikelyObjectHeaderStart(data, int(candidate.offset)) {
			continue
		}
		obj, err := x.parseObjectAt(candidate.offset, candidate.ref)
		if err != nil {
			continue
		}
		dict, ok := objectAsDict(obj)
		if !ok || !isPageDict(dict) {
			continue
		}
		pageRefs = append(pageRefs, candidate.ref)
	}

	return pageRefs
}

func objectAsDict(obj entity.Object) (*entity.Dict, bool) {
	if dict, ok := obj.(*entity.Dict); ok {
		return dict, true
	}
	if streamObj, ok := obj.(*entity.Stream); ok {
		dict := streamObj.Dict()
		return dict, dict != nil
	}
	return nil, false
}

func (x *Table) catalogHasResolvablePages(catalog *entity.Dict) bool {
	if catalog == nil {
		return false
	}

	pagesRaw := catalog.GetRaw(entity.Name("/Pages"))
	if pagesRaw == nil {
		pagesRaw = catalog.GetRaw(entity.Name("Pages"))
	}
	switch raw := pagesRaw.(type) {
	case entity.Ref:
		obj, err := x.Fetch(raw)
		if err != nil {
			return false
		}
		pagesDict, ok := obj.(*entity.Dict)
		if !ok {
			return false
		}
		return x.pagesDictLooksValid(pagesDict) && x.pagesCountConsistentWithParentRefs(raw, pagesDict)
	case *entity.Dict:
		return x.pagesDictLooksValid(raw)
	}

	pagesVal := catalog.Get(entity.Name("/Pages"))
	if pagesVal == nil {
		pagesVal = catalog.Get(entity.Name("Pages"))
	}
	if pagesVal == nil {
		return false
	}

	switch v := pagesVal.(type) {
	case *entity.Dict:
		return x.pagesDictLooksValid(v)
	case entity.Ref:
		obj, err := x.Fetch(v)
		if err != nil {
			return false
		}
		pagesDict, ok := obj.(*entity.Dict)
		if !ok {
			return false
		}
		return x.pagesDictLooksValid(pagesDict) && x.pagesCountConsistentWithParentRefs(v, pagesDict)
	default:
		return false
	}
}

func (x *Table) pagesCountConsistentWithParentRefs(pagesRef entity.Ref, pagesDict *entity.Dict) bool {
	countVal := pagesDict.Get(entity.Name("/Count"))
	if countVal == nil {
		countVal = pagesDict.Get(entity.Name("Count"))
	}
	countObj, ok := countVal.(*entity.Integer)
	if !ok {
		return true
	}

	declaredCount := int(countObj.Value())
	if declaredCount <= 0 {
		return true
	}

	parentMentions := countParentRefMentions(x.stream, pagesRef)
	if parentMentions <= 0 {
		return true
	}

	return declaredCount >= parentMentions
}

func (x *Table) pagesDictLooksValid(dict *entity.Dict) bool {
	if !isPagesDict(dict) {
		return false
	}

	kidsVal := dict.Get(entity.Name("/Kids"))
	if kidsVal == nil {
		kidsVal = dict.Get(entity.Name("Kids"))
	}
	kids, ok := kidsVal.(*entity.Array)
	if !ok || kids.Len() == 0 {
		return false
	}

	if rawPageRefs := x.collectPageRefsFromRawPatterns(x.stream); len(rawPageRefs) > 0 {
		countVal := dict.Get(entity.Name("/Count"))
		if countVal == nil {
			countVal = dict.Get(entity.Name("Count"))
		}
		if countObj, ok := countVal.(*entity.Integer); ok {
			if int(countObj.Value()) > 0 && int(countObj.Value()) < len(rawPageRefs) {
				return false
			}
		}
	}

	for i := 0; i < kids.Len(); i++ {
		kid := kids.Get(i)
		switch v := kid.(type) {
		case *entity.Dict:
			if isPageDict(v) || isPagesDict(v) {
				return true
			}
		case entity.Ref:
			obj, err := x.Fetch(v)
			if err != nil {
				continue
			}
			kidDict, ok := obj.(*entity.Dict)
			if !ok {
				continue
			}
			if isPageDict(kidDict) || isPagesDict(kidDict) {
				return true
			}
		}
	}

	return false
}

func isPagesDict(dict *entity.Dict) bool {
	if dict == nil {
		return false
	}

	typeVal := dict.Get(entity.Name("/Type"))
	if typeVal == nil {
		typeVal = dict.Get(entity.Name("Type"))
	}
	if typeName, ok := typeVal.(entity.Name); ok {
		if typeName == entity.Name("Pages") || typeName == entity.Name("/Pages") {
			return true
		}
		if typeName == entity.Name("Page") || typeName == entity.Name("/Page") {
			return false
		}
	}

	return dict.Get(entity.Name("/Kids")) != nil || dict.Get(entity.Name("Kids")) != nil
}

func isPageDict(dict *entity.Dict) bool {
	if dict == nil {
		return false
	}

	typeVal := dict.Get(entity.Name("/Type"))
	if typeVal == nil {
		typeVal = dict.Get(entity.Name("Type"))
	}
	if typeName, ok := typeVal.(entity.Name); ok {
		if typeName == entity.Name("Page") || typeName == entity.Name("/Page") {
			return true
		}
		if typeName == entity.Name("Pages") || typeName == entity.Name("/Pages") {
			return false
		}
	}

	parentVal := dict.Get(entity.Name("/Parent"))
	if parentVal == nil {
		parentVal = dict.Get(entity.Name("Parent"))
	}
	contentsVal := dict.Get(entity.Name("/Contents"))
	if contentsVal == nil {
		contentsVal = dict.Get(entity.Name("Contents"))
	}
	mediaBoxVal := dict.Get(entity.Name("/MediaBox"))
	if mediaBoxVal == nil {
		mediaBoxVal = dict.Get(entity.Name("MediaBox"))
	}

	return parentVal != nil && (contentsVal != nil || mediaBoxVal != nil)
}

// parseTrailer parses the trailer dictionary.
func (x *Table) parseTrailer() error {
	// Find "trailer" keyword
	idx := bytes.LastIndex(x.stream, []byte("trailer"))
	if idx == -1 {
		return fmt.Errorf("trailer not found")
	}

	// Parse trailer dictionary
	// Use bytes.Reader directly to avoid buffering issues
	reader := bytes.NewReader(x.stream[idx+7:])
	lexer := parser.NewLexer(reader)

	p := parser.NewParser(lexer, x)
	obj, err := p.ParseObject()
	if err != nil {
		return err
	}

	dict, ok := obj.(*entity.Dict)
	if !ok {
		return fmt.Errorf("trailer is not a dictionary")
	}

	x.trailer = dict

	// Extract file ID for encryption
	if idVal := dict.Get(entity.Name("/ID")); idVal != nil {
		if arr, ok := idVal.(*entity.Array); ok && arr.Len() > 0 {
			if id := arr.Get(0); id != nil {
				if str, ok := id.(*entity.String); ok {
					x.fileID = stringToBytes(str)
				}
			}
		}
	}

	x.trailer = dict
	return nil
}

// resolveCatalog resolves the catalog dictionary from the trailer.
func (x *Table) resolveCatalog() error {
	if x.trailer == nil {
		return fmt.Errorf("trailer not parsed")
	}

	tryScanFallback := func(rootErr error) error {
		if err := x.findCatalogInScannedObjects(); err == nil && x.catalog != nil {
			return nil
		}
		return rootErr
	}

	// Get /Root from trailer
	// Note: Get() auto-dereferences, so it might return the catalog directly
	rootVal := x.trailer.Get(entity.Name("/Root"))
	if rootVal == nil {
		return tryScanFallback(fmt.Errorf("catalog root not found"))
	}

	// Check if it's already a catalog dictionary (auto-dereferenced)
	if catalog, ok := rootVal.(*entity.Dict); ok {
		// Verify it's actually a catalog
		if typeVal := catalog.Get(entity.Name("/Type")); typeVal == entity.Name("Catalog") || typeVal == entity.Name("/Catalog") {
			x.catalog = catalog
			return nil
		}
		return tryScanFallback(fmt.Errorf("catalog dictionary has invalid type"))
	}

	// Otherwise, it should be a reference that needs to be fetched
	ref, ok := rootVal.(entity.Ref)
	if !ok {
		return tryScanFallback(fmt.Errorf("catalog root is not a reference or catalog"))
	}

	// Fetch catalog object
	obj, err := x.Fetch(ref)
	if err != nil {
		return tryScanFallback(err)
	}

	catalog, ok := obj.(*entity.Dict)
	if !ok {
		return tryScanFallback(fmt.Errorf("catalog is not a dictionary"))
	}

	x.catalog = catalog
	return nil
}

// Fetch retrieves the object at the given reference.
func (x *Table) Fetch(ref entity.Ref) (entity.Object, error) {
	// Check cache first
	if obj, ok := x.cache[ref]; ok {
		return obj, nil
	}

	// Check if entry exists
	objNum := int(ref.Num())
	if objNum >= len(x.entries) || x.entries[objNum] == nil {
		if x.tryRecoverUncompressedEntry(ref, 0) {
			return x.Fetch(ref)
		}
		if x.tryRecoverCompressedEntry(ref) {
			return x.Fetch(ref)
		}
		return nil, errors.NotFoundf("xref_fetch", "object %d not found", ref.Num())
	}

	entry := x.entries[objNum]

	if entry.Free {
		return nil, errors.NotFoundf("xref_fetch", "object %d is free", ref.Num())
	}

	if entry.Type == repository.EntryTypeUncompressed {
		// Parse object from offset
		obj, err := x.parseObjectAt(entry.Offset, ref)
		if err != nil {
			if x.tryRecoverUncompressedEntry(ref, entry.Offset) {
				return x.Fetch(ref)
			}
			if x.tryRecoverCompressedEntry(ref) {
				return x.Fetch(ref)
			}
			return nil, err
		}

		// Cache the parsed object
		x.cache[ref] = obj
		return obj, nil
	}

	if entry.Type == repository.EntryTypeCompressed {
		// Object is compressed in an object stream
		// entry.ObjectStreamNumber contains the object stream number
		// entry.ObjectStreamIndex contains the index within the stream
		obj, err := x.parseObjectStream(entry.ObjectStreamNumber, entry.ObjectStreamIndex, ref)
		if err != nil {
			return nil, err
		}

		// Cache the parsed object
		x.cache[ref] = obj
		return obj, nil
	}

	if x.tryRecoverUncompressedEntry(ref, 0) {
		return x.Fetch(ref)
	}
	if x.tryRecoverCompressedEntry(ref) {
		return x.Fetch(ref)
	}

	return nil, errors.NotFoundf("xref_fetch", "object %d has unsupported type", ref.Num())
}

// tryRecoverUncompressedEntry scans raw PDF bytes for a missing indirect object header.
// This recovers malformed xref tables that miss valid "N G obj" entries.
func (x *Table) tryRecoverUncompressedEntry(ref entity.Ref, skipOffset uint64) bool {
	objNum := int(ref.Num())
	if objNum < 0 {
		return false
	}

	offset, gen, ok := x.findUncompressedObjectOffset(objNum, ref.Gen(), skipOffset)
	if !ok {
		return false
	}

	for len(x.entries) <= objNum {
		x.entries = append(x.entries, nil)
	}

	x.entries[objNum] = &repository.XRefEntry{
		Offset:     offset,
		Generation: gen,
		Type:       repository.EntryTypeUncompressed,
		Free:       false,
	}

	return true
}

// findUncompressedObjectOffset finds "objNum gen obj" in the raw byte stream.
// It prefers the expected generation and falls back to the first matching header.
func (x *Table) findUncompressedObjectOffset(objNum int, expectedGen uint16, skipOffset uint64) (uint64, uint16, bool) {
	if objNum < 0 || len(x.stream) == 0 {
		return 0, 0, false
	}

	target := []byte(strconv.Itoa(objNum))
	searchPos := 0
	foundFallback := false
	var fallbackOffset uint64
	var fallbackGen uint16

	for searchPos < len(x.stream) {
		idxRel := bytes.Index(x.stream[searchPos:], target)
		if idxRel == -1 {
			break
		}
		idx := searchPos + idxRel
		searchPos = idx + len(target)

		if skipOffset > 0 && uint64(idx) == skipOffset {
			continue
		}
		if idx > 0 && isDigitByte(x.stream[idx-1]) {
			continue
		}
		if !isLikelyObjectHeaderStart(x.stream, idx) {
			continue
		}

		reader := bytes.NewReader(x.stream[idx:])
		lexer := parser.NewLexer(reader)

		tokenObjNum, err := lexer.NextToken()
		if err != nil || tokenObjNum.Type != parser.TokenNumber {
			continue
		}
		if parseInt64(tokenObjNum.Value) != int64(objNum) {
			continue
		}

		tokenGen, err := lexer.NextToken()
		if err != nil || tokenGen.Type != parser.TokenNumber {
			continue
		}
		genValue := parseInt64(tokenGen.Value)
		if genValue < 0 || genValue > 65535 {
			continue
		}
		gen := uint16(genValue)

		tokenObjKeyword, err := lexer.NextToken()
		if err != nil || tokenObjKeyword.Type != parser.TokenKeyword || tokenObjKeyword.Value != "obj" {
			continue
		}
		if x.findObjectEnd(x.stream, idx+3) == -1 {
			continue
		}

		if gen == expectedGen {
			return uint64(idx), gen, true
		}

		if !foundFallback {
			foundFallback = true
			fallbackOffset = uint64(idx)
			fallbackGen = gen
		}
	}

	if foundFallback {
		return fallbackOffset, fallbackGen, true
	}

	return 0, 0, false
}

func (x *Table) tryRecoverCompressedEntry(ref entity.Ref) bool {
	refNum := ref.Num()
	if loc, ok := x.objStreamLocations[refNum]; ok {
		return x.setRecoveredCompressedEntry(refNum, loc)
	}

	if !x.objStreamIndexBuilt {
		x.buildObjectStreamLocations()
		x.objStreamIndexBuilt = true
		if loc, ok := x.objStreamLocations[refNum]; ok {
			return x.setRecoveredCompressedEntry(refNum, loc)
		}
	}

	return false
}

func (x *Table) setRecoveredCompressedEntry(refNum uint32, loc objectStreamLocation) bool {
	objNum := int(refNum)
	if objNum < 0 {
		return false
	}
	for len(x.entries) <= objNum {
		x.entries = append(x.entries, nil)
	}
	x.entries[objNum] = &repository.XRefEntry{
		Type:               repository.EntryTypeCompressed,
		Free:               false,
		ObjectStreamNumber: loc.streamNum,
		ObjectStreamIndex:  loc.index,
	}
	return true
}

func (x *Table) buildObjectStreamLocations() {
	for streamObjNum, entry := range x.entries {
		if entry == nil || entry.Free || entry.Type != repository.EntryTypeUncompressed {
			continue
		}

		ref := entity.NewRef(uint32(streamObjNum), entry.Generation)
		obj, err := x.parseObjectAt(entry.Offset, ref)
		if err != nil {
			continue
		}

		streamObj, ok := obj.(*entity.Stream)
		if !ok {
			continue
		}

		dict := streamObj.Dict()
		if dict == nil {
			continue
		}
		typeVal := dict.Get(entity.Name("/Type"))
		if typeVal != entity.Name("ObjStm") && typeVal != entity.Name("/ObjStm") {
			continue
		}

		nVal, ok := dict.Get(entity.Name("/N")).(*entity.Integer)
		if !ok || nVal.Value() <= 0 {
			continue
		}
		firstVal, ok := dict.Get(entity.Name("/First")).(*entity.Integer)
		if !ok || firstVal.Value() < 0 {
			continue
		}

		decoded, err := pdfstream.NewFromEntity(streamObj).Decode()
		if err != nil {
			continue
		}

		firstOffset := int(firstVal.Value())
		count := int(nVal.Value())
		if firstOffset <= 0 || firstOffset > len(decoded) {
			continue
		}

		header := decoded[:firstOffset]
		lexer := parser.NewLexer(bytes.NewReader(header))

		for i := 0; i < count; i++ {
			objNumTok, err := lexer.NextToken()
			if err != nil || objNumTok.Type != parser.TokenNumber {
				break
			}
			offsetTok, err := lexer.NextToken()
			if err != nil || offsetTok.Type != parser.TokenNumber {
				break
			}

			objNum := parseInt64(objNumTok.Value)
			if objNum < 0 || objNum > int64(^uint32(0)) {
				continue
			}
			locRef := uint32(objNum)
			if _, exists := x.objStreamLocations[locRef]; exists {
				continue
			}
			x.objStreamLocations[locRef] = objectStreamLocation{
				streamNum: uint32(streamObjNum),
				index:     uint16(i),
			}
		}
	}
}

// FetchCached retrieves a cached object without parsing.
func (x *Table) FetchCached(ref entity.Ref) (entity.Object, bool) {
	obj, ok := x.cache[ref]
	return obj, ok
}

// Cache stores an object in the cache.
func (x *Table) Cache(ref entity.Ref, obj entity.Object) {
	x.cache[ref] = obj
}

// GetCatalog returns the catalog dictionary.
func (x *Table) GetCatalog() (*entity.Dict, error) {
	if x.catalog == nil {
		return nil, errors.NotFound("get_catalog", fmt.Errorf("catalog not resolved"))
	}
	return x.catalog, nil
}

// GetTrailer returns the trailer dictionary.
func (x *Table) GetTrailer() (*entity.Dict, error) {
	if x.trailer == nil {
		return nil, errors.NotFound("get_trailer", fmt.Errorf("trailer not parsed"))
	}
	return x.trailer, nil
}

// GetNumObjects returns the number of objects in the XRef table.
func (x *Table) GetNumObjects() int {
	return len(x.entries)
}

// RawData returns the original PDF byte stream.
func (x *Table) RawData() []byte {
	return x.stream
}

// StartXRefOffset returns the latest startxref offset from the original stream.
func (x *Table) StartXRefOffset() (uint64, error) {
	return x.findStartXRef()
}

// parseObjectAt parses an indirect object at the given offset.
func (x *Table) parseObjectAt(offset uint64, ref entity.Ref) (entity.Object, error) {
	if int(offset) >= len(x.stream) {
		return nil, fmt.Errorf("object offset out of bounds")
	}

	reader := bytes.NewReader(x.stream[offset:])
	lexer := parser.NewLexer(reader)

	// Parse object number and generation (should match what we're looking for)
	// Format: "N N obj"
	token1, err := lexer.NextToken()
	if err != nil {
		return nil, err
	}
	if token1.Type != parser.TokenNumber {
		return nil, fmt.Errorf("expected object number, got %s", token1.Type)
	}

	token2, err := lexer.NextToken()
	if err != nil {
		return nil, err
	}
	if token2.Type != parser.TokenNumber {
		return nil, fmt.Errorf("expected generation number, got %s", token2.Type)
	}

	token3, err := lexer.NextToken()
	if err != nil {
		return nil, err
	}
	if token3.Type != parser.TokenKeyword || token3.Value != "obj" {
		return nil, fmt.Errorf("expected 'obj', got %s %q", token3.Type, token3.Value)
	}

	// Now parse the actual object content using the parser
	p := parser.NewParser(lexer, x)
	obj, err := p.ParseObject()
	if err != nil {
		return nil, err
	}

	// If this is a stream object, extract raw stream bytes and wrap as entity.Stream.
	// The parser returns the dictionary part first; stream data lives in the raw PDF bytes.
	if dict, ok := obj.(*entity.Dict); ok {
		if streamData, streamErr := x.extractStreamDataFromDict(dict, offset); streamErr == nil {
			obj = entity.NewStream(dict, streamData)
		}
	}

	// Decrypt the object if encryption is enabled and this is not the encryption dict itself
	if x.encryption != nil && x.encryption.IsAuthenticated() {
		// Don't decrypt the encryption dictionary itself (usually object 1)
		if ref.Num() != 1 {
			obj = x.decryptObject(obj, ref)
		}
	}

	return obj, nil
}

// decryptObject decrypts an object if it's encrypted.
func (x *Table) decryptObject(obj entity.Object, ref entity.Ref) entity.Object {
	if x.encryption == nil {
		return obj
	}

	switch o := obj.(type) {
	case *entity.String:
		// Decrypt string
		// Convert string value to bytes
		strData := []byte(o.Value())
		if decrypted, err := x.encryption.DecryptStringForObject(strData, ref.Num(), ref.Gen()); err == nil {
			return entity.NewString(string(decrypted))
		}
	case *entity.Dict:
		// Check if this is a stream dictionary
		// Stream data will be decrypted separately
		return x.decryptDict(o, ref)
	case *entity.Stream:
		decryptedDict := o.Dict()
		if decryptedDict != nil {
			decryptedDict = x.decryptDict(decryptedDict, ref)
		}
		decryptedData, err := x.encryption.DecryptStream(o.RawBytes(), ref.Num(), ref.Gen())
		if err != nil {
			return entity.NewStream(decryptedDict, o.RawBytes())
		}
		return entity.NewStream(decryptedDict, decryptedData)
	case *entity.Array:
		// Recursively decrypt array elements
		// Create a new array with decrypted elements
		decryptedItems := make([]entity.Object, o.Len())
		for i := 0; i < o.Len(); i++ {
			if val := o.Get(i); val != nil {
				decryptedItems[i] = x.decryptObject(val, ref)
			} else {
				decryptedItems[i] = nil
			}
		}
		return entity.NewArray(decryptedItems...)
	}

	return obj
}

// decryptDict decrypts dictionary contents (not the stream data).
func (x *Table) decryptDict(dict *entity.Dict, ref entity.Ref) *entity.Dict {
	// Stream decryption happens separately when extracting stream data
	// This just decrypts string values in the dictionary
	for _, key := range dict.Keys() {
		val := dict.Get(key)
		if str, ok := val.(*entity.String); ok {
			// Convert string value to bytes
			strData := []byte(str.Value())
			if decrypted, err := x.encryption.DecryptStringForObject(strData, ref.Num(), ref.Gen()); err == nil {
				dict.Set(key, entity.NewString(string(decrypted)))
			}
		}
	}
	return dict
}

// ReadUint32 reads a big-endian uint32 from a byte slice.
func ReadUint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}

// ReadUint16 reads a big-endian uint16 from a byte slice.
func ReadUint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

// parseObjectStream parses an object from an object stream.
// streamNum is the object number of the object stream
// index is the index of the object within the stream (0-based)
// ref is the reference of the object being extracted
func (x *Table) parseObjectStream(streamNum uint32, index uint16, ref entity.Ref) (entity.Object, error) {
	// Get the offset of the object stream
	objNum := int(streamNum)
	if objNum >= len(x.entries) || x.entries[objNum] == nil {
		return nil, fmt.Errorf("object stream %d not found in XRef", streamNum)
	}

	// Fetch the object stream object
	streamRef := entity.NewRef(streamNum, 0)
	streamObj, err := x.Fetch(streamRef)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch object stream %d: %w", streamNum, err)
	}

	var (
		streamDict  *entity.Dict
		decodedData []byte
	)

	switch v := streamObj.(type) {
	case *entity.Stream:
		streamDict = v.Dict()
		infraStream := pdfstream.NewFromEntity(v)
		decodedData, err = infraStream.Decode()
		if err != nil {
			return nil, fmt.Errorf("failed to decode object stream %d: %w", streamNum, err)
		}
	case *entity.Dict:
		// Backward-compatible fallback path if the object is still parsed as a plain dict.
		streamDict = v
		streamOffset := x.entries[objNum].Offset
		streamData, extractErr := x.extractStreamDataFromDict(streamDict, streamOffset)
		if extractErr != nil {
			return nil, fmt.Errorf("failed to extract object stream data: %w", extractErr)
		}
		decodedData, err = x.decodeStream(streamData, streamDict)
		if err != nil {
			return nil, fmt.Errorf("failed to decode object stream: %w", err)
		}
	default:
		return nil, fmt.Errorf("object stream %d has invalid type %T", streamNum, streamObj)
	}

	// Check if it's an object stream
	typeVal := streamDict.Get(entity.Name("/Type"))
	if typeVal != entity.Name("ObjStm") {
		return nil, fmt.Errorf("object %d is not an object stream (Type=%v)", streamNum, typeVal)
	}

	// Get /N (number of objects in the stream)
	nVal := streamDict.Get(entity.Name("/N"))
	if nVal == nil {
		return nil, fmt.Errorf("object stream missing /N")
	}
	n, ok := nVal.(*entity.Integer)
	if !ok {
		return nil, fmt.Errorf("invalid /N type: %T", nVal)
	}
	numObjects := int(n.Value())

	if int(index) >= numObjects {
		return nil, fmt.Errorf("object index %d out of range (0-%d)", index, numObjects-1)
	}

	// Get /First (offset to first object in stream data)
	firstVal := streamDict.Get(entity.Name("/First"))
	if firstVal == nil {
		return nil, fmt.Errorf("object stream missing /First")
	}
	first, ok := firstVal.(*entity.Integer)
	if !ok {
		return nil, fmt.Errorf("invalid /First type: %T", firstVal)
	}
	firstOffset := int(first.Value())

	// Parse the object offsets from the beginning of the stream
	// The first part contains: objNum0 offset0 objNum1 offset1 objNum2 offset2 ...
	// Where offsets are from the start of the object data (at position First)
	offsets := make([]int, numObjects)
	objNumbers := make([]int64, numObjects)

	// Parse using lexer for robustness
	reader := bytes.NewReader(decodedData[:firstOffset])
	lexer := parser.NewLexer(reader)

	// Read interleaved object numbers and offsets
	for i := 0; i < numObjects; i++ {
		// Read object number
		token, err := lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("failed to read object number %d: %w", i, err)
		}
		if token.Type != parser.TokenNumber {
			return nil, fmt.Errorf("expected object number, got %s", token.Type)
		}
		objNum := parseInt64(token.Value)
		objNumbers[i] = objNum

		// Read offset
		token, err = lexer.NextToken()
		if err != nil {
			return nil, fmt.Errorf("failed to read offset %d: %w", i, err)
		}
		if token.Type != parser.TokenNumber {
			return nil, fmt.Errorf("expected offset, got %s", token.Type)
		}
		offset := parseInt64(token.Value)
		offsets[i] = int(offset)
	}

	// Find the object at the given index
	if int(index) >= len(offsets) {
		return nil, fmt.Errorf("object index %d out of range", index)
	}

	objStart := firstOffset + offsets[int(index)]
	var objEnd int
	if int(index)+1 < len(offsets) {
		objEnd = firstOffset + offsets[int(index)+1]
	} else {
		objEnd = len(decodedData)
	}

	// Extract and parse the object
	objData := decodedData[objStart:objEnd]

	// Parse the object
	objReader := bytes.NewReader(objData)
	objLexer := parser.NewLexer(objReader)
	objParser := parser.NewParser(objLexer, x)

	obj, err := objParser.ParseObject()
	if err != nil {
		return nil, err
	}

	// Decrypt the object if encryption is enabled
	if x.encryption != nil && x.encryption.IsAuthenticated() {
		obj = x.decryptObject(obj, ref)
	}

	return obj, nil
}

// extractStreamDataFromDict extracts stream data from a stream dictionary.
// objectOffset is the offset where the object is located (for position-based extraction)
func (x *Table) extractStreamDataFromDict(streamDict *entity.Dict, objectOffset uint64) ([]byte, error) {
	length, err := x.resolveStreamLength(streamDict)
	if err != nil {
		return nil, err
	}

	if length < 0 || int(length) > len(x.stream) {
		return nil, fmt.Errorf("invalid stream length: %d", length)
	}

	// Search for "stream" keyword starting from the known object offset.
	// Use "endstream" as the upper bound (not "endobj") because binary stream data
	// may contain the byte sequence "endobj", causing a false early boundary.
	searchStart := int(objectOffset)
	searchEnd := len(x.stream)
	if endStreamPos := bytes.Index(x.stream[searchStart:], []byte("endstream")); endStreamPos != -1 {
		searchEnd = searchStart + endStreamPos
	} else if endObjPos := bytes.Index(x.stream[searchStart:], []byte("endobj")); endObjPos != -1 {
		searchEnd = searchStart + endObjPos
	}

	streamPos := findStreamKeywordOffset(x.stream, searchStart, searchEnd)
	if streamPos < 0 {
		return nil, fmt.Errorf("could not find 'stream' keyword after offset %d", objectOffset)
	}

	streamPos = skipStreamKeywordEOL(x.stream, streamPos+len("stream"))

	// Check if we have enough data
	if streamPos+int(length) > len(x.stream) {
		return nil, fmt.Errorf("stream data exceeds file bounds")
	}

	// Extract the stream data
	return x.stream[streamPos : streamPos+int(length)], nil
}

func skipStreamKeywordEOL(data []byte, pos int) int {
	if pos >= len(data) {
		return pos
	}
	if data[pos] == '\r' {
		pos++
		if pos < len(data) && data[pos] == '\n' {
			pos++
		}
		return pos
	}
	if data[pos] == '\n' {
		return pos + 1
	}
	return pos
}

func findStreamKeywordOffset(data []byte, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	if start >= end {
		return -1
	}

	searchAt := start
	for searchAt < end {
		relative := bytes.Index(data[searchAt:end], []byte("stream"))
		if relative < 0 {
			return -1
		}

		pos := searchAt + relative
		// "stream" may directly follow ">>" (compact PDFs write `>>stream\r\n`)
		// or a whitespace character. Reject only if preceded by a non-separator
		// character that would make this a false keyword (e.g. "downstream").
		prevByte := byte(0)
		if pos > 0 {
			prevByte = data[pos-1]
		}
		beforeOK := pos == 0 || isWhitespaceByte(prevByte) || prevByte == '>'
		afterPos := pos + len("stream")
		afterOK := afterPos < len(data) && isWhitespaceByte(data[afterPos])
		if beforeOK && afterOK {
			return pos
		}
		searchAt = pos + len("stream")
	}

	return -1
}

func (x *Table) resolveStreamLength(streamDict *entity.Dict) (int64, error) {
	if streamDict == nil {
		return 0, fmt.Errorf("stream missing /Length")
	}

	lengthVal := streamDict.Get(entity.Name("/Length"))
	if lengthVal == nil {
		return 0, fmt.Errorf("stream missing /Length")
	}

	switch v := lengthVal.(type) {
	case *entity.Integer:
		return v.Value(), nil
	case *entity.Real:
		return int64(v.Value()), nil
	case entity.Ref:
		resolved, err := x.Fetch(v)
		if err != nil {
			return 0, fmt.Errorf("resolve /Length ref %d %d R: %w", v.Num(), v.Gen(), err)
		}
		switch resolvedValue := resolved.(type) {
		case *entity.Integer:
			return resolvedValue.Value(), nil
		case *entity.Real:
			return int64(resolvedValue.Value()), nil
		default:
			return 0, fmt.Errorf("invalid resolved /Length type: %T", resolved)
		}
	default:
		return 0, fmt.Errorf("invalid /Length type: %T", lengthVal)
	}
}

// isWhitespaceByte checks if a byte is a PDF whitespace character.
func isWhitespaceByte(b byte) bool {
	return b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20
}

// stringToBytes converts a PDF String to bytes.
func stringToBytes(str *entity.String) []byte {
	if str == nil {
		return nil
	}
	// For encrypted PDFs, the ID value is often hex encoded
	value := str.Value()
	if len(value) > 0 && value[0] == '<' {
		// Hex string
		hexStr := value[1 : len(value)-1]
		result := make([]byte, len(hexStr)/2)
		for i := 0; i < len(hexStr); i += 2 {
			if i+1 < len(hexStr) {
				var b byte
				switch {
				case hexStr[i] >= '0' && hexStr[i] <= '9':
					b = (hexStr[i] - '0') << 4
				case hexStr[i] >= 'a' && hexStr[i] <= 'f':
					b = (hexStr[i] - 'a' + 10) << 4
				case hexStr[i] >= 'A' && hexStr[i] <= 'F':
					b = (hexStr[i] - 'A' + 10) << 4
				}
				switch {
				case hexStr[i+1] >= '0' && hexStr[i+1] <= '9':
					b |= hexStr[i+1] - '0'
				case hexStr[i+1] >= 'a' && hexStr[i+1] <= 'f':
					b |= hexStr[i+1] - 'a' + 10
				case hexStr[i+1] >= 'A' && hexStr[i+1] <= 'F':
					b |= hexStr[i+1] - 'A' + 10
				}
				result[i/2] = b
			}
		}
		return result
	}
	// Regular string - convert to bytes
	return []byte(value)
}

// ParseEncryptionDict parses the encryption dictionary from the trailer.
func (x *Table) ParseEncryptionDict(password string) error {
	if x.trailer == nil {
		return fmt.Errorf("trailer not parsed")
	}

	// Get /Encrypt from trailer
	encryptVal := x.trailer.Get(entity.Name("/Encrypt"))
	if encryptVal == nil {
		return nil // No encryption
	}

	// Get the encryption dictionary
	var encryptDict *entity.Dict
	switch v := encryptVal.(type) {
	case entity.Ref:
		// Fetch the encryption dictionary
		obj, err := x.Fetch(v)
		if err != nil {
			return fmt.Errorf("failed to fetch encryption dictionary: %w", err)
		}
		var ok bool
		encryptDict, ok = obj.(*entity.Dict)
		if !ok {
			return fmt.Errorf("encryption dictionary is not a dictionary")
		}
	case *entity.Dict:
		encryptDict = v
	default:
		return fmt.Errorf("invalid /Encrypt type: %T", encryptVal)
	}

	// Get file ID for encryption
	var fileID []byte
	if idVal := x.trailer.Get(entity.Name("/ID")); idVal != nil {
		if arr, ok := idVal.(*entity.Array); ok && arr.Len() > 0 {
			if id := arr.Get(0); id != nil {
				if str, ok := id.(*entity.String); ok {
					fileID = stringToBytes(str)
				}
			}
		}
	}

	// Create encryption handler
	handler, err := crypto.CreateEncryptionHandlerFromDict(encryptDict, fileID, password)
	if err != nil {
		return fmt.Errorf("failed to create encryption handler: %w", err)
	}

	x.encryption = handler
	return nil
}

// IsEncrypted returns true if the PDF is encrypted.
func (x *Table) IsEncrypted() bool {
	if x.trailer == nil {
		return false
	}

	return x.trailer.Get(entity.Name("/Encrypt")) != nil
}

// IsAuthenticated returns true if the PDF has been successfully authenticated.
func (x *Table) IsAuthenticated() bool {
	return x.encryption != nil && x.encryption.IsAuthenticated()
}

// GetStreamData extracts decoded stream data from a stream dictionary reference.
// Returns the decoded bytes or an error if the stream cannot be extracted.
func (x *Table) GetStreamData(ref entity.Ref) ([]byte, error) {
	// Fetch the object
	obj, err := x.Fetch(ref)
	if err != nil {
		return nil, fmt.Errorf("fetch object: %w", err)
	}

	switch typed := obj.(type) {
	case *entity.Stream:
		decodedData, err := x.decodeStreamData(typed.Dict(), typed.RawBytes())
		if err != nil {
			return typed.RawBytes(), nil
		}
		return decodedData, nil
	case *entity.Dict:
		streamDict := typed

		// Get the entry to find the offset
		objNum := int(ref.Num())
		if objNum >= len(x.entries) || x.entries[objNum] == nil {
			return nil, fmt.Errorf("object not in xref table")
		}

		entry := x.entries[objNum]
		if entry.Free {
			return nil, fmt.Errorf("object is free")
		}

		// Extract raw stream data
		rawData, err := x.extractStreamDataFromDict(streamDict, entry.Offset)
		if err != nil {
			return nil, fmt.Errorf("extract stream data: %w", err)
		}

		// Decrypt if necessary
		if x.encryption != nil && x.encryption.IsAuthenticated() {
			if ref.Num() != 1 { // Don't decrypt encryption dict
				decrypted, err := x.encryption.DecryptStream(rawData, ref.Num(), ref.Gen())
				if err == nil {
					rawData = decrypted
				}
			}
		}

		// Decode filters
		decodedData, err := x.decodeStreamData(streamDict, rawData)
		if err != nil {
			// If decoding fails, return raw data
			return rawData, nil
		}
		return decodedData, nil
	default:
		return nil, fmt.Errorf("object is not a stream: %T", obj)
	}
}

// decodeStreamData decodes stream data using the filters specified in the dict.
func (x *Table) decodeStreamData(streamDict *entity.Dict, rawData []byte) ([]byte, error) {
	if streamDict == nil {
		return rawData, nil
	}

	streamObj := entity.NewStream(streamDict, rawData)
	return pdfstream.NewFromEntity(streamObj).Decode()
}
