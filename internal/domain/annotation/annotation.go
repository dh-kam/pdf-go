// Package annotation provides PDF annotation interfaces and types.
//
//revive:disable:exported
package annotation

import (
	"image"
	"time"
)

// AnnotationType represents the type of PDF annotation.
type AnnotationType string

const (
	TypeText           AnnotationType = "Text"
	TypeLink           AnnotationType = "Link"
	TypeFreeText       AnnotationType = "FreeText"
	TypeLine           AnnotationType = "Line"
	TypeSquare         AnnotationType = "Square"
	TypeCircle         AnnotationType = "Circle"
	TypePolygon        AnnotationType = "Polygon"
	TypePolyLine       AnnotationType = "PolyLine"
	TypeHighlight      AnnotationType = "Highlight"
	TypeUnderline      AnnotationType = "Underline"
	TypeSquiggly       AnnotationType = "Squiggly"
	TypeStrikeOut      AnnotationType = "StrikeOut"
	TypeStamp          AnnotationType = "Stamp"
	TypeCaret          AnnotationType = "Caret"
	TypeInk            AnnotationType = "Ink"
	TypePopup          AnnotationType = "Popup"
	TypeFileAttachment AnnotationType = "FileAttachment"
	TypeSound          AnnotationType = "Sound"
	TypeMovie          AnnotationType = "Movie"
	TypeWidget         AnnotationType = "Widget"
	TypeScreen         AnnotationType = "Screen"
	TypePrinterMark    AnnotationType = "PrinterMark"
	TypeTrapNet        AnnotationType = "TrapNet"
	TypeWatermark      AnnotationType = "Watermark"
	Type3D             AnnotationType = "3D"
	TypeRedact         AnnotationType = "Redact"
)

// AnnotationFlags represents annotation flags.
type AnnotationFlags uint32

const (
	FlagInvisible AnnotationFlags = 1 << iota
	FlagHidden
	FlagPrint
	FlagNoZoom
	FlagNoRotate
	FlagNoView
	FlagReadOnly
	FlagLocked
	FlagToggleNoView
	FlagLockedContents
)

// Annotation represents a PDF annotation.
type Annotation interface {
	// Type returns the annotation type.
	Type() AnnotationType

	// Rect returns the annotation rectangle.
	Rect() image.Rectangle

	// Contents returns the annotation text content.
	Contents() string

	// Flags returns the annotation flags.
	Flags() AnnotationFlags

	// Name returns the annotation name (optional).
	Name() string

	// Modified returns the last modified date.
	Modified() time.Time

	// Appearance returns the appearance stream.
	Appearance() Appearance

	// SetAppearance sets the appearance stream.
	SetAppearance(Appearance)
}

// LinkAction represents the action for a link annotation.
type LinkAction interface {
	// Type returns the action type (URI, GoTo, etc.).
	Type() string

	// URI returns the target URI (for URI actions).
	URI() string

	// Dest returns the destination (for GoTo actions).
	Dest() interface{}
}

// LinkAnnotation represents a link annotation.
type LinkAnnotation interface {
	Annotation

	// HightlightingMode returns the highlighting mode.
	HighlightingMode() string

	// Action returns the link action.
	Action() LinkAction

	// SetAction sets the link action.
	SetAction(LinkAction)
}

// TextAnnotation represents a text annotation.
type TextAnnotation interface {
	Annotation

	// Open returns true if the annotation is initially open.
	Open() bool

	// Icon returns the annotation icon name.
	Icon() string

	// State returns the annotation state.
	State() string

	// StateModel returns the state model.
	StateModel() string
}

// WidgetAnnotation represents a form widget annotation.
type WidgetAnnotation interface {
	Annotation

	// FieldType returns the field type (Btn, Tx, Ch, Sig).
	FieldType() string

	// Value returns the field value.
	Value() interface{}

	// DefaultValue returns the default value.
	DefaultValue() interface{}

	// Options returns the options for choice fields.
	Options() []string
}

// Appearance represents an annotation appearance stream.
type Appearance interface {
	// Normal returns the normal appearance.
	Normal() AppearanceStream

	// Rollover returns the rollover appearance.
	Rollover() AppearanceStream

	// Down returns the down appearance.
	Down() AppearanceStream

	// SetNormal sets the normal appearance.
	SetNormal(AppearanceStream)

	// SetRollover sets the rollover appearance.
	SetRollover(AppearanceStream)

	// SetDown sets the down appearance.
	SetDown(AppearanceStream)
}

// AppearanceStream represents an appearance XObject.
type AppearanceStream interface {
	// Dictionary returns the appearance dictionary.
	Dictionary() map[string]interface{}

	// BoundingBox returns the appearance bounding box.
	BoundingBox() image.Rectangle

	// SetDictionary sets the appearance dictionary.
	SetDictionary(map[string]interface{})
}

// BorderStyle represents the border style for annotations.
type BorderStyle struct {
	Type      string
	DashArray []float64
	Width     float64
}

// AppearanceCharacteristics represents appearance characteristics.
type AppearanceCharacteristics struct {
	Border          BorderStyle
	BorderColor     []float64
	BackgroundColor []float64
	Rotation        int
}

// AnnotationList represents a list of annotations.
type AnnotationList interface {
	// Annotations returns all annotations.
	Annotations() []Annotation

	// Add adds an annotation to the list.
	Add(Annotation)

	// Remove removes an annotation from the list.
	Remove(Annotation)

	// GetByID returns an annotation by its ID.
	GetByID(id string) (Annotation, bool)

	// GetByRect returns annotations at the given rectangle.
	GetByRect(rect image.Rectangle) []Annotation

	// GetByType returns annotations of the given type.
	GetByType(typ AnnotationType) []Annotation
}

// Parser parses annotation data from PDF objects.
type Parser interface {
	// ParseAnnotation parses an annotation from a PDF dictionary.
	ParseAnnotation(dict map[string]interface{}) (Annotation, error)

	// ParseAnnotationList parses multiple annotations.
	ParseAnnotationList(arr []interface{}) (AnnotationList, error)
}

// Renderer renders annotation appearances.
type Renderer interface {
	// Render renders an annotation appearance.
	Render(Annotation) (Appearance, error)

	// RenderLink renders a link annotation appearance.
	RenderLink(LinkAnnotation) (Appearance, error)

	// RenderText renders a text annotation appearance.
	RenderText(TextAnnotation) (Appearance, error)

	// RenderWidget renders a widget annotation appearance.
	RenderWidget(WidgetAnnotation) (Appearance, error)
}
