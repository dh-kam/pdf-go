package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestWordCount_Basic(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	count, err := doc.WordCount(0)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	countByJavaAlias, err := doc.GetWordCount(1)
	require.NoError(t, err)
	assert.Equal(t, count, countByJavaAlias)

	_, err = doc.GetWordCount(0)
	require.Error(t, err)
}
