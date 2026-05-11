package parser_test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrailerParserDebug(t *testing.T) {
	// Simulate trailer data (as it would be after skipping "trailer")
	data := []byte("\n<<\n/Size 18\n/Info 17 0 R\n/Root 1 0 R\n>>")

	t.Logf("Input data: %q", data)

	reader := bufio.NewReader(bytes.NewReader(data))
	lexer := parser.NewLexer(reader)

	// Manually trace through parsing
	token1, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenDictStart, token1.Type)
	t.Logf("First token (should be <<): Type=%s Value=%q err=%v", token1.Type, token1.Value, err)

	token2, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token2.Type)
	assert.Equal(t, "Size", token2.Value)
	t.Logf("Second token (should be Size): Type=%s Value=%q err=%v", token2.Type, token2.Value, err)

	token3, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenNumber, token3.Type)
	assert.Equal(t, "18", token3.Value)
	t.Logf("Third token (should be 18): Type=%s Value=%q err=%v", token3.Type, token3.Value, err)

	// Now try with parser
	t.Log("\n--- Now with parser ---")
	reader2 := bufio.NewReader(bytes.NewReader(data))
	lexer2 := parser.NewLexer(reader2)
	xref := &mockXRef{}
	p := parser.NewParser(lexer2, xref)

	// Get first token to see what parser sees
	firstToken, _ := lexer2.Peek()
	t.Logf("Parser.Peek() returns: Type=%s Value=%q", firstToken.Type, firstToken.Value)

	// But ParseObject should use NextToken, not Peek
	obj, err := p.ParseObject()
	require.NoError(t, err)
	require.NotNil(t, obj)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok, "expected Dict, got %T", obj)
	t.Logf("Parsed dict successfully")
	sizeVal := dict.Get(entity.Name("/Size"))
	require.NotNil(t, sizeVal)

	peek, err := lexer2.Peek()
	require.NoError(t, err)
	assert.Equal(t, parser.TokenEOF, peek.Type)
}
