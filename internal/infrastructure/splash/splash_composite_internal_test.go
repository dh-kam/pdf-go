package splash

import (
	"math"
	"testing"
)

// rgb is a tiny helper for building 3-component RGB colors in tests.
func rgb(r, g, b byte) Color { return Color{r, g, b, 0, 0, 0, 0, 0} }

// cmyk builds a 4-component color.
func cmyk(c, m, y, k byte) Color { return Color{c, m, y, k, 0, 0, 0, 0} }

func runBlend(f BlendFunc, src, dst Color, mode ColorMode) Color {
	var out Color
	f(&src, &dst, &out, mode)
	return out
}

// -------------------- Per-mode point sample --------------------
//
// PDF spec 11.3.5.2 reference values, computed by hand:
//   Cs = 128 (0.502), Cb = 64 (0.251)
//   Multiply  = (128*64)/255 = 32
//   Screen    = 64 + 128 - Div255(8192) = 192 - 32 = 160
//   Overlay   = HardLight(Cs=64, Cb=128) — Cb<=127 path: 2*64*128/255 -> 64
//               (note: our Overlay swaps src/dst — Cs(src)=128, Cb(dst)=64,
//                so cb<=127 → 2*128*64/255 = 64)
//   Darken    = min(128,64) = 64
//   Lighten   = max(128,64) = 128
//   ColorDodge: cs=128, cs<255 → (64*255)/(255-128) = (64*255)/127 = 128
//   ColorBurn:  cs=128, cs!=0 → 255 - min(255, (255-64)*255/128) = 255 - min(255,(191*255)/128) = 255 - min(255, 380) = 255 - 255 = 0
//   HardLight: cs=128 (>127) → 2*128 + 2*64 - 2*128*64/255 - 255 = 256 + 128 - 64 - 255 = 65
//   SoftLight: cs=128 -> 0.502 > 0.5; cb=0.251 > 0.25 → d = sqrt(0.251) ≈ 0.5010
//              out = 0.251 + (2*0.502 - 1) * (0.5010 - 0.251) = 0.251 + 0.004*0.250 = 0.252  ≈ 64
//   Difference: |128-64| = 64
//   Exclusion:  128 + 64 - 2*Div255(128*64) = 192 - 64 = 128

func TestBlendSeparablePointSample(t *testing.T) {
	src := rgb(128, 128, 128)
	dst := rgb(64, 64, 64)
	cases := []struct {
		name string
		f    BlendFunc
		want byte
	}{
		{"Normal", BlendNormal, 128},
		{"Multiply", BlendMultiply, 32},
		{"Screen", BlendScreen, 160},
		{"Overlay", BlendOverlay, 64},
		{"Darken", BlendDarken, 64},
		{"Lighten", BlendLighten, 128},
		{"ColorDodge", BlendColorDodge, 128},
		{"ColorBurn", BlendColorBurn, 0},
		{"HardLight", BlendHardLight, 65},
		{"Difference", BlendDifference, 64},
		{"Exclusion", BlendExclusion, 128},
	}
	for _, c := range cases {
		got := runBlend(c.f, src, dst, ModeRGB8)
		if got[0] != c.want || got[1] != c.want || got[2] != c.want {
			t.Errorf("%s: got %v, want all=%d", c.name, got[:3], c.want)
		}
	}
}

func TestBlendSoftLightPointSample(t *testing.T) {
	src := rgb(128, 128, 128)
	dst := rgb(64, 64, 64)
	got := runBlend(BlendSoftLight, src, dst, ModeRGB8)
	// Recompute expected with same math (avoid drift from hand-rounding).
	cs := 128.0 / 255.0
	cb := 64.0 / 255.0
	d := math.Sqrt(cb)
	want := byte(Round((cb + (2*cs-1)*(d-cb)) * 255))
	for i := 0; i < 3; i++ {
		if got[i] != want {
			t.Errorf("SoftLight ch%d: got %d, want %d", i, got[i], want)
		}
	}
}

// -------------------- Idempotence on Normal --------------------

func TestBlendNormalIdempotent(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 23 {
			for b := 0; b < 256; b += 31 {
				src := rgb(byte(r), byte(g), byte(b))
				for dr := 0; dr < 256; dr += 51 {
					dst := rgb(byte(dr), 0, 0)
					out := runBlend(BlendNormal, src, dst, ModeRGB8)
					if out[0] != src[0] || out[1] != src[1] || out[2] != src[2] {
						t.Fatalf("Normal not idempotent: src=%v dst=%v out=%v", src[:3], dst[:3], out[:3])
					}
				}
			}
		}
	}
}

// -------------------- Multiply commutativity --------------------

func TestBlendMultiplyCommutative(t *testing.T) {
	for a := 0; a < 256; a += 11 {
		for b := 0; b < 256; b += 13 {
			ca := rgb(byte(a), byte(a), byte(a))
			cb := rgb(byte(b), byte(b), byte(b))
			x := runBlend(BlendMultiply, ca, cb, ModeRGB8)
			y := runBlend(BlendMultiply, cb, ca, ModeRGB8)
			if x != y {
				t.Fatalf("Multiply not commutative: a=%d b=%d, got %v vs %v", a, b, x[:3], y[:3])
			}
		}
	}
}

// -------------------- Darken+Lighten symmetry --------------------

func TestBlendDarkenLightenSymmetry(t *testing.T) {
	for a := 0; a < 256; a += 7 {
		for b := 0; b < 256; b += 5 {
			ca := rgb(byte(a), 0, 0)
			cb := rgb(byte(b), 0, 0)
			d := runBlend(BlendDarken, ca, cb, ModeRGB8)
			l := runBlend(BlendLighten, ca, cb, ModeRGB8)
			// d should be min, l should be max — together they recover {a,b}.
			min, max := byte(a), byte(b)
			if min > max {
				min, max = max, min
			}
			if d[0] != min {
				t.Fatalf("Darken(%d,%d) = %d, want %d", a, b, d[0], min)
			}
			if l[0] != max {
				t.Fatalf("Lighten(%d,%d) = %d, want %d", a, b, l[0], max)
			}
		}
	}
}

// -------------------- Difference anti-symmetric --------------------

func TestBlendDifferenceCommutative(t *testing.T) {
	for a := 0; a < 256; a += 9 {
		for b := 0; b < 256; b += 11 {
			ca := rgb(byte(a), byte(a), byte(a))
			cb := rgb(byte(b), byte(b), byte(b))
			x := runBlend(BlendDifference, ca, cb, ModeRGB8)
			y := runBlend(BlendDifference, cb, ca, ModeRGB8)
			if x != y {
				t.Fatalf("Difference not symmetric: a=%d b=%d, got %v vs %v", a, b, x[:3], y[:3])
			}
		}
	}
}

// -------------------- HardLight at 50% --------------------
//
// PDF spec: HardLight(Cs=0x80, Cb) — boundary case. With cs=128 (>127) the
// "Screen" branch fires: 2*128 + 2*Cb - Div255(2*128*Cb) - 255
//                      = 256 + 2*Cb - Div255(256*Cb) - 255
//                      = 1 + 2*Cb - Div255(256*Cb)
// For Cb=0: 1+0-0 = 1 (NOT exactly 0 due to boundary).
// For Cb=255: 1 + 510 - Div255(65280) = 511 - 256 = 255.
// We assert HardLight(0x80, x) is monotonic in x and reaches 255 at x=255.

func TestBlendHardLightAt128(t *testing.T) {
	src := rgb(0x80, 0x80, 0x80)
	prev := -1
	for cb := 0; cb < 256; cb++ {
		dst := rgb(byte(cb), byte(cb), byte(cb))
		out := runBlend(BlendHardLight, src, dst, ModeRGB8)
		if int(out[0]) < prev {
			t.Fatalf("HardLight(0x80,%d) = %d not monotonic vs prev %d", cb, out[0], prev)
		}
		prev = int(out[0])
	}
	// At cb=255 we expect 255.
	out := runBlend(BlendHardLight, src, rgb(255, 255, 255), ModeRGB8)
	if out[0] != 255 {
		t.Errorf("HardLight(0x80, 255) = %d, want 255", out[0])
	}
	// At cb=0 we expect 0 or 1.
	out = runBlend(BlendHardLight, src, rgb(0, 0, 0), ModeRGB8)
	if out[0] > 1 {
		t.Errorf("HardLight(0x80, 0) = %d, want 0 or 1", out[0])
	}
}

// -------------------- Non-separable: Lum/Sat/Color/Luminosity --------------------
//
// PDF spec 11.3.5.3 — vector-level invariants:
//   Hue        : SetLum(SetSat(Cs, Sat(Cb)), Lum(Cb)) — output Lum ≈ Lum(Cb)
//   Saturation : SetLum(SetSat(Cb, Sat(Cs)), Lum(Cb)) — output Lum ≈ Lum(Cb), Sat ≈ Sat(Cs)
//   Color      : SetLum(Cs, Lum(Cb)) — output Lum ≈ Lum(Cb)
//   Luminosity : SetLum(Cb, Lum(Cs)) — output Lum ≈ Lum(Cs)

func lumRGB(c Color) float64 {
	return 0.3*float64(c[0])/255 + 0.59*float64(c[1])/255 + 0.11*float64(c[2])/255
}

func satRGB(c Color) float64 {
	r := float64(c[0]) / 255
	g := float64(c[1]) / 255
	b := float64(c[2]) / 255
	return sat(r, g, b)
}

func TestBlendNonSeparablePreservation(t *testing.T) {
	// Choose colors with distinct lum/sat to exercise the equations.
	src := rgb(200, 60, 30)  // warm
	dst := rgb(40, 120, 200) // cool
	const tol = 2.0 / 255    // <=2 byte tolerance for round-trip drift

	// Hue: output Lum ≈ Lum(dst), output Sat ≈ Sat(dst)
	hue := runBlend(BlendHue, src, dst, ModeRGB8)
	if math.Abs(lumRGB(hue)-lumRGB(dst)) > tol {
		t.Errorf("Hue: lum=%.4f, want %.4f (lum of dst)", lumRGB(hue), lumRGB(dst))
	}
	if math.Abs(satRGB(hue)-satRGB(dst)) > tol {
		t.Errorf("Hue: sat=%.4f, want %.4f (sat of dst)", satRGB(hue), satRGB(dst))
	}

	// Saturation: output Lum ≈ Lum(dst), Sat ≈ Sat(src)
	s := runBlend(BlendSaturation, src, dst, ModeRGB8)
	if math.Abs(lumRGB(s)-lumRGB(dst)) > tol {
		t.Errorf("Saturation: lum=%.4f, want %.4f", lumRGB(s), lumRGB(dst))
	}
	if math.Abs(satRGB(s)-satRGB(src)) > tol {
		t.Errorf("Saturation: sat=%.4f, want %.4f", satRGB(s), satRGB(src))
	}

	// Color: output Lum ≈ Lum(dst)
	col := runBlend(BlendColor, src, dst, ModeRGB8)
	if math.Abs(lumRGB(col)-lumRGB(dst)) > tol {
		t.Errorf("Color: lum=%.4f, want %.4f", lumRGB(col), lumRGB(dst))
	}

	// Luminosity: output Lum ≈ Lum(src)
	l := runBlend(BlendLuminosity, src, dst, ModeRGB8)
	if math.Abs(lumRGB(l)-lumRGB(src)) > tol {
		t.Errorf("Luminosity: lum=%.4f, want %.4f", lumRGB(l), lumRGB(src))
	}
}

// -------------------- ColorDodge / ColorBurn boundary --------------------

func TestBlendColorDodgeBoundary(t *testing.T) {
	// Cs=255 → 255 always.
	out := runBlend(BlendColorDodge, rgb(255, 255, 255), rgb(50, 100, 200), ModeRGB8)
	if out[0] != 255 || out[1] != 255 || out[2] != 255 {
		t.Errorf("ColorDodge(255, *) = %v, want all 255", out[:3])
	}
	// Cs=0 → Cb (since 0*255/255 = 0, capped, returns 0... actually min(255, Cb*255/255) = Cb).
	out = runBlend(BlendColorDodge, rgb(0, 0, 0), rgb(50, 100, 200), ModeRGB8)
	if out[0] != 50 || out[1] != 100 || out[2] != 200 {
		t.Errorf("ColorDodge(0, dst) = %v, want %v", out[:3], []byte{50, 100, 200})
	}
}

func TestBlendColorBurnBoundary(t *testing.T) {
	// Cs=0 → 0 always.
	out := runBlend(BlendColorBurn, rgb(0, 0, 0), rgb(50, 100, 200), ModeRGB8)
	if out[0] != 0 || out[1] != 0 || out[2] != 0 {
		t.Errorf("ColorBurn(0, *) = %v, want all 0", out[:3])
	}
	// Cs=255 → Cb.
	out = runBlend(BlendColorBurn, rgb(255, 255, 255), rgb(50, 100, 200), ModeRGB8)
	if out[0] != 50 || out[1] != 100 || out[2] != 200 {
		t.Errorf("ColorBurn(255, dst) = %v, want %v", out[:3], []byte{50, 100, 200})
	}
}

// -------------------- Darken/Lighten/Multiply identities --------------------

func TestBlendMultiplyByWhiteIsIdentity(t *testing.T) {
	for v := 0; v < 256; v += 17 {
		c := rgb(byte(v), byte(v), byte(v))
		out := runBlend(BlendMultiply, rgb(255, 255, 255), c, ModeRGB8)
		// Div255(255*v) = v (within 1 LSB)
		if out[0] != byte(v) {
			t.Errorf("Multiply(255, %d) = %d, want %d", v, out[0], v)
		}
	}
}

func TestBlendScreenByBlackIsIdentity(t *testing.T) {
	for v := 0; v < 256; v += 17 {
		c := rgb(byte(v), byte(v), byte(v))
		// Screen(0, v) = v + 0 - 0 = v
		out := runBlend(BlendScreen, rgb(0, 0, 0), c, ModeRGB8)
		if out[0] != byte(v) {
			t.Errorf("Screen(0, %d) = %d, want %d", v, out[0], v)
		}
	}
}

// -------------------- CMYK mode coverage for separable modes --------------------

func TestBlendMultiplyCMYK(t *testing.T) {
	src := cmyk(128, 64, 200, 32)
	dst := cmyk(64, 128, 50, 16)
	got := runBlend(BlendMultiply, src, dst, ModeCMYK8)
	want := [4]byte{
		byte(255 - ((255-128)*(255-64))/255),
		byte(255 - ((255-64)*(255-128))/255),
		byte(255 - ((255-200)*(255-50))/255),
		byte(255 - ((255-32)*(255-16))/255),
	}
	for i := 0; i < 4; i++ {
		if got[i] != want[i] {
			t.Errorf("Multiply CMYK ch%d: got %d, want %d", i, got[i], want[i])
		}
	}
}

// -------------------- Pipe integration smoke --------------------
//
// Install BlendMultiply, fill a black source over a white destination — expect
// black (Multiply: 0*255/255 = 0). Without blendFunc the AA path with aSrc=255
// would hit the noTransparency fast path; setting blendFunc forces the AA
// branch to consult the blend hook.

func TestPipeBlendMultiplyBlackOverWhite(t *testing.T) {
	bm := NewBitmap(4, 1, ModeRGB8, true)
	for i := range bm.data {
		bm.data[i] = 255 // white background
	}
	for i := range bm.alpha {
		bm.alpha[i] = 255
	}
	s, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SetBlendFunc(BlendMultiply)

	var p pipe
	black := Color{0, 0, 0, 0, 0, 0, 0, 0}
	// usesShape=true forces the AA path which honors blendFunc.
	s.pipeInit(&p, 0, 0, nil, &black, 255, true, false)
	p.shape = 255
	for i := 0; i < 4; i++ {
		p.run(&p)
	}
	for i := 0; i < 4; i++ {
		off := i * 3
		if bm.data[off] != 0 || bm.data[off+1] != 0 || bm.data[off+2] != 0 {
			t.Errorf("pixel %d: got %v, want black", i, bm.data[off:off+3])
		}
	}
}

func TestPipeBlendMultiplyHalfOverHalf(t *testing.T) {
	bm := NewBitmap(1, 1, ModeRGB8, true)
	bm.data[0], bm.data[1], bm.data[2] = 128, 128, 128
	bm.alpha[0] = 255
	s, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SetBlendFunc(BlendMultiply)
	var p pipe
	half := Color{128, 128, 128, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &half, 255, true, false)
	p.shape = 255
	p.run(&p)
	// Multiply(128,128) = 128*128/255 = 64.
	want := byte((128 * 128) / 255)
	if bm.data[0] != want || bm.data[1] != want || bm.data[2] != want {
		t.Errorf("pipe Multiply(128,128): got %v, want all %d", bm.data[:3], want)
	}
}

func TestPipeBlendUsesPopplerAlphaBlendPrecedence(t *testing.T) {
	bm := NewBitmap(1, 1, ModeRGB8, true)
	bm.data[0], bm.data[1], bm.data[2] = 10, 20, 30
	bm.alpha[0] = 100
	s, err := New(bm, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SetBlendFunc(BlendMultiply)

	var p pipe
	src := Color{70, 80, 90, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &src, 200, true, false)
	p.shape = 255
	p.run(&p)

	aSrc := 200
	aDst := 100
	aResult := aSrc + aDst - Div255(aSrc*aDst)
	blend := (70 * 10) / 255
	inner := (255-aDst)*70 + aDst*blend
	want := byte(((aResult-aSrc)*10 + aSrc*inner/255) / aResult)
	if bm.data[0] != want {
		t.Fatalf("alpha blend precedence red = %d, want %d", bm.data[0], want)
	}
}

// -------------------- Helper sanity --------------------

func TestLumIsWeighted(t *testing.T) {
	// Pure red channel weight = 0.3.
	if math.Abs(lum(1, 0, 0)-0.3) > 1e-9 {
		t.Errorf("lum(1,0,0) = %v, want 0.3", lum(1, 0, 0))
	}
	if math.Abs(lum(0, 1, 0)-0.59) > 1e-9 {
		t.Errorf("lum(0,1,0) = %v, want 0.59", lum(0, 1, 0))
	}
	if math.Abs(lum(0, 0, 1)-0.11) > 1e-9 {
		t.Errorf("lum(0,0,1) = %v, want 0.11", lum(0, 0, 1))
	}
}

func TestSetLumPreservesLum(t *testing.T) {
	r, g, b := setLum(0.2, 0.5, 0.8, 0.42)
	got := lum(r, g, b)
	if math.Abs(got-0.42) > 1e-9 {
		t.Errorf("setLum result lum=%v, want 0.42", got)
	}
}

func TestSetSatPreservesSat(t *testing.T) {
	r, g, b := setSat(0.2, 0.5, 0.8, 0.4)
	got := sat(r, g, b)
	if math.Abs(got-0.4) > 1e-9 {
		t.Errorf("setSat result sat=%v, want 0.4", got)
	}
}
