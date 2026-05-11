package renderer

import "github.com/dh-kam/pdf-go/internal/domain/entity"

func (e *Evaluator) applyEmbeddedSimpleFontBBoxFromDict(dict *entity.Dict, font entity.Font, fontData []byte) entity.Font {
	if dict == nil || font == nil || len(fontData) == 0 {
		return font
	}
	if nameValueForEncoding(dict.Get(entity.Name("Subtype"))) != "Type1" {
		return font
	}

	descObj := e.resolveDirectObject(dict.Get(entity.Name("FontDescriptor")))
	desc, ok := descObj.(*entity.Dict)
	if !ok {
		return font
	}

	bboxObj := e.resolveDirectObject(desc.Get(entity.Name("FontBBox")))
	bboxArr, ok := bboxObj.(*entity.Array)
	if !ok || bboxArr.Len() != 4 {
		return font
	}

	var bbox [4]float64
	for i := 0; i < 4; i++ {
		v, ok := objectFloat(bboxArr.Get(i))
		if !ok {
			return font
		}
		bbox[i] = v
	}

	if bbox[2] <= bbox[0] || bbox[3] <= bbox[1] {
		return font
	}

	return &fontBBoxOverrideFont{
		base: font,
		bbox: bbox,
	}
}
