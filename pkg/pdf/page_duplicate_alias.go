package pdf

// AddDuplicatePageAfter duplicates one page after target page number.
// Page numbers are 1-based.
func (d *Document) AddDuplicatePageAfter(targetPageNumber, sourcePageNumber int, _ bool) bool {
	if !d.IsValidPage(targetPageNumber) || !d.IsValidPage(sourcePageNumber) {
		return false
	}

	sourceIndex := sourcePageNumber - 1
	insertIndex := targetPageNumber
	return d.InsertPage(insertIndex, sourceIndex) == nil
}

// AddDuplicatePageBefore duplicates one page before target page number.
// Page numbers are 1-based.
func (d *Document) AddDuplicatePageBefore(targetPageNumber, sourcePageNumber int, _ bool) bool {
	if !d.IsValidPage(targetPageNumber) || !d.IsValidPage(sourcePageNumber) {
		return false
	}

	sourceIndex := sourcePageNumber - 1
	insertIndex := targetPageNumber - 1
	return d.InsertPage(insertIndex, sourceIndex) == nil
}

// AddDuplicatePages duplicates multiple pages after target page number.
// It returns inserted page numbers in insertion order (1-based).
func (d *Document) AddDuplicatePages(sourcePageNumbers []int, targetPageNumber int, _ bool) []int {
	if len(sourcePageNumbers) == 0 || !d.IsValidPage(targetPageNumber) {
		return nil
	}

	baseOrder := d.PageOrder()
	if len(baseOrder) == 0 {
		return nil
	}

	insertIndex := targetPageNumber
	inserted := make([]int, 0, len(sourcePageNumbers))
	for _, pageNumber := range sourcePageNumbers {
		if pageNumber <= 0 || pageNumber > len(baseOrder) {
			continue
		}

		sourceRef := baseOrder[pageNumber-1]
		sourceIndex := d.findPageOrderIndexBySourceRef(sourceRef)
		if sourceIndex < 0 {
			continue
		}

		if err := d.InsertPage(insertIndex, sourceIndex); err != nil {
			continue
		}
		inserted = append(inserted, insertIndex+1)
		insertIndex++
	}

	if len(inserted) == 0 {
		return nil
	}
	return inserted
}

func (d *Document) findPageOrderIndexBySourceRef(sourceRef int) int {
	order := d.PageOrder()
	for i, value := range order {
		if value == sourceRef {
			return i
		}
	}
	return -1
}
