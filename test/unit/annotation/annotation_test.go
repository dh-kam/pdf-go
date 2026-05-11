// Package annotation_test provides tests for PDF annotations.
package annotation_test

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainannotation "github.com/dh-kam/pdf-go/internal/domain/annotation"
	"github.com/dh-kam/pdf-go/internal/infrastructure/annotation"
)

func TestNewBaseAnnotation(t *testing.T) {
	rect := image.Rect(10, 10, 100, 100)
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, rect)

	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeText, annot.Type())
	assert.Equal(t, rect, annot.Rect())
}

func TestBaseAnnotation_SetContents(t *testing.T) {
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	annot.SetContents("Test content")

	assert.Equal(t, "Test content", annot.Contents())
}

func TestBaseAnnotation_SetFlags(t *testing.T) {
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	flags := domainannotation.FlagHidden | domainannotation.FlagPrint
	annot.SetFlags(flags)

	assert.Equal(t, flags, annot.Flags())
}

func TestBaseAnnotation_SetName(t *testing.T) {
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	annot.SetName("annot1")

	assert.Equal(t, "annot1", annot.Name())
}

func TestBaseAnnotation_SetAppearance(t *testing.T) {
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	appearance := annotation.NewAppearance()
	annot.SetAppearance(appearance)

	assert.Equal(t, appearance, annot.Appearance())
}

func TestNewLinkAnnotation(t *testing.T) {
	rect := image.Rect(10, 10, 100, 50)
	annot := annotation.NewLinkAnnotation(rect)

	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeLink, annot.Type())
	assert.Equal(t, rect, annot.Rect())
	assert.Nil(t, annot.Action())
}

func TestLinkAnnotation_SetAction(t *testing.T) {
	rect := image.Rect(10, 10, 100, 50)
	annot := annotation.NewLinkAnnotation(rect)

	action := annotation.NewURIAction("https://example.com")
	annot.SetAction(action)

	assert.Equal(t, action, annot.Action())
	assert.Equal(t, "URI", action.Type())
	assert.Equal(t, "https://example.com", action.URI())
}

func TestLinkAnnotation_HighlightingMode(t *testing.T) {
	annot := annotation.NewLinkAnnotation(image.Rect(0, 0, 100, 50))
	annot.SetHighlightingMode("I")

	assert.Equal(t, "I", annot.HighlightingMode())
}

func TestNewTextAnnotation(t *testing.T) {
	rect := image.Rect(10, 10, 100, 100)
	annot := annotation.NewTextAnnotation(rect)

	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeText, annot.Type())
	assert.Equal(t, rect, annot.Rect())
	assert.False(t, annot.Open())
}

func TestTextAnnotation_SetOpen(t *testing.T) {
	annot := annotation.NewTextAnnotation(image.Rect(0, 0, 100, 100))
	annot.SetOpen(true)

	assert.True(t, annot.Open())
}

func TestTextAnnotation_Icon(t *testing.T) {
	annot := annotation.NewTextAnnotation(image.Rect(0, 0, 100, 100))
	annot.SetIcon("Comment")

	assert.Equal(t, "Comment", annot.Icon())
}

func TestTextAnnotation_State(t *testing.T) {
	annot := annotation.NewTextAnnotation(image.Rect(0, 0, 100, 100))
	annot.SetState("Marked")

	assert.Equal(t, "Marked", annot.State())
}

func TestNewWidgetAnnotation(t *testing.T) {
	rect := image.Rect(10, 10, 100, 30)
	annot := annotation.NewWidgetAnnotation(rect)

	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeWidget, annot.Type())
	assert.Equal(t, rect, annot.Rect())
}

func TestWidgetAnnotation_FieldType(t *testing.T) {
	annot := annotation.NewWidgetAnnotation(image.Rect(0, 0, 100, 30))
	annot.SetFieldType("Tx")

	assert.Equal(t, "Tx", annot.FieldType())
}

func TestWidgetAnnotation_Value(t *testing.T) {
	annot := annotation.NewWidgetAnnotation(image.Rect(0, 0, 100, 30))
	annot.SetValue("test value")

	assert.Equal(t, "test value", annot.Value())
}

func TestWidgetAnnotation_Options(t *testing.T) {
	annot := annotation.NewWidgetAnnotation(image.Rect(0, 0, 100, 30))
	options := []string{"Option1", "Option2", "Option3"}
	annot.SetOptions(options)

	assert.Equal(t, options, annot.Options())
}

func TestNewAppearance(t *testing.T) {
	appearance := annotation.NewAppearance()

	assert.NotNil(t, appearance)
	assert.Nil(t, appearance.Normal())
	assert.Nil(t, appearance.Rollover())
	assert.Nil(t, appearance.Down())
}

func TestAppearance_SetNormal(t *testing.T) {
	appearance := annotation.NewAppearance()
	stream := annotation.NewAppearanceStream(image.Rect(0, 0, 100, 100))
	appearance.SetNormal(stream)

	assert.Equal(t, stream, appearance.Normal())
}

func TestAppearance_SetRollover(t *testing.T) {
	appearance := annotation.NewAppearance()
	stream := annotation.NewAppearanceStream(image.Rect(0, 0, 100, 100))
	appearance.SetRollover(stream)

	assert.Equal(t, stream, appearance.Rollover())
}

func TestAppearance_SetDown(t *testing.T) {
	appearance := annotation.NewAppearance()
	stream := annotation.NewAppearanceStream(image.Rect(0, 0, 100, 100))
	appearance.SetDown(stream)

	assert.Equal(t, stream, appearance.Down())
}

func TestNewAppearanceStream(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	stream := annotation.NewAppearanceStream(bounds)

	assert.NotNil(t, stream)
	assert.Equal(t, bounds, stream.BoundingBox())
	assert.NotNil(t, stream.Dictionary())
}

func TestAppearanceStream_Dictionary(t *testing.T) {
	stream := annotation.NewAppearanceStream(image.Rect(0, 0, 100, 100))
	dict := map[string]interface{}{
		"Subtype": "Form",
		"BBox":    []interface{}{0.0, 0.0, 100.0, 100.0},
	}
	stream.SetDictionary(dict)

	assert.Equal(t, dict, stream.Dictionary())
}

func TestNewAnnotationList(t *testing.T) {
	list := annotation.NewAnnotationList()

	assert.NotNil(t, list)
	assert.Empty(t, list.Annotations())
}

func TestAnnotationList_Add(t *testing.T) {
	list := annotation.NewAnnotationList()
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))

	list.Add(annot)

	assert.Len(t, list.Annotations(), 1)
	assert.Equal(t, annot, list.Annotations()[0])
}

func TestAnnotationList_Remove(t *testing.T) {
	list := annotation.NewAnnotationList()
	annot := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))

	list.Add(annot)
	assert.Len(t, list.Annotations(), 1)

	list.Remove(annot)
	assert.Empty(t, list.Annotations())
}

func TestAnnotationList_GetByID(t *testing.T) {
	list := annotation.NewAnnotationList()
	annot1 := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	annot1.SetName("annot1")
	annot2 := annotation.NewBaseAnnotation(domainannotation.TypeLink, image.Rect(0, 0, 100, 100))
	annot2.SetName("annot2")

	list.Add(annot1)
	list.Add(annot2)

	// Test getting existing annotation
	found, ok := list.GetByID("annot1")
	assert.True(t, ok)
	assert.Equal(t, annot1, found)

	// Test getting non-existing annotation
	_, ok = list.GetByID("annot3")
	assert.False(t, ok)
}

func TestAnnotationList_GetByRect(t *testing.T) {
	list := annotation.NewAnnotationList()
	annot1 := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	annot2 := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(200, 200, 300, 300))
	annot3 := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(50, 50, 150, 150))

	list.Add(annot1)
	list.Add(annot2)
	list.Add(annot3)

	// Test overlapping rectangle
	results := list.GetByRect(image.Rect(25, 25, 75, 75))
	assert.Len(t, results, 2) // annot1 and annot3

	// Test non-overlapping rectangle
	results = list.GetByRect(image.Rect(400, 400, 500, 500))
	assert.Empty(t, results)
}

func TestAnnotationList_GetByType(t *testing.T) {
	list := annotation.NewAnnotationList()
	annot1 := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))
	annot2 := annotation.NewBaseAnnotation(domainannotation.TypeLink, image.Rect(0, 0, 100, 100))
	annot3 := annotation.NewBaseAnnotation(domainannotation.TypeText, image.Rect(0, 0, 100, 100))

	list.Add(annot1)
	list.Add(annot2)
	list.Add(annot3)

	results := list.GetByType(domainannotation.TypeText)
	assert.Len(t, results, 2)

	results = list.GetByType(domainannotation.TypeLink)
	assert.Len(t, results, 1)

	results = list.GetByType(domainannotation.TypeWidget)
	assert.Empty(t, results)
}

func TestParseRect(t *testing.T) {
	tests := []struct {
		name    string
		input   []interface{}
		want    image.Rectangle
		wantErr bool
	}{
		{
			name:  "valid rectangle",
			input: []interface{}{10.0, 20.0, 100.0, 200.0},
			want:  image.Rect(10, 20, 100, 200),
		},
		{
			name:  "rectangle with integers",
			input: []interface{}{float64(10), float64(20), float64(100), float64(200)},
			want:  image.Rect(10, 20, 100, 200),
		},
		{
			name:    "too few elements",
			input:   []interface{}{10.0, 20.0, 100.0},
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   []interface{}{10.0, "invalid", 100.0, 200.0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := annotation.ParseRect(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	flags := annotation.ParseFlags(0x04)
	assert.Equal(t, domainannotation.FlagPrint, flags)

	flags = annotation.ParseFlags(0x14) // Hidden | Print
	assert.Equal(t, domainannotation.AnnotationFlags(0x14), flags)
}

func TestNewParser(t *testing.T) {
	parser := annotation.NewParser()
	assert.NotNil(t, parser)
}

func TestParser_ParseAnnotation_Link(t *testing.T) {
	parser := annotation.NewParser()

	dict := map[string]interface{}{
		"Subtype": "Link",
		"Rect":    []interface{}{10.0, 20.0, 100.0, 50.0},
		"URI":     "https://example.com",
	}

	annot, err := parser.ParseAnnotation(dict)
	require.NoError(t, err)
	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeLink, annot.Type())
}

func TestParser_ParseAnnotation_Text(t *testing.T) {
	parser := annotation.NewParser()

	dict := map[string]interface{}{
		"Subtype":  "Text",
		"Rect":     []interface{}{10.0, 20.0, 100.0, 100.0},
		"Contents": "This is a comment",
		"Open":     true,
		"Name":     "Comment",
	}

	annot, err := parser.ParseAnnotation(dict)
	require.NoError(t, err)
	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeText, annot.Type())

	// Try to cast to TextAnnotation
	if textAnnot, ok := annot.(*annotation.TextAnnotation); ok {
		assert.True(t, textAnnot.Open())
		assert.Equal(t, "Comment", textAnnot.Icon())
		assert.Equal(t, "This is a comment", textAnnot.Contents())
	} else {
		require.Fail(t, "Expected TextAnnotation")
	}
}

func TestParser_ParseAnnotation_Widget(t *testing.T) {
	parser := annotation.NewParser()

	dict := map[string]interface{}{
		"Subtype": "Widget",
		"Rect":    []interface{}{10.0, 20.0, 100.0, 30.0},
		"FT":      "Tx",
		"V":       "default value",
	}

	annot, err := parser.ParseAnnotation(dict)
	require.NoError(t, err)
	assert.NotNil(t, annot)
	assert.Equal(t, domainannotation.TypeWidget, annot.Type())

	if widgetAnnot, ok := annot.(*annotation.WidgetAnnotation); ok {
		assert.Equal(t, "Tx", widgetAnnot.FieldType())
		assert.Equal(t, "default value", widgetAnnot.Value())
	} else {
		require.Fail(t, "Expected WidgetAnnotation")
	}
}

func TestParser_ParseAnnotationList(t *testing.T) {
	parser := annotation.NewParser()

	arr := []interface{}{
		map[string]interface{}{
			"Subtype": "Text",
			"Rect":    []interface{}{10.0, 20.0, 100.0, 100.0},
		},
		map[string]interface{}{
			"Subtype": "Link",
			"Rect":    []interface{}{200.0, 200.0, 300.0, 250.0},
		},
	}

	list, err := parser.ParseAnnotationList(arr)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Len(t, list.Annotations(), 2)
}

func TestParser_ParseAnnotation_Invalid(t *testing.T) {
	parser := annotation.NewParser()

	// No subtype
	dict := map[string]interface{}{
		"Rect": []interface{}{10.0, 20.0, 100.0, 50.0},
	}

	_, err := parser.ParseAnnotation(dict)
	assert.Error(t, err)
}

func TestParser_ParseAnnotation_NoRect(t *testing.T) {
	parser := annotation.NewParser()

	dict := map[string]interface{}{
		"Subtype": "Link",
	}

	_, err := parser.ParseAnnotation(dict)
	assert.Error(t, err)
}

func TestURIAction(t *testing.T) {
	action := annotation.NewURIAction("https://example.com")

	assert.Equal(t, "URI", action.Type())
	assert.Equal(t, "https://example.com", action.URI())
	assert.Nil(t, action.Dest())
}

func TestGoToAction(t *testing.T) {
	dest := []interface{}{1, "XYZ", 0, 100, 0}
	action := annotation.NewGoToAction(dest)

	assert.Equal(t, "GoTo", action.Type())
	assert.Equal(t, dest, action.Dest())
	assert.Equal(t, "", action.URI())
}

func TestBaseAction(t *testing.T) {
	action := annotation.NewBaseAction("Custom")

	assert.Equal(t, "Custom", action.Type())
	assert.Equal(t, "", action.URI())
	assert.Nil(t, action.Dest())
}
