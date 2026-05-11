package pdf_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestFormDataExportImport_XFDF(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	xfdf, err := doc.ExportFormDataXFDF()
	require.NoError(t, err)
	require.NotEmpty(t, xfdf)

	data, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	require.NotNil(t, data)

	require.Contains(t, data.Fields, "Name")
	require.Contains(t, data.Fields, "Check")
	assert.Equal(t, []string{"Off"}, data.Fields["Check"])
}

func TestFormDataExportImport_FDF(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	fdf, err := doc.ExportFormDataFDF()
	require.NoError(t, err)
	require.NotEmpty(t, fdf)

	data, err := pdf.ParseFormDataFDF(fdf)
	require.NoError(t, err)
	require.NotNil(t, data)

	require.Contains(t, data.Fields, "Name")
	require.Contains(t, data.Fields, "Check")
	assert.Equal(t, []string{"Off"}, data.Fields["Check"])
}

func TestImportFormDataXFDF_ApplyToSession(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="Name"><value>Bob</value></field>
    <field name="Check"><value>Yes</value></field>
    <field name="Unknown"><value>ignored</value></field>
  </fields>
</xfdf>`)

	applied, err := doc.ImportFormDataXFDF(input)
	require.NoError(t, err)
	assert.Equal(t, 2, applied)

	xfdf, err := doc.ExportFormDataXFDF()
	require.NoError(t, err)

	parsed, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"Bob"}, parsed.Fields["Name"])
	assert.Equal(t, []string{"Yes"}, parsed.Fields["Check"])
}

func TestImportFormDataFDF_ApplyToSession(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	fdf := buildTestFDF(map[string][]string{
		"Name":  {"Charlie"},
		"Check": {"Yes"},
	})

	applied, err := doc.ImportFormDataFDF(fdf)
	require.NoError(t, err)
	assert.Equal(t, 2, applied)

	exported, err := doc.ExportFormDataFDF()
	require.NoError(t, err)

	parsed, err := pdf.ParseFormDataFDF(exported)
	require.NoError(t, err)
	assert.Equal(t, []string{"Charlie"}, parsed.Fields["Name"])
	assert.Equal(t, []string{"Yes"}, parsed.Fields["Check"])
}

func TestSetFormFieldValue_UnknownField(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	err = doc.SetFormFieldValue("UnknownField", "x")
	require.Error(t, err)
}

func buildTestFDF(fields map[string][]string) []byte {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	objectCount := len(names) + 1
	objects := make([]string, objectCount+1)
	refs := make([]string, 0, len(names))
	for i, name := range names {
		objNum := i + 2
		refs = append(refs, fmt.Sprintf("%d 0 R", objNum))
		objects[objNum] = buildTestFDFFieldObject(name, fields[name])
	}
	objects[1] = fmt.Sprintf("<< /Type /Catalog /FDF << /Fields [%s] >> >>", strings.Join(refs, " "))

	var buf bytes.Buffer
	buf.WriteString("%FDF-1.2\n")
	buf.WriteString("%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, objectCount+1)
	for i := 1; i <= objectCount; i++ {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i, objects[i])
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", objectCount+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= objectCount; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Root 1 0 R /Size %d >>\n", objectCount+1)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return buf.Bytes()
}

func buildTestFDFFieldObject(name string, values []string) string {
	if len(values) == 0 {
		values = []string{""}
	}

	var sb strings.Builder
	sb.WriteString("<< /T ")
	sb.WriteString(testLiteral(name))
	sb.WriteString(" /V ")
	if len(values) == 1 {
		sb.WriteString(testLiteral(values[0]))
	} else {
		sb.WriteString("[")
		for i, v := range values {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(testLiteral(v))
		}
		sb.WriteString("]")
	}
	sb.WriteString(" >>")
	return sb.String()
}

func testLiteral(value string) string {
	var sb strings.Builder
	sb.WriteByte('(')
	for _, r := range value {
		switch r {
		case '\\':
			sb.WriteString("\\\\")
		case '(':
			sb.WriteString("\\(")
		case ')':
			sb.WriteString("\\)")
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte(')')
	return sb.String()
}
