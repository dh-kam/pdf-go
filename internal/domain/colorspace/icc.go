package colorspace

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"math"
	"os"
)

// ICCBasedColorSpace represents a PDF ICCBased color space.
type ICCBasedColorSpace struct {
	n         int
	alternate ColorSpace
	ranges    [][2]float64
	profile   *iccMatrixProfile
}

// NewICCBasedColorSpace creates a new ICCBased color space.
func NewICCBasedColorSpace(n int, alternate ColorSpace, ranges [][2]float64, profileData []byte) *ICCBasedColorSpace {
	if alternate == nil {
		alternate = defaultICCBasedAlternate(n)
	}
	if alternate == nil {
		n = 3
		alternate = NewDeviceRGB()
	}
	if len(ranges) != n {
		ranges = make([][2]float64, n)
		for i := range ranges {
			ranges[i] = [2]float64{0, 1}
		}
	}

	return &ICCBasedColorSpace{
		n:         n,
		alternate: alternate,
		ranges:    append([][2]float64(nil), ranges...),
		profile:   parseICCMatrixProfile(profileData, n),
	}
}

// Type returns ColorSpaceICCBased.
func (cs *ICCBasedColorSpace) Type() ColorSpaceType {
	return ColorSpaceICCBased
}

// Name returns "ICCBased".
func (cs *ICCBasedColorSpace) Name() string {
	return "ICCBased"
}

// GetNumComponents returns the ICC profile component count.
func (cs *ICCBasedColorSpace) GetNumComponents() int {
	if cs == nil {
		return 0
	}
	return cs.n
}

// ConvertToRGBA converts ICCBased components to RGBA.
func (cs *ICCBasedColorSpace) ConvertToRGBA(values []float64) color.RGBA {
	if cs == nil {
		return color.RGBA{0, 0, 0, 255}
	}
	if cs.profile != nil && cs.n == 3 && len(values) >= 3 {
		return cs.profile.convertRGB(values)
	}
	if cs.alternate != nil {
		return cs.alternate.ConvertToRGBA(values)
	}
	return color.RGBA{0, 0, 0, 255}
}

type iccMatrixProfile struct {
	colorant        [3][3]float64
	trc             [3]iccToneCurve
	white           [3]float64
	passthroughSRGB bool
}

type iccToneCurve struct {
	identity bool
	gamma    float64
	table    []float64
}

func parseICCMatrixProfile(data []byte, n int) *iccMatrixProfile {
	if n != 3 || len(data) < 132 || string(data[36:40]) != "acsp" {
		return nil
	}
	if string(data[16:20]) != "RGB " || string(data[20:24]) != "XYZ " {
		return nil
	}
	if isSRGBICCProfile(data) {
		return &iccMatrixProfile{passthroughSRGB: true}
	}
	if os.Getenv("PDF_ENABLE_ICC_MATRIX_TRC") != "1" {
		return nil
	}

	tags := readICCTagTable(data)
	required := []string{"rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC"}
	for _, sig := range required {
		if _, ok := tags[sig]; !ok {
			return nil
		}
	}

	profile := &iccMatrixProfile{
		white: [3]float64{0.9642, 1.0, 0.8249},
	}
	var ok bool
	profile.colorant[0], ok = parseICCXYZTag(data, tags["rXYZ"])
	if !ok {
		return nil
	}
	profile.colorant[1], ok = parseICCXYZTag(data, tags["gXYZ"])
	if !ok {
		return nil
	}
	profile.colorant[2], ok = parseICCXYZTag(data, tags["bXYZ"])
	if !ok {
		return nil
	}
	profile.trc[0], ok = parseICCCurveTag(data, tags["rTRC"])
	if !ok {
		return nil
	}
	profile.trc[1], ok = parseICCCurveTag(data, tags["gTRC"])
	if !ok {
		return nil
	}
	profile.trc[2], ok = parseICCCurveTag(data, tags["bTRC"])
	if !ok {
		return nil
	}
	if tag, hasWhitePoint := tags["wtpt"]; hasWhitePoint {
		if white, parsed := parseICCXYZTag(data, tag); parsed {
			profile.white = white
		}
	}
	return profile
}

type iccTagRecord struct {
	offset uint32
	size   uint32
}

func readICCTagTable(data []byte) map[string]iccTagRecord {
	count := int(binary.BigEndian.Uint32(data[128:132]))
	tags := make(map[string]iccTagRecord, count)
	pos := 132
	for i := 0; i < count; i++ {
		if pos+12 > len(data) {
			break
		}
		sig := string(data[pos : pos+4])
		offset := binary.BigEndian.Uint32(data[pos+4 : pos+8])
		size := binary.BigEndian.Uint32(data[pos+8 : pos+12])
		if offset <= uint32(len(data)) && size <= uint32(len(data))-offset {
			tags[sig] = iccTagRecord{offset: offset, size: size}
		}
		pos += 12
	}
	return tags
}

func parseICCXYZTag(data []byte, tag iccTagRecord) ([3]float64, bool) {
	var out [3]float64
	start := int(tag.offset)
	if tag.size < 20 || start+20 > len(data) || string(data[start:start+4]) != "XYZ " {
		return out, false
	}
	out[0] = readS15Fixed16(data[start+8 : start+12])
	out[1] = readS15Fixed16(data[start+12 : start+16])
	out[2] = readS15Fixed16(data[start+16 : start+20])
	return out, true
}

func parseICCCurveTag(data []byte, tag iccTagRecord) (iccToneCurve, bool) {
	curve := iccToneCurve{identity: true}
	start := int(tag.offset)
	if tag.size < 12 || start+12 > len(data) || string(data[start:start+4]) != "curv" {
		return curve, false
	}
	count := int(binary.BigEndian.Uint32(data[start+8 : start+12]))
	switch {
	case count == 0:
		return curve, true
	case count == 1:
		if start+14 > len(data) {
			return curve, false
		}
		curve.identity = false
		curve.gamma = float64(binary.BigEndian.Uint16(data[start+12:start+14])) / 256.0
		return curve, true
	default:
		if start+12+count*2 > len(data) {
			return curve, false
		}
		curve.identity = false
		curve.table = make([]float64, count)
		for i := 0; i < count; i++ {
			raw := binary.BigEndian.Uint16(data[start+12+i*2 : start+14+i*2])
			curve.table[i] = float64(raw) / 65535.0
		}
		return curve, true
	}
}

func readS15Fixed16(raw []byte) float64 {
	return float64(int32(binary.BigEndian.Uint32(raw))) / 65536.0
}

func (p *iccMatrixProfile) convertRGB(values []float64) color.RGBA {
	// Poppler converts graphics color components to 8-bit LCMS inputs before
	// running the ICC transform.
	if p.passthroughSRGB {
		return color.RGBA{
			R: ConvertComponentToByte(values[0]),
			G: ConvertComponentToByte(values[1]),
			B: ConvertComponentToByte(values[2]),
			A: 255,
		}
	}
	r := p.trc[0].eval(float64(ConvertComponentToByte(values[0])) / 255.0)
	g := p.trc[1].eval(float64(ConvertComponentToByte(values[1])) / 255.0)
	b := p.trc[2].eval(float64(ConvertComponentToByte(values[2])) / 255.0)

	xyz := [3]float64{
		p.colorant[0][0]*r + p.colorant[1][0]*g + p.colorant[2][0]*b,
		p.colorant[0][1]*r + p.colorant[1][1]*g + p.colorant[2][1]*b,
		p.colorant[0][2]*r + p.colorant[1][2]*g + p.colorant[2][2]*b,
	}
	xyzD65 := normalizeXYZWhitePoint(p.white, [3]float64{0.95047, 1.0, 1.08883}, xyz)
	rgb := matrixProduct(srgbD65XYZToRGBMatrix, xyzD65)

	return color.RGBA{
		R: uint8(math.Round(clamp(srgbTransferFunction(rgb[0])*255.0, 0, 255))),
		G: uint8(math.Round(clamp(srgbTransferFunction(rgb[1])*255.0, 0, 255))),
		B: uint8(math.Round(clamp(srgbTransferFunction(rgb[2])*255.0, 0, 255))),
		A: 255,
	}
}

func (c iccToneCurve) eval(v float64) float64 {
	v = clamp01(v)
	if c.identity {
		return v
	}
	if c.gamma > 0 {
		return math.Pow(v, c.gamma)
	}
	if len(c.table) == 0 {
		return v
	}
	if v <= 0 {
		return c.table[0]
	}
	last := len(c.table) - 1
	if v >= 1 {
		return c.table[last]
	}
	pos := v * float64(last)
	i := int(pos)
	frac := pos - float64(i)
	return c.table[i]*(1-frac) + c.table[i+1]*frac
}

func normalizeXYZWhitePoint(src, dst, xyz [3]float64) [3]float64 {
	if src == dst {
		return xyz
	}
	lms := matrixProduct(bradfordScaleMatrix, xyz)
	lms = [3]float64{
		lms[0] * dst[0] / src[0],
		lms[1] * dst[1] / src[1],
		lms[2] * dst[2] / src[2],
	}
	return matrixProduct(bradfordScaleInverseMatrix, lms)
}

func isSRGBICCProfile(data []byte) bool {
	return bytes.Contains(data, []byte("sRGB IEC61966-2.1")) ||
		bytes.Contains(data, []byte("IEC 61966-2.1 Default RGB")) ||
		bytes.Contains(data, []byte("IEC sRGB"))
}
