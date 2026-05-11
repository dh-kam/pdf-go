package font_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/registry"
)

// mockFont is a simple mock for testing
type mockFont struct {
	name string
}

func (m *mockFont) CharCodeToGlyph(code uint32) (uint32, error) {
	return code, nil
}

func (m *mockFont) GlyphName(glyph uint32) string {
	return "test"
}

func (m *mockFont) GetGlyphWidth(glyph uint32) (float64, error) {
	return 500.0, nil
}

func (m *mockFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}

func (m *mockFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return &entity.GlyphPath{}, nil
}

func (m *mockFont) IsCIDFont() bool {
	return false
}

func (m *mockFont) IsSymbolic() bool {
	return false
}

func (m *mockFont) UnitsPerEm() uint16 {
	return 1000
}

func (m *mockFont) Name() string {
	return m.name
}

func TestRegistry_NewRegistry(t *testing.T) {
	r := registry.NewRegistry()

	assert.NotNil(t, r)
	assert.Equal(t, 0, r.Count())
}

func TestRegistry_Register(t *testing.T) {
	r := registry.NewRegistry()
	font := &mockFont{name: "TestFont"}

	r.Register("TestFont", font)

	assert.Equal(t, 1, r.Count())
}

func TestRegistry_Get(t *testing.T) {
	r := registry.NewRegistry()
	font := &mockFont{name: "TestFont"}

	t.Run("existing font", func(t *testing.T) {
		r.Register("TestFont", font)

		retrieved, ok := r.Get("TestFont")
		assert.True(t, ok)
		assert.Equal(t, font, retrieved)
	})

	t.Run("non-existing font", func(t *testing.T) {
		_, ok := r.Get("NonExistent")
		assert.False(t, ok)
	})
}

func TestRegistry_List(t *testing.T) {
	r := registry.NewRegistry()

	font1 := &mockFont{name: "Font1"}
	font2 := &mockFont{name: "Font2"}
	font3 := &mockFont{name: "Font3"}

	r.Register("Font1", font1)
	r.Register("Font2", font2)
	r.Register("Font3", font3)

	names := r.List()
	assert.Equal(t, 3, len(names))

	// Check that all names are present (order may vary)
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	assert.True(t, nameMap["Font1"])
	assert.True(t, nameMap["Font2"])
	assert.True(t, nameMap["Font3"])
}

func TestRegistry_Count(t *testing.T) {
	r := registry.NewRegistry()

	assert.Equal(t, 0, r.Count())

	r.Register("Font1", &mockFont{name: "Font1"})
	assert.Equal(t, 1, r.Count())

	r.Register("Font2", &mockFont{name: "Font2"})
	assert.Equal(t, 2, r.Count())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := registry.NewRegistry()

	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(n int) {
			font := &mockFont{name: "ConcurrentFont"}
			r.Register("Font"+string(rune(n)), font)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			r.Get("Font1")
			r.List()
			r.Count()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should not crash
	assert.True(t, true)
}

func TestGlobalRegistry(t *testing.T) {
	// Clear global registry before test
	font := &mockFont{name: "GlobalFont"}

	t.Run("register and get", func(t *testing.T) {
		registry.RegisterGlobal("GlobalFont", font)

		retrieved, ok := registry.GetGlobal("GlobalFont")
		assert.True(t, ok)
		assert.Equal(t, font, retrieved)
	})

	t.Run("list global fonts", func(t *testing.T) {
		names := registry.ListGlobal()
		assert.Contains(t, names, "GlobalFont")
	})
}
