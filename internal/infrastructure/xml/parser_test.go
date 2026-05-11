package xml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXMLParser_SimpleElement(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><child>text</child></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	require.NotNil(t, doc)

	root := doc.DocumentElement()
	assert.Equal(t, "root", root.NodeName())
	assert.True(t, root.HasChildNodes())

	children := root.ChildNodes()
	require.Len(t, children, 1)

	child := children[0]
	assert.Equal(t, "child", child.NodeName())
	assert.Equal(t, "text", child.TextContent())
}

func TestXMLParser_NestedElements(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><parent><child>value</child></parent></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	parent := root.FirstChild()
	assert.Equal(t, "parent", parent.NodeName())

	child := parent.FirstChild()
	assert.Equal(t, "child", child.NodeName())
	assert.Equal(t, "value", child.TextContent())
}

func TestXMLParser_Attributes(t *testing.T) {
	parser := NewXMLParser(false, true)

	xml := `<root id="123" name="test"/>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	attrs := root.Attributes()
	require.Len(t, attrs, 2)

	assert.Equal(t, "id", attrs[0].Name)
	assert.Equal(t, "123", attrs[0].Value)

	assert.Equal(t, "name", attrs[1].Name)
	assert.Equal(t, "test", attrs[1].Value)
}

func TestXMLParser_SelfClosingTag(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><child/></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	children := root.ChildNodes()
	require.Len(t, children, 1)

	child := children[0]
	assert.Equal(t, "child", child.NodeName())
	assert.False(t, child.HasChildNodes())
}

func TestXMLParser_CDATA(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><![CDATA[<special>characters</special>]]></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	text := root.TextContent()
	assert.Equal(t, "<special>characters</special>", text)
}

func TestXMLParser_Comment(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><!-- comment --><child>text</child></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	// Comments should be ignored
	children := root.ChildNodes()
	require.Len(t, children, 1)
	assert.Equal(t, "child", children[0].NodeName())
}

func TestXMLParser_EntityResolution(t *testing.T) {
	parser := NewXMLParser(false, false)

	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "named entities",
			xml:      `<root>&lt;&gt;&amp;&quot;&apos;</root>`,
			expected: `<>&"'`,
		},
		{
			name:     "numeric entities",
			xml:      `<root>&#65;&#66;&#67;</root>`,
			expected: "ABC",
		},
		{
			name:     "hex entities",
			xml:      `<root>&#x41;&#x42;&#x43;</root>`,
			expected: "ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parser.ParseFromString(tt.xml)
			require.NoError(t, err)

			root := doc.DocumentElement()
			assert.Equal(t, tt.expected, root.TextContent())
		})
	}
}

func TestXMLParser_LowerCaseName(t *testing.T) {
	parser := NewXMLParser(true, false)

	xml := `<ROOT><CHILD>text</CHILD></ROOT>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	assert.Equal(t, "root", root.NodeName())

	child := root.FirstChild()
	assert.Equal(t, "child", child.NodeName())
}

func TestXMLParser_Whitespace(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root>
		<child>   text   </child>
	</root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	child := root.FirstChild()
	// Whitespace-only text nodes should be ignored
	assert.Equal(t, "   text   ", child.TextContent())
}

func TestXMLParser_MultipleSiblings(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><a>1</a><b>2</b><c>3</c></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	children := root.ChildNodes()
	require.Len(t, children, 3)

	assert.Equal(t, "a", children[0].NodeName())
	assert.Equal(t, "1", children[0].TextContent())

	assert.Equal(t, "b", children[1].NodeName())
	assert.Equal(t, "2", children[1].TextContent())

	assert.Equal(t, "c", children[2].NodeName())
	assert.Equal(t, "3", children[2].TextContent())
}

func TestXMLParser_EmptyElement(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root></root>`
	doc, err := parser.ParseFromString(xml)

	require.NoError(t, err)
	root := doc.DocumentElement()

	assert.Equal(t, "root", root.NodeName())
	assert.False(t, root.HasChildNodes())
}

func TestXMLParser_MalformedXML(t *testing.T) {
	parser := NewXMLParser(false, false)

	tests := []struct {
		name string
		xml  string
	}{
		{"unterminated element", `<root><child>`},
		{"unterminated attribute", `<root attr="value`},
		{"unterminated comment", `<root><!-- comment`},
		{"unterminated CDATA", `<root><![CDATA[text`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parser.ParseFromString(tt.xml)
			assert.Error(t, err)
			assert.Nil(t, doc)
		})
	}
}

func TestDOMNode_Navigation(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><a>1</a><b>2</b><c>3</c></root>`
	doc, err := parser.ParseFromString(xml)
	require.NoError(t, err)

	root := doc.DocumentElement()
	firstChild := root.FirstChild()

	assert.Equal(t, "a", firstChild.NodeName())

	secondChild := firstChild.NextSibling()
	require.NotNil(t, secondChild)
	assert.Equal(t, "b", secondChild.NodeName())

	thirdChild := secondChild.NextSibling()
	require.NotNil(t, thirdChild)
	assert.Equal(t, "c", thirdChild.NodeName())

	noMoreSiblings := thirdChild.NextSibling()
	assert.Nil(t, noMoreSiblings)
}

func TestDOMNode_ParentNode(t *testing.T) {
	parser := NewXMLParser(false, false)

	xml := `<root><child>text</child></root>`
	doc, err := parser.ParseFromString(xml)
	require.NoError(t, err)

	root := doc.DocumentElement()
	child := root.FirstChild()

	parent := child.ParentNode()
	require.NotNil(t, parent)
	assert.Equal(t, "root", parent.NodeName())
}
