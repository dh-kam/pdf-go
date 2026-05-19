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

const exact100GeoTopoPagesRelPath = "test/testdata/splash_baseline/exact100_geotopo_pages.tsv"

type exact100GeoTopoCase struct {
	PDFRel string
	Page   int
	Reason string
}

// TestSplashExact100GeoTopoRegressions pins the GeoTopo pages that drove the
// Poppler parity fixes. The expected bitmap is generated from pdftoppm at test
// time so official testdata stays small and source-controlled.
func TestSplashExact100GeoTopoRegressions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Poppler bitmap integration test in short mode")
	}
	if !splashHasFreeType {
		t.Skip("GeoTopo exact100 glyph regressions require the FreeType-backed glyph path")
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

	cases := loadExact100GeoTopoCases(t, filepath.Join(root, filepath.FromSlash(exact100GeoTopoPagesRelPath)))
	scanRoot := filepath.Join(root, "test", "testdata", "sample-files")
	outRoot := t.TempDir()

	type group struct {
		pdfRel  string
		pages   []int
		reasons map[int]string
	}
	groupOrder := make([]string, 0, len(cases))
	groups := make(map[string]*group)
	for _, tc := range cases {
		g, ok := groups[tc.PDFRel]
		if !ok {
			g = &group{pdfRel: tc.PDFRel, reasons: map[int]string{}}
			groups[tc.PDFRel] = g
			groupOrder = append(groupOrder, tc.PDFRel)
		}
		g.pages = append(g.pages, tc.Page)
		g.reasons[tc.Page] = tc.Reason
	}
	sort.Strings(groupOrder)

	for _, key := range groupOrder {
		g := groups[key]
		pdfPath := filepath.Join(scanRoot, filepath.FromSlash(g.pdfRel))
		if _, err := os.Stat(pdfPath); err != nil {
			t.Fatalf("source PDF missing %s: %v", pdfPath, err)
		}

		uniq := uniqueSortedInts(g.pages)
		docDir := filepath.Join(outRoot, corpusSanitizeDocID(g.pdfRel))
		popplerDir := filepath.Join(docDir, "poppler")
		oursDir := filepath.Join(docDir, "ours")
		if err := os.MkdirAll(popplerDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", popplerDir, err)
		}
		if err := os.MkdirAll(oursDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", oursDir, err)
		}

		minPage, maxPage := uniq[0], uniq[len(uniq)-1]
		password := corpusPasswordFor(g.pdfRel)
		if err := tier1RunPoppler(pdfPath, popplerDir, corpusDPI, password, minPage, maxPage); err != nil {
			t.Fatalf("pdftoppm %s p%d-%d: %v", g.pdfRel, minPage, maxPage, err)
		}

		args := []string{
			"--backend=splash",
			"--output", oursDir,
			"--dpi", strconv.Itoa(corpusDPI),
			"--pages", tier1FormatPagesArg(uniq),
			"--quiet",
		}
		if password != "" {
			args = append(args, "--password", password)
		}
		args = append(args, pdfPath)
		cmd := exec.Command(bin, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("pdfrender %s pages %v: %v\n%s", g.pdfRel, uniq, err, strings.TrimSpace(string(out)))
		}

		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		for _, page := range uniq {
			page := page
			t.Run(fmt.Sprintf("%s_p%d_%s", baseName, page, g.reasons[page]), func(t *testing.T) {
				oursPNG := filepath.Join(oursDir, fmt.Sprintf("%s_page_%04d.png", baseName, page))
				popplerPNG, err := tier1FindPopplerPNG(popplerDir, page)
				if err != nil {
					t.Fatalf("locate poppler PNG: %v", err)
				}
				exact, sim, err := corpusComparePNGs(oursPNG, popplerPNG)
				if err != nil {
					t.Fatalf("compare PNGs: %v", err)
				}
				if exact != 100.0 {
					t.Fatalf("exact %.9f%%, similarity %.9f%%; want exact 100.000000000%%", exact, sim)
				}
			})
		}
	}
}

func loadExact100GeoTopoCases(t *testing.T, path string) []exact100GeoTopoCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exact100 geotopo cases %s: %v", path, err)
	}

	var out []exact100GeoTopoCase
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "pdf\t") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("%s:%d: expected 3 TSV fields, got %d: %q", path, lineNo+1, len(fields), raw)
		}
		page, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("%s:%d: invalid page %q: %v", path, lineNo+1, fields[1], err)
		}
		out = append(out, exact100GeoTopoCase{
			PDFRel: filepath.ToSlash(fields[0]),
			Page:   page,
			Reason: fields[2],
		})
	}
	if len(out) == 0 {
		t.Fatalf("%s: no exact100 cases", path)
	}
	return out
}
