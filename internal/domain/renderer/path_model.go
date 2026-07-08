package renderer

import "math"

// Path represents a PDF graphics path.
type Path struct {
	elements     []pathElement
	coords       []float64
	elementCache []PathElement
	currentX     float64
	currentY     float64
	moveX        float64
	moveY        float64
	closed       bool
}

type pathElement struct {
	coordStart int32
	kind       uint8
}

// PathElement represents an element in a path.
type PathElement interface {
	Type() PathElementType
}

// PathElementType is the type of path element.
type PathElementType int

const (
	PathMoveTo PathElementType = iota
	PathLineTo
	PathCurveTo
	PathClose
)

// MoveTo represents a move-to operation.
type MoveTo struct {
	X, Y float64
}

// Type is an exported API.
func (m *MoveTo) Type() PathElementType { return PathMoveTo }

// LineTo represents a line-to operation.
type LineTo struct {
	X, Y float64
}

// Type is an exported API.
func (l *LineTo) Type() PathElementType { return PathLineTo }

// CurveTo represents a cubic Bézier curve.
type CurveTo struct {
	X1, Y1 float64 // First control point
	X2, Y2 float64 // Second control point
	X, Y   float64 // End point
}

// Type is an exported API.
func (c *CurveTo) Type() PathElementType { return PathCurveTo }

// Close represents a close-path operation.
type Close struct{}

// Type is an exported API.
func (c *Close) Type() PathElementType { return PathClose }

// NewPath creates a new empty path.
func NewPath() *Path {
	return &Path{
		elements: make([]pathElement, 0, 8),
	}
}

// AddElement adds an element to the path.
func (p *Path) AddElement(elem PathElement) {
	switch e := elem.(type) {
	case *MoveTo:
		p.MoveTo(e.X, e.Y)
	case *LineTo:
		p.LineTo(e.X, e.Y)
	case *CurveTo:
		p.CurveTo(e.X1, e.Y1, e.X2, e.Y2, e.X, e.Y)
	case *Close:
		p.ClosePath()
	}
}

// Elements returns all path elements.
func (p *Path) Elements() []PathElement {
	if len(p.elements) == 0 {
		return nil
	}
	if len(p.elementCache) == len(p.elements) {
		return p.elementCache
	}
	p.elementCache = make([]PathElement, len(p.elements))
	for i, elem := range p.elements {
		start := int(elem.coordStart)
		switch PathElementType(elem.kind) {
		case PathMoveTo:
			p.elementCache[i] = &MoveTo{X: p.coords[start], Y: p.coords[start+1]}
		case PathLineTo:
			p.elementCache[i] = &LineTo{X: p.coords[start], Y: p.coords[start+1]}
		case PathCurveTo:
			p.elementCache[i] = &CurveTo{
				X1: p.coords[start], Y1: p.coords[start+1],
				X2: p.coords[start+2], Y2: p.coords[start+3],
				X: p.coords[start+4], Y: p.coords[start+5],
			}
		case PathClose:
			p.elementCache[i] = &Close{}
		}
	}
	return p.elementCache
}

// ElementCount returns the number of stored path elements.
func (p *Path) ElementCount() int {
	if p == nil {
		return 0
	}
	return len(p.elements)
}

// ElementAt returns a path element in value form without allocating interface wrappers.
func (p *Path) ElementAt(index int) (PathElementType, float64, float64, float64, float64, float64, float64, bool) {
	if p == nil || index < 0 || index >= len(p.elements) {
		return 0, 0, 0, 0, 0, 0, 0, false
	}
	elem := p.elements[index]
	start := int(elem.coordStart)
	switch typ := PathElementType(elem.kind); typ {
	case PathMoveTo, PathLineTo:
		return typ, 0, 0, 0, 0, p.coords[start], p.coords[start+1], true
	case PathCurveTo:
		return typ,
			p.coords[start], p.coords[start+1],
			p.coords[start+2], p.coords[start+3],
			p.coords[start+4], p.coords[start+5],
			true
	case PathClose:
		return typ, 0, 0, 0, 0, 0, 0, true
	default:
		return typ, 0, 0, 0, 0, 0, 0, true
	}
}

// PointCapacityHint returns the number of Splash path points represented by this path.
func (p *Path) PointCapacityHint() int {
	if p == nil {
		return 0
	}
	pointCapacity := 0
	for _, elem := range p.elements {
		if PathElementType(elem.kind) == PathCurveTo {
			pointCapacity += 3
		} else {
			pointCapacity++
		}
	}
	return pointCapacity
}

// CurrentPoint returns the current point on the path.
func (p *Path) CurrentPoint() (float64, float64) {
	return p.currentX, p.currentY
}

// MovePoint returns the move-to point (start of current subpath).
func (p *Path) MovePoint() (float64, float64) {
	return p.moveX, p.moveY
}

// IsEmpty returns true if the path has no elements.
func (p *Path) IsEmpty() bool {
	return len(p.elements) == 0
}

// Clear removes all elements from the path.
func (p *Path) Clear() {
	p.elements = p.elements[:0]
	p.coords = p.coords[:0]
	p.elementCache = nil
	p.currentX = 0
	p.currentY = 0
	p.moveX = 0
	p.moveY = 0
	p.closed = false
}

// Clone creates a deep copy of the path.
func (p *Path) Clone() *Path {
	if len(p.elements) == 0 {
		return &Path{}
	}
	clone := &Path{
		elements: make([]pathElement, len(p.elements)),
		coords:   make([]float64, len(p.coords)),
		currentX: p.currentX,
		currentY: p.currentY,
		moveX:    p.moveX,
		moveY:    p.moveY,
		closed:   p.closed,
	}
	copy(clone.elements, p.elements)
	copy(clone.coords, p.coords)
	return clone
}

// MoveTo appends a move-to operation.
func (p *Path) MoveTo(x, y float64) {
	start := len(p.coords)
	p.coords = append(p.coords, x, y)
	p.elements = append(p.elements, pathElement{
		coordStart: int32(start),
		kind:       uint8(PathMoveTo),
	})
	p.elementCache = nil
	p.currentX = x
	p.currentY = y
	p.moveX = x
	p.moveY = y
	p.closed = false
}

// LineTo appends a line-to operation.
func (p *Path) LineTo(x, y float64) {
	start := len(p.coords)
	p.coords = append(p.coords, x, y)
	p.elements = append(p.elements, pathElement{
		coordStart: int32(start),
		kind:       uint8(PathLineTo),
	})
	p.elementCache = nil
	p.currentX = x
	p.currentY = y
	p.closed = false
}

// CurveTo appends a cubic Bezier curve.
func (p *Path) CurveTo(x1, y1, x2, y2, x, y float64) {
	start := len(p.coords)
	p.coords = append(p.coords, x1, y1, x2, y2, x, y)
	p.elements = append(p.elements, pathElement{
		coordStart: int32(start),
		kind:       uint8(PathCurveTo),
	})
	p.elementCache = nil
	p.currentX = x
	p.currentY = y
	p.closed = false
}

// ClosePath appends a close-path operation.
func (p *Path) ClosePath() {
	p.elements = append(p.elements, pathElement{
		kind: uint8(PathClose),
	})
	p.elementCache = nil
	p.currentX = p.moveX
	p.currentY = p.moveY
	p.closed = true
}

// AddRect appends a rectangle (move + 3 lines + close) as a single bulk operation.
// This avoids 5 separate appends and type switches in AddElement.
func (p *Path) AddRect(x1, y1, x2, y2, x3, y3, x4, y4 float64) {
	// Pre-allocate space for 5 elements in one grow.
	if cap(p.elements)-len(p.elements) < 5 {
		grown := make([]pathElement, len(p.elements), len(p.elements)+5)
		copy(grown, p.elements)
		p.elements = grown
	}
	if cap(p.coords)-len(p.coords) < 8 {
		grown := make([]float64, len(p.coords), len(p.coords)+8)
		copy(grown, p.coords)
		p.coords = grown
	}

	start := len(p.coords)
	p.coords = append(p.coords, x1, y1, x2, y2, x3, y3, x4, y4)
	p.elements = append(p.elements,
		pathElement{coordStart: int32(start), kind: uint8(PathMoveTo)},
		pathElement{coordStart: int32(start + 2), kind: uint8(PathLineTo)},
		pathElement{coordStart: int32(start + 4), kind: uint8(PathLineTo)},
		pathElement{coordStart: int32(start + 6), kind: uint8(PathLineTo)},
		pathElement{kind: uint8(PathClose)},
	)

	p.elementCache = nil
	p.currentX = x1
	p.currentY = y1
	p.moveX = x1
	p.moveY = y1
	p.closed = true
}

// GetBounds returns the bounding box of the path.
// Returns (xMin, yMin, xMax, yMax).
func (p *Path) GetBounds() (float64, float64, float64, float64) {
	if len(p.elements) == 0 {
		return 0, 0, 0, 0
	}

	xMin, yMin := math.MaxFloat64, math.MaxFloat64
	xMax, yMax := -math.MaxFloat64, -math.MaxFloat64

	for _, elem := range p.elements {
		start := int(elem.coordStart)
		switch PathElementType(elem.kind) {
		case PathMoveTo, PathLineTo:
			x := p.coords[start]
			y := p.coords[start+1]
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
		case PathCurveTo:
			x1, y1 := p.coords[start], p.coords[start+1]
			x2, y2 := p.coords[start+2], p.coords[start+3]
			x, y := p.coords[start+4], p.coords[start+5]
			if x1 < xMin {
				xMin = x1
			}
			if x1 > xMax {
				xMax = x1
			}
			if y1 < yMin {
				yMin = y1
			}
			if y1 > yMax {
				yMax = y1
			}
			if x2 < xMin {
				xMin = x2
			}
			if x2 > xMax {
				xMax = x2
			}
			if y2 < yMin {
				yMin = y2
			}
			if y2 > yMax {
				yMax = y2
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
		}
	}

	return xMin, yMin, xMax, yMax
}
