package renderer

import (
	"crypto/sha256"
	"os"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/cff"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/truetype"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/type1"
)

type defaultFontCandidateResolver struct{}

func (defaultFontCandidateResolver) ResolveCandidate(e *Evaluator, dict *entity.Dict, subtype, baseFont string, embeddedFontData []byte, embeddedErr error) entity.Font {
	switch subtype {
	case "Type1":
		return e.resolveType1FontCandidate(baseFont, embeddedFontData, embeddedErr)
	case "CIDFontType0":
		return e.newEmbeddedCIDFontType0Candidate(embeddedFontData, embeddedErr)
	case "TrueType", "CIDFontType2":
		return e.newEmbeddedTrueTypeFont(embeddedFontData, embeddedErr)
	case "Type0":
		return e.resolveType0FontCandidate(dict, baseFont)
	case "Type3":
		return e.resolveType3FontCandidate(dict, baseFont)
	default:
		return nil
	}
}

func (e *Evaluator) resolveType1FontCandidate(baseFont string, embeddedFontData []byte, embeddedErr error) entity.Font {
	type1Mode := strings.TrimSpace(os.Getenv("PDF_DEBUG_TYPE1_MODE"))

	if shouldUseFallbackType1ForBaseFontDebug(baseFont) {
		if preferred, ok := e.preferredFallbackFont(baseFont); ok {
			return preferred
		}
		font, err := e.getDefaultFont(baseFont)
		if err == nil {
			return font
		}
	}

	if shouldUseEmbeddedType1ForBaseFontDebug(baseFont) {
		if font := e.newEmbeddedType1Font(embeddedFontData, embeddedErr); font != nil {
			return font
		}
	}

	if type1Mode == "fallback-first" {
		if preferred, ok := e.preferredFallbackFont(baseFont); ok {
			return preferred
		}
		return e.newEmbeddedType1Font(embeddedFontData, embeddedErr)
	}

	// Default: embedded-first — use the actual embedded Type1 font data
	if font := e.newEmbeddedType1Font(embeddedFontData, embeddedErr); font != nil {
		return font
	}
	if preferred, ok := e.preferredFallbackFont(baseFont); ok {
		return preferred
	}
	return nil
}

func (e *Evaluator) resolveType0FontCandidate(dict *entity.Dict, baseFont string) entity.Font {
	descendantDict, ok := e.resolveFirstDescendantFontDict(dict)
	if !ok || descendantDict == nil {
		return nil
	}

	font, err := e.getFontFromDict(descendantDict, baseFont)
	if err != nil {
		return nil
	}

	// CIDFontType2 descendants must be treated as CID fonts so text is split
	// into 2-byte CIDs before glyph lookup.
	subtypeName := nameValueForEncoding(descendantDict.Get(pdfNameSubtype))
	if subtypeName == "CIDFontType0" {
		font = e.wrapCIDFontType0CWithCIDToGIDMap(font)
	}
	if subtypeName == "CIDFontType2" {
		cidToGID := descendantDict.Get(pdfNameCIDToGIDMap)
		isIdentity := cidToGID == nil
		if cidToGIDName, ok := cidToGID.(entity.Name); ok && cidToGIDName.Value() == "Identity" {
			isIdentity = true
		}
		if font != nil && !font.IsCIDFont() {
			if cidToGIDMap, ok := e.parseCIDToGIDMap(cidToGID); ok {
				font = &cidToGIDMappedFont{
					base:     font,
					cidToGID: cidToGIDMap,
				}
				font = e.applyFontMetricsFromDict(descendantDict, font)
			} else if isIdentity {
				toUnicode := e.parseType0ToUnicodeMap(dict)
				embeddedFontData, embeddedErr := e.getEmbeddedFontData(descendantDict)
				preferToUnicodeCMap := shouldUseToUnicodeCMapForCIDIdentity(
					baseFont,
					toUnicode,
					embeddedFontData,
					embeddedErr,
				)
				font = &cidIdentityFont{
					base:                font,
					toUnicode:           toUnicode,
					preferToUnicodeCMap: preferToUnicodeCMap,
				}
				// Poppler's GfxCIDFont keeps /W and /DW advances keyed by CID.
				// Apply metrics after the wrapper so CharCodeToGlyph uses the
				// CID map rather than the embedded TrueType cmap glyph key.
				font = e.applyFontMetricsFromDict(descendantDict, font)
			}
		}
	}

	return font
}

func shouldUseToUnicodeCMapForCIDIdentity(baseFont string, toUnicode map[uint32]rune, embeddedFontData []byte, embeddedErr error) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_CID_IDENTITY_TOUNICODE_CMAP"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	case "auto":
		return shouldAutoUseToUnicodeCMapForCIDIdentity(baseFont, toUnicode, embeddedFontData, embeddedErr)
	default:
		return shouldAutoUseToUnicodeCMapForCIDIdentity(baseFont, toUnicode, embeddedFontData, embeddedErr)
	}
}

func shouldAutoUseToUnicodeCMapForCIDIdentity(baseFont string, toUnicode map[uint32]rune, embeddedFontData []byte, embeddedErr error) bool {
	if len(toUnicode) == 0 || !isLatinCIDIdentityFallbackFont(baseFont) || !isSimpleLatinToUnicodeMap(toUnicode) {
		return false
	}
	if embeddedErr != nil || len(embeddedFontData) == 0 {
		return true
	}
	_, err := truetype.NewFontFromBytes(embeddedFontData)
	return err != nil
}

func isLatinCIDIdentityFallbackFont(baseFont string) bool {
	switch strings.TrimSpace(stripSubsetPrefix(baseFont)) {
	case "ArialMT", "Arial", "Helvetica", "HelveticaNeue", "Times-Roman", "Courier":
		return true
	default:
		return false
	}
}

func isSimpleLatinToUnicodeMap(toUnicode map[uint32]rune) bool {
	if len(toUnicode) == 0 {
		return false
	}
	for _, r := range toUnicode {
		switch {
		case r >= 0x20 && r <= 0x7e:
			continue
		case r >= 0xa0 && r <= 0xff:
			continue
		case r == 0x2022 || r == 0x2013 || r == 0x2014:
			continue
		default:
			return false
		}
	}
	return true
}

func (e *Evaluator) wrapCIDFontType0CWithCIDToGIDMap(font entity.Font) entity.Font {
	if font == nil {
		return font
	}
	if font.IsCIDFont() {
		return font
	}
	if !shouldEnableCIDFontType0CCIDToGIDMapDebug() {
		return &cidDirectFont{base: font}
	}
	mapper, ok := font.(cidToGIDMapFont)
	if !ok {
		return &cidDirectFont{base: font}
	}
	cidToGID, ok := mapper.CIDToGIDMap()
	if !ok || len(cidToGID) == 0 {
		return &cidDirectFont{base: font}
	}
	return &cidToGIDMappedFont{
		base:     font,
		cidToGID: cidToGID,
	}
}

func shouldEnableCIDFontType0CCIDToGIDMapDebug() bool {
	// Poppler SplashOutputDev loads raw CIDFontType0/CIDFontType0C descendants
	// through loadCIDFont without passing a codeToGID map. Keep the reversed CFF
	// charset mapping as a diagnostic opt-in because OpenType CFF and TrueType
	// descendants use separate mapping paths.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_CIDTYPE0C_CIDTOGID_MAP"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (e *Evaluator) resolveFirstDescendantFontDict(dict *entity.Dict) (*entity.Dict, bool) {
	descendantFonts, ok := dict.Get(pdfNameDescendantFonts).(*entity.Array)
	if !ok || descendantFonts.Len() == 0 {
		return nil, false
	}

	descendant := descendantFonts.Get(0)
	if descendantDict, ok := descendant.(*entity.Dict); ok {
		return descendantDict, true
	}

	ref, ok := descendant.(entity.Ref)
	if !ok || e.xref == nil {
		return nil, false
	}

	resolved, err := e.xref.Fetch(ref)
	if err != nil {
		return nil, false
	}

	descendantDict, ok := resolved.(*entity.Dict)
	return descendantDict, ok
}

func (e *Evaluator) parseCIDToGIDMap(obj entity.Object) (map[uint32]uint32, bool) {
	stream, ok := e.resolveStreamObject(obj)
	if !ok {
		return nil, false
	}

	data, err := stream.Decode()
	if err != nil || len(data) == 0 {
		data = stream.RawBytes()
	}
	if len(data) < 2 {
		return nil, false
	}

	result := make(map[uint32]uint32)
	for i := 0; i+1 < len(data); i += 2 {
		gid := uint32(data[i])<<8 | uint32(data[i+1])
		if gid == 0 {
			continue
		}
		result[uint32(i/2)] = gid
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

// parseType0ToUnicodeMap parses the ToUnicode CMap stream from a Type0 font dict,
// returning a CID→Unicode rune mapping used to resolve glyph IDs via the TrueType cmap.
func (e *Evaluator) parseType0ToUnicodeMap(dict *entity.Dict) map[uint32]rune {
	if dict == nil {
		return nil
	}
	tuObj := dict.Get(pdfNameToUnicode)
	if tuObj == nil {
		return nil
	}
	stream, ok := e.resolveStreamObject(tuObj)
	if !ok {
		return nil
	}
	data, err := stream.Decode()
	if err != nil || len(data) == 0 {
		return nil
	}
	return parseCIDToUnicodeData(data)
}

// parseCIDToUnicodeData parses a ToUnicode CMap and returns a CID→rune map.
func parseCIDToUnicodeData(data []byte) map[uint32]rune {
	result := make(map[uint32]rune)
	tokens := tokenizeCMapData(data)
	i := 0
	for i < len(tokens) {
		switch tokens[i] {
		case "beginbfchar":
			i = parseBFCharCIDs(tokens, i+1, result)
		case "beginbfrange":
			i = parseBFRangeCIDs(tokens, i+1, result)
		default:
			i++
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func tokenizeCMapData(data []byte) []string {
	var tokens []string
	i := 0
	for i < len(data) {
		// Skip whitespace
		for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r' || data[i] == '\n') {
			i++
		}
		if i >= len(data) {
			break
		}
		if data[i] == '<' {
			// Hex string
			j := i + 1
			for j < len(data) && data[j] != '>' {
				j++
			}
			tokens = append(tokens, string(data[i:j+1]))
			i = j + 1
		} else if data[i] == '[' {
			// Array
			tokens = append(tokens, "[")
			i++
		} else if data[i] == ']' {
			tokens = append(tokens, "]")
			i++
		} else if data[i] == '%' {
			// Comment: skip to end of line
			for i < len(data) && data[i] != '\n' {
				i++
			}
		} else {
			// Word
			j := i
			for j < len(data) && data[j] != ' ' && data[j] != '\t' && data[j] != '\r' && data[j] != '\n' && data[j] != '<' && data[j] != '[' && data[j] != ']' {
				j++
			}
			tokens = append(tokens, string(data[i:j]))
			i = j
		}
	}
	return tokens
}

func parseBFCharCIDs(tokens []string, start int, result map[uint32]rune) int {
	i := start
	for i+1 < len(tokens) {
		if tokens[i] == "endbfchar" {
			return i + 1
		}
		src, srcOK := parseHexToken(tokens[i])
		dst, dstOK := parseHexToken(tokens[i+1])
		if srcOK && dstOK {
			result[src] = rune(dst)
			i += 2
		} else {
			i++
		}
	}
	return i
}

func parseBFRangeCIDs(tokens []string, start int, result map[uint32]rune) int {
	i := start
	for i+2 < len(tokens) {
		if tokens[i] == "endbfrange" {
			return i + 1
		}
		lo, loOK := parseHexToken(tokens[i])
		hi, hiOK := parseHexToken(tokens[i+1])
		if !loOK || !hiOK {
			i++
			continue
		}
		// Third element: hex string (sequential range) or array
		if tokens[i+2] == "[" {
			// Array form: each element maps lo+j → element[j]
			i += 3
			j := uint32(0)
			for i < len(tokens) && tokens[i] != "]" {
				if v, ok := parseHexToken(tokens[i]); ok && lo+j <= hi {
					result[lo+j] = rune(v)
					j++
				}
				i++
			}
			if i < len(tokens) {
				i++ // skip "]"
			}
		} else if base, baseOK := parseHexToken(tokens[i+2]); baseOK {
			// Sequential range: lo+j → base+j
			for j := uint32(0); lo+j <= hi; j++ {
				result[lo+j] = rune(base + j)
			}
			i += 3
		} else {
			i++
		}
	}
	return i
}

func parseHexToken(s string) (uint32, bool) {
	if len(s) < 2 || s[0] != '<' || s[len(s)-1] != '>' {
		return 0, false
	}
	hex := s[1 : len(s)-1]
	var v uint32
	for _, c := range hex {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint32(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= uint32(c-'A') + 10
		default:
			return 0, false
		}
	}
	return v, true
}

func (e *Evaluator) newEmbeddedType1Font(fontData []byte, fontErr error) entity.Font {
	if fontErr != nil {
		return nil
	}

	// Poppler routes FontFile3 /Subtype /Type1C through its Type1C loader.
	// This gate is intentionally diagnostic for now: the current CFF wrapper
	// lacks full Poppler/FoFi width and encoding parity, so defaulting to it
	// causes broad text-layout regressions even though the route is structural.
	if shouldPreferCFFForType1CDebug() && looksLikeCFFEmbeddedFont(fontData) {
		if cffFont, cffErr := cff.NewFont(fontData); cffErr == nil {
			return cffFont
		}
	}

	if font := e.embeddedType1FontFromBytes(fontData); font != nil {
		return font
	}

	// Type1 parser failed — try CFF (Type1C) format via FreeType.
	// PDF embeds CFF fonts in FontFile3 streams; they use glyph names for encoding.
	cffFont, cffErr := cff.NewFont(fontData)
	if cffErr == nil {
		return cffFont
	}
	return nil
}

func (e *Evaluator) embeddedType1FontFromBytes(fontData []byte) entity.Font {
	key, cacheable := embeddedFontDataCacheKey(fontData)
	if cacheable && e.type1FontCache != nil {
		if cached := e.type1FontCache[key]; cached != nil {
			return cached
		}
	}

	font, err := type1.NewFontFromBytes(fontData)
	if err != nil {
		return nil
	}
	if cacheable {
		e.ensureType1FontCache()[key] = font
	}
	return font
}

func (e *Evaluator) newEmbeddedCIDFontType0Candidate(fontData []byte, fontErr error) entity.Font {
	if fontErr != nil {
		return nil
	}
	if looksLikeCFFEmbeddedFont(fontData) {
		if cffFont, cffErr := cff.NewFont(fontData); cffErr == nil {
			return e.wrapCIDFontType0CWithCIDToGIDMap(cffFont)
		}
	}
	return e.newEmbeddedTrueTypeFont(fontData, fontErr)
}

func looksLikeCFFEmbeddedFont(fontData []byte) bool {
	if len(fontData) < 4 {
		return false
	}
	major := fontData[0]
	headerSize := fontData[2]
	offSize := fontData[3]
	return major == 1 && headerSize >= 4 && offSize >= 1 && offSize <= 4
}

func (e *Evaluator) shouldTrustEmbeddedType1CFont(dict *entity.Dict) bool {
	if dict == nil || nameValueForEncoding(dict.Get(pdfNameSubtype)) != "Type1" {
		return false
	}
	fontData, err := e.getEmbeddedFontData(dict)
	return err == nil && looksLikeCFFEmbeddedFont(fontData)
}

func shouldPreferCFFForType1CDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_TYPE1C_CFF_FIRST")) == "1"
}

func (e *Evaluator) newEmbeddedTrueTypeFont(fontData []byte, fontErr error) entity.Font {
	if fontErr != nil {
		return nil
	}

	if key, cacheable := embeddedFontDataCacheKey(fontData); cacheable {
		if e.trueTypeFontCache != nil {
			if cached := e.trueTypeFontCache[key]; cached != nil {
				return cached
			}
		}
		font, err := truetype.NewFontFromBytes(fontData)
		if err != nil {
			return nil
		}
		e.ensureTrueTypeFontCache()[key] = font
		return font
	}

	font, err := truetype.NewFontFromBytes(fontData)
	if err != nil {
		return nil
	}
	return font
}

func embeddedFontDataCacheKey(fontData []byte) (embeddedFontCacheKey, bool) {
	if len(fontData) == 0 {
		return embeddedFontCacheKey{}, false
	}
	return embeddedFontCacheKey{sum: sha256.Sum256(fontData), size: len(fontData)}, true
}
