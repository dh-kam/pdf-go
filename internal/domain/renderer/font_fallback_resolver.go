package renderer

import (
	"os/exec"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/truetype"
)

type fontFallbackResolver interface {
	ResolveMissingCandidate(evaluator *Evaluator, dict *entity.Dict, subtype, baseFont string) (entity.Font, error)
	ResolveNonRenderableCandidate(evaluator *Evaluator, dict *entity.Dict, subtype, baseFont string, current entity.Font) (entity.Font, bool)
}

type defaultFontFallbackResolver struct{}

func (defaultFontFallbackResolver) ResolveMissingCandidate(evaluator *Evaluator, _ *entity.Dict, subtype, baseFont string) (entity.Font, error) {
	if font := resolveMissingSystemTrueTypeFont(subtype, baseFont); font != nil {
		return font, nil
	}
	return evaluator.getDefaultFont(baseFont)
}

func (defaultFontFallbackResolver) ResolveNonRenderableCandidate(evaluator *Evaluator, _ *entity.Dict, _ string, baseFont string, _ entity.Font) (entity.Font, bool) {
	font, err := evaluator.getDefaultFont(baseFont)
	if err != nil || font == nil {
		return nil, false
	}
	return font, true
}

func resolveMissingSystemTrueTypeFont(subtype, baseFont string) entity.Font {
	if subtype != "TrueType" || !isNarrowSystemFontFallback(baseFont) {
		return nil
	}
	path, ok := fontConfigMatchFile(baseFont)
	if !ok {
		return nil
	}
	font, err := truetype.NewFont(path)
	if err != nil {
		return nil
	}
	return font
}

func isNarrowSystemFontFallback(baseFont string) bool {
	name := stripSubsetPrefix(baseFont)
	switch strings.TrimSpace(name) {
	case "Ubuntu", "Ubuntu-Regular":
		return true
	default:
		return false
	}
}

func fontConfigMatchFile(baseFont string) (string, bool) {
	name := stripSubsetPrefix(baseFont)
	if strings.TrimSpace(name) == "" {
		return "", false
	}
	out, err := exec.Command("fc-match", "-f", "%{file}", name).Output()
	if err != nil {
		return "", false
	}
	path := strings.TrimSpace(string(out))
	return path, path != ""
}
