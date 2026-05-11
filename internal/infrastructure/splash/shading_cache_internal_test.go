package splash

import (
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type linearTestFunction struct{}

func (linearTestFunction) Evaluate(inputs []float64) ([]float64, error) {
	if len(inputs) == 0 {
		return []float64{0}, nil
	}
	return []float64{inputs[0]}, nil
}

func (linearTestFunction) GetInputSize() int { return 1 }

func (linearTestFunction) GetOutputSize() int { return 1 }

func (linearTestFunction) GetDomain() [][2]float64 { return [][2]float64{{0, 1}} }

func (linearTestFunction) GetRange() [][2]float64 { return [][2]float64{{0, 1}} }

func TestSplashAxialParameterRangeUsesClippedBBox(t *testing.T) {
	shading := entity.NewAxialShading("DeviceGray", 0, 0, 10, 0, []entity.Function{linearTestFunction{}}, [2]bool{})
	lower, upper, ok := splashUnivariateParameterRange(shading, [4]float64{2, -1, 5, 1})
	if !ok {
		t.Fatal("expected axial range")
	}
	if math.Abs(lower-0.2) > 1e-12 || math.Abs(upper-0.5) > 1e-12 {
		t.Fatalf("range: got %.12f..%.12f, want 0.2..0.5", lower, upper)
	}
}

func TestSplashRadialParameterRangeMatchesPopplerConcentricBox(t *testing.T) {
	shading := entity.NewRadialShading("DeviceGray", 0, 0, 0, 0, 0, 10, []entity.Function{linearTestFunction{}}, [2]bool{})
	lower, upper, ok := splashUnivariateParameterRange(shading, [4]float64{-5, -5, 5, 5})
	if !ok {
		t.Fatal("expected radial range")
	}
	if math.Abs(lower) > 1e-12 {
		t.Fatalf("lower: got %.12f, want 0", lower)
	}
	wantUpper := math.Sqrt(50) / 10
	if math.Abs(upper-wantUpper) > 1e-5 {
		t.Fatalf("upper: got %.12f, want %.12f", upper, wantUpper)
	}
}

func TestSplashUnivariateCacheSamplesOnlyParameterRange(t *testing.T) {
	shading := entity.NewAxialShading("DeviceGray", 0, 0, 10, 0, []entity.Function{linearTestFunction{}}, [2]bool{})
	cache := newSplashUnivariateShadingColorCache(shading, [6]float64{1, 0, 0, 1, 0, 0}, [4]float64{2, 0, 4, 1})
	if cache == nil {
		t.Fatal("expected cache")
	}
	if len(cache.bounds) != 2 {
		t.Fatalf("cache size: got %d, want 2", len(cache.bounds))
	}
	if math.Abs(cache.bounds[0]-0.2) > 1e-12 || math.Abs(cache.bounds[1]-0.4) > 1e-12 {
		t.Fatalf("bounds: got %.12f..%.12f, want 0.2..0.4", cache.bounds[0], cache.bounds[1])
	}
}
