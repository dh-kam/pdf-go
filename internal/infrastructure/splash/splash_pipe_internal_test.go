package splash

import "testing"

// makeBitmapForTest constructs a Bitmap with allocated data + alpha planes.
func makeBitmapForTest(w, h int, mode ColorMode) *Bitmap {
	bpp := bytesPerPixel(mode)
	b := &Bitmap{
		width:   w,
		height:  h,
		mode:    mode,
		rowSize: w * bpp,
	}
	b.data = make([]byte, w*h*bpp)
	b.alpha = make([]byte, w*h)
	return b
}

func TestPipeInitDispatchPerMode(t *testing.T) {
	cases := []struct {
		mode ColorMode
		want string
	}{
		{ModeMono8, "Mono8"},
		{ModeRGB8, "RGB8"},
		{ModeCMYK8, "CMYK8"},
		{ModeDeviceN8, "DeviceN8"},
	}
	for _, c := range cases {
		s, err := New(makeBitmapForTest(4, 4, c.mode), false)
		if err != nil {
			t.Fatalf("%s: New: %v", c.want, err)
		}
		var p pipe
		col := Color{255, 255, 255, 255, 0, 0, 0, 0}
		s.pipeInit(&p, 0, 0, nil, &col, 255, false, false)
		if p.run == nil {
			t.Errorf("%s: pipeInit did not assign run", c.want)
		}
		if !p.noTransparency {
			t.Errorf("%s: noTransparency expected true (aInput=255, !usesShape)", c.want)
		}
	}
}

func TestPipeRunSimpleRGB8(t *testing.T) {
	s, _ := New(makeBitmapForTest(10, 10, ModeRGB8), false)
	var p pipe
	col := Color{200, 100, 50, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, false, false)
	for y := 0; y < 10; y++ {
		s.pipeSetXY(&p, 0, y)
		pipeRun(&p, 10)
	}
	for i := 0; i < 100; i++ {
		if s.bitmap.data[i*3] != 200 || s.bitmap.data[i*3+1] != 100 || s.bitmap.data[i*3+2] != 50 {
			t.Fatalf("rgb8 simple: pixel %d not opaque src color", i)
		}
		if s.bitmap.alpha[i] != 255 {
			t.Fatalf("rgb8 simple: alpha[%d] = %d, want 255", i, s.bitmap.alpha[i])
		}
	}
}

func TestPipeRunSimpleMono8(t *testing.T) {
	s, _ := New(makeBitmapForTest(10, 10, ModeMono8), false)
	var p pipe
	col := Color{77, 0, 0, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, false, false)
	for y := 0; y < 10; y++ {
		s.pipeSetXY(&p, 0, y)
		pipeRun(&p, 10)
	}
	for i := 0; i < 100; i++ {
		if s.bitmap.data[i] != 77 {
			t.Fatalf("mono8: data[%d] = %d, want 77", i, s.bitmap.data[i])
		}
	}
}

func TestPipeRunSimpleCMYK8(t *testing.T) {
	s, _ := New(makeBitmapForTest(10, 10, ModeCMYK8), false)
	var p pipe
	col := Color{10, 20, 30, 40, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, false, false)
	for y := 0; y < 10; y++ {
		s.pipeSetXY(&p, 0, y)
		pipeRun(&p, 10)
	}
	for i := 0; i < 100; i++ {
		if s.bitmap.data[i*4] != 10 || s.bitmap.data[i*4+1] != 20 || s.bitmap.data[i*4+2] != 30 || s.bitmap.data[i*4+3] != 40 {
			t.Fatalf("cmyk8: pixel %d wrong", i)
		}
	}
}

func TestPipeRunSimpleDeviceN8(t *testing.T) {
	s, _ := New(makeBitmapForTest(10, 10, ModeDeviceN8), false)
	var p pipe
	col := Color{1, 2, 3, 4, 5, 6, 7, 8}
	s.pipeInit(&p, 0, 0, nil, &col, 255, false, false)
	for y := 0; y < 10; y++ {
		s.pipeSetXY(&p, 0, y)
		pipeRun(&p, 10)
	}
	for i := 0; i < 100; i++ {
		for k := 0; k < splashMaxColorComps; k++ {
			if s.bitmap.data[i*splashMaxColorComps+k] != byte(k+1) {
				t.Fatalf("devN: pixel %d comp %d", i, k)
			}
		}
	}
}

// AA path: dest=0, shape=128 → aSrc = div255(255*128) = 128.
// alpha2 = 128, c0 = ((128-128)*0 + 128*src) / 128 = src.
func TestPipeRunAARGB8MidShape(t *testing.T) {
	s, _ := New(makeBitmapForTest(4, 4, ModeRGB8), false)
	var p pipe
	col := Color{200, 100, 50, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 128
	p.run(&p)
	if s.bitmap.data[0] != 200 || s.bitmap.data[1] != 100 || s.bitmap.data[2] != 50 {
		t.Fatalf("aa rgb8 onto zero dst: got [%d %d %d]", s.bitmap.data[0], s.bitmap.data[1], s.bitmap.data[2])
	}
	if s.bitmap.alpha[0] != 128 {
		t.Fatalf("aa rgb8 alpha = %d, want 128", s.bitmap.alpha[0])
	}
}

func TestPipeRunAAMono8MidShape(t *testing.T) {
	s, _ := New(makeBitmapForTest(4, 4, ModeMono8), false)
	var p pipe
	col := Color{180, 0, 0, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 128
	p.run(&p)
	if s.bitmap.data[0] != 180 {
		t.Fatalf("aa mono8 onto zero dst: %d, want 180", s.bitmap.data[0])
	}
	if s.bitmap.alpha[0] != 128 {
		t.Fatalf("aa mono8 alpha = %d, want 128", s.bitmap.alpha[0])
	}
}

func TestPipeRunAACMYK8MidShape(t *testing.T) {
	s, _ := New(makeBitmapForTest(4, 4, ModeCMYK8), false)
	var p pipe
	col := Color{10, 20, 30, 40, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 128
	p.run(&p)
	if s.bitmap.data[0] != 10 || s.bitmap.data[1] != 20 || s.bitmap.data[2] != 30 || s.bitmap.data[3] != 40 {
		t.Fatalf("aa cmyk: %v", s.bitmap.data[:4])
	}
	if s.bitmap.alpha[0] != 128 {
		t.Fatalf("aa cmyk alpha = %d", s.bitmap.alpha[0])
	}
}

func TestPipeRunAADeviceN8MidShape(t *testing.T) {
	s, _ := New(makeBitmapForTest(4, 4, ModeDeviceN8), false)
	var p pipe
	col := Color{1, 2, 3, 4, 5, 6, 7, 8}
	s.pipeInit(&p, 0, 0, nil, &col, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 128
	p.run(&p)
	for k := 0; k < splashMaxColorComps; k++ {
		if s.bitmap.data[k] != byte(k+1) {
			t.Fatalf("aa devN comp %d = %d, want %d", k, s.bitmap.data[k], k+1)
		}
	}
	if s.bitmap.alpha[0] != 128 {
		t.Fatalf("aa devN alpha = %d", s.bitmap.alpha[0])
	}
}

// Verify AA blend with non-zero dest is a midpoint between src and dst when shape=128.
// aSrc=128, aDest=255, aResult = 128+255-div255(128*255) = 128+255-128 = 255.
// alpha2=255, c = ((255-128)*dst + 128*src)/255 = (127*dst + 128*src)/255 ~ midpoint.
func TestPipeRunAARGB8BlendsMid(t *testing.T) {
	b := makeBitmapForTest(2, 1, ModeRGB8)
	b.data[0], b.data[1], b.data[2] = 0, 0, 0
	b.alpha[0] = 255
	s, _ := New(b, false)
	var p pipe
	col := Color{255, 255, 255, 0, 0, 0, 0, 0}
	s.pipeInit(&p, 0, 0, nil, &col, 255, true, false)
	s.pipeSetXY(&p, 0, 0)
	p.shape = 128
	p.run(&p)
	got := int(b.data[0])
	if got < 120 || got > 135 {
		t.Fatalf("aa midblend: got %d, expected ~128", got)
	}
}
