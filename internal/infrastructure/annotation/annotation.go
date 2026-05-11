// Package annotation provides annotation implementation.
//
//revive:disable:exported
package annotation

import (
	"fmt"
	"image"
	"time"

	"github.com/dh-kam/pdf-go/internal/domain/annotation"
	domainerrors "github.com/dh-kam/pdf-go/internal/domain/errors"
)

// BaseAnnotation provides a base implementation for annotations.
type BaseAnnotation struct {
	modified   time.Time
	appearance annotation.Appearance
	annotType  annotation.AnnotationType
	contents   string
	name       string
	rect       image.Rectangle
	flags      annotation.AnnotationFlags
}

// NewBaseAnnotation creates a new base annotation.
func NewBaseAnnotation(typ annotation.AnnotationType, rect image.Rectangle) *BaseAnnotation {
	return &BaseAnnotation{
		annotType: typ,
		rect:      rect,
		modified:  time.Now(),
	}
}

// Type returns the annotation type.
func (a *BaseAnnotation) Type() annotation.AnnotationType {
	return a.annotType
}

// Rect returns the annotation rectangle.
func (a *BaseAnnotation) Rect() image.Rectangle {
	return a.rect
}

// SetRect sets the annotation rectangle.
func (a *BaseAnnotation) SetRect(rect image.Rectangle) {
	a.rect = rect
}

// Contents returns the annotation text content.
func (a *BaseAnnotation) Contents() string {
	return a.contents
}

// SetContents sets the annotation text content.
func (a *BaseAnnotation) SetContents(contents string) {
	a.contents = contents
}

// Flags returns the annotation flags.
func (a *BaseAnnotation) Flags() annotation.AnnotationFlags {
	return a.flags
}

// SetFlags sets the annotation flags.
func (a *BaseAnnotation) SetFlags(flags annotation.AnnotationFlags) {
	a.flags = flags
}

// Name returns the annotation name.
func (a *BaseAnnotation) Name() string {
	return a.name
}

// SetName sets the annotation name.
func (a *BaseAnnotation) SetName(name string) {
	a.name = name
}

// Modified returns the last modified date.
func (a *BaseAnnotation) Modified() time.Time {
	return a.modified
}

// SetModified sets the last modified date.
func (a *BaseAnnotation) SetModified(t time.Time) {
	a.modified = t
}

// Appearance returns the appearance stream.
func (a *BaseAnnotation) Appearance() annotation.Appearance {
	return a.appearance
}

// SetAppearance sets the appearance stream.
func (a *BaseAnnotation) SetAppearance(appearance annotation.Appearance) {
	a.appearance = appearance
}

// LinkAnnotation implements a link annotation.
type LinkAnnotation struct {
	action annotation.LinkAction
	*BaseAnnotation
	highlightingMode string
}

// NewLinkAnnotation creates a new link annotation.
func NewLinkAnnotation(rect image.Rectangle) *LinkAnnotation {
	return &LinkAnnotation{
		BaseAnnotation: NewBaseAnnotation(annotation.TypeLink, rect),
	}
}

// HighlightingMode returns the highlighting mode.
func (a *LinkAnnotation) HighlightingMode() string {
	return a.highlightingMode
}

// SetHighlightingMode sets the highlighting mode.
func (a *LinkAnnotation) SetHighlightingMode(mode string) {
	a.highlightingMode = mode
}

// Action returns the link action.
func (a *LinkAnnotation) Action() annotation.LinkAction {
	return a.action
}

// SetAction sets the link action.
func (a *LinkAnnotation) SetAction(action annotation.LinkAction) {
	a.action = action
}

// TextAnnotation implements a text annotation.
type TextAnnotation struct {
	*BaseAnnotation
	icon       string
	state      string
	stateModel string
	open       bool
}

// NewTextAnnotation creates a new text annotation.
func NewTextAnnotation(rect image.Rectangle) *TextAnnotation {
	return &TextAnnotation{
		BaseAnnotation: NewBaseAnnotation(annotation.TypeText, rect),
	}
}

// Open returns true if the annotation is initially open.
func (a *TextAnnotation) Open() bool {
	return a.open
}

// SetOpen sets whether the annotation is initially open.
func (a *TextAnnotation) SetOpen(open bool) {
	a.open = open
}

// Icon returns the annotation icon name.
func (a *TextAnnotation) Icon() string {
	return a.icon
}

// SetIcon sets the annotation icon name.
func (a *TextAnnotation) SetIcon(icon string) {
	a.icon = icon
}

// State returns the annotation state.
func (a *TextAnnotation) State() string {
	return a.state
}

// SetState sets the annotation state.
func (a *TextAnnotation) SetState(state string) {
	a.state = state
}

// StateModel returns the state model.
func (a *TextAnnotation) StateModel() string {
	return a.stateModel
}

// SetStateModel sets the state model.
func (a *TextAnnotation) SetStateModel(model string) {
	a.stateModel = model
}

// WidgetAnnotation implements a form widget annotation.
type WidgetAnnotation struct {
	*BaseAnnotation
	fieldType    string
	value        interface{}
	defaultValue interface{}
	options      []string
}

// NewWidgetAnnotation creates a new widget annotation.
func NewWidgetAnnotation(rect image.Rectangle) *WidgetAnnotation {
	return &WidgetAnnotation{
		BaseAnnotation: NewBaseAnnotation(annotation.TypeWidget, rect),
	}
}

// FieldType returns the field type.
func (a *WidgetAnnotation) FieldType() string {
	return a.fieldType
}

// SetFieldType sets the field type.
func (a *WidgetAnnotation) SetFieldType(fieldType string) {
	a.fieldType = fieldType
}

// Value returns the field value.
func (a *WidgetAnnotation) Value() interface{} {
	return a.value
}

// SetValue sets the field value.
func (a *WidgetAnnotation) SetValue(value interface{}) {
	a.value = value
}

// DefaultValue returns the default value.
func (a *WidgetAnnotation) DefaultValue() interface{} {
	return a.defaultValue
}

// SetDefaultValue sets the default value.
func (a *WidgetAnnotation) SetDefaultValue(defaultValue interface{}) {
	a.defaultValue = defaultValue
}

// Options returns the options for choice fields.
func (a *WidgetAnnotation) Options() []string {
	return a.options
}

// SetOptions sets the options for choice fields.
func (a *WidgetAnnotation) SetOptions(options []string) {
	a.options = options
}

// AppearanceStream implements an appearance XObject.
type AppearanceStream struct {
	dict        map[string]interface{}
	boundingBox image.Rectangle
}

// NewAppearanceStream creates a new appearance stream.
func NewAppearanceStream(bounds image.Rectangle) *AppearanceStream {
	return &AppearanceStream{
		dict:        make(map[string]interface{}),
		boundingBox: bounds,
	}
}

// Dictionary returns the appearance dictionary.
func (a *AppearanceStream) Dictionary() map[string]interface{} {
	return a.dict
}

// SetDictionary sets the appearance dictionary.
func (a *AppearanceStream) SetDictionary(dict map[string]interface{}) {
	a.dict = dict
}

// BoundingBox returns the appearance bounding box.
func (a *AppearanceStream) BoundingBox() image.Rectangle {
	return a.boundingBox
}

// SetBoundingBox sets the appearance bounding box.
func (a *AppearanceStream) SetBoundingBox(bounds image.Rectangle) {
	a.boundingBox = bounds
}

// Appearance implements annotation appearance.
type Appearance struct {
	normal   annotation.AppearanceStream
	rollover annotation.AppearanceStream
	down     annotation.AppearanceStream
}

// NewAppearance creates a new appearance.
func NewAppearance() *Appearance {
	return &Appearance{}
}

// Normal returns the normal appearance.
func (a *Appearance) Normal() annotation.AppearanceStream {
	return a.normal
}

// SetNormal sets the normal appearance.
func (a *Appearance) SetNormal(stream annotation.AppearanceStream) {
	a.normal = stream
}

// Rollover returns the rollover appearance.
func (a *Appearance) Rollover() annotation.AppearanceStream {
	return a.rollover
}

// SetRollover sets the rollover appearance.
func (a *Appearance) SetRollover(stream annotation.AppearanceStream) {
	a.rollover = stream
}

// Down returns the down appearance.
func (a *Appearance) Down() annotation.AppearanceStream {
	return a.down
}

// SetDown sets the down appearance.
func (a *Appearance) SetDown(stream annotation.AppearanceStream) {
	a.down = stream
}

// AnnotationList implements a list of annotations.
type AnnotationList struct {
	annotations []annotation.Annotation
}

// NewAnnotationList creates a new annotation list.
func NewAnnotationList() *AnnotationList {
	return &AnnotationList{
		annotations: make([]annotation.Annotation, 0),
	}
}

// Annotations returns all annotations.
func (l *AnnotationList) Annotations() []annotation.Annotation {
	return l.annotations
}

// Add adds an annotation to the list.
func (l *AnnotationList) Add(annot annotation.Annotation) {
	l.annotations = append(l.annotations, annot)
}

// Remove removes an annotation from the list.
func (l *AnnotationList) Remove(annot annotation.Annotation) {
	for i, a := range l.annotations {
		if a == annot {
			l.annotations = append(l.annotations[:i], l.annotations[i+1:]...)
			break
		}
	}
}

// GetByID returns an annotation by its ID (name).
func (l *AnnotationList) GetByID(id string) (annotation.Annotation, bool) {
	for _, a := range l.annotations {
		if a.Name() == id {
			return a, true
		}
	}
	return nil, false
}

// GetByRect returns annotations that intersect the given rectangle.
func (l *AnnotationList) GetByRect(rect image.Rectangle) []annotation.Annotation {
	var result []annotation.Annotation
	for _, a := range l.annotations {
		if rect.Overlaps(a.Rect()) {
			result = append(result, a)
		}
	}
	return result
}

// GetByType returns annotations of the given type.
func (l *AnnotationList) GetByType(typ annotation.AnnotationType) []annotation.Annotation {
	var result []annotation.Annotation
	for _, a := range l.annotations {
		if a.Type() == typ {
			result = append(result, a)
		}
	}
	return result
}

// ParseRect parses a rectangle from an array of 4 numbers.
func ParseRect(arr []interface{}) (image.Rectangle, error) {
	if len(arr) < 4 {
		return image.Rectangle{}, domainerrors.Invalid("annotation_rect", fmt.Errorf("rectangle must have 4 elements"))
	}

	var x1, y1, x2, y2 float64
	var ok bool

	if x1, ok = arr[0].(float64); !ok {
		return image.Rectangle{}, domainerrors.Invalid("annotation_rect", fmt.Errorf("invalid x1 coordinate"))
	}
	if y1, ok = arr[1].(float64); !ok {
		return image.Rectangle{}, domainerrors.Invalid("annotation_rect", fmt.Errorf("invalid y1 coordinate"))
	}
	if x2, ok = arr[2].(float64); !ok {
		return image.Rectangle{}, domainerrors.Invalid("annotation_rect", fmt.Errorf("invalid x2 coordinate"))
	}
	if y2, ok = arr[3].(float64); !ok {
		return image.Rectangle{}, domainerrors.Invalid("annotation_rect", fmt.Errorf("invalid y2 coordinate"))
	}

	return image.Rect(
		int(x1),
		int(y1),
		int(x2),
		int(y2),
	), nil
}

// ParseFlags parses annotation flags from an integer.
func ParseFlags(flags int) annotation.AnnotationFlags {
	return annotation.AnnotationFlags(flags)
}
