package renderer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestEvaluator_ShowText_CIDCodeUnits(t *testing.T) {
	e := NewEvaluator(nil)
	font := &mockCIDRenderFont{}
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(10)

	cidBytes := string([]byte{0x4E, 0x00, 0x4E, 0x8C})
	err := e.showText(Operator{
		Opcode:   "Tj",
		Operands: []entity.Object{entity.NewString(cidBytes)},
	})
	require.NoError(t, err)

	assert.Equal(t, "一二", e.ExtractedText())
	assert.Equal(t, []uint32{0x4E00, 0x4E00, 0x4E8C, 0x4E8C}, font.charCodes)
	assert.InDelta(t, 20.0, e.textMatrix[4], 0.001)
}

type mockCIDRenderFont struct {
	charCodes []uint32
}

func (m *mockCIDRenderFont) CharCodeToGlyph(code uint32) (uint32, error) {
	m.charCodes = append(m.charCodes, code)
	switch code {
	case 0x4E00:
		return 1, nil
	case 0x4E8C:
		return 2, nil
	default:
		return 0, fmt.Errorf("unknown code: %x", code)
	}
}

func (m *mockCIDRenderFont) GlyphName(glyph uint32) string {
	switch glyph {
	case 1:
		return "uni4E00"
	case 2:
		return "uni4E8C"
	default:
		return ""
	}
}

func (m *mockCIDRenderFont) GetGlyphWidth(glyph uint32) (float64, error) {
	switch glyph {
	case 1, 2:
		return 1000, nil
	default:
		return 0, fmt.Errorf("unknown glyph: %d", glyph)
	}
}

func (m *mockCIDRenderFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}

func (m *mockCIDRenderFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCIDRenderFont) IsCIDFont() bool {
	return true
}

func (m *mockCIDRenderFont) IsSymbolic() bool {
	return false
}

func (m *mockCIDRenderFont) UnitsPerEm() uint16 {
	return 1000
}

func (m *mockCIDRenderFont) Name() string {
	return "MockCIDFont"
}
