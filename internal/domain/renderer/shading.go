// Package renderer provides PDF content stream evaluation and rendering.
package renderer

import (
	"fmt"
	"image/color"
	"io"
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

// paintShading paints a shading pattern - 'sh' operator.
func (e *Evaluator) paintShading(op Operator) error {
	if len(op.Operands) != 1 {
		return errors.Invalid("operator_sh", nil)
	}

	// Get the shading name
	shadingName, ok := op.Operands[0].(entity.Name)
	if !ok {
		return errors.Invalid("operator_sh", fmt.Errorf("shading name must be a name"))
	}

	// Get the shading pattern from resources
	if e.resources == nil {
		return nil // No resources, skip
	}

	shadingObj := e.getResourceEntry(entity.Name("Shading"), shadingName)
	if shadingObj == nil {
		return nil // Shading not found, skip
	}

	// Parse the shading object, preserving mesh stream payload when present.
	shading, err := e.parseShadingObject(shadingObj)
	if err != nil {
		return errors.Invalid("parse_shading", err)
	}

	// Render the shading to canvas if set
	if e.canvas != nil && shading != nil {
		return e.renderShading(shading)
	}

	return nil
}

// getResourceEntry resolves named entries from a resource category dictionary.
// It first checks resources like /Font, /XObject, /Shading and then falls back
// to direct lookup for compatibility.
func (e *Evaluator) getResourceEntry(category entity.Name, name entity.Name) entity.Object {
	if e.resources == nil {
		return nil
	}

	categoryObj := e.resolveResourceEntryObject(e.resources.Get(category), 0)
	if categoryStream, ok := categoryObj.(*entity.Stream); ok {
		categoryObj = categoryStream.Dict()
	}
	if categoryDict, ok := categoryObj.(*entity.Dict); ok {
		if resourceObj := e.resolveResourceEntryObject(categoryDict.Get(name), 0); resourceObj != nil {
			return resourceObj
		}
	}

	return e.resolveResourceEntryObject(e.resources.Get(name), 0)
}

func (e *Evaluator) resolveResourceEntryObject(obj entity.Object, depth int) entity.Object {
	if obj == nil || depth > 8 {
		return obj
	}

	ref, ok := obj.(entity.Ref)
	if !ok || e.xref == nil {
		return obj
	}

	resolved, err := e.xref.Fetch(ref)
	if err != nil {
		return obj
	}

	return e.resolveResourceEntryObject(resolved, depth+1)
}

// parseShading parses a shading dictionary into a Shading object.
func (e *Evaluator) parseShading(dict *entity.Dict) (*entity.Shading, error) {
	// Get shading type
	typeVal := dict.Get(entity.Name("ShadingType"))
	if typeVal == nil {
		return nil, fmt.Errorf("shading missing ShadingType")
	}

	var shadingType entity.ShadingType
	switch v := typeVal.(type) {
	case *entity.Integer:
		shadingType = entity.ShadingType(v.Value())
	case *entity.Real:
		shadingType = entity.ShadingType(int(v.Value()))
	default:
		return nil, fmt.Errorf("invalid ShadingType")
	}
	if shadingType < entity.ShadingFunctionBased || shadingType > entity.ShadingTensorProductPatch {
		return nil, fmt.Errorf("unsupported ShadingType: %d", shadingType)
	}

	// Get color space
	csVal := dict.Get(entity.Name("ColorSpace"))
	var colorSpace = "DeviceRGB" // Default
	if csVal != nil {
		if csName, ok := csVal.(entity.Name); ok {
			colorSpace = string(csName)
		}
	}

	shading := entity.NewShading(shadingType, colorSpace)
	e.parseShadingCommon(dict, shading)

	// Parse type-specific fields based on shading type
	switch shadingType {
	case entity.ShadingAxial:
		// Axial shading (linear gradient)
		return e.parseAxialShading(dict, shading)
	case entity.ShadingRadial:
		// Radial shading (radial gradient)
		return e.parseRadialShading(dict, shading)
	case entity.ShadingFunctionBased:
		// Function-based shading
		return e.parseFunctionShading(dict, shading)
	case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud,
		entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
		return e.parseMeshShading(dict, shading)
	default:
		return shading, nil
	}
}

func (e *Evaluator) parseShadingCommon(dict *entity.Dict, shading *entity.Shading) {
	bboxVal := dict.Get(entity.Name("BBox"))
	if bboxArr, ok := bboxVal.(*entity.Array); ok && bboxArr.Len() >= 4 {
		var bbox [4]float64
		for i := 0; i < 4; i++ {
			switch v := bboxArr.Get(i).(type) {
			case *entity.Integer:
				bbox[i] = float64(v.Value())
			case *entity.Real:
				bbox[i] = v.Value()
			}
		}
		shading.SetBBox(bbox)
	}

	aaVal := dict.Get(entity.Name("AntiAlias"))
	switch v := aaVal.(type) {
	case *entity.Boolean:
		shading.SetAntiAlias(v.Value())
	case *entity.Integer:
		shading.SetAntiAlias(v.Value() != 0)
	}
}

// parseAxialShading parses an axial shading (linear gradient).
func (e *Evaluator) parseAxialShading(dict *entity.Dict, shading *entity.Shading) (*entity.Shading, error) {
	// Get coordinates array
	coordsVal := dict.Get(entity.Name("Coords"))
	if coordsVal == nil {
		return nil, fmt.Errorf("axial shading missing Coords")
	}

	coordsArr, ok := coordsVal.(*entity.Array)
	if !ok || coordsArr.Len() < 4 {
		return nil, fmt.Errorf("axial shading Coords must be an array with at least 4 elements")
	}

	// Extract coordinates: x0, y0, x1, y1
	coords := make([]float64, 0, 4)
	for i := 0; i < 4 && i < coordsArr.Len(); i++ {
		if val := coordsArr.Get(i); val != nil {
			if num, ok := val.(*entity.Integer); ok {
				coords = append(coords, float64(num.Value()))
			} else if num, ok := val.(*entity.Real); ok {
				coords = append(coords, num.Value())
			}
		}
	}

	if len(coords) < 4 {
		return nil, fmt.Errorf("axial shading Coords must have at least 4 numeric values")
	}

	shading.SetCoords(coords)
	parseUnivariateShadingDomain(dict, shading)

	if functionObj := dict.Get(entity.Name("Function")); functionObj != nil {
		functions, err := e.parseShadingFunctionList(functionObj)
		if err == nil && len(functions) > 0 {
			shading.SetFunctions(functions)
		}
	}

	// Get extend array
	extendVal := dict.Get(entity.Name("Extend"))
	if extendVal != nil {
		if extendArr, ok := extendVal.(*entity.Array); ok && extendArr.Len() >= 2 {
			extend := [2]bool{false, false}
			if val0 := extendArr.Get(0); val0 != nil {
				if boolVal, ok := val0.(*entity.Boolean); ok {
					extend[0] = boolVal.Value()
				} else if intVal, ok := val0.(*entity.Integer); ok {
					extend[0] = intVal.Value() != 0
				}
			}
			if val1 := extendArr.Get(1); val1 != nil {
				if boolVal, ok := val1.(*entity.Boolean); ok {
					extend[1] = boolVal.Value()
				} else if intVal, ok := val1.(*entity.Integer); ok {
					extend[1] = intVal.Value() != 0
				}
			}
			shading.SetExtend(extend)
		}
	}

	return shading, nil
}

// parseRadialShading parses a radial shading (radial gradient).
func (e *Evaluator) parseRadialShading(dict *entity.Dict, shading *entity.Shading) (*entity.Shading, error) {
	// Get coordinates array
	coordsVal := dict.Get(entity.Name("Coords"))
	if coordsVal == nil {
		return nil, fmt.Errorf("radial shading missing Coords")
	}

	coordsArr, ok := coordsVal.(*entity.Array)
	if !ok || coordsArr.Len() < 6 {
		return nil, fmt.Errorf("radial shading Coords must be an array with at least 6 elements")
	}

	// Extract coordinates: x0, y0, r0, x1, y1, r1
	coords := make([]float64, 0, 6)
	for i := 0; i < 6 && i < coordsArr.Len(); i++ {
		if val := coordsArr.Get(i); val != nil {
			if num, ok := val.(*entity.Integer); ok {
				coords = append(coords, float64(num.Value()))
			} else if num, ok := val.(*entity.Real); ok {
				coords = append(coords, num.Value())
			}
		}
	}

	if len(coords) < 6 {
		return nil, fmt.Errorf("radial shading Coords must have at least 6 numeric values")
	}

	shading.SetCoords(coords)
	parseUnivariateShadingDomain(dict, shading)

	if functionObj := dict.Get(entity.Name("Function")); functionObj != nil {
		functions, err := e.parseShadingFunctionList(functionObj)
		if err == nil && len(functions) > 0 {
			shading.SetFunctions(functions)
		}
	}

	// Get extend array
	extendVal := dict.Get(entity.Name("Extend"))
	if extendVal != nil {
		if extendArr, ok := extendVal.(*entity.Array); ok && extendArr.Len() >= 2 {
			extend := [2]bool{false, false}
			if val0 := extendArr.Get(0); val0 != nil {
				if boolVal, ok := val0.(*entity.Boolean); ok {
					extend[0] = boolVal.Value()
				} else if intVal, ok := val0.(*entity.Integer); ok {
					extend[0] = intVal.Value() != 0
				}
			}
			if val1 := extendArr.Get(1); val1 != nil {
				if boolVal, ok := val1.(*entity.Boolean); ok {
					extend[1] = boolVal.Value()
				} else if intVal, ok := val1.(*entity.Integer); ok {
					extend[1] = intVal.Value() != 0
				}
			}
			shading.SetExtend(extend)
		}
	}

	return shading, nil
}

func parseUnivariateShadingDomain(dict *entity.Dict, shading *entity.Shading) {
	if dict == nil || shading == nil {
		return
	}
	domain, err := parseShadingFloatArray(dict.Get(entity.Name("Domain")), 2)
	if err != nil || len(domain) < 2 {
		return
	}
	shading.SetDomain([4]float64{domain[0], domain[1], 0, 0})
}

// parseFunctionShading parses a function-based shading.
func (e *Evaluator) parseFunctionShading(dict *entity.Dict, shading *entity.Shading) (*entity.Shading, error) {
	// Get domain array
	domainVal := dict.Get(entity.Name("Domain"))
	if domainVal != nil {
		if domainArr, ok := domainVal.(*entity.Array); ok && domainArr.Len() >= 4 {
			domain := shading.GetDomain()
			for i := 0; i < 4 && i < domainArr.Len(); i++ {
				if val := domainArr.Get(i); val != nil {
					if num, ok := val.(*entity.Integer); ok {
						domain[i] = float64(num.Value())
					} else if num, ok := val.(*entity.Real); ok {
						domain[i] = num.Value()
					}
				}
			}
			shading.SetDomain(domain)
		}
	}

	// Get matrix array
	matrixVal := dict.Get(entity.Name("Matrix"))
	if matrixVal != nil {
		if matrixArr, ok := matrixVal.(*entity.Array); ok && matrixArr.Len() >= 6 {
			matrix := shading.GetMatrix()
			for i := 0; i < 6 && i < matrixArr.Len(); i++ {
				if val := matrixArr.Get(i); val != nil {
					if num, ok := val.(*entity.Integer); ok {
						matrix[i] = float64(num.Value())
					} else if num, ok := val.(*entity.Real); ok {
						matrix[i] = num.Value()
					}
				}
			}
			shading.SetMatrix(matrix)
		}
	}

	if functionObj := dict.Get(entity.Name("Function")); functionObj != nil {
		functions, err := e.parseShadingFunctionList(functionObj)
		if err == nil && len(functions) > 0 {
			shading.SetFunctions(functions)
		}
	}

	return shading, nil
}

func (e *Evaluator) parseShadingFunctionList(obj entity.Object) ([]entity.Function, error) {
	if obj == nil {
		return nil, fmt.Errorf("shading function is nil")
	}

	if arr, ok := obj.(*entity.Array); ok {
		out := make([]entity.Function, 0, arr.Len())
		for i := 0; i < arr.Len(); i++ {
			fn, err := e.parseShadingFunctionObject(arr.Get(i))
			if err != nil {
				continue
			}
			out = append(out, fn)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("shading function array has no valid functions")
		}
		return out, nil
	}

	fn, err := e.parseShadingFunctionObject(obj)
	if err != nil {
		return nil, err
	}
	return []entity.Function{fn}, nil
}

func (e *Evaluator) parseShadingFunctionObject(obj entity.Object) (entity.Function, error) {
	var dict *entity.Dict
	switch v := obj.(type) {
	case *entity.Dict:
		dict = v
	case *entity.Stream:
		dict = v.Dict()
	default:
		return nil, fmt.Errorf("invalid shading function object: %T", obj)
	}
	if dict == nil {
		return nil, fmt.Errorf("shading function dictionary is nil")
	}

	functionTypeObj := dict.Get(entity.Name("FunctionType"))
	functionType, ok := extractInt(functionTypeObj)
	if !ok {
		return nil, fmt.Errorf("invalid function type")
	}

	switch functionType {
	case 0:
		return e.parseSampledShadingFunction(obj, dict)
	case 2:
		return e.parseExponentialShadingFunction(dict)
	case 3:
		return e.parseStitchingShadingFunction(dict)
	case 4:
		return e.parsePostScriptShadingFunction(obj, dict)
	default:
		return nil, fmt.Errorf("unsupported function type: %d", functionType)
	}
}

func (e *Evaluator) parseSampledShadingFunction(obj entity.Object, dict *entity.Dict) (*entity.SampledFunction, error) {
	fn := &entity.SampledFunction{
		Interpolate: true,
	}

	if domain, err := parseShadingFloatArray(dict.Get(entity.Name("Domain")), 2); err == nil && len(domain) >= 2 {
		fn.Domain = toPairFloatRanges(domain)
	}
	if rng, err := parseShadingFloatArray(dict.Get(entity.Name("Range")), 0); err == nil && len(rng) >= 2 {
		fn.RangeVal = toPairFloatRanges(rng)
	}

	size, err := parseShadingIntArray(dict.Get(entity.Name("Size")), 1)
	if err != nil {
		return nil, fmt.Errorf("sampled function size: %w", err)
	}
	fn.Size = size

	bitsPerSample := 8
	if bitsObj := dict.Get(entity.Name("BitsPerSample")); bitsObj != nil {
		if bits, ok := extractInt(bitsObj); ok {
			bitsPerSample = bits
		}
	}

	if bitsPerSample <= 0 || bitsPerSample > 32 {
		return nil, fmt.Errorf("invalid BitsPerSample: %d", bitsPerSample)
	}

	if encode, err := parseShadingFloatArray(dict.Get(entity.Name("Encode")), 0); err == nil && len(encode) >= 2 {
		fn.Encode = toPairFloatRanges(encode)
	}
	if len(fn.Encode) == 0 {
		fn.Encode = make([][2]float64, len(fn.Size))
		for i, dim := range fn.Size {
			fn.Encode[i] = [2]float64{0, float64(dim - 1)}
		}
	}

	if decode, err := parseShadingFloatArray(dict.Get(entity.Name("Decode")), 0); err == nil && len(decode) >= 2 {
		fn.Decode = toPairFloatRanges(decode)
	}
	if len(fn.Decode) == 0 && len(fn.RangeVal) > 0 {
		fn.Decode = append([][2]float64(nil), fn.RangeVal...)
	}

	streamObj, ok := obj.(*entity.Stream)
	if !ok {
		return nil, fmt.Errorf("sampled function stream is required")
	}

	decodedData, err := stream.NewFromEntity(streamObj).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode sampled function stream: %w", err)
	}

	totalPoints, err := sampledFunctionTotalPoints(fn.Size)
	if err != nil {
		return nil, err
	}

	outputSize := len(fn.RangeVal)
	if outputSize == 0 {
		outputSize = len(fn.Decode)
	}
	if outputSize == 0 {
		outputSize = 1
	}

	samples, err := decodePackedSamples(decodedData, bitsPerSample, totalPoints*outputSize)
	if err != nil {
		return nil, err
	}
	fn.Samples = samples

	return fn, nil
}

func (e *Evaluator) parseExponentialShadingFunction(dict *entity.Dict) (*entity.ExponentialFunction, error) {
	exp := &entity.ExponentialFunction{
		C0:       []float64{0},
		C1:       []float64{1},
		Exponent: 1,
	}

	if domain, err := parseShadingFloatArray(dict.Get(entity.Name("Domain")), 2); err == nil && len(domain) >= 2 {
		exp.Domain = [][2]float64{{domain[0], domain[1]}}
	}
	if rng, err := parseShadingFloatArray(dict.Get(entity.Name("Range")), 0); err == nil && len(rng) >= 2 {
		exp.RangeVal = toPairFloatRanges(rng)
	}
	if c0, err := parseShadingFloatArray(dict.Get(entity.Name("C0")), 0); err == nil && len(c0) > 0 {
		exp.C0 = c0
	}
	if c1, err := parseShadingFloatArray(dict.Get(entity.Name("C1")), 0); err == nil && len(c1) > 0 {
		exp.C1 = c1
	}
	if nObj := dict.Get(entity.Name("N")); nObj != nil {
		if n, ok := extractFloat(nObj); ok {
			exp.Exponent = n
		}
	}

	return exp, nil
}

func (e *Evaluator) parseStitchingShadingFunction(dict *entity.Dict) (*entity.StitchingFunction, error) {
	st := &entity.StitchingFunction{}

	if domain, err := parseShadingFloatArray(dict.Get(entity.Name("Domain")), 2); err == nil && len(domain) >= 2 {
		st.Domain = [][2]float64{{domain[0], domain[1]}}
	}
	if rng, err := parseShadingFloatArray(dict.Get(entity.Name("Range")), 0); err == nil && len(rng) >= 2 {
		st.RangeVal = toPairFloatRanges(rng)
	}
	if bounds, err := parseShadingFloatArray(dict.Get(entity.Name("Bounds")), 0); err == nil {
		st.Bounds = bounds
	}
	if encode, err := parseShadingFloatArray(dict.Get(entity.Name("Encode")), 0); err == nil {
		st.Encode = toPairFloatRanges(encode)
	}

	functionsObj := dict.Get(entity.Name("Functions"))
	functions, err := e.parseShadingFunctionList(functionsObj)
	if err != nil {
		// Some files may use single Function instead of Functions array.
		if functionObj := dict.Get(entity.Name("Function")); functionObj != nil {
			functions, err = e.parseShadingFunctionList(functionObj)
		}
	}
	if err == nil && len(functions) > 0 {
		st.Functions = functions
	}

	return st, nil
}

func (e *Evaluator) parsePostScriptShadingFunction(obj entity.Object, dict *entity.Dict) (*entity.PostScriptFunction, error) {
	ps := &entity.PostScriptFunction{}

	if domain, err := parseShadingFloatArray(dict.Get(entity.Name("Domain")), 0); err == nil && len(domain) >= 2 {
		ps.Domain = toPairFloatRanges(domain)
	}
	if rng, err := parseShadingFloatArray(dict.Get(entity.Name("Range")), 0); err == nil && len(rng) >= 2 {
		ps.RangeVal = toPairFloatRanges(rng)
	}

	if streamObj, ok := obj.(*entity.Stream); ok {
		decodedData, err := stream.NewFromEntity(streamObj).Decode()
		if err != nil {
			return nil, fmt.Errorf("decode PostScript function stream: %w", err)
		}
		ps.Program = string(decodedData)
	}

	return ps, nil
}

func parseShadingIntArray(obj entity.Object, minLen int) ([]int, error) {
	if obj == nil {
		return nil, fmt.Errorf("int array is nil")
	}
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil, fmt.Errorf("int array is not array")
	}

	out := make([]int, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		if v, ok := extractInt(arr.Get(i)); ok {
			out = append(out, v)
		}
	}

	if minLen > 0 && len(out) < minLen {
		return nil, fmt.Errorf("int array too short: %d", len(out))
	}
	return out, nil
}

func parseShadingFloatArray(obj entity.Object, minLen int) ([]float64, error) {
	if obj == nil {
		return nil, fmt.Errorf("float array is nil")
	}
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil, fmt.Errorf("float array is not array")
	}

	out := make([]float64, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		if v, ok := extractFloat(arr.Get(i)); ok {
			out = append(out, v)
		}
	}

	if minLen > 0 && len(out) < minLen {
		return nil, fmt.Errorf("float array too short: %d", len(out))
	}
	return out, nil
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

func decodePackedSamples(data []byte, bitsPerSample, sampleCount int) ([]float64, error) {
	if sampleCount < 0 {
		return nil, fmt.Errorf("invalid sample count: %d", sampleCount)
	}
	if sampleCount == 0 {
		return []float64{}, nil
	}
	if bitsPerSample <= 0 || bitsPerSample > 32 {
		return nil, fmt.Errorf("invalid bits per sample: %d", bitsPerSample)
	}

	maxSample := float64((uint64(1) << uint(bitsPerSample)) - 1)
	if maxSample <= 0 {
		return nil, fmt.Errorf("invalid bits per sample: %d", bitsPerSample)
	}

	totalBits := len(data) * 8
	requiredBits := sampleCount * bitsPerSample
	if requiredBits > totalBits {
		return nil, fmt.Errorf("insufficient sampled function data: need %d bits, got %d", requiredBits, totalBits)
	}

	samples := make([]float64, sampleCount)
	bitPos := 0
	for i := 0; i < sampleCount; i++ {
		raw := uint64(0)
		for b := 0; b < bitsPerSample; b++ {
			byteIdx := bitPos / 8
			bitIdx := 7 - (bitPos % 8)
			raw = (raw << 1) | uint64((data[byteIdx]>>uint(bitIdx))&1)
			bitPos++
		}
		samples[i] = float64(raw) / maxSample
	}
	return samples, nil
}

func toPairFloatRanges(values []float64) [][2]float64 {
	if len(values) < 2 {
		return nil
	}
	size := len(values) / 2
	out := make([][2]float64, 0, size)
	for i := 0; i+1 < len(values); i += 2 {
		out = append(out, [2]float64{values[i], values[i+1]})
	}
	return out
}

func extractFloat(obj entity.Object) (float64, bool) {
	switch v := obj.(type) {
	case *entity.Real:
		return v.Value(), true
	case *entity.Integer:
		return float64(v.Value()), true
	default:
		return 0, false
	}
}

func extractInt(obj entity.Object) (int, bool) {
	switch v := obj.(type) {
	case *entity.Integer:
		return int(v.Value()), true
	case *entity.Real:
		return int(v.Value()), true
	default:
		return 0, false
	}
}

func (e *Evaluator) parseMeshShading(dict *entity.Dict, shading *entity.Shading) (*entity.Shading, error) {
	if v, ok := dict.Get(entity.Name("BitsPerCoordinate")).(*entity.Integer); ok {
		shading.SetBitsPerCoord(int(v.Value()))
	}
	if v, ok := dict.Get(entity.Name("BitsPerComponent")).(*entity.Integer); ok {
		shading.SetBitsPerComp(int(v.Value()))
	}
	if v, ok := dict.Get(entity.Name("BitsPerFlag")).(*entity.Integer); ok {
		shading.SetBitsPerFlag(int(v.Value()))
	}

	decodeVal := dict.Get(entity.Name("Decode"))
	if decodeArr, ok := decodeVal.(*entity.Array); ok && decodeArr.Len() > 0 {
		decode := make([]float64, 0, decodeArr.Len())
		for i := 0; i < decodeArr.Len(); i++ {
			switch v := decodeArr.Get(i).(type) {
			case *entity.Integer:
				decode = append(decode, float64(v.Value()))
			case *entity.Real:
				decode = append(decode, v.Value())
			}
		}
		shading.SetDecode(decode)
	}
	if functionObj := dict.Get(entity.Name("Function")); functionObj != nil {
		functions, err := e.parseShadingFunctionList(functionObj)
		if err == nil && len(functions) > 0 {
			shading.SetFunctions(functions)
		}
	}

	return shading, nil
}

func (e *Evaluator) parseShadingObject(obj entity.Object) (*entity.Shading, error) {
	if obj == nil {
		return nil, nil
	}

	resolved := obj
	if ref, ok := obj.(entity.Ref); ok {
		if e.xref == nil {
			return nil, fmt.Errorf("cannot resolve shading ref without xref")
		}
		fetched, err := e.xref.Fetch(ref)
		if err != nil {
			return nil, err
		}
		resolved = fetched
	}

	var (
		dict      *entity.Dict
		streamObj *entity.Stream
	)
	switch v := resolved.(type) {
	case *entity.Dict:
		dict = v
	case *entity.Stream:
		dict = v.Dict()
		streamObj = v
	default:
		return nil, fmt.Errorf("invalid shading object: %T", obj)
	}
	if dict == nil {
		return nil, fmt.Errorf("shading dict is nil")
	}

	shading, err := e.parseShading(dict)
	if err != nil {
		return nil, err
	}
	if err := e.populateMeshShadingVertices(shading, streamObj); err != nil {
		return nil, err
	}
	return shading, nil
}

func (e *Evaluator) populateMeshShadingVertices(shading *entity.Shading, streamObj *entity.Stream) error {
	if shading == nil || streamObj == nil {
		return nil
	}

	switch shading.GetShadingType() {
	case entity.ShadingFreeFormGouraud:
		vertices, err := decodeFreeFormGouraudVertices(streamObj, shading)
		if err != nil {
			return err
		}
		shading.SetVertices(vertices)
		return nil
	case entity.ShadingTensorProductPatch:
		patches, err := decodeTensorProductPatchMesh(streamObj, shading)
		if err != nil {
			return err
		}
		shading.SetPatches(patches)
		return nil
	default:
		return nil
	}
}

func decodeFreeFormGouraudVertices(streamObj *entity.Stream, shading *entity.Shading) ([]entity.Vertex, error) {
	if streamObj == nil || shading == nil {
		return nil, nil
	}

	data, err := stream.NewFromEntity(streamObj).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode mesh shading stream: %w", err)
	}

	colorComponents, err := meshShadingColorComponentCount(shading)
	if err != nil {
		return nil, err
	}

	reader := &shadingBitReader{data: data}
	vertices := make([]entity.Vertex, 0)

	var previousTriangle [3]entity.Vertex
	havePreviousTriangle := false

	for {
		flag, firstVertex, err := readMeshVertexRecord(reader, shading, colorComponents)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch flag {
		case 0:
			secondFlag, secondVertex, err := readMeshVertexRecord(reader, shading, colorComponents)
			if err != nil {
				return nil, err
			}
			thirdFlag, thirdVertex, err := readMeshVertexRecord(reader, shading, colorComponents)
			if err != nil {
				return nil, err
			}
			_ = secondFlag
			_ = thirdFlag

			triangle := [3]entity.Vertex{firstVertex, secondVertex, thirdVertex}
			vertices = append(vertices, triangle[:]...)
			previousTriangle = triangle
			havePreviousTriangle = true

		case 1:
			if !havePreviousTriangle {
				return nil, fmt.Errorf("type 4 shading continuation without base triangle")
			}
			triangle := [3]entity.Vertex{previousTriangle[1], previousTriangle[2], firstVertex}
			vertices = append(vertices, triangle[:]...)
			previousTriangle = triangle

		case 2:
			if !havePreviousTriangle {
				return nil, fmt.Errorf("type 4 shading continuation without base triangle")
			}
			triangle := [3]entity.Vertex{previousTriangle[0], previousTriangle[2], firstVertex}
			vertices = append(vertices, triangle[:]...)
			previousTriangle = triangle

		default:
			return nil, fmt.Errorf("unsupported type 4 edge flag: %d", flag)
		}
	}

	return vertices, nil
}

func decodeTensorProductPatchMesh(streamObj *entity.Stream, shading *entity.Shading) ([]entity.Patch, error) {
	if streamObj == nil || shading == nil {
		return nil, nil
	}
	data, err := stream.NewFromEntity(streamObj).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode tensor patch shading stream: %w", err)
	}
	colorComponents, err := meshShadingColorComponentCount(shading)
	if err != nil {
		return nil, err
	}
	decode := shading.GetDecode()
	requiredDecode := 4 + colorComponents*2
	if len(decode) < requiredDecode {
		return nil, fmt.Errorf("tensor patch shading decode array too short: got %d want %d", len(decode), requiredDecode)
	}
	reader := &shadingBitReader{data: data}
	patches := make([]entity.Patch, 0)
	for {
		flagRaw, err := reader.readBits(shading.GetBitsPerFlag())
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		flag := int(flagRaw)
		nPts, nColors := 12, 2
		if flag == 0 {
			nPts, nColors = 16, 4
		}
		points := make([][2]float64, nPts)
		for i := 0; i < nPts; i++ {
			xRaw, err := reader.readBits(shading.GetBitsPerCoord())
			if err != nil {
				if err == io.EOF {
					return patches, nil
				}
				return nil, err
			}
			yRaw, err := reader.readBits(shading.GetBitsPerCoord())
			if err != nil {
				if err == io.EOF {
					return patches, nil
				}
				return nil, err
			}
			points[i][0] = decodeMeshValue(xRaw, shading.GetBitsPerCoord(), decode[0], decode[1])
			points[i][1] = decodeMeshValue(yRaw, shading.GetBitsPerCoord(), decode[2], decode[3])
		}
		colors := make([][]float64, nColors)
		for i := 0; i < nColors; i++ {
			colorValues := make([]float64, colorComponents)
			for j := 0; j < colorComponents; j++ {
				raw, err := reader.readBits(shading.GetBitsPerComp())
				if err != nil {
					if err == io.EOF {
						return patches, nil
					}
					return nil, err
				}
				decodeIndex := 4 + j*2
				value := decodeMeshValue(raw, shading.GetBitsPerComp(), decode[decodeIndex], decode[decodeIndex+1])
				if len(shading.GetFunctions()) == 0 {
					value = quantizeMeshColorComponent(value)
				}
				colorValues[j] = value
			}
			colors[i] = colorValues
		}
		patch, err := buildTensorProductPatch(flag, points, colors, patches)
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch)
		reader.alignToByte()
	}
	return patches, nil
}

func buildTensorProductPatch(flag int, points [][2]float64, colors [][]float64, previous []entity.Patch) (entity.Patch, error) {
	var patch entity.Patch
	setPoint := func(i, j, src int) {
		patch.X[i][j] = points[src][0]
		patch.Y[i][j] = points[src][1]
	}
	copyColor := func(i int, j int, src []float64) {
		patch.Colors[i][j] = append([]float64(nil), src...)
	}
	switch flag {
	case 0:
		for _, req := range []struct{ i, j, src int }{
			{0, 0, 0}, {0, 1, 1}, {0, 2, 2}, {0, 3, 3},
			{1, 3, 4}, {2, 3, 5}, {3, 3, 6}, {3, 2, 7},
			{3, 1, 8}, {3, 0, 9}, {2, 0, 10}, {1, 0, 11},
			{1, 1, 12}, {1, 2, 13}, {2, 2, 14}, {2, 1, 15},
		} {
			setPoint(req.i, req.j, req.src)
		}
		copyColor(0, 0, colors[0])
		copyColor(0, 1, colors[1])
		copyColor(1, 1, colors[2])
		copyColor(1, 0, colors[3])
	case 1, 2, 3:
		if len(previous) == 0 {
			return entity.Patch{}, fmt.Errorf("tensor patch continuation without base patch")
		}
		prev := previous[len(previous)-1]
		switch flag {
		case 1:
			copyPatchEdge(&patch, prev, [][4]int{{0, 0, 0, 3}, {0, 1, 1, 3}, {0, 2, 2, 3}, {0, 3, 3, 3}})
			copyColor(0, 0, prev.Colors[0][1])
			copyColor(0, 1, prev.Colors[1][1])
		case 2:
			copyPatchEdge(&patch, prev, [][4]int{{0, 0, 3, 3}, {0, 1, 3, 2}, {0, 2, 3, 1}, {0, 3, 3, 0}})
			copyColor(0, 0, prev.Colors[1][1])
			copyColor(0, 1, prev.Colors[1][0])
		case 3:
			copyPatchEdge(&patch, prev, [][4]int{{0, 0, 3, 0}, {0, 1, 2, 0}, {0, 2, 1, 0}, {0, 3, 0, 0}})
			copyColor(0, 0, prev.Colors[1][0])
			copyColor(0, 1, prev.Colors[0][0])
		}
		for _, req := range []struct{ i, j, src int }{
			{1, 3, 0}, {2, 3, 1}, {3, 3, 2}, {3, 2, 3},
			{3, 1, 4}, {3, 0, 5}, {2, 0, 6}, {1, 0, 7},
			{1, 1, 8}, {1, 2, 9}, {2, 2, 10}, {2, 1, 11},
		} {
			setPoint(req.i, req.j, req.src)
		}
		copyColor(1, 1, colors[0])
		copyColor(1, 0, colors[1])
	default:
		// Poppler consumes unknown patch flags using the continuation record
		// length, then keeps the zero-initialized patch.
		return patch, nil
	}
	return patch, nil
}

func copyPatchEdge(dst *entity.Patch, src entity.Patch, mappings [][4]int) {
	for _, m := range mappings {
		dst.X[m[0]][m[1]] = src.X[m[2]][m[3]]
		dst.Y[m[0]][m[1]] = src.Y[m[2]][m[3]]
	}
}

func meshShadingColorComponentCount(shading *entity.Shading) (int, error) {
	if shading == nil {
		return 0, fmt.Errorf("mesh shading is nil")
	}
	if len(shading.GetFunctions()) > 0 {
		return 1, nil
	}

	if pairs := len(shading.GetDecode())/2 - 2; pairs > 0 {
		return pairs, nil
	}

	switch shading.GetColorSpace() {
	case "DeviceGray":
		return 1, nil
	case "DeviceRGB":
		return 3, nil
	case "DeviceCMYK":
		return 4, nil
	default:
		return 0, fmt.Errorf("unsupported mesh shading color space: %s", shading.GetColorSpace())
	}
}

func readMeshVertexRecord(reader *shadingBitReader, shading *entity.Shading, colorComponents int) (int, entity.Vertex, error) {
	if reader == nil || shading == nil {
		return 0, entity.Vertex{}, fmt.Errorf("mesh vertex reader requires reader and shading")
	}
	if colorComponents <= 0 {
		return 0, entity.Vertex{}, fmt.Errorf("mesh shading has no color components")
	}

	decode := shading.GetDecode()
	requiredDecode := 4 + colorComponents*2
	if len(decode) < requiredDecode {
		return 0, entity.Vertex{}, fmt.Errorf("mesh shading decode array too short: got %d want %d", len(decode), requiredDecode)
	}

	flagValue, err := reader.readBits(shading.GetBitsPerFlag())
	if err != nil {
		return 0, entity.Vertex{}, err
	}
	xRaw, err := reader.readBits(shading.GetBitsPerCoord())
	if err != nil {
		return 0, entity.Vertex{}, err
	}
	yRaw, err := reader.readBits(shading.GetBitsPerCoord())
	if err != nil {
		return 0, entity.Vertex{}, err
	}

	colors := make([]float64, 0, colorComponents)
	for i := 0; i < colorComponents; i++ {
		raw, err := reader.readBits(shading.GetBitsPerComp())
		if err != nil {
			return 0, entity.Vertex{}, err
		}
		decodeIndex := 4 + i*2
		colors = append(colors, quantizeMeshColorComponent(decodeMeshValue(raw, shading.GetBitsPerComp(), decode[decodeIndex], decode[decodeIndex+1])))
	}
	reader.alignToByte()

	x := decodeMeshValue(xRaw, shading.GetBitsPerCoord(), decode[0], decode[1])
	y := decodeMeshValue(yRaw, shading.GetBitsPerCoord(), decode[2], decode[3])
	return int(flagValue), entity.NewVertex(x, y, colors), nil
}

func decodeMeshValue(raw uint64, bits int, min, max float64) float64 {
	if bits <= 0 {
		return min
	}
	maxRaw := math.Exp2(float64(bits)) - 1
	if maxRaw <= 0 {
		return min
	}
	return min + (float64(raw)/maxRaw)*(max-min)
}

func quantizeMeshColorComponent(component float64) float64 {
	const gfxColorComp1 = 0x10000

	return float64(int(component*gfxColorComp1)) / gfxColorComp1
}

type shadingBitReader struct {
	data   []byte
	bitPos int
}

func (r *shadingBitReader) remainingBits() int {
	if r == nil {
		return 0
	}
	return len(r.data)*8 - r.bitPos
}

func (r *shadingBitReader) readBits(count int) (uint64, error) {
	if r == nil {
		return 0, fmt.Errorf("bit reader is nil")
	}
	if count < 0 {
		return 0, fmt.Errorf("invalid bit count: %d", count)
	}
	if count == 0 {
		return 0, nil
	}
	if r.remainingBits() < count {
		return 0, io.EOF
	}

	var value uint64
	for i := 0; i < count; i++ {
		byteIndex := r.bitPos / 8
		bitIndex := 7 - (r.bitPos % 8)
		value = (value << 1) | uint64((r.data[byteIndex]>>uint(bitIndex))&1)
		r.bitPos++
	}
	return value, nil
}

func (r *shadingBitReader) alignToByte() {
	if r == nil {
		return
	}
	if remainder := r.bitPos % 8; remainder != 0 {
		r.bitPos += 8 - remainder
	}
}

// renderShading renders a shading pattern to the canvas.
func (e *Evaluator) renderShading(shading *entity.Shading) error {
	bbox, ok := e.currentShadingBBoxForShading(shading)
	if !ok {
		return nil
	}
	deviceShading := e.transformShadingToDevice(shading)

	// Prefer the canvas-native shading renderer when available.
	if e.canvas != nil {
		e.syncCanvasColors()
		pattern := entity.NewShadingPattern("", deviceShading)
		if shading.GetShadingType() == entity.ShadingAxial || shading.GetShadingType() == entity.ShadingRadial {
			pattern = entity.NewShadingPattern("", shading)
			pattern.SetMatrix(e.graphics.transform)
		}
		if err := e.canvas.DrawShadingPattern(pattern, bbox); err == nil {
			return nil
		}
	}

	switch deviceShading.GetShadingType() {
	case entity.ShadingAxial:
		return e.renderAxialShading(deviceShading, bbox)
	case entity.ShadingRadial:
		return e.renderRadialShading(deviceShading, bbox)
	case entity.ShadingFunctionBased, entity.ShadingFreeFormGouraud,
		entity.ShadingLatticeGouraud, entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
		startColor, _ := e.getShadingColors(deviceShading)
		return e.fillShadingBBox(startColor, bbox)
	default:
		return nil
	}
}

func (e *Evaluator) currentShadingBBoxForShading(shading *entity.Shading) ([4]float64, bool) {
	if shading != nil && !shading.HasBBox() && e.canvas != nil {
		type currentClipBBoxCanvas interface {
			CurrentClipBBox() ([4]float64, bool)
		}
		if clipCanvas, ok := e.canvas.(currentClipBBoxCanvas); ok {
			if bbox, ok := clipCanvas.CurrentClipBBox(); ok && bbox[2] > bbox[0] && bbox[3] > bbox[1] {
				return bbox, true
			}
		}
	}
	return e.currentShadingBBox()
}

func (e *Evaluator) currentShadingBBox() ([4]float64, bool) {
	if e.graphics.pathClip != nil && !e.graphics.pathClip.IsEmpty() {
		xMin, yMin, xMax, yMax := e.graphics.pathClip.GetBounds()
		if xMax > xMin && yMax > yMin {
			return [4]float64{xMin, yMin, xMax, yMax}, true
		}
	}
	if !e.graphics.path.IsEmpty() {
		xMin, yMin, xMax, yMax := e.graphics.path.GetBounds()
		if xMax > xMin && yMax > yMin {
			return [4]float64{xMin, yMin, xMax, yMax}, true
		}
	}

	if e.canvas == nil {
		return [4]float64{}, false
	}
	b := e.canvas.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return [4]float64{}, false
	}
	return [4]float64{float64(b.Min.X), float64(b.Min.Y), float64(b.Max.X), float64(b.Max.Y)}, true
}

func (e *Evaluator) transformShadingToDevice(shading *entity.Shading) *entity.Shading {
	if shading == nil {
		return nil
	}

	transformed := *shading
	coords := shading.GetCoords()
	if len(coords) > 0 {
		mapped := append([]float64(nil), coords...)
		switch shading.GetShadingType() {
		case entity.ShadingAxial:
			if len(mapped) >= 4 {
				x0, y0 := e.transformPoint(mapped[0], mapped[1])
				x1, y1 := e.transformPoint(mapped[2], mapped[3])
				mapped[0], mapped[1] = x0, y0
				mapped[2], mapped[3] = x1, y1
			}
		case entity.ShadingRadial:
			if len(mapped) >= 6 {
				x0, y0 := e.transformPoint(mapped[0], mapped[1])
				x1, y1 := e.transformPoint(mapped[3], mapped[4])
				scale := matrixAverageScale(e.graphics.transform)
				mapped[0], mapped[1] = x0, y0
				mapped[3], mapped[4] = x1, y1
				mapped[2] *= scale
				mapped[5] *= scale
			}
		}
		transformed.SetCoords(mapped)
	}

	if shading.GetShadingType() == entity.ShadingFunctionBased {
		transformed.SetMatrix(multiplyMatrix(e.graphics.transform, shading.GetMatrix()))
	}

	return &transformed
}

func matrixAverageScale(m [6]float64) float64 {
	sx := math.Hypot(m[0], m[1])
	sy := math.Hypot(m[2], m[3])
	switch {
	case sx == 0 && sy == 0:
		return 1
	case sx == 0:
		return sy
	case sy == 0:
		return sx
	default:
		return (sx + sy) / 2
	}
}

func (e *Evaluator) fillShadingBBox(fill color.Color, bbox [4]float64) error {
	if e.canvas == nil {
		return nil
	}

	x := bbox[0]
	y := bbox[1]
	w := bbox[2] - bbox[0]
	h := bbox[3] - bbox[1]
	if w <= 0 || h <= 0 {
		return nil
	}

	e.canvas.SetFillColor(fill)
	e.canvas.Rectangle(x, y, w, h)
	e.canvas.Fill()
	return nil
}

// renderAxialShading renders an axial shading (linear gradient).
func (e *Evaluator) renderAxialShading(shading *entity.Shading, bbox [4]float64) error {
	coords := shading.GetCoords()
	if len(coords) < 4 {
		return fmt.Errorf("axial shading: invalid coordinates")
	}

	// Get gradient start and end points
	x0, y0 := coords[0], coords[1]
	x1, y1 := coords[2], coords[3]

	// Calculate gradient vector
	dx := x1 - x0
	dy := y1 - y0

	// If extend flags are set, extend the gradient beyond start/end
	if shading.GetExtend()[0] {
		// Extend before start
		x0 -= dx * 100
		y0 -= dy * 100
	}
	if shading.GetExtend()[1] {
		// Extend beyond end
		x1 += dx * 100
		y1 += dy * 100
	}

	// Calculate color at start and end
	startColor, _ := e.getShadingColors(shading)

	// Draw the gradient
	// For now, use a simple horizontal/vertical gradient
	// A full implementation would use proper gradient interpolation
	return e.fillShadingBBox(startColor, bbox)
}

// renderRadialShading renders a radial shading (radial gradient).
func (e *Evaluator) renderRadialShading(shading *entity.Shading, bbox [4]float64) error {
	coords := shading.GetCoords()
	if len(coords) < 6 {
		return fmt.Errorf("radial shading: invalid coordinates")
	}

	// Get circle centers and radii
	_, _, _ = coords[0], coords[1], coords[2] // x0, y0, r0
	_, _, _ = coords[3], coords[4], coords[5] // x1, y1, r1

	// Calculate color at start and end
	startColor, _ := e.getShadingColors(shading)

	// Draw the radial gradient.
	return e.fillShadingBBox(startColor, bbox)
}

// getShadingColors returns the start and end colors for a shading.
func (e *Evaluator) getShadingColors(shading *entity.Shading) (startColor, endColor color.Color) {
	if shading != nil {
		functions := shading.GetFunctions()
		if len(functions) > 0 && functions[0] != nil {
			startColor = evaluateShadingFunctionColor(functions[0], 0.0)
			endColor = evaluateShadingFunctionColor(functions[0], 1.0)
			return startColor, endColor
		}
	}

	// Default: white to black gradient
	startColor = color.White
	endColor = color.Black

	return startColor, endColor
}

func evaluateShadingFunctionColor(fn entity.Function, t float64) color.Color {
	values, err := fn.Evaluate([]float64{t})
	if err != nil || len(values) == 0 {
		return color.White
	}

	clamp := func(v float64) uint8 {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		return colorspace.ConvertComponentToByte(v)
	}

	if len(values) == 1 {
		gray := clamp(values[0])
		return color.RGBA{R: gray, G: gray, B: gray, A: 255}
	}

	r := clamp(values[0])
	g := r
	b := r
	if len(values) > 1 {
		g = clamp(values[1])
	}
	if len(values) > 2 {
		b = clamp(values[2])
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
