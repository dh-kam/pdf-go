package pdf

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"

	domaintext "github.com/dh-kam/pdf-go/internal/domain/text"
)

func TestFindTextSearchRanges_Basic(t *testing.T) {
	ranges := findTextSearchRanges("Hello hello HELLO", "hello", false, false, 0)
	if assert.Len(t, ranges, 3) {
		assert.Equal(t, textSearchRange{start: 0, end: 5}, ranges[0])
		assert.Equal(t, textSearchRange{start: 6, end: 11}, ranges[1])
		assert.Equal(t, textSearchRange{start: 12, end: 17}, ranges[2])
	}
}

func TestFindTextSearchRanges_WholeWord(t *testing.T) {
	ranges := findTextSearchRanges("cat catalog scatter cat", "cat", false, true, 0)
	if assert.Len(t, ranges, 2) {
		assert.Equal(t, textSearchRange{start: 0, end: 3}, ranges[0])
		assert.Equal(t, textSearchRange{start: 20, end: 23}, ranges[1])
	}
}

func TestFindTextSearchRanges_MaxResults(t *testing.T) {
	ranges := findTextSearchRanges("a a a a", "a", true, true, 2)
	assert.Len(t, ranges, 2)
}

func TestResolveSearchPageRange(t *testing.T) {
	start, end, err := resolveSearchPageRange(10, -1, -1)
	assert.NoError(t, err)
	assert.Equal(t, 0, start)
	assert.Equal(t, 9, end)

	_, _, err = resolveSearchPageRange(10, 5, 3)
	assert.Error(t, err)
}

func TestBuildTextSearchContext(t *testing.T) {
	text := []rune("0123456789 abcdefghijklmnopqrstuvwxyz 0123456789")
	context := buildTextSearchContext(text, 11, 16)
	assert.Contains(t, context, "abcde")
	assert.Contains(t, context, "...")
}

func TestTextSearchResultSetHelpers(t *testing.T) {
	set := &TextSearchResultSet{
		Query: "lorem",
		Pages: []TextSearchPageResult{
			{
				PageIndex: 0,
				Matches: []TextSearchMatch{
					{PageIndex: 0, Start: 0, End: 5, Text: "Lorem"},
				},
			},
			{
				PageIndex: 2,
				Matches: []TextSearchMatch{
					{PageIndex: 2, Start: 3, End: 8, Text: "lorem"},
					{PageIndex: 2, Start: 9, End: 14, Text: "lorem"},
				},
			},
		},
	}

	assert.Equal(t, 2, set.SearchedPageCount())
	assert.Len(t, set.SearchedForPosition(0), 1)
	assert.Len(t, set.SearchedForPosition(1), 2)
	assert.Nil(t, set.SearchedForPosition(2))
	assert.Len(t, set.FlattenedMatches(), 3)
}

func TestGroupMatchesByPage(t *testing.T) {
	grouped := groupMatchesByPage([]TextSearchMatch{
		{PageIndex: 3, Text: "A"},
		{PageIndex: 1, Text: "B"},
		{PageIndex: 3, Text: "C"},
	})

	if assert.Len(t, grouped, 2) {
		assert.Equal(t, 3, grouped[0].PageIndex)
		assert.Len(t, grouped[0].Matches, 2)
		assert.Equal(t, 1, grouped[1].PageIndex)
		assert.Len(t, grouped[1].Matches, 1)
	}
}

func TestBuildTextSearchItemSpans(t *testing.T) {
	spans := buildTextSearchItemSpans([]domaintext.TextItem{
		{Text: "Hi", BoundingBox: image.Rect(1, 2, 11, 12)},
		{Text: " ", BoundingBox: image.Rect(11, 2, 12, 12)},
		{Text: "Go", BoundingBox: image.Rect(12, 2, 22, 12)},
	})

	if assert.Len(t, spans, 3) {
		assert.Equal(t, 0, spans[0].start)
		assert.Equal(t, 2, spans[0].end)
		assert.Equal(t, 2, spans[1].start)
		assert.Equal(t, 3, spans[1].end)
		assert.Equal(t, 3, spans[2].start)
		assert.Equal(t, 5, spans[2].end)
	}
}

func TestComputeSearchMatchGeometry(t *testing.T) {
	spans := []textSearchItemSpan{
		{start: 0, end: 2, xMin: 1, yMin: 10, xMax: 11, yMax: 20},
		{start: 3, end: 5, xMin: 12, yMin: 12, xMax: 22, yMax: 22},
	}

	bounds, pgPoints, ok := computeSearchMatchGeometry(0, 5, spans)
	assert.True(t, ok)
	assert.Equal(t, [4]float64{1, 10, 22, 22}, bounds)
	assert.Equal(t, []float64{1, 10, 22, 10, 22, 22, 1, 22}, pgPoints)
}
