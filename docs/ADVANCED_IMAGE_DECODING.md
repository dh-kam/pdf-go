# Advanced Image Decoding Support

This document describes the advanced image decoding support for JPEG2000 and JBIG2 formats in the go-pdf library.

## Overview

The library supports decoding of advanced image formats commonly found in PDF documents:

- **JPEG2000 (JPX)**: ISO/IEC 15444-1 standard
- **JBIG2**: ISO/IEC 11544 standard

## Architecture

The advanced image decoding implementation follows a modular architecture:

```
internal/infrastructure/image/
├── decoder/
│   └── advanced.go          # Advanced decoder interface
├── jpx/
│   ├── decoder.go           # JPEG2000 decoder
│   └── wrapper.go           # Domain interface wrapper
├── jbig2/
│   ├── decoder.go           # JBIG2 decoder
│   └── wrapper.go           # Domain interface wrapper
└── decoder.go               # Main decoder (updated)

internal/infrastructure/cgo/
├── jpx/
│   ├── openjpeg.go          # CGo OpenJPEG wrapper
│   └── stub.go              # Stub implementation
└── jbig2/
    ├── jbig2dec.go          # CGo jbig2dec wrapper
    └── stub.go              # Stub implementation
```

## JPEG2000 (JPX) Support

### Features

- **JP2 File Format**: Complete JP2 file header parsing
  - Signature box detection
  - Image header box (ihdr) parsing
  - Color specification box (colr) parsing
- **JPEG 2000 Codestream**: Basic support for raw codestreams
- **Color Spaces**: Grayscale, RGB, and CMYK
- **Multiple Resolution Levels**: Box structure parsing

### Implementation Options

1. **Native Go Implementation** (default):
   - Basic JP2 file parsing
   - Placeholder image generation for stub
   - No external dependencies

2. **CGo OpenJPEG Wrapper** (optional):
   - Full JPEG2000 decoding via OpenJPEG library
   - Requires: `libopenjp2` development files
   - Build without: `-tags nojpx`

### Usage Example

```go
import (
    "github.com/dh-kam/pdf-go/internal/infrastructure/image/jpx"
)

// Create decoder
decoder := jpx.NewDecoder()

// Check format
if decoder.CanDecode(data) {
    // Decode
    img, err := decoder.Decode(data)
    if err != nil {
        // Handle error
    }
    // Use image
}
```

## JBIG2 Support

### Features

- **JBIG2 File Format**: File header and segment parsing
  - Signature detection
  - Segment structure parsing
  - Page information extraction
- **Embedded JBIG2**: Support for embedded data without file header
- **Segment Types**: Page information, text regions, generic regions
- **Compression**: Arithmetic coding and MMR placeholders

### Implementation Options

1. **Native Go Implementation** (default):
   - Basic JBIG2 file parsing
   - Segment structure detection
   - Placeholder image generation

2. **CGo jbig2dec Wrapper** (optional):
   - Full JBIG2 decoding via jbig2dec library
   - Requires: `jbig2dec` development files
   - Build without: `-tags nojbig2`

### Usage Example

```go
import (
    "github.com/dh-kam/pdf-go/internal/infrastructure/image/jbig2"
)

// Create decoder
decoder := jbig2.NewDecoder()

// Check format
if decoder.CanDecode(data) {
    // Decode
    img, err := decoder.Decode(data)
    if err != nil {
        // Handle error
    }
    // Use image
}
```

## Integration with Main Decoder

The advanced decoders are automatically registered with the main image decoder:

```go
import (
    "github.com/dh-kam/pdf-go/internal/infrastructure/image"
)

// Main decoder includes JPX and JBIG2 support
decoder := imageinfrastructure.NewDecoder()

// Decode PDF image data
imgData := &image.ImageData{
    Width:             100,
    Height:            100,
    ColorSpace:        image.ColorSpaceDeviceRGB,
    BitsPerComponent:  8,
    Filter:            image.FilterJPX, // or FilterJBIG2
    Data:              jpxData,
}

result, err := decoder.Decode(imgData)
```

## Build Tags

The implementation uses Go build tags to control CGo usage:

### Build with CGo Support (requires libraries)

```bash
go build -o bin/pdfinfo ./cmd/pdfinfo
```

### Build without CGo (stub implementations)

```bash
go build -tags='nojpx,nojbig2' -o bin/pdfinfo ./cmd/pdfinfo
```

### Makefile Targets

```bash
# Build with full CGo support
make build-all-cgo

# Build without CGo
make build-no-cgo

# Build specific targets
make build-pdfinfo
make build-pdftext
make build-pdfrender
```

## Format Detection

Both decoders provide format detection via magic byte signatures:

### JPEG2000 Signatures

- **JP2**: `0x0000000C 0x6A502020 0x0D0A870A`
- **Codestream**: `0xFF4F 0xFF51` (SOC marker + SIZ marker)

### JBIG2 Signatures

- **Standalone**: `0x974A4232 0x0D0A1A0A`
- **Embedded**: Segment structure detection

## API Reference

### Advanced Decoder Interface

```go
type AdvancedDecoder interface {
    Decode(data []byte) (image.Image, error)
    SupportedFormats() []string
    CanDecode(data []byte) bool
}
```

### JPEG2000 Decoder

```go
type Decoder struct {
    useCGo bool  // Automatically detected
}

func NewDecoder() *Decoder
func (d *Decoder) Decode(data []byte) (image.Image, error)
func (d *Decoder) DecodeConfig(data []byte) (image.Config, error)
func (d *Decoder) ColorSpace() image.ColorSpace
func (d *Decoder) CanDecode(data []byte) bool
func (d *Decoder) SupportedFormats() []string
```

### JBIG2 Decoder

```go
type Decoder struct {
    useCGo bool  // Automatically detected
}

func NewDecoder() *Decoder
func (d *Decoder) Decode(data []byte) (image.Image, error)
func (d *Decoder) DecodeConfig(data []byte) (image.Config, error)
func (d *Decoder) ColorSpace() image.ColorSpace
func (d *Decoder) CanDecode(data []byte) bool
func (d *Decoder) SupportedFormats() []string
```

## Testing

Comprehensive tests are provided for both decoders:

```bash
# Run tests without CGo
go test -tags='nojpx,nojbig2' ./test/unit/image/...

# Run specific tests
go test -run TestJPX ./test/unit/image/...
go test -run TestJBIG2 ./test/unit/image/...
```

## Performance Considerations

### Native Implementation

- **Pros**: No external dependencies, cross-platform
- **Cons**: Limited functionality, placeholder images
- **Use Case**: Applications that don't require full format support

### CGo Implementation

- **Pros**: Full format support, production-ready
- **Cons**: External library dependencies, build complexity
- **Use Case**: Production applications requiring complete format support

## Future Enhancements

Potential improvements for the native implementations:

1. **JPEG2000**:
   - Full codestream parsing
   - Tile and resolution level decoding
   - Progressive decoding support

2. **JBIG2**:
   - Arithmetic coding implementation
   - MMR compression decoding
   - Symbol dictionary handling
   - Text and generic region decoding

## Standards References

- [ISO/IEC 15444-1](https://www.iso.org/standard/78314.html) - JPEG 2000 image coding standard
- [ISO/IEC 11544](https://www.iso.org/standard/19078.html) - JBIG2 standard
- [TIFF 6.0](https://www.adobe.io/open/standards/TIFF.html) - For JBIG2 in TIFF
- [PDF 1.7 Specification](https://opensource.adobe.com/dc-acrobat/pdf-reference-pdf/) - Section 3.3.6 and 7.4.8

## License

This implementation follows the same license as the go-pdf project.

## Contributing

Contributions to improve the native implementations or CGo wrappers are welcome. Please ensure:

1. Tests pass for both CGo and non-CGO builds
2. Documentation is updated
3. Build tags are properly used
4. Backward compatibility is maintained
