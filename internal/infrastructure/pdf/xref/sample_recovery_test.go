package xref

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTable_UnreadableMetadataSample_ExposesLinearizedBlankFallback(t *testing.T) {
	pdfPath := sampleRecoveryPath(t, "017-unreadable-meta-data", "unreadablemetadata.pdf")
	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	table := NewTable(data)
	require.NoError(t, table.Parse())

	linearizedCount, ok := table.LinearizedPageCount()
	require.True(t, ok)
	assert.Equal(t, 4, linearizedCount)

	catalog, err := table.GetCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)

	pagesVal := catalog.Get(entity.Name("/Pages"))
	pagesDict, ok := pagesVal.(*entity.Dict)
	require.True(t, ok)
	require.NotNil(t, pagesDict)

	countObj, ok := pagesDict.Get(entity.Name("/Count")).(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(1), countObj.Value())

	pageRefs, err := table.RecoverPageRefsByObjectScan()
	require.NoError(t, err)
	assert.Equal(t, []entity.Ref{
		entity.NewRef(178, 0),
		entity.NewRef(1, 0),
		entity.NewRef(43, 0),
	}, pageRefs)
}

func sampleRecoveryPath(t *testing.T, group, name string) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(
		filepath.Dir(testFile),
		"..", "..", "..", "..",
		"test", "testdata", "sample-files",
		group,
		name,
	)
}
