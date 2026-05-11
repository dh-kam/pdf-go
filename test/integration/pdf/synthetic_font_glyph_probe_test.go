package pdf_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

type syntheticFontGlyphProbeCase struct {
	name       string
	pdfPath    string
	pageNumber int
	selector   syntheticFontGlyphProbeSelector
	maxCodes   int
	dpi        int
}

type syntheticFontGlyphProbeSelector struct {
	resourceName     string
	subtype          string
	baseFontContains string
}

type syntheticFontGlyphProbeVariant struct {
	name string
}

type syntheticFontCodeCount struct {
	code  uint32
	count int
}

type syntheticFontGlyphProbeSource struct {
	name         string
	resourceName string
	baseFont     string
	subtype      string
	isCID        bool
	codeCounts   []syntheticFontCodeCount
	rootObject   entity.Object
	doc          *entity.Document
}

type syntheticFontImportedObject struct {
	number int
	object entity.Object
}

type syntheticFontGraphImporter struct {
	t          *testing.T
	xref       entity.XRef
	nextNumber int
	refNumbers map[entity.Ref]int
	imported   []syntheticFontImportedObject
}

type syntheticFontDiffHotspot struct {
	index int
	count int
}

type syntheticFontGlyphProbeResult struct {
	source      syntheticFontGlyphProbeSource
	variant     string
	exact       float64
	similarity  float64
	placement   samplePagePlacementProbeResult
	rowHotspots []syntheticFontDiffHotspot
	colHotspots []syntheticFontDiffHotspot
}

func TestSyntheticFontGlyphCompositionProbeAgainstPoppler(t *testing.T) {
	for _, tc := range syntheticFontGlyphProbeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			source := loadSyntheticFontGlyphProbeSource(t, tc)
			defer func() {
				require.NoError(t, source.doc.Close())
			}()

			t.Logf(
				"selected resource=%s base=%s subtype=%s cid=%t top_codes=%s",
				source.resourceName,
				emptyDashForSyntheticFontProbe(source.baseFont),
				emptyDashForSyntheticFontProbe(source.subtype),
				source.isCID,
				formatSyntheticFontCodeCountsForProbe(source.codeCounts),
			)

			for _, variant := range syntheticFontGlyphProbeVariants() {
				variant := variant
				t.Run(variant.name, func(t *testing.T) {
					result := measureSyntheticFontGlyphProbeAgainstPoppler(t, source, variant, tc.dpi)

					t.Logf(
						"%s exact=%.4f similarity=%.4f best_shift_similarity=%.4f best_shift=(%d,%d) diff_bounds=%v rows=%s cols=%s",
						result.variant,
						result.exact,
						result.similarity,
						result.placement.bestShiftSimilarity,
						result.placement.bestShiftX,
						result.placement.bestShiftY,
						result.placement.diffBounds,
						formatSyntheticFontDiffHotspotsForProbe(result.rowHotspots),
						formatSyntheticFontDiffHotspotsForProbe(result.colHotspots),
					)

					require.NotEmpty(t, result.source.codeCounts)
					require.Greater(t, result.placement.originalSimilarity, 0.0)
				})
			}
		})
	}
}

func syntheticFontGlyphProbeCases() []syntheticFontGlyphProbeCase {
	sampleDir := getSampleDir()

	return []syntheticFontGlyphProbeCase{
		{
			name:       "type1_sfrm1095_page95",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 95,
			selector: syntheticFontGlyphProbeSelector{
				subtype:          "Type1",
				baseFontContains: "SFRM1095",
			},
			maxCodes: 12,
			dpi:      150,
		},
		{
			name:       "type1_sfbx1095_page95",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 95,
			selector: syntheticFontGlyphProbeSelector{
				subtype:          "Type1",
				baseFontContains: "SFBX1095",
			},
			maxCodes: 12,
			dpi:      150,
		},
		{
			name:       "type1_sfsx1440_page95",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 95,
			selector: syntheticFontGlyphProbeSelector{
				subtype:          "Type1",
				baseFontContains: "SFSX1440",
			},
			maxCodes: 12,
			dpi:      150,
		},
		{
			name:       "type1_cmr10_page95",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 95,
			selector: syntheticFontGlyphProbeSelector{
				subtype:          "Type1",
				baseFontContains: "CMR10",
			},
			maxCodes: 12,
			dpi:      150,
		},
		{
			name:       "type1_sfrm1095_page109",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 109,
			selector: syntheticFontGlyphProbeSelector{
				resourceName:     "F16",
				subtype:          "Type1",
				baseFontContains: "SFRM1095",
			},
			maxCodes: 12,
			dpi:      150,
		},
		{
			name:       "type0_used_font_page1",
			pdfPath:    filepath.Join(sampleDir, "015-arabic", "habibi-oneline-cmap.pdf"),
			pageNumber: 1,
			selector: syntheticFontGlyphProbeSelector{
				subtype: "Type0",
			},
			maxCodes: 12,
			dpi:      150,
		},
	}
}

func syntheticFontGlyphProbeVariants() []syntheticFontGlyphProbeVariant {
	return []syntheticFontGlyphProbeVariant{
		{name: "plain_tj"},
		{name: "kerned_tj"},
	}
}

func measureSyntheticFontGlyphProbeAgainstPoppler(
	t *testing.T,
	source syntheticFontGlyphProbeSource,
	variant syntheticFontGlyphProbeVariant,
	dpi int,
) syntheticFontGlyphProbeResult {
	t.Helper()

	pdfBytes := buildSyntheticFontGlyphProbePDF(t, source, variant)
	pdfName := fmt.Sprintf("%s_%s.pdf", source.name, variant.name)
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", dpi)
	placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 4)
	rowHotspots, colHotspots := collectSyntheticFontDiffHotspotsForProbe(t, popplerPNG, oursPNG, 5)

	return syntheticFontGlyphProbeResult{
		source:      source,
		variant:     variant.name,
		exact:       placement.originalExact,
		similarity:  placement.originalSimilarity,
		placement:   placement,
		rowHotspots: rowHotspots,
		colHotspots: colHotspots,
	}
}

func buildSyntheticFontGlyphProbePDF(
	t *testing.T,
	source syntheticFontGlyphProbeSource,
	variant syntheticFontGlyphProbeVariant,
) []byte {
	t.Helper()

	importer := newSyntheticFontGraphImporter(t, source.doc.XRef(), 5)
	fontObjectNumber := importer.importRoot(source.rootObject)
	content := buildSyntheticFontGlyphProbeContent(source, variant)

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 900 240] /Resources << /Font << /F1 %d 0 R >> >> /Contents 4 0 R >>",
			fontObjectNumber,
		)),
		syntheticStreamObject(fmt.Sprintf("<< /Length %d >>", len(content)), content),
	}

	for _, imported := range importer.objects() {
		objects = append(objects, serializeSyntheticFontEntityObject(imported.object))
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticFontGlyphProbeContent(
	source syntheticFontGlyphProbeSource,
	variant syntheticFontGlyphProbeVariant,
) []byte {
	codes := syntheticFontGlyphSequenceForProbe(source.codeCounts)
	primary := encodeSyntheticFontCodesForProbe(codes, source.isCID)
	mirror := encodeSyntheticFontCodesForProbe(reverseSyntheticFontCodesForProbe(codes), source.isCID)

	var buf bytes.Buffer
	buf.WriteString("BT\n/F1 44 Tf\n1 0 0 1 36 150 Tm\n")
	switch variant.name {
	case "kerned_tj":
		buf.WriteString(buildSyntheticFontKerningArrayForProbe(codes, source.isCID))
	default:
		fmt.Fprintf(&buf, "<%s> Tj\n", primary)
	}
	buf.WriteString("0 -56 Td\n")
	fmt.Fprintf(&buf, "<%s> Tj\n", mirror)
	buf.WriteString("0 -56 Td\n")
	fmt.Fprintf(&buf, "<%s> Tj\nET\n", primary)
	return buf.Bytes()
}

func buildSyntheticFontKerningArrayForProbe(codes []uint32, isCID bool) string {
	if len(codes) == 0 {
		return "[] TJ\n"
	}

	var buf strings.Builder
	buf.WriteString("[")
	for idx, code := range codes {
		if idx > 0 {
			buf.WriteByte(' ')
			switch idx % 3 {
			case 0:
				buf.WriteString("-40")
			case 1:
				buf.WriteString("30")
			default:
				buf.WriteString("0")
			}
			buf.WriteByte(' ')
		}
		buf.WriteString("<")
		buf.WriteString(encodeSyntheticFontCodesForProbe([]uint32{code}, isCID))
		buf.WriteString(">")
	}
	buf.WriteString("] TJ\n")
	return buf.String()
}

func loadSyntheticFontGlyphProbeSource(
	t *testing.T,
	tc syntheticFontGlyphProbeCase,
) syntheticFontGlyphProbeSource {
	t.Helper()

	doc, err := internalpdf.Open(tc.pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(tc.pageNumber - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	fonts, ok := resolveSyntheticFontDictForProbe(doc.XRef(), resources.Get(entity.Name("Font")))
	require.True(t, ok)
	require.NotNil(t, fonts)

	fontResources := loadSyntheticFontPageResourcesForProbe(t, doc.XRef(), fonts)

	contents, err := page.Contents()
	require.NoError(t, err)

	evaluator := domainrenderer.NewEvaluator(doc.XRef())
	evaluator.SetResources(resources)
	require.NoError(t, evaluator.Evaluate(contents))

	countsByFont := collectSyntheticFontCodeFrequencyByResource(t, evaluator.GetOperators(), fontResources)
	selected := selectSyntheticFontProbeResource(t, fontResources, countsByFont, tc.selector)
	selected.doc = doc
	selected.name = tc.name
	selected.codeCounts = trimSyntheticFontCodeCountsForProbe(selected.codeCounts, tc.maxCodes)
	require.NotEmpty(t, selected.codeCounts)

	return selected
}

func loadSyntheticFontPageResourcesForProbe(
	t *testing.T,
	xref entity.XRef,
	fonts *entity.Dict,
) map[string]syntheticFontGlyphProbeSource {
	t.Helper()

	out := make(map[string]syntheticFontGlyphProbeSource, fonts.Len())
	for _, key := range sortedSyntheticFontNamesForProbe(fonts.Keys()) {
		resourceName := normalizeSyntheticFontResourceNameForProbe(key.Value())
		rawObject := fonts.GetRaw(key)
		fontDict, ok := resolveSyntheticFontDictForProbe(xref, rawObject)
		if !ok || fontDict == nil {
			continue
		}

		subtype := syntheticNameValueForProbe(fontDict.Get(entity.Name("Subtype")))
		baseFont := syntheticNameValueForProbe(fontDict.Get(entity.Name("BaseFont")))
		out[resourceName] = syntheticFontGlyphProbeSource{
			resourceName: resourceName,
			baseFont:     baseFont,
			subtype:      subtype,
			isCID:        syntheticFontSubtypeIsCIDForProbe(subtype),
			rootObject:   rawObject,
		}
	}

	return out
}

func collectSyntheticFontCodeFrequencyByResource(
	t *testing.T,
	operators []domainrenderer.Operator,
	resources map[string]syntheticFontGlyphProbeSource,
) map[string][]syntheticFontCodeCount {
	t.Helper()

	counts := make(map[string]map[uint32]int, len(resources))
	currentFont := ""

	addString := func(resourceName string, str *entity.String) {
		if str == nil {
			return
		}
		resource, ok := resources[resourceName]
		if !ok {
			return
		}
		codeCounts := counts[resourceName]
		if codeCounts == nil {
			codeCounts = make(map[uint32]int)
			counts[resourceName] = codeCounts
		}
		for _, code := range splitSyntheticFontStringCodesForProbe(str.Value(), resource.isCID) {
			codeCounts[code]++
		}
	}

	for _, op := range operators {
		switch op.Opcode {
		case "Tf":
			if len(op.Operands) == 0 {
				continue
			}
			name, ok := op.Operands[0].(entity.Name)
			if !ok {
				continue
			}
			currentFont = normalizeSyntheticFontResourceNameForProbe(name.Value())
		case "Tj", "'", "\"":
			if currentFont == "" || len(op.Operands) == 0 {
				continue
			}
			str, ok := op.Operands[len(op.Operands)-1].(*entity.String)
			if ok {
				addString(currentFont, str)
			}
		case "TJ":
			if currentFont == "" || len(op.Operands) == 0 {
				continue
			}
			arr, ok := op.Operands[0].(*entity.Array)
			if !ok {
				continue
			}
			for _, item := range arr.Items() {
				str, ok := item.(*entity.String)
				if ok {
					addString(currentFont, str)
				}
			}
		}
	}

	out := make(map[string][]syntheticFontCodeCount, len(counts))
	for resourceName, resourceCounts := range counts {
		pairs := make([]syntheticFontCodeCount, 0, len(resourceCounts))
		for code, count := range resourceCounts {
			pairs = append(pairs, syntheticFontCodeCount{code: code, count: count})
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].count == pairs[j].count {
				return pairs[i].code < pairs[j].code
			}
			return pairs[i].count > pairs[j].count
		})
		out[resourceName] = pairs
	}

	return out
}

func selectSyntheticFontProbeResource(
	t *testing.T,
	resources map[string]syntheticFontGlyphProbeSource,
	countsByFont map[string][]syntheticFontCodeCount,
	selector syntheticFontGlyphProbeSelector,
) syntheticFontGlyphProbeSource {
	t.Helper()

	matches := make([]syntheticFontGlyphProbeSource, 0, len(resources))
	for resourceName, resource := range resources {
		if selector.resourceName != "" && resourceName != selector.resourceName {
			continue
		}
		if selector.subtype != "" && resource.subtype != selector.subtype {
			continue
		}
		if selector.baseFontContains != "" && !strings.Contains(resource.baseFont, selector.baseFontContains) {
			continue
		}

		codeCounts := countsByFont[resourceName]
		if len(codeCounts) == 0 {
			continue
		}

		resource.codeCounts = append([]syntheticFontCodeCount(nil), codeCounts...)
		matches = append(matches, resource)
	}

	require.NotEmpty(t, matches)
	sort.Slice(matches, func(i, j int) bool {
		leftTotal := totalSyntheticFontCodeCountForProbe(matches[i].codeCounts)
		rightTotal := totalSyntheticFontCodeCountForProbe(matches[j].codeCounts)
		if leftTotal == rightTotal {
			if len(matches[i].codeCounts) == len(matches[j].codeCounts) {
				return matches[i].resourceName < matches[j].resourceName
			}
			return len(matches[i].codeCounts) > len(matches[j].codeCounts)
		}
		return leftTotal > rightTotal
	})

	return matches[0]
}

func newSyntheticFontGraphImporter(
	t *testing.T,
	xref entity.XRef,
	startNumber int,
) *syntheticFontGraphImporter {
	t.Helper()

	return &syntheticFontGraphImporter{
		t:          t,
		xref:       xref,
		nextNumber: startNumber,
		refNumbers: make(map[entity.Ref]int),
	}
}

func (i *syntheticFontGraphImporter) importRoot(obj entity.Object) int {
	i.t.Helper()

	if ref, ok := obj.(entity.Ref); ok {
		return i.importRef(ref)
	}

	number := i.allocate()
	i.imported = append(i.imported, syntheticFontImportedObject{
		number: number,
		object: i.rewriteObject(obj),
	})
	return number
}

func (i *syntheticFontGraphImporter) importRef(ref entity.Ref) int {
	if number, ok := i.refNumbers[ref]; ok {
		return number
	}

	rawObject, err := i.xref.Fetch(ref)
	require.NoError(i.t, err)

	number := i.allocate()
	i.refNumbers[ref] = number
	i.imported = append(i.imported, syntheticFontImportedObject{
		number: number,
		object: i.rewriteObject(rawObject),
	})
	return number
}

func (i *syntheticFontGraphImporter) rewriteObject(obj entity.Object) entity.Object {
	if obj == nil {
		return entity.NewNull()
	}

	switch typed := obj.(type) {
	case entity.Ref:
		return entity.NewRef(uint32(i.importRef(typed)), 0)
	case *entity.Dict:
		out := entity.NewDict()
		for _, key := range sortedSyntheticFontNamesForProbe(typed.Keys()) {
			out.Set(key, i.rewriteObject(typed.GetRaw(key)))
		}
		return out
	case *entity.Array:
		items := make([]entity.Object, 0, typed.Len())
		for _, item := range typed.Items() {
			items = append(items, i.rewriteObject(item))
		}
		return entity.NewArray(items...)
	case *entity.Stream:
		dictObj := i.rewriteObject(typed.Dict())
		dict, ok := dictObj.(*entity.Dict)
		require.True(i.t, ok)
		dict.Set(entity.Name("Length"), entity.NewInteger(int64(len(typed.RawBytes()))))
		data := append([]byte(nil), typed.RawBytes()...)
		return entity.NewStream(dict, data)
	default:
		return obj.Clone()
	}
}

func (i *syntheticFontGraphImporter) objects() []syntheticFontImportedObject {
	out := append([]syntheticFontImportedObject(nil), i.imported...)
	sort.Slice(out, func(left, right int) bool {
		return out[left].number < out[right].number
	})
	return out
}

func (i *syntheticFontGraphImporter) allocate() int {
	number := i.nextNumber
	i.nextNumber++
	return number
}

func serializeSyntheticFontEntityObject(obj entity.Object) []byte {
	switch typed := obj.(type) {
	case *entity.Stream:
		var buf bytes.Buffer
		buf.WriteString(serializeSyntheticFontPrimitiveObject(typed.Dict()))
		buf.WriteString("\nstream\n")
		buf.Write(typed.RawBytes())
		buf.WriteString("\nendstream")
		return buf.Bytes()
	default:
		return []byte(serializeSyntheticFontPrimitiveObject(obj))
	}
}

func serializeSyntheticFontPrimitiveObject(obj entity.Object) string {
	if obj == nil {
		return "null"
	}

	switch typed := obj.(type) {
	case *entity.Boolean, *entity.Integer, *entity.Real, *entity.Null:
		return typed.String()
	case entity.Name:
		return serializeSyntheticFontName(typed)
	case *entity.String:
		return "<" + strings.ToUpper(hex.EncodeToString([]byte(typed.Value()))) + ">"
	case entity.Ref:
		return fmt.Sprintf("%d %d R", typed.Num(), typed.Gen())
	case *entity.Array:
		var buf strings.Builder
		buf.WriteString("[")
		for index, item := range typed.Items() {
			if index > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(serializeSyntheticFontPrimitiveObject(item))
		}
		buf.WriteString("]")
		return buf.String()
	case *entity.Dict:
		var buf strings.Builder
		buf.WriteString("<<")
		keys := sortedSyntheticFontNamesForProbe(typed.Keys())
		for index, key := range keys {
			if index > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(serializeSyntheticFontName(key))
			buf.WriteByte(' ')
			buf.WriteString(serializeSyntheticFontPrimitiveObject(typed.GetRaw(key)))
		}
		buf.WriteString(">>")
		return buf.String()
	default:
		panic(fmt.Sprintf("unsupported synthetic font probe object type %T", obj))
	}
}

func collectSyntheticFontDiffHotspotsForProbe(
	t *testing.T,
	popplerPNG string,
	oursPNG string,
	limit int,
) ([]syntheticFontDiffHotspot, []syntheticFontDiffHotspot) {
	t.Helper()

	popplerImg, err := parityLoadPNG(popplerPNG)
	require.NoError(t, err)
	oursImg, err := parityLoadPNG(oursPNG)
	require.NoError(t, err)
	require.True(t, popplerImg.Bounds().Eq(oursImg.Bounds()))

	rowCounts := make(map[int]int)
	colCounts := make(map[int]int)
	bounds := popplerImg.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pr, pg, pb, _ := popplerImg.At(x, y).RGBA()
			or, og, ob, _ := oursImg.At(x, y).RGBA()
			if pr == or && pg == og && pb == ob {
				continue
			}
			rowCounts[y]++
			colCounts[x]++
		}
	}

	return topSyntheticFontDiffHotspotsForProbe(rowCounts, limit), topSyntheticFontDiffHotspotsForProbe(colCounts, limit)
}

func topSyntheticFontDiffHotspotsForProbe(counts map[int]int, limit int) []syntheticFontDiffHotspot {
	if len(counts) == 0 {
		return nil
	}

	out := make([]syntheticFontDiffHotspot, 0, len(counts))
	for index, count := range counts {
		out = append(out, syntheticFontDiffHotspot{index: index, count: count})
	}

	sort.Slice(out, func(left, right int) bool {
		if out[left].count == out[right].count {
			return out[left].index < out[right].index
		}
		return out[left].count > out[right].count
	})

	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func splitSyntheticFontStringCodesForProbe(value string, isCID bool) []uint32 {
	raw := []byte(value)
	if !isCID {
		out := make([]uint32, 0, len(raw))
		for _, b := range raw {
			out = append(out, uint32(b))
		}
		return out
	}

	out := make([]uint32, 0, (len(raw)+1)/2)
	for idx := 0; idx < len(raw); {
		if idx+1 < len(raw) {
			out = append(out, uint32(raw[idx])<<8|uint32(raw[idx+1]))
			idx += 2
			continue
		}
		out = append(out, uint32(raw[idx]))
		idx++
	}
	return out
}

func syntheticFontGlyphSequenceForProbe(codeCounts []syntheticFontCodeCount) []uint32 {
	out := make([]uint32, 0, len(codeCounts))
	for _, entry := range codeCounts {
		out = append(out, entry.code)
	}
	return out
}

func reverseSyntheticFontCodesForProbe(codes []uint32) []uint32 {
	out := append([]uint32(nil), codes...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func encodeSyntheticFontCodesForProbe(codes []uint32, isCID bool) string {
	if len(codes) == 0 {
		return ""
	}

	var buf strings.Builder
	for _, code := range codes {
		if isCID {
			if code <= 0xff {
				fmt.Fprintf(&buf, "%02X", code)
				continue
			}
			fmt.Fprintf(&buf, "%04X", code)
			continue
		}
		fmt.Fprintf(&buf, "%02X", byte(code))
	}
	return buf.String()
}

func formatSyntheticFontCodeCountsForProbe(codeCounts []syntheticFontCodeCount) string {
	if len(codeCounts) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(codeCounts))
	for _, entry := range codeCounts {
		parts = append(parts, fmt.Sprintf("0x%X(x%d)", entry.code, entry.count))
	}
	return strings.Join(parts, ",")
}

func formatSyntheticFontDiffHotspotsForProbe(hotspots []syntheticFontDiffHotspot) string {
	if len(hotspots) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(hotspots))
	for _, hotspot := range hotspots {
		parts = append(parts, fmt.Sprintf("%d:%d", hotspot.index, hotspot.count))
	}
	return strings.Join(parts, ",")
}

func trimSyntheticFontCodeCountsForProbe(
	codeCounts []syntheticFontCodeCount,
	limit int,
) []syntheticFontCodeCount {
	if limit <= 0 || len(codeCounts) <= limit {
		return append([]syntheticFontCodeCount(nil), codeCounts...)
	}
	return append([]syntheticFontCodeCount(nil), codeCounts[:limit]...)
}

func totalSyntheticFontCodeCountForProbe(codeCounts []syntheticFontCodeCount) int {
	total := 0
	for _, entry := range codeCounts {
		total += entry.count
	}
	return total
}

func resolveSyntheticFontDictForProbe(xref entity.XRef, obj entity.Object) (*entity.Dict, bool) {
	resolved := resolveSyntheticFontObjectForProbe(xref, obj)
	dict, ok := resolved.(*entity.Dict)
	return dict, ok
}

func resolveSyntheticFontObjectForProbe(xref entity.XRef, obj entity.Object) entity.Object {
	ref, ok := obj.(entity.Ref)
	if !ok || xref == nil {
		return obj
	}

	resolved, err := xref.Fetch(ref)
	if err != nil {
		return obj
	}
	return resolved
}

func sortedSyntheticFontNamesForProbe(names []entity.Name) []entity.Name {
	out := append([]entity.Name(nil), names...)
	sort.Slice(out, func(left, right int) bool {
		return strings.TrimPrefix(out[left].Value(), "/") < strings.TrimPrefix(out[right].Value(), "/")
	})
	return out
}

func syntheticFontSubtypeIsCIDForProbe(subtype string) bool {
	return subtype == "Type0" || strings.HasPrefix(subtype, "CIDFontType")
}

func syntheticNameValueForProbe(obj entity.Object) string {
	name, ok := obj.(entity.Name)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(name.Value(), "/")
}

func normalizeSyntheticFontResourceNameForProbe(name string) string {
	return strings.TrimPrefix(name, "/")
}

func serializeSyntheticFontName(name entity.Name) string {
	return "/" + strings.TrimPrefix(name.Value(), "/")
}

func emptyDashForSyntheticFontProbe(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
