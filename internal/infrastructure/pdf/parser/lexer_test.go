package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexer_SkipsWhitespaceAndComments(t *testing.T) {
	lexer := NewLexer(strings.NewReader("  \t\n% comment\n123"))
	token, err := lexer.NextToken()
	require.NoError(t, err)
	require.Equal(t, TokenNumber, token.Type)
	assert.Equal(t, "123", token.Value)
	assert.Greater(t, token.Pos, 0)
}

func TestLexer_PeekReturnsBufferedToken(t *testing.T) {
	lexer := NewLexer(strings.NewReader("10 20"))

	peeked, err := lexer.Peek()
	require.NoError(t, err)
	assert.Equal(t, TokenNumber, peeked.Type)
	assert.Equal(t, "10", peeked.Value)

	token, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, peeked.Type, token.Type)
	assert.Equal(t, peeked.Value, token.Value)
}

func TestLexer_NextToken_Structures(t *testing.T) {
	lexer := NewLexerBytes([]byte("<< /Type /Page >> [1 (a\\n) <4142> ]"))

	token, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenDictStart, token.Type)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenKeyword, token.Type)
	assert.Equal(t, "Type", token.Value)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenKeyword, token.Type)
	assert.Equal(t, "Page", token.Value)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenDictEnd, token.Type)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenArrayStart, token.Type)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenNumber, token.Type)
	assert.Equal(t, "1", token.Value)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenString, token.Type)
	assert.Equal(t, "a\n", token.Value)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenHexString, token.Type)
	assert.Equal(t, "4142", token.Value)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenArrayEnd, token.Type)
}

func TestLexer_UnsupportedNumberAndEOF(t *testing.T) {
	lexer := NewLexerBytes([]byte("+"))
	token, err := lexer.NextToken()
	require.Error(t, err)
	assert.Equal(t, "invalid number at position 0", err.Error())

	assert.Equal(t, Token{}, token)
}

func TestLexer_LeadingDotRealNumber(t *testing.T) {
	lexer := NewLexerBytes([]byte(".75 -.5"))

	token, err := lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenReal, token.Type)
	assert.Equal(t, ".75", token.Value)

	token, err = lexer.NextToken()
	require.NoError(t, err)
	assert.Equal(t, TokenReal, token.Type)
	assert.Equal(t, "-.5", token.Value)
}

func TestLexer_Helpers(t *testing.T) {
	assert.True(t, isHexDigit('A'))
	assert.True(t, isHexDigit('f'))
	assert.False(t, isHexDigit('Z'))
	assert.Equal(t, byte(0x0B), hexDecode('0', 'B'))
	assert.Equal(t, byte(13), octalDecode([]byte("15")))
}
