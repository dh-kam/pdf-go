package splash

import (
	"testing"

	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
)

// newGroupSplashRGB returns a Splash bound to a fresh RGB8 bitmap whose data
// plane is preset to paper-white (so the contrast against group draws is
// visible) and whose alpha plane is fully opaque.
func newGroupSplashRGB(t *testing.T, w, h int) *Splash {
	t.Helper()
	bm := NewBitmap(w, h, ModeRGB8, true)
	for i := range bm.data {
		bm.data[i] = 0xFF
	}
	for i := range bm.alpha {
		bm.alpha[i] = 0xFF
	}
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.state.fillAlpha = 1
	s.state.strokeAlpha = 1
	return s
}

// TestBeginPaintRoundTrip verifies that drawing into a group then painting it
// transfers the group rect onto the parent (PDF spec 11.4.7 Begin/Paint).
func TestBeginPaintRoundTrip(t *testing.T) {
	s := newGroupSplashRGB(t, 20, 20)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 20, 20}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if s.bitmap == parent {
		t.Fatalf("Begin should swap bitmap")
	}
	// Paint a 10x10 black opaque block into the group bitmap (manually,
	// to keep the test free of fillImpl coupling).
	g := s.bitmap
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 0
			g.data[off+1] = 0
			g.data[off+2] = 0
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("Paint should restore parent bitmap")
	}
	// Inside the rect: black on white = black.
	off := 7*parent.rowSize + 7*3
	if parent.data[off] != 0 || parent.data[off+1] != 0 || parent.data[off+2] != 0 {
		t.Fatalf("(7,7) parent: got %02x%02x%02x, want 000000",
			parent.data[off], parent.data[off+1], parent.data[off+2])
	}
	// Outside the rect: untouched white.
	off = 1*parent.rowSize + 1*3
	if parent.data[off] != 0xFF || parent.data[off+1] != 0xFF || parent.data[off+2] != 0xFF {
		t.Fatalf("(1,1) parent: got %02x%02x%02x, want FFFFFF",
			parent.data[off], parent.data[off+1], parent.data[off+2])
	}
}

func TestPaintTransparencyGroupAppliesTransfer(t *testing.T) {
	s := newGroupSplashRGB(t, 2, 2)
	var red, green, blue, gray [256]byte
	for i := 0; i < 256; i++ {
		v := byte(i)
		red[i] = v
		green[i] = v
		blue[i] = v
		gray[i] = v
	}
	red[10] = 11
	green[20] = 22
	blue[30] = 33
	s.SetTransfer(red, green, blue, gray)

	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	group := s.bitmap
	group.data[0] = 10
	group.data[1] = 20
	group.data[2] = 30
	group.alpha[0] = 255

	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	if parent.data[0] != 11 || parent.data[1] != 22 || parent.data[2] != 33 {
		t.Fatalf("transferred group pixel = %d,%d,%d; want 11,22,33",
			parent.data[0], parent.data[1], parent.data[2])
	}
}

func TestEndTransparencyGroupAsSoftMaskAppliesBackdropAndTransfer(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	if err := s.BeginTransparencyGroup([4]float64{1, 1, 3, 3}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	group := s.bitmap
	off := 2*group.rowSize + 2*3
	group.data[off+0] = 255
	group.data[off+1] = 255
	group.data[off+2] = 255
	group.alpha[2*group.width+2] = 128

	var transfer [256]uint8
	for i := range transfer {
		transfer[i] = uint8(255 - i)
	}
	mask, err := s.EndTransparencyGroupAsSoftMaskWithOptions(domainrenderer.SoftMaskOptions{
		HasBackdrop:    true,
		BackdropRGB:    [3]uint8{255, 0, 0},
		BackdropLum:    77,
		Transfer:       transfer,
		TransferActive: true,
	})
	if err != nil {
		t.Fatalf("EndTransparencyGroupAsSoftMaskWithOptions: %v", err)
	}
	if got := mask.data[0]; got != 77 {
		t.Fatalf("outside group mask = %d, want backdrop luminosity 77", got)
	}
	if got := mask.data[2*mask.rowSize+2]; got != 89 {
		t.Fatalf("inside group mask = %d, want transferred composited luminosity 89", got)
	}
}

func TestPaintTransparencyGroupUsesCompositeBounds(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{2, 2, 4, 4}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	group := s.bitmap
	for y := 0; y < group.height; y++ {
		for x := 0; x < group.width; x++ {
			off := y*group.rowSize + x*3
			group.data[off+0] = 0
			group.data[off+1] = 0
			group.data[off+2] = 0
			group.alpha[y*group.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}

	inside := 3*parent.rowSize + 3*3
	if parent.data[inside] != 0 || parent.data[inside+1] != 0 || parent.data[inside+2] != 0 {
		t.Fatalf("(3,3) parent: got %02x%02x%02x, want 000000",
			parent.data[inside], parent.data[inside+1], parent.data[inside+2])
	}
	for _, pt := range [][2]int{{1, 1}, {5, 5}} {
		off := pt[1]*parent.rowSize + pt[0]*3
		if parent.data[off] != 0xFF || parent.data[off+1] != 0xFF || parent.data[off+2] != 0xFF {
			t.Fatalf("(%d,%d) parent: got %02x%02x%02x, want FFFFFF",
				pt[0], pt[1], parent.data[off], parent.data[off+1], parent.data[off+2])
		}
	}
}

func TestCompositeGroupRectAppliesNonIsolatedCorrection(t *testing.T) {
	dst := NewBitmap(1, 1, ModeRGB8, false)
	src := NewBitmap(1, 1, ModeRGB8, true)
	for i := 0; i < 3; i++ {
		dst.data[i] = 100
		src.data[i] = 150
	}
	src.alpha[0] = 128

	compositeGroupRect(src, dst, nil, nil, 1, true, [4]int{0, 0, 1, 1}, nil)

	for i := 0; i < 3; i++ {
		if dst.data[i] != 149 {
			t.Fatalf("channel %d = %d, want 149", i, dst.data[i])
		}
	}
}

func TestBeginTransparencyGroupDisablesStrokeAdjustLikePoppler(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	s.state.strokeAdjust = true

	if err := s.BeginTransparencyGroup([4]float64{0, 0, 4, 4}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if s.state.strokeAdjust {
		t.Fatalf("group render should disable strokeAdjust")
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	if !s.state.strokeAdjust {
		t.Fatalf("Paint should restore parent strokeAdjust")
	}
}

func TestPaintTransparencyGroupHonorsParentClip(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	if err := s.ClipToRect(0, 0, 4, 8); err != nil {
		t.Fatalf("ClipToRect: %v", err)
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 8, 8}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	group := s.bitmap
	for y := 0; y < group.height; y++ {
		for x := 0; x < group.width; x++ {
			off := y*group.rowSize + x*3
			group.data[off+0] = 0
			group.data[off+1] = 0
			group.data[off+2] = 0
			group.alpha[y*group.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}

	inside := 2*parent.rowSize + 2*3
	if parent.data[inside] != 0 || parent.data[inside+1] != 0 || parent.data[inside+2] != 0 {
		t.Fatalf("(2,2) parent: got %02x%02x%02x, want 000000",
			parent.data[inside], parent.data[inside+1], parent.data[inside+2])
	}
	outside := 2*parent.rowSize + 5*3
	if parent.data[outside] != 0xFF || parent.data[outside+1] != 0xFF || parent.data[outside+2] != 0xFF {
		t.Fatalf("(5,2) parent: got %02x%02x%02x, want FFFFFF",
			parent.data[outside], parent.data[outside+1], parent.data[outside+2])
	}
}

func TestNonIsolatedGroupAlpha0ProbeMatchesPopplerPipeAlpha(t *testing.T) {
	s := newGroupSplashRGB(t, 2, 2)
	parent := s.bitmap
	for i := range parent.alpha {
		parent.alpha[i] = 128
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, false, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	group := s.bitmap
	if got := group.alpha[0]; got != 0 {
		t.Fatalf("group alpha should start transparent under alpha0 probe, got %d", got)
	}
	if !s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != parent {
		t.Fatalf("alpha0 backdrop was not installed for non-isolated group")
	}

	s.state.fillAlpha = 0.5
	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, byte(Round(s.state.fillAlpha*255)), false, false)
	pipeRun(&p, 1)

	// Poppler Splash::pipeRun with alpha0=128, aDest=0, aSrc=128:
	// alphaI = 192, color = (64*255 + 128*0) / 192 = 85.
	if got := group.data[0]; got < 84 || got > 86 {
		t.Fatalf("group R with alpha0 correction = %d, want ~85", got)
	}
	if got := group.alpha[0]; got != 128 {
		t.Fatalf("group alpha stores aResult=%d, want 128", got)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != nil {
		t.Fatalf("alpha0 state should be restored after group end")
	}
}

func TestCroppedNonIsolatedGroupAlpha0UsesCropOffset(t *testing.T) {
	s := newGroupSplashRGB(t, 5, 4)
	parent := s.bitmap
	for i := range parent.alpha {
		parent.alpha[i] = 0
	}
	parent.alpha[1*parent.width+2] = 128

	tx, ty, err := s.BeginCroppedTransparencyGroup([4]float64{2, 1, 4, 3}, false, false, nil)
	if err != nil {
		t.Fatalf("BeginCroppedTransparencyGroup: %v", err)
	}
	if tx != 2 || ty != 1 {
		t.Fatalf("crop offset=(%d,%d), want (2,1)", tx, ty)
	}
	if s.nonIsoAlpha0 != parent || s.nonIsoAlpha0X != 2 || s.nonIsoAlpha0Y != 1 {
		t.Fatalf("alpha0 offset=(%d,%d), want (2,1)", s.nonIsoAlpha0X, s.nonIsoAlpha0Y)
	}

	group := s.bitmap
	s.state.fillAlpha = 0.5
	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, byte(Round(s.state.fillAlpha*255)), false, false)
	pipeRun(&p, 1)

	if got := group.data[0]; got < 84 || got > 86 {
		t.Fatalf("cropped group R with alpha0 offset = %d, want ~85", got)
	}
	if got := group.alpha[0]; got != 128 {
		t.Fatalf("cropped group alpha stores aResult=%d, want 128", got)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != nil || s.nonIsoAlpha0X != 0 || s.nonIsoAlpha0Y != 0 {
		t.Fatalf("alpha0 state should be restored after group end")
	}
}

func TestGroupCompositeAlpha0GateUsesAncestorBackdrop(t *testing.T) {
	s := newGroupSplashRGB(t, 2, 2)
	parent := s.bitmap
	for i := range parent.alpha {
		parent.alpha[i] = 128
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, false, false, nil); err != nil {
		t.Fatalf("Begin outer: %v", err)
	}
	outer := s.bitmap
	if !s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != parent {
		t.Fatalf("outer group did not install alpha0 backdrop")
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, true, false, nil); err != nil {
		t.Fatalf("Begin inner: %v", err)
	}
	inner := s.bitmap
	inner.data[0], inner.data[1], inner.data[2] = 0, 0, 0
	inner.alpha[0] = 128

	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint inner: %v", err)
	}
	// Poppler Splash::pipeRun with alpha0=128, aDest=0, aSrc=128:
	// alphaI=192 and Normal blend yields (64*255 + 128*0) / 192 = 85.
	if got := outer.data[0]; got < 84 || got > 86 {
		t.Fatalf("outer R after alpha0 group composite = %d, want ~85", got)
	}
	if got := outer.alpha[0]; got != 128 {
		t.Fatalf("outer alpha stores aResult=%d, want 128", got)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End outer: %v", err)
	}
}

func TestGroupCompositeAlpha0CanBeDisabledForIsolation(t *testing.T) {
	t.Setenv("PDF_DEBUG_SPLASH_GROUP_ALPHA0_COMPOSITE", "0")
	s := newGroupSplashRGB(t, 2, 2)
	parent := s.bitmap
	for i := range parent.alpha {
		parent.alpha[i] = 128
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, false, false, nil); err != nil {
		t.Fatalf("Begin outer: %v", err)
	}
	outer := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, true, false, nil); err != nil {
		t.Fatalf("Begin inner: %v", err)
	}
	inner := s.bitmap
	inner.data[0], inner.data[1], inner.data[2] = 0, 0, 0
	inner.alpha[0] = 128

	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint inner: %v", err)
	}
	if got := outer.data[0]; got != 0 {
		t.Fatalf("outer R with alpha0 composite disabled = %d, want legacy black", got)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End outer: %v", err)
	}
}

func TestSoftMaskNestedIsolatedGroupFreshStateDefault(t *testing.T) {
	s := newGroupSplashRGB(t, 2, 2)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, false, false, nil); err != nil {
		t.Fatalf("Begin soft-mask parent: %v", err)
	}
	s.markTopGroupForSoftMask()
	if !s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != parent {
		t.Fatalf("soft-mask parent should install non-isolated alpha0 backdrop")
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, true, false, nil); err != nil {
		t.Fatalf("Begin isolated child: %v", err)
	}
	if s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != nil || s.nonIsoAlpha0X != 0 || s.nonIsoAlpha0Y != 0 {
		t.Fatalf("isolated child inherited soft-mask alpha0 state")
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End isolated child: %v", err)
	}
	if !s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != parent {
		t.Fatalf("ending isolated child should restore soft-mask parent alpha0 state")
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End soft-mask parent: %v", err)
	}
}

func TestSoftMaskNestedIsolatedGroupFreshStateGateCanBeDisabled(t *testing.T) {
	t.Setenv("GO_PDF_SMASK_NESTED_ISOLATED_FRESH_STATE", "0")
	s := newGroupSplashRGB(t, 2, 2)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, false, false, nil); err != nil {
		t.Fatalf("Begin soft-mask parent: %v", err)
	}
	s.markTopGroupForSoftMask()
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, true, false, nil); err != nil {
		t.Fatalf("Begin isolated child: %v", err)
	}
	if !s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != parent {
		t.Fatalf("disabled gate should preserve existing alpha0 inheritance")
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End isolated child: %v", err)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End soft-mask parent: %v", err)
	}
}

func TestNonIsolatedGroupAlpha0CanBeDisabledForIsolation(t *testing.T) {
	t.Setenv("PDF_DEBUG_SPLASH_NON_ISOLATED_ALPHA0", "0")
	s := newGroupSplashRGB(t, 2, 2)
	parent := s.bitmap
	for i := range parent.alpha {
		parent.alpha[i] = 128
	}
	if err := s.BeginTransparencyGroup([4]float64{0, 0, 2, 2}, false, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	group := s.bitmap
	if got := group.alpha[0]; got != 128 {
		t.Fatalf("group alpha should preserve copied backdrop when alpha0 is disabled, got %d", got)
	}
	if s.state.inNonIsolatedGroup || s.nonIsoAlpha0 != nil {
		t.Fatalf("alpha0 backdrop should not be installed when disabled")
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End: %v", err)
	}
}

// TestGroupStackDepth nests Begin twice, draws in the inner group, then Paints
// twice — the inner content must reach the outermost (parent) bitmap.
func TestGroupStackDepth(t *testing.T) {
	s := newGroupSplashRGB(t, 16, 16)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin outer: %v", err)
	}
	outer := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin inner: %v", err)
	}
	if s.bitmap == outer || s.bitmap == parent {
		t.Fatalf("inner group did not swap bitmap")
	}
	// Black block at (4..8) in inner.
	g := s.bitmap
	for y := 4; y < 8; y++ {
		for x := 4; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 0
			g.data[off+1] = 0
			g.data[off+2] = 0
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint inner: %v", err)
	}
	if s.bitmap != outer {
		t.Fatalf("after inner Paint expected outer bitmap")
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint outer: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("after outer Paint expected parent bitmap")
	}
	off := 5*parent.rowSize + 5*3
	if parent.data[off] != 0 || parent.data[off+1] != 0 || parent.data[off+2] != 0 {
		t.Fatalf("(5,5) parent: got %02x%02x%02x, want 000000",
			parent.data[off], parent.data[off+1], parent.data[off+2])
	}
}

func TestEndTransparencyGroupAsSoftMaskUsesGroupAlpha(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	group := s.bitmap
	group.alpha[1*group.width+1] = 128
	group.alpha[2*group.width+2] = 255

	mask, err := s.EndTransparencyGroupAsSoftMask(true)
	if err != nil {
		t.Fatalf("EndTransparencyGroupAsSoftMask: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("soft mask group should restore parent bitmap")
	}
	if got := softMaskByte(mask, 1, 1); got != 128 {
		t.Fatalf("mask(1,1)=%d, want 128", got)
	}
	if got := softMaskByte(mask, 2, 2); got != 255 {
		t.Fatalf("mask(2,2)=%d, want 255", got)
	}
	if got := softMaskByte(mask, 0, 0); got != 0 {
		t.Fatalf("mask(0,0)=%d, want 0", got)
	}
}

func TestEndTransparencyGroupAsSoftMaskUsesGroupBounds(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	parent := s.bitmap
	for y := 0; y < parent.height; y++ {
		for x := 0; x < parent.width; x++ {
			off := y*parent.rowSize + x*3
			parent.data[off+0] = 200
			parent.data[off+1] = 10
			parent.data[off+2] = 10
			parent.alpha[y*parent.width+x] = 255
		}
	}

	if err := s.BeginTransparencyGroup([4]float64{1, 1, 3, 3}, false, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	mask, err := s.EndTransparencyGroupAsSoftMask(false)
	if err != nil {
		t.Fatalf("EndTransparencyGroupAsSoftMask: %v", err)
	}
	if got := softMaskByte(mask, 0, 0); got != 0 {
		t.Fatalf("mask outside group bounds=%d, want 0", got)
	}
	if got := softMaskByte(mask, 1, 1); got == 0 {
		t.Fatalf("mask inside group bounds should preserve group luminosity")
	}
}

func TestCroppedTransparencyGroupAsSoftMaskCopiesAtOffset(t *testing.T) {
	s := newGroupSplashRGB(t, 6, 5)
	parent := s.bitmap
	tx, ty, err := s.BeginCroppedTransparencyGroup([4]float64{2, 1, 4, 3}, true, false, nil)
	if err != nil {
		t.Fatalf("BeginCroppedTransparencyGroup: %v", err)
	}
	if tx != 2 || ty != 1 {
		t.Fatalf("crop offset=(%d,%d), want (2,1)", tx, ty)
	}
	if s.bitmap == parent {
		t.Fatalf("BeginCroppedTransparencyGroup should swap bitmap")
	}
	if s.bitmap.width != 3 || s.bitmap.height != 3 {
		t.Fatalf("cropped bitmap size=%dx%d, want 3x3", s.bitmap.width, s.bitmap.height)
	}

	group := s.bitmap
	off := 0*group.rowSize + 0*3
	group.data[off+0] = 255
	group.data[off+1] = 255
	group.data[off+2] = 255
	group.alpha[0] = 255

	mask, err := s.EndTransparencyGroupAsSoftMask(false)
	if err != nil {
		t.Fatalf("EndTransparencyGroupAsSoftMask: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("soft mask group should restore parent bitmap")
	}
	if got := softMaskByte(mask, 2, 1); got != 255 {
		t.Fatalf("mask at cropped origin=%d, want 255", got)
	}
	if got := softMaskByte(mask, 0, 0); got != 0 {
		t.Fatalf("mask outside cropped group=%d, want 0", got)
	}
}

// TestSoftMaskGating sets a uniform 50% mask and runs the AA pipe with a
// black, opaque source on a white backdrop. Result must be ~50% gray
// (black at 50% alpha over white = 127 or 128 due to integer rounding).
func TestSoftMaskGating(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	parent := s.bitmap
	mask := NewBitmap(4, 4, ModeMono8, false)
	for i := range mask.data {
		mask.data[i] = 128
	}
	s.SetSoftMask(mask)

	// Drive the AA pipe directly with full-coverage shape and opaque alpha.
	src := Color{0, 0, 0}
	var p pipe
	for y := 0; y < 4; y++ {
		s.pipeInit(&p, 0, y, nil, &src, 255, true, false)
		p.shape = 255
		pipeRun(&p, 4)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			off := y*parent.rowSize + x*3
			r := parent.data[off]
			if r < 124 || r > 132 {
				t.Fatalf("(%d,%d): got R=%d, want ~128 (50%% black over white)", x, y, r)
			}
		}
	}
}

func TestSoftMaskWithoutShapeUsesMask(t *testing.T) {
	s := newGroupSplashRGB(t, 1, 1)
	parent := s.bitmap
	mask := NewBitmap(1, 1, ModeMono8, false)
	mask.data[0] = 128
	s.SetSoftMask(mask)

	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, 255, false, false)
	p.run(&p)

	r := parent.data[0]
	if r < 124 || r > 132 {
		t.Fatalf("got R=%d, want ~128 from soft mask without source shape", r)
	}
}

func TestSoftMaskWithoutAlphaPlaneTreatsBackdropAsOpaque(t *testing.T) {
	bm := NewBitmap(1, 1, ModeRGB8, false)
	for i := range bm.data {
		bm.data[i] = 0xFF
	}
	s, err := New(bm, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mask := NewBitmap(1, 1, ModeMono8, false)
	mask.data[0] = 128
	s.SetSoftMask(mask)

	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, 255, false, false)
	p.run(&p)

	r := bm.data[0]
	if r < 124 || r > 132 {
		t.Fatalf("got R=%d, want ~128 from soft mask over opaque RGB bitmap", r)
	}
}

// TestSoftMaskCleared verifies that SetSoftMask(nil) restores the no-mask
// path so a fully-opaque source paints solid black.
func TestSoftMaskCleared(t *testing.T) {
	s := newGroupSplashRGB(t, 4, 4)
	parent := s.bitmap
	// First install then clear.
	mask := NewBitmap(4, 4, ModeMono8, false)
	for i := range mask.data {
		mask.data[i] = 128
	}
	s.SetSoftMask(mask)
	s.SetSoftMask(nil)
	if s.state.softMask != nil {
		t.Fatalf("softMask should be nil after clear")
	}
	src := Color{0, 0, 0}
	var p pipe
	for y := 0; y < 4; y++ {
		s.pipeInit(&p, 0, y, nil, &src, 255, true, false)
		p.shape = 255
		pipeRun(&p, 4)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			off := y*parent.rowSize + x*3
			if parent.data[off] != 0 {
				t.Fatalf("(%d,%d): got R=%d, want 0 (full black)", x, y, parent.data[off])
			}
		}
	}
}

// TestGroupBlendMode draws a 50% gray block into a group, then paints over a
// fully-white parent under BlendMultiply. Multiply: 128 * 255 / 255 == 128;
// alpha-mix on opaque src yields 128 (the blended source replaces white).
func TestGroupBlendMode(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, BlendMultiply); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	g := s.bitmap
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 128
			g.data[off+1] = 128
			g.data[off+2] = 128
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	off := 4*parent.rowSize + 4*3
	r, gg, b := parent.data[off], parent.data[off+1], parent.data[off+2]
	if r < 124 || r > 132 || gg < 124 || gg > 132 || b < 124 || b > 132 {
		t.Fatalf("(4,4) parent: got %d,%d,%d, want ~128,128,128", r, gg, b)
	}
}

// TestGroupPaintUsesRestoredParentBlendMode verifies the Poppler Form group
// sequence: render group contents with Normal, restore the parent state, then
// composite with the parent's active blend mode.
func TestGroupPaintUsesRestoredParentBlendMode(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			off := y*parent.rowSize + x*3
			parent.data[off+0] = 128
			parent.data[off+1] = 128
			parent.data[off+2] = 128
		}
	}
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	g := s.bitmap
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 128
			g.data[off+1] = 128
			g.data[off+2] = 128
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	s.SetBlendFunc(BlendMultiply)
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	off := 4*parent.rowSize + 4*3
	r, gg, b := parent.data[off], parent.data[off+1], parent.data[off+2]
	if r < 62 || r > 66 || gg < 62 || gg > 66 || b < 62 || b > 66 {
		t.Fatalf("(4,4) parent: got %d,%d,%d, want ~64,64,64", r, gg, b)
	}
}

// TestEndTransparencyGroupDiscards verifies that End discards an unpainted
// group and restores the parent target.
func TestEndTransparencyGroupDiscards(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	g := s.bitmap
	// Draw black; should NOT reach parent because we End instead of Paint.
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			off := y*g.rowSize + x*3
			g.data[off+0] = 0
			g.data[off+1] = 0
			g.data[off+2] = 0
			g.alpha[y*g.width+x] = 0xFF
		}
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("End should restore parent")
	}
	off := 4*parent.rowSize + 4*3
	if parent.data[off] != 0xFF {
		t.Fatalf("(4,4) parent should remain white after discard, got R=%d", parent.data[off])
	}
}

func TestBeginTransparencyGroupFreshPatternAlphaGate(t *testing.T) {
	t.Setenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA", "1")
	s := newGroupSplashRGB(t, 8, 8)
	parent := s.bitmap
	s.state.strokeAlpha = 0.4
	s.state.fillAlpha = 0.25
	s.state.patternStrokeAlpha = 0.4
	s.state.patternFillAlpha = 0.25
	s.state.multiplyPatternAlpha = true
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if s.bitmap == parent {
		t.Fatalf("Begin should swap bitmap")
	}
	if s.state.strokeAlpha != 1 || s.state.fillAlpha != 1 {
		t.Fatalf("group alpha = stroke %v fill %v, want 1,1", s.state.strokeAlpha, s.state.fillAlpha)
	}
	if s.state.patternStrokeAlpha != 1 || s.state.patternFillAlpha != 1 || s.state.multiplyPatternAlpha {
		t.Fatalf("group pattern alpha = stroke %v fill %v multiply %v, want 1,1,false",
			s.state.patternStrokeAlpha, s.state.patternFillAlpha, s.state.multiplyPatternAlpha)
	}
	if err := s.PaintTransparencyGroup(); err != nil {
		t.Fatalf("Paint: %v", err)
	}
	if s.bitmap != parent {
		t.Fatalf("Paint should restore parent")
	}
	if s.state.strokeAlpha != 0.4 || s.state.fillAlpha != 0.25 {
		t.Fatalf("restored alpha = stroke %v fill %v, want 0.4,0.25", s.state.strokeAlpha, s.state.fillAlpha)
	}
	if s.state.patternStrokeAlpha != 0.4 || s.state.patternFillAlpha != 0.25 || !s.state.multiplyPatternAlpha {
		t.Fatalf("restored pattern alpha = stroke %v fill %v multiply %v, want 0.4,0.25,true",
			s.state.patternStrokeAlpha, s.state.patternFillAlpha, s.state.multiplyPatternAlpha)
	}
}

func TestBeginTransparencyGroupFreshPatternAlphaNextGroup(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	s.state.fillAlpha = 0.5
	s.state.patternFillAlpha = 0.5
	s.state.multiplyPatternAlpha = true
	s.freshPatternAlphaForNextGroup = true
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if s.freshPatternAlphaForNextGroup {
		t.Fatalf("freshPatternAlphaForNextGroup should be consumed")
	}
	if s.state.fillAlpha != 1 || s.state.patternFillAlpha != 1 || s.state.multiplyPatternAlpha {
		t.Fatalf("group fill alpha = %v pattern %v multiply %v, want 1,1,false",
			s.state.fillAlpha, s.state.patternFillAlpha, s.state.multiplyPatternAlpha)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if s.state.fillAlpha != 0.5 || s.state.patternFillAlpha != 0.5 || !s.state.multiplyPatternAlpha {
		t.Fatalf("restored fill alpha = %v pattern %v multiply %v, want 0.5,0.5,true",
			s.state.fillAlpha, s.state.patternFillAlpha, s.state.multiplyPatternAlpha)
	}
}

func TestNestedTransparencyGroupFreshPatternAlphaDefault(t *testing.T) {
	s := newGroupSplashRGB(t, 8, 8)
	s.state.fillAlpha = 0.25
	s.state.patternFillAlpha = 0.25
	s.state.multiplyPatternAlpha = true
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin outer: %v", err)
	}
	if s.state.fillAlpha != 0.25 || s.state.patternFillAlpha != 0.25 || !s.state.multiplyPatternAlpha {
		t.Fatalf("outer group should keep parent pattern alpha state")
	}
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin inner: %v", err)
	}
	if s.state.fillAlpha != 1 || s.state.patternFillAlpha != 1 || s.state.multiplyPatternAlpha {
		t.Fatalf("nested group fill alpha = %v pattern %v multiply %v, want 1,1,false",
			s.state.fillAlpha, s.state.patternFillAlpha, s.state.multiplyPatternAlpha)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End inner: %v", err)
	}
	if s.state.fillAlpha != 0.25 || s.state.patternFillAlpha != 0.25 || !s.state.multiplyPatternAlpha {
		t.Fatalf("inner end should restore parent pattern alpha state")
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End outer: %v", err)
	}
}

func TestNestedTransparencyGroupFreshPatternAlphaCanBeDisabled(t *testing.T) {
	t.Setenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA_NESTED", "0")
	s := newGroupSplashRGB(t, 8, 8)
	s.state.fillAlpha = 0.25
	s.state.patternFillAlpha = 0.25
	s.state.multiplyPatternAlpha = true
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin outer: %v", err)
	}
	if err := s.BeginTransparencyGroup([4]float64{}, true, false, nil); err != nil {
		t.Fatalf("Begin inner: %v", err)
	}
	if s.state.fillAlpha != 0.25 || s.state.patternFillAlpha != 0.25 || !s.state.multiplyPatternAlpha {
		t.Fatalf("nested group should keep pattern alpha state when disabled")
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End inner: %v", err)
	}
	if err := s.EndTransparencyGroup(); err != nil {
		t.Fatalf("End outer: %v", err)
	}
}

// TestSoftMaskOutOfBounds drives the AA pipe past the mask's right/bottom
// edges; softMaskByte must clamp to 0 (fully masked) so destination stays
// untouched white instead of panicking.
func TestSoftMaskOutOfBounds(t *testing.T) {
	s := newGroupSplashRGB(t, 6, 6)
	parent := s.bitmap
	mask := NewBitmap(2, 2, ModeMono8, false)
	for i := range mask.data {
		mask.data[i] = 255
	}
	s.SetSoftMask(mask)
	src := Color{0, 0, 0}
	var p pipe
	s.pipeInit(&p, 0, 0, nil, &src, 255, true, false)
	p.shape = 255
	pipeRun(&p, 6)
	// (0,0) and (1,0): mask=255 → black.
	if parent.data[0*parent.rowSize+0*3] != 0 {
		t.Fatalf("(0,0): expected black under mask=255")
	}
	if parent.data[0*parent.rowSize+1*3] != 0 {
		t.Fatalf("(1,0): expected black under mask=255")
	}
	// (2,0)..(5,0): out of mask bounds → mask byte = 0 → src alpha = 0 →
	// destination preserved white.
	for x := 2; x < 6; x++ {
		off := 0*parent.rowSize + x*3
		if parent.data[off] != 0xFF {
			t.Fatalf("(%d,0): expected untouched white, got R=%d", x, parent.data[off])
		}
	}
}
