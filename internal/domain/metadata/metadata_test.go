package metadata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetadata_Fields(t *testing.T) {
	const raw = "xmp payload"
	meta := NewMetadata(raw)

	assert.Equal(t, raw, meta.RawData())

	title := []string{"title-a", "title-b"}
	creator := []string{"creator-a"}
	subject := []string{"subject-a", "subject-b"}
	keywords := []string{"key-a", "key-b"}

	meta.SetTitle(title)
	meta.SetCreator(creator)
	meta.SetSubject(subject)
	meta.SetDescription("desc")
	meta.SetCreatorTool("tool")
	meta.SetProducer("producer")
	meta.SetKeywords(keywords)

	assert.Equal(t, title, meta.Title())
	assert.Equal(t, creator, meta.Creator())
	assert.Equal(t, subject, meta.Subject())
	assert.Equal(t, keywords, meta.Keywords())
	assert.Equal(t, "desc", meta.Description())
	assert.Equal(t, "tool", meta.CreatorTool())
	assert.Equal(t, "producer", meta.Producer())
}

func TestMetadata_Dates(t *testing.T) {
	meta := NewMetadata("")
	now := time.Date(2026, time.February, 16, 10, 0, 0, 0, time.UTC)
	other := now.Add(time.Hour)

	meta.SetCreateDate(now)
	meta.SetModifyDate(other)
	meta.SetMetadataDate(other)

	assert.True(t, meta.CreateDate().Equal(now))
	assert.True(t, meta.ModifyDate().Equal(other))
	assert.True(t, meta.MetadataDate().Equal(other))
}
