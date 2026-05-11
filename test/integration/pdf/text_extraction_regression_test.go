package pdf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infraText "github.com/dh-kam/pdf-go/internal/infrastructure/text"
	pdfusecase "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

func TestTextExtractionRegression_LibreOffice(t *testing.T) {
	pdfFile := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")
	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	text := extractFirstPageText(t, pdfFile)

	assert.Contains(t, text, "Lorem ipsum")
	assert.Contains(t, text, "dolor sit amet")
	assert.False(t, hasUnexpectedControl(text), "extracted text contains unexpected control characters")
}

func TestTextExtractionRegression_ArabicCMapVariants(t *testing.T) {
	base := filepath.Join(getSampleDir(), "015-arabic")
	habibiPath := filepath.Join(base, "habibi.pdf")
	onelinePath := filepath.Join(base, "habibi-oneline-cmap.pdf")

	if _, err := os.Stat(habibiPath); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", habibiPath)
		return
	}
	if _, err := os.Stat(onelinePath); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", onelinePath)
		return
	}

	habibiText := extractFirstPageText(t, habibiPath)
	onelineText := extractFirstPageText(t, onelinePath)

	assert.Contains(t, habibiText, "حَبيبي")
	assert.Contains(t, habibiText, "habibi")
	assert.Contains(t, onelineText, "حَبيبي")
	assert.Contains(t, onelineText, "habibi")
	assert.Equal(t, normalizeWhitespace(habibiText), normalizeWhitespace(onelineText))
}

func extractFirstPageText(t *testing.T, path string) string {
	t.Helper()

	doc, err := pdfusecase.Open(path)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.GetPage(0)
	require.NoError(t, err)

	extractor := infraText.NewExtractor()
	text, err := extractor.ExtractToText(page)
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(text))

	return text
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func hasUnexpectedControl(text string) bool {
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 {
			return true
		}
	}
	return false
}
