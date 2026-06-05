package splash

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
)

func TestPackShadingOutputUsesParsedColorMapper(t *testing.T) {
	mapper := colorspace.NewDeviceCMYK()
	rgba := mapper.ConvertToRGBA([]float64{1, 0, 0, 0})

	got := packShadingOutput([]float64{1, 0, 0, 0}, mapper, "DeviceCMYK", ModeRGB8)

	require.Equal(t, Color{rgba.R, rgba.G, rgba.B}, got)
	require.NotEqual(t, Color{0, 255, 255}, got)
}
