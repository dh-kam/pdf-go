package renderer

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/cff"
)

func (e *Evaluator) applyEmbeddedType1CGlyphSourceFromDict(dict *entity.Dict, font entity.Font, fontData []byte) entity.Font {
	if dict == nil || font == nil || len(fontData) == 0 || !looksLikeCFFEmbeddedFont(fontData) {
		return font
	}

	cffFont, err := cff.NewFont(fontData)
	if err != nil {
		return font
	}

	encodingMap := e.resolveSimpleFontEncoding(dict.Get(entity.Name("Encoding")))
	if len(encodingMap) == 0 {
		encodingMap = e.resolveEmbeddedType1Encoding(dict)
	}
	if len(encodingMap) == 0 {
		return font
	}

	sourceByCode := map[uint32]glyphSourceOverride{}
	targetByCode := map[uint32]uint32{}
	nameByCode := map[uint32]string{}
	for code, name := range encodingMap {
		targetGlyph, err := font.CharCodeToGlyph(uint32(code))
		if err != nil {
			continue
		}
		for _, candidate := range encodingGlyphNameCandidates(name) {
			sourceGlyph, ok := cffFont.GlyphIDByName(candidate)
			if !ok {
				continue
			}
			sourceByCode[uint32(code)] = glyphSourceOverride{
				font:  cffFont,
				glyph: sourceGlyph,
			}
			targetByCode[uint32(code)] = targetGlyph
			nameByCode[uint32(code)] = candidate
			break
		}
	}
	if len(sourceByCode) == 0 {
		return font
	}

	if os.Getenv("PDF_DEBUG_TYPE1C_GLYPH_SOURCE") == "1" {
		fmt.Fprintf(os.Stderr, "TYPE1C_CODETOGID overrides=%d\n", len(sourceByCode))
		for code, override := range sourceByCode {
			fmt.Fprintf(os.Stderr, "TYPE1C_CODETOGID code=%d name=%s targetGlyph=%d sourceGlyph=%d\n",
				code, nameByCode[code], targetByCode[code], override.glyph)
		}
	}

	var cacheBBox [4]float64
	var cacheUnits uint16
	hasCacheBBox := false
	if bboxFont, ok := any(cffFont).(freeTypeBoundingBoxFont); ok {
		xMin, yMin, xMax, yMax, units, found := bboxFont.FreeTypeBoundingBox()
		if found && units > 0 && xMax > xMin && yMax > yMin {
			cacheBBox = [4]float64{xMin, yMin, xMax, yMax}
			cacheUnits = units
			hasCacheBBox = true
		}
	}

	return &type1CCodeToGIDFont{
		base:         font,
		sourceByCode: sourceByCode,
		targetByCode: targetByCode,
		nameByCode:   nameByCode,
		cacheBBox:    cacheBBox,
		cacheUnits:   cacheUnits,
		hasCacheBBox: hasCacheBBox,
	}
}
