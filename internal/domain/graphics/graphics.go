// Package graphics provides graphics state and operations for PDF rendering.
//
//revive:disable:exported
package graphics

import (
	"fmt"
	"image/color"
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// State represents the graphics state.
type State struct {
	font              entity.Font
	fillColor         color.Color
	strokeColor       color.Color
	parent            *State
	currentPath       *Path
	clipPath          *Path
	dashArray         []float64
	textMatrix        [6]float64
	ctm               [6]float64
	fontSize          float64
	dashPhase         float64
	miterLimit        float64
	textLeading       float64
	textRise          float64
	charSpacing       float64
	wordSpacing       float64
	horizontalScaling float64
	textRenderMode    int
	lineJoin          int
	lineCap           int
	lineWidth         float64
}

// NewState creates a new graphics state with default values.
func NewState() *State {
	return &State{
		ctm:               [6]float64{1, 0, 0, 1, 0, 0},
		fillColor:         color.Black,
		strokeColor:       color.Black,
		lineWidth:         1.0,
		lineCap:           0,
		lineJoin:          0,
		miterLimit:        10.0,
		dashArray:         nil,
		dashPhase:         0,
		fontSize:          12.0,
		textMatrix:        [6]float64{1, 0, 0, 1, 0, 0},
		textLeading:       0.0,
		textRise:          0.0,
		charSpacing:       0.0,
		wordSpacing:       0.0,
		horizontalScaling: 100.0,
		textRenderMode:    0,
		currentPath:       NewPath(),
	}
}

// Save creates a copy of the current state and pushes it onto the stack.
func (s *State) Save() *State {
	newState := *s
	newState.parent = s
	newState.currentPath = NewPath()
	if s.clipPath != nil {
		newState.clipPath = s.clipPath.Clone()
	}
	return &newState
}

// Restore restores the previous state from the stack.
func (s *State) Restore() *State {
	if s.parent != nil {
		return s.parent
	}
	return s
}

// GetCTM returns the current transformation matrix.
func (s *State) GetCTM() [6]float64 {
	return s.ctm
}

// SetCTM sets the current transformation matrix.
func (s *State) SetCTM(ctm [6]float64) {
	s.ctm = ctm
}

// Transform applies a transformation matrix to the CTM.
func (s *State) Transform(m [6]float64) {
	// Matrix multiplication: newCTM = m * ctm
	// PDF uses column-major order for matrices
	newCTM := [6]float64{
		m[0]*s.ctm[0] + m[2]*s.ctm[1],
		m[1]*s.ctm[0] + m[3]*s.ctm[1],
		m[0]*s.ctm[2] + m[2]*s.ctm[3],
		m[1]*s.ctm[2] + m[3]*s.ctm[3],
		m[0]*s.ctm[4] + m[2]*s.ctm[5] + m[4], // Translation x
		m[1]*s.ctm[4] + m[3]*s.ctm[5] + m[5], // Translation y
	}
	s.ctm = newCTM
}

// Translate translates the coordinate system.
func (s *State) Translate(tx, ty float64) {
	m := [6]float64{1, 0, 0, 1, tx, ty}
	s.Transform(m)
}

// Scale scales the coordinate system.
func (s *State) Scale(sx, sy float64) {
	m := [6]float64{sx, 0, 0, sy, 0, 0}
	s.Transform(m)
}

// Rotate rotates the coordinate system.
func (s *State) Rotate(angle float64) {
	rad := angle * math.Pi / 180.0
	cos := math.Cos(rad)
	sin := math.Sin(rad)
	m := [6]float64{cos, sin, -sin, cos, 0, 0}
	s.Transform(m)
}

// GetFillColor returns the fill color.
func (s *State) GetFillColor() color.Color {
	return s.fillColor
}

// SetFillColor sets the fill color.
func (s *State) SetFillColor(c color.Color) {
	s.fillColor = c
}

// GetStrokeColor returns the stroke color.
func (s *State) GetStrokeColor() color.Color {
	return s.strokeColor
}

// SetStrokeColor sets the stroke color.
func (s *State) SetStrokeColor(c color.Color) {
	s.strokeColor = c
}

// GetLineWidth returns the line width.
func (s *State) GetLineWidth() float64 {
	return s.lineWidth
}

// SetLineWidth sets the line width.
func (s *State) SetLineWidth(width float64) {
	s.lineWidth = width
}

// GetLineCap returns the line cap style.
func (s *State) GetLineCap() int {
	return s.lineCap
}

// SetLineCap sets the line cap style (0=butt, 1=round, 2=square).
func (s *State) SetLineCap(cap int) {
	s.lineCap = cap
}

// GetLineJoin returns the line join style.
func (s *State) GetLineJoin() int {
	return s.lineJoin
}

// SetLineJoin sets the line join style (0=miter, 1=round, 2=bevel).
func (s *State) SetLineJoin(join int) {
	s.lineJoin = join
}

// GetMiterLimit returns the miter limit.
func (s *State) GetMiterLimit() float64 {
	return s.miterLimit
}

// SetMiterLimit sets the miter limit.
func (s *State) SetMiterLimit(limit float64) {
	// PDF spec requires miter limit to be >= 1
	if limit < 1 {
		limit = 1
	}
	s.miterLimit = limit
}

// GetDashArray returns the dash pattern.
func (s *State) GetDashArray() []float64 {
	return s.dashArray
}

// SetDashArray sets the dash pattern.
func (s *State) SetDashArray(dash []float64, phase float64) {
	s.dashArray = dash
	s.dashPhase = phase
}

// GetDashPhase returns the dash phase.
func (s *State) GetDashPhase() float64 {
	return s.dashPhase
}

// GetFont returns the current font.
func (s *State) GetFont() entity.Font {
	return s.font
}

// SetFont sets the current font.
func (s *State) SetFont(font entity.Font) {
	s.font = font
}

// GetFontSize returns the font size.
func (s *State) GetFontSize() float64 {
	return s.fontSize
}

// SetFontSize sets the font size.
func (s *State) SetFontSize(size float64) {
	s.fontSize = size
}

// GetTextMatrix returns the text matrix.
func (s *State) GetTextMatrix() [6]float64 {
	return s.textMatrix
}

// SetTextMatrix sets the text matrix.
func (s *State) SetTextMatrix(tm [6]float64) {
	s.textMatrix = tm
}

// GetTextLeading returns the text leading.
func (s *State) GetTextLeading() float64 {
	return s.textLeading
}

// SetTextLeading sets the text leading.
func (s *State) SetTextLeading(leading float64) {
	s.textLeading = leading
}

// GetCharSpacing returns the character spacing.
func (s *State) GetCharSpacing() float64 {
	return s.charSpacing
}

// SetCharSpacing sets the character spacing.
func (s *State) SetCharSpacing(spacing float64) {
	s.charSpacing = spacing
}

// GetWordSpacing returns the word spacing.
func (s *State) GetWordSpacing() float64 {
	return s.wordSpacing
}

// SetWordSpacing sets the word spacing.
func (s *State) SetWordSpacing(spacing float64) {
	s.wordSpacing = spacing
}

// GetTextRenderMode returns the text rendering mode.
func (s *State) GetTextRenderMode() int {
	return s.textRenderMode
}

// SetTextRenderMode sets the text rendering mode.
func (s *State) SetTextRenderMode(mode int) {
	s.textRenderMode = mode
}

// GetTextRise returns the text rise.
func (s *State) GetTextRise() float64 {
	return s.textRise
}

// SetTextRise sets the text rise.
func (s *State) SetTextRise(rise float64) {
	s.textRise = rise
}

// GetHorizontalScaling returns the horizontal scaling percentage.
func (s *State) GetHorizontalScaling() float64 {
	return s.horizontalScaling
}

// SetHorizontalScaling sets the horizontal scaling percentage.
func (s *State) SetHorizontalScaling(scaling float64) {
	s.horizontalScaling = scaling
}

// GetCurrentPath returns the current path.
func (s *State) GetCurrentPath() *Path {
	return s.currentPath
}

// SetCurrentPath sets the current path.
func (s *State) SetCurrentPath(path *Path) {
	s.currentPath = path
}

// GetClipPath returns the clipping path.
func (s *State) GetClipPath() *Path {
	return s.clipPath
}

// SetClipPath sets the clipping path.
func (s *State) SetClipPath(path *Path) {
	s.clipPath = path
}

// Path represents a graphics path.
type Path struct {
	commands []PathCommand
	fillRule FillRule
}

// NewPath creates a new empty path.
func NewPath() *Path {
	return &Path{
		commands: make([]PathCommand, 0),
		fillRule: FillRuleNonZero,
	}
}

// Clone creates a copy of the path.
func (p *Path) Clone() *Path {
	newPath := &Path{
		fillRule: p.fillRule,
	}
	if p.commands != nil {
		newPath.commands = make([]PathCommand, len(p.commands))
		copy(newPath.commands, p.commands)
	}
	return newPath
}

// PathCommand represents a path command.
type PathCommand interface {
	Execute(state *State) (float64, float64) // Returns (x, y) current position
}

// PathCommandType represents the type of path command.
type PathCommandType int

const (
	PathMoveTo PathCommandType = iota
	PathLineTo
	PathCurveTo
	PathClosePath
)

// FillRule represents the fill rule for paths.
type FillRule int

const (
	FillRuleNonZero FillRule = iota
	FillRuleEvenOdd
)

// Path commands implementation

// MoveTo moves the current position.
type MoveTo struct {
	X, Y float64
}

// Execute executes the operation.
func (m *MoveTo) Execute(state *State) (float64, float64) {
	return m.X, m.Y
}

// LineTo draws a line to the specified position.
type LineTo struct {
	X, Y float64
}

// Execute executes the operation.
func (l *LineTo) Execute(state *State) (float64, float64) {
	return l.X, l.Y
}

// CurveTo draws a cubic Bézier curve.
type CurveTo struct {
	X1, Y1, X2, Y2, X3, Y3 float64
}

// Execute executes the operation.
func (c *CurveTo) Execute(state *State) (float64, float64) {
	return c.X3, c.Y3
}

// ClosePath closes the current subpath.
type ClosePath struct{}

// Execute executes the operation.
func (c *ClosePath) Execute(state *State) (float64, float64) {
	// Return to the start of the current subpath
	// For now, just return current position
	return 0, 0
}

// Rectangle creates a rectangular path.
func Rectangle(x, y, width, height float64) *Path {
	path := NewPath()
	path.AddCommand(&MoveTo{X: x, Y: y})
	path.AddCommand(&LineTo{X: x + width, Y: y})
	path.AddCommand(&LineTo{X: x + width, Y: y + height})
	path.AddCommand(&LineTo{X: x, Y: y + height})
	path.AddCommand(&ClosePath{})
	return path
}

// AddCommand adds a command to the path.
func (p *Path) AddCommand(cmd PathCommand) {
	p.commands = append(p.commands, cmd)
}

// GetCommands returns all commands in the path.
func (p *Path) GetCommands() []PathCommand {
	return p.commands
}

// SetFillRule sets the fill rule.
func (p *Path) SetFillRule(rule FillRule) {
	p.fillRule = rule
}

// GetFillRule returns the fill rule.
func (p *Path) GetFillRule() FillRule {
	return p.fillRule
}

// GetBounds returns the bounding box of the path.
func (p *Path) GetBounds() (xMin, yMin, xMax, yMax float64) {
	if len(p.commands) == 0 {
		return 0, 0, 0, 0
	}

	xMin, yMin = 1e100, 1e100
	xMax, yMax = -1e100, -1e100

	// Simple bounding box computation
	for _, cmd := range p.commands {
		switch c := cmd.(type) {
		case *MoveTo:
			if c.X < xMin {
				xMin = c.X
			}
			if c.X > xMax {
				xMax = c.X
			}
			if c.Y < yMin {
				yMin = c.Y
			}
			if c.Y > yMax {
				yMax = c.Y
			}
		case *LineTo:
			if c.X < xMin {
				xMin = c.X
			}
			if c.X > xMax {
				xMax = c.X
			}
			if c.Y < yMin {
				yMin = c.Y
			}
			if c.Y > yMax {
				yMax = c.Y
			}
		case *CurveTo:
			// For curves, consider all control points
			points := []float64{c.X1, c.Y1, c.X2, c.Y2, c.X3, c.Y3}
			for i := 0; i < len(points); i += 2 {
				if points[i] < xMin {
					xMin = points[i]
				}
				if points[i] > xMax {
					xMax = points[i]
				}
				if points[i+1] < yMin {
					yMin = points[i+1]
				}
				if points[i+1] > yMax {
					yMax = points[i+1]
				}
			}
		}
	}

	return xMin, yMin, xMax, yMax
}

// Matrix operations for transformations.

// IdentityMatrix returns the identity matrix.
func IdentityMatrix() [6]float64 {
	return [6]float64{1, 0, 0, 1, 0, 0}
}

// MultiplyMatrix multiplies two matrices.
func MultiplyMatrix(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[2]*b[1],
		a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3],
		a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4],
		a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

// InverseMatrix inverts a matrix.
func InverseMatrix(m [6]float64) ([6]float64, error) {
	det := m[0]*m[3] - m[1]*m[2]
	if det == 0 {
		return [6]float64{}, fmt.Errorf("singular matrix")
	}

	invDet := 1.0 / det
	return [6]float64{
		m[3] * invDet,
		-m[1] * invDet,
		-m[2] * invDet,
		m[0] * invDet,
		(m[2]*m[5] - m[3]*m[4]) * invDet,
		(m[1]*m[4] - m[0]*m[5]) * invDet,
	}, nil
}

// TransformPoint applies a matrix to a point.
func TransformPoint(m [6]float64, x, y float64) (tx, ty float64) {
	tx = m[0]*x + m[2]*y + m[4]
	ty = m[1]*x + m[3]*y + m[5]
	return
}
