package pdf

// GetPage is a Java-parity alias of Page.
func (d *Document) GetPage(pageIndex int) (*Page, error) {
	return d.Page(pageIndex)
}

// GetPageAnnotations returns page annotations for the given page index.
func (d *Document) GetPageAnnotations(pageIndex int) ([]*Annotation, error) {
	page, err := d.Page(pageIndex)
	if err != nil {
		return nil, err
	}
	return page.Annotations()
}

// GetTextInRange is a Java-parity alias of TextRange.
func (d *Document) GetTextInRange(pageIndex, start, end int) (string, error) {
	return d.TextRange(pageIndex, start, end)
}

// GetTextLineCount returns text line count for one page.
func (d *Document) GetTextLineCount(pageIndex int) (int, error) {
	lines, err := d.TextLines(pageIndex)
	if err != nil {
		return 0, err
	}
	return len(lines), nil
}

// PageToLabel resolves a 1-based page number to page label.
// It returns empty string when no page labels exist or the page has no label.
func (d *Document) PageToLabel(pageNumber int) string {
	if pageNumber <= 0 {
		return ""
	}

	lookup, ok := d.pageLabelLookup()
	if !ok {
		return ""
	}
	for label, page := range lookup {
		if page == pageNumber {
			return label
		}
	}
	return ""
}

// IsValidPage checks whether the given 1-based page number is valid.
func (d *Document) IsValidPage(pageNumber int) bool {
	pageCount, err := d.PageCount()
	if err != nil {
		return false
	}
	return pageNumber > 0 && pageNumber <= pageCount
}

// HasNextPage reports whether the given 1-based page number has a next page.
func (d *Document) HasNextPage(pageNumber int) bool {
	return d.IsValidPage(pageNumber + 1)
}

// HasPrevPage reports whether the given 1-based page number has a previous page.
func (d *Document) HasPrevPage(pageNumber int) bool {
	return d.IsValidPage(pageNumber - 1)
}

// Length is a Java-parity alias of FileSize.
func (d *Document) Length() int64 {
	return d.FileSize()
}

// Accept executes a Java-style visitor callback with this document.
func (d *Document) Accept(visitor func(*Document)) {
	if visitor == nil {
		return
	}
	visitor(d)
}

// Run executes a Java-style runnable callback.
func (d *Document) Run(runnable func()) {
	if runnable == nil {
		return
	}
	runnable()
}
