package splashintegration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPhase0CSVByteIdentity is the most important Phase-0 gate.
//
// See /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md §3.
// With SPLASH_BACKEND=0 (or unset) the parity report CSV must remain
// byte-for-byte identical to the baseline `report.csv` captured on `main`.
// Any drift means the splash plumbing leaked into the default code path —
// a Phase-0 hard fail.
func TestPhase0CSVByteIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Phase-0 CSV byte-identity gate in short mode")
	}
	if os.Getenv("SPLASH_PHASE0_GATE") != "1" {
		t.Skip("Set SPLASH_PHASE0_GATE=1 to run the full byte-identity gate (slow; runs full corpus)")
	}

	root := repoRoot(t)

	baselinePath := filepath.Join(root, "test", "testdata", "output", "render_parity", "report.csv")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Skipf("no baseline CSV present at %s — first invocation; run `make render-parity-report-test` on main first", baselinePath)
	}
	baselineSha, err := sha256File(baselinePath)
	if err != nil {
		t.Fatalf("hash baseline CSV: %v", err)
	}

	// Stage the baseline aside so the parity test's RemoveAll(outputRoot)
	// doesn't destroy our reference before we hash the regenerated copy.
	staged := filepath.Join(t.TempDir(), "baseline.report.csv")
	if data, err := os.ReadFile(baselinePath); err == nil {
		if err := os.WriteFile(staged, data, 0o644); err != nil {
			t.Fatalf("stage baseline CSV: %v", err)
		}
	} else {
		t.Fatalf("read baseline CSV: %v", err)
	}

	// Run the parity report under SPLASH_BACKEND=0 (the legacy path).
	// We use `make render-parity-report-test` so this exercises exactly the
	// same invocation the original CSV was produced under.
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not available — render parity baseline cannot be reproduced")
	}

	cmd := exec.Command("make", "render-parity-report-test")
	cmd.Dir = root
	env := os.Environ()
	env = append(env, "SPLASH_BACKEND=0")
	cmd.Env = env

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make render-parity-report-test failed: %v\n%s", err, string(out))
	}

	regenSha, err := sha256File(baselinePath)
	if err != nil {
		t.Fatalf("hash regenerated CSV: %v", err)
	}

	if regenSha != baselineSha {
		t.Fatalf("Phase-0 CSV byte-identity FAILED:\n  baseline (staged):       %s (sha256=%s)\n  regenerated (current):   %s (sha256=%s)\n"+
			"  This means SPLASH_BACKEND=0 no longer reproduces `main`'s report.csv byte-for-byte.\n"+
			"  Investigate canvas_factory dispatch — Phase 0 forbids any drift on the legacy path.",
			staged, baselineSha, baselinePath, regenSha)
	}
}
