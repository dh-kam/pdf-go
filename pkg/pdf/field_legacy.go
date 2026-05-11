package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldGetNumFields returns the number of flattened form fields.
func (d *Document) FieldGetNumFields() (int, error) {
	fields, err := d.FormFields()
	if err != nil {
		return 0, err
	}
	return len(fields), nil
}

// FieldGetTitle returns field full name by flattened field index.
func (d *Document) FieldGetTitle(fieldIndex int) (string, error) {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return "", err
	}
	return field.Name, nil
}

// FieldGetStringValue returns one string property/value from the field.
func (d *Document) FieldGetStringValue(fieldIndex int, key string) (string, error) {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return "", err
	}

	switch normalizeFieldKey(key) {
	case "", "V", "VALUE":
		values := formValueToStrings(field.Value)
		if len(values) == 0 {
			return "", nil
		}
		return values[0], nil
	case "DV", "DEFAULTVALUE":
		values := formValueToStrings(field.DefaultValue)
		if len(values) == 0 {
			return "", nil
		}
		return values[0], nil
	case "T", "TITLE", "NAME":
		return field.Name, nil
	case "PARTIALNAME":
		return field.PartialName, nil
	case "FT", "TYPE":
		return field.Type, nil
	case "FF", "FLAGS":
		return strconv.Itoa(field.Flags), nil
	case "OPT", "OPTIONS":
		return strings.Join(field.Options, ","), nil
	default:
		return "", nil
	}
}

// FieldGetNameValue returns one name-like property from the field.
func (d *Document) FieldGetNameValue(fieldIndex int, key string) (string, error) {
	return d.FieldGetStringValue(fieldIndex, key)
}

// FieldGetBooleanValue returns one boolean-like property from the field.
func (d *Document) FieldGetBooleanValue(fieldIndex int, key string, defaultValue bool) (bool, error) {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return false, err
	}

	switch normalizeFieldKey(key) {
	case "READONLY":
		return field.Flags&1 != 0, nil
	case "REQUIRED":
		return field.Flags&2 != 0, nil
	case "", "V", "VALUE":
		value, valueErr := d.FieldGetStringValue(fieldIndex, key)
		if valueErr != nil {
			return false, valueErr
		}
		parsed, parseErr := strconv.ParseBool(strings.TrimSpace(value))
		if parseErr != nil {
			return defaultValue, nil
		}
		return parsed, nil
	default:
		return defaultValue, nil
	}
}

// FieldGetRealValue returns one numeric-like property from the field.
func (d *Document) FieldGetRealValue(fieldIndex int, key string) (float64, error) {
	value, err := d.FieldGetStringValue(fieldIndex, key)
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, parseErr := strconv.ParseFloat(trimmed, 64)
	if parseErr != nil {
		return 0, nil
	}
	return parsed, nil
}

// FieldSetStringValue updates one string-like field property.
// It returns false when the key is unsupported.
func (d *Document) FieldSetStringValue(fieldIndex int, key, value string) (bool, error) {
	field, err := d.fieldByIndex(fieldIndex)
	if err != nil {
		return false, err
	}

	switch normalizeFieldKey(key) {
	case "", "V", "VALUE", "DV", "DEFAULTVALUE":
		if setErr := d.SetFormFieldValue(field.Name, value); setErr != nil {
			return false, setErr
		}
		return true, nil
	default:
		return false, nil
	}
}

// FieldSetBooleanValue updates one boolean-like field property.
// It returns false when the key is unsupported.
func (d *Document) FieldSetBooleanValue(fieldIndex int, key string, value bool) (bool, error) {
	stringValue := "false"
	if value {
		stringValue = "true"
	}
	return d.FieldSetStringValue(fieldIndex, key, stringValue)
}

// FieldSetValue updates field value by flattened field index.
// It returns 1 on success and 0 on failure.
func (d *Document) FieldSetValue(fieldIndex int, value string) (int, error) {
	ok, err := d.FieldSetStringValue(fieldIndex, "V", value)
	if err != nil {
		return 0, err
	}
	if ok {
		return 1, nil
	}
	return 0, nil
}

// FieldFindByRefNo returns field index by native field reference number.
// Session-level implementation does not expose native field reference numbers and returns -1.
func (d *Document) FieldFindByRefNo(refNo int) int {
	return -1
}

func (d *Document) fieldByIndex(fieldIndex int) (*FormField, error) {
	fields, err := d.FormFields()
	if err != nil {
		return nil, err
	}
	if fieldIndex < 0 || fieldIndex >= len(fields) {
		return nil, fmt.Errorf("field index out of range: %d", fieldIndex)
	}
	return fields[fieldIndex], nil
}

func normalizeFieldKey(key string) string {
	trimmed := strings.TrimSpace(key)
	trimmed = strings.TrimPrefix(trimmed, "/")
	return strings.ToUpper(trimmed)
}
