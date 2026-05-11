package test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"

	"github.com/stretchr/testify/require"
)

func TestParserWithoutXRef(t *testing.T) {
	// Simple trailer dictionary data
	trailerData := []byte("\n<<\n/Size 18\n/Root 1 0 R\n>>")

	t.Logf("Trailer data: %q", trailerData)

	// Test with parser WITHOUT XRef table (nil)
	reader := bufio.NewReader(bytes.NewReader(trailerData))
	lexer := parser.NewLexer(reader)
	p := parser.NewParser(lexer, nil) // Pass nil instead of xrefTable

	obj, err := p.ParseObject()
	require.NoError(t, err)
	require.NotNil(t, obj)
}
