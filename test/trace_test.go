package test

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"

	"io"

	"github.com/stretchr/testify/require"
)

func TestFscanfBehavior(t *testing.T) {
	// Simulate XRef subsection line
	line := "0 18\n"
	reader := bufio.NewReader(bytes.NewReader([]byte(line)))

	var startNum, count int
	n, err := fmt.Fscanf(reader, "%d %d", &startNum, &count)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, 0, startNum)
	require.Equal(t, 18, count)

	// Check remaining bytes are exhausted for this input.
	remaining, err := reader.ReadBytes('\n')
	require.NoError(t, err)
	require.Equal(t, []byte{'\n'}, remaining)

	// Additional reads should also indicate EOF.
	more, err := reader.ReadByte()
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, byte(0), more)
}
