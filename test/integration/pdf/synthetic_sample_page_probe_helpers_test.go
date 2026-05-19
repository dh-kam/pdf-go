package pdf_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	infrarenderer "github.com/dh-kam/pdf-go/internal/infrastructure/renderer"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

type imageDiffBoundsProbeResult struct {
	bounds  image.Rectangle
	hasDiff bool
}

type shiftedImageParityProbeResult struct {
	exact      float64
	similarity float64
	shiftX     int
	shiftY     int
}

type samplePagePlacementProbeResult struct {
	originalExact       float64
	originalSimilarity  float64
	bestShiftExact      float64
	bestShiftSimilarity float64
	bestShiftX          int
	bestShiftY          int
	diffBounds          image.Rectangle
	hasDiffBounds       bool
}

type imageSamplingTraceProbeResult struct {
	filter                string
	colorSpace            string
	sampler               string
	reason                string
	experimentalCandidate string
	ctm                   [6]float64
	phaseX                float64
	phaseY                float64
	dstX                  float64
	dstY                  float64
	dstW                  float64
	dstH                  float64
	srcW                  int
	srcH                  int
}

func (r samplePagePlacementProbeResult) improvement() float64 {
	return r.bestShiftSimilarity - r.originalSimilarity
}

var imageSamplingTraceProbePattern = regexp.MustCompile(
	`^\[image-sampling\] doc=.* page=\d+ filter=([^ ]+) colorspace=([^ ]+).* sampler=([^ ]+) reason=([^ ]+) experimental_candidate=([^ ]+) ctm=\[([^\]]+)\] phase=\(x=([-0-9.]+) y=([-0-9.]+)\) dst=\(x=([-0-9.]+) y=([-0-9.]+) w=([-0-9.]+) h=([-0-9.]+)\) src=(\d+)x(\d+)$`,
)

func measureSamplePagePlacementProbeAgainstPopplerAtDPI(
	t *testing.T,
	pdfPath string,
	pageNum int,
	mode string,
	dpi int,
	searchRadius int,
) samplePagePlacementProbeResult {
	t.Helper()

	popplerPNG, oursPNG := renderSamplePageAgainstPopplerAtDPIToPNGs(t, pdfPath, pageNum, mode, dpi)

	return measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, searchRadius)
}

func measureSyntheticPDFPlacementProbeAgainstPopplerAtDPI(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	dpi int,
	searchRadius int,
) samplePagePlacementProbeResult {
	return measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t,
		pdfName,
		pdfBytes,
		dpi,
		searchRadius,
		"",
	)
}

func measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	dpi int,
	searchRadius int,
	mode string,
) samplePagePlacementProbeResult {
	t.Helper()

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(dpi)
	if mode != "" {
		opts.ImageSamplingMode = mode
	}

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	return measurePNGPlacementProbeForProbe(t, popplerPages[1], oursPNG, searchRadius)
}

func probeSyntheticPDFModeAndReferencesAgainstPopplerAtDPI(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	dpi int,
	mode string,
	references map[string]image.Image,
) (float64, map[string]float64) {
	t.Helper()

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)
	popplerPNG := popplerPages[1]

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(dpi)
	if mode != "" {
		opts.ImageSamplingMode = mode
	}

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	_, currentSimilarity, err := parityComparePNGs(oursPNG, popplerPNG)
	require.NoError(t, err)

	refScores := make(map[string]float64, len(references))
	for name, ref := range references {
		refPath := filepath.Join(root, name+".png")
		require.NoError(t, parityWritePNG(refPath, ref))
		_, similarity, err := parityComparePNGs(refPath, popplerPNG)
		require.NoError(t, err)
		refScores[name] = similarity
	}

	return currentSimilarity, refScores
}

func measurePNGPlacementProbeForProbe(
	t *testing.T,
	popplerPNG string,
	oursPNG string,
	searchRadius int,
) samplePagePlacementProbeResult {
	t.Helper()

	originalExact, originalSimilarity, err := parityComparePNGs(oursPNG, popplerPNG)
	require.NoError(t, err)

	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
	diffBounds := imageDifferenceBoundsForProbe(popplerImg, oursImg)
	bestShift := bestShiftedImageParityForProbe(t, popplerImg, oursImg, searchRadius)

	return samplePagePlacementProbeResult{
		originalExact:       originalExact,
		originalSimilarity:  originalSimilarity,
		bestShiftExact:      bestShift.exact,
		bestShiftSimilarity: bestShift.similarity,
		bestShiftX:          bestShift.shiftX,
		bestShiftY:          bestShift.shiftY,
		diffBounds:          diffBounds.bounds,
		hasDiffBounds:       diffBounds.hasDiff,
	}
}

func renderSamplePageAgainstPopplerAtDPIToPNGs(
	t *testing.T,
	pdfPath string,
	pageNum int,
	mode string,
	dpi int,
) (string, string) {
	t.Helper()
	requirePopplerProbeOptIn(t)

	root := t.TempDir()
	popplerRoot := filepath.Join(root, "poppler")
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, pageNum)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(dpi)
	opts.ImageSamplingMode = mode

	oursPNG := filepath.Join(root, "ours.png")
	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	return popplerPages[pageNum], oursPNG
}

func renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	mode string,
	dpi int,
) (string, string) {
	t.Helper()
	requirePopplerProbeOptIn(t)

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(dpi)
	opts.ImageSamplingMode = mode

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	return popplerPages[1], oursPNG
}

func renderSamplePageImageSamplingTraceForProbe(
	t *testing.T,
	pdfPath string,
	pageNum int,
	mode string,
	dpi int,
) []imageSamplingTraceProbeResult {
	t.Helper()

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(pageNum - 1)
	require.NoError(t, err)

	opts := domainrenderer.DefaultRenderOptions()
	opts.DPI = float64(dpi)
	opts.EnableCache = false
	opts.DebugImageSampling = true
	opts.ImageSamplingMode = mode

	renderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	traceOutput := captureImageSamplingTraceOutputForProbe(t, func() {
		_, renderErr := renderer.RenderPage(context.Background(), page, opts)
		require.NoError(t, renderErr)
	})

	return parseImageSamplingTraceForProbe(t, traceOutput)
}

func renderSyntheticPDFImageSamplingTraceForProbe(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	mode string,
	dpi int,
) []imageSamplingTraceProbeResult {
	t.Helper()

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))

	return renderSamplePageImageSamplingTraceForProbe(t, pdfPath, 1, mode, dpi)
}

func captureImageSamplingTraceOutputForProbe(t *testing.T, render func()) string {
	t.Helper()

	originalStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = writePipe
	defer func() {
		os.Stderr = originalStderr
	}()

	render()

	require.NoError(t, writePipe.Close())
	traceBytes, err := io.ReadAll(readPipe)
	require.NoError(t, err)
	require.NoError(t, readPipe.Close())

	return string(bytes.TrimSpace(traceBytes))
}

func parseImageSamplingTraceForProbe(
	t *testing.T,
	traceOutput string,
) []imageSamplingTraceProbeResult {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(traceOutput), "\n")
	results := make([]imageSamplingTraceProbeResult, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := imageSamplingTraceProbePattern.FindStringSubmatch(line)
		require.Lenf(t, match, 15, "unexpected image sampling trace: %s", line)

		results = append(results, imageSamplingTraceProbeResult{
			filter:                match[1],
			colorSpace:            match[2],
			sampler:               match[3],
			reason:                match[4],
			experimentalCandidate: match[5],
			ctm:                   parseTraceCTMForProbe(t, match[6]),
			phaseX:                parseTraceFloatForProbe(t, match[7]),
			phaseY:                parseTraceFloatForProbe(t, match[8]),
			dstX:                  parseTraceFloatForProbe(t, match[9]),
			dstY:                  parseTraceFloatForProbe(t, match[10]),
			dstW:                  parseTraceFloatForProbe(t, match[11]),
			dstH:                  parseTraceFloatForProbe(t, match[12]),
			srcW:                  parseTraceIntForProbe(t, match[13]),
			srcH:                  parseTraceIntForProbe(t, match[14]),
		})
	}

	return results
}

func parseTraceCTMForProbe(t *testing.T, raw string) [6]float64 {
	t.Helper()

	fields := strings.Fields(raw)
	require.Len(t, fields, 6)

	var out [6]float64
	for i, field := range fields {
		out[i] = parseTraceFloatForProbe(t, field)
	}

	return out
}

func parseTraceFloatForProbe(t *testing.T, raw string) float64 {
	t.Helper()

	value, err := strconv.ParseFloat(raw, 64)
	require.NoError(t, err)
	return value
}

func parseTraceIntForProbe(t *testing.T, raw string) int {
	t.Helper()

	value, err := strconv.Atoi(raw)
	require.NoError(t, err)
	return value
}

func loadPNGAsRGBAForProbe(t *testing.T, path string) *image.RGBA {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	require.NoError(t, err)

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba
}

func imageDifferenceBoundsForProbe(left, right *image.RGBA) imageDiffBoundsProbeResult {
	if !left.Bounds().Eq(right.Bounds()) {
		return imageDiffBoundsProbeResult{
			bounds:  left.Bounds().Union(right.Bounds()),
			hasDiff: true,
		}
	}

	minX := left.Bounds().Max.X
	minY := left.Bounds().Max.Y
	maxX := left.Bounds().Min.X - 1
	maxY := left.Bounds().Min.Y - 1
	hasDiff := false

	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			if left.RGBAAt(x, y) == right.RGBAAt(x, y) {
				continue
			}
			hasDiff = true
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}

	if !hasDiff {
		return imageDiffBoundsProbeResult{}
	}

	return imageDiffBoundsProbeResult{
		bounds:  image.Rect(minX, minY, maxX+1, maxY+1),
		hasDiff: true,
	}
}

func bestShiftedImageParityForProbe(
	t *testing.T,
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	searchRadius int,
) shiftedImageParityProbeResult {
	t.Helper()

	root := t.TempDir()
	popplerPNG := filepath.Join(root, "poppler.png")
	require.NoError(t, parityWritePNG(popplerPNG, popplerImg))

	best := shiftedImageParityProbeResult{
		exact:      -1,
		similarity: -1,
	}

	for dy := -searchRadius; dy <= searchRadius; dy++ {
		for dx := -searchRadius; dx <= searchRadius; dx++ {
			shifted := shiftRGBAForProbe(oursImg, dx, dy)
			shiftedPNG := filepath.Join(root, shiftedImageProbeName(dx, dy))
			require.NoError(t, parityWritePNG(shiftedPNG, shifted))

			exact, similarity, err := parityComparePNGs(shiftedPNG, popplerPNG)
			require.NoError(t, err)

			candidate := shiftedImageParityProbeResult{
				exact:      exact,
				similarity: similarity,
				shiftX:     dx,
				shiftY:     dy,
			}
			if shiftedImageParityBetterForProbe(candidate, best) {
				best = candidate
			}
		}
	}

	return best
}

func shiftedImageParityBetterForProbe(candidate, best shiftedImageParityProbeResult) bool {
	if candidate.similarity != best.similarity {
		return candidate.similarity > best.similarity
	}
	if candidate.exact != best.exact {
		return candidate.exact > best.exact
	}

	candidateDistance := absIntForProbe(candidate.shiftX) + absIntForProbe(candidate.shiftY)
	bestDistance := absIntForProbe(best.shiftX) + absIntForProbe(best.shiftY)
	if candidateDistance != bestDistance {
		return candidateDistance < bestDistance
	}
	if candidate.shiftY != best.shiftY {
		return candidate.shiftY < best.shiftY
	}
	return candidate.shiftX < best.shiftX
}

func shiftRGBAForProbe(src *image.RGBA, dx, dy int) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			sx := x - dx
			sy := y - dy
			if sx < src.Bounds().Min.X || sx >= src.Bounds().Max.X ||
				sy < src.Bounds().Min.Y || sy >= src.Bounds().Max.Y {
				continue
			}
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

func shiftedImageProbeName(dx, dy int) string {
	return filepath.Base(
		filepath.Join(
			".",
			"shift_"+itoaForProbe(dx)+"_"+itoaForProbe(dy)+".png",
		),
	)
}

func itoaForProbe(v int) string {
	if v < 0 {
		return "neg" + itoaForProbe(-v)
	}
	if v == 0 {
		return "0"
	}

	digits := make([]byte, 0, 4)
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func absIntForProbe(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func filledRGBAForProbe(bounds image.Rectangle, values [][]uint8) *image.RGBA {
	img := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			v := values[y-bounds.Min.Y][x-bounds.Min.X]
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}
