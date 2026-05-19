package pdf_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dh-kam/pdf-go/pkg/pdf"
	"github.com/stretchr/testify/require"
)

func renderPageSimilarityAgainstPopplerForProbe(
	t *testing.T,
	target realPageProbeTarget,
	popplerPNG string,
	env map[string]string,
) float64 {
	return renderPageSimilarityAgainstPopplerForProbeAtDPI(t, target, popplerPNG, env, defaultRealPageProbeDPI)
}

func renderPageSimilarityAgainstPopplerForProbeAtDPI(
	t *testing.T,
	target realPageProbeTarget,
	popplerPNG string,
	env map[string]string,
	dpi int,
) float64 {
	t.Helper()

	doc, err := pdf.Open(target.pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(target.pageNumber - 1)
	require.NoError(t, err)

	restore := setProbeEnvForRender(t, env)
	defer restore()

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(dpi)

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)

	oursPNG := filepath.Join(t.TempDir(), "ours.png")
	require.NoError(t, parityWritePNG(oursPNG, img))

	_, similarity, err := parityComparePNGs(oursPNG, popplerPNG)
	require.NoError(t, err)
	return similarity
}

func preparePopplerPageForProbe(t *testing.T, target realPageProbeTarget) string {
	return preparePopplerPageForProbeAtDPI(t, target, defaultRealPageProbeDPI)
}

func preparePopplerPageForProbeAtDPI(t *testing.T, target realPageProbeTarget, dpi int) string {
	t.Helper()
	requirePopplerProbeOptIn(t)

	root := t.TempDir()
	popplerRoot := filepath.Join(root, "poppler")

	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, renderPopplerPageForProbe(target.pdfPath, target.pageNumber, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, target.pageNumber)
	return popplerPages[target.pageNumber]
}

func renderPopplerPageForProbe(pdfPath string, pageNumber int, outDir string, dpi int) error {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return err
	}

	cmd := exec.Command(
		"pdftoppm",
		"-f", fmt.Sprintf("%d", pageNumber),
		"-l", fmt.Sprintf("%d", pageNumber),
		"-png",
		"-r", fmt.Sprintf("%d", dpi),
		pdfPath,
		filepath.Join(outDir, "rendered"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}
