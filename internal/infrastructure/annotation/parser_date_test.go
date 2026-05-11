package annotation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePDFDate_FullFormat(t *testing.T) {
	parsed, err := parsePDFDate("D:20260216134530+09'00'")
	require.NoError(t, err)

	expected := time.Date(2026, time.February, 16, 13, 45, 30, 0, time.FixedZone("", 9*60*60))
	assert.Equal(t, expected, parsed)
}

func TestParsePDFDate_PartialDateAndZulu(t *testing.T) {
	parsed, err := parsePDFDate("D:20260216Z")
	require.NoError(t, err)

	expected := time.Date(2026, time.February, 16, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, parsed)
}

func TestParsePDFDate_Invalid(t *testing.T) {
	_, err := parsePDFDate("D:20AB")
	require.Error(t, err)
}
