package metadataparser

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataParser_SimpleMetadata(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description>
      <dc:title>Test Document</dc:title>
      <dc:description>A test PDF document</dc:description>
      <pdf:Producer>Test Producer</pdf:Producer>
      <xmp:CreatorTool>Test Tool</xmp:CreatorTool>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, []string{"Test Document"}, metadata.Title())
	assert.Equal(t, "A test PDF document", metadata.Description())
	assert.Equal(t, "Test Producer", metadata.Producer())
	assert.Equal(t, "Test Tool", metadata.CreatorTool())
}

func TestMetadataParser_ArrayMetadata(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:dc="http://purl.org/dc/elements/1.1/">
  <rdf:Description>
    <dc:creator>
      <rdf:Seq>
        <rdf:li>Author One</rdf:li>
        <rdf:li>Author Two</rdf:li>
      </rdf:Seq>
    </dc:creator>
    <dc:subject>
      <rdf:Bag>
        <rdf:li>Keyword1</rdf:li>
        <rdf:li>Keyword2</rdf:li>
      </rdf:Bag>
    </dc:subject>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, []string{"Author One", "Author Two"}, metadata.Creator())
	assert.Equal(t, []string{"Keyword1", "Keyword2"}, metadata.Subject())
}

func TestMetadataParser_EmptyMetadata(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Empty(t, metadata.Title())
	assert.Empty(t, metadata.Creator())
	assert.Empty(t, metadata.Subject())
}

func TestMetadataParser_MalformedXML(t *testing.T) {
	xmp := `<invalid xml`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	// Should not return error, just empty metadata
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Should preserve raw data
	assert.Equal(t, xmp, metadata.RawData())
}

func TestMetadataParser_NoRDFRoot(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<root>
  <title>Not RDF</title>
</root>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Should return empty metadata since it's not valid RDF
	assert.Empty(t, metadata.Title())
}

func TestMetadataParser_WithXMPMetaWrapper(t *testing.T) {
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
           xmlns:dc="http://purl.org/dc/elements/1.1/">
    <rdf:Description>
      <dc:title>Document with xmpmeta</dc:title>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, []string{"Document with xmpmeta"}, metadata.Title())
}

func TestMetadataParser_EntityResolution(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description>
    <dc:title>Test &lt;HTML&gt; &amp; &quot;Entities&quot;</dc:title>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, []string{`Test <HTML> & "Entities"`}, metadata.Title())
}

func TestMetadataParser_MultipleDescriptions(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description>
    <dc:title>Title One</dc:title>
  </rdf:Description>
  <rdf:Description>
    <dc:description>Description Two</dc:description>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Should parse both descriptions
	assert.Equal(t, []string{"Title One"}, metadata.Title())
	assert.Equal(t, "Description Two", metadata.Description())
}

func TestMetadataParser_CaseInsensitive(t *testing.T) {
	// XML parser should lowercase element names
	xmp := `<?xml version="1.0"?>
<RDF:RDF xmlns:RDF="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <RDF:Description>
    <DC:title>Case Test</DC:title>
  </RDF:Description>
</RDF:RDF>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Should work with lowercase matching
	assert.Equal(t, []string{"Case Test"}, metadata.Title())
}

func TestMetadataParser_WhitespaceHandling(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description>
    <dc:title>
      Title with whitespace
    </dc:title>
    <dc:creator>
      <rdf:Seq>
        <rdf:li>  Author Name  </rdf:li>
      </rdf:Seq>
    </dc:creator>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Whitespace should be trimmed
	title := metadata.Title()
	require.Len(t, title, 1)
	assert.Contains(t, title[0], "Title with whitespace")

	creator := metadata.Creator()
	require.Len(t, creator, 1)
	assert.Equal(t, "Author Name", creator[0])
}

func TestMetadataParser_RawDataPreserved(t *testing.T) {
	xmp := `<original>data</original>`

	parser := NewMetadataParser(xmp)
	metadata, err := parser.Parse()

	require.NoError(t, err)
	assert.Equal(t, xmp, metadata.RawData())
}

func TestMetadataParser_DateFields_ISO8601(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description>
    <xmp:CreateDate>2026-02-16T13:45:30+09:00</xmp:CreateDate>
    <xmp:ModifyDate>2026-02-16T05:00:00Z</xmp:ModifyDate>
    <xmp:MetadataDate>2026-02-16</xmp:MetadataDate>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	md, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, md)

	loc := time.FixedZone("", 9*60*60)
	assert.Equal(t, time.Date(2026, time.February, 16, 13, 45, 30, 0, loc).Format(time.RFC3339), md.CreateDate().Format(time.RFC3339))
	assert.Equal(t, time.Date(2026, time.February, 16, 5, 0, 0, 0, time.UTC).Format(time.RFC3339), md.ModifyDate().Format(time.RFC3339))
	assert.Equal(t, time.Date(2026, time.February, 16, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), md.MetadataDate().Format(time.RFC3339))
}

func TestMetadataParser_DateFields_PDFDateFallback(t *testing.T) {
	xmp := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description>
    <xmp:CreateDate>D:20260216134530+09'00'</xmp:CreateDate>
    <xmp:ModifyDate>D:20260215120000-05'30'</xmp:ModifyDate>
  </rdf:Description>
</rdf:RDF>`

	parser := NewMetadataParser(xmp)
	md, err := parser.Parse()

	require.NoError(t, err)
	require.NotNil(t, md)

	assert.Equal(t, time.Date(2026, time.February, 16, 13, 45, 30, 0, time.FixedZone("", 9*60*60)).Format(time.RFC3339), md.CreateDate().Format(time.RFC3339))
	assert.Equal(t, time.Date(2026, time.February, 15, 12, 0, 0, 0, time.FixedZone("", -(5*60*60+30*60))).Format(time.RFC3339), md.ModifyDate().Format(time.RFC3339))
}
