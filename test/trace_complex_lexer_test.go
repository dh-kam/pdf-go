package test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceComplexTrailer(t *testing.T) {
	// Complex trailer with indirect references
	trailerData := []byte("\n<<\n/Size 18\n/Info 17 0 R\n/Root 1 0 R\n>>")

	t.Logf("Trailer data: %q", trailerData)

	// Create lexer
	reader := bufio.NewReader(bytes.NewReader(trailerData))
	lexer := parser.NewLexer(reader)

	// Simulate what the parser does
	t.Log("\n=== Simulating parser flow ===")

	// Manual token tracing to verify lexer sequence
	token1, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenDictStart, token1.Type)

	token2, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token2.Type)
	assert.Equal(t, "Size", token2.Value)

	token3, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenNumber, token3.Type)
	assert.Equal(t, "18", token3.Value)

	token4, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token4.Type)
	assert.Equal(t, "Info", token4.Value)

	token5, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenNumber, token5.Type)
	assert.Equal(t, "17", token5.Value)

	token6, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenNumber, token6.Type)
	assert.Equal(t, "0", token6.Value)

	token7, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token7.Type)
	assert.Equal(t, "R", token7.Value)

	// Now let's try with the actual parser
	t.Log("\n=== Using actual parser ===")
	reader2 := bufio.NewReader(bytes.NewReader(trailerData))
	lexer2 := parser.NewLexer(reader2)
	p := parser.NewParser(lexer2, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)
	require.NotNil(t, obj)
}
