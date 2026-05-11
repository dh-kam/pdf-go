// Package canvas provides canvas implementations for PDF rendering.
package canvas

import (
	"image"
	"log"
	"sync"

	domaincanvas "github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/infrastructure/splash"
)

// Backend identifiers for NewCanvas. Use these constants rather than string
// literals at call sites so the compiler catches typos.
const (
	BackendImageCanvas = "image-canvas"
	BackendSplash      = "splash"
)

var splashWarnOnce sync.Once

// NewCanvas returns the rendering canvas selected by backend.
//
// Valid values:
//   - "image-canvas" (or empty): the default legacy ImageCanvas backend.
//   - "splash": the experimental splash backend (Phase 1+, partial coverage).
//
// All primitives route to the selected backend; there is no per-primitive
// override. Unknown values fall back to image-canvas with a warning so the
// renderer cannot accidentally produce a nil canvas — but pdfrender's CLI
// validation should reject unknown values before reaching this function.
func NewCanvas(width, height int, backend string) domaincanvas.Canvas {
	switch backend {
	case BackendSplash:
		splashWarnOnce.Do(func() {
			log.Println("splash: --backend=splash — using experimental splash backend (Phase 1+, partial coverage)")
		})
		return splash.NewBackend(width, height)
	case BackendImageCanvas, "":
		return NewImageCanvas(image.Rect(0, 0, width, height))
	default:
		log.Printf("canvas: unknown backend %q, falling back to %q", backend, BackendImageCanvas)
		return NewImageCanvas(image.Rect(0, 0, width, height))
	}
}
