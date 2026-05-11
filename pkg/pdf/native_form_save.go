package pdf

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type nativeFieldEntry struct {
	fieldType string
	ref       entity.Ref
}

type xrefEntryUpdate struct {
	num    uint32
	gen    uint16
	offset int
}

type nativeSaveContext struct {
	raw         []byte
	prevXRef    uint64
	rootRef     entity.Ref
	objectCount int
}

type nativeObjectAllocator struct {
	next uint32
}

func newNativeObjectAllocator(start int) *nativeObjectAllocator {
	if start < 1 {
		start = 1
	}
	return &nativeObjectAllocator{
		next: uint32(start),
	}
}

// ObserveRef is an exported API.
func (a *nativeObjectAllocator) ObserveRef(ref entity.Ref) {
	if ref.Num() >= a.next {
		a.next = ref.Num() + 1
	}
}

// Next is an exported API.
func (a *nativeObjectAllocator) Next() entity.Ref {
	ref := entity.NewRef(a.next, 0)
	a.next++
	return ref
}

// SaveWithNativeFormUpdates writes a new PDF with current session form overrides
// persisted into AcroForm field objects via incremental updates.
func (d *Document) SaveWithNativeFormUpdates(path string) error {
	valueOverrides := d.snapshotFormValueOverrides()
	optionOverrides := d.snapshotFormOptionOverrides()
	if len(valueOverrides) == 0 && len(optionOverrides) == 0 {
		return fmt.Errorf("no form overrides to persist")
	}

	updates, err := d.collectNativeFormUpdates(valueOverrides, optionOverrides)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return fmt.Errorf("no writable form field objects found for overrides")
	}

	ctx, err := d.resolveNativeSaveContext()
	if err != nil {
		return err
	}

	out, err := writeNativeIncrementalUpdate(ctx.raw, updates, ctx.rootRef, ctx.prevXRef, ctx.objectCount)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write PDF: %w", err)
	}
	return nil
}

// SaveWithNativeSessionUpdates writes a new PDF with current session mutations
// persisted directly as standard PDF objects via incremental updates.
func (d *Document) SaveWithNativeSessionUpdates(path string) error {
	ctx, err := d.resolveNativeSaveContext()
	if err != nil {
		return err
	}

	updates := make(map[entity.Ref]entity.Object)

	formValueOverrides := d.snapshotFormValueOverrides()
	formOptionOverrides := d.snapshotFormOptionOverrides()
	formUpdates, err := d.collectNativeFormUpdates(formValueOverrides, formOptionOverrides)
	if err != nil {
		return err
	}
	for ref, obj := range formUpdates {
		updates[ref] = obj
	}

	outlines, outlinesSet := d.snapshotNativeOutlines()
	annotationOverrides := d.snapshotAnnotationOverrides()
	signatureFields := d.snapshotSignatureFields()
	addedAttachments := d.addedAttachmentSnapshot()
	deletedAttachments := d.deletedAttachmentSnapshot()

	allocator := newNativeObjectAllocator(ctx.objectCount)
	allocator.ObserveRef(ctx.rootRef)
	for ref := range updates {
		allocator.ObserveRef(ref)
	}

	if outlinesSet {
		if err := d.collectNativeOutlineUpdates(outlines, ctx.rootRef, allocator, updates); err != nil {
			return err
		}
	}

	if len(annotationOverrides) > 0 {
		if err := d.collectNativeAnnotationUpdates(annotationOverrides, allocator, updates); err != nil {
			return err
		}
	}
	if len(signatureFields) > 0 {
		if err := d.collectNativeSignatureFieldUpdates(signatureFields, ctx.rootRef, allocator, updates); err != nil {
			return err
		}
	}
	if len(addedAttachments) > 0 || len(deletedAttachments) > 0 {
		if err := d.collectNativeAttachmentUpdates(
			ctx.rootRef,
			addedAttachments,
			deletedAttachments,
			allocator,
			updates,
		); err != nil {
			return err
		}
	}

	if len(updates) == 0 {
		return fmt.Errorf("no session updates to persist")
	}

	out, err := writeNativeIncrementalUpdate(ctx.raw, updates, ctx.rootRef, ctx.prevXRef, ctx.objectCount)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write PDF: %w", err)
	}
	return nil
}

func (d *Document) resolveNativeSaveContext() (*nativeSaveContext, error) {
	x, ok := d.doc.XRef().(sessionPersistenceXRef)
	if !ok {
		return nil, fmt.Errorf("document xref does not support persistence")
	}

	raw := x.RawData()
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty PDF stream")
	}

	prevXRef, err := x.StartXRefOffset()
	if err != nil {
		return nil, fmt.Errorf("resolve startxref: %w", err)
	}

	trailer, err := x.GetTrailer()
	if err != nil {
		return nil, fmt.Errorf("load trailer: %w", err)
	}

	rootRef, err := extractTrailerRootRef(trailer)
	if err != nil {
		return nil, err
	}

	return &nativeSaveContext{
		raw:         raw,
		prevXRef:    prevXRef,
		rootRef:     rootRef,
		objectCount: x.GetNumObjects(),
	}, nil
}

func (d *Document) collectNativeFormUpdates(
	valueOverrides map[string][]string,
	optionOverrides map[string][]string,
) (map[entity.Ref]entity.Object, error) {
	if len(valueOverrides) == 0 && len(optionOverrides) == 0 {
		return map[entity.Ref]entity.Object{}, nil
	}

	entries, err := d.collectNativeFormFieldEntries()
	if err != nil {
		return nil, err
	}

	updates := make(map[entity.Ref]entity.Object)
	for name, entry := range entries {
		values, hasValue := valueOverrides[name]
		options, hasOptions := optionOverrides[name]
		if !hasValue && !hasOptions {
			continue
		}

		if hasOptions && !isChoiceFieldType(entry.fieldType) {
			// Session state may include stale/non-choice overrides; ignore safely.
			hasOptions = false
		}
		if !hasValue && !hasOptions {
			continue
		}

		obj, fetchErr := d.doc.XRef().Fetch(entry.ref)
		if fetchErr != nil {
			return nil, fmt.Errorf("fetch form field %s: %w", name, fetchErr)
		}

		dict, ok := obj.(*entity.Dict)
		if !ok {
			return nil, fmt.Errorf("form field %s is not dictionary: %T", name, obj)
		}

		cloned, ok := dict.Clone().(*entity.Dict)
		if !ok {
			return nil, fmt.Errorf("clone form field %s: unexpected type", name)
		}

		if hasValue {
			cloned.Set(entity.Name("/V"), formValuesToNativeObject(entry.fieldType, values))
		}
		if hasOptions {
			cloned.Set(entity.Name("/Opt"), choiceOptionsToNativeObject(options))
		}
		updates[entry.ref] = cloned
	}

	return updates, nil
}

func (d *Document) snapshotNativeOutlines() ([]*Outline, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.outlinesSet {
		return nil, false
	}

	return cloneOutlines(d.outlines), true
}

func (d *Document) snapshotAnnotationOverrides() map[int][]annotationSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make(map[int][]annotationSnapshot, len(d.annotationOverrides))
	for pageIndex, snapshots := range d.annotationOverrides {
		out[pageIndex] = cloneAnnotationSnapshots(snapshots)
	}
	return out
}

func (d *Document) snapshotSignatureFields() map[string]signatureFieldSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return cloneSignatureFieldSnapshots(d.signatureFields)
}

func (d *Document) collectNativeAttachmentUpdates(
	rootRef entity.Ref,
	added map[string]sessionAttachmentState,
	deleted map[string]bool,
	allocator *nativeObjectAllocator,
	updates map[entity.Ref]entity.Object,
) error {
	base, err := d.baseAttachments()
	if err != nil {
		return err
	}
	final := mergeAttachmentView(base, added, deleted)

	rootDict, err := d.writableCatalogRootDict(rootRef, updates)
	if err != nil {
		return err
	}

	hasNames := rootDict.GetRaw(entity.Name("Names")) != nil || rootDict.GetRaw(entity.Name("/Names")) != nil
	if len(final) == 0 && !hasNames {
		return nil
	}

	namesRef, namesDict, err := d.resolveWritableNamesDict(rootDict, updates)
	if err != nil {
		return err
	}

	if len(final) == 0 {
		pruned := cloneDictWithoutKeys(namesDict, "EmbeddedFiles")
		if namesRef.Num() != 0 {
			updates[namesRef] = pruned
		} else {
			rootDict.Set(entity.Name("Names"), pruned)
		}
		return nil
	}

	sort.Slice(final, func(i, j int) bool {
		leftKey := attachmentOverlayKey(final[i].Name, final[i].FileName)
		rightKey := attachmentOverlayKey(final[j].Name, final[j].FileName)
		if leftKey == rightKey {
			return final[i].FileName < final[j].FileName
		}
		return leftKey < rightKey
	})

	nameItems := make([]entity.Object, 0, len(final)*2)
	for _, att := range final {
		if att == nil {
			continue
		}
		name := strings.TrimSpace(att.Name)
		if name == "" {
			name = strings.TrimSpace(att.FileName)
		}
		if name == "" {
			continue
		}

		streamRef := allocator.Next()
		fileSpecRef := allocator.Next()
		updates[streamRef] = buildNativeAttachmentStream(att)
		updates[fileSpecRef] = buildNativeAttachmentFileSpec(att, streamRef)

		nameItems = append(nameItems, entity.NewString(name), fileSpecRef)
	}

	if len(nameItems) == 0 {
		pruned := cloneDictWithoutKeys(namesDict, "EmbeddedFiles")
		if namesRef.Num() != 0 {
			updates[namesRef] = pruned
		} else {
			rootDict.Set(entity.Name("Names"), pruned)
		}
		return nil
	}

	embeddedRef := allocator.Next()
	embeddedDict := entity.NewDict()
	embeddedDict.Set(entity.Name("Names"), entity.NewArray(nameItems...))
	updates[embeddedRef] = embeddedDict

	rebuiltNames := cloneDictWithoutKeys(namesDict, "EmbeddedFiles")
	rebuiltNames.Set(entity.Name("EmbeddedFiles"), embeddedRef)
	if namesRef.Num() != 0 {
		updates[namesRef] = rebuiltNames
	} else {
		rootDict.Set(entity.Name("Names"), rebuiltNames)
	}
	return nil
}

func (d *Document) resolveWritableNamesDict(
	rootDict *entity.Dict,
	updates map[entity.Ref]entity.Object,
) (entity.Ref, *entity.Dict, error) {
	namesObj := rootDict.GetRaw(entity.Name("Names"))
	if namesObj == nil {
		namesObj = rootDict.GetRaw(entity.Name("/Names"))
	}
	if namesObj == nil {
		namesDict := entity.NewDict()
		rootDict.Set(entity.Name("Names"), namesDict)
		return entity.Ref{}, namesDict, nil
	}

	if namesRef, ok := namesObj.(entity.Ref); ok {
		if existing, found := updates[namesRef]; found {
			dict, ok := existing.(*entity.Dict)
			if !ok {
				return entity.Ref{}, nil, fmt.Errorf("names update is not dictionary: %T", existing)
			}
			return namesRef, dict, nil
		}

		fetched, err := d.doc.XRef().Fetch(namesRef)
		if err != nil {
			return entity.Ref{}, nil, fmt.Errorf("fetch names dictionary: %w", err)
		}
		dict, ok := fetched.(*entity.Dict)
		if !ok {
			return entity.Ref{}, nil, fmt.Errorf("names object is not dictionary: %T", fetched)
		}
		cloned, ok := dict.Clone().(*entity.Dict)
		if !ok {
			return entity.Ref{}, nil, fmt.Errorf("clone names dictionary: unexpected type")
		}
		return namesRef, cloned, nil
	}

	dict, err := d.asDict(namesObj)
	if err != nil {
		return entity.Ref{}, nil, err
	}
	cloned, ok := dict.Clone().(*entity.Dict)
	if !ok {
		return entity.Ref{}, nil, fmt.Errorf("clone inline names dictionary: unexpected type")
	}
	return entity.Ref{}, cloned, nil
}

func cloneDictWithoutKeys(dict *entity.Dict, excluded ...string) *entity.Dict {
	out := entity.NewDict()
	if dict == nil {
		return out
	}

	ignored := make(map[string]struct{}, len(excluded))
	for _, key := range excluded {
		ignored[strings.TrimLeft(key, "/")] = struct{}{}
	}

	for _, key := range dict.Keys() {
		normalized := strings.TrimLeft(key.Value(), "/")
		if _, skip := ignored[normalized]; skip {
			continue
		}
		out.Set(entity.Name(normalized), dict.GetRaw(key))
	}
	return out
}

func buildNativeAttachmentStream(att *Attachment) *entity.Stream {
	data := []byte{}
	if att != nil {
		data = att.Data()
		if data == nil {
			data = []byte{}
		}
	}

	dict := entity.NewDict()
	dict.Set(entity.Name("Type"), entity.NewName("EmbeddedFile"))
	if att != nil {
		mimeType := strings.TrimSpace(strings.TrimPrefix(att.MIMEType, "/"))
		if mimeType != "" {
			dict.Set(entity.Name("Subtype"), entity.NewName(mimeType))
		}
	}
	params := entity.NewDict()
	params.Set(entity.Name("Size"), entity.NewInteger(int64(len(data))))
	dict.Set(entity.Name("Params"), params)
	dict.Set(entity.Name("Length"), entity.NewInteger(int64(len(data))))
	return entity.NewStream(dict, data)
}

func buildNativeAttachmentFileSpec(att *Attachment, streamRef entity.Ref) *entity.Dict {
	dict := entity.NewDict()
	dict.Set(entity.Name("Type"), entity.NewName("Filespec"))

	fileName := ""
	description := ""
	if att != nil {
		fileName = strings.TrimSpace(att.FileName)
		if fileName == "" {
			fileName = strings.TrimSpace(att.Name)
		}
		description = strings.TrimSpace(att.Description)
	}
	if fileName != "" {
		dict.Set(entity.Name("F"), entity.NewString(fileName))
		dict.Set(entity.Name("UF"), entity.NewString(fileName))
	}
	if description != "" {
		dict.Set(entity.Name("Desc"), entity.NewString(description))
	}

	ef := entity.NewDict()
	ef.Set(entity.Name("F"), streamRef)
	ef.Set(entity.Name("UF"), streamRef)
	dict.Set(entity.Name("EF"), ef)
	return dict
}

func (d *Document) collectNativeOutlineUpdates(
	outlines []*Outline,
	rootRef entity.Ref,
	allocator *nativeObjectAllocator,
	updates map[entity.Ref]entity.Object,
) error {
	clonedRoot, err := d.writableCatalogRootDict(rootRef, updates)
	if err != nil {
		return err
	}

	pageRefs, err := d.buildNativePageRefList()
	if err != nil {
		return err
	}

	outlinesRootRef := allocator.Next()
	clonedRoot.Set(entity.Name("/Outlines"), outlinesRootRef)
	updates[rootRef] = clonedRoot

	outlinesRoot := entity.NewDict()
	outlinesRoot.Set(entity.Name("Type"), entity.NewName("Outlines"))

	siblingRefs, err := d.buildNativeOutlineSiblings(outlines, outlinesRootRef, pageRefs, allocator, updates, 0)
	if err != nil {
		return err
	}
	if len(siblingRefs) > 0 {
		outlinesRoot.Set(entity.Name("First"), siblingRefs[0])
		outlinesRoot.Set(entity.Name("Last"), siblingRefs[len(siblingRefs)-1])
		outlinesRoot.Set(entity.Name("Count"), entity.NewInteger(int64(nativeOutlineDescendantCount(outlines))))
	}

	updates[outlinesRootRef] = outlinesRoot
	return nil
}

func (d *Document) writableCatalogRootDict(
	rootRef entity.Ref,
	updates map[entity.Ref]entity.Object,
) (*entity.Dict, error) {
	if existing, ok := updates[rootRef]; ok {
		dict, ok := existing.(*entity.Dict)
		if !ok {
			return nil, fmt.Errorf("catalog root update is not dictionary: %T", existing)
		}
		return dict, nil
	}

	rootObj, err := d.doc.XRef().Fetch(rootRef)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog root: %w", err)
	}

	rootDict, ok := rootObj.(*entity.Dict)
	if !ok {
		return nil, fmt.Errorf("catalog root is not dictionary: %T", rootObj)
	}

	clonedRoot, ok := rootDict.Clone().(*entity.Dict)
	if !ok {
		return nil, fmt.Errorf("clone catalog root: unexpected type")
	}
	updates[rootRef] = clonedRoot
	return clonedRoot, nil
}

func (d *Document) buildNativePageRefList() ([]entity.Ref, error) {
	pageCount, err := d.doc.PageCount()
	if err != nil {
		return nil, err
	}

	refs := make([]entity.Ref, pageCount)
	for i := 0; i < pageCount; i++ {
		page, pageErr := d.doc.GetPage(i)
		if pageErr != nil {
			return nil, pageErr
		}

		ref := page.Ref()
		if ref.Num() == 0 {
			return nil, fmt.Errorf("page %d has no indirect reference", i)
		}
		refs[i] = ref
	}
	return refs, nil
}

func (d *Document) buildNativeOutlineSiblings(
	items []*Outline,
	parentRef entity.Ref,
	pageRefs []entity.Ref,
	allocator *nativeObjectAllocator,
	updates map[entity.Ref]entity.Object,
	depth int,
) ([]entity.Ref, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if depth > 64 {
		return nil, fmt.Errorf("outline depth exceeded")
	}

	refs := make([]entity.Ref, len(items))
	for i := range items {
		refs[i] = allocator.Next()
	}

	for i, item := range items {
		if item == nil {
			item = &Outline{}
		}

		dict := entity.NewDict()
		dict.Set(entity.Name("Title"), entity.NewString(item.Title))
		dict.Set(entity.Name("Parent"), parentRef)
		if i > 0 {
			dict.Set(entity.Name("Prev"), refs[i-1])
		}
		if i+1 < len(refs) {
			dict.Set(entity.Name("Next"), refs[i+1])
		}

		actionObj, actionErr := d.buildNativeOutlineActionObject(item.Action, pageRefs, 0)
		if actionErr != nil {
			return nil, actionErr
		}
		if actionObj != nil {
			dict.Set(entity.Name("A"), actionObj)
		} else if destObj := nativeDestinationObject(item.Dest, item.PageIndex, pageRefs); destObj != nil {
			dict.Set(entity.Name("Dest"), destObj)
		}

		childRefs, childErr := d.buildNativeOutlineSiblings(item.Children, refs[i], pageRefs, allocator, updates, depth+1)
		if childErr != nil {
			return nil, childErr
		}
		if len(childRefs) > 0 {
			dict.Set(entity.Name("First"), childRefs[0])
			dict.Set(entity.Name("Last"), childRefs[len(childRefs)-1])
			count := item.Count
			if count == 0 {
				count = nativeOutlineDescendantCount(item.Children)
			}
			dict.Set(entity.Name("Count"), entity.NewInteger(int64(count)))
		}

		updates[refs[i]] = dict
	}

	return refs, nil
}

func nativeOutlineDescendantCount(items []*Outline) int {
	total := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		total++
		total += nativeOutlineDescendantCount(item.Children)
	}
	return total
}

func (d *Document) buildNativeOutlineActionObject(
	action *OutlineAction,
	pageRefs []entity.Ref,
	depth int,
) (*entity.Dict, error) {
	if action == nil {
		return nil, nil
	}
	if depth > 16 {
		return nil, fmt.Errorf("outline action depth exceeded")
	}

	dict := entity.NewDict()
	if action.Type != "" {
		dict.Set(entity.Name("S"), entity.NewName(action.Type))
	}

	switch action.Type {
	case "URI":
		if action.URI != "" {
			dict.Set(entity.Name("URI"), entity.NewString(action.URI))
		}
	case "GoTo":
		if dest := nativeDestinationObject(action.Dest, action.PageIndex, pageRefs); dest != nil {
			dict.Set(entity.Name("D"), dest)
		}
	case "GoToR":
		if action.File != "" {
			dict.Set(entity.Name("F"), entity.NewString(action.File))
		}
		if dest := nativeDestinationObject(action.Dest, action.PageIndex, pageRefs); dest != nil {
			dict.Set(entity.Name("D"), dest)
		}
		if action.HasNewWindow {
			dict.Set(entity.Name("NewWindow"), entity.NewBoolean(action.NewWindow))
		}
	case "Named":
		if action.Named != "" {
			dict.Set(entity.Name("N"), entity.NewName(action.Named))
		}
	case "Launch":
		if action.File != "" {
			dict.Set(entity.Name("F"), entity.NewString(action.File))
		}
		if action.Command != "" {
			dict.Set(entity.Name("P"), entity.NewString(action.Command))
		}
		if action.Directory != "" {
			dict.Set(entity.Name("D"), entity.NewString(action.Directory))
		}
		if action.Operation != "" {
			dict.Set(entity.Name("O"), entity.NewString(action.Operation))
		}
	case "JavaScript":
		if action.JavaScript != "" {
			dict.Set(entity.Name("JS"), entity.NewString(action.JavaScript))
		}
	case "SubmitForm", "ImportData":
		if action.File != "" {
			dict.Set(entity.Name("F"), entity.NewString(action.File))
		}
		if len(action.FieldNames) > 0 {
			fieldItems := make([]entity.Object, 0, len(action.FieldNames))
			for _, field := range action.FieldNames {
				fieldItems = append(fieldItems, entity.NewString(field))
			}
			dict.Set(entity.Name("Fields"), entity.NewArray(fieldItems...))
		}
		if action.Flags != 0 {
			dict.Set(entity.Name("Flags"), entity.NewInteger(int64(action.Flags)))
		}
	case "ResetForm":
		if len(action.FieldNames) > 0 {
			fieldItems := make([]entity.Object, 0, len(action.FieldNames))
			for _, field := range action.FieldNames {
				fieldItems = append(fieldItems, entity.NewString(field))
			}
			dict.Set(entity.Name("Fields"), entity.NewArray(fieldItems...))
		}
		if action.Flags != 0 {
			dict.Set(entity.Name("Flags"), entity.NewInteger(int64(action.Flags)))
		}
	case "Hide":
		if action.HasHide {
			dict.Set(entity.Name("H"), entity.NewBoolean(action.Hide))
		}
		targetObj := nativeNamesObject(action.HideTargets)
		if targetObj != nil {
			dict.Set(entity.Name("T"), targetObj)
		}
	case "Rendition":
		if action.RenditionOperation != 0 {
			dict.Set(entity.Name("OP"), entity.NewInteger(int64(action.RenditionOperation)))
		}
		if action.RenditionName != "" || action.RenditionFile != "" || action.RenditionMIMEType != "" {
			rendition := entity.NewDict()
			if action.RenditionName != "" {
				rendition.Set(entity.Name("N"), entity.NewString(action.RenditionName))
			}
			if action.RenditionMIMEType != "" {
				rendition.Set(entity.Name("CT"), entity.NewString(action.RenditionMIMEType))
			}
			if action.RenditionFile != "" {
				clip := entity.NewDict()
				fileSpec := entity.NewDict()
				fileSpec.Set(entity.Name("F"), entity.NewString(action.RenditionFile))
				clip.Set(entity.Name("D"), fileSpec)
				if action.RenditionMIMEType != "" {
					clip.Set(entity.Name("CT"), entity.NewString(action.RenditionMIMEType))
				}
				rendition.Set(entity.Name("C"), clip)
			}
			dict.Set(entity.Name("R"), rendition)
		}
	default:
		if action.URI != "" {
			dict.Set(entity.Name("URI"), entity.NewString(action.URI))
		}
		if action.Named != "" {
			dict.Set(entity.Name("N"), entity.NewName(action.Named))
		}
		if action.File != "" {
			dict.Set(entity.Name("F"), entity.NewString(action.File))
		}
		if action.JavaScript != "" {
			dict.Set(entity.Name("JS"), entity.NewString(action.JavaScript))
		}
		if dest := nativeDestinationObject(action.Dest, action.PageIndex, pageRefs); dest != nil {
			dict.Set(entity.Name("D"), dest)
		}
	}

	if len(action.NextActions) > 0 {
		if len(action.NextActions) == 1 {
			nextAction, err := d.buildNativeOutlineActionObject(action.NextActions[0], pageRefs, depth+1)
			if err != nil {
				return nil, err
			}
			if nextAction != nil {
				dict.Set(entity.Name("Next"), nextAction)
			}
		} else {
			nextItems := make([]entity.Object, 0, len(action.NextActions))
			for _, next := range action.NextActions {
				nextAction, err := d.buildNativeOutlineActionObject(next, pageRefs, depth+1)
				if err != nil {
					return nil, err
				}
				if nextAction != nil {
					nextItems = append(nextItems, nextAction)
				}
			}
			if len(nextItems) > 0 {
				dict.Set(entity.Name("Next"), entity.NewArray(nextItems...))
			}
		}
	}

	return dict, nil
}

func nativeNamesObject(values []string) entity.Object {
	if len(values) == 0 {
		return nil
	}

	normalized := normalizeFormFieldValues(values)
	if len(normalized) == 0 {
		return nil
	}
	if len(normalized) == 1 {
		return entity.NewString(normalized[0])
	}

	items := make([]entity.Object, 0, len(normalized))
	for _, value := range normalized {
		items = append(items, entity.NewString(value))
	}
	return entity.NewArray(items...)
}

func nativeDestinationObject(dest Object, pageIndex int, pageRefs []entity.Ref) entity.Object {
	if entityDest, ok := nativePublicObjectToEntity(dest); ok {
		return entityDest
	}
	if pageIndex >= 0 && pageIndex < len(pageRefs) {
		return entity.NewArray(pageRefs[pageIndex], entity.NewName("Fit"))
	}
	return nil
}

func nativePublicObjectToEntity(obj Object) (entity.Object, bool) {
	switch v := obj.(type) {
	case nil:
		return nil, false
	case entity.Object:
		return v.Clone(), true
	case *Dict:
		if v == nil || v.dict == nil {
			return nil, false
		}
		return v.dict.Clone(), true
	case *Array:
		if v == nil || v.array == nil {
			return nil, false
		}
		return v.array.Clone(), true
	case string:
		return entity.NewString(v), true
	case bool:
		return entity.NewBoolean(v), true
	case int:
		return entity.NewInteger(int64(v)), true
	case int8:
		return entity.NewInteger(int64(v)), true
	case int16:
		return entity.NewInteger(int64(v)), true
	case int32:
		return entity.NewInteger(int64(v)), true
	case int64:
		return entity.NewInteger(v), true
	case uint:
		return entity.NewInteger(int64(v)), true
	case uint8:
		return entity.NewInteger(int64(v)), true
	case uint16:
		return entity.NewInteger(int64(v)), true
	case uint32:
		return entity.NewInteger(int64(v)), true
	case uint64:
		return entity.NewInteger(int64(v)), true
	case float32:
		return entity.NewReal(float64(v)), true
	case float64:
		return entity.NewReal(v), true
	default:
		return nil, false
	}
}

func (d *Document) collectNativeAnnotationUpdates(
	overrides map[int][]annotationSnapshot,
	allocator *nativeObjectAllocator,
	updates map[entity.Ref]entity.Object,
) error {
	pageIndexes := make([]int, 0, len(overrides))
	for pageIndex := range overrides {
		pageIndexes = append(pageIndexes, pageIndex)
	}
	sort.Ints(pageIndexes)

	for _, pageIndex := range pageIndexes {
		page, err := d.doc.GetPage(pageIndex)
		if err != nil {
			return err
		}

		pageRef := page.Ref()
		if pageRef.Num() == 0 {
			return fmt.Errorf("page %d has no indirect reference", pageIndex)
		}

		pageDict := page.Dict()
		if pageDict == nil {
			return fmt.Errorf("page %d dictionary is nil", pageIndex)
		}

		clonedPage, ok := pageDict.Clone().(*entity.Dict)
		if !ok {
			return fmt.Errorf("clone page dictionary %d: unexpected type", pageIndex)
		}

		snapshots := overrides[pageIndex]
		annotRefs := make([]entity.Object, 0, len(snapshots))
		for _, snapshot := range snapshots {
			annotRef := allocator.Next()
			annotRefs = append(annotRefs, annotRef)
			appearanceRef := allocator.Next()
			updates[appearanceRef] = buildNativeAnnotationAppearanceStream(snapshot)
			updates[annotRef] = buildNativeAnnotationDict(snapshot, pageRef, appearanceRef)
		}

		clonedPage.Set(entity.Name("Annots"), entity.NewArray(annotRefs...))
		updates[pageRef] = clonedPage
	}

	return nil
}

func buildNativeAnnotationDict(snapshot annotationSnapshot, pageRef entity.Ref, appearanceRef entity.Ref) *entity.Dict {
	dict := entity.NewDict()
	dict.Set(entity.Name("Type"), entity.NewName("Annot"))

	subtype := strings.TrimSpace(strings.TrimLeft(snapshot.Type, "/"))
	if subtype == "" {
		subtype = "Text"
	}
	dict.Set(entity.Name("Subtype"), entity.NewName(subtype))

	rectItems := []entity.Object{
		entity.NewReal(snapshot.Rect[0]),
		entity.NewReal(snapshot.Rect[1]),
		entity.NewReal(snapshot.Rect[2]),
		entity.NewReal(snapshot.Rect[3]),
	}
	dict.Set(entity.Name("Rect"), entity.NewArray(rectItems...))
	dict.Set(entity.Name("P"), pageRef)
	dict.Set(entity.Name("Contents"), entity.NewString(snapshot.Contents))
	if snapshot.Name != "" {
		dict.Set(entity.Name("NM"), entity.NewString(snapshot.Name))
	}
	if len(snapshot.PathList) > 0 {
		inkItems := make([]entity.Object, 0, len(snapshot.PathList))
		for _, path := range snapshot.PathList {
			if len(path) == 0 {
				continue
			}
			pathItems := make([]entity.Object, 0, len(path))
			for _, value := range path {
				pathItems = append(pathItems, entity.NewReal(value))
			}
			inkItems = append(inkItems, entity.NewArray(pathItems...))
		}
		if len(inkItems) > 0 {
			dict.Set(entity.Name("InkList"), entity.NewArray(inkItems...))
		}
		if len(snapshot.PathList) == 1 && len(snapshot.PathList[0]) > 0 {
			vertexItems := make([]entity.Object, 0, len(snapshot.PathList[0]))
			for _, value := range snapshot.PathList[0] {
				vertexItems = append(vertexItems, entity.NewReal(value))
			}
			dict.Set(entity.Name("Vertices"), entity.NewArray(vertexItems...))
		}
	}

	pointsSource := snapshot.PgPoints
	if len(pointsSource) == 0 {
		for _, path := range snapshot.PathList {
			pointsSource = append(pointsSource, path...)
		}
	}
	if len(pointsSource) > 0 {
		points := make([]entity.Object, 0, len(pointsSource))
		for _, value := range pointsSource {
			points = append(points, entity.NewReal(value))
		}
		dict.Set(entity.Name("PgPts"), entity.NewArray(points...))
	}
	if len(snapshot.HeadPoints) > 0 {
		headPoints := make([]entity.Object, 0, len(snapshot.HeadPoints))
		for _, value := range snapshot.HeadPoints {
			headPoints = append(headPoints, entity.NewReal(value))
		}
		dict.Set(entity.Name("HeadPts"), entity.NewArray(headPoints...))
	}
	if userData := buildAnnotationUserDataDict(snapshot.UserData); userData != nil {
		dict.Set(entity.Name("UD"), userData)
		if cloned, ok := userData.Clone().(*entity.Dict); ok {
			dict.Set(entity.Name("UserData"), cloned)
		}
	}
	if appearanceRef.Num() != 0 {
		appearance := entity.NewDict()
		appearance.Set(entity.Name("N"), appearanceRef)
		dict.Set(entity.Name("AP"), appearance)
		dict.Set(entity.Name("AS"), entity.NewName("N"))
	}

	return dict
}

func buildNativeAnnotationAppearanceStream(snapshot annotationSnapshot) *entity.Stream {
	width := snapshot.Rect[2] - snapshot.Rect[0]
	height := snapshot.Rect[3] - snapshot.Rect[1]
	if width < 0 {
		width = -width
	}
	if height < 0 {
		height = -height
	}
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	content := buildAnnotationAppearanceContent(snapshot, width, height)
	streamDict := entity.NewDict()
	streamDict.Set(entity.Name("Type"), entity.NewName("XObject"))
	streamDict.Set(entity.Name("Subtype"), entity.NewName("Form"))
	streamDict.Set(entity.Name("FormType"), entity.NewInteger(1))
	streamDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(width),
		entity.NewReal(height),
	))
	streamDict.Set(entity.Name("Resources"), entity.NewDict())
	streamDict.Set(entity.Name("Length"), entity.NewInteger(int64(len(content))))

	return entity.NewStream(streamDict, []byte(content))
}

func (d *Document) collectNativeSignatureFieldUpdates(
	fields map[string]signatureFieldSnapshot,
	rootRef entity.Ref,
	allocator *nativeObjectAllocator,
	updates map[entity.Ref]entity.Object,
) error {
	rootDict, err := d.writableCatalogRootDict(rootRef, updates)
	if err != nil {
		return err
	}

	acroRef, acroDict, err := d.resolveWritableAcroForm(rootDict, allocator, updates)
	if err != nil {
		return err
	}
	allocator.ObserveRef(acroRef)

	fieldsObj := acroDict.Get(entity.Name("Fields"))
	var fieldsArr *entity.Array
	if fieldsObj != nil {
		arr, arrErr := d.asArray(fieldsObj)
		if arrErr != nil {
			return arrErr
		}
		items := make([]entity.Object, 0, arr.Len())
		for i := 0; i < arr.Len(); i++ {
			items = append(items, arr.Get(i))
		}
		fieldsArr = entity.NewArray(items...)
	} else {
		fieldsArr = entity.NewArray()
	}

	pageRefs := make(map[int]entity.Ref, len(fields))
	for _, field := range fields {
		page, pageErr := d.doc.GetPage(field.PageIndex)
		if pageErr != nil {
			return pageErr
		}
		pageRef := page.Ref()
		if pageRef.Num() == 0 {
			return fmt.Errorf("signature page %d has no indirect reference", field.PageIndex)
		}
		pageRefs[field.PageIndex] = pageRef
	}

	for _, field := range sortedSignatureFieldSnapshots(fields) {
		pageRef := pageRefs[field.PageIndex]
		valueRef := allocator.Next()
		widgetRef := allocator.Next()
		appearanceRef := allocator.Next()

		updates[valueRef] = buildNativeSignatureValueDict(field)
		updates[appearanceRef] = buildNativeSignatureAppearanceStream(field)
		updates[widgetRef] = buildNativeSignatureWidgetDict(field, pageRef, valueRef, appearanceRef)

		fieldItems := append(fieldsArr.Items(), widgetRef)
		fieldsArr = entity.NewArray(fieldItems...)

		if err := d.appendPageAnnotationRef(pageRef, widgetRef, updates); err != nil {
			return err
		}
	}

	acroDict.Set(entity.Name("Fields"), fieldsArr)
	acroDict.Set(entity.Name("SigFlags"), entity.NewInteger(3))
	updates[acroRef] = acroDict
	return nil
}

func sortedSignatureFieldSnapshots(fields map[string]signatureFieldSnapshot) []signatureFieldSnapshot {
	out := make([]signatureFieldSnapshot, 0, len(fields))
	for _, item := range fields {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FieldName == out[j].FieldName {
			return out[i].PageIndex < out[j].PageIndex
		}
		return out[i].FieldName < out[j].FieldName
	})
	return out
}

func (d *Document) resolveWritableAcroForm(
	rootDict *entity.Dict,
	allocator *nativeObjectAllocator,
	updates map[entity.Ref]entity.Object,
) (entity.Ref, *entity.Dict, error) {
	acroObj := rootDict.GetRaw(entity.Name("AcroForm"))
	if acroObj == nil {
		acroObj = rootDict.GetRaw(entity.Name("/AcroForm"))
	}
	if acroObj == nil {
		ref := allocator.Next()
		rootDict.Set(entity.Name("AcroForm"), ref)
		acroDict := entity.NewDict()
		acroDict.Set(entity.Name("Fields"), entity.NewArray())
		return ref, acroDict, nil
	}

	if acroRef, ok := acroObj.(entity.Ref); ok {
		if existing, ok := updates[acroRef]; ok {
			dict, ok := existing.(*entity.Dict)
			if !ok {
				return entity.Ref{}, nil, fmt.Errorf("acroform update is not dictionary: %T", existing)
			}
			return acroRef, dict, nil
		}

		fetched, err := d.doc.XRef().Fetch(acroRef)
		if err != nil {
			return entity.Ref{}, nil, fmt.Errorf("fetch acroform: %w", err)
		}
		dict, ok := fetched.(*entity.Dict)
		if !ok {
			return entity.Ref{}, nil, fmt.Errorf("acroform is not dictionary: %T", fetched)
		}
		cloned, ok := dict.Clone().(*entity.Dict)
		if !ok {
			return entity.Ref{}, nil, fmt.Errorf("clone acroform: unexpected type")
		}
		return acroRef, cloned, nil
	}

	acroDict, err := d.asDict(acroObj)
	if err != nil {
		return entity.Ref{}, nil, err
	}
	cloned, ok := acroDict.Clone().(*entity.Dict)
	if !ok {
		return entity.Ref{}, nil, fmt.Errorf("clone inline acroform: unexpected type")
	}

	ref := allocator.Next()
	rootDict.Set(entity.Name("AcroForm"), ref)
	return ref, cloned, nil
}

func (d *Document) appendPageAnnotationRef(
	pageRef entity.Ref,
	annotRef entity.Ref,
	updates map[entity.Ref]entity.Object,
) error {
	pageDict, err := d.resolveWritablePageDict(pageRef, updates)
	if err != nil {
		return err
	}

	annotsObj := pageDict.GetRaw(entity.Name("Annots"))
	if annotsObj == nil {
		annotsObj = pageDict.GetRaw(entity.Name("/Annots"))
	}
	var items []entity.Object
	if annotsObj != nil {
		arr, arrErr := d.asArray(annotsObj)
		if arrErr != nil {
			return arrErr
		}
		items = make([]entity.Object, 0, arr.Len()+1)
		for i := 0; i < arr.Len(); i++ {
			items = append(items, arr.Get(i))
		}
	} else {
		items = make([]entity.Object, 0, 1)
	}
	items = append(items, annotRef)
	pageDict.Set(entity.Name("Annots"), entity.NewArray(items...))
	updates[pageRef] = pageDict
	return nil
}

func (d *Document) resolveWritablePageDict(
	pageRef entity.Ref,
	updates map[entity.Ref]entity.Object,
) (*entity.Dict, error) {
	if existing, ok := updates[pageRef]; ok {
		dict, ok := existing.(*entity.Dict)
		if !ok {
			return nil, fmt.Errorf("page update is not dictionary: %T", existing)
		}
		return dict, nil
	}

	pageObj, err := d.doc.XRef().Fetch(pageRef)
	if err != nil {
		return nil, err
	}
	pageDict, ok := pageObj.(*entity.Dict)
	if !ok {
		return nil, fmt.Errorf("page is not dictionary: %T", pageObj)
	}
	cloned, ok := pageDict.Clone().(*entity.Dict)
	if !ok {
		return nil, fmt.Errorf("clone page dictionary: unexpected type")
	}
	updates[pageRef] = cloned
	return cloned, nil
}

func buildNativeSignatureValueDict(field signatureFieldSnapshot) *entity.Dict {
	dict := entity.NewDict()
	dict.Set(entity.Name("Type"), entity.NewName("Sig"))
	dict.Set(entity.Name("Filter"), entity.NewName("Adobe.PPKLite"))
	dict.Set(entity.Name("SubFilter"), entity.NewName("adbe.pkcs7.detached"))

	modifiedAt := field.ModifiedAt
	if strings.TrimSpace(modifiedAt) == "" {
		modifiedAt = time.Now().UTC().Format("D:20060102150405Z")
	}
	dict.Set(entity.Name("M"), entity.NewString(modifiedAt))
	if field.Name != "" {
		dict.Set(entity.Name("Name"), entity.NewString(field.Name))
	}
	if field.Reason != "" {
		dict.Set(entity.Name("Reason"), entity.NewString(field.Reason))
	}
	if field.Location != "" {
		dict.Set(entity.Name("Location"), entity.NewString(field.Location))
	}

	byteRange := field.ByteRange
	if len(byteRange) != 4 {
		byteRange = []int64{0, 0, 0, 0}
	}
	dict.Set(entity.Name("ByteRange"), entity.NewArray(
		entity.NewInteger(byteRange[0]),
		entity.NewInteger(byteRange[1]),
		entity.NewInteger(byteRange[2]),
		entity.NewInteger(byteRange[3]),
	))
	dict.Set(entity.Name("Contents"), entity.NewString(string(field.Contents)))
	return dict
}

func buildNativeSignatureWidgetDict(
	field signatureFieldSnapshot,
	pageRef entity.Ref,
	valueRef entity.Ref,
	appearanceRef entity.Ref,
) *entity.Dict {
	dict := entity.NewDict()
	dict.Set(entity.Name("Type"), entity.NewName("Annot"))
	dict.Set(entity.Name("Subtype"), entity.NewName("Widget"))
	dict.Set(entity.Name("FT"), entity.NewName("Sig"))
	dict.Set(entity.Name("T"), entity.NewString(field.FieldName))
	dict.Set(entity.Name("F"), entity.NewInteger(4))
	dict.Set(entity.Name("P"), pageRef)
	dict.Set(entity.Name("V"), valueRef)
	dict.Set(entity.Name("Rect"), entity.NewArray(
		entity.NewReal(field.Rect[0]),
		entity.NewReal(field.Rect[1]),
		entity.NewReal(field.Rect[2]),
		entity.NewReal(field.Rect[3]),
	))

	if appearanceRef.Num() != 0 {
		ap := entity.NewDict()
		ap.Set(entity.Name("N"), appearanceRef)
		dict.Set(entity.Name("AP"), ap)
		dict.Set(entity.Name("AS"), entity.NewName("N"))
	}

	return dict
}

func buildNativeSignatureAppearanceStream(field signatureFieldSnapshot) *entity.Stream {
	width := field.Rect[2] - field.Rect[0]
	height := field.Rect[3] - field.Rect[1]
	if width < 0 {
		width = -width
	}
	if height < 0 {
		height = -height
	}
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	content := fmt.Sprintf(
		"q\n0.95 0.95 0.95 rg\n0 0 %.2f %.2f re\nf\n0 0 0 RG\n1 w\n0 0 %.2f %.2f re\nS\nQ\n",
		width, height, width, height,
	)

	streamDict := entity.NewDict()
	streamDict.Set(entity.Name("Type"), entity.NewName("XObject"))
	streamDict.Set(entity.Name("Subtype"), entity.NewName("Form"))
	streamDict.Set(entity.Name("FormType"), entity.NewInteger(1))
	streamDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(width),
		entity.NewReal(height),
	))
	streamDict.Set(entity.Name("Resources"), entity.NewDict())
	streamDict.Set(entity.Name("Length"), entity.NewInteger(int64(len(content))))
	return entity.NewStream(streamDict, []byte(content))
}

func buildAnnotationAppearanceContent(snapshot annotationSnapshot, width, height float64) string {
	subtype := strings.ToLower(strings.TrimSpace(strings.TrimLeft(snapshot.Type, "/")))
	switch subtype {
	case "highlight":
		return fmt.Sprintf("q\n1 1 0 rg\n0 0 %.2f %.2f re\nf\nQ\n", width, height)
	case "text":
		return fmt.Sprintf(
			"q\n1 1 0 rg\n0 0 %.2f %.2f re\nf\n0 0 0 RG\n1 w\n0 0 %.2f %.2f re\nS\nQ\n",
			width, height, width, height,
		)
	default:
		return fmt.Sprintf(
			"q\n1 1 1 rg\n0 0 %.2f %.2f re\nf\n0 0 0 RG\n0.75 w\n0 0 %.2f %.2f re\nS\nQ\n",
			width, height, width, height,
		)
	}
}

func (d *Document) collectNativeFormFieldEntries() (map[string]nativeFieldEntry, error) {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return map[string]nativeFieldEntry{}, nil
	}

	acroObj := catalog.Get(entity.Name("AcroForm"))
	if acroObj == nil {
		return map[string]nativeFieldEntry{}, nil
	}
	acroDict, err := d.asDict(acroObj)
	if err != nil {
		return nil, err
	}

	fieldsObj := acroDict.Get(entity.Name("Fields"))
	if fieldsObj == nil {
		return map[string]nativeFieldEntry{}, nil
	}
	fieldsArr, err := d.asArray(fieldsObj)
	if err != nil {
		return nil, err
	}

	out := make(map[string]nativeFieldEntry)
	visited := make(map[*entity.Dict]struct{})
	for i := 0; i < fieldsArr.Len(); i++ {
		if err := d.collectNativeFieldNode(fieldsArr.Get(i), "", inheritedFieldAttrs{}, visited, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (d *Document) collectNativeFieldNode(
	obj entity.Object,
	parentName string,
	parentAttrs inheritedFieldAttrs,
	visited map[*entity.Dict]struct{},
	out map[string]nativeFieldEntry,
) error {
	var (
		ref    entity.Ref
		hasRef bool
	)

	if v, ok := obj.(entity.Ref); ok {
		ref = v
		hasRef = true
	}

	dict, err := d.asDict(obj)
	if err != nil {
		return err
	}
	if _, seen := visited[dict]; seen {
		return nil
	}
	visited[dict] = struct{}{}

	attrs := mergeFieldAttrs(dict, parentAttrs)
	partial := extractEntityString(dict.Get(entity.Name("T")))
	fullName := parentName
	if partial != "" {
		if fullName == "" {
			fullName = partial
		} else {
			fullName = fullName + "." + partial
		}
	}
	if fullName != "" && hasRef {
		out[fullName] = nativeFieldEntry{
			ref:       ref,
			fieldType: attrs.fieldType,
		}
	}

	kidsObj := dict.Get(entity.Name("Kids"))
	if kidsObj == nil {
		return nil
	}

	kidsArr, err := d.asArray(kidsObj)
	if err != nil {
		return err
	}
	for i := 0; i < kidsArr.Len(); i++ {
		if err := d.collectNativeFieldNode(kidsArr.Get(i), fullName, attrs, visited, out); err != nil {
			return err
		}
	}
	return nil
}

func formValuesToNativeObject(fieldType string, values []string) entity.Object {
	normalized := normalizeFormFieldValues(values)
	if len(normalized) == 0 {
		normalized = []string{""}
	}

	if fieldType == "Btn" {
		name := normalized[0]
		if strings.TrimSpace(name) == "" {
			name = "Off"
		}
		return entity.NewName(name)
	}

	if len(normalized) == 1 {
		return entity.NewString(normalized[0])
	}

	items := make([]entity.Object, 0, len(normalized))
	for _, value := range normalized {
		items = append(items, entity.NewString(value))
	}
	return entity.NewArray(items...)
}

func choiceOptionsToNativeObject(options []string) entity.Object {
	normalized := normalizeChoiceFieldOptions(options)
	items := make([]entity.Object, 0, len(normalized))
	for _, option := range normalized {
		items = append(items, entity.NewString(option))
	}
	return entity.NewArray(items...)
}

func isChoiceFieldType(fieldType string) bool {
	normalized := strings.TrimSpace(strings.TrimPrefix(fieldType, "/"))
	return normalized == "Ch"
}

func extractTrailerRootRef(trailer *entity.Dict) (entity.Ref, error) {
	if trailer == nil {
		return entity.Ref{}, fmt.Errorf("trailer is nil")
	}

	if rootObj := trailer.GetRaw(entity.Name("/Root")); rootObj != nil {
		if rootRef, ok := rootObj.(entity.Ref); ok {
			return rootRef, nil
		}
	}
	if rootObj := trailer.GetRaw(entity.Name("Root")); rootObj != nil {
		if rootRef, ok := rootObj.(entity.Ref); ok {
			return rootRef, nil
		}
	}

	for _, key := range trailer.Keys() {
		if strings.TrimLeft(key.Value(), "/") != "Root" {
			continue
		}
		rootObj := trailer.GetRaw(key)
		if rootRef, ok := rootObj.(entity.Ref); ok {
			return rootRef, nil
		}
	}

	return entity.Ref{}, fmt.Errorf("trailer /Root is not reference")
}

func writeNativeIncrementalUpdate(
	raw []byte,
	updates map[entity.Ref]entity.Object,
	rootRef entity.Ref,
	prevXRef uint64,
	objectCount int,
) ([]byte, error) {
	var out bytes.Buffer
	out.Write(raw)
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		out.WriteByte('\n')
	}

	refs := make([]entity.Ref, 0, len(updates))
	for ref := range updates {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Num() == refs[j].Num() {
			return refs[i].Gen() < refs[j].Gen()
		}
		return refs[i].Num() < refs[j].Num()
	})

	entries := make([]xrefEntryUpdate, 0, len(refs))
	for _, ref := range refs {
		encoded, err := encodePDFObject(updates[ref])
		if err != nil {
			return nil, fmt.Errorf("encode object %d %d: %w", ref.Num(), ref.Gen(), err)
		}

		offset := out.Len()
		fmt.Fprintf(&out, "%d %d obj\n", ref.Num(), ref.Gen())
		out.WriteString(encoded)
		out.WriteString("\nendobj\n")

		entries = append(entries, xrefEntryUpdate{
			num:    ref.Num(),
			gen:    ref.Gen(),
			offset: offset,
		})
	}

	xrefOffset := out.Len()
	out.WriteString("xref\n")
	writeXRefSubsections(&out, entries)

	size := objectCount
	for _, ref := range refs {
		if int(ref.Num())+1 > size {
			size = int(ref.Num()) + 1
		}
	}

	fmt.Fprintf(
		&out,
		"trailer\n<< /Size %d /Root %d %d R /Prev %d >>\n",
		size,
		rootRef.Num(),
		rootRef.Gen(),
		prevXRef,
	)
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return out.Bytes(), nil
}

func writeXRefSubsections(out *bytes.Buffer, entries []xrefEntryUpdate) {
	if len(entries) == 0 {
		return
	}

	start := 0
	for i := 1; i <= len(entries); i++ {
		contiguous := i < len(entries) && entries[i].num == entries[i-1].num+1
		if contiguous {
			continue
		}

		sub := entries[start:i]
		fmt.Fprintf(out, "%d %d\n", sub[0].num, len(sub))
		for _, entry := range sub {
			fmt.Fprintf(out, "%010d %05d n \n", entry.offset, entry.gen)
		}
		start = i
	}
}

func encodePDFObject(obj entity.Object) (string, error) {
	switch v := obj.(type) {
	case nil:
		return "null", nil
	case *entity.Boolean:
		if v.Value() {
			return "true", nil
		}
		return "false", nil
	case *entity.Integer:
		return fmt.Sprintf("%d", v.Value()), nil
	case *entity.Real:
		return fmt.Sprintf("%.6f", v.Value()), nil
	case *entity.String:
		return encodePDFLiteralString(v.Value()), nil
	case entity.Name:
		return encodePDFName(v.Value()), nil
	case entity.Ref:
		return fmt.Sprintf("%d %d R", v.Num(), v.Gen()), nil
	case *entity.Null:
		return "null", nil
	case *entity.Array:
		var b strings.Builder
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			item, err := encodePDFObject(v.Get(i))
			if err != nil {
				return "", err
			}
			b.WriteString(item)
		}
		b.WriteByte(']')
		return b.String(), nil
	case *entity.Dict:
		keys := v.Keys()
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].Value() < keys[j].Value()
		})

		var b strings.Builder
		b.WriteString("<<")
		for _, key := range keys {
			value := v.GetRaw(key)
			encodedValue, err := encodePDFObject(value)
			if err != nil {
				return "", err
			}
			b.WriteByte(' ')
			b.WriteString(encodePDFName(key.Value()))
			b.WriteByte(' ')
			b.WriteString(encodedValue)
		}
		if len(keys) > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(">>")
		return b.String(), nil
	case *entity.Stream:
		dictEncoded, err := encodePDFObject(v.Dict())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s\nstream\n%s\nendstream", dictEncoded, string(v.RawBytes())), nil
	default:
		return "", fmt.Errorf("unsupported object type: %T", obj)
	}
}

func encodePDFName(value string) string {
	normalized := strings.TrimLeft(value, "/")
	if normalized == "" {
		return "/"
	}

	var b strings.Builder
	b.WriteByte('/')
	for i := 0; i < len(normalized); i++ {
		ch := normalized[i]
		if shouldEscapeNameChar(ch) {
			fmt.Fprintf(&b, "#%02X", ch)
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func shouldEscapeNameChar(ch byte) bool {
	if ch <= 0x20 || ch >= 0x7f {
		return true
	}
	switch ch {
	case '#', '%', '(', ')', '<', '>', '[', ']', '{', '}', '/', ' ':
		return true
	default:
		return false
	}
}

func encodePDFLiteralString(value string) string {
	var b strings.Builder
	b.WriteByte('(')
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			b.WriteString("\\\\")
		case '(':
			b.WriteString("\\(")
		case ')':
			b.WriteString("\\)")
		case '\r':
			b.WriteString("\\r")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		default:
			b.WriteByte(value[i])
		}
	}
	b.WriteByte(')')
	return b.String()
}
