package splash

import (
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

const shadingFunctionConstantTolerance = 1e-9

func shadingHasVaryingSampledFunction(shading *entity.Shading) bool {
	if shading == nil {
		return false
	}
	for _, fn := range shading.GetFunctions() {
		if sampledFunctionMayVaryRecursive(fn) {
			return true
		}
	}
	return false
}

func sampledFunctionMayVaryRecursive(fn entity.Function) bool {
	if fn == nil {
		return false
	}
	switch typed := fn.(type) {
	case *entity.SampledFunction:
		return sampledFunctionMayVary(typed)
	case *entity.StitchingFunction:
		for _, child := range typed.Functions {
			if sampledFunctionMayVaryRecursive(child) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func sampledFunctionMayVary(fn *entity.SampledFunction) bool {
	if fn == nil {
		return false
	}
	totalPoints := 1
	for _, size := range fn.Size {
		if size <= 0 {
			return false
		}
		totalPoints *= size
	}
	if totalPoints <= 0 {
		return false
	}
	outputs := len(fn.RangeVal)
	if outputs == 0 {
		outputs = len(fn.Decode)
	}
	if outputs == 0 && len(fn.Samples) >= totalPoints {
		outputs = len(fn.Samples) / totalPoints
	}
	if outputs <= 0 || len(fn.Samples) < totalPoints*outputs {
		return false
	}
	for out := 0; out < outputs; out++ {
		first := fn.Samples[out]
		for point := 1; point < totalPoints; point++ {
			if math.Abs(fn.Samples[point*outputs+out]-first) > shadingFunctionConstantTolerance {
				return true
			}
		}
	}
	return false
}
