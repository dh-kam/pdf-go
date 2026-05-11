package splash

// splashMaxColorComps mirrors splashMaxColorComps (SplashTypes.h:86).
const splashMaxColorComps = 8

// splashAASize mirrors splashAASize (SplashTypes.h:46).
const splashAASize = 4

// Color is a fixed-size pixel value sized to splashMaxColorComps (SplashTypes.h:88).
type Color [splashMaxColorComps]byte

// Coord is the Splash coordinate type alias (SplashTypes.h:36-40).
type Coord = float64

// ColorMode selects the bitmap pixel layout (SplashTypes.h:56).
type ColorMode uint8

const (
	// ModeMono1 is splashModeMono1 (SplashTypes.h:58).
	ModeMono1 ColorMode = iota
	// ModeMono8 is splashModeMono8 (SplashTypes.h:60).
	ModeMono8
	// ModeRGB8 is splashModeRGB8 (SplashTypes.h:61).
	ModeRGB8
	// ModeBGR8 is splashModeBGR8 (SplashTypes.h:63).
	ModeBGR8
	// ModeXBGR8 is splashModeXBGR8 (SplashTypes.h:65).
	ModeXBGR8
	// ModeCMYK8 is splashModeCMYK8 (SplashTypes.h:67).
	ModeCMYK8
	// ModeDeviceN8 is splashModeDeviceN8 (SplashTypes.h:69).
	ModeDeviceN8
)

// LineCap mirrors the splashLineCap* constants (SplashState.h:37-39).
type LineCap int

const (
	// LineCapButt is splashLineCapButt (SplashState.h:37).
	LineCapButt LineCap = 0
	// LineCapRound is splashLineCapRound (SplashState.h:38).
	LineCapRound LineCap = 1
	// LineCapProjecting is splashLineCapProjecting (SplashState.h:39).
	LineCapProjecting LineCap = 2
)

// LineJoin mirrors the splashLineJoin* constants (SplashState.h:45-47).
type LineJoin int

const (
	// LineJoinMiter is splashLineJoinMiter (SplashState.h:45).
	LineJoinMiter LineJoin = 0
	// LineJoinRound is splashLineJoinRound (SplashState.h:46).
	LineJoinRound LineJoin = 1
	// LineJoinBevel is splashLineJoinBevel (SplashState.h:47).
	LineJoinBevel LineJoin = 2
)
