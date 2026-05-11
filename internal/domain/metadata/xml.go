package metadata

// XMLNode represents a node in an XML document tree.
// It provides a simple DOM-like interface for XML navigation.
type XMLNode interface {
	// NodeName returns the name of the node (element name or "#text" for text nodes).
	NodeName() string

	// NodeValue returns the value of text nodes, or empty string for elements.
	NodeValue() string

	// ChildNodes returns all child nodes.
	ChildNodes() []XMLNode

	// FirstChild returns the first child node, or nil if none.
	FirstChild() XMLNode

	// NextSibling returns the next sibling node, or nil if none.
	NextSibling() XMLNode

	// ParentNode returns the parent node, or nil if none.
	ParentNode() XMLNode

	// TextContent returns the concatenated text content of the node and all descendants.
	TextContent() string

	// HasChildNodes returns true if the node has child nodes.
	HasChildNodes() bool

	// Attributes returns the attributes of an element node.
	Attributes() []XMLAttribute
}

// XMLAttribute represents an attribute of an XML element.
type XMLAttribute struct {
	Name  string
	Value string
}

// XMLDocument represents an XML document.
type XMLDocument interface {
	// DocumentElement returns the root element of the document.
	DocumentElement() XMLNode
}
