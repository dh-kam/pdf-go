package pdf

import (
	"encoding/xml"
	"fmt"
)

type pageTextXMLDoc struct {
	XMLName xml.Name          `xml:"page"`
	Text    string            `xml:"text,omitempty"`
	Items   []pageTextXMLItem `xml:"items>item,omitempty"`
	Index   int               `xml:"index,attr"`
}

type pageTextXMLItem struct {
	Text     string  `xml:"text,attr,omitempty"`
	Font     string  `xml:"font,attr,omitempty"`
	X        float64 `xml:"x,attr"`
	Y        float64 `xml:"y,attr"`
	Width    float64 `xml:"width,attr"`
	Height   float64 `xml:"height,attr"`
	FontSize float64 `xml:"fontSize,attr,omitempty"`
}

// GetPageTextAsXMLSL is a Java-parity API that exports one page text as XML.
func (d *Document) GetPageTextAsXMLSL(pageIndex int) (string, error) {
	text, err := d.Text(pageIndex)
	if err != nil {
		return "", err
	}

	lines, err := d.TextLines(pageIndex)
	if err != nil {
		return "", err
	}

	items := make([]pageTextXMLItem, 0)
	for i := range lines {
		for j := range lines[i].Items {
			item := lines[i].Items[j]
			items = append(items, pageTextXMLItem{
				Text:     item.Text,
				Font:     item.Font,
				X:        item.X,
				Y:        item.Y,
				Width:    item.Width,
				Height:   item.Height,
				FontSize: item.FontSize,
			})
		}
	}

	doc := pageTextXMLDoc{
		Index: pageIndex,
		Text:  text,
		Items: items,
	}
	encoded, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal page text xml: %w", err)
	}

	return xml.Header + string(encoded), nil
}
