package freetype

import (
	"fmt"
	"os"
	"testing"
)

func TestBitmapDimensions(t *testing.T) {
	// Use the embedded CMR10 font from pdflatex-4-pages.pdf
	// First, try to get font data from the test file
	fontPath := "/workspace/pdf-reader/go-pdf/test/testdata/sample-files/004-pdflatex-4-pages/pdflatex-4-pages.pdf"
	if _, err := os.Stat(fontPath); err != nil {
		t.Skip("test PDF not found")
	}
	
	// We need raw Type1 font data to test. Let's use a system font instead.
	// Try /usr/share/fonts/type1/gsfonts/n021003l.pfb (Nimbus Roman = Times-Roman)
	pfbPaths := []string{
		"/usr/share/fonts/type1/gsfonts/n021003l.pfb",
		"/usr/share/ghostscript/9.55.0/Resource/Font/Times-Roman",
		"/usr/share/fonts/X11/Type1/NimbusRoman-Regular.pfb",
		"/usr/share/fonts/X11/Type1/C059-Italic.pfb",
	}
	
	var fontData []byte
	for _, p := range pfbPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			fontData = data
			fmt.Printf("Using font: %s (%d bytes)\n", p, len(fontData))
			break
		}
	}
	
	if len(fontData) == 0 {
		t.Skip("no system Type1 font found")
	}
	
	// Render 'A' at sizePt=10, dpi=1501 (like our compute)
	sizePt := 10.0
	dpi := 1501
	glyph := uint32('A') // 65
	
	buf, bw, bh, bleft, btop, err := RenderGlyphBitmap(fontData, glyph, sizePt, dpi)
	if err != nil {
		t.Logf("RenderGlyphBitmap at dpi=%d: %v", dpi, err)
	} else {
		t.Logf("'A' at sizePt=%.0f dpi=%d: buf_len=%d bw=%d bh=%d bleft=%d btop=%d", sizePt, dpi, len(buf), bw, bh, bleft, btop)
	}
	
	// Same at 150 DPI
	buf2, bw2, bh2, bleft2, btop2, err2 := RenderGlyphBitmap(fontData, glyph, sizePt, 150)
	if err2 != nil {
		t.Logf("RenderGlyphBitmap at dpi=150: %v", err2)
	} else {
		t.Logf("'A' at sizePt=%.0f dpi=150: buf_len=%d bw=%d bh=%d bleft=%d btop=%d", sizePt, len(buf2), bw2, bh2, bleft2, btop2)
	}
}
