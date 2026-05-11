package text

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseToUnicodeCMap_BFCharMultiline(t *testing.T) {
	cmapData := []byte(`
6 beginbfchar
<0003> <>
<03f2> <>
<0392> <>
<03f4> <>
<02f4> <>
<03a3> <062d064e0628064a0628064a0020>
endbfchar
`)

	mapper := parseToUnicodeCMap(cmapData)
	if mapper == nil {
		require.FailNowf(t, "test failed", "expected mapper to be parsed")
	}

	decoded, ok := mapper.Decode([]byte{0x03, 0xA3})
	if !ok {
		require.FailNowf(t, "test failed", "expected code to be mapped")
	}
	if decoded != "حَبيبي " {
		require.FailNowf(t, "test failed", "unexpected decoded text: %q", decoded)
	}
}

func TestParseToUnicodeCMap_BFCharSingleLine(t *testing.T) {
	cmapData := []byte(`
6 beginbfchar
<0003> <> <03f2> <> <0392> <> <03f4> <> <02f4> <> <03a3> <062d064e0628064a0628064a0020>
endbfchar
`)

	mapper := parseToUnicodeCMap(cmapData)
	if mapper == nil {
		require.FailNowf(t, "test failed", "expected mapper to be parsed")
	}

	decoded, ok := mapper.Decode([]byte{0x03, 0xA3})
	if !ok {
		require.FailNowf(t, "test failed", "expected code to be mapped")
	}
	if decoded != "حَبيبي " {
		require.FailNowf(t, "test failed", "unexpected decoded text: %q", decoded)
	}
}

func TestParseToUnicodeCMap_BFRange(t *testing.T) {
	cmapData := []byte(`
1 begincodespacerange
<20> <22>
endcodespacerange
1 beginbfrange
<20> <22> <0041>
endbfrange
`)

	mapper := parseToUnicodeCMap(cmapData)
	if mapper == nil {
		require.FailNowf(t, "test failed", "expected mapper to be parsed")
	}

	decoded, ok := mapper.Decode([]byte{0x20, 0x21, 0x22})
	if !ok {
		require.FailNowf(t, "test failed", "expected range to be mapped")
	}
	if decoded != "ABC" {
		require.FailNowf(t, "test failed", "unexpected decoded text: %q", decoded)
	}
}

func TestExtractTextFromBytes_UsesFontToUnicode(t *testing.T) {
	cmapData := []byte(`
1 beginbfchar
<03a3> <062d064e0628064a0628064a0020>
endbfchar
`)
	mapper := parseToUnicodeCMap(cmapData)
	if mapper == nil {
		require.FailNowf(t, "test failed", "expected mapper to be parsed")
	}

	fontMappings := map[string]*fontTextMapping{
		"F1": {toUnicode: mapper},
	}

	content := []byte("BT /F1 12 Tf <03A3> Tj ET")
	text := extractTextFromBytes(content, fontMappings)
	if text != "حَبيبي" {
		require.FailNowf(t, "test failed", "unexpected extracted text: %q", text)
	}
}

func TestExtractTextFromBytes_DoubleQuoteOperatorWithIntegerSpacing(t *testing.T) {
	content := []byte("BT 12 TL 20 5 (B) \" ET")
	text := extractTextFromBytes(content, nil)
	if text != "B" {
		require.FailNowf(t, "test failed", "unexpected extracted text: %q", text)
	}
}

func TestExtractTextFromBytes_SingleQuoteStartsNewLine(t *testing.T) {
	content := []byte("BT (A) Tj (B) ' ET")
	text := extractTextFromBytes(content, nil)
	if text != "A\nB" {
		require.FailNowf(t, "test failed", "unexpected extracted text: %q", text)
	}
}

func TestExtractTextFromBytes_DoubleQuoteStartsNewLine(t *testing.T) {
	content := []byte("BT (A) Tj 20 5 (B) \" ET")
	text := extractTextFromBytes(content, nil)
	if text != "A\nB" {
		require.FailNowf(t, "test failed", "unexpected extracted text: %q", text)
	}
}

func TestExtractTextFromBytes_TStarStartsNewLine(t *testing.T) {
	content := []byte("BT (A) Tj T* (B) Tj ET")
	text := extractTextFromBytes(content, nil)
	if text != "A\nB" {
		require.FailNowf(t, "test failed", "unexpected extracted text: %q", text)
	}
}
