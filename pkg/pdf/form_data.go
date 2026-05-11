package pdf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

const xfdfNamespace = "http://ns.adobe.com/xfdf/"

// FormData represents imported/exported form field values.
type FormData struct {
	Fields map[string][]string
}

// ExportFormDataXFDF exports current AcroForm values as XFDF bytes.
func (d *Document) ExportFormDataXFDF() ([]byte, error) {
	fields, err := d.FormFields()
	if err != nil {
		return nil, fmt.Errorf("load form fields: %w", err)
	}

	data := buildFormDataFromFields(fields)
	return buildXFDFFromFormData(data)
}

// ExportFormDataFDF exports current AcroForm values as FDF bytes.
func (d *Document) ExportFormDataFDF() ([]byte, error) {
	fields, err := d.FormFields()
	if err != nil {
		return nil, fmt.Errorf("load form fields: %w", err)
	}

	data := buildFormDataFromFields(fields)
	return buildFDFFromFormData(data), nil
}

// ParseFormDataXFDF parses XFDF data into FormData.
func ParseFormDataXFDF(data []byte) (*FormData, error) {
	if len(data) == 0 {
		return &FormData{Fields: map[string][]string{}}, nil
	}

	var doc xfdfDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse xfdf: %w", err)
	}

	out := &FormData{Fields: map[string][]string{}}
	for _, field := range doc.Fields.Fields {
		collectXFDFFieldValues(field, "", out.Fields)
	}

	return out, nil
}

// ParseFormDataFDF parses FDF data into FormData.
func ParseFormDataFDF(data []byte) (*FormData, error) {
	out := &FormData{Fields: map[string][]string{}}
	if len(data) == 0 {
		return out, nil
	}

	xrefTable := xref.NewTable(data)
	if err := xrefTable.Parse(); err != nil {
		return nil, fmt.Errorf("parse fdf xref: %w", err)
	}

	catalog, err := xrefTable.GetCatalog()
	if err != nil || catalog == nil {
		// FDF can be minimal and may omit /Type /Catalog. Fall back to trailer /Root.
		trailer, trailerErr := xrefTable.GetTrailer()
		if trailerErr != nil || trailer == nil {
			return nil, fmt.Errorf("resolve fdf catalog: %w", err)
		}
		catalog, err = resolveDictObject(xrefTable, trailer.Get(entity.Name("/Root")))
		if err != nil || catalog == nil {
			return nil, fmt.Errorf("resolve fdf root: %w", err)
		}
	}

	fdfDict := catalog
	if fdfObj := catalog.Get(entity.Name("/FDF")); fdfObj != nil {
		parsed, parseErr := resolveDictObject(xrefTable, fdfObj)
		if parseErr != nil {
			return nil, parseErr
		}
		fdfDict = parsed
	}

	fieldsObj := fdfDict.Get(entity.Name("/Fields"))
	if fieldsObj == nil {
		return out, nil
	}

	fieldsArr, err := resolveArrayObject(xrefTable, fieldsObj)
	if err != nil {
		return nil, err
	}

	visited := make(map[*entity.Dict]struct{})
	for i := 0; i < fieldsArr.Len(); i++ {
		parseFDFFieldNode(xrefTable, fieldsArr.Get(i), "", out.Fields, visited)
	}

	return out, nil
}

func buildFormDataFromFields(fields []*FormField) *FormData {
	data := &FormData{Fields: map[string][]string{}}
	for _, field := range fields {
		if field == nil || field.Name == "" {
			continue
		}

		values := formValueToStrings(field.Value)
		if len(values) == 0 {
			values = []string{""}
		}
		data.Fields[field.Name] = mergeUniqueStrings(data.Fields[field.Name], values)
	}
	return data
}

type xfdfDocument struct {
	XMLName xml.Name   `xml:"xfdf"`
	XMLNS   string     `xml:"xmlns,attr,omitempty"`
	Fields  xfdfFields `xml:"fields"`
}

type xfdfFields struct {
	Fields []xfdfField `xml:"field"`
}

type xfdfField struct {
	Name   string      `xml:"name,attr"`
	Values []string    `xml:"value"`
	Fields []xfdfField `xml:"field"`
}

func buildXFDFFromFormData(data *FormData) ([]byte, error) {
	doc := xfdfDocument{
		XMLNS:  xfdfNamespace,
		Fields: xfdfFields{Fields: []xfdfField{}},
	}

	names := make([]string, 0, len(data.Fields))
	for name := range data.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		values := append([]string(nil), data.Fields[name]...)
		if len(values) == 0 {
			values = []string{""}
		}
		doc.Fields.Fields = append(doc.Fields.Fields, xfdfField{
			Name:   name,
			Values: values,
		})
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal xfdf: %w", err)
	}

	result := append([]byte(xml.Header), body...)
	result = append(result, '\n')
	return result, nil
}

func collectXFDFFieldValues(field xfdfField, parent string, out map[string][]string) {
	fullName := parent
	if field.Name != "" {
		if parent == "" {
			fullName = field.Name
		} else {
			fullName = parent + "." + field.Name
		}
	}

	if fullName != "" {
		values := append([]string(nil), field.Values...)
		if len(values) == 0 && len(field.Fields) == 0 {
			values = []string{""}
		}
		if len(values) > 0 {
			out[fullName] = mergeUniqueStrings(out[fullName], values)
		}
	}

	for _, child := range field.Fields {
		collectXFDFFieldValues(child, fullName, out)
	}
}

func buildFDFFromFormData(data *FormData) []byte {
	names := make([]string, 0, len(data.Fields))
	for name := range data.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	objectCount := len(names) + 1 // Root + fields
	objects := make([]string, objectCount+1)

	fieldRefs := make([]string, 0, len(names))
	for i, name := range names {
		objNum := i + 2
		fieldRefs = append(fieldRefs, fmt.Sprintf("%d 0 R", objNum))
		objects[objNum] = buildFDFFieldObject(name, data.Fields[name])
	}

	objects[1] = fmt.Sprintf("<< /Type /Catalog /FDF << /Fields [%s] >> >>", strings.Join(fieldRefs, " "))

	var buf bytes.Buffer
	buf.WriteString("%FDF-1.2\n")
	buf.WriteString("%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, objectCount+1)
	for i := 1; i <= objectCount; i++ {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i, objects[i])
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", objectCount+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= objectCount; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}

	fmt.Fprintf(&buf, "trailer\n<< /Root 1 0 R /Size %d >>\n", objectCount+1)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return buf.Bytes()
}

func buildFDFFieldObject(name string, values []string) string {
	if len(values) == 0 {
		values = []string{""}
	}

	var sb strings.Builder
	sb.WriteString("<< /T ")
	sb.WriteString(toPDFLiteralString(name))
	sb.WriteString(" /V ")
	if len(values) == 1 {
		sb.WriteString(toPDFLiteralString(values[0]))
	} else {
		sb.WriteString("[")
		for i, v := range values {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(toPDFLiteralString(v))
		}
		sb.WriteString("]")
	}
	sb.WriteString(" >>")
	return sb.String()
}

func toPDFLiteralString(value string) string {
	var sb strings.Builder
	sb.WriteByte('(')
	for _, r := range value {
		switch r {
		case '\\':
			sb.WriteString("\\\\")
		case '(':
			sb.WriteString("\\(")
		case ')':
			sb.WriteString("\\)")
		case '\n':
			sb.WriteString("\\n")
		case '\r':
			sb.WriteString("\\r")
		case '\t':
			sb.WriteString("\\t")
		case '\b':
			sb.WriteString("\\b")
		case '\f':
			sb.WriteString("\\f")
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte(')')
	return sb.String()
}

func parseFDFFieldNode(
	x entity.XRef,
	obj entity.Object,
	parentName string,
	out map[string][]string,
	visited map[*entity.Dict]struct{},
) {
	dict, err := resolveDictObject(x, obj)
	if err != nil || dict == nil {
		return
	}
	if _, seen := visited[dict]; seen {
		return
	}
	visited[dict] = struct{}{}

	partial := extractEntityString(dict.Get(entity.Name("/T")))
	fullName := parentName
	if partial != "" {
		if fullName == "" {
			fullName = partial
		} else {
			fullName = fullName + "." + partial
		}
	}

	if fullName != "" {
		if vObj := dict.Get(entity.Name("/V")); vObj != nil {
			values := entityValueToStrings(vObj)
			if len(values) == 0 {
				values = []string{""}
			}
			out[fullName] = mergeUniqueStrings(out[fullName], values)
		}
	}

	kidsObj := dict.Get(entity.Name("/Kids"))
	if kidsObj == nil {
		return
	}

	kidsArr, err := resolveArrayObject(x, kidsObj)
	if err != nil {
		return
	}

	for i := 0; i < kidsArr.Len(); i++ {
		parseFDFFieldNode(x, kidsArr.Get(i), fullName, out, visited)
	}
}

func resolveDictObject(x entity.XRef, obj entity.Object) (*entity.Dict, error) {
	switch v := obj.(type) {
	case *entity.Dict:
		return v, nil
	case entity.Ref:
		fetched, err := x.Fetch(v)
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

func resolveArrayObject(x entity.XRef, obj entity.Object) (*entity.Array, error) {
	switch v := obj.(type) {
	case *entity.Array:
		return v, nil
	case entity.Ref:
		fetched, err := x.Fetch(v)
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

func formValueToStrings(value Object) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return []string{v}
	case int:
		return []string{fmt.Sprintf("%d", v)}
	case float64:
		return []string{fmt.Sprintf("%g", v)}
	case bool:
		if v {
			return []string{"true"}
		}
		return []string{"false"}
	case []string:
		return normalizeFormFieldValues(v)
	case *Array:
		values := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			values = append(values, formValueToStrings(v.Get(i))...)
		}
		return values
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}

func entityValueToStrings(obj entity.Object) []string {
	switch v := obj.(type) {
	case nil:
		return nil
	case *entity.String:
		return []string{decodePDFTextString(v.Value())}
	case entity.Name:
		return []string{v.Value()}
	case *entity.Integer:
		return []string{fmt.Sprintf("%d", v.Value())}
	case *entity.Real:
		return []string{fmt.Sprintf("%g", v.Value())}
	case *entity.Boolean:
		if v.Value() {
			return []string{"true"}
		}
		return []string{"false"}
	case *entity.Array:
		values := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			values = append(values, entityValueToStrings(v.Get(i))...)
		}
		return values
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}

func mergeUniqueStrings(base, next []string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(out))
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range next {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
