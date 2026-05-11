package splash

// pipeRunSimpleRGB8 mirrors Splash::pipeRunSimpleRGB8 (Splash.cc:780-789).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunSimpleRGB8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(p.x, p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 3
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 3
			p.x++
			return
		}
		src = c
	}
	p.destRow[p.destOff+0] = src[0]
	p.destRow[p.destOff+1] = src[1]
	p.destRow[p.destOff+2] = src[2]
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff += 3
	p.x++
}

// pipeRunAARGB8 mirrors Splash::pipeRunAARGB8 (Splash.cc:939-986).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
// Blend hook mirrors Splash.cc:535-541 + result-color AlphaBlendRGB Splash.cc:639-648.
func pipeRunAARGB8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(p.x, p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 3
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 3
			p.x++
			return
		}
		src = c
	}
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	} else {
		// SplashPipe treats bitmaps without an alpha plane as fully opaque.
		aDest = 0xFF
	}
	aSrc := pipeSourceAlpha(p)

	dR := p.destRow[p.destOff+0]
	dG := p.destRow[p.destOff+1]
	dB := p.destRow[p.destOff+2]

	var c0, c1, c2, aResult byte
	if p.blendFunc != nil {
		// Splash.cc:535-541 — compute cBlend from src + dst, then alpha-mix
		// per Splash.cc:644-647 with alphaIm1 = aDest, alphaI = aResult.
		var dst, blend Color
		dst[0], dst[1], dst[2] = dR, dG, dB
		p.blendFunc(&src, &dst, &blend, p.mode)
		if aSrc == 0 && aDest == 0 {
			c0, c1, c2, aResult = 0, 0, 0, 0
		} else {
			aResult = aSrc + aDest - byte(Div255(int(aSrc)*int(aDest)))
			alphaI := int(aResult)
			if alphaI == 0 {
				c0, c1, c2 = 0, 0, 0
			} else {
				// inner = (255-aDest)*cSrc + aDest*cBlend; result = ((alphaI-aSrc)*cDest + aSrc*inner/255) / alphaI
				inv := 255 - int(aDest)
				in0 := inv*int(src[0]) + int(aDest)*int(blend[0])
				in1 := inv*int(src[1]) + int(aDest)*int(blend[1])
				in2 := inv*int(src[2]) + int(aDest)*int(blend[2])
				diff := alphaI - int(aSrc)
				c0 = byte((diff*int(dR) + int(aSrc)*in0/255) / alphaI)
				c1 = byte((diff*int(dG) + int(aSrc)*in1/255) / alphaI)
				c2 = byte((diff*int(dB) + int(aSrc)*in2/255) / alphaI)
			}
		}
	} else if aSrc == 255 {
		c0 = src[0]
		c1 = src[1]
		c2 = src[2]
		aResult = 255
	} else if aSrc == 0 && aDest == 0 {
		c0, c1, c2, aResult = 0, 0, 0, 0
	} else {
		aResult = aSrc + aDest - byte(Div255(int(aSrc)*int(aDest)))
		alpha2 := int(aResult)
		alphaSrc := int(aSrc)
		alphaDestWeight := alpha2 - alphaSrc
		c0 = byte((alphaDestWeight*int(dR) + alphaSrc*int(src[0])) / alpha2)
		c1 = byte((alphaDestWeight*int(dG) + alphaSrc*int(src[1])) / alpha2)
		c2 = byte((alphaDestWeight*int(dB) + alphaSrc*int(src[2])) / alpha2)
	}
	p.destRow[p.destOff+0] = c0
	p.destRow[p.destOff+1] = c1
	p.destRow[p.destOff+2] = c2
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = aResult
		p.aDestOff++
	}
	p.destOff += 3
	p.x++
}
