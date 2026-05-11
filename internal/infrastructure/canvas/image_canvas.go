// Package canvas provides canvas implementations for PDF rendering.
//
//revive:disable:exported
//nolint:unused
package canvas

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"math"
	"os"
	"reflect"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/vector"

	"github.com/dh-kam/pdf-go/internal/domain/canvas"
	domaincolorspace "github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/domain/renderer"
)

const (
	imageDirectCopyEpsilon = 1e-6
	maxPatternCellPixels   = 16 * 1024 * 1024
	maxPatternCellSize     = 8192
)

func shouldSkipStrokeSegmentsForDebug() bool {
	return os.Getenv("PDF_DEBUG_SKIP_STROKE_SEGMENTS") == "1"
}

func shouldSkipClosedStrokeOutlinesForDebug() bool {
	return os.Getenv("PDF_DEBUG_SKIP_CLOSED_STROKE_OUTLINES") == "1"
}

func shouldTraceStrokeForDebug() bool {
	return os.Getenv("PDF_DEBUG_STROKE_TRACE") == "1"
}

func shouldTraceStrokeSubpathsForDebug() bool {
	return os.Getenv("PDF_DEBUG_STROKE_TRACE_SUBPATHS") == "1"
}

func shouldSkipPopplerAAOutlinesForDebug() bool {
	return os.Getenv("PDF_DEBUG_SKIP_POPPLER_AA_OUTLINES") == "1"
}

func shouldSkipStrokeAdjustOutlinesForDebug() bool {
	return os.Getenv("PDF_DEBUG_SKIP_STROKE_ADJUST_OUTLINES") == "1"
}

func shouldSkipStrokeNarrowQuadsForDebug() bool {
	return os.Getenv("PDF_DEBUG_SKIP_STROKE_NARROW_QUADS") == "1"
}

func shouldUseStrokeAdjustTrailingInsetForDebug() bool {
	return os.Getenv("PDF_DEBUG_STROKE_ADJUST_TRAILING_INSET") == "1"
}

func shouldReplayTilingPatternPerTileForDebug() bool {
	return os.Getenv("PDF_DEBUG_TILING_PATTERN_PER_TILE_REPLAY") == "1"
}

func shouldReplayTilingPatternPerTile(ops []renderer.Operator, cellWidth int, cellHeight int, stepMismatch bool) bool {
	if cellWidth <= 6 || cellHeight <= 6 {
		return true
	}
	if stepMismatch && tilingPatternOperatorsContainFill(ops) {
		return true
	}
	return shouldReplayTilingPatternPerTileForDebug()
}

func tilingPatternOperatorsContainFill(ops []renderer.Operator) bool {
	for _, op := range ops {
		switch op.Opcode {
		case "f", "F", "f*", "B", "B*", "b", "b*":
			return true
		}
	}
	return false
}

func tilingPatternStepMismatch(cellWidth, cellHeight, xStep, yStep float64) bool {
	const eps = 1e-6
	return math.Abs(cellWidth-xStep) > eps || math.Abs(cellHeight-yStep) > eps
}

// ImageCanvas implements the Canvas interface using Go's image package.
type ImageCanvas struct {
	strokePattern       entity.Pattern
	fillPattern         entity.Pattern
	fillColor           color.Color
	strokeColor         color.Color
	transferRed         [256]uint8
	transferGreen       [256]uint8
	transferBlue        [256]uint8
	transferGray        [256]uint8
	transferActive      bool
	clipPath            *graphics.Path
	clipRect            *transformedAxisAlignedRect
	img                 *image.RGBA
	paperColor          color.RGBA
	clipMask            *image.Alpha
	currentPath         *graphics.Path
	stateStack          []*canvasState
	dashPattern         []float64
	transform           [6]float64
	textPosition        [2]float64
	lineWidth           float64
	dashPhase           float64
	miterLimit          float64
	lineJoin            int
	lineCap             int
	glyphRasterStrategy GlyphRasterStrategy
	height              int
	width               int
	inTextBlock         bool
	paperColorActive    bool
	// glyphTransform is the linear part of the text rendering matrix [a,b,c,d].
	// It scales/rotates glyph paths from user space to device space.
	glyphTransform [4]float64
	// pageYOriginPx is the exact float page height in canvas pixels (pageHeight_pt * scaleY).
	// Used for Y-coordinate baseline computation instead of the integer c.height, which
	// may differ by up to 1 pixel from the true float value due to ceiling rounding.
	pageYOriginPx float64
}

type canvasState struct {
	fillColor      color.Color
	strokeColor    color.Color
	strokePattern  entity.Pattern
	fillPattern    entity.Pattern
	transferRed    [256]uint8
	transferGreen  [256]uint8
	transferBlue   [256]uint8
	transferGray   [256]uint8
	transferActive bool
	clipPath       *graphics.Path
	clipRect       *transformedAxisAlignedRect
	clipMask       *image.Alpha
	dashPattern    []float64
	transform      [6]float64
	lineWidth      float64
	dashPhase      float64
	miterLimit     float64
	lineJoin       int
	lineCap        int
}

type glyphDrawCommand struct {
	kind entity.PathCmdType
	x    float64
	y    float64
	c1x  float64
	c1y  float64
	c2x  float64
	c2y  float64
}

// NewImageCanvas creates a new image canvas with the specified bounds.
func NewImageCanvas(bounds image.Rectangle) canvas.Canvas {
	width := bounds.Dx()
	height := bounds.Dy()

	return &ImageCanvas{
		img:            image.NewRGBA(bounds),
		width:          width,
		height:         height,
		fillColor:      color.Black,
		strokeColor:    color.Black,
		lineWidth:      1.0,
		lineCap:        0,
		lineJoin:       0,
		miterLimit:     10.0,
		dashPattern:    nil,
		dashPhase:      0,
		currentPath:    graphics.NewPath(),
		transform:      [6]float64{1, 0, 0, 1, 0, 0},
		glyphTransform: [4]float64{1, 0, 0, 1},
		textPosition:   [2]float64{0, 0},
		inTextBlock:    false,
	}
}

// SetColorTransfer sets Poppler-style RGB/gray transfer lookup tables.
func (c *ImageCanvas) SetColorTransfer(red, green, blue, gray [256]uint8, active bool) {
	c.transferRed = red
	c.transferGreen = green
	c.transferBlue = blue
	c.transferGray = gray
	c.transferActive = active
}

// SetOpaquePaperBackground initializes the Poppler paper backdrop.
func (c *ImageCanvas) SetOpaquePaperBackground(bg color.Color) {
	rgba := colorToRGBA(bg)
	rgba.A = 255
	draw.Draw(c.img, c.img.Bounds(), &image.Uniform{C: rgba}, image.Point{}, draw.Src)
	c.paperColor = rgba
	c.paperColorActive = true
}

// Width returns the canvas width.
func (c *ImageCanvas) Width() int {
	return c.width
}

// Height returns the canvas height.
func (c *ImageCanvas) Height() int {
	return c.height
}

// Bounds returns the canvas bounds.
func (c *ImageCanvas) Bounds() image.Rectangle {
	return c.img.Bounds()
}

// MoveTo moves the current position to (x, y).
func (c *ImageCanvas) MoveTo(x, y float64) {
	c.currentPath.AddCommand(&graphics.MoveTo{X: x, Y: y})
}

// LineTo draws a line to the specified position.
func (c *ImageCanvas) LineTo(x, y float64) {
	c.currentPath.AddCommand(&graphics.LineTo{X: x, Y: y})
}

// CurveTo draws a cubic Bézier curve.
func (c *ImageCanvas) CurveTo(c1x, c1y, c2x, c2y, x, y float64) {
	c.currentPath.AddCommand(&graphics.CurveTo{
		X1: c1x, Y1: c1y,
		X2: c2x, Y2: c2y,
		X3: x, Y3: y,
	})
}

// Rectangle draws a rectangle.
func (c *ImageCanvas) Rectangle(x, y, width, height float64) {
	c.currentPath.AddCommand(&graphics.MoveTo{X: x, Y: y})
	c.currentPath.AddCommand(&graphics.LineTo{X: x + width, Y: y})
	c.currentPath.AddCommand(&graphics.LineTo{X: x + width, Y: y + height})
	c.currentPath.AddCommand(&graphics.LineTo{X: x, Y: y + height})
	c.currentPath.AddCommand(&graphics.ClosePath{})
}

// ClosePath closes the current path.
func (c *ImageCanvas) ClosePath() {
	c.currentPath.AddCommand(&graphics.ClosePath{})
}

// Fill fills the current path using the fill color.
func (c *ImageCanvas) Fill() {
	c.renderPath(true)
	c.currentPath = graphics.NewPath()
}

// FillEvenOdd fills the current path using the even-odd rule.
func (c *ImageCanvas) FillEvenOdd() {
	c.renderPathEvenOdd()
	c.currentPath = graphics.NewPath()
}

// Stroke strokes the current path using the stroke color.
func (c *ImageCanvas) Stroke() {
	c.renderPath(false)
	c.currentPath = graphics.NewPath()
}

// FillAndStroke fills and strokes the current path while reusing transformed segments.
func (c *ImageCanvas) FillAndStroke() {
	commands := c.currentPath.GetCommands()
	if len(commands) == 0 {
		c.currentPath = graphics.NewPath()
		return
	}

	c.renderPath(true)
	stroke := colorToRGBA(c.strokeColor)
	strokeWidth := c.effectiveStrokeWidth()
	if len(c.dashPattern) == 0 && c.lineJoin == 0 && !shouldSkipClosedStrokeOutlinesForDebug() {
		segments := c.buildTransformedStrokeSegments(commands)
		if c.renderClosedStrokeOutlines("FillAndStrokeClosedOutlines", commands, stroke, c.lineWidth) {
			c.traceStrokeForDebug("FillAndStrokeClosedOutlines", commands, segments, strokeWidth)
			c.currentPath = graphics.NewPath()
			return
		}
	}
	segments := c.applyDashPattern(c.buildTransformedStrokeSegments(commands))
	c.traceStrokeForDebug("FillAndStroke", commands, segments, strokeWidth)
	if len(segments) > 0 && !shouldSkipStrokeSegmentsForDebug() {
		c.renderStrokeSegments(segments, stroke, strokeWidth)
	}
	c.currentPath = graphics.NewPath()
}

// FillEvenOddAndStroke fills (even-odd) and strokes the current path while reusing transformed segments.
func (c *ImageCanvas) FillEvenOddAndStroke() {
	commands := c.currentPath.GetCommands()
	if len(commands) == 0 {
		c.currentPath = graphics.NewPath()
		return
	}

	segments := c.buildTransformedStrokeSegments(commands)
	if len(segments) == 0 {
		c.currentPath = graphics.NewPath()
		return
	}

	if c.fillPattern != nil {
		if c.renderPathFillPattern(commands, true) {
			stroke := colorToRGBA(c.strokeColor)
			strokeWidth := c.effectiveStrokeWidth()
			if len(c.dashPattern) == 0 && c.lineJoin == 0 && !shouldSkipClosedStrokeOutlinesForDebug() {
				if c.renderClosedStrokeOutlines("FillEvenOddAndStrokePatternClosedOutlines", commands, stroke, c.lineWidth) {
					c.traceStrokeForDebug("FillEvenOddAndStrokePatternClosedOutlines", commands, segments, strokeWidth)
					c.currentPath = graphics.NewPath()
					return
				}
			}
			dashedSegments := c.applyDashPattern(segments)
			c.traceStrokeForDebug("FillEvenOddAndStrokePattern", commands, dashedSegments, strokeWidth)
			if !shouldSkipStrokeSegmentsForDebug() {
				c.renderStrokeSegments(dashedSegments, stroke, strokeWidth)
			}
			c.currentPath = graphics.NewPath()
			return
		}
	}

	c.renderPathEvenOddFromSegments(segments, colorToRGBA(c.fillColor))
	stroke := colorToRGBA(c.strokeColor)
	strokeWidth := c.effectiveStrokeWidth()
	if len(c.dashPattern) == 0 && c.lineJoin == 0 && !shouldSkipClosedStrokeOutlinesForDebug() {
		if c.renderClosedStrokeOutlines("FillEvenOddAndStrokeClosedOutlines", commands, stroke, c.lineWidth) {
			c.traceStrokeForDebug("FillEvenOddAndStrokeClosedOutlines", commands, segments, strokeWidth)
			c.currentPath = graphics.NewPath()
			return
		}
	}
	dashedSegments := c.applyDashPattern(segments)
	c.traceStrokeForDebug("FillEvenOddAndStroke", commands, dashedSegments, strokeWidth)
	if !shouldSkipStrokeSegmentsForDebug() {
		c.renderStrokeSegments(dashedSegments, stroke, strokeWidth)
	}
	c.currentPath = graphics.NewPath()
}

// Clip sets the current path as a clipping path (nonzero winding rule).
func (c *ImageCanvas) Clip() {
	previousClipPath := c.clipPath
	c.clipPath = c.currentPath.Clone()
	c.clipPath.SetFillRule(graphics.FillRuleNonZero)
	c.currentPath = graphics.NewPath()
	c.updateClipMask(previousClipPath)
}

// SetClipPathDirect sets a clipping path from evaluator path elements.
// Uses reflection to handle evaluator path types without circular imports.
func (c *ImageCanvas) SetClipPathDirect(elements []interface{}, fillRule graphics.FillRule) {
	previousClipPath := c.clipPath
	clipPath := graphics.NewPath()
	for _, elem := range elements {
		// Use reflection to extract X, Y fields from evaluator path elements
		val := reflect.ValueOf(elem)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		if val.Kind() == reflect.Struct {
			// Try to match by field names
			xField := val.FieldByName("X")
			yField := val.FieldByName("Y")
			x1Field := val.FieldByName("X1")
			y1Field := val.FieldByName("Y1")
			x2Field := val.FieldByName("X2")
			y2Field := val.FieldByName("Y2")

			// Check if this is a MoveTo/LineTo type (has X, Y)
			if xField.IsValid() && yField.IsValid() {
				xVal := xField.Float()
				yVal := yField.Float()

				// Check if there are more fields (CurveTo has X1, Y1, X2, Y2, X, Y)
				if x1Field.IsValid() && y1Field.IsValid() && x2Field.IsValid() && y2Field.IsValid() {
					// This is CurveTo
					clipPath.AddCommand(&graphics.CurveTo{
						X1: x1Field.Float(),
						Y1: y1Field.Float(),
						X2: x2Field.Float(),
						Y2: y2Field.Float(),
						X3: xVal,
						Y3: yVal,
					})
				} else {
					// Use type name to distinguish MoveTo from LineTo
					// Types from renderer package have names like "renderer.MoveTo"
					typeName := val.Type().Name()
					fullTypeName := val.Type().String() // Includes package name
					switch typeName {
					case "MoveTo", "renderer.MoveTo":
						clipPath.AddCommand(&graphics.MoveTo{X: xVal, Y: yVal})
					case "LineTo", "renderer.LineTo":
						clipPath.AddCommand(&graphics.LineTo{X: xVal, Y: yVal})
					default:
						// Fallback: check full type name for package-prefixed versions
						if fullTypeName == "renderer.MoveTo" {
							clipPath.AddCommand(&graphics.MoveTo{X: xVal, Y: yVal})
						} else if fullTypeName == "renderer.LineTo" {
							clipPath.AddCommand(&graphics.LineTo{X: xVal, Y: yVal})
						} else {
							// Unknown type - use MoveTo as fallback
							clipPath.AddCommand(&graphics.MoveTo{X: xVal, Y: yVal})
						}
					}
				}
			} else if val.NumField() == 0 {
				// Empty struct - Close
				clipPath.AddCommand(&graphics.ClosePath{})
			}
		} else if cmd, ok := elem.(graphics.PathCommand); ok {
			clipPath.AddCommand(cmd)
		}
	}
	clipPath.SetFillRule(fillRule)
	c.clipPath = clipPath
	c.currentPath = graphics.NewPath()
	c.updateClipMask(previousClipPath)
}

// EoClip sets the current path as a clipping path (even-odd rule).
func (c *ImageCanvas) EoClip() {
	previousClipPath := c.clipPath
	path := c.currentPath.Clone()
	path.SetFillRule(graphics.FillRuleEvenOdd)
	c.clipPath = path
	c.currentPath = graphics.NewPath()
	c.updateClipMask(previousClipPath)
}

// updateClipMask updates the clip mask based on the current clip path.
func (c *ImageCanvas) updateClipMask(previousClipPath *graphics.Path) {
	if c.clipPath == nil {
		c.clipMask = nil
		c.clipRect = nil
		return
	}

	commands := c.clipPath.GetCommands()
	if len(commands) == 0 {
		c.clipMask = nil
		c.clipRect = nil
		return
	}

	// Note: clipping path coordinates are in PDF space (Y-up), need Y-flip
	bounds, ok := c.pathBounds(commands, true)
	if !ok {
		return
	}

	// Check if we need even-odd fill rule
	useEvenOdd := c.clipPath.GetFillRule() == graphics.FillRuleEvenOdd

	previousMask := c.clipMask
	previousRect := c.clipRect
	if previousMask != nil && clipPathsEqual(previousClipPath, c.clipPath) {
		return
	}
	nextMask := image.NewAlpha(image.Rect(0, 0, c.width, c.height))

	if !useEvenOdd {
		if nextRect, rectOK := c.transformedAxisAlignedRect(commands); rectOK && (previousMask == nil || previousRect != nil) {
			if previousRect != nil {
				intersectedRect, ok := c.intersectTransformedAxisAlignedRects(*previousRect, nextRect)
				if !ok {
					c.clipMask = nextMask
					c.clipRect = nil
					return
				}
				nextRect = intersectedRect
			}
			c.clipMask = c.maskForTransformedRect(nextRect)
			c.clipRect = &nextRect
			return
		}
	}

	if useEvenOdd {
		// For even-odd rule: rasterize each subpath separately and XOR them
		// This simulates the even-odd winding rule
		subpaths := c.splitIntoSubpaths(commands)

		for _, subpathCmds := range subpaths {
			if len(subpathCmds) == 0 {
				continue
			}

			// Get bounds for this subpath
			subBounds, ok := c.pathBounds(subpathCmds, true)
			if !ok {
				continue
			}

			// Create temporary mask for this subpath
			subMask := image.NewAlpha(image.Rect(0, 0, c.width, c.height))

			// Rasterize this subpath
			ras := vector.NewRasterizer(subBounds.Dx(), subBounds.Dy())
			c.tracePathToRasterizer(ras, subpathCmds, subBounds.Min.X, subBounds.Min.Y, true, subBounds)
			c.drawRasterPathSafe(
				ras,
				subMask,
				subBounds,
				image.Opaque,
				image.Point{},
				"updateClipMask (subpath)",
			)

			// XOR with main clip mask (this implements even-odd rule)
			maskBounds := nextMask.Bounds()
			for y := maskBounds.Min.Y; y < maskBounds.Max.Y; y++ {
				for x := maskBounds.Min.X; x < maskBounds.Max.X; x++ {
					currentAlpha := nextMask.AlphaAt(x, y).A
					subAlpha := subMask.AlphaAt(x, y).A

					// XOR: flip if this pixel is set in submask
					if subAlpha > 128 {
						if currentAlpha > 0 {
							nextMask.SetAlpha(x, y, color.Alpha{A: 0})
						} else {
							nextMask.SetAlpha(x, y, color.Alpha{A: 255})
						}
					}
				}
			}
		}
	} else {
		// For nonzero rule: rasterize all paths normally
		ras := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
		c.tracePathToRasterizer(ras, commands, bounds.Min.X, bounds.Min.Y, true, bounds)
		c.drawRasterPathSafe(
			ras,
			nextMask,
			bounds,
			image.Opaque,
			image.Point{},
			"updateClipMask",
		)
	}

	// PDF clipping path semantics intersect the new clip with the existing clip.
	if previousMask != nil {
		maskBounds := nextMask.Bounds()
		for y := maskBounds.Min.Y; y < maskBounds.Max.Y; y++ {
			for x := maskBounds.Min.X; x < maskBounds.Max.X; x++ {
				nextAlpha := nextMask.AlphaAt(x, y).A
				prevAlpha := previousMask.AlphaAt(x, y).A
				nextMask.SetAlpha(x, y, color.Alpha{A: intersectClipMaskAlpha(prevAlpha, nextAlpha)})
			}
		}
	}

	c.clipMask = nextMask
	c.clipRect = nil
}

func clipPathsEqual(left, right *graphics.Path) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.GetFillRule() != right.GetFillRule() {
		return false
	}
	leftCommands := left.GetCommands()
	rightCommands := right.GetCommands()
	if len(leftCommands) != len(rightCommands) {
		return false
	}
	for i := range leftCommands {
		if !reflect.DeepEqual(leftCommands[i], rightCommands[i]) {
			return false
		}
	}
	return true
}

func intersectClipMaskAlpha(previousAlpha, nextAlpha uint8) uint8 {
	switch {
	case previousAlpha == 0 || nextAlpha == 0:
		return 0
	case previousAlpha == 255:
		return nextAlpha
	case nextAlpha == 255:
		return previousAlpha
	default:
		if previousAlpha < nextAlpha {
			return previousAlpha
		}
		return nextAlpha
	}
}

// splitIntoSubpaths splits path commands into subpaths (separated by MoveTo)
func (c *ImageCanvas) splitIntoSubpaths(commands []graphics.PathCommand) [][]graphics.PathCommand {
	var subpaths [][]graphics.PathCommand
	var currentSubpath []graphics.PathCommand

	for _, cmd := range commands {
		switch cmd.(type) {
		case *graphics.MoveTo:
			// Start a new subpath
			if len(currentSubpath) > 0 {
				subpaths = append(subpaths, currentSubpath)
			}
			currentSubpath = []graphics.PathCommand{cmd}
		default:
			if len(currentSubpath) > 0 {
				currentSubpath = append(currentSubpath, cmd)
			}
		}
	}

	if len(currentSubpath) > 0 {
		subpaths = append(subpaths, currentSubpath)
	}

	return subpaths
}

// DrawImage draws an image at the specified position and size.
func (c *ImageCanvas) DrawImage(img image.Image, x, y, width, height float64, interpolate bool) error {
	phaseX := 0.5
	phaseY := 0.5
	if !interpolate {
		phaseX = 0
		phaseY = 0
	}
	return c.DrawImageWithPhase(img, x, y, width, height, interpolate, phaseX, phaseY)
}

// DrawImageWithPhase draws an image with configurable sampling phase offsets.
func (c *ImageCanvas) DrawImageWithPhase(
	img image.Image,
	x, y, width, height float64,
	interpolate bool,
	phaseX, phaseY float64,
) error {
	return c.drawImageWithPhaseAndSamplerAndEdgeMode(
		img,
		x,
		y,
		width,
		height,
		interpolate,
		"",
		phaseX,
		phaseY,
		domainimage.ImageEdgeModeDefault,
	)
}

// DrawImageWithPhaseAndSampler draws an image with configurable sampling phase and sampler.
func (c *ImageCanvas) DrawImageWithPhaseAndSampler(
	img image.Image,
	x, y, width, height float64,
	interpolate bool,
	sampler string,
	phaseX, phaseY float64,
) error {
	return c.drawImageWithPhaseAndSamplerAndEdgeMode(
		img,
		x,
		y,
		width,
		height,
		interpolate,
		sampler,
		phaseX,
		phaseY,
		domainimage.ImageEdgeModeDefault,
	)
}

func (c *ImageCanvas) DrawImageWithPhaseSamplerAndEdgeMode(
	img image.Image,
	x, y, width, height float64,
	interpolate bool,
	sampler string,
	phaseX, phaseY float64,
	edgeMode string,
) error {
	return c.drawImageWithPhaseAndSamplerAndEdgeMode(
		img,
		x,
		y,
		width,
		height,
		interpolate,
		sampler,
		phaseX,
		phaseY,
		edgeMode,
	)
}

func (c *ImageCanvas) DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(
	img image.Image,
	mask domainimage.ImageMask,
	x, y, width, height float64,
	interpolate bool,
	sampler string,
	phaseX, phaseY float64,
	edgeMode string,
) error {
	maskImg := softMaskDrawImage(mask)
	if maskImg == nil {
		return c.drawImageWithPhaseAndSamplerAndEdgeMode(
			img,
			x,
			y,
			width,
			height,
			interpolate,
			sampler,
			phaseX,
			phaseY,
			domainimage.ImageEdgeModeDefault,
		)
	}

	dstRect := c.imageDestinationRect(x, y, width, height).Intersect(c.img.Bounds())
	if dstRect.Empty() {
		return nil
	}

	// Poppler renders the soft mask into a page-sized bitmap first, installs it
	// on Splash, then draws the RGB image with the mask active.
	srcCanvas := c.newImageScratchCanvas()
	if err := srcCanvas.drawImageWithPhaseAndSamplerAndEdgeMode(
		img,
		x,
		y,
		width,
		height,
		interpolate,
		sampler,
		phaseX,
		phaseY,
		edgeMode,
	); err != nil {
		return err
	}

	maskCanvas := c.newImageScratchCanvas()
	if err := maskCanvas.drawImageWithPhaseAndSamplerAndEdgeMode(
		maskImg,
		x,
		y,
		width,
		height,
		interpolate,
		sampler,
		phaseX,
		phaseY,
		domainimage.ImageEdgeModeDefault,
	); err != nil {
		return err
	}

	for py := dstRect.Min.Y; py < dstRect.Max.Y; py++ {
		for px := dstRect.Min.X; px < dstRect.Max.X; px++ {
			maskAlpha := maskCanvas.img.RGBAAt(px, py).R
			if maskAlpha == 0 {
				continue
			}

			srcColor := srcCanvas.img.RGBAAt(px, py)
			if srcColor.A == 0 {
				continue
			}

			srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
			if c.clipMask != nil {
				clipAlpha := c.clipMask.AlphaAt(px, py).A
				if clipAlpha == 0 {
					continue
				}
				srcMasked = applyPremultipliedAlpha(srcMasked, clipAlpha)
			}
			if srcMasked.A == 0 {
				continue
			}

			dstColor := c.img.RGBAAt(px, py)
			c.img.SetRGBA(px, py, compositeOver(dstColor, srcMasked))
		}
	}
	return nil
}

func (c *ImageCanvas) newImageScratchCanvas() *ImageCanvas {
	scratch := NewImageCanvas(c.img.Bounds()).(*ImageCanvas)
	scratch.transform = c.transform
	scratch.pageYOriginPx = c.pageYOriginPx
	return scratch
}

func (c *ImageCanvas) imageDestinationRect(x, y, width, height float64) image.Rectangle {
	p00X, p00Y := c.transformPoint(x, y)
	p10X, p10Y := c.transformPoint(x+width, y)
	p01X, p01Y := c.transformPoint(x, y+height)
	p11X, p11Y := c.transformPoint(x+width, y+height)

	minX := math.Min(math.Min(p00X, p10X), math.Min(p01X, p11X))
	maxX := math.Max(math.Max(p00X, p10X), math.Max(p01X, p11X))
	minY := math.Min(
		math.Min(float64(c.height)-p00Y, float64(c.height)-p10Y),
		math.Min(float64(c.height)-p01Y, float64(c.height)-p11Y),
	)
	maxY := math.Max(
		math.Max(float64(c.height)-p00Y, float64(c.height)-p10Y),
		math.Max(float64(c.height)-p01Y, float64(c.height)-p11Y),
	)

	return image.Rect(
		int(math.Floor(minX)),
		int(math.Floor(minY)),
		int(math.Ceil(maxX)),
		int(math.Ceil(maxY)),
	)
}

func softMaskDrawImage(mask domainimage.ImageMask) image.Image {
	if mask == nil || mask.Image() == nil {
		return nil
	}
	if !mask.IsInverted() {
		return mask.Image()
	}
	return invertedSoftMaskImage{src: mask.Image()}
}

type invertedSoftMaskImage struct {
	src image.Image
}

func (m invertedSoftMaskImage) ColorModel() color.Model {
	return color.GrayModel
}

func (m invertedSoftMaskImage) Bounds() image.Rectangle {
	return m.src.Bounds()
}

func (m invertedSoftMaskImage) At(x, y int) color.Color {
	gray := color.GrayModel.Convert(m.src.At(x, y)).(color.Gray)
	return color.Gray{Y: 255 - gray.Y}
}

func (c *ImageCanvas) drawImageWithPhaseAndSamplerAndEdgeMode(
	img image.Image,
	x, y, width, height float64,
	interpolate bool,
	sampler string,
	phaseX, phaseY float64,
	edgeMode string,
) error {
	if math.IsNaN(phaseX) || math.IsInf(phaseX, 0) {
		phaseX = 0.5
	}
	if math.IsNaN(phaseY) || math.IsInf(phaseY, 0) {
		phaseY = 0.5
	}

	srcBounds := img.Bounds()
	srcWidth := float64(srcBounds.Dx())
	srcHeight := float64(srcBounds.Dy())
	if srcWidth <= 0 || srcHeight <= 0 {
		return nil
	}
	originalImg := img
	img = flippedImage{src: img}

	// Transform rectangle corners in PDF user coordinates (including rotation/skew/scale).
	// p00 and p11 are the image's axis-aligned corners before transform.
	p00X, p00Y := c.transformPoint(x, y)
	p10X, p10Y := c.transformPoint(x+width, y)
	p01X, p01Y := c.transformPoint(x, y+height)
	p11X, p11Y := c.transformPoint(x+width, y+height)
	// Convert transformed points to canvas coordinates (Y-up -> Y-down).
	minX := math.Min(math.Min(p00X, p10X), math.Min(p01X, p11X))
	maxX := math.Max(math.Max(p00X, p10X), math.Max(p01X, p11X))
	minY := math.Min(
		math.Min(float64(c.height)-p00Y, float64(c.height)-p10Y),
		math.Min(float64(c.height)-p01Y, float64(c.height)-p11Y),
	)
	maxY := math.Max(
		math.Max(float64(c.height)-p00Y, float64(c.height)-p10Y),
		math.Max(float64(c.height)-p01Y, float64(c.height)-p11Y),
	)

	dstMinXF := minX
	dstMaxXF := maxX
	dstMinYF := minY
	dstMaxYF := maxY

	dstRect := image.Rect(
		int(math.Floor(dstMinXF)),
		int(math.Floor(dstMinYF)),
		int(math.Ceil(dstMaxXF)),
		int(math.Ceil(dstMaxYF)),
	)
	if dstRect.Empty() {
		return nil
	}

	if c.canUseAxisAlignedBoxDownscaleFastPath(
		sampler,
		p00X,
		p00Y,
		p10X,
		p10Y,
		p01X,
		p01Y,
		srcBounds,
		dstRect,
		dstMinXF,
		dstMinYF,
	) {
		c.drawAxisAlignedBoxDownscale(originalImg, srcBounds, dstRect)
		return nil
	}

	if c.canUseAxisAlignedPopplerStyle2xBoxFastPath(
		sampler,
		p00X,
		p00Y,
		p10X,
		p10Y,
		p01X,
		p01Y,
		srcBounds,
		dstRect,
		dstMinXF,
		dstMinYF,
	) {
		c.drawAxisAlignedPopplerStyle2xBox(originalImg, srcBounds, dstRect)
		return nil
	}

	if c.canUseAxisAlignedSplashDownscaleFastPath(
		sampler,
		p00X,
		p00Y,
		p10X,
		p10Y,
		p01X,
		p01Y,
		srcBounds,
	) {
		c.drawAxisAlignedSplashDownscale(originalImg, srcBounds, p00X, p00Y, p10X, p01Y)
		return nil
	}

	if c.canUseAxisAlignedSplashScaleOnlyFastPath(
		sampler,
		p00X,
		p00Y,
		p10X,
		p10Y,
		p01X,
		p01Y,
		dstMinXF,
		dstMaxXF,
		dstMinYF,
		dstMaxYF,
	) {
		c.drawAxisAlignedSplashScaleOnly(originalImg, srcBounds, dstMinXF, dstMaxXF, dstMinYF, dstMaxYF, interpolate)
		return nil
	}

	if c.canUseAxisAlignedTransparentEdgeOverWhiteFastPath(
		edgeMode,
		p00X,
		p00Y,
		p10X,
		p10Y,
		p01X,
		p01Y,
	) {
		c.drawAxisAlignedTransparentEdgeOverWhite(
			originalImg,
			dstMinXF,
			dstMinYF,
			math.Abs(p10X-p00X),
			math.Abs(p01Y-p00Y),
		)
		return nil
	}

	// Preserve exact pixels for translation-only 1:1 draws. This avoids
	// introducing interpolation/phase artifacts when the source can be copied
	// directly into the destination image.
	if c.clipMask == nil &&
		nearlyEqual(math.Abs(width), srcWidth) &&
		nearlyEqual(math.Abs(height), srcHeight) &&
		nearlyEqual(p10Y, p00Y) &&
		nearlyEqual(p01X, p00X) &&
		(p10X-p00X) > 0 &&
		(p01Y-p00Y) > 0 &&
		nearlyEqual(math.Abs(p10X-p00X), srcWidth) &&
		nearlyEqual(math.Abs(p01Y-p00Y), srcHeight) &&
		isNearlyInteger(dstMinXF) &&
		isNearlyInteger(dstMinYF) &&
		dstRect.Dx() == srcBounds.Dx() &&
		dstRect.Dy() == srcBounds.Dy() {
		clipped := dstRect.Intersect(c.img.Bounds())
		if clipped.Empty() {
			return nil
		}
		srcPoint := srcBounds.Min.Add(clipped.Min.Sub(dstRect.Min))
		draw.Draw(c.img, clipped, originalImg, srcPoint, draw.Over)
		return nil
	}

	// Build an affine transform that maps source image coordinates into canvas
	// coordinates (Y-down).
	//
	// The image coordinate space is based on source pixel indices:
	//  u-axis: (x+width, y) - (x, y)
	//  v-axis: (x, y+height) - (x, y)
	// After CTM and Y-axis inversion:
	//  dstX = p00X + a*u + b*v + phaseX*a + phaseY*b
	//  dstY = H - p00Y - c*u - d*v - phaseX*c - phaseY*d
	uScaleX := (p10X - p00X) / srcWidth
	uScaleY := (p10Y - p00Y) / srcWidth
	vScaleX := (p01X - p00X) / srcHeight
	vScaleY := (p01Y - p00Y) / srcHeight

	transform := f64.Aff3{
		uScaleX,
		vScaleX,
		p00X + uScaleX*phaseX + vScaleX*phaseY,
		-uScaleY,
		-vScaleY,
		float64(c.height) - p00Y - (uScaleY*phaseX + vScaleY*phaseY),
	}

	// Splash bilinear upscale: use when image is axis-aligned and both dimensions upscale.
	// Poppler always uses bilinear for upscaling regardless of the PDF Interpolate flag.
	if !strings.Contains(sampler, "catmull") &&
		c.canUseAxisAlignedSplashBilinear(p00X, p00Y, p10X, p10Y, p01X, p01Y, srcBounds.Dx(), srcBounds.Dy()) {
		c.drawAxisAlignedSplashBilinear(originalImg, srcBounds, p00X, p00Y, p10X, p01Y)
		return nil
	}

	// Apply clipping if set
	if c.clipMask != nil {
		// Render to a temporary image first so we can apply clip alpha manually.
		tmpRect := image.Rect(0, 0, dstRect.Dx(), dstRect.Dy())
		tmpImg := image.NewRGBA(tmpRect)
		tmpTransform := transform
		tmpTransform[2] -= float64(dstRect.Min.X)
		tmpTransform[5] -= float64(dstRect.Min.Y)
		if interpolate {
			if strings.Contains(sampler, "catmull") {
				xdraw.CatmullRom.Transform(tmpImg, tmpTransform, img, srcBounds, draw.Src, nil)
			} else {
				xdraw.ApproxBiLinear.Transform(tmpImg, tmpTransform, img, srcBounds, draw.Src, nil)
			}
		} else {
			xdraw.NearestNeighbor.Transform(tmpImg, tmpTransform, img, srcBounds, draw.Src, nil)
		}

		// Apply clip mask
		bounds := c.img.Bounds()
		for dy := 0; dy < dstRect.Dy(); dy++ {
			for dx := 0; dx < dstRect.Dx(); dx++ {
				dstX := dstRect.Min.X + dx
				dstY := dstRect.Min.Y + dy

				// Check if destination point is within bounds
				if dstX < bounds.Min.X || dstX >= bounds.Max.X ||
					dstY < bounds.Min.Y || dstY >= bounds.Max.Y {
					continue
				}

				// Get the alpha from clip mask
				maskAlpha := c.clipMask.AlphaAt(dstX, dstY).A
				if maskAlpha == 0 {
					continue // Skip pixels outside clip region
				}

				// Get the color from the temporary image
				srcColor := tmpImg.RGBAAt(tmpRect.Min.X+dx, tmpRect.Min.Y+dy)
				if srcColor.A == 0 {
					continue // Skip transparent pixels
				}

				srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
				if srcMasked.A == 0 {
					continue
				}
				dstColor := c.img.RGBAAt(dstX, dstY)
				c.img.SetRGBA(dstX, dstY, compositeOver(dstColor, srcMasked))
			}
		}
	} else {
		if interpolate {
			if strings.Contains(sampler, "catmull") {
				xdraw.CatmullRom.Transform(c.img, transform, img, srcBounds, draw.Over, nil)
			} else {
				xdraw.ApproxBiLinear.Transform(c.img, transform, img, srcBounds, draw.Over, nil)
			}
		} else {
			xdraw.NearestNeighbor.Transform(c.img, transform, img, srcBounds, draw.Over, nil)
		}
	}

	return nil
}

// Save saves the current canvas state.
func (c *ImageCanvas) Save() {
	state := &canvasState{
		transform:      c.transform,
		fillColor:      c.fillColor,
		strokeColor:    c.strokeColor,
		fillPattern:    c.fillPattern,
		strokePattern:  c.strokePattern,
		transferRed:    c.transferRed,
		transferGreen:  c.transferGreen,
		transferBlue:   c.transferBlue,
		transferGray:   c.transferGray,
		transferActive: c.transferActive,
		lineWidth:      c.lineWidth,
		lineCap:        c.lineCap,
		lineJoin:       c.lineJoin,
		miterLimit:     c.miterLimit,
		dashPattern:    c.dashPattern,
		dashPhase:      c.dashPhase,
		clipPath:       c.clipPath,
		clipRect:       c.clipRect,
		clipMask:       c.clipMask,
	}
	c.stateStack = append(c.stateStack, state)
}

// Restore restores the previous canvas state.
func (c *ImageCanvas) Restore() {
	if len(c.stateStack) == 0 {
		return
	}
	state := c.stateStack[len(c.stateStack)-1]
	c.stateStack = c.stateStack[:len(c.stateStack)-1]

	c.transform = state.transform
	c.fillColor = state.fillColor
	c.strokeColor = state.strokeColor
	c.transferRed = state.transferRed
	c.transferGreen = state.transferGreen
	c.transferBlue = state.transferBlue
	c.transferGray = state.transferGray
	c.transferActive = state.transferActive
	c.lineWidth = state.lineWidth
	c.lineCap = state.lineCap
	c.lineJoin = state.lineJoin
	c.miterLimit = state.miterLimit
	c.fillPattern = state.fillPattern
	c.strokePattern = state.strokePattern
	c.dashPattern = state.dashPattern
	c.dashPhase = state.dashPhase
	c.clipPath = state.clipPath
	c.clipRect = state.clipRect
	c.clipMask = state.clipMask
}

// Transform applies a transformation matrix.
func (c *ImageCanvas) Transform(matrix [6]float64) {
	// Multiply current transform by new transform
	newTransform := [6]float64{
		matrix[0]*c.transform[0] + matrix[2]*c.transform[1],
		matrix[1]*c.transform[0] + matrix[3]*c.transform[1],
		matrix[0]*c.transform[2] + matrix[2]*c.transform[3],
		matrix[1]*c.transform[2] + matrix[3]*c.transform[3],
		matrix[0]*c.transform[4] + matrix[2]*c.transform[5] + matrix[4],
		matrix[1]*c.transform[4] + matrix[3]*c.transform[5] + matrix[5],
	}
	c.transform = newTransform
}

// SetGlyphTransform sets the linear part [a,b,c,d] of the text rendering matrix
// for scaling/rotating glyph path coordinates from user space to device space.
func (c *ImageCanvas) SetGlyphTransform(t [4]float64) {
	c.glyphTransform = t
}

// SetPageYOriginPx sets the exact floating-point page height in canvas pixels
// (pageHeight_pt * scaleY). This is used for Y baseline calculation instead of
// the integer c.height, which may differ by a sub-pixel due to ceiling rounding.
func (c *ImageCanvas) SetPageYOriginPx(yOrigin float64) {
	c.pageYOriginPx = yOrigin
}

// canvasYOrigin returns the exact Y-down canvas origin when available.
// Poppler transforms paths and shadings in canvas space before integer clipping,
// so all PDF-space Y flips need the same origin.
func (c *ImageCanvas) canvasYOrigin() float64 {
	if c.pageYOriginPx > 0 {
		return c.pageYOriginPx
	}
	return float64(c.height)
}

// glyphYOrigin returns the float Y origin for glyph baseline computations.
func (c *ImageCanvas) glyphYOrigin() float64 {
	return c.canvasYOrigin()
}

// SetFillColor sets the fill color.
func (c *ImageCanvas) SetFillColor(c2 color.Color) {
	c.fillColor = c2
}

// SetStrokeColor sets the stroke color.
func (c *ImageCanvas) SetStrokeColor(c2 color.Color) {
	c.strokeColor = c2
}

// SetLineWidth sets the line width.
func (c *ImageCanvas) SetLineWidth(width float64) {
	c.lineWidth = width
}

// SetLineCap sets the line cap style (0=butt, 1=round, 2=square).
func (c *ImageCanvas) SetLineCap(cap int) {
	c.lineCap = cap
}

// SetLineJoin sets the line join style (0=miter, 1=round, 2=bevel).
func (c *ImageCanvas) SetLineJoin(join int) {
	c.lineJoin = join
}

// SetMiterLimit sets the miter limit.
func (c *ImageCanvas) SetMiterLimit(limit float64) {
	c.miterLimit = limit
}

// SetDashPattern sets the dash pattern.
func (c *ImageCanvas) SetDashPattern(dash []float64, phase float64) {
	c.dashPattern = dash
	c.dashPhase = phase
}

// Image returns the rendered image.
func (c *ImageCanvas) Image() image.Image {
	return c.img
}

// Reset clears the canvas and resets all state.
func (c *ImageCanvas) Reset() {
	draw.Draw(c.img, c.img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	c.currentPath = graphics.NewPath()
	c.clipPath = nil
	c.clipRect = nil
	c.clipMask = nil
	c.stateStack = nil
	c.transform = [6]float64{1, 0, 0, 1, 0, 0}
	c.fillColor = color.Black
	c.strokeColor = color.Black
	c.transferRed = [256]uint8{}
	c.transferGreen = [256]uint8{}
	c.transferBlue = [256]uint8{}
	c.transferGray = [256]uint8{}
	c.transferActive = false
	c.paperColor = color.RGBA{}
	c.paperColorActive = false
	c.lineWidth = 1.0
	c.lineCap = 0
	c.lineJoin = 0
	c.miterLimit = 10.0
	c.dashPattern = nil
	c.dashPhase = 0
	c.textPosition = [2]float64{0, 0}
	c.inTextBlock = false
}

// transformPoint transforms a point using the current transform matrix.
func (c *ImageCanvas) transformPoint(x, y float64) (float64, float64) {
	tx := c.transform[0]*x + c.transform[2]*y + c.transform[4]
	ty := c.transform[1]*x + c.transform[3]*y + c.transform[5]
	return tx, ty
}

// renderPath renders the current path (either filled or stroked).
func (c *ImageCanvas) renderPath(fill bool) {
	commands := c.currentPath.GetCommands()
	if len(commands) == 0 {
		return
	}
	if !fill {
		if len(c.dashPattern) == 0 && c.lineJoin == 0 && !shouldSkipClosedStrokeOutlinesForDebug() {
			if c.renderClosedStrokeOutlines("StrokeClosedOutlines", commands, colorToRGBA(c.strokeColor), c.lineWidth) {
				c.traceStrokeForDebug("StrokeClosedOutlines", commands, c.buildTransformedStrokeSegments(commands), c.effectiveStrokeWidth())
				return
			}
		}
		segments := c.applyDashPattern(c.buildTransformedStrokeSegments(commands))
		if len(segments) == 0 {
			return
		}
		c.traceStrokeForDebug("StrokeSegments", commands, segments, c.effectiveStrokeWidth())
		if shouldSkipStrokeSegmentsForDebug() {
			return
		}
		c.renderStrokeSegments(segments, colorToRGBA(c.strokeColor), c.effectiveStrokeWidth())
		return
	}

	if c.fillPattern != nil {
		if c.renderPathFillPattern(commands, false) {
			return
		}
	}

	bounds, ok := c.pathBounds(commands, true)
	if !ok {
		return
	}
	ras := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
	c.tracePathToRasterizer(ras, commands, bounds.Min.X, bounds.Min.Y, true, bounds)

	// Convert to RGBA if needed
	rgba := colorToRGBA(c.fillColor)

	// Apply clipping if set
	if c.clipMask != nil {
		// Render only the local path region, then apply clip mask on the same region.
		tmpImg := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		if !c.drawRasterPathSafe(
			ras,
			tmpImg,
			tmpImg.Bounds(),
			&image.Uniform{rgba},
			image.Point{},
			"renderPath fill (clip path)",
		) {
			return
		}

		canvasBounds := c.img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			localY := y - bounds.Min.Y
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				localX := x - bounds.Min.X
				// Get the alpha from clip mask
				maskAlpha := c.clipMask.AlphaAt(x, y).A
				if maskAlpha == 0 {
					continue // Skip pixels outside clip region
				}

				// Get the color from the temporary image
				srcColor := tmpImg.RGBAAt(localX, localY)
				if srcColor.A == 0 {
					continue // Skip transparent pixels
				}

				srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
				if srcMasked.A == 0 {
					continue
				}
				if x < canvasBounds.Min.X || x >= canvasBounds.Max.X ||
					y < canvasBounds.Min.Y || y >= canvasBounds.Max.Y {
					continue
				}
				dstColor := c.img.RGBAAt(x, y)
				c.img.SetRGBA(x, y, compositeOver(dstColor, srcMasked))
			}
		}
	} else {
		// No clipping, draw directly
		c.drawRasterPathSafe(
			ras,
			c.img,
			bounds,
			&image.Uniform{rgba},
			image.Point{},
			"renderPath fill",
		)
	}
}

func (c *ImageCanvas) drawRasterPathSafe(
	ras *vector.Rasterizer,
	dst draw.Image,
	bounds image.Rectangle,
	src image.Image,
	sp image.Point,
	context string,
) (ok bool) {
	if dst == nil || bounds.Empty() {
		return false
	}

	// Rasterizer internally expects destination-origin aligned paths.
	// For canvases with non-zero bounds origin (e.g. tiling pattern cells),
	// render into a temporary raster and then copy into the destination.
	if dst.Bounds().Min != (image.Point{}) {
		scratchBounds := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
		scratch := image.NewRGBA(scratchBounds)
		if tmpOK := c.drawRasterPathSafeWithScratch(ras, scratch, scratchBounds, src, sp, context); !tmpOK {
			return false
		}

		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				tmpColor := scratch.RGBAAt(x, y)
				if tmpColor.A == 0 {
					continue
				}

				dstX := bounds.Min.X + x
				dstY := bounds.Min.Y + y
				if dstX < dst.Bounds().Min.X || dstX >= dst.Bounds().Max.X ||
					dstY < dst.Bounds().Min.Y || dstY >= dst.Bounds().Max.Y {
					continue
				}
				dst.Set(dstX, dstY, tmpColor)
			}
		}

		return true
	}

	ok = true
	defer func() {
		if rec := recover(); rec != nil {
			ok = false
			fmt.Printf("Warning: rasterizer draw panic in %s (dst=%v bounds=%v): %v\n", context, dst.Bounds(), bounds, rec)
		}
	}()

	ras.Draw(dst, bounds, src, sp)
	return
}

func (c *ImageCanvas) drawRasterPathSafeWithScratch(
	ras *vector.Rasterizer,
	dst *image.RGBA,
	bounds image.Rectangle,
	src image.Image,
	sp image.Point,
	context string,
) (ok bool) {
	if dst == nil || bounds.Empty() {
		return false
	}

	ok = true
	defer func() {
		if rec := recover(); rec != nil {
			ok = false
			fmt.Printf("Warning: rasterizer draw panic in %s (scratch dst=%v bounds=%v): %v\n", context, dst.Bounds(), bounds, rec)
		}
	}()

	ras.Draw(dst, bounds, src, sp)
	return
}

func (c *ImageCanvas) renderPathFillPattern(commands []graphics.PathCommand, evenOdd bool) bool {
	if c.fillPattern == nil {
		return false
	}
	binaryClipAlpha := c.usesPopplerBinaryClipAlphaForPatternFill()

	rect, rectOK := c.transformedAxisAlignedRect(commands)
	var bounds image.Rectangle
	var ok bool
	var mask *image.Alpha
	if rectOK && !evenOdd {
		bounds = rect.pixelBounds
		ok = !bounds.Empty()
	} else {
		bounds, ok = c.pathBounds(commands, true)
		if !ok {
			return false
		}
		mask = c.createPathMask(commands, bounds, evenOdd)
		if mask == nil {
			return false
		}
	}

	patternCanvas := NewImageCanvas(c.img.Bounds()).(*ImageCanvas)
	patternCanvas.fillColor = c.fillColor
	patternCanvas.strokeColor = c.strokeColor
	patternCanvas.fillPattern = c.fillPattern
	patternCanvas.strokePattern = c.strokePattern
	patternCanvas.pageYOriginPx = c.glyphYOrigin()

	patternBBox := [4]float64{
		float64(bounds.Min.X),
		float64(bounds.Min.Y),
		float64(bounds.Max.X),
		float64(bounds.Max.Y),
	}

	switch pattern := c.fillPattern.(type) {
	case *entity.TilingPattern:
		if err := patternCanvas.DrawTilingPattern(pattern, patternBBox); err != nil {
			return false
		}
	case *entity.ShadingPattern:
		effectivePattern := entity.NewShadingPattern(pattern.Name(), pattern.GetShading())
		effectivePattern.SetMatrix(graphics.MultiplyMatrix(c.transform, pattern.Matrix()))
		if err := patternCanvas.DrawShadingPattern(effectivePattern, patternBBox); err != nil {
			return false
		}
	default:
		return false
	}

	patternImage, ok := patternCanvas.Image().(*image.RGBA)
	if !ok {
		return false
	}
	alphaRect := rect
	alphaRectOK := rectOK
	clipMask := c.clipMask
	if rectOK && c.clipRect != nil {
		intersectedRect, ok := c.intersectTransformedAxisAlignedRects(rect, *c.clipRect)
		if !ok {
			return true
		}
		alphaRect = intersectedRect
		bounds = bounds.Intersect(intersectedRect.pixelBounds)
		if bounds.Empty() {
			return true
		}
		clipMask = nil
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			alpha := c.patternFillPixelAlpha(x, y, alphaRect, alphaRectOK, mask)
			if alpha == 0 {
				continue
			}
			if clipMask != nil {
				clipAlpha := clipMask.AlphaAt(x, y).A
				if clipAlpha == 0 {
					continue
				}
				alpha = intersectClipMaskAlpha(alpha, clipAlpha)
			}
			if binaryClipAlpha && alpha > 0 {
				alpha = 255
			}

			srcColor := patternImage.RGBAAt(x, y)
			dstColor := c.img.RGBAAt(x, y)
			srcColor = applyPremultipliedAlpha(srcColor, alpha)
			if srcColor.A == 0 {
				continue
			}
			c.img.SetRGBA(x, y, c.applyColorTransfer(compositeOver(dstColor, srcColor)))
		}
	}
	return true
}

func (c *ImageCanvas) usesPopplerBinaryClipAlphaForPatternFill() bool {
	_, _, _, fillAlpha := c.fillColor.RGBA()
	if fillAlpha != 0xffff {
		return false
	}
	pattern, ok := c.fillPattern.(*entity.ShadingPattern)
	if !ok {
		return false
	}
	shading := pattern.GetShading()
	if shading == nil {
		return false
	}
	switch shading.GetShadingType() {
	case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud:
		return true
	default:
		return false
	}
}

type transformedAxisAlignedRect struct {
	minX        float64
	minY        float64
	maxX        float64
	maxY        float64
	pixelBounds image.Rectangle
}

func (c *ImageCanvas) pathBounds(commands []graphics.PathCommand, applyTransform bool) (image.Rectangle, bool) {
	minX := math.MaxFloat64
	minY := math.MaxFloat64
	maxX := -math.MaxFloat64
	maxY := -math.MaxFloat64
	hasPoint := false
	heightF := c.canvasYOrigin()
	boundsOffsetX := float64(c.img.Bounds().Min.X)
	boundsOffsetY := float64(c.img.Bounds().Min.Y)

	updateBounds := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
		hasPoint = true
	}

	var startX, startY float64
	hasCurrent := false

	toCanvasXY := func(x, y float64) (float64, float64) {
		if applyTransform {
			tx, ty := c.transformPoint(x, y)
			return tx - boundsOffsetX, heightF - ty + boundsOffsetY
		}
		// applyTransform=false: coordinates are already in canvas space (Y-down)
		// No Y-flip needed
		return x - boundsOffsetX, y + boundsOffsetY
	}

	for _, command := range commands {
		switch cmd := command.(type) {
		case *graphics.MoveTo:
			startX, startY = toCanvasXY(cmd.X, cmd.Y)
			hasCurrent = true
			updateBounds(startX, startY)
		case *graphics.LineTo:
			if !hasCurrent {
				// Cairo behavior: lineto without current point acts as moveto
				startX, startY = toCanvasXY(cmd.X, cmd.Y)
				hasCurrent = true
				updateBounds(startX, startY)
				continue
			}
			nextX, nextY := toCanvasXY(cmd.X, cmd.Y)
			updateBounds(nextX, nextY)
		case *graphics.CurveTo:
			if !hasCurrent {
				// Cairo behavior: curveto without current point uses first control point as implicit moveto
				startX, startY = toCanvasXY(cmd.X1, cmd.Y1)
				hasCurrent = true
				updateBounds(startX, startY)
			}
			c1x, c1y := toCanvasXY(cmd.X1, cmd.Y1)
			c2x, c2y := toCanvasXY(cmd.X2, cmd.Y2)
			endX, endY := toCanvasXY(cmd.X3, cmd.Y3)
			updateBounds(c1x, c1y)
			updateBounds(c2x, c2y)
			updateBounds(endX, endY)
		case *graphics.ClosePath:
			if !hasCurrent {
				continue
			}
			updateBounds(startX, startY)
		}
	}

	if !hasPoint {
		return image.Rectangle{}, false
	}

	clampedMinX := math.Max(0, math.Floor(minX))
	clampedMinY := math.Max(0, math.Floor(minY))
	clampedMaxX := math.Min(float64(c.width), math.Ceil(maxX))
	clampedMaxY := math.Min(float64(c.height), math.Ceil(maxY))
	if clampedMaxX <= clampedMinX || clampedMaxY <= clampedMinY {
		return image.Rectangle{}, false
	}

	return image.Rect(int(clampedMinX), int(clampedMinY), int(clampedMaxX), int(clampedMaxY)), true
}

func (c *ImageCanvas) transformedAxisAlignedRect(commands []graphics.PathCommand) (transformedAxisAlignedRect, bool) {
	if len(commands) != 5 {
		return transformedAxisAlignedRect{}, false
	}

	move, ok := commands[0].(*graphics.MoveTo)
	if !ok {
		return transformedAxisAlignedRect{}, false
	}
	line1, ok := commands[1].(*graphics.LineTo)
	if !ok {
		return transformedAxisAlignedRect{}, false
	}
	line2, ok := commands[2].(*graphics.LineTo)
	if !ok {
		return transformedAxisAlignedRect{}, false
	}
	line3, ok := commands[3].(*graphics.LineTo)
	if !ok {
		return transformedAxisAlignedRect{}, false
	}
	if _, ok := commands[4].(*graphics.ClosePath); !ok {
		return transformedAxisAlignedRect{}, false
	}

	p0x, p0y := c.toImageSpacePoint(move.X, move.Y)
	p1x, p1y := c.toImageSpacePoint(line1.X, line1.Y)
	p2x, p2y := c.toImageSpacePoint(line2.X, line2.Y)
	p3x, p3y := c.toImageSpacePoint(line3.X, line3.Y)
	points := [4][2]float64{
		{p0x, p0y},
		{p1x, p1y},
		{p2x, p2y},
		{p3x, p3y},
	}

	const eps = 1e-6
	if !approxEqual(points[0][1], points[1][1], eps) ||
		!approxEqual(points[1][0], points[2][0], eps) ||
		!approxEqual(points[2][1], points[3][1], eps) ||
		!approxEqual(points[3][0], points[0][0], eps) {
		return transformedAxisAlignedRect{}, false
	}

	minX := math.Min(points[0][0], points[2][0])
	maxX := math.Max(points[0][0], points[2][0])
	minY := math.Min(points[0][1], points[2][1])
	maxY := math.Max(points[0][1], points[2][1])
	if maxX <= minX || maxY <= minY {
		return transformedAxisAlignedRect{}, false
	}

	pixelBounds := image.Rect(
		int(math.Max(0, math.Floor(minX))),
		int(math.Max(0, math.Floor(minY))),
		int(math.Min(float64(c.width), math.Ceil(maxX))),
		int(math.Min(float64(c.height), math.Ceil(maxY))),
	)
	if pixelBounds.Empty() {
		return transformedAxisAlignedRect{}, false
	}

	return transformedAxisAlignedRect{
		minX:        minX,
		minY:        minY,
		maxX:        maxX,
		maxY:        maxY,
		pixelBounds: pixelBounds,
	}, true
}

func (c *ImageCanvas) intersectTransformedAxisAlignedRects(
	left transformedAxisAlignedRect,
	right transformedAxisAlignedRect,
) (transformedAxisAlignedRect, bool) {
	minX := math.Max(left.minX, right.minX)
	minY := math.Max(left.minY, right.minY)
	maxX := math.Min(left.maxX, right.maxX)
	maxY := math.Min(left.maxY, right.maxY)
	if maxX <= minX || maxY <= minY {
		return transformedAxisAlignedRect{}, false
	}

	pixelBounds := image.Rect(
		int(math.Max(0, math.Floor(minX))),
		int(math.Max(0, math.Floor(minY))),
		int(math.Min(float64(c.width), math.Ceil(maxX))),
		int(math.Min(float64(c.height), math.Ceil(maxY))),
	)
	if pixelBounds.Empty() {
		return transformedAxisAlignedRect{}, false
	}

	return transformedAxisAlignedRect{
		minX:        minX,
		minY:        minY,
		maxX:        maxX,
		maxY:        maxY,
		pixelBounds: pixelBounds,
	}, true
}

func (c *ImageCanvas) maskForTransformedRect(rect transformedAxisAlignedRect) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, c.width, c.height))
	bounds := rect.pixelBounds.Intersect(mask.Bounds())
	if bounds.Empty() {
		return mask
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			alpha := c.patternFillPixelAlpha(x, y, rect, true, nil)
			if alpha == 0 {
				continue
			}
			mask.SetAlpha(x, y, color.Alpha{A: alpha})
		}
	}

	return mask
}

func (c *ImageCanvas) toImageSpacePoint(x, y float64) (float64, float64) {
	tx, ty := c.transformPoint(x, y)
	return tx - float64(c.img.Bounds().Min.X), c.canvasYOrigin() - ty + float64(c.img.Bounds().Min.Y)
}

func (c *ImageCanvas) patternFillPixelAlpha(
	x, y int,
	rect transformedAxisAlignedRect,
	rectOK bool,
	mask *image.Alpha,
) uint8 {
	if rectOK {
		return splashRectCoverageAlpha(x, y, rect)
	}
	if mask == nil {
		return 0
	}
	return mask.AlphaAt(x, y).A
}

var splashAAGamma = [...]uint8{0, 4, 11, 21, 32, 45, 59, 74, 90, 108, 126, 145, 166, 187, 209, 231, 255}

func splashRectCoverageAlpha(x, y int, rect transformedAxisAlignedRect) uint8 {
	const aaSize = 4

	yMinI := int(math.Floor(rect.minY))
	yMaxI := int(math.Ceil(rect.maxY)) - 1
	if y < yMinI || y > yMaxI {
		return 0
	}

	firstSubpixel := x * aaSize
	lastSubpixel := firstSubpixel + aaSize - 1
	rectFirstSubpixel := int(math.Floor(rect.minX * aaSize))
	rectLastSubpixel := int(math.Ceil(rect.maxX*aaSize)) - 1
	if rectFirstSubpixel > firstSubpixel {
		firstSubpixel = rectFirstSubpixel
	}
	if rectLastSubpixel < lastSubpixel {
		lastSubpixel = rectLastSubpixel
	}
	if lastSubpixel < firstSubpixel {
		return 0
	}

	xCoverage := lastSubpixel - firstSubpixel + 1
	if xCoverage > aaSize {
		xCoverage = aaSize
	}
	return splashAAGamma[xCoverage*aaSize]
}

func approxEqual(left, right, eps float64) bool {
	return math.Abs(left-right) <= eps
}

func (c *ImageCanvas) tracePathToRasterizer(
	ras *vector.Rasterizer,
	commands []graphics.PathCommand,
	offsetX int,
	offsetY int,
	applyTransform bool,
	bounds image.Rectangle,
) {
	heightF := c.canvasYOrigin()
	widthF := float64(bounds.Dx())
	traceHeightF := float64(bounds.Dy())
	ox := float64(offsetX)
	oy := float64(offsetY)
	hasCurrent := false

	toLocalXY := func(x, y float64) (float32, float32) {
		if applyTransform {
			tx, ty := c.transformPoint(x, y)
			localX := tx - ox
			localY := heightF - ty - oy
			if localX < 0 {
				localX = 0
			}
			if localX > widthF {
				localX = widthF
			}
			if localY < 0 {
				localY = 0
			}
			if localY > traceHeightF {
				localY = traceHeightF
			}
			return float32(localX), float32(localY)
		}
		// applyTransform=false: coordinates are already in canvas space (Y-down)
		// No Y-flip needed
		localX := x - ox
		localY := y - oy
		if localX < 0 {
			localX = 0
		}
		if localX > widthF {
			localX = widthF
		}
		if localY < 0 {
			localY = 0
		}
		if localY > traceHeightF {
			localY = traceHeightF
		}
		return float32(localX), float32(localY)
	}

	for _, command := range commands {
		switch cmd := command.(type) {
		case *graphics.MoveTo:
			x, y := toLocalXY(cmd.X, cmd.Y)
			ras.MoveTo(x, y)
			hasCurrent = true
		case *graphics.LineTo:
			if !hasCurrent {
				// Cairo behavior: lineto without current point acts as moveto
				x, y := toLocalXY(cmd.X, cmd.Y)
				ras.MoveTo(x, y)
				hasCurrent = true
				continue
			}
			x, y := toLocalXY(cmd.X, cmd.Y)
			ras.LineTo(x, y)
		case *graphics.CurveTo:
			if !hasCurrent {
				// Cairo behavior: curveto without current point uses first control point as implicit moveto
				c1x, c1y := toLocalXY(cmd.X1, cmd.Y1)
				ras.MoveTo(c1x, c1y)
				hasCurrent = true
			}
			c1x, c1y := toLocalXY(cmd.X1, cmd.Y1)
			c2x, c2y := toLocalXY(cmd.X2, cmd.Y2)
			endX, endY := toLocalXY(cmd.X3, cmd.Y3)
			ras.CubeTo(c1x, c1y, c2x, c2y, endX, endY)
		case *graphics.ClosePath:
			if !hasCurrent {
				continue
			}
			ras.ClosePath()
		}
	}
}

func (c *ImageCanvas) renderStrokeSegments(segments []lineSegment, stroke color.RGBA, lineWidth float64) {
	if lineWidth <= 0 {
		return
	}
	if len(segments) == 0 {
		return
	}
	if c.renderRoundCapJoinStrokeOutlines(segments, stroke, lineWidth) {
		return
	}

	subpaths := splitStrokeSegmentSubpaths(segments)
	fallbackSegments := make([]lineSegment, 0, len(segments))
	specializedOutlines := make([][]strokePoint, 0)
	for _, subpath := range subpaths {
		outlines := c.openMiterStrokeSubpathOutlines(subpath, lineWidth)
		if len(outlines) > 0 {
			specializedOutlines = append(specializedOutlines, outlines...)
			continue
		}
		fallbackSegments = append(fallbackSegments, subpath.segments...)
	}
	if len(specializedOutlines) > 0 {
		c.renderStrokeOutlines(specializedOutlines, stroke)
	}
	if len(fallbackSegments) == 0 {
		return
	}

	minX, minY, maxX, maxY := segmentBounds(fallbackSegments)
	pad := int(math.Ceil(lineWidth/2.0)) + 2
	minX -= pad
	minY -= pad
	maxX += pad
	maxY += pad
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > c.width {
		maxX = c.width
	}
	if maxY > c.height {
		maxY = c.height
	}
	if minX >= maxX || minY >= maxY {
		return
	}

	tmpRect := image.Rect(minX, minY, maxX, maxY)
	tmpImg := image.NewRGBA(tmpRect)
	stroke = premultiplyColor(stroke)

	// Track which fallback segments are 1-segment subpaths so butt-cap clipping
	// can be safely applied at both endpoints. For multi-segment subpaths the
	// internal join points must remain unclipped to avoid gaps at miter joins.
	soloEndpoint := make(map[int]bool, len(fallbackSegments))
	idx := 0
	for _, subpath := range subpaths {
		// Skip subpaths consumed by openMiterStrokeSubpathOutlines specialization.
		if c.openMiterStrokeSubpathOutlinesConsumed(subpath, lineWidth) {
			continue
		}
		if len(subpath.segments) == 1 {
			soloEndpoint[idx] = true
		}
		idx += len(subpath.segments)
	}
	for i, s := range fallbackSegments {
		cap := c.lineCap
		if !soloEndpoint[i] {
			// Treat segment as joined at both ends → use round-cap-equivalent
			// (which is harmless since adjacent segment overlaps it at the join).
			cap = 1
		}
		drawLineSegment(tmpImg, s.x1, s.y1, s.x2, s.y2, stroke, lineWidth, cap)
	}

	c.drawTempImageWithClip(tmpImg)
}

func (c *ImageCanvas) openMiterStrokeSubpathOutlines(subpath strokeSegmentSubpath, lineWidth float64) [][]strokePoint {
	if subpath.closed {
		return nil
	}
	if !c.shouldRenderOpenMiterStrokeOutlines(subpath.segments, lineWidth) {
		return nil
	}
	outline, ok := buildOpenMiterJoinStrokeOutline(subpath.segments, lineWidth/2, c.miterLimit)
	if !ok {
		return nil
	}
	return [][]strokePoint{outline}
}

func (c *ImageCanvas) openMiterStrokeSubpathOutlinesConsumed(subpath strokeSegmentSubpath, lineWidth float64) bool {
	if subpath.closed {
		return false
	}
	if !c.shouldRenderOpenMiterStrokeOutlines(subpath.segments, lineWidth) {
		return false
	}
	_, ok := buildOpenMiterJoinStrokeOutline(subpath.segments, lineWidth/2, c.miterLimit)
	return ok
}

func (c *ImageCanvas) renderRoundCapJoinStrokeOutlines(segments []lineSegment, stroke color.RGBA, lineWidth float64) bool {
	if !c.shouldRenderRoundCapJoinStrokeOutlines(segments, lineWidth) {
		return false
	}

	outlines := buildRoundCapJoinStrokeOutlines(segments, lineWidth/2)
	if len(outlines) == 0 {
		return false
	}
	return c.renderStrokeOutlines(outlines, stroke)
}

func (c *ImageCanvas) renderStrokeOutlines(outlines [][]strokePoint, stroke color.RGBA) bool {
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	for _, outline := range outlines {
		for _, point := range outline {
			if point.x < minX {
				minX = point.x
			}
			if point.x > maxX {
				maxX = point.x
			}
			if point.y < minY {
				minY = point.y
			}
			if point.y > maxY {
				maxY = point.y
			}
		}
	}

	bounds := image.Rect(
		int(math.Max(0, math.Floor(minX-2))),
		int(math.Max(0, math.Floor(minY-2))),
		int(math.Min(float64(c.width), math.Ceil(maxX+2))),
		int(math.Min(float64(c.height), math.Ceil(maxY+2))),
	)
	if bounds.Empty() {
		return true
	}

	if !shouldSkipPopplerAAOutlinesForDebug() {
		if tmpImg, ok := rasterizePopplerAAOutlines(outlines, bounds, stroke, c.width, c.height); ok {
			c.drawTempImageWithClip(tmpImg)
			return true
		}
	}

	ras := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
	for _, outline := range outlines {
		if len(outline) == 0 {
			continue
		}
		ras.MoveTo(float32(outline[0].x-float64(bounds.Min.X)), float32(outline[0].y-float64(bounds.Min.Y)))
		for _, point := range outline[1:] {
			ras.LineTo(float32(point.x-float64(bounds.Min.X)), float32(point.y-float64(bounds.Min.Y)))
		}
		ras.ClosePath()
	}

	tmpImg := image.NewRGBA(bounds)
	if !c.drawRasterPathSafe(ras, tmpImg, tmpImg.Bounds(), &image.Uniform{stroke}, image.Point{}, "renderRoundCapJoinStrokeOutlines") {
		return false
	}
	c.drawTempImageWithClip(tmpImg)
	return true
}

func (c *ImageCanvas) shouldRenderRoundCapJoinStrokeOutlines(segments []lineSegment, lineWidth float64) bool {
	if lineWidth <= 0 {
		return false
	}
	if len(segments) < 2 {
		return false
	}
	if len(c.dashPattern) != 0 {
		return false
	}
	if c.lineCap != 1 || c.lineJoin != 1 {
		return false
	}
	return !strokeSegmentsAreAxisAligned(segments)
}

func (c *ImageCanvas) shouldRenderOpenMiterStrokeOutlines(segments []lineSegment, lineWidth float64) bool {
	if lineWidth <= 0 {
		return false
	}
	if len(segments) < 2 {
		return false
	}
	if len(c.dashPattern) != 0 {
		return false
	}
	if c.lineCap != 0 || c.lineJoin != 0 {
		return false
	}
	return !strokeSegmentsAreAxisAligned(segments)
}

func (c *ImageCanvas) renderClosedStrokeOutlines(debugLabel string, commands []graphics.PathCommand, stroke color.RGBA, lineWidth float64) bool {
	if lineWidth <= 0 {
		return false
	}
	subpaths, ok := c.buildClosedLinearStrokeSubpaths(commands)
	if !ok || len(subpaths) == 0 {
		return false
	}

	outlines := make([][]strokePoint, 0, len(subpaths)*2)
	for index, subpath := range subpaths {
		left, right, ok := c.closedStrokeOutlineSides(subpath.points, lineWidth)
		if !ok {
			return false
		}
		c.traceClosedStrokeSubpathForDebug(debugLabel, index, subpath.points, lineWidth)
		left = c.transformStrokeOutlinePoints(left)
		right = c.transformStrokeOutlinePoints(right)
		if math.Abs(strokePolygonArea(right)) > math.Abs(strokePolygonArea(left)) {
			left, right = right, left
		}
		outlines = append(outlines, left, reverseStrokePoints(right))
	}
	if len(outlines) == 0 {
		return false
	}
	// Capture the pre-strokeAdjust outlines so the strokeNarrow integer-quad
	// fast path can verify the snap-rounding produces a non-degenerate ring
	// (outer strictly contains inner after rounding). When the original
	// outlines round to outer == inner the snap collapses the ring; we must
	// fall back to the AA scanner in that case.
	preAdjustOutlines := outlines
	if !shouldSkipStrokeAdjustOutlinesForDebug() {
		outlines = strokeAdjustAxisAlignedOutlines(outlines)
	}

	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	for _, outline := range outlines {
		for _, point := range outline {
			if point.x < minX {
				minX = point.x
			}
			if point.x > maxX {
				maxX = point.x
			}
			if point.y < minY {
				minY = point.y
			}
			if point.y > maxY {
				maxY = point.y
			}
		}
	}

	bounds := image.Rect(
		int(math.Max(0, math.Floor(minX-2))),
		int(math.Max(0, math.Floor(minY-2))),
		int(math.Min(float64(c.width), math.Ceil(maxX+2))),
		int(math.Min(float64(c.height), math.Ceil(maxY+2))),
	)
	if bounds.Empty() {
		return true
	}

	// Poppler strokeAdjust path: when the closed-stroke outlines describe a
	// pair of integer-aligned outer + inner axis-aligned rectangles (the
	// result of stroking a single rectangle after strokeAdjustAxisAlignedOutlines
	// rounded every point to integer device coordinates), Poppler renders the
	// ring as four independent integer-aligned bars (one per edge), each
	// filled with full alpha. There is no inner-edge AA bleed because every
	// boundary lies on an integer pixel row/column. Match that behavior with
	// a direct integer-rect fill instead of routing through the AA-supersampled
	// scanner, which otherwise emits a faint AA tail across the inside of the
	// ring at the supersample row coinciding with the inner top/bottom edge.
	if !shouldSkipStrokeNarrowQuadsForDebug() {
		if tmpImg, ok := rasterizeStrokeNarrowAxisAlignedQuads(outlines, preAdjustOutlines, bounds, stroke); ok {
			c.drawTempImageWithClip(tmpImg)
			return true
		}
	}

	if !shouldSkipPopplerAAOutlinesForDebug() {
		if tmpImg, ok := rasterizePopplerAAOutlines(outlines, bounds, stroke, c.width, c.height); ok {
			c.drawTempImageWithClip(tmpImg)
			return true
		}
	}

	ras := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
	for _, outline := range outlines {
		if len(outline) == 0 {
			continue
		}
		ras.MoveTo(float32(outline[0].x-float64(bounds.Min.X)), float32(outline[0].y-float64(bounds.Min.Y)))
		for _, point := range outline[1:] {
			ras.LineTo(float32(point.x-float64(bounds.Min.X)), float32(point.y-float64(bounds.Min.Y)))
		}
		ras.ClosePath()
	}

	tmpImg := image.NewRGBA(bounds)
	if !c.drawRasterPathSafe(ras, tmpImg, tmpImg.Bounds(), &image.Uniform{stroke}, image.Point{}, "renderClosedStrokeOutlines") {
		return false
	}
	c.drawTempImageWithClip(tmpImg)
	return true
}

func (c *ImageCanvas) traceClosedStrokeSubpathForDebug(label string, index int, points []strokePoint, lineWidth float64) {
	if !shouldTraceStrokeSubpathsForDebug() {
		return
	}
	userMinX, userMinY, userMaxX, userMaxY := strokePointBounds(points)
	devicePoints := c.transformStrokeOutlinePoints(points)
	devMinX, devMinY, devMaxX, devMaxY := strokePointBounds(devicePoints)
	fmt.Fprintf(
		os.Stderr,
		"PDF_STROKE_SUBPATH op=%s index=%d points=%d line_width=%.6f axis_aligned=%t user_bounds=%.6f,%.6f,%.6f,%.6f device_bounds=%.6f,%.6f,%.6f,%.6f device_size=%.6f,%.6f area=%.6f\n",
		label,
		index,
		len(points),
		lineWidth,
		strokePolylineIsAxisAligned(devicePoints),
		userMinX,
		userMinY,
		userMaxX,
		userMaxY,
		devMinX,
		devMinY,
		devMaxX,
		devMaxY,
		devMaxX-devMinX,
		devMaxY-devMinY,
		math.Abs(strokePolygonArea(devicePoints)),
	)
}

func (c *ImageCanvas) buildClosedLinearStrokeSubpaths(commands []graphics.PathCommand) ([]closedStrokeSubpath, bool) {
	subpaths := make([]closedStrokeSubpath, 0, 1)
	current := make([]strokePoint, 0, len(commands))
	hasCurrent := false
	closed := false

	flush := func() bool {
		if len(current) == 0 {
			return true
		}
		if !closed || len(current) < 2 {
			return false
		}
		points := append([]strokePoint(nil), current...)
		subpaths = append(subpaths, closedStrokeSubpath{points: points})
		current = current[:0]
		hasCurrent = false
		closed = false
		return true
	}

	for _, command := range commands {
		switch cmd := command.(type) {
		case *graphics.MoveTo:
			if !flush() {
				return nil, false
			}
			current = append(current, strokePoint{x: cmd.X, y: cmd.Y})
			hasCurrent = true
		case *graphics.LineTo:
			point := strokePoint{x: cmd.X, y: cmd.Y}
			if !hasCurrent {
				current = append(current, point)
				hasCurrent = true
				continue
			}
			current = append(current, point)
		case *graphics.CurveTo:
			return nil, false
		case *graphics.ClosePath:
			if !hasCurrent {
				continue
			}
			if len(current) > 1 {
				closed = true
				if !flush() {
					return nil, false
				}
			}
		}
	}
	if !flush() {
		return nil, false
	}
	return subpaths, len(subpaths) > 0
}

func (c *ImageCanvas) transformStrokeOutlinePoints(points []strokePoint) []strokePoint {
	out := make([]strokePoint, len(points))
	yOrigin := c.canvasYOrigin()
	for index, point := range points {
		tx, ty := c.transformPoint(point.x, point.y)
		out[index] = strokePoint{x: tx, y: yOrigin - ty}
	}
	return out
}

func (c *ImageCanvas) closedStrokeOutlineSides(points []strokePoint, lineWidth float64) ([]strokePoint, []strokePoint, bool) {
	cleaned := removeDuplicateStrokeClosure(points)
	if len(cleaned) < 2 {
		return nil, nil, false
	}

	halfWidth := lineWidth / 2
	miterLimit := c.miterLimit
	if miterLimit <= 0 {
		miterLimit = 10
	}

	left := make([]strokePoint, len(cleaned))
	right := make([]strokePoint, len(cleaned))
	for index := range cleaned {
		prev := cleaned[(index+len(cleaned)-1)%len(cleaned)]
		cur := cleaned[index]
		next := cleaned[(index+1)%len(cleaned)]

		leftPoint, ok := strokeMiterPoint(prev, cur, next, halfWidth, 1, miterLimit)
		if !ok {
			return nil, nil, false
		}
		rightPoint, ok := strokeMiterPoint(prev, cur, next, halfWidth, -1, miterLimit)
		if !ok {
			return nil, nil, false
		}
		left[index] = leftPoint
		right[index] = rightPoint
	}
	return left, right, true
}

func (c *ImageCanvas) effectiveStrokeWidth() float64 {
	if c.lineWidth <= 0 {
		return c.lineWidth
	}
	return c.lineWidth * c.effectiveStrokeScale()
}

func (c *ImageCanvas) effectiveStrokeScale() float64 {
	scaleX := math.Hypot(c.transform[0], c.transform[1])
	scaleY := math.Hypot(c.transform[2], c.transform[3])

	switch {
	case scaleX > 0 && scaleY > 0:
		return (scaleX + scaleY) / 2
	case scaleX > 0:
		return scaleX
	case scaleY > 0:
		return scaleY
	default:
		return 1
	}
}

func (c *ImageCanvas) applyDashPattern(segments []lineSegment) []lineSegment {
	pattern, phase, ok := c.effectiveDashPattern()
	if !ok || len(segments) == 0 {
		return segments
	}

	index := 0
	draw := true
	remaining := pattern[index]
	advance := func() {
		index = (index + 1) % len(pattern)
		draw = !draw
		remaining = pattern[index]
	}

	for phase > 1e-9 {
		if remaining <= 1e-9 {
			advance()
			continue
		}
		step := math.Min(remaining, phase)
		remaining -= step
		phase -= step
		if remaining <= 1e-9 {
			advance()
		}
	}

	dashed := make([]lineSegment, 0, len(segments))
	for _, segment := range segments {
		length := segment.length()
		if length <= 1e-9 {
			continue
		}

		consumed := 0.0
		for consumed < length-1e-9 {
			if remaining <= 1e-9 {
				advance()
				continue
			}
			step := math.Min(remaining, length-consumed)
			if draw && step > 1e-9 {
				startT := consumed / length
				endT := (consumed + step) / length
				dashed = append(dashed, segment.slice(startT, endT))
			}
			consumed += step
			remaining -= step
			if remaining <= 1e-9 {
				advance()
			}
		}
	}

	return dashed
}

func (c *ImageCanvas) effectiveDashPattern() ([]float64, float64, bool) {
	if len(c.dashPattern) == 0 {
		return nil, 0, false
	}

	scale := c.effectiveStrokeScale()
	if scale <= 0 {
		scale = 1
	}

	pattern := make([]float64, 0, len(c.dashPattern)*2)
	total := 0.0
	for _, entry := range c.dashPattern {
		length := math.Abs(entry) * scale
		pattern = append(pattern, length)
		total += length
	}
	if len(pattern)%2 == 1 {
		dup := append([]float64(nil), pattern...)
		pattern = append(pattern, dup...)
		total *= 2
	}
	if total <= 1e-9 {
		return nil, 0, false
	}

	phase := math.Mod(math.Abs(c.dashPhase)*scale, total)
	return pattern, phase, true
}

// snapToHalfInt snaps v to the nearest half-integer (pixel center).
// E.g. 75.169 → 75.5, 74.3 → 74.5, 75.8 → 75.5 (or 76.5 if > 75.75...)
func snapToHalfInt(v float64) float64 {
	return math.Floor(v) + 0.5
}

func drawLineSegment(dst *image.RGBA, x1, y1, x2, y2 float64, col color.RGBA, width float64, lineCap int) {
	// For thin axis-aligned strokes, snap center to nearest pixel center and
	// width to integer to match Cairo's hint_metrics behavior. For butt-capped
	// thin strokes we also snap endpoints in the stroke direction to match
	// Poppler's strokeAdjust-on behavior (eliminates sub-pixel AA at endpoints).
	const thinStrokeThreshold = 1.5
	// Track original (pre-snap) endpoints for axis-aligned BUTT bleed.
	origX1, origX2, origY1, origY2 := x1, y1, x2, y2
	thinAxisAlignedHorizontal := false
	thinAxisAlignedVertical := false
	if width < thinStrokeThreshold {
		dx := math.Abs(x2 - x1)
		dy := math.Abs(y2 - y1)
		if dy < 0.5 && dx > dy { // horizontal
			cy := (y1 + y2) / 2.0
			snapped := math.Round(cy-0.5) + 0.5
			y1, y2 = snapped, snapped
			width = math.Max(1.0, math.Round(width))
			if lineCap == 0 {
				x1 = math.Round(x1)
				x2 = math.Round(x2)
				thinAxisAlignedHorizontal = true
			}
		} else if dx < 0.5 && dy > dx { // vertical
			cx := (x1 + x2) / 2.0
			snapped := math.Round(cx-0.5) + 0.5
			x1, x2 = snapped, snapped
			width = math.Max(1.0, math.Round(width))
			if lineCap == 0 {
				y1 = math.Round(y1)
				y2 = math.Round(y2)
				thinAxisAlignedVertical = true
			}
		}
	}

	radius := width / 2
	if radius <= 0 {
		return
	}

	// Cap extension beyond endpoints: 0 for butt, radius for round/square.
	endExtend := radius
	if lineCap == 0 {
		endExtend = 0
	}
	minX := int(math.Floor(math.Min(x1, x2) - endExtend - 1))
	maxX := int(math.Ceil(math.Max(x1, x2) + endExtend + 1))
	minY := int(math.Floor(math.Min(y1, y2) - endExtend - 1))
	maxY := int(math.Ceil(math.Max(y1, y2) + endExtend + 1))

	b := dst.Bounds()
	if minX < b.Min.X {
		minX = b.Min.X
	}
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxX > b.Max.X {
		maxX = b.Max.X
	}
	if maxY > b.Max.Y {
		maxY = b.Max.Y
	}
	if minX >= maxX || minY >= maxY {
		return
	}

	butt := lineCap == 0
	segDX := x2 - x1
	segDY := y2 - y1
	segLenSq := segDX*segDX + segDY*segDY
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			const aaSize = 4
			coverageCount := 0
			for yy := 0; yy < aaSize; yy++ {
				py := float64(y) + (float64(yy)+0.5)/aaSize
				for xx := 0; xx < aaSize; xx++ {
					px := float64(x) + (float64(xx)+0.5)/aaSize
					if butt && segLenSq > 0 {
						t := ((px-x1)*segDX + (py-y1)*segDY) / segLenSq
						if t < 0 || t > 1 {
							continue
						}
					}
					if distancePointToSegment(px, py, x1, y1, x2, y2) <= radius {
						coverageCount++
					}
				}
			}
			if coverageCount == 0 {
				continue
			}

			src := col
			if coverageCount < aaSize*aaSize {
				src = applyPremultipliedAlpha(src, splashAAGamma[coverageCount])
			}
			if src.A == 0 {
				continue
			}
			if src.A == 255 {
				dst.SetRGBA(x, y, src)
				continue
			}
			dst.SetRGBA(x, y, splashCompositeOver(dst.RGBAAt(x, y), src))
		}
	}

	// Endpoint AA bleed for thin axis-aligned BUTT-capped strokes: when an
	// endpoint was rounded toward the segment interior (frac > 0.5 for left,
	// < 0.5 for right), Poppler renders a faint AA pixel one step outside the
	// snapped boundary. Approximate via LUT lookup proportional to the lost
	// sub-pixel extension width (empirically multiplier 12 matches Poppler's
	// gamma-corrected coverage for both small and medium extensions).
	if thinAxisAlignedHorizontal {
		px := int(y1 - 0.5) // snapped row
		if px >= b.Min.Y && px < b.Max.Y {
			leftFrac := origX1 - math.Floor(origX1)
			rightFrac := origX2 - math.Floor(origX2)
			if leftFrac > 0.5 {
				bleedX := int(math.Floor(origX1))
				if bleedX >= b.Min.X && bleedX < b.Max.X && bleedX < int(math.Min(x1, x2)) {
					applyEndpointBleed(dst, bleedX, px, col, 1.0-leftFrac)
				}
			}
			if rightFrac > 0.5 {
				bleedX := int(math.Round(origX2))
				if bleedX >= b.Min.X && bleedX < b.Max.X && bleedX > int(math.Max(x1, x2))-1 {
					applyEndpointBleed(dst, bleedX, px, col, 1.0-rightFrac)
				}
			}
			_ = origY1
			_ = origY2
		}
	}
	if thinAxisAlignedVertical {
		py := int(x1 - 0.5)
		if py >= b.Min.X && py < b.Max.X {
			topFrac := origY1 - math.Floor(origY1)
			bottomFrac := origY2 - math.Floor(origY2)
			if topFrac > 0.5 {
				bleedY := int(math.Floor(origY1))
				if bleedY >= b.Min.Y && bleedY < b.Max.Y && bleedY < int(math.Min(y1, y2)) {
					applyEndpointBleed(dst, py, bleedY, col, 1.0-topFrac)
				}
			}
			if bottomFrac > 0.5 {
				bleedY := int(math.Round(origY2))
				if bleedY >= b.Min.Y && bleedY < b.Max.Y && bleedY > int(math.Max(y1, y2))-1 {
					applyEndpointBleed(dst, py, bleedY, col, 1.0-bottomFrac)
				}
			}
			_ = origX1
			_ = origX2
		}
	}
}

// applyEndpointBleed paints a LUT-quantized alpha bleed at the given pixel.
// extension is the sub-pixel width (0..0.5) the original stroke extended past
// the snapped boundary. Empirically derived: any non-zero extension under
// 0.25 maps to LUT[1], 0.25..<0.5 maps to LUT[2]. This matches Poppler's
// gamma-corrected coverage at axis-aligned thin stroke endpoints.
func applyEndpointBleed(dst *image.RGBA, x, y int, col color.RGBA, extension float64) {
	if extension <= 0 {
		return
	}
	idx := int(math.Floor(extension*4)) + 1
	if idx > 16 {
		idx = 16
	}
	src := applyPremultipliedAlpha(col, splashAAGamma[idx])
	if src.A == 0 {
		return
	}
	dst.SetRGBA(x, y, splashCompositeOver(dst.RGBAAt(x, y), src))
}

func distancePointToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	den := dx*dx + dy*dy
	if den == 0 {
		return math.Hypot(px-x1, py-y1)
	}

	t := ((px-x1)*dx + (py-y1)*dy) / den
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx := x1 + t*dx
	cy := y1 + t*dy
	return math.Hypot(px-cx, py-cy)
}

func removeDuplicateStrokeClosure(points []strokePoint) []strokePoint {
	if len(points) < 2 {
		return points
	}
	first := points[0]
	last := points[len(points)-1]
	if math.Abs(first.x-last.x) > 1e-9 || math.Abs(first.y-last.y) > 1e-9 {
		return points
	}
	return points[:len(points)-1]
}

func strokeMiterPoint(prev, cur, next strokePoint, halfWidth, side, miterLimit float64) (strokePoint, bool) {
	prevDir, ok := normalizedStrokeVector(cur.x-prev.x, cur.y-prev.y)
	if !ok {
		return strokePoint{}, false
	}
	nextDir, ok := normalizedStrokeVector(next.x-cur.x, next.y-cur.y)
	if !ok {
		return strokePoint{}, false
	}

	prevNormal := strokePoint{x: -prevDir.y * side * halfWidth, y: prevDir.x * side * halfWidth}
	nextNormal := strokePoint{x: -nextDir.y * side * halfWidth, y: nextDir.x * side * halfWidth}
	prevOffset := strokePoint{x: cur.x + prevNormal.x, y: cur.y + prevNormal.y}
	nextOffset := strokePoint{x: cur.x + nextNormal.x, y: cur.y + nextNormal.y}

	intersection, ok := strokeLineIntersection(prevOffset, prevDir, nextOffset, nextDir)
	if !ok || math.Hypot(intersection.x-cur.x, intersection.y-cur.y) > miterLimit*halfWidth {
		return nextOffset, true
	}
	return intersection, true
}

func normalizedStrokeVector(dx, dy float64) (strokePoint, bool) {
	length := math.Hypot(dx, dy)
	if length <= 1e-9 {
		return strokePoint{}, false
	}
	return strokePoint{x: dx / length, y: dy / length}, true
}

func strokeLineIntersection(p1, d1, p2, d2 strokePoint) (strokePoint, bool) {
	denom := strokeCross(d1, d2)
	if math.Abs(denom) <= 1e-9 {
		return strokePoint{}, false
	}
	delta := strokePoint{x: p2.x - p1.x, y: p2.y - p1.y}
	t := strokeCross(delta, d2) / denom
	return strokePoint{x: p1.x + d1.x*t, y: p1.y + d1.y*t}, true
}

func strokeCross(left, right strokePoint) float64 {
	return left.x*right.y - left.y*right.x
}

func strokePolygonArea(points []strokePoint) float64 {
	if len(points) < 3 {
		return 0
	}
	area := 0.0
	for index, point := range points {
		next := points[(index+1)%len(points)]
		area += point.x*next.y - next.x*point.y
	}
	return area / 2
}

func reverseStrokePoints(points []strokePoint) []strokePoint {
	out := make([]strokePoint, len(points))
	for index := range points {
		out[index] = points[len(points)-1-index]
	}
	return out
}

func strokePointBounds(points []strokePoint) (minX, minY, maxX, maxY float64) {
	if len(points) == 0 {
		return 0, 0, 0, 0
	}
	minX, maxX = points[0].x, points[0].x
	minY, maxY = points[0].y, points[0].y
	for _, point := range points[1:] {
		if point.x < minX {
			minX = point.x
		}
		if point.x > maxX {
			maxX = point.x
		}
		if point.y < minY {
			minY = point.y
		}
		if point.y > maxY {
			maxY = point.y
		}
	}
	return minX, minY, maxX, maxY
}

func strokeAdjustAxisAlignedOutlines(outlines [][]strokePoint) [][]strokePoint {
	if !strokeOutlinesAreAxisAligned(outlines) {
		return outlines
	}

	applyTrailingInset := shouldUseStrokeAdjustTrailingInsetForDebug() && strokeAdjustTrailingInsetEligible(outlines)
	adjusted := make([][]strokePoint, len(outlines))
	for outlineIndex, outline := range outlines {
		adjusted[outlineIndex] = make([]strokePoint, len(outline))
		maxX := 0.0
		maxY := 0.0
		if applyTrailingInset {
			_, _, maxX, maxY = strokePointBounds(outline)
			maxX = math.RoundToEven(maxX)
			maxY = math.RoundToEven(maxY)
		}
		for pointIndex, point := range outline {
			// Use round-half-to-even (banker's rounding) to match Poppler's
			// strokeAdjust tie-breaking. This affects the rare case where a
			// stroke outline coordinate lands exactly on an x.5 boundary
			// (e.g. 010 forms Submit widget outer y_top = 372.500 in device
			// space) — Poppler snaps such ties to the even integer (372)
			// while math.Round (round-half-away-from-zero) would snap to 373,
			// producing a 1-row mismatch in the bar.
			adjustedPoint := strokePoint{
				x: math.RoundToEven(point.x),
				y: math.RoundToEven(point.y),
			}
			if applyTrailingInset {
				if adjustedPoint.x == maxX {
					adjustedPoint.x -= 0.01
				}
				if adjustedPoint.y == maxY {
					adjustedPoint.y -= 0.01
				}
			}
			adjusted[outlineIndex][pointIndex] = adjustedPoint
		}
	}
	return adjusted
}

func strokeAdjustTrailingInsetEligible(outlines [][]strokePoint) bool {
	if len(outlines) != 2 {
		return false
	}
	for _, outline := range outlines {
		if len(outline) < 4 {
			return false
		}
		if !strokeOutlineHasHalfPixelTie(outline) {
			return false
		}
	}
	return true
}

func strokeOutlineHasHalfPixelTie(outline []strokePoint) bool {
	const eps = 1e-9
	for _, point := range outline {
		if strokeCoordHasHalfPixelTie(point.x, eps) || strokeCoordHasHalfPixelTie(point.y, eps) {
			return true
		}
	}
	return false
}

func strokeCoordHasHalfPixelTie(value float64, eps float64) bool {
	frac := math.Abs(value - math.Trunc(value))
	return math.Abs(frac-0.5) <= eps
}

func strokePolylineIsAxisAligned(points []strokePoint) bool {
	const eps = 1e-9
	if len(points) < 2 {
		return false
	}
	for index, point := range points {
		next := points[(index+1)%len(points)]
		if math.Abs(point.x-next.x) > eps && math.Abs(point.y-next.y) > eps {
			return false
		}
	}
	return true
}

func strokeOutlinesAreAxisAligned(outlines [][]strokePoint) bool {
	const eps = 1e-9
	for _, outline := range outlines {
		if len(outline) < 2 {
			return false
		}
		for index, point := range outline {
			next := outline[(index+1)%len(outline)]
			if math.Abs(point.x-next.x) > eps && math.Abs(point.y-next.y) > eps {
				return false
			}
		}
	}
	return true
}

// rasterizeStrokeNarrowAxisAlignedQuads matches Poppler's strokeAdjust path
// for stroked axis-aligned rectangles. When the closed-stroke outline pair is
// an integer-aligned outer + inner rectangle (i.e. the strokeAdjust hint
// rounded all 8 corners to integer device coordinates), Poppler renders the
// resulting ring as four independent integer-aligned bars (top/right/bottom/
// left), each filled with full alpha. There is NO sub-pixel AA at any edge,
// so the corner cells avoid the faint AA tails the AA-supersampled scanner
// would otherwise produce when the inner top-edge horizontal segment lands
// on an integer supersample row and gets folded into a non-zero-winding span
// that briefly covers the entire interior.
//
// Returns (img, true) only when the outlines are exactly two integer-aligned
// axis-aligned rectangles with the inner strictly contained in the outer AND
// the pre-strokeAdjust outline rounding still yields distinct integer outer/
// inner edges (the snap doesn't degenerate the ring) — otherwise fall back
// to the AA scanner so we don't draw a misshapen 0/1-pixel ring.
func rasterizeStrokeNarrowAxisAlignedQuads(outlines, preAdjustOutlines [][]strokePoint, bounds image.Rectangle, stroke color.RGBA) (*image.RGBA, bool) {
	if bounds.Empty() {
		return nil, false
	}
	if len(outlines) != 2 {
		return nil, false
	}
	if len(preAdjustOutlines) != 2 {
		return nil, false
	}
	if !preStrokeAdjustOutlinesYieldDistinctIntegerEdges(preAdjustOutlines) {
		return nil, false
	}
	outerMinX, outerMinY, outerMaxX, outerMaxY, ok := axisAlignedIntegerRectFromOutline(outlines[0])
	if !ok {
		return nil, false
	}
	innerMinX, innerMinY, innerMaxX, innerMaxY, ok := axisAlignedIntegerRectFromOutline(outlines[1])
	if !ok {
		return nil, false
	}
	// Pick the larger as outer.
	if (outerMaxX-outerMinX)*(outerMaxY-outerMinY) < (innerMaxX-innerMinX)*(innerMaxY-innerMinY) {
		outerMinX, innerMinX = innerMinX, outerMinX
		outerMaxX, innerMaxX = innerMaxX, outerMaxX
		outerMinY, innerMinY = innerMinY, outerMinY
		outerMaxY, innerMaxY = innerMaxY, outerMaxY
	}
	// Inner must be strictly inside outer for the ring to make sense.
	if !(outerMinX < innerMinX && outerMinY < innerMinY && innerMaxX < outerMaxX && innerMaxY < outerMaxY) {
		return nil, false
	}

	tmpImg := image.NewRGBA(bounds)
	clip := func(minX, minY, maxX, maxY int) (int, int, int, int, bool) {
		if minX < bounds.Min.X {
			minX = bounds.Min.X
		}
		if minY < bounds.Min.Y {
			minY = bounds.Min.Y
		}
		if maxX > bounds.Max.X {
			maxX = bounds.Max.X
		}
		if maxY > bounds.Max.Y {
			maxY = bounds.Max.Y
		}
		return minX, minY, maxX, maxY, minX < maxX && minY < maxY
	}
	fillBar := func(minX, minY, maxX, maxY int) {
		minX, minY, maxX, maxY, valid := clip(minX, minY, maxX, maxY)
		if !valid {
			return
		}
		for y := minY; y < maxY; y++ {
			for x := minX; x < maxX; x++ {
				tmpImg.SetRGBA(x, y, stroke)
			}
		}
	}
	// Four bars covering the ring (corners are painted by both adjacent bars
	// — that's intentional and matches Poppler's overlapping per-segment
	// quads after strokeAdjust collapses each quad to an integer-aligned bar).
	fillBar(outerMinX, outerMinY, outerMaxX, innerMinY) // top
	fillBar(outerMinX, innerMaxY, outerMaxX, outerMaxY) // bottom
	fillBar(outerMinX, innerMinY, innerMinX, innerMaxY) // left (between top/bottom bars)
	fillBar(innerMaxX, innerMinY, outerMaxX, innerMaxY) // right
	return tmpImg, true
}

// preStrokeAdjustOutlinesYieldDistinctIntegerEdges reports whether every
// outline's coordinates round to a set of integers that keeps the outer and
// inner edges 1+ pixel apart in both axes. This mirrors Poppler's strokeAdjust
// behavior: when the snap rounds outer and inner to the same integer the
// adjuster forces them apart by 1, but otherwise the rounded ring is exactly
// what Poppler emits via the per-segment quad pipeline. Returns false when
// the rounded inner is not strictly inside the rounded outer (i.e. the snap
// would degenerate the ring), in which case we fall back to the AA scanner.
func preStrokeAdjustOutlinesYieldDistinctIntegerEdges(outlines [][]strokePoint) bool {
	if len(outlines) != 2 {
		return false
	}
	bbox := func(outline []strokePoint) (minX, minY, maxX, maxY int, ok bool) {
		if len(outline) < 4 {
			return 0, 0, 0, 0, false
		}
		minXf, minYf := outline[0].x, outline[0].y
		maxXf, maxYf := outline[0].x, outline[0].y
		for _, point := range outline[1:] {
			if point.x < minXf {
				minXf = point.x
			}
			if point.x > maxXf {
				maxXf = point.x
			}
			if point.y < minYf {
				minYf = point.y
			}
			if point.y > maxYf {
				maxYf = point.y
			}
		}
		return int(math.RoundToEven(minXf)), int(math.RoundToEven(minYf)), int(math.RoundToEven(maxXf)), int(math.RoundToEven(maxYf)), true
	}
	mnX0, mnY0, mxX0, mxY0, ok := bbox(outlines[0])
	if !ok {
		return false
	}
	mnX1, mnY1, mxX1, mxY1, ok := bbox(outlines[1])
	if !ok {
		return false
	}
	// Pick the larger as outer.
	if (mxX0-mnX0)*(mxY0-mnY0) < (mxX1-mnX1)*(mxY1-mnY1) {
		mnX0, mnX1 = mnX1, mnX0
		mxX0, mxX1 = mxX1, mxX0
		mnY0, mnY1 = mnY1, mnY0
		mxY0, mxY1 = mxY1, mxY0
	}
	return mnX0 < mnX1 && mnY0 < mnY1 && mxX1 < mxX0 && mxY1 < mxY0
}

// axisAlignedIntegerRectFromOutline returns the bounding rect of the outline
// when the outline is an axis-aligned closed rectangle whose every coordinate
// is integral (within a tight epsilon). The outline may have 4 or 5 points
// (5 = closed loop with duplicate first/last). Returns ok=false otherwise.
func axisAlignedIntegerRectFromOutline(outline []strokePoint) (minX, minY, maxX, maxY int, ok bool) {
	const eps = 1e-6
	if len(outline) < 4 {
		return 0, 0, 0, 0, false
	}
	// Drop trailing duplicate-of-first if present.
	pts := outline
	if len(pts) >= 2 {
		first := pts[0]
		last := pts[len(pts)-1]
		if math.Abs(first.x-last.x) <= eps && math.Abs(first.y-last.y) <= eps {
			pts = pts[:len(pts)-1]
		}
	}
	if len(pts) != 4 {
		return 0, 0, 0, 0, false
	}
	for _, point := range pts {
		if math.Abs(point.x-math.Round(point.x)) > eps || math.Abs(point.y-math.Round(point.y)) > eps {
			return 0, 0, 0, 0, false
		}
	}
	for index, point := range pts {
		next := pts[(index+1)%len(pts)]
		dx := math.Abs(point.x - next.x)
		dy := math.Abs(point.y - next.y)
		// Must be strictly axis-aligned (one of dx/dy zero, the other > 0).
		if !((dx <= eps && dy > eps) || (dy <= eps && dx > eps)) {
			return 0, 0, 0, 0, false
		}
	}
	xs := []float64{pts[0].x, pts[1].x, pts[2].x, pts[3].x}
	ys := []float64{pts[0].y, pts[1].y, pts[2].y, pts[3].y}
	mnX, mxX := xs[0], xs[0]
	mnY, mxY := ys[0], ys[0]
	for i := 1; i < 4; i++ {
		if xs[i] < mnX {
			mnX = xs[i]
		}
		if xs[i] > mxX {
			mxX = xs[i]
		}
		if ys[i] < mnY {
			mnY = ys[i]
		}
		if ys[i] > mxY {
			mxY = ys[i]
		}
	}
	if mxX-mnX < 1-eps || mxY-mnY < 1-eps {
		return 0, 0, 0, 0, false
	}
	// Each corner of the rect must appear exactly once among the 4 points.
	corners := [4][2]float64{{mnX, mnY}, {mxX, mnY}, {mxX, mxY}, {mnX, mxY}}
	for _, corner := range corners {
		found := false
		for _, point := range pts {
			if math.Abs(point.x-corner[0]) <= eps && math.Abs(point.y-corner[1]) <= eps {
				found = true
				break
			}
		}
		if !found {
			return 0, 0, 0, 0, false
		}
	}
	return int(math.Round(mnX)), int(math.Round(mnY)), int(math.Round(mxX)), int(math.Round(mxY)), true
}

func rasterizePopplerAAOutlines(outlines [][]strokePoint, bounds image.Rectangle, stroke color.RGBA, width, height int) (*image.RGBA, bool) {
	if bounds.Empty() || width <= 0 || height <= 0 {
		return nil, false
	}
	intersections, ok := popplerAAIntersections(outlines, height)
	if !ok || len(intersections) == 0 {
		return nil, false
	}

	tmpImg := image.NewRGBA(bounds)
	widthAA := width * 4
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		coverage := make([]uint8, bounds.Dx())
		for yy := 0; yy < 4; yy++ {
			line := intersections[y*4+yy]
			if len(line) == 0 {
				continue
			}
			for _, span := range popplerAASpans(line, widthAA) {
				start := max(span.x0, bounds.Min.X*4)
				end := min(span.x1, bounds.Max.X*4)
				for xx := start; xx < end; xx++ {
					index := xx/4 - bounds.Min.X
					if index >= 0 && index < len(coverage) {
						coverage[index]++
					}
				}
			}
		}
		for xOffset, count := range coverage {
			if count == 0 {
				continue
			}
			if count > 16 {
				count = 16
			}
			src := applyPremultipliedAlpha(stroke, splashAAGamma[count])
			if src.A != 0 {
				tmpImg.SetRGBA(bounds.Min.X+xOffset, y, src)
			}
		}
	}

	return tmpImg, true
}

func popplerAAIntersections(outlines [][]strokePoint, height int) (map[int][]popplerAAIntersection, bool) {
	intersections := make(map[int][]popplerAAIntersection)
	yMinClip := 0
	yMaxClip := height*4 - 1
	for _, outline := range outlines {
		if len(outline) < 2 {
			continue
		}
		for index, point := range outline {
			next := outline[(index+1)%len(outline)]
			if math.IsNaN(point.x) || math.IsNaN(point.y) || math.IsNaN(next.x) || math.IsNaN(next.y) {
				return nil, false
			}
			addPopplerAASegmentIntersection(intersections, point.x*4, point.y*4, next.x*4, next.y*4, yMinClip, yMaxClip)
		}
	}
	for y := range intersections {
		sort.Slice(intersections[y], func(i, j int) bool {
			return intersections[y][i].x0 < intersections[y][j].x0
		})
	}
	return intersections, true
}

func addPopplerAASegmentIntersection(
	intersections map[int][]popplerAAIntersection,
	x0, y0, x1, y1 float64,
	yMinClip, yMaxClip int,
) {
	const eps = 1e-12

	flip := y0 > y1
	segYMin := y0
	segYMax := y1
	if flip {
		segYMin, segYMax = y1, y0
	}
	count := -1
	if flip {
		count = 1
	}

	if math.Abs(y1-y0) <= eps {
		y := int(math.Floor(y0))
		if y >= yMinClip && y <= yMaxClip {
			appendPopplerAAIntersection(intersections, segYMin, segYMax, y, int(math.Floor(x0)), int(math.Floor(x1)), 0)
		}
		return
	}

	yStart := int(math.Floor(segYMin))
	if yStart < yMinClip {
		yStart = yMinClip
	}
	yEnd := int(math.Floor(segYMax))
	if yEnd > yMaxClip {
		yEnd = yMaxClip
	}
	if yStart > yEnd {
		return
	}

	if math.Abs(x1-x0) <= eps {
		x := int(math.Floor(x0))
		for y := yStart; y <= yEnd; y++ {
			appendPopplerAAIntersection(intersections, segYMin, segYMax, y, x, x, count)
		}
		return
	}

	segXMin, segXMax := x0, x1
	if segXMin > segXMax {
		segXMin, segXMax = segXMax, segXMin
	}
	dxdy := (x1 - x0) / (y1 - y0)
	xbase := x0 - y0*dxdy
	xx0 := xbase + float64(yStart)*dxdy
	if xx0 < segXMin {
		xx0 = segXMin
	} else if xx0 > segXMax {
		xx0 = segXMax
	}
	xStart := int(math.Floor(xx0))
	for y := yStart; y <= yEnd; y++ {
		xx1 := xbase + float64(y+1)*dxdy
		if xx1 < segXMin {
			xx1 = segXMin
		} else if xx1 > segXMax {
			xx1 = segXMax
		}
		xEnd := int(math.Floor(xx1))
		appendPopplerAAIntersection(intersections, segYMin, segYMax, y, xStart, xEnd, count)
		xx0 = xx1
		xStart = xEnd
	}
}

func appendPopplerAAIntersection(
	intersections map[int][]popplerAAIntersection,
	segYMin, segYMax float64,
	y, x0, x1, count int,
) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if !(segYMin <= float64(y) && float64(y) < segYMax) {
		count = 0
	}
	intersections[y] = append(intersections[y], popplerAAIntersection{x0: x0, x1: x1, count: count})
}

func popplerAASpans(line []popplerAAIntersection, widthAA int) []popplerAASpan {
	spans := make([]popplerAASpan, 0, len(line)/2)
	interIndex := 0
	interCount := 0
	for interIndex < len(line) {
		xx0 := line[interIndex].x0
		xx1 := line[interIndex].x1
		interCount += line[interIndex].count
		interIndex++
		for interIndex < len(line) && (line[interIndex].x0 <= xx1 || interCount != 0) {
			if line[interIndex].x1 > xx1 {
				xx1 = line[interIndex].x1
			}
			interCount += line[interIndex].count
			interIndex++
		}
		if xx0 < 0 {
			xx0 = 0
		}
		xx1++
		if xx1 > widthAA {
			xx1 = widthAA
		}
		if xx0 < xx1 {
			spans = append(spans, popplerAASpan{x0: xx0, x1: xx1})
		}
	}
	return spans
}

// drawLine draws a simple line (simplified implementation).
func (c *ImageCanvas) drawLine(x, y int, col color.Color) {
	bounds := c.img.Bounds()
	if x < 0 || x >= bounds.Dx() || y < 0 || y >= bounds.Dy() {
		return
	}
	c.img.Set(x, bounds.Dy()-1-y, col)
}

type lineSegment struct {
	x1 float64
	y1 float64
	x2 float64
	y2 float64
}

type strokePoint struct {
	x float64
	y float64
}

type closedStrokeSubpath struct {
	points []strokePoint
}

type strokeSegmentSubpath struct {
	segments []lineSegment
	closed   bool
}

type popplerAAIntersection struct {
	x0    int
	x1    int
	count int
}

type popplerAASpan struct {
	x0 int
	x1 int
}

func (s lineSegment) length() float64 {
	return math.Hypot(s.x2-s.x1, s.y2-s.y1)
}

func (s lineSegment) slice(startT, endT float64) lineSegment {
	return lineSegment{
		x1: s.x1 + (s.x2-s.x1)*startT,
		y1: s.y1 + (s.y2-s.y1)*startT,
		x2: s.x1 + (s.x2-s.x1)*endT,
		y2: s.y1 + (s.y2-s.y1)*endT,
	}
}

func splitStrokeSegmentSubpaths(segments []lineSegment) []strokeSegmentSubpath {
	if len(segments) == 0 {
		return nil
	}

	subpaths := make([]strokeSegmentSubpath, 0, 1)
	start := 0
	for index := 1; index < len(segments); index++ {
		prev := segments[index-1]
		cur := segments[index]
		if strokePointsNear(prev.x2, prev.y2, cur.x1, cur.y1) {
			continue
		}
		subpaths = append(subpaths, buildStrokeSegmentSubpath(segments[start:index]))
		start = index
	}
	subpaths = append(subpaths, buildStrokeSegmentSubpath(segments[start:]))
	return subpaths
}

func buildStrokeSegmentSubpath(segments []lineSegment) strokeSegmentSubpath {
	subpath := strokeSegmentSubpath{segments: segments}
	if len(segments) == 0 {
		return subpath
	}
	first := segments[0]
	last := segments[len(segments)-1]
	subpath.closed = len(segments) > 1 && strokePointsNear(first.x1, first.y1, last.x2, last.y2)
	return subpath
}

func buildRoundCapJoinStrokeOutlines(segments []lineSegment, radius float64) [][]strokePoint {
	if radius <= 0 || len(segments) == 0 {
		return nil
	}

	subpaths := splitStrokeSegmentSubpaths(segments)
	outlines := make([][]strokePoint, 0, len(segments)*2)
	for _, subpath := range subpaths {
		outlines = append(outlines, buildRoundCapJoinStrokeSubpathOutlines(subpath, radius)...)
	}
	return outlines
}

func buildRoundCapJoinStrokeSubpathOutlines(subpath strokeSegmentSubpath, radius float64) [][]strokePoint {
	if radius <= 0 || len(subpath.segments) == 0 {
		return nil
	}

	outlines := make([][]strokePoint, 0, len(subpath.segments)*2)
	dirs := make([]strokePoint, 0, len(subpath.segments))
	for _, segment := range subpath.segments {
		if outline, ok := strokeSegmentRectangleOutline(segment, radius); ok {
			outlines = append(outlines, outline)
		}
		dir, ok := normalizedStrokeVector(segment.x2-segment.x1, segment.y2-segment.y1)
		if !ok {
			dirs = append(dirs, strokePoint{})
			continue
		}
		dirs = append(dirs, dir)
	}

	points := strokeSegmentSubpathPoints(subpath.segments)
	if len(points) == 0 {
		return outlines
	}
	if subpath.closed {
		points = removeDuplicateStrokeClosure(points)
	}
	if len(points) == 1 {
		if outline := strokeCircleOutline(points[0], radius); len(outline) > 0 {
			outlines = append(outlines, outline)
		}
		return outlines
	}

	if !subpath.closed {
		if outline := strokeRoundStartCapOutline(points[0], dirs[0], radius); len(outline) > 0 {
			outlines = append(outlines, outline)
		}
		if outline := strokeRoundEndCapOutline(points[len(points)-1], dirs[len(dirs)-1], radius); len(outline) > 0 {
			outlines = append(outlines, outline)
		}
	}

	joinCount := len(points)
	if !subpath.closed {
		joinCount--
	}
	for index := 1; index < joinCount; index++ {
		prevDir := dirs[index-1]
		nextDir := dirs[index%len(dirs)]
		if outline := strokeRoundJoinOutline(points[index], prevDir, nextDir, radius); len(outline) > 0 {
			outlines = append(outlines, outline)
		}
	}

	return outlines
}

func buildOpenMiterJoinStrokeOutline(segments []lineSegment, radius, miterLimit float64) ([]strokePoint, bool) {
	if radius <= 0 || len(segments) < 2 {
		return nil, false
	}
	points := strokeSegmentSubpathPoints(segments)
	if len(points) < 3 {
		return nil, false
	}
	if miterLimit <= 0 {
		miterLimit = 10
	}

	left := make([]strokePoint, len(points))
	right := make([]strokePoint, len(points))

	startDir, ok := normalizedStrokeVector(points[1].x-points[0].x, points[1].y-points[0].y)
	if !ok {
		return nil, false
	}
	startNormal := strokePoint{x: -startDir.y * radius, y: startDir.x * radius}
	left[0] = strokePoint{x: points[0].x + startNormal.x, y: points[0].y + startNormal.y}
	right[0] = strokePoint{x: points[0].x - startNormal.x, y: points[0].y - startNormal.y}

	for index := 1; index < len(points)-1; index++ {
		leftPoint, ok := strokeMiterPoint(points[index-1], points[index], points[index+1], radius, 1, miterLimit)
		if !ok {
			return nil, false
		}
		rightPoint, ok := strokeMiterPoint(points[index-1], points[index], points[index+1], radius, -1, miterLimit)
		if !ok {
			return nil, false
		}
		left[index] = leftPoint
		right[index] = rightPoint
	}

	lastIndex := len(points) - 1
	endDir, ok := normalizedStrokeVector(
		points[lastIndex].x-points[lastIndex-1].x,
		points[lastIndex].y-points[lastIndex-1].y,
	)
	if !ok {
		return nil, false
	}
	endNormal := strokePoint{x: -endDir.y * radius, y: endDir.x * radius}
	left[lastIndex] = strokePoint{x: points[lastIndex].x + endNormal.x, y: points[lastIndex].y + endNormal.y}
	right[lastIndex] = strokePoint{x: points[lastIndex].x - endNormal.x, y: points[lastIndex].y - endNormal.y}

	outline := make([]strokePoint, 0, len(left)+len(right))
	outline = append(outline, left...)
	for index := len(right) - 1; index >= 0; index-- {
		outline = append(outline, right[index])
	}
	area := strokePolygonArea(outline)
	if math.Abs(area) <= 1e-9 {
		return nil, false
	}
	if area > 0 {
		outline = reverseStrokePoints(outline)
	}
	return outline, true
}

func strokeSegmentRectangleOutline(segment lineSegment, radius float64) ([]strokePoint, bool) {
	dir, ok := normalizedStrokeVector(segment.x2-segment.x1, segment.y2-segment.y1)
	if !ok {
		return nil, false
	}

	normal := strokePoint{x: -dir.y * radius, y: dir.x * radius}
	outline := []strokePoint{
		{x: segment.x1 + normal.x, y: segment.y1 + normal.y},
		{x: segment.x2 + normal.x, y: segment.y2 + normal.y},
		{x: segment.x2 - normal.x, y: segment.y2 - normal.y},
		{x: segment.x1 - normal.x, y: segment.y1 - normal.y},
	}
	if math.Abs(strokePolygonArea(outline)) <= 1e-9 {
		return nil, false
	}
	if strokePolygonArea(outline) > 0 {
		outline = reverseStrokePoints(outline)
	}
	return outline, true
}

func strokeCircleOutline(center strokePoint, radius float64) []strokePoint {
	if radius <= 0 {
		return nil
	}

	steps := 16
	switch {
	case radius >= 3:
		steps = 32
	case radius >= 1.5:
		steps = 24
	}

	outline := make([]strokePoint, 0, steps)
	for index := 0; index < steps; index++ {
		angle := 2 * math.Pi * (1 - float64(index)/float64(steps))
		outline = append(outline, strokePoint{
			x: center.x + math.Cos(angle)*radius,
			y: center.y + math.Sin(angle)*radius,
		})
	}
	if strokePolygonArea(outline) > 0 {
		outline = reverseStrokePoints(outline)
	}
	return outline
}

func strokeRoundStartCapOutline(center, dir strokePoint, radius float64) []strokePoint {
	left := strokePoint{x: center.x - dir.y*radius, y: center.y + dir.x*radius}
	right := strokePoint{x: center.x + dir.y*radius, y: center.y - dir.x*radius}
	through := strokePoint{x: center.x - dir.x*radius, y: center.y - dir.y*radius}
	return strokeSectorOutline(center, radius, left, right, through)
}

func strokeRoundEndCapOutline(center, dir strokePoint, radius float64) []strokePoint {
	left := strokePoint{x: center.x - dir.y*radius, y: center.y + dir.x*radius}
	right := strokePoint{x: center.x + dir.y*radius, y: center.y - dir.x*radius}
	through := strokePoint{x: center.x + dir.x*radius, y: center.y + dir.y*radius}
	return strokeSectorOutline(center, radius, left, right, through)
}

func strokeRoundJoinOutline(center, prevDir, nextDir strokePoint, radius float64) []strokePoint {
	const eps = 1e-9

	cross := strokeCross(prevDir, nextDir)
	hasAngle := math.Abs(cross) > eps || prevDir.x*nextDir.x < 0 || prevDir.y*nextDir.y < 0
	if !hasAngle {
		return nil
	}

	if cross < 0 {
		start := strokePoint{x: center.x - nextDir.y*radius, y: center.y + nextDir.x*radius}
		end := strokePoint{x: center.x - prevDir.y*radius, y: center.y + prevDir.x*radius}
		throughDir := strokePoint{x: -(prevDir.y + nextDir.y), y: prevDir.x + nextDir.x}
		throughDir, ok := normalizedStrokeVector(throughDir.x, throughDir.y)
		if !ok {
			throughDir = strokePoint{x: -prevDir.y, y: prevDir.x}
		}
		through := strokePoint{x: center.x + throughDir.x*radius, y: center.y + throughDir.y*radius}
		return strokeSectorOutline(center, radius, start, end, through)
	}

	start := strokePoint{x: center.x + prevDir.y*radius, y: center.y - prevDir.x*radius}
	end := strokePoint{x: center.x + nextDir.y*radius, y: center.y - nextDir.x*radius}
	throughDir := strokePoint{x: prevDir.y + nextDir.y, y: -(prevDir.x + nextDir.x)}
	throughDir, ok := normalizedStrokeVector(throughDir.x, throughDir.y)
	if !ok {
		throughDir = strokePoint{x: prevDir.y, y: -prevDir.x}
	}
	through := strokePoint{x: center.x + throughDir.x*radius, y: center.y + throughDir.y*radius}
	return strokeSectorOutline(center, radius, start, end, through)
}

func strokeSectorOutline(center strokePoint, radius float64, start, end, through strokePoint) []strokePoint {
	arc := strokeArcPointsThrough(center, radius, start, end, through)
	if len(arc) < 2 {
		return nil
	}
	outline := make([]strokePoint, 0, len(arc)+1)
	outline = append(outline, center)
	outline = append(outline, arc...)
	return strokeOutlineClockwise(outline)
}

func strokeArcPointsThrough(center strokePoint, radius float64, start, end, through strokePoint) []strokePoint {
	const maxArcStep = math.Pi / 12

	startAngle := math.Atan2(start.y-center.y, start.x-center.x)
	endAngle := math.Atan2(end.y-center.y, end.x-center.x)
	throughAngle := math.Atan2(through.y-center.y, through.x-center.x)

	ccwEnd := strokeNormalizeAngleDelta(endAngle - startAngle)
	if ccwEnd < 0 {
		ccwEnd += 2 * math.Pi
	}
	ccwThrough := strokeNormalizeAngleDelta(throughAngle - startAngle)
	if ccwThrough < 0 {
		ccwThrough += 2 * math.Pi
	}

	delta := ccwEnd
	if ccwThrough > ccwEnd+1e-9 {
		delta = ccwEnd - 2*math.Pi
	}
	if math.Abs(delta) <= 1e-9 {
		return []strokePoint{start, end}
	}

	steps := int(math.Ceil(math.Abs(delta) / maxArcStep))
	if steps < 1 {
		steps = 1
	}
	points := make([]strokePoint, 0, steps+1)
	for index := 0; index <= steps; index++ {
		angle := startAngle + delta*float64(index)/float64(steps)
		points = append(points, strokePoint{
			x: center.x + math.Cos(angle)*radius,
			y: center.y + math.Sin(angle)*radius,
		})
	}
	return points
}

func strokeNormalizeAngleDelta(delta float64) float64 {
	for delta <= -math.Pi {
		delta += 2 * math.Pi
	}
	for delta > math.Pi {
		delta -= 2 * math.Pi
	}
	return delta
}

func strokeOutlineClockwise(points []strokePoint) []strokePoint {
	if len(points) < 3 || math.Abs(strokePolygonArea(points)) <= 1e-9 {
		return nil
	}
	if strokePolygonArea(points) > 0 {
		return reverseStrokePoints(points)
	}
	return points
}

func strokeSegmentSubpathPoints(segments []lineSegment) []strokePoint {
	if len(segments) == 0 {
		return nil
	}

	points := make([]strokePoint, 0, len(segments)+1)
	points = append(points, strokePoint{x: segments[0].x1, y: segments[0].y1})
	points = append(points, strokePoint{x: segments[0].x2, y: segments[0].y2})
	for _, segment := range segments[1:] {
		if !strokePointsNear(points[len(points)-1].x, points[len(points)-1].y, segment.x1, segment.y1) {
			return nil
		}
		points = append(points, strokePoint{x: segment.x2, y: segment.y2})
	}
	return points
}

func strokePointsNear(x1, y1, x2, y2 float64) bool {
	const eps = 1e-6
	return math.Abs(x1-x2) <= eps && math.Abs(y1-y2) <= eps
}

func strokeSegmentsAreAxisAligned(segments []lineSegment) bool {
	const eps = 1e-6

	for _, segment := range segments {
		dx := math.Abs(segment.x2 - segment.x1)
		dy := math.Abs(segment.y2 - segment.y1)
		if dx > eps && dy > eps {
			return false
		}
	}
	return true
}

func (c *ImageCanvas) renderPathEvenOdd() {
	commands := c.currentPath.GetCommands()
	if len(commands) == 0 {
		return
	}

	if c.fillPattern != nil {
		if c.renderPathFillPattern(commands, true) {
			return
		}
	}

	segments := c.buildTransformedPathSegments(commands)
	if len(segments) == 0 {
		return
	}

	c.renderPathEvenOddFromSegments(segments, colorToRGBA(c.fillColor))
}

func (c *ImageCanvas) renderPathEvenOddFromSegments(segments []lineSegment, fillColor color.RGBA) {
	minX, minY, maxX, maxY := segmentBounds(segments)
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > c.width {
		maxX = c.width
	}
	if maxY > c.height {
		maxY = c.height
	}
	if minX >= maxX || minY >= maxY {
		return
	}

	tmpImg := image.NewRGBA(image.Rect(minX, minY, maxX, maxY))

	for y := minY; y < maxY; y++ {
		py := float64(y) + 0.5
		for x := minX; x < maxX; x++ {
			px := float64(x) + 0.5
			if pointInPathEvenOdd(px, py, segments) {
				tmpImg.SetRGBA(x, y, fillColor)
			}
		}
	}

	c.drawTempImageWithClip(tmpImg)
}

func (c *ImageCanvas) createPathMask(commands []graphics.PathCommand, bounds image.Rectangle, evenOdd bool) *image.Alpha {
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil
	}

	if evenOdd {
		segments := c.buildTransformedPathSegments(commands)
		if len(segments) == 0 {
			return nil
		}

		mask := image.NewAlpha(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			py := float64(y) + 0.5
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if pointInPathEvenOdd(float64(x)+0.5, py, segments) {
					mask.Set(x, y, color.Alpha{A: 255})
				}
			}
		}
		return mask
	}

	ras := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
	c.tracePathToRasterizer(ras, commands, bounds.Min.X, bounds.Min.Y, true, bounds)
	mask := image.NewAlpha(bounds)
	if !c.drawRasterPathSafe(ras, mask, bounds, image.Opaque, image.Point{}, "createPathMask") {
		return nil
	}
	return mask
}

func colorToHex(value color.Color) string {
	if value == nil {
		return ""
	}

	rgba := colorToRGBA(value)
	return fmt.Sprintf("%02X%02X%02X", rgba.R, rgba.G, rgba.B)
}

func (c *ImageCanvas) buildTransformedPathSegmentsWithTolerance(commands []graphics.PathCommand, tolerance float64) []lineSegment {
	segments := make([]lineSegment, 0, len(commands))
	yOrigin := c.canvasYOrigin()

	var curX, curY float64
	var startX, startY float64
	hasCurrent := false

	for _, command := range commands {
		switch cmd := command.(type) {
		case *graphics.MoveTo:
			tx, ty := c.transformPoint(cmd.X, cmd.Y)
			curX, curY = tx, yOrigin-ty
			startX, startY = curX, curY
			hasCurrent = true
		case *graphics.LineTo:
			if !hasCurrent {
				// Cairo behavior: lineto without current point acts as moveto
				tx, ty := c.transformPoint(cmd.X, cmd.Y)
				curX, curY = tx, yOrigin-ty
				startX, startY = curX, curY
				hasCurrent = true
				continue
			}
			tx, ty := c.transformPoint(cmd.X, cmd.Y)
			nextX, nextY := tx, yOrigin-ty
			segments = append(segments, lineSegment{x1: curX, y1: curY, x2: nextX, y2: nextY})
			curX, curY = nextX, nextY
		case *graphics.CurveTo:
			if !hasCurrent {
				// Cairo behavior: curveto without current point uses first control point as implicit moveto
				tx1, ty1 := c.transformPoint(cmd.X1, cmd.Y1)
				curX, curY = tx1, yOrigin-ty1
				startX, startY = curX, curY
				hasCurrent = true
			}
			tx1, ty1 := c.transformPoint(cmd.X1, cmd.Y1)
			tx2, ty2 := c.transformPoint(cmd.X2, cmd.Y2)
			tx3, ty3 := c.transformPoint(cmd.X3, cmd.Y3)
			c1x, c1y := tx1, yOrigin-ty1
			c2x, c2y := tx2, yOrigin-ty2
			endX, endY := tx3, yOrigin-ty3

			// Use adaptive flatness-based subdivision (à la Cairo) for accurate
			// curve positioning. Recursively subdivide until each sub-segment
			// deviates less than flatnessTolerance pixels from a straight line.
			segments = flattenCubicBezier(segments, curX, curY, c1x, c1y, c2x, c2y, endX, endY, tolerance, 0)
			curX, curY = endX, endY
		case *graphics.ClosePath:
			if !hasCurrent {
				continue
			}
			if curX != startX || curY != startY {
				segments = append(segments, lineSegment{x1: curX, y1: curY, x2: startX, y2: startY})
			}
			curX, curY = startX, startY
		}
	}

	return segments
}

// flatnessTolerance controls the maximum deviation (in pixels) allowed when
// approximating a cubic Bézier curve with line segments. Lower values produce
// more accurate curves at the cost of more segments. Cairo uses 0.1.
const flatnessTolerance = 0.1

// roundStrokeFlatnessTolerance tightens curve flattening for the round stroke
// outline path, which otherwise compounds approximation error at every join.
const roundStrokeFlatnessTolerance = 0.025

func (c *ImageCanvas) buildTransformedStrokeSegments(commands []graphics.PathCommand) []lineSegment {
	tolerance := flatnessTolerance
	if c.lineCap == 1 && c.lineJoin == 1 {
		tolerance = roundStrokeFlatnessTolerance
	}
	return c.buildTransformedPathSegmentsWithTolerance(commands, tolerance)
}

func (c *ImageCanvas) buildTransformedPathSegments(commands []graphics.PathCommand) []lineSegment {
	return c.buildTransformedPathSegmentsWithTolerance(commands, flatnessTolerance)
}

// flattenCubicBezier appends line segments approximating the cubic Bézier
// P0=(x0,y0), P1=(c1x,c1y), P2=(c2x,c2y), P3=(x3,y3) to segs, using
// recursive midpoint subdivision until the curve is flat enough.
// tolerance is the maximum allowed pixel deviation from a straight line.
// depth is used to cap recursion (max ~50 levels handles any practical curve).
func flattenCubicBezier(segs []lineSegment, x0, y0, c1x, c1y, c2x, c2y, x3, y3, tolerance float64, depth int) []lineSegment {
	if depth > 50 {
		segs = append(segs, lineSegment{x1: x0, y1: y0, x2: x3, y2: y3})
		return segs
	}

	// Cairo flatness test: measure deviation of control points from the chord.
	// ux, uy: deviation of P1 from the chord endpoint estimate
	// vx, vy: deviation of P2 from the chord endpoint estimate
	ux := 3*c1x - 2*x0 - x3
	uy := 3*c1y - 2*y0 - y3
	vx := 3*c2x - 2*x3 - x0
	vy := 3*c2y - 2*y3 - y0

	// Check flatness: if max(|u|², |v|²) ≤ 16 * tol², the curve is flat enough.
	uSq := ux*ux + uy*uy
	vSq := vx*vx + vy*vy
	maxSq := uSq
	if vSq > maxSq {
		maxSq = vSq
	}
	threshold := 16 * tolerance * tolerance
	if maxSq <= threshold {
		segs = append(segs, lineSegment{x1: x0, y1: y0, x2: x3, y2: y3})
		return segs
	}

	// Subdivide at t=0.5 using de Casteljau's algorithm.
	m01x := (x0 + c1x) * 0.5
	m01y := (y0 + c1y) * 0.5
	m12x := (c1x + c2x) * 0.5
	m12y := (c1y + c2y) * 0.5
	m23x := (c2x + x3) * 0.5
	m23y := (c2y + y3) * 0.5
	m012x := (m01x + m12x) * 0.5
	m012y := (m01y + m12y) * 0.5
	m123x := (m12x + m23x) * 0.5
	m123y := (m12y + m23y) * 0.5
	midX := (m012x + m123x) * 0.5
	midY := (m012y + m123y) * 0.5

	segs = flattenCubicBezier(segs, x0, y0, m01x, m01y, m012x, m012y, midX, midY, tolerance, depth+1)
	segs = flattenCubicBezier(segs, midX, midY, m123x, m123y, m23x, m23y, x3, y3, tolerance, depth+1)
	return segs
}

func pointInPathEvenOdd(px, py float64, segments []lineSegment) bool {
	inside := false

	for _, segment := range segments {
		y1 := segment.y1
		y2 := segment.y2
		if (y1 > py) == (y2 > py) {
			continue
		}
		xIntersect := segment.x1 + (py-y1)*(segment.x2-segment.x1)/(segment.y2-segment.y1)
		if px < xIntersect {
			inside = !inside
		}
	}

	return inside
}

func segmentBounds(segments []lineSegment) (minX, minY, maxX, maxY int) {
	if len(segments) == 0 {
		return 0, 0, 0, 0
	}

	minXF, minYF := segments[0].x1, segments[0].y1
	maxXF, maxYF := minXF, minYF

	for _, segment := range segments {
		for _, point := range [][2]float64{{segment.x1, segment.y1}, {segment.x2, segment.y2}} {
			if point[0] < minXF {
				minXF = point[0]
			}
			if point[0] > maxXF {
				maxXF = point[0]
			}
			if point[1] < minYF {
				minYF = point[1]
			}
			if point[1] > maxYF {
				maxYF = point[1]
			}
		}
	}

	return int(math.Floor(minXF)), int(math.Floor(minYF)), int(math.Ceil(maxXF)), int(math.Ceil(maxYF))
}

type strokeDebugStats struct {
	commands      int
	moves         int
	lines         int
	curves        int
	closes        int
	subpaths      int
	closedSubpath int
	openSubpath   int
}

func (c *ImageCanvas) traceStrokeForDebug(label string, commands []graphics.PathCommand, segments []lineSegment, lineWidth float64) {
	if !shouldTraceStrokeForDebug() {
		return
	}
	stats := collectStrokeDebugStats(commands)
	minX, minY, maxX, maxY := segmentBounds(segments)
	fmt.Fprintf(
		os.Stderr,
		"PDF_STROKE_TRACE op=%s cmds=%d moves=%d lines=%d curves=%d closes=%d subpaths=%d closed=%d open=%d segments=%d width=%.6f cap=%d join=%d miter=%.6f dash_count=%d dash_phase=%.6f bounds=%d,%d,%d,%d transform=%.6f,%.6f,%.6f,%.6f,%.6f,%.6f\n",
		label,
		stats.commands,
		stats.moves,
		stats.lines,
		stats.curves,
		stats.closes,
		stats.subpaths,
		stats.closedSubpath,
		stats.openSubpath,
		len(segments),
		lineWidth,
		c.lineCap,
		c.lineJoin,
		c.miterLimit,
		len(c.dashPattern),
		c.dashPhase,
		minX,
		minY,
		maxX,
		maxY,
		c.transform[0],
		c.transform[1],
		c.transform[2],
		c.transform[3],
		c.transform[4],
		c.transform[5],
	)
}

func collectStrokeDebugStats(commands []graphics.PathCommand) strokeDebugStats {
	stats := strokeDebugStats{commands: len(commands)}
	hasSubpath := false
	subpathClosed := false
	flush := func() {
		if !hasSubpath {
			return
		}
		stats.subpaths++
		if subpathClosed {
			stats.closedSubpath++
		} else {
			stats.openSubpath++
		}
		hasSubpath = false
		subpathClosed = false
	}
	for _, command := range commands {
		switch command.(type) {
		case *graphics.MoveTo:
			flush()
			stats.moves++
			hasSubpath = true
		case *graphics.LineTo:
			stats.lines++
			if !hasSubpath {
				hasSubpath = true
			}
		case *graphics.CurveTo:
			stats.curves++
			if !hasSubpath {
				hasSubpath = true
			}
		case *graphics.ClosePath:
			stats.closes++
			if hasSubpath {
				subpathClosed = true
			}
		}
	}
	flush()
	return stats
}

func colorToRGBA(value color.Color) color.RGBA {
	if rgba, ok := value.(color.RGBA); ok {
		return rgba
	}
	r, g, b, a := value.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func (c *ImageCanvas) applyColorTransfer(col color.RGBA) color.RGBA {
	if !c.transferActive {
		return col
	}
	return color.RGBA{
		R: c.transferRed[col.R],
		G: c.transferGreen[col.G],
		B: c.transferBlue[col.B],
		A: col.A,
	}
}

func premultiplyColor(src color.RGBA) color.RGBA {
	if src.A == 0 || src.A == 255 {
		return src
	}
	return color.RGBA{
		R: uint8(uint16(src.R) * uint16(src.A) / 255),
		G: uint8(uint16(src.G) * uint16(src.A) / 255),
		B: uint8(uint16(src.B) * uint16(src.A) / 255),
		A: src.A,
	}
}

// setPixelWithClip writes a PDF-space pixel (x,y; Y-up) honoring clip mask and alpha composition.
func (c *ImageCanvas) setPixelWithClip(x, y int, col color.RGBA) {
	dstX := x
	dstY := c.height - 1 - y
	if dstX < 0 || dstX >= c.width || dstY < 0 || dstY >= c.height {
		return
	}

	src := col
	if c.clipMask != nil {
		maskAlpha := c.clipMask.AlphaAt(dstX, dstY).A
		if maskAlpha == 0 {
			return
		}
		src = applyPremultipliedAlpha(src, maskAlpha)
	}
	if src.A == 0 {
		return
	}
	if src.A == 255 {
		c.img.SetRGBA(dstX, dstY, c.applyColorTransfer(src))
		return
	}
	dst := c.img.RGBAAt(dstX, dstY)
	c.img.SetRGBA(dstX, dstY, c.applyColorTransfer(compositeOver(dst, src)))
}

func (c *ImageCanvas) setCanvasPixelWithClip(x, y int, col color.RGBA) {
	if x < 0 || x >= c.width || y < 0 || y >= c.height {
		return
	}

	src := col
	if c.clipMask != nil {
		maskAlpha := c.clipMask.AlphaAt(x, y).A
		if maskAlpha == 0 {
			return
		}
		src = applyPremultipliedAlpha(src, maskAlpha)
	}
	if src.A == 0 {
		return
	}
	if src.A == 255 {
		c.img.SetRGBA(x, y, c.applyColorTransfer(src))
		return
	}
	dst := c.img.RGBAAt(x, y)
	c.img.SetRGBA(x, y, c.applyColorTransfer(compositeOver(dst, src)))
}

func (c *ImageCanvas) drawTempImageWithClip(tmpImg *image.RGBA) {
	tmpBounds := tmpImg.Bounds()
	targetBounds := tmpBounds.Intersect(c.img.Bounds())
	if targetBounds.Empty() {
		return
	}

	if c.clipMask == nil {
		draw.Draw(c.img, targetBounds, tmpImg, tmpBounds.Min, draw.Over)
		return
	}

	for y := targetBounds.Min.Y; y < targetBounds.Max.Y; y++ {
		for x := targetBounds.Min.X; x < targetBounds.Max.X; x++ {
			maskAlpha := c.clipMask.AlphaAt(x, y).A
			if maskAlpha == 0 {
				continue
			}

			srcColor := tmpImg.RGBAAt(x, y)
			if srcColor.A == 0 {
				continue
			}

			srcMasked := applyPremultipliedAlpha(srcColor, maskAlpha)
			if srcMasked.A == 0 {
				continue
			}
			dstColor := c.img.RGBAAt(x, y)
			c.img.SetRGBA(x, y, compositeOver(dstColor, srcMasked))
		}
	}
}

func applyPremultipliedAlpha(src color.RGBA, alpha uint8) color.RGBA {
	if alpha == 255 {
		return src
	}
	if alpha == 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: splashDiv255(uint32(src.R) * uint32(alpha)),
		G: splashDiv255(uint32(src.G) * uint32(alpha)),
		B: splashDiv255(uint32(src.B) * uint32(alpha)),
		A: splashDiv255(uint32(src.A) * uint32(alpha)),
	}
}

func splashDiv255(x uint32) uint8 {
	return uint8((x + (x >> 8) + 0x80) >> 8)
}

func compositeOver(dst, src color.RGBA) color.RGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	inv := uint16(255 - src.A)
	return color.RGBA{
		R: uint8(uint16(src.R) + uint16(dst.R)*inv/255),
		G: uint8(uint16(src.G) + uint16(dst.G)*inv/255),
		B: uint8(uint16(src.B) + uint16(dst.B)*inv/255),
		A: uint8(uint16(src.A) + uint16(dst.A)*inv/255),
	}
}

func splashCompositeOver(dst, src color.RGBA) color.RGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	inv := uint32(255 - src.A)
	return color.RGBA{
		R: uint8(uint32(src.R) + uint32(splashDiv255(uint32(dst.R)*inv))),
		G: uint8(uint32(src.G) + uint32(splashDiv255(uint32(dst.G)*inv))),
		B: uint8(uint32(src.B) + uint32(splashDiv255(uint32(dst.B)*inv))),
		A: uint8(uint32(src.A) + uint32(splashDiv255(uint32(dst.A)*inv))),
	}
}

// SetFillPattern sets the fill pattern.
func (c *ImageCanvas) SetFillPattern(pattern entity.Pattern) {
	c.fillPattern = pattern
}

// SetStrokePattern sets the stroke pattern.
func (c *ImageCanvas) SetStrokePattern(pattern entity.Pattern) {
	c.strokePattern = pattern
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func normalizePatternSpan(primary, fallback float64) (float64, error) {
	if isFinitePositive(primary) {
		return primary, nil
	}
	if isFinitePositive(fallback) {
		return fallback, nil
	}
	return 0, fmt.Errorf("invalid pattern span primary=%v fallback=%v", primary, fallback)
}

func patternCellBounds(bbox [4]float64, xStep, yStep float64) (image.Rectangle, error) {
	width, err := normalizePatternSpan(bbox[2]-bbox[0], xStep)
	if err != nil {
		return image.Rectangle{}, err
	}
	height, err := normalizePatternSpan(bbox[3]-bbox[1], yStep)
	if err != nil {
		return image.Rectangle{}, err
	}

	pixelWidth := int(math.Ceil(width))
	if pixelWidth < 1 {
		pixelWidth = 1
	}
	pixelHeight := int(math.Ceil(height))
	if pixelHeight < 1 {
		pixelHeight = 1
	}

	if pixelWidth > maxPatternCellSize || pixelHeight > maxPatternCellSize {
		return image.Rectangle{}, fmt.Errorf("pattern cell too large: %dx%d", pixelWidth, pixelHeight)
	}
	if pixelWidth > 0 && pixelHeight > maxPatternCellPixels/pixelWidth {
		return image.Rectangle{}, fmt.Errorf("pattern cell exceeds pixel budget: %dx%d", pixelWidth, pixelHeight)
	}

	return image.Rect(0, 0, pixelWidth, pixelHeight), nil
}

// DrawTilingPattern draws a tiling pattern to fill the specified bounding box.
func (c *ImageCanvas) DrawTilingPattern(pattern *entity.TilingPattern, bbox [4]float64) error {
	patternBBox := pattern.GetBBox()

	// Extract scale from the effective pattern matrix (already CTM × patternMatrix).
	// This converts pattern-space units to device-space (pixel) units.
	matrix := pattern.Matrix()
	scaleX := math.Hypot(matrix[0], matrix[1])
	scaleY := math.Hypot(matrix[2], matrix[3])
	if scaleX <= 0 {
		scaleX = 1
	}
	if scaleY <= 0 {
		scaleY = 1
	}

	rawXStep := math.Abs(pattern.GetXStep())
	rawYStep := math.Abs(pattern.GetYStep())
	rawCellW := math.Abs(patternBBox[2] - patternBBox[0])
	rawCellH := math.Abs(patternBBox[3] - patternBBox[1])

	var err error
	rawXStep, err = normalizePatternSpan(rawXStep, rawCellW)
	if err != nil {
		return fmt.Errorf("invalid tiling pattern x step: %w", err)
	}
	rawYStep, err = normalizePatternSpan(rawYStep, rawCellH)
	if err != nil {
		return fmt.Errorf("invalid tiling pattern y step: %w", err)
	}
	rawCellW, err = normalizePatternSpan(rawCellW, rawXStep)
	if err != nil {
		return fmt.Errorf("invalid tiling pattern width: %w", err)
	}
	rawCellH, err = normalizePatternSpan(rawCellH, rawYStep)
	if err != nil {
		return fmt.Errorf("invalid tiling pattern height: %w", err)
	}

	// Convert pattern-space step dimensions to device-space (pixel) dimensions.
	xStepPx := rawXStep * scaleX
	yStepPx := rawYStep * scaleY

	cellScaleX := scaleX
	cellScaleY := scaleY

	// Fill area in canvas Y-down space.
	fillXMin := bbox[0]
	fillXMax := bbox[2]
	fillYMin := bbox[1] // canvas Y top (smallest row index)
	fillYMax := bbox[3] // canvas Y bottom (largest row index)
	fillWidth := fillXMax - fillXMin
	fillHeight := fillYMax - fillYMin
	if math.IsNaN(fillWidth) || math.IsNaN(fillHeight) || math.IsInf(fillWidth, 0) || math.IsInf(fillHeight, 0) {
		return fmt.Errorf("invalid tiling fill bbox: %v", bbox)
	}
	if fillWidth <= 0 || fillHeight <= 0 {
		return nil
	}

	content := pattern.GetContent()
	resources := pattern.GetResources()

	// The pattern tiles from its origin (matrix translation in device Y-up space).
	originX := matrix[4]
	originYDev := matrix[5] // device Y-up

	// Convert fill bbox from canvas Y-down to device Y-up ranges.
	fillYDevMin := float64(c.height) - fillYMax // device Y of fill bottom edge
	fillYDevMax := float64(c.height) - fillYMin // device Y of fill top edge

	// Compute cell size in pixels (based on BBox dimensions).
	// Check this before tile range to catch oversized cells early.
	cellWPx := rawCellW * cellScaleX
	cellHPx := rawCellH * cellScaleY
	cellW_int := int(math.Ceil(cellWPx))
	cellH_int := int(math.Ceil(cellHPx))
	if cellW_int <= 0 {
		cellW_int = 1
	}
	if cellH_int <= 0 {
		cellH_int = 1
	}
	if cellW_int > maxPatternCellSize || cellH_int > maxPatternCellSize ||
		cellW_int*cellH_int > maxPatternCellPixels {
		return fmt.Errorf("tiling pattern cell too large: %dx%d", cellW_int, cellH_int)
	}

	// Tile indices that cover the fill area.
	// A tile at index i has content in device X range [tileX + bbox[0]*scaleX, tileX + bbox[2]*scaleX].
	// For content to intersect [fillXMin, fillXMax]:
	//   tileX + bbox[2]*scaleX > fillXMin  →  i > (fillXMin - bbox[2]*scaleX - originX) / xStepPx
	//   tileX + bbox[0]*scaleX < fillXMax  →  i < (fillXMax - bbox[0]*scaleX - originX) / xStepPx
	bboxX0Px := patternBBox[0] * scaleX
	bboxX2Px := patternBBox[2] * scaleX
	bboxY1Px := patternBBox[1] * scaleY // PDF Y-up: bottom of bbox
	bboxY3Px := patternBBox[3] * scaleY // PDF Y-up: top of bbox
	startI := int(math.Floor((fillXMin-bboxX2Px-originX)/xStepPx)) - 1
	endI := int(math.Ceil((fillXMax-bboxX0Px-originX)/xStepPx)) + 1
	startJ := int(math.Floor((fillYDevMin-bboxY3Px-originYDev)/yStepPx)) - 1
	endJ := int(math.Ceil((fillYDevMax-bboxY1Px-originYDev)/yStepPx)) + 1

	numX := endI - startI + 1
	numY := endJ - startJ + 1
	if numX <= 0 || numY <= 0 {
		return nil
	}
	const maxTiles = 16384
	if numX > maxTiles || numY > maxTiles || numX*numY > maxTiles*4 {
		return fmt.Errorf("too many pattern tiles: %dx%d", numX, numY)
	}

	if len(content) > 0 {
		parserEval := renderer.NewEvaluator(&patternXRef{})
		ops, err := parserEval.ParseContentOperators(content)
		if err != nil {
			return fmt.Errorf("parse tiling pattern tile content: %w", err)
		}
		if len(ops) == 0 {
			return nil
		}
		stepMismatch := tilingPatternStepMismatch(rawCellW, rawCellH, rawXStep, rawYStep)
		if os.Getenv("PDF_DEBUG_PATTERN") == "1" {
			r, g, b, a := c.fillColor.RGBA()
			clen := 50
			if clen > len(content) {
				clen = len(content)
			}
			fmt.Fprintf(os.Stderr, "DEBUG DrawTilingPattern: pattern %q paintType=%d, fillColor=RGBA(%d,%d,%d,%d), content=%q\n",
				pattern.Name(), pattern.GetPaintType(), r>>8, g>>8, b>>8, a>>8,
				string(content[:clen]))
			fmt.Fprintf(os.Stderr, "DEBUG DrawTilingPattern: matrix=%v scaleX=%.4f scaleY=%.4f originX=%.4f originYDev=%.4f xStepPx=%.4f yStepPx=%.4f cellW=%.4f cellH=%.4f\n",
				matrix, scaleX, scaleY, originX, originYDev, xStepPx, yStepPx, cellWPx, cellHPx)
			fmt.Fprintf(os.Stderr, "DEBUG DrawTilingPattern: patternBBox=%v rawXStep=%.4f rawYStep=%.4f rawCellW=%.4f rawCellH=%.4f cellW_int=%d cellH_int=%d\n",
				patternBBox, rawXStep, rawYStep, rawCellW, rawCellH, cellW_int, cellH_int)
			fmt.Fprintf(os.Stderr, "DEBUG DrawTilingPattern: stepMismatch=%t hasFillOps=%t opCount=%d\n",
				stepMismatch, tilingPatternOperatorsContainFill(ops), len(ops))
		}

		// Poppler's fallback path replays each tile as vector content. For very small
		// cells, pre-rendering one prototype and stamping it loses device-phase
		// information and measurably hurts parity on PGF dot patterns.
		if shouldReplayTilingPatternPerTile(ops, cellW_int, cellH_int, stepMismatch) {
			return c.drawTilingPatternByTileReplay(
				pattern,
				ops,
				resources,
				scaleX,
				scaleY,
				patternBBox,
				startI,
				endI,
				startJ,
				endJ,
				originX,
				originYDev,
				xStepPx,
				yStepPx,
			)
		}

		// Pre-render one cell canvas and stamp it at each tile position.
		// The cell canvas is sized to the pattern BBox in pixels (ceiling).
		// Pattern space → cell canvas via: [scaleX, 0, 0, scaleY, cellTX, cellTY].
		// Convention: bbox bottom (y=bbox[1]) aligns with cell bottom (row = cellH_int when using
		// float64(c.height) for Y-flip). Pattern (0,0) maps to cell row (cellH_int + bbox[1]*scaleY).
		// Stamp position accounts for bbox[3]*scaleY offset.
		cellTX := -patternBBox[0] * cellScaleX
		cellTY := -patternBBox[1] * cellScaleY

		cellImg := image.NewRGBA(image.Rect(0, 0, cellW_int, cellH_int))
		cellCanvas := NewImageCanvas(image.Rect(0, 0, cellW_int, cellH_int)).(*ImageCanvas)
		cellCanvas.img = cellImg
		cellCanvas.height = cellH_int
		cellCanvas.width = cellW_int
		cellCanvas.SetPageYOriginPx(cellHPx) // Use float cell height for precise glyph rendering

		cellEval := renderer.NewEvaluator(&patternXRef{})
		cellEval.SetCanvas(cellCanvas)
		if resources != nil {
			cellEval.SetResources(resources)
		}
		if pattern.IsUncolored() {
			cellEval.SetFillColor(c.fillColor)
			cellEval.SetStrokeColor(c.fillColor)
		}
		cellEval.SetFillPattern(nil)
		cellEval.SetStrokePattern(nil)
		cellEval.SetInitialTransform([6]float64{cellScaleX, 0, 0, cellScaleY, cellTX, cellTY})
		cellEval.ExecuteOperators(ops)

		// Bbox corners in device pixels relative to tile origin.
		bboxLeftX := patternBBox[0] * cellScaleX
		bboxTopY := patternBBox[3] * cellScaleY

		cellSrcBounds := cellImg.Bounds()
		mainBounds := c.img.Bounds()

		for i := startI; i <= endI; i++ {
			tileX := originX + float64(i)*xStepPx
			cellDestX := int(math.Round(tileX + bboxLeftX))
			// X step clip: tile owns cols [ceil(tileX), ceil(tileX+xStepPx)).
			stepClipXMin := int(math.Ceil(tileX))
			stepClipXMax := int(math.Ceil(tileX + xStepPx))

			for j := startJ; j <= endJ; j++ {
				tileY := originYDev + float64(j)*yStepPx
				cellDestY := int(math.Ceil(float64(c.height) - tileY - bboxTopY))
				// Y step clip: tile owns rows [ceil(c.height-tileY-yStepPx), ceil(c.height-tileY)).
				stepClipYMin := int(math.Ceil(float64(c.height) - tileY - yStepPx))
				stepClipYMax := int(math.Ceil(float64(c.height) - tileY))

				dstRect := image.Rect(cellDestX, cellDestY, cellDestX+cellW_int, cellDestY+cellH_int)
				stepClip := image.Rect(stepClipXMin, stepClipYMin, stepClipXMax, stepClipYMax)
				clipped := dstRect.Intersect(mainBounds).Intersect(stepClip)
				if clipped.Empty() {
					// When step clip is entirely off-canvas (large bbox offset relative to step),
					// fall back to cell bounds clipping to paint the visible content.
					clipped = dstRect.Intersect(mainBounds)
					if clipped.Empty() {
						continue
					}
				}
				srcPt := cellSrcBounds.Min.Add(clipped.Min.Sub(dstRect.Min))
				draw.Draw(c.img, clipped, cellImg, srcPt, draw.Over)
			}
		}
	}

	return nil
}

func (c *ImageCanvas) drawTilingPatternByTileReplay(
	pattern *entity.TilingPattern,
	ops []renderer.Operator,
	resources *entity.Dict,
	scaleX float64,
	scaleY float64,
	patternBBox [4]float64,
	startI int,
	endI int,
	startJ int,
	endJ int,
	originX float64,
	originYDev float64,
	xStepPx float64,
	yStepPx float64,
) error {
	if len(ops) == 0 {
		return nil
	}

	mainBounds := c.img.Bounds()
	yOrigin := c.canvasYOrigin()
	bboxMinX := math.Min(patternBBox[0], patternBBox[2]) * scaleX
	bboxMaxX := math.Max(patternBBox[0], patternBBox[2]) * scaleX
	bboxMinYDev := math.Min(patternBBox[1], patternBBox[3]) * scaleY
	bboxMaxYDev := math.Max(patternBBox[1], patternBBox[3]) * scaleY

	for i := startI; i <= endI; i++ {
		tileX := originX + float64(i)*xStepPx
		for j := startJ; j <= endJ; j++ {
			tileYDev := originYDev + float64(j)*yStepPx

			tileMinX := int(math.Floor(tileX+bboxMinX)) - 2
			tileMaxX := int(math.Ceil(tileX+bboxMaxX)) + 2
			tileMinY := int(math.Floor(yOrigin-(tileYDev+bboxMaxYDev))) - 2
			tileMaxY := int(math.Ceil(yOrigin-(tileYDev+bboxMinYDev))) + 2

			tileBounds := image.Rect(tileMinX, tileMinY, tileMaxX, tileMaxY).Intersect(mainBounds)
			if tileBounds.Empty() {
				continue
			}

			tileCanvas := NewImageCanvas(tileBounds).(*ImageCanvas)
			tileCanvas.width = c.width
			tileCanvas.height = c.height
			tileCanvas.pageYOriginPx = yOrigin

			tileEval := renderer.NewEvaluator(&patternXRef{})
			tileEval.SetCanvas(tileCanvas)
			if resources != nil {
				tileEval.SetResources(resources)
			}
			if pattern.IsUncolored() {
				tileEval.SetFillColor(c.fillColor)
				tileEval.SetStrokeColor(c.fillColor)
			}
			tileEval.SetFillPattern(nil)
			tileEval.SetStrokePattern(nil)
			tileEval.SetInitialTransform([6]float64{scaleX, 0, 0, scaleY, tileX, tileYDev})
			tileEval.ExecuteOperators(ops)

			draw.Draw(c.img, tileBounds, tileCanvas.img, tileBounds.Min, draw.Over)
		}
	}

	return nil
}

// DrawShadingPattern draws a shading pattern (gradient) to fill the specified bounding box.
func (c *ImageCanvas) DrawShadingPattern(pattern *entity.ShadingPattern, bbox [4]float64) error {
	shading := pattern.GetShading()
	if shading == nil {
		return fmt.Errorf("shading pattern has no shading object")
	}

	// Get the pattern transformation matrix
	matrix := pattern.Matrix()

	// Save current state
	c.Save()

	// Apply pattern transformation
	c.Transform(matrix)

	// Render based on shading type
	switch shading.GetShadingType() {
	case entity.ShadingAxial:
		err := c.drawAxialShading(shading, bbox)
		if err != nil {
			c.Restore()
			return err
		}
	case entity.ShadingRadial:
		err := c.drawRadialShading(shading, bbox)
		if err != nil {
			c.Restore()
			return err
		}
	case entity.ShadingFunctionBased:
		err := c.drawFunctionBasedShading(shading, bbox)
		if err != nil {
			c.Restore()
			return err
		}
	case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud:
		err := c.drawGouraudShading(shading, bbox)
		if err != nil {
			c.Restore()
			return err
		}
	case entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
		err := c.drawPatchMeshShading(shading, bbox)
		if err != nil {
			c.Restore()
			return err
		}
	default:
		c.Restore()
		return fmt.Errorf("unsupported shading type: %d", shading.GetShadingType())
	}

	// Restore state
	c.Restore()

	return nil
}

func normalizePDFBBox(bbox [4]float64) ([4]float64, bool) {
	minX := math.Min(bbox[0], bbox[2])
	maxX := math.Max(bbox[0], bbox[2])
	minY := math.Min(bbox[1], bbox[3])
	maxY := math.Max(bbox[1], bbox[3])
	if maxX <= minX || maxY <= minY {
		return [4]float64{}, false
	}
	return [4]float64{minX, minY, maxX, maxY}, true
}

func pdfBBoxToPixelBounds(bbox [4]float64, width, height int) (image.Rectangle, bool) {
	normalizedBBox, ok := normalizePDFBBox(bbox)
	if !ok {
		return image.Rectangle{}, false
	}

	minX := int(math.Max(0, math.Floor(normalizedBBox[0])))
	minY := int(math.Max(0, math.Floor(normalizedBBox[1])))
	maxX := int(math.Min(float64(width), math.Ceil(normalizedBBox[2])))
	maxY := int(math.Min(float64(height), math.Ceil(normalizedBBox[3])))
	if maxX <= minX || maxY <= minY {
		return image.Rectangle{}, false
	}

	return image.Rect(minX, minY, maxX, maxY), true
}

func (c *ImageCanvas) transformedShadingPixelBounds(shading *entity.Shading) (image.Rectangle, bool) {
	shadingBBox, ok := normalizePDFBBox(shading.GetBBox())
	if !ok {
		return image.Rect(0, 0, c.width, c.height), true
	}

	corners := [4][2]float64{
		{shadingBBox[0], shadingBBox[1]},
		{shadingBBox[0], shadingBBox[3]},
		{shadingBBox[2], shadingBBox[1]},
		{shadingBBox[2], shadingBBox[3]},
	}

	minX := math.MaxFloat64
	minY := math.MaxFloat64
	maxX := -math.MaxFloat64
	maxY := -math.MaxFloat64
	yOrigin := c.glyphYOrigin()
	for _, corner := range corners {
		tx, ty := c.transformPoint(corner[0], corner[1])
		canvasY := yOrigin - ty
		if tx < minX {
			minX = tx
		}
		if tx > maxX {
			maxX = tx
		}
		if canvasY < minY {
			minY = canvasY
		}
		if canvasY > maxY {
			maxY = canvasY
		}
	}

	return pdfBBoxToPixelBounds([4]float64{minX, minY, maxX, maxY}, c.width, c.height)
}

func (c *ImageCanvas) shadingPatternPoint(x, y float64, inverse [6]float64) (float64, float64) {
	return graphics.TransformPoint(inverse, x, y)
}

func (c *ImageCanvas) shadingPatternInverseMatrix() ([6]float64, error) {
	return graphics.InverseMatrix(c.transform)
}

func (c *ImageCanvas) transformShadingVertex(v entity.Vertex) entity.Vertex {
	tx, ty := c.transformPoint(v.X, v.Y)
	// Splash rounds Gouraud vertices after converting them to Y-down canvas space.
	ty = c.glyphYOrigin() - ty
	colors := append([]float64(nil), v.Colors...)
	return entity.NewVertex(tx, ty, colors)
}

// drawAxialShading draws an axial (linear) shading.
func (c *ImageCanvas) drawAxialShading(shading *entity.Shading, bbox [4]float64) error {
	coords := shading.GetCoords()
	if len(coords) < 4 {
		return fmt.Errorf("invalid axial shading coordinates")
	}

	x0, y0 := coords[0], coords[1]
	x1, y1 := coords[2], coords[3]

	// Calculate gradient direction
	dx := x1 - x0
	dy := y1 - y0
	length := math.Sqrt(dx*dx + dy*dy)

	if length == 0 {
		return fmt.Errorf("zero-length axial shading")
	}

	// Normalize direction
	dirX := dx / length
	dirY := dy / length

	// Get the shading function
	functions := shading.GetFunctions()
	if len(functions) == 0 {
		return fmt.Errorf("axial shading has no function")
	}

	// Get extend flags
	extend := shading.GetExtend()

	inverse, err := c.shadingPatternInverseMatrix()
	if err != nil {
		return err
	}

	domainMin, domainMax := shadingUnivariateDomain(shading)
	colorCache := c.newUnivariateShadingColorCache(shading, math.Hypot(dx, dy), bbox)

	// Rasterize the bounding box
	xMin := int(math.Floor(bbox[0]))
	yMin := int(math.Floor(bbox[1]))
	xMax := int(math.Ceil(bbox[2]))
	yMax := int(math.Ceil(bbox[3]))

	// Sample at y-down top-left corner (Poppler convention). See drawRadialShading
	// for the y-up/y-down derivation: integer device-pixel index in y-down maps
	// to (x, y+1) in our y-up iteration.
	for y := yMin; y < yMax; y++ {
		for x := xMin; x < xMax; x++ {
			px, py := c.shadingPatternPoint(float64(x), float64(y)+1, inverse)

			// Calculate projection onto gradient line
			t := ((px-x0)*dirX + (py-y0)*dirY) / length

			// Apply extend flags
			if t < 0 {
				if !extend[0] {
					continue
				}
				t = 0
			} else if t > 1 {
				if !extend[1] {
					continue
				}
				t = 1
			}

			fnInput := domainMin + t*(domainMax-domainMin)

			colors, ok := colorCache.Evaluate(fnInput)
			if !ok {
				var err error
				colors, err = evaluateShadingColorFunctions(functions, []float64{fnInput})
				if err != nil {
					continue
				}
			}
			if len(colors) == 0 {
				continue
			}

			// Convert to RGBA color
			col := c.colorArrayToRGBA(colors)

			c.setPixelWithClip(x, y, col)
		}
	}

	return nil
}

// drawRadialShading draws a radial shading.
func (c *ImageCanvas) drawRadialShading(shading *entity.Shading, bbox [4]float64) error {
	coords := shading.GetCoords()
	if len(coords) < 6 {
		return fmt.Errorf("invalid radial shading coordinates")
	}

	x0, y0, r0 := coords[0], coords[1], coords[2]
	x1, y1, r1 := coords[3], coords[4], coords[5]

	// Get the shading function
	functions := shading.GetFunctions()
	if len(functions) == 0 {
		return fmt.Errorf("radial shading has no function")
	}

	// Get extend flags
	extend := shading.GetExtend()

	inverse, err := c.shadingPatternInverseMatrix()
	if err != nil {
		return err
	}

	// Calculate distance between centers
	dx := x1 - x0
	dy := y1 - y0
	d := math.Sqrt(dx*dx + dy*dy)
	dr := r1 - r0

	// Rasterize the bounding box
	xMin := int(math.Floor(bbox[0]))
	yMin := int(math.Floor(bbox[1]))
	xMax := int(math.Ceil(bbox[2]))
	yMax := int(math.Ceil(bbox[3]))

	domainMin, domainMax := shadingUnivariateDomain(shading)
	colorCache := c.newUnivariateShadingColorCache(shading, d+math.Abs(dr), bbox)

	// Precompute quadratic coefficients for PDF radial shading t-parameter.
	// For point P=(px,py) find the largest t in [0,1] such that:
	//   |P - C(t)|^2 = R(t)^2  where C(t)=C0+t*(C1-C0), R(t)=r0+t*(r1-r0)
	// Expanding: A*t^2 + B*t + C = 0 where:
	//   A = (dx^2+dy^2) - dr^2
	//   B(px,py) = -2*( (px-x0)*dx + (py-y0)*dy + r0*dr )
	//   C(px,py) = (px-x0)^2 + (py-y0)^2 - r0^2
	A := d*d - dr*dr

	// Sample at the pixel's y-down top-left corner. Poppler's
	// SplashUnivariatePattern::getColor uses integer device-pixel indices, which
	// in our y-up iteration (where setPixelWithClip flips dstY = c.height-1-y)
	// becomes (x, y+1). Matters for radial shadings where iso-t curves bend
	// around the center.
	for y := yMin; y < yMax; y++ {
		for x := xMin; x < xMax; x++ {
			px, py := c.shadingPatternPoint(float64(x), float64(y)+1, inverse)

			fnInput, valid := popplerRadialParameter(
				px,
				py,
				x0,
				y0,
				r0,
				dx,
				dy,
				dr,
				A,
				extend,
				domainMin,
				domainMax,
			)
			if !valid {
				continue
			}

			colors, ok := colorCache.Evaluate(fnInput)
			if !ok {
				var err error
				colors, err = evaluateShadingColorFunctions(functions, []float64{fnInput})
				if err != nil {
					continue
				}
			}
			if len(colors) == 0 {
				continue
			}

			// Convert to RGBA color
			col := c.colorArrayToRGBA(colors)

			c.setPixelWithClip(x, y, col)
		}
	}

	return nil
}

func shadingUnivariateDomain(shading *entity.Shading) (float64, float64) {
	domain := shading.GetDomain()
	if domain[0] != 0 || domain[1] != 0 {
		return domain[0], domain[1]
	}
	return 0, 1
}

type univariateShadingColorCache struct {
	bounds []float64
	coeff  float64
	values [][]float64
	last   int
}

func (c *ImageCanvas) newUnivariateShadingColorCache(shading *entity.Shading, distance float64, bbox [4]float64) *univariateShadingColorCache {
	if shading == nil || distance <= 0 {
		return nil
	}
	functions := shading.GetFunctions()
	if len(functions) == 0 {
		return nil
	}
	domainMin, domainMax := shadingUnivariateDomain(shading)
	if domainMin == domainMax {
		return nil
	}

	area := math.Abs((bbox[2] - bbox[0]) * (bbox[3] - bbox[1]))
	if area <= 0 {
		return nil
	}

	maxSize := int(math.Ceil(matrixNorm(c.transform) * distance))
	if maxSize < 2 {
		maxSize = 2
	}
	if float64(maxSize) > area {
		return nil
	}

	cache := &univariateShadingColorCache{
		bounds: make([]float64, maxSize),
		coeff:  float64(maxSize-1) / (domainMax - domainMin),
		values: make([][]float64, maxSize),
		last:   1,
	}
	step := (domainMax - domainMin) / float64(maxSize-1)
	for i := 0; i < maxSize; i++ {
		t := domainMin + float64(i)*step
		colors, err := evaluateShadingColorFunctions(functions, []float64{t})
		if err != nil || len(colors) == 0 {
			return nil
		}
		cache.bounds[i] = t
		cache.values[i] = colors
	}
	return cache
}

func (c *univariateShadingColorCache) Evaluate(t float64) ([]float64, bool) {
	if c == nil || len(c.bounds) < 2 || len(c.values) != len(c.bounds) {
		return nil, false
	}
	last := c.last
	if last <= 0 || last >= len(c.bounds) {
		last = 1
	}
	if c.bounds[last-1] >= t {
		last = lowerBoundFloat64(c.bounds[:last], t)
	} else if c.bounds[last] < t {
		last = last + 1 + lowerBoundFloat64(c.bounds[last+1:], t)
	}
	if last < 1 {
		last = 1
	}
	if last >= len(c.bounds) {
		last = len(c.bounds) - 1
	}
	c.last = last

	upper := c.values[last]
	lower := c.values[last-1]
	if len(upper) != len(lower) {
		return nil, false
	}
	x := (t - c.bounds[last-1]) * c.coeff
	ix := 1.0 - x
	out := make([]float64, len(upper))
	for i := range out {
		out[i] = ix*lower[i] + x*upper[i]
	}
	return out, true
}

func lowerBoundFloat64(values []float64, target float64) int {
	lo, hi := 0, len(values)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if values[mid] < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func matrixNorm(m [6]float64) float64 {
	i := m[0]*m[0] + m[1]*m[1]
	j := m[2]*m[2] + m[3]*m[3]
	f := 0.5 * (i + j)
	g := 0.5 * (i - j)
	h := m[0]*m[2] + m[1]*m[3]
	return math.Sqrt(f + math.Hypot(g, h))
}

func evaluateShadingColorFunctions(functions []entity.Function, inputs []float64) ([]float64, error) {
	if len(functions) == 0 {
		return nil, fmt.Errorf("shading has no function")
	}
	if len(functions) == 1 {
		return functions[0].Evaluate(inputs)
	}

	colors := make([]float64, 0, len(functions))
	for _, function := range functions {
		values, err := function.Evaluate(inputs)
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

const radialEpsilon = 1.0 / 1024.0 / 1024.0

func popplerRadialParameter(
	px, py float64,
	x0, y0, r0 float64,
	dx, dy, dr float64,
	a float64,
	extend [2]bool,
	domainMin, domainMax float64,
) (float64, bool) {
	xs := px - x0
	ys := py - y0
	b := xs*dx + ys*dy + r0*dr
	c := xs*xs + ys*ys - r0*r0

	var s0, s1 float64
	if math.Abs(a) <= radialEpsilon {
		if math.Abs(b) <= radialEpsilon {
			return 0, false
		}
		s0 = 0.5 * c / b
		s1 = s0
	} else {
		d := b*b - a*c
		if d < 0 {
			return 0, false
		}
		sqrtD := math.Sqrt(d)
		invA := 1 / a
		s0 = (b + sqrtD) * invA
		s1 = (b - sqrtD) * invA
	}

	if value, ok := acceptPopplerRadialParameter(s0, r0, dr, extend, domainMin, domainMax); ok {
		return value, true
	}
	return acceptPopplerRadialParameter(s1, r0, dr, extend, domainMin, domainMax)
}

func acceptPopplerRadialParameter(
	s float64,
	r0 float64,
	dr float64,
	extend [2]bool,
	domainMin float64,
	domainMax float64,
) (float64, bool) {
	if r0+s*dr < 0 {
		return 0, false
	}
	switch {
	case s >= 0 && s <= 1:
		return domainMin + (domainMax-domainMin)*s, true
	case s < 0 && extend[0]:
		return domainMin, true
	case s > 1 && extend[1]:
		return domainMax, true
	default:
		return 0, false
	}
}

// drawFunctionBasedShading draws a function-based shading.
func (c *ImageCanvas) drawFunctionBasedShading(shading *entity.Shading, bbox [4]float64) error {
	domain := shading.GetDomain()
	matrix := shading.GetMatrix()

	functions := shading.GetFunctions()
	if len(functions) == 0 {
		return fmt.Errorf("function-based shading has no function")
	}

	inverse, err := c.shadingPatternInverseMatrix()
	if err != nil {
		return err
	}

	// Rasterize the bounding box
	xMin := int(math.Floor(bbox[0]))
	yMin := int(math.Floor(bbox[1]))
	xMax := int(math.Ceil(bbox[2]))
	yMax := int(math.Ceil(bbox[3]))

	for y := yMin; y < yMax; y++ {
		for x := xMin; x < xMax; x++ {
			// Map the device pixel center into shading space.
			px, py := c.shadingPatternPoint(float64(x)+0.5, float64(y)+0.5, inverse)

			// Apply matrix transformation
			sx := matrix[0]*px + matrix[2]*py + matrix[4]
			sy := matrix[1]*px + matrix[3]*py + matrix[5]

			// Check if point is in domain
			if sx < domain[0] || sx > domain[1] || sy < domain[2] || sy > domain[3] {
				continue
			}

			// Normalize to [0, 1]
			u := (sx - domain[0]) / (domain[1] - domain[0])
			v := (sy - domain[2]) / (domain[3] - domain[2])

			// Evaluate the shading function.
			colors, err := evaluateShadingColorFunctions(functions, []float64{u, v})
			if err != nil {
				continue
			}

			// Convert to RGBA color
			col := c.colorArrayToRGBA(colors)

			c.setPixelWithClip(x, y, col)
		}
	}

	return nil
}

// drawGouraudShading draws a Gouraud-shaded triangle mesh.
func (c *ImageCanvas) drawGouraudShading(shading *entity.Shading, bbox [4]float64) error {
	vertices := shading.GetVertices()
	if len(vertices) < 3 {
		return fmt.Errorf("gouraud shading requires at least 3 vertices")
	}

	clipBounds, ok := c.transformedShadingPixelBounds(shading)
	if !ok {
		return nil
	}

	// Simple triangle rendering with color interpolation
	// For proper implementation, would use scanline rasterization
	// with barycentric interpolation
	for i := 0; i < len(vertices)-2; i += 3 {
		if i+2 >= len(vertices) {
			break
		}

		v0 := c.transformShadingVertex(vertices[i])
		v1 := c.transformShadingVertex(vertices[i+1])
		v2 := c.transformShadingVertex(vertices[i+2])

		// Draw triangle with interpolated colors
		c.drawGouraudTriangle(v0, v1, v2, shading.GetFunctions(), clipBounds)
	}

	return nil
}

type gouraudScanVertex struct {
	x      int
	y      int
	colors []float64
}

type gouraudScanEdge struct {
	edge           [2]int
	limitMap       [2]float64
	colorSlope     []float64
	colorIntercept []float64
}

// drawGouraudTriangle draws a triangle with Gouraud shading.
func (c *ImageCanvas) drawGouraudTriangle(v0, v1, v2 entity.Vertex, functions []entity.Function, clipBounds image.Rectangle) {
	if c.drawGouraudTriangleScanline(v0, v1, v2, functions, clipBounds) {
		return
	}

	// Fall back to the older barycentric rasterizer for unsupported color layouts.
	c.drawGouraudTriangleBarycentric(v0, v1, v2, functions, clipBounds)
}

func (c *ImageCanvas) drawGouraudTriangleScanline(v0, v1, v2 entity.Vertex, functions []entity.Function, clipBounds image.Rectangle) bool {
	if len(v0.Colors) == 0 || len(v0.Colors) != len(v1.Colors) || len(v0.Colors) != len(v2.Colors) {
		return false
	}

	vertices := [3]gouraudScanVertex{
		c.newGouraudScanVertex(v0),
		c.newGouraudScanVertex(v1),
		c.newGouraudScanVertex(v2),
	}
	sortGouraudScanVertices(vertices[:])

	if (vertices[0].x-vertices[2].x)*(vertices[1].y-vertices[2].y)-(vertices[1].x-vertices[2].x)*(vertices[0].y-vertices[2].y) == 0 {
		return true
	}

	colorComps := len(vertices[0].colors)
	left := gouraudScanEdge{
		colorSlope:     make([]float64, colorComps),
		colorIntercept: make([]float64, colorComps),
	}
	right := gouraudScanEdge{
		colorSlope:     make([]float64, colorComps),
		colorIntercept: make([]float64, colorComps),
	}

	left.edge[0] = 0
	right.edge[0] = 0
	if vertices[0].y == vertices[1].y {
		left.edge[0] = 1
		left.edge[1] = 2
		right.edge[1] = 2
	} else {
		left.edge[1] = 1
		right.edge[1] = 2
	}
	if !configureGouraudScanEdge(vertices[:], &left) || !configureGouraudScanEdge(vertices[:], &right) {
		return true
	}

	xa := float64(vertices[1].y)*left.limitMap[0] + left.limitMap[1]
	xt := float64(vertices[1].y)*right.limitMap[0] + right.limitMap[1]
	if xa > xt {
		left, right = right, left
	}

	hasFurtherSegment := vertices[1].y < vertices[2].y
	leftColors := make([]float64, colorComps)
	rightColors := make([]float64, colorComps)
	curColors := make([]float64, colorComps)

	for y := vertices[0].y; y <= vertices[2].y; y++ {
		if hasFurtherSegment && y == vertices[1].y {
			if left.edge[1] == 1 {
				left.edge = [2]int{1, 2}
				if !configureGouraudScanEdge(vertices[:], &left) {
					return true
				}
			} else if right.edge[1] == 1 {
				right.edge = [2]int{1, 2}
				if !configureGouraudScanEdge(vertices[:], &right) {
					return true
				}
			}
			hasFurtherSegment = false
		}

		yf := float64(y)
		xa = yf*left.limitMap[0] + left.limitMap[1]
		xt = yf*right.limitMap[0] + right.limitMap[1]
		scanLimitL := splashRoundCoord(xa)
		scanLimitR := splashRoundCoord(xt)
		if scanLimitL > scanLimitR {
			continue
		}

		evaluateGouraudEdgeColors(left, y, leftColors)
		evaluateGouraudEdgeColors(right, y, rightColors)
		scanLeftColors := leftColors
		scanRightColors := rightColors

		if y < clipBounds.Min.Y || y >= clipBounds.Max.Y {
			continue
		}

		startX := scanLimitL
		if startX < clipBounds.Min.X {
			startX = clipBounds.Min.X
		}
		endX := scanLimitR
		if endX >= clipBounds.Max.X {
			endX = clipBounds.Max.X - 1
		}
		if startX > endX {
			continue
		}

		spanWidth := scanLimitR - scanLimitL
		for x := startX; x <= endX; x++ {
			if spanWidth == 0 {
				copy(curColors, scanLeftColors)
			} else {
				t := float64(x-scanLimitL) / float64(spanWidth)
				for i := 0; i < colorComps; i++ {
					curColors[i] = scanLeftColors[i] + (scanRightColors[i]-scanLeftColors[i])*t
				}
			}

			evaluatedColors, ok := evaluateGouraudTriangleColors(curColors, functions)
			if !ok {
				continue
			}
			if len(functions) > 0 {
				evaluatedColors = quantizeGouraudFunctionOutput(evaluatedColors)
			}
			c.setCanvasPixelWithClip(x, y, c.colorArrayToRGBA(evaluatedColors))
		}
	}

	return true
}

func (c *ImageCanvas) drawGouraudTriangleBarycentric(v0, v1, v2 entity.Vertex, functions []entity.Function, clipBounds image.Rectangle) {
	// Calculate bounding box of triangle
	xMin := int(math.Min(math.Min(v0.X, v1.X), v2.X))
	xMax := int(math.Max(math.Max(v0.X, v1.X), v2.X))
	yMin := int(math.Min(math.Min(v0.Y, v1.Y), v2.Y))
	yMax := int(math.Max(math.Max(v0.Y, v1.Y), v2.Y))

	if xMin < clipBounds.Min.X {
		xMin = clipBounds.Min.X
	}
	if yMin < clipBounds.Min.Y {
		yMin = clipBounds.Min.Y
	}
	if xMax >= clipBounds.Max.X {
		xMax = clipBounds.Max.X - 1
	}
	if yMax >= clipBounds.Max.Y {
		yMax = clipBounds.Max.Y - 1
	}
	if xMin > xMax || yMin > yMax {
		return
	}

	// Iterate over pixels in bounding box
	for y := yMin; y <= yMax; y++ {
		for x := xMin; x <= xMax; x++ {
			px := float64(x)
			py := float64(y)

			// Calculate barycentric coordinates
			denom := (v1.Y-v2.Y)*(v0.X-v2.X) + (v2.X-v1.X)*(v0.Y-v2.Y)
			if denom == 0 {
				continue
			}

			w0 := ((v1.Y-v2.Y)*(px-v2.X) + (v2.X-v1.X)*(py-v2.Y)) / denom
			w1 := ((v2.Y-v0.Y)*(px-v2.X) + (v0.X-v2.X)*(py-v2.Y)) / denom
			w2 := 1 - w0 - w1

			// Check if point is inside triangle
			if w0 < 0 || w1 < 0 || w2 < 0 {
				continue
			}

			// Interpolate colors
			numComps := len(v0.Colors)
			if numComps == 0 {
				continue
			}

			colors := make([]float64, numComps)
			for i := 0; i < numComps; i++ {
				colors[i] = w0*v0.Colors[i] + w1*v1.Colors[i] + w2*v2.Colors[i]
			}

			evaluatedColors, ok := evaluateGouraudTriangleColors(colors, functions)
			if !ok {
				continue
			}

			// Convert to RGBA color
			col := c.colorArrayToRGBA(evaluatedColors)

			c.setCanvasPixelWithClip(x, y, col)
		}
	}
}

func (c *ImageCanvas) newGouraudScanVertex(v entity.Vertex) gouraudScanVertex {
	return gouraudScanVertex{
		x:      splashRoundCoord(v.X),
		y:      splashRoundCoord(v.Y),
		colors: append([]float64(nil), v.Colors...),
	}
}

func splashRoundCoord(value float64) int {
	return int(math.Floor(value + 0.5))
}

func sortGouraudScanVertices(vertices []gouraudScanVertex) {
	if vertices[0].y > vertices[1].y {
		vertices[0], vertices[1] = vertices[1], vertices[0]
	}
	if vertices[1].y > vertices[2].y {
		vertices[1], vertices[2] = vertices[2], vertices[1]
		if vertices[0].y > vertices[1].y {
			vertices[0], vertices[1] = vertices[1], vertices[0]
		}
	}
}

func configureGouraudScanEdge(vertices []gouraudScanVertex, edge *gouraudScanEdge) bool {
	from := vertices[edge.edge[0]]
	to := vertices[edge.edge[1]]
	dy := to.y - from.y
	if dy == 0 {
		return false
	}

	edge.limitMap[0] = float64(to.x-from.x) / float64(dy)
	edge.limitMap[1] = float64(from.x) - float64(from.y)*edge.limitMap[0]
	for i := range edge.colorSlope {
		edge.colorSlope[i] = (to.colors[i] - from.colors[i]) / float64(dy)
		edge.colorIntercept[i] = from.colors[i] - float64(from.y)*edge.colorSlope[i]
	}

	return true
}

func evaluateGouraudEdgeColors(edge gouraudScanEdge, y int, out []float64) {
	yf := float64(y)
	for i := range out {
		out[i] = yf*edge.colorSlope[i] + edge.colorIntercept[i]
	}
}

func evaluateGouraudTriangleColors(colors []float64, functions []entity.Function) ([]float64, bool) {
	if len(functions) == 0 {
		return colors, true
	}

	switch {
	case len(functions) == 1:
		functionColors, err := functions[0].Evaluate(colors)
		if err != nil {
			return nil, false
		}
		return functionColors, true
	case len(colors) == 1:
		evaluatedColors := make([]float64, 0, len(functions))
		for _, function := range functions {
			functionColor, err := function.Evaluate([]float64{colors[0]})
			if err != nil || len(functionColor) == 0 {
				return nil, false
			}
			evaluatedColors = append(evaluatedColors, functionColor[0])
		}
		return evaluatedColors, true
	default:
		return nil, false
	}
}

func quantizeGouraudFunctionOutput(colors []float64) []float64 {
	const gfxColorComp1 = 0x10000

	out := make([]float64, len(colors))
	for i, component := range colors {
		out[i] = float64(int(component*gfxColorComp1)) / gfxColorComp1
	}
	return out
}

// drawPatchMeshShading draws a patch mesh shading.
func (c *ImageCanvas) drawPatchMeshShading(shading *entity.Shading, bbox [4]float64) error {
	vertices := shading.GetVertices()
	if len(vertices) < 4 {
		return fmt.Errorf("patch mesh shading requires at least 4 vertices")
	}

	clipBounds, ok := c.transformedShadingPixelBounds(shading)
	if !ok {
		return nil
	}

	// Simplified implementation - render patches as quads
	// Full implementation would render Coons or tensor-product patches
	for i := 0; i < len(vertices)-3; i += 4 {
		if i+3 >= len(vertices) {
			break
		}

		v0 := c.transformShadingVertex(vertices[i])
		v1 := c.transformShadingVertex(vertices[i+1])
		v2 := c.transformShadingVertex(vertices[i+2])
		v3 := c.transformShadingVertex(vertices[i+3])

		// Render as two triangles
		c.drawGouraudTriangle(v0, v1, v2, shading.GetFunctions(), clipBounds)
		c.drawGouraudTriangle(v0, v2, v3, shading.GetFunctions(), clipBounds)
	}

	return nil
}

// colorArrayToRGBA converts a color array to color.RGBA.
func (c *ImageCanvas) colorArrayToRGBA(colors []float64) color.RGBA {
	// Assume RGB color space
	var r, g, b, a uint8

	switch len(colors) {
	case 1:
		// Grayscale
		val := colorComponentToByte(colors[0])
		return color.RGBA{R: val, G: val, B: val, A: 255}
	case 3:
		// RGB
		r = colorComponentToByte(colors[0])
		g = colorComponentToByte(colors[1])
		b = colorComponentToByte(colors[2])
		a = 255
	case 4:
		return domaincolorspace.ConvertDeviceCMYKToRGBA(colors)
	default:
		return color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}

	return color.RGBA{R: r, G: g, B: b, A: a}
}

func colorComponentToByte(component float64) uint8 {
	if component < 0 {
		return 0
	}
	if component > 1 {
		return 255
	}
	const gfxColorComp1 = 0x10000
	fixed := int(component * gfxColorComp1)
	return uint8(((fixed << 8) - fixed + 0x8000) >> 16)
}

// patternXRef is a minimal XRef implementation for pattern content streams.
// Patterns don't have indirect references, so most methods return errors or nil.
type patternXRef struct{}

// Fetch is an exported API.
func (p *patternXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	return nil, fmt.Errorf("patterns do not support indirect references")
}

// GetNumObjects returns the requested value.
func (p *patternXRef) GetNumObjects() int {
	return 0
}

// Parse is an exported API.
func (p *patternXRef) Parse() error {
	return nil
}

// GetTrailer returns the requested value.
func (p *patternXRef) GetTrailer() *entity.Dict {
	return nil
}

// GetCatalog returns the requested value.
func (p *patternXRef) GetCatalog() (entity.Object, error) {
	return nil, fmt.Errorf("patterns do not have catalog")
}
