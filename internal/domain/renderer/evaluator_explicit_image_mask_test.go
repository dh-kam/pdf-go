package renderer

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestResolveExplicitImageMaskMatchesPopplerDecodePolarity(t *testing.T) {
	tests := []struct {
		name          string
		decode        *entity.Array
		wantFirstGray uint8
		wantNextGray  uint8
	}{
		{
			name:          "decode zero-one paints zero bits",
			decode:        entity.NewArray(entity.NewInteger(0), entity.NewInteger(1)),
			wantFirstGray: 0x00,
			wantNextGray:  0xFF,
		},
		{
			name:          "decode one-zero paints one bits",
			decode:        entity.NewArray(entity.NewInteger(1), entity.NewInteger(0)),
			wantFirstGray: 0xFF,
			wantNextGray:  0x00,
		},
		{
			name:          "missing decode paints zero bits",
			wantFirstGray: 0x00,
			wantNextGray:  0xFF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dict := entity.NewDict()
			dict.Set(entity.Name("W"), entity.NewInteger(2))
			dict.Set(entity.Name("H"), entity.NewInteger(1))
			dict.Set(entity.Name("IM"), entity.NewBoolean(true))
			if tt.decode != nil {
				dict.Set(entity.Name("D"), tt.decode)
			}
			maskStream := entity.NewStream(dict, []byte{0b1000_0000})

			mask := NewEvaluator(nil).resolveExplicitImageMask(maskStream)
			require.NotNil(t, mask)
			require.False(t, mask.IsInverted())

			gotFirst := color.GrayModel.Convert(mask.Image().At(0, 0)).(color.Gray).Y
			gotNext := color.GrayModel.Convert(mask.Image().At(1, 0)).(color.Gray).Y
			require.Equal(t, tt.wantFirstGray, gotFirst)
			require.Equal(t, tt.wantNextGray, gotNext)
		})
	}
}

func TestResolveExplicitImageMaskRejectsSoftMaskImage(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	dict.Set(entity.Name("Height"), entity.NewInteger(1))
	maskStream := entity.NewStream(dict, []byte{0})

	mask := NewEvaluator(nil).resolveExplicitImageMask(maskStream)
	require.Nil(t, mask)
}
