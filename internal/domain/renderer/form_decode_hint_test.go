package renderer

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestFormStreamDecodeSizeHintOnlyAppliesToLargeForms(t *testing.T) {
	t.Run("nil stream", func(t *testing.T) {
		assert.Zero(t, formStreamDecodeSizeHint(nil))
	})

	t.Run("tiny form", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Subtype"), entity.Name("Form"))
		stream := entity.NewStream(dict, make([]byte, 32*1024-1))
		assert.Zero(t, formStreamDecodeSizeHint(stream))
	})

	t.Run("medium form", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Subtype"), entity.Name("Form"))
		stream := entity.NewStream(dict, make([]byte, 32*1024))
		assert.Equal(t, 192*1024, formStreamDecodeSizeHint(stream))
	})

	t.Run("large form", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Subtype"), entity.Name("Form"))
		stream := entity.NewStream(dict, make([]byte, 128*1024))
		assert.Equal(t, 2*1024*1024, formStreamDecodeSizeHint(stream))
	})

	t.Run("large image is excluded", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.Name("Subtype"), entity.Name("Image"))
		stream := entity.NewStream(dict, make([]byte, 256*1024))
		assert.Zero(t, formStreamDecodeSizeHint(stream))
	})
}
