package parser_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

// mockXRef is a mock XRef implementation for testing.
type mockXRef struct{}

func (m *mockXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	// Return a simple string for testing
	return entity.NewString("fetched"), nil
}

func TestParser_ParseObject_Boolean(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"true", "true", true},
		{"false", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))
			p := parser.NewParser(lexer, nil)

			obj, err := p.ParseObject()
			require.NoError(t, err)

			b, ok := obj.(*entity.Boolean)
			require.True(t, ok, "expected Boolean type")
			assert.Equal(t, tt.want, b.Value())
		})
	}
}

func TestParser_ParseObject_Null(t *testing.T) {
	lexer := parser.NewLexerBytes([]byte("null"))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	_, ok := obj.(*entity.Null)
	assert.True(t, ok, "expected Null type")
}

func TestParser_ParseObject_Integer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{"zero", "0", 0},
		{"positive", "42", 42},
		{"negative", "-42", -42},
		{"large", "123456789", 123456789},
		{"positive sign", "+123", 123},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))
			p := parser.NewParser(lexer, nil)

			obj, err := p.ParseObject()
			require.NoError(t, err)

			n, ok := obj.(*entity.Integer)
			require.True(t, ok, "expected Integer type")
			assert.Equal(t, tt.want, n.Value())
		})
	}
}

func TestParser_ParseObject_Real(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{"simple", "3.14", 3.14},
		{"negative", "-2.5", -2.5},
		{"positive sign", "+1.5", 1.5},
		{"zero", "0.0", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))
			p := parser.NewParser(lexer, nil)

			obj, err := p.ParseObject()
			require.NoError(t, err)

			r, ok := obj.(*entity.Real)
			require.True(t, ok, "expected Real type")
			assert.InDelta(t, tt.want, r.Value(), 0.001)
		})
	}
}

func TestParser_ParseObject_String(t *testing.T) {
	input := "(Hello World)"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	s, ok := obj.(*entity.String)
	require.True(t, ok, "expected String type")
	assert.Equal(t, "Hello World", s.Value())
}

func TestParser_ParseObject_HexString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "<48656C6C6F>", "Hello"},
		{"with whitespace", "<48 65 6C 6C 6F>", "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))
			p := parser.NewParser(lexer, nil)

			obj, err := p.ParseObject()
			require.NoError(t, err)

			s, ok := obj.(*entity.String)
			require.True(t, ok, "expected String type")
			assert.Equal(t, tt.expected, s.Value())
			assert.True(t, s.IsHex())
		})
	}
}

func TestParser_ParseObject_Name(t *testing.T) {
	input := "/Type"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	n, ok := obj.(entity.Name)
	require.True(t, ok, "expected Name type")
	assert.Equal(t, "Type", n.Value())
}

func TestParser_ParseObject_Array(t *testing.T) {
	input := "[1 2 3]"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	arr, ok := obj.(*entity.Array)
	require.True(t, ok, "expected Array type")
	assert.Equal(t, 3, arr.Len())

	// Check elements
	for i := 0; i < 3; i++ {
		elem := arr.Get(i)
		n, ok := elem.(*entity.Integer)
		require.True(t, ok, "element %d should be Integer", i)
		assert.Equal(t, int64(i+1), n.Value())
	}
}

func TestParser_ParseObject_EmptyArray(t *testing.T) {
	input := "[]"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	arr, ok := obj.(*entity.Array)
	require.True(t, ok, "expected Array type")
	assert.Equal(t, 0, arr.Len())
}

func TestParser_ParseObject_NestedArray(t *testing.T) {
	input := "[1 [2 3] 4]"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	arr, ok := obj.(*entity.Array)
	require.True(t, ok, "expected Array type")
	assert.Equal(t, 3, arr.Len())

	// Middle element should be an array
	middle := arr.Get(1)
	nested, ok := middle.(*entity.Array)
	require.True(t, ok, "middle element should be Array")
	assert.Equal(t, 2, nested.Len())
}

func TestParser_ParseObject_Dictionary(t *testing.T) {
	input := "<< /Type /Page /Count 5 >>"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok, "expected Dict type")
	assert.Equal(t, 2, dict.Len())

	// Check Type
	typeVal := dict.Get(entity.Name("/Type"))
	assert.Equal(t, entity.Name("Page"), typeVal)

	// Check Count
	countVal := dict.Get(entity.Name("/Count"))
	c, ok := countVal.(*entity.Integer)
	require.True(t, ok, "Count should be Integer")
	assert.Equal(t, int64(5), c.Value())
}

func TestParser_ParseObject_EmptyDictionary(t *testing.T) {
	input := "<< >>"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok, "expected Dict type")
	assert.Equal(t, 0, dict.Len())
}

func TestParser_ParseObject_NestedDictionary(t *testing.T) {
	input := "<< /Parent << /Type /Pages >> /Count 5 >>"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok, "expected Dict type")
	assert.Equal(t, 2, dict.Len())

	// Check nested dictionary
	parentVal := dict.Get(entity.Name("/Parent"))
	parent, ok := parentVal.(*entity.Dict)
	require.True(t, ok, "Parent should be Dict")
	assert.Equal(t, 1, parent.Len())
}

func TestParser_ParseObject_DictionaryWithArray(t *testing.T) {
	input := "<< /Kids [10 20 30] >>"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok, "expected Dict type")

	kidsVal := dict.Get(entity.Name("/Kids"))
	kids, ok := kidsVal.(*entity.Array)
	require.True(t, ok, "Kids should be Array")
	assert.Equal(t, 3, kids.Len())
}

func TestParser_ParseIndirectReference(t *testing.T) {
	input := "10 0 R"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	ref, err := p.ParseIndirectReference()
	require.NoError(t, err)

	assert.Equal(t, uint32(10), ref.Num())
	assert.Equal(t, uint16(0), ref.Gen())
}

func TestParser_ParseIndirectReference_WithGeneration(t *testing.T) {
	input := "5 2 R"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	ref, err := p.ParseIndirectReference()
	require.NoError(t, err)

	assert.Equal(t, uint32(5), ref.Num())
	assert.Equal(t, uint16(2), ref.Gen())
}

func TestParser_ParseIndirectReference_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing R", "10 0"},
		{"invalid object number", "abc 0 R"},
		{"invalid generation", "10 abc R"},
		{"wrong terminator", "10 0 X"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))
			p := parser.NewParser(lexer, nil)

			_, err := p.ParseIndirectReference()
			assert.Error(t, err)
		})
	}
}

func TestParser_ParseObject_StringWithEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"newline", "(Hello\\nWorld)", "Hello\nWorld"},
		{"tab", "(Hello\\tWorld)", "Hello\tWorld"},
		{"carriage return", "(Hello\\rWorld)", "Hello\rWorld"},
		{"backslash", "(Hello\\\\World)", "Hello\\World"},
		{"paren", "(Hello\\(World\\))", "Hello(World)"}, // Escaped ( and ) within string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := parser.NewLexerBytes([]byte(tt.input))
			p := parser.NewParser(lexer, nil)

			obj, err := p.ParseObject()
			require.NoError(t, err)

			s, ok := obj.(*entity.String)
			require.True(t, ok, "expected String type")
			assert.Equal(t, tt.expected, s.Value())
		})
	}
}

func TestParser_ParseObject_Multiple(t *testing.T) {
	input := "123 456 true false null /Type"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	// Parse 123
	obj, err := p.ParseObject()
	require.NoError(t, err)
	n, ok := obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(123), n.Value())

	// Parse 456
	obj, err = p.ParseObject()
	require.NoError(t, err)
	n, ok = obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(456), n.Value())

	// Parse true
	obj, err = p.ParseObject()
	require.NoError(t, err)
	b, ok := obj.(*entity.Boolean)
	require.True(t, ok)
	assert.True(t, b.Value())

	// Parse false
	obj, err = p.ParseObject()
	require.NoError(t, err)
	b, ok = obj.(*entity.Boolean)
	require.True(t, ok)
	assert.False(t, b.Value())

	// Parse null
	obj, err = p.ParseObject()
	require.NoError(t, err)
	_, ok = obj.(*entity.Null)
	assert.True(t, ok)

	// Parse /Type
	obj, err = p.ParseObject()
	require.NoError(t, err)
	name, ok := obj.(entity.Name)
	require.True(t, ok)
	assert.Equal(t, "Type", name.Value())
}

func TestParser_ParseObject_DictionaryWithMultipleTypes(t *testing.T) {
	input := "<< /Type /Catalog /Pages 3 /Version 1.4 /Hidden true /Visible null >>"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok)

	// Check Type
	typeVal := dict.Get(entity.Name("/Type"))
	assert.Equal(t, entity.Name("Catalog"), typeVal)

	// Check Pages
	pagesVal := dict.Get(entity.Name("/Pages"))
	pages, ok := pagesVal.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(3), pages.Value())

	// Check Version
	verVal := dict.Get(entity.Name("/Version"))
	ver, ok := verVal.(*entity.Real)
	require.True(t, ok)
	assert.InDelta(t, 1.4, ver.Value(), 0.01)

	// Check Hidden
	hiddenVal := dict.Get(entity.Name("/Hidden"))
	hidden, ok := hiddenVal.(*entity.Boolean)
	require.True(t, ok)
	assert.True(t, hidden.Value())

	// Check Visible
	visibleVal := dict.Get(entity.Name("/Visible"))
	_, ok = visibleVal.(*entity.Null)
	assert.True(t, ok)
}

func TestParser_ParseObject_ArrayWithMixedTypes(t *testing.T) {
	input := "[123 3.14 true false null /Type (string)]"
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	arr, ok := obj.(*entity.Array)
	require.True(t, ok)
	assert.Equal(t, 7, arr.Len())

	// Check first element (integer)
	elem := arr.Get(0)
	n, ok := elem.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(123), n.Value())

	// Check second element (real)
	elem = arr.Get(1)
	r, ok := elem.(*entity.Real)
	require.True(t, ok)
	assert.InDelta(t, 3.14, r.Value(), 0.01)
}

func TestParser_ParseObject_HexStringOddLength(t *testing.T) {
	input := "<48656C6C6F>" // "Hello" in hex, even length
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	s, ok := obj.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "Hello", s.Value())
}

func TestParser_ParseObject_HexStringOddLengthPadding(t *testing.T) {
	input := "<48656C6C6F4>" // Odd length, should be padded with 0
	lexer := parser.NewLexerBytes([]byte(input))
	p := parser.NewParser(lexer, nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	s, ok := obj.(*entity.String)
	require.True(t, ok)
	// '4' becomes '40' which is '@'
	assert.Equal(t, "Hello@", s.Value())
}
