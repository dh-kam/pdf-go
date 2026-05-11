package xml

import (
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/metadata"
)

// DOMNode implements the XMLNode interface.
// It represents a node in an XML document tree.
type DOMNode struct {
	nodeName   string
	nodeValue  string
	childNodes []*DOMNode
	parentNode *DOMNode
	attributes []metadata.XMLAttribute
}

// NewDOMNode creates a new DOM node with the given name and value.
func NewDOMNode(nodeName, nodeValue string) *DOMNode {
	return &DOMNode{
		nodeName:   nodeName,
		nodeValue:  nodeValue,
		childNodes: make([]*DOMNode, 0),
	}
}

// NodeName returns the name of the node.
func (n *DOMNode) NodeName() string {
	return n.nodeName
}

// NodeValue returns the value of the node.
func (n *DOMNode) NodeValue() string {
	return n.nodeValue
}

// ChildNodes returns all child nodes.
func (n *DOMNode) ChildNodes() []metadata.XMLNode {
	nodes := make([]metadata.XMLNode, len(n.childNodes))
	for i, child := range n.childNodes {
		nodes[i] = child
	}
	return nodes
}

// FirstChild returns the first child node, or nil if none.
func (n *DOMNode) FirstChild() metadata.XMLNode {
	if len(n.childNodes) == 0 {
		return nil
	}
	return n.childNodes[0]
}

// NextSibling returns the next sibling node, or nil if none.
func (n *DOMNode) NextSibling() metadata.XMLNode {
	if n.parentNode == nil {
		return nil
	}

	siblings := n.parentNode.childNodes
	for i, child := range siblings {
		if child == n && i+1 < len(siblings) {
			return siblings[i+1]
		}
	}
	return nil
}

// ParentNode returns the parent node, or nil if none.
func (n *DOMNode) ParentNode() metadata.XMLNode {
	return n.parentNode
}

// TextContent returns the concatenated text content of the node and all descendants.
func (n *DOMNode) TextContent() string {
	if len(n.childNodes) == 0 {
		return n.nodeValue
	}

	var builder strings.Builder
	for _, child := range n.childNodes {
		builder.WriteString(child.TextContent())
	}
	return builder.String()
}

// HasChildNodes returns true if the node has child nodes.
func (n *DOMNode) HasChildNodes() bool {
	return len(n.childNodes) > 0
}

// Attributes returns the attributes of the node.
func (n *DOMNode) Attributes() []metadata.XMLAttribute {
	return n.attributes
}

// setAttributes sets the attributes of the node.
func (n *DOMNode) setAttributes(attrs []metadata.XMLAttribute) {
	n.attributes = attrs
}

// Document implements the XMLDocument interface.
type Document struct {
	documentElement *DOMNode
}

// NewDocument creates a new XML document with the given root element.
func NewDocument(root *DOMNode) *Document {
	return &Document{
		documentElement: root,
	}
}

// DocumentElement returns the root element of the document.
func (d *Document) DocumentElement() metadata.XMLNode {
	return d.documentElement
}
