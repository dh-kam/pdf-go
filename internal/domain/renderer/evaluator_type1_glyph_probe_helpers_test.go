package renderer

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/vector"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
	"github.com/dh-kam/pdf-go/test/testutil"
)

type glyphSetSignature struct {
	commands  int
	nonEmpty  int
	widthSum  string
	boundsSum string
}

type glyphPathComplexitySignature struct {
	moves      int
	lines      int
	curves     int
	closes     int
	boundsArea float64
}

func (s glyphPathComplexitySignature) totalSegments() int {
	return s.lines + s.curves + s.closes
}

func (s glyphPathComplexitySignature) areaPerSegment() float64 {
	totalSegments := s.totalSegments()
	if totalSegments == 0 {
		return 0
	}
	return s.boundsArea / float64(totalSegments)
}

func (s glyphPathComplexitySignature) segmentDensity() float64 {
	if s.boundsArea == 0 {
		return 0
	}
	return float64(s.totalSegments()) / s.boundsArea
}

func (s glyphPathComplexitySignature) curveShare() float64 {
	totalSegments := s.totalSegments()
	if totalSegments == 0 {
		return 0
	}
	return float64(s.curves) / float64(totalSegments)
}

func (s glyphPathComplexitySignature) lineShare() float64 {
	totalSegments := s.totalSegments()
	if totalSegments == 0 {
		return 0
	}
	return float64(s.lines) / float64(totalSegments)
}

func (s glyphPathComplexitySignature) closeShare() float64 {
	totalSegments := s.totalSegments()
	if totalSegments == 0 {
		return 0
	}
	return float64(s.closes) / float64(totalSegments)
}

type glyphRasterSignature struct {
	nonZeroPixels int
	alphaSum      uint64
}

type glyphStripRasterSignature struct {
	nonZeroPixels int
	alphaSum      uint64
	width         int
	height        int
}

type alphaDiffSignature struct {
	nonZeroPixels int
	alphaAbsDiff  uint64
}

type type1GlyphProbeFixture struct {
	fontDict *entity.Dict
	baseFont string
}

type type1GlyphProbeMode struct {
	name  string
	key   string
	value string
}

type resolvedType1ProbeFont struct {
	doc  *entity.Document
	font entity.Font
}

type syntheticExpandedGlyphSetProbeCase struct {
	name           string
	pdfPath        string
	pageNum        int
	fontResource   string
	codes          []int
	topCodes       []int
	secondaryCodes []int
	longTailCodes  []int
	nonLowerCodes  []int
}

type syntheticExpandedGlyphSetResolvedProbe struct {
	name  string
	codes []int
	doc   *entity.Document
	font  entity.Font
}

type syntheticExpandedGlyphSetOutlineProbeResult struct {
	name       string
	complexity glyphPathComplexitySignature
	lowRatio   float64
}

type syntheticExpandedGlyphSetOutlineOrderingProbeResult struct {
	page95Name  string
	page95      syntheticExpandedGlyphSetOutlineProbeResult
	page109Name string
	page109     syntheticExpandedGlyphSetOutlineProbeResult
}

type syntheticExpandedGlyphSetOutlineProbePair struct {
	page95  syntheticExpandedGlyphSetOutlineProbeResult
	page109 syntheticExpandedGlyphSetOutlineProbeResult
}

func unwrapStandardFallbackChainForProbe(t *testing.T, font entity.Font) (*widthMappedFont, *encodedFont, entity.Font) {
	t.Helper()

	for depth := 0; depth < 16; depth++ {
		if widthMapped, ok := font.(*widthMappedFont); ok {
			require.NotNil(t, widthMapped.base)

			encoded, ok := widthMapped.base.(*encodedFont)
			require.True(t, ok)
			require.NotNil(t, encoded.base)

			return widthMapped, encoded, encoded.base
		}
		unwrapper, ok := font.(fontBaseUnwrapper)
		require.Truef(t, ok, "font chain does not contain widthMappedFont: %s", describeFontForProbe(font))
		font = unwrapper.BaseFont()
	}

	t.Fatalf("font chain exceeded unwrap depth: %s", describeFontForProbe(font))
	return nil, nil, nil
}

func collectGlyphSetSignature(t *testing.T, font entity.Font, codes []int) glyphSetSignature {
	t.Helper()

	signature := glyphSetSignature{}
	var widthSum, leftSum, topSum, rightSum, bottomSum float64

	for _, code := range codes {
		glyph, err := font.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		width, err := font.GetGlyphWidth(glyph)
		require.NoError(t, err)
		widthSum += width

		path, err := font.RenderGlyph(glyph, 1000)
		require.NoError(t, err)
		require.NotNil(t, path)

		signature.commands += len(path.Commands)
		if len(path.Commands) > 0 {
			signature.nonEmpty++
		}
		leftSum += path.Bounds[0]
		topSum += path.Bounds[1]
		rightSum += path.Bounds[2]
		bottomSum += path.Bounds[3]
	}

	signature.widthSum = fmt.Sprintf("%.2f", widthSum)
	signature.boundsSum = fmt.Sprintf("%.2f,%.2f,%.2f,%.2f", leftSum, topSum, rightSum, bottomSum)
	return signature
}

func collectGlyphPathComplexitySignature(t *testing.T, font entity.Font, codes []int) glyphPathComplexitySignature {
	t.Helper()

	signature := glyphPathComplexitySignature{}
	for _, code := range codes {
		glyph, err := font.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		path, err := font.RenderGlyph(glyph, 1000)
		require.NoError(t, err)
		require.NotNil(t, path)

		boundsWidth := maxFloatForProbe(0, path.Bounds[2]-path.Bounds[0])
		boundsHeight := maxFloatForProbe(0, path.Bounds[3]-path.Bounds[1])
		signature.boundsArea += boundsWidth * boundsHeight

		for _, cmd := range path.Commands {
			switch cmd.(type) {
			case *entity.PathMoveTo:
				signature.moves++
			case *entity.PathLineTo:
				signature.lines++
			case *entity.PathCurveTo:
				signature.curves++
			case *entity.PathClose:
				signature.closes++
			default:
				t.Fatalf("unsupported glyph path command type %T", cmd)
			}
		}
	}

	return signature
}

func collectGlyphSetRasterSignature(t *testing.T, font entity.Font, codes []int) glyphRasterSignature {
	t.Helper()

	signature := glyphRasterSignature{}
	for _, code := range codes {
		glyph, err := font.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		path, err := font.RenderGlyph(glyph, 1000)
		require.NoError(t, err)
		require.NotNil(t, path)

		alpha := rasterizeGlyphPathForProbe(t, path, 2)
		for y := alpha.Bounds().Min.Y; y < alpha.Bounds().Max.Y; y++ {
			for x := alpha.Bounds().Min.X; x < alpha.Bounds().Max.X; x++ {
				a := alpha.AlphaAt(x, y).A
				if a > 0 {
					signature.nonZeroPixels++
					signature.alphaSum += uint64(a)
				}
			}
		}
	}
	return signature
}

func collectGlyphStripRasterSignature(t *testing.T, font entity.Font, codes []int) glyphStripRasterSignature {
	mask := rasterizeGlyphStripForProbe(t, font, codes, false)
	signature := glyphStripRasterSignature{width: mask.Bounds().Dx(), height: mask.Bounds().Dy()}
	for y := mask.Bounds().Min.Y; y < mask.Bounds().Max.Y; y++ {
		for x := mask.Bounds().Min.X; x < mask.Bounds().Max.X; x++ {
			a := mask.AlphaAt(x, y).A
			if a > 0 {
				signature.nonZeroPixels++
				signature.alphaSum += uint64(a)
			}
		}
	}
	return signature
}

func rasterizeGlyphStripForProbe(t *testing.T, font entity.Font, codes []int, roundAdvance bool) *image.Alpha {
	t.Helper()

	type placedGlyph struct {
		path  *entity.GlyphPath
		shift float64
	}

	placed := make([]placedGlyph, 0, len(codes))
	var advanceX float64
	minY, maxY, maxX := 1e9, -1e9, 0.0

	for _, code := range codes {
		glyph, err := font.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		width, err := font.GetGlyphWidth(glyph)
		require.NoError(t, err)

		path, err := font.RenderGlyph(glyph, 1000)
		require.NoError(t, err)
		require.NotNil(t, path)
		require.NotEmpty(t, path.Commands)

		minY = minFloatForProbe(minY, path.Bounds[1])
		maxY = maxFloatForProbe(maxY, path.Bounds[3])
		maxX = maxFloatForProbe(maxX, advanceX+path.Bounds[2])
		placed = append(placed, placedGlyph{path: path, shift: advanceX})

		if roundAdvance {
			advanceX += math.Round(width)
		} else {
			advanceX += width
		}
	}

	padding := 2
	width := int(maxX) + padding*2 + 2
	height := int(maxY-minY) + padding*2 + 2
	require.Greater(t, width, 0)
	require.Greater(t, height, 0)

	ras := vector.NewRasterizer(width, height)
	for _, placedGlyph := range placed {
		appendGlyphPathToRasterizerForProbe(t, ras, placedGlyph.path, placedGlyph.shift, minY, padding)
	}

	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	ras.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

func rasterizeGlyphStripAtScaleForProbe(
	t *testing.T,
	font entity.Font,
	codes []int,
	roundAdvance bool,
	scale float64,
) *image.Alpha {
	t.Helper()
	require.Greater(t, scale, 0.0)

	type placedGlyph struct {
		path  *entity.GlyphPath
		shift float64
	}

	placed := make([]placedGlyph, 0, len(codes))
	var advanceX float64
	minY, maxY, maxX := 1e9, -1e9, 0.0

	for _, code := range codes {
		glyph, err := font.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		width, err := font.GetGlyphWidth(glyph)
		require.NoError(t, err)

		path, err := font.RenderGlyph(glyph, 1000)
		require.NoError(t, err)
		require.NotNil(t, path)
		require.NotEmpty(t, path.Commands)

		minY = minFloatForProbe(minY, path.Bounds[1]*scale)
		maxY = maxFloatForProbe(maxY, path.Bounds[3]*scale)
		maxX = maxFloatForProbe(maxX, advanceX+path.Bounds[2]*scale)
		placed = append(placed, placedGlyph{path: path, shift: advanceX})

		advance := width * scale
		if roundAdvance {
			advanceX += math.Round(advance)
		} else {
			advanceX += advance
		}
	}

	padding := 2
	width := int(math.Ceil(maxX)) + padding*2 + 2
	height := int(math.Ceil(maxY-minY)) + padding*2 + 2
	require.Greater(t, width, 0)
	require.Greater(t, height, 0)

	ras := vector.NewRasterizer(width, height)
	for _, placedGlyph := range placed {
		appendScaledGlyphPathToRasterizerForProbe(t, ras, placedGlyph.path, placedGlyph.shift, minY, padding, scale)
	}

	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	ras.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

func rasterizeGlyphStripSupersampledReferenceForProbe(
	t *testing.T,
	font entity.Font,
	codes []int,
	roundAdvance bool,
	scale float64,
	supersample int,
) *image.Alpha {
	t.Helper()
	require.Greater(t, supersample, 1)

	hiRes := rasterizeGlyphStripAtScaleForProbe(t, font, codes, roundAdvance, scale*float64(supersample))
	return downsampleAlphaMaskForProbe(hiRes, supersample)
}

func rasterizeGlyphPathForProbe(t *testing.T, glyphPath *entity.GlyphPath, padding int) *image.Alpha {
	t.Helper()
	require.NotNil(t, glyphPath)
	require.NotEmpty(t, glyphPath.Commands)

	minX, minY := glyphPath.Bounds[0], glyphPath.Bounds[1]
	maxX, maxY := glyphPath.Bounds[2], glyphPath.Bounds[3]

	width := int(maxX-minX) + padding*2 + 2
	height := int(maxY-minY) + padding*2 + 2
	require.Greater(t, width, 0)
	require.Greater(t, height, 0)

	ras := vector.NewRasterizer(width, height)
	var startX, startY float64
	hasStart := false

	for _, cmd := range glyphPath.Commands {
		switch typed := cmd.(type) {
		case *entity.PathMoveTo:
			startX = typed.X - minX + float64(padding)
			startY = maxY - typed.Y + float64(padding)
			ras.MoveTo(float32(startX), float32(startY))
			hasStart = true
		case *entity.PathLineTo:
			require.True(t, hasStart)
			ras.LineTo(float32(typed.X-minX+float64(padding)), float32(maxY-typed.Y+float64(padding)))
		case *entity.PathCurveTo:
			require.True(t, hasStart)
			ras.CubeTo(
				float32(typed.X1-minX+float64(padding)),
				float32(maxY-typed.Y1+float64(padding)),
				float32(typed.X2-minX+float64(padding)),
				float32(maxY-typed.Y2+float64(padding)),
				float32(typed.X3-minX+float64(padding)),
				float32(maxY-typed.Y3+float64(padding)),
			)
		case *entity.PathClose:
			if hasStart {
				ras.LineTo(float32(startX), float32(startY))
				ras.ClosePath()
			}
		default:
			t.Fatalf("unsupported glyph path command type %T", cmd)
		}
	}

	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	ras.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

func appendGlyphPathToRasterizerForProbe(
	t *testing.T,
	ras *vector.Rasterizer,
	glyphPath *entity.GlyphPath,
	shiftX float64,
	globalMinY float64,
	padding int,
) {
	t.Helper()

	var startX, startY float64
	hasStart := false
	for _, cmd := range glyphPath.Commands {
		switch typed := cmd.(type) {
		case *entity.PathMoveTo:
			startX = typed.X + shiftX + float64(padding)
			startY = glyphPath.Bounds[3] - (typed.Y - globalMinY) + float64(padding)
			ras.MoveTo(float32(startX), float32(startY))
			hasStart = true
		case *entity.PathLineTo:
			require.True(t, hasStart)
			ras.LineTo(
				float32(typed.X+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]-(typed.Y-globalMinY)+float64(padding)),
			)
		case *entity.PathCurveTo:
			require.True(t, hasStart)
			ras.CubeTo(
				float32(typed.X1+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]-(typed.Y1-globalMinY)+float64(padding)),
				float32(typed.X2+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]-(typed.Y2-globalMinY)+float64(padding)),
				float32(typed.X3+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]-(typed.Y3-globalMinY)+float64(padding)),
			)
		case *entity.PathClose:
			if hasStart {
				ras.LineTo(float32(startX), float32(startY))
				ras.ClosePath()
			}
		default:
			t.Fatalf("unsupported glyph path command type %T", cmd)
		}
	}
}

func appendScaledGlyphPathToRasterizerForProbe(
	t *testing.T,
	ras *vector.Rasterizer,
	glyphPath *entity.GlyphPath,
	shiftX float64,
	globalMinY float64,
	padding int,
	scale float64,
) {
	t.Helper()

	var startX, startY float64
	hasStart := false
	for _, cmd := range glyphPath.Commands {
		switch typed := cmd.(type) {
		case *entity.PathMoveTo:
			startX = typed.X*scale + shiftX + float64(padding)
			startY = glyphPath.Bounds[3]*scale - (typed.Y*scale - globalMinY) + float64(padding)
			ras.MoveTo(float32(startX), float32(startY))
			hasStart = true
		case *entity.PathLineTo:
			require.True(t, hasStart)
			ras.LineTo(
				float32(typed.X*scale+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]*scale-(typed.Y*scale-globalMinY)+float64(padding)),
			)
		case *entity.PathCurveTo:
			require.True(t, hasStart)
			ras.CubeTo(
				float32(typed.X1*scale+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]*scale-(typed.Y1*scale-globalMinY)+float64(padding)),
				float32(typed.X2*scale+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]*scale-(typed.Y2*scale-globalMinY)+float64(padding)),
				float32(typed.X3*scale+shiftX+float64(padding)),
				float32(glyphPath.Bounds[3]*scale-(typed.Y3*scale-globalMinY)+float64(padding)),
			)
		case *entity.PathClose:
			if hasStart {
				ras.LineTo(float32(startX), float32(startY))
				ras.ClosePath()
			}
		default:
			t.Fatalf("unsupported glyph path command type %T", cmd)
		}
	}
}

func downsampleAlphaMaskForProbe(src *image.Alpha, factor int) *image.Alpha {
	if factor <= 1 {
		return src
	}

	dstWidth := (src.Bounds().Dx() + factor - 1) / factor
	dstHeight := (src.Bounds().Dy() + factor - 1) / factor
	dst := image.NewAlpha(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < dstHeight; y++ {
		for x := 0; x < dstWidth; x++ {
			var sum uint32
			var count uint32
			for sy := y * factor; sy < minIntForProbe((y+1)*factor, src.Bounds().Dy()); sy++ {
				for sx := x * factor; sx < minIntForProbe((x+1)*factor, src.Bounds().Dx()); sx++ {
					sum += uint32(src.AlphaAt(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy).A)
					count++
				}
			}
			if count == 0 {
				continue
			}
			dst.SetAlpha(x, y, color.Alpha{A: uint8(sum / count)})
		}
	}

	return dst
}

func compareAlphaMasksForProbe(left, right *image.Alpha) alphaDiffSignature {
	bounds := left.Bounds().Union(right.Bounds())
	signature := alphaDiffSignature{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			leftA := alphaAtForProbe(left, x, y)
			rightA := alphaAtForProbe(right, x, y)
			if leftA != rightA {
				signature.nonZeroPixels++
			}
			if leftA > rightA {
				signature.alphaAbsDiff += uint64(leftA - rightA)
			} else {
				signature.alphaAbsDiff += uint64(rightA - leftA)
			}
		}
	}
	return signature
}

func alphaMaskSumForProbe(img *image.Alpha) uint64 {
	var sum uint64
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			sum += uint64(img.AlphaAt(x, y).A)
		}
	}
	return sum
}

func alphaDiffRatioForProbe(left, right *image.Alpha) float64 {
	diff := compareAlphaMasksForProbe(left, right)
	sum := alphaMaskSumForProbe(right)
	if sum == 0 {
		return 0
	}
	return float64(diff.alphaAbsDiff) / float64(sum)
}

func alphaAtForProbe(img *image.Alpha, x, y int) uint8 {
	if !image.Pt(x, y).In(img.Bounds()) {
		return 0
	}
	return img.AlphaAt(x, y).A
}

func loadType1GlyphProbeFixture(
	t *testing.T,
	pdfPath string,
	pageNum int,
	fontResource string,
) (*entity.Document, type1GlyphProbeFixture) {
	t.Helper()

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(pageNum - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	fonts, ok := resolveFontDictForProbe(doc.XRef(), resources.Get(entity.Name("Font")))
	require.True(t, ok)
	require.NotNil(t, fonts)

	fontDictObj := fonts.Get(entity.Name(fontResource))
	fontDict, ok := resolveFontDictForProbe(doc.XRef(), fontDictObj)
	require.True(t, ok)
	require.NotNil(t, fontDict)

	return doc, type1GlyphProbeFixture{
		fontDict: fontDict,
		baseFont: nameValueForProbe(fontDict.Get(entity.Name("BaseFont"))),
	}
}

func loadResolvedType1ProbeFont(
	t *testing.T,
	pdfPath string,
	pageNum int,
	fontResource string,
) resolvedType1ProbeFont {
	t.Helper()

	// Probe tests were written under fallback-first assumptions.
	// Force fallback-first for these diagnostic tests.
	t.Setenv("PDF_DEBUG_TYPE1_MODE", "fallback-first")

	doc, fixture := loadType1GlyphProbeFixture(t, pdfPath, pageNum, fontResource)
	t.Setenv("PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE", fixture.baseFont)

	e := NewEvaluator(doc.XRef())
	font, err := e.getFontFromDict(fixture.fontDict, fixture.baseFont)
	require.NoError(t, err)
	require.NotNil(t, font)

	return resolvedType1ProbeFont{
		doc:  doc,
		font: font,
	}
}

func syntheticExpandedGlyphSetProbeCases() []syntheticExpandedGlyphSetProbeCase {
	return []syntheticExpandedGlyphSetProbeCase{
		{
			name:         "009_p95_sfrm1095",
			pdfPath:      "../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F16",
			topCodes: []int{
				101, 110, 105, 100, 117, 109,
			},
			secondaryCodes: []int{
				103, 97, 115, 116, 98, 114,
			},
			longTailCodes: []int{
				108, 111, 104, 118, 99, 107, 112,
			},
			nonLowerCodes: []int{
				44, 46, 75, 41, 40, 45, 69, 83, 228, 49, 50,
			},
			codes: []int{
				101, 110, 105, 100, 117, 109,
				103, 97, 115, 116, 98, 114,
				108, 111, 104, 118, 99, 107, 112,
				44, 46, 75, 41, 40, 45, 69, 83, 228, 49, 50,
			},
		},
		{
			name:         "009_p109_sfrm1095",
			pdfPath:      "../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      109,
			fontResource: "F16",
			topCodes: []int{
				101, 98, 110, 97, 109,
			},
			secondaryCodes: []int{
				111, 116, 105, 114, 115, 108,
			},
			longTailCodes: []int{
				99, 117, 100, 104, 107, 103, 120, 112, 102,
			},
			nonLowerCodes: []int{
				65, 49, 58, 44, 47, 48, 50, 51, 84,
			},
			codes: []int{
				101, 98, 110, 97, 109,
				111, 116, 105, 114, 115, 108,
				99, 117, 100, 104, 107, 103, 120, 112, 102,
				65, 49, 58, 44, 47, 48, 50, 51, 84,
			},
		},
	}
}

func loadSyntheticExpandedGlyphSetResolvedProbe(
	t *testing.T,
	tc syntheticExpandedGlyphSetProbeCase,
) syntheticExpandedGlyphSetResolvedProbe {
	t.Helper()

	resolved := loadResolvedType1ProbeFont(t, tc.pdfPath, tc.pageNum, tc.fontResource)
	return syntheticExpandedGlyphSetResolvedProbe{
		name:  tc.name,
		codes: tc.codes,
		doc:   resolved.doc,
		font:  resolved.font,
	}
}

func measureSyntheticExpandedGlyphSetOutlineProbeResult(
	t *testing.T,
	tc syntheticExpandedGlyphSetProbeCase,
) syntheticExpandedGlyphSetOutlineProbeResult {
	t.Helper()

	resolved := loadSyntheticExpandedGlyphSetResolvedProbe(t, tc)
	defer func() {
		require.NoError(t, resolved.doc.Close())
	}()

	return syntheticExpandedGlyphSetOutlineProbeResult{
		name:       tc.name,
		complexity: collectGlyphPathComplexitySignature(t, resolved.font, tc.codes),
		lowRatio:   syntheticLowScaleRasterErrorRatioForProbe(t, resolved.font, tc.codes, 0.02),
	}
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) largerLowRatioName() string {
	return testutil.LargerProbeName(r.page95Name, r.page95.lowRatio, r.page109Name, r.page109.lowRatio)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) largerLowRatioCanonicalKey() string {
	return testutil.LargerProbeCanonicalPageKey(r.page95Name, r.page95.lowRatio, r.page109Name, r.page109.lowRatio)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) largerAreaPerSegmentName() string {
	return testutil.LargerProbeName(
		r.page95Name,
		r.page95.complexity.areaPerSegment(),
		r.page109Name,
		r.page109.complexity.areaPerSegment(),
	)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) largerAreaPerSegmentCanonicalKey() string {
	return testutil.LargerProbeCanonicalPageKey(
		r.page95Name,
		r.page95.complexity.areaPerSegment(),
		r.page109Name,
		r.page109.complexity.areaPerSegment(),
	)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) lowerSegmentDensityName() string {
	return testutil.SmallerProbeName(
		r.page95Name,
		r.page95.complexity.segmentDensity(),
		r.page109Name,
		r.page109.complexity.segmentDensity(),
	)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) lowerSegmentDensityCanonicalKey() string {
	return testutil.SmallerProbeCanonicalPageKey(
		r.page95Name,
		r.page95.complexity.segmentDensity(),
		r.page109Name,
		r.page109.complexity.segmentDensity(),
	)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) lowerCurveShareName() string {
	return testutil.SmallerProbeName(
		r.page95Name,
		r.page95.complexity.curveShare(),
		r.page109Name,
		r.page109.complexity.curveShare(),
	)
}

func (r syntheticExpandedGlyphSetOutlineOrderingProbeResult) lowerCurveShareCanonicalKey() string {
	return testutil.SmallerProbeCanonicalPageKey(
		r.page95Name,
		r.page95.complexity.curveShare(),
		r.page109Name,
		r.page109.complexity.curveShare(),
	)
}

func measureSyntheticExpandedGlyphSetOutlineOrderingForProbe(
	t *testing.T,
) syntheticExpandedGlyphSetOutlineOrderingProbeResult {
	t.Helper()

	pair := measureSyntheticExpandedGlyphSetOutlineProbePair(t)

	return syntheticExpandedGlyphSetOutlineOrderingProbeResult{
		page95Name:  pair.page95.name,
		page95:      pair.page95,
		page109Name: pair.page109.name,
		page109:     pair.page109,
	}
}

func measureSyntheticExpandedGlyphSetOutlineProbePair(
	t *testing.T,
) syntheticExpandedGlyphSetOutlineProbePair {
	t.Helper()

	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	return syntheticExpandedGlyphSetOutlineProbePair{
		page95:  measureSyntheticExpandedGlyphSetOutlineProbeResult(t, cases[0]),
		page109: measureSyntheticExpandedGlyphSetOutlineProbeResult(t, cases[1]),
	}
}

func syntheticTimesWidthMappedFontForCodes(
	t *testing.T,
	source entity.Font,
	codes []int,
) *widthMappedFont {
	t.Helper()

	times, ok := standard.GetFont("Times-Roman")
	require.True(t, ok)

	widths := make(map[uint32]float64, len(codes))
	for _, code := range codes {
		targetGlyph, err := times.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		sourceGlyph, err := source.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		width, err := source.GetGlyphWidth(sourceGlyph)
		require.NoError(t, err)

		widths[targetGlyph] = width
	}

	return &widthMappedFont{
		base:   times,
		widths: widths,
	}
}

func type1GlyphProbeModes() []type1GlyphProbeMode {
	return []type1GlyphProbeMode{
		{name: "current"},
		{name: "force_helvetica", key: "PDF_DEBUG_FORCE_BASE_FONT_MAP", value: "SFRM1095=Helvetica,CMR10=Helvetica"},
		{name: "force_courier", key: "PDF_DEBUG_FORCE_BASE_FONT_MAP", value: "SFRM1095=Courier,CMR10=Courier"},
	}
}

func applyType1GlyphProbeMode(t *testing.T, mode type1GlyphProbeMode) {
	t.Helper()
	if mode.key != "" {
		t.Setenv(mode.key, mode.value)
	}
}

func minFloatForProbe(left, right float64) float64 {
	if right < left {
		return right
	}
	return left
}

func maxFloatForProbe(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}

func minIntForProbe(left, right int) int {
	if right < left {
		return right
	}
	return left
}
