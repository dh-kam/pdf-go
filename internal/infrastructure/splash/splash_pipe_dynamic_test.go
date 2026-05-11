package splash

import "testing"

// rampPattern is a synthetic dynamic Pattern used by tests in this file. Its
// GetColor returns a color whose first byte equals min(255, x). This isolates
// the pipe-level per-pixel pattern fetch (Splash.cc:312-316) from the more
// elaborate AxialShader/TilingPattern math which is covered separately.
type rampPattern struct {
	mode ColorMode
	// fixedY: when non-negative, GetColor must be invoked at this y (verifies
	// the pipe forwards the correct device row to the pattern).
	fixedY int
	// hits records every (x, y) the pipe asked the pattern about; useful for
	// asserting per-pixel invocation.
	hits [][2]int
}

func (r *rampPattern) GetColor(x, y int, c *Color) bool {
	r.hits = append(r.hits, [2]int{x, y})
	v := byte(0)
	if x >= 0 {
		if x > 255 {
			v = 255
		} else {
			v = byte(x)
		}
	}
	switch r.mode {
	case ModeMono8:
		*c = Color{v, 0, 0, 0, 0, 0, 0, 0}
	case ModeRGB8:
		*c = Color{v, byte(255 - int(v)), v / 2, 0, 0, 0, 0, 0}
	case ModeCMYK8:
		*c = Color{v, byte(255 - int(v)), v / 2, v / 4, 0, 0, 0, 0}
	case ModeDeviceN8:
		*c = Color{v, byte(1 + int(v)/2), byte(2 + int(v)/3), byte(3 + int(v)/4),
			byte(4 + int(v)/5), byte(5 + int(v)/6), byte(6 + int(v)/7), byte(7 + int(v)/8)}
	default:
		*c = Color{v, v, v, v, 0, 0, 0, 0}
	}
	return true
}

func (r *rampPattern) TestPosition(x, y int) bool { return true }
func (r *rampPattern) IsStatic() bool             { return false }
func (r *rampPattern) IsCMYK() bool {
	return r.mode == ModeCMYK8 || r.mode == ModeDeviceN8
}

// holePattern reports false at every pixel — exercises the Splash.cc:313-315
// "pattern hole" branch where the pipe must still advance its cursor.
type holePattern struct{ hits int }

func (h *holePattern) GetColor(x, y int, c *Color) bool { h.hits++; return false }
func (h *holePattern) TestPosition(x, y int) bool       { return true }
func (h *holePattern) IsStatic() bool                   { return false }
func (h *holePattern) IsCMYK() bool                     { return false }

// TestPipeDynamicPatternRGB8Simple verifies that the simple-mode RGB8 pipe
// fetches src per-pixel from a non-static pattern instead of caching cSrcVal
// once at pipeInit time (the Phase-1 bug).
func TestPipeDynamicPatternRGB8Simple(t *testing.T) {
	b := makeBitmapForTest(8, 1, ModeRGB8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeRGB8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
	if p.pattern == nil {
		t.Fatalf("dynamic pattern should leave p.pattern non-nil")
	}
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 8)
	for x := 0; x < 8; x++ {
		got := b.data[x*3]
		if got != byte(x) {
			t.Fatalf("RGB8 simple: pixel %d R=%d, want %d (per-pixel pattern not invoked)", x, got, x)
		}
		// G channel is 255-x, B is x/2 — confirms the entire src color refreshes per pixel.
		if b.data[x*3+1] != byte(255-x) {
			t.Fatalf("RGB8 simple: pixel %d G=%d, want %d", x, b.data[x*3+1], 255-x)
		}
		if b.data[x*3+2] != byte(x/2) {
			t.Fatalf("RGB8 simple: pixel %d B=%d, want %d", x, b.data[x*3+2], x/2)
		}
	}
	if len(pat.hits) != 8 {
		t.Fatalf("RGB8 simple: pattern.GetColor invoked %d times, want 8", len(pat.hits))
	}
}

// TestPipeDynamicPatternRGB8AA verifies the AA-mode RGB8 pipe per-pixel fetch.
func TestPipeDynamicPatternRGB8AA(t *testing.T) {
	b := makeBitmapForTest(8, 1, ModeRGB8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeRGB8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 255 // full coverage → result equals src.
	for i := 0; i < 8; i++ {
		p.run(&p)
	}
	for x := 0; x < 8; x++ {
		if b.data[x*3] != byte(x) {
			t.Fatalf("RGB8 AA: pixel %d R=%d, want %d", x, b.data[x*3], x)
		}
	}
}

// TestPipeDynamicPatternMono8Simple covers the Mono8 simple variant.
func TestPipeDynamicPatternMono8Simple(t *testing.T) {
	b := makeBitmapForTest(16, 1, ModeMono8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeMono8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 16)
	for x := 0; x < 16; x++ {
		if b.data[x] != byte(x) {
			t.Fatalf("Mono8 simple: pixel %d = %d, want %d", x, b.data[x], x)
		}
	}
}

// TestPipeDynamicPatternMono8AA covers the Mono8 AA variant.
func TestPipeDynamicPatternMono8AA(t *testing.T) {
	b := makeBitmapForTest(16, 1, ModeMono8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeMono8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 255
	for i := 0; i < 16; i++ {
		p.run(&p)
	}
	for x := 0; x < 16; x++ {
		if b.data[x] != byte(x) {
			t.Fatalf("Mono8 AA: pixel %d = %d, want %d", x, b.data[x], x)
		}
	}
}

// TestPipeDynamicPatternCMYK8Simple covers the CMYK8 simple variant.
func TestPipeDynamicPatternCMYK8Simple(t *testing.T) {
	b := makeBitmapForTest(8, 1, ModeCMYK8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeCMYK8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 8)
	for x := 0; x < 8; x++ {
		got := b.data[x*4 : x*4+4]
		want := [4]byte{byte(x), byte(255 - x), byte(x / 2), byte(x / 4)}
		for k := 0; k < 4; k++ {
			if got[k] != want[k] {
				t.Fatalf("CMYK8 simple: pixel %d comp %d = %d, want %d", x, k, got[k], want[k])
			}
		}
	}
}

// TestPipeDynamicPatternCMYK8AA covers the CMYK8 AA variant.
func TestPipeDynamicPatternCMYK8AA(t *testing.T) {
	b := makeBitmapForTest(8, 1, ModeCMYK8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeCMYK8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 255
	for i := 0; i < 8; i++ {
		p.run(&p)
	}
	for x := 0; x < 8; x++ {
		if b.data[x*4] != byte(x) {
			t.Fatalf("CMYK8 AA: pixel %d C=%d, want %d", x, b.data[x*4], x)
		}
	}
}

// TestPipeDynamicPatternDeviceN8Simple covers the DeviceN8 simple variant.
func TestPipeDynamicPatternDeviceN8Simple(t *testing.T) {
	b := makeBitmapForTest(8, 1, ModeDeviceN8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeDeviceN8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 8)
	for x := 0; x < 8; x++ {
		off := x * splashMaxColorComps
		if b.data[off] != byte(x) {
			t.Fatalf("DeviceN8 simple: pixel %d comp 0 = %d, want %d", x, b.data[off], x)
		}
	}
}

// TestPipeDynamicPatternDeviceN8AA covers the DeviceN8 AA variant.
func TestPipeDynamicPatternDeviceN8AA(t *testing.T) {
	b := makeBitmapForTest(8, 1, ModeDeviceN8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeDeviceN8}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 255
	for i := 0; i < 8; i++ {
		p.run(&p)
	}
	for x := 0; x < 8; x++ {
		off := x * splashMaxColorComps
		if b.data[off] != byte(x) {
			t.Fatalf("DeviceN8 AA: pixel %d comp 0 = %d, want %d", x, b.data[off], x)
		}
	}
}

// TestPipeStaticPatternRegression — Phase 1 SolidColor patterns must remain
// behavior-equivalent to the noPat fast path: the constant cSrcVal is cached
// at pipeInit and pattern.GetColor is never called during pipeRun.
func TestPipeStaticPatternRegression(t *testing.T) {
	b := makeBitmapForTest(4, 1, ModeRGB8)
	s, _ := New(b, false)
	col := Color{17, 42, 99, 0, 0, 0, 0, 0}
	pat := NewSolidColor(col)
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
	if p.pattern != nil {
		t.Fatalf("static pattern: pipeInit must NOT keep p.pattern set (caches cSrc instead)")
	}
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 4)
	for x := 0; x < 4; x++ {
		if b.data[x*3] != 17 || b.data[x*3+1] != 42 || b.data[x*3+2] != 99 {
			t.Fatalf("static regression: pixel %d = [%d %d %d]",
				x, b.data[x*3], b.data[x*3+1], b.data[x*3+2])
		}
	}
}

// TestPipeDynamicPatternHoleSkip — when GetColor returns false the pipe must
// advance the cursor without writing color or alpha, matching Splash.cc:313-315.
func TestPipeDynamicPatternHoleSkip(t *testing.T) {
	b := makeBitmapForTest(4, 1, ModeRGB8)
	// pre-fill bitmap with a sentinel so we can detect any errant write.
	for i := range b.data {
		b.data[i] = 0xAB
	}
	for i := range b.alpha {
		b.alpha[i] = 0xCD
	}
	s, _ := New(b, false)
	pat := &holePattern{}
	var p pipe
	s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 4)
	for i := 0; i < len(b.data); i++ {
		if b.data[i] != 0xAB {
			t.Fatalf("hole skip: data[%d] = %d, expected sentinel 0xAB (pipe wrote through a hole)", i, b.data[i])
		}
	}
	for i := 0; i < len(b.alpha); i++ {
		if b.alpha[i] != 0xCD {
			t.Fatalf("hole skip: alpha[%d] = %d, expected sentinel 0xCD", i, b.alpha[i])
		}
	}
	if pat.hits != 4 {
		t.Fatalf("hole skip: GetColor invoked %d times, want 4", pat.hits)
	}
	if p.x != 4 {
		t.Fatalf("hole skip: pipe x advanced to %d, want 4", p.x)
	}
}

// TestPipeDynamicPatternForwardsXY — verify each pipe call asks the pattern
// at the current device coordinate (not (0,0) cached at init time).
func TestPipeDynamicPatternForwardsXY(t *testing.T) {
	b := makeBitmapForTest(5, 3, ModeRGB8)
	s, _ := New(b, false)
	pat := &rampPattern{mode: ModeRGB8}
	var p pipe
	s.pipeInit(&p, 0, 2, pat, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 2)
	pipeRun(&p, 5)
	if len(pat.hits) != 5 {
		t.Fatalf("expected 5 GetColor calls, got %d", len(pat.hits))
	}
	for x := 0; x < 5; x++ {
		if pat.hits[x][0] != x || pat.hits[x][1] != 2 {
			t.Fatalf("hit %d = (%d, %d), want (%d, 2)", x, pat.hits[x][0], pat.hits[x][1], x)
		}
	}
}

// TestPipeDynamicAxialShaderEndToEnd integrates AxialShader through the pipe
// to confirm the gradient actually emerges per pixel — without the fix, every
// pixel would be the t=0 color.
//
// Axial axis lies along device row y=1 (so the (x, y+1) sample convention
// projects pixels on row y=0 onto fy=1). Bitmap row 0 is the rendered row.
func TestPipeDynamicAxialShaderEndToEnd(t *testing.T) {
	b := makeBitmapForTest(50, 2, ModeRGB8)
	s, _ := New(b, false)
	// Axis at fy=1 (sample row when device y=0 thanks to (x, y+1) convention).
	shader := NewAxialShader(0, 1, 49, 1, 0, 1, false, false, linearGray, ModeRGB8)
	var p pipe
	s.pipeInit(&p, 0, 0, shader, nil, 255, false, false)
	if p.pattern == nil {
		t.Fatalf("AxialShader is dynamic — pipe.pattern must be non-nil")
	}
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 50)
	pix0 := b.data[0]
	pix25 := b.data[25*3]
	pix49 := b.data[49*3]
	if pix0 > 5 {
		t.Fatalf("axial start: R=%d, want ~0", pix0)
	}
	if pix49 < 250 {
		t.Fatalf("axial end: R=%d, want ~255", pix49)
	}
	if pix25 < 100 || pix25 > 160 {
		t.Fatalf("axial mid: R=%d, want ~128", pix25)
	}
	// Without the fix every pixel would equal pix0 — assert variation explicitly.
	if pix0 == pix49 {
		t.Fatalf("axial: all pixels identical (%d) — dynamic pattern fanout broken", pix0)
	}
}

// TestPipeDynamicRadialShaderEndToEnd integrates RadialShader through the pipe
// — center vs edge colors must differ. (x, y+1) convention means a device row
// y=0 samples fy=1, so we center the gradient at (10, 1).
func TestPipeDynamicRadialShaderEndToEnd(t *testing.T) {
	b := makeBitmapForTest(21, 2, ModeRGB8)
	s, _ := New(b, false)
	shader := NewRadialShader(10, 1, 0, 10, 1, 10, 0, 1, false, false, linearGray, ModeRGB8)
	var p pipe
	s.pipeInit(&p, 0, 0, shader, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 21)
	center := b.data[10*3]
	edge := b.data[0]
	if center == edge {
		t.Fatalf("radial: center and edge identical (%d) — dynamic pattern fanout broken", center)
	}
}

// TestPipeDynamicTilingPatternEndToEnd integrates a 4-pixel-wide checker
// tiling: pixels 4 apart land in the same tile cell offset (same color);
// pixels 2 apart land in different cells (different colors).
func TestPipeDynamicTilingPatternEndToEnd(t *testing.T) {
	cell := makeBitmapForTest(4, 4, ModeRGB8)
	// Fill cell with a horizontal ramp so cell-x → R channel.
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			off := y*cell.rowSize + x*3
			cell.data[off] = byte(x * 60)
			cell.data[off+1] = 0
			cell.data[off+2] = 0
			cell.alpha[y*4+x] = 0xFF
		}
	}
	tiling := NewTilingPattern(cell, [4]float64{0, 0, 4, 4}, 4, 4,
		[6]float64{1, 0, 0, 1, 0, 0}, 1, Color{})

	b := makeBitmapForTest(16, 1, ModeRGB8)
	s, _ := New(b, false)
	var p pipe
	s.pipeInit(&p, 0, 0, tiling, nil, 255, false, false)
	s.pipeSetXY(&p, 0, 0)
	pipeRun(&p, 16)

	// Pixels 4 apart hit the same cell-x → same R.
	if b.data[0] != b.data[4*3] {
		t.Fatalf("tiling: x=0 (R=%d) and x=4 (R=%d) should match (period=4)",
			b.data[0], b.data[4*3])
	}
	// Pixels 2 apart hit different cell-x → different R.
	if b.data[0] == b.data[2*3] {
		t.Fatalf("tiling: x=0 and x=2 produced the same R (%d) — tiling collapsed to constant",
			b.data[0])
	}
}

// TestPipeDynamicDispatchAllModes spot-checks pipeInit/pickRun keeps p.pattern
// set across all four color modes when the pattern is non-static.
func TestPipeDynamicDispatchAllModes(t *testing.T) {
	cases := []struct {
		mode ColorMode
		name string
	}{
		{ModeMono8, "Mono8"},
		{ModeRGB8, "RGB8"},
		{ModeCMYK8, "CMYK8"},
		{ModeDeviceN8, "DeviceN8"},
	}
	for _, c := range cases {
		s, _ := New(makeBitmapForTest(4, 4, c.mode), false)
		pat := &rampPattern{mode: c.mode}
		var p pipe
		s.pipeInit(&p, 0, 0, pat, nil, 255, false, false)
		if p.pattern == nil {
			t.Fatalf("%s: dynamic pattern dropped at pipeInit", c.name)
		}
		if p.run == nil {
			t.Fatalf("%s: pipeInit did not assign run", c.name)
		}
	}
}
