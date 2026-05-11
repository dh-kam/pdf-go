package canvas_test

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	infrastructureCanvas "github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
)

func TestImageCanvas_NewCanvas(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	c := infrastructureCanvas.NewImageCanvas(bounds)

	assert.NotNil(t, c)
	assert.Equal(t, 100, c.Width())
	assert.Equal(t, 100, c.Height())
	assert.Equal(t, bounds, c.Bounds())
}

func TestImageCanvas_Properties(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Test fill color
	c.SetFillColor(color.RGBA{255, 0, 0, 255})

	// Test stroke color
	c.SetStrokeColor(color.RGBA{0, 255, 0, 255})

	// Test line width
	c.SetLineWidth(2.5)

	// Test line cap
	c.SetLineCap(1)
	c.SetLineCap(2)
	c.SetLineCap(0)

	// Test line join
	c.SetLineJoin(1)
	c.SetLineJoin(2)
	c.SetLineJoin(0)

	// Test miter limit
	c.SetMiterLimit(5.0)

	// Test dash pattern
	c.SetDashPattern([]float64{5, 3}, 1.0)
	c.SetDashPattern(nil, 0)
}

func TestImageCanvas_Transform(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Test translation
	c.Transform([6]float64{1, 0, 0, 1, 10, 20})

	// Test scaling
	c.Transform([6]float64{2, 0, 0, 2, 0, 0})

	// Test identity (no-op)
	c.Transform([6]float64{1, 0, 0, 1, 0, 0})
}

func TestImageCanvas_SaveRestore(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Set some properties
	c.SetFillColor(color.RGBA{255, 0, 0, 255})
	c.SetLineWidth(5.0)

	// Save state
	c.Save()

	// Change properties
	c.SetFillColor(color.RGBA{0, 255, 0, 255})
	c.SetLineWidth(10.0)

	// Restore state
	c.Restore()

	// Properties should be restored
	// Note: We can't directly access the internal state, but the
	// Restore operation should have reset them
}

func TestImageCanvas_SaveRestoreEmptyStack(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Restore without save should not panic
	c.Restore()
	c.Restore()
}

func TestImageCanvas_StateStackMultiple(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Push multiple states
	c.SetFillColor(color.RGBA{255, 0, 0, 255})
	c.Save()

	c.SetFillColor(color.RGBA{0, 255, 0, 255})
	c.Save()

	c.SetFillColor(color.RGBA{0, 0, 255, 255})
	c.Save()

	// Pop states
	c.Restore() // Should go back to green
	c.Restore() // Should go back to red
	c.Restore() // Should go back to initial (black)
}

func TestImageCanvas_PathOperations(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Test MoveTo
	c.MoveTo(10, 20)
	c.LineTo(30, 40)
	c.CurveTo(50, 60, 70, 80, 90, 100)
	c.ClosePath()
	c.Rectangle(5, 5, 50, 50)

	// Test Fill and Stroke (should not panic)
	c.Fill()
	c.Stroke()

	// Reset path
	c.Reset()
}

func TestImageCanvas_FillEvenOdd(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	evenOddCanvas, ok := c.(interface{ FillEvenOdd() })
	require.True(t, ok)

	c.SetFillColor(color.RGBA{255, 0, 0, 255})
	c.Rectangle(10, 10, 80, 80)
	c.Rectangle(30, 30, 40, 40)
	evenOddCanvas.FillEvenOdd()

	img, ok := c.Image().(*image.RGBA)
	require.True(t, ok)

	outer := img.RGBAAt(20, 20)
	center := img.RGBAAt(50, 50)
	assert.True(t, outer.A > 0)
	assert.Equal(t, uint8(0), center.A)
}

func TestImageCanvas_Clip(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Create a path for clipping
	c.Rectangle(10, 10, 80, 80)

	// Set as clipping path
	c.Clip()

	// Even-odd clipping
	c.Rectangle(15, 15, 70, 70)
	c.EoClip()
}

func TestImageCanvas_TextOperations(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Test text block
	c.BeginText(10, 20)
	c.MoveTextPoint(5, 5)
	c.ShowText("Hello")
	c.EndText()

	// Test direct text drawing
	err := c.DrawText("World", 10, 30, nil, 12)
	// Should not error even with nil font (placeholder implementation)
	require.NoError(t, err)
}

func TestImageCanvas_DrawText_CIDCodeUnits(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	font := &mockCanvasCIDFont{}

	err := c.DrawText(string([]byte{0x4E, 0x00, 0x4E, 0x8C}), 10, 30, font, 12)
	require.NoError(t, err)
	assert.Equal(t, []uint32{0x4E00, 0x4E8C}, font.charCodes)
}

func TestImageCanvas_ImageOperations(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Create a simple test image
	testImg := image.NewRGBA(image.Rect(0, 0, 50, 50))
	testImg.Set(25, 25, color.RGBA{255, 0, 0, 255})

	// Draw image (should not panic)
	err := c.DrawImage(testImg, 10, 10, 50, 50, false)
	require.NoError(t, err)

	// Draw with transform
	c.Save()
	c.Transform([6]float64{1, 0, 0, 1, 20, 20})
	err = c.DrawImage(testImg, 0, 0, 25, 25, false)
	require.NoError(t, err)
	c.Restore()
}

type mockCanvasCIDFont struct {
	charCodes []uint32
}

func (m *mockCanvasCIDFont) CharCodeToGlyph(code uint32) (uint32, error) {
	m.charCodes = append(m.charCodes, code)
	switch code {
	case 0x4E00:
		return 1, nil
	case 0x4E8C:
		return 2, nil
	default:
		return 0, fmt.Errorf("unknown char code: %x", code)
	}
}

func (m *mockCanvasCIDFont) GlyphName(glyph uint32) string { return "" }

func (m *mockCanvasCIDFont) GetGlyphWidth(glyph uint32) (float64, error) {
	return 1000, nil
}

func (m *mockCanvasCIDFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}

func (m *mockCanvasCIDFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCanvasCIDFont) IsCIDFont() bool { return true }

func (m *mockCanvasCIDFont) IsSymbolic() bool { return false }

func (m *mockCanvasCIDFont) UnitsPerEm() uint16 { return 1000 }

func (m *mockCanvasCIDFont) Name() string { return "MockCanvasCIDFont" }

func TestImageCanvas_Reset(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Set some properties and create state
	c.SetFillColor(color.RGBA{255, 0, 0, 255})
	c.MoveTo(10, 20)
	c.Save()

	// Reset
	c.Reset()

	// After reset, state should be cleared
	// Subsequent operations should work
	c.SetFillColor(color.RGBA{0, 255, 0, 255})
	c.MoveTo(30, 40)
}

func TestImageCanvas_Image(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	img := c.Image()
	assert.NotNil(t, img)

	// Check it's the expected type
	_, ok := img.(*image.RGBA)
	assert.True(t, ok, "Image should be *image.RGBA")
}

func TestImageCanvas_EmptyPath(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Fill/stroke on empty path should not panic
	c.Fill()
	c.Stroke()
}

func TestImageCanvas_TransformPoint(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Apply a translation
	c.Transform([6]float64{1, 0, 0, 1, 10, 20})

	// Draw something - the transform should affect where it's drawn
	c.MoveTo(0, 0)
	c.LineTo(10, 10)
	c.Stroke()
}

func TestImageCanvas_DashPattern(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Set dash pattern
	c.SetDashPattern([]float64{5, 3, 2, 3}, 1.0)

	// Clear dash pattern
	c.SetDashPattern(nil, 0)
	c.SetDashPattern([]float64{}, 0)
}

func TestImageCanvas_MultipleTransforms(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	// Apply multiple transforms in sequence
	c.Transform([6]float64{1, 0, 0, 1, 10, 0}) // Translate x by 10
	c.Transform([6]float64{1, 0, 0, 1, 0, 20}) // Translate y by 20
	c.Transform([6]float64{2, 0, 0, 2, 0, 0})  // Scale by 2

	// The accumulated transform should be:
	// Scale(2) * Translate(0, 20) * Translate(10, 0)
	// Result: translate should be (20, 40) and scale (2, 2)
}

func TestImageCanvas_NestedSaveRestore(t *testing.T) {
	c := infrastructureCanvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	c.SetFillColor(color.RGBA{100, 100, 100, 255})

	c.Save()
	c.SetFillColor(color.RGBA{200, 100, 100, 255})

	c.Save()
	c.SetFillColor(color.RGBA{100, 200, 100, 255})

	// Nested restore
	c.Restore() // Back to (200, 100, 100)
	c.Restore() // Back to (100, 100, 100)

	// Should not error
	c.Restore() // No-op (stack empty)
}
