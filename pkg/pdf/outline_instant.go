package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// GetTopLevelOutlineCountSL returns top-level outline count.
func (d *Document) GetTopLevelOutlineCountSL() (int, error) {
	outlines, err := d.Outlines()
	if err != nil {
		return 0, err
	}
	return len(outlines), nil
}

// InstantOutlineGetKids returns dot-path children indexes for one outline path.
// Empty path means root.
func (d *Document) InstantOutlineGetKids(path string) ([]string, error) {
	items, err := d.outlineChildrenByPath(path)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	trimmed := strings.TrimSpace(path)
	out := make([]string, len(items))
	for i := range items {
		if trimmed == "" {
			out[i] = strconv.Itoa(i)
			continue
		}
		out[i] = trimmed + "." + strconv.Itoa(i)
	}
	return out, nil
}

// InstantOutlineGetTitle returns one outline title by dot-path index.
func (d *Document) InstantOutlineGetTitle(path string) (string, error) {
	item, err := d.outlineItemByDotPath(path)
	if err != nil {
		return "", err
	}
	return item.Title, nil
}

// InstantOutlineGetDestPage returns one outline destination page index by dot-path index.
func (d *Document) InstantOutlineGetDestPage(path string) (int, error) {
	item, err := d.outlineItemByDotPath(path)
	if err != nil {
		return -1, err
	}
	return item.PageIndex, nil
}

// InstantOutlineGetDestURI returns one outline destination URI by dot-path index.
func (d *Document) InstantOutlineGetDestURI(path string) (string, error) {
	item, err := d.outlineItemByDotPath(path)
	if err != nil {
		return "", err
	}
	if item.Action == nil {
		return "", nil
	}
	return item.Action.URI, nil
}

// InstantOutlineHasKids reports whether one outline has child items.
func (d *Document) InstantOutlineHasKids(path string) (bool, error) {
	item, err := d.outlineItemByDotPath(path)
	if err != nil {
		return false, err
	}
	return len(item.Children) > 0, nil
}

// InstantOutlineGetType maps one outline item to Java-style type code.
// 1: page destination, 4: URI destination, 0: other/unknown.
func (d *Document) InstantOutlineGetType(path string) (int, error) {
	item, err := d.outlineItemByDotPath(path)
	if err != nil {
		return 0, err
	}

	if item.Action != nil && item.Action.URI != "" {
		return 4, nil
	}
	if item.PageIndex >= 0 {
		return 1, nil
	}
	return 0, nil
}

func (d *Document) outlineChildrenByPath(path string) ([]*Outline, error) {
	outlines, err := d.Outlines()
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return outlines, nil
	}

	indices, err := parseOutlineDotPath(trimmed)
	if err != nil {
		return nil, err
	}

	current := outlines
	for _, idx := range indices {
		if idx < 0 || idx >= len(current) {
			return nil, fmt.Errorf("outline index out of range: %d", idx)
		}
		current = current[idx].Children
	}
	return current, nil
}

func (d *Document) outlineItemByDotPath(path string) (*Outline, error) {
	indices, err := parseOutlineDotPath(path)
	if err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		return nil, fmt.Errorf("outline path is empty")
	}

	outlines, err := d.Outlines()
	if err != nil {
		return nil, err
	}

	current := outlines
	var item *Outline
	for _, idx := range indices {
		if idx < 0 || idx >= len(current) {
			return nil, fmt.Errorf("outline index out of range: %d", idx)
		}
		item = current[idx]
		current = item.Children
	}
	return item, nil
}

func parseOutlineDotPath(path string) ([]int, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("invalid outline path segment: %s", part)
		}
		out = append(out, value)
	}
	return out, nil
}
