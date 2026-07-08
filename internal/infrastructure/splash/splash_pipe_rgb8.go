package splash

// pipeRunSimpleRGB8 mirrors Splash::pipeRunSimpleRGB8 (Splash.cc:780-789).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunSimpleRGB8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(pipePatternX(p), p.y, &c) {
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
	p.destRow[p.destOff+0] = p.s.state.rgbTransferR[src[0]]
	p.destRow[p.destOff+1] = p.s.state.rgbTransferG[src[1]]
	p.destRow[p.destOff+2] = p.s.state.rgbTransferB[src[2]]
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
		if !p.pattern.GetColor(pipePatternX(p), p.y, &c) {
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
		// per Splash.cc:644-647. Non-isolated groups adjust alphaI/alphaIm1
		// with alpha0Ptr before the result-color equation.
		var dstColor Color
		dstColor[0], dstColor[1], dstColor[2] = dR, dG, dB
		blend := p.blendFunc(src, dstColor, p.mode)
		if aSrc == 0 && aDest == 0 {
			c0, c1, c2, aResult = 0, 0, 0, 0
		} else {
			var alphaI, alphaIm1 int
			aResult, alphaI, alphaIm1 = pipeResultAlphas(p, aSrc, aDest)
			if alphaI == 0 {
				c0, c1, c2 = 0, 0, 0
			} else {
				// inner = (255-aDest)*cSrc + aDest*cBlend; result = ((alphaI-aSrc)*cDest + aSrc*inner/255) / alphaI
				inv := 255 - alphaIm1
				in0 := inv*int(src[0]) + alphaIm1*int(blend[0])
				in1 := inv*int(src[1]) + alphaIm1*int(blend[1])
				in2 := inv*int(src[2]) + alphaIm1*int(blend[2])
				diff := alphaI - int(aSrc)
				c0 = p.s.state.rgbTransferR[byte((diff*int(dR)+int(aSrc)*in0/255)/alphaI)]
				c1 = p.s.state.rgbTransferG[byte((diff*int(dG)+int(aSrc)*in1/255)/alphaI)]
				c2 = p.s.state.rgbTransferB[byte((diff*int(dB)+int(aSrc)*in2/255)/alphaI)]
			}
		}
	} else if aSrc == 255 && p.alpha0 == nil {
		c0 = p.s.state.rgbTransferR[src[0]]
		c1 = p.s.state.rgbTransferG[src[1]]
		c2 = p.s.state.rgbTransferB[src[2]]
		aResult = 255
	} else if aSrc == 0 && aDest == 0 {
		c0, c1, c2, aResult = 0, 0, 0, 0
	} else {
		var alpha2 int
		aResult, alpha2, _ = pipeResultAlphas(p, aSrc, aDest)
		alphaSrc := int(aSrc)
		alphaDestWeight := alpha2 - alphaSrc
		c0 = p.s.state.rgbTransferR[byte((alphaDestWeight*int(dR)+alphaSrc*int(src[0]))/alpha2)]
		c1 = p.s.state.rgbTransferG[byte((alphaDestWeight*int(dG)+alphaSrc*int(src[1]))/alpha2)]
		c2 = p.s.state.rgbTransferB[byte((alphaDestWeight*int(dB)+alphaSrc*int(src[2]))/alpha2)]
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

// pipeRunSimpleRGB8NoPat is the allocation-free fast path of pipeRunSimpleRGB8 (when p.pattern == nil).
func pipeRunSimpleRGB8NoPat(p *pipe) {
	src := p.cSrc
	p.destRow[p.destOff+0] = p.s.state.rgbTransferR[src[0]]
	p.destRow[p.destOff+1] = p.s.state.rgbTransferG[src[1]]
	p.destRow[p.destOff+2] = p.s.state.rgbTransferB[src[2]]
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff += 3
	p.x++
}

// pipeRunAARGB8NoPat is the allocation-free fast path of pipeRunAARGB8 (when p.pattern == nil and p.blendFunc == nil).
func pipeRunAARGB8NoPat(p *pipe) {
	src := p.cSrc
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	} else {
		aDest = 0xFF
	}
	aSrc := pipeSourceAlpha(p)

	dR := p.destRow[p.destOff+0]
	dG := p.destRow[p.destOff+1]
	dB := p.destRow[p.destOff+2]

	var c0, c1, c2, aResult byte
	if aSrc == 255 && p.alpha0 == nil {
		c0 = p.s.state.rgbTransferR[src[0]]
		c1 = p.s.state.rgbTransferG[src[1]]
		c2 = p.s.state.rgbTransferB[src[2]]
		aResult = 255
	} else if aSrc == 0 && aDest == 0 {
		c0, c1, c2, aResult = 0, 0, 0, 0
	} else {
		var alpha2 int
		aResult, alpha2, _ = pipeResultAlphas(p, aSrc, aDest)
		alphaSrc := int(aSrc)
		alphaDestWeight := alpha2 - alphaSrc
		c0 = p.s.state.rgbTransferR[byte((alphaDestWeight*int(dR)+alphaSrc*int(src[0]))/alpha2)]
		c1 = p.s.state.rgbTransferG[byte((alphaDestWeight*int(dG)+alphaSrc*int(src[1]))/alpha2)]
		c2 = p.s.state.rgbTransferB[byte((alphaDestWeight*int(dB)+alphaSrc*int(src[2]))/alpha2)]
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

