package pdf

import "fmt"

// InsertPage inserts a duplicate of an existing page at the target index.
// Both indexes are 0-based and refer to the current session page order.
func (d *Document) InsertPage(index, sourcePageIndex int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.ensurePageOrderLocked(); err != nil {
		return err
	}

	if sourcePageIndex < 0 || sourcePageIndex >= len(d.pageOrder) {
		return fmt.Errorf("source page index out of range: %d", sourcePageIndex)
	}
	if index < 0 || index > len(d.pageOrder) {
		return fmt.Errorf("insert index out of range: %d", index)
	}

	pageRef := d.pageOrder[sourcePageIndex]
	d.pageOrder = append(d.pageOrder, 0)
	copy(d.pageOrder[index+1:], d.pageOrder[index:])
	d.pageOrder[index] = pageRef
	return nil
}

// RemovePage removes a page from the current session order.
func (d *Document) RemovePage(index int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.ensurePageOrderLocked(); err != nil {
		return err
	}

	if index < 0 || index >= len(d.pageOrder) {
		return fmt.Errorf("page index out of range: %d", index)
	}

	d.pageOrder = append(d.pageOrder[:index], d.pageOrder[index+1:]...)
	return nil
}

// MovePage moves a page in the current session order.
func (d *Document) MovePage(fromIndex, toIndex int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.ensurePageOrderLocked(); err != nil {
		return err
	}

	if fromIndex < 0 || fromIndex >= len(d.pageOrder) {
		return fmt.Errorf("from index out of range: %d", fromIndex)
	}
	if toIndex < 0 || toIndex >= len(d.pageOrder) {
		return fmt.Errorf("to index out of range: %d", toIndex)
	}
	if fromIndex == toIndex {
		return nil
	}

	value := d.pageOrder[fromIndex]
	d.pageOrder = append(d.pageOrder[:fromIndex], d.pageOrder[fromIndex+1:]...)
	if toIndex > fromIndex {
		toIndex--
	}

	d.pageOrder = append(d.pageOrder, 0)
	copy(d.pageOrder[toIndex+1:], d.pageOrder[toIndex:])
	d.pageOrder[toIndex] = value
	return nil
}

// PageOrder returns a copy of the current session page order.
// Each value is the original source page index.
func (d *Document) PageOrder() []int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]int, len(d.pageOrder))
	copy(out, d.pageOrder)
	return out
}

// AddOutline appends an outline item under a parent path.
// parentPath is a sequence of child indexes from the root.
// Empty parentPath means append at root level.
func (d *Document) AddOutline(parentPath []int, item *Outline) error {
	if item == nil {
		return fmt.Errorf("outline item is nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.ensureMutableOutlinesLocked(); err != nil {
		return err
	}

	target, err := d.resolveOutlineChildrenByPathLocked(parentPath)
	if err != nil {
		return err
	}

	clone := cloneOutline(item)
	*target = append(*target, clone)
	d.outlinesSet = true
	return nil
}

// RemoveOutline removes an outline item by path.
// path is a sequence of child indexes from the root.
func (d *Document) RemoveOutline(path []int) error {
	if len(path) == 0 {
		return fmt.Errorf("outline path is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.ensureMutableOutlinesLocked(); err != nil {
		return err
	}

	parentPath := path[:len(path)-1]
	removeIndex := path[len(path)-1]

	target, err := d.resolveOutlineChildrenByPathLocked(parentPath)
	if err != nil {
		return err
	}

	if removeIndex < 0 || removeIndex >= len(*target) {
		return fmt.Errorf("outline index out of range: %d", removeIndex)
	}

	*target = append((*target)[:removeIndex], (*target)[removeIndex+1:]...)
	d.outlinesSet = true
	return nil
}

func (d *Document) ensurePageOrderLocked() error {
	if len(d.pageOrder) > 0 {
		return nil
	}
	if d.doc == nil {
		return fmt.Errorf("document is not initialized")
	}

	count, err := d.doc.PageCount()
	if err != nil {
		return err
	}

	d.pageOrder = make([]int, count)
	for i := 0; i < count; i++ {
		d.pageOrder[i] = i
	}
	return nil
}

func (d *Document) ensureMutableOutlinesLocked() error {
	if d.outlinesSet {
		return nil
	}

	items, err := d.loadOutlinesFromPDF()
	if err != nil {
		return err
	}

	d.outlines = cloneOutlines(items)
	d.outlinesSet = true
	return nil
}

func (d *Document) resolveOutlineChildrenByPathLocked(path []int) (*[]*Outline, error) {
	current := &d.outlines
	for _, idx := range path {
		if idx < 0 || idx >= len(*current) {
			return nil, fmt.Errorf("outline path index out of range: %d", idx)
		}
		current = &((*current)[idx].Children)
	}
	return current, nil
}

func cloneOutlines(items []*Outline) []*Outline {
	if len(items) == 0 {
		return nil
	}
	out := make([]*Outline, 0, len(items))
	for _, item := range items {
		out = append(out, cloneOutline(item))
	}
	return out
}

func cloneOutline(item *Outline) *Outline {
	if item == nil {
		return nil
	}

	clone := &Outline{
		Title:     item.Title,
		Count:     item.Count,
		PageIndex: item.PageIndex,
		Color:     item.Color,
		Dest:      item.Dest,
		Children:  cloneOutlines(item.Children),
	}

	if item.Action != nil {
		clone.Action = cloneOutlineAction(item.Action)
	}

	return clone
}

func cloneOutlineAction(action *OutlineAction) *OutlineAction {
	if action == nil {
		return nil
	}

	clone := *action
	clone.FieldNames = append([]string(nil), action.FieldNames...)
	clone.HideTargets = append([]string(nil), action.HideTargets...)
	if len(action.NextActions) > 0 {
		clone.NextActions = make([]*OutlineAction, 0, len(action.NextActions))
		for _, next := range action.NextActions {
			clone.NextActions = append(clone.NextActions, cloneOutlineAction(next))
		}
	} else {
		clone.NextActions = nil
	}
	return &clone
}
