package renderer

import (
	"os"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

// getDefaultFont returns a standard font as fallback.
func (e *Evaluator) getDefaultFont(baseFont string) (entity.Font, error) {
	fontName := normalizeBaseFontName(baseFont)
	if forced := forcedFallbackFontNameForBaseFontDebug(baseFont); forced != "" {
		fontName = forced
	}
	if forced := strings.TrimSpace(os.Getenv("PDF_DEBUG_FORCE_FONT")); forced != "" {
		fontName = forced
	}
	if fontName == "" {
		fontName = "Times-Roman"
	}

	font, ok := standard.GetFont(fontName)
	if !ok {
		font, _ = standard.GetFont("Times-Roman")
	}

	return font, nil
}

func (e *Evaluator) preferredFallbackFont(baseFont string) (entity.Font, bool) {
	normalized := normalizeBaseFontName(baseFont)
	if normalized == "" {
		return nil, false
	}

	subsetFree := stripSubsetPrefix(strings.TrimPrefix(strings.TrimSpace(baseFont), "/"))
	if normalized == subsetFree {
		return nil, false
	}
	if shouldDeferSubsetFallbackToResolver(subsetFree) {
		return nil, false
	}

	font, err := e.getDefaultFont(baseFont)
	if err != nil || font == nil {
		return nil, false
	}
	return font, true
}

func shouldDeferSubsetFallbackToResolver(subsetFree string) bool {
	// cm-super SFRM fonts need late fallback resolution so experiments and
	// Poppler-parity probes can distinguish them from Computer Modern CMR.
	return strings.HasPrefix(strings.TrimSpace(subsetFree), "SFRM")
}
