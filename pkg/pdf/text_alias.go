package pdf

// GetPageTextSL is a Java-parity alias of Text.
func (d *Document) GetPageTextSL(pageIndex int) (string, error) {
	return d.Text(pageIndex)
}

// FindTextInPageSL is a Java-parity alias of SearchTextInPage with default search options.
func (d *Document) FindTextInPageSL(pageIndex int, query string) ([]TextSearchMatch, error) {
	return d.SearchTextInPage(pageIndex, query, TextSearchOptions{})
}

// FastFindTextInPage is a Java-parity alias of SearchTextInPage with default search options.
func (d *Document) FastFindTextInPage(pageIndex int, query string) ([]TextSearchMatch, error) {
	return d.SearchTextInPage(pageIndex, query, TextSearchOptions{})
}
