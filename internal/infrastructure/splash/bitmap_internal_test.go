package splash

import "testing"

func TestBitmapClearWithAlphaClearsFreshAlpha(t *testing.T) {
	bm := NewBitmap(2, 2, ModeRGB8, true)
	bm.alpha[0] = 123

	bm.ClearWithAlpha(Color{}, 0)

	for i, alpha := range bm.alpha {
		if alpha != 0 {
			t.Fatalf("alpha[%d] = %d, want 0", i, alpha)
		}
	}
}
