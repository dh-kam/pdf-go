// Package colorspace provides color space handling for PDF rendering.
//
//revive:disable:exported
package colorspace

import (
	"fmt"
	"image/color"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// ColorSpace represents a PDF color space.
type ColorSpace interface {
	// Type returns the color space type.
	Type() ColorSpaceType
	// Name returns the color space name.
	Name() string
	// ConvertToRGBA converts color values to RGBA.
	ConvertToRGBA(values []float64) color.RGBA
	// GetNumComponents returns the number of color components.
	GetNumComponents() int
}

// ColorSpaceType represents the type of a color space.
type ColorSpaceType int

const (
	// ColorSpaceDeviceGray is DeviceGray color space.
	ColorSpaceDeviceGray ColorSpaceType = iota
	// ColorSpaceDeviceRGB is DeviceRGB color space.
	ColorSpaceDeviceRGB
	// ColorSpaceDeviceCMYK is DeviceCMYK color space.
	ColorSpaceDeviceCMYK
	// ColorSpacePattern is Pattern color space.
	ColorSpacePattern
	// ColorSpaceICCBased is ICCBased color space.
	ColorSpaceICCBased
	// ColorSpaceCalGray is CalGray color space.
	ColorSpaceCalGray
	// ColorSpaceCalRGB is CalRGB color space.
	ColorSpaceCalRGB
	// ColorSpaceLab is Lab color space.
	ColorSpaceLab
	// ColorSpaceIndexed is Indexed color space.
	ColorSpaceIndexed
	// ColorSpaceSeparation is Separation color space.
	ColorSpaceSeparation
	// ColorSpaceDeviceN is DeviceN color space.
	ColorSpaceDeviceN
)

// String returns the string representation of the color space type.
func (t ColorSpaceType) String() string {
	switch t {
	case ColorSpaceDeviceGray:
		return "DeviceGray"
	case ColorSpaceDeviceRGB:
		return "DeviceRGB"
	case ColorSpaceDeviceCMYK:
		return "DeviceCMYK"
	case ColorSpacePattern:
		return "Pattern"
	case ColorSpaceICCBased:
		return "ICCBased"
	case ColorSpaceCalGray:
		return "CalGray"
	case ColorSpaceCalRGB:
		return "CalRGB"
	case ColorSpaceLab:
		return "Lab"
	case ColorSpaceIndexed:
		return "Indexed"
	case ColorSpaceSeparation:
		return "Separation"
	case ColorSpaceDeviceN:
		return "DeviceN"
	default:
		return "Unknown"
	}
}

// DeviceGray represents the DeviceGray color space.
type DeviceGray struct{}

// NewDeviceGray creates a new DeviceGray color space.
func NewDeviceGray() *DeviceGray {
	return &DeviceGray{}
}

// Type returns ColorSpaceDeviceGray.
func (cs *DeviceGray) Type() ColorSpaceType {
	return ColorSpaceDeviceGray
}

// Name returns "DeviceGray".
func (cs *DeviceGray) Name() string {
	return "DeviceGray"
}

// ConvertToRGBA converts a gray value to RGBA.
func (cs *DeviceGray) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) == 0 {
		return color.RGBA{0, 0, 0, 255}
	}
	g := ConvertComponentToByte(values[0])
	return color.RGBA{R: g, G: g, B: g, A: 255}
}

// GetNumComponents returns 1 for DeviceGray.
func (cs *DeviceGray) GetNumComponents() int {
	return 1
}

// DeviceRGB represents the DeviceRGB color space.
type DeviceRGB struct{}

// NewDeviceRGB creates a new DeviceRGB color space.
func NewDeviceRGB() *DeviceRGB {
	return &DeviceRGB{}
}

// Type returns ColorSpaceDeviceRGB.
func (cs *DeviceRGB) Type() ColorSpaceType {
	return ColorSpaceDeviceRGB
}

// Name returns "DeviceRGB".
func (cs *DeviceRGB) Name() string {
	return "DeviceRGB"
}

// ConvertToRGBA converts RGB values to RGBA.
func (cs *DeviceRGB) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) < 3 {
		return color.RGBA{0, 0, 0, 255}
	}
	r := ConvertComponentToByte(values[0])
	g := ConvertComponentToByte(values[1])
	b := ConvertComponentToByte(values[2])
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// GetNumComponents returns 3 for DeviceRGB.
func (cs *DeviceRGB) GetNumComponents() int {
	return 3
}

// DeviceCMYK represents the DeviceCMYK color space.
type DeviceCMYK struct{}

// NewDeviceCMYK creates a new DeviceCMYK color space.
func NewDeviceCMYK() *DeviceCMYK {
	return &DeviceCMYK{}
}

// Type returns ColorSpaceDeviceCMYK.
func (cs *DeviceCMYK) Type() ColorSpaceType {
	return ColorSpaceDeviceCMYK
}

// Name returns "DeviceCMYK".
func (cs *DeviceCMYK) Name() string {
	return "DeviceCMYK"
}

// ConvertToRGBA converts CMYK values to RGBA using Poppler's DeviceCMYK matrix.
func (cs *DeviceCMYK) ConvertToRGBA(values []float64) color.RGBA {
	return ConvertDeviceCMYKToRGBA(values)
}

// ConvertDeviceCMYKToRGBA converts CMYK color operands to RGBA.
//
// Poppler stores color operator operands as 16.16 GfxColorComp values with
// dblToCol(), converts them through cmykToRGBMatrixMultiplication(), then writes
// Splash RGB bytes with colToByte().
func ConvertDeviceCMYKToRGBA(values []float64) color.RGBA {
	if len(values) < 4 {
		return color.RGBA{0, 0, 0, 255}
	}

	c := popplerDblToColToDbl(values[0])
	m := popplerDblToColToDbl(values[1])
	y := popplerDblToColToDbl(values[2])
	k := popplerDblToColToDbl(values[3])

	r, g, b := deviceCMYKToRGB(c, m, y, k)

	return color.RGBA{
		R: popplerDblToColToByte(r),
		G: popplerDblToColToByte(g),
		B: popplerDblToColToByte(b),
		A: 255,
	}
}

// ConvertDeviceCMYKBytesToRGBA converts CMYK image samples to RGBA.
//
// Poppler's getRGBLine() path uses byteToDbl() for input samples and
// dblToByte() for output samples, which differs by up to one byte from the
// color-operator path above.
func ConvertDeviceCMYKBytesToRGBA(c, m, y, k uint8) color.RGBA {
	r, g, b := deviceCMYKToRGB(
		float64(c)/255.0,
		float64(m)/255.0,
		float64(y)/255.0,
		float64(k)/255.0,
	)

	return color.RGBA{
		R: popplerDblToByte(r),
		G: popplerDblToByte(g),
		B: popplerDblToByte(b),
		A: 255,
	}
}

func deviceCMYKToRGB(c, m, y, k float64) (float64, float64, float64) {
	c = clamp01(c)
	m = clamp01(m)
	y = clamp01(y)
	k = clamp01(k)

	c1 := 1 - c
	m1 := 1 - m
	y1 := 1 - y
	k1 := 1 - k

	x := c1 * m1 * y1 * k1
	r := x
	g := x
	b := x

	x = c1 * m1 * y1 * k
	r += 0.1373 * x
	g += 0.1216 * x
	b += 0.1255 * x

	x = c1 * m1 * y * k1
	r += x
	g += 0.9490 * x

	x = c1 * m1 * y * k
	r += 0.1098 * x
	g += 0.1020 * x

	x = c1 * m * y1 * k1
	r += 0.9255 * x
	b += 0.5490 * x

	x = c1 * m * y1 * k
	r += 0.1412 * x

	x = c1 * m * y * k1
	r += 0.9294 * x
	g += 0.1098 * x
	b += 0.1412 * x

	x = c1 * m * y * k
	r += 0.1333 * x

	x = c * m1 * y1 * k1
	g += 0.6784 * x
	b += 0.9373 * x

	x = c * m1 * y1 * k
	g += 0.0588 * x
	b += 0.1412 * x

	x = c * m1 * y * k1
	g += 0.6510 * x
	b += 0.3137 * x

	x = c * m1 * y * k
	g += 0.0745 * x

	x = c * m * y1 * k1
	r += 0.1804 * x
	g += 0.1922 * x
	b += 0.5725 * x

	x = c * m * y1 * k
	b += 0.0078 * x

	x = c * m * y * k1
	r += 0.2118 * x
	g += 0.2119 * x
	b += 0.2235 * x

	return clamp01(r), clamp01(g), clamp01(b)
}

func popplerDblToColToDbl(v float64) float64 {
	return float64(popplerDblToCol(v)) / 0x10000
}

// ConvertComponentToByte converts a PDF color component using Poppler's
// dblToCol plus colToByte quantization.
func ConvertComponentToByte(v float64) uint8 {
	return popplerDblToColToByte(v)
}

func popplerDblToColToByte(v float64) uint8 {
	col := popplerDblToCol(v)
	return uint8(((col << 8) - col + 0x8000) >> 16)
}

func popplerDblToCol(v float64) int {
	v = clamp01(v)
	return int(v * 0x10000)
}

func popplerDblToByte(v float64) uint8 {
	v = clamp01(v)
	// Poppler's dblToByte uses round-half-up: (int)(v*255 + 0.5).
	return uint8(v*255.0 + 0.5)
}

// GetNumComponents returns 4 for DeviceCMYK.
func (cs *DeviceCMYK) GetNumComponents() int {
	return 4
}

// PatternColorSpace represents the Pattern color space.
type PatternColorSpace struct {
	baseColorSpace ColorSpace // Base color space for uncolored patterns
	uncolored      bool       // True if this is an uncolored tiling pattern
}

// NewPatternColorSpace creates a new Pattern color space.
func NewPatternColorSpace(baseColorSpace ColorSpace, uncolored bool) *PatternColorSpace {
	return &PatternColorSpace{
		baseColorSpace: baseColorSpace,
		uncolored:      uncolored,
	}
}

// Type returns ColorSpacePattern.
func (cs *PatternColorSpace) Type() ColorSpaceType {
	return ColorSpacePattern
}

// Name returns "Pattern".
func (cs *PatternColorSpace) Name() string {
	return "Pattern"
}

// ConvertToRGBA converts color values in the pattern color space to RGBA.
// For colored patterns, this ignores the color values and uses the pattern's colors.
// For uncolored patterns, this uses the base color space.
func (cs *PatternColorSpace) ConvertToRGBA(values []float64) color.RGBA {
	if cs.baseColorSpace != nil {
		return cs.baseColorSpace.ConvertToRGBA(values)
	}
	return color.RGBA{0, 0, 0, 255}
}

// GetNumComponents returns 0 for colored patterns, or the base color space's component count for uncolored patterns.
func (cs *PatternColorSpace) GetNumComponents() int {
	if cs.baseColorSpace != nil {
		return cs.baseColorSpace.GetNumComponents()
	}
	return 0
}

// GetBaseColorSpace returns the base color space.
func (cs *PatternColorSpace) GetBaseColorSpace() ColorSpace {
	return cs.baseColorSpace
}

// IsUncolored returns true if this is an uncolored tiling pattern.
func (cs *PatternColorSpace) IsUncolored() bool {
	return cs.uncolored
}

// IndexedColorSpace represents the Indexed color space.
type IndexedColorSpace struct {
	base       ColorSpace
	lookup     []byte
	hival      int
	colorCount int
}

// NewIndexedColorSpace creates a new Indexed color space.
func NewIndexedColorSpace(base ColorSpace, hival int, lookup []byte) *IndexedColorSpace {
	// Calculate color count based on base color space
	baseComps := base.GetNumComponents()
	colorCount := (hival + 1) * baseComps

	return &IndexedColorSpace{
		base:       base,
		hival:      hival,
		lookup:     lookup,
		colorCount: colorCount,
	}
}

// Type returns ColorSpaceIndexed.
func (cs *IndexedColorSpace) Type() ColorSpaceType {
	return ColorSpaceIndexed
}

// Name returns "Indexed".
func (cs *IndexedColorSpace) Name() string {
	return "Indexed"
}

// ConvertToRGBA converts an indexed color value to RGBA.
func (cs *IndexedColorSpace) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) == 0 {
		return color.RGBA{0, 0, 0, 255}
	}

	index := int(values[0])
	if index < 0 || index > cs.hival {
		return color.RGBA{0, 0, 0, 255}
	}

	baseComps := cs.base.GetNumComponents()
	offset := index * baseComps

	if offset+baseComps > len(cs.lookup) {
		return color.RGBA{0, 0, 0, 255}
	}

	// Convert lookup table values to float64 [0, 1]
	colorValues := make([]float64, baseComps)
	for i := 0; i < baseComps; i++ {
		colorValues[i] = float64(cs.lookup[offset+i]) / 255.0
	}

	return cs.base.ConvertToRGBA(colorValues)
}

// GetNumComponents returns 1 for Indexed color space (the index).
func (cs *IndexedColorSpace) GetNumComponents() int {
	return 1
}

// SeparationColorSpace represents the Separation color space.
type SeparationColorSpace struct {
	alternate    ColorSpace
	tintFunction entity.Function
	name         string
}

// NewSeparationColorSpace creates a new Separation color space.
func NewSeparationColorSpace(name string, alternate ColorSpace, tintFunction entity.Function) *SeparationColorSpace {
	return &SeparationColorSpace{
		name:         name,
		alternate:    alternate,
		tintFunction: tintFunction,
	}
}

// Type returns ColorSpaceSeparation.
func (cs *SeparationColorSpace) Type() ColorSpaceType {
	return ColorSpaceSeparation
}

// Name returns the separation colorant name.
func (cs *SeparationColorSpace) Name() string {
	return cs.name
}

// ConvertToRGBA converts a tint value to RGBA.
func (cs *SeparationColorSpace) ConvertToRGBA(values []float64) color.RGBA {
	if len(values) == 0 || cs.tintFunction == nil {
		if cs.alternate != nil {
			return cs.alternate.ConvertToRGBA([]float64{0})
		}
		return color.RGBA{0, 0, 0, 255}
	}

	// Evaluate the tint function
	colorValues, err := cs.tintFunction.Evaluate(values)
	if err != nil || colorValues == nil {
		if cs.alternate != nil {
			return cs.alternate.ConvertToRGBA([]float64{0})
		}
		return color.RGBA{0, 0, 0, 255}
	}

	if cs.alternate != nil {
		return cs.alternate.ConvertToRGBA(colorValues)
	}

	return color.RGBA{0, 0, 0, 255}
}

// GetNumComponents returns 1 for Separation color space (the tint).
func (cs *SeparationColorSpace) GetNumComponents() int {
	return 1
}

// Registry manages color spaces for PDF rendering.
type Registry struct {
	colorSpaces map[string]ColorSpace
}

// NewRegistry creates a new color space registry.
func NewRegistry() *Registry {
	registry := &Registry{
		colorSpaces: make(map[string]ColorSpace),
	}

	// Register standard color spaces
	registry.Register("DeviceGray", NewDeviceGray())
	registry.Register("DeviceRGB", NewDeviceRGB())
	registry.Register("DeviceCMYK", NewDeviceCMYK())

	return registry
}

// Register registers a color space.
func (r *Registry) Register(name string, cs ColorSpace) {
	r.colorSpaces[name] = cs
}

// Get retrieves a color space by name.
func (r *Registry) Get(name string) (ColorSpace, bool) {
	cs, ok := r.colorSpaces[name]
	return cs, ok
}

// ParseColorSpace parses a color space from a PDF object.
func (r *Registry) ParseColorSpace(obj entity.Object) (ColorSpace, error) {
	if obj == nil {
		return nil, fmt.Errorf("nil color space object")
	}

	// Handle named color spaces
	if name, ok := obj.(entity.Name); ok {
		cs, ok := r.Get(name.Value())
		if !ok {
			return nil, fmt.Errorf("unknown color space: %s", name.Value())
		}
		return cs, nil
	}

	// Handle array color spaces
	if arr, ok := obj.(*entity.Array); ok {
		if arr.Len() == 0 {
			return nil, fmt.Errorf("empty color space array")
		}

		first := arr.Get(0)
		typeName, ok := first.(entity.Name)
		if !ok {
			return nil, fmt.Errorf("color space type must be a name")
		}

		switch typeName.Value() {
		case "Pattern":
			return r.parsePatternColorSpace(arr)
		case "ICCBased":
			return r.parseICCBasedColorSpace(arr)
		case "Indexed":
			return r.parseIndexedColorSpace(arr)
		case "Separation":
			return r.parseSeparationColorSpace(arr)
		case "DeviceN":
			return r.parseDeviceNColorSpace(arr)
		case "CalGray":
			return r.parseCalGrayColorSpace(arr)
		case "CalRGB":
			return r.parseCalRGBColorSpace(arr)
		case "Lab":
			return r.parseLabColorSpace(arr)
		default:
			return nil, fmt.Errorf("unknown color space type: %s", typeName.Value())
		}
	}

	return nil, fmt.Errorf("invalid color space object")
}

// parsePatternColorSpace parses a Pattern color space.
func (r *Registry) parsePatternColorSpace(arr *entity.Array) (ColorSpace, error) {
	var base ColorSpace
	uncolored := false

	if arr.Len() > 1 {
		baseObj := arr.Get(1)
		if baseObj != nil {
			baseCS, err := r.ParseColorSpace(baseObj)
			if err == nil {
				base = baseCS
				uncolored = true
			}
		}
	}

	return NewPatternColorSpace(base, uncolored), nil
}

// parseICCBasedColorSpace parses an ICCBased color space.
func (r *Registry) parseICCBasedColorSpace(arr *entity.Array) (ColorSpace, error) {
	// ICCBased color space has a stream as the second element
	if arr.Len() < 2 {
		return nil, fmt.Errorf("iccbased color space requires stream object")
	}

	stream, ok := arr.Get(1).(*entity.Stream)
	if !ok {
		return nil, fmt.Errorf("iccbased color space requires stream object")
	}

	// Get the alternate color space
	var alternate ColorSpace = NewDeviceRGB() // Default alternate

	altObj := stream.Dict().Get(entity.NewName("Alternate"))
	if altObj != nil {
		altCS, err := r.ParseColorSpace(altObj)
		if err == nil {
			alternate = altCS
		}
	}

	return alternate, nil
}

// parseIndexedColorSpace parses an Indexed color space.
func (r *Registry) parseIndexedColorSpace(arr *entity.Array) (ColorSpace, error) {
	if arr.Len() < 4 {
		return nil, fmt.Errorf("indexed color space requires base, hival, and lookup table")
	}

	// Parse base color space
	baseObj := arr.Get(1)
	base, err := r.ParseColorSpace(baseObj)
	if err != nil {
		return nil, fmt.Errorf("invalid base color space: %w", err)
	}

	// Parse hival
	hivalObj := arr.Get(2)
	var hival int
	switch obj := hivalObj.(type) {
	case *entity.Integer:
		hival = int(obj.Value())
	default:
		return nil, fmt.Errorf("hival must be an integer")
	}

	// Parse lookup table
	lookupObj := arr.Get(3)
	var lookup []byte
	switch obj := lookupObj.(type) {
	case *entity.String:
		lookup = []byte(obj.Value())
	case *entity.Stream:
		data, err := obj.Decode()
		if err != nil {
			return nil, fmt.Errorf("failed to decode lookup table: %w", err)
		}
		lookup = data
	default:
		return nil, fmt.Errorf("lookup table must be a string or stream")
	}

	return NewIndexedColorSpace(base, hival, lookup), nil
}

// parseSeparationColorSpace parses a Separation color space.
func (r *Registry) parseSeparationColorSpace(arr *entity.Array) (ColorSpace, error) {
	if arr.Len() < 4 {
		return nil, fmt.Errorf("separation color space requires name, alternate, and tint function")
	}

	// Get colorant name
	nameObj := arr.Get(1)
	name, ok := nameObj.(entity.Name)
	if !ok {
		return nil, fmt.Errorf("colorant name must be a name object")
	}

	// Parse alternate color space
	altObj := arr.Get(2)
	alternate, err := r.ParseColorSpace(altObj)
	if err != nil {
		return nil, fmt.Errorf("invalid alternate color space: %w", err)
	}

	// Parse tint function
	funcObj := arr.Get(3)
	tintFunction, err := parseFunctionFromObject(funcObj)
	if err != nil {
		return nil, fmt.Errorf("invalid tint function: %w", err)
	}

	return NewSeparationColorSpace(name.Value(), alternate, tintFunction), nil
}

// parseDeviceNColorSpace parses a DeviceN color space.
func (r *Registry) parseDeviceNColorSpace(arr *entity.Array) (ColorSpace, error) {
	// DeviceN is similar to Separation but with multiple colorants
	// For simplicity, treat it as RGB for now
	return NewDeviceRGB(), nil
}

// parseCalGrayColorSpace parses a CalGray color space.
func (r *Registry) parseCalGrayColorSpace(arr *entity.Array) (ColorSpace, error) {
	// CalGray is a CIE-based gray color space
	// For simplicity, treat it as DeviceGray for now
	return NewDeviceGray(), nil
}

// parseCalRGBColorSpace parses a CalRGB color space.
func (r *Registry) parseCalRGBColorSpace(arr *entity.Array) (ColorSpace, error) {
	// CalRGB is a CIE-based RGB color space
	// For simplicity, treat it as DeviceRGB for now
	return NewDeviceRGB(), nil
}

// parseLabColorSpace parses a Lab color space.
func (r *Registry) parseLabColorSpace(arr *entity.Array) (ColorSpace, error) {
	// Lab is a CIE L*a*b* color space
	// For simplicity, treat it as DeviceRGB for now
	return NewDeviceRGB(), nil
}

// parseFunctionFromObject parses a function from a PDF object.
func parseFunctionFromObject(obj entity.Object) (entity.Function, error) {
	if obj == nil {
		return nil, fmt.Errorf("nil function object")
	}

	var dict *entity.Dict
	switch o := obj.(type) {
	case *entity.Dict:
		dict = o
	case *entity.Stream:
		dict = o.Dict()
	default:
		return nil, fmt.Errorf("function must be a dictionary or stream")
	}

	// Get function type
	typeObj := dict.Get(entity.NewName("FunctionType"))
	if typeObj == nil {
		return nil, fmt.Errorf("function type not specified")
	}

	var functionType int
	switch obj := typeObj.(type) {
	case *entity.Integer:
		functionType = int(obj.Value())
	default:
		return nil, fmt.Errorf("invalid function type")
	}

	switch functionType {
	case 2:
		return parseExponentialFunction(dict), nil
	case 3:
		return parseStitchingFunction(dict), nil
	case 0:
		fn := parseSampledFunction(dict, obj)
		return fn, nil
	case 4:
		return parsePostScriptFunction(dict, obj), nil
	default:
		return nil, fmt.Errorf("unsupported function type: %d", functionType)
	}
}

// parseExponentialFunction parses an exponential interpolation function.
func parseExponentialFunction(dict *entity.Dict) entity.Function {
	fn := &entity.ExponentialFunction{
		C0:       []float64{0},
		C1:       []float64{1},
		Exponent: 1,
		N:        1,
	}

	// Parse C0
	c0Obj := dict.Get(entity.NewName("C0"))
	if c0Obj != nil {
		if arr, ok := c0Obj.(*entity.Array); ok {
			c0 := make([]float64, arr.Len())
			for i := 0; i < arr.Len(); i++ {
				if item := arr.Get(i); item != nil {
					switch v := item.(type) {
					case *entity.Real:
						c0[i] = v.Value()
					case *entity.Integer:
						c0[i] = float64(v.Value())
					}
				}
			}
			fn.C0 = c0
		}
	}

	// Parse C1
	c1Obj := dict.Get(entity.NewName("C1"))
	if c1Obj != nil {
		if arr, ok := c1Obj.(*entity.Array); ok {
			c1 := make([]float64, arr.Len())
			for i := 0; i < arr.Len(); i++ {
				if item := arr.Get(i); item != nil {
					switch v := item.(type) {
					case *entity.Real:
						c1[i] = v.Value()
					case *entity.Integer:
						c1[i] = float64(v.Value())
					}
				}
			}
			fn.C1 = c1
		}
	}

	// Parse N
	nObj := dict.Get(entity.NewName("N"))
	if nObj != nil {
		switch v := nObj.(type) {
		case *entity.Real:
			fn.Exponent = v.Value()
		case *entity.Integer:
			fn.Exponent = float64(v.Value())
		}
	}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		if arr, ok := domainObj.(*entity.Array); ok && arr.Len() >= 2 {
			domain := make([]float64, 2)
			for i := 0; i < 2; i++ {
				if item := arr.Get(i); item != nil {
					switch v := item.(type) {
					case *entity.Real:
						domain[i] = v.Value()
					case *entity.Integer:
						domain[i] = float64(v.Value())
					}
				}
			}
			fn.Domain = [][2]float64{{domain[0], domain[1]}}
		}
	}

	// Set n
	if len(fn.C0) > len(fn.C1) {
		fn.N = len(fn.C0)
	} else {
		fn.N = len(fn.C1)
	}

	return fn
}

// parseStitchingFunction parses a stitching function.
func parseStitchingFunction(dict *entity.Dict) entity.Function {
	fn := &entity.StitchingFunction{
		Functions: make([]entity.Function, 0),
		Bounds:    make([]float64, 0),
		Encode:    make([][2]float64, 0),
	}

	// Parse Functions
	funcsObj := dict.Get(entity.NewName("Functions"))
	if funcsObj != nil {
		if arr, ok := funcsObj.(*entity.Array); ok {
			for i := 0; i < arr.Len(); i++ {
				if item := arr.Get(i); item != nil {
					if f, err := parseFunctionFromObject(item); err == nil {
						fn.Functions = append(fn.Functions, f)
					}
				}
			}
		}
	}

	// Parse Bounds
	boundsObj := dict.Get(entity.NewName("Bounds"))
	if boundsObj != nil {
		if arr, ok := boundsObj.(*entity.Array); ok {
			for i := 0; i < arr.Len(); i++ {
				if item := arr.Get(i); item != nil {
					switch v := item.(type) {
					case *entity.Real:
						fn.Bounds = append(fn.Bounds, v.Value())
					case *entity.Integer:
						fn.Bounds = append(fn.Bounds, float64(v.Value()))
					}
				}
			}
		}
	}

	// Parse Encode
	encodeObj := dict.Get(entity.NewName("Encode"))
	if encodeObj != nil {
		if arr, ok := encodeObj.(*entity.Array); ok {
			for i := 0; i < arr.Len(); i += 2 {
				if i+1 < arr.Len() {
					var encode0, encode1 float64 = 0, 1
					if item0 := arr.Get(i); item0 != nil {
						switch v := item0.(type) {
						case *entity.Real:
							encode0 = v.Value()
						case *entity.Integer:
							encode0 = float64(v.Value())
						}
					}
					if item1 := arr.Get(i + 1); item1 != nil {
						switch v := item1.(type) {
						case *entity.Real:
							encode1 = v.Value()
						case *entity.Integer:
							encode1 = float64(v.Value())
						}
					}
					fn.Encode = append(fn.Encode, [2]float64{encode0, encode1})
				}
			}
		}
	}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		if arr, ok := domainObj.(*entity.Array); ok && arr.Len() >= 2 {
			domain := make([]float64, 2)
			for i := 0; i < 2; i++ {
				if item := arr.Get(i); item != nil {
					switch v := item.(type) {
					case *entity.Real:
						domain[i] = v.Value()
					case *entity.Integer:
						domain[i] = float64(v.Value())
					}
				}
			}
			fn.Domain = [][2]float64{{domain[0], domain[1]}}
		}
	}

	return fn
}

// parseSampledFunction parses a sampled function.
func parseSampledFunction(dict *entity.Dict, streamObj entity.Object) entity.Function {
	fn := &entity.SampledFunction{
		Size:   []int{1},
		Encode: [][2]float64{{0, 1}},
		Decode: [][2]float64{{0, 1}},
	}

	// Parse Size
	sizeObj := dict.Get(entity.NewName("Size"))
	if sizeObj != nil {
		if arr, ok := sizeObj.(*entity.Array); ok {
			size := make([]int, arr.Len())
			for i := 0; i < arr.Len(); i++ {
				if item := arr.Get(i); item != nil {
					if v, ok := item.(*entity.Integer); ok {
						size[i] = int(v.Value())
					}
				}
			}
			fn.Size = size
		}
	}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		if arr, ok := domainObj.(*entity.Array); ok {
			domain := make([][2]float64, 0)
			for i := 0; i < arr.Len(); i += 2 {
				if i+1 < arr.Len() {
					var d0, d1 float64
					if item0 := arr.Get(i); item0 != nil {
						switch v := item0.(type) {
						case *entity.Real:
							d0 = v.Value()
						case *entity.Integer:
							d0 = float64(v.Value())
						}
					}
					if item1 := arr.Get(i + 1); item1 != nil {
						switch v := item1.(type) {
						case *entity.Real:
							d1 = v.Value()
						case *entity.Integer:
							d1 = float64(v.Value())
						}
					}
					domain = append(domain, [2]float64{d0, d1})
				}
			}
			fn.Domain = domain
		}
	}

	// Parse Range
	rangeObj := dict.Get(entity.NewName("Range"))
	if rangeObj != nil {
		if arr, ok := rangeObj.(*entity.Array); ok {
			rng := make([][2]float64, 0)
			for i := 0; i < arr.Len(); i += 2 {
				if i+1 < arr.Len() {
					var r0, r1 float64
					if item0 := arr.Get(i); item0 != nil {
						switch v := item0.(type) {
						case *entity.Real:
							r0 = v.Value()
						case *entity.Integer:
							r0 = float64(v.Value())
						}
					}
					if item1 := arr.Get(i + 1); item1 != nil {
						switch v := item1.(type) {
						case *entity.Real:
							r1 = v.Value()
						case *entity.Integer:
							r1 = float64(v.Value())
						}
					}
					rng = append(rng, [2]float64{r0, r1})
				}
			}
			fn.RangeVal = rng
		}
	}

	// Parse samples from stream
	if stream, ok := streamObj.(*entity.Stream); ok {
		data, err := stream.Decode()
		if err == nil {
			fn.Samples = parseSampleValues(data)
		}
	}

	return fn
}

// parsePostScriptFunction parses a PostScript calculator function (type 4).
func parsePostScriptFunction(dict *entity.Dict, obj entity.Object) entity.Function {
	fn := &entity.PostScriptFunction{}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		if arr, ok := domainObj.(*entity.Array); ok {
			domain := make([][2]float64, 0, arr.Len()/2)
			for i := 0; i+1 < arr.Len(); i += 2 {
				var d0, d1 float64
				if item0 := arr.Get(i); item0 != nil {
					switch v := item0.(type) {
					case *entity.Real:
						d0 = v.Value()
					case *entity.Integer:
						d0 = float64(v.Value())
					}
				}
				if item1 := arr.Get(i + 1); item1 != nil {
					switch v := item1.(type) {
					case *entity.Real:
						d1 = v.Value()
					case *entity.Integer:
						d1 = float64(v.Value())
					}
				}
				domain = append(domain, [2]float64{d0, d1})
			}
			fn.Domain = domain
		}
	}

	// Parse Range
	rangeObj := dict.Get(entity.NewName("Range"))
	if rangeObj != nil {
		if arr, ok := rangeObj.(*entity.Array); ok {
			rng := make([][2]float64, 0, arr.Len()/2)
			for i := 0; i+1 < arr.Len(); i += 2 {
				var r0, r1 float64
				if item0 := arr.Get(i); item0 != nil {
					switch v := item0.(type) {
					case *entity.Real:
						r0 = v.Value()
					case *entity.Integer:
						r0 = float64(v.Value())
					}
				}
				if item1 := arr.Get(i + 1); item1 != nil {
					switch v := item1.(type) {
					case *entity.Real:
						r1 = v.Value()
					case *entity.Integer:
						r1 = float64(v.Value())
					}
				}
				rng = append(rng, [2]float64{r0, r1})
			}
			fn.RangeVal = rng
		}
	}

	// Parse PostScript program from stream data.
	if streamObj, ok := obj.(*entity.Stream); ok {
		if data, err := streamObj.Decode(); err == nil {
			fn.Program = string(data)
		}
	}

	return fn
}

// parseSampleValues parses sample values from a byte array.
func parseSampleValues(data []byte) []float64 {
	values := make([]float64, len(data))
	for i, b := range data {
		values[i] = float64(b) / 255.0
	}
	return values
}
