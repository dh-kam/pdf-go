package splash

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type glyphCacheHeightTestFont struct {
	bbox         [4]float64
	units        uint16
	popplerBBox  [4]float64
	popplerUnits uint16
	hasPoppler   bool
}

func (f *glyphCacheHeightTestFont) CharCodeToGlyph(code uint32) (uint32, error) {
	return code, nil
}

func (f *glyphCacheHeightTestFont) GlyphName(glyph uint32) string {
	return ""
}

func (f *glyphCacheHeightTestFont) GetGlyphWidth(glyph uint32) (float64, error) {
	return 500, nil
}

func (f *glyphCacheHeightTestFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3]
}

func (f *glyphCacheHeightTestFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, errors.New("not implemented")
}

func (f *glyphCacheHeightTestFont) IsCIDFont() bool {
	return false
}

func (f *glyphCacheHeightTestFont) IsSymbolic() bool {
	return false
}

func (f *glyphCacheHeightTestFont) UnitsPerEm() uint16 {
	if f.units == 0 {
		return 1000
	}
	return f.units
}

func (f *glyphCacheHeightTestFont) Name() string {
	return "GlyphCacheHeightTestFont"
}

func (f *glyphCacheHeightTestFont) PopplerGlyphCacheBBox() (float64, float64, float64, float64, uint16, bool) {
	if !f.hasPoppler {
		return 0, 0, 0, 0, 0, false
	}
	return f.popplerBBox[0], f.popplerBBox[1], f.popplerBBox[2], f.popplerBBox[3], f.popplerUnits, true
}

func TestSplashGlyphCacheHeightUsesPopplerIntegerTruncation(t *testing.T) {
	font := &glyphCacheHeightTestFont{
		bbox:  [4]float64{0, -2.9, 100, 2.9},
		units: 1,
	}

	assert.Equal(t, 7.0, splashGlyphCacheHeight(font, 1, 1))
}

func TestSplashGlyphCacheHeightPrefersPopplerBBoxProvider(t *testing.T) {
	font := &glyphCacheHeightTestFont{
		bbox:         [4]float64{0, -100, 100, 100},
		units:        1000,
		popplerBBox:  [4]float64{0, -3000, 100, 1000},
		popplerUnits: 1000,
		hasPoppler:   true,
	}

	assert.Equal(t, 83.0, splashGlyphCacheHeight(font, 10, 2))
}
