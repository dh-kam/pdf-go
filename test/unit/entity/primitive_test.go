// Package entity_test provides unit tests for PDF entity types.
package entity_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// TestObjectType_String tests the ObjectType String method.
func TestObjectType_String(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		typ      entity.ObjectType
	}{
		{"Boolean", "Boolean", entity.TypeBoolean},
		{"Integer", "Integer", entity.TypeInteger},
		{"Real", "Real", entity.TypeReal},
		{"String", "String", entity.TypeString},
		{"Name", "Name", entity.TypeName},
		{"Array", "Array", entity.TypeArray},
		{"Dictionary", "Dictionary", entity.TypeDictionary},
		{"Stream", "Stream", entity.TypeStream},
		{"Null", "Null", entity.TypeNull},
		{"Ref", "Ref", entity.TypeRef},
		{"Unknown", "Unknown", entity.ObjectType(999)},
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
		trueVal := entity.NewBoolean(true)
		assert.True(t, trueVal.Value())
		assert.Equal(t, entity.TypeBoolean, trueVal.Type())

		falseVal := entity.NewBoolean(false)
		assert.False(t, falseVal.Value())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		trueVal := entity.NewBoolean(true)
		assert.Equal(t, "true", trueVal.String())

		falseVal := entity.NewBoolean(false)
		assert.Equal(t, "false", falseVal.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := entity.NewBoolean(true)
		cloned := original.Clone().(*entity.Boolean)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.True(t, original != cloned) // Different pointers
	})
}

// TestInteger tests the Integer type.
func TestInteger(t *testing.T) {
	t.Run("NewInteger creates correct value", func(t *testing.T) {
		val := entity.NewInteger(42)
		assert.Equal(t, int64(42), val.Value())
		assert.Equal(t, entity.TypeInteger, val.Type())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		val := entity.NewInteger(-123)
		assert.Equal(t, "-123", val.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := entity.NewInteger(999)
		cloned := original.Clone().(*entity.Integer)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.True(t, original != cloned)
	})

	t.Run("Handles zero", func(t *testing.T) {
		val := entity.NewInteger(0)
		assert.Equal(t, int64(0), val.Value())
		assert.Equal(t, "0", val.String())
	})
}

// TestReal tests the Real type.
func TestReal(t *testing.T) {
	t.Run("NewReal creates correct value", func(t *testing.T) {
		val := entity.NewReal(3.14)
		assert.InDelta(t, 3.14, val.Value(), 0.001)
		assert.Equal(t, entity.TypeReal, val.Type())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		val := entity.NewReal(2.5)
		assert.Equal(t, "2.500000", val.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := entity.NewReal(1.23)
		cloned := original.Clone().(*entity.Real)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.True(t, original != cloned)
	})

	t.Run("Handles negative values", func(t *testing.T) {
		val := entity.NewReal(-0.5)
		assert.InDelta(t, -0.5, val.Value(), 0.001)
	})
}

// TestString tests the String type.
func TestString(t *testing.T) {
	t.Run("NewString creates literal string", func(t *testing.T) {
		val := entity.NewString("hello")
		assert.Equal(t, "hello", val.Value())
		assert.False(t, val.IsHex())
		assert.Equal(t, "(hello)", val.String())
		assert.Equal(t, entity.TypeString, val.Type())
	})

	t.Run("NewHexString creates hex string", func(t *testing.T) {
		val := entity.NewHexString("48656C6C6F")
		assert.Equal(t, "48656C6C6F", val.Value())
		assert.True(t, val.IsHex())
		assert.Equal(t, "<48656C6C6F>", val.String())
	})

	t.Run("Clone creates independent copy", func(t *testing.T) {
		original := entity.NewString("test")
		cloned := original.Clone().(*entity.String)

		assert.Equal(t, original.Value(), cloned.Value())
		assert.Equal(t, original.IsHex(), cloned.IsHex())
		assert.True(t, original != cloned)
	})

	t.Run("Encoding returns correct value", func(t *testing.T) {
		val := entity.NewString("test")
		assert.Equal(t, "pdfdoc", val.Encoding())
	})

	t.Run("Handles empty string", func(t *testing.T) {
		val := entity.NewString("")
		assert.Equal(t, "", val.Value())
		assert.Equal(t, "()", val.String())
	})
}

// TestName tests the Name type.
func TestName(t *testing.T) {
	t.Run("NewName creates correct name", func(t *testing.T) {
		name := entity.NewName("Type")
		assert.Equal(t, "Type", name.Value())
		assert.Equal(t, entity.TypeName, name.Type())
	})

	t.Run("String adds leading slash", func(t *testing.T) {
		name := entity.NewName("Font")
		assert.Equal(t, "/Font", name.String())
	})

	t.Run("Clone returns same value (immutable)", func(t *testing.T) {
		original := entity.NewName("Test")
		cloned := original.Clone().(entity.Name)

		assert.Equal(t, original, cloned)
	})

	t.Run("Handles special characters", func(t *testing.T) {
		name := entity.NewName("Helvetica-Bold")
		assert.Equal(t, "Helvetica-Bold", name.Value())
	})
}

// TestNull tests the Null type.
func TestNull(t *testing.T) {
	t.Run("NewNull creates null object", func(t *testing.T) {
		null := entity.NewNull()
		assert.Equal(t, entity.TypeNull, null.Type())
		assert.Equal(t, "null", null.String())
	})

	t.Run("Clone creates new null", func(t *testing.T) {
		original := entity.NewNull()
		cloned := original.Clone().(*entity.Null)

		assert.True(t, original != cloned)
	})
}

// TestRef tests the Ref type.
func TestRef(t *testing.T) {
	t.Run("NewRef creates correct reference", func(t *testing.T) {
		ref := entity.NewRef(123, 0)
		assert.Equal(t, uint32(123), ref.Num())
		assert.Equal(t, uint16(0), ref.Gen())
		assert.Equal(t, entity.TypeRef, ref.Type())
	})

	t.Run("String with zero generation", func(t *testing.T) {
		ref := entity.NewRef(42, 0)
		assert.Equal(t, "42R", ref.String())
	})

	t.Run("String with non-zero generation", func(t *testing.T) {
		ref := entity.NewRef(10, 5)
		assert.Equal(t, "10R5", ref.String())
	})

	t.Run("Clone returns same value (immutable)", func(t *testing.T) {
		original := entity.NewRef(1, 2)
		cloned := original.Clone().(entity.Ref)

		assert.Equal(t, original, cloned)
	})
}

// TestArray tests the Array type.
func TestArray(t *testing.T) {
	t.Run("NewArray creates empty array", func(t *testing.T) {
		arr := entity.NewArray()
		assert.Equal(t, 0, arr.Len())
		assert.Equal(t, entity.TypeArray, arr.Type())
	})

	t.Run("NewArray with items", func(t *testing.T) {
		boolVal := entity.NewBoolean(true)
		intVal := entity.NewInteger(42)
		arr := entity.NewArray(boolVal, intVal)

		assert.Equal(t, 2, arr.Len())
		assert.Equal(t, boolVal, arr.Get(0))
		assert.Equal(t, intVal, arr.Get(1))
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		boolVal := entity.NewBoolean(true)
		intVal := entity.NewInteger(42)
		arr := entity.NewArray(boolVal, intVal)

		result := arr.String()
		assert.Contains(t, result, "true")
		assert.Contains(t, result, "42")
	})

	t.Run("Get returns nil for out of bounds", func(t *testing.T) {
		arr := entity.NewArray()
		assert.Nil(t, arr.Get(0))
		assert.Nil(t, arr.Get(-1))
	})

	t.Run("Get returns nil for negative index", func(t *testing.T) {
		arr := entity.NewArray(entity.NewInteger(1))
		assert.Nil(t, arr.Get(-1))
	})

	t.Run("Clone creates deep copy", func(t *testing.T) {
		original := entity.NewArray(
			entity.NewBoolean(true),
			entity.NewInteger(42),
		)
		cloned := original.Clone().(*entity.Array)

		assert.Equal(t, original.Len(), cloned.Len())
		for i := 0; i < original.Len(); i++ {
			assert.Equal(t, original.Get(i).String(), cloned.Get(i).String())
		}
		assert.True(t, original != cloned)
	})

	t.Run("Items returns all items", func(t *testing.T) {
		item1 := entity.NewBoolean(true)
		item2 := entity.NewInteger(42)
		arr := entity.NewArray(item1, item2)

		items := arr.Items()
		assert.Equal(t, 2, len(items))
		assert.Equal(t, item1, items[0])
		assert.Equal(t, item2, items[1])
	})
}

// TestDict tests the Dict type.
func TestDict(t *testing.T) {
	t.Run("NewDict creates empty dictionary", func(t *testing.T) {
		dict := entity.NewDict()
		assert.Equal(t, 0, dict.Len())
		assert.Equal(t, entity.TypeDictionary, dict.Type())
	})

	t.Run("Set and Get work correctly", func(t *testing.T) {
		dict := entity.NewDict()
		key := entity.NewName("Type")
		value := entity.NewName("Page")

		dict.Set(key, value)
		assert.True(t, dict.Has(key))
		assert.Equal(t, value, dict.Get(key))
	})

	t.Run("Get returns nil for missing key", func(t *testing.T) {
		dict := entity.NewDict()
		key := entity.NewName("Missing")
		assert.Nil(t, dict.Get(key))
	})

	t.Run("GetTry returns first found value", func(t *testing.T) {
		dict := entity.NewDict()
		key1 := entity.NewName("Type1")
		key2 := entity.NewName("Type2")
		value2 := entity.NewName("Found")

		dict.Set(key2, value2)

		result := dict.GetTry(key1, key2)
		assert.Equal(t, value2, result)
	})

	t.Run("GetTry returns nil if none found", func(t *testing.T) {
		dict := entity.NewDict()
		result := dict.GetTry(
			entity.NewName("Missing1"),
			entity.NewName("Missing2"),
		)
		assert.Nil(t, result)
	})

	t.Run("Keys returns all keys", func(t *testing.T) {
		dict := entity.NewDict()
		key1 := entity.NewName("Key1")
		key2 := entity.NewName("Key2")

		dict.Set(key1, entity.NewBoolean(true))
		dict.Set(key2, entity.NewBoolean(false))

		keys := dict.Keys()
		assert.Equal(t, 2, len(keys))
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		dict := entity.NewDict()
		key := entity.NewName("Type")
		value := entity.NewName("Page")
		dict.Set(key, value)

		result := dict.String()
		assert.Contains(t, result, "/Type")
		assert.Contains(t, result, "/Page")
	})

	t.Run("Clone creates deep copy", func(t *testing.T) {
		original := entity.NewDict()
		key := entity.NewName("Test")
		value := entity.NewInteger(42)
		original.Set(key, value)

		cloned := original.Clone().(*entity.Dict)

		assert.Equal(t, original.Len(), cloned.Len())
		assert.Equal(t, original.Get(key).String(), cloned.Get(key).String())
		assert.True(t, original != cloned)
	})

	t.Run("NewDictWithXRef creates dict with XRef", func(t *testing.T) {
		mockXRef := &testableXRef{
			returnObj: entity.NewInteger(999),
		}
		dict := entity.NewDictWithXRef(mockXRef)
		assert.NotNil(t, dict)
	})

	t.Run("Get with XRef dereferences indirect references", func(t *testing.T) {
		mockXRef := &testableXRef{
			returnObj: entity.NewInteger(999),
		}
		dict := entity.NewDictWithXRef(mockXRef)
		key := entity.NewName("Value")
		ref := entity.NewRef(10, 0)

		dict.Set(key, ref)
		result := dict.Get(key)

		assert.Equal(t, "999", result.String())
	})
}

// TestStream tests the Stream type.
func TestStream(t *testing.T) {
	t.Run("NewStream creates stream", func(t *testing.T) {
		dict := entity.NewDict()
		data := []byte("test data")

		stream := entity.NewStream(dict, data)
		assert.Equal(t, entity.TypeStream, stream.Type())
		assert.Equal(t, dict, stream.Dict())
		assert.Equal(t, data, stream.RawBytes())
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		data := []byte("test data")
		stream := entity.NewStream(entity.NewDict(), data)

		result := stream.String()
		assert.Contains(t, result, "9 bytes")
	})

	t.Run("Decode returns raw data (placeholder)", func(t *testing.T) {
		data := []byte("test data")
		stream := entity.NewStream(entity.NewDict(), data)

		decoded, err := stream.Decode()
		require.NoError(t, err)
		assert.Equal(t, data, decoded)
	})

	t.Run("Clone creates deep copy", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.NewName("Length"), entity.NewInteger(4))
		data := []byte("test")

		original := entity.NewStream(dict, data)
		cloned := original.Clone().(*entity.Stream)

		assert.Equal(t, original.String(), cloned.String())
		assert.True(t, original != cloned)
		cloned.RawBytes()[0] = 'b'
		assert.Equal(t, []byte("test"), original.RawBytes())
		assert.Equal(t, []byte("best"), cloned.RawBytes())
	})

	t.Run("SetDict and SetData work correctly", func(t *testing.T) {
		stream := entity.NewStream(entity.NewDict(), []byte("original"))

		newDict := entity.NewDict()
		newData := []byte("modified")

		stream.SetDict(newDict)
		stream.SetData(newData)

		assert.Equal(t, newDict, stream.Dict())
		assert.Equal(t, newData, stream.RawBytes())
	})

	t.Run("Handles empty data", func(t *testing.T) {
		stream := entity.NewStream(entity.NewDict(), []byte{})
		assert.Equal(t, []byte{}, stream.RawBytes())
		assert.Equal(t, "Stream(0 bytes)", stream.String())
	})
}

// testableXRef is a test-specific mock that returns configurable objects.
type testableXRef struct {
	returnObj entity.Object
	returnErr error
}

func (m *testableXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	return m.returnObj, m.returnErr
}
