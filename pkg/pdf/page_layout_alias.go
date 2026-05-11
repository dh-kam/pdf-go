package pdf

import (
	"fmt"
	"math"
)

const (
	defaultPageBackgroundColor        = 0xFFFFFFFF
	defaultPageSubtitleHighlightColor = 0xFFFFFF00
)

// GetPageBackgroundColor returns default page background color in ARGB.
func (d *Document) GetPageBackgroundColor() int {
	return defaultPageBackgroundColor
}

// GetPageSubtitleHighlightColor returns subtitle highlight color in ARGB.
func (d *Document) GetPageSubtitleHighlightColor() int {
	return defaultPageSubtitleHighlightColor
}

// SetPageMediaBoxSL sets session media box override for one page.
func (d *Document) SetPageMediaBoxSL(pageIndex int, mediaBox [4]float64) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}
	if !validBox(mediaBox) {
		return fmt.Errorf("invalid media box: %v", mediaBox)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.pageMediaBoxes[sourceIndex] = mediaBox
	return nil
}

// SetPageCropBoxSL sets session crop box override for one page.
func (d *Document) SetPageCropBoxSL(pageIndex int, cropBox [4]float64) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}
	if !validBox(cropBox) {
		return fmt.Errorf("invalid crop box: %v", cropBox)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.pageCropBoxes[sourceIndex] = cropBox
	return nil
}

// SetPageRotate sets session page rotation override.
func (d *Document) SetPageRotate(pageIndex int, rotate int) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}

	normalized := normalizeRotation(rotate)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pageRotations[sourceIndex] = normalized
	return nil
}

// GetTextColumnRotationSL returns rotation for one text column.
// Session-level parity maps this to page rotation.
func (d *Document) GetTextColumnRotationSL(pageIndex, flowIndex, columnIndex int) (int, error) {
	return d.GetPageRotate(pageIndex)
}

// GetSelection returns selected option indexes for one choice field.
func (d *Document) GetSelection(fieldName string) ([]int, error) {
	field, err := d.fieldByName(fieldName)
	if err != nil {
		return nil, err
	}
	if field.Type != "Ch" {
		return nil, fmt.Errorf("field is not choice type: %s", field.Name)
	}

	values := formValueToStrings(field.Value)
	if len(values) == 0 {
		return []int{}, nil
	}

	indexes := make([]int, 0, len(values))
	for _, value := range values {
		matched := -1
		for i, option := range field.Options {
			if option == value {
				matched = i
				break
			}
		}
		if matched >= 0 {
			indexes = append(indexes, matched)
		}
	}
	return indexes, nil
}

// GetHittedTextBlockBounds returns hit-test bounds for text block query.
// Session-level parity returns intersection with page bounds.
func (d *Document) GetHittedTextBlockBounds(pageIndex int, queryRect [4]float64) ([4]float64, error) {
	return d.hitBoundsInPage(pageIndex, queryRect)
}

// GetHittedTextColumnBounds returns hit-test bounds for text column query.
// Session-level parity returns intersection with page bounds.
func (d *Document) GetHittedTextColumnBounds(pageIndex int, queryRect [4]float64) ([4]float64, error) {
	return d.hitBoundsInPage(pageIndex, queryRect)
}

// LookupStyledTextInColumn returns styled text XML for one page.
func (d *Document) LookupStyledTextInColumn(pageIndex int) (string, error) {
	return d.GetPageTextAsXMLSL(pageIndex)
}

func (d *Document) hitBoundsInPage(pageIndex int, queryRect [4]float64) ([4]float64, error) {
	if !validBox(queryRect) {
		return [4]float64{}, fmt.Errorf("invalid query rect: %v", queryRect)
	}

	page, err := d.Page(pageIndex)
	if err != nil {
		return [4]float64{}, err
	}

	pageBox := page.MediaBox()
	intersect := [4]float64{
		math.Max(pageBox[0], queryRect[0]),
		math.Max(pageBox[1], queryRect[1]),
		math.Min(pageBox[2], queryRect[2]),
		math.Min(pageBox[3], queryRect[3]),
	}
	if !validBox(intersect) {
		return [4]float64{}, nil
	}
	return intersect, nil
}

func validBox(box [4]float64) bool {
	return box[2] >= box[0] && box[3] >= box[1]
}

func normalizeRotation(rotation int) int {
	normalized := rotation % 360
	if normalized < 0 {
		normalized += 360
	}
	steps := int(math.Round(float64(normalized) / 90.0))
	return (steps * 90) % 360
}

func (d *Document) pageMediaBoxOverride(sourceIndex int) ([4]float64, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	box, ok := d.pageMediaBoxes[sourceIndex]
	return box, ok
}

func (d *Document) pageCropBoxOverride(sourceIndex int) ([4]float64, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	box, ok := d.pageCropBoxes[sourceIndex]
	return box, ok
}

func (d *Document) pageRotationOverride(sourceIndex int) (int, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rotation, ok := d.pageRotations[sourceIndex]
	return rotation, ok
}
