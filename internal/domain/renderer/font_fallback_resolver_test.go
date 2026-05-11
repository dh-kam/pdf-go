package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestDefaultFontFallbackResolver_ResolveMissingCandidate_UsesDefaultFont(t *testing.T) {
	eval := NewEvaluator(nil)
	resolver := defaultFontFallbackResolver{}

	font, err := resolver.ResolveMissingCandidate(eval, entity.NewDict(), "Type1", "ABCDEF+CMR10")
	require.NoError(t, err)
	require.NotNil(t, font)
	assert.Equal(t, "Times New Roman", font.Name())

	font, err = resolver.ResolveMissingCandidate(eval, entity.NewDict(), "Type1", "XYZABC+SFRM1095")
	require.NoError(t, err)
	require.NotNil(t, font)
	assert.Equal(t, "Times New Roman", font.Name())
}

func TestDefaultFontFallbackResolver_ResolveNonRenderableCandidate_UsesDefaultFont(t *testing.T) {
	eval := NewEvaluator(nil)
	resolver := defaultFontFallbackResolver{}
	current := &encodingTestFont{
		widths: map[uint32]float64{65: 500},
		names:  map[uint32]string{65: "A"},
	}

	font, ok := resolver.ResolveNonRenderableCandidate(eval, entity.NewDict(), "Type1", "XYZABC+SFRM1095", current)
	require.True(t, ok)
	require.NotNil(t, font)
	assert.Equal(t, "Times New Roman", font.Name())
}

func TestIsNarrowSystemFontFallback_OnlyAllowsUbuntuFamily(t *testing.T) {
	assert.True(t, isNarrowSystemFontFallback("Ubuntu"))
	assert.True(t, isNarrowSystemFontFallback("Ubuntu-Regular"))
	assert.True(t, isNarrowSystemFontFallback("ABCDEF+Ubuntu-Regular"))

	assert.False(t, isNarrowSystemFontFallback(""))
	assert.False(t, isNarrowSystemFontFallback("Helvetica"))
	assert.False(t, isNarrowSystemFontFallback("XYZABC+CMR10"))
}
