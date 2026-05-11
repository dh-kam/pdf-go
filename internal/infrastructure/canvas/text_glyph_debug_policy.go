package canvas

import (
	"os"
	"strconv"
	"strings"
)

// textGlyphGammaForDebug returns the gamma correction exponent for text glyph
// alpha masks. Values < 1.0 make text bolder (boosting thin anti-aliased strokes).
// Default 0.6 approximates the visual effect of FreeType's auto-hinting.
func textGlyphGammaForDebug() float64 {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_TEXT_GLYPH_GAMMA"))
	if raw == "" {
		return 1.0 // no correction by default
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v > 2.0 {
		return 1.0
	}
	return v
}

func textGlyphSupersampleFactorForDebug() int {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE"))
	if raw == "" {
		return 2
	}

	factor, err := strconv.Atoi(raw)
	if err != nil || factor < 1 {
		return 1
	}
	if factor > 4 {
		return 4
	}
	return factor
}
