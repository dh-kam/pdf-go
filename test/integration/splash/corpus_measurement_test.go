// Phase 3 SP4 exit-gate measurement.
//
// TestSplashCorpusMeasurement renders the full 286-page corpus once under
// --backend=image-canvas (legacy baseline) and once under --backend=splash
// (Phase 3 experiment) and computes exact100 / similarity / regression
// deltas. The Phase 3 SP4 exit gate requires splash_exact100 >= 130.
//
// This test is HEAVY (~30-60 minutes) and is gated behind SPLASH_CORPUS_RUN=1
// in addition to skipping in -short mode and when the binary or corpus
// directory is absent. Routine CI runs MUST skip it.
//
// Stdlib-only by design (per file-ownership contract for QA2).
package splashintegration

import (
	"encoding/csv"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// corpusBackend is one of the two backends measured in this test.
type corpusBackend string

const (
	corpusBackendImageCanvas corpusBackend = "image-canvas"
	corpusBackendSplash      corpusBackend = "splash"

	// phase3ExitGate is the SP4 exit-gate floor: splash exact100 must be >= this.
	phase3ExitGate = 130

	// corpusDPI matches what TestRenderParityReportAgainstPoppler uses by default.
	corpusDPI = 150
)

// corpusRow is one (pdf, page) measurement under a single backend.
type corpusRow struct {
	PDFRelPath        string
	Page              int
	ExactPercent      float64
	SimilarityPercent float64
	Error             string
}

// corpusKey identifies a (pdf, page) pair across the two backends.
type corpusKey struct {
	PDF  string
	Page int
}

// corpusWatchEntry lists the 8 pages that must not regress at the Phase 3 gate.
// These are the same 8 pages tracked by watchset_test.go and
// watchset_splash_probe_test.go.
type corpusWatchEntry struct {
	Label string // human-readable label, e.g. "GeoTopo p23"
	PDF   string // basename matched against corpusRow.PDFRelPath suffix
	Page  int
}

// corpusWatchSet is the canonical 8-page watch list.
//
// PDF basenames here match the relative-path basename produced by
// parityFindPDFs (e.g. "GeoTopo.pdf").
var corpusWatchSet = []corpusWatchEntry{
	{"GeoTopo p23", "GeoTopo.pdf", 23},
	{"GeoTopo p35", "GeoTopo.pdf", 35},
	{"GeoTopo p44", "GeoTopo.pdf", 44},
	{"GeoTopo p55", "GeoTopo.pdf", 55},
	{"GeoTopo p96", "GeoTopo.pdf", 96},
	{"GeoTopo p97", "GeoTopo.pdf", 97},
	{"011-google-doc-document p1", "google-doc-document.pdf", 1},
	{"012-libreoffice-form p1", "libreoffice-form.pdf", 1},
}

// TestSplashCorpusMeasurement is the Phase 3 SP4 exit-gate measurement.
//
// SKIP conditions:
//   - testing.Short()
//   - SPLASH_CORPUS_RUN env var unset (avoid surprising heavy runs)
//   - bin/pdfrender absent
//   - corpus dir test/testdata/sample-files absent
//   - pdftoppm not on PATH
//
// On success the test writes:
//   - tmp/splash_corpus_p3/baseline.csv (image-canvas measurements)
//   - tmp/splash_corpus_p3/splash.csv   (splash measurements)
//   - tmp/splash_corpus_p3/summary.md   (human-readable verdict)
//
// On gate failure (splash exact100 < 130 OR any watch-set regression) the
// test calls t.Errorf so CI surfaces the failure.
func TestSplashCorpusMeasurement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping corpus measurement in -short mode")
	}
	if os.Getenv("SPLASH_CORPUS_RUN") != "1" {
		t.Skip("Set SPLASH_CORPUS_RUN=1 to run (heavy: ~30-60 min)")
	}

	root := parityRepoRoot(t)

	bin := filepath.Join(root, "bin", "pdfrender")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("bin/pdfrender absent: %v (run `make build-pdfrender`)", err)
	}
	if !pdfrenderHasBackendFlag(t, root) {
		t.Skip("pdfrender lacks --backend flag; rebuild after backend wiring")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed (poppler-utils)")
	}

	scanRoot := filepath.Join(root, "test", "testdata", "sample-files")
	if _, err := os.Stat(scanRoot); err != nil {
		t.Skipf("corpus dir absent: %s (%v)", scanRoot, err)
	}

	pdfFiles, err := corpusFindPDFs(scanRoot)
	if err != nil {
		t.Fatalf("scan corpus: %v", err)
	}
	if len(pdfFiles) == 0 {
		t.Skipf("no PDFs under %s", scanRoot)
	}
	t.Logf("Phase 3 corpus measurement: %d source PDFs under %s", len(pdfFiles), scanRoot)

	outRoot := filepath.Join(root, "tmp", "splash_corpus_p3")
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outRoot, err)
	}

	// Render the corpus twice — once per backend — and write a CSV per backend.
	baselineRows := corpusRunBackend(t, root, bin, scanRoot, pdfFiles, corpusBackendImageCanvas, outRoot)
	if err := corpusWriteCSV(filepath.Join(outRoot, "baseline.csv"), baselineRows); err != nil {
		t.Fatalf("write baseline.csv: %v", err)
	}
	t.Logf("baseline (image-canvas) rows: %d", len(baselineRows))

	splashRows := corpusRunBackend(t, root, bin, scanRoot, pdfFiles, corpusBackendSplash, outRoot)
	if err := corpusWriteCSV(filepath.Join(outRoot, "splash.csv"), splashRows); err != nil {
		t.Fatalf("write splash.csv: %v", err)
	}
	t.Logf("splash rows: %d", len(splashRows))

	// Compute deltas. Index by (pdf, page) so we can join the two backends.
	baselineByKey := corpusIndexByKey(baselineRows)
	splashByKey := corpusIndexByKey(splashRows)

	baselineExact100 := corpusCountExact100(baselineRows)
	splashExact100 := corpusCountExact100(splashRows)

	// Per-page similarity delta (splash - baseline).
	deltas := make([]corpusPageDelta, 0, len(baselineByKey))
	regressionCount := 0
	for k, b := range baselineByKey {
		s, ok := splashByKey[k]
		if !ok {
			continue
		}
		d := corpusPageDelta{
			Key:       k,
			Baseline:  b.SimilarityPercent,
			Splash:    s.SimilarityPercent,
			Delta:     s.SimilarityPercent - b.SimilarityPercent,
			HasResult: b.Error == "" && s.Error == "",
		}
		deltas = append(deltas, d)
		// "Regression" = splash strictly worse than baseline (any non-zero
		// negative delta). Zero is treated as parity.
		if d.HasResult && d.Delta < 0 {
			regressionCount++
		}
	}

	// Top 10 gainers (largest +delta) and top 10 losses (largest -delta).
	sort.SliceStable(deltas, func(i, j int) bool { return deltas[i].Delta > deltas[j].Delta })
	gainers := corpusFirstN(deltas, 10)
	sort.SliceStable(deltas, func(i, j int) bool { return deltas[i].Delta < deltas[j].Delta })
	losses := corpusFirstN(deltas, 10)

	// Watch-set check.
	watchRows := make([]watchEntry, 0, len(corpusWatchSet))
	watchSetRegressed := false
	for _, w := range corpusWatchSet {
		row := watchEntry{Label: w.Label}
		// Match by basename suffix.
		var matched *corpusKey
		for k := range baselineByKey {
			if filepath.Base(k.PDF) == w.PDF && k.Page == w.Page {
				kk := k
				matched = &kk
				break
			}
		}
		if matched != nil {
			b := baselineByKey[*matched]
			s, ok := splashByKey[*matched]
			if ok {
				row.Found = true
				row.Baseline = b.SimilarityPercent
				row.Splash = s.SimilarityPercent
				row.Delta = s.SimilarityPercent - b.SimilarityPercent
				if row.Delta < 0 {
					watchSetRegressed = true
				}
			}
		}
		watchRows = append(watchRows, row)
	}

	verdict := "PASS"
	if splashExact100 < phase3ExitGate || watchSetRegressed {
		verdict = "FAIL"
	}

	// Write summary.md.
	summaryPath := filepath.Join(outRoot, "summary.md")
	if err := corpusWriteSummary(summaryPath, summaryInputs{
		BaselineExact100: baselineExact100,
		SplashExact100:   splashExact100,
		BaselineRows:     len(baselineRows),
		SplashRows:       len(splashRows),
		Regressions:      regressionCount,
		Gainers:          corpusToSummaryEntries(gainers),
		Losses:           corpusToSummaryEntries(losses),
		WatchRows:        watchRows,
		Verdict:          verdict,
	}); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
	t.Logf("Phase 3 summary written: %s", summaryPath)
	t.Logf("baseline_exact100=%d splash_exact100=%d delta=%+d regressions=%d verdict=%s",
		baselineExact100, splashExact100, splashExact100-baselineExact100, regressionCount, verdict)

	// Hard-fail if Phase 3 gate not met.
	if splashExact100 < phase3ExitGate {
		t.Errorf("Phase 3 SP4 gate FAILED: splash_exact100=%d < %d (target). See %s",
			splashExact100, phase3ExitGate, summaryPath)
	}
	if watchSetRegressed {
		t.Errorf("Phase 3 watch-set regression: at least one of the 8 watch pages dropped. See %s",
			summaryPath)
	}
}

// summaryInputs collects the data needed to render summary.md.
type summaryInputs struct {
	BaselineExact100 int
	SplashExact100   int
	BaselineRows     int
	SplashRows       int
	Regressions      int
	Gainers          []summaryEntry
	Losses           []summaryEntry
	WatchRows        []watchEntry
	Verdict          string
}

type summaryEntry struct {
	PDF      string
	Page     int
	Baseline float64
	Splash   float64
	Delta    float64
}

type watchEntry struct {
	Label    string
	Found    bool
	Baseline float64
	Splash   float64
	Delta    float64
}

// corpusPageDelta is a (pdf, page) similarity delta between the two backends.
type corpusPageDelta struct {
	Key       corpusKey
	Baseline  float64
	Splash    float64
	Delta     float64
	HasResult bool
}

func corpusToSummaryEntries(in []corpusPageDelta) []summaryEntry {
	out := make([]summaryEntry, 0, len(in))
	for _, d := range in {
		out = append(out, summaryEntry{
			PDF:      d.Key.PDF,
			Page:     d.Key.Page,
			Baseline: d.Baseline,
			Splash:   d.Splash,
			Delta:    d.Delta,
		})
	}
	return out
}

// corpusFirstN copies the first N elements of deltas (or fewer if len < N).
func corpusFirstN(deltas []corpusPageDelta, n int) []corpusPageDelta {
	if len(deltas) < n {
		n = len(deltas)
	}
	out := make([]corpusPageDelta, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, deltas[i])
	}
	return out
}

// corpusFindPDFs walks scanRoot and returns all *.pdf paths sorted.
func corpusFindPDFs(scanRoot string) ([]string, error) {
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

// corpusRunBackend renders every page of every PDF under the given backend,
// running pdftoppm once per PDF for the reference, and returns one corpusRow
// per (pdf, page).
func corpusRunBackend(
	t *testing.T,
	root, bin, scanRoot string,
	pdfFiles []string,
	backend corpusBackend,
	outRoot string,
) []corpusRow {
	t.Helper()
	rows := make([]corpusRow, 0, 320)
	backendRunDir := filepath.Join(outRoot, "render", string(backend))
	if err := os.RemoveAll(backendRunDir); err != nil {
		t.Fatalf("clean %s: %v", backendRunDir, err)
	}
	if err := os.MkdirAll(backendRunDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", backendRunDir, err)
	}

	for _, pdfPath := range pdfFiles {
		relPath, err := filepath.Rel(scanRoot, pdfPath)
		if err != nil {
			rows = append(rows, corpusRow{
				PDFRelPath: pdfPath,
				Error:      fmt.Sprintf("rel: %v", err),
			})
			continue
		}
		password := corpusPasswordFor(relPath)
		docID := corpusSanitizeDocID(relPath)

		// Reference (poppler) PNGs go alongside the rendered output for this
		// (pdf, backend) pair so we can clean up the whole tree easily.
		docDir := filepath.Join(backendRunDir, docID)
		popplerDir := filepath.Join(docDir, "poppler")
		oursDir := filepath.Join(docDir, "ours")
		if err := os.MkdirAll(popplerDir, 0o755); err != nil {
			rows = append(rows, corpusRow{PDFRelPath: relPath, Error: fmt.Sprintf("mkdir poppler: %v", err)})
			continue
		}
		if err := os.MkdirAll(oursDir, 0o755); err != nil {
			rows = append(rows, corpusRow{PDFRelPath: relPath, Error: fmt.Sprintf("mkdir ours: %v", err)})
			continue
		}

		// pdftoppm reference (per PDF, all pages).
		if err := corpusRunPoppler(pdfPath, popplerDir, corpusDPI, password); err != nil {
			rows = append(rows, corpusRow{
				PDFRelPath: relPath,
				Error:      fmt.Sprintf("pdftoppm: %v", err),
			})
			continue
		}
		popplerPages, err := corpusListPopplerPages(popplerDir)
		if err != nil {
			rows = append(rows, corpusRow{PDFRelPath: relPath, Error: fmt.Sprintf("list poppler: %v", err)})
			continue
		}
		if len(popplerPages) == 0 {
			rows = append(rows, corpusRow{PDFRelPath: relPath, Error: "no poppler pages"})
			continue
		}

		// pdfrender (under the chosen backend, all pages).
		args := []string{
			"--backend=" + string(backend),
			"--output", oursDir,
			"--dpi", strconv.Itoa(corpusDPI),
			"--quiet",
		}
		if password != "" {
			args = append(args, "--password", password)
		}
		args = append(args, pdfPath)
		cmd := exec.Command(bin, args...)
		if cmdOut, runErr := cmd.CombinedOutput(); runErr != nil {
			rows = append(rows, corpusRow{
				PDFRelPath: relPath,
				Error:      fmt.Sprintf("pdfrender %s: %v: %s", backend, runErr, sanitizeOneLine(string(cmdOut))),
			})
			continue
		}

		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))

		// Compare each poppler page against ours.
		pages := make([]int, 0, len(popplerPages))
		for p := range popplerPages {
			pages = append(pages, p)
		}
		sort.Ints(pages)
		for _, pageNumber := range pages {
			oursPNG := filepath.Join(oursDir, fmt.Sprintf("%s_page_%04d.png", baseName, pageNumber))
			row := corpusRow{PDFRelPath: relPath, Page: pageNumber}
			if _, err := os.Stat(oursPNG); err != nil {
				row.Error = fmt.Sprintf("ours missing: %v", err)
				rows = append(rows, row)
				continue
			}
			exact, sim, err := corpusComparePNGs(oursPNG, popplerPages[pageNumber])
			if err != nil {
				row.Error = fmt.Sprintf("compare: %v", err)
				rows = append(rows, row)
				continue
			}
			row.ExactPercent = exact
			row.SimilarityPercent = sim
			rows = append(rows, row)
		}
	}
	return rows
}

// corpusIndexByKey collapses rows into a (pdf, page) -> row map. Multiple
// rows for the same key (shouldn't happen in practice) are last-wins.
func corpusIndexByKey(rows []corpusRow) map[corpusKey]corpusRow {
	out := make(map[corpusKey]corpusRow, len(rows))
	for _, r := range rows {
		if r.Page <= 0 {
			continue
		}
		out[corpusKey{PDF: r.PDFRelPath, Page: r.Page}] = r
	}
	return out
}

// corpusCountExact100 counts rows where ExactPercent == 100.0 (and Error == "").
func corpusCountExact100(rows []corpusRow) int {
	n := 0
	for _, r := range rows {
		if r.Error != "" || r.Page <= 0 {
			continue
		}
		if r.ExactPercent >= 100.0 {
			n++
		}
	}
	return n
}

// corpusRunPoppler invokes pdftoppm to produce reference PNGs.
func corpusRunPoppler(pdfPath, outDir string, dpi int, password string) error {
	args := []string{"-png", "-r", strconv.Itoa(dpi)}
	if password != "" {
		args = append(args, "-upw", password)
	}
	args = append(args, pdfPath, filepath.Join(outDir, "rendered"))
	cmd := exec.Command("pdftoppm", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// corpusListPopplerPages enumerates rendered-NN.png files from pdftoppm's
// output and returns a map of page-number -> absolute path.
func corpusListPopplerPages(popplerRoot string) (map[int]string, error) {
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

// corpusComparePNGs returns (exactPercent, similarityPercent) — the same
// metrics as TestRenderParityReportAgainstPoppler.
func corpusComparePNGs(oursPath, popplerPath string) (float64, float64, error) {
	ours, err := corpusLoadPNG(oursPath)
	if err != nil {
		return 0, 0, fmt.Errorf("load ours: %w", err)
	}
	poppler, err := corpusLoadPNG(popplerPath)
	if err != nil {
		return 0, 0, fmt.Errorf("load poppler: %w", err)
	}
	if !ours.Bounds().Eq(poppler.Bounds()) {
		return 0, 0, fmt.Errorf("dimension mismatch: ours=%v poppler=%v", ours.Bounds(), poppler.Bounds())
	}
	bounds := ours.Bounds()
	totalPixels := bounds.Dx() * bounds.Dy()
	if totalPixels == 0 {
		return 0, 0, fmt.Errorf("empty image")
	}
	matching := 0
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
				matching++
			}
			totalDiff += math.Abs(ourR-popR) + math.Abs(ourG-popG) + math.Abs(ourB-popB)
		}
	}
	exact := float64(matching) * 100.0 / float64(totalPixels)
	sim := 100.0 * (1.0 - totalDiff/(float64(totalPixels)*255.0*3.0))
	if sim < 0 {
		sim = 0
	}
	return exact, sim, nil
}

func corpusLoadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// corpusPasswordFor mirrors parityPasswordForRelPath in the pdf integration
// package — only the libreoffice-writer-password fixture needs a password.
func corpusPasswordFor(relPath string) string {
	normalized := filepath.ToSlash(relPath)
	if strings.HasSuffix(normalized, "005-libreoffice-writer-password/libreoffice-writer-password.pdf") {
		return "openpassword"
	}
	return ""
}

// corpusSanitizeDocID makes a relPath safe for use as a directory name.
func corpusSanitizeDocID(relPath string) string {
	r := strings.NewReplacer("/", "__", "\\", "__", ":", "_", " ", "_")
	return r.Replace(relPath)
}

// corpusWriteCSV writes one row per measurement.
func corpusWriteCSV(path string, rows []corpusRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"pdf", "page", "exact_percent", "similarity_percent", "error"}); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			r.PDFRelPath,
			strconv.Itoa(r.Page),
			fmt.Sprintf("%.9f", r.ExactPercent),
			fmt.Sprintf("%.9f", r.SimilarityPercent),
			r.Error,
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return w.Error()
}

// corpusWriteSummary renders the human-readable Phase 3 verdict markdown.
func corpusWriteSummary(path string, in summaryInputs) error {
	var b strings.Builder
	delta := in.SplashExact100 - in.BaselineExact100
	fmt.Fprintf(&b,
		"# Phase 3 Corpus Measurement: exact100 baseline=%d splash=%d delta=%+d\n\n",
		in.BaselineExact100, in.SplashExact100, delta)
	fmt.Fprintf(&b, "- Rows (baseline / splash): %d / %d\n", in.BaselineRows, in.SplashRows)
	fmt.Fprintf(&b, "- Regressions (splash similarity < baseline): %d\n", in.Regressions)
	fmt.Fprintf(&b, "- Phase 3 SP4 gate: splash_exact100 >= %d\n\n", phase3ExitGate)

	fmt.Fprintf(&b, "## Top 10 gainers (splash − baseline similarity, descending)\n\n")
	fmt.Fprintf(&b, "| pdf | page | baseline%% | splash%% | delta |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|\n")
	for _, e := range in.Gainers {
		fmt.Fprintf(&b, "| `%s` | %d | %.4f | %.4f | %+.4f |\n",
			e.PDF, e.Page, e.Baseline, e.Splash, e.Delta)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Top 10 regressions (splash − baseline similarity, ascending)\n\n")
	fmt.Fprintf(&b, "| pdf | page | baseline%% | splash%% | delta |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|\n")
	for _, e := range in.Losses {
		fmt.Fprintf(&b, "| `%s` | %d | %.4f | %.4f | %+.4f |\n",
			e.PDF, e.Page, e.Baseline, e.Splash, e.Delta)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Watch-set (must not regress)\n\n")
	fmt.Fprintf(&b, "| watch page | baseline%% | splash%% | delta | found |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---|\n")
	for _, w := range in.WatchRows {
		if !w.Found {
			fmt.Fprintf(&b, "| %s | - | - | - | no |\n", w.Label)
			continue
		}
		fmt.Fprintf(&b, "| %s | %.4f | %.4f | %+.4f | yes |\n",
			w.Label, w.Baseline, w.Splash, w.Delta)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Phase 3 SP4 gate verdict: **%s**\n", in.Verdict)
	if in.Verdict != "PASS" {
		fmt.Fprintf(&b, "\n_FAIL conditions: splash_exact100 < %d OR any watch-set delta < 0._\n",
			phase3ExitGate)
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
