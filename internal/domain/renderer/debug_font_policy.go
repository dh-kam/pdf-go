package renderer

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

func shouldIgnoreMappedWidth500ForDebug(fontName string) bool {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_IGNORE_MAPPED_WIDTH_500"))
	if raw == "" {
		return false
	}

	normalizedName := normalizeDebugFontName(fontName)
	for _, token := range strings.Split(raw, ",") {
		candidate := normalizeDebugFontName(token)
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == normalizedName {
			return true
		}
	}
	return false
}

func normalizeDebugFontName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "/")
	return strings.ToLower(name)
}

func shouldUseEmbeddedType1ForBaseFontDebug(baseFont string) bool {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE"))
	return debugBaseFontListMatches(raw, baseFont)
}

func shouldUseFallbackType1ForBaseFontDebug(baseFont string) bool {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE"))
	return debugBaseFontListMatches(raw, baseFont)
}

func debugBaseFontListMatches(raw string, baseFont string) bool {
	if raw == "" {
		return false
	}

	normalizedBase := normalizeDebugFontName(normalizeBaseFontName(baseFont))
	trimmedBase := normalizeDebugFontName(baseFont)
	subsetFreeBase := normalizeDebugFontName(stripSubsetPrefix(baseFont))
	for _, token := range strings.Split(raw, ",") {
		candidate := normalizeDebugFontName(token)
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == normalizedBase || candidate == trimmedBase || candidate == subsetFreeBase {
			return true
		}
	}
	return false
}

func forcedFallbackFontNameForBaseFontDebug(baseFont string) string {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_FORCE_BASE_FONT_MAP"))
	if raw == "" {
		return ""
	}

	normalizedBase := normalizeDebugFontName(normalizeBaseFontName(baseFont))
	trimmedBase := normalizeDebugFontName(baseFont)
	subsetFreeBase := normalizeDebugFontName(stripSubsetPrefix(baseFont))
	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 {
			continue
		}
		left := normalizeDebugFontName(parts[0])
		right := strings.TrimSpace(parts[1])
		if left == "" || right == "" {
			continue
		}
		if left == normalizedBase || left == trimmedBase || left == subsetFreeBase {
			return right
		}
	}
	return ""
}

func shouldSkipDebugTextFont(debugName string, font entity.Font) bool {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_TEXT_BASE_FONTS"))
	if raw == "" {
		return false
	}

	candidates := []string{
		normalizeDebugFontName(debugName),
		normalizeDebugFontName(stripSubsetPrefix(debugName)),
		normalizeDebugFontName(normalizeBaseFontName(debugName)),
	}
	if font != nil {
		candidates = append(candidates, normalizeDebugFontName(font.Name()))
	}

	for _, token := range strings.Split(raw, ",") {
		want := normalizeDebugFontName(token)
		if want == "" {
			continue
		}
		if want == "*" {
			return true
		}
		for _, candidate := range candidates {
			if candidate != "" && candidate == want {
				return true
			}
		}
	}
	return false
}

func applyGlyphSourceOverrideFontForDebug(baseFont string, font entity.Font) entity.Font {
	if font == nil {
		return nil
	}

	overrides := glyphSourceOverridesForBaseFontDebug(baseFont)
	if len(overrides) == 0 {
		return font
	}

	glyphOverrides := make(map[uint32]glyphSourceOverride, len(overrides))
	for code, sourceFontName := range overrides {
		sourceFont, ok := standard.GetFont(sourceFontName)
		if !ok {
			continue
		}

		targetGlyph, err := font.CharCodeToGlyph(uint32(code))
		if err != nil {
			continue
		}
		sourceGlyph, err := sourceFont.CharCodeToGlyph(uint32(code))
		if err != nil {
			continue
		}

		glyphOverrides[targetGlyph] = glyphSourceOverride{
			font:  sourceFont,
			glyph: sourceGlyph,
		}
	}
	if len(glyphOverrides) == 0 {
		return font
	}

	return &glyphSourceOverrideFont{
		base:      font,
		overrides: glyphOverrides,
	}
}

func glyphSourceOverridesForBaseFontDebug(baseFont string) map[int]string {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP"))
	if raw == "" {
		return nil
	}

	normalizedBase := normalizeDebugFontName(normalizeBaseFontName(baseFont))
	trimmedBase := normalizeDebugFontName(baseFont)
	subsetFreeBase := normalizeDebugFontName(stripSubsetPrefix(baseFont))
	result := make(map[int]string)

	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		leftRight := strings.SplitN(token, "=", 2)
		if len(leftRight) != 2 {
			continue
		}
		left := strings.TrimSpace(leftRight[0])
		sourceFont := strings.TrimSpace(leftRight[1])
		if left == "" || sourceFont == "" {
			continue
		}

		baseAndCode := strings.SplitN(left, ":", 2)
		if len(baseAndCode) != 2 {
			continue
		}
		leftBase := normalizeDebugFontName(baseAndCode[0])
		if leftBase == "" {
			continue
		}
		if leftBase != normalizedBase && leftBase != trimmedBase && leftBase != subsetFreeBase {
			continue
		}

		code, err := strconv.Atoi(strings.TrimSpace(baseAndCode[1]))
		if err != nil || code < 0 {
			continue
		}
		result[code] = sourceFont
	}

	return result
}

func formatGlyphSourceOverrideSpecForDebug(baseFont string, code int, sourceFont string) string {
	return fmt.Sprintf("%s:%d=%s", baseFont, code, sourceFont)
}
