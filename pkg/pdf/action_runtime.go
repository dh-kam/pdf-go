package pdf

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ActionExecutionOptions configures outline action execution.
type ActionExecutionOptions struct {
	OnLaunch                      func(action *OutlineAction) error
	OnFileAttachment              func(action *OutlineAction) error
	OnGoTo                        func(action *OutlineAction) error
	OnURI                         func(action *OutlineAction) error
	OnNamed                       func(action *OutlineAction) error
	OnSubmitForm                  func(action *OutlineAction, data *FormData) error
	OnImportData                  func(action *OutlineAction, data *FormData) error
	OnHide                        func(action *OutlineAction, hiddenTargets []string) error
	ImportDataByFile              map[string][]byte
	NativeRenditionCommandRunner  func(command string, args ...string) error
	OnGoToR                       func(action *OutlineAction) error
	OnScrollLock                  func(action *OutlineAction) error
	OnJavaScript                  func(action *OutlineAction) error
	OnMultimedia                  func(action *OutlineAction) error
	OnRendition                   func(action *OutlineAction) error
	OnRichMedia                   func(action *OutlineAction) error
	OnRenditionPlayback           func(action *OutlineAction, mediaFile string) error
	ImportData                    []byte
	EnableNativeRenditionPlayback bool
}

// ActionExecutionResult describes the outcome of action execution.
type ActionExecutionResult struct {
	SubmittedFormData  *FormData
	ImportedFormData   *FormData
	JavaScript         string
	Type               string
	RenditionName      string
	RenditionMIMEType  string
	RenditionFile      string
	File               string
	URI                string
	HiddenTargets      []string
	NextResults        []*ActionExecutionResult
	ResetFieldCount    int
	PageIndex          int
	AppliedFieldCount  int
	RenditionOperation int
	Hide               bool
	HandlerInvoked     bool
}

// ExecuteOutlineAction executes a parsed outline action in session scope.
// It does not perform external process execution or network requests.
func (d *Document) ExecuteOutlineAction(action *OutlineAction, options ActionExecutionOptions) (*ActionExecutionResult, error) {
	if action == nil {
		return nil, fmt.Errorf("outline action is nil")
	}
	return d.executeOutlineAction(action, options, 0)
}

func (d *Document) executeOutlineAction(action *OutlineAction, options ActionExecutionOptions, depth int) (*ActionExecutionResult, error) {
	if depth > 16 {
		return nil, fmt.Errorf("outline action chain depth exceeded")
	}

	result := &ActionExecutionResult{
		Type:               action.Type,
		PageIndex:          action.PageIndex,
		Hide:               action.Hide,
		URI:                action.URI,
		File:               action.File,
		RenditionFile:      action.RenditionFile,
		RenditionMIMEType:  action.RenditionMIMEType,
		JavaScript:         action.JavaScript,
		RenditionName:      action.RenditionName,
		RenditionOperation: action.RenditionOperation,
	}

	switch action.Type {
	case "GoTo":
		if options.OnGoTo != nil {
			if err := options.OnGoTo(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "GoToR":
		if options.OnGoToR != nil {
			if err := options.OnGoToR(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "Named":
		if options.OnNamed != nil {
			if err := options.OnNamed(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "SubmitForm":
		formData, err := d.buildSubmitFormData(action)
		if err != nil {
			return nil, err
		}
		result.SubmittedFormData = formData
		if options.OnSubmitForm != nil {
			if err := options.OnSubmitForm(action, formData); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "ResetForm":
		cleared, err := d.resetFormByAction(action)
		if err != nil {
			return nil, err
		}
		result.ResetFieldCount = cleared
	case "ImportData":
		imported, applied, err := d.importDataByAction(action, options)
		if err != nil {
			return nil, err
		}
		result.ImportedFormData = imported
		result.AppliedFieldCount = applied
		if options.OnImportData != nil {
			if err := options.OnImportData(action, imported); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "Hide":
		targets := dedupeStrings(action.HideTargets)
		updated := d.applyHideTargets(targets, action.Hide)
		result.HiddenTargets = updated
		if options.OnHide != nil {
			if err := options.OnHide(action, updated); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "FileAttachment":
		if options.OnFileAttachment != nil {
			if err := options.OnFileAttachment(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "ScrollLock":
		if options.OnScrollLock != nil {
			if err := options.OnScrollLock(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "Rendition":
		if options.OnMultimedia != nil {
			if err := options.OnMultimedia(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
		if options.OnRendition != nil {
			if err := options.OnRendition(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
		if options.OnRenditionPlayback != nil && strings.TrimSpace(action.RenditionFile) != "" {
			if err := options.OnRenditionPlayback(action, action.RenditionFile); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
		if options.EnableNativeRenditionPlayback && strings.TrimSpace(action.RenditionFile) != "" {
			if err := executeNativeRenditionPlayback(action.RenditionFile, options.NativeRenditionCommandRunner); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "Multimedia", "Movie", "Sound":
		if options.OnMultimedia != nil {
			if err := options.OnMultimedia(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		} else if options.OnRendition != nil {
			if err := options.OnRendition(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
		if options.OnRenditionPlayback != nil && strings.TrimSpace(action.RenditionFile) != "" {
			if err := options.OnRenditionPlayback(action, action.RenditionFile); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
		if options.EnableNativeRenditionPlayback && strings.TrimSpace(action.RenditionFile) != "" {
			if err := executeNativeRenditionPlayback(action.RenditionFile, options.NativeRenditionCommandRunner); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "RichMedia", "RichMediaExecute", "RichMediaPresentation":
		if options.OnRichMedia != nil {
			if err := options.OnRichMedia(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "Launch":
		if options.OnLaunch != nil {
			if err := options.OnLaunch(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "URI":
		if options.OnURI != nil {
			if err := options.OnURI(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	case "JavaScript":
		if options.OnJavaScript != nil {
			if err := options.OnJavaScript(action); err != nil {
				return nil, err
			}
			result.HandlerInvoked = true
		}
	default:
		// Keep result as passive action payload for caller-side handling.
	}

	if len(action.NextActions) > 0 {
		result.NextResults = make([]*ActionExecutionResult, 0, len(action.NextActions))
		for i, next := range action.NextActions {
			nextResult, err := d.executeOutlineAction(next, options, depth+1)
			if err != nil {
				return nil, fmt.Errorf("execute next action %d: %w", i, err)
			}
			result.NextResults = append(result.NextResults, nextResult)
		}
	}

	return result, nil
}

// ExecuteOutlineActionByPath executes an outline action at the given tree path.
func (d *Document) ExecuteOutlineActionByPath(path []int, options ActionExecutionOptions) (*ActionExecutionResult, error) {
	action, err := d.outlineActionByPath(path)
	if err != nil {
		return nil, err
	}

	return d.ExecuteOutlineAction(action, options)
}

// IsActionTargetHidden returns whether a hide action target is currently hidden.
func (d *Document) IsActionTargetHidden(target string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.hiddenTargets[target]
}

func (d *Document) buildSubmitFormData(action *OutlineAction) (*FormData, error) {
	fields, err := d.FormFields()
	if err != nil {
		return nil, fmt.Errorf("load form fields: %w", err)
	}

	data := buildFormDataFromFields(fields)
	if len(action.FieldNames) == 0 {
		return data, nil
	}

	nameSet := make(map[string]struct{}, len(action.FieldNames))
	for _, name := range action.FieldNames {
		nameSet[name] = struct{}{}
	}

	filtered := &FormData{Fields: make(map[string][]string)}
	for name, values := range data.Fields {
		_, listed := nameSet[name]
		include := listed
		if action.ExcludeFields {
			include = !listed
		}
		if !include {
			continue
		}
		filtered.Fields[name] = append([]string(nil), values...)
	}

	return filtered, nil
}

func (d *Document) resetFormByAction(action *OutlineAction) (int, error) {
	fieldNames, err := d.formFieldNameSet()
	if err != nil {
		return 0, err
	}
	if len(fieldNames) == 0 {
		return 0, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	previous := len(d.formValues)
	if len(action.FieldNames) == 0 {
		d.formValues = make(map[string][]string)
		return previous, nil
	}

	targets := make(map[string]struct{}, len(action.FieldNames))
	for _, name := range action.FieldNames {
		if _, ok := fieldNames[name]; ok {
			targets[name] = struct{}{}
		}
	}

	cleared := 0
	if action.ExcludeFields {
		for name := range d.formValues {
			if _, keep := targets[name]; keep {
				continue
			}
			delete(d.formValues, name)
			cleared++
		}
		return cleared, nil
	}

	for name := range targets {
		if _, exists := d.formValues[name]; !exists {
			continue
		}
		delete(d.formValues, name)
		cleared++
	}

	return cleared, nil
}

func (d *Document) importDataByAction(action *OutlineAction, options ActionExecutionOptions) (*FormData, int, error) {
	data := options.ImportData
	if len(data) == 0 && len(options.ImportDataByFile) > 0 && action.File != "" {
		data = options.ImportDataByFile[action.File]
	}
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("import data is required for ImportData action")
	}

	parsed, err := parseActionImportData(data)
	if err != nil {
		return nil, 0, err
	}

	applied, err := d.ApplyFormData(parsed)
	if err != nil {
		return nil, 0, err
	}

	return parsed, applied, nil
}

func parseActionImportData(data []byte) (*FormData, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return &FormData{Fields: map[string][]string{}}, nil
	}

	if bytes.HasPrefix(trimmed, []byte("<")) {
		parsed, err := ParseFormDataXFDF(trimmed)
		if err == nil {
			return parsed, nil
		}
	}
	if bytes.HasPrefix(trimmed, []byte("%FDF")) {
		parsed, err := ParseFormDataFDF(trimmed)
		if err == nil {
			return parsed, nil
		}
	}

	if parsed, err := ParseFormDataXFDF(trimmed); err == nil {
		return parsed, nil
	}
	if parsed, err := ParseFormDataFDF(trimmed); err == nil {
		return parsed, nil
	}

	return nil, fmt.Errorf("unsupported import data format")
}

func (d *Document) applyHideTargets(targets []string, hide bool) []string {
	if len(targets) == 0 {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	updated := make([]string, 0, len(targets))
	for _, target := range targets {
		if strings.TrimSpace(target) == "" {
			continue
		}
		d.hiddenTargets[target] = hide
		updated = append(updated, target)
	}

	return updated
}

func (d *Document) outlineActionByPath(path []int) (*OutlineAction, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("outline path is required")
	}

	outlines, err := d.Outlines()
	if err != nil {
		return nil, err
	}
	if len(outlines) == 0 {
		return nil, fmt.Errorf("outline tree is empty")
	}

	current := outlines
	var node *Outline
	for _, idx := range path {
		if idx < 0 || idx >= len(current) {
			return nil, fmt.Errorf("outline path index out of range: %d", idx)
		}
		node = current[idx]
		current = node.Children
	}

	if node == nil || node.Action == nil {
		return nil, fmt.Errorf("outline action not found at path")
	}
	return node.Action, nil
}

func executeNativeRenditionPlayback(mediaFile string, runner func(command string, args ...string) error) error {
	command, args, err := nativeOpenCommand(mediaFile)
	if err != nil {
		return err
	}

	if runner != nil {
		return runner(command, args...)
	}

	cmd := exec.Command(command, args...)
	return cmd.Start()
}

func nativeOpenCommand(path string) (string, []string, error) {
	target := strings.TrimSpace(path)
	if target == "" {
		return "", nil, fmt.Errorf("rendition media file is empty")
	}

	switch runtime.GOOS {
	case "darwin":
		return "open", []string{target}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}, nil
	default:
		return "xdg-open", []string{target}, nil
	}
}
