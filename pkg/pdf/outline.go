package pdf

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

// Outline represents one bookmark/outline item in a PDF document.
type Outline struct {
	Dest      Object
	Action    *OutlineAction
	Title     string
	Children  []*Outline
	Count     int
	PageIndex int
	Color     int
}

// OutlineAction represents an outline action dictionary.
type OutlineAction struct {
	Dest               Object
	Directory          string
	Operation          string
	RenditionName      string
	File               string
	RenditionFile      string
	RenditionMIMEType  string
	JavaScript         string
	Named              string
	Command            string
	Type               string
	URI                string
	FieldNames         []string
	HideTargets        []string
	NextActions        []*OutlineAction
	Flags              int
	PageIndex          int
	RenditionOperation int
	ExcludeFields      bool
	Hide               bool
	HasHide            bool
	NewWindow          bool
	HasNewWindow       bool
}

// Outlines returns the document outline tree (bookmarks).
// It returns nil, nil when the document has no outlines.
func (d *Document) Outlines() ([]*Outline, error) {
	d.mu.RLock()
	if d.outlinesSet {
		cloned := cloneOutlines(d.outlines)
		d.mu.RUnlock()
		if len(cloned) == 0 {
			return nil, nil
		}
		return cloned, nil
	}
	d.mu.RUnlock()

	return d.loadOutlinesFromPDF()
}

func (d *Document) loadOutlinesFromPDF() ([]*Outline, error) {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil, nil
	}

	outlinesObj := catalog.Get(entity.Name("Outlines"))
	if outlinesObj == nil {
		return nil, nil
	}

	outlinesDict, err := d.asDict(outlinesObj)
	if err != nil {
		return nil, err
	}

	first := outlinesDict.Get(entity.Name("First"))
	if first == nil {
		return nil, nil
	}

	pageRefToIndex := d.buildPageRefToIndexMap()
	visited := make(map[*entity.Dict]struct{})
	items, err := d.parseOutlineSiblings(first, visited, pageRefToIndex)
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (d *Document) parseOutlineSiblings(start entity.Object, visited map[*entity.Dict]struct{}, pageRefToIndex map[entity.Ref]int) ([]*Outline, error) {
	items := make([]*Outline, 0)
	current := start

	for current != nil {
		dict, err := d.asDict(current)
		if err != nil {
			return nil, err
		}

		if _, seen := visited[dict]; seen {
			break
		}
		visited[dict] = struct{}{}

		item, err := d.parseOutlineNode(dict, visited, pageRefToIndex)
		if err != nil {
			return nil, err
		}
		items = append(items, item)

		current = dict.Get(entity.Name("Next"))
	}

	return items, nil
}

func (d *Document) parseOutlineNode(dict *entity.Dict, visited map[*entity.Dict]struct{}, pageRefToIndex map[entity.Ref]int) (*Outline, error) {
	item := &Outline{
		Title:     extractEntityString(dict.Get(entity.Name("Title"))),
		Count:     extractEntityInt(dict.Get(entity.Name("Count"))),
		PageIndex: -1,
		Color:     parseOutlineColor(dict.Get(entity.Name("C"))),
	}

	if dest := dict.Get(entity.Name("Dest")); dest != nil {
		item.Dest, item.PageIndex = d.resolveDestination(dest, pageRefToIndex)
	}

	if actionObj := dict.Get(entity.Name("A")); actionObj != nil {
		action, err := d.parseOutlineAction(actionObj, pageRefToIndex)
		if err != nil {
			return nil, err
		}
		item.Action = action
		if item.PageIndex < 0 && action != nil && action.PageIndex >= 0 {
			item.PageIndex = action.PageIndex
		}
	}

	if firstChild := dict.Get(entity.Name("First")); firstChild != nil {
		children, err := d.parseOutlineSiblings(firstChild, visited, pageRefToIndex)
		if err != nil {
			return nil, err
		}
		item.Children = children
	}

	return item, nil
}

func (d *Document) parseOutlineAction(obj entity.Object, pageRefToIndex map[entity.Ref]int) (*OutlineAction, error) {
	return d.parseOutlineActionWithDepth(obj, pageRefToIndex, 0)
}

func (d *Document) parseOutlineActionWithDepth(obj entity.Object, pageRefToIndex map[entity.Ref]int, depth int) (*OutlineAction, error) {
	if depth > 16 {
		return nil, fmt.Errorf("outline action depth exceeded")
	}

	actionDict, err := d.asDict(obj)
	if err != nil {
		return nil, err
	}

	file, command, directory, operation := d.extractLaunchActionDetails(actionDict)

	action := &OutlineAction{
		Type:       extractEntityNameOrString(actionDict.Get(entity.Name("S"))),
		PageIndex:  -1,
		URI:        extractEntityString(actionDict.Get(entity.Name("URI"))),
		File:       file,
		Named:      extractEntityNameOrString(actionDict.Get(entity.Name("N"))),
		Command:    command,
		Directory:  directory,
		Operation:  operation,
		JavaScript: extractActionJavaScript(actionDict),
	}
	action.Flags = extractEntityInt(actionDict.Get(entity.Name("Flags")))
	action.ExcludeFields = action.Flags&0x1 != 0
	action.FieldNames = d.extractActionNames(actionDict.Get(entity.Name("Fields")), 0)

	if newWindowObj := actionDict.Get(entity.Name("NewWindow")); newWindowObj != nil {
		if newWindow, ok := newWindowObj.(*entity.Boolean); ok {
			action.NewWindow = newWindow.Value()
			action.HasNewWindow = true
		}
	}
	if action.Type == "Hide" {
		action.Hide = true
		if hideObj := actionDict.Get(entity.Name("H")); hideObj != nil {
			if hide, ok := hideObj.(*entity.Boolean); ok {
				action.Hide = hide.Value()
				action.HasHide = true
			}
		}
		action.HideTargets = d.extractActionNames(actionDict.Get(entity.Name("T")), 0)
	}
	if action.Type == "Rendition" {
		action.RenditionOperation = extractEntityInt(actionDict.Get(entity.Name("OP")))
		if renditionObj := actionDict.Get(entity.Name("R")); renditionObj != nil {
			if renditionDict, renditionErr := d.asDict(renditionObj); renditionErr == nil {
				action.RenditionName = extractEntityString(renditionDict.Get(entity.Name("N")))
				action.RenditionFile, action.RenditionMIMEType = d.extractRenditionMedia(renditionDict)
			}
		}
	}

	if dest := actionDict.Get(entity.Name("D")); dest != nil {
		action.Dest, action.PageIndex = d.resolveDestination(dest, pageRefToIndex)
	}
	if nextObj := actionDict.Get(entity.Name("Next")); nextObj != nil {
		nextActions, nextErr := d.parseOutlineActionList(nextObj, pageRefToIndex, depth+1)
		if nextErr != nil {
			return nil, nextErr
		}
		action.NextActions = nextActions
	}

	return action, nil
}

func (d *Document) parseOutlineActionList(obj entity.Object, pageRefToIndex map[entity.Ref]int, depth int) ([]*OutlineAction, error) {
	if obj == nil {
		return nil, nil
	}
	if depth > 16 {
		return nil, fmt.Errorf("outline action list depth exceeded")
	}

	if arr, ok := obj.(*entity.Array); ok {
		out := make([]*OutlineAction, 0, arr.Len())
		for i := 0; i < arr.Len(); i++ {
			action, err := d.parseOutlineActionWithDepth(arr.Get(i), pageRefToIndex, depth+1)
			if err != nil {
				return nil, err
			}
			if action != nil {
				out = append(out, action)
			}
		}
		return out, nil
	}

	action, err := d.parseOutlineActionWithDepth(obj, pageRefToIndex, depth+1)
	if err != nil {
		return nil, err
	}
	if action == nil {
		return nil, nil
	}
	return []*OutlineAction{action}, nil
}

func (d *Document) extractActionNames(obj entity.Object, depth int) []string {
	if obj == nil || depth > 16 {
		return nil
	}

	switch v := obj.(type) {
	case *entity.String:
		name := extractEntityString(v)
		if name == "" {
			return nil
		}
		return []string{name}
	case entity.Name:
		name := v.Value()
		if name == "" {
			return nil
		}
		return []string{name}
	case *entity.Array:
		out := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			out = append(out, d.extractActionNames(v.Get(i), depth+1)...)
		}
		return dedupeStrings(out)
	case *entity.Dict:
		for _, key := range []entity.Name{"T", "NM", "N"} {
			name := extractEntityNameOrString(v.Get(key))
			if name != "" {
				return []string{name}
			}
		}
		return nil
	case entity.Ref:
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return nil
		}
		return d.extractActionNames(fetched, depth+1)
	default:
		return nil
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (d *Document) extractLaunchActionDetails(actionDict *entity.Dict) (file, command, directory, operation string) {
	if actionDict == nil {
		return "", "", "", ""
	}

	file = extractActionFile(actionDict)
	command = extractEntityString(actionDict.Get(entity.Name("P")))
	directory = extractEntityString(actionDict.Get(entity.Name("D")))
	operation = extractEntityString(actionDict.Get(entity.Name("O")))

	for _, platformKey := range []string{"Win", "Mac", "Unix"} {
		platformObj := actionDict.Get(entity.Name(platformKey))
		if platformObj == nil {
			continue
		}

		platformDict, err := d.asDict(platformObj)
		if err != nil {
			continue
		}

		if file == "" {
			file = extractActionFile(platformDict)
		}
		if command == "" {
			command = extractEntityString(platformDict.Get(entity.Name("P")))
		}
		if directory == "" {
			directory = extractEntityString(platformDict.Get(entity.Name("D")))
		}
		if operation == "" {
			operation = extractEntityString(platformDict.Get(entity.Name("O")))
		}
	}

	return file, command, directory, operation
}

func (d *Document) extractRenditionMedia(renditionDict *entity.Dict) (file, mimeType string) {
	if renditionDict == nil {
		return "", ""
	}

	file = extractActionFile(renditionDict)
	mimeType = extractEntityString(renditionDict.Get(entity.Name("CT")))

	clipObj := renditionDict.Get(entity.Name("C"))
	if clipObj == nil {
		return file, mimeType
	}

	clipDict, err := d.asDict(clipObj)
	if err != nil {
		return file, mimeType
	}

	if file == "" {
		file = extractActionFile(clipDict)
	}
	if mimeType == "" {
		mimeType = extractEntityString(clipDict.Get(entity.Name("CT")))
	}

	dataObj := clipDict.Get(entity.Name("D"))
	if dataObj == nil {
		return file, mimeType
	}

	switch v := dataObj.(type) {
	case *entity.String:
		if file == "" {
			file = extractEntityString(v)
		}
	case entity.Name:
		if file == "" {
			file = v.Value()
		}
	case *entity.Dict:
		if file == "" {
			file = extractActionFile(v)
		}
		if mimeType == "" {
			mimeType = extractEntityString(v.Get(entity.Name("CT")))
		}
	default:
		dataDict, convErr := d.asDict(dataObj)
		if convErr != nil {
			return file, mimeType
		}
		if file == "" {
			file = extractActionFile(dataDict)
		}
		if mimeType == "" {
			mimeType = extractEntityString(dataDict.Get(entity.Name("CT")))
		}
	}

	return file, mimeType
}

func (d *Document) resolveDestination(dest entity.Object, pageRefToIndex map[entity.Ref]int) (Object, int) {
	wrapped := wrapObject(dest)
	return wrapped, d.resolveDestinationPageIndex(dest, pageRefToIndex, 0)
}

func (d *Document) buildPageRefToIndexMap() map[entity.Ref]int {
	pageRefToIndex := make(map[entity.Ref]int)

	pageCount, err := d.doc.PageCount()
	if err != nil {
		return pageRefToIndex
	}

	for i := 0; i < pageCount; i++ {
		page, pageErr := d.doc.GetPage(i)
		if pageErr != nil {
			continue
		}
		pageRefToIndex[page.Ref()] = i
	}

	return pageRefToIndex
}

func (d *Document) asDict(obj entity.Object) (*entity.Dict, error) {
	switch v := obj.(type) {
	case *entity.Dict:
		return v, nil
	case entity.Ref:
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return nil, fmt.Errorf("fetch dict ref: %w", err)
		}
		dict, ok := fetched.(*entity.Dict)
		if !ok {
			return nil, fmt.Errorf("object is not dictionary: %T", fetched)
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("object is not dictionary: %T", obj)
	}
}

func (d *Document) resolveDestinationPageIndex(dest entity.Object, pageRefToIndex map[entity.Ref]int, depth int) int {
	if depth > 16 || dest == nil {
		return -1
	}

	switch v := dest.(type) {
	case entity.Ref:
		if idx, ok := pageRefToIndex[v]; ok {
			return idx
		}
		fetched, err := d.doc.XRef().Fetch(v)
		if err == nil {
			return d.resolveDestinationPageIndex(fetched, pageRefToIndex, depth+1)
		}
	case *entity.Array:
		if v.Len() > 0 {
			return d.resolveDestinationPageIndex(v.Get(0), pageRefToIndex, depth+1)
		}
	case *entity.Dict:
		if dObj := v.Get(entity.Name("D")); dObj != nil {
			return d.resolveDestinationPageIndex(dObj, pageRefToIndex, depth+1)
		}
	case *entity.String:
		if named := d.lookupNamedDestination(v.Value()); named != nil {
			return d.resolveDestinationPageIndex(named, pageRefToIndex, depth+1)
		}
	case entity.Name:
		if named := d.lookupNamedDestination(v.Value()); named != nil {
			return d.resolveDestinationPageIndex(named, pageRefToIndex, depth+1)
		}
	}

	return -1
}

func (d *Document) lookupNamedDestination(name string) entity.Object {
	catalog := d.doc.Catalog()
	if catalog == nil || name == "" {
		return nil
	}

	// 1) Legacy catalog /Dests dictionary.
	if destsObj := catalog.Get(entity.Name("Dests")); destsObj != nil {
		if destsDict, err := d.asDict(destsObj); err == nil {
			if dest := lookupDestinationInDict(destsDict, name); dest != nil {
				return dest
			}
		}
	}

	// 2) Name tree: catalog /Names /Dests.
	namesObj := catalog.Get(entity.Name("Names"))
	if namesObj == nil {
		return nil
	}
	namesDict, err := d.asDict(namesObj)
	if err != nil {
		return nil
	}

	destsTreeObj := namesDict.Get(entity.Name("Dests"))
	if destsTreeObj == nil {
		return nil
	}
	destsTree, err := d.asDict(destsTreeObj)
	if err != nil {
		return nil
	}

	return d.findDestinationInNameTree(destsTree, name, 0)
}

func (d *Document) findDestinationInNameTree(node *entity.Dict, name string, depth int) entity.Object {
	if node == nil || depth > 32 {
		return nil
	}

	if namesObj := node.Get(entity.Name("Names")); namesObj != nil {
		if namesArr, ok := namesObj.(*entity.Array); ok {
			for i := 0; i+1 < namesArr.Len(); i += 2 {
				key := extractEntityNameOrString(namesArr.Get(i))
				if key != name {
					continue
				}
				return normalizeDestinationValue(namesArr.Get(i + 1))
			}
		}
	}

	if kidsObj := node.Get(entity.Name("Kids")); kidsObj != nil {
		if kids, ok := kidsObj.(*entity.Array); ok {
			for i := 0; i < kids.Len(); i++ {
				kidDict, err := d.asDict(kids.Get(i))
				if err != nil {
					continue
				}
				if found := d.findDestinationInNameTree(kidDict, name, depth+1); found != nil {
					return found
				}
			}
		}
	}

	return nil
}

func lookupDestinationInDict(destsDict *entity.Dict, name string) entity.Object {
	if destsDict == nil || name == "" {
		return nil
	}
	if direct := destsDict.Get(entity.Name(name)); direct != nil {
		return normalizeDestinationValue(direct)
	}
	return nil
}

func normalizeDestinationValue(obj entity.Object) entity.Object {
	if dict, ok := obj.(*entity.Dict); ok {
		if dObj := dict.Get(entity.Name("D")); dObj != nil {
			return dObj
		}
	}
	return obj
}

func extractActionFile(actionDict *entity.Dict) string {
	if actionDict == nil {
		return ""
	}

	fileObj := actionDict.Get(entity.Name("F"))
	if fileObj == nil {
		return ""
	}

	switch v := fileObj.(type) {
	case *entity.String:
		return extractEntityString(v)
	case entity.Name:
		return v.Value()
	case *entity.Dict:
		if uf := v.Get(entity.Name("UF")); uf != nil {
			if s := extractEntityString(uf); s != "" {
				return s
			}
		}
		if f := v.Get(entity.Name("F")); f != nil {
			if s := extractEntityString(f); s != "" {
				return s
			}
		}
	}

	return ""
}

func extractActionJavaScript(actionDict *entity.Dict) string {
	if actionDict == nil {
		return ""
	}

	jsObj := actionDict.Get(entity.Name("JS"))
	if jsObj == nil {
		return ""
	}

	switch v := jsObj.(type) {
	case *entity.String:
		return extractEntityString(v)
	case entity.Name:
		return v.Value()
	case *entity.Stream:
		decoded, err := pdfstream.NewFromEntity(v).Decode()
		if err == nil {
			return string(decoded)
		}
	}

	return ""
}

func extractEntityString(obj entity.Object) string {
	switch v := obj.(type) {
	case *entity.String:
		return decodePDFTextString(v.Value())
	case entity.Name:
		return decodePDFTextString(v.Value())
	default:
		return ""
	}
}

func extractEntityNameOrString(obj entity.Object) string {
	switch v := obj.(type) {
	case entity.Name:
		return v.Value()
	case *entity.String:
		return v.Value()
	default:
		return ""
	}
}

func extractEntityInt(obj entity.Object) int {
	if v, ok := obj.(*entity.Integer); ok {
		return int(v.Value())
	}
	return 0
}

func extractEntityFloat(obj entity.Object) (float64, bool) {
	switch v := obj.(type) {
	case *entity.Real:
		return v.Value(), true
	case *entity.Integer:
		return float64(v.Value()), true
	default:
		return 0, false
	}
}

func parseOutlineColor(obj entity.Object) int {
	const defaultColor = 0xFF000000

	arr, ok := obj.(*entity.Array)
	if !ok || arr.Len() < 3 {
		return defaultColor
	}

	rf, rok := extractEntityFloat(arr.Get(0))
	gf, gok := extractEntityFloat(arr.Get(1))
	bf, bok := extractEntityFloat(arr.Get(2))
	if !rok || !gok || !bok {
		return defaultColor
	}

	toByte := func(v float64) int {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		return int(v * 255)
	}

	r := toByte(rf)
	g := toByte(gf)
	b := toByte(bf)
	return 0xFF000000 | (r << 16) | (g << 8) | b
}

func decodePDFTextString(value string) string {
	raw := []byte(value)
	if len(raw) < 2 {
		return value
	}

	if raw[0] == 0xFE && raw[1] == 0xFF {
		return decodeUTF16(raw[2:], binary.BigEndian)
	}
	if raw[0] == 0xFF && raw[1] == 0xFE {
		return decodeUTF16(raw[2:], binary.LittleEndian)
	}

	return value
}

func decodeUTF16(data []byte, order binary.ByteOrder) string {
	if len(data) < 2 {
		return ""
	}
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return ""
	}

	u16 := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		u16 = append(u16, order.Uint16(data[i:i+2]))
	}

	return string(utf16.Decode(u16))
}
