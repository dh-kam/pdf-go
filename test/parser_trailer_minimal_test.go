package test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserTrailerMinimal(t *testing.T) {
	// Simple trailer dictionary data
	trailerData := []byte("\n<<\n/Size 18\n/Info 17 0 R\n/Root 1 0 R\n>>")

	t.Logf("Trailer data: %q", trailerData)

	// Create a minimal XRef table (empty, just for interface)
	data := []byte("%PDF-1.3\n")
	xrefTable := xref.NewTable(data)

	// Test with lexer directly
	t.Log("\n=== Test 1: Lexer only ===")
	reader1 := bufio.NewReader(bytes.NewReader(trailerData))
	lexer1 := parser.NewLexer(reader1)

	for i := 0; i < 5; i++ {
		token, err := lexer1.NextToken()
		require.NoError(t, err)
		assert.NotEqual(t, parser.TokenEOF, token.Type, "token %d should not be EOF", i)
		t.Logf("Token %d: %s %q", i, token.Type, token.Value)
		if token.Type == parser.TokenDictEnd {
			break
		}
	}

	// Test with parser
	t.Log("\n=== Test 2: With parser ===")
	reader2 := bufio.NewReader(bytes.NewReader(trailerData))
	lexer2 := parser.NewLexer(reader2)
	p := parser.NewParser(lexer2, xrefTable)

	obj, err := p.ParseObject()
	require.NoError(t, err)
	require.NotNil(t, obj)
}
