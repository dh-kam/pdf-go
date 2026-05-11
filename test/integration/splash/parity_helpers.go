// Helper utilities for the Phase 1 splash stroke parity gate
// (see /workspace/pdf-reader/tmp/splash_port_design/05_test_strategy.md
// §1.3 golden-bitmap tier).
//
// These helpers are intentionally separate from the Q2 harness in
// golden_corpus_test.go: they are used by parity_test.go (this owner)
// and will be reused by future Phase 2 parity tests. Stdlib only.
package splashintegration

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// fixtureEntry is one row from MANIFEST.tsv. The Q1 manifest format is
// 4-column (fixture, primitive, bytes, sha256) for shipped entries and
// "# TODO" prefix for the 9 not-yet-built fixtures.
type fixtureEntry struct {
	Name      string // basename without extension, e.g. "05_thin_hline_1px"
	RelPath   string // e.g. "pdfs/05_thin_hline_1px.pdf" or "expected/...png"
	Primitive string // e.g. "thin-hstroke"
	Bytes     int
	Sha256    string
	IsTODO    bool
}

// parityRepoRoot resolves the go-pdf module root (parent of test/integration/splash).
// Named distinctly from repoRoot in golden_corpus_test.go to avoid collision.
func parityRepoRoot(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to resolve current test file via runtime.Caller")
	}
	// .../go-pdf/test/integration/splash/parity_helpers.go -> .../go-pdf
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))
}

// loadFixtureManifest parses MANIFEST.tsv (Q1's 4-column format).
//
// Header line:  "fixture\tprimitive\tbytes\tsha256"
// Data line:    "<relpath>\t<primitive>\t<bytes>\t<sha256>"
// TODO line:    "# TODO\t<relpath>\t<primitive>\t<note>"
//
// Comment-only lines (starting with '#' but not '# TODO') are ignored.
func loadFixtureManifest(t *testing.T, path string) []fixtureEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest %s: %v", path, err)
	}
	out := make([]fixtureEntry, 0, 32)
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Header row.
		if strings.HasPrefix(line, "fixture\t") {
			continue
		}
		// TODO row: "# TODO\t<relpath>\t<primitive>\t<note>".
		if strings.HasPrefix(line, "# TODO") {
			fields := strings.Split(line, "\t")
			if len(fields) < 4 {
				continue
			}
			out = append(out, fixtureEntry{
				Name:      basenameNoExt(fields[1]),
				RelPath:   fields[1],
				Primitive: fields[2],
				IsTODO:    true,
			})
			continue
		}
		// Pure comment.
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			t.Fatalf("manifest %s line %d malformed: %q", path, lineNo+1, line)
		}
		var nbytes int
		fmt.Sscanf(strings.TrimSpace(fields[2]), "%d", &nbytes)
		out = append(out, fixtureEntry{
			Name:      basenameNoExt(fields[0]),
			RelPath:   strings.TrimSpace(fields[0]),
			Primitive: strings.TrimSpace(fields[1]),
			Bytes:     nbytes,
			Sha256:    strings.TrimSpace(fields[3]),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func basenameNoExt(rel string) string {
	base := filepath.Base(rel)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// sha256Hex returns the lower-case hex sha256 of the given bytes.
//
// Distinct from the existing sha256Bytes helper in golden_corpus_test.go to
// avoid sharing a name (Go allows it within one package, but keeping the
// helper file self-contained is clearer for future Phase 2 reuse).
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// pdfrenderHasBackendFlag reports whether `pdfrender --help` advertises the
// new --backend flag with a "splash" choice. Phase 2 wires the splash backend
// behind --backend=splash; if the binary predates W1's wiring the test skips.
//
// (Renamed from pdfrenderHasStrokeBackend in Phase 1: the SPLASH_BACKEND_STROKE
// env knob has been retired.)
func pdfrenderHasBackendFlag(t *testing.T, repoRootPath string) bool {
	t.Helper()
	bin := filepath.Join(repoRootPath, "bin", "pdfrender")
	var cmd *exec.Cmd
	if _, err := os.Stat(bin); err == nil {
		cmd = exec.Command(bin, "--help")
	} else {
		cmd = exec.Command("go", "run", "./cmd/pdfrender", "--help")
		cmd.Dir = repoRootPath
	}
	out, _ := cmd.CombinedOutput()
	if len(out) == 0 {
		return false
	}
	helpText := string(out)
	return strings.Contains(helpText, "--backend") && strings.Contains(helpText, "splash")
}

// renderPDFViaSplash invokes the pdfrender CLI with --backend=splash and
// returns the produced page-1 PNG bytes.
//
// Phase 2: the env-var seam (SPLASH_BACKEND / SPLASH_BACKEND_STROKE) is gone;
// W1 wired a real --backend flag through cobra. We pass --backend=splash on
// the args slice and inject no env overlay.
func renderPDFViaSplash(t *testing.T, pdfPath string) ([]byte, error) {
	t.Helper()
	repoRootPath := parityRepoRoot(t)
	outDir := t.TempDir()

	bin := filepath.Join(repoRootPath, "bin", "pdfrender")
	args := []string{
		"--backend=splash",
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
		runArgs := append([]string{"run", "./cmd/pdfrender"}, args...)
		cmd = exec.Command("go", runArgs...)
		cmd.Dir = repoRootPath
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdfrender failed: %v\n%s", err, string(out))
	}

	base := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	produced := filepath.Join(outDir, base+"_page_0001.png")
	pngBytes, err := os.ReadFile(produced)
	if err != nil {
		return nil, fmt.Errorf("read produced PNG %s: %w", produced, err)
	}
	return pngBytes, nil
}

// stripPNGtIMEChunk returns a copy of pngData with any tIME chunks removed.
//
// PNG layout: 8-byte signature, then a sequence of chunks. Each chunk is
// [length:uint32 BE][type:4 bytes][data:length bytes][crc:uint32 BE].
// tIME is non-critical (lowercase first char of type), so removal does not
// break decoders. The function validates the result by re-decoding.
func stripPNGtIMEChunk(pngData []byte) []byte {
	const sigLen = 8
	if len(pngData) < sigLen {
		return pngData
	}
	// PNG signature: 89 50 4E 47 0D 0A 1A 0A
	want := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.Equal(pngData[:sigLen], want) {
		return pngData
	}

	out := make([]byte, 0, len(pngData))
	out = append(out, pngData[:sigLen]...)

	i := sigLen
	for i+8 <= len(pngData) {
		length := binary.BigEndian.Uint32(pngData[i : i+4])
		// Bounds: length+12 (4 len + 4 type + length data + 4 crc).
		end := i + 4 + 4 + int(length) + 4
		if end > len(pngData) {
			// Truncated; return original to be safe.
			return pngData
		}
		ctype := string(pngData[i+4 : i+8])
		if ctype != "tIME" {
			out = append(out, pngData[i:end]...)
		}
		// Stop after IEND for cleanliness (its CRC is included).
		if ctype == "IEND" {
			i = end
			break
		}
		i = end
	}

	// Validate by decoding; on failure return the original (best-effort).
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		return pngData
	}
	return out
}

// comparePNGsPixelEqual decodes both PNG byte slices and compares their RGBA
// pixel data byte-by-byte. Returns (true, 0, nil) if pixel-equal; otherwise
// (false, mismatchCount, nil). Per R-B 2026-04-27: parity gate is pixel-level,
// not PNG-byte-level. DEFLATE bitstream differences between Go compress/flate
// and libpng zlib are encoder-only and not rendering correctness.
//
// On dimension mismatch or decode error, mismatchCount is -1 and err is set.
func comparePNGsPixelEqual(a, b []byte) (equal bool, mismatchCount int, err error) {
	imgA, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return false, -1, fmt.Errorf("decode A: %w", err)
	}
	imgB, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return false, -1, fmt.Errorf("decode B: %w", err)
	}
	if imgA.Bounds() != imgB.Bounds() {
		return false, -1, fmt.Errorf("size mismatch: %v vs %v", imgA.Bounds(), imgB.Bounds())
	}
	rgbaA := toRGBA(imgA)
	rgbaB := toRGBA(imgB)
	if len(rgbaA.Pix) != len(rgbaB.Pix) {
		return false, -1, fmt.Errorf("Pix length mismatch: %d vs %d",
			len(rgbaA.Pix), len(rgbaB.Pix))
	}
	mismatchCount = 0
	for i := range rgbaA.Pix {
		if rgbaA.Pix[i] != rgbaB.Pix[i] {
			mismatchCount++
		}
	}
	return mismatchCount == 0, mismatchCount, nil
}

// toRGBA returns img as *image.RGBA (zero-copy if already RGBA, otherwise
// draws into a fresh RGBA buffer with the same bounds).
func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba
}

// saveDiffArtifacts writes the produced and expected PNGs to
// tmp/splash_parity_failures/<fixture>.{actual,expected}.png so a human can
// inspect the diff after a parity failure. Best-effort: errors are logged
// via t.Logf rather than failing the test.
func saveDiffArtifacts(t *testing.T, fixture string, produced, expected []byte) {
	t.Helper()
	root := parityRepoRoot(t)
	dir := filepath.Join(root, "tmp", "splash_parity_failures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("saveDiffArtifacts: mkdir %s: %v", dir, err)
		return
	}
	actualPath := filepath.Join(dir, fixture+".actual.png")
	expectedPath := filepath.Join(dir, fixture+".expected.png")
	if err := os.WriteFile(actualPath, produced, 0o644); err != nil {
		t.Logf("saveDiffArtifacts: write %s: %v", actualPath, err)
	}
	if err := os.WriteFile(expectedPath, expected, 0o644); err != nil {
		t.Logf("saveDiffArtifacts: write %s: %v", expectedPath, err)
	}
}
