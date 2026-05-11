package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestBookmarkManage_AccessorsAndFind(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.mu.Lock()
	doc.outlinesSet = true
	doc.outlines = []*Outline{
		{
			Title:     "Root-A",
			PageIndex: 0,
			Color:     0xFFFF0000,
		},
		{
			Title:     "Root-B",
			PageIndex: 2,
			Color:     0xFF0000FF,
			Children: []*Outline{
				{
					Title:     "Child-B1",
					PageIndex: 1,
					Color:     0xFF00FF00,
				},
			},
		},
	}
	doc.mu.Unlock()

	count, err := doc.GetBookmarkCount()
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	title, err := doc.GetBookmarkTitle(1)
	require.NoError(t, err)
	assert.Equal(t, "Root-B", title)

	pageNo, err := doc.GetBookmarkPageNo(2)
	require.NoError(t, err)
	assert.Equal(t, 1, pageNo)

	color, err := doc.GetBookmarkColor(0)
	require.NoError(t, err)
	assert.Equal(t, 0xFFFF0000, color)

	index, err := doc.FindBookmarkByPage(1)
	require.NoError(t, err)
	assert.Equal(t, 2, index)
}

func TestBookmarkManage_AddRemoveAndClear(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0, 1, 2}

	require.NoError(t, doc.AddBookmark(1, "Added", 0x0000FF))
	require.NoError(t, doc.SetBookmarkTitle(0, "Updated"))
	require.NoError(t, doc.SetBookmarkColor(0, 0x00AA00))

	count, err := doc.GetBookmarkCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	title, err := doc.GetBookmarkTitle(0)
	require.NoError(t, err)
	assert.Equal(t, "Updated", title)

	color, err := doc.GetBookmarkColor(0)
	require.NoError(t, err)
	assert.Equal(t, 0xFF00AA00, color)

	require.NoError(t, doc.RemoveBookmark(0))
	count, err = doc.GetBookmarkCount()
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	require.NoError(t, doc.RemoveAllBookmark())
	count, err = doc.GetBookmarkCount()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestBookmarkManage_RemoveBookmark_FlattenedIndex(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.mu.Lock()
	doc.outlinesSet = true
	doc.outlines = []*Outline{
		{
			Title: "Root",
			Children: []*Outline{
				{Title: "Child"},
			},
		},
	}
	doc.mu.Unlock()

	require.NoError(t, doc.RemoveBookmark(1))

	outlines, err := doc.Outlines()
	require.NoError(t, err)
	require.Len(t, outlines, 1)
	assert.Len(t, outlines[0].Children, 0)
}

func TestBookmarkManage_ImportAndOutlineXML(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<bookmarks>
  <bookmark title="Root" page="1" color="#ff0000">
    <bookmark title="Child" page="2" color="#00ff00"></bookmark>
  </bookmark>
</bookmarks>
`

	path := filepath.Join(t.TempDir(), "bookmark.xml")
	require.NoError(t, os.WriteFile(path, []byte(xmlContent), 0o644))

	require.NoError(t, doc.ImportBookmark(path))

	count, err := doc.GetBookmarkCount()
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	rootPage, err := doc.GetBookmarkPageNo(0)
	require.NoError(t, err)
	assert.Equal(t, 0, rootPage)

	rootColor, err := doc.GetBookmarkColor(0)
	require.NoError(t, err)
	assert.Equal(t, 0xFFFF0000, rootColor)

	outlineXML, err := doc.GetOutlineXMLSL()
	require.NoError(t, err)
	assert.Contains(t, outlineXML, `<bookmark title="Root" page="1" color="#ff0000">`)
	assert.Contains(t, outlineXML, `<bookmark title="Child" page="2" color="#00ff00"></bookmark>`)
}

func TestBookmarkManage_InvalidInputs(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.pageOrder = []int{0}

	_, err := doc.GetBookmarkTitle(0)
	require.Error(t, err)

	require.Error(t, doc.AddBookmark(0, " ", 0))
	require.Error(t, doc.AddBookmark(3, "x", 0))
	require.Error(t, doc.RemoveBookmark(0))
	require.Error(t, doc.SetBookmarkTitle(0, "x"))
	require.Error(t, doc.SetBookmarkColor(0, 0x00AA00))
	require.Error(t, doc.ImportBookmark(" "))
}
