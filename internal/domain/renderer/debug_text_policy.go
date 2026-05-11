package renderer

import (
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type textRenderPolicy interface {
	ShouldSkipAllText() bool
	ShouldSkipTextFont(debugName string, font entity.Font) bool
	ShouldUseFastPathTextRenderMode() bool
	HasSkippedTextCodes(debugName string, font entity.Font) bool
	ShouldSkipTextCode(debugName string, font entity.Font, code uint32) bool
}

type defaultTextRenderPolicy struct{}

func (defaultTextRenderPolicy) ShouldSkipAllText() bool {
	return shouldSkipAllTextForDebug()
}

func (defaultTextRenderPolicy) ShouldSkipTextFont(debugName string, font entity.Font) bool {
	return shouldSkipTextFontForDebug(debugName, font)
}

func (defaultTextRenderPolicy) ShouldUseFastPathTextRenderMode() bool {
	return shouldUseFastPathTextRenderModeForDebug()
}

func (defaultTextRenderPolicy) HasSkippedTextCodes(debugName string, font entity.Font) bool {
	return hasSkippedTextCodesForDebug(debugName, font)
}

func (defaultTextRenderPolicy) ShouldSkipTextCode(debugName string, font entity.Font, code uint32) bool {
	return shouldSkipTextCodeForDebug(debugName, font, code)
}

func shouldSkipAllTextForDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_TEXT")) == "1"
}

func shouldSkipTextFontForDebug(debugName string, font entity.Font) bool {
	return shouldSkipDebugTextFont(debugName, font)
}

func shouldUseFastPathTextRenderModeForDebug() bool {
	return strings.TrimSpace(os.Getenv("PDF_DEBUG_TEXT_RENDER_MODE")) == "fast-path"
}

func hasSkippedTextCodesForDebug(debugName string, font entity.Font) bool {
	return len(debugTextCodeSetForBase(debugName, font)) > 0
}

func shouldSkipTextCodeForDebug(debugName string, font entity.Font, code uint32) bool {
	skipCodes := debugTextCodeSetForBase(debugName, font)
	if len(skipCodes) == 0 {
		return false
	}
	_, ok := skipCodes[code]
	return ok
}

func debugTextCodeSetForBase(debugName string, font entity.Font) map[uint32]struct{} {
	raw := strings.TrimSpace(os.Getenv("PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE"))
	if raw == "" {
		return nil
	}

	targets := debugTextCodeMap(raw)
	if len(targets) == 0 {
		return nil
	}

	names := []string{
		normalizeDebugFontName(debugName),
		normalizeDebugFontName(stripSubsetPrefix(debugName)),
		normalizeDebugFontName(normalizeBaseFontName(debugName)),
	}
	if font != nil {
		names = append(names, normalizeDebugFontName(font.Name()))
	}

	for _, name := range names {
		if name == "" {
			continue
		}
		if codes, ok := targets[name]; ok {
			return codes
		}
	}
	return nil
}

func debugTextCodeMap(raw string) map[string]map[uint32]struct{} {
	out := make(map[string]map[uint32]struct{})
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, values, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		name = normalizeDebugFontName(name)
		if name == "" {
			continue
		}

		codeSet := make(map[uint32]struct{})
		for _, part := range strings.Split(values, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			code, err := strconv.Atoi(part)
			if err != nil || code < 0 || code > 0x10FFFF {
				continue
			}
			codeSet[uint32(code)] = struct{}{}
		}
		if len(codeSet) > 0 {
			out[name] = codeSet
		}
	}
	return out
}
