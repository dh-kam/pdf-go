package renderer

import "math"

// Path represents a PDF graphics path.
type Path struct {
	elements []PathElement
	currentX float64
	currentY float64
	moveX    float64
	moveY    float64
	closed   bool
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
		elements: make([]PathElement, 0, 8),
	}
}

// AddElement adds an element to the path.
func (p *Path) AddElement(elem PathElement) {
	p.elements = append(p.elements, elem)

	switch e := elem.(type) {
	case *MoveTo:
		p.currentX = e.X
		p.currentY = e.Y
		p.moveX = e.X
		p.moveY = e.Y
		p.closed = false
	case *LineTo:
		p.currentX = e.X
		p.currentY = e.Y
		p.closed = false
	case *CurveTo:
		p.currentX = e.X
		p.currentY = e.Y
		p.closed = false
	case *Close:
		p.currentX = p.moveX
		p.currentY = p.moveY
		p.closed = true
	}
}

// Elements returns all path elements.
func (p *Path) Elements() []PathElement {
	return p.elements
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
		elements: make([]PathElement, len(p.elements)),
		currentX: p.currentX,
		currentY: p.currentY,
		moveX:    p.moveX,
		moveY:    p.moveY,
		closed:   p.closed,
	}
	copy(clone.elements, p.elements)
	return clone
}

// AddRect appends a rectangle (move + 3 lines + close) as a single bulk operation.
// This avoids 5 separate heap allocations and type switches in AddElement.
func (p *Path) AddRect(x1, y1, x2, y2, x3, y3, x4, y4 float64) {
	// Pre-allocate space for 5 elements in one grow.
	if cap(p.elements)-len(p.elements) < 5 {
		grown := make([]PathElement, len(p.elements), len(p.elements)+5)
		copy(grown, p.elements)
		p.elements = grown
	}

	p.elements = append(p.elements,
		&MoveTo{X: x1, Y: y1},
		&LineTo{X: x2, Y: y2},
		&LineTo{X: x3, Y: y3},
		&LineTo{X: x4, Y: y4},
		&Close{},
	)

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
		switch e := elem.(type) {
		case *MoveTo:
			if e.X < xMin {
				xMin = e.X
			}
			if e.X > xMax {
				xMax = e.X
			}
			if e.Y < yMin {
				yMin = e.Y
			}
			if e.Y > yMax {
				yMax = e.Y
			}
		case *LineTo:
			if e.X < xMin {
				xMin = e.X
			}
			if e.X > xMax {
				xMax = e.X
			}
			if e.Y < yMin {
				yMin = e.Y
			}
			if e.Y > yMax {
				yMax = e.Y
			}
		case *CurveTo:
			for _, pt := range []struct{ X, Y float64 }{
				{e.X1, e.Y1}, {e.X2, e.Y2}, {e.X, e.Y},
			} {
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
		}
	}

	return xMin, yMin, xMax, yMax
}
