package renderer

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func (e *Evaluator) getEmbeddedFontData(fontDict *entity.Dict) ([]byte, error) {
	descObj := fontDict.Get(entity.Name("FontDescriptor"))
	if descObj == nil {
		return nil, fmt.Errorf("font descriptor missing")
	}

	descriptor, ok := descObj.(*entity.Dict)
	if !ok {
		if ref, ok := descObj.(entity.Ref); ok && e.xref != nil {
			resolved, err := e.xref.Fetch(ref)
			if err != nil {
				return nil, err
			}
			descriptorDict, ok := resolved.(*entity.Dict)
			if !ok {
				return nil, fmt.Errorf("font descriptor is not a dictionary")
			}
			descriptor = descriptorDict
		} else {
			return nil, fmt.Errorf("font descriptor is not a dictionary")
		}
	}

	fontFileKeys := []entity.Name{
		entity.Name("FontFile"),
		entity.Name("FontFile2"),
		entity.Name("FontFile3"),
	}

	for _, key := range fontFileKeys {
		fontFileObj := descriptor.Get(key)
		if fontFileObj == nil {
			continue
		}
		fontStream, ok := e.resolveStreamObject(fontFileObj)
		if !ok {
			continue
		}
		fontData, err := fontStream.Decode()
		if err != nil {
			return nil, err
		}
		if len(fontData) == 0 {
			return nil, fmt.Errorf("embedded font stream is empty")
		}
		return fontData, nil
	}

	return nil, fmt.Errorf("font descriptor missing FontFile")
}
