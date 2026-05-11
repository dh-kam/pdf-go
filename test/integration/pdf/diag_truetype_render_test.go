package pdf_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/font/truetype"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestDiagTrueTypeRender002(t *testing.T) {
	pdfData, _ := os.ReadFile("../../testdata/compare_legacy/pdfs/sample_002-trivial-libre-office-writer.pdf")

	// Extract font file from object 5 (FontFile2 for DejaVuSans)
	fontData := extractObjectStream(pdfData, 5)
	if fontData == nil {
		t.Fatal("Could not extract font data from object 5")
	}
	t.Logf("Font data: %d bytes, header=%x", len(fontData), fontData[:4])

	ttf, err := truetype.NewFontFromBytes(fontData)
	if err != nil {
		t.Fatalf("TrueType parse: %v", err)
	}

	t.Logf("UPE=%d, NumGlyphs=%d", ttf.UnitsPerEm(), 0 /* can't access directly */)

	// Test charCode 0-27 (the range used by this subset font)
	rendered := 0
	for code := uint32(0); code < 28; code++ {
		glyph, gerr := ttf.CharCodeToGlyph(code)
		if gerr != nil {
			if code < 5 {
				t.Logf("  code=%d: err=%v", code, gerr)
			}
			continue
		}
		path, rerr := ttf.RenderGlyph(glyph, 100)
		cmds := 0
		if path != nil {
			cmds = len(path.Commands)
		}
		if cmds > 0 {
			rendered++
		}
		if code < 5 || cmds > 0 {
			t.Logf("  code=%d glyph=%d cmds=%d err=%v", code, glyph, cmds, rerr)
		}
	}
	t.Logf("Rendered glyphs: %d/28", rendered)

	// Render page
	doc, _ := pdf.Open("../../testdata/compare_legacy/pdfs/sample_002-trivial-libre-office-writer.pdf")
	defer doc.Close()
	page, _ := doc.Page(0)
	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72
	img, err := renderer.RenderPage(context.Background(), page, opts)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	bounds := img.Bounds()
	nonWhite := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if r < 0xFF00 {
				nonWhite++
			}
		}
	}
	t.Logf("Page render: %dx%d, non-white: %d", bounds.Dx(), bounds.Dy(), nonWhite)
	if nonWhite == 0 {
		t.Error("BLANK!")
	}
}

func extractObjectStream(data []byte, objNum int) []byte {
	pattern := fmt.Sprintf(`%d 0 obj`, objNum)
	re := regexp.MustCompile(pattern)
	loc := re.FindIndex(data)
	if loc == nil {
		return nil
	}

	// Find "stream\n" after the obj definition
	streamIdx := bytes.Index(data[loc[0]:], []byte("stream"))
	if streamIdx < 0 {
		return nil
	}
	start := loc[0] + streamIdx + 6
	if start < len(data) && data[start] == '\r' {
		start++
	}
	if start < len(data) && data[start] == '\n' {
		start++
	}

	endIdx := bytes.Index(data[start:], []byte("endstream"))
	if endIdx < 0 {
		return nil
	}
	raw := data[start : start+endIdx]

	// Try zlib decompress
	r, err := zlib.NewReader(bytes.NewReader(raw))
	if err == nil {
		decoded, _ := io.ReadAll(r)
		r.Close()
		if len(decoded) > 0 {
			return decoded
		}
	}
	return raw
}
