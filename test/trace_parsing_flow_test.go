package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestTraceParsingFlow(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	xrefTable := xref.NewTable(data)

	t.Log("=== Before Parse() ===")
	trailer1, _ := xrefTable.GetTrailer()
	if trailer1 != nil {
		t.Log("Trailer already exists!")
	} else {
		t.Log("Trailer is nil (expected)")
	}
	catalog1, _ := xrefTable.GetCatalog()
	if catalog1 != nil {
		t.Log("Catalog already exists!")
	} else {
		t.Log("Catalog is nil (expected)")
	}

	t.Log("\n=== Calling Parse() ===")
	err = xrefTable.Parse()
	if err != nil {
		t.Logf("Parse error: %v", err)
	}

	t.Log("\n=== After Parse() ===")
	trailer2, _ := xrefTable.GetTrailer()
	if trailer2 != nil {
		t.Log("Trailer was parsed")
		rootVal := trailer2.Get("Root")
		t.Logf("Root value: %v (%T)", rootVal, rootVal)
	} else {
		t.Log("Trailer is nil!")
	}
	catalog2, _ := xrefTable.GetCatalog()
	if catalog2 != nil {
		t.Log("Catalog was resolved")
	} else {
		t.Log("Catalog is nil!")
	}
}
