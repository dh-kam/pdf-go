package splash

// Benchmark suite for splash hot paths (SP4 Phase 5, R8/R9 mitigation).
//
// All benchmarks share an RGB8 1920x1080 destination unless otherwise noted.
// Each benchmark builds its inputs OUTSIDE the timed region (b.ResetTimer()
// after setup) so allocations attributed to the benchmark are only those
// the hot path itself produces. b.ReportAllocs() forces the testing
// framework to surface B/op + allocs/op alongside ns/op.
//
// Run:
//   go test -bench=. -benchmem -count=10 ./internal/infrastructure/splash/...

import (
	"testing"

	"github.com/dh-kam/pdf-go/internal/infrastructure/splash/xpath"
)

// ---------------------------------------------------------------------------
// shared bench fixtures
// ---------------------------------------------------------------------------

// newBenchSplashRGB returns an RGB8 Splash with paper-white background and a
// solid-color fill pattern installed. vectorAA toggles the AA pipeline.
func newBenchSplashRGB(b *testing.B, w, h int, vectorAA bool) *Splash {
	b.Helper()
	bm := NewBitmap(w, h, ModeRGB8, false)
	bm.Clear(Color{0xFF, 0xFF, 0xFF})
	s, err := New(bm, vectorAA)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	s.SetFillPattern(NewSolidColor(Color{0x00, 0x00, 0x00}))
	s.SetFillAlpha(1)
	return s
}

// benchRectPath builds a closed axis-aligned rectangle path.
func benchRectPath(b *testing.B, x0, y0, x1, y1 float64) *xpath.Path {
	b.Helper()
	p := xpath.NewPath()
	if err := p.MoveTo(x0, y0); err != nil {
		b.Fatalf("MoveTo: %v", err)
	}
	if err := p.LineTo(x1, y0); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(x1, y1); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(x0, y1); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		b.Fatalf("Close: %v", err)
	}
	return p
}

// benchDiagonalEdgePath builds a path with N short, near-diagonal segments to
// stress the AA scanner edge list. Each "tooth" contributes 2 sloped edges
// that share neither x nor y, forcing the scanner to compute intersections
// per scanline. The path is closed so even-odd fill produces ~edges spans.
func benchDiagonalEdgePath(b *testing.B, edges, width, height int) *xpath.Path {
	b.Helper()
	p := xpath.NewPath()
	// MoveTo top-left; build a saw-tooth descending diagonally.
	if err := p.MoveTo(0, 0); err != nil {
		b.Fatalf("MoveTo: %v", err)
	}
	teeth := edges / 2
	if teeth < 1 {
		teeth = 1
	}
	dx := float64(width) / float64(teeth)
	dy := float64(height) / float64(teeth)
	x, y := 0.0, 0.0
	for i := 0; i < teeth; i++ {
		x += dx * 0.5
		y += dy
		if err := p.LineTo(x, y); err != nil {
			b.Fatalf("LineTo: %v", err)
		}
		x += dx * 0.5
		y -= dy * 0.5
		if err := p.LineTo(x, y); err != nil {
			b.Fatalf("LineTo: %v", err)
		}
	}
	if err := p.LineTo(float64(width), float64(height)); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.LineTo(0, float64(height)); err != nil {
		b.Fatalf("LineTo: %v", err)
	}
	if err := p.Close(false); err != nil {
		b.Fatalf("Close: %v", err)
	}
	return p
}

// constSrcRGB returns an ImageSource that fills every requested row with the
// provided per-pixel color. Allocation-free in the hot loop.
func constSrcRGB(srcW int, color [3]byte) ImageSource {
	return func(_ int, dst, _ []byte) error {
		for x := 0; x < srcW; x++ {
			dst[3*x+0] = color[0]
			dst[3*x+1] = color[1]
			dst[3*x+2] = color[2]
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// 1. BenchmarkSpanFillSolid — 1920x1080 axis-aligned solid rect via Splash.Fill.
// ---------------------------------------------------------------------------
//
// Measures the pipe inner loop on the simplest geometry (4 edges, 1 span/row).
// Captures the cost of per-row pipeRunSimpleRGB8 + scanner walk for a full HD
// frame. Hot path: splash_fill.go fillImpl → xpath scanner → pipe.run.
func BenchmarkSpanFillSolid(b *testing.B) {
	const W, H = 1920, 1080
	s := newBenchSplashRGB(b, W, H, true)
	p := benchRectPath(b, 0, 0, W, H)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Fill(p, false)
	}
}

// ---------------------------------------------------------------------------
// 2. BenchmarkAAEdgeScan — many sloped edges, full AA fill.
// ---------------------------------------------------------------------------
//
// Stresses the AA scanner hot loop indirectly. ~4000 edges across a 1024x768
// frame forces lots of per-row intersections, exercising the inner sort +
// nextSpan walk in xpath.Scanner.
func BenchmarkAAEdgeScan(b *testing.B) {
	const W, H = 1024, 768
	s := newBenchSplashRGB(b, W, H, true)
	p := benchDiagonalEdgePath(b, 4000, W, H)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Fill(p, false)
	}
}

// ---------------------------------------------------------------------------
// 3. BenchmarkBilinearUp — 64x64 RGB8 source → 1024x1024 dst.
// ---------------------------------------------------------------------------
//
// Triggers scaleImageYupXupBilinear: 16x scale-up on both axes with
// interpolate=true forces the bilinear path (isImageInterpolationRequired
// returns true for any explicit interpolate=true).
func BenchmarkBilinearUp(b *testing.B) {
	const SRC, DST = 64, 1024
	s := newBenchSplashRGB(b, DST, DST, false)
	src := constSrcRGB(SRC, [3]byte{0x80, 0x40, 0xC0})
	mat := [6]float64{float64(DST), 0, 0, float64(DST), 0, 0}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.DrawImage(src, SRC, SRC, mat, true)
	}
}

// ---------------------------------------------------------------------------
// 4. BenchmarkBilinearDown — 4096x4096 → 256x256 (downsample).
// ---------------------------------------------------------------------------
//
// Exercises the downsampling kernel + last-row clamp path that we recently
// fixed (lineBuf2 stale-data bug, 2026-04-26). Throughput regression here
// signals a memory-traffic problem, not an algorithmic one.
func BenchmarkBilinearDown(b *testing.B) {
	const SRC, DST = 4096, 256
	s := newBenchSplashRGB(b, DST, DST, false)
	src := constSrcRGB(SRC, [3]byte{0x80, 0x40, 0xC0})
	mat := [6]float64{float64(DST), 0, 0, float64(DST), 0, 0}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.DrawImage(src, SRC, SRC, mat, true)
	}
}

// ---------------------------------------------------------------------------
// 5. BenchmarkGlyphBlitAA — 1000 8-bit AA glyph blits at 12pt.
// ---------------------------------------------------------------------------
//
// Mimics a body-copy text page: 1000 small (12x16) AA glyphs scattered across
// a 1920x1080 frame. Hot path: splash_glyph.go fillGlyph2 AA branch →
// pipeRunAARGB8 (per-pixel shape blend).
func BenchmarkGlyphBlitAA(b *testing.B) {
	const W, H = 1920, 1080
	const GW, GH = 12, 16
	const NGLYPHS = 1000

	// One shared glyph bitmap with a diagonal stripe of partial-coverage alpha.
	gdata := make([]byte, GW*GH)
	for y := 0; y < GH; y++ {
		for x := 0; x < GW; x++ {
			if x == y || x+1 == y || x == y+1 {
				gdata[y*GW+x] = 200
			} else if (x+y)%4 == 0 {
				gdata[y*GW+x] = 80
			}
		}
	}
	g := &GlyphBitmap{X: 0, Y: 0, W: GW, H: GH, AA: true, Data: gdata}

	// Pre-compute 1000 (x,y) origins distributed across the frame.
	origins := make([][2]float64, NGLYPHS)
	for i := 0; i < NGLYPHS; i++ {
		origins[i][0] = float64((i * 13) % (W - GW))
		origins[i][1] = float64((i * 7) % (H - GH))
	}

	s := newBenchSplashRGB(b, W, H, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < NGLYPHS; j++ {
			_ = s.FillGlyph(origins[j][0], origins[j][1], g)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. BenchmarkBlendNormalSrcOver — 1920x1080 over 1920x1080, BlendNormal.
// ---------------------------------------------------------------------------
//
// Measures the pipe AA blend hot path under the default Normal blend (src
// captured at pipeInit time, no per-pixel BlendFunc call). usesShape=true
// forces the AA dispatch (pipeRunAARGB8) so per-pixel shape blending is
// exercised even though the blend formula is the trivial copy.
func BenchmarkBlendNormalSrcOver(b *testing.B) {
	const W, H = 1920, 1080
	s := newBenchSplashRGB(b, W, H, true)
	s.SetFillAlpha(0.5) // forces non-fast path → AA dispatch.
	p := benchRectPath(b, 0, 0, W, H)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Fill(p, false)
	}
}

// ---------------------------------------------------------------------------
// 7. BenchmarkBlendMultiply — same shape, BlendMultiply blend func.
// ---------------------------------------------------------------------------
//
// Same geometry as BenchmarkBlendNormalSrcOver but installs the Multiply
// blend func, forcing the per-pixel BlendFunc dispatch in pipeRunAARGB8.
// Difference vs BlendNormalSrcOver isolates the cost of the indirect call.
func BenchmarkBlendMultiply(b *testing.B) {
	const W, H = 1920, 1080
	s := newBenchSplashRGB(b, W, H, true)
	s.SetFillAlpha(0.5)
	s.SetBlendFunc(BlendMultiply)
	p := benchRectPath(b, 0, 0, W, H)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Fill(p, false)
	}
}
