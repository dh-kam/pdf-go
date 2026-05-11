package splash

import (
	"math"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// makeDashedPath converts the solid path p into a dashed path using the
// state.lineDash array and state.lineDashPhase (Splash.cc:2162-2280).
//
// The function walks every subpath of p, splitting each line segment into
// alternating on/off chunks driven by the dash array. Curves should already
// be flattened by the caller (strokeImpl flattens via xpath.NewXPath before
// invoking this helper, mirroring Splash.cc:1899-1908).
func (s *Splash) makeDashedPath(p *xpath.Path) (*xpath.Path, error) {
	dPath := xpath.NewPath()
	if p == nil || p.IsEmpty() {
		return dPath, nil
	}

	// Sum dash array; an all-zero dash means "draw nothing" per Acrobat
	// (Splash.cc:2171-2178).
	var lineDashTotal float64
	for _, d := range s.state.lineDash {
		lineDashTotal += d
	}
	if lineDashTotal == 0 {
		return dPath, nil
	}

	// Reduce phase modulo total length (Splash.cc:2179-2181).
	lineDashStartPhase := s.state.lineDashPhase
	i := Floor(lineDashStartPhase / lineDashTotal)
	lineDashStartPhase -= float64(i) * lineDashTotal

	// Find the dash entry the phase lands inside (Splash.cc:2182-2193).
	lineDashStartOn := true
	lineDashStartIdx := 0
	if lineDashStartPhase > 0 {
		for lineDashStartIdx < len(s.state.lineDash) && lineDashStartPhase >= s.state.lineDash[lineDashStartIdx] {
			lineDashStartOn = !lineDashStartOn
			lineDashStartPhase -= s.state.lineDash[lineDashStartIdx]
			lineDashStartIdx++
		}
		if lineDashStartIdx == len(s.state.lineDash) {
			return dPath, nil
		}
	}

	n := p.Length()
	idx := 0
	for idx < n {
		// Find end of subpath: index j such that flags[j] has pathFlagLast or it's
		// the final point in the path (Splash.cc:2202-2204).
		j := idx
		for j < n-1 {
			_, fj := p.Point(j)
			if fj&pathFlagLast != 0 {
				break
			}
			j++
		}

		lineDashOn := lineDashStartOn
		lineDashIdx := lineDashStartIdx
		lineDashDist := s.state.lineDash[lineDashIdx] - lineDashStartPhase

		newPath := true
		for k := idx; k < j; k++ {
			pa, _ := p.Point(k)
			pb, _ := p.Point(k + 1)
			x0, y0 := pa.X, pa.Y
			x1, y1 := pb.X, pb.Y
			segLen := math.Hypot(x1-x0, y1-y0)

			for segLen > 0 {
				if lineDashDist >= segLen {
					if lineDashOn {
						if newPath {
							_ = dPath.MoveTo(x0, y0)
							newPath = false
						}
						_ = dPath.LineTo(x1, y1)
					}
					lineDashDist -= segLen
					segLen = 0
				} else {
					t := lineDashDist / segLen
					xa := x0 + t*(x1-x0)
					ya := y0 + t*(y1-y0)
					if lineDashOn {
						if newPath {
							_ = dPath.MoveTo(x0, y0)
							newPath = false
						}
						_ = dPath.LineTo(xa, ya)
					}
					x0 = xa
					y0 = ya
					segLen -= lineDashDist
					lineDashDist = 0
				}

				if lineDashDist <= 0 {
					lineDashOn = !lineDashOn
					lineDashIdx++
					if lineDashIdx == len(s.state.lineDash) {
						lineDashIdx = 0
					}
					lineDashDist = s.state.lineDash[lineDashIdx]
					newPath = true
				}
			}
		}
		idx = j + 1
	}

	// Acrobat fallback for degenerate input: if the dashed path is empty but the
	// original path collapses to a single point, emit a zero-length subpath at
	// that point (Splash.cc:2266-2277).
	if dPath.IsEmpty() {
		allSame := true
		for k := 0; k < n-1 && allSame; k++ {
			pa, _ := p.Point(k)
			pb, _ := p.Point(k + 1)
			if pa.X != pb.X || pa.Y != pb.Y {
				allSame = false
			}
		}
		if allSame && n > 0 {
			pa, _ := p.Point(0)
			_ = dPath.MoveTo(pa.X, pa.Y)
			_ = dPath.LineTo(pa.X, pa.Y)
		}
	}

	return dPath, nil
}
