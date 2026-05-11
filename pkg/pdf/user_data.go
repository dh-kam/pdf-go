package pdf

import "fmt"

// PutUserData stores binary user data by namespace and key.
// Data is kept for the document lifetime in memory.
func (d *Document) PutUserData(namespace, key string, data []byte) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	scope := d.userData[namespace]
	if scope == nil {
		scope = make(map[string][]byte)
		d.userData[namespace] = scope
	}

	copied := make([]byte, len(data))
	copy(copied, data)
	scope[key] = copied
	return nil
}

// PutUserDataString stores string user data by namespace and key.
func (d *Document) PutUserDataString(namespace, key, value string) error {
	return d.PutUserData(namespace, key, []byte(value))
}

// GetUserData returns a copy of binary user data.
func (d *Document) GetUserData(namespace, key string) ([]byte, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	scope := d.userData[namespace]
	if scope == nil {
		return nil, false
	}

	value, ok := scope[key]
	if !ok {
		return nil, false
	}

	copied := make([]byte, len(value))
	copy(copied, value)
	return copied, true
}

// GetUserDataString returns string user data.
func (d *Document) GetUserDataString(namespace, key string) (string, bool) {
	data, ok := d.GetUserData(namespace, key)
	if !ok {
		return "", false
	}
	return string(data), true
}

// DeleteUserData removes user data for namespace and key.
func (d *Document) DeleteUserData(namespace, key string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	scope := d.userData[namespace]
	if scope == nil {
		return
	}
	delete(scope, key)
	if len(scope) == 0 {
		delete(d.userData, namespace)
	}
}

// PutUserDataForPage stores page-scoped string user data (0-based page index).
func (d *Document) PutUserDataForPage(pageIndex int, key, value string) error {
	if pageIndex < 0 {
		return fmt.Errorf("invalid page index: %d", pageIndex)
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	entry := d.pageUserData[pageIndex]
	if entry == nil {
		entry = make(map[string]string)
		d.pageUserData[pageIndex] = entry
	}
	entry[key] = value
	return nil
}

// GetUserDataForPage returns page-scoped string user data (0-based page index).
func (d *Document) GetUserDataForPage(pageIndex int, key string) (string, bool) {
	if pageIndex < 0 || key == "" {
		return "", false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	entry := d.pageUserData[pageIndex]
	if entry == nil {
		return "", false
	}

	value, ok := entry[key]
	return value, ok
}
