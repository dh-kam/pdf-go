package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestOutlineInstant_BasicQueries(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.mu.Lock()
	doc.outlinesSet = true
	doc.outlines = []*Outline{
		{
			Title:     "Chapter 1",
			PageIndex: 0,
			Children: []*Outline{
				{
					Title: "Link",
					Action: &OutlineAction{
						Type: "URI",
						URI:  "https://example.com",
					},
				},
			},
		},
		{
			Title:     "Chapter 2",
			PageIndex: 2,
		},
	}
	doc.mu.Unlock()

	count, err := doc.GetTopLevelOutlineCountSL()
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	kids, err := doc.InstantOutlineGetKids("")
	require.NoError(t, err)
	assert.Equal(t, []string{"0", "1"}, kids)

	title, err := doc.InstantOutlineGetTitle("0")
	require.NoError(t, err)
	assert.Equal(t, "Chapter 1", title)

	hasKids, err := doc.InstantOutlineHasKids("0")
	require.NoError(t, err)
	assert.True(t, hasKids)

	kids, err = doc.InstantOutlineGetKids("0")
	require.NoError(t, err)
	assert.Equal(t, []string{"0.0"}, kids)

	destPage, err := doc.InstantOutlineGetDestPage("0")
	require.NoError(t, err)
	assert.Equal(t, 0, destPage)

	destURI, err := doc.InstantOutlineGetDestURI("0.0")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", destURI)

	typ, err := doc.InstantOutlineGetType("0")
	require.NoError(t, err)
	assert.Equal(t, 1, typ)

	typ, err = doc.InstantOutlineGetType("0.0")
	require.NoError(t, err)
	assert.Equal(t, 4, typ)
}

func TestOutlineInstant_InvalidPath(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.mu.Lock()
	doc.outlinesSet = true
	doc.outlines = []*Outline{
		{Title: "A"},
	}
	doc.mu.Unlock()

	_, err := doc.InstantOutlineGetTitle("bad")
	require.Error(t, err)

	_, err = doc.InstantOutlineGetTitle("9")
	require.Error(t, err)

	_, err = doc.InstantOutlineGetType("")
	require.Error(t, err)
}
