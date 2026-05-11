package parser_test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

func TestTrailerParserParsing(t *testing.T) {
	// Simulate trailer data (as it would be after skipping "trailer")
	data := []byte("\n<<\n/Size 18\n/Info 17 0 R\n/Root 1 0 R\n>>")

	t.Logf("Input data: %q", data)

	reader := bufio.NewReader(bytes.NewReader(data))
	lexer := parser.NewLexer(reader)
	xref := &mockXRef{}
	p := parser.NewParser(lexer, xref)

	obj, err := p.ParseObject()
	if err != nil {
		require.FailNowf(t, "test failed", "ParseObject failed: %v", err)
	}

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok, "Expected Dict, got %T", obj)

	t.Logf("Parsed dict successfully")
	sizeVal := dict.Get(entity.Name("/Size"))
	t.Logf("Size value: %v (%T)", sizeVal, sizeVal)
}
