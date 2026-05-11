package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestParseObjectAt(t *testing.T) {
	data, err := os.ReadFile("/workspace/pdf-reader/go-pdf/test.pdf")
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	xrefTable := xref.NewTable(data)

	// Parse the PDF to populate XRef entries
	err = xrefTable.Parse()
	t.Logf("Parse error: %v", err)

	// Try to fetch object 1 (should be the catalog)
	ref := entity.NewRef(1, 0)
	obj, err := xrefTable.Fetch(ref)
	if err != nil {
		t.Logf("Fetch error: %v", err)
	} else {
		t.Logf("Fetch success: %T", obj)
		if dict, ok := obj.(*entity.Dict); ok {
			t.Logf("Object is a dictionary")
			typeVal := dict.Get(entity.Name("Type"))
			t.Logf("Type: %v (%T)", typeVal, typeVal)
		}
	}
}
