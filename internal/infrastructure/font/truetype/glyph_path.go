// Package truetype provides TrueType/OpenType font implementation.
package truetype

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	compositeArgWords         uint16 = 0x0001
	compositeArgsAreXYValues  uint16 = 0x0002
	compositeHaveScale        uint16 = 0x0008
	compositeMoreComponents   uint16 = 0x0020
	compositeHaveXYScale      uint16 = 0x0040
	compositeHaveTwoByTwo     uint16 = 0x0080
	compositeHaveInstructions uint16 = 0x0100
)

// PathOp represents a path operation type.
type PathOp int

const (
	opMoveTo PathOp = iota
	opLineTo
	opQuadTo
	opClosePath
)

// PathElement represents a single element in a glyph path.
type PathElement struct {
	X      float64
	Y      float64
	CX     float64 // Control point X for quadratic curves
	CY     float64 // Control point Y for quadratic curves
	Op     PathOp
	IsCtrl bool // True if this is a control point
}

// GlyphPath represents a glyph outline as a series of path elements.
type GlyphPath struct {
	elements  []PathElement
	startX    float64
	startY    float64
	lastX     float64
	lastY     float64
	hasMoveTo bool
}

// NewGlyphPath creates a new glyph path.
func NewGlyphPath() *GlyphPath {
	return &GlyphPath{
		elements: make([]PathElement, 0),
	}
}

// MoveTo starts a new subpath at the given point.
func (p *GlyphPath) MoveTo(x, y float64) {
	p.startX = x
	p.startY = y
	p.lastX = x
	p.lastY = y
	p.hasMoveTo = true
	p.elements = append(p.elements, PathElement{X: x, Y: y, Op: opMoveTo})
}

// LineTo adds a line from the current point to the given point.
func (p *GlyphPath) LineTo(x, y float64) {
	if !p.hasMoveTo {
		p.MoveTo(0, 0)
	}
	p.lastX = x
	p.lastY = y
	p.elements = append(p.elements, PathElement{X: x, Y: y, Op: opLineTo})
}

// QuadTo adds a quadratic Bézier curve from the current point.
func (p *GlyphPath) QuadTo(cx, cy, x, y float64) {
	if !p.hasMoveTo {
		p.MoveTo(0, 0)
	}
	p.elements = append(p.elements, PathElement{
		X: x, Y: y,
		CX: cx, CY: cy,
		Op: opQuadTo,
	})
	p.lastX = x
	p.lastY = y
}

// ClosePath closes the current subpath.
func (p *GlyphPath) ClosePath() {
	if !p.hasMoveTo {
		return
	}
	p.elements = append(p.elements, PathElement{X: p.startX, Y: p.startY, Op: opClosePath})
	p.lastX = p.startX
	p.lastY = p.startY
	p.hasMoveTo = false
}

// Elements returns the path elements.
func (p *GlyphPath) Elements() []PathElement {
	return p.elements
}

// AddOnCurvePoint adds an on-curve point to the path.
func (p *GlyphPath) AddOnCurvePoint(x, y float64) {
	p.LineTo(x, y)
}

// AddOffCurvePoint adds an off-curve (control) point.
func (p *GlyphPath) AddOffCurvePoint(x, y float64) {
	p.elements = append(p.elements, PathElement{
		X:      x,
		Y:      y,
		Op:     opQuadTo,
		IsCtrl: true,
	})
}

// parseGlyphOutline parses the glyph outline data from the glyf table.
func (f *Font) parseGlyphOutline(glyphData []byte, numberOfContours int16) (*GlyphPath, error) {
	path := NewGlyphPath()

	if numberOfContours == 0 {
		// Empty glyph
		return path, nil
	}

	// Simple glyph
	if numberOfContours > 0 {
		return f.parseSimpleGlyph(glyphData, numberOfContours)
	}

	// Composite glyph
	return f.parseCompositeGlyph(glyphData)
}

// parseCompositeGlyph parses a composite glyph made of multiple component glyphs.
func (f *Font) parseCompositeGlyph(data []byte) (*GlyphPath, error) {
	r := bytes.NewReader(data)
	path := NewGlyphPath()

	var haveInstructions bool

	componentCount := 0
	for {
		componentCount++
		if componentCount > 256 {
			return nil, fmt.Errorf("too many composite glyph components")
		}

		// Read component flags
		var flags uint16
		if err := binary.Read(r, binary.BigEndian, &flags); err != nil {
			return nil, fmt.Errorf("failed to read component flags: %w", err)
		}

		// Read glyph index
		var glyphIndex uint16
		if err := binary.Read(r, binary.BigEndian, &glyphIndex); err != nil {
			return nil, fmt.Errorf("failed to read glyph index: %w", err)
		}

		// Parse optional arguments based on flags
		var arg1, arg2 int16

		if flags&compositeArgWords != 0 {
			// ARG_1_AND_2_ARE_WORDS
			if err := binary.Read(r, binary.BigEndian, &arg1); err != nil {
				return nil, fmt.Errorf("failed to read arg1: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &arg2); err != nil {
				return nil, fmt.Errorf("failed to read arg2: %w", err)
			}
		} else {
			// ARG_1_AND_2_ARE_BYTES
			var b1, b2 int8
			if err := binary.Read(r, binary.BigEndian, &b1); err != nil {
				return nil, fmt.Errorf("failed to read arg1: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &b2); err != nil {
				return nil, fmt.Errorf("failed to read arg2: %w", err)
			}
			arg1 = int16(b1)
			arg2 = int16(b2)
		}

		// Determine x and y offsets based on flags
		var x, y float64
		if flags&compositeArgsAreXYValues != 0 {
			// ARGS_ARE_XY_VALUES
			x = float64(arg1)
			y = float64(arg2)
		} else {
			// ARGS_ARE_POINT_VALUES (not implemented)
			x = 0
			y = 0
		}

		// Parse transformation if present
		var transform [6]float64
		switch {
		case flags&compositeHaveScale != 0:
			// WE_HAVE_A_SCALE
			var raw int16
			if err := binary.Read(r, binary.BigEndian, &raw); err != nil {
				return nil, fmt.Errorf("failed to read scale: %w", err)
			}
			scale := fixed2Dot14ToFloat(raw)
			transform = [6]float64{scale, 0, 0, scale, x, y}
		case flags&compositeHaveXYScale != 0:
			// WE_HAVE_AN_X_AND_Y_SCALE
			var xRaw, yRaw int16
			if err := binary.Read(r, binary.BigEndian, &xRaw); err != nil {
				return nil, fmt.Errorf("failed to read xscale: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &yRaw); err != nil {
				return nil, fmt.Errorf("failed to read yscale: %w", err)
			}
			xscale := fixed2Dot14ToFloat(xRaw)
			yscale := fixed2Dot14ToFloat(yRaw)
			transform = [6]float64{xscale, 0, 0, yscale, x, y}
		case flags&compositeHaveTwoByTwo != 0:
			// WE_HAVE_A_TWO_BY_TWO
			var xxRaw, yxRaw, xyRaw, yyRaw int16
			if err := binary.Read(r, binary.BigEndian, &xxRaw); err != nil {
				return nil, fmt.Errorf("failed to read xx: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &yxRaw); err != nil {
				return nil, fmt.Errorf("failed to read yx: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &xyRaw); err != nil {
				return nil, fmt.Errorf("failed to read xy: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &yyRaw); err != nil {
				return nil, fmt.Errorf("failed to read yy: %w", err)
			}
			xx := fixed2Dot14ToFloat(xxRaw)
			yx := fixed2Dot14ToFloat(yxRaw)
			xy := fixed2Dot14ToFloat(xyRaw)
			yy := fixed2Dot14ToFloat(yyRaw)
			transform = [6]float64{xx, yx, xy, yy, x, y}
		default:
			// No transformation, just translation
			transform = [6]float64{1, 0, 0, 1, x, y}
		}

		// Check for instructions flag
		if flags&compositeHaveInstructions != 0 {
			haveInstructions = true
		}

		// Get the component glyph and render it
		componentGlyphData, err := f.file.GetGlyphData(glyphIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to get component glyph %d: %w", glyphIndex, err)
		}

		// Parse the component glyph recursively
		componentPath, err := f.parseGlyphOutline(componentGlyphData.Instructions, componentGlyphData.NumberOfContours)
		if err != nil {
			return nil, fmt.Errorf("failed to parse component glyph %d: %w", glyphIndex, err)
		}

		// Apply transformation to the component path and add to main path
		f.transformAndAddPath(path, componentPath, transform)

		if flags&compositeMoreComponents == 0 {
			break
		}
	}

	// Read instructions if present
	if haveInstructions {
		var instructionsLen uint16
		if err := binary.Read(r, binary.BigEndian, &instructionsLen); err != nil {
			return nil, fmt.Errorf("failed to read instructions length: %w", err)
		}
		if instructionsLen > 0 {
			// Skip instructions (they're for hinting, not outline data)
			if _, err := r.Seek(int64(instructionsLen), io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("failed to skip instructions: %w", err)
			}
		}
	}

	return path, nil
}

func fixed2Dot14ToFloat(value int16) float64 {
	return float64(value) / 16384.0
}

// transformAndAddPath applies a transformation to a path and adds it to the main path.
func (f *Font) transformAndAddPath(mainPath, componentPath *GlyphPath, transform [6]float64) {
	for _, elem := range componentPath.Elements() {
		// Apply transformation to the point
		tx := transform[0]*elem.X + transform[2]*elem.Y + transform[4]
		ty := transform[1]*elem.X + transform[3]*elem.Y + transform[5]

		// Also transform control point if present
		var tcx, tcy float64
		if elem.Op == opQuadTo {
			tcx = transform[0]*elem.CX + transform[2]*elem.CY + transform[4]
			tcy = transform[1]*elem.CX + transform[3]*elem.CY + transform[5]
		}

		// Add the transformed element to the main path
		switch elem.Op {
		case opMoveTo:
			mainPath.MoveTo(tx, ty)
		case opLineTo:
			mainPath.LineTo(tx, ty)
		case opQuadTo:
			mainPath.QuadTo(tcx, tcy, tx, ty)
		case opClosePath:
			mainPath.ClosePath()
		}
	}
}

// parseSimpleGlyph parses a simple (non-composite) glyph.
func (f *Font) parseSimpleGlyph(data []byte, numContours int16) (*GlyphPath, error) {
	r := bytes.NewReader(data)
	path := NewGlyphPath()

	// Read end points of contours
	endPts := make([]uint16, numContours)
	for i := int16(0); i < numContours; i++ {
		if err := binary.Read(r, binary.BigEndian, &endPts[i]); err != nil {
			return nil, fmt.Errorf("failed to read endpoint: %w", err)
		}
	}

	// Skip instruction length and instructions
	var instLen uint16
	if err := binary.Read(r, binary.BigEndian, &instLen); err != nil {
		return nil, fmt.Errorf("failed to read instruction length: %w", err)
	}
	if instLen > 0 {
		if _, err := r.Seek(int64(instLen), io.SeekCurrent); err != nil {
			return nil, fmt.Errorf("failed to skip instructions: %w", err)
		}
	}

	// Read flags
	var flags []byte
	totalPoints := int(endPts[numContours-1]) + 1
	for i := 0; i < totalPoints; {
		flag := byte(0)
		if err := binary.Read(r, binary.BigEndian, &flag); err != nil {
			return nil, fmt.Errorf("failed to read flag: %w", err)
		}
		flags = append(flags, flag)
		i++

		// Repeat flag if bit 3 is set
		if flag&0x8 != 0 {
			var repeatCount byte
			if err := binary.Read(r, binary.BigEndian, &repeatCount); err != nil {
				return nil, fmt.Errorf("failed to read repeat count: %w", err)
			}
			for j := byte(0); j < repeatCount; j++ {
				flags = append(flags, flag)
				i++
			}
		}
	}

	// Read X coordinates (delta-encoded; first point is relative to 0)
	xCoords := make([]int16, totalPoints)
	for i := 0; i < totalPoints; i++ {
		flag := flags[i]
		var prevX int16
		if i > 0 {
			prevX = xCoords[i-1]
		}
		switch {
		case flag&0x2 != 0:
			// 1-byte delta
			var dx uint8
			if err := binary.Read(r, binary.BigEndian, &dx); err != nil {
				return nil, fmt.Errorf("failed to read x delta: %w", err)
			}
			if flag&0x10 != 0 {
				// Positive
				xCoords[i] = prevX + int16(dx)
			} else {
				xCoords[i] = prevX - int16(dx)
			}
		case flag&0x10 != 0:
			// Same as previous
			xCoords[i] = prevX
		default:
			// 2-byte delta
			var dx int16
			if err := binary.Read(r, binary.BigEndian, &dx); err != nil {
				return nil, fmt.Errorf("failed to read x delta: %w", err)
			}
			xCoords[i] = prevX + dx
		}
	}

	// Read Y coordinates (delta-encoded; first point is relative to 0)
	yCoords := make([]int16, totalPoints)
	for i := 0; i < totalPoints; i++ {
		flag := flags[i]
		var prevY int16
		if i > 0 {
			prevY = yCoords[i-1]
		}
		switch {
		case flag&0x4 != 0:
			// 1-byte delta
			var dy uint8
			if err := binary.Read(r, binary.BigEndian, &dy); err != nil {
				return nil, fmt.Errorf("failed to read y delta: %w", err)
			}
			if flag&0x20 != 0 {
				// Positive
				yCoords[i] = prevY + int16(dy)
			} else {
				yCoords[i] = prevY - int16(dy)
			}
		case flag&0x20 != 0:
			// Same as previous
			yCoords[i] = prevY
		default:
			// 2-byte delta
			var dy int16
			if err := binary.Read(r, binary.BigEndian, &dy); err != nil {
				return nil, fmt.Errorf("failed to read y delta: %w", err)
			}
			yCoords[i] = prevY + dy
		}
	}

	// Convert points to path using TrueType quadratic spline rules.
	// Key rule: two consecutive off-curve points have an implicit on-curve
	// midpoint between them (TrueType spec section 2).
	contourStart := 0
	for _, endPt := range endPts {
		n := int(endPt) - contourStart + 1
		if n == 0 {
			contourStart = int(endPt) + 1
			continue
		}

		xs := make([]float64, n)
		ys := make([]float64, n)
		onCurve := make([]bool, n)
		for k := 0; k < n; k++ {
			idx := contourStart + k
			xs[k] = float64(xCoords[idx])
			ys[k] = float64(yCoords[idx])
			onCurve[k] = (flags[idx] & 0x1) != 0
		}

		// Find first on-curve point to use as the path start.
		firstOnCurve := -1
		for k := 0; k < n; k++ {
			if onCurve[k] {
				firstOnCurve = k
				break
			}
		}

		var startX, startY float64
		var pendingOffCurve bool
		var pcx, pcy float64
		var processFrom, processCount int

		if firstOnCurve >= 0 {
			startX, startY = xs[firstOnCurve], ys[firstOnCurve]
			processFrom = firstOnCurve + 1
			processCount = n - 1
		} else {
			// All off-curve: synthesize start as midpoint of last and first.
			startX = (xs[n-1] + xs[0]) / 2
			startY = (ys[n-1] + ys[0]) / 2
			// First off-curve point becomes the initial pending control point.
			pendingOffCurve = true
			pcx, pcy = xs[0], ys[0]
			processFrom = 1
			processCount = n - 1
		}

		path.MoveTo(startX, startY)

		for k := 0; k < processCount; k++ {
			idx := (processFrom + k) % n
			if onCurve[idx] {
				if pendingOffCurve {
					path.QuadTo(pcx, pcy, xs[idx], ys[idx])
					pendingOffCurve = false
				} else {
					path.LineTo(xs[idx], ys[idx])
				}
			} else {
				if pendingOffCurve {
					// Two consecutive off-curve points: insert implicit on-curve midpoint.
					midX := (pcx + xs[idx]) / 2
					midY := (pcy + ys[idx]) / 2
					path.QuadTo(pcx, pcy, midX, midY)
				}
				pendingOffCurve = true
				pcx, pcy = xs[idx], ys[idx]
			}
		}

		// Close contour: if a pending off-curve remains, curve back to start.
		if pendingOffCurve {
			path.QuadTo(pcx, pcy, startX, startY)
		}
		path.ClosePath()
		contourStart = int(endPt) + 1
	}

	return path, nil
}
