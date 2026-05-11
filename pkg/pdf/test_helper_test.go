package pdf

import (
	"path/filepath"
	"testing"
)

func samplePDFPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(
		"..",
		"..",
		"test",
		"testdata",
		"sample-files",
		"001-trivial",
		"minimal-document.pdf",
	)
}
