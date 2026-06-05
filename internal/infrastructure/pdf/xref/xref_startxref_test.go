package xref

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindStartXRefAcceptsCarriageReturnOnlyAtEOF(t *testing.T) {
	data := []byte("%PDF-1.7\r1 0 obj\r<<>>\rendobj\rstartxref\r116\r%%EOF\r")
	table := NewTable(data)

	offset, err := table.findStartXRef()

	require.NoError(t, err)
	assert.Equal(t, uint64(116), offset)
}

func TestFindStartXRefAcceptsEOFImmediatelyAfterOffset(t *testing.T) {
	data := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\nstartxref\n42")
	table := NewTable(data)

	offset, err := table.findStartXRef()

	require.NoError(t, err)
	assert.Equal(t, uint64(42), offset)
}
