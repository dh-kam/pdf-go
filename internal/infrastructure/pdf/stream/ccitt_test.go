package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCCITTFaxDecoder_Group3TwoDFallbackUses1DPath(t *testing.T) {
	data := []byte{0x00}

	oneD := NewCCITTFaxDecoder(0, 1728, 1, false)
	_, oneDErr := oneD.Decode(data)

	twoD := NewCCITTFaxDecoder(1, 1728, 1, false)
	_, twoDErr := twoD.Decode(data)

	require.Error(t, twoDErr)
	assert.NotContains(t, twoDErr.Error(), "not implemented")

	if oneDErr != nil {
		assert.Contains(t, oneDErr.Error(), "ccitt")
		assert.Contains(t, twoDErr.Error(), "ccitt")
	}
}
