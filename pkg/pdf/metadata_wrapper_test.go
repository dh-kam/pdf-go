package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/metadata"
)

func TestMetadataAndWrappers(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	assert.Nil(t, doc.GetMetadata())

	meta := metadata.NewMetadata("<xmp>sample</xmp>")
	meta.SetTitle([]string{"Sample Title"})
	meta.SetCreator([]string{"Alice"})
	meta.SetSubject([]string{"Integration"})
	meta.SetDescription("A small description")
	meta.SetCreatorTool("go-pdf")
	meta.SetProducer("GoPDF")
	meta.SetKeywords([]string{"pdf", "test"})

	entityDoc := entity.NewDocument(nil)
	entityDoc.SetParsedMetadata(meta)
	withMetadata := newDocument(entityDoc)

	docMetadata := withMetadata.GetMetadata()
	require.NotNil(t, docMetadata, "parsed metadata should be wrapped")
	assert.Equal(t, []string{"Sample Title"}, docMetadata.Title())
	assert.Equal(t, []string{"Alice"}, docMetadata.Author())
	assert.Equal(t, []string{"Integration"}, docMetadata.Subject())
	assert.Equal(t, "A small description", docMetadata.Description())
	assert.Equal(t, "go-pdf", docMetadata.CreatorTool())
	assert.Equal(t, "GoPDF", docMetadata.Producer())
	assert.Equal(t, []string{"pdf", "test"}, docMetadata.Keywords())

	var nilMeta *Metadata
	assert.Nil(t, nilMeta.Title())
	assert.Empty(t, nilMeta.Keywords())
	assert.Equal(t, "", nilMeta.Description())
}
