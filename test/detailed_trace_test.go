package test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestDetailedParsingFlow(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	// Find startxref offset
	startxrefIdx := bytes.LastIndex(data, []byte("startxref"))
	startxrefBytes := data[startxrefIdx+10:]
	var offset uint64
	fmt.Sscanf(string(startxrefBytes), "%d", &offset)
	t.Logf("startxref offset: %d", offset)

	// Step 1: Parse XRef table using the approach from parseXRefAt
	t.Log("\n=== Step 1: Parse XRef table ===")
	reader1 := bufio.NewReader(bytes.NewReader(data[offset:]))
	lexer1 := parser.NewLexer(reader1)

	// Check if it's "xref" or XRef stream
	token1, _ := lexer1.NextToken()
	t.Logf("First token: %s %q", token1.Type, token1.Value)

	if token1.Type == parser.TokenKeyword && token1.Value == "xref" {
		t.Log("This is a traditional XRef table")
		// Read subsection header
		startToken, _ := lexer1.NextToken()
		countToken, _ := lexer1.NextToken()
		t.Logf("Subsection: start=%s count=%s", startToken.Value, countToken.Value)

		// Read a few entries to simulate parseTraditionalXRef
		for i := 0; i < 3; i++ {
			offsetToken, _ := lexer1.NextToken()
			genToken, _ := lexer1.NextToken()
			typeToken, _ := lexer1.NextToken()
			t.Logf("Entry %d: offset=%s gen=%s type=%s", i, offsetToken.Value, genToken.Value, typeToken.Value)
		}
	}

	// Step 2: Parse trailer (like parseTrailer does)
	t.Log("\n=== Step 2: Parse trailer ===")
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	trailerData := data[trailerIdx+7:] // Skip "trailer"
	t.Logf("Trailer data (first 50 bytes): %q", trailerData[:50])

	// Create XRef table (like parseTrailer does)
	xrefTable := xref.NewTable(data)

	// Parse trailer dictionary
	reader2 := bufio.NewReader(bytes.NewReader(trailerData))
	lexer2 := parser.NewLexer(reader2)
	p := parser.NewParser(lexer2, xrefTable)

	t.Log("Calling p.ParseObject()...")
	obj, err := p.ParseObject()
	if err != nil {
		t.Logf("ParseObject error: %v", err)

		// Let's see what the lexer returns
		t.Log("\n=== Checking lexer tokens ===")
		reader3 := bufio.NewReader(bytes.NewReader(trailerData))
		lexer3 := parser.NewLexer(reader3)
		for i := 0; i < 5; i++ {
			token, err := lexer3.NextToken()
			if err != nil {
				t.Logf("Token %d: Error %v", i, err)
				break
			}
			t.Logf("Token %d: %s %q", i, token.Type, token.Value)
			if token.Type.String() == "EOF" {
				break
			}
		}
	} else {
		t.Logf("ParseObject success: %T", obj)
		if dict, ok := obj.(*entity.Dict); ok {
			t.Logf("Dict keys: %d", dict.Len())
		}
	}
}
