package pdf

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultBookmarkColor = 0xFF000000

type bookmarkEntry struct {
	path []int
	item *Outline
}

// GetBookmarkCount is a Java-parity helper that returns the flattened bookmark count.
func (d *Document) GetBookmarkCount() (int, error) {
	entries, err := d.flattenBookmarkEntries()
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// GetBookmarkTitle is a Java-parity helper that returns bookmark title by flattened index.
func (d *Document) GetBookmarkTitle(index int) (string, error) {
	entries, err := d.flattenBookmarkEntries()
	if err != nil {
		return "", err
	}
	if index < 0 || index >= len(entries) {
		return "", fmt.Errorf("bookmark index out of range: %d", index)
	}
	return entries[index].item.Title, nil
}

// GetBookmarkPageNo is a Java-parity helper that returns bookmark destination page index.
func (d *Document) GetBookmarkPageNo(index int) (int, error) {
	entries, err := d.flattenBookmarkEntries()
	if err != nil {
		return -1, err
	}
	if index < 0 || index >= len(entries) {
		return -1, fmt.Errorf("bookmark index out of range: %d", index)
	}
	return entries[index].item.PageIndex, nil
}

// GetBookmarkColor is a Java-parity helper that returns bookmark color as ARGB.
func (d *Document) GetBookmarkColor(index int) (int, error) {
	entries, err := d.flattenBookmarkEntries()
	if err != nil {
		return 0, err
	}
	if index < 0 || index >= len(entries) {
		return 0, fmt.Errorf("bookmark index out of range: %d", index)
	}
	return normalizeBookmarkColor(entries[index].item.Color), nil
}

// FindBookmarkByPage is a Java-parity helper that finds flattened bookmark index by page index.
func (d *Document) FindBookmarkByPage(pageIndex int) (int, error) {
	entries, err := d.flattenBookmarkEntries()
	if err != nil {
		return -1, err
	}
	for i := range entries {
		if entries[i].item.PageIndex == pageIndex {
			return i, nil
		}
	}
	return -1, nil
}

// AddBookmark appends a top-level bookmark.
func (d *Document) AddBookmark(pageIndex int, title string, color int) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("bookmark title is empty")
	}

	pageCount, err := d.PageCount()
	if err != nil {
		return err
	}
	if pageIndex < 0 || pageIndex >= pageCount {
		return fmt.Errorf("page index out of range: %d", pageIndex)
	}

	return d.AddOutline(nil, &Outline{
		Title:     title,
		PageIndex: pageIndex,
		Color:     normalizeBookmarkColor(color),
	})
}

// SetBookmarkTitle updates bookmark title by flattened bookmark index.
func (d *Document) SetBookmarkTitle(index int, title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("bookmark title is empty")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	path, err := d.flattenBookmarkPathByIndexLocked(index)
	if err != nil {
		return err
	}
	item, err := d.outlineByPathLocked(path)
	if err != nil {
		return err
	}
	item.Title = title
	d.outlinesSet = true
	return nil
}

// SetBookmarkColor updates bookmark color by flattened bookmark index.
func (d *Document) SetBookmarkColor(index int, color int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	path, err := d.flattenBookmarkPathByIndexLocked(index)
	if err != nil {
		return err
	}
	item, err := d.outlineByPathLocked(path)
	if err != nil {
		return err
	}
	item.Color = normalizeBookmarkColor(color)
	d.outlinesSet = true
	return nil
}

// RemoveBookmark removes a bookmark by flattened bookmark index.
func (d *Document) RemoveBookmark(index int) error {
	entries, err := d.flattenBookmarkEntries()
	if err != nil {
		return err
	}
	if index < 0 || index >= len(entries) {
		return fmt.Errorf("bookmark index out of range: %d", index)
	}
	return d.RemoveOutline(entries[index].path)
}

// RemoveAllBookmark removes all bookmarks in current session state.
func (d *Document) RemoveAllBookmark() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.outlines = nil
	d.outlinesSet = true
	return nil
}

// ImportBookmark imports bookmarks from an XML file.
func (d *Document) ImportBookmark(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("import path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bookmark xml: %w", err)
	}

	var xmlDoc bookmarkXMLDoc
	if err := xml.Unmarshal(data, &xmlDoc); err != nil {
		return fmt.Errorf("unmarshal bookmark xml: %w", err)
	}

	items := make([]*Outline, 0, len(xmlDoc.Bookmarks))
	for i := range xmlDoc.Bookmarks {
		items = append(items, bookmarkXMLNodeToOutline(xmlDoc.Bookmarks[i]))
	}

	d.mu.Lock()
	d.outlines = items
	d.outlinesSet = true
	d.mu.Unlock()
	return nil
}

// GetOutlineXMLSL returns current outline tree serialized as XML.
func (d *Document) GetOutlineXMLSL() (string, error) {
	outlines, err := d.Outlines()
	if err != nil {
		return "", err
	}

	doc := bookmarkXMLDoc{
		Bookmarks: make([]bookmarkXMLNode, 0, len(outlines)),
	}
	for i := range outlines {
		doc.Bookmarks = append(doc.Bookmarks, outlineToBookmarkXMLNode(outlines[i]))
	}

	encoded, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal outline xml: %w", err)
	}
	return xml.Header + string(encoded) + "\n", nil
}

func (d *Document) flattenBookmarkEntries() ([]bookmarkEntry, error) {
	outlines, err := d.Outlines()
	if err != nil {
		return nil, err
	}

	entries := make([]bookmarkEntry, 0)
	var walk func(items []*Outline, parentPath []int)
	walk = func(items []*Outline, parentPath []int) {
		for i := range items {
			path := append(append([]int(nil), parentPath...), i)
			entries = append(entries, bookmarkEntry{
				path: path,
				item: items[i],
			})
			if len(items[i].Children) > 0 {
				walk(items[i].Children, path)
			}
		}
	}
	walk(outlines, nil)
	return entries, nil
}

func (d *Document) flattenBookmarkPathByIndexLocked(index int) ([]int, error) {
	if err := d.ensureMutableOutlinesLocked(); err != nil {
		return nil, err
	}

	entries := make([]bookmarkEntry, 0)
	var walk func(items []*Outline, parentPath []int)
	walk = func(items []*Outline, parentPath []int) {
		for i := range items {
			path := append(append([]int(nil), parentPath...), i)
			entries = append(entries, bookmarkEntry{path: path, item: items[i]})
			if len(items[i].Children) > 0 {
				walk(items[i].Children, path)
			}
		}
	}
	walk(d.outlines, nil)

	if index < 0 || index >= len(entries) {
		return nil, fmt.Errorf("bookmark index out of range: %d", index)
	}
	return entries[index].path, nil
}

func (d *Document) outlineByPathLocked(path []int) (*Outline, error) {
	current := d.outlines
	var item *Outline
	for _, idx := range path {
		if idx < 0 || idx >= len(current) {
			return nil, fmt.Errorf("outline path index out of range: %d", idx)
		}
		item = current[idx]
		current = item.Children
	}
	if item == nil {
		return nil, fmt.Errorf("outline item is nil")
	}
	return item, nil
}

func bookmarkXMLNodeToOutline(node bookmarkXMLNode) *Outline {
	item := &Outline{
		Title:     node.Title,
		PageIndex: -1,
		Color:     parseBookmarkColorString(node.Color),
	}
	if node.Page > 0 {
		item.PageIndex = node.Page - 1
	}
	if node.Action != "" || node.URI != "" || node.File != "" || node.Named != "" {
		action := &OutlineAction{
			Type:  node.Action,
			URI:   node.URI,
			File:  node.File,
			Named: node.Named,
		}
		if item.PageIndex >= 0 {
			action.PageIndex = item.PageIndex
		} else {
			action.PageIndex = -1
		}
		item.Action = action
	}
	if len(node.Bookmarks) > 0 {
		item.Children = make([]*Outline, 0, len(node.Bookmarks))
		for i := range node.Bookmarks {
			item.Children = append(item.Children, bookmarkXMLNodeToOutline(node.Bookmarks[i]))
		}
	}
	return item
}

func parseBookmarkColorString(value string) int {
	colorText := strings.TrimSpace(value)
	if colorText == "" {
		return defaultBookmarkColor
	}

	if strings.HasPrefix(colorText, "#") {
		if len(colorText) == 7 {
			if parsed, err := strconv.ParseInt(colorText[1:], 16, 32); err == nil {
				return normalizeBookmarkColor(int(parsed))
			}
		}
		if len(colorText) == 9 {
			if parsed, err := strconv.ParseInt(colorText[1:], 16, 32); err == nil {
				return normalizeBookmarkColor(int(parsed))
			}
		}
	}

	if parsed, err := strconv.ParseInt(colorText, 0, 32); err == nil {
		return normalizeBookmarkColor(int(parsed))
	}

	return defaultBookmarkColor
}

func normalizeBookmarkColor(color int) int {
	return defaultBookmarkColor | (color & 0x00FFFFFF)
}
