// Package repository defines interfaces for PDF data access.
package repository

import (
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// XRef provides access to PDF objects through cross-reference table.
type XRef interface {
	// Fetch retrieves the object at the given reference.
	Fetch(ref entity.Ref) (entity.Object, error)

	// FetchCached retrieves a cached object without parsing.
	FetchCached(ref entity.Ref) (entity.Object, bool)

	// Cache stores an object in the cache.
	Cache(ref entity.Ref, obj entity.Object)

	// GetCatalog returns the catalog dictionary.
	GetCatalog() (*entity.Dict, error)

	// GetTrailer returns the trailer dictionary.
	GetTrailer() (*entity.Dict, error)

	// GetNumObjects returns the number of objects in the XRef table.
	GetNumObjects() int
}

// XRefEntry represents a single entry in the cross-reference table.
type XRefEntry struct {
	Object             entity.Object
	Offset             uint64
	Type               EntryType
	ObjectStreamNumber uint32
	Generation         uint16
	ObjectStreamIndex  uint16
	Free               bool
}

// EntryType represents the type of XRef entry.
type EntryType int

const (
	// EntryTypeFree represents a free entry.
	EntryTypeFree EntryType = iota
	// EntryTypeUncompressed represents an uncompressed object (has byte offset).
	EntryTypeUncompressed
	// EntryTypeCompressed represents an object stream compressed entry.
	EntryTypeCompressed
	// EntryTypeNull represents a null/invalid entry.
	EntryTypeNull
)
