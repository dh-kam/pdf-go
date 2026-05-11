package parser_test

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugParen(t *testing.T) {
	input := "(Hello\\(World\\))"
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, "Hello(World)", token.Value, "escaped parentheses should be unescaped")
	assert.Equal(t, parser.TokenString, token.Type)
}
