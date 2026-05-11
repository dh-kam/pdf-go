package canvas_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
)

func TestImageCanvas_NewCanvas(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	c := canvas.NewImageCanvas(bounds)

	assert.NotNil(t, c)
	assert.Equal(t, 100, c.Width())
	assert.Equal(t, 100, c.Height())
	assert.Equal(t, bounds, c.Bounds())
}

func TestImageCanvas_Properties(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))

	c.SetFillColor(color.RGBA{255, 0, 0, 255})
	c.SetStrokeColor(color.RGBA{0, 255, 0, 255})
	c.SetLineWidth(2.5)
	c.SetLineCap(2)
	c.SetLineJoin(2)
	c.SetMiterLimit(5.0)
	c.SetDashPattern([]float64{5, 3}, 1.0)
	c.SetDashPattern(nil, 0)
}

func TestImageCanvas_TransformAndState(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.Transform([6]float64{1, 0, 0, 1, 10, 20})
	c.Transform([6]float64{2, 0, 0, 2, 0, 0})

	c.Save()
	c.SetLineWidth(5)
	c.Restore()
	c.Restore() // safe no-op
}

func TestImageCanvas_PathOperations(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.MoveTo(10, 20)
	c.LineTo(30, 40)
	c.CurveTo(50, 60, 70, 80, 90, 100)
	c.ClosePath()
	c.Rectangle(5, 5, 50, 50)
	c.Fill()
	c.Stroke()
	c.Reset()
}

func TestImageCanvas_FillEvenOdd(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.SetFillColor(color.RGBA{255, 0, 0, 255})
	c.Rectangle(10, 10, 80, 80)
	c.Rectangle(30, 30, 40, 40)
	c.Fill()

	img, ok := c.Image().(*image.RGBA)
	require.True(t, ok)
	assert.True(t, img.RGBAAt(20, 20).A > 0)
	assert.True(t, img.RGBAAt(50, 50).A > 0)
}

func TestImageCanvas_Clip(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.Rectangle(10, 10, 80, 80)
	c.Clip()
	c.Rectangle(15, 15, 70, 70)
	c.EoClip()
}

func TestImageCanvas_ClipIntersectsExistingMask(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.SetFillColor(color.RGBA{255, 0, 0, 255})

	c.Rectangle(10, 10, 80, 80)
	c.Clip()
	c.Rectangle(30, 30, 20, 20)
	c.Clip()
	c.Rectangle(0, 0, 100, 100)
	c.Fill()

	img, ok := c.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{255, 0, 0, 255}, img.RGBAAt(35, 60))
	assert.Equal(t, color.RGBA{}, img.RGBAAt(20, 20))
	assert.Equal(t, color.RGBA{}, img.RGBAAt(60, 60))
}

func TestImageCanvas_CurveClipDoesNotInvertMask(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.SetFillColor(color.RGBA{255, 0, 0, 255})

	c.MoveTo(20, 70)
	c.CurveTo(35, 90, 65, 90, 80, 70)
	c.CurveTo(90, 55, 90, 35, 80, 20)
	c.CurveTo(65, 5, 35, 5, 20, 20)
	c.CurveTo(10, 35, 10, 55, 20, 70)
	c.Clip()
	c.Rectangle(0, 0, 100, 100)
	c.Fill()

	img, ok := c.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{255, 0, 0, 255}, img.RGBAAt(50, 50))
	assert.Equal(t, color.RGBA{}, img.RGBAAt(5, 5))
}

func TestImageCanvas_DrawImage(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	img.Set(10, 10, color.RGBA{255, 0, 0, 255})

	err := c.DrawImage(img, 10, 10, 20, 20, false)
	require.NoError(t, err)
	c.Rectangle(10, 10, 10, 10)
	c.Clip()
	err = c.DrawImage(img, 5, 5, 10, 10, false)
	require.NoError(t, err)
}

func TestImageCanvas_TextOperations(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.BeginText(10, 20)
	c.MoveTextPoint(5, 5)
	require.NoError(t, c.ShowText("Hello"))
	c.EndText()
	err := c.DrawText("World", 10, 30, nil, 12)
	require.NoError(t, err)
}

func TestImageCanvas_DrawTextCIDFont(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	font := &mockCanvasCIDFont{}
	err := c.DrawText(string([]byte{0x4E, 0x00, 0x4E, 0x8C}), 10, 30, font, 12)
	require.NoError(t, err)
	assert.Equal(t, []uint32{0x4E00, 0x4E8C}, font.charCodes)
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
		return 0, assert.AnError
	}
}

func (m *mockCanvasCIDFont) GlyphName(glyph uint32) string               { return "" }
func (m *mockCanvasCIDFont) GetGlyphWidth(glyph uint32) (float64, error) { return 1000, nil }
func (m *mockCanvasCIDFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}
func (m *mockCanvasCIDFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, nil
}
func (m *mockCanvasCIDFont) IsCIDFont() bool    { return true }
func (m *mockCanvasCIDFont) IsSymbolic() bool   { return false }
func (m *mockCanvasCIDFont) UnitsPerEm() uint16 { return 1000 }
func (m *mockCanvasCIDFont) Name() string       { return "Mock" }

func TestImageCanvas_ResetAndEmptyPath(t *testing.T) {
	c := canvas.NewImageCanvas(image.Rect(0, 0, 100, 100))
	c.Fill()
	c.Stroke()
	c.Reset()

	c.SetFillColor(color.RGBA{0, 255, 0, 255})
	c.MoveTo(10, 10)
}
