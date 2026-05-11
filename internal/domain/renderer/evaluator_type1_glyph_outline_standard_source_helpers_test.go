package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

type standardGlyphSourceProbeResult struct {
	fontName   string
	code       int
	glyph      uint32
	glyphName  string
	path       *entity.GlyphPath
	bounds     [4]float64
	complexity glyphPathComplexitySignature
	lowRatio   float64
}

type standardGlyphSourceAlternativesProbeResult struct {
	code        int
	sources     []standardGlyphSourceProbeResult
	bestDensity standardGlyphSourceProbeResult
}

type standardGlyphSourceSelectionProbeResult struct {
	code            int
	bestDensityFont string
	bestDensity     float64
}

type standardGlyphSourceSelectionSetProbeResult struct {
	selections []standardGlyphSourceSelectionProbeResult
}

type standardGlyphSourceSelectionSetSummaryProbeResult struct {
	codeCount        int
	timesDensityAvg  float64
	bestDensityAvg   float64
	timesLowRatioAvg float64
	bestLowRatioAvg  float64
	maxLowRatioDelta float64
}

func lowRatioSpreadForProbe(
	result standardGlyphSourceAlternativesProbeResult,
) float64 {
	if len(result.sources) == 0 {
		return 0
	}

	lowest := result.sources[0].lowRatio
	highest := result.sources[0].lowRatio
	for _, source := range result.sources[1:] {
		if source.lowRatio < lowest {
			lowest = source.lowRatio
		}
		if source.lowRatio > highest {
			highest = source.lowRatio
		}
	}

	return highest - lowest
}

func measureStandardGlyphSourceProbe(
	t *testing.T,
	fontName string,
	code int,
) standardGlyphSourceProbeResult {
	t.Helper()

	font, ok := standard.GetFont(fontName)
	require.True(t, ok)

	glyph, err := font.CharCodeToGlyph(uint32(code))
	require.NoError(t, err)

	path, err := font.RenderGlyph(glyph, 1000)
	require.NoError(t, err)
	require.NotNil(t, path)

	return standardGlyphSourceProbeResult{
		fontName:   fontName,
		code:       code,
		glyph:      glyph,
		glyphName:  font.GlyphName(glyph),
		path:       path,
		bounds:     path.Bounds,
		complexity: collectGlyphPathComplexitySignature(t, font, []int{code}),
		lowRatio:   syntheticLowScaleRasterErrorRatioForProbe(t, font, []int{code}, 0.02),
	}
}

func measureStandardGlyphSourceAlternativesForProbe(
	t *testing.T,
	code int,
	fontNames ...string,
) standardGlyphSourceAlternativesProbeResult {
	t.Helper()
	require.NotEmpty(t, fontNames)

	sources := make([]standardGlyphSourceProbeResult, 0, len(fontNames))
	for _, fontName := range fontNames {
		sources = append(sources, measureStandardGlyphSourceProbe(t, fontName, code))
	}

	best := sources[0]
	for _, candidate := range sources[1:] {
		if candidate.complexity.segmentDensity() > best.complexity.segmentDensity() {
			best = candidate
		}
	}

	return standardGlyphSourceAlternativesProbeResult{
		code:        code,
		sources:     sources,
		bestDensity: best,
	}
}

func sourceByFontNameForProbe(
	t *testing.T,
	result standardGlyphSourceAlternativesProbeResult,
	fontName string,
) standardGlyphSourceProbeResult {
	t.Helper()

	for _, source := range result.sources {
		if source.fontName == fontName {
			return source
		}
	}

	t.Fatalf("font %s not found for code %d", fontName, result.code)
	return standardGlyphSourceProbeResult{}
}

func measureStandardGlyphSourceSelectionSetForProbe(
	t *testing.T,
	codes []int,
	fontNames ...string,
) standardGlyphSourceSelectionSetProbeResult {
	t.Helper()
	require.NotEmpty(t, codes)

	selections := make([]standardGlyphSourceSelectionProbeResult, 0, len(codes))
	for _, code := range codes {
		result := measureStandardGlyphSourceAlternativesForProbe(t, code, fontNames...)
		selections = append(selections, standardGlyphSourceSelectionProbeResult{
			code:            code,
			bestDensityFont: result.bestDensity.fontName,
			bestDensity:     result.bestDensity.complexity.segmentDensity(),
		})
	}

	return standardGlyphSourceSelectionSetProbeResult{selections: selections}
}

func selectionByCodeForProbe(
	t *testing.T,
	result standardGlyphSourceSelectionSetProbeResult,
	code int,
) standardGlyphSourceSelectionProbeResult {
	t.Helper()

	for _, selection := range result.selections {
		if selection.code == code {
			return selection
		}
	}

	t.Fatalf("selection for code %d not found", code)
	return standardGlyphSourceSelectionProbeResult{}
}

func distinctBestDensityFontsForProbe(
	result standardGlyphSourceSelectionSetProbeResult,
) []string {
	seen := make(map[string]struct{}, len(result.selections))
	fonts := make([]string, 0, len(result.selections))
	for _, selection := range result.selections {
		if _, ok := seen[selection.bestDensityFont]; ok {
			continue
		}
		seen[selection.bestDensityFont] = struct{}{}
		fonts = append(fonts, selection.bestDensityFont)
	}
	return fonts
}

func measureStandardGlyphSourceSelectionSetSummaryForProbe(
	t *testing.T,
	codes []int,
	fontNames ...string,
) standardGlyphSourceSelectionSetSummaryProbeResult {
	t.Helper()
	require.NotEmpty(t, codes)

	var timesDensitySum float64
	var bestDensitySum float64
	var timesLowRatioSum float64
	var bestLowRatioSum float64
	var maxLowRatioDelta float64

	for _, code := range codes {
		result := measureStandardGlyphSourceAlternativesForProbe(t, code, fontNames...)
		times := sourceByFontNameForProbe(t, result, "Times-Roman")
		timesDensitySum += times.complexity.segmentDensity()
		bestDensitySum += result.bestDensity.complexity.segmentDensity()
		timesLowRatioSum += times.lowRatio
		bestLowRatioSum += result.bestDensity.lowRatio
		if spread := lowRatioSpreadForProbe(result); spread > maxLowRatioDelta {
			maxLowRatioDelta = spread
		}
	}

	codeCount := float64(len(codes))
	return standardGlyphSourceSelectionSetSummaryProbeResult{
		codeCount:        len(codes),
		timesDensityAvg:  timesDensitySum / codeCount,
		bestDensityAvg:   bestDensitySum / codeCount,
		timesLowRatioAvg: timesLowRatioSum / codeCount,
		bestLowRatioAvg:  bestLowRatioSum / codeCount,
		maxLowRatioDelta: maxLowRatioDelta,
	}
}
