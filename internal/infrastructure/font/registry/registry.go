// Package registry provides font registry for managing font instances.
package registry

import (
	"sync"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// Registry manages font instances.
type Registry struct {
	fonts map[string]entity.Font
	mu    sync.RWMutex
}

// NewRegistry creates a new font registry.
func NewRegistry() *Registry {
	return &Registry{
		fonts: make(map[string]entity.Font),
	}
}

// Register adds a font to the registry.
func (r *Registry) Register(name string, font entity.Font) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fonts[name] = font
}

// Get retrieves a font by name.
func (r *Registry) Get(name string) (entity.Font, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	font, ok := r.fonts[name]
	return font, ok
}

// List returns all registered font names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.fonts))
	for name := range r.fonts {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered fonts.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.fonts)
}

// global registry instance
var global = NewRegistry()

// RegisterGlobal registers a font in the global registry.
func RegisterGlobal(name string, font entity.Font) {
	global.Register(name, font)
}

// GetGlobal retrieves a font from the global registry.
func GetGlobal(name string) (entity.Font, bool) {
	return global.Get(name)
}

// ListGlobal returns all font names from the global registry.
func ListGlobal() []string {
	return global.List()
}
