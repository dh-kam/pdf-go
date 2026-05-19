package test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestFullParsingTrace(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	// Find trailer position
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	t.Logf("Trailer at position: %d", trailerIdx)

	// Create XRef table
	xrefTable := xref.NewTable(data)

	// Try to parse - this should fail
	t.Log("Calling Parse()...")
	err = xrefTable.Parse()
	if err != nil {
		t.Logf("Parse error: %v", err)
	}

	// Check if trailer was parsed
	trailer, _ := xrefTable.GetTrailer()
	if trailer != nil {
		t.Log("Trailer was parsed successfully")
	} else {
		t.Log("Trailer was NOT parsed")
	}
}
