package splash

import (
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

func TestApplyTextGlyphPathIntegerSlopeBiasScopesToFlag(t *testing.T) {
	xPath := &xpath.XPath{Segs: []xpath.XPathSeg{
		{X0: 0, Y0: 0, X1: 3, Y1: 1, DXDY: 3, DYDX: 1.0 / 3},
		{X0: 0, Y0: 0, X1: 2.5, Y1: 1, DXDY: 2.5, DYDX: 0.4},
		{X0: 0, Y0: 0, X1: 0, Y1: 1, DXDY: 0, Flags: xpath.XPathVert},
	}}

	splash := &Splash{}
	splash.applyTextGlyphPathIntegerSlopeBias(xPath)
	if xPath.Segs[0].DXDY != 3 {
		t.Fatalf("disabled bias changed slope: %.17g", xPath.Segs[0].DXDY)
	}

	splash.textGlyphPathIntegerSlopeBias = true
	splash.applyTextGlyphPathIntegerSlopeBias(xPath)

	if !(xPath.Segs[0].DXDY < 3 && math.Abs(xPath.Segs[0].DXDY-(3-textGlyphIntegerSlopeBiasEpsilon)) < 1e-15) {
		t.Fatalf("integer slope not nudged toward zero: %.17g", xPath.Segs[0].DXDY)
	}
	if xPath.Segs[1].DXDY != 2.5 {
		t.Fatalf("non-integer slope changed: %.17g", xPath.Segs[1].DXDY)
	}
	if xPath.Segs[2].DXDY != 0 {
		t.Fatalf("vertical slope changed: %.17g", xPath.Segs[2].DXDY)
	}
}
