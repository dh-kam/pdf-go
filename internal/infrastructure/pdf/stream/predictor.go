// Package stream provides PDF stream handling and filtering.
package stream

import (
	"errors"
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

var (
	// ErrInvalidPredictor indicates an unsupported predictor type.
	ErrInvalidPredictor = errors.New("invalid predictor type")
	// ErrInvalidParameters indicates invalid predictor parameters.
	ErrInvalidParameters = errors.New("invalid predictor parameters")
	// ErrInvalidDataSize indicates the data size doesn't match expected dimensions.
	ErrInvalidDataSize = errors.New("invalid data size for given parameters")
)

// Predictor represents a predictor decoder interface.
type Predictor interface {
	// Decode decodes predictor-encoded data.
	Decode(data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error)
}

// DecodeParams represents predictor decode parameters from DecodeParms dictionary.
type DecodeParams struct {
	Predictor        int // Predictor type (1, 2, 10-15)
	Columns          int // Number of samples per row
	Colors           int // Number of color components per sample (default 1)
	BitsPerComponent int // Bits per component (default 8)
}

// GetDecodeParams extracts decode parameters from a dictionary.
// If params is nil, returns default values.
// If params is a dictionary, extracts Predictor, Columns, Colors, and BitsPerComponent.
func GetDecodeParams(params *entity.Dict) (*DecodeParams, error) {
	dp := &DecodeParams{
		Predictor:        1, // No prediction
		Columns:          1, // Default per PDF spec
		Colors:           1, // Default per PDF spec
		BitsPerComponent: 8, // Default per PDF spec
	}

	if params == nil {
		return dp, nil
	}

	// Extract Predictor
	if val := params.Get(entity.Name("Predictor")); val != nil {
		if integer, ok := val.(*entity.Integer); ok {
			dp.Predictor = int(integer.Value())
		}
	}

	// Extract Columns
	if val := params.Get(entity.Name("Columns")); val != nil {
		if integer, ok := val.(*entity.Integer); ok {
			dp.Columns = int(integer.Value())
		}
	}

	// Extract Colors
	if val := params.Get(entity.Name("Colors")); val != nil {
		if integer, ok := val.(*entity.Integer); ok {
			dp.Colors = int(integer.Value())
		}
	}

	// Extract BitsPerComponent
	if val := params.Get(entity.Name("BitsPerComponent")); val != nil {
		if integer, ok := val.(*entity.Integer); ok {
			dp.BitsPerComponent = int(integer.Value())
		}
	}

	// Validate parameters
	if dp.Columns <= 0 {
		return nil, fmt.Errorf("%w: columns must be positive", ErrInvalidParameters)
	}
	if dp.Colors <= 0 {
		return nil, fmt.Errorf("%w: colors must be positive", ErrInvalidParameters)
	}
	if dp.BitsPerComponent <= 0 || dp.BitsPerComponent > 16 {
		return nil, fmt.Errorf("%w: bitsPerComponent must be 1-16", ErrInvalidParameters)
	}

	return dp, nil
}

// GetPredictor returns a predictor decoder for the given type.
func GetPredictor(predictorType int) (Predictor, error) {
	switch predictorType {
	case 1:
		return &NoPredictor{}, nil
	case 2:
		return &TIFFPredictor{}, nil
	case 10, 11, 12, 13, 14, 15:
		// Predictor 10 uses PNG predictor with algorithm from filter byte
		// Predictors 11-15 directly specify PNG algorithm (no filter byte)
		return &PNGPredictor{predictorType: predictorType}, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidPredictor, predictorType)
	}
}

// NoPredictor implements predictor 1 (no prediction).
type NoPredictor struct{}

// Decode returns the data unchanged for predictor 1.
func (p *NoPredictor) Decode(data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	// Predictor 1 is "no prediction" - pass through unchanged
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// TIFFPredictor implements predictor 2 (TIFF predictor 2 - horizontal differencing).
type TIFFPredictor struct{}

// Decode decodes TIFF predictor 2 (horizontal differencing) encoded data.
// TIFF predictor works on a byte-per-sample basis, regardless of bitsPerComponent.
// It assumes each component is stored as a full byte (8 bits).
func (p *TIFFPredictor) Decode(data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// TIFF predictor works on bytes, so we need to calculate bytes per row
	// For TIFF predictor, each sample is assumed to be a full byte
	bytesPerRow := columns * colors

	if len(data)%bytesPerRow != 0 {
		return nil, fmt.Errorf("%w: data length %d is not a multiple of row size %d",
			ErrInvalidDataSize, len(data), bytesPerRow)
	}

	result := make([]byte, len(data))
	copy(result, data)

	// Apply horizontal differencing reversal
	// Each byte is decoded as: original[i] = encoded[i] + original[i-1]
	// The first byte of each component sample is not predicted
	rows := len(data) / bytesPerRow
	for row := 0; row < rows; row++ {
		rowOffset := row * bytesPerRow
		for col := 1; col < bytesPerRow; col++ {
			result[rowOffset+col] += result[rowOffset+col-1]
		}
	}

	return result, nil
}

// PNGPredictor implements PNG prediction (predictors 10-15).
type PNGPredictor struct {
	predictorType int // 10=read filter byte, 11=None, 12=Sub, 13=Up, 14=Average, 15=Paeth
}

// Decode decodes PNG predictor encoded data.
// PNG prediction uses a filter byte per row followed by the filtered data.
// For predictor type 10, the filter byte is included in the data.
// For predictor types 11-15, the filter byte is NOT included (algorithm is fixed).
func (p *PNGPredictor) Decode(data []byte, columns int, colors int, bitsPerComponent int) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Calculate bytes per row
	bytesPerSample := (bitsPerComponent + 7) / 8
	samplesPerRow := columns * colors
	bytesPerRow := samplesPerRow * bytesPerSample

	if bytesPerRow == 0 {
		return nil, fmt.Errorf("%w: bytes per row is zero", ErrInvalidParameters)
	}

	// For predictor type 10, data includes filter byte per row
	// For predictor types 11-15, data does NOT include filter byte (algorithm is fixed)
	hasFilterByte := (p.predictorType == 10)

	var totalRowSize int
	if hasFilterByte {
		totalRowSize = bytesPerRow + 1 // +1 for filter byte
	} else {
		totalRowSize = bytesPerRow
	}

	if len(data)%totalRowSize != 0 {
		return nil, fmt.Errorf("%w: data length %d is not a multiple of row size %d",
			ErrInvalidDataSize, len(data), totalRowSize)
	}

	rows := len(data) / totalRowSize
	result := make([]byte, rows*bytesPerRow)

	for row := 0; row < rows; row++ {
		rowOffset := row * totalRowSize
		var rowData []byte
		resultOffset := row * bytesPerRow

		// If algorithm is specified (1-4) from predictor type 12-15, use it
		// Otherwise use the filter byte from the data (predictor 10)
		algorithm := 0
		if hasFilterByte {
			// Predictor type 10: read filter byte from data
			filterByte := data[rowOffset]
			rowData = data[rowOffset+1 : rowOffset+1+bytesPerRow]
			algorithm = int(filterByte)
		} else {
			// Predictor types 11-15: algorithm is fixed, no filter byte
			rowData = data[rowOffset : rowOffset+bytesPerRow]
			algorithm = p.predictorType - 11 // 11->0, 12->1, 13->2, 14->3, 15->4
		}

		var err error
		switch algorithm {
		case 0: // None
			err = p.decodeNone(rowData, result[resultOffset:resultOffset+bytesPerRow])
		case 1: // Sub
			err = p.decodeSub(rowData, result[resultOffset:resultOffset+bytesPerRow], bytesPerSample)
		case 2: // Up
			err = p.decodeUp(rowData, result[resultOffset:resultOffset+bytesPerRow], result[:resultOffset])
		case 3: // Average
			err = p.decodeAverage(rowData, result[resultOffset:resultOffset+bytesPerRow], bytesPerSample, result[:resultOffset])
		case 4: // Paeth
			err = p.decodePaeth(rowData, result[resultOffset:resultOffset+bytesPerRow], bytesPerSample, result[:resultOffset])
		default:
			return nil, fmt.Errorf("%w: unknown PNG filter type %d", ErrInvalidPredictor, algorithm)
		}

		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// decodeNone implements PNG None filter (filter type 0).
func (p *PNGPredictor) decodeNone(rowData, result []byte) error {
	copy(result, rowData)
	return nil
}

// decodeSub implements PNG Sub filter (filter type 1).
// Sub(x) = Raw(x) + Raw(x - bytesPerSample)
func (p *PNGPredictor) decodeSub(rowData, result []byte, bytesPerSample int) error {
	bytesPerRow := len(rowData)
	if len(result) != bytesPerRow {
		return fmt.Errorf("%w: row size mismatch", ErrInvalidDataSize)
	}

	for i := 0; i < bytesPerRow; i++ {
		if i < bytesPerSample {
			result[i] = rowData[i]
		} else {
			result[i] = rowData[i] + result[i-bytesPerSample]
		}
	}
	return nil
}

// decodeUp implements PNG Up filter (filter type 2).
// Up(x) = Raw(x) + Prior(x)
// where Prior(x) is the corresponding byte from the previous row.
func (p *PNGPredictor) decodeUp(rowData, result, prevRow []byte) error {
	bytesPerRow := len(rowData)
	if len(result) != bytesPerRow {
		return fmt.Errorf("%w: row size mismatch", ErrInvalidDataSize)
	}

	if len(prevRow) == 0 {
		// First row - no previous row, treat as None
		copy(result, rowData)
	} else {
		for i := 0; i < bytesPerRow; i++ {
			result[i] = rowData[i] + prevRow[i]
		}
	}
	return nil
}

// decodeAverage implements PNG Average filter (filter type 3).
// Average(x) = Raw(x) + floor((Raw(x - bytesPerSample) + Prior(x)) / 2)
func (p *PNGPredictor) decodeAverage(rowData, result []byte, bytesPerSample int, prevRow []byte) error {
	bytesPerRow := len(rowData)
	if len(result) != bytesPerRow {
		return fmt.Errorf("%w: row size mismatch", ErrInvalidDataSize)
	}

	for i := 0; i < bytesPerRow; i++ {
		var left byte
		if i >= bytesPerSample {
			left = result[i-bytesPerSample]
		}

		var up byte
		if len(prevRow) > 0 {
			up = prevRow[i]
		}

		avg := (int(left) + int(up)) / 2
		result[i] = rowData[i] + byte(avg)
	}
	return nil
}

// decodePaeth implements PNG Paeth filter (filter type 4).
// Paeth(x) = Raw(x) + PaethPredictor(Raw(x - bytesPerSample), Prior(x), Prior(x - bytesPerSample))
func (p *PNGPredictor) decodePaeth(rowData, result []byte, bytesPerSample int, prevRow []byte) error {
	bytesPerRow := len(rowData)
	if len(result) != bytesPerRow {
		return fmt.Errorf("%w: row size mismatch", ErrInvalidDataSize)
	}

	for i := 0; i < bytesPerRow; i++ {
		var left byte
		if i >= bytesPerSample {
			left = result[i-bytesPerSample]
		}

		var up byte
		var upLeft byte
		if len(prevRow) > 0 {
			up = prevRow[i]
			if i >= bytesPerSample {
				upLeft = prevRow[i-bytesPerSample]
			}
		}

		predictor := paeth(left, up, upLeft)
		result[i] = rowData[i] + predictor
	}
	return nil
}

// paeth implements the Paeth predictor function from PNG specification.
func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := abs(p - int(a))
	pb := abs(p - int(b))
	pc := abs(p - int(c))

	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	default:
		return c
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ApplyPredictor applies the predictor to decoded data.
// This is a convenience function that extracts decode params and applies the predictor.
func ApplyPredictor(data []byte, params *entity.Dict) ([]byte, error) {
	decodeParams, err := GetDecodeParams(params)
	if err != nil {
		return nil, err
	}

	if decodeParams.Predictor == 1 {
		// No prediction - return data as-is
		return data, nil
	}

	predictor, err := GetPredictor(decodeParams.Predictor)
	if err != nil {
		return nil, err
	}

	return predictor.Decode(data, decodeParams.Columns, decodeParams.Colors, decodeParams.BitsPerComponent)
}

// ApplyPredictorWithParams applies the predictor with explicit decode parameters.
func ApplyPredictorWithParams(data []byte, params *DecodeParams) ([]byte, error) {
	if params.Predictor == 1 {
		// No prediction - return data as-is
		return data, nil
	}

	predictor, err := GetPredictor(params.Predictor)
	if err != nil {
		return nil, err
	}

	return predictor.Decode(data, params.Columns, params.Colors, params.BitsPerComponent)
}

// EncodeWithTIFFPredictor applies TIFF predictor 2 encoding (horizontal differencing).
// This is provided for completeness - typically only decoding is needed for PDF reading.
func EncodeWithTIFFPredictor(data []byte, columns int, colors int) []byte {
	if len(data) == 0 {
		return data
	}

	bytesPerRow := columns * colors
	result := make([]byte, len(data))

	rows := len(data) / bytesPerRow
	for row := 0; row < rows; row++ {
		rowOffset := row * bytesPerRow
		// First byte stays the same
		result[rowOffset] = data[rowOffset]
		// Subsequent bytes are the difference from the previous byte
		for col := 1; col < bytesPerRow; col++ {
			result[rowOffset+col] = data[rowOffset+col] - data[rowOffset+col-1]
		}
	}

	return result
}

// EncodeWithPNGPredictor applies PNG prediction encoding.
// This is provided for completeness - typically only decoding is needed for PDF reading.
func EncodeWithPNGPredictor(data []byte, columns int, colors int, bitsPerComponent int, algorithm int) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if algorithm < 0 || algorithm > 4 {
		return nil, fmt.Errorf("%w: invalid PNG filter algorithm %d", ErrInvalidPredictor, algorithm)
	}

	bytesPerSample := (bitsPerComponent + 7) / 8
	samplesPerRow := columns * colors
	bytesPerRow := samplesPerRow * bytesPerSample

	if len(data)%bytesPerRow != 0 {
		return nil, fmt.Errorf("%w: data length %d is not a multiple of row size %d",
			ErrInvalidDataSize, len(data), bytesPerRow)
	}

	rows := len(data) / bytesPerRow
	result := make([]byte, rows*(bytesPerRow+1))

	prevRow := make([]byte, bytesPerRow)

	for row := 0; row < rows; row++ {
		rowData := data[row*bytesPerRow : (row+1)*bytesPerRow]
		rowOffset := row * (bytesPerRow + 1)
		filterOffset := rowOffset
		dataOffset := rowOffset + 1

		result[filterOffset] = byte(algorithm)

		switch algorithm {
		case 0: // None
			copy(result[dataOffset:], rowData)
		case 1: // Sub
			for i := 0; i < bytesPerRow; i++ {
				if i < bytesPerSample {
					result[dataOffset+i] = rowData[i]
				} else {
					result[dataOffset+i] = rowData[i] - rowData[i-bytesPerSample]
				}
			}
		case 2: // Up
			for i := 0; i < bytesPerRow; i++ {
				result[dataOffset+i] = rowData[i] - prevRow[i]
			}
		case 3: // Average
			for i := 0; i < bytesPerRow; i++ {
				var left byte
				if i >= bytesPerSample {
					left = rowData[i-bytesPerSample]
				}
				avg := (int(left) + int(prevRow[i])) / 2
				result[dataOffset+i] = rowData[i] - byte(avg)
			}
		case 4: // Paeth
			for i := 0; i < bytesPerRow; i++ {
				var left byte
				if i >= bytesPerSample {
					left = rowData[i-bytesPerSample]
				}
				var upLeft byte
				if i >= bytesPerSample {
					upLeft = prevRow[i-bytesPerSample]
				}
				predictor := paeth(left, prevRow[i], upLeft)
				result[dataOffset+i] = rowData[i] - predictor
			}
		}

		// Update prevRow for next iteration
		copy(prevRow, rowData)
	}

	return result, nil
}

// CreateDecodeParams creates a DecodeParams dictionary from the given parameters.
func CreateDecodeParams(predictor, columns, colors, bitsPerComponent int) *entity.Dict {
	dict := entity.NewDict()
	dict.Set(entity.Name("Predictor"), entity.NewInteger(int64(predictor)))
	dict.Set(entity.Name("Columns"), entity.NewInteger(int64(columns)))
	dict.Set(entity.Name("Colors"), entity.NewInteger(int64(colors)))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(int64(bitsPerComponent)))
	return dict
}
