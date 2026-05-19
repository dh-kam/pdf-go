package test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestDetailedTracing(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	// Find trailer position
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	trailerData := data[trailerIdx+7:]

	t.Logf("Trailer data: %q", trailerData[:100])

	// Create XRef table
	xrefTable := xref.NewTable(data)

	// Parse trailer using the same approach as parseTrailer
	t.Log("\n=== Parsing trailer ===")
	reader := bytes.NewReader(trailerData)

	// Create lexer
	lexer := parser.NewLexer(reader)

	// Create parser
	p := parser.NewParser(lexer, xrefTable)

	// Parse object
	obj, err := p.ParseObject()
	if err != nil {
		t.Logf("ParseObject error: %v", err)
		return
	}

	t.Logf("ParseObject success: %T", obj)

	// Check if it's a dictionary
	dict, ok := obj.(*entity.Dict)
	if !ok {
		t.Logf("Object is not a dictionary: %T", obj)
		return
	}

	t.Logf("Dict keys:")
	for _, key := range dict.Keys() {
		val := dict.Get(key)
		t.Logf("  %s: %v (%T)", key, val, val)

		// Check if Root
		if string(key) == "Root" {
			if ref, ok := val.(entity.Ref); ok {
				t.Logf("    Root IS a Ref: %d %d", ref.Num(), ref.Gen())
			} else {
				t.Logf("    Root is NOT a Ref!")
			}
		}
	}
}
