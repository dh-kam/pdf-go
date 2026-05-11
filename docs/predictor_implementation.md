# PDF Stream Predictor Implementation

## Overview
This implementation adds support for PDF stream predictors according to the PDF specification (ISO 32000-1, section 7.4.4.4). Predictors are used in PDF compression to improve compression ratios by predicting pixel values based on previous samples.

## Implementation Details

### Files Created/Modified

1. **`internal/infrastructure/pdf/stream/predictor.go`** (NEW)
   - Predictor interface and implementations
   - Support for predictors 1, 2, 10-15
   - DecodeParams extraction and validation
   - Helper functions for encoding/decoding

2. **`internal/infrastructure/pdf/stream/stream.go`** (MODIFIED)
   - Updated Decode() method to apply predictors
   - Support for DecodeParms with or without filters
   - Proper handling of predictor application after filter decompression

3. **`test/unit/stream/predictor_test.go`** (NEW)
   - Comprehensive tests for all predictor types
   - Edge case testing
   - Integration tests with stream decoder

## Supported Predictor Types

### Predictor 1: No Prediction
- Pass-through mode, no transformation
- Implemented by `NoPredictor`

### Predictor 2: TIFF Predictor
- Horizontal differencing (TIFF predictor 2)
- Works on byte-per-sample basis
- Implemented by `TIFFPredictor`

### Predictors 10-15: PNG Predictors
- Predictor 10: PNG encoding with filter byte per row
- Predictor 11: PNG None filter
- Predictor 12: PNG Sub filter
- Predictor 13: PNG Up filter
- Predictor 14: PNG Average filter
- Predictor 15: PNG Paeth filter
- Implemented by `PNGPredictor`

## API Usage

### Basic Decoding

```go
import "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"

// Get a predictor for a specific type
predictor, err := stream.GetPredictor(2) // TIFF predictor
if err != nil {
    return err
}

// Decode data with predictor
decoded, err := predictor.Decode(data, columns, colors, bitsPerComponent)
```

### Using with Stream Decoder

```go
// Stream decoder automatically applies predictors from DecodeParms
dict := entity.NewDict()
dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))
dict.Set(entity.Name("DecodeParms"), decodeParams)

s := stream.NewStream(dict, data)
decoded, err := s.Decode()
```

### Decode Parameters

```go
// Extract decode parameters from dictionary
params, err := stream.GetDecodeParams(decodeParmsDict)
if err != nil {
    return err
}

// Apply predictor with parameters
decoded, err := stream.ApplyPredictor(data, decodeParmsDict)
```

## Test Coverage

The implementation includes comprehensive tests for:

- All predictor types (1, 2, 10-15)
- Edge cases (empty data, single column, single row)
- Multi-color and multi-bit-per-component data
- Error handling (invalid parameters, mismatched data sizes)
- Integration with existing filter chain
- Round-trip encoding/decoding

## References

- PDF 1.7 specification, section 7.4.4.4 "LZW and Flate Prediction"
- PNG specification for PNG predictor algorithms
- TIFF 6.0 specification for TIFF predictor (TIFF predictor 2)

## Notes

- TIFF predictor works on byte-per-sample basis regardless of bitsPerComponent
- PNG predictors support bitsPerComponent from 1-16
- Predictor 10 reads filter byte from data; predictors 11-15 use fixed algorithm
- Stream decoder applies predictor after filter decompression
- Predictor is applied even when no filter is present (if DecodeParms specifies it)
