package splash

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

func TestSplashClipToRectMasksFillAA(t *testing.T) {
	bm := NewBitmap(8, 8, ModeRGB8, false)
	bm.Clear(Color{0xFF, 0xFF, 0xFF})
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SetFillPattern(NewSolidColor(Color{}))
	if err := s.ClipToRect(2, 2, 6, 6); err != nil {
		t.Fatalf("ClipToRect: %v", err)
	}

	p := xpath.NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	_ = p.LineTo(8, 0)
	_ = p.LineTo(8, 8)
	_ = p.LineTo(0, 8)
	_ = p.Close(true)
	if err := s.Fill(p, false); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	assertRGB := func(x, y int, want Color) {
		t.Helper()
		off := y*bm.rowSize + x*3
		got := Color{bm.data[off], bm.data[off+1], bm.data[off+2]}
		if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got[:3], want[:3])
		}
	}
	assertRGB(1, 3, Color{0xFF, 0xFF, 0xFF})
	assertRGB(2, 3, Color{})
	assertRGB(5, 5, Color{})
	assertRGB(6, 5, Color{0xFF, 0xFF, 0xFF})
}
