package registry

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type mockFont struct{}

func (m *mockFont) CharCodeToGlyph(code uint32) (uint32, error) {
	return code, nil
}

func (m *mockFont) GlyphName(glyph uint32) string {
	return "mock"
}

func (m *mockFont) GetGlyphWidth(glyph uint32) (float64, error) {
	return 500, nil
}

func (m *mockFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 10, 10
}

func (m *mockFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return &entity.GlyphPath{}, nil
}

func (m *mockFont) IsCIDFont() bool    { return false }
func (m *mockFont) IsSymbolic() bool   { return false }
func (m *mockFont) UnitsPerEm() uint16 { return 1000 }
func (m *mockFont) Name() string       { return "mock" }

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.Equal(t, 0, r.Count())
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	font := &mockFont{}

	r.Register("Helvetica", font)

	got, ok := r.Get("Helvetica")
	assert.True(t, ok)
	assert.Same(t, font, got)
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register("Times", &mockFont{})
	r.Register("Courier", &mockFont{})
	r.Register("Times", &mockFont{})

	list := r.List()
	sort.Strings(list)
	assert.Equal(t, []string{"Courier", "Times"}, list)
}

func TestGlobalRegistryRoundTrip(t *testing.T) {
	key := "mock-font-registry-test-key"
	font := &mockFont{}

	RegisterGlobal(key, font)
	got, ok := GetGlobal(key)
	require.True(t, ok)
	assert.Same(t, font, got)

	names := ListGlobal()
	assert.Contains(t, names, key)
}
