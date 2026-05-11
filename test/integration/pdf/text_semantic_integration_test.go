package pdf_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestTextSemanticAPI_Basic(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	text, err := doc.Text(0)
	require.NoError(t, err)
	assert.Contains(t, text, "Lorem ipsum")

	lines, err := doc.TextLines(0)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	lineText := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Text != "" {
			lineText = append(lineText, line.Text)
		}
	}
	require.NotEmpty(t, lineText)
	assert.Contains(t, strings.Join(lineText, "\n"), "Lorem ipsum")

	paragraphs, err := doc.TextParagraphs(0)
	require.NoError(t, err)
	require.NotEmpty(t, paragraphs)
	assert.NotEmpty(t, paragraphs[0].Text)

	rangeText, err := doc.TextRange(0, 0, 12)
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(rangeText))
}

func TestTextSemanticAPI_InvalidInputs(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	_, err = doc.TextLines(-1)
	require.Error(t, err)

	_, err = doc.TextRange(0, 5, 1)
	require.Error(t, err)
}
