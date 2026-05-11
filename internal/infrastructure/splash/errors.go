package splash

import "errors"

// Sentinel errors mirroring SplashError codes (SplashErrorCodes.h).
var (
	// ErrNoCurPt mirrors splashErrNoCurPt (SplashErrorCodes.h).
	ErrNoCurPt = errors.New("splash: no current point")
	// ErrEmptyPath mirrors splashErrEmptyPath (SplashErrorCodes.h).
	ErrEmptyPath = errors.New("splash: empty path")
	// ErrBogusPath mirrors splashErrBogusPath (SplashErrorCodes.h).
	ErrBogusPath = errors.New("splash: bogus path")
	// ErrNoSave mirrors splashErrNoSave (SplashErrorCodes.h).
	ErrNoSave = errors.New("splash: state stack is empty")
	// ErrOpenFile mirrors splashErrOpenFile (SplashErrorCodes.h).
	ErrOpenFile = errors.New("splash: couldn't open file")
	// ErrNoGlyph mirrors splashErrNoGlyph (SplashErrorCodes.h).
	ErrNoGlyph = errors.New("splash: glyph not available")
	// ErrModeMismatch mirrors splashErrModeMismatch (SplashErrorCodes.h).
	ErrModeMismatch = errors.New("splash: color mode mismatch")
	// ErrSingularMatrix mirrors splashErrSingularMatrix (SplashErrorCodes.h).
	ErrSingularMatrix = errors.New("splash: singular matrix")
	// ErrBadArg mirrors splashErrBadArg (SplashErrorCodes.h).
	ErrBadArg = errors.New("splash: bad argument")
	// ErrZeroImage mirrors splashErrZeroImage (SplashErrorCodes.h).
	ErrZeroImage = errors.New("splash: image of size 0x0")
	// ErrGeneric mirrors splashErrGeneric (SplashErrorCodes.h).
	ErrGeneric = errors.New("splash: generic error")
)

var errNotImplemented = errors.New("splash: not implemented")
