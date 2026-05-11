package entity

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recoveryXRefMock struct {
	missingRef       Ref
	missingErr       error
	recoveredCatalog *Dict
	recovered        bool
}

type pageScanXRefMock struct {
	pagesRef            Ref
	pagesDict           *Dict
	scannedRefs         []Ref
	recoverCalls        int
	syntheticCatalog    bool
	linearizedPageCount int
}

func (m *recoveryXRefMock) Fetch(ref Ref) (Object, error) {
	if ref == m.missingRef {
		return nil, m.missingErr
	}
	return nil, errors.New("unexpected fetch")
}

func (m *recoveryXRefMock) RebuildCatalogByObjectScan() error {
	m.recovered = true
	return nil
}

func (m *recoveryXRefMock) GetCatalog() (*Dict, error) {
	if !m.recovered {
		return nil, errors.New("not recovered")
	}
	return m.recoveredCatalog, nil
}

func (m *pageScanXRefMock) Fetch(ref Ref) (Object, error) {
	if ref == m.pagesRef {
		return m.pagesDict, nil
	}
	for _, scanned := range m.scannedRefs {
		if ref == scanned {
			page := NewDict()
			page.Set(Name("/Type"), Name("Page"))
			return page, nil
		}
	}
	return nil, errors.New("unexpected fetch")
}

func (m *pageScanXRefMock) RecoverPageRefsByObjectScan() ([]Ref, error) {
	m.recoverCalls++
	out := make([]Ref, len(m.scannedRefs))
	copy(out, m.scannedRefs)
	return out, nil
}

func (m *pageScanXRefMock) UsesSyntheticCatalog() bool {
	return m.syntheticCatalog
}

func (m *pageScanXRefMock) LinearizedPageCount() (int, bool) {
	if m.linearizedPageCount <= 0 {
		return 0, false
	}
	return m.linearizedPageCount, true
}

func TestDocumentPageCount_RecoversCatalogOnMissingPagesRef(t *testing.T) {
	missingPagesRef := NewRef(172, 0)

	pageDict := NewDict()
	pageDict.Set(Name("/Type"), Name("Page"))

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(1))
	pagesDict.Set(Name("/Kids"), NewArray(pageDict))

	recoveredCatalog := NewDict()
	recoveredCatalog.Set(Name("/Type"), Name("Catalog"))
	recoveredCatalog.Set(Name("/Pages"), pagesDict)

	initialCatalog := NewDict()
	initialCatalog.Set(Name("/Type"), Name("Catalog"))
	initialCatalog.Set(Name("/Pages"), missingPagesRef)

	x := &recoveryXRefMock{
		missingRef:       missingPagesRef,
		missingErr:       errors.New("object 172 not found"),
		recoveredCatalog: recoveredCatalog,
	}

	doc := NewDocument(x)
	doc.SetCatalog(initialCatalog)

	count, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.True(t, x.recovered)
}

func TestDocumentGetPage_RecoversCatalogOnMissingPagesRef(t *testing.T) {
	missingPagesRef := NewRef(172, 0)

	pageDict := NewDict()
	pageDict.Set(Name("/Type"), Name("Page"))

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(1))
	pagesDict.Set(Name("/Kids"), NewArray(pageDict))

	recoveredCatalog := NewDict()
	recoveredCatalog.Set(Name("/Type"), Name("Catalog"))
	recoveredCatalog.Set(Name("/Pages"), pagesDict)

	initialCatalog := NewDict()
	initialCatalog.Set(Name("/Type"), Name("Catalog"))
	initialCatalog.Set(Name("/Pages"), missingPagesRef)

	x := &recoveryXRefMock{
		missingRef:       missingPagesRef,
		missingErr:       errors.New("object 172 not found"),
		recoveredCatalog: recoveredCatalog,
	}

	doc := NewDocument(x)
	doc.SetCatalog(initialCatalog)

	page, err := doc.GetPage(0)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.Equal(t, 0, page.Index())
	assert.Equal(t, Name("Page"), page.Dict().Get(Name("/Type")))
	assert.True(t, x.recovered)
}

func TestDocumentPageCount_KeepsDeclaredCountWhenPositiveCountDeltaIsTooSmall(t *testing.T) {
	pagesRef := NewRef(172, 0)

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(1))
	pagesDict.Set(Name("/Kids"), NewArray(NewRef(1, 0)))

	catalog := NewDict()
	catalog.Set(Name("/Type"), Name("Catalog"))
	catalog.Set(Name("/Pages"), pagesRef)

	x := &pageScanXRefMock{
		pagesRef:    pagesRef,
		pagesDict:   pagesDict,
		scannedRefs: []Ref{NewRef(1, 0), NewRef(2, 0)},
	}

	doc := NewDocument(x)
	doc.SetCatalog(catalog)

	count, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 1, x.recoverCalls)
}

func TestDocumentPageCount_RecoversByPageScanWhenPositiveCountLooksTooSmall(t *testing.T) {
	pagesRef := NewRef(172, 0)

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(1))
	pagesDict.Set(Name("/Kids"), NewArray(NewRef(1, 0)))

	catalog := NewDict()
	catalog.Set(Name("/Type"), Name("Catalog"))
	catalog.Set(Name("/Pages"), pagesRef)

	x := &pageScanXRefMock{
		pagesRef:    pagesRef,
		pagesDict:   pagesDict,
		scannedRefs: []Ref{NewRef(1, 0), NewRef(2, 0), NewRef(3, 0), NewRef(4, 0)},
	}

	doc := NewDocument(x)
	doc.SetCatalog(catalog)

	count, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 4, count)
	assert.Equal(t, 1, x.recoverCalls)
}

func TestDocumentPageCount_UsesLinearizedBlankFallbackWhenSyntheticCatalogIsSmaller(t *testing.T) {
	pagesRef := NewRef(172, 0)

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(1))
	pagesDict.Set(Name("/Kids"), NewArray(NewRef(178, 0)))

	catalog := NewDict()
	catalog.Set(Name("/Type"), Name("Catalog"))
	catalog.Set(Name("/Pages"), pagesDict)

	x := &pageScanXRefMock{
		pagesRef:            pagesRef,
		pagesDict:           pagesDict,
		scannedRefs:         []Ref{NewRef(178, 0), NewRef(1, 0), NewRef(43, 0)},
		syntheticCatalog:    true,
		linearizedPageCount: 4,
	}

	doc := NewDocument(x)
	doc.SetCatalog(catalog)

	count, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 4, count)

	page, err := doc.GetPage(3)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.Equal(t, 3, page.Index())
	assert.Equal(t, [4]float64{0, 0, 0, 0}, page.MediaBox())
}

func TestDocumentGetPage_RecoversByPageScanWhenPositiveCountLooksTooSmall(t *testing.T) {
	pagesRef := NewRef(172, 0)

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(1))
	pagesDict.Set(Name("/Kids"), NewArray(NewRef(1, 0)))

	catalog := NewDict()
	catalog.Set(Name("/Type"), Name("Catalog"))
	catalog.Set(Name("/Pages"), pagesRef)

	x := &pageScanXRefMock{
		pagesRef:    pagesRef,
		pagesDict:   pagesDict,
		scannedRefs: []Ref{NewRef(1, 0), NewRef(2, 0), NewRef(3, 0), NewRef(4, 0)},
	}

	doc := NewDocument(x)
	doc.SetCatalog(catalog)

	page, err := doc.GetPage(3)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.Equal(t, 3, page.Index())
	assert.Equal(t, 1, x.recoverCalls)
}

func TestDocumentPageCount_RecoversByPageScanWhenCountIsZero(t *testing.T) {
	pagesRef := NewRef(172, 0)

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(0))
	pagesDict.Set(Name("/Kids"), NewArray())

	catalog := NewDict()
	catalog.Set(Name("/Type"), Name("Catalog"))
	catalog.Set(Name("/Pages"), pagesRef)

	x := &pageScanXRefMock{
		pagesRef:    pagesRef,
		pagesDict:   pagesDict,
		scannedRefs: []Ref{NewRef(1, 0), NewRef(2, 0), NewRef(3, 0)},
	}

	doc := NewDocument(x)
	doc.SetCatalog(catalog)

	count, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Equal(t, 1, x.recoverCalls)

	recoveredPages, ok := doc.Catalog().Get(Name("/Pages")).(*Dict)
	require.True(t, ok)
	recoveredCount, ok := recoveredPages.Get(Name("/Count")).(*Integer)
	require.True(t, ok)
	assert.Equal(t, int64(3), recoveredCount.Value())
}
