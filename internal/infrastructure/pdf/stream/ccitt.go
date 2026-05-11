package stream

import (
	"bytes"
	"fmt"
	"io"

	"golang.org/x/image/ccitt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func init() {
	RegisterDecoder(FilterCCITTFax, &CCITTFaxFactory{})
}

// CCITTFaxFactory creates CCITTFax decoders.
type CCITTFaxFactory struct{}

// CreateDecoder creates a new CCITTFax decoder.
func (f *CCITTFaxFactory) CreateDecoder() (Decoder, error) {
	return &CCITTFaxDecoder{}, nil
}

// CCITTFaxDecoder implements CCITT Group 3/4 fax decoding.
type CCITTFaxDecoder struct {
	k                int
	columns          int
	rows             int
	blackIs1         bool
	encodedByteAlign bool
}

// Decode decodes CCITTFax-encoded data.
func (d *CCITTFaxDecoder) Decode(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty CCITT data")
	}

	decoded, err := d.decodeWithSubFormat(data, d.subFormat())
	if err == nil {
		return decoded, nil
	}

	// K>0 (mixed Group3 1D/2D) is not directly expressed by x/image/ccitt.
	// Try Group4 as a practical fallback before returning an error.
	if d.k > 0 {
		if fallback, fallbackErr := d.decodeWithSubFormat(data, ccitt.Group4); fallbackErr == nil {
			return fallback, nil
		}
	}

	return nil, fmt.Errorf(
		"ccitt decode failed (k=%d columns=%d rows=%d blackIs1=%t align=%t): %w",
		d.k,
		d.effectiveColumns(),
		d.effectiveRows(),
		d.blackIs1,
		d.encodedByteAlign,
		err,
	)
}

// SetDecodeParams sets CCITT-specific decode parameters.
func (d *CCITTFaxDecoder) SetDecodeParams(params *entity.Dict) {
	if params == nil {
		return
	}

	if value, ok := objectToInt(params.Get(entity.Name("K"))); ok {
		d.k = value
	}
	if value, ok := objectToInt(params.Get(entity.Name("Columns"))); ok {
		d.columns = value
	}
	if value, ok := objectToInt(params.Get(entity.Name("Rows"))); ok {
		d.rows = value
	}
	if value, ok := objectToBool(params.Get(entity.Name("BlackIs1"))); ok {
		d.blackIs1 = value
	}
	if value, ok := objectToBool(params.Get(entity.Name("EncodedByteAlign"))); ok {
		d.encodedByteAlign = value
	}
}

func (d *CCITTFaxDecoder) decodeWithSubFormat(data []byte, subFormat ccitt.SubFormat) ([]byte, error) {
	opts := &ccitt.Options{
		Align:  d.encodedByteAlign,
		Invert: d.blackIs1,
	}

	reader := ccitt.NewReader(
		bytes.NewReader(data),
		ccitt.MSB,
		subFormat,
		d.effectiveColumns(),
		d.effectiveRows(),
		opts,
	)

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

func (d *CCITTFaxDecoder) subFormat() ccitt.SubFormat {
	if d.k < 0 {
		return ccitt.Group4
	}
	return ccitt.Group3
}

func (d *CCITTFaxDecoder) effectiveColumns() int {
	if d.columns > 0 {
		return d.columns
	}
	return 1728
}

func (d *CCITTFaxDecoder) effectiveRows() int {
	if d.rows > 0 {
		return d.rows
	}
	return ccitt.AutoDetectHeight
}

func objectToInt(obj entity.Object) (int, bool) {
	switch value := obj.(type) {
	case *entity.Integer:
		return int(value.Value()), true
	case *entity.Real:
		return int(value.Value()), true
	default:
		return 0, false
	}
}

func objectToBool(obj entity.Object) (bool, bool) {
	switch value := obj.(type) {
	case *entity.Boolean:
		return value.Value(), true
	case *entity.Integer:
		return value.Value() != 0, true
	default:
		return false, false
	}
}

// NewCCITTFaxDecoder creates a CCITTFax decoder with parameters.
func NewCCITTFaxDecoder(k, columns, rows int, blackIs1 bool) *CCITTFaxDecoder {
	return &CCITTFaxDecoder{
		k:        k,
		columns:  columns,
		rows:     rows,
		blackIs1: blackIs1,
	}
}
