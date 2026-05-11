package pdf_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

var lexOperatorCoverage = map[string][]string{
	"002_text.pdf":                            {"BT", "ET", "Tf", "Tj"},
	"003_path.pdf":                            {"m", "l", "re", "S"},
	"004_fill.pdf":                            {"re", "f", "rg"},
	"005_stroke.pdf":                          {"S", "w"},
	"006_transform.pdf":                       {"cm"},
	"007_clipping.pdf":                        {"W", "n"},
	"008_gradient.pdf":                        {"rg", "re", "f"},
	"009_graphics_state.pdf":                  {"q", "Q", "rg", "re", "f"},
	"010_curveto.pdf":                         {"m", "c", "S"},
	"011_closepath.pdf":                       {"m", "l", "h", "B"},
	"012_lineto.pdf":                          {"m", "l", "h", "B"},
	"013_moveto.pdf":                          {"m", "l", "h", "B"},
	"014_text_matrix.pdf":                     {"BT", "ET", "Tf", "Tj", "Tm", "cm"},
	"015_text_spacing.pdf":                    {"BT", "ET", "Tf", "Tj", "Tc", "Tw"},
	"016_linecap.pdf":                         {"J", "S"},
	"017_linejoin.pdf":                        {"j", "S"},
	"018_dash.pdf":                            {"d", "S"},
	"019_simple_image.pdf":                    {"Do"},
	"020_rect.pdf":                            {"re", "B"},
	"021_fillstroke.pdf":                      {"re", "B"},
	"022_eofill.pdf":                          {"m", "c", "f*"},
	"023_text_modes.pdf":                      {"BT", "ET", "Tf", "Td", "Tj", "Tr"},
	"024_clip.pdf":                            {"m", "c", "W", "n", "re", "f"},
	"025_eoclip.pdf":                          {"m", "c", "W*", "n", "re", "f"},
	"026_simple_clip.pdf":                     {"re", "W", "n", "f"},
	"030_clip_rect.pdf":                       {"re", "W", "n", "f"},
	"031_clip_eo.pdf":                         {"re", "W*", "n", "f"},
	"032_clip_circle.pdf":                     {"c", "W", "n", "f"},
	"033_clip_stroke.pdf":                     {"re", "W", "n", "f", "S"},
	"034_clip_polygon.pdf":                    {"m", "l", "W", "n", "f"},
	"035_clip_nested.pdf":                     {"re", "W", "W*", "f"},
	"036_clip_curve.pdf":                      {"m", "c", "W", "n", "re", "f"},
	"037_clip_eo_overlap.pdf":                 {"re", "W*", "n", "f"},
	"040_path_moveto_lineto.pdf":              {"m", "l", "S"},
	"041_pathto_curveto.pdf":                  {"m", "c", "S"},
	"042_pathto_closepath.pdf":                {"m", "l", "h", "S"},
	"043_pathto_rectangle.pdf":                {"re", "f"},
	"044_path_stroke.pdf":                     {"m", "l", "S"},
	"045_path_transform.pdf":                  {"cm", "m", "l", "S"},
	"046_path_multiple.pdf":                   {"m", "l", "S"},
	"047_pathto_curve_complex.pdf":            {"m", "c", "S"},
	"048_triangle.pdf":                        {"m", "l", "h", "f"},
	"049_stroke_simple.pdf":                   {"m", "l", "S"},
	"050_text_leading.pdf":                    {"BT", "ET", "Tf", "Td", "TL", "T*", "Tj"},
	"051_text_hscale.pdf":                     {"BT", "ET", "Tf", "Td", "Tz", "Tj"},
	"052_text_rise.pdf":                       {"BT", "ET", "Tf", "Td", "Ts", "Tj"},
	"053_color_operators.pdf":                 {"g", "G", "rg", "RG", "k", "K", "re", "f", "S", "B"},
	"054_inline_image_bi.pdf":                 {"q", "Q", "cm", "BI", "ID", "EI"},
	"055_colorspace_operators.pdf":            {"cs", "CS", "sc", "SC", "re", "f", "S", "B"},
	"056_text_td_td.pdf":                      {"BT", "ET", "Tf", "Td", "TD", "Tj"},
	"057_curve_shortcuts_vy.pdf":              {"m", "v", "y", "S"},
	"058_xobject_do_form.pdf":                 {"q", "Q", "cm", "Do"},
	"059_shading_operator.pdf":                {"re", "W", "n", "sh"},
	"060_color_n_operators.pdf":               {"cs", "CS", "scn", "SCN", "re", "f", "S"},
	"061_text_array_tj.pdf":                   {"BT", "ET", "Tf", "Td", "TJ"},
	"062_close_stroke_s.pdf":                  {"m", "l", "s"},
	"063_fill_stroke_evenodd_bstar.pdf":       {"m", "l", "h", "B*"},
	"064_close_fill_stroke_b.pdf":             {"m", "l", "b"},
	"065_close_fill_stroke_evenodd_bstar.pdf": {"m", "l", "b*"},
}

var lexTrackedOperators = []string{
	"BT", "ET", "Tf", "Tj", "TJ", "Tm", "Td", "TD", "Tc", "Tw", "Tr", "TL", "T*", "Tz", "Ts",
	"m", "l", "c", "v", "y", "h", "re", "S", "s", "f", "f*", "B", "B*", "b", "b*", "n", "W", "W*",
	"q", "Q", "cm", "J", "j", "d",
	"Do", "BI", "ID", "EI",
	"CS", "cs", "SC", "SCN", "sc", "scn", "G", "g", "RG", "rg", "K", "k",
	"sh",
	"d0", "d1",
}

var lexUnsupportedOperators = map[string]string{
	"d0": "Type3 glyph metrics operator is not implemented and is currently treated as a no-op",
	"d1": "Type3 glyph metrics operator is not implemented and is currently treated as a no-op",
}

func TestLexRenderParityAgainstPoppler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping lex render parity report in short mode")
	}

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	repoRoot := parityRepoRoot(t)
	workspaceRoot := filepath.Clean(filepath.Join(repoRoot, ".."))
	scanRoot := parityEnvOrDefault("PDF_LEX_PARITY_SCAN_ROOT", filepath.Join(workspaceRoot, "tmp", "lex_tests"))
	outputRoot := parityEnvOrDefault("PDF_LEX_PARITY_OUTPUT_ROOT", filepath.Join(repoRoot, "test", "testdata", "output", "lex_render_parity"))
	threshold := parityEnvFloat("PDF_LEX_PARITY_THRESHOLD", 99.0)
	dpi := parityEnvInt("PDF_LEX_PARITY_DPI", 72)

	require.NoError(t, os.RemoveAll(outputRoot))
	require.NoError(t, os.MkdirAll(outputRoot, 0o755))

	pdfFiles, err := lexFindPDFs(scanRoot)
	require.NoError(t, err)
	require.NotEmpty(t, pdfFiles, "No lex PDFs found under %s", scanRoot)

	rows := make([]parityRow, 0, len(pdfFiles))
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
	require.NoError(t, lexWriteSummary(summaryPath, scanRoot, threshold, rows))

	htmlPath := filepath.Join(outputRoot, "index.html")
	require.NoError(t, parityWriteHTML(htmlPath, threshold, rows))

	t.Logf("Lex render parity report: %s", reportPath)
	t.Logf("Lex render parity summary: %s", summaryPath)
	t.Logf("Lex render parity html: %s", htmlPath)

	require.Zero(t, failCount, "Lex render parity target %.2f%% not met. See report: %s", threshold, reportPath)
}

func lexFindPDFs(scanRoot string) ([]string, error) {
	entries, err := os.ReadDir(scanRoot)
	if err != nil {
		return nil, err
	}

	pdfs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".pdf") {
			continue
		}
		if len(name) < 4 || name[0] < '0' || name[0] > '9' {
			continue
		}
		path := filepath.Join(scanRoot, name)
		doc, err := pdf.Open(path)
		if err != nil {
			return nil, err
		}
		pageCount, err := doc.PageCount()
		doc.Close()
		if err != nil {
			return nil, err
		}
		if pageCount == 0 {
			continue
		}
		pdfs = append(pdfs, path)
	}
	sort.Strings(pdfs)
	return pdfs, nil
}

func TestLexFindPDFsSkipsNonNumericFixtures(t *testing.T) {
	scanRoot := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(scanRoot, "066_numeric.pdf"), buildType3SmokePDF(false), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scanRoot, "type3_d0_smoke.pdf"), buildType3SmokePDF(false), 0o644))

	pdfs, err := lexFindPDFs(scanRoot)
	require.NoError(t, err)
	require.Len(t, pdfs, 1)
	require.Equal(t, filepath.Join(scanRoot, "066_numeric.pdf"), pdfs[0])
}

func lexWriteSummary(path, scanRoot string, threshold float64, rows []parityRow) error {
	type lexResult struct {
		name       string
		exact      float64
		similarity float64
		err        string
	}

	confirmed := make([]lexResult, 0, len(rows))
	unconfirmed := make([]lexResult, 0, len(rows))
	errorCount := 0
	coveredOps := make(map[string]struct{}, len(lexTrackedOperators))
	trackedOps := make(map[string]struct{}, len(lexTrackedOperators))
	supportedCoveredCount := 0
	supportedTrackedCount := 0
	for _, op := range lexTrackedOperators {
		trackedOps[op] = struct{}{}
		if _, unsupported := lexUnsupportedOperators[op]; !unsupported {
			supportedTrackedCount++
		}
	}

	for _, row := range rows {
		result := lexResult{
			name:       row.PDFPath,
			exact:      row.ExactPercent,
			similarity: row.SimilarityPercent,
			err:        row.Error,
		}
		if row.Error != "" {
			errorCount++
			unconfirmed = append(unconfirmed, result)
			continue
		}
		if row.Pass {
			confirmed = append(confirmed, result)
			for _, op := range lexOperatorCoverage[row.PDFPath] {
				if _, tracked := trackedOps[op]; !tracked {
					continue
				}
				if _, seen := coveredOps[op]; seen {
					continue
				}
				coveredOps[op] = struct{}{}
				if _, unsupported := lexUnsupportedOperators[op]; !unsupported {
					supportedCoveredCount++
				}
			}
			continue
		}
		unconfirmed = append(unconfirmed, result)
	}

	sort.Slice(confirmed, func(i, j int) bool {
		return confirmed[i].name < confirmed[j].name
	})
	sort.Slice(unconfirmed, func(i, j int) bool {
		if unconfirmed[i].err != "" || unconfirmed[j].err != "" {
			if unconfirmed[i].err == "" {
				return true
			}
			if unconfirmed[j].err == "" {
				return false
			}
		}
		if unconfirmed[i].similarity == unconfirmed[j].similarity {
			return unconfirmed[i].name < unconfirmed[j].name
		}
		return unconfirmed[i].similarity < unconfirmed[j].similarity
	})

	var b strings.Builder
	b.WriteString("# Lex Render Parity Summary\n\n")
	b.WriteString(fmt.Sprintf("- Scan root: `%s`\n", scanRoot))
	b.WriteString(fmt.Sprintf("- Threshold: `%.2f%%`\n", threshold))
	b.WriteString(fmt.Sprintf("- Total lex cases: `%d`\n", len(rows)))
	b.WriteString(fmt.Sprintf("- Confirmed (`>= %.2f%%`): `%d`\n", threshold, len(confirmed)))
	b.WriteString(fmt.Sprintf("- Unconfirmed (`< %.2f%%` or error): `%d`\n", threshold, len(unconfirmed)))
	b.WriteString(fmt.Sprintf("- Error rows: `%d`\n\n", errorCount))
	b.WriteString(fmt.Sprintf("- Supported operator coverage: `%d/%d`\n", supportedCoveredCount, supportedTrackedCount))
	b.WriteString(fmt.Sprintf("- Unsupported tracked operators: `%d`\n\n", len(lexUnsupportedOperators)))

	b.WriteString("## Operator Coverage\n\n")
	for _, op := range lexTrackedOperators {
		status := "remaining"
		note := ""
		if reason, unsupported := lexUnsupportedOperators[op]; unsupported {
			status = "unsupported"
			note = fmt.Sprintf(" (%s)", reason)
		}
		if _, ok := coveredOps[op]; ok {
			status = "covered"
		}
		b.WriteString(fmt.Sprintf("- `%s`: %s%s\n", op, status, note))
	}

	b.WriteString("## Confirmed\n\n")
	if len(confirmed) == 0 {
		b.WriteString("- None\n")
	} else {
		for _, result := range confirmed {
			b.WriteString(fmt.Sprintf("- `%s`: exact `%.4f%%`, similarity `%.4f%%`\n", result.name, result.exact, result.similarity))
		}
	}

	b.WriteString("\n## Unconfirmed\n\n")
	if len(unconfirmed) == 0 {
		b.WriteString("- None\n")
	} else {
		for _, result := range unconfirmed {
			if result.err != "" {
				b.WriteString(fmt.Sprintf("- `%s`: error `%s`\n", result.name, result.err))
				continue
			}
			b.WriteString(fmt.Sprintf("- `%s`: exact `%.4f%%`, similarity `%.4f%%`\n", result.name, result.exact, result.similarity))
		}
	}

	b.WriteString(fmt.Sprintf("\n- Report CSV: `%s`\n", filepath.Join(filepath.Dir(path), "report.csv")))
	b.WriteString(fmt.Sprintf("- HTML report: `%s`\n", filepath.Join(filepath.Dir(path), "index.html")))

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func TestLexWriteSummaryMarksUnsupportedOperatorsSeparately(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "summary.md")
	rows := []parityRow{
		{
			PDFPath:           "050_text_leading.pdf",
			ExactPercent:      99.8,
			SimilarityPercent: 99.9,
			Pass:              true,
		},
		{
			PDFPath:           "054_inline_image_bi.pdf",
			ExactPercent:      99.1,
			SimilarityPercent: 99.4,
			Pass:              true,
		},
		{
			PDFPath:           "999_unknown.pdf",
			ExactPercent:      90.0,
			SimilarityPercent: 98.0,
			Pass:              false,
		},
	}

	require.NoError(t, lexWriteSummary(outputPath, "/tmp/lex_tests", 99.0, rows))

	summary, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	text := string(summary)
	require.Contains(t, text, "- Supported operator coverage: `13/56`")
	require.Contains(t, text, "- Unsupported tracked operators: `2`")
	require.Contains(t, text, "- `d0`: unsupported (Type3 glyph metrics operator is not implemented and is currently treated as a no-op)")
	require.Contains(t, text, "- `d1`: unsupported (Type3 glyph metrics operator is not implemented and is currently treated as a no-op)")
	require.Contains(t, text, "- `999_unknown.pdf`: exact `90.0000%`, similarity `98.0000%`")
	require.NotContains(t, text, "- `w`:")
}
