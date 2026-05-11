package pdf

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// AnnotFindAnnot is a Java-parity compatibility helper.
func (d *Document) AnnotFindAnnot(refNo int) int {
	if refNo < 0 {
		return -1
	}
	return refNo
}

// AnnotGetAppearanceName returns the selected appearance state name.
func (d *Document) AnnotGetAppearanceName(annotation *Annotation) string {
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return ""
	}
	if as, ok := annotationObjectToString(dict.Get(entity.Name("AS"))); ok {
		return as
	}
	return ""
}

// AnnotGetRefNumFromNm returns annotation index matched by NM name in one page.
// It accepts a 1-based page number.
func (d *Document) AnnotGetRefNumFromNm(pageNumber int, name string) int {
	_, index, err := d.annotationByPageAndName(pageNumber, name)
	if err != nil {
		return -1
	}
	return index
}

// AnnotHasAppearance reports whether annotation has appearance dictionary.
func (d *Document) AnnotHasAppearance(annotation *Annotation, appearanceName string) bool {
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return false
	}
	ap := dict.Get(entity.Name("AP"))
	apDict, ok := ap.(*entity.Dict)
	if !ok || apDict == nil {
		return false
	}
	if strings.TrimSpace(appearanceName) == "" {
		return apDict.Len() > 0
	}
	return apDict.Get(entity.Name(strings.TrimSpace(appearanceName))) != nil
}

// AnnotLockAnnotsInPageSL returns annotation count in one page.
// It accepts a 1-based page number.
func (d *Document) AnnotLockAnnotsInPageSL(pageNumber int) int {
	if !d.IsValidPage(pageNumber) {
		return -1
	}
	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil {
		return -1
	}
	return len(annots)
}

// AnnotLockSyncAnnotsInPageSL validates page for synchronized annotation operations.
func (d *Document) AnnotLockSyncAnnotsInPageSL(pageNumber int) bool {
	return d.IsValidPage(pageNumber)
}

// AnnotSelectAppearance updates selected appearance metadata for one annotation.
func (d *Document) AnnotSelectAppearance(annotation *Annotation, appearanceName string, selected bool) int {
	if annotation == nil {
		return -1
	}
	value := strings.TrimSpace(appearanceName)
	if value == "" {
		value = "N"
	}
	if !selected {
		value = ""
	}
	if d.setAnnotationUserData(annotation, "appearance_name", value) {
		return 0
	}
	return -1
}

// AnnotSetEditable stores editable metadata for one annotation.
func (d *Document) AnnotSetEditable(annotation *Annotation, editable bool) int {
	if annotation == nil {
		return -1
	}
	if d.setAnnotationUserData(annotation, "editable", strconv.FormatBool(editable)) {
		return 0
	}
	return -1
}

// AnnotUnlockAnnotsInPageSL clears annotation-lock metadata in session scope.
func (d *Document) AnnotUnlockAnnotsInPageSL() int {
	pageCount, err := d.PageCount()
	if err != nil {
		return -1
	}
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		annots, annotErr := d.GetPageAnnotations(pageIndex)
		if annotErr != nil {
			continue
		}
		for annotationIndex := range annots {
			if err := d.DeletePageAnnotationUserData(pageIndex, annotationIndex, "locked"); err != nil {
				continue
			}
		}
	}
	return 0
}

// FindUserAnnotationPage finds one page containing the given annotation index.
// It returns a 1-based page number, or 0 when not found.
func (d *Document) FindUserAnnotationPage(annotationIndex int) int {
	if annotationIndex < 0 {
		return 0
	}
	pageCount, err := d.PageCount()
	if err != nil {
		return 0
	}
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		annots, annotErr := d.GetPageAnnotations(pageIndex)
		if annotErr != nil {
			return 0
		}
		if annotationIndex < len(annots) {
			return pageIndex + 1
		}
	}
	return 0
}

// GetAnnotation returns one annotation by page number and name (NM) or numeric index text.
// It accepts a 1-based page number.
func (d *Document) GetAnnotation(pageNumber int, name string) *Annotation {
	annot, _, err := d.annotationByPageAndName(pageNumber, name)
	if err != nil {
		return nil
	}
	return annot
}

// GetAnnotationActionDestURI returns annotation action destination URI.
func (d *Document) GetAnnotationActionDestURI(annotation *Annotation) string {
	actionDict := annotationActionDict(annotation)
	if actionDict == nil {
		return ""
	}
	if uri, ok := annotationObjectToString(actionDict.Get(entity.Name("URI"))); ok {
		return uri
	}
	return ""
}

// GetAnnotationActionTargetNames returns simple target-name list from action dictionary.
func (d *Document) GetAnnotationActionTargetNames(annotation *Annotation) []string {
	actionDict := annotationActionDict(annotation)
	if actionDict == nil {
		return nil
	}

	target := actionDict.Get(entity.Name("T"))
	switch v := target.(type) {
	case *entity.String:
		return []string{v.Value()}
	case entity.Name:
		return []string{string(v)}
	case *entity.Array:
		out := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			if value, ok := annotationObjectToString(v.Get(i)); ok && value != "" {
				out = append(out, value)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

// GetAnnotationActionTargetRefNos returns integer target refs from action dictionary.
func (d *Document) GetAnnotationActionTargetRefNos(annotation *Annotation) []int {
	actionDict := annotationActionDict(annotation)
	if actionDict == nil {
		return nil
	}

	target := actionDict.Get(entity.Name("T"))
	switch v := target.(type) {
	case *entity.Integer:
		return []int{int(v.Value())}
	case *entity.Array:
		out := make([]int, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			value, ok := annotationObjectToFloat64(v.Get(i))
			if ok {
				out = append(out, int(math.Round(value)))
				continue
			}
			if text, textOK := annotationObjectToString(v.Get(i)); textOK && strings.TrimSpace(text) != "" {
				// Java compatibility fallback: non-numeric targets keep positional ref numbers.
				out = append(out, i+1)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

// GetAnnotationAuthor returns annotation author (T) value.
func (d *Document) GetAnnotationAuthor(pageNumber int, name string) string {
	return d.annotationStringValueByName(pageNumber, name, "T")
}

// GetAnnotationBooleanValue returns annotation boolean value by key.
func (d *Document) GetAnnotationBooleanValue(annotation *Annotation, key string, defaultValue bool) bool {
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return defaultValue
	}
	obj := dict.Get(entity.Name(strings.TrimSpace(key)))
	value, ok := obj.(*entity.Boolean)
	if !ok || value == nil {
		return defaultValue
	}
	return value.Value()
}

// GetAnnotationBounds returns annotation bounds at zoom scale.
func (d *Document) GetAnnotationBounds(pageNumber int, name string, zoom float64) [4]float64 {
	annot := d.GetAnnotation(pageNumber, name)
	if annot == nil {
		return [4]float64{}
	}
	rect := annot.Rect()
	if zoom <= 0 {
		zoom = 1.0
	}
	return [4]float64{rect[0] * zoom, rect[1] * zoom, rect[2] * zoom, rect[3] * zoom}
}

// GetAnnotationColor returns annotation color as ARGB integer pointer.
func (d *Document) GetAnnotationColor(pageNumber int, name string) *int {
	return d.annotationColorByName(pageNumber, name, "C")
}

// GetAnnotationContents returns annotation contents.
func (d *Document) GetAnnotationContents(pageNumber int, name string) string {
	annot := d.GetAnnotation(pageNumber, name)
	if annot == nil {
		return ""
	}
	return annot.Contents()
}

// GetAnnotationCreationDate returns annotation creation date string.
func (d *Document) GetAnnotationCreationDate(pageNumber int, name string) string {
	return d.annotationStringValueByName(pageNumber, name, "CreationDate")
}

// GetAnnotationHidden returns annotation hidden flag.
func (d *Document) GetAnnotationHidden(annotation *Annotation) bool {
	return d.GetAnnotationBooleanValue(annotation, "Hidden", false)
}

// GetAnnotationImageData writes one annotation appearance payload to writer.
// It accepts a 1-based page number and an annotation index in that page.
func (d *Document) GetAnnotationImageData(pageNumber int, annotationIndex int, writer io.Writer) bool {
	if writer == nil || !d.IsValidPage(pageNumber) || annotationIndex < 0 {
		return false
	}

	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil || annotationIndex >= len(annots) {
		return false
	}

	payload := d.annotationImagePayload(annots[annotationIndex])
	if len(payload) == 0 {
		return false
	}

	_, err = writer.Write(payload)
	return err == nil
}

// GetAnnotationImageRect returns annotation rectangle.
func (d *Document) GetAnnotationImageRect(annotation *Annotation) [4]float64 {
	if annotation == nil {
		return [4]float64{}
	}
	return annotation.Rect()
}

// GetAnnotationInnerColor returns annotation inner color as ARGB integer pointer.
func (d *Document) GetAnnotationInnerColor(pageNumber int, name string) *int {
	return d.annotationColorByName(pageNumber, name, "IC")
}

// GetAnnotationIntValue returns annotation integer value by key.
func (d *Document) GetAnnotationIntValue(annotation *Annotation, key string, defaultValue int) int {
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return defaultValue
	}
	obj := dict.Get(entity.Name(strings.TrimSpace(key)))
	value, ok := annotationObjectToFloat64(obj)
	if !ok {
		return defaultValue
	}
	return int(math.Round(value))
}

// GetAnnotationJavaScript returns JavaScript source from annotation action.
func (d *Document) GetAnnotationJavaScript(annotation *Annotation) string {
	actionDict := annotationActionDict(annotation)
	if actionDict == nil {
		return ""
	}
	if js, ok := annotationObjectToString(actionDict.Get(entity.Name("JS"))); ok {
		return js
	}
	return ""
}

// GetAnnotationModifyDate returns annotation modify date string.
func (d *Document) GetAnnotationModifyDate(pageNumber int, name string) string {
	return d.annotationStringValueByName(pageNumber, name, "M")
}

// GetAnnotationNameValue returns annotation name-typed value by key.
func (d *Document) GetAnnotationNameValue(pageNumber int, name, key string) string {
	return d.annotationStringValueByName(pageNumber, name, key)
}

// GetAnnotationReplies returns annotation reply items.
// It accepts a 1-based page number and an annotation index in that page.
func (d *Document) GetAnnotationReplies(pageNumber int, annotationIndex int, includeNested bool) []map[string]string {
	if !d.IsValidPage(pageNumber) || annotationIndex < 0 {
		return nil
	}

	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil || annotationIndex >= len(annots) {
		return nil
	}

	targetName := strings.TrimSpace(annotationName(annots[annotationIndex]))
	parentNames := map[string]struct{}{}
	if targetName != "" {
		parentNames[strings.ToLower(targetName)] = struct{}{}
	}
	parentIndexes := map[string]struct{}{
		strconv.Itoa(annotationIndex): {},
	}

	accepted := make(map[int]struct{})
	for {
		changed := false
		for i := range annots {
			if i == annotationIndex {
				continue
			}
			if _, ok := accepted[i]; ok {
				continue
			}
			replyToName, replyToIndex := annotationReplyTarget(annots[i])
			if !replyTargetMatches(replyToName, replyToIndex, parentNames, parentIndexes) {
				continue
			}
			accepted[i] = struct{}{}
			changed = true

			if includeNested {
				name := strings.TrimSpace(annotationName(annots[i]))
				if name != "" {
					parentNames[strings.ToLower(name)] = struct{}{}
				}
				parentIndexes[strconv.Itoa(i)] = struct{}{}
			}
		}
		if !includeNested || !changed {
			break
		}
	}

	if len(accepted) == 0 {
		return nil
	}

	out := make([]map[string]string, 0, len(accepted))
	for i := range annots {
		if _, ok := accepted[i]; !ok {
			continue
		}
		out = append(out, d.replyMapFromAnnotation(annots[i], i))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetAnnotationStringValue returns annotation string value by key.
func (d *Document) GetAnnotationStringValue(pageNumber int, name, key string) string {
	return d.annotationStringValueByName(pageNumber, name, key)
}

// GetAnnotationSubject returns annotation subject.
func (d *Document) GetAnnotationSubject(pageNumber int, name string) string {
	return d.annotationStringValueByName(pageNumber, name, "Subj")
}

// GetAnnotationTransparency returns annotation transparency alpha (0..255).
func (d *Document) GetAnnotationTransparency(pageNumber int, name string) int {
	annot := d.GetAnnotation(pageNumber, name)
	dict := annotationEntityDict(annot)
	if dict == nil {
		return 255
	}
	alpha := 1.0
	if value, ok := annotationObjectToFloat64(dict.Get(entity.Name("CA"))); ok {
		alpha = value
	} else if value, ok := annotationObjectToFloat64(dict.Get(entity.Name("ca"))); ok {
		alpha = value
	}
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	return int(math.Round(alpha * 255.0))
}

// GetAnnotationType returns annotation subtype.
func (d *Document) GetAnnotationType(pageNumber int, name string) string {
	annot := d.GetAnnotation(pageNumber, name)
	if annot == nil {
		return ""
	}
	return annot.Type()
}

// IsAnnotationHasImageData reports whether annotation has image appearance payload.
func (d *Document) IsAnnotationHasImageData(annotation *Annotation) bool {
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return false
	}
	return dict.Get(entity.Name("AP")) != nil
}

// IsAnnotationResetFormFlagsExclude reports reset-form exclusion flag.
func (d *Document) IsAnnotationResetFormFlagsExclude(_ *Annotation) bool {
	return false
}

// IsAnnotationSubmitFormFlagsExclude reports submit-form exclusion flag.
func (d *Document) IsAnnotationSubmitFormFlagsExclude(_ *Annotation) bool {
	return false
}

// LookupAnnotationDetail marks one annotation as detail-loaded in session metadata.
func (d *Document) LookupAnnotationDetail(annotation *Annotation) {
	_ = d.setAnnotationUserData(annotation, "detail_loaded", "true")
}

// ReleaseAnnotationDetail clears annotation detail-loaded metadata.
func (d *Document) ReleaseAnnotationDetail(annotation *Annotation) {
	_ = d.setAnnotationUserData(annotation, "detail_loaded", "false")
}

// RemoveAnnotation removes one annotation by best-effort page/index matching.
func (d *Document) RemoveAnnotation(annotation *Annotation) bool {
	pageIndex, annotationIndex, ok := d.findAnnotationLocation(annotation)
	if !ok {
		return false
	}
	return d.RemovePageAnnotation(pageIndex, annotationIndex) == nil
}

// IsPlayedOnceAnnotation reports played-once state for multimedia annotations.
func (d *Document) IsPlayedOnceAnnotation(annotation *Annotation) bool {
	if annotation == nil {
		return false
	}
	userData := annotation.UserDataList()
	marker := strings.ToLower(strings.TrimSpace(annotationUserDataLookup(
		userData,
		"played_once",
		"playedOnce",
	)))
	return marker == "true" || marker == "1" || marker == "yes" || marker == "y"
}

// SetPlayedOnceAnnotation marks one annotation as played once.
func (d *Document) SetPlayedOnceAnnotation(annotation *Annotation) {
	_ = d.setAnnotationUserData(annotation, "played_once", "true")
}

// ResetPlayedOnceAnnotationRefs clears played-once metadata in one page.
// It accepts a 1-based page number.
func (d *Document) ResetPlayedOnceAnnotationRefs(pageNumber int) {
	if !d.IsValidPage(pageNumber) {
		return
	}
	pageIndex := pageNumber - 1
	annots, err := d.GetPageAnnotations(pageIndex)
	if err != nil {
		return
	}
	for i := range annots {
		if deleteErr := d.DeletePageAnnotationUserData(pageIndex, i, "played_once"); deleteErr != nil {
			continue
		}
	}
}

// IsAddedPage reports whether page is a duplicated/injected page in session page order.
func (d *Document) IsAddedPage(pageNumber int) bool {
	if !d.IsValidPage(pageNumber) {
		return false
	}
	order := d.PageOrder()
	if pageNumber <= 0 || pageNumber > len(order) {
		return false
	}
	sourceRef := order[pageNumber-1]
	first := -1
	for i, value := range order {
		if value != sourceRef {
			continue
		}
		first = i + 1
		break
	}
	return first > 0 && first != pageNumber
}

// IsPageCropped reports whether crop box differs from media box.
func (d *Document) IsPageCropped(pageNumber int) bool {
	if !d.IsValidPage(pageNumber) {
		return false
	}
	media, err := d.GetPageMediaBoxSL(pageNumber - 1)
	if err != nil {
		return false
	}
	crop, err := d.GetPageCropBoxSL(pageNumber - 1)
	if err != nil {
		return false
	}
	return media != crop
}

// IsImageAppendedAsTag reports appended-image tag state.
func (d *Document) IsImageAppendedAsTag(pageNumber int, tag string) bool {
	if !d.IsValidPage(pageNumber) {
		return false
	}
	trimmedTag := strings.TrimSpace(tag)
	if trimmedTag == "" {
		return false
	}

	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil {
		return false
	}
	for i := range annots {
		userData := annots[i].UserDataList()
		imageTag := annotationUserDataLookup(userData, "image_tag", "imageTag", "tag")
		if !strings.EqualFold(strings.TrimSpace(imageTag), trimmedTag) {
			continue
		}
		marker := strings.ToLower(strings.TrimSpace(annotationUserDataLookup(
			userData,
			"image_appended_as_tag",
			"imageAppendedAsTag",
			"appended_as_tag",
		)))
		return marker == "" || marker == "true" || marker == "1" || marker == "yes" || marker == "y"
	}
	return false
}

// InsertReplyAfter inserts one reply annotation after a target annotation index.
// It accepts a 1-based page number.
func (d *Document) InsertReplyAfter(pageNumber, annotationIndex int, reply interface{}, insertIndex int) bool {
	if insertIndex < 0 {
		insertIndex = annotationIndex + 1
	} else {
		insertIndex++
	}
	return d.insertReplyAnnotation(pageNumber, annotationIndex, reply, insertIndex)
}

// InsertReplyBefore inserts one reply annotation before a target annotation index.
// It accepts a 1-based page number.
func (d *Document) InsertReplyBefore(pageNumber, annotationIndex int, reply interface{}, insertIndex int) bool {
	if insertIndex < 0 {
		insertIndex = annotationIndex
	}
	return d.insertReplyAnnotation(pageNumber, annotationIndex, reply, insertIndex)
}

// AddReply appends one reply annotation in page annotation list.
// It accepts a 1-based page number.
func (d *Document) AddReply(pageNumber, annotationIndex int, reply interface{}) bool {
	return d.insertReplyAnnotation(pageNumber, annotationIndex, reply, -1)
}

// IsBtnFieldCommandSetImage reports whether button-field JS command sets image.
func (d *Document) IsBtnFieldCommandSetImage(value interface{}) bool {
	script := ""
	switch v := value.(type) {
	case *Annotation:
		script = d.GetAnnotationJavaScript(v)
	case string:
		script = v
	case map[string]string:
		for key, item := range v {
			trimmedKey := strings.TrimSpace(strings.ToLower(key))
			if trimmedKey == "script" || trimmedKey == "js" || trimmedKey == "command" {
				script = item
				break
			}
		}
	case map[string]interface{}:
		if raw, ok := legacyMapValueCI(v, "script", "js", "command", "contents"); ok {
			parsed, _ := legacyStringFromAny(raw)
			script = parsed
		}
	default:
		parsed, _ := legacyStringFromAny(value)
		script = parsed
	}

	lowered := strings.ToLower(strings.TrimSpace(script))
	if lowered == "" {
		return false
	}
	return strings.Contains(lowered, "buttonimporticon") ||
		strings.Contains(lowered, "setbuttonimage") ||
		(strings.Contains(lowered, "setimage") && strings.Contains(lowered, "button"))
}

func (d *Document) insertReplyAnnotation(
	pageNumber, annotationIndex int,
	reply interface{},
	insertIndex int,
) bool {
	if !d.IsValidPage(pageNumber) || annotationIndex < 0 {
		return false
	}

	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil || annotationIndex >= len(annots) {
		return false
	}

	spec, ok := legacyReplySpecFromAny(reply)
	if !ok {
		return false
	}

	target := annots[annotationIndex]
	if strings.TrimSpace(spec.Type) == "" {
		spec.Type = "Text"
	}
	if spec.Rect[2] <= spec.Rect[0] || spec.Rect[3] <= spec.Rect[1] {
		spec.Rect = replyRect(target.Rect())
	}
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = d.nextReplyName(pageNumber)
	}

	nextUserData := cloneStringMap(spec.UserData)
	if nextUserData == nil {
		nextUserData = make(map[string]string)
	}

	replyToName := strings.TrimSpace(annotationName(target))
	if replyToName == "" {
		replyToName = strconv.Itoa(annotationIndex)
	}
	nextUserData["reply_to"] = replyToName
	nextUserData["reply_to_index"] = strconv.Itoa(annotationIndex)
	spec.UserData = nextUserData

	pageIndex := pageNumber - 1
	if insertIndex < 0 || insertIndex >= len(annots) {
		return d.AddPageAnnotation(pageIndex, spec) == nil
	}

	specs := make([]AnnotationSpec, 0, len(annots)+1)
	for i := range annots {
		if i == insertIndex {
			specs = append(specs, spec)
		}
		specs = append(specs, annotationToSpec(annots[i]))
	}
	if insertIndex == len(annots) {
		specs = append(specs, spec)
	}
	return d.ReplacePageAnnotations(pageIndex, specs) == nil
}

func legacyReplySpecFromAny(value interface{}) (AnnotationSpec, bool) {
	spec := AnnotationSpec{
		Type: "Text",
	}
	switch v := value.(type) {
	case nil:
		return AnnotationSpec{}, false
	case AnnotationSpec:
		spec = v
	case *Annotation:
		if v == nil {
			return AnnotationSpec{}, false
		}
		spec = annotationToSpec(v)
	case string:
		spec.Contents = v
	case map[string]string:
		for key, item := range v {
			trimmedKey := strings.TrimSpace(strings.ToLower(key))
			switch trimmedKey {
			case "name", "nm":
				spec.Name = item
			case "type", "subtype":
				spec.Type = item
			case "contents", "content", "text":
				spec.Contents = item
			case "author":
				if spec.UserData == nil {
					spec.UserData = make(map[string]string)
				}
				spec.UserData["author"] = item
			case "subject":
				if spec.UserData == nil {
					spec.UserData = make(map[string]string)
				}
				spec.UserData["subject"] = item
			}
		}
	case map[string]interface{}:
		if raw, ok := legacyMapValueCI(v, "name", "nm"); ok {
			spec.Name, _ = legacyStringFromAny(raw)
		}
		if raw, ok := legacyMapValueCI(v, "type", "subtype"); ok {
			spec.Type, _ = legacyStringFromAny(raw)
		}
		if raw, ok := legacyMapValueCI(v, "contents", "content", "text"); ok {
			spec.Contents, _ = legacyStringFromAny(raw)
		}
		if raw, ok := legacyMapValueCI(v, "rect", "bounds"); ok {
			if parsed, parseOK := legacyRectFromAny(raw); parseOK {
				spec.Rect = parsed
			}
		}

		if raw, ok := legacyMapValueCI(v, "author"); ok {
			if parsed, parseOK := legacyStringFromAny(raw); parseOK {
				if spec.UserData == nil {
					spec.UserData = make(map[string]string)
				}
				spec.UserData["author"] = parsed
			}
		}
		if raw, ok := legacyMapValueCI(v, "subject"); ok {
			if parsed, parseOK := legacyStringFromAny(raw); parseOK {
				if spec.UserData == nil {
					spec.UserData = make(map[string]string)
				}
				spec.UserData["subject"] = parsed
			}
		}
	default:
		return AnnotationSpec{}, false
	}

	if strings.TrimSpace(spec.Name) == "" &&
		strings.TrimSpace(spec.Contents) == "" &&
		len(spec.UserData) == 0 {
		return AnnotationSpec{}, false
	}
	return spec, true
}

func (d *Document) replyMapFromAnnotation(annotation *Annotation, annotationIndex int) map[string]string {
	out := map[string]string{
		"annotation_index": strconv.Itoa(annotationIndex),
		"name":             annotationName(annotation),
		"type":             annotation.Type(),
		"contents":         annotation.Contents(),
	}

	replyToName, replyToIndex := annotationReplyTarget(annotation)
	if strings.TrimSpace(replyToName) != "" {
		out["reply_to"] = replyToName
	}
	if strings.TrimSpace(replyToIndex) != "" {
		out["reply_to_index"] = replyToIndex
	}

	userData := annotation.UserDataList()
	author := annotationUserDataLookup(userData, "author")
	subject := annotationUserDataLookup(userData, "subject")
	if dict := annotationEntityDict(annotation); dict != nil {
		if strings.TrimSpace(author) == "" {
			author = annotationStringValueFromDict(dict, "T")
		}
		if strings.TrimSpace(subject) == "" {
			subject = annotationStringValueFromDict(dict, "Subj")
		}
	}
	if strings.TrimSpace(author) != "" {
		out["author"] = author
	}
	if strings.TrimSpace(subject) != "" {
		out["subject"] = subject
	}
	return out
}

func annotationReplyTarget(annotation *Annotation) (string, string) {
	userData := annotation.UserDataList()
	replyToName := annotationUserDataLookup(userData, "reply_to", "replyTo", "inReplyTo")
	replyToIndex := annotationUserDataLookup(userData, "reply_to_index", "replyToIndex", "inReplyToIndex")

	dict := annotationEntityDict(annotation)
	if dict == nil {
		return strings.TrimSpace(replyToName), strings.TrimSpace(replyToIndex)
	}

	if strings.TrimSpace(replyToName) == "" {
		replyToName = annotationStringValueFromDict(dict, "IRTNM")
	}

	irt := dict.Get(entity.Name("IRT"))
	switch value := irt.(type) {
	case *entity.Dict:
		if strings.TrimSpace(replyToName) == "" {
			replyToName = annotationStringValueFromDict(value, "NM")
		}
	case entity.Ref:
		if strings.TrimSpace(replyToIndex) == "" {
			replyToIndex = strconv.Itoa(int(value.Num()))
		}
	default:
		if strings.TrimSpace(replyToName) == "" {
			if parsed, ok := annotationObjectToString(irt); ok {
				replyToName = parsed
			}
		}
	}

	return strings.TrimSpace(replyToName), strings.TrimSpace(replyToIndex)
}

func replyTargetMatches(
	replyToName, replyToIndex string,
	parentNames map[string]struct{},
	parentIndexes map[string]struct{},
) bool {
	name := strings.ToLower(strings.TrimSpace(replyToName))
	if name != "" {
		if _, ok := parentNames[name]; ok {
			return true
		}
	}

	index := strings.TrimSpace(replyToIndex)
	if index != "" {
		if _, ok := parentIndexes[index]; ok {
			return true
		}
	}
	return false
}

func annotationUserDataLookup(input map[string]string, keys ...string) string {
	if len(input) == 0 {
		return ""
	}
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		for currentKey, value := range input {
			if strings.EqualFold(strings.TrimSpace(currentKey), trimmed) {
				return value
			}
		}
	}
	return ""
}

func replyRect(base [4]float64) [4]float64 {
	x0 := base[2]
	y0 := base[1]
	width := base[2] - base[0]
	height := base[3] - base[1]
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 40
	}
	return [4]float64{x0 + 4, y0, x0 + 4 + width, y0 + height}
}

func (d *Document) nextReplyName(pageNumber int) string {
	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil {
		return "reply-1"
	}

	seen := make(map[string]struct{}, len(annots))
	for i := range annots {
		name := strings.ToLower(strings.TrimSpace(annotationName(annots[i])))
		if name != "" {
			seen[name] = struct{}{}
		}
	}

	for index := 1; ; index++ {
		candidate := fmt.Sprintf("reply-%d", index)
		if _, ok := seen[strings.ToLower(candidate)]; !ok {
			return candidate
		}
	}
}

func (d *Document) annotationImagePayload(annotation *Annotation) []byte {
	if annotation == nil {
		return nil
	}

	userData := annotation.UserDataList()
	streamHandleText := annotationUserDataLookup(
		userData,
		"image_handle",
		"image_stream_handle",
		"stream_handle",
	)
	if handle, ok := legacyIntFromAny(streamHandleText); ok && handle > 0 {
		d.mu.RLock()
		stream := d.legacyStreams[handle]
		d.mu.RUnlock()
		if stream != nil && len(stream.data) > 0 {
			return append([]byte(nil), stream.data...)
		}
	}

	dict := annotationEntityDict(annotation)
	if dict == nil {
		return nil
	}
	return annotationAppearancePayload(dict.Get(entity.Name("AP")))
}

func annotationAppearancePayload(obj entity.Object) []byte {
	switch value := obj.(type) {
	case *entity.Stream:
		if decoded, err := value.Decode(); err == nil && len(decoded) > 0 {
			return append([]byte(nil), decoded...)
		}
		raw := value.RawBytes()
		if len(raw) > 0 {
			return append([]byte(nil), raw...)
		}
	case *entity.Dict:
		for _, key := range []entity.Name{"N", "R", "D"} {
			if payload := annotationAppearancePayload(value.Get(key)); len(payload) > 0 {
				return payload
			}
		}
		for _, key := range value.Keys() {
			if payload := annotationAppearancePayload(value.GetRaw(key)); len(payload) > 0 {
				return payload
			}
		}
	case *entity.Array:
		for i := 0; i < value.Len(); i++ {
			if payload := annotationAppearancePayload(value.Get(i)); len(payload) > 0 {
				return payload
			}
		}
	}
	return nil
}

func (d *Document) setAnnotationUserData(annotation *Annotation, key, value string) bool {
	if annotation == nil {
		return false
	}
	pageIndex, annotationIndex, ok := d.findAnnotationLocation(annotation)
	if !ok {
		return false
	}
	return d.SetPageAnnotationUserData(pageIndex, annotationIndex, key, value) == nil
}

func (d *Document) annotationByPageAndName(pageNumber int, name string) (*Annotation, int, error) {
	if !d.IsValidPage(pageNumber) {
		return nil, -1, strconv.ErrRange
	}
	annots, err := d.GetPageAnnotations(pageNumber - 1)
	if err != nil {
		return nil, -1, err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, -1, strconv.ErrSyntax
	}

	if idx, parseErr := strconv.Atoi(trimmed); parseErr == nil {
		if idx >= 0 && idx < len(annots) {
			return annots[idx], idx, nil
		}
	}

	for i, item := range annots {
		if strings.EqualFold(strings.TrimSpace(annotationName(item)), trimmed) {
			return item, i, nil
		}
	}
	return nil, -1, strconv.ErrSyntax
}

func (d *Document) annotationStringValueByName(pageNumber int, name, key string) string {
	annot := d.GetAnnotation(pageNumber, name)
	dict := annotationEntityDict(annot)
	if dict == nil {
		return ""
	}
	value, ok := annotationObjectToString(dict.Get(entity.Name(strings.TrimSpace(key))))
	if !ok {
		return ""
	}
	return value
}

func (d *Document) annotationColorByName(pageNumber int, name, key string) *int {
	annot := d.GetAnnotation(pageNumber, name)
	dict := annotationEntityDict(annot)
	if dict == nil {
		return nil
	}
	obj := dict.Get(entity.Name(strings.TrimSpace(key)))
	arr, ok := obj.(*entity.Array)
	if !ok || arr.Len() < 3 {
		return nil
	}

	components := [3]float64{}
	for i := 0; i < 3; i++ {
		value, valueOK := annotationObjectToFloat64(arr.Get(i))
		if !valueOK {
			return nil
		}
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}
		components[i] = value
	}

	r := int(math.Round(components[0] * 255.0))
	g := int(math.Round(components[1] * 255.0))
	b := int(math.Round(components[2] * 255.0))
	argb := (255 << 24) | (r << 16) | (g << 8) | b
	return &argb
}

func annotationEntityDict(annotation *Annotation) *entity.Dict {
	if annotation == nil || annotation.annotation == nil || annotation.snapshot != nil {
		return nil
	}
	return annotation.annotation.Dict()
}

func annotationActionDict(annotation *Annotation) *entity.Dict {
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return nil
	}
	action := dict.Get(entity.Name("A"))
	actionDict, ok := action.(*entity.Dict)
	if !ok {
		return nil
	}
	return actionDict
}

func (d *Document) findAnnotationLocation(target *Annotation) (int, int, bool) {
	if target == nil {
		return 0, 0, false
	}
	pageCount, err := d.PageCount()
	if err != nil {
		return 0, 0, false
	}
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		annots, annotErr := d.GetPageAnnotations(pageIndex)
		if annotErr != nil {
			return 0, 0, false
		}
		for annotationIndex, item := range annots {
			if annotationEquivalent(item, target) {
				return pageIndex, annotationIndex, true
			}
		}
	}
	return 0, 0, false
}

func annotationEquivalent(a, b *Annotation) bool {
	if a == nil || b == nil {
		return false
	}
	nameA := strings.TrimSpace(annotationName(a))
	nameB := strings.TrimSpace(annotationName(b))
	if nameA != "" && nameB != "" {
		return strings.EqualFold(nameA, nameB)
	}
	if a.Type() != b.Type() {
		return false
	}
	if a.Contents() != b.Contents() {
		return false
	}
	return a.Rect() == b.Rect()
}

func annotationName(annotation *Annotation) string {
	if annotation == nil {
		return ""
	}
	if annotation.snapshot != nil {
		return strings.TrimSpace(annotation.snapshot.Name)
	}
	dict := annotationEntityDict(annotation)
	if dict == nil {
		return ""
	}
	name, _ := annotationObjectToString(dict.Get(entity.Name("NM")))
	return strings.TrimSpace(name)
}
