package pdf

import (
	"fmt"
	"sort"
	"strings"
)

// GetAnnotations is a Java-parity alias of page annotation lookup.
// It accepts a 1-based page number.
func (d *Document) GetAnnotations(pageNumber int) ([]*Annotation, error) {
	if !d.IsValidPage(pageNumber) {
		return nil, fmt.Errorf("invalid page number: %d", pageNumber)
	}
	return d.GetPageAnnotations(pageNumber - 1)
}

// IsOpened reports whether document handle is available.
func (d *Document) IsOpened() bool {
	return d != nil && d.doc != nil
}

// IsReadyForClose reports whether document can be closed now.
func (d *Document) IsReadyForClose() bool {
	return true
}

// IsClosedOrReadyForClose reports whether close can proceed.
func (d *Document) IsClosedOrReadyForClose() bool {
	return !d.IsOpened() || d.IsReadyForClose()
}

// IsNowRendering reports whether rendering is currently in progress.
func (d *Document) IsNowRendering() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nowRenderingCount > 0
}

// IsNowRenderingForThumbnail reports whether thumbnail rendering is in progress.
func (d *Document) IsNowRenderingForThumbnail() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nowThumbnailRenderCount > 0
}

// IsNowSaveProcessing reports whether save is currently in progress.
func (d *Document) IsNowSaveProcessing() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nowSaveProcessing
}

// IsNowSigning reports whether signing is currently in progress.
func (d *Document) IsNowSigning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nowSigning
}

// RemovePageSL removes one page by 1-based page number.
func (d *Document) RemovePageSL(pageNumber int, _ bool) bool {
	if !d.IsValidPage(pageNumber) {
		return false
	}
	return d.RemovePage(pageNumber-1) == nil
}

// RemovePagesSL removes multiple pages by 1-based page numbers.
// It returns removed page numbers in the same order as requested input.
func (d *Document) RemovePagesSL(pageNumbers []int, _ bool) []int {
	if len(pageNumbers) == 0 {
		return nil
	}

	unique := make(map[int]struct{}, len(pageNumbers))
	toRemoveDesc := make([]int, 0, len(pageNumbers))
	for _, pageNumber := range pageNumbers {
		if !d.IsValidPage(pageNumber) {
			continue
		}
		if _, exists := unique[pageNumber]; exists {
			continue
		}
		unique[pageNumber] = struct{}{}
		toRemoveDesc = append(toRemoveDesc, pageNumber)
	}
	if len(toRemoveDesc) == 0 {
		return nil
	}

	sort.Slice(toRemoveDesc, func(i, j int) bool {
		return toRemoveDesc[i] > toRemoveDesc[j]
	})

	removedSet := make(map[int]struct{}, len(toRemoveDesc))
	for _, pageNumber := range toRemoveDesc {
		if d.RemovePage(pageNumber-1) == nil {
			removedSet[pageNumber] = struct{}{}
		}
	}

	removed := make([]int, 0, len(removedSet))
	seen := make(map[int]struct{}, len(removedSet))
	for _, pageNumber := range pageNumbers {
		if _, ok := removedSet[pageNumber]; !ok {
			continue
		}
		if _, ok := seen[pageNumber]; ok {
			continue
		}
		seen[pageNumber] = struct{}{}
		removed = append(removed, pageNumber)
	}

	if len(removed) == 0 {
		return nil
	}
	return removed
}

// Save stores document to the original file path.
func (d *Document) Save() bool {
	path := d.GetFilePath()
	if strings.TrimSpace(path) == "" {
		return false
	}
	return d.SaveAs(path)
}

// SaveAs stores document to the given path.
func (d *Document) SaveAs(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	d.mu.Lock()
	d.nowSaveProcessing = true
	d.mu.Unlock()

	err := d.SaveWithNativeSessionUpdates(path)

	d.mu.Lock()
	d.nowSaveProcessing = false
	if err == nil {
		d.filePath = path
		d.savedAfterOpen = true
	}
	d.mu.Unlock()

	return err == nil
}
