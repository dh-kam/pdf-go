// Package pattern provides pattern parsing for PDF documents.
//
//revive:disable:exported
package pattern

import (
	"fmt"
	"image/color"
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// DictKey is a type for PDF dictionary keys.
type DictKey string

const (
	KeyType              DictKey = "Type"
	KeySubtype           DictKey = "Subtype"
	KeyPatternType       DictKey = "PatternType"
	KeyPaintType         DictKey = "PaintType"
	KeyTilingType        DictKey = "TilingType"
	KeyBBox              DictKey = "BBox"
	KeyXStep             DictKey = "XStep"
	KeyYStep             DictKey = "YStep"
	KeyMatrix            DictKey = "Matrix"
	KeyResources         DictKey = "Resources"
	KeyShading           DictKey = "Shading"
	KeyFunction          DictKey = "Function"
	KeyFunctions         DictKey = "Functions"
	KeyColorSpace        DictKey = "ColorSpace"
	KeyBackground        DictKey = "Background"
	KeyAntiAlias         DictKey = "AntiAlias"
	KeyCoords            DictKey = "Coords"
	KeyDomain            DictKey = "Domain"
	KeyMatrixShading     DictKey = "Matrix"
	KeyExtend            DictKey = "Extend"
	KeyBitsPerFlag       DictKey = "BitsPerFlag"
	KeyBitsPerCoordinate DictKey = "BitsPerCoordinate"
	KeyBitsPerComponent  DictKey = "BitsPerComponent"
	KeyDecode            DictKey = "Decode"
	KeyData              DictKey = "Data"
)

// ParsePattern parses a pattern from a PDF dictionary.
func ParsePattern(dict *entity.Dict, xref entity.XRef, name string) (entity.Pattern, error) {
	if dict == nil {
		return nil, errors.Invalid("pattern", nil)
	}

	// Get pattern type
	patternTypeObj := dict.Get(entity.NewName(string(KeyPatternType)))
	if patternTypeObj == nil {
		return nil, errors.Missing("pattern_type")
	}

	var patternType entity.PatternType
	obj, ok := patternTypeObj.(*entity.Integer)
	if !ok {
		return nil, errors.Invalid("pattern_type", nil)
	}
	patternType = entity.PatternType(obj.Value())

	switch patternType {
	case entity.PatternTiling:
		return ParseTilingPattern(dict, xref, name)
	case entity.PatternShading:
		return ParseShadingPattern(dict, xref, name)
	default:
		return nil, errors.Invalid("pattern_type", fmt.Errorf("unknown pattern type: %d", patternType))
	}
}

// ParseTilingPattern parses a tiling pattern from a PDF dictionary.
func ParseTilingPattern(dict *entity.Dict, xref entity.XRef, name string) (*entity.TilingPattern, error) {
	pattern := entity.NewTilingPattern(name, 1, entity.TilingConstantSpacing)

	// Parse PaintType (1=colored, 2=uncolored)
	paintTypeObj := dict.Get(entity.NewName(string(KeyPaintType)))
	if paintTypeObj != nil {
		if obj, ok := paintTypeObj.(*entity.Integer); ok {
			pattern.SetPaintType(int(obj.Value()))
		}
	}

	// Parse TilingType (1, 2, or 3)
	tilingTypeObj := dict.Get(entity.NewName(string(KeyTilingType)))
	if tilingTypeObj != nil {
		if obj, ok := tilingTypeObj.(*entity.Integer); ok {
			pattern.SetTilingType(entity.TilingType(obj.Value()))
		}
	}

	// Parse BBox
	bboxObj := dict.Get(entity.NewName(string(KeyBBox)))
	if bboxObj != nil {
		bbox, err := parseFloatArray(bboxObj, 4)
		if err == nil {
			pattern.SetBBox([4]float64{bbox[0], bbox[1], bbox[2], bbox[3]})
		}
	}

	// Parse XStep
	xStepObj := dict.Get(entity.NewName(string(KeyXStep)))
	if xStepObj != nil {
		switch obj := xStepObj.(type) {
		case *entity.Real:
			pattern.SetXStep(obj.Value())
		case *entity.Integer:
			pattern.SetXStep(float64(obj.Value()))
		}
	}

	// Parse YStep
	yStepObj := dict.Get(entity.NewName(string(KeyYStep)))
	if yStepObj != nil {
		switch obj := yStepObj.(type) {
		case *entity.Real:
			pattern.SetYStep(obj.Value())
		case *entity.Integer:
			pattern.SetYStep(float64(obj.Value()))
		}
	}

	// Parse Matrix
	matrixObj := dict.Get(entity.NewName(string(KeyMatrix)))
	if matrixObj != nil {
		matrix, err := parseFloatArray(matrixObj, 6)
		if err == nil {
			pattern.SetMatrix([6]float64{matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]})
		}
	}

	// Parse Resources
	resourcesObj := dict.Get(entity.NewName(string(KeyResources)))
	if resourcesObj != nil {
		if obj, ok := resourcesObj.(*entity.Dict); ok {
			pattern.SetResources(obj)
		}
	}

	return pattern, nil
}

// ParseShadingPattern parses a shading pattern from a PDF dictionary.
func ParseShadingPattern(dict *entity.Dict, xref entity.XRef, name string) (*entity.ShadingPattern, error) {
	// Get the shading object
	shadingObj := dict.Get(entity.NewName(string(KeyShading)))
	if shadingObj == nil {
		return nil, errors.Missing("shading")
	}

	var shadingDict *entity.Dict
	switch obj := shadingObj.(type) {
	case *entity.Dict:
		shadingDict = obj
	case entity.Ref:
		// Dereference the indirect object
		resolved, err := xref.Fetch(obj)
		if err != nil {
			return nil, err
		}
		switch res := resolved.(type) {
		case *entity.Dict:
			shadingDict = res
		case *entity.Stream:
			shadingDict = res.Dict()
		default:
			return nil, errors.Invalid("shading", nil)
		}
	case *entity.Stream:
		shadingDict = obj.Dict()
	default:
		return nil, errors.Invalid("shading", nil)
	}

	shading, err := ParseShading(shadingDict, xref)
	if err != nil {
		return nil, err
	}

	pattern := entity.NewShadingPattern(name, shading)

	// Parse Matrix
	matrixObj := dict.Get(entity.NewName(string(KeyMatrix)))
	if matrixObj != nil {
		matrix, err := parseFloatArray(matrixObj, 6)
		if err == nil {
			pattern.SetMatrix([6]float64{matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]})
		}
	}

	return pattern, nil
}

// ParseShading parses a shading object from a PDF dictionary.
func ParseShading(dict *entity.Dict, xref entity.XRef) (*entity.Shading, error) {
	if dict == nil {
		return nil, errors.Invalid("shading", nil)
	}

	// Get shading type
	shadingTypeObj := dict.Get(entity.NewName("ShadingType"))
	if shadingTypeObj == nil {
		return nil, errors.Missing("shading_type")
	}

	var shadingType entity.ShadingType
	obj, ok := shadingTypeObj.(*entity.Integer)
	if !ok {
		return nil, errors.Invalid("shading_type", nil)
	}
	shadingType = entity.ShadingType(obj.Value())

	// Get color space
	colorSpaceObj := dict.Get(entity.NewName(string(KeyColorSpace)))
	if colorSpaceObj == nil {
		return nil, errors.Missing("color_space")
	}

	var colorSpace string
	switch obj := colorSpaceObj.(type) {
	case entity.Name:
		colorSpace = obj.Value()
	case *entity.String:
		colorSpace = obj.Value()
	default:
		return nil, errors.Invalid("color_space", nil)
	}

	shading := entity.NewShading(shadingType, colorSpace)

	// Parse Background
	backgroundObj := dict.Get(entity.NewName(string(KeyBackground)))
	if backgroundObj != nil {
		background, err := parseBackgroundColor(backgroundObj, colorSpace)
		if err == nil {
			shading.SetBackground(background)
		}
	}

	// Parse BBox
	bboxObj := dict.Get(entity.NewName(string(KeyBBox)))
	if bboxObj != nil {
		bbox, err := parseFloatArray(bboxObj, 4)
		if err == nil {
			shading.SetBBox([4]float64{bbox[0], bbox[1], bbox[2], bbox[3]})
		}
	}

	// Parse AntiAlias
	antiAliasObj := dict.Get(entity.NewName(string(KeyAntiAlias)))
	if antiAliasObj != nil {
		if obj, ok := antiAliasObj.(*entity.Boolean); ok {
			shading.SetAntiAlias(obj.Value())
		}
	}

	// Parse type-specific fields
	switch shadingType {
	case entity.ShadingFunctionBased:
		if err := parseFunctionBasedShading(shading, dict); err != nil {
			return nil, err
		}
	case entity.ShadingAxial:
		if err := parseAxialShading(shading, dict); err != nil {
			return nil, err
		}
	case entity.ShadingRadial:
		if err := parseRadialShading(shading, dict); err != nil {
			return nil, err
		}
	case entity.ShadingFreeFormGouraud, entity.ShadingLatticeGouraud:
		if err := parseGouraudShading(shading, dict); err != nil {
			return nil, err
		}
	case entity.ShadingCoonsPatch, entity.ShadingTensorProductPatch:
		if err := parsePatchMeshShading(shading, dict); err != nil {
			return nil, err
		}
	}

	return shading, nil
}

// parseFunctionBasedShading parses function-based shading (type 1).
func parseFunctionBasedShading(shading *entity.Shading, dict *entity.Dict) error {
	// Parse Domain
	domainObj := dict.Get(entity.NewName(string(KeyDomain)))
	if domainObj != nil {
		domain, err := parseFloatArray(domainObj, 4)
		if err == nil {
			shading.SetDomain([4]float64{domain[0], domain[1], domain[2], domain[3]})
		}
	}

	// Parse Matrix
	matrixObj := dict.Get(entity.NewName(string(KeyMatrixShading)))
	if matrixObj != nil {
		matrix, err := parseFloatArray(matrixObj, 6)
		if err == nil {
			shading.SetMatrix([6]float64{matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]})
		}
	}

	// Parse Function(s)
	functionObj := dict.Get(entity.NewName(string(KeyFunction)))
	if functionObj != nil {
		function, err := parseFunction(functionObj)
		if err != nil {
			return err
		}
		shading.SetFunctions([]entity.Function{function})
	}

	return nil
}

// parseAxialShading parses axial shading (type 2).
func parseAxialShading(shading *entity.Shading, dict *entity.Dict) error {
	// Parse Coords [x0 y0 x1 y1]
	coordsObj := dict.Get(entity.NewName(string(KeyCoords)))
	if coordsObj != nil {
		coords, err := parseFloatArray(coordsObj, 4)
		if err == nil {
			shading.SetCoords(coords)
		}
	}

	// Parse Function(s)
	functionObj := dict.Get(entity.NewName(string(KeyFunction)))
	if functionObj != nil {
		function, err := parseFunction(functionObj)
		if err != nil {
			return err
		}
		shading.SetFunctions([]entity.Function{function})
	}

	// Parse Extend [extend0 extend1]
	extendObj := dict.Get(entity.NewName(string(KeyExtend)))
	if extendObj != nil {
		extend, err := parseBoolArray(extendObj, 2)
		if err == nil {
			shading.SetExtend(extend)
		}
	}

	return nil
}

// parseRadialShading parses radial shading (type 3).
func parseRadialShading(shading *entity.Shading, dict *entity.Dict) error {
	// Parse Coords [x0 y0 r0 x1 y1 r1]
	coordsObj := dict.Get(entity.NewName(string(KeyCoords)))
	if coordsObj != nil {
		coords, err := parseFloatArray(coordsObj, 6)
		if err == nil {
			shading.SetCoords(coords)
		}
	}

	// Parse Function(s)
	functionObj := dict.Get(entity.NewName(string(KeyFunction)))
	if functionObj != nil {
		function, err := parseFunction(functionObj)
		if err != nil {
			return err
		}
		shading.SetFunctions([]entity.Function{function})
	}

	// Parse Extend [extend0 extend1]
	extendObj := dict.Get(entity.NewName(string(KeyExtend)))
	if extendObj != nil {
		extend, err := parseBoolArray(extendObj, 2)
		if err == nil {
			shading.SetExtend(extend)
		}
	}

	return nil
}

// parseGouraudShading parses Gouraud-shaded triangle mesh (type 4 or 5).
func parseGouraudShading(shading *entity.Shading, dict *entity.Dict) error {
	// Parse BitsPerCoordinate
	bitsPerCoordObj := dict.Get(entity.NewName(string(KeyBitsPerCoordinate)))
	if bitsPerCoordObj != nil {
		if obj, ok := bitsPerCoordObj.(*entity.Integer); ok {
			shading.SetBitsPerCoord(int(obj.Value()))
		}
	}

	// Parse BitsPerComponent
	bitsPerCompObj := dict.Get(entity.NewName(string(KeyBitsPerComponent)))
	if bitsPerCompObj != nil {
		if obj, ok := bitsPerCompObj.(*entity.Integer); ok {
			shading.SetBitsPerComp(int(obj.Value()))
		}
	}

	// Parse Decode array
	decodeObj := dict.Get(entity.NewName(string(KeyDecode)))
	if decodeObj != nil {
		decode, err := parseFloatArray(decodeObj, 0)
		if err == nil {
			shading.SetDecode(decode)
		}
	}

	return nil
}

// parsePatchMeshShading parses patch mesh shading (type 6 or 7).
func parsePatchMeshShading(shading *entity.Shading, dict *entity.Dict) error {
	// Parse BitsPerFlag
	bitsPerFlagObj := dict.Get(entity.NewName(string(KeyBitsPerFlag)))
	if bitsPerFlagObj != nil {
		if obj, ok := bitsPerFlagObj.(*entity.Integer); ok {
			shading.SetBitsPerFlag(int(obj.Value()))
		}
	}

	// Parse BitsPerCoordinate
	bitsPerCoordObj := dict.Get(entity.NewName(string(KeyBitsPerCoordinate)))
	if bitsPerCoordObj != nil {
		if obj, ok := bitsPerCoordObj.(*entity.Integer); ok {
			shading.SetBitsPerCoord(int(obj.Value()))
		}
	}

	// Parse BitsPerComponent
	bitsPerCompObj := dict.Get(entity.NewName(string(KeyBitsPerComponent)))
	if bitsPerCompObj != nil {
		if obj, ok := bitsPerCompObj.(*entity.Integer); ok {
			shading.SetBitsPerComp(int(obj.Value()))
		}
	}

	// Parse Decode array
	decodeObj := dict.Get(entity.NewName(string(KeyDecode)))
	if decodeObj != nil {
		decode, err := parseFloatArray(decodeObj, 0)
		if err == nil {
			shading.SetDecode(decode)
		}
	}

	return nil
}

// parseFunction parses a function object.
func parseFunction(obj entity.Object) (entity.Function, error) {
	if obj == nil {
		return nil, errors.Missing("function")
	}

	var dict *entity.Dict
	switch o := obj.(type) {
	case *entity.Dict:
		dict = o
	case *entity.Stream:
		dict = o.Dict()
	default:
		return nil, errors.Invalid("function", nil)
	}

	// Get function type
	functionTypeObj := dict.Get(entity.NewName("FunctionType"))
	if functionTypeObj == nil {
		return nil, errors.Missing("function_type")
	}

	var functionType int
	intObj, ok := functionTypeObj.(*entity.Integer)
	if !ok {
		return nil, errors.Invalid("function_type", nil)
	}
	functionType = int(intObj.Value())

	switch functionType {
	case 0:
		return parseSampledFunction(dict, obj)
	case 2:
		return parseExponentialFunction(dict)
	case 3:
		return parseStitchingFunction(dict)
	case 4:
		return parsePostScriptFunction(dict, obj)
	default:
		return nil, errors.Invalid("function_type", fmt.Errorf("unsupported function type: %d", functionType))
	}
}

// parseSampledFunction parses a sampled function (type 0).
func parseSampledFunction(dict *entity.Dict, obj entity.Object) (*entity.SampledFunction, error) {
	sampledFunc := &entity.SampledFunction{
		Interpolate: true,
	}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		domain, err := parseFloatArray(domainObj, 0)
		if err == nil && len(domain) >= 2 {
			sampledFunc.Domain = make([][2]float64, len(domain)/2)
			for i := 0; i < len(domain)/2; i++ {
				sampledFunc.Domain[i] = [2]float64{domain[i*2], domain[i*2+1]}
			}
		}
	}

	// Parse Range
	rangeObj := dict.Get(entity.NewName("Range"))
	if rangeObj != nil {
		rng, err := parseFloatArray(rangeObj, 0)
		if err == nil && len(rng) >= 2 {
			sampledFunc.RangeVal = make([][2]float64, len(rng)/2)
			for i := 0; i < len(rng)/2; i++ {
				sampledFunc.RangeVal[i] = [2]float64{rng[i*2], rng[i*2+1]}
			}
		}
	}

	// Parse Size
	sizeObj := dict.Get(entity.NewName("Size"))
	if sizeObj != nil {
		size, err := parseIntArray(sizeObj)
		if err == nil {
			sampledFunc.Size = size
		}
	}

	// Parse Samples
	if stream, ok := obj.(*entity.Stream); ok {
		data, err := stream.Decode()
		if err == nil {
			sampledFunc.Samples = parseSampleValues(data)
		}
	}

	// Parse Encode
	encodeObj := dict.Get(entity.NewName("Encode"))
	if encodeObj != nil {
		encode, err := parseFloatArray(encodeObj, 0)
		if err == nil && len(encode) >= 2 {
			sampledFunc.Encode = make([][2]float64, len(encode)/2)
			for i := 0; i < len(encode)/2; i++ {
				sampledFunc.Encode[i] = [2]float64{encode[i*2], encode[i*2+1]}
			}
		}
	}

	// Parse Decode
	decodeObj := dict.Get(entity.NewName("Decode"))
	if decodeObj != nil {
		decode, err := parseFloatArray(decodeObj, 0)
		if err == nil && len(decode) >= 2 {
			sampledFunc.Decode = make([][2]float64, len(decode)/2)
			for i := 0; i < len(decode)/2; i++ {
				sampledFunc.Decode[i] = [2]float64{decode[i*2], decode[i*2+1]}
			}
		}
	}

	return sampledFunc, nil
}

// parseExponentialFunction parses an exponential interpolation function (type 2).
func parseExponentialFunction(dict *entity.Dict) (*entity.ExponentialFunction, error) {
	expFunc := &entity.ExponentialFunction{}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		domain, err := parseFloatArray(domainObj, 2)
		if err == nil {
			expFunc.Domain = [][2]float64{{domain[0], domain[1]}}
		}
	}

	// Parse Range
	rangeObj := dict.Get(entity.NewName("Range"))
	if rangeObj != nil {
		rng, err := parseFloatArray(rangeObj, 0)
		if err == nil && len(rng) >= 2 {
			expFunc.RangeVal = make([][2]float64, len(rng)/2)
			for i := 0; i < len(rng)/2; i++ {
				expFunc.RangeVal[i] = [2]float64{rng[i*2], rng[i*2+1]}
			}
		}
	}

	// Parse C0
	c0Obj := dict.Get(entity.NewName("C0"))
	if c0Obj != nil {
		c0, err := parseFloatArray(c0Obj, 0)
		if err == nil {
			expFunc.C0 = c0
		}
	}
	if len(expFunc.C0) == 0 {
		expFunc.C0 = []float64{0.0}
	}

	// Parse C1
	c1Obj := dict.Get(entity.NewName("C1"))
	if c1Obj != nil {
		c1, err := parseFloatArray(c1Obj, 0)
		if err == nil {
			expFunc.C1 = c1
		}
	}
	if len(expFunc.C1) == 0 {
		expFunc.C1 = []float64{1.0}
	}

	// Parse N (exponent)
	nObj := dict.Get(entity.NewName("N"))
	if nObj != nil {
		switch obj := nObj.(type) {
		case *entity.Real:
			expFunc.Exponent = obj.Value()
		case *entity.Integer:
			expFunc.Exponent = float64(obj.Value())
		}
	}

	expFunc.N = len(expFunc.C0)
	if len(expFunc.C1) > expFunc.N {
		expFunc.N = len(expFunc.C1)
	}

	return expFunc, nil
}

// parseStitchingFunction parses a stitching function (type 3).
func parseStitchingFunction(dict *entity.Dict) (*entity.StitchingFunction, error) {
	stitchFunc := &entity.StitchingFunction{}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		domain, err := parseFloatArray(domainObj, 2)
		if err == nil {
			stitchFunc.Domain = [][2]float64{{domain[0], domain[1]}}
		}
	}

	// Parse Range
	rangeObj := dict.Get(entity.NewName("Range"))
	if rangeObj != nil {
		rng, err := parseFloatArray(rangeObj, 0)
		if err == nil && len(rng) >= 2 {
			stitchFunc.RangeVal = make([][2]float64, len(rng)/2)
			for i := 0; i < len(rng)/2; i++ {
				stitchFunc.RangeVal[i] = [2]float64{rng[i*2], rng[i*2+1]}
			}
		}
	}

	// Parse Functions
	functionsObj := dict.Get(entity.NewName("Functions"))
	if functionsObj != nil {
		functions, err := parseFunctionArray(functionsObj)
		if err == nil {
			stitchFunc.Functions = functions
		}
	}

	// Parse Bounds
	boundsObj := dict.Get(entity.NewName("Bounds"))
	if boundsObj != nil {
		bounds, err := parseFloatArray(boundsObj, 0)
		if err == nil {
			stitchFunc.Bounds = bounds
		}
	}

	// Parse Encode
	encodeObj := dict.Get(entity.NewName("Encode"))
	if encodeObj != nil {
		encode, err := parseFloatArray(encodeObj, 0)
		if err == nil && len(encode) >= 2 {
			stitchFunc.Encode = make([][2]float64, len(encode)/2)
			for i := 0; i < len(encode)/2; i++ {
				stitchFunc.Encode[i] = [2]float64{encode[i*2], encode[i*2+1]}
			}
		}
	}

	return stitchFunc, nil
}

// parsePostScriptFunction parses a PostScript calculator function (type 4).
func parsePostScriptFunction(dict *entity.Dict, obj entity.Object) (*entity.PostScriptFunction, error) {
	fn := &entity.PostScriptFunction{}

	// Parse Domain
	domainObj := dict.Get(entity.NewName("Domain"))
	if domainObj != nil {
		domain, err := parseFloatArray(domainObj, 0)
		if err == nil && len(domain) >= 2 {
			fn.Domain = make([][2]float64, len(domain)/2)
			for i := 0; i < len(domain)/2; i++ {
				fn.Domain[i] = [2]float64{domain[i*2], domain[i*2+1]}
			}
		}
	}

	// Parse Range
	rangeObj := dict.Get(entity.NewName("Range"))
	if rangeObj != nil {
		rng, err := parseFloatArray(rangeObj, 0)
		if err == nil && len(rng) >= 2 {
			fn.RangeVal = make([][2]float64, len(rng)/2)
			for i := 0; i < len(rng)/2; i++ {
				fn.RangeVal[i] = [2]float64{rng[i*2], rng[i*2+1]}
			}
		}
	}

	// Parse program stream.
	if streamObj, ok := obj.(*entity.Stream); ok {
		data, err := streamObj.Decode()
		if err == nil {
			fn.Program = string(data)
		}
	}

	return fn, nil
}

// parseFunctionArray parses an array of functions.
func parseFunctionArray(obj entity.Object) ([]entity.Function, error) {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil, errors.Invalid("function_array", nil)
	}

	functions := make([]entity.Function, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		funcObj := arr.Get(i)
		if funcObj == nil {
			continue
		}
		fn, err := parseFunction(funcObj)
		if err != nil {
			return nil, err
		}
		functions = append(functions, fn)
	}

	return functions, nil
}

// parseFloatArray parses a float64 array from a PDF object.
func parseFloatArray(obj entity.Object, expectedLen int) ([]float64, error) {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil, errors.Invalid("float_array", nil)
	}

	result := make([]float64, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		item := arr.Get(i)
		if item == nil {
			continue
		}

		var value float64
		switch v := item.(type) {
		case *entity.Integer:
			value = float64(v.Value())
		case *entity.Real:
			value = v.Value()
		default:
			continue
		}

		result = append(result, value)
	}

	if expectedLen > 0 && len(result) != expectedLen {
		return nil, errors.Invalid("float_array_length", nil)
	}

	return result, nil
}

// parseIntArray parses an int array from a PDF object.
func parseIntArray(obj entity.Object) ([]int, error) {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil, errors.Invalid("int_array", nil)
	}

	result := make([]int, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		item := arr.Get(i)
		if item == nil {
			continue
		}

		var value int
		switch v := item.(type) {
		case *entity.Integer:
			value = int(v.Value())
		case *entity.Real:
			value = int(v.Value())
		default:
			continue
		}

		result = append(result, value)
	}

	return result, nil
}

// parseBoolArray parses a bool array from a PDF object.
func parseBoolArray(obj entity.Object, expectedLen int) ([2]bool, error) {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return [2]bool{}, errors.Invalid("bool_array", nil)
	}

	if arr.Len() != expectedLen {
		return [2]bool{}, errors.Invalid("bool_array_length", nil)
	}

	result := [2]bool{false, false}
	for i := 0; i < arr.Len() && i < 2; i++ {
		item := arr.Get(i)
		if item == nil {
			continue
		}

		if v, ok := item.(*entity.Boolean); ok {
			result[i] = v.Value()
		}
	}

	return result, nil
}

// parseSampleValues parses sample values from a byte array.
func parseSampleValues(data []byte) []float64 {
	values := make([]float64, len(data))
	for i, b := range data {
		values[i] = float64(b) / 255.0
	}
	return values
}

func parseBackgroundColor(obj entity.Object, colorSpace string) (color.Color, error) {
	values, err := parseFloatArray(obj, 0)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("background array is empty")
	}

	switch normalizeColorSpaceName(colorSpace) {
	case "DeviceGray":
		gray := uint8(math.Round(clamp01(values[0]) * 255.0))
		return color.Gray{Y: gray}, nil
	case "DeviceCMYK":
		if len(values) < 4 {
			return nil, fmt.Errorf("cmyk background requires 4 values")
		}
		c := clamp01(values[0])
		m := clamp01(values[1])
		y := clamp01(values[2])
		k := clamp01(values[3])
		r := uint8(math.Round((1.0 - math.Min(1.0, c+k)) * 255.0))
		g := uint8(math.Round((1.0 - math.Min(1.0, m+k)) * 255.0))
		b := uint8(math.Round((1.0 - math.Min(1.0, y+k)) * 255.0))
		return color.RGBA{R: r, G: g, B: b, A: 255}, nil
	default:
		if len(values) < 3 {
			return nil, fmt.Errorf("rgb background requires 3 values")
		}
		r := uint8(math.Round(clamp01(values[0]) * 255.0))
		g := uint8(math.Round(clamp01(values[1]) * 255.0))
		b := uint8(math.Round(clamp01(values[2]) * 255.0))
		return color.RGBA{R: r, G: g, B: b, A: 255}, nil
	}
}

func normalizeColorSpaceName(name string) string {
	switch name {
	case "/DeviceGray", "DeviceGray", "G":
		return "DeviceGray"
	case "/DeviceCMYK", "DeviceCMYK", "CMYK":
		return "DeviceCMYK"
	case "/DeviceRGB", "DeviceRGB", "RGB":
		return "DeviceRGB"
	default:
		return "DeviceRGB"
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// ParseAxialGradientPoints parses axial gradient coordinates.
func ParseAxialGradientPoints(shading *entity.Shading) (x0, y0, x1, y1 float64, err error) {
	coords := shading.GetCoords()
	if len(coords) < 4 {
		return 0, 0, 0, 0, fmt.Errorf("invalid axial shading coordinates")
	}
	return coords[0], coords[1], coords[2], coords[3], nil
}

// ParseRadialGradientPoints parses radial gradient coordinates.
func ParseRadialGradientPoints(shading *entity.Shading) (x0, y0, r0, x1, y1, r1 float64, err error) {
	coords := shading.GetCoords()
	if len(coords) < 6 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid radial shading coordinates")
	}
	return coords[0], coords[1], coords[2], coords[3], coords[4], coords[5], nil
}

// ValidatePattern validates a pattern object.
func ValidatePattern(pattern entity.Pattern) error {
	switch p := pattern.(type) {
	case *entity.TilingPattern:
		if p.GetXStep() <= 0 || p.GetYStep() <= 0 {
			return fmt.Errorf("invalid tiling pattern step size")
		}
	case *entity.ShadingPattern:
		shading := p.GetShading()
		if shading == nil {
			return fmt.Errorf("shading pattern has no shading object")
		}
	}
	return nil
}

// TransformPatternPoint applies a pattern's transformation matrix to a point.
func TransformPatternPoint(pattern entity.Pattern, x, y float64) (float64, float64) {
	matrix := pattern.Matrix()
	tx := matrix[0]*x + matrix[2]*y + matrix[4]
	ty := matrix[1]*x + matrix[3]*y + matrix[5]
	return tx, ty
}
