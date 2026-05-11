package parser_test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrailerParsing(t *testing.T) {
	// Simulate trailer data (as it would be after skipping "trailer")
	data := []byte("\n<<\n/Size 18\n/Info 17 0 R\n/Root 1 0 R\n>>")

	t.Logf("Input data: %q", data)
	t.Logf("Input bytes: % x", data)

	reader := bufio.NewReader(bytes.NewReader(data))
	lexer := parser.NewLexer(reader)

	for i := 0; i < 10; i++ {
		token, err := lexer.NextToken()
		require.NoError(t, err)
		t.Logf("Token %d: Type=%s Value=%q", i, token.Type, token.Value)
		if token.Type == parser.TokenEOF {
			assert.Equal(t, parser.TokenEOF, token.Type)
			break
		}
	}
}
