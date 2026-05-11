package splash

import "github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"

// state mirrors SplashState (SplashState.h:53-129).
type state struct {
	matrix               [6]float64
	strokePattern        Pattern
	fillPattern          Pattern
	strokeAlpha          float64
	fillAlpha            float64
	patternStrokeAlpha   float64
	patternFillAlpha     float64
	multiplyPatternAlpha bool
	lineWidth            float64
	miterLimit           float64
	flatness             float64
	lineDashPhase        float64
	lineCap              int
	lineJoin             int
	lineDash             []float64
	strokeAdjust         bool
	clip                 any // TODO(phase2): retype to *xpath.Clip once xpath subpkg lands.
	next                 *state
	grayTransfer         [256]byte
	rgbTransferR         [256]byte
	rgbTransferG         [256]byte
	rgbTransferB         [256]byte
	cmykTransferC        [256]byte
	cmykTransferM        [256]byte
	cmykTransferY        [256]byte
	cmykTransferK        [256]byte
	deviceNTransfer      [splashMaxColorComps][256]byte
	// blendFunc mirrors SplashState::blendFunc (SplashState.h:117). Per-pixel
	// blend formula installed by SetBlendFunc; nil means default Normal blend.
	blendFunc BlendFunc
	// softMask mirrors SplashState::softMask (SplashState.h:121, Splash.cc:475-485).
	// Single-channel (ModeMono8) alpha bitmap multiplied into aSrc per pixel.
	softMask *Bitmap
}

// copy returns a shallow clone of s (SplashState.h:61).
func (s *state) copy() *state {
	if s == nil {
		return nil
	}
	c := *s
	c.next = nil
	if clip, ok := s.clip.(*xpath.Clip); ok && clip != nil {
		c.clip = clip.Clone()
	}
	if s.lineDash != nil {
		c.lineDash = append([]float64(nil), s.lineDash...)
	}
	return &c
}
