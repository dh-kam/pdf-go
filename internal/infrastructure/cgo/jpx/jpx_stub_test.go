//go:build nojpx

package jpx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAvailable(t *testing.T) {
	assert.False(t, IsAvailable())
}

func TestDecodeReturnsUnavailableError(t *testing.T) {
	_, err := Decode(nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not available"))
}

func TestDecodeConfigReturnsUnavailableError(t *testing.T) {
	_, err := DecodeConfig(nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not available"))
}
