package splash

// groupState saves a parent's render target while drawing into a child
// transparency-group bitmap (Splash.cc:5021-5254 + PDF spec 11.4.7).
type groupState struct {
	bbox        [4]float64
	blendMode   BlendFunc
	isolated    bool
	knockout    bool
	groupBitmap *Bitmap
	savedBitmap *Bitmap
	savedAaBuf  []byte
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
	parent := s.bitmap
	w := parent.Width()
	h := parent.Height()
	// Allocate a same-size sub-bitmap. Phase-4 simple model: groups always
	// cover the parent canvas (bbox kept for parity / future cropping).
	gb := NewBitmap(w, h, parent.mode, true)
	if isolated {
		// PDF spec 11.4.7.5: isolated groups start with transparent backdrop.
		// gb.data and gb.alpha already zero from make, which is transparent
		// black — exactly what we need.
	} else {
		// Non-isolated: copy parent backdrop in (color + alpha).
		copy(gb.data, parent.data)
		if gb.alpha != nil && parent.alpha != nil {
			copy(gb.alpha, parent.alpha)
		}
	}
	gs := &groupState{
		bbox:        bbox,
		blendMode:   blendMode,
		isolated:    isolated,
		knockout:    knockout,
		groupBitmap: gb,
		savedBitmap: parent,
		savedAaBuf:  s.aaBuf,
	}
	s.groupStack = append(s.groupStack, gs)
	s.bitmap = gb
	if s.vectorAA && gb.Width() > 0 {
		s.aaBuf = make([]byte, splashAASize*gb.Width())
	}
	return nil
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
	s.bitmap = dst
	s.aaBuf = top.savedAaBuf

	compositeGroup(src, dst, top.blendMode, s.state.softMask, s.state.fillAlpha)
	return nil
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
	s.bitmap = top.savedBitmap
	s.aaBuf = top.savedAaBuf
	return nil
}

// compositeGroup blits src onto dst per PDF spec 11.4.5 (Compositing). For
// each pixel: aSrc = src.alpha * mask / 255; aResult = aSrc + aDst - aSrc*aDst/255;
// cBlend = blendMode(cSrc, cDst); cResult = ((aResult-aSrc)*cDst + aSrc*((255-aDst)*cSrc + aDst*cBlend)/255) / aResult.
// When blendMode is nil this collapses to Normal (cBlend == cSrc).
func compositeGroup(src, dst *Bitmap, blendMode BlendFunc, softMask *Bitmap, alpha float64) {
	if src == nil || dst == nil || src.width != dst.width || src.height != dst.height {
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
	w := dst.width
	h := dst.height
	for y := 0; y < h; y++ {
		dRowOff := y * dst.rowSize
		sRowOff := y * src.rowSize
		for x := 0; x < w; x++ {
			// Source alpha from group bitmap.
			var sAlpha byte = 255
			if src.alpha != nil {
				sAlpha = src.alpha[y*src.width+x]
			}
			aSrc := byte(Div255(int(sAlpha) * int(a8)))
			if softMask != nil {
				aSrc = byte(Div255(int(aSrc) * int(softMaskByte(softMask, x, y))))
			}
			if aSrc == 0 {
				continue
			}
			// Destination alpha.
			var aDst byte = 255
			if dst.alpha != nil {
				aDst = dst.alpha[y*dst.width+x]
			}
			// Color components.
			var srcC, dstC, blendC Color
			for i := 0; i < bpp; i++ {
				srcC[i] = src.data[sRowOff+x*bpp+i]
				dstC[i] = dst.data[dRowOff+x*bpp+i]
			}
			if blendMode != nil {
				blendMode(&srcC, &dstC, &blendC, mode)
			} else {
				blendC = srcC
			}
			aResult := aSrc + aDst - byte(Div255(int(aSrc)*int(aDst)))
			alphaI := int(aResult)
			if alphaI == 0 {
				for i := 0; i < bpp; i++ {
					dst.data[dRowOff+x*bpp+i] = 0
				}
				if dst.alpha != nil {
					dst.alpha[y*dst.width+x] = 0
				}
				continue
			}
			inv := 255 - int(aDst)
			diff := alphaI - int(aSrc)
			for i := 0; i < bpp; i++ {
				inner := inv*int(srcC[i]) + int(aDst)*int(blendC[i])
				v := (diff*int(dstC[i]) + int(aSrc)*inner/255) / alphaI
				if v < 0 {
					v = 0
				} else if v > 255 {
					v = 255
				}
				dst.data[dRowOff+x*bpp+i] = byte(v)
			}
			if dst.alpha != nil {
				dst.alpha[y*dst.width+x] = aResult
			}
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
