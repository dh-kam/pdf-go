package metadataparser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dh-kam/pdf-go/internal/domain/metadata"
	"github.com/dh-kam/pdf-go/internal/infrastructure/xml"
)

// MetadataParser parses XMP (Extensible Metadata Platform) metadata from PDF documents.
type MetadataParser struct {
	metadataMap map[string]interface{}
	data        string
}

// NewMetadataParser creates a new metadata parser with the given XMP data.
func NewMetadataParser(data string) *MetadataParser {
	return &MetadataParser{
		data:        data,
		metadataMap: make(map[string]interface{}),
	}
}

// Parse parses the XMP metadata and returns a Metadata entity.
func (p *MetadataParser) Parse() (*metadata.Metadata, error) {
	// Repair Ghostscript-generated metadata
	repairedData := p.repair(p.data)

	// Parse XML
	parser := xml.NewXMLParser(true, false)
	doc, err := parser.ParseFromString(repairedData)
	if err != nil {
		// If parsing fails, return empty metadata with raw data
		return metadata.NewMetadata(p.data), nil
	}

	// Parse RDF structure
	p.parseRDF(doc)

	// Build Metadata entity from parsed map
	return p.buildMetadata(), nil
}

// repair fixes common issues in Ghostscript-generated metadata.
func (p *MetadataParser) repair(data string) string {
	// Remove junk before the first tag
	data = regexp.MustCompile(`^[^<]+`).ReplaceAllString(data, "")

	// Fix Ghostscript UTF-16BE encoding issues
	// Pattern: >\376\377([^<]+)
	re := regexp.MustCompile(`>\\376\\377([^<]+)`)
	data = re.ReplaceAllStringFunc(data, func(match string) string {
		codes := re.FindStringSubmatch(match)[1]

		// Decode octal escapes (\ddd)
		octalRe := regexp.MustCompile(`\\([0-3])([0-7])([0-7])`)
		codes = octalRe.ReplaceAllStringFunc(codes, func(octal string) string {
			parts := octalRe.FindStringSubmatch(octal)
			d1Val := int(parts[1][0] - '0')
			d2Val := int(parts[2][0] - '0')
			d3Val := int(parts[3][0] - '0')
			charCode := d1Val*64 + d2Val*8 + d3Val
			return string(rune(charCode))
		})

		// Unescape XML entities
		codes = strings.ReplaceAll(codes, "&amp;", "&")
		codes = strings.ReplaceAll(codes, "&apos;", "'")
		codes = strings.ReplaceAll(codes, "&gt;", ">")
		codes = strings.ReplaceAll(codes, "&lt;", "<")
		codes = strings.ReplaceAll(codes, "&quot;", "\"")

		// Convert UTF-16BE to UTF-8
		var result strings.Builder
		result.WriteString(">")

		for i := 0; i+1 < len(codes); i += 2 {
			highByte := int(codes[i])
			lowByte := int(codes[i+1])
			charCode := highByte*256 + lowByte

			// Only include printable characters and escape special XML chars
			if charCode >= 32 && charCode < 127 && charCode != '<' && charCode != '>' && charCode != '&' {
				result.WriteRune(rune(charCode))
			} else {
				result.WriteString("&#x")
				result.WriteString(strings.ToUpper(strings.TrimPrefix(
					strings.ToUpper(string(rune(0x10000+charCode))), "1")))
				result.WriteString(";")
			}
		}

		return result.String()
	})

	return data
}

// parseRDF parses the RDF/XML structure.
func (p *MetadataParser) parseRDF(doc metadata.XMLDocument) {
	root := doc.DocumentElement()
	if root == nil {
		return
	}

	rdf := root
	if rdf.NodeName() != "rdf:rdf" {
		// Wrapped in <xmpmeta> or <x:xmpmeta>
		rdf = rdf.FirstChild()
		for rdf != nil && rdf.NodeName() != "rdf:rdf" {
			rdf = rdf.NextSibling()
		}
	}

	if rdf == nil || rdf.NodeName() != "rdf:rdf" || !rdf.HasChildNodes() {
		return
	}

	// Process each rdf:Description
	for _, child := range rdf.ChildNodes() {
		if child.NodeName() != "rdf:description" {
			continue
		}

		for _, entry := range child.ChildNodes() {
			name := entry.NodeName()
			if name == "#text" {
				continue
			}

			// Handle array types (dc:creator, dc:subject)
			if name == "dc:creator" || name == "dc:subject" {
				p.parseArray(entry)
			} else {
				// Simple text value
				value := strings.TrimSpace(entry.TextContent())
				p.metadataMap[name] = value
			}
		}
	}
}

// parseArray parses an array metadata field (Bag, Seq, Alt).
func (p *MetadataParser) parseArray(entry metadata.XMLNode) {
	if !entry.HasChildNodes() {
		return
	}

	// Child must be a Bag (unordered), Seq (ordered), or Alt (alternatives)
	children := entry.ChildNodes()
	if len(children) == 0 {
		return
	}

	seqNode := children[0]
	sequence := p.getSequence(seqNode)

	if len(sequence) > 0 {
		values := make([]string, 0, len(sequence))
		for _, node := range sequence {
			values = append(values, strings.TrimSpace(node.TextContent()))
		}
		p.metadataMap[entry.NodeName()] = values
	}
}

// getSequence extracts rdf:li elements from a Bag/Seq/Alt container.
func (p *MetadataParser) getSequence(entry metadata.XMLNode) []metadata.XMLNode {
	name := entry.NodeName()
	if name != "rdf:bag" && name != "rdf:seq" && name != "rdf:alt" {
		return nil
	}

	result := make([]metadata.XMLNode, 0)
	for _, node := range entry.ChildNodes() {
		if node.NodeName() == "rdf:li" {
			result = append(result, node)
		}
	}
	return result
}

// buildMetadata creates a Metadata entity from the parsed map.
func (p *MetadataParser) buildMetadata() *metadata.Metadata {
	m := metadata.NewMetadata(p.data)

	// Dublin Core fields
	if val, ok := p.metadataMap["dc:title"]; ok {
		if arr, ok := val.([]string); ok {
			m.SetTitle(arr)
		} else if str, ok := val.(string); ok {
			m.SetTitle([]string{str})
		}
	}

	if val, ok := p.metadataMap["dc:creator"]; ok {
		if arr, ok := val.([]string); ok {
			m.SetCreator(arr)
		}
	}

	if val, ok := p.metadataMap["dc:subject"]; ok {
		if arr, ok := val.([]string); ok {
			m.SetSubject(arr)
		}
	}

	if val, ok := p.metadataMap["dc:description"]; ok {
		if str, ok := val.(string); ok {
			m.SetDescription(str)
		}
	}

	// XMP Basic fields
	if val, ok := p.metadataMap["xmp:creatortool"]; ok {
		if str, ok := val.(string); ok {
			m.SetCreatorTool(str)
		}
	}

	if date, ok := p.parseDateField("xmp:createdate", "xap:createdate"); ok {
		m.SetCreateDate(date)
	}

	if date, ok := p.parseDateField("xmp:modifydate", "xap:modifydate"); ok {
		m.SetModifyDate(date)
	}

	if date, ok := p.parseDateField("xmp:metadatadate", "xap:metadatadate"); ok {
		m.SetMetadataDate(date)
	}

	// PDF fields
	if val, ok := p.metadataMap["pdf:producer"]; ok {
		if str, ok := val.(string); ok {
			m.SetProducer(str)
		}
	}

	if val, ok := p.metadataMap["pdf:keywords"]; ok {
		if arr, ok := val.([]string); ok {
			m.SetKeywords(arr)
		} else if str, ok := val.(string); ok {
			m.SetKeywords([]string{str})
		}
	}

	return m
}

func (p *MetadataParser) parseDateField(keys ...string) (time.Time, bool) {
	for _, key := range keys {
		raw, ok := p.metadataMap[key]
		if !ok {
			continue
		}

		switch value := raw.(type) {
		case string:
			if parsed, err := parseMetadataDate(value); err == nil {
				return parsed, true
			}
		case []string:
			for _, entry := range value {
				if parsed, err := parseMetadataDate(entry); err == nil {
					return parsed, true
				}
			}
		}
	}

	return time.Time{}, false
}

func parseMetadataDate(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("metadata date is empty")
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05.999999999Z0700",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}

	if looksLikePDFDate(value) {
		return parsePDFDateString(value)
	}

	return time.Time{}, fmt.Errorf("unsupported metadata date format: %s", value)
}

func looksLikePDFDate(value string) bool {
	if strings.HasPrefix(value, "D:") {
		return true
	}

	if len(value) < 4 {
		return false
	}

	for i := 0; i < 4; i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func parsePDFDateString(value string) (time.Time, error) {
	date := strings.TrimSpace(value)
	if strings.HasPrefix(date, "D:") {
		date = date[2:]
	}

	if len(date) < 4 {
		return time.Time{}, fmt.Errorf("invalid pdf date: %s", value)
	}

	year, err := parseNumericPart(date, 0, 4)
	if err != nil {
		return time.Time{}, err
	}

	month, err := parseOptionalNumericPart(date, 4, 2, 1, 12, 1)
	if err != nil {
		return time.Time{}, err
	}
	day, err := parseOptionalNumericPart(date, 6, 2, 1, 31, 1)
	if err != nil {
		return time.Time{}, err
	}
	hour, err := parseOptionalNumericPart(date, 8, 2, 0, 23, 0)
	if err != nil {
		return time.Time{}, err
	}
	minute, err := parseOptionalNumericPart(date, 10, 2, 0, 59, 0)
	if err != nil {
		return time.Time{}, err
	}
	second, err := parseOptionalNumericPart(date, 12, 2, 0, 59, 0)
	if err != nil {
		return time.Time{}, err
	}

	location := time.UTC
	if len(date) > 14 {
		tz, tzErr := parsePDFDateTimezone(date[14:])
		if tzErr != nil {
			return time.Time{}, tzErr
		}
		location = tz
	}

	return time.Date(year, time.Month(month), day, hour, minute, second, 0, location), nil
}

func parsePDFDateTimezone(tz string) (*time.Location, error) {
	if tz == "" || tz == "Z" {
		return time.UTC, nil
	}

	sign := tz[0]
	if sign != '+' && sign != '-' {
		return nil, fmt.Errorf("invalid pdf timezone: %s", tz)
	}

	offsetHours, err := parseOptionalNumericPart(tz, 1, 2, 0, 23, 0)
	if err != nil {
		return nil, err
	}

	offsetMinutes := 0
	if len(tz) > 3 {
		trimmed := strings.TrimPrefix(tz[3:], "'")
		trimmed = strings.TrimPrefix(trimmed, ":")
		trimmed = strings.TrimSuffix(trimmed, "'")
		if len(trimmed) >= 2 {
			offsetMinutes, err = parseNumericPart(trimmed, 0, 2)
			if err != nil {
				return nil, err
			}
			if offsetMinutes < 0 || offsetMinutes > 59 {
				return nil, fmt.Errorf("invalid pdf timezone minutes: %d", offsetMinutes)
			}
		}
	}

	totalOffset := offsetHours*3600 + offsetMinutes*60
	if sign == '-' {
		totalOffset *= -1
	}

	return time.FixedZone("", totalOffset), nil
}

func parseOptionalNumericPart(value string, start, length, min, max, defaultValue int) (int, error) {
	if len(value) < start+length {
		return defaultValue, nil
	}
	part, err := parseNumericPart(value, start, length)
	if err != nil {
		return 0, err
	}
	if part < min || part > max {
		return 0, fmt.Errorf("numeric part out of range: %d", part)
	}
	return part, nil
}

func parseNumericPart(value string, start, length int) (int, error) {
	if len(value) < start+length {
		return 0, fmt.Errorf("invalid numeric segment in date")
	}

	parsed := 0
	for i := 0; i < length; i++ {
		ch := value[start+i]
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid numeric segment in date")
		}
		parsed = parsed*10 + int(ch-'0')
	}
	return parsed, nil
}
