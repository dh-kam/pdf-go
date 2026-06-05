package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRefusesRepositoryRootOutput(t *testing.T) {
	err := run(config{
		repoRoot: ".",
		outDir:   ".",
		dpi:      150,
		workers:  1,
	})
	if err == nil {
		t.Fatal("expected repository-root output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use repository root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesRepositoryAncestor(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	if err := os.Mkdir(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	err := validateOutputDirectory(repoRoot, root, "")
	if err == nil {
		t.Fatal("expected repository ancestor output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use repository root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesScanRootAncestor(t *testing.T) {
	repoRoot := t.TempDir()
	scanRoot := filepath.Join("test", "2nd")
	outDir := filepath.Join(repoRoot, "test")

	err := validateOutputDirectory(repoRoot, outDir, scanRoot)
	if err == nil {
		t.Fatal("expected scan-root ancestor output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use scan root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesScanRootItself(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, "test", "2nd")

	err := validateOutputDirectory(repoRoot, outDir, "test/2nd")
	if err == nil {
		t.Fatal("expected scan-root output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use scan root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesSymlinkedScanRootAncestor(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, "test", "2nd"), 0o755); err != nil {
		t.Fatal(err)
	}
	repoLink := filepath.Join(root, "repo-link")
	if err := os.Symlink(repoRoot, repoLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := validateOutputDirectory(repoRoot, filepath.Join(repoLink, "test"), "test/2nd")
	if err == nil {
		t.Fatal("expected symlinked scan-root ancestor output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use scan root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryAllowsRepositoryChild(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, "tmp", "compare")

	if err := validateOutputDirectory(repoRoot, outDir, "test/2nd"); err != nil {
		t.Fatalf("expected repository child output directory to be allowed: %v", err)
	}
}

func TestCompareDocumentSkipsZeroPagePDF(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "zero.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pdftoppm := writeScript(t, dir, "pdftoppm", `#!/bin/sh
echo 'Syntax Error: Invalid page count 0 Wrong page range given: the first page (1) can not be after the last page (0).' >&2
exit 1
`)
	pdfrender := writeScript(t, dir, "pdfrender", `#!/bin/sh
exit 0
`)

	rows := compareDocument(config{
		repoRoot:          dir,
		outDir:            filepath.Join(dir, "out"),
		dpi:               150,
		workers:           1,
		backend:           "splash",
		imageSamplingMode: "legacy",
		pdftoppm:          pdftoppm,
		pdfrender:         pdfrender,
	}, pdfJob{
		path: pdfPath,
		rel:  "zero.pdf",
		slug: "0001_zero",
	}, nil)
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want zero-page PDF to be skipped", len(rows))
	}
}

func TestComparePNGsCollectsBadPixelDiagnostics(t *testing.T) {
	dir := t.TempDir()
	popplerPath := filepath.Join(dir, "poppler.png")
	oursPath := filepath.Join(dir, "ours.png")
	diffPath := filepath.Join(dir, "diff.png")

	poppler := image.NewRGBA(image.Rect(0, 0, 4, 3))
	ours := image.NewRGBA(image.Rect(0, 0, 4, 3))
	fillImage(poppler, color.RGBA{255, 255, 255, 255})
	fillImage(ours, color.RGBA{255, 255, 255, 255})
	poppler.SetRGBA(1, 1, color.RGBA{10, 20, 30, 255})
	poppler.SetRGBA(2, 1, color.RGBA{10, 20, 30, 255})
	ours.SetRGBA(1, 1, color.RGBA{12, 18, 40, 255})
	ours.SetRGBA(2, 1, color.RGBA{10, 20, 31, 255})
	poppler.SetRGBA(0, 2, color.RGBA{100, 100, 100, 255})
	ours.SetRGBA(0, 2, color.RGBA{90, 110, 100, 255})

	writeTestPNG(t, popplerPath, poppler)
	writeTestPNG(t, oursPath, ours)

	stats, diag, err := comparePNGs(popplerPath, oursPath, diffPath, compareOptions{
		tileSize:      2,
		writeDiff:     false,
		badPixelLimit: 2,
		needPixels:    true,
		needRegions:   true,
	})
	if err != nil {
		t.Fatalf("comparePNGs returned error: %v", err)
	}
	if stats.matchedPixels != 9 || stats.totalPixels != 12 {
		t.Fatalf("stats = matched %d total %d, want 9/12", stats.matchedPixels, stats.totalPixels)
	}
	if len(diag.pixels) != 2 {
		t.Fatalf("bad pixel samples = %d, want 2", len(diag.pixels))
	}
	if diag.pixels[0].x != 1 || diag.pixels[0].y != 1 || diag.pixels[0].deltaMax != 10 || diag.pixels[0].regionID != 1 {
		t.Fatalf("first sample = %+v, want x=1 y=1 deltaMax=10 regionID=1", diag.pixels[0])
	}
	if diag.pixels[1].x != 2 || diag.pixels[1].y != 1 || diag.pixels[1].deltaMax != 1 || diag.pixels[1].regionID != 1 {
		t.Fatalf("second sample = %+v, want x=2 y=1 deltaMax=1 regionID=1", diag.pixels[1])
	}
	if len(diag.regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(diag.regions))
	}
	if diag.regions[0].badPixels != 2 || diag.regions[0].x0 != 1 || diag.regions[0].x1 != 2 || diag.regions[0].maxDelta != 10 {
		t.Fatalf("first region = %+v, want 2-pixel region across x=1..2 maxDelta=10", diag.regions[0])
	}
	if diag.regions[1].badPixels != 1 || diag.regions[1].x0 != 0 || diag.regions[1].y0 != 2 {
		t.Fatalf("second region = %+v, want single pixel at 0,2", diag.regions[1])
	}
	if _, err := os.Stat(diffPath); !os.IsNotExist(err) {
		t.Fatalf("diff path stat err = %v, want not exist when writeDiff=false", err)
	}
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fillImage(img *image.RGBA, c color.RGBA) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func writeTestPNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
