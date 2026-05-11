package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	_ "image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	e2eBinaryCacheDirOnce sync.Once
	e2eBinaryCacheDir     string
	e2eBinaryCacheDirErr  error
	e2eBinaryPathCache    sync.Map
)

func TestCLI_PDFInfo(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	out := runGoCommand(t, root, "run", "-tags=nojpx,nojbig2", "./cmd/pdfinfo", "-p", sample)
	assert.Contains(t, out, "Page Count: 1")
	assert.Contains(t, out, "Page 1:")
}

func TestCLI_PDFInfo_Outlines(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "006-pdflatex-outline", "pdflatex-outline.pdf")

	out := runGoCommand(t, root, "run", "-tags=nojpx,nojbig2", "./cmd/pdfinfo", "-o", sample)
	assert.Contains(t, out, "Outlines:")
	assert.Contains(t, out, "- Foo")
}

func TestCLI_PDFInfo_OutlinesJSON(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "006-pdflatex-outline", "pdflatex-outline.pdf")

	out := runGoCommand(t, root, "run", "-tags=nojpx,nojbig2", "./cmd/pdfinfo", "-j", "-o", sample)

	var result struct {
		Outlines []struct {
			Title      string `json:"title"`
			ActionType string `json:"action_type"`
		} `json:"outlines"`
	}
	err := json.Unmarshal([]byte(out), &result)
	require.NoError(t, err)
	require.NotEmpty(t, result.Outlines)

	assert.NotEmpty(t, result.Outlines[0].Title)
	assert.Equal(t, "GoTo", result.Outlines[0].ActionType)
}

func TestCLI_PDFInfo_FormFields(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "010-pdflatex-forms", "pdflatex-forms.pdf")

	out := runGoCommand(t, root, "run", "-tags=nojpx,nojbig2", "./cmd/pdfinfo", "-f", sample)
	assert.Contains(t, out, "Form Fields:")
	assert.Contains(t, out, "[")
}

func TestCLI_PDFText(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	out := runGoCommand(t, root, "run", "-tags=nojpx,nojbig2", "./cmd/pdftext", sample)
	assert.Contains(t, out, "Lorem ipsum")
	assert.Contains(t, out, "dolor sit amet")
	assert.NotContains(t, out, "\x01")
}

func TestCLI_PDFRender(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")
	outputDir := t.TempDir()
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")

	_ = runCommand(t, root, pdfrender, "-q", "-p", "1", "-o", outputDir, sample)

	expected := filepath.Join(outputDir, "002-trivial-libre-office-writer_page_0001.png")
	_, err := os.Stat(expected)
	require.NoError(t, err)
}

func assertCLIImageSamplingTraceContains(
	t *testing.T,
	root string,
	sample string,
	expected []string,
) {
	t.Helper()

	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	for _, pattern := range expected {
		assert.Contains(t, out, pattern)
	}
}

func assertCLITinyGray4x4DownscaleTrace(
	t *testing.T,
	root string,
	sample string,
	filter string,
	sampler string,
	reason string,
) {
	t.Helper()

	assertCLIImageSamplingTraceContains(t, root, sample, []string{
		"filter=" + filter,
		"colorspace=DeviceGray",
		"edge_candidate=rejected_non_rgb_colorspace",
		"edge_mode=default",
		"sampler=" + sampler,
		"reason=" + reason,
		"experimental_candidate=rejected_strict_downscale",
		"ctm=[4.000000 0.000000 0.000000 4.000000 0.000000 0.000000]",
		"dst=(x=0.0000 y=0.0000 w=4.0000 h=4.0000) src=16x16",
	})
}

func assertCLITinyGrayDownscaleTraceAtDPI(
	t *testing.T,
	root string,
	sample string,
	dpi string,
	filter string,
	sampler string,
	reason string,
	ctm string,
	dst string,
) {
	t.Helper()

	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"--dpi", dpi,
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	for _, pattern := range []string{
		"filter=" + filter,
		"colorspace=DeviceGray",
		"edge_candidate=rejected_non_rgb_colorspace",
		"edge_mode=default",
		"sampler=" + sampler,
		"reason=" + reason,
		ctm,
		dst,
	} {
		assert.Contains(t, out, pattern)
	}
}

func assertCLIIndexedLargeDownscaleTrace(
	t *testing.T,
	root string,
	sample string,
	indexedBase string,
	ctm string,
	dst string,
) {
	t.Helper()

	assertCLIImageSamplingTraceContains(t, root, sample, []string{
		"sampler=auto_downscale_bilinear",
		"reason=auto_interpolate=false_downscale",
		"experimental_candidate=rejected_large_source",
		"colorspace=Indexed",
		"indexed_base=" + indexedBase,
		"indexed_palette_entries=256",
		"cmyk_candidate=rejected_non_cmyk_indexed_base",
		"ctm=" + ctm,
		"phase=(x=0.0000 y=0.0000)",
		"dst=" + dst,
	})
}

func TestCLI_PDFRender_ImageSamplingTrace_007TinyGrayDownscale(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "007-imagemagick-images", "imagemagick-images.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Equal(t, 6, strings.Count(out, "sampler=auto_box_tiny_iccbased_gray_downscale"))
	assert.Equal(t, 6, strings.Count(out, "reason=auto_interpolate=false_downscale_tiny_iccbased_gray"))
	assert.Equal(t, 6, strings.Count(out, "experimental_candidate=rejected_strict_downscale"))
	assert.Equal(t, 6, strings.Count(out, "ctm=[4.000000 0.000000 0.000000 4.000000 0.000000 0.000000]"))
	assert.Equal(t, 6, strings.Count(out, "dst=(x=0.0000 y=0.0000 w=4.0000 h=4.0000) src=16x16"))
}

func TestCLI_PDFRender_ImageSamplingTrace_007CCITTFaxMatchesSmileBucket(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "007-imagemagick-images", "imagemagick-CCITTFaxDecode.pdf")

	assertCLITinyGray4x4DownscaleTrace(
		t,
		root,
		sample,
		"CCITTFaxDecode",
		"auto_approx_bilinear_tiny_gray_ccittfax_downscale",
		"auto_interpolate=false_downscale_tiny_gray_ccittfax",
	)
	assertCLIImageSamplingTraceContains(t, root, sample, []string{
		"phase=(x=0.5000 y=0.5000)",
	})
}

func TestCLI_PDFRender_ImageSamplingTrace_007TinyGrayDownscaleAt150DPI(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "007-imagemagick-images", "imagemagick-images.pdf")

	assertCLITinyGrayDownscaleTraceAtDPI(
		t,
		root,
		sample,
		"150",
		"FlateDecode",
		"auto_box_tiny_iccbased_gray_downscale",
		"auto_interpolate=false_downscale_tiny_iccbased_gray",
		"ctm=[8.000000 0.000000 0.000000 8.000000 0.000000 0.000000]",
		"dst=(x=0.0000 y=0.0000 w=8.0000 h=8.0000) src=16x16",
	)
}

func TestCLI_PDFRender_ImageSamplingTrace_007MainPage4ExperimentalDCTGrayIgnoreICC(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "007-imagemagick-images", "imagemagick-images.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "4",
		"--image-sampling-mode", "experimental-dct-gray-ignore-icc-v1",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "filter=DCTDecode")
	assert.Contains(t, out, "colorspace=DeviceGray")
	assert.Contains(t, out, "gray_icc_candidate=candidate_tiny_dct_iccbased_gray_downscale")
	assert.Contains(t, out, "gray_icc_profile_mode=ignore")
	assert.Contains(t, out, "sampler=auto_box_tiny_iccbased_gray_downscale")
}

func TestCLI_PDFRender_ImageSamplingTrace_007MainPage4LegacySelectiveDCTGrayIgnoreICC(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "007-imagemagick-images", "imagemagick-images.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "4",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "filter=DCTDecode")
	assert.Contains(t, out, "colorspace=DeviceGray")
	assert.Contains(t, out, "gray_icc_candidate=candidate_tiny_dct_iccbased_gray_downscale")
	assert.Contains(t, out, "gray_icc_profile_mode=legacy_selective_ignore")
	assert.Contains(t, out, "sampler=auto_box_tiny_iccbased_gray_downscale")
}

func TestCLI_PDFRender_ImageSamplingTrace_019LargeSourceDownscale(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "sample-files", "019-grayscale-image", "grayscale-image.pdf")

	assertCLIImageSamplingTraceContains(t, root, sample, []string{
		"sampler=experimental_indexed_origin_downscale_bilinear",
		"reason=legacy_selective_indexed_origin_downscale_phase",
		"experimental_candidate=rejected_large_source",
		"colorspace=Indexed",
		"indexed_base=DeviceGray",
		"indexed_palette_entries=256",
		"indexed_gray_candidate=candidate_large_indexed_gray_origin_downscale",
		"cmyk_candidate=rejected_non_cmyk_indexed_base",
		"ctm=[243.000000 0.000000 0.000000 338.000000 0.000000 0.000000]",
		"phase=(x=0.5000 y=0.5000)",
		"dst=(x=0.0000 y=0.0000 w=243.0000 h=338.0000) src=324x450",
	})
}

func TestCLI_PDFRender_ImageSamplingTrace_018RGBSubpixelDownscale(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "018-base64-image", "base64image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "filter=FlateDecode")
	assert.Contains(t, out, "colorspace=DeviceRGB")
	assert.Contains(t, out, "sampler=auto_downscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_downscale")
	assert.Contains(t, out, "experimental_candidate=rejected_colorspace")
	assert.Contains(t, out, "ctm=[375.336197 0.000000 0.000000 300.999394 4.836078 536.169673]")
	assert.Contains(t, out, "phase=(x=0.0000 y=0.0000)")
	assert.Contains(t, out, "dst=(x=4.8361 y=536.1697 w=375.3362 h=300.9994) src=781x627")
}

func TestCLI_PDFRender_ImageSamplingTrace_003RGBSubpixelUpscale(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "003-pdflatex-image", "pdflatex-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "filter=DCTDecode")
	assert.Contains(t, out, "colorspace=DeviceRGB")
	assert.Contains(t, out, "sampler=auto_upscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_upscale")
	assert.Contains(t, out, "experimental_candidate=rejected_colorspace")
	assert.Contains(t, out, "edge_candidate=candidate_positive_subpixel_vertical_offset")
	assert.Contains(t, out, "edge_mode=default")
	assert.Contains(t, out, "ctm=[300.364873 0.000000 0.000000 200.026132 147.817564 412.629907]")
	assert.Contains(t, out, "phase=(x=0.0000 y=0.0000)")
	assert.Contains(t, out, "dst=(x=147.8176 y=412.6299 w=300.3649 h=200.0261) src=300x200")
}

func TestCLI_PDFRender_ImageSamplingTrace_008InlineRGBUpscale(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "008-reportlab-inline-image", "inline-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "filter=none")
	assert.Contains(t, out, "colorspace=DeviceRGB")
	assert.Contains(t, out, "sampler=auto_upscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_upscale")
	assert.Contains(t, out, "experimental_candidate=rejected_colorspace")
	assert.Contains(t, out, "ctm=[100.121692 0.000000 0.000000 100.013090 100.121692 100.013090]")
	assert.Contains(t, out, "phase=(x=0.0000 y=0.0000)")
	assert.Contains(t, out, "dst=(x=100.1217 y=100.0131 w=100.1217 h=100.0131) src=16x16")
}

func TestCLI_PDFRender_ImageSamplingTrace_004TextOnlyHasNoImageTrace(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "004-pdflatex-4-pages", "pdflatex-4-pages.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.NotContains(t, out, "sampler=")
	assert.NotContains(t, out, "colorspace=")
	assert.NotContains(t, out, "ctm=[")
}

func TestCLI_PDFRender_ImageSamplingTrace_SmileMatchesTinyGrayCCITTFaxSignature(t *testing.T) {
	root := getRepoRoot(t)
	sample := filepath.Join(root, "test", "testdata", "pdf_samples", "smile.pdf")

	assertCLITinyGray4x4DownscaleTrace(
		t,
		root,
		sample,
		"CCITTFaxDecode",
		"auto_approx_bilinear_tiny_gray_ccittfax_downscale",
		"auto_interpolate=false_downscale_tiny_gray_ccittfax",
	)
	assertCLIImageSamplingTraceContains(t, root, sample, []string{
		"phase=(x=0.5000 y=0.5000)",
	})
}

func TestCLI_PDFRender_ImageSamplingTrace_023OffsetLargeSourceDownscale(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "023-cmyk-image", "cmyk-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "legacy",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "sampler=auto_downscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_downscale")
	assert.Contains(t, out, "experimental_candidate=rejected_large_source")
	assert.Contains(t, out, "colorspace=Indexed")
	assert.Contains(t, out, "indexed_base=DeviceCMYK")
	assert.Contains(t, out, "indexed_palette_entries=256")
	assert.Contains(t, out, "cmyk_conversion_mode=hybrid-75")
	assert.Contains(t, out, "cmyk_candidate=candidate_large_indexed_cmyk_downscale")
	assert.Contains(t, out, "ctm=[468.000000 0.000000 0.000000 624.000000 72.000000 96.000000]")
	assert.Contains(t, out, "phase=(x=0.0000 y=0.0000)")
	assert.Contains(t, out, "dst=(x=72.0000 y=96.0000 w=468.0000 h=624.0000) src=756x1008")
}

func TestCLI_PDFRender_ImageSamplingTrace_SampleCorpusIndexedDeviceCMYKOnlyDoc023(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sampleRoot := filepath.Join(root, "test", "testdata", "sample-files")
	outputDir := t.TempDir()

	var hits []string
	err := filepath.WalkDir(sampleRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".pdf" {
			return nil
		}

		out := runCommand(
			t,
			root,
			pdfrender,
			"-q",
			"-p", "1",
			"--image-sampling-mode", "legacy",
			"--debug-image-sampling",
			"-o", outputDir,
			path,
		)
		if strings.Contains(out, "indexed_base=DeviceCMYK") {
			relPath, err := filepath.Rel(sampleRoot, path)
			if err != nil {
				return err
			}
			hits = append(hits, filepath.ToSlash(relPath))
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(hits)
	assert.Equal(t, []string{"023-cmyk-image/cmyk-image.pdf"}, hits)
}

func TestCLI_PDFRender_ImageSamplingTrace_019ExperimentalIndexedOriginDownscalePhase(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "019-grayscale-image", "grayscale-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "experimental-indexed-origin-downscale-phase-v1",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "sampler=experimental_indexed_origin_downscale_bilinear")
	assert.Contains(t, out, "reason=experimental_indexed_origin_downscale_phase")
	assert.Contains(t, out, "phase=(x=0.5000 y=0.5000)")
	assert.Contains(t, out, "ctm=[243.000000 0.000000 0.000000 338.000000 0.000000 0.000000]")
}

func TestCLI_PDFRender_ImageSamplingTrace_023ExperimentalIndexedOriginDownscalePhaseKeepsZeroPhase(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "023-cmyk-image", "cmyk-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "experimental-indexed-origin-downscale-phase-v1",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "sampler=experimental_indexed_origin_downscale_bilinear")
	assert.Contains(t, out, "reason=experimental_indexed_origin_downscale_phase")
	assert.Contains(t, out, "cmyk_conversion_mode=default")
	assert.Contains(t, out, "phase=(x=0.0000 y=0.0000)")
	assert.Contains(t, out, "ctm=[468.000000 0.000000 0.000000 624.000000 72.000000 96.000000]")
}

func TestCLI_PDFRender_ImageSamplingTrace_023ExperimentalIndexedCMYKStdlib(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "023-cmyk-image", "cmyk-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "experimental-indexed-cmyk-stdlib-v1",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "sampler=auto_downscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_downscale")
	assert.Contains(t, out, "colorspace=Indexed")
	assert.Contains(t, out, "indexed_base=DeviceCMYK")
	assert.Contains(t, out, "cmyk_conversion_mode=stdlib")
	assert.Contains(t, out, "cmyk_candidate=candidate_large_indexed_cmyk_downscale")
	assert.Contains(t, out, "ctm=[468.000000 0.000000 0.000000 624.000000 72.000000 96.000000]")
}

func TestCLI_PDFRender_ImageSamplingTrace_023ExperimentalIndexedCMYKSimple(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "023-cmyk-image", "cmyk-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "experimental-indexed-cmyk-simple-v1",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "sampler=auto_downscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_downscale")
	assert.Contains(t, out, "colorspace=Indexed")
	assert.Contains(t, out, "indexed_base=DeviceCMYK")
	assert.Contains(t, out, "cmyk_conversion_mode=simple-subtractive")
	assert.Contains(t, out, "cmyk_candidate=candidate_large_indexed_cmyk_downscale")
	assert.Contains(t, out, "ctm=[468.000000 0.000000 0.000000 624.000000 72.000000 96.000000]")
}

func TestCLI_PDFRender_ImageSamplingTrace_023ExperimentalIndexedCMYKHybrid75(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "023-cmyk-image", "cmyk-image.pdf")
	outputDir := t.TempDir()

	out := runCommand(
		t,
		root,
		pdfrender,
		"-q",
		"-p", "1",
		"--image-sampling-mode", "experimental-indexed-cmyk-hybrid75-v1",
		"--debug-image-sampling",
		"-o", outputDir,
		sample,
	)

	assert.Contains(t, out, "sampler=auto_downscale_bilinear")
	assert.Contains(t, out, "reason=auto_interpolate=false_downscale")
	assert.Contains(t, out, "colorspace=Indexed")
	assert.Contains(t, out, "indexed_base=DeviceCMYK")
	assert.Contains(t, out, "cmyk_conversion_mode=hybrid-75")
	assert.Contains(t, out, "cmyk_candidate=candidate_large_indexed_cmyk_downscale")
	assert.Contains(t, out, "ctm=[468.000000 0.000000 0.000000 624.000000 72.000000 96.000000]")
}

func TestCLI_PDFRender_ExactBaseline(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")

	for _, tc := range exactBaselineRenderCases(root) {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIRenderMatchesBaseline(t, root, pdfrender, tc)
		})
	}
}

func TestCLI_PDFRender_Hybrid75MatchesLegacyForFocusBaselines(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")

	for _, tc := range exactBaselineRenderCases(root) {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIRenderModesMatch(
				t,
				root,
				pdfrender,
				tc.pdfPath,
				[]string{"-q", "-p", "1", "--image-sampling-mode", "legacy"},
				[]string{"-q", "-p", "1", "--image-sampling-mode", "experimental-indexed-cmyk-hybrid75-v1"},
			)
		})
	}
}

func TestCLI_PDFRender_CMYKNotBlank(t *testing.T) {
	root := getRepoRoot(t)
	pdfrender := buildNoCGoBinary(t, root, "./cmd/pdfrender")
	sample := filepath.Join(root, "test", "testdata", "sample-files", "023-cmyk-image", "cmyk-image.pdf")
	outputDir := t.TempDir()

	_ = runCommand(t, root, pdfrender, "-q", "-p", "1", "-d", "150", "-o", outputDir, sample)

	renderedPath := filepath.Join(outputDir, "cmyk-image_page_0001.png")
	f, err := os.Open(renderedPath)
	require.NoError(t, err)
	defer f.Close()

	img, _, err := image.Decode(f)
	require.NoError(t, err)

	nonWhite := countNonWhitePixels(img)
	assert.Greater(t, nonWhite, 0, "CMYK rendered output should not be fully white")
}

func runGoCommand(t *testing.T, workdir string, args ...string) string {
	t.Helper()
	return runCommand(t, workdir, "go", args...)
}

type exactBaselineCase struct {
	name        string
	pdfPath     string
	outputName  string
	baselinePNG string
}

func exactBaselineRenderCases(root string) []exactBaselineCase {
	return []exactBaselineCase{
		{
			name:        "minimal_document",
			pdfPath:     filepath.Join(root, "test", "testdata", "sample-files", "001-trivial", "minimal-document.pdf"),
			outputName:  "minimal-document_page_0001.png",
			baselinePNG: filepath.Join(root, "test", "testdata", "output", "minimal-document.pdf.png"),
		},
		{
			name:        "libreoffice_writer",
			pdfPath:     filepath.Join(root, "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf"),
			outputName:  "002-trivial-libre-office-writer_page_0001.png",
			baselinePNG: filepath.Join(root, "test", "testdata", "output", "002-trivial-libre-office-writer.pdf.png"),
		},
		{
			name:        "pdflatex_image",
			pdfPath:     filepath.Join(root, "test", "testdata", "sample-files", "003-pdflatex-image", "pdflatex-image.pdf"),
			outputName:  "pdflatex-image_page_0001.png",
			baselinePNG: filepath.Join(root, "test", "testdata", "output", "pdflatex-image.pdf.png"),
		},
		{
			name:        "imagemagick_images",
			pdfPath:     filepath.Join(root, "test", "testdata", "sample-files", "007-imagemagick-images", "imagemagick-images.pdf"),
			outputName:  "imagemagick-images_page_0001.png",
			baselinePNG: filepath.Join(root, "test", "testdata", "output", "imagemagick-images.pdf.png"),
		},
		{
			name:        "inline_image",
			pdfPath:     filepath.Join(root, "test", "testdata", "sample-files", "008-reportlab-inline-image", "inline-image.pdf"),
			outputName:  "inline-image_page_0001.png",
			baselinePNG: filepath.Join(root, "test", "testdata", "output", "inline-image.pdf.png"),
		},
		{
			name:        "google_doc_document",
			pdfPath:     filepath.Join(root, "test", "testdata", "sample-files", "011-google-doc-document", "google-doc-document.pdf"),
			outputName:  "google-doc-document_page_0001.png",
			baselinePNG: filepath.Join(root, "test", "testdata", "output", "google-doc-document.pdf.png"),
		},
	}
}

func assertCLIRenderMatchesBaseline(t *testing.T, root string, pdfrender string, tc exactBaselineCase, extraArgs ...string) {
	t.Helper()

	outputDir := t.TempDir()
	args := []string{"-q", "-p", "1"}
	args = append(args, extraArgs...)
	args = append(args, "-o", outputDir, tc.pdfPath)
	_ = runCommand(t, root, pdfrender, args...)

	renderedPath := filepath.Join(outputDir, tc.outputName)
	rendered, err := os.ReadFile(renderedPath)
	require.NoError(t, err)

	expected, err := os.ReadFile(tc.baselinePNG)
	require.NoError(t, err)

	if bytes.Equal(rendered, expected) {
		return
	}

	renderedHash := sha256.Sum256(rendered)
	expectedHash := sha256.Sum256(expected)
	require.FailNowf(
		t,
		"test failed",
		"%s\nrendered: %s (%s)\nexpected: %s (%s)",
		tc.name,
		renderedPath,
		hex.EncodeToString(renderedHash[:]),
		tc.baselinePNG,
		hex.EncodeToString(expectedHash[:]),
	)
}

func assertCLIRenderModesMatch(t *testing.T, root string, pdfrender string, pdfPath string, leftArgs []string, rightArgs []string) {
	t.Helper()

	leftDir := t.TempDir()
	rightDir := t.TempDir()

	leftCLIArgs := append(append([]string{}, leftArgs...), "-o", leftDir, pdfPath)
	rightCLIArgs := append(append([]string{}, rightArgs...), "-o", rightDir, pdfPath)
	_ = runCommand(t, root, pdfrender, leftCLIArgs...)
	_ = runCommand(t, root, pdfrender, rightCLIArgs...)

	leftPNG, err := firstPNGInDir(leftDir)
	require.NoError(t, err)
	rightPNG, err := firstPNGInDir(rightDir)
	require.NoError(t, err)

	leftRendered, err := os.ReadFile(leftPNG)
	require.NoError(t, err)
	rightRendered, err := os.ReadFile(rightPNG)
	require.NoError(t, err)

	if bytes.Equal(leftRendered, rightRendered) {
		return
	}

	leftHash := sha256.Sum256(leftRendered)
	rightHash := sha256.Sum256(rightRendered)
	require.FailNowf(
		t,
		"test failed",
		"legacy/hybrid mismatch\nleft: %s (%s)\nright: %s (%s)",
		leftPNG,
		hex.EncodeToString(leftHash[:]),
		rightPNG,
		hex.EncodeToString(rightHash[:]),
	)
}

func firstPNGInDir(root string) (string, error) {
	var first string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".png" {
			return nil
		}
		first = path
		return filepath.SkipAll
	})
	if err != nil {
		return "", err
	}
	if first == "" {
		return "", os.ErrNotExist
	}
	return first, nil
}

func runCommand(t *testing.T, workdir string, command string, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command failed: %s %v\noutput:\n%s", command, args, string(out))

	return string(out)
}

func buildNoCGoBinary(t *testing.T, workdir string, pkg string) string {
	t.Helper()

	cacheKey := workdir + "|" + pkg
	if cached, ok := e2eBinaryPathCache.Load(cacheKey); ok {
		return cached.(string)
	}

	e2eBinaryCacheDirOnce.Do(func() {
		e2eBinaryCacheDir, e2eBinaryCacheDirErr = os.MkdirTemp("", "go-pdf-e2e-bin-*")
	})
	require.NoError(t, e2eBinaryCacheDirErr)

	binaryName := filepath.Base(pkg)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(e2eBinaryCacheDir, binaryName)
	_ = runGoCommand(t, workdir, "build", "-tags=nojpx,nojbig2", "-o", binaryPath, pkg)

	actual, _ := e2eBinaryPathCache.LoadOrStore(cacheKey, binaryPath)
	return actual.(string)
}

func getRepoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func countNonWhitePixels(img image.Image) int {
	if img == nil {
		return 0
	}

	bounds := img.Bounds()
	nonWhite := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r != 0xFFFF || g != 0xFFFF || b != 0xFFFF || a != 0xFFFF {
				nonWhite++
			}
		}
	}
	return nonWhite
}
