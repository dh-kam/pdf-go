package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type config struct {
	repoRoot             string
	scanRoot             string
	outDir               string
	pdftoppm             string
	pdfrender            string
	pdfrenderBuildTags   string
	backend              string
	imageSamplingMode    string
	dpi                  int
	workers              int
	timeout              time.Duration
	tileSize             int
	skipCompressedCopies bool
	pureGoJPEG           bool
	keepImages           bool
	badPixelsOut         string
	badRegionsOut        string
	badPixelLimitPerPage int
}

const defaultPDFRenderBuildTags = ""

type pdfJob struct {
	index int
	path  string
	rel   string
	slug  string
}

type pageRow struct {
	PDF           string
	PDFRel        string
	Page          int
	PopplerPNG    string
	OursPNG       string
	DiffPNG       string
	PopplerRel    string
	OursRel       string
	DiffRel       string
	Width         int
	Height        int
	MatchedPixels int64
	TotalPixels   int64
	ExactPercent  float64
	Exact100      bool
	Error         string
}

type summary struct {
	Documents      int
	Rows           int
	ExactPages     int
	ErrorRows      int
	MatchedPixels  int64
	TotalPixels    int64
	OverallPercent float64
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	timeoutSec := flag.Int("timeout-sec", 0, "per-document render timeout in seconds; <=0 disables timeout")
	flag.StringVar(&cfg.repoRoot, "repo-root", ".", "repository root")
	flag.StringVar(&cfg.scanRoot, "scan-root", "test/testdata/compare/pdfs", "comma-separated PDF scan roots")
	flag.StringVar(&cfg.outDir, "out", "tmp/full_render_compare", "output directory")
	flag.StringVar(&cfg.pdftoppm, "pdftoppm", "pdftoppm", "pdftoppm executable")
	flag.StringVar(&cfg.pdfrender, "pdfrender", "", "pdfrender executable; built into output tools directory when empty")
	flag.StringVar(&cfg.pdfrenderBuildTags, "pdfrender-build-tags", defaultPDFRenderBuildTags, "optional Go build tags for auto-built pdfrender")
	flag.StringVar(&cfg.backend, "backend", "splash", "pdfrender backend: splash or image-canvas")
	flag.StringVar(&cfg.imageSamplingMode, "image-sampling-mode", "legacy", "pdfrender image sampling mode")
	flag.IntVar(&cfg.dpi, "dpi", 150, "render DPI")
	flag.IntVar(&cfg.workers, "workers", 4, "pdfrender workers")
	flag.IntVar(&cfg.tileSize, "tile-size", 48, "diff marker tile size in pixels")
	flag.BoolVar(&cfg.skipCompressedCopies, "skip-compressed-duplicates", false, "skip GeoTopo-komprimiert duplicate fixtures")
	flag.BoolVar(&cfg.pureGoJPEG, "pure-go-jpeg", false, "run ours with pure-Go JPEG DCTDecode path by enabling djpeg-go and disabling ImageMagick JPEG fallback")
	flag.BoolVar(&cfg.keepImages, "keep-images", true, "keep rendered poppler/ours/diff PNGs in the output directory")
	flag.StringVar(&cfg.badPixelsOut, "bad-pixels-out", "", "optional CSV path for sampled mismatching pixels; relative paths are under -out")
	flag.StringVar(&cfg.badRegionsOut, "bad-regions-out", "", "optional CSV path for connected mismatching pixel regions; relative paths are under -out")
	flag.IntVar(&cfg.badPixelLimitPerPage, "bad-pixel-limit-per-page", 2000, "maximum bad-pixel samples per page written to -bad-pixels-out")
	flag.Parse()
	cfg.timeout = time.Duration(*timeoutSec) * time.Second
	return cfg
}

func run(cfg config) error {
	var err error
	cfg.repoRoot, err = filepath.Abs(cfg.repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	cfg.outDir, err = filepath.Abs(cfg.outDir)
	if err != nil {
		return fmt.Errorf("resolve output dir: %w", err)
	}
	if err := validateOutputDirectory(cfg.repoRoot, cfg.outDir, cfg.scanRoot); err != nil {
		return err
	}
	if cfg.tileSize <= 0 {
		cfg.tileSize = 48
	}
	if cfg.dpi <= 0 {
		return fmt.Errorf("dpi must be positive")
	}
	if cfg.workers <= 0 {
		cfg.workers = 1
	}
	if cfg.badPixelLimitPerPage < 0 {
		cfg.badPixelLimitPerPage = 0
	}
	if err := os.RemoveAll(cfg.outDir); err != nil {
		return fmt.Errorf("clean output dir: %w", err)
	}
	for _, dir := range []string{"poppler", "ours", "diff", "logs", "tools"} {
		if err := os.MkdirAll(filepath.Join(cfg.outDir, dir), 0o755); err != nil {
			return fmt.Errorf("create output subdir %s: %w", dir, err)
		}
	}
	if cfg.pdfrender == "" {
		cfg.pdfrender = filepath.Join(cfg.outDir, "tools", exeName("pdfrender"))
		if err := buildPDFRender(cfg); err != nil {
			return err
		}
	}
	jobs, err := scanPDFs(cfg)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return fmt.Errorf("no PDF files found under %q", cfg.scanRoot)
	}
	diag, err := newDiagnosticsWriter(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Comparing %d PDF files at %d DPI with backend=%s pure-go-jpeg=%t\n", len(jobs), cfg.dpi, cfg.backend, cfg.pureGoJPEG)

	rows := make([]pageRow, 0)
	for _, job := range jobs {
		fmt.Printf("[%d/%d] %s\n", job.index+1, len(jobs), job.rel)
		docRows := compareDocument(cfg, job, diag)
		rows = append(rows, docRows...)
	}
	if diag != nil {
		if err := diag.Close(); err != nil {
			return err
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].PDFRel != rows[j].PDFRel {
			return rows[i].PDFRel < rows[j].PDFRel
		}
		return rows[i].Page < rows[j].Page
	})
	sum := summarize(rows, len(jobs))
	if err := writeCSV(filepath.Join(cfg.outDir, "report.csv"), rows); err != nil {
		return err
	}
	if err := writeHTML(filepath.Join(cfg.outDir, "index.html"), cfg, sum, rows); err != nil {
		return err
	}
	fmt.Printf("HTML report: %s\n", filepath.Join(cfg.outDir, "index.html"))
	fmt.Printf("CSV report:  %s\n", filepath.Join(cfg.outDir, "report.csv"))
	fmt.Printf("Exact100:    %d/%d pages (%.2f%%)\n", sum.ExactPages, sum.Rows, percent(int64(sum.ExactPages), int64(sum.Rows)))
	return nil
}

func buildPDFRender(cfg config) error {
	args := []string{"build", "-o", cfg.pdfrender}
	if strings.TrimSpace(cfg.pdfrenderBuildTags) != "" {
		args = append(args, "-tags", cfg.pdfrenderBuildTags)
	}
	args = append(args, "./cmd/pdfrender")
	var stderr bytes.Buffer
	cmd := exec.Command("go", args...)
	cmd.Dir = cfg.repoRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"PDF_FREETYPE_GO=1",
	)
	cmd.Stderr = &stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build pdfrender: %w\n%s", err, stderr.String())
	}
	return nil
}

func validateOutputDirectory(repoRoot, outDir, scanRoot string) error {
	repoRoot = filepath.Clean(repoRoot)
	outDir = filepath.Clean(outDir)
	if isFilesystemRoot(outDir) {
		return fmt.Errorf("refusing to use filesystem root as output directory: %q", outDir)
	}
	if anyPathContainsOrEqual(pathAliases(outDir), pathAliases(repoRoot)) {
		return fmt.Errorf("refusing to use repository root or ancestor as output directory: %q contains %q", outDir, repoRoot)
	}
	for _, root := range resolvedScanRoots(repoRoot, scanRoot) {
		if anyPathContainsOrEqual(pathAliases(outDir), pathAliases(root)) {
			return fmt.Errorf("refusing to use scan root or ancestor as output directory: %q contains scan root %q", outDir, root)
		}
	}
	return nil
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	return filepath.IsAbs(clean) && filepath.Dir(clean) == clean
}

func resolvedScanRoots(repoRoot, scanRoot string) []string {
	roots := strings.Split(scanRoot, ",")
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(repoRoot, root)
		}
		out = append(out, filepath.Clean(root))
	}
	return out
}

func pathContainsOrEqual(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func anyPathContainsOrEqual(parents, children []string) bool {
	for _, parent := range parents {
		for _, child := range children {
			if pathContainsOrEqual(parent, child) {
				return true
			}
		}
	}
	return false
}

func pathAliases(path string) []string {
	clean := filepath.Clean(path)
	aliases := []string{clean}
	if resolved, ok := evalExistingPathPrefix(clean); ok && resolved != clean {
		aliases = append(aliases, resolved)
	}
	return aliases
}

func evalExistingPathPrefix(path string) (string, bool) {
	clean := filepath.Clean(path)
	suffix := []string{}
	for cur := clean; ; cur = filepath.Dir(cur) {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			parts := append([]string{resolved}, suffix...)
			return filepath.Clean(filepath.Join(parts...)), true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		suffix = append([]string{filepath.Base(cur)}, suffix...)
	}
}

func scanPDFs(cfg config) ([]pdfJob, error) {
	roots := strings.Split(cfg.scanRoot, ",")
	seen := map[string]bool{}
	var paths []string
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(cfg.repoRoot, root)
		}
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if shouldSkipDir(name, path, cfg) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(filepath.Ext(name), ".pdf") {
				return nil
			}
			if cfg.skipCompressedCopies && strings.Contains(strings.ToLower(name), "komprimiert") {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if seen[abs] {
				return nil
			}
			seen[abs] = true
			paths = append(paths, abs)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("scan %s: %w", root, err)
		}
	}
	sort.Strings(paths)
	jobs := make([]pdfJob, 0, len(paths))
	for i, path := range paths {
		rel, err := filepath.Rel(cfg.repoRoot, path)
		if err != nil {
			rel = path
		}
		jobs = append(jobs, pdfJob{
			index: i,
			path:  path,
			rel:   filepath.ToSlash(rel),
			slug:  fmt.Sprintf("%04d_%s", i+1, slugify(rel)),
		})
	}
	return jobs, nil
}

func shouldSkipDir(name string, path string, cfg config) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	switch name {
	case "bin", "build", "tmp", "__pycache__":
		return true
	}
	abs, err := filepath.Abs(path)
	if err == nil && pathContainsOrEqual(cfg.outDir, abs) {
		return true
	}
	return false
}

func compareDocument(cfg config, job pdfJob, diag *diagnosticsWriter) []pageRow {
	popplerDir := filepath.Join(cfg.outDir, "poppler", job.slug)
	oursDir := filepath.Join(cfg.outDir, "ours", job.slug)
	diffDir := filepath.Join(cfg.outDir, "diff", job.slug)
	logDir := filepath.Join(cfg.outDir, "logs", job.slug)
	for _, dir := range []string{popplerDir, oursDir, diffDir, logDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return []pageRow{errorRow(job, 0, fmt.Sprintf("create dir: %v", err))}
		}
	}
	password := passwordFor(job.rel)
	popplerErr := renderPoppler(cfg, job.path, popplerDir, logDir, password)
	oursErr := renderOurs(cfg, job.path, oursDir, logDir, password)
	popplerPages := collectPages(popplerDir, regexp.MustCompile(`^page-(\d+)\.png$`))
	oursPages := collectPages(oursDir, regexp.MustCompile(`^page_page_(\d+)\.png$`))
	maxPage := maxPageNumber(popplerPages, oursPages)
	if maxPage == 0 {
		errText := strings.TrimSpace(strings.Join(nonEmpty(popplerErr, oursErr), "; "))
		if isZeroPageRenderFailure(errText) {
			return nil
		}
		if errText == "" {
			errText = "no rendered pages"
		}
		return []pageRow{errorRow(job, 0, errText)}
	}
	rows := make([]pageRow, 0, maxPage)
	for page := 1; page <= maxPage; page++ {
		row := pageRow{PDF: filepath.Base(job.path), PDFRel: job.rel, Page: page}
		if popplerErr != "" {
			row.Error = appendError(row.Error, "poppler: "+popplerErr)
		}
		if oursErr != "" {
			row.Error = appendError(row.Error, "ours: "+oursErr)
		}
		popplerPNG := popplerPages[page]
		oursPNG := oursPages[page]
		if popplerPNG == "" {
			row.Error = appendError(row.Error, "missing poppler page")
		}
		if oursPNG == "" {
			row.Error = appendError(row.Error, "missing ours page")
		}
		row.PopplerPNG = popplerPNG
		row.OursPNG = oursPNG
		row.PopplerRel = relToOut(cfg, popplerPNG)
		row.OursRel = relToOut(cfg, oursPNG)
		if popplerPNG != "" && oursPNG != "" {
			diffPNG := filepath.Join(diffDir, fmt.Sprintf("page-%04d-xor.png", page))
			stats, pageDiag, err := comparePNGs(popplerPNG, oursPNG, diffPNG, compareOptions{
				tileSize:      cfg.tileSize,
				writeDiff:     cfg.keepImages,
				badPixelLimit: cfg.badPixelLimitPerPage,
				needPixels:    diag != nil && diag.HasPixelOutput(),
				needRegions:   diag != nil && diag.HasRegionOutput(),
			})
			if err != nil {
				row.Error = appendError(row.Error, err.Error())
			} else {
				if diag != nil {
					diag.Write(job.rel, page, pageDiag)
				}
				if cfg.keepImages {
					row.DiffPNG = diffPNG
					row.DiffRel = relToOut(cfg, diffPNG)
				}
				row.Width = stats.width
				row.Height = stats.height
				row.MatchedPixels = stats.matchedPixels
				row.TotalPixels = stats.totalPixels
				row.ExactPercent = stats.exactPercent
				row.Exact100 = stats.matchedPixels == stats.totalPixels && stats.totalPixels > 0
			}
			if !cfg.keepImages {
				row.PopplerPNG = ""
				row.OursPNG = ""
				row.PopplerRel = ""
				row.OursRel = ""
			}
		}
		rows = append(rows, row)
	}
	if !cfg.keepImages {
		_ = os.RemoveAll(popplerDir)
		_ = os.RemoveAll(oursDir)
		_ = os.RemoveAll(diffDir)
	}
	return rows
}

func isZeroPageRenderFailure(errText string) bool {
	lower := strings.ToLower(errText)
	return strings.Contains(lower, "invalid page count 0") ||
		strings.Contains(lower, "the first page (1) can not be after the last page (0)")
}

func renderPoppler(cfg config, pdfPath, outDir, logDir, password string) string {
	args := []string{"-r", strconv.Itoa(cfg.dpi), "-png", "-aa", "yes", "-aaVector", "yes", "-cropbox"}
	if password != "" {
		args = append(args, "-upw", password)
	}
	args = append(args, pdfPath, filepath.Join(outDir, "page"))
	return runLogged(cfg, logDir, "poppler", cfg.pdftoppm, args...)
}

func renderOurs(cfg config, pdfPath, outDir, logDir, password string) string {
	args := []string{
		"-q",
		"-d", strconv.Itoa(cfg.dpi),
		"-o", outDir,
		"--prefix", "page",
		"--backend", cfg.backend,
		"--workers", strconv.Itoa(cfg.workers),
		"--image-sampling-mode", cfg.imageSamplingMode,
	}
	if password != "" {
		args = append(args, "--password", password)
	}
	args = append(args, pdfPath)
	return runLogged(cfg, logDir, "ours", cfg.pdfrender, args...)
}

func runLogged(cfg config, logDir, name, command string, args ...string) string {
	ctx := context.Background()
	cancel := func() {}
	if cfg.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cfg.repoRoot
	if name == "ours" {
		cmd.Env = append(os.Environ(), "PDF_FREETYPE_GO=1")
		if cfg.pureGoJPEG {
			cmd.Env = append(cmd.Env, "GO_PDF_ENABLE_DJPEG_GO=1", "GO_PDF_DISABLE_IMAGEMAGICK_JPEG=1")
		}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	_ = os.WriteFile(filepath.Join(logDir, name+".stdout.log"), stdout.Bytes(), 0o644)
	_ = os.WriteFile(filepath.Join(logDir, name+".stderr.log"), stderr.Bytes(), 0o644)
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("timeout after %s", cfg.timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return oneLine(msg)
	}
	return ""
}

func collectPages(dir string, re *regexp.Regexp) map[int]string {
	out := map[int]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(entry.Name())
		if len(matches) != 2 {
			continue
		}
		page, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		out[page] = filepath.Join(dir, entry.Name())
	}
	return out
}

type compareStats struct {
	width         int
	height        int
	matchedPixels int64
	totalPixels   int64
	exactPercent  float64
}

type compareOptions struct {
	tileSize      int
	writeDiff     bool
	badPixelLimit int
	needPixels    bool
	needRegions   bool
}

type mismatchPixel struct {
	x          int
	y          int
	popplerR   uint8
	popplerG   uint8
	popplerB   uint8
	oursR      uint8
	oursG      uint8
	oursB      uint8
	deltaR     int
	deltaG     int
	deltaB     int
	deltaMax   int
	regionID   int
	sampleRank int
	flat       int
}

type mismatchRegion struct {
	id          int
	x0          int
	y0          int
	x1          int
	y1          int
	badPixels   int
	centroidX   float64
	centroidY   float64
	maxDelta    int
	nearSampleX int
	nearSampleY int
	hasSample   bool
}

type compareDiagnostics struct {
	pixels  []mismatchPixel
	regions []mismatchRegion
}

func comparePNGs(popplerPath, oursPath, diffPath string, opts compareOptions) (compareStats, compareDiagnostics, error) {
	popplerImg, err := decodePNG(popplerPath)
	if err != nil {
		return compareStats{}, compareDiagnostics{}, fmt.Errorf("decode poppler png: %w", err)
	}
	oursImg, err := decodePNG(oursPath)
	if err != nil {
		return compareStats{}, compareDiagnostics{}, fmt.Errorf("decode ours png: %w", err)
	}
	pb := popplerImg.Bounds()
	ob := oursImg.Bounds()
	width := maxInt(pb.Dx(), ob.Dx())
	height := maxInt(pb.Dy(), ob.Dy())
	if width <= 0 || height <= 0 {
		return compareStats{}, compareDiagnostics{}, fmt.Errorf("invalid image dimensions")
	}
	tileSize := opts.tileSize
	if tileSize <= 0 {
		tileSize = 48
	}
	var diff *image.RGBA
	var mismatchTiles []bool
	tilesX := 0
	tilesY := 0
	if opts.writeDiff {
		diff = image.NewRGBA(image.Rect(0, 0, width, height))
		tilesX = (width + tileSize - 1) / tileSize
		tilesY = (height + tileSize - 1) / tileSize
		mismatchTiles = make([]bool, tilesX*tilesY)
	}
	var mismatchMap []bool
	var deltaMaxMap []uint8
	if opts.needRegions {
		size := width * height
		mismatchMap = make([]bool, size)
		deltaMaxMap = make([]uint8, size)
	}
	sampleLimit := opts.badPixelLimit
	if !opts.needPixels {
		sampleLimit = 0
	}
	samples := make([]mismatchPixel, 0, minInt(sampleLimit, 1024))
	var matched int64
	total := int64(width) * int64(height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pr, pg, pbv, pok := rgbAt(popplerImg, x, y)
			or, og, obv, ook := rgbAt(oursImg, x, y)
			if pok && ook && pr == or && pg == og && pbv == obv {
				matched++
				if opts.writeDiff {
					diff.SetRGBA(x, y, color.RGBA{245, 245, 245, 255})
				}
				continue
			}
			dr := absInt(int(pr) - int(or))
			dg := absInt(int(pg) - int(og))
			db := absInt(int(pbv) - int(obv))
			deltaMax := maxInt(maxInt(dr, dg), db)
			flat := y*width + x
			if opts.needRegions {
				mismatchMap[flat] = true
				deltaMaxMap[flat] = uint8(deltaMax)
			}
			if sampleLimit > 0 && len(samples) < sampleLimit {
				samples = append(samples, mismatchPixel{
					x: x, y: y,
					popplerR: pr, popplerG: pg, popplerB: pbv,
					oursR: or, oursG: og, oursB: obv,
					deltaR: dr, deltaG: dg, deltaB: db, deltaMax: deltaMax,
					flat: flat,
				})
			}
			if opts.writeDiff {
				mismatchTiles[(y/tileSize)*tilesX+x/tileSize] = true
				if pok && ook {
					diff.SetRGBA(x, y, color.RGBA{amplifyXOR(pr ^ or), amplifyXOR(pg ^ og), amplifyXOR(pbv ^ obv), 255})
				} else {
					diff.SetRGBA(x, y, color.RGBA{255, 0, 255, 255})
				}
			}
		}
	}
	var regions []mismatchRegion
	if opts.needRegions {
		regions, samples = buildMismatchRegions(mismatchMap, deltaMaxMap, width, height, samples)
	}
	for i := range samples {
		samples[i].sampleRank = i + 1
	}
	if opts.writeDiff {
		drawTileComponentEllipses(diff, mismatchTiles, tilesX, tilesY, tileSize, width, height)
		if err := os.MkdirAll(filepath.Dir(diffPath), 0o755); err != nil {
			return compareStats{}, compareDiagnostics{}, err
		}
		f, err := os.Create(diffPath)
		if err != nil {
			return compareStats{}, compareDiagnostics{}, err
		}
		if err := png.Encode(f, diff); err != nil {
			_ = f.Close()
			return compareStats{}, compareDiagnostics{}, err
		}
		if err := f.Close(); err != nil {
			return compareStats{}, compareDiagnostics{}, err
		}
	}
	return compareStats{
		width:         width,
		height:        height,
		matchedPixels: matched,
		totalPixels:   total,
		exactPercent:  percent(matched, total),
	}, compareDiagnostics{pixels: samples, regions: regions}, nil
}

func buildMismatchRegions(mismatchMap []bool, deltaMaxMap []uint8, width, height int, samples []mismatchPixel) ([]mismatchRegion, []mismatchPixel) {
	if width <= 0 || height <= 0 || len(mismatchMap) == 0 {
		return nil, samples
	}
	sampleByFlat := make(map[int]int, len(samples))
	for i := range samples {
		sampleByFlat[samples[i].flat] = i
	}
	visited := make([]bool, len(mismatchMap))
	queue := make([]int, 0, 1024)
	regions := make([]mismatchRegion, 0)
	for flat, mismatch := range mismatchMap {
		if !mismatch || visited[flat] {
			continue
		}
		region := mismatchRegion{
			id: len(regions) + 1,
			x0: flat % width,
			y0: flat / width,
			x1: flat % width,
			y1: flat / width,
		}
		var sumX, sumY int64
		visited[flat] = true
		queue = append(queue[:0], flat)
		for head := 0; head < len(queue); head++ {
			cur := queue[head]
			x := cur % width
			y := cur / width
			region.badPixels++
			sumX += int64(x)
			sumY += int64(y)
			if x < region.x0 {
				region.x0 = x
			}
			if x > region.x1 {
				region.x1 = x
			}
			if y < region.y0 {
				region.y0 = y
			}
			if y > region.y1 {
				region.y1 = y
			}
			if cur < len(deltaMaxMap) && int(deltaMaxMap[cur]) > region.maxDelta {
				region.maxDelta = int(deltaMaxMap[cur])
			}
			if sampleIndex, ok := sampleByFlat[cur]; ok {
				samples[sampleIndex].regionID = region.id
				if !region.hasSample {
					region.nearSampleX = x
					region.nearSampleY = y
					region.hasSample = true
				}
			}
			for _, next := range neighborPixels(cur, x, y, width, height) {
				if mismatchMap[next] && !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		region.centroidX = float64(sumX) / float64(region.badPixels)
		region.centroidY = float64(sumY) / float64(region.badPixels)
		regions = append(regions, region)
	}
	return regions, samples
}

func neighborPixels(flat, x, y, width, height int) []int {
	out := make([]int, 0, 4)
	if x > 0 {
		out = append(out, flat-1)
	}
	if x+1 < width {
		out = append(out, flat+1)
	}
	if y > 0 {
		out = append(out, flat-width)
	}
	if y+1 < height {
		out = append(out, flat+width)
	}
	return out
}

type diagnosticsWriter struct {
	pixelFile    *os.File
	regionFile   *os.File
	pixelWriter  *csv.Writer
	regionWriter *csv.Writer
	err          error
	closed       bool
}

func newDiagnosticsWriter(cfg config) (*diagnosticsWriter, error) {
	pixelPath := resolveDiagnosticsPath(cfg.outDir, cfg.badPixelsOut)
	regionPath := resolveDiagnosticsPath(cfg.outDir, cfg.badRegionsOut)
	if pixelPath == "" && regionPath == "" {
		return nil, nil
	}

	d := &diagnosticsWriter{}
	if pixelPath != "" {
		file, writer, err := createCSVWriter(pixelPath)
		if err != nil {
			return nil, fmt.Errorf("create bad pixels csv: %w", err)
		}
		d.pixelFile = file
		d.pixelWriter = writer
		if err := writer.Write([]string{
			"pdf", "page", "x", "y",
			"poppler_r", "poppler_g", "poppler_b",
			"ours_r", "ours_g", "ours_b",
			"dr", "dg", "db", "delta_max", "region_id", "sample_rank",
		}); err != nil {
			_ = d.Close()
			return nil, err
		}
	}
	if regionPath != "" {
		file, writer, err := createCSVWriter(regionPath)
		if err != nil {
			_ = d.Close()
			return nil, fmt.Errorf("create bad regions csv: %w", err)
		}
		d.regionFile = file
		d.regionWriter = writer
		if err := writer.Write([]string{
			"pdf", "page", "region_id", "x0", "y0", "x1", "y1",
			"width", "height", "bad_pixels", "centroid_x", "centroid_y",
			"dominant_delta", "nearest_sample",
		}); err != nil {
			_ = d.Close()
			return nil, err
		}
	}
	return d, nil
}

func resolveDiagnosticsPath(outDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(outDir, path)
}

func createCSVWriter(path string) (*os.File, *csv.Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, csv.NewWriter(file), nil
}

func (d *diagnosticsWriter) HasPixelOutput() bool {
	return d != nil && d.pixelWriter != nil
}

func (d *diagnosticsWriter) HasRegionOutput() bool {
	return d != nil && d.regionWriter != nil
}

func (d *diagnosticsWriter) Write(pdf string, page int, diag compareDiagnostics) {
	if d == nil || d.err != nil {
		return
	}
	if d.pixelWriter != nil {
		for _, p := range diag.pixels {
			if err := d.pixelWriter.Write([]string{
				pdf, strconv.Itoa(page), strconv.Itoa(p.x), strconv.Itoa(p.y),
				strconv.Itoa(int(p.popplerR)), strconv.Itoa(int(p.popplerG)), strconv.Itoa(int(p.popplerB)),
				strconv.Itoa(int(p.oursR)), strconv.Itoa(int(p.oursG)), strconv.Itoa(int(p.oursB)),
				strconv.Itoa(p.deltaR), strconv.Itoa(p.deltaG), strconv.Itoa(p.deltaB),
				strconv.Itoa(p.deltaMax), strconv.Itoa(p.regionID), strconv.Itoa(p.sampleRank),
			}); err != nil {
				d.err = err
				return
			}
		}
	}
	if d.regionWriter != nil {
		for _, r := range diag.regions {
			nearestSample := ""
			if r.hasSample {
				nearestSample = fmt.Sprintf("%d:%d", r.nearSampleX, r.nearSampleY)
			}
			if err := d.regionWriter.Write([]string{
				pdf, strconv.Itoa(page), strconv.Itoa(r.id),
				strconv.Itoa(r.x0), strconv.Itoa(r.y0), strconv.Itoa(r.x1), strconv.Itoa(r.y1),
				strconv.Itoa(r.x1 - r.x0 + 1), strconv.Itoa(r.y1 - r.y0 + 1), strconv.Itoa(r.badPixels),
				fmt.Sprintf("%.2f", r.centroidX), fmt.Sprintf("%.2f", r.centroidY),
				strconv.Itoa(r.maxDelta), nearestSample,
			}); err != nil {
				d.err = err
				return
			}
		}
	}
}

func (d *diagnosticsWriter) Close() error {
	if d == nil || d.closed {
		return nil
	}
	d.closed = true
	if d.pixelWriter != nil {
		d.pixelWriter.Flush()
		if err := d.pixelWriter.Error(); err != nil && d.err == nil {
			d.err = err
		}
	}
	if d.regionWriter != nil {
		d.regionWriter.Flush()
		if err := d.regionWriter.Error(); err != nil && d.err == nil {
			d.err = err
		}
	}
	if d.pixelFile != nil {
		if err := d.pixelFile.Close(); err != nil && d.err == nil {
			d.err = err
		}
	}
	if d.regionFile != nil {
		if err := d.regionFile.Close(); err != nil && d.err == nil {
			d.err = err
		}
	}
	return d.err
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func rgbAt(img image.Image, x, y int) (uint8, uint8, uint8, bool) {
	b := img.Bounds()
	if x < 0 || y < 0 || x >= b.Dx() || y >= b.Dy() {
		return 0, 0, 0, false
	}
	switch im := img.(type) {
	case *image.RGBA:
		off := im.PixOffset(b.Min.X+x, b.Min.Y+y)
		return im.Pix[off], im.Pix[off+1], im.Pix[off+2], true
	case *image.NRGBA:
		off := im.PixOffset(b.Min.X+x, b.Min.Y+y)
		return im.Pix[off], im.Pix[off+1], im.Pix[off+2], true
	default:
		r, g, bv, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(bv >> 8), true
	}
}

func amplifyXOR(v uint8) uint8 {
	if v == 0 {
		return 0
	}
	return 64 + uint8((uint16(v)*191)/255)
}

func drawTileComponentEllipses(img *image.RGBA, marked []bool, tilesX, tilesY, tileSize, width, height int) {
	visited := make([]bool, len(marked))
	queue := make([]int, 0, len(marked))
	red := color.RGBA{255, 0, 0, 255}
	for i, ok := range marked {
		if !ok || visited[i] {
			continue
		}
		visited[i] = true
		queue = append(queue[:0], i)
		minTX, maxTX := i%tilesX, i%tilesX
		minTY, maxTY := i/tilesX, i/tilesX
		for head := 0; head < len(queue); head++ {
			cur := queue[head]
			tx := cur % tilesX
			ty := cur / tilesX
			if tx < minTX {
				minTX = tx
			}
			if tx > maxTX {
				maxTX = tx
			}
			if ty < minTY {
				minTY = ty
			}
			if ty > maxTY {
				maxTY = ty
			}
			for _, n := range neighborTiles(tx, ty, tilesX, tilesY) {
				if marked[n] && !visited[n] {
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}
		x0 := clamp(minTX*tileSize-8, 0, width-1)
		y0 := clamp(minTY*tileSize-8, 0, height-1)
		x1 := clamp((maxTX+1)*tileSize+8, 0, width-1)
		y1 := clamp((maxTY+1)*tileSize+8, 0, height-1)
		drawEllipse(img, x0, y0, x1, y1, red, 4)
	}
}

func neighborTiles(tx, ty, tilesX, tilesY int) []int {
	out := make([]int, 0, 4)
	if tx > 0 {
		out = append(out, ty*tilesX+tx-1)
	}
	if tx+1 < tilesX {
		out = append(out, ty*tilesX+tx+1)
	}
	if ty > 0 {
		out = append(out, (ty-1)*tilesX+tx)
	}
	if ty+1 < tilesY {
		out = append(out, (ty+1)*tilesX+tx)
	}
	return out
}

func drawEllipse(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, stroke int) {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	cx := float64(x0+x1) / 2
	cy := float64(y0+y1) / 2
	rx := math.Max(float64(x1-x0)/2, 6)
	ry := math.Max(float64(y1-y0)/2, 6)
	tolerance := math.Max(float64(stroke)/math.Min(rx, ry), 0.04)
	for y := clamp(y0-stroke, 0, img.Bounds().Dy()-1); y <= clamp(y1+stroke, 0, img.Bounds().Dy()-1); y++ {
		for x := clamp(x0-stroke, 0, img.Bounds().Dx()-1); x <= clamp(x1+stroke, 0, img.Bounds().Dx()-1); x++ {
			dx := (float64(x) - cx) / rx
			dy := (float64(y) - cy) / ry
			d := dx*dx + dy*dy
			if math.Abs(d-1) <= tolerance {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func summarize(rows []pageRow, docs int) summary {
	sum := summary{Documents: docs, Rows: len(rows)}
	for _, row := range rows {
		if row.Exact100 && row.Error == "" {
			sum.ExactPages++
		}
		if row.Error != "" {
			sum.ErrorRows++
		}
		sum.MatchedPixels += row.MatchedPixels
		sum.TotalPixels += row.TotalPixels
	}
	sum.OverallPercent = percent(sum.MatchedPixels, sum.TotalPixels)
	return sum
}

func writeCSV(path string, rows []pageRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"pdf", "page", "width", "height", "matched_pixels", "total_pixels", "exact_percent", "exact100", "poppler_png", "ours_png", "diff_png", "error"}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write([]string{
			row.PDFRel,
			strconv.Itoa(row.Page),
			strconv.Itoa(row.Width),
			strconv.Itoa(row.Height),
			strconv.FormatInt(row.MatchedPixels, 10),
			strconv.FormatInt(row.TotalPixels, 10),
			fmt.Sprintf("%.8f", row.ExactPercent),
			strconv.FormatBool(row.Exact100 && row.Error == ""),
			row.PopplerRel,
			row.OursRel,
			row.DiffRel,
			row.Error,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeHTML(path string, cfg config, sum summary, rows []pageRow) error {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>PDF Render Compare</title>`)
	b.WriteString(`<style>
body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f6f3ed;color:#201b16}
.dashboard{position:sticky;top:0;z-index:5;background:#201b16;color:#fff;padding:18px 22px;box-shadow:0 4px 18px rgba(0,0,0,.18)}
.dashboard h1{margin:0 0 12px;font-size:22px}
.cards{display:flex;gap:12px;flex-wrap:wrap}
.card{background:#342d26;border:1px solid #5a4f44;border-radius:12px;padding:10px 14px;min-width:150px}
.label{font-size:12px;color:#d7cfc6;text-transform:uppercase;letter-spacing:.04em}.value{font-size:22px;font-weight:700}
.meta{font-size:12px;color:#e6ded5;margin-top:10px}
table{border-collapse:separate;border-spacing:0 10px;width:100%;padding:14px 18px 30px}
th{position:sticky;top:117px;background:#f6f3ed;text-align:left;font-size:12px;text-transform:uppercase;letter-spacing:.04em;padding:8px;z-index:4}
td{background:#fff;border-top:1px solid #e1d9ce;border-bottom:1px solid #e1d9ce;padding:8px;vertical-align:top}
td:first-child{border-left:1px solid #e1d9ce;border-radius:12px 0 0 12px}
td:last-child{border-right:1px solid #e1d9ce;border-radius:0 12px 12px 0}
.doc{max-width:360px;font-size:13px;word-break:break-all}.page{font-weight:700;font-size:16px}
.stat{font-variant-numeric:tabular-nums;white-space:nowrap}.ok{color:#0a7a38;font-weight:700}.fail{color:#b00020;font-weight:700}
.err{max-width:360px;color:#b00020;font-size:12px;white-space:pre-wrap}
img.thumb{max-width:280px;max-height:360px;border:1px solid #d5cbbf;background:#eee;image-rendering:auto}
a{color:#7a3b00;text-decoration:none}a:hover{text-decoration:underline}
</style></head><body>`)
	b.WriteString(`<section class="dashboard"><h1>PDF Render Compare Dashboard</h1><div class="cards">`)
	writeCard(&b, "Documents", formatInt(int64(sum.Documents)))
	writeCard(&b, "Pages", formatInt(int64(sum.Rows)))
	writeCard(&b, "Exact 100 Pages", fmt.Sprintf("%s / %s", formatInt(int64(sum.ExactPages)), formatInt(int64(sum.Rows))))
	writeCard(&b, "Exact 100 Rate", fmt.Sprintf("%.2f%%", percent(int64(sum.ExactPages), int64(sum.Rows))))
	writeCard(&b, "Pixel Match", fmt.Sprintf("%s / %s", formatInt(sum.MatchedPixels), formatInt(sum.TotalPixels)))
	writeCard(&b, "Overall Exact", fmt.Sprintf("%.6f%%", sum.OverallPercent))
	writeCard(&b, "Error Rows", formatInt(int64(sum.ErrorRows)))
	b.WriteString(`</div><div class="meta">`)
	b.WriteString("scan-root=" + html.EscapeString(cfg.scanRoot) + " · dpi=" + strconv.Itoa(cfg.dpi) + " · backend=" + html.EscapeString(cfg.backend) + " · pure-go-jpeg=" + strconv.FormatBool(cfg.pureGoJPEG) + " · poppler=-cropbox")
	b.WriteString(`</div></section>`)
	b.WriteString(`<table><thead><tr><th>PDF / Page</th><th>Stats</th><th>Poppler</th><th>Ours</th><th>XOR + Red Circles</th></tr></thead><tbody>`)
	for _, row := range rows {
		statusClass := "fail"
		statusText := "DIFF"
		if row.Exact100 && row.Error == "" {
			statusClass = "ok"
			statusText = "EXACT100"
		}
		b.WriteString(`<tr><td><div class="doc"><a href="`)
		b.WriteString(html.EscapeString(relFromOutDir(filepath.Dir(path), filepath.Join(cfg.repoRoot, filepath.FromSlash(row.PDFRel)))))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(row.PDFRel))
		b.WriteString(`</a></div><div class="page">page `)
		b.WriteString(strconv.Itoa(row.Page))
		b.WriteString(`</div><div class="`)
		b.WriteString(statusClass)
		b.WriteString(`">`)
		b.WriteString(statusText)
		b.WriteString(`</div>`)
		if row.Error != "" {
			b.WriteString(`<div class="err">`)
			b.WriteString(html.EscapeString(row.Error))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</td><td class="stat">`)
		b.WriteString(formatInt(row.MatchedPixels) + " / " + formatInt(row.TotalPixels) + " px<br>")
		b.WriteString(fmt.Sprintf("%.8f%%<br>", row.ExactPercent))
		if row.Width > 0 && row.Height > 0 {
			b.WriteString(strconv.Itoa(row.Width) + " x " + strconv.Itoa(row.Height))
		}
		b.WriteString(`</td><td>`)
		writeImageLink(&b, row.PopplerRel)
		b.WriteString(`</td><td>`)
		writeImageLink(&b, row.OursRel)
		b.WriteString(`</td><td>`)
		writeImageLink(&b, row.DiffRel)
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></body></html>`)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeCard(b *strings.Builder, label, value string) {
	b.WriteString(`<div class="card"><div class="label">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</div><div class="value">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</div></div>`)
}

func writeImageLink(b *strings.Builder, rel string) {
	if rel == "" {
		b.WriteString(`<span class="fail">missing</span>`)
		return
	}
	esc := html.EscapeString(rel)
	b.WriteString(`<a href="` + esc + `"><img class="thumb" src="` + esc + `" loading="lazy"></a>`)
}

func errorRow(job pdfJob, page int, err string) pageRow {
	return pageRow{PDF: filepath.Base(job.path), PDFRel: job.rel, Page: page, Error: err}
}

func maxPageNumber(a, b map[int]string) int {
	maxPage := 0
	for page := range a {
		if page > maxPage {
			maxPage = page
		}
	}
	for page := range b {
		if page > maxPage {
			maxPage = page
		}
	}
	return maxPage
}

func passwordFor(rel string) string {
	if strings.Contains(rel, "libreoffice-writer-password") {
		return "openpassword"
	}
	return ""
}

func relToOut(cfg config, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(cfg.outDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func relFromOutDir(outDir, target string) string {
	rel, err := filepath.Rel(outDir, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}

func slugify(s string) string {
	s = strings.TrimSuffix(s, filepath.Ext(s))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r)
		if ok {
			b.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func exeName(name string) string {
	if os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}

func oneLine(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	out := strings.Join(fields, " ")
	if len(out) > 500 {
		return out[:500] + "..."
	}
	return out
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func appendError(base, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return base
	}
	if base == "" {
		return next
	}
	return base + "; " + next
}

func percent(n, d int64) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) * 100 / float64(d)
}

func formatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	pre := len(s) % 3
	if pre == 0 {
		pre = 3
	}
	out = append(out, s[:pre]...)
	for i := pre; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func clamp(v, min, max int) int {
	if max < min {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
