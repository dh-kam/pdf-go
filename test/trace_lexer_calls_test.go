package test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceLexerCalls(t *testing.T) {
	trailerData := []byte("\n<<\n/Size 18\n>>")

	t.Logf("Trailer data: %q", trailerData)

	// Create lexer
	reader := bufio.NewReader(bytes.NewReader(trailerData))
	lexer := parser.NewLexer(reader)

	// Simulate what the parser does
	t.Log("\n=== Simulating parser flow ===")

	// Call 1: ParseObject calls NextToken() to get first token
	t.Log("Call 1: ParseObject.NextToken()")
	token1, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenDictStart, token1.Type)
	assert.Equal(t, "<<", token1.Value)

	// Call 2: Since token is <<, parseDict is called
	t.Log("Call 2: parseDict.NextToken() (should be Size)")
	token2, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token2.Type)
	assert.Equal(t, "Size", token2.Value)

	// Call 3: parseDict calls ParseObject to get value
	t.Log("Call 3: parseDict.ParseObject() for Size value")
	token3, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenNumber, token3.Type)
	assert.Equal(t, "18", token3.Value)

	// Now let's try with the actual parser
	t.Log("\n=== Using actual parser ===")
	reader2 := bufio.NewReader(bytes.NewReader(trailerData))
	lexer2 := parser.NewLexer(reader2)
	p := parser.NewParser(lexer2, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)
	require.NotNil(t, obj)
}
