package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestGetEmbeddedFontData_ReturnsDecodedFontFileStream(t *testing.T) {
	eval := NewEvaluator(nil)

	descriptor := entity.NewDict()
	descriptor.Set(entity.Name("FontFile2"), entity.NewStream(entity.NewDict(), []byte{1, 2, 3, 4}))

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("FontDescriptor"), descriptor)

	data, err := eval.getEmbeddedFontData(fontDict)
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3, 4}, data)
}

func TestGetEmbeddedFontData_ReturnsErrorWhenDescriptorMissing(t *testing.T) {
	eval := NewEvaluator(nil)

	fontDict := entity.NewDict()
	data, err := eval.getEmbeddedFontData(fontDict)
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "font descriptor missing")
}
