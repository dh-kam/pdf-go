package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldLegacy_ReadAccessors(t *testing.T) {
	doc := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"X", "Y"})

	count, err := doc.FieldGetNumFields()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	title, err := doc.FieldGetTitle(0)
	require.NoError(t, err)
	assert.Equal(t, "ChoiceA", title)

	typ, err := doc.FieldGetNameValue(0, "FT")
	require.NoError(t, err)
	assert.Equal(t, "Ch", typ)

	options, err := doc.FieldGetStringValue(0, "Opt")
	require.NoError(t, err)
	assert.Equal(t, "X,Y", options)

	required, err := doc.FieldGetBooleanValue(0, "Required", true)
	require.NoError(t, err)
	assert.False(t, required)
}

func TestFieldLegacy_Setters(t *testing.T) {
	doc := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"X", "Y"})

	ok, err := doc.FieldSetStringValue(0, "V", "X")
	require.NoError(t, err)
	assert.True(t, ok)

	value, err := doc.FieldGetStringValue(0, "V")
	require.NoError(t, err)
	assert.Equal(t, "X", value)

	ok, err = doc.FieldSetBooleanValue(0, "V", true)
	require.NoError(t, err)
	assert.True(t, ok)

	value, err = doc.FieldGetStringValue(0, "V")
	require.NoError(t, err)
	assert.Equal(t, "true", value)

	result, err := doc.FieldSetValue(0, "Y")
	require.NoError(t, err)
	assert.Equal(t, 1, result)

	value, err = doc.FieldGetStringValue(0, "V")
	require.NoError(t, err)
	assert.Equal(t, "Y", value)
}

func TestFieldLegacy_InvalidAndUnsupported(t *testing.T) {
	doc := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"X", "Y"})

	_, err := doc.FieldGetTitle(3)
	require.Error(t, err)

	ok, err := doc.FieldSetStringValue(0, "Unsupported", "x")
	require.NoError(t, err)
	assert.False(t, ok)

	assert.Equal(t, -1, doc.FieldFindByRefNo(123))
}
