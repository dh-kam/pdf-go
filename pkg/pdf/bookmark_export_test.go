package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestExportBookmark_WritesXML(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	doc.mu.Lock()
	doc.outlinesSet = true
	doc.outlines = []*Outline{
		{
			Title:     "Root",
			PageIndex: 0,
			Action: &OutlineAction{
				Type: "URI",
				URI:  "https://example.com",
			},
			Children: []*Outline{
				{
					Title:     "Child",
					PageIndex: 1,
				},
			},
		},
	}
	doc.mu.Unlock()

	path := filepath.Join(t.TempDir(), "bookmark.xml")
	require.NoError(t, doc.ExportBookmark(path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<bookmarks>")
	assert.Contains(t, string(data), "title=\"Root\"")
	assert.Contains(t, string(data), "title=\"Child\"")
	assert.Contains(t, string(data), "action=\"URI\"")
	assert.Contains(t, string(data), "uri=\"https://example.com\"")
}

func TestExportBookmark_EmptyPath(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	err := doc.ExportBookmark("  ")
	require.Error(t, err)
}
