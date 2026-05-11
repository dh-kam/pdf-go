package parser_test

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugLexer(t *testing.T) {
	input := "/Type /Catalog /Pages 123"
	lexer := parser.NewLexerBytes([]byte(input))
	expected := []struct {
		typ parser.TokenType
		val string
	}{
		{parser.TokenKeyword, "Type"},
		{parser.TokenKeyword, "Catalog"},
		{parser.TokenKeyword, "Pages"},
		{parser.TokenNumber, "123"},
		{parser.TokenEOF, ""},
	}

	for i := 0; i < 5; i++ {
		token, err := lexer.NextToken()
		require.NoError(t, err)
		assert.Equal(t, expected[i].typ, token.Type, "token %d type", i)
		assert.Equal(t, expected[i].val, token.Value, "token %d value", i)
	}
}
