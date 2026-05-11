// Package splashintegration contains the Phase-0 scaffolding tests for the
// Splash backend port (see /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md
// §1.3 and §3). The tests in this package validate that the harness wiring
// is correct (deterministic snapshots, byte-identity gate, fixture drift
// detection) — they do NOT yet enforce pixel parity against the Q1
// `expected/*.png` references. That correctness gate lands in Phase 4.
package splashintegration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// repoRoot resolves the go-pdf module root (parent of test/integration/splash).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to resolve current test file via runtime.Caller")
	}
	// .../go-pdf/test/integration/splash/golden_corpus_test.go -> .../go-pdf
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))
}

// splashGoldenRoot is the canonical fixture root owned by Q1.
func splashGoldenRoot(root string) string {
	return filepath.Join(root, "test", "testdata", "splash_golden")
}

// listFixtures returns sorted absolute paths to PDFs under <root>/pdfs/*.pdf.
func listFixtures(goldenRoot string) ([]string, error) {
	pdfDir := filepath.Join(goldenRoot, "pdfs")
	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		out = append(out, filepath.Join(pdfDir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// sha256File returns the lower-case hex sha256 of the file at path.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sha256Bytes returns the lower-case hex sha256 of the given bytes.
func sha256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// parseManifest reads a TSV "<sha256>\t<relpath>\n" manifest.
// Lines starting with '#' or empty lines are ignored. Returns map relpath->sha.
func parseManifest(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			// Tolerate space-separated manifests.
			fields = strings.Fields(line)
		}
		if len(fields) < 2 {
			return nil, fmt.Errorf("manifest line malformed: %q", line)
		}
		sha := strings.TrimSpace(fields[0])
		rel := strings.TrimSpace(fields[1])
		out[rel] = sha
	}
	return out, nil
}

// renderWithPDFRender invokes the pdfrender CLI with SPLASH_BACKEND set to the
// requested value and returns the path to the produced PNG (page 1).
//
// In Phase 0 the splash backend may not be wired into pdfrender yet; in that
// case pdfrender simply ignores SPLASH_BACKEND and renders via the existing
// canvas. This is fine for the harness — we only need a deterministic PNG.
func renderWithPDFRender(t *testing.T, repoRootPath, pdfPath, outDir, splashBackend string) (string, error) {
	t.Helper()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir output: %w", err)
	}

	bin := filepath.Join(repoRootPath, "bin", "pdfrender")
	args := []string{
		"--output", outDir,
		"--pages", "1",
		"--dpi", "150",
		"--quiet",
		pdfPath,
	}

	var cmd *exec.Cmd
	if _, err := os.Stat(bin); err == nil {
		cmd = exec.Command(bin, args...)
	} else {
		// Fall back to `go run ./cmd/pdfrender` so the test works on a clean
		// checkout without `make build`.
		runArgs := append([]string{"run", "./cmd/pdfrender"}, args...)
		cmd = exec.Command("go", runArgs...)
		cmd.Dir = repoRootPath
	}

	// Inherit the environment but override SPLASH_BACKEND deterministically.
	env := os.Environ()
	env = append(env, "SPLASH_BACKEND="+splashBackend)
	cmd.Env = env

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdfrender failed: %v\n%s", err, string(out))
	}

	// pdfrender emits <prefix>_page_0001.png where prefix is the input base name.
	base := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	produced := filepath.Join(outDir, base+"_page_0001.png")
	if _, err := os.Stat(produced); err != nil {
		return "", fmt.Errorf("expected output %s missing: %w", produced, err)
	}
	return produced, nil
}

// pdfrenderHonorsSplashBackend returns true if `pdfrender --help` mentions
// SPLASH_BACKEND. In Phase 0 this is expected to be false until D4 wires
// the canvas factory through. The check is best-effort; on failure the test
// caller should NOT fail — they should call t.Skipf.
func pdfrenderHonorsSplashBackend(t *testing.T, repoRootPath string) bool {
	t.Helper()
	bin := filepath.Join(repoRootPath, "bin", "pdfrender")
	var cmd *exec.Cmd
	if _, err := os.Stat(bin); err == nil {
		cmd = exec.Command(bin, "--help")
	} else {
		cmd = exec.Command("go", "run", "./cmd/pdfrender", "--help")
		cmd.Dir = repoRootPath
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		// `--help` may return non-zero on some cobra configurations; still inspect output.
		if len(out) == 0 {
			return false
		}
	}
	return strings.Contains(strings.ToUpper(string(out)), "SPLASH_BACKEND")
}

// TestSplashGoldenCorpus is the Phase-0 splash golden harness test.
//
// See /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md §1.3 +
// §3. This test validates the wiring (deterministic PNGs, manifest drift
// detection, snapshot capture/replay), NOT pixel parity vs Q1's
// `expected/*.png` — that gate lands in Phase 4.
func TestSplashGoldenCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping splash golden corpus in short mode")
	}

	root := repoRoot(t)
	goldenRoot := splashGoldenRoot(root)

	if _, err := os.Stat(filepath.Join(goldenRoot, "pdfs")); err != nil {
		t.Skip("Q1 fixtures not present (test/testdata/splash_golden/pdfs missing)")
	}

	pdfFiles, err := listFixtures(goldenRoot)
	if err != nil {
		t.Skipf("Q1 fixtures not present: %v", err)
	}
	if len(pdfFiles) == 0 {
		t.Skip("Q1 fixtures not present (test/testdata/splash_golden/pdfs is empty)")
	}

	if !pdfrenderHonorsSplashBackend(t, root) {
		t.Skipf("pdfrender does not honor SPLASH_BACKEND yet (Phase 0 wiring incomplete) — harness will skip")
	}

	// Manifest drift detection: if MANIFEST.tsv exists, every listed entry
	// must match actual sha256.
	manifestPath := filepath.Join(goldenRoot, "MANIFEST.tsv")
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		manifest, err := parseManifest(manifestPath)
		if err != nil {
			t.Fatalf("parse MANIFEST.tsv: %v", err)
		}
		for rel, expected := range manifest {
			full := filepath.Join(goldenRoot, rel)
			actual, err := sha256File(full)
			if err != nil {
				t.Errorf("manifest entry %q: %v", rel, err)
				continue
			}
			if actual != expected {
				t.Errorf("manifest drift for %q: expected %s, got %s", rel, expected, actual)
			}
		}
	}

	snapshotMode := os.Getenv("SPLASH_GOLDEN_SNAPSHOT") == "1"
	producedRoot := filepath.Join(goldenRoot, "produced")

	for _, pdfPath := range pdfFiles {
		pdfPath := pdfPath
		name := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		t.Run(name, func(t *testing.T) {
			// Produce twice to verify determinism.
			outDirA := filepath.Join(t.TempDir(), "a")
			pngA, err := renderWithPDFRender(t, root, pdfPath, outDirA, "1")
			if err != nil {
				t.Fatalf("first render: %v", err)
			}
			outDirB := filepath.Join(t.TempDir(), "b")
			pngB, err := renderWithPDFRender(t, root, pdfPath, outDirB, "1")
			if err != nil {
				t.Fatalf("second render: %v", err)
			}

			bytesA, err := os.ReadFile(pngA)
			if err != nil {
				t.Fatalf("read first PNG: %v", err)
			}
			bytesB, err := os.ReadFile(pngB)
			if err != nil {
				t.Fatalf("read second PNG: %v", err)
			}

			shaA := sha256Bytes(bytesA)
			shaB := sha256Bytes(bytesB)
			if shaA != shaB {
				t.Fatalf("nondeterministic splash render for %s: %s vs %s", name, shaA, shaB)
			}
			if len(bytesA) == 0 {
				t.Fatalf("produced empty PNG for %s", name)
			}

			// Snapshot compare/capture under <golden>/produced/<name>.png.
			snapshotPath := filepath.Join(producedRoot, name+".png")
			if snapshotMode {
				if err := os.MkdirAll(producedRoot, 0o755); err != nil {
					t.Fatalf("mkdir produced: %v", err)
				}
				if err := os.WriteFile(snapshotPath, bytesA, 0o644); err != nil {
					t.Fatalf("write snapshot %s: %v", snapshotPath, err)
				}
				t.Logf("captured snapshot: %s (sha256=%s)", snapshotPath, shaA)
				return
			}
			if _, err := os.Stat(snapshotPath); err != nil {
				t.Logf("no snapshot for %s yet — run with SPLASH_GOLDEN_SNAPSHOT=1 to capture", name)
				return
			}
			snapBytes, err := os.ReadFile(snapshotPath)
			if err != nil {
				t.Fatalf("read snapshot %s: %v", snapshotPath, err)
			}
			snapSha := sha256Bytes(snapBytes)
			if snapSha != shaA {
				t.Errorf("snapshot drift for %s: snapshot=%s produced=%s\n"+
					"  Phase-0 acceptance: byte-equality between consecutive runs (PASSED) — \n"+
					"  this is a snapshot-vs-current drift, regenerate with SPLASH_GOLDEN_SNAPSHOT=1 if intentional.",
					name, snapSha, shaA)
			}
		})
	}
}
