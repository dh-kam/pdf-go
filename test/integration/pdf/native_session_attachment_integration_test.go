package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSaveWithNativeSessionUpdates_AttachmentsReopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "025-attachment", "with-attachment.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	baseAttachments, err := doc.Attachments()
	require.NoError(t, err)
	require.NotEmpty(t, baseAttachments)

	removedKey := baseAttachments[0].Name
	if removedKey == "" {
		removedKey = baseAttachments[0].FileName
	}
	require.NotEmpty(t, removedKey)
	require.NoError(t, doc.DeleteAttachment(removedKey))

	require.NoError(t, doc.AddAttachment(pdf.AttachmentSpec{
		Name:        "native-added",
		FileName:    "native-added.txt",
		Description: "native session added attachment",
		MIMEType:    "text/plain",
		Data:        []byte("native-attachment-payload"),
	}))

	output := filepath.Join(t.TempDir(), "native-session-attachment-updated.pdf")
	require.NoError(t, doc.SaveWithNativeSessionUpdates(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	reopenedItems, err := reopened.Attachments()
	require.NoError(t, err)
	require.NotEmpty(t, reopenedItems)

	for _, item := range reopenedItems {
		assert.NotEqual(t, removedKey, item.Name)
		assert.NotEqual(t, removedKey, item.FileName)
	}

	payload, err := reopened.ExportAttachmentData("native-added")
	require.NoError(t, err)
	assert.Equal(t, []byte("native-attachment-payload"), payload)
}
