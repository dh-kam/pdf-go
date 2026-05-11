package pdf

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaintext "github.com/dh-kam/pdf-go/internal/domain/text"
)

func TestTextAPIs_EmptyPageAndRangePaths(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 200, 200}, rotate: 0},
	})

	text, err := doc.Text(0)
	require.NoError(t, err)
	assert.Equal(t, "", text)

	_, err = doc.Text(10)
	require.Error(t, err)

	_, err = doc.TextRange(0, 3, 1)
	require.Error(t, err)

	got, err := doc.TextRange(0, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "", got)

	got, err = doc.TextRange(0, 10, 20)
	require.NoError(t, err)
	assert.Equal(t, "", got)

	lines, err := doc.TextLines(0)
	require.NoError(t, err)
	assert.Empty(t, lines)

	paragraphs, err := doc.TextParagraphs(0)
	require.NoError(t, err)
	assert.Empty(t, paragraphs)
}

func TestTextHelpers(t *testing.T) {
	items := []domaintext.TextItem{
		{
			Text:        "  c  ",
			Font:        "F1",
			FontSize:    11,
			BoundingBox: image.Rect(20, 10, 40, 20),
		},
		{
			Text:        "a",
			Font:        "F1",
			FontSize:    10,
			BoundingBox: image.Rect(0, 40, 10, 50),
		},
		{
			Text:        " b ",
			Font:        "F1",
			FontSize:    10,
			BoundingBox: image.Rect(12, 40, 20, 50),
		},
		{
			Text:        "   ",
			BoundingBox: image.Rect(0, 0, 1, 1),
		},
	}

	sorted := toSortedTextItems(items)
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].Text)
	assert.Equal(t, "b", sorted[1].Text)
	assert.Equal(t, "c", sorted[2].Text)

	line := joinLineText([]TextItem{
		{Text: "a", X: 0, Width: 5},
		{Text: "b", X: 8, Width: 5},
		{Text: "c", X: 20, Width: 5},
	})
	assert.Equal(t, "a b c", line)

	paragraph := joinParagraphText([]TextLine{
		{Text: "line1"},
		{Text: ""},
		{Text: "line2"},
	})
	assert.Equal(t, "line1\nline2", paragraph)

	assert.Equal(t, 12.0, maxLineHeight(TextLine{
		Items: []TextItem{{Height: 8}, {Height: 12}, {Height: 3}},
	}))
	assert.Equal(t, 0.0, maxLineHeight(TextLine{}))

	assert.Equal(t, "hello world", normalizeWhitespace("  hello\t world \n"))
	assert.Equal(t, "", normalizeWhitespace(""))
	assert.True(t, endsWithSpace("x "))
	assert.False(t, endsWithSpace("x"))
	assert.False(t, endsWithSpace(""))
	assert.Equal(t, 3.5, abs(-3.5))
	assert.Equal(t, 2.0, abs(2.0))
}
