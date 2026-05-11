package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestLabelToPage_NoPageLabels(t *testing.T) {
	doc := newDocumentWithPageLabels(5, nil)
	assert.Equal(t, 0, doc.LabelToPage("1"))
}

func TestLabelToPage_DecimalAndPrefixRanges(t *testing.T) {
	labels := entity.NewDict()
	labels.Set(entity.Name("Nums"), entity.NewArray(
		entity.NewInteger(0),
		makeEntityDict(map[entity.Name]entity.Object{
			entity.Name("S"):  entity.NewName("D"),
			entity.Name("St"): entity.NewInteger(1),
		}),
		entity.NewInteger(3),
		makeEntityDict(map[entity.Name]entity.Object{
			entity.Name("S"):  entity.NewName("D"),
			entity.Name("P"):  entity.NewString("A-"),
			entity.Name("St"): entity.NewInteger(10),
		}),
	))

	doc := newDocumentWithPageLabels(6, labels)
	assert.Equal(t, 1, doc.LabelToPage("1"))
	assert.Equal(t, 3, doc.LabelToPage("3"))
	assert.Equal(t, 4, doc.LabelToPage("A-10"))
	assert.Equal(t, 6, doc.LabelToPage("A-12"))
	assert.Equal(t, 0, doc.LabelToPage("A-13"))
}

func TestLabelToPage_RomanAndAlphabetic(t *testing.T) {
	labels := entity.NewDict()
	labels.Set(entity.Name("Nums"), entity.NewArray(
		entity.NewInteger(0),
		makeEntityDict(map[entity.Name]entity.Object{
			entity.Name("S"): entity.NewName("r"),
		}),
		entity.NewInteger(2),
		makeEntityDict(map[entity.Name]entity.Object{
			entity.Name("S"):  entity.NewName("A"),
			entity.Name("P"):  entity.NewString("Sec-"),
			entity.Name("St"): entity.NewInteger(1),
		}),
	))

	doc := newDocumentWithPageLabels(5, labels)
	assert.Equal(t, 1, doc.LabelToPage("i"))
	assert.Equal(t, 2, doc.LabelToPage("ii"))
	assert.Equal(t, 3, doc.LabelToPage("Sec-A"))
	assert.Equal(t, 5, doc.LabelToPage("Sec-C"))
}

func TestLabelToPage_NumberTreeKids(t *testing.T) {
	kidA := entity.NewDict()
	kidA.Set(entity.Name("Nums"), entity.NewArray(
		entity.NewInteger(0),
		makeEntityDict(map[entity.Name]entity.Object{
			entity.Name("S"): entity.NewName("D"),
		}),
	))

	kidB := entity.NewDict()
	kidB.Set(entity.Name("Nums"), entity.NewArray(
		entity.NewInteger(2),
		makeEntityDict(map[entity.Name]entity.Object{
			entity.Name("S"): entity.NewName("a"),
		}),
	))

	labels := entity.NewDict()
	labels.Set(entity.Name("Kids"), entity.NewArray(kidA, kidB))

	doc := newDocumentWithPageLabels(4, labels)
	assert.Equal(t, 1, doc.LabelToPage("1"))
	assert.Equal(t, 3, doc.LabelToPage("a"))
	assert.Equal(t, 4, doc.LabelToPage("b"))
}

func TestGetWordCount_InvalidPageNumber(t *testing.T) {
	doc := newDocumentWithPageLabels(1, nil)
	_, err := doc.GetWordCount(0)
	require.Error(t, err)
}

func TestCountWordTokens(t *testing.T) {
	assert.Equal(t, 0, countWordTokens(""))
	assert.Equal(t, 0, countWordTokens("  \n\t "))
	assert.Equal(t, 2, countWordTokens("hello world"))
	assert.Equal(t, 3, countWordTokens("hello, world! again."))
	assert.Equal(t, 2, countWordTokens("don't stop"))
	assert.Equal(t, 2, countWordTokens("v2_model done"))
}

func newDocumentWithPageLabels(pageCount int, pageLabels entity.Object) *Document {
	entityDoc := entity.NewDocument(nil)

	pages := entity.NewDict()
	pages.Set(entity.Name("Type"), entity.NewName("Pages"))
	pages.Set(entity.Name("Count"), entity.NewInteger(int64(pageCount)))
	pages.Set(entity.Name("Kids"), entity.NewArray())

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.NewName("Catalog"))
	catalog.Set(entity.Name("Pages"), pages)
	if pageLabels != nil {
		catalog.Set(entity.Name("PageLabels"), pageLabels)
	}

	entityDoc.SetCatalog(catalog)
	return newDocument(entityDoc)
}

func makeEntityDict(items map[entity.Name]entity.Object) *entity.Dict {
	dict := entity.NewDict()
	for key, value := range items {
		dict.Set(key, value)
	}
	return dict
}
