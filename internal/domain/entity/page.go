// Package entity defines PDF page entities.
package entity

import (
	stderrors "errors"

	pdferrors "github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Page represents a PDF page.
type Page struct {
	resources   *Dict
	dict        *Dict
	doc         *Document
	annotations []*Annotation
	contents    []Object
	mediaBox    [4]float64
	cropBox     [4]float64
	index       int
	rotate      int
	ref         Ref
}

// NewPage creates a new Page.
func NewPage(doc *Document, dict *Dict, ref Ref, index int) *Page {
	return &Page{
		doc:   doc,
		dict:  dict,
		ref:   ref,
		index: index,
	}
}

// NewTestPage creates a minimal page for testing purposes.
func NewTestPage() *Page {
	return &Page{
		dict:     NewDict(),
		contents: []Object{},
	}
}

// Document returns the parent document.
func (p *Page) Document() *Document {
	return p.doc
}

// Dict returns the page dictionary.
func (p *Page) Dict() *Dict {
	return p.dict
}

// Ref returns the indirect reference to this page.
func (p *Page) Ref() Ref {
	return p.ref
}

// Index returns the page index (0-based).
func (p *Page) Index() int {
	return p.index
}

// MediaBox returns the media box rectangle [x1, y1, x2, y2].
func (p *Page) MediaBox() [4]float64 {
	if p.mediaBox[0] == 0 && p.mediaBox[1] == 0 && p.mediaBox[2] == 0 && p.mediaBox[3] == 0 {
		p.parseMediaBox()
	}
	return p.mediaBox
}

// parseMediaBox parses the media box from the page dictionary.
func (p *Page) parseMediaBox() {
	mediaBoxVal := p.inheritedValue(Name("MediaBox"))
	if mediaBoxVal == nil {
		// Default to US Letter size (in points)
		p.mediaBox = [4]float64{0, 0, 612, 792}
		return
	}

	mediaBoxArr, ok := mediaBoxVal.(*Array)
	if !ok || mediaBoxArr.Len() != 4 {
		// Default to US Letter size
		p.mediaBox = [4]float64{0, 0, 612, 792}
		return
	}

	for i := 0; i < 4; i++ {
		elem := mediaBoxArr.Get(i)
		switch v := elem.(type) {
		case *Integer:
			p.mediaBox[i] = float64(v.Value())
		case *Real:
			p.mediaBox[i] = v.Value()
		default:
			p.mediaBox[i] = 0
		}
	}
}

// CropBox returns the crop box rectangle.
func (p *Page) CropBox() [4]float64 {
	if p.cropBox[0] == 0 && p.cropBox[1] == 0 && p.cropBox[2] == 0 && p.cropBox[3] == 0 {
		p.parseCropBox()
	}
	if p.cropBox[0] == 0 && p.cropBox[1] == 0 && p.cropBox[2] == 0 && p.cropBox[3] == 0 {
		return p.MediaBox()
	}
	return p.cropBox
}

// parseCropBox parses the crop box from the page dictionary.
func (p *Page) parseCropBox() {
	cropBoxVal := p.inheritedValue(Name("CropBox"))
	if cropBoxVal == nil {
		return
	}

	cropBoxArr, ok := cropBoxVal.(*Array)
	if !ok || cropBoxArr.Len() != 4 {
		return
	}

	for i := 0; i < 4; i++ {
		elem := cropBoxArr.Get(i)
		switch v := elem.(type) {
		case *Integer:
			p.cropBox[i] = float64(v.Value())
		case *Real:
			p.cropBox[i] = v.Value()
		}
	}
}

// Rotate returns the page rotation in degrees.
func (p *Page) Rotate() int {
	if p.rotate == 0 {
		p.parseRotate()
	}
	return p.rotate
}

// parseRotate parses the rotation from the page dictionary.
func (p *Page) parseRotate() {
	rotateVal := p.inheritedValue(Name("Rotate"))
	if rotateVal == nil {
		return
	}

	rotate, ok := rotateVal.(*Integer)
	if !ok {
		return
	}

	// Normalize to 0, 90, 180, 270
	angle := int(rotate.Value()) % 360
	if angle < 0 {
		angle += 360
	}

	// Round to nearest 90 degrees
	switch {
	case angle < 45:
		p.rotate = 0
	case angle < 135:
		p.rotate = 90
	case angle < 225:
		p.rotate = 180
	case angle < 315:
		p.rotate = 270
	default:
		p.rotate = 0
	}
}

// Resources returns the page resources dictionary.
func (p *Page) Resources() (*Dict, error) {
	if p.resources == nil {
		resourcesVal := p.inheritedValue(Name("Resources"))
		if resourcesVal == nil {
			p.resources = NewDict()
			return p.resources, nil
		}

		switch v := resourcesVal.(type) {
		case Ref:
			obj, err := p.doc.XRef().Fetch(v)
			if err != nil {
				return nil, pdferrors.IO("page_resources", err)
			}
			resources, ok := obj.(*Dict)
			if !ok {
				return nil, pdferrors.Invalid("page_resources", nil)
			}
			p.resources = resources
		case *Dict:
			p.resources = v
		default:
			p.resources = NewDict()
		}
	}

	return p.resources, nil
}

// inheritedValue resolves page tree inheritable attributes from the page
// dictionary and its parent /Pages nodes.
func (p *Page) inheritedValue(key Name) Object {
	if p == nil || p.dict == nil {
		return nil
	}

	cur := p.dict
	seen := map[*Dict]struct{}{}

	for cur != nil {
		if _, ok := seen[cur]; ok {
			return nil
		}
		seen[cur] = struct{}{}

		if v := cur.Get(key); v != nil {
			return v
		}

		parentVal := cur.Get(Name("Parent"))
		if parentVal == nil {
			return nil
		}

		switch v := parentVal.(type) {
		case *Dict:
			cur = v
		case Ref:
			if p.doc == nil || p.doc.XRef() == nil {
				return nil
			}
			obj, err := p.doc.XRef().Fetch(v)
			if err != nil {
				return nil
			}
			parentDict, ok := obj.(*Dict)
			if !ok {
				return nil
			}
			cur = parentDict
		default:
			return nil
		}
	}

	return nil
}

// Contents returns the page content streams.
func (p *Page) Contents() ([]Object, error) {
	if p.contents == nil {
		contentsVal := p.dict.Get(Name("Contents"))
		if contentsVal == nil {
			p.contents = []Object{}
			return p.contents, nil
		}

		switch v := contentsVal.(type) {
		case Ref:
			obj, err := p.doc.XRef().Fetch(v)
			if err != nil {
				// Keep rendering best-effort when one content stream ref is stale.
				if isNotFoundFetchError(err) {
					p.contents = []Object{}
					return p.contents, nil
				}
				return nil, pdferrors.IO("page_contents", err)
			}
			p.contents = []Object{obj}
		case *Stream:
			p.contents = []Object{v}
		case *Array:
			p.contents = make([]Object, 0, v.Len())
			for i := 0; i < v.Len(); i++ {
				elem := v.Get(i)
				if ref, ok := elem.(Ref); ok {
					obj, err := p.doc.XRef().Fetch(ref)
					if err != nil {
						if isNotFoundFetchError(err) {
							continue
						}
						return nil, pdferrors.IO("page_contents", err)
					}
					p.contents = append(p.contents, obj)
				} else {
					p.contents = append(p.contents, elem)
				}
			}
		case *Dict:
			p.contents = []Object{v}
		default:
			p.contents = []Object{}
		}
	}

	return p.contents, nil
}

func isNotFoundFetchError(err error) bool {
	var pdfErr *pdferrors.PDFError
	return stderrors.As(err, &pdfErr) && pdfErr.Type == pdferrors.ErrTypeNotFound
}

// GetContent returns the first content stream for the page.
// Returns nil if there is no content.
func (p *Page) GetContent() Object {
	contents, err := p.Contents()
	if err != nil || len(contents) == 0 {
		return nil
	}
	return contents[0]
}

// Annotations returns the page annotations.
func (p *Page) Annotations() ([]*Annotation, error) {
	if p.annotations == nil {
		annotsVal := p.dict.Get(Name("Annots"))
		if annotsVal == nil {
			p.annotations = []*Annotation{}
			return p.annotations, nil
		}

		annotsArray, ok := annotsVal.(*Array)
		if !ok {
			p.annotations = []*Annotation{}
			return p.annotations, nil
		}

		p.annotations = make([]*Annotation, 0, annotsArray.Len())
		for i := 0; i < annotsArray.Len(); i++ {
			elem := annotsArray.Get(i)
			var annotDict *Dict

			switch v := elem.(type) {
			case Ref:
				obj, err := p.doc.XRef().Fetch(v)
				if err != nil {
					continue
				}
				annotDict, ok = obj.(*Dict)
				if !ok {
					continue
				}
			case *Dict:
				annotDict = v
			default:
				continue
			}

			p.annotations = append(p.annotations, NewAnnotation(annotDict))
		}
	}

	return p.annotations, nil
}

// Width returns the page width in points.
func (p *Page) Width() float64 {
	box := p.MediaBox()
	return box[2] - box[0]
}

// Height returns the page height in points.
func (p *Page) Height() float64 {
	box := p.MediaBox()
	return box[3] - box[1]
}

// Annotation represents a PDF annotation.
type Annotation struct {
	dict *Dict
}

// NewAnnotation creates a new Annotation.
func NewAnnotation(dict *Dict) *Annotation {
	return &Annotation{dict: dict}
}

// Dict returns the annotation dictionary.
func (a *Annotation) Dict() *Dict {
	return a.dict
}

// Type returns the annotation type.
func (a *Annotation) Type() Name {
	typeVal := a.dict.Get(Name("Subtype"))
	if typeVal == nil {
		return ""
	}
	if name, ok := typeVal.(Name); ok {
		return name
	}
	return ""
}

// Contents returns the annotation contents.
func (a *Annotation) Contents() string {
	contentsVal := a.dict.Get(Name("Contents"))
	if contentsVal == nil {
		return ""
	}
	if str, ok := contentsVal.(*String); ok {
		return str.Value()
	}
	return ""
}

// Rect returns the annotation rectangle [x1, y1, x2, y2].
func (a *Annotation) Rect() [4]float64 {
	rectVal := a.dict.Get(Name("Rect"))
	if rectVal == nil {
		return [4]float64{}
	}

	rectArr, ok := rectVal.(*Array)
	if !ok || rectArr.Len() != 4 {
		return [4]float64{}
	}

	var rect [4]float64
	for i := 0; i < 4; i++ {
		elem := rectArr.Get(i)
		switch v := elem.(type) {
		case *Integer:
			rect[i] = float64(v.Value())
		case *Real:
			rect[i] = v.Value()
		}
	}

	return rect
}
