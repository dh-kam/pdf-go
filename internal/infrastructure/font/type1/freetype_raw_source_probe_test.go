package type1

import (
	"testing"

	"github.com/stretchr/testify/require"

	ftcgo "github.com/dh-kam/pdf-go/internal/infrastructure/font/freetype"
)

func TestSFRM1095FreeTypeRawSourceProbe(t *testing.T) {
	if !ftcgo.IsAvailable() {
		t.Skip("freetype unavailable")
	}

	font := loadSampleType1FontForProbe(
		t,
		"../../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
		109,
		"F16",
	)
	require.NotNil(t, font)
	require.NotNil(t, font.file)

	codes := []int{101, 97, 116, 46}
	rawData := font.file.RawData()
	otfData := font.OTFData()
	require.NotEmpty(t, rawData)
	require.NotEmpty(t, otfData)

	for _, code := range codes {
		_, rawW, rawH, rawLeft, rawTop, rawErr := ftcgo.RenderGlyphBitmap(rawData, uint32(code), 12, 150)
		_, otfW, otfH, otfLeft, otfTop, otfErr := ftcgo.RenderGlyphBitmap(otfData, uint32(code), 12, 150)
		t.Logf(
			"code=%d name=%s raw_ok=%t raw_dims=%dx%d raw_bearing=(%d,%d) otf_ok=%t otf_dims=%dx%d otf_bearing=(%d,%d)",
			code,
			font.EncodingName(byte(code)),
			rawErr == nil,
			rawW,
			rawH,
			rawLeft,
			rawTop,
			otfErr == nil,
			otfW,
			otfH,
			otfLeft,
			otfTop,
		)
	}
}
