package colorspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestRegistry_ParseColorSpace_BasicBranches(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("DeviceRGB")
	assert.True(t, ok)
	r.Register("CustomGray", NewDeviceGray())
	custom, ok := r.Get("CustomGray")
	assert.True(t, ok)
	assert.Equal(t, "DeviceGray", custom.Name())

	cs, err := r.ParseColorSpace(entity.Name("DeviceRGB"))
	require.NoError(t, err)
	assert.Equal(t, "DeviceRGB", cs.Name())

	_, err = r.ParseColorSpace(nil)
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.Name("UnknownColorSpace"))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray())
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(entity.NewInteger(1)))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(entity.NewName("UnknownType")))
	assert.Error(t, err)
}

func TestRegistry_ParseColorSpace_PatternAndICC(t *testing.T) {
	r := NewRegistry()

	// Pattern without base.
	patternOnly, err := r.ParseColorSpace(entity.NewArray(entity.NewName("Pattern")))
	require.NoError(t, err)
	require.IsType(t, &PatternColorSpace{}, patternOnly)
	assert.False(t, patternOnly.(*PatternColorSpace).IsUncolored())

	// Pattern with base color space (uncolored pattern).
	patternWithBase, err := r.ParseColorSpace(entity.NewArray(
		entity.NewName("Pattern"),
		entity.NewName("DeviceCMYK"),
	))
	require.NoError(t, err)
	require.IsType(t, &PatternColorSpace{}, patternWithBase)
	assert.True(t, patternWithBase.(*PatternColorSpace).IsUncolored())
	assert.Equal(t, "DeviceCMYK", patternWithBase.(*PatternColorSpace).GetBaseColorSpace().Name())

	_, err = r.ParseColorSpace(entity.NewArray(entity.NewName("ICCBased")))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(entity.NewName("ICCBased"), entity.NewInteger(1)))
	assert.Error(t, err)

	iccDict := entity.NewDict()
	iccDict.Set(entity.NewName("Alternate"), entity.NewName("DeviceCMYK"))
	iccStream := entity.NewStream(iccDict, []byte{0x00})
	iccCS, err := r.ParseColorSpace(entity.NewArray(entity.NewName("ICCBased"), iccStream))
	require.NoError(t, err)
	assert.Equal(t, "DeviceCMYK", iccCS.Name())
}

func TestRegistry_ParseColorSpace_IndexedAndSimpleCIE(t *testing.T) {
	r := NewRegistry()

	_, err := r.ParseColorSpace(entity.NewArray(entity.NewName("Indexed")))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(
		entity.NewName("Indexed"),
		entity.NewName("UnknownBase"),
		entity.NewInteger(1),
		entity.NewString("\x00"),
	))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(
		entity.NewName("Indexed"),
		entity.NewName("DeviceRGB"),
		entity.NewName("bad-hival"),
		entity.NewString("\x00"),
	))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(
		entity.NewName("Indexed"),
		entity.NewName("DeviceRGB"),
		entity.NewInteger(1),
		entity.NewInteger(7),
	))
	assert.Error(t, err)

	indexedByString, err := r.ParseColorSpace(entity.NewArray(
		entity.NewName("Indexed"),
		entity.NewName("DeviceRGB"),
		entity.NewInteger(1),
		entity.NewString(string([]byte{0, 0, 0, 255, 255, 255})),
	))
	require.NoError(t, err)
	require.IsType(t, &IndexedColorSpace{}, indexedByString)
	assert.Equal(t, "Indexed", indexedByString.Name())

	streamLookup := entity.NewStream(entity.NewDict(), []byte{10, 20, 30, 40, 50, 60})
	indexedByStream, err := r.ParseColorSpace(entity.NewArray(
		entity.NewName("Indexed"),
		entity.NewName("DeviceRGB"),
		entity.NewInteger(1),
		streamLookup,
	))
	require.NoError(t, err)
	require.IsType(t, &IndexedColorSpace{}, indexedByStream)

	devN, err := r.ParseColorSpace(entity.NewArray(entity.NewName("DeviceN")))
	require.NoError(t, err)
	assert.Equal(t, "DeviceRGB", devN.Name())

	calGray, err := r.ParseColorSpace(entity.NewArray(entity.NewName("CalGray")))
	require.NoError(t, err)
	assert.Equal(t, "DeviceGray", calGray.Name())

	calRGB, err := r.ParseColorSpace(entity.NewArray(entity.NewName("CalRGB")))
	require.NoError(t, err)
	assert.Equal(t, "DeviceRGB", calRGB.Name())

	lab, err := r.ParseColorSpace(entity.NewArray(entity.NewName("Lab")))
	require.NoError(t, err)
	assert.Equal(t, "DeviceRGB", lab.Name())
}

func TestRegistry_ParseColorSpace_Separation(t *testing.T) {
	r := NewRegistry()

	_, err := r.ParseColorSpace(entity.NewArray(entity.NewName("Separation")))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(
		entity.NewName("Separation"),
		entity.NewInteger(1),
		entity.NewName("DeviceRGB"),
		entity.NewDict(),
	))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(
		entity.NewName("Separation"),
		entity.NewName("Spot"),
		entity.NewName("UnknownAlt"),
		entity.NewDict(),
	))
	assert.Error(t, err)

	_, err = r.ParseColorSpace(entity.NewArray(
		entity.NewName("Separation"),
		entity.NewName("Spot"),
		entity.NewName("DeviceRGB"),
		entity.NewInteger(42),
	))
	assert.Error(t, err)

	fnDict := entity.NewDict()
	fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(2))
	fnDict.Set(entity.NewName("N"), entity.NewReal(1.0))
	fnDict.Set(entity.NewName("C0"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(0)))
	fnDict.Set(entity.NewName("C1"), entity.NewArray(entity.NewReal(1), entity.NewReal(0), entity.NewReal(0)))
	fnDict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))

	sepCS, err := r.ParseColorSpace(entity.NewArray(
		entity.NewName("Separation"),
		entity.NewName("SpotRed"),
		entity.NewName("DeviceRGB"),
		fnDict,
	))
	require.NoError(t, err)
	require.IsType(t, &SeparationColorSpace{}, sepCS)
	assert.Equal(t, "SpotRed", sepCS.Name())
}

func TestParseFunctionFromObject_Branches(t *testing.T) {
	_, err := parseFunctionFromObject(nil)
	assert.Error(t, err)

	_, err = parseFunctionFromObject(entity.NewInteger(1))
	assert.Error(t, err)

	noType := entity.NewDict()
	_, err = parseFunctionFromObject(noType)
	assert.Error(t, err)

	invalidType := entity.NewDict()
	invalidType.Set(entity.NewName("FunctionType"), entity.NewName("bad"))
	_, err = parseFunctionFromObject(invalidType)
	assert.Error(t, err)

	unsupported := entity.NewDict()
	unsupported.Set(entity.NewName("FunctionType"), entity.NewInteger(9))
	_, err = parseFunctionFromObject(unsupported)
	assert.Error(t, err)

	type2 := entity.NewDict()
	type2.Set(entity.NewName("FunctionType"), entity.NewInteger(2))
	type2.Set(entity.NewName("C0"), entity.NewArray(entity.NewInteger(1)))
	type2.Set(entity.NewName("C1"), entity.NewArray(entity.NewReal(3)))
	type2.Set(entity.NewName("N"), entity.NewInteger(2))
	type2.Set(entity.NewName("Domain"), entity.NewArray(entity.NewInteger(0), entity.NewInteger(2)))

	fn2, err := parseFunctionFromObject(type2)
	require.NoError(t, err)
	require.IsType(t, &entity.ExponentialFunction{}, fn2)

	subFn := entity.NewDict()
	subFn.Set(entity.NewName("FunctionType"), entity.NewInteger(2))
	subFn.Set(entity.NewName("N"), entity.NewReal(1))
	type3 := entity.NewDict()
	type3.Set(entity.NewName("FunctionType"), entity.NewInteger(3))
	type3.Set(entity.NewName("Functions"), entity.NewArray(subFn))
	type3.Set(entity.NewName("Bounds"), entity.NewArray(entity.NewReal(0.5)))
	type3.Set(entity.NewName("Encode"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	type3.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))

	fn3, err := parseFunctionFromObject(type3)
	require.NoError(t, err)
	require.IsType(t, &entity.StitchingFunction{}, fn3)

	type0Dict := entity.NewDict()
	type0Dict.Set(entity.NewName("FunctionType"), entity.NewInteger(0))
	type0Dict.Set(entity.NewName("Size"), entity.NewArray(entity.NewInteger(2)))
	type0Dict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewInteger(0), entity.NewInteger(1)))
	type0Dict.Set(entity.NewName("Range"), entity.NewArray(entity.NewInteger(0), entity.NewInteger(1)))
	type0Stream := entity.NewStream(type0Dict, []byte{0, 255})

	fn0, err := parseFunctionFromObject(type0Stream)
	require.NoError(t, err)
	require.IsType(t, &entity.SampledFunction{}, fn0)

	type4Dict := entity.NewDict()
	type4Dict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
	type4Dict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	type4Dict.Set(entity.NewName("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	type4Stream := entity.NewStream(type4Dict, []byte("{ dup }"))

	fn4, err := parseFunctionFromObject(type4Stream)
	require.NoError(t, err)
	require.IsType(t, &entity.PostScriptFunction{}, fn4)
}

func TestParseSampleValues_MapsToUnitRange(t *testing.T) {
	values := parseSampleValues([]byte{0, 127, 255})
	require.Len(t, values, 3)
	assert.InDelta(t, 0.0, values[0], 1e-9)
	assert.InDelta(t, 127.0/255.0, values[1], 1e-9)
	assert.InDelta(t, 1.0, values[2], 1e-9)
}
