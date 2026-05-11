package parser_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

func TestLexer_NextToken_Keywords(t *testing.T) {
	input := "/Type /Catalog /Pages 123"
	lexer := parser.NewLexerBytes([]byte(input))

	tests := []struct {
		expectedValue string
		expectedType  parser.TokenType
	}{
		{"Type", parser.TokenKeyword},
		{"Catalog", parser.TokenKeyword},
		{"Pages", parser.TokenKeyword},
		{"123", parser.TokenNumber},
		{"", parser.TokenEOF},
	}

	for i, tt := range tests {
		token, err := lexer.NextToken()
		assert.NoError(t, err, "test %d: should not return error", i)
		assert.Equal(t, tt.expectedType, token.Type, "test %d: token type mismatch", i)
		assert.Equal(t, tt.expectedValue, token.Value, "test %d: token value mismatch", i)
	}
}

func TestLexer_NextToken_Numbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []parser.Token
	}{
		{
			name:  "integers",
			input: "0 1 -1 42 100",
			expected: []parser.Token{
				{Type: parser.TokenNumber, Value: "0"},
				{Type: parser.TokenNumber, Value: "1"},
				{Type: parser.TokenNumber, Value: "-1"},
				{Type: parser.TokenNumber, Value: "42"},
				{Type: parser.TokenNumber, Value: "100"},
			},
		},
		{
			name:  "real numbers",
			input: "0.0 1.5 -3.14 123.456",
			expected: []parser.Token{
				{Type: parser.TokenReal, Value: "0.0"},
				{Type: parser.TokenReal, Value: "1.5"},
				{Type: parser.TokenReal, Value: "-3.14"},
				{Type: parser.TokenReal, Value: "123.456"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))

			for i, expected := range tt.expected {
				token, err := lexer.NextToken()
				assert.NoError(t, err, "token %d: should not return error", i)
				assert.Equal(t, expected.Type, token.Type, "token %d: type mismatch", i)
				assert.Equal(t, expected.Value, token.Value, "token %d: value mismatch", i)
			}

			// Should get EOF at the end
			token, err := lexer.NextToken()
			assert.NoError(t, err)
			assert.Equal(t, parser.TokenEOF, token.Type)
		})
	}
}

func TestLexer_NextToken_Dictionary(t *testing.T) {
	input := "<< /Type /Page >>"
	lexer := parser.NewLexerBytes([]byte(input))

	expected := []parser.Token{
		{Type: parser.TokenDictStart, Value: "<<"},
		{Type: parser.TokenKeyword, Value: "Type"},
		{Type: parser.TokenKeyword, Value: "Page"},
		{Type: parser.TokenDictEnd, Value: ">>"},
	}

	for i, exp := range expected {
		token, err := lexer.NextToken()
		assert.NoError(t, err, "token %d: should not return error", i)
		assert.Equal(t, exp.Type, token.Type, "token %d: type mismatch", i)
		assert.Equal(t, exp.Value, token.Value, "token %d: value mismatch", i)
	}
}

func TestLexer_NextToken_Array(t *testing.T) {
	input := "[1 2 3]"
	lexer := parser.NewLexerBytes([]byte(input))

	expected := []parser.Token{
		{Type: parser.TokenArrayStart, Value: "["},
		{Type: parser.TokenNumber, Value: "1"},
		{Type: parser.TokenNumber, Value: "2"},
		{Type: parser.TokenNumber, Value: "3"},
		{Type: parser.TokenArrayEnd, Value: "]"},
	}

	for i, exp := range expected {
		token, err := lexer.NextToken()
		assert.NoError(t, err, "token %d: should not return error", i)
		assert.Equal(t, exp.Type, token.Type, "token %d: type mismatch", i)
		assert.Equal(t, exp.Value, token.Value, "token %d: value mismatch", i)
	}
}

func TestLexer_NextToken_String(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "(Hello World)",
			expected: "Hello World",
		},
		{
			name:     "string with escape",
			input:    "(Hello\\nWorld)",
			expected: "Hello\nWorld",
		},
		{
			name:     "string with parentheses",
			input:    "(Hello (nested) World)",
			expected: "Hello (nested) World",
		},
		{
			name:     "string with tab escape",
			input:    "(Hello\\tWorld)",
			expected: "Hello\tWorld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))

			token, err := lexer.NextToken()
			assert.NoError(t, err)
			assert.Equal(t, parser.TokenString, token.Type)
			assert.Equal(t, tt.expected, token.Value)
		})
	}
}

func TestLexer_NextToken_HexString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple hex string",
			input:    "<48656C6C6F>",
			expected: "48656C6C6F",
		},
		{
			name:     "hex string with whitespace",
			input:    "<48 65 6C 6C 6F>",
			expected: "48656C6C6F",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))

			token, err := lexer.NextToken()
			assert.NoError(t, err)
			assert.Equal(t, parser.TokenHexString, token.Type)
			assert.Equal(t, tt.expected, token.Value)
		})
	}
}

func TestLexer_NextToken_Comment(t *testing.T) {
	input := "% This is a comment\n/Type"
	lexer := parser.NewLexerBytes([]byte(input))

	// First token should be after the comment
	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token.Type)
	assert.Equal(t, "Type", token.Value)
}

func TestLexer_NextToken_NameWithHexEscape(t *testing.T) {
	input := "/Name#20With#20Spaces"
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token.Type)
	assert.Equal(t, "Name With Spaces", token.Value)
}

func TestLexer_Peek(t *testing.T) {
	input := "/Type /Catalog"
	lexer := parser.NewLexerBytes([]byte(input))

	// Peek at first token
	token1, err := lexer.Peek()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token1.Type)
	assert.Equal(t, "Type", token1.Value)

	// Peek again - should return same token
	token2, err := lexer.Peek()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token2.Type)
	assert.Equal(t, "Type", token2.Value)

	// Consume the peeked token
	token3, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token3.Type)
	assert.Equal(t, "Type", token3.Value)

	// Next token should be Catalog
	token4, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token4.Type)
	assert.Equal(t, "Catalog", token4.Value)
}

func TestLexer_NextToken_EmptyInput(t *testing.T) {
	lexer := parser.NewLexerBytes([]byte(""))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenEOF, token.Type)
}

func TestLexer_NextToken_OnlyWhitespace(t *testing.T) {
	lexer := parser.NewLexerBytes([]byte("   \n\t  "))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenEOF, token.Type)
}

func TestLexer_NextToken_MultipleComments(t *testing.T) {
	input := "% Comment 1\n% Comment 2\n/Type"
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenKeyword, token.Type)
	assert.Equal(t, "Type", token.Value)
}

func TestLexer_NextToken_LineColumn(t *testing.T) {
	input := "1\n2 3"
	lexer := parser.NewLexerBytes([]byte(input))

	// First line
	token1, _ := lexer.NextToken()
	assert.Equal(t, 0, token1.Pos)

	// Second line
	token2, _ := lexer.NextToken()
	assert.Greater(t, token2.Pos, token1.Pos)
}

func TestLexer_NextToken_InvalidNumber(t *testing.T) {
	input := "+"
	lexer := parser.NewLexerBytes([]byte(input))

	_, err := lexer.NextToken()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid number")
}

func TestLexer_NextToken_SpecialCharacters(t *testing.T) {
	input := "(Test\\r\\n\\t\\b\\f)"
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenString, token.Type)
	assert.Equal(t, "Test\r\n\t\b\f", token.Value)
}

func TestLexer_NextToken_OctalEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single digit", "(\\101)", "A"},
		{"two digits", "(\\101\\102)", "AB"},
		{"three digits", "(\\101\\102\\103)", "ABC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))

			token, err := lexer.NextToken()
			assert.NoError(t, err)
			assert.Equal(t, parser.TokenString, token.Type)
			assert.Equal(t, tt.expected, token.Value)
		})
	}
}

func TestLexer_NextToken_LineContinuation(t *testing.T) {
	input := "(Hello\\\nWorld)"
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenString, token.Type)
	assert.Equal(t, "HelloWorld", token.Value)
}

func TestLexer_NextToken_CRLFLineContinuation(t *testing.T) {
	input := "(Hello\\\r\nWorld)"
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenString, token.Type)
	assert.Equal(t, "HelloWorld", token.Value)
}

func TestLexer_NextToken_NestedParentheses(t *testing.T) {
	tests := []struct {
		name  string
		input string
		value string
	}{
		{
			name:  "one level",
			input: "(Text (with) parens)",
			value: "Text (with) parens",
		},
		{
			name:  "two levels",
			input: "(Text ((nested)) parens)",
			value: "Text ((nested)) parens",
		},
		{
			name:  "multiple",
			input: "((a)(b)(c))",
			value: "(a)(b)(c)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))

			token, err := lexer.NextToken()
			assert.NoError(t, err)
			assert.Equal(t, parser.TokenString, token.Type)
			assert.Equal(t, tt.value, token.Value)
		})
	}
}

func TestLexer_ScanHexString_InvalidCharsIgnored(t *testing.T) {
	input := "<48 65 6C 6C 6F  !@#$%^&*()>" // Non-hex chars should be ignored
	lexer := parser.NewLexerBytes([]byte(input))

	token, err := lexer.NextToken()
	assert.NoError(t, err)
	assert.Equal(t, parser.TokenHexString, token.Type)
	assert.Equal(t, "48656C6C6F", token.Value)
}

// BenchmarkLexer_SimpleKeywords benchmarks simple keyword scanning.
func BenchmarkLexer_SimpleKeywords(b *testing.B) {
	input := strings.Repeat("/Keyword ", 1000)
	data := []byte(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer := parser.NewLexerBytes(data)
		for {
			token, err := lexer.NextToken()
			if err != nil || token.Type == parser.TokenEOF {
				break
			}
		}
	}
}

// BenchmarkLexer_Dictionary benchmarks dictionary parsing.
func BenchmarkLexer_Dictionary(b *testing.B) {
	input := "<< /Type /Page /Contents 123 456 /Resources << /Font << /F1 10 >> >> >>"
	data := []byte(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer := parser.NewLexerBytes(data)
		for {
			token, err := lexer.NextToken()
			if err != nil || token.Type == parser.TokenEOF {
				break
			}
		}
	}
}
