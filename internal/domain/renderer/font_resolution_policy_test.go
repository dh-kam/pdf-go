package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

func TestResolveFirstDescendantFontDict_ResolvesDirectAndRef(t *testing.T) {
	childDict := entity.NewDict()
	childDict.Set(entity.Name("Subtype"), entity.Name("CIDFontType2"))

	t.Run("direct", func(t *testing.T) {
		eval := NewEvaluator(nil)
		parent := entity.NewDict()
		parent.Set(entity.Name("DescendantFonts"), entity.NewArray(childDict))

		resolved, ok := eval.resolveFirstDescendantFontDict(parent)
		require.True(t, ok)
		assert.Same(t, childDict, resolved)
	})

	t.Run("ref", func(t *testing.T) {
		ref := entity.NewRef(10, 0)
		eval := NewEvaluator(&testMapXRef{
			objects: map[entity.Ref]entity.Object{
				ref: childDict,
			},
		})
		parent := entity.NewDict()
		parent.Set(entity.Name("DescendantFonts"), entity.NewArray(ref))

		resolved, ok := eval.resolveFirstDescendantFontDict(parent)
		require.True(t, ok)
		assert.Same(t, childDict, resolved)
	})
}

func TestResolveType0FontCandidate_DelegatesToResolverForDescendantSubtype(t *testing.T) {
	font, ok := standard.GetFont("Helvetica")
	require.True(t, ok)

	descendant := entity.NewDict()
	descendant.Set(entity.Name("Subtype"), entity.Name("CIDFontType2"))

	parent := entity.NewDict()
	parent.Set(entity.Name("DescendantFonts"), entity.NewArray(descendant))

	eval := NewEvaluator(nil)
	resolver := &stubFontCandidateResolver{font: font}
	eval.fontResolver = resolver

	resolved := eval.resolveType0FontCandidate(parent, "ParentType0")
	require.NotNil(t, resolved)
	require.True(t, resolver.called)
	assert.Equal(t, "CIDFontType2", resolver.subtype)
	assert.Equal(t, "ParentType0", resolver.base)
}

func TestWrapCIDFontType0CWithCIDToGIDMap_DefaultKeepsRawCIDGlyphIDs(t *testing.T) {
	eval := NewEvaluator(nil)
	font := &cidToGIDMapTestFont{
		glyphRenderTestFont: &glyphRenderTestFont{
			name:    "RawCIDType0C",
			charMap: map[uint32]uint32{3296: 3296},
		},
		cidToGID: map[uint32]uint32{3296: 22},
	}

	wrapped := eval.wrapCIDFontType0CWithCIDToGIDMap(font)

	require.IsType(t, &cidDirectFont{}, wrapped)
	assert.True(t, wrapped.IsCIDFont())
	glyph, err := wrapped.CharCodeToGlyph(3296)
	require.NoError(t, err)
	assert.Equal(t, uint32(3296), glyph)
}

func TestWrapCIDFontType0CWithCIDToGIDMap_DebugGateEnablesMap(t *testing.T) {
	t.Setenv("PDF_DEBUG_CIDTYPE0C_CIDTOGID_MAP", "1")
	eval := NewEvaluator(nil)
	font := &cidToGIDMapTestFont{
		glyphRenderTestFont: &glyphRenderTestFont{
			name:    "RawCIDType0C",
			charMap: map[uint32]uint32{3296: 3296},
		},
		cidToGID: map[uint32]uint32{3296: 22},
	}

	wrapped := eval.wrapCIDFontType0CWithCIDToGIDMap(font)

	require.IsType(t, &cidToGIDMappedFont{}, wrapped)
	glyph, err := wrapped.CharCodeToGlyph(3296)
	require.NoError(t, err)
	assert.Equal(t, uint32(22), glyph)
}

func TestDefaultFontCandidateResolver_Type3ReturnsFont(t *testing.T) {
	eval := NewEvaluator(nil)
	resolved := defaultFontCandidateResolver{}.ResolveCandidate(eval, entity.NewDict(), "Type3", "Type3Font", nil, assert.AnError)
	assert.NotNil(t, resolved)
}

type cidToGIDMapTestFont struct {
	*glyphRenderTestFont
	cidToGID map[uint32]uint32
}

func (f *cidToGIDMapTestFont) CIDToGIDMap() (map[uint32]uint32, bool) {
	return f.cidToGID, len(f.cidToGID) > 0
}
