package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

type stubFontFallbackResolver struct {
	missingFont     entity.Font
	missingErr      error
	nonRenderable   entity.Font
	nonRenderableOK bool
	missingCalled   bool
	nonRenderCalled bool
	subtype         string
	base            string
}

type mappedFontFallbackResolver struct {
	byBase map[string]entity.Font
}

func (s *stubFontFallbackResolver) ResolveMissingCandidate(_ *Evaluator, _ *entity.Dict, subtype, baseFont string) (entity.Font, error) {
	s.missingCalled = true
	s.subtype = subtype
	s.base = baseFont
	return s.missingFont, s.missingErr
}

func (s *stubFontFallbackResolver) ResolveNonRenderableCandidate(_ *Evaluator, _ *entity.Dict, subtype, baseFont string, _ entity.Font) (entity.Font, bool) {
	s.nonRenderCalled = true
	s.subtype = subtype
	s.base = baseFont
	return s.nonRenderable, s.nonRenderableOK
}

func (m mappedFontFallbackResolver) ResolveMissingCandidate(evaluator *Evaluator, _ *entity.Dict, _ string, baseFont string) (entity.Font, error) {
	if font, ok := m.byBase[baseFont]; ok {
		return font, nil
	}
	return evaluator.getDefaultFont(baseFont)
}

func (m mappedFontFallbackResolver) ResolveNonRenderableCandidate(evaluator *Evaluator, _ *entity.Dict, _ string, baseFont string, _ entity.Font) (entity.Font, bool) {
	if font, ok := m.byBase[baseFont]; ok {
		return font, true
	}

	font, err := evaluator.getDefaultFont(baseFont)
	if err != nil || font == nil {
		return nil, false
	}
	return font, true
}

func TestPreferredFallbackFont_OnlyUsesImmediateFallbackForNormalizedSubsetFamilies(t *testing.T) {
	eval := NewEvaluator(nil)

	cmrFont, cmrOK := eval.preferredFallbackFont("ABCDEF+CMR10")
	require.True(t, cmrOK)
	require.NotNil(t, cmrFont)
	assert.Equal(t, "Times New Roman", cmrFont.Name())

	sfrmFont, sfrmOK := eval.preferredFallbackFont("XYZABC+SFRM1095")
	assert.False(t, sfrmOK)
	assert.Nil(t, sfrmFont)

	plainFont, plainOK := eval.preferredFallbackFont("Times-Roman")
	assert.False(t, plainOK)
	assert.Nil(t, plainFont)
}

func TestGetFontFromDict_Type1SubsetFallbackPathsDifferButConvergeToSameDefaultFont(t *testing.T) {
	eval := NewEvaluator(nil)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))

	cmrFont, err := eval.getFontFromDict(fontDict, "ABCDEF+CMR10")
	require.NoError(t, err)
	require.NotNil(t, cmrFont)
	assert.Equal(t, "Times New Roman", cmrFont.Name())

	sfrmImmediate, sfrmImmediateOK := eval.preferredFallbackFont("XYZABC+SFRM1095")
	assert.False(t, sfrmImmediateOK)
	assert.Nil(t, sfrmImmediate)

	sfrmFont, err := eval.getFontFromDict(fontDict, "XYZABC+SFRM1095")
	require.NoError(t, err)
	require.NotNil(t, sfrmFont)
	assert.Equal(t, "Times New Roman", sfrmFont.Name())
}

func TestGetFontFromDict_ExperimentalSFRM1095LateFallbackBreaksCurrentCollapseAtResolverBoundary(t *testing.T) {
	eval := NewEvaluator(nil)

	helvetica, ok := standard.GetFont("Helvetica")
	require.True(t, ok)
	eval.fontFallback = mappedFontFallbackResolver{
		byBase: map[string]entity.Font{
			"XYZABC+SFRM1095": helvetica,
		},
	}

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))

	cmrFont, err := eval.getFontFromDict(fontDict, "ABCDEF+CMR10")
	require.NoError(t, err)
	require.NotNil(t, cmrFont)
	assert.Equal(t, "Times New Roman", cmrFont.Name())

	sfrmFont, err := eval.getFontFromDict(fontDict, "XYZABC+SFRM1095")
	require.NoError(t, err)
	require.NotNil(t, sfrmFont)
	assert.Equal(t, "Helvetica", sfrmFont.Name())
	assert.NotEqual(t, cmrFont.Name(), sfrmFont.Name())
}
