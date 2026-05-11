package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestCompactAs_Reopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "001-trivial", "minimal-document.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	beforeCount, err := doc.PageCount()
	require.NoError(t, err)

	output := filepath.Join(t.TempDir(), "compact.pdf")
	require.NoError(t, doc.CompactAs(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	afterCount, err := reopened.PageCount()
	require.NoError(t, err)
	assert.Equal(t, beforeCount, afterCount)
}

func TestExportPages_Reopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "001-trivial", "minimal-document.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	output := filepath.Join(t.TempDir(), "exported-pages.pdf")
	require.NoError(t, doc.ExportPages(output, 0, 0, true, true))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	count, err := reopened.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
