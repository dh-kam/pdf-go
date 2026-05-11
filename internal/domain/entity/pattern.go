// Package entity defines pattern domain entities for PDF documents.
package entity

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// PatternType represents the type of a PDF pattern.
type PatternType int

const (
	// PatternTiling represents a tiling pattern (repeated pattern cell).
	PatternTiling PatternType = iota
	// PatternShading represents a shading pattern (gradient fill).
	PatternShading
)

// String returns the string representation of the pattern type.
func (t PatternType) String() string {
	switch t {
	case PatternTiling:
		return "Tiling"
	case PatternShading:
		return "Shading"
	default:
		return "Unknown"
	}
}

// Pattern is the interface that all PDF pattern types must implement.
type Pattern interface {
	// Type returns the PatternType of this pattern.
	Type() PatternType
	// Name returns the pattern name.
	Name() string
	// Matrix returns the pattern transformation matrix.
	Matrix() [6]float64
}

// TilingType represents the tiling type for tiling patterns.
type TilingType int

const (
	// TilingConstantSpacing maintains constant spacing between pattern cells.
	TilingConstantSpacing TilingType = iota + 1
	// TilingNoDistortion avoids distortion when scaling the pattern.
	TilingNoDistortion
	// TilingConstantSpacingFaster maintains constant spacing with faster tiling.
	TilingConstantSpacingFaster
)

// TilingPattern represents a PDF tiling pattern.
// Tiling patterns define a small content stream (the pattern cell) that is
// repeated to fill an area, similar to wallpaper.
type TilingPattern struct {
	resources  *Dict
	name       string
	content    []byte
	matrix     [6]float64
	bbox       [4]float64
	paintType  int
	tilingType TilingType
	xStep      float64
	yStep      float64
}

// NewTilingPattern creates a new TilingPattern.
func NewTilingPattern(name string, paintType int, tilingType TilingType) *TilingPattern {
	return &TilingPattern{
		name:       name,
		paintType:  paintType,
		tilingType: tilingType,
		matrix:     [6]float64{1, 0, 0, 1, 0, 0},
		resources:  NewDict(),
	}
}

// Type returns PatternTiling.
func (p *TilingPattern) Type() PatternType {
	return PatternTiling
}

// Name returns the pattern name.
func (p *TilingPattern) Name() string {
	return p.name
}

// Matrix returns the pattern transformation matrix.
func (p *TilingPattern) Matrix() [6]float64 {
	return p.matrix
}

// SetMatrix sets the pattern transformation matrix.
func (p *TilingPattern) SetMatrix(matrix [6]float64) {
	p.matrix = matrix
}

// GetPaintType returns the paint type (1=colored, 2=uncolored).
func (p *TilingPattern) GetPaintType() int {
	return p.paintType
}

// SetPaintType sets the paint type.
func (p *TilingPattern) SetPaintType(paintType int) {
	p.paintType = paintType
}

// GetTilingType returns the tiling type.
func (p *TilingPattern) GetTilingType() TilingType {
	return p.tilingType
}

// SetTilingType sets the tiling type.
func (p *TilingPattern) SetTilingType(tilingType TilingType) {
	p.tilingType = tilingType
}

// GetBBox returns the bounding box of the pattern cell.
func (p *TilingPattern) GetBBox() [4]float64 {
	return p.bbox
}

// SetBBox sets the bounding box of the pattern cell.
func (p *TilingPattern) SetBBox(bbox [4]float64) {
	p.bbox = bbox
}

// GetXStep returns the horizontal spacing between pattern cells.
func (p *TilingPattern) GetXStep() float64 {
	return p.xStep
}

// SetXStep sets the horizontal spacing between pattern cells.
func (p *TilingPattern) SetXStep(xStep float64) {
	p.xStep = xStep
}

// GetYStep returns the vertical spacing between pattern cells.
func (p *TilingPattern) GetYStep() float64 {
	return p.yStep
}

// SetYStep sets the vertical spacing between pattern cells.
func (p *TilingPattern) SetYStep(yStep float64) {
	p.yStep = yStep
}

// GetResources returns the resource dictionary for the pattern cell.
func (p *TilingPattern) GetResources() *Dict {
	return p.resources
}

// SetResources sets the resource dictionary for the pattern cell.
func (p *TilingPattern) SetResources(resources *Dict) {
	p.resources = resources
}

// GetContent returns the content stream for drawing the pattern cell.
func (p *TilingPattern) GetContent() []byte {
	return p.content
}

// SetContent sets the content stream for drawing the pattern cell.
func (p *TilingPattern) SetContent(content []byte) {
	p.content = content
}

// IsColored returns true if this is a colored tiling pattern.
func (p *TilingPattern) IsColored() bool {
	return p.paintType == 1
}

// IsUncolored returns true if this is an uncolored tiling pattern.
func (p *TilingPattern) IsUncolored() bool {
	return p.paintType == 2
}

// ShadingPattern represents a PDF shading pattern.
// Shading patterns define a gradient fill that smoothly transitions colors.
type ShadingPattern struct {
	shading *Shading
	name    string
	matrix  [6]float64
}

// NewShadingPattern creates a new ShadingPattern.
func NewShadingPattern(name string, shading *Shading) *ShadingPattern {
	return &ShadingPattern{
		name:    name,
		shading: shading,
		matrix:  [6]float64{1, 0, 0, 1, 0, 0},
	}
}

// Type returns PatternShading.
func (p *ShadingPattern) Type() PatternType {
	return PatternShading
}

// Name returns the pattern name.
func (p *ShadingPattern) Name() string {
	return p.name
}

// Matrix returns the pattern transformation matrix.
func (p *ShadingPattern) Matrix() [6]float64 {
	return p.matrix
}

// SetMatrix sets the pattern transformation matrix.
func (p *ShadingPattern) SetMatrix(matrix [6]float64) {
	p.matrix = matrix
}

// GetShading returns the shading object.
func (p *ShadingPattern) GetShading() *Shading {
	return p.shading
}

// SetShading sets the shading object.
func (p *ShadingPattern) SetShading(shading *Shading) {
	p.shading = shading
}

// ShadingType represents the type of a PDF shading.
type ShadingType int

const (
	// ShadingFunctionBased is type 1: Function-based shading.
	ShadingFunctionBased ShadingType = iota + 1
	// ShadingAxial is type 2: Axial shading (linear gradient).
	ShadingAxial
	// ShadingRadial is type 3: Radial shading (radial gradient).
	ShadingRadial
	// ShadingFreeFormGouraud is type 4: Free-form Gouraud-shaded triangle mesh.
	ShadingFreeFormGouraud
	// ShadingLatticeGouraud is type 5: Lattice-form Gouraud-shaded triangle mesh.
	ShadingLatticeGouraud
	// ShadingCoonsPatch is type 6: Coons patch mesh.
	ShadingCoonsPatch
	// ShadingTensorProductPatch is type 7: Tensor-product patch mesh.
	ShadingTensorProductPatch
)

// String returns the string representation of the shading type.
func (t ShadingType) String() string {
	switch t {
	case ShadingFunctionBased:
		return "FunctionBased"
	case ShadingAxial:
		return "Axial"
	case ShadingRadial:
		return "Radial"
	case ShadingFreeFormGouraud:
		return "FreeFormGouraud"
	case ShadingLatticeGouraud:
		return "LatticeGouraud"
	case ShadingCoonsPatch:
		return "CoonsPatch"
	case ShadingTensorProductPatch:
		return "TensorProductPatch"
	default:
		return "Unknown"
	}
}

// Shading represents a PDF shading object.
// Shadings define color gradients for filling areas.
type Shading struct {
	background   color.Color
	colorSpace   string
	decode       []float64
	vertices     []Vertex
	patches      []Patch
	functions    []Function
	coords       []float64
	matrix       [6]float64
	bbox         [4]float64
	hasBBox      bool
	domain       [4]float64
	bitsPerFlag  int
	bitsPerCoord int
	bitsPerComp  int
	shadingType  ShadingType
	extend       [2]bool
	antiAlias    bool
}

// NewShading creates a new Shading.
func NewShading(shadingType ShadingType, colorSpace string) *Shading {
	return &Shading{
		shadingType: shadingType,
		colorSpace:  colorSpace,
		antiAlias:   false,
		extend:      [2]bool{false, false},
	}
}

// GetShadingType returns the shading type.
func (s *Shading) GetShadingType() ShadingType {
	return s.shadingType
}

// SetShadingType sets the shading type.
func (s *Shading) SetShadingType(shadingType ShadingType) {
	s.shadingType = shadingType
}

// GetColorSpace returns the color space name.
func (s *Shading) GetColorSpace() string {
	return s.colorSpace
}

// SetColorSpace sets the color space name.
func (s *Shading) SetColorSpace(colorSpace string) {
	s.colorSpace = colorSpace
}

// GetBackground returns the background color.
func (s *Shading) GetBackground() color.Color {
	return s.background
}

// SetBackground sets the background color.
func (s *Shading) SetBackground(background color.Color) {
	s.background = background
}

// GetBBox returns the bounding box.
func (s *Shading) GetBBox() [4]float64 {
	return s.bbox
}

// HasBBox reports whether the shading explicitly defines a bounding box.
func (s *Shading) HasBBox() bool {
	return s.hasBBox
}

// SetBBox sets the bounding box.
func (s *Shading) SetBBox(bbox [4]float64) {
	s.bbox = bbox
	s.hasBBox = true
}

// GetAntiAlias returns the anti-aliasing flag.
func (s *Shading) GetAntiAlias() bool {
	return s.antiAlias
}

// SetAntiAlias sets the anti-aliasing flag.
func (s *Shading) SetAntiAlias(antiAlias bool) {
	s.antiAlias = antiAlias
}

// GetCoords returns the coordinates array.
func (s *Shading) GetCoords() []float64 {
	return s.coords
}

// SetCoords sets the coordinates array.
func (s *Shading) SetCoords(coords []float64) {
	s.coords = coords
}

// GetDomain returns the domain for function-based shading.
func (s *Shading) GetDomain() [4]float64 {
	return s.domain
}

// SetDomain sets the domain for function-based shading.
func (s *Shading) SetDomain(domain [4]float64) {
	s.domain = domain
}

// GetMatrix returns the transformation matrix for function-based shading.
func (s *Shading) GetMatrix() [6]float64 {
	return s.matrix
}

// SetMatrix sets the transformation matrix for function-based shading.
func (s *Shading) SetMatrix(matrix [6]float64) {
	s.matrix = matrix
}

// GetFunctions returns the function array.
func (s *Shading) GetFunctions() []Function {
	return s.functions
}

// SetFunctions sets the function array.
func (s *Shading) SetFunctions(functions []Function) {
	s.functions = functions
}

// GetExtend returns the extend flags for axial/radial shading.
func (s *Shading) GetExtend() [2]bool {
	return s.extend
}

// SetExtend sets the extend flags for axial/radial shading.
func (s *Shading) SetExtend(extend [2]bool) {
	s.extend = extend
}

// GetVertices returns the vertices for mesh shadings.
func (s *Shading) GetVertices() []Vertex {
	return s.vertices
}

// SetVertices sets the vertices for mesh shadings.
func (s *Shading) SetVertices(vertices []Vertex) {
	s.vertices = vertices
}

// GetPatches returns the decoded patch mesh records for type 6/7 shadings.
func (s *Shading) GetPatches() []Patch {
	return s.patches
}

// SetPatches sets the decoded patch mesh records for type 6/7 shadings.
func (s *Shading) SetPatches(patches []Patch) {
	s.patches = patches
}

// GetBitsPerFlag returns the bits per flag for mesh shadings.
func (s *Shading) GetBitsPerFlag() int {
	return s.bitsPerFlag
}

// SetBitsPerFlag sets the bits per flag for mesh shadings.
func (s *Shading) SetBitsPerFlag(bitsPerFlag int) {
	s.bitsPerFlag = bitsPerFlag
}

// GetBitsPerCoord returns the bits per coordinate for mesh shadings.
func (s *Shading) GetBitsPerCoord() int {
	return s.bitsPerCoord
}

// SetBitsPerCoord sets the bits per coordinate for mesh shadings.
func (s *Shading) SetBitsPerCoord(bitsPerCoord int) {
	s.bitsPerCoord = bitsPerCoord
}

// GetBitsPerComp returns the bits per component for mesh shadings.
func (s *Shading) GetBitsPerComp() int {
	return s.bitsPerComp
}

// SetBitsPerComp sets the bits per component for mesh shadings.
func (s *Shading) SetBitsPerComp(bitsPerComp int) {
	s.bitsPerComp = bitsPerComp
}

// GetDecode returns the decode arrays for mesh shadings.
func (s *Shading) GetDecode() []float64 {
	return s.decode
}

// SetDecode sets the decode arrays for mesh shadings.
func (s *Shading) SetDecode(decode []float64) {
	s.decode = decode
}

// Vertex represents a vertex in a mesh shading with its color.
type Vertex struct {
	Colors []float64
	X      float64
	Y      float64
}

// Patch represents one Coons or tensor-product patch mesh record.
type Patch struct {
	Colors [2][2][]float64
	X      [4][4]float64
	Y      [4][4]float64
}

// NewVertex creates a new Vertex.
func NewVertex(x, y float64, colors []float64) Vertex {
	return Vertex{
		X:      x,
		Y:      y,
		Colors: colors,
	}
}

// Function represents a PDF function used in shading.
type Function interface {
	// Evaluate evaluates the function at the given input values.
	Evaluate(inputs []float64) ([]float64, error)
	// GetInputSize returns the number of input values.
	GetInputSize() int
	// GetOutputSize returns the number of output values.
	GetOutputSize() int
	// GetDomain returns the input domain [min, max] for each input.
	GetDomain() [][2]float64
	// GetRange returns the output range [min, max] for each output.
	GetRange() [][2]float64
}

// SampledFunction represents a sampled function (type 0).
type SampledFunction struct {
	Domain      [][2]float64
	RangeVal    [][2]float64
	Size        []int        // Number of samples in each dimension
	Samples     []float64    // Sample values
	Encode      [][2]float64 // Linear encoder for input values
	Decode      [][2]float64 // Linear decoder for output values
	Interpolate bool         // Whether to interpolate between samples
}

// Evaluate evaluates the sampled function at the given input values.
func (f *SampledFunction) Evaluate(inputs []float64) ([]float64, error) {
	if len(inputs) != len(f.Size) {
		return nil, fmt.Errorf("invalid input size: expected %d, got %d", len(f.Size), len(inputs))
	}

	totalPoints, err := sampledFunctionTotalPoints(f.Size)
	if err != nil {
		return nil, err
	}

	outputSize := sampledFunctionOutputSize(f, totalPoints)
	if outputSize <= 0 {
		return nil, fmt.Errorf("sampled function has no output channels")
	}

	requiredSamples := totalPoints * outputSize
	if len(f.Samples) < requiredSamples {
		return nil, fmt.Errorf("insufficient samples: need %d, got %d", requiredSamples, len(f.Samples))
	}

	lowIndices := make([]int, len(f.Size))
	highIndices := make([]int, len(f.Size))
	fracs := make([]float64, len(f.Size))

	for dim, size := range f.Size {
		domain := [2]float64{0, 1}
		if dim < len(f.Domain) {
			domain = f.Domain[dim]
		}

		encode := [2]float64{0, float64(size - 1)}
		if dim < len(f.Encode) {
			encode = f.Encode[dim]
		}

		in := clampFloat64(inputs[dim], domain[0], domain[1])
		encoded := mapRange(in, domain[0], domain[1], encode[0], encode[1])

		encodeMin := math.Min(encode[0], encode[1])
		encodeMax := math.Max(encode[0], encode[1])
		encoded = clampFloat64(encoded, encodeMin, encodeMax)
		encoded = clampFloat64(encoded, 0, float64(size-1))

		low := int(math.Floor(encoded))
		high := int(math.Ceil(encoded))
		frac := encoded - float64(low)
		if !f.Interpolate {
			high = low
			frac = 0
		}

		if low < 0 {
			low = 0
		}
		if high < 0 {
			high = 0
		}
		if low >= size {
			low = size - 1
		}
		if high >= size {
			high = size - 1
		}

		lowIndices[dim] = low
		highIndices[dim] = high
		fracs[dim] = frac
	}

	cornerCount := 1 << len(f.Size)
	outputs := make([]float64, outputSize)
	for out := 0; out < outputSize; out++ {
		var value float64
		for corner := 0; corner < cornerCount; corner++ {
			weight := 1.0
			sampleIndex := 0
			stride := 1

			for dim := 0; dim < len(f.Size); dim++ {
				idx := lowIndices[dim]
				useHigh := (corner>>dim)&1 == 1
				if useHigh {
					idx = highIndices[dim]
				}

				if highIndices[dim] != lowIndices[dim] {
					if useHigh {
						weight *= fracs[dim]
					} else {
						weight *= 1 - fracs[dim]
					}
				}

				sampleIndex += idx * stride
				stride *= f.Size[dim]
			}

			if weight == 0 {
				continue
			}

			pos := sampleIndex*outputSize + out
			if pos >= 0 && pos < len(f.Samples) {
				value += weight * f.Samples[pos]
			}
		}

		if out < len(f.Decode) {
			value = mapRange(value, 0, 1, f.Decode[out][0], f.Decode[out][1])
		} else if out < len(f.RangeVal) {
			value = mapRange(value, 0, 1, f.RangeVal[out][0], f.RangeVal[out][1])
		}

		if out < len(f.RangeVal) {
			value = clampFloat64(value, f.RangeVal[out][0], f.RangeVal[out][1])
		}
		outputs[out] = value
	}

	return outputs, nil
}

func sampledFunctionTotalPoints(size []int) (int, error) {
	if len(size) == 0 {
		return 0, fmt.Errorf("sampled function size is empty")
	}

	total := 1
	for _, s := range size {
		if s <= 0 {
			return 0, fmt.Errorf("invalid sampled function dimension size: %d", s)
		}
		total *= s
	}
	return total, nil
}

func sampledFunctionOutputSize(f *SampledFunction, totalPoints int) int {
	if len(f.RangeVal) > 0 {
		return len(f.RangeVal)
	}
	if len(f.Decode) > 0 {
		return len(f.Decode)
	}
	if totalPoints > 0 && len(f.Samples) > 0 {
		return len(f.Samples) / totalPoints
	}
	return 0
}

func mapRange(v, inMin, inMax, outMin, outMax float64) float64 {
	if inMax == inMin {
		return outMin
	}
	return outMin + (v-inMin)*(outMax-outMin)/(inMax-inMin)
}

func clampFloat64(v, minVal, maxVal float64) float64 {
	if minVal > maxVal {
		minVal, maxVal = maxVal, minVal
	}
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

// GetInputSize returns the number of input values.
func (f *SampledFunction) GetInputSize() int {
	return len(f.Size)
}

// GetOutputSize returns the number of output values.
func (f *SampledFunction) GetOutputSize() int {
	return len(f.RangeVal)
}

// GetDomain returns the input domain.
func (f *SampledFunction) GetDomain() [][2]float64 {
	return f.Domain
}

// GetRange returns the output range.
func (f *SampledFunction) GetRange() [][2]float64 {
	return f.RangeVal
}

// ExponentialFunction represents an exponential interpolation function (type 2).
type ExponentialFunction struct {
	Domain   [][2]float64
	RangeVal [][2]float64
	C0       []float64 // Function value at domain minimum
	C1       []float64 // Function value at domain maximum
	Exponent float64   // Interpolation exponent
	N        int       // Number of output values
}

// Evaluate evaluates the exponential function at the given input value.
func (f *ExponentialFunction) Evaluate(inputs []float64) ([]float64, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("exponential function requires exactly 1 input, got %d", len(inputs))
	}

	if len(f.C0) == 0 && len(f.C1) == 0 {
		return nil, fmt.Errorf("exponential function has no output coefficients")
	}

	x := inputs[0]
	// Clamp x to domain. Per PDF spec, f(x) = C0 + x^N * (C1 - C0) uses x directly.
	if len(f.Domain) > 0 {
		x = clampFloat64(x, f.Domain[0][0], f.Domain[0][1])
	}

	outputCount := f.N
	if outputCount <= 0 {
		outputCount = len(f.C0)
		if len(f.C1) > outputCount {
			outputCount = len(f.C1)
		}
		if len(f.RangeVal) > outputCount {
			outputCount = len(f.RangeVal)
		}
	}

	outputs := make([]float64, outputCount)
	for i := 0; i < outputCount; i++ {
		c0 := getExponentialCoeff(f.C0, i)
		c1 := getExponentialCoeff(f.C1, i)
		if f.Exponent == 1 {
			outputs[i] = c0 + x*(c1-c0)
		} else {
			outputs[i] = c0 + (c1-c0)*math.Pow(x, f.Exponent)
		}

		if i < len(f.RangeVal) {
			outputs[i] = clampFloat64(outputs[i], f.RangeVal[i][0], f.RangeVal[i][1])
		}
	}

	return outputs, nil
}

func getExponentialCoeff(values []float64, idx int) float64 {
	if len(values) == 0 {
		return 0
	}
	if idx < len(values) {
		return values[idx]
	}
	return values[len(values)-1]
}

// GetInputSize returns the number of input values.
func (f *ExponentialFunction) GetInputSize() int {
	return 1
}

// GetOutputSize returns the number of output values.
func (f *ExponentialFunction) GetOutputSize() int {
	return f.N
}

// GetDomain returns the input domain.
func (f *ExponentialFunction) GetDomain() [][2]float64 {
	return f.Domain
}

// GetRange returns the output range.
func (f *ExponentialFunction) GetRange() [][2]float64 {
	return f.RangeVal
}

// StitchingFunction represents a stitching function (type 3).
type StitchingFunction struct {
	Domain    [][2]float64
	RangeVal  [][2]float64
	Functions []Function   // Sub-functions to stitch together
	Bounds    []float64    // Division points between sub-functions
	Encode    [][2]float64 // How to encode inputs for each sub-function
}

// Evaluate evaluates the stitching function at the given input value.
func (f *StitchingFunction) Evaluate(inputs []float64) ([]float64, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("stitching function requires exactly 1 input, got %d", len(inputs))
	}
	if len(f.Functions) == 0 {
		return nil, fmt.Errorf("stitching function has no sub-functions")
	}

	x := inputs[0]
	domain := [2]float64{0, 1}
	if len(f.Domain) > 0 {
		domain = f.Domain[0]
	}
	x = clampFloat64(x, domain[0], domain[1])

	functionIndex, segMin, segMax := f.resolveSegment(x, domain)
	encode := [2]float64{0, 1}
	if functionIndex < len(f.Encode) {
		encode = f.Encode[functionIndex]
	}

	encoded := encode[0]
	if segMax > segMin {
		encoded = encode[0] + (x-segMin)*(encode[1]-encode[0])/(segMax-segMin)
	}

	outputs, err := f.Functions[functionIndex].Evaluate([]float64{encoded})
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

func (f *StitchingFunction) resolveSegment(x float64, domain [2]float64) (idx int, segMin, segMax float64) {
	numFunctions := len(f.Functions)
	if numFunctions <= 1 {
		return 0, domain[0], domain[1]
	}

	for i := 0; i < numFunctions; i++ {
		segMin = domain[0]
		if i > 0 && i-1 < len(f.Bounds) {
			segMin = f.Bounds[i-1]
		}

		segMax = domain[1]
		if i < len(f.Bounds) {
			segMax = f.Bounds[i]
		}

		// PDF stitching intervals:
		// [d0,b1), [b1,b2), ..., [b_{k-1},d1]
		if i == numFunctions-1 || x < segMax {
			return i, segMin, segMax
		}
	}

	return numFunctions - 1, domain[0], domain[1]
}

// GetInputSize returns the number of input values.
func (f *StitchingFunction) GetInputSize() int {
	return 1
}

// GetOutputSize returns the number of output values.
func (f *StitchingFunction) GetOutputSize() int {
	if len(f.Functions) > 0 {
		return f.Functions[0].GetOutputSize()
	}
	return 0
}

// GetDomain returns the input domain.
func (f *StitchingFunction) GetDomain() [][2]float64 {
	return f.Domain
}

// GetRange returns the output range.
func (f *StitchingFunction) GetRange() [][2]float64 {
	return f.RangeVal
}

// PostScriptFunction represents a PostScript calculator function (type 4).
type PostScriptFunction struct {
	Domain   [][2]float64
	RangeVal [][2]float64
	Program  string
}

// Evaluate evaluates the PostScript calculator function.
func (f *PostScriptFunction) Evaluate(inputs []float64) ([]float64, error) {
	outputSize := f.GetOutputSize()
	if outputSize <= 0 {
		outputSize = 1
	}

	clampedInputs := make([]float64, len(inputs))
	copy(clampedInputs, inputs)
	for i := 0; i < len(clampedInputs) && i < len(f.Domain); i++ {
		clampedInputs[i] = clampFloat64(clampedInputs[i], f.Domain[i][0], f.Domain[i][1])
	}

	tokens, err := parsePostScriptProgram(f.Program)
	if err != nil || len(tokens) == 0 {
		return f.fallbackOutputs(clampedInputs, outputSize), nil
	}

	stack := make([]postScriptValue, 0, len(clampedInputs)+8)
	for _, input := range clampedInputs {
		stack = append(stack, postScriptValue{number: input})
	}

	if err := executePostScriptTokens(tokens, &stack); err != nil {
		return f.fallbackOutputs(clampedInputs, outputSize), nil
	}
	if len(stack) < outputSize {
		return f.fallbackOutputs(clampedInputs, outputSize), nil
	}

	start := len(stack) - outputSize
	out := make([]float64, outputSize)
	for i := 0; i < outputSize; i++ {
		out[i] = stack[start+i].number
		if i < len(f.RangeVal) {
			out[i] = clampFloat64(out[i], f.RangeVal[i][0], f.RangeVal[i][1])
		}
	}

	return out, nil
}

func (f *PostScriptFunction) fallbackOutputs(inputs []float64, outputSize int) []float64 {
	if outputSize <= 0 {
		outputSize = 1
	}

	out := make([]float64, outputSize)
	if len(inputs) == 0 {
		return out
	}

	for i := 0; i < outputSize; i++ {
		src := i
		if src >= len(inputs) {
			src = len(inputs) - 1
		}
		out[i] = inputs[src]
		if i < len(f.RangeVal) {
			out[i] = clampFloat64(out[i], f.RangeVal[i][0], f.RangeVal[i][1])
		}
	}

	return out
}

// GetInputSize returns the number of input values.
func (f *PostScriptFunction) GetInputSize() int {
	return len(f.Domain)
}

// GetOutputSize returns the number of output values.
func (f *PostScriptFunction) GetOutputSize() int {
	if len(f.RangeVal) > 0 {
		return len(f.RangeVal)
	}
	if len(f.Domain) > 0 {
		return len(f.Domain)
	}
	return 1
}

// GetDomain returns the input domain.
func (f *PostScriptFunction) GetDomain() [][2]float64 {
	return f.Domain
}

// GetRange returns the output range.
func (f *PostScriptFunction) GetRange() [][2]float64 {
	return f.RangeVal
}

type postScriptTokenType int

const (
	postScriptTokenOperator postScriptTokenType = iota
	postScriptTokenNumber
	postScriptTokenProcedure
)

type postScriptToken struct {
	operator string
	proc     []postScriptToken
	number   float64
	kind     postScriptTokenType
}

type postScriptValue struct {
	proc   []postScriptToken
	number float64
	isProc bool
}

func parsePostScriptProgram(program string) ([]postScriptToken, error) {
	tokens := tokenizePostScriptProgram(program)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty PostScript program")
	}

	parsed, idx, err := parsePostScriptTokenList(tokens, 0)
	if err != nil {
		return nil, err
	}
	if idx != len(tokens) {
		return nil, fmt.Errorf("unexpected PostScript token sequence")
	}

	// Type 4 streams are usually wrapped in a top-level procedure.
	if len(parsed) == 1 && parsed[0].kind == postScriptTokenProcedure {
		return parsed[0].proc, nil
	}
	return parsed, nil
}

func tokenizePostScriptProgram(program string) []string {
	out := make([]string, 0, 64)
	for i := 0; i < len(program); {
		ch := program[i]
		if ch == '%' {
			for i < len(program) && program[i] != '\n' && program[i] != '\r' {
				i++
			}
			continue
		}
		if isPostScriptWhitespace(ch) {
			i++
			continue
		}
		if ch == '{' || ch == '}' {
			out = append(out, string(ch))
			i++
			continue
		}

		start := i
		for i < len(program) {
			curr := program[i]
			if isPostScriptWhitespace(curr) || curr == '{' || curr == '}' || curr == '%' {
				break
			}
			i++
		}
		if i > start {
			out = append(out, strings.TrimSpace(program[start:i]))
		}
	}
	return out
}

func parsePostScriptTokenList(tokens []string, start int) ([]postScriptToken, int, error) {
	out := make([]postScriptToken, 0, 16)
	i := start
	for i < len(tokens) {
		tok := tokens[i]
		switch tok {
		case "{":
			proc, next, err := parsePostScriptTokenList(tokens, i+1)
			if err != nil {
				return nil, 0, err
			}
			out = append(out, postScriptToken{kind: postScriptTokenProcedure, proc: proc})
			i = next
			continue
		case "}":
			return out, i + 1, nil
		default:
			if num, err := strconv.ParseFloat(tok, 64); err == nil {
				out = append(out, postScriptToken{kind: postScriptTokenNumber, number: num})
			} else {
				out = append(out, postScriptToken{kind: postScriptTokenOperator, operator: tok})
			}
		}
		i++
	}

	return out, i, nil
}

func executePostScriptTokens(tokens []postScriptToken, stack *[]postScriptValue) error {
	for _, token := range tokens {
		switch token.kind {
		case postScriptTokenNumber:
			*stack = append(*stack, postScriptValue{number: token.number})
		case postScriptTokenProcedure:
			*stack = append(*stack, postScriptValue{isProc: true, proc: token.proc})
		case postScriptTokenOperator:
			if err := executePostScriptOperator(token.operator, stack); err != nil {
				return err
			}
		}
	}
	return nil
}

func executePostScriptOperator(operator string, stack *[]postScriptValue) error {
	pop := func() (postScriptValue, error) {
		if len(*stack) == 0 {
			return postScriptValue{}, fmt.Errorf("PostScript stack underflow")
		}
		last := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		return last, nil
	}

	popNumber := func() (float64, error) {
		v, err := pop()
		if err != nil {
			return 0, err
		}
		if v.isProc {
			return 0, fmt.Errorf("expected number on PostScript stack")
		}
		return v.number, nil
	}

	popProcedure := func() ([]postScriptToken, error) {
		v, err := pop()
		if err != nil {
			return nil, err
		}
		if !v.isProc {
			return nil, fmt.Errorf("expected procedure on PostScript stack")
		}
		return v.proc, nil
	}

	switch operator {
	case "true":
		*stack = append(*stack, postScriptValue{number: 1})
	case "false":
		*stack = append(*stack, postScriptValue{number: 0})
	case "dup":
		if len(*stack) == 0 {
			return fmt.Errorf("PostScript dup stack underflow")
		}
		*stack = append(*stack, (*stack)[len(*stack)-1])
	case "exch":
		if len(*stack) < 2 {
			return fmt.Errorf("PostScript exch stack underflow")
		}
		n := len(*stack)
		(*stack)[n-1], (*stack)[n-2] = (*stack)[n-2], (*stack)[n-1]
	case "pop":
		if _, err := pop(); err != nil {
			return err
		}
	case "copy":
		n, err := popNumber()
		if err != nil {
			return err
		}
		count := int(math.Round(n))
		if count < 0 || count > len(*stack) {
			return fmt.Errorf("invalid PostScript copy count: %d", count)
		}
		base := len(*stack) - count
		dup := make([]postScriptValue, count)
		copy(dup, (*stack)[base:])
		*stack = append(*stack, dup...)
	case "index":
		n, err := popNumber()
		if err != nil {
			return err
		}
		index := int(math.Round(n))
		if index < 0 || index >= len(*stack) {
			return fmt.Errorf("invalid PostScript index: %d", index)
		}
		*stack = append(*stack, (*stack)[len(*stack)-1-index])
	case "roll":
		jf, err := popNumber()
		if err != nil {
			return err
		}
		nf, err := popNumber()
		if err != nil {
			return err
		}
		n := int(math.Round(nf))
		j := int(math.Round(jf))
		if n < 0 || n > len(*stack) {
			return fmt.Errorf("invalid PostScript roll count: %d", n)
		}
		if n == 0 {
			return nil
		}
		j %= n
		if j < 0 {
			j += n
		}
		base := len(*stack) - n
		segment := append([]postScriptValue(nil), (*stack)[base:]...)
		for i := 0; i < n; i++ {
			(*stack)[base+(i+j)%n] = segment[i]
		}
	case "if":
		proc, err := popProcedure()
		if err != nil {
			return err
		}
		cond, err := popNumber()
		if err != nil {
			return err
		}
		if cond != 0 {
			if err := executePostScriptTokens(proc, stack); err != nil {
				return err
			}
		}
	case "ifelse":
		falseProc, err := popProcedure()
		if err != nil {
			return err
		}
		trueProc, err := popProcedure()
		if err != nil {
			return err
		}
		cond, err := popNumber()
		if err != nil {
			return err
		}
		if cond != 0 {
			if err := executePostScriptTokens(trueProc, stack); err != nil {
				return err
			}
		} else {
			if err := executePostScriptTokens(falseProc, stack); err != nil {
				return err
			}
		}
	case "add", "sub", "mul", "div", "idiv", "mod", "exp", "atan",
		"eq", "ne", "gt", "ge", "lt", "le", "and", "or", "xor", "max", "min":
		b, err := popNumber()
		if err != nil {
			return err
		}
		a, err := popNumber()
		if err != nil {
			return err
		}

		result := 0.0
		switch operator {
		case "add":
			result = a + b
		case "sub":
			result = a - b
		case "mul":
			result = a * b
		case "div":
			if b == 0 {
				return fmt.Errorf("PostScript division by zero")
			}
			result = a / b
		case "idiv":
			if b == 0 {
				return fmt.Errorf("PostScript integer division by zero")
			}
			result = math.Trunc(a / b)
		case "mod":
			if b == 0 {
				return fmt.Errorf("PostScript modulo by zero")
			}
			result = math.Mod(a, b)
		case "exp":
			result = math.Pow(a, b)
		case "atan":
			result = math.Atan2(a, b) * 180 / math.Pi
		case "eq":
			if a == b {
				result = 1
			}
		case "ne":
			if a != b {
				result = 1
			}
		case "gt":
			if a > b {
				result = 1
			}
		case "ge":
			if a >= b {
				result = 1
			}
		case "lt":
			if a < b {
				result = 1
			}
		case "le":
			if a <= b {
				result = 1
			}
		case "and":
			if (a != 0) && (b != 0) {
				result = 1
			}
		case "or":
			if (a != 0) || (b != 0) {
				result = 1
			}
		case "xor":
			if (a != 0) != (b != 0) {
				result = 1
			}
		case "max":
			result = math.Max(a, b)
		case "min":
			result = math.Min(a, b)
		}
		*stack = append(*stack, postScriptValue{number: result})
	case "abs", "neg", "ceiling", "floor", "round", "truncate", "sqrt",
		"sin", "cos", "ln", "log", "cvi", "cvr", "not":
		v, err := popNumber()
		if err != nil {
			return err
		}

		result := 0.0
		switch operator {
		case "abs":
			result = math.Abs(v)
		case "neg":
			result = -v
		case "ceiling":
			result = math.Ceil(v)
		case "floor":
			result = math.Floor(v)
		case "round":
			result = math.Round(v)
		case "truncate", "cvi":
			result = math.Trunc(v)
		case "sqrt":
			if v < 0 {
				return fmt.Errorf("PostScript sqrt of negative value")
			}
			result = math.Sqrt(v)
		case "sin":
			result = math.Sin(v * math.Pi / 180)
		case "cos":
			result = math.Cos(v * math.Pi / 180)
		case "ln":
			if v <= 0 {
				return fmt.Errorf("PostScript ln domain error")
			}
			result = math.Log(v)
		case "log":
			if v <= 0 {
				return fmt.Errorf("PostScript log domain error")
			}
			result = math.Log10(v)
		case "cvr":
			result = v
		case "not":
			if v == 0 {
				result = 1
			}
		}
		*stack = append(*stack, postScriptValue{number: result})
	default:
		return fmt.Errorf("unsupported PostScript operator: %s", operator)
	}

	return nil
}

func isPostScriptWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

// AxialShadingCoords represents coordinates for axial shading (type 2).
type AxialShadingCoords struct {
	X0, Y0 float64 // Starting point
	X1, Y1 float64 // Ending point
}

// RadialShadingCoords represents coordinates for radial shading (type 3).
type RadialShadingCoords struct {
	X0, Y0 float64 // Center of starting circle
	R0     float64 // Radius of starting circle
	X1, Y1 float64 // Center of ending circle
	R1     float64 // Radius of ending circle
}

// NewAxialShading creates a new axial shading (linear gradient).
func NewAxialShading(colorSpace string, x0, y0, x1, y1 float64, functions []Function, extend [2]bool) *Shading {
	shading := NewShading(ShadingAxial, colorSpace)
	shading.coords = []float64{x0, y0, x1, y1}
	shading.functions = functions
	shading.extend = extend
	return shading
}

// NewRadialShading creates a new radial shading.
func NewRadialShading(colorSpace string, x0, y0, r0, x1, y1, r1 float64, functions []Function, extend [2]bool) *Shading {
	shading := NewShading(ShadingRadial, colorSpace)
	shading.coords = []float64{x0, y0, r0, x1, y1, r1}
	shading.functions = functions
	shading.extend = extend
	return shading
}

// NewFunctionBasedShading creates a new function-based shading (type 1).
func NewFunctionBasedShading(colorSpace string, domain [4]float64, matrix [6]float64, functions []Function) *Shading {
	shading := NewShading(ShadingFunctionBased, colorSpace)
	shading.domain = domain
	shading.matrix = matrix
	shading.functions = functions
	return shading
}

// NewGouraudShading creates a new Gouraud-shaded triangle mesh (type 4 or 5).
func NewGouraudShading(colorSpace string, shadingType ShadingType, vertices []Vertex, bitsPerCoord, bitsPerComp int, decode []float64) *Shading {
	shading := NewShading(shadingType, colorSpace)
	shading.vertices = vertices
	shading.bitsPerCoord = bitsPerCoord
	shading.bitsPerComp = bitsPerComp
	shading.decode = decode
	return shading
}

// NewPatchMeshShading creates a new patch mesh shading (type 6 or 7).
func NewPatchMeshShading(colorSpace string, shadingType ShadingType, vertices []Vertex, bitsPerFlag, bitsPerCoord, bitsPerComp int, decode []float64) *Shading {
	shading := NewShading(shadingType, colorSpace)
	shading.vertices = vertices
	shading.bitsPerFlag = bitsPerFlag
	shading.bitsPerCoord = bitsPerCoord
	shading.bitsPerComp = bitsPerComp
	shading.decode = decode
	return shading
}
