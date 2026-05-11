package renderer

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type syntheticGlyphCodeOutlineProbeResult struct {
	code           int
	glyph          uint32
	glyphName      string
	decodedRune    rune
	hasDecodedRune bool
	path           *entity.GlyphPath
	bounds         [4]float64
	complexity     glyphPathComplexitySignature
	lowRatio       float64
}

type syntheticGlyphCodeOrderingProbeResult struct {
	lowestDensity syntheticGlyphCodeOutlineProbeResult
	lowestCurve   syntheticGlyphCodeOutlineProbeResult
}

type syntheticNonLowerCode49Code58OrderingProbeResult struct {
	code49 syntheticGlyphCodeOutlineProbeResult
	code58 syntheticGlyphCodeOutlineProbeResult
}

type glyphLineSegmentProbe struct {
	fromX float64
	fromY float64
	toX   float64
	toY   float64
}

func (s glyphLineSegmentProbe) absDX() float64 {
	return math.Abs(s.toX - s.fromX)
}

func (s glyphLineSegmentProbe) absDY() float64 {
	return math.Abs(s.toY - s.fromY)
}

func (r syntheticGlyphCodeOutlineProbeResult) boundsWidth() float64 {
	return r.bounds[2] - r.bounds[0]
}

func (r syntheticGlyphCodeOutlineProbeResult) boundsHeight() float64 {
	return r.bounds[3] - r.bounds[1]
}

func (r syntheticGlyphCodeOutlineProbeResult) boundsAspectRatio() float64 {
	width := r.boundsWidth()
	if width == 0 {
		return 0
	}
	return r.boundsHeight() / width
}

func (r syntheticGlyphCodeOutlineProbeResult) segmentsPerWidth() float64 {
	width := r.boundsWidth()
	if width == 0 {
		return 0
	}
	return float64(r.complexity.totalSegments()) / width
}

func (r syntheticGlyphCodeOutlineProbeResult) verticalExtent() float64 {
	return r.boundsHeight()
}

func (r syntheticGlyphCodeOutlineProbeResult) verticalSegmentSpanPerWidth() float64 {
	width := r.boundsWidth()
	if width == 0 {
		return 0
	}
	return maxAbsDYForSegments(lineSegmentsForProbeFromResult(r)) / width
}

func (r syntheticGlyphCodeOutlineProbeResult) verticalStrokeComplexity() float64 {
	return r.verticalSegmentSpanPerWidth() * float64(r.complexity.totalSegments())
}

func lineSegmentsForProbe(
	t *testing.T,
	result syntheticGlyphCodeOutlineProbeResult,
) []glyphLineSegmentProbe {
	t.Helper()
	require.NotNil(t, result.path)

	return lineSegmentsFromPathForProbe(t, result.path)
}

func lineSegmentsFromPathForProbe(
	t *testing.T,
	path *entity.GlyphPath,
) []glyphLineSegmentProbe {
	t.Helper()
	require.NotNil(t, path)

	segments := make([]glyphLineSegmentProbe, 0, len(path.Commands))
	var currentX, currentY float64
	var hasCurrent bool
	for _, cmd := range path.Commands {
		switch typed := cmd.(type) {
		case *entity.PathMoveTo:
			currentX = typed.X
			currentY = typed.Y
			hasCurrent = true
		case *entity.PathLineTo:
			require.True(t, hasCurrent)
			segments = append(segments, glyphLineSegmentProbe{
				fromX: currentX,
				fromY: currentY,
				toX:   typed.X,
				toY:   typed.Y,
			})
			currentX = typed.X
			currentY = typed.Y
		case *entity.PathCurveTo:
			currentX = typed.X3
			currentY = typed.Y3
			hasCurrent = true
		case *entity.PathClose:
		default:
			t.Fatalf("unsupported glyph path command type %T", cmd)
		}
	}

	return segments
}

func maxAbsDXForSegments(segments []glyphLineSegmentProbe) float64 {
	var maxAbsDX float64
	for _, segment := range segments {
		if segment.absDX() > maxAbsDX {
			maxAbsDX = segment.absDX()
		}
	}
	return maxAbsDX
}

func maxAbsDYForSegments(segments []glyphLineSegmentProbe) float64 {
	var maxAbsDY float64
	for _, segment := range segments {
		if segment.absDY() > maxAbsDY {
			maxAbsDY = segment.absDY()
		}
	}
	return maxAbsDY
}

func codeCanonicalKeyForProbe(code int) string {
	return fmt.Sprintf("code%d", code)
}

func measureSyntheticGlyphCodeOutlineProbeResults(
	t *testing.T,
	tc syntheticExpandedGlyphSetProbeCase,
	codes []int,
) []syntheticGlyphCodeOutlineProbeResult {
	t.Helper()

	resolved := loadSyntheticExpandedGlyphSetResolvedProbe(t, tc)
	defer func() {
		require.NoError(t, resolved.doc.Close())
	}()

	results := make([]syntheticGlyphCodeOutlineProbeResult, 0, len(codes))
	for _, code := range codes {
		glyph, err := resolved.font.CharCodeToGlyph(uint32(code))
		require.NoError(t, err)

		glyphName := resolved.font.GlyphName(glyph)
		decodedRune, hasDecodedRune := decodeGlyphName(glyphName)
		path, err := resolved.font.RenderGlyph(glyph, 1000)
		require.NoError(t, err)
		require.NotNil(t, path)

		results = append(results, syntheticGlyphCodeOutlineProbeResult{
			code:           code,
			glyph:          glyph,
			glyphName:      glyphName,
			decodedRune:    decodedRune,
			hasDecodedRune: hasDecodedRune,
			path:           path,
			bounds:         path.Bounds,
			complexity:     collectGlyphPathComplexitySignature(t, resolved.font, []int{code}),
			lowRatio:       syntheticLowScaleRasterErrorRatioForProbe(t, resolved.font, []int{code}, 0.02),
		})
	}
	return results
}

func lowestSegmentDensityCodeForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	lowest := results[0]
	for _, result := range results[1:] {
		if result.complexity.segmentDensity() < lowest.complexity.segmentDensity() {
			lowest = result
		}
	}
	return lowest
}

func lowestCurveShareCodeForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	lowest := results[0]
	for _, result := range results[1:] {
		if result.complexity.curveShare() < lowest.complexity.curveShare() {
			lowest = result
		}
	}
	return lowest
}

func highestLowRatioCodeForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	highest := results[0]
	for _, result := range results[1:] {
		if result.lowRatio > highest.lowRatio {
			highest = result
		}
	}
	return highest
}

func highestSegmentsPerWidthCodeForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	highest := results[0]
	for _, result := range results[1:] {
		if result.segmentsPerWidth() > highest.segmentsPerWidth() {
			highest = result
		}
	}
	return highest
}

func highestVerticalSegmentSpanPerWidthCodeForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	highest := results[0]
	for _, result := range results[1:] {
		if result.verticalSegmentSpanPerWidth() > highest.verticalSegmentSpanPerWidth() {
			highest = result
		}
	}
	return highest
}

func highestVerticalStrokeComplexityCodeForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	highest := results[0]
	for _, result := range results[1:] {
		if result.verticalStrokeComplexity() > highest.verticalStrokeComplexity() {
			highest = result
		}
	}
	return highest
}

func codeByValueForProbe(
	t *testing.T,
	results []syntheticGlyphCodeOutlineProbeResult,
	code int,
) syntheticGlyphCodeOutlineProbeResult {
	t.Helper()

	for _, result := range results {
		if result.code == code {
			return result
		}
	}

	t.Fatalf("code %d not found", code)
	return syntheticGlyphCodeOutlineProbeResult{}
}

func measureSyntheticGlyphCodeOrderingForProbe(
	t *testing.T,
	tc syntheticExpandedGlyphSetProbeCase,
	codes []int,
) syntheticGlyphCodeOrderingProbeResult {
	t.Helper()

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, tc, codes)
	return syntheticGlyphCodeOrderingProbeResult{
		lowestDensity: lowestSegmentDensityCodeForProbe(t, results),
		lowestCurve:   lowestCurveShareCodeForProbe(t, results),
	}
}

func measureSyntheticNonLowerCode49Code58OrderingForProbe(
	t *testing.T,
) syntheticNonLowerCode49Code58OrderingProbeResult {
	t.Helper()

	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	return syntheticNonLowerCode49Code58OrderingProbeResult{
		code49: codeByValueForProbe(t, results, 49),
		code58: codeByValueForProbe(t, results, 58),
	}
}

func measurePage109NonLowerCodeOutlineProbeResultsForProbe(
	t *testing.T,
) []syntheticGlyphCodeOutlineProbeResult {
	t.Helper()

	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	return measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
}

func (r syntheticNonLowerCode49Code58OrderingProbeResult) largerLowRatioCanonicalKey() string {
	if r.code49.lowRatio > r.code58.lowRatio {
		return codeCanonicalKeyForProbe(r.code49.code)
	}
	return codeCanonicalKeyForProbe(r.code58.code)
}

func (r syntheticNonLowerCode49Code58OrderingProbeResult) largerVerticalExtentCanonicalKey() string {
	if r.code49.verticalExtent() > r.code58.verticalExtent() {
		return codeCanonicalKeyForProbe(r.code49.code)
	}
	return codeCanonicalKeyForProbe(r.code58.code)
}

func (r syntheticNonLowerCode49Code58OrderingProbeResult) largerVerticalSegmentSpanCanonicalKey() string {
	segments49 := lineSegmentsForProbeFromResult(r.code49)
	segments58 := lineSegmentsForProbeFromResult(r.code58)
	if maxAbsDYForSegments(segments49) > maxAbsDYForSegments(segments58) {
		return codeCanonicalKeyForProbe(r.code49.code)
	}
	return codeCanonicalKeyForProbe(r.code58.code)
}

func (r syntheticNonLowerCode49Code58OrderingProbeResult) largerVerticalSegmentSpanPerWidthCanonicalKey() string {
	if r.code49.verticalSegmentSpanPerWidth() > r.code58.verticalSegmentSpanPerWidth() {
		return codeCanonicalKeyForProbe(r.code49.code)
	}
	return codeCanonicalKeyForProbe(r.code58.code)
}

func lineSegmentsForProbeFromResult(
	result syntheticGlyphCodeOutlineProbeResult,
) []glyphLineSegmentProbe {
	if result.path == nil {
		return nil
	}
	segments := make([]glyphLineSegmentProbe, 0, len(result.path.Commands))
	var currentX, currentY float64
	var hasCurrent bool
	for _, cmd := range result.path.Commands {
		switch typed := cmd.(type) {
		case *entity.PathMoveTo:
			currentX = typed.X
			currentY = typed.Y
			hasCurrent = true
		case *entity.PathLineTo:
			if !hasCurrent {
				return nil
			}
			segments = append(segments, glyphLineSegmentProbe{
				fromX: currentX,
				fromY: currentY,
				toX:   typed.X,
				toY:   typed.Y,
			})
			currentX = typed.X
			currentY = typed.Y
		case *entity.PathCurveTo:
			currentX = typed.X3
			currentY = typed.Y3
			hasCurrent = true
		case *entity.PathClose:
		}
	}
	return segments
}
