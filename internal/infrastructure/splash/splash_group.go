package splash

import (
	"fmt"
	"math"
	"os"
	"strings"

	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// groupState saves a parent's render target while drawing into a child
// transparency-group bitmap (Splash.cc:5021-5254 + PDF spec 11.4.7).
type groupState struct {
	bbox                      [4]float64
	compositeBounds           [4]int
	cropped                   bool
	tx                        int
	ty                        int
	blendMode                 BlendFunc
	isolated                  bool
	knockout                  bool
	groupBitmap               *Bitmap
	savedBitmap               *Bitmap
	savedAaBuf                []byte
	savedClip                 any
	groupClip                 any
	savedAlpha0               *Bitmap
	savedAlpha0X              int
	savedAlpha0Y              int
	savedNonIsolated          bool
	savedStrokeAdjust         bool
	savedStrokeAlpha          float64
	savedFillAlpha            float64
	savedBlendFunc            BlendFunc
	// savedSoftMask captures the parent's soft mask at BeginTransparencyGroup
	// time. Poppler renders each group in a fresh child Splash (whose state has
	// no soft mask) and composites via Splash::composite, whose pipe applies
	// state->softMask (Splash.cc:475). Gfx::drawForm restores the parent
	// GfxState before paintTransparencyGroup, so the soft mask active at the Do
	// is the one applied to the group composite. We share one Splash, so an
	// inner /gs with /SMask /None (or the form reset) clobbers s.state.softMask
	// during the group; without restoring it, PaintTransparencyGroup composites
	// with a nil mask. Saved at begin, restored in restoreTransparencyGroupState.
	savedSoftMask             softMaskStateSnapshot
	savedPatternStrokeAlpha   float64
	savedPatternFillAlpha     float64
	savedMultiplyPatternAlpha bool
	freshPatternAlphaApplied  bool
	forSoftMask               bool
}

// BeginTransparencyGroup pushes a fresh sub-bitmap onto the group stack and
// redirects subsequent rendering into it (Splash.cc:5021 begin path,
// PDF spec 11.4.7). bbox is in device coordinates (x0,y0,x1,y1); when zero,
// the parent bitmap's full extent is used. blendMode is applied at the
// matching PaintTransparencyGroup; nil means Normal.
func (s *Splash) BeginTransparencyGroup(bbox [4]float64, isolated, knockout bool, blendMode BlendFunc) error {
	if s == nil || s.bitmap == nil {
		return ErrBadArg
	}
	freshPatternAlpha := s.consumeFreshPatternAlphaForGroup()
	s.traceFreshPatternAlphaGroup("begin", freshPatternAlpha, bbox)
	s.traceGroupBegin("begin", bbox, [4]int{}, 0, 0, isolated, knockout, blendMode)
	parent := s.bitmap
	w := parent.Width()
	h := parent.Height()
	// Allocate a same-size sub-bitmap. Phase-4 simple model: groups always
	// cover the parent canvas (bbox kept for parity / future cropping).
	gb := NewBitmap(w, h, parent.mode, true)
	useAlpha0 := !isolated && usePopplerNonIsolatedAlpha0()
	if isolated {
		// PDF spec 11.4.7.5: isolated groups start with transparent backdrop.
		// gb.data and gb.alpha already zero from make, which is transparent
		// black — exactly what we need.
	} else if useAlpha0 {
		// Poppler Splash::blitTransparent copies backdrop color but clears the
		// group shape, then setInNonIsolatedGroup keeps the original backdrop
		// alpha available to pipeRun via alpha0Ptr.
		copy(gb.data, parent.data)
	} else {
		// Non-isolated: copy parent backdrop in (color + alpha).
		copy(gb.data, parent.data)
		if gb.alpha != nil && parent.alpha != nil {
			copy(gb.alpha, parent.alpha)
		}
	}
	gs := &groupState{
		bbox:                      bbox,
		compositeBounds:           transparencyGroupCompositeBounds(bbox, w, h),
		blendMode:                 blendMode,
		isolated:                  isolated,
		knockout:                  knockout,
		groupBitmap:               gb,
		savedBitmap:               parent,
		savedAaBuf:                s.aaBuf,
		savedClip:                 s.state.clip,
		savedAlpha0:               s.nonIsoAlpha0,
		savedAlpha0X:              s.nonIsoAlpha0X,
		savedAlpha0Y:              s.nonIsoAlpha0Y,
		savedNonIsolated:          s.state.inNonIsolatedGroup,
		savedStrokeAdjust:         s.state.strokeAdjust,
		savedStrokeAlpha:          s.state.strokeAlpha,
		savedFillAlpha:            s.state.fillAlpha,
		savedPatternStrokeAlpha:   s.state.patternStrokeAlpha,
		savedPatternFillAlpha:     s.state.patternFillAlpha,
		savedMultiplyPatternAlpha: s.state.multiplyPatternAlpha,
		savedBlendFunc:            s.state.blendFunc,
		savedSoftMask:             s.captureSoftMaskState(),
	}
	s.groupStack = append(s.groupStack, gs)
	s.bitmap = gb
	gs.freshPatternAlphaApplied = s.applyFreshGroupPatternAlpha(freshPatternAlpha)
	if useAlpha0 {
		s.nonIsoAlpha0 = parent
		s.nonIsoAlpha0X = 0
		s.nonIsoAlpha0Y = 0
		s.state.inNonIsolatedGroup = true
	}
	s.resetNestedIsolatedGroupStateForSoftMask(isolated)
	if s.vectorAA && gb.Width() > 0 {
		s.aaBuf = make([]byte, splashAASize*gb.Width())
	}
	// Poppler creates a cropped bitmap for the transformed group bbox, then
	// shifts the CTM/clip into that child bitmap. This full-page group model
	// keeps page-device coordinates, so use the same integer crop rectangle as
	// the child clip instead of inheriting the parent's fractional BBox clip.
	cb := gs.compositeBounds
	groupClip := xpath.NewClip(cb[0], cb[1], cb[2]-1, cb[3]-1, s.vectorAA)
	gs.groupClip = groupClip
	s.state.clip = groupClip
	s.traceGroupClipInstalled("begin", groupClip, cb, 0, 0)
	// SplashState defaults strokeAdjust to false and SplashOutputDev does not
	// copy the page-level setting into the group renderer.
	s.state.strokeAdjust = false
	return nil
}

// BeginCroppedTransparencyGroup starts a Poppler-style cropped transparency
// group. Poppler allocates the temporary bitmap to the transformed bbox and
// shifts the CTM/clip by (-tx,-ty) while replaying the group contents.
func (s *Splash) BeginCroppedTransparencyGroup(bbox [4]float64, isolated, knockout bool, blendMode BlendFunc) (int, int, error) {
	if s == nil || s.bitmap == nil {
		return 0, 0, ErrBadArg
	}
	freshPatternAlpha := s.consumeFreshPatternAlphaForGroup()
	s.traceFreshPatternAlphaGroup("begin-cropped", freshPatternAlpha, bbox)
	s.traceGroupBegin("begin-cropped", bbox, [4]int{}, 0, 0, isolated, knockout, blendMode)
	parent := s.bitmap
	parentW := parent.Width()
	parentH := parent.Height()
	bounds := transparencyGroupCompositeBounds(bbox, parentW, parentH)
	tx := bounds[0]
	ty := bounds[1]
	w := bounds[2] - bounds[0]
	h := bounds[3] - bounds[1]
	s.traceGroupBegin("begin-cropped-bounds", bbox, bounds, tx, ty, isolated, knockout, blendMode)
	if w <= 0 || h <= 0 {
		return 0, 0, ErrBadArg
	}
	gb := NewBitmap(w, h, parent.mode, true)
	useAlpha0 := !isolated && usePopplerNonIsolatedAlpha0()
	if !isolated {
		copyBitmapRect(parent, gb, tx, ty, 0, 0, w, h, !useAlpha0)
	}

	gs := &groupState{
		bbox:                      bbox,
		compositeBounds:           [4]int{0, 0, w, h},
		cropped:                   true,
		tx:                        tx,
		ty:                        ty,
		blendMode:                 blendMode,
		isolated:                  isolated,
		knockout:                  knockout,
		groupBitmap:               gb,
		savedBitmap:               parent,
		savedAaBuf:                s.aaBuf,
		savedClip:                 s.state.clip,
		savedAlpha0:               s.nonIsoAlpha0,
		savedAlpha0X:              s.nonIsoAlpha0X,
		savedAlpha0Y:              s.nonIsoAlpha0Y,
		savedNonIsolated:          s.state.inNonIsolatedGroup,
		savedStrokeAdjust:         s.state.strokeAdjust,
		savedStrokeAlpha:          s.state.strokeAlpha,
		savedFillAlpha:            s.state.fillAlpha,
		savedPatternStrokeAlpha:   s.state.patternStrokeAlpha,
		savedPatternFillAlpha:     s.state.patternFillAlpha,
		savedMultiplyPatternAlpha: s.state.multiplyPatternAlpha,
		savedBlendFunc:            s.state.blendFunc,
		savedSoftMask:             s.captureSoftMaskState(),
	}
	s.groupStack = append(s.groupStack, gs)
	s.bitmap = gb
	gs.freshPatternAlphaApplied = s.applyFreshGroupPatternAlpha(freshPatternAlpha)
	if useAlpha0 {
		s.nonIsoAlpha0 = parent
		s.nonIsoAlpha0X = tx
		s.nonIsoAlpha0Y = ty
		s.state.inNonIsolatedGroup = true
	}
	s.resetNestedIsolatedGroupStateForSoftMask(isolated)
	if s.vectorAA && gb.Width() > 0 {
		s.aaBuf = make([]byte, splashAASize*gb.Width())
	}
	groupClip := xpath.NewClip(0, 0, w-1, h-1, s.vectorAA)
	gs.groupClip = groupClip
	s.state.clip = groupClip
	s.traceGroupClipInstalled("begin-cropped", groupClip, [4]int{0, 0, w, h}, tx, ty)
	s.state.strokeAdjust = false
	return tx, ty, nil
}

// PaintTransparencyGroup composites the topmost group bitmap back onto its
// parent under the saved blend mode and the current state's softMask
// (Splash.cc:5076 paint path + Splash.cc:639-648 alpha-blend, PDF spec 11.4.7).
// The parent bitmap is restored as the current target.
func (s *Splash) PaintTransparencyGroup() error {
	if len(s.groupStack) == 0 {
		return ErrNoSave
	}
	top := s.groupStack[len(s.groupStack)-1]
	s.groupStack = s.groupStack[:len(s.groupStack)-1]

	src := top.groupBitmap
	dst := top.savedBitmap

	// Restore parent as current target before compositing so any per-pixel
	// helper that reads s.bitmap sees the correct (parent) target.
	parentClip := s.state.clip
	if parentClip == top.groupClip {
		parentClip = top.savedClip
	}
	s.restoreTransparencyGroupState(top, parentClip)

	// Poppler restores the parent Splash state before compositing a Form
	// transparency group, so the parent blend mode active at paint time wins over
	// the Normal blend used while rendering inside the group.
	blendMode := s.state.blendFunc
	if blendMode == nil {
		blendMode = top.blendMode
	}
	clip, _ := s.state.clip.(*xpath.Clip)
	var alpha0 *Bitmap
	alpha0X, alpha0Y := 0, 0
	if useGroupAlpha0CompositeDebug() && s.state.inNonIsolatedGroup && s.nonIsoAlpha0 != nil && s.nonIsoAlpha0.alpha != nil {
		alpha0 = s.nonIsoAlpha0
		alpha0X = s.nonIsoAlpha0X
		alpha0Y = s.nonIsoAlpha0Y
	}
	if top.cropped {
		compositeGroupRectAt(s, src, dst, blendMode, s.state.softMask, s.state.fillAlpha, !top.isolated, top.compositeBounds, clip, top.tx, top.ty, alpha0, alpha0X, alpha0Y)
		if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
			fmt.Fprintf(os.Stderr, "GROUPPOP cropped=%t isolated=%t compositeAlpha=%.4f blendSet=%t remainingDepth=%d\n",
				top.cropped, top.isolated, s.state.fillAlpha, blendMode != nil, len(s.groupStack))
		}
		return nil
	}
	compositeGroupRectAt(s, src, dst, blendMode, s.state.softMask, s.state.fillAlpha, !top.isolated, top.compositeBounds, clip, 0, 0, alpha0, alpha0X, alpha0Y)
	if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
		fmt.Fprintf(os.Stderr, "GROUPPOP cropped=%t isolated=%t compositeAlpha=%.4f blendSet=%t remainingDepth=%d\n",
			top.cropped, top.isolated, s.state.fillAlpha, blendMode != nil, len(s.groupStack))
	}
	return nil
}

func transparencyGroupCompositeBounds(bbox [4]float64, width, height int) [4]int {
	if width <= 0 || height <= 0 || bbox == ([4]float64{}) {
		return [4]int{0, 0, maxInt(width, 0), maxInt(height, 0)}
	}

	xMin := math.Min(bbox[0], bbox[2])
	xMax := math.Max(bbox[0], bbox[2])
	yMin := math.Min(bbox[1], bbox[3])
	yMax := math.Max(bbox[1], bbox[3])

	x0 := int(math.Floor(xMin))
	if x0 < 0 {
		x0 = 0
	} else if x0 >= width {
		x0 = width - 1
	}
	y0 := int(math.Floor(yMin))
	if y0 < 0 {
		y0 = 0
	} else if y0 >= height {
		y0 = height - 1
	}

	x1 := int(math.Ceil(xMax)) + 1
	if x1 > width {
		x1 = width
	}
	if x1 <= x0 {
		x1 = x0 + 1
	}
	y1 := int(math.Ceil(yMax)) + 1
	if y1 > height {
		y1 = height
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}

	return [4]int{x0, y0, x1, y1}
}

// EndTransparencyGroupAsSoftMask converts the topmost transparency group into
// a page-sized soft-mask bitmap and restores the parent render target. This
// mirrors SplashOutputDev::setSoftMask after Gfx::doSoftMask renders the mask
// Form XObject into a transparency group.
func (s *Splash) EndTransparencyGroupAsSoftMask(alpha bool) (*Bitmap, error) {
	return s.EndTransparencyGroupAsSoftMaskWithOptions(domainrenderer.SoftMaskOptions{Alpha: alpha})
}

// EndTransparencyGroupAsSoftMaskWithOptions converts the topmost transparency
// group into a page-sized soft mask using Poppler's backdrop and transfer rules.
func (s *Splash) EndTransparencyGroupAsSoftMaskWithOptions(options domainrenderer.SoftMaskOptions) (*Bitmap, error) {
	if len(s.groupStack) == 0 {
		return nil, ErrNoSave
	}
	top := s.groupStack[len(s.groupStack)-1]
	s.groupStack = s.groupStack[:len(s.groupStack)-1]

	src := top.groupBitmap
	dst := top.savedBitmap
	s.restoreTransparencyGroupState(top, top.savedClip)
	if src == nil || dst == nil {
		return nil, ErrBadArg
	}

	mask := NewBitmap(dst.Width(), dst.Height(), ModeMono8, false)
	if mask == nil {
		return nil, ErrBadArg
	}
	srcData := src.Data()
	maskData := mask.Data()
	srcAlpha := src.Alpha()
	mode := src.Mode()
	bpp := bytesPerPixel(mode)
	if useSoftMaskCompositeBackground() && !options.Alpha && options.HasBackdrop {
		compositeSoftMaskBackground(src, options)
		srcAlpha = src.Alpha()
	}
	tracePixels := softMaskTracePixels()
	if len(tracePixels) > 0 {
		traceSoftMaskHeader("begin", top, options, src, mask)
		traceSoftMaskInitialPixels(tracePixels, top, options, mask)
	}
	if options.HasBackdrop {
		for i := range maskData {
			maskData[i] = options.BackdropLum
		}
	}
	if top.cropped {
		for y := 0; y < src.Height(); y++ {
			dstY := top.ty + y
			if dstY < 0 || dstY >= mask.Height() {
				continue
			}
			srcOff := y * src.RowSize()
			maskOff := dstY * mask.RowSize()
			alphaOff := y * src.Width()
			for x := 0; x < src.Width(); x++ {
				dstX := top.tx + x
				if dstX < 0 || dstX >= mask.Width() {
					continue
				}
				var value byte
				alphaValue := byte(255)
				if options.Alpha {
					if srcAlpha != nil {
						value = srcAlpha[alphaOff+x]
						alphaValue = value
					} else {
						value = 255
					}
				} else {
					if srcAlpha != nil {
						alphaValue = srcAlpha[alphaOff+x]
					}
					value = groupLuminosityByteWithBackdrop(mode, srcData[srcOff+x*bpp:], alphaValue, options)
				}
				if options.TransferActive {
					value = options.Transfer[value]
				}
				maskData[maskOff+dstX] = value
				if shouldTraceSoftMaskPixel(tracePixels, dstX, dstY) {
					traceSoftMaskPixel("write", dstX, dstY, x, y, mode, srcData[srcOff+x*bpp:], alphaValue, value, options)
				}
			}
		}
		traceSoftMaskFinalPixels(tracePixels, mask)
		return mask, nil
	}
	x0, y0, x1, y1 := top.compositeBounds[0], top.compositeBounds[1], top.compositeBounds[2], top.compositeBounds[3]
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > src.Width() {
		x1 = src.Width()
	}
	if x1 > mask.Width() {
		x1 = mask.Width()
	}
	if y1 > src.Height() {
		y1 = src.Height()
	}
	if y1 > mask.Height() {
		y1 = mask.Height()
	}
	for y := y0; y < y1; y++ {
		srcOff := y * src.RowSize()
		maskOff := y * mask.RowSize()
		alphaOff := y * src.Width()
		for x := x0; x < x1; x++ {
			var value byte
			alphaValue := byte(255)
			if options.Alpha {
				if srcAlpha != nil {
					value = srcAlpha[alphaOff+x]
					alphaValue = value
				} else {
					value = 255
				}
			} else {
				if srcAlpha != nil {
					alphaValue = srcAlpha[alphaOff+x]
				}
				value = groupLuminosityByteWithBackdrop(mode, srcData[srcOff+x*bpp:], alphaValue, options)
			}
			if options.TransferActive {
				value = options.Transfer[value]
			}
			maskData[maskOff+x] = value
			if shouldTraceSoftMaskPixel(tracePixels, x, y) {
				traceSoftMaskPixel("write", x, y, x, y, mode, srcData[srcOff+x*bpp:], alphaValue, value, options)
			}
		}
	}
	traceSoftMaskFinalPixels(tracePixels, mask)
	return mask, nil
}

// EndTransparencyGroup discards the topmost group state without compositing
// (Splash.cc:5230 end path). Safe no-op when the group was already painted —
// PaintTransparencyGroup pops on its own. Returned for API completeness so
// callers can pair Begin/End deterministically; the spec lets the same
// implementation drop pending groups when a save/restore unwind happens.
func (s *Splash) EndTransparencyGroup() error {
	if len(s.groupStack) == 0 {
		return nil
	}
	top := s.groupStack[len(s.groupStack)-1]
	s.groupStack = s.groupStack[:len(s.groupStack)-1]
	s.restoreTransparencyGroupState(top, top.savedClip)
	return nil
}

func (s *Splash) consumeFreshPatternAlphaForGroup() bool {
	force := s.freshPatternAlphaForNextGroup
	s.freshPatternAlphaForNextGroup = false
	depth := len(s.groupStack)
	return force ||
		useFreshPatternAlphaInGroups() ||
		(depth > 0 && useFreshPatternAlphaInNestedGroups()) ||
		(depth > 1 && useFreshPatternAlphaInDeepNestedGroups())
}

func (s *Splash) applyFreshGroupPatternAlpha(enabled bool) bool {
	if s == nil || s.state == nil || !enabled || !s.state.multiplyPatternAlpha {
		return false
	}
	// Poppler creates a fresh Splash for transparency groups and only copies
	// fill/stroke patterns, not the transient pattern-opacity multiplier.
	s.state.strokeAlpha = 1
	s.state.fillAlpha = 1
	s.state.patternStrokeAlpha = 1
	s.state.patternFillAlpha = 1
	s.state.multiplyPatternAlpha = false
	return true
}

func (s *Splash) traceFreshPatternAlphaGroup(kind string, enabled bool, bbox [4]float64) {
	if os.Getenv("PDF_DEBUG_SPLASH_GROUP_ALPHA_TRACE") == "" || s == nil || s.state == nil {
		return
	}
	fmt.Fprintf(os.Stderr,
		"SPLASH_GROUP_ALPHA_TRACE kind=%s depth=%d enabled=%t multiply=%t fillAlpha=%.6f strokeAlpha=%.6f patternFillAlpha=%.6f patternStrokeAlpha=%.6f bbox=[%.3f %.3f %.3f %.3f]\n",
		kind, len(s.groupStack), enabled, s.state.multiplyPatternAlpha, s.state.fillAlpha, s.state.strokeAlpha,
		s.state.patternFillAlpha, s.state.patternStrokeAlpha, bbox[0], bbox[1], bbox[2], bbox[3])
}

func (s *Splash) markTopGroupForSoftMask() {
	if s == nil || len(s.groupStack) == 0 {
		return
	}
	s.groupStack[len(s.groupStack)-1].forSoftMask = true
	if os.Getenv("PDF_DEBUG_SPLASH_GROUP_BEGIN_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "SPLASH_GROUP_BEGIN_TRACE layer=splash kind=mark-softmask depth=%d\n", len(s.groupStack))
	}
}

func (s *Splash) traceGroupBegin(kind string, bbox [4]float64, bounds [4]int, tx, ty int, isolated, knockout bool, blendMode BlendFunc) {
	if os.Getenv("PDF_DEBUG_SPLASH_GROUP_BEGIN_TRACE") == "" || s == nil || s.state == nil {
		return
	}
	clipDesc := splashClipTraceDescription(s.state.clip)
	fmt.Fprintf(os.Stderr,
		"SPLASH_GROUP_BEGIN_TRACE layer=splash kind=%s depth=%d isolated=%t knockout=%t hasBlend=%t bbox=[%.12f %.12f %.12f %.12f] bounds=[%d %d %d %d] tx=%d ty=%d parentClip=%s bitmap=%dx%d\n",
		kind, len(s.groupStack), isolated, knockout, blendMode != nil,
		bbox[0], bbox[1], bbox[2], bbox[3], bounds[0], bounds[1], bounds[2], bounds[3], tx, ty,
		clipDesc, s.bitmap.Width(), s.bitmap.Height())
}

func (s *Splash) traceGroupClipInstalled(kind string, clip any, bounds [4]int, tx, ty int) {
	if os.Getenv("PDF_DEBUG_SPLASH_GROUP_BEGIN_TRACE") == "" || s == nil || s.bitmap == nil {
		return
	}
	fmt.Fprintf(os.Stderr,
		"SPLASH_GROUP_BEGIN_TRACE layer=splash kind=%s-installed depth=%d bounds=[%d %d %d %d] tx=%d ty=%d groupClip=%s bitmap=%dx%d\n",
		kind, len(s.groupStack), bounds[0], bounds[1], bounds[2], bounds[3], tx, ty,
		splashClipTraceDescription(clip), s.bitmap.Width(), s.bitmap.Height())
}

func splashClipTraceDescription(clip any) string {
	c, ok := clip.(*xpath.Clip)
	if !ok || c == nil {
		return "none"
	}
	vx0, vy0, vx1, vy1, vok := c.VectorEffectiveBounds()
	ex0, ey0, ex1, ey1, eok := c.EffectiveBounds()
	return fmt.Sprintf("vectorOK=%t vector=[%.12f %.12f %.12f %.12f] effectiveOK=%t effective=[%.12f %.12f %.12f %.12f]",
		vok, vx0, vy0, vx1, vy1, eok, ex0, ey0, ex1, ey1)
}

func (s *Splash) resetNestedIsolatedGroupStateForSoftMask(isolated bool) {
	if s == nil || s.state == nil || !isolated || !useSoftMaskNestedIsolatedFreshState() || len(s.groupStack) < 2 {
		return
	}
	parent := s.groupStack[len(s.groupStack)-2]
	if parent == nil || !parent.forSoftMask {
		return
	}
	// Poppler allocates a fresh Splash for the child group, so the soft-mask
	// group's non-isolated alpha0 state is not inherited by isolated children.
	s.nonIsoAlpha0 = nil
	s.nonIsoAlpha0X = 0
	s.nonIsoAlpha0Y = 0
	s.state.inNonIsolatedGroup = false
}

func (s *Splash) restoreTransparencyGroupState(top *groupState, clip any) {
	s.bitmap = top.savedBitmap
	s.aaBuf = top.savedAaBuf
	s.state.strokeAdjust = top.savedStrokeAdjust
	s.state.clip = clip
	s.nonIsoAlpha0 = top.savedAlpha0
	s.nonIsoAlpha0X = top.savedAlpha0X
	s.nonIsoAlpha0Y = top.savedAlpha0Y
	s.state.inNonIsolatedGroup = top.savedNonIsolated
	// Restore the parent's alpha state unconditionally. Poppler renders each
	// transparency group in a fresh child Splash, so the parent's fill/stroke/
	// pattern alpha is never modified by the group. This backend shares a single
	// Splash, so both the fresh-pattern-alpha reset (applyFreshGroupPatternAlpha)
	// AND the group's own content (e.g. an inner /gs setting /ca) mutate
	// s.state during the group. Restoring only when a pattern-alpha reset was
	// applied lets an inner /ca leak past the group: the leftover alpha is then
	// used by this group's own composite (PaintTransparencyGroup reads
	// s.state.fillAlpha) and by subsequent painting. The saved values were
	// captured at BeginTransparencyGroup from the parent state, so restoring them
	// reproduces Poppler's "parent splash untouched" semantics for both the
	// pattern and non-pattern cases.
	s.state.strokeAlpha = top.savedStrokeAlpha
	s.state.fillAlpha = top.savedFillAlpha
	s.state.patternStrokeAlpha = top.savedPatternStrokeAlpha
	s.state.patternFillAlpha = top.savedPatternFillAlpha
	s.state.multiplyPatternAlpha = top.savedMultiplyPatternAlpha
	// Restore the parent's blend mode too. Poppler renders each group in a fresh
	// child Splash with blend=Normal, then composites onto the parent under the
	// parent's blend active at paint time. We share one Splash, so an inner /gs
	// (or the group reset itself) mutates s.state.blendFunc; restore it so
	// PaintTransparencyGroup reads the true parent blend.
	s.state.blendFunc = top.savedBlendFunc
	// Restore the parent's soft mask. Poppler's Gfx::drawForm restores the parent
	// GfxState before paintTransparencyGroup, and Splash::composite applies
	// state->softMask through its pipe (Splash.cc:475), so the soft mask active at
	// the Do is applied to the group composite. We share one Splash, so an inner
	// /gs with /SMask /None clobbers s.state.softMask during the group; restore
	// the value captured at begin so PaintTransparencyGroup composites under the
	// parent's mask. Without this, masked form content renders fully opaque.
	s.restoreSoftMaskState(top.savedSoftMask)
}

func usePopplerNonIsolatedAlpha0() bool {
	v := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_NON_ISOLATED_ALPHA0"))
	return v == "" || (v != "0" && !strings.EqualFold(v, "false"))
}

func useFreshPatternAlphaInGroups() bool {
	v := strings.TrimSpace(os.Getenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func useFreshPatternAlphaInFormGroups() bool {
	v := strings.TrimSpace(os.Getenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA_FORM"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func useFreshPatternAlphaInNestedGroups() bool {
	v := strings.TrimSpace(os.Getenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA_NESTED"))
	return v == "" || (v != "0" && !strings.EqualFold(v, "false"))
}

func useFreshPatternAlphaInDeepNestedGroups() bool {
	v := strings.TrimSpace(os.Getenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA_DEEP_NESTED"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func useFreshPatternAlphaInSoftMaskGroups() bool {
	v := strings.TrimSpace(os.Getenv("PDF_SPLASH_GROUP_FRESH_PATTERN_ALPHA_SOFTMASK"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func useSoftMaskNestedIsolatedFreshState() bool {
	v := strings.TrimSpace(os.Getenv("GO_PDF_SMASK_NESTED_ISOLATED_FRESH_STATE"))
	return v == "" || (v != "0" && !strings.EqualFold(v, "false"))
}

func useGroupAlpha0CompositeDebug() bool {
	v := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GROUP_ALPHA0_COMPOSITE"))
	return v == "" || (v != "0" && !strings.EqualFold(v, "false"))
}

func useGroupPopplerSoftMaskAlphaOrder() bool {
	v := strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_GROUP_POPPLER_SMASK_ALPHA_ORDER"))
	return v == "" || (v != "0" && !strings.EqualFold(v, "false"))
}

// compositeGroup blits src onto dst per PDF spec 11.4.5 (Compositing). For
// each pixel: aSrc = src.alpha * mask / 255; aResult = aSrc + aDst - aSrc*aDst/255;
// cBlend = blendMode(cSrc, cDst); cResult = ((aResult-aSrc)*cDst + aSrc*((255-aDst)*cSrc + aDst*cBlend)/255) / aResult.
// When blendMode is nil this collapses to Normal (cBlend == cSrc).
func compositeGroup(src, dst *Bitmap, blendMode BlendFunc, softMask *Bitmap, alpha float64) {
	if dst == nil {
		return
	}
	compositeGroupRect(src, dst, blendMode, softMask, alpha, false, [4]int{0, 0, dst.width, dst.height}, nil)
}

func compositeGroupRect(src, dst *Bitmap, blendMode BlendFunc, softMask *Bitmap, alpha float64, nonIsolated bool, bounds [4]int, clip *xpath.Clip) {
	compositeGroupRectAt(nil, src, dst, blendMode, softMask, alpha, nonIsolated, bounds, clip, 0, 0, nil, 0, 0)
}

func compositeGroupRectAt(sp *Splash, src, dst *Bitmap, blendMode BlendFunc, softMask *Bitmap, alpha float64, nonIsolated bool, bounds [4]int, clip *xpath.Clip, dstX, dstY int, alpha0 *Bitmap, alpha0X, alpha0Y int) {
	if src == nil || dst == nil || src.mode != dst.mode {
		return
	}
	mode := dst.mode
	bpp := bytesPerPixel(mode)
	a8 := byte(255)
	if alpha < 1 {
		if alpha < 0 {
			alpha = 0
		}
		a8 = byte(Round(alpha * 255.0))
	}
	x0, y0, x1, y1 := bounds[0], bounds[1], bounds[2], bounds[3]
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > src.width {
		x1 = src.width
	}
	if y1 > src.height {
		y1 = src.height
	}
	if x0 >= x1 || y0 >= y1 {
		return
	}
	tracePixels := groupCompositeTracePixels()
	for y := y0; y < y1; y++ {
		dy := dstY + y
		if dy < 0 || dy >= dst.height {
			continue
		}
		dRowOff := dy * dst.rowSize
		sRowOff := y * src.rowSize
		for x := x0; x < x1; x++ {
			dx := dstX + x
			if dx < 0 || dx >= dst.width {
				continue
			}
			if clip != nil && !clip.Test(dx, dy) {
				if shouldTraceGroupCompositePixel(tracePixels, dx, dy) {
					xMin, yMin, xMax, yMax := clip.Bounds()
					fmt.Fprintf(os.Stderr, "SPLASH_GROUP_COMPOSITE_TRACE dst=(%d,%d) CLIP-REJECT bounds=(%.17g,%.17g)-(%.17g,%.17g) hasPath=%t\n",
						dx, dy, xMin, yMin, xMax, yMax, clip.HasPathClip())
				}
				continue
			}
			// Source alpha from group bitmap.
			var sAlpha byte = 255
			if src.alpha != nil {
				sAlpha = src.alpha[y*src.width+x]
			}
			aSrc := byte(Div255(int(sAlpha) * int(a8)))
			if softMask != nil {
				maskAlpha := softMaskByte(softMask, dx, dy)
				if useGroupPopplerSoftMaskAlphaOrder() {
					// Splash::pipeRun applies the soft mask to aInput before
					// multiplying by source shape for grouped composites.
					aSrc = byte(Div255(Div255(int(a8)*int(maskAlpha)) * int(sAlpha)))
				} else {
					aSrc = byte(Div255(int(aSrc) * int(maskAlpha)))
				}
			}
			traceGroup := shouldTraceGroupCompositePixel(tracePixels, dx, dy)
			traceLast := shouldTraceLastWriterPixel(dx, dy)
			trace := traceGroup || traceLast
			before := lastWriterSample{}
			if trace {
				before = captureLastWriterSample(dst, dx, dy)
			}
			if aSrc == 0 {
				if traceGroup {
					traceGroupCompositePixel(dx, dy, x, y, alpha, sAlpha, aSrc, 0, 0, 0, Color{}, Color{}, Color{}, before, captureLastWriterSample(dst, dx, dy), nonIsolated, blendMode != nil, softMask != nil, true)
				}
				continue
			}
			// Destination alpha.
			var aDst byte = 255
			if dst.alpha != nil {
				aDst = dst.alpha[dy*dst.width+dx]
			}
			// Color components.
			var srcC, dstC, blendC Color
			for i := 0; i < bpp; i++ {
				srcC[i] = src.data[sRowOff+x*bpp+i]
				dstC[i] = dst.data[dRowOff+dx*bpp+i]
			}
			if nonIsolated && sAlpha != 0 {
				t := (int(aDst)*255)/int(sAlpha) - int(aDst)
				for i := 0; i < bpp; i++ {
					srcC[i] = clipGroupByte(int(srcC[i]) + ((int(srcC[i])-int(dstC[i]))*t)/255)
				}
			}
			if blendMode != nil {
				blendC = blendMode(srcC, dstC, mode)
			} else {
				blendC = srcC
			}
			aResult := aSrc + aDst - byte(Div255(int(aSrc)*int(aDst)))
			alphaI := int(aResult)
			alphaIm1 := int(aDst)
			if a0, ok := groupCompositeAlpha0(alpha0, alpha0X+dx, alpha0Y+dy); ok {
				alphaI = int(aResult) + int(a0) - Div255(int(aResult)*int(a0))
				alphaIm1 = int(a0) + int(aDst) - Div255(int(a0)*int(aDst))
			}
			if alphaI == 0 {
				for i := 0; i < bpp; i++ {
					dst.data[dRowOff+dx*bpp+i] = 0
				}
				if dst.alpha != nil {
					dst.alpha[dy*dst.width+dx] = 0
				}
				if traceGroup {
					traceGroupCompositePixel(dx, dy, x, y, alpha, sAlpha, aSrc, aDst, alphaI, alphaIm1, srcC, dstC, blendC, before, captureLastWriterSample(dst, dx, dy), nonIsolated, blendMode != nil, softMask != nil, false)
				}
				traceLastWriter("groupComposite", sp, dst, dx, dy, before)
				continue
			}
			inv := 255 - alphaIm1
			diff := alphaI - int(aSrc)
			for i := 0; i < bpp; i++ {
				inner := inv*int(srcC[i]) + alphaIm1*int(blendC[i])
				v := (diff*int(dstC[i]) + int(aSrc)*inner/255) / alphaI
				if v < 0 {
					v = 0
				} else if v > 255 {
					v = 255
				}
				dst.data[dRowOff+dx*bpp+i] = groupCompositeTransferByte(sp, mode, i, byte(v))
			}
			if dst.alpha != nil {
				dst.alpha[dy*dst.width+dx] = aResult
			}
			if traceGroup {
				traceGroupCompositePixel(dx, dy, x, y, alpha, sAlpha, aSrc, aDst, alphaI, alphaIm1, srcC, dstC, blendC, before, captureLastWriterSample(dst, dx, dy), nonIsolated, blendMode != nil, softMask != nil, false)
			}
			traceLastWriter("groupComposite", sp, dst, dx, dy, before)
		}
	}
}

func groupCompositeTransferByte(sp *Splash, mode ColorMode, component int, value byte) byte {
	if sp == nil || sp.state == nil {
		return value
	}
	switch mode {
	case ModeMono1, ModeMono8:
		return sp.state.grayTransfer[value]
	case ModeRGB8:
		switch component {
		case 0:
			return sp.state.rgbTransferR[value]
		case 1:
			return sp.state.rgbTransferG[value]
		case 2:
			return sp.state.rgbTransferB[value]
		}
	case ModeBGR8:
		switch component {
		case 0:
			return sp.state.rgbTransferB[value]
		case 1:
			return sp.state.rgbTransferG[value]
		case 2:
			return sp.state.rgbTransferR[value]
		}
	case ModeXBGR8:
		switch component {
		case 0:
			return sp.state.rgbTransferB[value]
		case 1:
			return sp.state.rgbTransferG[value]
		case 2:
			return sp.state.rgbTransferR[value]
		case 3:
			return 255
		}
	case ModeCMYK8:
		switch component {
		case 0:
			return sp.state.cmykTransferC[value]
		case 1:
			return sp.state.cmykTransferM[value]
		case 2:
			return sp.state.cmykTransferY[value]
		case 3:
			return sp.state.cmykTransferK[value]
		}
	case ModeDeviceN8:
		if component >= 0 && component < len(sp.state.deviceNTransfer) {
			return sp.state.deviceNTransfer[component][value]
		}
	}
	return value
}

func groupCompositeAlpha0(alpha0 *Bitmap, x, y int) (byte, bool) {
	if alpha0 == nil || alpha0.alpha == nil || x < 0 || y < 0 || x >= alpha0.width || y >= alpha0.height {
		return 0, false
	}
	return alpha0.alpha[y*alpha0.width+x], true
}

func groupCompositeTracePixels() []aaLineTracePixel {
	return parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_GROUP_COMPOSITE_TRACE"))
}

func shouldTraceGroupCompositePixel(pixels []aaLineTracePixel, x, y int) bool {
	for _, pixel := range pixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func traceGroupCompositePixel(dx, dy, sx, sy int, alpha float64, sAlpha, aSrc, aDst byte, alphaI, alphaIm1 int, srcC, dstC, blendC Color, before, after lastWriterSample, nonIsolated, blend, softMask, skipped bool) {
	fmt.Fprintf(os.Stderr,
		"SPLASH_GROUP_COMPOSITE_TRACE dst=(%d,%d) src=(%d,%d) alpha=%.6f sAlpha=%d aSrc=%d aDst=%d alphaI=%d alphaIm1=%d srcC=(%d,%d,%d) dstC=(%d,%d,%d) blendC=(%d,%d,%d) nonIsolated=%t blend=%t softMask=%t skipped=%t before=(%d,%d,%d,a=%d,ok=%t) after=(%d,%d,%d,a=%d,ok=%t)\n",
		dx, dy, sx, sy, alpha, sAlpha, aSrc, aDst, alphaI, alphaIm1,
		srcC[0], srcC[1], srcC[2], dstC[0], dstC[1], dstC[2], blendC[0], blendC[1], blendC[2],
		nonIsolated, blend, softMask, skipped,
		before.color[0], before.color[1], before.color[2], before.alpha, before.ok,
		after.color[0], after.color[1], after.color[2], after.alpha, after.ok,
	)
}

func softMaskTracePixels() []aaLineTracePixel {
	return parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_SOFTMASK_TRACE"))
}

func shouldTraceSoftMaskPixel(pixels []aaLineTracePixel, x, y int) bool {
	for _, pixel := range pixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func traceSoftMaskHeader(kind string, top *groupState, options domainrenderer.SoftMaskOptions, src, mask *Bitmap) {
	if top == nil || src == nil || mask == nil {
		return
	}
	fmt.Fprintf(os.Stderr,
		"SPLASH_SOFTMASK_TRACE kind=%s cropped=%t tx=%d ty=%d bounds=[%d,%d,%d,%d] src=%dx%d mode=%v mask=%dx%d alpha=%t hasBackdrop=%t backdropLum=%d backdropRGB=(%d,%d,%d) transfer=%t\n",
		kind, top.cropped, top.tx, top.ty,
		top.compositeBounds[0], top.compositeBounds[1], top.compositeBounds[2], top.compositeBounds[3],
		src.width, src.height, src.mode, mask.width, mask.height,
		options.Alpha, options.HasBackdrop, options.BackdropLum,
		options.BackdropRGB[0], options.BackdropRGB[1], options.BackdropRGB[2],
		options.TransferActive,
	)
}

func traceSoftMaskInitialPixels(pixels []aaLineTracePixel, top *groupState, options domainrenderer.SoftMaskOptions, mask *Bitmap) {
	if top == nil || mask == nil {
		return
	}
	for _, pixel := range pixels {
		initial, ok := bitmapMonoByte(mask, pixel.x, pixel.y)
		inBounds := false
		if top.cropped {
			sx := pixel.x - top.tx
			sy := pixel.y - top.ty
			inBounds = sx >= top.compositeBounds[0] && sx < top.compositeBounds[2] &&
				sy >= top.compositeBounds[1] && sy < top.compositeBounds[3]
		} else {
			inBounds = pixel.x >= top.compositeBounds[0] && pixel.x < top.compositeBounds[2] &&
				pixel.y >= top.compositeBounds[1] && pixel.y < top.compositeBounds[3]
		}
		fmt.Fprintf(os.Stderr,
			"SPLASH_SOFTMASK_TRACE target=(%d,%d) initial=%d ok=%t inCompositeBounds=%t hasBackdrop=%t backdropLum=%d\n",
			pixel.x, pixel.y, initial, ok, inBounds, options.HasBackdrop, options.BackdropLum,
		)
	}
}

func traceSoftMaskPixel(kind string, dx, dy, sx, sy int, mode ColorMode, px []byte, alpha, value byte, options domainrenderer.SoftMaskOptions) {
	c0, c1, c2, c3 := byte(0), byte(0), byte(0), byte(0)
	if len(px) > 0 {
		c0 = px[0]
	}
	if len(px) > 1 {
		c1 = px[1]
	}
	if len(px) > 2 {
		c2 = px[2]
	}
	if len(px) > 3 {
		c3 = px[3]
	}
	rawLum := groupLuminosityByte(mode, px)
	backdropLum := groupLuminosityByteWithBackdrop(mode, px, alpha, options)
	fmt.Fprintf(os.Stderr,
		"SPLASH_SOFTMASK_TRACE kind=%s dst=(%d,%d) src=(%d,%d) mode=%v px=(%d,%d,%d,%d) alpha=%d rawLum=%d backdropLum=%d final=%d\n",
		kind, dx, dy, sx, sy, mode, c0, c1, c2, c3, alpha, rawLum, backdropLum, value,
	)
}

func traceSoftMaskFinalPixels(pixels []aaLineTracePixel, mask *Bitmap) {
	for _, pixel := range pixels {
		value, ok := bitmapMonoByte(mask, pixel.x, pixel.y)
		fmt.Fprintf(os.Stderr,
			"SPLASH_SOFTMASK_TRACE target=(%d,%d) final=%d ok=%t\n",
			pixel.x, pixel.y, value, ok,
		)
	}
}

func bitmapMonoByte(bitmap *Bitmap, x, y int) (byte, bool) {
	if bitmap == nil || bitmap.data == nil || x < 0 || y < 0 || x >= bitmap.width || y >= bitmap.height {
		return 0, false
	}
	bpp := bytesPerPixel(bitmap.mode)
	if bpp <= 0 {
		return 0, false
	}
	off := y*bitmap.rowSize + x*bpp
	if off < 0 || off >= len(bitmap.data) {
		return 0, false
	}
	return bitmap.data[off], true
}

func clipGroupByte(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func copyBitmapRect(src, dst *Bitmap, sx, sy, dx, dy, w, h int, copyAlpha bool) {
	if src == nil || dst == nil || w <= 0 || h <= 0 || src.mode != dst.mode {
		return
	}
	bpp := bytesPerPixel(src.mode)
	if bpp <= 0 {
		return
	}
	for row := 0; row < h; row++ {
		srcY := sy + row
		dstY := dy + row
		if srcY < 0 || srcY >= src.height || dstY < 0 || dstY >= dst.height {
			continue
		}
		srcX0 := sx
		dstX0 := dx
		width := w
		if srcX0 < 0 {
			shift := -srcX0
			srcX0 += shift
			dstX0 += shift
			width -= shift
		}
		if dstX0 < 0 {
			shift := -dstX0
			srcX0 += shift
			dstX0 += shift
			width -= shift
		}
		if srcX0+width > src.width {
			width = src.width - srcX0
		}
		if dstX0+width > dst.width {
			width = dst.width - dstX0
		}
		if width <= 0 {
			continue
		}
		srcOff := srcY*src.rowSize + srcX0*bpp
		dstOff := dstY*dst.rowSize + dstX0*bpp
		copy(dst.data[dstOff:dstOff+width*bpp], src.data[srcOff:srcOff+width*bpp])
		if copyAlpha && src.alpha != nil && dst.alpha != nil {
			copy(dst.alpha[dstY*dst.width+dstX0:dstY*dst.width+dstX0+width], src.alpha[srcY*src.width+srcX0:srcY*src.width+srcX0+width])
		}
	}
}

// softMaskByte returns the single-channel alpha value at (x,y) from a
// ModeMono8 mask bitmap (Splash.cc:1208-1209 indexing). Out-of-bounds reads
// return 0 (fully masked) so the rasterizer never panics on an undersized
// mask. Inline byte access avoids touching bitmap.go (out of scope).
func softMaskByte(m *Bitmap, x, y int) byte {
	if m == nil || x < 0 || y < 0 || x >= m.width || y >= m.height {
		return 0
	}
	rs := m.rowSize
	if rs <= 0 {
		rs = m.width
	}
	off := y*rs + x
	if off < 0 || off >= len(m.data) {
		return 0
	}
	return m.data[off]
}

func groupLuminosityByte(mode ColorMode, px []byte) byte {
	switch mode {
	case ModeMono8:
		if len(px) > 0 {
			return px[0]
		}
	case ModeCMYK8, ModeDeviceN8:
		if len(px) >= 4 {
			lum := (255 - int(px[3])) - (30*int(px[0])+59*int(px[1])+11*int(px[2]))/100
			if lum < 0 {
				return 0
			}
			if lum > 255 {
				return 255
			}
			return byte(lum)
		}
	default:
		if len(px) >= 3 {
			return rgbLuminosityByte(px[0], px[1], px[2])
		}
	}
	return 0
}

func groupLuminosityByteWithBackdrop(mode ColorMode, px []byte, alpha byte, options domainrenderer.SoftMaskOptions) byte {
	if !options.HasBackdrop || alpha == 255 {
		return groupLuminosityByte(mode, px)
	}
	if alpha == 0 {
		return options.BackdropLum
	}
	switch mode {
	case ModeMono8:
		if len(px) > 0 {
			return compositeBackdropComponent(px[0], alpha, options.BackdropLum)
		}
	case ModeRGB8:
		if len(px) >= 3 {
			r := compositeBackdropComponent(px[0], alpha, options.BackdropRGB[0])
			g := compositeBackdropComponent(px[1], alpha, options.BackdropRGB[1])
			b := compositeBackdropComponent(px[2], alpha, options.BackdropRGB[2])
			return rgbLuminosityByte(r, g, b)
		}
	case ModeBGR8, ModeXBGR8:
		if len(px) >= 3 {
			b := compositeBackdropComponent(px[0], alpha, options.BackdropRGB[2])
			g := compositeBackdropComponent(px[1], alpha, options.BackdropRGB[1])
			r := compositeBackdropComponent(px[2], alpha, options.BackdropRGB[0])
			return rgbLuminosityByte(r, g, b)
		}
	}
	return groupLuminosityByte(mode, px)
}

func rgbLuminosityByte(r, g, b byte) byte {
	return byte((30*int(r) + 59*int(g) + 11*int(b) + 50) / 100)
}

func compositeBackdropComponent(src, alpha, backdrop byte) byte {
	return byte(Div255(int(255-alpha)*int(backdrop) + int(alpha)*int(src)))
}

func useSoftMaskCompositeBackground() bool {
	raw := strings.TrimSpace(os.Getenv("GO_PDF_SMASK_COMPOSITE_BACKGROUND"))
	return raw != "" && raw != "0" && !strings.EqualFold(raw, "false")
}

func compositeSoftMaskBackground(bitmap *Bitmap, options domainrenderer.SoftMaskOptions) {
	if bitmap == nil || bitmap.data == nil || bitmap.alpha == nil || !options.HasBackdrop {
		return
	}
	mode := bitmap.mode
	bpp := bytesPerPixel(mode)
	for y := 0; y < bitmap.height; y++ {
		rowOff := y * bitmap.rowSize
		alphaOff := y * bitmap.width
		for x := 0; x < bitmap.width; x++ {
			alpha := bitmap.alpha[alphaOff+x]
			px := bitmap.data[rowOff+x*bpp:]
			compositeSoftMaskBackgroundPixel(mode, px, alpha, options)
			bitmap.alpha[alphaOff+x] = 255
		}
	}
}

func compositeSoftMaskBackgroundPixel(mode ColorMode, px []byte, alpha byte, options domainrenderer.SoftMaskOptions) {
	switch mode {
	case ModeMono8:
		if len(px) >= 1 {
			px[0] = compositeBackdropComponent(px[0], alpha, options.BackdropLum)
		}
	case ModeRGB8:
		if len(px) >= 3 {
			compositeSoftMaskBackgroundRGB(px, alpha, options.BackdropRGB[0], options.BackdropRGB[1], options.BackdropRGB[2])
		}
	case ModeBGR8:
		if len(px) >= 3 {
			compositeSoftMaskBackgroundRGB(px, alpha, options.BackdropRGB[2], options.BackdropRGB[1], options.BackdropRGB[0])
		}
	case ModeXBGR8:
		if len(px) >= 4 {
			compositeSoftMaskBackgroundRGB(px, alpha, options.BackdropRGB[2], options.BackdropRGB[1], options.BackdropRGB[0])
			px[3] = 255
		}
	}
}

func compositeSoftMaskBackgroundRGB(px []byte, alpha byte, c0, c1, c2 byte) {
	if alpha == 0 {
		px[0], px[1], px[2] = c0, c1, c2
		return
	}
	if alpha == 255 {
		return
	}
	px[0] = compositeBackdropComponent(px[0], alpha, c0)
	px[1] = compositeBackdropComponent(px[1], alpha, c1)
	px[2] = compositeBackdropComponent(px[2], alpha, c2)
}
