package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestTextSearch_Basic(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	matches, err := doc.SearchText("Lorem", pdf.TextSearchOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	first := matches[0]
	assert.Equal(t, 0, first.PageIndex)
	assert.Equal(t, "Lorem", first.Text)
	assert.NotEmpty(t, first.Context)
	assert.Len(t, first.PgPoints, 8)
	assert.LessOrEqual(t, first.Bounds[0], first.Bounds[2])
	assert.LessOrEqual(t, first.Bounds[1], first.Bounds[3])
}

func TestTextSearch_CallbackFlow(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	started := 0
	finished := 0
	disposed := 0
	seenPages := 0
	progressCalls := 0

	matches, err := doc.SearchText("lorem", pdf.TextSearchOptions{
		CaseSensitive: false,
		OnStarted: func(totalPages int) {
			started++
			assert.GreaterOrEqual(t, totalPages, 1)
		},
		OnSearchedInPage: func(pageIndex int, pageMatches []pdf.TextSearchMatch) {
			seenPages++
			assert.GreaterOrEqual(t, pageIndex, 0)
			for _, m := range pageMatches {
				assert.Equal(t, pageIndex, m.PageIndex)
			}
		},
		OnProgress: func(donePages, totalPages int) {
			progressCalls++
			assert.GreaterOrEqual(t, donePages, 1)
			assert.GreaterOrEqual(t, totalPages, donePages)
		},
		OnFinished: func(totalMatches int) {
			finished++
			assert.Greater(t, totalMatches, 0)
		},
		OnDisposed: func() {
			disposed++
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	assert.Equal(t, 1, started)
	assert.Equal(t, 1, finished)
	assert.Equal(t, 1, disposed)
	assert.GreaterOrEqual(t, seenPages, 1)
	assert.GreaterOrEqual(t, progressCalls, 1)
}

func TestTextSearch_ResultSetParity(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	set, err := doc.SearchTextResultSet("lorem", pdf.TextSearchOptions{})
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.Equal(t, "lorem", set.Query)
	assert.GreaterOrEqual(t, set.SearchedPageCount(), 1)

	firstPageMatches := set.SearchedForPosition(0)
	require.NotEmpty(t, firstPageMatches)
	assert.NotEmpty(t, firstPageMatches[0].Text)
	assert.NotEmpty(t, set.FlattenedMatches())
}
