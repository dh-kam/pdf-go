package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestApplyPredictor_PNGPredictorTwelveUsesRowFilterBytes(t *testing.T) {
	params := entity.NewDict()
	params.Set(entity.Name("Predictor"), entity.NewInteger(12))
	params.Set(entity.Name("Columns"), entity.NewInteger(5))

	encoded := []byte{
		0, 1, 2, 3, 4, 5,
		2, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1,
	}

	decoded, err := ApplyPredictor(encoded, params)

	require.NoError(t, err)
	assert.Equal(t, []byte{
		1, 2, 3, 4, 5,
		2, 3, 4, 5, 6,
		1, 2, 3, 4, 5,
	}, decoded)
}

func TestApplyPredictor_PNGPredictorUsesPixelStrideForRGB(t *testing.T) {
	tests := []struct {
		name    string
		encoded []byte
		want    []byte
	}{
		{
			name: "sub",
			encoded: []byte{
				1,
				10, 20, 30,
				5, 5, 5,
			},
			want: []byte{
				10, 20, 30,
				15, 25, 35,
			},
		},
		{
			name: "average",
			encoded: []byte{
				3,
				10, 20, 30,
				10, 15, 20,
			},
			want: []byte{
				10, 20, 30,
				15, 25, 35,
			},
		},
		{
			name: "paeth",
			encoded: []byte{
				4,
				10, 20, 30,
				5, 5, 5,
			},
			want: []byte{
				10, 20, 30,
				15, 25, 35,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := entity.NewDict()
			params.Set(entity.Name("Predictor"), entity.NewInteger(15))
			params.Set(entity.Name("Columns"), entity.NewInteger(2))
			params.Set(entity.Name("Colors"), entity.NewInteger(3))
			params.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))

			decoded, err := ApplyPredictor(tt.encoded, params)

			require.NoError(t, err)
			assert.Equal(t, tt.want, decoded)
		})
	}
}
