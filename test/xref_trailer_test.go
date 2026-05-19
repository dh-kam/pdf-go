package test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestTrailerWithActualData(t *testing.T) {
	// Read actual PDF
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	// Find trailer position
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	if trailerIdx == -1 {
		t.Log("trailer keyword not found; attempting parser path for xref stream PDF")
		xrefTable := xref.NewTable(data)
		err = xrefTable.Parse()
		require.NoError(t, err)
		return
	}
	t.Logf("Found 'trailer' at position %d", trailerIdx)

	// Show bytes around trailer
	start := trailerIdx - 10
	if start < 0 {
		start = 0
	}
	end := trailerIdx + 50
	if end > len(data) {
		end = len(data)
	}
	t.Logf("Bytes around trailer (pos %d): %q", start, data[start:end])
	t.Logf("Hex: % x", data[start:end])

	// Show bytes after "trailer"
	afterTrailer := data[trailerIdx+7:]
	sampleLen := 100
	if sampleLen > len(afterTrailer) {
		sampleLen = len(afterTrailer)
	}
	t.Logf("Bytes after 'trailer' (first %d): %q", sampleLen, afterTrailer[:sampleLen])
	t.Logf("Hex: % x", afterTrailer[:sampleLen])

	// Now try to parse
	xrefTable := xref.NewTable(data)
	err = xrefTable.Parse()
	t.Logf("Parse result: %v", err)
}
