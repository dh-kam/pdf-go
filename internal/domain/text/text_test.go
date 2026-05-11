package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextLayer_AddItemAndText(t *testing.T) {
	layer := &TextLayer{}
	layer.AddItem(TextItem{Text: "Hello"})
	layer.AddItem(TextItem{Text: " "})
	layer.AddItem(TextItem{Text: "World"})

	items := layer.GetItems()
	assert.Len(t, items, 3)
	assert.Equal(t, "Hello World", layer.Text())
	assert.Equal(t, "Hello", items[0].Text)
	assert.Equal(t, " ", items[1].Text)
	assert.Equal(t, "World", items[2].Text)
}

func TestTextLayer_Empty(t *testing.T) {
	layer := &TextLayer{}

	assert.Equal(t, 0, len(layer.GetItems()))
	assert.Equal(t, "", layer.Text())
}

func TestNewTextExtractor_Defaults(t *testing.T) {
	extractor := NewTextExtractor()
	assert.NotNil(t, extractor)

	extractor.SetPreserveSpacing(false)
	extractor.SetPreserveSpacing(true)
	extractor.SetIncludeInvisible(true)
	extractor.SetIncludeInvisible(false)
}

func TestTextItemFields(t *testing.T) {
	item := TextItem{
		Text:     "Sample",
		Unicode:  "U+0053",
		Font:     "Helvetica",
		FontSize: 12.5,
	}

	assert.Equal(t, "Sample", item.Text)
	assert.Equal(t, "U+0053", item.Unicode)
	assert.Equal(t, "Helvetica", item.Font)
	assert.Equal(t, 12.5, item.FontSize)
}

func TestWritingModeConstants(t *testing.T) {
	assert.Equal(t, WritingModeHorizontal, WritingMode(0))
	assert.Equal(t, WritingModeVertical, WritingMode(1))
}
