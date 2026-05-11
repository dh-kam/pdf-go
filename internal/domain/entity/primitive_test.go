package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestObjectType_String tests the ObjectType String method.
func TestObjectType_String(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		typ      ObjectType
	}{
		{"Boolean", "Boolean", TypeBoolean},
		{"Integer", "Integer", TypeInteger},
		{"Real", "Real", TypeReal},
		{"String", "String", TypeString},
		{"Name", "Name", TypeName},
		{"Array", "Array", TypeArray},
		{"Dictionary", "Dictionary", TypeDictionary},
		{"Stream", "Stream", TypeStream},
		{"Null", "Null", TypeNull},
		{"Ref", "Ref", TypeRef},
		{"Unknown", "Unknown", ObjectType(999)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.typ.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBoolean tests the Boolean type.
func TestBoolean(t *testing.T) {
	t.Run("NewBoolean creates correct value", func(t *testing.T) {
		trueVal := NewBoolean(true)
		assert.True(t, trueVal.Value())
		assert.Equal(t, TypeBoolean, trueVal.Type())

		falseVal := NewBoolean(false)
		assert.False(t, falseVal.Value())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		trueVal := NewBoolean(true)
		assert.Equal(t, "true", trueVal.String())

		falseVal := NewBoolean(false)
		assert.Equal(t, "false", falseVal.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := NewBoolean(true)
		cloned := original.Clone().(*Boolean)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.True(t, original != cloned)
	})
}

// TestInteger tests the Integer type.
func TestInteger(t *testing.T) {
	t.Run("NewInteger creates correct value", func(t *testing.T) {
		val := NewInteger(42)
		assert.Equal(t, int64(42), val.Value())
		assert.Equal(t, TypeInteger, val.Type())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		val := NewInteger(-123)
		assert.Equal(t, "-123", val.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := NewInteger(999)
		cloned := original.Clone().(*Integer)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.True(t, original != cloned)
	})

	t.Run("Handles zero", func(t *testing.T) {
		val := NewInteger(0)
		assert.Equal(t, int64(0), val.Value())
		assert.Equal(t, "0", val.String())
	})
}

// TestReal tests the Real type.
func TestReal(t *testing.T) {
	t.Run("NewReal creates correct value", func(t *testing.T) {
		val := NewReal(3.14)
		assert.InDelta(t, 3.14, val.Value(), 0.001)
		assert.Equal(t, TypeReal, val.Type())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		val := NewReal(2.5)
		assert.Equal(t, "2.500000", val.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := NewReal(1.23)
		cloned := original.Clone().(*Real)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.True(t, original != cloned)
	})

	t.Run("Handles negative values", func(t *testing.T) {
		val := NewReal(-0.5)
		assert.InDelta(t, -0.5, val.Value(), 0.001)
	})
}

// TestString tests the String type.
func TestString(t *testing.T) {
	t.Run("NewString creates literal string", func(t *testing.T) {
		val := NewString("hello")
		assert.Equal(t, "hello", val.Value())
		assert.False(t, val.IsHex())
		assert.Equal(t, "(hello)", val.String())
		assert.Equal(t, TypeString, val.Type())
	})

	t.Run("NewHexString creates hex string", func(t *testing.T) {
		val := NewHexString("48656C6C6F")
		assert.Equal(t, "48656C6C6F", val.Value())
		assert.True(t, val.IsHex())
		assert.Equal(t, "<48656C6C6F>", val.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := NewString("test")
		cloned := original.Clone().(*String)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.Equal(t, original.IsHex(), cloned.IsHex())
		assert.True(t, original != cloned)
	})

	t.Run("Encoding returns correct value", func(t *testing.T) {
		val := NewString("test")
		assert.Equal(t, "pdfdoc", val.Encoding())
	})

	t.Run("Handles empty string", func(t *testing.T) {
		val := NewString("")
		assert.Equal(t, "", val.Value())
		assert.Equal(t, "()", val.String())
	})
}

// TestName tests the Name type.
func TestName(t *testing.T) {
	t.Run("NewName creates correct name", func(t *testing.T) {
		name := NewName("Type")
		assert.Equal(t, "Type", name.Value())
		assert.Equal(t, TypeName, name.Type())
	})

	t.Run("String adds leading slash", func(t *testing.T) {
		name := NewName("Font")
		assert.Equal(t, "/Font", name.String())
	})

	t.Run("Clone returns same value (immutable)", func(t *testing.T) {
		original := NewName("Test")
		cloned := original.Clone().(Name)

		assert.Equal(t, original, cloned)
	})

	t.Run("Handles special characters", func(t *testing.T) {
		name := NewName("Helvetica-Bold")
		assert.Equal(t, "Helvetica-Bold", name.Value())
	})
}

// TestNull tests the Null type.
func TestNull(t *testing.T) {
	t.Run("NewNull creates null object", func(t *testing.T) {
		null := NewNull()
		assert.Equal(t, TypeNull, null.Type())
		assert.Equal(t, "null", null.String())
	})

	t.Run("Clone creates new null", func(t *testing.T) {
		original := NewNull()
		cloned := original.Clone().(*Null)

		assert.True(t, original != cloned)
	})
}

// TestRef tests the Ref type.
func TestRef(t *testing.T) {
	t.Run("NewRef creates correct reference", func(t *testing.T) {
		ref := NewRef(123, 0)
		assert.Equal(t, uint32(123), ref.Num())
		assert.Equal(t, uint16(0), ref.Gen())
		assert.Equal(t, TypeRef, ref.Type())
	})

	t.Run("String with zero generation", func(t *testing.T) {
		ref := NewRef(42, 0)
		assert.Equal(t, "42R", ref.String())
	})

	t.Run("String with non-zero generation", func(t *testing.T) {
		ref := NewRef(10, 5)
		assert.Equal(t, "10R5", ref.String())
	})

	t.Run("Clone returns same value (immutable)", func(t *testing.T) {
		original := NewRef(1, 2)
		cloned := original.Clone().(Ref)

		assert.Equal(t, original, cloned)
	})
}

// TestArray tests the Array type.
func TestArray(t *testing.T) {
	t.Run("NewArray creates empty array", func(t *testing.T) {
		arr := NewArray()
		assert.Equal(t, 0, arr.Len())
		assert.Equal(t, TypeArray, arr.Type())
	})

	t.Run("NewArray with items", func(t *testing.T) {
		boolVal := NewBoolean(true)
		intVal := NewInteger(42)
		arr := NewArray(boolVal, intVal)

		assert.Equal(t, 2, arr.Len())
		assert.Equal(t, boolVal, arr.Get(0))
		assert.Equal(t, intVal, arr.Get(1))
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		boolVal := NewBoolean(true)
		intVal := NewInteger(42)
		arr := NewArray(boolVal, intVal)

		result := arr.String()
		assert.Contains(t, result, "true")
		assert.Contains(t, result, "42")
	})

	t.Run("Get returns nil for out of bounds", func(t *testing.T) {
		arr := NewArray()
		assert.Nil(t, arr.Get(0))
		assert.Nil(t, arr.Get(-1))
	})

	t.Run("Get returns nil for negative index", func(t *testing.T) {
		arr := NewArray(NewInteger(1))
		assert.Nil(t, arr.Get(-1))
	})

	t.Run("Clone creates deep copy", func(t *testing.T) {
		original := NewArray(
			NewBoolean(true),
			NewInteger(42),
		)
		cloned := original.Clone().(*Array)

		assert.Equal(t, original.Len(), cloned.Len())
		for i := 0; i < original.Len(); i++ {
			assert.Equal(t, original.Get(i).String(), cloned.Get(i).String())
		}
		assert.True(t, original != cloned)
	})

	t.Run("Items returns all items", func(t *testing.T) {
		item1 := NewBoolean(true)
		item2 := NewInteger(42)
		arr := NewArray(item1, item2)

		items := arr.Items()
		assert.Equal(t, 2, len(items))
		assert.Equal(t, item1, items[0])
		assert.Equal(t, item2, items[1])
	})
}

// TestDict tests the Dict type.
func TestDict(t *testing.T) {
	t.Run("NewDict creates empty dictionary", func(t *testing.T) {
		dict := NewDict()
		assert.Equal(t, 0, dict.Len())
		assert.Equal(t, TypeDictionary, dict.Type())
	})

	t.Run("Set and Get work correctly", func(t *testing.T) {
		dict := NewDict()
		key := NewName("Type")
		value := NewName("Page")

		dict.Set(key, value)
		assert.True(t, dict.Has(key))
		assert.Equal(t, value, dict.Get(key))
	})

	t.Run("Get returns nil for missing key", func(t *testing.T) {
		dict := NewDict()
		key := NewName("Missing")
		assert.Nil(t, dict.Get(key))
	})

	t.Run("GetTry returns first found value", func(t *testing.T) {
		dict := NewDict()
		key1 := NewName("Type1")
		key2 := NewName("Type2")
		value2 := NewName("Found")

		dict.Set(key2, value2)

		result := dict.GetTry(key1, key2)
		assert.Equal(t, value2, result)
	})

	t.Run("GetTry returns nil if none found", func(t *testing.T) {
		dict := NewDict()
		result := dict.GetTry(
			NewName("Missing1"),
			NewName("Missing2"),
		)
		assert.Nil(t, result)
	})

	t.Run("Keys returns all keys", func(t *testing.T) {
		dict := NewDict()
		key1 := NewName("Key1")
		key2 := NewName("Key2")

		dict.Set(key1, NewBoolean(true))
		dict.Set(key2, NewBoolean(false))

		keys := dict.Keys()
		assert.Equal(t, 2, len(keys))
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		dict := NewDict()
		key := NewName("Type")
		value := NewName("Page")
		dict.Set(key, value)

		result := dict.String()
		assert.Contains(t, result, "/Type")
		assert.Contains(t, result, "/Page")
	})

	t.Run("Clone creates deep copy", func(t *testing.T) {
		original := NewDict()
		key := NewName("Test")
		value := NewInteger(42)
		original.Set(key, value)

		cloned := original.Clone().(*Dict)

		assert.Equal(t, original.Len(), cloned.Len())
		assert.Equal(t, original.Get(key).String(), cloned.Get(key).String())
		assert.True(t, original != cloned)
	})

	t.Run("Get with XRef dereferences indirect references", func(t *testing.T) {
		mockXRef := &testXRef{obj: NewInteger(999)}
		dict := NewDictWithXRef(mockXRef)
		key := NewName("Value")
		ref := NewRef(10, 0)

		dict.Set(key, ref)
		result := dict.Get(key)

		assert.Equal(t, "999", result.String())
	})
}

// TestStream tests the Stream type.
func TestStream(t *testing.T) {
	t.Run("NewStream creates stream", func(t *testing.T) {
		dict := NewDict()
		data := []byte("test data")

		stream := NewStream(dict, data)
		assert.Equal(t, TypeStream, stream.Type())
		assert.Equal(t, dict, stream.Dict())
		assert.Equal(t, data, stream.RawBytes())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		data := []byte("test data")
		stream := NewStream(NewDict(), data)

		result := stream.String()
		assert.Contains(t, result, "9 bytes")
	})

	t.Run("Decode returns raw data (placeholder)", func(t *testing.T) {
		data := []byte("test data")
		stream := NewStream(NewDict(), data)

		decoded, err := stream.Decode()
		assert.NoError(t, err)
		assert.Equal(t, data, decoded)
	})

	t.Run("Clone creates deep copy", func(t *testing.T) {
		dict := NewDict()
		dict.Set(NewName("Length"), NewInteger(4))
		data := []byte("test")

		original := NewStream(dict, data)
		cloned := original.Clone().(*Stream)

		assert.Equal(t, original.String(), cloned.String())
		assert.True(t, original != cloned)
		assert.NotSame(t, original.RawBytes(), cloned.RawBytes())
	})

	t.Run("SetDict and SetData work correctly", func(t *testing.T) {
		stream := NewStream(NewDict(), []byte("original"))

		newDict := NewDict()
		newData := []byte("modified")

		stream.SetDict(newDict)
		stream.SetData(newData)

		assert.Equal(t, newDict, stream.Dict())
		assert.Equal(t, newData, stream.RawBytes())
	})

	t.Run("Handles empty data", func(t *testing.T) {
		stream := NewStream(NewDict(), []byte{})
		assert.Equal(t, []byte{}, stream.RawBytes())
		assert.Equal(t, "Stream(0 bytes)", stream.String())
	})
}

// testXRef is a mock for testing Dict.Get with XRef dereferencing.
type testXRef struct {
	obj Object
}

func (m *testXRef) Fetch(ref Ref) (Object, error) {
	return m.obj, nil
}
