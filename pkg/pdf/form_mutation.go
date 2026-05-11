package pdf

import "fmt"

// ImportFormDataXFDF parses XFDF input and applies values to matching form fields.
// It returns the number of fields updated in the current session.
func (d *Document) ImportFormDataXFDF(data []byte) (int, error) {
	parsed, err := ParseFormDataXFDF(data)
	if err != nil {
		return 0, err
	}

	return d.ApplyFormData(parsed)
}

// ImportFormDataFDF parses FDF input and applies values to matching form fields.
// It returns the number of fields updated in the current session.
func (d *Document) ImportFormDataFDF(data []byte) (int, error) {
	parsed, err := ParseFormDataFDF(data)
	if err != nil {
		return 0, err
	}

	return d.ApplyFormData(parsed)
}

// ApplyFormData applies form values to matching form fields in the current session.
// Unknown field names are ignored.
func (d *Document) ApplyFormData(data *FormData) (int, error) {
	if data == nil {
		return 0, fmt.Errorf("form data is nil")
	}

	fieldNames, err := d.formFieldNameSet()
	if err != nil {
		return 0, err
	}
	if len(fieldNames) == 0 || len(data.Fields) == 0 {
		return 0, nil
	}

	applied := 0

	d.mu.Lock()
	defer d.mu.Unlock()

	for name, values := range data.Fields {
		if _, ok := fieldNames[name]; !ok {
			continue
		}
		d.formValues[name] = normalizeFormFieldValues(values)
		applied++
	}

	return applied, nil
}

// SetFormFieldValue sets one string value for a form field in the current session.
func (d *Document) SetFormFieldValue(name, value string) error {
	return d.SetFormFieldValues(name, []string{value})
}

// SetFormFieldValues sets one or more values for a form field in the current session.
func (d *Document) SetFormFieldValues(name string, values []string) error {
	if name == "" {
		return fmt.Errorf("form field name is required")
	}

	fieldNames, err := d.formFieldNameSet()
	if err != nil {
		return err
	}
	if _, ok := fieldNames[name]; !ok {
		return fmt.Errorf("form field not found: %s", name)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.formValues[name] = normalizeFormFieldValues(values)
	return nil
}

// ClearFormFieldValue removes a session override for one form field.
func (d *Document) ClearFormFieldValue(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.formValues, name)
}

// ClearFormDataOverrides removes all session form value overrides.
func (d *Document) ClearFormDataOverrides() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.formValues = make(map[string][]string)
}

// SetChoiceFieldItems overrides choice options for one field in the current session.
func (d *Document) SetChoiceFieldItems(name string, options []string) error {
	field, err := d.choiceFieldByName(name)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.formOptions[field.Name] = normalizeChoiceFieldOptions(options)
	return nil
}

// AddChoiceFieldItem appends one choice option in the current session.
func (d *Document) AddChoiceFieldItem(name, option string) error {
	field, err := d.choiceFieldByName(name)
	if err != nil {
		return err
	}

	merged := append([]string(nil), field.Options...)
	merged = append(merged, option)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.formOptions[field.Name] = normalizeChoiceFieldOptions(merged)
	return nil
}

// RemoveChoiceFieldItem removes one choice option by index in the current session.
func (d *Document) RemoveChoiceFieldItem(name string, index int) error {
	field, err := d.choiceFieldByName(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(field.Options) {
		return fmt.Errorf("choice field index out of range: %d", index)
	}

	updated := make([]string, 0, len(field.Options)-1)
	updated = append(updated, field.Options[:index]...)
	updated = append(updated, field.Options[index+1:]...)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.formOptions[field.Name] = normalizeChoiceFieldOptions(updated)
	return nil
}

// ClearChoiceFieldItems clears all choice options in the current session.
func (d *Document) ClearChoiceFieldItems(name string) error {
	field, err := d.choiceFieldByName(name)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.formOptions[field.Name] = []string{}
	return nil
}

// ClearChoiceFieldOverrides removes all session choice option overrides.
func (d *Document) ClearChoiceFieldOverrides() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.formOptions = make(map[string][]string)
}

func (d *Document) formFieldNameSet() (map[string]struct{}, error) {
	fields, err := d.FormFields()
	if err != nil {
		return nil, fmt.Errorf("load form fields: %w", err)
	}

	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field == nil || field.Name == "" {
			continue
		}
		set[field.Name] = struct{}{}
	}
	return set, nil
}

func (d *Document) choiceFieldByName(name string) (*FormField, error) {
	if name == "" {
		return nil, fmt.Errorf("form field name is required")
	}

	fields, err := d.FormFields()
	if err != nil {
		return nil, fmt.Errorf("load form fields: %w", err)
	}
	for _, field := range fields {
		if field == nil || field.Name != name {
			continue
		}
		if field.Type != "Ch" {
			return nil, fmt.Errorf("form field is not choice type: %s", name)
		}
		return field, nil
	}
	return nil, fmt.Errorf("form field not found: %s", name)
}

func normalizeFormFieldValues(values []string) []string {
	if len(values) == 0 {
		return []string{""}
	}

	out := make([]string, len(values))
	copy(out, values)
	return out
}

func normalizeChoiceFieldOptions(options []string) []string {
	if len(options) == 0 {
		return []string{}
	}

	out := make([]string, len(options))
	copy(out, options)
	return out
}
