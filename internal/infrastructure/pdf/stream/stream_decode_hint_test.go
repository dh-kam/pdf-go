package stream

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestStreamDecodeSizeHintUsesImageDictionary(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(17))
	dict.Set(entity.Name("Height"), entity.NewInteger(11))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	dict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))

	s := NewStream(dict, nil)

	require.Equal(t, 17*11*3, s.decodeSizeHint(nil))
}

func TestStreamDecodeSizeHintUsesSlashPrefixedImageColorSpace(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(17))
	dict.Set(entity.Name("Height"), entity.NewInteger(11))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	dict.Set(entity.Name("ColorSpace"), entity.Name("/DeviceRGB"))

	s := NewStream(dict, nil)

	require.Equal(t, 17*11*3, s.decodeSizeHint(nil))
	require.Equal(t, 17*11*3, s.exactDecodeSizeHint(nil, true))
}

func TestStreamDecodeSizeHintUsesImageMaskDictionary(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("W"), entity.NewInteger(17))
	dict.Set(entity.Name("H"), entity.NewInteger(11))
	dict.Set(entity.Name("ImageMask"), entity.NewBoolean(true))

	s := NewStream(dict, nil)

	require.Equal(t, ((17+7)/8)*11, s.decodeSizeHint(nil))
}

func TestStreamDecodeSizeHintUsesCallerOverride(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(17))
	dict.Set(entity.Name("Height"), entity.NewInteger(11))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	dict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))

	s := NewStream(dict, nil)
	s.SetDecodeSizeHint(4096)

	require.Equal(t, 4096, s.decodeSizeHint(nil))
}

func TestStreamExactDecodeSizeHintUsesType1FontLengths(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Length1"), entity.NewInteger(725))
	dict.Set(entity.Name("Length2"), entity.NewInteger(34883))
	dict.Set(entity.Name("Length3"), entity.NewInteger(0))

	s := NewStream(dict, nil)

	require.Equal(t, 35608, s.exactDecodeSizeHint(nil, true))
	require.Equal(t, 0, s.exactDecodeSizeHint(nil, false))
}

func TestStreamExactDecodeSizeHintUsesImageDictionaryForLastFilter(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(17))
	dict.Set(entity.Name("Height"), entity.NewInteger(11))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	dict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))

	s := NewStream(dict, nil)

	require.Equal(t, 17*11*3, s.exactDecodeSizeHint(nil, true))
	require.Equal(t, 0, s.exactDecodeSizeHint(nil, false))
}

func TestStreamExactDecodeSizeHintIncludesPNGPredictorBytes(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Height"), entity.NewInteger(11))

	params := entity.NewDict()
	params.Set(entity.Name("Predictor"), entity.NewInteger(15))
	params.Set(entity.Name("Columns"), entity.NewInteger(17))
	params.Set(entity.Name("Colors"), entity.NewInteger(3))
	params.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))

	s := NewStream(dict, nil)

	require.Equal(t, 17*11*3+11, s.exactDecodeSizeHint(params, true))
}

func TestFlateInitialDecodeBufferSizeHonorsTinyHint(t *testing.T) {
	decoder := &FlateDecoder{}
	decoder.SetDecodeSizeHint(30)

	require.Equal(t, 30, decoder.initialDecodeBufferSize(25))
}

func TestFlateInitialDecodeBufferSizeKeepsDefaultMinimumWithoutHint(t *testing.T) {
	decoder := &FlateDecoder{}

	require.Equal(t, 1024, decoder.initialDecodeBufferSize(25))
}

func TestFlateInitialDecodeBufferSizeHonorsTinyExactHint(t *testing.T) {
	decoder := &FlateDecoder{}
	decoder.SetExactDecodeSizeHint(31)

	require.Equal(t, 31, decoder.initialDecodeBufferSize(25))
}

func TestFlateInitialDecodeBufferSizePrefersSmallerExactHint(t *testing.T) {
	decoder := &FlateDecoder{}
	decoder.SetDecodeSizeHint(1024)
	decoder.SetExactDecodeSizeHint(1024)

	require.Equal(t, 1024, decoder.initialDecodeBufferSize(2048))
}

func TestDecodeParamsListWithoutDecodeParmsAvoidsAllocation(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))
	s := NewStream(dict, nil)

	params, err := s.getDecodeParamsList(1)

	require.NoError(t, err)
	require.Nil(t, params)
}
