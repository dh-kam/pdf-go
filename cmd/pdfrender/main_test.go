package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestParsePageSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    []int
		wantErr bool
	}{
		{name: "empty", spec: "", want: nil},
		{name: "single page", spec: "3", want: []int{3}},
		{name: "range", spec: "2-4", want: []int{2, 3, 4}},
		{name: "mixed", spec: "1,3,5-7", want: []int{1, 3, 5, 6, 7}},
		{name: "invalid number", spec: "a", wantErr: true},
		{name: "reverse range", spec: "5-2", wantErr: true},
		{name: "zero", spec: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePageSpec(tt.spec)
			if tt.wantErr {
				if err == nil {
					require.FailNowf(t, "test failed", "expected error, got nil")
				}
				return
			}
			if err != nil {
				require.FailNowf(t, "test failed", "unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				require.FailNowf(t, "test failed", "unexpected pages: got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestResolveOutputDir(t *testing.T) {
	tests := []struct {
		name      string
		inputPath string
		outputDir string
		want      string
	}{
		{name: "explicit output", inputPath: "docs/test.pdf", outputDir: "out", want: "out"},
		{name: "default output", inputPath: "/tmp/sample.pdf", outputDir: "", want: "sample_rendered"},
		{name: "no extension", inputPath: "/tmp/sample", outputDir: "", want: "sample_rendered"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveOutputDir(tt.inputPath, tt.outputDir)
			if got != tt.want {
				require.FailNowf(t, "test failed", "unexpected output dir: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestResolvePagesToRender(t *testing.T) {
	tests := []struct {
		name      string
		pages     []int
		pageCount int
		want      []int
		errMsg    string
	}{
		{name: "all", pages: nil, pageCount: 3, want: []int{0, 1, 2}},
		{name: "subset", pages: []int{1, 3}, pageCount: 4, want: []int{0, 2}},
		{name: "invalid", pages: []int{0}, pageCount: 2, want: nil, errMsg: "page 0 out of range (1-2)"},
		{name: "invalid too high", pages: []int{3}, pageCount: 2, want: nil, errMsg: "page 3 out of range (1-2)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePagesToRender(tt.pages, tt.pageCount)
			if tt.errMsg != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSaveImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	t.Run("png", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "page.png")
		require.NoError(t, saveImage(img, path, "png"))
		f, err := os.Open(path)
		require.NoError(t, err)
		defer f.Close()
		_, err = png.Decode(f)
		require.NoError(t, err)
	})

	t.Run("unsupported format returns error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "page.bmp")
		require.Error(t, saveImage(img, path, "bmp"))
		_, err := os.Stat(path)
		require.NoError(t, err)
	})
}

func TestNormalizeImageFormat(t *testing.T) {
	t.Run("png", func(t *testing.T) {
		format, err := normalizeImageFormat("png")
		require.NoError(t, err)
		assert.Equal(t, "png", format)
	})

	t.Run("upper-case png", func(t *testing.T) {
		format, err := normalizeImageFormat("PNG")
		require.NoError(t, err)
		assert.Equal(t, "png", format)
	})

	t.Run("empty", func(t *testing.T) {
		format, err := normalizeImageFormat("")
		require.NoError(t, err)
		assert.Equal(t, "png", format)
	})

	t.Run("unsupported", func(t *testing.T) {
		_, err := normalizeImageFormat("jpg")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported format")
	})
}

func TestNormalizeImageSamplingMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    string
		wantErr bool
	}{
		{name: "default empty", mode: "", want: domainrenderer.ImageSamplingModeLegacy},
		{name: "default keyword", mode: "default", want: domainrenderer.ImageSamplingModeLegacy},
		{name: "legacy", mode: "legacy", want: domainrenderer.ImageSamplingModeLegacy},
		{
			name: "adaptive",
			mode: domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
			want: domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		},
		{
			name: "splash experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1,
			want: domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1,
		},
		{
			name: "indexed origin downscale experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1,
			want: domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1,
		},
		{
			name: "indexed cmyk simple experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1,
			want: domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1,
		},
		{
			name: "indexed cmyk hybrid75 experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1,
			want: domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1,
		},
		{
			name: "indexed cmyk stdlib experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalIndexedCMYKStdlibV1,
			want: domainrenderer.ImageSamplingModeExperimentalIndexedCMYKStdlibV1,
		},
		{
			name: "rgb transparent edge experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
			want: domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
		},
		{
			name: "dct gray ignore icc experimental",
			mode: domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
			want: domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
		},
		{name: "invalid", mode: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeImageSamplingMode(tt.mode)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd, err := newRootCmd()
	require.NoError(t, err)
	require.NotNil(t, cmd)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "pdfrender version")

	cmd, err = newRootCmd()
	require.NoError(t, err)
	cmd.SetArgs([]string{})
	assert.Error(t, cmd.Execute())
}

func TestNewRootCmd_DebugImageSamplingFlag(t *testing.T) {
	cmd, err := newRootCmd()
	require.NoError(t, err)
	cmd.SetArgs([]string{
		"--version",
		"--debug-image-sampling",
		"--image-sampling-mode", domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	})
	require.NoError(t, cmd.Execute())
}

func TestProcessPDF(t *testing.T) {
	testPath := testPDFPath(t)
	outputDir := t.TempDir()

	err := processPDF(testPath, []int{1}, options{
		outputDir: outputDir,
		format:    "png",
		workers:   1,
		quiet:     true,
	})
	require.NoError(t, err)

	outputPath := filepath.Join(outputDir, "002-trivial-libre-office-writer_page_0001.png")
	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}

func TestNewRootCmd_RendersToPNG(t *testing.T) {
	outputDir := t.TempDir()
	input := testPDFPath(t)

	cmd, err := newRootCmd()
	require.NoError(t, err)
	cmd.SetArgs([]string{
		"--quiet",
		"--output", outputDir,
		"--pages", "1",
		input,
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	require.NoError(t, cmd.Execute())

	prefix := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	outputPath := filepath.Join(outputDir, prefix+"_page_0001.png")
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	_, err = png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
}

func TestResolveWorkerCount(t *testing.T) {
	tests := []struct {
		name              string
		workers           int
		maxInflightPixels int64
		maxPagePixels     int64
		expected          int
	}{
		{
			name:              "no limit keeps workers",
			workers:           8,
			maxInflightPixels: 0,
			maxPagePixels:     5000,
			expected:          8,
		},
		{
			name:              "reduces workers by budget",
			workers:           8,
			maxInflightPixels: 15000,
			maxPagePixels:     5000,
			expected:          3,
		},
		{
			name:              "minimum one worker",
			workers:           8,
			maxInflightPixels: 1000,
			maxPagePixels:     5000,
			expected:          1,
		},
		{
			name:              "defaults workers when invalid",
			workers:           0,
			maxInflightPixels: 0,
			maxPagePixels:     0,
			expected:          4,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := resolveWorkerCount(tc.workers, tc.maxInflightPixels, tc.maxPagePixels)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestResolveCacheSize(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		cacheSize int
		expected  int
	}{
		{
			name:      "explicit cache size",
			workers:   4,
			cacheSize: 20,
			expected:  20,
		},
		{
			name:      "auto by workers",
			workers:   4,
			cacheSize: 0,
			expected:  8,
		},
		{
			name:      "auto minimum bound",
			workers:   1,
			cacheSize: 0,
			expected:  4,
		},
		{
			name:      "auto maximum bound",
			workers:   20,
			cacheSize: 0,
			expected:  12,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := resolveCacheSize(tc.workers, tc.cacheSize)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestResolveCacheTTLSeconds(t *testing.T) {
	assert.Equal(t, 300*time.Second, resolveCacheTTLSeconds(0))
	assert.Equal(t, 300*time.Second, resolveCacheTTLSeconds(-5))
	assert.Equal(t, 42*time.Second, resolveCacheTTLSeconds(42))
}

func TestEstimatePagePixelsForSelection(t *testing.T) {
	testPath := testPDFPath(t)
	data, err := os.ReadFile(testPath)
	require.NoError(t, err)

	xrefTable := xref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	doc := entity.NewDocument(xrefTable)
	catalog, err := xrefTable.GetCatalog()
	if err == nil {
		doc.SetCatalog(catalog)
	}

	estimates, maxPixels, err := estimatePagePixelsForSelection(doc, []int{0}, 150, 1.0)
	require.NoError(t, err)
	require.Len(t, estimates, 1)
	assert.Greater(t, maxPixels, int64(0))
	assert.Greater(t, estimates[0].width, 0)
	assert.Greater(t, estimates[0].height, 0)
	assert.Greater(t, estimates[0].pixels, int64(0))
}

func testPDFPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "..", "..", "test", "testdata", "sample-files", "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")
}
