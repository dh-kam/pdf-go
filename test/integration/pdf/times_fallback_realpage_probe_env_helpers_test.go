package pdf_test

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mergeProbeEnv(envs ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, env := range envs {
		for key, value := range env {
			merged[key] = value
		}
	}
	return merged
}

func skipBaseFontsExceptForProbe(baseFonts []string, keep string) string {
	keep = strings.TrimSpace(keep)
	if keep == "" {
		return strings.Join(baseFonts, ",")
	}

	skipped := make([]string, 0, len(baseFonts))
	for _, name := range baseFonts {
		if strings.EqualFold(strings.TrimSpace(name), keep) {
			continue
		}
		skipped = append(skipped, strings.TrimSpace(name))
	}
	sort.Strings(skipped)
	return strings.Join(skipped, ",")
}

func targetFontOnlyEnvForProbe(baseFonts []string, keep string) map[string]string {
	return map[string]string{
		"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": skipBaseFontsExceptForProbe(baseFonts, keep),
	}
}

func fullTargetFontSkipEnvForProbe(targetOnlyEnv map[string]string, baseFont string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": baseFont + "," + targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
	})
}

func codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, codeSpec string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": codeSpec,
	})
}

func fastPathWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_TEXT_RENDER_MODE": "fast-path",
	})
}

func glyphSupersampleWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, factor int) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE": strconv.Itoa(factor),
	})
}

func forcedFastPathWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, forcedEnv map[string]string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, forcedEnv, map[string]string{
		"PDF_DEBUG_TEXT_RENDER_MODE": "fast-path",
	})
}

func embeddedWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, baseFont string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE": baseFont,
	})
}

func fallbackWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, baseFont string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE": baseFont,
	})
}

func expandedCodeSkipWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, tc realPageLowercaseProbeCase) map[string]string {
	return codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.expandedCodeSpec())
}

func setProbeEnvForRender(t *testing.T, env map[string]string) func() {
	t.Helper()

	keys := map[string]struct{}{
		"PDF_DEBUG_FORCE_BASE_FONT_MAP":      {},
		"PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP":   {},
		"PDF_DEBUG_SKIP_FILL_PATHS":          {},
		"PDF_DEBUG_SKIP_IMAGES":              {},
		"PDF_DEBUG_SKIP_STROKE_PATHS":        {},
		"PDF_DEBUG_SKIP_XOBJECTS":            {},
		"PDF_DEBUG_SKIP_TEXT":                {},
		"PDF_DEBUG_SKIP_TEXT_BASE_FONTS":     {},
		"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": {},
		"PDF_DEBUG_TEXT_RENDER_MODE":         {},
		"PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE":   {},
		"PDF_DEBUG_TEXT_GLYPH_GAMMA":         {},
		"PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE":  {},
		"PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE":  {},
	}
	for key := range env {
		keys[key] = struct{}{}
	}

	orderedKeys := make([]string, 0, len(keys))
	for key := range keys {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Strings(orderedKeys)

	previous := make(map[string]*string, len(orderedKeys))
	for _, key := range orderedKeys {
		if value, ok := os.LookupEnv(key); ok {
			saved := value
			previous[key] = &saved
		} else {
			previous[key] = nil
		}
		_ = os.Unsetenv(key)
	}
	for key, value := range env {
		require.NoError(t, os.Setenv(key, value))
	}

	return func() {
		for _, key := range orderedKeys {
			saved := previous[key]
			if saved == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *saved)
		}
	}
}
