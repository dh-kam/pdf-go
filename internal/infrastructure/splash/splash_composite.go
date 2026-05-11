package splash

import "math"

// BlendFunc mirrors SplashBlendFunc (SplashTypes.h:207).
//
// Each implementation reads src and dst component-wise (or holistically for the
// non-separable HSL modes), writes the per-pixel blend result to blend, and is
// selected at the per-pixel site Splash.cc:535-541 inside pipeRun. The blend
// output is then alpha-composited against the destination by the same pipeRun.
type BlendFunc func(src, dst, blend *Color, mode ColorMode)

// nComps returns the number of color components for a Splash mode (matches the
// per-mode switch in pipeRun, Splash.cc:535-541 + SplashTypes.h:56-72).
func nComps(mode ColorMode) int {
	switch mode {
	case ModeMono1, ModeMono8:
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

// rgbIndices returns the (r,g,b) component indices for a Splash mode. For BGR8
// the order is reversed; XBGR8 keeps (B,G,R) layout per SplashTypes.h:64-65.
// CMYK has no RGB triplet — caller handles separately.
func rgbIndices(mode ColorMode) (int, int, int) {
	switch mode {
	case ModeBGR8, ModeXBGR8:
		return 2, 1, 0
	}
	return 0, 1, 2
}

// blendCopyExtra preserves any color components NOT touched by the blend
// formula (e.g. spot channels in DeviceN). PDF spec 11.3.5: "non-process
// colorants are unchanged by blend modes".
func blendCopyExtra(src, blend *Color, n int) {
	for k := n; k < splashMaxColorComps; k++ {
		blend[k] = src[k]
	}
}

// BlendNormal — PDF spec 11.3.5.2 / Splash.cc default (state->blendFunc==nil).
// B(Cs, Cb) = Cs.
func BlendNormal(src, dst, blend *Color, mode ColorMode) {
	*blend = *src
}

// BlendMultiply mirrors Poppler's splashOutBlendMultiply. RGB uses truncating
// /255, and subtractive modes blend in additive space before converting back.
func BlendMultiply(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	if mode == ModeCMYK8 || mode == ModeDeviceN8 {
		for i := 0; i < n; i++ {
			cs := 255 - int(src[i])
			cb := 255 - int(dst[i])
			blend[i] = byte(255 - (cs*cb)/255)
		}
		blendCopyExtra(src, blend, n)
		return
	}
	for i := 0; i < n; i++ {
		blend[i] = byte((int(src[i]) * int(dst[i])) / 255)
	}
	blendCopyExtra(src, blend, n)
}

// BlendScreen — PDF spec 11.3.5.2: B = Cb + Cs - Cs*Cb/255.
func BlendScreen(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		blend[i] = byte(int(src[i]) + int(dst[i]) - Div255(int(src[i])*int(dst[i])))
	}
	blendCopyExtra(src, blend, n)
}

// BlendOverlay — PDF spec 11.3.5.2: B = HardLight(Cb, Cs) (src/dst swapped).
func BlendOverlay(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		cs := int(src[i])
		cb := int(dst[i])
		if cb <= 127 {
			blend[i] = byte(Div255(2 * cs * cb))
		} else {
			blend[i] = byte(2*cs + 2*cb - Div255(2*cs*cb) - 255)
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendDarken — PDF spec 11.3.5.2: B = min(Cs, Cb).
func BlendDarken(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		if src[i] < dst[i] {
			blend[i] = src[i]
		} else {
			blend[i] = dst[i]
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendLighten — PDF spec 11.3.5.2: B = max(Cs, Cb).
func BlendLighten(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		if src[i] > dst[i] {
			blend[i] = src[i]
		} else {
			blend[i] = dst[i]
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendColorDodge — PDF spec 11.3.5.2: brightens Cb by Cs.
func BlendColorDodge(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		cs := int(src[i])
		cb := int(dst[i])
		if cs == 255 {
			blend[i] = 255
		} else {
			v := (cb * 255) / (255 - cs)
			if v > 255 {
				v = 255
			}
			blend[i] = byte(v)
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendColorBurn — PDF spec 11.3.5.2: darkens Cb by Cs.
func BlendColorBurn(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		cs := int(src[i])
		cb := int(dst[i])
		if cs == 0 {
			blend[i] = 0
		} else {
			v := ((255 - cb) * 255) / cs
			if v > 255 {
				v = 255
			}
			blend[i] = byte(255 - v)
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendHardLight — PDF spec 11.3.5.2: Multiply or Screen by sign of (Cs-128).
func BlendHardLight(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		cs := int(src[i])
		cb := int(dst[i])
		if cs <= 127 {
			blend[i] = byte(Div255(2 * cs * cb))
		} else {
			blend[i] = byte(2*cs + 2*cb - Div255(2*cs*cb) - 255)
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendSoftLight — PDF spec 11.3.5.2 (equation 8.4): piecewise smooth dodge/burn.
//
// For Cs <= 0.5:  B = Cb - (1-2*Cs) * Cb * (1-Cb)
// For Cs >  0.5:  B = Cb + (2*Cs - 1) * (D(Cb) - Cb)
//
//	where D(x) = (16*x - 12)*x*x + 4*x  for x <= 0.25
//	             sqrt(x)                 for x >  0.25
//
// Implemented in 0..255 byte space using float intermediates only for the
// transcendental D(x); arithmetic mirrors the PDF spec exactly.
func BlendSoftLight(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		blend[i] = softLight(src[i], dst[i])
	}
	blendCopyExtra(src, blend, n)
}

// softLight is the per-channel PDF 11.3.5.2 SoftLight in [0,255] space.
func softLight(s, b byte) byte {
	cs := float64(s) / 255.0
	cb := float64(b) / 255.0
	var out float64
	if cs <= 0.5 {
		out = cb - (1.0-2.0*cs)*cb*(1.0-cb)
	} else {
		var d float64
		if cb <= 0.25 {
			d = ((16.0*cb-12.0)*cb + 4.0) * cb
		} else {
			d = math.Sqrt(cb)
		}
		out = cb + (2.0*cs-1.0)*(d-cb)
	}
	if out < 0 {
		out = 0
	} else if out > 1 {
		out = 1
	}
	return byte(Round(out * 255.0))
}

// BlendDifference — PDF spec 11.3.5.2: B = |Cs - Cb|.
func BlendDifference(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		if src[i] >= dst[i] {
			blend[i] = src[i] - dst[i]
		} else {
			blend[i] = dst[i] - src[i]
		}
	}
	blendCopyExtra(src, blend, n)
}

// BlendExclusion — PDF spec 11.3.5.2: B = Cs + Cb - 2*Cs*Cb/255.
func BlendExclusion(src, dst, blend *Color, mode ColorMode) {
	n := nComps(mode)
	for i := 0; i < n; i++ {
		cs := int(src[i])
		cb := int(dst[i])
		blend[i] = byte(cs + cb - 2*Div255(cs*cb))
	}
	blendCopyExtra(src, blend, n)
}

// ----- Non-separable blend modes (PDF spec 11.3.5.3, equations 11.3-11.6) -----
//
// Operate over RGB triple as a unit. For CMYK/DeviceN we convert via 255-c
// inversion so the same RGB equations apply (matches Adobe + Splash convention
// for non-separable modes on subtractive spaces, PDF spec 11.3.5.3).

// BlendHue — PDF spec 11.3.5.3 eq 11.6: SetLum(SetSat(Cs, Sat(Cb)), Lum(Cb)).
func BlendHue(src, dst, blend *Color, mode ColorMode) {
	nonSepBlend(src, dst, blend, mode, func(rs, gs, bs, rb, gb, bbv float64) (float64, float64, float64) {
		r, g, b := setSat(rs, gs, bs, sat(rb, gb, bbv))
		return setLum(r, g, b, lum(rb, gb, bbv))
	})
}

// BlendSaturation — PDF spec 11.3.5.3 eq 11.7: SetLum(SetSat(Cb, Sat(Cs)), Lum(Cb)).
func BlendSaturation(src, dst, blend *Color, mode ColorMode) {
	nonSepBlend(src, dst, blend, mode, func(rs, gs, bs, rb, gb, bbv float64) (float64, float64, float64) {
		r, g, b := setSat(rb, gb, bbv, sat(rs, gs, bs))
		return setLum(r, g, b, lum(rb, gb, bbv))
	})
}

// BlendColor — PDF spec 11.3.5.3 eq 11.8: SetLum(Cs, Lum(Cb)).
func BlendColor(src, dst, blend *Color, mode ColorMode) {
	nonSepBlend(src, dst, blend, mode, func(rs, gs, bs, rb, gb, bbv float64) (float64, float64, float64) {
		return setLum(rs, gs, bs, lum(rb, gb, bbv))
	})
}

// BlendLuminosity — PDF spec 11.3.5.3 eq 11.9: SetLum(Cb, Lum(Cs)).
func BlendLuminosity(src, dst, blend *Color, mode ColorMode) {
	nonSepBlend(src, dst, blend, mode, func(rs, gs, bs, rb, gb, bbv float64) (float64, float64, float64) {
		return setLum(rb, gb, bbv, lum(rs, gs, bs))
	})
}

// nonSepBlend dispatches a non-separable RGB blend equation across the four
// modes Splash supports. CMYK is handled by inverting components before/after
// the RGB equation (PDF spec 11.3.5.3 note 2). Spot/extra channels are copied
// through unchanged.
func nonSepBlend(src, dst, blend *Color, mode ColorMode, eq func(rs, gs, bs, rb, gb, bbv float64) (float64, float64, float64)) {
	if mode == ModeMono1 || mode == ModeMono8 {
		// Mono: treat the single component as luminance — Lum-based modes
		// reduce to copying the chosen luminance source. PDF spec 11.3.5.3.
		var ls, lb byte = src[0], dst[0]
		// Reuse eq by treating gray as r=g=b: the non-separable RGB equations
		// then reduce algebraically to a function of (ls, lb) only.
		fs := float64(ls) / 255.0
		fb := float64(lb) / 255.0
		r, _, _ := eq(fs, fs, fs, fb, fb, fb)
		blend[0] = byteClampFloat(r)
		blendCopyExtra(src, blend, 1)
		return
	}
	if mode == ModeCMYK8 {
		// Convert CMY to RGB via 1 - c, run equation, invert back; K is
		// derived as min of the result triple per PDF spec 11.3.5.3.
		rs := 1.0 - float64(src[0])/255.0
		gs := 1.0 - float64(src[1])/255.0
		bs := 1.0 - float64(src[2])/255.0
		rb := 1.0 - float64(dst[0])/255.0
		gb := 1.0 - float64(dst[1])/255.0
		bbv := 1.0 - float64(dst[2])/255.0
		r, g, b := eq(rs, gs, bs, rb, gb, bbv)
		blend[0] = byteClampFloat(1.0 - r)
		blend[1] = byteClampFloat(1.0 - g)
		blend[2] = byteClampFloat(1.0 - b)
		// K passes through from src per Splash convention (no separable K
		// component in the RGB equations).
		blend[3] = src[3]
		blendCopyExtra(src, blend, 4)
		return
	}
	if mode == ModeDeviceN8 {
		// Process channels (first 4) treated as CMYK; spot channels copied.
		rs := 1.0 - float64(src[0])/255.0
		gs := 1.0 - float64(src[1])/255.0
		bs := 1.0 - float64(src[2])/255.0
		rb := 1.0 - float64(dst[0])/255.0
		gb := 1.0 - float64(dst[1])/255.0
		bbv := 1.0 - float64(dst[2])/255.0
		r, g, b := eq(rs, gs, bs, rb, gb, bbv)
		blend[0] = byteClampFloat(1.0 - r)
		blend[1] = byteClampFloat(1.0 - g)
		blend[2] = byteClampFloat(1.0 - b)
		blend[3] = src[3]
		blendCopyExtra(src, blend, 4)
		return
	}
	// RGB / BGR / XBGR.
	ri, gi, bi := rgbIndices(mode)
	rs := float64(src[ri]) / 255.0
	gs := float64(src[gi]) / 255.0
	bs := float64(src[bi]) / 255.0
	rb := float64(dst[ri]) / 255.0
	gb := float64(dst[gi]) / 255.0
	bbv := float64(dst[bi]) / 255.0
	r, g, b := eq(rs, gs, bs, rb, gb, bbv)
	blend[ri] = byteClampFloat(r)
	blend[gi] = byteClampFloat(g)
	blend[bi] = byteClampFloat(b)
	if mode == ModeXBGR8 {
		blend[3] = src[3]
	}
	blendCopyExtra(src, blend, nComps(mode))
}

// ----- HSL helpers per PDF spec 11.3.5.3 -----

// lum is PDF spec 11.3.5.3 eq 11.10: Lum(C) = 0.3 R + 0.59 G + 0.11 B.
func lum(r, g, b float64) float64 { return 0.3*r + 0.59*g + 0.11*b }

// sat is PDF spec 11.3.5.3 eq 11.13: max(R,G,B) - min(R,G,B).
func sat(r, g, b float64) float64 {
	mx := r
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	return mx - mn
}

// setLum is PDF spec 11.3.5.3 eq 11.11: shifts (r,g,b) so its luminance becomes
// l, then clips back to [0,1] via ClipColor.
func setLum(r, g, b, l float64) (float64, float64, float64) {
	d := l - lum(r, g, b)
	return clipColor(r+d, g+d, b+d)
}

// setSat is PDF spec 11.3.5.3 eq 11.12: rescale (r,g,b) so the spread between
// max and min becomes s, preserving the relative ordering.
func setSat(r, g, b, s float64) (float64, float64, float64) {
	// Identify min, mid, max channel.
	type ch struct {
		v   float64
		idx int
	}
	cs := [3]ch{{r, 0}, {g, 1}, {b, 2}}
	// Sort ascending — 3 elements, do branchless bubble.
	if cs[0].v > cs[1].v {
		cs[0], cs[1] = cs[1], cs[0]
	}
	if cs[1].v > cs[2].v {
		cs[1], cs[2] = cs[2], cs[1]
	}
	if cs[0].v > cs[1].v {
		cs[0], cs[1] = cs[1], cs[0]
	}
	out := [3]float64{}
	if cs[2].v > cs[0].v {
		out[cs[1].idx] = ((cs[1].v - cs[0].v) * s) / (cs[2].v - cs[0].v)
		out[cs[2].idx] = s
	} else {
		out[cs[1].idx] = 0
		out[cs[2].idx] = 0
	}
	out[cs[0].idx] = 0
	return out[0], out[1], out[2]
}

// clipColor is PDF spec 11.3.5.3 eq 11.14: pulls out-of-gamut colors back into
// [0,1] while preserving luminance.
func clipColor(r, g, b float64) (float64, float64, float64) {
	l := lum(r, g, b)
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	mx := r
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	if mn < 0 {
		k := l - mn
		if k == 0 {
			r, g, b = l, l, l
		} else {
			r = l + (r-l)*l/k
			g = l + (g-l)*l/k
			b = l + (b-l)*l/k
		}
	}
	if mx > 1 {
		k := mx - l
		if k == 0 {
			r, g, b = l, l, l
		} else {
			r = l + (r-l)*(1-l)/k
			g = l + (g-l)*(1-l)/k
			b = l + (b-l)*(1-l)/k
		}
	}
	return r, g, b
}

// byteClampFloat rounds a [0,1] float to 0..255 byte, clamping on overflow.
func byteClampFloat(x float64) byte {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 255
	}
	return byte(Round(x * 255.0))
}
