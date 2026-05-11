package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestGeoTopoPageCountRegression(t *testing.T) {
	sampleDir := getSampleDir()

	cases := []struct {
		name     string
		relPath  string
		expected int
	}{
		{
			name:     "GeoTopo",
			relPath:  "009-pdflatex-geotopo/GeoTopo.pdf",
			expected: 117,
		},
		{
			name:     "GeoTopo-komprimiert",
			relPath:  "009-pdflatex-geotopo/GeoTopo-komprimiert.pdf",
			expected: 117,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			doc, err := pdf.Open(filepath.Join(sampleDir, filepath.FromSlash(tc.relPath)))
			require.NoError(t, err)
			defer doc.Close()

			pageCount, err := doc.PageCount()
			require.NoError(t, err)
			require.Equal(t, tc.expected, pageCount)
		})
	}
}
