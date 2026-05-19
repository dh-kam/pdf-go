package test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

func TestTraceXRefParsing(t *testing.T) {
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

	// Parse XRef table using the lexer
	t.Log("\n=== Parsing XRef table ===")
	lexer := parser.NewLexerBytes(data[offset:])

	// Read "xref" token
	token, _ := lexer.NextToken()
	t.Logf("Token 1: %s %q", token.Type, token.Value)

	// Read subsection header
	token2, _ := lexer.NextToken()
	token3, _ := lexer.NextToken()
	count := 0
	if _, err := fmt.Sscanf(token3.Value, "%d", &count); err == nil {
		t.Logf("Subsection: start=%s count=%d", token2.Value, count)
	}

	// Read all XRef entries
	for i := 0; i < count; i++ {
		offsetToken, _ := lexer.NextToken()
		genToken, _ := lexer.NextToken()
		typeToken, _ := lexer.NextToken()
		t.Logf("Entry %d: offset=%s gen=%s type=%s", i, offsetToken.Value, genToken.Value, typeToken.Value)
	}

	// Check what's next (should be trailer)
	t.Log("\n=== Checking what comes after XRef entries ===")
	for i := 0; i < 15; i++ {
		token, err := lexer.NextToken()
		if err != nil {
			t.Logf("Token %d: Error %v", i, err)
			break
		}
		t.Logf("Token %d: %s %q", i, token.Type, token.Value)
		if token.Type.String() == "EOF" {
			break
		}
	}
}
