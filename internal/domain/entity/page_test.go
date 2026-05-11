// Package entity tests for Page functionality.
package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pdferrors "github.com/dh-kam/pdf-go/internal/domain/errors"
)

type pageContentsXRefMock struct {
	objects map[Ref]Object
	errs    map[Ref]error
}

func (m *pageContentsXRefMock) Fetch(ref Ref) (Object, error) {
	if m.errs != nil {
		if err, ok := m.errs[ref]; ok {
			return nil, err
		}
	}
	if m.objects != nil {
		if obj, ok := m.objects[ref]; ok {
			return obj, nil
		}
	}
	return nil, pdferrors.NotFoundf("xref_fetch", "object %d not found", ref.Num())
}

// TestNewPage tests the NewPage constructor.
func TestNewPage(t *testing.T) {
	t.Run("NewPage creates page with nil document", func(t *testing.T) {
		dict := NewDict()
		ref := NewRef(10, 0)
		page := NewPage(nil, dict, ref, 0)

		assert.Nil(t, page.Document())
		assert.Equal(t, dict, page.Dict())
		assert.Equal(t, ref, page.Ref())
		assert.Equal(t, 0, page.Index())
	})

	t.Run("NewPage creates page with document", func(t *testing.T) {
		doc := &Document{}
		dict := NewDict()
		ref := NewRef(20, 0)
		page := NewPage(doc, dict, ref, 5)

		assert.Equal(t, doc, page.Document())
		assert.Equal(t, dict, page.Dict())
		assert.Equal(t, ref, page.Ref())
		assert.Equal(t, 5, page.Index())
	})
}

// TestNewTestPage tests the NewTestPage constructor.
func TestNewTestPage(t *testing.T) {
	t.Run("NewTestPage creates minimal page", func(t *testing.T) {
		page := NewTestPage()
		assert.NotNil(t, page)
		assert.NotNil(t, page.Dict())
		assert.Nil(t, page.Document())
	})
}

// TestPage_Accessors tests the Page accessor methods.
func TestPage_Accessors(t *testing.T) {
	t.Run("Document returns nil for page without document", func(t *testing.T) {
		page := NewTestPage()
		assert.Nil(t, page.Document())
	})

	t.Run("Dict returns the page dictionary", func(t *testing.T) {
		dict := NewDict()
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, dict, page.Dict())
	})

	t.Run("Ref returns the page reference", func(t *testing.T) {
		ref := NewRef(10, 0)
		page := NewPage(nil, nil, ref, 0)
		assert.Equal(t, ref, page.Ref())
	})

	t.Run("Index returns the page index", func(t *testing.T) {
		page := NewPage(nil, nil, Ref{}, 42)
		assert.Equal(t, 42, page.Index())
	})
}

// TestPage_MediaBox tests the MediaBox method.
func TestPage_MediaBox(t *testing.T) {
	t.Run("MediaBox returns default US Letter when not set", func(t *testing.T) {
		page := NewTestPage()
		box := page.MediaBox()
		assert.Equal(t, [4]float64{0, 0, 612, 792}, box)
	})

	t.Run("MediaBox parses from array", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
			NewInteger(100),
			NewInteger(200),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.MediaBox()
		assert.Equal(t, [4]float64{0, 0, 100, 200}, box)
	})

	t.Run("MediaBox parses from real array", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewReal(0.0),
			NewReal(0.0),
			NewReal(8.5),
			NewReal(11.0),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.MediaBox()
		assert.InDelta(t, 0.0, box[0], 0.001)
		assert.InDelta(t, 0.0, box[1], 0.001)
		assert.InDelta(t, 8.5, box[2], 0.001)
		assert.InDelta(t, 11.0, box[3], 0.001)
	})

	t.Run("MediaBox handles mixed integer and real", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewReal(0.5),
			NewInteger(100),
			NewReal(200.5),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.MediaBox()
		assert.InDelta(t, 0.0, box[0], 0.001)
		assert.InDelta(t, 0.5, box[1], 0.001)
		assert.InDelta(t, 100.0, box[2], 0.001)
		assert.InDelta(t, 200.5, box[3], 0.001)
	})

	t.Run("MediaBox handles invalid array", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.MediaBox()
		// Should default to US Letter
		assert.Equal(t, [4]float64{0, 0, 612, 792}, box)
	})

	t.Run("MediaBox handles non-array value", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewString("invalid"))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.MediaBox()
		// Should default to US Letter
		assert.Equal(t, [4]float64{0, 0, 612, 792}, box)
	})

	t.Run("MediaBox is cached", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
			NewInteger(100),
			NewInteger(200),
		))
		page := NewPage(nil, dict, Ref{}, 0)

		// First call
		box1 := page.MediaBox()
		// Second call should return cached value
		box2 := page.MediaBox()
		assert.Equal(t, box1, box2)
	})
}

// TestPage_CropBox tests the CropBox method.
func TestPage_CropBox(t *testing.T) {
	t.Run("CropBox returns MediaBox when not set", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
			NewInteger(100),
			NewInteger(200),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.CropBox()
		assert.Equal(t, [4]float64{0, 0, 100, 200}, box)
	})

	t.Run("CropBox parses from array", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
			NewInteger(100),
			NewInteger(200),
		))
		dict.Set(NewName("CropBox"), NewArray(
			NewInteger(10),
			NewInteger(20),
			NewInteger(90),
			NewInteger(180),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.CropBox()
		assert.Equal(t, [4]float64{10, 20, 90, 180}, box)
	})

	t.Run("CropBox handles invalid array and returns MediaBox", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
			NewInteger(100),
			NewInteger(200),
		))
		dict.Set(NewName("CropBox"), NewArray(
			NewInteger(0),
			NewInteger(0),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		box := page.CropBox()
		// Should return MediaBox
		assert.Equal(t, [4]float64{0, 0, 100, 200}, box)
	})
}

// TestPage_Rotate tests the Rotate method.
func TestPage_Rotate(t *testing.T) {
	t.Run("Rotate returns 0 when not set", func(t *testing.T) {
		page := NewTestPage()
		assert.Equal(t, 0, page.Rotate())
	})

	t.Run("Rotate normalizes to 0", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(10))
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 0, page.Rotate())
	})

	t.Run("Rotate normalizes to 90", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(90))
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 90, page.Rotate())

		dict.Set(NewName("Rotate"), NewInteger(100))
		page = NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 90, page.Rotate())
	})

	t.Run("Rotate normalizes to 180", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(180))
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 180, page.Rotate())

		dict.Set(NewName("Rotate"), NewInteger(200))
		page = NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 180, page.Rotate())
	})

	t.Run("Rotate normalizes to 270", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(270))
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 270, page.Rotate())

		dict.Set(NewName("Rotate"), NewInteger(300))
		page = NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 270, page.Rotate())
	})

	t.Run("Rotate normalizes 315 to 0", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(315))
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 0, page.Rotate())
	})

	t.Run("Rotate handles negative angles", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(-90))
		page := NewPage(nil, dict, Ref{}, 0)
		assert.Equal(t, 270, page.Rotate())
	})

	t.Run("Rotate is cached", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rotate"), NewInteger(90))
		page := NewPage(nil, dict, Ref{}, 0)

		// First call
		rotate1 := page.Rotate()
		// Second call should return cached value
		rotate2 := page.Rotate()
		assert.Equal(t, rotate1, rotate2)
	})
}

// TestPage_Width tests the Width method.
func TestPage_Width(t *testing.T) {
	t.Run("Width returns correct value for US Letter", func(t *testing.T) {
		page := NewTestPage()
		width := page.Width()
		assert.InDelta(t, 612, width, 0.001)
	})

	t.Run("Width returns correct value for custom MediaBox", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(100),
			NewInteger(100),
			NewInteger(500),
			NewInteger(400),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		width := page.Width()
		assert.InDelta(t, 400, width, 0.001)
	})
}

// TestPage_Height tests the Height method.
func TestPage_Height(t *testing.T) {
	t.Run("Height returns correct value for US Letter", func(t *testing.T) {
		page := NewTestPage()
		height := page.Height()
		assert.InDelta(t, 792, height, 0.001)
	})

	t.Run("Height returns correct value for custom MediaBox", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("MediaBox"), NewArray(
			NewInteger(100),
			NewInteger(100),
			NewInteger(500),
			NewInteger(400),
		))
		page := NewPage(nil, dict, Ref{}, 0)
		height := page.Height()
		assert.InDelta(t, 300, height, 0.001)
	})
}

// TestAnnotation tests the Annotation type.
func TestAnnotation(t *testing.T) {
	t.Run("NewAnnotation creates annotation with dictionary", func(t *testing.T) {
		dict := NewDict()
		annot := NewAnnotation(dict)
		assert.Equal(t, dict, annot.Dict())
	})

	t.Run("Type returns Subtype value", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Subtype"), NewName("/Text"))
		annot := NewAnnotation(dict)
		assert.Equal(t, Name("/Text"), annot.Type())
	})

	t.Run("Type returns empty string when Subtype not set", func(t *testing.T) {
		dict := NewDict()
		annot := NewAnnotation(dict)
		assert.Equal(t, Name(""), annot.Type())
	})

	t.Run("Type returns empty string for non-Name Subtype", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Subtype"), NewString("Text"))
		annot := NewAnnotation(dict)
		assert.Equal(t, Name(""), annot.Type())
	})

	t.Run("Contents returns string value", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Contents"), NewString("Test annotation"))
		annot := NewAnnotation(dict)
		assert.Equal(t, "Test annotation", annot.Contents())
	})

	t.Run("Contents returns empty string when Contents not set", func(t *testing.T) {
		dict := NewDict()
		annot := NewAnnotation(dict)
		assert.Empty(t, annot.Contents())
	})

	t.Run("Contents returns empty string for non-String Contents", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Contents"), NewInteger(123))
		annot := NewAnnotation(dict)
		assert.Empty(t, annot.Contents())
	})

	t.Run("Rect returns array value", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rect"), NewArray(
			NewInteger(10),
			NewInteger(20),
			NewInteger(100),
			NewInteger(200),
		))
		annot := NewAnnotation(dict)
		rect := annot.Rect()
		assert.Equal(t, [4]float64{10, 20, 100, 200}, rect)
	})

	t.Run("Rect returns zero array when Rect not set", func(t *testing.T) {
		dict := NewDict()
		annot := NewAnnotation(dict)
		rect := annot.Rect()
		assert.Equal(t, [4]float64{0, 0, 0, 0}, rect)
	})

	t.Run("Rect returns zero array for invalid Rect", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rect"), NewArray(
			NewInteger(10),
			NewInteger(20),
		))
		annot := NewAnnotation(dict)
		rect := annot.Rect()
		assert.Equal(t, [4]float64{0, 0, 0, 0}, rect)
	})

	t.Run("Rect handles real values", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rect"), NewArray(
			NewReal(10.5),
			NewReal(20.5),
			NewReal(100.5),
			NewReal(200.5),
		))
		annot := NewAnnotation(dict)
		rect := annot.Rect()
		assert.InDelta(t, 10.5, rect[0], 0.001)
		assert.InDelta(t, 20.5, rect[1], 0.001)
		assert.InDelta(t, 100.5, rect[2], 0.001)
		assert.InDelta(t, 200.5, rect[3], 0.001)
	})

	t.Run("Rect handles mixed integer and real", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Rect"), NewArray(
			NewInteger(10),
			NewReal(20.5),
			NewInteger(100),
			NewReal(200.5),
		))
		annot := NewAnnotation(dict)
		rect := annot.Rect()
		assert.InDelta(t, 10.0, rect[0], 0.001)
		assert.InDelta(t, 20.5, rect[1], 0.001)
		assert.InDelta(t, 100.0, rect[2], 0.001)
		assert.InDelta(t, 200.5, rect[3], 0.001)
	})
}

// TestPage_Contents tests the Contents method.
func TestPage_Contents(t *testing.T) {
	t.Run("Contents returns empty array when not set", func(t *testing.T) {
		page := NewTestPage()
		contents, err := page.Contents()
		assert.NoError(t, err)
		assert.Empty(t, contents)
	})

	t.Run("Contents is cached", func(t *testing.T) {
		page := NewTestPage()
		contents1, err := page.Contents()
		assert.NoError(t, err)
		contents2, err := page.Contents()
		assert.NoError(t, err)
		assert.Equal(t, contents1, contents2)
	})

	t.Run("Contents single ref missing is treated as empty", func(t *testing.T) {
		contentRef := NewRef(10, 0)
		xref := &pageContentsXRefMock{
			errs: map[Ref]error{
				contentRef: pdferrors.NotFoundf("xref_fetch", "object %d not found", contentRef.Num()),
			},
		}

		doc := NewDocument(xref)
		dict := NewDict()
		dict.Set(NewName("Contents"), contentRef)
		page := NewPage(doc, dict, NewRef(1, 0), 0)

		contents, err := page.Contents()
		assert.NoError(t, err)
		assert.Empty(t, contents)
	})

	t.Run("Contents array skips missing ref and keeps valid streams", func(t *testing.T) {
		validRef := NewRef(10, 0)
		missingRef := NewRef(11, 0)
		validStream := NewStream(NewDict(), []byte("q Q"))
		inlineStream := NewStream(NewDict(), []byte("BT ET"))
		xref := &pageContentsXRefMock{
			objects: map[Ref]Object{
				validRef: validStream,
			},
			errs: map[Ref]error{
				missingRef: pdferrors.NotFoundf("xref_fetch", "object %d not found", missingRef.Num()),
			},
		}

		doc := NewDocument(xref)
		dict := NewDict()
		dict.Set(NewName("Contents"), NewArray(validRef, missingRef, inlineStream))
		page := NewPage(doc, dict, NewRef(1, 0), 0)

		contents, err := page.Contents()
		assert.NoError(t, err)
		assert.Len(t, contents, 2)
		assert.Equal(t, validStream, contents[0])
		assert.Equal(t, inlineStream, contents[1])
	})

	t.Run("Contents returns error when fetch fails with non-notfound", func(t *testing.T) {
		contentRef := NewRef(10, 0)
		xref := &pageContentsXRefMock{
			errs: map[Ref]error{
				contentRef: pdferrors.IO("xref_fetch", assert.AnError),
			},
		}

		doc := NewDocument(xref)
		dict := NewDict()
		dict.Set(NewName("Contents"), contentRef)
		page := NewPage(doc, dict, NewRef(1, 0), 0)

		contents, err := page.Contents()
		assert.Error(t, err)
		assert.Nil(t, contents)
	})
}

// TestPage_GetContent tests the GetContent method.
func TestPage_GetContent(t *testing.T) {
	t.Run("GetContent returns nil when no contents", func(t *testing.T) {
		page := NewTestPage()
		content := page.GetContent()
		assert.Nil(t, content)
	})
}

// TestPage_Annotations tests the Annotations method.
func TestPage_Annotations(t *testing.T) {
	t.Run("Annotations returns empty array when not set", func(t *testing.T) {
		page := NewTestPage()
		annots, err := page.Annotations()
		assert.NoError(t, err)
		assert.Empty(t, annots)
	})

	t.Run("Annotations is cached", func(t *testing.T) {
		page := NewTestPage()
		annots1, err := page.Annotations()
		assert.NoError(t, err)
		annots2, err := page.Annotations()
		assert.NoError(t, err)
		assert.Equal(t, annots1, annots2)
	})
}

// TestPage_Resources tests the Resources method.
func TestPage_Resources(t *testing.T) {
	t.Run("Resources returns new dict when not set", func(t *testing.T) {
		page := NewTestPage()
		resources, err := page.Resources()
		assert.NoError(t, err)
		assert.NotNil(t, resources)
	})
}
