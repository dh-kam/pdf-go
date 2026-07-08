// Package stream provides PDF stream handling and filtering.
package stream

import (
	"bytes"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

const (
	streamNameBitsPerComponent entity.Name = "/BitsPerComponent"
	streamNameBPC              entity.Name = "/BPC"
	streamNameColorSpace       entity.Name = "/ColorSpace"
	streamNameCS               entity.Name = "/CS"
	streamNameDecodeParms      entity.Name = "/DecodeParms"
	streamNameFilter           entity.Name = "/Filter"
	streamNameH                entity.Name = "/H"
	streamNameHeight           entity.Name = "/Height"
	streamNameColumns          entity.Name = "/Columns"
	streamNameColors           entity.Name = "/Colors"
	streamNameRows             entity.Name = "/Rows"
	streamNameK                entity.Name = "/K"
	streamNameBlackIs1         entity.Name = "/BlackIs1"
	streamNameEncodedByteAlign entity.Name = "/EncodedByteAlign"
	streamNameEarlyChange      entity.Name = "/EarlyChange"
	streamNameIM               entity.Name = "/IM"
	streamNameImageMask        entity.Name = "/ImageMask"
	streamNameLength1          entity.Name = "/Length1"
	streamNameLength2          entity.Name = "/Length2"
	streamNameLength3          entity.Name = "/Length3"
	streamNameN                entity.Name = "/N"
	streamNamePredictor        entity.Name = "/Predictor"
	streamNameW                entity.Name = "/W"
	streamNameWidth            entity.Name = "/Width"
)

// Stream represents a PDF stream with optional filters.
type Stream struct {
	dict                   *entity.Dict
	data                   []byte
	decoded                []byte // Cached decoded data
	decodeSizeHintOverride int
}

func init() {
	entity.RegisterStreamDecoder(func(dict *entity.Dict, data []byte) ([]byte, error) {
		return NewStream(dict, data).Decode()
	})
}

// NewStream creates a new Stream.
func NewStream(dict *entity.Dict, data []byte) *Stream {
	return &Stream{
		dict: dict,
		data: data,
	}
}

// SetDecodeSizeHint sets an optional caller-provided decoded byte count hint.
func (s *Stream) SetDecodeSizeHint(size int) {
	if s == nil || size <= 0 {
		return
	}
	const maxDecodeSizeHint = 512 << 20
	if size > maxDecodeSizeHint {
		return
	}
	s.decodeSizeHintOverride = size
}

// Dict returns the stream dictionary.
func (s *Stream) Dict() *entity.Dict {
	return s.dict
}

// RawData returns the raw (encoded) stream data.
func (s *Stream) RawData() []byte {
	return s.data
}

// Length returns the length of the raw data.
func (s *Stream) Length() int {
	return len(s.data)
}

// Decode decodes the stream data using the specified filters.
func (s *Stream) Decode() ([]byte, error) {
	if s.decoded != nil {
		return s.decoded, nil
	}

	// Get filters from stream dictionary
	filterVal := s.dict.Get(streamNameFilter)
	if filterVal == nil {
		// No filter, but may still have predictor to apply
		// Check for DecodeParms
		decodeParmsVal := s.dict.Get(streamNameDecodeParms)
		if decodeParmsVal != nil {
			if decodeParms, ok := decodeParmsVal.(*entity.Dict); ok {
				// Apply predictor if specified
				if hasPredictor(decodeParms) {
					data, err := ApplyPredictor(s.data, decodeParms)
					if err != nil {
						return nil, err
					}
					s.decoded = data
					return s.decoded, nil
				}
			}
		}
		// No filter or predictor, data is already decoded
		s.decoded = s.data
		return s.decoded, nil
	}

	var filters []entity.Name
	switch v := filterVal.(type) {
	case entity.Name:
		filters = []entity.Name{v}
	case *entity.Array:
		filters = make([]entity.Name, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem := v.Get(i)
			if name, ok := elem.(entity.Name); ok {
				filters[i] = name
			}
		}
	default:
		// Unknown filter type, return raw data
		s.decoded = s.data
		return s.decoded, nil
	}

	// Get DecodeParms for each filter
	decodeParamsList, err := s.getDecodeParamsList(len(filters))
	if err != nil {
		return nil, err
	}

	// Apply filters in order
	data := s.data
	for i, filter := range filters {
		decoder, err := GetDecoder(filter)
		if err != nil {
			return nil, err
		}

		// Some filters (for example CCITTFax) require per-filter DecodeParms.
		if decodeParamsAwareDecoder, ok := decoder.(decodeParamsAware); ok {
			var params *entity.Dict
			if i < len(decodeParamsList) {
				params = decodeParamsList[i]
			}
			decodeParamsAwareDecoder.SetDecodeParams(params)
		}
		if hintAwareDecoder, ok := decoder.(decodeSizeHintAware); ok {
			var params *entity.Dict
			if i < len(decodeParamsList) {
				params = decodeParamsList[i]
			}
			hintAwareDecoder.SetDecodeSizeHint(s.decodeSizeHint(params))
		}
		if exactHintAwareDecoder, ok := decoder.(exactDecodeSizeHintAware); ok {
			var params *entity.Dict
			if i < len(decodeParamsList) {
				params = decodeParamsList[i]
			}
			exactHintAwareDecoder.SetExactDecodeSizeHint(s.exactDecodeSizeHint(params, i == len(filters)-1))
		}

		data, err = decoder.Decode(data)
		if err != nil {
			return nil, err
		}

		// Apply predictor if specified in DecodeParms
		// Predictor is applied after the filter decompression
		if i < len(decodeParamsList) && decodeParamsList[i] != nil {
			params := decodeParamsList[i]
			if hasPredictor(params) {
				data, err = ApplyPredictorInPlace(data, params)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	s.decoded = data
	return s.decoded, nil
}

// getDecodeParamsList extracts DecodeParms for each filter.
// Returns a slice where each element corresponds to the DecodeParms for that filter.
func (s *Stream) getDecodeParamsList(numFilters int) ([]*entity.Dict, error) {
	// Get DecodeParms from stream dictionary
	decodeParmsVal := s.dict.Get(streamNameDecodeParms)
	if decodeParmsVal == nil {
		return nil, nil
	}

	decodeParamsList := make([]*entity.Dict, numFilters)
	switch v := decodeParmsVal.(type) {
	case *entity.Dict:
		// Single DecodeParms for a single filter
		if numFilters == 1 {
			decodeParamsList[0] = v
		}
	case *entity.Array:
		// Array of DecodeParms, one for each filter
		for i := 0; i < v.Len() && i < numFilters; i++ {
			elem := v.Get(i)
			if dict, ok := elem.(*entity.Dict); ok {
				decodeParamsList[i] = dict
			}
		}
	}

	return decodeParamsList, nil
}

// hasPredictor checks if the DecodeParms dictionary specifies a predictor other than 1 (no prediction).
func hasPredictor(params *entity.Dict) bool {
	if params == nil {
		return false
	}
	val := params.Get(streamNamePredictor)
	if val == nil {
		return false
	}
	if integer, ok := val.(*entity.Integer); ok {
		return integer.Value() != 1
	}
	return false
}

// Bytes returns the decoded stream data.
func (s *Stream) Bytes() ([]byte, error) {
	return s.Decode()
}

// Reader returns a reader for the decoded stream data.
func (s *Stream) Reader() (io.ReadCloser, error) {
	data, err := s.Decode()
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Reset clears the cached decoded data.
func (s *Stream) Reset() {
	s.decoded = nil
}

func (s *Stream) decodeSizeHint(params *entity.Dict) int {
	if s == nil || s.dict == nil {
		return 0
	}
	size := s.decodeParamsSizeHint(params)
	if imageSize := s.imageDecodeSizeHint(); imageSize > size {
		size = imageSize
	}
	if s.decodeSizeHintOverride > size {
		size = s.decodeSizeHintOverride
	}
	return size
}

func (s *Stream) exactDecodeSizeHint(params *entity.Dict, isLastFilter bool) int {
	if s == nil || s.dict == nil {
		return 0
	}
	if !isLastFilter {
		return 0
	}

	size := 0
	for _, name := range []entity.Name{streamNameLength1, streamNameLength2, streamNameLength3} {
		value := dictInt(s.dict, name)
		if value <= 0 {
			continue
		}
		size += value
	}
	const maxDecodeSizeHint = 512 << 20
	if size <= 0 || size > maxDecodeSizeHint {
		size = 0
	}
	if size > 0 {
		return size
	}

	if params != nil && hasPredictor(params) {
		if size := s.decodeParamsSizeHint(params); size > 0 {
			return size
		}
	}
	if size := s.imageDecodeSizeHint(); size > 0 {
		return size
	}
	return 0
}

func (s *Stream) decodeParamsSizeHint(params *entity.Dict) int {
	height := dictInt(s.dict, streamNameHeight, streamNameH)
	if height <= 0 {
		return 0
	}
	decodeParams, err := GetDecodeParams(params)
	if err != nil || decodeParams.Columns <= 0 || decodeParams.Colors <= 0 || decodeParams.BitsPerComponent <= 0 {
		return 0
	}
	rowBytes := (decodeParams.Columns*decodeParams.Colors*decodeParams.BitsPerComponent + 7) / 8
	if rowBytes <= 0 {
		return 0
	}
	size := rowBytes * height
	if decodeParams.Predictor >= 10 {
		size += height // PNG predictors include one filter byte per row before predictor decoding.
	}
	const maxDecodeSizeHint = 512 << 20
	if size <= 0 || size > maxDecodeSizeHint {
		return 0
	}
	return size
}

func (s *Stream) imageDecodeSizeHint() int {
	if s == nil || s.dict == nil {
		return 0
	}
	width := dictInt(s.dict, streamNameWidth, streamNameW)
	height := dictInt(s.dict, streamNameHeight, streamNameH)
	if width <= 0 || height <= 0 {
		return 0
	}
	bitsPerComponent := dictInt(s.dict, streamNameBitsPerComponent, streamNameBPC)
	if bitsPerComponent <= 0 {
		if isImageMaskDict(s.dict) {
			bitsPerComponent = 1
		} else {
			bitsPerComponent = 8
		}
	}
	components := imageColorComponents(s.dict.Get(streamNameColorSpace))
	if components <= 0 {
		components = imageColorComponents(s.dict.Get(streamNameCS))
	}
	if components <= 0 {
		if isImageMaskDict(s.dict) {
			components = 1
		} else {
			return 0
		}
	}
	rowBits := width * components * bitsPerComponent
	if rowBits <= 0 {
		return 0
	}
	size := ((rowBits + 7) / 8) * height
	const maxDecodeSizeHint = 512 << 20
	if size <= 0 || size > maxDecodeSizeHint {
		return 0
	}
	return size
}

func isImageMaskDict(dict *entity.Dict) bool {
	if dict == nil {
		return false
	}
	if b, ok := dict.Get(streamNameImageMask).(*entity.Boolean); ok {
		return b.Value()
	}
	if b, ok := dict.Get(streamNameIM).(*entity.Boolean); ok {
		return b.Value()
	}
	return false
}

func imageColorComponents(obj entity.Object) int {
	switch v := obj.(type) {
	case entity.Name:
		switch v {
		case entity.Name("DeviceGray"), entity.Name("/DeviceGray"), entity.Name("G"), entity.Name("/G"):
			return 1
		case entity.Name("DeviceRGB"), entity.Name("/DeviceRGB"), entity.Name("RGB"), entity.Name("/RGB"):
			return 3
		case entity.Name("DeviceCMYK"), entity.Name("/DeviceCMYK"), entity.Name("CMYK"), entity.Name("/CMYK"):
			return 4
		}
	case *entity.Array:
		if v.Len() == 0 {
			return 0
		}
		name, _ := v.Get(0).(entity.Name)
		switch name {
		case entity.Name("Indexed"), entity.Name("/Indexed"), entity.Name("I"), entity.Name("/I"), entity.Name("Separation"), entity.Name("/Separation"):
			return 1
		case entity.Name("DeviceN"), entity.Name("/DeviceN"):
			if names, ok := v.Get(1).(*entity.Array); ok {
				return names.Len()
			}
		case entity.Name("ICCBased"), entity.Name("/ICCBased"):
			if stream, ok := v.Get(1).(*entity.Stream); ok {
				return dictInt(stream.Dict(), streamNameN)
			}
			if dict, ok := v.Get(1).(*entity.Dict); ok {
				return dictInt(dict, streamNameN)
			}
		}
	}
	return 0
}

func dictInt(dict *entity.Dict, keys ...entity.Name) int {
	if dict == nil {
		return 0
	}
	for _, key := range keys {
		if integer, ok := dict.Get(key).(*entity.Integer); ok {
			return int(integer.Value())
		}
	}
	return 0
}

// Decoder represents a stream filter decoder.
type Decoder interface {
	// Decode decodes the input data.
	Decode(data []byte) ([]byte, error)
}

// decodeParamsAware is implemented by decoders that consume DecodeParms.
type decodeParamsAware interface {
	// SetDecodeParams sets per-filter decode parameters from stream dictionary.
	SetDecodeParams(params *entity.Dict)
}

// decodeSizeHintAware is implemented by decoders that can preallocate output.
type decodeSizeHintAware interface {
	// SetDecodeSizeHint sets an estimated decoded byte count.
	SetDecodeSizeHint(size int)
}

// exactDecodeSizeHintAware is implemented by decoders that can consume trusted decoded size hints.
type exactDecodeSizeHintAware interface {
	// SetExactDecodeSizeHint sets a trusted decoded byte count.
	SetExactDecodeSizeHint(size int)
}

// DecoderFactory creates a decoder for a given filter type.
type DecoderFactory interface {
	// CreateDecoder creates a new decoder.
	CreateDecoder() (Decoder, error)
}

// decoderFactories holds registered decoder factories.
var decoderFactories = map[entity.Name]DecoderFactory{}

// RegisterDecoder registers a decoder factory for a filter type.
func RegisterDecoder(name entity.Name, factory DecoderFactory) {
	decoderFactories[name] = factory
}

// GetDecoder gets a decoder for the given filter type.
func GetDecoder(name entity.Name) (Decoder, error) {
	factory, ok := decoderFactories[name]
	if !ok {
		return nil, &UnsupportedFilterError{Filter: name}
	}
	return factory.CreateDecoder()
}

// UnsupportedFilterError indicates an unsupported filter type.
type UnsupportedFilterError struct {
	Filter entity.Name
}

// Error returns the error message.
func (e *UnsupportedFilterError) Error() string {
	return "unsupported filter: " + string(e.Filter)
}

// Filter names.
const (
	FilterASCIIHex  entity.Name = "ASCIIHexDecode"
	FilterASCII85   entity.Name = "ASCII85Decode"
	FilterLZW       entity.Name = "LZWDecode"
	FilterFlate     entity.Name = "FlateDecode"
	FilterRunLength entity.Name = "RunLengthDecode"
	FilterCCITTFax  entity.Name = "CCITTFaxDecode"
	FilterDCT       entity.Name = "DCTDecode"
	FilterJPX       entity.Name = "JPXDecode"
	FilterCrypt     entity.Name = "Crypt"
	FilterBrotli    entity.Name = "BrotliDecode"
)

// NewFromEntity creates an infrastructure Stream from an entity Stream.
func NewFromEntity(entityStream *entity.Stream) *Stream {
	return &Stream{
		dict: entityStream.Dict(),
		data: entityStream.RawBytes(),
	}
}
