package test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestSimulateParsingFlow(t *testing.T) {
	data, err := os.ReadFile(rootTestPDFPath(t))
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to read PDF: %v", err)
	}

	// Find startxref offset
	startxrefIdx := bytes.LastIndex(data, []byte("startxref"))
	startxrefBytes := data[startxrefIdx+10:]
	var offset uint64
	fmt.Sscanf(string(startxrefBytes), "%d", &offset)
	t.Logf("startxref offset: %d", offset)

	// Step 1: Parse XRef table (simulate parseXRefAt)
	t.Log("\n=== Step 1: Parse XRef table ===")
	xrefReader := bufio.NewReader(bytes.NewReader(data[offset:]))
	xrefLexer := parser.NewLexer(xrefReader)

	// Read "xref" token
	token, err := xrefLexer.NextToken()
	t.Logf("Token 1: Type=%s Value=%q err=%v", token.Type, token.Value, err)

	// Simulate reading some XRef entries (using Fscanf like parseTraditionalXRef)
	var startNum, count int
	n, err := fmt.Fscanf(xrefReader, "%d %d", &startNum, &count)
	t.Logf("Fscanf result: n=%d, startNum=%d, count=%d, err=%v", n, startNum, count, err)

	// Skip to next line
	line, _ := xrefReader.ReadBytes('\n')
	t.Logf("Line after Fscanf: %q", line)

	// Try to read next token with lexer
	token, err = xrefLexer.NextToken()
	t.Logf("Next token from lexer: Type=%s Value=%q err=%v", token.Type, token.Value, err)

	// Step 2: Now try to parse trailer (simulate parseTrailer)
	t.Log("\n=== Step 2: Parse trailer ===")
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	trailerData := data[trailerIdx+7:] // Skip "trailer"
	t.Logf("Trailer data (first 50 bytes): %q", trailerData[:50])

	// Create new reader for trailer (like parseTrailer does)
	trailerReader := bufio.NewReader(bytes.NewReader(trailerData))
	trailerLexer := parser.NewLexer(trailerReader)

	// Get first few tokens
	for i := 0; i < 5; i++ {
		token, err := trailerLexer.NextToken()
		if err != nil {
			t.Logf("Token %d: Error %v", i, err)
			break
		}
		t.Logf("Token %d: Type=%s Value=%q", i, token.Type, token.Value)
		if token.Type.String() == "EOF" {
			break
		}
	}

	// Now try with actual parser
	t.Log("\n=== Step 3: Parse trailer with parser ===")
	trailerReader2 := bufio.NewReader(bytes.NewReader(trailerData))
	trailerLexer2 := parser.NewLexer(trailerReader2)
	xrefTable := xref.NewTable(data) // Create XRef table for parser
	p := parser.NewParser(trailerLexer2, xrefTable)

	obj, err := p.ParseObject()
	if err != nil {
		t.Logf("ParseObject error: %v", err)
	} else {
		t.Logf("ParseObject success: %T", obj)
	}
}
