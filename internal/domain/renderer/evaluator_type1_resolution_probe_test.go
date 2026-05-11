package renderer

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

func TestSampleType1ResolvedFontUsageProbe(t *testing.T) {
	testCases := []struct {
		name    string
		pdfPath string
		pageNum int
	}{
		{
			name:    "004_p3",
			pdfPath: "../../../test/testdata/sample-files/004-pdflatex-4-pages/pdflatex-4-pages.pdf",
			pageNum: 3,
		},
		{
			name:    "009_p95",
			pdfPath: "../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum: 95,
		},
		{
			name:    "009_p109",
			pdfPath: "../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum: 109,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := internalpdf.Open(tc.pdfPath)
			require.NoError(t, err)

			page, err := doc.GetPage(tc.pageNum - 1)
			require.NoError(t, err)

			resources, err := page.Resources()
			require.NoError(t, err)
			require.NotNil(t, resources)

			contents, err := page.Contents()
			require.NoError(t, err)

			e := NewEvaluator(doc.XRef())
			e.SetResources(resources)
			require.NoError(t, e.Evaluate(contents))

			usedByFont := collectUsedCodesByFontForProbe(e.GetOperators())
			require.NotEmpty(t, usedByFont)

			foundType1 := 0
			for resourceName, usedCodes := range usedByFont {
				fontObj := e.getResourceEntry(entity.Name("Font"), entity.Name(resourceName))
				fontDict, ok := resolveFontDictForProbe(doc.XRef(), fontObj)
				if !ok || fontDict == nil {
					continue
				}
				if nameValueForProbe(fontDict.Get(entity.Name("Subtype"))) != "Type1" {
					continue
				}

				baseFont := nameValueForProbe(fontDict.Get(entity.Name("BaseFont")))
				encodingMap := e.resolveSimpleFontEncoding(fontDict.Get(entity.Name("Encoding")))
				font, err := e.getFontFromDict(fontDict, baseFont)
				require.NoError(t, err)
				font = e.applyFontEncodingFromDict(fontDict, font)
				font = e.applyFontMetricsFromDict(fontDict, font)
				require.NotNil(t, font)
				foundType1++

				emptyUsed := make([]int, 0)
				unresolvedUsed := make([]int, 0)
				width500Used := make([]int, 0)
				for _, code := range usedCodes {
					glyph, err := font.CharCodeToGlyph(uint32(code))
					if err != nil {
						unresolvedUsed = append(unresolvedUsed, code)
						continue
					}

					width, err := font.GetGlyphWidth(glyph)
					require.NoError(t, err)
					if width == 500 {
						width500Used = append(width500Used, code)
					}

					path, err := font.RenderGlyph(glyph, 1000)
					if err != nil {
						unresolvedUsed = append(unresolvedUsed, code)
						continue
					}
					if path == nil || len(path.Commands) == 0 {
						emptyUsed = append(emptyUsed, code)
					}
				}

				t.Logf(
					"%s/%s base=%s resolved_font=%s used=%v unresolved_used=%v unresolved_names=%v alias_status=%v empty_used=%v width500_used=%v width500_sources=%v",
					tc.name,
					resourceName,
					baseFont,
					describeFontForProbe(font),
					usedCodes,
					unresolvedUsed,
					namesForCodesForProbe(encodingMap, unresolvedUsed),
					aliasStatusForProbe(font, encodingMap, unresolvedUsed),
					emptyUsed,
					width500Used,
					widthSourceStatusForProbe(font, width500Used),
				)
			}

			require.Greater(t, foundType1, 0)
		})
	}
}

func collectUsedCodesByFontForProbe(ops []Operator) map[string][]int {
	used := make(map[string]map[int]struct{})
	currentFont := ""

	for _, op := range ops {
		switch op.Opcode {
		case "Tf":
			if len(op.Operands) == 0 {
				continue
			}
			if name, ok := op.Operands[0].(entity.Name); ok {
				currentFont = normalizeResourceNameForProbe(name.Value())
			}

		case "Tj", "'", "\"":
			if currentFont == "" || len(op.Operands) == 0 {
				continue
			}
			if str, ok := op.Operands[len(op.Operands)-1].(*entity.String); ok {
				addStringCodesForProbe(used, currentFont, str.Value())
			}

		case "TJ":
			if currentFont == "" || len(op.Operands) == 0 {
				continue
			}
			arr, ok := op.Operands[0].(*entity.Array)
			if !ok {
				continue
			}
			for i := 0; i < arr.Len(); i++ {
				if str, ok := arr.Get(i).(*entity.String); ok {
					addStringCodesForProbe(used, currentFont, str.Value())
				}
			}
		}
	}

	out := make(map[string][]int, len(used))
	for resourceName, codeSet := range used {
		codes := make([]int, 0, len(codeSet))
		for code := range codeSet {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		out[resourceName] = codes
	}
	return out
}

func addStringCodesForProbe(used map[string]map[int]struct{}, resourceName string, value string) {
	set := used[resourceName]
	if set == nil {
		set = make(map[int]struct{})
		used[resourceName] = set
	}
	for _, b := range []byte(value) {
		set[int(b)] = struct{}{}
	}
}

func normalizeResourceNameForProbe(name string) string {
	return strings.TrimPrefix(name, "/")
}

func resolveFontDictForProbe(xref entity.XRef, obj entity.Object) (*entity.Dict, bool) {
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

func nameValueForProbe(obj entity.Object) string {
	if name, ok := obj.(entity.Name); ok {
		return name.Value()
	}
	return ""
}

func namesForCodesForProbe(encodingMap map[byte]string, codes []int) []string {
	if len(codes) == 0 {
		return nil
	}
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		name := encodingMap[byte(code)]
		if name == "" {
			name = "<none>"
		}
		out = append(out, fmt.Sprintf("%d:%s", code, name))
	}
	return out
}

func describeFontForProbe(font entity.Font) string {
	if font == nil {
		return "<nil>"
	}
	switch typed := font.(type) {
	case *widthMappedFont:
		return fmt.Sprintf("%T(%s)->%s", font, font.Name(), describeFontForProbe(typed.base))
	case *encodedFont:
		return fmt.Sprintf("%T(%s)->%s", font, font.Name(), describeFontForProbe(typed.base))
	case *glyphSourceOverrideFont:
		return fmt.Sprintf("%T(%s)->%s", font, font.Name(), describeFontForProbe(typed.base))
	default:
		return fmt.Sprintf("%T(%s)", font, font.Name())
	}
}

func widthSourceStatusForProbe(font entity.Font, codes []int) []string {
	if len(codes) == 0 {
		return nil
	}
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		out = append(out, fmt.Sprintf("%d:%s", code, widthSourceForProbe(font, uint32(code))))
	}
	return out
}

func widthSourceForProbe(font entity.Font, code uint32) string {
	switch typed := font.(type) {
	case *widthMappedFont:
		glyph, err := typed.CharCodeToGlyph(code)
		if err != nil {
			return "glyph_error"
		}
		if _, ok := typed.widths[glyph]; ok {
			return "mapped"
		}
		if typed.defaultWidth > 0 {
			if width, err := typed.base.GetGlyphWidth(glyph); err == nil {
				if width == 500 {
					return "base500"
				}
				return "base"
			}
			return "default"
		}
		return "base_only"
	case *encodedFont:
		return "encoded->" + widthSourceForProbe(typed.base, code)
	default:
		return fmt.Sprintf("%T", font)
	}
}

func aliasStatusForProbe(font entity.Font, encodingMap map[byte]string, codes []int) []string {
	if len(codes) == 0 {
		return nil
	}

	out := make([]string, 0, len(codes))
	for _, code := range codes {
		name := encodingMap[byte(code)]
		candidates := encodingGlyphNameCandidates(name)
		status := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			mapped := false
			if namedFont, ok := font.(glyphIDByNameFont); ok {
				if _, found := namedFont.GlyphIDByName(candidate); found {
					mapped = true
				}
			}
			status = append(status, fmt.Sprintf("%s=%t", candidate, mapped))
		}
		if len(status) == 0 {
			status = append(status, "<none>")
		}
		out = append(out, fmt.Sprintf("%d:[%s]", code, strings.Join(status, ",")))
	}
	return out
}
