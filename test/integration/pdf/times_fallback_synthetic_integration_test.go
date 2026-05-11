package pdf_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSyntheticTimesRomanTextProbe_CurrentBeatsHelveticaButCourierCanOutscoreCurrent(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "times_roman_text.pdf")
	popplerRoot := filepath.Join(root, "poppler")

	require.NoError(t, os.WriteFile(pdfPath, buildSyntheticTimesRomanTextPDF("etonia etonia"), 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	testCases := []struct {
		name string
		env  map[string]string
	}{
		{name: "current"},
		{name: "force_helvetica", env: map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "Times-Roman=Helvetica"}},
		{name: "force_courier", env: map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "Times-Roman=Courier"}},
	}

	scores := make(map[string]parityScore, len(testCases))
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := pdf.Open(pdfPath)
			require.NoError(t, err)
			defer doc.Close()

			page, err := doc.Page(0)
			require.NoError(t, err)

			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
			opts := pdf.DefaultRenderOptions()
			opts.DPI = 72

			img, err := renderer.RenderPage(context.Background(), page, opts)
			require.NoError(t, err)

			oursPNG := filepath.Join(root, fmt.Sprintf("%s.png", tc.name))
			require.NoError(t, parityWritePNG(oursPNG, img))

			exact, similarity, err := parityComparePNGs(oursPNG, popplerPages[1])
			require.NoError(t, err)

			scores[tc.name] = parityScore{
				exact:      exact,
				similarity: similarity,
			}
			t.Logf("%s exact=%.4f similarity=%.4f", tc.name, exact, similarity)
		})
	}

	require.Greater(t, scores["current"].similarity, scores["force_helvetica"].similarity)
	require.Greater(t, scores["force_courier"].similarity, scores["current"].similarity)
}

func buildSyntheticTimesRomanTextPDF(text string) []byte {
	content := []byte(fmt.Sprintf("BT\n/F1 48 Tf\n1 0 0 1 12 24 Tm\n(%s) Tj\nET\n", text))
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 220 80] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
		syntheticStreamObject(fmt.Sprintf("<< /Length %d >>", len(content)), content),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman /Encoding /WinAnsiEncoding >>"),
	}
	return buildSyntheticPDF(objects)
}
