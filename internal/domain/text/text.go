// Package text provides text extraction interfaces for PDF documents.
//
//revive:disable:exported
package text

import (
	"image"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// TextItem represents a piece of extracted text with position information.
type TextItem struct {
	Text        string
	Unicode     string
	Font        string
	FontSize    float64
	BoundingBox image.Rectangle // In PDF coordinate space (bottom-up)
	WritingMode WritingMode     // 0=horizontal, 1=vertical
}

// TextLayer represents a layer of text on a PDF page.
type TextLayer struct {
	Items []TextItem
}

// AddItem adds a text item to the layer.
func (l *TextLayer) AddItem(item TextItem) {
	l.Items = append(l.Items, item)
}

// GetItems returns all text items in the layer.
func (l *TextLayer) GetItems() []TextItem {
	return l.Items
}

// Text returns the concatenated text of all items.
func (l *TextLayer) Text() string {
	var result string
	for _, item := range l.Items {
		result += item.Text
	}
	return result
}

// Extractor extracts text from PDF pages.
type Extractor interface {
	// Extract extracts text from a page.
	Extract(page *entity.Page) (*TextLayer, error)

	// ExtractToText extracts text and returns it as a string.
	ExtractToText(page *entity.Page) (string, error)
}

// WritingMode represents the text writing direction.
type WritingMode int

const (
	WritingModeHorizontal WritingMode = iota
	WritingModeVertical
)

// TextExtractor extracts text from PDF content streams.
type TextExtractor struct {
	// Configuration
	preserveSpacing  bool
	includeInvisible bool
}

// NewTextExtractor creates a new text extractor.
func NewTextExtractor() *TextExtractor {
	return &TextExtractor{
		preserveSpacing:  true,
		includeInvisible: false,
	}
}

// SetPreserveSpacing sets whether to preserve spacing in extracted text.
func (e *TextExtractor) SetPreserveSpacing(preserve bool) {
	e.preserveSpacing = preserve
}

// SetIncludeInvisible sets whether to include invisible text.
func (e *TextExtractor) SetIncludeInvisible(include bool) {
	e.includeInvisible = include
}
