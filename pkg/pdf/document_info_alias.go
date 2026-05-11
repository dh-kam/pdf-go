package pdf

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// GetFilePath returns the open path when the document was opened from a file.
func (d *Document) GetFilePath() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.filePath
}

// GetErrorString returns empty string in headless library mode.
func (d *Document) GetErrorString() string {
	return ""
}

// GetPdfTempPath returns process temp directory for PDF temporary files.
func (d *Document) GetPdfTempPath() string {
	return os.TempDir()
}

// GetMediaTempPath returns process temp directory for media temporary files.
func (d *Document) GetMediaTempPath() string {
	return os.TempDir()
}

// GetProcHandle returns 0 in non-native headless library mode.
func (d *Document) GetProcHandle() int {
	return 0
}

// GetStreamedBytesForOpen returns streamed byte count for open document data.
func (d *Document) GetStreamedBytesForOpen() int64 {
	raw, err := d.rawPDFData()
	if err == nil {
		return int64(len(raw))
	}
	return d.Length()
}

// GetPDFTitle returns one best-effort title string from metadata or info dictionary.
func (d *Document) GetPDFTitle() string {
	meta := d.GetMetadata()
	if meta != nil {
		titles := meta.Title()
		if len(titles) > 0 {
			return titles[0]
		}
	}

	if d == nil || d.doc == nil {
		return ""
	}
	info := d.doc.Info()
	if info == nil {
		return ""
	}
	titleObj := info.Get(entity.Name("Title"))
	if titleObj == nil {
		return ""
	}
	if titleString, ok := titleObj.(*entity.String); ok {
		return titleString.Value()
	}
	return ""
}

// GetPDFVersion returns major PDF version number.
func (d *Document) GetPDFVersion() int {
	version := d.GetPDFVersionSL()
	if version <= 0 {
		return 0
	}
	return int(math.Floor(version))
}

// GetPDFVersionSL returns parsed PDF version from header.
func (d *Document) GetPDFVersionSL() float64 {
	raw, err := d.rawPDFData()
	if err != nil || len(raw) == 0 {
		return 0
	}
	return parsePDFVersion(raw)
}

// WriteToStream writes document bytes to the given writer.
func (d *Document) WriteToStream(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}

	raw, err := d.rawPDFData()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write pdf stream: %w", err)
	}
	return nil
}

func parsePDFVersion(raw []byte) float64 {
	marker := []byte("%PDF-")
	idx := bytes.Index(raw, marker)
	if idx < 0 {
		return 0
	}

	start := idx + len(marker)
	end := start
	for end < len(raw) {
		b := raw[end]
		if (b >= '0' && b <= '9') || b == '.' {
			end++
			continue
		}
		break
	}
	if end <= start {
		return 0
	}

	version, err := strconv.ParseFloat(string(raw[start:end]), 64)
	if err != nil {
		return 0
	}
	return version
}
