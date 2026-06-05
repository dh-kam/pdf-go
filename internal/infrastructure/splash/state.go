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
	softMask       *Bitmap
	softMaskClip   clipBoundsSnapshot
	softMaskClipOK bool
	// inNonIsolatedGroup mirrors SplashState::inNonIsolatedGroup. When true,
	// pipeRun folds the saved backdrop alpha into alphaI/alphaIm1.
	inNonIsolatedGroup bool
}

type clipBoundsSnapshot struct {
	vector      [4]float64
	vectorOK    bool
	effective   [4]float64
	effectiveOK bool
	offsetX     int
	offsetY     int
}

type softMaskStateSnapshot struct {
	mask   *Bitmap
	clip   clipBoundsSnapshot
	clipOK bool
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

func (s *state) resetTransfer() {
	if s == nil {
		return
	}
	for i := 0; i < 256; i++ {
		v := byte(i)
		s.rgbTransferR[i] = v
		s.rgbTransferG[i] = v
		s.rgbTransferB[i] = v
		s.grayTransfer[i] = v
		s.cmykTransferC[i] = v
		s.cmykTransferM[i] = v
		s.cmykTransferY[i] = v
		s.cmykTransferK[i] = v
		for cp := range s.deviceNTransfer {
			s.deviceNTransfer[cp][i] = v
		}
	}
}

func (s *state) setTransfer(red, green, blue, gray [256]byte) {
	if s == nil {
		return
	}
	// Mirrors SplashState::setTransfer, including the subtractive tables being
	// derived from the currently installed RGB/gray tables before replacement.
	for i := 0; i < 256; i++ {
		s.cmykTransferC[i] = 255 - s.rgbTransferR[255-i]
		s.cmykTransferM[i] = 255 - s.rgbTransferG[255-i]
		s.cmykTransferY[i] = 255 - s.rgbTransferB[255-i]
		s.cmykTransferK[i] = 255 - s.grayTransfer[255-i]
	}
	for i := 0; i < 256; i++ {
		s.deviceNTransfer[0][i] = 255 - s.rgbTransferR[255-i]
		s.deviceNTransfer[1][i] = 255 - s.rgbTransferG[255-i]
		s.deviceNTransfer[2][i] = 255 - s.rgbTransferB[255-i]
		s.deviceNTransfer[3][i] = 255 - s.grayTransfer[255-i]
	}
	s.rgbTransferR = red
	s.rgbTransferG = green
	s.rgbTransferB = blue
	s.grayTransfer = gray
}
