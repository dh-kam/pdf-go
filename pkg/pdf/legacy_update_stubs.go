//nolint:revive,errcheck // Java legacy parity aliases intentionally preserve API shape and best-effort mutation behavior.
package pdf

import (
	"strconv"
	"strings"
)

// UpdateAnnotationAuthor is an exported API.
func (d *Document) UpdateAnnotationAuthor(args ...interface{}) {
	d.updateAnnotationStringKey("T", args...)
}

// UpdateAnnotationBorder is an exported API.
func (d *Document) UpdateAnnotationBorder(args ...interface{}) {
	d.updateAnnotationStringKey("Border", args...)
}

// UpdateAnnotationColorAndTransparency is an exported API.
func (d *Document) UpdateAnnotationColorAndTransparency(args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}

	if color, colorOK := legacyStringArg(args, 1); colorOK {
		d.updateAnnotationByKey(annotation, pageIndex, annotationIndex, "C", color)
	}
	if alpha, alphaOK := legacyStringArg(args, 2); alphaOK {
		d.updateAnnotationByKey(annotation, pageIndex, annotationIndex, "CA", alpha)
	}
}

// UpdateAnnotationContents is an exported API.
func (d *Document) UpdateAnnotationContents(args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	contents := annotation.Contents()
	if value, ok := legacyStringArg(args, 1); ok {
		contents = value
	}
	_ = d.SetPageAnnotationContents(pageIndex, annotationIndex, contents)
}

// UpdateAnnotationCreationDate is an exported API.
func (d *Document) UpdateAnnotationCreationDate(args ...interface{}) {
	d.updateAnnotationStringKey("CreationDate", args...)
}

// UpdateAnnotationInnerColor is an exported API.
func (d *Document) UpdateAnnotationInnerColor(args ...interface{}) {
	d.updateAnnotationStringKey("IC", args...)
}

// UpdateAnnotationInstantNoDisplay is an exported API.
func (d *Document) UpdateAnnotationInstantNoDisplay(args ...interface{}) {
	d.updateAnnotationBoolKey("NoView", args...)
}

// UpdateAnnotationLocked is an exported API.
func (d *Document) UpdateAnnotationLocked(args ...interface{}) {
	d.updateAnnotationBoolKey("Locked", args...)
}

// UpdateAnnotationModifiedDate is an exported API.
func (d *Document) UpdateAnnotationModifiedDate(args ...interface{}) {
	d.updateAnnotationStringKey("M", args...)
}

// UpdateAnnotationNM is an exported API.
func (d *Document) UpdateAnnotationNM(args ...interface{}) {
	d.updateAnnotationStringKey("NM", args...)
}

// UpdateAnnotationNameValue is an exported API.
func (d *Document) UpdateAnnotationNameValue(args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	key, keyOK := legacyStringArg(args, 1)
	if !keyOK || strings.TrimSpace(key) == "" {
		return
	}
	value, _ := legacyStringArg(args, 2)
	d.updateAnnotationByKey(annotation, pageIndex, annotationIndex, key, value)
}

// UpdateAnnotationPage is an exported API.
func (d *Document) UpdateAnnotationPage(args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	targetPageNumber, ok := legacyIntArg(args, 1)
	if !ok || !d.IsValidPage(targetPageNumber) {
		return
	}

	spec := annotationToSpec(annotation)
	if overrides, hasOverride, err := d.PageAnnotationOverrides(pageIndex); err == nil && hasOverride {
		if annotationIndex >= 0 && annotationIndex < len(overrides) {
			spec = overrides[annotationIndex]
		}
	}
	_ = d.RemovePageAnnotation(pageIndex, annotationIndex)
	_ = d.AddPageAnnotation(targetPageNumber-1, spec)
}

// UpdateAnnotationRect is an exported API.
func (d *Document) UpdateAnnotationRect(args ...interface{}) {
	_, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}

	rect, rectOK := legacyRectArg(args, 1)
	if !rectOK {
		return
	}
	_ = d.SetPageAnnotationRect(pageIndex, annotationIndex, rect)
}

// UpdateAnnotationRectWithCheck is an exported API.
func (d *Document) UpdateAnnotationRectWithCheck(args ...interface{}) {
	d.UpdateAnnotationRect(args...)
}

// UpdateAnnotationStringValue is an exported API.
func (d *Document) UpdateAnnotationStringValue(args ...interface{}) {
	d.UpdateAnnotationNameValue(args...)
}

// UpdateAnnotationSubject is an exported API.
func (d *Document) UpdateAnnotationSubject(args ...interface{}) {
	d.updateAnnotationStringKey("Subj", args...)
}

// UpdateAnnotationTextColor is an exported API.
func (d *Document) UpdateAnnotationTextColor(args ...interface{}) {
	d.updateAnnotationStringKey("TextColor", args...)
}

// UpdateAnnotationTextRotation is an exported API.
func (d *Document) UpdateAnnotationTextRotation(args ...interface{}) {
	d.updateAnnotationStringKey("Rotate", args...)
}

// UpdateAnnotationTransformAdj is an exported API.
func (d *Document) UpdateAnnotationTransformAdj(args ...interface{}) {
	d.updateAnnotationStringKey("TransformAdj", args...)
}

// UpdateAnnotationTransparency is an exported API.
func (d *Document) UpdateAnnotationTransparency(args ...interface{}) {
	d.updateAnnotationStringKey("CA", args...)
}

// UpdateAnnotationVisible is an exported API.
func (d *Document) UpdateAnnotationVisible(args ...interface{}) { d.updateAnnotationVisible(args...) }

// UpdateButtonField is an exported API.
func (d *Document) UpdateButtonField(args ...interface{}) { d.updateFormFieldValue(args...) }

// UpdateButtonFieldPush is an exported API.
func (d *Document) UpdateButtonFieldPush(args ...interface{}) { d.updateFormFieldValue(args...) }

// UpdateButtonFieldState is an exported API.
func (d *Document) UpdateButtonFieldState(args ...interface{}) { d.updateFormFieldValue(args...) }

// UpdateChoiceField is an exported API.
func (d *Document) UpdateChoiceField(args ...interface{}) { d.updateFormFieldValue(args...) }

// UpdateFieldFlagHidden is an exported API.
func (d *Document) UpdateFieldFlagHidden(args ...interface{}) { d.updateFieldFlag("Hidden", args...) }

// UpdateFieldFlagPassword is an exported API.
func (d *Document) UpdateFieldFlagPassword(args ...interface{}) {
	d.updateFieldFlag("Password", args...)
}

// UpdateFieldFlagReadOnly is an exported API.
func (d *Document) UpdateFieldFlagReadOnly(args ...interface{}) {
	d.updateFieldFlag("ReadOnly", args...)
}

// UpdateFieldFlagRequired is an exported API.
func (d *Document) UpdateFieldFlagRequired(args ...interface{}) {
	d.updateFieldFlag("Required", args...)
}

// UpdateFontStyle is an exported API.
func (d *Document) UpdateFontStyle(args ...interface{}) {
	d.updateAnnotationStringKey("FontStyle", args...)
}

// UpdateFontWeight is an exported API.
func (d *Document) UpdateFontWeight(args ...interface{}) {
	d.updateAnnotationStringKey("FontWeight", args...)
}

// UpdateFreeTextAnnotationFontSize is an exported API.
func (d *Document) UpdateFreeTextAnnotationFontSize(args ...interface{}) {
	d.updateAnnotationStringKey("FontSize", args...)
}

// UpdateInkAnnotationTransformAdj is an exported API.
func (d *Document) UpdateInkAnnotationTransformAdj(args ...interface{}) {
	d.updateAnnotationStringKey("InkTransformAdj", args...)
}

// UpdateLineAnnotationTransformAdj is an exported API.
func (d *Document) UpdateLineAnnotationTransformAdj(args ...interface{}) {
	d.updateAnnotationStringKey("LineTransformAdj", args...)
}

// UpdateMeasureAnnotationTransformAdj is an exported API.
func (d *Document) UpdateMeasureAnnotationTransformAdj(args ...interface{}) {
	d.updateAnnotationStringKey("MeasureTransformAdj", args...)
}

// UpdatePolygonAnnotationTransformAdj is an exported API.
func (d *Document) UpdatePolygonAnnotationTransformAdj(args ...interface{}) {
	d.updateAnnotationStringKey("PolygonTransformAdj", args...)
}

// UpdateTextDecoration is an exported API.
func (d *Document) UpdateTextDecoration(args ...interface{}) {
	d.updateAnnotationStringKey("TextDecoration", args...)
}

// UpdateTextField is an exported API.
func (d *Document) UpdateTextField(args ...interface{}) { d.updateFormFieldValue(args...) }

// UpdateTextQuadding is an exported API.
func (d *Document) UpdateTextQuadding(args ...interface{}) {
	if len(args) > 0 {
		if _, ok := args[0].(*Annotation); ok {
			d.updateAnnotationStringKey("Q", args...)
			return
		}
	}
	d.updateFieldMetadata("Q", args...)
}

// UpdateAnnotationUserData is an exported API.
func (d *Document) UpdateAnnotationUserData(args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}

	if len(args) >= 3 {
		key, keyOK := legacyStringArg(args, 1)
		value, valueOK := legacyStringArg(args, 2)
		if keyOK && valueOK {
			_ = d.SetPageAnnotationUserData(pageIndex, annotationIndex, key, value)
			return
		}
	}

	for key, value := range annotation.UserDataList() {
		_ = d.SetPageAnnotationUserData(pageIndex, annotationIndex, key, value)
	}
}

// UpdateInkAnnotationPoints is an exported API.
func (d *Document) UpdateInkAnnotationPoints(args ...interface{}) {
	_, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	if pathList, ok := legacyPathListArg(args, 1); ok {
		_ = d.SetPageAnnotationPathList(pageIndex, annotationIndex, pathList)
		return
	}
	if pgPoints, ok := legacyFloatSliceArg(args, 1); ok {
		_ = d.SetPageAnnotationPgPoints(pageIndex, annotationIndex, pgPoints)
	}
}

// UpdateLineAnnotationHead is an exported API.
func (d *Document) UpdateLineAnnotationHead(args ...interface{}) {
	_, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	headPoints, ok := legacyFloatSliceArg(args, 1)
	if !ok {
		return
	}
	_ = d.SetPageAnnotationHeadPoints(pageIndex, annotationIndex, headPoints)
}

// UpdatePolygonAnnotationPoints is an exported API.
func (d *Document) UpdatePolygonAnnotationPoints(args ...interface{}) {
	d.UpdateInkAnnotationPoints(args...)
}

// UpdateTextAnnotationName is an exported API.
func (d *Document) UpdateTextAnnotationName(args ...interface{}) {
	d.updateAnnotationStringKey("NM", args...)
}

// UpdateTextMarkupAnnotationPoints is an exported API.
func (d *Document) UpdateTextMarkupAnnotationPoints(args ...interface{}) {
	d.UpdateInkAnnotationPoints(args...)
}

func (d *Document) legacyAnnotationLocation(args ...interface{}) (*Annotation, int, int, bool) {
	if len(args) == 0 {
		return nil, 0, 0, false
	}
	annotation, ok := args[0].(*Annotation)
	if !ok || annotation == nil {
		return nil, 0, 0, false
	}
	pageIndex, annotationIndex, found := d.findAnnotationLocation(annotation)
	if !found {
		return nil, 0, 0, false
	}
	return annotation, pageIndex, annotationIndex, true
}

func (d *Document) updateAnnotationStringKey(key string, args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	value, valueOK := legacyStringArg(args, 1)
	if !valueOK {
		value = annotation.Contents()
	}
	d.updateAnnotationByKey(annotation, pageIndex, annotationIndex, key, value)
}

func (d *Document) updateAnnotationBoolKey(key string, args ...interface{}) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}

	value := false
	if parsed, parsedOK := legacyBoolArg(args, 1); parsedOK {
		value = parsed
	}
	d.updateAnnotationByKey(annotation, pageIndex, annotationIndex, key, strconv.FormatBool(value))
}

func (d *Document) updateAnnotationByKey(_ *Annotation, pageIndex, annotationIndex int, key, value string) {
	normalized := strings.TrimSpace(key)
	switch normalized {
	case "Contents":
		_ = d.SetPageAnnotationContents(pageIndex, annotationIndex, value)
	case "Rect":
		if rect, ok := legacyRectFromString(value); ok {
			_ = d.SetPageAnnotationRect(pageIndex, annotationIndex, rect)
		}
	default:
		_ = d.SetPageAnnotationUserData(pageIndex, annotationIndex, normalized, value)
	}
}

func (d *Document) updateFieldFlag(flag string, args ...interface{}) {
	if len(args) == 0 {
		return
	}
	fieldName, ok := legacyStringArg(args, 0)
	if !ok || strings.TrimSpace(fieldName) == "" {
		return
	}
	enabled := false
	if value, valueOK := legacyBoolArg(args, 1); valueOK {
		enabled = value
	}
	_ = d.PutUserDataString("form_flags", fieldName+"."+strings.TrimSpace(flag), strconv.FormatBool(enabled))
}

func (d *Document) updateFieldMetadata(key string, args ...interface{}) {
	if len(args) == 0 {
		return
	}
	fieldName, ok := legacyStringArg(args, 0)
	if !ok || strings.TrimSpace(fieldName) == "" {
		return
	}
	value, valueOK := legacyStringArg(args, 1)
	if !valueOK {
		if intValue, intOK := legacyIntArg(args, 1); intOK {
			value = strconv.Itoa(intValue)
		}
	}
	if strings.TrimSpace(value) == "" {
		return
	}
	_ = d.PutUserDataString("form_meta", fieldName+"."+strings.TrimSpace(key), value)
}

func (d *Document) updateAnnotationVisible(args ...interface{}) {
	_, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(args...)
	if !ok {
		return
	}
	visible := true
	if value, ok := legacyBoolArg(args, 1); ok {
		visible = value
	}
	_ = d.SetPageAnnotationUserData(pageIndex, annotationIndex, "Hidden", strconv.FormatBool(!visible))
}

func (d *Document) updateFormFieldValue(args ...interface{}) {
	if len(args) < 2 {
		return
	}
	fieldName, ok := legacyStringArg(args, 0)
	if !ok || strings.TrimSpace(fieldName) == "" {
		return
	}
	switch value := args[1].(type) {
	case string:
		_ = d.SetFormFieldValue(fieldName, value)
	case bool:
		if value {
			_ = d.SetFormFieldValue(fieldName, "On")
		} else {
			_ = d.SetFormFieldValue(fieldName, "Off")
		}
	case int:
		field, err := d.fieldByName(fieldName)
		if err != nil || value < 0 || value >= len(field.Options) {
			return
		}
		_ = d.SetFormFieldValue(fieldName, field.Options[value])
	case []int:
		if len(value) == 0 {
			return
		}
		field, err := d.fieldByName(fieldName)
		if err != nil || value[0] < 0 || value[0] >= len(field.Options) {
			return
		}
		_ = d.SetFormFieldValue(fieldName, field.Options[value[0]])
	}
}

func annotationToSpec(annotation *Annotation) AnnotationSpec {
	if annotation == nil {
		return AnnotationSpec{}
	}
	return AnnotationSpec{
		Name:       annotationName(annotation),
		Type:       annotation.Type(),
		Contents:   annotation.Contents(),
		Rect:       annotation.Rect(),
		PgPoints:   annotation.PgPoints(),
		HeadPoints: annotation.HeadPoints(),
		PathList:   annotation.PathList(),
		UserData:   annotation.UserDataList(),
	}
}

func legacyStringArg(args []interface{}, index int) (string, bool) {
	if index < 0 || index >= len(args) {
		return "", false
	}
	value, ok := args[index].(string)
	return value, ok
}

func legacyIntArg(args []interface{}, index int) (int, bool) {
	if index < 0 || index >= len(args) {
		return 0, false
	}
	value, ok := args[index].(int)
	return value, ok
}

func legacyBoolArg(args []interface{}, index int) (bool, bool) {
	if index < 0 || index >= len(args) {
		return false, false
	}
	value, ok := args[index].(bool)
	return value, ok
}

func legacyRectArg(args []interface{}, index int) ([4]float64, bool) {
	if index < 0 || index >= len(args) {
		return [4]float64{}, false
	}
	switch value := args[index].(type) {
	case [4]float64:
		return value, true
	case []float64:
		if len(value) != 4 {
			return [4]float64{}, false
		}
		return [4]float64{value[0], value[1], value[2], value[3]}, true
	case string:
		return legacyRectFromString(value)
	default:
		return [4]float64{}, false
	}
}

func legacyRectFromString(value string) ([4]float64, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return [4]float64{}, false
	}
	var rect [4]float64
	for i := range rect {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err != nil {
			return [4]float64{}, false
		}
		rect[i] = parsed
	}
	return rect, true
}

func legacyFloatSliceArg(args []interface{}, index int) ([]float64, bool) {
	if index < 0 || index >= len(args) {
		return nil, false
	}
	value, ok := args[index].([]float64)
	if !ok {
		return nil, false
	}
	return append([]float64(nil), value...), true
}

func legacyPathListArg(args []interface{}, index int) ([][]float64, bool) {
	if index < 0 || index >= len(args) {
		return nil, false
	}
	value, ok := args[index].([][]float64)
	if !ok {
		return nil, false
	}
	return clonePathList(value), true
}
