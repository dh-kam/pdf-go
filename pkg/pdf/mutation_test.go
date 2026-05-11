package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestPageMutation_InsertMoveRemove(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0, 1, 2}

	require.NoError(t, doc.InsertPage(1, 2))
	assert.Equal(t, []int{0, 2, 1, 2}, doc.PageOrder())

	require.NoError(t, doc.MovePage(3, 0))
	assert.Equal(t, []int{2, 0, 2, 1}, doc.PageOrder())

	require.NoError(t, doc.RemovePage(2))
	assert.Equal(t, []int{2, 0, 1}, doc.PageOrder())
}

func TestPageMutation_InvalidInput(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0, 1}

	require.Error(t, doc.InsertPage(-1, 0))
	require.Error(t, doc.InsertPage(0, 5))
	require.Error(t, doc.MovePage(0, 5))
	require.Error(t, doc.RemovePage(5))
}

func TestOutlineMutation_AddAndRemove(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	require.NoError(t, doc.AddOutline(nil, &Outline{Title: "Root", PageIndex: 0}))
	require.NoError(t, doc.AddOutline([]int{0}, &Outline{Title: "Child", PageIndex: 1}))

	outlines, err := doc.Outlines()
	require.NoError(t, err)
	require.Len(t, outlines, 1)
	assert.Equal(t, "Root", outlines[0].Title)
	require.Len(t, outlines[0].Children, 1)
	assert.Equal(t, "Child", outlines[0].Children[0].Title)

	require.NoError(t, doc.RemoveOutline([]int{0, 0}))
	outlines, err = doc.Outlines()
	require.NoError(t, err)
	require.Len(t, outlines, 1)
	assert.Len(t, outlines[0].Children, 0)
}

func TestOutlineMutation_InvalidInput(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))

	require.Error(t, doc.AddOutline(nil, nil))
	require.Error(t, doc.RemoveOutline(nil))
	require.Error(t, doc.RemoveOutline([]int{0}))
}
