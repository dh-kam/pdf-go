package content

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
)

type mockEvaluatorXRef struct {
	objects map[entity.Ref]entity.Object
}

func (m *mockEvaluatorXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	if m.objects == nil {
		return nil, nil
	}
	return m.objects[ref], nil
}

type mockTextFont struct{}

func (m *mockTextFont) CharCodeToGlyph(code uint32) (uint32, error) { return code, nil }

func (m *mockTextFont) GlyphName(glyph uint32) string {
	switch glyph {
	case 65:
		return "A"
	case 66:
		return "B"
	default:
		return ".notdef"
	}
}

func (m *mockTextFont) GetGlyphWidth(glyph uint32) (float64, error) { return 500, nil }

func (m *mockTextFont) GetBoundingBox() (float64, float64, float64, float64) { return 0, 0, 500, 500 }

func (m *mockTextFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return &entity.GlyphPath{}, nil
}

func (m *mockTextFont) IsCIDFont() bool { return false }

func (m *mockTextFont) IsSymbolic() bool { return false }

func (m *mockTextFont) UnitsPerEm() uint16 { return 1000 }

func (m *mockTextFont) Name() string { return "MockTextFont" }

type mockCIDTextFont struct{}

func (m *mockCIDTextFont) CharCodeToGlyph(code uint32) (uint32, error) { return code, nil }

func (m *mockCIDTextFont) GlyphName(glyph uint32) string {
	switch glyph {
	case 0x4E00:
		return "uni4E00"
	default:
		return ""
	}
}

func (m *mockCIDTextFont) GetGlyphWidth(glyph uint32) (float64, error) { return 500, nil }

func (m *mockCIDTextFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 500, 500
}

func (m *mockCIDTextFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return &entity.GlyphPath{}, nil
}

func (m *mockCIDTextFont) IsCIDFont() bool { return true }

func (m *mockCIDTextFont) IsSymbolic() bool { return false }

func (m *mockCIDTextFont) UnitsPerEm() uint16 { return 1000 }

func (m *mockCIDTextFont) Name() string { return "MockCIDTextFont" }

func TestTextEvaluator_ProcessObject_ResolvesRef(t *testing.T) {
	ref := entity.NewRef(10, 0)
	streamObj := entity.NewStream(entity.NewDict(), []byte("BT ET"))
	xref := &mockEvaluatorXRef{objects: map[entity.Ref]entity.Object{ref: streamObj}}

	evaluator := NewTextEvaluator(xref)
	err := evaluator.ProcessObject(ref)
	require.NoError(t, err)
}

func TestTextEvaluator_ProcessDict_UsesAndRestoresResources(t *testing.T) {
	evaluator := NewTextEvaluator(nil)
	globalResources := entity.NewDict()
	evaluator.SetResources(globalResources)

	localResources := entity.NewDict()
	dict := entity.NewDict()
	dict.Set(entity.Name("Resources"), localResources)
	dict.Set(entity.Name("Contents"), entity.NewStream(entity.NewDict(), []byte("BT ET")))

	err := evaluator.ProcessDict(dict)
	require.NoError(t, err)
	assert.Equal(t, globalResources, evaluator.GetResources())
}

func TestTextEvaluator_DecodeText_UsesFont(t *testing.T) {
	evaluator := NewTextEvaluator(nil)
	evaluator.SetFont(&mockTextFont{})

	decoded := evaluator.DecodeText([]byte{65, 66})
	assert.Equal(t, "AB", decoded)
}

func TestTextEvaluator_AddTextItem(t *testing.T) {
	evaluator := NewTextEvaluator(nil)
	evaluator.AddTextItem("hello", 10, 20)

	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 1)
	assert.Equal(t, "hello", items[0].Text)
	assert.Equal(t, "hello", items[0].Unicode)
}

func TestTextEvaluator_DecodeText_CIDTwoByteCharCode(t *testing.T) {
	evaluator := NewTextEvaluator(nil)
	evaluator.SetFont(&mockCIDTextFont{})

	decoded := evaluator.DecodeText([]byte{0x4E, 0x00})
	assert.Equal(t, "一", decoded)
}

func TestDecodeGlyphNameToRune_UniName(t *testing.T) {
	decoded, ok := decodeGlyphNameToRune("uni00A9")
	require.True(t, ok)
	assert.Equal(t, '©', decoded)
}

func TestTextEvaluator_ExecuteOperator_TfAppliesFontSize(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ExecuteOperator("Tf", []float64{12})
	require.NoError(t, err)

	evaluator.AddTextItem("size", 0, 0)
	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 1)
	assert.Equal(t, float64(12), items[0].FontSize)
}

func TestTextEvaluator_ProcessBytes_HandlesTj(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT (Hello) Tj ET"))
	require.NoError(t, err)
	assert.Equal(t, "Hello", evaluator.GetTextLayer().Text())
}

func TestTextEvaluator_ProcessBytes_HandlesTJArray(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT [(He) -120 (llo)] TJ ET"))
	require.NoError(t, err)
	assert.Equal(t, "Hello", evaluator.GetTextLayer().Text())
}

func TestTextEvaluator_ProcessBytes_HandlesQuoteOperators(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT 12 TL (A) ' 20 5 (B) \" ET"))
	require.NoError(t, err)
	assert.Equal(t, "AB", evaluator.GetTextLayer().Text())
}

func TestTextEvaluator_ProcessBytes_TStarUsesNegativeLeading(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT 12 TL (A) Tj T* (B) Tj ET"))
	require.NoError(t, err)

	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 2)
	assert.Equal(t, "A", items[0].Text)
	assert.Equal(t, "B", items[1].Text)
	assert.Equal(t, 0, items[0].BoundingBox.Min.Y)
	assert.Equal(t, -12, items[1].BoundingBox.Min.Y)
}

func TestTextEvaluator_ProcessBytes_SingleQuoteUsesNegativeLeading(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT 12 TL (A) ' ET"))
	require.NoError(t, err)

	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 1)
	assert.Equal(t, "A", items[0].Text)
	assert.Equal(t, -12, items[0].BoundingBox.Min.Y)
}

func TestTextEvaluator_ProcessBytes_TfResolvesStandardFont(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	fontResource := entity.NewDict()
	fontResource.Set(entity.Name("BaseFont"), entity.Name("Helvetica"))
	fonts := entity.NewDict()
	fonts.Set(entity.Name("F1"), fontResource)
	resources := entity.NewDict()
	resources.Set(entity.Name("Font"), fonts)
	evaluator.SetResources(resources)

	err := evaluator.ProcessBytes([]byte("BT /F1 16 Tf (A) Tj ET"))
	require.NoError(t, err)

	require.NotNil(t, evaluator.GetFont())
	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 1)
	assert.Equal(t, float64(16), items[0].FontSize)
}

func TestTextEvaluator_ProcessBytes_TdWithIntegerOperands(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT 0 20 Td (A) Tj ET"))
	require.NoError(t, err)

	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 1)
	assert.Equal(t, "A", items[0].Text)
	assert.Equal(t, 0, items[0].BoundingBox.Min.X)
	assert.Equal(t, 20, items[0].BoundingBox.Min.Y)
}

func TestTextEvaluator_ProcessBytes_TmWithIntegerOperands(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	err := evaluator.ProcessBytes([]byte("BT 1 0 0 1 100 200 Tm (A) Tj ET"))
	require.NoError(t, err)

	items := evaluator.GetTextLayer().GetItems()
	require.Len(t, items, 1)
	assert.Equal(t, "A", items[0].Text)
	assert.Equal(t, 100, items[0].BoundingBox.Min.X)
	assert.Equal(t, 200, items[0].BoundingBox.Min.Y)
}

func TestTextEvaluator_AccessorsAndProcessArray(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	require.NotNil(t, evaluator.GetRegistry())

	state := graphics.NewState()
	evaluator.SetState(state)
	assert.Same(t, state, evaluator.GetState())

	arr := entity.NewArray(nil, entity.NewStream(entity.NewDict(), []byte("BT ET")))
	err := evaluator.ProcessArray(arr)
	require.NoError(t, err)
}

func TestTextEvaluator_OperatorExecuteBranches(t *testing.T) {
	evaluator := NewTextEvaluator(nil)

	assert.NoError(t, (&textSetCharSpacing{te: evaluator}).Execute(nil, []float64{1.25}))
	assert.Equal(t, 1.25, evaluator.charSpacing)

	assert.NoError(t, (&textSetWordSpacing{te: evaluator}).Execute(nil, []float64{2.5}))
	assert.Equal(t, 2.5, evaluator.wordSpacing)

	assert.NoError(t, (&textSetHorizScaling{te: evaluator}).Execute(nil, []float64{85}))
	assert.Equal(t, 85.0, evaluator.horizScale)

	assert.NoError(t, (&textSetRenderMode{te: evaluator}).Execute(nil, []float64{3}))
	assert.Equal(t, 3, evaluator.renderMode)

	assert.NoError(t, (&textSetTextRise{te: evaluator}).Execute(nil, []float64{4.5}))
	assert.Equal(t, 4.5, evaluator.textRise)

	assert.NoError(t, (&textShowText{te: evaluator}).Execute(nil, nil))
	assert.NoError(t, (&textShowTextArray{te: evaluator}).Execute(nil, nil))

	err := (&textMoveText{te: evaluator}).Execute(nil, []float64{1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "requires 2 operands")

	err = (&textMoveTextSetLeading{te: evaluator}).Execute(nil, []float64{1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "requires 2 operands")

	err = (&textSetTextMatrix{te: evaluator}).Execute(nil, []float64{1, 0, 0, 1, 10})
	require.Error(t, err)
	assert.ErrorContains(t, err, "requires 6 operands")

	evaluator.textLeading = 7
	evaluator.lineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
	assert.NoError(t, (&textNextLineShow{te: evaluator}).Execute(nil, nil))
	assert.Equal(t, -7.0, evaluator.textMatrix[5])

	err = (&textSetSpacingShow{te: evaluator}).Execute(nil, []float64{1, 2})
	require.Error(t, err)
	assert.ErrorContains(t, err, "requires 3 operands")

	evaluator.textLeading = 5
	evaluator.lineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
	assert.NoError(t, (&textSetSpacingShow{te: evaluator}).Execute(nil, []float64{10, 20, 30}))
	assert.Equal(t, 10.0, evaluator.wordSpacing)
	assert.Equal(t, 20.0, evaluator.charSpacing)
	assert.Equal(t, -5.0, evaluator.textMatrix[5])
}
