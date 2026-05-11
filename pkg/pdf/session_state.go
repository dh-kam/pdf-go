package pdf

import (
	"encoding/json"
	"fmt"
)

// sessionState contains session-level mutable document state.
type sessionState struct {
	FormValues          map[string][]string               `json:"form_values,omitempty"`
	FormOptions         map[string][]string               `json:"form_options,omitempty"`
	AddedAttachments    map[string]sessionAttachmentState `json:"added_attachments,omitempty"`
	DeletedAttachments  map[string]bool                   `json:"deleted_attachments,omitempty"`
	HiddenTargets       map[string]bool                   `json:"hidden_targets,omitempty"`
	AnnotationOverrides map[int][]annotationSnapshot      `json:"annotation_overrides,omitempty"`
	SignatureFields     map[string]signatureFieldSnapshot `json:"signature_fields,omitempty"`
	PageOrder           []int                             `json:"page_order,omitempty"`
	Outlines            []*Outline                        `json:"outlines,omitempty"`
}

// ExportSessionState exports session-level state as JSON bytes.
func (d *Document) ExportSessionState() ([]byte, error) {
	state := d.snapshotSessionState()
	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal session state: %w", err)
	}
	return data, nil
}

// ImportSessionState imports session-level state from JSON bytes.
func (d *Document) ImportSessionState(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("session state is empty")
	}

	var state sessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshal session state: %w", err)
	}

	if err := d.validateSessionState(&state); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.pageOrder = append([]int(nil), state.PageOrder...)
	if len(state.Outlines) > 0 {
		d.outlines = cloneOutlines(state.Outlines)
		d.outlinesSet = true
	} else {
		d.outlines = nil
		d.outlinesSet = false
	}

	d.formValues = make(map[string][]string, len(state.FormValues))
	for name, values := range state.FormValues {
		d.formValues[name] = normalizeFormFieldValues(values)
	}
	d.formOptions = make(map[string][]string, len(state.FormOptions))
	for name, options := range state.FormOptions {
		d.formOptions[name] = normalizeChoiceFieldOptions(options)
	}
	d.addedAttachments = make(map[string]sessionAttachmentState, len(state.AddedAttachments))
	for key, item := range state.AddedAttachments {
		d.addedAttachments[key] = copySessionAttachmentState(item)
	}
	d.deletedAttachments = make(map[string]bool, len(state.DeletedAttachments))
	for key, deleted := range state.DeletedAttachments {
		d.deletedAttachments[key] = deleted
	}

	d.hiddenTargets = make(map[string]bool, len(state.HiddenTargets))
	for target, hide := range state.HiddenTargets {
		d.hiddenTargets[target] = hide
	}

	d.annotationOverrides = make(map[int][]annotationSnapshot, len(state.AnnotationOverrides))
	for pageIndex, items := range state.AnnotationOverrides {
		d.annotationOverrides[pageIndex] = cloneAnnotationSnapshots(items)
	}
	d.signatureFields = cloneSignatureFieldSnapshots(state.SignatureFields)

	return nil
}

func (d *Document) snapshotSessionState() sessionState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state := sessionState{
		PageOrder:           append([]int(nil), d.pageOrder...),
		FormValues:          make(map[string][]string, len(d.formValues)),
		FormOptions:         make(map[string][]string, len(d.formOptions)),
		AddedAttachments:    make(map[string]sessionAttachmentState, len(d.addedAttachments)),
		DeletedAttachments:  make(map[string]bool, len(d.deletedAttachments)),
		HiddenTargets:       make(map[string]bool, len(d.hiddenTargets)),
		AnnotationOverrides: make(map[int][]annotationSnapshot, len(d.annotationOverrides)),
		SignatureFields:     make(map[string]signatureFieldSnapshot, len(d.signatureFields)),
	}
	if d.outlinesSet {
		state.Outlines = cloneOutlines(d.outlines)
	}

	for name, values := range d.formValues {
		state.FormValues[name] = normalizeFormFieldValues(values)
	}
	for name, options := range d.formOptions {
		state.FormOptions[name] = normalizeChoiceFieldOptions(options)
	}
	for key, item := range d.addedAttachments {
		state.AddedAttachments[key] = copySessionAttachmentState(item)
	}
	for key, deleted := range d.deletedAttachments {
		state.DeletedAttachments[key] = deleted
	}
	for target, hide := range d.hiddenTargets {
		state.HiddenTargets[target] = hide
	}
	for pageIndex, items := range d.annotationOverrides {
		state.AnnotationOverrides[pageIndex] = cloneAnnotationSnapshots(items)
	}
	state.SignatureFields = cloneSignatureFieldSnapshots(d.signatureFields)

	return state
}

func (d *Document) validateSessionState(state *sessionState) error {
	if state == nil {
		return fmt.Errorf("session state is nil")
	}

	pageCount, err := d.doc.PageCount()
	if err != nil {
		return err
	}

	for _, source := range state.PageOrder {
		if source < 0 || source >= pageCount {
			return fmt.Errorf("invalid page order index: %d", source)
		}
	}

	for pageIndex := range state.AnnotationOverrides {
		if pageIndex < 0 || pageIndex >= pageCount {
			return fmt.Errorf("invalid annotation override page index: %d", pageIndex)
		}
	}
	for name, sig := range state.SignatureFields {
		if name == "" {
			return fmt.Errorf("invalid signature field name: empty")
		}
		if sig.PageIndex < 0 || sig.PageIndex >= pageCount {
			return fmt.Errorf("invalid signature field page index: %d", sig.PageIndex)
		}
	}

	return nil
}
