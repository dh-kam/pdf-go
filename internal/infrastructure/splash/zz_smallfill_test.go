package splash

import (
	"testing"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

func TestSmallBitmapFill(t *testing.T) {
	bm := NewBitmap(22, 22, ModeRGB8, false)
	sp, _ := New(bm, true)
	sp.SetFillPattern(NewSolidColor(Color{0xff, 0xff, 0xff}))
	sp.SetFillAlpha(1)
	sp.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	p := xpath.NewPath()
	p.MoveToDroppingEmptySubpath(5, 5)
	p.LineTo(15, 5)
	p.LineTo(15, 15)
	p.LineTo(5, 15)
	p.Close(true)
	if err := sp.Fill(p, false); err != nil {
		t.Fatalf("fill: %v", err)
	}
	sum := 0
	for _, b := range bm.Data() {
		sum += int(b)
	}
	t.Logf("sum=%d over %d bytes", sum, len(bm.Data()))
	if sum == 0 {
		t.Errorf("fill produced no coverage")
	}
}
