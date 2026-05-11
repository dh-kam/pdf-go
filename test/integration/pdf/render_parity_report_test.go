package pdf_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"html"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

type parityRow struct {
	PDFPath           string
	Page              int
	PopplerPNG        string
	OursPNG           string
	ExactPercent      float64
	SimilarityPercent float64
	Pass              bool
	Error             string
}

func TestRenderParityReportAgainstPoppler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping render parity report in short mode")
	}

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	repoRoot := parityRepoRoot(t)
	scanRoot := parityEnvOrDefault("PDF_PARITY_SCAN_ROOT", getParityFixtureDir())
	outputRoot := parityEnvOrDefault("PDF_PARITY_OUTPUT_ROOT", filepath.Join(repoRoot, "test", "testdata", "output", "render_parity"))
	threshold := parityEnvFloat("PDF_PARITY_THRESHOLD", 99.0)
	dpi := parityEnvInt("PDF_PARITY_DPI", 150)

	require.NoError(t, os.RemoveAll(outputRoot))
	require.NoError(t, os.MkdirAll(outputRoot, 0o755))

	pdfFiles, err := parityFindPDFs(scanRoot)
	require.NoError(t, err)
	require.NotEmpty(t, pdfFiles, "No PDF files found under %s", scanRoot)

	rows := make([]parityRow, 0, 256)
	failCount := 0

	for _, pdfPath := range pdfFiles {
		relPath, err := filepath.Rel(scanRoot, pdfPath)
		require.NoError(t, err)

		t.Run(relPath, func(t *testing.T) {
			docRows, docFailCount := parityRenderOneDocument(t, scanRoot, outputRoot, pdfPath, relPath, dpi, threshold)
			rows = append(rows, docRows...)
			failCount += docFailCount
		})
	}

	reportPath := filepath.Join(outputRoot, "report.csv")
	require.NoError(t, parityWriteCSV(reportPath, rows))

	summaryPath := filepath.Join(outputRoot, "summary.md")
	require.NoError(t, parityWriteSummary(summaryPath, scanRoot, threshold, rows))

	htmlPath := filepath.Join(outputRoot, "index.html")
	require.NoError(t, parityWriteHTML(htmlPath, threshold, rows))

	t.Logf("Render parity report: %s", reportPath)
	t.Logf("Render parity summary: %s", summaryPath)
	t.Logf("Render parity html: %s", htmlPath)

	require.Zero(t, failCount, "Render parity target %.2f%% not met. See report: %s", threshold, reportPath)
}

func parityRenderOneDocument(t *testing.T, scanRoot, outputRoot, pdfPath, relPath string, dpi int, threshold float64) ([]parityRow, int) {
	t.Helper()

	rows := make([]parityRow, 0, 16)
	failCount := 0

	docID := paritySanitizeDocID(relPath)
	docRoot := filepath.Join(outputRoot, "docs", docID)
	popplerRoot := filepath.Join(docRoot, "poppler")
	oursRoot := filepath.Join(docRoot, "ours")
	password := parityPasswordForRelPath(relPath)
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, os.MkdirAll(oursRoot, 0o755))

	if err := parityRunPoppler(pdfPath, popplerRoot, dpi, password); err != nil {
		return append(rows, parityRow{PDFPath: relPath, Error: fmt.Sprintf("pdftoppm failed: %v", err)}), 1
	}

	popplerPages, err := parityListPopplerPages(popplerRoot)
	if err != nil {
		return append(rows, parityRow{PDFPath: relPath, Error: fmt.Sprintf("list poppler pages: %v", err)}), 1
	}
	if len(popplerPages) == 0 {
		return append(rows, parityRow{PDFPath: relPath, Error: "no poppler pages generated"}), 1
	}

	doc, err := parityOpenDocument(pdfPath, password)
	if err != nil {
		return append(rows, parityRow{PDFPath: relPath, Error: fmt.Sprintf("open pdf: %v", err)}), 1
	}
	defer doc.Close()

	pageCount, err := doc.PageCount()
	if err != nil {
		return append(rows, parityRow{PDFPath: relPath, Error: fmt.Sprintf("page count: %v", err)}), 1
	}
	if pageCount != len(popplerPages) {
		return append(rows, parityRow{
			PDFPath: relPath,
			Error:   fmt.Sprintf("page count mismatch: ours=%d poppler=%d", pageCount, len(popplerPages)),
		}), 1
	}

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	renderOptions := pdf.DefaultRenderOptions()
	renderOptions.DPI = float64(dpi)

	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		pageNumber := pageIndex + 1
		popplerPNG := popplerPages[pageNumber]
		oursPNG := filepath.Join(oursRoot, fmt.Sprintf("rendered_page_%04d.png", pageNumber))

		row := parityRow{
			PDFPath:    relPath,
			Page:       pageNumber,
			PopplerPNG: popplerPNG,
			OursPNG:    oursPNG,
		}

		page, err := doc.Page(pageIndex)
		if err != nil {
			row.Error = fmt.Sprintf("load page: %v", err)
			rows = append(rows, row)
			failCount++
			continue
		}

		img, err := renderer.RenderPage(context.Background(), page, renderOptions)
		if err != nil {
			row.Error = fmt.Sprintf("render page: %v", err)
			rows = append(rows, row)
			failCount++
			continue
		}

		if err := parityWritePNG(oursPNG, img); err != nil {
			row.Error = fmt.Sprintf("write ours png: %v", err)
			rows = append(rows, row)
			failCount++
			continue
		}

		exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPNG)
		if err != nil {
			row.Error = fmt.Sprintf("compare png: %v", err)
			rows = append(rows, row)
			failCount++
			continue
		}

		row.ExactPercent = exactPercent
		row.SimilarityPercent = similarityPercent
		row.Pass = similarityPercent >= threshold
		if !row.Pass {
			failCount++
		}
		rows = append(rows, row)
	}

	return rows, failCount
}

func parityRepoRoot(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to resolve current test file")
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))
}

func getParityFixtureDir() string {
	_, testFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(testFile), "testdata")
}

func parityEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parityEnvFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func parityEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parityFindPDFs(scanRoot string) ([]string, error) {
	pdfs := make([]string, 0, 64)
	err := filepath.Walk(scanRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".pdf") {
			pdfs = append(pdfs, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(pdfs)
	return pdfs, nil
}

func paritySanitizeDocID(relPath string) string {
	replacer := strings.NewReplacer("/", "__", "\\", "__", ":", "_", " ", "_")
	return replacer.Replace(relPath)
}

func parityRunPoppler(pdfPath, outDir string, dpi int, passwords ...string) error {
	password := ""
	if len(passwords) > 0 {
		password = passwords[0]
	}

	args := []string{"-png", "-r", strconv.Itoa(dpi)}
	if password != "" {
		args = append(args, "-upw", password)
	}
	args = append(args, pdfPath, filepath.Join(outDir, "rendered"))
	cmd := exec.Command("pdftoppm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func parityOpenDocument(pdfPath, password string) (*pdf.Document, error) {
	if password == "" {
		return pdf.Open(pdfPath)
	}
	return pdf.OpenWithPassword(pdfPath, password)
}

func parityPasswordForRelPath(relPath string) string {
	normalized := filepath.ToSlash(relPath)
	if strings.HasSuffix(normalized, "005-libreoffice-writer-password/libreoffice-writer-password.pdf") || normalized == "libreoffice-writer-password.pdf" {
		return "openpassword"
	}

	return ""
}

func parityListPopplerPages(popplerRoot string) (map[int]string, error) {
	entries, err := os.ReadDir(popplerRoot)
	if err != nil {
		return nil, err
	}
	pages := make(map[int]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "rendered-") || !strings.HasSuffix(name, ".png") {
			continue
		}
		pageValue := strings.TrimSuffix(strings.TrimPrefix(name, "rendered-"), ".png")
		pageNumber, err := strconv.Atoi(pageValue)
		if err != nil {
			return nil, fmt.Errorf("invalid poppler page name %q", name)
		}
		pages[pageNumber] = filepath.Join(popplerRoot, name)
	}
	return pages, nil
}

func parityWritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func parityComparePNGs(oursPath, popplerPath string) (float64, float64, error) {
	ours, err := parityLoadPNG(oursPath)
	if err != nil {
		return 0, 0, fmt.Errorf("load ours png: %w", err)
	}
	poppler, err := parityLoadPNG(popplerPath)
	if err != nil {
		return 0, 0, fmt.Errorf("load poppler png: %w", err)
	}
	if !ours.Bounds().Eq(poppler.Bounds()) {
		return 0, 0, fmt.Errorf("dimension mismatch: ours=%v poppler=%v", ours.Bounds(), poppler.Bounds())
	}

	bounds := ours.Bounds()
	totalPixels := bounds.Dx() * bounds.Dy()
	if totalPixels == 0 {
		return 0, 0, fmt.Errorf("empty image")
	}

	matchingPixels := 0
	totalDiff := 0.0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			or, og, ob, _ := ours.At(x, y).RGBA()
			pr, pg, pb, _ := poppler.At(x, y).RGBA()

			ourR := float64(or >> 8)
			ourG := float64(og >> 8)
			ourB := float64(ob >> 8)
			popR := float64(pr >> 8)
			popG := float64(pg >> 8)
			popB := float64(pb >> 8)

			if ourR == popR && ourG == popG && ourB == popB {
				matchingPixels++
			}

			totalDiff += math.Abs(ourR-popR) + math.Abs(ourG-popG) + math.Abs(ourB-popB)
		}
	}

	exactPercent := float64(matchingPixels) * 100.0 / float64(totalPixels)
	similarityPercent := 100.0 * (1.0 - totalDiff/(float64(totalPixels)*255.0*3.0))
	if similarityPercent < 0 {
		similarityPercent = 0
	}
	return exactPercent, similarityPercent, nil
}

func parityLoadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func parityWriteCSV(path string, rows []parityRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	if err := writer.Write([]string{
		"pdf",
		"page",
		"poppler_png",
		"ours_png",
		"exact_percent",
		"similarity_percent",
		"pass",
		"error",
	}); err != nil {
		return err
	}

	for _, row := range rows {
		record := []string{
			row.PDFPath,
			strconv.Itoa(row.Page),
			row.PopplerPNG,
			row.OursPNG,
			fmt.Sprintf("%.9f", row.ExactPercent),
			fmt.Sprintf("%.9f", row.SimilarityPercent),
			strconv.FormatBool(row.Pass),
			row.Error,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return writer.Error()
}

func parityWriteSummary(path, scanRoot string, threshold float64, rows []parityRow) error {
	totalPages := 0
	passPages := 0
	errorPages := 0
	totalSimilarity := 0.0

	for _, row := range rows {
		if row.Page > 0 {
			totalPages++
			totalSimilarity += row.SimilarityPercent
			if row.Pass {
				passPages++
			}
		}
		if row.Error != "" {
			errorPages++
		}
	}

	avgSimilarity := 0.0
	if totalPages > 0 {
		avgSimilarity = totalSimilarity / float64(totalPages)
	}

	content := strings.Join([]string{
		"# Render Parity Summary",
		"",
		fmt.Sprintf("- Scan root: `%s`", scanRoot),
		fmt.Sprintf("- Threshold: `%.2f%%`", threshold),
		fmt.Sprintf("- Total rows: `%d`", len(rows)),
		fmt.Sprintf("- Page rows: `%d`", totalPages),
		fmt.Sprintf("- Pass pages: `%d`", passPages),
		fmt.Sprintf("- Error rows: `%d`", errorPages),
		fmt.Sprintf("- Average similarity: `%.4f%%`", avgSimilarity),
		"",
		fmt.Sprintf("- Report CSV: `%s`", filepath.Join(filepath.Dir(path), "report.csv")),
	}, "\n")

	return os.WriteFile(path, []byte(content), 0o644)
}

func parityWriteHTML(path string, threshold float64, rows []parityRow) error {
	baseDir := filepath.Dir(path)
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>Render Parity Report</title>")
	b.WriteString("<style>")
	b.WriteString("body{font-family:system-ui,sans-serif;margin:24px;background:#f5f5f5;color:#111;}h1{margin:0 0 8px;}p{margin:0 0 16px;}table{width:100%;border-collapse:collapse;background:#fff;}th,td{border:1px solid #ddd;padding:8px;vertical-align:top;font-size:14px;}th{background:#f0f0f0;position:sticky;top:0;}tr.fail{background:#fff1f1;}tr.error{background:#fff7e6;}img{display:block;max-width:320px;height:auto;border:1px solid #ccc;background:#fff;}code{font-family:ui-monospace,monospace;font-size:12px;word-break:break-all;}.pass{color:#0a7a28;font-weight:600;}.failtxt{color:#b42318;font-weight:600;}.errtxt{color:#b54708;font-weight:600;}")
	b.WriteString("</style></head><body>")
	b.WriteString("<h1>Render Parity Report</h1>")
	b.WriteString("<p>")
	b.WriteString(fmt.Sprintf("Threshold: <strong>%.2f%%</strong>", threshold))
	b.WriteString("</p>")
	b.WriteString("<table><thead><tr>")
	b.WriteString("<th>PDF</th><th>Page</th><th>Similarity</th><th>Exact</th><th>Status</th><th>Poppler</th><th>Ours</th><th>Error</th>")
	b.WriteString("</tr></thead><tbody>")

	for _, row := range rows {
		className := ""
		status := "<span class=\"pass\">PASS</span>"
		if row.Error != "" {
			className = "error"
			status = "<span class=\"errtxt\">ERROR</span>"
		} else if !row.Pass {
			className = "fail"
			status = "<span class=\"failtxt\">FAIL</span>"
		}

		b.WriteString("<tr")
		if className != "" {
			b.WriteString(" class=\"")
			b.WriteString(className)
			b.WriteString("\"")
		}
		b.WriteString(">")
		b.WriteString("<td><code>")
		b.WriteString(html.EscapeString(row.PDFPath))
		b.WriteString("</code></td>")
		b.WriteString("<td>")
		if row.Page > 0 {
			b.WriteString(strconv.Itoa(row.Page))
		}
		b.WriteString("</td>")
		b.WriteString("<td>")
		if row.Page > 0 {
			b.WriteString(fmt.Sprintf("%.4f%%", row.SimilarityPercent))
		}
		b.WriteString("</td>")
		b.WriteString("<td>")
		if row.Page > 0 {
			b.WriteString(fmt.Sprintf("%.4f%%", row.ExactPercent))
		}
		b.WriteString("</td>")
		b.WriteString("<td>")
		b.WriteString(status)
		b.WriteString("</td>")
		b.WriteString("<td>")
		if row.PopplerPNG != "" {
			relPoppler, err := filepath.Rel(baseDir, row.PopplerPNG)
			if err != nil {
				return err
			}
			b.WriteString("<img src=\"")
			b.WriteString(html.EscapeString(filepath.ToSlash(relPoppler)))
			b.WriteString("\" alt=\"poppler\">")
		}
		b.WriteString("</td>")
		b.WriteString("<td>")
		if row.OursPNG != "" {
			relOurs, err := filepath.Rel(baseDir, row.OursPNG)
			if err != nil {
				return err
			}
			b.WriteString("<img src=\"")
			b.WriteString(html.EscapeString(filepath.ToSlash(relOurs)))
			b.WriteString("\" alt=\"ours\">")
		}
		b.WriteString("</td>")
		b.WriteString("<td><code>")
		b.WriteString(html.EscapeString(row.Error))
		b.WriteString("</code></td></tr>")
	}

	b.WriteString("</tbody></table></body></html>")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
