package type1

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

func TestSampleType1EmptyGlyphCommandProbe(t *testing.T) {
	testCases := []struct {
		name         string
		pdfPath      string
		pageNum      int
		fontResource string
		codes        []int
	}{
		{
			name:         "009_p95_F176",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F176",
			codes:        []int{37, 46, 47, 48, 50, 111},
		},
		{
			name:         "009_p95_F78",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F78",
			codes:        []int{45, 46, 71, 83},
		},
		{
			name:         "009_p95_F57",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F57",
			codes:        []int{83},
		},
		{
			name:         "009_p95_F50",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F50",
			codes:        []int{78},
		},
		{
			name:         "009_p95_F49",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F49",
			codes:        []int{252, 255},
		},
		{
			name:         "009_p95_F78_remaining",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F78",
			codes:        []int{220},
		},
		{
			name:         "009_p109_F180",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      109,
			fontResource: "F180",
			codes:        []int{97},
		},
		{
			name:         "009_p109_F47",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      109,
			fontResource: "F47",
			codes:        []int{105},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			font := loadSampleType1FontForProbe(t, tc.pdfPath, tc.pageNum, tc.fontResource)
			require.NotNil(t, font)
			charStrings, _, _, err := font.file.GetType1CharStringData()
			require.NoError(t, err)

			for _, code := range tc.codes {
				glyphID, err := font.CharCodeToGlyph(uint32(code))
				require.NoError(t, err)
				glyph, ok := font.glyphs[glyphID]
				require.True(t, ok)
				require.NotNil(t, glyph)

				path, err := font.RenderGlyph(glyphID, 1000)
				require.NoError(t, err)
				commandTypes := summarizeCommandTypesForProbe(glyph.Commands)
				t.Logf(
					"%s code=%d name=%s has_charstring=%t raw=%d commands=%d cmd_types=%v path_commands=%d width=%.2f",
					tc.fontResource,
					code,
					glyph.Name,
					charStrings[glyph.Name] != nil,
					len(glyph.CharString),
					len(glyph.Commands),
					commandTypes,
					len(path.Commands),
					glyph.Width,
				)
			}
		})
	}
}

func TestSampleType1CharStringTokenProbe(t *testing.T) {
	font := loadSampleType1FontForProbe(
		t,
		"../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
		95,
		"F78",
	)
	require.NotNil(t, font)

	data, err := font.file.GetCharStrings()
	require.NoError(t, err)
	charStrings, _, _, err := font.file.GetType1CharStringData()
	require.NoError(t, err)

	for _, glyphName := range []string{"hyphen", "period", "G", "S"} {
		pos := findToken(data, "/"+glyphName)
		if pos < 0 {
			raw, ok := charStrings[glyphName]
			commandTypes := []string{}
			if ok && len(raw) > 0 {
				decoded, err := NewCharStringDecoderWithSubrs(raw, nil).Decode()
				require.NoError(t, err)
				commandTypes = summarizeCommandTypesForProbe(decoded)
			}
			t.Logf("%s token_missing raw_len=%d exists=%t decoded_cmds=%v", glyphName, len(raw), ok, commandTypes)
			continue
		}

		tok, next, ok := nextToken(data, pos)
		require.True(t, ok)
		lenTok, afterLen, ok := nextToken(data, next)
		require.True(t, ok)
		marker, afterMarker, ok := nextToken(data, afterLen)
		require.True(t, ok)

		start := afterMarker
		end := afterMarker + 16
		if end > len(data) {
			end = len(data)
		}
		t.Logf(
			"%s tok=%q len=%q marker=%q bytes=% x decoded_cmds=%v leniv_probe=%v",
			glyphName,
			tok,
			lenTok,
			marker,
			data[start:end],
			summarizeDecodedCharStringForProbe(charStrings[glyphName]),
			probeLenIVVariantsForProbe(data[afterMarker:afterMarker+mustAtoiForProbe(lenTok)]),
		)
	}
}

func TestSampleType1MissingCharStringNameProbe(t *testing.T) {
	testCases := []struct {
		name         string
		pdfPath      string
		pageNum      int
		fontResource string
		codes        []int
	}{
		{
			name:         "009_p95_F176",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F176",
			codes:        []int{37, 46, 47, 48, 50},
		},
		{
			name:         "009_p95_F57",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F57",
			codes:        []int{83},
		},
		{
			name:         "009_p109_F180",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      109,
			fontResource: "F180",
			codes:        []int{45, 97},
		},
		{
			name:         "009_p109_F47",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      109,
			fontResource: "F47",
			codes:        []int{105},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			font := loadSampleType1FontForProbe(t, tc.pdfPath, tc.pageNum, tc.fontResource)
			require.NotNil(t, font)

			charStrings, _, _, err := font.file.GetType1CharStringData()
			require.NoError(t, err)

			for _, code := range tc.codes {
				expectedName := font.EncodingName(byte(code))
				candidates := findSimilarCharStringNamesForProbe(expectedName, charStrings)
				t.Logf(
					"%s code=%d expected=%q has_exact=%t candidates=%v",
					tc.fontResource,
					code,
					expectedName,
					charStrings[expectedName] != nil,
					candidates,
				)
			}
		})
	}
}

func TestSampleType1CharStringCandidateSelectionProbe(t *testing.T) {
	testCases := []struct {
		name         string
		pdfPath      string
		pageNum      int
		fontResource string
	}{
		{
			name:         "009_p95_F176",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F176",
		},
		{
			name:         "009_p95_F57",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F57",
		},
		{
			name:         "009_p109_F180",
			pdfPath:      "../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      109,
			fontResource: "F180",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			font := loadSampleType1FontForProbe(t, tc.pdfPath, tc.pageNum, tc.fontResource)
			require.NotNil(t, font)
			require.NotNil(t, font.file)

			candidates := map[string][]byte{
				"binary": append([]byte(nil), font.file.Binary...),
			}
			if len(font.file.Binary) > eexecDiscard {
				decrypted, err := DecryptEexec(font.file.Binary)
				require.NoError(t, err)
				candidates["decrypted"] = decrypted
			}

			for label, data := range candidates {
				lenIV := parseType1LenIV(data)
				charStrings, err := parseType1CharStrings(data, lenIV)
				if err != nil {
					t.Logf("%s %s parse_error=%v", tc.fontResource, label, err)
					continue
				}
				keys := sortedCharStringKeysForProbe(charStrings, 12)
				t.Logf(
					"%s %s lenIV=%d charstrings=%d sample_keys=%v",
					tc.fontResource,
					label,
					lenIV,
					len(charStrings),
					keys,
				)
			}
		})
	}
}

func summarizeDecodedCharStringForProbe(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	decoded, err := NewCharStringDecoderWithSubrs(raw, nil).Decode()
	if err != nil {
		return []string{"decode_error"}
	}
	return summarizeCommandTypesForProbe(decoded)
}

func findSimilarCharStringNamesForProbe(expected string, charStrings map[string][]byte) []string {
	if expected == "" {
		return nil
	}
	expectedNorm := normalizeCharStringNameForProbe(expected)
	matches := make([]string, 0, 8)
	for name := range charStrings {
		nameNorm := normalizeCharStringNameForProbe(name)
		if nameNorm == expectedNorm ||
			strings.Contains(nameNorm, expectedNorm) ||
			strings.Contains(expectedNorm, nameNorm) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	if len(matches) > 8 {
		matches = matches[:8]
	}
	return matches
}

func normalizeCharStringNameForProbe(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func sortedCharStringKeysForProbe(charStrings map[string][]byte, limit int) []string {
	keys := make([]string, 0, len(charStrings))
	for key := range charStrings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	return keys
}

func probeLenIVVariantsForProbe(payload []byte) []string {
	type candidate struct {
		label string
		lenIV int
	}
	candidates := []candidate{
		{label: "lenIV=4", lenIV: 4},
		{label: "lenIV=0", lenIV: 0},
		{label: "lenIV=-1", lenIV: -1},
	}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		decoded, err := DecryptCharStringWithLenIV(payload, candidate.lenIV)
		if err != nil {
			out = append(out, candidate.label+":decrypt_error")
			continue
		}
		commands, err := NewCharStringDecoderWithSubrs(decoded, nil).Decode()
		if err != nil {
			out = append(out, candidate.label+":decode_error")
			continue
		}
		out = append(out, candidate.label+":"+joinCommandTypesForProbe(commands))
	}
	return out
}

func joinCommandTypesForProbe(commands []Command) string {
	types := summarizeCommandTypesForProbe(commands)
	if len(types) == 0 {
		return "[]"
	}
	return "[" + strings.Join(types, ",") + "]"
}

func mustAtoiForProbe(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func loadSampleType1FontForProbe(t *testing.T, pdfPath string, pageNum int, fontResource string) *Font {
	t.Helper()

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(pageNum - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	fonts, ok := resolveDictForType1Probe(doc.XRef(), resources.Get(entity.Name("Font")))
	require.True(t, ok)
	require.NotNil(t, fonts)

	fontDict, ok := resolveDictForType1Probe(doc.XRef(), fonts.Get(entity.Name(fontResource)))
	require.True(t, ok)
	require.NotNil(t, fontDict)

	data, ok := extractFontFileBytesForType1Probe(doc.XRef(), fontDict)
	require.True(t, ok)
	require.NotEmpty(t, data)

	font, err := NewFontFromBytes(data)
	require.NoError(t, err)
	return font
}

func summarizeCommandTypesForProbe(commands []Command) []string {
	counts := map[string]int{}
	for _, cmd := range commands {
		counts[cmd.Type.String()]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key)
	}
	return out
}

func extractFontFileBytesForType1Probe(xref entity.XRef, fontDict *entity.Dict) ([]byte, bool) {
	descriptor, ok := resolveDictForType1Probe(xref, fontDict.Get(entity.Name("FontDescriptor")))
	if !ok || descriptor == nil {
		return nil, false
	}

	for _, key := range []entity.Name{"FontFile", "FontFile2", "FontFile3"} {
		obj := descriptor.Get(key)
		if obj == nil {
			continue
		}
		streamObj, ok := resolveStreamForType1Probe(xref, obj)
		if !ok || streamObj == nil {
			continue
		}
		decoded, err := pdfstream.NewFromEntity(streamObj).Decode()
		if err != nil {
			continue
		}
		return decoded, true
	}

	return nil, false
}

func resolveDictForType1Probe(xref entity.XRef, obj entity.Object) (*entity.Dict, bool) {
	switch value := obj.(type) {
	case *entity.Dict:
		return value, true
	case entity.Ref:
		if xref == nil {
			return nil, false
		}
		resolved, err := xref.Fetch(value)
		if err != nil {
			return nil, false
		}
		dict, ok := resolved.(*entity.Dict)
		return dict, ok
	default:
		return nil, false
	}
}

func resolveStreamForType1Probe(xref entity.XRef, obj entity.Object) (*entity.Stream, bool) {
	switch value := obj.(type) {
	case *entity.Stream:
		return value, true
	case entity.Ref:
		if xref == nil {
			return nil, false
		}
		resolved, err := xref.Fetch(value)
		if err != nil {
			return nil, false
		}
		streamObj, ok := resolved.(*entity.Stream)
		return streamObj, ok
	default:
		return nil, false
	}
}
