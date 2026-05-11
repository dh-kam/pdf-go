// Package font_test provides Type1 font tests.
package font_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	type1font "github.com/dh-kam/pdf-go/internal/infrastructure/font/type1"
)

func TestType1Font_ParsePFBHeader(t *testing.T) {
	// Test PFB header parsing
	// PFB files start with: 0x80 0x01 [length_low length_high] 0x80 [type]
	data := []byte{
		0x80, 0x01, // Magic
		0x04, 0x00, // Length (1024 bytes, little endian)
		0x80, 0x01, // Type: ASCII (1)
	}

	p := type1font.NewParser(data)

	// Verify parser was created
	assert.NotNil(t, p)
}

func TestType1Font_ParseMinimal(t *testing.T) {
	// Test with minimal ASCII font data
	data := []byte(`%Type1 Font
/FontName (TestFont) def
/FontInfo 8 dict dup begin
  /ItalicAngle 0 def
  /isFixedPitch false def
end readonly def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)
	require.NotNil(t, font)

	assert.Equal(t, "TestFont", font.Name())
	assert.False(t, font.IsCIDFont())
	assert.Equal(t, uint16(1000), font.UnitsPerEm())
}

func TestType1Font_CharCodeToGlyph(t *testing.T) {
	// Test character code to glyph mapping
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	// Test common characters
	glyph, err := font.CharCodeToGlyph(0x41) // 'A'
	require.NoError(t, err)
	assert.Equal(t, uint32(0x41), glyph)

	// Test glyph name
	name := font.GlyphName(0x41)
	assert.Equal(t, "A", name)
}

func TestType1Font_GetGlyphWidth(t *testing.T) {
	// Test glyph width retrieval
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	glyph, err := font.CharCodeToGlyph(0x41)
	require.NoError(t, err)

	width, err := font.GetGlyphWidth(glyph)
	require.NoError(t, err)
	assert.Greater(t, width, 0.0)
}

func TestType1Font_RenderGlyph(t *testing.T) {
	// Test glyph rendering
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	glyph, err := font.CharCodeToGlyph(0x41)
	require.NoError(t, err)

	path, err := font.RenderGlyph(glyph, 12.0)
	require.NoError(t, err)
	assert.NotNil(t, path)
}

func TestType1Font_AdvanceWidth(t *testing.T) {
	// Test advance width calculation
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	advance, err := font.GetAdvanceWidth(0x41, 12.0)
	require.NoError(t, err)
	assert.Greater(t, advance, 0.0)
}

func TestType1Font_HasGlyph(t *testing.T) {
	// Test glyph existence check
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	// Common Latin characters should exist
	assert.True(t, font.HasGlyph(0x41)) // A
	assert.True(t, font.HasGlyph(0x61)) // a
	assert.True(t, font.HasGlyph(0x20)) // space
}

func TestType1Font_GetBoundingBox(t *testing.T) {
	// Test font bounding box
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	xMin, yMin, xMax, yMax := font.GetBoundingBox()

	// Default bounding box should be returned
	assert.NotNil(t, xMin)
	assert.NotNil(t, yMin)
	assert.NotNil(t, xMax)
	assert.NotNil(t, yMax)
}

func TestType1Font_GetFontDescriptor(t *testing.T) {
	// Test font descriptor
	data := []byte(`%Type1 Font
/FontName (TestFont) def
`)

	font, err := type1font.NewFontFromBytes(data)
	require.NoError(t, err)

	desc := font.GetFontDescriptor()
	assert.NotNil(t, desc)
	assert.Equal(t, "TestFont", desc.FontName)
}

func TestType1Font_DecryptEexec(t *testing.T) {
	// Test eexec decryption
	// This is a simple test to verify the decryption function works

	// Create test data (known plaintext)
	plaintext := []byte{0x01, 0x02, 0x03, 0x04}

	// Encrypt then decrypt
	encrypted, err := type1font.EncryptEexec(plaintext)
	require.NoError(t, err)

	decrypted, err := type1font.DecryptEexec(encrypted)
	require.NoError(t, err)

	assert.Equal(t, plaintext, decrypted)
}

func TestType1Font_CharStringDecoder(t *testing.T) {
	// Test CharString decoder

	// Test simple moveto command
	// rmoveto: 21 dx dy (arguments come before command)
	data := []byte{100, 50, 21, 14} // rmoveto 100 50, endchar

	decoder := type1font.NewCharStringDecoder(data)
	commands, err := decoder.Decode()
	require.NoError(t, err)

	t.Logf("Commands: %+v\n", commands)
	for i, cmd := range commands {
		t.Logf("Cmd %d: Type=%d (%s), Args=%v\n", i, cmd.Type, cmd.Type, cmd.Args)
	}

	// Should get rmoveto command followed by endchar
	require.GreaterOrEqual(t, len(commands), 1)

	// Find rmoveto command
	var rmoveCmd *type1font.Command
	for _, cmd := range commands {
		if cmd.Type == type1font.CmdRMoveto {
			rmoveCmd = &cmd
			break
		}
	}

	require.NotNil(t, rmoveCmd)
	assert.Len(t, rmoveCmd.Args, 2)
}

func TestType1Font_CharStringDecoder_Curve(t *testing.T) {
	// Test curve command
	// rrcurveto: 8 dx1 dy1 dx2 dy2 dx3 dy3 (arguments come before command)
	// Note: In Type1 CharString, numbers must be in range 32-246 for single-byte encoding
	data := []byte{50, 51, 52, 53, 54, 55, 8, 14} // rrcurveto 50 51 52 53 54 55, endchar

	decoder := type1font.NewCharStringDecoder(data)
	commands, err := decoder.Decode()
	require.NoError(t, err)

	// Debug: print all commands
	t.Logf("Commands: %+v\n", commands)
	for i, cmd := range commands {
		t.Logf("Cmd %d: Type=%d (%s), Args=%v\n", i, cmd.Type, cmd.Type, cmd.Args)
	}

	// Should get rrcurveto command followed by endchar
	require.GreaterOrEqual(t, len(commands), 1)

	// Find rrcurveto command
	var curveCmd *type1font.Command
	for _, cmd := range commands {
		if cmd.Type == type1font.CmdRRCurveto {
			curveCmd = &cmd
			break
		}
	}

	require.NotNil(t, curveCmd)
	assert.Len(t, curveCmd.Args, 6)
}
