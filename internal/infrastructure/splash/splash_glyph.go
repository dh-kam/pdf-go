package splash

import (
	"fmt"
	"image"
	"math"
	"os"

	"golang.org/x/image/vector"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// fillGlyph blits a pre-rasterized GlyphBitmap at user-space (x,y) (Splash.cc:2603-2615).
func (s *Splash) fillGlyph(x, y float64, g *GlyphBitmap) error {
	if g == nil || g.Data == nil || s.bitmap == nil || s.bitmap.data == nil {
		return ErrBadArg
	}
	xt, yt := x*s.state.matrix[0]+y*s.state.matrix[2]+s.state.matrix[4],
		x*s.state.matrix[1]+y*s.state.matrix[3]+s.state.matrix[5]
	x0 := Floor(xt)
	y0 := Floor(yt)
	fillGlyph2(s, x0, y0, g)
	return nil
}

// fillGlyph2 mirrors Splash::fillGlyph2 (Splash.cc:2618-2738).
func fillGlyph2(s *Splash, x0, y0 int, g *GlyphBitmap) {
	xStart := x0 - g.X
	yStart := y0 - g.Y
	xxLimit := g.W
	yyLimit := g.H
	xShift := 0
	srcOff := 0

	rowStrideSrc := g.W
	if !g.AA {
		rowStrideSrc = (g.W + 7) / 8
	}

	if yStart < 0 {
		srcOff += rowStrideSrc * (-yStart)
		yyLimit += yStart
		yStart = 0
	}
	if xStart < 0 {
		if g.AA {
			srcOff += -xStart
		} else {
			srcOff += (-xStart) / 8
			xShift = (-xStart) % 8
		}
		xxLimit += xStart
		xStart = 0
	}
	if xxLimit+xStart >= s.bitmap.width {
		xxLimit = s.bitmap.width - xStart
	}
	if yyLimit+yStart >= s.bitmap.height {
		yyLimit = s.bitmap.height - yStart
	}
	if xxLimit <= 0 || yyLimit <= 0 {
		return
	}

	clip, _ := s.state.clip.(*xpath.Clip)
	var p pipe
	aInput := byte(Round(s.state.fillAlpha * 255))

	if g.AA {
		s.pipeInit(&p, xStart, yStart, s.state.fillPattern, nil, aInput, true, false)
		for yy := 0; yy < yyLimit; yy++ {
			dstY := yStart + yy
			clipRes := glyphRowClipResult(clip, xStart, xStart+xxLimit-1, dstY)
			if clipRes == xpath.ClipAllOutside {
				continue
			}
			s.pipeSetXY(&p, xStart, dstY)
			rowBase := srcOff + yy*g.W
			for xx := 0; xx < xxLimit; xx++ {
				if clipRes == xpath.ClipPartial && glyphPixelOutsideClip(clip, xStart+xx, dstY) {
					s.pipeIncX(&p)
					continue
				}
				alpha := g.Data[rowBase+xx]
				if alpha != 0 {
					dstX := xStart + xx
					if shouldTraceGlyphPixel(dstX, dstY) {
						traceGlyphPixelBefore(&p, dstX, dstY, xx, yy, alpha, g, xStart, yStart)
					}
					p.shape = alpha
					p.run(&p)
					if shouldTraceGlyphPixel(dstX, dstY) {
						traceGlyphPixelAfter(&p, dstX, dstY)
					}
				} else {
					s.pipeIncX(&p)
				}
			}
		}
	} else {
		widthEight := (g.W + 7) / 8
		s.pipeInit(&p, xStart, yStart, s.state.fillPattern, nil, aInput, false, false)
		for yy := 0; yy < yyLimit; yy++ {
			dstY := yStart + yy
			clipRes := glyphRowClipResult(clip, xStart, xStart+xxLimit-1, dstY)
			if clipRes == xpath.ClipAllOutside {
				continue
			}
			s.pipeSetXY(&p, xStart, dstY)
			rowBase := srcOff + yy*widthEight
			for xx := 0; xx < xxLimit; xx += 8 {
				var alpha0 byte
				if xShift > 0 && xx < xxLimit-8 {
					alpha0 = g.Data[rowBase+xx/8]<<xShift | g.Data[rowBase+xx/8+1]>>(8-xShift)
				} else {
					alpha0 = g.Data[rowBase+xx/8]
				}
				for xx1 := 0; xx1 < 8 && xx+xx1 < xxLimit; xx1++ {
					if clipRes == xpath.ClipPartial && glyphPixelOutsideClip(clip, xStart+xx+xx1, dstY) {
						s.pipeIncX(&p)
						alpha0 <<= 1
						continue
					}
					if alpha0&0x80 != 0 {
						p.shape = 255
						p.run(&p)
					} else {
						s.pipeIncX(&p)
					}
					alpha0 <<= 1
				}
			}
		}
	}
}

func glyphRowClipResult(clip *xpath.Clip, x0, x1, y int) xpath.ClipResult {
	if clip == nil {
		return xpath.ClipAllInside
	}
	return clip.TestSpan(x0, x1, y)
}

func glyphPixelOutsideClip(clip *xpath.Clip, x, y int) bool {
	return clip != nil && !clip.Test(x, y)
}

var glyphTracePixels = parseSplashTracePixels(os.Getenv("PDF_DEBUG_SPLASH_GLYPH_TRACE"))

func shouldTraceGlyphPixel(x, y int) bool {
	for _, pixel := range glyphTracePixels {
		if pixel.x == x && pixel.y == y {
			return true
		}
	}
	return false
}

func traceGlyphPixelBefore(p *pipe, x, y, glyphX, glyphY int, alpha byte, g *GlyphBitmap, xStart, yStart int) {
	if p.colorBytesPerPixel < 3 || p.destOff+2 >= len(p.destRow) {
		return
	}
	aDest := byte(0xff)
	if p.aDestRow != nil && p.aDestOff < len(p.aDestRow) {
		aDest = p.aDestRow[p.aDestOff]
	}
	src := p.cSrc
	if p.pattern != nil {
		var patternSrc Color
		if p.pattern.GetColor(x, y, &patternSrc) {
			src = patternSrc
		}
	}
	fmt.Fprintf(os.Stderr, "SPLASH_GLYPH_TRACE before x=%d y=%d glyphXY=(%d,%d) alpha=%d g=(x:%d y:%d w:%d h:%d aa:%t) start=(%d,%d) src=(%d,%d,%d) dst=(%d,%d,%d) aDest=%d\n",
		x, y, glyphX, glyphY, alpha, g.X, g.Y, g.W, g.H, g.AA, xStart, yStart,
		src[0], src[1], src[2],
		p.destRow[p.destOff], p.destRow[p.destOff+1], p.destRow[p.destOff+2], aDest)
}

func traceGlyphPixelAfter(p *pipe, x, y int) {
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
	fmt.Fprintf(os.Stderr, "SPLASH_GLYPH_TRACE after x=%d y=%d dst=(%d,%d,%d) aDest=%d\n",
		x, y, p.destRow[off], p.destRow[off+1], p.destRow[off+2], aDest)
}

// RasterizeGlyph rasterizes a glyph outline path into an 8-bit alpha
// GlyphBitmap (per D1 LOCKED 2026-04-27). Splash-native counterpart to
// canvas/glyph_raster_*.go; the two coexist by design.
//
// Algorithm: golang.org/x/image/vector analytic-coverage rasterizer (the same
// engine ImageCanvas's go-vector strategy uses, supersample factor 1 — vector
// already produces analytic edge coverage matching Poppler/Splash glyph
// bitmaps from FreeType). The earlier 4×4 box-AA supersample with aaGamma 1.5
// produced consistently lighter edge alpha (e.g. shape 166 vs ref 184 at a
// vertical Arial stroke edge) because Splash::fillGlyph2 (Splash.cc:2618-2738)
// blits the FreeType-produced 8-bit alpha as-is — fillGlyph2 does NOT remap
// shape through aaGamma. Using analytic-coverage linear rasterization mirrors
// FreeType output, restoring 011-google-doc-document p1 parity to ≥99 %.
func RasterizeGlyph(p *xpath.Path, dpi float64, hint bool) *GlyphBitmap {
	_ = hint
	if p == nil || p.IsEmpty() {
		return &GlyphBitmap{AA: true}
	}
	scale := dpi
	if scale <= 0 {
		scale = 1
	}

	// Bounding box in device space at "scale" (matches the legacy box-AA path
	// so glyph X/Y origin placement is unchanged — vector.Rasterizer clips
	// to the bitmap so a slightly-larger bbox is harmless).
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i := 0; i < p.Length(); i++ {
		pt, _ := p.Point(i)
		dx := pt.X * scale
		dy := pt.Y * scale
		if dx < minX {
			minX = dx
		}
		if dx > maxX {
			maxX = dx
		}
		if dy < minY {
			minY = dy
		}
		if dy > maxY {
			maxY = dy
		}
	}
	x0 := Floor(minX)
	y0 := Floor(minY)
	x1 := Ceil(maxX)
	y1 := Ceil(maxY)
	w := x1 - x0
	h := y1 - y0
	if w <= 0 || h <= 0 {
		return &GlyphBitmap{X: -x0, Y: -y0, W: 0, H: 0, AA: true}
	}

	ras := vector.NewRasterizer(w, h)
	var subStart [2]float64
	var subValid bool
	for i := 0; i < p.Length(); {
		pt, fl := p.Point(i)
		px := pt.X*scale - float64(x0)
		py := pt.Y*scale - float64(y0)
		if fl&0x01 != 0 { // pathFirst
			if subValid {
				// implicit close: vector requires explicit ClosePath when
				// the path indicates pathClosed; otherwise leave the subpath
				// open (the previous LineTo chain still rasterises correctly).
			}
			subStart = [2]float64{px, py}
			subValid = true
			ras.MoveTo(float32(px), float32(py))
			if fl&0x02 != 0 { // single-point subpath (pathFirst|pathLast).
				if fl&0x04 != 0 {
					ras.ClosePath()
				}
				subValid = false
			}
			i++
			continue
		}
		if fl&0x08 != 0 && i+2 < p.Length() {
			// Curve: this point is the first control, next is the second
			// control, the one after carries pathLast/pathClosed and is the
			// curve endpoint.
			pt2, _ := p.Point(i + 1)
			pt3, fl3 := p.Point(i + 2)
			c1x := pt.X*scale - float64(x0)
			c1y := pt.Y*scale - float64(y0)
			c2x := pt2.X*scale - float64(x0)
			c2y := pt2.Y*scale - float64(y0)
			ex := pt3.X*scale - float64(x0)
			ey := pt3.Y*scale - float64(y0)
			ras.CubeTo(float32(c1x), float32(c1y), float32(c2x), float32(c2y), float32(ex), float32(ey))
			if fl3&0x04 != 0 && subValid {
				ras.LineTo(float32(subStart[0]), float32(subStart[1]))
				ras.ClosePath()
				subValid = false
			}
			i += 3
			continue
		}
		// Plain LineTo endpoint.
		ras.LineTo(float32(px), float32(py))
		if fl&0x04 != 0 && subValid {
			ras.LineTo(float32(subStart[0]), float32(subStart[1]))
			ras.ClosePath()
			subValid = false
		}
		i++
	}

	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	ras.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	out := make([]byte, w*h)
	copy(out, mask.Pix)

	return &GlyphBitmap{X: -x0, Y: -y0, W: w, H: h, AA: true, Data: out}
}
