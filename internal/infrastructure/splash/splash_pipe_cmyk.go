package splash

// pipeRunSimpleCMYK8 mirrors Splash::pipeRunSimpleCMYK8 (Splash.cc:823-842).
// Phase-1 simplification: no overprint mask.
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunSimpleCMYK8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(pipePatternX(p), p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 4
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 4
			p.x++
			return
		}
		src = c
	}
	p.destRow[p.destOff+0] = p.s.state.cmykTransferC[src[0]]
	p.destRow[p.destOff+1] = p.s.state.cmykTransferM[src[1]]
	p.destRow[p.destOff+2] = p.s.state.cmykTransferY[src[2]]
	p.destRow[p.destOff+3] = p.s.state.cmykTransferK[src[3]]
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff += 4
	p.x++
}

// pipeRunAACMYK8 mirrors Splash::pipeRunAACMYK8 (Splash.cc:1102-1152).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunAACMYK8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(pipePatternX(p), p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 4
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += 4
			p.x++
			return
		}
		src = c
	}
	d0 := p.destRow[p.destOff+0]
	d1 := p.destRow[p.destOff+1]
	d2 := p.destRow[p.destOff+2]
	d3 := p.destRow[p.destOff+3]
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	}
	aSrc := pipeSourceAlpha(p)
	aResult, alpha2, _ := pipeResultAlphas(p, aSrc, aDest)

	var c0, c1, c2, c3 byte
	if alpha2 == 0 {
		c0, c1, c2, c3 = 0, 0, 0, 0
	} else {
		c0 = p.s.state.cmykTransferC[byte(((alpha2-int(aSrc))*int(d0)+int(aSrc)*int(src[0]))/alpha2)]
		c1 = p.s.state.cmykTransferM[byte(((alpha2-int(aSrc))*int(d1)+int(aSrc)*int(src[1]))/alpha2)]
		c2 = p.s.state.cmykTransferY[byte(((alpha2-int(aSrc))*int(d2)+int(aSrc)*int(src[2]))/alpha2)]
		c3 = p.s.state.cmykTransferK[byte(((alpha2-int(aSrc))*int(d3)+int(aSrc)*int(src[3]))/alpha2)]
	}
	p.destRow[p.destOff+0] = c0
	p.destRow[p.destOff+1] = c1
	p.destRow[p.destOff+2] = c2
	p.destRow[p.destOff+3] = c3
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = aResult
		p.aDestOff++
	}
	p.destOff += 4
	p.x++
}

// pipeRunSimpleDeviceN8 mirrors Splash::pipeRunSimpleDeviceN8 (Splash.cc:847-861).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunSimpleDeviceN8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(pipePatternX(p), p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += splashMaxColorComps
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += splashMaxColorComps
			p.x++
			return
		}
		src = c
	}
	for i := 0; i < splashMaxColorComps; i++ {
		p.destRow[p.destOff+i] = p.s.state.deviceNTransfer[i][src[i]]
	}
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff += splashMaxColorComps
	p.x++
}

// pipeRunAADeviceN8 mirrors Splash::pipeRunAADeviceN8 (Splash.cc:1159-1202).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunAADeviceN8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(pipePatternX(p), p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += splashMaxColorComps
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff += splashMaxColorComps
			p.x++
			return
		}
		src = c
	}
	var dest [splashMaxColorComps]byte
	for i := 0; i < splashMaxColorComps; i++ {
		dest[i] = p.destRow[p.destOff+i]
	}
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	}
	aSrc := pipeSourceAlpha(p)
	aResult, alpha2, _ := pipeResultAlphas(p, aSrc, aDest)

	for i := 0; i < splashMaxColorComps; i++ {
		var c byte
		if alpha2 == 0 {
			c = 0
		} else {
			c = p.s.state.deviceNTransfer[i][byte(((alpha2-int(aSrc))*int(dest[i])+int(aSrc)*int(src[i]))/alpha2)]
		}
		p.destRow[p.destOff+i] = c
	}
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = aResult
		p.aDestOff++
	}
	p.destOff += splashMaxColorComps
	p.x++
}

// pipeRunSimpleCMYK8NoPat is the allocation-free fast path of pipeRunSimpleCMYK8 (when p.pattern == nil).
func pipeRunSimpleCMYK8NoPat(p *pipe) {
	src := p.cSrc
	p.destRow[p.destOff+0] = p.s.state.cmykTransferC[src[0]]
	p.destRow[p.destOff+1] = p.s.state.cmykTransferM[src[1]]
	p.destRow[p.destOff+2] = p.s.state.cmykTransferY[src[2]]
	p.destRow[p.destOff+3] = p.s.state.cmykTransferK[src[3]]
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff += 4
	p.x++
}

// pipeRunAACMYK8NoPat is the allocation-free fast path of pipeRunAACMYK8 (when p.pattern == nil and p.blendFunc == nil).
func pipeRunAACMYK8NoPat(p *pipe) {
	src := p.cSrc
	d0 := p.destRow[p.destOff+0]
	d1 := p.destRow[p.destOff+1]
	d2 := p.destRow[p.destOff+2]
	d3 := p.destRow[p.destOff+3]
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	}
	aSrc := pipeSourceAlpha(p)
	aResult, alpha2, _ := pipeResultAlphas(p, aSrc, aDest)

	var c0, c1, c2, c3 byte
	if alpha2 == 0 {
		c0, c1, c2, c3 = 0, 0, 0, 0
	} else {
		c0 = p.s.state.cmykTransferC[byte(((alpha2-int(aSrc))*int(d0)+int(aSrc)*int(src[0]))/alpha2)]
		c1 = p.s.state.cmykTransferM[byte(((alpha2-int(aSrc))*int(d1)+int(aSrc)*int(src[1]))/alpha2)]
		c2 = p.s.state.cmykTransferY[byte(((alpha2-int(aSrc))*int(d2)+int(aSrc)*int(src[2]))/alpha2)]
		c3 = p.s.state.cmykTransferK[byte(((alpha2-int(aSrc))*int(d3)+int(aSrc)*int(src[3]))/alpha2)]
	}
	p.destRow[p.destOff+0] = c0
	p.destRow[p.destOff+1] = c1
	p.destRow[p.destOff+2] = c2
	p.destRow[p.destOff+3] = c3
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = aResult
		p.aDestOff++
	}
	p.destOff += 4
	p.x++
}

// pipeRunSimpleDeviceN8NoPat is the allocation-free fast path of pipeRunSimpleDeviceN8 (when p.pattern == nil).
func pipeRunSimpleDeviceN8NoPat(p *pipe) {
	src := p.cSrc
	for i := 0; i < splashMaxColorComps; i++ {
		p.destRow[p.destOff+i] = p.s.state.deviceNTransfer[i][src[i]]
	}
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff += splashMaxColorComps
	p.x++
}

// pipeRunAADeviceN8NoPat is the allocation-free fast path of pipeRunAADeviceN8 (when p.pattern == nil and p.blendFunc == nil).
func pipeRunAADeviceN8NoPat(p *pipe) {
	src := p.cSrc
	var dest [splashMaxColorComps]byte
	for i := 0; i < splashMaxColorComps; i++ {
		dest[i] = p.destRow[p.destOff+i]
	}
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	}
	aSrc := pipeSourceAlpha(p)
	aResult, alpha2, _ := pipeResultAlphas(p, aSrc, aDest)

	for i := 0; i < splashMaxColorComps; i++ {
		var c byte
		if alpha2 == 0 {
			c = 0
		} else {
			c = p.s.state.deviceNTransfer[i][byte(((alpha2-int(aSrc))*int(dest[i])+int(aSrc)*int(src[i]))/alpha2)]
		}
		p.destRow[p.destOff+i] = c
	}
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = aResult
		p.aDestOff++
	}
	p.destOff += splashMaxColorComps
	p.x++
}
