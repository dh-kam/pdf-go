// Package main provides the pdfrender CLI tool.
// pdfrender renders PDF pages to images.
package main

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
	"github.com/dh-kam/pdf-go/internal/infrastructure/renderer"
	appversion "github.com/dh-kam/pdf-go/internal/version"
)

const pngFormat = "png"

// options represents CLI options.
type options struct {
	outputDir          string
	format             string
	prefix             string
	password           string
	dpi                float64
	scale              float64
	workers            int
	cacheSize          int
	cacheTTLSec        int
	maxPagePixels      int64
	maxInflightPix     int64
	quiet              bool
	enableCache        bool
	debugImageSampling bool
	imageSamplingMode  string
	failOnPageError    bool
	backend            string
}

// renderStats represents rendering statistics.
type renderStats struct {
	outputFiles []string
	totalPages  int
	rendered    int
	failed      int
}

func main() {
	cmd, err := newRootCmd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() (*cobra.Command, error) {
	cfg := viper.New()
	cfg.SetEnvPrefix("PDFRENDER")
	cfg.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	cfg.AutomaticEnv()

	cmd := &cobra.Command{
		Use:           "pdfrender [flags] <pdf-file> [pdf-file ...]",
		Short:         "Render PDF pages to PNG images",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			showVersion, err := cmd.Flags().GetBool("version")
			if err != nil {
				return err
			}
			if showVersion {
				return nil
			}
			if len(args) == 0 {
				return fmt.Errorf("at least one PDF file is required")
			}
			return nil
		},
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			configFile := cfg.GetString("config")
			if configFile == "" {
				return nil
			}
			cfg.SetConfigFile(configFile)
			if err := cfg.ReadInConfig(); err != nil {
				return fmt.Errorf("read config file %q: %w", configFile, err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.GetBool("version") {
				cmd.Printf("pdfrender version %s\n", appversion.Current)
				return nil
			}

			pages, err := parsePageSpec(cfg.GetString("pages"))
			if err != nil {
				return fmt.Errorf("parse --pages: %w", err)
			}

			format, err := normalizeImageFormat(cfg.GetString("format"))
			if err != nil {
				return fmt.Errorf("invalid --format: %w", err)
			}
			imageSamplingMode, err := normalizeImageSamplingMode(cfg.GetString("image-sampling-mode"))
			if err != nil {
				return fmt.Errorf("invalid --image-sampling-mode: %w", err)
			}
			backend, err := normalizeBackend(cfg.GetString("backend"))
			if err != nil {
				return fmt.Errorf("invalid --backend: %w", err)
			}

			opts := options{
				outputDir:          cfg.GetString("output"),
				format:             format,
				prefix:             cfg.GetString("prefix"),
				password:           cfg.GetString("password"),
				dpi:                cfg.GetFloat64("dpi"),
				scale:              cfg.GetFloat64("scale"),
				workers:            cfg.GetInt("workers"),
				cacheSize:          cfg.GetInt("cache-size"),
				cacheTTLSec:        cfg.GetInt("cache-ttl-sec"),
				maxPagePixels:      cfg.GetInt64("max-page-pixels"),
				maxInflightPix:     cfg.GetInt64("max-inflight-pixels"),
				quiet:              cfg.GetBool("quiet"),
				enableCache:        cfg.GetBool("enable-cache"),
				debugImageSampling: cfg.GetBool("debug-image-sampling"),
				imageSamplingMode:  imageSamplingMode,
				failOnPageError:    cfg.GetBool("fail-on-page-error"),
				backend:            backend,
			}

			failed := 0
			for _, pdfFile := range args {
				if err := processPDF(pdfFile, pages, opts); err != nil {
					cmd.PrintErrf("Error processing %s: %v\n", pdfFile, err)
					failed++
				}
			}

			if failed > 0 {
				return fmt.Errorf("failed to process %d file(s)", failed)
			}
			return nil
		},
	}

	cmd.SetVersionTemplate("pdfrender version {{.Version}}\n")
	cmd.Version = appversion.Current

	flags := cmd.Flags()
	flags.BoolP("version", "v", false, "Show version information and exit")
	flags.String("config", "", "Path to configuration file")
	flags.StringP("output", "o", "", "Output directory (default: <filename>_rendered)")
	flags.StringP("pages", "p", "", "Render specific pages (e.g., 1, 1-5, 1,3,5)")
	flags.Float64P("dpi", "d", 72, "Render at specified DPI")
	flags.Float64P("scale", "s", 1.0, "Scale factor")
	flags.StringP("format", "f", pngFormat, "Output format (currently only png is supported)")
	flags.String("prefix", "", "Prefix for output files (default: input filename)")
	flags.IntP("workers", "w", 4, "Number of concurrent render workers")
	flags.Int("cache-size", 0, "Rendered-page cache size when --enable-cache is set (0: auto by workers)")
	flags.Int("cache-ttl-sec", 300, "Rendered-page cache TTL in seconds when --enable-cache is set")
	flags.Int64("max-page-pixels", 0, "Fail a page when estimated pixels exceed this limit (0: no limit)")
	flags.Int64("max-inflight-pixels", 0, "Auto-reduce workers so worst-case in-flight pixels stay under this limit (0: no limit)")
	flags.BoolP("quiet", "q", false, "Suppress progress output")
	flags.Bool("enable-cache", false, "Enable rendered-page cache during this command")
	flags.Bool("debug-image-sampling", false, "Print image sampling trace (doc/page/filter/CTM/sampler) to stderr")
	flags.String(
		"image-sampling-mode",
		domainrenderer.ImageSamplingModeLegacy,
		"Image sampling mode: legacy | adaptive-dct-iccbased-v1 | experimental-splash-scale-only-v1 | experimental-indexed-origin-downscale-phase-v1 (experimental)",
	)
	flags.String("password", "", "Password for encrypted PDF files")
	flags.Bool("fail-on-page-error", false, "Exit with an error when any page fails to render")
	flags.String(
		"backend",
		canvas.BackendSplash,
		"Rendering backend: splash (default) or image-canvas.",
	)

	if err := cfg.BindPFlags(flags); err != nil {
		return nil, fmt.Errorf("bind flags: %w", err)
	}

	return cmd, nil
}

// processPDF processes a single PDF file and renders pages to images.
func processPDF(filePath string, pages []int, opts options) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	xrefTable := xref.NewTable(data)
	if err := xrefTable.Parse(); err != nil {
		return fmt.Errorf("parse PDF: %w", err)
	}

	if xrefTable.IsEncrypted() {
		if err := xrefTable.ParseEncryptionDict(opts.password); err != nil {
			return fmt.Errorf("parse encryption (try using --password): %w", err)
		}
		if !xrefTable.IsAuthenticated() {
			return fmt.Errorf("invalid password or unsupported encryption")
		}
	}

	doc := entity.NewDocument(xrefTable)
	catalog, err := xrefTable.GetCatalog()
	if err == nil {
		doc.SetCatalog(catalog)
	}

	pageCount, err := doc.PageCount()
	if err != nil {
		return fmt.Errorf("get page count: %w", err)
	}

	pagesToRender, err := resolvePagesToRender(pages, pageCount)
	if err != nil {
		return err
	}

	outputDir := resolveOutputDir(filePath, opts.outputDir)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	pageEstimates, maxEstimatedPixels, err := estimatePagePixelsForSelection(doc, pagesToRender, opts.dpi, opts.scale)
	if err != nil {
		return fmt.Errorf("estimate page pixels: %w", err)
	}

	effectiveWorkers := resolveWorkerCount(opts.workers, opts.maxInflightPix, maxEstimatedPixels)
	effectiveCacheSize := resolveCacheSize(effectiveWorkers, opts.cacheSize)
	effectiveCacheTTL := resolveCacheTTLSeconds(opts.cacheTTLSec)
	if !opts.quiet && opts.maxInflightPix > 0 && effectiveWorkers != normalizeWorkers(opts.workers) {
		fmt.Printf("Adjusting workers %d -> %d due to --max-inflight-pixels=%d (max page pixels=%d)\n",
			normalizeWorkers(opts.workers),
			effectiveWorkers,
			opts.maxInflightPix,
			maxEstimatedPixels,
		)
	}
	if !opts.quiet && opts.enableCache {
		fmt.Printf("Cache config: size=%d, ttl=%s\n", effectiveCacheSize, effectiveCacheTTL)
	}

	rendererOptions := renderer.RendererOptions{
		MaxWorkers: effectiveWorkers,
		CacheSize:  effectiveCacheSize,
		CacheTTL:   effectiveCacheTTL,
		Backend:    opts.backend,
	}
	if rendererOptions.MaxWorkers <= 0 {
		rendererOptions.MaxWorkers = 4
	}

	r := renderer.NewConcurrentRenderer(rendererOptions)

	ctx := context.Background()
	stats := renderStats{
		totalPages:  len(pagesToRender),
		outputFiles: make([]string, 0, len(pagesToRender)),
	}

	renderOpts := domainrenderer.RenderOptions{
		DPI:                opts.dpi,
		Scale:              opts.scale,
		EnableCache:        opts.enableCache,
		DebugImageSampling: opts.debugImageSampling,
		ImageSamplingMode:  opts.imageSamplingMode,
		BackgroundColor:    nil,
	}

	for i, pageIndex := range pagesToRender {
		if !opts.quiet {
			fmt.Printf("Rendering page %d/%d...\n", i+1, len(pagesToRender))
		}
		if opts.maxPagePixels > 0 {
			if estimate, ok := pageEstimates[pageIndex]; ok && estimate.pixels > opts.maxPagePixels {
				fmt.Fprintf(
					os.Stderr,
					"Warning: Skipping page %d: estimated pixels %d (%dx%d) exceed limit %d\n",
					pageIndex+1,
					estimate.pixels,
					estimate.width,
					estimate.height,
					opts.maxPagePixels,
				)
				stats.failed++
				continue
			}
		}

		page, err := doc.GetPage(pageIndex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not get page %d: %v\n", pageIndex+1, err)
			stats.failed++
			continue
		}

		img, err := r.RenderPage(ctx, page, renderOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not render page %d: %v\n", pageIndex+1, err)
			stats.failed++
			continue
		}

		prefix := opts.prefix
		if prefix == "" {
			prefix = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		}

		outputFileName := fmt.Sprintf("%s_page_%04d.%s", prefix, pageIndex+1, pngFormat)
		outputPath := filepath.Join(outputDir, outputFileName)

		if err := saveImage(img, outputPath, opts.format); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not save page %d: %v\n", pageIndex+1, err)
			stats.failed++
			continue
		}

		stats.rendered++
		stats.outputFiles = append(stats.outputFiles, outputPath)

		if !opts.quiet {
			fmt.Printf("  Saved: %s\n", outputPath)
		}
	}

	if !opts.quiet {
		fmt.Printf("\nRendering complete:\n")
		fmt.Printf("  Total pages: %d\n", stats.totalPages)
		fmt.Printf("  Rendered: %d\n", stats.rendered)
		if stats.failed > 0 {
			fmt.Printf("  Failed: %d\n", stats.failed)
		}
		fmt.Printf("  Output directory: %s\n", outputDir)
	}

	if stats.failed > 0 && stats.rendered == 0 {
		return fmt.Errorf("all pages failed to render")
	}
	if opts.failOnPageError && stats.failed > 0 {
		return fmt.Errorf("%d page(s) failed to render", stats.failed)
	}

	return nil
}

func resolvePagesToRender(pages []int, pageCount int) ([]int, error) {
	if len(pages) == 0 {
		pagesToRender := make([]int, pageCount)
		for i := 0; i < pageCount; i++ {
			pagesToRender[i] = i
		}
		return pagesToRender, nil
	}

	pagesToRender := make([]int, 0, len(pages))
	for _, p := range pages {
		if p < 1 || p > pageCount {
			return nil, fmt.Errorf("page %d out of range (1-%d)", p, pageCount)
		}
		pagesToRender = append(pagesToRender, p-1)
	}
	return pagesToRender, nil
}

func resolveOutputDir(filePath string, outputDir string) string {
	if outputDir != "" {
		return outputDir
	}

	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	return baseName + "_rendered"
}

func normalizeImageFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", pngFormat, "image/png":
		return pngFormat, nil
	default:
		return "", fmt.Errorf("unsupported format %q (only %s is supported)", format, pngFormat)
	}
}

func normalizeBackend(backend string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", canvas.BackendSplash:
		return canvas.BackendSplash, nil
	case canvas.BackendImageCanvas:
		return canvas.BackendImageCanvas, nil
	default:
		return "", fmt.Errorf(
			"unsupported backend %q (valid: %s, %s)",
			backend,
			canvas.BackendImageCanvas,
			canvas.BackendSplash,
		)
	}
}

func normalizeImageSamplingMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "", "default", domainrenderer.ImageSamplingModeLegacy:
		return domainrenderer.ImageSamplingModeLegacy, nil
	case domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1:
		return domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1, nil
	case domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1:
		return domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1, nil
	case domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1:
		return domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1, nil
	case domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1:
		return domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1, nil
	case domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1:
		return domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1, nil
	case domainrenderer.ImageSamplingModeExperimentalIndexedCMYKStdlibV1:
		return domainrenderer.ImageSamplingModeExperimentalIndexedCMYKStdlibV1, nil
	case domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1:
		return domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1, nil
	case domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1:
		return domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1, nil
	default:
		return "", fmt.Errorf(
			"unsupported mode %q (supported: %s, %s, %s, %s, %s, %s, %s, %s, %s)",
			mode,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
			domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1,
			domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1,
			domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1,
			domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1,
			domainrenderer.ImageSamplingModeExperimentalIndexedCMYKStdlibV1,
			domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
			domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
		)
	}
}

type pagePixelEstimate struct {
	width  int
	height int
	pixels int64
}

func estimatePagePixelsForSelection(
	doc *entity.Document,
	pageIndexes []int,
	dpi float64,
	scale float64,
) (map[int]pagePixelEstimate, int64, error) {
	out := make(map[int]pagePixelEstimate, len(pageIndexes))
	var maxPixels int64

	for _, pageIndex := range pageIndexes {
		page, err := doc.GetPage(pageIndex)
		if err != nil {
			return nil, 0, fmt.Errorf("get page %d: %w", pageIndex+1, err)
		}
		width, height, pixels, err := estimatePagePixels(page, dpi, scale)
		if err != nil {
			return nil, 0, fmt.Errorf("estimate page %d: %w", pageIndex+1, err)
		}
		out[pageIndex] = pagePixelEstimate{
			width:  width,
			height: height,
			pixels: pixels,
		}
		if pixels > maxPixels {
			maxPixels = pixels
		}
	}

	return out, maxPixels, nil
}

func estimatePagePixels(page *entity.Page, dpi float64, scale float64) (int, int, int64, error) {
	if page == nil {
		return 0, 0, 0, fmt.Errorf("nil page")
	}
	if dpi <= 0 {
		dpi = 72.0
	}
	if scale <= 0 {
		scale = 1.0
	}

	pageBox := page.CropBox()
	xMin := math.Min(pageBox[0], pageBox[2])
	xMax := math.Max(pageBox[0], pageBox[2])
	yMin := math.Min(pageBox[1], pageBox[3])
	yMax := math.Max(pageBox[1], pageBox[3])
	pageWidth := xMax - xMin
	pageHeight := yMax - yMin
	rotation := normalizePageRotation(page.Rotate())
	if rotation == 90 || rotation == 270 {
		pageWidth, pageHeight = pageHeight, pageWidth
	}

	width := pointsToPixels(pageWidth, dpi, scale)
	height := pointsToPixels(pageHeight, dpi, scale)
	if width <= 0 || height <= 0 {
		return 0, 0, 0, fmt.Errorf("invalid image size: %dx%d", width, height)
	}

	if width > math.MaxInt64/height {
		return 0, 0, 0, fmt.Errorf("pixel count overflow: %dx%d", width, height)
	}

	return width, height, int64(width * height), nil
}

func pointsToPixels(points, dpi, scale float64) int {
	if points < 0 {
		points = -points
	}
	pixels := points * (dpi / 72.0) * scale
	roundedUp := int(math.Ceil(pixels - 1e-9))
	if roundedUp < 1 {
		return 1
	}
	return roundedUp
}

func normalizePageRotation(rotation int) int {
	normalized := rotation % 360
	if normalized < 0 {
		normalized += 360
	}
	switch normalized {
	case 90, 180, 270:
		return normalized
	default:
		return 0
	}
}

func normalizeWorkers(workers int) int {
	if workers <= 0 {
		return 4
	}
	return workers
}

func resolveWorkerCount(workers int, maxInflightPixels, maxPagePixels int64) int {
	effectiveWorkers := normalizeWorkers(workers)
	if maxInflightPixels <= 0 || maxPagePixels <= 0 {
		return effectiveWorkers
	}

	budgetWorkers := int(maxInflightPixels / maxPagePixels)
	if budgetWorkers < 1 {
		budgetWorkers = 1
	}
	if budgetWorkers < effectiveWorkers {
		return budgetWorkers
	}
	return effectiveWorkers
}

func resolveCacheSize(workers, cacheSize int) int {
	if cacheSize > 0 {
		return cacheSize
	}

	// Auto-tuned defaults from profiling:
	// keep cache modest to avoid memory growth while preserving reuse.
	effectiveWorkers := normalizeWorkers(workers)
	autoSize := effectiveWorkers * 2
	if autoSize < 4 {
		autoSize = 4
	}
	if autoSize > 12 {
		autoSize = 12
	}
	return autoSize
}

func resolveCacheTTLSeconds(cacheTTLSec int) time.Duration {
	if cacheTTLSec <= 0 {
		return 300 * time.Second
	}
	return time.Duration(cacheTTLSec) * time.Second
}

// saveImage saves an image to a file.
func saveImage(img image.Image, path string, format string) (retErr error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	switch strings.ToLower(strings.TrimSpace(format)) {
	case pngFormat:
		// pdftoppm/libpng emits IHDR + pHYs + IDAT(BestCompression, 78 DA) + IEND.
		// Match that layout so the splash backend's parity gate
		// (test/integration/splash) can compare SHA256 byte-for-byte. Pixel
		// equality alone is not enough — the test compares raw PNG bytes after
		// stripping the tIME chunk.
		retErr = encodePNGCanonical(file, img)
	default:
		retErr = fmt.Errorf("unsupported image format: %s", format)
	}

	return retErr
}

// parsePageSpec parses a page specification string.
// Supports: single pages (1), ranges (1-5), and comma-separated lists (1,3,5).
func parsePageSpec(spec string) ([]int, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}

	parts := strings.Split(spec, ",")
	pages := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid page range: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start page in range %s: %w", part, err)
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end page in range %s: %w", part, err)
			}

			if start < 1 || end < 1 {
				return nil, fmt.Errorf("page numbers must be >= 1 in range %s", part)
			}

			if start > end {
				return nil, fmt.Errorf("start page must be <= end page in range %s", part)
			}

			for i := start; i <= end; i++ {
				pages = append(pages, i)
			}
			continue
		}

		page, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid page number: %s", part)
		}
		if page < 1 {
			return nil, fmt.Errorf("page numbers must be >= 1")
		}
		pages = append(pages, page)
	}

	return pages, nil
}
