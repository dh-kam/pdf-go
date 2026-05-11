package splash

import "math"

// AAGammaExp is splashAAGamma (Splash.cc:66): the gamma exponent used
// to build the antialias coverage LUT.
const AAGammaExp = 1.5

// aaSize is splashAASize (SplashTypes.h:46): the sub-pixel scale factor.
const aaSize = 4

// aaGammaLen is splashAASize*splashAASize+1 = 17 (Splash.h:325).
const aaGammaLen = aaSize*aaSize + 1

// Floor returns int(floor(x)) using the portable branch of splashFloor (SplashMath.h:80-86).
func Floor(x float64) int {
	if x > 0 {
		return int(x)
	}
	return int(math.Floor(x))
}

// Ceil returns int(ceil(x)) per splashCeil (SplashMath.h:130).
func Ceil(x float64) int {
	return int(math.Ceil(x))
}

// Round returns Floor(x+0.5) per splashRound (SplashMath.h:175); NOT banker's rounding.
func Round(x float64) int {
	return Floor(x + 0.5)
}

// Avg returns 0.5*(x+y) per splashAvg (SplashMath.h:179-182).
func Avg(x, y float64) float64 {
	return 0.5 * (x + y)
}

// Div255 maps [0,255*255] -> [0,255] via (x+(x>>8)+0x80)>>8 (Splash.cc:74-77).
func Div255(x int) int {
	return (x + (x >> 8) + 0x80) >> 8
}

// AAGamma returns the 17-entry LUT aaGamma[i] = round(pow(i/16, 1.5)*255) (Splash.cc:1455-1457).
func AAGamma() [aaGammaLen]uint8 {
	var lut [aaGammaLen]uint8
	for i := 0; i < aaGammaLen; i++ {
		lut[i] = uint8(Round(math.Pow(float64(i)/float64(aaGammaLen-1), AAGammaExp) * 255))
	}
	return lut
}
