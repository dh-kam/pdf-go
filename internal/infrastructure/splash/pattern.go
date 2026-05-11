package splash

// Pattern is the source-color provider for fill and stroke (SplashPattern.h:34).
type Pattern interface {
	// GetColor mirrors SplashPattern::getColor (SplashPattern.h:47).
	GetColor(x, y int, c *Color) bool
	// TestPosition mirrors SplashPattern::testPosition (SplashPattern.h:50).
	TestPosition(x, y int) bool
	// IsStatic mirrors SplashPattern::isStatic (SplashPattern.h:54).
	IsStatic() bool
	// IsCMYK mirrors SplashPattern::isCMYK (SplashPattern.h:57).
	IsCMYK() bool
}

// AlphaPattern is an optional Pattern extension for sources that provide a
// per-pixel alpha plane separate from their source color.
type AlphaPattern interface {
	PatternAlpha(x, y int) byte
}

// SolidColor is a constant-color Pattern (SplashPattern.h:66).
type SolidColor struct {
	c Color
}

// NewSolidColor constructs a SolidColor (SplashPattern.h:66).
func NewSolidColor(c Color) *SolidColor {
	return &SolidColor{c: c}
}

// GetColor mirrors SplashSolidColor::getColor (SplashPattern.h:75).
func (p *SolidColor) GetColor(x, y int, c *Color) bool {
	if c == nil {
		return false
	}
	*c = p.c
	return true
}

// TestPosition mirrors SplashSolidColor::testPosition (SplashPattern.h:77).
func (p *SolidColor) TestPosition(x, y int) bool { return false }

// IsStatic mirrors SplashSolidColor::isStatic (SplashPattern.h:79).
func (p *SolidColor) IsStatic() bool { return true }

// IsCMYK mirrors SplashSolidColor::isCMYK (SplashPattern.h:81).
func (p *SolidColor) IsCMYK() bool { return false }
