package freetype

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRawCFFFreeTypeGoFaceResolveGlyphIndex_MapsExternalCIDToLocalGID(t *testing.T) {
	face := &rawCFFFreeTypeGoFace{
		cidToGID: map[uint32]uint32{
			3296: 22,
		},
	}

	assert.Equal(t, 22, face.resolveGlyphIndex(3296))
	assert.Equal(t, 12, face.resolveGlyphIndex(12))
	assert.Equal(t, -1, face.resolveGlyphIndex(-1))
}
