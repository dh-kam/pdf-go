package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/metadata"
)

func TestDetectPDFVersion(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "unknown short", data: []byte("%PDF"), want: "Unknown"},
		{name: "unknown prefix", data: []byte("testdata"), want: "Unknown"},
		{name: "valid", data: []byte("%PDF-2.0"), want: "2.0"},
		{name: "short valid", data: []byte("%PDF-"), want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectPDFVersion(tt.data))
		})
	}
}

func TestStringifyPDFValue(t *testing.T) {
	assert.Equal(t, "", stringifyPDFValue(nil))
	assert.Equal(t, "hello", stringifyPDFValue("hello"))
	assert.Equal(t, "12", stringifyPDFValue(12))
	assert.Equal(t, "3.14", stringifyPDFValue(3.14))
	assert.Equal(t, "true", stringifyPDFValue(true))
	assert.Equal(t, "false", stringifyPDFValue(false))
	assert.Equal(t, "[1 2 3]", stringifyPDFValue([]int{1, 2, 3}))
}

func TestJoinStrings(t *testing.T) {
	assert.Equal(t, "", joinStrings(nil, ","))
	assert.Equal(t, "a", joinStrings([]string{"a"}, ","))
	assert.Equal(t, "a,b,c", joinStrings([]string{"a", "b", "c"}, ","))
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 2, indexOf([]string{"a", "b", "c"}, "c"))
	assert.Equal(t, -1, indexOf([]string{"a", "b"}, "z"))
}

func TestIsEncrypted(t *testing.T) {
	assert.False(t, isEncrypted(nil))

	d := entity.NewDict()
	d.Set(entity.Name("/Info"), entity.NewString("v"))
	assert.False(t, isEncrypted(d))

	d.Set(entity.Name("/Encrypt"), entity.NewBoolean(true))
	assert.True(t, isEncrypted(d))
}

func TestExtractDocumentInfo(t *testing.T) {
	d := entity.NewDict()
	d.Set(entity.Name("/Title"), entity.NewString("Title"))
	d.Set(entity.Name("/Author"), entity.NewString("Author"))
	d.Set(entity.Name("/Subject"), entity.NewString("Sub"))
	d.Set(entity.Name("/Keywords"), entity.NewString("k1"))
	d.Set(entity.Name("/Creator"), entity.NewString("Creator"))
	d.Set(entity.Name("/Producer"), entity.NewString("Producer"))

	docInfo := extractDocumentInfo(d)
	assert.Equal(t, "Title", docInfo.Title)
	assert.Equal(t, "Author", docInfo.Author)
	assert.Equal(t, "Sub", docInfo.Subject)
	assert.Equal(t, "k1", docInfo.Keywords)
	assert.Equal(t, "Creator", docInfo.Creator)
	assert.Equal(t, "Producer", docInfo.Producer)
}

func TestExtractMetadata(t *testing.T) {
	doc := entity.NewDocument(nil)
	tmeta := metadata.NewMetadata("raw")
	tmeta.SetTitle([]string{"t1", "t2"})
	tmeta.SetCreator([]string{"c1"})
	tmeta.SetSubject([]string{"s1", "s2"})
	tmeta.SetDescription("desc")
	tmeta.SetProducer("producer")
	tmeta.SetCreatorTool("tool")
	tmeta.SetKeywords([]string{"k1", "k2"})
	tmeta.SetCreateDate(time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC))
	tmeta.SetModifyDate(time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC))
	doc.SetParsedMetadata(tmeta)

	metadata := extractMetadata(doc)
	assert.Equal(t, map[string]string{
		"Title":       "t1",
		"Creator":     "c1",
		"Subject":     "s1, s2",
		"Description": "desc",
		"Producer":    "producer",
		"CreatorTool": "tool",
		"Keywords":    "k1, k2",
		"CreateDate":  "2026-02-16 00:00:00",
		"ModifyDate":  "2026-02-16 12:00:00",
	}, metadata)
}

func TestProcessPDF_MissingFile(t *testing.T) {
	err := processPDF("/tmp/does-not-exist.pdf", Options{})
	require.Error(t, err)
}

func TestProcessPDF_JsonOutput(t *testing.T) {
	out := captureStdout(t, func() {
		err := processPDF(testPDFPath(t), Options{JSON: true})
		require.NoError(t, err)
	})

	var info PDFInfo
	require.NoError(t, json.Unmarshal([]byte(out), &info))
	assert.Equal(t, "1.5", info.PDFVersion)
	assert.Greater(t, info.PageCount, 0)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	buf := &bytes.Buffer{}
	readDone := make(chan error, 1)
	go func() {
		_, readErr := io.Copy(buf, r)
		readDone <- readErr
	}()

	fn()
	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	require.NoError(t, <-readDone)
	require.NoError(t, r.Close())

	return buf.String()
}

func testPDFPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "..", "..", "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")
}
