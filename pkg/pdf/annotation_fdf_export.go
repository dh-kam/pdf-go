package pdf

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type annotationFDFEntry struct {
	Type     string
	Contents string
	Rect     [4]float64
	Page     int
}

// ExportPageAnnotationsToFDF exports annotations in selected pages to an FDF file.
func (d *Document) ExportPageAnnotationsToFDF(pageIndexes []int, path string) error {
	if len(pageIndexes) == 0 {
		return fmt.Errorf("page indexes are required")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is empty")
	}

	entries, err := d.collectAnnotationFDFEntries(pageIndexes)
	if err != nil {
		return err
	}

	data := buildFDFFromAnnotations(entries)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write annotation fdf: %w", err)
	}
	return nil
}

func (d *Document) collectAnnotationFDFEntries(pageIndexes []int) ([]annotationFDFEntry, error) {
	indexes := dedupeSortedInts(pageIndexes)
	entries := make([]annotationFDFEntry, 0)

	for _, pageIndex := range indexes {
		page, err := d.Page(pageIndex)
		if err != nil {
			return nil, err
		}
		annotations, err := page.Annotations()
		if err != nil {
			return nil, err
		}
		for i := range annotations {
			item := annotations[i]
			entries = append(entries, annotationFDFEntry{
				Page:     pageIndex,
				Type:     normalizeAnnotationSubtype(item.Type()),
				Rect:     item.Rect(),
				Contents: item.Contents(),
			})
		}
	}
	return entries, nil
}

func dedupeSortedInts(input []int) []int {
	if len(input) == 0 {
		return nil
	}
	values := append([]int(nil), input...)
	sort.Ints(values)
	out := make([]int, 0, len(values))
	prevSet := false
	prev := 0
	for _, value := range values {
		if !prevSet || value != prev {
			out = append(out, value)
			prev = value
			prevSet = true
		}
	}
	return out
}

func normalizeAnnotationSubtype(typ string) string {
	trimmed := strings.TrimSpace(typ)
	if trimmed == "" {
		return "Text"
	}
	return strings.TrimPrefix(trimmed, "/")
}

func buildFDFFromAnnotations(entries []annotationFDFEntry) []byte {
	objectCount := len(entries) + 1 // root + annotations
	objects := make([]string, objectCount+1)

	annotRefs := make([]string, 0, len(entries))
	for i := range entries {
		objNum := i + 2
		annotRefs = append(annotRefs, fmt.Sprintf("%d 0 R", objNum))
		objects[objNum] = buildFDFAnnotationObject(entries[i])
	}

	objects[1] = fmt.Sprintf("<< /Type /Catalog /FDF << /Annots [%s] >> >>", strings.Join(annotRefs, " "))

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

func buildFDFAnnotationObject(entry annotationFDFEntry) string {
	rect := fmt.Sprintf("[%s %s %s %s]",
		formatPDFReal(entry.Rect[0]),
		formatPDFReal(entry.Rect[1]),
		formatPDFReal(entry.Rect[2]),
		formatPDFReal(entry.Rect[3]),
	)

	return fmt.Sprintf(
		"<< /Type /Annot /Page %d /Subtype /%s /Rect %s /Contents %s >>",
		entry.Page,
		normalizeAnnotationSubtype(entry.Type),
		rect,
		toPDFLiteralString(entry.Contents),
	)
}

func formatPDFReal(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
