//go:build ignore

package main

import (
	"encoding/csv"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type targetDoc struct {
	Name          string
	PDFPath       string
	ContentObject string
	ImageObject   string
}

func main() {
	repoRoot := "/workspace/pdf-reader/go-pdf"
	outDir := filepath.Join(repoRoot, "test", "testdata", "output", "render_parity_diagnostics")
	must(os.RemoveAll(outDir))
	must(os.MkdirAll(outDir, 0o755))

	reportRows := loadReport(filepath.Join(repoRoot, "test", "testdata", "output", "render_parity", "report.csv"))
	baseFixtureRoot := filepath.Join(repoRoot, "test", "integration", "pdf", "testdata")
	docs := []targetDoc{
		{
			Name:          "003-pdflatex-image page1",
			PDFPath:       filepath.Join(baseFixtureRoot, "003-pdflatex-image", "pdflatex-image.pdf"),
			ContentObject: "4",
			ImageObject:   "1",
		},
		{
			Name:          "007-imagemagick-images page1",
			PDFPath:       filepath.Join(baseFixtureRoot, "007-imagemagick-images", "imagemagick-images.pdf"),
			ContentObject: "4",
			ImageObject:   "8",
		},
	}

	var body strings.Builder
	body.WriteString("<!doctype html><html><head><meta charset=\"utf-8\">")
	body.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	body.WriteString("<title>Render Parity Diagnostics</title>")
	body.WriteString("<style>")
	body.WriteString("body{font-family:system-ui,sans-serif;margin:24px;background:#f5f5f5;color:#111;}h1,h2,h3{margin:0 0 12px;}section{margin:0 0 28px;padding:20px;background:#fff;border:1px solid #ddd;}pre{white-space:pre-wrap;word-break:break-word;background:#fafafa;border:1px solid #eee;padding:12px;}table{border-collapse:collapse;width:100%;margin:12px 0;}th,td{border:1px solid #ddd;padding:8px;vertical-align:top;}img{max-width:320px;height:auto;border:1px solid #ccc;background:#fff;}code{font-family:ui-monospace,monospace;}")
	body.WriteString("</style></head><body>")
	body.WriteString("<h1>Render Parity Diagnostics</h1>")
	body.WriteString("<p>Current poppler/ours outputs are shown together with the raw PDF content stream and image object dictionary used in rendering.</p>")

	for _, doc := range docs {
		relPDF, _ := filepath.Rel(baseFixtureRoot, doc.PDFPath)
		row := reportRows[relPDF+"#1"]
		content := extractObject(doc.PDFPath, doc.ContentObject)
		imageDict := extractObject(doc.PDFPath, doc.ImageObject)
		popplerRel := rel(filepath.Join(outDir, "index.html"), row["poppler_png"])
		oursRel := rel(filepath.Join(outDir, "index.html"), row["ours_png"])

		body.WriteString("<section>")
		body.WriteString("<h2>" + html.EscapeString(doc.Name) + "</h2>")
		body.WriteString("<table><tr><th>Metric</th><th>Value</th></tr>")
		body.WriteString("<tr><td>PDF</td><td><code>" + html.EscapeString(relPDF) + "</code></td></tr>")
		body.WriteString("<tr><td>Similarity</td><td>" + html.EscapeString(row["similarity_percent"]) + "%</td></tr>")
		body.WriteString("<tr><td>Exact</td><td>" + html.EscapeString(row["exact_percent"]) + "%</td></tr>")
		body.WriteString("<tr><td>Error</td><td><code>" + html.EscapeString(row["error"]) + "</code></td></tr>")
		body.WriteString("</table>")
		body.WriteString("<table><tr><th>Poppler</th><th>Ours</th></tr>")
		body.WriteString("<tr><td><img src=\"" + html.EscapeString(popplerRel) + "\"></td><td><img src=\"" + html.EscapeString(oursRel) + "\"></td></tr></table>")
		body.WriteString("<h3>Content Stream</h3><pre>" + html.EscapeString(content) + "</pre>")
		body.WriteString("<h3>Image Object</h3><pre>" + html.EscapeString(imageDict) + "</pre>")
		body.WriteString("</section>")
	}

	body.WriteString("</body></html>")
	must(os.WriteFile(filepath.Join(outDir, "index.html"), []byte(body.String()), 0o644))
}

func loadReport(path string) map[string]map[string]string {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	must(err)
	rows := map[string]map[string]string{}
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		row := map[string]string{}
		for i, col := range header {
			row[col] = rec[i]
		}
		rows[row["pdf"]+"#"+row["page"]] = row
	}
	return rows
}

func extractObject(path, objNum string) string {
	data, err := os.ReadFile(path)
	must(err)
	pattern := regexp.MustCompile("(?s)" + regexp.QuoteMeta(objNum) + ` 0 obj\s*(.*?)\s*endobj`)
	m := pattern.FindSubmatch(data)
	if m == nil {
		return "object not found"
	}
	return string(m[1])
}

func rel(base, target string) string {
	if target == "" {
		return ""
	}
	r, err := filepath.Rel(filepath.Dir(base), target)
	must(err)
	return filepath.ToSlash(r)
}

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("diagnostic generation failed: %v", err))
	}
}
