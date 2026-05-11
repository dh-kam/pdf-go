package pdf_test

import (
	"os/exec"
	"testing"
)

func TestType1RealPageParityProbe(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	targets := []realPageProbeTarget{
		{
			name:       "009_p95_sfrm1095",
			pdfPath:    getSampleDir() + "/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNumber: 95,
		},
		{
			name:       "009_p109_sfrm1095",
			pdfPath:    getSampleDir() + "/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNumber: 109,
		},
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, target)
			similarity := renderPageSimilarityAgainstPopplerForProbe(t, target, popplerPNG, nil)
			t.Logf("current_similarity=%.4f", similarity)
		})
	}
}
