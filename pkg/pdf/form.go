package pdf

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// FormField represents one AcroForm field node.
type FormField struct {
	Value        Object
	DefaultValue Object
	Name         string
	PartialName  string
	Type         string
	Options      []string
	Kids         []*FormField
	Flags        int
	PageIndex    int
}

type inheritedFieldAttrs struct {
	value           entity.Object
	defaultValue    entity.Object
	fieldType       string
	options         []string
	flags           int
	hasFlags        bool
	hasValue        bool
	hasDefaultValue bool
	hasOptions      bool
}

// FormFieldTree returns the AcroForm field tree.
// It returns nil, nil when the document has no AcroForm fields.
func (d *Document) FormFieldTree() ([]*FormField, error) {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil, nil
	}

	acroObj := catalog.Get(entity.Name("AcroForm"))
	if acroObj == nil {
		return nil, nil
	}

	acroDict, err := d.asDict(acroObj)
	if err != nil {
		return nil, err
	}

	fieldsObj := acroDict.Get(entity.Name("Fields"))
	if fieldsObj == nil {
		return nil, nil
	}

	fieldsArr, err := d.asArray(fieldsObj)
	if err != nil {
		return nil, err
	}

	visited := make(map[*entity.Dict]struct{})
	pageRefToIndex := d.buildPageRefToIndexMap()
	formValueOverrides := d.snapshotFormValueOverrides()
	formOptionOverrides := d.snapshotFormOptionOverrides()
	roots := make([]*FormField, 0, fieldsArr.Len())
	for i := 0; i < fieldsArr.Len(); i++ {
		field, parseErr := d.parseFormFieldNode(
			fieldsArr.Get(i),
			"",
			inheritedFieldAttrs{},
			visited,
			pageRefToIndex,
			formValueOverrides,
			formOptionOverrides,
		)
		if parseErr != nil {
			return nil, parseErr
		}
		if field != nil {
			roots = append(roots, field)
		}
	}

	if len(roots) == 0 {
		return nil, nil
	}
	return roots, nil
}

// FormFields returns flattened terminal form fields.
func (d *Document) FormFields() ([]*FormField, error) {
	roots, err := d.FormFieldTree()
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, nil
	}

	flat := make([]*FormField, 0)
	var walk func(node *FormField)
	walk = func(node *FormField) {
		if node == nil {
			return
		}
		if len(node.Kids) == 0 {
			flat = append(flat, node)
			return
		}

		// Keep non-terminal fields that carry their own values/properties.
		if node.Type != "" || node.Value != nil || node.DefaultValue != nil || len(node.Options) > 0 {
			flat = append(flat, node)
		}
		for _, child := range node.Kids {
			walk(child)
		}
	}

	for _, root := range roots {
		walk(root)
	}

	if len(flat) == 0 {
		return nil, nil
	}
	return flat, nil
}

func (d *Document) parseFormFieldNode(
	obj entity.Object,
	parentName string,
	parentAttrs inheritedFieldAttrs,
	visited map[*entity.Dict]struct{},
	pageRefToIndex map[entity.Ref]int,
	formValueOverrides map[string][]string,
	formOptionOverrides map[string][]string,
) (*FormField, error) {
	dict, err := d.asDict(obj)
	if err != nil {
		return nil, err
	}
	if _, seen := visited[dict]; seen {
		return nil, nil
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

	field := &FormField{
		Name:        fullName,
		PartialName: partial,
		Type:        attrs.fieldType,
		PageIndex:   -1,
	}
	if attrs.hasFlags {
		field.Flags = attrs.flags
	}
	if attrs.hasValue {
		field.Value = wrapObject(attrs.value)
	}
	if attrs.hasDefaultValue {
		field.DefaultValue = wrapObject(attrs.defaultValue)
	}
	if attrs.hasOptions {
		field.Options = append([]string(nil), attrs.options...)
	}
	if values, ok := formValueOverrides[field.Name]; ok {
		field.Value = overrideValuesToObject(values)
	}
	if options, ok := formOptionOverrides[field.Name]; ok {
		field.Options = normalizeChoiceFieldOptions(options)
	}

	if extractEntityNameOrString(dict.Get(entity.Name("Subtype"))) == "Widget" {
		if pageObj := dict.Get(entity.Name("P")); pageObj != nil {
			field.PageIndex = d.resolveDestinationPageIndex(pageObj, pageRefToIndex, 0)
		}
	}

	kidsObj := dict.Get(entity.Name("Kids"))
	if kidsObj != nil {
		kidsArr, arrErr := d.asArray(kidsObj)
		if arrErr != nil {
			return nil, arrErr
		}
		field.Kids = make([]*FormField, 0, kidsArr.Len())
		for i := 0; i < kidsArr.Len(); i++ {
			child, childErr := d.parseFormFieldNode(
				kidsArr.Get(i),
				fullName,
				attrs,
				visited,
				pageRefToIndex,
				formValueOverrides,
				formOptionOverrides,
			)
			if childErr != nil {
				return nil, childErr
			}
			if child != nil {
				field.Kids = append(field.Kids, child)
			}
		}
	}

	if field.Name == "" && field.Type == "" && field.Value == nil && field.DefaultValue == nil &&
		len(field.Options) == 0 && len(field.Kids) == 0 {
		return nil, nil
	}

	return field, nil
}

func mergeFieldAttrs(dict *entity.Dict, parent inheritedFieldAttrs) inheritedFieldAttrs {
	attrs := parent

	if ftObj := dict.Get(entity.Name("FT")); ftObj != nil {
		attrs.fieldType = extractEntityNameOrString(ftObj)
	}
	if ffObj := dict.Get(entity.Name("Ff")); ffObj != nil {
		if ff, ok := ffObj.(*entity.Integer); ok {
			attrs.flags = int(ff.Value())
			attrs.hasFlags = true
		}
	}
	if vObj := dict.Get(entity.Name("V")); vObj != nil {
		attrs.value = vObj
		attrs.hasValue = true
	}
	if dvObj := dict.Get(entity.Name("DV")); dvObj != nil {
		attrs.defaultValue = dvObj
		attrs.hasDefaultValue = true
	}
	if optObj := dict.Get(entity.Name("Opt")); optObj != nil {
		opts := parseFormFieldOptions(optObj)
		attrs.options = opts
		attrs.hasOptions = true
	}

	return attrs
}

func parseFormFieldOptions(obj entity.Object) []string {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil
	}

	out := make([]string, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		item := arr.Get(i)
		switch v := item.(type) {
		case *entity.String:
			out = append(out, extractEntityString(v))
		case entity.Name:
			out = append(out, v.Value())
		case *entity.Array:
			// Choice option can be [export, display]
			if v.Len() > 1 {
				out = append(out, extractEntityNameOrString(v.Get(1)))
			} else if v.Len() == 1 {
				out = append(out, extractEntityNameOrString(v.Get(0)))
			}
		}
	}

	return out
}

func (d *Document) snapshotFormValueOverrides() map[string][]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.formValues) == 0 {
		return map[string][]string{}
	}

	out := make(map[string][]string, len(d.formValues))
	for name, values := range d.formValues {
		out[name] = normalizeFormFieldValues(values)
	}
	return out
}

func (d *Document) snapshotFormOptionOverrides() map[string][]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.formOptions) == 0 {
		return map[string][]string{}
	}

	out := make(map[string][]string, len(d.formOptions))
	for name, options := range d.formOptions {
		out[name] = normalizeChoiceFieldOptions(options)
	}
	return out
}

func overrideValuesToObject(values []string) Object {
	values = normalizeFormFieldValues(values)
	if len(values) == 1 {
		return values[0]
	}
	return values
}

func (d *Document) asArray(obj entity.Object) (*entity.Array, error) {
	switch v := obj.(type) {
	case *entity.Array:
		return v, nil
	case entity.Ref:
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return nil, fmt.Errorf("fetch array ref: %w", err)
		}
		arr, ok := fetched.(*entity.Array)
		if !ok {
			return nil, fmt.Errorf("object is not array: %T", fetched)
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("object is not array: %T", obj)
	}
}
