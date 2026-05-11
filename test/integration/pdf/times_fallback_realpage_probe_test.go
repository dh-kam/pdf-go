package pdf_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetProbeEnvForRender_RestoresPreviousValues(t *testing.T) {
	t.Setenv("PDF_DEBUG_FORCE_BASE_FONT_MAP", "CMR10=Courier")
	_ = os.Unsetenv("PDF_DEBUG_SKIP_TEXT_BASE_FONTS")
	t.Setenv("PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE", "CMR10=101")

	restore := setProbeEnvForRender(t, map[string]string{
		"PDF_DEBUG_SKIP_TEXT_BASE_FONTS":     "SFRM1095",
		"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": "SFRM1095=101,110",
	})

	require.Equal(t, "", os.Getenv("PDF_DEBUG_FORCE_BASE_FONT_MAP"))
	require.Equal(t, "SFRM1095", os.Getenv("PDF_DEBUG_SKIP_TEXT_BASE_FONTS"))
	require.Equal(t, "SFRM1095=101,110", os.Getenv("PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE"))

	restore()

	require.Equal(t, "CMR10=Courier", os.Getenv("PDF_DEBUG_FORCE_BASE_FONT_MAP"))
	_, ok := os.LookupEnv("PDF_DEBUG_SKIP_TEXT_BASE_FONTS")
	require.False(t, ok)
	require.Equal(t, "CMR10=101", os.Getenv("PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE"))
}
