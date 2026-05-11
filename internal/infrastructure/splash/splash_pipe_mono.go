package splash

// pipeRunSimpleMono8 mirrors Splash::pipeRunSimpleMono8 (Splash.cc:768-775).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunSimpleMono8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(p.x, p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff++
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff++
			p.x++
			return
		}
		src = c
	}
	p.destRow[p.destOff] = src[0]
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = 255
		p.aDestOff++
	}
	p.destOff++
	p.x++
}

// pipeRunAAMono8 mirrors Splash::pipeRunAAMono8 (Splash.cc:903-932).
// Dynamic-pattern branch mirrors Splash.cc:312-316.
func pipeRunAAMono8(p *pipe) {
	src := p.cSrc
	if p.pattern != nil {
		var c Color
		if !p.pattern.GetColor(p.x, p.y, &c) {
			// Splash.cc:313-315: pattern hole — advance cursor and skip pixel.
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff++
			p.x++
			return
		}
		if !pipeSetPatternAlpha(p) {
			if p.aDestRow != nil {
				p.aDestOff++
			}
			p.destOff++
			p.x++
			return
		}
		src = c
	}
	dest := p.destRow[p.destOff]
	var aDest byte
	if p.aDestRow != nil {
		aDest = p.aDestRow[p.aDestOff]
	}
	aSrc := pipeSourceAlpha(p)
	aResult := aSrc + aDest - byte(Div255(int(aSrc)*int(aDest)))
	alpha2 := aResult
	var c0 byte
	if alpha2 == 0 {
		c0 = 0
	} else {
		c0 = byte((int(alpha2-aSrc)*int(dest) + int(aSrc)*int(src[0])) / int(alpha2))
	}
	p.destRow[p.destOff] = c0
	if p.aDestRow != nil {
		p.aDestRow[p.aDestOff] = aResult
		p.aDestOff++
	}
	p.destOff++
	p.x++
}
