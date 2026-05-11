package pdf

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// Attachment represents one embedded file in the PDF Names/EmbeddedFiles tree.
type Attachment struct {
	data        []byte
	Description string
	FileName    string
	MIMEType    string
	Name        string
	Size        int
}

// AttachmentSpec defines attachment payload for session-level add operation.
type AttachmentSpec struct {
	Data        []byte
	Description string
	FileName    string
	MIMEType    string
	Name        string
}

type sessionAttachmentState struct {
	Data        []byte `json:"data,omitempty"`
	Description string `json:"description,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	Name        string `json:"name,omitempty"`
	Size        int    `json:"size,omitempty"`
}

// AttachedFileList is a Java-parity alias of Attachments.
func (d *Document) AttachedFileList() ([]*Attachment, error) {
	return d.Attachments()
}

// GetAttachedFileList is a Java-parity alias of AttachedFileList.
func (d *Document) GetAttachedFileList() ([]*Attachment, error) {
	return d.AttachedFileList()
}

// Data returns a copy of the decoded attachment bytes.
func (a *Attachment) Data() []byte {
	if a == nil || len(a.data) == 0 {
		return nil
	}
	out := make([]byte, len(a.data))
	copy(out, a.data)
	return out
}

// AddAttachment adds an attachment in the current session overlay.
func (d *Document) AddAttachment(spec AttachmentSpec) error {
	name := strings.TrimSpace(spec.Name)
	fileName := strings.TrimSpace(spec.FileName)
	if name == "" && fileName == "" {
		return fmt.Errorf("attachment name or file name is required")
	}
	if name == "" {
		name = fileName
	}
	if fileName == "" {
		fileName = name
	}
	key := attachmentOverlayKey(name, fileName)
	if key == "" {
		return fmt.Errorf("attachment key is empty")
	}

	mimeType := strings.TrimSpace(spec.MIMEType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileName))
	}

	data := append([]byte(nil), spec.Data...)
	state := sessionAttachmentState{
		Data:        data,
		Description: spec.Description,
		FileName:    fileName,
		MIMEType:    mimeType,
		Name:        name,
		Size:        len(data),
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.addedAttachments[key] = state
	delete(d.deletedAttachments, key)
	delete(d.deletedAttachments, normalizeAttachmentKey(name))
	delete(d.deletedAttachments, normalizeAttachmentKey(fileName))
	return nil
}

// AddAttachmentFromFile adds an attachment from local file in the current session overlay.
func (d *Document) AddAttachmentFromFile(name, path string) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("attachment path is empty")
	}
	data, err := os.ReadFile(trimmedPath)
	if err != nil {
		return fmt.Errorf("read attachment file: %w", err)
	}
	fileName := filepath.Base(trimmedPath)
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = fileName
	}
	return d.AddAttachment(AttachmentSpec{
		Name:     trimmedName,
		FileName: fileName,
		MIMEType: mime.TypeByExtension(filepath.Ext(fileName)),
		Data:     data,
	})
}

// AttachFile is a Java-parity alias of AddAttachmentFromFile.
func (d *Document) AttachFile(name, path string) error {
	return d.AddAttachmentFromFile(name, path)
}

// Attachments returns embedded files from the PDF Names/EmbeddedFiles tree.
func (d *Document) Attachments() ([]*Attachment, error) {
	base, err := d.baseAttachments()
	if err != nil {
		return nil, err
	}
	added, deleted := d.snapshotAttachmentOverlays()
	if len(base) == 0 && len(added) == 0 {
		return nil, nil
	}

	out := mergeAttachmentView(base, added, deleted)
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func mergeAttachmentView(base []*Attachment, added map[string]sessionAttachmentState, deleted map[string]bool) []*Attachment {
	out := make([]*Attachment, 0, len(base)+len(added))
	seen := make(map[string]struct{}, len(base)+len(added))

	for _, att := range base {
		key := attachmentOverlayKey(att.Name, att.FileName)
		if isAttachmentDeletedForKey(deleted, key, att) {
			continue
		}
		if key != "" {
			if _, ok := added[key]; ok {
				continue
			}
		}
		appendUniqueAttachment(&out, seen, att)
	}

	addedKeys := make([]string, 0, len(added))
	for key := range added {
		addedKeys = append(addedKeys, key)
	}
	sort.Strings(addedKeys)
	for _, key := range addedKeys {
		state := added[key]
		att := state.toAttachment()
		if isAttachmentDeletedForKey(deleted, key, att) {
			continue
		}
		appendUniqueAttachment(&out, seen, att)
	}

	return out
}

func (d *Document) baseAttachments() ([]*Attachment, error) {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil, nil
	}

	namesObj := catalog.Get(entity.Name("Names"))
	if namesObj == nil {
		return nil, nil
	}
	namesDict, err := d.asDict(namesObj)
	if err != nil {
		return nil, err
	}

	embeddedObj := namesDict.Get(entity.Name("EmbeddedFiles"))
	if embeddedObj == nil {
		return nil, nil
	}
	embeddedTree, err := d.asDict(embeddedObj)
	if err != nil {
		return nil, err
	}

	entries := make([]nameTreeEntry, 0)
	if err := d.collectNameTreeEntries(embeddedTree, 0, map[*entity.Dict]struct{}{}, &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	out := make([]*Attachment, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		att, parseErr := d.parseAttachment(entry.name, entry.value)
		if parseErr != nil || att == nil {
			continue
		}
		appendUniqueAttachment(&out, seen, att)
	}

	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Attachment returns one attachment matched by name or file name.
func (d *Document) Attachment(name string) (*Attachment, error) {
	key := normalizeAttachmentKey(name)
	if key == "" {
		return nil, fmt.Errorf("attachment name is empty")
	}

	items, err := d.Attachments()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if normalizeAttachmentKey(item.Name) == key || normalizeAttachmentKey(item.FileName) == key {
			return item, nil
		}
	}
	return nil, fmt.Errorf("attachment not found: %s", name)
}

// ExportAttachmentData returns attachment bytes matched by name or file name.
func (d *Document) ExportAttachmentData(name string) ([]byte, error) {
	att, err := d.Attachment(name)
	if err != nil {
		return nil, err
	}
	return att.Data(), nil
}

// ExportAttachedFileData is a Java-parity alias of ExportAttachmentData.
func (d *Document) ExportAttachedFileData(name string) ([]byte, error) {
	return d.ExportAttachmentData(name)
}

// ExportAttachmentToFile writes attachment bytes to file.
func (d *Document) ExportAttachmentToFile(name, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is empty")
	}

	data, err := d.ExportAttachmentData(name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write attachment file: %w", err)
	}
	return nil
}

// DeleteAttachment hides an attachment in the current session overlay.
func (d *Document) DeleteAttachment(name string) error {
	key := normalizeAttachmentKey(name)
	if key == "" {
		return fmt.Errorf("attachment name is empty")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletedAttachments[key] = true
	for addedKey, state := range d.addedAttachments {
		if normalizeAttachmentKey(state.Name) == key || normalizeAttachmentKey(state.FileName) == key {
			d.deletedAttachments[addedKey] = true
		}
	}
	return nil
}

// DeleteAttachedFile is a Java-parity alias of DeleteAttachment.
func (d *Document) DeleteAttachedFile(name string) error {
	return d.DeleteAttachment(name)
}

// RestoreAttachment removes attachment hide override in the current session overlay.
func (d *Document) RestoreAttachment(name string) {
	key := normalizeAttachmentKey(name)
	if key == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.deletedAttachments, key)
	for addedKey, state := range d.addedAttachments {
		if normalizeAttachmentKey(state.Name) == key || normalizeAttachmentKey(state.FileName) == key {
			delete(d.deletedAttachments, addedKey)
			delete(d.deletedAttachments, normalizeAttachmentKey(state.Name))
			delete(d.deletedAttachments, normalizeAttachmentKey(state.FileName))
		}
	}
}

// IsAttachmentDeleted returns whether attachment hide override is set.
func (d *Document) IsAttachmentDeleted(name string) bool {
	key := normalizeAttachmentKey(name)
	if key == "" {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.deletedAttachments[key]
}

type nameTreeEntry struct {
	value entity.Object
	name  string
}

func normalizeAttachmentKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func attachmentOverlayKey(name, fileName string) string {
	key := normalizeAttachmentKey(name)
	if key != "" {
		return key
	}
	return normalizeAttachmentKey(fileName)
}

func (d *Document) deletedAttachmentSnapshot() map[string]bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make(map[string]bool, len(d.deletedAttachments))
	for key, deleted := range d.deletedAttachments {
		out[key] = deleted
	}
	return out
}

func (d *Document) addedAttachmentSnapshot() map[string]sessionAttachmentState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make(map[string]sessionAttachmentState, len(d.addedAttachments))
	for key, state := range d.addedAttachments {
		out[key] = copySessionAttachmentState(state)
	}
	return out
}

func (d *Document) snapshotAttachmentOverlays() (map[string]sessionAttachmentState, map[string]bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	added := make(map[string]sessionAttachmentState, len(d.addedAttachments))
	for key, state := range d.addedAttachments {
		added[key] = copySessionAttachmentState(state)
	}
	deleted := make(map[string]bool, len(d.deletedAttachments))
	for key, hidden := range d.deletedAttachments {
		deleted[key] = hidden
	}
	return added, deleted
}

func isAttachmentDeletedForKey(deleted map[string]bool, key string, att *Attachment) bool {
	if len(deleted) == 0 {
		return false
	}
	if key != "" && deleted[key] {
		return true
	}
	if att == nil {
		return false
	}
	if deleted[normalizeAttachmentKey(att.Name)] {
		return true
	}
	if deleted[normalizeAttachmentKey(att.FileName)] {
		return true
	}
	return false
}

func appendUniqueAttachment(out *[]*Attachment, seen map[string]struct{}, att *Attachment) {
	if out == nil || att == nil {
		return
	}
	identity := attachmentIdentity(att)
	if identity != "" {
		if _, ok := seen[identity]; ok {
			return
		}
		seen[identity] = struct{}{}
	}
	*out = append(*out, copyAttachment(att))
}

func attachmentIdentity(att *Attachment) string {
	if att == nil {
		return ""
	}
	return normalizeAttachmentKey(att.Name) + "\x00" + normalizeAttachmentKey(att.FileName)
}

func copyAttachment(att *Attachment) *Attachment {
	if att == nil {
		return nil
	}
	out := *att
	out.data = append([]byte(nil), att.data...)
	return &out
}

func copySessionAttachmentState(state sessionAttachmentState) sessionAttachmentState {
	out := state
	out.Data = append([]byte(nil), state.Data...)
	return out
}

func (s sessionAttachmentState) toAttachment() *Attachment {
	att := &Attachment{
		Description: s.Description,
		FileName:    s.FileName,
		MIMEType:    s.MIMEType,
		Name:        s.Name,
		Size:        s.Size,
		data:        append([]byte(nil), s.Data...),
	}
	if att.Size == 0 {
		att.Size = len(att.data)
	}
	return att
}

func (d *Document) collectNameTreeEntries(node *entity.Dict, depth int, visited map[*entity.Dict]struct{}, out *[]nameTreeEntry) error {
	if node == nil || depth > 32 {
		return nil
	}
	if _, ok := visited[node]; ok {
		return nil
	}
	visited[node] = struct{}{}

	if namesObj := node.Get(entity.Name("Names")); namesObj != nil {
		namesArr, ok := namesObj.(*entity.Array)
		if !ok {
			return fmt.Errorf("name tree Names is not array")
		}
		for i := 0; i+1 < namesArr.Len(); i += 2 {
			name := extractEntityString(namesArr.Get(i))
			if name == "" {
				name = extractEntityNameOrString(namesArr.Get(i))
			}
			if name == "" {
				continue
			}
			*out = append(*out, nameTreeEntry{
				name:  name,
				value: namesArr.Get(i + 1),
			})
		}
	}

	if kidsObj := node.Get(entity.Name("Kids")); kidsObj != nil {
		kids, ok := kidsObj.(*entity.Array)
		if !ok {
			return fmt.Errorf("name tree Kids is not array")
		}
		for i := 0; i < kids.Len(); i++ {
			kidDict, err := d.asDict(kids.Get(i))
			if err != nil {
				continue
			}
			if err := d.collectNameTreeEntries(kidDict, depth+1, visited, out); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Document) parseAttachment(name string, obj entity.Object) (*Attachment, error) {
	fileSpec, err := d.asDict(obj)
	if err != nil {
		return nil, err
	}

	fileName := extractEntityString(fileSpec.Get(entity.Name("UF")))
	if fileName == "" {
		fileName = extractEntityString(fileSpec.Get(entity.Name("F")))
	}
	if fileName == "" {
		fileName = name
	}

	att := &Attachment{
		Name:        name,
		FileName:    fileName,
		Description: extractEntityString(fileSpec.Get(entity.Name("Desc"))),
	}

	efObj := fileSpec.Get(entity.Name("EF"))
	if efObj == nil {
		return att, nil
	}
	efDict, err := d.asDict(efObj)
	if err != nil {
		return att, nil
	}

	streamObj := efDict.Get(entity.Name("UF"))
	if streamObj == nil {
		streamObj = efDict.Get(entity.Name("F"))
	}
	if streamObj == nil {
		return att, nil
	}

	stream, err := d.asStream(streamObj)
	if err != nil {
		return att, nil
	}

	data, decodeErr := stream.Decode()
	if decodeErr != nil {
		data = stream.RawBytes()
	}
	att.data = append([]byte(nil), data...)
	att.Size = len(att.data)

	streamDict := stream.Dict()
	if streamDict != nil {
		mimeType := extractEntityNameOrString(streamDict.Get(entity.Name("Subtype")))
		mimeType = strings.TrimPrefix(mimeType, "/")
		att.MIMEType = mimeType

		if paramsObj := streamDict.Get(entity.Name("Params")); paramsObj != nil {
			if paramsDict, paramsErr := d.asDict(paramsObj); paramsErr == nil {
				if sizeObj, ok := paramsDict.Get(entity.Name("Size")).(*entity.Integer); ok {
					att.Size = int(sizeObj.Value())
				}
			}
		}
	}

	return att, nil
}

func (d *Document) asStream(obj entity.Object) (*entity.Stream, error) {
	switch v := obj.(type) {
	case *entity.Stream:
		return v, nil
	case entity.Ref:
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return nil, fmt.Errorf("fetch stream ref: %w", err)
		}
		stream, ok := fetched.(*entity.Stream)
		if !ok {
			return nil, fmt.Errorf("object is not stream: %T", fetched)
		}
		return stream, nil
	default:
		return nil, fmt.Errorf("object is not stream: %T", obj)
	}
}
