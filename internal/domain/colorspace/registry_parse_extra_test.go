package colorspace

import (
	"encoding/binary"
	"image/color"
	"math"
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
	iccDict.Set(entity.NewName("N"), entity.NewInteger(4))
	iccDict.Set(entity.NewName("Alternate"), entity.NewName("DeviceCMYK"))
	iccStream := entity.NewStream(iccDict, []byte{0x00})
	iccCS, err := r.ParseColorSpace(entity.NewArray(entity.NewName("ICCBased"), iccStream))
	require.NoError(t, err)
	assert.Equal(t, ColorSpaceICCBased, iccCS.Type())
	assert.Equal(t, 4, iccCS.GetNumComponents())
	assert.Equal(t, NewDeviceCMYK().ConvertToRGBA([]float64{0, 0, 0, 1}), iccCS.ConvertToRGBA([]float64{0, 0, 0, 1}))
}

func TestICCBasedColorSpace_RGBMatrixTRCProfile(t *testing.T) {
	t.Setenv("PDF_ENABLE_ICC_MATRIX_TRC", "1")

	r := NewRegistry()
	dict := entity.NewDict()
	dict.Set(entity.NewName("N"), entity.NewInteger(3))
	dict.Set(entity.NewName("Alternate"), entity.NewName("DeviceRGB"))
	stream := entity.NewStream(dict, buildTestRGBMatrixICCProfile())

	cs, err := r.ParseColorSpace(entity.NewArray(entity.NewName("ICCBased"), stream))
	require.NoError(t, err)
	require.Equal(t, ColorSpaceICCBased, cs.Type())

	red := cs.ConvertToRGBA([]float64{1, 0, 0})
	assert.Equal(t, color.RGBA{R: 255, G: 0, B: 0, A: 255}, red)

	mid := cs.ConvertToRGBA([]float64{0.5, 0.5, 0.5})
	assert.InDelta(t, 128, int(mid.R), 1)
	assert.InDelta(t, 128, int(mid.G), 1)
	assert.InDelta(t, 128, int(mid.B), 1)
}

func buildTestRGBMatrixICCProfile() []byte {
	tags := []struct {
		sig  string
		data []byte
	}{
		{sig: "rXYZ", data: buildTestXYZTag(0.4124564, 0.2126729, 0.0193339)},
		{sig: "gXYZ", data: buildTestXYZTag(0.3575761, 0.7151522, 0.1191920)},
		{sig: "bXYZ", data: buildTestXYZTag(0.1804375, 0.0721750, 0.9503041)},
		{sig: "rTRC", data: buildTestCurveGammaTag(2.2)},
		{sig: "gTRC", data: buildTestCurveGammaTag(2.2)},
		{sig: "bTRC", data: buildTestCurveGammaTag(2.2)},
		{sig: "wtpt", data: buildTestXYZTag(0.95047, 1.0, 1.08883)},
	}

	tagTableOffset := 128
	tagRecordOffset := tagTableOffset + 4
	tagDataOffset := tagRecordOffset + len(tags)*12
	total := tagDataOffset
	for _, tag := range tags {
		total += len(tag.data)
	}

	profile := make([]byte, total)
	binary.BigEndian.PutUint32(profile[0:4], uint32(total))
	copy(profile[16:20], "RGB ")
	copy(profile[20:24], "XYZ ")
	copy(profile[36:40], "acsp")
	binary.BigEndian.PutUint32(profile[tagTableOffset:tagRecordOffset], uint32(len(tags)))

	offset := tagDataOffset
	for i, tag := range tags {
		record := tagRecordOffset + i*12
		copy(profile[record:record+4], tag.sig)
		binary.BigEndian.PutUint32(profile[record+4:record+8], uint32(offset))
		binary.BigEndian.PutUint32(profile[record+8:record+12], uint32(len(tag.data)))
		copy(profile[offset:offset+len(tag.data)], tag.data)
		offset += len(tag.data)
	}
	return profile
}

func buildTestXYZTag(x, y, z float64) []byte {
	tag := make([]byte, 20)
	copy(tag[0:4], "XYZ ")
	writeTestS15Fixed16(tag[8:12], x)
	writeTestS15Fixed16(tag[12:16], y)
	writeTestS15Fixed16(tag[16:20], z)
	return tag
}

func buildTestCurveGammaTag(gamma float64) []byte {
	tag := make([]byte, 14)
	copy(tag[0:4], "curv")
	binary.BigEndian.PutUint32(tag[8:12], 1)
	binary.BigEndian.PutUint16(tag[12:14], uint16(math.Round(gamma*256)))
	return tag
}

func writeTestS15Fixed16(out []byte, value float64) {
	binary.BigEndian.PutUint32(out, uint32(int32(math.Round(value*65536))))
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

	tint := entity.NewDict()
	tint.Set(entity.NewName("FunctionType"), entity.NewInteger(2))
	tint.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	tint.Set(entity.NewName("C0"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(0)))
	tint.Set(entity.NewName("C1"), entity.NewArray(entity.NewReal(1), entity.NewReal(0.5), entity.NewReal(0)))
	tint.Set(entity.NewName("N"), entity.NewReal(1))
	devNParsed, err := r.ParseColorSpace(entity.NewArray(
		entity.NewName("DeviceN"),
		entity.NewArray(entity.NewName("Spot")),
		entity.NewName("DeviceRGB"),
		tint,
	))
	require.NoError(t, err)
	require.IsType(t, &DeviceNColorSpace{}, devNParsed)
	assert.Equal(t, "DeviceN", devNParsed.Name())
	assert.Equal(t, 1, devNParsed.GetNumComponents())
	assert.Equal(t, color.RGBA{R: 255, G: 128, B: 0, A: 255}, devNParsed.ConvertToRGBA([]float64{1}))

	calGray, err := r.ParseColorSpace(entity.NewArray(entity.NewName("CalGray")))
	require.NoError(t, err)
	assert.Equal(t, "DeviceGray", calGray.Name())

	calRGB, err := r.ParseColorSpace(entity.NewArray(entity.NewName("CalRGB")))
	require.NoError(t, err)
	assert.Equal(t, "CalRGB", calRGB.Name())

	lab, err := r.ParseColorSpace(entity.NewArray(entity.NewName("Lab")))
	require.NoError(t, err)
	assert.Equal(t, "DeviceRGB", lab.Name())
}

func TestRegistry_ParseColorSpace_CalRGBDictionary(t *testing.T) {
	r := NewRegistry()
	dict := entity.NewDict()
	dict.Set(entity.NewName("WhitePoint"), entity.NewArray(entity.NewReal(0.9505), entity.NewReal(1), entity.NewReal(1.089)))
	dict.Set(entity.NewName("BlackPoint"), entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(0)))
	dict.Set(entity.NewName("Gamma"), entity.NewArray(entity.NewReal(1), entity.NewReal(1), entity.NewReal(1)))
	dict.Set(entity.NewName("Matrix"), entity.NewArray(
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(0), entity.NewReal(1), entity.NewReal(0),
		entity.NewReal(1), entity.NewReal(1), entity.NewReal(1),
	))

	cs, err := r.ParseColorSpace(entity.NewArray(entity.NewName("CalRGB"), dict))
	require.NoError(t, err)
	require.Equal(t, ColorSpaceCalRGB, cs.Type())

	rgba := cs.ConvertToRGBA([]float64{0, 0, 1})
	assert.Greater(t, rgba.R, uint8(240))
	assert.Greater(t, rgba.G, uint8(240))
	assert.Greater(t, rgba.B, uint8(240))
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

func TestParseSampledFunction_PopplerBitsEncodeDecodeDefaults(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.NewName("FunctionType"), entity.NewInteger(0))
	dict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	dict.Set(entity.NewName("Range"), entity.NewArray(
		entity.NewReal(10), entity.NewReal(20),
		entity.NewReal(30), entity.NewReal(40),
	))
	dict.Set(entity.NewName("Size"), entity.NewArray(entity.NewInteger(2)))
	dict.Set(entity.NewName("BitsPerSample"), entity.NewInteger(4))
	stream := entity.NewStream(dict, []byte{0x0f, 0x80})

	parsed, err := parseFunctionFromObject(stream)
	require.NoError(t, err)
	fn, ok := parsed.(*entity.SampledFunction)
	require.True(t, ok)
	require.True(t, fn.Interpolate)
	assert.Equal(t, [][2]float64{{0, 1}}, fn.Encode)
	assert.Equal(t, fn.RangeVal, fn.Decode)
	assert.Equal(t, []float64{0, 1, 8.0 / 15.0, 0}, fn.Samples)

	out, err := fn.Evaluate([]float64{0})
	require.NoError(t, err)
	assert.Equal(t, []float64{10, 40}, out)
}
