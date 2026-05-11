package pdf

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

type bookmarkXMLDoc struct {
	XMLName   xml.Name          `xml:"bookmarks"`
	Bookmarks []bookmarkXMLNode `xml:"bookmark"`
}

type bookmarkXMLNode struct {
	Bookmarks []bookmarkXMLNode `xml:"bookmark,omitempty"`
	Title     string            `xml:"title,attr,omitempty"`
	Page      int               `xml:"page,attr,omitempty"`
	Color     string            `xml:"color,attr,omitempty"`
	Action    string            `xml:"action,attr,omitempty"`
	URI       string            `xml:"uri,attr,omitempty"`
	File      string            `xml:"file,attr,omitempty"`
	Named     string            `xml:"named,attr,omitempty"`
}

// ExportBookmark exports current outline tree as XML to the given file path.
func (d *Document) ExportBookmark(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("export path is empty")
	}

	outlines, err := d.Outlines()
	if err != nil {
		return err
	}

	doc := bookmarkXMLDoc{
		Bookmarks: make([]bookmarkXMLNode, 0),
	}
	for i := range outlines {
		doc.Bookmarks = append(doc.Bookmarks, outlineToBookmarkXMLNode(outlines[i]))
	}

	encoded, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bookmark xml: %w", err)
	}

	data := append([]byte(xml.Header), encoded...)
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write bookmark xml: %w", err)
	}
	return nil
}

func outlineToBookmarkXMLNode(item *Outline) bookmarkXMLNode {
	if item == nil {
		return bookmarkXMLNode{}
	}

	node := bookmarkXMLNode{
		Title: item.Title,
	}
	if item.PageIndex >= 0 {
		node.Page = item.PageIndex + 1 // Java-parity page number.
	}
	node.Color = fmt.Sprintf("#%06x", item.Color&0x00FFFFFF)
	if item.Action != nil {
		node.Action = item.Action.Type
		node.URI = item.Action.URI
		node.File = item.Action.File
		node.Named = item.Action.Named
		if node.Page == 0 && item.Action.PageIndex >= 0 {
			node.Page = item.Action.PageIndex + 1
		}
	}

	if len(item.Children) == 0 {
		return node
	}

	node.Bookmarks = make([]bookmarkXMLNode, 0, len(item.Children))
	for i := range item.Children {
		node.Bookmarks = append(node.Bookmarks, outlineToBookmarkXMLNode(item.Children[i]))
	}
	return node
}
