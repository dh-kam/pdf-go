package renderer

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceStackLookupFallsBackToParentAndShadows(t *testing.T) {
	parentFont := entity.NewDict()
	parentValue := entity.NewName("ParentFont")
	parentFont.Set(entity.Name("F1"), parentValue)
	parent := entity.NewDict()
	parent.Set(entity.Name("Font"), parentFont)

	childFont := entity.NewDict()
	childValue := entity.NewName("ChildFont")
	childFont.Set(entity.Name("F1"), childValue)
	child := entity.NewDict()
	child.Set(entity.Name("Font"), childFont)

	e := NewEvaluator(nil)
	e.SetResourceStack([]*entity.Dict{child, parent})

	assert.Equal(t, childValue, e.getResourceEntry(entity.Name("Font"), entity.Name("F1")))

	e.SetResourceStack([]*entity.Dict{nil, parent})
	assert.Equal(t, parentValue, e.getResourceEntry(entity.Name("Font"), entity.Name("F1")))
}

func TestResolvePatternParsesShadingWithMatchedParentResources(t *testing.T) {
	parentColorSpaces := entity.NewDict()
	parentColorSpaces.Set(entity.Name("CS1"), entity.Name("DeviceGray"))
	parentPatterns := entity.NewDict()
	parentPatterns.Set(entity.Name("P1"), newTestShadingPatternDict(entity.Name("CS1")))
	parent := entity.NewDict()
	parent.Set(entity.Name("ColorSpace"), parentColorSpaces)
	parent.Set(entity.Name("Pattern"), parentPatterns)

	childColorSpaces := entity.NewDict()
	childColorSpaces.Set(entity.Name("CS1"), entity.Name("DeviceRGB"))
	child := entity.NewDict()
	child.Set(entity.Name("ColorSpace"), childColorSpaces)

	e := NewEvaluator(nil)
	e.SetResourceStack([]*entity.Dict{child, parent})

	pattern, err := e.resolvePattern(entity.Name("P1"))
	require.NoError(t, err)
	shadingPattern, ok := pattern.(*entity.ShadingPattern)
	require.True(t, ok)
	require.NotNil(t, shadingPattern.GetShading())
	assert.Equal(t, "DeviceGray", shadingPattern.GetShading().GetColorSpace())
}

func TestPatternForCanvasCarriesTilingResourceStack(t *testing.T) {
	parent := entity.NewDict()
	child := entity.NewDict()
	patternResources := entity.NewDict()

	pattern := entity.NewTilingPattern("P1", 1, entity.TilingConstantSpacing)
	pattern.SetResources(patternResources)

	e := NewEvaluator(nil)
	e.SetResourceStack([]*entity.Dict{child, parent})

	cloned, ok := e.patternForCanvas(pattern).(*entity.TilingPattern)
	require.True(t, ok)

	stack := cloned.GetResourceStack()
	require.Len(t, stack, 3)
	assert.Same(t, patternResources, stack[0])
	assert.Same(t, child, stack[1])
	assert.Same(t, parent, stack[2])
}

func newTestShadingPatternDict(colorSpace entity.Object) *entity.Dict {
	shading := entity.NewDict()
	shading.Set(entity.Name("ShadingType"), entity.NewInteger(2))
	shading.Set(entity.Name("ColorSpace"), colorSpace)
	shading.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(10),
		entity.NewInteger(0),
	))

	pattern := entity.NewDict()
	pattern.Set(entity.Name("PatternType"), entity.NewInteger(2))
	pattern.Set(entity.Name("Shading"), shading)
	return pattern
}
