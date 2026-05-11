package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type attachmentXRef struct {
	objects map[entity.Ref]entity.Object
}

func (x *attachmentXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	obj, ok := x.objects[ref]
	if !ok {
		return nil, assert.AnError
	}
	return obj, nil
}

func TestAttachments_EmbeddedFilesNameTree(t *testing.T) {
	streamDict := entity.NewDict()
	streamDict.Set(entity.Name("Subtype"), entity.NewName("text/plain"))
	paramsDict := entity.NewDict()
	paramsDict.Set(entity.Name("Size"), entity.NewInteger(5))
	streamDict.Set(entity.Name("Params"), paramsDict)
	stream := entity.NewStream(streamDict, []byte("hello"))

	ef := entity.NewDict()
	ef.Set(entity.Name("F"), stream)

	spec := entity.NewDict()
	spec.Set(entity.Name("Type"), entity.NewName("Filespec"))
	spec.Set(entity.Name("F"), entity.NewString("hello.txt"))
	spec.Set(entity.Name("Desc"), entity.NewString("greeting"))
	spec.Set(entity.Name("EF"), ef)

	embedded := entity.NewDict()
	embedded.Set(entity.Name("Names"), entity.NewArray(
		entity.NewString("att-key"),
		spec,
	))

	names := entity.NewDict()
	names.Set(entity.Name("EmbeddedFiles"), embedded)

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Names"), names)

	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(catalog)
	doc := newDocument(entityDoc)

	attachments, err := doc.Attachments()
	require.NoError(t, err)
	require.Len(t, attachments, 1)

	att := attachments[0]
	assert.Equal(t, "att-key", att.Name)
	assert.Equal(t, "hello.txt", att.FileName)
	assert.Equal(t, "greeting", att.Description)
	assert.Equal(t, "text/plain", att.MIMEType)
	assert.Equal(t, 5, att.Size)
	assert.Equal(t, []byte("hello"), att.Data())
}

func TestAttachments_WithRefObjectsAndKids(t *testing.T) {
	streamRef := entity.NewRef(20, 0)
	specRef := entity.NewRef(10, 0)
	kidRef := entity.NewRef(2, 0)

	stream := entity.NewStream(entity.NewDict(), []byte("payload"))

	ef := entity.NewDict()
	ef.Set(entity.Name("F"), streamRef)

	spec := entity.NewDict()
	spec.Set(entity.Name("F"), entity.NewString("file.bin"))
	spec.Set(entity.Name("EF"), ef)

	kid := entity.NewDict()
	kid.Set(entity.Name("Names"), entity.NewArray(
		entity.NewString("ref-key"),
		specRef,
	))

	embedded := entity.NewDict()
	embedded.Set(entity.Name("Kids"), entity.NewArray(kidRef))

	names := entity.NewDict()
	names.Set(entity.Name("EmbeddedFiles"), embedded)

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Names"), names)

	xref := &attachmentXRef{
		objects: map[entity.Ref]entity.Object{
			streamRef: stream,
			specRef:   spec,
			kidRef:    kid,
		},
	}

	entityDoc := entity.NewDocument(xref)
	entityDoc.SetCatalog(catalog)
	doc := newDocument(entityDoc)

	attachments, err := doc.Attachments()
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	assert.Equal(t, "ref-key", attachments[0].Name)
	assert.Equal(t, "file.bin", attachments[0].FileName)
	assert.Equal(t, []byte("payload"), attachments[0].Data())
}

func TestAttachments_NoEmbeddedFiles(t *testing.T) {
	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(entity.NewDict())
	doc := newDocument(entityDoc)

	attachments, err := doc.Attachments()
	require.NoError(t, err)
	assert.Nil(t, attachments)
}

func TestAttachments_DeleteRestoreAndExport(t *testing.T) {
	stream := entity.NewStream(entity.NewDict(), []byte("payload"))
	ef := entity.NewDict()
	ef.Set(entity.Name("F"), stream)

	spec := entity.NewDict()
	spec.Set(entity.Name("F"), entity.NewString("file.bin"))
	spec.Set(entity.Name("EF"), ef)

	embedded := entity.NewDict()
	embedded.Set(entity.Name("Names"), entity.NewArray(
		entity.NewString("ref-key"),
		spec,
	))
	names := entity.NewDict()
	names.Set(entity.Name("EmbeddedFiles"), embedded)
	catalog := entity.NewDict()
	catalog.Set(entity.Name("Names"), names)

	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(catalog)
	doc := newDocument(entityDoc)

	data, err := doc.ExportAttachmentData("file.bin")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)

	require.NoError(t, doc.DeleteAttachment("ref-key"))
	assert.True(t, doc.IsAttachmentDeleted("ref-key"))

	attachments, err := doc.Attachments()
	require.NoError(t, err)
	assert.Nil(t, attachments)

	doc.RestoreAttachment("ref-key")
	assert.False(t, doc.IsAttachmentDeleted("ref-key"))
	attachments, err = doc.Attachments()
	require.NoError(t, err)
	require.Len(t, attachments, 1)
}

func TestAttachments_ExportToFile(t *testing.T) {
	stream := entity.NewStream(entity.NewDict(), []byte("payload"))
	ef := entity.NewDict()
	ef.Set(entity.Name("F"), stream)

	spec := entity.NewDict()
	spec.Set(entity.Name("F"), entity.NewString("file.bin"))
	spec.Set(entity.Name("EF"), ef)

	embedded := entity.NewDict()
	embedded.Set(entity.Name("Names"), entity.NewArray(
		entity.NewString("export-key"),
		spec,
	))
	names := entity.NewDict()
	names.Set(entity.Name("EmbeddedFiles"), embedded)
	catalog := entity.NewDict()
	catalog.Set(entity.Name("Names"), names)

	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(catalog)
	doc := newDocument(entityDoc)

	outPath := filepath.Join(t.TempDir(), "out.bin")
	require.NoError(t, doc.ExportAttachmentToFile("export-key", outPath))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)
}

func TestAttachments_JavaParityAliases(t *testing.T) {
	stream := entity.NewStream(entity.NewDict(), []byte("payload"))
	ef := entity.NewDict()
	ef.Set(entity.Name("F"), stream)

	spec := entity.NewDict()
	spec.Set(entity.Name("F"), entity.NewString("file.bin"))
	spec.Set(entity.Name("EF"), ef)

	embedded := entity.NewDict()
	embedded.Set(entity.Name("Names"), entity.NewArray(
		entity.NewString("alias-key"),
		spec,
	))
	names := entity.NewDict()
	names.Set(entity.Name("EmbeddedFiles"), embedded)
	catalog := entity.NewDict()
	catalog.Set(entity.Name("Names"), names)

	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(catalog)
	doc := newDocument(entityDoc)

	items, err := doc.AttachedFileList()
	require.NoError(t, err)
	require.Len(t, items, 1)

	items, err = doc.GetAttachedFileList()
	require.NoError(t, err)
	require.Len(t, items, 1)

	data, err := doc.ExportAttachedFileData("alias-key")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)

	require.NoError(t, doc.DeleteAttachedFile("alias-key"))
	items, err = doc.AttachedFileList()
	require.NoError(t, err)
	assert.Nil(t, items)
}

func TestAttachments_AddAttachmentOverlay(t *testing.T) {
	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(entity.NewDict())
	doc := newDocument(entityDoc)

	require.NoError(t, doc.AddAttachment(AttachmentSpec{
		Name:        "overlay-key",
		FileName:    "overlay.txt",
		Description: "overlay desc",
		MIMEType:    "text/plain",
		Data:        []byte("overlay-payload"),
	}))

	items, err := doc.Attachments()
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "overlay-key", items[0].Name)
	assert.Equal(t, "overlay.txt", items[0].FileName)
	assert.Equal(t, "overlay desc", items[0].Description)
	assert.Equal(t, "text/plain", items[0].MIMEType)
	assert.Equal(t, []byte("overlay-payload"), items[0].Data())

	require.NoError(t, doc.DeleteAttachment("overlay.txt"))
	items, err = doc.Attachments()
	require.NoError(t, err)
	assert.Nil(t, items)

	doc.RestoreAttachment("overlay-key")
	items, err = doc.Attachments()
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestAttachments_AddAttachmentFromFileAndAttachAlias(t *testing.T) {
	entityDoc := entity.NewDocument(nil)
	entityDoc.SetCatalog(entity.NewDict())
	doc := newDocument(entityDoc)

	filePath := filepath.Join(t.TempDir(), "hello.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o644))

	require.NoError(t, doc.AddAttachmentFromFile("from-file", filePath))
	data, err := doc.ExportAttachmentData("from-file")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)

	require.NoError(t, doc.DeleteAttachment("from-file"))
	require.NoError(t, doc.AttachFile("alias-file", filePath))
	data, err = doc.ExportAttachmentData("alias-file")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}
