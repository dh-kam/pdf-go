package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuantizeMeshColorComponentUsesPopplerFixedPointTruncation(t *testing.T) {
	value := (42.9 / 65536.0)

	assert.Equal(t, 42.0/65536.0, quantizeMeshColorComponent(value))
	assert.Equal(t, 1.0, quantizeMeshColorComponent(1))
}
