// Package pdf provides PDF loading use cases.
package pdf

import (
	"fmt"
	"io"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	metadataparser "github.com/dh-kam/pdf-go/internal/infrastructure/metadata"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

// Open opens a PDF document from a file path.
func Open(path string) (*entity.Document, error) {
	return OpenWithPassword(path, "")
}

// OpenWithPassword opens a PDF document from a file path with a password.
func OpenWithPassword(path, password string) (*entity.Document, error) {
	// Read entire file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return OpenBytesWithPassword(data, password)
}

// OpenBytes opens a PDF document from bytes.
func OpenBytes(data []byte) (*entity.Document, error) {
	return OpenBytesWithPassword(data, "")
}

// OpenBytesWithPassword opens a PDF document from bytes with a password.
func OpenBytesWithPassword(data []byte, password string) (*entity.Document, error) {
	// Create XRef table
	xrefTable := xref.NewTable(data)

	// Parse the PDF
	if err := xrefTable.Parse(); err != nil {
		return nil, fmt.Errorf("parse pdf: %w", err)
	}

	// Handle encryption
	if xrefTable.IsEncrypted() {
		if err := xrefTable.ParseEncryptionDict(password); err != nil {
			return nil, fmt.Errorf("parse encryption: %w", err)
		}

		// Check if authentication was successful
		if !xrefTable.IsAuthenticated() {
			return nil, fmt.Errorf("invalid password or unsupported encryption")
		}
	}

	// Create document
	doc := entity.NewDocument(xrefTable)

	// Set catalog from parsed trailer
	catalog, catalogErr := xrefTable.GetCatalog()
	if catalogErr == nil && catalog != nil {
		doc.SetCatalog(catalog)
	}

	// Resolve /Info dictionary from trailer.
	trailer, trailerErr := xrefTable.GetTrailer()
	if trailerErr == nil && trailer != nil {
		if infoObj := trailer.Get(entity.Name("/Info")); infoObj != nil {
			if infoDict, ok := infoObj.(*entity.Dict); ok {
				doc.SetInfo(infoDict)
			}
		}
	}

	// Set file size
	doc.SetFileSize(int64(len(data)))

	// Parse metadata if present
	if err := parseMetadata(doc); err != nil {
		// Non-fatal error - just log and continue
		// In a real implementation, you might want to log this
		_ = err
	}

	return doc, nil
}

// OpenReader opens a PDF document from an io.Reader.
func OpenReader(r io.Reader) (*entity.Document, error) {
	return OpenReaderWithPassword(r, "")
}

// OpenReaderWithPassword opens a PDF document from an io.Reader with a password.
func OpenReaderWithPassword(r io.Reader, password string) (*entity.Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	return OpenBytesWithPassword(data, password)
}

// parseMetadata extracts and parses XMP metadata from the catalog.
func parseMetadata(doc *entity.Document) error {
	catalog := doc.Catalog()
	if catalog == nil {
		return nil // No catalog, no metadata
	}

	// Get /Metadata reference from catalog
	metadataRef := catalog.Get(entity.Name("/Metadata"))
	if metadataRef == nil {
		return nil // No metadata stream
	}

	var (
		streamData []byte
		err        error
	)

	switch v := metadataRef.(type) {
	case entity.Ref:
		// Get the xref table to extract stream data
		xrefTable, ok := doc.XRef().(*xref.Table)
		if !ok {
			return fmt.Errorf("xref is not a Table")
		}
		streamData, err = xrefTable.GetStreamData(v)
		if err != nil {
			return fmt.Errorf("get stream data: %w", err)
		}
	case *entity.Stream:
		infraStream := pdfstream.NewFromEntity(v)
		streamData, err = infraStream.Decode()
		if err != nil {
			return fmt.Errorf("decode metadata stream: %w", err)
		}
	default:
		// Unsupported metadata object type.
		return nil
	}

	// Parse XMP metadata
	parser := metadataparser.NewMetadataParser(string(streamData))
	meta, err := parser.Parse()
	if err != nil {
		return fmt.Errorf("parse metadata: %w", err)
	}

	// Save parsed metadata
	doc.SetParsedMetadata(meta)

	return nil
}
