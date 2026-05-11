package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestType1GlyphProbeModes_ContainsCurrentAndForcedModes(t *testing.T) {
	t.Parallel()

	modes := type1GlyphProbeModes()
	require.Len(t, modes, 3)

	byName := make(map[string]type1GlyphProbeMode, len(modes))
	for _, mode := range modes {
		byName[mode.name] = mode
	}

	require.Contains(t, byName, "current")
	require.Empty(t, byName["current"].key)

	require.Contains(t, byName, "force_helvetica")
	require.Equal(t, "PDF_DEBUG_FORCE_BASE_FONT_MAP", byName["force_helvetica"].key)
	require.Equal(t, "SFRM1095=Helvetica,CMR10=Helvetica", byName["force_helvetica"].value)

	require.Contains(t, byName, "force_courier")
	require.Equal(t, "PDF_DEBUG_FORCE_BASE_FONT_MAP", byName["force_courier"].key)
	require.Equal(t, "SFRM1095=Courier,CMR10=Courier", byName["force_courier"].value)
}
