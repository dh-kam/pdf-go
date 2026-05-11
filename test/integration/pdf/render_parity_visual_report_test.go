package pdf_test

import (
	"encoding/csv"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRenderParityVisualReport generates an enhanced visual HTML report from
// the CSV produced by TestRenderParityReportAgainstPoppler. Run after that test.
func TestRenderParityVisualReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping visual report in short mode")
	}

	repoRoot := parityRepoRoot(t)
	outputRoot := parityEnvOrDefault("PDF_PARITY_OUTPUT_ROOT",
		filepath.Join(repoRoot, "test", "testdata", "output", "render_parity"))
	csvPath := filepath.Join(outputRoot, "report.csv")

	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		t.Skipf("report.csv not found at %s — run TestRenderParityReportAgainstPoppler first", csvPath)
	}

	rows, err := visualLoadCSV(csvPath)
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	diffRoot := filepath.Join(outputRoot, "docs")
	for i := range rows {
		if rows[i].PopplerPNG == "" || rows[i].OursPNG == "" {
			continue
		}
		docID := visualDocID(rows[i].PDFPath)
		diffDir := filepath.Join(diffRoot, docID, "diff")
		require.NoError(t, os.MkdirAll(diffDir, 0o755))
		diffPNG := filepath.Join(diffDir, fmt.Sprintf("diff-%04d.png", rows[i].Page))
		if err := visualGenerateDiffPNG(rows[i].PopplerPNG, rows[i].OursPNG, diffPNG); err != nil {
			t.Logf("diff gen failed for %s p%d: %v", rows[i].PDFPath, rows[i].Page, err)
		} else {
			rows[i].DiffPNG = diffPNG
		}
	}

	htmlPath := filepath.Join(outputRoot, "visual_report.html")
	require.NoError(t, visualWriteHTML(htmlPath, rows))
	t.Logf("Visual report: %s", htmlPath)
}

// ---- data types ----

type visualRow struct {
	PDFPath           string
	Page              int
	PopplerPNG        string
	OursPNG           string
	DiffPNG           string
	ExactPercent      float64
	SimilarityPercent float64
	Pass              bool
	Error             string
}

// ---- CSV loader ----

func visualLoadCSV(path string) ([]visualRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("empty CSV")
	}

	var rows []visualRow
	for _, rec := range records[1:] { // skip header
		if len(rec) < 7 {
			continue
		}
		page, _ := strconv.Atoi(rec[1])
		exact, _ := strconv.ParseFloat(rec[4], 64)
		sim, _ := strconv.ParseFloat(rec[5], 64)
		pass := rec[6] == "true"
		errStr := ""
		if len(rec) >= 8 {
			errStr = rec[7]
		}
		rows = append(rows, visualRow{
			PDFPath:           rec[0],
			Page:              page,
			PopplerPNG:        rec[2],
			OursPNG:           rec[3],
			ExactPercent:      exact,
			SimilarityPercent: sim,
			Pass:              pass,
			Error:             errStr,
		})
	}
	return rows, nil
}

func visualDocID(relPath string) string {
	return strings.ReplaceAll(relPath, "/", "__")
}

// ---- diff image generation ----

func visualGenerateDiffPNG(popplerPath, oursPath, diffPath string) error {
	popImg, err := parityLoadPNG(popplerPath)
	if err != nil {
		return fmt.Errorf("load poppler: %w", err)
	}
	ourImg, err := parityLoadPNG(oursPath)
	if err != nil {
		return fmt.Errorf("load ours: %w", err)
	}

	popBounds := popImg.Bounds()
	ourBounds := ourImg.Bounds()
	if popBounds != ourBounds {
		return fmt.Errorf("size mismatch: poppler=%v ours=%v", popBounds, ourBounds)
	}

	diffImg := visualComputeDiff(popImg, ourImg, popBounds)

	f, err := os.Create(diffPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, diffImg)
}

func visualComputeDiff(popImg, ourImg image.Image, bounds image.Rectangle) *image.RGBA {
	w := bounds.Dx()
	h := bounds.Dy()
	diff := image.NewRGBA(image.Rect(0, 0, w, h))
	diffMask := make([]bool, w*h)

	// Fill diff image with amplified heatmap.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pr, pg, pb, _ := popImg.At(x, y).RGBA()
			or, og, ob, _ := ourImg.At(x, y).RGBA()

			dr := visualAbsDiff(pr, or) >> 8
			dg := visualAbsDiff(pg, og) >> 8
			db := visualAbsDiff(pb, ob) >> 8
			maxD := dr
			if dg > maxD {
				maxD = dg
			}
			if db > maxD {
				maxD = db
			}

			dx := x - bounds.Min.X
			dy := y - bounds.Min.Y

			if maxD == 0 {
				diff.SetRGBA(dx, dy, color.RGBA{R: 230, G: 230, B: 230, A: 255})
			} else {
				diffMask[dy*w+dx] = true
				amp := maxD * 6
				if amp > 255 {
					amp = 255
				}
				// Heatmap: yellow → orange → red
				var rc, gc, bc uint8
				if amp < 100 {
					rc = 255
					gc = uint8(255 - amp*2)
					bc = 0
				} else {
					rc = 255
					gc = 0
					bc = 0
				}
				diff.SetRGBA(dx, dy, color.RGBA{R: rc, G: gc, B: bc, A: 255})
			}
		}
	}

	// Find diff clusters and draw red rectangles.
	bboxes := visualFindClusters(diffMask, w, h, 16, 12)
	for _, bb := range bboxes {
		visualDrawRect(diff, bb, 3, color.RGBA{R: 220, G: 0, B: 0, A: 255})
	}

	return diff
}

func visualAbsDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// visualFindClusters finds bounding boxes of clusters of true pixels in mask.
// gridSize: cell size for coarse clustering; pad: padding added to each bbox.
func visualFindClusters(mask []bool, w, h, gridSize, pad int) []image.Rectangle {
	if w <= 0 || h <= 0 {
		return nil
	}
	gw := (w + gridSize - 1) / gridSize
	gh := (h + gridSize - 1) / gridSize
	grid := make([]bool, gw*gh)

	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			if mask[dy*w+dx] {
				gx := dx / gridSize
				gy := dy / gridSize
				grid[gy*gw+gx] = true
			}
		}
	}

	// Flood-fill on grid to find connected components.
	label := make([]int, gw*gh)
	numLabels := 0
	for i := range label {
		label[i] = -1
	}

	var flood func(gx, gy, lbl int)
	flood = func(gx, gy, lbl int) {
		if gx < 0 || gx >= gw || gy < 0 || gy >= gh {
			return
		}
		idx := gy*gw + gx
		if !grid[idx] || label[idx] >= 0 {
			return
		}
		label[idx] = lbl
		flood(gx+1, gy, lbl)
		flood(gx-1, gy, lbl)
		flood(gx, gy+1, lbl)
		flood(gx, gy-1, lbl)
	}

	for gy := 0; gy < gh; gy++ {
		for gx := 0; gx < gw; gx++ {
			if grid[gy*gw+gx] && label[gy*gw+gx] < 0 {
				flood(gx, gy, numLabels)
				numLabels++
			}
		}
	}

	if numLabels == 0 {
		return nil
	}

	// Compute pixel bounding box per component.
	minX := make([]int, numLabels)
	minY := make([]int, numLabels)
	maxX := make([]int, numLabels)
	maxY := make([]int, numLabels)
	for i := range minX {
		minX[i] = math.MaxInt32
		minY[i] = math.MaxInt32
		maxX[i] = math.MinInt32
		maxY[i] = math.MinInt32
	}

	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			if !mask[dy*w+dx] {
				continue
			}
			gx := dx / gridSize
			gy := dy / gridSize
			lbl := label[gy*gw+gx]
			if lbl < 0 {
				continue
			}
			if dx < minX[lbl] {
				minX[lbl] = dx
			}
			if dy < minY[lbl] {
				minY[lbl] = dy
			}
			if dx > maxX[lbl] {
				maxX[lbl] = dx
			}
			if dy > maxY[lbl] {
				maxY[lbl] = dy
			}
		}
	}

	bboxes := make([]image.Rectangle, 0, numLabels)
	for i := 0; i < numLabels; i++ {
		if maxX[i] < minX[i] {
			continue
		}
		x0 := visualClamp(minX[i]-pad, 0, w-1)
		y0 := visualClamp(minY[i]-pad, 0, h-1)
		x1 := visualClamp(maxX[i]+pad, 0, w-1)
		y1 := visualClamp(maxY[i]+pad, 0, h-1)
		bboxes = append(bboxes, image.Rect(x0, y0, x1+1, y1+1))
	}
	return bboxes
}

func visualClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func visualDrawRect(img draw.Image, r image.Rectangle, thickness int, c color.RGBA) {
	for t := 0; t < thickness; t++ {
		x0 := r.Min.X + t
		x1 := r.Max.X - 1 - t
		y0 := r.Min.Y + t
		y1 := r.Max.Y - 1 - t
		if x0 > x1 || y0 > y1 {
			break
		}
		bounds := img.Bounds()
		// Top / bottom
		for x := x0; x <= x1; x++ {
			if p := (image.Point{x, y0}); p.In(bounds) {
				img.Set(x, y0, c)
			}
			if p := (image.Point{x, y1}); p.In(bounds) {
				img.Set(x, y1, c)
			}
		}
		// Left / right
		for y := y0; y <= y1; y++ {
			if p := (image.Point{x0, y}); p.In(bounds) {
				img.Set(x0, y, c)
			}
			if p := (image.Point{x1, y}); p.In(bounds) {
				img.Set(x1, y, c)
			}
		}
	}
}

// ---- HTML writer ----

func visualWriteHTML(path string, rows []visualRow) error {
	baseDir := filepath.Dir(path)

	// Group rows by PDF.
	type pdfGroup struct {
		pdfPath string
		rows    []visualRow
	}
	var groups []pdfGroup
	groupIndex := map[string]int{}
	for _, row := range rows {
		if _, ok := groupIndex[row.PDFPath]; !ok {
			groupIndex[row.PDFPath] = len(groups)
			groups = append(groups, pdfGroup{pdfPath: row.PDFPath})
		}
		idx := groupIndex[row.PDFPath]
		groups[idx].rows = append(groups[idx].rows, row)
	}

	totalPages := 0
	passPages := 0
	for _, row := range rows {
		if row.Page > 0 {
			totalPages++
			if row.Pass {
				passPages++
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ko"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>PDF Render Parity — Visual Report</title>`)
	b.WriteString(`<style>`)
	b.WriteString(`
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#1a1a2e;color:#e0e0e0;padding:16px}
h1{font-size:1.4rem;margin-bottom:4px;color:#fff}
.subtitle{font-size:.85rem;color:#aaa;margin-bottom:20px}
.summary-bar{display:flex;gap:16px;flex-wrap:wrap;margin-bottom:20px}
.stat{background:#16213e;border-radius:8px;padding:10px 16px;font-size:.85rem}
.stat strong{display:block;font-size:1.5rem;font-weight:700;color:#4fc3f7}
.stat.pass strong{color:#66bb6a}
.stat.fail strong{color:#ef5350}

.pdf-section{margin-bottom:24px;border-radius:10px;overflow:hidden;border:1px solid #2d2d4e}
.pdf-header{background:#16213e;padding:10px 14px;display:flex;align-items:center;gap:10px;cursor:pointer;user-select:none}
.pdf-header:hover{background:#1e2d50}
.pdf-title{font-size:.9rem;font-weight:600;flex:1;word-break:break-all}
.pdf-stats{font-size:.8rem;color:#aaa;white-space:nowrap}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:.75rem;font-weight:700}
.badge.pass{background:#1b5e20;color:#a5d6a7}
.badge.fail{background:#b71c1c;color:#ffcdd2}
.badge.error{background:#e65100;color:#ffe0b2}

.page-table{width:100%;border-collapse:collapse;background:#0d0d1a}
.page-table th{background:#111128;padding:6px 10px;font-size:.75rem;color:#888;text-align:left;border-bottom:1px solid #2d2d4e;position:sticky;top:0}
.page-table td{padding:6px;border-bottom:1px solid #1a1a2e;vertical-align:top}
.page-table tr:last-child td{border-bottom:none}
.page-table tr.row-fail{background:#1a0000}
.page-table tr.row-error{background:#1a0d00}

.col-label{font-size:.75rem;color:#888;text-align:center;margin-bottom:3px}
.img-wrap{position:relative;display:inline-block}
.img-wrap img{display:block;width:100%;height:auto;border:1px solid #333;border-radius:4px}
.img-cols{display:grid;grid-template-columns:1fr 1fr 1fr;gap:6px}

.info-cell{padding:4px 8px;min-width:130px}
.info-pg{font-size:1.1rem;font-weight:700;color:#fff;margin-bottom:6px}
.info-row{display:flex;justify-content:space-between;font-size:.8rem;margin-bottom:3px;gap:8px}
.info-label{color:#888}
.info-value{font-weight:600;color:#e0e0e0}
.info-value.good{color:#66bb6a}
.info-value.warn{color:#ffa726}
.info-value.bad{color:#ef5350}
.status-badge{display:inline-block;padding:3px 10px;border-radius:12px;font-size:.8rem;font-weight:700;margin-top:6px}
.status-badge.pass{background:#1b5e20;color:#a5d6a7}
.status-badge.fail{background:#b71c1c;color:#ffcdd2}
.status-badge.error{background:#e65100;color:#ffe0b2}
.errmsg{font-size:.7rem;color:#ef9a9a;margin-top:4px;word-break:break-all}

.collapsed .page-table-wrap{display:none}
.toggle-icon{margin-left:4px;font-size:.8rem;color:#888;transition:transform .2s}
.collapsed .toggle-icon{transform:rotate(-90deg)}
`)
	b.WriteString(`</style></head><body>`)

	b.WriteString(`<h1>📄 PDF Render Parity — Visual Report</h1>`)
	b.WriteString(fmt.Sprintf(`<p class="subtitle">Threshold: 99.00%% &nbsp;|&nbsp; %d PDFs &nbsp;|&nbsp; %d pages</p>`,
		len(groups), totalPages))

	b.WriteString(`<div class="summary-bar">`)
	b.WriteString(fmt.Sprintf(`<div class="stat pass"><strong>%d</strong>PASS pages</div>`, passPages))
	failPages := totalPages - passPages
	b.WriteString(fmt.Sprintf(`<div class="stat fail"><strong>%d</strong>FAIL pages</div>`, failPages))
	b.WriteString(fmt.Sprintf(`<div class="stat"><strong>%.2f%%</strong>Pass rate</div>`,
		func() float64 {
			if totalPages == 0 {
				return 0
			}
			return float64(passPages) * 100 / float64(totalPages)
		}()))
	b.WriteString(`</div>`)

	for _, grp := range groups {
		grpPass := 0
		grpTotal := 0
		hasError := false
		hasFail := false
		for _, row := range grp.rows {
			if row.Page > 0 {
				grpTotal++
				if row.Pass {
					grpPass++
				} else if row.Error != "" {
					hasError = true
				} else {
					hasFail = true
				}
			} else if row.Error != "" {
				hasError = true
			}
		}

		sectionClass := "pdf-section"
		badgeClass := "pass"
		badgeText := "PASS"
		if hasError {
			badgeClass = "error"
			badgeText = "ERROR"
		} else if hasFail {
			badgeClass = "fail"
			badgeText = "FAIL"
		}

		b.WriteString(`<div class="` + sectionClass + `">`)
		b.WriteString(`<div class="pdf-header" onclick="this.parentElement.classList.toggle('collapsed')">`)
		b.WriteString(`<span class="badge ` + badgeClass + `">` + badgeText + `</span>`)
		b.WriteString(`<span class="pdf-title">` + html.EscapeString(grp.pdfPath) + `</span>`)
		b.WriteString(`<span class="pdf-stats">` +
			fmt.Sprintf("%d/%d pages", grpPass, grpTotal) +
			`</span>`)
		b.WriteString(`<span class="toggle-icon">▼</span>`)
		b.WriteString(`</div>`)

		b.WriteString(`<div class="page-table-wrap">`)
		b.WriteString(`<table class="page-table"><thead><tr>`)
		b.WriteString(`<th style="width:32%">Poppler</th><th style="width:32%">Ours</th><th style="width:32%">Diff (amplified)</th><th>Info</th>`)
		b.WriteString(`</tr></thead><tbody>`)

		for _, row := range grp.rows {
			rowClass := ""
			if row.Error != "" {
				rowClass = " row-error"
			} else if !row.Pass && row.Page > 0 {
				rowClass = " row-fail"
			}
			b.WriteString(`<tr class="` + rowClass + `">`)

			// Poppler image
			b.WriteString(`<td>`)
			if row.PopplerPNG != "" {
				b.WriteString(`<div class="col-label">Poppler</div>`)
				if rel, err := filepath.Rel(baseDir, row.PopplerPNG); err == nil {
					b.WriteString(`<img src="` + html.EscapeString(filepath.ToSlash(rel)) + `" alt="poppler" loading="lazy">`)
				}
			}
			b.WriteString(`</td>`)

			// Ours image
			b.WriteString(`<td>`)
			if row.OursPNG != "" {
				b.WriteString(`<div class="col-label">Ours</div>`)
				if rel, err := filepath.Rel(baseDir, row.OursPNG); err == nil {
					b.WriteString(`<img src="` + html.EscapeString(filepath.ToSlash(rel)) + `" alt="ours" loading="lazy">`)
				}
			}
			b.WriteString(`</td>`)

			// Diff image
			b.WriteString(`<td>`)
			if row.DiffPNG != "" {
				b.WriteString(`<div class="col-label">Diff ✕6 + regions</div>`)
				if rel, err := filepath.Rel(baseDir, row.DiffPNG); err == nil {
					b.WriteString(`<img src="` + html.EscapeString(filepath.ToSlash(rel)) + `" alt="diff" loading="lazy">`)
				}
			} else {
				b.WriteString(`<div class="col-label">Diff</div><div style="color:#555;font-size:.75rem;padding:8px">n/a</div>`)
			}
			b.WriteString(`</td>`)

			// Info
			b.WriteString(`<td><div class="info-cell">`)
			if row.Page > 0 {
				b.WriteString(fmt.Sprintf(`<div class="info-pg">Page %d</div>`, row.Page))

				simClass := "good"
				if row.SimilarityPercent < 99.0 {
					simClass = "bad"
				} else if row.SimilarityPercent < 99.5 {
					simClass = "warn"
				}
				exactClass := "good"
				if row.ExactPercent < 90.0 {
					exactClass = "bad"
				} else if row.ExactPercent < 95.0 {
					exactClass = "warn"
				}

				b.WriteString(`<div class="info-row"><span class="info-label">nearSim</span>`)
				b.WriteString(fmt.Sprintf(`<span class="info-value %s">%.4f%%</span></div>`, simClass, row.SimilarityPercent))

				b.WriteString(`<div class="info-row"><span class="info-label">Exact match</span>`)
				b.WriteString(fmt.Sprintf(`<span class="info-value %s">%.4f%%</span></div>`, exactClass, row.ExactPercent))

				statusBadge := `<span class="status-badge pass">PASS</span>`
				if row.Error != "" {
					statusBadge = `<span class="status-badge error">ERROR</span>`
				} else if !row.Pass {
					statusBadge = `<span class="status-badge fail">FAIL</span>`
				}
				b.WriteString(statusBadge)
			}
			if row.Error != "" {
				b.WriteString(`<div class="errmsg">` + html.EscapeString(row.Error) + `</div>`)
			}
			b.WriteString(`</div></td>`)
			b.WriteString(`</tr>`)
		}

		b.WriteString(`</tbody></table></div></div>`)
	}

	b.WriteString(`<script>
// Collapse sections that are all-PASS by default (keep FAILs expanded)
document.querySelectorAll('.pdf-section').forEach(function(sec){
  var hasFail = sec.querySelector('.badge.fail, .badge.error');
  if(!hasFail) sec.classList.add('collapsed');
});
</script>`)
	b.WriteString(`</body></html>`)

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
