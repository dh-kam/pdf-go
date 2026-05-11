package renderer

import (
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
	case "TrueType", "CIDFontType0", "CIDFontType2":
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

	// For CIDFontType2 descendants with Identity CIDToGIDMap, wrap in cidIdentityFont
	// so text is processed as 2-byte CIDs and char codes map directly to glyph IDs.
	subtypeName := nameValueForEncoding(descendantDict.Get(entity.Name("Subtype")))
	if subtypeName == "CIDFontType2" {
		cidToGID := descendantDict.Get(entity.Name("CIDToGIDMap"))
		isIdentity := cidToGID == nil
		if cidToGIDName, ok := cidToGID.(entity.Name); ok && cidToGIDName.Value() == "Identity" {
			isIdentity = true
		}
		if isIdentity && font != nil && !font.IsCIDFont() {
			toUnicode := e.parseType0ToUnicodeMap(dict)
			font = &cidIdentityFont{base: font, toUnicode: toUnicode}
		}
	}

	return font
}

func (e *Evaluator) resolveFirstDescendantFontDict(dict *entity.Dict) (*entity.Dict, bool) {
	descendantFonts, ok := dict.Get(entity.Name("DescendantFonts")).(*entity.Array)
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

// parseType0ToUnicodeMap parses the ToUnicode CMap stream from a Type0 font dict,
// returning a CID→Unicode rune mapping used to resolve glyph IDs via the TrueType cmap.
func (e *Evaluator) parseType0ToUnicodeMap(dict *entity.Dict) map[uint32]rune {
	if dict == nil {
		return nil
	}
	tuObj := dict.Get(entity.Name("ToUnicode"))
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

	font, err := type1.NewFontFromBytes(fontData)
	if err == nil {
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
	if dict == nil || nameValueForEncoding(dict.Get(entity.Name("Subtype"))) != "Type1" {
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

	font, err := truetype.NewFontFromBytes(fontData)
	if err != nil {
		return nil
	}
	return font
}
