package type1

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_ParseReturnsErrorForShortData(t *testing.T) {
	p := NewParser([]byte{0x01})
	_, err := p.Parse()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too short")
}

func TestParser_ParsePFB_Simple(t *testing.T) {
	// EOF marker only.
	data := []byte{0x80, 0x01, 0x00, 0x00, 0x80, 0x03}
	p := NewParser(data)

	fontFile, err := p.Parse()
	require.NoError(t, err)
	require.NotNil(t, fontFile)
	assert.NotNil(t, fontFile.Segments)
	assert.Empty(t, fontFile.Binary)
	assert.Empty(t, fontFile.ASCII)
}

func TestParser_ParsePFAWithoutEexec(t *testing.T) {
	data := []byte(`/FontName (NoEexec) def
/FontInfo << /ItalicAngle 0 /isFixedPitch false >>`)

	p := NewParser(data)
	fontFile, err := p.Parse()
	require.NoError(t, err)
	require.NotNil(t, fontFile)
	assert.Equal(t, data, fontFile.ASCII)
	assert.Empty(t, fontFile.Binary)
}

func TestParser_ParsePFAWithEexecHex(t *testing.T) {
	data := []byte(`/FontName (Test) def
/FontInfo << /ItalicAngle 10 /FontBBox [0 0 100 200] /isFixedPitch true >>
currentfile eexec
48656c6c6f`) // "Hello" in hex

	p := NewParser(data)
	fontFile, err := p.Parse()
	require.NoError(t, err)
	require.NotNil(t, fontFile)
	assert.Equal(t, "Test", fontFile.FontName)
	assert.Equal(t, []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f}, fontFile.Binary)
}

func TestParser_ParseFontDictionary(t *testing.T) {
	data := []byte(`/FontName (FontNameInTest) def
/FontInfo << /ItalicAngle -12 /isFixedPitch true /FontBBox [1 2 3 4] >>
/Encoding 0`)

	p := NewParser(data)
	font, err := p.Parse()
	require.NoError(t, err)
	assert.Equal(t, "FontNameInTest", font.FontName)
	assert.InDelta(t, -12, font.FontInfo.ItalicAngle, 0.0001)
	assert.True(t, font.FontInfo.IsFixedPitch)
	assert.Equal(t, [4]float64{1, 2, 3, 4}, font.FontInfo.FontBBox)
}

func TestParser_FindEexecStartAndHexDecoding(t *testing.T) {
	const input = "/FontName/Test eexec \n41 42 43\n"
	assert.Equal(t, 22, findEexecStart(input))

	decoded, err := decodeHexString("GG 00 01 02")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x01, 0x02}, decoded)
}

func TestFont_NewFromBytesAndBasics(t *testing.T) {
	data := []byte(`/FontName (TestFont) def
/FontInfo << /ItalicAngle 11 /isFixedPitch false /FontBBox [0 0 500 700] >>`)

	font, err := NewFontFromBytes(data)
	require.NoError(t, err)
	require.NotNil(t, font)

	assert.Equal(t, "TestFont", font.Name())
	assert.False(t, font.IsCIDFont())
	assert.Equal(t, uint16(1000), font.UnitsPerEm())
	assert.True(t, font.HasGlyph(0x41))

	glyph, err := font.CharCodeToGlyph(0x41)
	require.NoError(t, err)
	assert.Equal(t, uint32(0x41), glyph)

	width, err := font.GetGlyphWidth(glyph)
	require.NoError(t, err)
	assert.Greater(t, width, 0.0)

	path, err := font.RenderGlyph(glyph, 12)
	require.NoError(t, err)
	assert.NotNil(t, path)

	advance, err := font.GetAdvanceWidth(glyph, 12)
	require.NoError(t, err)
	assert.Greater(t, advance, 0.0)

	xMin, yMin, xMax, yMax := font.GetBoundingBox()
	assert.Equal(t, 0.0, xMin)
	assert.Equal(t, 0.0, yMin)
	assert.Equal(t, 500.0, xMax)
	assert.Equal(t, 700.0, yMax)

	desc := font.GetFontDescriptor()
	require.NotNil(t, desc)
	assert.Equal(t, "TestFont", desc.FontName)
}

func TestFont_FromReaderAndFile(t *testing.T) {
	data := []byte(`/FontName (FileFont) def`)

	r := bytes.NewReader(data)
	fontFile, err := ReadFromReader(r)
	require.NoError(t, err)
	require.NotNil(t, fontFile)
	assert.Equal(t, data, fontFile.ASCII)

	tmp := t.TempDir()
	path := tmp + "/font.pfa"
	require.NoError(t, os.WriteFile(path, data, 0o600))

	fontFile2, err := ReadFromFile(path)
	require.NoError(t, err)
	require.NotNil(t, fontFile2)
	assert.Equal(t, fontFile2.ASCII, fontFile.ASCII)
}

func TestFontFileGetCharStringsFallbackAndErrors(t *testing.T) {
	font := &FontFile{ASCII: []byte(`/CharStrings 0 dict`), Binary: nil}
	cs, err := font.GetCharStrings()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0c, 0x01, 0x0c, 0x0e}, cs)

	empty := &FontFile{}
	_, err = empty.GetCharStrings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no binary data")
}

func TestFont_GeneratePathAndDefaultGlyph(t *testing.T) {
	fontForPath := &Font{file: &FontFile{}, glyphs: map[uint32]*Glyph{}}
	fontForPath.chars = map[string]*Glyph{}

	path, err := fontForPath.generatePath([]Command{
		{Type: CmdRMoveto, Args: []float64{10, 20}},
		{Type: CmdRLineto, Args: []float64{5, 5}},
		{Type: CmdRRCurveto, Args: []float64{1, 2, 3, 4, 5, 6}},
		{Type: CmdHStem, Args: []float64{1, 2, 3, 4}},
		{Type: CmdEndChar},
	}, 1000)
	require.NoError(t, err)
	assert.Len(t, path.Commands, 3)

	fontForPath.createDefaultGlyph(123)
	assert.Contains(t, fontForPath.glyphs, uint32(123))

	path2, err := fontForPath.RenderGlyph(1234, 1000)
	require.NoError(t, err)
	assert.NotNil(t, path2)
}

func TestDecryptEexecAndCharString(t *testing.T) {
	// Encrypt 4 random bytes (discard prefix) + plaintext
	randomPrefix := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	plaintext := []byte{0x01, 0x02, 0x03, 0x04}
	encrypted, err := EncryptEexec(append(randomPrefix, plaintext...))
	require.NoError(t, err)

	decrypted, err := DecryptEexec(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	_, err = DecryptEexec([]byte{1, 2, 3})
	require.Error(t, err)

	_, err = DecryptCharString(nil)
	require.NoError(t, err)
	decoded, err := DecryptCharStringWithLenIV([]byte{0x01, 0x02}, -1)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02}, decoded)
}

func TestCharStringDecoderBranches(t *testing.T) {
	decoder := NewCharStringDecoder([]byte{
		255, 0, 0, 0, 1, // number
		0x0c, 0x07, // escape command (Sbw)
		1, 2, 3, 247, 10, 251, 1, // numbers
		byte(CmdRMoveto), 0, // moveto + end
	})
	commands, err := decoder.Decode()
	require.NoError(t, err)
	assert.NotEmpty(t, commands)

	_, err = NewCharStringDecoder([]byte{0x0c}).nextCommand()
	require.Error(t, err)

	_, err = NewCharStringDecoder([]byte{247}).nextCommand()
	require.Error(t, err)
}

func TestParseFloatAndArrayHelpers(t *testing.T) {
	assert.InDelta(t, 12.5, parseFloat("12.5"), 0.0001)
	assert.InDelta(t, -1.25, parseFloat("-1.25"), 0.0001)
	assert.Equal(t, [4]float64{1, 2, 3, 4}, parseArray4("{1 2 3 4}"))
	assert.Equal(t, []string{"1", "2", "3", "4"}, splitArray("[1 2 3 4]"))
}

func TestDictParsingHelpers(t *testing.T) {
	dict := "/FontInfo << /ItalicAngle 10 /isFixedPitch true /FontBBox [1 2 3 4] >>"
	assert.Equal(t, "<<", extractDictValue(dict, "FontInfo"))

	assert.Equal(t, "Demo", extractFontName("/FontName (Demo)"))
	end := findMatchingDictEnd(dict, 0)
	assert.Greater(t, end, 0)
}

func TestPFAEncodingDefaults(t *testing.T) {
	raw := string([]byte("/Encoding /StandardEncoding"))
	encoding := extractEncoding(raw)
	assert.Empty(t, encoding)
	assert.IsType(t, map[byte]string{}, encoding)
}

func TestFontEncodingName(t *testing.T) {
	font := &Font{
		encoding: map[byte]string{
			13: "dotlessi",
			20: "dotlessj",
		},
	}

	assert.Equal(t, "dotlessi", font.EncodingName(13))
	assert.Equal(t, "dotlessj", font.EncodingName(20))
	assert.Empty(t, font.EncodingName(99))
}

func TestFontCreateFallbackGlyphUsesStandardGlyphWhenAvailable(t *testing.T) {
	font := &Font{
		file:     &FontFile{},
		glyphs:   map[uint32]*Glyph{},
		chars:    map[string]*Glyph{},
		fontName: "ABCDEF+SFTT0900",
	}

	font.createFallbackGlyph(uint32('%'), "percent")

	glyph, ok := font.glyphs[uint32('%')]
	require.True(t, ok)
	require.NotNil(t, glyph)
	require.NotNil(t, glyph.fallback)
	assert.Greater(t, glyph.Width, 0.0)
	assert.NotEmpty(t, glyph.Path.Commands)

	path, err := font.RenderGlyph(uint32('%'), 12)
	require.NoError(t, err)
	assert.NotEmpty(t, path.Commands)
}

func TestIsEexecEncrypted(t *testing.T) {
	assert.True(t, isEexecEncrypted([]byte{0x00, 0x00, 0x00, 0x00}))
	assert.True(t, isEexecEncrypted([]byte{0x01, 0x00, 0x00, 0x00}))
	assert.False(t, isEexecEncrypted([]byte{0x00, 0x01, 0x00, 0x00}))
}
