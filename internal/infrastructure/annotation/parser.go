// Package annotation provides annotation parser implementation.
package annotation

import (
	"fmt"
	"image"
	"strings"
	"time"

	domainannotation "github.com/dh-kam/pdf-go/internal/domain/annotation"
	domainerrors "github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Parser implements annotation parsing from PDF dictionaries.
type Parser struct{}

// NewParser creates a new annotation parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseAnnotation parses an annotation from a PDF dictionary.
func (p *Parser) ParseAnnotation(dict map[string]interface{}) (domainannotation.Annotation, error) {
	if dict == nil {
		return nil, domainerrors.Invalid("annotation_dict", nil)
	}

	// Get annotation type (required)
	subtype, ok := dict["Subtype"].(string)
	if !ok {
		return nil, domainerrors.Invalid("annotation_subtype", nil)
	}

	// Get rectangle (required)
	rectArr, ok := dict["Rect"].([]interface{})
	if !ok {
		return nil, domainerrors.Invalid("annotation_rect", nil)
	}
	rect, err := ParseRect(rectArr)
	if err != nil {
		return nil, err
	}

	// Create annotation based on type
	annotType := domainannotation.AnnotationType(subtype)

	switch annotType {
	case domainannotation.TypeLink:
		return p.parseLinkAnnotation(dict, rect)
	case domainannotation.TypeText:
		return p.parseTextAnnotation(dict, rect)
	case domainannotation.TypeWidget:
		return p.parseWidgetAnnotation(dict, rect)
	default:
		// Create a base annotation for other types
		return p.parseBaseAnnotation(dict, rect, annotType)
	}
}

// parseBaseAnnotation parses a base annotation.
func (p *Parser) parseBaseAnnotation(dict map[string]interface{}, rect image.Rectangle, annotType domainannotation.AnnotationType) (domainannotation.Annotation, error) {
	annot := NewBaseAnnotation(annotType, rect)

	// Optional fields
	if contents, ok := dict["Contents"].(string); ok {
		annot.SetContents(contents)
	}

	if name, ok := dict["NM"].(string); ok {
		annot.SetName(name)
	}

	if flags, ok := dict["F"].(float64); ok {
		annot.SetFlags(ParseFlags(int(flags)))
	}

	if modified, ok := dict["M"].(string); ok {
		if t, err := parsePDFDate(modified); err == nil {
			annot.SetModified(t)
		}
	}

	// Parse appearance if present
	if ap, ok := dict["AP"].(map[string]interface{}); ok {
		appearance, err := p.parseAppearance(ap)
		if err == nil {
			annot.SetAppearance(appearance)
		}
	}

	return annot, nil
}

// parseLinkAnnotation parses a link annotation.
func (p *Parser) parseLinkAnnotation(dict map[string]interface{}, rect image.Rectangle) (domainannotation.Annotation, error) {
	annot := NewLinkAnnotation(rect)

	// Parse base annotation fields
	baseAnnot, err := p.parseBaseAnnotation(dict, rect, domainannotation.TypeLink)
	if err != nil {
		return nil, err
	}

	// Copy base fields
	annot.SetContents(baseAnnot.Contents())
	annot.SetFlags(baseAnnot.Flags())
	annot.SetName(baseAnnot.Name())
	annot.SetModified(baseAnnot.Modified())
	annot.SetAppearance(baseAnnot.Appearance())

	// Link-specific fields
	if h, ok := dict["H"].(string); ok {
		annot.SetHighlightingMode(h)
	}

	// Parse action
	if actionDict, ok := dict["A"].(map[string]interface{}); ok {
		action := p.parseAction(actionDict)
		annot.SetAction(action)
	}

	// Parse destination (for GoTo actions)
	if dest, ok := dict["Dest"]; ok {
		action := NewGoToAction(dest)
		annot.SetAction(action)
	}

	return annot, nil
}

// parseTextAnnotation parses a text annotation.
func (p *Parser) parseTextAnnotation(dict map[string]interface{}, rect image.Rectangle) (domainannotation.Annotation, error) {
	annot := NewTextAnnotation(rect)

	// Parse base annotation fields
	baseAnnot, err := p.parseBaseAnnotation(dict, rect, domainannotation.TypeText)
	if err != nil {
		return nil, err
	}

	// Copy base fields
	annot.SetContents(baseAnnot.Contents())
	annot.SetFlags(baseAnnot.Flags())
	annot.SetName(baseAnnot.Name())
	annot.SetModified(baseAnnot.Modified())
	annot.SetAppearance(baseAnnot.Appearance())

	// Text-specific fields
	if open, ok := dict["Open"].(bool); ok {
		annot.SetOpen(open)
	}

	if icon, ok := dict["Name"].(string); ok {
		annot.SetIcon(icon)
	}

	if state, ok := dict["State"].(string); ok {
		annot.SetState(state)
	}

	if stateModel, ok := dict["StateModel"].(string); ok {
		annot.SetStateModel(stateModel)
	}

	return annot, nil
}

// parseWidgetAnnotation parses a widget annotation.
func (p *Parser) parseWidgetAnnotation(dict map[string]interface{}, rect image.Rectangle) (domainannotation.Annotation, error) {
	annot := NewWidgetAnnotation(rect)

	// Parse base annotation fields
	baseAnnot, err := p.parseBaseAnnotation(dict, rect, domainannotation.TypeWidget)
	if err != nil {
		return nil, err
	}

	// Copy base fields
	annot.SetContents(baseAnnot.Contents())
	annot.SetFlags(baseAnnot.Flags())
	annot.SetName(baseAnnot.Name())
	annot.SetModified(baseAnnot.Modified())
	annot.SetAppearance(baseAnnot.Appearance())

	// Widget-specific fields
	if fieldType, ok := dict["FT"].(string); ok {
		annot.SetFieldType(fieldType)
	}

	if value, ok := dict["V"]; ok {
		annot.SetValue(value)
	}

	if defaultValue, ok := dict["DV"]; ok {
		annot.SetDefaultValue(defaultValue)
	}

	// Parse options for choice fields
	if opt, ok := dict["Opt"].([]interface{}); ok {
		options := make([]string, 0, len(opt))
		for _, o := range opt {
			if str, ok := o.(string); ok {
				options = append(options, str)
			}
		}
		annot.SetOptions(options)
	}

	return annot, nil
}

// parseAction parses an action dictionary.
func (p *Parser) parseAction(dict map[string]interface{}) domainannotation.LinkAction {
	if dict == nil {
		return nil
	}

	// Get action type
	actionType, ok := dict["S"].(string)
	if !ok {
		return nil
	}

	switch actionType {
	case "URI":
		return p.parseURIAction(dict)
	case "GoTo":
		return p.parseGoToAction(dict)
	default:
		return NewBaseAction(actionType)
	}
}

// parseURIAction parses a URI action.
func (p *Parser) parseURIAction(dict map[string]interface{}) domainannotation.LinkAction {
	uri := ""
	if uriValue, ok := dict["URI"].(string); ok {
		uri = uriValue
	}
	return NewURIAction(uri)
}

// parseGoToAction parses a GoTo action.
func (p *Parser) parseGoToAction(dict map[string]interface{}) domainannotation.LinkAction {
	dest, _ := dict["D"]
	return NewGoToAction(dest)
}

// parseAppearance parses an appearance dictionary.
func (p *Parser) parseAppearance(dict map[string]interface{}) (domainannotation.Appearance, error) {
	appearance := NewAppearance()

	// Parse normal appearance
	if normal, ok := dict["N"]; ok {
		if stream, err := p.parseAppearanceStream(normal); err == nil {
			appearance.SetNormal(stream)
		}
	}

	// Parse rollover appearance
	if rollover, ok := dict["R"]; ok {
		if stream, err := p.parseAppearanceStream(rollover); err == nil {
			appearance.SetRollover(stream)
		}
	}

	// Parse down appearance
	if down, ok := dict["D"]; ok {
		if stream, err := p.parseAppearanceStream(down); err == nil {
			appearance.SetDown(stream)
		}
	}

	return appearance, nil
}

// parseAppearanceStream parses an appearance stream.
func (p *Parser) parseAppearanceStream(stream interface{}) (domainannotation.AppearanceStream, error) {
	// For now, create a default appearance stream
	// A full implementation would parse the XObject stream
	return NewAppearanceStream(image.Rect(0, 0, 100, 100)), nil
}

// ParseAnnotationList parses multiple annotations from an array.
func (p *Parser) ParseAnnotationList(arr []interface{}) (domainannotation.AnnotationList, error) {
	list := NewAnnotationList()

	for _, item := range arr {
		dict, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		annot, err := p.ParseAnnotation(dict)
		if err != nil {
			// Skip invalid annotations
			continue
		}

		list.Add(annot)
	}

	return list, nil
}

// parsePDFDate parses a PDF date string.
func parsePDFDate(dateStr string) (time.Time, error) {
	// PDF date format: D:YYYYMMDDHHmmSSOHH'mm'
	date := strings.TrimSpace(dateStr)
	if strings.HasPrefix(date, "D:") {
		date = date[2:]
	}
	if len(date) < 4 {
		return time.Time{}, fmt.Errorf("invalid pdf date: %s", dateStr)
	}

	year, err := parseDatePart(date, 0, 4, 0, 9999, 0, false)
	if err != nil {
		return time.Time{}, err
	}
	month, err := parseDatePart(date, 4, 2, 1, 12, 1, true)
	if err != nil {
		return time.Time{}, err
	}
	day, err := parseDatePart(date, 6, 2, 1, 31, 1, true)
	if err != nil {
		return time.Time{}, err
	}
	hour, err := parseDatePart(date, 8, 2, 0, 23, 0, true)
	if err != nil {
		return time.Time{}, err
	}
	minute, err := parseDatePart(date, 10, 2, 0, 59, 0, true)
	if err != nil {
		return time.Time{}, err
	}
	second, err := parseDatePart(date, 12, 2, 0, 59, 0, true)
	if err != nil {
		return time.Time{}, err
	}

	location := time.UTC
	if len(date) > 14 {
		location, err = parseDateTimezone(date[14:])
		if err != nil {
			return time.Time{}, err
		}
	}

	return time.Date(year, time.Month(month), day, hour, minute, second, 0, location), nil
}

func parseDatePart(value string, start, size, min, max, defaultValue int, optional bool) (int, error) {
	if len(value) < start+size {
		if optional {
			return defaultValue, nil
		}
		return 0, fmt.Errorf("invalid pdf date segment")
	}

	part := 0
	for i := 0; i < size; i++ {
		ch := value[start+i]
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid pdf date segment")
		}
		part = part*10 + int(ch-'0')
	}

	if part < min || part > max {
		return 0, fmt.Errorf("pdf date segment out of range")
	}
	return part, nil
}

func parseDateTimezone(value string) (*time.Location, error) {
	if value == "" || value == "Z" {
		return time.UTC, nil
	}

	sign := value[0]
	if sign != '+' && sign != '-' {
		return nil, fmt.Errorf("invalid pdf date timezone")
	}

	offsetHour, err := parseDatePart(value, 1, 2, 0, 23, 0, false)
	if err != nil {
		return nil, err
	}

	offsetMinute := 0
	if len(value) > 3 {
		tzRest := strings.TrimPrefix(value[3:], "'")
		tzRest = strings.TrimPrefix(tzRest, ":")
		tzRest = strings.TrimSuffix(tzRest, "'")
		if len(tzRest) >= 2 {
			offsetMinute, err = parseDatePart(tzRest, 0, 2, 0, 59, 0, false)
			if err != nil {
				return nil, err
			}
		}
	}

	offset := offsetHour*3600 + offsetMinute*60
	if sign == '-' {
		offset *= -1
	}

	return time.FixedZone("", offset), nil
}

// BaseAction provides a base implementation for actions.
type BaseAction struct {
	ActionType string
}

// NewBaseAction creates a new base action.
func NewBaseAction(actionType string) *BaseAction {
	return &BaseAction{ActionType: actionType}
}

// Type returns the action type.
func (a *BaseAction) Type() string {
	return a.ActionType
}

// URI returns the target URI (empty for non-URI actions).
func (a *BaseAction) URI() string {
	return ""
}

// Dest returns the destination (nil for non-GoTo actions).
func (a *BaseAction) Dest() interface{} {
	return nil
}

// URIAction implements a URI action.
type URIAction struct {
	BaseAction
	uri string
}

// NewURIAction creates a new URI action.
func NewURIAction(uri string) *URIAction {
	return &URIAction{
		BaseAction: BaseAction{ActionType: "URI"},
		uri:        uri,
	}
}

// URI returns the target URI.
func (a *URIAction) URI() string {
	return a.uri
}

// GoToAction implements a GoTo action.
type GoToAction struct {
	dest interface{}
	BaseAction
}

// NewGoToAction creates a new GoTo action.
func NewGoToAction(dest interface{}) *GoToAction {
	return &GoToAction{
		BaseAction: BaseAction{ActionType: "GoTo"},
		dest:       dest,
	}
}

// Dest returns the destination.
func (a *GoToAction) Dest() interface{} {
	return a.dest
}
