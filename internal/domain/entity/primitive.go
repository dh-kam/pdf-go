// Package entity defines the core domain entities for PDF documents.
package entity

import (
	"fmt"
	"strings"
	"sync"
)

// ObjectType represents the type of a PDF object.
type ObjectType int

const (
	// TypeBoolean represents a PDF boolean object.
	TypeBoolean ObjectType = iota
	// TypeInteger represents a PDF integer object.
	TypeInteger
	// TypeReal represents a PDF real number object.
	TypeReal
	// TypeString represents a PDF string object.
	TypeString
	// TypeName represents a PDF name object.
	TypeName
	// TypeArray represents a PDF array object.
	TypeArray
	// TypeDictionary represents a PDF dictionary object.
	TypeDictionary
	// TypeStream represents a PDF stream object.
	TypeStream
	// TypeNull represents the PDF null object.
	TypeNull
	// TypeRef represents a PDF indirect reference.
	TypeRef
)

// String returns the string representation of the object type.
func (t ObjectType) String() string {
	switch t {
	case TypeBoolean:
		return "Boolean"
	case TypeInteger:
		return "Integer"
	case TypeReal:
		return "Real"
	case TypeString:
		return "String"
	case TypeName:
		return "Name"
	case TypeArray:
		return "Array"
	case TypeDictionary:
		return "Dictionary"
	case TypeStream:
		return "Stream"
	case TypeNull:
		return "Null"
	case TypeRef:
		return "Ref"
	default:
		return "Unknown"
	}
}

// Object is the interface that all PDF object types must implement.
type Object interface {
	// Type returns the ObjectType of this object.
	Type() ObjectType
	// String returns the string representation of this object.
	String() string
	// Clone creates a deep copy of this object.
	Clone() Object
}

// Boolean represents a PDF boolean value.
type Boolean struct {
	value bool
}

// NewBoolean creates a new Boolean object.
func NewBoolean(value bool) *Boolean {
	return &Boolean{value: value}
}

// Type returns TypeBoolean.
func (b *Boolean) Type() ObjectType {
	return TypeBoolean
}

// String returns the string representation.
func (b *Boolean) String() string {
	if b.value {
		return "true"
	}
	return "false"
}

// Clone creates a copy of this Boolean.
func (b *Boolean) Clone() Object {
	return NewBoolean(b.value)
}

// Value returns the boolean value.
func (b *Boolean) Value() bool {
	return b.value
}

// Integer represents a PDF integer value.
type Integer struct {
	value int64
}

// NewInteger creates a new Integer object.
func NewInteger(value int64) *Integer {
	return &Integer{value: value}
}

// Type returns TypeInteger.
func (i *Integer) Type() ObjectType {
	return TypeInteger
}

// String returns the string representation.
func (i *Integer) String() string {
	return fmt.Sprintf("%d", i.value)
}

// Clone creates a copy of this Integer.
func (i *Integer) Clone() Object {
	return NewInteger(i.value)
}

// Value returns the integer value.
func (i *Integer) Value() int64 {
	return i.value
}

// Real represents a PDF real number value.
type Real struct {
	value float64
}

// NewReal creates a new Real object.
func NewReal(value float64) *Real {
	return &Real{value: value}
}

// Type returns TypeReal.
func (r *Real) Type() ObjectType {
	return TypeReal
}

// String returns the string representation.
func (r *Real) String() string {
	return fmt.Sprintf("%f", r.value)
}

// Clone creates a copy of this Real.
func (r *Real) Clone() Object {
	return NewReal(r.value)
}

// Value returns the float64 value.
func (r *Real) Value() float64 {
	return r.value
}

// String represents a PDF string value.
type String struct {
	value    string
	encoding string
	isHex    bool
}

// NewString creates a new String object.
func NewString(value string) *String {
	return &String{
		value:    value,
		isHex:    false,
		encoding: "pdfdoc",
	}
}

// NewHexString creates a new hex-encoded String object.
func NewHexString(value string) *String {
	return &String{
		value:    value,
		isHex:    true,
		encoding: "pdfdoc",
	}
}

// Type returns TypeString.
func (s *String) Type() ObjectType {
	return TypeString
}

// String returns the string representation.
func (s *String) String() string {
	if s.isHex {
		return "<" + s.value + ">"
	}
	return "(" + s.value + ")"
}

// Clone creates a copy of this String.
func (s *String) Clone() Object {
	return &String{
		value:    s.value,
		isHex:    s.isHex,
		encoding: s.encoding,
	}
}

// Value returns the string value.
func (s *String) Value() string {
	return s.value
}

// IsHex returns true if this is a hex-encoded string.
func (s *String) IsHex() bool {
	return s.isHex
}

// Encoding returns the character encoding.
func (s *String) Encoding() string {
	return s.encoding
}

// Name represents a PDF name object.
// Name objects are immutable.
type Name string

// NewName creates a new Name object.
func NewName(name string) Name {
	return Name(name)
}

// Type returns TypeName.
func (n Name) Type() ObjectType {
	return TypeName
}

// String returns the string representation.
func (n Name) String() string {
	return "/" + string(n)
}

// Clone returns the name itself (names are immutable).
func (n Name) Clone() Object {
	return n
}

// Value returns the name value without the leading slash.
func (n Name) Value() string {
	return string(n)
}

// Null represents the PDF null object.
type Null struct{}

// NewNull creates a new Null object.
func NewNull() *Null {
	return &Null{}
}

// Type returns TypeNull.
func (n *Null) Type() ObjectType {
	return TypeNull
}

// String returns "null".
func (n *Null) String() string {
	return "null"
}

// Clone returns a new Null object.
func (n *Null) Clone() Object {
	return NewNull()
}

// Ref represents an indirect reference to a PDF object.
type Ref struct {
	num uint32
	gen uint16
}

// NewRef creates a new Ref object.
func NewRef(num uint32, gen uint16) Ref {
	return Ref{
		num: num,
		gen: gen,
	}
}

// Type returns TypeRef.
func (r Ref) Type() ObjectType {
	return TypeRef
}

// String returns the string representation (e.g., "123R" or "123R5").
func (r Ref) String() string {
	if r.gen == 0 {
		return fmt.Sprintf("%dR", r.num)
	}
	return fmt.Sprintf("%dR%d", r.num, r.gen)
}

// Clone returns the reference itself (refs are immutable).
func (r Ref) Clone() Object {
	return r
}

// Num returns the object number.
func (r Ref) Num() uint32 {
	return r.num
}

// Gen returns the generation number.
func (r Ref) Gen() uint16 {
	return r.gen
}

// Array represents a PDF array object.
type Array struct {
	items []Object
}

// NewArray creates a new Array object.
func NewArray(items ...Object) *Array {
	return &Array{
		items: items,
	}
}

// Type returns TypeArray.
func (a *Array) Type() ObjectType {
	return TypeArray
}

// String returns the string representation.
func (a *Array) String() string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, item := range a.items {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(item.String())
	}
	sb.WriteString("]")
	return sb.String()
}

// Clone creates a deep copy of this Array.
func (a *Array) Clone() Object {
	items := make([]Object, len(a.items))
	for i, item := range a.items {
		items[i] = item.Clone()
	}
	return NewArray(items...)
}

// Len returns the number of items in the array.
func (a *Array) Len() int {
	return len(a.items)
}

// Get returns the item at the given index.
// Returns nil if the index is out of bounds.
func (a *Array) Get(index int) Object {
	if index < 0 || index >= len(a.items) {
		return nil
	}
	return a.items[index]
}

// Items returns all items in the array.
func (a *Array) Items() []Object {
	return a.items
}

// Dict represents a PDF dictionary object.
type Dict struct {
	items map[Name]Object
	xref  XRef // Optional: for resolving indirect references
}

// XRef is the interface for resolving indirect references.
type XRef interface {
	// Fetch fetches the object at the given reference.
	Fetch(ref Ref) (Object, error)
}

// NewDict creates a new Dict object.
func NewDict() *Dict {
	return &Dict{
		items: make(map[Name]Object),
	}
}

// NewDictWithXRef creates a new Dict with an XRef for auto-dereferencing.
func NewDictWithXRef(xref XRef) *Dict {
	return &Dict{
		items: make(map[Name]Object),
		xref:  xref,
	}
}

// Type returns TypeDictionary.
func (d *Dict) Type() ObjectType {
	return TypeDictionary
}

// String returns the string representation.
func (d *Dict) String() string {
	var sb strings.Builder
	sb.WriteString("<<")
	first := true
	for key, value := range d.items {
		if !first {
			sb.WriteString(" ")
		}
		first = false
		sb.WriteString(key.String())
		sb.WriteString(" ")
		sb.WriteString(value.String())
	}
	sb.WriteString(">>")
	return sb.String()
}

// Clone creates a deep copy of this Dict.
func (d *Dict) Clone() Object {
	items := make(map[Name]Object, len(d.items))
	for key, value := range d.items {
		items[key] = value.Clone()
	}
	return &Dict{
		items: items,
		xref:  d.xref,
	}
}

// Get returns the value for the given key.
// If the value is an indirect reference and an XRef is available,
// it will be automatically dereferenced.
// Returns nil if the key is not found.
func (d *Dict) Get(key Name) Object {
	value, ok := d.lookupKey(key)
	if !ok {
		return nil
	}

	// Auto-dereference if XRef is available
	if ref, ok := value.(Ref); ok && d.xref != nil {
		obj, err := d.xref.Fetch(ref)
		if err == nil {
			return obj
		}
	}

	return value
}

// GetRaw returns the value for the given key without auto-dereferencing.
func (d *Dict) GetRaw(key Name) Object {
	value, ok := d.lookupKey(key)
	if !ok {
		return nil
	}
	return value
}

// lookupKey looks up a key with slash-insensitive fallback.
// PDF names are commonly represented as "/Name", but some call sites
// pass names without a leading slash.
func (d *Dict) lookupKey(key Name) (Object, bool) {
	if value, ok := d.items[key]; ok {
		return value, true
	}

	keyStr := string(key)
	if keyStr == "" {
		return nil, false
	}

	// Try the alternative representation:
	// "/Name" <-> "Name"
	if keyStr[0] == '/' {
		value, ok := d.items[Name(keyStr[1:])]
		return value, ok
	}

	value, ok := d.items[Name("/"+keyStr)]
	return value, ok
}

// GetTry tries multiple keys in order, returning the first found value.
func (d *Dict) GetTry(keys ...Name) Object {
	for _, key := range keys {
		if value := d.Get(key); value != nil {
			return value
		}
	}
	return nil
}

// Set sets the value for the given key.
func (d *Dict) Set(key Name, value Object) {
	d.items[key] = value
}

// Has returns true if the dictionary contains the given key.
func (d *Dict) Has(key Name) bool {
	_, ok := d.lookupKey(key)
	return ok
}

// Keys returns all keys in the dictionary.
func (d *Dict) Keys() []Name {
	keys := make([]Name, 0, len(d.items))
	for key := range d.items {
		keys = append(keys, key)
	}
	return keys
}

// Len returns the number of entries in the dictionary.
func (d *Dict) Len() int {
	return len(d.items)
}

// Stream represents a PDF stream object (dictionary + data).
type Stream struct {
	dict *Dict
	data []byte
}

var (
	streamDecoderMu sync.RWMutex
	streamDecoder   func(dict *Dict, data []byte) ([]byte, error)
)

// RegisterStreamDecoder registers a decoder hook used by Stream.Decode.
func RegisterStreamDecoder(decoder func(dict *Dict, data []byte) ([]byte, error)) {
	streamDecoderMu.Lock()
	defer streamDecoderMu.Unlock()
	streamDecoder = decoder
}

// NewStream creates a new Stream object.
func NewStream(dict *Dict, data []byte) *Stream {
	return &Stream{
		dict: dict,
		data: data,
	}
}

// Type returns TypeStream.
func (s *Stream) Type() ObjectType {
	return TypeStream
}

// String returns the string representation.
func (s *Stream) String() string {
	return fmt.Sprintf("Stream(%d bytes)", len(s.data))
}

// Clone creates a deep copy of this Stream.
func (s *Stream) Clone() Object {
	data := make([]byte, len(s.data))
	copy(data, s.data)
	var dictClone *Dict
	if s.dict != nil {
		if cloned, ok := s.dict.Clone().(*Dict); ok {
			dictClone = cloned
		}
	}
	return NewStream(dictClone, data)
}

// Dict returns the stream's dictionary.
func (s *Stream) Dict() *Dict {
	return s.dict
}

// RawBytes returns the raw (encoded) stream data.
func (s *Stream) RawBytes() []byte {
	return s.data
}

// Decode decodes stream data using an optional registered decoder hook.
// If no decoder is registered, it returns raw stream bytes.
func (s *Stream) Decode() ([]byte, error) {
	streamDecoderMu.RLock()
	decoder := streamDecoder
	streamDecoderMu.RUnlock()

	if decoder != nil {
		return decoder(s.dict, s.data)
	}

	return s.data, nil
}

// SetDict sets the stream's dictionary.
func (s *Stream) SetDict(dict *Dict) {
	s.dict = dict
}

// SetData sets the stream data.
func (s *Stream) SetData(data []byte) {
	s.data = data
}
