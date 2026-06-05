package image

import (
	"bytes"
	"compress/flate"
	"compress/lzw"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
)

type stubDecoder struct {
	wasUsed bool
}

type testSeparationMapper struct{}

type testRGBMapper struct{}

func (s *stubDecoder) Decode(data []byte) (image.Image, error) {
	s.wasUsed = true
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}

func (s *stubDecoder) DecodeConfig(_ []byte) (image.Config, error) {
	return image.Config{Width: 1, Height: 1}, nil
}

func (s *stubDecoder) ColorSpace() domainimage.ColorSpace {
	return domainimage.ColorSpaceDeviceRGB
}

func (testSeparationMapper) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) == 0 {
		return color.RGBA{A: 255}
	}
	v := uint8(values[0] * 255)
	return color.RGBA{R: v, A: 255}
}

func (testSeparationMapper) ConvertImageTintToRGBA(tint float64) color.RGBA {
	v := uint8(tint*255 + 0.5)
	return color.RGBA{R: v, A: 255}
}

func (testSeparationMapper) GetNumComponents() int {
	return 1
}

func (testRGBMapper) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) < 3 {
		return color.RGBA{A: 255}
	}
	return color.RGBA{
		R: uint8(values[2]*255 + 0.5),
		G: uint8(values[1]*255 + 0.5),
		B: uint8(values[0]*255 + 0.5),
		A: 255,
	}
}

func (testRGBMapper) GetNumComponents() int {
	return 3
}

func testJBIG2EndOfFileSegment() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x01, // segment number
		0x33,                   // end-of-file segment
		0x00,                   // no referred-to segments
		0x00,                   // global page association
		0x00, 0x00, 0x00, 0x00, // segment data length
	}
}

func TestDecoder_NewDecoder_RegistersDefaultCodecs(t *testing.T) {
	d := NewDecoder()
	require.NotNil(t, d)

	// JPEG codec should be available by default.
	_, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterDCT,
		Data:             jpegTestData(t),
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
	})
	require.NoError(t, err)
}

func TestScaleSampleToBytePopplerImageByteLookupGate(t *testing.T) {
	assert.Equal(t, uint8(25), scaleSampleToByte(0.1, 1))

	t.Setenv("PDF_DEBUG_IMAGE_BYTE_LOOKUP_QUANT", "1")
	assert.Equal(t, uint8(26), scaleSampleToByte(0.1, 1))
	assert.Equal(t, uint8(0), scaleSampleToByte(-1, 1))
	assert.Equal(t, uint8(255), scaleSampleToByte(2, 1))
}

func TestDecoder_DecodeJBIG2UsesPDFDimensionsForNativeFallback(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterJBIG2,
		Data:             testJBIG2EndOfFileSegment(),
		Width:            13,
		Height:           7,
		BitsPerComponent: 1,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Equal(t, 13, img.Width())
	assert.Equal(t, 7, img.Height())
}

func TestDecoder_DecodeRawSeparationImageAppliesDecodeBeforeTint(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0, 128, 255},
		Width:            3,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceSeparation,
		ColorMapper:      testSeparationMapper{},
		Decode:           []float64{1, 0},
	})
	require.NoError(t, err)

	p0 := color.RGBAModel.Convert(img.Image().At(0, 0)).(color.RGBA)
	p1 := color.RGBAModel.Convert(img.Image().At(1, 0)).(color.RGBA)
	p2 := color.RGBAModel.Convert(img.Image().At(2, 0)).(color.RGBA)
	assert.Equal(t, uint8(255), p0.R)
	assert.Equal(t, uint8(127), p1.R)
	assert.Equal(t, uint8(0), p2.R)
}

func TestDecoder_DecodeRawCalRGBImageUsesColorMapper(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{10, 20, 30},
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceCalRGB,
		ColorMapper:      testRGBMapper{},
		Decode:           []float64{0, 1, 0, 1, 0, 1},
	})
	require.NoError(t, err)

	p := color.RGBAModel.Convert(img.Image().At(0, 0)).(color.RGBA)
	assert.Equal(t, color.RGBA{R: 30, G: 20, B: 10, A: 255}, p)
}

func TestParseICCChannelCurvesSkipsSRGBEquivalentMatrixProfile(t *testing.T) {
	profile := testSRGBEquivalentMatrixProfile()
	require.NotContains(t, string(profile), "sRGB")
	require.NotContains(t, string(profile), "IEC61966")

	curves, ok := parseICCChannelCurves(profile, 3)
	assert.False(t, ok)
	assert.Nil(t, curves)
}

func TestPopplerRGBICCPostScaleTransformMatchesLCMSSRGBSamples(t *testing.T) {
	t.Setenv("PDF_DEBUG_SPLASH_ENABLE_ICC_POST_SCALE", "")
	t.Setenv("PDF_DEBUG_SPLASH_DISABLE_ICC_POST_SCALE", "")

	transform, ok := newPopplerRGBICCTransform(&domainimage.ImageData{
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
		BitsPerComponent: 8,
		ICCProfile:       testSRGBEquivalentMatrixProfile(),
		ICCComponents:    3,
		Filter:           domainimage.FilterFlate,
	})
	require.True(t, ok)

	tests := []struct {
		name string
		in   color.RGBA
		want color.RGBA
	}{
		{name: "green edge", in: color.RGBA{R: 10, G: 255, B: 0, A: 255}, want: color.RGBA{R: 11, G: 255, B: 0, A: 255}},
		{name: "cyan low red", in: color.RGBA{R: 0, G: 255, B: 81, A: 255}, want: color.RGBA{R: 1, G: 255, B: 81, A: 255}},
		{name: "cyan blue step", in: color.RGBA{R: 0, G: 255, B: 91, A: 255}, want: color.RGBA{R: 1, G: 255, B: 91, A: 255}},
		{name: "cyan high blue preserved", in: color.RGBA{R: 0, G: 255, B: 102, A: 255}, want: color.RGBA{R: 0, G: 255, B: 102, A: 255}},
		{name: "green 254 low blue", in: color.RGBA{R: 0, G: 254, B: 60, A: 255}, want: color.RGBA{R: 1, G: 254, B: 60, A: 255}},
		{name: "green 254 high blue preserved", in: color.RGBA{R: 0, G: 254, B: 70, A: 255}, want: color.RGBA{R: 0, G: 254, B: 70, A: 255}},
		{name: "red boundary", in: color.RGBA{R: 1, G: 255, B: 81, A: 255}, want: color.RGBA{R: 2, G: 255, B: 81, A: 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b := transform.ApplyRGB(tt.in.R, tt.in.G, tt.in.B)
			assert.Equal(t, tt.want, color.RGBA{R: r, G: g, B: b, A: 255})
		})
	}
}

func TestParseICCParametricCurveUsesSpecThresholds(t *testing.T) {
	tests := []struct {
		name   string
		typ    uint16
		params []float64
		input  float64
		want   float64
	}{
		{name: "type1 below threshold", typ: 1, params: []float64{2, 1, -0.25}, input: 0.2, want: 0},
		{name: "type1 above threshold", typ: 1, params: []float64{2, 1, -0.25}, input: 0.5, want: 0.0625},
		{name: "type2 below threshold", typ: 2, params: []float64{2, 1, -0.25, 0.1}, input: 0.2, want: 0.1},
		{name: "type2 above threshold", typ: 2, params: []float64{2, 1, -0.25, 0.1}, input: 0.5, want: 0.1625},
		{name: "type3 below d", typ: 3, params: []float64{2, 1, 0, 0.5, 0.2}, input: 0.1, want: 0.05},
		{name: "type3 above d", typ: 3, params: []float64{2, 1, 0, 0.5, 0.2}, input: 0.5, want: 0.25},
		{name: "type4 below d", typ: 4, params: []float64{2, 1, 0, 0.5, 0.2, 0.1, 0.03}, input: 0.1, want: 0.08},
		{name: "type4 above d", typ: 4, params: []float64{2, 1, 0, 0.5, 0.2, 0.1, 0.03}, input: 0.5, want: 0.35},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			curve := parseICCParametricCurve(testParametricTRCTag(tt.typ, tt.params...))
			require.NotNil(t, curve)
			assert.InDelta(t, tt.want, curve(tt.input), 0.0001)
		})
	}
}

func TestDecoder_DecodeNilData(t *testing.T) {
	d := NewDecoder()
	_, err := d.Decode(nil)
	require.Error(t, err)
}

func testSRGBEquivalentMatrixProfile() []byte {
	tags := []struct {
		name string
		data []byte
	}{
		{name: "rXYZ", data: testXYZTag(0.4360657, 0.2224884, 0.0139160)},
		{name: "gXYZ", data: testXYZTag(0.3851471, 0.7168732, 0.0970764)},
		{name: "bXYZ", data: testXYZTag(0.1430664, 0.0606079, 0.7140961)},
		{name: "wtpt", data: testXYZTag(0.964203, 1, 0.824905)},
		{name: "rTRC", data: testSRGBParametricTRCTag()},
		{name: "gTRC", data: testSRGBParametricTRCTag()},
		{name: "bTRC", data: testSRGBParametricTRCTag()},
	}

	tagTableSize := 128 + 4 + len(tags)*12
	total := tagTableSize
	for _, tag := range tags {
		total += len(tag.data)
	}
	profile := make([]byte, total)
	copy(profile[16:20], []byte("RGB "))
	copy(profile[20:24], []byte("XYZ "))
	copy(profile[36:40], []byte("acsp"))
	binary.BigEndian.PutUint32(profile[128:132], uint32(len(tags)))

	dataOffset := tagTableSize
	for i, tag := range tags {
		record := 132 + i*12
		copy(profile[record:record+4], []byte(tag.name))
		binary.BigEndian.PutUint32(profile[record+4:record+8], uint32(dataOffset))
		binary.BigEndian.PutUint32(profile[record+8:record+12], uint32(len(tag.data)))
		copy(profile[dataOffset:dataOffset+len(tag.data)], tag.data)
		dataOffset += len(tag.data)
	}

	return profile
}

func testXYZTag(x, y, z float64) []byte {
	data := make([]byte, 20)
	copy(data[0:4], []byte("XYZ "))
	putS15Fixed16(data[8:12], x)
	putS15Fixed16(data[12:16], y)
	putS15Fixed16(data[16:20], z)
	return data
}

func testSRGBParametricTRCTag() []byte {
	return testParametricTRCTag(4,
		2.4,
		1.0/1.055,
		0.055/1.055,
		1.0/12.92,
		0.04045,
		0,
		0,
	)
}

func testParametricTRCTag(typ uint16, params ...float64) []byte {
	data := make([]byte, 12+len(params)*4)
	copy(data[0:4], []byte("para"))
	binary.BigEndian.PutUint16(data[8:10], typ)
	for i, param := range params {
		putS15Fixed16(data[12+i*4:16+i*4], param)
	}
	return data
}

func putS15Fixed16(dst []byte, value float64) {
	raw := int32(math.Round(value * 65536))
	binary.BigEndian.PutUint32(dst, uint32(raw))
}

func TestDecoder_UsesProvidedColorSpaceForDCT(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterDCT,
		Data:             jpegTestGrayData(t),
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Equal(t, domainimage.ColorSpaceDeviceGray, img.ColorSpace())
}

func TestDecoder_DecodeGrayImage(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x80},
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Equal(t, 1, img.Width())
	assert.Equal(t, 1, img.Height())
	assert.Equal(t, domainimage.ColorSpaceDeviceGray, img.ColorSpace())

	gray, ok := img.Image().(*image.Gray)
	require.True(t, ok)
	assert.Equal(t, uint8(0x80), gray.GrayAt(0, 0).Y)
}

func TestParseBinaryPPMWithComments(t *testing.T) {
	img, err := parseBinaryPPM([]byte("P6\n# generated by decoder\n2 1\n255\n\x01\x02\x03\x04\x05\x06"))
	require.NoError(t, err)

	rgba, ok := img.(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{R: 1, G: 2, B: 3, A: 255}, rgba.RGBAAt(0, 0))
	assert.Equal(t, color.RGBA{R: 4, G: 5, B: 6, A: 255}, rgba.RGBAAt(1, 0))
}

func TestParseBinaryPGMWithComments(t *testing.T) {
	img, err := parseBinaryPGM([]byte("P5\n# generated by decoder\n2 1\n255\n\x01\x02"))
	require.NoError(t, err)

	gray, ok := img.(*image.Gray)
	require.True(t, ok)
	assert.Equal(t, uint8(1), gray.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(2), gray.GrayAt(1, 0).Y)
}

func TestRGBAFromImageMagickCMYKPopplerLineInvertsAdobeSamples(t *testing.T) {
	img, err := rgbaFromImageMagickCMYKPopplerLine([]byte{0xff, 0xff, 0xff, 0xff}, 1, 1, true)
	require.NoError(t, err)
	assert.Equal(t, color.RGBA{R: 255, G: 255, B: 255, A: 255}, img.RGBAAt(0, 0))

	withoutInvert, err := rgbaFromImageMagickCMYKPopplerLine([]byte{0xff, 0xff, 0xff, 0xff}, 1, 1, false)
	require.NoError(t, err)
	assert.NotEqual(t, color.RGBA{R: 255, G: 255, B: 255, A: 255}, withoutInvert.RGBAAt(0, 0))
}

func TestRGBAFromImageMagickCMYKPopplerLineRejectsWrongLength(t *testing.T) {
	_, err := rgbaFromImageMagickCMYKPopplerLine([]byte{0, 0, 0}, 1, 1, false)
	require.Error(t, err)
}

func TestDecoder_DecodeFlateWithDecompressedData(t *testing.T) {
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, err := w.Write([]byte{0x10, 0x20, 0x30, 0x40})
	require.NoError(t, err)
	require.NoError(t, w.Close())

	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterFlate,
		Data:             compressed.Bytes(),
		Width:            2,
		Height:           2,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	require.NoError(t, err)

	gray, ok := img.Image().(*image.Gray)
	require.True(t, ok)
	assert.Equal(t, []byte{0x10, 0x20, 0x30, 0x40}, gray.Pix)
}

func TestDecoder_DecodeFlateFallbackToRawDeflate(t *testing.T) {
	var compressed bytes.Buffer
	fw, err := flate.NewWriter(&compressed, flate.BestSpeed)
	require.NoError(t, err)

	payload := []byte{0x10, 0x20, 0x30, 0x40}
	_, err = fw.Write(payload)
	require.NoError(t, err)
	require.NoError(t, fw.Close())

	d := NewDecoder()
	out, err := d.decompress(compressed.Bytes(), domainimage.FilterFlate, nil)
	require.NoError(t, err)
	assert.Equal(t, payload, out)
}

func TestDecoder_DecodeASCIIHexUnsupported(t *testing.T) {
	d := NewDecoder()
	_, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterASCIIHex,
		Data:             []byte(""),
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	assert.Error(t, err)
}

func TestDecoder_DecodeCMYKAndRGBConversions(t *testing.T) {
	d := NewDecoder()

	rgb, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{16, 32, 48},
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
	})
	require.NoError(t, err)
	rgba, ok := rgb.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{16, 32, 48, 255}, rgba.RGBAAt(0, 0))

	decoded, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0, 0, 0, 0},
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceCMYK,
	})
	require.NoError(t, err)
	rgbaCMYK, ok := decoded.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{255, 255, 255, 255}, rgbaCMYK.RGBAAt(0, 0))
}

func TestDecoder_DecodeIndexedCMYKMatchesDirectCMYKConversion(t *testing.T) {
	d := NewDecoder()

	indexed, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x00, 0xFF},
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceIndexed,
		IndexedBase:      domainimage.ColorSpaceDeviceCMYK,
		IndexedLookup: []byte{
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0xFF,
		},
	})
	require.NoError(t, err)

	direct, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF},
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceCMYK,
	})
	require.NoError(t, err)

	indexedRGBA, ok := indexed.Image().(*image.RGBA)
	require.True(t, ok)
	directRGBA, ok := direct.Image().(*image.RGBA)
	require.True(t, ok)

	assert.Equal(t, directRGBA.RGBAAt(0, 0), indexedRGBA.RGBAAt(0, 0))
	assert.Equal(t, directRGBA.RGBAAt(1, 0), indexedRGBA.RGBAAt(1, 0))
}

func TestDecoder_DecodeIndexedRGBHonorsDecodeArray(t *testing.T) {
	d := NewDecoder()

	indexed, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x00, 0xFF},
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceIndexed,
		IndexedBase:      domainimage.ColorSpaceDeviceRGB,
		IndexedLookup: []byte{
			0xFF, 0x00, 0x00,
			0x00, 0x00, 0xFF,
		},
		Decode: []float64{1, 0},
	})
	require.NoError(t, err)

	direct, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00},
		Width:            2,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
	})
	require.NoError(t, err)

	indexedRGBA, ok := indexed.Image().(*image.RGBA)
	require.True(t, ok)
	directRGBA, ok := direct.Image().(*image.RGBA)
	require.True(t, ok)

	assert.Equal(t, directRGBA.RGBAAt(0, 0), indexedRGBA.RGBAAt(0, 0))
	assert.Equal(t, directRGBA.RGBAAt(1, 0), indexedRGBA.RGBAAt(1, 0))
	assert.NotEqual(t, indexedRGBA.RGBAAt(0, 0), indexedRGBA.RGBAAt(1, 0))
}

func TestDecoder_DecodeCMYKSimpleSubtractiveConversionMode(t *testing.T) {
	d := NewDecoder()

	img, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x00, 0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x00},
		Width:              2,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeSimpleSubtractive,
	})
	require.NoError(t, err)

	rgba, ok := img.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, rgba.RGBAAt(0, 0))
	assert.Equal(t, color.RGBA{0, 255, 255, 255}, rgba.RGBAAt(1, 0))
}

func TestDecoder_DecodeIndexedCMYKSimpleSubtractiveConversionMode(t *testing.T) {
	d := NewDecoder()

	img, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x00, 0x01},
		Width:              2,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceIndexed,
		IndexedBase:        domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeSimpleSubtractive,
		IndexedLookup: []byte{
			0x00, 0x00, 0x00, 0xFF,
			0xFF, 0x00, 0x00, 0x00,
		},
	})
	require.NoError(t, err)

	rgba, ok := img.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, rgba.RGBAAt(0, 0))
	assert.Equal(t, color.RGBA{0, 255, 255, 255}, rgba.RGBAAt(1, 0))
}

func TestDecoder_DecodeCMYKStdlibConversionMode(t *testing.T) {
	d := NewDecoder()

	img, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x00, 0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x00},
		Width:              2,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeStdlib,
	})
	require.NoError(t, err)

	rgba, ok := img.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, rgba.RGBAAt(0, 0))
	assert.Equal(t, color.RGBA{0, 255, 255, 255}, rgba.RGBAAt(1, 0))
}

func TestDecoder_DecodeCMYKHybrid75ConversionMode(t *testing.T) {
	d := NewDecoder()

	defaultImg, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x80, 0x10, 0x20, 0x30},
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceCMYK,
	})
	require.NoError(t, err)

	simpleImg, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x80, 0x10, 0x20, 0x30},
		Width:              1,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeSimpleSubtractive,
	})
	require.NoError(t, err)

	hybridImg, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x80, 0x10, 0x20, 0x30},
		Width:              1,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeHybrid75,
	})
	require.NoError(t, err)

	defaultRGBA := defaultImg.Image().(*image.RGBA).RGBAAt(0, 0)
	simpleRGBA := simpleImg.Image().(*image.RGBA).RGBAAt(0, 0)
	hybridRGBA := hybridImg.Image().(*image.RGBA).RGBAAt(0, 0)

	assert.Equal(t, uint8(255), hybridRGBA.A)
	assert.InDelta(t, blendChannel(defaultRGBA.R, simpleRGBA.R, 0.75), hybridRGBA.R, 0)
	assert.InDelta(t, blendChannel(defaultRGBA.G, simpleRGBA.G, 0.75), hybridRGBA.G, 0)
	assert.InDelta(t, blendChannel(defaultRGBA.B, simpleRGBA.B, 0.75), hybridRGBA.B, 0)
}

func TestDecoder_DecodeIndexedCMYKStdlibConversionMode(t *testing.T) {
	d := NewDecoder()

	img, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x00, 0x01},
		Width:              2,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceIndexed,
		IndexedBase:        domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeStdlib,
		IndexedLookup: []byte{
			0x00, 0x00, 0x00, 0xFF,
			0xFF, 0x00, 0x00, 0x00,
		},
	})
	require.NoError(t, err)

	rgba, ok := img.Image().(*image.RGBA)
	require.True(t, ok)
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, rgba.RGBAAt(0, 0))
	assert.Equal(t, color.RGBA{0, 255, 255, 255}, rgba.RGBAAt(1, 0))
}

func TestDecoder_DecodeIndexedCMYKHybrid75ConversionMode(t *testing.T) {
	d := NewDecoder()

	defaultImg, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x00},
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceIndexed,
		IndexedBase:      domainimage.ColorSpaceDeviceCMYK,
		IndexedLookup: []byte{
			0x80, 0x10, 0x20, 0x30,
		},
	})
	require.NoError(t, err)

	simpleImg, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x00},
		Width:              1,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceIndexed,
		IndexedBase:        domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeSimpleSubtractive,
		IndexedLookup: []byte{
			0x80, 0x10, 0x20, 0x30,
		},
	})
	require.NoError(t, err)

	hybridImg, err := d.Decode(&domainimage.ImageData{
		Filter:             domainimage.FilterNone,
		Data:               []byte{0x00},
		Width:              1,
		Height:             1,
		BitsPerComponent:   8,
		ColorSpace:         domainimage.ColorSpaceIndexed,
		IndexedBase:        domainimage.ColorSpaceDeviceCMYK,
		CMYKConversionMode: domainimage.CMYKConversionModeHybrid75,
		IndexedLookup: []byte{
			0x80, 0x10, 0x20, 0x30,
		},
	})
	require.NoError(t, err)

	defaultRGBA := defaultImg.Image().(*image.RGBA).RGBAAt(0, 0)
	simpleRGBA := simpleImg.Image().(*image.RGBA).RGBAAt(0, 0)
	hybridRGBA := hybridImg.Image().(*image.RGBA).RGBAAt(0, 0)

	assert.Equal(t, uint8(255), hybridRGBA.A)
	assert.InDelta(t, blendChannel(defaultRGBA.R, simpleRGBA.R, 0.75), hybridRGBA.R, 0)
	assert.InDelta(t, blendChannel(defaultRGBA.G, simpleRGBA.G, 0.75), hybridRGBA.G, 0)
	assert.InDelta(t, blendChannel(defaultRGBA.B, simpleRGBA.B, 0.75), hybridRGBA.B, 0)
}

func TestDecoder_ApplyDecodeTransforms(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0x00, 0x80},
		Width:            1,
		Height:           2,
		BitsPerComponent: 8,
		Decode:           []float64{1, 0},
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	require.NoError(t, err)

	gray, ok := img.Image().(*image.Gray)
	require.True(t, ok)
	assert.Equal(t, uint8(255), gray.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(127), gray.GrayAt(0, 1).Y)
}

func TestDecoder_ApplyDecodeTransformsForNon8BPC(t *testing.T) {
	d := NewDecoder()
	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterNone,
		Data:             []byte{0b10000000, 0x00},
		Width:            1,
		Height:           2,
		BitsPerComponent: 1,
		Decode:           []float64{1, 0},
		ColorSpace:       domainimage.ColorSpaceDeviceGray,
	})
	require.NoError(t, err)

	gray, ok := img.Image().(*image.Gray)
	require.True(t, ok)
	assert.Equal(t, uint8(0), gray.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(255), gray.GrayAt(0, 1).Y)
}

func TestDecodeArrayClampsOutOfRange(t *testing.T) {
	value := decodeArray(32767, []float64{255, 0}, 255)
	assert.Equal(t, uint8(0), clampToByte(value, false))
}

func TestApplyCurveToBytePreservesEndpoints(t *testing.T) {
	curve := func(v float64) float64 {
		return 0.08 + v*0.5
	}

	assert.Equal(t, uint8(0), applyCurveToByte(0, curve))
	assert.Equal(t, uint8(255), applyCurveToByte(255, curve))
	assert.NotEqual(t, uint8(128), applyCurveToByte(128, curve))
}

func TestDecoder_RegisterCustomDecoder(t *testing.T) {
	d := NewDecoder()
	stub := &stubDecoder{}
	d.RegisterDecoder(domainimage.FilterDCT, stub)

	img, err := d.Decode(&domainimage.ImageData{
		Filter:           domainimage.FilterDCT,
		Data:             []byte("4142"),
		Width:            1,
		Height:           1,
		BitsPerComponent: 8,
		ColorSpace:       domainimage.ColorSpaceDeviceRGB,
	})
	require.NoError(t, err)
	_, ok := img.Image().(*image.RGBA)
	require.True(t, ok)
	assert.True(t, stub.wasUsed)
}

func TestDecoder_DecodeLZWAndASCII85Branches(t *testing.T) {
	d := NewDecoder()

	var compressed bytes.Buffer
	w := lzw.NewWriter(&compressed, lzw.MSB, 8)
	_, err := w.Write([]byte("LZW"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	lzwDecoded, err := d.decodeLZW(compressed.Bytes())
	require.NoError(t, err)
	assert.Equal(t, []byte("LZW"), lzwDecoded)

	_, err = d.decodeLZW(nil)
	require.NoError(t, err)

	var lsbCompressed bytes.Buffer
	lsbWriter := lzw.NewWriter(&lsbCompressed, lzw.LSB, 8)
	_, err = lsbWriter.Write([]byte("LSBFALLBACK"))
	require.NoError(t, err)
	require.NoError(t, lsbWriter.Close())

	lsbDecoded, err := d.decodeLZW(lsbCompressed.Bytes())
	require.NoError(t, err)
	assert.Equal(t, []byte("LSBFALLBACK"), lsbDecoded)

	ascii85Decoded, err := d.decodeASCII85([]byte("!!!!!~>"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 0, 0, 0}, ascii85Decoded)

	ascii85Decoded, err = d.decodeASCII85([]byte("!!!!"))
	require.NoError(t, err)
	assert.Len(t, ascii85Decoded, 3)

	ascii85Decoded, err = d.decodeASCII85([]byte("<~\n!!!!!\n~>"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 0, 0, 0}, ascii85Decoded)

	_, err = d.decodeASCII85([]byte("{"))
	require.Error(t, err)
	_, err = d.decodeASCII85(nil)
	require.Error(t, err)
}

func TestDecoder_DecodeRGBImageNon8BPC(t *testing.T) {
	d := NewDecoder()
	img, err := d.decodeRGBImage([]byte{0b11100000}, 1, 1, 1, 1, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, color.RGBA{R: 255, G: 255, B: 255, A: 255}, img.RGBAAt(0, 0))
}

func TestDecoder_Decode16BPCImagesUsePopplerHighByteScaling(t *testing.T) {
	d := NewDecoder()

	gray, err := d.decodeGrayImage([]byte{0x80, 0x00}, 1, 1, 16, 2, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, uint8(128), gray.GrayAt(0, 0).Y)

	rgb, err := d.decodeRGBImage([]byte{0xff, 0x00, 0x80, 0x00, 0x00, 0xff}, 1, 1, 16, 6, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, color.RGBA{R: 255, G: 128, B: 0, A: 255}, rgb.RGBAAt(0, 0))
}

func TestDecoder_DecodeNonStandardHighBPCImagesLikePopplerImageStream(t *testing.T) {
	d := NewDecoder()

	gray, err := d.decodeGrayImage([]byte{0x12, 0x3a, 0xbc}, 2, 1, 12, 3, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, uint8(0x23), gray.GrayAt(0, 0).Y)
	assert.Equal(t, uint8(0xbc), gray.GrayAt(1, 0).Y)
}

func TestApplyICCToneCurveBypassesSRGBRGBProfiles(t *testing.T) {
	srgb := image.NewRGBA(image.Rect(0, 0, 1, 1))
	srgb.SetRGBA(0, 0, color.RGBA{R: 128, G: 64, B: 32, A: 255})
	got := applyICCToneCurve(srgb, testRGBGammaICCProfile(true), 3).(*image.RGBA)
	assert.Equal(t, color.RGBA{R: 128, G: 64, B: 32, A: 255}, got.RGBAAt(0, 0))

	nonSRGB := image.NewRGBA(image.Rect(0, 0, 1, 1))
	nonSRGB.SetRGBA(0, 0, color.RGBA{R: 128, G: 64, B: 32, A: 255})
	got = applyICCToneCurve(nonSRGB, testRGBGammaICCProfile(false), 3).(*image.RGBA)
	assert.Less(t, got.RGBAAt(0, 0).R, uint8(128))
	assert.Less(t, got.RGBAAt(0, 0).G, uint8(64))
	assert.Less(t, got.RGBAAt(0, 0).B, uint8(32))
}

func jpegTestData(t *testing.T) []byte {
	t.Helper()
	rgb := image.NewRGBA(image.Rect(0, 0, 1, 1))
	rgb.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})

	var encoded bytes.Buffer
	err := jpeg.Encode(&encoded, rgb, &jpeg.Options{Quality: 80})
	require.NoError(t, err)

	return encoded.Bytes()
}

func testRGBGammaICCProfile(srgb bool) []byte {
	const (
		tagHeaderStart = 128
		tagCountSize   = 4
		tagRecordSize  = 12
		curveOffset    = tagHeaderStart + tagCountSize + 3*tagRecordSize
		curveSize      = 16
	)
	profile := make([]byte, curveOffset+curveSize)
	if srgb {
		copy(profile[64:], []byte("sRGB IEC61966-2.1"))
	}
	binary.BigEndian.PutUint32(profile[tagHeaderStart:tagHeaderStart+4], 3)
	for i, tag := range []string{"rTRC", "gTRC", "bTRC"} {
		offset := tagHeaderStart + tagCountSize + i*tagRecordSize
		copy(profile[offset:offset+4], []byte(tag))
		binary.BigEndian.PutUint32(profile[offset+4:offset+8], curveOffset)
		binary.BigEndian.PutUint32(profile[offset+8:offset+12], curveSize)
	}
	copy(profile[curveOffset:curveOffset+4], []byte("para"))
	binary.BigEndian.PutUint32(profile[curveOffset+12:curveOffset+16], 2<<16)
	return profile
}

func jpegTestGrayData(t *testing.T) []byte {
	t.Helper()
	gray := image.NewGray(image.Rect(0, 0, 1, 1))
	gray.SetGray(0, 0, color.Gray{Y: 200})

	var encoded bytes.Buffer
	err := jpeg.Encode(&encoded, gray, &jpeg.Options{Quality: 80})
	require.NoError(t, err)

	return encoded.Bytes()
}
