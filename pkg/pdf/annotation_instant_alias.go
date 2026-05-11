package pdf

import "fmt"

// GetFieldAnnotations returns widget annotations for one field.
func (d *Document) GetFieldAnnotations(fieldName string) ([]*Annotation, error) {
	return d.fieldWidgetAnnotations(fieldName)
}

// GetFieldAnnotationPages returns 1-based page numbers containing field widgets.
func (d *Document) GetFieldAnnotationPages(fieldName string) ([]int, error) {
	field, err := d.fieldByName(fieldName)
	if err != nil {
		return nil, err
	}
	if field.PageIndex >= 0 {
		return []int{field.PageIndex + 1}, nil
	}

	pageCount, err := d.PageCount()
	if err != nil {
		return nil, err
	}
	out := make([]int, 0)
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		page, pageErr := d.Page(pageIndex)
		if pageErr != nil {
			return nil, pageErr
		}
		widgets, collectErr := collectWidgetAnnotations(page)
		if collectErr != nil {
			return nil, collectErr
		}
		if len(widgets) > 0 {
			out = append(out, pageIndex+1)
		}
	}
	return out, nil
}

// InstantGetAnnotation returns one annotation by 1-based page number and annotation index.
func (d *Document) InstantGetAnnotation(pageNumber, annotationIndex int) (*Annotation, error) {
	if !d.IsValidPage(pageNumber) {
		return nil, fmt.Errorf("invalid page number: %d", pageNumber)
	}
	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil {
		return nil, err
	}
	if annotationIndex < 0 || annotationIndex >= len(annots) {
		return nil, fmt.Errorf("annotation index out of range: %d", annotationIndex)
	}
	return annots[annotationIndex], nil
}

// InstantGetFieldAnnotation returns one field widget annotation by index.
func (d *Document) InstantGetFieldAnnotation(fieldName string, annotationIndex int) (*Annotation, error) {
	return d.GetFieldAnnotation(fieldName, annotationIndex)
}

// InstantGetFieldAnnotations returns field widget annotations.
func (d *Document) InstantGetFieldAnnotations(fieldName string) ([]*Annotation, error) {
	return d.GetFieldAnnotations(fieldName)
}
