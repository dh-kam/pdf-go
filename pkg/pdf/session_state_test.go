package pdf

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestExportSessionState(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	data, err := doc.ExportSessionState()
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestImportSessionState_InvalidInput(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	require.Error(t, doc.ImportSessionState(nil))
	require.Error(t, doc.ImportSessionState([]byte("{invalid-json")))
}

func TestSessionState_DeletedAttachmentsRoundTrip(t *testing.T) {
	source := newDocument(newMinimalEntityDocument())
	require.NoError(t, source.DeleteAttachment("file.bin"))

	data, err := source.ExportSessionState()
	require.NoError(t, err)

	target := newDocument(newMinimalEntityDocument())
	require.NoError(t, target.ImportSessionState(data))

	assert.True(t, target.IsAttachmentDeleted("file.bin"))
}

func TestSessionState_AddedAttachmentsRoundTrip(t *testing.T) {
	source := newDocument(newMinimalEntityDocument())
	require.NoError(t, source.AddAttachment(AttachmentSpec{
		Name:     "session-added",
		FileName: "session-added.txt",
		Data:     []byte("payload"),
		MIMEType: "text/plain",
	}))

	data, err := source.ExportSessionState()
	require.NoError(t, err)

	target := newDocument(newMinimalEntityDocument())
	require.NoError(t, target.ImportSessionState(data))

	items, err := target.Attachments()
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "session-added", items[0].Name)
	assert.Equal(t, "session-added.txt", items[0].FileName)
	assert.Equal(t, []byte("payload"), items[0].Data())
}

func TestSessionState_JSONContainsDeletedAttachments(t *testing.T) {
	doc := newDocument(newMinimalEntityDocument())
	require.NoError(t, doc.DeleteAttachment("example.txt"))

	data, err := doc.ExportSessionState()
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	_, ok := payload["deleted_attachments"]
	assert.True(t, ok)
}

func TestSessionState_JSONContainsAddedAttachments(t *testing.T) {
	doc := newDocument(newMinimalEntityDocument())
	require.NoError(t, doc.AddAttachment(AttachmentSpec{
		Name:     "json-added",
		FileName: "json-added.txt",
		Data:     []byte("payload"),
	}))

	data, err := doc.ExportSessionState()
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	_, ok := payload["added_attachments"]
	assert.True(t, ok)
}

func TestSessionState_FormOptionsRoundTrip(t *testing.T) {
	source := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"A", "B"})
	require.NoError(t, source.SetChoiceFieldItems("ChoiceA", []string{"X", "Y"}))

	data, err := source.ExportSessionState()
	require.NoError(t, err)

	target := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"A", "B"})
	require.NoError(t, target.ImportSessionState(data))

	field := loadFieldByName(t, target, "ChoiceA")
	assert.Equal(t, []string{"X", "Y"}, field.Options)
}

func TestSessionState_JSONContainsFormOptions(t *testing.T) {
	doc := newChoiceFieldDocument(t, "ChoiceA", "Ch", []string{"A", "B"})
	require.NoError(t, doc.SetChoiceFieldItems("ChoiceA", []string{"X"}))

	data, err := doc.ExportSessionState()
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	_, ok := payload["form_options"]
	assert.True(t, ok)
}

func newMinimalEntityDocument() *entity.Document {
	entityDoc := entity.NewDocument(nil)

	pages := entity.NewDict()
	pages.Set(entity.Name("Type"), entity.NewName("Pages"))
	pages.Set(entity.Name("Count"), entity.NewInteger(0))
	pages.Set(entity.Name("Kids"), entity.NewArray())

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.NewName("Catalog"))
	catalog.Set(entity.Name("Pages"), pages)
	entityDoc.SetCatalog(catalog)

	return entityDoc
}
