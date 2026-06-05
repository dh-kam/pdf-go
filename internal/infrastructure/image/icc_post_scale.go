package image

import (
	"encoding/binary"
	stdimage "image"
	"math"
	"os"

	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
)

type postScaleICCRGBImage struct {
	*stdimage.RGBA
	transform *popplerRGBICCTransform
}

func (i *postScaleICCRGBImage) PopplerPostScaleICCRGB(r, g, b byte) (byte, byte, byte) {
	if i == nil || i.transform == nil {
		return r, g, b
	}
	return i.transform.ApplyRGB(r, g, b)
}

type popplerRGBICCTransform struct {
	curves         []func(float64) float64
	toRGB          [3][3]float64
	outputByteBias float64
	skiaSRGBLine   bool
}

func newPopplerPostScaleICCRGBImage(img *stdimage.RGBA, data *domainimage.ImageData) stdimage.Image {
	transform, ok := newPopplerRGBICCTransform(data)
	if !ok {
		return img
	}
	return &postScaleICCRGBImage{
		RGBA:      img,
		transform: transform,
	}
}

func newPopplerRGBICCTransform(data *domainimage.ImageData) (*popplerRGBICCTransform, bool) {
	if os.Getenv("PDF_DEBUG_SPLASH_DISABLE_ICC_POST_SCALE") == "1" ||
		data == nil ||
		data.ColorSpace != domainimage.ColorSpaceDeviceRGB ||
		data.BitsPerComponent != 8 ||
		data.ICCComponents != 3 ||
		len(data.ICCProfile) == 0 ||
		len(data.Decode) != 0 ||
		!isPopplerRawICCRGBFilter(data.Filter) ||
		data.Mask != nil {
		return nil, false
	}
	if data.ImageEdgeMode != domainimage.ImageEdgeModeDefault {
		return nil, false
	}
	if isPopplerSkiaSRGBLineProfile(data.ICCProfile) {
		return &popplerRGBICCTransform{skiaSRGBLine: true}, true
	}
	if os.Getenv("PDF_DEBUG_SPLASH_ENABLE_ICC_POST_SCALE") != "1" {
		return nil, false
	}

	transform, ok := parseRGBMatrixTRCICCTransform(data.ICCProfile)
	if !ok {
		return nil, false
	}
	return transform, true
}

func isPopplerRawICCRGBFilter(filter domainimage.ImageFilter) bool {
	return filter == domainimage.FilterNone ||
		filter == domainimage.FilterFlate ||
		filter == domainimage.FilterLZW
}

func isPopplerSkiaSRGBLineProfile(profile []byte) bool {
	const (
		tagHeaderStart = 128
		tagRecordSize  = 12
	)
	if len(profile) < tagHeaderStart+4 || string(profile[16:20]) != "RGB " || string(profile[20:24]) != "XYZ " {
		return false
	}
	tagCount := int(binary.BigEndian.Uint32(profile[tagHeaderStart : tagHeaderStart+4]))
	if tagCount <= 0 {
		return false
	}
	return isExactICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("rXYZ"), [3]int32{0x6fa2, 0x38f5, 0x0390}) &&
		isExactICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("gXYZ"), [3]int32{0x6299, 0xb785, 0x18da}) &&
		isExactICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("bXYZ"), [3]int32{0x24a0, 0x0f84, 0xb6cf}) &&
		isExactICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("wtpt"), [3]int32{0xf6d6, 0x10000, 0xd32d}) &&
		isSRGBParametricTRCTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("rTRC")) &&
		isSRGBParametricTRCTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("gTRC")) &&
		isSRGBParametricTRCTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("bTRC"))
}

func isExactICCXYZTag(profile []byte, tagCount, tagHeaderStart, tagRecordSize int, tagName []byte, want [3]int32) bool {
	data, ok := parseICCTagData(profile, tagCount, tagHeaderStart, tagRecordSize, 20, tagName)
	if !ok || string(data[0:4]) != "XYZ " {
		return false
	}
	for i, off := range []int{8, 12, 16} {
		if int32(binary.BigEndian.Uint32(data[off:off+4])) != want[i] {
			return false
		}
	}
	return true
}

func parseRGBMatrixTRCICCTransform(profile []byte) (*popplerRGBICCTransform, bool) {
	const (
		tagHeaderStart = 128
		tagRecordSize  = 12
	)
	if len(profile) < tagHeaderStart+4 || string(profile[16:20]) != "RGB " || string(profile[20:24]) != "XYZ " {
		return nil, false
	}
	tagCount := int(binary.BigEndian.Uint32(profile[tagHeaderStart : tagHeaderStart+4]))
	if tagCount <= 0 {
		return nil, false
	}

	rXYZ, ok := parseICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("rXYZ"))
	if !ok {
		return nil, false
	}
	gXYZ, ok := parseICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("gXYZ"))
	if !ok {
		return nil, false
	}
	bXYZ, ok := parseICCXYZTag(profile, tagCount, tagHeaderStart, tagRecordSize, []byte("bXYZ"))
	if !ok {
		return nil, false
	}

	curves := make([]func(float64) float64, 3)
	for i, tag := range [][]byte{[]byte("rTRC"), []byte("gTRC"), []byte("bTRC")} {
		curves[i] = parseICCOneCurve(profile, tagCount, tagHeaderStart, tagRecordSize, 8, tag)
		if curves[i] == nil {
			return nil, false
		}
	}

	srcToXYZ := [3][3]float64{
		{rXYZ[0], gXYZ[0], bXYZ[0]},
		{rXYZ[1], gXYZ[1], bXYZ[1]},
		{rXYZ[2], gXYZ[2], bXYZ[2]},
	}
	xyzToSRGB, ok := invert3x3(lcmsSRGBD50ToXYZMatrix())
	if !ok {
		return nil, false
	}
	toRGB := multiply3x3(xyzToSRGB, srcToXYZ)
	return &popplerRGBICCTransform{
		curves:         curves,
		toRGB:          toRGB,
		outputByteBias: lcmsOutputByteBias,
	}, true
}

const lcmsOutputByteBias = 0.12

func lcmsSRGBD50ToXYZMatrix() [3][3]float64 {
	return [3][3]float64{
		{float64(0x6fa0) / 65536.0, float64(0x6297) / 65536.0, float64(0x249f) / 65536.0},
		{float64(0x38f5) / 65536.0, float64(0xb787) / 65536.0, float64(0x0f84) / 65536.0},
		{float64(0x0390) / 65536.0, float64(0x18d9) / 65536.0, float64(0xb6c3) / 65536.0},
	}
}

func multiply3x3(a, b [3][3]float64) [3][3]float64 {
	var out [3][3]float64
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] = a[row][0]*b[0][col] + a[row][1]*b[1][col] + a[row][2]*b[2][col]
		}
	}
	return out
}

func invert3x3(m [3][3]float64) ([3][3]float64, bool) {
	det := m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
	if math.Abs(det) < 1e-12 {
		return [3][3]float64{}, false
	}
	invDet := 1.0 / det
	out := [3][3]float64{
		{
			(m[1][1]*m[2][2] - m[1][2]*m[2][1]) * invDet,
			(m[0][2]*m[2][1] - m[0][1]*m[2][2]) * invDet,
			(m[0][1]*m[1][2] - m[0][2]*m[1][1]) * invDet,
		},
		{
			(m[1][2]*m[2][0] - m[1][0]*m[2][2]) * invDet,
			(m[0][0]*m[2][2] - m[0][2]*m[2][0]) * invDet,
			(m[0][2]*m[1][0] - m[0][0]*m[1][2]) * invDet,
		},
		{
			(m[1][0]*m[2][1] - m[1][1]*m[2][0]) * invDet,
			(m[0][1]*m[2][0] - m[0][0]*m[2][1]) * invDet,
			(m[0][0]*m[1][1] - m[0][1]*m[1][0]) * invDet,
		},
	}
	return out, true
}

func (t *popplerRGBICCTransform) ApplyRGB(r, g, b byte) (byte, byte, byte) {
	if t != nil && t.skiaSRGBLine {
		return applyPopplerSkiaSRGBLineTransform(r, g, b)
	}
	if t == nil || len(t.curves) != 3 {
		return r, g, b
	}
	linear := [3]float64{
		applyICCInputCurve(r, t.curves[0]),
		applyICCInputCurve(g, t.curves[1]),
		applyICCInputCurve(b, t.curves[2]),
	}
	out := [3]float64{
		t.toRGB[0][0]*linear[0] + t.toRGB[0][1]*linear[1] + t.toRGB[0][2]*linear[2],
		t.toRGB[1][0]*linear[0] + t.toRGB[1][1]*linear[1] + t.toRGB[1][2]*linear[2],
		t.toRGB[2][0]*linear[0] + t.toRGB[2][1]*linear[1] + t.toRGB[2][2]*linear[2],
	}
	return encodeSRGBByte(out[0], t.outputByteBias),
		encodeSRGBByte(out[1], t.outputByteBias),
		encodeSRGBByte(out[2], t.outputByteBias)
}

func applyPopplerSkiaSRGBLineTransform(r, g, b byte) (byte, byte, byte) {
	// Poppler builds the image line transform with CHANNELS_SH(3)|BYTES_SH(1)
	// rather than TYPE_RGB_8. For this compact Skia sRGB profile, LCMS only
	// nudges red by one byte in a narrow high-green/low-blue band.
	increment := false
	switch g {
	case 254:
		increment = (r <= 6 && b <= 63) || (r >= 7 && r <= 9 && b <= 62)
	case 255:
		increment = (r <= 1 && b <= 95) || (r >= 2 && r <= 9 && b <= 94) || (r == 10 && b <= 65)
	}
	if increment {
		r++
	}
	return r, g, b
}

func applyICCInputCurve(v byte, curve func(float64) float64) float64 {
	x := float64(v) / 255.0
	if curve == nil {
		return x
	}
	y := curve(x)
	if y < 0 {
		return 0
	}
	if y > 1 {
		return 1
	}
	return y
}

func encodeSRGBByte(v, byteBias float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	var encoded float64
	if v <= 0.0031308 {
		encoded = 12.92 * v
	} else {
		encoded = 1.055*math.Pow(v, 1.0/2.4) - 0.055
	}
	if encoded <= 0 {
		return 0
	}
	if encoded >= 1 {
		return 255
	}
	return byte(math.Floor(encoded*255.0 + byteBias + 0.5))
}
