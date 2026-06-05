package xref

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/repository"
)

func TestDecodeStreamData_NoFilter(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	raw := []byte("plain stream bytes")

	decoded, err := table.decodeStreamData(dict, raw)
	require.NoError(t, err)
	assert.Equal(t, raw, decoded)
}

func TestDecodeStreamData_FlateDecode(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))

	expected := []byte("decoded stream content")
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write(expected)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	decoded, err := table.decodeStreamData(dict, compressed.Bytes())
	require.NoError(t, err)
	assert.Equal(t, expected, decoded)
}

func TestDecodeStreamData_UnsupportedFilter(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("UnknownDecode"))

	_, err := table.decodeStreamData(dict, []byte("raw"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported filter")
}

func TestExtractXRefStreamDataPreservesLeadingNullStreamByte(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Type /XRef /Length 4 >>\n" +
			"stream\n" +
			"\x00ABC\n" +
			"endstream\n" +
			"endobj\n",
	)
	table := NewTable(data)
	dict := entity.NewDict()
	dict.Set(entity.Name("/Length"), entity.NewInteger(4))

	streamData, err := table.extractStreamDataFromOffset(0, dict)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 'A', 'B', 'C'}, streamData)
}

func TestExtractXRefStreamDataPreservesLeadingSpaceStreamByte(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Type /XRef /Length 4 >>\n" +
			"stream\n" +
			" ABC\n" +
			"endstream\n" +
			"endobj\n",
	)
	table := NewTable(data)
	dict := entity.NewDict()
	dict.Set(entity.Name("/Length"), entity.NewInteger(4))

	streamData, err := table.extractStreamDataFromOffset(0, dict)
	require.NoError(t, err)
	assert.Equal(t, []byte{' ', 'A', 'B', 'C'}, streamData)
}

func TestReadXRefStreamEntryDefaultsOmittedTypeToUncompressed(t *testing.T) {
	table := NewTable(nil)
	data := []byte{0x00, 0x00, 0x12, 0x34, 0x00, 0x02}
	pos := 0

	entry, size, err := table.readXRefStreamEntry(data, &pos, []int{0, 4, 2})

	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, repository.EntryTypeUncompressed, entry.Type)
	assert.False(t, entry.Free)
	assert.Equal(t, uint64(0x1234), entry.Offset)
	assert.Equal(t, uint16(2), entry.Generation)
	assert.Equal(t, 6, size)
	assert.Equal(t, 6, pos)
}

func TestParseXRefStreamWithPNGPredictor12CompressedPageTree(t *testing.T) {
	pdfData := buildCompressedPageTreePDF(t)
	table := NewTable(pdfData)

	require.NoError(t, table.Parse())

	catalog, err := table.GetCatalog()
	require.NoError(t, err)

	doc := entity.NewDocument(table)
	doc.SetCatalog(catalog)

	count, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	page, err := doc.GetPage(0)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.Equal(t, [4]float64{0, 0, 10, 10}, page.MediaBox())
}

func buildCompressedPageTreePDF(t *testing.T) []byte {
	t.Helper()

	obj2 := "<< /Type /Catalog /Pages 3 0 R >>"
	obj3 := "<< /Type /Pages /Count 1 /Kids [4 0 R] >>"
	obj4 := "<< /Type /Page /Parent 3 0 R /MediaBox [0 0 10 10] >>"
	header := fmt.Sprintf("2 0 3 %d 4 %d ", len(obj2), len(obj2)+len(obj3))
	objectStreamData := []byte(header + obj2 + obj3 + obj4)
	compressedObjectStream := zlibCompress(t, objectStreamData)

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n")
	obj1Offset := pdf.Len()
	fmt.Fprintf(&pdf, "1 0 obj\n<< /Type /ObjStm /N 3 /First %d /Length %d /Filter /FlateDecode >>\nstream\n", len(header), len(compressedObjectStream))
	pdf.Write(compressedObjectStream)
	pdf.WriteString("\nendstream\nendobj\n")

	xrefOffset := pdf.Len()
	xrefRows := buildPredictor12UpXRefRows([]xrefStreamTestEntry{
		{typ: 0, field2: 0, field3: 65535},
		{typ: 1, field2: uint32(obj1Offset), field3: 0},
		{typ: 2, field2: 1, field3: 0},
		{typ: 2, field2: 1, field3: 1},
		{typ: 2, field2: 1, field3: 2},
		{typ: 1, field2: uint32(xrefOffset), field3: 0},
	})
	compressedXRef := zlibCompress(t, xrefRows)
	fmt.Fprintf(&pdf, "5 0 obj\n<< /Type /XRef /Size 6 /Root 2 0 R /W [1 4 2] /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 7 >> /Length %d >>\nstream\n", len(compressedXRef))
	pdf.Write(compressedXRef)
	fmt.Fprintf(&pdf, "\nendstream\nendobj\nstartxref\n%d\n%%EOF\n", xrefOffset)

	return pdf.Bytes()
}

type xrefStreamTestEntry struct {
	typ    byte
	field2 uint32
	field3 uint16
}

func buildPredictor12UpXRefRows(entries []xrefStreamTestEntry) []byte {
	var out bytes.Buffer
	prev := make([]byte, 7)
	for _, entry := range entries {
		row := make([]byte, 7)
		row[0] = entry.typ
		var field2 [4]byte
		binary.BigEndian.PutUint32(field2[:], entry.field2)
		copy(row[1:5], field2[:])
		var field3 [2]byte
		binary.BigEndian.PutUint16(field3[:], entry.field3)
		copy(row[5:7], field3[:])

		out.WriteByte(2) // PNG filter byte: Up.
		for i, value := range row {
			out.WriteByte(value - prev[i])
		}
		copy(prev, row)
	}
	return out.Bytes()
}

func zlibCompress(t *testing.T, data []byte) []byte {
	t.Helper()

	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return compressed.Bytes()
}
