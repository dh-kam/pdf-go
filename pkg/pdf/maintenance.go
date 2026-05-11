package pdf

import (
	"fmt"
	"os"
	"strings"
)

// CompactAs writes a compacted document copy to path.
// It prefers native session save and falls back to raw byte copy when possible.
func (d *Document) CompactAs(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is empty")
	}

	if err := d.SaveWithNativeSessionUpdates(path); err == nil {
		return nil
	}

	raw, err := d.rawPDFData()
	if err != nil {
		return fmt.Errorf("compact save failed: %w", err)
	}
	if writeErr := os.WriteFile(path, raw, 0o644); writeErr != nil {
		return fmt.Errorf("write compacted pdf: %w", writeErr)
	}
	return nil
}

// ExportPages exports a page range to a new PDF file.
// startPage/endPage are 0-based and inclusive.
func (d *Document) ExportPages(path string, startPage, endPage int, includeAnnotations, includeForms bool) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is empty")
	}

	pageCount, err := d.PageCount()
	if err != nil {
		return err
	}
	if startPage < 0 || endPage < 0 || startPage > endPage || endPage >= pageCount {
		return fmt.Errorf("invalid page range: [%d,%d]", startPage, endPage)
	}

	selected := make([]int, 0, endPage-startPage+1)
	for pageIndex := startPage; pageIndex <= endPage; pageIndex++ {
		sourceIndex, resolveErr := d.resolveSourcePageIndex(pageIndex)
		if resolveErr != nil {
			return resolveErr
		}
		selected = append(selected, sourceIndex)
	}

	baseSnapshots := make(map[int][]annotationSnapshot, len(selected))
	if !includeAnnotations || !includeForms {
		for _, sourceIndex := range selected {
			snapshots, loadErr := d.loadPageAnnotationSnapshots(sourceIndex)
			if loadErr != nil {
				return loadErr
			}
			baseSnapshots[sourceIndex] = snapshots
		}
	}

	oldPageOrder := d.PageOrder()
	oldAnnots := d.snapshotAnnotationOverrides()
	oldFormValues := d.snapshotFormValueOverrides()
	oldFormOptions := d.snapshotFormOptionOverrides()

	restore := func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.pageOrder = append([]int(nil), oldPageOrder...)
		d.annotationOverrides = oldAnnots
		d.formValues = oldFormValues
		d.formOptions = oldFormOptions
	}
	defer restore()

	d.mu.Lock()
	d.pageOrder = append([]int(nil), selected...)
	for _, sourceIndex := range selected {
		current, ok := d.annotationOverrides[sourceIndex]
		if !ok {
			current = baseSnapshots[sourceIndex]
		}

		next := cloneAnnotationSnapshots(current)
		if !includeForms {
			filtered, _ := filterOutWidgetAnnotations(next)
			next = filtered
		}
		if !includeAnnotations {
			next = []annotationSnapshot{}
		}

		d.annotationOverrides[sourceIndex] = cloneAnnotationSnapshots(next)
	}
	if !includeForms {
		d.formValues = map[string][]string{}
		d.formOptions = map[string][]string{}
	}
	d.mu.Unlock()

	return d.SaveWithNativeSessionUpdates(path)
}

func (d *Document) rawPDFData() ([]byte, error) {
	if d == nil || d.doc == nil || d.doc.XRef() == nil {
		return nil, fmt.Errorf("pdf stream is unavailable")
	}

	provider, ok := d.doc.XRef().(interface {
		RawData() []byte
	})
	if !ok {
		return nil, fmt.Errorf("raw pdf data provider is unavailable")
	}

	raw := provider.RawData()
	if len(raw) == 0 {
		return nil, fmt.Errorf("raw pdf data is empty")
	}

	out := make([]byte, len(raw))
	copy(out, raw)
	return out, nil
}
