// Package stream provides PDF stream handling and filtering.
package stream

import (
	"bytes"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// Stream represents a PDF stream with optional filters.
type Stream struct {
	dict    *entity.Dict
	data    []byte
	decoded []byte // Cached decoded data
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
	filterVal := s.dict.Get(entity.Name("Filter"))
	if filterVal == nil {
		// No filter, but may still have predictor to apply
		// Check for DecodeParms
		decodeParmsVal := s.dict.Get(entity.Name("DecodeParms"))
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

		data, err = decoder.Decode(data)
		if err != nil {
			return nil, err
		}

		// Apply predictor if specified in DecodeParms
		// Predictor is applied after the filter decompression
		if i < len(decodeParamsList) && decodeParamsList[i] != nil {
			params := decodeParamsList[i]
			if hasPredictor(params) {
				data, err = ApplyPredictor(data, params)
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
	decodeParamsList := make([]*entity.Dict, numFilters)

	// Get DecodeParms from stream dictionary
	decodeParmsVal := s.dict.Get(entity.Name("DecodeParms"))
	if decodeParmsVal == nil {
		return decodeParamsList, nil
	}

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
	val := params.Get(entity.Name("Predictor"))
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
