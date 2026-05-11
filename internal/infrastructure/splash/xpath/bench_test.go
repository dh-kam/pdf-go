package xpath

// Benchmark suite for xpath sub-package hot paths (SP4 Phase 5, R8/R9).
//
// The xpath package owns the AA scanner and path/curve flattening — both
// allocate per-fill in the production renderer, so micro-benchmarks here
// surface allocation regressions immediately.
//
// Run:
//   go test -bench=. -benchmem -count=10 \
//       ./internal/infrastructure/splash/xpath/...

import (
	"testing"
)

// ---------------------------------------------------------------------------
// shared bench fixtures
// ---------------------------------------------------------------------------

// benchCurvyPath builds a path with `segments` cubic curves stitched together.
// The control-point spacing forces moderate subdivision in addCurve, so the
// benchmark stresses both the recursion and segment-emit paths.
func benchCurvyPath(b *testing.B, segments int) *Path {
	b.Helper()
	p := NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		b.Fatalf("MoveTo: %v", err)
	}
	x, y := 0.0, 0.0
	for i := 0; i < segments; i++ {
		dx := float64((i%5)*8 + 4)
		dy := float64((i%3)*6 + 4)
		// Cubic with off-axis control points to avoid trivially-flat curves.
		x1 := x + dx*0.25
		y1 := y + dy
		x2 := x + dx*0.75
		y2 := y - dy*0.5
		x3 := x + dx
		y3 := y + dy*0.25
		if err := p.CurveTo(x1, y1, x2, y2, x3, y3); err != nil {
			b.Fatalf("CurveTo[%d]: %v", i, err)
		}
		x, y = x3, y3
	}
	return p
}

// benchScannerForRect builds a sorted XPath of a closed rect and returns its
// Scanner, ready to walk. Used by RenderAALine + ComputeIntersections benches.
func benchScannerForRect(b *testing.B, w, h float64) *Scanner {
	b.Helper()
	p := NewPath()
	if err := p.MoveTo(0, 0); err != nil {
		b.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(w, 0); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(w, h); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(0, h); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		b.Fatalf("Close: %v", err)
	}
	mat := [6]float64{1, 0, 0, 1, 0, 0}
	x := NewXPath(p, mat, 1.0, true)
	x.AAScale()
	x.Sort()
	xMin, yMin, xMax, yMax := 0, 0, int(w*aaSize), int(h*aaSize)
	return NewScanner(x, false, xMin, yMin, xMax, yMax)
}

// ---------------------------------------------------------------------------
// 1. BenchmarkXPathBuild — flatten a 200-segment cubic-curve path.
// ---------------------------------------------------------------------------
//
// Covers NewXPath + addCurve recursion. The path is built once outside the
// timed loop; only the flatten step is timed.
func BenchmarkXPathBuild(b *testing.B) {
	p := benchCurvyPath(b, 200)
	mat := [6]float64{1, 0, 0, 1, 0, 0}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewXPath(p, mat, 1.0, true)
	}
}

// ---------------------------------------------------------------------------
// 2. BenchmarkScannerComputeIntersections — sort + per-row intersections.
// ---------------------------------------------------------------------------
//
// Builds a fresh Scanner from a pre-flattened XPath each iteration. The
// Scanner ctor walks every segment, populates per-row intersect lists, and
// stable-sorts each row. This is the dominant cost on dense vector pages.
func BenchmarkScannerComputeIntersections(b *testing.B) {
	const W, H = 1024.0, 768.0
	// Pre-build the XPath ONCE outside the timed region.
	p := NewPath()
	_ = p.MoveTo(0, 0)
	_ = p.LineTo(W, 0)
	_ = p.LineTo(W, H)
	_ = p.LineTo(0, H)
	_ = p.Close(false)
	mat := [6]float64{1, 0, 0, 1, 0, 0}
	xp := NewXPath(p, mat, 1.0, true)
	xp.AAScale()
	xp.Sort()
	xMin, yMin, xMax, yMax := 0, 0, int(W*aaSize), int(H*aaSize)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewScanner(xp, false, xMin, yMin, xMax, yMax)
	}
}

// ---------------------------------------------------------------------------
// 3. BenchmarkRenderAALine — single-row AA bitmap rendering.
// ---------------------------------------------------------------------------
//
// The tightest hot loop inside vector fill: one device row, four sub-rows of
// Mono1 coverage. RenderAALine zeros aaBuf and walks the row's intersects.
// Per-iteration allocs should be 0 (aaBuf is reused).
func BenchmarkRenderAALine(b *testing.B) {
	const W, H = 1024.0, 16.0
	s := benchScannerForRect(b, W, H)
	xMin, _, xMax, _ := s.BBox()
	xMinDev := xMin / aaSize
	xMaxDev := xMax / aaSize
	width := (xMaxDev - xMinDev + 1) * aaSize
	rowSize := (width + 7) >> 3
	aaBuf := make([]byte, rowSize*aaSize)

	// Row sitting in the middle of the rect.
	yDev := int(H) / 2

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.RenderAALine(yDev, aaBuf, xMinDev, xMaxDev)
	}
}

// ---------------------------------------------------------------------------
// 4. BenchmarkClipCombineDeep — 8 nested rect ∩ path clips.
// ---------------------------------------------------------------------------
//
// Each ClipToPath grows the scanners array and triggers another flag-byte
// pass on subsequent TestRect / TestSpan. We measure clip-stack construction
// cost (rect intersect path ⊕ rect ⊕ path ... 8 deep), which corresponds
// to how nested form XObjects accumulate clips in real PDFs.
func BenchmarkClipCombineDeep(b *testing.B) {
	mat := [6]float64{1, 0, 0, 1, 0, 0}
	// Pre-build 4 distinct path-clips, alternated 8 times deep below.
	paths := make([]*Path, 4)
	for k := 0; k < 4; k++ {
		p := NewPath()
		ox := float64(20 + k*5)
		oy := float64(20 + k*5)
		_ = p.MoveTo(ox, oy)
		_ = p.LineTo(800-ox, oy)
		_ = p.LineTo(800-ox, 600-oy)
		_ = p.LineTo(ox, 600-oy)
		_ = p.Close(false)
		paths[k] = p
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := NewClip(0, 0, 1023, 767, true)
		for d := 0; d < 8; d++ {
			// Alternate rect ∩ path to mimic the nested-XObject case.
			if d%2 == 0 {
				_ = c.ClipToRect(float64(d), float64(d),
					float64(1023-d), float64(767-d))
			} else {
				_ = c.ClipToPath(paths[d%4], mat, 1.0, false)
			}
		}
	}
}
