package pdf

import (
	"github.com/dh-kam/pdf-go/internal/domain/metadata"
)

// Metadata represents PDF document metadata extracted from XMP.
type Metadata struct {
	*metadata.Metadata
}

// GetMetadata returns the parsed XMP metadata from the document.
// Returns nil if no metadata is present or if parsing failed.
func (d *Document) GetMetadata() *Metadata {
	meta := d.doc.ParsedMetadata()
	if meta == nil {
		return nil
	}
	return &Metadata{Metadata: meta}
}

// Title returns the document title(s).
func (m *Metadata) Title() []string {
	if m == nil || m.Metadata == nil {
		return nil
	}
	return m.Metadata.Title()
}

// Author returns the document creator(s)/author(s).
func (m *Metadata) Author() []string {
	if m == nil || m.Metadata == nil {
		return nil
	}
	return m.Metadata.Creator()
}

// Subject returns the document subject(s)/keyword(s).
func (m *Metadata) Subject() []string {
	if m == nil || m.Metadata == nil {
		return nil
	}
	return m.Metadata.Subject()
}

// Description returns the document description.
func (m *Metadata) Description() string {
	if m == nil || m.Metadata == nil {
		return ""
	}
	return m.Metadata.Description()
}

// Producer returns the PDF producer software.
func (m *Metadata) Producer() string {
	if m == nil || m.Metadata == nil {
		return ""
	}
	return m.Metadata.Producer()
}

// CreatorTool returns the tool used to create the original document.
func (m *Metadata) CreatorTool() string {
	if m == nil || m.Metadata == nil {
		return ""
	}
	return m.Metadata.CreatorTool()
}

// Keywords returns the document keywords.
func (m *Metadata) Keywords() []string {
	if m == nil || m.Metadata == nil {
		return nil
	}
	return m.Metadata.Keywords()
}
