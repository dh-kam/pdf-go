// Package entity defines PDF document entities.
package entity

import (
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/metadata"
)

// Document represents a PDF document.
type Document struct {
	xref           XRef
	catalog        *Dict
	info           *Dict
	metadataDict   *Dict // Raw metadata stream dictionary
	parsedMetadata *metadata.Metadata
	fileSize       int64
}

type catalogRecoveryXRef interface {
	RebuildCatalogByObjectScan() error
	GetCatalog() (*Dict, error)
}

type pageScanRecoveryXRef interface {
	RecoverPageRefsByObjectScan() ([]Ref, error)
}

type linearizedFallbackXRef interface {
	LinearizedPageCount() (int, bool)
}

// NewDocument creates a new Document.
func NewDocument(xref XRef) *Document {
	return &Document{
		xref: xref,
	}
}

// XRef returns the cross-reference table.
func (d *Document) XRef() XRef {
	return d.xref
}

// Catalog returns the document catalog.
func (d *Document) Catalog() *Dict {
	return d.catalog
}

// SetCatalog sets the document catalog.
func (d *Document) SetCatalog(catalog *Dict) {
	d.catalog = catalog
}

// Info returns the document info dictionary.
func (d *Document) Info() *Dict {
	return d.info
}

// SetInfo sets the document info dictionary.
func (d *Document) SetInfo(info *Dict) {
	d.info = info
}

// Metadata returns the document metadata dictionary (raw stream).
func (d *Document) Metadata() *Dict {
	return d.metadataDict
}

// SetMetadata sets the document metadata dictionary (raw stream).
func (d *Document) SetMetadata(metadata *Dict) {
	d.metadataDict = metadata
}

// ParsedMetadata returns the parsed XMP metadata.
func (d *Document) ParsedMetadata() *metadata.Metadata {
	return d.parsedMetadata
}

// SetParsedMetadata sets the parsed XMP metadata.
func (d *Document) SetParsedMetadata(meta *metadata.Metadata) {
	d.parsedMetadata = meta
}

// SetFileSize sets the file size.
func (d *Document) SetFileSize(size int64) {
	d.fileSize = size
}

// FileSize returns the file size.
func (d *Document) FileSize() int64 {
	return d.fileSize
}

func (d *Document) tryRecoverCatalog() bool {
	recoverable, ok := d.xref.(catalogRecoveryXRef)
	if !ok {
		return false
	}
	if err := recoverable.RebuildCatalogByObjectScan(); err != nil {
		return false
	}
	catalog, err := recoverable.GetCatalog()
	if err != nil || catalog == nil {
		return false
	}
	d.catalog = catalog
	return true
}

func (d *Document) tryRecoverPagesByScan(minCount int) (*Dict, bool) {
	if minCount > 2 {
		return nil, false
	}

	recoverable, ok := d.xref.(pageScanRecoveryXRef)
	if !ok {
		return nil, false
	}

	pageRefs, err := recoverable.RecoverPageRefsByObjectScan()
	if err != nil || len(pageRefs) == 0 || len(pageRefs) <= minCount+1 {
		return nil, false
	}

	kids := make([]Object, len(pageRefs))
	for i, ref := range pageRefs {
		kids[i] = ref
	}

	pagesDict := NewDict()
	pagesDict.Set(Name("/Type"), Name("Pages"))
	pagesDict.Set(Name("/Count"), NewInteger(int64(len(pageRefs))))
	pagesDict.Set(Name("/Kids"), NewArray(kids...))

	if d.catalog != nil {
		d.catalog.Set(Name("/Pages"), pagesDict)
	}

	return pagesDict, true
}

func (d *Document) tryRecoverSuspiciousPositivePagesCount(pagesDict *Dict) (*Dict, bool) {
	if pagesDict == nil {
		return nil, false
	}

	countVal := pagesDict.Get(Name("/Count"))
	if countVal == nil {
		countVal = pagesDict.Get(Name("Count"))
	}

	countObj, ok := countVal.(*Integer)
	if !ok {
		return nil, false
	}

	declaredCount := int(countObj.Value())
	if declaredCount <= 0 {
		return nil, false
	}

	fallback, ok := d.xref.(linearizedFallbackXRef)
	if ok {
		if linearizedCount, hasLinearized := fallback.LinearizedPageCount(); hasLinearized && linearizedCount > declaredCount {
			return nil, false
		}
	}

	return d.tryRecoverPagesByScan(declaredCount)
}

func (d *Document) blankPageFallbackCount() (int, bool) {
	fallback, ok := d.xref.(linearizedFallbackXRef)
	if !ok {
		return 0, false
	}

	linearizedCount, ok := fallback.LinearizedPageCount()
	if !ok || linearizedCount <= 0 {
		return 0, false
	}

	pagesDict, err := d.resolvePagesDict(false)
	if err != nil {
		return linearizedCount, true
	}

	currentCount := d.pageCountFromDict(pagesDict)
	if currentCount >= linearizedCount {
		return 0, false
	}

	return linearizedCount, true
}

func (d *Document) pageCountFromDict(pagesDict *Dict) int {
	if pagesDict == nil {
		return 0
	}

	countVal := pagesDict.Get(Name("/Count"))
	if countVal == nil {
		countVal = pagesDict.Get(Name("Count"))
	}
	if countObj, ok := countVal.(*Integer); ok && countObj.Value() > 0 {
		return int(countObj.Value())
	}

	count, err := d.countLeafPages(pagesDict)
	if err != nil {
		return 0
	}

	return count
}

func (d *Document) newBlankPlaceholderPage(index int) *Page {
	box := NewArray(
		NewInteger(0),
		NewInteger(0),
		NewInteger(0),
		NewInteger(0),
	)

	pageDict := NewDict()
	pageDict.Set(Name("/Type"), Name("Page"))
	pageDict.Set(Name("/MediaBox"), box)
	pageDict.Set(Name("/CropBox"), box.Clone())

	return NewPage(d, pageDict, Ref{}, index)
}

func (d *Document) resolvePagesDict(allowRecover bool) (*Dict, error) {
	if d.catalog == nil {
		return nil, ErrCatalogNotFound
	}

	pagesRef := d.catalog.Get(Name("/Pages"))
	if pagesRef == nil {
		if allowRecover && d.tryRecoverCatalog() {
			return d.resolvePagesDict(false)
		}
		return nil, ErrPagesNotFound
	}

	switch v := pagesRef.(type) {
	case Ref:
		obj, err := d.xref.Fetch(v)
		if err != nil {
			if allowRecover && d.tryRecoverCatalog() {
				return d.resolvePagesDict(false)
			}
			return nil, err
		}
		pagesDict, ok := obj.(*Dict)
		if !ok {
			return nil, ErrInvalidPagesType
		}
		if allowRecover {
			if recovered, ok := d.tryRecoverSuspiciousPositivePagesCount(pagesDict); ok {
				return recovered, nil
			}
		}
		return pagesDict, nil
	case *Dict:
		if allowRecover {
			if recovered, ok := d.tryRecoverSuspiciousPositivePagesCount(v); ok {
				return recovered, nil
			}
		}
		return v, nil
	default:
		return nil, ErrInvalidPagesType
	}
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() (int, error) {
	if fallbackCount, ok := d.blankPageFallbackCount(); ok {
		return fallbackCount, nil
	}

	pagesDict, err := d.resolvePagesDict(true)
	if err != nil {
		return 0, err
	}

	// Get /Count from Pages dictionary
	countVal := pagesDict.Get(Name("/Count"))
	if countVal == nil {
		// Count may not be present, need to count leaf pages
		count, err := d.countLeafPages(pagesDict)
		if err != nil {
			return 0, err
		}
		// Page-scan recovery can mutate XRef caches; only use it when the
		// resolved tree yields no usable count.
		if count <= 0 {
			if _, ok := d.tryRecoverPagesByScan(count); ok {
				return d.PageCount()
			}
		}
		return count, nil
	}

	count, ok := countVal.(*Integer)
	if !ok {
		return 0, ErrInvalidCountType
	}

	resolvedCount := int(count.Value())
	if resolvedCount <= 0 {
		if _, ok := d.tryRecoverPagesByScan(resolvedCount); ok {
			return d.PageCount()
		}
	}

	return resolvedCount, nil
}

// countLeafPages counts leaf pages by traversing the page tree.
func (d *Document) countLeafPages(pagesDict *Dict) (int, error) {
	kidsVal := pagesDict.Get(Name("/Kids"))
	if kidsVal == nil {
		return 0, ErrKidsNotFound
	}

	kids, ok := kidsVal.(*Array)
	if !ok {
		return 0, ErrInvalidKidsType
	}

	count := 0
	for i := 0; i < kids.Len(); i++ {
		kidRef := kids.Get(i)
		if kidRef == nil {
			continue
		}

		var kidDict *Dict
		switch v := kidRef.(type) {
		case Ref:
			obj, err := d.xref.Fetch(v)
			if err != nil {
				continue
			}
			var ok bool
			kidDict, ok = obj.(*Dict)
			if !ok {
				continue
			}
		case *Dict:
			kidDict = v
		default:
			continue
		}

		// Check if this is a Page or Pages node
		typeVal := kidDict.Get(Name("/Type"))
		if typeVal == nil {
			continue
		}

		typeName, ok := typeVal.(Name)
		if !ok {
			continue
		}

		if typeName == "Page" {
			count++
		} else if typeName == "Pages" {
			subCount, err := d.countLeafPages(kidDict)
			if err != nil {
				continue
			}
			count += subCount
		}
	}

	return count, nil
}

// Document errors.
var (
	ErrCatalogNotFound  = &PDFError{Op: "document_catalog", Err: ErrNotFound, Type: ErrTypeInvalid}
	ErrPagesNotFound    = &PDFError{Op: "document_pages", Err: ErrNotFound, Type: ErrTypeInvalid}
	ErrKidsNotFound     = &PDFError{Op: "document_kids", Err: ErrNotFound, Type: ErrTypeInvalid}
	ErrInvalidPagesType = &PDFError{Op: "document_pages", Err: ErrInvalid, Type: ErrTypeInvalid}
	ErrInvalidKidsType  = &PDFError{Op: "document_kids", Err: ErrInvalid, Type: ErrTypeInvalid}
	ErrInvalidCountType = &PDFError{Op: "document_count", Err: ErrInvalid, Type: ErrTypeInvalid}
)

// PDFError is a base error type for PDF operations.
type PDFError struct {
	Err  error
	Op   string
	Type ErrorType
}

// Error returns the error message.
func (e *PDFError) Error() string {
	if e.Err == nil {
		return e.Op
	}
	return e.Op + ": " + e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *PDFError) Unwrap() error {
	return e.Err
}

// Error types.
var (
	ErrNotFound = &basicError{"not found"}
	ErrInvalid  = &basicError{"invalid"}
)

type basicError struct {
	msg string
}

// Error returns the error message.
func (e *basicError) Error() string {
	return e.msg
}

// ErrorType represents the category of an error.
type ErrorType int

const (
	// ErrTypeInvalid indicates invalid input or state.
	ErrTypeInvalid ErrorType = iota
	// ErrTypeNotFound indicates a requested entity was not found.
	ErrTypeNotFound
	// ErrTypeEncryption indicates encryption/decryption failures.
	ErrTypeEncryption
	// ErrTypeFont indicates font processing failures.
	ErrTypeFont
	// ErrTypeRendering indicates rendering pipeline failures.
	ErrTypeRendering
)

// Close closes the document and releases resources.
func (d *Document) Close() error {
	// If we have a Closer, close it
	if closer, ok := d.xref.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// GetPage returns the page at the specified index (0-based).
func (d *Document) GetPage(index int) (*Page, error) {
	if fallbackCount, ok := d.blankPageFallbackCount(); ok {
		if index < 0 || index >= fallbackCount {
			return nil, ErrPageNotFound
		}
		return d.newBlankPlaceholderPage(index), nil
	}

	pagesDict, err := d.resolvePagesDict(true)
	if err != nil {
		return nil, err
	}

	// Traverse page tree to find the page at index
	pageRef, err := d.findPageAtIndex(pagesDict, index)
	if err != nil {
		return nil, err
	}

	// Fetch the page dictionary
	var pageDict *Dict
	var pageRefVal Ref

	switch v := pageRef.(type) {
	case Ref:
		pageRefVal = v
		obj, err := d.xref.Fetch(v)
		if err != nil {
			return nil, err
		}
		var ok bool
		pageDict, ok = obj.(*Dict)
		if !ok {
			return nil, ErrInvalidPageType
		}
	case *Dict:
		pageDict = v
		pageRefVal = Ref{} // Zero ref for inline dictionaries
	default:
		return nil, ErrInvalidPageType
	}

	// Create Page object
	page := NewPage(d, pageDict, pageRefVal, index)
	return page, nil
}

// findPageAtIndex finds the page reference at the given index by traversing the page tree.
func (d *Document) findPageAtIndex(pagesDict *Dict, index int) (Object, error) {
	// Get Kids array
	kidsVal := pagesDict.Get(Name("/Kids"))
	if kidsVal == nil {
		return nil, ErrKidsNotFound
	}

	kids, ok := kidsVal.(*Array)
	if !ok {
		return nil, ErrInvalidKidsType
	}

	// Track current index as we traverse
	currentIndex := 0

	// First, check if this is a leaf node (Pages node with direct Page children)
	for i := 0; i < kids.Len(); i++ {
		kidRef := kids.Get(i)
		if kidRef == nil {
			continue
		}

		// Fetch kid dictionary
		var kidDict *Dict
		switch v := kidRef.(type) {
		case Ref:
			obj, err := d.xref.Fetch(v)
			if err != nil {
				continue
			}
			var ok bool
			kidDict, ok = obj.(*Dict)
			if !ok {
				continue
			}
		case *Dict:
			kidDict = v
		default:
			continue
		}

		// Check type
		typeVal := kidDict.Get(Name("/Type"))
		if typeVal == nil {
			// Might be a leaf page without explicit Type
			typeVal = kidDict.Get(Name("/Contents"))
		}

		if typeVal != nil {
			typeName, ok := typeVal.(Name)
			if ok && typeName == "Pages" {
				// This is a Pages node - check Count
				countVal := kidDict.Get(Name("Count"))
				if countVal != nil {
					if count, ok := countVal.(*Integer); ok {
						pagesInSubtree := int(count.Value())
						// Check if the desired page is in this subtree
						if index < currentIndex+pagesInSubtree {
							// Recurse into this subtree
							subIndex := index - currentIndex
							return d.findPageAtIndex(kidDict, subIndex)
						}
						// Skip this subtree
						currentIndex += pagesInSubtree
						continue
					}
				}
				// No Count - recurse
				subRef, err := d.findPageAtIndex(kidDict, index-currentIndex)
				if err == nil {
					return subRef, nil
				}
				// Continue to next kid
				continue
			}
		}

		// This is a Page node or leaf node
		if currentIndex == index {
			return kidRef, nil
		}
		currentIndex++
	}

	return nil, ErrPageNotFound
}

// ErrPageNotFound is returned when a page index is out of range.
var ErrPageNotFound = &PDFError{Op: "document_page", Err: ErrNotFound, Type: ErrTypeInvalid}

// ErrInvalidPageType is returned when a page is not a dictionary.
var ErrInvalidPageType = &PDFError{Op: "document_page", Err: ErrInvalid, Type: ErrTypeInvalid}
