package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestExecuteOutlineAction_SubmitAndReset(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	require.NoError(t, doc.SetFormFieldValue("Name", "Bob"))
	require.NoError(t, doc.SetFormFieldValue("Check", "Yes"))

	submitResult, err := doc.ExecuteOutlineAction(&pdf.OutlineAction{
		Type:       "SubmitForm",
		FieldNames: []string{"Name"},
	}, pdf.ActionExecutionOptions{})
	require.NoError(t, err)
	require.NotNil(t, submitResult.SubmittedFormData)
	assert.Equal(t, []string{"Bob"}, submitResult.SubmittedFormData.Fields["Name"])
	_, hasCheck := submitResult.SubmittedFormData.Fields["Check"]
	assert.False(t, hasCheck)

	resetResult, err := doc.ExecuteOutlineAction(&pdf.OutlineAction{
		Type:       "ResetForm",
		FieldNames: []string{"Name"},
	}, pdf.ActionExecutionOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, resetResult.ResetFieldCount)

	xfdf, err := doc.ExportFormDataXFDF()
	require.NoError(t, err)

	parsed, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"Yes"}, parsed.Fields["Check"])
	assert.NotEqual(t, []string{"Bob"}, parsed.Fields["Name"])
}

func TestExecuteOutlineAction_ImportDataXFDF(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	importPayload := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="Name"><value>Delta</value></field>
    <field name="Check"><value>Yes</value></field>
  </fields>
</xfdf>`)

	result, err := doc.ExecuteOutlineAction(&pdf.OutlineAction{
		Type: "ImportData",
	}, pdf.ActionExecutionOptions{
		ImportData: importPayload,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.AppliedFieldCount)

	fdf, err := doc.ExportFormDataFDF()
	require.NoError(t, err)

	parsed, err := pdf.ParseFormDataFDF(fdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"Delta"}, parsed.Fields["Name"])
	assert.Equal(t, []string{"Yes"}, parsed.Fields["Check"])
}
