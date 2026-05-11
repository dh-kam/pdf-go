package metadata

import "time"

// Metadata represents PDF document metadata extracted from XMP (Extensible Metadata Platform).
// It contains standard metadata fields from Dublin Core, XMP Basic, and PDF namespaces.
type Metadata struct {
	createDate   time.Time
	modifyDate   time.Time
	metadataDate time.Time
	description  string
	creatorTool  string
	producer     string
	rawData      string
	title        []string
	creator      []string
	subject      []string
	keywords     []string
}

// NewMetadata creates a new Metadata instance with optional raw data.
func NewMetadata(rawData string) *Metadata {
	return &Metadata{
		rawData: rawData,
	}
}

// Title returns the document title(s).
// XMP allows multiple titles for different languages.
func (m *Metadata) Title() []string {
	return m.title
}

// SetTitle sets the document title(s).
func (m *Metadata) SetTitle(title []string) {
	m.title = title
}

// Creator returns the document creator(s)/author(s).
func (m *Metadata) Creator() []string {
	return m.creator
}

// SetCreator sets the document creator(s)/author(s).
func (m *Metadata) SetCreator(creator []string) {
	m.creator = creator
}

// Subject returns the document subject(s)/keyword(s).
func (m *Metadata) Subject() []string {
	return m.subject
}

// SetSubject sets the document subject(s)/keyword(s).
func (m *Metadata) SetSubject(subject []string) {
	m.subject = subject
}

// Description returns the document description.
func (m *Metadata) Description() string {
	return m.description
}

// SetDescription sets the document description.
func (m *Metadata) SetDescription(description string) {
	m.description = description
}

// CreateDate returns the document creation date.
func (m *Metadata) CreateDate() time.Time {
	return m.createDate
}

// SetCreateDate sets the document creation date.
func (m *Metadata) SetCreateDate(date time.Time) {
	m.createDate = date
}

// ModifyDate returns the document modification date.
func (m *Metadata) ModifyDate() time.Time {
	return m.modifyDate
}

// SetModifyDate sets the document modification date.
func (m *Metadata) SetModifyDate(date time.Time) {
	m.modifyDate = date
}

// CreatorTool returns the tool used to create the original document.
func (m *Metadata) CreatorTool() string {
	return m.creatorTool
}

// SetCreatorTool sets the tool used to create the original document.
func (m *Metadata) SetCreatorTool(tool string) {
	m.creatorTool = tool
}

// MetadataDate returns the metadata modification date.
func (m *Metadata) MetadataDate() time.Time {
	return m.metadataDate
}

// SetMetadataDate sets the metadata modification date.
func (m *Metadata) SetMetadataDate(date time.Time) {
	m.metadataDate = date
}

// Producer returns the PDF producer software.
func (m *Metadata) Producer() string {
	return m.producer
}

// SetProducer sets the PDF producer software.
func (m *Metadata) SetProducer(producer string) {
	m.producer = producer
}

// Keywords returns the document keywords.
func (m *Metadata) Keywords() []string {
	return m.keywords
}

// SetKeywords sets the document keywords.
func (m *Metadata) SetKeywords(keywords []string) {
	m.keywords = keywords
}

// RawData returns the original XMP XML string.
func (m *Metadata) RawData() string {
	return m.rawData
}
