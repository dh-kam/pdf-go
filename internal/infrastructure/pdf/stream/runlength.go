package stream

import (
	"bytes"
)

func init() {
	RegisterDecoder(FilterRunLength, &RunLengthFactory{})
}

// RunLengthFactory creates RunLength decoders.
type RunLengthFactory struct{}

// CreateDecoder creates a new RunLength decoder.
func (f *RunLengthFactory) CreateDecoder() (Decoder, error) {
	return &RunLengthDecoder{}, nil
}

// RunLengthDecoder implements RunLength decoding.
type RunLengthDecoder struct{}

// Decode decodes RunLength-encoded data.
func (d *RunLengthDecoder) Decode(data []byte) ([]byte, error) {
	var result bytes.Buffer
	pos := 0

	for pos < len(data) {
		b := data[pos]
		pos++

		switch {
		case b < 128:
			// Literal run: copy next n+1 bytes
			runLen := int(b) + 1
			if pos+runLen > len(data) {
				runLen = len(data) - pos
			}
			result.Write(data[pos : pos+runLen])
			pos += runLen
		case b > 128:
			// Repeat run: repeat next byte (257-b) times
			runLen := 257 - int(b)
			if pos >= len(data) {
				break
			}
			repeatByte := data[pos]
			pos++
			for i := 0; i < runLen; i++ {
				result.WriteByte(repeatByte)
			}
		default:
			// b == 128 is EOD (End of Data) - stop processing
			return result.Bytes(), nil
		}
	}

	return result.Bytes(), nil
}
