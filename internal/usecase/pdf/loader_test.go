package pdf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestOpen_ReturnsErrorForMissingPath(t *testing.T) {
	doc, err := Open("/__missing__/not-found.pdf")
	assert.Nil(t, doc)
	assert.Error(t, err)
}

func TestOpen_PathBasedOpen_SetsFileSize(t *testing.T) {
	pdfPath := getLibreOfficeSamplePath(t)
	info, err := os.Stat(pdfPath)
	require.NoError(t, err)
	require.NotNil(t, info)

	doc, err := Open(pdfPath)
	require.NoError(t, err)
	require.NotNil(t, doc)
	defer doc.Close()

	assert.Equal(t, info.Size(), doc.FileSize())
}

func TestOpenWithPassword_UsesBaseOpenPath(t *testing.T) {
	pdfPath := getLibreOfficeSamplePath(t)
	doc, err := OpenWithPassword(pdfPath, "")
	require.NoError(t, err)
	require.NotNil(t, doc)
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err)
	assert.Greater(t, pageCount, 0)
}

func TestOpenBytes_OpenReader_BehaveSame(t *testing.T) {
	pdfPath := getLibreOfficeSamplePath(t)
	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	doc1, err := OpenBytes(data)
	require.NoError(t, err)
	require.NotNil(t, doc1)
	defer doc1.Close()
	assert.Equal(t, int64(len(data)), doc1.FileSize())

	doc2, err := OpenReader(bytes.NewReader(data))
	require.NoError(t, err)
	require.NotNil(t, doc2)
	defer doc2.Close()
	assert.Equal(t, int64(len(data)), doc2.FileSize())
}

func TestOpenReader_FailWithInvalidData(t *testing.T) {
	doc, err := OpenReader(strings.NewReader("not a pdf file"))
	assert.Nil(t, doc)
	assert.Error(t, err)
}

func TestOpenReaderWithPassword_SupportsPasswordArg(t *testing.T) {
	pdfPath := getLibreOfficeSamplePath(t)
	f, err := os.Open(pdfPath)
	require.NoError(t, err)
	defer f.Close()

	doc, err := OpenReaderWithPassword(f, "")
	require.NoError(t, err)
	require.NotNil(t, doc)
	defer doc.Close()

	assert.NotZero(t, doc.FileSize())
}

func TestParseMetadata_WithStreamMetadata(t *testing.T) {
	doc := entity.NewDocument(nil)
	dict := entity.NewDict()
	stream := entity.NewStream(entity.NewDict(), []byte(`<?xml version="1.0"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/"
    xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:xmp="http://ns.adobe.com/xmp/1.0/"
    xmlns:pdf="http://ns.adobe.com/pdf/1.1/"
    xmlns:xap="http://ns.adobe.com/xap/1.0/">
  <rdf:RDF>
    <rdf:Description>
      <dc:title>
        <rdf:Alt>
          <rdf:li>Sample</rdf:li>
        </rdf:Alt>
      </dc:title>
      <dc:creator>
        <rdf:Seq>
          <rdf:li>Tester</rdf:li>
        </rdf:Seq>
      </dc:creator>
      <xmp:CreateDate>2026-02-16T12:00:00Z</xmp:CreateDate>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>`))
	dict.Set(entity.Name("/Metadata"), stream)
	doc.SetCatalog(dict)

	err := parseMetadata(doc)

	require.NoError(t, err)
	require.NotNil(t, doc.ParsedMetadata())

	parsed := doc.ParsedMetadata()
	assert.Equal(t, []string{"Sample"}, parsed.Title())
	assert.Equal(t, []string{"Tester"}, parsed.Creator())
	assert.False(t, parsed.CreateDate().IsZero())
	assert.Empty(t, parsed.Producer())
}

func TestParseMetadata_IgnoresUnsupportedMetadataType(t *testing.T) {
	doc := entity.NewDocument(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("/Metadata"), entity.NewInteger(1))
	doc.SetCatalog(dict)

	err := parseMetadata(doc)
	assert.NoError(t, err)
	assert.Nil(t, doc.ParsedMetadata())
}

func TestParseMetadata_RefMetadataRequiresXrefTable(t *testing.T) {
	doc := entity.NewDocument(&stubXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("/Metadata"), entity.NewRef(1, 0))
	doc.SetCatalog(dict)

	err := parseMetadata(doc)

	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "xref is not a Table"))
}

func TestParseMetadata_NilCatalog_ShortCircuits(t *testing.T) {
	doc := entity.NewDocument(nil)
	err := parseMetadata(doc)
	assert.NoError(t, err)
	assert.Nil(t, doc.ParsedMetadata())
}

type stubXRef struct{}

func (s *stubXRef) Fetch(_ entity.Ref) (entity.Object, error) {
	return nil, errors.New("stub fetch")
}

func getLibreOfficeSamplePath(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	testDir := filepath.Dir(testFile)
	return filepath.Join(testDir, "..", "..", "..", "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")
}
