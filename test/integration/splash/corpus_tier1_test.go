// Phase 5 SP4 PR-time corpus tier-1 gate.
//
// TestSplashCorpusTier1 renders ~30 representative pages under
// --backend=splash, compares them against pdftoppm reference PNGs, and
// fails loudly when any page falls below its pinned similarity floor in
// test/testdata/splash_baseline/tier1_floors.tsv.
//
// This is the lightweight PR-time complement to the heavy 286-page
// TestSplashCorpusMeasurement (tier-2, nightly). Tier-1 is meant to fit
// inside ~90s on a typical dev box and run on every PR push:
//
//	make splash-corpus-tier1
//
// Auto-promotion: set SPLASH_TIER1_SNAPSHOT=1 to rewrite the floors TSV
// with the current similarity values (use sparingly — drift is the whole
// reason the floor exists).
//
// Stdlib-only by design (per file-ownership contract for QA2).
package splashintegration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// tier1Page pins one (pdf, page) tuple to be measured by the tier-1 gate.
// The PDFRel field is a forward-slash relative path under
// test/testdata/sample-files/, e.g. "009-pdflatex-geotopo/GeoTopo.pdf".
// The Label is a short human-readable hint for log output.
type tier1Page struct {
	Label   string
	PDFRel  string
	Page    int
	Bucket  string // diversity bucket: "text", "image", "shading", "form", "password", "glyph", "multipage", "watch"
}

// tier1Pages is the canonical 30-page tier-1 set. The first 8 entries
// mirror corpusWatchSet (and watchSetWantPages) — they MUST stay in sync.
//
// Selection criteria:
//   - 8 watch-set pages (regression-critical)
//   - ~22 representative pages spanning the corpus diversity buckets
//     (text, image, shading, form, password, glyph, multi-page)
//   - prefer pages that render quickly (page 1 of small docs, mid GeoTopo pages)
//
// Total: 30 pages. Target wall-time: ≤90s.
var tier1Pages = []tier1Page{
	// === Watch-set 8 pages (must stay in sync with corpusWatchSet) ===
	{"GeoTopo p23", "009-pdflatex-geotopo/GeoTopo.pdf", 23, "watch"},
	{"GeoTopo p35", "009-pdflatex-geotopo/GeoTopo.pdf", 35, "watch"},
	{"GeoTopo p44", "009-pdflatex-geotopo/GeoTopo.pdf", 44, "watch"},
	{"GeoTopo p55", "009-pdflatex-geotopo/GeoTopo.pdf", 55, "watch"},
	{"GeoTopo p96", "009-pdflatex-geotopo/GeoTopo.pdf", 96, "watch"},
	{"GeoTopo p97", "009-pdflatex-geotopo/GeoTopo.pdf", 97, "watch"},
	{"google-doc p1", "011-google-doc-document/google-doc-document.pdf", 1, "watch"},
	{"libreoffice-form p1", "012-libreoffice-form/libreoffice-form.pdf", 1, "watch"},

	// === ~22 representative pages across diversity buckets ===
	// Text-heavy, simple
	{"trivial p1", "001-trivial/minimal-document.pdf", 1, "text"},
	{"libre-writer p1", "002-trivial-libre-office-writer/002-trivial-libre-office-writer.pdf", 1, "text"},
	{"latex-4pages p1", "004-pdflatex-4-pages/pdflatex-4-pages.pdf", 1, "text"},
	{"latex-4pages p3", "004-pdflatex-4-pages/pdflatex-4-pages.pdf", 3, "multipage"},
	{"latex-outline p1", "006-pdflatex-outline/pdflatex-outline.pdf", 1, "text"},

	// Images
	{"latex-image p1", "003-pdflatex-image/pdflatex-image.pdf", 1, "image"},
	{"imagemagick p1", "007-imagemagick-images/imagemagick-images.pdf", 1, "image"},
	{"reportlab-inline p1", "008-reportlab-inline-image/inline-image.pdf", 1, "image"},
	{"base64-image p1", "018-base64-image/base64image.pdf", 1, "image"},
	{"grayscale-image p1", "019-grayscale-image/grayscale-image.pdf", 1, "image"},
	{"cmyk-image p1", "023-cmyk-image/cmyk-image.pdf", 1, "image"},

	// Shading/glyph-rich GeoTopo (additional non-watch pages)
	{"GeoTopo p1", "009-pdflatex-geotopo/GeoTopo.pdf", 1, "glyph"},
	{"GeoTopo p16", "009-pdflatex-geotopo/GeoTopo.pdf", 16, "shading"},

	// Forms / annotations
	{"latex-forms p1", "010-pdflatex-forms/pdflatex-forms.pdf", 1, "form"},
	{"annotations p1", "024-annotations/annotated_pdf.pdf", 1, "form"},
	{"libre-link p1", "016-libre-office-link/libre-office-link.pdf", 1, "form"},

	// Password + special-encoding
	{"password p1", "005-libreoffice-writer-password/libreoffice-writer-password.pdf", 1, "password"},
	{"arabic p1", "015-arabic/habibi.pdf", 1, "glyph"},

	// PDF/A + structure
	{"pdfa p1", "021-pdfa/crazyones-pdfa.pdf", 1, "text"},
	{"pdfkit p1", "022-pdfkit/pdfkit.pdf", 1, "text"},

	// Multi-column / layout
	{"multicolumn p1", "026-latex-multicolumn/multicolumn.pdf", 1, "multipage"},
	{"reportlab-overlay p1", "013-reportlab-overlay/reportlab-overlay.pdf", 1, "text"},
	{"outlines p1", "014-outlines/mistitled_outlines_example.pdf", 1, "text"},
}

// tier1FloorEntry is one row from tier1_floors.tsv.
type tier1FloorEntry struct {
	PDFBasename       string
	Page              int
	SimilarityFloor   float64
	ExactPercentFloor float64
	Phase             string
}

// tier1Result is one (pdf, page) measurement carried through the tier-1 run.
type tier1Result struct {
	Label             string
	PDFBasename       string
	Page              int
	ExactPercent      float64
	SimilarityPercent float64
	Floor             float64
	HasFloor          bool
	Error             string
}

// tier1FloorsRelPath is the pinned tier-1 floors TSV.
const tier1FloorsRelPath = "test/testdata/splash_baseline/tier1_floors.tsv"

// TestSplashCorpusTier1 — Phase 5 PR-time corpus gate.
//
// SKIP conditions:
//   - testing.Short()
//   - bin/pdfrender absent (suggest `make build-pdfrender`)
//   - pdftoppm absent (suggest `apt install poppler-utils`)
//   - --backend flag missing from pdfrender help (rebuild needed)
//
// Pass criteria per page: similarity_percent >= floor (tracking
// improvement, not strict 100%).
//
// Snapshot mode: SPLASH_TIER1_SNAPSHOT=1 rewrites the floors TSV with the
// values measured this run. Use to capture/promote floors when splash
// improves; combine with SPLASH_TIER1_PHASE=PN to tag the captured phase.
func TestSplashCorpusTier1(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash corpus tier-1 in -short mode")
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

	floorsPath := filepath.Join(root, filepath.FromSlash(tier1FloorsRelPath))
	floors, floorsErr := loadTier1Floors(floorsPath)
	snapshotMode := os.Getenv("SPLASH_TIER1_SNAPSHOT") == "1"
	if floorsErr != nil && !snapshotMode {
		if os.IsNotExist(floorsErr) {
			t.Skipf("tier-1 floors absent at %s — run with SPLASH_TIER1_SNAPSHOT=1 to capture", floorsPath)
		}
		t.Fatalf("load tier-1 floors %s: %v", floorsPath, floorsErr)
	}

	outRoot := filepath.Join(root, "tmp", "splash_corpus_tier1")
	if err := os.RemoveAll(outRoot); err != nil {
		t.Fatalf("clean %s: %v", outRoot, err)
	}
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outRoot, err)
	}

	// Group pinned pages by source PDF so we render each PDF once with --pages
	// targeting only the pages we need — keeps wall-time low for big docs
	// (e.g. GeoTopo's 117 pages -> render only 8 needed).
	type pdfGroup struct {
		pdfRel string
		pages  []int
		labels map[int]string // page -> label
	}
	groupOrder := make([]string, 0, len(tier1Pages))
	groupByPDF := make(map[string]*pdfGroup)
	for _, p := range tier1Pages {
		g, ok := groupByPDF[p.PDFRel]
		if !ok {
			g = &pdfGroup{pdfRel: p.PDFRel, labels: map[int]string{}}
			groupByPDF[p.PDFRel] = g
			groupOrder = append(groupOrder, p.PDFRel)
		}
		g.pages = append(g.pages, p.Page)
		g.labels[p.Page] = p.Label
	}

	results := make([]tier1Result, 0, len(tier1Pages))

	for _, key := range groupOrder {
		g := groupByPDF[key]
		pdfPath := filepath.Join(scanRoot, filepath.FromSlash(g.pdfRel))
		if _, err := os.Stat(pdfPath); err != nil {
			for _, page := range g.pages {
				results = append(results, tier1Result{
					Label:       g.labels[page],
					PDFBasename: filepath.Base(g.pdfRel),
					Page:        page,
					Error:       fmt.Sprintf("source PDF missing: %v", err),
				})
			}
			continue
		}

		password := corpusPasswordFor(g.pdfRel)
		docID := corpusSanitizeDocID(g.pdfRel)
		docDir := filepath.Join(outRoot, docID)
		popplerDir := filepath.Join(docDir, "poppler")
		oursDir := filepath.Join(docDir, "ours")
		if err := os.MkdirAll(popplerDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", popplerDir, err)
		}
		if err := os.MkdirAll(oursDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", oursDir, err)
		}

		// pdftoppm reference — render only the targeted pages via -f/-l, one
		// invocation covering [min..max] then we filter to the exact set.
		minPage, maxPage := g.pages[0], g.pages[0]
		for _, p := range g.pages {
			if p < minPage {
				minPage = p
			}
			if p > maxPage {
				maxPage = p
			}
		}
		if err := tier1RunPoppler(pdfPath, popplerDir, corpusDPI, password, minPage, maxPage); err != nil {
			for _, page := range g.pages {
				results = append(results, tier1Result{
					Label:       g.labels[page],
					PDFBasename: filepath.Base(g.pdfRel),
					Page:        page,
					Error:       fmt.Sprintf("pdftoppm: %v", err),
				})
			}
			continue
		}

		// pdfrender splash backend — pass --pages "p1,p2,..." to skip irrelevant pages.
		uniq := uniqueSortedInts(g.pages)
		pagesArg := tier1FormatPagesArg(uniq)
		args := []string{
			"--backend=splash",
			"--output", oursDir,
			"--dpi", strconv.Itoa(corpusDPI),
			"--pages", pagesArg,
			"--quiet",
		}
		if password != "" {
			args = append(args, "--password", password)
		}
		args = append(args, pdfPath)
		cmd := exec.Command(bin, args...)
		if cmdOut, runErr := cmd.CombinedOutput(); runErr != nil {
			for _, page := range g.pages {
				results = append(results, tier1Result{
					Label:       g.labels[page],
					PDFBasename: filepath.Base(g.pdfRel),
					Page:        page,
					Error:       fmt.Sprintf("pdfrender splash: %v: %s", runErr, sanitizeOneLine(string(cmdOut))),
				})
			}
			continue
		}

		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		for _, page := range uniq {
			r := tier1Result{
				Label:       g.labels[page],
				PDFBasename: filepath.Base(g.pdfRel),
				Page:        page,
			}
			oursPNG := filepath.Join(oursDir, fmt.Sprintf("%s_page_%04d.png", baseName, page))
			if _, err := os.Stat(oursPNG); err != nil {
				r.Error = fmt.Sprintf("ours missing: %v", err)
				results = append(results, r)
				continue
			}
			popplerPNG, err := tier1FindPopplerPNG(popplerDir, page)
			if err != nil {
				r.Error = fmt.Sprintf("poppler missing: %v", err)
				results = append(results, r)
				continue
			}
			exact, sim, err := corpusComparePNGs(oursPNG, popplerPNG)
			if err != nil {
				r.Error = fmt.Sprintf("compare: %v", err)
				results = append(results, r)
				continue
			}
			r.ExactPercent = exact
			r.SimilarityPercent = sim
			if floors != nil {
				if entry, ok := floors[tier1FloorKey(r.PDFBasename, r.Page)]; ok {
					r.Floor = entry.SimilarityFloor
					r.HasFloor = true
				}
			}
			results = append(results, r)
		}
	}

	// Snapshot mode: rewrite floors TSV from current results and exit.
	if snapshotMode {
		phase := os.Getenv("SPLASH_TIER1_PHASE")
		if phase == "" {
			phase = "P5"
		}
		if err := writeTier1Floors(floorsPath, results, phase); err != nil {
			t.Fatalf("write tier-1 floors snapshot %s: %v", floorsPath, err)
		}
		captured := 0
		for _, r := range results {
			if r.Error == "" {
				captured++
			}
		}
		t.Logf("captured tier-1 floors snapshot at %s (phase=%s, %d/%d pages captured)",
			floorsPath, phase, captured, len(results))
		return
	}

	// Verify every pinned page has a floor entry (warn, don't fail — fresh
	// pages legitimately appear before they're pinned).
	missingFloors := 0
	for _, r := range results {
		if r.Error == "" && !r.HasFloor {
			missingFloors++
			t.Logf("WARN no tier-1 floor pinned for %s p%d (sim=%.4f%%, exact=%.4f%%)",
				r.PDFBasename, r.Page, r.SimilarityPercent, r.ExactPercent)
		}
	}

	// Per-page t.Run for clearer CI output. Strictly compare similarity.
	pageFails := 0
	for _, r := range results {
		r := r
		subName := fmt.Sprintf("%s_p%d", strings.TrimSuffix(r.PDFBasename, ".pdf"), r.Page)
		t.Run(subName, func(t *testing.T) {
			if r.Error != "" {
				t.Fatalf("tier-1 measurement error for %s p%d (%s): %s",
					r.PDFBasename, r.Page, r.Label, r.Error)
			}
			if !r.HasFloor {
				t.Logf("(no floor pinned) %s p%d sim=%.4f%% exact=%.4f%%",
					r.PDFBasename, r.Page, r.SimilarityPercent, r.ExactPercent)
				return
			}
			if r.SimilarityPercent+1e-6 < r.Floor {
				t.Errorf("TIER-1 REGRESSION %s p%d (%s): similarity %.4f%% < floor %.4f%% (Δ=%+.4f)",
					r.PDFBasename, r.Page, r.Label,
					r.SimilarityPercent, r.Floor, r.SimilarityPercent-r.Floor)
			} else {
				t.Logf("OK %s p%d (%s): sim=%.4f%% (floor=%.4f%%, Δ=%+.4f), exact=%.4f%%",
					r.PDFBasename, r.Page, r.Label,
					r.SimilarityPercent, r.Floor,
					r.SimilarityPercent-r.Floor, r.ExactPercent)
			}
		})
	}
	for _, r := range results {
		if r.Error == "" && r.HasFloor && r.SimilarityPercent+1e-6 < r.Floor {
			pageFails++
		}
	}
	t.Logf("tier-1 summary: pages=%d missing_floors=%d failures=%d",
		len(results), missingFloors, pageFails)
}

// loadTier1Floors parses tier1_floors.tsv. Map key = "<basename>|<page>".
//
// Format: pdf_basename<TAB>page<TAB>similarity_floor<TAB>exact_percent_floor<TAB>captured_phase
// Lines starting with '#' or blank are ignored.
func loadTier1Floors(path string) (map[string]tier1FloorEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]tier1FloorEntry)
	for lineno, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			return nil, fmt.Errorf("tier1_floors.tsv:%d: expected 5 tab-separated fields, got %d (%q)",
				lineno+1, len(fields), line)
		}
		page, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("tier1_floors.tsv:%d: page %q: %w", lineno+1, fields[1], err)
		}
		simFloor, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return nil, fmt.Errorf("tier1_floors.tsv:%d: similarity_floor %q: %w", lineno+1, fields[2], err)
		}
		exactFloor, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		if err != nil {
			return nil, fmt.Errorf("tier1_floors.tsv:%d: exact_percent_floor %q: %w", lineno+1, fields[3], err)
		}
		entry := tier1FloorEntry{
			PDFBasename:       strings.TrimSpace(fields[0]),
			Page:              page,
			SimilarityFloor:   simFloor,
			ExactPercentFloor: exactFloor,
			Phase:             strings.TrimSpace(fields[4]),
		}
		out[tier1FloorKey(entry.PDFBasename, entry.Page)] = entry
	}
	return out, nil
}

// writeTier1Floors writes a fresh floors TSV from the given results. Only
// rows with Error=="" are emitted (skip pages where measurement failed).
//
// We intentionally write floors equal to the measured value (no head-room).
// The tier-1 invariant is "must not regress below today's number" — any
// improvement should be promoted explicitly via a follow-up snapshot.
func writeTier1Floors(path string, results []tier1Result, phase string) error {
	var sb strings.Builder
	sb.WriteString("# Splash port — Phase 5 tier-1 PR-time similarity floors.\n")
	sb.WriteString("# Source: TestSplashCorpusTier1 (SPLASH_TIER1_SNAPSHOT=1).\n")
	sb.WriteString("# Format: pdf_basename<TAB>page<TAB>similarity_floor<TAB>exact_percent_floor<TAB>captured_phase\n")
	sb.WriteString("# Hard rule: similarity_percent must not drop below similarity_floor.\n")
	for _, r := range results {
		if r.Error != "" {
			continue
		}
		fmt.Fprintf(&sb, "%s\t%d\t%.4f\t%.4f\t%s\n",
			r.PDFBasename, r.Page, r.SimilarityPercent, r.ExactPercent, phase)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// tier1FloorKey is the map key used in loadTier1Floors / lookup.
func tier1FloorKey(basename string, page int) string {
	return fmt.Sprintf("%s|%d", basename, page)
}

// tier1RunPoppler is a thin wrapper around pdftoppm with -f/-l page bounds.
// We render only the inclusive page range [first..last] to keep wall-time low.
func tier1RunPoppler(pdfPath, outDir string, dpi int, password string, first, last int) error {
	args := []string{"-png", "-r", strconv.Itoa(dpi), "-f", strconv.Itoa(first), "-l", strconv.Itoa(last)}
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

// tier1FindPopplerPNG locates the pdftoppm-produced PNG for a given page.
//
// pdftoppm zero-pads the page number to the width of the document's total
// page count (NOT the -f/-l range). For a 117-page doc the file is
// "rendered-023.png", not "rendered-23.png", even when only page 23 was
// requested. We probe widths 1..6 to handle anything realistic, then fall
// back to a directory scan as a last resort.
func tier1FindPopplerPNG(popplerDir string, page int) (string, error) {
	for width := 1; width <= 6; width++ {
		candidate := filepath.Join(popplerDir, fmt.Sprintf("rendered-%0*d.png", width, page))
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	// Last-resort scan: enumerate the directory.
	entries, err := os.ReadDir(popplerDir)
	if err != nil {
		return "", err
	}
	prefix := fmt.Sprintf("rendered-")
	suffix := ".png"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		num := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		n, perr := strconv.Atoi(num)
		if perr != nil {
			continue
		}
		if n == page {
			return filepath.Join(popplerDir, name), nil
		}
	}
	return "", fmt.Errorf("no rendered-*.png matching page %d in %s", page, popplerDir)
}

// tier1FormatPagesArg formats a sorted unique slice of pages as the
// --pages CLI argument: "1,3,5-7" style. We use plain comma-separated for
// simplicity since pdfrender accepts that form.
func tier1FormatPagesArg(pages []int) string {
	parts := make([]string, len(pages))
	for i, p := range pages {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}

// uniqueSortedInts returns a sorted, deduplicated copy of in.
func uniqueSortedInts(in []int) []int {
	seen := make(map[int]struct{}, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}
