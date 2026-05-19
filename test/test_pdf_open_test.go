package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestPDFOpenDetailed(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	xrefTable := xref.NewTable(data)

	t.Log("Calling Parse()...")
	err = xrefTable.Parse()
	if err != nil {
		t.Logf("Parse error: %v", err)
	}

	// Check if trailer was parsed
	trailer, _ := xrefTable.GetTrailer()
	if trailer != nil {
		t.Log("Trailer was parsed successfully")
		rootVal := trailer.Get("Root")
		t.Logf("Root value: %v (%T)", rootVal, rootVal)
	} else {
		t.Log("Trailer was NOT parsed")
	}

	// Check if catalog was resolved
	catalog, _ := xrefTable.GetCatalog()
	if catalog != nil {
		t.Log("Catalog was resolved successfully")
	} else {
		t.Log("Catalog was NOT resolved")
	}
}
