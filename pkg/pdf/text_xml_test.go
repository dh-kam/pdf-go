package pdf

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPageTextAsXMLSL_EmptyPage(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 100}, rotate: 0},
	})

	out, err := doc.GetPageTextAsXMLSL(0)
	require.NoError(t, err)
	assert.Contains(t, out, "<?xml")
	assert.Contains(t, out, "<page")

	var parsed pageTextXMLDoc
	require.NoError(t, xml.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, 0, parsed.Index)
}

func TestGetPageTextAsXMLSL_InvalidPage(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 100}, rotate: 0},
	})

	_, err := doc.GetPageTextAsXMLSL(10)
	require.Error(t, err)
}
