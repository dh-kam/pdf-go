package splash

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
)

// pipe mirrors SplashPipe (Splash.cc:145-186).
type pipe struct {
	pattern   Pattern
	aInput    byte
	usesShape bool
	cSrc      Color
	shape     byte
	patAlpha  byte
	knockout  bool

	x, y int
	// sampleXOff is a debug-only pattern sampling offset used while isolating
	// Poppler shaded-fill edge phase differences. It must stay zero by default.
	sampleXOff int

	// Direct row+offset addressing (Go does not let us hold a "moving pointer"
	// the way C++ SplashColorPtr does, so we hold the row slice + cursor).
	destRow  []byte
	destOff  int
	aDestRow []byte
	aDestOff int

	colorBytesPerPixel int

	noTransparency bool

	// blendFunc captures state.blendFunc at pipeInit time (Splash.cc:535-541).
	// nil means Normal blend (fast path, no extra work in pipe.run).
	blendFunc BlendFunc
	mode      ColorMode

	// softMask mirrors pipe->softMaskPtr indexing logic (Splash.cc:475-485,
	// 1208-1209). When non-nil the AA path multiplies aSrc by softMask byte
	// at the current device (x,y).
	softMask *Bitmap
	// alpha0 mirrors pipe->alpha0Ptr for Poppler non-isolated group painting.
	alpha0  *Bitmap
	alpha0X int
	alpha0Y int

	s   *Splash
	run func(p *pipe)
}

var imagePipePool = sync.Pool{
	New: func() any { return new(pipe) },
}

// pipeInit mirrors Splash::pipeInit (Splash.cc:212-293).
func (s *Splash) pipeInit(p *pipe, x, y int, pat Pattern, cSrc *Color, aInput byte, usesShape, nonIsoGroup bool) {
	p.s = s
	p.pattern = nil
	if pat != nil {
		if pat.IsStatic() {
			pat.GetColor(x, y, &p.cSrc)
		} else {
			p.pattern = pat
		}
	} else if cSrc != nil {
		p.cSrc = *cSrc
	}
	p.aInput = aInput
	p.usesShape = usesShape
	p.shape = 0
	p.sampleXOff = 0
	p.patAlpha = 255
	p.knockout = false
	_, hasPatternAlpha := pat.(AlphaPattern)
	p.colorBytesPerPixel = bytesPerPixel(s.bitmap.mode)
	// Capture blendFunc, mode, and softMask for per-pixel hooks
	// (Splash.cc:475-485 softMask, Splash.cc:535-541 blendFunc).
	p.blendFunc = s.state.blendFunc
	p.mode = s.bitmap.mode
	p.softMask = s.state.softMask
	p.alpha0 = nil
	p.alpha0X = 0
	p.alpha0Y = 0
	if s.state.inNonIsolatedGroup && s.nonIsoAlpha0 != nil && s.nonIsoAlpha0.alpha != nil {
		p.alpha0 = s.nonIsoAlpha0
		p.alpha0X = s.nonIsoAlpha0X
		p.alpha0Y = s.nonIsoAlpha0Y
	}
	p.noTransparency = aInput == 255 && !usesShape && !nonIsoGroup && !hasPatternAlpha && p.alpha0 == nil

	s.pipeSetXY(p, x, y)

	// Splash.cc:239,260 — Simple fast paths require !state->blendFunc and
	// !state->softMask.
	noTrans := p.noTransparency && p.blendFunc == nil && p.softMask == nil
	p.run = pickRun(s.bitmap.mode, noTrans, usesShape, p.pattern == nil, p.blendFunc == nil)
	if len(pipeTracePixels) > 0 || len(lastWriterPixels) > 0 {
		run := p.run
		p.run = func(pp *pipe) {
			x, y := pp.x, pp.y
			before := lastWriterSample{}
			if shouldTraceLastWriterPixel(x, y) {
				before = captureLastWriterSample(pp.s.bitmap, x, y)
			}
			if shouldTracePipePixel(x, y) {
				tracePipePixelBefore(pp, x, y)
			}
			run(pp)
			if shouldTracePipePixel(x, y) {
				tracePipePixelAfter(pp, x, y)
			}
			traceLastWriter("pipe", pp.s, pp.s.bitmap, x, y, before)
		}
	}
}

func pipeSourceAlpha(p *pipe) byte {
	aSrc := p.aInput
	if p.patAlpha != 255 {
		aSrc = byte(Div255(int(aSrc) * int(p.patAlpha)))
	}
	if p.softMask != nil {
		aSrc = byte(Div255(int(aSrc) * int(softMaskByte(p.softMask, p.x, p.y))))
	}
	if p.usesShape {
		aSrc = byte(Div255(int(aSrc) * int(p.shape)))
	}
	if shouldTracePipePixel(p.x, p.y) {
		fmt.Fprintf(os.Stderr, "SPLASH_PIPE_TRACE x=%d y=%d aInput=%d patAlpha=%d softMask=%t shape=%d usesShape=%t aSrc=%d cSrc=(%d,%d,%d)\n",
			p.x, p.y, p.aInput, p.patAlpha, p.softMask != nil, p.shape, p.usesShape, aSrc, p.cSrc[0], p.cSrc[1], p.cSrc[2])
	}
	return aSrc
}

func pipeResultAlphas(p *pipe, aSrc, aDest byte) (byte, int, int) {
	aResult := aSrc + aDest - byte(Div255(int(aSrc)*int(aDest)))
	alphaI := int(aResult)
	alphaIm1 := int(aDest)
	if alpha0, ok := pipeAlpha0(p); ok {
		alphaI = int(aResult) + int(alpha0) - Div255(int(aResult)*int(alpha0))
		alphaIm1 = int(alpha0) + int(aDest) - Div255(int(alpha0)*int(aDest))
	}
	return aResult, alphaI, alphaIm1
}

func pipeAlpha0(p *pipe) (byte, bool) {
	if p == nil || p.alpha0 == nil || p.alpha0.alpha == nil {
		return 0, false
	}
	x := p.alpha0X + p.x
	y := p.alpha0Y + p.y
	if x < 0 || y < 0 || x >= p.alpha0.width || y >= p.alpha0.height {
		return 0, false
	}
	return p.alpha0.alpha[y*p.alpha0.width+x], true
}

func pipeSetPatternAlpha(p *pipe) bool {
	p.patAlpha = 255
	if p.pattern == nil {
		return true
	}
	if alphaPattern, ok := p.pattern.(AlphaPattern); ok {
		p.patAlpha = alphaPattern.PatternAlpha(p.x, p.y)
		return p.patAlpha != 0
	}
	return true
}

func pipePatternX(p *pipe) int {
	if p == nil {
		return 0
	}
	return p.x + p.sampleXOff
}

// pickRun selects the per-mode dispatch function (Splash.cc:259-292).
//
// The Simple* and AA* variants both honor a dynamic pattern (p.pattern != nil)
// by calling pattern.GetColor(p.x, p.y, &src) per pixel — see Splash.cc:312-316.
// The noPat fast path (Splash.cc:259-280) caches one cSrcVal at pipeInit time;
// for dynamic patterns we keep the same dispatch but the variant body branches
// on p.pattern internally. Phase 3 — correctness over speed (Approach A).
func pickRun(m ColorMode, noTransparency, usesShape, noPat, noBlend bool) func(*pipe) {
	if noTransparency {
		if noPat {
			switch m {
			case ModeMono8:
				return pipeRunSimpleMono8NoPat
			case ModeRGB8:
				return pipeRunSimpleRGB8NoPat
			case ModeCMYK8:
				return pipeRunSimpleCMYK8NoPat
			case ModeDeviceN8:
				return pipeRunSimpleDeviceN8NoPat
			}
		}
		switch m {
		case ModeMono8:
			return pipeRunSimpleMono8
		case ModeRGB8:
			return pipeRunSimpleRGB8
		case ModeCMYK8:
			return pipeRunSimpleCMYK8
		case ModeDeviceN8:
			return pipeRunSimpleDeviceN8
		}
	}
	if !noTransparency && usesShape {
		if noPat && noBlend {
			switch m {
			case ModeMono8:
				return pipeRunAAMono8NoPat
			case ModeRGB8:
				return pipeRunAARGB8NoPat
			case ModeCMYK8:
				return pipeRunAACMYK8NoPat
			case ModeDeviceN8:
				return pipeRunAADeviceN8NoPat
			}
		}
		switch m {
		case ModeMono8:
			return pipeRunAAMono8
		case ModeRGB8:
			return pipeRunAARGB8
		case ModeCMYK8:
			return pipeRunAACMYK8
		case ModeDeviceN8:
			return pipeRunAADeviceN8
		}
	}
	// Fallback: AA path with shape
	if noPat && noBlend {
		switch m {
		case ModeMono8:
			return pipeRunAAMono8NoPat
		case ModeRGB8:
			return pipeRunAARGB8NoPat
		case ModeCMYK8:
			return pipeRunAACMYK8NoPat
		case ModeDeviceN8:
			return pipeRunAADeviceN8NoPat
		}
	}
	switch m {
	case ModeMono8:
		return pipeRunAAMono8
	case ModeRGB8:
		return pipeRunAARGB8
	case ModeCMYK8:
		return pipeRunAACMYK8
	case ModeDeviceN8:
		return pipeRunAADeviceN8
	}
	return func(*pipe) {}
}

// bytesPerPixel reports the color stride for a Splash mode (Splash.cc:1211-1232).
func bytesPerPixel(m ColorMode) int {
	switch m {
	case ModeMono8:
		return 1
	case ModeRGB8, ModeBGR8:
		return 3
	case ModeXBGR8, ModeCMYK8:
		return 4
	case ModeDeviceN8:
		return splashMaxColorComps
	}
	return 1
}

// pipeRun dispatches to the per-mode run function (Splash.cc:296).
func pipeRun(p *pipe, w int) {
	for i := 0; i < w; i++ {
		p.run(p)
	}
}

// pipeSetXY mirrors Splash::pipeSetXY (Splash.cc:1204-1243). Mono1 path skipped — D2.
func (s *Splash) pipeSetXY(p *pipe, x, y int) {
	p.x = x
	p.y = y
	bpp := p.colorBytesPerPixel
	rs := rowStride(s.bitmap, bpp)
	p.destRow = s.bitmap.data
	p.destOff = y*rs + x*bpp
	if s.bitmap.alpha != nil {
		p.aDestRow = s.bitmap.alpha
		p.aDestOff = y*s.bitmap.width + x
	} else {
		p.aDestRow = nil
		p.aDestOff = 0
	}
}

// pipeIncX advances the pipe cursor by one pixel (Splash.cc:1245-1281).
func (s *Splash) pipeIncX(p *pipe) {
	p.x++
	p.destOff += p.colorBytesPerPixel
	if p.aDestRow != nil {
		p.aDestOff++
	}
}

var pipeTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_PIPE_TRACE"))

func shouldTracePipePixel(x, y int) bool {
	for _, pixel := range pipeTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func tracePipePixelBefore(p *pipe, x, y int) {
	if p.colorBytesPerPixel < 3 || p.destOff+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil && p.aDestOff < len(p.aDestRow) {
		aDest = p.aDestRow[p.aDestOff]
	}
	src := p.cSrc
	patternHit := false
	if p.pattern != nil {
		patternHit = p.pattern.GetColor(pipePatternX(p), y, &src)
	}
	softMask := byte(0)
	hasSoftMask := p.softMask != nil
	if hasSoftMask {
		softMask = softMaskByte(p.softMask, x, y)
	}
	context := ""
	if p.s != nil && p.s.debugPaintContext != "" {
		context = fmt.Sprintf(" ctx=%q", p.s.debugPaintContext)
	}
	fmt.Fprintf(os.Stderr, "SPLASH_PIPE_TRACE before x=%d y=%d src=(%d,%d,%d) pattern=%t patternHit=%t aInput=%d usesShape=%t shape=%d softMask=%d hasSoftMask=%t dst=(%d,%d,%d) aDest=%d run=%p%s\n",
		x, y, src[0], src[1], src[2], p.pattern != nil, patternHit,
		p.aInput, p.usesShape, p.shape, softMask, hasSoftMask,
		p.destRow[p.destOff], p.destRow[p.destOff+1], p.destRow[p.destOff+2], aDest,
		p.run, context)
	if os.Getenv("PDF_DEBUG_SPLASH_PIPE_STACK") != "" && src[0] == 0 && src[1] == 0 && src[2] == 0 {
		fmt.Fprintf(os.Stderr, "SPLASH_PIPE_TRACE stack x=%d y=%d\n%s", x, y, debug.Stack())
	}
}

func tracePipePixelAfter(p *pipe, x, y int) {
	off := p.destOff - p.colorBytesPerPixel
	if p.colorBytesPerPixel < 3 || off < 0 || off+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil {
		aOff := p.aDestOff - 1
		if aOff >= 0 && aOff < len(p.aDestRow) {
			aDest = p.aDestRow[aOff]
		}
	}
	context := ""
	if p.s != nil && p.s.debugPaintContext != "" {
		context = fmt.Sprintf(" ctx=%q", p.s.debugPaintContext)
	}
	fmt.Fprintf(os.Stderr, "SPLASH_PIPE_TRACE after x=%d y=%d dst=(%d,%d,%d) aDest=%d%s\n",
		x, y, p.destRow[off], p.destRow[off+1], p.destRow[off+2], aDest, context)
}

// rowStride returns the byte stride of a row, falling back to width*bpp when
// rowSize is zero (the Phase-0 NewBitmap left rowSize unset).
func rowStride(b *Bitmap, bpp int) int {
	if b.rowSize > 0 {
		return b.rowSize
	}
	return b.width * bpp
}
