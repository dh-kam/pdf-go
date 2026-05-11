package pdf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactAs_EmptyPath(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 100}, rotate: 0},
	})
	err := doc.CompactAs(" ")
	require.Error(t, err)
}

func TestExportPages_InvalidRange(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 100}, rotate: 0},
	})
	err := doc.ExportPages(filepath.Join(t.TempDir(), "out.pdf"), 1, 0, true, true)
	require.Error(t, err)
}
