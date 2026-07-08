package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestImageXObjectDecodeSizeHintUsesResolvedColorSpace(t *testing.T) {
	e := NewEvaluator(nil)

	require.Equal(t, 17*11*3, e.imageXObjectDecodeSizeHint(entity.Name("DeviceRGB"), 17, 11, 8, false))
	require.Equal(t, 17*11*4, e.imageXObjectDecodeSizeHint(entity.Name("DeviceCMYK"), 17, 11, 8, false))
	require.Equal(t, 17*11, e.imageXObjectDecodeSizeHint(entity.Name("DeviceGray"), 17, 11, 8, false))
}

func TestImageXObjectDecodeSizeHintUsesOneComponentForIndexedAndMasks(t *testing.T) {
	e := NewEvaluator(nil)
	indexed := entity.NewArray(
		entity.Name("Indexed"),
		entity.Name("DeviceRGB"),
		entity.NewInteger(1),
		entity.NewString("\x00\x00\x00\xff\xff\xff"),
	)

	require.Equal(t, 17*11, e.imageXObjectDecodeSizeHint(indexed, 17, 11, 8, false))
	require.Equal(t, ((17+7)/8)*11, e.imageXObjectDecodeSizeHint(nil, 17, 11, 1, true))
}
