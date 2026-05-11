package pdf

import (
	"fmt"
	"strings"
)

// FlattenAnnotation performs a session-level flatten by removing the annotation entry.
// The rendered appearance stream is not composited into page content in this fallback mode.
func (d *Document) FlattenAnnotation(pageIndex, annotationIndex int) error {
	return d.RemovePageAnnotation(pageIndex, annotationIndex)
}

// FlattenAllFormField performs a session-level form flatten by removing widget annotations.
// keepValues is reserved for Java-parity call shape and currently does not change behavior.
func (d *Document) FlattenAllFormField(keepValues bool) error {
	_ = keepValues

	pageCount, err := d.PageCount()
	if err != nil {
		return err
	}

	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
		if err != nil {
			return err
		}

		base, err := d.loadPageAnnotationSnapshots(sourceIndex)
		if err != nil {
			return fmt.Errorf("load page annotations: %w", err)
		}

		d.mu.Lock()
		current, ok := d.annotationOverrides[sourceIndex]
		if !ok {
			current = base
		}
		filtered, removed := filterOutWidgetAnnotations(current)
		if removed {
			d.annotationOverrides[sourceIndex] = cloneAnnotationSnapshots(filtered)
		}
		d.mu.Unlock()
	}

	return nil
}

func filterOutWidgetAnnotations(input []annotationSnapshot) ([]annotationSnapshot, bool) {
	if len(input) == 0 {
		return nil, false
	}

	out := make([]annotationSnapshot, 0, len(input))
	removed := false
	for i := range input {
		typ := strings.TrimPrefix(strings.TrimSpace(input[i].Type), "/")
		if strings.EqualFold(typ, "Widget") {
			removed = true
			continue
		}
		out = append(out, input[i])
	}
	return out, removed
}
