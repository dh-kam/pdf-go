package splash

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/renderer"
	infraimage "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// newTestBackend returns a *splashCanvas (concrete type) for white-box tests.
func newTestBackend(t *testing.T, w, h int) *splashCanvas {
	t.Helper()
	c, ok := NewBackend(w, h).(*splashCanvas)
	if !ok {
		t.Fatalf("NewBackend did not return *splashCanvas")
	}
	c.SetPageYOriginPx(float64(h))
	return c
}

// readBackendPixel returns the RGB pixel at (x, y) from the backend's bitmap.
func readBackendPixel(t *testing.T, c *splashCanvas, x, y int) (byte, byte, byte) {
	t.Helper()
	bm := c.s.bitmap
	if bm.Mode() != ModeRGB8 {
		t.Fatalf("expected ModeRGB8, got %d", bm.Mode())
	}
	off := y*bm.RowSize() + x*3
	data := bm.Data()
	if off+3 > len(data) {
		t.Fatalf("readBackendPixel out of range (%d,%d)", x, y)
	}
	return data[off], data[off+1], data[off+2]
}

func assertBBoxEqual(t *testing.T, got, want [4]float64) {
	t.Helper()
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("bbox[%d] = %.12f, want %.12f; got=%v", i, got[i], want[i], got)
		}
	}
}

func TestSetFillColorSplitsPremultipliedAlpha(t *testing.T) {
	c := newTestBackend(t, 4, 4)
	c.SetFillColor(color.RGBA{R: 143, G: 143, B: 143, A: 191})
	c.Rectangle(0, 0, 4, 4)
	c.Fill()

	got := c.Image().(*image.RGBA).RGBAAt(1, 1)
	if got.R != 207 || got.G != 207 || got.B != 207 || got.A != 255 {
		t.Fatalf("premultiplied fill over white = (%d,%d,%d,%d), want (207,207,207,255)", got.R, got.G, got.B, got.A)
	}
}

func TestMatchingFillClipKeepsScannerYFloorPhase(t *testing.T) {
	const nearIntegerY = 2.0 - 1e-13
	path := nearIntegerTrianglePath(nearIntegerY)

	clipOnly := newTestBackend(t, 16, 16)
	clipOnly.path = path.Clone()
	clipOnly.Clip()
	if got := backendClipAACoverage(t, clipOnly, 2, 1); got != 0 {
		t.Fatalf("clip-only snapped coverage = %d, want 0", got)
	}

	fillThenClip := newTestBackend(t, 16, 16)
	fillThenClip.path = path.Clone()
	fillThenClip.Fill()
	fillThenClip.path = path.Clone()
	fillThenClip.Clip()
	if got := backendClipAACoverage(t, fillThenClip, 2, 1); got == 0 {
		t.Fatalf("matching fill/clip coverage = 0, want unsnapped coverage")
	}
}

func TestRotatedClosedFillClipKeepsScannerYFloorPhase(t *testing.T) {
	const nearIntegerY = 2.0 - 1e-13
	fillPath := nearIntegerTrianglePath(nearIntegerY)
	clipPath := rotatedNearIntegerTrianglePath(nearIntegerY)

	if !pathsMatchForFillClipScannerPhase(fillPath, clipPath) {
		t.Fatal("rotated closed fill/clip path did not match")
	}

	clipOnly := newTestBackend(t, 16, 16)
	clipOnly.path = clipPath.Clone()
	clipOnly.Clip()
	if got := backendClipAACoverage(t, clipOnly, 2, 1); got != 0 {
		t.Fatalf("rotated clip-only snapped coverage = %d, want 0", got)
	}

	fillThenClip := newTestBackend(t, 16, 16)
	fillThenClip.path = fillPath.Clone()
	fillThenClip.Fill()
	fillThenClip.path = clipPath.Clone()
	fillThenClip.Clip()
	if got := backendClipAACoverage(t, fillThenClip, 2, 1); got == 0 {
		t.Fatalf("rotated matching fill/clip coverage = 0, want unsnapped coverage")
	}
}

func nearIntegerTrianglePath(y float64) *xpath.Path {
	p := xpath.NewPath()
	_ = p.MoveTo(1, y)
	_ = p.LineTo(8, y)
	_ = p.LineTo(8, 8)
	_ = p.Close(false)
	return p
}

func rotatedNearIntegerTrianglePath(y float64) *xpath.Path {
	p := xpath.NewPath()
	_ = p.MoveTo(8, y)
	_ = p.LineTo(8, 8)
	_ = p.LineTo(1, y)
	_ = p.Close(false)
	return p
}

func backendClipAACoverage(t *testing.T, c *splashCanvas, x, y int) int {
	t.Helper()
	clip := c.s.ensureClip()
	rowSize := (c.width*splashAASize + 7) >> 3
	aaBuf := make([]byte, rowSize*splashAASize)
	for i := range aaBuf {
		aaBuf[i] = 0xff
	}
	clip.ClipAALine(y, aaBuf, 0, c.width-1)
	cell := x * splashAASize
	coverage := 0
	for yy := 0; yy < splashAASize; yy++ {
		rowOff := yy * rowSize
		byteIdx := rowOff + (cell >> 3)
		if cell&7 == 0 {
			coverage += bitCount4[(aaBuf[byteIdx]>>4)&0x0f]
		} else {
			coverage += bitCount4[aaBuf[byteIdx]&0x0f]
		}
	}
	return coverage
}

func TestApplyAnnotationMultiplyMaskUpdatesLiveBitmap(t *testing.T) {
	c := newTestBackend(t, 3, 2)
	mask := image.NewRGBA(image.Rect(0, 0, 3, 2))
	mask.SetRGBA(1, 0, color.RGBA{A: 255})
	mask.SetRGBA(2, 0, color.RGBA{A: 128})

	c.ApplyAnnotationMultiplyMask(mask, color.RGBA{R: 255, G: 255, A: 255})

	r, g, b := readBackendPixel(t, c, 1, 0)
	if r != 255 || g != 255 || b != 0 {
		t.Fatalf("full highlight mask pixel = (%d,%d,%d), want yellow", r, g, b)
	}
	r, g, b = readBackendPixel(t, c, 2, 0)
	if r != 255 || g != 255 || b == 255 {
		t.Fatalf("partial highlight mask pixel = (%d,%d,%d), want partially yellowed", r, g, b)
	}
	r, g, b = readBackendPixel(t, c, 0, 0)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("unmasked pixel = (%d,%d,%d), want white", r, g, b)
	}
}

func TestApplyAnnotationMultiplyMaskCanvasUsesAnnotationAlphaMask(t *testing.T) {
	c := newTestBackend(t, 3, 2)
	mask, ok := c.NewAnnotationMaskCanvas(c.Bounds(), 2).(*splashCanvas)
	if !ok {
		t.Fatal("NewAnnotationMaskCanvas did not return *splashCanvas")
	}
	alpha := mask.s.bitmap.Alpha()
	alpha[0] = 255
	alpha[1] = 127
	alpha[2] = 0
	maskData := mask.s.bitmap.Data()
	for i := 0; i < 9; i++ {
		maskData[i] = 255
	}

	c.ApplyAnnotationMultiplyMaskCanvas(mask, color.RGBA{R: 255, G: 255, A: 255})

	r, g, b := readBackendPixel(t, c, 0, 0)
	if r != 255 || g != 255 || b != 0 {
		t.Fatalf("opaque alpha mask pixel = (%d,%d,%d), want yellow", r, g, b)
	}
	r, g, b = readBackendPixel(t, c, 1, 0)
	if r != 255 || g != 255 || b != 0 {
		t.Fatalf("partial alpha mask source pixel = (%d,%d,%d), want yellow source color", r, g, b)
	}
	visible := c.Image().(*image.RGBA).RGBAAt(1, 0)
	if visible.R != 255 || visible.G != 255 || visible.B != 128 || visible.A != 255 {
		t.Fatalf("partial alpha mask visible pixel = (%d,%d,%d,%d), want (255,255,128,255)", visible.R, visible.G, visible.B, visible.A)
	}
	r, g, b = readBackendPixel(t, c, 2, 0)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("transparent alpha mask pixel = (%d,%d,%d), want white", r, g, b)
	}
}

func TestApplyAnnotationMultiplyMaskCanvasUpdatesTransparentPageAlpha(t *testing.T) {
	t.Setenv("PDF_DEBUG_SPLASH_TRANSPARENT_PAGE_ALPHA", "1")
	c := newTestBackend(t, 2, 1)
	mask, ok := c.NewAnnotationMaskCanvas(c.Bounds(), 1).(*splashCanvas)
	if !ok {
		t.Fatal("NewAnnotationMaskCanvas did not return *splashCanvas")
	}
	alpha := mask.s.bitmap.Alpha()
	alpha[0] = 255

	c.ApplyAnnotationMultiplyMaskCanvas(mask, color.RGBA{R: 255, G: 255, A: 255})

	got := c.Image().(*image.RGBA).RGBAAt(0, 0)
	if got.R != 255 || got.G != 255 || got.B != 0 || got.A != 255 {
		t.Fatalf("transparent-page highlight pixel = (%d,%d,%d,%d), want opaque yellow", got.R, got.G, got.B, got.A)
	}
}

func TestSetFillPatternGouraudAppliesPatternMatrix(t *testing.T) {
	c := newTestBackend(t, 12, 12)

	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	shading.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(3, 0, []float64{1, 0, 0}),
		entity.NewVertex(0, 3, []float64{1, 0, 0}),
	})
	pattern := entity.NewShadingPattern("mesh-translate", shading)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 4, 4})

	c.SetFillPattern(pattern)
	c.Rectangle(0, 0, 12, 12)
	c.Fill()

	r, g, b := readBackendPixel(t, c, 5, 6)
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("translated Gouraud pattern pixel = (%d,%d,%d), want red", r, g, b)
	}
	r, g, b = readBackendPixel(t, c, 1, 6)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("outside Gouraud pattern pixel = (%d,%d,%d), want white", r, g, b)
	}
}

func TestSetFillPatternGouraudClipsToCurrentFillPath(t *testing.T) {
	c := newTestBackend(t, 12, 12)

	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	shading.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(12, 0, []float64{1, 0, 0}),
		entity.NewVertex(0, 12, []float64{1, 0, 0}),
	})
	pattern := entity.NewShadingPattern("mesh-clip", shading)

	c.SetFillPattern(pattern)
	c.Rectangle(0, 0, 4, 12)
	c.Fill()

	r, g, b := readBackendPixel(t, c, 2, 6)
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("inside clipped Gouraud pattern pixel = (%d,%d,%d), want red", r, g, b)
	}
	r, g, b = readBackendPixel(t, c, 8, 2)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("Gouraud direct fill leaked outside current path: got (%d,%d,%d), want white", r, g, b)
	}
}

func TestClipBoundsToShadingCacheBBoxFlipsYAndAppliesInverseMatrix(t *testing.T) {
	got, ok := clipBoundsToShadingCacheBBox([4]float64{10, 20, 30, 60}, true, 100, [6]float64{0.5, 0, 0, 0.25, -2, 3})
	if !ok {
		t.Fatal("clipBoundsToShadingCacheBBox returned !ok")
	}
	want := [4]float64{3, 13, 13, 23}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("bbox[%d] = %.12f, want %.12f; got=%v", i, got[i], want[i], got)
		}
	}
}

func TestClipBoundsToShadingCacheBBoxRejectsInvalidBounds(t *testing.T) {
	if _, ok := clipBoundsToShadingCacheBBox([4]float64{10, 20, 30, 60}, false, 100, [6]float64{1, 0, 0, 1, 0, 0}); ok {
		t.Fatal("clipBoundsToShadingCacheBBox accepted boundsOK=false")
	}
	if _, ok := clipBoundsToShadingCacheBBox([4]float64{10, 20, 10, 60}, true, 100, [6]float64{1, 0, 0, 1, 0, 0}); ok {
		t.Fatal("clipBoundsToShadingCacheBBox accepted empty x bounds")
	}
	if _, ok := clipBoundsToShadingCacheBBox([4]float64{10, 20, 30, 20}, true, 100, [6]float64{1, 0, 0, 1, 0, 0}); ok {
		t.Fatal("clipBoundsToShadingCacheBBox accepted empty y bounds")
	}
}

func TestCurrentClipBBoxForShadingCacheUsesSavedSoftMaskClip(t *testing.T) {
	c := newTestBackend(t, 10, 10)
	c.s.state.clip = xpath.NewClip(1, 2, 8, 7, c.s.vectorAA)

	if err := c.s.BeginTransparencyGroup([4]float64{0, 0, 10, 10}, true, false, nil); err != nil {
		t.Fatalf("BeginTransparencyGroup failed: %v", err)
	}

	got, ok := c.currentClipBBoxForShadingCache([6]float64{1, 0, 0, 1, 0, 0})
	if !ok {
		t.Fatal("currentClipBBoxForShadingCache returned !ok before soft-mask marking")
	}
	assertBBoxEqual(t, got, [4]float64{0, 0, 10, 10})

	c.s.markTopGroupForSoftMask()
	got, ok = c.currentClipBBoxForShadingCache([6]float64{1, 0, 0, 1, 0, 0})
	if !ok {
		t.Fatal("currentClipBBoxForShadingCache returned !ok for soft-mask group")
	}
	assertBBoxEqual(t, got, [4]float64{1, 2, 9, 8})
}

func TestCurrentClipBBoxForShadingCacheIntersectsActiveSoftMaskClipInCroppedGroup(t *testing.T) {
	c := newTestBackend(t, 200, 200)
	c.s.state.clip = xpath.NewClip(10, 20, 99, 119, c.s.vectorAA)
	c.s.state.softMask = NewBitmap(200, 200, ModeMono8, false)
	c.s.state.softMaskClip = clipBoundsSnapshot{
		vector:      [4]float64{50, 70, 170, 150},
		vectorOK:    true,
		effective:   [4]float64{50, 70, 170, 150},
		effectiveOK: true,
		offsetX:     0,
		offsetY:     0,
	}
	c.s.state.softMaskClipOK = true
	c.s.state.blendFunc = BlendMultiply
	c.s.groupStack = []*groupState{{cropped: true, tx: 40, ty: 50}}

	got, ok := c.currentClipBBoxForShadingCache([6]float64{1, 0, 0, 1, 0, 0})
	if !ok {
		t.Fatal("currentClipBBoxForShadingCache returned !ok for active soft-mask clip")
	}
	assertBBoxEqual(t, got, [4]float64{10, 100, 100, 180})
}

func TestCurrentClipBBoxForShadingCacheIgnoresActiveSoftMaskClipOutsideCroppedGroup(t *testing.T) {
	c := newTestBackend(t, 200, 200)
	c.s.state.clip = xpath.NewClip(10, 20, 99, 119, c.s.vectorAA)
	c.s.state.softMask = NewBitmap(200, 200, ModeMono8, false)
	c.s.state.softMaskClip = clipBoundsSnapshot{
		vector:      [4]float64{50, 70, 170, 150},
		vectorOK:    true,
		effective:   [4]float64{50, 70, 170, 150},
		effectiveOK: true,
	}
	c.s.state.softMaskClipOK = true
	c.s.state.blendFunc = BlendMultiply
	c.s.groupStack = []*groupState{{}}

	got, ok := c.currentClipBBoxForShadingCache([6]float64{1, 0, 0, 1, 0, 0})
	if !ok {
		t.Fatal("currentClipBBoxForShadingCache returned !ok without cropped group")
	}
	assertBBoxEqual(t, got, [4]float64{10, 80, 100, 180})
}

func TestCurrentClipBBoxForShadingCacheIntersectsEnclosingSoftMaskSavedClip(t *testing.T) {
	c := newTestBackend(t, 200, 200)
	c.s.state.clip = xpath.NewClip(10, 20, 99, 119, c.s.vectorAA)
	c.s.state.softMask = NewBitmap(200, 200, ModeMono8, false)
	c.s.state.blendFunc = BlendMultiply
	c.s.groupStack = []*groupState{
		{forSoftMask: true, savedClip: xpath.NewClip(50, 70, 169, 149, c.s.vectorAA)},
		{cropped: true, tx: 40, ty: 50},
	}

	got, ok := c.currentClipBBoxForShadingCache([6]float64{1, 0, 0, 1, 0, 0})
	if !ok {
		t.Fatal("currentClipBBoxForShadingCache returned !ok for enclosing soft-mask clip")
	}
	assertBBoxEqual(t, got, [4]float64{10, 100, 100, 180})
}

func TestSoftMaskClipStackSurvivesNestedClearSoftMask(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	c.s.state.clip = xpath.NewClip(2, 3, 9, 11, c.s.vectorAA)
	if err := c.BeginSoftMaskGroupDeviceBBox([4]float64{0, 0, 20, 20}, false, false); err != nil {
		t.Fatalf("BeginSoftMaskGroupDeviceBBox failed: %v", err)
	}
	if len(c.softMaskClipStack) != 1 {
		t.Fatalf("softMaskClipStack length = %d, want 1", len(c.softMaskClipStack))
	}

	c.ClearSoftMask()
	if len(c.softMaskClipStack) != 1 {
		t.Fatalf("ClearSoftMask cleared softMaskClipStack; length = %d, want 1", len(c.softMaskClipStack))
	}
	clip, ok := c.popSoftMaskClip()
	if !ok {
		t.Fatal("popSoftMaskClip returned !ok after nested clear")
	}
	if !clip.vectorOK || clip.vector != ([4]float64{2, 3, 10, 12}) {
		t.Fatalf("soft-mask clip snapshot = ok=%t %v, want [2 3 10 12]", clip.vectorOK, clip.vector)
	}
}

func TestSoftMaskAxialCacheEffectiveBoundsCandidate(t *testing.T) {
	c := newTestBackend(t, 4, 4)
	matrix := [6]float64{50, 330, -330, 50, 0, 0}
	vectorBBox := [4]float64{0.45, -0.50, 1.00, 0.45}
	effectiveBBox := [4]float64{0.455, -0.505, 1.005, 0.445}

	if c.softMaskAxialCacheEffectiveBoundsCandidate(matrix, vectorBBox, effectiveBBox, true, true) {
		t.Fatal("candidate should require soft mask, blend function, and nested group stack")
	}

	c.s.state.softMask = NewBitmap(1, 1, ModeMono8, false)
	c.s.state.blendFunc = BlendMultiply
	c.s.groupStack = []*groupState{{}, {}}

	if !c.softMaskAxialCacheEffectiveBoundsCandidate(matrix, vectorBBox, effectiveBBox, true, true) {
		t.Fatal("candidate should accept the scoped soft-mask axial shading shape")
	}
	if c.softMaskAxialCacheEffectiveBoundsCandidate([6]float64{20, 330, -330, 50, 0, 0}, vectorBBox, effectiveBBox, true, true) {
		t.Fatal("candidate accepted a matrix outside the tourism axial shape")
	}
	if c.softMaskAxialCacheEffectiveBoundsCandidate(matrix, [4]float64{0.20, -0.50, 1.00, 0.45}, effectiveBBox, true, true) {
		t.Fatal("candidate accepted a vector bbox outside the scoped range")
	}
	if c.softMaskAxialCacheEffectiveBoundsCandidate(matrix, vectorBBox, [4]float64{0.47, -0.505, 1.005, 0.445}, true, true) {
		t.Fatal("candidate accepted an effective bbox delta above tolerance")
	}
	if c.softMaskAxialCacheEffectiveBoundsCandidate(matrix, vectorBBox, effectiveBBox, false, true) {
		t.Fatal("candidate accepted missing vector bounds")
	}
	if c.softMaskAxialCacheEffectiveBoundsCandidate(matrix, vectorBBox, effectiveBBox, true, false) {
		t.Fatal("candidate accepted missing effective bounds")
	}
}

func TestCroppedTopEdgeAxialShadingUsesIntegerYOrigin(t *testing.T) {
	c := newTestBackend(t, 12, 3544)
	matrix := [6]float64{0, 7.540418125, -7.540418125, 0, 5.878137083, 3535.159214167}
	bbox := [4]float64{0, 3535, 11, 3543}

	if c.croppedTopEdgeAxialShadingUsesIntegerYOrigin(matrix, bbox, entity.ShadingAxial) {
		t.Fatal("candidate accepted a top-edge axial shading outside cropped groups")
	}

	c.s.groupStack = append(c.s.groupStack, &groupState{})
	if c.croppedTopEdgeAxialShadingUsesIntegerYOrigin(matrix, bbox, entity.ShadingAxial) {
		t.Fatal("candidate accepted a non-cropped transparency group")
	}

	c.s.groupStack[len(c.s.groupStack)-1].cropped = true
	if !c.croppedTopEdgeAxialShadingUsesIntegerYOrigin(matrix, bbox, entity.ShadingAxial) {
		t.Fatal("candidate rejected the scoped tourism cropped top-edge axial shading")
	}
	if got := c.shadingFillPathYOrigin(matrix, bbox, entity.ShadingAxial); got != float64(c.height) {
		t.Fatalf("shadingFillPathYOrigin = %.9f, want %.9f", got, float64(c.height))
	}
	if c.croppedTopEdgeAxialShadingUsesIntegerYOrigin(matrix, bbox, entity.ShadingRadial) {
		t.Fatal("candidate accepted a non-axial shading")
	}
	if c.croppedTopEdgeAxialShadingUsesIntegerYOrigin(
		[6]float64{0, 9.459626042, -9.459626042, 0, 6.898221250, 3533.300431458},
		[4]float64{0, 3533, 14, 3543},
		entity.ShadingAxial,
	) {
		t.Fatal("candidate accepted the wider Fm37 top-edge axial shading")
	}
	if c.croppedTopEdgeAxialShadingUsesIntegerYOrigin(
		[6]float64{6.088538750, 7.256037292, -7.256037292, 6.088538750, 0.287894792, 3533.779266250},
		[4]float64{0, 3529, 4, 3543},
		entity.ShadingAxial,
	) {
		t.Fatal("candidate accepted a rotated top-edge axial shading")
	}
}

func TestPopplerColorComponentToByteUsesFixedPointQuantization(t *testing.T) {
	if got := popplerColorComponentToByte(0.1); got != 25 {
		t.Fatalf("popplerColorComponentToByte(0.1) = %d, want 25", got)
	}
	if got := popplerColorComponentToByte(0.5); got != 128 {
		t.Fatalf("popplerColorComponentToByte(0.5) = %d, want 128", got)
	}
}

func TestStrokePathHasDeviceAlignedButtCapPlane(t *testing.T) {
	tests := []struct {
		name      string
		lineCap   int
		lineDash  []float64
		width     float64
		strokeAdj bool
		points    [][2]float64
		want      bool
	}{
		{
			name:      "google vertical half-pixel caps with unaligned side edges",
			lineCap:   int(LineCapButt),
			width:     2.08333339,
			strokeAdj: true,
			points:    [][2]float64{{294.7916667, 862.5}, {294.7916667, 1114.5833333}},
			want:      true,
		},
		{
			name:      "google vertical one side edge aligned",
			lineCap:   int(LineCapButt),
			width:     2.08333339,
			strokeAdj: true,
			points:    [][2]float64{{151.0416667, 862.5}, {151.0416667, 1114.5833333}},
			want:      true,
		},
		{
			name:      "geotopo p82 short tick stays on poppler matrix path",
			lineCap:   int(LineCapButt),
			width:     1.67798832,
			strokeAdj: true,
			points:    [][2]float64{{679.8175, 1286.5341}, {679.8175, 1294.9242}},
			want:      false,
		},
		{
			name:      "geotopo p20 colored vertical stroke stays on poppler matrix path",
			lineCap:   int(LineCapButt),
			width:     1.66043750,
			strokeAdj: true,
			points:    [][2]float64{{626.17910417, 210.73862500}, {626.17910417, 262.10708333}},
			want:      false,
		},
		{
			name:      "geotopo p86 vertical marker stays on poppler matrix path",
			lineCap:   int(LineCapButt),
			width:     1.80594164,
			strokeAdj: true,
			points:    [][2]float64{{681.10057939, 1157.66820205}, {681.10057939, 1166.69802353}},
			want:      false,
		},
		{
			name:      "horizontal half-pixel cap without side-edge alignment keeps default path",
			lineCap:   int(LineCapButt),
			width:     2.08333339,
			strokeAdj: true,
			points:    [][2]float64{{150, 863.5416667}, {1087.5, 863.5416667}},
			want:      false,
		},
		{
			name:      "dash keeps default poppler matrix path",
			lineCap:   int(LineCapButt),
			lineDash:  []float64{3, 2},
			width:     2.08333339,
			strokeAdj: true,
			points:    [][2]float64{{294.7916667, 862.5}, {294.7916667, 1114.5833333}},
			want:      false,
		},
		{
			name:      "round cap keeps default poppler matrix path",
			lineCap:   int(LineCapRound),
			width:     2.08333339,
			strokeAdj: true,
			points:    [][2]float64{{294.7916667, 862.5}, {294.7916667, 1114.5833333}},
			want:      false,
		},
		{
			name:      "stroke adjust disabled keeps default poppler matrix path",
			lineCap:   int(LineCapButt),
			width:     2.08333339,
			strokeAdj: false,
			points:    [][2]float64{{294.7916667, 862.5}, {294.7916667, 1114.5833333}},
			want:      false,
		},
		{
			name:      "three point path keeps default poppler matrix path",
			lineCap:   int(LineCapButt),
			width:     2.08333339,
			strokeAdj: true,
			points:    [][2]float64{{294.7916667, 862.5}, {294.7916667, 1000}, {294.7916667, 1114.5833333}},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestBackend(t, 1200, 1600)
			c.s.state.lineCap = tt.lineCap
			c.s.state.lineWidth = tt.width
			c.s.state.lineDash = tt.lineDash
			c.s.state.strokeAdjust = tt.strokeAdj
			c.path = xpath.NewPath()
			if len(tt.points) > 0 {
				if err := c.path.MoveTo(tt.points[0][0], tt.points[0][1]); err != nil {
					t.Fatalf("MoveTo: %v", err)
				}
			}
			for _, pt := range tt.points[1:] {
				if err := c.path.LineTo(pt[0], pt[1]); err != nil {
					t.Fatalf("LineTo: %v", err)
				}
			}
			got := c.strokePathHasDeviceAlignedButtCapPlane()
			if got != tt.want {
				t.Fatalf("strokePathHasDeviceAlignedButtCapPlane() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPopplerStrokePathAndMatrixUsesIdentityForDeviceAlignedButtCaps(t *testing.T) {
	c := newTestBackend(t, 1200, 1600)
	c.s.state.lineCap = int(LineCapButt)
	c.s.state.lineWidth = 2.08333339
	c.s.state.strokeAdjust = true
	c.path = xpath.NewPath()
	if err := c.path.MoveTo(294.7916667, 862.5); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := c.path.LineTo(294.7916667, 1114.5833333); err != nil {
		t.Fatalf("LineTo: %v", err)
	}

	_, matrix := c.popplerStrokePathAndMatrix()
	if matrix != [6]float64{1, 0, 0, 1, 0, 0} {
		t.Fatalf("device-aligned butt cap matrix = %v, want identity", matrix)
	}
}

func TestPopplerStrokePathAndMatrixKeepsDefaultForUnalignedTick(t *testing.T) {
	c := newTestBackend(t, 1200, 1600)
	c.s.state.lineCap = int(LineCapButt)
	c.s.state.lineWidth = 1.67798832
	c.s.state.strokeAdjust = true
	c.path = xpath.NewPath()
	if err := c.path.MoveTo(679.8175, 1286.5341); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if err := c.path.LineTo(679.8175, 1294.9242); err != nil {
		t.Fatalf("LineTo: %v", err)
	}

	_, matrix := c.popplerStrokePathAndMatrix()
	if matrix != c.pathYFlipMatrix() {
		t.Fatalf("unaligned tick matrix = %v, want path Y-flip matrix %v", matrix, c.pathYFlipMatrix())
	}
}

func TestStrokeDropsMoveOnlySubpathLikePopplerGfxPath(t *testing.T) {
	c := newTestBackend(t, 32, 32)
	c.SetStrokeColor(color.Black)
	c.SetLineWidth(6)
	c.SetLineCap(int(LineCapRound))

	c.MoveTo(16, 16)
	c.Stroke()

	r, g, b := readBackendPixel(t, c, 16, 16)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("move-only stroke changed center pixel to (%d,%d,%d), want white", r, g, b)
	}
}

func TestStrokeKeepsZeroLengthLineSegmentRoundCap(t *testing.T) {
	c := newTestBackend(t, 32, 32)
	c.SetStrokeColor(color.Black)
	c.SetLineWidth(6)
	c.SetLineCap(int(LineCapRound))

	c.MoveTo(16, 16)
	c.LineTo(16, 16)
	c.Stroke()

	r, g, b := readBackendPixel(t, c, 16, 16)
	if r == 255 && g == 255 && b == 255 {
		t.Fatal("zero-length line segment did not paint center pixel")
	}
}

func TestStrokePathWithCTMDropsMoveOnlySubpathLikePopplerGfxPath(t *testing.T) {
	c := newTestBackend(t, 32, 32)
	c.SetStrokeColor(color.Black)
	c.SetLineCap(int(LineCapRound))

	c.StrokePathWithCTM(
		[]renderer.PathElement{&renderer.MoveTo{X: 16, Y: 16}},
		[6]float64{1, 0, 0, 1, 0, 0},
		6,
		nil,
		0,
	)

	r, g, b := readBackendPixel(t, c, 16, 16)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("raw move-only stroke changed center pixel to (%d,%d,%d), want white", r, g, b)
	}
}

func TestStrokePathWithCTMKeepsZeroLengthLineSegmentRoundCap(t *testing.T) {
	c := newTestBackend(t, 32, 32)
	c.SetStrokeColor(color.Black)
	c.SetLineCap(int(LineCapRound))

	c.StrokePathWithCTM(
		[]renderer.PathElement{
			&renderer.MoveTo{X: 16, Y: 16},
			&renderer.LineTo{X: 16, Y: 16},
		},
		[6]float64{1, 0, 0, 1, 0, 0},
		6,
		nil,
		0,
	)

	r, g, b := readBackendPixel(t, c, 16, 16)
	if r == 255 && g == 255 && b == 255 {
		t.Fatal("raw zero-length line segment did not paint center pixel")
	}
}

func TestFillPathWithCTMAppliesRawPathMatrix(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	c.SetFillColor(color.Black)

	c.FillPathWithCTM(
		[]renderer.PathElement{
			&renderer.MoveTo{X: 1, Y: 1},
			&renderer.LineTo{X: 4, Y: 1},
			&renderer.LineTo{X: 4, Y: 4},
			&renderer.LineTo{X: 1, Y: 4},
			&renderer.Close{},
		},
		[6]float64{2, 0, 0, 2, 0, 0},
		false,
	)

	r, g, b := readBackendPixel(t, c, 3, 14)
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("raw fill CTM interior pixel = (%d,%d,%d), want black", r, g, b)
	}
	r, g, b = readBackendPixel(t, c, 1, 14)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("raw fill CTM outside pixel = (%d,%d,%d), want white", r, g, b)
	}
	if c.s.state.matrix != [6]float64{1, 0, 0, 1, 0, 0} {
		t.Fatalf("raw fill CTM left matrix %v, want identity", c.s.state.matrix)
	}
}

func TestImageDrawMatrixUsesPopplerType3ImageCTM(t *testing.T) {
	c := newTestBackend(t, 10, 17)
	c.SetPageYOriginPx(16.25)
	composed := [6]float64{2, 3, 4, 5, 6, 7}

	if got := c.imageDrawMatrix(composed); got != [6]float64{2, -3, 4, -5, 6, 9.25} {
		t.Fatalf("imageDrawMatrix outside Type3 = %v", got)
	}

	c.BeginType3Glyph()
	if got := c.imageDrawMatrix(composed); got != [6]float64{2, -3, -4, 5, 10, 4.25} {
		t.Fatalf("imageDrawMatrix inside Type3 = %v", got)
	}

	c.BeginType3Glyph()
	c.EndType3Glyph()
	if got := c.imageDrawMatrix(composed); got != [6]float64{2, -3, -4, 5, 10, 4.25} {
		t.Fatalf("nested Type3 should keep Poppler image matrix, got %v", got)
	}

	c.EndType3Glyph()
	if got := c.imageDrawMatrix(composed); got != [6]float64{2, -3, 4, -5, 6, 9.25} {
		t.Fatalf("imageDrawMatrix after Type3 = %v", got)
	}
}

func TestDrawImageWithSourceAlphaUsesImageAlpha(t *testing.T) {
	c := newTestBackend(t, 1, 1)
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})

	if err := c.DrawImageWithSourceAlpha(img, 0, 0, 1, 1, false); err != nil {
		t.Fatalf("DrawImageWithSourceAlpha: %v", err)
	}

	got := c.Image().(*image.RGBA).RGBAAt(0, 0)
	if got.R != 255 || got.G >= 255 || got.B >= 255 {
		t.Fatalf("source alpha did not blend red over white, got (%d,%d,%d,%d)", got.R, got.G, got.B, got.A)
	}
}

func TestDrawImageMaskUsesCurrentFill(t *testing.T) {
	c := newTestBackend(t, 1, 1)
	c.SetFillColor(color.RGBA{A: 255})
	gray := image.NewGray(image.Rect(0, 0, 1, 1))
	gray.SetGray(0, 0, color.Gray{Y: 255})

	if err := c.DrawImageMask(infraimage.NewBitmapMaskFromImage(gray, false), 0, 0, 1, 1); err != nil {
		t.Fatalf("DrawImageMask: %v", err)
	}

	got := c.Image().(*image.RGBA).RGBAAt(0, 0)
	if got.R != 0 || got.G != 0 || got.B != 0 || got.A != 255 {
		t.Fatalf("image mask fill pixel = (%d,%d,%d,%d), want opaque black", got.R, got.G, got.B, got.A)
	}
}

func TestStrokeTextGlyphPathDisablesStrokeAdjustOnlyDuringTextStroke(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	c.s.state.strokeAdjust = true
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}

	if err := c.strokeTextGlyphPath(font, 65, 4, 5, 10); err != nil {
		t.Fatalf("strokeTextGlyphPath: %v", err)
	}
	if !c.s.state.strokeAdjust {
		t.Fatal("strokeTextGlyphPath did not restore strokeAdjust")
	}
}

func TestStrokeTextGlyphPathUsesPopplerZeroLineWidth(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	c.SetGlyphTransform([4]float64{150.0 / 72.0, 0, 0, 150.0 / 72.0})
	c.s.state.lineWidth = 0
	font := &stubFont{name: "Stub", unitsPerEm: 1000, advance: 500}

	if got := c.popplerTextZeroStrokeLineWidth(); got != 1.0/150.0 {
		t.Fatalf("popplerTextZeroStrokeLineWidth = %.12f, want %.12f", got, 1.0/150.0)
	}
	if err := c.strokeTextGlyphPath(font, 65, 4, 5, 10); err != nil {
		t.Fatalf("strokeTextGlyphPath: %v", err)
	}
	if c.s.state.lineWidth != 0 {
		t.Fatalf("strokeTextGlyphPath lineWidth = %v, want restored zero", c.s.state.lineWidth)
	}
}

func TestSplashCanvasAlphaSettersForwardExtGStateAlpha(t *testing.T) {
	c := newTestBackend(t, 1, 1)

	c.SetFillAlpha(0.25)
	c.SetStrokeAlpha(0.5)

	if c.s.state.fillAlpha != 0.25 {
		t.Fatalf("fillAlpha = %v, want 0.25", c.s.state.fillAlpha)
	}
	if c.s.state.strokeAlpha != 0.5 {
		t.Fatalf("strokeAlpha = %v, want 0.5", c.s.state.strokeAlpha)
	}
}

func TestQuantizeType3GlyphOriginFloorsCacheablePlacementByDefault(t *testing.T) {
	c := newTestBackend(t, 10, 17)
	c.SetPageYOriginPx(1753.9375)

	x, y := c.QuantizeType3GlyphOrigin(330.747, 1169.4335)
	if x != 330 {
		t.Fatalf("quantized Type3 x = %v, want 330", x)
	}
	if y != 1169.9375 {
		t.Fatalf("quantized Type3 y = %v, want 1169.9375", y)
	}
}

func TestQuantizeType3GlyphOriginRawDebugMode(t *testing.T) {
	t.Setenv("PDF_DEBUG_TYPE3_GLYPH_ORIGIN_MODE", "raw")
	c := newTestBackend(t, 10, 17)
	c.SetPageYOriginPx(1753.9375)

	x, y := c.QuantizeType3GlyphOrigin(330.747, 1169.4335)
	if x != 330.747 {
		t.Fatalf("raw Type3 x = %v, want 330.747", x)
	}
	if y != 1169.4335 {
		t.Fatalf("raw Type3 y = %v, want 1169.4335", y)
	}
}

func TestType3GlyphCacheRectangleAddsPopplerMaskFringe(t *testing.T) {
	c := newTestBackend(t, 12, 14)
	c.SetFillColor(color.Black)
	c.BeginType3GlyphCache()
	c.Rectangle(2, 4, 4, 4)
	c.Fill()
	c.EndType3GlyphCache()

	assertRGB := func(x, y int, want byte) {
		t.Helper()
		got := c.Image().(*image.RGBA).RGBAAt(x, y)
		if got.R != want || got.G != want || got.B != want {
			t.Fatalf("pixel(%d,%d) = (%d,%d,%d), want (%d,%d,%d)", x, y, got.R, got.G, got.B, want, want, want)
		}
	}

	assertRGB(2, 6, 0)
	assertRGB(2, 5, 223)
	assertRGB(6, 6, 223)
	assertRGB(2, 10, 223)
	assertRGB(6, 5, 251)
	assertRGB(6, 10, 251)
	assertRGB(1, 6, 255)
}

// linearShadingFunction implements entity.Function as a linear ramp
// f(t) = t (one input → 3 outputs (t, t, t) for RGB).
type linearShadingFunction struct{}

func (linearShadingFunction) Evaluate(inputs []float64) ([]float64, error) {
	t := 0.0
	if len(inputs) > 0 {
		t = inputs[0]
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return []float64{t, t, t}, nil
}
func (linearShadingFunction) GetInputSize() int       { return 1 }
func (linearShadingFunction) GetOutputSize() int      { return 3 }
func (linearShadingFunction) GetDomain() [][2]float64 { return [][2]float64{{0, 1}} }
func (linearShadingFunction) GetRange() [][2]float64  { return [][2]float64{{0, 1}, {0, 1}, {0, 1}} }

// TestDrawShadingPatternAxial verifies axial shading produces a left→right
// horizontal gradient through the splash backend.
func TestDrawShadingPatternAxial(t *testing.T) {
	c := newTestBackend(t, 20, 8)
	// Axial: x0=0..x1=20, t0=0..t1=1, no /Extend, RGB function (t,t,t).
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 20, 0,
		[]entity.Function{linearShadingFunction{}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh1", shading)
	if err := c.DrawShadingPattern(pattern, [4]float64{0, 0, 20, 8}); err != nil {
		t.Fatalf("DrawShadingPattern err: %v", err)
	}
	// Sample left edge vs right edge — left should be darker than right.
	lr, lg, lb := readBackendPixel(t, c, 1, 4)
	rr, rg, rb := readBackendPixel(t, c, 18, 4)
	if !(lr < rr || lg < rg || lb < rb) {
		t.Fatalf("expected gradient left<right; got left=(%d,%d,%d) right=(%d,%d,%d)", lr, lg, lb, rr, rg, rb)
	}
}

// TestDrawShadingPatternRadial verifies radial shading produces a center≠edge
// pattern through the splash backend.
func TestDrawShadingPatternRadial(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	// Radial: small circle at (10,10,r=0) → big circle at (10,10,r=10).
	shading := entity.NewRadialShading("DeviceRGB", 10, 10, 0, 10, 10, 10,
		[]entity.Function{linearShadingFunction{}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh2", shading)
	if err := c.DrawShadingPattern(pattern, [4]float64{0, 0, 20, 20}); err != nil {
		t.Fatalf("DrawShadingPattern err: %v", err)
	}
	cR, cG, cB := readBackendPixel(t, c, 10, 10)
	eR, eG, eB := readBackendPixel(t, c, 1, 1)
	if cR == eR && cG == eG && cB == eB {
		t.Fatalf("expected radial center≠edge; got both=(%d,%d,%d)", cR, cG, cB)
	}
}

// TestDrawShadingPatternUnsupported verifies an unknown ShadingType returns
// a clear error.
func TestDrawShadingPatternUnsupported(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	shading := entity.NewShading(entity.ShadingType(99), "DeviceRGB")
	pattern := entity.NewShadingPattern("Bad", shading)
	err := c.DrawShadingPattern(pattern, [4]float64{0, 0, 8, 8})
	if err == nil {
		t.Fatalf("expected error for unsupported ShadingType, got nil")
	}
}

// TestDrawShadingPatternGouraud verifies gouraud mesh fill renders a triangle
// with interpolated vertex colors.
func TestDrawShadingPatternGouraud(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	// One triangle: red at (0,0), green at (20,0), blue at (10,20). Colors are
	// in [0,1] per channel — packShadingOutput scales to 0..255.
	verts := []entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(20, 0, []float64{0, 1, 0}),
		entity.NewVertex(10, 20, []float64{0, 0, 1}),
	}
	shading := entity.NewGouraudShading("DeviceRGB", entity.ShadingFreeFormGouraud, verts, 16, 8, nil)
	pattern := entity.NewShadingPattern("Gou", shading)
	if err := c.DrawShadingPattern(pattern, [4]float64{0, 0, 20, 20}); err != nil {
		t.Fatalf("DrawShadingPattern err: %v", err)
	}
	// Some pixel inside the triangle must have been written (non-paper).
	found := false
	for y := 0; y < 20 && !found; y++ {
		for x := 0; x < 20 && !found; x++ {
			r, g, b := readBackendPixel(t, c, x, y)
			if !(r == 0xFF && g == 0xFF && b == 0xFF) {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("gouraud fill produced no non-paper pixels")
	}
}

// TestDrawTilingPatternFallback verifies tiling pattern wiring is invoked and
// modifies the bitmap (Phase-3 fallback cell — visibly wrong but proves wiring).
func TestDrawTilingPatternFallback(t *testing.T) {
	c := newTestBackend(t, 20, 20)
	tp := entity.NewTilingPattern("Tile", 1, entity.TilingConstantSpacing)
	tp.SetBBox([4]float64{0, 0, 4, 4})
	tp.SetXStep(4)
	tp.SetYStep(4)
	tp.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	if err := c.DrawTilingPattern(tp, [4]float64{0, 0, 20, 20}); err != nil {
		t.Fatalf("DrawTilingPattern err: %v", err)
	}
	// At least one black pixel should have been written by the diagonal stripe
	// fallback cell.
	hasBlack := false
	for y := 0; y < 20 && !hasBlack; y++ {
		for x := 0; x < 20 && !hasBlack; x++ {
			r, g, b := readBackendPixel(t, c, x, y)
			if r == 0 && g == 0 && b == 0 {
				hasBlack = true
			}
		}
	}
	if !hasBlack {
		t.Fatalf("tiling fallback wrote no black pixels — wiring not invoked")
	}
}

// TestDrawTilingPatternNil verifies a nil pattern is a safe no-op.
func TestDrawTilingPatternNil(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	if err := c.DrawTilingPattern(nil, [4]float64{0, 0, 8, 8}); err != nil {
		t.Fatalf("nil tiling: expected nil err, got %v", err)
	}
}

// TestDrawImageRGBUpscale builds a 4×4 RGB source and draws it as an 8×8 rect
// through the splash backend.
func TestDrawImageRGBUpscale(t *testing.T) {
	c := newTestBackend(t, 16, 16)
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			// Encode position into red so we can detect propagation.
			src.Set(x, y, color.RGBA{R: byte(x * 60), G: byte(y * 60), B: 0, A: 255})
		}
	}
	if err := c.DrawImage(src, 0, 0, 8, 8, true); err != nil {
		t.Fatalf("DrawImage err: %v", err)
	}
	// The image is drawn after a Y-flip (PDF bottom-up). With src data varying
	// in R by column and G by row, at least one non-paper pixel must exist in
	// the destination rect.
	hit := 0
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			r, g, b := readBackendPixel(t, c, x, y)
			if !(r == 0xFF && g == 0xFF && b == 0xFF) {
				hit++
			}
		}
	}
	if hit == 0 {
		t.Fatalf("DrawImage wrote no non-paper pixels")
	}
}

// TestDrawImageNil verifies a nil image is a safe no-op.
func TestDrawImageNil(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	if err := c.DrawImage(nil, 0, 0, 8, 8, false); err != nil {
		t.Fatalf("nil image: expected nil err, got %v", err)
	}
}

func TestDrawImageWithSoftMaskInstallsPageMask(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	maskImg := image.NewGray(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 0x20, G: 0x40, B: 0x80, A: 0xff})
			if x < 2 {
				maskImg.SetGray(x, y, color.Gray{Y: 0xff})
			}
		}
	}

	mask := infraimage.NewBitmapMaskFromImage(maskImg, false)
	if err := c.DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(src, mask, 0, 0, 4, 4, false, "", 0, 0, ""); err != nil {
		t.Fatalf("DrawImageWithSoftMaskPhaseSamplerAndEdgeMode err: %v", err)
	}
	if c.s.state.softMask != nil {
		t.Fatalf("soft mask was not cleared after drawing")
	}

	hasInk := false
	hasPaper := false
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			r, g, b := readBackendPixel(t, c, x, y)
			if r == 0xff && g == 0xff && b == 0xff {
				hasPaper = true
				continue
			}
			hasInk = true
		}
	}
	if !hasInk || !hasPaper {
		t.Fatalf("expected masked draw to leave both ink and paper pixels, hasInk=%v hasPaper=%v", hasInk, hasPaper)
	}
}

func TestDrawImageWithSoftMaskIgnoresSourceAlpha(t *testing.T) {
	c := newTestBackend(t, 4, 4)
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	maskImg := image.NewGray(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 0xff, A: 0x80})
			maskImg.SetGray(x, y, color.Gray{Y: 0xff})
		}
	}

	mask := infraimage.NewBitmapMaskFromImage(maskImg, false)
	if err := c.DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(src, mask, 0, 0, 2, 2, false, "", 0, 0, ""); err != nil {
		t.Fatalf("DrawImageWithSoftMaskPhaseSamplerAndEdgeMode err: %v", err)
	}

	hasSourceOpaqueRed := false
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			r, g, b := readBackendPixel(t, c, x, y)
			if r > 0xf0 && g < 0x20 && b < 0x20 {
				hasSourceOpaqueRed = true
			}
		}
	}
	if !hasSourceOpaqueRed {
		t.Fatalf("soft-masked image should ignore source alpha and paint opaque source color")
	}
}

// TestSetFillPatternAxialShading verifies SetFillPattern translates an axial
// ShadingPattern into a splash AxialShader (no longer a black-solid fallback).
func TestSetFillPatternAxialShading(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 8, 0,
		[]entity.Function{linearShadingFunction{}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh", shading)
	c.SetFillPattern(pattern)
	if _, ok := c.s.state.fillPattern.(*AxialShader); !ok {
		t.Fatalf("expected fillPattern *AxialShader, got %T", c.s.state.fillPattern)
	}
}

func TestSetFillPatternAxialShadingSkipsTopLevelConstantPendingFill(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 8, 0,
		[]entity.Function{&entity.ExponentialFunction{
			C0:       []float64{1, 0.84, 0.06},
			C1:       []float64{1, 0.84, 0.06},
			Exponent: 1,
			N:        3,
		}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh", shading)

	c.SetFillPattern(pattern)

	if c.pendingFillShadingPattern != nil {
		t.Fatalf("constant top-level axial shading should stay on regular pattern path")
	}
}

func TestSetFillPatternAxialShadingSkipsTopLevelExponentialPendingFill(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 8, 0,
		[]entity.Function{&entity.ExponentialFunction{
			C0:       []float64{0.2, 0.5, 1},
			C1:       []float64{1, 0, 0.4},
			Exponent: 1,
			N:        3,
		}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh", shading)

	c.SetFillPattern(pattern)

	if c.pendingFillShadingPattern != nil {
		t.Fatalf("top-level exponential axial shading should stay on regular pattern path")
	}
}

func TestSetFillPatternAxialShadingUsesPendingFillForSampledTopLevel(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 8, 0,
		[]entity.Function{&entity.SampledFunction{
			Size:        []int{2},
			RangeVal:    [][2]float64{{0, 1}, {0, 1}, {0, 1}},
			Samples:     []float64{0, 0, 0, 1, 1, 1},
			Interpolate: true,
		}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh", shading)

	c.SetFillPattern(pattern)

	if c.pendingFillShadingPattern != pattern {
		t.Fatalf("varying top-level axial shading should use shaded-fill path")
	}
}

func TestSetFillPatternAxialShadingUsesPendingFillInTransparencyGroup(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	c.s.groupStack = append(c.s.groupStack, &groupState{})
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 8, 0,
		[]entity.Function{&entity.ExponentialFunction{
			C0:       []float64{1, 0.84, 0.06},
			C1:       []float64{1, 0.84, 0.06},
			Exponent: 1,
			N:        3,
		}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh", shading)

	c.SetFillPattern(pattern)

	if c.pendingFillShadingPattern != pattern {
		t.Fatalf("group axial shading should use shaded-fill path even when constant")
	}
}

// TestSetFillPatternNilPreservesSolidColor locks the renderer sync contract:
// syncCanvasColors calls SetFillColor before SetFillPattern(nil), and nil must
// not clobber that solid color with black.
func TestSetFillPatternNilPreservesSolidColor(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	c.SetFillColor(color.RGBA{R: 0x21, G: 0x43, B: 0x65, A: 0xff})

	before := c.s.state.fillPattern
	c.SetFillPattern(nil)

	if c.s.state.fillPattern != before {
		t.Fatalf("nil fill pattern replaced solid color: before=%T after=%T", before, c.s.state.fillPattern)
	}
	assertSolidPatternColor(t, c.s.state.fillPattern, Color{0x21, 0x43, 0x65})
}

// TestSetStrokePatternAxialShading mirrors TestSetFillPatternAxialShading for
// stroke ops.
func TestSetStrokePatternAxialShading(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	shading := entity.NewAxialShading("DeviceRGB", 0, 0, 8, 0,
		[]entity.Function{linearShadingFunction{}}, [2]bool{true, true})
	pattern := entity.NewShadingPattern("Sh", shading)
	c.SetStrokePattern(pattern)
	if _, ok := c.s.state.strokePattern.(*AxialShader); !ok {
		t.Fatalf("expected strokePattern *AxialShader, got %T", c.s.state.strokePattern)
	}
}

// TestSetStrokePatternNilPreservesSolidColor mirrors the nil fill-pattern
// contract for stroke state.
func TestSetStrokePatternNilPreservesSolidColor(t *testing.T) {
	c := newTestBackend(t, 8, 8)
	c.SetStrokeColor(color.RGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff})

	before := c.s.state.strokePattern
	c.SetStrokePattern(nil)

	if c.s.state.strokePattern != before {
		t.Fatalf("nil stroke pattern replaced solid color: before=%T after=%T", before, c.s.state.strokePattern)
	}
	assertSolidPatternColor(t, c.s.state.strokePattern, Color{0xaa, 0xbb, 0xcc})
}

// TestGlyphPathToXPathPreservesYDownCoordinates prevents reintroducing the
// double-Y-flip that rendered fallback glyph paths upside-down.
func TestGlyphPathToXPathPreservesYDownCoordinates(t *testing.T) {
	gp := &entity.GlyphPath{Commands: []entity.PathCommand{
		&entity.PathMoveTo{X: 0, Y: 0},
		&entity.PathLineTo{X: 4, Y: -8},
		&entity.PathLineTo{X: 8, Y: 0},
		&entity.PathClose{},
	}}

	p := glyphPathToXPath(gp)
	if p.Length() < 3 {
		t.Fatalf("expected at least 3 glyph path points, got %d", p.Length())
	}
	apex, _ := p.Point(1)
	if apex.Y != -8 {
		t.Fatalf("glyph Y coordinate was flipped: got apex Y=%v, want -8", apex.Y)
	}
}

func assertSolidPatternColor(t *testing.T, pattern Pattern, want Color) {
	t.Helper()
	solid, ok := pattern.(*SolidColor)
	if !ok {
		t.Fatalf("expected *SolidColor, got %T", pattern)
	}
	var got Color
	if !solid.GetColor(0, 0, &got) {
		t.Fatalf("solid pattern returned no color")
	}
	if got != want {
		t.Fatalf("solid color mismatch: got=%v want=%v", got, want)
	}
}
