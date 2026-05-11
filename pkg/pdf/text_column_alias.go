package pdf

import "fmt"

// TextColumn is a lightweight Java-parity text-column descriptor.
type TextColumn struct {
	Page        int
	FlowIndex   int
	ColumnIndex int
	StyledText  string
}

// GetTextColumnListSL returns text columns for a page in Java-parity shape.
// It accepts optional 1-based page number and falls back to current page.
func (d *Document) GetTextColumnListSL(pageNumbers ...int) []TextColumn {
	pageNumber := d.currentPageNumber()
	if len(pageNumbers) > 0 {
		pageNumber = pageNumbers[0]
	}
	if !d.IsValidPage(pageNumber) {
		return nil
	}

	styledText, err := d.LookupStyledTextInColumn(pageNumber - 1)
	if err != nil {
		styledText = ""
	}

	return []TextColumn{
		{
			Page:        pageNumber,
			FlowIndex:   0,
			ColumnIndex: 0,
			StyledText:  styledText,
		},
	}
}

// GetTextParagraphList returns paragraph list for a 1-based page number.
func (d *Document) GetTextParagraphList(pageNumber int) ([]TextParagraph, error) {
	if !d.IsValidPage(pageNumber) {
		return nil, fmt.Errorf("invalid page number: %d", pageNumber)
	}
	return d.TextParagraphs(pageNumber - 1)
}
