// Package canvas provides canvas interfaces for PDF rendering.
package canvas

import (
	"image"
	"image/color"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// Canvas represents a drawing surface for PDF rendering.
type Canvas interface {
	// Size operations
	Width() int
	Height() int
	Bounds() image.Rectangle

	// Drawing operations
	MoveTo(x, y float64)
	LineTo(x, y float64)
	CurveTo(c1x, c1y, c2x, c2y, x, y float64)
	Rectangle(x, y, width, height float64)
	ClosePath()

	// Path operations
	Fill()
	Stroke()
	Clip()
	EoClip()

	// Text operations
	DrawText(text string, x, y float64, font entity.Font, fontSize float64) error
	BeginText(x, y float64)
	EndText()
	ShowText(text string) error
	MoveTextPoint(tx, ty float64)

	// Image operations
	DrawImage(img image.Image, x, y, width, height float64, interpolate bool) error

	// State operations
	Save()
	Restore()
	Transform(matrix [6]float64)

	// Properties
	SetFillColor(c color.Color)
	SetStrokeColor(c color.Color)
	SetLineWidth(width float64)
	SetLineCap(cap int)
	SetLineJoin(join int)
	SetMiterLimit(limit float64)
	SetDashPattern(dash []float64, phase float64)

	// Pattern operations
	SetFillPattern(pattern entity.Pattern)
	SetStrokePattern(pattern entity.Pattern)
	DrawTilingPattern(pattern *entity.TilingPattern, bbox [4]float64) error
	DrawShadingPattern(pattern *entity.ShadingPattern, bbox [4]float64) error

	// Rendering
	Image() image.Image
	Reset()
}
