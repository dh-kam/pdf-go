package renderer

import (
	"fmt"
	"testing"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

func TestDiag_GeoTopoFontName(t *testing.T) {
	doc, err := internalpdf.Open("../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	page, err := doc.GetPage(108) // page 109
	if err != nil {
		t.Fatal(err)
	}

	resources, err := page.Resources()
	if err != nil {
		t.Fatal(err)
	}

	fontObjs := resources.Get(entity.Name("Font"))
	fontDict, ok := fontObjs.(*entity.Dict)
	if !ok {
		t.Fatal("no font dict")
	}

	e := NewEvaluator(doc.XRef())
	e.SetResources(resources)

	for _, key := range fontDict.Keys() {
		if key.Value() != "/F16" {
			continue
		}
		val := fontDict.Get(key)
		var fd *entity.Dict
		if d, ok := val.(*entity.Dict); ok {
			fd = d
		} else if ref, ok := val.(entity.Ref); ok {
			obj, _ := doc.XRef().Fetch(ref)
			fd, _ = obj.(*entity.Dict)
		}
		if fd == nil {
			continue
		}
		baseFont := ""
		if n, ok := fd.Get(entity.Name("BaseFont")).(entity.Name); ok {
			baseFont = n.Value()
		}
		fmt.Printf("F16 BaseFont: %s\n", baseFont)
		
		// Check preferredFallbackFont
		preferred, ok := e.preferredFallbackFont(baseFont)
		fmt.Printf("preferredFallbackFont ok=%v font=%v\n", ok, preferred != nil)
		if preferred != nil {
			fmt.Printf("fallback font name: %s\n", preferred.Name())
		}
		
		// Check what font is returned in fallback-first mode
		t.Setenv("PDF_DEBUG_TYPE1_MODE", "fallback-first")
		font, err := e.getFontFromDict(fd, baseFont)
		if err == nil && font != nil {
			fmt.Printf("getFontFromDict name=%s isCID=%v\n", font.Name(), font.IsCIDFont())
			// Check slash (code 47) and one (code 49) rendering
			for _, code := range []uint32{47, 49} {
				glyph, gerr := font.CharCodeToGlyph(code)
				if gerr != nil {
					fmt.Printf("  code %d: CharCodeToGlyph error: %v\n", code, gerr)
					continue
				}
				path, perr := font.RenderGlyph(glyph, 1000)
				if perr != nil {
					fmt.Printf("  code %d glyph %d: RenderGlyph error: %v\n", code, glyph, perr)
					continue
				}
				if path == nil {
					fmt.Printf("  code %d glyph %d: path nil\n", code, glyph)
					continue
				}
				w := path.Bounds[2] - path.Bounds[0]
				h := path.Bounds[3] - path.Bounds[1]
				area := w * h
				total := 0
				for range path.Commands {
					total++
				}
				fmt.Printf("  code %d glyph %d: bounds=[%.1f,%.1f,%.1f,%.1f] area=%.1f segs=%d density=%.6f\n",
					code, glyph, path.Bounds[0], path.Bounds[1], path.Bounds[2], path.Bounds[3], area, total, float64(total)/area)
			}
		}
	}
}
