// Package canvas — SP4 Phase 5 cleanup guard test.
//
// This test pins the set of production files outside `internal/infrastructure/canvas/`
// that consume the concrete `*ImageCanvas` type or `NewImageCanvas` constructor.
// Adding a new external coupling to `*ImageCanvas` blocks the eventual deletion
// of the legacy backend (see `tmp/splash_port_design/c1_cleanup_plan.md`).
//
// If you are intentionally adding a new consumer (and have a good reason), update
// the allowlist below in the same PR. If you are migrating an existing consumer
// off `*ImageCanvas` (good!), shrink the allowlist.
//
// Tests under `_test.go` files, the `cmd/` debug-tools, and `tmp/` probe binaries
// are deliberately excluded from the scan — they are not gating splash cutover.
package canvas

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// allowedExternalImageCanvasConsumers is the Phase-5 baseline (2026-04-27).
// Each entry is a path relative to the go-pdf module root that is permitted
// to mention `canvas.ImageCanvas` or `canvas.NewImageCanvas` in non-test code.
//
// The 5c blocker list in c1_cleanup_plan.md §2 maps directly to these entries.
var allowedExternalImageCanvasConsumers = []string{
	// Phase-5c blockers — must be migrated before image_canvas.go can be deleted.
	"internal/infrastructure/renderer/annotation_rendering.go",
	"internal/infrastructure/renderer/concurrent_renderer.go",
	"cmd/pdfrender/main.go",

	// Dev probe tools — slated for retirement alongside Phase 5b/5c.
	"cmd/match_matrix.go",
	"cmd/match_matrix2.go",
	"cmd/formula_compare.go",
	"cmd/testglyph/main.go",
}

var imageCanvasCouplingRe = regexp.MustCompile(`\bcanvas\.(NewImageCanvas|ImageCanvas)\b`)

// TestNoExternalImageCanvasCouplingBeyondAllowlist scans every `.go` file under
// the module root (excluding `_test.go`, the `canvas` package itself, and `tmp/`)
// and fails if a file outside the allowlist references the concrete `ImageCanvas`
// type or its constructor.
func TestNoExternalImageCanvasCouplingBeyondAllowlist(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Fatalf("locate module root: %v", err)
	}

	allowed := make(map[string]struct{}, len(allowedExternalImageCanvasConsumers))
	for _, p := range allowedExternalImageCanvasConsumers {
		allowed[filepath.ToSlash(p)] = struct{}{}
	}

	var violations []string
	walkErr := filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip irrelevant subtrees up front.
			rel, _ := filepath.Rel(moduleRoot, path)
			rel = filepath.ToSlash(rel)
			switch {
			case rel == "tmp", strings.HasPrefix(rel, "tmp/"):
				return filepath.SkipDir
			case rel == "internal/infrastructure/canvas":
				return filepath.SkipDir
			case rel == "test":
				// Allow descent — but the file filter below skips _test.go anyway.
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !imageCanvasCouplingRe.Match(data) {
			return nil
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		rel = filepath.ToSlash(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}
		violations = append(violations, rel)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk module: %v", walkErr)
	}

	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf(
			"new external consumer(s) of *canvas.ImageCanvas detected (Phase-5 cleanup blocker).\n"+
				"Either migrate to the splash backend / domaincanvas.Canvas interface, OR\n"+
				"add the file to allowedExternalImageCanvasConsumers in cleanup_check_test.go\n"+
				"with a justification in the PR description.\n"+
				"Offending files:\n  - %s",
			strings.Join(violations, "\n  - "),
		)
	}
}

// findModuleRoot walks up from the test's working directory looking for go.mod.
// We do not import build/runtime helpers because this test must work even if the
// caller invoked `go test` from a sub-package directory.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
