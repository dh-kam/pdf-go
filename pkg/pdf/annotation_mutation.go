package pdf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// AnnotationSpec represents editable annotation data in session scope.
type AnnotationSpec struct {
	Name       string
	UserData   map[string]string
	Type       string
	Contents   string
	PgPoints   []float64
	HeadPoints []float64
	PathList   [][]float64
	Rect       [4]float64
}

type annotationSnapshot struct {
	Name       string
	UserData   map[string]string
	Type       string
	Contents   string
	PgPoints   []float64
	HeadPoints []float64
	PathList   [][]float64
	Rect       [4]float64
}

// AddPageAnnotation appends one annotation to a page in the current session.
func (d *Document) AddPageAnnotation(pageIndex int, annotation AnnotationSpec) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}

	base, err := d.loadPageAnnotationSnapshots(sourceIndex)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	current, ok := d.annotationOverrides[sourceIndex]
	if !ok {
		current = base
	}
	current = append(current, annotationSpecToSnapshot(annotation))
	d.annotationOverrides[sourceIndex] = cloneAnnotationSnapshots(current)
	return nil
}

// RemovePageAnnotation removes one annotation by index from session page annotations.
func (d *Document) RemovePageAnnotation(pageIndex, annotationIndex int) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}

	base, err := d.loadPageAnnotationSnapshots(sourceIndex)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	current, ok := d.annotationOverrides[sourceIndex]
	if !ok {
		current = base
	}
	if annotationIndex < 0 || annotationIndex >= len(current) {
		return fmt.Errorf("annotation index out of range: %d", annotationIndex)
	}

	current = append(current[:annotationIndex], current[annotationIndex+1:]...)
	d.annotationOverrides[sourceIndex] = cloneAnnotationSnapshots(current)
	return nil
}

// ReplacePageAnnotations replaces all annotations for one page in session scope.
func (d *Document) ReplacePageAnnotations(pageIndex int, annotations []AnnotationSpec) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}

	snapshots := make([]annotationSnapshot, len(annotations))
	for i, item := range annotations {
		snapshots[i] = annotationSpecToSnapshot(item)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.annotationOverrides[sourceIndex] = cloneAnnotationSnapshots(snapshots)
	return nil
}

// SetPageAnnotationPgPoints updates one annotation point list in session scope.
func (d *Document) SetPageAnnotationPgPoints(pageIndex, annotationIndex int, pgPoints []float64) error {
	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		snapshot.PgPoints = cloneFloat64Slice(pgPoints)
		return nil
	})
}

// SetPageAnnotationType updates one annotation type in session scope.
func (d *Document) SetPageAnnotationType(pageIndex, annotationIndex int, annotationType string) error {
	annotationType = strings.TrimSpace(annotationType)
	if annotationType == "" {
		return fmt.Errorf("annotation type is required")
	}

	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		snapshot.Type = annotationType
		return nil
	})
}

// SetPageAnnotationRect updates one annotation rectangle in session scope.
func (d *Document) SetPageAnnotationRect(pageIndex, annotationIndex int, rect [4]float64) error {
	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		snapshot.Rect = rect
		return nil
	})
}

// SetPageAnnotationContents updates one annotation contents string in session scope.
func (d *Document) SetPageAnnotationContents(pageIndex, annotationIndex int, contents string) error {
	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		snapshot.Contents = contents
		return nil
	})
}

// SetPageAnnotationHeadPoints updates one annotation head point list in session scope.
func (d *Document) SetPageAnnotationHeadPoints(pageIndex, annotationIndex int, headPoints []float64) error {
	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		snapshot.HeadPoints = cloneFloat64Slice(headPoints)
		return nil
	})
}

// SetPageAnnotationPathList updates one annotation path list in session scope.
func (d *Document) SetPageAnnotationPathList(pageIndex, annotationIndex int, pathList [][]float64) error {
	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		snapshot.PathList = clonePathList(pathList)
		return nil
	})
}

// SetPageAnnotationUserData sets one user data key/value for one annotation in session scope.
func (d *Document) SetPageAnnotationUserData(pageIndex, annotationIndex int, key, value string) error {
	normalizedKey := normalizeAnnotationUserDataKey(key)
	if normalizedKey == "" {
		return fmt.Errorf("key is required")
	}

	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		if snapshot.UserData == nil {
			snapshot.UserData = make(map[string]string)
		}
		snapshot.UserData[normalizedKey] = value
		return nil
	})
}

// DeletePageAnnotationUserData removes one user data key from one annotation in session scope.
func (d *Document) DeletePageAnnotationUserData(pageIndex, annotationIndex int, key string) error {
	normalizedKey := normalizeAnnotationUserDataKey(key)
	if normalizedKey == "" {
		return fmt.Errorf("key is required")
	}

	return d.mutatePageAnnotation(pageIndex, annotationIndex, func(snapshot *annotationSnapshot) error {
		if len(snapshot.UserData) == 0 {
			return nil
		}
		delete(snapshot.UserData, normalizedKey)
		if len(snapshot.UserData) == 0 {
			snapshot.UserData = nil
		}
		return nil
	})
}

// ClearPageAnnotationOverrides clears session annotation override for one page.
func (d *Document) ClearPageAnnotationOverrides(pageIndex int) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.annotationOverrides, sourceIndex)
	return nil
}

// PageAnnotationOverrides returns annotation override data for one page.
func (d *Document) PageAnnotationOverrides(pageIndex int) ([]AnnotationSpec, bool, error) {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return nil, false, err
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	snapshots, ok := d.annotationOverrides[sourceIndex]
	if !ok {
		return nil, false, nil
	}

	out := make([]AnnotationSpec, len(snapshots))
	for i := range snapshots {
		out[i] = annotationSnapshotToSpec(snapshots[i])
	}
	return out, true, nil
}

func (d *Document) pageAnnotationOverride(sourceIndex int) ([]annotationSnapshot, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	snapshots, ok := d.annotationOverrides[sourceIndex]
	if !ok {
		return nil, false
	}
	return cloneAnnotationSnapshots(snapshots), true
}

func (d *Document) resolveSourcePageIndex(pageIndex int) (int, error) {
	if pageIndex < 0 {
		return 0, fmt.Errorf("page index out of range: %d", pageIndex)
	}

	d.mu.RLock()
	sourceIndex := pageIndex
	if len(d.pageOrder) > 0 {
		if pageIndex < 0 || pageIndex >= len(d.pageOrder) {
			d.mu.RUnlock()
			return 0, fmt.Errorf("page index out of range: %d", pageIndex)
		}
		sourceIndex = d.pageOrder[pageIndex]
		d.mu.RUnlock()
		return sourceIndex, nil
	}
	d.mu.RUnlock()

	pageCount, err := d.doc.PageCount()
	if err != nil {
		return 0, err
	}
	if pageIndex >= pageCount {
		return 0, fmt.Errorf("page index out of range: %d", pageIndex)
	}

	return sourceIndex, nil
}

func (d *Document) loadPageAnnotationSnapshots(sourceIndex int) ([]annotationSnapshot, error) {
	page, err := d.doc.GetPage(sourceIndex)
	if err != nil {
		return nil, err
	}

	annots, err := page.Annotations()
	if err != nil {
		return nil, err
	}

	out := make([]annotationSnapshot, len(annots))
	for i, annot := range annots {
		dict := annot.Dict()
		out[i] = annotationSnapshot{
			Name:       annotationStringValueFromDict(dict, "NM"),
			Type:       string(annot.Type()),
			Rect:       annot.Rect(),
			Contents:   annot.Contents(),
			PgPoints:   annotationPgPointsFromDict(dict),
			HeadPoints: annotationHeadPointsFromDict(dict),
			PathList:   annotationPathListFromDict(dict),
			UserData:   annotationUserDataFromDict(dict),
		}
	}
	return out, nil
}

func cloneAnnotationSnapshots(input []annotationSnapshot) []annotationSnapshot {
	out := make([]annotationSnapshot, len(input))
	for i := range input {
		out[i] = annotationSnapshot{
			Name:       input[i].Name,
			Type:       input[i].Type,
			Rect:       input[i].Rect,
			Contents:   input[i].Contents,
			PgPoints:   cloneFloat64Slice(input[i].PgPoints),
			HeadPoints: cloneFloat64Slice(input[i].HeadPoints),
			PathList:   clonePathList(input[i].PathList),
			UserData:   cloneStringMap(input[i].UserData),
		}
	}
	return out
}

func cloneFloat64Slice(input []float64) []float64 {
	if len(input) == 0 {
		return nil
	}
	out := make([]float64, len(input))
	copy(out, input)
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func clonePathList(input [][]float64) [][]float64 {
	if len(input) == 0 {
		return nil
	}

	out := make([][]float64, len(input))
	for i := range input {
		out[i] = cloneFloat64Slice(input[i])
	}
	return out
}

func annotationSpecToSnapshot(spec AnnotationSpec) annotationSnapshot {
	return annotationSnapshot{
		Name:       strings.TrimSpace(spec.Name),
		Type:       spec.Type,
		Rect:       spec.Rect,
		Contents:   spec.Contents,
		PgPoints:   cloneFloat64Slice(spec.PgPoints),
		HeadPoints: cloneFloat64Slice(spec.HeadPoints),
		PathList:   clonePathList(spec.PathList),
		UserData:   cloneStringMap(spec.UserData),
	}
}

func annotationSnapshotToSpec(snapshot annotationSnapshot) AnnotationSpec {
	return AnnotationSpec{
		Name:       snapshot.Name,
		Type:       snapshot.Type,
		Rect:       snapshot.Rect,
		Contents:   snapshot.Contents,
		PgPoints:   cloneFloat64Slice(snapshot.PgPoints),
		HeadPoints: cloneFloat64Slice(snapshot.HeadPoints),
		PathList:   clonePathList(snapshot.PathList),
		UserData:   cloneStringMap(snapshot.UserData),
	}
}

func (d *Document) mutatePageAnnotation(pageIndex, annotationIndex int, mutate func(snapshot *annotationSnapshot) error) error {
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return err
	}

	base, err := d.loadPageAnnotationSnapshots(sourceIndex)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	current, ok := d.annotationOverrides[sourceIndex]
	if !ok {
		current = base
	}
	if annotationIndex < 0 || annotationIndex >= len(current) {
		return fmt.Errorf("annotation index out of range: %d", annotationIndex)
	}

	target := current[annotationIndex]
	if err := mutate(&target); err != nil {
		return err
	}
	current[annotationIndex] = target
	d.annotationOverrides[sourceIndex] = cloneAnnotationSnapshots(current)
	return nil
}

func annotationPgPointsFromDict(dict *entity.Dict) []float64 {
	if dict == nil {
		return nil
	}

	for _, key := range []entity.Name{"PgPts", "L", "Vertices", "QuadPoints"} {
		if points := annotationFloatSliceFromObject(dict.Get(key)); len(points) > 0 {
			return points
		}
	}

	// Ink annotations encode points as an array of point arrays.
	if points := annotationFloatSliceFromObject(dict.Get(entity.Name("InkList"))); len(points) > 0 {
		return points
	}

	return nil
}

func annotationHeadPointsFromDict(dict *entity.Dict) []float64 {
	if dict == nil {
		return nil
	}
	return annotationFloatSliceFromObject(dict.Get(entity.Name("HeadPts")))
}

func annotationPathListFromDict(dict *entity.Dict) [][]float64 {
	if dict == nil {
		return nil
	}

	inkObj := dict.Get(entity.Name("InkList"))
	inkArray, ok := inkObj.(*entity.Array)
	if ok {
		out := make([][]float64, 0, inkArray.Len())
		for i := 0; i < inkArray.Len(); i++ {
			path := annotationFloatSliceFromObject(inkArray.Get(i))
			if len(path) > 0 {
				out = append(out, path)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	for _, key := range []entity.Name{"Vertices", "L"} {
		if path := annotationFloatSliceFromObject(dict.Get(key)); len(path) > 0 {
			return [][]float64{path}
		}
	}

	return nil
}

func annotationFloatSliceFromObject(obj entity.Object) []float64 {
	array, ok := obj.(*entity.Array)
	if !ok {
		return nil
	}
	return annotationArrayToFloatSlice(array)
}

func annotationArrayToFloatSlice(array *entity.Array) []float64 {
	if array == nil || array.Len() == 0 {
		return nil
	}

	out := make([]float64, 0, array.Len())
	for i := 0; i < array.Len(); i++ {
		item := array.Get(i)
		if nested, ok := item.(*entity.Array); ok {
			out = append(out, annotationArrayToFloatSlice(nested)...)
			continue
		}

		value, ok := annotationObjectToFloat64(item)
		if ok {
			out = append(out, value)
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func annotationObjectToFloat64(obj entity.Object) (float64, bool) {
	switch value := obj.(type) {
	case *entity.Integer:
		return float64(value.Value()), true
	case *entity.Real:
		return value.Value(), true
	default:
		return 0, false
	}
}

func annotationUserDataFromDict(dict *entity.Dict) map[string]string {
	if dict == nil {
		return nil
	}

	for _, key := range []entity.Name{"UD", "UserData", "UserDataList"} {
		if parsed := annotationStringMapFromObject(dict.Get(key)); len(parsed) > 0 {
			return parsed
		}
	}
	return nil
}

func annotationStringMapFromObject(obj entity.Object) map[string]string {
	dict, ok := obj.(*entity.Dict)
	if !ok || dict == nil {
		return nil
	}

	keys := dict.Keys()
	if len(keys) == 0 {
		return nil
	}

	out := make(map[string]string, len(keys))
	for _, key := range keys {
		normalizedKey := normalizeAnnotationUserDataKey(string(key))
		if normalizedKey == "" {
			continue
		}
		value, ok := annotationObjectToString(dict.GetRaw(key))
		if !ok {
			continue
		}
		out[normalizedKey] = value
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func annotationObjectToString(obj entity.Object) (string, bool) {
	switch value := obj.(type) {
	case *entity.String:
		return value.Value(), true
	case entity.Name:
		return string(value), true
	case *entity.Integer:
		return strconv.FormatInt(value.Value(), 10), true
	case *entity.Real:
		return strconv.FormatFloat(value.Value(), 'f', -1, 64), true
	case *entity.Boolean:
		return strconv.FormatBool(value.Value()), true
	default:
		return "", false
	}
}

func annotationStringValueFromDict(dict *entity.Dict, key string) string {
	if dict == nil {
		return ""
	}
	value, ok := annotationObjectToString(dict.Get(entity.Name(strings.TrimSpace(key))))
	if !ok {
		return ""
	}
	return value
}

func buildAnnotationUserDataDict(userData map[string]string) *entity.Dict {
	if len(userData) == 0 {
		return nil
	}

	keys := make([]string, 0, len(userData))
	for key := range userData {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := entity.NewDict()
	for _, key := range keys {
		normalizedKey := normalizeAnnotationUserDataKey(key)
		if normalizedKey == "" {
			continue
		}
		out.Set(entity.Name(normalizedKey), entity.NewString(userData[key]))
	}
	if out.Len() == 0 {
		return nil
	}
	return out
}

func normalizeAnnotationUserDataKey(key string) string {
	return strings.TrimSpace(strings.TrimPrefix(key, "/"))
}
