package pdf

import (
	"fmt"
	"strings"
)

// FieldChSetItems replaces all choice items for one field index.
func (d *Document) FieldChSetItems(fieldIndex int, exportValues, displayValues []string) error {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return err
	}
	if field.Type != "Ch" {
		return fmt.Errorf("field is not choice type: %s", field.Name)
	}

	options := selectChoiceOptions(exportValues, displayValues)
	return d.SetChoiceFieldItems(field.Name, options)
}

// FieldChAddItem adds one choice item for one field index.
func (d *Document) FieldChAddItem(fieldIndex int, exportValue, displayValue string, insertIndex int) error {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return err
	}
	if field.Type != "Ch" {
		return fmt.Errorf("field is not choice type: %s", field.Name)
	}

	option := strings.TrimSpace(displayValue)
	if option == "" {
		option = exportValue
	}

	current := append([]string(nil), field.Options...)
	if insertIndex < 0 || insertIndex >= len(current) {
		current = append(current, option)
	} else {
		current = append(current, "")
		copy(current[insertIndex+1:], current[insertIndex:])
		current[insertIndex] = option
	}
	return d.SetChoiceFieldItems(field.Name, current)
}

// FieldChRemoveItem removes one choice item for one field index.
func (d *Document) FieldChRemoveItem(fieldIndex, removeIndex int) error {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return err
	}
	if field.Type != "Ch" {
		return fmt.Errorf("field is not choice type: %s", field.Name)
	}
	return d.RemoveChoiceFieldItem(field.Name, removeIndex)
}

// FieldChClearItems clears all choice items for one field index.
func (d *Document) FieldChClearItems(fieldIndex int) error {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return err
	}
	if field.Type != "Ch" {
		return fmt.Errorf("field is not choice type: %s", field.Name)
	}
	return d.ClearChoiceFieldItems(field.Name)
}

// FieldChSetSelection sets one or more selected indexes for one choice field index.
func (d *Document) FieldChSetSelection(fieldIndex int, selectedIndexes []int) error {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return err
	}
	if field.Type != "Ch" {
		return fmt.Errorf("field is not choice type: %s", field.Name)
	}
	if len(selectedIndexes) == 0 {
		return d.SetFormFieldValues(field.Name, []string{""})
	}

	values := make([]string, 0, len(selectedIndexes))
	for _, selected := range selectedIndexes {
		if selected < 0 || selected >= len(field.Options) {
			return fmt.Errorf("selected index out of range: %d", selected)
		}
		values = append(values, field.Options[selected])
	}
	return d.SetFormFieldValues(field.Name, values)
}

// FieldChSetCurSel sets one selected index for one choice field index and returns selected index.
func (d *Document) FieldChSetCurSel(fieldIndex, selectedIndex int) (int, error) {
	if err := d.FieldChSetSelection(fieldIndex, []int{selectedIndex}); err != nil {
		return -1, err
	}
	return selectedIndex, nil
}

// GetFieldAnnotationCount returns widget annotation count for one field name.
func (d *Document) GetFieldAnnotationCount(fieldName string) (int, error) {
	annots, err := d.fieldWidgetAnnotations(fieldName)
	if err != nil {
		return 0, err
	}
	return len(annots), nil
}

// GetFieldAnnotation returns one widget annotation by field name and index.
func (d *Document) GetFieldAnnotation(fieldName string, annotationIndex int) (*Annotation, error) {
	annots, err := d.fieldWidgetAnnotations(fieldName)
	if err != nil {
		return nil, err
	}
	if annotationIndex < 0 || annotationIndex >= len(annots) {
		return nil, fmt.Errorf("annotation index out of range: %d", annotationIndex)
	}
	return annots[annotationIndex], nil
}

// GetFieldTip returns field tip text.
// Session-level parity returns full field name.
func (d *Document) GetFieldTip(fieldName string) (string, error) {
	field, err := d.fieldByName(fieldName)
	if err != nil {
		return "", err
	}
	return field.Name, nil
}

// GetSingleAnnotButtonFieldExportValue returns one value for one button-like field.
func (d *Document) GetSingleAnnotButtonFieldExportValue(fieldName string) (string, error) {
	field, err := d.fieldByName(fieldName)
	if err != nil {
		return "", err
	}
	values := formValueToStrings(field.Value)
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

// GetUnduplicatedNewFieldTitle returns a non-conflicting field title candidate.
func (d *Document) GetUnduplicatedNewFieldTitle(base string) string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		trimmed = "Field"
	}

	fields, err := d.FormFields()
	if err != nil {
		return trimmed
	}

	exists := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field == nil || field.Name == "" {
			continue
		}
		exists[field.Name] = struct{}{}
	}

	if _, ok := exists[trimmed]; !ok {
		return trimmed
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s%d", trimmed, i)
		if _, ok := exists[candidate]; !ok {
			return candidate
		}
	}
}

// SetFormFieldEmbedFontPath sets default embedded form-field font path.
// Session-level parity stores path string and returns true when path is non-empty.
func (d *Document) SetFormFieldEmbedFontPath(path string) bool {
	return strings.TrimSpace(path) != ""
}

// UpdateFieldValue updates one field value by field name.
func (d *Document) UpdateFieldValue(fieldName, value string) error {
	return d.SetFormFieldValue(fieldName, value)
}

// UpdateFieldFormattedValue updates one field formatted value by field name.
func (d *Document) UpdateFieldFormattedValue(fieldName, value string) error {
	return d.SetFormFieldValue(fieldName, value)
}

func selectChoiceOptions(exportValues, displayValues []string) []string {
	if len(displayValues) > 0 {
		return displayValues
	}
	return exportValues
}

func (d *Document) fieldByName(fieldName string) (*FormField, error) {
	name := strings.TrimSpace(fieldName)
	if name == "" {
		return nil, fmt.Errorf("field name is empty")
	}

	fields, err := d.FormFields()
	if err != nil {
		return nil, err
	}
	for _, field := range fields {
		if field != nil && field.Name == name {
			return field, nil
		}
	}
	return nil, fmt.Errorf("field not found: %s", name)
}

func (d *Document) fieldWidgetAnnotations(fieldName string) ([]*Annotation, error) {
	field, err := d.fieldByName(fieldName)
	if err != nil {
		return nil, err
	}
	if field.PageIndex >= 0 {
		page, pageErr := d.Page(field.PageIndex)
		if pageErr != nil {
			return nil, pageErr
		}
		return collectWidgetAnnotations(page)
	}

	pageCount, err := d.PageCount()
	if err != nil {
		return nil, err
	}

	out := make([]*Annotation, 0)
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		page, pageErr := d.Page(pageIndex)
		if pageErr != nil {
			return nil, pageErr
		}
		widgets, collectErr := collectWidgetAnnotations(page)
		if collectErr != nil {
			return nil, collectErr
		}
		out = append(out, widgets...)
	}
	return out, nil
}

func collectWidgetAnnotations(page *Page) ([]*Annotation, error) {
	annots, err := page.Annotations()
	if err != nil {
		return nil, err
	}

	out := make([]*Annotation, 0, len(annots))
	for _, annot := range annots {
		if annot != nil && strings.EqualFold(annot.Type(), "Widget") {
			out = append(out, annot)
		}
	}
	return out, nil
}
