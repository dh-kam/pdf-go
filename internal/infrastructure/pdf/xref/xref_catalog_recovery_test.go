package xref

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestRebuildCatalogByObjectScan_ReplacesBrokenPagesRefWithScannedPages(t *testing.T) {
	data := []byte(
		"%PDF-1.4\n" +
			"1 0 obj\n" +
			"<</Type/Page/Parent 172 0 R/Contents 4 0 R>>\n" +
			"endobj\n" +
			"2 0 obj\n" +
			"<</Type/Page/Parent 172 0 R/Contents 5 0 R>>\n" +
			"endobj\n" +
			"4 0 obj\n" +
			"<</Length 0>>\n" +
			"stream\n" +
			"\n" +
			"endstream\n" +
			"endobj\n" +
			"5 0 obj\n" +
			"<</Length 0>>\n" +
			"stream\n" +
			"\n" +
			"endstream\n" +
			"endobj\n" +
			"176 0 obj\n" +
			"<</Metadata 173 0 R/Pages 172 0 R/Type/Catalog>>\n" +
			"endobj\n",
	)

	table := NewTable(data)
	require.NoError(t, table.RebuildCatalogByObjectScan())

	catalog, err := table.GetCatalog()
	require.NoError(t, err)

	pagesObj := catalog.Get(entity.Name("/Pages"))
	pagesDict, ok := pagesObj.(*entity.Dict)
	require.True(t, ok, "recovered catalog should embed a pages dictionary")

	countObj, ok := pagesDict.Get(entity.Name("/Count")).(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(2), countObj.Value())

	kids, ok := pagesDict.Get(entity.Name("/Kids")).(*entity.Array)
	require.True(t, ok)
	require.Equal(t, 2, kids.Len())

	firstRef, ok := kids.Get(0).(entity.Ref)
	require.True(t, ok)
	secondRef, ok := kids.Get(1).(entity.Ref)
	require.True(t, ok)
	assert.Equal(t, uint32(1), firstRef.Num())
	assert.Equal(t, uint32(2), secondRef.Num())

	doc := entity.NewDocument(table)
	doc.SetCatalog(catalog)

	pageCount, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 2, pageCount)
}

func TestRebuildCatalogByObjectScan_IgnoresStalePageGenerations(t *testing.T) {
	data := []byte(
		"%PDF-1.4\n" +
			"1 0 obj\n" +
			"<</Type/Page/Parent 2 0 R/Contents 3 0 R>>\n" +
			"endobj\n" +
			"1 1 obj\n" +
			"<</Type/Page/Parent 2 0 R/Contents 4 0 R>>\n" +
			"endobj\n" +
			"2 0 obj\n" +
			"<</Type/Pages/Count 1/Kids [1 1 R]>>\n" +
			"endobj\n" +
			"3 0 obj\n" +
			"<</Length 0>>\n" +
			"stream\n" +
			"\n" +
			"endstream\n" +
			"endobj\n" +
			"4 0 obj\n" +
			"<</Length 0>>\n" +
			"stream\n" +
			"\n" +
			"endstream\n" +
			"endobj\n" +
			"5 0 obj\n" +
			"<</Type/Catalog/Pages 2 0 R>>\n" +
			"endobj\n",
	)

	table := NewTable(data)
	require.NoError(t, table.RebuildCatalogByObjectScan())

	catalog, err := table.GetCatalog()
	require.NoError(t, err)

	pagesObj := catalog.Get(entity.Name("/Pages"))
	pagesDict, ok := pagesObj.(*entity.Dict)
	require.True(t, ok)

	countObj, ok := pagesDict.Get(entity.Name("/Count")).(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(1), countObj.Value())

	kids, ok := pagesDict.Get(entity.Name("/Kids")).(*entity.Array)
	require.True(t, ok)
	require.Equal(t, 1, kids.Len())

	onlyRef, ok := kids.Get(0).(entity.Ref)
	require.True(t, ok)
	assert.Equal(t, uint32(1), onlyRef.Num())
	assert.Equal(t, uint16(1), onlyRef.Gen())
}
