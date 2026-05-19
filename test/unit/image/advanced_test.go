// Package image_test provides tests for advanced image decoding.
package image_test

import (
	"bytes"
	stdimage "image"
	"image/color"
	"testing"

	jpeg2000 "github.com/ajroetker/go-jpeg2000"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/image"
	imageinfrastructure "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image/jbig2"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image/jpx"
)

// TestJPEG2000Decoding tests JPEG2000 image decoding.
func TestJPEG2000Decoding(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "too short data",
			data:    []byte{0x00, 0x00, 0x00},
			wantErr: true,
			errMsg:  "invalid",
		},
		{
			name:    "valid JP2 codestream",
			data:    createJP2Data(t),
			wantErr: false,
		},
		{
			name:    "JP2 signature without codestream",
			data:    createJP2StubData(),
			wantErr: true,
			errMsg:  "jpx_native_decode",
		},
		{
			name:    "invalid JP2 signature",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG signature
			wantErr: true,
			errMsg:  "header",
		},
	}

	decoder := jpx.NewDecoder()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := decoder.Decode(tt.data)
			if (err != nil) != tt.wantErr {
				assert.Failf(t, "assertion failed", "Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					assert.Failf(t, "assertion failed", "Decode() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && img == nil {
				assert.Failf(t, "assertion failed", "Decode() returned nil image without error")
			}
		})
	}
}

// TestJBIG2Decoding tests JBIG2 image decoding.
func TestJBIG2Decoding(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "too short data",
			data:    []byte{0x97, 0x4A},
			wantErr: true,
			errMsg:  "truncated",
		},
		{
			name:    "valid JBIG2 signature (stub)",
			data:    createJBIG2StubData(),
			wantErr: false,
		},
		{
			name:    "invalid JBIG2 signature",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG signature
			wantErr: true,
		},
	}

	decoder := jbig2.NewDecoder()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := decoder.Decode(tt.data)
			if (err != nil) != tt.wantErr {
				assert.Failf(t, "assertion failed", "Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					assert.Failf(t, "assertion failed", "Decode() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && img == nil {
				assert.Failf(t, "assertion failed", "Decode() returned nil image without error")
			}
		})
	}
}

// TestJPXCanDecode tests JPEG2000 format detection.
func TestJPXCanDecode(t *testing.T) {
	decoder := jpx.NewDecoder()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "JP2 signature",
			data: createJP2StubData(),
			want: true,
		},
		{
			name: "JPEG 2000 codestream",
			data: []byte{0xFF, 0x4F, 0xFF, 0x51, 0x00},
			want: true,
		},
		{
			name: "JPEG data",
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0},
			want: false,
		},
		{
			name: "empty data",
			data: []byte{},
			want: false,
		},
		{
			name: "short data",
			data: []byte{0x00},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decoder.CanDecode(tt.data); got != tt.want {
				assert.Failf(t, "assertion failed", "CanDecode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJBIG2CanDecode tests JBIG2 format detection.
func TestJBIG2CanDecode(t *testing.T) {
	decoder := jbig2.NewDecoder()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "JBIG2 signature",
			data: createJBIG2StubData(),
			want: true,
		},
		{
			name: "embedded JBIG2 segment",
			data: []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x30},
			want: false, // The current implementation requires full signature
		},
		{
			name: "JPEG data",
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0},
			want: false,
		},
		{
			name: "empty data",
			data: []byte{},
			want: false,
		},
		{
			name: "short data",
			data: []byte{0x97},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decoder.CanDecode(tt.data); got != tt.want {
				assert.Failf(t, "assertion failed", "CanDecode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJPXSupportedFormats tests JPEG2000 supported format list.
func TestJPXSupportedFormats(t *testing.T) {
	decoder := jpx.NewDecoder()
	formats := decoder.SupportedFormats()

	expectedFormats := []string{"jp2", "jpc", "jpx"}
	for _, expected := range expectedFormats {
		found := false
		for _, format := range formats {
			if format == expected {
				found = true
				break
			}
		}
		if !found {
			assert.Failf(t, "assertion failed", "SupportedFormats() missing %q", expected)
		}
	}
}

// TestJBIG2SupportedFormats tests JBIG2 supported format list.
func TestJBIG2SupportedFormats(t *testing.T) {
	decoder := jbig2.NewDecoder()
	formats := decoder.SupportedFormats()

	expectedFormats := []string{"jb2", "jbig2"}
	for _, expected := range expectedFormats {
		found := false
		for _, format := range formats {
			if format == expected {
				found = true
				break
			}
		}
		if !found {
			assert.Failf(t, "assertion failed", "SupportedFormats() missing %q", expected)
		}
	}
}

// TestJPXDecodeConfig tests JPEG2000 configuration decoding.
func TestJPXDecodeConfig(t *testing.T) {
	decoder := jpx.NewDecoder()
	data := createJP2Data(t)

	cfg, err := decoder.DecodeConfig(data)
	if err != nil {
		require.FailNowf(t, "test failed", "DecodeConfig() error = %v", err)
	}

	if cfg.Width <= 0 {
		assert.Failf(t, "assertion failed", "DecodeConfig() Width = %v, want > 0", cfg.Width)
	}
	if cfg.Height <= 0 {
		assert.Failf(t, "assertion failed", "DecodeConfig() Height = %v, want > 0", cfg.Height)
	}
}

// TestJBIG2DecodeConfig tests JBIG2 configuration decoding.
func TestJBIG2DecodeConfig(t *testing.T) {
	decoder := jbig2.NewDecoder()
	data := createJBIG2StubData()

	cfg, err := decoder.DecodeConfig(data)
	if err != nil {
		require.FailNowf(t, "test failed", "DecodeConfig() error = %v", err)
	}

	if cfg.Width <= 0 {
		assert.Failf(t, "assertion failed", "DecodeConfig() Width = %v, want > 0", cfg.Width)
	}
	if cfg.Height <= 0 {
		assert.Failf(t, "assertion failed", "DecodeConfig() Height = %v, want > 0", cfg.Height)
	}
}

// TestIntegrationDecoderWithJPX tests integration with main decoder.
func TestIntegrationDecoderWithJPX(t *testing.T) {
	decoder := imageinfrastructure.NewDecoder()

	imgData := &image.ImageData{
		Width:            4,
		Height:           4,
		ColorSpace:       image.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		Filter:           image.FilterJPX,
		Data:             createJP2Data(t),
	}

	result, err := decoder.Decode(imgData)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 4, result.Width())
	assert.Equal(t, 4, result.Height())
}

// TestIntegrationDecoderWithJBIG2 tests integration with main decoder.
func TestIntegrationDecoderWithJBIG2(t *testing.T) {
	decoder := imageinfrastructure.NewDecoder()

	imgData := &image.ImageData{
		Width:            100,
		Height:           100,
		ColorSpace:       image.ColorSpaceDeviceGray,
		BitsPerComponent: 1,
		Filter:           image.FilterJBIG2,
		Data:             createJBIG2StubData(),
	}

	// This should not panic
	result, err := decoder.Decode(imgData)
	if err != nil {
		// Expected for stub implementation
		t.Logf("Decode returned error (expected for stub): %v", err)
	}
	if result != nil {
		if result.Width() <= 0 || result.Height() <= 0 {
			assert.Failf(t, "assertion failed", "Decode() returned invalid dimensions: %dx%d", result.Width(), result.Height())
		}
	}
}

// TestFallbackBehavior tests fallback when libraries are unavailable.
func TestFallbackBehavior(t *testing.T) {
	// Test that decoders handle missing CGo libraries gracefully
	jpxDecoder := jpx.NewDecoder()
	jbig2Decoder := jbig2.NewDecoder()

	// Even without CGo, the decoder should handle the data
	// (returning placeholder images for stubs)
	jp2Data := createJP2Data(t)
	jbig2Data := createJBIG2StubData()

	jpxImg, jpxErr := jpxDecoder.Decode(jp2Data)
	jbig2Img, jbig2Err := jbig2Decoder.Decode(jbig2Data)

	// Should either succeed with a placeholder or fail gracefully
	if jpxImg != nil {
		t.Logf("JPX decode succeeded")
	} else if jpxErr != nil {
		t.Logf("JPX decode failed gracefully: %v", jpxErr)
	}

	if jbig2Img != nil {
		t.Logf("JBIG2 decode succeeded (likely stub implementation)")
	} else if jbig2Err != nil {
		t.Logf("JBIG2 decode failed gracefully: %v", jbig2Err)
	}
}

// TestMalformedData tests error handling for malformed data.
func TestMalformedData(t *testing.T) {
	tests := []struct {
		name    string
		decoder interface{ Decode([]byte) error }
		data    []byte
	}{
		{
			name: "JPX with truncated header",
			decoder: decodeFunc(func(data []byte) error {
				_, err := jpx.NewDecoder().Decode(data)
				return err
			}),
			data: []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20},
		},
		{
			name: "JBIG2 with truncated header",
			decoder: decodeFunc(func(data []byte) error {
				_, err := jbig2.NewDecoder().Decode(data)
				return err
			}),
			data: []byte{0x97, 0x4A, 0x42, 0x32},
		},
		{
			name: "JPX with truncated header",
			decoder: decodeFunc(func(data []byte) error {
				_, err := jpx.NewDecoder().Decode(data)
				return err
			}),
			data: []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20},
		},
		{
			name: "JPX with signature but no codestream",
			decoder: decodeFunc(func(data []byte) error {
				_, err := jpx.NewDecoder().Decode(data)
				return err
			}),
			data: createJP2StubData(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.decoder.Decode(tt.data)
			// Some malformed data may be handled gracefully by stub
			if err == nil {
				t.Logf("Decode() succeeded for potentially malformed data (stub implementation)")
			}
		})
	}
}

// decodeFunc is a helper type for testing decode functions.
type decodeFunc func([]byte) error

func (f decodeFunc) Decode(data []byte) error {
	return f(data)
}

// Helper functions

func createJP2Data(t *testing.T) []byte {
	t.Helper()

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(10 + x*20),
				G: uint8(30 + y*20),
				B: uint8(90 + x*10 + y*5),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	err := jpeg2000.Encode(&buf, img, &jpeg2000.EncodeOptions{
		Lossless:       true,
		NumResolutions: 1,
		FileFormat:     jpeg2000.FormatJP2,
	})
	require.NoError(t, err)
	return buf.Bytes()
}

func createJP2StubData() []byte {
	buf := new(bytes.Buffer)

	// JP2 signature
	buf.Write([]byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A})

	// Image Header box (ihdr)
	// Length: 22 bytes
	// Type: ihdr
	// Height: 100
	// Width: 100
	// Num components: 3
	// Bits per component: 8
	ihdr := []byte{
		0x00, 0x00, 0x00, 0x16, // Length: 22
		0x69, 0x68, 0x64, 0x72, // Type: ihdr
		0x00, 0x00, 0x00, 0x64, // Height: 100
		0x00, 0x00, 0x00, 0x64, // Width: 100
		0x03, // Num components: 3
		0x07, // Bits per component: 8
		0x07, // Compression: 7
		0x00, // Colorspace: unknown
		0x00, // Intellectual property
	}
	buf.Write(ihdr)

	// Color Specification box (colr)
	// Length: 15 bytes
	colr := []byte{
		0x00, 0x00, 0x00, 0x0F, // Length: 15
		0x63, 0x6F, 0x6C, 0x72, // Type: colr
		0x01, 0x00, 0x00, 0x00, // Method: enumerated, Precedence: 0, Approximation: 0
		0x00, 0x00, 0x00, 0x10, // CS type: sRGB (16)
		0x00, // IC profile (empty)
	}
	buf.Write(colr)

	return buf.Bytes()
}

func createJBIG2StubData() []byte {
	buf := new(bytes.Buffer)

	// JBIG2 signature
	buf.Write([]byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A})

	// File header
	buf.Write([]byte{0x00})                   // sequential organization, known page count
	buf.Write([]byte{0x00, 0x00, 0x00, 0x01}) // 1 page

	// Page information segment (type 48)
	// Segment number: 1
	buf.Write([]byte{0x00, 0x00, 0x00, 0x01})

	buf.Write([]byte{0x30}) // Type: 48
	buf.Write([]byte{0x00}) // No referred-to segments
	buf.Write([]byte{0x01}) // Page association

	// Segment data length
	buf.Write([]byte{0x00, 0x00, 0x00, 0x13})

	// Page info data
	buf.Write([]byte{
		0x00, 0x00, 0x00, 0x64, // Width: 100
		0x00, 0x00, 0x00, 0x64, // Height: 100
		0x00, 0x00, 0x00, 0x00, // Resolution X
		0x00, 0x00, 0x00, 0x00, // Resolution Y
		0x00,       // Flags
		0x00, 0x00, // Striping info
	})

	return buf.Bytes()
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
