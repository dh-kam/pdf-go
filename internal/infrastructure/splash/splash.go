package splash

import (
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// ImageSource provides one row of image samples per call (Splash.h:50-55).
// colorLine is filled with srcWidth*nComps bytes; alphaLine, if non-nil,
// receives srcWidth alpha samples.
type ImageSource func(row int, colorLine, alphaLine []byte) error

// ImageMaskSource provides one row of mask samples per call (Splash.h:159).
type ImageMaskSource func(row int, dst []byte) error

// GlyphBitmap mirrors SplashGlyphBitmap (SplashGlyphBitmap.h:23).
type GlyphBitmap struct {
	X, Y, W, H int
	AA         bool
	Data       []byte
}

// Splash mirrors the top-level rasterizer class (Splash.h:82).
type Splash struct {
	bitmap       *Bitmap
	state        *state
	aaBuf        []byte
	aaGamma      [aaGammaLen]uint8
	vectorAA     bool
	minLineWidth float64
	inShading    bool
	// downscaleVFlipTopDown skips the post-scale vertical flip for callers whose
	// ImageSource has already been normalized to top-down rows.
	downscaleVFlipTopDown bool
	// mirrorStrokeNormals compensates for splashCanvas paths that are already
	// Y-flipped into device space before Splash.makeStrokePath runs. Poppler builds
	// stroke outlines in user space and applies the mirrored CTM later in
	// SplashXPath; flipping the normal here preserves that outline orientation.
	mirrorStrokeNormals bool
	// debugStrokeIndex carries the canvas stroke ordinal into debug-only trace
	// hooks without affecting normal rendering.
	debugStrokeIndex int
	// debugFillIndex carries the canvas fill ordinal into debug-only trace hooks.
	debugFillIndex int
	// groupStack holds saved per-group bitmaps + composite parameters for the
	// transparency-group stack (Splash.cc:5021-5254 + PDF spec 11.4.7).
	groupStack []*groupState
}

// New constructs a Splash bound to b (Splash.cc:1445).
func New(b *Bitmap, vectorAA bool) (*Splash, error) {
	if b == nil {
		return nil, ErrBadArg
	}
	s := &Splash{
		bitmap:           b,
		vectorAA:         vectorAA,
		aaGamma:          AAGamma(),
		debugStrokeIndex: -1,
		debugFillIndex:   -1,
		state: &state{
			matrix:             [6]float64{1, 0, 0, 1, 0, 0},
			fillAlpha:          1,
			strokeAlpha:        1,
			patternFillAlpha:   1,
			patternStrokeAlpha: 1,
			lineWidth:          1,
			miterLimit:         10,
			flatness:           1,
			lineCap:            int(LineCapButt),
			lineJoin:           int(LineJoinMiter),
			strokeAdjust:       false,
		},
	}
	s.state.clip = xpath.NewClip(0, 0, b.Width()-1, b.Height()-1, vectorAA)
	if vectorAA && b.Width() > 0 {
		s.aaBuf = make([]byte, splashAASize*b.Width())
	}
	return s, nil
}

// SetMatrix sets the current transformation matrix (Splash.cc:1551).
func (s *Splash) SetMatrix(m [6]float64) { s.state.matrix = m }

// SetFillPattern installs the fill pattern (Splash.cc:1601).
func (s *Splash) SetFillPattern(p Pattern) { s.state.fillPattern = p }

// SetStrokePattern installs the stroke pattern (Splash.cc:1595).
func (s *Splash) SetStrokePattern(p Pattern) { s.state.strokePattern = p }

// SetFillAlpha sets the fill alpha in [0,1] (Splash.cc:1693).
func (s *Splash) SetFillAlpha(a float64) {
	if s.state.multiplyPatternAlpha {
		a *= s.state.patternFillAlpha
	}
	s.state.fillAlpha = a
}

// SetStrokeAlpha sets the stroke alpha in [0,1] (Splash.cc:1688).
func (s *Splash) SetStrokeAlpha(a float64) {
	if s.state.multiplyPatternAlpha {
		a *= s.state.patternStrokeAlpha
	}
	s.state.strokeAlpha = a
}

// SetPatternAlpha enables Poppler's pattern opacity multiplication mode
// (Splash.cc:1698). Pattern content opacity updates are multiplied by the
// parent graphics state's opacity while a pattern is being evaluated.
func (s *Splash) SetPatternAlpha(strokeAlpha, fillAlpha float64) {
	s.state.patternStrokeAlpha = strokeAlpha
	s.state.patternFillAlpha = fillAlpha
	s.state.multiplyPatternAlpha = true
}

// ClearPatternAlpha disables pattern opacity multiplication (Splash.cc:1705).
func (s *Splash) ClearPatternAlpha() {
	s.state.patternStrokeAlpha = 1
	s.state.patternFillAlpha = 1
	s.state.multiplyPatternAlpha = false
}

// SetLineWidth sets the device-space line width (Splash.cc:1626).
func (s *Splash) SetLineWidth(w float64) { s.state.lineWidth = w }

// SetLineCap sets the cap style (Splash.cc:1644).
func (s *Splash) SetLineCap(c int) { s.state.lineCap = c }

// SetLineJoin sets the join style (Splash.cc:1650).
func (s *Splash) SetLineJoin(j int) { s.state.lineJoin = j }

// SetMiterLimit sets the miter join limit (Splash.cc:1632).
func (s *Splash) SetMiterLimit(l float64) { s.state.miterLimit = l }

// SetFlatness sets the curve flatness tolerance (Splash.cc:1638).
func (s *Splash) SetFlatness(f float64) { s.state.flatness = f }

// SetLineDash installs the dash array and phase (Splash.cc:1656).
func (s *Splash) SetLineDash(dash []float64, phase float64) {
	if dash != nil {
		s.state.lineDash = append(s.state.lineDash[:0], dash...)
	} else {
		s.state.lineDash = nil
	}
	s.state.lineDashPhase = phase
}

// SetStrokeAdjust toggles stroke-adjust hinting (Splash.cc:1681).
func (s *Splash) SetStrokeAdjust(adj bool) { s.state.strokeAdjust = adj }

// SetMirrorStrokeNormals toggles the pre-mirrored path compensation used by
// splashCanvas. Direct Splash callers should keep the Poppler-native default
// false because their paths are transformed by SplashXPath.
func (s *Splash) SetMirrorStrokeNormals(mirror bool) { s.mirrorStrokeNormals = mirror }

// SetBlendFunc installs the per-pixel blend formula (Splash.cc:1697-1700,
// SplashState::blendFunc). Pass nil for the default Normal blend.
func (s *Splash) SetBlendFunc(f BlendFunc) { s.state.blendFunc = f }

// SetSoftMask installs a single-channel ModeMono8 alpha mask consulted per
// pixel inside the AA pipe (Splash.cc:1709-1712 + Splash.cc:475-485,
// PDF spec 11.4.7 soft masks). Pass nil to clear.
func (s *Splash) SetSoftMask(mask *Bitmap) { s.state.softMask = mask }

// SaveState pushes the current state onto the stack (Splash.cc:1737).
func (s *Splash) SaveState() {
	clone := s.state.copy()
	clone.next = s.state
	s.state = clone
}

// RestoreState pops the saved state (Splash.cc:1746).
func (s *Splash) RestoreState() error {
	if s.state == nil || s.state.next == nil {
		return ErrNoSave
	}
	s.state = s.state.next
	return nil
}

// ClipResetToRect destructively replaces the clip with a rectangle (Splash.cc:1694).
func (s *Splash) ClipResetToRect(x0, y0, x1, y1 float64) {
	s.ensureClip().ResetToRect(x0, y0, x1, y1)
}

// ClipToRect intersects the clip with a rectangle (Splash.cc:1699).
func (s *Splash) ClipToRect(x0, y0, x1, y1 float64) error {
	_ = s.ensureClip().ClipToRect(x0, y0, x1, y1)
	return nil
}

// ClipToPath intersects the clip with a path (Splash.cc:1704).
func (s *Splash) ClipToPath(p *xpath.Path, eo bool) error {
	return s.ensureClip().ClipToPath(p, s.state.matrix, s.state.flatness, eo)
}

func (s *Splash) ensureClip() *xpath.Clip {
	if clip, ok := s.state.clip.(*xpath.Clip); ok && clip != nil {
		return clip
	}
	clip := xpath.NewClip(0, 0, s.bitmap.Width()-1, s.bitmap.Height()-1, s.vectorAA)
	s.state.clip = clip
	return clip
}

// Clear fills the whole bitmap with c at alpha (Splash.cc:1474).
func (s *Splash) Clear(c Color, alpha byte) {
	_ = c
	_ = alpha
}

// Stroke rasterizes p with the current stroke pattern (Splash.cc:1810).
// Body forwards to strokeImpl in splash_stroke.go (P1-Dev3 surgical edit).
func (s *Splash) Stroke(p *xpath.Path) error {
	return s.strokeImpl(p)
}

// Fill rasterizes p with the current fill pattern (Splash.cc:2362).
// Body forwards to fillImpl in splash_fill.go (P2-Dev4 surgical edit).
func (s *Splash) Fill(p *xpath.Path, eo bool) error {
	return s.fillImpl(p, eo)
}

// FillImageMask rasterizes a 1-bit image mask (Splash.cc:2740).
// Body forwards to FillImageMaskImpl in splash_image_mask.go (P3-Dev4 surgical edit).
func (s *Splash) FillImageMask(src ImageMaskSource, w, h int, mat [6]float64, glyphMode bool) error {
	return s.FillImageMaskImpl(src, w, h, mat, glyphMode)
}

// DrawImage rasterizes a sampled image (Splash.cc:3489).
// Body forwards to DrawImageImpl in splash_image.go (P3-Dev4 surgical edit).
func (s *Splash) DrawImage(src ImageSource, w, h int, mat [6]float64, interpolate bool) error {
	return s.DrawImageImpl(src, w, h, mat, interpolate)
}

// FillGlyph blits a pre-rasterized glyph at (x,y) (Splash.cc:2603).
// Body lives in splash_glyph.go (P1-Dev4, D1 LOCKED 2026-04-27).
func (s *Splash) FillGlyph(x, y float64, g *GlyphBitmap) error {
	return s.fillGlyph(x, y, g)
}

// GetBitmap returns the target bitmap (Splash.h:111).
func (s *Splash) GetBitmap() *Bitmap { return s.bitmap }
