package pdf

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	domaintext "github.com/dh-kam/pdf-go/internal/domain/text"
	infrastructuretext "github.com/dh-kam/pdf-go/internal/infrastructure/text"
)

const (
	defaultLineTolerance = 3.0
)

// TextItem represents extracted text with basic geometry.
type TextItem struct {
	Text     string
	Font     string
	X        float64
	Y        float64
	Width    float64
	Height   float64
	FontSize float64
}

// TextLine represents one semantic line.
type TextLine struct {
	Text  string
	Items []TextItem
	Y     float64
}

// TextParagraph represents a paragraph grouped from lines.
type TextParagraph struct {
	Text  string
	Lines []TextLine
}

type textLineBuilder struct {
	items []TextItem
	y     float64
}

// Text extracts plain text from a page (0-based index).
func (d *Document) Text(pageIndex int) (string, error) {
	page, err := d.doc.GetPage(pageIndex)
	if err != nil {
		return "", fmt.Errorf("get page %d: %w", pageIndex, err)
	}

	extractor := infrastructuretext.NewExtractor()
	extractor.SetPreserveSpacing(true)

	text, err := extractor.ExtractToText(page)
	if err != nil {
		return "", fmt.Errorf("extract text: %w", err)
	}
	return text, nil
}

// TextRange extracts a rune-based text range from a page.
// start is inclusive and end is exclusive.
func (d *Document) TextRange(pageIndex, start, end int) (string, error) {
	if start < 0 || end < 0 || start > end {
		return "", fmt.Errorf("invalid range: [%d,%d)", start, end)
	}

	text, err := d.Text(pageIndex)
	if err != nil {
		return "", err
	}

	if start == end {
		return "", nil
	}

	runes := []rune(text)
	if start >= len(runes) {
		return "", nil
	}
	if end > len(runes) {
		end = len(runes)
	}

	return string(runes[start:end]), nil
}

// TextLines extracts semantic lines from a page using text item geometry.
func (d *Document) TextLines(pageIndex int) ([]TextLine, error) {
	layer, err := d.extractTextLayer(pageIndex)
	if err != nil {
		return nil, err
	}

	items := toSortedTextItems(layer.GetItems())
	if len(items) == 0 {
		return []TextLine{}, nil
	}

	builders := make([]*textLineBuilder, 0)
	for _, item := range items {
		if item.Text == "" {
			continue
		}

		bestIdx := -1
		bestDelta := defaultLineTolerance + 1
		for i, line := range builders {
			delta := abs(item.Y - line.y)
			if delta <= defaultLineTolerance && delta < bestDelta {
				bestIdx = i
				bestDelta = delta
			}
		}

		if bestIdx == -1 {
			builders = append(builders, &textLineBuilder{
				y:     item.Y,
				items: []TextItem{item},
			})
			continue
		}

		line := builders[bestIdx]
		line.items = append(line.items, item)
		line.y = (line.y*float64(len(line.items)-1) + item.Y) / float64(len(line.items))
	}

	lines := make([]TextLine, 0, len(builders))
	for _, builder := range builders {
		sort.Slice(builder.items, func(i, j int) bool {
			if builder.items[i].X == builder.items[j].X {
				return builder.items[i].Text < builder.items[j].Text
			}
			return builder.items[i].X < builder.items[j].X
		})

		lines = append(lines, TextLine{
			Y:     builder.y,
			Items: append([]TextItem(nil), builder.items...),
			Text:  joinLineText(builder.items),
		})
	}

	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Y == lines[j].Y {
			return lines[i].Text < lines[j].Text
		}
		return lines[i].Y > lines[j].Y
	})

	return lines, nil
}

// TextParagraphs extracts semantic paragraphs by grouping nearby lines.
func (d *Document) TextParagraphs(pageIndex int) ([]TextParagraph, error) {
	lines, err := d.TextLines(pageIndex)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return []TextParagraph{}, nil
	}

	paragraphs := make([]TextParagraph, 0)
	current := TextParagraph{
		Lines: []TextLine{lines[0]},
	}

	for i := 1; i < len(lines); i++ {
		prev := lines[i-1]
		curr := lines[i]

		prevHeight := maxLineHeight(prev)
		if prevHeight <= 0 {
			prevHeight = 12
		}
		gap := prev.Y - curr.Y
		if gap > prevHeight*1.6 {
			current.Text = joinParagraphText(current.Lines)
			paragraphs = append(paragraphs, current)
			current = TextParagraph{Lines: []TextLine{curr}}
			continue
		}

		current.Lines = append(current.Lines, curr)
	}

	current.Text = joinParagraphText(current.Lines)
	paragraphs = append(paragraphs, current)
	return paragraphs, nil
}

func (d *Document) extractTextLayer(pageIndex int) (*domaintext.TextLayer, error) {
	page, err := d.doc.GetPage(pageIndex)
	if err != nil {
		return nil, fmt.Errorf("get page %d: %w", pageIndex, err)
	}

	extractor := infrastructuretext.NewExtractor()
	extractor.SetPreserveSpacing(true)
	layer, err := extractor.Extract(page)
	if err != nil {
		return nil, fmt.Errorf("extract page text layer: %w", err)
	}
	return layer, nil
}

func toSortedTextItems(items []domaintext.TextItem) []TextItem {
	out := make([]TextItem, 0, len(items))
	for _, item := range items {
		text := normalizeWhitespace(item.Text)
		if text == "" {
			continue
		}
		out = append(out, TextItem{
			Text:     text,
			X:        float64(item.BoundingBox.Min.X),
			Y:        float64(item.BoundingBox.Min.Y),
			Width:    float64(item.BoundingBox.Max.X - item.BoundingBox.Min.X),
			Height:   float64(item.BoundingBox.Max.Y - item.BoundingBox.Min.Y),
			Font:     item.Font,
			FontSize: item.FontSize,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Y == out[j].Y {
			return out[i].X < out[j].X
		}
		return out[i].Y > out[j].Y
	})
	return out
}

func joinLineText(items []TextItem) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	prevRight := items[0].X + items[0].Width
	sb.WriteString(items[0].Text)

	for i := 1; i < len(items); i++ {
		item := items[i]
		gap := item.X - prevRight
		if gap > 1.5 && !endsWithSpace(sb.String()) {
			sb.WriteByte(' ')
		}
		sb.WriteString(item.Text)
		prevRight = item.X + item.Width
	}

	return strings.TrimSpace(normalizeWhitespace(sb.String()))
}

func joinParagraphText(lines []TextLine) string {
	if len(lines) == 0 {
		return ""
	}
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Text == "" {
			continue
		}
		parts = append(parts, line.Text)
	}
	return strings.Join(parts, "\n")
}

func maxLineHeight(line TextLine) float64 {
	maxH := 0.0
	for _, item := range line.Items {
		if item.Height > maxH {
			maxH = item.Height
		}
	}
	return maxH
}

func normalizeWhitespace(text string) string {
	if text == "" {
		return ""
	}
	fields := strings.Fields(text)
	return strings.Join(fields, " ")
}

func endsWithSpace(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text)
	return r == ' '
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
