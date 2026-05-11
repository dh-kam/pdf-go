// Package xpath contains path/edge/scanner/clip primitives ported from Poppler 24.02 Splash
// (SplashPath/SplashXPath/SplashXPathScanner/SplashClip). It is the geometric kernel of the
// Splash port and has no dependency on the parent splash package.
package xpath

import "errors"

// errNotImplemented is the sentinel returned by skeleton bodies during Phase 0.
var errNotImplemented = errors.New("xpath: not implemented")
