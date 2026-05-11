package text_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/text"
	infrastructureText "github.com/dh-kam/pdf-go/internal/infrastructure/text"
)

func TestTextLayer_NewLayer(t *testing.T) {
	layer := &text.TextLayer{}

	assert.NotNil(t, layer)
	assert.Equal(t, 0, len(layer.GetItems()))
	assert.Equal(t, "", layer.Text())
}

func TestTextLayer_AddItem(t *testing.T) {
	layer := &text.TextLayer{}

	item := text.TextItem{
		Text:     "Hello",
		FontSize: 12,
	}

	layer.AddItem(item)

	assert.Equal(t, 1, len(layer.GetItems()))
	assert.Equal(t, "Hello", layer.Text())
}

func TestTextLayer_MultipleItems(t *testing.T) {
	layer := &text.TextLayer{}

	layer.AddItem(text.TextItem{Text: "Hello"})
	layer.AddItem(text.TextItem{Text: " "})
	layer.AddItem(text.TextItem{Text: "World"})

	assert.Equal(t, 3, len(layer.GetItems()))
	assert.Equal(t, "Hello World", layer.Text())
}

func TestTextLayer_BoundingBox(t *testing.T) {
	layer := &text.TextLayer{}

	item := text.TextItem{
		Text:     "Test",
		FontSize: 12,
	}
	// BoundingBox would be set by the extractor
	// For testing, we can verify the field exists

	layer.AddItem(item)

	items := layer.GetItems()
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "Test", items[0].Text)
}

func TestTextExtractor_NewExtractor(t *testing.T) {
	extractor := infrastructureText.NewExtractor()

	assert.NotNil(t, extractor)
}

func TestTextExtractor_Options(t *testing.T) {
	extractor := infrastructureText.NewExtractor()

	// Test preserve spacing
	extractor.SetPreserveSpacing(true)
	extractor.SetPreserveSpacing(false)

	// Test include invisible
	extractor.SetIncludeInvisible(true)
	extractor.SetIncludeInvisible(false)
}

func TestTextExtractor_ExtractEmptyPage(t *testing.T) {
	extractor := infrastructureText.NewExtractor()

	// Create a minimal page with no content
	page := entity.NewTestPage()

	layer, err := extractor.Extract(page)
	require.NoError(t, err)
	assert.NotNil(t, layer)
	assert.Equal(t, 0, len(layer.GetItems()))
}

func TestTextExtractor_ExtractToTextEmptyPage(t *testing.T) {
	extractor := infrastructureText.NewExtractor()

	page := entity.NewTestPage()

	text, err := extractor.ExtractToText(page)
	require.NoError(t, err)
	assert.Equal(t, "", text)
}

func TestTextItem_WritingMode(t *testing.T) {
	item := text.TextItem{
		Text:        "Test",
		WritingMode: text.WritingModeHorizontal,
	}

	assert.Equal(t, "Test", item.Text)
	assert.Equal(t, text.WritingModeHorizontal, item.WritingMode)
	assert.Equal(t, 0, int(item.WritingMode))

	item.WritingMode = text.WritingModeVertical
	assert.Equal(t, text.WritingModeVertical, item.WritingMode)
	assert.Equal(t, 1, int(item.WritingMode))
}

func TestTextItem_FontProperties(t *testing.T) {
	item := text.TextItem{
		Text:     "Test",
		Font:     "Helvetica",
		FontSize: 14.5,
	}

	assert.Equal(t, "Test", item.Text)
	assert.Equal(t, "Helvetica", item.Font)
	assert.Equal(t, 14.5, item.FontSize)
}

func TestTextLayer_EmptyText(t *testing.T) {
	layer := &text.TextLayer{}

	// Add items with empty text
	layer.AddItem(text.TextItem{Text: ""})
	layer.AddItem(text.TextItem{Text: "Hello"})
	layer.AddItem(text.TextItem{Text: ""})

	assert.Equal(t, 3, len(layer.GetItems()))
	assert.Equal(t, "Hello", layer.Text())
}

func TestTextLayer_SpecialCharacters(t *testing.T) {
	layer := &text.TextLayer{}

	layer.AddItem(text.TextItem{Text: "Hello"})
	layer.AddItem(text.TextItem{Text: "世"})
	layer.AddItem(text.TextItem{Text: "界"})
	assert.Equal(t, "Hello世界", layer.Text())
}

func TestTextExtractor_ExtractionModes(t *testing.T) {
	extractor := infrastructureText.NewExtractor()

	// Test with preserve spacing
	extractor.SetPreserveSpacing(true)

	// Test without preserve spacing
	extractor.SetPreserveSpacing(false)

	// Test with invisible text
	extractor.SetIncludeInvisible(true)

	// Test without invisible text
	extractor.SetIncludeInvisible(false)
}

func TestExtractFromStream_Empty(t *testing.T) {
	// Test extracting from an empty or null stream
	result, err := infrastructureText.ExtractFromStream(nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestTextLayer_Concatenation(t *testing.T) {
	layer := &text.TextLayer{}

	// Test that text is properly concatenated
	layer.AddItem(text.TextItem{Text: "A"})
	layer.AddItem(text.TextItem{Text: "B"})
	layer.AddItem(text.TextItem{Text: "C"})

	assert.Equal(t, "ABC", layer.Text())
}

func TestTextLayer_GetItems(t *testing.T) {
	layer := &text.TextLayer{}

	item1 := text.TextItem{Text: "First"}
	item2 := text.TextItem{Text: "Second"}

	layer.AddItem(item1)
	layer.AddItem(item2)

	items := layer.GetItems()
	assert.Equal(t, 2, len(items))
	assert.Equal(t, "First", items[0].Text)
	assert.Equal(t, "Second", items[1].Text)
}

func TestWritingMode_Constants(t *testing.T) {
	// Test that writing mode constants are properly defined
	assert.Equal(t, 0, int(text.WritingModeHorizontal))
	assert.Equal(t, 1, int(text.WritingModeVertical))
}
