//go:build nocairo

package canvas

// CairoRasterStrategy stub when Cairo is not available.
type CairoRasterStrategy struct{}

// IsCairoAvailable returns false when Cairo is not available.
func IsCairoAvailable() bool { return false }

// defaultCairoStrategyIfAvailable returns nil when Cairo is not available.
func defaultCairoStrategyIfAvailable() GlyphRasterStrategy { return nil }
