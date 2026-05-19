package font_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

func TestStandardFont_GetFont(t *testing.T) {
	tests := []struct {
		name      string
		fontName  string
		wantFound bool
	}{
		{"Times-Roman", "Times-Roman", true},
		{"Times-Bold", "Times-Bold", true},
		{"Times-Italic", "Times-Italic", true},
		{"Times-BoldItalic", "Times-BoldItalic", true},
		{"Helvetica", "Helvetica", true},
		{"Helvetica-Bold", "Helvetica-Bold", true},
		{"Helvetica-Oblique", "Helvetica-Oblique", true},
		{"Helvetica-BoldOblique", "Helvetica-BoldOblique", true},
		{"Courier", "Courier", true},
		{"Courier-Bold", "Courier-Bold", true},
		{"Courier-Oblique", "Courier-Oblique", true},
		{"Courier-BoldOblique", "Courier-BoldOblique", true},
		{"Symbol", "Symbol", true},
		{"ZapfDingbats", "ZapfDingbats", true},
		{"Unknown Font", "Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			font, found := standard.GetFont(tt.fontName)
			if tt.wantFound {
				assert.True(t, found, "font should be found")
				assert.NotNil(t, font, "font should not be nil")
			} else {
				assert.False(t, found, "font should not be found")
			}
		})
	}
}

func TestStandardFont_Name(t *testing.T) {
	font, found := standard.GetFont("Helvetica")
	require.True(t, found, "Helvetica font should be found")

	assert.Equal(t, "Helvetica", font.Name())
}

func TestStandardFont_IsCIDFont(t *testing.T) {
	font, found := standard.GetFont("Times-Roman")
	require.True(t, found)

	assert.False(t, font.IsCIDFont(), "standard fonts are not CID fonts")
}

func TestStandardFont_IsSymbolic(t *testing.T) {
	t.Run("Symbol font is symbolic", func(t *testing.T) {
		font, found := standard.GetFont("Symbol")
		require.True(t, found)
		assert.True(t, font.IsSymbolic())
	})

	t.Run("ZapfDingbats font is symbolic", func(t *testing.T) {
		font, found := standard.GetFont("ZapfDingbats")
		require.True(t, found)
		assert.True(t, font.IsSymbolic())
	})

	t.Run("Helvetica is not symbolic", func(t *testing.T) {
		font, found := standard.GetFont("Helvetica")
		require.True(t, found)
		assert.False(t, font.IsSymbolic())
	})
}

func TestStandardFont_UnitsPerEm(t *testing.T) {
	font, found := standard.GetFont("Times-Roman")
	require.True(t, found)

	assert.Equal(t, uint16(2048), font.UnitsPerEm())
}

func TestStandardFont_CharCodeToGlyph(t *testing.T) {
	font, found := standard.GetFont("Helvetica")
	require.True(t, found)

	tests := []struct {
		name      string
		charCode  uint32
		wantGlyph uint32
		wantError bool
	}{
		{"ASCII A", 65, 65, false},
		{"ASCII space", 32, 32, false},
		{"ASCII tilde", 126, 126, false},
		{"Unicode code point", 256, 256, false},
		{"way out of range", 0x110000, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glyph, err := font.CharCodeToGlyph(tt.charCode)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantGlyph, glyph)
			}
		})
	}
}

func TestStandardFont_GetGlyphWidth(t *testing.T) {
	font, found := standard.GetFont("Helvetica")
	require.True(t, found)

	tests := []struct {
		name      string
		glyph     uint32
		wantError bool
	}{
		{"glyph 0", 0, false},
		{"glyph 65 (A)", 65, false},
		{"glyph 255", 255, false},
		{"Unicode glyph", 256, false},
		{"out of Unicode range", 0x110000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, err := font.GetGlyphWidth(tt.glyph)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, width, 0.0, "width should be positive")
			}
		})
	}
}

func TestStandardFont_GetBoundingBox(t *testing.T) {
	font, found := standard.GetFont("Times-Roman")
	require.True(t, found)

	xMin, yMin, xMax, yMax := font.GetBoundingBox()
	// Standard fonts have default bounding boxes
	assert.NotNil(t, font)
	// The actual values depend on the font metrics
	assert.True(t, xMin <= xMax, "xMin should be <= xMax")
	assert.True(t, yMin <= yMax, "yMin should be <= yMax")
}

func TestStandardFont_RenderGlyph(t *testing.T) {
	font, found := standard.GetFont("Helvetica")
	require.True(t, found)

	tests := []struct {
		name    string
		glyph   uint32
		size    float64
		wantErr bool
	}{
		{name: "simple glyph", glyph: 65, size: 12.0, wantErr: false},
		{name: "space has no outline", glyph: 32, size: 12.0, wantErr: true},
		{name: "larger size", glyph: 65, size: 24.0, wantErr: false},
		{name: "small size", glyph: 65, size: 8.0, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := font.RenderGlyph(tt.glyph, tt.size)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, path)
				assert.NotNil(t, path.Commands)
				assert.Equal(t, 4, len(path.Bounds), "bounds should have 4 values")

				// Check that outline bounds are populated; advance width is tested separately.
				width, widthErr := font.GetGlyphWidth(tt.glyph)
				require.NoError(t, widthErr)
				assert.Greater(t, width, 0.0)
				assert.Greater(t, path.Bounds[2]-path.Bounds[0], 0.0, "outline width should be positive")
				assert.Greater(t, path.Bounds[3]-path.Bounds[1], 0.0, "outline height should be positive")
			}
		})
	}
}

func TestStandardFont_GlyphName(t *testing.T) {
	font, found := standard.GetFont("Helvetica")
	require.True(t, found)

	tests := []struct {
		wantContains string
		glyph        uint32
	}{
		{wantContains: "A", glyph: 65},            // ASCII character
		{wantContains: "space", glyph: 32},        // Adobe glyph name
		{wantContains: ".notdef.0", glyph: 0},     // Control character
		{wantContains: ".notdef.127", glyph: 127}, // DEL character
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			name := font.GlyphName(tt.glyph)
			assert.Contains(t, name, tt.wantContains)
		})
	}
}

func TestGetStandard14Names(t *testing.T) {
	names := standard.GetStandard14Names()

	assert.Equal(t, 14, len(names), "should have 14 standard fonts")

	expectedFonts := []string{
		"Times-Roman",
		"Times-Bold",
		"Times-Italic",
		"Times-BoldItalic",
		"Helvetica",
		"Helvetica-Bold",
		"Helvetica-Oblique",
		"Helvetica-BoldOblique",
		"Courier",
		"Courier-Bold",
		"Courier-Oblique",
		"Courier-BoldOblique",
		"Symbol",
		"ZapfDingbats",
	}

	for _, expected := range expectedFonts {
		assert.Contains(t, names, expected, "should contain %s", expected)
	}
}

func TestStandardFont_CourierMonospace(t *testing.T) {
	// Courier is monospace - all glyphs should have same width
	fonts := []string{"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique"}

	for _, fontName := range fonts {
		t.Run(fontName, func(t *testing.T) {
			font, found := standard.GetFont(fontName)
			require.True(t, found)

			width1, _ := font.GetGlyphWidth(65)  // 'A'
			width2, _ := font.GetGlyphWidth(66)  // 'B'
			width3, _ := font.GetGlyphWidth(32)  // Space
			width4, _ := font.GetGlyphWidth(105) // 'i'

			assert.Equal(t, width1, width2, "Courier should be monospace")
			assert.Equal(t, width2, width3, "Courier should be monospace")
			assert.Equal(t, width3, width4, "Courier should be monospace")
		})
	}
}

func TestStandardFont_HelveticaProportional(t *testing.T) {
	// Helvetica is proportional - different glyphs have different widths
	font, found := standard.GetFont("Helvetica")
	require.True(t, found)

	widthI, _ := font.GetGlyphWidth(73)     // 'I'
	widthM, _ := font.GetGlyphWidth(77)     // 'M'
	widthSpace, _ := font.GetGlyphWidth(32) // Space

	// In real Helvetica, I is narrower than M and space is narrowest
	assert.NotEqual(t, widthI, widthM, "Helvetica should be proportional")
	assert.NotEqual(t, widthM, widthSpace, "Helvetica should be proportional")
}
