package splash

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync/atomic"
)

type lastWriterSample struct {
	color Color
	alpha byte
	ok    bool
}

var (
	lastWriterPixels     = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_LAST_WRITER"))
	textFillStrokePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_TEXT_FILL_STROKE_TRACE"))
	lastWriterSeq        uint64
)

func shouldTraceLastWriterPixel(x, y int) bool {
	for _, pixel := range lastWriterPixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func captureLastWriterSample(bm *Bitmap, x, y int) lastWriterSample {
	if bm == nil || bm.data == nil || x < 0 || y < 0 || x >= bm.width || y >= bm.height {
		return lastWriterSample{}
	}
	bpp := bytesPerPixel(bm.mode)
	if bpp <= 0 {
		return lastWriterSample{}
	}
	off := y*bm.rowSize + x*bpp
	if off < 0 || off+bpp > len(bm.data) {
		return lastWriterSample{}
	}
	sample := lastWriterSample{alpha: 0xff, ok: true}
	for i := 0; i < bpp && i < len(sample.color); i++ {
		sample.color[i] = bm.data[off+i]
	}
	if bm.alpha != nil {
		alphaOff := y*bm.width + x
		if alphaOff >= 0 && alphaOff < len(bm.alpha) {
			sample.alpha = bm.alpha[alphaOff]
		}
	}
	return sample
}

func traceLastWriter(label string, sp *Splash, bm *Bitmap, x, y int, before lastWriterSample) {
	if !shouldTraceLastWriterPixel(x, y) {
		return
	}
	after := captureLastWriterSample(bm, x, y)
	changed := before.ok != after.ok || before.alpha != after.alpha || before.color != after.color
	seq := atomic.AddUint64(&lastWriterSeq, 1)
	state := ""
	context := ""
	if sp != nil {
		softMask := false
		blend := false
		if sp.state != nil {
			softMask = sp.state.softMask != nil
			blend = sp.state.blendFunc != nil
		}
		state = fmt.Sprintf(" fillIndex=%d strokeIndex=%d groupDepth=%d softMask=%t blend=%t", sp.debugFillIndex, sp.debugStrokeIndex, len(sp.groupStack), softMask, blend)
		if sp.debugPaintContext != "" {
			context = fmt.Sprintf(" ctx=%q", sp.debugPaintContext)
		}
	}
	fmt.Fprintf(os.Stderr,
		"SPLASH_LAST_WRITER seq=%d op=%s mode=%v x=%d y=%d%s%s before=(%d,%d,%d,a=%d,ok=%t) after=(%d,%d,%d,a=%d,ok=%t) changed=%t\n",
		seq, label, bitmapModeForTrace(bm), x, y,
		state, context,
		before.color[0], before.color[1], before.color[2], before.alpha, before.ok,
		after.color[0], after.color[1], after.color[2], after.alpha, after.ok,
		changed,
	)
	if os.Getenv("PDF_DEBUG_SPLASH_LAST_WRITER_STACK") != "" {
		fmt.Fprintf(os.Stderr, "SPLASH_LAST_WRITER stack seq=%d x=%d y=%d\n%s", seq, x, y, debug.Stack())
	}
}

func traceTextFillStrokePixels(stage string, sp *Splash) {
	if sp == nil || sp.bitmap == nil || len(textFillStrokePixels) == 0 {
		return
	}
	state := ""
	context := ""
	if sp.state != nil {
		state = fmt.Sprintf(
			" fillIndex=%d strokeIndex=%d lineWidth=%.9g lineCap=%d lineJoin=%d miter=%.9g fillAlpha=%.9g strokeAlpha=%.9g groupDepth=%d softMask=%t blend=%t",
			sp.debugFillIndex, sp.debugStrokeIndex,
			sp.state.lineWidth, sp.state.lineCap, sp.state.lineJoin, sp.state.miterLimit, sp.state.fillAlpha, sp.state.strokeAlpha,
			len(sp.groupStack), sp.state.softMask != nil, sp.state.blendFunc != nil,
		)
	} else {
		state = fmt.Sprintf(" fillIndex=%d strokeIndex=%d groupDepth=%d", sp.debugFillIndex, sp.debugStrokeIndex, len(sp.groupStack))
	}
	if sp.debugPaintContext != "" {
		context = fmt.Sprintf(" ctx=%q", sp.debugPaintContext)
	}
	for _, pixel := range textFillStrokePixels {
		sample := captureLastWriterSample(sp.bitmap, pixel.x, pixel.y)
		fmt.Fprintf(os.Stderr,
			"SPLASH_TEXT_FILL_STROKE stage=%s mode=%v x=%d y=%d%s%s sample=(%d,%d,%d,a=%d,ok=%t)\n",
			stage, bitmapModeForTrace(sp.bitmap), pixel.x, pixel.y,
			state, context,
			sample.color[0], sample.color[1], sample.color[2], sample.alpha, sample.ok,
		)
	}
}

func bitmapModeForTrace(bm *Bitmap) ColorMode {
	if bm == nil {
		return ModeRGB8
	}
	return bm.mode
}
