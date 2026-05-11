package splash

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"

	domaincanvas "github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// splashCanvas adapts a *Splash to the domain/canvas.Canvas interface (Splash.h:82, 02_api_design.md §8).
type splashCanvas struct {
	s      *Splash
	path   *xpath.Path
	width  int
	height int
	curX   float64
	curY   float64
	hasCur bool
	// pageYOriginPx is the float page height in canvas pixels. The renderer
	// passes this so the splash backend can fold a Y-flip into the CTM —
	// PDF user space is bottom-up, but the splash bitmap is top-down. The
	// renderer's initial transform has tY=0 and expects the canvas to perform
	// the flip itself (image_canvas does this per-vertex; we fold it into the
	// CTM in Transform() so xpath.NewXPath emits already-flipped device coords).
	pageYOriginPx float64

	// Text state — mirrors ImageCanvas.textPosition / inTextBlock and tracks
	// current font for ShowText (PDF 1.7 §9.4 text object operators).
	currentFont   entity.Font
	fontSize      float64
	textX         float64
	textY         float64
	inText        bool
	textUserSpace bool
	textCTM       [6]float64
	// type3Depth is non-zero while the evaluator replays a Type3 CharProc.
	// Embedded images in that path match Poppler's rounded SplashBitmap Y
	// origin, while normal glyph text keeps the exact float page origin.
	type3Depth int
	// strokeIndex counts Stroke calls for narrow diagnostic gates.
	strokeIndex int
	// fillIndex counts Fill calls for narrow diagnostic gates.
	fillIndex int
	// pendingFillTilingPattern keeps the original PDF tiling pattern around for
	// Poppler's fallback path, where overlapping/gapped tiles are replayed as
	// vector content under the parent fill clip instead of sampled from a single
	// pre-rendered cell bitmap.
	pendingFillTilingPattern *entity.TilingPattern
	pendingFillTilingTint    Color
	// pendingFillShadingPattern preserves a shading pattern installed via scn
	// so Fill can mirror Poppler's doShadingPatternFill path for mesh shadings:
	// current path becomes a clip, then the shading-specific rasterizer runs.
	pendingFillShadingPattern *entity.ShadingPattern

	// glyphCache memoises rasterized GlyphBitmaps keyed by font+glyph+size.
	// Splash::fillGlyph is hot in text-heavy pages; without this every Tj
	// rasterizes from scratch (Splash.cc:2603 callers in SplashOutputDev).
	glyphCache map[glyphCacheKey]*GlyphBitmap

	// glyphTransform mirrors ImageCanvas.glyphTransform — the linear part
	// [a,b,c,d] of the text rendering matrix (TRM = CTM × textMatrix). The
	// renderer's text positioning yields glyph origins in DEVICE pixels
	// (CurrentPosition returns trm[4],trm[5]) but RenderGlyph emits coords
	// at the PDF font-space size, so we must scale by gt[0]/gt[3] (and apply
	// any rotation/shear in gt[1]/gt[2]) when rasterising glyph paths.
	// Defaults to identity so unit tests that bypass the renderer continue
	// to work with the historical 1:1 mapping.
	glyphTransform [4]float64

	// annotationMask marks a generated annotation appearance mask. Poppler
	// paints highlight appearances into a transparent Form XObject, so the
	// alpha plane carries source coverage more faithfully than recovering it
	// from black-on-white RGB luminance.
	annotationMask bool
}

// glyphCacheKey identifies a rasterized glyph for memoisation. fontSize is
// quantized to 1/64pt so near-identical sizes still hit the cache (matches
// poppler's SplashOutputDev::doUpdateFont fontMatrix quantization). gtQ
// quantises the linear part of the glyph transform [a,b,c,d] (×1024 ints)
// so cached entries at different CTM scales/rotations remain distinct.
// phaseQ encodes the X sub-pixel phase (0..3 for poppler-FT phased renderer),
// or -1 for the path-based glyph (no phase dependency).
//
// font is the entity.Font interface value itself, not Name(): cm-super /
// LaTeX subset fonts (CMR8, CMTI8, CMSY10, …) commonly return Name()=="" so
// keying by name alone collapsed multiple distinct fonts onto a single cache
// slot, causing glyph bitmaps from one font to be served for an identically-
// numbered glyph in another (e.g. CMR's 'g' getting CMSY's '}' bitmap on
// GeoTopo p47 — manifesting as Topolo}ie / we}zusammen). Concrete font
// implementations are pointer types, so the interface value is comparable
// and uniquely identifies each font instance.
type glyphCacheKey struct {
	font    entity.Font
	glyph   uint32
	sizeQ   int64
	gtQ     [4]int64
	phaseXQ int8
	phaseYQ int8
}

// NewBackend returns a domain/canvas.Canvas backed by the Splash rasterizer (02_api_design.md §8).
//
// Hotfix #2: the bitmap is cleared to paper-white before returning so the
// renderer paints glyphs and fills onto a visible page (matching pdftoppm's
// behaviour) instead of leaving a transparent-black canvas where black ink
// would be invisible.
func NewBackend(width, height int) domaincanvas.Canvas {
	bm := NewBitmap(width, height, ModeRGB8, true)
	sp, _ := New(bm, true)
	// Match pdftoppm's SplashOutputDev::startPage which calls
	// splash->setStrokeAdjust(globalParams->getStrokeAdjust()) — strokeAdjust
	// is enabled by default in poppler builds, so pdftoppm snaps axis-aligned
	// rect/stroke edges to integer pixel boundaries (Splash.cc:1681,
	// SplashOutputDev.cc:startPage). Without this the splash backend produces
	// AA fringe along rect edges that pdftoppm does not.
	sp.SetStrokeAdjust(os.Getenv("PDF_DEBUG_SPLASH_DISABLE_STROKE_ADJUST") != "1")
	// The evaluator feeds path coordinates already flipped into Splash's Y-down
	// device space. Poppler builds stroke outlines before applying that mirrored
	// CTM, so compensate the stroke normal only for this canvas adapter.
	sp.SetMirrorStrokeNormals(os.Getenv("PDF_SPLASH_DISABLE_MIRROR_STROKE_NORMALS") == "")
	if usePopplerTransparentPageAlpha() {
		bm.ClearWithAlpha(paperColor(bm.Mode()), 0)
	} else {
		bm.Clear(paperColor(bm.Mode()))
	}
	return &splashCanvas{
		s:              sp,
		path:           xpath.NewPath(),
		width:          width,
		height:         height,
		glyphTransform: [4]float64{1, 0, 0, 1},
	}
}

func newAnnotationMaskBackend(width, height int) *splashCanvas {
	bm := NewBitmap(width, height, ModeRGB8, true)
	sp, _ := New(bm, true)
	sp.SetStrokeAdjust(os.Getenv("PDF_DEBUG_SPLASH_DISABLE_STROKE_ADJUST") != "1")
	sp.SetMirrorStrokeNormals(os.Getenv("PDF_SPLASH_DISABLE_MIRROR_STROKE_NORMALS") == "")
	bm.ClearWithAlpha(paperColor(bm.Mode()), 0)
	return &splashCanvas{
		s:              sp,
		path:           xpath.NewPath(),
		width:          width,
		height:         height,
		glyphTransform: [4]float64{1, 0, 0, 1},
		annotationMask: true,
	}
}

func usePopplerTransparentPageAlpha() bool {
	return os.Getenv("PDF_DEBUG_SPLASH_OPAQUE_PAGE_ALPHA") != "1"
}

// SetOpaquePaperBackground lets the renderer install the page paper without
// drawing an opaque rectangle. Poppler keeps paper in SplashOutputDev and
// composites it at endPage; PDF_DEBUG_SPLASH_OPAQUE_PAGE_ALPHA keeps the old
// opaque-clear behavior as a diagnostic fallback.
func (c *splashCanvas) SetOpaquePaperBackground(bg color.Color) {
	if c == nil || c.s == nil || c.s.bitmap == nil {
		return
	}
	paper, _ := convertColorAndAlpha(bg, c.s.bitmap.mode)
	if usePopplerTransparentPageAlpha() {
		c.s.bitmap.ClearWithAlpha(paper, 0)
		return
	}
	c.s.bitmap.Clear(paper)
}

// paperColor returns the paper-white pixel for the given Splash mode.
func paperColor(mode ColorMode) Color {
	switch mode {
	case ModeCMYK8:
		// 0,0,0,0 = white in CMYK (no ink).
		return Color{}
	case ModeDeviceN8:
		return Color{}
	default:
		// Mono8 / RGB8 / BGR8 / XBGR8 — fill all bytes with 0xFF for white.
		return Color{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	}
}

// convertColor maps a stdlib color.Color to a splash.Color packed for the
// active bitmap mode (Splash.cc:1601 fill-pattern install path).
func convertColor(col color.Color, mode ColorMode) Color {
	c, _ := convertColorAndAlpha(col, mode)
	return c
}

// convertColorAndAlpha splits Go's alpha-premultiplied color.Color contract into
// Poppler's separate source color and alpha channels.
func convertColorAndAlpha(col color.Color, mode ColorMode) (Color, float64) {
	if col == nil {
		return Color{}, 1
	}
	r16, g16, b16, a16 := col.RGBA()
	alpha := float64(a16) / 0xffff
	r8 := unpremultiplyColorComponent(r16, a16)
	g8 := unpremultiplyColorComponent(g16, a16)
	b8 := unpremultiplyColorComponent(b16, a16)
	switch mode {
	case ModeMono8:
		// ITU-R BT.709 luma — matches the SplashSolidColor mono path.
		y := (2126*uint32(r8) + 7152*uint32(g8) + 722*uint32(b8) + 5000) / 10000
		if y > 0xFF {
			y = 0xFF
		}
		return Color{byte(y)}, alpha
	case ModeBGR8:
		return Color{b8, g8, r8}, alpha
	case ModeXBGR8:
		return Color{b8, g8, r8, 0}, alpha
	case ModeCMYK8:
		// Naive CMYK conversion (corpus is RGB; this is a fallback).
		k := uint32(0xFF)
		if uint32(r8) < k {
			k = uint32(r8)
		}
		if uint32(g8) < k {
			k = uint32(g8)
		}
		if uint32(b8) < k {
			k = uint32(b8)
		}
		c := byte(0xFF - r8)
		m := byte(0xFF - g8)
		yC := byte(0xFF - b8)
		return Color{c, m, yC, byte(0xFF - k)}, alpha
	case ModeDeviceN8:
		// Phase 2 corpus has no DeviceN — fall back to RGB packing.
		return Color{r8, g8, b8}, alpha
	default: // ModeRGB8 (and ModeMono1 unused).
		return Color{r8, g8, b8}, alpha
	}
}

func unpremultiplyColorComponent(component, alpha uint32) byte {
	if alpha == 0 {
		return 0
	}
	v := (uint64(component)*0xff + uint64(alpha)/2) / uint64(alpha)
	if v > 0xff {
		return 0xff
	}
	return byte(v)
}

// Width returns the canvas width in pixels.
func (c *splashCanvas) Width() int { return c.width }

// Height returns the canvas height in pixels.
func (c *splashCanvas) Height() int { return c.height }

// Bounds returns the pixel rectangle of the canvas.
func (c *splashCanvas) Bounds() image.Rectangle {
	return image.Rect(0, 0, c.width, c.height)
}

// CurrentClipBBox returns the current clip bbox in evaluator coordinates
// (device X with PDF-style bottom-up Y), matching Poppler's state->getClipBBox
// input to SplashOutputDev::univariateShadedFill.
func (c *splashCanvas) CurrentClipBBox() ([4]float64, bool) {
	if c == nil || c.s == nil {
		return [4]float64{}, false
	}
	xMin, yMin, xMax, yMax, ok := c.s.ensureClip().VectorEffectiveBounds()
	if !ok || xMax <= xMin || yMax <= yMin {
		return [4]float64{}, false
	}
	yOrigin := c.flipYOrigin()
	return [4]float64{
		math.Floor(xMin),
		math.Floor(yOrigin - yMax),
		math.Ceil(xMax),
		math.Ceil(yOrigin - yMin),
	}, true
}

// MoveTo appends a moveto to the in-progress path (SplashPath::moveTo, SplashPath.h:83).
// The evaluator hands us coordinates that have been multiplied through the
// CTM but with Y still in PDF (bottom-up) orientation; flipY converts them to
// the splash bitmap's top-down axis (matches image_canvas's `yOrigin-ty`
// per-vertex flip in image_canvas.go:3857/3870/3884-3886).
func (c *splashCanvas) MoveTo(x, y float64) {
	_ = c.path.MoveToDroppingEmptySubpath(x, c.flipY(y))
	c.curX, c.curY, c.hasCur = x, y, true
}

// LineTo appends a lineto to the in-progress path (SplashPath::lineTo, SplashPath.h:86).
func (c *splashCanvas) LineTo(x, y float64) {
	_ = c.path.LineTo(x, c.flipY(y))
	c.curX, c.curY, c.hasCur = x, y, true
}

// CurveTo appends a cubic Bezier to the in-progress path (SplashPath::curveTo, SplashPath.h:90).
func (c *splashCanvas) CurveTo(c1x, c1y, c2x, c2y, x, y float64) {
	_ = c.path.CurveTo(c1x, c.flipY(c1y), c2x, c.flipY(c2y), x, c.flipY(y))
	c.curX, c.curY, c.hasCur = x, y, true
}

// Rectangle appends a four-segment closed rectangle to the path (SplashPath::moveTo+lineTo+close).
func (c *splashCanvas) Rectangle(x, y, width, height float64) {
	y0 := c.flipY(y)
	y1 := c.flipY(y + height)
	_ = c.path.MoveTo(x, y0)
	_ = c.path.LineTo(x+width, y0)
	_ = c.path.LineTo(x+width, y1)
	_ = c.path.LineTo(x, y1)
	_ = c.path.Close(false)
}

// ClosePath closes the current subpath (SplashPath::close, SplashPath.h:95).
func (c *splashCanvas) ClosePath() {
	_ = c.path.Close(false)
}

// Fill rasterizes the accumulated path with the fill pattern (Splash::fill, Splash.cc:2362).
func (c *splashCanvas) Fill() {
	c.fill(false)
}

// FillEvenOdd rasterizes the accumulated path with the even-odd rule.
func (c *splashCanvas) FillEvenOdd() {
	c.fill(true)
}

func (c *splashCanvas) fill(evenOdd bool) {
	fillIndex := c.fillIndex
	c.fillIndex++
	if c.fillPendingShadingPattern(evenOdd) {
		c.path = xpath.NewPath()
		c.hasCur = false
		return
	}
	if c.fillPendingTilingPatternByTileReplay(evenOdd) {
		c.path = xpath.NewPath()
		c.hasCur = false
		return
	}
	if shouldSkipSplashFillIndexForDebug(fillIndex) {
		c.path = xpath.NewPath()
		c.hasCur = false
		return
	}
	if shouldTraceSplashFillIndex(fillIndex) {
		c.debugTraceFill(fillIndex, evenOdd)
	}

	c.s.debugFillIndex = fillIndex
	_ = c.s.Fill(c.path, evenOdd)
	c.s.debugFillIndex = -1
	c.path = xpath.NewPath()
	c.hasCur = false
}

// Stroke rasterizes the accumulated path with the stroke pattern (Splash::stroke, Splash.cc:1810).
func (c *splashCanvas) Stroke() {
	strokeIndex := c.strokeIndex
	c.strokeIndex++
	if os.Getenv("PDF_DEBUG_SPLASH_STROKE_TRACE") != "" {
		c.debugTraceStroke(strokeIndex)
	}
	if shouldSkipSplashStrokeIndexForDebug(strokeIndex) {
		c.path = xpath.NewPath()
		c.hasCur = false
		return
	}
	c.s.debugStrokeIndex = strokeIndex
	strokePath, strokeMatrix := c.popplerStrokePathAndMatrix()
	savedMatrix := c.s.state.matrix
	savedMirrorStrokeNormals := c.s.mirrorStrokeNormals
	savedStrokeAdjust := c.s.state.strokeAdjust
	c.s.SetMatrix(strokeMatrix)
	c.s.SetMirrorStrokeNormals(false)
	if shouldDisableStrokeAdjustForDebugStrokeIndex(strokeIndex) {
		c.s.SetStrokeAdjust(false)
	}
	_ = c.s.Stroke(strokePath)
	c.s.SetMirrorStrokeNormals(savedMirrorStrokeNormals)
	c.s.SetStrokeAdjust(savedStrokeAdjust)
	c.s.SetMatrix(savedMatrix)
	c.s.debugStrokeIndex = -1
	c.path = xpath.NewPath()
	c.hasCur = false
}

func (c *splashCanvas) popplerStrokePathAndMatrix() (*xpath.Path, [6]float64) {
	if c == nil {
		return xpath.NewPath(), [6]float64{1, 0, 0, 1, 0, 0}
	}
	matrix := c.pathYFlipMatrix()
	if c.path == nil {
		return xpath.NewPath(), matrix
	}
	if c.strokePathHasDeviceAlignedButtCapPlane() {
		return c.path.Clone(), [6]float64{1, 0, 0, 1, 0, 0}
	}
	// The evaluator already applies the PDF CTM and backend.MoveTo/LineTo then
	// flips Y into Splash bitmap space. Poppler keeps the path pre-flip and
	// applies the Y-down CTM in SplashXPath, after makeStrokePath has built the
	// outline. Undo the eager flip for stroke geometry, then pass the same flip
	// as Splash's matrix so stroke-adjust and AA scanning still see device coords.
	return c.path.Transformed(matrix), matrix
}

func (c *splashCanvas) strokePathHasDeviceAlignedButtCapPlane() bool {
	if c == nil || c.s == nil || c.s.state == nil || c.path == nil {
		return false
	}
	if os.Getenv("PDF_DEBUG_SPLASH_DISABLE_DEVICE_CAP_GATE") == "1" {
		return false
	}
	if !c.s.state.strokeAdjust ||
		c.s.state.lineCap != int(LineCapButt) ||
		len(c.s.state.lineDash) != 0 ||
		c.s.state.lineWidth <= 0 {
		return false
	}
	if c.path.Length() != 2 {
		return false
	}
	p0, f0 := c.path.Point(0)
	p1, f1 := c.path.Point(1)
	if f0&pathFlagFirst == 0 ||
		f1&pathFlagLast == 0 ||
		f0&pathFlagClosed != 0 ||
		f1&pathFlagClosed != 0 {
		return false
	}
	const axisEpsilon = 1e-9
	if math.Abs(p1.X-p0.X) <= axisEpsilon {
		if strokePathIsLongOnePointVerticalGridLine(p0, p1, c.s.state.lineWidth) {
			traceDeviceCapGate("vertical-grid", p0, p1, c.s.state.lineWidth)
			return true
		}
		return false
	}
	if math.Abs(p1.Y-p0.Y) <= axisEpsilon {
		return false
	}
	return false
}

func strokePathIsLongOnePointVerticalGridLine(p0, p1 xpath.PathPoint, lineWidth float64) bool {
	const (
		onePointAt150DPI = 150.0 / 72.0
		widthEpsilon     = 0.01
		minGridLength    = 32.0
	)
	if math.Abs(lineWidth-onePointAt150DPI) > widthEpsilon {
		return false
	}
	if math.Abs(p1.Y-p0.Y) <= minGridLength {
		return false
	}
	return strokeCapPlaneAlreadyHalfPixel(p0.Y) ||
		strokeCapPlaneAlreadyHalfPixel(p1.Y)
}

func traceDeviceCapGate(reason string, p0, p1 xpath.PathPoint, lineWidth float64) {
	if os.Getenv("PDF_DEBUG_SPLASH_DEVICE_CAP_GATE_TRACE") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "SPLASH_DEVICE_CAP_GATE reason=%s width=%.8f p0=(%.8f,%.8f) p1=(%.8f,%.8f)\n",
		reason, lineWidth, p0.X, p0.Y, p1.X, p1.Y)
}

func (c *splashCanvas) debugTraceStroke(index int) {
	if c == nil || c.s == nil || c.s.state == nil || c.path == nil {
		return
	}
	var col Color
	if c.s.state.strokePattern != nil && c.s.state.strokePattern.IsStatic() {
		_ = c.s.state.strokePattern.GetColor(0, 0, &col)
	}
	mode := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_STROKE_TRACE"))
	isOrange := col[0] == 0xff && col[1] == 0x80 && col[2] == 0x00
	if mode != "all" && !isOrange {
		return
	}
	x0, y0, x1, y1 := c.debugPathBounds()
	fmt.Fprintf(os.Stderr, "SPLASH_STROKE_TRACE index=%d color=(%d,%d,%d) width=%.8f cap=%d join=%d miter=%.4f matrix=%v pathLen=%d bounds=(%.3f,%.3f)-(%.3f,%.3f)\n",
		index, col[0], col[1], col[2], c.s.state.lineWidth, c.s.state.lineCap, c.s.state.lineJoin, c.s.state.miterLimit, c.s.state.matrix, c.path.Length(), x0, y0, x1, y1)
	for i := 0; i < c.path.Length(); i++ {
		pt, flag := c.path.Point(i)
		fmt.Fprintf(os.Stderr, "  pt[%03d]=(%.8f,%.8f) flag=0x%02x\n", i, pt.X, pt.Y, flag)
	}
}

func (c *splashCanvas) debugPathBounds() (x0, y0, x1, y1 float64) {
	if c.path == nil || c.path.Length() == 0 {
		return 0, 0, 0, 0
	}
	pt, _ := c.path.Point(0)
	x0, y0, x1, y1 = pt.X, pt.Y, pt.X, pt.Y
	for i := 1; i < c.path.Length(); i++ {
		pt, _ = c.path.Point(i)
		if pt.X < x0 {
			x0 = pt.X
		}
		if pt.Y < y0 {
			y0 = pt.Y
		}
		if pt.X > x1 {
			x1 = pt.X
		}
		if pt.Y > y1 {
			y1 = pt.Y
		}
	}
	return x0, y0, x1, y1
}

func shouldSkipSplashStrokeIndexForDebug(index int) bool {
	return debugIndexListContains(os.Getenv("PDF_DEBUG_SPLASH_SKIP_STROKE_INDEX"), index)
}

func shouldSkipSplashFillIndexForDebug(index int) bool {
	return debugIndexListContains(os.Getenv("PDF_DEBUG_SPLASH_SKIP_FILL_INDEX"), index)
}

func debugIndexListContains(raw string, index int) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for _, field := range strings.Split(raw, ",") {
		want, err := strconv.Atoi(strings.TrimSpace(field))
		if err == nil && want == index {
			return true
		}
	}
	return false
}

func shouldDisableStrokeAdjustForDebugStrokeIndex(index int) bool {
	return debugIndexListContains(os.Getenv("PDF_DEBUG_SPLASH_DISABLE_STROKE_ADJUST_FOR_STROKE_INDEX"), index)
}

func shouldTraceSplashFillIndex(index int) bool {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_FILL_TRACE"))
	if raw == "" {
		return false
	}
	if raw == "all" {
		return true
	}
	want, err := strconv.Atoi(raw)
	return err == nil && want == index
}

func (c *splashCanvas) debugTraceFill(index int, evenOdd bool) {
	if c == nil || c.s == nil || c.s.state == nil || c.path == nil {
		return
	}
	var col Color
	if c.s.state.fillPattern != nil && c.s.state.fillPattern.IsStatic() {
		_ = c.s.state.fillPattern.GetColor(0, 0, &col)
	}
	x0, y0, x1, y1 := c.debugPathBounds()
	pendingShading := "nil"
	pendingPatches := 0
	if c.pendingFillShadingPattern != nil && c.pendingFillShadingPattern.GetShading() != nil {
		shading := c.pendingFillShadingPattern.GetShading()
		pendingShading = shading.GetShadingType().String()
		pendingPatches = len(shading.GetPatches())
	}
	fmt.Fprintf(os.Stderr, "SPLASH_FILL_TRACE index=%d color=(%d,%d,%d) evenOdd=%t type3Depth=%d pendingShading=%s pendingPatches=%d matrix=%v pathLen=%d bounds=(%.3f,%.3f)-(%.3f,%.3f)\n",
		index, col[0], col[1], col[2], evenOdd, c.type3Depth, pendingShading, pendingPatches, c.s.state.matrix, c.path.Length(), x0, y0, x1, y1)
	for i := 0; i < c.path.Length(); i++ {
		pt, flag := c.path.Point(i)
		fmt.Fprintf(os.Stderr, "  pt[%03d]=(%.8f,%.8f) flag=0x%02x\n", i, pt.X, pt.Y, flag)
	}
}

// Clip intersects the clip with the current path, non-zero rule (Splash::clipToPath, Splash.cc:1704).
func (c *splashCanvas) Clip() {
	c.intersectCurrentPathVectorClipBounds()
	_ = c.s.ClipToPath(c.path, false)
	c.path = xpath.NewPath()
	c.hasCur = false
}

// EoClip intersects the clip with the current path, even-odd rule (Splash::clipToPath, Splash.cc:1704).
func (c *splashCanvas) EoClip() {
	c.intersectCurrentPathVectorClipBounds()
	_ = c.s.ClipToPath(c.path, true)
	c.path = xpath.NewPath()
	c.hasCur = false
}

func (c *splashCanvas) intersectCurrentPathVectorClipBounds() {
	if c == nil || c.s == nil || c.path == nil || c.path.Length() == 0 {
		return
	}
	pt, _ := c.path.Point(0)
	xMin, xMax := pt.X, pt.X
	yMin, yMax := pt.Y, pt.Y
	for i := 1; i < c.path.Length(); i++ {
		pt, _ = c.path.Point(i)
		if pt.X < xMin {
			xMin = pt.X
		}
		if pt.X > xMax {
			xMax = pt.X
		}
		if pt.Y < yMin {
			yMin = pt.Y
		}
		if pt.Y > yMax {
			yMax = pt.Y
		}
	}
	if os.Getenv("PDF_DEBUG_SPLASH_CLIP_BBOX_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "SPLASH_CLIP_BBOX_TRACE pathLen=%d matrix=%v bounds=(%.9f,%.9f)-(%.9f,%.9f)\n",
			c.path.Length(), c.s.state.matrix, xMin, yMin, xMax, yMax)
		for i := 0; i < c.path.Length(); i++ {
			pt, flag := c.path.Point(i)
			fmt.Fprintf(os.Stderr, "  clip_pt[%03d]=(%.9f,%.9f) flag=0x%02x\n", i, pt.X, pt.Y, flag)
		}
	}
	c.s.ensureClip().IntersectVectorBounds(xMin, yMin, xMax, yMax)
}

// DrawText rasterizes text at (x, y) glyph-by-glyph through Splash::fillGlyph
// (PDF 1.7 §9.4 Tj/TJ; Splash.cc:2603 fillGlyph dispatch).
//
// Path A (D1 LOCKED 2026-04-27): glyph outlines come from entity.Font.RenderGlyph
// already scaled to fontSize, are converted to xpath.Path with Y flipped into
// splash bitmap space, then rasterized via splash.RasterizeGlyph and blitted
// via Splash.FillGlyph. A bitmap renderer (FreeType phased) fast path mirroring
// ImageCanvas would be a future Phase 4 follow-up.
func (c *splashCanvas) DrawText(text string, x, y float64, font entity.Font, fontSize float64) error {
	if font == nil || len(text) == 0 || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	c.currentFont = font
	c.fontSize = fontSize
	c.BeginText(x, y)
	err := c.ShowText(text)
	c.EndText()
	return err
}

// DrawTextUserSpace mirrors Poppler's SplashOutputDev::drawChar path: glyph
// origins stay in PDF user space until Splash::fillChar applies the page CTM.
func (c *splashCanvas) DrawTextUserSpace(text string, x, y float64, ctm [6]float64, font entity.Font, fontSize float64) error {
	if font == nil || len(text) == 0 || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	c.currentFont = font
	c.fontSize = fontSize
	c.textUserSpace = true
	c.textCTM = ctm
	c.BeginText(x, y)
	err := c.ShowText(text)
	c.EndText()
	c.textUserSpace = false
	c.textCTM = [6]float64{}
	return err
}

// BeginText opens a text object: stores the text origin in PDF user space (Y-up)
// for subsequent ShowText / MoveTextPoint calls (PDF 1.7 §9.4.1 BT operator).
func (c *splashCanvas) BeginText(x, y float64) {
	c.textX = x
	c.textY = y
	c.inText = true
}

// EndText closes the text object (PDF 1.7 §9.4.1 ET operator).
func (c *splashCanvas) EndText() {
	c.inText = false
}

// ShowText emits glyphs for text at the current text position, advancing the
// text X cursor by each glyph's PDF-space advance width (PDF 1.7 §9.4.4 Tj;
// Splash.cc:2603 fillGlyph blit per glyph).
func (c *splashCanvas) ShowText(text string) error {
	if c.currentFont == nil || len(text) == 0 || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	font := c.currentFont
	fontSize := c.fontSize
	unitsPerEm := float64(font.UnitsPerEm())
	if unitsPerEm <= 0 {
		unitsPerEm = 1000
	}
	codes := splashSplitTextCodes([]byte(text), font)
	curX := c.textX
	curY := c.textY
	userToSplash := flipYMatrix(c.textCTM, c.flipYOrigin())
	for _, charCode := range codes {
		glyph, err := font.CharCodeToGlyph(charCode)
		if err == nil {
			glyphX, glyphY := curX, c.flipY(curY)
			if c.textUserSpace {
				glyphX, glyphY = splashTransformPoint(userToSplash, curX, curY)
			}
			glyphX = splashGlyphSnapCoord(glyphX)
			glyphY = splashGlyphSnapCoord(glyphY)
			if os.Getenv("SPLASH_DEBUG_GT") != "" {
				phaseXSlot := splashGlyphXPhaseSlot(glyphX, font, fontSize, math.Abs(c.glyphTransform[3]))
				phaseYSlot := splashGlyphYPhaseSlot(glyphY, font, fontSize, math.Abs(c.glyphTransform[3]))
				fracX := glyphX - math.Floor(glyphX)
				fracY := glyphY - math.Floor(glyphY)
				cacheH := splashGlyphCacheHeight(font, fontSize, math.Abs(c.glyphTransform[3]))
				fmt.Fprintf(os.Stderr, "showGlyph font=%q code=%d glyph=%d x=%.17g y=%.17g glyphXY=(%.17g,%.17g) frac=(%.17g,%.17g) phase=(%.2f,%.2f) cacheH=%.4f userSpace=%t ctm=%v fillPattern=%T matrix=%v\n",
					font.Name(), charCode, glyph, curX, curY, glyphX, glyphY, fracX, fracY, phaseXSlot, phaseYSlot, cacheH, c.textUserSpace, c.textCTM, c.s.state.fillPattern, c.s.state.matrix)
			}
			// Pass the final Splash-space glyph origin as phase source. Poppler
			// keys FreeType glyphs by both x and y fractional phases before
			// flooring the blit origin in Splash::fillGlyph.
			gb := c.fetchGlyphPhased(font, glyph, fontSize, glyphX, glyphY)
			if gb != nil && gb.W > 0 && gb.H > 0 {
				// Place glyph at (curX, flipY(curY)) — the glyph path was
				// already flipped into splash top-down space when rasterized,
				// so this is the splash-bitmap origin for the glyph.
				_ = c.s.FillGlyph(glyphX, glyphY, gb)
			}
		}
		width, werr := font.GetGlyphWidth(glyph)
		if werr != nil {
			width = 500
		}
		curX += (width / unitsPerEm) * fontSize
	}
	c.textX = curX
	return nil
}

// MoveTextPoint translates the current text origin by (tx, ty) in PDF user
// space (PDF 1.7 §9.4.2 Td operator).
func (c *splashCanvas) MoveTextPoint(tx, ty float64) {
	c.textX += tx
	c.textY += ty
}

// SetGlyphTransform sets the linear part [a,b,c,d] of the text rendering
// matrix used to map glyph path coordinates (PDF font space) to device pixels.
// Mirrors ImageCanvas.SetGlyphTransform; the renderer calls this via
// syncCanvasGlyphTransform before each text run so glyph rasterisation
// honours the current CTM scale (e.g. 150/72 ≈ 2.083 at 150 DPI).
func (c *splashCanvas) SetGlyphTransform(t [4]float64) {
	c.glyphTransform = t
	if os.Getenv("SPLASH_DEBUG_GT") != "" {
		fmt.Fprintf(os.Stderr, "SetGlyphTransform: %v\n", t)
	}
}

// fetchGlyph returns a rasterized GlyphBitmap for (font, glyph, fontSize),
// using c.glyphCache to amortise repeated rasterization of the same glyph
// (Splash.cc:2603 hot path mitigation).
//
// Phase 4-Dev6 (2026-04-28): when the font implements BitmapGlyphRendererPhased
// (FreeType-backed Type1/TrueType/CFF) we route through FT_Render_Glyph in
// normal AA mode — exactly like Poppler's SplashFTFont::makeGlyph
// (SplashFTFont.cc:187-263). This bypasses the splash port's path-based 4×4
// AA scanner with aaGamma=1.5, which over-fills edges and produces ~5pp
// darker glyphs vs Poppler on LaTeX cm-super pages (GeoTopo p55: 94.27%
// → 99%+). Path-based fallback retained for fonts without FT.
func (c *splashCanvas) fetchGlyph(font entity.Font, glyph uint32, fontSize float64) *GlyphBitmap {
	return c.fetchGlyphPhased(font, glyph, fontSize, 0, 0)
}

// fetchGlyphPhased is fetchGlyph with explicit sub-pixel phase support.
// phaseX/phaseY are Splash-space glyph origins used by the FT phased renderer
// to align glyph anti-aliasing with the fractional cursor position.
func (c *splashCanvas) fetchGlyphPhased(font entity.Font, glyph uint32, fontSize, phaseX, phaseY float64) *GlyphBitmap {
	if c.glyphCache == nil {
		c.glyphCache = make(map[glyphCacheKey]*GlyphBitmap)
	}
	gt := c.glyphTransform
	if gt == ([4]float64{}) {
		gt = [4]float64{1, 0, 0, 1}
	}
	// Quantise the linear part of the glyph transform so cache keys are
	// stable across float jitter while still distinguishing different scales
	// (e.g. cached entries at 150 DPI vs 72 DPI must not collide).
	gtQ := [4]int64{
		int64(math.Round(gt[0] * 1024)),
		int64(math.Round(gt[1] * 1024)),
		int64(math.Round(gt[2] * 1024)),
		int64(math.Round(gt[3] * 1024)),
	}

	// Try FreeType bitmap fast path first (matches ImageCanvas behaviour and
	// Poppler's SplashFTFont::makeGlyph). Rotated/sheared glyphs use the full
	// 2x2 matrix path; axis-aligned glyphs keep the existing scale-only path.
	// Set SPLASH_DISABLE_FT_GLYPH=1 to force the path-based fallback.
	useFTFastPath := os.Getenv("SPLASH_DISABLE_FT_GLYPH") == ""
	axisAligned := math.Abs(gt[1]) < 1e-6 && math.Abs(gt[2]) < 1e-6 && gt[0] > 0 && gt[3] > 0
	if useFTFastPath && !axisAligned {
		scaleY := math.Hypot(gt[2], gt[3])
		if mpr, ok := unwrapSplashMatrixPhasedRenderer(font); ok && scaleY > 0 {
			phaseXSlot := splashGlyphXPhaseSlot(phaseX, font, fontSize, scaleY)
			phaseYSlot := splashGlyphYPhaseSlot(phaseY, font, fontSize, scaleY)
			phaseXQ := splashGlyphPhaseQ(phaseXSlot)
			phaseYQ := splashGlyphPhaseQ(phaseYSlot)
			key := glyphCacheKey{
				font:    font,
				glyph:   glyph,
				sizeQ:   int64(math.Round(fontSize * 64)),
				gtQ:     gtQ,
				phaseXQ: phaseXQ,
				phaseYQ: phaseYQ,
			}
			if gb, ok := c.glyphCache[key]; ok {
				return gb
			}
			buf, bw, bh, bleft, btop, err := mpr.RenderGlyphBitmapMatrixPhased(glyph, fontSize, gt, phaseXSlot, phaseYSlot)
			if err == nil && len(buf) > 0 && bw > 0 && bh > 0 {
				gb := &GlyphBitmap{
					X:    -bleft,
					Y:    btop,
					W:    bw,
					H:    bh,
					AA:   true,
					Data: buf,
				}
				c.glyphCache[key] = gb
				if os.Getenv("SPLASH_DEBUG_GT") != "" {
					fmt.Fprintf(os.Stderr, "fetchGlyph(FT-matrix) font=%s glyph=%d size=%.8f gt=%v phase=(%.2f,%.2f) bw=%d bh=%d bleft=%d btop=%d\n",
						font.Name(), glyph, fontSize, gt, phaseXSlot, phaseYSlot, bw, bh, bleft, btop)
				}
				return gb
			}
		}
	}
	if useFTFastPath && axisAligned {
		// Prefer the Transformed (axis-anisotropic) phased renderer when the
		// font supports it: this mirrors ImageCanvas.DrawText
		// (image_canvas_text.go:46) and Poppler's SplashFTFont path which
		// passes pixel_size_x / pixel_size_y separately to FreeType. Even when
		// scaleX==scaleY, the Transformed entry point uses a slightly different
		// pixel_size formula (size_pt*scale vs size_pt*dpi/72) — the resulting
		// 1e-3 float jitter triggers different FT 26.6 fixed-point rounding,
		// producing visibly different glyph alpha edges (GeoTopo p96 cm-super
		// colon dot: 14k splash-only diff pixels vs canvas).
		scaleX := math.Abs(gt[0])
		scaleY := math.Abs(gt[3])
		if tpr, ok := unwrapSplashTransformedPhasedRenderer(font); ok && scaleX > 0 && scaleY > 0 && os.Getenv("PDF_DEBUG_SPLASH_GLYPH_SKIP_TRANSFORMED") == "" {
			phaseXSlot := splashGlyphXPhaseSlot(phaseX, font, fontSize, scaleY)
			phaseYSlot := splashGlyphYPhaseSlot(phaseY, font, fontSize, scaleY)
			phaseXQ := splashGlyphPhaseQ(phaseXSlot)
			phaseYQ := splashGlyphPhaseQ(phaseYSlot)
			key := glyphCacheKey{
				font:    font,
				glyph:   glyph,
				sizeQ:   int64(math.Round(fontSize * 64)),
				gtQ:     gtQ,
				phaseXQ: phaseXQ,
				phaseYQ: phaseYQ,
			}
			if gb, ok := c.glyphCache[key]; ok {
				return gb
			}
			buf, bw, bh, bleft, btop, err := tpr.RenderGlyphBitmapTransformedPhased(glyph, fontSize, scaleX, scaleY, phaseXSlot, phaseYSlot)
			if err == nil && len(buf) > 0 && bw > 0 && bh > 0 {
				gb := &GlyphBitmap{
					X:    -bleft,
					Y:    btop,
					W:    bw,
					H:    bh,
					AA:   true,
					Data: buf,
				}
				c.glyphCache[key] = gb
				if os.Getenv("SPLASH_DEBUG_GT") != "" {
					fmt.Fprintf(os.Stderr, "fetchGlyph(FT-transformed) font=%s glyph=%d size=%.8f sx=%.8f sy=%.8f phase=(%.2f,%.2f) bw=%d bh=%d bleft=%d btop=%d\n",
						font.Name(), glyph, fontSize, scaleX, scaleY, phaseXSlot, phaseYSlot, bw, bh, bleft, btop)
				}
				return gb
			}
		}
		if math.Abs(gt[0]-gt[3]) < 0.01 {
			scale := 0.5 * (gt[0] + gt[3])
			dpi := int(math.Round(72 * scale))
			if dpi > 0 {
				if pr, ok := unwrapSplashPhasedRenderer(font); ok {
					phaseXSlot := splashGlyphXPhaseSlot(phaseX, font, fontSize, scale)
					phaseYSlot := splashGlyphYPhaseSlot(phaseY, font, fontSize, scale)
					phaseXQ := splashGlyphPhaseQ(phaseXSlot)
					phaseYQ := splashGlyphPhaseQ(phaseYSlot)
					key := glyphCacheKey{
						font:    font,
						glyph:   glyph,
						sizeQ:   int64(math.Round(fontSize * 64)),
						gtQ:     gtQ,
						phaseXQ: phaseXQ,
						phaseYQ: phaseYQ,
					}
					if gb, ok := c.glyphCache[key]; ok {
						return gb
					}
					buf, bw, bh, bleft, btop, err := pr.RenderGlyphBitmapPhased(glyph, fontSize, dpi, phaseXSlot, phaseYSlot)
					if err == nil && len(buf) > 0 && bw > 0 && bh > 0 {
						// FT returns bleft as offset right of origin where
						// bitmap left edge starts; btop as rows above baseline
						// at bitmap top. fillGlyph2 places (xStart, yStart) =
						// (Floor(xt) - g.X, Floor(yt) - g.Y); we want
						// xStart = Floor(xt) + bleft, yStart = Floor(yt) - btop
						// so g.X = -bleft, g.Y = btop.
						gb := &GlyphBitmap{
							X:    -bleft,
							Y:    btop,
							W:    bw,
							H:    bh,
							AA:   true,
							Data: buf,
						}
						c.glyphCache[key] = gb
						if os.Getenv("SPLASH_DEBUG_GT") != "" {
							fmt.Fprintf(os.Stderr, "fetchGlyph(FT) font=%s glyph=%d size=%.8f dpi=%d phase=(%.2f,%.2f) bw=%d bh=%d bleft=%d btop=%d\n",
								font.Name(), glyph, fontSize, dpi, phaseXSlot, phaseYSlot, bw, bh, bleft, btop)
						}
						return gb
					}
				} else if br, ok := unwrapSplashBitmapRenderer(font); ok {
					key := glyphCacheKey{
						font:    font,
						glyph:   glyph,
						sizeQ:   int64(math.Round(fontSize * 64)),
						gtQ:     gtQ,
						phaseXQ: -1,
						phaseYQ: -1,
					}
					if gb, ok := c.glyphCache[key]; ok {
						return gb
					}
					buf, bw, bh, bleft, btop, err := br.RenderGlyphBitmap(glyph, fontSize, dpi)
					if err == nil && len(buf) > 0 && bw > 0 && bh > 0 {
						gb := &GlyphBitmap{
							X:    -bleft,
							Y:    btop,
							W:    bw,
							H:    bh,
							AA:   true,
							Data: buf,
						}
						c.glyphCache[key] = gb
						return gb
					}
				}
			}
		}
	}

	// Path-based fallback (legacy P4-Dev1 behaviour).
	key := glyphCacheKey{
		font:    font,
		glyph:   glyph,
		sizeQ:   int64(math.Round(fontSize * 64)),
		gtQ:     gtQ,
		phaseXQ: -2, // distinct from FT cache slots
		phaseYQ: -2,
	}
	if gb, ok := c.glyphCache[key]; ok {
		return gb
	}
	gp, err := font.RenderGlyph(glyph, fontSize)
	if err != nil || gp == nil || len(gp.Commands) == 0 {
		c.glyphCache[key] = nil
		return nil
	}
	p := glyphPathToXPathWithTransform(gp, gt)
	if p.IsEmpty() {
		c.glyphCache[key] = nil
		return nil
	}
	gb := RasterizeGlyph(p, 1.0, false)
	if os.Getenv("SPLASH_DEBUG_GT") != "" {
		fmt.Fprintf(os.Stderr, "fetchGlyph(path) font=%s glyph=%d size=%.2f gt=%v gpW=?? gbW=%d gbH=%d\n", font.Name(), glyph, fontSize, gt, gb.W, gb.H)
	}
	c.glyphCache[key] = gb
	return gb
}

// unwrapSplashBitmapRenderer mirrors canvas.unwrapBitmapRenderer — splits the
// font wrapper chain to find a font with FreeType bitmap support.
func unwrapSplashBitmapRenderer(font entity.Font) (entity.BitmapGlyphRenderer, bool) {
	if br, ok := font.(entity.BitmapGlyphRenderer); ok {
		return br, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		if base := u.BaseFont(); base != nil {
			return unwrapSplashBitmapRenderer(base)
		}
	}
	return nil, false
}

// splashTransformedPhasedRenderer mirrors canvas.transformedPhasedGlyphRenderer —
// FreeType-backed fonts that accept separate scaleX/scaleY (matches poppler's
// SplashFTFont path).
type splashTransformedPhasedRenderer interface {
	RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error)
}

// splashMatrixPhasedRenderer mirrors Poppler's SplashFTFont full 2x2 matrix
// path for rotated or sheared glyphs.
type splashMatrixPhasedRenderer interface {
	RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error)
}

// unwrapSplashTransformedPhasedRenderer walks the BaseFont chain looking for a
// font that implements RenderGlyphBitmapTransformedPhased.
func unwrapSplashTransformedPhasedRenderer(font entity.Font) (splashTransformedPhasedRenderer, bool) {
	if pr, ok := font.(splashTransformedPhasedRenderer); ok {
		return pr, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		if base := u.BaseFont(); base != nil {
			return unwrapSplashTransformedPhasedRenderer(base)
		}
	}
	return nil, false
}

// unwrapSplashMatrixPhasedRenderer walks the BaseFont chain looking for a font
// that implements RenderGlyphBitmapMatrixPhased.
func unwrapSplashMatrixPhasedRenderer(font entity.Font) (splashMatrixPhasedRenderer, bool) {
	if pr, ok := font.(splashMatrixPhasedRenderer); ok {
		return pr, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		if base := u.BaseFont(); base != nil {
			return unwrapSplashMatrixPhasedRenderer(base)
		}
	}
	return nil, false
}

// unwrapSplashPhasedRenderer mirrors canvas.unwrapPhasedRenderer — splits the
// font wrapper chain to find a font with phased FreeType bitmap support.
func unwrapSplashPhasedRenderer(font entity.Font) (entity.BitmapGlyphRendererPhased, bool) {
	if pr, ok := font.(entity.BitmapGlyphRendererPhased); ok {
		return pr, true
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if u, ok := font.(baseUnwrapper); ok {
		if base := u.BaseFont(); base != nil {
			return unwrapSplashPhasedRenderer(base)
		}
	}
	return nil, false
}

// splashGlyphXPhaseSlot mirrors canvas.popplerGlyphXPhaseForFont — the
// FreeType phased renderer expects 0/0.25/0.5/0.75 sub-pixel offsets, and
// large glyphs (cache height > 50px) skip phasing for cache reuse parity
// with poppler (canvas/image_canvas_text.go:152).
func splashGlyphXPhaseSlot(x float64, font entity.Font, fontSize, scaleY float64) float64 {
	if splashGlyphCacheHeight(font, fontSize, scaleY) > 50 {
		return 0
	}
	frac := x - math.Floor(x)
	phaseSource := frac * 4
	if raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GLYPH_HALF_EPS")); raw != "" {
		if eps, err := strconv.ParseFloat(raw, 64); err == nil && eps > 0 && math.Abs(phaseSource-2) <= eps {
			phaseSource -= eps
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GLYPH_PHASE_EPS")); raw != "" {
		if eps, err := strconv.ParseFloat(raw, 64); err == nil && eps > 0 {
			phaseSource -= eps
		}
	}
	phase := math.Floor(phaseSource)
	if phase < 0 {
		return 0
	}
	if phase > 3 {
		return 0.75
	}
	slot := phase / 4
	if raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GLYPH_PHASE_FORCE")); raw != "" {
		if forced, err := strconv.ParseFloat(raw, 64); err == nil {
			if forced < 0 {
				return 0
			}
			if forced > 0.75 {
				return 0.75
			}
			return math.Floor(forced*4) / 4
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GLYPH_PHASE_BIAS")); raw != "" {
		if bias, err := strconv.ParseFloat(raw, 64); err == nil {
			slot += bias
			for slot < 0 {
				slot += 1
			}
			for slot >= 1 {
				slot -= 1
			}
			return math.Floor(slot*4) / 4
		}
	}
	return slot
}

func splashGlyphYPhaseSlot(y float64, font entity.Font, fontSize, scaleY float64) float64 {
	if os.Getenv("PDF_DEBUG_SPLASH_GLYPH_Y_PHASE") == "" {
		return 0
	}
	return splashGlyphXPhaseSlot(y, font, fontSize, scaleY)
}

func splashGlyphSnapCoord(v float64) float64 {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GLYPH_SNAP_EPS"))
	if raw == "" {
		return v
	}
	eps, err := strconv.ParseFloat(raw, 64)
	if err != nil || eps <= 0 {
		return v
	}
	nearest := math.Round(v)
	if math.Abs(v-nearest) <= eps {
		return nearest
	}
	return v
}

func splashGlyphPhaseQ(phase float64) int8 {
	q := int8(math.Round(phase * 4))
	if q < 0 {
		return 0
	}
	if q > 3 {
		return 3
	}
	return q
}

func splashGlyphCacheHeight(font entity.Font, fontSize, scaleY float64) float64 {
	if font == nil || fontSize <= 0 || scaleY <= 0 {
		return 0
	}
	if _, yMin, _, yMax, units, ok := splashPopplerGlyphCacheBBox(font); ok && units > 0 {
		scale := fontSize * scaleY / float64(units)
		y0 := int(yMin * scale)
		y1 := int(yMax * scale)
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		return float64(y1-y0) + 3
	}
	_, yMin, _, yMax := font.GetBoundingBox()
	unitsPerEm := float64(font.UnitsPerEm())
	if unitsPerEm <= 0 {
		unitsPerEm = 1000
	}
	scale := fontSize * scaleY / unitsPerEm
	y0 := int(yMin * scale)
	y1 := int(yMax * scale)
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	return float64(y1-y0) + 3
}

type splashPopplerGlyphCacheBBoxProvider interface {
	PopplerGlyphCacheBBox() (float64, float64, float64, float64, uint16, bool)
}

func splashPopplerGlyphCacheBBox(font entity.Font) (float64, float64, float64, float64, uint16, bool) {
	if provider, ok := font.(splashPopplerGlyphCacheBBoxProvider); ok {
		return provider.PopplerGlyphCacheBBox()
	}
	type baseUnwrapper interface {
		BaseFont() entity.Font
	}
	if unwrapper, ok := font.(baseUnwrapper); ok {
		if base := unwrapper.BaseFont(); base != nil {
			return splashPopplerGlyphCacheBBox(base)
		}
	}
	return 0, 0, 0, 0, 0, false
}

// glyphPathToXPath converts an entity.GlyphPath (already scaled to fontSize)
// into an xpath.Path used by RasterizeGlyph.
//
// IMPORTANT (P4 hotfix 2026-04-27): entity.Font.RenderGlyph returns coordinates
// in a Y-DOWN convention — apex of an 'A' has NEGATIVE Y, baseline at Y=0
// (mirrors freetype/sfnt conventions when paired with the legacy
// canvas.glyphTransform whose gt[3] is positive). Splash's bitmap is also
// Y-down. The two conventions match — we MUST NOT negate Y here.
//
// The earlier P4-Dev1 implementation negated Y, which produced an upside-down
// glyph bitmap (apex at the bottom row of the cache); when Splash.fillGlyph
// then placed g.Y=0 at flipY(curY) the entire glyph also rendered BELOW the
// baseline, yielding the 180°-rotated text seen in Phase 4 corpus output.
func glyphPathToXPath(gp *entity.GlyphPath) *xpath.Path {
	return glyphPathToXPathWithTransform(gp, [4]float64{1, 0, 0, 1})
}

// glyphPathToXPathWithTransform converts an entity.GlyphPath to an xpath.Path,
// applying the linear glyph transform gt = [a,b,c,d] so glyph coordinates land
// in device pixels (matches ImageCanvas.renderGlyphPath transformGlyphPoint at
// image_canvas_text.go:231). Without this the path stays at PDF user-space
// scale while glyph origins are placed in device pixels, producing glyphs at
// 72/dpi of their intended size and proportionally wide letter spacing.
func glyphPathToXPathWithTransform(gp *entity.GlyphPath, gt [4]float64) *xpath.Path {
	out := xpath.NewPath()
	if gp == nil {
		return out
	}
	tx := func(x, y float64) (float64, float64) {
		return gt[0]*x + gt[2]*y, gt[1]*x + gt[3]*y
	}
	for _, cmd := range gp.Commands {
		switch v := cmd.(type) {
		case *entity.PathMoveTo:
			x, y := tx(v.X, v.Y)
			_ = out.MoveTo(x, y)
		case *entity.PathLineTo:
			x, y := tx(v.X, v.Y)
			_ = out.LineTo(x, y)
		case *entity.PathCurveTo:
			x1, y1 := tx(v.X1, v.Y1)
			x2, y2 := tx(v.X2, v.Y2)
			x3, y3 := tx(v.X3, v.Y3)
			_ = out.CurveTo(x1, y1, x2, y2, x3, y3)
		case *entity.PathClose:
			_ = out.Close(true)
		}
	}
	return out
}

// splashSplitTextCodes mirrors canvas.splitTextCodes — CIDFont strings carry
// 2-byte codes, simple fonts carry 1-byte (PDF 1.7 §9.7.6.3).
func splashSplitTextCodes(raw []byte, font entity.Font) []uint32 {
	if len(raw) == 0 {
		return nil
	}
	if font != nil && font.IsCIDFont() {
		out := make([]uint32, 0, (len(raw)+1)/2)
		for i := 0; i < len(raw); {
			if i+1 < len(raw) {
				out = append(out, uint32(raw[i])<<8|uint32(raw[i+1]))
				i += 2
				continue
			}
			out = append(out, uint32(raw[i]))
			i++
		}
		return out
	}
	out := make([]uint32, 0, len(raw))
	for _, b := range raw {
		out = append(out, uint32(b))
	}
	return out
}

// DrawImage routes a stdlib image.Image into Splash::drawImage (Splash.cc:4090).
//
// The image is streamed row-by-row through an ImageSource closure into
// DrawImageImpl. The placement matrix is built so that source space [0,1]² maps
// to splash device space, composing three transforms (P4 image-pipeline fix
// 2026-04-27 — see memory image_pipeline_ctm_2026_04_27):
//
//  1. unitMat = [width, 0, 0, height, x, y] — places the image rectangle within
//     the source-space rect emitted by the evaluator (it uses (0,0,1,1) so the
//     evaluator's CTM owns the entire placement; the (x,y,w,h) parameters are
//     here for direct callers that bypass the evaluator).
//  2. canvasCTM = c.s.state.matrix — the imageCTM the evaluator stored via the
//     wrapping Save / Transform(imageCTM) / Restore around DrawImage in
//     image_rendering.go:740. Without composing this in, the image renders at
//     1×1 device pixel because the unitMat alone is identity.
//  3. Y-flip (a, -b, c, -d, e, yOrigin - f) — the canvasCTM is in PDF Y-up
//     device-pixel space; splash bitmap is Y-down. Folding the flip into the
//     matrix lets DrawImageImpl pick the mat[3]<0 vertical-flip kernel
//     (Splash.cc:3581) without a separate transpose pass.
func (c *splashCanvas) DrawImage(img image.Image, x, y, width, height float64, interpolate bool) error {
	// Poppler's regular SplashOutputDev::drawImage passes srcAlpha=false.
	// PDF alpha is handled by SMask/Mask-specific paths instead of borrowing the
	// destination bitmap alpha channel.
	return c.drawImage(img, x, y, width, height, interpolate, false)
}

func (c *splashCanvas) drawImage(
	img image.Image,
	x, y, width, height float64,
	interpolate bool,
	sourceAlpha bool,
) error {
	if img == nil || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}
	mode := c.s.bitmap.Mode()
	nComps := nCompsForMode(mode)
	if nComps == 0 {
		return ErrModeMismatch
	}

	flipSourceY := !c.inType3Glyph()
	src := func(row int, colorLine, alphaLine []byte) error {
		if row < 0 || row >= srcH {
			return nil
		}
		// PDF image space has origin at bottom-left (Y-up); stdlib image.Image
		// has row 0 at the top. Mirror image_canvas's flippedImage{} wrapper
		// (image_canvas_image_fastpath.go:710) so the (0,1)² source mapping
		// matches the evaluator's CTM convention. Without this the smile.png
		// from 007-imagemagick lands upside-down on the splash bitmap.
		srcY := bounds.Min.Y + row
		if flipSourceY {
			srcY = bounds.Min.Y + (srcH - 1 - row)
		}
		for col := 0; col < srcW; col++ {
			r16, g16, b16, a16 := img.At(bounds.Min.X+col, srcY).RGBA()
			r8 := byte(r16 >> 8)
			g8 := byte(g16 >> 8)
			b8 := byte(b16 >> 8)
			a8 := byte(a16 >> 8)
			if !sourceAlpha && a8 != 0 && a8 != 0xFF {
				r8 = unpremultiplyByte(r8, a8)
				g8 = unpremultiplyByte(g8, a8)
				b8 = unpremultiplyByte(b8, a8)
			}
			switch mode {
			case ModeMono8:
				y := (2126*uint32(r8) + 7152*uint32(g8) + 722*uint32(b8) + 5000) / 10000
				if y > 0xFF {
					y = 0xFF
				}
				colorLine[col] = byte(y)
			case ModeBGR8:
				colorLine[col*3+0] = b8
				colorLine[col*3+1] = g8
				colorLine[col*3+2] = r8
			case ModeXBGR8:
				colorLine[col*4+0] = b8
				colorLine[col*4+1] = g8
				colorLine[col*4+2] = r8
				colorLine[col*4+3] = 0
			case ModeCMYK8:
				k := uint32(0xFF)
				if uint32(r8) < k {
					k = uint32(r8)
				}
				if uint32(g8) < k {
					k = uint32(g8)
				}
				if uint32(b8) < k {
					k = uint32(b8)
				}
				colorLine[col*4+0] = byte(0xFF - r8)
				colorLine[col*4+1] = byte(0xFF - g8)
				colorLine[col*4+2] = byte(0xFF - b8)
				colorLine[col*4+3] = byte(0xFF - k)
			default: // ModeRGB8 (and DeviceN8 fallback)
				colorLine[col*3+0] = r8
				colorLine[col*3+1] = g8
				colorLine[col*3+2] = b8
			}
			if alphaLine != nil {
				alphaLine[col] = a8
			}
		}
		return nil
	}

	// Compose unit mat → canvasCTM → Y-flip into the single image-to-device
	// matrix expected by DrawImageImpl.
	unitMat := [6]float64{width, 0, 0, height, x, y}
	canvasCTM := c.s.state.matrix
	composed := composeMatrix(canvasCTM, unitMat)
	mat := c.imageDrawMatrix(composed)
	return c.s.drawImageImpl(src, srcW, srcH, mat, interpolate, sourceAlpha)
}

// DrawImageWithSoftMaskPhaseSamplerAndEdgeMode mirrors
// SplashOutputDev::drawSoftMaskedImage: render the SMask into a page-sized
// Mono8 bitmap, install it as Splash's soft mask, then draw the source image.
func (c *splashCanvas) DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(
	img image.Image,
	mask domainimage.ImageMask,
	x, y, width, height float64,
	interpolate bool,
	_ string,
	_, _ float64,
	_ string,
) error {
	if img == nil || mask == nil || mask.Image() == nil || c.s == nil || c.s.bitmap == nil {
		return c.DrawImage(img, x, y, width, height, interpolate)
	}

	maskImg := mask.Image()
	maskBounds := maskImg.Bounds()
	maskW, maskH := maskBounds.Dx(), maskBounds.Dy()
	if maskW <= 0 || maskH <= 0 {
		return c.DrawImage(img, x, y, width, height, interpolate)
	}
	maskBitmap := NewBitmap(c.width, c.height, ModeMono8, false)
	if maskBitmap == nil || maskBitmap.data == nil {
		return ErrBadArg
	}
	maskSplash, err := New(maskBitmap, c.s.vectorAA)
	if err != nil {
		return err
	}
	maskSplash.downscaleVFlipTopDown = true
	maskBitmap.Clear(Color{})

	flipSourceY := !c.inType3Glyph()
	maskSrc := func(row int, colorLine, _ []byte) error {
		if row < 0 || row >= maskH {
			return nil
		}
		srcY := maskBounds.Min.Y + row
		if flipSourceY {
			srcY = maskBounds.Min.Y + (maskH - 1 - row)
		}
		for col := 0; col < maskW; col++ {
			gray := color.GrayModel.Convert(maskImg.At(maskBounds.Min.X+col, srcY)).(color.Gray).Y
			if mask.IsInverted() {
				gray = 0xFF - gray
			}
			colorLine[col] = gray
		}
		return nil
	}

	unitMat := [6]float64{width, 0, 0, height, x, y}
	composed := composeMatrix(c.s.state.matrix, unitMat)
	mat := c.imageDrawMatrix(composed)
	if err := maskSplash.DrawImageImpl(maskSrc, maskW, maskH, mat, interpolate); err != nil {
		return err
	}

	prevMask := c.s.state.softMask
	prevDownscaleVFlipTopDown := c.s.downscaleVFlipTopDown
	c.s.SetSoftMask(maskBitmap)
	c.s.downscaleVFlipTopDown = true
	defer c.s.SetSoftMask(prevMask)
	defer func() {
		c.s.downscaleVFlipTopDown = prevDownscaleVFlipTopDown
	}()
	return c.drawImage(img, x, y, width, height, interpolate, false)
}

// composeMatrix returns the column-major composition outer ∘ inner — i.e. the
// transform that applies inner first, then outer (PDF column-vector convention,
// Splash.cc:1539 SplashMatrixMultiply).
func composeMatrix(outer, inner [6]float64) [6]float64 {
	return [6]float64{
		outer[0]*inner[0] + outer[2]*inner[1],
		outer[1]*inner[0] + outer[3]*inner[1],
		outer[0]*inner[2] + outer[2]*inner[3],
		outer[1]*inner[2] + outer[3]*inner[3],
		outer[0]*inner[4] + outer[2]*inner[5] + outer[4],
		outer[1]*inner[4] + outer[3]*inner[5] + outer[5],
	}
}

// flipYMatrix folds [1, 0, 0, -1, 0, yOrigin] into the LEFT side of m, so the
// composed matrix maps PDF Y-up device-pixel space to splash Y-down bitmap
// pixels in one step (mirrors the per-vertex flipY done in MoveTo/LineTo).
func flipYMatrix(m [6]float64, yOrigin float64) [6]float64 {
	return [6]float64{m[0], -m[1], m[2], -m[3], m[4], yOrigin - m[5]}
}

func splashTransformPoint(m [6]float64, x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

func (c *splashCanvas) pathYFlipMatrix() [6]float64 {
	return [6]float64{1, 0, 0, -1, 0, c.flipYOrigin()}
}

func popplerImageMatrixFromCTM(ctm [6]float64) [6]float64 {
	return [6]float64{
		ctm[0],
		ctm[1],
		-ctm[2],
		-ctm[3],
		ctm[2] + ctm[4],
		ctm[3] + ctm[5],
	}
}

func (c *splashCanvas) imageDrawMatrix(composed [6]float64) [6]float64 {
	if c.inType3Glyph() {
		// Poppler evaluates Type3 CharProcs with a device-space CTM, then
		// SplashOutputDev::drawSoftMaskedImage converts that CTM to a Splash
		// image matrix with [a b -c -d c+e d+f]. Our evaluator keeps CTMs Y-up,
		// so first convert to Poppler's Y-down CTM with the exact page origin.
		return popplerImageMatrixFromCTM(flipYMatrix(composed, c.flipYOrigin()))
	}
	return flipYMatrix(composed, c.flipYOrigin())
}

func (c *splashCanvas) inType3Glyph() bool {
	return c.type3Depth > 0
}

// flipYOrigin returns the float page Y origin used to convert PDF Y-up coords
// to splash Y-down — preferring the renderer-supplied float origin over the
// integer canvas height (mirrors flipY()).
func (c *splashCanvas) flipYOrigin() float64 {
	if c.pageYOriginPx > 0 {
		return c.pageYOriginPx
	}
	return float64(c.height)
}

func (c *splashCanvas) imageFlipYOrigin() float64 {
	return c.flipYOrigin()
}

// BeginType3Glyph marks entry into a Type3 CharProc replay.
func (c *splashCanvas) BeginType3Glyph() { c.type3Depth++ }

// EndType3Glyph marks exit from a Type3 CharProc replay.
func (c *splashCanvas) EndType3Glyph() {
	if c.type3Depth > 0 {
		c.type3Depth--
	}
}

// Save pushes the graphics state (Splash::saveState, Splash.cc:1737).
func (c *splashCanvas) Save() { c.s.SaveState() }

// Restore pops the graphics state (Splash::restoreState, Splash.cc:1746).
func (c *splashCanvas) Restore() { _ = c.s.RestoreState() }

// Transform replaces the CTM (Splash::setMatrix, Splash.cc:1551). The CTM is
// only consulted by image rendering; path/fill/stroke ops receive device-space
// coordinates from the evaluator (already multiplied by graphics.transform),
// so for the splash backend the matrix passed in is effectively the residual
// nonlinear part needed by image fast paths and we forward it as-is.
func (c *splashCanvas) Transform(matrix [6]float64) { c.s.SetMatrix(matrix) }

// SetPageYOriginPx records the float page height in canvas pixels. Mirrors
// the same hook image_canvas exposes — the concurrent renderer calls this
// before evaluating page contents so the path methods below can flip
// PDF-bottom-up coords into the splash bitmap's top-down axis.
func (c *splashCanvas) SetPageYOriginPx(yOrigin float64) {
	c.pageYOriginPx = yOrigin
}

// flipY converts an evaluator-space Y (device pixel space, but with Y still
// pointing up because the renderer's initial CTM does not flip Y — see
// concurrent_renderer.go default branch tY=0) into splash-bitmap Y.
func (c *splashCanvas) flipY(y float64) float64 {
	if c.pageYOriginPx > 0 {
		return c.pageYOriginPx - y
	}
	return float64(c.height) - y
}

// QuantizeType3GlyphOrigin mirrors Poppler's Type3 cache blit placement.
// SplashOutputDev::drawType3Glyph() calls Splash::fillGlyph(0,0), which floors
// the transformed glyph origin before blitting the cached glyph bitmap.
func (c *splashCanvas) QuantizeType3GlyphOrigin(x, y float64) (float64, float64) {
	yDown := c.flipYOrigin() - y
	return math.Floor(x), c.flipYOrigin() - math.Floor(yDown)
}

// SetFillColor installs a SolidColor fill pattern from a stdlib color.Color
// (Splash::setFillPattern, Splash.cc:1601). Hotfix #2 wired this away from
// the Phase 0 stub: without it the renderer's `canvas.SetFillColor(black)`
// left state.fillPattern==nil and Fill emitted zero bytes.
func (c *splashCanvas) SetFillColor(col color.Color) {
	if c.s == nil || c.s.bitmap == nil {
		return
	}
	c.pendingFillTilingPattern = nil
	c.pendingFillTilingTint = Color{}
	c.pendingFillShadingPattern = nil
	sc, alpha := convertColorAndAlpha(col, c.s.bitmap.Mode())
	c.s.SetFillPattern(NewSolidColor(sc))
	c.s.SetFillAlpha(alpha)
}

// SetStrokeColor installs a SolidColor stroke pattern from a stdlib
// color.Color (Splash::setStrokePattern, Splash.cc:1595). Mirror of
// SetFillColor — see Hotfix #2 notes there.
func (c *splashCanvas) SetStrokeColor(col color.Color) {
	if c.s == nil || c.s.bitmap == nil {
		return
	}
	sc, alpha := convertColorAndAlpha(col, c.s.bitmap.Mode())
	c.s.SetStrokePattern(NewSolidColor(sc))
	c.s.SetStrokeAlpha(alpha)
}

// SetLineWidth forwards to Splash (Splash.cc:1626).
func (c *splashCanvas) SetLineWidth(width float64) { c.s.SetLineWidth(width) }

// SetLineCap forwards to Splash (Splash.cc:1644).
func (c *splashCanvas) SetLineCap(cap int) { c.s.SetLineCap(cap) }

// SetLineJoin forwards to Splash (Splash.cc:1650).
func (c *splashCanvas) SetLineJoin(join int) { c.s.SetLineJoin(join) }

// SetMiterLimit forwards to Splash (Splash.cc:1632).
func (c *splashCanvas) SetMiterLimit(limit float64) { c.s.SetMiterLimit(limit) }

// SetDashPattern forwards to Splash (Splash.cc:1656).
func (c *splashCanvas) SetDashPattern(dash []float64, phase float64) { c.s.SetLineDash(dash, phase) }

// SetFillPattern installs a domain Pattern as the fill source for subsequent
// Fill calls (Splash::setFillPattern, Splash.cc:1601).
//
// Patterns flow through two paths in the renderer:
//   - Shading patterns invoked by the `sh` operator come straight to
//     DrawShadingPattern below, which routes to FillAxialShading /
//     FillRadialShading / FillGouraudTriangleShadedFill directly — they do
//     NOT pass through SetFillPattern.
//   - Tiling patterns invoked by `scn` + `f` set the pattern here and rely on
//     the next Fill() to honour it. We translate axial/radial shading patterns
//     into splash shaders here as well.
//
// IMPORTANT (P4 hotfix 2026-04-27): when pattern == nil this MUST be a no-op.
// renderer.Evaluator.syncCanvasColors calls SetFillPattern unconditionally
// after SetFillColor; passing nil here used to fall through and overwrite the
// fillPattern with a BLACK solid, producing the all-black 011-google-doc and
// 022-pdfkit pages observed in the corpus. Mirror image_canvas's nil
// passthrough so the SetFillColor solid we just installed survives.
func (c *splashCanvas) SetFillPattern(pattern entity.Pattern) {
	if c.s == nil || c.s.bitmap == nil {
		return
	}
	if pattern == nil {
		// Preserve the fillPattern installed by SetFillColor — image_canvas
		// stores nil here as a marker; for splash the equivalent is "leave the
		// solid color pattern untouched".
		c.pendingFillTilingPattern = nil
		c.pendingFillTilingTint = Color{}
		c.pendingFillShadingPattern = nil
		return
	}
	mode := c.s.bitmap.Mode()
	if tp, ok := pattern.(*entity.TilingPattern); ok && tp != nil {
		tint := c.currentFillPatternColor(mode)
		c.pendingFillTilingPattern = tp
		c.pendingFillTilingTint = tint
		c.pendingFillShadingPattern = nil
		if splashPattern := c.buildSplashTilingPattern(tp, tint); splashPattern != nil {
			c.s.SetFillPattern(splashPattern)
		}
		return
	}
	c.pendingFillTilingPattern = nil
	c.pendingFillTilingTint = Color{}
	c.pendingFillShadingPattern = nil
	if shp, ok := pattern.(*entity.ShadingPattern); ok && shp != nil {
		if shading := shp.GetShading(); shading != nil {
			switch shading.GetShadingType() {
			case entity.ShadingAxial:
				if shader, err := c.buildAxialPatternShader(shading, shp.Matrix(), mode); err == nil {
					c.s.SetFillPattern(shader)
					return
				}
			case entity.ShadingRadial:
				if shader, err := c.buildRadialPatternShader(shading, shp.Matrix(), mode); err == nil {
					c.s.SetFillPattern(shader)
					return
				}
			case entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
				if len(shading.GetPatches()) > 0 {
					c.pendingFillShadingPattern = shp
					return
				}
			case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud:
				if shader, err := c.buildGouraudPatternShader(shading, shp.Matrix(), mode); err == nil {
					c.pendingFillShadingPattern = shp
					c.s.SetFillPattern(shader)
					return
				}
			}
		}
	}
	// Unknown non-nil patterns leave the solid fill pattern from SetFillColor in
	// place. Clobbering with black-solid here would lose the user's color for any
	// subsequent solid Fill() that races the pattern install.
}

func (c *splashCanvas) fillPendingShadingPattern(evenOdd bool) bool {
	if c == nil || c.s == nil || c.s.bitmap == nil || c.path == nil || c.path.Length() == 0 {
		return false
	}
	pattern := c.pendingFillShadingPattern
	if pattern == nil {
		return false
	}
	shading := pattern.GetShading()
	if shading == nil {
		return false
	}
	switch shading.GetShadingType() {
	case entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
		return c.fillPatchMeshShadingPattern(pattern, evenOdd)
	case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud:
	default:
		return false
	}
	shader, err := c.buildGouraudPatternShader(shading, pattern.Matrix(), c.s.bitmap.Mode())
	if err != nil || shader == nil {
		return false
	}

	c.s.SaveState()
	if err := c.s.ClipToPath(c.path, evenOdd); err != nil {
		_ = c.s.RestoreState()
		return false
	}
	if err := c.s.FillGouraudTriangleShadedFill(shader, nil, false); err != nil {
		_ = c.s.RestoreState()
		return false
	}
	_ = c.s.RestoreState()
	return true
}

func (c *splashCanvas) fillPatchMeshShadingPattern(pattern *entity.ShadingPattern, evenOdd bool) bool {
	if c == nil || c.s == nil || c.path == nil || c.path.Length() == 0 || pattern == nil {
		return false
	}
	shading := pattern.GetShading()
	if shading == nil || len(shading.GetPatches()) == 0 {
		return false
	}
	if os.Getenv("PDF_DEBUG_SPLASH_PATCH_TRACE") != "" {
		first := shading.GetPatches()[0]
		fmt.Fprintf(os.Stderr, "SPLASH_PATCH_TRACE pattern=%s type=%s patches=%d matrix=%v c00=%v c01=%v c11=%v c10=%v\n",
			pattern.Name(), shading.GetShadingType(), len(shading.GetPatches()), pattern.Matrix(),
			first.Colors[0][0], first.Colors[0][1], first.Colors[1][1], first.Colors[1][0])
	}
	c.s.SaveState()
	if err := c.s.ClipToPath(c.path, evenOdd); err != nil {
		_ = c.s.RestoreState()
		return false
	}
	oldVectorAA := c.s.vectorAA
	c.s.vectorAA = false
	defer func() {
		c.s.vectorAA = oldVectorAA
	}()
	if err := c.fillPatchMeshShading(pattern); err != nil {
		_ = c.s.RestoreState()
		return false
	}
	_ = c.s.RestoreState()
	return true
}

func (c *splashCanvas) fillPatchMeshShading(pattern *entity.ShadingPattern) error {
	if pattern == nil || pattern.GetShading() == nil {
		return fmt.Errorf("splashCanvas: patch mesh shading pattern is nil")
	}
	shading := pattern.GetShading()
	patches := shading.GetPatches()
	startDepth := patchMeshInitialDepth(len(patches))
	for _, patch := range patches {
		if err := c.fillPatchMeshPatch(shading, patch, pattern.Matrix(), startDepth); err != nil {
			return err
		}
	}
	return nil
}

func patchMeshInitialDepth(patches int) int {
	switch {
	case patches > 128:
		return 3
	case patches > 64:
		return 2
	case patches > 16:
		return 1
	default:
		return 0
	}
}

func (c *splashCanvas) fillPatchMeshPatch(shading *entity.Shading, patch entity.Patch, matrix [6]float64, depth int) error {
	const (
		patchMaxDepth   = 6
		patchColorDelta = 3.0 / 256.0
		parameterDelta  = 5e-3
	)
	colorComps := len(patch.Colors[0][0])
	if colorComps == 0 {
		return nil
	}
	threshold := patchColorDelta
	if len(shading.GetFunctions()) > 0 {
		threshold = parameterDelta
		colorComps = 1
	}
	flat := depth >= patchMaxDepth
	for i := 0; i < colorComps && !flat; i++ {
		c00 := patch.Colors[0][0][i]
		c01 := patch.Colors[0][1][i]
		c11 := patch.Colors[1][1][i]
		c10 := patch.Colors[1][0][i]
		if math.Abs(c00-c01) > threshold || math.Abs(c01-c11) > threshold ||
			math.Abs(c11-c10) > threshold || math.Abs(c10-c00) > threshold {
			break
		}
		if i == colorComps-1 {
			flat = true
		}
	}
	if flat {
		col, ok := c.patchMeshColor(shading, patch.Colors[0][0])
		if !ok {
			return nil
		}
		c.s.SetFillPattern(NewSolidColor(col))
		return c.s.Fill(c.patchMeshPath(patch, matrix), false)
	}
	p00, p10, p01, p11 := splitPatchMeshPatch(patch, colorComps)
	if err := c.fillPatchMeshPatch(shading, p00, matrix, depth+1); err != nil {
		return err
	}
	if err := c.fillPatchMeshPatch(shading, p10, matrix, depth+1); err != nil {
		return err
	}
	if err := c.fillPatchMeshPatch(shading, p01, matrix, depth+1); err != nil {
		return err
	}
	return c.fillPatchMeshPatch(shading, p11, matrix, depth+1)
}

func (c *splashCanvas) patchMeshColor(shading *entity.Shading, values []float64) (Color, bool) {
	if shading == nil || len(values) == 0 || c == nil || c.s == nil || c.s.bitmap == nil {
		return Color{}, false
	}
	colors := values
	if len(shading.GetFunctions()) > 0 {
		evaluated, err := evalShadingFunctions(shading.GetFunctions(), []float64{values[0]})
		if err != nil || len(evaluated) == 0 {
			return Color{}, false
		}
		colors = evaluated
	}
	return packShadingOutput(colors, shading.GetColorSpace(), c.s.bitmap.Mode()), true
}

func (c *splashCanvas) patchMeshPath(patch entity.Patch, matrix [6]float64) *xpath.Path {
	p := xpath.NewPath()
	x, y := c.transformPatchMeshPoint(patch.X[0][0], patch.Y[0][0], matrix)
	_ = p.MoveTo(x, y)
	x1, y1 := c.transformPatchMeshPoint(patch.X[0][1], patch.Y[0][1], matrix)
	x2, y2 := c.transformPatchMeshPoint(patch.X[0][2], patch.Y[0][2], matrix)
	x3, y3 := c.transformPatchMeshPoint(patch.X[0][3], patch.Y[0][3], matrix)
	_ = p.CurveTo(x1, y1, x2, y2, x3, y3)
	x1, y1 = c.transformPatchMeshPoint(patch.X[1][3], patch.Y[1][3], matrix)
	x2, y2 = c.transformPatchMeshPoint(patch.X[2][3], patch.Y[2][3], matrix)
	x3, y3 = c.transformPatchMeshPoint(patch.X[3][3], patch.Y[3][3], matrix)
	_ = p.CurveTo(x1, y1, x2, y2, x3, y3)
	x1, y1 = c.transformPatchMeshPoint(patch.X[3][2], patch.Y[3][2], matrix)
	x2, y2 = c.transformPatchMeshPoint(patch.X[3][1], patch.Y[3][1], matrix)
	x3, y3 = c.transformPatchMeshPoint(patch.X[3][0], patch.Y[3][0], matrix)
	_ = p.CurveTo(x1, y1, x2, y2, x3, y3)
	x1, y1 = c.transformPatchMeshPoint(patch.X[2][0], patch.Y[2][0], matrix)
	x2, y2 = c.transformPatchMeshPoint(patch.X[1][0], patch.Y[1][0], matrix)
	x3, y3 = c.transformPatchMeshPoint(patch.X[0][0], patch.Y[0][0], matrix)
	_ = p.CurveTo(x1, y1, x2, y2, x3, y3)
	_ = p.Close(false)
	return p
}

func (c *splashCanvas) transformPatchMeshPoint(x, y float64, matrix [6]float64) (float64, float64) {
	tx := x*matrix[0] + y*matrix[2] + matrix[4]
	ty := x*matrix[1] + y*matrix[3] + matrix[5]
	return tx, c.flipY(ty)
}

func splitPatchMeshPatch(patch entity.Patch, colorComps int) (entity.Patch, entity.Patch, entity.Patch, entity.Patch) {
	var p00, p10, p01, p11 entity.Patch
	var xx, yy [4][8]float64
	for i := 0; i < 4; i++ {
		xx[i][0], yy[i][0] = patch.X[i][0], patch.Y[i][0]
		xx[i][1], yy[i][1] = 0.5*(patch.X[i][0]+patch.X[i][1]), 0.5*(patch.Y[i][0]+patch.Y[i][1])
		xxm, yym := 0.5*(patch.X[i][1]+patch.X[i][2]), 0.5*(patch.Y[i][1]+patch.Y[i][2])
		xx[i][6], yy[i][6] = 0.5*(patch.X[i][2]+patch.X[i][3]), 0.5*(patch.Y[i][2]+patch.Y[i][3])
		xx[i][2], yy[i][2] = 0.5*(xx[i][1]+xxm), 0.5*(yy[i][1]+yym)
		xx[i][5], yy[i][5] = 0.5*(xxm+xx[i][6]), 0.5*(yym+yy[i][6])
		xx[i][3], yy[i][3] = 0.5*(xx[i][2]+xx[i][5]), 0.5*(yy[i][2]+yy[i][5])
		xx[i][4], yy[i][4] = xx[i][3], yy[i][3]
		xx[i][7], yy[i][7] = patch.X[i][3], patch.Y[i][3]
	}
	for i := 0; i < 4; i++ {
		p00.X[0][i], p00.Y[0][i] = xx[0][i], yy[0][i]
		p00.X[1][i], p00.Y[1][i] = 0.5*(xx[0][i]+xx[1][i]), 0.5*(yy[0][i]+yy[1][i])
		xxm, yym := 0.5*(xx[1][i]+xx[2][i]), 0.5*(yy[1][i]+yy[2][i])
		p10.X[2][i], p10.Y[2][i] = 0.5*(xx[2][i]+xx[3][i]), 0.5*(yy[2][i]+yy[3][i])
		p00.X[2][i], p00.Y[2][i] = 0.5*(p00.X[1][i]+xxm), 0.5*(p00.Y[1][i]+yym)
		p10.X[1][i], p10.Y[1][i] = 0.5*(xxm+p10.X[2][i]), 0.5*(yym+p10.Y[2][i])
		p00.X[3][i], p00.Y[3][i] = 0.5*(p00.X[2][i]+p10.X[1][i]), 0.5*(p00.Y[2][i]+p10.Y[1][i])
		p10.X[0][i], p10.Y[0][i] = p00.X[3][i], p00.Y[3][i]
		p10.X[3][i], p10.Y[3][i] = xx[3][i], yy[3][i]
	}
	for i := 4; i < 8; i++ {
		j := i - 4
		p01.X[0][j], p01.Y[0][j] = xx[0][i], yy[0][i]
		p01.X[1][j], p01.Y[1][j] = 0.5*(xx[0][i]+xx[1][i]), 0.5*(yy[0][i]+yy[1][i])
		xxm, yym := 0.5*(xx[1][i]+xx[2][i]), 0.5*(yy[1][i]+yy[2][i])
		p11.X[2][j], p11.Y[2][j] = 0.5*(xx[2][i]+xx[3][i]), 0.5*(yy[2][i]+yy[3][i])
		p01.X[2][j], p01.Y[2][j] = 0.5*(p01.X[1][j]+xxm), 0.5*(p01.Y[1][j]+yym)
		p11.X[1][j], p11.Y[1][j] = 0.5*(xxm+p11.X[2][j]), 0.5*(yym+p11.Y[2][j])
		p01.X[3][j], p01.Y[3][j] = 0.5*(p01.X[2][j]+p11.X[1][j]), 0.5*(p01.Y[2][j]+p11.Y[1][j])
		p11.X[0][j], p11.Y[0][j] = p01.X[3][j], p01.Y[3][j]
		p11.X[3][j], p11.Y[3][j] = xx[3][i], yy[3][i]
	}
	for i := 0; i < colorComps; i++ {
		setPatchColorComp(&p00, 0, 0, i, patch.Colors[0][0][i])
		setPatchColorComp(&p00, 0, 1, i, (patch.Colors[0][0][i]+patch.Colors[0][1][i])/2)
		setPatchColorComp(&p01, 0, 0, i, p00.Colors[0][1][i])
		setPatchColorComp(&p01, 0, 1, i, patch.Colors[0][1][i])
		setPatchColorComp(&p01, 1, 1, i, (patch.Colors[0][1][i]+patch.Colors[1][1][i])/2)
		setPatchColorComp(&p11, 0, 1, i, p01.Colors[1][1][i])
		setPatchColorComp(&p11, 1, 1, i, patch.Colors[1][1][i])
		setPatchColorComp(&p11, 1, 0, i, (patch.Colors[1][1][i]+patch.Colors[1][0][i])/2)
		setPatchColorComp(&p10, 1, 1, i, p11.Colors[1][0][i])
		setPatchColorComp(&p10, 1, 0, i, patch.Colors[1][0][i])
		setPatchColorComp(&p10, 0, 0, i, (patch.Colors[1][0][i]+patch.Colors[0][0][i])/2)
		setPatchColorComp(&p00, 1, 0, i, p10.Colors[0][0][i])
		setPatchColorComp(&p00, 1, 1, i, (p00.Colors[1][0][i]+p01.Colors[1][1][i])/2)
		setPatchColorComp(&p01, 1, 0, i, p00.Colors[1][1][i])
		setPatchColorComp(&p11, 0, 0, i, p00.Colors[1][1][i])
		setPatchColorComp(&p10, 0, 1, i, p00.Colors[1][1][i])
	}
	return p00, p10, p01, p11
}

func setPatchColorComp(p *entity.Patch, i, j, comp int, value float64) {
	if len(p.Colors[i][j]) <= comp {
		next := make([]float64, comp+1)
		copy(next, p.Colors[i][j])
		p.Colors[i][j] = next
	}
	p.Colors[i][j][comp] = value
}

// SetStrokePattern mirrors SetFillPattern for stroke ops (Splash.cc:1595).
//
// Same routing logic as SetFillPattern: shading patterns get translated into
// splash shaders; nil and other pattern kinds leave the SetStrokeColor solid
// in place.
func (c *splashCanvas) SetStrokePattern(pattern entity.Pattern) {
	if c.s == nil || c.s.bitmap == nil {
		return
	}
	if pattern == nil {
		return
	}
	mode := c.s.bitmap.Mode()
	if shp, ok := pattern.(*entity.ShadingPattern); ok && shp != nil {
		if shading := shp.GetShading(); shading != nil {
			switch shading.GetShadingType() {
			case entity.ShadingAxial:
				if shader, err := c.buildAxialPatternShader(shading, shp.Matrix(), mode); err == nil {
					c.s.SetStrokePattern(shader)
					return
				}
			case entity.ShadingRadial:
				if shader, err := c.buildRadialPatternShader(shading, shp.Matrix(), mode); err == nil {
					c.s.SetStrokePattern(shader)
					return
				}
			case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud,
				entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
				if shader, err := c.buildGouraudPatternShader(shading, shp.Matrix(), mode); err == nil {
					c.s.SetStrokePattern(shader)
					return
				}
			}
		}
	}
}

// DrawTilingPattern wires entity.TilingPattern → Splash.FillWithTilingPattern
// (PDF 1.7 §8.7.3.3, Splash.cc:2324 fill driver; SplashOutputDev.cc:doTiling).
//
// P4-Dev4 (Phase 4): the cell content stream is now executed by recursing
// through a fresh renderer.Evaluator into a sub-splashCanvas. The resulting
// cell bitmap is wrapped as splash.TilingPattern and the bbox is filled by
// FillWithTilingPattern. When the pattern carries no content (or evaluation
// fails) we fall back to the diagonal-stripe synth cell from Phase 3 so the
// fill remains visible.
func (c *splashCanvas) currentFillPatternColor(mode ColorMode) Color {
	if c != nil && c.s != nil {
		if solid, ok := c.s.state.fillPattern.(*SolidColor); ok && solid != nil {
			var tint Color
			if solid.GetColor(0, 0, &tint) {
				return tint
			}
		}
	}
	return convertColor(color.Black, mode)
}

func (c *splashCanvas) fillPendingTilingPatternByTileReplay(evenOdd bool) bool {
	if c == nil || c.s == nil || c.s.bitmap == nil || c.path == nil || c.path.Length() == 0 {
		return false
	}
	pattern := c.pendingFillTilingPattern
	if pattern == nil || len(pattern.GetContent()) == 0 {
		return false
	}

	ops, err := renderer.NewEvaluator(tilingPatternXRef{}).ParseContentOperators(pattern.GetContent())
	if err != nil || len(ops) == 0 {
		return false
	}

	patternBBox := pattern.GetBBox()
	matrix := pattern.Matrix()
	scaleX := math.Hypot(matrix[0], matrix[1])
	scaleY := math.Hypot(matrix[2], matrix[3])
	if scaleX <= 0 {
		scaleX = 1
	}
	if scaleY <= 0 {
		scaleY = 1
	}

	rawXStep, rawYStep, rawCellW, rawCellH, ok := splashTilingPatternSpans(patternBBox, pattern.GetXStep(), pattern.GetYStep())
	if !ok {
		return false
	}
	stepMismatch := splashTilingPatternStepMismatch(rawCellW, rawCellH, rawXStep, rawYStep)
	cellW := int(math.Ceil(rawCellW * scaleX))
	cellH := int(math.Ceil(rawCellH * scaleY))
	if !shouldReplaySplashTilingPatternPerTile(cellW, cellH, stepMismatch) {
		return false
	}

	xStepPx := rawXStep * scaleX
	yStepPx := rawYStep * scaleY
	if xStepPx <= 0 || yStepPx <= 0 {
		return false
	}

	fillXMin, fillYMin, fillXMax, fillYMax := c.debugPathBounds()
	yOrigin := c.canvasYOrigin()
	fillYDevMin := yOrigin - fillYMax
	fillYDevMax := yOrigin - fillYMin

	bboxX0Px := patternBBox[0] * scaleX
	bboxX2Px := patternBBox[2] * scaleX
	bboxY1Px := patternBBox[1] * scaleY
	bboxY3Px := patternBBox[3] * scaleY
	originX := matrix[4]
	originYDev := matrix[5]

	startI := int(math.Floor((fillXMin-bboxX2Px-originX)/xStepPx)) - 1
	endI := int(math.Ceil((fillXMax-bboxX0Px-originX)/xStepPx)) + 1
	startJ := int(math.Floor((fillYDevMin-bboxY3Px-originYDev)/yStepPx)) - 1
	endJ := int(math.Ceil((fillYDevMax-bboxY1Px-originYDev)/yStepPx)) + 1
	if os.Getenv("PDF_SPLASH_TILING_FULL_AFFINE_REPLAY") != "" {
		invMatrix, ok := invertAffine(matrix)
		if !ok {
			return false
		}
		patXMin, patYMin, patXMax, patYMax := transformedBBoxBounds([4]float64{fillXMin, fillYDevMin, fillXMax, fillYDevMax}, invMatrix)
		startI, endI = splashTilingPatternTileRange(patXMin, patXMax, patternBBox[0], patternBBox[2], rawXStep)
		startJ, endJ = splashTilingPatternTileRange(patYMin, patYMax, patternBBox[1], patternBBox[3], rawYStep)
	}
	if os.Getenv("PDF_SPLASH_TILING_POPPLER_RANGE") != "" {
		if patternBBox[0] < patternBBox[2] {
			startI = int(math.Ceil((fillXMin - bboxX2Px - originX) / xStepPx))
			endI = int(math.Floor((fillXMax - bboxX0Px - originX) / xStepPx))
		} else {
			startI = int(math.Ceil((fillXMin - bboxX0Px - originX) / xStepPx))
			endI = int(math.Floor((fillXMax - bboxX2Px - originX) / xStepPx))
		}
		if patternBBox[1] < patternBBox[3] {
			startJ = int(math.Ceil((fillYDevMin - bboxY3Px - originYDev) / yStepPx))
			endJ = int(math.Floor((fillYDevMax - bboxY1Px - originYDev) / yStepPx))
		} else {
			startJ = int(math.Ceil((fillYDevMin - bboxY1Px - originYDev) / yStepPx))
			endJ = int(math.Floor((fillYDevMax - bboxY3Px - originYDev) / yStepPx))
		}
	}
	numX := endI - startI + 1
	numY := endJ - startJ + 1
	const maxReplayTiles = 16384
	if numX <= 0 || numY <= 0 || numX > maxReplayTiles || numY > maxReplayTiles || numX*numY > maxReplayTiles*4 {
		return false
	}

	if err := c.drawTilingPatternByTileReplay(pattern, ops, pattern.GetResources(), patternBBox, matrix, rawXStep, rawYStep, scaleX, scaleY, startI, endI, startJ, endJ, originX, originYDev, xStepPx, yStepPx, evenOdd); err != nil {
		return false
	}
	return true
}

func (c *splashCanvas) drawTilingPatternByTileReplay(
	pattern *entity.TilingPattern,
	ops []renderer.Operator,
	resources *entity.Dict,
	patternBBox [4]float64,
	matrix [6]float64,
	rawXStep float64,
	rawYStep float64,
	scaleX float64,
	scaleY float64,
	startI int,
	endI int,
	startJ int,
	endJ int,
	originX float64,
	originYDev float64,
	xStepPx float64,
	yStepPx float64,
	evenOdd bool,
) error {
	// Poppler's SplashOutputDev::tilingPatternFill fast path rejects any
	// XStep/YStep != BBox extent. Gfx::doTilingPatternFill then clips to the
	// parent fill path and replays the original pattern stream per tile.
	origPath := c.path
	c.s.SaveState()
	if err := c.s.ClipToPath(origPath, evenOdd); err != nil {
		_ = c.s.RestoreState()
		return err
	}
	parentStrokeAlpha := c.s.state.strokeAlpha
	parentFillAlpha := c.s.state.fillAlpha
	c.s.SetPatternAlpha(parentStrokeAlpha, parentFillAlpha)
	defer func() {
		c.s.ClearPatternAlpha()
		_ = c.s.RestoreState()
	}()

	mode := c.s.bitmap.Mode()
	parentFill := splashColorToNRGBA(c.pendingFillTilingTint, mode)
	yOrigin := c.canvasYOrigin()
	mainBounds := image.Rect(0, 0, c.width, c.height)
	bboxMinX := math.Min(patternBBox[0], patternBBox[2]) * scaleX
	bboxMaxX := math.Max(patternBBox[0], patternBBox[2]) * scaleX
	bboxMinYDev := math.Min(patternBBox[1], patternBBox[3]) * scaleY
	bboxMaxYDev := math.Max(patternBBox[1], patternBBox[3]) * scaleY
	exactBBoxClip := os.Getenv("PDF_SPLASH_TILING_EXACT_BBOX_CLIP") != ""
	pathBBoxClip := os.Getenv("PDF_SPLASH_TILING_BBOX_PATH_CLIP") != ""
	fullAffineReplay := os.Getenv("PDF_SPLASH_TILING_FULL_AFFINE_REPLAY") != ""

	c.path = xpath.NewPath()
	for j := startJ; j <= endJ; j++ {
		for i := startI; i <= endI; i++ {
			tileX := originX + float64(i)*xStepPx
			tileYDev := originYDev + float64(j)*yStepPx
			tileTransform := [6]float64{scaleX, 0, 0, scaleY, tileX, tileYDev}
			var tileBBoxPath *xpath.Path
			tileClipX0 := tileX + bboxMinX
			tileClipX1 := tileX + bboxMaxX
			tileClipY0 := yOrigin - (tileYDev + bboxMaxYDev)
			tileClipY1 := yOrigin - (tileYDev + bboxMinYDev)
			if fullAffineReplay {
				tileTransform = matrix
				tileTransform[4] = float64(i)*rawXStep*matrix[0] + float64(j)*rawYStep*matrix[2] + matrix[4]
				tileTransform[5] = float64(i)*rawXStep*matrix[1] + float64(j)*rawYStep*matrix[3] + matrix[5]
				var xMin, yMin, xMax, yMax float64
				tileBBoxPath, xMin, yMin, xMax, yMax = transformedBBoxPath(patternBBox, tileTransform, yOrigin)
				tileClipX0 = xMin
				tileClipX1 = xMax
				tileClipY0 = yMin
				tileClipY1 = yMax
			}
			tileMinX := int(math.Floor(tileClipX0))
			tileMaxX := int(math.Ceil(tileClipX1))
			tileMinY := int(math.Floor(tileClipY0))
			tileMaxY := int(math.Ceil(tileClipY1))
			tileBounds := image.Rect(tileMinX, tileMinY, tileMaxX, tileMaxY).Intersect(mainBounds)
			if tileBounds.Empty() {
				continue
			}

			c.s.SaveState()
			if fullAffineReplay && tileBBoxPath != nil {
				_ = c.s.ClipToPath(tileBBoxPath, false)
			} else if pathBBoxClip {
				// Poppler's Gfx::drawForm clips by constructing the transformed
				// BBox path, not by installing a rectangular clip directly.
				tileClip := xpath.NewPath()
				_ = tileClip.MoveTo(tileClipX0, tileClipY0)
				_ = tileClip.LineTo(tileClipX1, tileClipY0)
				_ = tileClip.LineTo(tileClipX1, tileClipY1)
				_ = tileClip.LineTo(tileClipX0, tileClipY1)
				_ = tileClip.Close(false)
				_ = c.s.ClipToPath(tileClip, false)
			} else if exactBBoxClip {
				// Poppler drawForm clips to the transformed BBox path before replaying
				// the pattern stream; keep sub-pixel clip coordinates for diagnostics.
				_ = c.s.ClipToRect(
					math.Max(tileClipX0, 0),
					math.Max(tileClipY0, 0),
					math.Min(tileClipX1, float64(c.width)),
					math.Min(tileClipY1, float64(c.height)),
				)
			} else {
				_ = c.s.ClipToRect(float64(tileBounds.Min.X), float64(tileBounds.Min.Y), float64(tileBounds.Max.X), float64(tileBounds.Max.Y))
			}
			tileEval := renderer.NewEvaluator(tilingPatternXRef{})
			tileEval.SetCanvas(c)
			if resources != nil {
				tileEval.SetResources(resources)
			}
			if pattern.IsUncolored() {
				tileEval.SetFillColor(parentFill)
				tileEval.SetStrokeColor(parentFill)
			}
			tileEval.SetFillPattern(nil)
			tileEval.SetStrokePattern(nil)
			// Poppler's Gfx::doTilingPatternFill initializes fill-pattern
			// fallback replay with lineWidth=0 before drawForm().
			_ = tileEval.SetLineWidth(renderer.Operator{Operands: []entity.Object{entity.NewInteger(0)}})
			tileEval.SetInitialTransform(tileTransform)
			tileEval.ExecuteOperators(ops)
			_ = c.s.RestoreState()
			c.path = xpath.NewPath()
		}
	}
	return nil
}

func transformedBBoxBounds(bbox [4]float64, matrix [6]float64) (float64, float64, float64, float64) {
	x0, y0 := applyAffine(matrix, bbox[0], bbox[1])
	xMin, xMax := x0, x0
	yMin, yMax := y0, y0
	for _, pt := range [][2]float64{
		{bbox[0], bbox[3]},
		{bbox[2], bbox[1]},
		{bbox[2], bbox[3]},
	} {
		x, y := applyAffine(matrix, pt[0], pt[1])
		if x < xMin {
			xMin = x
		}
		if x > xMax {
			xMax = x
		}
		if y < yMin {
			yMin = y
		}
		if y > yMax {
			yMax = y
		}
	}
	return xMin, yMin, xMax, yMax
}

func (c *splashCanvas) currentClipBBoxForShadingCache(matrix [6]float64) ([4]float64, bool) {
	if c == nil || c.s == nil {
		return [4]float64{}, false
	}
	xMin, yMin, xMax, yMax, ok := c.s.ensureClip().VectorEffectiveBounds()
	if !ok || xMax <= xMin || yMax <= yMin {
		return [4]float64{}, false
	}
	inv, ok := invertAffine(matrix)
	if !ok {
		return [4]float64{}, false
	}
	yOrigin := c.flipYOrigin()
	deviceBBox := [4]float64{xMin, yOrigin - yMax, xMax, yOrigin - yMin}
	uxMin, uyMin, uxMax, uyMax := transformedBBoxBounds(deviceBBox, inv)
	return [4]float64{uxMin, uyMin, uxMax, uyMax}, true
}

func transformedBBoxPath(bbox [4]float64, matrix [6]float64, yOrigin float64) (*xpath.Path, float64, float64, float64, float64) {
	points := [4][2]float64{
		{bbox[0], bbox[1]},
		{bbox[2], bbox[1]},
		{bbox[2], bbox[3]},
		{bbox[0], bbox[3]},
	}
	path := xpath.NewPath()
	var xMin, yMin, xMax, yMax float64
	for i, pt := range points {
		x, yDev := applyAffine(matrix, pt[0], pt[1])
		y := yOrigin - yDev
		if i == 0 {
			xMin, xMax = x, x
			yMin, yMax = y, y
			_ = path.MoveTo(x, y)
			continue
		}
		if x < xMin {
			xMin = x
		}
		if x > xMax {
			xMax = x
		}
		if y < yMin {
			yMin = y
		}
		if y > yMax {
			yMax = y
		}
		_ = path.LineTo(x, y)
	}
	_ = path.Close(false)
	return path, xMin, yMin, xMax, yMax
}

func splashTilingPatternTileRange(minValue, maxValue, bbox0, bbox1, step float64) (int, int) {
	if bbox0 < bbox1 {
		return int(math.Ceil((minValue - bbox1) / step)), int(math.Floor((maxValue - bbox0) / step))
	}
	return int(math.Ceil((minValue - bbox0) / step)), int(math.Floor((maxValue - bbox1) / step))
}

func (c *splashCanvas) canvasYOrigin() float64 {
	if c != nil && c.pageYOriginPx > 0 {
		return c.pageYOriginPx
	}
	if c != nil {
		return float64(c.height)
	}
	return 0
}

func splashColorToNRGBA(col Color, mode ColorMode) color.NRGBA {
	switch mode {
	case ModeBGR8:
		return color.NRGBA{R: col[2], G: col[1], B: col[0], A: 0xff}
	default:
		return color.NRGBA{R: col[0], G: col[1], B: col[2], A: 0xff}
	}
}

func shouldReplaySplashTilingPatternPerTile(cellWidth, cellHeight int, stepMismatch bool) bool {
	if os.Getenv("PDF_SPLASH_TILING_REPLAY_STEP_MISMATCH_ONLY") != "" {
		return stepMismatch
	}
	if cellWidth <= 6 || cellHeight <= 6 {
		return true
	}
	return stepMismatch
}

func splashTilingPatternStepMismatch(cellWidth, cellHeight, xStep, yStep float64) bool {
	const eps = 1e-6
	return math.Abs(cellWidth-xStep) > eps || math.Abs(cellHeight-yStep) > eps
}

func splashTilingPatternSpans(bbox [4]float64, xStep, yStep float64) (float64, float64, float64, float64, bool) {
	rawXStep := math.Abs(xStep)
	rawYStep := math.Abs(yStep)
	rawCellW := math.Abs(bbox[2] - bbox[0])
	rawCellH := math.Abs(bbox[3] - bbox[1])
	var ok bool
	rawXStep, ok = splashPatternSpan(rawXStep, rawCellW)
	if !ok {
		return 0, 0, 0, 0, false
	}
	rawYStep, ok = splashPatternSpan(rawYStep, rawCellH)
	if !ok {
		return 0, 0, 0, 0, false
	}
	rawCellW, ok = splashPatternSpan(rawCellW, rawXStep)
	if !ok {
		return 0, 0, 0, 0, false
	}
	rawCellH, ok = splashPatternSpan(rawCellH, rawYStep)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return rawXStep, rawYStep, rawCellW, rawCellH, true
}

func splashPatternSpan(primary, fallback float64) (float64, bool) {
	if !math.IsNaN(primary) && !math.IsInf(primary, 0) && primary > 0 {
		return primary, true
	}
	if !math.IsNaN(fallback) && !math.IsInf(fallback, 0) && fallback > 0 {
		return fallback, true
	}
	return 0, false
}

func (c *splashCanvas) buildSplashTilingPattern(pattern *entity.TilingPattern, tint Color) *TilingPattern {
	if pattern == nil || c == nil || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	mode := c.s.bitmap.Mode()
	pBBox := pattern.GetBBox()

	// Pull the device-space scale out of the (already CTM × patternMatrix)
	// effective matrix the evaluator stores on the pattern (evaluator.go:3858).
	matrix := pattern.Matrix()
	scaleX := math.Hypot(matrix[0], matrix[1])
	scaleY := math.Hypot(matrix[2], matrix[3])
	if scaleX <= 0 {
		scaleX = 1
	}
	if scaleY <= 0 {
		scaleY = 1
	}

	rawCellW := math.Abs(pBBox[2] - pBBox[0])
	rawCellH := math.Abs(pBBox[3] - pBBox[1])
	cellWPx := rawCellW * scaleX
	cellHPx := rawCellH * scaleY
	cellW := int(math.Ceil(cellWPx))
	cellH := int(math.Ceil(cellHPx))
	if cellW <= 0 {
		cellW = 4
	}
	if cellH <= 0 {
		cellH = 4
	}
	// Clamp to a sensible upper bound so a degenerate /BBox doesn't allocate
	// gigabytes (mirrors image_canvas.maxPatternCellSize spirit).
	const cellCap = 1024
	if cellW > cellCap {
		cellW = cellCap
	}
	if cellH > cellCap {
		cellH = cellCap
	}

	// Render the cell content stream into a sub-Splash canvas. The cell origin
	// in pattern space is at pBBox[xMin, yMin]; we translate so that pattern
	// space (pBBox[0], pBBox[1]) lands at sub-bitmap pixel (0, 0).
	content := pattern.GetContent()
	var cellBitmap *Bitmap
	if len(content) > 0 {
		cellTX := -pBBox[0] * scaleX
		cellTY := -pBBox[1] * scaleY
		rendered, err := renderTilingCell(pattern, cellW, cellH, cellHPx, scaleX, scaleY, cellTX, cellTY)
		if err == nil && rendered != nil {
			cellBitmap = rendered
		}
	}
	if cellBitmap == nil {
		cellBitmap = synthFallbackTilingCell(cellW, cellH, mode)
	}

	xStep := pattern.GetXStep()
	if xStep == 0 {
		xStep = rawCellW
	}
	if xStep == 0 {
		xStep = float64(cellW)
	}
	yStep := pattern.GetYStep()
	if yStep == 0 {
		yStep = rawCellH
	}
	if yStep == 0 {
		yStep = float64(cellH)
	}
	return NewTilingPatternWithMode(cellBitmap, pBBox, xStep, yStep, pattern.Matrix(), pattern.GetPaintType(), tint, mode)
}

func (c *splashCanvas) DrawTilingPattern(pattern *entity.TilingPattern, bbox [4]float64) error {
	if pattern == nil || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	splashPattern := c.buildSplashTilingPattern(pattern, c.currentFillPatternColor(c.s.bitmap.Mode()))
	if splashPattern == nil {
		return nil
	}

	path := c.deviceBBoxPath(bbox)
	return c.s.FillWithTilingPattern(splashPattern, path, false)
}

// renderTilingCell rasterizes a tiling pattern's content stream into a fresh
// splash bitmap by recursing through renderer.Evaluator with a sub-splashCanvas
// as the target (SplashOutputDev.cc:doTiling sub-render path).
func renderTilingCell(pattern *entity.TilingPattern, cellW, cellH int, cellHPx, scaleX, scaleY, cellTX, cellTY float64) (*Bitmap, error) {
	if pattern == nil || cellW <= 0 || cellH <= 0 {
		return nil, fmt.Errorf("invalid cell dimensions")
	}
	sub, ok := NewBackend(cellW, cellH).(*splashCanvas)
	if !ok || sub == nil {
		return nil, fmt.Errorf("sub-canvas allocation failed")
	}
	// Clear the cell bitmap to transparent (alpha=0, data=0) — this is essential
	// for tiling pattern correctness: only painted pixels (alpha > 0) tint the
	// destination via TilingPattern.GetColor (cellHasAlpha branch). Unpainted
	// regions are pattern holes that let the parent paper show through. Without
	// this, paper-white pixels in the cell would be tinted by PaintType=2's
	// TintColor, painting the entire bbox solid (PaintType=1: would also leak
	// the paper color where there should be a hole). Mirrors legacy
	// image_canvas.DrawTilingPattern using a default-zero RGBA cellImg.
	if sub.s != nil && sub.s.bitmap != nil {
		bm := sub.s.bitmap
		for i := range bm.data {
			bm.data[i] = 0
		}
		if bm.alpha != nil {
			for i := range bm.alpha {
				bm.alpha[i] = 0
			}
		}
	}
	if cellHPx > 0 {
		sub.SetPageYOriginPx(cellHPx)
	} else {
		sub.SetPageYOriginPx(float64(cellH))
	}

	eval := renderer.NewEvaluator(tilingPatternXRef{})
	eval.SetCanvas(sub)
	if res := pattern.GetResources(); res != nil {
		eval.SetResources(res)
	}
	if pattern.IsUncolored() {
		// PaintType=2 uncolored cells receive their tint from the parent's
		// fill color via TilingPattern.GetColor (Splash.cc tinting path);
		// seed black so explicit color ops in the cell have a baseline.
		eval.SetFillColor(color.Black)
		eval.SetStrokeColor(color.Black)
	}
	eval.SetFillPattern(nil)
	eval.SetStrokePattern(nil)
	eval.SetInitialTransform([6]float64{scaleX, 0, 0, scaleY, cellTX, cellTY})

	ops, err := eval.ParseContentOperators(pattern.GetContent())
	if err != nil {
		return nil, fmt.Errorf("parse tiling content: %w", err)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("empty tiling content")
	}
	eval.ExecuteOperators(ops)

	if sub.s == nil || sub.s.bitmap == nil {
		return nil, fmt.Errorf("sub-canvas bitmap missing")
	}
	return sub.s.bitmap, nil
}

// synthFallbackTilingCell produces the Phase-3 diagonal-stripe placeholder cell
// used when no content stream is available — keeps the bbox visibly filled.
func synthFallbackTilingCell(cellW, cellH int, mode ColorMode) *Bitmap {
	if cellW <= 0 {
		cellW = 4
	}
	if cellH <= 0 {
		cellH = 4
	}
	bm := NewBitmap(cellW, cellH, mode, true)
	bm.Clear(paperColor(mode))
	black := convertColor(color.Black, mode)
	bpp := bytesPerPixel(mode)
	for yy := 0; yy < cellH; yy++ {
		for xx := 0; xx < cellW; xx++ {
			if (xx+yy)%2 == 0 {
				continue
			}
			off := yy*bm.rowSize + xx*bpp
			if off+bpp > len(bm.data) {
				continue
			}
			for k := 0; k < bpp; k++ {
				bm.data[off+k] = black[k]
			}
			if bm.alpha != nil {
				bm.alpha[yy*cellW+xx] = 0xFF
			}
		}
	}
	return bm
}

// tilingPatternXRef is a no-op entity.XRef for tiling pattern content streams —
// pattern cells must not contain indirect references (PDF 1.7 §8.7.3.3).
type tilingPatternXRef struct{}

// Fetch returns an error because tiling cells should not have indirect refs.
func (tilingPatternXRef) Fetch(entity.Ref) (entity.Object, error) {
	return nil, fmt.Errorf("tiling pattern cell: indirect refs unsupported")
}

// DrawShadingPattern wires entity.ShadingPattern → the appropriate Splash
// shaded-fill driver based on ShadingType (PDF 1.7 §8.7.4, Splash.cc:6240).
func (c *splashCanvas) DrawShadingPattern(pattern *entity.ShadingPattern, bbox [4]float64) error {
	if pattern == nil || c.s == nil || c.s.bitmap == nil {
		return nil
	}
	shading := pattern.GetShading()
	if shading == nil {
		return fmt.Errorf("splashCanvas: shading pattern has no shading object")
	}

	path := c.shadingFillPath(pattern.Matrix(), bbox, shading.HasBBox())

	mode := c.s.bitmap.Mode()
	switch shading.GetShadingType() {
	case entity.ShadingAxial:
		cacheBBox := bbox
		if clipCacheBBox, ok := c.currentClipBBoxForShadingCache(pattern.Matrix()); ok {
			cacheBBox = clipCacheBBox
		}
		shader, err := c.buildAxialPatternShader(shading, pattern.Matrix(), mode, bbox, cacheBBox)
		if err != nil {
			return err
		}
		return c.s.FillAxialShadingWithBBox(shader, path, shading.HasBBox())
	case entity.ShadingRadial:
		cacheBBox := bbox
		if clipCacheBBox, ok := c.currentClipBBoxForShadingCache(pattern.Matrix()); ok {
			cacheBBox = clipCacheBBox
		}
		shader, err := c.buildRadialPatternShader(shading, pattern.Matrix(), mode, bbox, cacheBBox)
		if err != nil {
			return err
		}
		return c.s.FillRadialShadingWithBBox(shader, path, shading.HasBBox())
	case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud,
		entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
		if shading.GetShadingType() == entity.ShadingCoonsPatch || shading.GetShadingType() == entity.ShadingTensorProductPatch {
			if len(shading.GetPatches()) == 0 {
				return fmt.Errorf("splashCanvas: patch mesh shading requires decoded patches")
			}
			c.s.SaveState()
			if path != nil {
				if err := c.s.ClipToPath(path, false); err != nil {
					_ = c.s.RestoreState()
					return err
				}
			}
			err := c.fillPatchMeshShading(pattern)
			_ = c.s.RestoreState()
			return err
		}
		shader, err := buildGouraudShader(shading, mode)
		if err != nil {
			return err
		}
		return c.s.FillGouraudTriangleShadedFill(shader, path, false)
	default:
		return fmt.Errorf("splashCanvas: unsupported ShadingType %d", shading.GetShadingType())
	}
}

func (c *splashCanvas) shadingFillPath(matrix [6]float64, bbox [4]float64, hasBBox bool) *xpath.Path {
	if hasBBox {
		return c.deviceBBoxPath(bbox)
	}

	// SplashOutputDev::univariateShadedFill floors/ceils the device clip bbox,
	// maps it through the inverse CTM, takes the user-space bbox, then converts
	// that rectangle back through the CTM. Building a device-axis rectangle
	// directly shifts antialiased edges for non-identity CTMs.
	inv, ok := invertAffine(matrix)
	if !ok {
		return c.deviceBBoxPath(bbox)
	}
	userCorners := [4][2]float64{
		applyAffinePoint(inv, bbox[0], bbox[1]),
		applyAffinePoint(inv, bbox[2], bbox[1]),
		applyAffinePoint(inv, bbox[0], bbox[3]),
		applyAffinePoint(inv, bbox[2], bbox[3]),
	}
	xMin, yMin := userCorners[0][0], userCorners[0][1]
	xMax, yMax := xMin, yMin
	for i := 1; i < len(userCorners); i++ {
		x, y := userCorners[i][0], userCorners[i][1]
		if x < xMin {
			xMin = x
		}
		if x > xMax {
			xMax = x
		}
		if y < yMin {
			yMin = y
		}
		if y > yMax {
			yMax = y
		}
	}
	deviceCorners := [4][2]float64{
		applyAffinePoint(matrix, xMin, yMin),
		applyAffinePoint(matrix, xMax, yMin),
		applyAffinePoint(matrix, xMax, yMax),
		applyAffinePoint(matrix, xMin, yMax),
	}
	return c.devicePointPath(deviceCorners)
}

func applyAffinePoint(matrix [6]float64, x, y float64) [2]float64 {
	tx, ty := applyAffine(matrix, x, y)
	return [2]float64{tx, ty}
}

func (c *splashCanvas) deviceBBoxPath(bbox [4]float64) *xpath.Path {
	return c.devicePointPath([4][2]float64{
		{bbox[0], bbox[1]},
		{bbox[2], bbox[1]},
		{bbox[2], bbox[3]},
		{bbox[0], bbox[3]},
	})
}

func (c *splashCanvas) devicePointPath(points [4][2]float64) *xpath.Path {
	path := xpath.NewPath()
	_ = path.MoveTo(points[0][0], c.flipY(points[0][1]))
	for i := 1; i < len(points); i++ {
		_ = path.LineTo(points[i][0], c.flipY(points[i][1]))
	}
	_ = path.Close(false)
	return path
}

// buildShadingFunc returns a t→splash.Color closure that evaluates the
// shading's PDF Function array and packs the result into the bitmap's mode
// (PDF 1.7 §8.7.4.5.3 Function evaluation).
func buildShadingFunc(shading *entity.Shading, mode ColorMode) func(t float64) Color {
	return buildShadingFuncWithCache(shading, mode, nil)
}

func buildShadingFuncWithCache(shading *entity.Shading, mode ColorMode, cache *splashUnivariateShadingColorCache) func(t float64) Color {
	functions := shading.GetFunctions()
	cs := shading.GetColorSpace()
	bg := shading.GetBackground()
	return func(t float64) Color {
		if cache != nil {
			if out, ok := cache.Evaluate(t); ok && len(out) > 0 {
				return packShadingOutput(out, cs, mode)
			}
		}
		out, err := evalShadingFunctions(functions, []float64{t})
		if err == nil && len(out) > 0 {
			return packShadingOutput(out, cs, mode)
		}
		if bg != nil {
			return convertColor(bg, mode)
		}
		return Color{}
	}
}

// evalShadingFunctions evaluates one or more PDF Functions on inputs, returning
// the concatenated outputs (single-output stitching mode mirrors
// canvas.evaluateShadingColorFunctions).
func evalShadingFunctions(functions []entity.Function, inputs []float64) ([]float64, error) {
	if len(functions) == 0 {
		return nil, fmt.Errorf("shading has no function")
	}
	if len(functions) == 1 {
		return functions[0].Evaluate(inputs)
	}
	colors := make([]float64, 0, len(functions))
	for _, fn := range functions {
		values, err := fn.Evaluate(inputs)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("shading function returned no values")
		}
		colors = append(colors, values[0])
	}
	return colors, nil
}

// packShadingOutput converts function output (typically [0,1] per channel) into
// a splash.Color packed for the active bitmap mode (Splash.cc:1601 install path).
func packShadingOutput(out []float64, cs string, mode ColorMode) Color {
	clamp := func(v float64) byte {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		return popplerColorComponentToByte(v)
	}
	var r8, g8, b8 byte
	switch len(out) {
	case 0:
		r8, g8, b8 = 0, 0, 0
	case 1:
		// Gray (DeviceGray / CalGray).
		v := clamp(out[0])
		r8, g8, b8 = v, v, v
	case 3:
		r8, g8, b8 = clamp(out[0]), clamp(out[1]), clamp(out[2])
	case 4:
		// CMYK → RGB (naive).
		c := out[0]
		m := out[1]
		y := out[2]
		k := out[3]
		r := (1 - c) * (1 - k)
		g := (1 - m) * (1 - k)
		bb := (1 - y) * (1 - k)
		r8, g8, b8 = clamp(r), clamp(g), clamp(bb)
	default:
		r8, g8, b8 = clamp(out[0]), clamp(out[len(out)/2]), clamp(out[len(out)-1])
	}
	_ = cs
	switch mode {
	case ModeMono8:
		y := (2126*uint32(r8) + 7152*uint32(g8) + 722*uint32(b8) + 5000) / 10000
		if y > 0xFF {
			y = 0xFF
		}
		return Color{byte(y)}
	case ModeBGR8:
		return Color{b8, g8, r8}
	case ModeXBGR8:
		return Color{b8, g8, r8, 0}
	case ModeCMYK8:
		k := uint32(0xFF)
		if uint32(r8) < k {
			k = uint32(r8)
		}
		if uint32(g8) < k {
			k = uint32(g8)
		}
		if uint32(b8) < k {
			k = uint32(b8)
		}
		return Color{byte(0xFF - r8), byte(0xFF - g8), byte(0xFF - b8), byte(0xFF - k)}
	case ModeDeviceN8:
		return Color{r8, g8, b8}
	default:
		return Color{r8, g8, b8}
	}
}

func popplerColorComponentToByte(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 0xFF
	}
	// Poppler GfxState.h: dblToCol truncates to 16.16 fixed point, then
	// colToByte computes ((x << 8) - x + 0x8000) >> 16.
	x := int64(v * 65536.0)
	y := ((x << 8) - x + 0x8000) >> 16
	if y < 0 {
		return 0
	}
	if y > 0xFF {
		return 0xFF
	}
	return byte(y)
}

// buildAxialShader translates an entity.Shading (Type 2) → AxialShader (Splash.cc:6240).
func buildAxialShader(shading *entity.Shading, mode ColorMode) (*AxialShader, error) {
	coords := shading.GetCoords()
	if len(coords) < 4 {
		return nil, fmt.Errorf("splashCanvas: axial shading requires 4 coords")
	}
	domain := shading.GetDomain()
	t0, t1 := domain[0], domain[1]
	if t0 == 0 && t1 == 0 {
		t0, t1 = 0, 1
	}
	extend := shading.GetExtend()
	fn := buildShadingFunc(shading, mode)
	return NewAxialShader(coords[0], coords[1], coords[2], coords[3], t0, t1, extend[0], extend[1], fn, mode), nil
}

func (c *splashCanvas) buildAxialPatternShader(shading *entity.Shading, matrix [6]float64, mode ColorMode, bboxes ...[4]float64) (*AxialShader, error) {
	coords := shading.GetCoords()
	if len(coords) < 4 {
		return nil, fmt.Errorf("splashCanvas: axial shading requires 4 coords")
	}
	inv, ok := invertAffine(matrix)
	if !ok {
		return nil, fmt.Errorf("splashCanvas: axial shading matrix is singular")
	}
	domain := shading.GetDomain()
	t0, t1 := domain[0], domain[1]
	if t0 == 0 && t1 == 0 {
		t0, t1 = 0, 1
	}
	extend := shading.GetExtend()
	fn := buildShadingFunc(shading, mode)
	if len(bboxes) > 0 && os.Getenv("PDF_DEBUG_SPLASH_DISABLE_SHADING_CACHE") != "1" {
		cacheBBox := bboxes[0]
		if len(bboxes) > 1 {
			cacheBBox = bboxes[1]
		}
		fn = buildShadingFuncWithCache(shading, mode, newSplashUnivariateShadingColorCache(shading, matrix, cacheBBox))
	}
	yOrigin := c.canvasYOrigin()
	transform := func(x, y float64) (float64, float64) {
		return applyAffine(inv, x, yOrigin-y)
	}
	return NewAxialShaderWithTransform(coords[0], coords[1], coords[2], coords[3], t0, t1, extend[0], extend[1], fn, mode, transform), nil
}

// buildRadialShader translates an entity.Shading (Type 3) → RadialShader (Splash.cc:6240).
func buildRadialShader(shading *entity.Shading, mode ColorMode) (*RadialShader, error) {
	coords := shading.GetCoords()
	if len(coords) < 6 {
		return nil, fmt.Errorf("splashCanvas: radial shading requires 6 coords")
	}
	domain := shading.GetDomain()
	t0, t1 := domain[0], domain[1]
	if t0 == 0 && t1 == 0 {
		t0, t1 = 0, 1
	}
	extend := shading.GetExtend()
	fn := buildShadingFunc(shading, mode)
	return NewRadialShader(coords[0], coords[1], coords[2], coords[3], coords[4], coords[5], t0, t1, extend[0], extend[1], fn, mode), nil
}

func (c *splashCanvas) buildRadialPatternShader(shading *entity.Shading, matrix [6]float64, mode ColorMode, bboxes ...[4]float64) (*RadialShader, error) {
	coords := shading.GetCoords()
	if len(coords) < 6 {
		return nil, fmt.Errorf("splashCanvas: radial shading requires 6 coords")
	}
	inv, ok := invertAffine(matrix)
	if !ok {
		return nil, fmt.Errorf("splashCanvas: radial shading matrix is singular")
	}
	domain := shading.GetDomain()
	t0, t1 := domain[0], domain[1]
	if t0 == 0 && t1 == 0 {
		t0, t1 = 0, 1
	}
	extend := shading.GetExtend()
	fn := buildShadingFunc(shading, mode)
	if len(bboxes) > 0 && os.Getenv("PDF_DEBUG_SPLASH_DISABLE_SHADING_CACHE") != "1" {
		cacheBBox := bboxes[0]
		if len(bboxes) > 1 {
			cacheBBox = bboxes[1]
		}
		fn = buildShadingFuncWithCache(shading, mode, newSplashUnivariateShadingColorCache(shading, matrix, cacheBBox))
	}
	yOrigin := c.canvasYOrigin()
	if os.Getenv("PDF_DEBUG_SPLASH_RADIAL_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "SPLASH_RADIAL_TRACE matrix=%v inv=%v coords=%v domain=%v extend=%v yOrigin=%.8f\n",
			matrix, inv, coords, domain, extend, yOrigin)
	}
	transform := func(x, y float64) (float64, float64) {
		return applyAffine(inv, x, yOrigin-y)
	}
	return NewRadialShaderWithTransform(coords[0], coords[1], coords[2], coords[3], coords[4], coords[5], t0, t1, extend[0], extend[1], fn, mode, transform), nil
}

// buildGouraudShader translates an entity.Shading (Type 4-7) → GouraudShader (Splash.cc:5255).
//
// Patch-mesh types (6, 7) are flattened to vertex-triplet triangles — full
// Coons/tensor evaluation is a Phase 4 follow-up; for now we fall back to the
// vertex list in groups of three.
//
// PDF 1.7 §8.7.4.5.5: when the shading dict has a Function, each vertex's
// stored color values are *function inputs* (typically a single parameter t in
// [0,1]) — the function evaluates to colorspace tuples that packShadingOutput
// then maps to the bitmap mode. Without a Function, the vertex colors are
// already colorspace tuples. This mirrors canvas.drawGouraudShading, which
// passes shading.GetFunctions() to the rasterizer and evaluates per-pixel.
// Pre-evaluating at the vertex preserves Splash's per-pixel-barycentric path
// while producing the correct colors for the common single-parameter case.
func buildGouraudShader(shading *entity.Shading, mode ColorMode) (*GouraudShader, error) {
	return buildGouraudShaderWithTransform(shading, mode, nil)
}

func (c *splashCanvas) buildGouraudPatternShader(shading *entity.Shading, matrix [6]float64, mode ColorMode) (*GouraudShader, error) {
	return buildGouraudShaderWithTransform(shading, mode, func(x, y float64) (float64, float64) {
		tx := x*matrix[0] + y*matrix[2] + matrix[4]
		ty := x*matrix[1] + y*matrix[3] + matrix[5]
		return tx, c.flipY(ty)
	})
}

func buildGouraudShaderWithTransform(shading *entity.Shading, mode ColorMode, transform func(x, y float64) (float64, float64)) (*GouraudShader, error) {
	verts := shading.GetVertices()
	if len(verts) < 3 {
		return nil, fmt.Errorf("splashCanvas: gouraud shading requires >=3 vertices")
	}
	cs := shading.GetColorSpace()
	functions := shading.GetFunctions()
	out := make([]GouraudVertex, 0, len(verts))
	for _, v := range verts {
		colors := v.Colors
		params := append([]float64(nil), v.Colors...)
		if len(functions) > 0 {
			if evaluated, err := evalShadingFunctions(functions, v.Colors); err == nil && len(evaluated) > 0 {
				colors = evaluated
			}
		}
		x, y := v.X, v.Y
		if transform != nil {
			x, y = transform(x, y)
		}
		out = append(out, GouraudVertex{
			X:      x,
			Y:      y,
			Color:  packShadingOutput(colors, cs, mode),
			Params: params,
		})
	}
	if len(functions) > 0 {
		return NewParameterizedGouraudShader(out, mode, functions, cs), nil
	}
	return NewGouraudShader(out, mode), nil
}

// Image returns the rendered bitmap as an *image.RGBA (SplashBitmap::getRGBLine).
//
// Splash's internal RGB8 bitmap is packed [R,G,B] per pixel, no alpha plane in
// the colour bytes. We expand it to the stdlib's RGBA layout so callers
// (concurrent_renderer, PNG encoder) get a fully opaque visible image.
func (c *splashCanvas) Image() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, c.width, c.height))
	if c.s == nil || c.s.bitmap == nil {
		return img
	}
	bm := c.s.bitmap
	src := bm.Data()
	if len(src) == 0 {
		return img
	}
	bpp := bytesPerPixel(bm.mode)
	rs := bm.rowSize
	if rs == 0 {
		rs = bm.width * bpp
	}
	dst := img.Pix
	dstStride := img.Stride
	switch bm.mode {
	case ModeRGB8:
		alpha := bm.Alpha()
		compositePaper := usePopplerTransparentPageAlpha() && len(alpha) >= bm.width*bm.height
		for y := 0; y < bm.height; y++ {
			srcOff := y * rs
			dstOff := y * dstStride
			alphaOff := y * bm.width
			for x := 0; x < bm.width; x++ {
				if compositePaper {
					a := int(alpha[alphaOff+x])
					invA := 255 - a
					dst[dstOff+0] = byte(Div255(invA*255 + a*int(src[srcOff+0])))
					dst[dstOff+1] = byte(Div255(invA*255 + a*int(src[srcOff+1])))
					dst[dstOff+2] = byte(Div255(invA*255 + a*int(src[srcOff+2])))
				} else {
					dst[dstOff+0] = src[srcOff+0]
					dst[dstOff+1] = src[srcOff+1]
					dst[dstOff+2] = src[srcOff+2]
				}
				dst[dstOff+3] = 255
				srcOff += 3
				dstOff += 4
			}
		}
	case ModeMono8:
		alpha := bm.Alpha()
		compositePaper := usePopplerTransparentPageAlpha() && len(alpha) >= bm.width*bm.height
		for y := 0; y < bm.height; y++ {
			srcOff := y * rs
			dstOff := y * dstStride
			alphaOff := y * bm.width
			for x := 0; x < bm.width; x++ {
				v := src[srcOff]
				if compositePaper {
					a := int(alpha[alphaOff+x])
					v = byte(Div255((255-a)*255 + a*int(v)))
				}
				dst[dstOff+0] = v
				dst[dstOff+1] = v
				dst[dstOff+2] = v
				dst[dstOff+3] = 255
				srcOff++
				dstOff += 4
			}
		}
	}
	return img
}

// NewAnnotationMaskCanvas returns a Splash-backed temporary canvas for
// generated annotation masks. Poppler renders generated highlight appearances
// through a transparent Form XObject, so the mask's alpha plane is the source
// coverage used by the later Multiply application.
func (c *splashCanvas) NewAnnotationMaskCanvas(bounds image.Rectangle, pageYOriginPx float64) domaincanvas.Canvas {
	if bounds.Empty() {
		return nil
	}
	mask := newAnnotationMaskBackend(bounds.Dx(), bounds.Dy())
	if mask == nil {
		return nil
	}
	if pageYOriginPx > 0 {
		mask.SetPageYOriginPx(pageYOriginPx)
	} else if c != nil && c.pageYOriginPx > 0 {
		mask.SetPageYOriginPx(c.pageYOriginPx)
	}
	return mask
}

// ApplyAnnotationMultiplyMaskCanvas applies a generated annotation highlight
// mask to the live Splash bitmap. Generated Splash masks use their alpha plane
// as source coverage; legacy/non-annotation Splash masks fall back to
// black-on-white luminance recovery.
func (c *splashCanvas) ApplyAnnotationMultiplyMaskCanvas(mask domaincanvas.Canvas, fill color.RGBA) {
	if maskSplash, ok := mask.(*splashCanvas); ok {
		c.applyAnnotationMultiplyMaskSplash(maskSplash, fill)
		return
	}
	if mask != nil {
		c.ApplyAnnotationMultiplyMask(mask.Image(), fill)
	}
}

// ApplyAnnotationMultiplyMask applies a generated annotation highlight mask to
// the live Splash bitmap. Image() returns a copy, so renderer-side pixel edits
// cannot update this backend directly.
func (c *splashCanvas) ApplyAnnotationMultiplyMask(mask image.Image, fill color.RGBA) {
	if c == nil || c.s == nil || c.s.bitmap == nil || mask == nil {
		return
	}
	bm := c.s.bitmap
	if bm.mode != ModeRGB8 {
		return
	}
	data := bm.Data()
	if len(data) == 0 {
		return
	}
	bounds := image.Rect(0, 0, bm.width, bm.height).Intersect(mask.Bounds())
	if bounds.Empty() {
		return
	}
	rs := bm.rowSize
	if rs == 0 {
		rs = bm.width * 3
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := y * rs
		alphaOff := y * bm.width
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a16 := mask.At(x, y).RGBA()
			maskAlpha := int(a16 >> 8)
			if maskAlpha == 0 {
				continue
			}
			off := row + x*3
			if off+2 >= len(data) {
				continue
			}
			applyAnnotationMultiplyPixel(bm, data, off, alphaOff+x, maskAlpha, fill)
		}
	}
}

func (c *splashCanvas) applyAnnotationMultiplyMaskSplash(mask *splashCanvas, fill color.RGBA) {
	if c == nil || c.s == nil || c.s.bitmap == nil || mask == nil || mask.s == nil || mask.s.bitmap == nil {
		return
	}
	bm := c.s.bitmap
	maskBM := mask.s.bitmap
	if bm.mode != ModeRGB8 || maskBM.mode != ModeRGB8 {
		c.ApplyAnnotationMultiplyMask(mask.Image(), fill)
		return
	}
	data := bm.Data()
	maskData := maskBM.Data()
	if len(data) == 0 || len(maskData) == 0 {
		return
	}
	bounds := image.Rect(0, 0, bm.width, bm.height).Intersect(image.Rect(0, 0, maskBM.width, maskBM.height))
	if bounds.Empty() {
		return
	}
	rs := bm.rowSize
	if rs == 0 {
		rs = bm.width * 3
	}
	maskRS := maskBM.rowSize
	if maskRS == 0 {
		maskRS = maskBM.width * 3
	}
	maskAlphaPlane := maskBM.Alpha()
	useAlphaMask := mask.annotationMask && len(maskAlphaPlane) >= maskBM.width*maskBM.height
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := y * rs
		maskRow := y * maskRS
		alphaOff := y * bm.width
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var maskAlpha int
			if useAlphaMask {
				maskAlphaOff := y*maskBM.width + x
				if maskAlphaOff >= len(maskAlphaPlane) {
					continue
				}
				maskAlpha = int(maskAlphaPlane[maskAlphaOff])
			} else {
				maskOff := maskRow + x*3
				if maskOff+2 >= len(maskData) {
					continue
				}
				maskAlpha = 255 - min3Int(int(maskData[maskOff]), int(maskData[maskOff+1]), int(maskData[maskOff+2]))
			}
			if maskAlpha == 0 {
				continue
			}
			off := row + x*3
			if off+2 >= len(data) {
				continue
			}
			applyAnnotationMultiplyPixel(bm, data, off, alphaOff+x, maskAlpha, fill)
		}
	}
}

func applyAnnotationMultiplyPixel(bm *Bitmap, data []byte, colorOff, alphaOff, maskAlpha int, fill color.RGBA) {
	if bm == nil || colorOff < 0 || colorOff+2 >= len(data) || maskAlpha <= 0 {
		return
	}

	src := Color{fill.R, fill.G, fill.B}
	dst := Color{data[colorOff], data[colorOff+1], data[colorOff+2]}
	var blend Color
	BlendMultiply(&src, &dst, &blend, bm.mode)

	aDest := 255
	if alphaOff >= 0 && alphaOff < len(bm.alpha) {
		aDest = int(bm.alpha[alphaOff])
	}
	aSrc := maskAlpha
	aResult := aSrc + aDest - Div255(aSrc*aDest)
	if aResult <= 0 {
		data[colorOff], data[colorOff+1], data[colorOff+2] = 0, 0, 0
		if alphaOff >= 0 && alphaOff < len(bm.alpha) {
			bm.alpha[alphaOff] = 0
		}
		return
	}

	invDest := 255 - aDest
	diff := aResult - aSrc
	for i := 0; i < 3; i++ {
		inner := invDest*int(src[i]) + aDest*int(blend[i])
		v := (diff*int(dst[i]) + aSrc*inner/255) / aResult
		if v < 0 {
			v = 0
		} else if v > 255 {
			v = 255
		}
		data[colorOff+i] = byte(v)
	}
	if alphaOff >= 0 && alphaOff < len(bm.alpha) {
		bm.alpha[alphaOff] = byte(aResult)
	}
}

func min3Int(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// Reset clears the in-progress path (SplashPath::reset, SplashPath.h).
func (c *splashCanvas) Reset() {
	c.path = xpath.NewPath()
	c.hasCur = false
}
