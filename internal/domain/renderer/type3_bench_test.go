package renderer

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// ---------------------------------------------------------------------------
// Type3 font rendering benchmarks
// ---------------------------------------------------------------------------

// BenchmarkType3GlyphRender benchmarks rendering a single Type3 glyph.
func BenchmarkType3GlyphRender(b *testing.B) {
	eval, font := newType3BenchEvaluator(b)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = eval.renderType3Glyph(font, 65, 72, 144, 48)
	}
}

// BenchmarkType3GlyphRenderParallel benchmarks rendering many glyphs sequentially.
func BenchmarkType3GlyphRenderParallel(b *testing.B) {
	eval, font := newType3BenchEvaluator(b)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for code := 65; code <= 90; code++ {
				_ = eval.renderType3Glyph(font, uint32(code), 72, 144, 48)
		}
	}
}

// BenchmarkType3FontResolution benchmarks resolving a Type3 font from a dictionary.
func BenchmarkType3FontResolution(b *testing.B) {
	eval := NewEvaluator(nil)
	fontDict := newType3BenchFontDict()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		font, _ := eval.getFontFromDict(fontDict, "TestType3Font")
		_ = font
	}
}

// BenchmarkType3CharProcEvaluation benchmarks parsing and executing a charproc content stream.
func BenchmarkType3CharProcEvaluation(b *testing.B) {
	eval, font := newType3BenchEvaluator(b)
	b.ResetTimer()
	b.ReportAllocs()

	charProc := font.CharProcForCode(65)
	require.NotNil(b, charProc)

	data := charProc.RawBytes()
	ops, err := eval.parseOperatorsOnly(data)
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		eval.executeCachedOperators(ops)
	}
}

// ---------------------------------------------------------------------------
// Type3 unit tests: renderType3Glyph coverage
// ---------------------------------------------------------------------------

func TestRenderType3Glyph_RendersCharProcContent(t *testing.T) {
	eval, font := newType3BenchEvaluator(t)
	eval.SetCanvas(newType3BenchCanvas())
	eval.SetFillColor(color.Black)
	eval.SetStrokeColor(color.Black)

	err := eval.renderType3Glyph(font, 65, 72, 144, 48)
	require.NoError(t, err)
}

func TestRenderType3Glyph_MissingCharProc(t *testing.T) {
	eval, font := newType3BenchEvaluator(t)
	eval.SetCanvas(newType3BenchCanvas())

	// charCode 99 ('c') is outside encoding range [65,90]
	err := eval.renderType3Glyph(font, 99, 72, 144, 48)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no charproc")
}

func TestRenderType3Glyph_NilCanvas(t *testing.T) {
	eval, font := newType3BenchEvaluator(t)

	// canvas is nil - renderType3Glyph should still evaluate charproc
	err := eval.renderType3Glyph(font, 65, 72, 144, 48)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Helper types and functions
// ---------------------------------------------------------------------------

// type3BenchCanvas is a minimal canvas implementation for benchmarking.
type type3BenchCanvas struct {
	path   []interface{}
	fill   color.Color
	stroke color.Color
	bounds image.Rectangle
}

func newType3BenchCanvas() *type3BenchCanvas {
	return &type3BenchCanvas{
		bounds: image.Rect(0, 0, 612, 792),
	}
}

func (c *type3BenchCanvas) Save()                         {}
func (c *type3BenchCanvas) Restore()                      {}
func (c *type3BenchCanvas) Bounds() image.Rectangle       { return c.bounds }
func (c *type3BenchCanvas) Width() int                    { return c.bounds.Dx() }
func (c *type3BenchCanvas) Height() int                   { return c.bounds.Dy() }
func (c *type3BenchCanvas) SetFillColor(clr color.Color)   { c.fill = clr }
func (c *type3BenchCanvas) SetStrokeColor(clr color.Color) { c.stroke = clr }
func (c *type3BenchCanvas) SetFillPattern(_ entity.Pattern)   {}
func (c *type3BenchCanvas) SetStrokePattern(_ entity.Pattern) {}
func (c *type3BenchCanvas) SetLineWidth(_ float64)             {}
func (c *type3BenchCanvas) SetLineCap(_ int)                 {}
func (c *type3BenchCanvas) SetLineJoin(_ int)                {}
func (c *type3BenchCanvas) SetMiterLimit(_ float64)           {}
func (c *type3BenchCanvas) SetDashPattern(_ []float64, _ float64) {}

func (c *type3BenchCanvas) MoveTo(x, y float64) {
	c.path = append(c.path, &MoveTo{X: x, Y: y})
}
func (c *type3BenchCanvas) LineTo(x, y float64) {
	c.path = append(c.path, &LineTo{X: x, Y: y})
}
func (c *type3BenchCanvas) CurveTo(x1, y1, x2, y2, x, y float64) {
	c.path = append(c.path, &CurveTo{X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
}
func (c *type3BenchCanvas) ClosePath() {
	c.path = append(c.path, &Close{})
}
func (c *type3BenchCanvas) Fill()          { c.path = nil }
func (c *type3BenchCanvas) Stroke()        { c.path = nil }
func (c *type3BenchCanvas) Clip()          {}
func (c *type3BenchCanvas) EoClip()        {}
func (c *type3BenchCanvas) FillEvenOdd()   {}
func (c *type3BenchCanvas) FillAndStroke() {}
func (c *type3BenchCanvas) FillEvenOddAndStroke() {}
func (c *type3BenchCanvas) Rectangle(_, _, _, _ float64) {}
func (c *type3BenchCanvas) Transform(_ [6]float64) {}
func (c *type3BenchCanvas) DrawImage(_ image.Image, _, _, _, _ float64, _ bool) error {
	return nil
}
func (c *type3BenchCanvas) DrawText(_ string, _, _ float64, _ entity.Font, _ float64) error {
	return nil
}
func (c *type3BenchCanvas) BeginText(_, _ float64) {}
func (c *type3BenchCanvas) EndText()                  {}
func (c *type3BenchCanvas) ShowText(_ string) error    { return nil }
func (c *type3BenchCanvas) MoveTextPoint(_, _ float64) {}
func (c *type3BenchCanvas) DrawShadingPattern(_ *entity.ShadingPattern, _ [4]float64) error {
	return nil
}
func (c *type3BenchCanvas) DrawTilingPattern(_ *entity.TilingPattern, _ [4]float64) error {
	return nil
}
func (c *type3BenchCanvas) Image() image.Image { return nil }
func (c *type3BenchCanvas) Reset() {}

// newType3BenchEvaluator creates an evaluator with a Type3 font for benchmarking.
func newType3BenchEvaluator(t testing.TB) (*Evaluator, *entity.Type3Font) {
	eval := NewEvaluator(nil)

	charProcs := make(map[string]*entity.Stream)
	encoding := make(map[uint32]string)
	widths := make(map[uint32]float64)

	for code := 65; code <= 90; code++ {
		glyphName := string(rune(code))
		charProcData := []byte(fmt.Sprintf("1000 0 d0\n0 0 1000 1000 re\nf\n"))
		charProcs[glyphName] = entity.NewStream(entity.NewDict(), charProcData)
		encoding[uint32(code)] = glyphName
		widths[uint32(code)] = 1000
	}

	font := entity.NewType3Font("TestType3Font",
		[6]float64{0.001, 0, 0, 0.001, 0, 0},
		charProcs,
		encoding,
		widths,
		65, 90,
		[4]float64{0, 0, 1000, 1000},
	)

	require.NotNil(t, font)
	return eval, font
}

// newType3BenchFontDict returns a dictionary mimicking a Type3 font resource.
func newType3BenchFontDict() *entity.Dict {
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type3"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("TestType3Font"))
	fontDict.Set(entity.Name("FontBBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(1),
		entity.NewInteger(1000), entity.NewInteger(1000),
	))

	charProcs := entity.NewDict()
	encoding := entity.NewDict()
	encoding.Set(entity.Name("Differences"), entity.NewArray(
		entity.NewInteger(65), entity.Name("A"),
	))

	charProcData := []byte("1000 0 d0\n0 0 1000 1000 re\nf\n")
	charProcs.Set(entity.Name("A"), entity.NewStream(entity.NewDict(), charProcData))

	fontDict.Set(entity.Name("CharProcs"), charProcs)
	fontDict.Set(entity.Name("Encoding"), encoding)
	fontDict.Set(entity.Name("FontMatrix"), entity.NewArray(
		entity.NewReal(0.001), entity.NewInteger(0), entity.NewInteger(0),
		entity.NewReal(0.001), entity.NewInteger(0), entity.NewInteger(0),
	))
	fontDict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
	fontDict.Set(entity.Name("LastChar"), entity.NewInteger(65))
	fontDict.Set(entity.Name("Widths"), entity.NewArray(entity.NewInteger(1000)))

	return fontDict
}
