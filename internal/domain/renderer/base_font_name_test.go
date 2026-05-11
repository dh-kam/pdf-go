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
}

func TestStripSubsetPrefix_TrimsSlashAndSubsetPrefix(t *testing.T) {
	assert.Equal(t, "CMR10", stripSubsetPrefix("/ABCDEF+CMR10"))
	assert.Equal(t, "SFRM1095", stripSubsetPrefix(" XYZABC+SFRM1095 "))
	assert.Equal(t, "Times-Roman", stripSubsetPrefix("Times-Roman"))
}
