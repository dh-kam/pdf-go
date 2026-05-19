package renderer

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveImageDecodeParmsExtractsJBIG2GlobalsStream(t *testing.T) {
	evaluator := &Evaluator{}
	paramsDict := entity.NewDict()
	paramsDict.Set(entity.Name("JBIG2Globals"), entity.NewStream(entity.NewDict(), []byte{0x01, 0x02, 0x03}))

	params := evaluator.resolveImageDecodeParms(paramsDict, 0)

	require.NotNil(t, params)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, params["JBIG2Globals"])
}

func TestResolveImageDecodeParmsSelectsFilterArrayEntry(t *testing.T) {
	evaluator := &Evaluator{}
	paramsDict := entity.NewDict()
	paramsDict.Set(entity.Name("JBIG2Globals"), entity.NewStream(entity.NewDict(), []byte{0x04, 0x05}))
	paramsArray := entity.NewArray(entity.NewNull(), paramsDict)

	params := evaluator.resolveImageDecodeParms(paramsArray, 1)

	require.NotNil(t, params)
	assert.Equal(t, []byte{0x04, 0x05}, params["JBIG2Globals"])
}
