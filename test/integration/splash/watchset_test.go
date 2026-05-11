// Package splashintegration — Phase 1 watch-set non-regression gate.
//
// See /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md §3
// (watch-set CI gate) and §4 (regression floor). The watch set is the eight
// hardest pages of the corpus. exact_percent for each MUST stay at or above
// its pinned floor; project memory: "similarity 만 오르고 exact 가 내려가면
// 되돌린다" (similarity-only progress is reverted).
package splashintegration

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// watchSetEntry is one pinned floor row.
type watchSetEntry struct {
	basename string  // substring matched against report.csv column "pdf"
	page     int     // page number (column "page")
	floor    float64 // exact_percent baseline floor (column "exact_percent")
	phase    string  // captured_phase tag (e.g. "P1")
}

// watchSetWantPages is the canonical list of (basename-substring, page) pairs
// from 05_test_strategy.md §3. The TSV baseline file MUST cover every entry
// here; missing entries fail the test loudly.
var watchSetWantPages = []struct {
	basename string
	page     int
}{
	{"GeoTopo", 23},
	{"GeoTopo", 35},
	{"GeoTopo", 44},
	{"GeoTopo", 55},
	{"GeoTopo", 96},
	{"GeoTopo", 97},
	{"google-doc-document", 1},
	{"libreoffice-form", 1},
}

// watchSetBaselineRelPath is the pinned-baseline TSV (Q1-owned territory; we
// only read it here — Phase 1 ships the values). See file header for format.
const watchSetBaselineRelPath = "test/testdata/splash_baseline/watchset.tsv"

// renderParityReportRelPath is the most recent parity report. Watch-set rows
// are filtered out of it. Test skips if absent (CI may not have generated it
// yet — bootstrap order: parity report -> watch-set gate).
const renderParityReportRelPath = "test/testdata/output/render_parity/report.csv"

// loadWatchSetBaseline parses the TSV. Returns map keyed by "basename|page".
func loadWatchSetBaseline(path string) (map[string]watchSetEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]watchSetEntry)
	for lineno, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			return nil, fmt.Errorf("watchset.tsv:%d: expected 4 tab-separated fields, got %d (%q)", lineno+1, len(fields), line)
		}
		page, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("watchset.tsv:%d: page %q: %w", lineno+1, fields[1], err)
		}
		floor, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return nil, fmt.Errorf("watchset.tsv:%d: floor %q: %w", lineno+1, fields[2], err)
		}
		entry := watchSetEntry{
			basename: strings.TrimSpace(fields[0]),
			page:     page,
			floor:    floor,
			phase:    strings.TrimSpace(fields[3]),
		}
		out[fmt.Sprintf("%s|%d", entry.basename, entry.page)] = entry
	}
	return out, nil
}

// reportRow is one CSV row we care about.
type reportRow struct {
	pdf          string
	page         int
	exactPercent float64
}

// readReportCSV returns rows for every (basename-substring, page) requested.
// If a request is unmatched, the returned map simply won't contain the key.
func readReportCSV(path string, wants []struct {
	basename string
	page     int
}) (map[string]reportRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate trailing empty error column
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("report.csv: empty")
	}
	header := records[0]
	idx := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				return i
			}
		}
		return -1
	}
	pdfCol := idx("pdf")
	pageCol := idx("page")
	exactCol := idx("exact_percent")
	if pdfCol < 0 || pageCol < 0 || exactCol < 0 {
		return nil, fmt.Errorf("report.csv: missing required columns (got %v)", header)
	}

	out := make(map[string]reportRow)
	for _, rec := range records[1:] {
		if len(rec) <= exactCol {
			continue
		}
		page, err := strconv.Atoi(strings.TrimSpace(rec[pageCol]))
		if err != nil {
			continue
		}
		exact, err := strconv.ParseFloat(strings.TrimSpace(rec[exactCol]), 64)
		if err != nil {
			continue
		}
		pdf := rec[pdfCol]
		// Match each want; the first matching row wins.
		for _, w := range wants {
			if w.page != page {
				continue
			}
			if !strings.Contains(pdf, w.basename) {
				continue
			}
			key := fmt.Sprintf("%s|%d", w.basename, w.page)
			if _, dup := out[key]; dup {
				continue
			}
			out[key] = reportRow{pdf: pdf, page: page, exactPercent: exact}
		}
	}
	return out, nil
}

// TestSplashWatchSet — Phase 1 non-regression gate (05_test_strategy.md §3).
func TestSplashWatchSet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash watch-set in short mode (slow)")
	}

	root := repoRoot(t)
	baselinePath := filepath.Join(root, filepath.FromSlash(watchSetBaselineRelPath))
	reportPath := filepath.Join(root, filepath.FromSlash(renderParityReportRelPath))

	baseline, err := loadWatchSetBaseline(baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("watch-set baseline absent at %s — run with SPLASH_WATCHSET_SNAPSHOT=1 to capture", baselinePath)
		}
		t.Fatalf("load watch-set baseline %s: %v", baselinePath, err)
	}

	// Verify every canonical entry is pinned.
	for _, w := range watchSetWantPages {
		key := fmt.Sprintf("%s|%d", w.basename, w.page)
		if _, ok := baseline[key]; !ok {
			t.Fatalf("watch-set baseline missing entry for %s p%d (expected by 05_test_strategy.md §3)", w.basename, w.page)
		}
	}

	if _, err := os.Stat(reportPath); err != nil {
		t.Skipf("render_parity/report.csv absent at %s — generate first via TestRenderParityReportAgainstPoppler", reportPath)
	}

	rows, err := readReportCSV(reportPath, watchSetWantPages)
	if err != nil {
		t.Fatalf("read report.csv %s: %v", reportPath, err)
	}

	// Snapshot mode: rewrite TSV from current report and skip the gate.
	snapshotMode := os.Getenv("SPLASH_WATCHSET_SNAPSHOT") == "1"
	if snapshotMode {
		var sb strings.Builder
		sb.WriteString("# Splash port — watch-set regression floor.\n")
		sb.WriteString("# Auto-captured by SPLASH_WATCHSET_SNAPSHOT=1 from " + reportPath + "\n")
		sb.WriteString("# Format: pdf_basename<TAB>page<TAB>exact_percent_floor<TAB>captured_phase\n")
		phase := os.Getenv("SPLASH_WATCHSET_PHASE")
		if phase == "" {
			phase = "P1"
		}
		for _, w := range watchSetWantPages {
			key := fmt.Sprintf("%s|%d", w.basename, w.page)
			row, ok := rows[key]
			if !ok {
				t.Fatalf("snapshot capture: report.csv has no row for %s p%d", w.basename, w.page)
			}
			fmt.Fprintf(&sb, "%s\t%d\t%.4f\t%s\n", w.basename, w.page, row.exactPercent, phase)
		}
		if err := os.WriteFile(baselinePath, []byte(sb.String()), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", baselinePath, err)
		}
		t.Logf("captured watch-set snapshot at %s", baselinePath)
		return
	}

	splashBackend := os.Getenv("SPLASH_BACKEND") == "1"
	splashForce := os.Getenv("SPLASH_BACKEND_FORCE_RUN") == "1"
	if splashBackend && !splashForce {
		t.Skip("splash backend not yet plumbed end-to-end through evaluator (Phase 4 default-on); set SPLASH_BACKEND_FORCE_RUN=1 to override")
	}

	for _, w := range watchSetWantPages {
		w := w
		key := fmt.Sprintf("%s|%d", w.basename, w.page)
		entry := baseline[key]
		t.Run(fmt.Sprintf("%s_p%d", strings.ReplaceAll(w.basename, ".pdf", ""), w.page), func(t *testing.T) {
			row, ok := rows[key]
			if !ok {
				t.Skipf("report.csv has no row for %s p%d (was the report regenerated for the full corpus?)", w.basename, w.page)
			}
			delta := row.exactPercent - entry.floor
			if row.exactPercent+1e-6 < entry.floor {
				t.Fatalf("WATCH SET REGRESSION: %s p%d: %.4f%% -> %.4f%% (Δ=%+.4f, baseline phase %s, file %s)",
					w.basename, w.page, entry.floor, row.exactPercent, delta, entry.phase, row.pdf)
			}
			t.Logf("OK %s p%d: floor=%.4f%% actual=%.4f%% (Δ=%+.4f, phase=%s)",
				w.basename, w.page, entry.floor, row.exactPercent, delta, entry.phase)
		})
	}
}
