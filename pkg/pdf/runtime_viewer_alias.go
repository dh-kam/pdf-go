package pdf

import (
	"reflect"
	"strings"
)

// AddListener registers one viewer listener in Java runtime.
func (d *Document) AddListener(listener interface{}) {
	if listener == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, current := range d.listeners {
		if listenersEqual(current, listener) {
			return
		}
	}
	d.listeners = append(d.listeners, listener)
}

// RemoveListener unregisters one viewer listener in Java runtime.
func (d *Document) RemoveListener(listener interface{}) {
	if listener == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for i, current := range d.listeners {
		if !listenersEqual(current, listener) {
			continue
		}
		d.listeners = append(d.listeners[:i], d.listeners[i+1:]...)
		return
	}
}

// PreparePageAndExecute updates current page then executes callback.
func (d *Document) PreparePageAndExecute(pageNumber int, runnable func()) {
	d.UpdatePage(pageNumber)
	if runnable != nil {
		runnable()
	}
}

// GetConfiguration returns minimal headless viewer configuration snapshot.
func (d *Document) GetConfiguration() map[string]interface{} {
	return map[string]interface{}{
		"book_direction": d.GetBookDirection(),
		"zoom":           d.GetZoom(),
		"width_fit":      d.IsWidthFit(),
		"height_fit":     d.IsHeightFit(),
		"night_mode":     d.NightModeGetMode(),
	}
}

// GetOpenFrom returns source marker of current open mode.
func (d *Document) GetOpenFrom() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if strings.TrimSpace(d.filePath) != "" {
		return "LocalPath"
	}
	if d.openFrom != "" {
		return d.openFrom
	}
	return "Stream"
}

// GetPDFBitmapMultiThreadRenderer returns nil in headless mode.
func (d *Document) GetPDFBitmapMultiThreadRenderer() interface{} {
	return nil
}

// GetPDFOpenFailCode returns 0 for successful open in headless mode.
func (d *Document) GetPDFOpenFailCode() int {
	return 0
}

// LookupBookDirectionFromViewerPreferences returns current book direction.
func (d *Document) LookupBookDirectionFromViewerPreferences(_ interface{}) int {
	return d.GetBookDirection()
}

// LookupPageLayoutOptions writes current layout options to the target container.
func (d *Document) LookupPageLayoutOptions(target interface{}) {
	layoutOptions := map[string]interface{}{
		"book_direction": d.GetBookDirection(),
		"width_fit":      d.IsWidthFit(),
		"height_fit":     d.IsHeightFit(),
		"zoom":           d.GetZoom(),
	}

	switch value := target.(type) {
	case *map[string]interface{}:
		if value == nil {
			return
		}
		next := make(map[string]interface{}, len(layoutOptions))
		for key, item := range layoutOptions {
			next[key] = item
		}
		*value = next
	case *int:
		if value == nil {
			return
		}
		mode := 0
		if d.IsWidthFit() {
			mode = 1
		} else if d.IsHeightFit() {
			mode = 2
		}
		*value = mode
	}
}

func listenersEqual(a interface{}, b interface{}) bool {
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if !va.IsValid() || !vb.IsValid() {
		return false
	}
	if va.Type() != vb.Type() {
		return false
	}
	if va.Comparable() && vb.Comparable() {
		return va.Interface() == vb.Interface()
	}
	return false
}
