package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestApplyFormData_NilInput(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	_, err := doc.ApplyFormData(nil)
	require.Error(t, err)
}

func TestApplyFormData_NoFormFields(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	applied, err := doc.ApplyFormData(&FormData{
		Fields: map[string][]string{
			"Name": {"Alice"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, applied)
}

func TestSetFormFieldValues_UnknownField(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	err := doc.SetFormFieldValues("Unknown", []string{"v"})
	require.Error(t, err)
}

func TestClearFormDataOverrides(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.formValues["field"] = []string{"value"}

	doc.ClearFormDataOverrides()

	assert.Empty(t, doc.formValues)
}

func TestOverrideValuesToObject(t *testing.T) {
	single := overrideValuesToObject([]string{"one"})
	assert.Equal(t, "one", single)

	multi := overrideValuesToObject([]string{"one", "two"})
	values, ok := multi.([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"one", "two"}, values)
}

func TestChoiceFieldMutation_SetAddRemoveClear(t *testing.T) {
	doc := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"A", "B"})

	require.NoError(t, doc.SetChoiceFieldItems("ChoiceA", []string{"X", "Y"}))
	field := loadFieldByName(t, doc, "ChoiceA")
	assert.Equal(t, []string{"X", "Y"}, field.Options)

	require.NoError(t, doc.AddChoiceFieldItem("ChoiceA", "Z"))
	field = loadFieldByName(t, doc, "ChoiceA")
	assert.Equal(t, []string{"X", "Y", "Z"}, field.Options)

	require.NoError(t, doc.RemoveChoiceFieldItem("ChoiceA", 1))
	field = loadFieldByName(t, doc, "ChoiceA")
	assert.Equal(t, []string{"X", "Z"}, field.Options)

	require.NoError(t, doc.ClearChoiceFieldItems("ChoiceA"))
	field = loadFieldByName(t, doc, "ChoiceA")
	assert.Empty(t, field.Options)
}

func TestChoiceFieldMutation_InvalidCases(t *testing.T) {
	doc := newChoiceFieldDocument(t, "TextA", "Tx", []string{"A"})

	err := doc.SetChoiceFieldItems("Unknown", []string{"A"})
	require.Error(t, err)

	err = doc.SetChoiceFieldItems("TextA", []string{"A"})
	require.Error(t, err)

	choiceDoc := newChoiceFieldDocument(t, "ChoiceB", "Ch", []string{"A"})
	err = choiceDoc.RemoveChoiceFieldItem("ChoiceB", 2)
	require.Error(t, err)
}

func newChoiceFieldDocument(t *testing.T, fieldName, fieldType string, options []string) *Document {
	t.Helper()

	field := entity.NewDict()
	field.Set(entity.Name("T"), entity.NewString(fieldName))
	field.Set(entity.Name("FT"), entity.NewName(fieldType))
	optItems := make([]entity.Object, 0, len(options))
	for _, option := range options {
		optItems = append(optItems, entity.NewString(option))
	}
	field.Set(entity.Name("Opt"), entity.NewArray(optItems...))

	acro := entity.NewDict()
	acro.Set(entity.Name("Fields"), entity.NewArray(field))

	pages := entity.NewDict()
	pages.Set(entity.Name("Type"), entity.NewName("Pages"))
	pages.Set(entity.Name("Count"), entity.NewInteger(0))
	pages.Set(entity.Name("Kids"), entity.NewArray())

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.NewName("Catalog"))
	catalog.Set(entity.Name("Pages"), pages)
	catalog.Set(entity.Name("AcroForm"), acro)

	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(catalog)
	return newDocument(entityDoc)
}

func loadFieldByName(t *testing.T, doc *Document, name string) *FormField {
	t.Helper()

	fields, err := doc.FormFields()
	require.NoError(t, err)
	for _, field := range fields {
		if field != nil && field.Name == name {
			return field
		}
	}
	require.FailNow(t, "field not found", name)
	return nil
}
