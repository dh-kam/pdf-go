package renderer

import "github.com/dh-kam/pdf-go/internal/domain/entity"

func (e *Evaluator) applyFontMetricsFromDict(dict *entity.Dict, font entity.Font) entity.Font {
	if font == nil || dict == nil {
		return font
	}

	if mapped := e.applySimpleFontWidthsFromDict(dict, font); mapped != nil {
		return mapped
	}
	if mapped := e.applyCIDFontWidthsFromDict(dict, font); mapped != nil {
		return mapped
	}
	return font
}

func (e *Evaluator) applySimpleFontWidthsFromDict(dict *entity.Dict, font entity.Font) entity.Font {
	widthsObj := dict.Get(entity.Name("Widths"))
	widthsArray, ok := widthsObj.(*entity.Array)
	if !ok || widthsArray.Len() == 0 {
		return nil
	}

	firstChar, hasFirst := objectInt(dict.Get(entity.Name("FirstChar")))
	lastChar, hasLast := objectInt(dict.Get(entity.Name("LastChar")))
	if !hasFirst || !hasLast {
		return nil
	}

	if firstChar > lastChar {
		return nil
	}

	// PDF Widths are in 1/1000 text-space units (per PDF spec section 9.6.3).
	// glyphAdvance divides by font.UnitsPerEm(), so we scale widths from 1/1000 em
	// to font units: width_fu = width_pdf * upem / 1000.
	// For upem=1000 (Type1) this is a no-op; for upem=2048 (TrueType) it doubles the value.
	upem := float64(1000)
	if u := font.UnitsPerEm(); u > 0 {
		upem = float64(u)
	}
	widthScale := upem / 1000.0

	codeWidths := map[uint32]float64{}
	widthsByCode := map[uint32]float64{}
	for i := 0; i < widthsArray.Len(); i++ {
		width, ok := objectFloat(widthsArray.Get(i))
		if !ok {
			continue
		}

		charCode := firstChar + i
		if charCode > lastChar {
			break
		}

		glyphID, err := font.CharCodeToGlyph(uint32(charCode))
		if err != nil {
			continue
		}
		scaledWidth := width * widthScale
		codeWidths[glyphID] = scaledWidth
		widthsByCode[uint32(charCode)] = scaledWidth
	}

	if len(codeWidths) == 0 {
		return font
	}

	defaultWidth := 500.0
	if dwObj, ok := dict.Get(entity.Name("DW")).(*entity.Integer); ok {
		defaultWidth = float64(dwObj.Value())
	} else if dwObj, ok := dict.Get(entity.Name("DW")).(*entity.Real); ok {
		defaultWidth = dwObj.Value()
	}

	return &widthMappedFont{
		base:         font,
		widths:       codeWidths,
		widthsByCode: widthsByCode,
		defaultWidth: defaultWidth,
	}
}

func (e *Evaluator) applyCIDFontWidthsFromDict(dict *entity.Dict, font entity.Font) entity.Font {
	widthsObj := dict.Get(entity.Name("W"))
	widthsArray, ok := widthsObj.(*entity.Array)
	if !ok || widthsArray.Len() == 0 {
		return nil
	}

	// CIDFont /W widths are PDF text-space widths in 1/1000 em. The rest of
	// the renderer expects GetGlyphWidth to return font units, so keep the
	// same scaling contract as simple /Widths.
	upem := float64(1000)
	if u := font.UnitsPerEm(); u > 0 {
		upem = float64(u)
	}
	widthScale := upem / 1000.0

	codeWidths := map[uint32]float64{}
	for i := 0; i < widthsArray.Len(); {
		firstCID, ok := objectInt(widthsArray.Get(i))
		if !ok {
			i++
			continue
		}
		i++
		if i >= widthsArray.Len() {
			break
		}

		if perCID, ok := widthsArray.Get(i).(*entity.Array); ok {
			i++
			for offset := 0; offset < perCID.Len(); offset++ {
				width, ok := objectFloat(perCID.Get(offset))
				if !ok {
					continue
				}
				e.addCIDWidth(codeWidths, font, firstCID+offset, width*widthScale)
			}
			continue
		}

		lastCID, ok := objectInt(widthsArray.Get(i))
		if !ok {
			i++
			continue
		}
		i++
		if i >= widthsArray.Len() {
			break
		}
		width, ok := objectFloat(widthsArray.Get(i))
		i++
		if !ok || lastCID < firstCID {
			continue
		}
		for cid := firstCID; cid <= lastCID; cid++ {
			e.addCIDWidth(codeWidths, font, cid, width*widthScale)
		}
	}

	if len(codeWidths) == 0 {
		return nil
	}
	return &widthMappedFont{
		base:   font,
		widths: codeWidths,
	}
}

func (e *Evaluator) addCIDWidth(widths map[uint32]float64, font entity.Font, cid int, width float64) {
	if cid < 0 {
		return
	}
	glyphID, err := font.CharCodeToGlyph(uint32(cid))
	if err != nil {
		return
	}
	widths[glyphID] = width
}
