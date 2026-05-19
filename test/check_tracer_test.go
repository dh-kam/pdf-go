package test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestCheckTracer(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	xrefTable := xref.NewTable(data)

	// Call Parse() to trigger the full parsing flow
	t.Log("Calling Parse()...")
	err = xrefTable.Parse()
	t.Logf("Parse error: %v", err)

	// Check trailer multiple times
	for i := 0; i < 3; i++ {
		t.Logf("\n=== Check %d ===", i+1)
		trailer, _ := xrefTable.GetTrailer()
		if trailer == nil {
			t.Log("Trailer is nil!")
			continue
		}

		rootVal := trailer.Get(entity.Name("Root"))
		t.Logf("Root value: %v (%T)", rootVal, rootVal)

		if ref, ok := rootVal.(entity.Ref); ok {
			t.Logf("  Root IS a Ref: %d %d", ref.Num(), ref.Gen())
		} else {
			t.Logf("  Root is NOT a Ref!")

			// Check if it's an integer
			if intVal, ok := rootVal.(*entity.Integer); ok {
				t.Logf("  Root is an Integer: %d", intVal.Value())
			}
		}
	}

	// Also check the raw trailer data
	t.Log("\n=== Checking raw trailer data ===")
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	trailerData := data[trailerIdx+7:]
	t.Logf("Raw trailer data (first 100 bytes): %q", trailerData[:100])

	// Count how many times "trailer" appears
	t.Log("\n=== Counting 'trailer' occurrences ===")
	count := bytes.Count(data, []byte("trailer"))
	t.Logf("'trailer' appears %d times in the PDF", count)
}
