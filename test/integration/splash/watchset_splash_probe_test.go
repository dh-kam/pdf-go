// Phase 2 splash watch-set diagnostic probe.
//
// Owned by P2-QA2 (see /workspace/pdf-reader/tmp/splash_port_design/
// 05_test_strategy.md §3 watch-set + 04_phase_plan.md Phase 2 exit criteria).
//
// Unlike TestSplashWatchSet (CSV-driven non-regression gate against the
// pinned watchset.tsv floor), this probe directly invokes pdfrender with
// --backend=splash on the 8 hardest watch-set pages and captures per-page
// status (success / Splash error / blank / partial) plus PNG byte counts
// and SHA-256 prefixes. It is a DIAGNOSTIC TOOL, not a pass/fail gate —
// it documents what Phase 2's Fill+Clip+Scanner backend can actually
// render on real corpus pages. Phase 3+ uses this baseline to track
// progress page by page.
//
// The probe asserts only one weak invariant: at least one watch-set page
// must produce a non-empty PNG. If splash backend is 100% broken end-to-end
// the probe fails loudly. Otherwise it logs and writes summary.tsv.
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

// watchSetProbeEntry maps a watch-set page to its source PDF.
//
// The 8 entries match watchSetWantPages in watchset_test.go but resolve to
// concrete on-disk PDF paths under test/testdata/sample-files/.
type watchSetProbeEntry struct {
	docID   string // short label for output filenames, e.g. "GeoTopo"
	relPath string // PDF path relative to repo root
	page    int    // 1-based page index
}

// watchSetProbeEntries is the canonical list driving the probe.
//
// Source PDFs live under test/testdata/sample-files/<NNN>-<slug>/<file>.pdf.
// If a PDF is missing on this checkout the probe skips that entry (it is a
// diagnostic, not a fixture-integrity gate).
var watchSetProbeEntries = []watchSetProbeEntry{
	{"GeoTopo", "test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf", 23},
	{"GeoTopo", "test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf", 35},
	{"GeoTopo", "test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf", 44},
	{"GeoTopo", "test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf", 55},
	{"GeoTopo", "test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf", 96},
	{"GeoTopo", "test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf", 97},
	{"google-doc-document", "test/testdata/sample-files/011-google-doc-document/google-doc-document.pdf", 1},
	{"libreoffice-form", "test/testdata/sample-files/012-libreoffice-form/libreoffice-form.pdf", 1},
}

// probeRecord is one row in summary.tsv.
type probeRecord struct {
	docID    string
	page     int
	status   string // "ok" | "err" | "missing" | "empty"
	bytes    int64
	shaShort string // first 12 hex chars; "-" when not applicable
	note     string // skip/error reason, free text
}

// TestSplashWatchSetProbe renders the watch-set under --backend=splash and
// records per-page status into tmp/splash_watchset_probe/summary.tsv plus
// individual PNGs. Diagnostic, not a pass/fail gate.
//
//nolint:gocyclo // straight-line probe loop; further factoring obscures intent.
func TestSplashWatchSetProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping splash watch-set probe in -short mode (slow: 8 pages × ~30s)")
	}

	root := parityRepoRoot(t)
	if !pdfrenderHasBackendFlag(t, root) {
		t.Skipf("pdfrender does not support --backend flag — " +
			"build with `go build -o bin/pdfrender ./cmd/pdfrender` " +
			"after W1's --backend=splash wiring landed.")
	}

	bin := filepath.Join(root, "bin", "pdfrender")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("pdfrender binary not built at %s — run `make build-pdfrender`", bin)
	}

	outRoot := filepath.Join(root, "tmp", "splash_watchset_probe")
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outRoot, err)
	}

	records := make([]probeRecord, 0, len(watchSetProbeEntries))
	successCount := 0
	nonEmptyCount := 0

	for _, entry := range watchSetProbeEntries {
		pdfPath := filepath.Join(root, filepath.FromSlash(entry.relPath))
		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		outName := fmt.Sprintf("%s_p%d.png", baseName, entry.page)

		rec := probeRecord{
			docID:    entry.docID,
			page:     entry.page,
			shaShort: "-",
		}

		if _, err := os.Stat(pdfPath); err != nil {
			rec.status = "missing"
			rec.note = fmt.Sprintf("source PDF absent: %v", err)
			t.Logf("%s p%d: status=missing (%s)", entry.docID, entry.page, pdfPath)
			records = append(records, rec)
			continue
		}

		// pdfrender names outputs <prefix>_page_NNNN.png. Use a dedicated
		// per-entry output dir so we can find the file deterministically and
		// then rename to the probe-friendly name.
		entryDir := filepath.Join(outRoot, fmt.Sprintf("%s_p%d", baseName, entry.page))
		if err := os.RemoveAll(entryDir); err != nil {
			t.Fatalf("clean entry dir %s: %v", entryDir, err)
		}
		if err := os.MkdirAll(entryDir, 0o755); err != nil {
			t.Fatalf("mkdir entry dir %s: %v", entryDir, err)
		}

		args := []string{
			"--backend=splash",
			"--output", entryDir,
			"--pages", strconv.Itoa(entry.page),
			"--dpi", "150",
			"--quiet",
			pdfPath,
		}
		cmd := exec.Command(bin, args...)
		stdoutErr, runErr := cmd.CombinedOutput()
		if runErr != nil {
			rec.status = "err"
			rec.note = sanitizeOneLine(fmt.Sprintf("pdfrender exit: %v: %s", runErr, string(stdoutErr)))
			t.Logf("%s p%d: status=err bytes=0 sha=- note=%s",
				entry.docID, entry.page, rec.note)
			records = append(records, rec)
			continue
		}

		producedPath := filepath.Join(entryDir, fmt.Sprintf("%s_page_%04d.png", baseName, entry.page))
		stat, statErr := os.Stat(producedPath)
		if statErr != nil {
			rec.status = "err"
			rec.note = sanitizeOneLine(fmt.Sprintf("PNG missing post-render: %v", statErr))
			t.Logf("%s p%d: status=err bytes=0 sha=- note=%s",
				entry.docID, entry.page, rec.note)
			records = append(records, rec)
			continue
		}
		rec.bytes = stat.Size()

		data, readErr := os.ReadFile(producedPath)
		if readErr != nil {
			rec.status = "err"
			rec.note = sanitizeOneLine(fmt.Sprintf("PNG read: %v", readErr))
			records = append(records, rec)
			continue
		}

		// Move into the probe-friendly stable filename for downstream tooling.
		stablePath := filepath.Join(outRoot, outName)
		if err := os.Rename(producedPath, stablePath); err != nil {
			// Best-effort copy fallback (cross-device etc).
			if writeErr := os.WriteFile(stablePath, data, 0o644); writeErr != nil {
				t.Fatalf("write %s: %v", stablePath, writeErr)
			}
		}
		_ = os.RemoveAll(entryDir)

		rec.shaShort = sha256Hex(data)[:12]
		switch {
		case rec.bytes == 0:
			rec.status = "empty"
			rec.note = "0-byte PNG"
		case rec.bytes < 200:
			rec.status = "empty"
			rec.note = "PNG suspiciously small (<200 bytes); likely blank/empty page"
		default:
			rec.status = "ok"
			nonEmptyCount++
		}
		if rec.status == "ok" {
			successCount++
		}
		t.Logf("%s p%d: status=%s bytes=%d sha=%s",
			entry.docID, entry.page, rec.status, rec.bytes, rec.shaShort)
		records = append(records, rec)
	}

	// Stable summary order (by docID then page).
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].docID != records[j].docID {
			return records[i].docID < records[j].docID
		}
		return records[i].page < records[j].page
	})

	summaryPath := filepath.Join(outRoot, "summary.tsv")
	if err := writeProbeSummary(summaryPath, records); err != nil {
		t.Fatalf("write summary %s: %v", summaryPath, err)
	}
	t.Logf("probe summary written to %s (ok=%d non_empty=%d total=%d)",
		summaryPath, successCount, nonEmptyCount, len(records))

	// Sanity invariant: if every watch-set page is empty/err/missing the
	// splash backend is 100% broken end-to-end. That's a hard fail regardless
	// of the diagnostic-vs-gate distinction.
	totalAttempted := 0
	for _, rec := range records {
		if rec.status != "missing" {
			totalAttempted++
		}
	}
	if totalAttempted > 0 && nonEmptyCount == 0 {
		t.Fatalf("splash backend produced ZERO non-empty PNGs across %d watch-set pages — "+
			"the --backend=splash pipeline appears 100%% broken end-to-end "+
			"(see %s for per-page detail)", totalAttempted, summaryPath)
	}
	if totalAttempted == 0 {
		t.Skipf("no watch-set source PDFs present on this checkout — probe is a no-op")
	}
}

// sanitizeOneLine collapses a multi-line string into a single TSV-safe line.
func sanitizeOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " | ")
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return strings.TrimSpace(s)
}

// writeProbeSummary writes the probe results as TSV: doc, page, status, bytes, sha12, note.
func writeProbeSummary(path string, records []probeRecord) error {
	var sb strings.Builder
	sb.WriteString("# splash watch-set diagnostic probe summary\n")
	sb.WriteString("# columns: doc<TAB>page<TAB>status<TAB>bytes<TAB>sha12<TAB>note\n")
	sb.WriteString("doc\tpage\tstatus\tbytes\tsha12\tnote\n")
	for _, rec := range records {
		fmt.Fprintf(&sb, "%s\t%d\t%s\t%d\t%s\t%s\n",
			rec.docID, rec.page, rec.status, rec.bytes, rec.shaShort, rec.note)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
