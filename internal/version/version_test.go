package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentVersion(t *testing.T) {
	assert.NotEmpty(t, Current)
	assert.Equal(t, "0.9.0-poppler24-02-0-202605.1", Current)
}
