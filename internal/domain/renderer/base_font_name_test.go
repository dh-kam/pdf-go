package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeBaseFontName(t *testing.T) {
	assert.Equal(t, "Times-Roman", normalizeBaseFontName("KUYGUP+NimbusRomNo9L-Regu"))
	assert.Equal(t, "Times-Italic", normalizeBaseFontName("AZLOMJ+CMMI9"))
	assert.Equal(t, "Courier", normalizeBaseFontName("RRLDLB+CMTT9"))
	assert.Equal(t, "Courier", normalizeBaseFontName("CNQMHB+SFTT1095"))
	assert.Equal(t, "Helvetica", normalizeBaseFontName("CUBAGX+SFSX1440"))
	assert.Equal(t, "Helvetica-Bold", normalizeBaseFontName("OUCZRR+NimbusSanL-Bold"))
	assert.Equal(t, "Helvetica-Bold", normalizeBaseFontName("ZBVPYI+Calibri-Bold"))
	assert.Equal(t, "Helvetica", normalizeBaseFontName("ArialMT"))
	assert.Equal(t, "Helvetica-Bold", normalizeBaseFontName("Arial-BoldMT"))
	assert.Equal(t, "Helvetica-Oblique", normalizeBaseFontName("Arial-ItalicMT"))
	assert.Equal(t, "Helvetica-BoldOblique", normalizeBaseFontName("Arial-BoldItalicMT"))
	assert.Equal(t, "Courier", normalizeBaseFontName("CourierNewPSMT"))
	assert.Equal(t, "Times-BoldItalic", normalizeBaseFontName("TimesNewRomanPS-BoldItalicMT"))
	assert.Equal(t, "Symbol", normalizeBaseFontName("SymbolMT"))
}

func TestStripSubsetPrefix_TrimsSlashAndSubsetPrefix(t *testing.T) {
	assert.Equal(t, "CMR10", stripSubsetPrefix("/ABCDEF+CMR10"))
	assert.Equal(t, "SFRM1095", stripSubsetPrefix(" XYZABC+SFRM1095 "))
	assert.Equal(t, "Times-Roman", stripSubsetPrefix("Times-Roman"))
}
