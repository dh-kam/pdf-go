package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestParser_HasBufferedObject(t *testing.T) {
	p := NewParser(NewLexerBytes([]byte("1 2")), nil)

	assert.False(t, p.HasBufferedObject())
}

func TestParser_ParseObject_Primitives(t *testing.T) {
	p := NewParser(NewLexerBytes([]byte("true false null /Type 42 0 R 12.5 (abc) <4142>")), nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)
	boolObj, ok := obj.(*entity.Boolean)
	require.True(t, ok)
	assert.True(t, boolObj.Value())

	obj, err = p.ParseObject()
	require.NoError(t, err)
	boolObj, ok = obj.(*entity.Boolean)
	require.True(t, ok)
	assert.False(t, boolObj.Value())

	obj, err = p.ParseObject()
	require.NoError(t, err)
	_, ok = obj.(*entity.Null)
	assert.True(t, ok)

	obj, err = p.ParseObject()
	require.NoError(t, err)
	nameObj, ok := obj.(entity.Name)
	require.True(t, ok)
	assert.Equal(t, "Type", nameObj.Value())

	obj, err = p.ParseObject()
	require.NoError(t, err)
	refObj, ok := obj.(entity.Ref)
	require.True(t, ok)
	ref := refObj
	assert.EqualValues(t, 42, ref.Num())
	assert.EqualValues(t, 0, ref.Gen())
	require.Equal(t, entity.NewRef(42, 0), ref)

	obj, err = p.ParseObject()
	require.NoError(t, err)
	realObj, ok := obj.(*entity.Real)
	require.True(t, ok)
	assert.InDelta(t, 12.5, realObj.Value(), 0.0001)

	obj, err = p.ParseObject()
	require.NoError(t, err)
	stringObj, ok := obj.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "abc", stringObj.Value())
	assert.False(t, stringObj.IsHex())

	obj, err = p.ParseObject()
	require.NoError(t, err)
	hexObj, ok := obj.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "AB", hexObj.Value())
	assert.True(t, hexObj.IsHex())

	_, err = p.ParseObject()
	assert.Error(t, err)
}

func TestParser_ParseObject_ReferenceFallbackAndBuffered(t *testing.T) {
	p := NewParser(NewLexerBytes([]byte("12 34")), nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)
	first, ok := obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(12), first.Value())
	assert.True(t, p.HasBufferedObject())

	obj, err = p.ParseObject()
	require.NoError(t, err)
	second, ok := obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(34), second.Value())
}

func TestParser_ParseObjectWithSpan_ReferenceFallbackAndBuffered(t *testing.T) {
	data := []byte("12 34")
	p := NewParser(NewLexerBytes(data), nil)

	obj, start, end, err := p.ParseObjectWithSpan()
	require.NoError(t, err)
	first, ok := obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(12), first.Value())
	assert.Equal(t, "12", string(data[start:end]))
	assert.True(t, p.HasBufferedObject())

	obj, start, end, err = p.ParseObjectWithSpan()
	require.NoError(t, err)
	second, ok := obj.(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(34), second.Value())
	assert.Equal(t, "34", string(data[start:end]))
}

func TestParser_ParseObjectWithSpan_StringPreservesRawLexeme(t *testing.T) {
	data := []byte("(a\\n)")
	p := NewParser(NewLexerBytes(data), nil)

	obj, start, end, err := p.ParseObjectWithSpan()
	require.NoError(t, err)

	stringObj, ok := obj.(*entity.String)
	require.True(t, ok)
	assert.Equal(t, "a\n", stringObj.Value())
	assert.Equal(t, "(a\\n)", string(data[start:end]))
}

func TestParser_ParseObject_DictAndArray(t *testing.T) {
	data := []byte("<< /Type /Page /Count 2 /Name /Test /Children [1 2 3] >>")
	p := NewParser(NewLexerBytes(data), nil)

	obj, err := p.ParseObject()
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok)
	assert.Equal(t, entity.NewName("Page"), dict.Get(entity.NewName("/Type")))
	assert.Equal(t, int64(2), dict.Get(entity.NewName("/Count")).(*entity.Integer).Value())

	children := dict.Get(entity.NewName("/Children")).(*entity.Array)
	require.Len(t, children.Items(), 3)
	assert.Equal(t, int64(1), children.Items()[0].(*entity.Integer).Value())
	assert.Equal(t, int64(2), children.Items()[1].(*entity.Integer).Value())
	assert.Equal(t, int64(3), children.Items()[2].(*entity.Integer).Value())
}

func TestParser_ParseObject_MalformedStructure(t *testing.T) {
	t.Run("invalid_integer", func(t *testing.T) {
		_, err := parseInteger("12x")
		require.Error(t, err)
	})

	t.Run("invalid_real", func(t *testing.T) {
		_, err := parseReal("1x")
		require.Error(t, err)
	})

	t.Run("incomplete_array", func(t *testing.T) {
		p := NewParser(NewLexerBytes([]byte("[1 2 3")), nil)
		obj, err := p.ParseObject()
		require.Error(t, err)
		assert.Nil(t, obj)
	})

	t.Run("incomplete_dict", func(t *testing.T) {
		p := NewParser(NewLexerBytes([]byte("<< /Type /Page")), nil)
		obj, err := p.ParseObject()
		require.Error(t, err)
		assert.Nil(t, obj)
	})
}

func TestParseInteger_And_ParseReal(t *testing.T) {
	value, err := parseInteger("123")
	require.NoError(t, err)
	assert.Equal(t, int64(123), value)

	value, err = parseInteger("-45")
	require.NoError(t, err)
	assert.Equal(t, int64(-45), value)

	value, err = parseInteger("+7")
	require.NoError(t, err)
	assert.Equal(t, int64(7), value)

	valueReal, err := parseReal("12.34")
	require.NoError(t, err)
	assert.InDelta(t, 12.34, valueReal, 0.0001)

	valueReal, err = parseReal("-0.5")
	require.NoError(t, err)
	assert.InDelta(t, -0.5, valueReal, 0.0001)
}

func TestParseIntegerReal_HexStringErrors(t *testing.T) {
	_, err := parseInteger("12x")
	require.Error(t, err)

	_, err = parseReal("1x")
	require.Error(t, err)

	value, err := decodeHexString("4142")
	require.NoError(t, err)
	assert.Equal(t, "AB", value)

	value, err = decodeHexString("414")
	require.NoError(t, err)
	assert.Equal(t, "A@", value)

	_, err = decodeHexString("41GG")
	require.Error(t, err)
}
