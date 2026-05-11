package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestDocumentUserData_BinaryAndString(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	err := doc.PutUserData("scope", "key", []byte("value"))
	require.NoError(t, err)

	got, ok := doc.GetUserData("scope", "key")
	require.True(t, ok)
	assert.Equal(t, []byte("value"), got)

	// Returned data must be a copy.
	got[0] = 'X'
	got2, ok := doc.GetUserData("scope", "key")
	require.True(t, ok)
	assert.Equal(t, []byte("value"), got2)

	err = doc.PutUserDataString("scope", "key2", "hello")
	require.NoError(t, err)
	text, ok := doc.GetUserDataString("scope", "key2")
	require.True(t, ok)
	assert.Equal(t, "hello", text)

	doc.DeleteUserData("scope", "key2")
	_, ok = doc.GetUserDataString("scope", "key2")
	assert.False(t, ok)
}

func TestDocumentUserData_PageScoped(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	err := doc.PutUserDataForPage(0, "note", "first-page")
	require.NoError(t, err)

	value, ok := doc.GetUserDataForPage(0, "note")
	require.True(t, ok)
	assert.Equal(t, "first-page", value)

	_, ok = doc.GetUserDataForPage(1, "note")
	assert.False(t, ok)
}

func TestDocumentUserData_InvalidInput(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	require.Error(t, doc.PutUserData("", "key", []byte("x")))
	require.Error(t, doc.PutUserData("scope", "", []byte("x")))
	require.Error(t, doc.PutUserDataForPage(-1, "k", "v"))
	require.Error(t, doc.PutUserDataForPage(0, "", "v"))
}
