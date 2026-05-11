package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestExportPageAnnotationsToFDF_WritesFDF(t *testing.T) {
	doc := newDocumentWithAnnotationPages([]annotationPageSpec{
		{
			annotations: []annotationSpec{
				{
					typ:      "Text",
					contents: "hello note",
					rect:     [4]float64{1, 2, 3, 4},
				},
			},
		},
		{
			annotations: nil,
		},
	})

	path := filepath.Join(t.TempDir(), "annots.fdf")
	require.NoError(t, doc.ExportPageAnnotationsToFDF([]int{0}, path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "%FDF-1.2")
	assert.Contains(t, text, "/Subtype /Text")
	assert.Contains(t, text, "/Page 0")
	assert.Contains(t, text, "(hello note)")
}

func TestExportPageAnnotationsToFDF_EmptyPages(t *testing.T) {
	doc := newDocumentWithAnnotationPages(nil)
	err := doc.ExportPageAnnotationsToFDF(nil, filepath.Join(t.TempDir(), "x.fdf"))
	require.Error(t, err)
}

type annotationPageSpec struct {
	annotations []annotationSpec
}

type annotationSpec struct {
	contents string
	typ      string
	rect     [4]float64
}

func newDocumentWithAnnotationPages(specs []annotationPageSpec) *Document {
	entityDoc := entity.NewDocument(nil)

	kids := make([]entity.Object, 0, len(specs))
	for _, pageSpec := range specs {
		page := entity.NewDict()
		page.Set(entity.Name("Type"), entity.NewName("Page"))
		page.Set(entity.Name("MediaBox"), entity.NewArray(
			entity.NewReal(0),
			entity.NewReal(0),
			entity.NewReal(100),
			entity.NewReal(100),
		))

		annotItems := make([]entity.Object, 0, len(pageSpec.annotations))
		for _, annotSpec := range pageSpec.annotations {
			annot := entity.NewDict()
			annot.Set(entity.Name("Subtype"), entity.NewName(annotSpec.typ))
			annot.Set(entity.Name("Contents"), entity.NewString(annotSpec.contents))
			annot.Set(entity.Name("Rect"), entity.NewArray(
				entity.NewReal(annotSpec.rect[0]),
				entity.NewReal(annotSpec.rect[1]),
				entity.NewReal(annotSpec.rect[2]),
				entity.NewReal(annotSpec.rect[3]),
			))
			annotItems = append(annotItems, annot)
		}
		page.Set(entity.Name("Annots"), entity.NewArray(annotItems...))
		kids = append(kids, page)
	}

	pages := entity.NewDict()
	pages.Set(entity.Name("Type"), entity.NewName("Pages"))
	pages.Set(entity.Name("Count"), entity.NewInteger(int64(len(specs))))
	pages.Set(entity.Name("Kids"), entity.NewArray(kids...))

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.NewName("Catalog"))
	catalog.Set(entity.Name("Pages"), pages)

	entityDoc.SetCatalog(catalog)
	return newDocument(entityDoc)
}
