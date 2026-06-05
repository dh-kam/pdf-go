package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestParseShadingFunctionResolvesIndirectRefs(t *testing.T) {
	functionDict := entity.NewDict()
	functionDict.Set(entity.Name("FunctionType"), entity.NewInteger(2))
	functionDict.Set(entity.Name("C0"), entity.NewArray(entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(0)))
	functionDict.Set(entity.Name("C1"), entity.NewArray(entity.NewInteger(1), entity.NewInteger(1), entity.NewInteger(1)))
	functionDict.Set(entity.Name("N"), entity.NewInteger(1))
	functionRef := entity.NewRef(10, 0)

	e := NewEvaluator(&testMapXRef{
		objects: map[entity.Ref]entity.Object{
			functionRef: functionDict,
		},
	})

	fn, err := e.parseShadingFunctionObject(functionRef)
	require.NoError(t, err)
	require.IsType(t, &entity.ExponentialFunction{}, fn)

	functions, err := e.parseShadingFunctionList(entity.NewArray(functionRef))
	require.NoError(t, err)
	require.Len(t, functions, 1)
	require.IsType(t, &entity.ExponentialFunction{}, functions[0])
}

func TestParseShadingResolvesResourceColorSpaceMapper(t *testing.T) {
	colorSpaces := entity.NewDict()
	colorSpaces.Set(entity.Name("CS1"), entity.Name("DeviceCMYK"))
	resources := entity.NewDict()
	resources.Set(entity.Name("ColorSpace"), colorSpaces)

	e := NewEvaluator(nil)
	e.resources = resources

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("CS1"))
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(1),
		entity.NewInteger(0),
	))

	shading, err := e.parseShading(shadingDict)
	require.NoError(t, err)
	require.Equal(t, "DeviceCMYK", shading.GetColorSpace())
	require.NotNil(t, shading.GetColorMapper())
	require.Equal(t, 4, shading.GetColorMapper().GetNumComponents())
}
