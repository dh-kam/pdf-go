package pdf

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleOpenPassword = "openpassword"

func TestOpen_EncryptedSampleRequiresPassword(t *testing.T) {
	doc, err := Open(getSampleFilePath(t, "005-libreoffice-writer-password", "libreoffice-writer-password.pdf"))
	require.Error(t, err)
	require.Nil(t, doc)
}

func TestOpenWithPassword_EncryptedSampleSucceeds(t *testing.T) {
	doc, err := OpenWithPassword(
		getSampleFilePath(t, "005-libreoffice-writer-password", "libreoffice-writer-password.pdf"),
		sampleOpenPassword,
	)
	require.NoError(t, err)
	require.NotNil(t, doc)
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err)
	require.Equal(t, 1, pageCount)
}

func TestOpen_UnreadableMetadataSampleUsesLinearizedBlankFallback(t *testing.T) {
	doc, err := Open(getSampleFilePath(t, "017-unreadable-meta-data", "unreadablemetadata.pdf"))
	require.NoError(t, err)
	require.NotNil(t, doc)
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err)
	require.Equal(t, 4, pageCount)

	page, err := doc.GetPage(3)
	require.NoError(t, err)
	require.NotNil(t, page)
	require.Equal(t, 3, page.Index())
	require.Equal(t, [4]float64{0, 0, 0, 0}, page.MediaBox())
}

func getSampleFilePath(t *testing.T, group, name string) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(
		filepath.Dir(testFile),
		"..", "..", "..",
		"test", "testdata", "sample-files",
		group,
		name,
	)
}
