package test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

func TestTraceXRefEntries(t *testing.T) {
	data, err := os.ReadFile("/workspace/pdf-reader/go-pdf/test.pdf")
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
	t.Logf("Token 2 (start): %s %q", token2.Type, token2.Value)

	token3, _ := lexer.NextToken()
	t.Logf("Token 3 (count): %s %q", token3.Type, token3.Value)

	// Read some XRef entries
	for i := 0; i < 5; i++ {
		token, _ := lexer.NextToken()
		t.Logf("Entry %d token 1 (offset): %s %q", i, token.Type, token.Value)
		token, _ = lexer.NextToken()
		t.Logf("Entry %d token 2 (gen): %s %q", i, token.Type, token.Value)
		token, _ = lexer.NextToken()
		t.Logf("Entry %d token 3 (type): %s %q", i, token.Type, token.Value)
	}

	// Check what's next (should be trailer)
	t.Log("\n=== Checking what comes after XRef entries ===")
	for i := 0; i < 10; i++ {
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
