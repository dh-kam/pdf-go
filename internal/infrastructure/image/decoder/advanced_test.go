package decoder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectFormat_SignatureMatch(t *testing.T) {
	signatures := append(JPEG2000Signatures(), JBIG2Signatures()...)

	jp2Data := []byte{
		0x00, 0x00, 0x00, 0x0C,
		0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
	}
	jb2Data := []byte{
		0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A,
	}
	jpcData := []byte{0xFF, 0x4F, 0xFF, 0x51}

	assert.Equal(t, "jp2", DetectFormat(jp2Data, signatures))
	assert.Equal(t, "jb2", DetectFormat(jb2Data, signatures))
	assert.Equal(t, "jpc", DetectFormat(jpcData, signatures))
}

func TestDetectFormat_NoMatch(t *testing.T) {
	signatures := append(JPEG2000Signatures(), JBIG2Signatures()...)

	data := []byte{0x00, 0x11, 0x22, 0x33}
	assert.Empty(t, DetectFormat(data, signatures))
}
